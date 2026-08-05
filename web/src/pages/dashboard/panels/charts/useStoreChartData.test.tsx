// useStoreChartData 훅 + matrixToEntries 변환 테스트.
// queryMatrixFn 을 주입해 네트워크 없이 매트릭스 응답을 시뮬레이션한다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

/** 마이크로태스크 큐를 비워 resolved/rejected 프로미스 핸들러를 실행시킨다. */
async function flushMicrotasks(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

// useAgents 를 모킹해 store 에이전트 목록을 제어한다. 각 테스트가 필요 시
// mockUseAgentsData 를 바꿔 이름/ID 매핑을 시뮬레이션한다. (SPEC-WEB-006)
let mockUseAgentsData: { data: Array<{ id: string; name: string; type: string }> } | undefined = {
  data: [{ id: 'store-1', name: 'store-1', type: 'store' }],
};
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: mockUseAgentsData }),
}));

import type { SeriesMatrix } from '@/services/api/seriesDataSource';
import type { StoreSourceConfig } from './chartChannelTypes';
import {
  matrixToEntries,
  useStoreChartData,
  type QueryMatrixFn,
  type ResolveKeysFn,
} from './useStoreChartData';

/** 기본 store 소스 설정 헬퍼. */
function makeConfig(overrides: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    series: [{ key: 'room:temp', alias: 'Temp' }],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
    ...overrides,
  };
}

/** 두 컬럼 × 두 행 매트릭스. */
const sampleMatrix: SeriesMatrix = {
  columns: ['room:temp', 'room:humidity'],
  rows: [
    { bucketStartMs: 1000, values: [21.5, 40] },
    { bucketStartMs: 2000, values: [22.0, null] },
  ],
};

describe('matrixToEntries', () => {
  it('컬럼 수와 시리즈 수가 일치하면 alias/tags 를 정렬 매핑한다', () => {
    const config = makeConfig({
      series: [
        { key: 'room:temp', alias: 'Temp', tags: { room: '1' } },
        { key: 'room:humidity', alias: 'Humidity' },
      ],
    });
    const { entries, seriesEntries, seriesNames } = matrixToEntries(
      sampleMatrix,
      config,
    );

    // 시리즈 이름은 alias 로 매핑된다.
    expect(seriesNames).toEqual(['Temp', 'Humidity']);

    // 평탄화 entries 는 null 을 제외한 숫자 값만 포함하고 timestamp 오름차순.
    expect(entries.map((e) => e.value)).toEqual([21.5, 40, 22.0]);
    expect(entries.map((e) => e.timestamp)).toEqual([1000, 1000, 2000]);

    // labels.name 으로 시리즈를 구분, tags 도 병합된다.
    const temp1000 = entries.find(
      (e) => e.timestamp === 1000 && e.labels?.name === 'Temp',
    );
    expect(temp1000?.labels).toMatchObject({ name: 'Temp', room: '1' });

    // 시리즈별 타임라인은 null 도 포함(라인 gap).
    expect(seriesEntries.get('Humidity')?.map((e) => e.value)).toEqual([40, null]);
    expect(seriesEntries.get('Temp')?.map((e) => e.value)).toEqual([21.5, 22.0]);
  });

  it('컬럼 수가 시리즈 수와 다르면 컬럼명을 시리즈 이름으로 사용한다', () => {
    // 시리즈 1개를 요청했지만 매트릭스가 2개 컬럼으로 확장된 경우.
    const config = makeConfig({ series: [{ key: 'room:temp', alias: 'Ignored' }] });
    const { seriesNames } = matrixToEntries(sampleMatrix, config);
    expect(seriesNames).toEqual(['room:temp', 'room:humidity']);
  });

  it('alias 가 설정된 시리즈는 커스텀 이름으로 렌더된다(범례/라인/카테고리 라벨)', () => {
    // 단일 컬럼 매트릭스 + alias 지정 → 표시 이름/labels.name 모두 alias 로 매핑된다.
    const matrix: SeriesMatrix = {
      columns: ['room:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    };
    const config = makeConfig({ series: [{ key: 'room:temp', alias: '실내 온도' }] });
    const { seriesNames, seriesEntries, entries } = matrixToEntries(matrix, config);

    // 시리즈 이름(범례/라인 dataKey)이 alias 다.
    expect(seriesNames).toEqual(['실내 온도']);
    expect([...seriesEntries.keys()]).toEqual(['실내 온도']);
    // Bar/Pie 카테고리 라벨로 쓰이는 labels.name 도 alias 다.
    expect(entries[0]!.labels?.name).toBe('실내 온도');
  });

  it('alias 가 비어있으면 컬럼/키 이름으로 폴백한다(하위 호환)', () => {
    const matrix: SeriesMatrix = {
      columns: ['room:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    };
    // 공백 alias 는 key/컬럼명으로 폴백.
    const config = makeConfig({ series: [{ key: 'room:temp', alias: '   ' }] });
    const { seriesNames } = matrixToEntries(matrix, config);
    expect(seriesNames).toEqual(['room:temp']);
  });

  it('per-line 스타일을 시리즈 표시 이름 기준 seriesStyles 로 노출한다(SPEC-WEB-005)', () => {
    const matrix: SeriesMatrix = {
      columns: ['room:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    };
    const config = makeConfig({
      series: [
        {
          key: 'room:temp',
          alias: 'Temp',
          color: '#ff0000',
          stroke_style: 'dashed',
          stroke_width: 4,
          smooth: true,
        },
      ],
    });
    const { seriesStyles } = matrixToEntries(matrix, config);
    expect(seriesStyles.get('Temp')).toEqual({
      color: '#ff0000',
      stroke_style: 'dashed',
      stroke_width: 4,
      smooth: true,
    });
  });

  it('data_type=boolean 시리즈를 booleanSeries 로 노출한다', () => {
    const matrix: SeriesMatrix = {
      columns: ['room:temp', 'room:power'],
      rows: [{ bucketStartMs: 1000, values: [21.5, 1] }],
    };
    const config = makeConfig({
      series: [
        { key: 'room:temp', alias: 'Temp', data_type: 'float' },
        { key: 'room:power', alias: 'Power', data_type: 'boolean' },
      ],
    });
    const { booleanSeries } = matrixToEntries(matrix, config);
    expect(booleanSeries.has('Power')).toBe(true);
    expect(booleanSeries.has('Temp')).toBe(false);
  });

  it('alias 의 태그 토큰을 시리즈 태그 값으로 해석해 표시 이름에 반영한다(SPEC-WEB-005)', () => {
    const matrix: SeriesMatrix = {
      columns: ['room:1:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    };
    const config = makeConfig({
      series: [
        {
          key: 'room:1:temp',
          alias: '{$.name}-{$.type}',
          tags: { name: 'TempSensor', type: 'inside' },
        },
      ],
    });
    const { seriesNames, seriesEntries, entries } = matrixToEntries(matrix, config);
    // 토큰이 태그 값으로 해석된 이름이 시리즈/라벨/평탄화 entries 에 반영된다.
    expect(seriesNames).toEqual(['TempSensor-inside']);
    expect([...seriesEntries.keys()]).toEqual(['TempSensor-inside']);
    expect(entries[0]!.labels?.name).toBe('TempSensor-inside');
  });

  it('누락 태그 토큰은 빈 문자열로 해석된다', () => {
    const matrix: SeriesMatrix = {
      columns: ['k'],
      rows: [{ bucketStartMs: 1000, values: [1] }],
    };
    const config = makeConfig({
      series: [{ key: 'k', alias: '{$.name}-{$.missing}', tags: { name: 'A' } }],
    });
    const { seriesNames } = matrixToEntries(matrix, config);
    expect(seriesNames).toEqual(['A-']);
  });
});

describe('useStoreChartData', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // 기본 에이전트 목록: id === name === 'store-1'.
    mockUseAgentsData = { data: [{ id: 'store-1', name: 'store-1', type: 'store' }] };
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('비활성(enabled=false) 이면 idle 을 유지하고 쿼리하지 않는다', () => {
    const queryFn = vi.fn<QueryMatrixFn>();
    const { result } = renderHook(() =>
      useStoreChartData(makeConfig(), false, { queryMatrixFn: queryFn }),
    );
    expect(result.current.status).toBe('idle');
    expect(result.current.entries).toEqual([]);
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('series 가 비어있으면 idle 을 유지한다', () => {
    const queryFn = vi.fn<QueryMatrixFn>();
    const { result } = renderHook(() =>
      useStoreChartData(makeConfig({ series: [] }), true, {
        queryMatrixFn: queryFn,
      }),
    );
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('활성화 시 즉시 1회 조회하고 connected 로 entries 를 채운다', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    const { result } = renderHook(() =>
      useStoreChartData(makeConfig(), true, {
        queryMatrixFn: queryFn,
        nowFn: () => 100_000,
      }),
    );

    // 즉시 호출됨(connecting → connected).
    expect(queryFn).toHaveBeenCalledTimes(1);
    // now=100000, window=60000 → startMs=40000, endMs=100000, interval=10000.
    const callArgs = queryFn.mock.calls[0]!;
    expect(callArgs[0]).toBe('store-1');
    expect(callArgs[1]).toMatchObject({
      keys: ['room:temp'],
      startMs: 40_000,
      endMs: 100_000,
      intervalMs: 10_000,
      aggregation: 'average',
    });

    await flushMicrotasks();
    expect(result.current.status).toBe('connected');
    expect(result.current.entries.length).toBeGreaterThan(0);
  });

  it('스타일(smooth)만 변경 시 재조회 없이 seriesStyles 가 즉시 갱신된다 (곡선 적용 회귀)', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue({
      columns: ['room:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    });
    const cfgFalse = makeConfig({
      series: [{ key: 'room:temp', alias: 'Temp', smooth: false }],
    });
    const cfgTrue = makeConfig({
      series: [{ key: 'room:temp', alias: 'Temp', smooth: true }],
    });
    const { result, rerender } = renderHook(
      ({ cfg }) =>
        useStoreChartData(cfg, true, {
          queryMatrixFn: queryFn,
          nowFn: () => 100_000,
        }),
      { initialProps: { cfg: cfgFalse } },
    );
    await flushMicrotasks();
    expect(result.current.seriesStyles.get('Temp')?.smooth).toBe(false);

    // smooth 만 변경 — key/tags/window 동일 → pollKey 불변 → 재조회 발생하면 안 된다.
    rerender({ cfg: cfgTrue });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(1);
    // 데이터 재조회 없이도 스타일은 즉시 반영되어야 한다.
    expect(result.current.seriesStyles.get('Temp')?.smooth).toBe(true);
  });

  it('refresh_interval_ms 주기로 폴링한다', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(makeConfig({ refresh_interval_ms: 5_000 }), true, {
        queryMatrixFn: queryFn,
      }),
    );
    expect(queryFn).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    expect(queryFn).toHaveBeenCalledTimes(2);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    expect(queryFn).toHaveBeenCalledTimes(3);
  });

  it('쿼리 실패 시 error 상태와 사유를 노출한다', async () => {
    const queryFn = vi
      .fn<QueryMatrixFn>()
      .mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() =>
      useStoreChartData(makeConfig(), true, { queryMatrixFn: queryFn }),
    );
    await flushMicrotasks();
    expect(result.current.status).toBe('error');
    expect(result.current.errorReason).toBe('boom');
  });

  it('언마운트 시 진행 중 요청을 abort 하고 폴링을 멈춘다', async () => {
    let capturedSignal: AbortSignal | undefined;
    const queryFn = vi.fn<QueryMatrixFn>((_, __, signal) => {
      capturedSignal = signal;
      return new Promise(() => {
        // 영원히 pending — abort 만 관찰한다.
      });
    });
    const { unmount } = renderHook(() =>
      useStoreChartData(makeConfig(), true, { queryMatrixFn: queryFn }),
    );
    expect(capturedSignal?.aborted).toBe(false);
    unmount();
    expect(capturedSignal?.aborted).toBe(true);

    // 언마운트 후에는 추가 폴링이 없어야 한다.
    const callsAfterUnmount = queryFn.mock.calls.length;
    await act(async () => {
      vi.advanceTimersByTime(10_000);
      await Promise.resolve();
    });
    expect(queryFn).toHaveBeenCalledTimes(callsAfterUnmount);
  });

  it('시리즈에 metric_type/tags 가 있으면 seriesFilters 를 전송한다', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(
        makeConfig({
          series: [
            { key: 'room:temp', metric_type: 'gauge', tags: { room: '1' } },
          ],
        }),
        true,
        { queryMatrixFn: queryFn },
      ),
    );
    const params = queryFn.mock.calls[0]![1];
    expect(params.seriesFilters).toEqual([
      { metricType: 'gauge', tags: { room: '1' } },
    ]);
    // 즉시 쿼리가 resolve 되며 발생하는 상태 업데이트를 flush 한다(act 경고 제거).
    await flushMicrotasks();
  });

  // --- agent_id 기반 이름 해석 (SPEC-WEB-006) ---
  // config 에 안정적인 agent_id 를 정본으로 저장하고, Store API(이름 주소) 호출 시
  // 저장된 id → 현재 에이전트 이름으로 해석해 호출한다. 에이전트 이름이 바뀌어도
  // id 는 불변이므로 항상 현재 이름으로 조회되어야 한다.
  it('agent_id 가 있으면 저장된 옛 이름 대신 현재 에이전트 이름으로 조회한다', async () => {
    // 저장된 config: agent_id='store-uuid', 저장 시점 이름은 'old'.
    // 현재 에이전트 목록: 같은 id 의 이름이 'new' 로 변경됨.
    mockUseAgentsData = { data: [{ id: 'store-uuid', name: 'new', type: 'store' }] };
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(
        makeConfig({ agent_id: 'store-uuid', agent_name: 'old' }),
        true,
        { queryMatrixFn: queryFn },
      ),
    );
    expect(queryFn).toHaveBeenCalledTimes(1);
    // 옛 이름('old')이 아닌 현재 이름('new')으로 호출되어야 한다.
    expect(queryFn.mock.calls[0]![0]).toBe('new');
    await flushMicrotasks();
  });

  it('agent_id 가 없으면(구 config) 저장된 이름을 그대로 사용한다(하위호환)', async () => {
    mockUseAgentsData = { data: [{ id: 'store-uuid', name: 'new', type: 'store' }] };
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(
        makeConfig({ agent_name: 'legacy-name' }),
        true,
        { queryMatrixFn: queryFn },
      ),
    );
    expect(queryFn).toHaveBeenCalledTimes(1);
    expect(queryFn.mock.calls[0]![0]).toBe('legacy-name');
    await flushMicrotasks();
  });

  it('agent_id 가 목록에 없으면 저장된 이름으로 폴백한다', async () => {
    mockUseAgentsData = { data: [{ id: 'other', name: 'other-name', type: 'store' }] };
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(
        makeConfig({ agent_id: 'missing-uuid', agent_name: 'snapshot-name' }),
        true,
        { queryMatrixFn: queryFn },
      ),
    );
    expect(queryFn).toHaveBeenCalledTimes(1);
    expect(queryFn.mock.calls[0]![0]).toBe('snapshot-name');
    await flushMicrotasks();
  });
});

// --- tag 자동 확장 모드 (SPEC-WEB-005) ---
// selection_mode='tag' 이면 폴링마다 tag_filters 매칭 키를 먼저 해석하고, 그 키들을
// 동적으로 시리즈로 확장해 queryMatrix 로 팬아웃한다. 태그 하위 키가 추가/삭제되면
// 다음 폴링에서 자동 반영된다.
describe('useStoreChartData tag 모드', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockUseAgentsData = { data: [{ id: 'store-1', name: 'store-1', type: 'store' }] };
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  /** 전달된 keys 로부터 매트릭스를 만드는 쿼리 스텁(각 키 1컬럼, 값 1). */
  const matrixFromKeys: QueryMatrixFn = (_agent, params) =>
    Promise.resolve({
      columns: params.keys,
      rows: [{ bucketStartMs: 1000, values: params.keys.map(() => 1) }],
    });

  it('tag_filters 매칭 키를 해석해 정확히 그 키들로 팬아웃하고 키당 1시리즈를 만든다', async () => {
    const resolveFn = vi
      .fn<ResolveKeysFn>()
      .mockResolvedValue(['room:1:temp', 'room:1:humidity']);
    const queryFn = vi.fn<QueryMatrixFn>(matrixFromKeys);
    const { result } = renderHook(() =>
      useStoreChartData(
        makeConfig({
          selection_mode: 'tag',
          tag_filters: { room: '1' },
          series: [],
        }),
        true,
        { queryMatrixFn: queryFn, resolveKeysFn: resolveFn, nowFn: () => 100_000 },
      ),
    );

    // 먼저 tag_filters 로 키를 해석한다.
    expect(resolveFn).toHaveBeenCalledTimes(1);
    expect(resolveFn.mock.calls[0]![0]).toBe('store-1');
    expect(resolveFn.mock.calls[0]![1]).toEqual({ room: '1' });

    await flushMicrotasks();

    // 해석된 키 집합으로 정확히 팬아웃한다.
    expect(queryFn).toHaveBeenCalledTimes(1);
    expect(queryFn.mock.calls[0]![1].keys).toEqual([
      'room:1:temp',
      'room:1:humidity',
    ]);
    // seriesFilters 는 부여하지 않는다(키의 모든 시리즈 조회).
    expect(queryFn.mock.calls[0]![1].seriesFilters).toBeUndefined();

    // 키당 1시리즈(별칭 = 키명).
    expect(result.current.status).toBe('connected');
    expect(result.current.seriesNames).toEqual([
      'room:1:temp',
      'room:1:humidity',
    ]);
  });

  it('다음 폴링에서 해석된 키 집합이 바뀌면 시리즈 집합도 바뀐다', async () => {
    const resolveFn = vi
      .fn<ResolveKeysFn>()
      .mockResolvedValueOnce(['room:1:temp', 'room:1:humidity'])
      .mockResolvedValueOnce(['room:1:temp', 'room:1:co2']);
    const queryFn = vi.fn<QueryMatrixFn>(matrixFromKeys);
    const { result } = renderHook(() =>
      useStoreChartData(
        makeConfig({
          selection_mode: 'tag',
          tag_filters: { room: '1' },
          series: [],
          refresh_interval_ms: 5_000,
        }),
        true,
        { queryMatrixFn: queryFn, resolveKeysFn: resolveFn, nowFn: () => 100_000 },
      ),
    );
    await flushMicrotasks();
    expect(result.current.seriesNames).toEqual([
      'room:1:temp',
      'room:1:humidity',
    ]);

    // 두 번째 폴링: room:1:humidity 가 사라지고 room:1:co2 가 추가됨.
    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(resolveFn).toHaveBeenCalledTimes(2);
    expect(result.current.seriesNames).toEqual(['room:1:temp', 'room:1:co2']);
  });

  it('매칭 키가 0개면 빈 차트(connected, no-data)로 표시하고 매트릭스 조회를 생략한다', async () => {
    const resolveFn = vi.fn<ResolveKeysFn>().mockResolvedValue([]);
    const queryFn = vi.fn<QueryMatrixFn>(matrixFromKeys);
    const { result } = renderHook(() =>
      useStoreChartData(
        makeConfig({
          selection_mode: 'tag',
          tag_filters: { room: '99' },
          series: [],
        }),
        true,
        { queryMatrixFn: queryFn, resolveKeysFn: resolveFn },
      ),
    );
    await flushMicrotasks();
    // 키 0개 → 매트릭스 조회 없이 빈 결과(크래시 없음).
    expect(queryFn).not.toHaveBeenCalled();
    expect(result.current.status).toBe('connected');
    expect(result.current.seriesNames).toEqual([]);
    expect(result.current.entries).toEqual([]);
  });

  it('tag_filters 가 비어있으면 idle 을 유지하고 아무 것도 조회하지 않는다', () => {
    const resolveFn = vi.fn<ResolveKeysFn>();
    const queryFn = vi.fn<QueryMatrixFn>();
    const { result } = renderHook(() =>
      useStoreChartData(
        makeConfig({ selection_mode: 'tag', tag_filters: {}, series: [] }),
        true,
        { queryMatrixFn: queryFn, resolveKeysFn: resolveFn },
      ),
    );
    expect(result.current.status).toBe('idle');
    expect(resolveFn).not.toHaveBeenCalled();
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('keys 모드(기본)에서는 키 해석기를 호출하지 않는다(하위 호환)', async () => {
    const resolveFn = vi.fn<ResolveKeysFn>();
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(makeConfig(), true, {
        queryMatrixFn: queryFn,
        resolveKeysFn: resolveFn,
      }),
    );
    expect(resolveFn).not.toHaveBeenCalled();
    expect(queryFn).toHaveBeenCalledTimes(1);
    await flushMicrotasks();
  });
});
