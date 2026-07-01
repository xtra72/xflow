// Store 에이전트 시리즈 매트릭스를 주기적으로 폴링해 차트용 ChartEntry 로 변환하는 훅.
//
// chart-emitter 채널 경로(useChartChannel/useChartChannels)와 공존하는 대체 데이터
// 소스다. 패널은 config.data_source === 'store' 일 때 이 훅을 사용하고, 그 외에는
// 기존 채널 훅을 사용한다. 반환 형상은 채널 훅과 최대한 일치시켜 패널 렌더 코드의
// 변경을 최소화한다(entries / status / closedReason / errorReason).
//
// 매트릭스(SeriesMatrix) → ChartEntry 변환 규칙:
//   - 컬럼(시리즈)마다 행을 순회하며 value=row.values[col], timestamp=bucketStartMs.
//   - flattened `entries`: 모든 시리즈를 timestamp 오름차순으로 평탄화(값이 null 인
//     버킷은 제외). Stat/Bar/Pie/Table 처럼 단일 타임라인을 소비하는 패널이 사용한다.
//     각 entry 는 labels.name = 시리즈 표시 이름을 가져 Bar/Pie 의 카테고리 분리에 쓰인다.
//   - `seriesEntries`: 시리즈 표시 이름 → ChartEntry[](null 포함). LineChart 가 시리즈별
//     라인을 그릴 때 사용한다(connectNulls 로 빈 버킷을 잇는다).
//
// 폴링: refresh_interval_ms(기본 5000ms) 주기로 queryMatrix 를 호출하고, 각 호출은
// AbortController 로 중단 가능하다. 언마운트/설정 변경/비활성화 시 인터벌 정리 + abort.
//
// @spec SPEC-WEB-005

import { useEffect, useMemo, useRef, useState } from 'react';

import type {
  SeriesMatrix,
  SeriesMatrixQuery,
  SeriesSelectorFilter,
} from '@/services/api/seriesDataSource';
import { storeSeriesDataSource } from '@/services/api/store';
import { resolveSeriesAlias } from './aliasTemplate';
import type {
  ChartConnectionStatus,
  ChartEntry,
  StoreSourceConfig,
} from './chartChannelTypes';

/** 기본 폴링 주기(ms). */
const DEFAULT_REFRESH_MS = 5_000;
/** 최소 폴링 주기(ms) — 과도한 요청 방지. */
const MIN_REFRESH_MS = 1_000;

/**
 * 매트릭스 쿼리 실행 함수 시그니처(테스트 주입용).
 * 기본 구현은 `storeSeriesDataSource(agentName).queryMatrix` 를 사용한다.
 */
export type QueryMatrixFn = (
  agentName: string,
  params: SeriesMatrixQuery,
  signal: AbortSignal,
) => Promise<SeriesMatrix>;

/** 훅 옵션(테스트 주입). */
export interface UseStoreChartDataOptions {
  /** 매트릭스 쿼리 실행기(테스트에서 네트워크 없이 주입). */
  queryMatrixFn?: QueryMatrixFn;
  /** 현재 시각 제공기(테스트 결정성 확보). 기본 Date.now. */
  nowFn?: () => number;
}

/**
 * 시리즈별 라인 스타일(LineChart 렌더용). SPEC-WEB-005.
 * store_source.series 의 per-line 스타일을 시리즈 표시 이름 기준으로 노출한다.
 */
export interface StoreSeriesStyle {
  color?: string;
  stroke_style?: 'solid' | 'dashed' | 'dotted';
  stroke_width?: number;
  smooth?: boolean;
}

/** 훅 결과 — 채널 훅과 형상을 맞춘다. */
export interface UseStoreChartDataResult {
  /** 평탄화된 단일 타임라인(값이 null 인 버킷 제외). */
  entries: ChartEntry[];
  /** 시리즈 표시 이름 → 타임라인(null 포함). LineChart 다중 라인용. */
  seriesEntries: Map<string, ChartEntry[]>;
  /** 시리즈 표시 이름 → per-line 스타일. LineChart 가 라인 색/두께/스타일 적용에 사용. */
  seriesStyles: Map<string, StoreSeriesStyle>;
  /** 컬럼(시리즈) 표시 이름 순서(요청/응답 순서 보존). */
  seriesNames: string[];
  /** 연결/조회 상태. idle | connecting | connected | error 만 사용한다. */
  status: ChartConnectionStatus;
  /** error 상태일 때의 사유 메시지. */
  errorReason?: string;
  /** 채널 훅 형상 호환을 위한 필드(Store 소스에서는 항상 undefined). */
  closedReason?: string;
}

/**
 * 기본 매트릭스 쿼리 실행기 — 실제 Store 백엔드를 호출한다.
 */
const defaultQueryMatrix: QueryMatrixFn = (agentName, params, signal) =>
  storeSeriesDataSource(agentName).queryMatrix(params, signal);

/**
 * config.series 를 SeriesMatrixQuery 의 keys/seriesFilters 로 변환한다.
 *
 * - keys: 각 시리즈의 key.
 * - seriesFilters: metric_type/tags 중 하나라도 있으면 SeriesSelectorFilter 를 만들고,
 *   둘 다 없으면 undefined(해당 key 의 모든 시리즈 조회).
 *   필터가 하나도 없으면 seriesFilters 자체를 생략한다(기존 동작 보존).
 */
function buildKeysAndFilters(config: StoreSourceConfig): {
  keys: string[];
  seriesFilters?: Array<SeriesSelectorFilter | undefined>;
} {
  const keys: string[] = [];
  const filters: Array<SeriesSelectorFilter | undefined> = [];
  let anyFilter = false;
  for (const ref of config.series) {
    keys.push(ref.key);
    const hasTags = ref.tags && Object.keys(ref.tags).length > 0;
    if (ref.metric_type || hasTags) {
      anyFilter = true;
      filters.push({
        metricType: ref.metric_type,
        tags: hasTags ? ref.tags : undefined,
      });
    } else {
      filters.push(undefined);
    }
  }
  return anyFilter ? { keys, seriesFilters: filters } : { keys };
}

/**
 * SeriesMatrix 를 시리즈별/평탄화 ChartEntry 로 변환한다.
 *
 * 컬럼 인덱스와 config.series 의 인덱스가 1:1 로 정렬되는 일반적인 경우, 시리즈의
 * alias/color/tags 메타데이터를 컬럼에 매핑한다. queryMatrix 가 한 key 를 다중
 * 시리즈로 확장해 컬럼 수가 더 많아지면, alias 매핑은 생략하고 컬럼명을 그대로 쓴다.
 */
export function matrixToEntries(
  matrix: SeriesMatrix,
  config: StoreSourceConfig,
): {
  entries: ChartEntry[];
  seriesEntries: Map<string, ChartEntry[]>;
  seriesStyles: Map<string, StoreSeriesStyle>;
  seriesNames: string[];
} {
  const seriesEntries = new Map<string, ChartEntry[]>();
  const seriesStyles = new Map<string, StoreSeriesStyle>();
  const flat: ChartEntry[] = [];
  // 컬럼 수가 요청 시리즈 수와 같을 때만 alias/tags/스타일 메타데이터를 정렬 매핑한다.
  const aligned = matrix.columns.length === config.series.length;

  const seriesNames: string[] = matrix.columns.map((colName, j) => {
    const ref = aligned ? config.series[j] : undefined;
    // SPEC-WEB-005: alias 의 `{$.tagKey}` 토큰을 시리즈 태그 값으로 치환한다.
    // 토큰 없는 plain alias 는 그대로 유지된다(하위 호환). 빈 alias 는 컬럼명 폴백.
    if (ref?.alias && ref.alias.trim() !== '') {
      return resolveSeriesAlias(ref.alias, ref.tags ?? {});
    }
    return colName;
  });

  matrix.columns.forEach((_colName, j) => {
    const name = seriesNames[j]!;
    const ref = aligned ? config.series[j] : undefined;
    // 예약 라벨 `name` 은 시리즈 표시 이름이다. 태그에 `name` 키가 있어도 시리즈
    // 이름이 우선하도록 tags 를 먼저 펼친 뒤 name 을 마지막에 둔다(Bar/Pie 카테고리
    // 라벨이 태그 값으로 덮어써지는 문제 방지).
    const baseLabels: Record<string, string> = { ...(ref?.tags ?? {}), name };
    // per-line 스타일을 시리즈 이름 기준으로 노출(LineChart 렌더용). 같은 이름이 둘
    // 이상이면 처음 등장한 시리즈의 스타일을 유지한다.
    if (ref && !seriesStyles.has(name)) {
      seriesStyles.set(name, {
        color: ref.color,
        stroke_style: ref.stroke_style,
        stroke_width: ref.stroke_width,
        smooth: ref.smooth,
      });
    }
    const perSeries: ChartEntry[] = [];
    for (const row of matrix.rows) {
      const v = row.values[j];
      const entry: ChartEntry = {
        timestamp: row.bucketStartMs,
        value: v,
        labels: baseLabels,
        meta: { seriesName: name },
      };
      // 시리즈별 타임라인은 null 도 포함(라인 gap 표현용).
      perSeries.push(entry);
      // 평탄화 타임라인은 숫자 값만 포함(stat/bar/pie/table 오염 방지).
      if (v !== null && Number.isFinite(v)) {
        flat.push(entry);
      }
    }
    // 같은 표시 이름이 둘 이상이면 뒤 시리즈가 앞 시리즈를 덮어쓰지 않도록 병합한다.
    const existing = seriesEntries.get(name);
    if (existing) existing.push(...perSeries);
    else seriesEntries.set(name, perSeries);
  });

  // 평탄화 타임라인을 timestamp 오름차순으로 정렬(여러 시리즈가 섞이므로).
  flat.sort((a, b) => a.timestamp - b.timestamp);

  return { entries: flat, seriesEntries, seriesStyles, seriesNames };
}

const EMPTY_RESULT: UseStoreChartDataResult = {
  entries: [],
  seriesEntries: new Map(),
  seriesStyles: new Map(),
  seriesNames: [],
  status: 'idle',
};

/**
 * Store 소스 차트 데이터 훅.
 *
 * @param config Store 소스 설정. undefined 면 비활성(idle).
 * @param enabled false 면 폴링하지 않고 idle 을 반환한다(패널이 채널 모드일 때).
 * @param options 테스트 주입(queryMatrixFn / nowFn).
 */
export function useStoreChartData(
  config: StoreSourceConfig | undefined,
  enabled: boolean,
  options: UseStoreChartDataOptions = {},
): UseStoreChartDataResult {
  const [result, setResult] = useState<UseStoreChartDataResult>(EMPTY_RESULT);

  // 옵션은 참조만 하므로 ref 로 안정화(불필요한 재구독 방지).
  const optionsRef = useRef(options);
  optionsRef.current = options;

  // 폴링 재시작을 결정하는 키 — config 의 의미있는 필드만 직렬화한다.
  // series 의 순서/필터/시간 파라미터가 바뀌면 재구독한다.
  const pollKey = useMemo(() => {
    if (!enabled || !config) return '';
    if (!config.series || config.series.length === 0) return '';
    if (config.time_window_ms <= 0 || config.interval_ms <= 0) return '';
    const seriesPart = config.series
      .map((s) => {
        const tagPart = s.tags
          ? Object.keys(s.tags)
              .sort()
              .map((k) => `${k}=${s.tags![k]}`)
              .join(',')
          : '';
        return `${s.key}|${s.metric_type ?? ''}|${tagPart}`;
      })
      .join('');
    return [
      config.agent_name,
      config.namespace ?? 'default',
      config.time_window_ms,
      config.interval_ms,
      config.aggregation,
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
      seriesPart,
    ].join('|');
  }, [enabled, config]);

  useEffect(() => {
    // 비활성/무효 설정: idle 로 리셋하고 폴링하지 않는다.
    if (pollKey === '' || !config) {
      setResult(EMPTY_RESULT);
      return;
    }

    let cancelled = false;
    let controller: AbortController | null = null;

    const refreshMs = Math.max(
      MIN_REFRESH_MS,
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
    );

    const run = async () => {
      const queryFn = optionsRef.current.queryMatrixFn ?? defaultQueryMatrix;
      const nowFn = optionsRef.current.nowFn ?? Date.now;
      const now = nowFn();
      const { keys, seriesFilters } = buildKeysAndFilters(config);
      const query: SeriesMatrixQuery = {
        keys,
        ...(seriesFilters ? { seriesFilters } : {}),
        startMs: now - config.time_window_ms,
        endMs: now,
        intervalMs: config.interval_ms,
        aggregation: config.aggregation,
      };
      // 이전 진행 중 요청을 중단하고 새 컨트롤러를 만든다.
      controller?.abort();
      controller = new AbortController();
      const signal = controller.signal;
      try {
        const matrix = await queryFn(config.agent_name, query, signal);
        if (cancelled || signal.aborted) return;
        const converted = matrixToEntries(matrix, config);
        setResult({
          entries: converted.entries,
          seriesEntries: converted.seriesEntries,
          seriesStyles: converted.seriesStyles,
          seriesNames: converted.seriesNames,
          status: 'connected',
        });
      } catch (err) {
        // abort 로 인한 취소는 에러로 보지 않는다.
        if (cancelled || signal.aborted) return;
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setResult((prev) => ({
          ...prev,
          status: 'error',
          errorReason: err instanceof Error ? err.message : String(err),
        }));
      }
    };

    // 즉시 1회 + 인터벌 폴링.
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

  return result;
}
