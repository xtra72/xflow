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

// agentBlock / deviceBlock 은 중첩 블록 config 헬퍼이다.
func agentBlock(fields map[string]any) map[string]any {
	return map[string]any{"agent": fields}
}

func deviceBlock(fields map[string]any) map[string]any {
	return map[string]any{"device": fields}
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

// agentDeviceGroupMsg 는 agent+device 그룹(둘 다 id-only)을 가진 메시지를 만든다.
func agentDeviceGroupMsg(agentID, deviceID string) message.Message {
	msg := message.New(message.WithID("m1"), message.WithType("event"))
	msg.Metadata().SetGroup("agent", map[string]string{"id": agentID})
	msg.Metadata().SetGroup("device", map[string]string{"id": deviceID})
	return msg
}

// TestEnrichNode_Process 는 enrich 노드의 주요 동작을 표 기반으로 검증한다(신규 중첩 형식).
func TestEnrichNode_Process(t *testing.T) {
	tests := []struct {
		name  string
		cfg   map[string]any
		msg   message.Message
		check func(t *testing.T, out message.Message)
	}{
		{
			name: "agent to_metadata rehydrates group preserving id",
			cfg:  agentBlock(map[string]any{"to_metadata": true}),
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
			cfg:  deviceBlock(map[string]any{"to_metadata": true}),
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
			cfg:  agentBlock(map[string]any{"to_payload": "agent_info"}),
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
			name: "both to_metadata and to_payload combined (single source)",
			cfg:  agentBlock(map[string]any{"to_metadata": true, "to_payload": "agent_info"}),
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
			cfg:  agentBlock(map[string]any{"id_source": "$.payload.device_id", "to_payload": "agent_info"}),
			msg: func() message.Message {
				return message.New(message.WithID("m1"),
					message.WithPayload(message.NewPayload(map[string]any{"device_id": "a-1"})))
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
			name: "missing id: marker set but no group filled",
			cfg:  agentBlock(map[string]any{"to_metadata": true}),
			msg:  message.New(message.WithID("m1"), message.WithType("event")), // agent 그룹 없음
			check: func(t *testing.T, out message.Message) {
				if _, ok := out.Metadata().GetGroup("agent"); ok {
					t.Error("id 없음에도 agent 그룹이 생성되었다")
				}
				// to_metadata 이므로 마커는 설정되어야 한다.
				if keep, _ := out.Metadata().Get(message.MetaKeySlimKeep); keep != "agent" {
					t.Errorf("to_metadata 는 마커를 설정해야 한다: %q", keep)
				}
			},
		},
		{
			name: "not-found id: marker set, group not filled",
			cfg:  agentBlock(map[string]any{"to_metadata": true, "to_payload": "agent_info"}),
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
				if keep, _ := out.Metadata().Get(message.MetaKeySlimKeep); keep != "agent" {
					t.Errorf("not-found 여도 to_metadata 마커는 설정: %q", keep)
				}
			},
		},
		{
			name: "id preservation: lookup id ignored, source id kept",
			cfg:  deviceBlock(map[string]any{"to_metadata": true}),
			msg:  deviceGroupMsg("d-1"),
			check: func(t *testing.T, out message.Message) {
				g, _ := out.Metadata().GetGroup("device")
				if g["id"] != "d-1" {
					t.Errorf("소스 id 가 보존되어야 한다: %#v", g)
				}
			},
		},
		{
			name: "enabled=false deactivates block (other block active)",
			cfg: map[string]any{
				"agent":  map[string]any{"enabled": false, "to_metadata": true},
				"device": map[string]any{"to_metadata": true},
			},
			msg: agentDeviceGroupMsg("a-1", "d-1"),
			check: func(t *testing.T, out message.Message) {
				// agent 는 비활성 → type 없음(마커도 agent 없음)
				ag, _ := out.Metadata().GetGroup("agent")
				if _, hasType := ag["type"]; hasType {
					t.Errorf("비활성 agent 는 재수화되면 안 된다: %#v", ag)
				}
				// device 는 활성 → 재수화됨
				dg, _ := out.Metadata().GetGroup("device")
				if dg["type"] != "HVACR.IDU" {
					t.Errorf("활성 device 는 재수화되어야 한다: %#v", dg)
				}
				if keep, _ := out.Metadata().Get(message.MetaKeySlimKeep); keep != "device" {
					t.Errorf("device 만 마커에 있어야 한다: %q", keep)
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

// TestEnrichNode_BothActive 는 agent 와 device 를 동시에 활성화하면 둘 다 독립적으로
// 보강됨을 검증한다(핵심 신규 기능).
func TestEnrichNode_BothActive(t *testing.T) {
	n := newEnrichNode(t, map[string]any{
		"agent":  map[string]any{"to_metadata": true, "to_payload": "agent_info"},
		"device": map[string]any{"to_metadata": true, "to_payload": "device_info"},
	})
	out, err := n.Process(context.Background(), agentDeviceGroupMsg("a-1", "d-1"))
	if err != nil {
		t.Fatalf("Process 에러: %v", err)
	}
	msg := out[0]

	// agent 그룹 + payload 보강
	ag, _ := msg.Metadata().GetGroup("agent")
	if ag["type"] != "serial" || ag["name"] != "reader" || ag["id"] != "a-1" {
		t.Errorf("agent 그룹 재수화 실패: %#v", ag)
	}
	ai, _ := msg.Payload().Get("agent_info")
	if obj, _ := ai.(map[string]any); obj["type"] != "serial" {
		t.Errorf("agent_info payload 실패: %#v", ai)
	}

	// device 그룹 + payload 보강
	dg, _ := msg.Metadata().GetGroup("device")
	if dg["type"] != "HVACR.IDU" || dg["name"] != "room1" || dg["id"] != "d-1" {
		t.Errorf("device 그룹 재수화 실패: %#v", dg)
	}
	di, _ := msg.Payload().Get("device_info")
	if obj, _ := di.(map[string]any); obj["type"] != "HVACR.IDU" {
		t.Errorf("device_info payload 실패: %#v", di)
	}

	// 마커에 두 그룹 모두 포함
	keep, _ := msg.Metadata().Get(message.MetaKeySlimKeep)
	if keep != "agent,device" {
		t.Errorf("마커에 두 그룹 모두 포함되어야 한다: %q", keep)
	}
}

// TestEnrichNode_BothActive_MixedLookup 는 device 는 조회 성공, agent 는 not-found 인
// 경우 각 소스가 독립적으로 처리됨을 검증한다.
func TestEnrichNode_BothActive_MixedLookup(t *testing.T) {
	n := newEnrichNode(t, map[string]any{
		"agent":  map[string]any{"to_metadata": true},
		"device": map[string]any{"to_metadata": true},
	})
	// agent id 는 미존재(unknown), device id 는 존재(d-1).
	out, err := n.Process(context.Background(), agentDeviceGroupMsg("unknown", "d-1"))
	if err != nil {
		t.Fatalf("Process 에러: %v", err)
	}
	msg := out[0]

	// agent: not-found → type 없음, id 보존
	ag, _ := msg.Metadata().GetGroup("agent")
	if _, hasType := ag["type"]; hasType {
		t.Errorf("agent not-found 인데 type 이 채워졌다: %#v", ag)
	}
	if ag["id"] != "unknown" {
		t.Errorf("agent id 보존 실패: %#v", ag)
	}

	// device: 성공 → 재수화
	dg, _ := msg.Metadata().GetGroup("device")
	if dg["type"] != "HVACR.IDU" {
		t.Errorf("device 재수화 실패: %#v", dg)
	}

	// 마커에는 to_metadata 인 두 소스 모두 포함(조회 성공 여부 무관).
	keep, _ := msg.Metadata().Get(message.MetaKeySlimKeep)
	if keep != "agent,device" {
		t.Errorf("두 소스 모두 마커에 있어야 한다(조회 결과 무관): %q", keep)
	}
}

// TestEnrichNode_PerSourceIDSourceDefaults 는 id_source 미지정 시 소스별 기본값이
// 적용됨을 검증한다.
func TestEnrichNode_PerSourceIDSourceDefaults(t *testing.T) {
	n := newEnrichNode(t, map[string]any{
		"agent":  map[string]any{"to_metadata": true},
		"device": map[string]any{"to_metadata": true},
	})
	// 기본 id_source 는 각각 $.metadata.agent.id / $.metadata.device.id.
	out, _ := n.Process(context.Background(), agentDeviceGroupMsg("a-1", "d-1"))
	ag, _ := out[0].Metadata().GetGroup("agent")
	dg, _ := out[0].Metadata().GetGroup("device")
	if ag["type"] != "serial" {
		t.Errorf("agent 기본 id_source 해석 실패: %#v", ag)
	}
	if dg["type"] != "HVACR.IDU" {
		t.Errorf("device 기본 id_source 해석 실패: %#v", dg)
	}
}

// TestEnrichNode_PayloadOnlyPerSource 는 to_payload 만 있는 소스는 마커를 설정하지
// 않으며 payload 만 기록함을 검증한다.
func TestEnrichNode_PayloadOnlyPerSource(t *testing.T) {
	n := newEnrichNode(t, deviceBlock(map[string]any{"to_payload": "device_info"}))
	out, _ := n.Process(context.Background(), deviceGroupMsg("d-1"))
	msg := out[0]

	// payload 기록됨
	di, ok := msg.Payload().Get("device_info")
	if !ok {
		t.Fatal("device_info payload 없음")
	}
	if obj, _ := di.(map[string]any); obj["type"] != "HVACR.IDU" {
		t.Errorf("device_info 실패: %#v", di)
	}
	// 마커는 설정 안 됨(payload-only)
	if keep, ok := msg.Metadata().Get(message.MetaKeySlimKeep); ok && keep != "" {
		t.Errorf("payload-only 는 마커를 설정하면 안 된다: %q", keep)
	}
	// device 그룹은 그대로(id-only, 재수화 안 함)
	dg, _ := msg.Metadata().GetGroup("device")
	if _, hasType := dg["type"]; hasType {
		t.Errorf("payload-only 는 metadata 그룹을 재수화하면 안 된다: %#v", dg)
	}
}

// TestEnrichNode_ConfigValidation 은 factory 단계의 config 검증을 검증한다(신규 형식).
func TestEnrichNode_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]any
		wantErr bool
	}{
		{"agent to_metadata active", agentBlock(map[string]any{"to_metadata": true}), false},
		{"device to_payload active", deviceBlock(map[string]any{"to_payload": "x"}), false},
		{"both active", map[string]any{
			"agent":  map[string]any{"to_metadata": true},
			"device": map[string]any{"to_payload": "x"},
		}, false},
		{"empty config → no active source", map[string]any{}, true},
		{"agent block but no outputs → inactive → no active source", agentBlock(map[string]any{}), true},
		{"agent enabled=false, no other → no active source", agentBlock(map[string]any{"enabled": false, "to_metadata": true}), true},
		{"agent not a map", map[string]any{"agent": "oops"}, true},
		{"to_metadata wrong type", agentBlock(map[string]any{"to_metadata": "yes"}), true},
		{"id_source wrong type", agentBlock(map[string]any{"id_source": 123, "to_metadata": true}), true},
		{"enabled wrong type", agentBlock(map[string]any{"enabled": "yes", "to_metadata": true}), true},
		{"to_payload wrong type", agentBlock(map[string]any{"to_payload": 5}), true},
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

// TestEnrichNode_InitRequiresLookup 은 활성 소스의 룩업 미주입 시 Init 이 실패함을 검증한다.
func TestEnrichNode_InitRequiresLookup(t *testing.T) {
	// agent 활성인데 agent 룩업 미주입 → Init 실패.
	def := flow.NodeDef{ID: "enr", Type: "enrich", Config: agentBlock(map[string]any{"to_metadata": true})}
	n, err := NewEnrichNode(def) // 룩업 미주입
	if err != nil {
		t.Fatalf("factory 는 성공해야 한다(주입은 Init 검증): %v", err)
	}
	if initErr := n.Init(context.Background()); initErr == nil {
		t.Error("agent 룩업 미주입 시 Init 은 실패해야 한다")
	}

	// device 만 활성이면 agent 룩업이 없어도 Init 성공해야 한다.
	def2 := flow.NodeDef{ID: "enr2", Type: "enrich", Config: deviceBlock(map[string]any{"to_metadata": true})}
	n2, err := NewEnrichNode(def2, WithDeviceInfoLookup(DeviceLookupFunc(fakeDeviceLookup)))
	if err != nil {
		t.Fatalf("factory 실패: %v", err)
	}
	if initErr := n2.Init(context.Background()); initErr != nil {
		t.Errorf("device 만 활성이면 device 룩업만으로 Init 성공해야 한다: %v", initErr)
	}
}

// TestEnrichNode_NeitherActive_NoOutput 는 활성 소스가 없으면 ErrEnrichNoOutput.
func TestEnrichNode_NeitherActive_NoOutput(t *testing.T) {
	def := flow.NodeDef{ID: "enr", Type: "enrich", Config: map[string]any{
		"agent":  map[string]any{"enabled": false, "to_metadata": true},
		"device": map[string]any{}, // 출력 없음 → 비활성
	}}
	_, err := NewEnrichNode(def,
		WithAgentInfoLookup(AgentLookupFunc(fakeAgentLookup)),
		WithDeviceInfoLookup(DeviceLookupFunc(fakeDeviceLookup)),
	)
	if err == nil {
		t.Fatal("활성 소스가 없으면 에러여야 한다")
	}
}

// TestEnrichNode_RegisteredInRegistry 는 enrich 타입이 빌트인으로 등록됨을 검증한다.
func TestEnrichNode_RegisteredInRegistry(t *testing.T) {
	reg := NewRegistry()
	if !reg.Has("enrich") {
		t.Fatal("enrich 노드 타입이 레지스트리에 등록되지 않았다")
	}
	def := flow.NodeDef{ID: "enr", Type: "enrich", Config: agentBlock(map[string]any{"to_metadata": true})}
	if _, err := reg.Create(def,
		WithAgentInfoLookup(AgentLookupFunc(fakeAgentLookup)),
	); err != nil {
		t.Errorf("레지스트리를 통한 enrich 생성 실패: %v", err)
	}
}
