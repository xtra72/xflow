package system

import (
	"context"
	"errors"
	"fmt"
)

// @spec SPEC-STORE-004
// store_series_reset.go — M4 단일 시리즈 reset/삭제 + 시리즈 모델 정합.
//
// M2 이후 저장 키와 레지스트리(staticKeys)는 EncodeSeriesKey 인코딩으로 키잉되므로,
// 사용자 관점 key 에 대한 reset 은 "그 key 아래의 시리즈들" 을 식별하여 시리즈 단위로
// 처리해야 한다(E8). 본 파일은 (namespace, key, metricFilter, tagsFilter) 로 대상 시리즈를
// 좁혀 정적/동적 정책을 시리즈 단위로 적용하는 ResetSeries 와, IsStaticKey 의 사용자 관점
// 보조 의미를 제공한다.

// ResetSeries 는 (namespace, key) 아래 필터에 일치하는 모든 시리즈를 reset 한다 (E8/AC-15).
//
// 식별자(필터) 의미 — QuerySeries 의 seriesMatchesFilter 와 동일 규칙:
//   - metricFilter 와 tagsFilter 가 모두 비어있으면 해당 key 의 **모든 시리즈**가 대상이다.
//     이것이 "식별자 누락 시 정책" 의 확정 결정이다: 사용자가 metric/tags 없이 "이 key 삭제"
//     를 요청하면, 그 key 의 전 시리즈(모든 metric/tags 조합)를 reset 한다. 이는 시리즈
//     도입 이전의 "key 1개 = 아이템 1개" 시절 사용자 직관("이 키를 지운다")을 보존한다.
//   - metricFilter 만 주어지면 그 metric 의 모든 tags 조합 시리즈가 대상이다(부분집합).
//   - metricFilter + tagsFilter 가 주어지면 더 좁혀 일치하는 단일/부분집합 시리즈가 대상이다.
//
// 시리즈별 정책 (현재 reset 정책을 시리즈 단위로 보존):
//   - 정적 시리즈(레지스트리에 등록된 키, IsStaticKey 의 직접 조회 적중) → ClearHistory
//     (값/엔트리 보존, 히스토리만 비움) → historyCleared 증가.
//   - 동적 시리즈(레지스트리 미등록 키) → DeleteEntry (엔트리+히스토리 삭제) → entriesDeleted 증가.
//
// 다른 시리즈에 대한 무영향(AC-15): 필터에 일치하지 않는 시리즈는 전혀 건드리지 않는다.
//
// best-effort: 개별 시리즈 처리 중 ErrKeyNotFound(동시 만료/삭제)는 건너뛴다. 그 외 도메인
// 에러는 즉시 반환한다. 일치 시리즈가 0개면 (0, 0, nil) 을 반환한다(S4 정합 — 에러 아님).
//
// namespace 빈 값은 "default" 로 정규화된다.
func (a *UserStoreAgent) ResetSeries(
	ctx context.Context,
	namespace, key, metricFilter string,
	tagsFilter map[string]string,
) (historyCleared, entriesDeleted int, err error) {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return 0, 0, fmt.Errorf("store reset series: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)

	// 네임스페이스의 모든 (접두사 제거된) 저장 키를 열거하여 시리즈로 디코드·매칭한다.
	rawKeys, listErr := store.Keys(ctx, "*")
	if listErr != nil {
		return 0, 0, listErr
	}

	for _, rawKey := range rawKeys {
		sid := decodeStorageKeyToSeries(rawKey)
		if !seriesMatchesFilter(sid, key, metricFilter, tagsFilter) {
			continue
		}

		// 정적/동적 분기는 저장 키(rawKey, 인코딩 또는 bare) 기준으로 판단한다.
		// IsStaticKey 는 레지스트리 존재 여부(직접 조회)로 정적 여부를 결정한다(characterization).
		if a.IsStaticKey(rawKey) {
			if cerr := store.ClearHistory(ctx, rawKey); cerr != nil {
				if errors.Is(cerr, ErrKeyNotFound) {
					continue
				}
				return historyCleared, entriesDeleted, cerr
			}
			historyCleared++
			continue
		}

		if derr := store.Delete(ctx, rawKey); derr != nil {
			if errors.Is(derr, ErrKeyNotFound) {
				continue
			}
			return historyCleared, entriesDeleted, derr
		}
		entriesDeleted++
	}

	return historyCleared, entriesDeleted, nil
}
