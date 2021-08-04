/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"
	"strings"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/types"
	log "github.com/sirupsen/logrus"
)

type Manager interface {
	GetMigConfig(gpu int) (types.MigConfig, error)
	SetMigConfig(gpu int, config types.MigConfig) error
	ClearAndGetInstancesToCreate(gpu int, desiredConfig []types.MigProfile) ([]types.MigProfile, error)
	GetMigPlacements() (map[int]map[int]string, error)
}

type nvmlMigConfigManager struct {
	nvml nvml.Interface
}

var _ Manager = (*nvmlMigConfigManager)(nil)

func tryNvmlShutdown(nvmlLib nvml.Interface) {
	ret := nvmlLib.Shutdown()
	if ret.Value() != nvml.SUCCESS {
		log.Warnf("Error shutting down NVML: %v", ret)
	}
}

func NewNvmlMigConfigManager() Manager {
	return &nvmlMigConfigManager{nvml.New()}
}

func (m *nvmlMigConfigManager) GetMigConfig(gpu int) (types.MigConfig, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device handle: %v", ret)
	}

	mode, _, ret := device.GetMigMode()
	if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
		return nil, fmt.Errorf("MIG not supported")
	}
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting MIG mode: %v", ret)
	}
	if mode != nvml.DEVICE_MIG_ENABLE {
		return nil, fmt.Errorf("MIG mode disabled")
	}

	migConfig := types.MigConfig{}
	for i := 0; i < nvml.GPU_INSTANCE_PROFILE_COUNT; i++ {
		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(i)
		if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
			continue
		}
		if ret.Value() != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting GPU instance profile info for '%v': %v", i, ret)
		}

		gis, ret := device.GetGpuInstances(&giProfileInfo)
		if ret.Value() != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting GPU instances for profile '%v': %v", i, ret)
		}

		for _, gi := range gis {
			for j := 0; j < nvml.COMPUTE_INSTANCE_PROFILE_COUNT; j++ {
				for k := 0; k < nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_COUNT; k++ {
					ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(j, k)
					if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
						continue
					}
					if ret.Value() != nvml.SUCCESS {
						return nil, fmt.Errorf("error getting Compute instance profile info for '(%v, %v)': %v", j, k, ret)
					}

					cis, ret := gi.GetComputeInstances(&ciProfileInfo)
					if ret.Value() != nvml.SUCCESS {
						return nil, fmt.Errorf("error getting Compute instances for profile '(%v, %v)': %v", j, k, ret)
					}

					for _, ci := range cis {
						if ret.Value() != nvml.SUCCESS {
							return nil, fmt.Errorf("error getting Compute instance info for '%v': %v", ci, ret)
						}

						mdt := types.NewMigProfile(ciProfileInfo.SliceCount, giProfileInfo.SliceCount, giProfileInfo.MemorySizeMB)
						migConfig[mdt]++
					}
				}
			}
		}
	}

	return migConfig, nil
}

func (m *nvmlMigConfigManager) SetMigConfig(gpu int, config types.MigConfig) error {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error getting device handle: %v", ret)
	}

	mode, _, ret := device.GetMigMode()
	if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
		return fmt.Errorf("MIG not supported")
	}
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error getting MIG mode: %v", ret)
	}
	if mode != nvml.DEVICE_MIG_ENABLE {
		return fmt.Errorf("MIG mode disabled")
	}

	err := iteratePermutationsUntilSuccess(config, func(mps []types.MigProfile) error {
		clearAttempts := 0
		maxClearAttempts := 1
		performedClearOperationSuccessfully := false

		for {
			existingConfig, err := m.GetMigConfig(gpu)
			if err != nil {
				return fmt.Errorf("error getting existing MigConfig: %v", err)
			}

			if performedClearOperationSuccessfully || len(existingConfig.Flatten()) == 0 {
				break
			}

			if clearAttempts == maxClearAttempts {
				return fmt.Errorf("exceeded maximum attempts to clear MigConfig")
			}

			mps, err = m.ClearAndGetInstancesToCreate(gpu, mps)
			if err != nil {
				return fmt.Errorf("error clearing MigConfig: %v", err)
			} else {
				performedClearOperationSuccessfully = true
			}

			clearAttempts++
		}

		for _, mdt := range mps {
			giProfileID, ciProfileID, ciEngProfileID, err := mdt.GetProfileIDs()
			if err != nil {
				return fmt.Errorf("error getting profile ids for '%v': %v", mdt, err)
			}

			giProfileInfo, ret := device.GetGpuInstanceProfileInfo(giProfileID)
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error getting GPU instance profile info for '%v': %v", mdt, ret)
			}

			gi, ret := device.CreateGpuInstance(&giProfileInfo)
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error creating GPU instance for '%v': %v", mdt, ret)
			}

			ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(ciProfileID, ciEngProfileID)
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error getting Compute instance profile info for '%v': %v", mdt, ret)
			}

			ci, ret := gi.CreateComputeInstance(&ciProfileInfo)
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error creating Compute instance for '%v': %v", mdt, ret)
			}

			valid := types.NewMigProfile(ciProfileInfo.SliceCount, giProfileInfo.SliceCount, giProfileInfo.MemorySizeMB)
			if mdt != valid {
				return fmt.Errorf("unsupported MIG Device specified %v, expected %v instead", mdt, valid)
			}

			info, _ := gi.GetInfo()
			uuid, _ := info.Device.GetUUID()
			cinfo, _ := ci.GetInfo()
			fmt.Printf("EREZ DEBUG:: gi info: id: %d, ProfileId: %d, Placement: %v, uuid: %s\n", info.Id, info.ProfileId, info.Placement, uuid)
			fmt.Printf("EREZ DEBUG:: ci info: id: %d, ProfileId: %d\n", cinfo.Id, cinfo.ProfileId)
		}

		return nil
	})
	if err != nil {
		_, e := m.ClearAndGetInstancesToCreate(gpu, []types.MigProfile{})
		if e != nil {
			log.Errorf("Error clearing MIG config on GPU %d, erroneous devices may persist", gpu)
		}
		return fmt.Errorf("error attempting multiple config orderings: %v", err)
	}

	maxMigDeviceCount, r := device.GetMaxMigDeviceCount()
	if r.Value() != nvml.SUCCESS {
		fmt.Println("EREZ DEBUG:: failed to get max mig devices")
		return nil
	}
	for ind := 0; ind < maxMigDeviceCount; ind++ {
		migDevice, r := device.GetMigDeviceHandleByIndex(ind)
		if r.Value() != nvml.SUCCESS {
			fmt.Printf("EREZ DEBUG:: failed to get mig device with index: %d. String: %s, value: %v\n", ind, r.String(), r.Value())
			continue
		}
		uuid, r := migDevice.GetUUID()
		if r.Value() != nvml.SUCCESS {
			fmt.Printf("EREZ DEBUG:: Failed to get UUID for mig device with index: %d. String: %s, value: %v\n", ind, r.String(), r.Value())
			continue
		}
		fmt.Printf("EREZ DEBUG:: MIG ID: %d, UUID: %s\n", ind, uuid)
	}

	return nil
}

func (m *nvmlMigConfigManager) ClearAndGetInstancesToCreate(gpu int, desiredConfig []types.MigProfile) ([]types.MigProfile, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return desiredConfig, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return desiredConfig, fmt.Errorf("error getting device handle: %v", ret)
	}

	mode, _, ret := device.GetMigMode()
	if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
		return desiredConfig, fmt.Errorf("MIG not supported")
	}
	if ret.Value() != nvml.SUCCESS {
		return desiredConfig, fmt.Errorf("error getting MIG mode: %v", ret)
	}
	if mode != nvml.DEVICE_MIG_ENABLE {
		return desiredConfig, fmt.Errorf("MIG mode disabled")
	}

	instancesToNotCreate := map[int]bool{}
	for i := 0; i < nvml.GPU_INSTANCE_PROFILE_COUNT; i++ {
		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(i)
		if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
			continue
		}
		if ret.Value() != nvml.SUCCESS {
			return desiredConfig, fmt.Errorf("error getting GPU instance profile info for '%v': %v", i, ret)
		}

		gis, ret := device.GetGpuInstances(&giProfileInfo)
		if ret.Value() != nvml.SUCCESS {
			return desiredConfig, fmt.Errorf("error getting GPU instances for profile '%v': %v", i, ret)
		}

		for _, gi := range gis {
			destroyGi := true
			for j := 0; j < nvml.COMPUTE_INSTANCE_PROFILE_COUNT; j++ {
				for k := 0; k < nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_COUNT; k++ {
					ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(j, k)
					if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
						continue
					}
					if ret.Value() != nvml.SUCCESS {
						return desiredConfig, fmt.Errorf("error getting Compute instance profile info for '(%v, %v)': %v", j, k, ret)
					}

					cis, ret := gi.GetComputeInstances(&ciProfileInfo)
					if ret.Value() != nvml.SUCCESS {
						return desiredConfig, fmt.Errorf("error getting Compute instances for profile '(%v, %v)': %v", j, k, ret)
					}

					for _, ci := range cis {
						ret := ci.Destroy()
						if ret.Value() != nvml.SUCCESS {
							if ret.Value() == nvml.ERROR_IN_USE && len(desiredConfig) > 0 && destroyGi {
								gpuInfo, ret := gi.GetInfo()
								if ret.Value() != nvml.SUCCESS {
									return desiredConfig, fmt.Errorf("error destroying Compute instance for profile '(%v, %v)': %v", j, k, ret)
								}
								index := getMigProfileInConfigByPosition(desiredConfig, device, gpuInfo.ProfileId, instancesToNotCreate)
								if index != -1 {
									instancesToNotCreate[index] = true
									destroyGi = false
								}
							} else {
								return desiredConfig, fmt.Errorf("error destroying Compute instance for profile '(%v, %v)': %v", j, k, ret)
							}
						}
					}
				}
			}

			if destroyGi {
				ret := gi.Destroy()
				if ret.Value() != nvml.SUCCESS {
					return desiredConfig, fmt.Errorf("error destroying GPU instance for profile '%v': %v", i, ret)
				}
			}
		}
	}

	newConfig := []types.MigProfile{}
	for i, item := range desiredConfig {
		if !instancesToNotCreate[i] {
			newConfig = append(newConfig, item)
		}
	}
	return newConfig, nil
}

func getMigProfileInConfigByPosition(config []types.MigProfile, device nvml.Device, profileId uint32, configNotToReCreate map[int]bool) int {
	for index, migProfile := range config {
		giProfileID, _, _, err := migProfile.GetProfileIDs()
		if err != nil {
			return -1
		}

		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(giProfileID)
		if ret.Value() != nvml.SUCCESS {
			return -1
		}

		if giProfileInfo.Id == profileId && !configNotToReCreate[index] {
			return index
		}
	}
	return -1
}

func iteratePermutationsUntilSuccess(config types.MigConfig, f func([]types.MigProfile) error) error {
	shouldSwap := func(mps []types.MigProfile, start, curr int) bool {
		for i := start; i < curr; i++ {
			if mps[i] == mps[curr] {
				return false
			}
		}
		return true
	}

	var iterate func(mps []types.MigProfile, f func([]types.MigProfile) error, index int) error
	iterate = func(mps []types.MigProfile, f func([]types.MigProfile) error, i int) error {
		if i >= len(mps) {
			err := f(mps)
			if err != nil {
				e := err.Error()
				log.Error(strings.ToUpper(e[0:1]) + e[1:])
			}
			return err
		}

		for j := i; j < len(mps); j++ {
			if shouldSwap(mps, i, j) {
				mps[i], mps[j] = mps[j], mps[i]

				err := iterate(mps, f, i+1)
				if err == nil {
					return nil
				}

				mps[i], mps[j] = mps[j], mps[i]
			}
		}

		return fmt.Errorf("all orderings failed")
	}

	return iterate(config.Flatten(), f, 0)
}

func (m *nvmlMigConfigManager) GetMigPlacements() (map[int]map[int]string, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	deviceCount, ret := m.nvml.DeviceGetCount()
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("Failed to read device count: %s", ret.String())
	}
	migPlacements := make(map[int]map[int]string)

	for gpuIndex := 0; gpuIndex < deviceCount; gpuIndex++ {
		gpuDevice, ret := m.nvml.DeviceGetHandleByIndex(gpuIndex)
		if ret.Value() != nvml.SUCCESS {
			return nil, fmt.Errorf("Failed to get device index %d: %s", gpuIndex, ret.String())
		}
		enabled, _, ret := gpuDevice.GetMigMode()
		if ret.Value() != nvml.SUCCESS || enabled != nvml.DEVICE_MIG_ENABLE {
			continue
		}
		maxMigDevices, ret := gpuDevice.GetMaxMigDeviceCount()
		if ret.Value() != nvml.SUCCESS {
			return nil, fmt.Errorf("Failed to read max mig devices: %s", ret.String())
		}

		migPlacements[gpuIndex] = make(map[int]string)

		for migIndex := 0; migIndex < maxMigDevices; migIndex++ {
			migDevice, ret := gpuDevice.GetMigDeviceHandleByIndex(migIndex)
			if ret.Value() != nvml.SUCCESS {
				break
			}
			uuid, ret := migDevice.GetUUID()
			if ret.Value() != nvml.SUCCESS {
				return nil, fmt.Errorf("Failed to read mig device UUID: gpu: %d, mig index: %d: %s", gpuIndex, migIndex, ret.String())
			}
			gpuInstanceId, ret := migDevice.GetGpuInstanceId()
			if ret.Value() != nvml.SUCCESS {
				return nil, fmt.Errorf("Failed to read mig device gpu instance id: gpu: %d, mig index: %d: %s", gpuIndex, migIndex, ret.String())
			}
			gpuInstance, ret := gpuDevice.GetGpuInstanceById(gpuInstanceId)
			if ret.Value() != nvml.SUCCESS {
				return nil, fmt.Errorf("Failed to get gpu instance with id %d for mig device %d on gpu %d: %s", gpuInstanceId, migIndex, gpuIndex, ret.String())
			}
			info, ret := gpuInstance.GetInfo()
			if ret.Value() != nvml.SUCCESS {
				return nil, fmt.Errorf("Failed to get gpu instance info: gpu instance id: %d, mig device %d on gpu index: %d: %s", gpuInstanceId, migIndex, gpuIndex, ret.String())
			}
			migPlacements[gpuIndex][int(info.Placement.Start)] = uuid
		}
	}
	return migPlacements, nil
}
