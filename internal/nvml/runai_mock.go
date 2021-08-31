package nvml

import "fmt"

func (d *MockA100Device) GetMaxMigDeviceCount() (int, Return) {
	return d.MaxMigDevices, MockReturn(SUCCESS)
}

func (d *MockA100Device) GetMigDeviceHandleByIndex(Index int) (Device, Return) {
	if Index < d.MaxMigDevices {
		migDevice := NewMockA100Device()
		migDevice.(*MockA100Device).Uuid = fmt.Sprintf("MIG-abcd-%d", Index)
		return migDevice, MockReturn(SUCCESS)
	}
	return nil, MockReturn(ERROR_INVALID_ARGUMENT)
}

func (d *MockA100Device) GetUUID() (string, Return) {
	return d.Uuid, MockReturn(SUCCESS)
}

func (d *MockA100Device) GetGpuInstanceId() (int, Return) {
	return d.InstanceId, MockReturn(SUCCESS)
}

func (d *MockA100Device) GetGpuInstanceById(Id int) (GpuInstance, Return) {
	if len(d.GpuInstances) == 0 {
		return nil, MockReturn(ERROR_NOT_FOUND)
	}
	var gi *MockA100GpuInstance
	for gi = range d.GpuInstances {
		break
	}
	return gi, MockReturn(SUCCESS)
}
