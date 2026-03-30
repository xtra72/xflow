package samsung

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/device/adapter"
)

// newNASAExecutor creates a CommandExecutor that translates unified device
// commands into NASAAgent.Process JSON requests.
func newNASAExecutor(agent *NASAAgent, addr NASAAddress) adapter.CommandExecutor {
	addrStr := addr.String() // "XX.XX.XX" format for ParseNASAAddress
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := processRequest{
			Command: command,
			Address: addrStr,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("nasa executor: marshal request: %w", err)
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
			return nil, fmt.Errorf("nasa executor: unmarshal response: %w", err)
		}

		return result, nil
	}
}
