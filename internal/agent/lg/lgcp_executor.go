package lg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/device/adapter"
)

// newLGCPExecutor 는 LGCP 에이전트의 Process 메서드를 CommandExecutor 로 브릿지한다.
// address 는 제어 대상 실내기의 HEX 주소 (예: "12345678") 이다.
func newLGCPExecutor(agent *LGCPAgent, address string) adapter.CommandExecutor {
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := lgcpProcessRequest{
			Command: command,
			Address: address,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("lgcp executor: marshal request: %w", err)
		}

		resp, err := agent.Process(data)
		if err != nil {
			return nil, err
		}

		if resp == nil {
			return nil, nil
		}

		var result map[string]any
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("lgcp executor: unmarshal response: %w", err)
		}

		return result, nil
	}
}
