// XFlow 디자인 토큰 시스템.
// CSS 변수 기반의 시맨틱 컬러 토큰을 Day/Night 프리셋과 함께 정의한다.
// 모든 컬러 값은 Tailwind CSS 기본 팔레트에서 추출하였다.

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

/** 토큰 이름(CSS 변수명)을 키, 헥스 컬러 값을 값으로 갖는 맵 */
export type ThemeTokens = Record<string, string>;

/** 개별 토큰 정보 */
export interface TokenInfo {
  /** 식별 이름 (예: "bg-primary") */
  name: string;
  /** CSS 변수명 (예: "--color-bg-primary") */
  cssVar: string;
  /** 사람이 읽을 수 있는 라벨 */
  label: string;
}

/** 카테고리별 토큰 그룹 */
export interface TokenCategory {
  /** 카테고리 식별자 (예: "background") */
  category: string;
  /** 카테고리 표시 라벨 */
  label: string;
  /** 소속 토큰 목록 */
  tokens: TokenInfo[];
}

// ---------------------------------------------------------------------------
// 토큰 카테고리 정의
// ---------------------------------------------------------------------------

/** 카테고리별로 분류된 전체 디자인 토큰 목록 */
export const TOKEN_CATEGORIES: TokenCategory[] = [
  {
    category: 'background',
    label: '배경',
    tokens: [
      { name: 'bg-primary', cssVar: '--color-bg-primary', label: '기본 배경' },
      { name: 'bg-secondary', cssVar: '--color-bg-secondary', label: '보조 배경' },
      { name: 'bg-surface', cssVar: '--color-bg-surface', label: '표면 배경' },
      { name: 'bg-elevated', cssVar: '--color-bg-elevated', label: '부유 배경' },
      { name: 'bg-sunken', cssVar: '--color-bg-sunken', label: '함몰 배경' },
    ],
  },
  {
    category: 'text',
    label: '텍스트',
    tokens: [
      { name: 'text-primary', cssVar: '--color-text-primary', label: '기본 텍스트' },
      { name: 'text-secondary', cssVar: '--color-text-secondary', label: '보조 텍스트' },
      { name: 'text-muted', cssVar: '--color-text-muted', label: '비활성 텍스트' },
      { name: 'text-inverse', cssVar: '--color-text-inverse', label: '반전 텍스트' },
    ],
  },
  {
    category: 'border',
    label: '테두리',
    tokens: [
      { name: 'border-default', cssVar: '--color-border-default', label: '기본 테두리' },
      { name: 'border-subtle', cssVar: '--color-border-subtle', label: '미세 테두리' },
      { name: 'border-strong', cssVar: '--color-border-strong', label: '강조 테두리' },
    ],
  },
  {
    category: 'status',
    label: '상태',
    tokens: [
      { name: 'status-running', cssVar: '--color-status-running', label: '실행 중' },
      { name: 'status-stopped', cssVar: '--color-status-stopped', label: '정지' },
      { name: 'status-error', cssVar: '--color-status-error', label: '오류' },
      { name: 'status-warning', cssVar: '--color-status-warning', label: '경고' },
      { name: 'status-info', cssVar: '--color-status-info', label: '정보' },
    ],
  },
  {
    category: 'interactive',
    label: '인터랙티브',
    tokens: [
      { name: 'interactive-primary', cssVar: '--color-interactive-primary', label: '기본 상호작용' },
      { name: 'interactive-hover', cssVar: '--color-interactive-hover', label: '호버 상호작용' },
      { name: 'interactive-active', cssVar: '--color-interactive-active', label: '활성 상호작용' },
      { name: 'interactive-muted', cssVar: '--color-interactive-muted', label: '비활성 상호작용' },
    ],
  },
];

// ---------------------------------------------------------------------------
// 전체 CSS 변수명 목록
// ---------------------------------------------------------------------------

/** 모든 시맨틱 토큰의 CSS 변수명 배열 */
export const ALL_TOKEN_VARS: string[] = TOKEN_CATEGORIES.flatMap((cat) =>
  cat.tokens.map((t) => t.cssVar),
);

// ---------------------------------------------------------------------------
// Day 프리셋 (라이트 모드)
// ---------------------------------------------------------------------------

/** Day(라이트) 테마 프리셋 값. Tailwind 기본 팔레트 기준. */
export const DAY_PRESET: ThemeTokens = {
  // 배경
  '--color-bg-primary': '#f9fafb', // gray-50
  '--color-bg-secondary': '#ffffff', // white
  '--color-bg-surface': '#ffffff', // white
  '--color-bg-elevated': '#ffffff', // white
  '--color-bg-sunken': '#f3f4f6', // gray-100

  // 텍스트
  '--color-text-primary': '#111827', // gray-900
  '--color-text-secondary': '#374151', // gray-700
  '--color-text-muted': '#6b7280', // gray-500
  '--color-text-inverse': '#ffffff', // white

  // 테두리
  '--color-border-default': '#e5e7eb', // gray-200
  '--color-border-subtle': '#f3f4f6', // gray-100
  '--color-border-strong': '#d1d5db', // gray-300

  // 상태
  '--color-status-running': '#22c55e', // green-500
  '--color-status-stopped': '#6b7280', // gray-500
  '--color-status-error': '#ef4444', // red-500
  '--color-status-warning': '#eab308', // yellow-500
  '--color-status-info': '#3b82f6', // blue-500

  // 인터랙티브
  '--color-interactive-primary': '#3b82f6', // blue-500
  '--color-interactive-hover': '#2563eb', // blue-600
  '--color-interactive-active': '#1d4ed8', // blue-700
  '--color-interactive-muted': '#93c5fd', // blue-300
};

// ---------------------------------------------------------------------------
// Night 프리셋 (다크 모드)
// ---------------------------------------------------------------------------

/** Night(다크) 테마 프리셋 값. Tailwind 기본 팔레트 기준. */
export const NIGHT_PRESET: ThemeTokens = {
  // 배경
  '--color-bg-primary': '#111827', // gray-900
  '--color-bg-secondary': '#1f2937', // gray-800
  '--color-bg-surface': '#1f2937', // gray-800
  '--color-bg-elevated': '#374151', // gray-700
  '--color-bg-sunken': '#030712', // gray-950

  // 텍스트
  '--color-text-primary': '#f3f4f6', // gray-100
  '--color-text-secondary': '#d1d5db', // gray-300
  '--color-text-muted': '#9ca3af', // gray-400
  '--color-text-inverse': '#111827', // gray-900

  // 테두리
  '--color-border-default': '#374151', // gray-700
  '--color-border-subtle': '#1f2937', // gray-800
  '--color-border-strong': '#4b5563', // gray-600

  // 상태
  '--color-status-running': '#22c55e', // green-500
  '--color-status-stopped': '#9ca3af', // gray-400
  '--color-status-error': '#f87171', // red-400
  '--color-status-warning': '#facc15', // yellow-400
  '--color-status-info': '#60a5fa', // blue-400

  // 인터랙티브
  '--color-interactive-primary': '#3b82f6', // blue-500
  '--color-interactive-hover': '#60a5fa', // blue-400
  '--color-interactive-active': '#93c5fd', // blue-300
  '--color-interactive-muted': '#1e40af', // blue-800
};
