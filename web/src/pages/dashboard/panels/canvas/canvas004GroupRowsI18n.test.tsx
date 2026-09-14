// 그룹 행이 더한 문구의 형상 가드 — **양쪽 로케일** (SPEC-CANVAS-004 REQ-01 · 시험 규율 E-K).
//
// 자매 파일 `canvas004I18n.test.tsx` 가 M6 의 **도크·줄** 문구를 지키고, 이 파일은 같은
// 규율로 **목록 행**의 문구를 지킨다. 겨누는 구멍은 그 파일과 같은 셋이다.
//
// **하나 — 기본 로케일이 `ko` 다.** `I18nProvider` 를 그냥 세우면 언제나 한국어이므로
// `en.json` 에서 키가 통째로 빠져도 화면 시험이 초록이다. 그래서 **로케일을 갈아 끼우며**
// 같은 단언을 돌린다.
//
// **둘 — `t()` 는 치환을 하지 않는다.** 치환자 **다중 집합**과 **횟수**를 양쪽에서 비교한다.
// 횟수를 재는 것이 `replace`/`replaceAll` 가드의 절반이다 — 한 문구에 같은 치환자가 둘
// 들어간 순간 `String.replace` 는 뒤쪽을 `{count}` 인 채로 화면에 남긴다.
//
// **셋 — 낱말 충돌**(위험 R17). i18n 의 `paletteGroup*` 에서 "그룹" 은 **팔레트 묶음**이고
// 004 의 그룹은 **노드**다. 목록 행의 키도 `paletteGroup*` 과 앞자리를 나누지 않는다.
//
// @spec SPEC-CANVAS-004 REQ-01 · REQ-08

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasElementsEditor from './CanvasElementsEditor';
import { PALETTE_GROUP_IDS, PALETTE_GROUP_TITLE_KEYS } from './shapes/paletteGroups';

const ELEMENTS = 'dashboard.canvas.elements';

/**
 * 목록 행이 더한 키 전량. **리터럴 목록인 것에 뜻이 있다** — 코드에서 유도하면 문구를
 * 지우며 호출부를 함께 지운 변경이 가드를 **줄이면서** 초록으로 남는다.
 */
const ADDED_KEYS: readonly string[] = [
  'groupLabel',
  'groupSummary',
  'groupDetailsAria',
  'groupMoveUpAria',
  'groupMoveDownAria',
  'groupDeleteAria',
  'groupPartAria',
  'groupPartsHint',
  'groupPartsEmpty',
].map((k) => `${ELEMENTS}.${k}`);

/** 치환자를 든 문구와 그 횟수. 번역을 다듬다 개수가 달라지면 그 자리에서 울린다. */
const TOKEN_COUNTS: ReadonlyArray<readonly [key: string, token: string, times: number]> = [
  [`${ELEMENTS}.groupSummary`, '{count}', 1],
  [`${ELEMENTS}.groupDetailsAria`, '{index}', 1],
  [`${ELEMENTS}.groupMoveUpAria`, '{index}', 1],
  [`${ELEMENTS}.groupMoveDownAria`, '{index}', 1],
  [`${ELEMENTS}.groupDeleteAria`, '{index}', 1],
  [`${ELEMENTS}.groupDeleteAria`, '{count}', 1],
  [`${ELEMENTS}.groupPartAria`, '{index}', 1],
  [`${ELEMENTS}.groupPartAria`, '{part}', 1],
];

const LOCALES = { ko, en } as const;
type LocaleName = keyof typeof LOCALES;
const LOCALE_NAMES: readonly LocaleName[] = ['ko', 'en'];

/** 점 표기 키를 트리에서 조회한다. `t()` 와 **같은 규칙**이다(문자열이 아니면 부재). */
function lookup(tree: unknown, key: string): string | undefined {
  const value = key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node !== null && typeof node === 'object'
          ? (node as Record<string, unknown>)[seg]
          : undefined,
      tree,
    );
  return typeof value === 'string' ? value : undefined;
}

/** 문구 안의 `{…}` 치환자 다중 집합. 정렬해 두어 비교가 순서에 흔들리지 않는다. */
function tokensOf(text: string): string[] {
  return [...text.matchAll(/\{[a-zA-Z0-9_]+\}/g)].map((m) => m[0]).sort();
}

// --- DOM 없는 형상 가드 ---------------------------------------------------

describe('목록 그룹 행의 문구는 ko · en 양쪽에 있다 (REQ-01)', () => {
  it('키 전량이 양쪽에서 **비어 있지 않은 문자열**로 잡힌다', () => {
    // 목록이 비면 아래 순회가 0회 돌고 초록이 된다 — 켜져 있음을 먼저 단언한다.
    expect(ADDED_KEYS).toHaveLength(9);
    const missing: string[] = [];
    for (const key of ADDED_KEYS) {
      for (const [name, tree] of Object.entries(LOCALES)) {
        const text = lookup(tree, key);
        if (text === undefined || text === '') missing.push(`${name}: ${key}`);
      }
    }
    expect(missing).toEqual([]);
  });

  it('두 로케일이 실제로 **번역**이다 — 한쪽을 복사해 두면 번역이 없는 것과 같다', () => {
    for (const key of ADDED_KEYS) {
      expect(lookup(ko, key), key).not.toBe(lookup(en, key));
    }
  });

  it('두 로케일의 **치환자 다중 집합이 같다** — 한쪽에서 하나가 빠지면 빨개진다', () => {
    const mismatched: string[] = [];
    for (const key of ADDED_KEYS) {
      const a = tokensOf(lookup(ko, key) ?? '');
      const b = tokensOf(lookup(en, key) ?? '');
      if (JSON.stringify(a) !== JSON.stringify(b)) {
        mismatched.push(`${key}: ko=${a.join(',')} en=${b.join(',')}`);
      }
    }
    expect(mismatched).toEqual([]);
  });

  it('`TOKEN_COUNTS` 가 **모든 치환자를 빠짐없이** 덮는다 — 목록이 뒤처지면 가드가 준다', () => {
    // 위 표는 **손으로 적은 목록**이라 새 키나 새 치환자가 조용히 빠질 수 있고, 빠진
    // 치환자는 횟수 가드를 받지 않는다. 그 순간 호출부의 `replaceAll` 을 `replace` 로
    // 되돌려도 아무 시험이 울지 않게 된다 — 이 시험이 그 구멍을 막는다.
    const listed = new Set(TOKEN_COUNTS.map(([key, token]) => `${key} ${token}`));
    const uncovered: string[] = [];
    for (const key of ADDED_KEYS) {
      for (const [name, tree] of Object.entries(LOCALES)) {
        for (const token of new Set(tokensOf(lookup(tree, key) ?? ''))) {
          if (!listed.has(`${key} ${token}`)) uncovered.push(`${name}: ${key} ${token}`);
        }
      }
    }
    expect(uncovered).toEqual([]);
  });

  it('치환자를 든 문구가 양쪽에서 **같은 횟수**로 말한다', () => {
    for (const [key, token, times] of TOKEN_COUNTS) {
      for (const [name, tree] of Object.entries(LOCALES) as [LocaleName, unknown][]) {
        const text = lookup(tree, key);
        expect(text, `${name}: ${key}`).toBeTypeOf('string');
        expect(tokensOf(text ?? '').filter((t) => t === token), `${name}: ${key}`).toHaveLength(
          times,
        );
      }
    }
  });

  it('키 이름 안에 점이 없다 — 어떤 조회 경로로도 닿지 않는 키를 만들지 않는다', () => {
    for (const key of ADDED_KEYS) {
      expect(key.split('.').filter((s) => s === ''), key).toEqual([]);
    }
  });

  it('`paletteGroup*` 과 **앞자리를 나누지 않는다** (위험 R17 — "그룹" 의 두 뜻)', () => {
    const palette = PALETTE_GROUP_IDS.map((id) => PALETTE_GROUP_TITLE_KEYS[id]!);
    expect(palette.length).toBeGreaterThan(0);
    const leading = (key: string): string => {
      const last = key.split('.').at(-1) ?? '';
      return /^[a-z]+/.exec(last)?.[0] ?? '';
    };
    const paletteLeading = new Set(palette.map(leading));
    expect(paletteLeading).toEqual(new Set(['palette']));
    for (const key of ADDED_KEYS) {
      expect(paletteLeading.has(leading(key)), key).toBe(false);
      expect(palette).not.toContain(key);
    }
  });
});

// --- 진짜 문구로 렌더 (E-K) ----------------------------------------------

const CONFIG: Record<string, unknown> = {
  channel_name: '',
  data_source: 'store',
  canvas: { width: 500, height: 400 },
  elements: [
    { id: 'r1', kind: 'rect', geometry: { x: 50, y: 80, w: 150, h: 160 }, style: {} },
    {
      id: 'grp-1',
      kind: 'group',
      geometry: { x: 73, y: 41, w: 317, h: 181 },
      parts: [
        { id: 'body', kind: 'rect', geometry: { x: 0, y: 0, w: 3000, h: 2000 }, style: {} },
        { id: 'stem', kind: 'ellipse', geometry: { x: 4100, y: 3300, w: 1700, h: 900 }, style: {} },
        { id: 'label', kind: 'text', geometry: { x: 5000, y: 9200 }, style: {}, text: '온도' },
      ],
    },
  ],
};

/**
 * 로케일을 고른다. `I18nProvider` 는 마운트 때 `xflow-locale` 을 읽는다.
 *
 * 이름이 `use` 로 시작하지 않는 것에 뜻이 있다 — 훅이 아닌데 훅처럼 이름 지으면
 * `react-hooks/rules-of-hooks` 가 순회 안의 호출을 훅 호출로 읽고 빨개진다.
 */
function pickLocale(name: LocaleName): void {
  globalThis.localStorage.setItem('xflow-locale', name);
}

function renderAt(locale: LocaleName): void {
  cleanup();
  globalThis.localStorage.clear();
  pickLocale(locale);
  render(
    <I18nProvider>
      <CanvasElementsEditor config={CONFIG} onConfigChange={() => {}} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  globalThis.localStorage.clear();
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

describe('두 로케일에서 그룹 행의 문구가 진짜 번역으로 선다 (E-K)', () => {
  it('머리줄이 **부품 수를 채워** 말한다 — `{count}` 가 벌거벗은 채 남지 않는다', () => {
    for (const locale of LOCALE_NAMES) {
      renderAt(locale);
      const text = screen.getByTestId('canvas-group-row-toggle-1').textContent ?? '';
      const label = lookup(LOCALES[locale], `${ELEMENTS}.groupLabel`) ?? '';
      const summary = lookup(LOCALES[locale], `${ELEMENTS}.groupSummary`) ?? '';
      expect(label, locale).not.toBe('');
      expect(text, locale).toContain(label);
      // 부품이 셋이므로 `{count}` 자리에 `3` 이 들어간다 — 채워진 결과를 통째로 비교한다.
      expect(text, locale).toContain(summary.replaceAll('{count}', '3'));
      expect(text, locale).not.toMatch(/\{[a-zA-Z]+\}/);
      expect(text, locale).not.toContain('dashboard.canvas');
    }
  });

  it('삭제 단추의 이름이 **순번과 부품 수 둘 다** 채워 말한다', () => {
    for (const locale of LOCALE_NAMES) {
      renderAt(locale);
      const label = screen.getByTestId('canvas-group-row-delete-1').getAttribute('aria-label') ?? '';
      const template = lookup(LOCALES[locale], `${ELEMENTS}.groupDeleteAria`) ?? '';
      expect(template, locale).not.toBe('');
      expect(label, locale).toBe(template.replaceAll('{index}', '2').replaceAll('{count}', '3'));
      expect(label, locale).not.toMatch(/\{[a-zA-Z]+\}/);
    }
  });

  it('부품 행의 이름이 **그 부품의 id** 를 채워 말한다', () => {
    for (const locale of LOCALE_NAMES) {
      renderAt(locale);
      fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
      // SPEC-CANVAS-009 M5 — 부품 행이 펼칠 수 있는 카드가 되면서 이름은 바깥 상자가
      // 아니라 **머리줄 단추**가 든다(펼침 상태를 `aria-expanded` 로 알리는 그 요소다).
      const label =
        screen.getByTestId('canvas-group-row-part-toggle-1-2').getAttribute('aria-label') ?? '';
      const template = lookup(LOCALES[locale], `${ELEMENTS}.groupPartAria`) ?? '';
      expect(label, locale).toBe(template.replaceAll('{index}', '2').replaceAll('{part}', 'label'));
      expect(label, locale).not.toMatch(/\{[a-zA-Z]+\}/);
    }
  });

  it('부품 안내가 두 로케일에서 제 문구이고 원문 키가 새지 않는다', () => {
    for (const locale of LOCALE_NAMES) {
      renderAt(locale);
      fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
      const text = screen.getByTestId('canvas-group-row-parts-hint-1').textContent ?? '';
      expect(text, locale).toBe(lookup(LOCALES[locale], `${ELEMENTS}.groupPartsHint`));
      expect(text, locale).not.toContain('dashboard.canvas');
    }
  });

  it('부품 행이 **종류 이름**을 두 로케일에서 제 문구로 말한다 (문구 부품)', () => {
    for (const locale of LOCALE_NAMES) {
      renderAt(locale);
      fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
      const text = screen.getByTestId('canvas-group-row-part-1-2').textContent ?? '';
      expect(text, locale).toContain(lookup(LOCALES[locale], `${ELEMENTS}.kindText`));
      expect(text, locale).not.toContain('dashboard.canvas');
    }
  });
});
