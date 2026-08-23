// @spec SPEC-TSDB-003 §2.3 (U3) · §2.5 (U5) · §2.7 (U7) · §2.8 (U8)
//
// 시리즈 열거(D5) 의 v3 경로 테스트다. 쿼리 생성과 행 접기가 모두 순수 함수이므로
// 네트워크 없이 전수 검증한다 — plan.md M3 의 조합 축은
// (사전 필터 0/1/N) × (태그 키 0/1/N) × (행 상한 지정/기본) 이다.
//
// **v3 는 httptest 왕복을 쓸 수 없다.** influxdb3-go 클라이언트는 Arrow Flight
// (gRPC) 로 질의하므로 net/http/httptest 서버로 대체할 수 없다. 대신 M2 가
// fluxQueryRunner 로 세운 규율을 따라 2단 호출을 함수 인자로 주입해 코어를
// 네트워크 없이 검증한다. 실 서버 왕복은 §HISTORY-0.5.0 의 실측 픽스처
// (TestFoldEnumRowsV3_실측JSON_회귀) 가 대신 잠근다.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== 공통 픽스처 =====

// f3EnumSpecV3 는 v3 쿼리 생성 테스트의 공통 입력이다.
// v2 의 f3EnumSpec 과 같은 값을 쓰되 bucket 은 v3 에서 쓰이지 않는다.
func f3EnumSpecV3() SeriesEnumSpec {
	return SeriesEnumSpec{
		Measurement: "cpu",
		StartMs:     1700000000000,
		EndMs:       1702592000000,
		Tags:        map[string]string{"region": "kr"},
		RowLimit:    20000,
	}
}

// ===== 컴파일 타임 계약 =====

// TestInfluxSeriesEnum_V3_인터페이스_준수 는 v3 클라이언트가 M3 이후
// InfluxSeriesEnumerator 를 만족함을 고정한다. M2 시점에는 만족하지 않았고
// 에이전트가 ErrSeriesEnumerationNotSupported 를 반환했다.
func TestInfluxSeriesEnum_V3_인터페이스_준수(t *testing.T) {
	t.Parallel()
	var _ InfluxSeriesEnumerator = (*influxV3Client)(nil)
}

// ===== v3 InfluxQL 쿼리 생성 (§2.5) =====

// TestBuildInfluxQLSeriesEnumQuery_Shape 는 F3 입력에 대한 생성 문자열이
// §2.5 의 2단계 템플릿과 바이트 단위로 일치함을 단언한다.
//
// 시간 술어가 **나노초 정수**인 것은 의도적이다. 실측에서 정수 술어가 동작함을
// 확인했고(§HISTORY-0.5.0), v2 경로의 time(v: <ns>) 와 단위가 같아진다.
func TestBuildInfluxQLSeriesEnumQuery_Shape(t *testing.T) {
	t.Parallel()

	got, err := BuildInfluxQLSeriesEnumQuery(f3EnumSpecV3())
	require.NoError(t, err)

	want := `SELECT * FROM "cpu"` + "\n" +
		` WHERE time >= 1700000000000000000 AND time < 1702592000000000000` + "\n" +
		`   AND "region" = 'kr'` + "\n" +
		` GROUP BY * LIMIT 20000`
	assert.Equal(t, want, got)
}

// TestBuildInfluxQLSeriesEnumQuery_SHOW_SERIES_미발행 은 UB1-1 을 기계 검증한다
// (plan.md M3.5). 공식 문서 두 곳이 미지원을 명시하며, 실측에서도 파싱 단계에서
// 거부되었다(§HISTORY-0.5.0 (1)).
func TestBuildInfluxQLSeriesEnumQuery_SHOW_SERIES_미발행(t *testing.T) {
	t.Parallel()

	specs := []SeriesEnumSpec{
		f3EnumSpecV3(),
		{Measurement: "m", StartMs: 1, EndMs: 2},
		{Measurement: "m", StartMs: 1, EndMs: 2, Tags: map[string]string{"a": "b", "c": "d"}},
	}
	for _, s := range specs {
		got, err := BuildInfluxQLSeriesEnumQuery(s)
		require.NoError(t, err)
		assert.NotContains(t, strings.ToUpper(got), "SHOW SERIES")
		assert.Contains(t, got, "GROUP BY *")
	}
}

// TestBuildInfluxQLSeriesEnumQuery_사전필터_0_1_N 는 사전 필터 개수 축을 덮는다.
// 맵 순회는 비결정적이므로 키 오름차순 고정이 관측 대상이다.
func TestBuildInfluxQLSeriesEnumQuery_사전필터_0_1_N(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags map[string]string
		want string
	}{
		{
			name: "0개",
			tags: nil,
			want: `SELECT * FROM "cpu"` + "\n" +
				` WHERE time >= 1000000 AND time < 2000000` + "\n" +
				` GROUP BY * LIMIT 20000`,
		},
		{
			name: "1개",
			tags: map[string]string{"host": "a"},
			want: `SELECT * FROM "cpu"` + "\n" +
				` WHERE time >= 1000000 AND time < 2000000` + "\n" +
				`   AND "host" = 'a'` + "\n" +
				` GROUP BY * LIMIT 20000`,
		},
		{
			name: "N개_키_오름차순",
			tags: map[string]string{"zone": "z", "host": "a", "region": "kr"},
			want: `SELECT * FROM "cpu"` + "\n" +
				` WHERE time >= 1000000 AND time < 2000000` + "\n" +
				`   AND "host" = 'a'` + "\n" +
				`   AND "region" = 'kr'` + "\n" +
				`   AND "zone" = 'z'` + "\n" +
				` GROUP BY * LIMIT 20000`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildInfluxQLSeriesEnumQuery(SeriesEnumSpec{
				Measurement: "cpu",
				StartMs:     1,
				EndMs:       2,
				Tags:        tt.tags,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildInfluxQLSeriesEnumQuery_행상한 은 원시 행 상한이 [1, 20000] 으로
// 접히는지 확인한다(§2.7). 상한을 서버에 두는 이유는 클라이언트 값이 백엔드
// 계산량을 무제한으로 늘릴 수 없어야 하기 때문이다.
func TestBuildInfluxQLSeriesEnumQuery_행상한(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  string
	}{
		{"미지정은_기본상한", 0, " GROUP BY * LIMIT 20000"},
		{"음수는_기본상한", -5, " GROUP BY * LIMIT 20000"},
		{"상한초과는_절삭", 999999, " GROUP BY * LIMIT 20000"},
		{"범위내는_보존", 500, " GROUP BY * LIMIT 500"},
		{"경계값_20000", 20000, " GROUP BY * LIMIT 20000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := f3EnumSpecV3()
			spec.RowLimit = tt.limit
			got, err := BuildInfluxQLSeriesEnumQuery(spec)
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(got, tt.want), "got=%q want suffix %q", got, tt.want)
		})
	}
}

// TestBuildInfluxQLSeriesEnumQuery_버킷_미지정_허용 은 v3 가 bucket 을 요구하지
// 않음을 고정한다. database 는 클라이언트 생성 시 고정되며 쿼리 문자열에
// 삽입되지 않는다 — BuildInfluxQLSeriesQuery 와 같은 규약이다.
func TestBuildInfluxQLSeriesEnumQuery_버킷_미지정_허용(t *testing.T) {
	t.Parallel()

	spec := f3EnumSpecV3()
	spec.Bucket = ""
	got, err := BuildInfluxQLSeriesEnumQuery(spec)
	require.NoError(t, err)
	assert.NotContains(t, got, "bucket")

	// bucket 이 지정돼도 쿼리에 나타나지 않는다.
	spec.Bucket = "metrics"
	got2, err := BuildInfluxQLSeriesEnumQuery(spec)
	require.NoError(t, err)
	assert.Equal(t, got, got2)
}

// TestBuildInfluxQLSeriesEnumQuery_형상_거부 는 식별자 내용과 무관한 형상 오류를
// 확인한다. validateShape 를 v2 와 공유하므로 판정이 갈리지 않는다.
func TestBuildInfluxQLSeriesEnumQuery_형상_거부(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec SeriesEnumSpec
		msg  string
	}{
		{"measurement_없음", SeriesEnumSpec{StartMs: 1, EndMs: 2}, "measurement is required"},
		{"창_역전", SeriesEnumSpec{Measurement: "cpu", StartMs: 5, EndMs: 5}, "end_ms must be greater"},
		{"창_음수폭", SeriesEnumSpec{Measurement: "cpu", StartMs: 9, EndMs: 3}, "end_ms must be greater"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildInfluxQLSeriesEnumQuery(tt.spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.msg)
		})
	}
}

// TestBuildInfluxQLSeriesEnumQuery_식별자_거부 는 이스케이프 불가 식별자가
// ErrUnescapableIdentifier 로 거부됨을 확인한다(§2.3 · UB1-9). HTTP 계층이 이
// 센티넬을 400 으로 매핑한다.
func TestBuildInfluxQLSeriesEnumQuery_식별자_거부(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec SeriesEnumSpec
	}{
		{"measurement_제어문자", SeriesEnumSpec{Measurement: "cp\nu", StartMs: 1, EndMs: 2}},
		{"태그키_제어문자", SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2,
			Tags: map[string]string{"ho\tst": "a"}}},
		{"태그값_제어문자", SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2,
			Tags: map[string]string{"host": "a\x00b"}}},
		{"bucket_제어문자", SeriesEnumSpec{Bucket: "me\ntrics", Measurement: "cpu", StartMs: 1, EndMs: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildInfluxQLSeriesEnumQuery(tt.spec)
			assert.ErrorIs(t, err, ErrUnescapableIdentifier)
		})
	}
}

// TestBuildInfluxQLSeriesEnumQuery_이스케이프 는 따옴표·백슬래시가 방언 규칙대로
// 이스케이프됨을 확인한다. 식별자는 큰따옴표, 태그 값은 작은따옴표 리터럴이므로
// 이스케이프 대상이 다르다 — 기존 escapeInfluxQLIdent ·
// escapeInfluxQLStringLiteral 을 그대로 재사용하며 신규 이스케이프 함수를
// 만들지 않는다(§2.3 · AC-08).
func TestBuildInfluxQLSeriesEnumQuery_이스케이프(t *testing.T) {
	t.Parallel()

	got, err := BuildInfluxQLSeriesEnumQuery(SeriesEnumSpec{
		Measurement: `cp"u\x`,
		StartMs:     1,
		EndMs:       2,
		Tags:        map[string]string{`ho"st`: `a'b\c`},
	})
	require.NoError(t, err)

	assert.Contains(t, got, `FROM "cp\"u\\x"`)
	assert.Contains(t, got, `AND "ho\"st" = 'a\'b\\c'`)
}

// TestBuildInfluxQLSeriesEnumQuery_점이_든_태그키 는 §HISTORY-0.4.0 이 관측한
// 점 든 태그 키가 v3 경로에서도 깨지지 않음을 잠근다. InfluxQL 식별자는
// 큰따옴표로 감싸므로 점이 경로 접근으로 해석되지 않는다.
func TestBuildInfluxQLSeriesEnumQuery_점이_든_태그키(t *testing.T) {
	t.Parallel()

	got, err := BuildInfluxQLSeriesEnumQuery(SeriesEnumSpec{
		Measurement: "temperature",
		StartMs:     1,
		EndMs:       2,
		Tags:        map[string]string{"device.type": "AM103"},
	})
	require.NoError(t, err)
	assert.Contains(t, got, `AND "device.type" = 'AM103'`)
}

// TestBuildInfluxQLSeriesEnumQuery_창_ns 는 ms → ns 변환을 확인한다.
func TestBuildInfluxQLSeriesEnumQuery_창_ns(t *testing.T) {
	t.Parallel()

	got, err := BuildInfluxQLSeriesEnumQuery(SeriesEnumSpec{
		Measurement: "cpu",
		StartMs:     1785542400000,
		EndMs:       1788220800000,
	})
	require.NoError(t, err)
	assert.Contains(t, got, " WHERE time >= 1785542400000000000 AND time < 1788220800000000000")
}

// ===== v3 행 접기 (§2.5) =====
//
// v2 의 foldEnumRows 를 재사용할 수 없는 이유가 셋이다(§HISTORY-0.5.0 (3)(4)).
// 아래 테스트가 그 셋을 하나씩 잠근다.

// TestFoldEnumRowsV3_제외컬럼 은 iox::measurement 와 time 이 태그로도 필드로도
// 새지 않음을 확인한다. **둘 다 밑줄이 없다** — v2 의 result·table 과 정확히
// 같은 함정 부류이며 밑줄 규칙만으로는 걸러지지 않는다.
func TestFoldEnumRowsV3_제외컬럼(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "cpu", "time": "2026-08-23T00:24:10.754182912", "host": "a", "value": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"value"}, got[0].Fields)
}

// TestFoldEnumRowsV3_밑줄접두_방어 는 밑줄 접두 컬럼을 방어적으로 제외함을
// 확인한다. v3 응답에서 관측되지는 않았으나, 걸러 두면 백엔드가 내부 컬럼을
// 추가했을 때 존재하지 않는 태그가 붙는 사고를 막는다.
func TestFoldEnumRowsV3_밑줄접두_방어(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "cpu", "time": "t", "_internal": "x", "host": "a", "value": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"value"}, got[0].Fields)
}

// TestFoldEnumRowsV3_결손태그는_키_부재 는 v2 와 갈리는 첫 지점을 잠근다.
// v2 는 group() 이 없는 컬럼을 **빈 문자열**로 채우지만, v3 는 **키 자체가 없다**.
func TestFoldEnumRowsV3_결손태그는_키_부재(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "t", "time": "x", "location": "실습실", "point": "복도", "spot": "복도", "value": 1.0},
		{"iox::measurement": "t", "time": "x", "location": "회의실", "value": 2.0}, // point·spot 키 부재
	}
	got := foldEnumRowsV3(rows, []string{"location", "point", "spot"})

	require.Len(t, got, 2)
	// 태그 직렬화 오름차순: "location=실습실,..." < "location=회의실"
	assert.Equal(t, map[string]string{"location": "실습실", "point": "복도", "spot": "복도"}, got[0].Tags)
	assert.Equal(t, map[string]string{"location": "회의실"}, got[1].Tags)
}

// TestFoldEnumRowsV3_시리즈당_다중행_필드_합집합 은 v2 와 갈리는 둘째 지점을
// 잠근다. first() 가 1행을 보장하는 v2 와 달리, 필드가 서로 다른 시각에 기록된
// 시리즈는 시각마다 별도 행으로 온다(실측: multi 의 host=c → 2행).
//
// 접기가 태그 집합 기준으로 중복을 합치고 **필드를 합집합**하지 않으면 같은
// 시리즈가 둘로 쪼개져 선택 표에 중복 행이 나타난다.
func TestFoldEnumRowsV3_시리즈당_다중행_필드_합집합(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "multi", "time": "t1", "host": "a", "humi": 40.0, "temp": 1.5},
		{"iox::measurement": "multi", "time": "t1", "host": "b", "temp": 2.5},
		{"iox::measurement": "multi", "time": "t1", "host": "c", "temp": 3.5},
		{"iox::measurement": "multi", "time": "t2", "host": "c", "humi": 55.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 3, "host=c 의 2행이 1개 시리즈로 접혀야 한다")
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"humi", "temp"}, got[0].Fields)
	assert.Equal(t, map[string]string{"host": "b"}, got[1].Tags)
	assert.Equal(t, []string{"temp"}, got[1].Fields)
	assert.Equal(t, map[string]string{"host": "c"}, got[2].Tags)
	assert.Equal(t, []string{"humi", "temp"}, got[2].Fields, "서로 다른 행의 필드가 합집합되어야 한다")
}

// TestFoldEnumRowsV3_태그와_필드는_타입으로_구분되지_않는다 는 v2 와 갈리는
// 셋째 지점 — 1단계 SHOW TAG KEYS 가 정본이어야 하는 이유 — 를 잠근다.
//
// 실측 measurement `ambig`: host 는 태그, status 는 **문자열 필드**다. 응답의
// JSON 타입만으로는 둘이 구분되지 않으므로, 태그 키 집합 없이 "문자열이면 태그"
// 로 판정하면 status 가 태그가 되어 존재하지 않는 시리즈가 만들어진다.
func TestFoldEnumRowsV3_태그와_필드는_타입으로_구분되지_않는다(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "ambig", "time": "t", "host": "a", "status": "ok", "temp": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"}) // SHOW TAG KEYS FROM "ambig" → host 하나

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags, "status 는 태그가 아니다")
	assert.Equal(t, []string{"status", "temp"}, got[0].Fields, "status 는 필드다")
}

// TestFoldEnumRowsV3_태그키_0개 는 태그 없는 measurement 가 시리즈 1개로
// 접힘을 확인한다(실측: heartbeat). §2.5 의 예외 분기이며 plan.md M3.4 다.
func TestFoldEnumRowsV3_태그키_0개(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "heartbeat", "time": "t", "value": 1.0},
	}
	got := foldEnumRowsV3(rows, nil)

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{}, got[0].Tags)
	assert.Equal(t, []string{"value"}, got[0].Fields)
}

// TestFoldEnumRowsV3_결정적_정렬 은 태그 직렬화 오름차순 정렬을 확인한다
// (§2.2 · UB1-16). 절단이 발생할 때 폴링마다 다른 부분집합이 잘리면 사용자가
// 고른 시리즈가 목록에서 사라졌다 나타났다 한다.
func TestFoldEnumRowsV3_결정적_정렬(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"time": "t", "host": "z", "value": 1.0},
		{"time": "t", "host": "a", "value": 1.0},
		{"time": "t", "host": "m", "value": 1.0},
	}
	want := []string{"a", "m", "z"}
	for i := 0; i < 20; i++ {
		got := foldEnumRowsV3(rows, []string{"host"})
		require.Len(t, got, 3)
		for j, w := range want {
			assert.Equal(t, w, got[j].Tags["host"])
		}
	}
}

// TestFoldEnumRowsV3_빈입력 은 행이 없을 때 빈 슬라이스(nil 아님)를 반환함을
// 확인한다. nil 을 반환하면 JSON 직렬화가 `null` 이 되어 프런트가 `.length` 에서
// 터진다.
func TestFoldEnumRowsV3_빈입력(t *testing.T) {
	t.Parallel()

	got := foldEnumRowsV3(nil, []string{"host"})
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// TestFoldEnumRowsV3_nil_값 은 nil 값의 처분을 확인한다.
// nil 태그 값은 "없음"이므로 태그가 아니고, nil 필드 값은 관측되지 않은
// 필드이므로 필드 목록에 넣지 않는다 — 넣으면 존재하지 않는 (field, 태그)
// 조합이 후보가 된다(§4.2 의 조용한 오답).
func TestFoldEnumRowsV3_nil_값(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"time": "t", "host": "a", "region": nil, "value": 1.0, "absent": nil},
	}
	got := foldEnumRowsV3(rows, []string{"host", "region"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"value"}, got[0].Fields)
}

// TestFoldEnumRowsV3_빈문자열_태그값_방어 는 빈 문자열 태그 값을 태그로 보지
// 않음을 확인한다. v3 는 결손을 키 부재로 표현하므로 관측되지 않은 형상이지만,
// InfluxDB 가 빈 태그 값을 저장하지 않는다는 사실은 양쪽 백엔드에 공통이다.
func TestFoldEnumRowsV3_빈문자열_태그값_방어(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"time": "t", "host": "a", "region": "", "value": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host", "region"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
}

// TestFoldEnumRowsV3_비문자열_태그값 은 문자열이 아닌 태그 값도 버리지 않음을
// 확인한다. 태그를 조용히 버리면 서로 다른 두 시리즈가 하나로 합쳐진다.
func TestFoldEnumRowsV3_비문자열_태그값(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"time": "t", "host": 42, "value": 1.0},
		{"time": "t", "host": 43, "value": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 2)
	assert.Equal(t, "42", got[0].Tags["host"])
	assert.Equal(t, "43", got[1].Tags["host"])
}

// TestFoldEnumRowsV3_빈_컬럼명 은 빈 키를 무시함을 확인한다.
func TestFoldEnumRowsV3_빈_컬럼명(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"": "x", "time": "t", "host": "a", "value": 1.0},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"value"}, got[0].Fields)
}

// TestFoldEnumRowsV3_필드없는_행도_태그집합을_만든다 는 필드가 하나도 없는 행이
// 빈 Fields 를 가진 시리즈를 만듦을 확인한다. 태그 집합은 실재하므로 버리면
// 안 된다.
func TestFoldEnumRowsV3_필드없는_행도_태그집합을_만든다(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"iox::measurement": "m", "time": "t", "host": "a"},
	}
	got := foldEnumRowsV3(rows, []string{"host"})

	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Empty(t, got[0].Fields)
	assert.NotNil(t, got[0].Fields)
}

// ===== 2단 오케스트레이션 (§2.5) =====

// TestEnumerateSeriesWithInfluxQL_2단_호출 은 §2.5 의 2단 경로가 순서대로
// 호출되고 인자가 옳음을 확인한다.
func TestEnumerateSeriesWithInfluxQL_2단_호출(t *testing.T) {
	t.Parallel()

	var order []string
	var gotMeasurement, gotQuery string

	res, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2, RowLimit: 100},
		func(_ context.Context, _, measurement string) ([]string, error) {
			order = append(order, "tagkeys")
			gotMeasurement = measurement
			return []string{"host"}, nil
		},
		func(_ context.Context, query string) ([]map[string]any, error) {
			order = append(order, "query")
			gotQuery = query
			return []map[string]any{
				{"iox::measurement": "cpu", "time": "t", "host": "a", "usage": 1.0},
			}, nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{"tagkeys", "query"}, order, "태그 키 조회가 먼저다 — 그 집합이 행 키 분류의 정본이다")
	assert.Equal(t, "cpu", gotMeasurement)
	assert.Contains(t, gotQuery, `SELECT * FROM "cpu"`)
	assert.Contains(t, gotQuery, "GROUP BY * LIMIT 100")

	require.Len(t, res.Series, 1)
	assert.Equal(t, map[string]string{"host": "a"}, res.Series[0].Tags)
	assert.Equal(t, []string{"usage"}, res.Series[0].Fields)
}

// TestEnumerateSeriesWithInfluxQL_FieldExact_false 는 OQ3 확정을 고정한다.
//
// 행에서 시리즈별 필드를 실제로 관측하므로 정확도는 높지만, LIMIT 이 표본을
// 자르면 필드가 누락될 수 있어 관측된 필드 집합은 **하한**이다(§HISTORY-0.5.0 (5)).
// v2 는 true 이며 그 비대칭이 능력 표에 드러난다(§2.6).
func TestEnumerateSeriesWithInfluxQL_FieldExact_false(t *testing.T) {
	t.Parallel()

	res, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2},
		func(_ context.Context, _, _ string) ([]string, error) { return nil, nil },
		func(_ context.Context, _ string) ([]map[string]any, error) { return nil, nil })

	require.NoError(t, err)
	assert.False(t, res.FieldExact)
}

// TestEnumerateSeriesWithInfluxQL_생성실패는_질의하지_않는다 는 순수 생성이
// 먼저 검증되어 잘못된 입력이 네트워크를 타지 않음을 확인한다.
func TestEnumerateSeriesWithInfluxQL_생성실패는_질의하지_않는다(t *testing.T) {
	t.Parallel()

	called := false
	_, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "", StartMs: 1, EndMs: 2},
		func(_ context.Context, _, _ string) ([]string, error) {
			called = true
			return nil, nil
		},
		func(_ context.Context, _ string) ([]map[string]any, error) {
			called = true
			return nil, nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement is required")
	assert.False(t, called, "생성이 실패하면 네트워크를 타지 않아야 한다")
}

// TestEnumerateSeriesWithInfluxQL_태그키_실패_전파 는 1단계 실패가 원인과 함께
// 전파됨을 확인한다.
func TestEnumerateSeriesWithInfluxQL_태그키_실패_전파(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("tag keys boom")
	queried := false
	_, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2},
		func(_ context.Context, _, _ string) ([]string, error) { return nil, sentinel },
		func(_ context.Context, _ string) ([]map[string]any, error) {
			queried = true
			return nil, nil
		})

	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "cpu")
	assert.False(t, queried, "태그 키 집합 없이 행을 분류할 수 없다")
}

// TestEnumerateSeriesWithInfluxQL_질의_실패_전파 는 2단계 실패가 원인과 함께
// 전파됨을 확인한다.
func TestEnumerateSeriesWithInfluxQL_질의_실패_전파(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("query boom")
	_, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2},
		func(_ context.Context, _, _ string) ([]string, error) { return []string{"host"}, nil },
		func(_ context.Context, _ string) ([]map[string]any, error) { return nil, sentinel })

	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "cpu")
}

// TestEnumerateSeriesWithInfluxQL_태그키_0개_measurement 는 태그 키가 0개일 때
// 시리즈 1개(빈 태그 맵)로 접힘을 end-to-end 로 확인한다(plan.md M3.4).
func TestEnumerateSeriesWithInfluxQL_태그키_0개_measurement(t *testing.T) {
	t.Parallel()

	res, err := enumerateSeriesWithInfluxQL(context.Background(),
		SeriesEnumSpec{Measurement: "heartbeat", StartMs: 1, EndMs: 2},
		func(_ context.Context, _, _ string) ([]string, error) { return nil, nil },
		func(_ context.Context, _ string) ([]map[string]any, error) {
			return []map[string]any{
				{"iox::measurement": "heartbeat", "time": "t1", "value": 1.0},
				{"iox::measurement": "heartbeat", "time": "t2", "value": 2.0},
			}, nil
		})

	require.NoError(t, err)
	require.Len(t, res.Series, 1)
	assert.Equal(t, map[string]string{}, res.Series[0].Tags)
	assert.Equal(t, []string{"value"}, res.Series[0].Fields)
}

// ===== v3 클라이언트 =====

// TestInfluxV3Client_EnumerateSeries_미지원센티넬_아님 은 M3 의 반전을 잠근다.
//
// M2 시점에는 v3 클라이언트가 InfluxSeriesEnumerator 를 만족하지 않아 에이전트가
// ErrSeriesEnumerationNotSupported 를 반환했다. 이제는 만족하므로, 실패하더라도
// 그 실패가 **미지원 센티넬이어서는 안 된다** — TestInfluxV3Client_ListMeasurements_501해제
// 와 같은 형태의 단언이다.
func TestInfluxV3Client_EnumerateSeries_미지원센티넬_아님(t *testing.T) {
	t.Parallel()

	c := &influxV3Client{}
	_, err := c.EnumerateSeries(context.Background(),
		SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	require.Error(t, err, "nil 클라이언트이므로 질의는 실패한다")
	assert.NotErrorIs(t, err, ErrSeriesEnumerationNotSupported,
		"v3 의 시리즈 열거는 InfluxQL GROUP BY * 로 동작해야 한다")
}

// TestInfluxV3Client_EnumerateSeries_생성실패 는 잘못된 spec 이 nil 클라이언트에
// 닿기 전에 거부됨을 확인한다.
func TestInfluxV3Client_EnumerateSeries_생성실패(t *testing.T) {
	t.Parallel()

	c := &influxV3Client{}
	_, err := c.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "", StartMs: 1, EndMs: 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement is required")
}

// ===== M1 실측 회귀 (SPEC-TSDB-003 §HISTORY-0.5.0) =====

// influx3ObservedTemperatureJSON 은 로컬 influxdb:3-core 인스턴스에 대해
// BuildInfluxQLSeriesEnumQuery 가 생성한 질의를 실행하고 **관측한 응답 원문**이다
// (db=metrics, measurement=temperature). 한 글자도 손대지 않았다.
//
// 이 픽스처가 잠그는 관측 사실 4가지:
//  1. iox::measurement · time 이 실재한다 — 제외하지 않으면 모든 시리즈에
//     존재하지 않는 태그/필드가 붙는다
//  2. 결손 태그는 **키 자체가 없다**(AM103-089152 행에 point·spot 부재)
//  3. 점이 든 태그 키가 그대로 온다(device.dev_eui 등 4종)
//  4. 태그 7개 시리즈 3개 + 태그 5개 시리즈 1개 = 4개 시리즈로 접힌다
const influx3ObservedTemperatureJSON = `[{"iox::measurement":"temperature","time":"2026-08-23T00:24:10.754182912","device.dev_eui":"24e124126d152590","device.id":"9a383ec1-e8a6-4608-9005-76ff441f461f","device.name":"EM500-CO2-152590","device.type":"EM500-CO2","location":"실습실","point":"전방 좌측","spot":"전방 좌측","value":24.3},{"iox::measurement":"temperature","time":"2026-08-23T00:24:10.754182912","device.dev_eui":"24e124136d151523","device.id":"0d67adc6-e88c-420a-bd30-c657f9e5a78e","device.name":"EM300-TH-151523","device.type":"EM300-TH","location":"실습실","point":"복도","spot":"복도","value":26.9},{"iox::measurement":"temperature","time":"2026-08-23T00:24:10.754182912","device.dev_eui":"24e124725d089152","device.id":"557874e2-f644-4512-9237-0f4c53adc135","device.name":"AM103-089152","device.type":"AM103","location":"회의실","value":26.2},{"iox::measurement":"temperature","time":"2026-08-23T00:24:10.754182912","device.dev_eui":"24e124785c389010","device.id":"bc790e8f-1de7-43b6-9cc9-965075bbe804","device.name":"EM320-TH-389010","device.type":"EM320-TH","location":"사무실","point":"사무공간 스위치 옆","spot":"사무공간 스위치 옆","value":26.8}]`

// influx3ObservedTemperatureTagKeys 는 같은 인스턴스에서
// `SHOW TAG KEYS FROM "temperature"` 가 돌려준 tagKey 컬럼 값 7개다(관측 순서 그대로).
var influx3ObservedTemperatureTagKeys = []string{
	"device.dev_eui", "device.id", "device.name", "device.type", "location", "point", "spot",
}

// TestFoldEnumRowsV3_실측JSON_회귀 는 실측 응답을 그대로 접어 4개 시리즈가
// 나오는지 확인한다. v3 클라이언트가 Arrow Flight 로 질의하므로 httptest 왕복이
// 불가능한 대신, 이 픽스처가 실 서버 응답 형상을 잠근다.
func TestFoldEnumRowsV3_실측JSON_회귀(t *testing.T) {
	t.Parallel()

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(influx3ObservedTemperatureJSON), &rows))
	require.Len(t, rows, 4)

	got := foldEnumRowsV3(rows, influx3ObservedTemperatureTagKeys)
	require.Len(t, got, 4, "4개 시리즈로 접혀야 한다")

	// (1) 구조/메타 컬럼이 태그로도 필드로도 새지 않는다.
	for _, s := range got {
		assert.NotContains(t, s.Tags, "iox::measurement")
		assert.NotContains(t, s.Tags, "time")
		assert.NotContains(t, s.Fields, "iox::measurement")
		assert.NotContains(t, s.Fields, "time")
		assert.Equal(t, []string{"value"}, s.Fields)
	}

	// (2) 결손 태그 시리즈는 5개, 나머지는 7개다.
	sizes := map[int]int{}
	for _, s := range got {
		sizes[len(s.Tags)]++
	}
	assert.Equal(t, map[int]int{7: 3, 5: 1}, sizes,
		"point·spot 이 없는 AM103-089152 만 태그 5개다")

	// (3) 점이 든 태그 키가 보존된다.
	found := false
	for _, s := range got {
		if s.Tags["device.name"] == "AM103-089152" {
			found = true
			assert.NotContains(t, s.Tags, "point")
			assert.NotContains(t, s.Tags, "spot")
			assert.Equal(t, "회의실", s.Tags["location"])
			assert.Equal(t, "AM103", s.Tags["device.type"])
		}
	}
	assert.True(t, found, "device.name 태그가 점 표기 그대로 보존되어야 한다")
}
