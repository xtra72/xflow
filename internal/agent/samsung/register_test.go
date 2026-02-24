package samsung

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

func TestRegisterSamsungNASATypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSamsungNASATypes(mgr)
	if err != nil {
		t.Fatalf("RegisterSamsungNASATypes() error = %v", err)
	}
}

func TestRegisterSamsungNASATypes_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSamsungNASATypes(mgr)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	err = RegisterSamsungNASATypes(mgr)
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}
