package lg

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

func TestRegisterLGLGAPTypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterLGLGAPTypes(mgr)
	if err != nil {
		t.Fatalf("RegisterLGLGAPTypes() error = %v", err)
	}
}

func TestRegisterLGLGAPTypes_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterLGLGAPTypes(mgr)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	err = RegisterLGLGAPTypes(mgr)
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}
