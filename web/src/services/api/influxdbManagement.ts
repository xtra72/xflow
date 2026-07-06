// InfluxDB 에이전트의 bucket / measurement 관리 API 클라이언트.
//
// 백엔드 REST 계약 (응답은 { data: ... } envelope — apiClient 인터셉터가 벗겨준다):
//   GET    /api/v1/influxdb/{agent_name}/buckets
//     → [{ id, name, orgId, retentionSeconds }]
//   POST   /api/v1/influxdb/{agent_name}/buckets   body { name, retentionSeconds? }
//     → { id, name, orgId, retentionSeconds }
//   DELETE /api/v1/influxdb/{agent_name}/buckets/{bucket}
//     → { deleted: true }
//   POST   /api/v1/influxdb/{agent_name}/buckets/{bucket}/truncate
//     → { truncated: true }
//   GET    /api/v1/influxdb/{agent_name}/measurements?bucket={name}
//     → ["m1", "m2"]
//   DELETE /api/v1/influxdb/{agent_name}/measurements/{name}?bucket={name}
//     → { deleted: true }
//
// v3 에이전트는 관리 미지원이라 백엔드가 501/400 + 에러 메시지를 반환할 수 있다.
// 이 경우 apiClient 인터셉터가 APIError 로 변환하며, UI 훅/컴포넌트에서 우아하게 처리한다.

import { del, get, post } from './client';

/**
 * InfluxDB 버킷 메타데이터.
 *
 * `retentionSeconds` 는 0 이면 무제한(만료 없음)을 의미한다.
 */
export interface InfluxBucket {
  id: string;
  name: string;
  orgId: string;
  retentionSeconds: number;
}

/** 버킷 생성 요청 본문. `retentionSeconds` 생략 시 백엔드 기본값(무제한)을 사용한다. */
export interface CreateBucketRequest {
  name: string;
  retentionSeconds?: number;
}

/** `GET /buckets` — 버킷 목록을 조회한다. */
export async function fetchInfluxBuckets(
  agentName: string,
): Promise<InfluxBucket[]> {
  const data = await get<InfluxBucket[]>(
    `/influxdb/${encodeURIComponent(agentName)}/buckets`,
  );
  // 백엔드가 null/미정의를 반환할 가능성에 대비해 빈 배열로 폴백한다.
  return data ?? [];
}

/** `POST /buckets` — 버킷을 생성하고 생성된 버킷 메타를 반환한다. */
export async function createInfluxBucket(
  agentName: string,
  req: CreateBucketRequest,
): Promise<InfluxBucket> {
  return post<InfluxBucket>(
    `/influxdb/${encodeURIComponent(agentName)}/buckets`,
    req,
  );
}

/** `DELETE /buckets/{bucket}` — 버킷을 삭제한다. */
export async function deleteInfluxBucket(
  agentName: string,
  bucket: string,
): Promise<void> {
  await del(
    `/influxdb/${encodeURIComponent(agentName)}/buckets/${encodeURIComponent(bucket)}`,
  );
}

/**
 * `POST /buckets/{bucket}/truncate` — 버킷을 초기화한다(데이터 전체 삭제, 버킷은 유지).
 *
 * 파괴적 작업이므로 호출부에서 반드시 강한 확인 절차를 거쳐야 한다.
 */
export async function truncateInfluxBucket(
  agentName: string,
  bucket: string,
): Promise<void> {
  await post<{ truncated: boolean }>(
    `/influxdb/${encodeURIComponent(agentName)}/buckets/${encodeURIComponent(bucket)}/truncate`,
  );
}

/** `GET /measurements?bucket={name}` — 지정 버킷의 measurement 이름 목록을 조회한다. */
export async function fetchInfluxMeasurements(
  agentName: string,
  bucket: string,
): Promise<string[]> {
  const params = new URLSearchParams({ bucket });
  const data = await get<string[]>(
    `/influxdb/${encodeURIComponent(agentName)}/measurements?${params}`,
  );
  return data ?? [];
}

/** `DELETE /measurements/{name}?bucket={name}` — 지정 버킷의 measurement 를 삭제한다. */
export async function deleteInfluxMeasurement(
  agentName: string,
  bucket: string,
  measurement: string,
): Promise<void> {
  const params = new URLSearchParams({ bucket });
  await del(
    `/influxdb/${encodeURIComponent(agentName)}/measurements/${encodeURIComponent(measurement)}?${params}`,
  );
}
