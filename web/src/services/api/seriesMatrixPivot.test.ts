// seriesMatrixPivot.ts 단위 테스트 — 주입 계약만 잠근다.
//
// 컬럼 구성 · 0행 자리 보존 · 버킷 합집합 피벗의 동작 자체는 `store.test.ts` 가
// `queryStoreMatrix` 를 통해 이미 전량 잠그고 있다. 여기서는 그 경로로는 만들 수
// 없는 것 — 페처 주입 계약 — 만 다룬다.
//
// @spec SPEC-TSDB-002 §4.3 (UB1-25)

import { describe, expect, it, vi } from 'vitest';

import type { SeriesMatrixQuery } from './seriesDataSource';
import { buildSeriesMatrix, type PivotKeySeries } from './seriesMatrixPivot';

const baseParams = (keys: string[]): SeriesMatrixQuery => ({
  keys,
  startMs: 0,
  endMs: 60_000,
  intervalMs: 60_000,
  aggregation: 'average',
});

const series = (buckets: Array<[number, number]>): PivotKeySeries => ({
  labels: undefined,
  buckets: new Map(buckets),
});

describe('buildSeriesMatrix: 페처 주입 계약', () => {
  it('키가 없으면 페처를 호출하지 않고 빈 매트릭스를 반환한다', async () => {
    const fetcher = vi.fn();
    const m = await buildSeriesMatrix(baseParams([]), fetcher);
    expect(m).toEqual({ columns: [], rows: [] });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('입력 검증은 페처 호출 전에 수행된다', async () => {
    const fetcher = vi.fn();
    await expect(
      buildSeriesMatrix({ ...baseParams(['k']), endMs: 0 }, fetcher),
    ).rejects.toThrow('종료 시각은 시작 시각 이후여야 합니다');
    await expect(
      buildSeriesMatrix({ ...baseParams(['k']), intervalMs: 0 }, fetcher),
    ).rejects.toThrow('인터벌은 양수여야 합니다');
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('페처는 원본 params 를 그대로 받고, 반환 인덱스가 keys 인덱스에 대응한다', async () => {
    const params = baseParams(['a', 'b']);
    const m = await buildSeriesMatrix(params, async (p) => {
      expect(p).toBe(params);
      return [[series([[0, 1]])], [series([[0, 2]])]];
    });
    expect(m.columns).toEqual(['a', 'b']);
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [1, 2] }]);
  });

  it('페처 결과가 짧거나 undefined 인 인덱스도 자리 보존 컬럼이 된다', async () => {
    const m = await buildSeriesMatrix(baseParams(['a', 'b', 'c']), async () => [
      [series([[0, 1]])],
      undefined,
    ]);
    expect(m.columns).toEqual(['a', 'b', 'c']);
    expect(m.rows).toEqual([{ bucketStartMs: 0, values: [1, null, null] }]);
  });
});
