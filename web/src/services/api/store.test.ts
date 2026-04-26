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
const delWithMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  delWith: delWithMock,
}));

import { APIError } from '@/types/api';

import {
  bucketAndAggregate,
  fetchStoreKeysWithTags,
  fetchStoreTagPairs,
  queryStoreMatrix,
  resetAllStoreKeys,
  resetStoreKey,
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
    // 결과는 클라이언트 집계로 (10+20)/2 = 15.
    expect(m.rows).toEqual([{ bucketStartMs: 1_000, values: [15] }]);
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

  it('tags 가 포함된 응답을 그대로 반환한다', async () => {
    getMock.mockResolvedValueOnce({
      keys: ['a', 'b'],
      tags: {
        a: { room: '1', type: 'temperature' },
        b: { room: '2' },
      },
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
  });

  it('tags 필드가 없으면 빈 객체로 폴백 (구버전 서버 호환)', async () => {
    getMock.mockResolvedValueOnce({ keys: ['a', 'b'] });
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(result.keys).toEqual(['a', 'b']);
    expect(result.tags).toEqual({});
  });

  it('keys 와 tags 가 모두 없으면 둘 다 빈 값 반환', async () => {
    getMock.mockResolvedValueOnce({});
    const result = await fetchStoreKeysWithTags('agent-a');
    expect(result.keys).toEqual([]);
    expect(result.tags).toEqual({});
  });
});
