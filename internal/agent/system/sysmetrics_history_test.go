package system

// 표본 이력 버퍼 테스트.
//
// 잠그는 것:
//   - 누적 카운터는 **저장 시점에** 초당 증가량으로 환산된다(패널이 환산하지 않는다)
//   - 환산할 수 없는 경우(첫 표본 · 되감김 · 경과 0)에는 값을 싣지 않는다
//   - 같은 카운터의 **누적 원값**은 `mode=total` 시리즈로 함께 실린다
//   - 인스턴스 축이 있는 그룹은 대상별 값 + 종합을 함께 낸다
//   - 보관은 기간과 표본 수 **두 축**으로 죈다

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleAt 은 표본 하나를 만든다. 누적 카운터는 인자로 받아 증가량을 검증할 수 있게 한다.
func sampleAt(ms int64, cpu float64, recv map[string]uint64) SysMetricsSample {
	s := SysMetricsSample{
		Timestamp: ms,
		CPU:       &CPUSample{UsagePercent: cpu, Cores: 8},
	}
	for name, v := range recv {
		s.Network = append(s.Network, NetworkSample{Interface: name, BytesRecv: v})
	}
	return s
}

/** 시리즈 하나의 값을 꺼낸다. 없으면 두 번째 반환이 false. */
func valueOf(res SysMetricsHistoryResult, rowIdx int, key SysMetricsSeriesKey) (float64, bool) {
	for i, s := range res.Series {
		if s == key {
			v := res.Points[rowIdx].Values[i]
			if v == nil {
				return 0, false
			}
			return *v, true
		}
	}
	return 0, false
}

func TestHistory_상태값은_첫_표본부터_실린다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 42, nil), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 1)

	v, ok := valueOf(res, 0, SysMetricsSeriesKey{Measurement: "usage_percent", Category: "cpu"})
	assert.True(t, ok, "cpu 는 상태값이라 기준점 없이도 실린다")
	assert.Equal(t, 42.0, v)
}

func TestHistory_누적_카운터는_첫_표본에_실리지_않는다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000}), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 1)

	// 기준점이 없으면 증가량을 만들 수 없다. 0 으로 대신하면 트래픽이 끊긴 것처럼 보인다.
	_, ok := valueOf(res, 0, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
	})
	assert.False(t, ok)
}

func TestHistory_누적_카운터는_초당_증가량으로_환산된다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000}), time.Hour)
	h.append(sampleAt(3_000, 10, map[string]uint64{"en0": 3_000}), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 2)

	// 2000 바이트 / 2 초 = 1000 B/s. 패널은 이 값을 그대로 그린다.
	v, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
	})
	require.True(t, ok)
	assert.Equal(t, 1_000.0, v)
}

func TestHistory_카운터가_되감기면_값을_싣지_않는다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 5_000}), time.Hour)
	h.append(sampleAt(3_000, 10, map[string]uint64{"en0": 10}), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 2)

	// 재부팅·인터페이스 재설정. 그대로 두면 거대한 음수 스파이크가 된다.
	_, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
	})
	assert.False(t, ok)
}

func TestHistory_경과가_0이면_값을_싣지_않는다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000}), time.Hour)
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 2_000}), time.Hour)

	res := h.query(0, 0, time.Hour)
	_, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
	})
	assert.False(t, ok, "0 으로 나누면 Infinity 가 된다")
}

func TestHistory_인스턴스_그룹은_대상별과_종합을_함께_낸다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000, "en1": 2_000}), time.Hour)
	h.append(sampleAt(2_000, 10, map[string]uint64{"en0": 2_000, "en1": 4_000}), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 2)

	en0, ok0 := valueOf(res, 1, SysMetricsSeriesKey{Measurement: "bytes_recv", Category: "network", Target: "en0"})
	en1, ok1 := valueOf(res, 1, SysMetricsSeriesKey{Measurement: "bytes_recv", Category: "network", Target: "en1"})
	total, okT := valueOf(res, 1, SysMetricsSeriesKey{Measurement: "bytes_recv", Category: "network"})

	require.True(t, ok0 && ok1 && okT)
	assert.Equal(t, 1_000.0, en0)
	assert.Equal(t, 2_000.0, en1)
	// 종합은 개별과 다른 시리즈다 — 대상을 고르지 않은 패널이 보는 값이다.
	assert.Equal(t, 3_000.0, total, "종합은 전체 합의 증가량이다")
}

func TestHistory_스토리지_종합_사용률은_합계_용량_기준이다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(SysMetricsSample{
		Timestamp: 1_000,
		Storage: []StorageSample{
			{Mountpoint: "/", UsedBytes: 900, TotalBytes: 1_000, UsagePercent: 90},
			{Mountpoint: "/data", UsedBytes: 100, TotalBytes: 1_000, UsagePercent: 10},
		},
	}, time.Hour)

	res := h.query(0, 0, time.Hour)
	v, ok := valueOf(res, 0, SysMetricsSeriesKey{Measurement: "usage_percent", Category: "storage"})
	require.True(t, ok)
	// 90% + 10% = 100% 가 아니라 (900+100)/(1000+1000) = 50% 여야 한다.
	assert.Equal(t, 50.0, v)
}

func TestHistory_보관_기간을_벗어난_표본은_버린다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 1, nil), time.Minute)
	h.append(sampleAt(2_000, 2, nil), time.Minute)
	// 보관 1분 — 첫 두 표본은 창 밖으로 밀린다.
	h.append(sampleAt(100_000, 3, nil), time.Minute)

	res := h.query(0, 0, time.Minute)
	require.Len(t, res.Points, 1)
	assert.Equal(t, int64(100_000), res.Points[0].TimeMs)
}

func TestHistory_표본_수_상한에_걸리면_잘라내고_알린다(t *testing.T) {
	h := newSysMetricsHistory()
	// 기간은 넉넉하지만 개수가 상한을 넘는다 — 주기를 낮췄을 때의 상황이다.
	for i := 0; i <= maxSysMetricsHistorySamples; i++ {
		h.append(sampleAt(int64(i)*10, float64(i%100), nil), 24*time.Hour)
	}

	res := h.query(0, 0, 24*time.Hour)
	assert.Len(t, res.Points, maxSysMetricsHistorySamples)
	assert.True(t, res.Truncated, "잘라냈다는 사실이 드러나야 한다")
	// 실제 보관 기간은 설정보다 짧다 — 설정값을 그대로 알리면 왜 짧은지 알 수 없다.
	assert.Less(t, res.RetentionSeconds, (24 * time.Hour).Seconds())
}

func TestHistory_질의_구간으로_거른다(t *testing.T) {
	h := newSysMetricsHistory()
	for i := 1; i <= 5; i++ {
		h.append(sampleAt(int64(i)*1_000, float64(i), nil), time.Hour)
	}

	res := h.query(2_000, 4_000, time.Hour)
	require.Len(t, res.Points, 2, "끝은 exclusive 다")
	assert.Equal(t, int64(2_000), res.Points[0].TimeMs)
	assert.Equal(t, int64(3_000), res.Points[1].TimeMs)
}

func TestHistory_시리즈_순서는_결정적이다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en1": 1, "en0": 2}), time.Hour)
	h.append(sampleAt(2_000, 10, map[string]uint64{"en1": 5, "en0": 9}), time.Hour)

	first := h.query(0, 0, time.Hour).Series
	second := h.query(0, 0, time.Hour).Series
	// 순서가 호출마다 바뀌면 열 위치가 흔들려 소비자가 컬럼을 잘못 귀속한다.
	assert.Equal(t, first, second)

	// 분류 → measurement → 대상 → 표현 순.
	for i := 1; i < len(first); i++ {
		a, b := first[i-1], first[i]
		if a.Category != b.Category || a.Measurement != b.Measurement {
			continue
		}
		if a.Target == b.Target {
			// 같은 대상의 증가량·누적 원값은 표현으로 갈린다.
			assert.Less(t, a.Mode, b.Mode)
			continue
		}
		assert.Less(t, a.Target, b.Target)
	}
}

func TestHistory_누적_원값은_첫_표본부터_실린다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000}), time.Hour)

	res := h.query(0, 0, time.Hour)
	require.Len(t, res.Points, 1)

	// 증가량과 달리 기준점이 필요 없다 — 원값이 곧 그릴 값이다.
	v, ok := valueOf(res, 0, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
		Mode: sysMetricsModeTotal,
	})
	require.True(t, ok)
	assert.Equal(t, 1_000.0, v)
}

func TestHistory_누적_원값과_증가량은_다른_시리즈다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 1_000}), time.Hour)
	h.append(sampleAt(3_000, 10, map[string]uint64{"en0": 3_000}), time.Hour)

	res := h.query(0, 0, time.Hour)

	rate, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
	})
	require.True(t, ok)
	assert.Equal(t, 1_000.0, rate, "2000 바이트 / 2 초")

	total, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
		Mode: sysMetricsModeTotal,
	})
	require.True(t, ok)
	assert.Equal(t, 3_000.0, total, "누적 원값은 환산하지 않는다")
}

func TestHistory_되감겨도_누적_원값은_실린다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 10, map[string]uint64{"en0": 5_000}), time.Hour)
	h.append(sampleAt(3_000, 10, map[string]uint64{"en0": 10}), time.Hour)

	res := h.query(0, 0, time.Hour)

	// 증가량은 스파이크를 막으려 비우지만, 원값은 재부팅 후의 사실 그대로다.
	total, ok := valueOf(res, 1, SysMetricsSeriesKey{
		Measurement: "bytes_recv", Category: "network", Target: "en0",
		Mode: sysMetricsModeTotal,
	})
	require.True(t, ok)
	assert.Equal(t, 10.0, total)
}

func TestHistory_상태값에는_표현_축이_없다(t *testing.T) {
	h := newSysMetricsHistory()
	h.append(sampleAt(1_000, 42, nil), time.Hour)

	// cpu·memory·storage 는 그 시점의 상태값이라 환산할 것이 없다. `mode` 를 실으면
	// 소비자가 고를 것이 없는 축을 하나 더 배워야 한다.
	for _, s := range h.query(0, 0, time.Hour).Series {
		if s.Category == "cpu" {
			assert.Empty(t, s.Mode)
		}
	}
}

// --- 배선 계약 (프론트엔드 파서와 맞물리는 지점) ---

// TestHistory_Process_응답_JSON_키는_파서와_같다 는 `get_history` 응답의 JSON 키를
// 고정한다.
//
// 프론트엔드 `sysmetricsHistory.ts` 의 `parseHistoryResult` 가 이 키들을 그대로
// 읽는다. 한쪽 이름만 바뀌면 파서는 **던지지 않고 빈 결과**로 떨어지므로(의도된
// 관대함), 화면에는 오류 없이 빈 차트만 남는다 — 그래서 이름을 테스트로 잠근다.
func TestHistory_Process_응답_JSON_키는_파서와_같다(t *testing.T) {
	a := &SysMetricsAgent{history: newSysMetricsHistory()}
	a.cfg.History = time.Hour
	a.history.append(sampleAt(1_000, 42, map[string]uint64{"en0": 1_000}), time.Hour)
	a.history.append(sampleAt(2_000, 43, map[string]uint64{"en0": 2_000}), time.Hour)

	raw, err := a.Process([]byte(`{"command":"get_history","params":{"start_ms":0,"end_ms":0}}`))
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	for _, key := range []string{"series", "points", "retention_seconds", "truncated"} {
		assert.Contains(t, body, key)
	}

	series := body["series"].([]any)
	require.NotEmpty(t, series)
	first := series[0].(map[string]any)
	assert.Contains(t, first, "measurement")
	assert.Contains(t, first, "category")

	points := body["points"].([]any)
	require.NotEmpty(t, points)
	point := points[0].(map[string]any)
	assert.Contains(t, point, "time_ms")
	assert.Contains(t, point, "values")

	// 대상이 빈 종합 시리즈는 target 을 싣지 않는다(omitempty) — 프론트의
	// `historySeriesTags` 가 "없으면 종합" 으로 읽는 축이다.
	for _, s := range series {
		m := s.(map[string]any)
		if m["category"] == "cpu" {
			assert.NotContains(t, m, "target")
		}
	}
}

// TestHistory_Process_모르는_커맨드는_오류다 는 화이트리스트 밖 이름이 조용히
// 성공하지 않음을 잠근다.
func TestHistory_Process_모르는_커맨드는_오류다(t *testing.T) {
	a := &SysMetricsAgent{history: newSysMetricsHistory()}
	_, err := a.Process([]byte(`{"command":"set_something"}`))
	assert.Error(t, err)
}
