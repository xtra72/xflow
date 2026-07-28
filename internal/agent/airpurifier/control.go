package airpurifier

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// 공통: 이벤트 방출 + 제어 결과/감사 시임
// ---------------------------------------------------------------------------

// sendEvent 는 노드 방출 이벤트를 msgCh 로 non-blocking 전송한다 (samsung sendEvent 패턴).
// 버퍼가 가득 차면 드롭하고 로그만 남긴다. 락 없이 호출한다.
func (a *AirPurifierAgent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("airpurifier: event marshal failed", "type", eventType, "error", err)
		return
	}
	select {
	case a.msgCh <- b:
	default:
		a.logger.Debug("airpurifier: msgCh full, dropping event", "type", eventType)
	}
}

// memberResult 는 단일 디바이스 제어 명령의 결과이다.
//
// device_id + command + status 를 담아 B3(응답 대기 resolve)·B4(셀렉터 fan-out 집계)·
// B7(원격 감사 소비)이 재사용한다. 현재는 fire-and-forget 이라 항상 status "ok".
type memberResult struct {
	DeviceID string `json:"device_id"`
	Command  string `json:"command"`
	Status   string `json:"status"`
}

// recordControlAudit 는 제어 감사 레코드 기록 표면이다 (REQ-AIRPUR-001-06-03). 구현은
// audit.go 에 있다 — controlDevice 가 개별/fan-out 멤버 제어 양쪽에서 호출한다(B7).

// controlDevice 는 단일 디바이스 제어의 공통 경로이다: 디바이스 확인 → 인코딩 → cmdSink
// 방출 → 감사 → 결과 반환 (REQ-AIRPUR-001-03-01..07).
//
// cmdSink 만 egress 로 사용하므로 direct(브로커 발행)·port(제어 출력 포트 emit) 두 모드가
// 동일한 코드 경로를 탄다 (REQ-AIRPUR-001-01-11). B2 는 fire-and-forget 이지만, B3 이
// 방출 직전 pending 등록 / 방출 직후 에코 대기를 재구조화 없이 삽입할 수 있도록 방출을
// 명시적 시임으로 분리해 둔다.
func (a *AirPurifierAgent) controlDevice(deviceID, command string, cmd commandPayload) (memberResult, error) {
	// 디바이스 존재 확인 (락 하에 스냅샷 후 해제 — RWMutex 재진입 트랩 회피).
	a.mu.RLock()
	_, exists := a.devices[deviceID]
	a.mu.RUnlock()
	if !exists {
		return memberResult{}, fmt.Errorf("%w: %q", ErrDeviceNotFound, deviceID)
	}

	payload, err := encodeCommandPayload(a.cfg.PayloadMapping, cmd)
	if err != nil {
		return memberResult{}, err
	}

	// B3: control_response_timeout>0 이면 방출 직전 pending 을 등록한다. register→emit→wait
	// 순서를 지켜, 방출 직후 race 로 되돌아오는 에코를 놓치지 않는다 (REQ-03-08). timeout==0
	// 이면 pending 없이 fire-and-forget 한다 (B2 동작 그대로).
	var p *pending
	if a.cfg.ControlResponseTimeout > 0 {
		p = a.pendings.register(deviceID, command, expectFromPayload(cmd), a.cfg.ControlResponseTimeout)
	}

	if err := a.cmdSink.SendCommand(deviceID, payload); err != nil {
		a.pendings.cancel(p) // 방출 실패 시 등록된 pending 정리 (p==nil 이면 no-op).
		return memberResult{}, err
	}

	// B3: 방출 직후 에코 대기 (timeout>0). 에코 해소 → status "ok"; 타임아웃 → ErrControlTimeout
	// + status "timeout" (pending 은 finalize 에서 이미 제거됨, REQ-03-09).
	if p != nil {
		if err := p.wait(); err != nil {
			return memberResult{DeviceID: deviceID, Command: command, Status: "timeout"}, err
		}
	}

	res := memberResult{DeviceID: deviceID, Command: command, Status: "ok"}
	a.recordControlAudit(res)
	return res, nil
}

// devicePowerSnapshot 은 디바이스의 현재 전원 상태를 스냅샷한다 (RLock 하에 읽고 해제).
// ok=false 는 미등록 디바이스를 뜻한다.
func (a *AirPurifierAgent) devicePowerSnapshot(deviceID string) (power bool, ok bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dev, exists := a.devices[deviceID]
	if !exists {
		return false, false
	}
	return dev.Power, true
}

// controlResponse 는 제어 명령 성공 응답 JSON 을 생성한다.
func controlResponse(deviceID, command string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": deviceID,
		"command":   command,
	})
}

// ---------------------------------------------------------------------------
// Module 3 — 개별 제어 (2-축: power on/off, fan_speed 1/2/3)
// ---------------------------------------------------------------------------

// handleSetPower 는 set_power 단일 디바이스 명령을 처리한다 (REQ-AIRPUR-001-03-01/02).
// device_id 없는(셀렉터) 경로는 dispatchControl 이 handleSelectorControl 로 라우팅한다(B4).
func (a *AirPurifierAgent) handleSetPower(req processRequest) ([]byte, error) {
	power, err := boolParam(req.Params, "power")
	if err != nil {
		return nil, err
	}
	if _, err := a.controlDevice(req.DeviceID, "set_power", commandPayload{Power: &power}); err != nil {
		return nil, err
	}
	return controlResponse(req.DeviceID, "set_power")
}

// handleSetFanSpeed 는 set_fan_speed 명령을 처리한다 (REQ-AIRPUR-001-03-03/05).
//
// 풍량은 {1,2,3} 만 유효하며(그 외 ErrInvalidFanSpeed, 방출 없음), 전원 OFF 상태에서는
// fan_speed_power_off_policy 에 따라 거부(기본 ErrPowerOff) 또는 전원 ON 선방출한다.
func (a *AirPurifierAgent) handleSetFanSpeed(req processRequest) ([]byte, error) {
	fs, err := intParam(req.Params, "fan_speed")
	if err != nil {
		return nil, err
	}
	if fs < 1 || fs > 3 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidFanSpeed, fs)
	}

	// 전원 OFF 게이트: 미등록이면 ErrDeviceNotFound 를 우선한다.
	power, ok := a.devicePowerSnapshot(req.DeviceID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
	}
	if !power {
		switch a.cfg.FanSpeedPowerOffPolicy {
		case fanSpeedPolicyPowerOnFirst:
			// 전원 ON 을 먼저 방출한 뒤 풍량을 방출한다 (순서 보장).
			on := true
			if _, err := a.controlDevice(req.DeviceID, "set_power", commandPayload{Power: &on}); err != nil {
				return nil, err
			}
		default: // fanSpeedPolicyReject
			return nil, fmt.Errorf("%w: device %q", ErrPowerOff, req.DeviceID)
		}
	}

	if _, err := a.controlDevice(req.DeviceID, "set_fan_speed", commandPayload{FanSpeed: &fs}); err != nil {
		return nil, err
	}
	return controlResponse(req.DeviceID, "set_fan_speed")
}

// handleSetMultiple 는 set_multiple 명령을 처리한다 (REQ-AIRPUR-001-03-04).
//
// power=true 이며 fan_speed 도 주어지면 전원 ON 을 먼저 방출한 뒤 풍량을 방출한다(순서 중요).
// 그 외에는 설정된 축을 단일 페이로드로 방출한다.
func (a *AirPurifierAgent) handleSetMultiple(req processRequest) ([]byte, error) {
	powerPtr, fanPtr, err := parseSetMultipleParams(req.Params)
	if err != nil {
		return nil, err
	}

	// 존재 확인 (미등록 → ErrDeviceNotFound).
	if _, ok := a.devicePowerSnapshot(req.DeviceID); !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
	}

	if powerPtr != nil && *powerPtr && fanPtr != nil {
		// REQ-03-04: 전원 ON 먼저, 그다음 fan_speed.
		if _, err := a.controlDevice(req.DeviceID, "set_power", commandPayload{Power: powerPtr}); err != nil {
			return nil, err
		}
		if _, err := a.controlDevice(req.DeviceID, "set_fan_speed", commandPayload{FanSpeed: fanPtr}); err != nil {
			return nil, err
		}
	} else {
		// 단일 결합 방출 (power 단독 / fan 단독 / power=false+fan).
		if _, err := a.controlDevice(req.DeviceID, "set_multiple", commandPayload{Power: powerPtr, FanSpeed: fanPtr}); err != nil {
			return nil, err
		}
	}
	return controlResponse(req.DeviceID, "set_multiple")
}

// ---------------------------------------------------------------------------
// Module 2 — 런타임 디바이스 CRUD (REQ-AIRPUR-001-02-04/06)
// ---------------------------------------------------------------------------

// handleAddDevice 는 add_device 명령을 처리한다 (Source="bridge", Online=false).
// direct 모드에서는 state 토픽을 구독하고, 양 모드에서 device_registered 이벤트를 방출한다.
func (a *AirPurifierAgent) handleAddDevice(req processRequest) ([]byte, error) {
	if req.DeviceID == "" {
		return nil, fmt.Errorf("%w: add_device requires device_id", ErrInvalidCommand)
	}

	a.mu.Lock()
	if _, exists := a.devices[req.DeviceID]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrDeviceAlreadyRegistered, req.DeviceID)
	}
	a.devices[req.DeviceID] = &Device{
		DeviceID: req.DeviceID,
		Name:     req.Name,
		GroupID:  req.GroupID,
		Station:  req.Station,
		Place:    req.Place,
		Index:    req.Index,
		Online:   false,
		Source:   "bridge",
	}
	a.mu.Unlock()

	// state 토픽 구독 (direct 모드만; port 모드는 client==nil 이라 건너뛴다). 락 해제 후 수행.
	if a.client != nil {
		if err := a.subscribeDeviceState(req.DeviceID); err != nil {
			a.logger.Error("airpurifier: add_device subscribe failed", "device_id", req.DeviceID, "error", err)
		}
	}

	a.sendEvent("device_registered", map[string]any{"device_id": req.DeviceID, "source": "bridge"})

	// B7: 로스터 변경 후 영속화(best-effort — 실패해도 등록 자체는 성공). 락 미보유 상태에서 호출.
	a.persistRoster()

	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": req.DeviceID,
		"command":   "add_device",
		"source":    "bridge",
	})
}

// handleRemoveDevice 는 remove_device 명령을 처리한다. Source="config" 디바이스는 보호된다.
func (a *AirPurifierAgent) handleRemoveDevice(req processRequest) ([]byte, error) {
	if req.DeviceID == "" {
		return nil, fmt.Errorf("%w: remove_device requires device_id", ErrInvalidCommand)
	}

	a.mu.Lock()
	dev, ok := a.devices[req.DeviceID]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
	}
	if dev.Source == "config" {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrConfigDeviceProtected, req.DeviceID)
	}
	delete(a.devices, req.DeviceID)
	a.mu.Unlock()

	// B5: direct 모드에서 제거된 디바이스의 state 토픽 구독을 해제한다 (B2 문서화 gap 종결).
	// 락 해제 후 수행한다. port 모드는 client==nil 이라 건너뛴다.
	if a.client != nil {
		topic := renderTopic(a.cfg.StateTopicTemplate, req.DeviceID)
		if err := a.client.Unsubscribe(topic); err != nil {
			a.logger.Warn("airpurifier: remove_device unsubscribe failed", "device_id", req.DeviceID, "topic", topic, "error", err)
		}
	}

	a.sendEvent("device_unregistered", map[string]any{"device_id": req.DeviceID})

	// B7: 로스터 변경 후 영속화(best-effort). 삭제된 디바이스가 스냅샷에서 빠져 파일에 반영된다.
	a.persistRoster()

	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": req.DeviceID,
		"command":   "remove_device",
	})
}

// handleSetDevice 는 set_device 명령을 처리한다 (name/group_id/station/place/index 부분 갱신).
//
// raw 페이로드의 키 존재 여부로 부분 갱신을 판정하여, 제공되지 않은 필드는 보존한다.
// group_id 변경은 GroupMembers 에 즉시 반영된다 (로스터 속성에서 도출하므로).
func (a *AirPurifierAgent) handleSetDevice(req processRequest, raw []byte) ([]byte, error) {
	if req.DeviceID == "" {
		return nil, fmt.Errorf("%w: set_device requires device_id", ErrInvalidCommand)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("airpurifier set_device: %w", err)
	}

	a.mu.Lock()
	dev, ok := a.devices[req.DeviceID]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
	}
	if _, present := fields["name"]; present {
		dev.Name = req.Name
	}
	if _, present := fields["group_id"]; present {
		dev.GroupID = req.GroupID
	}
	if _, present := fields["station"]; present {
		dev.Station = req.Station
	}
	if _, present := fields["place"]; present {
		dev.Place = req.Place
	}
	if _, present := fields["index"]; present {
		dev.Index = req.Index
	}
	a.mu.Unlock()

	// B7: 로스터 변경 후 영속화(best-effort). 설정 디바이스 변경은 스냅샷에서 제외되어 반영되지 않는다.
	a.persistRoster()

	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": req.DeviceID,
		"command":   "set_device",
	})
}

// handleListDevices 는 list_devices 명령을 처리한다. 전체 디바이스를 JSON 으로 반환한다.
func (a *AirPurifierAgent) handleListDevices() ([]byte, error) {
	devs := a.ListDevices() // RLock 하에 정렬된 값 복사본을 반환한다.
	out := make([]map[string]any, 0, len(devs))
	for _, d := range devs {
		out = append(out, map[string]any{
			"device_id": d.DeviceID,
			"name":      d.Name,
			"group_id":  d.GroupID,
			"station":   d.Station,
			"place":     d.Place,
			"index":     d.Index,
			"online":    d.Online,
			"power":     d.Power,
			"fan_speed": d.FanSpeed,
			"source":    d.Source,
		})
	}
	return json.Marshal(map[string]any{"status": "ok", "devices": out})
}

// ---------------------------------------------------------------------------
// params 추출 헬퍼 (JSON 숫자는 float64 로 전달되므로 통일 처리).
// ---------------------------------------------------------------------------

// boolParam 은 params 에서 boolean 값을 추출한다. 누락/타입 불일치 시 ErrInvalidCommand.
func boolParam(params map[string]any, key string) (bool, error) {
	v, ok := params[key]
	if !ok {
		return false, fmt.Errorf("%w: missing %q param", ErrInvalidCommand, key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %q must be a boolean", ErrInvalidCommand, key)
	}
	return b, nil
}

// intParam 은 params 에서 정수 값을 추출한다. 누락/타입 불일치 시 ErrInvalidCommand.
func intParam(params map[string]any, key string) (int, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("%w: missing %q param", ErrInvalidCommand, key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("%w: %q must be an integer", ErrInvalidCommand, key)
	}
}

// parseSetMultipleParams 는 set_multiple params 에서 power/fan_speed 축을 파싱·검증한다.
//
// 존재하는 축만 non-nil 로 반환하며(부분 갱신), 풍량은 {1,2,3} 만 허용한다. 두 축 모두
// 없으면 ErrInvalidCommand. 개별 제어(handleSetMultiple)와 셀렉터 fan-out(buildControlPlan)이
// 동일 검증을 공유하도록 추출됐다 (REQ-04-08 중복 구현 금지).
func parseSetMultipleParams(params map[string]any) (power *bool, fan *int, err error) {
	if _, has := params["power"]; has {
		p, e := boolParam(params, "power")
		if e != nil {
			return nil, nil, e
		}
		power = &p
	}
	if _, has := params["fan_speed"]; has {
		fs, e := intParam(params, "fan_speed")
		if e != nil {
			return nil, nil, e
		}
		if fs < 1 || fs > 3 {
			return nil, nil, fmt.Errorf("%w: got %d", ErrInvalidFanSpeed, fs)
		}
		fan = &fs
	}
	if power == nil && fan == nil {
		return nil, nil, fmt.Errorf("%w: set_multiple requires power and/or fan_speed", ErrInvalidCommand)
	}
	return power, fan, nil
}
