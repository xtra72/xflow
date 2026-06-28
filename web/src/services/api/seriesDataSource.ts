// 시리즈 데이터 소스 통합 추상화.
//
// TSDB 에이전트와 Store 에이전트 모두에 대해 동일한 인터페이스로
// "시리즈 키 목록 조회" + "시간 구간 매트릭스 쿼리"를 제공한다.
// UI 컴포넌트(패널/모달/매트릭스)는 `SeriesDataSource` 구현체에만 의존하고
// agentType 에 따라 분기하지 않는다.
//
// @spec SPEC-WEB-005

import { storeSeriesDataSource } from './store';
import { tsdbSeriesDataSource } from './tsdb';

// ---- Types ----

/** 지원하는 데이터 소스 유형. */
export type SeriesDataSourceKind = 'tsdb' | 'store';

/**
 * `useKeys()` 훅이 반환하는 페이지 응답.
 * pagination 은 반드시 포함되며, 전체 개수와 페이지 수를 알려준다.
 */
export interface SeriesKeysPage {
  keys: string[];
  pagination: {
    page: number;
    size: number;
    total: number;
    totalPages: number;
  };
}

/** 매트릭스 쿼리 파라미터. 시간은 UTC epoch milliseconds(int64). */
/**
 * 시리즈별 조회 필터(저장소 기준 분류 — key + metric_type + tags).
 * `keys[i]` 와 같은 인덱스로 짝지어, 해당 key 의 조회를 특정 metric/tags 시리즈로 좁힌다.
 * 미지정(undefined)이면 그 key 의 모든 시리즈를 조회한다(기존 동작).
 */
export interface SeriesSelectorFilter {
  metricType?: string;
  tags?: Record<string, string>;
}

export interface SeriesMatrixQuery {
  /**
   * 요청 순서대로 컬럼을 구성할 시리즈/스토어 키 배열.
   * 시리즈별 선택 시 같은 key 가 metric/tags 가 다른 채로 중복될 수 있으며,
   * 그 경우 `seriesFilters` 로 각 시리즈를 구분한다.
   */
  keys: string[];
  /** `keys` 와 같은 인덱스의 시리즈 필터(선택). 시리즈별 분류/선택에 사용. */
  seriesFilters?: Array<SeriesSelectorFilter | undefined>;
  /** 시작 시각 — UTC epoch ms. */
  startMs: number;
  /** 종료 시각 — UTC epoch ms (exclusive 로 가정). */
  endMs: number;
  /** 버킷 크기 — milliseconds. Go duration 파서 결과를 전달받는다. */
  intervalMs: number;
  /** 집계 함수 — UI 표기(`average`) 그대로 전달한다. first/last 는 버킷 내 첫/마지막 값. */
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last';
  /**
   * 빈 버킷 채우기 전략(인터벌 구간에 값이 없을 때). 생략/'' 이면 빈 버킷 생략.
   * TSDB 소스는 백엔드에서 계산한다. Store 소스는 현재 미지원(무시).
   */
  fill?: '' | 'null' | 'zero' | 'previous' | 'avg';
}

/**
 * 통합 매트릭스 응답.
 *
 * - `columns`: 입력 `keys` 와 동일한 배열 (순서 보존).
 * - `rows[i].values[j]`: `columns[j]` 키의 `rows[i].bucketStartMs` 버킷 값.
 *   값이 없으면 `null` 을 사용한다.
 */
export interface SeriesMatrix {
  columns: string[];
  rows: Array<{
    bucketStartMs: number;
    values: Array<number | null>;
  }>;
}

/**
 * `useKeys()` 훅 반환 타입 — @tanstack/react-query 의 `UseQueryResult` 를 흉내낸
 * 최소 형상만 노출한다. 콜사이트는 `data`, `isLoading`, `isError`, `error`, `refetch`
 * 만 사용한다.
 */
export interface SeriesKeysQueryResult {
  data: SeriesKeysPage | undefined;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
}

/**
 * 에이전트별로 바인딩된 통합 데이터 소스.
 *
 * - `useKeys`: React Hook. 페이지네이션된 키 목록을 조회한다.
 * - `queryMatrix`: 일반 async 함수. 선택된 키들의 매트릭스 쿼리를 수행한다.
 *
 * Hook 과 일반 함수가 섞여있으므로 구현체는 "훅 하나 + 함수 하나" 만 노출하는
 * 객체를 반환한다 (class 대신 object literal 사용).
 */
export interface SeriesDataSource {
  kind: SeriesDataSourceKind;
  useKeys: (params: { page: number; size: number }) => SeriesKeysQueryResult;
  queryMatrix: (
    params: SeriesMatrixQuery,
    signal?: AbortSignal,
  ) => Promise<SeriesMatrix>;
}

// ---- Factory ----

/**
 * `kind` 에 따라 적절한 구현체를 반환한다.
 *
 * - `kind === 'store'`: `agentName` 이 반드시 제공되어야 한다 (백엔드 라우트가 name 기반).
 * - `kind === 'tsdb'`: 백엔드가 싱글톤이므로 `agentId` 는 옵션 (미래 확장 대비).
 *
 * 팩토리가 순수 함수라 같은 입력에 대해 매번 새 객체를 만들지만, 내부 훅은
 * 파라미터를 참조만 하므로 참조 동일성이 필요한 곳에서는 상위에서 memo 처리한다.
 */
export function useSeriesDataSource(params: {
  kind: SeriesDataSourceKind;
  agentName?: string;
  agentId?: string;
}): SeriesDataSource {
  if (params.kind === 'store') {
    if (!params.agentName) {
      throw new Error(
        'useSeriesDataSource: store 데이터 소스는 agentName 이 필요합니다',
      );
    }
    return storeSeriesDataSource(params.agentName);
  }
  return tsdbSeriesDataSource(params.agentId);
}

// ---- Shared aggregation helpers ----

/**
 * 숫자 배열의 집계 값을 반환한다.
 * 숫자가 하나도 없으면 `null` 을 반환한다.
 */
export function aggregateValues(
  values: number[],
  aggregation: SeriesMatrixQuery['aggregation'],
): number | null {
  if (values.length === 0) return null;
  switch (aggregation) {
    case 'min': {
      let m = values[0]!;
      for (let i = 1; i < values.length; i++) {
        const v = values[i]!;
        if (v < m) m = v;
      }
      return m;
    }
    case 'max': {
      let m = values[0]!;
      for (let i = 1; i < values.length; i++) {
        const v = values[i]!;
        if (v > m) m = v;
      }
      return m;
    }
    case 'average': {
      let sum = 0;
      for (const v of values) sum += v;
      return sum / values.length;
    }
    case 'first':
      return values[0]!;
    case 'last':
      return values[values.length - 1]!;
    default: {
      // 타입 가드: exhaustive switch 를 컴파일 시 강제.
      const _exhaustive: never = aggregation;
      return _exhaustive;
    }
  }
}
