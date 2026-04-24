package system

import (
	"context"
	"path"
	"strings"
	"time"
)

// NamespacedStore 는 네임스페이스 기반 접근 제어를 제공하는 Store 래퍼이다.
// 데코레이터 패턴을 사용하여 내부 Store에 네임스페이스 접두사를 자동으로 붙인다.
type NamespacedStore struct {
	inner     Store  // 래핑된 내부 저장소
	namespace string // 이 인스턴스의 네임스페이스
}

// NewNamespacedStore 는 주어진 Store를 네임스페이스로 래핑한다.
func NewNamespacedStore(store Store, namespace string) *NamespacedStore {
	return &NamespacedStore{
		inner:     store,
		namespace: namespace,
	}
}

// prefixKey 는 네임스페이스 접두사를 붙인 내부 키를 반환한다.
// 예: namespace="flow-abc", key="temperature" → "flow-abc:temperature"
func (ns *NamespacedStore) prefixKey(key string) string {
	return ns.namespace + ":" + key
}

// stripPrefix 는 내부 키에서 네임스페이스 접두사를 제거한다.
// 예: "flow-abc:temperature" → "temperature"
func (ns *NamespacedStore) stripPrefix(fullKey string) string {
	prefix := ns.namespace + ":"
	return strings.TrimPrefix(fullKey, prefix)
}

// Get 은 주어진 키에 해당하는 엔트리를 반환한다.
// 내부적으로 네임스페이스 접두사를 붙여 조회하고, Namespace 필드를 설정한다.
func (ns *NamespacedStore) Get(ctx context.Context, key string) (StoreEntry, error) {
	entry, err := ns.inner.Get(ctx, ns.prefixKey(key))
	if err != nil {
		return StoreEntry{}, err
	}
	entry.Namespace = ns.namespace
	return entry, nil
}

// keyGatekeeper 는 사용자 관점 key 가 쓰기 허용 대상인지 검사하는 훅이다.
// agentStore 가 이 인터페이스를 구현하며, allow_dynamic_keys=false 일 때
// 정적 키 목록에 없는 키 쓰기를 거부하기 위해 NamespacedStore 가 type 단언으로 호출한다.
//
// @spec SPEC-STORE-003
type keyGatekeeper interface {
	checkKeyAllowed(key string) error
}

// Set 은 주어진 키에 값을 저장한다.
// 내부적으로 네임스페이스 접두사를 붙여 저장한다.
// @spec SPEC-STORE-003: 네임스페이스 접두사가 붙기 전 사용자 관점 key 로
// keyGatekeeper 검증을 먼저 수행한다. 거부된 쓰기는 엔트리/히스토리에 기록되지 않는다.
func (ns *NamespacedStore) Set(ctx context.Context, key string, value any) error {
	if gk, ok := ns.inner.(keyGatekeeper); ok {
		if err := gk.checkKeyAllowed(key); err != nil {
			return err
		}
	}
	fullKey := ns.prefixKey(key)
	if err := ns.inner.Set(ctx, fullKey, value); err != nil {
		return err
	}
	ns.setNamespace(fullKey)
	return nil
}

// SetWithTTL 은 주어진 키에 TTL과 함께 값을 저장한다.
// 내부적으로 네임스페이스 접두사를 붙여 저장한다.
// @spec SPEC-STORE-003: Set 과 동일하게 사용자 관점 key 로 gatekeeper 검증 수행.
func (ns *NamespacedStore) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	if gk, ok := ns.inner.(keyGatekeeper); ok {
		if err := gk.checkKeyAllowed(key); err != nil {
			return err
		}
	}
	fullKey := ns.prefixKey(key)
	if err := ns.inner.SetWithTTL(ctx, fullKey, value, ttl); err != nil {
		return err
	}
	ns.setNamespace(fullKey)
	return nil
}

// namespaceWriter 는 storeItem의 namespace 필드를 직접 설정하기 위한 내부 인터페이스이다.
type namespaceWriter interface {
	setItemNamespace(key string, namespace string)
}

// setNamespace 는 저장된 항목에 네임스페이스를 기록한다.
func (ns *NamespacedStore) setNamespace(fullKey string) {
	if nw, ok := ns.inner.(namespaceWriter); ok {
		nw.setItemNamespace(fullKey, ns.namespace)
	}
}

// Delete 는 주어진 키를 삭제한다.
// 내부적으로 네임스페이스 접두사를 붙여 삭제한다.
func (ns *NamespacedStore) Delete(ctx context.Context, key string) error {
	return ns.inner.Delete(ctx, ns.prefixKey(key))
}

// Has 는 주어진 키가 존재하고 만료되지 않았는지 확인한다.
// 내부적으로 네임스페이스 접두사를 붙여 확인한다.
func (ns *NamespacedStore) Has(ctx context.Context, key string) (bool, error) {
	return ns.inner.Has(ctx, ns.prefixKey(key))
}

// Keys 는 이 네임스페이스에 속한 키 목록을 반환한다.
// 내부 키에서 네임스페이스 접두사를 제거하여 반환한다.
// 패턴은 접두사가 제거된 키에 대해 적용된다.
func (ns *NamespacedStore) Keys(ctx context.Context, pattern string) ([]string, error) {
	// 내부 스토어에서 이 네임스페이스의 모든 키를 가져온다
	prefix := ns.namespace + ":"
	allKeys, err := ns.inner.Keys(ctx, prefix+"*")
	if err != nil {
		return nil, err
	}

	matchAll := pattern == "" || pattern == "*"
	var result []string

	for _, fullKey := range allKeys {
		stripped := ns.stripPrefix(fullKey)
		if matchAll {
			result = append(result, stripped)
			continue
		}
		// 접두사가 제거된 키에 대해 패턴 매칭
		matched, matchErr := path.Match(pattern, stripped)
		if matchErr != nil {
			continue
		}
		if matched {
			result = append(result, stripped)
		}
	}

	return result, nil
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 반환한다.
// 내부적으로 네임스페이스 접두사를 붙여 조회한다.
func (ns *NamespacedStore) GetHistory(ctx context.Context, key string) ([]HistoryEntry, error) {
	return ns.inner.GetHistory(ctx, ns.prefixKey(key))
}

// QueryHistory 는 HistoryQuery 조건에 따라 시계열 엔트리를 반환한다.
// 내부적으로 네임스페이스 접두사를 붙여 위임한다.
func (ns *NamespacedStore) QueryHistory(ctx context.Context, key string, q HistoryQuery) ([]HistoryEntry, error) {
	return ns.inner.QueryHistory(ctx, ns.prefixKey(key), q)
}

// Clear 는 이 네임스페이스의 모든 키를 삭제한다.
// 다른 네임스페이스의 키는 영향받지 않는다.
func (ns *NamespacedStore) Clear(ctx context.Context) error {
	// 이 네임스페이스의 모든 키를 조회
	prefix := ns.namespace + ":"
	keys, err := ns.inner.Keys(ctx, prefix+"*")
	if err != nil {
		return err
	}

	// 각 키를 개별 삭제 (내부 키는 이미 접두사가 포함됨)
	for _, fullKey := range keys {
		if delErr := ns.inner.Delete(ctx, fullKey); delErr != nil {
			return delErr
		}
	}

	return nil
}
