// influxdbManagement API 클라이언트의 응답 언래핑 회귀 테스트.
//
// 백엔드는 목록 응답을 { buckets: [...] } / { measurements: [...], count } 객체로 반환한다
// (envelope 언래핑 후). 프론트가 이를 배열로 가정해 그대로 반환하면 소비 측 `.map()` 이
// "T.map is not a function" 으로 크래시한다. 아래 테스트는 객체에서 배열을 올바로 추출하는지
// 검증하고, 하위 호환(직접 배열)·빈 응답 폴백도 함께 확인한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./client', () => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn(),
}));

import { get } from './client';
import {
  fetchInfluxBuckets,
  fetchInfluxMeasurements,
  type InfluxBucket,
} from './influxdbManagement';

const getMock = vi.mocked(get);

const BUCKET: InfluxBucket = {
  id: '1',
  name: 'b1',
  orgId: 'org',
  retentionSeconds: 0,
};

describe('influxdbManagement API — 목록 응답 언래핑', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('fetchInfluxBuckets: 백엔드 { buckets: [...] } 객체에서 배열을 추출한다', async () => {
    getMock.mockResolvedValueOnce({ buckets: [BUCKET] } as unknown as InfluxBucket[]);
    const result = await fetchInfluxBuckets('agent');
    expect(Array.isArray(result)).toBe(true);
    expect(result).toEqual([BUCKET]);
  });

  it('fetchInfluxBuckets: 이미 배열이면 그대로 반환한다 (하위 호환)', async () => {
    getMock.mockResolvedValueOnce([BUCKET]);
    expect(await fetchInfluxBuckets('agent')).toEqual([BUCKET]);
  });

  it('fetchInfluxBuckets: null 또는 빈 객체면 빈 배열로 폴백한다', async () => {
    getMock.mockResolvedValueOnce(null as unknown as InfluxBucket[]);
    expect(await fetchInfluxBuckets('agent')).toEqual([]);
    getMock.mockResolvedValueOnce({} as unknown as InfluxBucket[]);
    expect(await fetchInfluxBuckets('agent')).toEqual([]);
  });

  it('fetchInfluxMeasurements: 백엔드 { measurements: [...] } 객체에서 배열을 추출한다', async () => {
    getMock.mockResolvedValueOnce({
      measurements: ['m1', 'm2'],
      count: 2,
    } as unknown as string[]);
    const result = await fetchInfluxMeasurements('agent', 'b1');
    expect(Array.isArray(result)).toBe(true);
    expect(result).toEqual(['m1', 'm2']);
  });

  it('fetchInfluxMeasurements: 이미 배열이면 그대로 반환한다 (하위 호환)', async () => {
    getMock.mockResolvedValueOnce(['m1']);
    expect(await fetchInfluxMeasurements('agent', 'b1')).toEqual(['m1']);
  });

  it('fetchInfluxMeasurements: null 또는 빈 객체면 빈 배열로 폴백한다', async () => {
    getMock.mockResolvedValueOnce(null as unknown as string[]);
    expect(await fetchInfluxMeasurements('agent', 'b1')).toEqual([]);
  });
});
