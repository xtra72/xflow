// 팔레트 해석 · 정규화 · 테마 파일 입출력 검증.
//
// 이 로직은 사용자가 컬러 테이블에서 만진 값이 저장되고 다시 살아나는 경로 전체를
// 책임진다. 여기가 틀리면 잘못된 값이 조용히 영속되거나, 남의 JSON 이 그대로
// 팔레트에 주입된다.

import { describe, expect, it } from 'vitest';

import {
  ALL_TOKEN_VARS,
  DAY_PRESET,
  NIGHT_PRESET,
  isValidHexColor,
  normalizeOverrides,
  parseThemeFile,
  resolveTokens,
  serializeThemeFile,
  THEME_FILE_SCHEMA,
  TOKEN_CATEGORIES,
} from './tokens';

describe('resolveTokens', () => {
  it('오버라이드가 없으면 프리셋 기본값을 그대로 돌려준다', () => {
    expect(resolveTokens('day')).toEqual(DAY_PRESET);
    expect(resolveTokens('night')).toEqual(NIGHT_PRESET);
  });

  it('오버라이드된 토큰만 덮고 나머지는 기본값을 유지한다', () => {
    const resolved = resolveTokens('day', { '--color-bg-primary': '#123456' });
    expect(resolved['--color-bg-primary']).toBe('#123456');
    expect(resolved['--color-text-primary']).toBe(DAY_PRESET['--color-text-primary']);
  });

  it('항상 전체 토큰을 채워 돌려준다 — 부분 팔레트가 새어나가지 않는다', () => {
    expect(Object.keys(resolveTokens('night', {})).sort()).toEqual([...ALL_TOKEN_VARS].sort());
  });
});

describe('isValidHexColor', () => {
  it.each(['#fff', '#FFFFFF', '#1a2b3c'])('%s 는 유효하다', (v) => {
    expect(isValidHexColor(v)).toBe(true);
  });

  it.each(['fff', '#12345', 'red', 'rgb(0,0,0)', '#gggggg', ''])('%s 는 거부한다', (v) => {
    expect(isValidHexColor(v)).toBe(false);
  });
});

describe('normalizeOverrides', () => {
  it('알 수 없는 CSS 변수를 버린다', () => {
    expect(normalizeOverrides('day', { '--evil-var': '#000000' })).toEqual({});
  });

  it('헥스가 아닌 값을 버린다', () => {
    expect(normalizeOverrides('day', { '--color-bg-primary': 'red' })).toEqual({});
  });

  it('프리셋 기본값과 같은 값은 오버라이드로 남기지 않는다', () => {
    const same = { '--color-bg-primary': DAY_PRESET['--color-bg-primary']! };
    expect(normalizeOverrides('day', same)).toEqual({});
  });

  it('값을 소문자로 정규화한다 — 대소문자만 다른 중복 저장을 막는다', () => {
    expect(normalizeOverrides('day', { '--color-bg-primary': '#ABCDEF' })).toEqual({
      '--color-bg-primary': '#abcdef',
    });
  });

  it('같은 값이라도 프리셋이 다르면 오버라이드로 남는다', () => {
    const dayDefault = DAY_PRESET['--color-bg-primary']!;
    expect(normalizeOverrides('night', { '--color-bg-primary': dayDefault })).toEqual({
      '--color-bg-primary': dayDefault,
    });
  });
});

describe('테마 파일 내보내기 / 가져오기', () => {
  it('내보낸 파일을 그대로 다시 읽어 같은 팔레트를 얻는다', () => {
    const tokens = resolveTokens('night', { '--color-bg-primary': '#101010' });
    const parsed = parseThemeFile(serializeThemeFile('night', tokens));

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.preset).toBe('night');
    expect(parsed.tokens).toEqual(tokens);
    expect(parsed.ignored).toBe(0);
  });

  it('내보낸 파일은 스키마 식별자와 전체 토큰을 담는다', () => {
    const file = JSON.parse(serializeThemeFile('day', DAY_PRESET));
    expect(file.schema).toBe(THEME_FILE_SCHEMA);
    expect(Object.keys(file.tokens).sort()).toEqual([...ALL_TOKEN_VARS].sort());
  });

  it('JSON 이 아니면 invalid-json 으로 거부한다', () => {
    expect(parseThemeFile('not json at all')).toEqual({ ok: false, error: 'invalid-json' });
  });

  it('XFlow 테마 파일이 아니면 invalid-schema 로 거부한다', () => {
    expect(parseThemeFile('{"tokens":{}}')).toEqual({ ok: false, error: 'invalid-schema' });
    expect(parseThemeFile('[]')).toEqual({ ok: false, error: 'invalid-schema' });
    expect(parseThemeFile('null')).toEqual({ ok: false, error: 'invalid-schema' });
  });

  it('쓸 수 있는 색이 하나도 없으면 no-valid-tokens 로 거부한다', () => {
    const junk = JSON.stringify({
      schema: THEME_FILE_SCHEMA,
      version: 1,
      preset: 'day',
      tokens: { '--color-bg-primary': 'not-a-color', '--unknown': '#ffffff' },
    });
    expect(parseThemeFile(junk)).toEqual({ ok: false, error: 'no-valid-tokens' });
  });

  it('일부만 잘못된 파일은 살릴 수 있는 색만 취하고 버린 개수를 알린다', () => {
    const mixed = JSON.stringify({
      schema: THEME_FILE_SCHEMA,
      version: 1,
      preset: 'day',
      tokens: {
        '--color-bg-primary': '#111111',
        '--color-text-primary': 'oops',
        '--injected-var': '#222222',
      },
    });
    const parsed = parseThemeFile(mixed);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.tokens).toEqual({ '--color-bg-primary': '#111111' });
    expect(parsed.ignored).toBe(2);
  });

  it('preset 이 없거나 알 수 없으면 day 로 폴백한다', () => {
    const noPreset = JSON.stringify({
      schema: THEME_FILE_SCHEMA,
      version: 1,
      tokens: { '--color-bg-primary': '#111111' },
    });
    const parsed = parseThemeFile(noPreset);
    expect(parsed.ok && parsed.preset).toBe('day');
  });
});

// --- 플로우 편집기 토큰 (SPEC-THEME-001 AC-03) ---

describe('flow 카테고리', () => {
  const flow = TOKEN_CATEGORIES.find((c) => c.category === 'flow');

  it('카테고리가 있고 세 토큰을 담는다', () => {
    expect(flow, 'flow 카테고리가 없다').toBeDefined();
    expect(flow!.tokens.map((t) => t.cssVar)).toEqual([
      '--color-flow-dot',
      '--color-flow-edge',
      '--color-flow-area',
    ]);
  });

  it('라벨과 힌트가 비어 있지 않다 — 표에서 무엇인지 읽혀야 한다', () => {
    expect(flow!.label).not.toBe('');
    for (const t of flow!.tokens) {
      expect(t.label, t.cssVar).not.toBe('');
      expect(t.hint, t.cssVar).not.toBe('');
    }
  });

  it('토큰이 21 → 29 로 늘었고, 기존 21개의 이름이 그대로다 (불변식 I2)', () => {
    // 21(원래) + 3(flow) + 5(글자용 상태색) = 29.
    expect(ALL_TOKEN_VARS).toHaveLength(29);

    // 기존 21개가 이름 그대로 살아 있다 — 새 토큰이 기존 칸을 밀어내지 않았다.
    const legacyNames = [
      'bg-primary', 'bg-secondary', 'bg-surface', 'bg-elevated', 'bg-sunken',
      'text-primary', 'text-secondary', 'text-muted', 'text-inverse',
      'border-default', 'border-subtle', 'border-strong',
      'status-running', 'status-stopped', 'status-error', 'status-warning', 'status-info',
      'interactive-primary', 'interactive-hover', 'interactive-active', 'interactive-muted',
    ];
    const present = new Set(ALL_TOKEN_VARS);
    for (const n of legacyNames) expect(present.has(`--color-${n}`), n).toBe(true);
    expect(legacyNames).toHaveLength(21);
  });

  it('글자용 상태색 다섯이 칠용 다섯과 짝을 이룬다', () => {
    // 짝이 어긋나면 "이 상태에는 글자색이 없다" 는 자리가 생기고, 그 자리만 박힌
    // 색으로 되돌아간다.
    for (const n of ['running', 'stopped', 'error', 'warning', 'info']) {
      expect(DAY_PRESET[`--color-status-${n}`], n).toMatch(/^#[0-9a-f]{6}$/);
      expect(DAY_PRESET[`--color-status-${n}-text`], `${n}-text`).toMatch(/^#[0-9a-f]{6}$/);
      expect(NIGHT_PRESET[`--color-status-${n}-text`], `${n}-text night`).toMatch(/^#[0-9a-f]{6}$/);
    }
  });

  it('day · night 프리셋 양쪽에 세 값이 다 있다', () => {
    for (const t of flow!.tokens) {
      expect(DAY_PRESET[t.cssVar], `day ${t.cssVar}`).toMatch(/^#[0-9a-f]{6}$/);
      expect(NIGHT_PRESET[t.cssVar], `night ${t.cssVar}`).toMatch(/^#[0-9a-f]{6}$/);
    }
  });

  it('점격자는 야간에 밝은 채로 남지 않는다 — 신고된 증상이 그것이다', () => {
    // day 와 night 이 같은 값이면 어두운 바탕에 밝은 점이 그대로 남는다.
    expect(NIGHT_PRESET['--color-flow-dot']).not.toBe(DAY_PRESET['--color-flow-dot']);
  });
});
