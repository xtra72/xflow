// store.ts 단위 테스트 — 클라이언트 사이드 페이지네이션, 버킷화, 집계, 매트릭스 병합.
//
// v0.3.0 Wave 2: 서버 집계 우선 경로 + 4xx 폴백 경로를 커버한다.
//   - 기본 경로: `interval_ms` + `aggregation` 을 포함한 요청, 서버가 반환한
//     버킷 단위 엔트리를 그대로 사용.
//   - 폴백 경로: 서버가 4xx 로 거부하면 `interval_ms`/`aggregation` 을 뺀 재요청
//     후 `bucketAndAggregate` 로 클라이언트 집계.
//
// @spec SPEC-WEB-005

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());
const delWithMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: putMock,
  delWith: delWithMock,
}));

import { APIError } from '@/types/api';

import {
  bucketAndAggregate,
  storeChartValue,
  fetchStoreKeyObjects,
  fetchStoreKeys,
  fetchStoreKeysWithTags,
  fetchStoreTagPairs,
  queryStoreMatrix,
  resetAllStoreKeys,
  resetStoreKey,
  setStoreKeyMeta,
  sliceKeysPage,
} from './store';

// ---- sliceKeysPage ----

describe('sliceKeysPage', () => {
  it('전체 목록보다 큰 페이지 번호는 빈 배열 반환', () => {
    const page = sliceKeysPage(['a', 'b', 'c'], 5, 10);
    expect(page.keys).toEqual([]);
    expect(page.pagination.total).toBe(3);
    expect(page.pagination.totalPages).toBe(1);
  });

  it('정확히 나누어 떨어지는 페이지 계산', () => {
    const keys = ['a', 'b', 'c', 'd'];
    const p1 = sliceKeysPage(keys, 1, 2);
    expect(p1.keys).toEqual(['a', 'b']);
    expect(p1.pagination).toEqual({ page: 1, size: 2, total: 4, totalPages: 2 });
    const p2 = sliceKeysPage(keys, 2, 2);
    expect(p2.keys).toEqual(['c', 'd']);
  });

  it('마지막 페이지가 짧아도 정상 슬라이스', () => {
    const keys = ['a', 'b', 'c', 'd', 'e'];
    const p3 = sliceKeysPage(keys, 3, 2);
    expect(p3.keys).toEqual(['e']);
    expect(p3.pagination.totalPages).toBe(3);
  });

  it('빈 목록 처리', () => {
    const page = sliceKeysPage([], 1, 10);
    expect(page.keys).toEqual([]);
    expect(page.pagination).toEqual({ page: 1, size: 10, total: 0, totalPages: 0 });
  });

  it('page/size 가 0 또는 음수인 경우 최소값 1 로 보정', () => {
    const page = sliceKeysPage(['a', 'b'], 0, 0);
    expect(page.pagination.page).toBe(1);
    expect(page.pagination.size).toBe(1);
  });
});

// ---- bucketAndAggregate ----

describe('storeChartValue (데이터 타입 변환)', () => {
  it('number(int/float)는 그대로', () => {
    expect(storeChartValue(42)).toBe(42);
    expect(storeChartValue(3.14)).toBeCloseTo(3.14);
  });
  it('boolean 은 1/0 으로', () => {
    expect(storeChartValue(true)).toBe(1);
    expect(storeChartValue(false)).toBe(0);
  });
  it('string 등은 제외(null)', () => {
    expect(storeChartValue('3.14')).toBeNull();
    expect(storeChartValue('cool')).toBeNull();
    expect(storeChartValue(null)).toBeNull();
    expect(storeChartValue(Number.POSITIVE_INFINITY)).toBeNull();
  });
});

describe('bucketAndAggregate', () => {
  // epoch-zero 정렬: 버킷 경계는 0, 3000, 6000, 9000, ... (intervalMs=3000 기준)
  // 범위 필터 [startMs, endMs) 는 그대로 적용된다.
  const startMs = 1_000;
  const endMs = 10_000;
  const intervalMs = 3_000;

  it('boolean 값은 1/0 으로 변환해 집계하고, string 은 제외한다', () => {
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: true },
        { timestamp: 2_000, value: false },
        { timestamp: 2_500, value: '문자열' as unknown as number },
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    // true(1)+false(0) 평균 = 0.5, 문자열은 제외.
    expect(m.get(0)).toBe(0.5);
  });

  it('단일 버킷 내 값들을 평균으로 집계', () => {
    // 모든 엔트리가 [0, 3000) 버킷에 들어가도록 배치.
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: 10 },
        { timestamp: 2_000, value: 20 },
        { timestamp: 2_500, value: 30 },
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(1);
    expect(m.get(0)).toBe(20); // (10+20+30)/3
  });

  it('min/max 집계', () => {
    const entries = [
      { timestamp: 1_500, value: 10 },
      { timestamp: 2_000, value: 30 },
      { timestamp: 2_500, value: 20 },
    ];
    expect(bucketAndAggregate(entries, startMs, endMs, intervalMs, 'min').get(0)).toBe(10);
    expect(bucketAndAggregate(entries, startMs, endMs, intervalMs, 'max').get(0)).toBe(30);
  });

  it('여러 버킷으로 분산되는 엔트리', () => {
    // 1500 → bucket 0, 4500 → bucket 3000, 7500 → bucket 6000.
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: 1 },
        { timestamp: 4_500, value: 2 },
        { timestamp: 7_500, value: 3 },
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(3);
    expect(m.get(0)).toBe(1);
    expect(m.get(3_000)).toBe(2);
    expect(m.get(6_000)).toBe(3);
  });

  it('범위 밖(start 이전, end 이상) 엔트리는 제외', () => {
    const m = bucketAndAggregate(
      [
        { timestamp: 500, value: 100 }, // 범위 밖 (t < startMs)
        { timestamp: 1_500, value: 1 }, // OK → bucket 0
        { timestamp: 10_000, value: 100 }, // endMs 에 걸침 → 제외 (exclusive)
        { timestamp: 20_000, value: 100 }, // 범위 밖
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(1);
    expect(m.get(0)).toBe(1);
  });

  it('비숫자 값은 스킵 (해당 버킷에서 제외)', () => {
    // 엔트리 3000 → bucket 3000 (epoch-zero 정렬).
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: 'not a number' },
        { timestamp: 2_000, value: null },
        { timestamp: 2_500, value: { nested: 1 } },
        { timestamp: 3_000, value: 42 }, // 유일하게 유효
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(1);
    expect(m.get(3_000)).toBe(42);
  });

  it('버킷 내 유효한 값이 전혀 없으면 해당 버킷은 결과에서 제외', () => {
    const m = bucketAndAggregate(
      [{ timestamp: 1_500, value: 'skip' }],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(0);
  });

  it('Infinity/NaN 값은 스킵', () => {
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: Number.NaN },
        { timestamp: 2_000, value: Number.POSITIVE_INFINITY },
        { timestamp: 2_500, value: 5 },
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.get(0)).toBe(5);
  });

  it('버킷 경계값은 다음 버킷으로 분류', () => {
    // intervalMs=3000 → 경계 3000 은 두 번째 버킷(3000~6000) 시작 (epoch-zero 정렬).
    const m = bucketAndAggregate(
      [{ timestamp: 4_000, value: 7 }],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.get(3_000)).toBe(7);
    expect(m.has(0)).toBe(false);
  });

  // ---------------------------------------------------------------------------
  // 벽시계 정렬 (epoch-zero alignment) 테스트.
  //
  // 사용자 startMs 가 인터벌 경계와 어긋나도, 버킷은 항상 epoch 0 기준
  // 벽시계 경계에 정렬되어야 한다.
  // ---------------------------------------------------------------------------

  it('1m 인터벌은 초 단위가 0인 버킷으로 정렬된다', () => {
    // start=14:23:45 (인터벌 경계와 어긋남), 인터벌=1m.
    // 14:23:45 → bucket 14:23:00 (초=0), 14:24:30 → bucket 14:24:00.
    const startMs1m = Date.UTC(2026, 3, 26, 14, 23, 45);
    const endMs1m = Date.UTC(2026, 3, 26, 14, 30, 0);
    const entries = [
      { timestamp: Date.UTC(2026, 3, 26, 14, 23, 45), value: 10 },
      { timestamp: Date.UTC(2026, 3, 26, 14, 24, 30), value: 20 },
    ];
    const result = bucketAndAggregate(entries, startMs1m, endMs1m, 60_000, 'average');
    const keys = Array.from(result.keys()).sort((a, b) => a - b);
    expect(keys).toEqual([
      Date.UTC(2026, 3, 26, 14, 23, 0),
      Date.UTC(2026, 3, 26, 14, 24, 0),
    ]);
    expect(result.get(Date.UTC(2026, 3, 26, 14, 23, 0))).toBe(10);
    expect(result.get(Date.UTC(2026, 3, 26, 14, 24, 0))).toBe(20);
  });

  it('5m 인터벌은 분이 5의 배수, 초가 0인 버킷으로 정렬된다', () => {
    // 14:23:45 → bucket 14:20:00 (분 mod 5 = 0).
    const startMs5m = Date.UTC(2026, 3, 26, 14, 23, 45);
    const endMs5m = Date.UTC(2026, 3, 26, 14, 30, 0);
    const entries = [{ timestamp: Date.UTC(2026, 3, 26, 14, 23, 45), value: 10 }];
    const result = bucketAndAggregate(entries, startMs5m, endMs5m, 5 * 60_000, 'average');
    const keys = Array.from(result.keys());
    expect(keys).toEqual([Date.UTC(2026, 3, 26, 14, 20, 0)]);
    expect(result.get(Date.UTC(2026, 3, 26, 14, 20, 0))).toBe(10);
  });

  it('1h 인터벌은 분과 초가 모두 0인 버킷으로 정렬된다', () => {
    // 14:23:45 → bucket 14:00:00 (분=0, 초=0).
    const startMs1h = Date.UTC(2026, 3, 26, 14, 23, 45);
    const endMs1h = Date.UTC(2026, 3, 26, 16, 0, 0);
    const entries = [{ timestamp: Date.UTC(2026, 3, 26, 14, 23, 45), value: 10 }];
    const result = bucketAndAggregate(entries, startMs1h, endMs1h, 3600_000, 'average');
    const keys = Array.from(result.keys());
    expect(keys).toEqual([Date.UTC(2026, 3, 26, 14, 0, 0)]);
    expect(result.get(Date.UTC(2026, 3, 26, 14, 0, 0))).toBe(10);
  });
});

// ---- queryStoreMatrix ----

describe('queryStoreMatrix', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('키가 없으면 백엔드 호출 없이 빈 매트릭스 반환', async () => {
    const m = await queryStoreMatrix('agent', {
      keys: [],
      startMs: 0,
      endMs: 1,
      intervalMs: 1,
      aggregation: 'average',
    });
    expect(postMock).not.toHaveBeenCalled();
    expect(m.columns).toEqual([]);
    expect(m.rows).toEqual([]);
  });

  it('end <= start 이면 에러를 던진다', async () => {
    await expect(
      queryStoreMatrix('a', {
        keys: ['k'],
        startMs: 100,
        endMs: 100,
        intervalMs: 10,
        aggregation: 'average',
      }),
    ).rejects.toThrow(/종료 시각/);
  });

  it('intervalMs 가 0 이하이면 에러를 던진다', async () => {
    await expect(
      queryStoreMatrix('a', {
        keys: ['k'],
        startMs: 0,
        endMs: 100,
        intervalMs: 0,
        aggregation: 'average',
      }),
    ).rejects.toThrow(/인터벌/);
  });

  it('단일 키: time_range 모드 + 서버 집계 (interval_ms + aggregation=avg) 요청', async () => {
    // 서버가 이미 버킷 단위로 집계된 엔트리를 반환한다.
    // aggregation='avg' 로 변환되어 서버가 (10+20)/2=15, 30 을 미리 계산해 내려준다.
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 1_000, value: 15 },
        { timestamp: 4_000, value: 30 },
      ],
      count: 2,
    });

    const m = await queryStoreMatrix('tsdb', {
      keys: ['indoor:1:room_temp'],
      startMs: 1_000,
      endMs: 7_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(postMock).toHaveBeenCalledTimes(1);
    const [url, body] = postMock.mock.calls[0]!;
    expect(url).toBe('/store/tsdb/query');
    expect(body).toEqual({
      key: 'indoor:1:room_temp',
      mode: 'time_range',
      start_ms: 1_000,
      end_ms: 7_000,
      namespace: 'default',
      interval_ms: 3_000,
      aggregation: 'avg',
    });

    expect(m.columns).toEqual(['indoor:1:room_temp']);
    expect(m.rows).toEqual([
      { bucketStartMs: 1_000, values: [15] },
      { bucketStartMs: 4_000, values: [30] },
    ]);
  });

  it('여러 키: 서버가 반환한 버킷 단위 엔트리를 매트릭스로 병합, 누락 버킷은 null', async () => {
    // 각 키가 서로 다른 버킷에서만 값을 가진다. 서버가 빈 버킷은 생략하므로
    // 프론트엔드에서 timestamp align 로직이 null 을 채워야 한다.
    postMock.mockImplementation(async (_url: string, body: unknown) => {
      const req = body as { key: string };
      if (req.key === 'A') {
        return { entries: [{ timestamp: 1_000, value: 10 }] };
      }
      return { entries: [{ timestamp: 4_000, value: 20 }] };
    });

    const m = await queryStoreMatrix('tsdb', {
      keys: ['A', 'B'],
      startMs: 1_000,
      endMs: 7_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(m.columns).toEqual(['A', 'B']);
    expect(m.rows).toEqual([
      { bucketStartMs: 1_000, values: [10, null] },
      { bucketStartMs: 4_000, values: [null, 20] },
    ]);
  });

  it('엔트리가 비어있는 키는 해당 컬럼 전체가 null', async () => {
    postMock.mockImplementationOnce(async () => ({ entries: [] }));

    const m = await queryStoreMatrix('tsdb', {
      keys: ['empty'],
      startMs: 0,
      endMs: 10_000,
      intervalMs: 1_000,
      aggregation: 'average',
    });
    expect(m.columns).toEqual(['empty']);
    expect(m.rows).toEqual([]);
  });

  it('네트워크 에러는 호출자에게 전파', async () => {
    postMock.mockRejectedValueOnce(new Error('network down'));
    await expect(
      queryStoreMatrix('tsdb', {
        keys: ['k'],
        startMs: 0,
        endMs: 10,
        intervalMs: 1,
        aggregation: 'average',
      }),
    ).rejects.toThrow('network down');
  });

  it('AbortSignal 을 axios config 로 각 요청에 전달', async () => {
    postMock.mockResolvedValue({ entries: [] });
    const controller = new AbortController();
    await queryStoreMatrix(
      'tsdb',
      {
        keys: ['k1', 'k2'],
        startMs: 0,
        endMs: 10,
        intervalMs: 1,
        aggregation: 'average',
      },
      controller.signal,
    );
    expect(postMock).toHaveBeenCalledTimes(2);
    for (const call of postMock.mock.calls) {
      const cfg = call[2] as { signal?: AbortSignal } | undefined;
      expect(cfg?.signal).toBe(controller.signal);
    }
  });

  it('aggregation=min 을 서버로 전달하고 키별 독립 집계 결과를 반환', async () => {
    // 서버가 min 집계를 수행해 단일 버킷으로 결과를 내려준다.
    postMock.mockImplementation(async (_url, body) => {
      const req = body as { key: string; aggregation?: string };
      expect(req.aggregation).toBe('min');
      if (req.key === 'X') {
        return { entries: [{ timestamp: 0, value: 3 }] };
      }
      return { entries: [{ timestamp: 0, value: 8 }] };
    });

    const m = await queryStoreMatrix('tsdb', {
      keys: ['X', 'Y'],
      startMs: 0,
      endMs: 1_000,
      intervalMs: 1_000,
      aggregation: 'min',
    });
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [3, 8] }]);
  });
});

// ---- 서버 집계 ↔ 클라이언트 폴백 ----

describe('queryStoreMatrix: server aggregation and fallback', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('aggregation=average 는 요청 바디에서 백엔드 표기 avg 로 변환된다', async () => {
    postMock.mockResolvedValueOnce({ entries: [] });
    await queryStoreMatrix('agent', {
      keys: ['k'],
      startMs: 0,
      endMs: 60_000,
      intervalMs: 10_000,
      aggregation: 'average',
    });
    const [, body] = postMock.mock.calls[0]!;
    expect((body as { aggregation: string }).aggregation).toBe('avg');
    expect((body as { interval_ms: number }).interval_ms).toBe(10_000);
  });

  it('서버 4xx (aggregation 미지원) 시 interval_ms/aggregation 없이 재요청 후 클라이언트 집계', async () => {
    // 1차: 서버 집계 시도 → 400
    // 2차: 폴백 요청 → 원본 엔트리 반환 → 클라이언트가 (10+20)/2=15 계산
    // epoch-zero 정렬: 1500, 2500 모두 bucket 0 [0, 3000) 에 속한다.
    postMock.mockImplementationOnce(async () => {
      throw new APIError('UNSUPPORTED', 'aggregation not supported', 400);
    });
    postMock.mockImplementationOnce(async () => ({
      entries: [
        { timestamp: 1_500, value: 10 },
        { timestamp: 2_500, value: 20 },
      ],
    }));

    const m = await queryStoreMatrix('agent', {
      keys: ['k'],
      startMs: 1_000,
      endMs: 4_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(postMock).toHaveBeenCalledTimes(2);
    // 1차: 서버 집계 필드 포함.
    const firstBody = postMock.mock.calls[0]![1] as Record<string, unknown>;
    expect(firstBody.interval_ms).toBe(3_000);
    expect(firstBody.aggregation).toBe('avg');
    // 2차 폴백: 서버 집계 필드 제거.
    const secondBody = postMock.mock.calls[1]![1] as Record<string, unknown>;
    expect(secondBody).not.toHaveProperty('interval_ms');
    expect(secondBody).not.toHaveProperty('aggregation');
    // 결과는 클라이언트 집계로 (10+20)/2 = 15. 버킷 시작은 0 (epoch-zero 정렬).
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [15] }]);
  });

  it('서버 5xx 는 폴백하지 않고 에러를 그대로 전파', async () => {
    postMock.mockImplementationOnce(async () => {
      throw new APIError('SERVER_ERROR', 'internal error', 500);
    });
    await expect(
      queryStoreMatrix('agent', {
        keys: ['k'],
        startMs: 0,
        endMs: 10,
        intervalMs: 1,
        aggregation: 'average',
      }),
    ).rejects.toBeInstanceOf(APIError);
    expect(postMock).toHaveBeenCalledTimes(1);
  });

  it('비-APIError (네트워크 에러) 는 폴백하지 않고 그대로 전파', async () => {
    postMock.mockImplementationOnce(async () => {
      throw new Error('network down');
    });
    await expect(
      queryStoreMatrix('agent', {
        keys: ['k'],
        startMs: 0,
        endMs: 10,
        intervalMs: 1,
        aggregation: 'average',
      }),
    ).rejects.toThrow('network down');
    expect(postMock).toHaveBeenCalledTimes(1);
  });

  it('비숫자/NaN 서버 응답 엔트리는 스킵', async () => {
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 100, value: 'bad' },
        { timestamp: 200, value: Number.NaN },
        { timestamp: 300, value: 42 },
      ],
    });
    const m = await queryStoreMatrix('agent', {
      keys: ['k'],
      startMs: 0,
      endMs: 1_000,
      intervalMs: 100,
      aggregation: 'max',
    });
    expect(m.rows).toEqual([{ bucketStartMs: 300, values: [42] }]);
  });
});

// ---- 다중 시리즈 분리 (SPEC-STORE-004 M5) ----

describe('queryStoreMatrix: 다중 시리즈 분리 (labels)', () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it('한 key 의 metric/tags 별 다중 시리즈를 독립 컬럼으로 분리한다', async () => {
    // 백엔드가 같은 key 에 대해 2개 시리즈(서로 다른 room)를 평탄화해 반환.
    // 같은 버킷(1000)에 두 시리즈 값이 섞여 있어도 labels 로 분리되어야 한다.
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 1_000, value: 21, labels: { __field__: 'temp', room: '1' } },
        { timestamp: 1_000, value: 22, labels: { __field__: 'temp', room: '2' } },
        { timestamp: 4_000, value: 23, labels: { __field__: 'temp', room: '1' } },
      ],
    });

    const m = await queryStoreMatrix('store', {
      keys: ['sensor'],
      startMs: 1_000,
      endMs: 7_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    // 시리즈 2개 → 라벨이 덧붙은 2개 컬럼.
    expect(m.columns).toEqual([
      'sensor · temp{room=1}',
      'sensor · temp{room=2}',
    ]);
    expect(m.rows).toEqual([
      { bucketStartMs: 1_000, values: [21, 22] },
      { bucketStartMs: 4_000, values: [23, null] },
    ]);
  });

  it('한 key 에서 단일 시리즈면 라벨을 붙이지 않고 key 만 컬럼명으로 쓴다', async () => {
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 1_000, value: 5, labels: { __field__: 'temp', room: '1' } },
      ],
    });

    const m = await queryStoreMatrix('store', {
      keys: ['sensor'],
      startMs: 1_000,
      endMs: 7_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    // 단일 시리즈 → 기존 동작 보존(키만 표기).
    expect(m.columns).toEqual(['sensor']);
    expect(m.rows).toEqual([{ bucketStartMs: 1_000, values: [5] }]);
  });

  it('여러 key 가 각각 다중 시리즈를 가지면 key 순서 → 시리즈 순서로 평탄화', async () => {
    postMock.mockImplementation(async (_url, body) => {
      const req = body as { key: string };
      if (req.key === 'A') {
        return {
          entries: [
            { timestamp: 0, value: 1, labels: { __field__: 'm', t: 'x' } },
            { timestamp: 0, value: 2, labels: { __field__: 'm', t: 'y' } },
          ],
        };
      }
      // B 는 라벨 없는 단일 시리즈.
      return { entries: [{ timestamp: 0, value: 9 }] };
    });

    const m = await queryStoreMatrix('store', {
      keys: ['A', 'B'],
      startMs: 0,
      endMs: 1_000,
      intervalMs: 1_000,
      aggregation: 'average',
    });

    expect(m.columns).toEqual(['A · m{t=x}', 'A · m{t=y}', 'B']);
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [1, 2, 9] }]);
  });

  it('폴백(4xx) 경로에서도 labels 기준으로 시리즈를 분리해 클라이언트 집계', async () => {
    // 1차 서버 집계 시도 → 400, 2차 폴백 → 원본 엔트리(라벨 포함) 반환.
    // epoch-zero 정렬: 1500/2500 → bucket 0. 두 시리즈를 각각 평균낸다.
    postMock.mockImplementationOnce(async () => {
      throw new APIError('UNSUPPORTED', 'aggregation not supported', 400);
    });
    postMock.mockImplementationOnce(async () => ({
      entries: [
        { timestamp: 1_500, value: 10, labels: { __field__: 'm', s: 'a' } },
        { timestamp: 2_500, value: 20, labels: { __field__: 'm', s: 'a' } },
        { timestamp: 1_500, value: 100, labels: { __field__: 'm', s: 'b' } },
      ],
    }));

    const m = await queryStoreMatrix('store', {
      keys: ['k'],
      startMs: 1_000,
      endMs: 4_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(postMock).toHaveBeenCalledTimes(2);
    expect(m.columns).toEqual(['k · m{s=a}', 'k · m{s=b}']);
    // s=a: (10+20)/2 = 15, s=b: 100.
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [15, 100] }]);
  });
});

describe('queryStoreMatrix: seriesFilters 시리즈별 조회 (#2)', () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it('seriesFilters 로 한 key 의 응답을 지정한 시리즈로만 좁힌다', async () => {
    // 백엔드는 key 의 모든 시리즈를 평탄화해 반환하지만, filter 서명과
    // 일치하는 시리즈(temp/room=2)만 남아 단일 컬럼이 된다.
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 1_000, value: 21, labels: { __field__: 'temp', room: '1' } },
        { timestamp: 1_000, value: 22, labels: { __field__: 'temp', room: '2' } },
      ],
    });

    const m = await queryStoreMatrix('store', {
      keys: ['sensor'],
      seriesFilters: [{ fieldName: 'temp', tags: { room: '2' } }],
      startMs: 1_000,
      endMs: 4_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    // filter 일치 시리즈 1개. 같은 key 가 1회만 요청되었으므로 라벨 미부착.
    expect(m.columns).toEqual(['sensor']);
    expect(m.rows).toEqual([{ bucketStartMs: 1_000, values: [22] }]);
  });

  it('같은 key 를 두 시리즈 필터로 중복 요청하면 각각 분리된 라벨 컬럼이 된다', async () => {
    // 두 번 요청되므로 매번 같은 응답을 반환(각 호출에서 서로 다른 시리즈로 좁혀짐).
    postMock.mockResolvedValue({
      entries: [
        { timestamp: 0, value: 21, labels: { __field__: 'temp', room: '1' } },
        { timestamp: 0, value: 22, labels: { __field__: 'temp', room: '2' } },
      ],
    });

    const m = await queryStoreMatrix('store', {
      keys: ['sensor', 'sensor'],
      seriesFilters: [
        { fieldName: 'temp', tags: { room: '1' } },
        { fieldName: 'temp', tags: { room: '2' } },
      ],
      startMs: 0,
      endMs: 3_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    // 같은 key 가 2회 요청 → 라벨 강제 부착으로 컬럼명 충돌 방지.
    expect(m.columns).toEqual(['sensor · temp{room=1}', 'sensor · temp{room=2}']);
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [21, 22] }]);
  });

  it('filter 미지정(undefined)이면 모든 시리즈를 반환한다 (기존 동작 보존)', async () => {
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 0, value: 1, labels: { __field__: 'temp', room: '1' } },
        { timestamp: 0, value: 2, labels: { __field__: 'temp', room: '2' } },
      ],
    });

    const m = await queryStoreMatrix('store', {
      keys: ['sensor'],
      seriesFilters: [undefined],
      startMs: 0,
      endMs: 3_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(m.columns).toEqual(['sensor · temp{room=1}', 'sensor · temp{room=2}']);
  });
});

// ---- fetchStoreTagPairs / fetchStoreKeysWithTags (SPEC-STORE-003) ----

describe('fetchStoreTagPairs', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('pairs 가 있는 응답을 그대로 반환한다', async () => {
    getMock.mockResolvedValueOnce({
      pairs: [
        { key: 'room', values: ['1', '2', '3'] },
        { key: 'type', values: ['temperature'] },
      ],
    });
    const result = await fetchStoreTagPairs('agent-a');
    expect(getMock).toHaveBeenCalledWith('/store/agent-a/tags');
    expect(result).toEqual([
      { key: 'room', values: ['1', '2', '3'] },
      { key: 'type', values: ['temperature'] },
    ]);
  });

  it('pairs 필드가 없으면 빈 배열 반환', async () => {
    getMock.mockResolvedValueOnce({});
    expect(await fetchStoreTagPairs('agent-a')).toEqual([]);
  });

  it('에이전트 이름을 URL-인코딩한다', async () => {
    getMock.mockResolvedValueOnce({ pairs: [] });
    await fetchStoreTagPairs('agent with spaces');
    expect(getMock).toHaveBeenCalledWith('/store/agent%20with%20spaces/tags');
  });

  it('서버 4xx 는 호출자로 전파된다 (구버전 서버)', async () => {
    getMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not found', 404));
    await expect(fetchStoreTagPairs('agent-a')).rejects.toBeInstanceOf(APIError);
  });
});

// ---- resetStoreKey / resetAllStoreKeys (SPEC-STORE-003) ----

describe('resetStoreKey', () => {
  beforeEach(() => {
    delWithMock.mockReset();
  });

  it('정적 키 응답(history_cleared)을 그대로 반환한다', async () => {
    delWithMock.mockResolvedValueOnce({ action: 'history_cleared', key: 'k1' });
    const result = await resetStoreKey('agent-a', 'k1');
    expect(result).toEqual({ action: 'history_cleared', key: 'k1' });
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent-a/keys/k1?namespace=default',
    );
  });

  it('동적 키 응답(entry_deleted)을 그대로 반환한다', async () => {
    delWithMock.mockResolvedValueOnce({ action: 'entry_deleted', key: 'k2' });
    const result = await resetStoreKey('agent-a', 'k2');
    expect(result).toEqual({ action: 'entry_deleted', key: 'k2' });
  });

  it('namespace 미지정 시 default 로 폴백한다', async () => {
    delWithMock.mockResolvedValueOnce({ action: 'history_cleared', key: 'k1' });
    await resetStoreKey('agent-a', 'k1');
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent-a/keys/k1?namespace=default',
    );
  });

  it('namespace 를 명시하면 쿼리스트링에 반영된다', async () => {
    delWithMock.mockResolvedValueOnce({ action: 'entry_deleted', key: 'k1' });
    await resetStoreKey('agent-a', 'k1', 'prod');
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent-a/keys/k1?namespace=prod',
    );
  });

  it('에이전트 이름과 키를 URL-인코딩한다', async () => {
    delWithMock.mockResolvedValueOnce({ action: 'history_cleared', key: 'a/b' });
    await resetStoreKey('agent with spaces', 'a/b');
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent%20with%20spaces/keys/a%2Fb?namespace=default',
    );
  });

  it('서버 404 (키 없음) 는 호출자로 전파된다', async () => {
    delWithMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not found', 404));
    await expect(resetStoreKey('agent-a', 'missing')).rejects.toBeInstanceOf(
      APIError,
    );
  });
});

describe('resetAllStoreKeys', () => {
  beforeEach(() => {
    delWithMock.mockReset();
  });

  it('백엔드 카운트 응답을 그대로 반환한다', async () => {
    delWithMock.mockResolvedValueOnce({
      history_cleared: 3,
      entries_deleted: 5,
    });
    const result = await resetAllStoreKeys('agent-a');
    expect(result).toEqual({ history_cleared: 3, entries_deleted: 5 });
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default',
    );
  });

  it('namespace 를 명시하면 쿼리스트링에 반영된다', async () => {
    delWithMock.mockResolvedValueOnce({
      history_cleared: 0,
      entries_deleted: 0,
    });
    await resetAllStoreKeys('agent-a', 'prod');
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=prod',
    );
  });

  it('에이전트 이름을 URL-인코딩한다', async () => {
    delWithMock.mockResolvedValueOnce({
      history_cleared: 0,
      entries_deleted: 0,
    });
    await resetAllStoreKeys('agent with spaces');
    expect(delWithMock).toHaveBeenCalledWith(
      '/store/agent%20with%20spaces/keys?namespace=default',
    );
  });

  it('백엔드 에러는 호출자로 전파된다', async () => {
    delWithMock.mockRejectedValueOnce(
      new APIError('INTERNAL', 'internal error', 500),
    );
    await expect(resetAllStoreKeys('agent-a')).rejects.toBeInstanceOf(APIError);
  });
});

describe('fetchStoreKeysWithTags', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  // v0.7.0 (M11): 백엔드는 이제 객체 배열을 반환. fetchStoreKeysWithTags 는
  // 신규 응답을 받아 v0.6.0 호환 형상(`keys`, `tags`) 을 derived 하고,
  // 신규 `keyObjects` 필드도 함께 노출한다.
  // @spec SPEC-WEB-005 v0.7.0 (M11)

  it('객체 배열 응답을 받아 keys/tags/keyObjects 모두 반환한다', async () => {
    getMock.mockResolvedValueOnce({
      count: 2,
      keys: [
        {
          key: 'a',
          registration: 'manual',
          data_type: 'float',
          field: 'gauge',
          tags: { room: '1', type: 'temperature' },
        },
        {
          key: 'b',
          registration: 'manual',
          data_type: 'float',
          field: 'gauge',
          tags: { room: '2' },
        },
      ],
    });
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(getMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default&pattern=*',
    );
    expect(result.keys).toEqual(['a', 'b']);
    expect(result.tags).toEqual({
      a: { room: '1', type: 'temperature' },
      b: { room: '2' },
    });
    expect(result.keyObjects).toHaveLength(2);
    expect(result.keyObjects[0]?.data_type).toBe('float');
    expect(result.keyObjects[0]?.registration).toBe('manual');
    expect(result.keyObjects[0]?.field).toBe('gauge');
  });

  it('태그가 비어있는 키는 derived tags 맵에서 제외된다 (v0.6.0 호환)', async () => {
    // 정적 키지만 태그가 비어있는 케이스. 이전 v0.2.0 응답에서는 백엔드가
    // 해당 키를 top-level tags 맵에서 생략했으므로, derived 동작도 일치시킨다.
    getMock.mockResolvedValueOnce({
      count: 2,
      keys: [
        {
          key: 'a',
          registration: 'manual',
          data_type: 'string',
          field: 'unknown',
          tags: {},
        },
        {
          key: 'b',
          registration: 'auto',
          data_type: 'int',
          field: 'counter',
          tags: { source: 'runtime' },
        },
      ],
    });
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(result.keys).toEqual(['a', 'b']);
    // a 는 태그가 비어있어 derived tags 에서 제외, b 만 포함.
    expect(result.tags).toEqual({ b: { source: 'runtime' } });
    // keyObjects 는 양쪽 모두 보존.
    expect(result.keyObjects).toHaveLength(2);
    expect(result.keyObjects[1]?.registration).toBe('auto');
  });

  it('keys 필드가 생략된 응답은 모두 빈 값으로 폴백', async () => {
    getMock.mockResolvedValueOnce({});
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(result.keys).toEqual([]);
    expect(result.tags).toEqual({});
    expect(result.keyObjects).toEqual([]);
  });

  it('빈 keys 배열 응답', async () => {
    getMock.mockResolvedValueOnce({ count: 0, keys: [] });
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(result.keys).toEqual([]);
    expect(result.tags).toEqual({});
    expect(result.keyObjects).toEqual([]);
  });
});

// ---- fetchStoreKeys / fetchStoreKeyObjects (SPEC-WEB-005 v0.7.0 M11) ----

describe('fetchStoreKeys (v0.7.0 호환 레이어)', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('객체 배열 응답에서 key 이름만 추출해 string[] 으로 반환한다', async () => {
    // 백엔드는 v0.3.0 객체 배열을 반환하지만, fetchStoreKeys 는 시리즈 페이지네이션
    // 호환을 위해 string[] 형태를 유지한다.
    getMock.mockResolvedValueOnce({
      count: 3,
      keys: [
        {
          key: 'k1',
          registration: 'manual',
          data_type: 'float',
          field: 'gauge',
          tags: {},
        },
        {
          key: 'k2',
          registration: 'auto',
          data_type: 'int',
          field: 'counter',
          tags: { source: 'runtime' },
        },
        {
          key: 'k3',
          registration: 'manual',
          data_type: 'string',
          field: 'unknown',
          tags: { room: '1' },
        },
      ],
    });
    const result = await fetchStoreKeys('agent-a');
    expect(getMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default&pattern=*',
    );
    expect(result).toEqual(['k1', 'k2', 'k3']);
  });

  it('keys 필드가 생략되면 빈 배열 반환', async () => {
    getMock.mockResolvedValueOnce({});
    expect(await fetchStoreKeys('agent-a')).toEqual([]);
  });

  // SPEC-WEB-005 tag 자동 모드: tag_filters 를 ?tag=k:v AND 파라미터로 전송한다.
  it('tagFilters 를 주면 ?tag=k:v AND 필터를 쿼리에 붙여 조회한다', async () => {
    getMock.mockResolvedValueOnce({
      keys: [
        { key: 'room:1:temp', registration: 'auto', data_type: 'float', field: 'gauge', tags: { room: '1', type: 'temperature' } },
      ],
    });
    const result = await fetchStoreKeys('agent-a', { room: '1', type: 'temperature' });
    expect(getMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default&pattern=*&tag=room%3A1&tag=type%3Atemperature',
    );
    expect(result).toEqual(['room:1:temp']);
  });

  it('tagFilters 없이 호출하면 기존 URL 을 유지한다(하위 호환)', async () => {
    getMock.mockResolvedValueOnce({ keys: [] });
    await fetchStoreKeys('agent-a');
    expect(getMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default&pattern=*',
    );
  });

  it('signal 을 주면 AbortSignal 을 config 로 전달한다', async () => {
    getMock.mockResolvedValueOnce({ keys: [] });
    const controller = new AbortController();
    await fetchStoreKeys('agent-a', { room: '1' }, controller.signal);
    expect(getMock).toHaveBeenCalledWith(
      '/store/agent-a/keys?namespace=default&pattern=*&tag=room%3A1',
      { signal: controller.signal },
    );
  });
});

describe('fetchStoreKeyObjects (v0.7.0 신규 API)', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('백엔드 객체 배열을 그대로 반환한다', async () => {
    const objects = [
      {
        key: 'k1',
        registration: 'manual' as const,
        data_type: 'float' as const,
        field: 'gauge',
        tags: { room: '1' },
      },
      {
        key: 'k2',
        registration: 'auto' as const,
        data_type: 'int' as const,
        field: 'counter',
        tags: {},
      },
    ];
    getMock.mockResolvedValueOnce({ count: 2, keys: objects });
    const result = await fetchStoreKeyObjects('agent-a');
    expect(result).toEqual(objects);
    expect(result[0]?.data_type).toBe('float');
    expect(result[1]?.registration).toBe('auto');
  });

  it('keys 필드가 생략되면 빈 배열 반환', async () => {
    getMock.mockResolvedValueOnce({});
    expect(await fetchStoreKeyObjects('agent-a')).toEqual([]);
  });
});

// ---- setStoreKeyMeta (SPEC-STORE-003 v0.4.0) ----

describe('setStoreKeyMeta', () => {
  beforeEach(() => {
    putMock.mockReset();
  });

  it('field/tags 를 meta 엔드포인트로 PUT 한다', async () => {
    const resp = {
      key: 'outdoor:humidity',
      field: 'humidity',
      tags: { room: 'kitchen' },
    };
    putMock.mockResolvedValueOnce(resp);

    const result = await setStoreKeyMeta('agent-a', 'outdoor:humidity', {
      field: 'humidity',
      tags: { room: 'kitchen' },
    });

    // 키의 콜론은 encodeURIComponent 로 %3A 인코딩되어야 한다.
    expect(putMock).toHaveBeenCalledWith(
      '/store/agent-a/keys/outdoor%3Ahumidity/meta',
      { field: 'humidity', tags: { room: 'kitchen' } },
    );
    expect(result).toEqual(resp);
  });

  it('agent 이름과 키를 모두 URL 인코딩한다', async () => {
    putMock.mockResolvedValueOnce({ key: 'k', field: 'unknown', tags: {} });

    await setStoreKeyMeta('agent a/b', 'ns:key with space', { tags: {} });

    expect(putMock).toHaveBeenCalledWith(
      `/store/${encodeURIComponent('agent a/b')}/keys/${encodeURIComponent('ns:key with space')}/meta`,
      { tags: {} },
    );
  });

  it('field 생략 시 tags 만 전송한다 (백엔드가 unknown normalize)', async () => {
    putMock.mockResolvedValueOnce({ key: 'k', field: 'unknown', tags: { a: '1' } });

    await setStoreKeyMeta('agent-a', 'k', { tags: { a: '1' } });

    expect(putMock).toHaveBeenCalledWith('/store/agent-a/keys/k/meta', {
      tags: { a: '1' },
    });
  });
});
