package node

import (
	"context"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/lg"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// SPEC-LG-HVACR-003 — lg_hvacr03 에이전트의 노드 재사용 검증
//
// lg_hvacr03 은 lg_hvacr02 와 명령 집합·device_state 형식이 같아 같은 노드
// 구현을 공유한다. 이 파일은 그 공유가 실제로 성립하는지를 고정한다.
// ---------------------------------------------------------------------------

// newHvacr03TestAgent 는 노드 타입 체크에 쓸 실제 Hvacr03Agent 를 만든다.
// 트랜스포트는 열지 않으므로 Process 호출은 하지 않고, 타입 판별만 검증한다.
func newHvacr03TestAgent(t *testing.T) agent.Agent {
	t.Helper()
	a, err := lg.NewHvacr03Agent(agent.AgentConfig{
		ID:   "node-test-hvacr03",
		Name: "node-test-hvacr03",
		Type: "lg_hvacr03",
		Transport: agent.TransportConfig{Options: map[string]any{
			"transport_type": "rtu",
			"serial_port":    "/dev/null",
		}},
	})
	if err != nil {
		t.Fatalf("NewHvacr03Agent: %v", err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background()) })
	return a
}

// TestLGHvacr03_NodeAcceptsHvacr03Agent 는 세 노드가 모두 lg_hvacr03 에이전트를
// 받아들이는지 확인한다. 타입 체크가 lg_hvacr02 전용으로 되돌아가면 실패한다.
func TestLGHvacr03_NodeAcceptsHvacr03Agent(t *testing.T) {
	cases := []struct {
		name     string
		nodeType string
		factory  func(flow.NodeDef, ...NodeOption) (Node, error)
	}{
		{"status", "lg_hvacr03_status", NewLGHvacr02StatusNode},
		{"control", "lg_hvacr03_control", NewLGHvacr02ControlNode},
		{"combined", "lg_hvacr03", NewLGHvacr02Node},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newHvacr03TestAgent(t)
			resolver := &mockLGHvacr02Resolver{
				transport: &mockLGHvacr02Transport{agent: a},
			}

			def := newLGHvacr02NodeDef("test-"+tc.name, tc.nodeType)
			n, err := tc.factory(def, WithAgentResolver(resolver))
			if err != nil {
				t.Fatalf("factory: %v", err)
			}
			if err := n.Configure(map[string]any{"agent_ref": "node-test-hvacr03"}); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			if err := n.Init(context.Background()); err != nil {
				t.Fatalf("Init rejected the lg_hvacr03 agent: %v", err)
			}
		})
	}
}

// TestLGHvacr03_NodeStillRejectsForeignAgent 는 무관한 에이전트가 여전히
// 거부되는지 확인한다. 타입 체크를 넓히면서 가드가 사라지지 않았음을 고정한다.
func TestLGHvacr03_NodeStillRejectsForeignAgent(t *testing.T) {
	foreign := &slowLGHvacr02Agent{}
	resolver := &mockLGHvacr02Resolver{
		transport: &mockLGHvacr02Transport{agent: foreign},
	}

	def := newLGHvacr02NodeDef("test-foreign", "lg_hvacr03_status")
	n, err := NewLGHvacr02StatusNode(def, WithAgentResolver(resolver))
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := n.Configure(map[string]any{"agent_ref": "foreign"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err == nil {
		t.Fatal("Init should reject an agent that is neither hvacr02 nor hvacr03")
	}
}

// TestLGHvacr03_NodeTypesRegistered 는 노드 타입 3종과 `_` 별칭이 모두 등록
// 되었는지 확인한다.
func TestLGHvacr03_NodeTypesRegistered(t *testing.T) {
	r := NewRegistry()

	canonical := []string{"lg-hvacr03-status", "lg-hvacr03-control", "lg-hvacr03"}
	aliases := []string{"lg_hvacr03_status", "lg_hvacr03_control", "lg_hvacr03"}

	for _, typ := range append(append([]string{}, canonical...), aliases...) {
		def := newLGHvacr02NodeDef("t", typ)
		if _, err := r.Create(def); err != nil {
			t.Errorf("node type %q is not registered: %v", typ, err)
		}
	}
}

// ---------------------------------------------------------------------------
// unit_id 어드레싱 필터 (10진 / 16진 공용)
// ---------------------------------------------------------------------------

// TestLGHvacr02MatchAddressing_DecimalUnitID 는 lg_hvacr03 의 10진 주소가
// 필터에서 올바르게 매칭되는지 확인한다.
//
// 보정 전에는 parseHexByte 가 "10" 을 0x10(16) 으로 읽어 N=10~15 가 조용히
// 빗나갔다. 특히 "15" 는 0x15(21) 로 읽혀 어떤 실내기와도 매칭되지 않았다.
func TestLGHvacr02MatchAddressing_DecimalUnitID(t *testing.T) {
	for _, addr := range []string{"0", "3", "9", "10", "12", "15"} {
		cfg := LGHvacr02NodeConfig{UnitID: addr}
		payload := map[string]any{"unit_id": addr}
		if !lgHvacr02MatchAddressing(payload, cfg) {
			t.Errorf("decimal unit_id %q should match itself", addr)
		}
	}
}

// TestLGHvacr02MatchAddressing_DecimalMismatch 는 다른 주소가 걸러지는지 확인한다.
func TestLGHvacr02MatchAddressing_DecimalMismatch(t *testing.T) {
	cfg := LGHvacr02NodeConfig{UnitID: "3"}
	for _, other := range []string{"0", "4", "13", "15"} {
		payload := map[string]any{"unit_id": other}
		if lgHvacr02MatchAddressing(payload, cfg) {
			t.Errorf("unit_id %q should not match filter %q", other, cfg.UnitID)
		}
	}
}

// TestLGHvacr02MatchAddressing_LGCPHexAddress 는 lg_hvacr02 의 8자리 hex 주소가
// 매칭되는지 확인한다.
//
// 보정 전에는 parseHexByte 가 1~2자리만 허용해 8자리 주소 파싱이 실패했고,
// 필터를 설정하면 모든 프레임이 걸러졌다.
func TestLGHvacr02MatchAddressing_LGCPHexAddress(t *testing.T) {
	cfg := LGHvacr02NodeConfig{UnitID: "44550067"}

	if !lgHvacr02MatchAddressing(map[string]any{"unit_id": "44550067"}, cfg) {
		t.Error("8-digit LGCP address should match itself")
	}
	if lgHvacr02MatchAddressing(map[string]any{"unit_id": "44550065"}, cfg) {
		t.Error("a different LGCP address should not match")
	}
	// 대소문자 무시.
	if !lgHvacr02MatchAddressing(map[string]any{"unit_id": "4455006A"},
		LGHvacr02NodeConfig{UnitID: "4455006a"}) {
		t.Error("LGCP address matching should be case-insensitive")
	}
}

// TestLGHvacr02MatchAddressing_HexBytePreserved 는 기존 1~2자리 hex 관용 동작이
// 보존되는지 확인한다 ("0x58" 과 "58" 을 같은 값으로 인정).
func TestLGHvacr02MatchAddressing_HexBytePreserved(t *testing.T) {
	cases := []struct {
		payload string
		filter  string
	}{
		{"58", "0x58"},
		{"0x58", "58"},
		{"58", "58"},
		{"58", "0X58"},
	}
	for _, c := range cases {
		cfg := LGHvacr02NodeConfig{UnitID: c.filter}
		if !lgHvacr02MatchAddressing(map[string]any{"unit_id": c.payload}, cfg) {
			t.Errorf("payload %q should match filter %q (hex byte equivalence)", c.payload, c.filter)
		}
	}
}

// TestLGHvacr02MatchAddressing_EmptyFilter 는 빈 필터가 모든 것을 통과시키는지
// 확인한다.
func TestLGHvacr02MatchAddressing_EmptyFilter(t *testing.T) {
	for _, filter := range []string{"", "   "} {
		cfg := LGHvacr02NodeConfig{UnitID: filter}
		if !lgHvacr02MatchAddressing(map[string]any{"unit_id": "anything"}, cfg) {
			t.Errorf("empty filter %q should pass everything", filter)
		}
	}
}

// TestLGHvacr02MatchAddressing_MissingUnitID 는 payload 에 unit_id 가 없으면
// 필터가 걸러내는지 확인한다.
func TestLGHvacr02MatchAddressing_MissingUnitID(t *testing.T) {
	cfg := LGHvacr02NodeConfig{UnitID: "3"}
	if lgHvacr02MatchAddressing(map[string]any{}, cfg) {
		t.Error("a payload without unit_id should not match an active filter")
	}
	if lgHvacr02MatchAddressing(map[string]any{"unit_id": 3}, cfg) {
		t.Error("a non-string unit_id should not match")
	}
}

func TestNormalizeUnitID(t *testing.T) {
	cases := map[string]string{
		"3":          "3",
		" 3 ":        "3",
		"0x58":       "58",
		"0X58":       "58",
		"4455006A":   "4455006a",
		"  44550067": "44550067",
	}
	for in, want := range cases {
		if got := normalizeUnitID(in); got != want {
			t.Errorf("normalizeUnitID(%q) = %q, want %q", in, got, want)
		}
	}
}
