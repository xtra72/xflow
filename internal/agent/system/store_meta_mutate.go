// @spec SPEC-STORE-003 v0.4.0
//
// store_meta_mutate.go 는 임의 엔트리(정적 + 동적)의 metric_type / tags 를 런타임에
// 설정하는 UserStoreAgent 진입점을 제공한다. Web UI 의 "타입/태그 지정" 기능이
// PUT /store/{agent}/keys/{key}/meta 핸들러를 통해 이 경로를 사용한다.
package system

import (
	"fmt"
)

// SetKeyMeta 는 지정된 key 의 metric_type 과 tags 를 설정/갱신한다.
//
// 검증:
//   - key 는 비어있지 않아야 한다.
//   - metricType 은 빈 문자열(→ "unknown" normalize) 이거나 ^[a-zA-Z0-9_-]+$ 를 만족해야 한다.
//   - tags 의 각 key 는 ^[a-zA-Z0-9_-]+$ 를 만족해야 한다 (value 는 임의 문자열 허용).
//
// 동적으로 등록된 키에도 적용 가능하며, 미등록 키이면 동적 string 키로 신규 등록된다.
// 정적(명시 data_type) 키의 DataType/Source 와 동적 키의 string DataType 은 보존된다.
//
// 반환:
//   - ErrInvalidMetricType: metric_type 정규식 위반
//   - ErrInvalidTagKey:     tag key 정규식 위반
//   - 그 외 입력 오류는 일반 error 로 반환
func (a *UserStoreAgent) SetKeyMeta(key string, metricType string, tags map[string]string) error {
	if key == "" {
		return fmt.Errorf("store set key meta: key must be non-empty")
	}

	// metric_type 검증/normalize (빈 문자열 → "unknown").
	normalized, err := validateMetricType(metricType)
	if err != nil {
		return fmt.Errorf("%w: metric_type=%q", ErrInvalidMetricType, metricType)
	}

	// tags key 검증.
	for tk := range tags {
		if !tagKeyPattern.MatchString(tk) {
			return fmt.Errorf("%w: tag=%q", ErrInvalidTagKey, tk)
		}
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return fmt.Errorf("store set key meta: agent is not initialized")
	}

	inner.SetKeyMeta(key, normalized, tags)
	return nil
}
