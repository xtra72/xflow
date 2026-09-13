// 색 형식을 아는 유일한 자리.
//
// 저장소에서 색 문자열의 자릿수를 세는 코드는 이 파일 하나뿐이어야 한다. 오늘
// `PanelColorFreeInput` 이 "값 형식 판정은 `normalizePanelColor` 한 곳에만 있다. 화면은
// 형식을 모른다" 고 적어 둔 규율을 그대로 잇고, 알파(8자리)까지 넓힌다.
//
// @spec SPEC-COLOR-001 §결정 4 · §결정 5 (M1)

/** 정규화된 색. 6자리, 또는 8자리(끝 두 자리 ≠ `ff`) 소문자 hex. */
export type ColorValue = string;

/** 색을 HSV 로 든 형태. `h` 는 0–359, `s`·`v`·`a` 는 0–1. */
export interface Hsva {
  h: number;
  s: number;
  v: number;
  a: number;
}

/** 16진 본문만 남았는지(`#` 과 공백을 뗀 뒤) 본다. */
const HEX_BODY = /^[0-9a-f]+$/;

/**
 * 8자리 hex 의 끝 두 자리가 `ff` 면 6자리로 접는다.
 *
 * 완전 불투명한 색의 표현을 **정확히 하나**로 두기 위함이다. 접지 않으면 `#3b82f6` 와
 * `#3b82f6ff` 가 같은 색인데도 문자열이 달라, 프리셋 선택 표시(문자열 비교)가 같은
 * 색에서 붙었다 안 붙었다 한다 — 오늘 대문자 문제와 정확히 같은 결함이다.
 */
function foldOpaque(hex: string): string {
  return hex.length === 9 && hex.endsWith('ff') ? hex.slice(0, 7) : hex;
}

/** 약식(3·4자리)을 각 자리를 두 번 써서 편다. */
function expandShorthand(body: string): string {
  return body.replace(/./g, (c) => c + c);
}

/**
 * 자유 입력 색을 저장 형식으로 맞춘다. 맞출 수 없으면 `null`.
 *
 * `normalizePanelColor` 가 정해 둔 셋을 그대로 잇는다 — (1) 3자리 약식을 펴고,
 * (2) 앞의 `#` 이 없어도 받고, (3) 소문자로 내린다. 세 번째가 형식 통일이 아니라
 * **동작**인 이유는 위 `foldOpaque` 의 머리말과 같다.
 *
 * `alpha` 가 꺼진 자리에서 8자리는 **거절이지 절삭이 아니다.** 잘라서 저장하면
 * 불투명도를 넣은 사용자에게 넣었다는 피드백을 준 뒤 조용히 버리게 된다. 거절하면
 * 초안이 눈앞에서 되돌아가므로 정직하다. 예외는 `#aabbccff` 로, 그것은 "알파 없음"
 * 과 **의미가 같으므로** 거절이 아니라 6자리로 접는다.
 *
 * `rgb()` · 이름 색 · `var(--x)` 는 받지 않는다 — 오늘도 그렇고, 이름 색 표를 흉내
 * 내면 표가 늘 모자라고 모자란 만큼이 조용히 다른 색이 된다.
 */
export function normalizeColor(input: string, opts: { alpha: boolean }): ColorValue | null {
  const body = input.trim().replace(/^#/, '').toLowerCase();
  if (!HEX_BODY.test(body)) return null;
  switch (body.length) {
    case 3:
      return `#${expandShorthand(body)}`;
    case 6:
      return `#${body}`;
    case 4:
      return opts.alpha ? foldOpaque(`#${expandShorthand(body)}`) : null;
    case 8:
      return opts.alpha ? foldOpaque(`#${body}`) : null;
    default:
      return null;
  }
}

/**
 * 색 위에 불투명도를 접는다. 결과 알파가 1 로 접히면 6자리로 되돌린다.
 *
 * 기존 알파와 **곱한다.** 사용자가 정한 반투명 위에 UI 의 틴트를 다시 접는 것이므로
 * 곱이 옳다. 이 규칙은 새것이 아니다 — `svgimport/svgStyle.ts` 의 `foldHex` 가 이미
 * 같은 곱(`existing * alpha`)을 한다.
 *
 * **읽을 수 없는 색이면 `undefined` 를 낸다.** 이것이 이 함수가 대체하는
 * `` `${색}NN` `` 이어붙이기와 가장 가까운 거동이다: 오늘 읽을 수 없는 색은 10자리
 * 무효 문자열이 되어 브라우저가 그 선언을 버린다. 원래 문자열을 그대로 돌려주면
 * 무효였던 선언이 **유효해져서** 오늘 없던 색이 화면에 나타난다.
 *
 * @param color 3·4·6·8자리 hex(`foldHex` 와 같은 범위). 그 밖은 `undefined`.
 * @param alpha 0–1. 범위 밖은 가둔다.
 */
export function withAlpha(color: string, alpha: number): ColorValue | undefined {
  if (!Number.isFinite(alpha)) return undefined;
  const base = normalizeColor(color, { alpha: true });
  if (base === null) return undefined;
  const existing = base.length === 9 ? parseInt(base.slice(7), 16) / 255 : 1;
  const folded = Math.min(1, Math.max(0, existing * alpha));
  const n = Math.round(folded * 255);
  if (n === 255) return base.slice(0, 7);
  return `${base.slice(0, 7)}${n.toString(16).padStart(2, '0')}`;
}

/**
 * hex 를 HSV 로 읽는다. 읽을 수 없으면 `null`.
 *
 * 2D 판을 끌 때 **HSV 를 상태로 들고** hex 는 그로부터 파생한다. hex 를 상태로 들면
 * 채도 0 이나 명도 0 에서 색상(hue)이 소실되어 슬라이더가 튄다.
 */
export function hexToHsv(hex: string): Hsva | null {
  const base = normalizeColor(hex, { alpha: true });
  if (base === null) return null;
  const r = parseInt(base.slice(1, 3), 16) / 255;
  const g = parseInt(base.slice(3, 5), 16) / 255;
  const b = parseInt(base.slice(5, 7), 16) / 255;
  const a = base.length === 9 ? parseInt(base.slice(7), 16) / 255 : 1;

  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const chroma = max - min;

  let h = 0;
  if (chroma !== 0) {
    if (max === r) h = ((g - b) / chroma) % 6;
    else if (max === g) h = (b - r) / chroma + 2;
    else h = (r - g) / chroma + 4;
    h *= 60;
    if (h < 0) h += 360;
  }
  return { h, s: max === 0 ? 0 : chroma / max, v: max, a };
}

/**
 * HSV 를 hex 로 되돌린다.
 *
 * `alpha` 가 꺼져 있거나 알파가 1 이면 6자리를 낸다(`foldOpaque` 와 같은 규칙).
 */
export function hsvToHex(hsv: Hsva, opts: { alpha: boolean }): ColorValue {
  const h = ((hsv.h % 360) + 360) % 360;
  const s = Math.min(1, Math.max(0, hsv.s));
  const v = Math.min(1, Math.max(0, hsv.v));

  const chroma = v * s;
  const x = chroma * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = v - chroma;
  const sector = Math.floor(h / 60) % 6;
  const rgb: readonly [number, number, number] =
    sector === 0
      ? [chroma, x, 0]
      : sector === 1
        ? [x, chroma, 0]
        : sector === 2
          ? [0, chroma, x]
          : sector === 3
            ? [0, x, chroma]
            : sector === 4
              ? [x, 0, chroma]
              : [chroma, 0, x];

  const body = rgb.map((c) => channelHex(c + m)).join('');
  if (!opts.alpha) return `#${body}`;
  const a = Math.round(Math.min(1, Math.max(0, hsv.a)) * 255);
  return a === 255 ? `#${body}` : `#${body}${a.toString(16).padStart(2, '0')}`;
}

/** 0–1 채널을 두 자리 소문자 hex 로. */
function channelHex(c: number): string {
  return Math.round(c * 255)
    .toString(16)
    .padStart(2, '0');
}
