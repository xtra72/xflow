package flow

import (
	"errors"
	"testing"
)

// TestParseNodePath_BasicDotNotation 은 기본 dot 표기법 경로가 올바르게 파싱되는지 검증한다 (AC-14).
func TestParseNodePath_BasicDotNotation(t *testing.T) {
	np, err := ParseNodePath("myflow.mynode")
	if err != nil {
		t.Fatalf("ParseNodePath(\"myflow.mynode\") 에러 = %v, 기대값 nil", err)
	}

	if np.FlowRef != "myflow" {
		t.Errorf("FlowRef = %q, 기대값 %q", np.FlowRef, "myflow")
	}

	if np.NodeRef != "mynode" {
		t.Errorf("NodeRef = %q, 기대값 %q", np.NodeRef, "mynode")
	}
}

// TestParseNodePath_BracketFlowRef 는 FlowRef에 dot이 포함된 bracket 이스케이프를 검증한다 (AC-15).
func TestParseNodePath_BracketFlowRef(t *testing.T) {
	np, err := ParseNodePath("[flow.name].nodeName")
	if err != nil {
		t.Fatalf("ParseNodePath(\"[flow.name].nodeName\") 에러 = %v, 기대값 nil", err)
	}

	if np.FlowRef != "flow.name" {
		t.Errorf("FlowRef = %q, 기대값 %q", np.FlowRef, "flow.name")
	}

	if np.NodeRef != "nodeName" {
		t.Errorf("NodeRef = %q, 기대값 %q", np.NodeRef, "nodeName")
	}
}

// TestParseNodePath_BothBrackets 는 양쪽 모두 bracket 이스케이프인 경우를 검증한다 (AC-16).
func TestParseNodePath_BothBrackets(t *testing.T) {
	np, err := ParseNodePath("[flow.a].[node.b]")
	if err != nil {
		t.Fatalf("ParseNodePath(\"[flow.a].[node.b]\") 에러 = %v, 기대값 nil", err)
	}

	if np.FlowRef != "flow.a" {
		t.Errorf("FlowRef = %q, 기대값 %q", np.FlowRef, "flow.a")
	}

	if np.NodeRef != "node.b" {
		t.Errorf("NodeRef = %q, 기대값 %q", np.NodeRef, "node.b")
	}
}

// TestParseNodePath_BracketNodeRef 는 NodeRef에만 dot이 포함된 bracket 이스케이프를 검증한다.
func TestParseNodePath_BracketNodeRef(t *testing.T) {
	np, err := ParseNodePath("flowName.[node.name]")
	if err != nil {
		t.Fatalf("ParseNodePath(\"flowName.[node.name]\") 에러 = %v, 기대값 nil", err)
	}

	if np.FlowRef != "flowName" {
		t.Errorf("FlowRef = %q, 기대값 %q", np.FlowRef, "flowName")
	}

	if np.NodeRef != "node.name" {
		t.Errorf("NodeRef = %q, 기대값 %q", np.NodeRef, "node.name")
	}
}

// TestParseNodePath_ErrorCases 는 잘못된 경로 입력에 대한 에러 반환을 검증한다 (AC-17).
func TestParseNodePath_ErrorCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "빈 문자열", input: ""},
		{name: "구분자 없음", input: "singlename"},
		{name: "닫히지 않은 bracket", input: "[unclosed.bracket"},
		{name: "빈 FlowRef", input: ".node"},
		{name: "빈 NodeRef", input: "flow."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseNodePath(tt.input)
			if err == nil {
				t.Errorf("ParseNodePath(%q) 에러 = nil, ErrInvalidPath 기대", tt.input)
				return
			}

			if !errors.Is(err, ErrInvalidPath) {
				t.Errorf("ParseNodePath(%q) 에러 = %v, ErrInvalidPath 기대", tt.input, err)
			}
		})
	}
}

// TestNodePath_String 은 NodePath.String() 메서드가 올바른 dot 표기법 문자열을 반환하는지 검증한다 (AC-18).
func TestNodePath_String(t *testing.T) {
	tests := []struct {
		name     string
		path     NodePath
		expected string
	}{
		{
			name:     "기본 dot 표기법",
			path:     NodePath{FlowRef: "flow1", NodeRef: "node1"},
			expected: "flow1.node1",
		},
		{
			name:     "FlowRef에 dot 포함",
			path:     NodePath{FlowRef: "flow.name", NodeRef: "node1"},
			expected: "[flow.name].node1",
		},
		{
			name:     "NodeRef에 dot 포함",
			path:     NodePath{FlowRef: "flow1", NodeRef: "node.name"},
			expected: "flow1.[node.name]",
		},
		{
			name:     "양쪽 모두 dot 포함",
			path:     NodePath{FlowRef: "flow.a", NodeRef: "node.b"},
			expected: "[flow.a].[node.b]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.path.String()
			if got != tt.expected {
				t.Errorf("NodePath.String() = %q, 기대값 %q", got, tt.expected)
			}
		})
	}
}

// TestParseNodePath_RoundTrip 은 ParseNodePath와 String() 간 왕복 변환이 일관성을 유지하는지 검증한다.
func TestParseNodePath_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "기본 경로", path: "flow1.node1"},
		{name: "FlowRef bracket", path: "[flow.name].node1"},
		{name: "NodeRef bracket", path: "flow1.[node.name]"},
		{name: "양쪽 bracket", path: "[flow.a].[node.b]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np, err := ParseNodePath(tt.path)
			if err != nil {
				t.Fatalf("ParseNodePath(%q) 에러 = %v", tt.path, err)
			}

			roundTripped := np.String()
			if roundTripped != tt.path {
				t.Errorf("왕복 변환 실패: 원본 = %q, 결과 = %q", tt.path, roundTripped)
			}

			// 왕복 변환 결과를 다시 파싱하여 동일한 NodePath인지 확인
			np2, err := ParseNodePath(roundTripped)
			if err != nil {
				t.Fatalf("두 번째 ParseNodePath(%q) 에러 = %v", roundTripped, err)
			}

			if np.FlowRef != np2.FlowRef {
				t.Errorf("FlowRef 불일치: %q != %q", np.FlowRef, np2.FlowRef)
			}
			if np.NodeRef != np2.NodeRef {
				t.Errorf("NodeRef 불일치: %q != %q", np.NodeRef, np2.NodeRef)
			}
		})
	}
}

// TestParseNodePath_UUIDPath 는 UUID 형식의 경로가 올바르게 파싱되는지 검증한다.
func TestParseNodePath_UUIDPath(t *testing.T) {
	input := "550e8400-e29b-41d4-a716-446655440000.a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	np, err := ParseNodePath(input)
	if err != nil {
		t.Fatalf("ParseNodePath(UUID 경로) 에러 = %v, 기대값 nil", err)
	}

	// UUID에는 dot이 포함되지 않으므로 첫 번째 dot에서 분리된다
	if np.FlowRef != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("FlowRef = %q, 기대값 %q", np.FlowRef, "550e8400-e29b-41d4-a716-446655440000")
	}

	if np.NodeRef != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("NodeRef = %q, 기대값 %q", np.NodeRef, "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	}
}
