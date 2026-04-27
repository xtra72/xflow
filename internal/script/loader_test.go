package script

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// mockStoreAccessor - StoreAccessor 인터페이스 모의 구현
// ============================================================

type mockStoreAccessor struct {
	data map[string]any
	err  error // 모든 호출에 대해 반환할 에러
}

func newMockStore() *mockStoreAccessor {
	return &mockStoreAccessor{data: make(map[string]any)}
}

func (m *mockStoreAccessor) Get(_ context.Context, key string) (any, error) {
	if m.err != nil {
		return nil, m.err
	}
	v, ok := m.data[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return v, nil
}

func (m *mockStoreAccessor) Set(_ context.Context, key string, value any) error {
	if m.err != nil {
		return m.err
	}
	m.data[key] = value
	return nil
}

func (m *mockStoreAccessor) Delete(_ context.Context, key string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.data, key)
	return nil
}

func (m *mockStoreAccessor) Has(_ context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, ok := m.data[key]
	return ok, nil
}

// ============================================================
// NewScriptLoader 테스트
// ============================================================

func TestNewScriptLoader_Default(t *testing.T) {
	loader := NewScriptLoader()
	require.NotNil(t, loader)
}

func TestNewScriptLoader_WithStoreAccessor(t *testing.T) {
	store := newMockStore()
	loader := NewScriptLoader(WithStoreAccessor(store))
	require.NotNil(t, loader)
}

// ============================================================
// LoadScript - SourceInline 테스트
// ============================================================

func TestLoadScript_Inline_Success(t *testing.T) {
	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type:    SourceInline,
		Content: "return 42",
		Name:    "inline-test",
	}

	content, err := loader.LoadScript(ctx, source)
	require.NoError(t, err)
	assert.Equal(t, "return 42", content)
}

func TestLoadScript_Inline_EmptyContent(t *testing.T) {
	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type:    SourceInline,
		Content: "",
		Name:    "empty-inline",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

// ============================================================
// LoadScript - SourceFile 테스트
// ============================================================

func TestLoadScript_File_Success(t *testing.T) {
	// 임시 디렉토리에 스크립트 파일 생성
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err := os.WriteFile(scriptPath, []byte("return 'from file'"), 0644)
	require.NoError(t, err)

	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type: SourceFile,
		Path: scriptPath,
		Name: "file-test",
	}

	content, err := loader.LoadScript(ctx, source)
	require.NoError(t, err)
	assert.Equal(t, "return 'from file'", content)
}

func TestLoadScript_File_NotFound(t *testing.T) {
	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type: SourceFile,
		Path: "/nonexistent/path/script.lua",
		Name: "missing-file",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

func TestLoadScript_File_EmptyPath(t *testing.T) {
	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type: SourceFile,
		Path: "",
		Name: "empty-path",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

// ============================================================
// LoadScript - SourceStore 테스트
// ============================================================

func TestLoadScript_Store_Success(t *testing.T) {
	store := newMockStore()
	store.data["my-script-key"] = "return 'from store'"

	loader := NewScriptLoader(WithStoreAccessor(store))
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "my-script-key",
		Name:     "store-test",
	}

	content, err := loader.LoadScript(ctx, source)
	require.NoError(t, err)
	assert.Equal(t, "return 'from store'", content)
}

func TestLoadScript_Store_KeyNotFound(t *testing.T) {
	store := newMockStore()

	loader := NewScriptLoader(WithStoreAccessor(store))
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "nonexistent-key",
		Name:     "missing-key",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptNotFound)
}

func TestLoadScript_Store_EmptyStoreKey(t *testing.T) {
	store := newMockStore()

	loader := NewScriptLoader(WithStoreAccessor(store))
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "",
		Name:     "empty-key",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

func TestLoadScript_Store_NilStoreAccessor(t *testing.T) {
	loader := NewScriptLoader() // store 미설정
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "my-key",
		Name:     "no-store",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

func TestLoadScript_Store_NonStringValue(t *testing.T) {
	store := newMockStore()
	store.data["int-key"] = 12345 // 문자열이 아닌 값

	loader := NewScriptLoader(WithStoreAccessor(store))
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "int-key",
		Name:     "non-string",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

func TestLoadScript_Store_ByteSliceValue(t *testing.T) {
	store := newMockStore()
	store.data["bytes-key"] = []byte("return 'from bytes'")

	loader := NewScriptLoader(WithStoreAccessor(store))
	ctx := context.Background()

	source := ScriptSource{
		Type:     SourceStore,
		StoreKey: "bytes-key",
		Name:     "bytes-val",
	}

	content, err := loader.LoadScript(ctx, source)
	require.NoError(t, err)
	assert.Equal(t, "return 'from bytes'", content)
}

// ============================================================
// LoadScript - 잘못된 SourceType 테스트
// ============================================================

func TestLoadScript_InvalidSourceType(t *testing.T) {
	loader := NewScriptLoader()
	ctx := context.Background()

	source := ScriptSource{
		Type: SourceType(99), // 정의되지 않은 타입
		Name: "invalid-type",
	}

	_, err := loader.LoadScript(ctx, source)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScriptSource)
}

// ============================================================
// StoreAccessor 인터페이스 컴파일 타임 확인
// ============================================================

func TestStoreAccessorInterface(t *testing.T) {
	var _ StoreAccessor = (*mockStoreAccessor)(nil)
}
