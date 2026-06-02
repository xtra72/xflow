package samsung

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/device/adapter"
)

// newHvacr01Executor creates a CommandExecutor that translates unified device
// commands into Hvacr01Agent.Process JSON requests.
func newHvacr01Executor(agent *Hvacr01Agent, addr NasaAddress) adapter.CommandExecutor {
	addrStr := addr.String() // "XX.XX.XX" format for ParseNasaAddress
	return func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		req := processRequest{
			Command: command,
			Address: addrStr,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("samsung_nasa executor: marshal request: %w", err)
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
			return nil, fmt.Errorf("samsung_nasa executor: unmarshal response: %w", err)
		}

		return result, nil
	}
}
