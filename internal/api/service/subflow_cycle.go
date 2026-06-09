package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

// flowNodeType 은 서브플로우 참조 노드의 타입 문자열이다.
// (SPEC-SUBFLOW-001 5.2 데이터 계약 — Type == "flow-node")
const flowNodeType = "flow-node"

// flowNodeFlowIDKey 는 flow-node 가 참조하는 플로우 id 를 담는 Config 키이다.
const flowNodeFlowIDKey = "flow_id"

// maxFlowReferenceDepth 는 플로우 참조 그래프 DFS 의 최대 탐색 깊이이다.
// 순환은 색칠로 검출되지만, 비정상적으로 깊은 합성(또는 검출 누락 방어)에 대비하여
// 안전 상한을 둔다. 상한 초과 시 명확한 에러를 반환한다.
// (SPEC-SUBFLOW-001 REQ-SUBFLOW-N01 — 확장 비용/깊이 안전 상한)
const maxFlowReferenceDepth = 8

// DetectFlowReferenceCycle 은 flow-node 참조로 형성되는 "플로우 → 플로우" 방향 그래프에서
// 순환(자기참조 포함)을 검출한다.
//
//   - flowID: 검사 대상 플로우의 id.
//   - def: flowID 의 (아직 저장되지 않았을 수 있는) 정의. 루트 노드의 참조 간선은 이 def 에서
//     추출한다. 그래프상의 다른 플로우는 repo.Get 으로 조회한다.
//   - repo: 참조 플로우 정의 조회용 저장소.
//
// 검출 항목(SPEC-SUBFLOW-001 그룹 E):
//   - 직접 자기참조: flow-node 의 flow_id 가 자신의 id 와 동일(REQ-SUBFLOW-E01).
//   - 간접 순환: A→B→A, A→B→C→A 등(REQ-SUBFLOW-E02).
//
// 순환이 발견되면 순환 경로(id 시퀀스)를 포함한 에러를 반환한다.
// 참조 플로우가 저장소에 없으면(REQ-SUBFLOW-F03) "referenced flow not found" 에러를 반환한다.
// 탐색 깊이가 maxFlowReferenceDepth 를 초과하면 에러를 반환한다(REQ-SUBFLOW-N01).
func DetectFlowReferenceCycle(ctx context.Context, flowID string, def flow.Flow, repo storage.FlowRepository) error {
	// 루트 플로우의 참조 간선은 전달된 def 에서 추출한다.
	// (저장 전 정의 검증을 위해 repo 가 아닌 def 를 사용해야 한다.)
	rootRefs := referencedFlowIDsFromFlow(def)

	// getRefs 는 주어진 플로우 id 의 참조 대상 id 목록을 반환한다.
	// 루트 id 는 def 기반 참조를 사용하고(저장 전 정의 반영), 그 외는 repo 에서 조회한다.
	// 이로써 "저장되지 않은 루트로 되돌아오는" 역참조(B→A, A 미저장)도 올바르게 검출된다.
	getRefs := func(id string) ([]string, error) {
		if id == flowID {
			return rootRefs, nil
		}
		f, err := repo.Get(ctx, id)
		if err != nil {
			if errors.Is(err, storage.ErrFlowNotFound) {
				return nil, fmt.Errorf("referenced flow not found: %q", id)
			}
			return nil, fmt.Errorf("referenced flow load failed (%q): %w", id, err)
		}
		return referencedFlowIDsFromFlow(f), nil
	}

	// DFS 색칠: gray = 현재 탐색 스택에 있는 노드, black = 탐색 완료.
	const (
		gray  = 1
		black = 2
	)
	color := make(map[string]int)

	var visit func(id string, path []string, depth int) error
	visit = func(id string, path []string, depth int) error {
		if depth > maxFlowReferenceDepth {
			return fmt.Errorf("플로우 참조 깊이 초과(최대 %d): %s", maxFlowReferenceDepth, strings.Join(append(path, id), " → "))
		}

		color[id] = gray
		path = append(path, id)

		refs, err := getRefs(id)
		if err != nil {
			return err
		}

		for _, next := range refs {
			switch color[next] {
			case gray:
				// 회색 노드를 다시 만나면 순환이다. 순환 경로를 구성한다.
				cyclePath := buildCyclePath(path, next)
				return fmt.Errorf("순환 참조: %s", strings.Join(cyclePath, " → "))
			case black:
				// 이미 탐색 완료된 가지 — 순환 없음, 건너뛴다.
				continue
			default:
				if err := visit(next, path, depth+1); err != nil {
					return err
				}
			}
		}

		color[id] = black
		return nil
	}

	return visit(flowID, nil, 0)
}

// buildCyclePath 는 현재 탐색 경로(path)에서 순환의 시작점(back) 이후 구간을 잘라
// 순환을 닫는 경로(… → back)를 만든다. 예) path=[A,B], back=A → [A, B, A].
func buildCyclePath(path []string, back string) []string {
	start := 0
	for i, id := range path {
		if id == back {
			start = i
			break
		}
	}
	cycle := make([]string, 0, len(path)-start+1)
	cycle = append(cycle, path[start:]...)
	cycle = append(cycle, back)
	return cycle
}

// referencedFlowIDsFromFlow 는 플로우 정의에서 flow-node 가 참조하는 플로우 id 목록을
// 추출한다. Type == "flow-node" 인 노드의 Config["flow_id"] 문자열을 수집한다.
// 빈 flow_id 는 무시한다. 중복은 제거한다(동일 대상 다중 참조라도 그래프 간선은 1개).
// (SPEC-SUBFLOW-001 5.2, 5.6)
func referencedFlowIDsFromFlow(f flow.Flow) []string {
	if f == nil {
		return nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, n := range f.Nodes() {
		if n.Type != flowNodeType {
			continue
		}
		flowID, ok := n.Config[flowNodeFlowIDKey].(string)
		if !ok || flowID == "" {
			continue
		}
		// REMOTE(remote://) 참조는 LOCAL 저장소의 플로우가 아니라 라이브 브리지 엔드포인트
		// 이므로(SUBFLOW v1.3 그룹 RB), 로컬 순환 그래프에서 제외한다. repo.Get 으로 조회
		// 하면 "referenced flow not found" 가 되어 배포가 잘못 거부된다. 형식 오류는 배포
		// 시 ExpandSubflows/rewireRemoteBridges 가 별도로 거부한다(REQ-SUBFLOW-R01).
		if _, _, isRemote, _ := parseRemoteFlowRef(flowID); isRemote {
			continue
		}
		if seen[flowID] {
			continue
		}
		seen[flowID] = true
		ids = append(ids, flowID)
	}
	return ids
}
