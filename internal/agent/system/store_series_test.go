package system

import (
	"sync"
	"testing"
)

// @spec SPEC-STORE-004
// store_series_test.go — M1 시리즈 식별/인코딩 기반의 TDD 테스트.
// SeriesID 정규화(tags 정렬, metric_type 기본값)와 EncodeSeriesKey 의 결정성·단사성을 검증한다.
// 본 단계는 순수 신규 토대이며 기존 동작/시그니처를 전혀 건드리지 않는다.

// -----------------------------------------------------------------------------
// 정규화: tags 순서 무관 동일 인코딩 (U2)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_TagOrderInvariance 는 tag 입력 순서가 달라도 동일한 인코딩이
// 생성됨을 검증한다 (AC-3 / U2 정렬 불변성).
func TestEncodeSeriesKey_TagOrderInvariance(t *testing.T) {
	a := SeriesID{
		Key:        "temp",
		MetricType: "temperature",
		Tags:       map[string]string{"room": "1", "floor": "2"},
	}
	b := SeriesID{
		Key:        "temp",
		MetricType: "temperature",
		Tags:       map[string]string{"floor": "2", "room": "1"},
	}

	if got, want := EncodeSeriesKey(a), EncodeSeriesKey(b); got != want {
		t.Fatalf("tag 순서만 다른 식별자가 다른 인코딩을 생성함: %q vs %q", got, want)
	}
}

// -----------------------------------------------------------------------------
// 정규화: metric_type 빈 값 → unknown (A3)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_EmptyMetricDefaultsUnknown 은 metric_type 빈 값이
// MetricTypeUnknown 으로 보정되어 명시적 "unknown" 과 동일하게 인코딩됨을 검증한다.
func TestEncodeSeriesKey_EmptyMetricDefaultsUnknown(t *testing.T) {
	empty := SeriesID{Key: "k", MetricType: "", Tags: nil}
	explicit := SeriesID{Key: "k", MetricType: MetricTypeUnknown, Tags: nil}

	if got, want := EncodeSeriesKey(empty), EncodeSeriesKey(explicit); got != want {
		t.Fatalf("빈 metric_type 가 unknown 으로 보정되지 않음: %q vs %q", got, want)
	}
}

// -----------------------------------------------------------------------------
// 정규화: Normalize 메서드의 직접 검증
// -----------------------------------------------------------------------------

// TestSeriesID_Normalize 는 Normalize 가 metric_type 기본값과 tags 깊은 복사를
// 적용하고, 원본을 변형하지 않음을 검증한다.
func TestSeriesID_Normalize(t *testing.T) {
	orig := SeriesID{Key: "dev", MetricType: "", Tags: map[string]string{"a": "1"}}
	norm := orig.Normalize()

	if norm.MetricType != MetricTypeUnknown {
		t.Fatalf("빈 metric_type 가 %q 로 보정되지 않음: %q", MetricTypeUnknown, norm.MetricType)
	}
	if norm.Key != "dev" {
		t.Fatalf("key 가 보존되지 않음: %q", norm.Key)
	}
	if norm.Tags["a"] != "1" {
		t.Fatalf("tags 가 보존되지 않음: %v", norm.Tags)
	}

	// 원본 metric_type 은 변형되지 않아야 한다(값 복사 시맨틱).
	if orig.MetricType != "" {
		t.Fatalf("Normalize 가 원본 metric_type 을 변형함: %q", orig.MetricType)
	}
}

// -----------------------------------------------------------------------------
// 식별: 같은 식별자 → 같은 키, 다른 식별자 → 다른 키 (U4 / N5)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_Determinism 은 같은 (key, metric, tags) 가 항상 같은 인코딩을
// 생성함을 검증한다 (U4 결정성).
func TestEncodeSeriesKey_Determinism(t *testing.T) {
	s := SeriesID{Key: "room", MetricType: "temperature", Tags: map[string]string{"area": "a"}}
	first := EncodeSeriesKey(s)
	for i := 0; i < 100; i++ {
		if got := EncodeSeriesKey(s); got != first {
			t.Fatalf("동일 식별자가 비결정적 인코딩을 생성함: %q vs %q", got, first)
		}
	}
}

// TestEncodeSeriesKey_Injectivity 는 key/metric/tags 중 하나라도 다르면 인코딩이
// 달라짐을 검증한다 (N5 충돌 회피, 단사성).
func TestEncodeSeriesKey_Injectivity(t *testing.T) {
	cases := []SeriesID{
		{Key: "room", MetricType: "temperature", Tags: nil},
		{Key: "room", MetricType: "humidity", Tags: nil},                               // metric 만 다름
		{Key: "room", MetricType: "temperature", Tags: map[string]string{"area": "a"}}, // tags 만 다름
		{Key: "room", MetricType: "temperature", Tags: map[string]string{"area": "b"}}, // tag value 만 다름
		{Key: "room", MetricType: "temperature", Tags: map[string]string{"zone": "a"}}, // tag key 만 다름
		{Key: "other", MetricType: "temperature", Tags: nil},                           // key 만 다름
		{Key: "room", MetricType: "temperature", Tags: map[string]string{"a": "1", "b": "2"}},
		{Key: "room", MetricType: "temperature", Tags: map[string]string{"a": "1"}},
	}

	seen := make(map[string]int)
	for i, s := range cases {
		enc := EncodeSeriesKey(s)
		if j, dup := seen[enc]; dup {
			t.Fatalf("서로 다른 식별자가 같은 인코딩으로 충돌: case[%d]=%+v 와 case[%d]=%+v → %q",
				i, s, j, cases[j], enc)
		}
		seen[enc] = i
	}
}

// -----------------------------------------------------------------------------
// 기본 시리즈: SeriesID{Key:k} == SeriesID{Key:k, MetricType:"unknown"} (AC-16)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_DefaultSeriesEquivalence 는 메타 미지정 단일 키가 명시적
// "unknown"/빈 tags 기본 시리즈와 동일하게 인코딩됨을 검증한다(점진 전환 안전성).
func TestEncodeSeriesKey_DefaultSeriesEquivalence(t *testing.T) {
	bare := SeriesID{Key: "simple"}
	explicit := SeriesID{Key: "simple", MetricType: MetricTypeUnknown, Tags: map[string]string{}}
	nilTags := SeriesID{Key: "simple", MetricType: "", Tags: nil}

	encBare := EncodeSeriesKey(bare)
	encExplicit := EncodeSeriesKey(explicit)
	encNil := EncodeSeriesKey(nilTags)

	if encBare != encExplicit {
		t.Fatalf("기본 시리즈 인코딩 불일치: bare=%q explicit=%q", encBare, encExplicit)
	}
	if encBare != encNil {
		t.Fatalf("기본 시리즈 인코딩 불일치: bare=%q nilTags=%q", encBare, encNil)
	}
}

// -----------------------------------------------------------------------------
// 엣지: 특수문자 포함 key 에서 metric/tags 경계 모호성 없음 (N5)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_ArbitraryKeyNoBoundaryAmbiguity 는 구분자 유사 문자(`|`, `,`,
// `=`)를 포함할 수 있는 디바이스 ID 형태의 key 에서도 단사성이 유지됨을 검증한다.
// metric_type/tags 는 ^[a-zA-Z0-9_-]+$ 라 구분자를 포함하지 않으므로, 임의 key 가
// 구분자를 포함해도 경계가 모호해지지 않아야 한다 (AC-18).
func TestEncodeSeriesKey_ArbitraryKeyNoBoundaryAmbiguity(t *testing.T) {
	// 의도적으로 인코딩 구분자(`|`, `,`, `=`)와 충돌을 노리는 key 들.
	cases := []SeriesID{
		{Key: "dev|a", MetricType: "temperature", Tags: nil},
		{Key: "dev", MetricType: "temperature", Tags: map[string]string{"x": "a"}}, // "temperature||dev" 와 충돌 시도
		{Key: "a|b|c", MetricType: "humidity", Tags: map[string]string{"r": "1"}},
		{Key: "uuid-1234.5678", MetricType: "temperature", Tags: nil},
		{Key: "k,m=v", MetricType: "temperature", Tags: nil},
		{Key: "", MetricType: "temperature", Tags: nil}, // 빈 key 도 다른 식별자로 취급
	}

	seen := make(map[string]int)
	for i, s := range cases {
		enc := EncodeSeriesKey(s)
		if j, dup := seen[enc]; dup {
			t.Fatalf("임의 key 경계 모호성으로 충돌: case[%d]=%+v 와 case[%d]=%+v → %q",
				i, s, j, cases[j], enc)
		}
		seen[enc] = i
	}
}

// -----------------------------------------------------------------------------
// 엣지: 빈 tags / 다수 tags
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_TagCardinality 는 빈 tags 와 다수 tags 가 모두 결정적으로
// 인코딩되며 서로 다름을 검증한다.
func TestEncodeSeriesKey_TagCardinality(t *testing.T) {
	none := SeriesID{Key: "k", MetricType: "m", Tags: map[string]string{}}
	one := SeriesID{Key: "k", MetricType: "m", Tags: map[string]string{"a": "1"}}
	many := SeriesID{Key: "k", MetricType: "m", Tags: map[string]string{"a": "1", "b": "2", "c": "3"}}

	encs := map[string]string{
		"none": EncodeSeriesKey(none),
		"one":  EncodeSeriesKey(one),
		"many": EncodeSeriesKey(many),
	}
	seen := make(map[string]string)
	for name, enc := range encs {
		if prev, dup := seen[enc]; dup {
			t.Fatalf("tag cardinality 충돌: %s 와 %s 가 같은 인코딩 %q", name, prev, enc)
		}
		seen[enc] = name
	}
}

// -----------------------------------------------------------------------------
// 라운드트립(선택): 디코딩으로 원 식별자 복원 가능 검증
// -----------------------------------------------------------------------------

// TestDecodeSeriesKey_RoundTrip 은 EncodeSeriesKey 결과를 DecodeSeriesKey 로
// 디코딩하면 정규화된 원 식별자를 복원함을 검증한다(디버깅/조회 보조 용도).
func TestDecodeSeriesKey_RoundTrip(t *testing.T) {
	cases := []SeriesID{
		{Key: "room", MetricType: "temperature", Tags: nil},
		{Key: "a|b|c", MetricType: "humidity", Tags: map[string]string{"r": "1", "z": "2"}},
		{Key: "uuid-1234.5678", MetricType: "", Tags: map[string]string{"area": "a"}},
		{Key: "", MetricType: "temperature", Tags: nil},
	}

	for _, s := range cases {
		enc := EncodeSeriesKey(s)
		dec, err := DecodeSeriesKey(enc)
		if err != nil {
			t.Fatalf("DecodeSeriesKey(%q) 오류: %v", enc, err)
		}
		// 디코딩 결과를 다시 인코딩하면 동일해야 한다(정규화 후 동등성).
		if got := EncodeSeriesKey(dec); got != enc {
			t.Fatalf("round-trip 불일치: 원본=%q 재인코딩=%q", enc, got)
		}
		want := s.Normalize()
		if dec.Key != want.Key || dec.MetricType != want.MetricType {
			t.Fatalf("디코딩 key/metric 불일치: got=%+v want=%+v", dec, want)
		}
		if len(dec.Tags) != len(want.Tags) {
			t.Fatalf("디코딩 tags 개수 불일치: got=%v want=%v", dec.Tags, want.Tags)
		}
		for k, v := range want.Tags {
			if dec.Tags[k] != v {
				t.Fatalf("디코딩 tag 불일치 [%s]: got=%q want=%q", k, dec.Tags[k], v)
			}
		}
	}
}

// TestDecodeSeriesKey_Invalid 는 인코딩 프레임/태그 구분자가 부족한 비정상 입력에
// 대해 ErrInvalidSeriesKey 를 반환함을 검증한다.
func TestDecodeSeriesKey_Invalid(t *testing.T) {
	cases := []string{
		"",            // 구분자 전무
		"metriconly",  // `|` 0개
		"metric|tags", // `|` 1개 (key 프레임 부족)
		"m|a=1,b|k",   // 태그 쌍 "b" 가 `=` 누락
	}
	for _, enc := range cases {
		if _, err := DecodeSeriesKey(enc); err == nil {
			t.Fatalf("비정상 입력 %q 가 오류를 반환하지 않음", enc)
		}
	}
}

// -----------------------------------------------------------------------------
// 동시성: 동시 인코딩 데이터 레이스 없음 (AC-17 보조)
// -----------------------------------------------------------------------------

// TestEncodeSeriesKey_ConcurrentSafe 는 다수 goroutine 이 동시에 인코딩을 호출해도
// 데이터 레이스 없이 동일 식별자가 동일 결과를 내는지 검증한다(go test -race).
func TestEncodeSeriesKey_ConcurrentSafe(t *testing.T) {
	s := SeriesID{Key: "room", MetricType: "temperature", Tags: map[string]string{"a": "1", "b": "2"}}
	want := EncodeSeriesKey(s)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := SeriesID{Key: "room", MetricType: "temperature", Tags: map[string]string{"a": "1", "b": "2"}}
			if got := EncodeSeriesKey(local); got != want {
				t.Errorf("동시 인코딩 결과 불일치: got=%q want=%q", got, want)
			}
		}()
	}
	wg.Wait()
}
