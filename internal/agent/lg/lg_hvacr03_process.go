package lg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 명령 인터페이스
//
// SPEC-LG-HVACR-003 § M7 / § 4.6. 요청 DTO 와 조회·관리 명령 9종은 lg_hvacr02 와
// 동일하게 유지하여 기존 노드·REST·제어 패널이 변경 없이 동작하게 한다.
// ---------------------------------------------------------------------------

// hvacr03ProcessRequest 는 Process 메서드의 JSON 요청 구조체이다.
// 필드 구성은 lg_hvacr02 의 hvacr02ProcessRequest 와 동일하다.
type hvacr03ProcessRequest struct {
	Command  string         `json:"command"`
	Count    int            `json:"count,omitempty"`
	Address  string         `json:"address,omitempty"`   // 실내기 주소 (10진 문자열)
	DeviceID string         `json:"device_id,omitempty"` // 디바이스 UUID
	Params   map[string]any `json:"params,omitempty"`
	NodeID   string         `json:"node_id,omitempty"`
	FlowID   string         `json:"flow_id,omitempty"`
	LastSeq  int64          `json:"last_seq,omitempty"`
}

// Process 는 JSON 명령을 처리한다.
func (a *Hvacr03Agent) Process(data []byte) ([]byte, error) {
	var req hvacr03ProcessRequest
	if err := json.Unmarshal(data, &req); err != nil {
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("lg_hvacr03 process: invalid request: %w", err)
	}

	var result []byte
	var err error

	switch req.Command {
	case "get_stats":
		result, err = a.processGetStats()
	case "get_recent":
		// count > 0: 최근 count 개 (비파괴) / count == 0: drain (파괴적)
		count := req.Count
		if count == 0 {
			result, err = a.processDrain(hvacr03RecentBufferSize, req.NodeID, req.FlowID)
		} else {
			if count < 0 {
				count = 10
			}
			result, err = a.processGetRecent(count, req.LastSeq, req.NodeID, req.FlowID)
		}
	case "drain":
		count := req.Count
		if count <= 0 {
			count = hvacr03RecentBufferSize
		}
		result, err = a.processDrain(count, req.NodeID, req.FlowID)
	case "get_all":
		result, err = a.processGetAll()
	case "request_state":
		// 노드의 inactivity-fallback 요청. 각 디바이스의 마지막 상태를
		// trigger="response" 로 push 경로에 방출한다.
		emitted := a.emitAllDeviceStates("response")
		result, err = json.Marshal(map[string]any{"status": "ok", "emitted": emitted})
	case "get_state":
		result, err = a.processGetState(&req)
	case "list_devices":
		result, err = a.processListDevices()
	case "remove_device":
		result, err = a.processRemoveDevice(&req)
	case "set_device":
		result, err = a.processSetDevice(&req)
	case "set_power", "set_mode", "set_fan_speed", "target_temperature", "set_multiple",
		"set_swing", "clear_filter_alarm", "set_lock", "set_temp_limit",
		"set_erv_mode", "set_erv_rapid", "set_erv_eco":
		result, err = a.processControlCommand(&req)
	default:
		a.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("lg_hvacr03: unsupported command %q", req.Command)
	}

	if err != nil {
		a.stats.IncrMessagesErrored()
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 조회 명령
// ---------------------------------------------------------------------------

// processGetStats 는 폴링·제어 통계를 반환한다.
func (a *Hvacr03Agent) processGetStats() ([]byte, error) {
	stats := a.statsExtra()
	a.reconnectMu.Lock()
	stats["reconnecting"] = a.isReconnecting
	stats["reconnect_attempts"] = a.reconnectAttempts
	a.reconnectMu.Unlock()
	return json.Marshal(stats)
}

// processGetState 는 단일 디바이스의 상태를 반환한다.
func (a *Hvacr03Agent) processGetState(req *hvacr03ProcessRequest) ([]byte, error) {
	if req.Address == "" {
		return json.Marshal(map[string]any{"status": "error", "error": "missing address"})
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	dev, ok := a.devices[req.Address]
	if !ok {
		return json.Marshal(map[string]any{"status": "not_found", "address": req.Address})
	}
	return json.Marshal(map[string]any{
		"status": "ok",
		"device": a.deviceDTOLocked(dev),
	})
}

// processGetAll 은 모든 디바이스의 즉시 스냅샷을 반환한다.
func (a *Hvacr03Agent) processGetAll() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	devices := make([]map[string]any, 0, len(a.devices))
	for _, dev := range a.devices {
		devices = append(devices, a.deviceDTOLocked(dev))
	}
	return json.Marshal(map[string]any{"status": "ok", "devices": devices})
}

// processGetRecent 는 최근 이벤트 중 lastSeq 이후의 새 이벤트만 반환한다.
func (a *Hvacr03Agent) processGetRecent(count int, lastSeq int64, nodeID, flowID string) ([]byte, error) {
	a.recentMu.RLock()
	defer a.recentMu.RUnlock()

	total := hvacr03RecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}
	if count > total {
		count = total
	}

	// 최신 항목부터 역순 추출 + lastSeq 필터링.
	// newestSeq 는 필터와 무관하게 링의 현재 최신 seq 이다. 이 값을 응답의 last_seq 로
	// 돌려주지 않으면 노드의 lastSeq 가 0 에 머물러 매 조회마다 백로그가 통째로
	// 재전송된다 (중복 발행의 근본 원인).
	result := make([]json.RawMessage, 0, count)
	var newestSeq int64
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + hvacr03RecentBufferSize) % hvacr03RecentBufferSize
		rec := a.recentEvents[idx]
		if i == 0 {
			newestSeq = rec.Seq
		}
		if lastSeq > 0 && rec.Seq <= lastSeq {
			break // seq 는 단조 증가 — 이후는 전부 이전 이벤트
		}
		result = append(result, rec.Event)
	}

	if n := int64(len(result)); n > 0 {
		a.stats.AddInternalMessagesSent(n)
		if nodeID != "" {
			a.stats.IncrNodeRefSent(nodeID, flowID)
		}
	}

	respLastSeq := newestSeq
	if respLastSeq < lastSeq {
		respLastSeq = lastSeq // 노드 lastSeq 가 후퇴하지 않게 에코한다
	}
	return json.Marshal(map[string]any{
		"count":    len(result),
		"frames":   result,
		"last_seq": respLastSeq,
	})
}

// processDrain 은 링 버퍼에서 이벤트를 소비하고 버퍼를 리셋한다.
func (a *Hvacr03Agent) processDrain(count int, nodeID, flowID string) ([]byte, error) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	total := hvacr03RecentBufferSize
	if !a.recentFull {
		total = a.recentIdx
	}
	if count > total {
		count = total
	}

	result := make([]json.RawMessage, 0, count)
	for i := 0; i < count; i++ {
		idx := (a.recentIdx - 1 - i + hvacr03RecentBufferSize) % hvacr03RecentBufferSize
		result = append(result, a.recentEvents[idx].Event)
	}

	a.recentIdx = 0
	a.recentFull = false

	if n := int64(len(result)); n > 0 {
		a.stats.AddInternalMessagesSent(n)
		if nodeID != "" {
			for range result {
				a.stats.IncrNodeRefSent(nodeID, flowID)
			}
		}
	}

	return json.Marshal(map[string]any{"count": len(result), "frames": result})
}

// ---------------------------------------------------------------------------
// 디바이스 관리 명령
// ---------------------------------------------------------------------------

// processListDevices 는 디바이스 목록을 report_enabled 포함으로 반환한다.
func (a *Hvacr03Agent) processListDevices() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	devices := make([]map[string]any, 0, len(a.devices))
	for _, dev := range a.devices {
		devices = append(devices, map[string]any{
			"address":        dev.Address,
			"device_id":      agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.Address),
			"device_type":    dev.Type,
			"online":         dev.Online,
			"source":         dev.Source,
			"report_enabled": dev.ReportEnabled,
		})
	}
	return json.Marshal(map[string]any{"status": "ok", "devices": devices})
}

// processRemoveDevice 는 In-memory 디바이스를 삭제한다.
// 삭제해도 스캔에서 다시 발견되면 자동 재등록되므로 source 구분 없이 허용한다.
func (a *Hvacr03Agent) processRemoveDevice(req *hvacr03ProcessRequest) ([]byte, error) {
	fillAddressFromParams(req)

	addr, dev, err := a.resolveDeviceMgmt(req)
	if err != nil {
		return nil, err
	}
	label := dev.Label

	a.mu.Lock()
	delete(a.devices, addr)
	delete(a.lastEmitted, addr)
	v2 := a.onDeviceStateChangeV2
	agentName := a.agentConfig.Name
	a.mu.Unlock()

	// UI 재조회 유도 (목록에서 사라지도록). 락 밖에서 호출한다.
	a.notifyDeviceChanges(v2, agentName, []string{addr})

	return json.Marshal(map[string]any{
		"status":    "ok",
		"address":   addr,
		"unit_id":   addr,
		"device_id": agent.ResolveDeviceID(context.Background(), agentName, addr),
		"name":      label,
	})
}

// processSetDevice 는 디바이스별 report_enabled / name 을 갱신한다.
func (a *Hvacr03Agent) processSetDevice(req *hvacr03ProcessRequest) ([]byte, error) {
	fillAddressFromParams(req)

	addr, dev, err := a.resolveDeviceMgmt(req)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if v, ok := req.Params["report_enabled"].(bool); ok {
		dev.ReportEnabled = v
	}
	if v, ok := req.Params["name"].(string); ok {
		dev.Label = v
	}
	resp := map[string]any{
		"status":         "ok",
		"address":        addr,
		"unit_id":        addr,
		"device_id":      agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr),
		"name":           dev.Label,
		"device_type":    dev.Type,
		"report_enabled": dev.ReportEnabled,
	}
	a.mu.Unlock()

	return json.Marshal(resp)
}

// fillAddressFromParams 는 API exec DTO 폴백이다 — params 안의 address/device_id 도
// 최상위 필드로 승격한다.
func fillAddressFromParams(req *hvacr03ProcessRequest) {
	if req.Address == "" {
		if v, ok := req.Params["address"].(string); ok {
			req.Address = v
		}
	}
	if req.DeviceID == "" {
		if v, ok := req.Params["device_id"].(string); ok {
			req.DeviceID = v
		}
	}
}
