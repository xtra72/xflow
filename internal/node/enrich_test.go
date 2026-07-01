package node

import (
	"context"
	"testing"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// fakeAgentLookup / fakeDeviceLookup 은 테스트용 룩업이다.
func fakeAgentLookup(id string) (RegistryMeta, bool) {
	if id == "a-1" {
		return RegistryMeta{Type: "serial", ID: "a-1", Name: "reader"}, true
	}
	return RegistryMeta{}, false
}

func fakeDeviceLookup(id string) (RegistryMeta, bool) {
	if id == "d-1" {
		return RegistryMeta{Type: "HVACR.IDU", ID: "d-1", Name: "room1"}, true
	}
	return RegistryMeta{}, false
}

// newEnrichNode 는 주입 옵션과 함께 enrich 노드를 생성하고 Init 한다.
func newEnrichNode(t *testing.T, cfg map[string]any) *EnrichNode {
	t.Helper()
	def := flow.NodeDef{ID: "enr", Type: "enrich", Name: "enr", Config: cfg}
	n, err := NewEnrichNode(def,
		WithAgentInfoLookup(AgentLookupFunc(fakeAgentLookup)),
		WithDeviceInfoLookup(DeviceLookupFunc(fakeDeviceLookup)),
	)
	if err != nil {
		t.Fatalf("NewEnrichNode 실패: %v", err)
	}
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("Init 실패: %v", err)
	}
	return n.(*EnrichNode)
}

// agentGroupMsg 는 agent 그룹(id-only)을 가진 슬림 메시지를 만든다.
func agentGroupMsg(id string) message.Message {
	msg := message.New(message.WithID("m1"), message.WithType("event"))
	msg.Metadata().SetGroup("agent", map[string]string{"id": id})
	return msg
}

// deviceGroupMsg 는 device 그룹(id-only)을 가진 슬림 메시지를 만든다.
func deviceGroupMsg(id string) message.Message {
	msg := message.New(message.WithID("m1"), message.WithType("event"))
	msg.Metadata().SetGroup("device", map[string]string{"id": id})
	return msg
}

// TestEnrichNode_Process 는 enrich 노드의 주요 동작을 표 기반으로 검증한다.
func TestEnrichNode_Process(t *testing.T) {
	tests := []struct {
		name  string
		cfg   map[string]any
		msg   message.Message
		check func(t *testing.T, out message.Message)
	}{
		{
			name: "agent to_metadata rehydrates group preserving id",
			cfg:  map[string]any{"source": "agent", "to_metadata": true},
			msg:  agentGroupMsg("a-1"),
			check: func(t *testing.T, out message.Message) {
				g, ok := out.Metadata().GetGroup("agent")
				if !ok {
					t.Fatal("agent 그룹이 없다")
				}
				if g["id"] != "a-1" || g["type"] != "serial" || g["name"] != "reader" {
					t.Errorf("agent 그룹 재수화 불일치: %#v", g)
				}
			},
		},
		{
			name: "device to_metadata rehydrates group",
			cfg:  map[string]any{"source": "device", "to_metadata": true},
			msg:  deviceGroupMsg("d-1"),
			check: func(t *testing.T, out message.Message) {
				g, _ := out.Metadata().GetGroup("device")
				if g["id"] != "d-1" || g["type"] != "HVACR.IDU" || g["name"] != "room1" {
					t.Errorf("device 그룹 재수화 불일치: %#v", g)
				}
			},
		},
		{
			name: "agent to_payload writes object",
			cfg:  map[string]any{"source": "agent", "to_payload": "agent_info"},
			msg:  agentGroupMsg("a-1"),
			check: func(t *testing.T, out message.Message) {
				v, ok := out.Payload().Get("agent_info")
				if !ok {
					t.Fatal("agent_info payload 키가 없다")
				}
				obj, ok := v.(map[string]any)
				if !ok {
					t.Fatalf("agent_info 가 객체가 아니다: %#v", v)
				}
				if obj["id"] != "a-1" || obj["type"] != "serial" || obj["name"] != "reader" {
					t.Errorf("payload 객체 불일치: %#v", obj)
				}
			},
		},
		{
			name: "both to_metadata and to_payload combined",
			cfg:  map[string]any{"source": "agent", "to_metadata": true, "to_payload": "agent_info"},
			msg:  agentGroupMsg("a-1"),
			check: func(t *testing.T, out message.Message) {
				g, _ := out.Metadata().GetGroup("agent")
				if g["type"] != "serial" {
					t.Errorf("메타데이터 재수화 실패: %#v", g)
				}
				v, ok := out.Payload().Get("agent_info")
				if !ok {
					t.Fatal("payload 키가 없다")
				}
				if obj, _ := v.(map[string]any); obj["name"] != "reader" {
					t.Errorf("payload 기록 실패: %#v", v)
				}
			},
		},
		{
			name: "id from payload source",
			cfg:  map[string]any{"source": "agent", "id_source": "$.payload.device_id", "to_payload": "agent_info"},
			msg: func() message.Message {
				m := message.New(message.WithID("m1"),
					message.WithPayload(message.NewPayload(map[string]any{"device_id": "a-1"})))
				return m
			}(),
			check: func(t *testing.T, out message.Message) {
				v, ok := out.Payload().Get("agent_info")
				if !ok {
					t.Fatal("agent_info 없음")
				}
				if obj, _ := v.(map[string]any); obj["type"] != "serial" {
					t.Errorf("payload id_source 해석 실패: %#v", v)
				}
			},
		},
		{
			name: "missing id passes through unchanged",
			cfg:  map[string]any{"source": "agent", "to_metadata": true},
			msg:  message.New(message.WithID("m1"), message.WithType("event")), // agent 그룹 없음
			check: func(t *testing.T, out message.Message) {
				if _, ok := out.Metadata().GetGroup("agent"); ok {
					t.Error("id 없음에도 agent 그룹이 생성되었다")
				}
			},
		},
		{
			name: "not-found id passes through without mutation",
			cfg:  map[string]any{"source": "agent", "to_metadata": true, "to_payload": "agent_info"},
			msg:  agentGroupMsg("unknown"),
			check: func(t *testing.T, out message.Message) {
				g, _ := out.Metadata().GetGroup("agent")
				if g["id"] != "unknown" {
					t.Errorf("원본 id 가 보존되어야 한다: %#v", g)
				}
				if _, hasType := g["type"]; hasType {
					t.Errorf("not-found 인데 type 이 추가되었다: %#v", g)
				}
				if _, ok := out.Payload().Get("agent_info"); ok {
					t.Error("not-found 인데 payload 가 기록되었다")
				}
			},
		},
		{
			name: "id preservation: lookup id ignored, source id kept",
			cfg:  map[string]any{"source": "device", "to_metadata": true},
			msg:  deviceGroupMsg("d-1"),
			check: func(t *testing.T, out message.Message) {
				g, _ := out.Metadata().GetGroup("device")
				if g["id"] != "d-1" {
					t.Errorf("소스 id 가 보존되어야 한다: %#v", g)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newEnrichNode(t, tt.cfg)
			out, err := n.Process(context.Background(), tt.msg)
			if err != nil {
				t.Fatalf("Process 에러: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("출력 메시지 개수 = %d, 기대 1", len(out))
			}
			tt.check(t, out[0])
		})
	}
}

// TestEnrichNode_ConfigValidation 은 factory 단계의 config 검증을 검증한다.
func TestEnrichNode_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]any
		wantErr bool
	}{
		{"valid agent to_metadata", map[string]any{"source": "agent", "to_metadata": true}, false},
		{"valid device to_payload", map[string]any{"source": "device", "to_payload": "x"}, false},
		{"missing source", map[string]any{"to_metadata": true}, true},
		{"invalid source", map[string]any{"source": "flow", "to_metadata": true}, true},
		{"no output", map[string]any{"source": "agent"}, true},
		{"to_metadata wrong type", map[string]any{"source": "agent", "to_metadata": "yes"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NodeDef{ID: "enr", Type: "enrich", Config: tt.cfg}
			_, err := NewEnrichNode(def,
				WithAgentInfoLookup(AgentLookupFunc(fakeAgentLookup)),
				WithDeviceInfoLookup(DeviceLookupFunc(fakeDeviceLookup)),
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEnrichNode err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestEnrichNode_InitRequiresLookup 은 주입 없이 Init 하면 실패함을 검증한다.
func TestEnrichNode_InitRequiresLookup(t *testing.T) {
	def := flow.NodeDef{ID: "enr", Type: "enrich", Config: map[string]any{"source": "agent", "to_metadata": true}}
	n, err := NewEnrichNode(def) // 룩업 미주입
	if err != nil {
		t.Fatalf("factory 는 성공해야 한다(주입은 Init 검증): %v", err)
	}
	if initErr := n.Init(context.Background()); initErr == nil {
		t.Error("agent 룩업 미주입 시 Init 은 실패해야 한다")
	}
}

// TestEnrichNode_RegisteredInRegistry 는 enrich 타입이 빌트인으로 등록됨을 검증한다.
func TestEnrichNode_RegisteredInRegistry(t *testing.T) {
	reg := NewRegistry()
	if !reg.Has("enrich") {
		t.Fatal("enrich 노드 타입이 레지스트리에 등록되지 않았다")
	}
	def := flow.NodeDef{ID: "enr", Type: "enrich", Config: map[string]any{"source": "agent", "to_metadata": true}}
	if _, err := reg.Create(def,
		WithAgentInfoLookup(AgentLookupFunc(fakeAgentLookup)),
	); err != nil {
		t.Errorf("레지스트리를 통한 enrich 생성 실패: %v", err)
	}
}
