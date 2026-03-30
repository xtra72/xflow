package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 컴파일 타임 Store 인터페이스 구현 검증
var _ Store = (*NamespacedStore)(nil)

func TestNamespacedStore_SetGet_PrefixVerification(t *testing.T) {
	// Arrange: 네임스페이스 "flow-abc"로 래핑된 스토어 생성
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	ns := NewNamespacedStore(inner, "flow-abc")
	ctx := context.Background()

	// Act: 네임스페이스 스토어를 통해 키 저장
	err := ns.Set(ctx, "temperature", 42)
	require.NoError(t, err)

	// Assert: 내부 스토어에는 "flow-abc:temperature" 키로 저장됨
	entry, err := inner.Get(ctx, "flow-abc:temperature")
	require.NoError(t, err)
	assert.Equal(t, 42, entry.Value)

	// Assert: 네임스페이스 스토어를 통해 "temperature"로 조회 가능
	entry, err = ns.Get(ctx, "temperature")
	require.NoError(t, err)
	assert.Equal(t, 42, entry.Value)
	assert.Equal(t, "flow-abc", entry.Namespace)
}

func TestNamespacedStore_NamespaceIsolation(t *testing.T) {
	// Arrange: 동일한 내부 스토어에 두 개의 네임스페이스 생성
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	nsA := NewNamespacedStore(inner, "ns-a")
	nsB := NewNamespacedStore(inner, "ns-b")
	ctx := context.Background()

	// Act: 같은 키 이름으로 다른 값 저장
	require.NoError(t, nsA.Set(ctx, "key", "value-a"))
	require.NoError(t, nsB.Set(ctx, "key", "value-b"))

	// Assert: 각 네임스페이스에서 각각의 값이 조회됨
	entryA, err := nsA.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value-a", entryA.Value)

	entryB, err := nsB.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value-b", entryB.Value)
}

func TestNamespacedStore_Keys_OnlyNamespaceKeys(t *testing.T) {
	// Arrange: 두 네임스페이스에 키 저장
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	nsA := NewNamespacedStore(inner, "ns-a")
	nsB := NewNamespacedStore(inner, "ns-b")
	ctx := context.Background()

	require.NoError(t, nsA.Set(ctx, "k1", "v1"))
	require.NoError(t, nsA.Set(ctx, "k2", "v2"))
	require.NoError(t, nsB.Set(ctx, "k3", "v3"))

	// Act: ns-a의 모든 키 조회
	keys, err := nsA.Keys(ctx, "*")
	require.NoError(t, err)

	// Assert: ns-a의 키만 반환되고, 접두사가 제거됨
	assert.ElementsMatch(t, []string{"k1", "k2"}, keys)
}

func TestNamespacedStore_Keys_PatternFilter(t *testing.T) {
	// Arrange: 네임스페이스에 다양한 키 저장
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	ns := NewNamespacedStore(inner, "myns")
	ctx := context.Background()

	require.NoError(t, ns.Set(ctx, "user-1", "alice"))
	require.NoError(t, ns.Set(ctx, "user-2", "bob"))
	require.NoError(t, ns.Set(ctx, "config-main", "data"))

	// Act: "user-*" 패턴으로 키 조회
	keys, err := ns.Keys(ctx, "user-*")
	require.NoError(t, err)

	// Assert: user-* 패턴에 매칭되는 키만 반환
	assert.ElementsMatch(t, []string{"user-1", "user-2"}, keys)
}

func TestNamespacedStore_Clear_OnlyNamespaceKeys(t *testing.T) {
	// Arrange: 두 네임스페이스에 키 저장
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	nsA := NewNamespacedStore(inner, "ns-a")
	nsB := NewNamespacedStore(inner, "ns-b")
	ctx := context.Background()

	require.NoError(t, nsA.Set(ctx, "k1", "v1"))
	require.NoError(t, nsB.Set(ctx, "k2", "v2"))

	// Act: ns-a만 클리어
	require.NoError(t, nsA.Clear(ctx))

	// Assert: ns-a의 키가 삭제됨
	_, err := nsA.Get(ctx, "k1")
	assert.ErrorIs(t, err, ErrKeyNotFound)

	// Assert: ns-b의 키는 유지됨
	entry, err := nsB.Get(ctx, "k2")
	require.NoError(t, err)
	assert.Equal(t, "v2", entry.Value)
}

func TestNamespacedStore_Has(t *testing.T) {
	// Arrange
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	ns := NewNamespacedStore(inner, "test")
	ctx := context.Background()

	require.NoError(t, ns.Set(ctx, "exists", "val"))

	// Act & Assert: 존재하는 키
	exists, err := ns.Has(ctx, "exists")
	require.NoError(t, err)
	assert.True(t, exists)

	// Act & Assert: 존재하지 않는 키
	exists, err = ns.Has(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestNamespacedStore_Delete(t *testing.T) {
	// Arrange
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	ns := NewNamespacedStore(inner, "test")
	ctx := context.Background()

	require.NoError(t, ns.Set(ctx, "to-delete", "val"))

	// Act: 키 삭제
	err := ns.Delete(ctx, "to-delete")
	require.NoError(t, err)

	// Assert: 삭제 확인
	_, err = ns.Get(ctx, "to-delete")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

func TestNamespacedStore_GlobalNamespace(t *testing.T) {
	// Arrange: 두 스토어가 "global" 네임스페이스 사용
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	nsA := NewNamespacedStore(inner, "global")
	nsB := NewNamespacedStore(inner, "global")
	ctx := context.Background()

	// Act: 한 스토어에서 저장
	require.NoError(t, nsA.Set(ctx, "shared", "data"))

	// Assert: 동일한 네임스페이스이므로 다른 인스턴스에서도 접근 가능
	entry, err := nsB.Get(ctx, "shared")
	require.NoError(t, err)
	assert.Equal(t, "data", entry.Value)
}

func TestNamespacedStore_CrossNamespaceKeyFormat(t *testing.T) {
	// Arrange: "ns-a"에서 "other:key"를 설정하면 내부적으로 "ns-a:other:key"가 됨
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	nsA := NewNamespacedStore(inner, "ns-a")
	nsOther := NewNamespacedStore(inner, "other")
	ctx := context.Background()

	// Act: ns-a에서 "other:key"를 설정 (다른 네임스페이스에 접근 시도)
	require.NoError(t, nsA.Set(ctx, "other:key", "tricky"))

	// Assert: "other" 네임스페이스의 "key"와는 다른 키임
	_, err := nsOther.Get(ctx, "key")
	assert.ErrorIs(t, err, ErrKeyNotFound)

	// Assert: ns-a에서는 "other:key"로 접근 가능
	entry, err := nsA.Get(ctx, "other:key")
	require.NoError(t, err)
	assert.Equal(t, "tricky", entry.Value)
}

func TestNamespacedStore_GetHistory(t *testing.T) {
	// Arrange: 히스토리가 활성화된 내부 스토어에 네임스페이스 래핑
	inner := NewVolatileStore(MaxKeyLength, 10, 0)
	ns := NewNamespacedStore(inner, "hist-ns")
	ctx := context.Background()

	// Act: 같은 키에 3번 Set → 히스토리 2개 생성
	require.NoError(t, ns.Set(ctx, "temp", "v1"))
	require.NoError(t, ns.Set(ctx, "temp", "v2"))
	require.NoError(t, ns.Set(ctx, "temp", "v3"))

	// Assert: 네임스페이스 스토어를 통해 히스토리 조회
	history, err := ns.GetHistory(ctx, "temp")
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, "v2", history[0].Value, "최신 히스토리가 먼저 와야 한다")
	assert.Equal(t, "v1", history[1].Value)
}

func TestNamespacedStore_GetHistory_Isolation(t *testing.T) {
	// Arrange: 두 네임스페이스에서 같은 키 이름 사용
	inner := NewVolatileStore(MaxKeyLength, 10, 0)
	nsA := NewNamespacedStore(inner, "ns-a")
	nsB := NewNamespacedStore(inner, "ns-b")
	ctx := context.Background()

	// Act: ns-a에서만 히스토리 생성
	require.NoError(t, nsA.Set(ctx, "key", "a1"))
	require.NoError(t, nsA.Set(ctx, "key", "a2"))
	require.NoError(t, nsB.Set(ctx, "key", "b1"))

	// Assert: ns-a는 히스토리 1개, ns-b는 히스토리 0개
	histA, err := nsA.GetHistory(ctx, "key")
	require.NoError(t, err)
	assert.Len(t, histA, 1)
	assert.Equal(t, "a1", histA[0].Value)

	histB, err := nsB.GetHistory(ctx, "key")
	require.NoError(t, err)
	assert.Empty(t, histB, "ns-b는 한 번만 Set했으므로 히스토리가 없어야 한다")
}

func TestNamespacedStore_SetWithTTL(t *testing.T) {
	// Arrange
	inner := NewVolatileStore(MaxKeyLength, 0, 0)
	ns := NewNamespacedStore(inner, "ttl-ns")
	ctx := context.Background()

	// Act: TTL과 함께 저장
	err := ns.SetWithTTL(ctx, "temp", "data", 5*60*1e9) // 5분
	require.NoError(t, err)

	// Assert: 조회 가능
	entry, err := ns.Get(ctx, "temp")
	require.NoError(t, err)
	assert.Equal(t, "data", entry.Value)
	assert.Equal(t, "ttl-ns", entry.Namespace)
	assert.True(t, entry.TTL > 0)
}
