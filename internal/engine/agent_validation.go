package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
)

// validateAgentRefs 는 플로우의 모든 노드가 참조하는 에이전트가 실제로 존재하며
// 활성화 상태인지 검증한다. agentManager 가 설정되지 않은 경우 검증을 건너뛴다
// (하위 호환성 유지).
//
// 각 노드의 AgentRef 에 대해 두 가지 검증을 수행한다:
//  1. 매니저에서 조회 가능한지 (존재 검증)
//  2. 조회된 AgentConfig.IsEnabled() 가 true 인지 (활성화 검증, R5.1, R5.2)
//
// 한 패스에서 누락(missing) 과 비활성화(disabled) 케이스를 모두 수집하여
// 보고한다 (R5.6 - early return 대신 누적). 두 클래스의 에러가 동시에 존재하면
// errors.Join 으로 묶어 반환하므로 호출자는 errors.Is 로 양쪽 모두 감지할 수 있다.
func (e *Engine) validateAgentRefs(f flow.Flow) error {
	if e.agentManager == nil {
		return nil // 검증 비활성화
	}

	var missing []missingAgentRef
	var disabled []disabledAgentRef
	for _, nd := range f.Nodes() {
		if nd.AgentRef == nil {
			continue
		}
		// 1. 존재 여부 확인
		cfg, found := lookupAgentConfig(e.agentManager, *nd.AgentRef)
		if !found {
			missing = append(missing, missingAgentRef{
				NodeID:   nd.ID,
				NodeName: nd.Name,
				NodeType: nd.Type,
				Ref:      *nd.AgentRef,
			})
			continue
		}
		// 2. 활성화 상태 확인 (R5.1, R5.2)
		// Enabled 가 nil 이면 기본 true 로 취급되므로 하위 호환성이 유지된다 (R7.1).
		if !cfg.IsEnabled() {
			disabled = append(disabled, disabledAgentRef{
				NodeID:   nd.ID,
				NodeName: nd.Name,
				NodeType: nd.Type,
				Ref:      *nd.AgentRef,
			})
		}
	}

	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", ErrAgentRefNotFound, formatMissingAgentError(e.agentManager, missing)))
	}
	if len(disabled) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", ErrAgentDisabled, formatDisabledAgentError(disabled)))
	}
	return errors.Join(errs...) // 빈 slice 는 nil, 단일 에러는 그대로, 다중 에러는 joined
}

// missingAgentRef 는 검증 실패한 단일 에이전트 참조(존재하지 않음)를 기록한다.
type missingAgentRef struct {
	NodeID   string
	NodeName string
	NodeType string
	Ref      flow.AgentRef
}

// disabledAgentRef 는 검증 실패한 단일 에이전트 참조(비활성화 상태)를 기록한다.
type disabledAgentRef struct {
	NodeID   string
	NodeName string
	NodeType string
	Ref      flow.AgentRef
}

// lookupAgentConfig 는 에이전트 매니저에서 AgentRef 에 해당하는 AgentConfig 를 반환한다.
// 조회 순서는 agentRefExists 와 동일하게 ID 우선, 이름 폴백이다.
// 조회 실패 시 두 번째 반환값이 false 이다.
func lookupAgentConfig(mgr agent.Manager, ref flow.AgentRef) (agent.AgentConfig, bool) {
	if ref.AgentID != "" {
		if a, err := mgr.Get(ref.AgentID); err == nil {
			return a.Info().Config, true
		}
	}
	if ref.AgentName != "" {
		for _, a := range mgr.List() {
			if a.Name() == ref.AgentName {
				return a.Info().Config, true
			}
		}
	}
	return agent.AgentConfig{}, false
}

// agentRefExists 는 에이전트 매니저에서 AgentRef 가 조회 가능한지 확인한다.
// 활성화 상태는 고려하지 않고 존재 여부만 검사한다. 일부 테스트에서 직접 호출되므로
// 유지된다. 내부 검증 경로는 lookupAgentConfig 를 사용한다.
//
// 조회 순서:
//  1. AgentID 로 정확히 검색
//  2. AgentName 으로 목록 검색 (폴백)
func agentRefExists(mgr agent.Manager, ref flow.AgentRef) bool {
	_, ok := lookupAgentConfig(mgr, ref)
	return ok
}

// formatMissingAgentError 는 누락된 에이전트 참조에 대한 사용자 친화적 에러 메시지를 생성한다.
// 각 누락 항목에 대해 노드 이름과 참조된 에이전트를 표시하고,
// 현재 등록된 에이전트 목록을 제안으로 함께 제공한다.
func formatMissingAgentError(mgr agent.Manager, missing []missingAgentRef) string {
	var sb strings.Builder

	// 첫 번째 항목 간단히 요약
	first := missing[0]
	refLabel := formatAgentRefLabel(first.Ref)
	sb.WriteString(fmt.Sprintf("node %q (type=%q) references agent %s which does not exist",
		first.NodeName, first.NodeType, refLabel))

	// 추가 항목
	if len(missing) > 1 {
		sb.WriteString(fmt.Sprintf(" (and %d more)", len(missing)-1))
	}

	// 현재 등록된 에이전트 목록
	available := listAvailableAgents(mgr)
	if len(available) > 0 {
		sb.WriteString(". available agents: ")
		sb.WriteString(strings.Join(available, ", "))
	} else {
		sb.WriteString(". no agents are currently registered")
	}

	return sb.String()
}

// formatDisabledAgentError 는 비활성화된 에이전트 참조에 대한 사용자 친화적 에러 메시지를 생성한다.
// R5.3 에 따라 각 항목에 대해 에이전트 식별자, 참조 노드, Enable API 안내를 포함한다.
// 모든 disabled 항목을 포함하여 세미콜론으로 구분한다 (R5.6).
func formatDisabledAgentError(disabled []disabledAgentRef) string {
	var sb strings.Builder
	for i, d := range disabled {
		if i > 0 {
			sb.WriteString("; ")
		}
		refLabel := formatAgentRefLabel(d.Ref)
		// Enable API 경로에 사용할 식별자: ID 우선, 없으면 이름 폴백.
		id := d.Ref.AgentID
		if id == "" {
			id = d.Ref.AgentName
		}
		sb.WriteString(fmt.Sprintf(
			"node %q (type=%q) references disabled agent %s: call POST /agents/%s/enable to activate",
			d.NodeName, d.NodeType, refLabel, id,
		))
	}
	return sb.String()
}

// formatAgentRefLabel 은 AgentRef를 사람이 읽기 쉬운 형태로 변환한다.
func formatAgentRefLabel(ref flow.AgentRef) string {
	switch {
	case ref.AgentName != "" && ref.AgentID != "" && ref.AgentName != ref.AgentID:
		return fmt.Sprintf("%q (id=%s)", ref.AgentName, ref.AgentID)
	case ref.AgentName != "":
		return fmt.Sprintf("%q", ref.AgentName)
	case ref.AgentID != "":
		return fmt.Sprintf("(id=%s)", ref.AgentID)
	default:
		return "<empty>"
	}
}

// listAvailableAgents 는 현재 매니저에 등록된 에이전트 이름 목록을 정렬하여 반환한다.
func listAvailableAgents(mgr agent.Manager) []string {
	agents := mgr.List()
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name())
	}
	sort.Strings(names)
	return names
}
