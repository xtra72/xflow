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

  // 종류를 정할 수 없으면 **store** 로 접는다.
  //
  // 종전에는 `channel` 이 그 자리였다. 채널이 패널 소스에서 빠지면서 폴백도 옮겨야 하는데,
  // store 를 고른 이유는 두 가지다. (1) 신규 패널이 이미 store 로 태어난다
  // (`uiStore.createDefaultPanel`). (2) 채널 모드로 저장된 패널은 설정을 열 때 store 로
  // 이관되므로(`SERIES_SOURCE_PANEL_TYPES`), 폴백과 이관 대상이 같아야 두 경로가 갈리지 않는다.
  //
  // **활성 판정을 함께 바꿨다.** 채널 폴백은 언제나 활성(`active: true`)이었지만 — 채널 훅이
  // 빈 상태를 스스로 처리했으므로 — store 는 시리즈를 골라야 활성이다. 활성을 그대로 참으로
  // 두면 고른 것이 없는 패널이 "조회 중" 으로 보인다.
  const storeFallback = () => {
    const store =
      options?.storeSourceOverride ?? (config.store_source as StoreSourceConfig | undefined);
    return isStoreSourceActive(store);
  };

  // 부재는 하위 호환 경로다 — 인식 불가와 구분해 경고를 붙이지 않는다(§2.4).
  if (raw === undefined || raw === null) {
    return { kind: 'store', active: storeFallback() };
  }
  if (typeof raw !== 'string' || !KNOWN_KINDS.has(raw)) {
    // 없어진 `'channel'` 과 손상된 값을 가른다.
    //
    //   - `'channel'`: 사용자가 고른 적 없는 값이 아니라 **폐지된 값**이다. 경고를 붙이지
    //     않고, 갖춰진 store 블록이 있으면 그대로 조회한다(구 패널의 구제 경로).
    //   - 그 밖(오타·손상): 경고를 붙이고 **조회하지 않는다**. 알 수 없는 값에서 조용히
    //     질의를 내면 사용자가 의도한 적 없는 조회가 돈다.
    if (raw === 'channel') return { kind: 'store', active: storeFallback() };
    return { kind: 'store', active: false, unknownKind: true };
  }

  const kind = raw as ChartDataSourceKind;
  switch (kind) {
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
 * 전수성을 강제**하게 한다 — 종류가 늘면 여기가 먼저 깨진다(§4.6).
 */
export const PANEL_SOURCE_CAPABILITIES: Record<ChartDataSourceKind, SourceCapabilities> = {
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

// ---- 다중 소스 ----

/**
 * 소스 **인스턴스** 하나.
 *
 * 종류 목록(`data_sources: ['store','tsdb']`)으로는 같은 종류를 둘 이상 쓸 수 없다 —
 * Store 에이전트 A 와 B 를 한 차트에 겹쳐 보는 것이 표현되지 않는다. 그래서 저장 단위를
 * **종류가 아니라 인스턴스**로 둔다.
 *
 * 설정 블록의 키 이름을 패널 최상위와 **같게** 둔 것이 하중 지지점이다. 그 덕에 인스턴스
 * 하나가 "작은 패널 config" 처럼 생겨, 종류·활성 판정을 단일 경로
 * (`resolvePanelSourceBinding`)에 그대로 위임할 수 있다 — 판정 규칙을 두 벌로 두면 갈린다.
 */
export interface PanelSourceEntry {
  kind: ChartDataSourceKind;
  store_source?: StoreSourceConfig;
  tsdb_source?: TsdbSourceConfig;
  sysmetrics_source?: SysmetricsSourceConfig;
}

/**
 * 한 종류가 가질 수 있는 인스턴스 수의 상한.
 *
 * 상한이 필요한 이유는 React 훅이다 — 조회 훅은 렌더마다 **같은 횟수** 불려야 하므로,
 * 인스턴스 수만큼 반복 호출할 수 없다. 고정 슬롯을 미리 잡고 남는 자리를 idle 로 둔다.
 * 넘치는 인스턴스는 조용히 버리지 않고 목록에서 잘라내며, 그 사실이 설정 화면에 보인다.
 */
export const MAX_SOURCES_PER_KIND = 4;

/**
 * 패널이 쓰는 소스 인스턴스 목록을 읽는다.
 *
 * 세 세대의 저장 형상을 모두 읽는다 — 저장된 대시보드가 그대로 동작해야 한다.
 *
 *   1. `sources: [{kind, ...블록}]`  — 현재 형상(같은 종류 여럿 가능)
 *   2. `data_sources: [kind, ...]`   — 종류 목록(종류당 하나)
 *   3. `data_source: kind`           — 단일 축
 *
 * 위에서부터 먼저 잡히는 것을 쓴다. 2·3 은 패널 최상위 블록을 인스턴스로 옮겨 담는다.
 */
export function readPanelSources(config: Record<string, unknown>): PanelSourceEntry[] {
  const raw = config.sources;
  if (Array.isArray(raw)) {
    const out: PanelSourceEntry[] = [];
    const perKind = new Map<ChartDataSourceKind, number>();
    for (const v of raw) {
      const kind = (v as PanelSourceEntry | undefined)?.kind;
      if (typeof kind !== 'string' || !KNOWN_KINDS.has(kind)) continue;
      const used = perKind.get(kind) ?? 0;
      if (used >= MAX_SOURCES_PER_KIND) continue;
      perKind.set(kind, used + 1);
      out.push(v as PanelSourceEntry);
    }
    if (out.length > 0) return out;
  }

  // 구 형상 — 종류마다 패널 최상위 블록을 하나씩 옮겨 담는다.
  const kinds = readPanelSourceKinds(config);
  const legacy = kinds.length > 0 ? kinds : [resolvePanelSourceBinding(config).kind];
  return legacy.map((kind) => ({
    kind,
    store_source: config.store_source as StoreSourceConfig | undefined,
    tsdb_source: config.tsdb_source as TsdbSourceConfig | undefined,
    sysmetrics_source: config.sysmetrics_source as SysmetricsSourceConfig | undefined,
  }));
}

/**
 * 인스턴스마다 종류·활성을 판정한다.
 *
 * 판정 자체는 단일 경로를 그대로 쓴다 — 인스턴스가 "작은 패널 config" 형상이라 그대로
 * 넘길 수 있다. 활성 조건을 여기서 다시 적으면 단일 소스와 다중 소스가 갈린다.
 */
export function resolvePanelSourceEntries(
  config: Record<string, unknown>,
  options?: ResolvePanelSourceOptions,
): Array<{ entry: PanelSourceEntry; binding: PanelSourceBinding }> {
  const entries = readPanelSources(config);

  // 단일 축(구 형상)은 **패널 자신의 판정을 그대로** 쓴다.
  //
  // 인스턴스로 옮겨 담은 뒤 다시 판정하면 그 과정에서 `data_source` 원값이 사라진다.
  // 인식 불가 문자열은 "store 로 접되 조회하지 않는다" 인데, 옮겨 담은 인스턴스는 종류가
  // 이미 `'store'` 라 그 사연을 잃고 활성으로 살아난다 — 사용자가 고른 적 없는 조회가 돈다.
  if (!Array.isArray(config.sources) && readPanelSourceKinds(config).length === 0) {
    return [{ entry: entries[0]!, binding: resolvePanelSourceBinding(config, options) }];
  }

  return entries.map((entry) => ({
    entry,
    binding: resolvePanelSourceBinding({ ...entry, data_source: entry.kind }, options),
  }));
}

/**
 * 패널이 **동시에** 쓰는 소스 종류 목록. 인스턴스 목록에서 종류만 뽑아 중복을 없앤다.
 *
 * 설정 화면의 종류 토글이 이 값을 읽는다 — 같은 종류를 둘 이상 켜도 토글은 하나다.
 */
/**
 * 이 패널에 **활성 시리즈 소스가 하나라도 있는가**.
 *
 * 활성 판정을 쓰는 자리(패널 렌더 분기, 설정 화면 미리보기 게이트)는 모두 이 술어를 쓴다.
 * `resolvePanelSourceBinding` 은 단일 축 해석기라 최상위 `store_source` / `tsdb_source` 만
 * 본다 — 소스가 목록(`sources[]`)으로 옮겨간 뒤로 그 자리에서 활성을 판정하면, 고른 시리즈가
 * 목록 쪽에만 있는 패널이 영원히 비활성이 된다(설정에서 시리즈를 골라도 미리보기가 합성
 * 샘플에 머무는 증상).
 *
 * 목록이 없는 구 패널은 `resolvePanelSourceEntries` 가 단일 축으로 되돌려 주므로 판정이
 * 종전과 같다.
 */
export function isPanelSeriesActive(
  config: Record<string, unknown>,
  options?: ResolvePanelSourceOptions,
): boolean {
  return resolvePanelSourceEntries(config, options).some(({ binding }) => binding.active);
}

export function resolvePanelSourceBindings(
  config: Record<string, unknown>,
  options?: ResolvePanelSourceOptions,
): PanelSourceBinding[] {
  const seen = new Set<ChartDataSourceKind>();
  const out: PanelSourceBinding[] = [];
  for (const { binding } of resolvePanelSourceEntries(config, options)) {
    if (seen.has(binding.kind)) {
      // 같은 종류가 여럿이면 **하나라도 활성이면 활성**이다 — 토글 하나가 그 종류 전체를
      // 대표하므로, 첫 인스턴스가 비활성이라고 종류 전체를 꺼진 것으로 보이면 안 된다.
      const prev = out.find((b) => b.kind === binding.kind)!;
      if (binding.active) prev.active = true;
      continue;
    }
    seen.add(binding.kind);
    out.push({ ...binding });
  }
  return out;
}

/**
 * `data_sources` 를 읽어 정규화한다. 필드가 없거나 쓸 값이 하나도 없으면 빈 배열이다
 * (호출부는 그때 단일 축으로 되돌아간다).
 */
export function readPanelSourceKinds(
  config: Record<string, unknown>,
): ChartDataSourceKind[] {
  const raw = config.data_sources;
  if (!Array.isArray(raw)) return [];
  const out: ChartDataSourceKind[] = [];
  for (const v of raw) {
    if (typeof v !== 'string' || !KNOWN_KINDS.has(v)) continue;
    const kind = v as ChartDataSourceKind;
    if (!out.includes(kind)) out.push(kind);
  }
  return out;
}

/** 소스 종류 → 사람이 읽을 이름. 시리즈 이름이 겹칠 때 꼬리표로 쓴다. */
export const SOURCE_LABELS: Record<ChartDataSourceKind, string> = {
  store: 'Store',
  tsdb: 'TSDB',
  sysmetrics: '시스템 지표',
};

/**
 * 인스턴스의 꼬리표. 같은 종류가 여럿이면 **몇 번째인지**까지 붙인다.
 *
 * `Store` 두 개가 같은 시리즈 이름을 내면 꼬리표까지 같아 구분이 되지 않는다. 종류 안에서의
 * 순번을 붙여야 두 줄이 화면에서 갈린다.
 */
export function sourceEntryLabel(
  entries: readonly PanelSourceEntry[],
  index: number,
): string {
  const kind = entries[index]!.kind;
  const sameKind = entries.filter((e) => e.kind === kind);
  if (sameKind.length <= 1) return SOURCE_LABELS[kind];
  const ordinal = entries.slice(0, index + 1).filter((e) => e.kind === kind).length;
  return `${SOURCE_LABELS[kind]} ${ordinal}`;
}

/**
 * 인스턴스 하나의 블록을 갈아 끼우는 **config 패치**를 만든다.
 *
 * 설정 화면 여러 곳(소스 편집기 · 시리즈 선택 표)이 같은 인스턴스를 고치므로, 되쓰기
 * 규칙을 한자리에 둔다 — 두 벌로 두면 한쪽만 `sources` 를 보고 다른 쪽은 최상위 블록을
 * 고쳐, 둘째 인스턴스의 편집이 첫째에 반영되는 식으로 어긋난다.
 *
 * 구 형상(아직 `sources` 가 없음)이면 최상위 블록을 그대로 고친다 — 인스턴스가 하나뿐이라
 * 그 둘이 같은 자리다.
 */
export function sourceEntryPatch(
  config: Record<string, unknown>,
  index: number,
  patch: Record<string, unknown>,
): Record<string, unknown> {
  if (!Array.isArray(config.sources)) return patch;
  const entries = readPanelSources(config);
  // 소스 블록만 인스턴스로 보낸다. 시리즈 표는 한 번의 패치에 소스 블록과 패널 단위 값
  // (히트맵 `sensor_positions`)을 함께 담으므로, 통째로 밀어 넣으면 그 값이 `sources[i]`
  // 밑에 묻혀 패널이 읽지 못한다.
  const inner: Record<string, unknown> = {};
  const outer: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(patch)) {
    if (SOURCE_ENTRY_KEYS.has(k)) inner[k] = v;
    else outer[k] = v;
  }
  return {
    ...outer,
    sources: entries.map((e, i) => (i === index ? ({ ...e, ...inner } as PanelSourceEntry) : e)),
  };
}

/** 인스턴스가 소유하는 키 — 그 밖의 키는 패널 단위 값이므로 최상위에 남는다. */
const SOURCE_ENTRY_KEYS: ReadonlySet<string> = new Set([
  'kind',
  'store_source',
  'tsdb_source',
  'sysmetrics_source',
]);
