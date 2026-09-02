package modbus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// F1 — 런타임 디바이스 등록 add/remove (REQ-MODBUS-008-04, M4)
// ---------------------------------------------------------------------------
//
// 노드→에이전트 Process() 명령 경로로 에이전트 재시작 없이 디바이스를 추가/삭제한다.
// 명령 표면은 기존 Process 문자열 스위치(agent.go)와 일관된 1-동사-1-case 스타일을 따른다
// (신규 add_device/remove_device — §5.5 확정, 신규 병렬 메커니즘 아님).
//
// 스레드 안전성(신규 요구, AC-10):
//
//	이전에는 a.devices/a.caches/a.devStats/a.groupStats 가 생성 시 1회 채워진 뒤 불변이라
//	폴링 goroutine 이 락 없이 읽을 수 있었다. 런타임 add/remove 는 이 컬렉션들을 변경하므로,
//	copy-on-write(새 슬라이스/맵을 만들어 a.mu.Lock() 하에 통째 교체)로 갱신하고 읽기 측은
//	a.mu.RLock() 스냅샷으로 안정적인 참조를 확보한다. set_config 의 register_groups
//	copy-on-write 선례와 동일한 원리다(-race 클린).

// processAddDevice 는 런타임에 디바이스를 추가한다(AC-07).
//
// 동작 규약:
//   - params 를 parseDeviceConfig(init 경로와 동일 검증 규칙)로 파싱·검증한다.
//   - 유효 트랜스포트는 F2 규칙(에이전트 기본 ?? per-device override)으로 선택하고,
//     F3 세션 공유가 활성이면 동일 엔드포인트의 기존 공유 트랜스포트에 참조를 추가한다.
//   - 디바이스를 생성해 connect(connect on add) 후 devices/캐시/통계/폴링 스케줄에 편입한다.
//   - 중복 ID 는 원자적으로 거부한다(부분 적용 없음). 검증 실패 시 아무 것도 적용하지 않는다.
func (a *ModbusAgent) processAddDevice(req *processRequest) ([]byte, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("modbus add_device: params required")
	}

	// 에이전트 기본값(트랜스포트/시리얼/타임아웃)을 스냅샷한다.
	a.mu.RLock()
	agentCfg := a.config
	a.mu.RUnlock()

	// (1) 파싱·검증 — init 경로와 동일한 parseDeviceConfig 규칙을 재사용한다.
	dc, err := parseDeviceConfig(req.Params, 0, agentCfg.Transport)
	if err != nil {
		return nil, fmt.Errorf("modbus add_device: %w", err)
	}
	if dc.ID == "" {
		return nil, fmt.Errorf("modbus add_device: %w", ErrMissingDeviceID)
	}

	// (2) 트랜스포트 선택 + 디바이스 생성은 a.mu.Lock() 하에 수행하여 중복 검사와 슬라이스
	// 교체를 원자적으로 만든다(부분 적용 없음). 네트워크 connect 는 락을 잠깐 놓고 수행해
	// 폴링 goroutine 의 RLock 을 장시간 막지 않는다.
	a.mu.Lock()
	if a.findDeviceLocked(dc.ID) != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("modbus add_device %q: %w", dc.ID, ErrDuplicateDevice)
	}
	dev := a.buildRuntimeDeviceLocked(dc)
	a.mu.Unlock()

	// connect on add — 실패는 치명적이지 않다. Start 와 동일하게 경고만 남기고 디바이스를
	// 오프라인으로 편입하면, 폴링 루프가 다음 틱부터 재연결을 시도한다.
	ctx, cancel := context.WithTimeout(context.Background(), agentCfg.RequestTimeout)
	if cerr := dev.Connect(ctx); cerr != nil {
		a.logger.Warn("modbus: add_device 디바이스 연결 실패(오프라인 편입)",
			"device", dc.ID, "error", cerr)
	}
	cancel()

	// (3) copy-on-write 로 컬렉션을 교체하고 폴링 스케줄에 편입한다(원자적 적용).
	a.mu.Lock()
	defer a.mu.Unlock()

	// 락을 놓은 사이 동일 ID 가 추가됐는지 재확인한다(동시 add 방어). 발생 시 이번 연결을
	// 정리하고 거부한다(부분 적용 없음).
	if a.findDeviceLocked(dc.ID) != nil {
		go func() { _ = dev.Close() }()
		return nil, fmt.Errorf("modbus add_device %q: %w", dc.ID, ErrDuplicateDevice)
	}

	newDevices := make([]*ModbusDevice, 0, len(a.devices)+1)
	newDevices = append(newDevices, a.devices...)
	newDevices = append(newDevices, dev)

	newCaches := cloneCacheMap(a.caches)
	cache := NewRegisterCache()
	if overlay := buildCacheTypeOverlay(dc.RegisterGroups); overlay != nil {
		cache.SetTypeOverlay(overlay)
	}
	newCaches[dc.ID] = cache

	// init-불변이던 통계 맵을 런타임 추가 디바이스/그룹까지 확장한다(확정 방침, AC-10).
	newDevStats := cloneCounterMap(a.devStats)
	newDevStats[dc.ID] = &requestCounters{}
	newGroupStats := cloneCounterMap(a.groupStats)
	for _, rg := range dc.RegisterGroups {
		newGroupStats[groupStatKey(dc.ID, rg.Name)] = &requestCounters{}
	}

	a.devices = newDevices
	a.caches = newCaches
	a.devStats = newDevStats
	a.groupStats = newGroupStats

	// 폴링 중(Running + started + cached)이면 poll_interval 지정 그룹의 전용 스케줄러를 시작한다.
	// 기본 케이던스 그룹은 pollDevices 가 다음 틱에 devices 스냅샷을 순회하며 자동 편입한다(AC-07).
	if a.CurrentState() == lifecycle.StateRunning && a.started && a.config.ReadMode == "cached" {
		a.startDeviceBlockLoops(dev)
	}

	a.logger.Info("modbus: add_device 완료",
		"device", dc.ID,
		"groups", len(dc.RegisterGroups),
		"deviceCount", len(newDevices),
	)

	resp := map[string]any{
		"status":       "device_added",
		"device_id":    dc.ID,
		"device_count": len(newDevices),
	}
	return json.Marshal(resp)
}

// processRemoveDevice 는 런타임에 디바이스를 삭제한다(AC-08).
//
// 동작 규약:
//   - 대상 디바이스를 폴링 대상에서 제외(그룹 스케줄러 정지)하고 컬렉션에서 제거한다.
//   - 연결은 close(close on remove)하되, F3 공유 연결(또는 RTU 단일 버스)을 다른 디바이스가
//     아직 참조 중이면 실제 close 하지 않는다(마지막-참조 close).
//   - 존재하지 않는 ID 삭제는 오류로 거부하고 에이전트는 직전 상태로 계속 동작한다(부분 적용 없음).
func (a *ModbusAgent) processRemoveDevice(req *processRequest) ([]byte, error) {
	id := req.DeviceID
	if id == "" {
		// device_id 최상위 필드가 비어 있으면 params.device_id 도 허용한다(명령 표면 유연성).
		if req.Params != nil {
			if s, ok := req.Params["device_id"].(string); ok {
				id = s
			}
		}
	}
	if id == "" {
		return nil, fmt.Errorf("modbus remove_device: %w", ErrMissingDeviceID)
	}

	a.mu.Lock()

	removed := a.findDeviceLocked(id)
	if removed == nil {
		// 미존재 ID → 오류 거부(직전 상태 유지, AC-08). 부분 적용 없음.
		a.mu.Unlock()
		return nil, fmt.Errorf("modbus remove_device %q: %w", id, ErrDeviceNotFound)
	}

	// 폴링 중이면 이 디바이스의 poll_interval 그룹 스케줄러를 개별 정지한다.
	if a.CurrentState() == lifecycle.StateRunning && a.started && a.config.ReadMode == "cached" {
		a.stopDeviceBlockLoops(removed.config.ID)
	}

	// copy-on-write 로 컬렉션에서 제거한다(원자적 적용 — 이 시점 이후의 폴러 RLock 스냅샷은
	// 제거된 디바이스를 더 이상 포함하지 않는다).
	newDevices := make([]*ModbusDevice, 0, len(a.devices))
	for _, d := range a.devices {
		if d != removed {
			newDevices = append(newDevices, d)
		}
	}
	a.devices = newDevices
	a.caches = cloneCacheMapExcept(a.caches, id)
	a.devStats = cloneCounterMapExcept(a.devStats, id)
	a.groupStats = cloneGroupStatsExcept(a.groupStats, removed.config.ID, removed.config.RegisterGroups)

	// 연결 close — 마지막-참조 판정(공유 refcount 감소 / 원시 잔여-참조 검사)은 a.mu 하에서
	// 수행하되, 블로킹 I/O 인 실제 하부 Close() 는 a.mu 해제 후에 실행한다(add 경로가 Connect 를
	// 락 밖에서 수행하는 것과 대칭). 원시 Close() 는 하부 트랜스포트 자신의 mu 에서 in-flight
	// SendAndReceive 완료(최대 requestTimeout)를 기다릴 수 있어, 이를 a.mu 하에서 호출하면
	// 그 창 동안 폴러의 RLock 이 막힌다. 컬렉션 교체는 이미 위에서 락 하에 원자적으로 끝났으므로
	// close 를 지연해도 제거된 디바이스가 다시 폴링되는 창은 생기지 않는다(AC-08).
	closeTransport := a.planTransportCloseLocked(removed, newDevices)
	deviceCount := len(newDevices)

	a.mu.Unlock()

	// a.mu 해제 후 실제 하부 Close() 수행(마지막-참조일 때만 non-nil).
	if closeTransport != nil {
		closeTransport()
	}

	a.logger.Info("modbus: remove_device 완료",
		"device", id,
		"deviceCount", deviceCount,
	)

	resp := map[string]any{
		"status":       "device_removed",
		"device_id":    id,
		"device_count": deviceCount,
	}
	return json.Marshal(resp)
}

// findDeviceLocked 는 device ID 로 디바이스를 검색한다(호출자가 a.mu 를 보유한다고 가정).
// 발견하지 못하면 nil 을 반환한다. add/remove 의 원자적 중복/존재 검사에 사용한다.
func (a *ModbusAgent) findDeviceLocked(id string) *ModbusDevice {
	for _, d := range a.devices {
		if d.config.ID == id {
			return d
		}
	}
	return nil
}

// buildRuntimeDeviceLocked 는 런타임 추가 디바이스의 트랜스포트를 선택해 디바이스를 생성한다
// (호출자가 a.mu 를 보유한다고 가정). F2/F3 규칙을 build 경로와 정합하게 적용한다:
//   - F3 세션 공유가 활성이면 동일 그룹 키의 기존 공유 트랜스포트에 참조를 추가(addRef)하거나
//     새 공유 트랜스포트를 만든다.
//   - RTU 상속(에이전트 기본 rtu + per-device override 없음)은 물리 시리얼 포트가 하나뿐이므로,
//     기존 상속 RTU 버스를 재사용해 두 번째 포트 오픈을 방지한다(단일 버스 보존).
//   - 그 외(tcp / rtu override, 비공유)는 자체 독립 트랜스포트를 생성한다(현 토폴로지).
func (a *ModbusAgent) buildRuntimeDeviceLocked(dc DeviceConfig) *ModbusDevice {
	cfg := a.config

	// F3 세션 공유 활성 → 그룹 키로 기존 공유 트랜스포트를 찾아 참조 추가 또는 신규 생성.
	if effectiveShareSession(dc, cfg) {
		key := deviceGroupKey(dc, cfg)
		if st := a.findSharedTransportLocked(key); st != nil {
			st.addRef()
			return newModbusDeviceWithTransport(dc, st, a.logger)
		}
		inner := a.buildInnerTransport(dc, cfg)
		st := newSharedTransport(inner)
		st.addRef()
		return newModbusDeviceWithTransport(dc, st, a.logger)
	}

	// RTU 상속 단일 버스: 기존 상속 RTU 디바이스의 트랜스포트를 재사용(두 번째 시리얼 오픈 방지).
	if effectiveTransportKind(dc, cfg) == TransportRTU && dc.Transport == "" {
		if tr := a.findInheritedRTUBusLocked(); tr != nil {
			return newModbusDeviceWithTransport(dc, tr, a.logger)
		}
	}

	// 그 외: 독립 트랜스포트(tcp 독립 / rtu override 독립).
	inner := a.buildInnerTransport(dc, cfg)
	return newModbusDeviceWithTransport(dc, inner, a.logger)
}

// findSharedTransportLocked 는 그룹 키가 일치하는 기존 *sharedTransport 를 반환한다
// (호출자가 a.mu 를 보유한다고 가정). 없으면 nil.
func (a *ModbusAgent) findSharedTransportLocked(key string) *sharedTransport {
	cfg := a.config
	for _, d := range a.devices {
		if st, ok := d.transport.(*sharedTransport); ok && deviceGroupKey(d.config, cfg) == key {
			return st
		}
	}
	return nil
}

// findInheritedRTUBusLocked 는 에이전트-기본 rtu 를 상속(per-device override 없음)하는 기존
// 디바이스의 트랜스포트(단일 버스)를 반환한다(호출자가 a.mu 를 보유한다고 가정). 없으면 nil.
func (a *ModbusAgent) findInheritedRTUBusLocked() ModbusTransport {
	cfg := a.config
	for _, d := range a.devices {
		if effectiveTransportKind(d.config, cfg) == TransportRTU && d.config.Transport == "" {
			return d.transport
		}
	}
	return nil
}

// planTransportCloseLocked 는 제거된 디바이스의 트랜스포트를 마지막-참조 규칙에 따라 실제로
// 닫아야 하는지 판정하고, 닫아야 한다면 그 블로킹 Close() 를 지연 실행용 클로저로 반환한다
// (호출자가 a.mu 를 보유한다고 가정, AC-08). 닫을 필요가 없으면 nil 을 반환한다.
//
// 상태 판정만 a.mu 하에서 수행하고, 반환된 클로저(실제 Close I/O)는 호출자가 a.mu 해제 후
// 실행한다. 원시 Close() 는 하부 트랜스포트 자신의 mu 에서 in-flight SendAndReceive 완료를
// 기다릴 수 있으므로, 이를 a.mu 밖으로 미뤄 폴러의 RLock 지연을 방지한다(LOW #1 수정).
//   - *sharedTransport: releaseRef() 로 a.mu 하에서 참조를 감소시키고, 마지막 참조일 때만
//     반환된 하부 트랜스포트의 Close() 를 지연 실행한다(참조가 남아 있으면 하부는 닫지 않음).
//   - 그 외(원시 트랜스포트): 기본 RTU 단일 버스처럼 여러 디바이스가 동일 인스턴스를 포인터로
//     공유할 수 있으므로, 남은 디바이스가 같은 인스턴스를 참조하지 않을 때만 close 를 계획한다.
func (a *ModbusAgent) planTransportCloseLocked(removed *ModbusDevice, remaining []*ModbusDevice) func() {
	tr := removed.transport
	if st, ok := tr.(*sharedTransport); ok {
		// 공유: refcount 감소는 락 하에서 즉시 끝내고, 마지막-참조일 때만 하부 Close 를 지연한다.
		if inner := st.releaseRef(); inner != nil {
			return func() { _ = inner.Close() }
		}
		return nil
	}
	for _, d := range remaining {
		if d.transport == tr {
			return nil // 남은 디바이스가 아직 참조 → close 하지 않음(다른 디바이스 트랜잭션 보존).
		}
	}
	return func() { _ = tr.Close() }
}

// ---------------------------------------------------------------------------
// copy-on-write 헬퍼 — 런타임 add/remove 가 컬렉션을 통째 교체할 때 사용한다.
// ---------------------------------------------------------------------------

// cloneCacheMap 은 캐시 맵을 얕은 복사한다(추가용, 여유 용량 +1).
func cloneCacheMap(m map[string]*RegisterCache) map[string]*RegisterCache {
	n := make(map[string]*RegisterCache, len(m)+1)
	for k, v := range m {
		n[k] = v
	}
	return n
}

// cloneCacheMapExcept 은 주어진 ID 를 제외하고 캐시 맵을 얕은 복사한다(삭제용).
func cloneCacheMapExcept(m map[string]*RegisterCache, exceptID string) map[string]*RegisterCache {
	n := make(map[string]*RegisterCache, len(m))
	for k, v := range m {
		if k != exceptID {
			n[k] = v
		}
	}
	return n
}

// cloneCounterMap 은 통계 카운터 맵을 얕은 복사한다(추가용, 여유 용량 +1).
func cloneCounterMap(m map[string]*requestCounters) map[string]*requestCounters {
	n := make(map[string]*requestCounters, len(m)+1)
	for k, v := range m {
		n[k] = v
	}
	return n
}

// cloneCounterMapExcept 은 주어진 ID 키를 제외하고 통계 맵을 얕은 복사한다(디바이스 통계 삭제용).
func cloneCounterMapExcept(m map[string]*requestCounters, exceptID string) map[string]*requestCounters {
	n := make(map[string]*requestCounters, len(m))
	for k, v := range m {
		if k != exceptID {
			n[k] = v
		}
	}
	return n
}

// cloneGroupStatsExcept 은 제거되는 디바이스의 그룹 통계 키를 제외하고 그룹 통계 맵을 복사한다.
func cloneGroupStatsExcept(m map[string]*requestCounters, deviceID string, groups []RegisterGroupConfig) map[string]*requestCounters {
	drop := make(map[string]struct{}, len(groups))
	for _, rg := range groups {
		drop[groupStatKey(deviceID, rg.Name)] = struct{}{}
	}
	n := make(map[string]*requestCounters, len(m))
	for k, v := range m {
		if _, isDropped := drop[k]; !isDropped {
			n[k] = v
		}
	}
	return n
}
