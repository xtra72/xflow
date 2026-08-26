// 외부 시계열 DB(TSDB) 시리즈 데이터 소스 어댑터 — InfluxDB 가 첫 백엔드.
//
// 패널이 `data_source: 'tsdb'` 를 고르면 이 모듈이 조회를 담당한다. Store 어댑터
// (`store.ts`)와 **같은 파이프라인**을 쓴다.
//
//   요청 1건 = 시리즈 1개 → 평탄 `chartQueryResponse` → 라벨 그룹화 → 버킷 합집합 피벗
//
// 뒷단 두 단계(`groupEntriesBySeries` · `buildSeriesMatrix`)는 데이터가 어디서
// 왔는지 모르므로 그대로 재사용한다. TSDB 전용 피벗 사본을 만드는 것은 금지된다
// (spec.md §2.16 #25 · UB1-25).
//
// Store 와 다른 것은 병렬 수집 정책 하나다. Store 는 `Promise.all`(한 키의 실패가
// 전체 실패)이고 여기는 `Promise.allSettled`(부분 실패 격리, spec.md §2.19). 시리즈
// 하나가 죽었다고 패널 전체를 비우지 않는다.
//
// @spec SPEC-TSDB-002 §2.6 (U6) · §2.18 (U11) · §2.19 (U12) · §4.3

import { useQuery } from '@tanstack/react-query';

import { post } from './client';
import { fetchInfluxMeasurements } from './influxdbManagement';
import type {
  SeriesDataSource,
  SeriesKeysPage,
  SeriesKeysQueryResult,
  SeriesMatrix,
  SeriesMatrixQuery,
} from './seriesDataSource';
import { buildSeriesMatrix, type PivotKeySeries } from './seriesMatrixPivot';
import {
  deriveGroupCombos,
  enumerateTsdbSeries,
  groupPageCount,
  sliceGroupPage,
  type TsdbSeriesEnumResult,
} from './tsdbSeriesEnum';
import { groupEntriesBySeries, sliceKeysPage, storeChartValue } from './store';

// ---- 에이전트 참조와 백엔드 파생 ----

/**
 * 백엔드 파생에 필요한 최소 에이전트 형상(`AgentInfo` 의 부분집합).
 *
 * `type` 이 정본이다 — 능력(interface/필드 존재)으로 백엔드를 추정하지 않는다.
 * 쓰기 경로가 같은 이유로 같은 축을 쓴다(`storage_write.go` 의 `newStorageBackend`):
 * Store 와 InfluxDB 에이전트가 둘 다 같은 처리 인터페이스를 가지므로 능력 단언은
 * 양쪽에 성립해 버린다.
 */
export interface TsdbAgentRef {
  id: string;
  name: string;
  type: string;
}

/**
 * 조회 대상 지시자 — `TsdbSourceConfig` 에서 라우팅에 필요한 부분만 추린 것.
 *
 * `TsdbSourceConfig` 를 그대로 넘겨도 된다. **기록된 백엔드 값은 읽지 않는다**
 * (UB1-22) — 이 인터페이스에 그 필드가 없는 것이 그 사실의 타입 수준 표현이다.
 */
export interface TsdbSourceRef {
  /** 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  agent_id?: string;
  /** 에이전트 이름(표시용 스냅샷 + 하위호환 폴백). */
  agent_name: string;
  /** [influxdb 전용] v2 = bucket, v3 = database. 비면 에이전트 기본값. */
  bucket?: string;
}

/**
 * 백엔드 파생 결과.
 *
 * - `'ok'`: 에이전트를 목록에서 찾았고 지원 백엔드다.
 * - `'fallback'`: 목록에서 찾지 못했다(미로딩 또는 삭제됨). 저장된 이름으로 질의하고
 *   404 는 오류 오버레이로 드러낸다(§2.17-3).
 * - `'mismatch'`: 찾았으나 지원하지 않는 타입이다. **질의하지 않는다**(§2.17-7).
 */
export type TsdbBackendResolution =
  | { status: 'ok' | 'fallback'; backend: 'influxdb'; agentName: string }
  | { status: 'mismatch'; agentName: string; agentType: string };

/** 참조된 에이전트가 지원 백엔드가 아닐 때 던지는 오류(§2.17-7 · UB2-7). */
export class TsdbBackendMismatchError extends Error {
  constructor(
    readonly agentName: string,
    readonly agentType: string,
  ) {
    super(`backend_mismatch: 에이전트 '${agentName}' 의 타입 '${agentType}' 은 TSDB 소스가 지원하지 않습니다`);
    this.name = 'TsdbBackendMismatchError';
  }
}

/**
 * 참조된 에이전트의 **실제 타입**에서 백엔드를 파생한다(§2.18).
 *
 * 해석 순서는 SPEC-WEB-006 규약을 따른다 — `agent_id` 가 정본이고, 목록에 없으면
 * 저장된 `agent_name` 으로 폴백한다(§2.17-3 · UB2-3). 이름조차 맞지 않으면 에이전트가
 * 삭제되었거나 목록이 아직 로드되지 않은 상태이며, 그때는 저장된 이름으로 질의해
 * 실패를 드러낸다. 조용히 다른 소스로 새지 않는 것이 요점이다.
 */
export function resolveTsdbBackend(
  ref: Pick<TsdbSourceRef, 'agent_id' | 'agent_name'>,
  agents: readonly TsdbAgentRef[] | undefined,
): TsdbBackendResolution {
  let matched: TsdbAgentRef | undefined;
  if (agents) {
    if (ref.agent_id) matched = agents.find((a) => a.id === ref.agent_id);
    if (!matched) matched = agents.find((a) => a.name === ref.agent_name);
  }

  if (matched) {
    switch (matched.type) {
      case 'influxdb':
        return { status: 'ok', backend: 'influxdb', agentName: matched.name };
      default:
        return { status: 'mismatch', agentName: matched.name, agentType: matched.type };
    }
  }
  return { status: 'fallback', backend: 'influxdb', agentName: ref.agent_name };
}

// ---- 백엔드 DTO ----

/**
 * `POST /api/v1/influxdb/{agent_name}/series/query` 요청 본문(§2.6).
 * Go 쪽 `dto.InfluxSeriesQueryRequest` 와 1:1 이다.
 */
interface InfluxSeriesQueryRequest {
  bucket?: string;
  measurement: string;
  field: string;
  tags?: Record<string, string>;
  /** @spec SPEC-TSDB-004 §2.1 — 시리즈를 나눌 태그 키. */
  group_by?: string[];
  /** @spec SPEC-TSDB-004 §2.7.1 — 페이지로 선택된 그룹 조합. */
  group_filter?: Array<Record<string, string>>;
  start_ms: number;
  end_ms: number;
  interval_ms: number;
  aggregation: SeriesMatrixQuery['aggregation'];
  fill?: string;
  /** `previous` 채우기의 사용 기간 제한(ms). 0·미지정이면 무제한. */
  fill_previous_max_ms?: number;
  /** 기간을 넘긴 버킷의 처리. 미지정이면 비움. */
  fill_previous_overflow?: string;
  /** 위가 `'value'` 일 때 채울 값. */
  fill_previous_overflow_value?: number;
}

/** 응답 엔트리 — Store 의 `chartQueryEntry` 와 같은 평탄 형상이다(UB1-23). */
interface InfluxSeriesQueryEntry {
  timestamp: number;
  value: unknown;
  labels?: Record<string, string>;
}

/** 응답 본문 — 신규 응답 타입을 만들지 않고 `chartQueryResponse` 를 재사용한다. */
interface InfluxSeriesQueryRawResponse {
  entries?: InfluxSeriesQueryEntry[];
  count?: number;
  truncated?: boolean;
}

// ---- 단일 시리즈 조회 ----

/** 시리즈 1건의 조회 대상(백엔드 중립 어휘 — influxdb 에서 key = measurement). */
export interface TsdbSeriesRequest {
  key: string;
  field: string;
  tags?: Record<string, string>;
  /** 시리즈를 나눌 태그 키 목록. @spec SPEC-TSDB-004 §2.1 */
  groupBy?: string[];
  /** 페이지로 선택된 그룹 조합 목록. @spec SPEC-TSDB-004 §2.7.1 */
  groupFilter?: Array<Record<string, string>>;
}

/** 시리즈 1건의 조회 창. 시리즈 축을 뺀 나머지 파라미터다. */
export interface TsdbSeriesWindow {
  bucket?: string;
  startMs: number;
  endMs: number;
  intervalMs: number;
  aggregation: SeriesMatrixQuery['aggregation'];
  fill?: SeriesMatrixQuery['fill'];
  /** `previous` 채우기의 사용 기간 제한(ms). 0·미지정이면 무제한. */
  fillPreviousMaxMs?: number;
  /** 기간을 넘긴 버킷의 처리. 미지정이면 비움. */
  fillPreviousOverflow?: '' | 'value';
  /** 위가 `'value'` 일 때 채울 값. */
  fillPreviousOverflowValue?: number;
}

/**
 * 서버가 이미 버킷 단위로 집계해 돌려준 엔트리를 버킷 맵으로 모은다.
 *
 * 버킷 **경계는 서버가 소유한다**(§2.8 · UB1-10) — 여기서 `floor(ts/interval)` 같은
 * 계산을 하지 않는다. 타임스탬프를 그대로 키로 쓴다.
 */
function collectSeriesBuckets(entries: InfluxSeriesQueryEntry[]): Map<number, number> {
  const result = new Map<number, number>();
  for (const e of entries) {
    const num = storeChartValue(e.value);
    if (num === null) continue;
    if (!Number.isFinite(e.timestamp)) continue;
    result.set(e.timestamp, num);
  }
  return result;
}

/**
 * 시리즈 1개를 조회하고 라벨 기준으로 그룹화해 반환한다(M6.3).
 *
 * 요청 1건이 시리즈 1개를 담으므로 보통 그룹은 1개다. 그럼에도 그룹화를 거치는
 * 이유는 Store 와 같은 라벨 규약(`__field__` + tags)을 쓰기 때문이며, 태그를 부분만
 * 지정해 여러 시리즈가 매칭되면 각각 독립 컬럼이 되어야 하기 때문이다.
 *
 * `field` 는 필수다 — "첫 번째 숫자 필드" 류의 폴백은 조용한 오답이므로 두지 않는다
 * (UB1-4). 비어 있으면 그 시리즈 요청만 실패하고 형제 시리즈는 영향받지 않는다.
 */
export async function fetchTsdbSeries(
  agentName: string,
  ref: TsdbSeriesRequest,
  params: TsdbSeriesWindow,
  signal?: AbortSignal,
): Promise<PivotKeySeries[]> {
  if (!ref.key) {
    throw new Error('fetchTsdbSeries: measurement(key) 가 필요합니다');
  }
  if (!ref.field) {
    throw new Error(`fetchTsdbSeries: '${ref.key}' 시리즈에 field 가 필요합니다`);
  }

  const hasTags = ref.tags !== undefined && Object.keys(ref.tags).length > 0;
  const body: InfluxSeriesQueryRequest = {
    ...(params.bucket ? { bucket: params.bucket } : {}),
    measurement: ref.key,
    field: ref.field,
    ...(hasTags ? { tags: ref.tags } : {}),
    // 빈 배열은 싣지 않는다 — 서버에서 "비면 현행" 이므로 보내도 무해하지만,
    // 요청 본문이 정확 일치 모드에서 본 축 도입 이전과 같아야 대조가 쉽다.
    ...(ref.groupBy && ref.groupBy.length > 0 ? { group_by: ref.groupBy } : {}),
    ...(ref.groupFilter && ref.groupFilter.length > 0
      ? { group_filter: ref.groupFilter }
      : {}),
    start_ms: params.startMs,
    end_ms: params.endMs,
    interval_ms: params.intervalMs,
    aggregation: params.aggregation,
    // `'avg'` 는 InfluxDB 양쪽 모두 대응물이 없어 백엔드가 400 으로 거부한다.
    // 다른 전략으로 조용히 바꾸지 않는다(UB1-17) — 능력 게이팅은 설정 UI 소관이다.
    ...(params.fill ? { fill: params.fill } : {}),
    // 사용 기간 제한은 `previous` 에서만 뜻이 있다. 그 밖의 전략에서 실어 보내면
    // 서버가 무시하긴 하지만, 요청만 보고 동작을 읽을 수 없게 된다.
    ...(params.fill === 'previous' && params.fillPreviousMaxMs
      ? {
          fill_previous_max_ms: params.fillPreviousMaxMs,
          ...(params.fillPreviousOverflow === 'value'
            ? {
                fill_previous_overflow: 'value',
                fill_previous_overflow_value: params.fillPreviousOverflowValue ?? 0,
              }
            : {}),
        }
      : {}),
  };

  const resp = await post<InfluxSeriesQueryRawResponse>(
    `/influxdb/${encodeURIComponent(agentName)}/series/query`,
    body,
    signal ? { signal } : undefined,
  );

  const groups = groupEntriesBySeries(resp?.entries ?? []);
  const out: PivotKeySeries[] = [];
  for (const [, g] of groups) {
    out.push({ labels: g.labels, buckets: collectSeriesBuckets(g.entries) });
  }
  return out;
}

// ---- 매트릭스 조회 ----

/** 매트릭스 조회 파라미터 — 공용 축 + influxdb 전용 bucket. */
export interface TsdbMatrixQuery extends SeriesMatrixQuery {
  /** [influxdb 전용] v2 = bucket, v3 = database. */
  bucket?: string;
  /**
   * 시리즈축 페이지네이션(SPEC-TSDB-004 §2.7.2).
   *
   * `size` 가 0 이하이거나 생략되면 페이지네이션이 비활성이고 그룹 전량을
   * 조회한다 — 저장된 config 의 동작이 변하지 않는다(§2.9 U9).
   */
  groupPage?: { page: number; size: number };
  /** 열거 주입 지점(테스트 seam). 생략 시 실제 D5 라우트를 호출한다. */
  enumerateFn?: (
    agentName: string,
    q: { measurement: string; bucket?: string; tags?: Record<string, string>; startMs: number; endMs: number },
    signal?: AbortSignal,
  ) => Promise<TsdbSeriesEnumResult>;
}

/** group by 항목 1건의 페이지 상황. `index` 는 `params.keys` 의 인덱스다. */
export interface TsdbGroupInfo {
  index: number;
  /** 전체 그룹 수(열거 결과 기준). */
  total: number;
  /** 현재 페이지(0 기반). 페이지네이션 비활성이면 0. */
  page: number;
  /** 전체 페이지 수. 비활성이면 1. */
  pageCount: number;
  /**
   * 열거가 상한에 걸려 잘렸는지. `true` 면 페이지를 전부 넘겨도 일부 그룹에
   * 도달하지 못한다 — UI 는 좁히는 방법을 안내해야 한다(§2.7.4).
   */
  truncated: boolean;
}

/** 실패한 시리즈 1건. `index` 는 `params.keys` 의 인덱스다. */
export interface TsdbSeriesFailure {
  index: number;
  error: unknown;
}

/**
 * 매트릭스 + 부분 실패 신호.
 *
 * 부분 실패를 성공으로 보고하지 않는다(§2.19). 상위(`useTsdbChartData`)는 이를
 * `status:'error'` 가 아닌 별도 신호로 표면화하고, 다음 폴링이 전부 성공하면 해제한다.
 */
export interface TsdbMatrixResult {
  matrix: SeriesMatrix;
  failures: TsdbSeriesFailure[];
  /** group by 항목별 페이지 상황. 그룹 축이 없으면 비어 있다. */
  groups?: TsdbGroupInfo[];
}

/** 취소 사유를 `useStoreChartData` 의 catch 규약(`name === 'AbortError'`)에 맞춘다. */
function abortedError(): Error {
  return new DOMException('요청이 취소되었습니다', 'AbortError');
}

/**
 * 여러 시리즈를 병렬 조회해 하나의 매트릭스로 병합한다(M6.4).
 *
 * - 컬럼 구성 · 0행 자리 보존 · 버킷 합집합 피벗은 `buildSeriesMatrix` 가 소유한다.
 *   이 함수에 남는 것은 "TSDB 에서 키별 시리즈를 어떻게 가져오는가" 하나뿐이다.
 * - `Promise.allSettled` 로 **부분 실패를 격리**한다. 실패한 시리즈는 빈 시리즈 배열이
 *   되어 전 버킷 `null` 컬럼으로 자리를 지키고(UB1-18), 실패 인덱스가 보고된다.
 * - `signal` 은 **모든 개별 요청에 전달**된다(UB1-5). 취소된 조회의 결과는 상태에
 *   반영하지 않으므로 매트릭스를 돌려주지 않고 AbortError 를 던진다(§2.19 #1).
 * - 전 시리즈가 실패하면 그것은 부분 실패가 아니라 **전체 실패**다(§2.14). 첫 오류를
 *   그대로 던져 상위가 오류 오버레이를 띄우고 마지막 성공 렌더를 보존하게 한다.
 */
export async function queryTsdbMatrix(
  agentName: string,
  params: TsdbMatrixQuery,
  signal?: AbortSignal,
): Promise<TsdbMatrixResult> {
  const failures: TsdbSeriesFailure[] = [];

  // 시리즈축 페이지네이션(SPEC-TSDB-004 §2.7.2).
  //
  // 열거는 **페이지를 자를 때만** 필요하다. 페이지네이션이 꺼져 있으면(기본값)
  // 그룹 전량을 어차피 가져오므로 열거할 이유가 없고, 그룹 수는 실제 결과에서
  // 세면 된다. 표시 하나를 위해 폴링마다 요청을 더하고 실패 지점을 늘리는 것은
  // 값을 못 한다.
  //
  // **열거 실패는 시리즈 실패가 아니다.** 열거는 보조 조회이며, 그 실패로 데이터가
  // 사라지면 안 된다. 실패 시 페이지 없이 전량을 조회한다 — 많이 가져오는 것이
  // 아무것도 못 보는 것보다 낫고, 그룹 정보만 비운다.
  const enumerate = params.enumerateFn ?? enumerateTsdbSeries;
  const groups: TsdbGroupInfo[] = [];
  const pageFilters = new Map<number, Array<Record<string, string>>>();
  const pageSize = params.groupPage?.size ?? 0;
  const pageIndex = params.groupPage?.page ?? 0;
  /** group by 축을 가진 인덱스 — 페이지네이션 여부와 무관하게 그룹 수를 센다. */
  const groupedIndexes: number[] = [];

  for (const [idx, key] of params.keys.entries()) {
    const groupBy = params.seriesFilters?.[idx]?.groupBy;
    if (!groupBy || groupBy.length === 0) continue;
    groupedIndexes.push(idx);
    // 사용자가 그룹을 **명시적으로 골랐으면** 그것이 곧 경계다. 페이지로 다시
    // 자르면 고른 것이 안 나오는 상태가 되어 선택이 무의미해진다.
    if ((params.seriesFilters?.[idx]?.groupFilter?.length ?? 0) > 0) continue;
    if (pageSize <= 0) continue;
    try {
      const enumResult = await enumerate(
        agentName,
        {
          measurement: key,
          ...(params.bucket ? { bucket: params.bucket } : {}),
          ...(params.seriesFilters?.[idx]?.tags
            ? { tags: params.seriesFilters[idx]!.tags! }
            : {}),
          startMs: params.startMs,
          endMs: params.endMs,
        },
        signal,
      );
      const combos = deriveGroupCombos(enumResult.series, groupBy);
      groups.push({
        index: idx,
        total: combos.length,
        page: pageIndex,
        pageCount: groupPageCount(combos.length, pageSize),
        truncated: enumResult.truncated,
      });
      pageFilters.set(idx, sliceGroupPage(combos, pageIndex, pageSize));
    } catch {
      // 페이지를 자를 수 없으면 전량을 가져온다. 그룹 정보는 아래에서 실제
      // 결과로부터 채워진다.
    }
  }

  const matrix = await buildSeriesMatrix(params, async (p) => {
    const window: TsdbSeriesWindow = {
      ...(params.bucket ? { bucket: params.bucket } : {}),
      startMs: p.startMs,
      endMs: p.endMs,
      intervalMs: p.intervalMs,
      aggregation: p.aggregation,
      ...(p.fill ? { fill: p.fill } : {}),
      ...(p.fillPreviousMaxMs ? { fillPreviousMaxMs: p.fillPreviousMaxMs } : {}),
      ...(p.fillPreviousOverflow ? { fillPreviousOverflow: p.fillPreviousOverflow } : {}),
      ...(p.fillPreviousOverflowValue !== undefined
        ? { fillPreviousOverflowValue: p.fillPreviousOverflowValue }
        : {}),
    };
    const settled = await Promise.allSettled(
      p.keys.map((key, idx) => {
        const filter = p.seriesFilters?.[idx];
        return fetchTsdbSeries(
          agentName,
          {
            key,
            field: filter?.fieldName ?? '',
            ...(filter?.tags ? { tags: filter.tags } : {}),
            ...(filter?.groupBy ? { groupBy: filter.groupBy } : {}),
            ...(pageFilters.has(idx)
              ? { groupFilter: pageFilters.get(idx)! }
              : filter?.groupFilter
                ? { groupFilter: filter.groupFilter }
                : {}),
          },
          window,
          signal,
        );
      }),
    );
    return settled.map((r, idx) => {
      if (r.status === 'fulfilled') return r.value;
      failures.push({ index: idx, error: r.reason });
      return [];
    });
  });

  if (signal?.aborted) throw abortedError();
  if (params.keys.length > 0 && failures.length === params.keys.length) {
    throw failures[0]!.error;
  }

  // 열거로 채우지 못한 group by 인덱스는 **실제 결과**에서 그룹 수를 센다.
  // 페이지네이션이 꺼져 있으면 전량을 가져왔으므로 이 수가 곧 전체 그룹 수다.
  for (const idx of groupedIndexes) {
    if (groups.some((g) => g.index === idx)) continue;
    const rendered = (matrix.columnOrigins ?? []).filter((o) => o === idx).length;
    groups.push({
      index: idx,
      total: rendered,
      page: 0,
      pageCount: 1,
      truncated: false,
    });
  }
  groups.sort((a, b) => a.index - b.index);

  return { matrix, failures, ...(groups.length > 0 ? { groups } : {}) };
}

/**
 * 백엔드 파생 + 매트릭스 조회를 한 번에 수행한다(M6.5 배선 지점).
 *
 * 불일치일 때 **질의가 나가지 않는다**는 사실이 이 함수의 존재 이유다 — 파생과 조회를
 * 호출부마다 따로 엮으면 어느 한 곳에서 순서가 뒤집혀 조용한 오라우팅이 생긴다.
 */
export async function queryTsdbSourceMatrix(
  ref: TsdbSourceRef,
  agents: readonly TsdbAgentRef[] | undefined,
  params: SeriesMatrixQuery,
  signal?: AbortSignal,
): Promise<TsdbMatrixResult> {
  const resolution = resolveTsdbBackend(ref, agents);
  if (resolution.status === 'mismatch') {
    throw new TsdbBackendMismatchError(resolution.agentName, resolution.agentType);
  }
  return queryTsdbMatrix(
    resolution.agentName,
    { ...params, ...(ref.bucket ? { bucket: ref.bucket } : {}) },
    signal,
  );
}

// ---- 통합 데이터 소스 어댑터 ----

/**
 * measurement 목록 조회 훅.
 *
 * 디스커버리 응답은 캐시하지 않는다(§2.10 · UB1-12) — 스키마는 쓰기와 함께 계속
 * 변하므로, 캐시된 목록은 방금 만든 measurement 를 감춘다. `staleTime: 0` 으로 두어
 * 마운트/재요청마다 다시 읽는다.
 */
function useTsdbMeasurements(
  agentName: string,
  bucket: string | undefined,
  params: { page: number; size: number },
): SeriesKeysQueryResult {
  const query = useQuery<string[], Error>({
    queryKey: ['influxdb', 'measurements', agentName, bucket ?? ''],
    queryFn: () => fetchInfluxMeasurements(agentName, bucket ?? ''),
    enabled: agentName !== '',
    staleTime: 0,
  });

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
 * 에이전트에 바인딩된 TSDB 데이터 소스(M6.6).
 *
 * `useKeys` 가 돌려주는 "키"는 **measurement 목록**이다(D1). 요청 축이
 * `key`(= measurement)이므로 이것이 `SeriesMatrixQuery.keys` 와 정확히 맞물린다.
 *
 * `kind: 'tsdb'` 는 **외부 시계열 DB** 를 뜻한다. 프로세스 내 저장소(memTSDB)는
 * `'memtsdb'` 이며 패널 데이터소스가 아니다(§1.2.1 · UB1-21).
 */
export function tsdbSeriesDataSourceFor(
  agent: TsdbAgentRef,
  options?: { bucket?: string },
): SeriesDataSource {
  const bucket = options?.bucket;
  const ref: TsdbSourceRef = {
    agent_id: agent.id,
    agent_name: agent.name,
    ...(bucket ? { bucket } : {}),
  };
  return {
    kind: 'tsdb',
    useKeys: (params) => useTsdbMeasurements(agent.name, bucket, params),
    queryMatrix: async (params, signal) => {
      const result = await queryTsdbSourceMatrix(ref, [agent], params, signal);
      return result.matrix;
    },
  };
}
