package airpurifier

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// TestRegisterAirPurifierTypes 는 타입 등록이 성공하는지 확인한다 (Module 7.1).
func TestRegisterAirPurifierTypes(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterAirPurifierTypes(mgr); err != nil {
		t.Fatalf("RegisterAirPurifierTypes() error = %v", err)
	}
}

// TestRegisterAirPurifierTypes_Duplicate 는 중복 등록이 에러를 반환하는지 확인한다.
func TestRegisterAirPurifierTypes_Duplicate(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterAirPurifierTypes(mgr); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := RegisterAirPurifierTypes(mgr); err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

// TestRegisterAirPurifierTypes_CreatesAgent 는 등록 후 Manager.Create 가
// "airpurifier" 타입으로 *AirPurifierAgent 를 생성하는지 확인한다 (Module 7.1).
func TestRegisterAirPurifierTypes_CreatesAgent(t *testing.T) {
	mgr := agent.NewManager()
	if err := RegisterAirPurifierTypes(mgr); err != nil {
		t.Fatalf("RegisterAirPurifierTypes() error = %v", err)
	}

	created, err := mgr.Create(baseAgentConfig(directOpts()))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	ap, ok := created.(*AirPurifierAgent)
	if !ok {
		t.Fatalf("Create() returned %T, want *AirPurifierAgent", created)
	}
	if ap.Type() != agentType {
		t.Fatalf("agent Type() = %q, want %q", ap.Type(), agentType)
	}

	// DeviceProvider 표면이 노출되어 OnStart 훅이 자동 등록할 수 있는지 확인한다.
	if ap.DeviceProvider() == nil {
		t.Fatal("DeviceProvider() returned nil")
	}
}
