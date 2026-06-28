package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/dto"
)

// TestFlowServiceAdapter_GetFlow_RunningFlowRepoNameWins 는 실행 중인(배포된) 플로우의
// 이름이 저장소에서 변경된 경우, GetFlow 가 저장소 이름(최신)을 반환하는지 검증한다.
//
// 재현 시나리오 (실서버 버그):
//  1. 플로우를 "OLD" 이름으로 생성/배포 → 엔진이 "OLD" 이름의 배포 객체를 보유.
//  2. 이름만 "NEW" 로 변경하여 저장소에만 저장(UpdateFlow). 엔진 배포 객체는 여전히 "OLD".
//  3. GetFlow 가 엔진 상태 기반으로 info 를 만들면 이름이 "OLD" 로 되돌아간다(버그).
//
// 기대: 저장소가 이름/설명의 source of truth 이므로 GetFlow 는 "NEW" 를 반환해야 하며,
// 동시에 running 런타임 필드(status 등)는 그대로 유지되어야 한다.
func TestFlowServiceAdapter_GetFlow_RunningFlowRepoNameWins(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)

	// 1. "OLD" 이름으로 플로우 생성
	created, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:        "OLD",
		Description: "원래 설명",
		Definition:  map[string]any{"name": "OLD", "nodes": []any{}, "wires": []any{}},
	})
	require.NoError(t, err, "create")

	// 2. 배포 + 시작하여 엔진이 "OLD" 이름의 배포 객체를 보유하도록 한다.
	require.NoError(t, adapter.StartFlow(ctx, created.ID), "start")

	// 시작 직후에는 엔진 상태가 source 이므로 이름이 "OLD" 이어야 한다(전제 확인).
	running, err := adapter.GetFlow(ctx, created.ID)
	require.NoError(t, err, "get after start")
	require.Equal(t, "running", running.Status, "배포/시작 후 running 상태여야 함")
	require.Equal(t, "OLD", running.Name, "전제: 이름 변경 전에는 OLD")

	// 3. 이름만 "NEW" 로 변경 → 저장소에는 반영되지만 엔진 배포 객체는 그대로 "OLD".
	newName := "NEW"
	newDesc := "변경된 설명"
	_, err = adapter.UpdateFlow(ctx, created.ID, &dto.FlowUpdateRequest{
		Name:        &newName,
		Description: &newDesc,
	})
	require.NoError(t, err, "update name")

	// 4. 핵심 검증: GetFlow 는 저장소의 최신 이름/설명을 반환해야 한다(엔진 OLD 가 아님).
	got, err := adapter.GetFlow(ctx, created.ID)
	require.NoError(t, err, "get after rename")
	assert.Equal(t, "NEW", got.Name, "실행 중에도 저장소 이름(NEW)이 우선되어야 함")
	assert.Equal(t, "변경된 설명", got.Description, "실행 중에도 저장소 설명이 우선되어야 함")

	// 런타임 필드는 그대로 유지되어야 한다(여전히 running).
	assert.Equal(t, "running", got.Status, "이름 변경이 런타임 상태에 영향을 주면 안 됨")
}

// TestFlowServiceAdapter_GetFlow_StoredFlowRepoNameRegression 는 미배포(stored) 플로우의
// 이름 변경이 여전히 정상 반영되는지(회귀 방지) 검증한다.
func TestFlowServiceAdapter_GetFlow_StoredFlowRepoNameRegression(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	created, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:       "stored-old",
		Definition: map[string]any{"name": "stored-old", "nodes": []any{}, "wires": []any{}},
	})
	require.NoError(t, err, "create")

	newName := "stored-new"
	_, err = adapter.UpdateFlow(ctx, created.ID, &dto.FlowUpdateRequest{Name: &newName})
	require.NoError(t, err, "update")

	got, err := adapter.GetFlow(ctx, created.ID)
	require.NoError(t, err, "get")
	assert.Equal(t, "stored-new", got.Name, "미배포 플로우는 저장소 이름을 그대로 반환해야 함")
	assert.Equal(t, "stored", got.Status, "미배포 플로우는 stored 상태여야 함")
}

// TestFlowServiceAdapter_ListFlows_RunningFlowRepoNameWins 는 실행 중인 플로우의 이름이
// 저장소에서 변경된 경우, ListFlows 가 저장소 이름(최신)을 반환하는지 검증한다.
// (플로우 목록/스위처가 변경된 이름을 표시해야 함)
func TestFlowServiceAdapter_ListFlows_RunningFlowRepoNameWins(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)

	created, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:       "list-old",
		Definition: map[string]any{"name": "list-old", "nodes": []any{}, "wires": []any{}},
	})
	require.NoError(t, err, "create")
	require.NoError(t, adapter.StartFlow(ctx, created.ID), "start")

	// 이름만 변경 → 저장소에만 반영.
	newName := "list-new"
	_, err = adapter.UpdateFlow(ctx, created.ID, &dto.FlowUpdateRequest{Name: &newName})
	require.NoError(t, err, "update")

	flows, total, err := adapter.ListFlows(ctx, dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 20},
	})
	require.NoError(t, err, "list")
	require.Equal(t, int64(1), total, "플로우 1개여야 함")
	require.Len(t, flows, 1)

	assert.Equal(t, "list-new", flows[0].Name, "목록에서도 저장소 이름(최신)이 우선되어야 함")
	assert.Equal(t, "running", flows[0].Status, "런타임 상태는 running 유지")
}
