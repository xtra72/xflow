// X축 범위 — **패널이 소유한다.**
//
// 이 파일은 두 번 뜻이 바뀌었고, 지금 형태가 그 결론이다.
//
//   1. 처음: 채널 모드는 패널의 `x_range`, 시리즈 소스는 소스의 조회 구간. 소유자가
//      갈리므로 X축 섹션은 시리즈 소스에서 편집기를 **감췄다**.
//   2. 다음: 감춘 탓에 시스템 지표 실시간 조회에서 표시 구간을 정할 자리가 어디에도 없었다.
//      그래서 X축 섹션이 소스의 `range` 를 직접 고치도록 이었다.
//   3. 지금: 소스가 **여럿**이 되면서 "어느 소스의 구간인가" 가 답이 없는 물음이 됐다.
//      Store A 와 Store B 가 서로 다른 구간을 보면 한 X축에 그릴 수 없다.
//
// 그래서 구간은 패널 옵션 하나다. 소스는 그 구간을 **받아** 조회한다 — 소스마다 구간을
// 따로 두지 않으므로 어긋날 자리가 없고, 소스를 더해도 축이 흔들리지 않는다.

import { readChartXRange, type ChartXRangeSource, type SeriesRange } from './seriesRange';

/**
 * 패널의 X축 범위를 읽는다.
 *
 * 구 어휘(`time_window_mode` · `recent_window_sec` · `max_points`)는 `readChartXRange` 가
 * 폴백으로 해석하므로 저장된 패널이 그대로 동작한다.
 */
export function readPanelXRange(config: Record<string, unknown>): SeriesRange {
  return readChartXRange(config as ChartXRangeSource);
}

/**
 * X축 범위를 고치는 config 패치.
 *
 * 저장 자리가 하나(`x_range`)이므로 소스 종류를 볼 필요가 없다. 소스 블록의 구간을 함께
 * 고치지 않는 것이 중요하다 — 소스가 여럿일 때 어느 블록을 고쳐야 할지 답이 없고, 패널
 * 구간이 정본이므로 소스 블록의 값은 조회 시점에 덮인다(`resolveSourceQueryRange`).
 */
export function panelXRangePatch(
  _config: Record<string, unknown>,
  range: SeriesRange,
): Record<string, unknown> {
  return { x_range: range };
}

/**
 * 패널이 **명시한** X축 구간. 선언하지 않았으면 `undefined`.
 *
 * `readPanelXRange` 와 갈라 두는 이유는 `max_points` 다. 그 필드는 라인 차트에서 "X축에
 * 남길 포인트 수" 지만, 바·파이·통계에서는 **막대(조각) 수**라는 전혀 다른 뜻이다. 표시용
 * 폴백으로 읽는 것은 무해하지만 그것을 조회 구간으로 밀어 넣으면, 막대 12개짜리 패널이
 * 데이터를 12점만 긁어 오게 된다.
 *
 * 그래서 조회에 얹는 것은 사용자가 X축 구간으로 **직접 고른 값**뿐이다.
 */
export function readPanelXRangeOverride(
  config: Record<string, unknown>,
): SeriesRange | undefined {
  const c = config as ChartXRangeSource;
  if (!c.x_range?.mode && c.time_window_mode === undefined) return undefined;
  return readChartXRange(c);
}

/**
 * 소스 블록에 **패널 구간을 얹는다** — 조회는 이 구간으로 나간다.
 *
 * 소스 블록에 남아 있는 `range` · `time_window_ms` 는 그 소스가 따로 저장한 값이다. 패널이
 * 구간을 선언했으면 그쪽이 정본이므로 조회 시점에 덮는다 — 덮지 않으면 X축은 10분인데
 * 조회는 1시간을 긁어 오고, 소스가 여럿이면 소스마다 다른 구간을 보게 된다.
 *
 * 패널이 선언하지 않았으면(`range` 가 `undefined`) 소스의 값을 그대로 둔다.
 */
export function withPanelRange<T extends { range?: SeriesRange } | undefined>(
  source: T,
  range: SeriesRange | undefined,
): T {
  if (!source || !range) return source;
  return { ...source, range } as T;
}
