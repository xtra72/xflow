// 색 대비 산술 — "읽히는가"를 눈이 아니라 수로 판정한다.
//
// 토큰화는 색을 옮기는 일이고, 옮긴 뒤 읽기 어려워졌는지는 "괜찮아 보인다"로 판정할
// 수 없다. WCAG 상대 휘도 공식이 그 판정의 유일한 근거다.
//
// oklch 변환이 함께 있는 것은 Tailwind v4 때문이다 — v4 는 기본 팔레트를 oklch 로
// 내므로(`--color-zinc-400: oklch(70.5% .015 286.067)`), 오늘 화면에 나오는 색과
// 토큰의 hex 값을 같은 공간에서 비교하려면 환산이 필요하다.
//
// @spec SPEC-THEME-001 §결정 1 (M1)

/** sRGB 8비트 삼원색. */
export interface Rgb {
  readonly r: number;
  readonly g: number;
  readonly b: number;
}

/** `#rrggbb` 를 삼원색으로. 여섯 자리만 받는다 — 팔레트에 알파는 없다. */
export function hexToRgb(hex: string): Rgb | null {
  const m = /^#([0-9a-f]{6})$/i.exec(hex.trim());
  if (m === null) return null;
  const n = parseInt(m[1]!, 16);
  return { r: (n >> 16) & 0xff, g: (n >> 8) & 0xff, b: n & 0xff };
}

/** 삼원색을 `#rrggbb` 로. */
export function rgbToHex({ r, g, b }: Rgb): string {
  const p = (v: number): string => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0');
  return `#${p(r)}${p(g)}${p(b)}`;
}

/**
 * oklch → sRGB.
 *
 * Björn Ottosson 의 Oklab 정의를 그대로 옮긴 것이다. 색역을 벗어난 값은 가두는데,
 * 이 파일이 다루는 것은 Tailwind 의 중립 팔레트뿐이라 실제로 벗어나는 값이 없다.
 *
 * @param l 밝기 0..1
 * @param c 채도
 * @param h 색상(도)
 */
export function oklchToRgb(l: number, c: number, h: number): Rgb {
  const rad = (h * Math.PI) / 180;
  const a = c * Math.cos(rad);
  const bb = c * Math.sin(rad);

  const lp = l + 0.3963377774 * a + 0.2158037573 * bb;
  const mp = l - 0.1055613458 * a - 0.0638541728 * bb;
  const sp = l - 0.0894841775 * a - 1.291485548 * bb;

  const l3 = lp ** 3;
  const m3 = mp ** 3;
  const s3 = sp ** 3;

  const lin = [
    4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3,
    -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3,
    -0.0041960863 * l3 - 0.7034186147 * m3 + 1.707614701 * s3,
  ].map((v) => {
    const enc = v <= 0.0031308 ? 12.92 * v : 1.055 * v ** (1 / 2.4) - 0.055;
    return Math.max(0, Math.min(255, Math.round(enc * 255)));
  });

  return { r: lin[0]!, g: lin[1]!, b: lin[2]! };
}

/** WCAG 2.x 상대 휘도. */
export function relativeLuminance({ r, g, b }: Rgb): number {
  const ch = (v: number): number => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * ch(r) + 0.7152 * ch(g) + 0.0722 * ch(b);
}

/**
 * 두 색의 WCAG 대비비(1..21).
 *
 * 본문 글자의 AA 기준은 4.5:1 이다. 이 함수는 판정하지 않고 수만 낸다 — 어느 기준을
 * 쓸지는 부르는 쪽이 정한다(큰 글자는 3:1 로도 통과한다).
 */
export function contrastRatio(fg: string, bg: string): number | null {
  const a = hexToRgb(fg);
  const b = hexToRgb(bg);
  if (a === null || b === null) return null;
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}
