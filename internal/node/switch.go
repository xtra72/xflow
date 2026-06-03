package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// switchMatchMode 는 스위치 노드의 매칭 처리 모드를 나타낸다.
type switchMatchMode int

const (
	// matchFirst 는 순차 평가하여 첫 번째 매칭 라우트로만 라우팅한다 (기본).
	matchFirst switchMatchMode = iota
	// matchAll 은 매칭되는 모든 라우트로 메시지를 팬아웃한다.
	matchAll
)

// switchPassMode 는 라우팅 시 메시지를 복제(Clone)할지 원본을 그대로
// 전달할지 결정하는 모드를 나타낸다.
type switchPassMode int

const (
	// passCopy 는 라우팅 전에 메시지를 Clone 한다 (기본).
	// Clone 은 ID 를 새로 생성하므로 출력 메시지는 입력과 다른 식별성을 갖는다.
	passCopy switchPassMode = iota
	// passOriginal 은 단일 방출 경로에서 원본 메시지 객체를 그대로 전달한다.
	// 원본의 _target_port 메타데이터를 설정하므로 ID/식별성이 하류로 보존된다.
	passOriginal
)

// SwitchRoute 는 스위치 노드의 라우팅 규칙을 정의하는 구조체이다.
// Condition이 true를 반환하면 메시지가 TargetPort로 전달된다.
type SwitchRoute struct {
	Condition  func(msg message.Message) bool
	TargetPort string
}

// SwitchNode 는 조건에 따라 메시지를 다른 포트로 라우팅하는 노드이다.
// 라우트를 순서대로 평가하여 첫 번째 매칭되는 라우트의 TargetPort로 전달한다(first 모드).
// all 모드에서는 매칭되는 모든 라우트의 포트로 메시지를 팬아웃한다.
type SwitchNode struct {
	*BaseNode
	routes      []SwitchRoute
	defaultPort string
	matchMode   switchMatchMode
	passMode    switchPassMode
	mu          sync.RWMutex
}

// 인터페이스 컴파일 체크
var _ Node = (*SwitchNode)(nil)

// NewSwitchNode 는 새로운 SwitchNode를 생성하는 팩토리 함수이다.
func NewSwitchNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SwitchNode{
		BaseNode:  base,
		matchMode: matchFirst,
		passMode:  passCopy,
	}
	return n, nil
}

// Init 은 SwitchNode를 초기화한다.
func (n *SwitchNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 라우트를 평가하여 메시지를 라우팅한다.
//
// first 모드: 라우트를 순서대로 평가하여 첫 번째 매칭되는 라우트의 포트로만
//
//	메시지 1건을 라우팅한다.
//
// all 모드: 매칭되는 모든 라우트의 포트로 각각 메시지를 팬아웃한다.
//
// 매칭이 하나도 없으면(두 모드 공통):
//   - defaultPort가 설정되어 있으면 기본 포트로 1건을 라우팅한다.
//   - defaultPort가 비어 있으면 빈 슬라이스를 반환한다 (드롭).
//
// pass_mode 에 따른 복제 규칙:
//   - copy(기본): 모든 경로에서 메시지를 Clone 하여 라우팅한다(원본 불변).
//   - original: 정확히 1건만 방출하는 경로(first 매칭, all 단일 매칭, default)에서는
//     원본 메시지에 직접 _target_port 를 설정하여 그대로 전달한다(ID/식별성 보존).
//
// 단, all 모드에서 2건 이상 매칭되면 pass_mode 와 무관하게 강제로 Clone 한다.
// 단일 객체는 서로 다른 포트의 _target_port 값을 동시에 가질 수 없기 때문이다.
//
// 방출 메시지에는 "_target_port" 메타데이터에 대상 포트 이름이 설정된다.
func (n *SwitchNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	routes := n.routes
	defPort := n.defaultPort
	mode := n.matchMode
	pmode := n.passMode
	n.mu.RUnlock()

	switch mode {
	case matchAll:
		// 매칭되는 라우트를 먼저 수집한다 (라우트 순서 보존).
		matched := make([]SwitchRoute, 0, len(routes))
		for _, route := range routes {
			if route.Condition(msg) {
				matched = append(matched, route)
			}
		}

		// 매칭 0건 → default/드롭 규칙 적용.
		if len(matched) == 0 {
			return n.routeDefault(msg, defPort, pmode), nil
		}

		// original + 단일 매칭 → 원본 그대로 전달(Clone 없음).
		if pmode == passOriginal && len(matched) == 1 {
			msg.Metadata().Set("_target_port", matched[0].TargetPort)
			return []message.Message{msg}, nil
		}

		// 그 외(copy, 또는 original + 다중 매칭) → 각 매칭마다 Clone.
		// 다중 매칭은 단일 객체가 서로 다른 _target_port 를 동시에 가질 수
		// 없으므로 original 이어도 강제로 Clone 한다.
		outs := make([]message.Message, 0, len(matched))
		for _, route := range matched {
			out := msg.Clone()
			out.Metadata().Set("_target_port", route.TargetPort)
			outs = append(outs, out)
		}
		return outs, nil

	default: // matchFirst
		for _, route := range routes {
			if route.Condition(msg) {
				if pmode == passOriginal {
					// 원본 그대로 전달(Clone 없음).
					msg.Metadata().Set("_target_port", route.TargetPort)
					return []message.Message{msg}, nil
				}
				out := msg.Clone()
				out.Metadata().Set("_target_port", route.TargetPort)
				return []message.Message{out}, nil
			}
		}
		return n.routeDefault(msg, defPort, pmode), nil
	}
}

// routeDefault 는 미매칭 시 기본 포트 라우팅 또는 드롭을 처리한다.
// defPort가 비어 있으면 빈 슬라이스를 반환한다 (드롭).
// defPort가 비어 있지 않으면 1건을 기본 포트로 라우팅하며,
// pass_mode 에 따라 원본을 그대로 보내거나(original) Clone 한다(copy).
func (n *SwitchNode) routeDefault(msg message.Message, defPort string, pmode switchPassMode) []message.Message {
	if defPort == "" {
		return []message.Message{}
	}
	if pmode == passOriginal {
		// default 방출은 단일 건이므로 원본 그대로 전달(ID 보존).
		msg.Metadata().Set("_target_port", defPort)
		return []message.Message{msg}
	}
	out := msg.Clone()
	out.Metadata().Set("_target_port", defPort)
	return []message.Message{out}
}

// Shutdown 은 SwitchNode를 종료한다.
func (n *SwitchNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 SwitchNode의 설정을 적용한다.
//
// 설정 키:
//   - "routes": 라우팅 규칙 배열. 두 가지 형식을 모두 허용한다.
//   - []SwitchRoute (Go 함수 클로저) — 기존 동작 보존(하위 호환).
//   - []any (각 원소가 map[string]any{"name": string, "condition": string})
//     — 각 condition을 compileCondition으로 컴파일하여 라우트를 구성한다.
//     배열 순서를 보존한다.
//   - "match_mode": "first"(기본) 또는 "all". 미설정·미인식 값은 first로 폴백.
//   - "default_port": 미매칭 시 라우팅할 포트 이름. 비면 드롭.
//   - "pass_mode": "copy"(기본) 또는 "original". 미설정·빈값·미인식 값은 copy로 폴백.
//     copy 는 라우팅 시 메시지를 Clone 하고, original 은 단일 방출 경로에서
//     원본을 그대로 전달하여 메시지 ID/식별성을 보존한다.
//
// 파싱은 원자적으로 적용한다: routes 파싱 중 에러가 발생하면 기존 설정을
// 변경하지 않고 에러를 반환한다(부분 적용 금지).
func (n *SwitchNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// routes를 먼저 파싱하여 에러를 선검출한다(부분 적용 방지).
	// routes 키가 없으면 nil을 유지하여 기존 라우트를 보존한다.
	var newRoutes []SwitchRoute
	routesProvided := false
	if raw, ok := config["routes"]; ok {
		routesProvided = true
		parsed, err := parseSwitchRoutes(raw)
		if err != nil {
			return err
		}
		newRoutes = parsed
	}

	// match_mode 파싱 (미설정·미인식 값은 first로 폴백).
	mode := matchFirst
	if v, ok := configString(config, "match_mode"); ok && v == "all" {
		mode = matchAll
	}

	// default_port 파싱.
	defPort := ""
	if s, ok := configString(config, "default_port"); ok {
		defPort = s
	}

	// pass_mode 파싱 (미설정·빈값·미인식 값은 copy로 폴백).
	pmode := passCopy
	if v, ok := configString(config, "pass_mode"); ok && v == "original" {
		pmode = passOriginal
	}

	// 원자적 설정 적용.
	n.mu.Lock()
	if routesProvided {
		n.routes = newRoutes
	}
	n.matchMode = mode
	n.defaultPort = defPort
	n.passMode = pmode
	n.mu.Unlock()

	return nil
}

// parseSwitchRoutes 는 config["routes"] 원시 값을 []SwitchRoute로 파싱한다.
//
// 허용 형식:
//   - []SwitchRoute: Go 함수 클로저 형식을 그대로 반환한다 (하위 호환).
//   - []any: 각 원소를 map[string]any{"name","condition"}로 해석하고,
//     condition을 compileCondition으로 컴파일한다. 배열 순서를 보존한다.
//
// 검증:
//   - name이 비어 있거나 문자열이 아니면 에러.
//   - condition이 비어 있거나 문자열이 아니면 에러.
//   - condition이 유효하지 않은 표현식이면 에러(실패한 라우트 식별 가능).
//
// 그 외 타입은 빈 슬라이스를 반환한다(무시).
func parseSwitchRoutes(raw any) ([]SwitchRoute, error) {
	// 1순위: Go 함수 클로저 형식 (기존 동작 보존).
	if r, ok := raw.([]SwitchRoute); ok {
		return r, nil
	}

	// 2순위: []any (각 원소가 {name, condition} 맵).
	arr, ok := raw.([]any)
	if !ok {
		// 알 수 없는 타입 → 라우트 없음(무시).
		return nil, nil
	}

	routes := make([]SwitchRoute, 0, len(arr))
	for i, elem := range arr {
		m, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("switch configure: 라우트[%d]: 객체 형식이 아닙니다 (got %T)", i, elem)
		}

		// name 검증.
		nameRaw, hasName := m["name"]
		name, ok := nameRaw.(string)
		if !hasName || !ok {
			return nil, fmt.Errorf("switch configure: 라우트[%d]: name이 문자열이 아닙니다", i)
		}
		if name == "" {
			return nil, fmt.Errorf("switch configure: 라우트[%d]: name이 비어 있습니다", i)
		}

		// condition 검증.
		condRaw, hasCond := m["condition"]
		condStr, ok := condRaw.(string)
		if !hasCond || !ok {
			return nil, fmt.Errorf("switch configure: 라우트[%d] %q: condition이 문자열이 아닙니다", i, name)
		}
		if condStr == "" {
			return nil, fmt.Errorf("switch configure: 라우트[%d] %q: condition이 비어 있습니다", i, name)
		}

		// 표현식 컴파일 (실패 시 라우트 식별 정보를 포함하여 에러 반환).
		cond, err := compileCondition(condStr)
		if err != nil {
			return nil, fmt.Errorf("switch configure: 라우트[%d] %q: %w", i, name, err)
		}

		routes = append(routes, SwitchRoute{
			Condition:  cond,
			TargetPort: name,
		})
	}

	return routes, nil
}

// Ports 는 SwitchNode의 포트 목록을 동적으로 파생한다.
//
// 파생 규칙:
//   - routes가 비어 있으면 BaseNode.Ports()로 폴백한다 (정적 [in, out, _error]).
//     프론트엔드 computePortsForNode의 [in, out] 폴백과 정렬되며, 기존 플로우
//     동작을 보존한다(REQ-SWITCH-050).
//   - routes가 있으면 입력 "in" + 각 route.TargetPort(출력, 순서 보존, 중복 제거)
//   - default_port(설정된 경우, 중복 아닌 경우)를 반환한다.
//   - 에러 포트는 BaseNode의 errorPort를 그대로 포함한다.
//
// route.TargetPort = 포트명 = 와이어 SourcePort = _target_port 규약이 성립하며,
// default 출력 포트 이름은 고정 "default"가 아니라 default_port 값 자체이다.
func (n *SwitchNode) Ports() []NodePort {
	n.mu.RLock()
	routes := n.routes
	defPort := n.defaultPort
	n.mu.RUnlock()

	// routes가 비면 BaseNode 정적 포트로 폴백 (in/out/_error).
	if len(routes) == 0 {
		return n.BaseNode.Ports()
	}

	ports := make([]NodePort, 0, len(routes)+2)

	// 입력 포트 "in".
	ports = append(ports, NodePort{
		ID:        "in",
		Name:      "in",
		Direction: flow.PortInput,
	})

	// 라우트별 출력 포트 (순서 보존, 중복 제거).
	seen := make(map[string]bool, len(routes)+1)
	for _, route := range routes {
		name := route.TargetPort
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		ports = append(ports, NodePort{
			ID:        name,
			Name:      name,
			Direction: flow.PortOutput,
		})
	}

	// default_port 출력 포트 (설정된 경우, 라우트와 중복 아닌 경우).
	if defPort != "" && !seen[defPort] {
		seen[defPort] = true
		ports = append(ports, NodePort{
			ID:        defPort,
			Name:      defPort,
			Direction: flow.PortOutput,
		})
	}

	// 에러 포트 (BaseNode와 동일하게 포함).
	if ep := n.BaseNode.GetErrorPort(); ep != nil {
		ports = append(ports, *ep)
	}

	return ports
}
