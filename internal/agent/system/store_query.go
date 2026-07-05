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
	if err := store.Delete(ctx, key); err != nil {
		return err
	}
	// @spec SPEC-STORE-004: 값 엔트리뿐 아니라 레지스트리 메타(staticKeys)도 제거하여
	// 동적 키가 완전히 사라지게 한다(전체 초기화 시 동적 키 삭제 요구). manual 정적 키는
	// reset 정책상 DeleteEntry 경로를 타지 않으므로(IsStaticKey=true → ClearHistory) 영향 없다.
	inner.RemoveStaticKey(key)
	return nil
}

// @spec SPEC-STORE-003
// @spec SPEC-STORE-004
// IsStaticKey 는 입력 key 가 정적(레지스트리 등록) 키인지 검사한다. reset 정책 분기
// (정적 → ClearHistory / 동적 → DeleteEntry) 에 사용된다.
//
// 시리즈 모델(M2 이후)에서의 의미 — 두 단계 판정:
//
//  1. 직접 조회(primary): key 가 레지스트리(staticKeys)에 그대로 존재하면 true.
//     레지스트리 키는 (a) 시리즈 인코딩 키(SetWithMeta 경로) 또는 (b) bare key(yaml 정적/
//     plain Set 으로 등록된 키)이다. ResetAll/ResetSeries 가 저장 키(인코딩 또는 bare)를
//     그대로 넘기면 이 경로가 적중한다. 이 동작은 SPEC-STORE-003 시절과 byte-identical 하게
//     보존된다(회귀 0).
//
//  2. 사용자 관점 보조 의미(fallback): 직접 조회가 빗나가면 key 를 "사용자 관점 key" 로 보고,
//     레지스트리에 디코드 후 SeriesID.Key == key 인 시리즈가 하나라도 존재하면 true 를
//     반환한다. 즉 "그 key 의 어떤 시리즈라도 정적이면 true". 이는 시리즈 인코딩을 모르는
//     호출자(예: 사용자 key 만 가진 코드)가 정적 여부를 물을 수 있게 하는 가산적 의미이며,
//     primary 경로를 침범하지 않는다(직접 적중 시 fallback 미실행).
//
// 정적성 정의(SPEC-STORE-004 수정): 정적 = Source=manual(yaml/수동 정의) 인 키만이다.
// 동적(Source=auto, 런타임 자동 등록) 키는 정적이 아니며, reset 시 DeleteEntry 경로로
// 값과 레지스트리 메타가 함께 제거된다("전체 초기화 시 동적 키 삭제" 요구). 과거에는
// 레지스트리 존재 여부로 판정해 동적 키도 정적으로 취급(ClearHistory 보존)되던 버그가 있었다.
func (a *UserStoreAgent) IsStaticKey(key string) bool {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return false
	}

	// 1) 직접 조회(primary): key 가 레지스트리에 그대로 있으면 그 Source 로 판정.
	//    (ResetAll/ResetSeries 는 저장 키 — 인코딩 또는 bare — 를 그대로 넘긴다.)
	if meta, ok := inner.StaticKeyMetaFor(key); ok {
		return meta.Source == SourceManual
	}

	// 2) fallback(사용자 관점): 미등록 key 면, 그 key 의 어떤 시리즈라도 manual 이면 true.
	if key == "" {
		return false
	}
	for regKey, meta := range inner.StaticKeysSnapshot() {
		if meta.Source != SourceManual {
			continue
		}
		if regKey == key || decodeStorageKeyToSeries(regKey).Key == key {
			return true
		}
	}
	return false
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
