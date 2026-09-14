// 글자용 상태색이 실제로 읽히는지 수로 잰다.
//
// `status-*` 다섯은 **점·막대를 칠하라고** 고른 값이다. 그 값을 글자에 쓰면 밝은
// 테마에서 AA(4.5:1)를 넘지 못한다 — 실측으로 `status-running` 이 3.21,
// `status-warning` 이 2.86 이었다. 점의 대비 기준과 글자의 대비 기준은 다르므로,
// 한 값으로 둘을 다 하려 하면 어느 한쪽이 진다. 그래서 `status-*-text` 다섯이 있다.
//
// 이 시험이 없으면 누군가 팔레트를 손질하다 글자용 값을 한 단 밝게 바꿔도 아무도
// 말하지 않는다. 화면은 멀쩡히 뜨고, 읽기만 어려워진다.
//
// @spec SPEC-THEME-001 REQ-03 (목록·스케줄 토큰화)

import { describe, expect, it } from 'vitest';

import { contrastRatio, hexToRgb, rgbToHex } from './contrast';
import { DAY_PRESET, NIGHT_PRESET, type ThemeTokens } from './tokens';

/** WCAG AA 본문 기준. */
const AA = 4.5;

/** 상태 배지가 칠에 쓰는 농도 — 구현부(`statusBadge`)와 같은 값이어야 한다. */
const TINT = { day: 0.15, night: 0.2 } as const;

const STATES = ['running', 'stopped', 'error', 'warning', 'info'] as const;

/** `fg` 를 `a` 비율로 `bg` 위에 얹은 결과. `color-mix` 가 화면에서 하는 일과 같다. */
function mix(fg: string, bg: string, a: number): string {
  const f = hexToRgb(fg)!;
  const b = hexToRgb(bg)!;
  return rgbToHex({
    r: f.r * a + b.r * (1 - a),
    g: f.g * a + b.g * (1 - a),
    b: f.b * a + b.b * (1 - a),
  });
}

function check(preset: ThemeTokens, tint: number, state: string): { onTint: number; onSurface: number } {
  const surface = preset['--color-bg-surface']!;
  const fill = preset[`--color-status-${state}`]!;
  const text = preset[`--color-status-${state}-text`]!;
  return {
    onTint: contrastRatio(text, mix(fill, surface, tint))!,
    onSurface: contrastRatio(text, surface)!,
  };
}

describe('글자용 상태색은 칠 위에서도 면 위에서도 읽힌다', () => {
  it.each(STATES)('%s — day 가 AA 를 넘는다', (state) => {
    const { onTint, onSurface } = check(DAY_PRESET, TINT.day, state);
    expect(onTint, `${state} 칠 위`).toBeGreaterThanOrEqual(AA);
    expect(onSurface, `${state} 면 위`).toBeGreaterThanOrEqual(AA);
  });

  it.each(STATES)('%s — night 가 AA 를 넘는다', (state) => {
    const { onTint, onSurface } = check(NIGHT_PRESET, TINT.night, state);
    expect(onTint, `${state} 칠 위`).toBeGreaterThanOrEqual(AA);
    expect(onSurface, `${state} 면 위`).toBeGreaterThanOrEqual(AA);
  });

  it('칠용 값을 글자로 쓰면 AA 를 넘지 못한다 — 토큰을 나눈 이유', () => {
    // 이 단언이 뒤집히는 날(칠용 값이 충분히 진해지는 날) 글자용 토큰은 없어도
    // 된다. 그때까지는 둘이 따로 있어야 한다.
    const surface = DAY_PRESET['--color-bg-surface']!;
    const running = contrastRatio(DAY_PRESET['--color-status-running']!, surface)!;
    const warning = contrastRatio(DAY_PRESET['--color-status-warning']!, surface)!;
    expect(running).toBeLessThan(AA);
    expect(warning).toBeLessThan(AA);
  });
});
