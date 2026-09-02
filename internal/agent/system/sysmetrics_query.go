package system

// 읽기 전용 커맨드 처리.
//
// `POST /agents/{id}/query` 로 들어오는 `get_history` 하나를 받는다. 전용 라우트를 새로
// 내지 않는 이유는 그 엔드포인트가 이미 화이트리스트 + `agent.read` 권한 모델을 갖고
// 있기 때문이다 — 라우트를 늘리면 권한 판정이 두 곳으로 갈린다.

import (
	"encoding/json"
	"fmt"
)

// sysMetricsRequest 는 커맨드 요청이다(다른 에이전트와 같은 형상).
type sysMetricsRequest struct {
	Command string         `json:"command"`
	Params  map[string]any `json:"params,omitempty"`
}

// Process 는 읽기 전용 커맨드를 처리한다.
//
// 상태를 바꾸는 커맨드는 두지 않는다 — 이 에이전트가 하는 일은 관측뿐이고, 쓰기
// 커맨드가 생기면 `queryReadOnlyCommands` 화이트리스트의 뜻이 흐려진다.
func (a *SysMetricsAgent) Process(data []byte) ([]byte, error) {
	var req sysMetricsRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("sysmetrics process: 잘못된 JSON: %w", err)
	}

	switch req.Command {
	case "get_history":
		return a.processGetHistory(&req)
	default:
		return nil, fmt.Errorf("sysmetrics: 알 수 없는 커맨드: %s", req.Command)
	}
}

// processGetHistory 는 이력 구간을 돌려준다.
//
// 파라미터(둘 다 선택):
//
//	start_ms  구간 시작(epoch ms). 0/미지정이면 버퍼의 처음부터.
//	end_ms    구간 끝(epoch ms, exclusive). 0/미지정이면 버퍼의 끝까지.
//
// 경계를 선택으로 둔 이유: 패널은 언제나 구간을 주지만, 진단 목적으로 버퍼 전체를
// 한 번에 보고 싶은 경우가 있다. 그때 없는 값을 억지로 만들게 하지 않는다.
func (a *SysMetricsAgent) processGetHistory(req *sysMetricsRequest) ([]byte, error) {
	start := int64Param(req.Params, "start_ms")
	end := int64Param(req.Params, "end_ms")

	a.mu.RLock()
	result := a.history.query(start, end, a.cfg.History)
	a.mu.RUnlock()

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("sysmetrics: 이력 직렬화 실패: %w", err)
	}
	return out, nil
}

// int64Param 은 파라미터에서 정수를 읽는다. 없거나 수가 아니면 0 이다.
//
// JSON 역직렬화는 수를 float64 로 주고, Go 호출부는 int64 를 준다. 둘 다 받는다.
func int64Param(params map[string]any, key string) int64 {
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}
