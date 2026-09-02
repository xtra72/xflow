// 여러 소스의 조회 결과를 **한 패널의 시리즈 목록으로 합친다** — 의존성 없는 순수 모듈.
//
// 종전에는 패널이 소스를 하나만 골랐고, `usePanelSeriesData` 는 훅 셋을 모두 부른 뒤
// **하나를 골라** 반환했다. 다중 소스는 그 "고르기" 를 "합치기" 로 바꾸는 일이다.
//
// 합치는 순간 종전에는 없던 물음이 생긴다. 그 답을 여기 한자리에 둔다 — 훅 안에 흩어 두면
// 같은 물음이 소스 종류만큼 곱해지고, 답이 서로 갈린다.
//
//   1. 이름이 겹치면?      → 겹칠 때만 소스 이름을 덧붙인다(안 겹치면 그대로 둔다).
//   2. 상태가 갈리면?      → 가장 심각한 것이 이긴다.
//   3. 순서는?             → 소스 목록 순서, 그 안에서는 조회 순서.
//
// 소스가 하나뿐이면 결과는 **입력 그대로**다. 그래야 저장된 단일 소스 패널의 그림이
// 한 픽셀도 변하지 않는다.

import type { ChartEntry } from './chartChannelTypes';
import type { StoreSeriesStyle, UseStoreChartDataResult } from './useStoreChartData';
import type { ChartConnectionStatus } from './chartChannelTypes';

/** 합칠 결과 하나 — 어느 소스에서 왔는지 이름표를 함께 받는다. */
export interface LabeledSeriesResult {
  /**
   * 이름 충돌 시 시리즈 이름 뒤에 붙일 꼬리표. 사람이 읽을 소스 이름이다
   * (`Store` · `TSDB` · `시스템 지표`).
   */
  label: string;
  result: MergeableResult;
}

/** 합치기에 필요한 만큼만 좁힌 결과 형상. */
export type MergeableResult = Pick<
  UseStoreChartDataResult,
  'entries' | 'seriesEntries' | 'seriesStyles' | 'seriesNames' | 'booleanSeries' | 'status'
> & {
  errorReason?: string;
  closedReason?: string;
  partialFailureCount?: number;
  backendMismatch?: boolean;
};

/**
 * 심각한 순서. 큰 값이 이긴다.
 *
 * `idle` 이 가장 낮은 것이 하중 지지점이다 — 고르지 않은 소스는 언제나 idle 이므로,
 * 그것이 이기면 실제로 조회 중인 소스의 상태가 가려진다.
 */
const STATUS_RANK: Record<ChartConnectionStatus, number> = {
  idle: 0,
  connected: 1,
  connecting: 2,
  disconnected: 3,
  closed: 4,
  error: 5,
};

/**
 * 상태를 합친다 — **가장 심각한 것이 이긴다.**
 *
 * 세 소스 중 하나가 실패하면 패널은 실패를 알려야 한다. 성공한 쪽을 이유로 조용히
 * 성공으로 표시하면, 사라진 줄이 왜 없는지 화면에서 알 길이 없다.
 */
export function mergeStatus(
  statuses: readonly ChartConnectionStatus[],
): ChartConnectionStatus {
  let worst: ChartConnectionStatus = 'idle';
  for (const s of statuses) {
    if ((STATUS_RANK[s] ?? 0) > (STATUS_RANK[worst] ?? 0)) worst = s;
  }
  return worst;
}

/**
 * 겹치는 이름에만 꼬리표를 붙인 이름표를 만든다.
 *
 * 안 겹치는 이름은 **그대로 둔다.** 모든 이름에 소스를 붙이면 소스가 하나뿐인 패널의
 * 범례까지 길어지고, 저장된 대시보드의 글자가 조용히 바뀐다.
 */
export function resolveMergedNames(
  sources: ReadonlyArray<{ label: string; names: readonly string[] }>,
): Map<string, string>[] {
  const seen = new Map<string, number>();
  for (const s of sources) {
    for (const n of s.names) seen.set(n, (seen.get(n) ?? 0) + 1);
  }
  return sources.map((s) => {
    const m = new Map<string, string>();
    for (const n of s.names) {
      m.set(n, (seen.get(n) ?? 0) > 1 ? `${n} (${s.label})` : n);
    }
    return m;
  });
}

/**
 * 여러 소스의 결과를 하나로 합친다.
 *
 * 소스가 하나면 **그 결과를 그대로 돌려준다** — 새 객체를 만들지 않는다. 참조가 바뀌면
 * 하위 `useMemo` 가 매 렌더 다시 돌아, 다중 소스를 쓰지 않는 패널이 느려진다.
 */
export function mergeSeriesResults(sources: readonly LabeledSeriesResult[]): MergeableResult {
  if (sources.length === 0) {
    return {
      entries: [],
      seriesEntries: new Map(),
      seriesStyles: new Map(),
      seriesNames: [],
      booleanSeries: new Set(),
      status: 'idle',
    };
  }
  if (sources.length === 1) return sources[0]!.result;

  const nameMaps = resolveMergedNames(
    sources.map((s) => ({ label: s.label, names: s.result.seriesNames })),
  );

  const seriesEntries = new Map<string, ChartEntry[]>();
  const seriesStyles = new Map<string, StoreSeriesStyle>();
  const booleanSeries = new Set<string>();
  const seriesNames: string[] = [];
  const entries: ChartEntry[] = [];
  let partialFailureCount = 0;
  let backendMismatch = false;
  let errorReason: string | undefined;
  let closedReason: string | undefined;

  sources.forEach((s, i) => {
    const names = nameMaps[i]!;
    for (const original of s.result.seriesNames) {
      const name = names.get(original) ?? original;
      seriesNames.push(name);
      const points = s.result.seriesEntries.get(original);
      if (points) seriesEntries.set(name, points);
      const style = s.result.seriesStyles.get(original);
      if (style) seriesStyles.set(name, style);
      if (s.result.booleanSeries.has(original)) booleanSeries.add(name);
    }
    entries.push(...s.result.entries);
    partialFailureCount += s.result.partialFailureCount ?? 0;
    if (s.result.backendMismatch) backendMismatch = true;
    // 사유는 **처음 만난 것**을 남긴다. 여러 소스가 각자 실패해도 화면에는 한 줄만 뜨므로,
    // 뒤엣것으로 덮으면 어느 소스의 이유인지 순서에 따라 달라진다.
    if (errorReason === undefined) errorReason = s.result.errorReason;
    if (closedReason === undefined) closedReason = s.result.closedReason;
  });

  // 평탄 타임라인은 시간순이어야 한다 — 소스별로 이어 붙이면 소스 경계에서 되감긴다.
  entries.sort((a, b) => a.timestamp - b.timestamp);

  return {
    entries,
    seriesEntries,
    seriesStyles,
    seriesNames,
    booleanSeries,
    status: mergeStatus(sources.map((s) => s.result.status)),
    ...(errorReason !== undefined ? { errorReason } : null),
    ...(closedReason !== undefined ? { closedReason } : null),
    ...(partialFailureCount > 0 ? { partialFailureCount } : null),
    ...(backendMismatch ? { backendMismatch: true } : null),
  };
}
