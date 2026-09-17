// 캔버스 상태 전이 트윈 엔진 (SPEC-CANVAS-001 T9).
//
// 규칙 평가 결과(= 적용될 최종 스타일)가 직전 프레임과 달라지면 이전 스타일에서 새
// 스타일로 지정된 지속 시간·이징으로 보간한다(REQ-05, §트윈 명세). 이 모듈은 그 보간의
// **수학만** 갖는다 — 시계는 갖지 않는다.
//
// DOM 무의존 규율: `requestAnimationFrame`·`performance.now`·`Date.now`·`document`·
// `window` 를 일절 쓰지 않고, 시각은 전부 `nowMs: number` **인자**로 들어온다. 시계를
// 소유하는 쪽은 렌더 루프(T10 `CanvasSurface.tsx`)이며 여기는 수학만 맡는다. 그래서
// 이 엔진은 가짜 타이머 없이 결정론적으로 시험된다 — 같은 상태·같은 `nowMs` 는 언제나
// 같은 값을 준다.
//
// 재사용 축: SPEC-CANVAS-003(값 구동 애니메이션)이 이 엔진을 다시 쓰도록 설계했다
// (§명세 "트윈 엔진은 기하 수치도 보간할 수 있게 만들어 003 이 엔진을 다시 쓰지 않게
// 한다"). 그래서 보간 원시형(`lerpNumber`·`lerpColor`·`lerpNumericRecord`)을 상태 기계와
// 분리해 따로 내보낸다 — 003 은 기하 레코드를 `lerpNumericRecord` 로 통과시키면 되고,
// 설정 화면 미리보기는 원시형만 빌려 쓸 수 있다.
//
// @spec SPEC-CANVAS-001

import type { ElementStyle, TweenEasing, TweenSpec } from './canvasConfig';

// --- 이징 ---------------------------------------------------------------

/** 정규화 진행도(0..1)를 이징된 진행도(0..1)로 옮기는 함수. */
export type EasingFn = (t: number) => number;

/**
 * 진행도를 [0,1] 로 죈다. **NaN 은 0 으로 떨어진다** — `!(v > 0)` 이 음수와 NaN 을 한
 * 번에 잡으므로, 손상된 진행도가 곱셈을 타고 NaN 값으로 번지지 않는다.
 */
function clamp01(v: number): number {
  if (!(v > 0)) return 0;
  return v > 1 ? 1 : v;
}

/** 등속. f(0)=0, f(1)=1. */
export function easeLinear(t: number): number {
  return clamp01(t);
}

/** 가속 진입(2차). f(0)=0, f(0.5)=0.25, f(1)=1. */
export function easeIn(t: number): number {
  const k = clamp01(t);
  return k * k;
}

/** 감속 이탈(2차). f(0)=0, f(0.5)=0.75, f(1)=1. */
export function easeOut(t: number): number {
  const k = clamp01(t);
  return k * (2 - k);
}

/** 가속 후 감속(2차). f(0)=0, f(0.5)=0.5, f(1)=1. */
export function easeInOut(t: number): number {
  const k = clamp01(t);
  return k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2;
}

/** 이징 토큰 → 함수. config 파서가 토큰 4종을 보장하지만 조회는 방어적으로 한다. */
export const EASING_FUNCTIONS: Readonly<Record<TweenEasing, EasingFn>> = {
  linear: easeLinear,
  'ease-in': easeIn,
  'ease-out': easeOut,
  'ease-in-out': easeInOut,
};

/**
 * 이징 토큰을 함수로 푼다. 미지 토큰·미지정은 등속으로 떨어진다 — 알 수 없는 이징에
 * 임의의 곡선을 고르면 사용자가 저술하지 않은 움직임이 생긴다.
 */
export function easingFor(easing: TweenEasing | undefined): EasingFn {
  if (easing === undefined) return easeLinear;
  return EASING_FUNCTIONS[easing] ?? easeLinear;
}

// --- 보간 원시형 ---------------------------------------------------------

/**
 * 수치 선형 보간. `t` 는 [0,1] 로 죄므로 구간 밖 입력이 값을 넘겨 쏘지 않는다.
 *
 * 끝점이 비유한(NaN·Infinity)이면 **그쪽을 없는 셈 치고 반대편을 그대로 준다** —
 * 손상된 끝점 하나 때문에 결과 전체가 NaN 이 되어 캔버스가 통째로 사라지는 편보다,
 * 성한 쪽 값을 그리는 편이 화면에서 원인을 볼 수 있다.
 */
export function lerpNumber(a: number, b: number, t: number): number {
  if (!Number.isFinite(a)) return b;
  if (!Number.isFinite(b)) return a;
  return a + (b - a) * clamp01(t);
}

/** 해석된 색. 채널은 0..255, 알파는 0..1. */
interface Rgba {
  r: number;
  g: number;
  b: number;
  a: number;
}

const HEX3_RE = /^#[0-9a-f]{3}$/i;
const HEX6_RE = /^#[0-9a-f]{6}$/i;

/** 0..255 로 죄고 정수로 만든다. */
function clampChannel(v: number): number {
  if (!(v > 0)) return 0;
  return v > 255 ? 255 : Math.round(v);
}

/**
 * CSS 색 문자열을 해석한다. 지원 형식은 `#rgb` · `#rrggbb` · `rgb(r,g,b)` ·
 * `rgba(r,g,b,a)` 넷이며, 그 밖(이름 있는 색·그라디언트·CSS 변수·쓰레기 문자열)은
 * `null` 이다. 색 이름표를 여기서 흉내 내면(예: 'red' → #f00) 표가 늘 모자라고,
 * 모자란 만큼이 조용히 다른 색으로 보간된다.
 */
function parseColor(raw: string): Rgba | null {
  const s = raw.trim();

  if (HEX3_RE.test(s)) {
    // 그룹 캡처 대신 잘라 쓴다 — noUncheckedIndexedAccess 아래에서 undefined 가지가 생기지 않는다.
    const r = s.slice(1, 2);
    const g = s.slice(2, 3);
    const b = s.slice(3, 4);
    return { r: parseInt(r + r, 16), g: parseInt(g + g, 16), b: parseInt(b + b, 16), a: 1 };
  }
  if (HEX6_RE.test(s)) {
    return {
      r: parseInt(s.slice(1, 3), 16),
      g: parseInt(s.slice(3, 5), 16),
      b: parseInt(s.slice(5, 7), 16),
      a: 1,
    };
  }

  const lower = s.toLowerCase();
  if (!lower.startsWith('rgb(') && !lower.startsWith('rgba(')) return null;
  if (!s.endsWith(')')) return null;

  const parts = s.slice(s.indexOf('(') + 1, -1).split(',');
  if (parts.length < 3 || parts.length > 4) return null;

  const r = Number(parts[0]);
  const g = Number(parts[1]);
  const b = Number(parts[2]);
  // 알파 생략은 불투명이다. `rgb()` 에 4번째 칸이 와도 같은 자리로 읽는다(CSS 관용).
  const a = parts.length === 4 ? Number(parts[3]) : 1;
  // 백분율 표기(`rgb(100%, 0%, 0%)`)는 Number 가 NaN 을 주므로 여기서 해석 실패로 떨어진다.
  if (!Number.isFinite(r) || !Number.isFinite(g) || !Number.isFinite(b)) return null;
  if (!Number.isFinite(a)) return null;

  return { r: clampChannel(r), g: clampChannel(g), b: clampChannel(b), a: clamp01(a) };
}

/**
 * 정규 출력 형식. 알파가 1이면 `rgb(...)`, 아니면 `rgba(...)` 한 가지로만 낸다.
 * 출력 형식을 하나로 고정해야 되먹임(직전 프레임 값을 다음 트윈의 시작점으로 다시
 * 해석하는 리타깃)이 왕복 손실 없이 돈다.
 */
function formatRgba(c: Rgba): string {
  const r = clampChannel(c.r);
  const g = clampChannel(c.g);
  const b = clampChannel(c.b);
  if (c.a >= 1) return `rgb(${r}, ${g}, ${b})`;
  // 알파는 셋째 자리에서 끊는다 — 부동소수 꼬리(0.30000000000000004)가 문자열에 남지 않게.
  return `rgba(${r}, ${g}, ${b}, ${Math.round(c.a * 1000) / 1000})`;
}

/**
 * 색 보간. 명세대로 **RGB 선형 보간**이며 알파도 함께 보간한다(§트윈 명세).
 *
 * 폴백 규율: 두 끝점 중 **하나라도** 해석되지 않으면 추측하지 않고 목표 색으로 즉시
 * 스냅한다(입력 문자열을 그대로 돌려준다). 이름 있는 색·`var(--x)`·그라디언트는 이
 * 모듈이 값을 알 수 없고, 알 수 없는 값을 검정이나 투명으로 가정하면 화면에 사용자가
 * 저술하지 않은 색이 한 프레임 번쩍인다. 스냅은 애니메이션을 잃지만 색은 지킨다.
 */
export function lerpColor(a: string, b: string, t: number): string {
  const from = parseColor(a);
  const to = parseColor(b);
  if (from === null || to === null) return b;
  const k = clamp01(t);
  return formatRgba({
    r: lerpNumber(from.r, to.r, k),
    g: lerpNumber(from.g, to.g, k),
    b: lerpNumber(from.b, to.b, k),
    a: lerpNumber(from.a, to.a, k),
  });
}

/**
 * 수치 레코드 보간. SPEC-CANVAS-003 이 **기하 수치**(`BoxGeometry`·`LineGeometry`·
 * `PointGeometry` 는 모두 number 레코드다)를 같은 엔진으로 흘리기 위한 자리다.
 *
 * 결과의 형상은 `to` 의 형상이다 — `to` 에 없는 키는 결과에도 없고, `from` 에만 있는
 * 키는 버린다. 목표에 없는 칸을 살려 두면 어느 시점에 사라질지가 정해지지 않는다.
 * 양쪽에 다 있는 유한 수치만 보간하고, `from` 쪽이 없거나 손상된 칸은 목표값을 그대로
 * 쓴다(= 즉시 전환).
 */
export function lerpNumericRecord<T extends Record<string, number>>(from: T, to: T, t: number): T {
  const out: Record<string, number> = { ...to };
  const k = clamp01(t);
  for (const key of Object.keys(out)) {
    const a = from[key];
    const b = out[key];
    if (typeof a === 'number' && typeof b === 'number') out[key] = lerpNumber(a, b, k);
  }
  return out as T;
}

// --- 트윈 상태 기계 -------------------------------------------------------

/** 색으로 보간하는 스타일 키(§트윈 명세). */
const COLOR_KEYS = ['fill', 'stroke', 'textColor'] as const;
/** 수치로 보간하는 스타일 키(§트윈 명세). */
const NUMBER_KEYS = ['opacity', 'strokeWidth', 'fontSize'] as const;

/**
 * 진행 중인 트윈 한 건. **불변**이다 — 진행은 필드를 고쳐 나아가는 것이 아니라
 * `sampleTween(state, nowMs)` 로 매번 다시 계산한다. 그래서 같은 상태를 몇 번을
 * 샘플해도 같은 값이 나오고, 프레임을 건너뛰어도 값이 밀리지 않는다.
 *
 * 타입 축: `ElementStyle` 을 상한으로 하는 제네릭이다. T5 의
 * `ResolvedStyle = ElementStyle & { text?: string }` 를 그대로 넣으면 결과도
 * `ResolvedStyle` 로 나와 통합 층에 변환이 필요 없다. 기본 인자가 `ElementStyle` 이라
 * 제네릭을 쓰지 않는 호출부는 종전과 같은 형상을 본다.
 */
export interface TweenState<S extends ElementStyle = ElementStyle> {
  /** 트윈 시작 시점의 스타일(리타깃이면 그 순간 보간 중이던 값). */
  readonly from: S;
  /** 목표 스타일. 즉시 전환 속성은 첫 샘플부터 이 값이다. */
  readonly to: S;
  /** `beginTween` 이 받은 시각. 시계가 비유한이면 0 이며 이때 지속 시간도 0 이 된다. */
  readonly startMs: number;
  /** 0 이면 즉시 전환(첫 샘플에서 done). */
  readonly durationMs: number;
  readonly easing: TweenEasing;
}

/**
 * 경과 진행도(0..1). 이징 적용 **전** 값이다.
 *
 * 방어 세 가지가 여기 모인다.
 * 1. `durationMs <= 0` → 1(즉시 전환). 루프를 깨우지 않는다(REQ-05 유휴 정지).
 * 2. `nowMs` 가 비유한 → 1. 0(시작에 고정)으로 두면 `done` 이 영원히 false 라 rAF 가
 *    멈추지 않는다 — 고장난 시계로 루프를 붙잡느니 목표를 그리고 멈추는 편이 낫다.
 * 3. `nowMs` 가 시작 시각보다 이르다(시계 역행) → 0. 음수 진행도는 만들지 않는다.
 */
function progressAt(state: TweenState, nowMs: number): number {
  if (state.durationMs <= 0) return 1;
  if (!Number.isFinite(nowMs)) return 1;
  const elapsed = nowMs - state.startMs;
  if (elapsed <= 0) return 0;
  if (elapsed >= state.durationMs) return 1;
  return elapsed / state.durationMs;
}

/**
 * 한 프레임의 스타일을 만든다.
 *
 * 결과 형상은 **목표(`to`)의 형상**이다. 그 위에서 색·수치 6종만, 그것도 **양쪽 끝점이
 * 모두 있을 때만** 보간으로 덮어쓴다. 따라서
 * - 즉시 전환 속성(`visible`·`fontWeight`·`align`·`strokeDash`, 그리고 얹혀 온 `text`)은
 *   스프레드로 첫 프레임부터 목표값이 된다(§트윈 명세 "즉시 전환"). 012 가 더한
 *   `strokeDash` 가 여기 서는 것은 **조건문이 아니라 구조**다 — 보간 목록 둘(`COLOR_KEYS`·
 *   `NUMBER_KEYS`)에 이름이 없으면 자동으로 이 갈래이고, 이산 축에는 보간할 중간값이
 *   없으므로 그것이 옳다(012 §결정 D6 · AC-12).
 * - 한쪽에만 있는 키도 목표값(또는 목표의 부재)을 그대로 따른다 — 없는 끝점을 렌더
 *   기본값으로 가정하면 사용자가 지정하지 않은 값에서 출발하는 애니메이션이 생긴다.
 *   그 결과 NaN 도 undefined 도 수치 자리에 새어 들지 않는다.
 */
function interpolateStyle<S extends ElementStyle>(from: S, to: S, eased: number): S {
  // 보간분만 따로 모아 목표 위에 덮는다 — 제네릭 객체에 직접 쓰면 쓰기 타입이 좁혀지지 않는다.
  const patch: ElementStyle = {};
  for (const key of COLOR_KEYS) {
    const a = from[key];
    const b = to[key];
    if (typeof a === 'string' && typeof b === 'string') patch[key] = lerpColor(a, b, eased);
  }
  for (const key of NUMBER_KEYS) {
    const a = from[key];
    const b = to[key];
    if (typeof a === 'number' && typeof b === 'number') patch[key] = lerpNumber(a, b, eased);
  }
  return { ...to, ...patch };
}

/**
 * 트윈을 시작한다. 입력은 복사해 담으므로 호출자가 나중에 원본을 고쳐도 진행 중인
 * 트윈이 흔들리지 않는다.
 *
 * `spec` 이 없거나 `duration_ms` 가 0(또는 손상)이면 지속 시간 0 — 첫 샘플에서 바로
 * 끝나며 렌더 루프를 깨우지 않는다(REQ-05). 시계가 비유한이면 지속 시간을 0 으로
 * 떨어뜨린다: 시작 시각이 NaN 인 트윈은 영원히 끝나지 않기 때문이다.
 */
export function beginTween<S extends ElementStyle>(
  from: S,
  to: S,
  spec: TweenSpec | undefined,
  nowMs: number,
): TweenState<S> {
  const clockOk = Number.isFinite(nowMs);
  const requested =
    spec !== undefined && Number.isFinite(spec.duration_ms) && spec.duration_ms > 0
      ? spec.duration_ms
      : 0;
  return {
    from: { ...from },
    to: { ...to },
    startMs: clockOk ? nowMs : 0,
    durationMs: clockOk ? requested : 0,
    easing: spec !== undefined ? spec.easing : 'linear',
  };
}

/**
 * 주어진 시각의 스타일과 종료 여부를 낸다. 상태도 입력도 고치지 않고 늘 새 객체를 낸다.
 *
 * `done: true` 는 렌더 루프에게 "이 요소는 더 그릴 것이 없다" 는 뜻이다. 모든 요소가
 * done 이면 루프는 다음 프레임을 예약하지 않는다(§렌더 루프 1. 유휴 정지).
 */
export function sampleTween<S extends ElementStyle>(
  state: TweenState<S>,
  nowMs: number,
): { style: S; done: boolean } {
  const progress = progressAt(state, nowMs);
  if (progress >= 1) return { style: { ...state.to }, done: true };
  return { style: interpolateStyle(state.from, state.to, easingFor(state.easing)(progress)), done: false };
}

/**
 * 진행 중인 트윈의 목표를 갈아 끼운다(AC-03).
 *
 * 새 트윈은 원래의 `from` 이 아니라 **`nowMs` 시점에 보간되어 있던 값**에서 출발한다.
 * 그래서 리타깃 직전 프레임과 직후 프레임의 값이 이어지고, 값이 튀지 않는다
 * (§트윈 명세 "현재 보간 중인 값에서 새 목표로 다시 트윈한다").
 */
export function retargetTween<S extends ElementStyle>(
  state: TweenState<S>,
  to: S,
  spec: TweenSpec | undefined,
  nowMs: number,
): TweenState<S> {
  return beginTween(sampleTween(state, nowMs).style, to, spec, nowMs);
}
