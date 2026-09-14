package lg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/device/adapter"
)

// newHvacr03Executor 는 Hvacr03Agent.Process 를 CommandExecutor 로 브릿지한다.
// address 는 제어 대상 실내기의 주소 (10진 문자열, 예: "3") 이다.
func newHvacr03Executor(a *Hvacr03Agent, address string) adapter.CommandExecutor {
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := hvacr03ProcessRequest{
			Command: command,
			Address: address,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("lg_hvacr03 executor: marshal request: %w", err)
		}

		resp, err := a.Process(data)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, nil
		}

		var result map[string]any
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("lg_hvacr03 executor: unmarshal response: %w", err)
		}
		return result, nil
	}
}
