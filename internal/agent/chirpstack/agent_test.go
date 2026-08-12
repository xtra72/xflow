package chirpstack

import (
	"context"
	"errors"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// newTestConfig 는 테스트용 비활성화(연결 없음) ChirpStack 에이전트 설정을 만든다.
// Enabled=false 이면 Init 이 브로커에 연결하지 않아 단위 테스트가 실제 MQTT
// 브로커 없이도 결정적으로 동작한다.
func newTestConfig(id, name string) agent.AgentConfig {
	disabled := false
	return agent.AgentConfig{
		ID:      id,
		Name:    name,
		Type:    "chirpstack",
		Enabled: &disabled,
	}
}

// TestNewChirpStackAgent_ImplementsAgent 는 ChirpStackAgent 가 agent.Agent 인터페이스를
// 구현하고 기본 식별 메서드가 설정을 반영하는지 검증한다 (REQ-M1-01/03).
func TestNewChirpStackAgent_ImplementsAgent(t *testing.T) {
	resetNameRegistryForTest()

	a, err := NewChirpStackAgent(newTestConfig("id-1", "cs-1"))
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	var _ agent.Agent = a

	if got := a.Type(); got != "chirpstack" {
		t.Errorf("Type() = %q, want %q", got, "chirpstack")
	}
	if got := a.Name(); got != "cs-1" {
		t.Errorf("Name() = %q, want %q", got, "cs-1")
	}
	if got := a.ID(); got != "id-1" {
		t.Errorf("ID() = %q, want %q", got, "id-1")
	}
}

// TestChirpStackAgent_NameUniqueness 는 이미 등록된 이름과 충돌하는 다른 ID 의
// 에이전트 생성이 조용히 덮어쓰지 않고 거부되는지 검증한다 (REQ-M1-04, AC-7).
func TestChirpStackAgent_NameUniqueness(t *testing.T) {
	resetNameRegistryForTest()

	if _, err := NewChirpStackAgent(newTestConfig("id-1", "dup")); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := NewChirpStackAgent(newTestConfig("id-2", "dup"))
	if err == nil {
		t.Fatal("expected name collision error, got nil")
	}
	if !errors.Is(err, ErrNameCollision) {
		t.Errorf("error = %v, want ErrNameCollision", err)
	}
}

// TestChirpStackAgent_SameIDReclaimIdempotent 는 동일 (name,id) 재생성(재시작 경로)이
// 충돌로 거부되지 않음을 검증한다.
func TestChirpStackAgent_SameIDReclaimIdempotent(t *testing.T) {
	resetNameRegistryForTest()

	if _, err := NewChirpStackAgent(newTestConfig("id-1", "cs")); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := NewChirpStackAgent(newTestConfig("id-1", "cs")); err != nil {
		t.Errorf("same (name,id) reclaim should be idempotent, got %v", err)
	}
}

// TestRegisterChirpStackTypes_ManagerCreate 는 타입 등록 후 매니저가 chirpstack
// 에이전트를 인스턴스화할 수 있는지 검증한다 (REQ-M1-01/02, AC-8).
func TestRegisterChirpStackTypes_ManagerCreate(t *testing.T) {
	resetNameRegistryForTest()

	mgr := agent.NewManager()
	if err := RegisterChirpStackTypes(mgr); err != nil {
		t.Fatalf("RegisterChirpStackTypes: %v", err)
	}

	a, err := mgr.Create(newTestConfig("cs-mgr-id", "cs-mgr"))
	if err != nil {
		t.Fatalf("manager Create(chirpstack): %v", err)
	}
	if a.Type() != "chirpstack" {
		t.Errorf("Type() = %q, want chirpstack", a.Type())
	}
	_ = context.Background()
}
