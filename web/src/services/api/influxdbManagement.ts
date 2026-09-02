// InfluxDB 에이전트의 bucket / measurement 관리 API 클라이언트.
//
// 백엔드 REST 계약 (응답은 { data: ... } envelope — apiClient 인터셉터가 벗겨준다):
//   GET    /api/v1/influxdb/{agent_name}/buckets
//     → { buckets: [{ id, name, orgId, retentionSeconds }] }
//   POST   /api/v1/influxdb/{agent_name}/buckets   body { name, retentionSeconds? }
//     → { id, name, orgId, retentionSeconds }
//   DELETE /api/v1/influxdb/{agent_name}/buckets/{bucket}
//     → { deleted: true }
//   POST   /api/v1/influxdb/{agent_name}/buckets/{bucket}/truncate
//     → { truncated: true }
//   GET    /api/v1/influxdb/{agent_name}/measurements?bucket={name}
//     → { measurements: ["m1", "m2"], count }
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
  // 백엔드는 { buckets: [...] } 객체로 응답한다(envelope 언래핑 후). 이를 배열로 가정하면
  // 소비 측 `.map()` 이 크래시하므로 nested 배열을 추출한다. 구버전/직접 배열 응답과
  // null·빈 객체도 방어적으로 빈 배열로 폴백한다.
  const data = await get<{ buckets?: InfluxBucket[] } | InfluxBucket[] | null>(
    `/influxdb/${encodeURIComponent(agentName)}/buckets`,
  );
  if (Array.isArray(data)) return data;
  return data?.buckets ?? [];
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
  // 백엔드는 { measurements: [...], count } 객체로 응답한다(envelope 언래핑 후).
  // buckets 와 동일하게 nested 배열을 추출하고, 직접 배열/null·빈 객체는 폴백한다.
  const data = await get<{ measurements?: string[] } | string[] | null>(
    `/influxdb/${encodeURIComponent(agentName)}/measurements?${params}`,
  );
  if (Array.isArray(data)) return data;
  return data?.measurements ?? [];
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

// ---- 스키마 디스커버리 (SPEC-TSDB-002 §2.10 D2/D3/D4) ----
//
// 백엔드 REST 계약 (M5 에서 신설, `internal/api/handler/influxdb_management.go`):
//   GET /api/v1/influxdb/{agent_name}/tag-keys?measurement=&bucket=
//     → { tag_keys: ["host", "region"], count }
//   GET /api/v1/influxdb/{agent_name}/tag-values?measurement=&tag_key=&bucket=
//     → { tag_values: ["a", "b"], count }
//   GET /api/v1/influxdb/{agent_name}/field-keys?measurement=&bucket=
//     → { field_keys: ["usage", "idle"], count }
//
// 세 라우트는 **관리 조작이 아니라 디스커버리**이므로 v3 에이전트에서도 활성이다
// (§2.10 · §2.13). 관리 조작(생성 · 삭제 · truncate)만 v3 에서 501 이다.
//
// 디스커버리 응답은 캐시하지 않는다(UB1-12) — 스키마는 쓰기와 함께 계속 변하므로
// 캐시된 목록은 방금 생성된 measurement/field 를 감춘다. 캐시 정책은 호출부 소관이며
// 이 모듈은 매 호출을 그대로 네트워크로 보낸다.

/**
 * 디스커버리 응답의 배열 필드를 방어적으로 추출한다.
 *
 * `fetchInfluxBuckets` · `fetchInfluxMeasurements` 와 같은 규칙이다 — 서버가 객체
 * envelope 를 주지만, 구버전/직접 배열 응답과 null 도 빈 배열로 접는다. 소비 측
 * `.map()` 이 크래시하지 않는 것이 이 폴백의 유일한 목적이다.
 */
function pickStringList(
  data: Record<string, unknown> | string[] | null,
  field: string,
): string[] {
  if (Array.isArray(data)) return data;
  const list = data?.[field];
  return Array.isArray(list) ? (list as string[]) : [];
}

/** `GET /tag-keys` — measurement 한정 태그 키 목록(D2). */
export async function fetchInfluxTagKeys(
  agentName: string,
  measurement: string,
  bucket?: string,
): Promise<string[]> {
  const params = new URLSearchParams({ measurement });
  if (bucket) params.set('bucket', bucket);
  const data = await get<Record<string, unknown> | string[] | null>(
    `/influxdb/${encodeURIComponent(agentName)}/tag-keys?${params}`,
  );
  return pickStringList(data, 'tag_keys');
}

/**
 * `GET /tag-values` — 지정 태그 키의 값 목록(D3).
 *
 * `tagKey` 는 필수다. 백엔드도 빈 값을 400 으로 거부한다 — 어느 키의 값인지 모르는
 * 목록은 쓸모가 없기 때문이다.
 */
export async function fetchInfluxTagValues(
  agentName: string,
  measurement: string,
  tagKey: string,
  bucket?: string,
  /**
   * 사전 필터(`k=v`). 값 목록을 그 조건 아래로 좁힌다.
   *
   * 그룹 미리보기가 이 인자를 쓴다 — 시리즈 열거는 접기 전 원시 행 상한에 걸려
   * 고빈도 measurement 에서 값 일부만 주지만, 태그 값 조회는 메타데이터 질의라
   * 그 상한과 무관하다. @spec SPEC-TSDB-004
   */
  filters?: Record<string, string>,
  /**
   * 조회 시간창. **비우면 백엔드의 암묵 기본값이 적용된다** — v3 의
   * SHOW TAG VALUES 는 최근 창만 훑고 Flux 는 -30d 다. 그 창 밖에서만 보고한
   * 장비가 목록에서 조용히 빠지므로, 패널이 그리는 창을 넘겨야 한다.
   * @spec SPEC-TSDB-004 UB1-19
   */
  window?: { startMs: number; endMs: number },
): Promise<string[]> {
  const params = new URLSearchParams({ measurement, tag_key: tagKey });
  if (bucket) params.set('bucket', bucket);
  if (filters && Object.keys(filters).length > 0) {
    params.set(
      'tags',
      Object.keys(filters)
        .sort()
        .map((k) => `${k}=${filters[k]}`)
        .join(','),
    );
  }
  if (window) {
    params.set('start_ms', String(window.startMs));
    params.set('end_ms', String(window.endMs));
  }
  const data = await get<Record<string, unknown> | string[] | null>(
    `/influxdb/${encodeURIComponent(agentName)}/tag-values?${params}`,
  );
  return pickStringList(data, 'tag_values');
}

/** `GET /field-keys` — measurement 한정 필드 키 목록(D4). */
export async function fetchInfluxFieldKeys(
  agentName: string,
  measurement: string,
  bucket?: string,
): Promise<string[]> {
  const params = new URLSearchParams({ measurement });
  if (bucket) params.set('bucket', bucket);
  const data = await get<Record<string, unknown> | string[] | null>(
    `/influxdb/${encodeURIComponent(agentName)}/field-keys?${params}`,
  );
  return pickStringList(data, 'field_keys');
}
