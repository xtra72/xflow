package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// command-set 노드 공통 헬퍼
// ---------------------------------------------------------------------------
// modbus-write / modbus-read / modbus-control 세 노드가 공유하는 유틸리티.
// 세 노드는 config(또는 payload)의 command_set(op 배열)을 순서대로 적용하고
// 결과 메시지를 하나만 emit한다. 살베지된 modbus_common.go 헬퍼를 재사용하며,
// 여기서는 command_set 파싱 / 응답 판정 / Server unit_id 주입만 담당한다.

var (
	// ErrModbusEmptyCommandSet 는 config/payload 어느 쪽에서도 command_set이
	// 제공되지 않았을 때 반환된다.
	ErrModbusEmptyCommandSet = fmt.Errorf("modbus: %w: command_set is empty", ErrInvalidConfig)
)

// ---------------------------------------------------------------------------
// modbusNodeBase 접근자 (control 노드 lifecycle / agent-type 분기용)
// ---------------------------------------------------------------------------

// underlyingModbusAgent 는 resolve된 원본 Agent를 잠금 하에 반환한다.
// control 노드의 lifecycle(Start/Stop/Pause/Resume) 직접 호출에 사용한다.
func (mb *modbusNodeBase) underlyingModbusAgent() agent.Agent {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.agent
}

// resolvedAgentType 는 감지된 agent 타입("server"/"client")을 반환한다.
// 아직 resolve되지 않았으면 빈 문자열이다.
func (mb *modbusNodeBase) resolvedAgentType() string {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.agentType
}

// ---------------------------------------------------------------------------
// command_set 파싱
// ---------------------------------------------------------------------------

// toOpList 는 command_set 값(any)을 op 맵 목록으로 변환한다.
// config는 보통 []any(각 원소가 map[string]any), 코드에서 직접 구성하면
// []map[string]any 로 전달될 수 있으므로 두 경우를 모두 처리한다.
func toOpList(v any) []map[string]any {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []map[string]any:
		return arr
	case []any:
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// valuesLen 은 values 값(보통 []any)의 길이를 반환한다. 배열이 아니면 0이다.
func valuesLen(values any) int {
	if arr, ok := values.([]any); ok {
		return len(arr)
	}
	return 0
}

// ---------------------------------------------------------------------------
// 오류 분류 / 응답 판정
// ---------------------------------------------------------------------------

// isHardModbusError 는 op 단위로 수집하지 않고 즉시 중단해야 하는
// 하드 트랜스포트 오류인지 판별한다 (타임아웃/취소/미해결 에이전트).
// 그 외의 에이전트 Process 오류(예: 잘못된 주소)는 op별 reason으로 수집한다.
func isHardModbusError(err error) bool {
	return errors.Is(err, ErrModbusNoResolver) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// injectServerUnitID 는 Server 명령 JSON의 params에 unit_id를 주입한다.
// 살베지된 빌더(buildModbusServerReadCommand/WriteCommand)는 unit_id를
// 포함하지 않으므로, unit_id(0=공유 컨테이너 포함)를 명시한 op에 대해
// 후처리로 주입하여 대상 컨테이너/디바이스를 선택한다.
func injectServerUnitID(cmdBytes []byte, unitID uint8) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(cmdBytes, &req); err != nil {
		return nil, err
	}
	params, ok := req["params"].(map[string]any)
	if !ok {
		params = map[string]any{}
		req["params"] = params
	}
	params["unit_id"] = unitID
	return json.Marshal(req)
}

// evalWriteResponse 는 쓰기/제어 op 응답에서 성공 여부와 실패 사유를 판별한다.
//   - Server 성공: {"ok": true}
//   - Client 성공: {"status": "ok"}
//   - 실패: {"error": ...} (+ 선택적 "exception_code") 또는 ok=false / status!="ok"
func evalWriteResponse(agentType string, respBytes []byte) (ok bool, reason string) {
	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return false, "invalid agent response: " + err.Error()
	}

	// 명시적 error 필드가 있으면 최우선으로 실패 사유로 사용한다.
	if e, has := resp["error"]; has {
		reason = toReasonString(e)
		if code, hasCode := resp["exception_code"]; hasCode {
			reason = fmt.Sprintf("%s (exception_code: %s)", reason, toReasonString(code))
		}
		return false, reason
	}

	switch agentType {
	case agentTypeServer:
		if v, has := resp["ok"]; has {
			if b, _ := v.(bool); b {
				return true, ""
			}
			return false, "server reported ok=false"
		}
		// ok 필드가 없고 error도 없으면 성공으로 간주한다 (get_status 등).
		return true, ""
	case agentTypeClient:
		if s, has := resp["status"]; has {
			if str, _ := s.(string); str == "ok" || str == "reconfigured" {
				return true, ""
			}
			return false, "client status: " + toReasonString(resp["status"])
		}
		return true, ""
	}
	return true, ""
}

// toReasonString 은 응답 값(문자열/기타)을 사람이 읽을 수 있는 사유 문자열로 변환한다.
func toReasonString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
