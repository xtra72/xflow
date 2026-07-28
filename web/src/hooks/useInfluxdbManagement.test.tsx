// useInfluxdbManagement 훅 테스트.
//
// 범위:
//   - useBuckets / useMeasurements 조회 (enabled 게이팅, 데이터 전달)
//   - 뮤테이션 성공 시 관련 쿼리 invalidate 검증
//   - 서비스 계층은 vi.mock 으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { ReactNode } from 'react';

import {
  useBuckets,
  useCreateBucket,
  useDeleteBucket,
  useDeleteMeasurement,
  useMeasurements,
  useTruncateBucket,
} from './useInfluxdbManagement';
import type { InfluxBucket } from '@/services/api/influxdbManagement';

vi.mock('@/services/api/influxdbManagement', () => ({
  fetchInfluxBuckets: vi.fn(),
  createInfluxBucket: vi.fn(),
  deleteInfluxBucket: vi.fn(),
  truncateInfluxBucket: vi.fn(),
  fetchInfluxMeasurements: vi.fn(),
  deleteInfluxMeasurement: vi.fn(),
}));

import * as api from '@/services/api/influxdbManagement';

const BUCKETS: InfluxBucket[] = [
  { id: 'b1', name: 'sensors', orgId: 'org1', retentionSeconds: 0 },
  { id: 'b2', name: 'logs', orgId: 'org1', retentionSeconds: 3600 },
];

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, wrapper };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('useBuckets', () => {
  it('agentName 이 있으면 버킷을 조회한다', async () => {
    (api.fetchInfluxBuckets as Mock).mockResolvedValue(BUCKETS);
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useBuckets('influx-a'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(BUCKETS);
    expect(api.fetchInfluxBuckets).toHaveBeenCalledWith('influx-a');
  });

  it('agentName 이 없으면 요청하지 않는다 (disabled)', () => {
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useBuckets(undefined), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
    expect(api.fetchInfluxBuckets).not.toHaveBeenCalled();
  });
});

describe('useMeasurements', () => {
  it('agentName 과 bucket 이 있으면 measurement 를 조회한다', async () => {
    (api.fetchInfluxMeasurements as Mock).mockResolvedValue(['m1', 'm2']);
    const { wrapper } = makeWrapper();

    const { result } = renderHook(
      () => useMeasurements('influx-a', 'sensors'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(['m1', 'm2']);
    expect(api.fetchInfluxMeasurements).toHaveBeenCalledWith(
      'influx-a',
      'sensors',
    );
  });

  it('bucket 이 없으면 요청하지 않는다 (disabled)', () => {
    const { wrapper } = makeWrapper();
    const { result } = renderHook(
      () => useMeasurements('influx-a', undefined),
      { wrapper },
    );

    expect(result.current.fetchStatus).toBe('idle');
    expect(api.fetchInfluxMeasurements).not.toHaveBeenCalled();
  });
});

describe('useCreateBucket', () => {
  it('성공 시 버킷 목록을 invalidate 한다', async () => {
    (api.createInfluxBucket as Mock).mockResolvedValue(BUCKETS[0]);
    const { queryClient, wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useCreateBucket('influx-a'), {
      wrapper,
    });

    await result.current.mutateAsync({ name: 'sensors', retentionSeconds: 0 });

    expect(api.createInfluxBucket).toHaveBeenCalledWith('influx-a', {
      name: 'sensors',
      retentionSeconds: 0,
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['influxdb', 'buckets', 'influx-a'],
    });
  });
});

describe('useDeleteBucket', () => {
  it('성공 시 버킷 목록과 measurement 캐시를 invalidate 한다', async () => {
    (api.deleteInfluxBucket as Mock).mockResolvedValue(undefined);
    const { queryClient, wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useDeleteBucket('influx-a'), {
      wrapper,
    });

    await result.current.mutateAsync('sensors');

    expect(api.deleteInfluxBucket).toHaveBeenCalledWith('influx-a', 'sensors');
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['influxdb', 'buckets', 'influx-a'],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['influxdb', 'measurements', 'influx-a', 'sensors'],
    });
  });
});

describe('useTruncateBucket', () => {
  it('성공 시 해당 버킷의 measurement 캐시를 invalidate 한다', async () => {
    (api.truncateInfluxBucket as Mock).mockResolvedValue(undefined);
    const { queryClient, wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useTruncateBucket('influx-a'), {
      wrapper,
    });

    await result.current.mutateAsync('sensors');

    expect(api.truncateInfluxBucket).toHaveBeenCalledWith('influx-a', 'sensors');
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['influxdb', 'measurements', 'influx-a', 'sensors'],
    });
  });
});

describe('useDeleteMeasurement', () => {
  it('성공 시 해당 버킷의 measurement 캐시를 invalidate 한다', async () => {
    (api.deleteInfluxMeasurement as Mock).mockResolvedValue(undefined);
    const { queryClient, wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useDeleteMeasurement('influx-a'), {
      wrapper,
    });

    await result.current.mutateAsync({ bucket: 'sensors', measurement: 'm1' });

    expect(api.deleteInfluxMeasurement).toHaveBeenCalledWith(
      'influx-a',
      'sensors',
      'm1',
    );
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['influxdb', 'measurements', 'influx-a', 'sensors'],
    });
  });
});
