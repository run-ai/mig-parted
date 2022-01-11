#!/usr/bin/env bash

WITH_REBOOT="false"
HOST_ROOT_MOUNT="/host"
NODE_NAME=""
MIG_CONFIG_FILE=""

function usage() {
  echo "USAGE:"
  echo "    ${0} -h "
  echo "    ${0} -n <node> -f <config-file> -c <selected-config> [ -r ]"
  echo ""
  echo "OPTIONS:"
  echo "    -h                   Display this help message"
  echo "    -r                   Automatically reboot the node if changing the MIG mode fails for any reason"
  echo "    -n <node>            The kubernetes node to change the MIG configuration on"
  echo "    -f <config>          The mig-parted configuration section"
  echo "    -m <host-root-mount> Target path where host root directory is mounted"
}

while getopts "hrn:f:m:" opt; do
  case ${opt} in
    h ) # process option h
      usage; exit 0
      ;;
    r ) # process option r
      WITH_REBOOT="true"
      ;;
    n ) # process option n
      NODE_NAME=${OPTARG}
      ;;
    f ) # process option f
      MIG_CONFIG_FILE=${OPTARG}
      ;;
    m ) # process option m
      HOST_ROOT_MOUNT=${OPTARG}
      ;;
    \? ) echo "Usage: ${0} -n <node> -f <config-file> -c <selected-config> [ -m <host-root-mount> -r ]"
      ;;
  esac
done

if [ "${NODE_NAME}" = "" ]; then
  echo "ERROR: missing -n <node> flag"
  usage; exit 1
fi
if [ "${MIG_CONFIG_FILE}" = "" ]; then
  echo "Error: missing -f <config-file> flag"
  usage; exit 1
fi

function __set_state_and_exit() {
	local state="${1}"
	local exit_code="${2}"

	echo "Changing the 'nvidia.com/mig.config.state' node label to '${state}'"
	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="${state}"
	if [ "${?}" != "0" ]; then
		echo "Unable to set 'nvidia.com/mig.config.state' to \'${state}\'"
		echo "Exiting with incorrect value in 'nvidia.com/mig.config.state'"
		exit 1
	fi

	exit ${exit_code}
}	

function exit_success() {
  MIG_DEVICES_UUID_MAP=$(nvidia-mig-parted export --placements -o json | base64 -w 0)
  echo ${MIG_DEVICES_UUID_MAP}
  kubectl annotate node ${NODE_NAME} --overwrite run.ai/mig-mapping=${MIG_DEVICES_UUID_MAP}
	__set_state_and_exit "success" 0
}

function exit_failed() {
	__set_state_and_exit "failed" 1
}

echo "Asserting that the requested configuration is present in the configuration file"

cat << EOF | nvidia-mig-parted assert --valid-config -f -
${MIG_CONFIG_FILE}
EOF

if [ "${?}" != "0" ]; then
	echo "Unable to validate the selected MIG configuration"
	exit_failed
fi

echo "Getting current value of the 'nvidia.com/mig.config.state' node label"
STATE=$(kubectl get node "${NODE_NAME}" -o=jsonpath='{.metadata.labels.nvidia\.com/mig\.config\.state}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/mig.config.state' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/mig.config.state=${STATE}'"

echo "Checking if the MIG mode setting in the selected config is currently applied or not"
echo "If the state is 'rebooting', we expect this to always return true"

cat << EOF | nvidia-mig-parted assert --mode-only -f -
${MIG_CONFIG_FILE}
EOF

if [ "${?}" != "0" ] && [ "${STATE}" == "rebooting" ]; then
	echo "MIG mode change did not take effect after rebooting"
	exit_failed
fi

echo "Checking if the selected MIG config is currently applied or not"

cat << EOF | nvidia-mig-parted assert -f -
${MIG_CONFIG_FILE}
EOF

if [ "${?}" = "0" ]; then
	exit_success
fi

echo "Changing the 'nvidia.com/mig.config.state' node label to 'pending'"
kubectl label --overwrite  \
	node ${NODE_NAME} \
	nvidia.com/mig.config.state="pending"
if [ "${?}" != "0" ]; then
	echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'"
	exit_failed
fi

echo "Applying the MIG mode change from the selected config to the node"
echo "If the -r option was passed, the node will be automatically rebooted if this is not successful"

cat << EOF | nvidia-mig-parted -d apply --mode-only -f -
${MIG_CONFIG_FILE}
EOF

if [ "${?}" != "0" ] && [ "${WITH_REBOOT}" = "true" ]; then
	echo "Changing the 'nvidia.com/mig.config.state' node label to 'rebooting'"
	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="rebooting"
	if [ "${?}" != "0" ]; then
		echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'"
		echo "Exiting so as not to reboot multiple times unexpectedly"
		exit_failed
	fi
	chroot ${HOST_ROOT_MOUNT} reboot
	exit 0
fi

echo "Applying the selected MIG config to the node"

cat << EOF | nvidia-mig-parted -d apply -f -
${MIG_CONFIG_FILE}
EOF

if [ "${?}" != "0" ]; then
	exit_failed
fi

echo "Restarting validator pod to re-run all validations"
kubectl delete pod \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-operator-validator

exit_success
