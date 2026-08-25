// 외부 TSDB 시리즈 매트릭스를 주기적으로 폴링해 차트용 ChartEntry 로 변환하는 훅.
//
// `useStoreChartData` 와 **같은 폴링/재구독/보존 규칙**을 쓴다. 두 훅이 규칙을 달리
// 가지면 소스를 갈아탄 사용자가 같은 설정에서 다른 갱신 거동을 보게 된다.
//
//   - `refresh_interval_ms`(기본 5000ms, 최소 1000ms) 주기 + 즉시 1회
//   - config 의 의미 있는 필드만 직렬화한 `pollKey` 가 바뀔 때만 재구독
//   - 각 조회는 `AbortController` 로 중단 가능하며, 취소된 결과는 상태에 반영하지 않음
//   - 조회 실패는 마지막 성공 렌더를 파괴하지 않고 `status:'error'` 만 갱신(§2.14)
//
// Store 와 다른 것은 둘이다.
//
//   1. **백엔드 파생** — 질의 전에 참조된 에이전트의 실제 타입에서 백엔드를 정한다.
//      지원하지 않는 타입이면 질의하지 않고 불일치 오류를 표시한다(§2.18 · UB2-7).
//   2. **부분 실패 신호** — 시리즈 하나가 실패해도 나머지를 렌더하고 실패 개수를
//      별도 신호로 노출한다. `status:'error'` 로 뭉개지 않는다(§2.19).
//
// 반환 형상은 `UseStoreChartDataResult` 를 확장하므로 6개 패널의 소비 코드가 그대로
// 동작한다 — 이것이 `usePanelSeriesData` 배선의 전제다(§2.3).
//
// @spec SPEC-TSDB-002 §2.14 (S2) · §2.18 (U11) · §2.19 (U12)

import { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { QueryClient, QueryClientContext, useQuery } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';
import type {
  SeriesMatrixQuery,
  SeriesSelectorFilter,
} from '@/services/api/seriesDataSource';
import {
  queryTsdbSourceMatrix,
  type TsdbGroupInfo,
  type TsdbMatrixQuery,
  resolveTsdbBackend,
  TsdbBackendMismatchError,
  type TsdbAgentRef,
  type TsdbMatrixResult,
  type TsdbSourceRef,
} from '@/services/api/tsdbSource';

import type { StoreSourceConfig, TsdbSourceConfig } from './chartChannelTypes';
import {
  matrixToEntries,
  type StoreSeriesStyle,
  type UseStoreChartDataResult,
} from './useStoreChartData';

/** 기본 폴링 주기(ms). Store 와 같은 값을 쓴다. */
const DEFAULT_REFRESH_MS = 5_000;
/** 최소 폴링 주기(ms) — 과도한 요청 방지. */
const MIN_REFRESH_MS = 1_000;

/**
 * 매트릭스 조회 실행 함수 시그니처(테스트 주입용).
 * 기본 구현은 `queryTsdbSourceMatrix` 다.
 */
export type QueryTsdbMatrixFn = (
  ref: TsdbSourceRef,
  agents: readonly TsdbAgentRef[] | undefined,
  params: SeriesMatrixQuery,
  signal: AbortSignal,
) => Promise<TsdbMatrixResult>;

/** 훅 옵션(테스트 주입). */
export interface UseTsdbChartDataOptions {
  /** 매트릭스 조회 실행기(테스트에서 네트워크 없이 주입). */
  queryTsdbFn?: QueryTsdbMatrixFn;
  /** 현재 시각 제공기(테스트 결정성 확보). 기본 Date.now. */
  nowFn?: () => number;
  /**
   * 시리즈축 페이지 커서(SPEC-TSDB-004 §2.7.2).
   *
   * config 가 아니라 옵션인 이유는 이것이 **보기 커서**이기 때문이다 — 페이지를
   * 넘길 때마다 대시보드 config 가 저장되면 안 된다.
   */
  groupPage?: number;
}

/**
 * TSDB 훅 결과 — `UseStoreChartDataResult` + 부분 실패 신호.
 *
 * 확장 인터페이스이므로 `UseStoreChartDataResult` 를 요구하는 자리에 그대로 넣을 수
 * 있다. 패널 렌더 코드가 한 줄도 바뀌지 않는 것이 이 형상의 목적이다.
 */
export interface UseTsdbChartDataResult extends UseStoreChartDataResult {
  /** 이번 조회에서 실패한 시리즈 개수. 0 이면 배지를 표시하지 않는다(§2.17-6). */
  partialFailureCount: number;
  /** 참조된 에이전트가 지원 백엔드가 아닌가(§2.18). 오류 사유 구분용. */
  backendMismatch?: boolean;
  /**
   * group by 항목별 페이지 상황(SPEC-TSDB-004 §2.7).
   * 그룹 축이 없으면 비어 있다.
   */
  groups?: TsdbGroupInfo[];
}

const EMPTY_RESULT: UseTsdbChartDataResult = {
  entries: [],
  seriesEntries: new Map(),
  seriesStyles: new Map(),
  seriesNames: [],
  booleanSeries: new Set(),
  status: 'idle',
  partialFailureCount: 0,
};

/** 기본 조회 실행기 — 백엔드 파생 후 실제 InfluxDB 어댑터를 호출한다. */
const defaultQueryTsdb: QueryTsdbMatrixFn = (ref, agents, params, signal) =>
  queryTsdbSourceMatrix(ref, agents, params, signal);

/**
 * `QueryClientProvider` 가 없을 때 쓰는 **비활성 대체 클라이언트**.
 *
 * 이 훅은 `usePanelSeriesData` 가 소스 종류와 무관하게 **항상** 호출한다(훅 규칙).
 * 그런데 패널 단위 테스트 다수는 `useStoreChartData` 를 모킹해 데이터 계층을 통째로
 * 걷어내고 Provider 없이 렌더한다 — 그 자리에서 react-query 컨텍스트를 요구하면
 * 소스와 무관한 패널 렌더가 전부 깨진다. 컨텍스트가 없으면 이 클라이언트를 쓰되
 * `enabled: false` 로 두어 **어떤 요청도 나가지 않는다**.
 */
let inertClient: QueryClient | undefined;
function inertQueryClient(): QueryClient {
  inertClient ??= new QueryClient();
  return inertClient;
}

/**
 * 백엔드 파생용 에이전트 목록.
 *
 * `useAgents()`(`hooks/useAgent.ts`)와 **같은 queryKey** 를 써서 캐시를 공유한다 —
 * 같은 화면에서 두 번 조회하지 않기 위함이다. 직접 `useAgents()` 를 부르지 않는
 * 이유는 위 `inertQueryClient` 주석의 Provider 부재 상황 하나뿐이다.
 */
function useDerivationAgents(): readonly TsdbAgentRef[] | undefined {
  const client = useContext(QueryClientContext);
  const query = useQuery(
    {
      queryKey: ['agents', undefined],
      queryFn: () => agentService.getAgents(),
      enabled: client !== undefined,
    },
    client ?? inertQueryClient(),
  );
  return query.data?.data;
}

/**
 * config.series 를 `SeriesMatrixQuery` 의 keys/seriesFilters 로 변환한다.
 *
 * Store 와 달리 `field` 가 필수이므로(§2.2) 필터는 **항상** 만들어진다. 필터를 생략해
 * "그 key 의 모든 시리즈"를 조회하는 Store 의 폴백은 TSDB 에 두지 않는다(UB1-4).
 */
function buildKeysAndFilters(config: TsdbSourceConfig): {
  keys: string[];
  seriesFilters: Array<SeriesSelectorFilter | undefined>;
} {
  const keys: string[] = [];
  const seriesFilters: Array<SeriesSelectorFilter | undefined> = [];
  for (const ref of config.series) {
    keys.push(ref.key);
    const hasTags = ref.tags !== undefined && Object.keys(ref.tags).length > 0;
    const hasGroupBy = ref.group_by !== undefined && ref.group_by.length > 0;
    seriesFilters.push({
      fieldName: ref.field,
      ...(hasTags ? { tags: ref.tags } : {}),
      // group by 항목은 이 인덱스가 컬럼 N개로 펼쳐진다(SPEC-TSDB-004 §2.1).
      // 소비자는 SeriesMatrix.columnOrigins 로 출처를 되짚는다.
      ...(hasGroupBy ? { groupBy: ref.group_by } : {}),
      ...(hasGroupBy && ref.group_filter && ref.group_filter.length > 0
        ? { groupFilter: ref.group_filter }
        : {}),
    });
  }
  return { keys, seriesFilters };
}

/**
 * TSDB config 를 `matrixToEntries` 가 읽는 부분만 남긴 형상으로 투영한다.
 *
 * 변환기는 `series`(별칭 · 태그 · 라인 스타일)와 `series_name_format` 만 읽는다.
 * 두 소스의 시리즈 어휘가 같기 때문에(§2.2) 변환기를 한 벌로 쓸 수 있다 —
 * TSDB 전용 변환기를 새로 만들면 표시 이름 규칙이 두 곳으로 갈린다.
 */
function asSeriesConfig(config: TsdbSourceConfig): StoreSourceConfig {
  return {
    agent_name: config.agent_name,
    series: config.series,
    time_window_ms: config.time_window_ms,
    interval_ms: config.interval_ms,
    aggregation: config.aggregation,
    ...(config.series_name_format
      ? { series_name_format: config.series_name_format }
      : {}),
  };
}

/**
 * TSDB 소스 차트 데이터 훅.
 *
 * @param config TSDB 소스 설정. undefined 면 비활성(idle).
 * @param enabled false 면 폴링하지 않고 idle 을 반환한다(패널이 다른 소스일 때).
 * @param options 테스트 주입(queryTsdbFn / nowFn).
 */
export function useTsdbChartData(
  config: TsdbSourceConfig | undefined,
  enabled: boolean,
  options: UseTsdbChartDataOptions = {},
): UseTsdbChartDataResult {
  const [result, setResult] = useState<UseTsdbChartDataResult>(EMPTY_RESULT);

  // 옵션은 참조만 하므로 ref 로 안정화(불필요한 재구독 방지).
  const optionsRef = useRef(options);
  optionsRef.current = options;

  // §2.18: 백엔드는 참조된 에이전트의 실제 타입에서 파생한다. config 에 기록된
  // backend 값은 라우팅에 쓰지 않는다(UB1-22).
  const agents = useDerivationAgents();
  const resolution = config ? resolveTsdbBackend(config, agents) : undefined;

  const agentsRef = useRef(agents);
  agentsRef.current = agents;
  const resolutionRef = useRef(resolution);
  resolutionRef.current = resolution;

  // 파생 결과를 문자열로 접어 pollKey 에 넣는다 — 에이전트 이름이 바뀌거나 타입이
  // 고쳐지면 재구독되어 자동으로 복구된다.
  const resolutionKey = resolution
    ? resolution.status === 'mismatch'
      ? `mismatch:${resolution.agentName}:${resolution.agentType}`
      : `${resolution.status}:${resolution.agentName}`
    : '';

  // 폴링 재시작을 결정하는 키 — config 의 의미있는 필드만 직렬화한다.
  const pollKey = useMemo(() => {
    if (!enabled || !config) return '';
    // 에이전트와 시리즈가 둘 다 있어야 조회 대상이 성립한다(§2.3).
    if (typeof config.agent_name !== 'string' || config.agent_name.trim() === '') return '';
    if (!config.series || config.series.length === 0) return '';
    // 조회 창이 없는 config 는 질의를 만들 수 없다. `<= 0` 이 아니라 `> 0` 의 부정으로
    // 쓰는 이유는 필드 자체가 없는(undefined) 구/부분 config 도 걸러내기 위함이다.
    if (!(config.time_window_ms > 0) || !(config.interval_ms > 0)) return '';

    const selectionPart = config.series
      .map((s) => {
        const tagPart = s.tags
          ? Object.keys(s.tags)
              .sort()
              .map((k) => `${k}=${s.tags![k]}`)
              .join(',')
          : '';
        // 그룹 축도 포함한다(SPEC-TSDB-004 UB1-14) — 빠뜨리면 그룹 기준을 켜도
        // 재구독이 일어나지 않아 차트가 옛 시리즈를 그대로 보여 준다. 사용자에게는
        // "설정은 저장되는데 그림이 안 바뀐다" 로 보인다.
        const groupPart = s.group_by ? [...s.group_by].sort().join(',') : '';
        // 고른 그룹이 바뀌면 재조회해야 한다 — UB1-14 와 같은 사유다.
        const pickPart = s.group_filter
          ? s.group_filter
              .map((c) =>
                Object.keys(c)
                  .sort()
                  .map((k) => `${k}=${c[k]}`)
                  .join(','),
              )
              .sort()
              .join(';')
          : '';
        // alias 도 포함한다 — 이름 편집이 재구독→재변환으로 범례에 반영되게 한다.
        // 그룹별 개별 이름도 같은 사유로 포함한다(§2.12) — 빠뜨리면 그룹 이름을
        // 고쳐도 범례가 그대로다.
        const mapPart = (m: Record<string, string> | undefined): string =>
          m
            ? Object.keys(m)
                .sort()
                .map((k) => `${k}=${m[k]}`)
                .join(';')
            : '';
        // 그룹별 색도 같은 사유로 포함한다(§2.14).
        return [
          s.key,
          s.field,
          tagPart,
          groupPart,
          pickPart,
          s.alias ?? '',
          mapPart(s.group_alias),
          s.color ?? '',
          mapPart(s.group_color),
        ].join('|');
      })
      .join('');

    return [
      resolutionKey,
      config.bucket ?? '',
      // 페이지 크기가 바뀌면 조회 형태가 달라진다(열거 유무 · group_filter).
      config.group_page_size ?? 0,
      config.time_window_ms,
      config.interval_ms,
      config.aggregation,
      config.fill ?? '',
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
      // 패널 단위 이름 형식도 재구독 축이다. 빠뜨리면 형식을 고쳐도 범례가
      // 그대로다 — selectionPart 의 alias 와 같은 사유(UB1-14 계열)이며,
      // 이름은 시리즈별(alias)과 패널별(format) 두 축에서 온다.
      config.series_name_format ?? '',
      selectionPart,
    ].join('|');
  }, [enabled, config, resolutionKey]);

  useEffect(() => {
    // 비활성/무효 설정: idle 로 리셋하고 폴링하지 않는다.
    if (pollKey === '' || !config) {
      setResult(EMPTY_RESULT);
      return;
    }

    // §2.17-7: 지원하지 않는 백엔드면 질의 자체를 하지 않는다. 폴링 타이머도 걸지
    // 않는다 — 고쳐지지 않을 요청을 5초마다 반복하는 것은 소음일 뿐이고, 에이전트가
    // 교체되면 resolutionKey 가 바뀌어 이 effect 가 다시 돈다.
    const current = resolutionRef.current;
    if (current && current.status === 'mismatch') {
      const err = new TsdbBackendMismatchError(current.agentName, current.agentType);
      setResult((prev) => ({
        ...prev,
        status: 'error',
        errorReason: err.message,
        backendMismatch: true,
      }));
      return;
    }

    let cancelled = false;
    let controller: AbortController | null = null;

    const refreshMs = Math.max(
      MIN_REFRESH_MS,
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
    );

    const run = async () => {
      const queryFn = optionsRef.current.queryTsdbFn ?? defaultQueryTsdb;
      const nowFn = optionsRef.current.nowFn ?? Date.now;
      const now = nowFn();
      // 이전 진행 중 요청을 중단하고 새 컨트롤러를 만든다(§2.19 #1).
      controller?.abort();
      controller = new AbortController();
      const signal = controller.signal;

      const { keys, seriesFilters } = buildKeysAndFilters(config);
      const query: TsdbMatrixQuery = {
        keys,
        seriesFilters,
        startMs: now - config.time_window_ms,
        endMs: now,
        intervalMs: config.interval_ms,
        aggregation: config.aggregation,
        ...(config.fill ? { fill: config.fill } : {}),
      };
      // 페이지 크기가 0 이면 페이지네이션 비활성 — 그룹 전량을 조회한다.
      // 저장된 config 에 group_page_size 가 없으면 이 경로가 그대로 현행이다(§2.9).
      const groupPageSize = config.group_page_size ?? 0;
      const ref: TsdbSourceRef = {
        ...(config.agent_id ? { agent_id: config.agent_id } : {}),
        agent_name: config.agent_name,
        ...(config.bucket ? { bucket: config.bucket } : {}),
      };

      try {
        const matrixResult = await queryFn(
          ref,
          agentsRef.current,
          groupPageSize > 0
            ? {
                ...query,
                groupPage: { page: optionsRef.current.groupPage ?? 0, size: groupPageSize },
              }
            : query,
          signal,
        );
        if (cancelled || signal.aborted) return;
        const converted = matrixToEntries(matrixResult.matrix, asSeriesConfig(config));
        setResult({
          entries: converted.entries,
          seriesEntries: converted.seriesEntries,
          seriesStyles: converted.seriesStyles,
          seriesNames: converted.seriesNames,
          booleanSeries: converted.booleanSeries,
          status: 'connected',
          // 전부 성공하면 배지가 사라진다(§2.17-6).
          partialFailureCount: matrixResult.failures.length,
          ...(matrixResult.groups ? { groups: matrixResult.groups } : {}),
        });
      } catch (err) {
        // abort 로 인한 취소는 에러로 보지 않는다.
        if (cancelled || signal.aborted) return;
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setResult((prev) => ({
          ...prev,
          status: 'error',
          errorReason: err instanceof Error ? err.message : String(err),
          backendMismatch: err instanceof TsdbBackendMismatchError,
        }));
      }
    };

    // 즉시 1회 + 인터벌 폴링. 재시도 간격이 폴링 주기를 넘지 않는다(§2.17-3).
    setResult((prev) => ({ ...prev, status: 'connecting' }));
    void run();
    const intervalId = window.setInterval(() => {
      void run();
    }, refreshMs);

    return () => {
      cancelled = true;
      controller?.abort();
      window.clearInterval(intervalId);
    };
    // pollKey 가 변경될 때만 재구독한다(config 객체 참조 변화는 무시).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pollKey]);

  // per-line 스타일은 순수 config 표현값이므로 재조회 없이 즉시 반영되어야 한다.
  // pollKey 는 스타일만 바뀐 편집(예: 곡선 토글)을 감지하지 않으므로, 마지막으로 알려진
  // 시리즈 이름에 현재 config 의 스타일을 재매핑한다(`useStoreChartData` 와 같은 규칙).
  const reactiveSeriesStyles = useMemo(() => {
    if (!config?.series || result.seriesNames.length !== config.series.length) {
      return result.seriesStyles;
    }
    const styles = new Map<string, StoreSeriesStyle>();
    result.seriesNames.forEach((name, j) => {
      const ref = config.series[j];
      if (ref && !styles.has(name)) {
        styles.set(name, {
          color: ref.color,
          stroke_style: ref.stroke_style,
          stroke_width: ref.stroke_width,
          smooth: ref.smooth,
        });
      }
    });
    return styles;
  }, [config, result.seriesNames, result.seriesStyles]);

  return { ...result, seriesStyles: reactiveSeriesStyles };
}
