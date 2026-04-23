// store.ts 단위 테스트 — 클라이언트 사이드 페이지네이션, 버킷화, 집계, 매트릭스 병합.
//
// @spec SPEC-WEB-005

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
}));

import {
  bucketAndAggregate,
  queryStoreMatrix,
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

describe('bucketAndAggregate', () => {
  const startMs = 1_000;
  const endMs = 10_000;
  const intervalMs = 3_000; // 3초 버킷: [1000, 4000), [4000, 7000), [7000, 10000)

  it('단일 버킷 내 값들을 평균으로 집계', () => {
    const m = bucketAndAggregate(
      [
        { timestamp: 1_500, value: 10 },
        { timestamp: 2_000, value: 20 },
        { timestamp: 3_500, value: 30 },
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(1);
    expect(m.get(1_000)).toBe(20); // (10+20+30)/3
  });

  it('min/max 집계', () => {
    const entries = [
      { timestamp: 1_500, value: 10 },
      { timestamp: 2_000, value: 30 },
      { timestamp: 3_500, value: 20 },
    ];
    expect(bucketAndAggregate(entries, startMs, endMs, intervalMs, 'min').get(1_000)).toBe(10);
    expect(bucketAndAggregate(entries, startMs, endMs, intervalMs, 'max').get(1_000)).toBe(30);
  });

  it('여러 버킷으로 분산되는 엔트리', () => {
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
    expect(m.get(1_000)).toBe(1);
    expect(m.get(4_000)).toBe(2);
    expect(m.get(7_000)).toBe(3);
  });

  it('범위 밖(start 이전, end 이상) 엔트리는 제외', () => {
    const m = bucketAndAggregate(
      [
        { timestamp: 500, value: 100 }, // 범위 밖
        { timestamp: 1_500, value: 1 }, // OK
        { timestamp: 10_000, value: 100 }, // endMs 에 걸침 → 제외 (exclusive)
        { timestamp: 20_000, value: 100 }, // 범위 밖
      ],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.size).toBe(1);
    expect(m.get(1_000)).toBe(1);
  });

  it('비숫자 값은 스킵 (해당 버킷에서 제외)', () => {
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
    expect(m.get(1_000)).toBe(42);
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
    expect(m.get(1_000)).toBe(5);
  });

  it('버킷 경계값은 다음 버킷으로 분류', () => {
    // startMs=1000, intervalMs=3000 → 경계 4000 은 두 번째 버킷(4000~7000) 시작
    const m = bucketAndAggregate(
      [{ timestamp: 4_000, value: 7 }],
      startMs,
      endMs,
      intervalMs,
      'average',
    );
    expect(m.get(4_000)).toBe(7);
    expect(m.has(1_000)).toBe(false);
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

  it('단일 키: time_range 모드 요청 + 평균 집계 매트릭스', async () => {
    postMock.mockResolvedValueOnce({
      entries: [
        { timestamp: 1_500, value: 10 },
        { timestamp: 2_500, value: 20 },
        { timestamp: 4_500, value: 30 },
      ],
      count: 3,
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
    });

    expect(m.columns).toEqual(['indoor:1:room_temp']);
    expect(m.rows).toEqual([
      { bucketStartMs: 1_000, values: [15] }, // (10+20)/2
      { bucketStartMs: 4_000, values: [30] },
    ]);
  });

  it('여러 키: 매트릭스 컬럼 순서 유지, 누락 버킷은 null', async () => {
    // 키 A 는 버킷 1000 에만, 키 B 는 버킷 4000 에만 데이터가 있다.
    postMock.mockImplementation(async (_url: string, body: unknown) => {
      const req = body as { key: string };
      if (req.key === 'A') {
        return { entries: [{ timestamp: 1_500, value: 10 }] };
      }
      return { entries: [{ timestamp: 4_500, value: 20 }] };
    });

    const m = await queryStoreMatrix('tsdb', {
      keys: ['A', 'B'],
      startMs: 1_000,
      endMs: 7_000,
      intervalMs: 3_000,
      aggregation: 'average',
    });

    expect(m.columns).toEqual(['A', 'B']);
    // 두 버킷이 각각 한 컬럼에만 값을 가진다.
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

  it('min 집계가 키별 독립적으로 계산된다', async () => {
    postMock.mockImplementation(async (_url, body) => {
      const req = body as { key: string };
      if (req.key === 'X') {
        return {
          entries: [
            { timestamp: 100, value: 5 },
            { timestamp: 200, value: 3 },
            { timestamp: 300, value: 7 },
          ],
        };
      }
      return {
        entries: [
          { timestamp: 100, value: 10 },
          { timestamp: 200, value: 8 },
        ],
      };
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
