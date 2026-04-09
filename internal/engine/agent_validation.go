package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
)

// validateAgentRefs 는 플로우의 모든 노드가 참조하는 에이전트가 실제로 존재하는지 검증한다.
// agentManager가 설정되지 않은 경우 검증을 건너뛴다 (하위 호환성 유지).
//
// 각 노드의 AgentRef를 확인하고, 매니저에서 조회를 시도한다. 조회 실패 시
// 노드 이름, 참조된 에이전트 이름/ID, 현재 등록된 에이전트 목록을 포함한
// 명확한 에러 메시지를 반환한다.
func (e *Engine) validateAgentRefs(f flow.Flow) error {
	if e.agentManager == nil {
		return nil // 검증 비활성화
	}

	var missing []missingAgentRef
	for _, nd := range f.Nodes() {
		if nd.AgentRef == nil {
			continue
		}
		if !agentRefExists(e.agentManager, *nd.AgentRef) {
			missing = append(missing, missingAgentRef{
				NodeID:   nd.ID,
				NodeName: nd.Name,
				NodeType: nd.Type,
				Ref:      *nd.AgentRef,
			})
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrAgentRefNotFound, formatMissingAgentError(e.agentManager, missing))
}

// missingAgentRef 는 검증 실패한 단일 에이전트 참조를 기록한다.
type missingAgentRef struct {
	NodeID   string
	NodeName string
	NodeType string
	Ref      flow.AgentRef
}

// agentRefExists 는 에이전트 매니저에서 AgentRef가 조회 가능한지 확인한다.
// 1. AgentID로 정확히 검색
// 2. AgentName으로 목록 검색 (폴백)
func agentRefExists(mgr agent.Manager, ref flow.AgentRef) bool {
	if ref.AgentID != "" {
		if _, err := mgr.Get(ref.AgentID); err == nil {
			return true
		}
	}
	if ref.AgentName != "" {
		for _, a := range mgr.List() {
			if a.Name() == ref.AgentName {
				return true
			}
		}
	}
	return false
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
