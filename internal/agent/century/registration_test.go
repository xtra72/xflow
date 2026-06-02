package century

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

func TestRegisterHvacr01Types_Succeeds(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterHvacr01Types(mgr); err != nil {
		t.Fatalf("RegisterHvacr01Types returned error: %v", err)
	}
}

func TestRegisterHvacr01Types_DuplicateFails(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterHvacr01Types(mgr); err != nil {
		t.Fatalf("first RegisterHvacr01Types returned error: %v", err)
	}
	if err := RegisterHvacr01Types(mgr); err == nil {
		t.Fatal("duplicate RegisterHvacr01Types succeeded, want error")
	}
}

func TestRegisterHvacr01Types_FactoryProducesAgent(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterHvacr01Types(mgr); err != nil {
		t.Fatalf("RegisterHvacr01Types returned error: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:   "test-century-1",
		Name: "Test Century",
		Type: "century_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/ttyTEST",
			},
		},
	}
	a, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("mgr.Create returned error: %v", err)
	}
	if a == nil {
		t.Fatal("mgr.Create returned nil agent")
	}
	if a.Type() != "century_hvacr01" {
		t.Errorf("agent.Type() = %q, want century_hvacr01", a.Type())
	}
}

func TestRegisterHvacr01Types_RejectsMissingSerialPort(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterHvacr01Types(mgr); err != nil {
		t.Fatalf("RegisterHvacr01Types returned error: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:   "test-century-2",
		Name: "Test Century",
		Type: "century_hvacr01",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{}, // missing serial_port
		},
	}
	_, err := mgr.Create(cfg)
	if err == nil {
		t.Fatal("mgr.Create returned no error, want serial_port-required error")
	}
}

func TestRegisterHvacr01Types_OnlyCenturyHvacName(t *testing.T) {
	t.Parallel()
	// AC-D1: the registered type name MUST be "century_hvacr01" — not "century" or anything else.
	mgr := agent.NewManager()
	if err := RegisterHvacr01Types(mgr); err != nil {
		t.Fatalf("RegisterHvacr01Types returned error: %v", err)
	}
	// Attempting to create with an unrelated type name should fail.
	wrongCfg := agent.AgentConfig{
		ID:   "wrong-type",
		Name: "Wrong",
		Type: "century",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{"serial_port": "/dev/ttyTEST"},
		},
	}
	if _, err := mgr.Create(wrongCfg); err == nil {
		t.Errorf(`mgr.Create with type "century" succeeded; want failure (only "century_hvacr01" must be registered)`)
	}
}
