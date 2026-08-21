package system

import (
	"sort"
	"strings"
)

// @spec SPEC-STORE-004
// store_series.go — Store 복합 식별(시리즈) 모델의 식별/인코딩 기반 (M1).
//
// 본 파일은 SPEC-STORE-004 의 "시리즈 식별을 1급 타입으로 도입" 토대를 신설한다.
// 저장 아이템을 단일 string key 가 아니라 (key, field, sorted(tags)) 복합
// 식별자(시리즈)로 식별하기 위한 순수 신규 타입/함수만 정의하며, 기존 동작·시그니처
// (VolatileStore/staticKeys/SetWithMeta/QueryHistory 등)는 전혀 변경하지 않는다.
// 기존 (key) 기반 경로의 시리즈화는 M2 이후의 범위이다.

// SeriesID 는 store 저장 아이템의 복합 식별자(시리즈)이다.
//
// 시리즈 식별자 = (Key, Field, sorted(Tags)) (namespace 스코프 내).
// 같은 Key 라도 Field 또는 (정규화된) Tags 가 하나라도 다르면 다른 시리즈이다.
//
// data_type 은 식별 차원이 아니므로 본 타입에 포함하지 않는다 (U3). data_type 은
// 시리즈가 보관하는 값의 타입 계약일 뿐이며, 같은 (Key, Field, Tags) 이면
// data_type 이 달라도 동일 시리즈로 취급된다.
//
// 필드:
//   - Key: 사용자 관점의 논리 key. 디바이스 UUID 등 임의 문자열일 수 있다
//     (`.`, `-`, 심지어 인코딩 구분자 `|`/`,`/`=` 를 포함할 수도 있다).
//   - Field: 측정 종류. ^[a-zA-Z0-9_-]+$ 정규식을 따르며 빈 값은 정규화 시
//     FieldUnknown("unknown") 으로 보정된다 (A3).
//   - Tags: 라벨 맵. key/value 모두 ^[a-zA-Z0-9_-]+$ 정규식을 따른다 (A3).
type SeriesID struct {
	Measurement string
	Field       string
	Tags        map[string]string
}

// Normalize 는 시리즈 식별자를 결정적 형태로 정규화한 복사본을 반환한다.
//
// 정규화 규칙 (SPEC §시리즈 식별자 정의 및 정규화):
//  1. Field 빈 값 → FieldUnknown ("unknown").
//  2. Tags 는 깊은 복사한다(호출자 변형으로부터 분리). 정렬 자체는 인코딩 시점에
//     수행되므로(맵은 순서가 없다) 여기서는 복사만 한다. nil tags 는 빈 집합으로
//     취급한다.
//  3. data_type 은 식별에 사용하지 않으므로 다루지 않는다 (U3).
//
// 원본 SeriesID 는 변형하지 않는다(값 복사 시맨틱). Normalize 는 인코딩 전에
// 적용되는 멱등(idempotent) 연산이다: Normalize(Normalize(s)) == Normalize(s).
func (s SeriesID) Normalize() SeriesID {
	field := s.Field
	if field == "" {
		field = FieldUnknown
	}

	// Tags 깊은 복사. nil 이면 빈 맵으로 정규화하여 "빈 tags == nil tags" 동등성을 보장한다.
	tags := make(map[string]string, len(s.Tags))
	for k, v := range s.Tags {
		tags[k] = v
	}

	return SeriesID{
		Measurement: s.Measurement,
		Field:       field,
		Tags:        tags,
	}
}

// 시리즈 키 인코딩 구분자.
//
// 인코딩 안전성 근거 (N5): Field 과 tag key/value 는 모두 ^[a-zA-Z0-9_-]+$
// 정규식을 따르므로(A3) 아래 구분자 `|`, `,`, `=` 중 어느 것도 포함할 수 없다.
// 반면 Key 는 임의 문자열이라 구분자를 포함할 수 있다. 따라서 인코딩은 "제약된
// 문자집합을 가진 필드(field, tags)를 앞쪽"에 배치하여, 두 개의 `|` 구분자 위치가
// 항상 결정적(첫 번째 `|`, 두 번째 `|`)이 되도록 한다. 그 뒤(두 번째 `|` 이후)는
// 전부 임의 Key 로 안전하게 흡수된다. 이로써 임의 Key 가 구분자를 포함하더라도
// 경계 모호성이 발생하지 않으며 단사성(injectivity)이 보장된다.
const (
	seriesFieldSep = "|" // field | tags | key 프레임 구분자
	seriesTagSep   = "," // 태그 쌍 사이 구분자
	seriesTagKV    = "=" // 태그 key=value 구분자
)

// EncodeSeriesKey 는 시리즈 식별자를 결정적 string 키로 직렬화한다.
//
// 인코딩 형식: `field | tagsEncoded | key`
//   - tagsEncoded = tag-key 사전순 정렬된 "k1=v1,k2=v2" (빈 tags 면 빈 문자열).
//   - field 빈 값은 "unknown" 으로 보정된다(Normalize).
//
// 성질:
//   - 결정성 (U4): 동일 식별자는 항상 동일 문자열을 생성한다. tag 순서가 달라도
//     정렬 덕분에 동일하다 (U2).
//   - 단사성 (N5): key/field/tags 중 하나라도 다르면 다른 문자열을 생성한다.
//     field/tags 는 구분자를 포함할 수 없으므로(A3) 두 `|` 위치가 결정적이고,
//     임의 key 가 구분자를 포함해도 충돌이 없다.
//   - 기본 시리즈: SeriesID{Measurement:k} 는 SeriesID{Measurement:k, Field:"unknown", Tags:{}}
//     와 동일하게 인코딩된다(메타 미지정 쓰기 = 단일 기본 시리즈, 점진 전환 안전).
//
// tsdb.BuildSeriesKey 규약과의 정렬(O1) 결정:
//
//	store 의 식별자는 tsdb 에 없는 임의 Key 차원을 포함하므로 tsdb.BuildSeriesKey 의
//	`measurement,tags` 형식과 byte-identical 하게 만들 수 없다(임의 key 가 measurement
//	위치에 오면 구분자 충돌로 N5 위반). 대신 "태그 인코딩 하위 규약"만 tsdb 와 정렬한다:
//	정렬된 `k1=v1,k2=v2` (콤마 join, `=` KV) 는 tsdb.BuildSeriesKey 의 태그 부분과
//	동일한 문법이다. 바깥 프레임(field|tags|key)만 store 고유로 둔다. 이로써 공유
//	하위 문법은 유지하면서 임의 key 안전성을 확보한다.
func EncodeSeriesKey(s SeriesID) string {
	n := s.Normalize()

	var b strings.Builder
	b.WriteString(n.Field)
	b.WriteString(seriesFieldSep)
	b.WriteString(encodeTags(n.Tags))
	b.WriteString(seriesFieldSep)
	b.WriteString(n.Measurement)
	return b.String()
}

// encodeTags 는 tags 를 tag-key 사전순으로 정렬한 "k1=v1,k2=v2" 문자열로 직렬화한다.
// 빈/ nil tags 는 빈 문자열을 반환한다. 이 하위 문법은 tsdb.BuildSeriesKey 의 태그
// 부분과 동일하다 (O1 부분 정렬).
func encodeTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(seriesTagSep)
		}
		b.WriteString(k)
		b.WriteString(seriesTagKV)
		b.WriteString(tags[k])
	}
	return b.String()
}

// DecodeSeriesKey 는 EncodeSeriesKey 의 역연산으로, 인코딩된 키를 정규화된
// 시리즈 식별자로 복원한다(디버깅/조회 보조 용도, M1 필수 아님이나 round-trip 보장).
//
// 디코딩은 두 `|` 구분자 위치가 결정적이라는 인코딩 불변식에 의존한다:
// 첫 번째 `|` 이전 = field, 두 번째 `|` 이전 = tagsEncoded, 그 이후 = key.
// field/tags 가 `|` 를 포함하지 않으므로(A3) SplitN(.., 3) 분해는 안전하다.
//
// 비정상 입력(구분자 부족)은 ErrInvalidSeriesKey 를 반환한다.
func DecodeSeriesKey(encoded string) (SeriesID, error) {
	parts := strings.SplitN(encoded, seriesFieldSep, 3)
	if len(parts) != 3 {
		return SeriesID{}, ErrInvalidSeriesKey
	}

	field, tagsEncoded, key := parts[0], parts[1], parts[2]
	tags, err := decodeTags(tagsEncoded)
	if err != nil {
		return SeriesID{}, err
	}

	// 이미 인코딩 시 정규화되었으므로 그대로 정규화 형태로 반환한다.
	return SeriesID{
		Measurement: key,
		Field:       field,
		Tags:        tags,
	}, nil
}

// decodeTags 는 "k1=v1,k2=v2" 문자열을 tags 맵으로 복원한다. 빈 문자열은 빈 맵을
// 반환한다. tag key/value 가 `,`/`=` 를 포함하지 않으므로(A3) 분해는 안전하다.
func decodeTags(encoded string) (map[string]string, error) {
	tags := map[string]string{}
	if encoded == "" {
		return tags, nil
	}
	for _, pair := range strings.Split(encoded, seriesTagSep) {
		kv := strings.SplitN(pair, seriesTagKV, 2)
		if len(kv) != 2 {
			return nil, ErrInvalidSeriesKey
		}
		tags[kv[0]] = kv[1]
	}
	return tags, nil
}
