// 인터벌(버킷) 간격 프리셋과 표기 — Store · TSDB 공용.
//
// 두 소스가 각자 눈금을 갖고 있으면 같은 개념이 화면마다 다르게 보여, 소스를 바꿀 때
// 값을 옮겨 적기 어렵다. 의존성 없는 순수 모듈이라 어느 쪽에서 import 해도 순환이
// 생기지 않는다(`ChartPanelSections` → `TsdbSourceSection` 방향 의존이 이미 있다).

/**
 * 인터벌(버킷) 간격 프리셋(ms). 에이전트 TSDB 뷰어와 같은 눈금이다.
 *
 * 목록에 없는 값도 저장될 수 있으므로("직접 입력") 선택을 잃지 않게 별도 경로를 둔다.
 */
export const INTERVAL_PRESETS_MS = [
  10_000, 30_000, 60_000, 300_000, 900_000, 1_800_000, 3_600_000, 21_600_000, 86_400_000,
] as const;

/** 인터벌 ms 를 사람이 읽는 눈금으로 표기한다(10s · 5m · 1h · 1d). */
export function formatIntervalMs(ms: number): string {
  if (ms % 86_400_000 === 0) return `${ms / 86_400_000}d`;
  if (ms % 3_600_000 === 0) return `${ms / 3_600_000}h`;
  if (ms % 60_000 === 0) return `${ms / 60_000}m`;
  return `${Math.round(ms / 1000)}s`;
}

/** 저장된 값이 프리셋 목록에 있는가. 없으면 "직접 입력" 경로로 표시한다. */
export function isIntervalPreset(ms: number): boolean {
  return (INTERVAL_PRESETS_MS as readonly number[]).includes(ms);
}
