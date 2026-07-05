// mirror_summary_test.go 는 v1.4(M9) 노드별 운영 요약(미러 파생)을 검증한다
// (@SPEC:SPEC-REMOTE-001 M9, REQ-K10/A15, E06).
//
// 검증 범위:
//   - 미러 행 status 별 카운트(플로우 running/stopped, 에이전트 connected, 디바이스 online).
//   - 빈 노드는 전부 0.
//   - 오프라인 노드여도 미러 행이 보존되므로 last-known 요약 제공(REQ-E06 — 행 삭제 없음).
package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMirror_NodeSummary 는 미러 행 status 분해로 운영 요약을 파생하는지 검증한다(REQ-K10).
func TestMirror_NodeSummary(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	// flow: running x2, stopped x1.
	require.NoError(t, repo.ReplaceFlows(ctx, "n", []MirroredResource{
		{ID: "f1", SourceInstanceID: "n", Kind: "flow", Status: "running"},
		{ID: "f2", SourceInstanceID: "n", Kind: "flow", Status: "running"},
		{ID: "f3", SourceInstanceID: "n", Kind: "flow", Status: "stopped"},
	}))
	// agent: connected x1, disconnected x2.
	require.NoError(t, repo.ReplaceAgents(ctx, "n", []MirroredResource{
		{ID: "a1", SourceInstanceID: "n", Kind: "agent", Status: "connected"},
		{ID: "a2", SourceInstanceID: "n", Kind: "agent", Status: "disconnected"},
		{ID: "a3", SourceInstanceID: "n", Kind: "agent", Status: "disconnected"},
	}))
	// device: online x1, offline x1.
	require.NoError(t, repo.ReplaceDevices(ctx, "n", []MirroredResource{
		{ID: "d1", SourceInstanceID: "n", Kind: "device", Status: "online"},
		{ID: "d2", SourceInstanceID: "n", Kind: "device", Status: "offline"},
	}))

	sum, err := repo.NodeSummary(ctx, "n")
	require.NoError(t, err)

	assert.Equal(t, 3, sum.Flows.Total)
	assert.Equal(t, 2, sum.Flows.Running)
	assert.Equal(t, 1, sum.Flows.Stopped)

	assert.Equal(t, 3, sum.Agents.Total)
	assert.Equal(t, 1, sum.Agents.Connected)

	assert.Equal(t, 2, sum.Devices.Total)
	assert.Equal(t, 1, sum.Devices.Online)
}

// TestMirror_NodeSummaryEmpty 는 미러가 없는 노드의 요약이 전부 0 인지 검증한다.
func TestMirror_NodeSummaryEmpty(t *testing.T) {
	repo := newMirrorRepo(t)
	sum, err := repo.NodeSummary(context.Background(), "empty")
	require.NoError(t, err)
	assert.Equal(t, NodeOperationalSummary{}, sum)
}

// TestMirror_NodeSummaryIsolatedByNode 는 노드별로 요약이 격리되는지 검증한다(출처 태깅).
func TestMirror_NodeSummaryIsolatedByNode(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.ReplaceFlows(ctx, "n1", []MirroredResource{
		{ID: "f1", SourceInstanceID: "n1", Kind: "flow", Status: "running"},
	}))
	require.NoError(t, repo.ReplaceFlows(ctx, "n2", []MirroredResource{
		{ID: "f1", SourceInstanceID: "n2", Kind: "flow", Status: "stopped"},
		{ID: "f2", SourceInstanceID: "n2", Kind: "flow", Status: "stopped"},
	}))

	s1, err := repo.NodeSummary(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, 1, s1.Flows.Total)
	assert.Equal(t, 1, s1.Flows.Running)

	s2, err := repo.NodeSummary(ctx, "n2")
	require.NoError(t, err)
	assert.Equal(t, 2, s2.Flows.Total)
	assert.Equal(t, 2, s2.Flows.Stopped)
}

// TestMirror_NodeSummaryOfflineLastKnown 는 노드가 오프라인이어도(미러 행 보존) 요약이
// last-known 미러로 제공되는지 검증한다(REQ-E06/A15). 미러 행은 online 과 무관하게
// 보존되므로, 요약은 마지막 push 상태를 반영한다.
func TestMirror_NodeSummaryOfflineLastKnown(t *testing.T) {
	repo := newMirrorRepo(t)
	ctx := context.Background()

	// 노드가 온라인일 때 push 한 마지막 상태.
	require.NoError(t, repo.ReplaceFlows(ctx, "n", []MirroredResource{
		{ID: "f1", SourceInstanceID: "n", Kind: "flow", Status: "running"},
		{ID: "f2", SourceInstanceID: "n", Kind: "flow", Status: "stopped"},
	}))

	// (노드 오프라인 — 미러 행은 삭제되지 않음, online 추적은 managed_nodes 가 보유)
	// 요약은 여전히 last-known 미러로 산출되어야 한다.
	sum, err := repo.NodeSummary(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Flows.Total, "오프라인이어도 last-known 미러로 요약 제공")
	assert.Equal(t, 1, sum.Flows.Running)
	assert.Equal(t, 1, sum.Flows.Stopped)
}
