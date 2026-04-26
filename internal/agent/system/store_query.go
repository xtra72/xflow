package system

import (
	"context"
	"fmt"
	"sort"
)

// defaultStoreNamespace 는 namespace 가 비어있을 때 사용되는 기본 값이다.
const defaultStoreNamespace = "default"

// StoreHistoryQueryer 는 Store 시스템 에이전트가 제공하는 쿼리 계약이다.
// HTTP 핸들러가 타입 단언을 통해 이 인터페이스로 에이전트를 검증한다.
type StoreHistoryQueryer interface {
	QueryHistory(ctx context.Context, namespace, key string, q HistoryQuery) ([]HistoryEntry, error)
}

// 컴파일 타임 인터페이스 준수 체크.
var _ StoreHistoryQueryer = (*UserStoreAgent)(nil)

// QueryHistory 는 SPEC-CHART-001 REQ-M3-01 (Store HTTP 쿼리 API) 의 진입점이다.
// 지정된 네임스페이스의 Store 인스턴스에서 QueryHistory 를 호출한다.
//
// namespace 가 빈 문자열이면 "default" 로 치환된다. 이로써 HTTP 요청 바디에
// namespace 필드가 생략되었을 때도 의미 있는 동작이 보장된다.
//
// query 는 내부적으로 Validate() 되며, 유효하지 않으면 에러를 반환한다.
// 키가 존재하지 않으면 Store 계층이 ErrKeyNotFound 를 반환하고, 그대로 전파된다.
func (a *UserStoreAgent) QueryHistory(
	ctx context.Context,
	namespace, key string,
	query HistoryQuery,
) ([]HistoryEntry, error) {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}

	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("store query: %w", err)
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("store query: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)
	return store.QueryHistory(ctx, key, query)
}

// ListStoreKeys 는 지정된 네임스페이스의 키 목록을 반환한다.
// pattern 이 빈 문자열이면 전체 키를 반환한다.
func (a *UserStoreAgent) ListStoreKeys(
	ctx context.Context,
	namespace, pattern string,
) ([]string, error) {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}
	if pattern == "" {
		pattern = "*"
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("store list keys: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)
	return store.Keys(ctx, pattern)
}

// @spec SPEC-STORE-003
// KeyTags 는 이 에이전트에 정의된 정적 키 → 태그 맵 전체 복사본을 반환한다.
// 정적 키가 하나도 없으면 빈 맵을 반환한다.
// 반환 맵은 호출자 전용 복사본으로, 에이전트 내부 상태와 분리되어 있다.
func (a *UserStoreAgent) KeyTags(_ context.Context) (map[string]map[string]string, error) {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("store key tags: agent is not initialized")
	}
	return inner.StaticKeyTags(), nil
}

// @spec SPEC-STORE-003
// ClearHistory 는 지정된 네임스페이스/키의 히스토리만 비우고 엔트리(value/ttl 등) 는 보존한다.
// 정책 분기(정적 키만 호출) 는 호출자(HTTP 핸들러) 의 책임이다.
//
// namespace 가 빈 문자열이면 "default" 로 치환된다.
// 키가 존재하지 않으면 ErrKeyNotFound 를 반환한다.
func (a *UserStoreAgent) ClearHistory(ctx context.Context, namespace, key string) error {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return fmt.Errorf("store clear history: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)
	return store.ClearHistory(ctx, key)
}

// @spec SPEC-STORE-003
// DeleteEntry 는 지정된 네임스페이스/키의 엔트리(값+히스토리) 를 모두 삭제한다.
// 동적 키 reset 시 핸들러가 이 메서드를 호출한다.
//
// namespace 가 빈 문자열이면 "default" 로 치환된다.
// 키가 존재하지 않더라도 에러를 반환하지 않는다 (Store.Delete 와 일관).
func (a *UserStoreAgent) DeleteEntry(ctx context.Context, namespace, key string) error {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return fmt.Errorf("store delete entry: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)
	return store.Delete(ctx, key)
}

// @spec SPEC-STORE-003
// IsStaticKey 는 사용자 관점 key 가 정적 키 목록에 정의되어 있는지 검사한다.
// HTTP 핸들러가 reset 정책을 결정할 때 사용한다.
func (a *UserStoreAgent) IsStaticKey(key string) bool {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return false
	}
	tags := inner.StaticTagsFor(key)
	// StaticTagsFor 는 정적 키가 아니면 빈 맵을 반환하지만, 정적 키가 빈 태그 맵을
	// 가질 수도 있으므로 별도 확인이 필요하다.
	if len(tags) > 0 {
		return true
	}
	all := inner.StaticKeyTags()
	_, ok := all[key]
	return ok
}

// @spec SPEC-STORE-003
// StaticTagPairs 는 모든 정적 키의 태그를 (태그 key → 정렬된 unique value 목록) 으로
// 집계하여 반환한다. /store/{agent_name}/tags 엔드포인트에 사용된다.
// 정적 키가 없거나 태그가 하나도 없으면 빈 맵을 반환한다.
func (a *UserStoreAgent) StaticTagPairs() map[string][]string {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return map[string][]string{}
	}
	all := inner.StaticKeyTags()

	// 태그 key → value 집합(중복 제거용).
	agg := make(map[string]map[string]struct{})
	for _, tags := range all {
		for tk, tv := range tags {
			if _, ok := agg[tk]; !ok {
				agg[tk] = make(map[string]struct{})
			}
			agg[tk][tv] = struct{}{}
		}
	}

	out := make(map[string][]string, len(agg))
	for tk, values := range agg {
		list := make([]string, 0, len(values))
		for v := range values {
			list = append(list, v)
		}
		// 안정적인 UI 표시를 위해 오름차순 정렬.
		sort.Strings(list)
		out[tk] = list
	}
	return out
}
