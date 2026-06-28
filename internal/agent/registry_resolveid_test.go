package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockAgent 는 ID()/Name() 가 자기 RWMutex 를 RLock 하는 에이전트를 모사한다
// (HVAC 에이전트와 동일 패턴 — 예: samsung Hvacr01Agent). 이를 통해
// "에이전트가 자기 락을 보유한 컨텍스트에서 이름→ID 해석을 호출할 때" 의 재귀
// 락 deadlock 을 재현/검증한다.
type lockAgent struct {
	*BaseAgent
	amu  sync.RWMutex
	id   string
	name string
}

func (a *lockAgent) ID() string {
	a.amu.RLock()
	defer a.amu.RUnlock()
	return a.id
}

func (a *lockAgent) Name() string {
	a.amu.RLock()
	defer a.amu.RUnlock()
	return a.name
}

func newLockAgent(t *testing.T, id, name string) *lockAgent {
	t.Helper()
	ba := NewBaseAgent()
	require.NoError(t, ba.Init(AgentConfig{ID: id, Name: name, Type: "lock-test"}))
	return &lockAgent{BaseAgent: ba, id: id, name: name}
}

// TestRegistry_ResolveID_NoAgentMethodCalls 는 ResolveID 가 에이전트 메서드
// (Name()/ID())를 호출하지 않음을 검증한다. 에이전트의 write 락을 보유한 상태에서
// ResolveID 를 호출해도 즉시 반환해야 한다 — 만약 내부에서 a.Name()/a.ID() 를
// 호출하면 보유 중인 write 락 때문에 RLock 이 막혀 블록(=프로덕션의 재귀 RLock
// deadlock)된다.
func TestRegistry_ResolveID_NoAgentMethodCalls(t *testing.T) {
	reg := NewRegistry()
	a := newLockAgent(t, "id-1", "Samsung HVACR")
	require.NoError(t, reg.Register(a)) // 등록 시점엔 락 미보유 → 안전

	// 에이전트의 write 락을 보유한다 (HVAC 메시지 처리 중 상태 모사).
	a.amu.Lock()
	defer a.amu.Unlock()

	done := make(chan string, 2)
	go func() { id, _ := reg.ResolveID("Samsung HVACR"); done <- id }() // 이름으로
	go func() { id, _ := reg.ResolveID("id-1"); done <- id }()          // ID 로

	for i := 0; i < 2; i++ {
		select {
		case id := <-done:
			assert.Equal(t, "id-1", id)
		case <-time.After(2 * time.Second):
			t.Fatal("ResolveID 가 에이전트 락 때문에 블록됨 — 재귀 RLock deadlock 회귀")
		}
	}
}

// TestRegistry_ResolveID_Lookup 는 ResolveID 의 기본 해석(ID/이름/미등록)을 검증한다.
func TestRegistry_ResolveID_Lookup(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(createTestAgent(t, "id-x", "Agent X", "custom")))

	if id, ok := reg.ResolveID("id-x"); !ok || id != "id-x" {
		t.Errorf("ID 해석 실패: got (%q,%v)", id, ok)
	}
	if id, ok := reg.ResolveID("Agent X"); !ok || id != "id-x" {
		t.Errorf("이름 해석 실패: got (%q,%v)", id, ok)
	}
	if _, ok := reg.ResolveID("없는것"); ok {
		t.Error("미등록 ref 는 false 여야 함")
	}
}

// TestRegistry_ResolveID_UnregisterClearsName 은 Unregister 후 이름 인덱스가
// 정리되는지 검증한다.
func TestRegistry_ResolveID_UnregisterClearsName(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(createTestAgent(t, "id-y", "Agent Y", "custom")))
	require.NoError(t, reg.Unregister("id-y"))

	if _, ok := reg.ResolveID("Agent Y"); ok {
		t.Error("Unregister 후 이름 해석은 false 여야 함")
	}
	if _, ok := reg.ResolveID("id-y"); ok {
		t.Error("Unregister 후 ID 해석은 false 여야 함")
	}
}
