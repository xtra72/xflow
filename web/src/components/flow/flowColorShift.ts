// 플로우 편집기 토큰화 — 색이 어디로 움직이는지 적은 표.
//
// 이 표가 SPEC-THEME-001 의 가장 값비싼 산출물이다. 111자리를 갈아 끼우는 동안 한
// 자리만 잘못된 토큰으로 보내면 **화면은 그려지고 시험도 통과하며 색만 다르다** —
// DOM 을 보는 시험은 클래스 이름이 바뀐 것을 회귀로 읽지 못한다.
//
// 그래서 교체보다 **먼저** 이 표를 못 박는다. 표는 클래스 쌍을 키로 삼는다: 29개
// 쌍이 111자리를 덮으므로, 판단해야 할 것은 111이 아니라 29다.
//
// 색이 "같아야 한다"고 적지 않은 이유는 spec.md §결정 1 에 있다 — 오늘 색은 Tailwind
// `zinc`(중립 회색)이고 토큰은 `gray`/`slate`(푸른 회색)라 같은 밝기 단이라도 같은
// 색이 아니다. 지킬 것은 "같다"가 아니라 **"모든 이동이 여기 적혀 있고, 어느 자리도
// 읽기 어려워지지 않는다"** 이다.
//
// @spec SPEC-THEME-001 §결정 1 · REQ-08 · 불변식 I5b (M1)

/**
 * Tailwind v4 중립 팔레트의 oklch 원값.
 *
 * v4 는 기본 팔레트를 oklch 로 낸다(`--color-zinc-400: oklch(70.5% .015 286.067)`).
 * 여기 적는 것은 빌드 산출물에서 그대로 읽어 온 값이며, `contrast.ts` 가 sRGB 로
 * 환산한다 — 손으로 옮긴 hex 를 적으면 Tailwind 가 팔레트를 손질했을 때 표가 조용히
 * 낡는다.
 */
export const ZINC_OKLCH: Readonly<Record<number, readonly [number, number, number]>> = {
  50: [0.985, 0, 0],
  100: [0.967, 0.001, 286.375],
  200: [0.92, 0.004, 286.32],
  300: [0.871, 0.006, 286.286],
  400: [0.705, 0.015, 286.067],
  500: [0.552, 0.016, 285.938],
  600: [0.442, 0.017, 285.786],
  700: [0.37, 0.013, 285.805],
  800: [0.274, 0.006, 286.033],
  900: [0.21, 0.006, 285.885],
};

/** 한 자리의 이동. */
export interface ColorShift {
  /** 무엇을 칠하는 자리인가 — 사람이 읽는 이름. */
  readonly where: string;
  /** 오늘 밝은 테마에서 쓰이는 `zinc` 단. */
  readonly lightShade: number;
  /**
   * 오늘 어두운 테마에서 쓰이는 `zinc` 단.
   *
   * `null` 은 **다크 변형이 없다**는 뜻이다 — 양쪽에서 같은 색으로 그려진다. 노드
   * 부제가 그 자리이며, 이 SPEC 이 고치는 것이 바로 그것이다.
   */
  readonly darkShade: number | null;
  /** 갈 곳. `--color-` 를 뺀 이름. */
  readonly token: string;
  /** 이 색이 놓이는 배경 토큰. 글자일 때만 채운다 — 대비 계산에 쓴다. */
  readonly on?: string;
  /** 왜 이 토큰인가. */
  readonly why: string;
}

/**
 * 29개 쌍이 111자리를 덮는다.
 *
 * `flow-*` 세 토큰은 이 표에 없다 — 그 셋은 값을 우리가 정하므로 이동이 0이고
 * (불변식 I5), `tokens.ts` 가 오늘 값을 그대로 담는다.
 */
export const FLOW_COLOR_SHIFTS: readonly ColorShift[] = [
  // --- 글자 ---
  {
    where: '노드 제목 · 툴바 강조 글자',
    lightShade: 900,
    darkShade: 100,
    token: 'text-primary',
    on: 'bg-surface',
    why: '본문·제목 층. 카드 위에서 가장 강한 글자다',
  },
  {
    where: '패널 제목 · 툴바 제목',
    lightShade: 800,
    darkShade: 100,
    token: 'text-primary',
    on: 'bg-surface',
    why: '위와 같은 층 — 900/800 을 갈라 둘 이유가 없었다',
  },
  {
    where: '본문 · 라벨',
    lightShade: 700,
    darkShade: 200,
    token: 'text-secondary',
    on: 'bg-surface',
    why: '레이블·설명 층',
  },
  {
    where: '아이콘 · 보조 글자',
    lightShade: 600,
    darkShade: 300,
    token: 'text-secondary',
    on: 'bg-surface',
    why: '위와 같은 층',
  },
  {
    where: '흐린 글자 · 부가 정보',
    lightShade: 500,
    darkShade: 400,
    token: 'text-muted',
    on: 'bg-surface',
    why: '힌트·캡션 층',
  },
  {
    where: '노드 부제(노드 타입) · 비활성 글자',
    lightShade: 400,
    // 다크 변형이 없다 — 밝은 테마에서도 어두운 테마에서도 zinc-400 이다.
    // 이 SPEC 이 고치는 자리이며, 이동표에서 대비가 **올라가는** 유일한 행이다.
    darkShade: null,
    token: 'text-muted',
    on: 'bg-surface',
    why: '힌트 층. day 에서 zinc-400 은 AA 를 넘지 못한다 — 토큰화가 곧 고침이다',
  },

  // --- 배경 ---
  {
    where: '아이콘 타일 · 약한 호버',
    lightShade: 100,
    darkShade: 800,
    token: 'bg-sunken',
    why: '카드 안에 들어앉은 면. 토큰 힌트가 "입력창·테이블 헤더"로 같은 층이다',
  },
  {
    where: '강한 호버 · 세로 구분선',
    lightShade: 200,
    darkShade: 700,
    token: 'bg-elevated',
    why: '약한 호버보다 한 단 위 — 두 단계 호버의 순서가 유지되어야 한다',
  },
  {
    where: '배지 · 연한 면',
    lightShade: 50,
    darkShade: 800,
    token: 'bg-secondary',
    why: '카드보다 살짝 다른 면. 가장 연한 단이다',
  },

  // --- 테두리 ---
  {
    where: '카드 · 입력 테두리',
    lightShade: 200,
    darkShade: 700,
    token: 'border-default',
    why: '토큰 힌트가 "카드·입력창"으로 그대로 맞는다',
  },
  {
    where: '미세 구분선',
    lightShade: 100,
    darkShade: null,
    token: 'border-subtle',
    why: '구분선 층',
  },

  // --- 링 (핸들·배지가 카드 배경을 오려 낸 것처럼 보이게 하는 테) ---
  {
    where: '핸들 · 경고 배지의 테',
    // 오늘은 `border-white dark:border-zinc-800` 이다. 흰색은 zinc 단이 아니므로
    // 밝은 쪽을 zinc-50 으로 근사해 적는다 — 이동표의 목적은 정확한 재현이 아니라
    // **어디로 얼마나 움직이는지**를 남기는 것이다.
    lightShade: 50,
    darkShade: 800,
    token: 'bg-surface',
    why: '테가 카드 배경과 같아야 핸들이 카드에서 오려 낸 것처럼 보인다',
  },
];
