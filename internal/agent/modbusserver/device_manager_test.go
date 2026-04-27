package modbusserver

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDeviceConfigs 는 테스트용 디바이스 설정을 생성한다.
func testDeviceConfigs(unitIDs ...byte) []DeviceConfig {
	configs := make([]DeviceConfig, 0, len(unitIDs))
	for _, uid := range unitIDs {
		configs = append(configs, DeviceConfig{
			UnitID: uid,
			Name:   "",
			RegisterMap: RegisterMapConfig{
				HoldingRegisters: []*RegisterAreaConfig{{
					StartAddress: 0,
					Count:        10,
				}},
			},
		})
	}
	return configs
}

func TestNewDeviceManager(t *testing.T) {
	configs := testDeviceConfigs(1, 2, 3)
	configs[0].Name = "device-1"
	configs[1].Name = "device-2"
	configs[2].Name = "device-3"

	dm, err := NewDeviceManager(configs, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, dm.DeviceCount())

	// 각 디바이스 조회
	dev1 := dm.GetDevice(1)
	require.NotNil(t, dev1)
	assert.Equal(t, byte(1), dev1.UnitID)
	assert.Equal(t, "device-1", dev1.Name)
	assert.NotNil(t, dev1.RegisterMap)
	assert.NotNil(t, dev1.ReqHandler)

	dev2 := dm.GetDevice(2)
	require.NotNil(t, dev2)
	assert.Equal(t, byte(2), dev2.UnitID)

	dev3 := dm.GetDevice(3)
	require.NotNil(t, dev3)
	assert.Equal(t, byte(3), dev3.UnitID)
}

func TestNewDeviceManager_EmptyConfigs(t *testing.T) {
	_, err := NewDeviceManager([]DeviceConfig{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDeviceConfig)
}

func TestNewDeviceManager_DuplicateUnitID(t *testing.T) {
	configs := testDeviceConfigs(1, 1)
	_, err := NewDeviceManager(configs, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateUnitID)
}

func TestDeviceManager_GetDevice_NotFound(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1), nil)
	require.NoError(t, err)

	dev := dm.GetDevice(99)
	assert.Nil(t, dev)
}

func TestDeviceManager_GetAllDevices(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(3, 1, 2), nil)
	require.NoError(t, err)

	devices := dm.GetAllDevices()
	require.Len(t, devices, 3)

	// 추가 순서가 보존됨
	assert.Equal(t, byte(3), devices[0].UnitID)
	assert.Equal(t, byte(1), devices[1].UnitID)
	assert.Equal(t, byte(2), devices[2].UnitID)
}

func TestDeviceManager_FirstDevice(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(5, 1, 10), nil)
	require.NoError(t, err)

	first := dm.FirstDevice()
	require.NotNil(t, first)
	assert.Equal(t, byte(5), first.UnitID)
}

func TestDeviceManager_AddDevice(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1), nil)
	require.NoError(t, err)

	rm := NewRegisterMap(RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	})
	newDev := &Device{
		UnitID:      2,
		Name:        "added-device",
		RegisterMap: rm,
		ReqHandler:  NewRequestHandler(rm, nil),
	}

	err = dm.AddDevice(newDev)
	require.NoError(t, err)
	assert.Equal(t, 2, dm.DeviceCount())

	dev := dm.GetDevice(2)
	require.NotNil(t, dev)
	assert.Equal(t, "added-device", dev.Name)
}

func TestDeviceManager_AddDevice_Duplicate(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1), nil)
	require.NoError(t, err)

	rm := NewRegisterMap(RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{
			StartAddress: 0,
			Count:        10,
		}},
	})
	newDev := &Device{
		UnitID:      1,
		RegisterMap: rm,
		ReqHandler:  NewRequestHandler(rm, nil),
	}

	err = dm.AddDevice(newDev)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateUnitID)
}

func TestDeviceManager_RemoveDevice(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1, 2, 3), nil)
	require.NoError(t, err)

	err = dm.RemoveDevice(2)
	require.NoError(t, err)
	assert.Equal(t, 2, dm.DeviceCount())
	assert.Nil(t, dm.GetDevice(2))

	// 순서 보존 확인
	devices := dm.GetAllDevices()
	require.Len(t, devices, 2)
	assert.Equal(t, byte(1), devices[0].UnitID)
	assert.Equal(t, byte(3), devices[1].UnitID)
}

func TestDeviceManager_RemoveDevice_NotFound(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1), nil)
	require.NoError(t, err)

	err = dm.RemoveDevice(99)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestDeviceStats(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1), nil)
	require.NoError(t, err)

	dev := dm.GetDevice(1)
	require.NotNil(t, dev)

	// 초기 상태
	assert.Equal(t, int64(0), dev.Stats.ReadCount.Load())
	assert.Equal(t, int64(0), dev.Stats.WriteCount.Load())
	assert.Equal(t, int64(0), dev.Stats.ErrorCount.Load())
	assert.True(t, dev.Stats.GetLastAccess().IsZero())

	// 통계 기록
	dev.Stats.RecordRead()
	dev.Stats.RecordRead()
	dev.Stats.RecordWrite()
	dev.Stats.RecordError()

	assert.Equal(t, int64(2), dev.Stats.ReadCount.Load())
	assert.Equal(t, int64(1), dev.Stats.WriteCount.Load())
	assert.Equal(t, int64(1), dev.Stats.ErrorCount.Load())
	assert.False(t, dev.Stats.GetLastAccess().IsZero())
}

func TestDeviceManager_ConcurrentAccess(t *testing.T) {
	dm, err := NewDeviceManager(testDeviceConfigs(1, 2, 3), nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	const goroutines = 100

	// 동시에 여러 고루틴에서 읽기 접근
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			uid := byte((id % 3) + 1)
			dev := dm.GetDevice(uid)
			if dev != nil {
				dev.Stats.RecordRead()
			}
			dm.GetAllDevices()
			dm.DeviceCount()
			dm.FirstDevice()
		}(i)
	}

	wg.Wait()

	// 모든 디바이스의 읽기 횟수 합산이 goroutines 와 같아야 한다
	total := int64(0)
	for _, dev := range dm.GetAllDevices() {
		total += dev.Stats.ReadCount.Load()
	}
	assert.Equal(t, int64(goroutines), total)
}
