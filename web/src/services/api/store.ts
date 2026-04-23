// Store 에이전트를 위한 시리즈 데이터 소스 어댑터.
//
// Store 백엔드는 TSDB 와 달리 서버 측 집계를 제공하지 않으므로, 프론트엔드에서
// `time_range` 모드로 원본 엔트리를 가져온 뒤 클라이언트 사이드에서 버킷화/집계를
//수행한다. 키 목록 엔드포인트도 페이지네이션이 없으므로 전체 목록을 받아 클라이언트
// 측에서 슬라이스한다.
//
// @spec SPEC-WEB-005

import { useQuery } from '@tanstack/react-query';

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

/** `GET /api/v1/store/{agent_name}/keys` 응답 형상 (envelope 제거 후). */
interface StoreKeysRawResponse {
  keys?: string[];
  count?: number;
}

/** `POST /api/v1/store/{agent_name}/query` 요청 형상. */
interface StoreQueryRequest {
  key: string;
  mode: 'time_range';
  start_ms: number;
  end_ms: number;
  namespace: string;
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
 */
export async function fetchStoreKeys(agentName: string): Promise<string[]> {
  const data = await get<StoreKeysRawResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?namespace=default&pattern=*`,
  );
  return data.keys ?? [];
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
 * 여러 스토어 키에 대해 `time_range` 쿼리를 병렬로 실행하고 매트릭스로 병합한다.
 *
 * - 개별 요청 실패 시 전체 프로미스가 rejected 된다.
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

  const responses = await Promise.all(
    params.keys.map((key) => {
      const body: StoreQueryRequest = {
        key,
        mode: 'time_range',
        start_ms: params.startMs,
        end_ms: params.endMs,
        namespace: 'default',
      };
      return post<StoreQueryRawResponse>(
        `/store/${encodeURIComponent(agentName)}/query`,
        body,
        signal ? { signal } : undefined,
      );
    }),
  );

  // 키별로 `bucketStart -> value` 맵을 구성한다.
  const perKeyBuckets: Array<Map<number, number>> = responses.map((resp) =>
    bucketAndAggregate(
      resp?.entries ?? [],
      params.startMs,
      params.endMs,
      params.intervalMs,
      params.aggregation,
    ),
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
