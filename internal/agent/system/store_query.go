package system

import (
	"context"
	"fmt"
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
