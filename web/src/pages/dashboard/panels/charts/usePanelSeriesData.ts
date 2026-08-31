// 패널 시리즈 데이터 훅 — 소스 종류로 키잉된 단일 조회의 진입점.
//
// 패널은 `data_source` 를 직접 비교해 훅을 고르지 않고 이 훅 하나를 호출한다.
// 소스 종류가 N개로 늘어도 패널 렌더 코드는 그대로다(spec.md §1.2.5 · §2.3).
//
// **반환 형상은 `UseStoreChartDataResult` 를 확장만 한다.** 6개 패널이 이미
// `entries` · `seriesEntries` · `seriesNames` · `seriesStyles` · `booleanSeries` ·
// `status` 를 소비하고 있으므로, 기존 필드를 그대로 두면 패널 렌더 코드가 한 줄도
// 바뀌지 않는다. 이것이 spec.md §2.3 이 말하는 "두 번째 하중 지지점" 이다.
//
// 확장은 **가산 전용**이다 — 추가 필드는 전부 옵셔널이므로 `UseStoreChartDataResult`
// 를 요구하는 자리에 그대로 대입된다. 필수 필드를 더하면 그 대입 가능성이 깨져 6개
// 패널을 동시에 고쳐야 하고, 그것은 이 SPEC 이 피하려던 바로 그 상태다.
//
// **훅 규칙 준수 패턴**: 소스 종류별 훅을 조건 없이 **전부** 호출하고, 진 쪽에는
// `undefined` 를 넘겨 idle 로 둔다(`GaugePanel.tsx` 가 이미 쓰는 패턴). 조건부 호출은
// React 훅 규칙 위반이며, 소스를 전환할 때 훅 순서가 바뀌어 상태가 뒤섞인다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4)

import type {
  StoreSourceConfig,
  SysmetricsSourceConfig,
  TsdbSourceConfig,
} from './chartChannelTypes';
import {
  resolvePanelSourceBinding,
  type PanelSourceBinding,
  type ResolvePanelSourceOptions,
} from './panelDataSource';
import {
  useStoreChartData,
  type UseStoreChartDataOptions,
  type UseStoreChartDataResult,
} from './useStoreChartData';
import type { TsdbGroupInfo } from '@/services/api/tsdbSource';
import { useTsdbChartData, type UseTsdbChartDataOptions } from './useTsdbChartData';
import { toStoreShapedConfig, useSysmetricsQueryFn } from './useSysMetricsChartData';

/**
 * 패널 시리즈 훅 결과 — `UseStoreChartDataResult` + TSDB 전용 상태 신호(가산).
 *
 * 두 신호가 여기 실리는 이유: `useTsdbChartData` 가 이미 계산하는데도 반환 타입이
 * `UseStoreChartDataResult` 로 좁혀져 있으면 소비자가 **볼 수 없다**. 그러면 §2.14 의
 * 부분 실패 배지와 §2.18 의 백엔드 불일치 문구를 화면에 띄울 방법이 사라진다.
 *
 * 둘 다 옵셔널인 이유는 store 경로에는 대응 개념이 없기 때문이다 — store 어댑터는
 * `Promise.all` 이라 부분 실패가 성립하지 않고(§4.3), 백엔드 파생도 하지 않는다.
 * 런타임에서도 store 경로는 이 키들을 **싣지 않는다**(있는데 0/false 인 것과 다르다).
 *
 * @spec SPEC-TSDB-002 §2.14 (S2) · §2.18 (U11)
 */
export interface UsePanelSeriesDataResult extends UseStoreChartDataResult {
  /** 이번 조회에서 실패한 시리즈 개수(TSDB 경로에서만). @spec §2.19 */
  partialFailureCount?: number;
  /** 참조된 에이전트가 지원 백엔드가 아닌가(TSDB 경로에서만). @spec §2.18 */
  backendMismatch?: boolean;
  /**
   * group by 항목별 페이지 상황(TSDB 경로에서만).
   * @spec SPEC-TSDB-004 §2.7
   *
   * 옵셔널인 것이 하중 지지점이다 — 필수로 두면 store 경로가 이 키를 만들어야
   * 하고, 그것이 §2.3 이 지키려던 "두 경로의 반환 키 집합 동일" 불변식을 깬다.
   */
  groups?: TsdbGroupInfo[];
}

/** `usePanelSeriesData` 의 선택 인자. */
export interface UsePanelSeriesDataOptions extends ResolvePanelSourceOptions {
  /** `useStoreChartData` 로 그대로 전달되는 테스트 주입 통로. */
  storeOptions?: UseStoreChartDataOptions;
  /** `useTsdbChartData` 로 그대로 전달되는 테스트 주입 통로. */
  tsdbOptions?: UseTsdbChartDataOptions;
  /**
   * sysmetrics 경로의 테스트 주입 통로.
   *
   * 이 소스는 Store 훅을 그대로 타므로 옵션 형상도 같다 — 다만 `queryMatrixFn` 을
   * 주지 않으면 이력 조회 함수가 기본으로 꽂힌다.
   */
  sysmetricsOptions?: UseStoreChartDataOptions;
}

/**
 * 패널이 채널 훅이 아니라 **시리즈 소스 훅**을 써야 하는가.
 *
 * `kind !== 'channel'` 로 쓰는 이유: 소스 종류가 늘어도 이 식은 그대로다. 종류마다
 * `=== 'store' || === 'tsdb'` 를 덧붙이면 그것이 spec.md §1.2.5 가 지목한
 * "지점 × 종류" 곱셈이며 UB1-1 이 금지하는 상태다.
 *
 * `channel` 은 활성 조건이 항상 참이므로(채널 훅이 빈 상태를 스스로 처리한다)
 * `binding.active` 만으로는 구분되지 않는다 — 종류 항이 반드시 필요하다.
 */
export function isPanelSeriesSource(binding: PanelSourceBinding): boolean {
  return binding.kind !== 'channel' && binding.active;
}

/**
 * 패널 config 에서 시리즈 데이터를 조회한다.
 *
 * | 판정된 종류 | 활성 | 반환 |
 * |-------------|------|------|
 * | `store` | 예 | Store 훅 결과 |
 * | `tsdb` | 예 | TSDB 훅 결과(부분 실패 신호를 함께 실은 확장 형상) |
 * | `sysmetrics` | 예 | 에이전트 이력을 Store 훅으로 조회한 결과(형상 동일) |
 * | 그 외 / 비활성 | — | idle(= 종전 `useStoreChartData(undefined, false)` 와 동일) |
 *
 * 마지막 행이 하위 호환의 핵심이다. 종전 패널들은 채널 모드에서도 store 훅을 비활성
 * 인자로 호출해 idle 결과를 읽었으므로, 여기서 idle 을 돌려주면 동작이 같다(§2.4).
 */
export function usePanelSeriesData(
  config: Record<string, unknown>,
  options?: UsePanelSeriesDataOptions,
): UsePanelSeriesDataResult {
  const binding = resolvePanelSourceBinding(config, options);

  // Store · sysmetrics 분기 — **같은 훅을 한 번만** 호출한다.
  //
  // sysmetrics 는 config 형상을 Store 로 옮기고(`toStoreShapedConfig`) 조회 함수만
  // 갈아끼우면 Store 경로 그대로다. 두 소스가 각자 `useStoreChartData` 를 부르면 진
  // 쪽이 idle 이라도 훅이 중복되고, 어느 호출이 실제 조회인지 읽기 어려워진다.
  const storeActive = binding.kind === 'store' && binding.active;
  const sysmetricsActive = binding.kind === 'sysmetrics' && binding.active;

  const storeSource =
    options?.storeSourceOverride ?? (config.store_source as StoreSourceConfig | undefined);
  const sysmetricsSource = config.sysmetrics_source as SysmetricsSourceConfig | undefined;

  const sysmetricsQueryFn = useSysmetricsQueryFn(sysmetricsSource?.agent_id ?? '');
  const seriesSource = sysmetricsActive
    ? toStoreShapedConfig(sysmetricsSource)
    : storeActive
      ? storeSource
      : undefined;
  const seriesOptions = sysmetricsActive
    ? { queryMatrixFn: sysmetricsQueryFn, ...options?.sysmetricsOptions }
    : options?.storeOptions;

  const storeResult = useStoreChartData(
    seriesSource,
    storeActive || sysmetricsActive,
    seriesOptions,
  );

  // TSDB 분기 — Store 와 같은 규칙으로 조건 없이 호출하고, 진 쪽은 config 를 넘기지
  // 않아 idle 로 둔다. 조건부 호출은 훅 규칙 위반이며 소스 전환 시 상태가 뒤섞인다.
  const tsdbActive = binding.kind === 'tsdb' && binding.active;
  const tsdbSource = config.tsdb_source as TsdbSourceConfig | undefined;
  const tsdbResult = useTsdbChartData(
    tsdbActive ? tsdbSource : undefined,
    tsdbActive,
    options?.tsdbOptions,
  );

  if (tsdbActive) return tsdbResult;
  // storeResult 는 비활성일 때 idle 형상을 반환하므로 채널 경로도 이 값으로 덮인다.
  return storeResult;
}
