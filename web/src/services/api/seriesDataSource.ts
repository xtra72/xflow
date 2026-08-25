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
import { tsdbSeriesDataSourceFor } from './tsdbSource';

// ---- Types ----

/**
 * 지원하는 데이터 소스 유형.
 *
 * - `'store'`: Store 에이전트(인메모리 TSDB, `/api/v1/store/{agent_name}/*`).
 * - `'tsdb'`: **에이전트를 통해 접근하는 외부 시계열 DB**(InfluxDB 가 첫 백엔드).
 *   패널 데이터소스로 노출되며 어댑터는 `tsdbSource.ts` 가 소유한다.
 * - `'memtsdb'`: 프로세스 내 시계열 저장소(`internal/tsdb/`, `/api/v1/tsdb/*`).
 *   플로우 노드 · WS 구독자용 내부 설비이며 **패널 데이터소스가 아니다**.
 *
 * `'tsdb'` 는 v0.3.0 이전까지 memTSDB 를 가리켰다. 같은 문자열이 두 화면에서
 * 반대 뜻을 갖는 것을 막기 위해 memTSDB 쪽을 `'memtsdb'` 로 개명하고 `'tsdb'` 를
 * 외부 TSDB 에 배정한다. 식별자 개명이며 동작 변경이 아니다.
 *
 * @spec SPEC-TSDB-002 §2.1 (U1) · §4.8
 */
export type SeriesDataSourceKind = 'store' | 'tsdb' | 'memtsdb';

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
 * 시리즈별 조회 필터(저장소 기준 분류 — key + field + tags).
 * `keys[i]` 와 같은 인덱스로 짝지어, 해당 key 의 조회를 특정 metric/tags 시리즈로 좁힌다.
 * 미지정(undefined)이면 그 key 의 모든 시리즈를 조회한다(기존 동작).
 */
export interface SeriesSelectorFilter {
  fieldName?: string;
  tags?: Record<string, string>;
  /**
   * 시리즈를 나눌 태그 키 목록. @spec SPEC-TSDB-004 §2.1
   *
   * 지정되면 이 인덱스가 컬럼 N개로 펼쳐진다. 소비자는 `SeriesMatrix.columnOrigins`
   * 로 그 컬럼들이 어느 인덱스에서 나왔는지 알 수 있다.
   */
  groupBy?: string[];
  /**
   * 반환할 그룹을 태그 값 **조합 목록**으로 제한한다(시리즈축 페이지네이션).
   * @spec SPEC-TSDB-004 §2.7.1
   */
  groupFilter?: Array<Record<string, string>>;
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
  /**
   * 인터벌(버킷) 집계.
   *
   * 백엔드마다 처리 범위가 다르다 — InfluxDB 는 일곱 종을 서버에서 처리하고,
   * Store 는 min/max/avg 만 서버에서 처리한 뒤 나머지는 `aggregateValues` 의
   * 클라이언트 경로로 내려온다. 계약은 넓게 두고 처리 위치만 달리한다.
   */
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last' | 'sum' | 'count';
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
  /**
   * `columns[j]` 가 요청의 몇 번째 키에서 나왔는지(SPEC-TSDB-004 §4.1).
   *
   * group by 는 요청 1건이 컬럼 N개를 만들므로 "컬럼 j ↔ 시리즈 j" 위치 대응이
   * 깨진다. 출처 인덱스를 함께 실어 표시 메타데이터(alias · color · 선 스타일)를
   * 정확히 귀속시킨다. 여러 컬럼이 같은 인덱스를 가리키면 그 인덱스는 group by
   * 항목이다.
   *
   * **옵셔널이다.** 생략하면 소비자가 종전 위치 정렬로 되돌아가므로, 이 필드를
   * 만들지 않는 생산자의 동작은 변하지 않는다.
   */
  columnOrigins?: number[];
  /**
   * `columns[j]` 시리즈의 원본 labels(`__field__` + tags).
   *
   * 표시 이름을 그룹의 실제 태그 값으로 만들 때 쓴다. 이름 문자열에서 태그를
   * 역파싱하는 방식은 값에 구분자가 들어가면 깨지므로 구조를 유지한 채 전달한다.
   */
  columnLabels?: Array<Record<string, string> | undefined>;
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
 * - `kind === 'tsdb'`: 외부 시계열 DB. `agentName` 이 필수이며, `agentType` 은 백엔드
 *   파생의 정본이다(SPEC-TSDB-002 §2.18). 타입을 알 수 없으면 어댑터가 백엔드 불일치
 *   오류를 내며, 이는 다른 소스로 조용히 새는 것보다 안전하다.
 * - `kind === 'memtsdb'`: 프로세스 내 저장소(`internal/tsdb/`). 백엔드가 싱글톤이므로
 *   `agentId` 는 옵션 (미래 확장 대비).
 *
 * **종류별 분기를 `switch` 로 전수 처리한다.** 종전에는 `'store'` 가 아니면 전부
 * memTSDB 어댑터로 흘러내렸고, 그 폴백은 `'tsdb'` 가 도달 가능해지는 순간
 * **외부 TSDB 요청을 memTSDB 로 오라우팅**한다. memTSDB 는 패널 데이터소스가 아니므로
 * (spec.md §1.2.1 · UB1-21) 이 경로는 닫혀 있어야 한다.
 *
 * 팩토리가 순수 함수라 같은 입력에 대해 매번 새 객체를 만들지만, 내부 훅은
 * 파라미터를 참조만 하므로 참조 동일성이 필요한 곳에서는 상위에서 memo 처리한다.
 */
export function useSeriesDataSource(params: {
  kind: SeriesDataSourceKind;
  agentName?: string;
  agentId?: string;
  /** 외부 TSDB 백엔드 파생의 정본(`kind === 'tsdb'` 에서만 쓰인다). */
  agentType?: string;
}): SeriesDataSource {
  switch (params.kind) {
    case 'store': {
      if (!params.agentName) {
        throw new Error(
          'useSeriesDataSource: store 데이터 소스는 agentName 이 필요합니다',
        );
      }
      return storeSeriesDataSource(params.agentName);
    }
    case 'tsdb': {
      if (!params.agentName) {
        throw new Error(
          'useSeriesDataSource: tsdb 데이터 소스는 agentName 이 필요합니다',
        );
      }
      return tsdbSeriesDataSourceFor({
        id: params.agentId ?? '',
        name: params.agentName,
        type: params.agentType ?? '',
      });
    }
    case 'memtsdb':
      return tsdbSeriesDataSource(params.agentId);
    default:
      return assertNeverKind(params.kind);
  }
}

/** 종류가 늘었는데 분기를 빠뜨리면 여기서 컴파일이 깨진다. */
function assertNeverKind(kind: never): never {
  throw new Error(`useSeriesDataSource: 알 수 없는 데이터 소스 종류 ${String(kind)}`);
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
    case 'sum': {
      let sum = 0;
      for (const v of values) sum += v;
      return sum;
    }
    case 'count':
      // 값의 크기가 아니라 표본 수다 — 결과 단위가 원본 필드와 다른 유일한 집계.
      return values.length;
    default: {
      // 타입 가드: exhaustive switch 를 컴파일 시 강제.
      const _exhaustive: never = aggregation;
      return _exhaustive;
    }
  }
}
