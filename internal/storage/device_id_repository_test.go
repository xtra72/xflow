package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// TestDeviceIDFileRepository_GetOrCreate_NewKey 는 신규 키 시 UUID 가 생성/영속되는지 확인.
func TestDeviceIDFileRepository_GetOrCreate_NewKey(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	id, err := r.GetOrCreate(context.Background(), "lg_hvacr01-bus1", "idu-1")
	require.NoError(t, err)
	assert.Len(t, id, 36, "UUID v4 길이 36")

	// 파일 존재 확인.
	_, err = os.Stat(filepath.Join(dir, "device_ids.json"))
	assert.NoError(t, err, "device_ids.json 파일이 생성되어야 함")
}

// TestDeviceIDFileRepository_GetOrCreate_Idempotent 는 동일 키 재호출 시 같은 UUID 반환.
func TestDeviceIDFileRepository_GetOrCreate_Idempotent(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	id1, err := r.GetOrCreate(context.Background(), "century_hvacr01", "0x3B")
	require.NoError(t, err)

	id2, err := r.GetOrCreate(context.Background(), "century_hvacr01", "0x3B")
	require.NoError(t, err)

	assert.Equal(t, id1, id2, "동일 키 재호출은 같은 UUID")
}

// TestDeviceIDFileRepository_DifferentKeys 는 다른 키는 다른 UUID 를 받는지 확인.
func TestDeviceIDFileRepository_DifferentKeys(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	id1, _ := r.GetOrCreate(context.Background(), "lg_hvacr01", "idu-1")
	id2, _ := r.GetOrCreate(context.Background(), "lg_hvacr01", "idu-2")
	id3, _ := r.GetOrCreate(context.Background(), "lg_hvacr02", "idu-1")

	assert.NotEqual(t, id1, id2, "같은 agent / 다른 unit_id")
	assert.NotEqual(t, id1, id3, "다른 agent / 같은 unit_id")
	assert.NotEqual(t, id2, id3)
}

// TestDeviceIDFileRepository_PersistenceAcrossInstances 는 같은 dir 재오픈 시 UUID 유지 확인.
func TestDeviceIDFileRepository_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// 첫 인스턴스: UUID 생성.
	r1, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	id1, err := r1.GetOrCreate(context.Background(), "samsung_nasa", "20.01.00")
	require.NoError(t, err)
	require.NoError(t, r1.Close())

	// 둘째 인스턴스: 같은 dir 재오픈 → 같은 UUID 반환되어야 함.
	r2, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r2.Close()

	id2, err := r2.GetOrCreate(context.Background(), "samsung_nasa", "20.01.00")
	require.NoError(t, err)
	assert.Equal(t, id1, id2, "재오픈 후에도 같은 UUID 유지")
}

// TestDeviceIDFileRepository_Get_NotExists 는 미존재 키는 빈 문자열 반환.
func TestDeviceIDFileRepository_Get_NotExists(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	id, err := r.Get(context.Background(), "absent", "absent-unit")
	require.NoError(t, err)
	assert.Equal(t, "", id)
}

// TestDeviceIDFileRepository_Delete 는 삭제 후 Get 이 빈 문자열 반환.
func TestDeviceIDFileRepository_Delete(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	_, _ = r.GetOrCreate(context.Background(), "lg_hvacr01", "idu-1")
	require.NoError(t, r.Delete(context.Background(), "lg_hvacr01", "idu-1"))

	id, _ := r.Get(context.Background(), "lg_hvacr01", "idu-1")
	assert.Equal(t, "", id)
}

// TestDeviceIDFileRepository_EmptyArgs 는 빈 agentName / unitID 에 에러 반환.
func TestDeviceIDFileRepository_EmptyArgs(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	_, err = r.GetOrCreate(context.Background(), "", "idu-1")
	assert.Error(t, err)

	_, err = r.GetOrCreate(context.Background(), "lg_hvacr01", "")
	assert.Error(t, err)
}

// TestDeviceIDMemoryRepository_Basic 는 in-memory 구현의 기본 동작 확인.
func TestDeviceIDMemoryRepository_Basic(t *testing.T) {
	r := NewDeviceIDMemoryRepository()
	defer r.Close()

	id1, err := r.GetOrCreate(context.Background(), "lg_hvacr01", "idu-1")
	require.NoError(t, err)
	assert.Len(t, id1, 36)

	id2, _ := r.GetOrCreate(context.Background(), "lg_hvacr01", "idu-1")
	assert.Equal(t, id1, id2)

	list, _ := r.List(context.Background())
	assert.Len(t, list, 1)
	assert.Equal(t, id1, list["lg_hvacr01:idu-1"])
}

// ---------------------------------------------------------------------------
// Set (지정 device_id 등록) 테스트 — File + Memory
// ---------------------------------------------------------------------------

// TestDeviceIDFileRepository_Set 은 지정 device_id 가 등록·영속되고, 이후
// GetOrCreate/Get 이 자동생성 대신 지정값을 반환하는지 검증한다.
func TestDeviceIDFileRepository_Set(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	ctx := context.Background()

	// 지정 등록.
	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "my-fixed-id-123"))

	// Get 은 지정값을 반환.
	got, err := r.Get(ctx, "samsung", "20.00.03")
	require.NoError(t, err)
	assert.Equal(t, "my-fixed-id-123", got)

	// GetOrCreate 는 새 UUID 를 만들지 않고 지정값을 반환.
	goc, err := r.GetOrCreate(ctx, "samsung", "20.00.03")
	require.NoError(t, err)
	assert.Equal(t, "my-fixed-id-123", goc, "GetOrCreate must return the Set value, not a new UUID")
}

// TestDeviceIDFileRepository_Set_Idempotent 은 같은 키에 같은 값 재지정이
// 에러 없이 no-op 인지 검증한다.
func TestDeviceIDFileRepository_Set_Idempotent(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	ctx := context.Background()
	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "fixed"))
	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "fixed"), "same key+value must be idempotent")
}

// TestDeviceIDFileRepository_Set_Conflict 은 같은 device_id 를 다른 키에 지정하면
// ErrDeviceIDConflict 를 반환하는지 검증한다(전역 유일성).
func TestDeviceIDFileRepository_Set_Conflict(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	ctx := context.Background()
	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "dup-id"))

	err = r.Set(ctx, "samsung", "20.00.04", "dup-id")
	require.Error(t, err)
	assert.True(t, errors.Is(err, agent.ErrDeviceIDConflict), "duplicate device_id on a different key must conflict")

	// 충돌 시 두 번째 키는 등록되지 않아야 한다.
	got, _ := r.Get(ctx, "samsung", "20.00.04")
	assert.Empty(t, got)
}

// TestDeviceIDFileRepository_Set_Persistence 는 지정 device_id 가 파일에 영속되어
// 새 인스턴스(재시작)에서도 유지되는지 검증한다.
func TestDeviceIDFileRepository_Set_Persistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	r1, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	require.NoError(t, r1.Set(ctx, "samsung", "20.00.03", "persisted-id"))
	require.NoError(t, r1.Close())

	// 재시작 시뮬레이션: 같은 디렉터리로 새 인스턴스.
	r2, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r2.Close()

	got, err := r2.Get(ctx, "samsung", "20.00.03")
	require.NoError(t, err)
	assert.Equal(t, "persisted-id", got, "Set value must survive a restart via the file")
}

// TestDeviceIDFileRepository_Set_EmptyArgs 는 빈 인자 시 에러를 검증한다.
func TestDeviceIDFileRepository_Set_EmptyArgs(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r.Close()

	ctx := context.Background()
	assert.Error(t, r.Set(ctx, "", "u", "d"))
	assert.Error(t, r.Set(ctx, "a", "", "d"))
	assert.Error(t, r.Set(ctx, "a", "u", ""))
}

// TestDeviceIDMemoryRepository_Set 은 메모리 저장소의 Set 동작(등록/idempotent/충돌)을 검증한다.
func TestDeviceIDMemoryRepository_Set(t *testing.T) {
	r := NewDeviceIDMemoryRepository()
	defer r.Close()
	ctx := context.Background()

	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "fixed-id"))

	goc, _ := r.GetOrCreate(ctx, "samsung", "20.00.03")
	assert.Equal(t, "fixed-id", goc, "GetOrCreate must return the Set value")

	// idempotent
	require.NoError(t, r.Set(ctx, "samsung", "20.00.03", "fixed-id"))

	// 충돌
	err := r.Set(ctx, "samsung", "20.00.04", "fixed-id")
	require.Error(t, err)
	assert.True(t, errors.Is(err, agent.ErrDeviceIDConflict))
}
