package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// makeFlowWithRefs 는 지정 id 를 가지며, refFlowIDs 각각을 참조하는 flow-node 들을 보유한
// 플로우를 생성한다. FlowFromJSON 으로 id 를 고정한다(NewFlow 는 무작위 id 생성).
func makeFlowWithRefs(t *testing.T, id string, refFlowIDs ...string) flow.Flow {
	t.Helper()

	nodes := make([]map[string]any, 0, len(refFlowIDs))
	for i, ref := range refFlowIDs {
		nodes = append(nodes, map[string]any{
			"id":   "fn-" + ref,
			"type": "flow-node",
			"name": "subflow",
			"config": map[string]any{
				"flow_id": ref,
			},
			// 노드 인덱스로 잠재적 이름 충돌만 방지(검출 로직과 무관).
			"_idx": i,
		})
	}

	def := map[string]any{
		"id":    id,
		"name":  id,
		"nodes": nodes,
		"wires": []any{},
	}

	data, err := json.Marshal(def)
	require.NoError(t, err)

	f, err := flow.FlowFromJSON(data)
	require.NoError(t, err)
	require.Equal(t, id, f.ID())
	return f
}

// TestDetectCycle_자기참조_거부 는 flow-node 가 자신의 플로우를 참조하면 거부되는지 확인한다
// (REQ-SUBFLOW-E01).
func TestDetectCycle_자기참조_거부(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	a := makeFlowWithRefs(t, "A", "A") // A 가 자기 자신을 참조

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
	assert.Contains(t, err.Error(), "A → A")
}

// TestDetectCycle_간접순환_ABA_거부 는 A→B→A 순환이 경로와 함께 거부되는지 확인한다
// (REQ-SUBFLOW-E02).
func TestDetectCycle_간접순환_ABA_거부(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// B 는 저장소에 있고 A 를 참조한다. A 는 (저장 전) B 를 참조하는 def 로 검사한다.
	b := makeFlowWithRefs(t, "B", "A")
	require.NoError(t, repo.Save(ctx, b))

	a := makeFlowWithRefs(t, "A", "B")

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
	assert.Contains(t, err.Error(), "A → B → A")
}

// TestDetectCycle_정상DAG_허용 은 A→B→C 비순환 그래프가 통과하는지 확인한다.
func TestDetectCycle_정상DAG_허용(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	c := makeFlowWithRefs(t, "C") // 말단(참조 없음)
	require.NoError(t, repo.Save(ctx, c))
	b := makeFlowWithRefs(t, "B", "C")
	require.NoError(t, repo.Save(ctx, b))

	a := makeFlowWithRefs(t, "A", "B")

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	assert.NoError(t, err)
}

// TestDetectCycle_누락참조_명확한에러 는 참조 플로우가 저장소에 없으면 명확한 에러가
// 반환되는지 확인한다(panic 금지, REQ-SUBFLOW-F03).
func TestDetectCycle_누락참조_명확한에러(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	a := makeFlowWithRefs(t, "A", "MISSING") // MISSING 은 저장소에 없음

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "referenced flow not found")
	assert.Contains(t, err.Error(), "MISSING")
}

// TestDetectCycle_깊이상한_초과시에러 는 참조 체인이 안전 상한을 초과하면 에러가
// 반환되는지 확인한다(REQ-SUBFLOW-N01).
func TestDetectCycle_깊이상한_초과시에러(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// L0 → L1 → ... → L10 (깊이 10 > 상한 8) 의 선형 비순환 체인을 만든다.
	// 말단을 먼저 저장하고 역순으로 연결한다.
	const chainLen = 11
	last := "L10"
	require.NoError(t, repo.Save(ctx, makeFlowWithRefs(t, last)))
	for i := chainLen - 2; i >= 1; i-- {
		id := "L" + itoa(i)
		next := "L" + itoa(i+1)
		require.NoError(t, repo.Save(ctx, makeFlowWithRefs(t, id, next)))
	}
	root := makeFlowWithRefs(t, "L0", "L1")

	err := DetectFlowReferenceCycle(ctx, "L0", root, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "깊이 초과")
}

// makeFlowWithModedRefs 는 (refFlowID, mode) 쌍 각각을 참조하는 flow-node 들을 보유한 플로우를
// 생성한다(SPEC-SUBFLOW-002 순환 검출 — 모드 혼합 그래프 검증용).
func makeFlowWithModedRefs(t *testing.T, id string, refs map[string]string) flow.Flow {
	t.Helper()
	nodes := make([]map[string]any, 0, len(refs))
	i := 0
	for ref, mode := range refs {
		nodes = append(nodes, map[string]any{
			"id":   "fn-" + ref,
			"type": "flow-node",
			"name": "subflow",
			"config": map[string]any{
				"flow_id": ref,
				"mode":    mode,
			},
			"_idx": i,
		})
		i++
	}
	def := map[string]any{"id": id, "name": id, "nodes": nodes, "wires": []any{}}
	data, err := json.Marshal(def)
	require.NoError(t, err)
	f, err := flow.FlowFromJSON(data)
	require.NoError(t, err)
	require.Equal(t, id, f.ID())
	return f
}

// TestDetectCycle_shared_자기참조_거부 는 mode=shared 직접 자기참조도 거부되는지 검증한다
// (SPEC-SUBFLOW-002 CY01 — shared 순환 거부).
func TestDetectCycle_shared_자기참조_거부(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	a := makeFlowWithModedRefs(t, "A", map[string]string{"A": "shared"})

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
	assert.Contains(t, err.Error(), "A → A")
}

// TestDetectCycle_shared_ABA_거부 는 A(shared)→B, B(shared)→A 연결 순환이 거부되는지
// 검증한다(CY01 — shared A↔B).
func TestDetectCycle_shared_ABA_거부(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	b := makeFlowWithModedRefs(t, "B", map[string]string{"A": "shared"})
	require.NoError(t, repo.Save(ctx, b))
	a := makeFlowWithModedRefs(t, "A", map[string]string{"B": "shared"})

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
	assert.Contains(t, err.Error(), "A → B → A")
}

// TestDetectCycle_혼합모드_순환_거부 는 shared/instance 가 섞인 순환(A shared→B, B instance→A)도
// 검출되는지 검증한다(CY02 — 혼합 그래프 순환).
func TestDetectCycle_혼합모드_순환_거부(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	b := makeFlowWithModedRefs(t, "B", map[string]string{"A": "instance"})
	require.NoError(t, repo.Save(ctx, b))
	a := makeFlowWithModedRefs(t, "A", map[string]string{"B": "shared"})

	err := DetectFlowReferenceCycle(ctx, "A", a, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
}

// itoa 는 작은 정수의 문자열 변환 헬퍼이다(테스트 전용).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
