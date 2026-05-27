package samsung

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

func TestRegisterSamsungHvacr01Types(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSamsungHvacr01Types(mgr)
	if err != nil {
		t.Fatalf("RegisterSamsungHvacr01Types() error = %v", err)
	}
}

func TestRegisterSamsungHvacr01Types_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSamsungHvacr01Types(mgr)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	err = RegisterSamsungHvacr01Types(mgr)
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}
