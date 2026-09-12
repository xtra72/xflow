// 004 M6 이 더한 문구의 형상 가드 — **양쪽 로케일** (SPEC-CANVAS-004 REQ-01 · 시험 규율 E-K).
//
// ## 겨누는 구멍 셋
//
// **하나 — 기본 로케일이 `ko` 다.** `I18nProvider` 를 그냥 세우면 언제나 한국어이므로
// (`lib/i18n` §DEFAULT_LOCALE), `en.json` 에서 키가 통째로 빠져도 화면 시험 전량이 초록이다.
// 그래서 여기서는 **로케일을 갈아 끼우며** 같은 단언을 돌린다.
//
// **둘 — `t()` 는 치환을 하지 않는다.** 치환은 호출부의 몫이고, 그래서 한쪽 로케일에서
// 치환자가 빠지거나 늘어나도 조용하다. 008 M11 이 세운 **치환자 다중 집합 비교**를 그대로
// 물려받는다 — 그리고 008 이 물린 그 자리(`.replace` 는 겹친 치환자의 첫 자리만 바꾼다)를
// 위해 각 문구의 치환자 **횟수**를 가드로 고정한다.
//
// **셋 — 낱말 충돌**(위험 R17). i18n 에는 이미 `paletteGroupPrimitive`/`General`/`Basic`/
// `Arrow` 가 있고 거기서 "그룹" 은 **팔레트 묶음**을 뜻한다. 004 의 `group` 은 **노드**다.
// 두 뜻이 한 화면에 뜨면 사용자가 "그룹" 을 두 가지로 읽으므로, 004 의 키는 **동작으로**
// 짓고 `paletteGroup*` 과 앞자리를 나누지 않는다. 그 사실을 가드로 둔다.
//
// @spec SPEC-CANVAS-004 REQ-01 · REQ-07

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import { DEFAULT_CANVAS_SIZE, type CanvasElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import { PALETTE_GROUP_IDS, PALETTE_GROUP_TITLE_KEYS } from './shapes/paletteGroups';

const EDIT = 'dashboard.canvas.edit';

/**
 * M6 이 더한 키 전량. **리터럴 목록인 것에 뜻이 있다** — 코드에서 유도하면 문구를 지우며
 * 호출부를 함께 지운 변경이 가드를 **줄이면서** 초록으로 남는다(008 이 같은 이유로 같은
 * 형태를 골랐다).
 */
const ADDED_KEYS: readonly string[] = [
  'dockGroup',
  'groupCreate',
  'groupUngroup',
  'groupUngroupAsk',
  'groupUngroupYes',
  'groupUngroupNo',
  'groupRefusalNested',
  'groupRefusalTooFew',
].map((k) => `${EDIT}.${k}`);

/**
 * 치환자를 든 문구와 그 횟수.
 *
 * **횟수를 적어 두는 것이 `replace`/`replaceAll` 가드의 절반이다.** 번역을 다듬다 치환자
 * 개수가 달라지면 그 자리에서 울린다 — 달라진 뒤에는 호출부를 `.replace` 로 되돌려도 아무
 * 시험이 울지 않기 때문이다(008 REPEATED_TOKENS 와 같은 규율).
 */
const TOKEN_COUNTS: ReadonlyArray<readonly [key: string, token: string, times: number]> = [
  [`${EDIT}.groupUngroupAsk`, '{rows}', 1],
];

const LOCALES = { ko, en } as const;
type LocaleName = keyof typeof LOCALES;

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

describe('M6 이 더한 문구는 ko · en 양쪽에 있다 (REQ-01 · 품질 게이트 Unified)', () => {
  it('키 전량이 양쪽에서 **비어 있지 않은 문자열**로 잡힌다', () => {
    // 목록이 비면 아래 순회가 0회 돌고 초록이 된다 — 켜져 있음을 먼저 단언한다.
    expect(ADDED_KEYS).toHaveLength(8);
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
    // 팔레트 쪽 "그룹" 은 **묶음**이고 004 의 "그룹" 은 **노드**다. 키가 앞자리를 나누면
    // 목록을 훑는 사람이 둘을 한 무리로 읽고, 다음 사람이 한쪽 문구를 다른 쪽 뜻으로 고친다.
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

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 160 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

const RECT: CanvasElement = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 73, y: 41, w: 120, h: 60 },
  style: {},
};

/** 규칙 표 **두 행**을 든 그룹 — 안내가 그 수를 말해야 한다. */
const GROUP: GroupElement = {
  id: 'grp-1',
  kind: 'group',
  geometry: { x: 73, y: 41, w: 317, h: 181 },
  parts: [{ id: 'p-a', kind: 'rect', geometry: { x: 0, y: 0, w: 3785, h: 3315 }, style: {} }],
  binding: { series: 's1', agg: 'last' },
  rules: [
    { op: 'nodata', value: 0, patch: { fill: '#111111' } },
    { op: 'gt', value: 10, patch: { fill: '#222222' } },
  ],
};

function Harness({
  docked,
  initial,
  select,
}: {
  docked: boolean;
  initial: readonly CanvasNode[];
  select: readonly string[];
}) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  const overlay = (
    <CanvasEditOverlay
      enabled
      elements={elements}
      projection={PROJ}
      textWidths={{}}
      onElementsChange={setElements}
    />
  );
  return (
    <I18nProvider>
      <CanvasEditSelectionContext value={state}>
        <button type="button" data-testid="pick" onClick={() => state.setSelection(new Set(select))}>
          pick
        </button>
        {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
      </CanvasEditSelectionContext>
    </I18nProvider>
  );
}

/**
 * 로케일을 고른다. `I18nProvider` 는 마운트 때 `xflow-locale` 을 읽는다.
 *
 * 이름이 `use` 로 시작하지 않는 것에 뜻이 있다 — 훅이 아닌데 훅처럼 이름 지으면
 * `react-hooks/rules-of-hooks` 가 순회 안의 호출을 훅 호출로 읽고 빨개진다.
 */
function pickLocale(name: LocaleName): void {
  globalThis.localStorage.setItem('xflow-locale', name);
}

beforeEach(() => {
  globalThis.localStorage.clear();
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

const LOCALE_NAMES: readonly LocaleName[] = ['ko', 'en'];
const SURFACES = [true, false] as const;

describe('두 로케일 × 두 표면에서 문구가 진짜 번역으로 선다 (E-J × E-K)', () => {
  it('단추 이름에 원문 키도 벌거벗은 치환자도 남지 않는다', () => {
    for (const locale of LOCALE_NAMES) {
      for (const docked of SURFACES) {
        cleanup();
        globalThis.localStorage.clear();
        pickLocale(locale);
        render(<Harness docked={docked} initial={[RECT, GROUP]} select={['grp-1']} />);
        fireEvent.click(screen.getByTestId('pick'));

        const where = `${locale}/${docked ? 'dock' : 'bar'}`;
        for (const id of ['canvas-group-create', 'canvas-group-ungroup']) {
          const label = screen.getByTestId(id).getAttribute('aria-label') ?? '';
          expect(label, `${where}: ${id}`).toBe(lookup(LOCALES[locale], `${EDIT}.${id === 'canvas-group-create' ? 'groupCreate' : 'groupUngroup'}`));
          expect(label, `${where}: ${id}`).not.toContain('dashboard.canvas');
          expect(label, `${where}: ${id}`).not.toMatch(/\{[a-zA-Z]+\}/);
        }
      }
    }
  });

  it('풀기 안내가 **버려지는 행의 수**를 채워 말한다 — `{rows}` 가 남지 않는다', () => {
    for (const locale of LOCALE_NAMES) {
      for (const docked of SURFACES) {
        cleanup();
        globalThis.localStorage.clear();
        pickLocale(locale);
        render(<Harness docked={docked} initial={[RECT, GROUP]} select={['grp-1']} />);
        fireEvent.click(screen.getByTestId('pick'));
        fireEvent.click(screen.getByTestId('canvas-group-ungroup'));

        const where = `${locale}/${docked ? 'dock' : 'bar'}`;
        const text = screen.getByTestId('canvas-group-ungroup-ask').textContent ?? '';
        const template = lookup(LOCALES[locale], `${EDIT}.groupUngroupAsk`) ?? '';
        expect(template, where).not.toBe('');
        // 치환자가 하나도 남지 않았고, **그 자리에 `2` 가 들어갔다**(그룹의 규칙 행 수).
        expect(text, where).not.toMatch(/\{[a-zA-Z]+\}/);
        expect(text, where).toBe(template.replaceAll('{rows}', '2'));
        expect(text, where).toContain('2');
      }
    }
  });

  it('거절 안내도 두 로케일에서 진짜 문구다', () => {
    for (const locale of LOCALE_NAMES) {
      cleanup();
      globalThis.localStorage.clear();
      pickLocale(locale);
      render(<Harness docked initial={[RECT, GROUP]} select={['grp-1', 'a']} />);
      fireEvent.click(screen.getByTestId('pick'));
      fireEvent.click(screen.getByTestId('canvas-group-create'));

      const text = screen.getByTestId('canvas-group-refusal').textContent ?? '';
      expect(text, locale).toBe(lookup(LOCALES[locale], `${EDIT}.groupRefusalNested`));
      expect(text, locale).not.toContain('dashboard.canvas');
    }
  });

  it('도크의 그룹 묶음 제목이 두 로케일에서 제 문구다', () => {
    for (const locale of LOCALE_NAMES) {
      cleanup();
      globalThis.localStorage.clear();
      pickLocale(locale);
      render(<Harness docked initial={[RECT, GROUP]} select={[]} />);

      const title = lookup(LOCALES[locale], `${EDIT}.dockGroup`) ?? '';
      expect(title, locale).not.toBe('');
      const section = screen.getByTestId('canvas-group-create').closest('section');
      expect(section, locale).not.toBeNull();
      expect(section!.textContent, locale).toContain(title);
    }
  });
});
