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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { APIError } from '@/types/api';
import type {
  SetStoreKeyMetaRequest,
  SetStoreKeyMetaResponse,
} from '@/types/store';

import { delWith, get, post, put } from './client';
import {
  aggregateValues,
  type SeriesDataSource,
  type SeriesKeysPage,
  type SeriesKeysQueryResult,
  type SeriesMatrix,
  type SeriesMatrixQuery,
  type SeriesSelectorFilter,
} from './seriesDataSource';
import { seriesDisplayName, seriesSignature } from './seriesLabels';

// ---- Backend DTOs ----

/**
 * Store 키의 데이터 타입 (SPEC-STORE-003 v0.3.0 M11).
 *
 * 백엔드가 키별로 직렬화 형식 정보를 제공한다. UI 가 직접 디스플레이/캐스팅
 * 결정에 사용하기 위해 별도 enum 으로 노출한다.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 * @spec SPEC-STORE-003 v0.3.0
 */
export type DataType = 'int' | 'float' | 'string' | 'boolean' | 'bytes' | 'json';

/**
 * 키 등록 출처 (SPEC-STORE-003 v0.3.0 M11).
 *
 * - `manual`: 설정 파일의 `keys` 배열에 등록된 정적 키.
 * - `auto`: 런타임 쓰기로 자동 생성된 동적 키.
 *
 * 정적/동적 분류는 reset 동작(히스토리 vs 엔트리 삭제)에도 영향을 준다.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 * @spec SPEC-STORE-003 v0.3.0
 */
export type RegistrationSource = 'manual' | 'auto';

/**
 * Store 키 메타데이터 객체 (SPEC-STORE-003 v0.3.0 M11).
 *
 * v0.3.0 BREAKING CHANGE: 키 목록 응답이 string 배열에서 객체 배열로 진화했다.
 * 각 객체는 키 이름, 등록 출처, 데이터 타입, 메트릭 타입, 태그 메타데이터를 포함한다.
 *
 * 예) {
 *   key: "indoor:1:room_temp",
 *   registration: "manual",
 *   data_type: "float",
 *   metric_type: "gauge",
 *   tags: { room: "1", type: "temperature" }
 * }
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 * @spec SPEC-STORE-003 v0.3.0
 */
export interface StoreKeyObject {
  key: string;
  registration: RegistrationSource;
  data_type: DataType;
  metric_type: string;
  tags: Record<string, string>;
}

/**
 * `GET /api/v1/store/{agent_name}/keys` 응답 형상 (envelope 제거 후).
 *
 * v0.3.0 (M11) BREAKING CHANGE:
 *   - `keys` 배열의 element 가 string → StoreKeyObject 로 변경.
 *   - 태그 정보는 각 객체의 `tags` 필드로 이동 (이전 top-level `tags` 맵 폐기).
 *   - 정적/동적 분류는 각 객체의 `registration` 필드로 표현.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 */
interface StoreKeysRawResponse {
  keys?: StoreKeyObject[];
  count?: number;
}

/**
 * 정적 키 태그 메타데이터 (v0.6.0 호환 derived 타입).
 * 키(Store 키 이름) → 태그맵(태그 키 → 태그 값) 매핑.
 *
 * v0.7.0 (M11) 부터 백엔드는 이 형상을 직접 제공하지 않는다.
 * `fetchStoreKeysWithTags` 가 새 응답에서 태그 비어있지 않은 키만 추려
 * 이 형상으로 derived 한다. Phase E 에서 소비처(TsdbDataViewerModal) 가
 * `keyObjects` 로 마이그레이션되면 이 타입은 제거된다.
 *
 * @spec SPEC-STORE-003
 * @spec SPEC-WEB-005 v0.7.0 (M11)
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

/**
 * 개별 엔트리 (값 타입은 런타임에 검증).
 *
 * @spec SPEC-STORE-004
 * `labels` 는 M3 백엔드부터 추가된 시리즈 식별자다. 예약 키 `__metric__` 는
 * metric_type, 그 외 키는 tag key=value 이다. 같은 store key 라도 metric/tags 가
 * 다른 다중 시리즈가 한 응답에 평탄화되어 섞여 올 수 있으므로, 클라이언트는 이
 * 라벨을 기준으로 시리즈를 분리해 각각 독립 컬럼/라인으로 렌더한다.
 * 라벨이 없는(undefined) 엔트리는 라벨 없는 단일 시리즈로 취급한다(기존 호환).
 */
interface StoreQueryEntry {
  timestamp: number;
  value: unknown;
  labels?: Record<string, string>;
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
 * `GET /api/v1/store/{agent_name}/keys` 를 호출해 키 이름 배열만 반환한다.
 *
 * v0.7.0 (M11) BREAKING CHANGE 호환 레이어:
 *   - 백엔드는 이제 객체 배열을 반환하지만, 이 함수는 시리즈 목록 페이지네이션
 *     전용이므로 `key` 필드만 추출해 string[] 형태를 유지한다.
 *   - 메타데이터(태그/등록출처/데이터타입) 가 필요한 호출자는
 *     `fetchStoreKeyObjects` 또는 `fetchStoreKeysWithTags` 를 사용한다.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 */
export async function fetchStoreKeys(agentName: string): Promise<string[]> {
  const data = await get<StoreKeysRawResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?namespace=default&pattern=*`,
  );
  // @spec SPEC-STORE-004
  // 백엔드 GET /keys 는 같은 key 를 metric/tags 별 다중 시리즈 행으로 반환한다.
  // 시리즈 선택 풀은 key 단위이므로(선택 시 해당 key 의 모든 시리즈를 한 번에 조회)
  // 등장 순서를 보존하며 중복 key 를 제거한다. 그렇지 않으면 멀티셀렉트 리스트에서
  // React key 충돌과 중복 체크박스가 발생한다.
  const seen = new Set<string>();
  const keys: string[] = [];
  for (const obj of data.keys ?? []) {
    if (seen.has(obj.key)) continue;
    seen.add(obj.key);
    keys.push(obj.key);
  }
  return keys;
}

/**
 * `GET /api/v1/store/{agent_name}/keys` 를 호출해 키 객체 배열을 그대로 반환한다.
 *
 * v0.3.0 (M11) 응답 형상에 직접 접근하고 싶은 신규 호출자(Phase E의 TsdbDataViewerModal,
 * StoreKeysEditor 등) 를 위한 신규 API. 기존 호출자는 `fetchStoreKeys` 또는
 * `fetchStoreKeysWithTags` 를 그대로 사용한다.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 */
export async function fetchStoreKeyObjects(
  agentName: string,
): Promise<StoreKeyObject[]> {
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
 * 버킷 정렬: epoch zero 기준 (벽시계 경계).
 * 사용자 `startMs` 가 인터벌 경계와 어긋나도 버킷은 항상 epoch 0 기준 벽시계
 * 경계에 정렬된다. 예) intervalMs=60000 → 모든 버킷의 초 = 0.
 * 1d 인터벌은 UTC 자정에 정렬됨 (로컬 자정 아님 — sub-day 인터벌에는 영향 없음).
 *
 * - 버킷 시작: `floor(t / intervalMs) * intervalMs`
 * - 범위 밖(t < startMs 또는 t >= endMs) 엔트리는 제외한다.
 *   `startMs` 는 이제 정렬에는 사용되지 않고 범위 하한 필터로만 쓰인다.
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
    // epoch-zero 정렬: 사용자 시작 시각과 무관하게 벽시계 경계에 맞춘다.
    const bucketStart = Math.floor(t / intervalMs) * intervalMs;
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
): 'min' | 'max' | 'avg' | null {
  switch (aggregation) {
    case 'average':
      return 'avg';
    case 'min':
      return 'min';
    case 'max':
      return 'max';
    default:
      // first/last 는 store 백엔드 서버 집계가 미지원 → null 반환하여
      // 클라이언트 측 bucketAndAggregate(aggregateValues) 경로를 사용한다.
      return null;
  }
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
 * @spec SPEC-STORE-004
 * 단일 store key 조회 결과를 시리즈 단위로 분리한 표현.
 *
 * - `signature`: labels 기준 결정적 서명. 라벨 없는 단일 시리즈는 "".
 * - `labels`: 원본 labels 맵(metric `__metric__` + tags). 라벨 없으면 undefined.
 * - `buckets`: 버킷 시작 시각 → 집계값 맵.
 */
interface KeySeries {
  signature: string;
  labels: Record<string, string> | undefined;
  buckets: Map<number, number>;
}

/**
 * @spec SPEC-STORE-004
 * 한 응답의 엔트리들을 labels 서명 기준으로 그룹화한다.
 *
 * 같은 labels(metric/tags) 를 가진 엔트리들이 하나의 시리즈로 묶이며, 같은 버킷
 * 시각에 서로 다른 시리즈가 섞여 있어도 서명으로 분리되어 값이 덮어써지지 않는다.
 * 그룹 순서는 응답에서 각 서명이 처음 등장한 순서를 보존한다(결정적 컬럼 순서).
 *
 * 라벨이 없는 엔트리는 서명 "" 으로 묶여 "라벨 없는 단일 시리즈" 가 된다(기존 호환).
 */
function groupEntriesBySeries(
  entries: StoreQueryEntry[],
): Map<string, { labels: Record<string, string> | undefined; entries: StoreQueryEntry[] }> {
  const groups = new Map<
    string,
    { labels: Record<string, string> | undefined; entries: StoreQueryEntry[] }
  >();
  for (const e of entries) {
    const sig = seriesSignature(e.labels);
    let g = groups.get(sig);
    if (!g) {
      g = {
        labels:
          e.labels && Object.keys(e.labels).length > 0 ? e.labels : undefined,
        entries: [],
      };
      groups.set(sig, g);
    }
    g.entries.push(e);
  }
  return groups;
}

/**
 * 서버 집계 응답(이미 버킷 단위) 엔트리들을 버킷 맵으로 수집한다.
 * 비숫자/비유한 값과 유한하지 않은 타임스탬프는 스킵한다.
 */
function collectAggregatedBuckets(entries: StoreQueryEntry[]): Map<number, number> {
  const result = new Map<number, number>();
  for (const e of entries) {
    if (typeof e.value !== 'number' || !Number.isFinite(e.value)) continue;
    if (!Number.isFinite(e.timestamp)) continue;
    result.set(e.timestamp, e.value);
  }
  return result;
}

/**
 * @spec SPEC-STORE-004
 * 개별 store key 에 대해 "서버 집계 시도 → 4xx 시 폴백" 을 수행하고, 응답을
 * labels 기준 다중 시리즈로 분리해 반환한다.
 *
 * 반환 배열의 각 원소는 하나의 시리즈(고유 metric/tags 조합)이며, 매트릭스 병합
 * 단계에서 각각 독립 컬럼으로 배치된다. 같은 key 에서 시리즈가 1개뿐이면 배열
 * 길이도 1 이고, 그 시리즈의 라벨이 비어있으면 기존 단일 컬럼 동작과 동일하다.
 */
/**
 * 시리즈 필터(저장소 기준 분류)를 결정적 서명으로 직렬화한다.
 * `KeySeries.signature` 와 같은 규칙(seriesSignature)을 사용하므로, 같은
 * (metric, tags) 조합이면 동일한 서명을 만들어 정확히 매칭된다.
 */
function selectorSignature(filter: SeriesSelectorFilter): string {
  const labels: Record<string, string> = { ...(filter.tags ?? {}) };
  if (filter.metricType) labels['__metric__'] = filter.metricType;
  return seriesSignature(labels);
}

async function fetchKeySeries(
  agentName: string,
  key: string,
  params: SeriesMatrixQuery,
  signal: AbortSignal | undefined,
  filter?: SeriesSelectorFilter,
): Promise<KeySeries[]> {
  const url = `/store/${encodeURIComponent(agentName)}/query`;
  const config = signal ? { signal } : undefined;
  // 시리즈별 선택(저장소 기준 분류): filter 가 주어지면, 응답에서 분리된 시리즈
  // 중 서명이 일치하는 것만 남긴다. filter 미지정이면 모든 시리즈를 반환한다(기존 동작).
  const targetSig = filter ? selectorSignature(filter) : null;
  const applyFilter = (series: KeySeries[]): KeySeries[] =>
    targetSig === null ? series : series.filter((s) => s.signature === targetSig);

  // 1차: 서버 측 집계 시도 (백엔드가 지원하는 집계일 때만).
  const backendAgg = toBackendAggregation(params.aggregation);
  if (backendAgg !== null) {
    const serverBody: StoreQueryRequest = {
      key,
      mode: 'time_range',
      start_ms: params.startMs,
      end_ms: params.endMs,
      namespace: 'default',
      interval_ms: params.intervalMs,
      aggregation: backendAgg,
    };

    try {
      const resp = await post<StoreQueryRawResponse>(url, serverBody, config);
      // 서버 집계 응답: 각 엔트리는 "버킷 시작 시각 + 집계값 (+ labels)" 이다.
      // labels 기준으로 시리즈를 분리해 각 시리즈의 버킷 맵을 구성한다.
      const groups = groupEntriesBySeries(resp?.entries ?? []);
      const out: KeySeries[] = [];
      for (const [signature, g] of groups) {
        out.push({
          signature,
          labels: g.labels,
          buckets: collectAggregatedBuckets(g.entries),
        });
      }
      return applyFilter(out);
    } catch (err) {
      if (!isAggregationUnsupportedError(err)) {
        throw err;
      }
      // 4xx: 구버전 서버 또는 파라미터 불허 → 클라이언트 집계 경로로 폴백.
    }
  }
  // first/last 또는 서버 미지원: 원본 엔트리 요청 + 클라이언트 측 버킷화/집계.

  // 2차: 원본 엔트리 요청 + 클라이언트 측 버킷화/집계. 폴백 경로도 labels 기준으로
  // 시리즈를 분리한 뒤 시리즈별로 bucketAndAggregate 를 적용한다.
  const fallbackBody: StoreQueryRequest = {
    key,
    mode: 'time_range',
    start_ms: params.startMs,
    end_ms: params.endMs,
    namespace: 'default',
  };
  const resp = await post<StoreQueryRawResponse>(url, fallbackBody, config);
  const groups = groupEntriesBySeries(resp?.entries ?? []);
  const out: KeySeries[] = [];
  for (const [signature, g] of groups) {
    out.push({
      signature,
      labels: g.labels,
      buckets: bucketAndAggregate(
        g.entries,
        params.startMs,
        params.endMs,
        params.intervalMs,
        params.aggregation,
      ),
    });
  }
  return applyFilter(out);
}

/**
 * 여러 스토어 키에 대해 `time_range` 쿼리를 병렬로 실행하고 매트릭스로 병합한다.
 *
 * - 기본적으로 서버 측 집계를 사용한다(`interval_ms` + `aggregation` 전송).
 * - 서버가 4xx 로 응답하면 자동으로 클라이언트 집계 경로로 폴백한다.
 * - 개별 요청(폴백 포함)이 최종적으로 실패하면 전체 프로미스가 rejected 된다.
 * - `signal` 로 axios 요청 중단을 전파할 수 있다 (개별 요청 모두에 주입).
 * - 결과 매트릭스의 행은 `bucketStartMs` 오름차순으로 정렬된다.
 *
 * @spec SPEC-STORE-004
 * 다중 시리즈 분리: 한 store key 가 백엔드에서 metric/tags 별 다중 시리즈로 반환되면
 * (labels 로 구분), 각 시리즈를 독립 컬럼으로 분리한다. 컬럼 표시 이름은 다음과 같다:
 *   - 한 key 에서 시리즈가 1개뿐이면 → store key 그대로 (기존 동작 보존).
 *   - 2개 이상이면 → `key · metric{tag=...}` 형태로 라벨을 덧붙여 구분.
 * 컬럼 순서는 요청 key 순서 → 각 key 안에서 시리즈 등장 순서를 보존한다.
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

  // 키별로 시리즈 배열을 병렬 조회 (서버 집계 우선, 4xx 시 폴백).
  // 같은 인덱스의 결과가 같은 요청 (key, seriesFilters[idx]) 에 대응한다.
  // 시리즈별 선택 시 같은 key 가 metric/tags 가 다른 채로 여러 인덱스에 중복될 수 있다.
  const perKeySeries: KeySeries[][] = await Promise.all(
    params.keys.map((key, idx) =>
      fetchKeySeries(agentName, key, params, signal, params.seriesFilters?.[idx]),
    ),
  );

  // 같은 key 가 몇 번 요청되었는지 — 시리즈별 선택으로 한 key 가 여러 인덱스에
  // 나뉘어 오면 컬럼명이 충돌하므로 라벨 표기를 강제한다.
  const keyRequestCount = new Map<string, number>();
  for (const key of params.keys) {
    keyRequestCount.set(key, (keyRequestCount.get(key) ?? 0) + 1);
  }

  // 요청 순서를 보존하며 모든 시리즈를 컬럼으로 평탄화한다.
  // 한 key 의 시리즈가 2개 이상이거나, 같은 key 가 여러 인덱스로 중복 요청되면
  // 라벨 표기를 덧붙여 컬럼명을 구분한다.
  const columns: string[] = [];
  const columnBuckets: Array<Map<number, number>> = [];
  params.keys.forEach((key, idx) => {
    const seriesList = perKeySeries[idx] ?? [];
    // 데이터가 전혀 없는 key 도 단일 컬럼(전부 null)으로 노출해 기존 동작을 보존한다.
    if (seriesList.length === 0) {
      columns.push(key);
      columnBuckets.push(new Map<number, number>());
      return;
    }
    const withLabel = seriesList.length > 1 || (keyRequestCount.get(key) ?? 0) > 1;
    for (const series of seriesList) {
      columns.push(seriesDisplayName(key, series.labels, withLabel));
      columnBuckets.push(series.buckets);
    }
  });

  // 전체 버킷 시작 시각의 합집합을 수집하고 정렬한다.
  const allBuckets = new Set<number>();
  for (const m of columnBuckets) {
    for (const ts of m.keys()) allBuckets.add(ts);
  }
  const sortedBuckets = Array.from(allBuckets).sort((a, b) => a - b);

  // 각 버킷에 대해 컬럼 순서대로 값을 배치한다 (없으면 null).
  const rows: SeriesMatrix['rows'] = sortedBuckets.map((bucketStartMs) => ({
    bucketStartMs,
    values: columnBuckets.map((m) => {
      const v = m.get(bucketStartMs);
      return v === undefined ? null : v;
    }),
  }));

  return {
    columns,
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
 * `GET /api/v1/store/{agent_name}/keys` 를 호출해 키 목록 + 태그 맵 + 객체 배열을 함께 받는다.
 *
 * v0.7.0 (M11) BREAKING CHANGE:
 *   - 백엔드 응답이 객체 배열로 진화함에 따라 이 함수는 새 응답을 받아
 *     v0.6.0 호환 형상(`keys` string[], `tags` StoreKeyTagsMap) 을 derived 한다.
 *   - 신규 호출자는 `keyObjects` 필드를 통해 등록출처/데이터타입 등 풀 메타데이터에
 *     접근할 수 있다. Phase E 에서 TsdbDataViewerModal 가 `keyObjects` 를 직접 사용하도록
 *     마이그레이션되면 `keys`/`tags` derived 필드는 제거 예정.
 *   - 태그가 비어 있는(`{}`) 키는 `tags` 맵에서 생략한다.
 *     v0.6.0 동작(정적 태그 보유 키만 포함)과 일관성을 유지하기 위함.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 * @spec SPEC-STORE-003
 */
export async function fetchStoreKeysWithTags(agentName: string): Promise<{
  keys: string[];
  tags: StoreKeyTagsMap;
  keyObjects: StoreKeyObject[];
}> {
  const data = await get<StoreKeysRawResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?namespace=default&pattern=*`,
  );
  const keyObjects = data.keys ?? [];

  // v0.6.0 호환을 위한 derived 필드. Phase E 에서 keyObjects 직접 사용으로 전환되면 제거.
  const keys = keyObjects.map((obj) => obj.key);
  const tags: StoreKeyTagsMap = {};
  for (const obj of keyObjects) {
    const objTags = obj.tags ?? {};
    if (Object.keys(objTags).length > 0) {
      tags[obj.key] = objTags;
    }
  }

  return { keys, tags, keyObjects };
}

/**
 * 스토어 에이전트의 키 목록 + 태그 + 키 객체를 React Query 로 캐싱한다.
 *
 * v0.7.0 (M11) 응답 shape: `{ keys, tags, keyObjects }`.
 * v0.6.0 호환을 위해 `keys`/`tags` 가 derived 필드로 유지된다.
 * 신규 호출자는 `keyObjects` 를 사용해 풀 메타데이터에 접근.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 * @spec SPEC-STORE-003
 */
export function useStoreKeysWithTags(agentName: string | undefined) {
  return useQuery<
    { keys: string[]; tags: StoreKeyTagsMap; keyObjects: StoreKeyObject[] },
    Error
  >({
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

// ---- Store reset (per-key / bulk) ----

/**
 * 단일 키 초기화 응답 형상.
 *
 * - `action: 'history_cleared'` : 정적 키 — 히스토리만 삭제 (현재 값 유지).
 * - `action: 'entry_deleted'`   : 동적 키 — 항목 자체 삭제 (다음 쓰기 시 재생성).
 *
 * @spec SPEC-STORE-003
 */
export interface StoreResetResult {
  action: 'history_cleared' | 'entry_deleted';
  key: string;
}

/**
 * 전체 초기화 응답 형상.
 *
 * - `history_cleared` : 정적 키 중 히스토리만 삭제된 건수.
 * - `entries_deleted` : 동적 키 중 항목 자체가 삭제된 건수.
 *
 * 일부 항목 처리에 실패해도 백엔드는 200 으로 베스트-에포트 결과를 반환한다.
 *
 * @spec SPEC-STORE-003
 */
export interface StoreResetAllResult {
  history_cleared: number;
  entries_deleted: number;
}

/**
 * 단일 키를 초기화한다.
 *
 * - 정적 키(설정 `keys` 배열에 등록된 키): 히스토리만 삭제하고 현재 값/TTL 은 보존.
 * - 동적 키: 엔트리 자체를 삭제 (다음 쓰기 시 재생성).
 *
 * 키가 존재하지 않으면 백엔드가 404 를 반환하며, `APIError` 로 전파된다.
 *
 * @spec SPEC-STORE-003
 */
export async function resetStoreKey(
  agentName: string,
  key: string,
  namespace?: string,
): Promise<StoreResetResult> {
  const ns = namespace ?? 'default';
  const url = `/store/${encodeURIComponent(agentName)}/keys/${encodeURIComponent(key)}?namespace=${encodeURIComponent(ns)}`;
  return delWith<StoreResetResult>(url);
}

/**
 * 모든 키를 초기화한다 (베스트-에포트).
 *
 * 정적/동적 분류에 따라 각각 히스토리 삭제 또는 엔트리 삭제를 수행하고,
 * 영향을 받은 건수를 반환한다.
 *
 * @spec SPEC-STORE-003
 */
export async function resetAllStoreKeys(
  agentName: string,
  namespace?: string,
): Promise<StoreResetAllResult> {
  const ns = namespace ?? 'default';
  const url = `/store/${encodeURIComponent(agentName)}/keys?namespace=${encodeURIComponent(ns)}`;
  return delWith<StoreResetAllResult>(url);
}

// ---- Key meta (metric_type / tags) ----

/**
 * 임의 엔트리(정적 + 동적)의 metric_type / tags 를 설정한다.
 *
 * `PUT /api/v1/store/{agent_name}/keys/{key}/meta` 를 호출한다.
 *
 * 동작 특성:
 *   - tags 는 **전체 교체** (merge 아님). 부분 수정 시 기존+변경 전체를 전송해야 한다.
 *   - metric_type 미지정/빈 문자열 → 백엔드가 `"unknown"` 으로 normalize.
 *   - 키는 URL 인코딩된다 (`:` → `%3A`). `encodeURIComponent` 가 콜론을 인코딩한다.
 *   - 검증 실패(metric_type 정규식 / tag key 정규식) 시 400 → `APIError` 로 전파된다.
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export async function setStoreKeyMeta(
  agentName: string,
  key: string,
  meta: SetStoreKeyMetaRequest,
): Promise<SetStoreKeyMetaResponse> {
  const url = `/store/${encodeURIComponent(agentName)}/keys/${encodeURIComponent(key)}/meta`;
  return put<SetStoreKeyMetaResponse>(url, meta);
}

/** `useSetStoreKeyMeta` mutation 변수. */
export interface SetStoreKeyMetaVars {
  agentName: string;
  key: string;
  meta: SetStoreKeyMetaRequest;
}

/**
 * 엔트리 메타데이터(metric_type/tags) 설정 mutation 훅.
 *
 * 성공 시 해당 에이전트의 store 키 목록/태그 캐시를 invalidate 하여 즉시 UI 에
 * 반영한다. State 엔트리(`['agents', agentId]`)는 호출자가 추가로 invalidate 해야
 * 한다(이 훅은 agentId 를 알지 못하므로 store 키 캐시만 담당한다).
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export function useSetStoreKeyMeta() {
  const queryClient = useQueryClient();
  return useMutation<SetStoreKeyMetaResponse, Error, SetStoreKeyMetaVars>({
    mutationFn: ({ agentName, key, meta }) =>
      setStoreKeyMeta(agentName, key, meta),
    onSuccess: (_data, { agentName }) => {
      void queryClient.invalidateQueries({
        queryKey: ['store', 'keys', agentName],
      });
      void queryClient.invalidateQueries({
        queryKey: ['store', 'keys-with-tags', agentName],
      });
      void queryClient.invalidateQueries({
        queryKey: ['store', 'tags', agentName],
      });
    },
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
