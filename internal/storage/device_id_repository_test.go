package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
