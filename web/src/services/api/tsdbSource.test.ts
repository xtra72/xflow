// tsdbSource.ts 단위 테스트 — 백엔드 파생 · 부분 실패 격리 · 취소 전파.
//
// 네트워크는 `./client` 모킹으로 대체한다. store.ts · influxdbManagement.ts 가
// 같은 모듈을 import 하므로 한 번의 모킹으로 전 경로가 덮인다.
//
// @spec SPEC-TSDB-002 §2.18 (U11) · §2.19 (U12) · §2.6 (U6)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { createElement, type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());
const delMock = vi.hoisted(() => vi.fn());
const delWithMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: putMock,
  del: delMock,
  delWith: delWithMock,
}));

import type { TsdbSourceConfig } from '@/pages/dashboard/panels/charts/chartChannelTypes';

import type { SeriesMatrixQuery } from './seriesDataSource';
import {
  queryTsdbMatrix,
  queryTsdbSourceMatrix,
  resolveTsdbBackend,
  tsdbSeriesDataSourceFor,
  TsdbBackendMismatchError,
  type TsdbAgentRef,
} from './tsdbSource';

/** 평탄 `chartQueryResponse` 형상의 응답을 만든다(F4). */
function seriesResponse(
  field: string,
  tags: Record<string, string>,
  points: Array<[number, number]>,
) {
  const entries = points.map(([timestamp, value]) => ({
    timestamp,
    value,
    labels: { __field__: field, ...tags },
  }));
  return { entries, count: entries.length, truncated: false };
}

/** 3시리즈 조회 파라미터(F1 축). */
function threeSeriesQuery(): SeriesMatrixQuery {
  return {
    keys: ['room.temp', 'room.humid', 'room.co2'],
    seriesFilters: [
      { fieldName: 'value' },
      { fieldName: 'value' },
      { fieldName: 'value' },
    ],
    startMs: 1_700_000_000_000,
    endMs: 1_700_000_300_000,
    intervalMs: 60_000,
    aggregation: 'average',
  };
}

const INFLUX_AGENT: TsdbAgentRef = { id: 'a-1', name: 'ix', type: 'influxdb' };
const STORE_AGENT: TsdbAgentRef = { id: 'a-2', name: 'st', type: 'store' };

beforeEach(() => {
  vi.clearAllMocks();
});

// ---- AC-50: 백엔드는 에이전트 타입에서 파생한다 ----

describe('백엔드 파생 (AC-50 · §2.18)', () => {
  const params: SeriesMatrixQuery = {
    keys: ['cpu'],
    seriesFilters: [{ fieldName: 'usage', tags: { host: 'a' } }],
    startMs: 1_700_000_000_000,
    endMs: 1_700_003_600_000,
    intervalMs: 60_000,
    aggregation: 'average',
  };

  it('M-OK: 에이전트 타입 influxdb → InfluxDB 어댑터', async () => {
    postMock.mockResolvedValue(
      seriesResponse('usage', { host: 'a' }, [[1_700_000_000_000, 21.5]]),
    );

    const result = await queryTsdbSourceMatrix(
      { agent_id: 'a-1', agent_name: 'ix' },
      [INFLUX_AGENT],
      params,
    );

    expect(postMock).toHaveBeenCalledTimes(1);
    expect(postMock.mock.calls[0]![0]).toBe('/influxdb/ix/series/query');
    expect(postMock.mock.calls[0]![1]).toMatchObject({
      measurement: 'cpu',
      field: 'usage',
      tags: { host: 'a' },
      start_ms: 1_700_000_000_000,
      end_ms: 1_700_003_600_000,
      interval_ms: 60_000,
      aggregation: 'average',
    });
    expect(result.matrix.columns).toEqual(['cpu']);
    expect(result.failures).toEqual([]);
  });

  it('M-UNSUPPORTED: 에이전트 타입 store → 백엔드 불일치 오류, 질의 없음', async () => {
    await expect(
      queryTsdbSourceMatrix({ agent_id: 'a-2', agent_name: 'st' }, [STORE_AGENT], params),
    ).rejects.toBeInstanceOf(TsdbBackendMismatchError);
    expect(postMock).not.toHaveBeenCalled();
  });

  it('M-MISSING: agent_id 해석 실패 → agent_name 폴백', async () => {
    postMock.mockResolvedValue(seriesResponse('usage', {}, [[1_700_000_000_000, 1]]));

    // 저장된 agent_id 는 목록에 없고, 저장된 이름만 현재 목록과 일치한다(UB2-3).
    const resolution = resolveTsdbBackend({ agent_id: 'gone', agent_name: 'ix' }, [
      INFLUX_AGENT,
    ]);
    expect(resolution).toMatchObject({ status: 'ok', agentName: 'ix' });

    await queryTsdbSourceMatrix({ agent_id: 'gone', agent_name: 'ix' }, [INFLUX_AGENT], params);
    expect(postMock.mock.calls[0]![0]).toBe('/influxdb/ix/series/query');
  });

  it('에이전트를 전혀 해석하지 못하면 저장된 이름으로 질의한다(404 는 오류 오버레이로 드러난다)', async () => {
    postMock.mockResolvedValue(seriesResponse('usage', {}, []));

    const resolution = resolveTsdbBackend({ agent_name: 'ghost' }, [INFLUX_AGENT]);
    expect(resolution).toMatchObject({ status: 'fallback', agentName: 'ghost' });

    await queryTsdbSourceMatrix({ agent_name: 'ghost' }, [INFLUX_AGENT], params);
    expect(postMock.mock.calls[0]![0]).toBe('/influxdb/ghost/series/query');
  });

  it('config.backend 와 에이전트 타입이 어긋나면 에이전트 타입이 이긴다', async () => {
    postMock.mockResolvedValue(seriesResponse('usage', {}, [[1_700_000_000_000, 3]]));

    // config 는 존재하지 않는 백엔드를 기록하고 있으나, 참조된 에이전트는 influxdb 다.
    // 라우팅은 에이전트 타입만 본다 — config 의 기록값은 읽지 않는다(UB1-22).
    const config = {
      backend: 'prometheus',
      agent_id: 'a-1',
      agent_name: 'ix',
      series: [],
      time_window_ms: 3_600_000,
      interval_ms: 60_000,
      aggregation: 'average',
    } as unknown as TsdbSourceConfig;

    await queryTsdbSourceMatrix(config, [INFLUX_AGENT], params);
    expect(postMock.mock.calls[0]![0]).toBe('/influxdb/ix/series/query');

    // 반대 방향 — config 가 influxdb 라고 기록해도 에이전트가 store 면 질의하지 않는다.
    postMock.mockClear();
    const lying = { ...config, backend: 'influxdb', agent_id: 'a-2', agent_name: 'st' } as TsdbSourceConfig;
    await expect(
      queryTsdbSourceMatrix(lying, [STORE_AGENT], params),
    ).rejects.toBeInstanceOf(TsdbBackendMismatchError);
    expect(postMock).not.toHaveBeenCalled();
  });
});

// ---- AC-18 · AC-52: 컬럼 자리 보존과 부분 실패 격리 ----

describe('queryTsdbMatrix — 컬럼 구성 (AC-18)', () => {
  it('3개 시리즈 중 1개가 0행이어도 컬럼 3개가 유지된다', async () => {
    postMock
      .mockResolvedValueOnce(
        seriesResponse('value', {}, [
          [1_700_000_000_000, 20],
          [1_700_000_060_000, 22],
        ]),
      )
      .mockResolvedValueOnce(seriesResponse('value', {}, [[1_700_000_000_000, 40]]))
      .mockResolvedValueOnce(seriesResponse('value', {}, []));

    const result = await queryTsdbMatrix('ix', threeSeriesQuery());

    expect(result.matrix.columns).toHaveLength(3);
    // 0행 시리즈도 자리를 지키며 전 버킷 null 이다(UB1-18).
    expect(result.matrix.rows.map((r) => r.values[2])).toEqual([null, null]);
    expect(result.failures).toEqual([]);
  });

  it('컬럼 순서가 요청 시리즈 순서와 같다', async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, [[1_700_000_000_000, 1]]));

    const result = await queryTsdbMatrix('ix', threeSeriesQuery());
    expect(result.matrix.columns).toEqual(['room.temp', 'room.humid', 'room.co2']);
  });
});

describe('queryTsdbMatrix — 부분 실패 격리와 취소 (AC-52 · §2.19)', () => {
  it('1개 시리즈 실패 시 나머지 컬럼은 보존되고 실패 인덱스를 보고한다', async () => {
    postMock
      .mockResolvedValueOnce(
        seriesResponse('value', {}, [
          [1_700_000_000_000, 20],
          [1_700_000_060_000, 22],
        ]),
      )
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce(seriesResponse('value', {}, [[1_700_000_060_000, 5]]));

    const result = await queryTsdbMatrix('ix', threeSeriesQuery());

    expect(result.failures.map((f) => f.index)).toEqual([1]);
    expect(result.matrix.columns).toEqual(['room.temp', 'room.humid', 'room.co2']);
    // 실패한 시리즈만 전 버킷 null, 형제 컬럼은 정상 렌더된다.
    expect(result.matrix.rows.map((r) => r.values[1])).toEqual([null, null]);
    expect(result.matrix.rows.map((r) => r.values[0])).toEqual([20, 22]);
    expect(result.matrix.rows.map((r) => r.values[2])).toEqual([null, 5]);
  });

  it('모든 시리즈가 실패하면 부분 실패가 아니라 전체 실패로 던진다', async () => {
    postMock.mockRejectedValue(new Error('down'));
    await expect(queryTsdbMatrix('ix', threeSeriesQuery())).rejects.toThrow('down');
  });

  it('AbortSignal 로 취소된 요청의 응답은 폐기된다', async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, [[1_700_000_000_000, 1]]));
    const controller = new AbortController();

    const pending = queryTsdbMatrix('ix', threeSeriesQuery(), controller.signal);
    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    // 각 요청에 같은 signal 이 전달된다(UB1-5).
    for (const call of postMock.mock.calls) {
      expect(call[2]).toMatchObject({ signal: controller.signal });
    }
  });
});

describe('queryTsdbMatrix — 요청 본문 규약 (§2.6 · §2.7)', () => {
  it('field 가 비어 있으면 그 시리즈만 실패한다(첫 숫자 필드 폴백 금지 — UB1-4)', async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, [[1_700_000_000_000, 1]]));

    const params: SeriesMatrixQuery = {
      keys: ['a', 'b'],
      seriesFilters: [undefined, { fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    };
    const result = await queryTsdbMatrix('ix', params);

    expect(result.failures.map((f) => f.index)).toEqual([0]);
    // field 없는 시리즈는 요청 자체가 나가지 않는다.
    expect(postMock).toHaveBeenCalledTimes(1);
  });

  it("fill:'avg' 를 다른 전략으로 조용히 대체하지 않고 그대로 보낸다(UB1-17)", async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, []));

    await queryTsdbMatrix('ix', {
      keys: ['a'],
      seriesFilters: [{ fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
      fill: 'avg',
    });

    expect(postMock.mock.calls[0]![1]).toMatchObject({ fill: 'avg' });
  });

  it('bucket 은 지정된 경우에만 본문에 포함된다', async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, []));

    await queryTsdbMatrix('ix', {
      keys: ['a'],
      seriesFilters: [{ fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
      bucket: 'metrics',
    });
    expect(postMock.mock.calls[0]![1]).toMatchObject({ bucket: 'metrics' });

    postMock.mockClear();
    await queryTsdbMatrix('ix', {
      keys: ['a'],
      seriesFilters: [{ fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    });
    expect(postMock.mock.calls[0]![1]).not.toHaveProperty('bucket');
  });
});

// ---- §2.6: measurement(key) 필수 ----

describe('fetchTsdbSeries — measurement(key) 필수 (§2.6)', () => {
  it('key 가 비어 있으면 그 시리즈 요청이 나가지 않고 실패한다', async () => {
    const params: SeriesMatrixQuery = {
      keys: [''],
      seriesFilters: [{ fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    };

    // 단일 시리즈가 전부 실패한 것이므로 부분 실패가 아니라 전체 실패다(§2.14).
    await expect(queryTsdbMatrix('ix', params)).rejects.toThrow('measurement(key)');
    expect(postMock).not.toHaveBeenCalled();
  });

  it('key 가 빈 시리즈만 실패하고 형제 시리즈는 정상 조회된다', async () => {
    postMock.mockResolvedValue(seriesResponse('value', {}, [[1_700_000_000_000, 7]]));

    const result = await queryTsdbMatrix('ix', {
      keys: ['', 'ok'],
      seriesFilters: [{ fieldName: 'value' }, { fieldName: 'value' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    });

    expect(result.failures.map((f) => f.index)).toEqual([0]);
    expect(result.matrix.columns).toEqual(['', 'ok']);
    expect(postMock).toHaveBeenCalledTimes(1);
  });
});

// ---- M6.6: 통합 데이터 소스 어댑터 ----

describe('tsdbSeriesDataSourceFor — 어댑터 형상 (M6.6 · §1.2.1)', () => {
  it("kind 는 'tsdb' 다 — 프로세스 내 저장소('memtsdb')와 구분된다(UB1-21)", () => {
    expect(tsdbSeriesDataSourceFor(INFLUX_AGENT).kind).toBe('tsdb');
  });

  it('queryMatrix 는 매트릭스만 돌려준다(부분 실패 신호는 훅 계층 소관)', async () => {
    postMock.mockResolvedValue(
      seriesResponse('usage', {}, [[1_700_000_000_000, 12]]),
    );

    const source = tsdbSeriesDataSourceFor(INFLUX_AGENT);
    const matrix = await source.queryMatrix({
      keys: ['cpu'],
      seriesFilters: [{ fieldName: 'usage' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    });

    expect(matrix.columns).toEqual(['cpu']);
    expect(matrix.rows.map((r) => r.values[0])).toEqual([12]);
    expect(postMock.mock.calls[0]![0]).toBe('/influxdb/ix/series/query');
    expect(matrix).not.toHaveProperty('failures');
  });

  it('options.bucket 은 요청 본문에 실리고, 없으면 실리지 않는다', async () => {
    postMock.mockResolvedValue(seriesResponse('usage', {}, []));

    const query: SeriesMatrixQuery = {
      keys: ['cpu'],
      seriesFilters: [{ fieldName: 'usage' }],
      startMs: 1_700_000_000_000,
      endMs: 1_700_000_300_000,
      intervalMs: 60_000,
      aggregation: 'average',
    };

    await tsdbSeriesDataSourceFor(INFLUX_AGENT, { bucket: 'metrics' }).queryMatrix(query);
    expect(postMock.mock.calls[0]![1]).toMatchObject({ bucket: 'metrics' });

    postMock.mockClear();
    await tsdbSeriesDataSourceFor(INFLUX_AGENT).queryMatrix(query);
    expect(postMock.mock.calls[0]![1]).not.toHaveProperty('bucket');
  });

  it('참조된 에이전트가 지원 백엔드가 아니면 질의하지 않는다(§2.17-7)', async () => {
    const source = tsdbSeriesDataSourceFor(STORE_AGENT);
    await expect(
      source.queryMatrix({
        keys: ['cpu'],
        seriesFilters: [{ fieldName: 'usage' }],
        startMs: 1_700_000_000_000,
        endMs: 1_700_000_300_000,
        intervalMs: 60_000,
        aggregation: 'average',
      }),
    ).rejects.toBeInstanceOf(TsdbBackendMismatchError);
    expect(postMock).not.toHaveBeenCalled();
  });
});

// ---- D1 · §2.10: useKeys 는 measurement 목록을 돌려준다 ----

/** react-query 훅을 감쌀 provider. 재시도를 끄고 캐시를 테스트마다 새로 만든다. */
function queryWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return wrapper;
}

describe('tsdbSeriesDataSourceFor.useKeys — measurement 디스커버리 (D1 · §2.10)', () => {
  it('measurement 목록을 페이지로 잘라 돌려준다', async () => {
    getMock.mockResolvedValue({ measurements: ['cpu', 'mem', 'disk'], count: 3 });

    const source = tsdbSeriesDataSourceFor(INFLUX_AGENT, { bucket: 'metrics' });
    const { result } = renderHook(() => source.useKeys({ page: 1, size: 2 }), {
      wrapper: queryWrapper(),
    });

    // 첫 렌더는 로딩이며 데이터가 없다.
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();

    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.keys).toEqual(['cpu', 'mem']);
    expect(result.current.data?.pagination).toMatchObject({
      page: 1,
      size: 2,
      total: 3,
      totalPages: 2,
    });
    expect(result.current.isError).toBe(false);
    expect(getMock.mock.calls[0]![0]).toContain('/influxdb/ix/measurements');
    expect(getMock.mock.calls[0]![0]).toContain('bucket=metrics');
  });

  it('bucket 이 없으면 빈 bucket 으로 질의한다(에이전트 기본값 위임)', async () => {
    getMock.mockResolvedValue({ measurements: ['cpu'], count: 1 });

    const source = tsdbSeriesDataSourceFor(INFLUX_AGENT);
    const { result } = renderHook(() => source.useKeys({ page: 1, size: 10 }), {
      wrapper: queryWrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(getMock.mock.calls[0]![0]).toContain('bucket=');
  });

  it('조회가 실패하면 isError 와 error 로 드러난다', async () => {
    getMock.mockRejectedValue(new Error('discovery down'));

    const source = tsdbSeriesDataSourceFor(INFLUX_AGENT);
    const { result } = renderHook(() => source.useKeys({ page: 1, size: 10 }), {
      wrapper: queryWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.data).toBeUndefined();
  });

  it('refetch 는 디스커버리를 다시 읽는다(캐시하지 않는다 — UB1-12)', async () => {
    getMock.mockResolvedValue({ measurements: ['cpu'], count: 1 });

    const source = tsdbSeriesDataSourceFor(INFLUX_AGENT);
    const { result } = renderHook(() => source.useKeys({ page: 1, size: 10 }), {
      wrapper: queryWrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    const before = getMock.mock.calls.length;

    getMock.mockResolvedValue({ measurements: ['cpu', 'mem'], count: 2 });
    act(() => {
      result.current.refetch();
    });

    await waitFor(() => expect(result.current.data?.keys).toEqual(['cpu', 'mem']));
    expect(getMock.mock.calls.length).toBeGreaterThan(before);
  });

  it('에이전트 이름이 비면 질의하지 않는다(enabled 게이팅)', () => {
    const source = tsdbSeriesDataSourceFor({ id: 'a-0', name: '', type: 'influxdb' });
    const { result } = renderHook(() => source.useKeys({ page: 1, size: 10 }), {
      wrapper: queryWrapper(),
    });

    expect(getMock).not.toHaveBeenCalled();
    expect(result.current.data).toBeUndefined();
  });
});
