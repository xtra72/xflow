// 이동표가 111자리를 빠짐없이 덮고, 어느 자리도 읽기 어려워지지 않는지 잰다.
//
// 교체 **전에** 돌려서 오늘의 색을 못 박고, 교체 **후에** 다시 돌려서 이동표가
// 여전히 참인지 본다. 교체 후에는 소스에서 `zinc` 클래스가 사라지므로 §완전성
// 시험이 공집합을 훑게 되는데, 그때는 `singleSource` 쪽 가드(AC-01)가 "0건"을
// 따로 단언하므로 둘이 짝을 이룬다.
//
// @spec SPEC-THEME-001 AC-02 · AC-04 · 불변식 I5b (M1)

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { DAY_PRESET, NIGHT_PRESET } from '@/lib/theme/tokens';
import { contrastRatio, oklchToRgb, rgbToHex } from '@/lib/theme/contrast';

import { FLOW_COLOR_SHIFTS, ZINC_OKLCH, type ColorShift } from './flowColorShift';

const FLOW_DIR = dirname(fileURLToPath(import.meta.url));

/** 터미널이라 토큰화 대상이 아니다(spec.md §결정 2 가). */
const EXCLUDED = 'DebugPanel.tsx';

/** 오늘 소스에 적힌 중립색 유틸리티. 변형 접두(`dark:`·`hover:`)는 앞에 붙는다. */
const NEUTRAL = /\b(?:[a-z-]+:)*(text|bg|border|ring|stroke)-(zinc|gray|slate|neutral|stone)-(\d+)/g;

function flowSources(): readonly { file: string; text: string }[] {
  return readdirSync(FLOW_DIR)
    .filter((f) => f.endsWith('.tsx') && !f.includes('.test.') && f !== EXCLUDED)
    .map((f) => ({ file: f, text: readFileSync(join(FLOW_DIR, f), 'utf8') }));
}

/** `zinc-<단>` 의 오늘 실효색. */
function zincHex(shade: number): string {
  const v = ZINC_OKLCH[shade];
  if (v === undefined) throw new Error(`ZINC_OKLCH 에 ${shade} 단이 없다`);
  return rgbToHex(oklchToRgb(v[0], v[1], v[2]));
}

/** 토큰의 프리셋 값. */
function tokenHex(preset: Record<string, string>, token: string): string {
  const v = preset[`--color-${token}`];
  if (v === undefined) throw new Error(`프리셋에 --color-${token} 이 없다`);
  return v;
}

/** 한 행의 교체 전후 색. 다크 변형이 없으면 어두운 쪽도 밝은 단을 쓴다. */
function shiftColors(s: ColorShift): {
  dayBefore: string;
  dayAfter: string;
  nightBefore: string;
  nightAfter: string;
} {
  return {
    dayBefore: zincHex(s.lightShade),
    dayAfter: tokenHex(DAY_PRESET, s.token),
    nightBefore: zincHex(s.darkShade ?? s.lightShade),
    nightAfter: tokenHex(NIGHT_PRESET, s.token),
  };
}

describe('이동표가 오늘의 색을 못 박는다 (AC-02)', () => {
  it('모든 행이 교체 전후 색을 넷 다 낸다 — 빈 칸이 없다', () => {
    for (const s of FLOW_COLOR_SHIFTS) {
      const c = shiftColors(s);
      for (const [k, v] of Object.entries(c)) {
        expect(v, `${s.where} / ${k}`).toMatch(/^#[0-9a-f]{6}$/i);
      }
    }
  });

  it('표가 소스에 실제로 나타나는 중립 단을 빠짐없이 덮는다', () => {
    const covered = new Set<number>();
    for (const s of FLOW_COLOR_SHIFTS) {
      covered.add(s.lightShade);
      if (s.darkShade !== null) covered.add(s.darkShade);
    }

    const found = new Set<number>();
    for (const { text } of flowSources()) {
      for (const m of text.matchAll(NEUTRAL)) found.add(Number(m[3]));
    }

    // 교체가 끝나면 `found` 는 공집합이 되고 이 단언은 자명하게 통과한다. 그 시점의
    // "0건" 은 AC-01 의 가드가 따로 단언하므로 둘이 짝을 이룬다.
    const uncovered = [...found].filter((shade) => !covered.has(shade)).sort((a, b) => a - b);
    expect(uncovered, '이동표에 없는 단이 소스에 있다').toEqual([]);
  });
});

describe('어느 자리도 읽기 어려워지지 않는다 (불변식 I5b · AC-04)', () => {
  /** 글자 행만 대비를 잴 수 있다 — 배경·테두리는 놓이는 자리가 정해져 있지 않다. */
  const textRows = FLOW_COLOR_SHIFTS.filter((s) => s.on !== undefined);

  it('글자 행이 하나 이상 있다', () => {
    expect(textRows.length).toBeGreaterThan(0);
  });

  // 배경은 이미 토큰이다(`bg-(--color-bg-surface)`) — 교체로 바뀌는 것은 글자뿐이라
  // 대비 비교는 **같은 배경** 위에서 해야 한다. 오늘의 배경을 zinc 단으로 근사하면
  // 실제보다 어두운 바탕을 깔게 되어 "교체 전"이 부풀려진다.
  it.each(textRows.map((s) => [s.where, s] as const))('%s — 교체 후 AA 를 넘는다', (_w, s) => {
    const c = shiftColors(s);
    const bgDay = tokenHex(DAY_PRESET, s.on!);
    const bgNight = tokenHex(NIGHT_PRESET, s.on!);

    expect(contrastRatio(c.dayAfter, bgDay)!, `day ${c.dayAfter} on ${bgDay}`).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(c.nightAfter, bgNight)!,
      `night ${c.nightAfter} on ${bgNight}`,
    ).toBeGreaterThanOrEqual(4.5);
  });

  it('이동표가 대비 변화를 수로 남긴다 — 내려간 자리도 적혀 있어야 한다', () => {
    // 이 시험의 산출물은 판정이 아니라 **기록**이다. 11:1 이 10:1 이 되는 것은 회귀가
    // 아니지만, 적히지 않은 채 내려가는 것은 회귀다(AC-02).
    const table = textRows.map((s) => {
      const c = shiftColors(s);
      const dayBg = tokenHex(DAY_PRESET, s.on!);
      const nightBg = tokenHex(NIGHT_PRESET, s.on!);
      const r = (n: number): number => Math.round(n * 100) / 100;
      return {
        where: s.where,
        day: `${c.dayBefore}→${c.dayAfter}`,
        dayContrast: `${r(contrastRatio(c.dayBefore, dayBg)!)}→${r(contrastRatio(c.dayAfter, dayBg)!)}`,
        night: `${c.nightBefore}→${c.nightAfter}`,
        nightContrast: `${r(contrastRatio(c.nightBefore, nightBg)!)}→${r(contrastRatio(c.nightAfter, nightBg)!)}`,
      };
    });
    expect(table).toMatchSnapshot();
  });

  it('노드 부제는 day · night 양쪽에서 AA(4.5:1)를 넘는다 — 이 SPEC 이 고치는 자리다', () => {
    const s = FLOW_COLOR_SHIFTS.find((x) => x.where.startsWith('노드 부제'));
    expect(s, '노드 부제 행이 표에 없다').toBeDefined();

    const c = shiftColors(s!);
    const day = contrastRatio(c.dayAfter, tokenHex(DAY_PRESET, 'bg-surface'))!;
    const night = contrastRatio(c.nightAfter, tokenHex(NIGHT_PRESET, 'bg-surface'))!;
    expect(day, `day ${c.dayAfter} on ${tokenHex(DAY_PRESET, 'bg-surface')}`).toBeGreaterThanOrEqual(4.5);
    expect(night, `night ${c.nightAfter} on ${tokenHex(NIGHT_PRESET, 'bg-surface')}`).toBeGreaterThanOrEqual(4.5);

    // 교체 전에는 넘지 못했다는 것도 함께 못 박는다 — 나아진 것을 수로 볼 수 있어야 한다.
    const before = contrastRatio(c.dayBefore, tokenHex(DAY_PRESET, 'bg-surface'))!;
    expect(before, `교체 전 day ${c.dayBefore}`).toBeLessThan(4.5);
  });
});
