package lg

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 디바이스 발견 및 관리
//
// SPEC-LG-HVACR-003 § M5. lg_hvacr02 의 디바이스 관리 구조를 복제하되, 발견 경로가
// 버스 프레임 관측이 아니라 FC02 전체 스캔이다.
// ---------------------------------------------------------------------------

// defaultHvacr03Label 은 주소에 대한 기본 라벨을 반환한다.
func defaultHvacr03Label(addr string) string {
	return "indoor-" + addr
}

// registerConfigDevices 는 설정에 정의된 디바이스를 등록한다.
// 이들은 스캔에서 미발견되어도 목록에서 제거되지 않는다 (오프라인 표시만 된다).
func (a *Hvacr03Agent) registerConfigDevices() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range a.hvacr03Config.Devices {
		n, err := pmbusParseUnitAddr(entry.Address, a.hvacr03Config.AddressBase)
		if err != nil {
			// 설정 파싱에서 이미 검증되었으므로 여기 도달하면 내부 모순이다.
			a.logger.Warn("lg_hvacr03: 설정 디바이스 주소 해석 실패", "address", entry.Address, "error", err)
			continue
		}
		a.registerOneDeviceLocked(entry.Address, n, entry, "config")
	}
}

// RegisterPinnedDevices 는 런타임 등록 디바이스를 반영한다 (roster 경로).
func (a *Hvacr03Agent) RegisterPinnedDevices(entries []agent.DeviceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range entries {
		n, err := pmbusParseUnitAddr(entry.Address, a.hvacr03Config.AddressBase)
		if err != nil {
			a.logger.Warn("lg_hvacr03: pinned 디바이스 주소 해석 실패", "address", entry.Address, "error", err)
			continue
		}
		source := entry.Source
		if source == "" {
			source = "config"
		}
		a.registerOneDeviceLocked(entry.Address, n, entry, source)
	}
}

// registerOneDeviceLocked 는 디바이스 하나를 등록하거나 기존 항목을 갱신한다.
// 호출 전제: a.mu 쓰기 락 보유.
func (a *Hvacr03Agent) registerOneDeviceLocked(addr string, n uint16, entry agent.DeviceEntry, source string) {
	label := entry.DisplayName
	if label == "" {
		label = entry.Name
	}
	if label == "" {
		label = defaultHvacr03Label(addr)
	}

	devType, pinned := a.resolveConfiguredTypeLocked(addr)

	dev, ok := a.devices[addr]
	if !ok {
		dev = &PmbusDevice{
			Address:       addr,
			UnitN:         n,
			Label:         label,
			Type:          devType,
			Source:        source,
			State:         &PmbusDeviceState{},
			ReportEnabled: true,
			TypePinned:    pinned,
		}
		a.devices[addr] = dev
	} else {
		dev.Label = label
		dev.Source = source
		if pinned {
			dev.Type = devType
			dev.TypePinned = true
		}
	}
	if entry.ReportEnabled != nil {
		dev.ReportEnabled = *entry.ReportEnabled
	}
}

// resolveConfiguredTypeLocked 는 설정에 지정된 기기 종류를 반환한다.
// 지정이 없으면 기본 종류와 pinned=false 를 반환한다.
// 호출 전제: a.mu 보유.
func (a *Hvacr03Agent) resolveConfiguredTypeLocked(addr string) (string, bool) {
	if dt, ok := a.hvacr03Config.DeviceTypes[addr]; ok {
		return dt, true
	}
	return pmbusDeviceTypeIDU, false
}

// ensureDeviceLocked 는 주소에 해당하는 디바이스를 반환하고, 없으면 자동 생성한다.
// auto_discovery 가 꺼져 있고 미등록 주소이면 nil 을 반환한다.
// 호출 전제: a.mu 쓰기 락 보유.
func (a *Hvacr03Agent) ensureDeviceLocked(addr string, n uint16) *PmbusDevice {
	if dev, ok := a.devices[addr]; ok {
		return dev
	}
	if !a.hvacr03Config.AutoDiscovery {
		return nil
	}
	devType, pinned := a.resolveConfiguredTypeLocked(addr)
	dev := &PmbusDevice{
		Address:       addr,
		UnitN:         n,
		Label:         defaultHvacr03Label(addr),
		Type:          devType,
		Source:        "auto",
		State:         &PmbusDeviceState{},
		ReportEnabled: true,
		TypePinned:    pinned,
	}
	a.devices[addr] = dev
	a.logger.Info("lg_hvacr03: 디바이스 자동 발견", "address", addr, "unit_n", n)
	return dev
}

// ListDevices 는 현재 관리 중인 모든 디바이스의 스냅샷을 반환한다.
func (a *Hvacr03Agent) ListDevices() []PmbusDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]PmbusDevice, 0, len(a.devices))
	for _, dev := range a.devices {
		cp := *dev
		if dev.State != nil {
			s := dev.State.snapshot()
			cp.State = &s
		}
		result = append(result, cp)
	}
	return result
}

// projectionOpts 는 현재 설정에 기반한 투영 옵션을 반환한다.
func (a *Hvacr03Agent) projectionOpts() pmbusProjectionOpts {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return pmbusProjectionOpts{fanAutoCode: a.hvacr03Config.FanAutoCode}
}

// projectionOptsLocked 는 a.mu 보유 상태에서 투영 옵션을 반환한다.
// 호출 전제: a.mu 보유 (재진입 방지).
func (a *Hvacr03Agent) projectionOptsLocked() pmbusProjectionOpts {
	return pmbusProjectionOpts{fanAutoCode: a.hvacr03Config.FanAutoCode}
}

// setAllDevicesOffline 은 모든 디바이스를 오프라인으로 표시한다.
// 트랜스포트 연결이 끊어졌을 때 호출한다 — 상태가 과거 값에 멈춰 있지 않게 한다.
func (a *Hvacr03Agent) setAllDevicesOffline() {
	a.mu.Lock()
	var offlined []string
	for addr, dev := range a.devices {
		if dev.Online {
			dev.Online = false
			offlined = append(offlined, addr)
		}
	}
	v2 := a.onDeviceStateChangeV2
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	a.notifyDeviceChanges(v2, agentName, offlined)
	if len(offlined) > 0 {
		a.logger.Info("lg_hvacr03: 연결 끊김으로 전 디바이스 오프라인", "count", len(offlined))
	}
}

// offlineWatchLoop 은 주기적으로 디바이스 타임아웃을 검사한다.
func (a *Hvacr03Agent) offlineWatchLoop() {
	a.mu.RLock()
	timeout := a.hvacr03Config.OfflineTimeout
	a.mu.RUnlock()

	interval := timeout / 2
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.checkDeviceTimeouts()
		}
	}
}

// checkDeviceTimeouts 은 LastSeen 기반으로 오프라인 전이를 수행한다.
func (a *Hvacr03Agent) checkDeviceTimeouts() {
	now := time.Now()

	a.mu.Lock()
	timeout := a.hvacr03Config.OfflineTimeout
	var offlined []string
	for addr, dev := range a.devices {
		if dev.Online && !dev.LastSeen.IsZero() && now.Sub(dev.LastSeen) > timeout {
			dev.Online = false
			offlined = append(offlined, addr)
		}
	}
	v2 := a.onDeviceStateChangeV2
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	a.notifyDeviceChanges(v2, agentName, offlined)
	for _, addr := range offlined {
		a.logger.Info("lg_hvacr03: 통신 타임아웃, 디바이스 오프라인", "address", addr, "timeout", timeout)
	}
}

// notifyDeviceChanges 는 디바이스 변경을 V2 콜백으로 알린다.
// 락 밖에서 호출해야 한다 — 콜백이 에이전트를 다시 호출할 수 있다.
func (a *Hvacr03Agent) notifyDeviceChanges(v2 agent.DeviceStateChangeCallbackV2, agentName string, addrs []string) {
	if v2 == nil {
		return
	}
	for _, addr := range addrs {
		globalID := fmt.Sprintf("%s:%s", agentName, addr)
		deviceUID := agent.ResolveDeviceID(context.Background(), agentName, addr)
		go v2(agentName, deviceUID, globalID)
	}
}

// ---------------------------------------------------------------------------
// 디바이스 관리 명령 (list_devices / remove_device / set_device)
//
// NASA / lg_hvacr02 와 동일한 DTO 형태를 유지하여 프론트엔드·REST 가 변경 없이
// 동작하게 한다. 자동 발견 전용이므로 add_device 는 지원하지 않는다.
// ---------------------------------------------------------------------------

// resolveDeviceMgmt 는 device_id(UUID) 또는 address 로 디바이스를 해석한다.
func (a *Hvacr03Agent) resolveDeviceMgmt(req *hvacr03ProcessRequest) (string, *PmbusDevice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var addr string
	switch {
	case req.DeviceID != "":
		byUUID, ok := a.addrByDeviceUUIDLocked(req.DeviceID)
		if !ok {
			return "", nil, ErrDeviceIDNotFound
		}
		addr = byUUID
	case req.Address != "":
		addr = req.Address
	default:
		return "", nil, fmt.Errorf("lg_hvacr03: address or device_id is required")
	}

	dev, ok := a.devices[addr]
	if !ok {
		return "", nil, ErrDeviceNotFound
	}
	return addr, dev, nil
}

// addrByDeviceUUIDLocked 는 UUID 를 각 디바이스의 emit-경로 UUID 와 대조해 주소를
// 역매칭한다.
//
// 호출 전제: a.mu 보유. 여기서 a.mu 를 재-lock 하지 않으며 a.Name() 도 호출하지
// 않는다 (RWMutex 비재진입 — 재진입 deadlock 회피).
func (a *Hvacr03Agent) addrByDeviceUUIDLocked(uuid string) (string, bool) {
	for addr := range a.devices {
		if agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr) == uuid {
			return addr, true
		}
	}
	return "", false
}

// deviceDTOLocked 는 디바이스의 조회용 DTO 를 생성한다.
// 호출 전제: a.mu 보유.
func (a *Hvacr03Agent) deviceDTOLocked(dev *PmbusDevice) map[string]any {
	d := map[string]any{
		"unit_id":     dev.Address,
		"device_id":   agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.Address),
		"name":        dev.Label,
		"device_type": dev.Type,
		"online":      dev.Online,
	}
	if dev.State != nil {
		d["state"] = dev.State.toProperties(dev.Type, a.projectionOptsLocked())
	}
	if !dev.LastSeen.IsZero() {
		d["last_seen_ms"] = dev.LastSeen.UnixMilli()
	}
	return d
}
