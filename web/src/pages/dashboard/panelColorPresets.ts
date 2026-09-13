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

/**
 * 자유 입력 색상을 저장 형식(`#rrggbb` 소문자)으로 맞춘다. 맞출 수 없으면 `null`.
 *
 * 저장 형식을 6자리로 고정하는 이유는 두 가지다.
 *   (1) `panelColor` 는 `--panel-accent` 와 `border: 4px solid ${panelColor}` 에
 *       **그대로** 끼워 넣어진다(`RemoteDashboardView`). 형식을 검증하지 않으면
 *       임의 문자열이 CSS 선언으로 새어 들어간다.
 *   (2) `<input type="color">` 의 value 는 `#rrggbb` 7자만 받는다. 3자리 약식을
 *       그대로 두면 네이티브 피커가 값을 못 읽고 검정으로 되돌린다.
 *
 * 소문자로 내리는 것도 형식 통일이 아니라 동작 때문이다 — 프리셋 선택 표시는
 * `panelColor === color` 문자열 비교라, 손으로 친 `#8B5CF6` 가 대문자로 남으면
 * 같은 색인데도 프리셋 스와치에 선택 표시가 붙지 않는다.
 */
export function normalizePanelColor(input: string): string | null {
  const body = input.trim().replace(/^#/, '');
  // 3자리 약식(#abc)은 각 자리를 두 번 써서 6자리로 편다.
  if (/^[0-9a-fA-F]{3}$/.test(body)) {
    return `#${body[0]}${body[0]}${body[1]}${body[1]}${body[2]}${body[2]}`.toLowerCase();
  }
  if (/^[0-9a-fA-F]{6}$/.test(body)) {
    return `#${body}`.toLowerCase();
  }
  return null;
}
