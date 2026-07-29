package xsfm

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// TestRegisterXSFMTypes 는 타입 등록이 성공하는지 확인한다 (Module 7.1).
func TestRegisterXSFMTypes(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterXSFMTypes(mgr); err != nil {
		t.Fatalf("RegisterXSFMTypes() error = %v", err)
	}
}

// TestRegisterXSFMTypes_Duplicate 는 중복 등록이 에러를 반환하는지 확인한다.
func TestRegisterXSFMTypes_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterXSFMTypes(mgr); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := RegisterXSFMTypes(mgr); err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

// TestRegisterXSFMTypes_CreatesAgent 는 등록 후 Manager.Create 가
// "xsfm" 타입으로 *XSFMAgent 를 생성하는지 확인한다 (Module 7.1).
func TestRegisterXSFMTypes_CreatesAgent(t *testing.T) {
	mgr := agent.NewManager()
	if err := RegisterXSFMTypes(mgr); err != nil {
		t.Fatalf("RegisterXSFMTypes() error = %v", err)
	}

	created, err := mgr.Create(baseAgentConfig(directOpts()))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	ap, ok := created.(*XSFMAgent)
	if !ok {
		t.Fatalf("Create() returned %T, want *XSFMAgent", created)
	}
	if ap.Type() != agentType {
		t.Fatalf("agent Type() = %q, want %q", ap.Type(), agentType)
	}

	// DeviceProvider 표면이 노출되어 OnStart 훅이 자동 등록할 수 있는지 확인한다.
	if ap.DeviceProvider() == nil {
		t.Fatal("DeviceProvider() returned nil")
	}
}
