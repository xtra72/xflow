package node

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// newGroupMessage 는 flat 키 1개와 nested group 1개를 가진 테스트 메시지를 만든다.
func newGroupMessage() message.Message {
	msg := message.New(
		message.WithID("dbg-group-1"),
		message.WithType("event"),
		message.WithMetadata("flatKey", "flatVal"),
	)
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})
	return msg
}

// TestDebugNode_JSONMetadataPreservesGroup 는 egress 슬림화 제거 후 format=json 전체
// 출력 시 metadata JSON 의 agent group 이 full(type/id)로 유지됨을 검증한다. flat 키도 유지.
func TestDebugNode_JSONMetadataPreservesGroup(t *testing.T) {
	def := flow.NodeDef{ID: "dbg", Type: "debug", Name: "dbg"}
	n, _ := NewDebugNode(def)
	dn := n.(*DebugNode)

	msg := newGroupMessage()

	// property 없음, format=json → buildMessageMap 경로
	out := dn.formatProcess(msg, "", "json", nil)

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("출력이 유효한 JSON 이 아니다: %v\n출력: %s", err, out)
	}

	md, ok := m["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata 가 객체가 아니다: %#v", m["metadata"])
	}

	// flat 키는 문자열로 유지
	if md["flatKey"] != "flatVal" {
		t.Errorf("flatKey = %v, 기대값 \"flatVal\"", md["flatKey"])
	}

	// group 은 여전히 중첩 객체이되 id-only 로 슬림화
	agent, ok := md["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent 가 중첩 객체로 출력되지 않았다: %#v (group 형태 유지되어야 함)", md["agent"])
	}
	if agent["id"] != "node-1" {
		t.Errorf("agent.id 가 보존되어야 한다: %#v", agent)
	}
	if agent["type"] != "serial" {
		t.Errorf("egress 에서 agent.type 이 full 로 유지되어야 한다: %#v", agent)
	}
}

// TestDebugNode_PropertyMetadataPreservesGroup 는 property=.metadata 추출 시 group 이
// full(type/id)로 노출됨을 검증한다 (egress 슬림화 제거 후).
func TestDebugNode_PropertyMetadataPreservesGroup(t *testing.T) {
	def := flow.NodeDef{ID: "dbg", Type: "debug", Name: "dbg"}
	n, _ := NewDebugNode(def)
	dn := n.(*DebugNode)

	msg := newGroupMessage()

	out := dn.formatProcess(msg, ".metadata", "json", nil)

	var md map[string]any
	if err := json.Unmarshal([]byte(out), &md); err != nil {
		t.Fatalf("property=.metadata 출력이 유효한 JSON 이 아니다: %v\n출력: %s", err, out)
	}

	if md["flatKey"] != "flatVal" {
		t.Errorf("flatKey = %v", md["flatKey"])
	}
	agent, ok := md["agent"].(map[string]any)
	if !ok {
		t.Fatalf(".metadata 에 agent group 이 노출되지 않았다: %#v", md["agent"])
	}
	if agent["id"] != "node-1" {
		t.Errorf("agent.id 가 보존되어야 한다: %#v", agent)
	}
	if agent["type"] != "serial" {
		t.Errorf("egress 에서 agent.type 이 full 로 유지되어야 한다: %#v", agent)
	}
}

// TestDebugNode_PlainLogLinePreservesGroup 는 plain 포맷 기본 로그 라인에 agent group 이
// full(type/id)로 포함됨을 검증한다 (egress 슬림화 제거 후).
func TestDebugNode_PlainLogLinePreservesGroup(t *testing.T) {
	def := flow.NodeDef{ID: "dbg", Type: "debug", Name: "dbg"}
	n, _ := NewDebugNode(def)
	dn := n.(*DebugNode)

	msg := newGroupMessage()

	out := dn.formatProcess(msg, "", "plain", nil)

	// plain 기본 로그 라인은 "time level name {json-body}" 형태.
	// body 의 metadata 에 agent group(id-only)이 포함되어야 한다.
	if !strings.Contains(out, "\"agent\"") {
		t.Errorf("plain 로그 라인에 agent group 이 없다: %s", out)
	}
	if !strings.Contains(out, "node-1") {
		t.Errorf("plain 로그 라인에 agent.id(node-1)가 없다: %s", out)
	}
	// egress 슬림화 제거: type 값("serial")도 그대로 유지되어야 한다.
	if !strings.Contains(out, "serial") {
		t.Errorf("egress 후 plain 로그 라인에 agent.type(serial)이 유지되어야 한다: %s", out)
	}
}

// TestDebugNode_Process_PassThrough_PreservesGroup 는 디버그 노드가 pass-through 시
// 메시지의 group 을 그대로 유지하는지(부작용 없음) 검증한다.
func TestDebugNode_Process_PassThrough_PreservesGroup(t *testing.T) {
	def := flow.NodeDef{ID: "dbg", Type: "debug", Name: "dbg"}
	n, _ := NewDebugNode(def)

	msg := newGroupMessage()
	out, err := n.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process 에러: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("출력 메시지 개수 = %d, 기대값 1", len(out))
	}
	agent, ok := out[0].Metadata().GetGroup("agent")
	if !ok || agent["type"] != "serial" {
		t.Errorf("pass-through 후 group 손실: %v ok=%v", agent, ok)
	}
}
