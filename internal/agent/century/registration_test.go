package century

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

func TestRegisterCenturyTypes_Succeeds(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterCenturyTypes(mgr); err != nil {
		t.Fatalf("RegisterCenturyTypes returned error: %v", err)
	}
}

func TestRegisterCenturyTypes_DuplicateFails(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterCenturyTypes(mgr); err != nil {
		t.Fatalf("first RegisterCenturyTypes returned error: %v", err)
	}
	if err := RegisterCenturyTypes(mgr); err == nil {
		t.Fatal("duplicate RegisterCenturyTypes succeeded, want error")
	}
}

func TestRegisterCenturyTypes_FactoryProducesAgent(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterCenturyTypes(mgr); err != nil {
		t.Fatalf("RegisterCenturyTypes returned error: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:   "test-century-1",
		Name: "Test Century",
		Type: "century-hvac",
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
	if a.Type() != "century-hvac" {
		t.Errorf("agent.Type() = %q, want century-hvac", a.Type())
	}
}

func TestRegisterCenturyTypes_RejectsMissingSerialPort(t *testing.T) {
	t.Parallel()
	mgr := agent.NewManager()
	if err := RegisterCenturyTypes(mgr); err != nil {
		t.Fatalf("RegisterCenturyTypes returned error: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:   "test-century-2",
		Name: "Test Century",
		Type: "century-hvac",
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

func TestRegisterCenturyTypes_OnlyCenturyHvacName(t *testing.T) {
	t.Parallel()
	// AC-D1: the registered type name MUST be "century-hvac" — not "century" or anything else.
	mgr := agent.NewManager()
	if err := RegisterCenturyTypes(mgr); err != nil {
		t.Fatalf("RegisterCenturyTypes returned error: %v", err)
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
		t.Errorf(`mgr.Create with type "century" succeeded; want failure (only "century-hvac" must be registered)`)
	}
}
