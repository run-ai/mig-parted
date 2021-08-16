package config

import (
	"fmt"

	"github.com/NVIDIA/mig-parted/internal/nvml"
)

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
