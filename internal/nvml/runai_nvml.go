package nvml

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func (d nvmlDevice) GetMaxMigDeviceCount() (int, Return) {
	count, r := nvml.Device(d).GetMaxMigDeviceCount()
	return count, nvmlReturn(r)
}

func (d nvmlDevice) GetMigDeviceHandleByIndex(Index int) (Device, Return) {
	migDevice, r := nvml.Device(d).GetMigDeviceHandleByIndex(Index)
	return nvmlDevice(migDevice), nvmlReturn(r)
}

func (d nvmlDevice) GetUUID() (string, Return) {
	uuid, r := nvml.Device(d).GetUUID()
	return uuid, nvmlReturn(r)
}

func (d nvmlDevice) GetGpuInstanceId() (int, Return) {
	id, r := nvml.Device(d).GetGpuInstanceId()
	return id, nvmlReturn(r)
}

func (d nvmlDevice) GetGpuInstanceById(Id int) (GpuInstance, Return) {
	gi, r := nvml.Device(d).GetGpuInstanceById(Id)
	return nvmlGpuInstance(gi), nvmlReturn(r)
}
