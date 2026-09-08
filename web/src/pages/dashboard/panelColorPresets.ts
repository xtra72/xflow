// 패널 색상(`config.panelColor`) 프리셋 팔레트.
//
// `panelColor` 는 대시보드 격자 래퍼의 좌/상단 테두리 악센트로 그려진다
// (`DashboardPage.tsx` — `--panel-accent`). 값은 패널 타입과 무관하므로 팔레트도
// 한 곳에만 있어야 한다.
//
// @spec SPEC-CHART-003 §2.1 [U1-2]

export const PANEL_COLORS = [
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#06b6d4', // cyan
  '#10b981', // emerald
  '#f59e0b', // amber
  '#ef4444', // red
  '#ec4899', // pink
  '#6b7280', // gray
] as const;
