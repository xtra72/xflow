// TSDB 시리즈 조회 및 쿼리 API 모듈.
// SPEC-WEB-005 의 프론트엔드 요구사항에 맞춰 백엔드 엔드포인트를 래핑한다.
//
// 설계 메모:
//   - 백엔드 `GET /api/v1/tsdb/series` 는 optional page/size/agent_id 파라미터를 받는다.
//   - 백엔드 `POST /api/v1/tsdb/query` 는 단일 series_key 와 RFC3339Nano 시각을 받는다.
//     (본 모듈은 UI 친화적인 `keys: []` + epoch ms 입력을 받아 내부적으로 fan-out 한다.)
//   - 집계 함수는 UI 에서 "average" 를 사용하고 백엔드 전송 시 "avg" 로 매핑한다.
//
// @spec SPEC-WEB-005

import type { PaginationMeta } from '@/types/api';

import { get, post } from './client';

// ---- Types: Series listing ----

/**
 * `GET /api/v1/tsdb/series` 응답의 (envelope 제거된) 형상.
 *
 * - `page` 또는 `size` 파라미터가 포함된 요청의 응답은 `pagination` 필드를 포함한다.
 * - 파라미터 없이 호출되면 `pagination` 은 undefined 다 (하위 호환).
 */
export interface TsdbSeriesListResponse {
  series: string[];
  count: number;
  pagination?: PaginationMeta;
}

/** `useTsdbSeries` 훅 파라미터. */
export interface TsdbSeriesListParams {
  page: number;
  size: number;
  agentId?: string;
}

// ---- Types: Query (UI-friendly) ----

/** UI 에서 노출하는 집계 함수 옵션. */
export type TsdbAggregation = 'min' | 'max' | 'average';

/**
 * 매트릭스 쿼리 요청 (UI 계층).
 *
 * - `keys`: 선택된 시리즈 키 배열. 각 키마다 백엔드에 병렬 요청을 전송한다.
 * - `startMs` / `endMs`: UTC epoch milliseconds (int64 UnixMilli 규약 준수).
 * - `interval`: Go duration string (예: "1m", "1h", "30s").
 * - `aggregation`: UI 표기. 백엔드 전송 시 "average" -> "avg" 로 치환한다.
 */
export interface TsdbQueryRequest {
  keys: string[];
  startMs: number;
  endMs: number;
  interval: string;
  aggregation: TsdbAggregation;
  agentId?: string;
}

/** 매트릭스 셀 1개 (버킷 × 시리즈 교차점). */
export interface TsdbQueryPoint {
  /** UTC epoch milliseconds. */
  timestampMs: number;
  /** 집계된 스칼라 값. 없으면 null. */
  value: number | null;
}

/** 시리즈별 결과. */
export interface TsdbQuerySeriesResult {
  key: string;
  points: TsdbQueryPoint[];
}

/**
 * 매트릭스 쿼리 응답 (UI 계층).
 * 백엔드에서 개별 시리즈 응답을 받아 하나의 응답으로 합친다.
 */
export interface TsdbQueryResponse {
  results: TsdbQuerySeriesResult[];
}

// ---- Backend DTO shapes (RFC3339Nano 기반) ----

interface BackendQueryRequest {
  series_key: string;
  start: string;
  end: string;
  bucket: string;
  aggregation: string;
}

interface BackendQueryPoint {
  timestamp: string;
  fields: Record<string, unknown>;
}

interface BackendSeriesResult {
  series_key: string;
  points: BackendQueryPoint[];
}

interface BackendQueryResponse {
  results: BackendSeriesResult[];
}

// ---- Time conversion utilities ----

/**
 * 로컬 `Date` 를 UTC epoch ms(int64 UnixMilli) 로 변환한다.
 * `Date.getTime()` 은 항상 UTC 기준이므로 로컬→UTC 변환이 자동으로 수행된다.
 */
export function toEpochMs(localDate: Date): number {
  return localDate.getTime();
}

/**
 * UTC epoch ms 를 브라우저 로컬 `Date` 로 변환한다.
 */
export function fromEpochMs(ms: number): Date {
  return new Date(ms);
}

/**
 * `<input type="datetime-local">` 값(ISO 8601 로컬, 타임존 없음)을 epoch ms 로 변환한다.
 * 빈 문자열이면 NaN 을 반환한다.
 */
export function datetimeLocalToEpochMs(value: string): number {
  if (!value) return Number.NaN;
  // `new Date("2026-04-23T00:00")` 는 로컬 타임존으로 해석된다.
  const d = new Date(value);
  return d.getTime();
}

/**
 * epoch ms 를 브라우저 로컬 `YYYY-MM-DD HH:mm:ss` 포맷으로 렌더링한다.
 */
export function formatLocalTimestamp(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  const y = d.getFullYear();
  const mo = pad(d.getMonth() + 1);
  const day = pad(d.getDate());
  const h = pad(d.getHours());
  const mi = pad(d.getMinutes());
  const s = pad(d.getSeconds());
  return `${y}-${mo}-${day} ${h}:${mi}:${s}`;
}

// ---- Interval helpers ----

/**
 * Go duration string 을 milliseconds 로 파싱한다.
 * 지원 단위: `ms`, `s`, `m`, `h`.
 * 잘못된 입력은 NaN 반환.
 */
export function parseIntervalToMs(interval: string): number {
  const match = /^(\d+)(ms|s|m|h)$/.exec(interval.trim());
  if (!match) return Number.NaN;
  const value = Number.parseInt(match[1]!, 10);
  if (!Number.isFinite(value) || value <= 0) return Number.NaN;
  const unit = match[2]!;
  switch (unit) {
    case 'ms':
      return value;
    case 's':
      return value * 1000;
    case 'm':
      return value * 60 * 1000;
    case 'h':
      return value * 60 * 60 * 1000;
    default:
      return Number.NaN;
  }
}

/** 인터벌 문자열이 유효한 Go duration 인지 검사한다. */
export function isValidInterval(interval: string): boolean {
  return Number.isFinite(parseIntervalToMs(interval));
}

/**
 * 예상 결과 행(버킷) 수를 계산한다.
 * `end > start` 와 유효 인터벌을 가정한다. 유효하지 않으면 0 반환.
 */
export function estimateBucketCount(
  startMs: number,
  endMs: number,
  interval: string,
): number {
  const intervalMs = parseIntervalToMs(interval);
  if (!Number.isFinite(intervalMs) || intervalMs <= 0) return 0;
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return 0;
  if (endMs <= startMs) return 0;
  return Math.ceil((endMs - startMs) / intervalMs);
}

// ---- Backend-facing helpers ----

/**
 * UI 집계 값을 백엔드 AggregateFunc 문자열로 매핑한다.
 * - "average" -> "avg"
 */
function mapAggregationToBackend(agg: TsdbAggregation): string {
  return agg === 'average' ? 'avg' : agg;
}

/**
 * 백엔드 단일 시리즈 응답에서 매트릭스용 스칼라 값을 추출한다.
 * - `fields` 가 단일 숫자 필드를 가지면 그 값을 사용한다.
 * - 여러 필드가 있으면 첫 번째 숫자 필드를 선택한다.
 * - 숫자 필드가 없으면 null 을 반환한다.
 */
function extractScalar(fields: Record<string, unknown>): number | null {
  for (const v of Object.values(fields)) {
    if (typeof v === 'number' && Number.isFinite(v)) {
      return v;
    }
  }
  return null;
}

/**
 * 백엔드 시리즈 응답을 UI 포맷으로 변환한다.
 * `requestedKey` 를 사용해 원래 요청 키를 보존한다 (백엔드가 key 를 정규화해 echo 할 수 있음).
 */
function convertBackendSeries(
  requestedKey: string,
  result: BackendSeriesResult,
): TsdbQuerySeriesResult {
  return {
    key: requestedKey,
    points: result.points.map((p) => ({
      timestampMs: new Date(p.timestamp).getTime(),
      value: extractScalar(p.fields),
    })),
  };
}

// ---- API functions ----

/**
 * `GET /api/v1/tsdb/series` 를 호출한다.
 *
 * 항상 `page`/`size` 를 함께 전송하여 페이지네이션 메타(`pagination`)를 포함한
 * 응답을 받도록 한다. size 상한은 백엔드에서 100 으로 클램프된다.
 */
export async function listTsdbSeries(
  params: TsdbSeriesListParams,
): Promise<TsdbSeriesListResponse> {
  const query: Record<string, string | number> = {
    page: params.page,
    size: params.size,
  };
  if (params.agentId) {
    query.agent_id = params.agentId;
  }
  return get<TsdbSeriesListResponse>('/tsdb/series', { params: query });
}

/**
 * 여러 시리즈에 대해 `POST /api/v1/tsdb/query` 를 병렬 호출하고 결과를 합친다.
 *
 * 각 키마다 독립적인 쿼리를 전송한 뒤 선택 순서대로 결과를 정렬한다.
 * 개별 요청 실패 시 전체 mutation 이 rejected 된다 (부분 실패 미지원).
 */
export async function queryTsdbMatrix(
  req: TsdbQueryRequest,
): Promise<TsdbQueryResponse> {
  if (req.keys.length === 0) {
    return { results: [] };
  }
  if (req.endMs <= req.startMs) {
    throw new Error('종료 시각은 시작 시각 이후여야 합니다');
  }
  if (!isValidInterval(req.interval)) {
    throw new Error('인터벌 문법이 올바르지 않습니다');
  }

  const startIso = new Date(req.startMs).toISOString();
  const endIso = new Date(req.endMs).toISOString();
  const aggregation = mapAggregationToBackend(req.aggregation);

  const responses = await Promise.all(
    req.keys.map((key) => {
      const body: BackendQueryRequest = {
        series_key: key,
        start: startIso,
        end: endIso,
        bucket: req.interval,
        aggregation,
      };
      return post<BackendQueryResponse>('/tsdb/query', body);
    }),
  );

  // 백엔드 응답이 results 배열을 반환하므로 첫 결과만 취한다.
  // (series_key 를 1개만 지정했기 때문에 results 의 길이는 0 또는 1 이다.)
  const results: TsdbQuerySeriesResult[] = req.keys.map((key, idx) => {
    const resp = responses[idx];
    const first = resp?.results?.[0];
    if (!first) {
      return { key, points: [] };
    }
    return convertBackendSeries(key, first);
  });

  return { results };
}
