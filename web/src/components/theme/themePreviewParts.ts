// 미리보기 조각 ↔ 토큰 대응 — **한 곳**에 둔다.
//
// 표에서 조각으로, 조각에서 표로 — 강조는 양방향이다. 두 방향이 각자 대응표를 들면
// 반드시 어긋나고, 어긋난 쪽은 "이 토큰이 어디 쓰이는지" 를 틀리게 알려 준다. 틀린
// 안내는 없느니만 못하다.
//
// @spec SPEC-THEME-001 §결정 3 · AC-05 · AC-08 (M5·M6)

/** 미리보기 한 조각. */
export interface PreviewPart {
  /** 조각 식별자 — `data-part` 와 테스트 아이디에 쓴다. */
  readonly id: string;
  /** 사람이 읽는 이름. */
  readonly label: string;
  /** 이 조각이 칠하는 데 쓰는 토큰들(`--color-` 를 뺀 이름). */
  readonly tokens: readonly string[];
}

/**
 * 일곱 조각.
 *
 * 하한은 "24토큰이 최소 한 번씩 나타난다" 이다(AC-05) — 나타나지 않는 토큰은
 * 미리보기로 고를 수 없다. 상한은 화면 크기다: 조각을 더 늘리면 축소 화면 안에서
 * 각 조각이 알아볼 수 없게 작아진다.
 */
export const PREVIEW_PARTS: readonly PreviewPart[] = [
  {
    id: 'shell',
    label: '앱 셸',
    tokens: ['bg-primary', 'bg-secondary', 'text-primary', 'border-subtle'],
  },
  {
    id: 'canvas',
    label: '편집기 캔버스',
    tokens: ['bg-secondary', 'flow-dot', 'flow-area'],
  },
  {
    id: 'node',
    label: '플로우 노드',
    tokens: ['bg-surface', 'bg-sunken', 'border-default', 'text-primary', 'text-muted'],
  },
  {
    id: 'edge',
    label: '연결선',
    tokens: ['flow-edge'],
  },
  {
    id: 'panel',
    label: '대시보드 패널',
    tokens: ['bg-surface', 'border-default', 'text-primary', 'text-secondary'],
  },
  {
    id: 'table',
    label: '목록 표',
    tokens: ['bg-sunken', 'bg-elevated', 'border-subtle', 'text-secondary', 'text-muted'],
  },
  {
    id: 'controls',
    label: '단추 · 상태',
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
