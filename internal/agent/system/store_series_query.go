package system

import (
	"context"
	"errors"
	"fmt"
)

// @spec SPEC-STORE-004
// store_series_query.go — M3 조회 경로 시리즈 fan-out.
//
// 한 key 아래 여러 (field, tags) 시리즈가 독립 저장되므로(M2), 조회는 key →
// 다중 시리즈 fan-out 으로 확장된다. 저장 키는 EncodeSeriesKey 로 인코딩되어 있으므로
// DecodeSeriesKey 로 디코드하여 SeriesID.Key == 조회 key 인 시리즈들을 모은다.
//
// 레거시/기본 키 호환: 메타 없는 일반 Set 으로 쓰여 시리즈 인코딩이 아닌 bare key
// (구분자 `|` 없음) 는 DecodeSeriesKey 가 실패한다. 이 경우 해당 raw key 를
// SeriesID{Measurement: raw}(field=unknown, tags 없음) 기본 시리즈로 해석하여 흡수한다.
// 이로써 plain Set 데이터와 시리즈 쓰기 데이터가 동일 조회 경로에서 함께 노출된다.

// SeriesResult 는 단일 시리즈의 조회 결과이다.
// Series 는 정규화된 시리즈 식별자이며, Entries 는 해당 시리즈의 시계열(최신순, 현재값 포함)이다.
type SeriesResult struct {
	Series  SeriesID
	Entries []HistoryEntry
}

// decodeStorageKeyToSeries 는 네임스페이스 접두사가 제거된 저장 키를 시리즈 식별자로
// 해석한다. 인코딩된 시리즈 키는 DecodeSeriesKey 로 복원하고, 비-시리즈(bare) 키는
// 기본 시리즈 SeriesID{Measurement: raw} 로 해석한다(레거시/plain Set 흡수).
func decodeStorageKeyToSeries(rawKey string) SeriesID {
	if sid, err := DecodeSeriesKey(rawKey); err == nil {
		return sid
	}
	// 비-시리즈 키: 기본 시리즈로 정규화(field=unknown, tags={}).
	return SeriesID{Measurement: rawKey}.Normalize()
}

// seriesMatchesFilter 는 시리즈가 (key, fieldFilter, tagsFilter) 조건을 만족하는지 검사한다.
//
// 규칙 (E5/S3):
//   - SeriesID.Key 는 조회 key 와 정확히 일치해야 한다.
//   - fieldFilter 가 빈 문자열이면 field 축은 통과(no-op), 아니면 정확히 일치해야 한다.
//   - tagsFilter 의 각 (k,v) 는 시리즈 tags 에 정확히 존재해야 한다(부분집합 AND 매칭).
//     tagsFilter 가 비어있으면 tags 축은 통과(no-op) → 해당 field 의 모든 tags 조합 시리즈.
func seriesMatchesFilter(sid SeriesID, key, fieldFilter string, tagsFilter map[string]string) bool {
	if sid.Measurement != key {
		return false
	}
	if fieldFilter != "" && sid.Field != fieldFilter {
		return false
	}
	for tk, tv := range tagsFilter {
		if got, ok := sid.Tags[tk]; !ok || got != tv {
			return false
		}
	}
	return true
}

// QuerySeries 는 지정된 (namespace, key) 아래 필터에 일치하는 모든 시리즈의 시계열을
// fan-out 으로 조회한다 (E4/E5/S3/S4).
//
// 동작:
//   - fieldFilter/tagsFilter 가 모두 비어있으면 해당 key 의 **모든 시리즈**를 반환한다(E4).
//   - fieldFilter 만 주어지면 그 field 의 **모든 tags 조합 시리즈**를 반환한다(S3).
//   - field+tags 가 주어지면 일치하는 **단일/부분집합 시리즈**만 반환한다(E5).
//   - 어떤 시리즈도 일치하지 않으면 빈 슬라이스를 반환한다(S4, 에러 아님).
//
// namespace 빈 값은 "default" 로 정규화된다(쓰기 경로 NodeStoreForNamespace 와 정합).
// query 는 각 시리즈에 동일 적용되며, Validate() 에 실패하면 에러를 반환한다.
//
// 반환 결과는 시리즈 키 인코딩의 오름차순으로 결정적 정렬되어, 동일 입력에 동일 순서를 보장한다.
func (a *UserStoreAgent) QuerySeries(
	ctx context.Context,
	namespace, key, fieldFilter string,
	tagsFilter map[string]string,
	query HistoryQuery,
) ([]SeriesResult, error) {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("store series query: %w", err)
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("store series query: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)

	// 네임스페이스의 모든 (접두사 제거된) 저장 키를 열거한다.
	rawKeys, err := store.Keys(ctx, "*")
	if err != nil {
		return nil, err
	}

	// 결정적 순서를 위해 시리즈 인코딩 키 기준 정렬 후 매칭한다.
	results := make([]SeriesResult, 0, len(rawKeys))
	for _, rawKey := range rawKeys {
		sid := decodeStorageKeyToSeries(rawKey)
		if !seriesMatchesFilter(sid, key, fieldFilter, tagsFilter) {
			continue
		}

		entries, qErr := store.QueryHistory(ctx, rawKey, query)
		if qErr != nil {
			// 키가 조회 직전에 만료/삭제되었을 수 있다(S4: 빈 결과로 흡수, 에러 아님).
			if isKeyNotFound(qErr) {
				continue
			}
			return nil, qErr
		}
		results = append(results, SeriesResult{Series: sid, Entries: entries})
	}

	sortSeriesResults(results)
	return results, nil
}

// isKeyNotFound 는 에러가 ErrKeyNotFound 계열인지 판별한다.
func isKeyNotFound(err error) bool {
	return errors.Is(err, ErrKeyNotFound)
}

// sortSeriesResults 는 시리즈 결과를 시리즈 인코딩 키 오름차순으로 정렬한다(결정적 순서).
func sortSeriesResults(results []SeriesResult) {
	// 작은 N(한 key 의 시리즈 수)에 대한 단순 삽입 정렬. 안정적·결정적.
	for i := 1; i < len(results); i++ {
		j := i
		for j > 0 && EncodeSeriesKey(results[j-1].Series) > EncodeSeriesKey(results[j].Series) {
			results[j-1], results[j] = results[j], results[j-1]
			j--
		}
	}
}
