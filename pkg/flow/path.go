package flow

import "strings"

// NodePath 는 플로우 내 특정 노드를 가리키는 주소 구조체이다.
// FlowRef 는 플로우 식별자(ID 또는 Name)를, NodeRef 는 노드 식별자(ID 또는 Name)를 나타낸다.
type NodePath struct {
	FlowRef string // 플로우 식별자 (ID 또는 Name)
	NodeRef string // 노드 식별자 (ID 또는 Name)
}

// ParseNodePath 는 dot 표기법 문자열을 파싱하여 NodePath를 반환한다.
// 이름에 dot이 포함된 경우 bracket([])으로 감싸서 이스케이프한다.
//
// 예시:
//   - "flow1.node1"           → NodePath{FlowRef: "flow1", NodeRef: "node1"}
//   - "[flow.name].nodeName"  → NodePath{FlowRef: "flow.name", NodeRef: "nodeName"}
//   - "[flow.a].[node.b]"     → NodePath{FlowRef: "flow.a", NodeRef: "node.b"}
//
// 빈 문자열, 구분자 없음, 빈 FlowRef/NodeRef, 닫히지 않은 bracket 등은
// ErrInvalidPath 를 반환한다.
func ParseNodePath(path string) (NodePath, error) {
	if path == "" {
		return NodePath{}, ErrInvalidPath
	}

	// 상태 기반 파서: 첫 번째 파트(FlowRef)를 파싱한 후,
	// dot 구분자를 확인하고, 두 번째 파트(NodeRef)를 파싱한다.
	flowRef, rest, err := parsePart(path)
	if err != nil {
		return NodePath{}, ErrInvalidPath
	}

	// 나머지 문자열이 없으면 구분자가 없는 것이다
	if rest == "" {
		return NodePath{}, ErrInvalidPath
	}

	// 구분자(dot)를 소비한다
	if rest[0] != '.' {
		return NodePath{}, ErrInvalidPath
	}
	rest = rest[1:]

	// 두 번째 파트(NodeRef)를 파싱한다
	nodeRef, remaining, err := parsePart(rest)
	if err != nil {
		return NodePath{}, ErrInvalidPath
	}

	// 파싱 후 남은 문자열이 있으면 유효하지 않다
	if remaining != "" {
		return NodePath{}, ErrInvalidPath
	}

	// FlowRef 또는 NodeRef가 비어있으면 유효하지 않다
	if flowRef == "" || nodeRef == "" {
		return NodePath{}, ErrInvalidPath
	}

	return NodePath{FlowRef: flowRef, NodeRef: nodeRef}, nil
}

// parsePart 는 경로 문자열에서 하나의 파트를 파싱한다.
// bracket 이스케이프를 지원하며, 파싱된 값과 나머지 문자열을 반환한다.
//
// 상태 머신:
//   - Normal 상태: '.'을 만나면 파싱을 종료하고, '['을 만나면 InBracket 상태로 전환한다.
//   - InBracket 상태: ']'을 만나면 Normal 상태로 복귀하고, '.'은 리터럴 문자로 처리한다.
func parsePart(s string) (part string, rest string, err error) {
	if s == "" {
		return "", "", nil
	}

	var b strings.Builder
	i := 0
	inBracket := false

	if s[0] == '[' {
		inBracket = true
		i = 1 // '[' 건너뛰기
	}

	for i < len(s) {
		ch := s[i]

		if inBracket {
			if ch == ']' {
				inBracket = false
				i++
				break
			}
			b.WriteByte(ch)
			i++
		} else {
			if ch == '.' {
				break
			}
			b.WriteByte(ch)
			i++
		}
	}

	// bracket이 닫히지 않았으면 에러
	if inBracket {
		return "", "", ErrInvalidPath
	}

	return b.String(), s[i:], nil
}

// String 은 NodePath를 dot 표기법 문자열로 변환한다.
// FlowRef 또는 NodeRef에 dot이 포함되어 있으면 bracket([])으로 감싸서 이스케이프한다.
//
// 예시:
//   - {FlowRef: "flow1", NodeRef: "node1"}       → "flow1.node1"
//   - {FlowRef: "flow.name", NodeRef: "node1"}   → "[flow.name].node1"
//   - {FlowRef: "flow.a", NodeRef: "node.b"}     → "[flow.a].[node.b]"
func (np NodePath) String() string {
	var b strings.Builder

	if strings.Contains(np.FlowRef, ".") {
		b.WriteByte('[')
		b.WriteString(np.FlowRef)
		b.WriteByte(']')
	} else {
		b.WriteString(np.FlowRef)
	}

	b.WriteByte('.')

	if strings.Contains(np.NodeRef, ".") {
		b.WriteByte('[')
		b.WriteString(np.NodeRef)
		b.WriteByte(']')
	} else {
		b.WriteString(np.NodeRef)
	}

	return b.String()
}
