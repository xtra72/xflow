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

// TestDebugNode_JSONMetadataIncludesGroup 는 format=json, 메시지 전체 출력 시
// metadata JSON 에 nested group 이 중첩 객체로 포함되는지 검증한다 (P2 Class A).
func TestDebugNode_JSONMetadataIncludesGroup(t *testing.T) {
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

	// group 은 중첩 객체로
	agent, ok := md["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent 가 중첩 객체로 출력되지 않았다: %#v (groups dropped?)", md["agent"])
	}
	if agent["type"] != "serial" || agent["id"] != "node-1" {
		t.Errorf("agent group 내용 불일치: %#v", agent)
	}
}

// TestDebugNode_PropertyMetadataShowsGroup 는 property=.metadata 추출 시 group 이
// 노출되는지 검증한다 (P2 Class A: extractProperty 가 Raw() 를 반환).
func TestDebugNode_PropertyMetadataShowsGroup(t *testing.T) {
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
		t.Fatalf(".metadata 에 group 이 노출되지 않았다: %#v", md["agent"])
	}
	if agent["type"] != "serial" {
		t.Errorf("agent.type = %v", agent["type"])
	}
}

// TestDebugNode_PlainLogLineIncludesGroup 는 plain 포맷 기본 로그 라인에 group 이
// 포함되는지 검증한다 (P2 Class A: buildLogLine body metadata 가 Raw()).
func TestDebugNode_PlainLogLineIncludesGroup(t *testing.T) {
	def := flow.NodeDef{ID: "dbg", Type: "debug", Name: "dbg"}
	n, _ := NewDebugNode(def)
	dn := n.(*DebugNode)

	msg := newGroupMessage()

	out := dn.formatProcess(msg, "", "plain", nil)

	// plain 기본 로그 라인은 "time level name {json-body}" 형태.
	// body 의 metadata 에 agent group 이 포함되어야 한다.
	if !strings.Contains(out, "\"agent\"") {
		t.Errorf("plain 로그 라인에 agent group 이 없다: %s", out)
	}
	if !strings.Contains(out, "serial") {
		t.Errorf("plain 로그 라인에 group 값(serial)이 없다: %s", out)
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
