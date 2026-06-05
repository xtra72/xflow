package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkItem(id, name, kind string, def map[string]any) InventoryItem {
	raw, _ := json.Marshal(def)
	return InventoryItem{ID: id, Name: name, Kind: kind, Definition: raw}
}

// TestApplyExposure_All 은 "all" 정책이 모든 항목을 노출하는지 검증한다(REQ-A04).
func TestApplyExposure_All(t *testing.T) {
	items := []InventoryItem{mkItem("1", "a", KindFlow, nil), mkItem("2", "b", KindFlow, nil)}
	got := applyExposure(ExposeAll, items)
	assert.Len(t, got, 2)
}

// TestApplyExposure_NoneAndEmpty 는 "none"/빈 정책이 전부 비노출인지 검증한다
// (보수적 opt-in 기본 — REQ-A04).
func TestApplyExposure_NoneAndEmpty(t *testing.T) {
	items := []InventoryItem{mkItem("1", "a", KindFlow, nil)}
	assert.Empty(t, applyExposure(ExposeNone, items))
	assert.Empty(t, applyExposure("", items))
	assert.NotNil(t, applyExposure("", items), "빈 결과는 non-nil 슬라이스여야 함")
}

// TestApplyExposure_ExplicitList 는 명시 목록이 ID 또는 Name 매칭으로 필터하는지
// 검증한다(REQ-E07).
func TestApplyExposure_ExplicitList(t *testing.T) {
	items := []InventoryItem{
		mkItem("id-1", "alpha", KindFlow, nil),
		mkItem("id-2", "beta", KindFlow, nil),
		mkItem("id-3", "gamma", KindFlow, nil),
	}
	// id-1 by ID, beta by Name. 공백 trim 도 검증.
	got := applyExposure("id-1, beta", items)
	require.Len(t, got, 2)
	assert.Equal(t, "id-1", got[0].ID)
	assert.Equal(t, "id-2", got[1].ID)
}

// TestApplyExposure_DoesNotMutateInput 은 입력 슬라이스를 변경하지 않음을 검증한다.
func TestApplyExposure_DoesNotMutateInput(t *testing.T) {
	items := []InventoryItem{mkItem("1", "a", KindFlow, nil)}
	_ = applyExposure(ExposeAll, items)
	_ = applyExposure(ExposeNone, items)
	assert.Len(t, items, 1)
}

// TestDiffItems_Add 는 신규 항목이 add 델타로 도출되는지 검증한다(REQ-E02).
func TestDiffItems_Add(t *testing.T) {
	old := []InventoryItem{mkItem("1", "a", KindFlow, nil)}
	next := []InventoryItem{mkItem("1", "a", KindFlow, nil), mkItem("2", "b", KindFlow, nil)}
	deltas := diffItems(KindFlow, old, next)
	require.Len(t, deltas, 1)
	assert.Equal(t, OpAdd, deltas[0].Op)
	assert.Equal(t, "2", deltas[0].Item.ID)
}

// TestDiffItems_Update 는 내용 변경이 update 델타로 도출되는지 검증한다(REQ-E02).
func TestDiffItems_Update(t *testing.T) {
	old := []InventoryItem{mkItem("1", "a", KindFlow, map[string]any{"x": 1})}
	next := []InventoryItem{mkItem("1", "a", KindFlow, map[string]any{"x": 2})}
	deltas := diffItems(KindFlow, old, next)
	require.Len(t, deltas, 1)
	assert.Equal(t, OpUpdate, deltas[0].Op)
	assert.Equal(t, "1", deltas[0].Item.ID)
}

// TestDiffItems_Remove 는 사라진 항목이 remove 델타로 도출되는지 검증한다(REQ-E02).
func TestDiffItems_Remove(t *testing.T) {
	old := []InventoryItem{mkItem("1", "a", KindFlow, nil), mkItem("2", "b", KindFlow, nil)}
	next := []InventoryItem{mkItem("1", "a", KindFlow, nil)}
	deltas := diffItems(KindFlow, old, next)
	require.Len(t, deltas, 1)
	assert.Equal(t, OpRemove, deltas[0].Op)
	assert.Equal(t, "2", deltas[0].Item.ID)
}

// TestDiffItems_NoChange 는 무변경 시 델타가 없음을 검증한다.
func TestDiffItems_NoChange(t *testing.T) {
	old := []InventoryItem{mkItem("1", "a", KindFlow, map[string]any{"x": 1})}
	next := []InventoryItem{mkItem("1", "a", KindFlow, map[string]any{"x": 1})}
	assert.Empty(t, diffItems(KindFlow, old, next))
}

// TestDiffItems_ExposureNarrowingEmitsRemoves 는 노출 축소(mid-session)가 remove
// 델타를 내보내는지 검증한다(REQ-A07 — 노출 해제 자원은 remove 로 신호).
func TestDiffItems_ExposureNarrowingEmitsRemoves(t *testing.T) {
	all := []InventoryItem{
		mkItem("id-1", "alpha", KindFlow, nil),
		mkItem("id-2", "beta", KindFlow, nil),
	}
	// 이전엔 all 노출, 이제 "alpha" 만 노출 → beta 는 remove 로 신호.
	old := applyExposure(ExposeAll, all)
	narrowed := applyExposure("alpha", all)
	deltas := diffItems(KindFlow, old, narrowed)
	require.Len(t, deltas, 1)
	assert.Equal(t, OpRemove, deltas[0].Op)
	assert.Equal(t, "id-2", deltas[0].Item.ID)
}

// TestItemsEqual 은 동등 판정이 모든 필드를 고려하는지 검증한다.
func TestItemsEqual(t *testing.T) {
	a := InventoryItem{ID: "1", Name: "x", Kind: KindFlow, Status: "running", UpdatedAt: 100, Definition: json.RawMessage(`{"a":1}`)}
	b := a
	assert.True(t, itemsEqual(a, b))

	b.Status = "stopped"
	assert.False(t, itemsEqual(a, b))

	b = a
	b.Definition = json.RawMessage(`{"a":2}`)
	assert.False(t, itemsEqual(a, b))

	b = a
	b.UpdatedAt = 200
	assert.False(t, itemsEqual(a, b))
}
