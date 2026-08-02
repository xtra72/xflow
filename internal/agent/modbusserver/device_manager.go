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
	order   []byte  // 디바이스 추가 순서 보존 (broadcast 시 첫 번째 디바이스 결정용)
	shared  *Device // unit_id 0 공유 컨테이너 (와이어 미서빙; devices/order 에서 제외). nil 가능.
}

// SharedContainer 는 unit_id 0 공유 컨테이너 디바이스를 반환한다(없으면 nil).
// 와이어 서빙 집합(GetDevice/GetAllDevices/FirstDevice)에는 포함되지 않는다.
func (dm *DeviceManager) SharedContainer() *Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.shared
}

// setSharedContainer 는 공유 컨테이너를 설정한다(cross-agent 상속 시 사용).
func (dm *DeviceManager) setSharedContainer(dev *Device) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.shared = dev
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
// unit_id 0 은 공유 컨테이너로 먼저 구성되어 dm.shared 에 저장되고(와이어 미서빙),
// 서빙 디바이스(1-247)는 로컬 세그먼트로 자체 맵을 구성하되 공유 세그먼트가 있으면
// deviceView 를 통해 컨테이너 맵으로 주소 변환 서빙한다.
func NewDeviceManager(configs []DeviceConfig, logger *slog.Logger) (*DeviceManager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("modbus-server: at least one device config is required: %w", ErrInvalidDeviceConfig)
	}

	dm := &DeviceManager{
		devices: make(map[byte]*Device, len(configs)),
		order:   make([]byte, 0, len(configs)),
	}

	// 1) 공유 컨테이너(unit_id 0) 를 먼저 구성한다. 컨테이너 세그먼트는 모두 로컬이다.
	var containerRM *RegisterMap
	for _, cfg := range configs {
		if cfg.UnitID != 0 {
			continue
		}
		containerRM = NewRegisterMap(cfg.RegisterMap)
		dm.shared = &Device{
			UnitID:       0,
			Name:         cfg.Name,
			RegisterMap:  containerRM,
			ReqHandler:   NewRequestHandler(containerRM, logger),
			RegisterDefs: cfg.RegisterDefs,
		}
	}

	// 2) 서빙 디바이스(1-247) 를 구성한다.
	for _, cfg := range configs {
		if cfg.UnitID == 0 {
			continue
		}
		if _, exists := dm.devices[cfg.UnitID]; exists {
			return nil, fmt.Errorf("modbus-server: duplicate unit_id %d: %w", cfg.UnitID, ErrDuplicateUnitID)
		}

		// 자체 맵은 로컬 세그먼트만 담는다(공유 세그먼트는 컨테이너가 서빙).
		ownRM := NewRegisterMap(localSegmentsConfig(cfg.RegisterMap))

		var store registerStore = ownRM
		if hasSharedSegment(cfg.RegisterMap) {
			store = newDeviceView(cfg.RegisterMap, ownRM, containerRM)
		}

		device := &Device{
			UnitID:       cfg.UnitID,
			Name:         cfg.Name,
			RegisterMap:  ownRM,
			ReqHandler:   newRequestHandlerWithStore(store, logger),
			RegisterDefs: cfg.RegisterDefs,
		}

		dm.devices[cfg.UnitID] = device
		dm.order = append(dm.order, cfg.UnitID)
	}

	return dm, nil
}

// localSegmentsConfig 는 register_map 설정에서 로컬 세그먼트만 남긴 사본을 반환한다.
// (공유 세그먼트는 디바이스 자체 맵에 스토리지를 할당하지 않고 컨테이너가 서빙한다.)
func localSegmentsConfig(cfg RegisterMapConfig) RegisterMapConfig {
	return RegisterMapConfig{
		Coils:            filterLocalSegments(cfg.Coils),
		DiscreteInputs:   filterLocalSegments(cfg.DiscreteInputs),
		HoldingRegisters: filterLocalSegments(cfg.HoldingRegisters),
		InputRegisters:   filterLocalSegments(cfg.InputRegisters),
	}
}

// filterLocalSegments 는 공유(IsShared) 세그먼트를 제외한 로컬 세그먼트만 반환한다.
func filterLocalSegments(segs []*RegisterAreaConfig) []*RegisterAreaConfig {
	out := make([]*RegisterAreaConfig, 0, len(segs))
	for _, s := range segs {
		if !s.IsShared {
			out = append(out, s)
		}
	}
	return out
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
