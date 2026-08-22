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

// ---- 종류 판정 ----

/** 인식 가능한 `data_source` 값 집합. 이 밖의 문자열은 `unknownKind` 로 폴백한다. */
const KNOWN_KINDS: ReadonlySet<string> = new Set<ChartDataSourceKind>([
  'channel',
  'store',
  'tsdb',
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
    default:
      return assertNeverKind(kind);
  }
}

/** 종류가 늘었는데 분기를 빠뜨리면 여기서 컴파일이 깨진다. */
function assertNeverKind(kind: never): never {
  throw new Error(`resolvePanelSourceBinding: 알 수 없는 소스 종류 ${String(kind)}`);
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
    // Store 백엔드는 fill 전략을 지원하지 않는다(지정해도 무시됨).
    fillStrategies: false,
    fillAvg: false,
    agentSelection: true,
    agentRequired: false,
    bucketSelection: false,
    tagBinding: true,
    seriesNameFormat: true,
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
