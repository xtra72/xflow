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

  it('alias(시리즈 표시 이름) 변경 시 재구독하여 범례 이름이 갱신된다 (범례 이름 안바뀜 회귀)', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue({
      columns: ['room:temp'],
      rows: [{ bucketStartMs: 1000, values: [21.5] }],
    });
    const cfgA = makeConfig({ series: [{ key: 'room:temp', alias: 'Temp' }] });
    const cfgB = makeConfig({ series: [{ key: 'room:temp', alias: 'Renamed' }] });
    const { result, rerender } = renderHook(
      ({ cfg }) =>
        useStoreChartData(cfg, true, {
          queryMatrixFn: queryFn,
          nowFn: () => 100_000,
        }),
      { initialProps: { cfg: cfgA } },
    );
    await flushMicrotasks();
    expect([...result.current.seriesEntries.keys()]).toEqual(['Temp']);

    // alias 만 변경 — pollKey 에 alias 가 포함되므로 재구독→재변환되어 표시 이름이 갱신된다.
    rerender({ cfg: cfgB });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect([...result.current.seriesEntries.keys()]).toEqual(['Renamed']);
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

  it('시리즈에 field/tags 가 있으면 seriesFilters 를 전송한다', async () => {
    const queryFn = vi.fn<QueryMatrixFn>().mockResolvedValue(sampleMatrix);
    renderHook(() =>
      useStoreChartData(
        makeConfig({
          series: [
            { key: 'room:temp', field: 'gauge', tags: { room: '1' } },
          ],
        }),
        true,
        { queryMatrixFn: queryFn },
      ),
    );
    const params = queryFn.mock.calls[0]![1];
    expect(params.seriesFilters).toEqual([
      { fieldName: 'gauge', tags: { room: '1' } },
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

// ===== AC-16 안전망 — Store 경로 렌더 결과 무변경 (SPEC-TSDB-004 M5) =====
//
// matrixToEntries 는 Store 와 TSDB 가 **공유**한다. M5 가 위치 정렬을 라벨/출처
// 기반 귀속으로 바꾸므로, 바꾸기 **전에** 현재 Store 동작을 골든으로 고정한다.
// 이 describe 가 깨지면 기존 대시보드의 표시 이름·색·선스타일이 흔들린 것이다.
describe('matrixToEntries — AC-16 Store 렌더 무변경 골든', () => {
  const storeMatrix: SeriesMatrix = {
    columns: ['room:temp', 'room:humidity', 'room:co2'],
    rows: [
      { bucketStartMs: 1000, values: [21.5, 40, null] },
      { bucketStartMs: 2000, values: [22.0, null, 800] },
    ],
  };

  const storeConfig = makeConfig({
    series: [
      {
        key: 'room:temp',
        alias: '실내 온도',
        tags: { room: '1' },
        color: '#ff0000',
        stroke_style: 'dashed',
        stroke_width: 3,
        smooth: true,
      },
      { key: 'room:humidity', tags: { room: '1' }, color: '#00ff00' },
      { key: 'room:co2', data_type: 'boolean' },
    ],
    series_name_format: '{$.measurement}',
  });

  it('시리즈 이름 · 스타일 · boolean 판정이 현행과 같다', () => {
    const r = matrixToEntries(storeMatrix, storeConfig);

    // 1) 표시 이름 — alias 우선, 없으면 name_format, 없으면 서술 표기.
    expect(r.seriesNames).toEqual(['실내 온도', 'room:humidity', 'room:co2']);

    // 2) per-line 스타일이 위치 순서대로 귀속된다.
    expect(r.seriesStyles.get('실내 온도')).toEqual({
      color: '#ff0000',
      stroke_style: 'dashed',
      stroke_width: 3,
      smooth: true,
    });
    expect(r.seriesStyles.get('room:humidity')?.color).toBe('#00ff00');

    // 3) data_type='boolean' 표기.
    expect(r.booleanSeries.has('room:co2')).toBe(true);
    expect(r.booleanSeries.has('실내 온도')).toBe(false);

    // 4) labels 에 tags 가 병합되고 name 이 마지막에 온다.
    const temp = r.entries.find((e) => e.labels?.name === '실내 온도');
    expect(temp?.labels).toMatchObject({ name: '실내 온도', room: '1' });

    // 5) 시리즈별 타임라인은 null 을 유지(라인 gap), 평탄화는 숫자만.
    expect(r.seriesEntries.get('room:humidity')?.map((e) => e.value)).toEqual([40, null]);
    expect(r.entries.map((e) => e.value)).toEqual([21.5, 40, 22.0, 800]);
    expect(r.entries.map((e) => e.timestamp)).toEqual([1000, 1000, 2000, 2000]);
  });

  it('컬럼 수 불일치 시 컬럼명 폴백이 유지된다', () => {
    // 출처 정보가 없는 매트릭스(다른 생산자)에서는 종전 정렬 규칙이 그대로다.
    const shorter = makeConfig({ series: [{ key: 'room:temp', alias: 'Ignored' }] });
    const r = matrixToEntries(storeMatrix, shorter);
    expect(r.seriesNames).toEqual(['room:temp', 'room:humidity', 'room:co2']);
    expect(r.seriesStyles.size).toBe(0);
  });
});

// ===== AC-15 — group by 혼재 패널 메타데이터 귀속 (SPEC-TSDB-004 M5) =====
//
// 종전 규칙(`aligned = columns.length === series.length`)은 패널 전체에 대한
// 단일 불리언이었다. group by 항목 하나가 컬럼을 늘리면 등식이 깨져 같은 패널의
// **정확 일치 항목까지** alias·color·선스타일을 전부 잃었다(UB1-7).
describe('matrixToEntries — AC-15 group by 혼재 패널', () => {
  // 컬럼 4개 · config 항목 2개 — 종전이라면 aligned=false 로 전부 폴백했다.
  //   [0] 정확 일치 room:temp
  //   [1..3] group by cpu (host=a/b/c)
  const mixedMatrix: SeriesMatrix = {
    columns: ['room:temp', 'cpu{host=a}', 'cpu{host=b}', 'cpu{host=c}'],
    rows: [{ bucketStartMs: 1000, values: [21.5, 1, 2, 3] }],
    columnOrigins: [0, 1, 1, 1],
    columnLabels: [
      { __field__: 'value' },
      { __field__: 'usage', host: 'a' },
      { __field__: 'usage', host: 'b' },
      { __field__: 'usage', host: 'c' },
    ],
  };

  const mixedConfig = makeConfig({
    series: [
      {
        key: 'room:temp',
        alias: '실내 온도',
        color: '#ff0000',
        stroke_style: 'dashed',
        stroke_width: 3,
      },
      {
        key: 'cpu',
        field: 'usage',
        alias: 'CPU',
        color: '#0000ff',
        stroke_style: 'dotted',
        stroke_width: 2,
        smooth: true,
      },
    ],
  });

  it('정확 일치 항목이 메타데이터를 그대로 유지한다 (UB1-7)', () => {
    const r = matrixToEntries(mixedMatrix, mixedConfig);

    expect(r.seriesNames[0]).toBe('실내 온도');
    expect(r.seriesStyles.get('실내 온도')).toEqual({
      color: '#ff0000',
      stroke_style: 'dashed',
      stroke_width: 3,
      smooth: undefined,
    });
  });

  it('group by 파생 컬럼은 서로 다른 이름을 갖되 태그를 이름에 넣지 않는다', () => {
    // v0.21.0 이전에는 구분자로 그룹 태그 값(`{host=a}`)을 붙였다. 사용자가
    // 이름을 지어 둔 자리에 기계 문자열이 덧붙는 것이 보고되어(§2.13),
    // 구분자는 태그 없는 최소 번호로 바뀌었다. 필요한 성질은 "서로 다를 것"뿐이다.
    const r = matrixToEntries(mixedMatrix, mixedConfig);
    const groupNames = r.seriesNames.slice(1);

    // 세 이름이 서로 달라야 한다 — 같으면 한 줄로 병합되어 그룹이 사라진다.
    expect(new Set(groupNames).size).toBe(3);
    for (const n of groupNames) {
      expect(n).toContain('CPU');
      expect(n).not.toContain('host');
    }
  });

  it('group by 파생 컬럼은 항목 color 를 쓰지 않고 선 모양은 공유한다 (OQ1)', () => {
    const r = matrixToEntries(mixedMatrix, mixedConfig);

    for (const name of r.seriesNames.slice(1)) {
      const style = r.seriesStyles.get(name);
      expect(style?.color).toBeUndefined();
      // 선 모양은 "이 항목의 선 모양" 이므로 전 그룹이 공유한다.
      expect(style?.stroke_style).toBe('dotted');
      expect(style?.stroke_width).toBe(2);
      expect(style?.smooth).toBe(true);
    }
  });

  it('group by 파생 컬럼의 labels 에 그룹 태그가 실린다', () => {
    const r = matrixToEntries(mixedMatrix, mixedConfig);
    const hosts = r.entries
      .filter((e) => e.labels?.host !== undefined)
      .map((e) => e.labels!.host);
    expect(new Set(hosts)).toEqual(new Set(['a', 'b', 'c']));
  });

  it('출처 정보가 없으면 종전 위치 정렬로 되돌아간다 (하위 호환)', () => {
    // columnOrigins 를 뺀 같은 매트릭스 — 컬럼 4 vs 시리즈 2 이므로 폴백.
    const noOrigins: SeriesMatrix = {
      columns: mixedMatrix.columns,
      rows: mixedMatrix.rows,
    };
    const r = matrixToEntries(noOrigins, mixedConfig);
    expect(r.seriesNames).toEqual(mixedMatrix.columns);
    expect(r.seriesStyles.size).toBe(0);
  });

  it('group by 항목만 있는 패널도 정상 동작한다', () => {
    const onlyGroup: SeriesMatrix = {
      columns: ['cpu{host=a}', 'cpu{host=b}'],
      rows: [{ bucketStartMs: 1000, values: [1, 2] }],
      columnOrigins: [0, 0],
      columnLabels: [
        { __field__: 'usage', host: 'a' },
        { __field__: 'usage', host: 'b' },
      ],
    };
    const cfg = makeConfig({
      series: [{ key: 'cpu', field: 'usage', color: '#0000ff' }],
    });
    const r = matrixToEntries(onlyGroup, cfg);
    expect(new Set(r.seriesNames).size).toBe(2);
    for (const n of r.seriesNames) expect(r.seriesStyles.get(n)?.color).toBeUndefined();
  });
});

describe('useStoreChartData — 이름 형식 변경이 재조회를 촉발한다 (TSDB 와 같은 축)', () => {
  it('series_name_format 만 바꿔도 다시 조회한다', async () => {
    const queryMatrixFn = vi.fn(async () => sampleMatrix);
    const { rerender } = renderHook(
      ({ fmt }: { fmt: string | undefined }) =>
        useStoreChartData(
          makeConfig(fmt === undefined ? {} : { series_name_format: fmt }),
          true,
          { queryMatrixFn },
        ),
      { initialProps: { fmt: undefined as string | undefined } },
    );
    await act(async () => {
      await Promise.resolve();
    });
    const before = queryMatrixFn.mock.calls.length;
    expect(before).toBeGreaterThan(0);

    rerender({ fmt: '{$.measurement}' });
    await act(async () => {
      await Promise.resolve();
    });
    expect(queryMatrixFn.mock.calls.length).toBeGreaterThan(before);
  });
});

describe('matrixToEntries — 이름 충돌 해소에 태그를 쓰지 않는다 (사용자 보고)', () => {
  // 1차 보고: `{$.location}` 을 지정했는데 범례가
  // `실습실 {device.dev_eui=…, device.type=EM300-TH, location=실습실}` 로 나온다.
  // → 구분에 기여하는 키(dev_eui)만 남기도록 좁혔다(v0.18.0).
  // 2차 보고: 그래도 `실습실 {device.dev_eui=24e124136d151523}` 이라 길다.
  // → dev_eui 는 읽어도 무엇인지 알 수 없으면서 지정한 이름보다 길다. 구분자는
  //   태그 없는 최소 번호로 바꾼다(§2.13). 정확한 통제는 그룹별 이름(§2.12).
  const matrix: SeriesMatrix = {
    columns: ['c0', 'c1', 'c2'],
    rows: [{ bucketStartMs: 1000, values: [1, 2, 3] }],
    columnOrigins: [0, 0, 0],
    columnLabels: [
      { __field__: 'value', 'device.dev_eui': 'A', 'device.type': 'EM300-TH', location: '실습실' },
      { __field__: 'value', 'device.dev_eui': 'B', 'device.type': 'EM300-TH', location: '실습실' },
      { __field__: 'value', 'device.dev_eui': 'C', 'device.type': 'EM300-TH', location: '실습실' },
    ],
  };
  const config = makeConfig({
    series: [{ key: 'th', field: 'value' }],
    series_name_format: '{$.tags.location}',
  });

  it('어떤 태그도 이름에 덧붙이지 않는다', () => {
    const r = matrixToEntries(matrix, config);
    for (const n of r.seriesNames) {
      expect(n).toContain('실습실');
      expect(n).not.toContain('device.dev_eui');
      expect(n).not.toContain('device.type');
      expect(n).not.toContain('location=');
      expect(n).not.toContain('{');
    }
  });

  it('세 줄이 서로 다른 이름을 갖는다 (병합되지 않는다)', () => {
    const r = matrixToEntries(matrix, config);
    expect(new Set(r.seriesNames).size).toBe(3);
    expect(r.seriesEntries.size).toBe(3);
  });

  it('시리즈별 이름을 지정하면 덧붙이지 않는다', () => {
    // 이름이 이미 서로 다르면 충돌 분기를 아예 타지 않는다 — 지정한 것만 나온다.
    const named = makeConfig({
      series: [{ key: 'th', field: 'value', alias: '{$.tags.device.dev_eui}' }],
      series_name_format: '{$.tags.location}',
    });
    const r = matrixToEntries(matrix, named);
    expect(new Set(r.seriesNames).size).toBe(3);
    for (const n of r.seriesNames) expect(n).not.toContain('{');
  });
});

// ===== 그룹별 개별 이름 (SPEC-TSDB-004 §2.12) =====

describe('matrixToEntries — 그룹별 개별 이름', () => {
  /** host 로 나뉜 3그룹 매트릭스. 항목은 하나다. */
  const grouped: SeriesMatrix = {
    columns: ['cpu.usage#0', 'cpu.usage#1', 'cpu.usage#2'],
    rows: [{ bucketStartMs: 0, values: [1, 2, 3] }],
    columnOrigins: [0, 0, 0],
    columnLabels: [
      { __field__: 'usage', host: 'a' },
      { __field__: 'usage', host: 'b' },
      { __field__: 'usage', host: 'c' },
    ],
  };

  it('group_alias 가 그룹마다 다른 이름을 준다', () => {
    const config = makeConfig({
      series: [
        {
          key: 'cpu',
          field: 'usage',
          group_by: ['host'],
          group_alias: { a: '실습실', c: '사무실' },
        },
      ],
    });
    const { seriesNames } = matrixToEntries(grouped, config);
    // 지정한 그룹은 그 이름 그대로. 군더더기 태그 표기가 붙지 않는다.
    expect(seriesNames[0]).toBe('실습실');
    expect(seriesNames[2]).toBe('사무실');
    // 지정하지 않은 그룹은 종전 규칙(형식/서술 표기)을 그대로 따른다.
    expect(seriesNames[1]).not.toBe('실습실');
    expect(seriesNames[1]).not.toBe('사무실');
  });

  it('그룹별 이름이 항목 이름(템플릿)을 이긴다', () => {
    const config = makeConfig({
      series: [
        {
          key: 'cpu',
          field: 'usage',
          alias: 'CPU {$.tags.host}',
          group_by: ['host'],
          group_alias: { b: '지정한 이름' },
        },
      ],
    });
    const { seriesNames } = matrixToEntries(grouped, config);
    expect(seriesNames[0]).toBe('CPU a');
    expect(seriesNames[1]).toBe('지정한 이름');
    expect(seriesNames[2]).toBe('CPU c');
  });

  it('다중 키 그룹은 정렬된 키 순서의 조합 서명으로 찾는다', () => {
    const multi: SeriesMatrix = {
      columns: ['m.f#0', 'm.f#1'],
      rows: [{ bucketStartMs: 0, values: [1, 2] }],
      columnOrigins: [0, 0],
      columnLabels: [
        { __field__: 'f', zone: 'z1', host: 'a' },
        { __field__: 'f', zone: 'z2', host: 'b' },
      ],
    };
    // group_by 를 역순으로 줘도 서명은 정렬 순서(host, zone)로 만든다.
    const config = makeConfig({
      series: [
        {
          key: 'm',
          field: 'f',
          group_by: ['zone', 'host'],
          group_alias: { [`a\u0000z1`]: '첫 조합' },
        },
      ],
    });
    const { seriesNames } = matrixToEntries(multi, config);
    expect(seriesNames[0]).toBe('첫 조합');
  });

  it('그룹 파생이 아닌 항목에서는 group_alias 를 무시한다', () => {
    const flat: SeriesMatrix = {
      columns: ['cpu.usage'],
      rows: [{ bucketStartMs: 0, values: [1] }],
      columnOrigins: [0],
      columnLabels: [{ __field__: 'usage', host: 'a' }],
    };
    const config = makeConfig({
      series: [{ key: 'cpu', field: 'usage', alias: '고정', group_alias: { a: '무시됨' } }],
    });
    // 펼쳐지지 않은 항목은 시리즈가 하나이며 그 이름은 항목 이름이다.
    expect(matrixToEntries(flat, config).seriesNames[0]).toBe('고정');
  });
});

// ===== 이름 충돌 해소는 태그를 이름에 넣지 않는다 (SPEC-TSDB-004 §2.13) =====

describe('matrixToEntries — 충돌 해소 표기', () => {
  /** location 으로 이름 지은 3그룹. 두 줄이 같은 이름이 된다. */
  const m: SeriesMatrix = {
    columns: ['t.v#0', 't.v#1', 't.v#2'],
    rows: [{ bucketStartMs: 0, values: [1, 2, 3] }],
    columnOrigins: [0, 0, 0],
    columnLabels: [
      { __field__: 'v', 'device.dev_eui': 'a1', location: '실습실 안쪽' },
      { __field__: 'v', 'device.dev_eui': 'b2', location: '실습실' },
      { __field__: 'v', 'device.dev_eui': 'c3', location: '실습실' },
    ],
  };
  const config = makeConfig({
    series: [{ key: 't', field: 'v', group_by: ['device.dev_eui'] }],
    series_name_format: '{$.tags.location}',
  });

  it('이름에 태그 정보를 넣지 않는다', () => {
    const { seriesNames } = matrixToEntries(m, config);
    for (const n of seriesNames) {
      expect(n).not.toContain('dev_eui');
      expect(n).not.toContain('a1');
      expect(n).not.toContain('b2');
      expect(n).not.toContain('c3');
      expect(n).not.toContain('{');
    }
  });

  it('충돌하지 않는 줄은 지정한 이름 그대로 둔다', () => {
    // 종전에는 같은 출처의 줄이 하나라도 충돌하면 전부에 표기를 붙였다.
    expect(matrixToEntries(m, config).seriesNames[0]).toBe('실습실 안쪽');
  });

  it('충돌한 줄만 구분자를 얻고 서로 다른 이름이 된다', () => {
    const { seriesNames, seriesEntries } = matrixToEntries(m, config);
    expect(seriesNames[1]).not.toBe(seriesNames[2]);
    expect(seriesNames[1]).toContain('실습실');
    expect(seriesNames[2]).toContain('실습실');
    // 이름이 갈려야 줄이 병합되지 않는다 — 3그룹이면 3줄이다.
    expect(seriesEntries.size).toBe(3);
  });
});

// ===== 그룹별 라인 색 (SPEC-TSDB-004 §2.14) =====

describe('matrixToEntries — 그룹별 라인 색', () => {
  const grouped: SeriesMatrix = {
    columns: ['cpu.usage#0', 'cpu.usage#1'],
    rows: [{ bucketStartMs: 0, values: [1, 2] }],
    columnOrigins: [0, 0],
    columnLabels: [
      { __field__: 'usage', host: 'a' },
      { __field__: 'usage', host: 'b' },
    ],
  };

  it('group_color 가 지정된 그룹만 그 색을 쓴다', () => {
    const config = makeConfig({
      series: [
        {
          key: 'cpu',
          field: 'usage',
          group_by: ['host'],
          group_alias: { a: '실습실', b: '사무실' },
          group_color: { a: '#ff0000' },
        },
      ],
    });
    const { seriesStyles } = matrixToEntries(grouped, config);
    expect(seriesStyles.get('실습실')?.color).toBe('#ff0000');
    // 지정하지 않은 그룹은 자동 팔레트로 넘긴다(색을 비워 둔다).
    expect(seriesStyles.get('사무실')?.color).toBeUndefined();
  });

  it('항목 color 는 그룹 파생 줄로 새지 않는다 (OQ1)', () => {
    const config = makeConfig({
      series: [
        {
          key: 'cpu',
          field: 'usage',
          color: '#0000ff',
          group_by: ['host'],
          group_alias: { a: '실습실', b: '사무실' },
          group_color: { b: '#00ff00' },
        },
      ],
    });
    const { seriesStyles } = matrixToEntries(grouped, config);
    // 색 하나를 N개 그룹에 나눠 줄 수 없으므로 항목 color 는 무시된다.
    expect(seriesStyles.get('실습실')?.color).toBeUndefined();
    expect(seriesStyles.get('사무실')?.color).toBe('#00ff00');
  });

  it('선 모양은 그룹 파생 줄이 항목 값을 그대로 공유한다', () => {
    const config = makeConfig({
      series: [
        {
          key: 'cpu',
          field: 'usage',
          stroke_style: 'dashed',
          stroke_width: 3,
          smooth: true,
          group_by: ['host'],
          group_alias: { a: '실습실', b: '사무실' },
        },
      ],
    });
    const { seriesStyles } = matrixToEntries(grouped, config);
    for (const n of ['실습실', '사무실']) {
      expect(seriesStyles.get(n)?.stroke_style).toBe('dashed');
      expect(seriesStyles.get(n)?.stroke_width).toBe(3);
      expect(seriesStyles.get(n)?.smooth).toBe(true);
    }
  });
});

// ===== 기본 이름에서 고정 태그 제거 (SPEC-TSDB-004 §2.15) =====

describe('matrixToEntries — 기본 이름의 고정 태그', () => {
  // 실제 보고된 설정: 사전 필터 location·device.type 은 고정이고 dev_eui 로 나눈다.
  const m: SeriesMatrix = {
    columns: ['a', 'b', 'c'],
    rows: [{ bucketStartMs: 0, values: [1, 2, 3] }],
    columnOrigins: [0, 0, 0],
    columnLabels: [
      { __field__: 'value', 'device.dev_eui': 'e1', 'device.type': 'EM300-TH', location: '실습실' },
      { __field__: 'value', 'device.dev_eui': 'e2', 'device.type': 'EM300-TH', location: '실습실' },
      { __field__: 'value', 'device.dev_eui': 'e3', 'device.type': 'EM300-TH', location: '실습실' },
    ],
  };
  const base = {
    key: 'temperature',
    field: 'value',
    tags: { location: '실습실', 'device.type': 'EM300-TH' },
    group_by: ['device.dev_eui'],
  };

  it('모든 줄에서 값이 같은 태그는 기본 이름에 넣지 않는다', () => {
    const { seriesNames } = matrixToEntries(m, makeConfig({ series: [base] }));
    for (const n of seriesNames) {
      // 구분에 기여하는 dev_eui 는 남는다.
      expect(n).toContain('device.dev_eui');
      // 세 줄 모두 같은 값이라 군더더기다.
      expect(n).not.toContain('device.type');
      expect(n).not.toContain('location');
    }
    expect(new Set(seriesNames).size).toBe(3);
  });

  it('이름 형식이 있으면 태그를 걷어내지 않는다 (템플릿이 참조한다)', () => {
    // location 은 고정이지만 형식이 그 값을 쓰므로 사라지면 안 된다.
    const cfg = makeConfig({ series: [base], series_name_format: '{$.tags.location}' });
    const { seriesNames } = matrixToEntries(m, cfg);
    for (const n of seriesNames) expect(n).toContain('실습실');
  });

  it('그룹별 이름이 있으면 그 이름이 그대로 나온다', () => {
    const cfg = makeConfig({
      series: [{ ...base, group_alias: { e1: '창가', e2: '중앙', e3: '문가' } }],
    });
    expect(matrixToEntries(m, cfg).seriesNames).toEqual(['창가', '중앙', '문가']);
  });

  it('그룹 파생이 아닌 항목의 이름은 종전 그대로다', () => {
    const flat: SeriesMatrix = {
      columns: ['x'],
      rows: [{ bucketStartMs: 0, values: [1] }],
      columnOrigins: [0],
      columnLabels: [{ __field__: 'value', location: '실습실' }],
    };
    const cfg = makeConfig({ series: [{ key: 'temperature', field: 'value', tags: { location: '실습실' } }] });
    // 시리즈가 하나면 "모든 줄에서 같다" 는 판정이 성립하지 않는다 — 유일한 식별 정보다.
    expect(matrixToEntries(flat, cfg).seriesNames[0]).toContain('location');
  });
});
