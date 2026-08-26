// 게이지 값 소스 판정 + 레거시 바인딩 이관 변환 (순수 로직).
//
// @spec SPEC-CHART-002 §2.8 [E2] · §2.9 [S1] · §4.5 — M5.1 / M5.3
//
// 게이지는 두 개의 값 공급 경로를 **병행**한다.
//
//   - `legacy`       : `config.dataSources[]` (chart-emitter 구독 · store latest 폴링 ·
//                      바인딩 없으면 static `config.value`). SPEC-CHART-002 이전부터
//                      존재하며 본 SPEC 은 이를 폐기하지 않는다(§7 OQ4).
//   - `store-source` : 다른 차트 패널과 공용인 `config.store_source` + `series_reduce`
//                      경로. M4 가 신설했다.
//
// 이 모듈은 **어느 경로가 이기는가**만 판정한다. 값 해석 자체는 `GaugePanel.tsx` 가
// 갖는다. 판정을 컴포넌트 밖으로 뽑는 이유는 게이지의 값 해석 경로가 7지점에 분산되어
// 있어(plan.md §4 위험 표) 분기 조건을 인라인으로 두면 조용한 회귀를 만들기 때문이다.
// 순수 함수로 두면 진리표를 전수 테스트할 수 있다(`gaugeLegacyBinding.test.ts`).
//
// React·DOM 의존이 없어야 한다 — 이 모듈은 순수 단위 테스트 대상이다.

import type {
  ChartDataSourceKind,
  SeriesReduceFunc,
  StoreSourceConfig,
  TsdbSourceConfig,
} from './chartChannelTypes';
import { DEFAULT_STORE_SOURCE_WINDOW } from './chartChannelTypes';
import {
  isStoreSourceActive,
  isTsdbSourceActive,
  resolvePanelSourceBinding,
} from './panelDataSource';

// ---- 레거시 바인딩 형상 ----

/**
 * `config.dataSources[]` 한 항목의 형상.
 *
 * `GaugePanel.tsx` 의 `GaugeDataSource` · `PanelSettingsDialog.tsx` 의
 * `DataSourceBinding` 과 구조적으로 동일하다(TypeScript 구조적 타이핑으로 상호
 * 대입 가능). 본 모듈이 순수(React 무의존)를 유지해야 해서 여기에 따로 둔다.
 *
 * `resource` / `flow` 는 타입에 선언만 되어 있고 **읽는 코드가 없다** — 해당
 * 바인딩만 가진 게이지는 static `config.value` 를 렌더한다(spec.md §6-5 v0.3.0
 * 정정, 특성화 CH-11 · CH-12). 본 SPEC 은 그 상태를 바꾸지 않는다.
 */
export interface GaugeLegacyDataSource {
  sourceType: 'resource' | 'flow' | 'chart-emitter' | 'store';
  resource?: string;
  flowId?: string;
  dataField?: string;
  channelName?: string;
  displayField?: string;
  /** store 소스 전용: Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  storeAgentId?: string;
  /** store 소스 전용: Store 에이전트 이름(표시/폴백). @spec SPEC-WEB-006 */
  storeAgent?: string;
  storeKey?: string;
  storeNamespace?: string;
}

// ---- 값 소스 판정 ----

/**
 * 게이지가 실제로 값을 읽어 올 경로.
 *
 * `'store-source'` 는 **공용 시리즈 소스 경로 전체**를 가리킨다 — Store 와 TSDB 둘 다다.
 * 이름이 `store-` 인 것은 그 경로가 `store_source` 하나뿐이던 시절의 잔재이며, 리터럴을
 * 바꾸면 이 값을 비교하는 두 호출부(`GaugePanel` · `PanelSettingsDialog`)와 진리표
 * 테스트가 함께 흔들리므로 그대로 둔다. 판정이 보는 것은 "채널이 아닌 소스가 값을 낼 수
 * 있는가" 이지 소스 종류가 무엇인가가 아니다.
 */
export type GaugeValueSource = 'store-source' | 'legacy';

/**
 * 판정에 필요한 config 파생 플래그 3종.
 *
 * 진리표의 "레거시 `dataSources`" 열은 **의도적으로 여기에 없다.** 7행 어느 쌍에서도
 * 레거시 유무가 선택 경로를 뒤집지 않기 때문이다 — 레거시가 없으면 `legacy` 경로가
 * static `config.value` 로 떨어질 뿐 경로 자체는 바뀌지 않는다. 판정 입력에 넣으면
 * "언젠가 영향을 줄 것"이라는 잘못된 신호가 된다.
 */
export interface GaugeValueSourceFlags {
  /** `config.data_source` 원값. 미지정(undefined)은 `'channel'` 로 해석된다. */
  dataSource: ChartDataSourceKind | undefined;
  /** `config.store_source` 가 실제로 데이터를 낼 수 있는 상태인가. */
  storeSourceActive: boolean;
  /**
   * `config.tsdb_source` 가 실제로 데이터를 낼 수 있는 상태인가.
   *
   * store 와 **별도 항**인 이유: 두 블록은 공존할 수 있고(소스를 오가며 설정이 남는다),
   * 어느 쪽이 값을 내는지는 `dataSource` 가 정한다. 하나로 합치면 TSDB 를 고른 게이지가
   * 남아 있는 store 설정 때문에 활성으로 잘못 판정된다.
   */
  tsdbSourceActive: boolean;
  /** `config.series_reduce` 가 지정되어 있는가(값이 아니라 **유무**가 스위치다). */
  hasSeriesReduce: boolean;
}

/**
 * `store_source` 활성 판정 — 기존 패널(`LineChartPanel`)과 **동일한 규칙**이다.
 *
 * - `tag` 모드: `tag_filters` 가 1개 이상이면 활성. 이 모드는 `series[]` 가 비어 있고
 *   폴링 시점마다 키를 동적 해석하므로 시리즈 길이로 판정할 수 없다(SPEC-WEB-005).
 * - 그 외(`keys` 모드 · `selection_mode` 미지정 구 config): `series[]` 가 1개 이상이면 활성.
 *
 * `data_source` 는 여기에 포함하지 않는다 — 그것은 진리표의 **별도 축**이다.
 */
export function isGaugeStoreSourceActive(store: StoreSourceConfig | undefined): boolean {
  // SPEC-TSDB-002 §2.3 [U3]: 활성 조건의 정본은 `panelDataSource` 하나뿐이다.
  // 여기서 같은 식을 한 벌 더 두면 두 곳이 갈릴 수 있다. 이름은 호출부 호환을 위해 남긴다.
  return isStoreSourceActive(store);
}

/** 패널 config 에서 판정 입력 플래그를 파생한다. */
export function gaugeValueSourceFlags(
  config: Record<string, unknown>,
): GaugeValueSourceFlags {
  return {
    dataSource: config.data_source as ChartDataSourceKind | undefined,
    storeSourceActive: isGaugeStoreSourceActive(
      config.store_source as StoreSourceConfig | undefined,
    ),
    tsdbSourceActive: isTsdbSourceActive(
      config.tsdb_source as TsdbSourceConfig | undefined,
    ),
    hasSeriesReduce: (config.series_reduce as SeriesReduceFunc | undefined) !== undefined,
  };
}

/**
 * 값 소스 판정 — plan.md M5 판정 진리표의 단일 정본.
 *
 * | `data_source` | 해당 소스 활성 | `series_reduce` | 결과 |
 * |---------------|----------------|-----------------|------|
 * | 미지정 / `'channel'` | — | — | `legacy` |
 * | `'store'` / `'tsdb'` | 비활성 | — | `legacy` |
 * | `'store'` / `'tsdb'` | 활성 | 부재 | `legacy` |
 * | `'store'` / `'tsdb'` | 활성 | 있음 | `store-source` |
 *
 * 세 조건의 논리곱이며, **신규 경로가 실제로 값을 낼 수 있을 때만** 레거시를
 * 밀어낸다(§2.9 [S1] 게이지 추가 조건 / §4.5). `data_source` 가 채널이 아니라는 것은
 * 사용자가 토글로 명시한 상태이므로, 그 상태에서 신규 경로가 준비되어 있으면 레거시
 * `chart-emitter` 가 계속 이겨서는 안 된다 — 그러면 토글이 고장난 것으로 보인다.
 *
 * TSDB 행은 store 행과 **완전히 같은 모양**이다. 게이지 고유의 규칙은 `series_reduce`
 * 논리곱 하나뿐이고 그것은 소스 종류와 직교하므로, 종류마다 조건을 덧붙이지 않고
 * "채널이 아닌 활성 소스" 로 한 번에 받는다.
 */
export function resolveGaugeValueSource(flags: GaugeValueSourceFlags): GaugeValueSource {
  // SPEC-TSDB-002 §2.3 [U3]: 소스 종류 판정(미지정·인식 불가 → channel 폴백 포함)은
  // 계약에 위임한다. `flags` 는 종류 원값만 갖고 소스 블록을 갖지 않으므로
  // `data_source` 만 담은 config 로 **종류**를 받고, 활성 항은 종류별 플래그
  // (= 계약의 `isStoreSourceActive` / `isTsdbSourceActive`) 를 그대로 쓴다.
  //
  // **`hasSeriesReduce` 논리곱은 여기 남는다.** 그것은 소스 활성이 아니라 게이지 고유의
  // 레거시 우선순위 규칙이며 SPEC-CHART-002 §2.9 가 소유한다. `panelDataSource` 로
  // 옮기면 게이지가 아닌 패널의 활성 판정까지 바뀐다.
  const { kind } = resolvePanelSourceBinding({ data_source: flags.dataSource });
  const sourceActive = seriesSourceActive(kind, flags);
  return sourceActive && flags.hasSeriesReduce ? 'store-source' : 'legacy';
}

/**
 * 판정된 종류에 대응하는 활성 플래그. `channel` 은 공용 시리즈 경로가 아니므로 항상 false 다.
 *
 * `switch` 로 두어 `ChartDataSourceKind` 가 늘면 컴파일이 먼저 깨지게 한다 — 새 종류를
 * 빠뜨리면 게이지만 조용히 레거시로 떨어지는데, 그것은 눈에 띄지 않는 회귀다.
 */
function seriesSourceActive(
  kind: ChartDataSourceKind,
  flags: GaugeValueSourceFlags,
): boolean {
  switch (kind) {
    case 'channel':
      return false;
    case 'store':
      return flags.storeSourceActive;
    case 'tsdb':
      return flags.tsdbSourceActive;
  }
}

// ---- 레거시 → store_source 이관 ----

/**
 * 이관 액션이 config 에 기록하는 patch.
 *
 * `dataSources` 키가 **없다는 것**이 비파괴성의 구조적 근거다. 이 patch 는 얕은 병합
 * (`patchConfig`)으로 적용되므로 기존 `config.dataSources` 는 손대지 않은 채 남는다
 * (§2.8 [E2] 4항 / AC-20).
 *
 * `interface` 가 아니라 `type` 인 이유: 이 patch 는 `onConfigChange(config:
 * Record<string, unknown>)` 로 그대로 넘어간다. TypeScript 는 타입 별칭에만 암묵적
 * 인덱스 시그니처를 부여하므로 `interface` 로 두면 호출부에서 캐스팅이 필요해진다.
 */
export type GaugeStoreMigrationPatch = {
  data_source: 'store';
  store_source: StoreSourceConfig;
  series_reduce: SeriesReduceFunc;
};

/**
 * 이관 대상 판별 — `config.dataSources[]` 의 **첫 번째 유효한** `store` 항목.
 *
 * "유효"의 기준은 `GaugePanel.tsx` 의 `pickStoreSource` 와 동일하다: 에이전트
 * (`storeAgentId` 또는 구 config 의 `storeAgent`)와 `storeKey` 가 모두 있어야 한다.
 * 실제로 값을 공급하고 있는 항목만 이관 대상이라는 뜻이다.
 *
 * `undefined` 를 반환하면 이관 액션은 비활성이며 안내 문구를 표시한다(§2.8 마지막 문단).
 */
export function findMigratableGaugeStoreBinding(
  config: Record<string, unknown>,
): GaugeLegacyDataSource | undefined {
  const list = config.dataSources as GaugeLegacyDataSource[] | undefined;
  if (!Array.isArray(list)) return undefined;
  return list.find(
    (d) => d?.sourceType === 'store' && (!!d.storeAgentId || !!d.storeAgent) && !!d.storeKey,
  );
}

/**
 * 이관 액션의 3상태.
 *
 * - `'available'`       : 이관 가능. 액션 활성.
 * - `'already-migrated'`: 이미 이관되어 공용 Store 소스가 활성이다. 액션 **비활성**.
 * - `'unavailable'`     : 이관할 유효한 레거시 store 바인딩이 없다. 액션 비활성.
 */
export type GaugeMigrationState = 'available' | 'already-migrated' | 'unavailable';

/**
 * 이관 액션 상태 판정 — 액션이 **파괴적으로 재실행되는 것**을 막는다.
 *
 * 이관은 `config.dataSources` 를 보존하므로(§2.8 [E2] 4항) 이관 후에도
 * `findMigratableGaugeStoreBinding` 은 계속 같은 항목을 찾아낸다. 즉 "레거시 항목이
 * 있는가" 만으로 활성 여부를 판정하면 버튼은 **영구히 활성**이다. 그 상태에서 사용자가
 * 이관 → `store_source` 를 손질(시리즈 추가 · 시간창 변경) → 버튼을 다시 누르면,
 * `buildGaugeStoreMigrationPatch` 가 1시리즈 · 1시간 기본값으로 `store_source` 를 통째로
 * 덮어써 **사용자가 방금 한 작업이 사라진다.**
 *
 * SPEC 의 이관 설계는 전부 비파괴다(§2.8 · UB1-3 · §4.5 되돌리기). 사용자 설정을 조용히
 * 버리는 버튼은 그 의도와 정면으로 어긋나므로, "이미 이관됨" 을 두 번째 비활성 조건으로
 * 둔다. 확인 대화상자가 아니라 **비활성 + 안내** 인 이유는 기존 `'unavailable'` 안내와
 * 같은 패턴이어야 사용자가 두 상태를 같은 방식으로 읽기 때문이다.
 *
 * 판정 기준에 `series_reduce` 는 **넣지 않는다.** `data_source === 'store'` + 활성
 * `store_source` 만으로 "보호할 사용자 설정이 존재한다" 가 성립하며, 그 상태에서
 * `series_reduce` 만 지워 둔 패널(게이지는 이때 레거시로 폴백한다 — §2.9)에서도
 * `store_source` 를 덮어쓰면 안 되기 때문이다.
 *
 * `'already-migrated'` 가 `'available'` 보다 **먼저** 판정된다 — 이관 후에는 두 조건이
 * 항상 동시에 참이기 때문이다(레거시 보존).
 */
export function resolveGaugeMigrationState(
  config: Record<string, unknown>,
): GaugeMigrationState {
  // SPEC-TSDB-002 §2.3 [U3]: 여기는 config 전체를 갖고 있으므로 계약에 그대로 물어본다.
  // `binding.active`(store 종류) 는 `gaugeValueSourceFlags(config).storeSourceActive` 와
  // 동치다 — 둘 다 `isStoreSourceActive(config.store_source)` 이다.
  const binding = resolvePanelSourceBinding(config);
  if (binding.kind === 'store' && binding.active) return 'already-migrated';
  return findMigratableGaugeStoreBinding(config) !== undefined ? 'available' : 'unavailable';
}

/**
 * 레거시 store 바인딩 1개 → 공용 `store_source` + `series_reduce:'last'` 변환.
 *
 * `series_reduce` 가 `'last'` 인 이유: 레거시 경로는 `mode:'latest'` 로 1키의 최신
 * 값 하나만 폴링한다. 윈도우 대표값 7종 중 그 의미에 가장 가까운 것이 `last` 다
 * (§2.8 [E2] 3항).
 *
 * 시간창/인터벌/집계/폴링 주기는 `DEFAULT_STORE_SOURCE_WINDOW` 를 쓴다 — spec 이
 * "기본값(`defaultStoreSource()`)" 이라고 못박은 그 값이며, 그 함수와 **같은 상수**를
 * 공유한다(§2.8 [E2] 1항). 값을 복제하지 않으므로 한쪽만 바뀌어 이관 결과가 조용히
 * 어긋날 여지가 없다.
 *
 * `config.dataSources` 는 **읽기만** 하고 손대지 않는다. 반환 patch 에 그 키가 없는
 * 것이 되돌리기(§4.5)의 근거다.
 */
export function buildGaugeStoreMigrationPatch(
  binding: GaugeLegacyDataSource,
): GaugeStoreMigrationPatch {
  const store: StoreSourceConfig = {
    ...(binding.storeAgentId ? { agent_id: binding.storeAgentId } : {}),
    agent_name: binding.storeAgent ?? '',
    namespace: binding.storeNamespace ?? 'default',
    selection_mode: 'keys',
    series: [{ key: binding.storeKey ?? '' }],
    ...DEFAULT_STORE_SOURCE_WINDOW,
  };
  return { data_source: 'store', store_source: store, series_reduce: 'last' };
}
