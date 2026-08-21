// 구간 대표값(window reduce) 순수 로직.
//
// 한 시리즈의 시간 윈도우 타임라인을 숫자 1개로 접는다. `store_source.aggregation`
// (버킷 집계 — 조회 축)과는 다른 축이며, 이 모듈은 **렌더 시점**의 표현 축만 다룬다.
// 두 축의 구분은 chartChannelTypes.ts 의 `ChartPanelConfigBase.series_reduce` 주석 참조.
//
// 표본 정규화(null·비유한·비수치 제외)를 이 모듈이 내부에서 수행하는 이유:
// 정규화를 호출자에게 맡기면 stat/gauge/bar/pie 4개 패널에 같은 필터가 복제되고
// 규칙이 갈린다. 규칙은 여기 한 곳에만 있어야 한다.
//
// @spec SPEC-CHART-002 §2.2 [U2]

import type { ChartEntry, SeriesReduceFunc } from './chartChannelTypes';
import type { StoreSeriesStyle } from './useStoreChartData';

/**
 * 사용자에게 노출하는 대표값 선택지(표시 순서 포함). @spec SPEC-CHART-002 §2.2
 *
 * `'first'` 는 여기에 없다 — `delta` 의 내부 입력으로만 쓰는 값이다.
 */
export const SERIES_REDUCE_FUNCS: readonly SeriesReduceFunc[] = [
  'max',
  'avg',
  'min',
  'last',
  'sum',
  'count',
  'delta',
];

/** 한 시리즈의 대표값 계산 결과(다중 출력의 단위). */
export interface ReducedSeries {
  /** 시리즈 표시 이름 — `storeSeriesLabel` 규칙 그대로다(§2.5). */
  name: string;
  /** 대표값. "값 없음" 은 `undefined` 이며 `0` 과 구분된다(§4.3). */
  value: number | undefined;
  /**
   * 시리즈에 지정된 색상. 미지정이면 `undefined` 로 남긴다.
   *
   * 팔레트 폴백(`pickSeriesColor(i)`)을 여기서 채우지 않는 이유: 폴백 규칙이 패널마다
   * 다르다(§2.6). bar/pie 는 팔레트를 채움색으로 쓰지만 stat/gauge 는 색 미지정 시
   * "기본 라벨색" 을 써야 하므로, 여기서 팔레트를 채우면 두 패널이 그것을 도로
   * 걷어내야 한다.
   */
  color: string | undefined;
}

/**
 * 시리즈 타임라인 하나를 대표값 1개로 접는다. @spec SPEC-CHART-002 §2.2 [U2]
 *
 * **표본(sample) 정의**: `value` 가 유한한 `number` 인 항목만 표본이다.
 * `null` · `NaN` · `Infinity` · 문자열 · 객체는 전부 제외된다. 숫자 모양 문자열
 * (`'3'`)도 강제 변환하지 않는다(UB1-5). `data_type='boolean'` 시리즈는 store 변환
 * 계층에서 이미 `1`/`0` 으로 정규화되어 도착하므로 별도 분기가 필요 없다.
 *
 * | 대표값 | 정의 | 표본 0개 |
 * |--------|------|----------|
 * | `max` / `min` | 표본 값의 최대 / 최소 | `undefined` |
 * | `avg` | 산술 평균(`sum / count`) | `undefined` |
 * | `last` | `timestamp` 최대 표본의 값(동률이면 배열상 뒤 항목) | `undefined` |
 * | `sum` | 표본 값의 합 — **0 이 아니다**(§4.3) | `undefined` |
 * | `count` | **수치** 표본의 개수 — 7종 중 유일하게 빈 윈도우에서 값을 갖는다 | `0` |
 * | `delta` | `last − first`. 표본이 1개뿐이면 변화량 미정의 | `undefined` |
 *
 * `count` 를 "수신 표본 수" 가 아니라 "수치 표본 수" 로 정의하는 이유는
 * `avg === sum / count` 항등을 성립시키기 위해서다.
 *
 * 입력 배열이 `timestamp` 정렬되어 있지 않아도 된다 — `last`/`first` 는 배열 위치가
 * 아니라 `timestamp` 로 판정한다(같은 표시 이름의 시리즈가 병합되면 정렬이 깨진다).
 *
 * 계산은 표본 수에 대해 단일 패스이며 중간 배열을 만들지 않는다(§5 성능).
 */
export function reduceSeries(
  entries: readonly ChartEntry[] | undefined,
  fn: SeriesReduceFunc,
): number | undefined {
  let count = 0;
  let sum = 0;
  let min = 0;
  let max = 0;
  let firstAt = 0;
  let firstValue = 0;
  let lastAt = 0;
  let lastValue = 0;

  for (const entry of entries ?? []) {
    const value = entry.value;
    // 표본 정규화 — 유한한 숫자만 통과시킨다(외부 입력 방어, §5 Secured).
    if (typeof value !== 'number' || !Number.isFinite(value)) continue;
    const at = entry.timestamp;

    if (count === 0) {
      min = value;
      max = value;
      firstAt = at;
      firstValue = value;
      lastAt = at;
      lastValue = value;
    } else {
      if (value < min) min = value;
      if (value > max) max = value;
      // 동률(`>=` / `<`)은 last 가 뒤 항목, first 가 앞 항목을 갖도록 갈라 둔다.
      if (at < firstAt) {
        firstAt = at;
        firstValue = value;
      }
      if (at >= lastAt) {
        lastAt = at;
        lastValue = value;
      }
    }
    count += 1;
    sum += value;
  }

  // count 만 빈 윈도우에서 값을 갖는다 — "0개를 셌다" 는 정확한 진술이기 때문이다.
  if (fn === 'count') return count;
  if (count === 0) return undefined;

  switch (fn) {
    case 'max':
      return max;
    case 'avg':
      return sum / count;
    case 'min':
      return min;
    case 'last':
      return lastValue;
    case 'sum':
      return sum;
    case 'delta':
      // 표본 1개는 변화량이 정의되지 않는다(0 이 아니다).
      return count < 2 ? undefined : lastValue - firstValue;
    default: {
      // 도달 불가 — 7종 유니온이 모두 처리되었음을 컴파일 시점에 강제한다.
      const exhaustive: never = fn;
      return exhaustive;
    }
  }
}

/**
 * 선택된 모든 시리즈의 대표값을 **시리즈 순서 그대로** 계산한다.
 * @spec SPEC-CHART-002 §2.4 [U4]
 *
 * `useStoreChartData` 의 반환값(`seriesEntries` / `seriesNames` / `seriesStyles`)을
 * 그대로 받는다. 순서는 `seriesNames` 를 따르며 값 크기로 재정렬하지 않는다 —
 * 매 폴링마다 타일이 자리를 바꾸면 읽을 수 없다(UB1-11).
 *
 * 표시 이름이 중복되는 시리즈는 출력도 하나로 합쳐진다(가정 6-6). `useStoreChartData`
 * 가 이미 같은 이름의 타임라인을 하나로 병합해 두므로, 이름을 두 번 렌더하면 같은 값이
 * 두 번 나올 뿐이다.
 *
 * 대표값이 `undefined` 인 시리즈도 항목을 남긴다 — 슬롯을 유지할지(stat/gauge) 생략할지
 * (bar/pie)는 패널이 정한다(§2.4).
 */
export function reduceAllSeries(
  seriesEntries: ReadonlyMap<string, ChartEntry[]>,
  seriesNames: readonly string[],
  seriesStyles: ReadonlyMap<string, StoreSeriesStyle>,
  fn: SeriesReduceFunc,
): ReducedSeries[] {
  const out: ReducedSeries[] = [];
  const seen = new Set<string>();
  for (const name of seriesNames) {
    if (seen.has(name)) continue;
    seen.add(name);
    out.push({
      name,
      value: reduceSeries(seriesEntries.get(name), fn),
      color: seriesStyles.get(name)?.color,
    });
  }
  return out;
}
