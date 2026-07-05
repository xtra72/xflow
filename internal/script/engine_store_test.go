package script

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// engine_store_test.go (message-slim-metadata / Follow-up A) 는 스크립트 엔진의
// 실행별(per-Execute) 스토어 바인딩을 엔진 레벨에서 검증한다.

// memStore 는 테스트용 in-memory StoreAccessor 이다(네임스페이스별 독립 맵).
type memStore struct {
	mu sync.Mutex
	m  map[string]any
}

func newMemStore() *memStore { return &memStore{m: map[string]any{}} }

func (s *memStore) Get(_ context.Context, key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil // 없으면 nil
}
func (s *memStore) Set(_ context.Context, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}
func (s *memStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}
func (s *memStore) Has(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	return ok, nil
}

func newStoreEngine(t *testing.T, opts ...EngineOption) *DefaultScriptEngine {
	t.Helper()
	base := []EngineOption{WithStdlib(StdlibOptions{EnableStore: true}, StdlibDeps{})}
	eng := NewScriptEngine(append(base, opts...)...)
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	return eng
}

func compile(t *testing.T, eng *DefaultScriptEngine, name, src string) string {
	t.Helper()
	id, err := eng.Compile(context.Background(), ScriptSource{Type: SourceInline, Name: name, Content: src})
	require.NoError(t, err)
	return id
}

// TestEngineStore_ExecuteWithStore_Binds 는 ExecuteWithStore 로 넘긴 스토어가
// 이번 실행의 xflow.store 에 바인딩됨을 검증한다.
func TestEngineStore_ExecuteWithStore_Binds(t *testing.T) {
	eng := newStoreEngine(t)
	store := newMemStore()

	sid := compile(t, eng, "s1", `xflow.store.set("k", "v"); return xflow.store.get("k")`)
	res, err := eng.ExecuteWithStore(context.Background(), sid, nil, store)
	require.NoError(t, err)
	assert.Equal(t, "v", res)
	assert.Equal(t, "v", store.m["k"], "하위 스토어에 실제로 기록되어야 한다")
}

// TestEngineStore_UnbindAfterExecute 는 실행 후 바인딩이 해제되어, 스토어 없이
// 실행한 다음 호출에서 xflow.store 가 nil 로 동작함을 검증한다(누출 없음).
// poolSize=1 로 같은 VM 을 재사용해도 바인딩이 남지 않아야 한다.
func TestEngineStore_UnbindAfterExecute(t *testing.T) {
	eng := newStoreEngine(t, WithPoolSize(1))
	store := newMemStore()
	_ = store.Set(context.Background(), "k", "leaked")

	sidBound := compile(t, eng, "bound", `return xflow.store.get("k")`)
	res, err := eng.ExecuteWithStore(context.Background(), sidBound, nil, store)
	require.NoError(t, err)
	assert.Equal(t, "leaked", res)

	// 스토어 바인딩 없이 같은 VM 재사용 → get 은 nil 이어야 한다(이전 바인딩 누출 금지).
	sidUnbound := compile(t, eng, "unbound", `local v = xflow.store.get("k"); if v == nil then return "nil" end; return v`)
	res2, err := eng.Execute(context.Background(), sidUnbound, nil)
	require.NoError(t, err)
	assert.Equal(t, "nil", res2, "실행 후 바인딩이 해제되어 다음 실행에 누출되면 안 된다")
}

// TestEngineStore_UnbindOnError 는 스크립트가 런타임 에러로 실패해도 바인딩이
// 해제됨을 검증한다(defer 보장). poolSize=1.
func TestEngineStore_UnbindOnError(t *testing.T) {
	eng := newStoreEngine(t, WithPoolSize(1))
	store := newMemStore()
	_ = store.Set(context.Background(), "k", "v")

	// 스토어 바인딩된 상태에서 의도적으로 런타임 에러 발생.
	sidErr := compile(t, eng, "err", `xflow.store.set("k","v"); error("boom")`)
	_, err := eng.ExecuteWithStore(context.Background(), sidErr, nil, store)
	require.Error(t, err)

	// 에러 후에도 바인딩이 해제되어야 한다 → 다음 무바인딩 실행에서 nil.
	sidCheck := compile(t, eng, "check", `local v = xflow.store.get("k"); if v == nil then return "nil" end; return v`)
	res, err := eng.Execute(context.Background(), sidCheck, nil)
	require.NoError(t, err)
	assert.Equal(t, "nil", res, "에러 경로에서도 defer 로 바인딩이 해제되어야 한다")
}

// TestEngineStore_Isolation 는 서로 다른 스토어로 두 번 실행 시 각 실행이 자기
// 스토어만 보고 서로 섞이지 않음을 검증한다(poolSize=1, 같은 VM 공유).
func TestEngineStore_Isolation(t *testing.T) {
	eng := newStoreEngine(t, WithPoolSize(1))
	storeA := newMemStore()
	storeB := newMemStore()

	sidSet := compile(t, eng, "set", `xflow.store.set("shared", msg); return "ok"`)
	_, err := eng.ExecuteWithStore(context.Background(), sidSet, "in-a", storeA)
	require.NoError(t, err)

	sidGet := compile(t, eng, "get", `local v = xflow.store.get("shared"); if v == nil then return "nil" end; return v`)
	// storeB 로 실행 → shared 없음 → nil.
	resB, err := eng.ExecuteWithStore(context.Background(), sidGet, nil, storeB)
	require.NoError(t, err)
	assert.Equal(t, "nil", resB, "storeB 는 storeA 의 값을 보면 안 된다")

	// storeA 로 실행 → 값 존재.
	resA, err := eng.ExecuteWithStore(context.Background(), sidGet, nil, storeA)
	require.NoError(t, err)
	assert.Equal(t, "in-a", resA)

	assert.Equal(t, "in-a", storeA.m["shared"])
	_, existsB := storeB.m["shared"]
	assert.False(t, existsB, fmt.Sprintf("storeB 에 shared 가 없어야 한다: %v", storeB.m))
}
