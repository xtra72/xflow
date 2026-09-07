// XFlow 디자인 토큰 시스템.
// CSS 변수 기반의 시맨틱 컬러 토큰을 Light/Dark 프리셋과 함께 정의한다.
//
// 이 파일이 컬러 테이블의 단일 원천(SSOT)이다. index.css 의 :root /
// [data-theme] 블록은 JS 없이도 초기 페인트가 올바르도록 같은 값을 복제해 두며,
// tokens.cssParity.test.ts 가 두 곳의 값이 어긋나지 않았는지 검증한다.

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

/** 토큰 이름(CSS 변수명)을 키, 헥스 컬러 값을 값으로 갖는 맵 */
export type ThemeTokens = Record<string, string>;

/** 편집 가능한 기본 팔레트 식별자. system 모드는 OS 설정에 따라 이 둘 중 하나로 해석된다. */
export type PresetId = 'day' | 'night';

/** 개별 토큰 정보 */
export interface TokenInfo {
  /** 식별 이름 (예: "bg-primary") */
  name: string;
  /** CSS 변수명 (예: "--color-bg-primary") */
  cssVar: string;
  /** 사람이 읽을 수 있는 라벨 */
  label: string;
  /** 이 토큰이 어디에 쓰이는지에 대한 짧은 설명 */
  hint: string;
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
      { name: 'bg-primary', cssVar: '--color-bg-primary', label: '기본 배경', hint: '페이지 바탕' },
      { name: 'bg-secondary', cssVar: '--color-bg-secondary', label: '보조 배경', hint: '사이드바 · 헤더' },
      { name: 'bg-surface', cssVar: '--color-bg-surface', label: '표면 배경', hint: '카드 · 패널' },
      { name: 'bg-elevated', cssVar: '--color-bg-elevated', label: '부유 배경', hint: '모달 · 드롭다운' },
      { name: 'bg-sunken', cssVar: '--color-bg-sunken', label: '함몰 배경', hint: '입력창 · 테이블 헤더' },
    ],
  },
  {
    category: 'text',
    label: '텍스트',
    tokens: [
      { name: 'text-primary', cssVar: '--color-text-primary', label: '기본 텍스트', hint: '본문 · 제목' },
      { name: 'text-secondary', cssVar: '--color-text-secondary', label: '보조 텍스트', hint: '레이블 · 설명' },
      { name: 'text-muted', cssVar: '--color-text-muted', label: '비활성 텍스트', hint: '힌트 · 캡션' },
      { name: 'text-inverse', cssVar: '--color-text-inverse', label: '반전 텍스트', hint: '어두운 배경 위 글자' },
    ],
  },
  {
    category: 'border',
    label: '테두리',
    tokens: [
      { name: 'border-default', cssVar: '--color-border-default', label: '기본 테두리', hint: '카드 · 입력창' },
      { name: 'border-subtle', cssVar: '--color-border-subtle', label: '미세 테두리', hint: '구분선' },
      { name: 'border-strong', cssVar: '--color-border-strong', label: '강조 테두리', hint: '호버 · 포커스' },
    ],
  },
  {
    category: 'status',
    label: '상태',
    tokens: [
      { name: 'status-running', cssVar: '--color-status-running', label: '실행 중', hint: '정상 · 성공' },
      { name: 'status-stopped', cssVar: '--color-status-stopped', label: '정지', hint: '중립 · 비활성' },
      { name: 'status-error', cssVar: '--color-status-error', label: '오류', hint: '실패 · 위험' },
      { name: 'status-warning', cssVar: '--color-status-warning', label: '경고', hint: '주의' },
      { name: 'status-info', cssVar: '--color-status-info', label: '정보', hint: '안내' },
    ],
  },
  {
    category: 'interactive',
    label: '인터랙티브',
    tokens: [
      { name: 'interactive-primary', cssVar: '--color-interactive-primary', label: '기본 상호작용', hint: '주요 버튼 · 링크' },
      { name: 'interactive-hover', cssVar: '--color-interactive-hover', label: '호버 상호작용', hint: '마우스 오버' },
      { name: 'interactive-active', cssVar: '--color-interactive-active', label: '활성 상호작용', hint: '눌림 · 선택' },
      { name: 'interactive-muted', cssVar: '--color-interactive-muted', label: '비활성 상호작용', hint: '선택 배경 · 비활성' },
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

/** CSS 변수명 -> 토큰 메타데이터 조회 맵 */
export const TOKEN_BY_VAR: Record<string, TokenInfo> = Object.fromEntries(
  TOKEN_CATEGORIES.flatMap((cat) => cat.tokens).map((t) => [t.cssVar, t]),
);

// ---------------------------------------------------------------------------
// Day 프리셋 (라이트 모드)
// ---------------------------------------------------------------------------

/**
 * Day(라이트) 테마 프리셋 값.
 *
 * v0.7.0: 라이트 모드가 "너무 밝다"는 문제를 해소하기 위해 표면 계층을 분리했다.
 * 이전에는 secondary/surface/elevated 가 모두 `#ffffff` 라 카드·모달·사이드바가
 * 한 덩어리 흰 면으로 보였다. 이제 sunken < primary < secondary < surface <
 * elevated 순으로 밝아지며, 순백은 최상단(모달·드롭다운)에만 남는다.
 */
export const DAY_PRESET: ThemeTokens = {
  // 배경 — 아래로 갈수록 밝다
  '--color-bg-sunken': '#e2e6ec', // 입력창 · 테이블 헤더
  '--color-bg-primary': '#eceff4', // 페이지 바탕
  '--color-bg-secondary': '#f4f6f9', // 사이드바 · 헤더
  '--color-bg-surface': '#fbfcfe', // 카드 · 패널
  '--color-bg-elevated': '#ffffff', // 모달 · 드롭다운

  // 텍스트
  '--color-text-primary': '#111827', // gray-900
  '--color-text-secondary': '#374151', // gray-700
  '--color-text-muted': '#6b7280', // gray-500
  '--color-text-inverse': '#ffffff', // white

  // 테두리
  '--color-border-default': '#dfe3e9',
  '--color-border-subtle': '#eaedf2',
  '--color-border-strong': '#c9cfd8',

  // 상태
  '--color-status-running': '#16a34a', // green-600
  '--color-status-stopped': '#6b7280', // gray-500
  '--color-status-error': '#dc2626', // red-600
  '--color-status-warning': '#ca8a04', // yellow-600
  '--color-status-info': '#2563eb', // blue-600

  // 인터랙티브
  '--color-interactive-primary': '#2563eb', // blue-600
  '--color-interactive-hover': '#1d4ed8', // blue-700
  '--color-interactive-active': '#1e40af', // blue-800
  '--color-interactive-muted': '#dbeafe', // blue-100 — 선택 행 배경
};

// ---------------------------------------------------------------------------
// Night 프리셋 (다크 모드)
// ---------------------------------------------------------------------------

/** Night(다크) 테마 프리셋 값. Tailwind 기본 팔레트 기준. */
export const NIGHT_PRESET: ThemeTokens = {
  // 배경 — 아래로 갈수록 밝다
  '--color-bg-sunken': '#030712', // gray-950
  '--color-bg-primary': '#111827', // gray-900
  '--color-bg-secondary': '#1a2333',
  '--color-bg-surface': '#1f2937', // gray-800
  '--color-bg-elevated': '#374151', // gray-700

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
  '--color-interactive-muted': '#1e3a8a', // blue-900 — 선택 행 배경
};

/** 프리셋 식별자 -> 기본 팔레트 */
export const PRESETS: Record<PresetId, ThemeTokens> = {
  day: DAY_PRESET,
  night: NIGHT_PRESET,
};

/** 편집 UI 에서 사용하는 프리셋 목록(표시 순서 고정) */
export const PRESET_IDS: PresetId[] = ['day', 'night'];

// ---------------------------------------------------------------------------
// 팔레트 해석 / 검증
// ---------------------------------------------------------------------------

/**
 * 기본 프리셋에 사용자 오버라이드를 덮어 최종 팔레트를 만든다.
 * 오버라이드에 없는 토큰은 프리셋 기본값을 그대로 쓴다.
 */
export function resolveTokens(preset: PresetId, overrides?: ThemeTokens): ThemeTokens {
  const base = PRESETS[preset];
  const out: ThemeTokens = {};
  for (const v of ALL_TOKEN_VARS) {
    out[v] = overrides?.[v] || base[v] || '';
  }
  return out;
}

/** `#rgb` / `#rrggbb` 형식의 헥스 컬러인지 검사한다. */
export function isValidHexColor(value: string): boolean {
  return /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.test(value.trim());
}

/**
 * 오버라이드 맵을 정규화한다.
 * - 알 수 없는 CSS 변수는 버린다
 * - 헥스 형식이 아닌 값은 버린다
 * - 프리셋 기본값과 같은 값은 오버라이드로 저장하지 않는다(불필요한 영속 방지)
 */
export function normalizeOverrides(preset: PresetId, tokens: ThemeTokens): ThemeTokens {
  const base = PRESETS[preset];
  const out: ThemeTokens = {};
  for (const [cssVar, raw] of Object.entries(tokens)) {
    if (!ALL_TOKEN_VARS.includes(cssVar)) continue;
    const value = String(raw).trim().toLowerCase();
    if (!isValidHexColor(value)) continue;
    if (value === (base[cssVar] ?? '').toLowerCase()) continue;
    out[cssVar] = value;
  }
  return out;
}

// ---------------------------------------------------------------------------
// 테마 파일 (내보내기 / 가져오기)
// ---------------------------------------------------------------------------

/** 내보내기 파일의 스키마 식별자 */
export const THEME_FILE_SCHEMA = 'xflow-theme';
/** 내보내기 파일의 스키마 버전 */
export const THEME_FILE_VERSION = 1;

/** `.json` 으로 주고받는 테마 팔레트 파일 형식 */
export interface ThemeFile {
  schema: typeof THEME_FILE_SCHEMA;
  version: number;
  /** 이 팔레트가 어느 모드용인지 (가져오기 시 기본 대상) */
  preset: PresetId;
  /** 해석된 전체 토큰 값 (오버라이드가 아니라 완전한 팔레트) */
  tokens: ThemeTokens;
}

/** 현재 팔레트를 내보내기용 JSON 문자열로 직렬화한다. */
export function serializeThemeFile(preset: PresetId, tokens: ThemeTokens): string {
  const file: ThemeFile = {
    schema: THEME_FILE_SCHEMA,
    version: THEME_FILE_VERSION,
    preset,
    tokens: Object.fromEntries(ALL_TOKEN_VARS.map((v) => [v, tokens[v] ?? ''])),
  };
  return `${JSON.stringify(file, null, 2)}\n`;
}

/** parseThemeFile 결과 */
export type ParseThemeResult =
  | { ok: true; preset: PresetId; tokens: ThemeTokens; ignored: number }
  | { ok: false; error: 'invalid-json' | 'invalid-schema' | 'no-valid-tokens' };

/**
 * 가져오기 파일 내용을 파싱·검증한다.
 * 알 수 없는 키나 헥스가 아닌 값은 버리고 몇 개를 버렸는지 `ignored` 로 알린다.
 */
export function parseThemeFile(text: string): ParseThemeResult {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { ok: false, error: 'invalid-json' };
  }

  if (typeof raw !== 'object' || raw === null) return { ok: false, error: 'invalid-schema' };
  const obj = raw as Record<string, unknown>;
  if (obj.schema !== THEME_FILE_SCHEMA) return { ok: false, error: 'invalid-schema' };
  if (typeof obj.tokens !== 'object' || obj.tokens === null) {
    return { ok: false, error: 'invalid-schema' };
  }

  const preset: PresetId = obj.preset === 'night' ? 'night' : 'day';
  const entries = Object.entries(obj.tokens as Record<string, unknown>);
  const tokens: ThemeTokens = {};
  for (const [cssVar, value] of entries) {
    if (!ALL_TOKEN_VARS.includes(cssVar)) continue;
    if (typeof value !== 'string' || !isValidHexColor(value)) continue;
    tokens[cssVar] = value.trim().toLowerCase();
  }

  if (Object.keys(tokens).length === 0) return { ok: false, error: 'no-valid-tokens' };
  return { ok: true, preset, tokens, ignored: entries.length - Object.keys(tokens).length };
}
