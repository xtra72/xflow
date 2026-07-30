package xsfm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// 공통: 이벤트 방출 + 제어 결과/감사 시임
// ---------------------------------------------------------------------------

// sendEvent 는 노드 방출 이벤트를 msgCh 로 non-blocking 전송한다 (samsung sendEvent 패턴).
// 버퍼가 가득 차면 드롭하고 로그만 남긴다. 락 없이 호출한다.
func (a *XSFMAgent) sendEvent(eventType string, data map[string]any) {
	evt := map[string]any{"type": eventType}
	for k, v := range data {
		evt[k] = v
	}
	b, err := json.Marshal(evt)
	if err != nil {
		a.logger.Warn("xsfm: event marshal failed", "type", eventType, "error", err)
		return
	}
	select {
	case a.msgCh <- b:
		// 내부 발신 경계: 노드로 이벤트 enqueue 성공.
		a.stats.IncrInternalMessagesSent()
	default:
		a.stats.IncrDroppedMessages()
		a.logger.Debug("xsfm: msgCh full, dropping event", "type", eventType)
	}
}

// logFrame 은 log_messages 옵션이 켜져 있을 때 송/수신 프레임을 INFO 로 로그한다 (opt-in 진단).
//
// 호출부가 a.cfg.LogMessages 를 먼저 확인하므로(토글 false 시 zero overhead), 여기서는 방출만
// 한다. dir 은 "RX"(상태 유입) 또는 "TX"(명령 방출), topic 은 대상 MQTT 토픽(port 모드는 재구성한
// 토픽 또는 device_id), summary 는 디코드된 축/값 요약이다.
func (a *XSFMAgent) logFrame(dir, topic string, payload []byte, summary string) {
	if a.logger == nil {
		return
	}
	a.logger.Info("xsfm: "+dir,
		"topic", topic,
		"len", len(payload),
		"payload", string(payload),
		"decoded", summary,
	)
}

// stateSummary 는 디코드된 상태 축(관측된 것만)을 사람이 읽을 요약 문자열로 만든다 (RX 로그용).
func stateSummary(st decodedState) string {
	parts := make([]string, 0, 3)
	if st.PowerSet {
		parts = append(parts, fmt.Sprintf("power=%v", st.Power))
	}
	if st.FanSpeedSet {
		parts = append(parts, fmt.Sprintf("fan_speed=%d", st.FanSpeed))
	}
	if st.OnlineSet {
		parts = append(parts, fmt.Sprintf("online=%v", st.Online))
	}
	return strings.Join(parts, " ")
}

// commandSummary 는 명령 페이로드의 설정된 축을 요약 문자열로 만든다 (blob 모드 TX 로그용).
func commandSummary(command string, cmd commandPayload) string {
	parts := make([]string, 0, 2)
	if cmd.Power != nil {
		parts = append(parts, fmt.Sprintf("power=%v", *cmd.Power))
	}
	if cmd.FanSpeed != nil {
		parts = append(parts, fmt.Sprintf("fan_speed=%d", *cmd.FanSpeed))
	}
	return strings.TrimSpace(command + " " + strings.Join(parts, " "))
}

// renderCommandTopic 은 TX 로그용으로 명령 토픽을 재구성한다 (attribute 모드는 {attribute} 포함).
// command_topic_template 이 비면(port 모드 등) "" 를 반환한다 — 로그는 device_id 로 식별한다.
func (a *XSFMAgent) renderCommandTopic(fields map[string]string, attribute string) string {
	if a.cfg.CommandTopicTemplate == "" {
		return ""
	}
	f := fields
	if attribute != "" {
		f = make(map[string]string, len(fields)+1)
		for k, v := range fields {
			f[k] = v
		}
		f[placeholderAttribute] = attribute
	}
	return renderTopic(a.cfg.CommandTopicTemplate, f)
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

// recordControlAudit 는 제어 감사 레코드 기록 표면이다 (REQ-XSFM-001-06-03). 구현은
// audit.go 에 있다 — controlDevice 가 개별/fan-out 멤버 제어 양쪽에서 호출한다(B7).

// controlDevice 는 단일 디바이스 제어의 공통 진입점이다: 디바이스 확인 → 주소 필드 도출 →
// 모드 디스패치 (REQ-XSFM-001-03-01..07, M14).
//
// cmdSink 만 egress 로 사용하므로 direct(브로커 발행)·port(제어 출력 포트 emit) 두 모드가
// 동일한 코드 경로를 탄다 (REQ-XSFM-001-01-11). 명령 템플릿에 {attribute} 가 있으면
// attribute-per-topic(축별 스칼라 발행), 없으면 기존 JSON-blob 발행으로 디스패치한다.
func (a *XSFMAgent) controlDevice(deviceID, command string, cmd commandPayload) (memberResult, error) {
	// 디바이스 존재 확인 + 주소 필드 스냅샷 (락 하에 뜬 뒤 해제 — RWMutex 재진입 트랩 회피).
	// dev.Station 도 함께 스냅샷하여 {line_code} 파생 해석(로스터 락 밖)의 입력으로 쓴다.
	a.mu.RLock()
	dev, exists := a.devices[deviceID]
	if !exists {
		a.mu.RUnlock()
		return memberResult{}, fmt.Errorf("%w: %q", ErrDeviceNotFound, deviceID)
	}
	fields := a.buildCommandFieldsLocked(dev)
	station := dev.Station
	a.mu.RUnlock()

	// {line_code} 파생 렌더 (SPEC-XSFM-LINE-001 RD-8, REQ-08-02/03/04): 명령 템플릿에 {line_code} 가
	// 있으면 디바이스의 파생 라인 ResolveLine(station) 으로 채운다. deviceFieldValue 는 라인을 알지
	// 못하므로(라인은 Device 필드가 아니라 station 파생값, station→line SSOT) buildCommandFieldsLocked
	// 은 {line_code} 를 비운 채 두며, 여기서 lineHint 로 주입한다. 라인 미배정이면 resolveLineFor 가
	// "" 를 반환해 빈 세그먼트로 렌더된다(REQ-08-04, 예: cmd//st99/...).
	//
	// @MX:WARN: [AUTO] 라인 해석(resolveLineFor→station 레지스트리 자체 락)은 반드시 로스터 락(a.mu)을
	// 해제한 뒤 수행한다(composeName 의 lineHint 패턴 미러). 위 station 스냅샷 후 RUnlock 을 거쳐
	// 이 지점에서 해석하므로 station 레지스트리 락이 로스터 락 내부에서 취득되지 않는다.
	// @MX:REASON: 로스터 락 보유 상태에서 station 레지스트리 락을 취득하면 프로젝트 RWMutex 재진입
	// deadlock 트랩에 걸린다(REQ-07-01·REQ-08-03). 락 순서를 직렬화(스냅샷→해제→해석→주입)해 회피한다.
	if a.cfg.commandHasLineCode {
		fields[placeholderLineCode] = a.resolveLineFor(station)
	}

	if a.cfg.commandHasAttribute {
		return a.controlDeviceAttr(deviceID, command, cmd, fields)
	}
	return a.controlDeviceBlob(deviceID, command, cmd, fields)
}

// controlDeviceBlob 는 JSON-blob 모드의 단일 디바이스 제어이다 (하위호환 경로). 설정된 축을
// 하나의 JSON 페이로드로 인코딩하여 주소 필드로 렌더한 토픽으로 1회 발행하고, 하나의 pending 을
// 등록해 에코를 기다린다 (REQ-03-08/09). timeout==0 이면 fire-and-forget.
func (a *XSFMAgent) controlDeviceBlob(deviceID, command string, cmd commandPayload, fields map[string]string) (memberResult, error) {
	payload, err := encodeCommandPayload(a.cfg.PayloadMapping, cmd)
	if err != nil {
		return memberResult{}, err
	}

	var p *pending
	if a.cfg.ControlResponseTimeout > 0 {
		p = a.pendings.register(deviceID, command, expectFromPayload(cmd), a.cfg.ControlResponseTimeout)
	}

	if a.cfg.LogMessages {
		a.logFrame("TX", a.renderCommandTopic(fields, ""), payload, commandSummary(command, cmd))
	}
	if err := a.cmdSink.SendCommand(outboundCommand{DeviceID: deviceID, Fields: fields, Payload: payload}); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.pendings.cancel(p)
		return memberResult{}, err
	}
	// 외부 발신 경계: 명령 publish 성공.
	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(payload)))
	a.stats.UpdateLastActivity()

	if p != nil {
		if err := p.wait(); err != nil {
			return memberResult{DeviceID: deviceID, Command: command, Status: "timeout"}, err
		}
	}

	res := memberResult{DeviceID: deviceID, Command: command, Status: "ok"}
	a.recordControlAudit(res)
	return res, nil
}

// controlDeviceAttr 는 attribute-per-topic 모드의 단일 디바이스 제어이다 (M14). 설정된 각 축을
// 독립 발행으로 분해한다: 전원은 payload_mapping.power_field 토큰의 토픽으로, 풍량은
// fan_speed_field 토큰의 토픽으로 축 스칼라를 발행한다(전원 먼저, 풍량 그다음). 각 축은 독립
// pending(축별 command kind 키)으로 에코를 기다리므로, 축별 상태 에코가 독립적으로 해소한다.
func (a *XSFMAgent) controlDeviceAttr(deviceID, command string, cmd commandPayload, fields map[string]string) (memberResult, error) {
	m := a.cfg.PayloadMapping
	type axisEmit struct {
		attr    string
		command string
		payload []byte
		expect  pendingExpect
	}
	var axes []axisEmit
	if cmd.Power != nil {
		axes = append(axes, axisEmit{
			attr:    m.Power.Name,
			command: "set_power",
			payload: m.commandWireBytes(m.Power.encodeBool(*cmd.Power)),
			expect:  expectFromPayload(commandPayload{Power: cmd.Power}),
		})
	}
	if cmd.FanSpeed != nil {
		axes = append(axes, axisEmit{
			attr:    m.FanSpeed.Name,
			command: "set_fan_speed",
			payload: m.commandWireBytes(m.FanSpeed.encodeInt(*cmd.FanSpeed)),
			expect:  expectFromPayload(commandPayload{FanSpeed: cmd.FanSpeed}),
		})
	}

	for _, ax := range axes {
		var p *pending
		if a.cfg.ControlResponseTimeout > 0 {
			p = a.pendings.register(deviceID, ax.command, ax.expect, a.cfg.ControlResponseTimeout)
		}
		out := outboundCommand{DeviceID: deviceID, Fields: fields, Attribute: ax.attr, Payload: ax.payload}
		if a.cfg.LogMessages {
			a.logFrame("TX", a.renderCommandTopic(fields, ax.attr), ax.payload, ax.command+" ["+ax.attr+"]")
		}
		if err := a.cmdSink.SendCommand(out); err != nil {
			a.stats.IncrExternalMessagesErrored()
			a.pendings.cancel(p)
			return memberResult{}, err
		}
		// 외부 발신 경계: 축별 명령 publish 성공.
		a.stats.IncrExternalMessagesSent()
		a.stats.AddBytesWritten(int64(len(ax.payload)))
		a.stats.UpdateLastActivity()
		if p != nil {
			if err := p.wait(); err != nil {
				return memberResult{DeviceID: deviceID, Command: command, Status: "timeout"}, err
			}
		}
	}

	res := memberResult{DeviceID: deviceID, Command: command, Status: "ok"}
	a.recordControlAudit(res)
	return res, nil
}

// devicePowerSnapshot 은 디바이스의 현재 전원 상태를 스냅샷한다 (RLock 하에 읽고 해제).
// ok=false 는 미등록 디바이스를 뜻한다.
func (a *XSFMAgent) devicePowerSnapshot(deviceID string) (power bool, ok bool) {
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

// handleSetPower 는 set_power 단일 디바이스 명령을 처리한다 (REQ-XSFM-001-03-01/02).
// device_id 없는(셀렉터) 경로는 dispatchControl 이 handleSelectorControl 로 라우팅한다(B4).
func (a *XSFMAgent) handleSetPower(req processRequest) ([]byte, error) {
	power, err := boolParam(req.Params, "power")
	if err != nil {
		return nil, err
	}
	if _, err := a.controlDevice(req.DeviceID, "set_power", commandPayload{Power: &power}); err != nil {
		return nil, err
	}
	return controlResponse(req.DeviceID, "set_power")
}

// handleSetFanSpeed 는 set_fan_speed 명령을 처리한다 (REQ-XSFM-001-03-03/05).
//
// 풍량은 {1,2,3} 만 유효하며(그 외 ErrInvalidFanSpeed, 방출 없음), 전원 OFF 상태에서는
// fan_speed_power_off_policy 에 따라 거부(기본 ErrPowerOff) 또는 전원 ON 선방출한다.
func (a *XSFMAgent) handleSetFanSpeed(req processRequest) ([]byte, error) {
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

// handleSetMultiple 는 set_multiple 명령을 처리한다 (REQ-XSFM-001-03-04).
//
// power=true 이며 fan_speed 도 주어지면 전원 ON 을 먼저 방출한 뒤 풍량을 방출한다(순서 중요).
// 그 외에는 설정된 축을 단일 페이로드로 방출한다.
func (a *XSFMAgent) handleSetMultiple(req processRequest) ([]byte, error) {
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
// Module 2 — 런타임 디바이스 CRUD (REQ-XSFM-001-02-04/06)
// ---------------------------------------------------------------------------

// handleAddDevice 는 add_device 명령을 처리한다 (Source="bridge", Online=false).
// 양 모드에서 device_registered 이벤트를 방출한다. state 토픽 구독은 Start 의 단일 와일드카드
// 구독이 모든 디바이스를 이미 커버하므로 디바이스별 구독을 하지 않는다 (M14).
func (a *XSFMAgent) handleAddDevice(req processRequest) ([]byte, error) {
	// 라인·역사 세그먼트 해석은 로스터 락 취득 전에 수행한다(레지스트리 간 락 중첩 금지, REQ-07-01).
	// line 미해석이면 "" → composeName 이 3-세그먼트로 합성한다(RD-4). stationDisp 는 역번호가 있으면
	// 역번호·없으면 역사 코드 폴백이다.
	line := a.resolveLineFor(req.Station)
	stationDisp := a.stationDisplayFor(req.Station)

	a.mu.Lock()

	// 명시적 device_id 경로(하위호환): 사용자/브리지가 부여한 device_id 를 그대로 로스터 키로
	// 쓴다. blob/{device_id} 모델·기존 테스트 경로를 정확히 보존한다. Name 은 사용자 제공 값 유지.
	if req.DeviceID != "" {
		if _, exists := a.devices[req.DeviceID]; exists {
			a.mu.Unlock()
			return nil, fmt.Errorf("%w: %q", ErrDeviceAlreadyRegistered, req.DeviceID)
		}
		dev := &Device{
			DeviceID: req.DeviceID,
			Name:     req.Name,
			GroupID:  req.GroupID,
			Station:  req.Station,
			Place:    req.Place,
			Index:    req.Index,
			Online:   false,
			Source:   "bridge",
		}
		a.devices[req.DeviceID] = dev
		a.indexDeviceLocked(dev) // 위치 계층이 있으면 보조 인덱스에도 등록.
		a.mu.Unlock()
		return a.finishAddDevice(req.DeviceID, dev.Name)
	}

	// device_id 미지정 경로(합성 주소 모델): UUID 를 생성하고 Name 을 합성한다. 같은 위치 계층
	// 주소가 이미 있으면(보조 인덱스 hit) 중복 UUID 를 만들지 않고 ErrDeviceAlreadyRegistered.
	ck := compositeKey(req.Station, req.Place, req.Index)
	if _, exists := a.secondary[ck]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: composite %q", ErrDeviceAlreadyRegistered, ck)
	}
	id := newDeviceID()
	// 사용자가 비어있지 않은 name 을 주면 커스텀 이름으로 고정(sticky)하고, 아니면 위치 계층에서
	// 파생한다. sticky 여부는 nameOverridden 으로 표시해 이후 주소 변경 시 재계산을 막는다.
	name := composeName(line, stationDisp, req.Place, req.Index)
	nameOverridden := false
	if req.Name != "" {
		name = req.Name
		nameOverridden = true
	}
	dev := &Device{
		DeviceID:       id,
		Name:           name,
		GroupID:        req.GroupID,
		Station:        req.Station,
		Place:          req.Place,
		Index:          req.Index,
		Online:         false,
		Source:         "bridge",
		composite:      true,
		nameOverridden: nameOverridden,
	}
	a.devices[id] = dev
	a.indexDeviceLocked(dev)
	a.mu.Unlock()
	return a.finishAddDevice(id, dev.Name)
}

// finishAddDevice 는 add_device 성공 후 공통 마무리(이벤트 방출·영속화)를 수행하고, UI 표시용으로
// device_id + name 을 담은 응답을 반환한다. 락 미보유 상태에서 호출한다.
func (a *XSFMAgent) finishAddDevice(deviceID, name string) ([]byte, error) {
	a.sendEvent("device_registered", map[string]any{"device_id": deviceID, "source": "bridge"})

	// B7: 로스터 변경 후 영속화(best-effort — 실패해도 등록 자체는 성공).
	a.persistRoster()

	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": deviceID,
		"name":      name,
		"command":   "add_device",
		"source":    "bridge",
	})
}

// handleRemoveDevice 는 remove_device 명령을 처리한다. Source="config" 디바이스는 보호된다.
func (a *XSFMAgent) handleRemoveDevice(req processRequest) ([]byte, error) {
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
	a.unindexDeviceLocked(dev) // 보조 인덱스 항목도 함께 제거(로스터와 동기화).
	a.mu.Unlock()

	// M14: state 토픽은 단일 와일드카드 구독이 모든 디바이스를 커버하므로 디바이스별 Unsubscribe
	// 를 하지 않는다. 제거된 디바이스로의 유입은 handleStateMessage 에서 자동 재등록될 수 있으나,
	// bridge 삭제는 런타임 명령이므로 이후 유입은 auto 로 재등록되는 정상 동작이다.

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
func (a *XSFMAgent) handleSetDevice(req processRequest, raw []byte) ([]byte, error) {
	if req.DeviceID == "" {
		return nil, fmt.Errorf("%w: set_device requires device_id", ErrInvalidCommand)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("xsfm set_device: %w", err)
	}

	// present 는 부분 갱신 대상 필드 판별기이다. HTTP exec 경로는 본문을 표준 {command, params}
	// 계약으로 재직렬화하므로 top-level 키에는 필드가 없고 params 에만 존재한다. top-level raw 키
	// 또는 params 키 중 어느 한쪽에 존재하면 갱신 대상으로 본다 (갱신 값 자체는 fillFromParams 가
	// 이미 req 로 backfill 해둠).
	present := func(key string) bool {
		if _, ok := fields[key]; ok {
			return true
		}
		if req.Params != nil {
			if _, ok := req.Params[key]; ok {
				return true
			}
		}
		return false
	}

	a.mu.Lock()
	dev, ok := a.devices[req.DeviceID]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
	}
	// 위치 계층(station/place/index) 변경 시 보조 인덱스를 갱신한다: 옛 compositeKey 를 먼저
	// 제거하고(변경 전 값 기준), 필드 갱신 후 새 compositeKey 로 재등록한다.
	addrChange := present("station") || present("place") || present("index")
	if addrChange {
		a.unindexDeviceLocked(dev)
	}
	// name 처리: 비어있지 않은 값이면 sticky override 로 고정하고, 명시적 빈값이면 override 를
	// 해제한다. blob(비-composite) 디바이스는 Name 을 준 값 그대로 반영한다(기존 동작 보존).
	if present("name") {
		switch {
		case req.Name != "":
			dev.Name = req.Name
			dev.nameOverridden = true
		case dev.composite:
			// composite + 명시적 빈값 = override 해제 → 아래에서 파생 이름으로 복귀.
			dev.nameOverridden = false
		default:
			// blob + 명시적 빈값: 사용자 관리값이므로 준 값(빈값)을 그대로 반영.
			dev.Name = req.Name
		}
	}
	if present("group_id") {
		dev.GroupID = req.GroupID
	}
	if present("station") {
		dev.Station = req.Station
	}
	if present("place") {
		dev.Place = req.Place
	}
	if present("index") {
		dev.Index = req.Index
	}
	// composite 파생 이름 재계산 판정: override 가 아닌 composite 디바이스에 한해, 주소 변경 또는
	// name override 해제(present("name"))를 트리거로 새 위치에서 Name 을 재계산한다(RD-4 4-세그먼트
	// 재계산 포함). nameOverridden(sticky)·blob 은 재계산 없음. 실제 재계산은 라인 세그먼트 해석을
	// 위해 로스터 락 해제 후 수행한다(레지스트리 간 락 중첩 금지, REQ-07-01).
	needRecompute := dev.composite && !dev.nameOverridden && (addrChange || present("name"))
	recStation, recPlace, recIndex := dev.Station, dev.Place, dev.Index
	if addrChange {
		a.indexDeviceLocked(dev)
	}
	a.mu.Unlock()

	// 파생 이름 재계산(RD-4): 라인 코드 해석(ResolveLine, station 레지스트리 자체 락)은 로스터 락을
	// 해제한 뒤 수행하고, 결과만 두 번째 짧은 임계구역에서 반영한다 — 두 락을 중첩하지 않는다.
	// 재확인(composite && !nameOverridden) 후 반영해 그 사이 override 가 걸렸으면 sticky 를 존중한다.
	if needRecompute {
		line := a.resolveLineFor(recStation)
		stationDisp := a.stationDisplayFor(recStation)
		newName := composeName(line, stationDisp, recPlace, recIndex)
		a.mu.Lock()
		if d, ok := a.devices[req.DeviceID]; ok && d.composite && !d.nameOverridden {
			d.Name = newName
		}
		a.mu.Unlock()
	}

	// B7: 로스터 변경 후 영속화(best-effort). 설정 디바이스 변경은 스냅샷에서 제외되어 반영되지 않는다.
	a.persistRoster()

	return json.Marshal(map[string]any{
		"status":    "ok",
		"device_id": req.DeviceID,
		"command":   "set_device",
	})
}

// handleListDevices 는 list_devices 명령을 처리한다. 전체 디바이스를 JSON 으로 반환한다.
func (a *XSFMAgent) handleListDevices() ([]byte, error) {
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
