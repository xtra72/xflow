package remote

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveInstanceID_GenerateOnce 는 instance_id 가 없을 때 1회 생성되어
// 파일에 영속되는지 검증한다 (REQ-REMOTE-A03).
func TestResolveInstanceID_GenerateOnce(t *testing.T) {
	dir := t.TempDir()

	id, err := ResolveInstanceID("", dir)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// 생성된 값은 유효한 UUID 여야 한다.
	_, parseErr := uuid.Parse(id)
	assert.NoError(t, parseErr, "생성된 instance_id 는 유효한 UUID 여야 함")

	// 파일이 실제로 생성되었는지 확인.
	path := filepath.Join(dir, instanceIDFileName)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, id, string(data), "파일 내용은 반환된 id 와 일치해야 함")
}

// TestResolveInstanceID_StableAcrossReloads 는 재호출(재시작 모사) 간 동일한
// instance_id 가 유지되는지 검증한다 (REQ-REMOTE-A03).
func TestResolveInstanceID_StableAcrossReloads(t *testing.T) {
	dir := t.TempDir()

	first, err := ResolveInstanceID("", dir)
	require.NoError(t, err)

	// 두 번째 호출은 기존 파일에서 로드해야 한다 (새로 생성 금지).
	second, err := ResolveInstanceID("", dir)
	require.NoError(t, err)

	assert.Equal(t, first, second, "재시작 간 instance_id 는 불변이어야 함")
}

// TestResolveInstanceID_ConfigOverride 는 명시적 config override 가 우선하며,
// override 사용 시 파일을 생성하지 않는지 검증한다 (REQ-REMOTE-A03, §5.2).
func TestResolveInstanceID_ConfigOverride(t *testing.T) {
	dir := t.TempDir()
	override := "fixed-node-001"

	id, err := ResolveInstanceID(override, dir)
	require.NoError(t, err)
	assert.Equal(t, override, id, "override 값이 그대로 사용되어야 함")

	// override 사용 시 파일은 생성되지 않아야 한다 (override 는 config 가 소유).
	path := filepath.Join(dir, instanceIDFileName)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "override 사용 시 영속 파일 생성 금지")
}

// TestResolveInstanceID_OverrideDoesNotClobberPersisted 는 override 가 이미
// 영속된 파일을 덮어쓰지 않음을 검증한다.
func TestResolveInstanceID_OverrideDoesNotClobberPersisted(t *testing.T) {
	dir := t.TempDir()

	// 먼저 자동 생성으로 영속.
	persisted, err := ResolveInstanceID("", dir)
	require.NoError(t, err)

	// override 호출은 파일을 변경하지 않아야 한다.
	_, err = ResolveInstanceID("override-id", dir)
	require.NoError(t, err)

	// 파일에는 여전히 원래 영속 값이 있어야 한다.
	path := filepath.Join(dir, instanceIDFileName)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, persisted, string(data))
}

// TestResolveInstanceID_CreatesDirectory 는 데이터 디렉토리가 없을 때 생성하는지
// 검증한다.
func TestResolveInstanceID_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")

	id, err := ResolveInstanceID("", dir)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	info, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// TestResolveInstanceID_TrimsWhitespace 는 영속 파일의 선후행 공백을 제거하는지
// 검증한다 (수동 편집 내성).
func TestResolveInstanceID_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, instanceIDFileName)
	require.NoError(t, os.WriteFile(path, []byte("  padded-id\n"), 0o600))

	id, err := ResolveInstanceID("", dir)
	require.NoError(t, err)
	assert.Equal(t, "padded-id", id)
}

// TestResolveInstanceID_ConcurrentGenerateOnce 는 동시 호출에서도 단일 id 만
// 영속되는지(파일 race 내성) 검증한다.
func TestResolveInstanceID_ConcurrentGenerateOnce(t *testing.T) {
	dir := t.TempDir()

	const goroutines = 16
	var wg sync.WaitGroup
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ResolveInstanceID("", dir)
		}(i)
	}
	wg.Wait()

	for i := 0; i < goroutines; i++ {
		require.NoError(t, errs[i])
		require.NotEmpty(t, results[i])
	}

	// 최종 영속 파일과 모든 반환값이 일치해야 한다 (단일 id 합의).
	path := filepath.Join(dir, instanceIDFileName)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	persisted := string(data)
	for i := 0; i < goroutines; i++ {
		assert.Equal(t, persisted, results[i],
			"동시 호출은 동일한 영속 instance_id 를 반환해야 함")
	}
}
