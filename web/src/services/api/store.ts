// Store 에이전트를 위한 시리즈 데이터 소스 어댑터.
//
// v0.3.0 (SPEC-WEB-005 Wave 2) 이전까지는 Store 백엔드가 서버 측 집계를 제공하지
// 않아 프론트엔드에서 항상 원본 엔트리를 받아 `bucketAndAggregate` 로 클라이언트
// 사이드 집계를 수행했다.
//
// v0.3.0 Wave 1 백엔드 변경으로 `/query` 엔드포인트가 `interval_ms` + `aggregation`
// 을 optional 로 받아 서버 측 버킷화+집계를 수행한다. 이 어댑터는 우선 서버 집계를
// 요청하고, 4xx (지원하지 않는 서버/파라미터) 응답 시 원본 엔트리 요청으로 폴백해
// `bucketAndAggregate` 로 클라이언트 사이드 집계를 수행한다.
//
// 키 목록 엔드포인트는 페이지네이션이 없으므로 전체 목록을 받아 클라이언트 측에서
// 슬라이스한다.
//
// @spec SPEC-WEB-005

import { useQuery } from '@tanstack/react-query';

import { APIError } from '@/types/api';

import { get, post } from './client';
import {
  aggregateValues,
  type SeriesDataSource,
  type SeriesKeysPage,
  type SeriesKeysQueryResult,
  type SeriesMatrix,
  type SeriesMatrixQuery,
} from './seriesDataSource';

// ---- Backend DTOs ----

/**
 * `GET /api/v1/store/{agent_name}/keys` 응답 형상 (envelope 제거 후).
 *
 * SPEC-STORE-003: `tags` 필드는 정적 키에 대한 태그 메타데이터를 담는다.
 *   { "키": { "태그키": "태그값", ... }, ... }
 * 동적 키(정적 등록되지 않은 키)는 tags 에 포함되지 않는다.
 * 정적 키가 전혀 없으면 백엔드가 `tags` 필드를 생략한다.
 */
interface StoreKeysRawResponse {
  keys?: string[];
  count?: number;
  tags?: Record<string, Record<string, string>>;
}

/**
 * 정적 키 태그 메타데이터.
 * 키(Store 키 이름) → 태그맵(태그 키 → 태그 값) 매핑.
 *
 * @spec SPEC-STORE-003
 */
export type StoreKeyTagsMap = Record<string, Record<string, string>>;

/**
 * `GET /api/v1/store/{agent_name}/tags` 응답 내 개별 태그 쌍.
 *
 * 예) { key: "room", values: ["1", "2", "3"] }
 *
 * @spec SPEC-STORE-003
 */
export interface StoreTagPair {
  key: string;
  values: string[];
}

/** `GET /api/v1/store/{agent_name}/tags` 응답 형상 (envelope 제거 후). */
interface StoreTagsRawResponse {
  pairs?: StoreTagPair[];
}

/**
 * `POST /api/v1/store/{agent_name}/query` 요청 형상.
 *
 * `interval_ms` / `aggregation` 은 v0.3.0 Wave 1 에서 추가된 optional 필드.
 * 서버가 이 조합을 지원하면 버킷 단위로 집계된 엔트리가 반환된다.
 * 미지원 서버(구버전) 또는 지원하지 않는 모드 조합에서는 4xx 로 실패한다.
 */
interface StoreQueryRequest {
  key: string;
  mode: 'time_range';
  start_ms: number;
  end_ms: number;
  namespace: string;
  /** 서버 집계 버킷 크기 (ms). > 0 + aggregation 동시 지정 시 서버 집계 경로 활성화. */
  interval_ms?: number;
  /** 서버 집계 함수. UI `average` 는 백엔드 `avg` 로 변환해 전달한다. */
  aggregation?: 'min' | 'max' | 'avg';
}

/** 개별 엔트리 (값 타입은 런타임에 검증). */
interface StoreQueryEntry {
  timestamp: number;
  value: unknown;
}

/** `POST /api/v1/store/{agent_name}/query` 응답 형상. */
interface StoreQueryRawResponse {
  entries?: StoreQueryEntry[];
  count?: number;
  truncated?: boolean;
}

// ---- API functions ----

/**
 * 스토어 전체 키를 서버에서 받아와 클라이언트 측에서 페이지로 슬라이스한다.
 * 서버가 페이지네이션을 지원하지 않으므로 "전체 → 자르기" 전략을 사용한다.
 */
export function sliceKeysPage(
  allKeys: string[],
  page: number,
  size: number,
): SeriesKeysPage {
  const safePage = Math.max(1, Math.floor(page));
  const safeSize = Math.max(1, Math.floor(size));
  const total = allKeys.length;
  const totalPages = total === 0 ? 0 : Math.ceil(total / safeSize);
  const startIdx = (safePage - 1) * safeSize;
  const endIdx = Math.min(startIdx + safeSize, total);
  const slice = startIdx < total ? allKeys.slice(startIdx, endIdx) : [];
  return {
    keys: slice,
    pagination: {
      page: safePage,
      size: safeSize,
      total,
      totalPages,
    },
  };
}

/**
 * `GET /api/v1/store/{agent_name}/keys` 를 호출한다.
 * 네임스페이스와 패턴은 현재 UI 상 기본값만 사용한다.
 *
 * SPEC-STORE-003 이후 응답에 optional `tags` 필드가 포함될 수 있지만,
 * 이 함수는 시리즈 목록 전용으로 키 배열만 반환한다.
 * 태그 정보는 `fetchStoreTagPairs` / `useStoreTagPairs` 를 사용한다.
 */
export async function fetchStoreKeys(agentName: string): Promise<string[]> {
  const data = await get<StoreKeysRawResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?namespace=default&pattern=*`,
  );
  return data.keys ?? [];
}

/**
 * `GET /api/v1/store/{agent_name}/tags` 를 호출해 사용 중인 태그 쌍 목록을 받는다.
 *
 * - 백엔드가 구버전(태그 엔드포인트 미지원)이면 404/400 등 4xx 를 돌려준다.
 *   → 호출자(useStoreTagPairs) 는 에러를 "태그 없음" 상태로 해석해 UI 를 숨긴다.
 * - 정적 키가 없으면 `pairs` 가 빈 배열이거나 필드 자체가 생략된다.
 *
 * @spec SPEC-STORE-003
 */
export async function fetchStoreTagPairs(
  agentName: string,
): Promise<StoreTagPair[]> {
  const data = await get<StoreTagsRawResponse>(
    `/store/${encodeURIComponent(agentName)}/tags`,
  );
  return data.pairs ?? [];
}

/**
 * 스토어 엔트리를 `intervalMs` 버킷으로 나누고 집계한다.
 *
 * - 버킷 시작: `startMs + floor((t - startMs) / intervalMs) * intervalMs`
 * - 범위 밖(t < startMs 또는 t >= endMs) 엔트리는 제외한다.
 * - 비숫자 값(value 가 number 가 아닌 경우)은 해당 버킷에서 스킵한다.
 * - 엔트리가 전혀 없는 버킷은 결과에 포함되지 않는다 (매트릭스 병합 단계에서 처리).
 */
export function bucketAndAggregate(
  entries: StoreQueryEntry[],
  startMs: number,
  endMs: number,
  intervalMs: number,
  aggregation: SeriesMatrixQuery['aggregation'],
): Map<number, number> {
  const bucketValues = new Map<number, number[]>();
  for (const e of entries) {
    const t = e.timestamp;
    if (!Number.isFinite(t)) continue;
    if (t < startMs || t >= endMs) continue;
    if (typeof e.value !== 'number' || !Number.isFinite(e.value)) continue;
    const bucketStart = startMs + Math.floor((t - startMs) / intervalMs) * intervalMs;
    let arr = bucketValues.get(bucketStart);
    if (!arr) {
      arr = [];
      bucketValues.set(bucketStart, arr);
    }
    arr.push(e.value);
  }

  const result = new Map<number, number>();
  for (const [bucketStart, values] of bucketValues) {
    const agg = aggregateValues(values, aggregation);
    if (agg !== null) {
      result.set(bucketStart, agg);
    }
  }
  return result;
}

/**
 * UI `aggregation` (average/min/max) 을 백엔드 문자열(`avg`/`min`/`max`) 로 변환한다.
 * `average` 만 `avg` 로 치환되며, 나머지는 동일하다.
 */
function toBackendAggregation(
  aggregation: SeriesMatrixQuery['aggregation'],
): 'min' | 'max' | 'avg' {
  return aggregation === 'average' ? 'avg' : aggregation;
}

/**
 * 서버가 집계 파라미터를 지원하지 않는다고 판단되는 에러 여부.
 *
 * - `APIError` + status 400~499 → 구버전 서버 또는 잘못된 파라미터 조합.
 * - 그 외(네트워크/5xx) 는 폴백하지 않고 그대로 throw.
 *
 * 400 은 지원하지 않는 파라미터(예: mode 제한, num_buckets 초과)를 포함하므로
 * 안전망으로 폴백을 시도한다. 폴백 경로도 실패하면 그 에러가 최종적으로 전파된다.
 */
function isAggregationUnsupportedError(err: unknown): boolean {
  if (err instanceof APIError) {
    return err.status >= 400 && err.status < 500;
  }
  return false;
}

/**
 * 개별 키에 대해 "서버 집계 시도 → 4xx 시 폴백" 을 수행한다.
 *
 * 반환값은 버킷 시작 시각에 정렬된 `Map<bucketStartMs, value>`. 각 경로에서
 * 반환 형태를 통일시켜 매트릭스 병합 단계의 로직을 단순하게 유지한다.
 */
async function fetchKeyBuckets(
  agentName: string,
  key: string,
  params: SeriesMatrixQuery,
  signal: AbortSignal | undefined,
): Promise<Map<number, number>> {
  const url = `/store/${encodeURIComponent(agentName)}/query`;
  const config = signal ? { signal } : undefined;

  // 1차: 서버 측 집계 시도.
  const serverBody: StoreQueryRequest = {
    key,
    mode: 'time_range',
    start_ms: params.startMs,
    end_ms: params.endMs,
    namespace: 'default',
    interval_ms: params.intervalMs,
    aggregation: toBackendAggregation(params.aggregation),
  };

  try {
    const resp = await post<StoreQueryRawResponse>(url, serverBody, config);
    // 서버 집계 응답의 각 엔트리는 "버킷 시작 시각 + 집계값" 이다.
    // 비어 있는 버킷은 서버가 생략해 돌려주므로 매트릭스 align 은 병합 단계에서 처리.
    const result = new Map<number, number>();
    for (const e of resp?.entries ?? []) {
      if (typeof e.value !== 'number' || !Number.isFinite(e.value)) continue;
      if (!Number.isFinite(e.timestamp)) continue;
      result.set(e.timestamp, e.value);
    }
    return result;
  } catch (err) {
    if (!isAggregationUnsupportedError(err)) {
      throw err;
    }
    // 4xx: 구버전 서버 또는 파라미터 불허 → 클라이언트 집계 경로로 폴백.
  }

  // 2차: 원본 엔트리 요청 + 클라이언트 측 버킷화/집계.
  const fallbackBody: StoreQueryRequest = {
    key,
    mode: 'time_range',
    start_ms: params.startMs,
    end_ms: params.endMs,
    namespace: 'default',
  };
  const resp = await post<StoreQueryRawResponse>(url, fallbackBody, config);
  return bucketAndAggregate(
    resp?.entries ?? [],
    params.startMs,
    params.endMs,
    params.intervalMs,
    params.aggregation,
  );
}

/**
 * 여러 스토어 키에 대해 `time_range` 쿼리를 병렬로 실행하고 매트릭스로 병합한다.
 *
 * - 기본적으로 서버 측 집계를 사용한다(`interval_ms` + `aggregation` 전송).
 * - 서버가 4xx 로 응답하면 자동으로 클라이언트 집계 경로로 폴백한다.
 * - 개별 요청(폴백 포함)이 최종적으로 실패하면 전체 프로미스가 rejected 된다.
 * - `signal` 로 axios 요청 중단을 전파할 수 있다 (개별 요청 모두에 주입).
 * - 결과 매트릭스의 행은 `bucketStartMs` 오름차순으로 정렬된다.
 */
export async function queryStoreMatrix(
  agentName: string,
  params: SeriesMatrixQuery,
  signal?: AbortSignal,
): Promise<SeriesMatrix> {
  if (params.keys.length === 0) {
    return { columns: [], rows: [] };
  }
  if (params.endMs <= params.startMs) {
    throw new Error('종료 시각은 시작 시각 이후여야 합니다');
  }
  if (!Number.isFinite(params.intervalMs) || params.intervalMs <= 0) {
    throw new Error('인터벌은 양수여야 합니다');
  }

  // 키별로 버킷 맵을 병렬 조회 (서버 집계 우선, 4xx 시 폴백).
  const perKeyBuckets: Array<Map<number, number>> = await Promise.all(
    params.keys.map((key) => fetchKeyBuckets(agentName, key, params, signal)),
  );

  // 전체 버킷 시작 시각의 합집합을 수집하고 정렬한다.
  const allBuckets = new Set<number>();
  for (const m of perKeyBuckets) {
    for (const ts of m.keys()) allBuckets.add(ts);
  }
  const sortedBuckets = Array.from(allBuckets).sort((a, b) => a - b);

  // 각 버킷에 대해 컬럼 순서대로 값을 배치한다 (없으면 null).
  const rows: SeriesMatrix['rows'] = sortedBuckets.map((bucketStartMs) => ({
    bucketStartMs,
    values: perKeyBuckets.map((m) => {
      const v = m.get(bucketStartMs);
      return v === undefined ? null : v;
    }),
  }));

  return {
    columns: [...params.keys],
    rows,
  };
}

// ---- React Query hook ----

/**
 * 스토어 키를 React Query 로 캐싱해 페이지 단위로 반환한다.
 * 전체 키를 한 번 받아 `sliceKeysPage` 로 슬라이스한다.
 */
function useStoreKeys(
  agentName: string,
  params: { page: number; size: number },
): SeriesKeysQueryResult {
  const query = useQuery<string[], Error>({
    queryKey: ['store', 'keys', agentName],
    queryFn: () => fetchStoreKeys(agentName),
    staleTime: 5_000,
  });

  // 데이터를 페이지로 슬라이스해 SeriesKeysPage 로 변환한다.
  const page: SeriesKeysPage | undefined = query.data
    ? sliceKeysPage(query.data, params.page, params.size)
    : undefined;

  return {
    data: page,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    refetch: () => {
      void query.refetch();
    },
  };
}

/**
 * `GET /api/v1/store/{agent_name}/keys` 를 호출해 키 목록과 태그 맵을 함께 받는다.
 *
 * SPEC-STORE-003: 응답의 `tags` 는 optional 이며, 정적 키가 없는 에이전트에서는
 * 필드 자체가 생략된다. 구버전 백엔드는 `tags` 를 무시하므로 단순히 `keys` 만 반환한다.
 *
 * @spec SPEC-STORE-003
 */
export async function fetchStoreKeysWithTags(
  agentName: string,
): Promise<{ keys: string[]; tags: StoreKeyTagsMap }> {
  const data = await get<StoreKeysRawResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?namespace=default&pattern=*`,
  );
  return {
    keys: data.keys ?? [],
    tags: data.tags ?? {},
  };
}

/**
 * 스토어 에이전트의 키 목록과 태그 메타데이터를 React Query 로 캐싱한다.
 *
 * @spec SPEC-STORE-003
 */
export function useStoreKeysWithTags(agentName: string | undefined) {
  return useQuery<{ keys: string[]; tags: StoreKeyTagsMap }, Error>({
    queryKey: ['store', 'keys-with-tags', agentName],
    queryFn: () => fetchStoreKeysWithTags(agentName!),
    enabled: Boolean(agentName),
    staleTime: 30_000,
    retry: false,
  });
}

/**
 * 스토어 에이전트의 태그 쌍 목록을 React Query 로 캐싱해 반환한다.
 *
 * - agentName 이 없으면 쿼리를 비활성화한다.
 * - 구버전 백엔드(태그 엔드포인트 미지원) 의 4xx 응답 시 `data` 는 빈 배열,
 *   `isError` 는 true 가 된다. UI 는 보통 `data?.length === 0` 또는 isError 여부로
 *   태그 필터 섹션을 숨긴다.
 *
 * @spec SPEC-STORE-003
 */
export function useStoreTagPairs(agentName: string | undefined) {
  return useQuery<StoreTagPair[], Error>({
    queryKey: ['store', 'tags', agentName],
    queryFn: () => fetchStoreTagPairs(agentName!),
    enabled: Boolean(agentName),
    staleTime: 30_000,
    // 4xx (구버전 서버 미지원) 는 재시도해도 의미 없다.
    retry: false,
  });
}

// ---- Factory ----

/**
 * 특정 스토어 에이전트 이름에 바인딩된 `SeriesDataSource` 를 생성한다.
 * 팩토리 호출 시점에 agentName 을 클로저로 고정한다.
 */
export function storeSeriesDataSource(agentName: string): SeriesDataSource {
  return {
    kind: 'store',
    useKeys: (params) => useStoreKeys(agentName, params),
    queryMatrix: (params, signal) => queryStoreMatrix(agentName, params, signal),
  };
}
