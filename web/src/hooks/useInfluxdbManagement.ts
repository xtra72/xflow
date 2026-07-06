// InfluxDB 버킷 / measurement 관리용 React Query 훅.
//
// 조회(useBuckets / useMeasurements)와 파괴적/생성 뮤테이션
// (useCreateBucket / useDeleteBucket / useTruncateBucket / useDeleteMeasurement)을 제공한다.
// 뮤테이션 성공 시 관련 쿼리를 invalidate 하여 목록을 최신으로 유지한다.
//
// v3 에이전트는 관리 미지원 → 백엔드가 501/400 을 반환하며 APIError 로 전파된다.
// 조회 훅은 retry:false 로 두어 미지원 서버에서 반복 재시도하지 않는다. 소비 컴포넌트는
// isError 로 미지원 안내를 렌더한다.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  createInfluxBucket,
  deleteInfluxBucket,
  deleteInfluxMeasurement,
  fetchInfluxBuckets,
  fetchInfluxMeasurements,
  truncateInfluxBucket,
  type CreateBucketRequest,
  type InfluxBucket,
} from '@/services/api/influxdbManagement';

/** 쿼리 키 헬퍼 — invalidate 시 동일 키를 재사용해 캐시를 정확히 무효화한다. */
const bucketsKey = (agentName: string) =>
  ['influxdb', 'buckets', agentName] as const;
const measurementsKey = (agentName: string, bucket: string) =>
  ['influxdb', 'measurements', agentName, bucket] as const;

/**
 * 버킷 목록을 조회한다.
 *
 * agentName 이 없으면 비활성화한다. v3(미지원) 서버의 4xx/5xx 는 retry 하지 않는다.
 */
export function useBuckets(agentName: string | undefined) {
  return useQuery<InfluxBucket[], Error>({
    queryKey: bucketsKey(agentName ?? ''),
    queryFn: () => fetchInfluxBuckets(agentName!),
    enabled: Boolean(agentName),
    staleTime: 10_000,
    retry: false,
  });
}

/**
 * 지정 버킷의 measurement 목록을 조회한다.
 *
 * agentName 또는 bucket 이 없으면 비활성화한다(버킷 미선택 시 요청하지 않음).
 */
export function useMeasurements(
  agentName: string | undefined,
  bucket: string | undefined,
) {
  return useQuery<string[], Error>({
    queryKey: measurementsKey(agentName ?? '', bucket ?? ''),
    queryFn: () => fetchInfluxMeasurements(agentName!, bucket!),
    enabled: Boolean(agentName) && Boolean(bucket),
    staleTime: 10_000,
    retry: false,
  });
}

/** 버킷 생성 mutation. 성공 시 버킷 목록을 invalidate 한다. */
export function useCreateBucket(agentName: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation<InfluxBucket, Error, CreateBucketRequest>({
    mutationFn: (req) => createInfluxBucket(agentName!, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: bucketsKey(agentName ?? ''),
      });
    },
  });
}

/** 버킷 삭제 mutation. 성공 시 버킷 목록과 해당 버킷의 measurement 캐시를 invalidate 한다. */
export function useDeleteBucket(agentName: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (bucket) => deleteInfluxBucket(agentName!, bucket),
    onSuccess: (_data, bucket) => {
      void queryClient.invalidateQueries({
        queryKey: bucketsKey(agentName ?? ''),
      });
      void queryClient.invalidateQueries({
        queryKey: measurementsKey(agentName ?? '', bucket),
      });
    },
  });
}

/**
 * 버킷 초기화(truncate) mutation. 성공 시 해당 버킷의 measurement 캐시를 invalidate 한다.
 *
 * 파괴적 작업(데이터 전체 삭제)이므로 호출부에서 강한 확인 절차를 거쳐야 한다.
 */
export function useTruncateBucket(agentName: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (bucket) => truncateInfluxBucket(agentName!, bucket),
    onSuccess: (_data, bucket) => {
      void queryClient.invalidateQueries({
        queryKey: measurementsKey(agentName ?? '', bucket),
      });
    },
  });
}

/** `useDeleteMeasurement` mutation 변수. */
export interface DeleteMeasurementVars {
  bucket: string;
  measurement: string;
}

/** measurement 삭제 mutation. 성공 시 해당 버킷의 measurement 캐시를 invalidate 한다. */
export function useDeleteMeasurement(agentName: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, DeleteMeasurementVars>({
    mutationFn: ({ bucket, measurement }) =>
      deleteInfluxMeasurement(agentName!, bucket, measurement),
    onSuccess: (_data, { bucket }) => {
      void queryClient.invalidateQueries({
        queryKey: measurementsKey(agentName ?? '', bucket),
      });
    },
  });
}
