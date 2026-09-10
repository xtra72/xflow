// SVG 경로 데이터의 토크나이저와 축약 (SPEC-CANVAS-007 M1).
//
// **이 모듈은 DOM 을 모른다** — 문자열이 들어오고 숫자가 나간다. 그래서 축약 산술의 결함이
// 트리 순회의 결함으로 위장하지 못한다(불변식 K7 이 지키는 값의 절반이 이것이다).
//
// **무엇이 넷이 되는가.** SVG 경로는 명령 열 가지(`M L H V C S Q T A Z`)에 상대형까지
// 스무 갈래인데 `PathCommand` 는 넷(`M`·`L`·`C`·`Z`)이다. `H`/`V`/`S`/`Q`/`T`/상대는
// **항등 변환**이라 무손실로 접히고, `A` 만 근사다(그 근사는 `svgArc.ts` 가 한다).
//
// **기각한 안 — 정규식 하나로 명령을 자른다.** 이 저장소의 `heatmap/svgAsset.ts` 가 정규식을
// 쓰지만 그것은 **루트 여는 태그 하나**만 읽기 때문이다. 경로 데이터의 문법에는 정규식이
// 표현하지 못하는 문맥이 셋 있다.
//
//   1. **플래그는 한 글자다.** `a50 30 20 0160 20` 은 합법이며 `fa=0 fs=1 x=60` 을 뜻한다.
//      SVGO · Illustrator · Figma 의 내보내기가 정확히 이 형태를 낸다. 플래그를 일반 수로
//      읽는 토크나이저는 `0160` 을 한 수로 삼켜 **그 뒤 전부가 밀린다** — 그림이 알아볼 수
//      없게 되지만 예외는 나지 않는다(위험 R2).
//   2. **`5.5.5` 는 두 수다**(`5.5` 그리고 `.5`). `parseFloat` 로 한 번에 읽으면 하나가 된다.
//   3. **부호가 곧 구분자다**(`10-20` 은 두 수다). 그리고 `.5` · `-.5` · `1e-5` · `1E5` 가
//      전부 합법이다.
//
// 셋 다 "어느 명령을 읽는 중인가" 와 "몇 번째 인자인가" 를 알아야 판정된다. 그래서 위치를
// 들고 앞으로 나아가는 스캐너다.
//
// **호를 이 모듈이 축약하지 않는 이유.** 호 축약은 좌표 하나가 아니라 **끝점→중심
// 매개변수화**라는 다른 산술이고, `transform` 보다 반드시 먼저 일어나야 한다는 순서 제약
// (불변식 K2)을 제 모듈에서 지는 편이 형상으로 읽힌다. 여기서는 축약기를 **인자로 받는다** —
// 그러면 변환 행렬이 이 모듈로 들어올 자리가 애초에 없다.
//
// @spec SPEC-CANVAS-007 REQ-01 · REQ-07

import type { PathCommand } from '../shapes/pathTypes';

import { arcToCubics } from './svgArc';

// --- 토큰 ---------------------------------------------------------------

/** 토크나이저가 내는 명령 글자. 대소문자가 곧 절대/상대다. */
export type PathSegmentCode =
  | 'M'
  | 'm'
  | 'L'
  | 'l'
  | 'H'
  | 'h'
  | 'V'
  | 'v'
  | 'C'
  | 'c'
  | 'S'
  | 's'
  | 'Q'
  | 'q'
  | 'T'
  | 't'
  | 'A'
  | 'a'
  | 'Z'
  | 'z';

/** 명령 한 개와 그 인자들. 인자 수는 명령마다 고정이며 **이미 검증되어 있다**. */
export interface PathSegment {
  readonly code: PathSegmentCode;
  readonly args: readonly number[];
}

/** 토크나이즈 결과. 버린 명령 수를 함께 들고 나오는 것은 보고가 그것을 셀 수 있게 하기 위해서다. */
export interface TokenizeResult {
  readonly segments: readonly PathSegment[];
  /** 손상되어 버린 명령 수(문법 오류 · 비유한 좌표). */
  readonly dropped: number;
}

/** 명령별 인자 수. `Z` 만 0 이다. */
const ARG_COUNT: Readonly<Record<PathSegmentCode, number>> = {
  M: 2,
  m: 2,
  L: 2,
  l: 2,
  H: 1,
  h: 1,
  V: 1,
  v: 1,
  C: 6,
  c: 6,
  S: 4,
  s: 4,
  Q: 4,
  q: 4,
  T: 2,
  t: 2,
  A: 7,
  a: 7,
  Z: 0,
  z: 0,
};

/**
 * 좌표쌍이 이어졌을 때 두 번째부터 무엇으로 읽는가.
 *
 * **`M` → `L` · `m` → `l` 이 이 표의 전부다.** 이것을 "M 이 여러 개" 로 읽으면 채워야 할
 * 도형이 **점들의 나열**이 되어 화면에서 사라진다. 나머지 명령은 자기 자신을 되풀이한다.
 */
const IMPLICIT_REPEAT: Readonly<Partial<Record<PathSegmentCode, PathSegmentCode>>> = {
  M: 'L',
  m: 'l',
};

function isCommandCode(ch: string | undefined): ch is PathSegmentCode {
  return ch !== undefined && 'MmLlHhVvCcSsQqTtAaZz'.includes(ch);
}

function isDigit(ch: string | undefined): boolean {
  return ch !== undefined && ch >= '0' && ch <= '9';
}

/** SVG 의 wsp — 공백 · 탭 · CR · LF · 폼피드. */
function isWsp(ch: string | undefined): boolean {
  return ch === ' ' || ch === '\t' || ch === '\r' || ch === '\n' || ch === '\f';
}

// --- 스캐너 -------------------------------------------------------------

/**
 * SVG 수 문법 하나를 가진 스캐너.
 *
 * **저장소에 두 번째 수 문법을 만들지 않는다.** `points` 속성도(`svgShapes`) `transform`
 * 인자도(`svgTransform`) 이 스캐너를 지난다 — 세 자리가 각자 `parseFloat` 를 부르면
 * `5.5.5` 를 셋이 서로 다르게 읽는다.
 */
class NumberScanner {
  private readonly src: string;
  private pos = 0;

  constructor(src: string) {
    this.src = src;
  }

  get done(): boolean {
    return this.pos >= this.src.length;
  }

  at(): string | undefined {
    return this.src[this.pos];
  }

  advance(): void {
    this.pos += 1;
  }

  skipWsp(): void {
    while (isWsp(this.at())) this.pos += 1;
  }

  /** wsp* comma? wsp* — 구분자를 건너뛴다. 부호는 건너뛰지 않는다(부호가 곧 구분자다). */
  skipCommaWsp(): void {
    this.skipWsp();
    if (this.at() === ',') {
      this.pos += 1;
      this.skipWsp();
    }
  }

  /**
   * 수 하나. 읽을 수 없으면 **위치를 되돌리고** `undefined` 를 낸다.
   *
   * 지수부의 `e` 를 되돌리는 갈래가 이 함수에서 가장 잘 빠지는 자리다 — `1e` 뒤에 자릿수가
   * 없으면 그 `e` 는 수의 일부가 아니라 다음 토큰이다.
   */
  readNumber(): number | undefined {
    this.skipCommaWsp();
    const start = this.pos;
    if (this.at() === '+' || this.at() === '-') this.pos += 1;
    let digits = 0;
    while (isDigit(this.at())) {
      this.pos += 1;
      digits += 1;
    }
    if (this.at() === '.') {
      this.pos += 1;
      while (isDigit(this.at())) {
        this.pos += 1;
        digits += 1;
      }
    }
    if (digits === 0) {
      this.pos = start;
      return undefined;
    }
    if (this.at() === 'e' || this.at() === 'E') {
      const beforeExponent = this.pos;
      this.pos += 1;
      if (this.at() === '+' || this.at() === '-') this.pos += 1;
      let exponentDigits = 0;
      while (isDigit(this.at())) {
        this.pos += 1;
        exponentDigits += 1;
      }
      if (exponentDigits === 0) this.pos = beforeExponent;
    }
    // `1e999` 는 문법에 맞고 값이 Infinity 다. 유한 검사는 호출부가 한다 — 여기서
    // 떨어뜨리면 "문법이 틀렸다" 와 "값이 범위를 넘었다" 가 구분되지 않는다.
    return Number(this.src.slice(start, this.pos));
  }

  /**
   * 플래그 하나 — **정확히 한 글자**다. `0` 또는 `1` 이 아니면 읽지 않는다.
   *
   * 이 함수가 `readNumber` 와 갈라져 있는 것이 붙은 플래그(`0160`)를 사는 유일한 이유다.
   */
  readFlag(): 0 | 1 | undefined {
    this.skipCommaWsp();
    const ch = this.at();
    if (ch === '0') {
      this.pos += 1;
      return 0;
    }
    if (ch === '1') {
      this.pos += 1;
      return 1;
    }
    return undefined;
  }

  /** 손상된 명령을 만난 자리에서 다음 명령 글자까지 건너뛴다. 그래야 나머지가 산다. */
  skipToNextCommand(): void {
    while (!this.done && !isCommandCode(this.at())) this.pos += 1;
  }
}

// --- 공개 수 목록 -------------------------------------------------------

/**
 * 공백·쉼표로 구분된 수 목록(`points` 속성 · `transform` 인자).
 *
 * 수가 아닌 것을 만나면 **거기서 멈추고 지금까지 읽은 것을 돌려준다** — SVG 사양이
 * "오류 지점까지 렌더" 라고 정한 그 규율이다. 예외를 던지지 않는다(REQ-07).
 */
export function parseNumberList(raw: string): number[] {
  const scanner = new NumberScanner(raw);
  const out: number[] = [];
  for (;;) {
    const n = scanner.readNumber();
    if (n === undefined) break;
    out.push(n);
  }
  return out;
}

// --- 토크나이즈 ---------------------------------------------------------

/**
 * 경로 데이터 문자열을 명령 목록으로 자른다.
 *
 * 손상 정책은 **명령 단위**다(REQ-07): 인자 하나가 문법에 안 맞거나 비유한이면 **그 명령
 * 하나만** 버리고 다음 명령 글자에서 다시 시작한다. 도형 전체도 문서 전체도 죽이지 않는다.
 */
export function tokenizePathData(d: string): TokenizeResult {
  const scanner = new NumberScanner(d);
  const segments: PathSegment[] = [];
  let dropped = 0;
  let previous: PathSegmentCode | undefined;

  scanner.skipWsp();
  while (!scanner.done) {
    let code: PathSegmentCode;
    const ch = scanner.at();
    if (isCommandCode(ch)) {
      scanner.advance();
      code = ch;
    } else if (previous !== undefined) {
      // 좌표쌍이 이어졌다 — 암묵 되풀이. `M` 뒤는 `L`, `m` 뒤는 `l` 이다.
      code = IMPLICIT_REPEAT[previous] ?? previous;
      if (ARG_COUNT[code] === 0) break; // `Z` 는 되풀이될 수 없다.
    } else {
      break; // 명령 글자 없이 시작한 쓰레기.
    }

    const argCount = ARG_COUNT[code];
    if (argCount === 0) {
      segments.push({ code, args: [] });
      previous = code;
      scanner.skipCommaWsp();
      continue;
    }

    const args: number[] = [];
    let broken = false;
    let malformed = false;
    for (let i = 0; i < argCount; i += 1) {
      // 호의 4·5번째 인자(0-기준 3·4)만 플래그다.
      const isFlag = (code === 'A' || code === 'a') && (i === 3 || i === 4);
      const value = isFlag ? scanner.readFlag() : scanner.readNumber();
      if (value === undefined) {
        malformed = true;
        break;
      }
      if (!Number.isFinite(value)) broken = true;
      args.push(value);
    }

    if (malformed) {
      // 스트림이 어긋났다 — 다음 명령 글자에서 다시 잡는다.
      dropped += 1;
      previous = undefined;
      scanner.skipToNextCommand();
      continue;
    }
    if (broken) {
      // 문법은 맞고 값이 비유한이다. 위치는 성하므로 그대로 이어 읽는다.
      dropped += 1;
      previous = code;
      scanner.skipCommaWsp();
      continue;
    }

    segments.push({ code, args });
    previous = code;
    scanner.skipCommaWsp();
  }

  return { segments, dropped };
}

// --- 축약 ---------------------------------------------------------------

/**
 * 호 하나를 3차 베지어들로 옮기는 함수. **인자로 받는다**(불변식 K2 — 변환 행렬이 호
 * 산술에 닿을 자리를 형상으로 없앤다).
 *
 * 빈 배열을 내면 그 호는 그릴 것이 없다는 뜻이며(두 끝점이 같은 경우), 현재 점도
 * 움직이지 않는다.
 */
export type ArcReducer = (
  x0: number,
  y0: number,
  rx: number,
  ry: number,
  xAxisRotationDeg: number,
  largeArc: 0 | 1,
  sweep: 0 | 1,
  x1: number,
  y1: number,
) => readonly PathCommand[];

interface ReduceState {
  /** 현재 점(절대). */
  cx: number;
  cy: number;
  /** 현재 부분 경로의 시작점 — **`Z` 뒤의 현재 점이 이것이다**(직전 점이 아니다). */
  sx: number;
  sy: number;
  /** 직전이 `C`/`S` 였을 때의 둘째 제어점(절대). 아니면 `undefined`. */
  cubicCtrl: { x: number; y: number } | undefined;
  /** 직전이 `Q`/`T` 였을 때의 2차 제어점(절대). 아니면 `undefined`. */
  quadCtrl: { x: number; y: number } | undefined;
}

/** `2·p0 − ctrl` — 직전 제어점을 현재 점에 대해 반사한다. */
function reflect(
  ctrl: { x: number; y: number } | undefined,
  cx: number,
  cy: number,
): { x: number; y: number } {
  // 폴백이 **현재 점**이라는 것이 이 함수에서 가장 잘 틀리는 자리다. 직전 제어점을
  // 그대로 쓰면 `L` 뒤의 `S` 가 엉뚱한 방향으로 휜다.
  if (ctrl === undefined) return { x: cx, y: cy };
  return { x: 2 * cx - ctrl.x, y: 2 * cy - ctrl.y };
}

/** 2차 베지어를 3차로 — **대수적 항등**이다(근사가 아니다). */
function quadraticToCubic(
  x0: number,
  y0: number,
  qx: number,
  qy: number,
  x1: number,
  y1: number,
): PathCommand {
  const twoThirds = 2 / 3;
  return {
    c: 'C',
    x1: x0 + twoThirds * (qx - x0),
    y1: y0 + twoThirds * (qy - y0),
    x2: x1 + twoThirds * (qx - x1),
    y2: y1 + twoThirds * (qy - y1),
    x: x1,
    y: y1,
  };
}

/**
 * 명령 목록을 `M`·`L`·`C`·`Z` 넷으로 축약한다.
 *
 * **첫 명령이 이동이 아니면 빈 목록이다.** SVG 사양이 그 문서를 오류로 정하고 아무것도
 * 그리지 않게 한다 — 없는 자리에서 선이 뻗어 나오는 그림을 지어내지 않는다.
 *
 * **첫 명령이 `m`(상대)이면 그 한 쌍이 절대가 되는 것은 현재 점을 원점으로 열기 때문이다**
 * (SVG 1.1 §8.3.2). 그 규칙을 따로 분기로 쓰지 않는다 — 첫 명령은 반드시 이동이고 그때
 * 현재 점은 언제나 `(0,0)` 이므로, 상대 덧셈이 곧 절대 대입이다. 분기를 세워 보았으나
 * **뮤테이션이 물지 않았다**(지워도 어느 시험도 빨개지지 않는다) — 아무것도 지키지 않는
 * 가드는 다음 사람에게 거짓말을 하므로 지운다. 이 규칙을 실제로 지키는 것은 아래
 * `ReduceState` 의 원점 초기화이며, 시험은 그것을 잰다.
 */
export function reducePathSegments(
  segments: readonly PathSegment[],
  arcToCubics: ArcReducer,
): PathCommand[] {
  const out: PathCommand[] = [];
  if (segments.length === 0) return out;
  const first = segments[0];
  if (first === undefined || (first.code !== 'M' && first.code !== 'm')) return out;

  // **원점에서 연다.** 첫 `m` 이 절대로 읽히는 것이 이 한 줄의 값이다(위 주석).
  const state: ReduceState = { cx: 0, cy: 0, sx: 0, sy: 0, cubicCtrl: undefined, quadCtrl: undefined };

  for (const seg of segments) {
    const relative = seg.code === seg.code.toLowerCase() && seg.code !== 'Z';
    // 상대 명령이 더하는 원점 — 현재 점이다.
    const ox = relative ? state.cx : 0;
    const oy = relative ? state.cy : 0;
    const a = seg.args;
    let nextCubicCtrl: { x: number; y: number } | undefined;
    let nextQuadCtrl: { x: number; y: number } | undefined;

    switch (seg.code) {
      case 'M':
      case 'm': {
        const x = (a[0] ?? 0) + ox;
        const y = (a[1] ?? 0) + oy;
        out.push({ c: 'M', x, y });
        state.cx = x;
        state.cy = y;
        state.sx = x;
        state.sy = y;
        break;
      }
      case 'L':
      case 'l': {
        const x = (a[0] ?? 0) + ox;
        const y = (a[1] ?? 0) + oy;
        out.push({ c: 'L', x, y });
        state.cx = x;
        state.cy = y;
        break;
      }
      case 'H':
      case 'h': {
        // 빠진 축은 **현재 점의 값**이다.
        const x = (a[0] ?? 0) + ox;
        out.push({ c: 'L', x, y: state.cy });
        state.cx = x;
        break;
      }
      case 'V':
      case 'v': {
        const y = (a[0] ?? 0) + oy;
        out.push({ c: 'L', x: state.cx, y });
        state.cy = y;
        break;
      }
      case 'C':
      case 'c': {
        const c1 = { x: (a[0] ?? 0) + ox, y: (a[1] ?? 0) + oy };
        const c2 = { x: (a[2] ?? 0) + ox, y: (a[3] ?? 0) + oy };
        const p = { x: (a[4] ?? 0) + ox, y: (a[5] ?? 0) + oy };
        out.push({ c: 'C', x1: c1.x, y1: c1.y, x2: c2.x, y2: c2.y, x: p.x, y: p.y });
        state.cx = p.x;
        state.cy = p.y;
        nextCubicCtrl = c2;
        break;
      }
      case 'S':
      case 's': {
        const c1 = reflect(state.cubicCtrl, state.cx, state.cy);
        const c2 = { x: (a[0] ?? 0) + ox, y: (a[1] ?? 0) + oy };
        const p = { x: (a[2] ?? 0) + ox, y: (a[3] ?? 0) + oy };
        out.push({ c: 'C', x1: c1.x, y1: c1.y, x2: c2.x, y2: c2.y, x: p.x, y: p.y });
        state.cx = p.x;
        state.cy = p.y;
        nextCubicCtrl = c2;
        break;
      }
      case 'Q':
      case 'q': {
        const q = { x: (a[0] ?? 0) + ox, y: (a[1] ?? 0) + oy };
        const p = { x: (a[2] ?? 0) + ox, y: (a[3] ?? 0) + oy };
        out.push(quadraticToCubic(state.cx, state.cy, q.x, q.y, p.x, p.y));
        state.cx = p.x;
        state.cy = p.y;
        nextQuadCtrl = q;
        break;
      }
      case 'T':
      case 't': {
        const q = reflect(state.quadCtrl, state.cx, state.cy);
        const p = { x: (a[0] ?? 0) + ox, y: (a[1] ?? 0) + oy };
        out.push(quadraticToCubic(state.cx, state.cy, q.x, q.y, p.x, p.y));
        state.cx = p.x;
        state.cy = p.y;
        nextQuadCtrl = q;
        break;
      }
      case 'A':
      case 'a': {
        const px = (a[5] ?? 0) + ox;
        const py = (a[6] ?? 0) + oy;
        const cubics = arcToCubics(
          state.cx,
          state.cy,
          a[0] ?? 0,
          a[1] ?? 0,
          a[2] ?? 0,
          (a[3] ?? 0) === 1 ? 1 : 0,
          (a[4] ?? 0) === 1 ? 1 : 0,
          px,
          py,
        );
        if (cubics.length === 0) break; // 두 끝점이 같다 — 호가 없다. 현재 점도 그대로다.
        for (const cmd of cubics) out.push(cmd);
        state.cx = px;
        state.cy = py;
        break;
      }
      case 'Z':
      case 'z': {
        out.push({ c: 'Z' });
        // **`Z` 뒤의 현재 점은 부분 경로의 시작점이다.** 직전 점으로 두면 `z` 뒤에 오는
        // 상대 명령이 어긋난 자리에서 출발한다 — 화면에서만 드러나는 부류다.
        state.cx = state.sx;
        state.cy = state.sy;
        break;
      }
    }

    state.cubicCtrl = nextCubicCtrl;
    state.quadCtrl = nextQuadCtrl;
  }

  return out;
}

/** 문자열 하나를 넷으로. 토크나이즈와 축약을 잇는 편의 입구다. */
export function reducePathData(d: string, arcReducer: ArcReducer): PathCommand[] {
  return reducePathSegments(tokenizePathData(d).segments, arcReducer);
}

/**
 * 실제 축약기를 물린 입구 — 문서 층(M5)이 부르는 것은 이것 하나다.
 *
 * 호 축약기를 인자로 남겨 둔 것은 시험이 그 위임을 **대역으로** 관측하기 위해서이고,
 * 이 함수는 그 인자를 한 곳에서 채워 호출부마다 고르는 일이 없게 한다.
 */
export function parseSvgPathData(d: string): PathCommand[] {
  return reducePathData(d, arcToCubics);
}
