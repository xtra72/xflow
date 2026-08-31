// 패널 데이터소스 판정 — 소스 종류로 키잉된 단일 조회의 정본.
//
// 패널은 `config.data_source === 'store'` 같은 하드코딩된 동등 비교를 하지 않고
// 이 모듈에 물어본다. 소스 종류가 N개로 늘어도 판정 코드가 "지점 × 종류"로
// 곱해지지 않게 하는 것이 목적이다(spec.md §1.2.5).
//
// **의존성 없는 순수 모듈**이다. React 를 끌어오지 않으므로 UI 없이 전수 테스트가
// 가능하고, `gaugeLegacyBinding.ts` 같은 다른 순수 모듈이 안전하게 import 할 수 있다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) · §2.13 (S1) · §2.17 (UB2)

import type {
  ChartDataSourceKind,
  StoreSourceConfig,
  SysmetricsSourceConfig,
  TsdbSourceConfig,
} from './chartChannelTypes';

// ---- 판정 결과 ----

/** 패널 config 에서 파생한 데이터소스 바인딩. @spec SPEC-TSDB-002 §2.3 */
export interface PanelSourceBinding {
  /** 해석된 소스 종류. 인식 불가 값은 `'channel'` 로 폴백한다. */
  kind: ChartDataSourceKind;
  /** 조회 가능한 상태인가(에이전트/시리즈/필터가 갖춰졌는가). */
  active: boolean;
  /**
   * `data_source` 가 인식 불가 값이어서 channel 로 폴백했는가(§2.17-2).
   *
   * **`data_source` 부재와는 다른 상태다.** 부재는 정상적인 하위 호환 경로이므로
   * 이 플래그가 붙지 않고, 인식 불가 값은 패널에 경고를 표시해야 하므로 붙는다.
   * 두 상태를 같은 값으로 뭉개면 구버전 config 마다 경고가 뜬다.
   */
  unknownKind?: boolean;
}

/** `resolvePanelSourceBinding` 의 선택 인자. */
export interface ResolvePanelSourceOptions {
  /**
   * `config.store_source` 대신 활성 판정에 쓸 **파생 store 소스**.
   *
   * 히트맵 전용 확장 지점이다(plan.md 3.3 — 선택지 b). `HeatmapPanel` 은
   * `config.store_source` 를 그대로 조회하지 않고, 체크된 시리즈만 남기고
   * `selection_mode` 를 `'keys'` 로 강제한 파생 소스(`heatmapStoreSource`)로
   * 조회한다. 그 파생 소스는 **config 키가 아니라 렌더 시점의 useMemo 값**이므로
   * 이 모듈이 config 에서 읽어낼 방법이 없다 — 그래서 호출부가 주입한다.
   *
   * 이 선택지가 동작 보존인 이유: 파생 소스는 (a) `series` 길이가 원본과 같고
   * (b) `selection_mode: 'keys'` · `tag_filters: undefined` 이므로 tag 항이 항상
   * 거짓이 된다. 즉 `heatmapStoreSource.series.length > 0` 이라는 히트맵의 현행
   * 판정과 정확히 같은 값을 낸다. 반대로 이 모듈이 `config.store_source` 를 읽게
   * 하면 tag 모드 히트맵이 새로 활성화되어 동작이 바뀐다.
   */
  storeSourceOverride?: StoreSourceConfig;
}

// ---- 활성 조건 술어 (spec.md §2.3 표의 유일한 정본) ----

/**
 * Store 소스 활성 조건.
 *
 * `series` 를 직접 고른 경우와, `selection_mode: 'tag'` 로 태그 매칭 키를 폴링
 * 시점마다 동적으로 확장하는 경우 둘 다 활성이다. 후자는 `series` 가 비어 있어도
 * 조회할 대상이 있다.
 */
export function isStoreSourceActive(store: StoreSourceConfig | undefined): boolean {
  if (!store) return false;
  const tagActive =
    store.selection_mode === 'tag' && Object.keys(store.tag_filters ?? {}).length > 0;
  return (store.series?.length ?? 0) > 0 || tagActive;
}

/**
 * TSDB 소스 활성 조건 — 에이전트와 시리즈가 **둘 다** 있어야 한다.
 *
 * 에이전트 참조가 없으면 어느 외부 DB 에 연결할지 알 수 없으므로 필수다(§2.18).
 * 태그 기반 동적 바인딩은 제공하지 않으므로(OQ6) Store 와 달리 tag 항이 없다.
 */
export function isTsdbSourceActive(tsdb: TsdbSourceConfig | undefined): boolean {
  if (!tsdb) return false;
  if (typeof tsdb.agent_name !== 'string' || tsdb.agent_name.trim() === '') return false;
  return (tsdb.series?.length ?? 0) > 0;
}

/**
 * sysmetrics 소스 활성 조건 — `agent_id` 와 값이 **둘 다** 있어야 한다.
 *
 * 에이전트가 없으면 어느 호스트의 지표인지 알 수 없고, 값이 없으면 그릴 선이 없다.
 * 대상 목록(interfaces/devices/mountpoints)은 활성 조건이 **아니다** — 빈 배열은
 * "종합 한 줄" 이라는 정상 상태이며, 조건에 넣으면 종합 차트를 만들 수 없다.
 *
 * Store · TSDB 와 달리 **`agent_name` 폴백이 없다.** 이 소스에는 이름만 저장된 구
 * config 가 존재하지 않으며(설정 UI 가 언제나 `agent_id` 를 함께 기록한다), 이름으로
 * 되찾으려면 조회 훅이 에이전트 목록을 읽어야 하는데 그 훅은 소스 종류와 무관하게
 * 항상 호출되는 자리에 있다. 판정과 조회가 같은 기준을 쓰도록 여기서도 id 만 본다.
 */
export function isSysmetricsSourceActive(
  sysmetrics: SysmetricsSourceConfig | undefined,
): boolean {
  if (!sysmetrics) return false;
  if (typeof sysmetrics.agent_id !== 'string' || sysmetrics.agent_id.trim() === '') return false;
  return (sysmetrics.series?.length ?? 0) > 0;
}

// ---- 종류 판정 ----

/** 인식 가능한 `data_source` 값 집합. 이 밖의 문자열은 `unknownKind` 로 폴백한다. */
const KNOWN_KINDS: ReadonlySet<string> = new Set<ChartDataSourceKind>([
  'channel',
  'store',
  'tsdb',
  'sysmetrics',
]);

/**
 * 패널 config 에서 데이터소스 바인딩을 판정한다. O(1) 순수 함수.
 *
 * | `data_source` | 해석 | 활성 조건 |
 * |---------------|------|-----------|
 * | 부재/null | `channel` | 항상 활성(채널 훅이 빈 상태를 스스로 처리) |
 * | `'channel'` | `channel` | 항상 활성 |
 * | `'store'` | `store` | `series` N개 **또는** tag 모드 + `tag_filters` N개 |
 * | `'tsdb'` | `tsdb` | `agent_name` 비어있지 않음 **그리고** `series` N개 |
 * | `'sysmetrics'` | `sysmetrics` | `agent_id` 비어있지 않음 **그리고** `series` N개 |
 * | 인식 불가 | `channel` + `unknownKind` | 항상 활성 |
 *
 * **게이지의 `series_reduce` 논리곱은 여기 포함하지 않는다.** 그것은 소스 활성이
 * 아니라 게이지 고유의 레거시 우선순위 규칙이며 SPEC-CHART-002 §2.9 가 소유한다
 * (`gaugeLegacyBinding.ts`). 여기로 옮기면 게이지가 아닌 패널의 활성 판정까지 바뀐다.
 */
export function resolvePanelSourceBinding(
  config: Record<string, unknown>,
  options?: ResolvePanelSourceOptions,
): PanelSourceBinding {
  const raw = config.data_source;

  // 부재는 하위 호환 경로다 — 인식 불가와 구분해 경고를 붙이지 않는다(§2.4).
  if (raw === undefined || raw === null) {
    return { kind: 'channel', active: true };
  }
  if (typeof raw !== 'string' || !KNOWN_KINDS.has(raw)) {
    return { kind: 'channel', active: true, unknownKind: true };
  }

  const kind = raw as ChartDataSourceKind;
  switch (kind) {
    case 'channel':
      return { kind: 'channel', active: true };
    case 'store': {
      const store =
        options?.storeSourceOverride ??
        (config.store_source as StoreSourceConfig | undefined);
      return { kind: 'store', active: isStoreSourceActive(store) };
    }
    case 'tsdb': {
      // §2.17-1: `tsdb_source` 가 없어도 channel 로 조용히 폴백하지 않는다.
      // 사용자가 명시한 선택을 뒤집으면 토글이 고장난 것으로 보인다.
      const tsdb = config.tsdb_source as TsdbSourceConfig | undefined;
      return { kind: 'tsdb', active: isTsdbSourceActive(tsdb) };
    }
    case 'sysmetrics': {
      // tsdb 와 같은 규칙 — 블록이 없어도 channel 로 되돌리지 않는다.
      const sysmetrics = config.sysmetrics_source as SysmetricsSourceConfig | undefined;
      return { kind: 'sysmetrics', active: isSysmetricsSourceActive(sysmetrics) };
    }
    default:
      return assertNeverKind(kind);
  }
}

/** 종류가 늘었는데 분기를 빠뜨리면 여기서 컴파일이 깨진다. */
function assertNeverKind(kind: never): never {
  throw new Error(`resolvePanelSourceBinding: 알 수 없는 소스 종류 ${String(kind)}`);
}

// ---- 조회 창 (X축 도메인 파생) ----

/**
 * **활성 소스**의 상대 시간 창 길이(ms). 창이 없거나 소스가 채널이면 `undefined`.
 *
 * 세 시리즈 소스가 모두 같은 이름의 필드(`time_window_ms`)를 갖지만, 어느 블록에서
 * 읽을지는 종류가 정한다. 종전에는 라인 차트가 `config.store_source.time_window_ms` 를
 * **하드코딩**해 store 가 아닌 소스에서는 언제나 `undefined` 였고, X축이 데이터 범위
 * (`dataMin`~`dataMax`)로 떨어졌다. TSDB 는 첫 응답에 창 전체가 들어와 티가 나지
 * 않았지만, 라이브로 쌓는 sysmetrics 는 점이 0~1개인 동안 축이 한 점으로 접혀 선이
 * 아예 보이지 않았다.
 *
 * 판정을 여기 두는 이유는 `resolvePanelSourceBinding` 과 같다 — 소스 블록을 어디서
 * 읽을지 아는 곳이 하나여야 종류가 늘 때 지점 × 종류로 곱해지지 않는다.
 */
export function panelSourceWindowMs(
  config: Record<string, unknown>,
  options?: ResolvePanelSourceOptions,
): number | undefined {
  const { kind } = resolvePanelSourceBinding(config, options);
  const block = ((): { time_window_ms?: number } | undefined => {
    switch (kind) {
      case 'channel':
        return undefined;
      case 'store':
        return (options?.storeSourceOverride ??
          (config.store_source as StoreSourceConfig | undefined));
      case 'tsdb':
        return config.tsdb_source as TsdbSourceConfig | undefined;
      case 'sysmetrics':
        return config.sysmetrics_source as SysmetricsSourceConfig | undefined;
      default:
        return assertNeverKind(kind);
    }
  })();
  const ms = block?.time_window_ms;
  return typeof ms === 'number' && Number.isFinite(ms) && ms > 0 ? ms : undefined;
}

// ---- 능력 게이팅 (spec.md §2.13 [S1]) ----

/**
 * 소스별 설정 능력. 미지원 항목은 **숨기지 않고** 비활성 + 사유로 표시한다.
 *
 * 모든 항목이 boolean 이므로 `channel` 은 전부 `false` 다 — 채널 경로에는 조회
 * 파라미터 자체가 없다(표의 `—`).
 */
export interface SourceCapabilities {
  /** 집계 `min`/`max`/`average`. */
  aggregationBasic: boolean;
  /** 집계 `first`/`last`. */
  aggregationFirstLast: boolean;
  /** `first`/`last` 를 백엔드가 직접 계산하는가(false = 클라이언트 집계 폴백). */
  aggregationFirstLastBackend: boolean;
  /** `fill: null|zero|previous`. */
  fillStrategies: boolean;
  /** `fill: 'avg'` — 어느 소스도 지원하지 않는다(§2.7). */
  fillAvg: boolean;
  /** 에이전트 선택 컨트롤 노출. */
  agentSelection: boolean;
  /** 에이전트 선택이 필수인가(미선택 시 소스 비활성). */
  agentRequired: boolean;
  /** bucket/database 선택(v2 는 목록, v3 는 자유 입력 — OQ10). */
  bucketSelection: boolean;
  /** 태그 기반 동적 바인딩(`selection_mode: 'tag'`) — OQ6. */
  tagBinding: boolean;
  /** 시리즈 이름 형식 템플릿. */
  seriesNameFormat: boolean;
  /** `series_reduce`(SPEC-CHART-002) — 소스와 직교한다(§2.4). */
  seriesReduce: boolean;
}

/**
 * 능력 표. `Record<ChartDataSourceKind, SourceCapabilities>` 로 두어 **컴파일러가
 * 3종 전수성을 강제**하게 한다 — 종류가 늘면 여기가 먼저 깨진다(§4.6).
 */
export const PANEL_SOURCE_CAPABILITIES: Record<ChartDataSourceKind, SourceCapabilities> = {
  channel: {
    aggregationBasic: false,
    aggregationFirstLast: false,
    aggregationFirstLastBackend: false,
    fillStrategies: false,
    fillAvg: false,
    agentSelection: false,
    agentRequired: false,
    bucketSelection: false,
    tagBinding: false,
    seriesNameFormat: false,
    seriesReduce: false,
  },
  store: {
    aggregationBasic: true,
    aggregationFirstLast: true,
    // Store 백엔드는 first/last 를 지원하지 않아 클라이언트 집계로 폴백한다.
    aggregationFirstLastBackend: false,
    // Store 백엔드도 fill 을 계산한다 — `/store/{agent}/query` 가 집계 결과의 빈
    // 버킷을 채운다. 직전값 사용 기간 제한 판단은 TSDB 와 같은 정본(fillpolicy)이다.
    fillStrategies: true,
    fillAvg: false,
    agentSelection: true,
    agentRequired: false,
    bucketSelection: false,
    tagBinding: true,
    seriesNameFormat: true,
    seriesReduce: true,
  },
  sysmetrics: {
    // 라이브 누적 소스에는 버킷 집계 축이 없다 — 폴링 주기가 곧 표본 간격이다.
    aggregationBasic: false,
    aggregationFirstLast: false,
    aggregationFirstLastBackend: false,
    // 스냅샷이 오지 않은 구간은 점 자체가 없다. 채울 대상이 없으므로 전략도 없다.
    fillStrategies: false,
    fillAvg: false,
    agentSelection: true,
    // 어느 호스트의 지표인지 알아야 하므로 필수다.
    agentRequired: true,
    bucketSelection: false,
    // 값 카탈로그가 고정이라 태그로 펼칠 축이 없다.
    tagBinding: false,
    seriesNameFormat: true,
    // 구간 대표값은 소스와 직교한다 — 통계·게이지가 누적된 창을 접는 데 쓴다.
    seriesReduce: true,
  },
  tsdb: {
    aggregationBasic: true,
    aggregationFirstLast: true,
    // InfluxDB 는 first/last 를 백엔드에서 직접 지원한다.
    aggregationFirstLastBackend: true,
    fillStrategies: true,
    // Flux · InfluxQL 어느 쪽에도 대응물이 없다 → 400 으로 거부한다(§2.7).
    fillAvg: false,
    agentSelection: true,
    // 어느 외부 DB 에 연결할지 알아야 하므로 필수다(§2.18).
    agentRequired: true,
    bucketSelection: true,
    // 태그 기반 동적 바인딩은 제공하지 않는다(OQ6).
    tagBinding: false,
    seriesNameFormat: true,
    seriesReduce: true,
  },
};

/** 소스 종류의 능력 표를 반환한다. */
export function panelSourceCapabilities(kind: ChartDataSourceKind): SourceCapabilities {
  return PANEL_SOURCE_CAPABILITIES[kind];
}

// ---- 백엔드 버전 축의 능력 (spec.md §2.13 [S1] · §HISTORY-0.4.0 (2)) ----

/**
 * InfluxDB 메이저 버전. 에이전트 config 의 `version` 필드(`'2' | '3'`)와 같은 어휘다
 * (`web/src/config/agentSchemas.ts` 의 `INFLUXDB_FIELDS`).
 *
 * 버전은 **소스 종류가 아니라 백엔드의 하위 축**이다. `ChartDataSourceKind` 를 늘리지
 * 않는 것이 §2.18 의 "백엔드는 파생" 과 일관된다.
 */
export type InfluxBackendVersion = '2' | '3';

/** InfluxDB 버전별 능력. 미지원 항목은 숨기지 않고 비활성 + 사유로 표시한다(§2.13). */
export interface TsdbBackendCapabilities {
  /**
   * bucket 을 **목록에서** 고를 수 있는가.
   *
   * v2 는 `GET /buckets` 로 목록을 준다. v3 는 관리 API 가 없으므로 자유 입력이다(OQ10).
   */
  bucketList: boolean;
  /**
   * 지정한 bucket 이 **질의에 반영되는가**.
   *
   * v3 는 `false` 다 — InfluxQL 템플릿 `SELECT ... FROM "<m>"` 에 database 를 담을
   * 자리가 없고, database 는 클라이언트 연결에 바인딩되어 있다(§HISTORY-0.4.0 (2)).
   * 이 사실을 UI 가 드러내지 않으면 사용자는 값을 바꿔도 결과가 그대로인 이유를 모른다.
   */
  bucketAffectsQuery: boolean;
  /**
   * bucket/measurement **관리** 조작(생성 · 삭제 · truncate).
   *
   * v3 는 `false` — 백엔드가 501 을 반환한다. **디스커버리는 v3 에서도 활성**이므로
   * (§2.10) 이 플래그와 혼동하지 않는다.
   */
  management: boolean;
}

/**
 * 버전별 능력 표. `Record<InfluxBackendVersion, ...>` 로 두어 버전이 늘면 컴파일이
 * 먼저 깨지게 한다 — 소스 종류 축(`PANEL_SOURCE_CAPABILITIES`)과 같은 규율이다.
 */
export const TSDB_BACKEND_CAPABILITIES: Record<
  InfluxBackendVersion,
  TsdbBackendCapabilities
> = {
  '2': { bucketList: true, bucketAffectsQuery: true, management: true },
  '3': { bucketList: false, bucketAffectsQuery: false, management: false },
};

/**
 * 에이전트 config 에서 InfluxDB 버전을 읽는다. 판독 불가면 `'2'` 로 본다.
 *
 * `'2'` 를 기본으로 두는 이유: v2 는 능력이 더 넓으므로(목록 선택 · 관리 가능) 잘못
 * 추정해도 사용자는 "되는 줄 알았는데 서버가 거부" 라는 **드러나는 실패**를 본다.
 * 반대로 v3 로 추정하면 되는 기능을 비활성으로 감춰 **조용한 기능 상실**이 된다.
 */
export function resolveInfluxVersion(
  config: Record<string, unknown> | undefined,
): InfluxBackendVersion {
  return String(config?.version ?? '') === '3' ? '3' : '2';
}

// ---- 미지원 사유 (spec.md §2.13 — "숨기지 않고 비활성 + 사유") ----

/**
 * 능력별 미지원 사유 i18n 키.
 *
 * 사유를 능력과 같은 자리에 두는 이유: 비활성 컨트롤과 그 사유가 따로 관리되면
 * 능력 표만 바뀌고 문구가 남아 **틀린 이유**를 보여주는 상태가 된다. 여기서는 둘이
 * 같은 모듈에 있으므로 한쪽만 고치기 어렵다.
 */
export const CAPABILITY_REASON_KEYS = {
  /** `fill: 'avg'` — Flux · InfluxQL 어느 쪽에도 대응물이 없다(§2.7 → 400). */
  fillAvg: 'dashboard.chart.capReasonFillAvg',
  /** Store 백엔드는 fill 전략 자체를 지원하지 않는다(지정해도 무시). */
  fillStore: 'dashboard.chart.capReasonFillStore',
  /** v3 는 bucket 이 질의에 도달하지 않는다. */
  bucketV3: 'dashboard.chart.capReasonBucketV3',
  /** v3 는 관리 조작이 501 이다(디스커버리는 활성). */
  managementV3: 'dashboard.chart.capReasonManagementV3',
  /** sysmetrics 는 폴링 주기가 곧 표본 간격이라 버킷 집계 축이 없다. */
  aggregationSysmetrics: 'dashboard.chart.capReasonAggSysmetrics',
  /** sysmetrics 는 스냅샷이 없는 구간에 점 자체가 없어 채울 대상이 없다. */
  fillSysmetrics: 'dashboard.chart.capReasonFillSysmetrics',
} as const;
