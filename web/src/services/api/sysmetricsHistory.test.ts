// sysmetrics 이력 조회 테스트.
//
// 잠그는 것:
//
//   - 응답 형상이 어긋나도 **던지지 않고** 빈 결과가 된다(패널이 오류 오버레이 대신
//     빈 차트를 그린다)
//   - 이력 시리즈의 태그·식별자가 Store 어휘와 같다 — 요청과 짝짓는 축이다
//   - 버킷 경계·집계가 Store 와 같은 함수로 계산된다

import { describe, expect, it, vi, beforeEach } from 'vitest';

const queryAgent = vi.hoisted(() => vi.fn());
vi.mock('./agentService', () => ({ queryAgent }));

import {
  historySeriesId,
  historySeriesTags,
  parseHistoryResult,
  querySysmetricsMatrix,
  type SysmetricsHistoryResult,
} from './sysmetricsHistory';
import type { SeriesMatrixQuery } from './seriesDataSource';

describe('parseHistoryResult — 형상이 어긋나면 빈 결과다', () => {
  const empty: SysmetricsHistoryResult = {
    series: [],
    points: [],
    retention_seconds: 0,
    truncated: false,
  };

  it.each([
    ['null', null],
    ['숫자', 42],
    ['배열 아닌 series', { series: {}, points: [] }],
    ['points 누락', { series: [] }],
  ])('%s 은 빈 결과다', (_label, raw) => {
    expect(parseHistoryResult(raw)).toEqual(empty);
  });

  it('정상 응답을 그대로 옮긴다', () => {
    const out = parseHistoryResult({
      series: [{ measurement: 'bytes_recv', category: 'network', target: 'en0' }],
      points: [{ time_ms: 1_000, values: [12] }],
      retention_seconds: 3_600,
      truncated: true,
    });
    expect(out.series).toHaveLength(1);
    expect(out.points).toEqual([{ time_ms: 1_000, values: [12] }]);
    expect(out.retention_seconds).toBe(3_600);
    expect(out.truncated).toBe(true);
  });

  it('형상이 깨진 원소만 걸러낸다', () => {
    // 한 원소가 이상하다고 전체를 버리면 나머지 시리즈까지 화면에서 사라진다.
    const out = parseHistoryResult({
      series: [{ measurement: 'usage_percent', category: 'cpu' }, { nope: 1 }],
      points: [{ time_ms: 1, values: [1] }, { time_ms: 'x', values: [] }],
    });
    expect(out.series).toHaveLength(1);
    expect(out.points).toHaveLength(1);
  });
});

describe('historySeriesTags / historySeriesId — Store 어휘', () => {
  it('대상이 없으면 분류 태그만 실린다', () => {
    expect(historySeriesTags({ measurement: 'usage_percent', category: 'cpu' })).toEqual({
      category: 'cpu',
    });
  });

  it('누적 원값 시리즈는 mode 태그를 싣는다', () => {
    // 요청 쪽(`sysmetricSeriesTags`)과 같은 규칙이라야 두 식별자가 맞물린다.
    expect(
      historySeriesTags({ measurement: 'bytes_recv', category: 'network', target: 'en0', mode: 'total' }),
    ).toEqual({ category: 'network', interface: 'en0', mode: 'total' });
  });

  it('기본 표현(증가량)에는 mode 태그가 없다', () => {
    // 실으면 이미 저장된 패널이 시리즈를 찾지 못해 빈 차트가 된다.
    expect(
      historySeriesTags({ measurement: 'bytes_recv', category: 'network', target: 'en0' }),
    ).toEqual({ category: 'network', interface: 'en0' });
  });

  it.each([
    ['network', 'en0', 'interface'],
    ['disk_io', 'disk0', 'device'],
    ['storage', '/data', 'mountpoint'],
  ])('%s 의 대상은 %s → %s 태그다', (category, target, tagKey) => {
    expect(historySeriesTags({ measurement: 'm', category, target })).toEqual({
      category,
      [tagKey]: target,
    });
  });

  it('모르는 분류는 target 태그로 떨어진다', () => {
    // 새 그룹이 붙어도 시리즈가 사라지지 않는다 — 이름만 덜 구체적일 뿐이다.
    expect(historySeriesTags({ measurement: 'm', category: 'weird', target: 'x' })).toEqual({
      category: 'weird',
      target: 'x',
    });
  });

  it('빈 대상은 종합이라 대상 태그를 붙이지 않는다', () => {
    expect(historySeriesTags({ measurement: 'bytes_recv', category: 'network', target: '' })).toEqual(
      { category: 'network' },
    );
  });

  it('식별자는 measurement + 태그로 정해진다', () => {
    const a = historySeriesId({ measurement: 'bytes_recv', category: 'network', target: 'en0' });
    const b = historySeriesId({ measurement: 'bytes_recv', category: 'network', target: 'en1' });
    expect(a).not.toBe(b);
    expect(a).toBe(historySeriesId({ measurement: 'bytes_recv', category: 'network', target: 'en0' }));
  });
});

describe('querySysmetricsMatrix — 버킷 접기', () => {
  beforeEach(() => queryAgent.mockReset());

  /** 요청 하나: cpu.usage_percent 를 1초 버킷으로. */
  function params(over: Partial<SeriesMatrixQuery> = {}): SeriesMatrixQuery {
    return {
      keys: ['usage_percent'],
      seriesFilters: [{ tags: { category: 'cpu' } }],
      startMs: 1_000,
      endMs: 5_000,
      intervalMs: 1_000,
      aggregation: 'average',
      ...over,
    } as SeriesMatrixQuery;
  }

  function respond(result: unknown) {
    // `queryAgent` 는 에이전트 결과를 **그대로** 돌려준다 — 클라이언트 인터셉터가
    // `{success, data}` 봉투를 이미 벗긴다. `{ result }` 로 감싸면 실제로는 없는
    // 계층을 테스트가 만들어 내는 것이다.
    queryAgent.mockResolvedValue(result);
  }

  it('같은 버킷의 표본을 집계해 한 점으로 접는다', async () => {
    respond({
      series: [{ measurement: 'usage_percent', category: 'cpu' }],
      points: [
        { time_ms: 1_100, values: [10] },
        { time_ms: 1_900, values: [20] },
        { time_ms: 2_100, values: [50] },
      ],
    });

    const matrix = await querySysmetricsMatrix('a1', params());
    // 1초 버킷: [1000,2000) 평균 15, [2000,3000) 50.
    expect(matrix.rows).toEqual([
      { bucketStartMs: 1_000, values: [15] },
      { bucketStartMs: 2_000, values: [50] },
    ]);
  });

  it('버킷 경계는 epoch 0 기준이다 — 조회 시작 시각과 무관하다', async () => {
    // Store 의 `bucketAndAggregate` 와 같은 규칙(벽시계 경계). 시작 시각 기준으로
    // 접으면 상대 창이 매 폴링마다 조금씩 밀리므로 같은 표본이 폴링마다 다른
    // 버킷에 들어가고, 점이 흔들린다.
    respond({
      series: [{ measurement: 'usage_percent', category: 'cpu' }],
      points: [{ time_ms: 1_500, values: [7] }],
    });

    const matrix = await querySysmetricsMatrix('a1', params({ startMs: 1_200, intervalMs: 1_000 }));
    expect(matrix.rows[0]!.bucketStartMs).toBe(1_000);
  });

  it('시작 시각을 옮겨도 같은 표본은 같은 버킷에 든다', async () => {
    // 상대 창은 폴링마다 startMs 가 달라진다 — 그래도 격자는 고정이어야 한다.
    respond({
      series: [{ measurement: 'usage_percent', category: 'cpu' }],
      points: [{ time_ms: 1_500, values: [7] }],
    });
    const a = await querySysmetricsMatrix('a1', params({ startMs: 1_000 }));

    respond({
      series: [{ measurement: 'usage_percent', category: 'cpu' }],
      points: [{ time_ms: 1_500, values: [7] }],
    });
    const b = await querySysmetricsMatrix('a1', params({ startMs: 1_337 }));

    expect(a.rows[0]!.bucketStartMs).toBe(b.rows[0]!.bucketStartMs);
  });

  it('태그가 다른 시리즈는 짝지어지지 않는다', async () => {
    respond({
      series: [{ measurement: 'usage_percent', category: 'memory' }],
      points: [{ time_ms: 1_100, values: [99] }],
    });

    const matrix = await querySysmetricsMatrix('a1', params());
    expect(matrix.rows).toEqual([]);
  });

  it('숫자가 아닌 값은 버린다', async () => {
    respond({
      series: [{ measurement: 'usage_percent', category: 'cpu' }],
      points: [
        { time_ms: 1_100, values: [null] },
        { time_ms: 1_200, values: [4] },
      ],
    });

    const matrix = await querySysmetricsMatrix('a1', params());
    expect(matrix.rows).toEqual([{ bucketStartMs: 1_000, values: [4] }]);
  });

  it('이미 취소된 요청은 조회하지 않는다', async () => {
    const ac = new AbortController();
    ac.abort();
    await expect(querySysmetricsMatrix('a1', params(), ac.signal)).rejects.toThrow();
    expect(queryAgent).not.toHaveBeenCalled();
  });

  it('구간을 그대로 에이전트에 넘긴다', async () => {
    respond({ series: [], points: [] });
    await querySysmetricsMatrix('a1', params());
    expect(queryAgent).toHaveBeenCalledWith('a1', {
      command: 'get_history',
      params: { start_ms: 1_000, end_ms: 5_000 },
    });
  });
});
