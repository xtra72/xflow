package flow

import (
	"regexp"
	"testing"
)

// uuidRegex 는 UUID v4 형식을 검증하는 정규식이다.
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isValidUUID 는 문자열이 유효한 UUID 형식인지 확인하는 테스트 헬퍼이다.
func isValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// TestPortDirection_Constants 는 PortDirection 상수 값이 올바른지 검증한다.
func TestPortDirection_Constants(t *testing.T) {
	tests := []struct {
		name     string
		got      PortDirection
		expected string
	}{
		{name: "PortInput 값", got: PortInput, expected: "input"},
		{name: "PortOutput 값", got: PortOutput, expected: "output"},
		{name: "PortError 값", got: PortError, expected: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("PortDirection = %q, 기대값 %q", tt.got, tt.expected)
			}
		})
	}
}

// TestBridgeDirection_Constants 는 BridgeDirection 상수 값이 올바른지 검증한다.
func TestBridgeDirection_Constants(t *testing.T) {
	tests := []struct {
		name     string
		got      BridgeDirection
		expected string
	}{
		{name: "BridgeIn 값", got: BridgeIn, expected: "in"},
		{name: "BridgeOut 값", got: BridgeOut, expected: "out"},
		{name: "BridgeInOut 값", got: BridgeInOut, expected: "inout"},
		{name: "BridgeRequestReply 값", got: BridgeRequestReply, expected: "request_reply"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("BridgeDirection = %q, 기대값 %q", tt.got, tt.expected)
			}
		})
	}
}

// TestNewNodeDef_Basic 은 기본 NewNodeDef 팩토리 함수가 올바른 노드를 생성하는지 검증한다 (AC-08).
func TestNewNodeDef_Basic(t *testing.T) {
	node := NewNodeDef("my-node", "transform")

	// ID가 유효한 UUID여야 한다
	if !isValidUUID(node.ID) {
		t.Errorf("NewNodeDef ID = %q, 유효한 UUID 형식이 아니다", node.ID)
	}

	// Name이 올바르게 설정되어야 한다
	if node.Name != "my-node" {
		t.Errorf("NewNodeDef Name = %q, 기대값 %q", node.Name, "my-node")
	}

	// Type이 올바르게 설정되어야 한다
	if node.Type != "transform" {
		t.Errorf("NewNodeDef Type = %q, 기대값 %q", node.Type, "transform")
	}

	// 기본 입력 포트가 1개 생성되어야 한다
	if len(node.Inputs) != 1 {
		t.Fatalf("NewNodeDef Inputs 수 = %d, 기대값 1", len(node.Inputs))
	}

	// 기본 입력 포트 이름이 "in"이어야 한다
	if node.Inputs[0].Name != "in" {
		t.Errorf("기본 입력 포트 Name = %q, 기대값 %q", node.Inputs[0].Name, "in")
	}

	// 기본 입력 포트 Direction이 PortInput이어야 한다
	if node.Inputs[0].Direction != PortInput {
		t.Errorf("기본 입력 포트 Direction = %q, 기대값 %q", node.Inputs[0].Direction, PortInput)
	}

	// 기본 입력 포트 ID가 유효한 UUID여야 한다
	if !isValidUUID(node.Inputs[0].ID) {
		t.Errorf("기본 입력 포트 ID = %q, 유효한 UUID 형식이 아니다", node.Inputs[0].ID)
	}

	// 기본 출력 포트가 1개 생성되어야 한다
	if len(node.Outputs) != 1 {
		t.Fatalf("NewNodeDef Outputs 수 = %d, 기대값 1", len(node.Outputs))
	}

	// 기본 출력 포트 이름이 "out"이어야 한다
	if node.Outputs[0].Name != "out" {
		t.Errorf("기본 출력 포트 Name = %q, 기대값 %q", node.Outputs[0].Name, "out")
	}

	// 기본 출력 포트 Direction이 PortOutput이어야 한다
	if node.Outputs[0].Direction != PortOutput {
		t.Errorf("기본 출력 포트 Direction = %q, 기대값 %q", node.Outputs[0].Direction, PortOutput)
	}

	// 기본 출력 포트 ID가 유효한 UUID여야 한다
	if !isValidUUID(node.Outputs[0].ID) {
		t.Errorf("기본 출력 포트 ID = %q, 유효한 UUID 형식이 아니다", node.Outputs[0].ID)
	}

	// Errors는 비어있어야 한다
	if len(node.Errors) != 0 {
		t.Errorf("NewNodeDef Errors = %v, 기대값 빈 슬라이스", node.Errors)
	}

	// AgentRef는 nil이어야 한다
	if node.AgentRef != nil {
		t.Errorf("NewNodeDef AgentRef = %v, 기대값 nil", node.AgentRef)
	}
}

// TestNewNodeDef_WithErrorPort 는 WithErrorPort 옵션이 에러 포트를 올바르게 생성하는지 검증한다 (AC-09).
func TestNewNodeDef_WithErrorPort(t *testing.T) {
	node := NewNodeDef("error-node", "processor", WithErrorPort())

	// Errors가 비어있지 않아야 한다
	if len(node.Errors) == 0 {
		t.Fatal("WithErrorPort 적용 후 Errors가 비어있다")
	}

	// Errors[0] Direction이 PortError여야 한다
	if node.Errors[0].Direction != PortError {
		t.Errorf("Errors[0] Direction = %q, 기대값 %q", node.Errors[0].Direction, PortError)
	}

	// Errors[0] Name이 "error"여야 한다
	if node.Errors[0].Name != "error" {
		t.Errorf("Errors[0] Name = %q, 기대값 %q", node.Errors[0].Name, "error")
	}

	// Errors[0] ID가 유효한 UUID여야 한다
	if !isValidUUID(node.Errors[0].ID) {
		t.Errorf("Errors[0] ID = %q, 유효한 UUID 형식이 아니다", node.Errors[0].ID)
	}
}

// TestNewNodeDef_WithAgentRef 는 WithAgentRef 옵션이 AgentRef를 올바르게 설정하는지 검증한다 (AC-10).
func TestNewNodeDef_WithAgentRef(t *testing.T) {
	ref := AgentRef{
		AgentID:   "agent-001",
		AgentName: "chat-agent",
		Direction: BridgeInOut,
	}
	node := NewNodeDef("agent-node", "agent", WithAgentRef(ref))

	// AgentRef가 nil이 아니어야 한다
	if node.AgentRef == nil {
		t.Fatal("WithAgentRef 적용 후 AgentRef가 nil이다")
	}

	// AgentRef 필드가 올바르게 설정되어야 한다
	if node.AgentRef.AgentID != "agent-001" {
		t.Errorf("AgentRef.AgentID = %q, 기대값 %q", node.AgentRef.AgentID, "agent-001")
	}
	if node.AgentRef.AgentName != "chat-agent" {
		t.Errorf("AgentRef.AgentName = %q, 기대값 %q", node.AgentRef.AgentName, "chat-agent")
	}
	if node.AgentRef.Direction != BridgeInOut {
		t.Errorf("AgentRef.Direction = %q, 기대값 %q", node.AgentRef.Direction, BridgeInOut)
	}
}

// TestNewNodeDef_WithInputPorts 는 WithInputPorts 옵션이 기본 입력 포트를 대체하는지 검증한다.
func TestNewNodeDef_WithInputPorts(t *testing.T) {
	customPorts := []Port{
		{ID: "port-1", Name: "data-in", Direction: PortInput},
		{ID: "port-2", Name: "config-in", Direction: PortInput},
	}
	node := NewNodeDef("multi-input", "merger", WithInputPorts(customPorts...))

	// 사용자 정의 포트로 교체되어야 한다
	if len(node.Inputs) != 2 {
		t.Fatalf("WithInputPorts 후 Inputs 수 = %d, 기대값 2", len(node.Inputs))
	}

	if node.Inputs[0].Name != "data-in" {
		t.Errorf("Inputs[0].Name = %q, 기대값 %q", node.Inputs[0].Name, "data-in")
	}
	if node.Inputs[1].Name != "config-in" {
		t.Errorf("Inputs[1].Name = %q, 기대값 %q", node.Inputs[1].Name, "config-in")
	}
}

// TestNewNodeDef_WithOutputPorts 는 WithOutputPorts 옵션이 기본 출력 포트를 대체하는지 검증한다.
func TestNewNodeDef_WithOutputPorts(t *testing.T) {
	customPorts := []Port{
		{ID: "out-1", Name: "success", Direction: PortOutput},
		{ID: "out-2", Name: "filtered", Direction: PortOutput},
		{ID: "out-3", Name: "rejected", Direction: PortOutput},
	}
	node := NewNodeDef("splitter", "router", WithOutputPorts(customPorts...))

	// 사용자 정의 포트로 교체되어야 한다
	if len(node.Outputs) != 3 {
		t.Fatalf("WithOutputPorts 후 Outputs 수 = %d, 기대값 3", len(node.Outputs))
	}

	expectedNames := []string{"success", "filtered", "rejected"}
	for i, expected := range expectedNames {
		if node.Outputs[i].Name != expected {
			t.Errorf("Outputs[%d].Name = %q, 기대값 %q", i, node.Outputs[i].Name, expected)
		}
	}
}

// TestNewNodeDef_WithNodeConfig 는 WithNodeConfig 옵션이 설정 항목을 올바르게 추가하는지 검증한다.
func TestNewNodeDef_WithNodeConfig(t *testing.T) {
	node := NewNodeDef("config-node", "processor",
		WithNodeConfig("timeout", 30),
		WithNodeConfig("retries", 3),
	)

	// Config가 nil이 아니어야 한다
	if node.Config == nil {
		t.Fatal("WithNodeConfig 적용 후 Config가 nil이다")
	}

	// 설정 값이 올바르게 저장되어야 한다
	if node.Config["timeout"] != 30 {
		t.Errorf("Config[timeout] = %v, 기대값 30", node.Config["timeout"])
	}
	if node.Config["retries"] != 3 {
		t.Errorf("Config[retries] = %v, 기대값 3", node.Config["retries"])
	}
}

// TestNewNodeDef_WithNodeMetadata 는 WithNodeMetadata 옵션이 메타데이터를 올바르게 추가하는지 검증한다.
func TestNewNodeDef_WithNodeMetadata(t *testing.T) {
	node := NewNodeDef("meta-node", "transform",
		WithNodeMetadata("author", "xtra"),
		WithNodeMetadata("version", "1.0"),
	)

	// Metadata가 nil이 아니어야 한다
	if node.Metadata == nil {
		t.Fatal("WithNodeMetadata 적용 후 Metadata가 nil이다")
	}

	// 메타데이터 값이 올바르게 저장되어야 한다
	if node.Metadata["author"] != "xtra" {
		t.Errorf("Metadata[author] = %q, 기대값 %q", node.Metadata["author"], "xtra")
	}
	if node.Metadata["version"] != "1.0" {
		t.Errorf("Metadata[version] = %q, 기대값 %q", node.Metadata["version"], "1.0")
	}
}

// TestNewNodeDef_MultipleOptions 는 여러 옵션을 동시에 적용할 수 있는지 검증한다.
func TestNewNodeDef_MultipleOptions(t *testing.T) {
	ref := AgentRef{
		AgentID:   "agent-x",
		AgentName: "worker",
		Direction: BridgeOut,
	}

	node := NewNodeDef("full-node", "agent",
		WithErrorPort(),
		WithAgentRef(ref),
		WithNodeConfig("max_batch", 100),
		WithNodeMetadata("env", "production"),
	)

	// 모든 옵션이 올바르게 적용되어야 한다
	if len(node.Errors) == 0 {
		t.Error("Errors가 비어있다")
	}
	if node.AgentRef == nil {
		t.Error("AgentRef가 nil이다")
	}
	if node.Config == nil || node.Config["max_batch"] != 100 {
		t.Errorf("Config[max_batch] = %v, 기대값 100", node.Config["max_batch"])
	}
	if node.Metadata == nil || node.Metadata["env"] != "production" {
		t.Errorf("Metadata[env] = %q, 기대값 %q", node.Metadata["env"], "production")
	}
}

// TestNewNodeDef_UniqueIDs 는 NewNodeDef가 호출될 때마다 고유한 ID를 생성하는지 검증한다.
func TestNewNodeDef_UniqueIDs(t *testing.T) {
	node1 := NewNodeDef("node-a", "type-a")
	node2 := NewNodeDef("node-b", "type-b")

	if node1.ID == node2.ID {
		t.Errorf("두 노드의 ID가 동일하다: %q", node1.ID)
	}
}
