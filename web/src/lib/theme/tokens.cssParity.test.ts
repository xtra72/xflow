// index.css 의 팔레트 블록과 tokens.ts 프리셋이 어긋나지 않았는지 검증한다.
//
// index.css 는 JS 로딩 전 첫 페인트를 위해 팔레트 값을 복제해 두므로 두 곳이
// 조용히 달라질 수 있다(실제로 v0.6.0 에서 bg-primary 가 #f1f5f9 / #f9fafb 로
// 갈라진 적이 있다). 이 테스트가 그 드리프트를 빌드 시점에 잡는다.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, it, expect } from 'vitest';

import { DAY_PRESET, NIGHT_PRESET, ALL_TOKEN_VARS, type ThemeTokens } from './tokens';

const css = readFileSync(join(__dirname, '../../index.css'), 'utf-8');

/** CSS 셀렉터 블록 안의 `--color-*: value;` 선언을 뽑아낸다. */
function extractBlock(selector: string): ThemeTokens {
  const start = css.indexOf(selector);
  if (start === -1) throw new Error(`셀렉터를 찾을 수 없다: ${selector}`);
  const open = css.indexOf('{', start);
  const close = css.indexOf('}', open);
  const body = css.slice(open + 1, close);

  const out: ThemeTokens = {};
  for (const m of body.matchAll(/(--color-[a-z-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;/g)) {
    out[m[1]!] = m[2]!.toLowerCase();
  }
  return out;
}

describe('index.css 팔레트 ↔ tokens.ts 프리셋 동기화', () => {
  it.each([
    [':root,\n[data-theme="day"]', DAY_PRESET, 'Day'],
    ['[data-theme="night"]', NIGHT_PRESET, 'Night'],
  ])('%s 블록이 %#번째 프리셋과 같은 값을 갖는다', (selector, presetTokens) => {
    const cssTokens = extractBlock(selector as string);
    const expected = Object.fromEntries(
      ALL_TOKEN_VARS.map((v) => [v, (presetTokens as ThemeTokens)[v]!.toLowerCase()]),
    );
    expect(cssTokens).toEqual(expected);
  });

  it('두 프리셋 모두 전체 토큰을 빠짐없이 정의한다', () => {
    for (const presetTokens of [DAY_PRESET, NIGHT_PRESET]) {
      expect(Object.keys(presetTokens).sort()).toEqual([...ALL_TOKEN_VARS].sort());
    }
  });

  it('라이트 팔레트의 표면 계층은 서로 다른 밝기를 갖는다', () => {
    // "라이트 모드가 너무 밝다"의 원인은 secondary/surface/elevated 가 모두
    // 순백(#ffffff)이라 계층이 사라진 것이었다. 회귀 방지 가드.
    const layers = [
      DAY_PRESET['--color-bg-sunken']!,
      DAY_PRESET['--color-bg-primary']!,
      DAY_PRESET['--color-bg-secondary']!,
      DAY_PRESET['--color-bg-surface']!,
      DAY_PRESET['--color-bg-elevated']!,
    ];
    expect(new Set(layers).size).toBe(layers.length);

    // sunken -> elevated 순으로 단조 증가해야 한다.
    const luminance = (hex: string) => {
      const n = Number.parseInt(hex.slice(1), 16);
      return ((n >> 16) & 0xff) * 0.299 + ((n >> 8) & 0xff) * 0.587 + (n & 0xff) * 0.114;
    };
    const lums = layers.map(luminance);
    for (let i = 1; i < lums.length; i += 1) {
      expect(lums[i]!).toBeGreaterThan(lums[i - 1]!);
    }
  });
});
