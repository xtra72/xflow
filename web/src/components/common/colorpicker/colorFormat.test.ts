// @spec SPEC-COLOR-001 §결정 4 · §결정 5 (M1) — AC-04 · AC-E6
//
// `colorFormat.ts` 는 저장소에서 색 자릿수를 세는 유일한 자리이므로 **분기 100%** 를
// 목표로 한다. 표의 모든 행을 `alpha` 두 값 각각으로 친다.

import { describe, expect, it } from 'vitest';

import { hexToHsv, hsvToHex, normalizeColor, withAlpha } from './colorFormat';

const BLUE = '#3b82f6';
const BLUE_A50 = '#3b82f680';
const BLUE_FF = '#3b82f6ff';

describe('normalizeColor — spec.md §결정 4 의 표 전 행', () => {
  // [입력, alpha:false 기대, alpha:true 기대]
  const TABLE: ReadonlyArray<readonly [string, string | null, string | null]> = [
    ['#abc', '#aabbcc', '#aabbcc'],
    ['abc', '#aabbcc', '#aabbcc'],
    ['#AABBCC', '#aabbcc', '#aabbcc'],
    ['#aabbcc', '#aabbcc', '#aabbcc'],
    ['#abcd', null, '#aabbccdd'],
    ['#abcf', null, '#aabbcc'],
    ['#aabbccdd', null, '#aabbccdd'],
    ['#aabbccff', null, '#aabbcc'],
    ['#aabbccddee', null, null],
    ['rgb(1,2,3)', null, null],
    ['red', null, null],
    ['var(--x)', null, null],
    ['', null, null],
    ['  ', null, null],
  ];

  it.each(TABLE)('%s → alpha:false=%s · alpha:true=%s', (input, noAlpha, withA) => {
    expect(normalizeColor(input, { alpha: false })).toBe(noAlpha);
    expect(normalizeColor(input, { alpha: true })).toBe(withA);
  });

  it('앞뒤 공백과 `#` 생략을 받는다 — 오늘 `normalizePanelColor` 의 거동 그대로', () => {
    expect(normalizeColor('  3b82f6  ', { alpha: false })).toBe(BLUE);
    expect(normalizeColor('#', { alpha: true })).toBeNull();
  });

  it('8자리 `ff` 만 접는다 — `fe` 는 8자리로 남는다', () => {
    expect(normalizeColor(BLUE_FF, { alpha: true })).toBe(BLUE);
    expect(normalizeColor('#3b82f6fe', { alpha: true })).toBe('#3b82f6fe');
    // 6자리는 접기 대상이 아니다(`length === 9` 아님)
    expect(normalizeColor('#aabbcc', { alpha: true })).toBe('#aabbcc');
    // 끝 **두** 자리가 `ff` 여야 접는다. 한 자리만 보면 알파 `0f`(거의 투명)가
    // 불투명으로 접혀 사라진다 — 돌연변이 검사가 잡아낸 자리다.
    expect(normalizeColor('#aabbcc0f', { alpha: true })).toBe('#aabbcc0f');
  });

  it('4자리 약식의 `f` 도 `ff` 로 펴진 뒤 접힌다', () => {
    expect(normalizeColor('#abcf', { alpha: true })).toBe('#aabbcc');
    expect(normalizeColor('#abce', { alpha: true })).toBe('#aabbccee');
  });

  it('길이가 5·7·9 면 hex 라도 거절한다', () => {
    for (const body of ['abcde', 'abcdef0', 'abcdef012']) {
      expect(normalizeColor(`#${body}`, { alpha: true })).toBeNull();
    }
  });
});

describe('withAlpha — §결정 5 의 곱', () => {
  it('6자리 위에 알파를 접는다', () => {
    expect(withAlpha(BLUE, 0x20 / 255)).toBe('#3b82f620');
    expect(withAlpha(BLUE, 0x30 / 255)).toBe('#3b82f630');
    expect(withAlpha(BLUE, 0x80 / 255)).toBe(BLUE_A50);
  });

  it('기존 알파와 곱한다 — `foldHex` 와 같은 식', () => {
    // 0x80/255 × 0x20/255 ≈ 0.0627 → round(0.0627×255) = 16 = 0x10
    expect(withAlpha(BLUE_A50, 0x20 / 255)).toBe('#3b82f610');
  });

  it('알파가 1 로 접히면 6자리로 되돌린다 — 8자리 `ff` 가 아니다', () => {
    expect(withAlpha(BLUE, 1)).toBe(BLUE);
    expect(withAlpha(BLUE_FF, 1)).toBe(BLUE);
    // 접는 문턱은 **정확히** 255 다. 254 까지 접으면 알파 `fe` 가 소리 없이
    // 불투명해진다 — 돌연변이 검사가 잡아낸 자리다.
    expect(withAlpha('#3b82f6fe', 1)).toBe('#3b82f6fe');
  });

  it('반올림이지 내림이 아니다 — I8 이 걸리는 자리', () => {
    // 0.5 × 255 = 127.5. 반올림이면 128(`80`), 내림이면 127(`7f`) 이다.
    // 이 한 줄이 없으면 `Math.round` 를 `Math.floor` 로 바꿔도 시험이 죽지 않는다.
    expect(withAlpha(BLUE, 0.5)).toBe(BLUE_A50);
  });

  it('약식도 받는다 — `foldHex` 가 3/4/6/8 을 받는 것과 같은 범위', () => {
    expect(withAlpha('#abc', 0x20 / 255)).toBe('#aabbcc20');
    expect(withAlpha('#abcd', 1)).toBe('#aabbccdd');
  });

  it('읽을 수 없는 색이면 `undefined` — 원래 문자열을 돌려주지 않는다', () => {
    expect(withAlpha('var(--color-text-muted)', 0.5)).toBeUndefined();
    expect(withAlpha('', 0.5)).toBeUndefined();
    expect(withAlpha('red', 0.5)).toBeUndefined();
  });

  it('유한하지 않은 알파는 `undefined`', () => {
    expect(withAlpha(BLUE, Number.NaN)).toBeUndefined();
    expect(withAlpha(BLUE, Number.POSITIVE_INFINITY)).toBeUndefined();
  });

  it('알파는 0–1 로 가둔다', () => {
    expect(withAlpha(BLUE, 2)).toBe(BLUE);
    expect(withAlpha(BLUE, -1)).toBe('#3b82f600');
    expect(withAlpha(BLUE, 0)).toBe('#3b82f600');
  });

  it('한 자리 알파는 0 으로 채워 두 자리를 유지한다', () => {
    expect(withAlpha(BLUE, 1 / 255)).toBe('#3b82f601');
  });
});

describe('hexToHsv / hsvToHex', () => {
  it('무채색은 색상 0 · 채도 0', () => {
    expect(hexToHsv('#000000')).toEqual({ h: 0, s: 0, v: 0, a: 1 });
    expect(hexToHsv('#ffffff')).toEqual({ h: 0, s: 0, v: 1, a: 1 });
  });

  it('세 축의 최대가 각각 R·G·B 인 갈래를 모두 지난다', () => {
    expect(hexToHsv('#ff0000')?.h).toBe(0);
    expect(hexToHsv('#00ff00')?.h).toBe(120);
    expect(hexToHsv('#0000ff')?.h).toBe(240);
  });

  it('색상이 음수로 떨어지는 갈래를 360 으로 되돌린다', () => {
    // magenta: max = r, g < b → (g-b)/chroma 가 음수
    expect(hexToHsv('#ff00ff')?.h).toBe(300);
  });

  it('알파를 읽는다', () => {
    expect(hexToHsv(BLUE_A50)?.a).toBeCloseTo(0x80 / 255, 10);
    expect(hexToHsv(BLUE)?.a).toBe(1);
  });

  it('읽을 수 없으면 `null`', () => {
    expect(hexToHsv('nope')).toBeNull();
  });

  it('여섯 색상 구획을 모두 지난다', () => {
    const full = { s: 1, v: 1, a: 1 };
    expect(hsvToHex({ ...full, h: 0 }, { alpha: false })).toBe('#ff0000');
    expect(hsvToHex({ ...full, h: 60 }, { alpha: false })).toBe('#ffff00');
    expect(hsvToHex({ ...full, h: 120 }, { alpha: false })).toBe('#00ff00');
    expect(hsvToHex({ ...full, h: 180 }, { alpha: false })).toBe('#00ffff');
    expect(hsvToHex({ ...full, h: 240 }, { alpha: false })).toBe('#0000ff');
    expect(hsvToHex({ ...full, h: 300 }, { alpha: false })).toBe('#ff00ff');
  });

  it('색상은 360 으로 감싸고 음수도 받는다', () => {
    expect(hsvToHex({ h: 360, s: 1, v: 1, a: 1 }, { alpha: false })).toBe('#ff0000');
    expect(hsvToHex({ h: -60, s: 1, v: 1, a: 1 }, { alpha: false })).toBe('#ff00ff');
  });

  it('채도·명도를 0–1 로 가둔다', () => {
    expect(hsvToHex({ h: 0, s: 2, v: 2, a: 1 }, { alpha: false })).toBe('#ff0000');
    expect(hsvToHex({ h: 0, s: -1, v: -1, a: 1 }, { alpha: false })).toBe('#000000');
  });

  it('`alpha` 가 켜져도 알파 1 이면 6자리다', () => {
    expect(hsvToHex({ h: 0, s: 1, v: 1, a: 1 }, { alpha: true })).toBe('#ff0000');
    expect(hsvToHex({ h: 0, s: 1, v: 1, a: 0.5 }, { alpha: true })).toBe('#ff000080');
    expect(hsvToHex({ h: 0, s: 1, v: 1, a: 2 }, { alpha: true })).toBe('#ff0000');
    expect(hsvToHex({ h: 0, s: 1, v: 1, a: -1 }, { alpha: true })).toBe('#ff000000');
  });

  it('`alpha` 가 꺼지면 알파를 아예 내지 않는다', () => {
    expect(hsvToHex({ h: 0, s: 1, v: 1, a: 0.5 }, { alpha: false })).toBe('#ff0000');
  });

  it('왕복이 항등이다 — 격자 전수', () => {
    const step = 17; // 0,17,…,255 — 16단계 × 3축 = 4096 색
    for (let r = 0; r <= 255; r += step) {
      for (let g = 0; g <= 255; g += step) {
        for (let b = 0; b <= 255; b += step) {
          const hex = `#${[r, g, b].map((c) => c.toString(16).padStart(2, '0')).join('')}`;
          const hsv = hexToHsv(hex);
          expect(hsv).not.toBeNull();
          expect(hsvToHex(hsv!, { alpha: false })).toBe(hex);
        }
      }
    }
  });

  it('알파를 포함한 왕복도 항등이다', () => {
    for (const a of [0x00, 0x01, 0x40, 0x80, 0xfe]) {
      const hex = `${BLUE}${a.toString(16).padStart(2, '0')}`;
      const hsv = hexToHsv(hex);
      expect(hsvToHex(hsv!, { alpha: true })).toBe(hex);
    }
  });
});
