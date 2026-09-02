package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 집계 경로가 DataPoint 를 복사하지 않는지 — 이 파일의 핵심 회귀 방어.
//
// 이전에는 QueryRange 가 구간 전체를 복사하며 포인트마다 Fields 맵을 새로
// 할당했고, 그 비용을 다운샘플링 *전에* 그리고 읽기 락을 쥔 채로 치렀다.
// 아래 테스트는 할당 수가 구간 크기에 비례하지 않음을 직접 잰다.

func newFilledSeries(t *testing.T, n int) (*Series, time.Time) {
	t.Helper()
	ResetMemoryUsage()
	s := NewSeries("temp,room=living")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"temperature": float64(i), "humidity": float64(i) * 2},
		}))
	}
	return s, base
}

func TestQueryRangeScalar_할당이_구간크기에_비례하지_않는다(t *testing.T) {
	const n = 4000
	s, base := newFilledSeries(t, n)
	start, end := base, base.Add(time.Duration(n)*time.Second)

	scalarAllocs := testing.AllocsPerRun(20, func() {
		pts, scanned, _ := s.QueryRangeScalar(start, end, "temperature")
		if scanned != n || len(pts) != n {
			t.Fatalf("scanned=%d len=%d, want %d", scanned, len(pts), n)
		}
	})

	copyAllocs := testing.AllocsPerRun(20, func() {
		if got := len(s.QueryRange(start, end)); got != n {
			t.Fatalf("len=%d, want %d", got, n)
		}
	})

	// 복사 경로는 포인트마다 맵을 하나씩 만든다 → 할당이 n 을 넘는다.
	assert.Greater(t, copyAllocs, float64(n),
		"복사 경로가 포인트당 할당을 하지 않는다면 이 테스트의 전제가 깨진 것이다")

	// 스칼라 경로는 슬라이스 한 번(+증가분)이면 충분하다.
	assert.Less(t, scalarAllocs, float64(10),
		"스칼라 경로에서 포인트당 할당이 되살아났다 (got=%v)", scalarAllocs)
}

func TestQueryRangeScalar_반환값_규약(t *testing.T) {
	s, base := newFilledSeries(t, 10)

	t.Run("scanned 는 값 추출 실패와 무관하게 원본 포인트 수다", func(t *testing.T) {
		// 존재하지 않는 필드 → 추출 0건, 그러나 스캔은 10건.
		pts, scanned, firstTS := s.QueryRangeScalar(base, base.Add(9*time.Second), "nonexistent")
		assert.Empty(t, pts)
		assert.Equal(t, 10, scanned)
		assert.Equal(t, base, firstTS, "firstTS 는 원본 첫 포인트 시각이다")
	})

	t.Run("빈 구간은 제로값을 돌려준다", func(t *testing.T) {
		far := base.Add(time.Hour)
		pts, scanned, firstTS := s.QueryRangeScalar(far, far.Add(time.Minute), "temperature")
		assert.Nil(t, pts)
		assert.Zero(t, scanned)
		assert.True(t, firstTS.IsZero())
	})

	t.Run("구간 경계는 양끝 포함이다", func(t *testing.T) {
		pts, scanned, _ := s.QueryRangeScalar(base, base.Add(2*time.Second), "temperature")
		assert.Equal(t, 3, scanned)
		require.Len(t, pts, 3)
		assert.Equal(t, 0.0, pts[0].v)
		assert.Equal(t, 2.0, pts[2].v)
	})
}

// appendScalar 는 필드 선택 규칙의 단일 출처다 — 규칙을 직접 고정해 둔다.
func TestAppendScalar_필드선택규칙(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("필드 지정 시 그 필드만 읽는다", func(t *testing.T) {
		dp := DataPoint{Timestamp: ts, Fields: map[string]any{"a": 1.0, "b": 2.0}}
		got := appendScalar(nil, dp, "b")
		require.Len(t, got, 1)
		assert.Equal(t, 2.0, got[0].v)
	})

	t.Run("지정 필드가 숫자가 아니면 건너뛴다", func(t *testing.T) {
		dp := DataPoint{Timestamp: ts, Fields: map[string]any{"a": "text"}}
		assert.Empty(t, appendScalar(nil, dp, "a"))
	})

	t.Run("필드 미지정이면 첫 숫자 필드를 쓴다", func(t *testing.T) {
		dp := DataPoint{Timestamp: ts, Fields: map[string]any{"only": 7.0}}
		got := appendScalar(nil, dp, "")
		require.Len(t, got, 1)
		assert.Equal(t, 7.0, got[0].v)
	})

	t.Run("필드 미지정 + 숫자 필드 없음이면 건너뛴다", func(t *testing.T) {
		dp := DataPoint{Timestamp: ts, Fields: map[string]any{"s": "x", "b": true}}
		assert.Empty(t, appendScalar(nil, dp, ""))
	})
}

// 스칼라 구현으로 갈아탄 뒤에도 Execute 의 결과가 그대로인지(동작 보존).
func TestExecute_스칼라전환_동작보존(t *testing.T) {
	ResetMemoryUsage()
	db := New(DefaultConfig())
	defer func() { _ = db.Close() }()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 0~59초, 값 = 초. 앞 2개는 대상 필드가 없다(선두 결측).
	for i := range 60 {
		fields := map[string]any{"temperature": float64(i)}
		if i < 2 {
			fields = map[string]any{"other": float64(i)}
		}
		require.NoError(t, db.WriteBatch([]WriteRequest{{
			Measurement: "temp",
			Tags:        map[string]string{"room": "living"},
			Fields:      fields,
			Timestamp:   base.Add(time.Duration(i) * time.Second),
		}}))
	}
	end := base.Add(59 * time.Second)

	t.Run("전체 범위 집계의 시각은 원본 첫 포인트 시각이다", func(t *testing.T) {
		res, err := db.Execute(Query{
			Measurement: "temp",
			Field:       "temperature",
			Aggregation: AggAvg,
			Start:       base,
			End:         end,
		})
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Len(t, res[0].Points, 1)

		// 값 있는 포인트는 2~59 → 평균 30.5.
		assert.InDelta(t, 30.5, res[0].Points[0].Fields["temperature"], 1e-9)
		// 시각은 값이 없는 선두 포인트(0초)를 따른다.
		assert.Equal(t, base, res[0].Points[0].Timestamp)
		assert.Equal(t, 60, res[0].Stats.ScannedPoints)
	})

	t.Run("다운샘플링은 빈 버킷을 생략한다", func(t *testing.T) {
		res, err := db.Execute(Query{
			Measurement:    "temp",
			Field:          "temperature",
			Aggregation:    AggMax,
			BucketInterval: 30 * time.Second,
			Start:          base,
			End:            end,
		})
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Len(t, res[0].Points, 2)
		assert.Equal(t, 29.0, res[0].Points[0].Fields["temperature"])
		assert.Equal(t, 59.0, res[0].Points[1].Fields["temperature"])
	})

	t.Run("fill 은 균일한 시간축을 만든다", func(t *testing.T) {
		res, err := db.Execute(Query{
			Measurement:    "temp",
			Field:          "temperature",
			Aggregation:    AggLast,
			BucketInterval: 20 * time.Second,
			Fill:           FillPrevious,
			Start:          base,
			End:            base.Add(60 * time.Second),
		})
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Len(t, res[0].Points, 3)
	})

	t.Run("원본 반환 경로는 그대로다", func(t *testing.T) {
		res, err := db.Execute(Query{
			Measurement: "temp",
			Start:       base,
			End:         base.Add(2 * time.Second),
		})
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Len(t, res[0].Points, 3)
	})
}

// 복사 경로와 스칼라 경로의 할당량 대비. 회귀 시 수치로 드러난다.
func BenchmarkQueryRange_복사(b *testing.B) {
	s, base, start, end := benchSeries(b)
	_ = base
	b.ReportAllocs()
	for b.Loop() {
		_ = s.QueryRange(start, end)
	}
}

func BenchmarkQueryRangeScalar_스칼라(b *testing.B) {
	s, base, start, end := benchSeries(b)
	_ = base
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = s.QueryRangeScalar(start, end, "temperature")
	}
}

func benchSeries(b *testing.B) (*Series, time.Time, time.Time, time.Time) {
	b.Helper()
	const n = 10_000
	ResetMemoryUsage()
	s := NewSeries("temp,room=living")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		if err := s.Write(DataPoint{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"temperature": float64(i), "humidity": float64(i) * 2},
		}); err != nil {
			b.Fatal(err)
		}
	}
	return s, base, base, base.Add(time.Duration(n) * time.Second)
}
