package modbusserver

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Device
// ---------------------------------------------------------------------------

// Device 는 단일 가상 Modbus 디바이스를 나타낸다.
// 각 디바이스는 고유한 UnitID, 이름, RegisterMap, RequestHandler 를 가진다.
type Device struct {
	UnitID       byte
	Name         string
	RegisterMap  *RegisterMap
	ReqHandler   *RequestHandler
	Stats        DeviceStats
	RegisterDefs []any // 디바이스별 레지스터 정의 (Bridge Adapter 매핑용)
}

// DeviceStats 는 디바이스별 통계를 추적한다.
type DeviceStats struct {
	ReadCount  atomic.Int64
	WriteCount atomic.Int64
	ErrorCount atomic.Int64
	LastAccess atomic.Value // time.Time
}

// RecordRead 는 읽기 요청 통계를 기록한다.
func (ds *DeviceStats) RecordRead() {
	ds.ReadCount.Add(1)
	ds.LastAccess.Store(time.Now())
}

// RecordWrite 는 쓰기 요청 통계를 기록한다.
func (ds *DeviceStats) RecordWrite() {
	ds.WriteCount.Add(1)
	ds.LastAccess.Store(time.Now())
}

// RecordError 는 에러 통계를 기록한다.
func (ds *DeviceStats) RecordError() {
	ds.ErrorCount.Add(1)
}

// GetLastAccess 는 마지막 접근 시간을 반환한다.
func (ds *DeviceStats) GetLastAccess() time.Time {
	v := ds.LastAccess.Load()
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

// ---------------------------------------------------------------------------
// DeviceManager
// ---------------------------------------------------------------------------

// DeviceManager 는 여러 가상 디바이스를 관리한다.
// UnitID 기반으로 디바이스를 조회하고, 스레드-세이프한 접근을 제공한다.
type DeviceManager struct {
	mu      sync.RWMutex
	devices map[byte]*Device
	order   []byte // 디바이스 추가 순서 보존 (broadcast 시 첫 번째 디바이스 결정용)
}

// NewEmptyDeviceManager 는 디바이스가 없는 빈 DeviceManager 를 반환한다.
// role=sub 서버가 자체 디바이스 없이 생성될 때 사용되며, Start 시점에
// 주 서버의 디바이스를 AddDevice 로 채운다(라이브 공유 RegisterMap).
func NewEmptyDeviceManager() *DeviceManager {
	return &DeviceManager{
		devices: make(map[byte]*Device),
		order:   make([]byte, 0),
	}
}

// NewDeviceManager 는 설정에서 디바이스 목록을 생성하여 DeviceManager 를 반환한다.
func NewDeviceManager(configs []DeviceConfig, logger *slog.Logger) (*DeviceManager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("modbus-server: at least one device config is required: %w", ErrInvalidDeviceConfig)
	}

	dm := &DeviceManager{
		devices: make(map[byte]*Device, len(configs)),
		order:   make([]byte, 0, len(configs)),
	}

	for _, cfg := range configs {
		if _, exists := dm.devices[cfg.UnitID]; exists {
			return nil, fmt.Errorf("modbus-server: duplicate unit_id %d: %w", cfg.UnitID, ErrDuplicateUnitID)
		}

		rm := NewRegisterMap(cfg.RegisterMap)
		reqHandler := NewRequestHandler(rm, logger)

		device := &Device{
			UnitID:       cfg.UnitID,
			Name:         cfg.Name,
			RegisterMap:  rm,
			ReqHandler:   reqHandler,
			RegisterDefs: cfg.RegisterDefs,
		}

		dm.devices[cfg.UnitID] = device
		dm.order = append(dm.order, cfg.UnitID)
	}

	return dm, nil
}

// GetDevice 는 UnitID 에 해당하는 디바이스를 반환한다.
// 존재하지 않으면 nil 을 반환한다.
func (dm *DeviceManager) GetDevice(unitID byte) *Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.devices[unitID]
}

// GetAllDevices 는 추가 순서대로 모든 디바이스를 반환한다.
func (dm *DeviceManager) GetAllDevices() []*Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*Device, 0, len(dm.order))
	for _, uid := range dm.order {
		if dev, ok := dm.devices[uid]; ok {
			result = append(result, dev)
		}
	}
	return result
}

// FirstDevice 는 첫 번째 디바이스를 반환한다.
// broadcast(UnitID=0) 읽기 요청 시 사용된다.
func (dm *DeviceManager) FirstDevice() *Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if len(dm.order) == 0 {
		return nil
	}
	return dm.devices[dm.order[0]]
}

// AddDevice 는 새 디바이스를 추가한다.
// 이미 존재하는 UnitID 이면 에러를 반환한다.
func (dm *DeviceManager) AddDevice(dev *Device) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.devices[dev.UnitID]; exists {
		return fmt.Errorf("modbus-server: device with unit_id %d already exists: %w", dev.UnitID, ErrDuplicateUnitID)
	}

	dm.devices[dev.UnitID] = dev
	dm.order = append(dm.order, dev.UnitID)
	return nil
}

// RemoveDevice 는 UnitID 에 해당하는 디바이스를 제거한다.
// 존재하지 않으면 에러를 반환한다.
func (dm *DeviceManager) RemoveDevice(unitID byte) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.devices[unitID]; !exists {
		return fmt.Errorf("modbus-server: device with unit_id %d not found: %w", unitID, ErrDeviceNotFound)
	}

	delete(dm.devices, unitID)

	// order 에서 제거
	for i, uid := range dm.order {
		if uid == unitID {
			dm.order = append(dm.order[:i], dm.order[i+1:]...)
			break
		}
	}

	return nil
}

// DeviceCount 는 현재 등록된 디바이스 수를 반환한다.
func (dm *DeviceManager) DeviceCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.devices)
}
