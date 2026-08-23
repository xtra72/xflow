// @spec SPEC-TSDB-003 §2.3 (U3) · §2.4 (U4) · §2.7 (U7) · §2.8 (U8)
//
// 시리즈 열거(D5) 의 v2 경로 테스트다. 쿼리 생성과 행 접기가 모두 순수 함수이므로
// 네트워크 없이 전수 검증한다 — plan.md M2 의 조합 축은
// (사전 필터 0/1/N) × (bucket 지정/미지정) × (창 지정/기본) 이다.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== 공통 픽스처 =====

// f3EnumSpec 은 acceptance.md 의 F3 픽스처다(쿼리 생성 테스트의 공통 입력).
func f3EnumSpec() SeriesEnumSpec {
	return SeriesEnumSpec{
		Bucket:      "metrics",
		Measurement: "cpu",
		StartMs:     1700000000000,
		EndMs:       1702592000000,
		Tags:        map[string]string{"region": "kr"},
		RowLimit:    20000,
	}
}

// ===== 컴파일 타임 계약 (AC-48) =====

// TestInfluxSeriesEnum_인터페이스_준수 는 열거가 InfluxSchemaDiscoverer 를 넓히지
// 않고 **별도 인터페이스**로 존재함을 고정한다(plan.md §4 위험 R5).
func TestInfluxSeriesEnum_인터페이스_준수(t *testing.T) {
	t.Parallel()
	var _ InfluxSeriesEnumerator = (*InfluxDBAgent)(nil)
	var _ InfluxSeriesEnumerator = (*influxV2Client)(nil)
	// v3 는 influxdb_seriesenum_v3_test.go 가 단언한다. 여기서는 다루지 않는다.
}

// ===== v2 Flux 쿼리 생성 (AC-09 · AC-21 · AC-25) =====

// TestBuildFluxSeriesEnumQuery_Shape 는 F3 입력에 대한 생성 문자열이 §2.4 의
// 그룹 키 파이프라인과 바이트 단위로 일치함을 단언한다(AC-09).
func TestBuildFluxSeriesEnumQuery_Shape(t *testing.T) {
	t.Parallel()

	got, err := BuildFluxSeriesEnumQuery(f3EnumSpec())
	require.NoError(t, err)

	want := `from(bucket: "metrics")` + "\n" +
		`  |> range(start: time(v: 1700000000000000000), stop: time(v: 1702592000000000000))` + "\n" +
		`  |> filter(fn: (r) => r._measurement == "cpu")` + "\n" +
		`  |> filter(fn: (r) => r["region"] == "kr")` + "\n" +
		`  |> first()` + "\n" +
		`  |> group()` + "\n" +
		`  |> limit(n: 20000)`
	assert.Equal(t, want, got)
}

// TestBuildFluxSeriesEnumQuery_last를_쓰지_않는다 는 OQ10 확정(first 채택)을
// 고정한다. last() 로 바뀌면 푸시다운은 유지되나 테스트 기대값이 흔들린다.
func TestBuildFluxSeriesEnumQuery_last를_쓰지_않는다(t *testing.T) {
	t.Parallel()

	got, err := BuildFluxSeriesEnumQuery(f3EnumSpec())
	require.NoError(t, err)
	assert.Contains(t, got, "|> first()")
	assert.NotContains(t, got, "|> last()")
	// distinct() 는 푸시다운 표에 없다(§2.4 · AC-10).
	assert.NotContains(t, got, "distinct(")
}

// TestBuildFluxSeriesEnumQuery_WindowNs 는 epoch ms → epoch ns 변환 정확도를
// 검증한다(AC-25). range() 는 ns 를 받으므로 ×10^6 이 빠지면 창이 1970 년으로
// 접힌다 — 증상이 "빈 결과"이므로 오류로 보이지 않는다.
func TestBuildFluxSeriesEnumQuery_WindowNs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		startMs, endMs int64
		wantStartNs    int64
		wantEndNs      int64
	}{
		{"F3 창", 1700000000000, 1702592000000, 1700000000000000000, 1702592000000000000},
		{"1ms 창", 1, 2, 1000000, 2000000},
		{"음수 시작(epoch 이전)", -1000, 1000, -1000000000, 1000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := f3EnumSpec()
			spec.StartMs, spec.EndMs = tt.startMs, tt.endMs
			got, err := BuildFluxSeriesEnumQuery(spec)
			require.NoError(t, err)
			assert.Contains(t, got,
				fmt.Sprintf("range(start: time(v: %d), stop: time(v: %d))", tt.wantStartNs, tt.wantEndNs))
		})
	}
}

// TestBuildFluxSeriesEnumQuery_사전필터_0_1_N 은 plan.md M2 조합 축의 첫 축이다.
// 필터 행은 태그 키 오름차순으로 고정된다 — 맵 순회는 비결정적이므로 정렬이
// 없으면 같은 입력이 매번 다른 쿼리를 만든다.
func TestBuildFluxSeriesEnumQuery_사전필터_0_1_N(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags map[string]string
		want []string
		deny []string
	}{
		{
			name: "0개 — 필터 행 없음",
			tags: nil,
			deny: []string{`r["`},
		},
		{
			name: "1개",
			tags: map[string]string{"region": "kr"},
			want: []string{`  |> filter(fn: (r) => r["region"] == "kr")` + "\n"},
		},
		{
			name: "N개 — 키 오름차순",
			tags: map[string]string{"region": "kr", "host": "a", "az": "z1"},
			want: []string{
				`  |> filter(fn: (r) => r["az"] == "z1")` + "\n" +
					`  |> filter(fn: (r) => r["host"] == "a")` + "\n" +
					`  |> filter(fn: (r) => r["region"] == "kr")` + "\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := f3EnumSpec()
			spec.Tags = tt.tags
			got, err := BuildFluxSeriesEnumQuery(spec)
			require.NoError(t, err)
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
			for _, d := range tt.deny {
				assert.NotContains(t, got, d)
			}
		})
	}
}

// TestBuildFluxSeriesEnumQuery_버킷_미지정은_오류 는 조합 축의 둘째 축이다.
// Flux 의 from() 은 bucket 을 구조적으로 요구한다 — 기본값 채움은 호출자
// (에이전트/클라이언트) 책임이며 여기까지 빈 값이 오면 버그다.
// BuildFluxSeriesQuery(§SPEC-TSDB-002 §2.7) 와 같은 규율이다.
func TestBuildFluxSeriesEnumQuery_버킷_미지정은_오류(t *testing.T) {
	t.Parallel()

	spec := f3EnumSpec()
	spec.Bucket = ""
	_, err := BuildFluxSeriesEnumQuery(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

// TestBuildFluxSeriesEnumQuery_형상_거부 는 measurement 누락과 뒤집힌 창을
// 거부함을 확인한다(§2.2 오류 매핑 표의 400 두 줄).
func TestBuildFluxSeriesEnumQuery_형상_거부(t *testing.T) {
	t.Parallel()

	t.Run("measurement 누락", func(t *testing.T) {
		t.Parallel()
		spec := f3EnumSpec()
		spec.Measurement = ""
		_, err := BuildFluxSeriesEnumQuery(spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "measurement")
	})

	t.Run("end <= start", func(t *testing.T) {
		t.Parallel()
		spec := f3EnumSpec()
		spec.EndMs = spec.StartMs
		_, err := BuildFluxSeriesEnumQuery(spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "end_ms")
	})
}

// TestBuildFluxSeriesEnumQuery_행상한 은 원시 행 상한이 생성 쿼리에 들어감을
// 확인한다(AC-21). 0 이하는 기본값으로, 상한 초과는 상한으로 절삭한다 —
// 클라이언트가 보낸 값이 백엔드 계산량을 무제한으로 늘릴 수 없어야 한다(§2.7).
func TestBuildFluxSeriesEnumQuery_행상한(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  string
	}{
		{"미지정(0) → 기본 20000", 0, "  |> limit(n: 20000)"},
		{"음수 → 기본 20000", -5, "  |> limit(n: 20000)"},
		{"명시 100", 100, "  |> limit(n: 100)"},
		{"상한 초과 → 20000 절삭", 999999, "  |> limit(n: 20000)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := f3EnumSpec()
			spec.RowLimit = tt.limit
			got, err := BuildFluxSeriesEnumQuery(spec)
			require.NoError(t, err)
			assert.Contains(t, got, tt.want)
		})
	}
}

// TestSeriesEnumQuery_RejectsUnescapableIdentifier 는 이스케이프 불가 식별자가
// ErrUnescapableIdentifier 로 거부됨을 확인한다(AC-08). 신규 이스케이프 함수를
// 만들지 않고 M4(SPEC-TSDB-002) 의 헬퍼를 재사용한 결과다(§2.3).
func TestSeriesEnumQuery_RejectsUnescapableIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mut  func(*SeriesEnumSpec)
	}{
		{"measurement 제어문자", func(s *SeriesEnumSpec) { s.Measurement = "cp\nu" }},
		{"bucket 제어문자", func(s *SeriesEnumSpec) { s.Bucket = "met\x00rics" }},
		{"태그 키 제어문자", func(s *SeriesEnumSpec) { s.Tags = map[string]string{"ho\tst": "a"} }},
		{"태그 값 제어문자", func(s *SeriesEnumSpec) { s.Tags = map[string]string{"host": "a\x1b"} }},
		{"measurement 비 UTF-8", func(s *SeriesEnumSpec) { s.Measurement = string([]byte{0xff, 0xfe}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := f3EnumSpec()
			tt.mut(&spec)
			_, err := BuildFluxSeriesEnumQuery(spec)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrUnescapableIdentifier)
		})
	}
}

// TestBuildFluxSeriesEnumQuery_이스케이프 는 사용자 데이터가 Flux 문자열을
// 벗어나지 못함을 확인한다(§2.3 · UB1-9). ${ 는 Flux 가 보간으로 해석하므로
// 함께 이스케이프되어야 한다.
func TestBuildFluxSeriesEnumQuery_이스케이프(t *testing.T) {
	t.Parallel()

	spec := SeriesEnumSpec{
		Bucket:      `b"1`,
		Measurement: `m"2`,
		StartMs:     1,
		EndMs:       2,
		Tags:        map[string]string{`t${x}`: `v"3`},
	}
	got, err := BuildFluxSeriesEnumQuery(spec)
	require.NoError(t, err)
	assert.Contains(t, got, `from(bucket: "b\"1")`)
	assert.Contains(t, got, `r._measurement == "m\"2"`)
	assert.Contains(t, got, `r["t\${x}"] == "v\"3"`)
}

// ===== 탐색 창 기본값 (AC-24 · §2.8 · OQ5 확정: 30일) =====

// TestResolveSeriesEnumWindow 는 조합 축의 셋째 축(창 지정/기본)이다.
// now 를 인자로 받아 순수 함수로 두었다 — time.Now() 를 내부에서 읽으면
// 테스트가 시계에 의존한다.
func TestResolveSeriesEnumWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()
	thirtyDaysMs := int64(30 * 24 * 60 * 60 * 1000)

	tests := []struct {
		name               string
		startMs, endMs     int64
		wantStart, wantEnd int64
	}{
		{"둘 다 미지정 → now-30d ~ now", 0, 0, nowMs - thirtyDaysMs, nowMs},
		{"끝만 미지정 → now 로 채움", 1700000000000, 0, 1700000000000, nowMs},
		{"시작만 미지정 → end-30d 로 채움", 0, 1700000000000, 1700000000000 - thirtyDaysMs, 1700000000000},
		{"둘 다 지정 → 보존", 1700000000000, 1702592000000, 1700000000000, 1702592000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStart, gotEnd := ResolveSeriesEnumWindow(tt.startMs, tt.endMs, now)
			assert.Equal(t, tt.wantStart, gotStart, "start")
			assert.Equal(t, tt.wantEnd, gotEnd, "end")
		})
	}
}

// ===== 행 접기 (AC-11 · AC-12 · AC-06) =====

// TestFoldEnumRows_ExcludesInternalColumns 는 밑줄 접두 컬럼이 태그에서
// 제외되고 _field 가 field 로 승격됨을 확인한다(AC-11).
// 판정 규칙은 filterInternalTagKeys(influxdb_schema.go:202) 와 같다.
func TestFoldEnumRows_ExcludesInternalColumns(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{{
		"_start":       "2026-01-01T00:00:00Z",
		"_stop":        "2026-01-31T00:00:00Z",
		"_measurement": "cpu",
		"_field":       "usage",
		"_value":       1,
		"host":         "a",
	}}

	got := foldEnumRows(rows)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"usage"}, got[0].Fields)
}

// TestFoldEnumRows_Flux구조컬럼_제외 는 Flux 주석 CSV 의 구조 컬럼
// (result · table) 이 태그로 오인되지 않음을 확인한다.
//
// 이 둘은 밑줄로 시작하지 않으므로 §2.4 의 "밑줄 접두" 규칙만으로는 걸러지지
// 않는다. 그러나 influxdb-client-go 의 FluxRecord.Values() 는 CSV 의 모든 컬럼을
// 그대로 담으므로(api/query.go:424-433) 걸러내지 않으면 **모든 시리즈에**
// result="_result" · table=0 이라는 존재하지 않는 태그가 붙는다.
// 그 결과는 오류가 아니라 조용한 오답이므로(§4.2) 여기서 막는다.
func TestFoldEnumRows_Flux구조컬럼_제외(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{{
		"result":       "_result",
		"table":        int64(0),
		"_measurement": "cpu",
		"_field":       "usage",
		"host":         "a",
	}}

	got := foldEnumRows(rows)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
}

// TestFoldEnumRows_GroupsByTagSet 는 (field, 태그 집합) 행이 태그 집합 기준으로
// 접힘을 확인한다(AC-12). 행 3개 → 태그 집합 2개(F1). fields 는 사전순이다.
func TestFoldEnumRows_GroupsByTagSet(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"_measurement": "cpu", "_field": "usage", "host": "a", "region": "kr"},
		{"_measurement": "cpu", "_field": "idle", "host": "a", "region": "kr"},
		{"_measurement": "cpu", "_field": "usage", "host": "b", "region": "kr"},
	}

	got := foldEnumRows(rows)
	require.Len(t, got, 2)
	assert.Equal(t, []EnumeratedSeries{
		{Tags: map[string]string{"host": "a", "region": "kr"}, Fields: []string{"idle", "usage"}},
		{Tags: map[string]string{"host": "b", "region": "kr"}, Fields: []string{"usage"}},
	}, got)
}

// TestFoldEnumRows_결정적_정렬 는 입력 순서와 무관하게 같은 출력을 냄을
// 확인한다(AC-06 · UB1-16). 폴링마다 다른 부분집합이 잘리면 사용자가 고른
// 시리즈가 목록에서 사라졌다 나타났다 한다(§2.2).
func TestFoldEnumRows_결정적_정렬(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"_field": "usage", "host": "c"},
		{"_field": "usage", "host": "a"},
		{"_field": "usage", "host": "b"},
	}
	reversed := []map[string]any{rows[2], rows[1], rows[0]}

	got := foldEnumRows(rows)
	gotReversed := foldEnumRows(reversed)

	assert.Equal(t, gotReversed, got)
	require.Len(t, got, 3)
	assert.Equal(t, "a", got[0].Tags["host"])
	assert.Equal(t, "b", got[1].Tags["host"])
	assert.Equal(t, "c", got[2].Tags["host"])
}

// TestFoldEnumRows_태그없는_행은_단일_시리즈 는 태그 키가 0개인 measurement 가
// tags:{} 인 시리즈 1개로 접힘을 확인한다(§2.5 의 예외 분기, v2 대응).
func TestFoldEnumRows_태그없는_행은_단일_시리즈(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"_measurement": "cpu", "_field": "usage", "_value": 1},
		{"_measurement": "cpu", "_field": "idle", "_value": 2},
	}

	got := foldEnumRows(rows)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].Tags)
	assert.Equal(t, []string{"idle", "usage"}, got[0].Fields)
}

// TestFoldEnumRows_빈_태그값은_태그가_아니다 는 빈 문자열 태그 값을 태그로
// 취급하지 않음을 확인한다.
//
// InfluxDB 는 빈 태그 값을 저장하지 않는다 — 값이 빈 태그는 존재하지 않는
// 태그와 같다. group() 이 서로 다른 스키마의 테이블을 한 테이블로 모을 때
// 없는 컬럼이 빈 값으로 채워지므로, 걸러내지 않으면 같은 시리즈가 둘로 쪼개진다.
func TestFoldEnumRows_빈_태그값은_태그가_아니다(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"_field": "usage", "host": "a", "region": ""},
		{"_field": "idle", "host": "a"},
	}

	got := foldEnumRows(rows)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"idle", "usage"}, got[0].Fields)
}

// TestFoldEnumRows_field_없는_행도_태그집합을_만든다 는 _field 컬럼이 없는 행이
// 태그 집합을 만들되 fields 는 비움을 확인한다. 태그 집합의 실재는 field 관측과
// 독립이며, 열거의 1차 산출물은 태그 집합이다(§2.1).
func TestFoldEnumRows_field_없는_행도_태그집합을_만든다(t *testing.T) {
	t.Parallel()

	got := foldEnumRows([]map[string]any{{"host": "a"}})
	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Empty(t, got[0].Fields)
}

// TestFoldEnumRows_빈_컬럼명과_nil_값 은 방어 분기 둘을 고정한다.
//
// 빈 컬럼명은 Flux 주석 CSV 의 무명 컬럼이고, nil 값은 group() 이 서로 다른
// 스키마를 모을 때 생기는 빈 칸이다. 둘 다 태그가 아니며, 태그로 새면
// 존재하지 않는 시리즈가 후보에 섞인다(§4.2).
func TestFoldEnumRows_빈_컬럼명과_nil_값(t *testing.T) {
	t.Parallel()

	got := foldEnumRows([]map[string]any{
		{"": "무명 컬럼", "_field": "usage", "host": "a", "region": nil},
	})
	require.Len(t, got, 1)
	assert.Equal(t, map[string]string{"host": "a"}, got[0].Tags)
	assert.Equal(t, []string{"usage"}, got[0].Fields)
}

// TestFoldEnumRows_빈입력 은 nil/빈 입력이 nil 이 아닌 빈 슬라이스를 냄을
// 확인한다. JSON 직렬화에서 null 과 [] 는 클라이언트에게 다른 뜻이다.
func TestFoldEnumRows_빈입력(t *testing.T) {
	t.Parallel()

	got := foldEnumRows(nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// TestFoldEnumRows_비문자열_태그값 은 문자열이 아닌 컬럼 값도 태그로 보존됨을
// 확인한다. 태그를 조용히 버리면 서로 다른 두 시리즈가 하나로 합쳐진다 —
// §4.2 가 막으려는 조용한 오답과 같은 종류다.
func TestFoldEnumRows_비문자열_태그값(t *testing.T) {
	t.Parallel()

	got := foldEnumRows([]map[string]any{
		{"_field": "usage", "port": int64(8086)},
		{"_field": "usage", "port": int64(9090)},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "8086", got[0].Tags["port"])
	assert.Equal(t, "9090", got[1].Tags["port"])
}

// TestSerializeTagSet_구분자_충돌 은 태그 직렬화가 단사(injective) 임을
// 확인한다. k=v 를 쉼표로 잇기만 하면 값에 = 나 , 가 들어간 서로 다른 태그
// 집합이 같은 키로 접힌다.
func TestSerializeTagSet_구분자_충돌(t *testing.T) {
	t.Parallel()

	a := serializeTagSet(map[string]string{"a": "b,c=d"})
	b := serializeTagSet(map[string]string{"a": "b", "c": "d"})
	assert.NotEqual(t, a, b)
}

// ===== v2 클라이언트 열거 (AC-12) =====

// TestInfluxSeriesEnum_V2_FieldExactTrue 는 v2 경로가 field_exact = true 를
// 반환함을 확인한다(§2.4 · AC-12). 그룹 키에 _field 가 포함되므로 각 태그
// 집합의 field 목록은 근사가 아니라 관측치다.
func TestInfluxSeriesEnum_V2_FieldExactTrue(t *testing.T) {
	t.Parallel()

	var seenQuery string
	run := func(_ context.Context, q string) ([]map[string]any, error) {
		seenQuery = q
		return []map[string]any{
			{"result": "_result", "table": int64(0), "_measurement": "cpu", "_field": "usage", "host": "a", "region": "kr"},
			{"result": "_result", "table": int64(0), "_measurement": "cpu", "_field": "idle", "host": "a", "region": "kr"},
			{"result": "_result", "table": int64(1), "_measurement": "cpu", "_field": "usage", "host": "b", "region": "kr"},
		}, nil
	}

	got, err := enumerateSeriesWithFlux(context.Background(), f3EnumSpec(), run)
	require.NoError(t, err)
	assert.True(t, got.FieldExact)
	assert.Equal(t, []EnumeratedSeries{
		{Tags: map[string]string{"host": "a", "region": "kr"}, Fields: []string{"idle", "usage"}},
		{Tags: map[string]string{"host": "b", "region": "kr"}, Fields: []string{"usage"}},
	}, got.Series)
	assert.Contains(t, seenQuery, `from(bucket: "metrics")`)
}

// TestInfluxSeriesEnum_V2_NoTagKeys 는 태그 키가 0개인 measurement 가 단일
// 항목을 반환함을 확인한다(AC-17 의 v2 대응).
func TestInfluxSeriesEnum_V2_NoTagKeys(t *testing.T) {
	t.Parallel()

	run := func(_ context.Context, _ string) ([]map[string]any, error) {
		return []map[string]any{
			{"result": "_result", "table": int64(0), "_measurement": "cpu", "_field": "usage"},
		}, nil
	}

	got, err := enumerateSeriesWithFlux(context.Background(), f3EnumSpec(), run)
	require.NoError(t, err)
	require.Len(t, got.Series, 1)
	assert.Empty(t, got.Series[0].Tags)
	assert.Equal(t, []string{"usage"}, got.Series[0].Fields)
}

// TestInfluxSeriesEnum_V2_쿼리생성_실패_전파 는 생성 단계 오류가 네트워크를
// 타기 전에 반환됨을 확인한다. HTTP 계층이 400 으로 매핑할 수 있어야 한다.
func TestInfluxSeriesEnum_V2_쿼리생성_실패_전파(t *testing.T) {
	t.Parallel()

	called := false
	run := func(_ context.Context, _ string) ([]map[string]any, error) {
		called = true
		return nil, nil
	}

	spec := f3EnumSpec()
	spec.Measurement = "cp\nu"
	_, err := enumerateSeriesWithFlux(context.Background(), spec, run)
	require.ErrorIs(t, err, ErrUnescapableIdentifier)
	assert.False(t, called, "쿼리 생성 실패 시 질의를 보내지 않는다")
}

// TestInfluxSeriesEnum_V2_질의_오류_전파 는 질의 오류가 measurement 문맥과 함께
// 전파됨을 확인한다.
func TestInfluxSeriesEnum_V2_질의_오류_전파(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	run := func(_ context.Context, _ string) ([]map[string]any, error) {
		return nil, sentinel
	}

	_, err := enumerateSeriesWithFlux(context.Background(), f3EnumSpec(), run)
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "cpu")
}

// TestInfluxSeriesEnum_V2_클라이언트_버킷_기본값 은 빈 bucket 이 클라이언트
// 기본 버킷으로 채워짐을 확인한다. resolveSchemaBucket 규약과 같다.
func TestInfluxSeriesEnum_V2_클라이언트_버킷_기본값(t *testing.T) {
	t.Parallel()

	// 기본 버킷도 비어 있으면 네트워크를 타기 전에 오류로 끝난다.
	c := &influxV2Client{}
	_, err := c.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

// TestInfluxSeriesEnum_V2_큐리플럭스_왕복 은 influxV2Client.EnumerateSeries 가
// 실제 Flux 주석 CSV 응답을 시리즈로 접음을 확인한다.
//
// 이 테스트가 존재하는 이유는 SPEC-TSDB-003 §6 가정 3 을 실행으로 고정하기
// 위함이다 — "queryFlux(influxdb_v2.go:83-103) 가 결과 행의 **모든 컬럼**을
// 통과시킨다". §2.4 의 열거 방식 전체가 이 가정에 얹혀 있으며, 가정이 깨지면
// 태그가 통째로 사라진다. httptest 인프로세스 서버를 쓰므로 실제 InfluxDB 는
// 필요 없다.
func TestInfluxSeriesEnum_V2_쿼리플럭스_왕복(t *testing.T) {
	t.Parallel()

	// Flux 주석 CSV. result · table 구조 컬럼이 함께 오는 실제 형상이다.
	const csv = "#datatype,string,long,string,string,string,string\r\n" +
		"#group,false,false,true,true,true,true\r\n" +
		"#default,_result,,,,,\r\n" +
		",result,table,_measurement,_field,host,region\r\n" +
		",,0,cpu,usage,a,kr\r\n" +
		",,0,cpu,idle,a,kr\r\n" +
		",,1,cpu,usage,b,kr\r\n"

	// 요청 본문은 {"query": "...", "dialect": {...}} JSON 이므로 디코드해서 본다.
	var seenQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		seenQuery = req.Query
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(csv))
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	got, err := c.EnumerateSeries(context.Background(), SeriesEnumSpec{
		Measurement: "cpu",
		StartMs:     1700000000000,
		EndMs:       1702592000000,
		Tags:        map[string]string{"region": "kr"},
	})
	require.NoError(t, err)
	assert.True(t, got.FieldExact)
	assert.Equal(t, []EnumeratedSeries{
		{Tags: map[string]string{"host": "a", "region": "kr"}, Fields: []string{"idle", "usage"}},
		{Tags: map[string]string{"host": "b", "region": "kr"}, Fields: []string{"usage"}},
	}, got.Series)

	// 빈 bucket 이 클라이언트 기본 버킷으로 채워져 쿼리에 실렸는지 확인한다.
	assert.Contains(t, seenQuery, `from(bucket: "metrics")`)
	assert.Contains(t, seenQuery, "|> group()")
}

// ===== 에이전트 위임 =====

// mockEnumClient 는 InfluxClient 와 InfluxSeriesEnumerator 를 동시에 만족하는
// 모의 클라이언트다. 기존 mockInfluxClient 를 넓히지 않는 이유는 위험 R5 와
// 같다 — 넓히면 그것을 쓰던 모든 테스트가 함께 흔들린다.
type mockEnumClient struct {
	mockInfluxClient
	enumFunc func(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error)
}

func (m *mockEnumClient) EnumerateSeries(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
	if m.enumFunc != nil {
		return m.enumFunc(ctx, spec)
	}
	return SeriesEnumResult{}, nil
}

// TestInfluxDBAgent_EnumerateSeries_위임 은 에이전트가 클라이언트에 위임하고
// 빈 bucket 을 에이전트 기본값으로 채움을 확인한다(ListTagKeys 규약과 동일).
func TestInfluxDBAgent_EnumerateSeries_위임(t *testing.T) {
	t.Parallel()

	var seen SeriesEnumSpec
	mock := &mockEnumClient{
		enumFunc: func(_ context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
			seen = spec
			return SeriesEnumResult{
				Series:     []EnumeratedSeries{{Tags: map[string]string{"host": "a"}, Fields: []string{"usage"}}},
				FieldExact: true,
			}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	got, err := a.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	require.NoError(t, err)
	assert.True(t, got.FieldExact)
	require.Len(t, got.Series, 1)
	require.NotEmpty(t, a.defaultBucket())
	assert.Equal(t, a.defaultBucket(), seen.Bucket)
}

// TestInfluxDBAgent_EnumerateSeries_명시_버킷_보존 은 지정된 bucket 이 기본값으로
// 덮이지 않음을 확인한다.
func TestInfluxDBAgent_EnumerateSeries_명시_버킷_보존(t *testing.T) {
	t.Parallel()

	var seen SeriesEnumSpec
	mock := &mockEnumClient{
		enumFunc: func(_ context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
			seen = spec
			return SeriesEnumResult{}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)

	_, err := a.EnumerateSeries(context.Background(),
		SeriesEnumSpec{Bucket: "explicit", Measurement: "cpu", StartMs: 1, EndMs: 2})
	require.NoError(t, err)
	assert.Equal(t, "explicit", seen.Bucket)
}

// TestInfluxDBAgent_EnumerateSeries_클라이언트_미초기화 는 미연결 상태에서
// errClientNotInitialized 를 반환함을 확인한다.
func TestInfluxDBAgent_EnumerateSeries_클라이언트_미초기화(t *testing.T) {
	t.Parallel()

	a := newTestInfluxDBAgent(nil)
	a.client = nil
	_, err := a.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	assert.ErrorIs(t, err, errClientNotInitialized)
}

// TestInfluxDBAgent_EnumerateSeries_미지원_클라이언트 는 열거를 구현하지 않는
// 클라이언트가 센티넬 오류로 표면화됨을 확인한다.
//
// 열거를 구현하지 않는 클라이언트가 이 경로를 탄다. "디스커버리
// 실패"로 뭉뚱그리지 않고 별도 센티넬로 두는 이유는 §2.5 와 같다 — 원인을 알 수
// 없는 오류가 사용자에게 가장 비싼 실패다.
func TestInfluxDBAgent_EnumerateSeries_미지원_클라이언트(t *testing.T) {
	t.Parallel()

	a := newTestInfluxDBAgent(&mockInfluxClient{})
	_, err := a.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	assert.ErrorIs(t, err, ErrSeriesEnumerationNotSupported)
}

// TestInfluxDBAgent_EnumerateSeries_오류_전파 는 클라이언트 오류가 그대로
// 전파됨을 확인한다.
func TestInfluxDBAgent_EnumerateSeries_오류_전파(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("enum failed")
	a := newTestInfluxDBAgent(&mockEnumClient{
		enumFunc: func(_ context.Context, _ SeriesEnumSpec) (SeriesEnumResult, error) {
			return SeriesEnumResult{}, sentinel
		},
	})

	_, err := a.EnumerateSeries(context.Background(), SeriesEnumSpec{Measurement: "cpu", StartMs: 1, EndMs: 2})
	assert.ErrorIs(t, err, sentinel)
}

// --- M1 실측 회귀 (SPEC-TSDB-003 §HISTORY-0.4.0) -----------------------------

// 아래 CSV 의 헤더 행과 데이터 12행은 실제 InfluxDB 2.x 인스턴스에 대해
// BuildFluxSeriesEnumQuery 가 생성한 질의를 실행하고 **관측한 원문**이다
// (measurement=temperature, 실습실/사무실 LoRaWAN 센서). 주석 3줄
// (#datatype · #group · #default)은 관측 시 전달되지 않아 컬럼 타입에서
// 재구성한 것이며, 데이터 행은 손대지 않았다.
//
// 이 테스트가 잠그는 관측 사실 3가지:
//  1. result · table 구조 컬럼이 실재한다 — 제외하지 않으면 모든 시리즈에
//     존재하지 않는 태그가 붙는다
//  2. group() 이 서로 다른 태그 집합을 한 테이블(table=0)로 합치며, 없는
//     태그 컬럼은 **빈 문자열**로 채워진다 — null 도, 별도 result 섹션도 아니다
//  3. 태그 키에 점(.)이 들어간다 — device.dev_eui 등
const observedEnumCSV = "" +
	"#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,double,string,string,string,string,string,string,string,string,string\r\n" +
	"#group,false,false,false,false,false,false,false,false,false,false,false,false,false,false,false\r\n" +
	"#default,_result,,,,,,,,,,,,,,\r\n" +
	",result,table,_start,_stop,_time,_value,_field,_measurement,device.dev_eui,device.id,device.name,device.type,location,point,spot\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:45:44.389Z,24.3,value,temperature,24e124126d152590,9a383ec1-e8a6-4608-9005-76ff441f461f,EM500-CO2-152590,EM500-CO2,실습실,전방 좌측,전방 좌측\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:52.005Z,24.3,value,temperature,24e124126d152862,e2dc56fb-3fa5-4054-ad7f-ac24835f5183,EM500-CO2-152862,EM500-CO2,실습실,후방 오른쪽,후방 우측\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:45:49.042Z,24.1,value,temperature,24e124128c067999,3ba5a374-87cb-4102-93fa-72ddc0c769a9,AM107-067999,AM107,실습실,후면 오른쪽,후면 오른쪽\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:45:49.698Z,24.8,value,temperature,24e124128c140101,ea26f95c-6494-4dc3-9e32-6c3921d39805,AM107-140101,AM107,실습실,전방 우측,전방 우측\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:54:10.574Z,26.9,value,temperature,24e124136d151523,0d67adc6-e88c-420a-bd30-c657f9e5a78e,EM300-TH-151523,EM300-TH,실습실,복도,복도\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:36.644Z,23.8,value,temperature,24e124136d151547,c7816aa9-0111-48c9-886f-8834ade8637e,EM300-TH-151547,EM300-TH,실습실,중앙 우측,중앙 우측\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:11.042Z,23.8,value,temperature,24e124136d151606,8a60c0fd-922c-426f-b11e-6d1e9a6376a4,EM300-TH-151606,EM300-TH,실습실,중앙 좌측,중앙 좌측\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:01.6Z,28.3,value,temperature,24e124136d151836,9256c289-2056-489e-8860-ffb0b9ee8144,EM300-TH-151836,EM300-TH,사무실 밖,입구 오른쪽,입구 오른쪽\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:45:58.524Z,26.7,value,temperature,24e124725d081175,b4298e09-4348-4f3d-af77-fe89bec8df76,AM103-081175,AM103,사무실,업무 공간 안쪽,업무 공간 안쪽\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:47:56.919Z,26.2,value,temperature,24e124725d089152,557874e2-f644-4512-9237-0f4c53adc135,AM103-089152,AM103,회의실,,\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:22.298Z,26.8,value,temperature,24e124785c389010,bc790e8f-1de7-43b6-9cc9-965075bbe804,EM320-TH-389010,EM320-TH,사무실,사무공간 스위치 옆,사무공간 스위치 옆\r\n" +
	",_result,0,2026-07-24T00:09:59.944155955Z,2026-08-23T00:09:59.944155955Z,2026-08-21T23:46:34.857Z,24.4,value,temperature,24e124785c389818,c88c9b61-437f-4260-b366-b208b1387aa7,EM320-TH-389818,EM320-TH,실습실,,\r\n\r\n"

func TestEnumerateSeries_실측CSV_회귀(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(observedEnumCSV))
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	res, err := c.EnumerateSeries(context.Background(), SeriesEnumSpec{
		Bucket:      "metrics",
		Measurement: "temperature",
		StartMs:     1753315799944,
		EndMs:       1755907799944,
	})
	require.NoError(t, err)

	// 12개 디바이스 = 12개 시리즈. 접기가 행을 잃지도 늘리지도 않는다.
	require.Len(t, res.Series, 12)
	assert.True(t, res.FieldExact, "v2 는 관측치이므로 field 축이 정확하다")

	// 구조 컬럼과 밑줄 컬럼은 어떤 시리즈에도 태그로 새지 않는다.
	forbidden := []string{"result", "table", "_start", "_stop", "_time", "_value", "_field", "_measurement"}
	for _, s := range res.Series {
		for _, bad := range forbidden {
			assert.NotContains(t, s.Tags, bad, "구조/내부 컬럼이 태그로 샜다")
		}
		assert.Equal(t, []string{"value"}, s.Fields)
	}

	// 태그 값이 빈 문자열인 컬럼은 태그가 아니다 — InfluxDB 는 빈 태그 값을
	// 저장하지 않으므로, group() 의 빈 채움은 "그 시리즈에 그 태그가 없다"는 뜻이다.
	byName := make(map[string]EnumeratedSeries, len(res.Series))
	for _, s := range res.Series {
		byName[s.Tags["device.name"]] = s
	}

	full, ok := byName["EM500-CO2-152590"]
	require.True(t, ok)
	assert.Len(t, full.Tags, 7, "point · spot 을 포함한 7개 태그")
	assert.Equal(t, "전방 좌측", full.Tags["point"])
	assert.Equal(t, "24e124126d152590", full.Tags["device.dev_eui"], "점이 든 태그 키가 보존된다")

	sparse, ok := byName["AM103-089152"]
	require.True(t, ok)
	assert.Len(t, sparse.Tags, 5, "point · spot 이 빈 값이므로 태그에서 빠진다")
	assert.NotContains(t, sparse.Tags, "point")
	assert.NotContains(t, sparse.Tags, "spot")
	assert.Equal(t, "회의실", sparse.Tags["location"])
}
