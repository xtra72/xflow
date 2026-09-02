package modbus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// F2 — 백엔드 update_device (연결 유지 in-place 수정, REQ-MODBUS-009-02, M2)
// ---------------------------------------------------------------------------
//
// 노드→에이전트 Process() 명령 경로로 기존 디바이스의 설정(register_groups/unit_id/
// poll_interval/request_timeout/reconnect_interval)을 연결을 유지한 채 in-place 로 변경한다.
// set_config(set_config.go)의 재구성 로직을 공통 헬퍼(parseDeviceReconfig/applyDeviceReconfigLocked)
// 로 추출해 단일 소스화하여 중복·드리프트를 방지한다(§5.2, 확정 방침).
//
// set_config 와의 차이:
//   - update_device 는 항상 device_id 를 요구하는 "디바이스 단위 수정" 표면이다.
//   - set_config 는 device_id 없이 에이전트 기본 케이던스만 바꾸는 경우도 허용한다.
//
// 연결 유지(HARD): update_device 는 대상 디바이스의 트랜스포트/연결을 절대 재생성하지 않는다
// (제거+재추가 아님). register_groups 변경 시 스케줄러/그룹 통계만 갱신하며, 이는 SPEC-008 이
// 확립한 copy-on-write 패턴(a.mu.Lock 하 통째 교체)을 준수하여 폴링 goroutine 과 경합하지 않는다.

// deviceReconfig 는 런타임 가변 재구성 필드의 파싱·검증 결과를 담는다(set_config/update_device 공용).
type deviceReconfig struct {
	newGroups []RegisterGroupConfig
	hasGroups bool

	newUnitID byte
	hasUnitID bool

	newPoll   time.Duration
	hasPoll   bool
	newReqTO  time.Duration
	hasReqTO  bool
	newReconn time.Duration
	hasReconn bool
}

// hasDeviceScoped 는 디바이스 스코프 필드(register_groups/unit_id) 변경이 요청되었는지 반환한다.
func (rc deviceReconfig) hasDeviceScoped() bool {
	return rc.hasGroups || rc.hasUnitID
}

// hasAny 는 하나 이상의 런타임 가변 필드가 요청되었는지 반환한다.
func (rc deviceReconfig) hasAny() bool {
	return rc.hasGroups || rc.hasUnitID || rc.hasPoll || rc.hasReqTO || rc.hasReconn
}

// parseDeviceReconfig 는 params 에서 런타임 가변 재구성 필드를 파싱·검증한다(set_config/update_device 공용 단일 소스).
// parseModbusConfig 와 동일한 규칙(parseSetConfigRegisterGroups/parseUnitIDParam/parseDurationParam)을
// 재사용하여 적용 전 전량 검증한다(부분 적용 금지 — 하나라도 무효면 아무 것도 적용하지 않는다).
func parseDeviceReconfig(params map[string]any) (deviceReconfig, error) {
	var rc deviceReconfig

	if v, ok := params["register_groups"]; ok {
		groups, err := parseSetConfigRegisterGroups(v)
		if err != nil {
			return deviceReconfig{}, err
		}
		rc.newGroups = groups
		rc.hasGroups = true
	}

	// unit_id (디바이스 스코프, 런타임 가변). 적용 전 검증(부분 적용 금지).
	if v, ok := params["unit_id"]; ok {
		uid, err := parseUnitIDParam(v)
		if err != nil {
			return deviceReconfig{}, err
		}
		rc.newUnitID = uid
		rc.hasUnitID = true
	}

	if v, ok := params["poll_interval"]; ok {
		d, err := parseDurationParam("poll_interval", v)
		if err != nil {
			return deviceReconfig{}, err
		}
		// SetPollInterval 과 동일한 하한(100ms)을 재사용한다.
		if d < 100*time.Millisecond {
			return deviceReconfig{}, fmt.Errorf("modbus set_config: poll_interval must be >= 100ms, got %v", d)
		}
		rc.newPoll = d
		rc.hasPoll = true
	}
	if v, ok := params["request_timeout"]; ok {
		d, err := parseDurationParam("request_timeout", v)
		if err != nil {
			return deviceReconfig{}, err
		}
		if d <= 0 {
			return deviceReconfig{}, fmt.Errorf("modbus set_config: request_timeout must be > 0, got %v", d)
		}
		rc.newReqTO = d
		rc.hasReqTO = true
	}
	if v, ok := params["reconnect_interval"]; ok {
		d, err := parseDurationParam("reconnect_interval", v)
		if err != nil {
			return deviceReconfig{}, err
		}
		if d <= 0 {
			return deviceReconfig{}, fmt.Errorf("modbus set_config: reconnect_interval must be > 0, got %v", d)
		}
		rc.newReconn = d
		rc.hasReconn = true
	}

	return rc, nil
}

// applyDeviceReconfigLocked 는 파싱된 재구성 필드를 a.mu.Lock() 하에서 원자적으로 적용한다
// (set_config/update_device 공용 단일 적용 경로). 트랜스포트/연결은 재생성하지 않는다(연결 유지).
// 디바이스 스코프 필드(register_groups/unit_id)가 없으면 dev 는 nil 일 수 있다(set_config 의
// 에이전트 전용 케이던스 변경 경로). 호출자는 a.mu.Lock() 을 보유해야 한다.
func (a *ModbusAgent) applyDeviceReconfigLocked(dev *ModbusDevice, rc deviceReconfig) {
	// 폴링 중(Running + started + cached)일 때만 그룹 스케줄러를 재구성한다.
	// Stop 은 먼저 StateStopping 으로 전이하므로, 정지 중에는 스케줄러를 건드리지 않는다(WaitGroup 경합 방지).
	polling := a.CurrentState() == lifecycle.StateRunning && a.started && a.config.ReadMode == "cached"

	if rc.hasGroups {
		// 이전 그룹 기준으로 실행 중인 개별 그룹 스케줄러를 정지한다.
		if polling {
			a.stopDeviceBlockLoops(dev.config.ID)
		}
		// copy-on-write: 새 그룹 슬라이스를 통째로 교체한다(읽기 측은 RLock 스냅샷).
		dev.config.RegisterGroups = rc.newGroups
		// data_type/byte_order 변경 반영을 위해 TypeOverlay 재구축(그룹에서 타입이 사라지면 해제).
		a.rebuildDeviceTypeOverlay(dev)
		// 새 그룹 중 poll_interval 지정 그룹의 스케줄러를 시작한다(다음 폴 시점부터 반영).
		if polling {
			a.startDeviceBlockLoops(dev)
		}
		a.logger.Info("modbus: 레지스터 그룹 재구성",
			"device", dev.config.ID,
			"groups", len(rc.newGroups),
			"polling", polling,
		)
	}

	if rc.hasUnitID {
		// unit_id 는 device.go 의 원자값(SSOT)에 저장한다. 폴링 goroutine 의 락-프리
		// 핫 패스가 다음 요청부터 새 unit_id 를 원자적으로 읽어 반영한다(재시작 없음).
		dev.setUnitID(rc.newUnitID)
		a.logger.Info("modbus: unit_id 변경",
			"device", dev.config.ID,
			"unit_id", rc.newUnitID,
		)
	}

	if rc.hasReqTO {
		a.config.RequestTimeout = rc.newReqTO
	}
	if rc.hasReconn {
		a.config.ReconnectInterval = rc.newReconn
	}
	if rc.hasPoll {
		a.config.PollInterval = rc.newPoll
		// pollLoop 에 즉시 반영(SetPollInterval 과 동일한 pollResetCh 시그널, 논블로킹).
		select {
		case a.pollResetCh <- rc.newPoll:
		default:
		}
		a.logger.Info("modbus: 기본 폴링 간격 변경", "interval", rc.newPoll)
	}
}

// reconcileGroupStatsLocked 는 register_groups 변경 시 그룹 통계 맵을 copy-on-write 로 정합화한다
// (update_device 전용, SPEC-008 패턴). 사라진 그룹 키는 제거하고 신규 그룹 키는 추가하되, 유지된
// 그룹의 통계는 보존한다. 디바이스 통계/캐시는 디바이스 ID 가 불변이므로 갱신할 필요가 없다.
// 호출자는 a.mu.Lock() 을 보유해야 한다. 폴링 goroutine 은 groupStats 를 RLock 스냅샷으로 읽으므로
// 맵 전체를 통째 교체하는 이 방식은 경합을 유발하지 않는다(-race).
func (a *ModbusAgent) reconcileGroupStatsLocked(deviceID string, oldGroups, newGroups []RegisterGroupConfig) {
	newKeys := make(map[string]struct{}, len(newGroups))
	for _, rg := range newGroups {
		newKeys[groupStatKey(deviceID, rg.Name)] = struct{}{}
	}

	next := cloneCounterMap(a.groupStats)
	// 사라진(신규 집합에 없는) 그룹 키 제거.
	for _, rg := range oldGroups {
		k := groupStatKey(deviceID, rg.Name)
		if _, keep := newKeys[k]; !keep {
			delete(next, k)
		}
	}
	// 신규 그룹 키 추가(기존 유지 그룹의 통계는 보존).
	for k := range newKeys {
		if _, exists := next[k]; !exists {
			next[k] = &requestCounters{}
		}
	}
	a.groupStats = next
}

// processUpdateDevice 는 기존 디바이스의 설정을 연결을 유지한 채 in-place 로 수정한다(AC-03/AC-04).
//
// 동작 규약:
//   - device_id 는 필수(디바이스 단위 수정 표면). 최상위 필드 또는 params.device_id 를 허용한다.
//   - init 전용 필드(transport 전환·RTU 시리얼 하드웨어 파라미터)는 set_config 와 동일한
//     rejectInitOnlyFields 규칙으로 거부한다(AC-04, 부분 적용 원천 차단).
//   - 변경 필드는 적용 전 전량 검증한다(parseDeviceReconfig, 부분 적용 금지).
//   - 대상 디바이스는 a.mu.Lock() 하에서 findDeviceLocked 로 조회하며, 미존재 시 ErrDeviceNotFound 로
//     원자적으로 거부한다(AC-04, 직전 상태 유지 — 이 시점 이전에는 아무 것도 적용하지 않았다).
//   - register_groups/unit_id/케이던스를 in-place 변경하고, register_groups 변경 시 그룹 통계를
//     copy-on-write 로 정합화한다. 트랜스포트/연결은 절대 재생성하지 않는다(연결 유지, AC-03).
func (a *ModbusAgent) processUpdateDevice(req *processRequest) ([]byte, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("modbus update_device: params required")
	}

	// device_id 는 최상위 필드 우선, 없으면 params.device_id 허용(remove_device 명령 표면과 정합).
	id := req.DeviceID
	if id == "" {
		if s, ok := req.Params["device_id"].(string); ok {
			id = s
		}
	}
	if id == "" {
		return nil, fmt.Errorf("modbus update_device: %w", ErrMissingDeviceID)
	}

	// (1) init 전용 필드 거부 — 어떤 변경보다 먼저 검사하여 부분 적용을 원천 차단한다(AC-04).
	// "port" 만 제외한다: TCP 디바이스의 접속 포트 변경을 지원해야 하기 때문이다.
	// RTU 디바이스의 host/port 변경은 applyEndpointChangeLocked 가 별도로 거부한다.
	if err := rejectInitOnlyFieldsExcept(req.Params, "port"); err != nil {
		return nil, err
	}

	// (2) 변경 필드 파싱·검증 — set_config 와 공유하는 단일 소스(parseDeviceReconfig). 적용 전 전량 검증.
	rc, err := parseDeviceReconfig(req.Params)
	if err != nil {
		return nil, err
	}
	// 엔드포인트(host/port)는 트랜스포트 재생성이 필요해 별도로 파싱한다. set_config 와 공유하지
	// 않는 이유: set_config 는 에이전트 스코프 케이던스 변경 경로이기도 해서 연결 교체 의미가 없다.
	ec, err := parseEndpointChange(req.Params)
	if err != nil {
		return nil, err
	}
	if !rc.hasAny() && !ec.wants() {
		return nil, fmt.Errorf("modbus update_device: no runtime-mutable fields provided")
	}

	// (3) a.mu.Lock() 하에 대상 조회 + 원자적 적용(부분 적용 없음). findDeviceLocked 로 조회하여
	// 미존재 ID 를 원자적으로 거부한다(이 시점 이전에는 상태를 변경하지 않았으므로 직전 상태 유지, AC-04).
	// 블로킹 I/O(구 트랜스포트 close / 신 트랜스포트 connect)는 a.mu 밖에서 수행해야 한다.
	// defer 는 LIFO 이므로, Unlock 보다 **먼저** 등록한 이 defer 가 Unlock 이후에 실행된다.
	var closeOld func()
	var connectTarget *ModbusDevice
	var reqTimeout time.Duration

	a.mu.Lock()
	defer func() {
		if closeOld != nil {
			closeOld()
		}
		if connectTarget != nil {
			a.connectReplacement(connectTarget, reqTimeout)
		}
	}()
	defer a.mu.Unlock()

	dev := a.findDeviceLocked(id)
	if dev == nil {
		return nil, fmt.Errorf("modbus update_device %q: %w", id, ErrDeviceNotFound)
	}

	// register_groups 변경 시 통계 정합을 위해 이전 그룹을 먼저 스냅샷한다(교체 전에 확보).
	oldGroups := dev.config.RegisterGroups

	// 엔드포인트 변경은 다른 필드보다 먼저 처리한다. 실패 시 아무것도 적용되지 않아야 하고
	// (부분 적용 금지), 성공하면 이후 필드는 교체된 디바이스에 적용되어야 하기 때문이다.
	replacement, closer, endpointChanged, err := a.applyEndpointChangeLocked(dev, ec)
	if err != nil {
		return nil, err
	}
	dev = replacement
	closeOld = closer

	// 적용 로직은 set_config 와 공유하는 단일 경로(applyDeviceReconfigLocked)를 사용한다.
	a.applyDeviceReconfigLocked(dev, rc)

	// 그룹 통계 정합(copy-on-write) — SPEC-008 패턴. 캐시는 device id 로 키잉되므로 엔드포인트가
	// 바뀌어도 그대로 유지된다(이력 보존).
	if rc.hasGroups {
		a.reconcileGroupStatsLocked(dev.config.ID, oldGroups, rc.newGroups)
	}

	// 엔드포인트 교체는 폴링 루프를 정지시킨다. 그룹 변경이 함께 있었다면
	// applyDeviceReconfigLocked 가 이미 새 디바이스로 루프를 띄웠으므로 중복 기동하지 않는다.
	polling := a.CurrentState() == lifecycle.StateRunning && a.started && a.config.ReadMode == "cached"
	if endpointChanged && !rc.hasGroups && polling {
		a.startDeviceBlockLoops(dev)
	}
	resp := map[string]any{
		"status":    "device_updated",
		"device_id": id,
		"applied":   appliedSetConfigKeys(rc.hasGroups, rc.hasUnitID, rc.hasPoll, rc.hasReqTO, rc.hasReconn),
	}
	if endpointChanged {
		resp["endpoint_changed"] = true
		// 락 해제 후 실행할 I/O 를 예약한다(위 defer 가 수행).
		connectTarget = dev
		reqTimeout = a.config.RequestTimeout
	}
	return json.Marshal(resp)
}
