package xsfm

import "time"

// minStateEmitInterval 은 주기 상태 방출기 ticker 의 최소 주기이다. state_emit_interval 이 매우
// 짧을 때 tick 간격이 0 이 되어 time.NewTicker 가 panic 하는 것을 막는 하한이다
// (SPEC-XSFM-AGENT-IO-001 RD-3, monitor.go 의 minMonitorInterval 미러).
const minStateEmitInterval = time.Millisecond

// emitStateReceived 는 수신된 파싱/정규화 상태를 device_state_received 로 노드에 전달한다
// (SPEC-XSFM-AGENT-IO-001 RD-1/RD-5). forward_received_to_node 가 ON 일 때 ingestState 에서
// 매 수신마다(변경 여부·방출 모드와 무관) 호출되는 패스스루 탭이다.
//
// emitStateChanged 와 조립은 동일하되 (1) 타입이 device_state_received 이고 (2) changed_fields 를
// 싣지 않는다(패스스루는 "변경분" 개념이 없다). 단일 디바이스 파싱 상태 메시지이며 원시 브로커
// 바이트가 아니다 — direct·port 양 모드에서 정규화된 상태 형태로 방출된다.
func (a *XSFMAgent) emitStateReceived(deviceID, groupID string, online bool, stateAxes map[string]any, timestampMs int64, meta map[string]string) {
	data := map[string]any{
		"device_id": deviceID,
		"online":    online,
		"timestamp": timestampMs,
	}
	if groupID != "" {
		data["group_id"] = groupID
	}
	// 관측 게이팅된 상태 축(power/fan_speed/online)을 최상위에 병합한다 (emitStateChanged 동일 규약).
	for k, v := range stateAxes {
		data[k] = v
	}
	// 추출된 주소/attribute placeholder 를 메타로 병합한다: 하류 influx 태그
	// (station_code/place_code/device_index/attribute)를 나른다. device_id 는 최상위와 중복,
	// 상태 축과 충돌하는 키는 건너뛴다(emitStateChanged 동일 규약).
	for k, v := range meta {
		if k == placeholderDeviceID {
			continue
		}
		if _, exists := data[k]; exists {
			continue
		}
		data[k] = v
	}
	a.sendEvent("device_state_received", data)
}

// @MX:WARN: 주기 상태 방출기 goroutine — stopCh + monitorWG 로 생명주기를 관리한다.
// @MX:REASON: 전용 goroutine 이 ticker 로 주기 방출하므로 Stop 의 monitorWG.Wait 가 종료를
// 확인하지 못하면 goroutine/타이머가 누수된다. 채널 송신 중 로스터 락을 잡지 않아야 한다.
//
// startStateEmitter 는 interval/both 모드의 주기 스냅샷 방출기 goroutine 을 기동한다
// (SPEC-XSFM-AGENT-IO-001 RD-2, startOfflineMonitor 패턴 미러). state_emit_mode 가
// interval/both 가 아니거나 state_emit_interval<=0 이면 미기동한다(REQ-02-08, 3-way 비활성).
//
// ticker 주기는 state_emit_interval 을 그대로 쓴다(offline 모니터의 timeout/4 나눔과 달리,
// 여기서는 interval 이 곧 방출 주기이다). minStateEmitInterval 하한 가드로 time.NewTicker panic 을
// 방지한다. 고루틴은 stopCh 관측 시 즉시 종료하며 Stop 의 monitorWG.Wait 가 종료를 확인한다.
func (a *XSFMAgent) startStateEmitter() {
	if a.cfg.StateEmitInterval <= 0 {
		return
	}
	if a.cfg.StateEmitMode != stateEmitModeInterval && a.cfg.StateEmitMode != stateEmitModeBoth {
		return
	}
	interval := a.cfg.StateEmitInterval
	if interval < minStateEmitInterval {
		interval = minStateEmitInterval
	}

	a.monitorWG.Add(1)
	go func() {
		defer a.monitorWG.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-t.C:
				a.emitStateSnapshot()
			}
		}
	}()
}

// emitStateSnapshot 은 전체 등록 디바이스의 상태 풀 스냅샷을 device_state_snapshot 으로 방출한다
// (SPEC-XSFM-AGENT-IO-001 RD-2/RD-5). 단일 {timestamp, devices:[...]} 배열 메시지이며 각 항목은
// deviceStateJSON(request_state 전체 응답 shape 재사용)이다. offline 디바이스도 online:false 로
// 포함한다(include-all, REQ-02-05). 등록 디바이스가 0개여도 devices:[] 로 방출한다(heartbeat).
//
// 동시성: ListDevices 가 RLock 하에서 정렬 값 복사본을 반환하므로, 락을 채널 송신에 걸쳐 잡지
// 않는다(REQ-02-07, monitor.go 규율 계승 — 스냅샷 후 락 해제 상태에서 조립·송신).
func (a *XSFMAgent) emitStateSnapshot() {
	devs := a.ListDevices() // RLock 하 스냅샷, 반환 시 락 해제됨.
	out := make([]map[string]any, 0, len(devs))
	for i := range devs {
		out = append(out, deviceStateJSON(&devs[i]))
	}
	a.sendEvent("device_state_snapshot", map[string]any{
		"timestamp": time.Now().UnixMilli(),
		"devices":   out,
	})
}
