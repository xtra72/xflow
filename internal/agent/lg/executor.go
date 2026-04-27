package lg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/device/adapter"
)

// newLGAPExecutor creates a CommandExecutor that translates unified device
// commands into LGAPAgent.Process JSON requests.
func newLGAPExecutor(agent *LGAPAgent, zone byte) adapter.CommandExecutor {
	zoneInt := int(zone)
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := processRequest{
			Command: command,
			Zone:    &zoneInt,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("lgap executor: marshal request: %w", err)
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
			return nil, fmt.Errorf("lgap executor: unmarshal response: %w", err)
		}

		return result, nil
	}
}
