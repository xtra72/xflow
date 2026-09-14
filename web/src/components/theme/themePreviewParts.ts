// 미리보기 조각 ↔ 토큰 대응 — **한 곳**에 둔다.
//
// 표에서 조각으로, 조각에서 표로 — 강조는 양방향이다. 두 방향이 각자 대응표를 들면
// 반드시 어긋나고, 어긋난 쪽은 "이 토큰이 어디 쓰이는지" 를 틀리게 알려 준다. 틀린
// 안내는 없느니만 못하다.
//
// 조각은 **화면 영역**으로 묶인다. 색은 한 화면에서만 쓰이지 않으므로, 플로우 하나만
// 보여 주면 "대시보드에서는 어떻게 보이나" 에 답할 수 없다.
//
// @spec SPEC-THEME-001 §결정 3 · AC-05 · AC-08

/** 미리보기가 흉내 내는 화면 영역. `always` 는 어느 탭에서나 보인다. */
export type PreviewArea = 'always' | 'dashboard' | 'flow' | 'list' | 'schedule';

/** 탭으로 고를 수 있는 영역 — `always` 는 탭이 아니다. */
export const PREVIEW_TABS: readonly { id: Exclude<PreviewArea, 'always'>; label: string }[] = [
  { id: 'dashboard', label: '대시보드' },
  { id: 'flow', label: '플로우' },
  { id: 'list', label: '에이전트 · 디바이스' },
  { id: 'schedule', label: '스케줄' },
];

/** 미리보기 한 조각. */
export interface PreviewPart {
  /** 조각 식별자 — `data-part` 와 테스트 아이디에 쓴다. */
  readonly id: string;
  /** 사람이 읽는 이름. */
  readonly label: string;
  /** 어느 화면의 조각인가. */
  readonly area: PreviewArea;
  /** 이 조각이 칠하는 데 쓰는 토큰들(`--color-` 를 뺀 이름). */
  readonly tokens: readonly string[];
}

/**
 * 조각 목록.
 *
 * 하한은 "**모든** 토큰이 최소 한 번씩 나타난다"(AC-05) — 나타나지 않는 토큰은 미리보기로
 * 고를 수 없다. 탭이 넷이므로 그 하한은 **탭 전체의 합집합**에 대해 성립한다.
 */
export const PREVIEW_PARTS: readonly PreviewPart[] = [
  // --- 어느 화면에나 있는 것 ---
  {
    id: 'shell',
    label: '앱 셸',
    area: 'always',
    tokens: ['bg-primary', 'bg-secondary', 'text-primary', 'border-subtle'],
  },
  {
    id: 'controls',
    label: '단추 · 상태',
    area: 'always',
    tokens: [
      'interactive-primary',
      'interactive-hover',
      'interactive-active',
      'interactive-muted',
      'text-inverse',
      'border-strong',
      'status-running',
      'status-stopped',
      'status-error',
      'status-warning',
      'status-info',
    ],
  },

  // --- 대시보드 ---
  {
    id: 'dashboard-stat',
    label: '통계 패널',
    area: 'dashboard',
    tokens: ['bg-surface', 'border-default', 'text-primary', 'text-secondary'],
  },
  {
    id: 'dashboard-chart',
    label: '차트 패널',
    area: 'dashboard',
    tokens: ['bg-surface', 'border-default', 'text-muted', 'interactive-primary'],
  },

  // --- 플로우 편집기 ---
  {
    id: 'canvas',
    label: '편집기 캔버스',
    area: 'flow',
    tokens: ['bg-secondary', 'flow-dot', 'flow-area'],
  },
  {
    id: 'node',
    label: '플로우 노드',
    area: 'flow',
    tokens: ['bg-surface', 'bg-sunken', 'border-default', 'text-primary', 'text-muted'],
  },
  {
    id: 'edge',
    label: '연결선',
    area: 'flow',
    tokens: ['flow-edge'],
  },

  // --- 에이전트 · 디바이스 목록 ---
  {
    id: 'list-header',
    label: '목록 머리글',
    area: 'list',
    tokens: ['bg-primary', 'text-muted', 'border-default'],
  },
  {
    id: 'list-rows',
    label: '목록 행',
    area: 'list',
    tokens: [
      'bg-surface',
      'bg-elevated',
      'border-subtle',
      'text-primary',
      'text-secondary',
      'status-running',
      'status-stopped',
      'status-error',
      // 배지의 글자는 칠과 다른 값을 쓴다 — 칠용 값은 점을 칠하라고 고른 것이라
      // 글자로 쓰면 밝은 테마에서 AA 를 넘지 못한다.
      'status-running-text',
      'status-stopped-text',
      'status-error-text',
    ],
  },

  // --- 스케줄 ---
  {
    id: 'schedule-rows',
    label: '스케줄 행',
    area: 'schedule',
    tokens: [
      'bg-surface',
      'bg-sunken',
      'border-default',
      'text-primary',
      'text-secondary',
      'text-muted',
      'interactive-muted',
      'interactive-active',
      'status-warning',
      'status-warning-text',
      'status-info',
      'status-info-text',
    ],
  },
];

/** 한 토큰을 쓰는 조각들. 표 → 미리보기 강조가 이것을 읽는다. */
export function partsUsingToken(cssVar: string): readonly string[] {
  const name = cssVar.replace(/^--color-/, '');
  return PREVIEW_PARTS.filter((p) => p.tokens.includes(name)).map((p) => p.id);
}

/** 한 조각이 쓰는 토큰들의 CSS 변수명. 미리보기 → 표 강조가 이것을 읽는다. */
export function tokensOfPart(partId: string): readonly string[] {
  const part = PREVIEW_PARTS.find((p) => p.id === partId);
  return part === undefined ? [] : part.tokens.map((t) => `--color-${t}`);
}

/**
 * 그 토큰을 볼 수 있는 탭.
 *
 * 표에서 색을 짚었는데 그 색을 쓰는 조각이 지금 안 보이는 탭에 있으면, 강조는 아무
 * 데도 나타나지 않고 사용자는 "이 색은 아무 데도 안 쓰이나" 로 읽는다. 그래서 탭을
 * 따라가게 한다.
 *
 * `always` 조각만 쓰는 토큰은 어느 탭에서나 보이므로 `undefined` 를 낸다 — 탭을
 * 옮길 이유가 없다.
 */
export function areaForToken(cssVar: string): Exclude<PreviewArea, 'always'> | undefined {
  const name = cssVar.replace(/^--color-/, '');
  const using = PREVIEW_PARTS.filter((p) => p.tokens.includes(name));
  if (using.some((p) => p.area === 'always')) return undefined;
  const tabbed = using.find((p) => p.area !== 'always');
  return tabbed === undefined ? undefined : (tabbed.area as Exclude<PreviewArea, 'always'>);
}
