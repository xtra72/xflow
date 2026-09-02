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

import {
  MAX_SOURCES_PER_KIND,
  resolvePanelSourceEntries,
  sourceEntryLabel,
  type PanelSourceBinding,
  type ResolvePanelSourceOptions,
} from './panelDataSource';
import { readPanelXRangeOverride, withPanelRange } from './panelXRange';
import {
  mergeSeriesResults,
  type LabeledSeriesResult,
} from './mergeSeriesResults';
import {
  useStoreChartData,
  type UseStoreChartDataOptions,
  type UseStoreChartDataResult,
} from './useStoreChartData';
import type { TsdbGroupInfo } from '@/services/api/tsdbSource';
import { useTsdbChartData, type UseTsdbChartDataOptions } from './useTsdbChartData';
import { toStoreShapedConfig, useSysmetricsQueryFn } from './useSysMetricsChartData';
import { useSysmetricsLiveSeries } from './useSysmetricsLiveSeries';

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
 * 채널이 소스에서 빠진 뒤로 **모든 종류가 시리즈 소스**다. 그래서 판정은 활성 여부만
 * 남았다. 술어를 없애지 않는 이유는 호출부(패널·설정 화면 20여 곳)가 "시리즈 소스인가" 를
 * 묻고 있기 때문이다 — `binding.active` 로 바꿔 적으면 그 질문이 코드에서 사라진다.
 */
export function isPanelSeriesSource(binding: PanelSourceBinding): boolean {
  return binding.active;
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
/**
 * 종류별 조회 슬롯을 채운다 — 활성 인스턴스를 앞에서부터, 남는 자리는 `undefined`.
 *
 * 훅은 렌더마다 **같은 횟수** 불려야 하므로 인스턴스 수만큼 반복 호출할 수 없다. 고정
 * 길이 배열을 만들어 남는 자리를 idle 로 두는 것이 그 제약을 지키는 유일한 길이다.
 */
function fillSlots<T>(values: readonly T[]): Array<T | undefined> {
  const slots: Array<T | undefined> = [];
  for (let i = 0; i < MAX_SOURCES_PER_KIND; i++) slots.push(values[i]);
  return slots;
}

export function usePanelSeriesData(
  config: Record<string, unknown>,
  options?: UsePanelSeriesDataOptions,
): UsePanelSeriesDataResult {
  // 소스는 **인스턴스 목록**이다 — 같은 종류를 둘 이상 쓸 수 있다. 저장된 패널은 구 형상
  // (단일 축·종류 목록)이 한 인스턴스씩으로 읽히므로 아래 계산이 종전과 같은 결과를 낸다.
  const resolved = resolvePanelSourceEntries(config, options);
  // X축 구간은 **패널 옵션**이다. 소스마다 구간을 따로 두면 소스가 여럿일 때 한 축에
  // 그릴 수 없으므로, 패널이 구간을 선언했으면 조회가 모두 그 구간으로 나간다.
  const panelRange = readPanelXRangeOverride(config);
  const active = resolved.filter((r) => r.binding.active);

  // 종류별로 나눈 뒤 고정 슬롯에 채운다. 시스템 지표는 조회 방식이 둘이라(이력/실시간)
  // 다시 갈린다 — 두 모델은 창·인터벌·집계의 뜻이 서로 달라 한 경로로 합칠 수 없다.
  const storeEntries = active.filter((r) => r.binding.kind === 'store');
  const tsdbEntries = active.filter((r) => r.binding.kind === 'tsdb');
  const sysEntries = active.filter((r) => r.binding.kind === 'sysmetrics');
  const isLive = (e: (typeof sysEntries)[number]): boolean =>
    e.entry.sysmetrics_source?.query_mode === 'live';
  const sysHistoryEntries = sysEntries.filter((e) => !isLive(e));
  const sysLiveEntries = sysEntries.filter(isLive);

  const storeSlots = fillSlots(storeEntries);
  const tsdbSlots = fillSlots(tsdbEntries);
  const sysHistorySlots = fillSlots(sysHistoryEntries);
  const sysLiveSlots = fillSlots(sysLiveEntries);

  // `storeSourceOverride` 는 파생 소스(히트맵)가 첫 store 인스턴스를 갈아 끼우는 통로다.
  const storeOverride = options?.storeSourceOverride;

  // 슬롯 길이가 상수이므로 map 안의 훅 호출은 렌더마다 같은 횟수로 돈다.
  const storeResults = storeSlots.map((slot, i) =>
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useStoreChartData(
      withPanelRange(
        slot ? ((i === 0 && storeOverride) || slot.entry.store_source) : undefined,
        panelRange,
      ),
      !!slot,
      options?.storeOptions,
    ),
  );

  const sysmetricsQueryFn = useSysmetricsQueryFn(
    sysHistoryEntries[0]?.entry.sysmetrics_source?.agent_id ?? '',
  );
  const sysHistoryResults = sysHistorySlots.map((slot) =>
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useStoreChartData(
      withPanelRange(
        slot ? toStoreShapedConfig(slot.entry.sysmetrics_source) : undefined,
        panelRange,
      ),
      !!slot,
      { queryMatrixFn: sysmetricsQueryFn, ...options?.sysmetricsOptions },
    ),
  );

  const tsdbResults = tsdbSlots.map((slot) =>
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useTsdbChartData(withPanelRange(slot?.entry.tsdb_source, panelRange), !!slot, options?.tsdbOptions),
  );

  const sysLiveResults = sysLiveSlots.map((slot) =>
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useSysmetricsLiveSeries(withPanelRange(slot?.entry.sysmetrics_source, panelRange), !!slot),
  );

  // 합치는 순서는 **사용자가 적은 인스턴스 순서**다 — 시리즈 순서와 색 배정이 그 순서를 따른다.
  const entries = resolved.map((r) => r.entry);
  const labeled: LabeledSeriesResult[] = [];
  const counters = { store: 0, tsdb: 0, sysHistory: 0, sysLive: 0 };
  resolved.forEach((r, index) => {
    if (!r.binding.active) return;
    const label = sourceEntryLabel(entries, index);
    if (r.binding.kind === 'store') {
      labeled.push({ label, result: storeResults[counters.store++]! });
    } else if (r.binding.kind === 'tsdb') {
      labeled.push({ label, result: tsdbResults[counters.tsdb++]! });
    } else if (isLive(r)) {
      labeled.push({ label, result: sysLiveResults[counters.sysLive++]! });
    } else {
      labeled.push({ label, result: sysHistoryResults[counters.sysHistory++]! });
    }
  });

  // 활성 소스가 하나도 없으면 종전과 같은 idle 형상을 돌려준다 — 첫 store 슬롯은 비활성일
  // 때 그 형상이므로 새 객체를 만들지 않는다(참조가 바뀌면 하위 useMemo 가 매 렌더 돈다).
  if (labeled.length === 0) return storeResults[0]!;
  const merged = mergeSeriesResults(labeled);
  // TSDB 만 갖는 축(group 페이지)은 합치기 형상에 없다 — 그 소스가 있을 때만 얹는다.
  const firstTsdb = tsdbResults[0];
  return firstTsdb?.groups !== undefined && labeled.some((l) => l.result === firstTsdb)
    ? { ...merged, groups: firstTsdb.groups }
    : merged;
}
