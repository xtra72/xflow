// 가져올 데이터 범위 — 기간(상대·절대) 또는 갯수.

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_CHART_POINTS,
  DEFAULT_RANGE_COUNT,
  MAX_RANGE_COUNT,
  chartXRangePoints,
  formatWindowMs,
  limitToRecent,
  readChartXRange,
  readSeriesRange,
  resolveChartXWindow,
  resolveSeriesWindow,
  seriesRangeKey,
  type SeriesRange,
} from './seriesRange';

const NOW = 1_700_000_000_000;
const INTERVAL = 60_000;

describe('readSeriesRange — 구 config 하위 호환', () => {
  it('range 가 없으면 time_window_ms 를 상대 기간으로 읽는다', () => {
    expect(readSeriesRange(undefined, 3_600_000)).toEqual({
      mode: 'relative',
      window_ms: 3_600_000,
    });
  });

  it('range 가 있으면 그대로 쓴다', () => {
    const r: SeriesRange = { mode: 'count', count: 50 };
    expect(readSeriesRange(r, 3_600_000)).toBe(r);
  });
});

describe('resolveSeriesWindow — 상대 기간', () => {
  it('now 에서 창 길이만큼 뒤로 잡는다', () => {
    expect(resolveSeriesWindow({ mode: 'relative', window_ms: 3_600_000 }, INTERVAL, NOW))
      .toEqual({ startMs: NOW - 3_600_000, endMs: NOW });
  });

  it('창 길이가 없거나 0 이하면 해석 불가', () => {
    expect(resolveSeriesWindow({ mode: 'relative' }, INTERVAL, NOW)).toBeUndefined();
    expect(resolveSeriesWindow({ mode: 'relative', window_ms: 0 }, INTERVAL, NOW)).toBeUndefined();
  });
});

describe('resolveSeriesWindow — 절대 구간', () => {
  it('지정한 시작/끝을 그대로 쓴다 (now 와 무관)', () => {
    expect(resolveSeriesWindow({ mode: 'absolute', start_ms: 100, end_ms: 500 }, INTERVAL, NOW))
      .toEqual({ startMs: 100, endMs: 500 });
  });

  it('시작 == 끝(한 점)도 허용한다', () => {
    expect(resolveSeriesWindow({ mode: 'absolute', start_ms: 100, end_ms: 100 }, INTERVAL, NOW))
      .toEqual({ startMs: 100, endMs: 100 });
  });

  it('한쪽이 비었거나 앞뒤가 뒤집히면 해석 불가 (임의 기본값으로 메우지 않는다)', () => {
    expect(resolveSeriesWindow({ mode: 'absolute', start_ms: 100 }, INTERVAL, NOW)).toBeUndefined();
    expect(resolveSeriesWindow({ mode: 'absolute', end_ms: 500 }, INTERVAL, NOW)).toBeUndefined();
    expect(resolveSeriesWindow({ mode: 'absolute', start_ms: 500, end_ms: 100 }, INTERVAL, NOW))
      .toBeUndefined();
  });
});

describe('resolveSeriesWindow — 갯수', () => {
  it('인터벌 × N 으로 구간을 역산하고 자를 개수를 함께 준다', () => {
    expect(resolveSeriesWindow({ mode: 'count', count: 10 }, INTERVAL, NOW))
      .toEqual({ startMs: NOW - 10 * INTERVAL, endMs: NOW, limitBuckets: 10 });
  });

  it('상한을 넘는 갯수는 클램프된다', () => {
    const out = resolveSeriesWindow({ mode: 'count', count: 999_999 }, INTERVAL, NOW)!;
    expect(out.limitBuckets).toBe(MAX_RANGE_COUNT);
    expect(out.startMs).toBe(NOW - MAX_RANGE_COUNT * INTERVAL);
  });

  it('갯수가 0 이하이거나 인터벌이 없으면 해석 불가', () => {
    expect(resolveSeriesWindow({ mode: 'count', count: 0 }, INTERVAL, NOW)).toBeUndefined();
    expect(resolveSeriesWindow({ mode: 'count', count: 10 }, 0, NOW)).toBeUndefined();
  });
});

describe('seriesRangeKey — 폴링 재시작 축', () => {
  it('상대·갯수는 해석 시각을 담지 않는다 (폴링마다 키가 바뀌면 안 된다)', () => {
    const rel: SeriesRange = { mode: 'relative', window_ms: 3_600_000 };
    expect(seriesRangeKey(rel)).toBe(seriesRangeKey(rel));
    expect(seriesRangeKey(rel)).not.toContain(String(NOW));
    expect(seriesRangeKey({ mode: 'count', count: 10 })).toBe('cnt:10');
  });

  it('설정이 바뀌면 키도 바뀐다', () => {
    expect(seriesRangeKey({ mode: 'relative', window_ms: 1000 }))
      .not.toBe(seriesRangeKey({ mode: 'relative', window_ms: 2000 }));
    expect(seriesRangeKey({ mode: 'absolute', start_ms: 1, end_ms: 2 }))
      .not.toBe(seriesRangeKey({ mode: 'absolute', start_ms: 1, end_ms: 3 }));
  });

  it('방식이 다르면 키도 다르다', () => {
    const keys = new Set([
      seriesRangeKey({ mode: 'relative', window_ms: 10 }),
      seriesRangeKey({ mode: 'absolute', start_ms: 10, end_ms: 10 }),
      seriesRangeKey({ mode: 'count', count: 10 }),
    ]);
    expect(keys.size).toBe(3);
  });
});

describe('limitToRecent — 갯수 방식의 뒤에서 자르기', () => {
  it('경계 정렬로 딸려 온 앞쪽 버킷을 버린다', () => {
    expect(limitToRecent([1, 2, 3, 4, 5], 3)).toEqual([3, 4, 5]);
  });

  it('개수가 모자라면 그대로 둔다', () => {
    expect(limitToRecent([1, 2], 5)).toEqual([1, 2]);
  });

  it('limitBuckets 가 없으면(상대·절대) 원본 복사본을 돌려준다', () => {
    const rows = [1, 2, 3];
    const out = limitToRecent(rows, undefined);
    expect(out).toEqual(rows);
    expect(out).not.toBe(rows);
  });
});

describe('formatWindowMs / 기본값', () => {
  it('가장 큰 단위로 표기한다', () => {
    expect(formatWindowMs(300_000)).toBe('5m');
    expect(formatWindowMs(3_600_000)).toBe('1h');
    expect(formatWindowMs(7 * 86_400_000)).toBe('7d');
  });

  it('갯수 기본값은 상한 이하다', () => {
    expect(DEFAULT_RANGE_COUNT).toBeGreaterThan(0);
    expect(DEFAULT_RANGE_COUNT).toBeLessThanOrEqual(MAX_RANGE_COUNT);
  });
});

describe('라인 차트 X축 범위 — 구 time_window_mode 와의 대응', () => {
  it('x_range 가 있으면 그대로 쓴다', () => {
    const r = readChartXRange({
      x_range: { mode: 'relative', window_ms: 5000 },
      // 구 필드가 남아 있어도 새 값이 이긴다.
      time_window_mode: 'points',
      max_points: 7,
    });
    expect(r).toEqual({ mode: 'relative', window_ms: 5000 });
  });

  it('points → count (max_points 승계)', () => {
    expect(readChartXRange({ time_window_mode: 'points', max_points: 250 })).toEqual({
      mode: 'count',
      count: 250,
    });
  });

  it('time_window_mode 가 없으면 구 기본값 points 로 본다', () => {
    expect(readChartXRange({})).toEqual({ mode: 'count', count: DEFAULT_CHART_POINTS });
  });

  it('recent → relative (초 → ms)', () => {
    expect(readChartXRange({ time_window_mode: 'recent', recent_window_sec: 90 })).toEqual({
      mode: 'relative',
      window_ms: 90_000,
    });
  });

  it('recent 인데 초가 없으면 구 렌더러 기본값(10분)을 쓴다', () => {
    expect(readChartXRange({ time_window_mode: 'recent' })).toEqual({
      mode: 'relative',
      window_ms: 600_000,
    });
  });

  it('fixed → absolute', () => {
    expect(
      readChartXRange({ time_window_mode: 'fixed', fixed_start_ms: 10, fixed_end_ms: 20 }),
    ).toEqual({ mode: 'absolute', start_ms: 10, end_ms: 20 });
  });
});

describe('resolveChartXWindow', () => {
  it('상대 기간은 [now-w, now]', () => {
    expect(resolveChartXWindow({ mode: 'relative', window_ms: 1000 }, 5000)).toEqual({
      startMs: 4000,
      endMs: 5000,
    });
  });

  it('기간이 0 이하면 해석하지 않는다', () => {
    expect(resolveChartXWindow({ mode: 'relative', window_ms: 0 }, 5000)).toBeUndefined();
  });

  it('갯수 방식은 시간으로 자르지 않는다', () => {
    expect(resolveChartXWindow({ mode: 'count', count: 10 }, 5000)).toBeUndefined();
  });

  it('절대 구간의 빈 쪽은 열어 둔다 — 구 ±Infinity 규약', () => {
    expect(resolveChartXWindow({ mode: 'absolute', start_ms: 10 }, 0)).toEqual({
      startMs: 10,
      endMs: Number.POSITIVE_INFINITY,
    });
    expect(resolveChartXWindow({ mode: 'absolute', end_ms: 20 }, 0)).toEqual({
      startMs: Number.NEGATIVE_INFINITY,
      endMs: 20,
    });
  });
});

describe('chartXRangePoints — 버퍼 크기', () => {
  it('갯수 방식은 그 값이 버퍼 크기다', () => {
    expect(chartXRangePoints({ mode: 'count', count: 300 })).toBe(300);
  });

  it('갯수는 상한을 넘지 않는다', () => {
    expect(chartXRangePoints({ mode: 'count', count: MAX_RANGE_COUNT + 1 })).toBe(MAX_RANGE_COUNT);
  });

  it('상대 기간은 1Hz 가정으로 기간의 2배', () => {
    expect(chartXRangePoints({ mode: 'relative', window_ms: 600_000 })).toBe(1200);
  });

  it('상대 기간이 짧아도 기본값 아래로 내려가지 않는다', () => {
    expect(chartXRangePoints({ mode: 'relative', window_ms: 10_000 })).toBe(DEFAULT_CHART_POINTS);
  });

  it('상대 기간이 길어도 5000 을 넘지 않는다', () => {
    expect(chartXRangePoints({ mode: 'relative', window_ms: 7 * 24 * 3600 * 1000 })).toBe(5000);
  });

  it('절대 구간은 기본 버퍼를 쓴다', () => {
    expect(chartXRangePoints({ mode: 'absolute', start_ms: 1, end_ms: 2 })).toBe(
      DEFAULT_CHART_POINTS,
    );
  });
});
