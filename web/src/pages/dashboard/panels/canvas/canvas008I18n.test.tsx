// 008 이 더한 문구 전량의 형상 가드 — 양쪽 로케일 (SPEC-CANVAS-008 M11 · REQ-06 · 시험 규율 D7).
//
// 이 파일이 겨누는 구멍 셋. 셋 다 **전량 초록인 채로** 화면에 원문 키나 벌거벗은 치환자를
// 배달할 수 있는 자리다.
//
// **하나 — 동적 키는 `i18nKeyShape.test.ts` 의 눈에 보이지 않는다.** 그 가드의 정규식은
// `t('리터럴')` 만 잡는다(`T_CALL`). 카탈로그 30종의 이름은 `t(shape.nameKey)` 로 불리므로
// 그 가드를 **지나가지 않는다** — `en.json` 에서 `cloud` 한 줄이 빠지면 ko/en 키 집합 비교가
// 그것을 잡지만, 카탈로그가 자라는 방향(도형을 더하며 ko 에만 적는 일)은 잡지 못한다. 여기서는
// **`SHAPE_CATALOG` 를 순회해** 30종 전량이 양쪽에 있음을 잰다. 목록에서 유도하므로 31번째
// 도형이 들어오면 가드가 저절로 넓어진다.
//
// **둘 — 지금까지의 진짜 번역 시험은 전부 `ko` 한 쪽이었다.** 기본 로케일이 `ko` 이므로
// (`lib/i18n/index.ts` §DEFAULT_LOCALE) `I18nProvider` 를 그냥 세우면 언제나 한국어다. `en`
// 쪽 문구에서 치환자가 하나 빠져 있어도 오늘의 시험 전량이 초록이다. 여기서는 **치환자 다중
// 집합을 양쪽에서 비교**하고, `en` 으로 실제 렌더해 두 자리가 모두 바뀌는지 잰다.
//
// **셋 — `replace` 대 `replaceAll`**(시험 규율 D7 · 위험 R13). 008 의 문구 셋이 치환자를
// **두 번** 말한다. 그 성질은 시험이 지키지 않으면 조용히 사라진다 — 번역을 다듬다 둘째
// 자리를 지우면 `replace` 로 되돌려도 아무 시험이 울지 않는다. 그래서 "두 번 말한다" 자체를
// 고정 입력이 아니라 **가드**로 둔다.
//
// @spec SPEC-CANVAS-008 REQ-02 · REQ-06 · AC-08 · AC-E10 · D7

import { useState } from 'react';

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import CanvasElementsEditor from './CanvasElementsEditor';
import { DEFAULT_CANVAS_SIZE, type CanvasElement, type PathElement } from './canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import { PALETTE_GROUP_TITLE_KEYS, PALETTE_GROUP_IDS } from './shapes/paletteGroups';
import { SHAPE_CATALOG } from './shapes/shapeCatalog';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import { useScratchpadStore } from './scratchpad/scratchpadStore';
import { SCRATCHPAD_MAX_ENTRIES, type ScratchpadEntry } from './scratchpad/scratchpadTypes';
import type { CanvasNode } from './group/groupTypes';

// --- 키 목록 -------------------------------------------------------------

const EDIT = 'dashboard.canvas.edit';

/**
 * 008 이 `dashboard.canvas.edit` 아래에 더한 키 전량.
 *
 * 리터럴 목록인 것에 뜻이 있다 — 코드에서 유도하면 "코드가 부르지 않는 키" 를 셀 수 없고,
 * 문구를 지우면서 호출부를 함께 지운 변경이 가드를 **줄이면서** 초록으로 남는다. 이 목록은
 * 손으로 적어야 그때 diff 에 드러난다.
 */
const EDIT_KEYS: readonly string[] = [
  'paletteGroupPrimitive',
  'paletteGroupGeneral',
  'paletteGroupBasic',
  'paletteGroupArrow',
  'paletteShapeAria',
  'dockScratchpad',
  'scratchpadDropHint',
  'scratchpadSave',
  'scratchpadEmpty',
  'scratchpadLocalOnly',
  'scratchpadAutoName',
  'scratchpadName',
  'scratchpadRemove',
  'scratchpadRemoveAsk',
  'scratchpadRemoveYes',
  'scratchpadRemoveNo',
  'scratchpadNoticeDismiss',
  'scratchpadSaved',
  'scratchpadVolatile',
  'scratchpadNoSelection',
  'scratchpadLimitEntries',
  'scratchpadLimitBytes',
  'scratchpadPlace',
].map((k) => `${EDIT}.${k}`);

/** 요소 목록 쪽에 더한 것 하나. */
const ELEMENT_KEYS: readonly string[] = ['dashboard.canvas.elements.kindPath'];

/** 카탈로그 이름 30종 — **목록에서 유도한다**(도형이 늘면 가드도 늘어난다). */
const CATALOG_NAME_KEYS: readonly string[] = SHAPE_CATALOG.map((s) => s.nameKey);

/**
 * 묶음 제목 셋 — 코드가 든 그 표에서 유도한다.
 *
 * **뒤집힌 수 (SPEC-CANVAS-011 REQ-01).** 008 에서는 넷이었다. 011 이 `원시형` 묶음을
 * 걷어내 셋이 되었다. 그 키(`paletteGroupPrimitive`)는 **소비처만 사라지고 로케일 파일에는
 * 남으므로** 위 `EDIT_KEYS` 에서는 빠지지 않는다 — 남긴 키가 조용히 지워지면 여기가 운다.
 */
const GROUP_TITLE_KEYS: readonly string[] = PALETTE_GROUP_IDS.map(
  (id) => PALETTE_GROUP_TITLE_KEYS[id],
);

const ALL_008_KEYS: readonly string[] = [
  ...EDIT_KEYS,
  ...ELEMENT_KEYS,
  ...CATALOG_NAME_KEYS,
  ...GROUP_TITLE_KEYS,
];

/**
 * 치환자를 **두 번 이상** 말하는 문구와 그 횟수.
 *
 * 008 이 이 형상을 일부러 만든 자리다(plan.md M11 · 위험 R13). 횟수를 여기 적어 두면
 * 번역을 다듬다 둘째 자리를 지우는 변경이 **그 자리에서** 울린다 — 지우고 나면
 * `replaceAll` 을 `replace` 로 되돌려도 아무 시험이 울지 않기 때문이다.
 */
const REPEATED_TOKENS: ReadonlyArray<readonly [key: string, token: string, times: number]> = [
  [`${EDIT}.paletteShapeAria`, '{shape}', 2],
  [`${EDIT}.scratchpadLimitEntries`, '{max}', 2],
  [`${EDIT}.scratchpadLimitBytes`, '{maxKb}', 2],
];

// --- 조회 ---------------------------------------------------------------

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

/**
 * 묶음 하나를 **편다**. 이미 펴져 있으면 아무 일도 하지 않는다 — 011 이 `기본` 을 펼친 채로
 * 태어나게 했으므로(REQ-01) 무조건 누르는 몸짓은 그 묶음을 **닫는다**.
 */
function openGroup(id: string): void {
  const head = screen.getByTestId(`canvas-palette-group-${id}`);
  if (head.getAttribute('aria-expanded') === 'false') fireEvent.click(head);
}

/** 문구 안의 `{…}` 치환자 다중 집합. 정렬해 두어 비교가 순서에 흔들리지 않는다. */
function tokensOf(text: string): string[] {
  return [...text.matchAll(/\{[a-zA-Z0-9_]+\}/g)].map((m) => m[0]).sort();
}

// --- 형상 가드 (DOM 없이) -------------------------------------------------

describe('008 이 더한 문구는 ko · en 양쪽에 있다 (REQ-06 · 품질 게이트 Unified)', () => {
  it('키 전량이 양쪽 로케일에서 **비어 있지 않은 문자열**로 잡힌다', () => {
    // 켜져 있음을 먼저 단언한다 — 목록이 비면 아래 순회가 0회 돌고 초록이 된다.
    expect(ALL_008_KEYS.length).toBeGreaterThanOrEqual(23 + 1 + 30 + 3);

    const missing: string[] = [];
    for (const key of ALL_008_KEYS) {
      for (const [name, tree] of Object.entries(LOCALES)) {
        const text = lookup(tree, key);
        if (text === undefined || text === '') missing.push(`${name}: ${key}`);
      }
    }
    expect(missing).toEqual([]);
  });

  it('카탈로그 30종의 이름이 **동적 키라서** 기존 가드를 지나지 않는다 — 여기서 센다', () => {
    // `i18nKeyShape.test.ts` 의 `T_CALL` 은 `t('리터럴')` 만 잡고, 카탈로그 이름은
    // `t(shape.nameKey)` 로 불린다. 그 구멍을 이 시험이 메운다.
    expect(CATALOG_NAME_KEYS).toHaveLength(30);
    for (const key of CATALOG_NAME_KEYS) {
      expect(lookup(ko, key), `ko: ${key}`).toBeTypeOf('string');
      expect(lookup(en, key), `en: ${key}`).toBeTypeOf('string');
    }
  });

  it('키 이름 안에 점이 없다 — 어떤 경로로도 닿지 않는 키를 만들지 않는다', () => {
    const segments = ALL_008_KEYS.flatMap((k) => k.split('.'));
    expect(segments.filter((s) => s === '')).toEqual([]);
    // 마지막 마디가 카탈로그 id 이므로 id 에 점이 들어오면 여기서 잡힌다.
    for (const key of CATALOG_NAME_KEYS) {
      expect(key.split('.').filter((s) => s === '')).toEqual([]);
    }
  });

  it('두 로케일의 **치환자 다중 집합이 같다** — 한쪽에서 하나가 빠지면 빨개진다', () => {
    const mismatched: string[] = [];
    for (const key of ALL_008_KEYS) {
      const koTokens = tokensOf(lookup(ko, key) ?? '');
      const enTokens = tokensOf(lookup(en, key) ?? '');
      if (JSON.stringify(koTokens) !== JSON.stringify(enTokens)) {
        mismatched.push(`${key}: ko=${koTokens.join(',')} en=${enTokens.join(',')}`);
      }
    }
    expect(mismatched).toEqual([]);
  });

  it('치환자를 **두 번** 말하는 문구가 양쪽에서 그대로 두 번이다 (D7 고정 입력의 가드)', () => {
    for (const [key, token, times] of REPEATED_TOKENS) {
      for (const [name, tree] of Object.entries(LOCALES) as [LocaleName, unknown][]) {
        const text = lookup(tree, key);
        expect(text, `${name}: ${key}`).toBeTypeOf('string');
        expect(tokensOf(text ?? '').filter((t) => t === token), `${name}: ${key}`).toHaveLength(
          times,
        );
      }
    }
  });
});

// --- 고정 입력 (렌더) -----------------------------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

const CLOUD: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

function pathEl(over: Partial<PathElement> = {}): PathElement {
  return {
    id: 'p1',
    kind: 'path',
    geometry: { x: 37, y: 61, w: 160, h: 90 },
    path: [...CLOUD],
    catalog_id: 'cloud',
    style: {},
    ...over,
  };
}

/** 도크가 선 표면. `en` 을 고르려면 로케일을 저장소에 심어 둔다(Provider 가 그것을 읽는다). */
function Harness({ initial }: { initial: readonly CanvasElement[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <I18nProvider>
      <CanvasEditSelectionContext value={state}>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={PROJ}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </CanvasEditDockRegion>
      </CanvasEditSelectionContext>
    </I18nProvider>
  );
}

/** 로케일을 고른다. `I18nProvider` 는 마운트 때 `xflow-locale` 을 읽는다. */
function useLocale(name: LocaleName): void {
  globalThis.localStorage.setItem('xflow-locale', name);
}

beforeEach(() => {
  globalThis.localStorage.clear();
  useScratchpadStore.setState({ entries: [], notice: null });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.localStorage.clear();
});

// --- en 으로 재는 치환 ----------------------------------------------------

describe('`en` 로케일에서도 치환이 두 자리 모두 일어난다 (D7 — 지금까지 ko 만 쟀다)', () => {
  it('카탈로그 칸의 접근성 이름에 벌거벗은 `{shape}` 가 남지 않는다', () => {
    useLocale('en');
    render(<Harness initial={[]} />);
    // 011 이후 `기본` 은 펼쳐진 채로 태어난다 — 무조건 누르면 닫힌다(REQ-01).
    openGroup('basic');

    const cell = screen.getByTestId('canvas-catalog-add-cloud');
    const label = cell.getAttribute('aria-label') ?? '';
    const name = lookup(en, `${EDIT}.catalogNames.cloud`) ?? '';

    expect(name).not.toBe('');
    expect(label).not.toContain('{');
    // 두 자리 모두 — `replace` 면 뒤쪽이 `{shape}` 로 남는다.
    expect(label.split(name)).toHaveLength(3);
    // WCAG 2.5.3 — 접근성 이름이 보이는 라벨을 **포함한다**.
    expect(cell.textContent).toContain(name);
    expect(label).toContain(name);
  });

  // 로케일 순회를 `it` **밖**에 둔다 — `useLocale` 은 이름이 훅을 닮아서 함수 본문의
  // 반복문 안에서 부르면 `react-hooks/rules-of-hooks` 가 운다(이 파일의 다른 순회와 같은 꼴).
  for (const locale of ['ko', 'en'] as const) {
    it(`${locale}: 원시형 네 칸도 접근성 이름이 보이는 이름을 품는다 (WCAG 2.5.3)`, () => {
      // 011 이 넷을 카탈로그와 **같은 격자의 칸**으로 만들었으므로, 이웃한 서른 칸이 지키는
      // 성질을 이 넷도 지키는지 잰다. 넷의 두 문구는 카탈로그와 달리 치환이 아니라 **따로
      // 적힌 두 문장**이라(`paletteRect` 와 `shapeRect`) 포함이 저절로 성립하지 않는다.
      //
      // ko 는 글자 그대로 품는다("사각형" ⊂ "사각형 놓기"). en 은 **대소문자만** 어긋난다
      // ("Rectangle" 대 "Place rectangle") — 그래서 접어서 잰다. WCAG 2.5.3 은 대소문자를
      // 따지지 않으므로 이것으로 충족이지만, 그 어긋남이 있다는 사실 자체를 여기 적어 둔다.
      // 011 이 만든 것이 아니라 008 이전부터 그랬고, 이 시험이 그것을 처음 고정한다.
      const pairs: ReadonlyArray<readonly [kind: string, ariaKey: string, nameKey: string]> = [
        ['rect', 'paletteRect', 'shapeRect'],
        ['ellipse', 'paletteEllipse', 'shapeEllipse'],
        ['line', 'paletteLine', 'shapeLine'],
        ['text', 'paletteText', 'shapeText'],
      ];

      useLocale(locale);
      render(<Harness initial={[]} />);
      for (const [kind, ariaKey, nameKey] of pairs) {
        const cell = screen.getByTestId(`canvas-palette-add-${kind}`);
        const label = cell.getAttribute('aria-label') ?? '';
        const name = lookup(LOCALES[locale], `${EDIT}.${nameKey}`) ?? '';
        expect(name, `${locale}:${nameKey}`).not.toBe('');
        expect(label, `${locale}:${ariaKey}`).toBe(lookup(LOCALES[locale], `${EDIT}.${ariaKey}`));
        // 보이는 이름이 곧 칸의 글자다.
        expect(cell.textContent, `${locale}:${kind} 의 보이는 이름`).toContain(name);
        // 접근성 이름이 그것을 품는다(대소문자 접어서).
        expect(label.toLowerCase(), `${locale}:${kind} 의 접근성 이름`).toContain(
          name.toLowerCase(),
        );
      }
    });
  }

  it('한도 안내가 상한을 두 번 말하고 두 자리 모두 숫자다', () => {
    useLocale('en');
    const entries: ScratchpadEntry[] = Array.from(
      { length: SCRATCHPAD_MAX_ENTRIES },
      (_unused, i) => ({
        id: `sp-${i + 1}`,
        name: '',
        created: 0,
        origin: { x: 0, y: 0 },
        elements: [
          { id: 'el-1', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
        ],
      }),
    );
    useScratchpadStore.setState({ entries });

    render(<Harness initial={[]} />);
    // 저장 시도를 store 로 직접 낸다 — 여기서 재는 것은 **문구**이지 몸짓이 아니다.
    act(() => {
      useScratchpadStore.getState().saveEntry([
        { id: 'el-9', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
      ]);
    });

    const text = screen.getByTestId('canvas-scratchpad-notice').textContent ?? '';
    expect(text).not.toContain('{');
    expect(text.split(String(SCRATCHPAD_MAX_ENTRIES))).toHaveLength(3);
  });

  // **뒤집힌 수 (SPEC-CANVAS-011 REQ-01 · AC-01).** 008 의 묶음은 넷이었다.
  it('묶음 머리 셋이 en 번역을 보이고 원문 키가 뜨지 않는다', () => {
    useLocale('en');
    render(<Harness initial={[]} />);
    for (const id of PALETTE_GROUP_IDS) {
      const head = screen.getByTestId(`canvas-palette-group-${id}`);
      expect(head.textContent).toBe(lookup(en, PALETTE_GROUP_TITLE_KEYS[id]));
      expect(head.textContent).not.toContain('dashboard.');
    }
  });
});

// --- 렌더된 것 전량에 원문 키도 벌거벗은 치환자도 없다 ----------------------

describe('도크 안의 008 표면에 원문 키도 벌거벗은 치환자도 없다 (양쪽 로케일)', () => {
  for (const locale of ['ko', 'en'] as const) {
    it(`${locale}: 묶음 셋을 모두 펴고 서랍에 항목을 하나 둔 상태에서도 없다`, () => {
      useLocale(locale);
      useScratchpadStore.setState({
        entries: [
          {
            id: 'sp-1',
            name: '',
            created: 0,
            origin: { x: 0, y: 0 },
            elements: [pathEl()],
          },
        ],
        notice: null,
      });
      render(<Harness initial={[pathEl()]} />);
      for (const id of PALETTE_GROUP_IDS) {
        const head = screen.getByTestId(`canvas-palette-group-${id}`);
        if (head.getAttribute('aria-expanded') === 'false') fireEvent.click(head);
      }

      const dock = screen.getByTestId('canvas-dock-panel');
      // 켜져 있음을 먼저 단언한다 — 칸이 0개면 아래 순회가 아무것도 재지 않는다.
      expect(dock.querySelectorAll('[data-testid^="canvas-catalog-add-"]')).toHaveLength(30);
      expect(screen.getByTestId('canvas-scratchpad-list')).toBeTruthy();

      const texts: string[] = [];
      for (const node of dock.querySelectorAll('*')) {
        if (node.childElementCount === 0 && node.textContent !== null) texts.push(node.textContent);
        for (const attr of ['aria-label', 'title', 'placeholder']) {
          const v = node.getAttribute(attr);
          if (v !== null) texts.push(v);
        }
      }

      expect(texts.filter((s) => s.includes('{'))).toEqual([]);
      expect(texts.filter((s) => s.startsWith('dashboard.'))).toEqual([]);
    });
  }
});

// --- 접근성: 이름 · 역할 · 키보드 도달 -------------------------------------

describe('드롭 존 · 목록 · 단추의 이름과 역할 (M11 · WCAG 2.5.3 · SC 2.5.7)', () => {
  beforeEach(() => {
    useScratchpadStore.setState({
      entries: [
        { id: 'sp-1', name: '', created: 0, origin: { x: 0, y: 0 }, elements: [pathEl()] },
      ],
      notice: null,
    });
  });

  it('서랍의 컨트롤 전부가 키보드로 닿는 `button` 이다 — 끌기 전용 조작이 없다', () => {
    render(<Harness initial={[]} />);
    const ids = [
      'canvas-scratchpad-save',
      'canvas-scratchpad-place-sp-1',
      'canvas-scratchpad-remove-sp-1',
    ];
    for (const id of ids) {
      const node = screen.getByTestId(id);
      expect(node.tagName, id).toBe('BUTTON');
      // `type="button"` 이 없으면 도크가 폼 안에 놓였을 때 제출 단추가 된다.
      expect(node.getAttribute('type'), id).toBe('button');
      // 명시적으로 손대지 않는다 — 기본 tab 차례가 곧 시각 차례다.
      expect(node.getAttribute('tabindex'), id).toBeNull();
      expect(node.getAttribute('aria-label'), id).toBeTruthy();
    }
    // 지우기는 두 걸음이고, 두 걸음째도 키보드로 닿는다.
    fireEvent.click(screen.getByTestId('canvas-scratchpad-remove-sp-1'));
    for (const id of ['canvas-scratchpad-remove-yes-sp-1', 'canvas-scratchpad-remove-no-sp-1']) {
      expect(screen.getByTestId(id).tagName, id).toBe('BUTTON');
      expect(screen.getByTestId(id).getAttribute('aria-label'), id).toBeTruthy();
    }
  });

  it('카탈로그 30칸이 전부 `button` 이고 각자 이름을 든다', () => {
    render(<Harness initial={[]} />);
    for (const id of PALETTE_GROUP_IDS) {
      const head = screen.getByTestId(`canvas-palette-group-${id}`);
      if (head.getAttribute('aria-expanded') === 'false') fireEvent.click(head);
    }
    const cells = screen.getAllByTestId(/^canvas-catalog-add-/);
    expect(cells).toHaveLength(30);
    for (const cell of cells) {
      expect(cell.tagName).toBe('BUTTON');
      expect(cell.getAttribute('type')).toBe('button');
      expect(cell.getAttribute('aria-label')).toBeTruthy();
      expect(cell.getAttribute('tabindex')).toBeNull();
    }
  });

  it('묶음 머리는 접힘을 `aria-expanded` 로 말하고, 펼쳤을 때만 몸통을 가리킨다', () => {
    render(<Harness initial={[]} />);
    const head = screen.getByTestId('canvas-palette-group-general');
    expect(head.getAttribute('aria-expanded')).toBe('false');
    // 없는 id 를 가리키는 `aria-controls` 는 보조기술에게 거짓말이다.
    expect(head.getAttribute('aria-controls')).toBeNull();

    fireEvent.click(head);
    expect(head.getAttribute('aria-expanded')).toBe('true');
    const controls = head.getAttribute('aria-controls');
    expect(controls).toBeTruthy();
    expect(document.getElementById(controls ?? '')).toBe(
      screen.getByTestId('canvas-palette-group-body-general'),
    );
  });

  it('장식은 이름을 나르지 않는다 — 008 이 그린 그림 전량이 `aria-hidden` 이다', () => {
    render(<Harness initial={[]} />);
    openGroup('basic');

    // **008 이 소유한 자리만 잰다.** 도크 전체로 넓히면 008 이 만들지 않은 공용 부품
    // (`components/property/FieldHelp.tsx` 의 물음표 아이콘 — `aria-hidden` 이 없다)이
    // 함께 걸린다. 그 부품은 단추가 이미 `aria-label` 을 들어 접근성 이름이 그림에서
    // 나오지 않으며, 고치는 일은 이 SPEC 이 건드리기로 한 파일 밖이다.
    const dock = screen.getByTestId('canvas-dock-panel');
    const owned = [
      ...dock.querySelectorAll('[data-testid^="canvas-palette-group-"]'),
      ...dock.querySelectorAll('[data-testid^="canvas-catalog-"]'),
      ...dock.querySelectorAll('[data-testid^="canvas-scratchpad-"]'),
    ];
    const graphics = owned.flatMap((node) => [...node.querySelectorAll('svg, canvas')]);
    // 켜져 있음을 먼저 단언한다 — 그림이 0개면 이 시험은 아무것도 재지 않는다.
    expect(graphics.length).toBeGreaterThan(10);
    expect(graphics.filter((g) => g.getAttribute('aria-hidden') !== 'true')).toEqual([]);
  });

  it('안내는 `role="status"` 로 나가고 닫는 단추가 제 이름을 든다', () => {
    render(<Harness initial={[]} />);
    act(() => {
      useScratchpadStore.getState().saveEntry([]);
    });
    const notice = screen.getByTestId('canvas-scratchpad-notice');
    expect(notice.getAttribute('role')).toBe('status');
    expect(
      screen.getByTestId('canvas-scratchpad-notice-dismiss').getAttribute('aria-label'),
    ).toBeTruthy();
  });
});

// --- 요소 목록의 이름: 카탈로그가 앞선다 -----------------------------------

describe('요소 목록의 경로 행은 **카탈로그 이름**을 말한다 (M11 · spec.md §출처 기록)', () => {
  function renderEditor(elements: readonly CanvasElement[]): void {
    render(
      <I18nProvider>
        <CanvasElementsEditor
          config={{ canvas: { ...DEFAULT_CANVAS_SIZE }, elements: [...elements] }}
          onConfigChange={vi.fn()}
        />
      </I18nProvider>,
    );
    for (const btn of screen.queryAllByTestId(/^canvas-element-toggle-\d+$/)) fireEvent.click(btn);
    fireEvent.click(screen.getByTestId('canvas-element-tab-style-0'));
  }

  it('`catalog_id` 가 카탈로그에 있으면 그 도형의 이름이다 — 정체 모를 "경로" 가 아니다', () => {
    useLocale('ko');
    renderEditor([pathEl({ catalog_id: 'cloud' })]);
    const select = screen.getByTestId('canvas-element-kind-0');
    const disabled = select.querySelector('option[disabled]');
    expect(disabled?.textContent).toBe(lookup(ko, `${EDIT}.catalogNames.cloud`));
    expect(disabled?.textContent).not.toBe(lookup(ko, 'dashboard.canvas.elements.kindPath'));
  });

  it('en 에서도 그 도형의 이름이다', () => {
    useLocale('en');
    renderEditor([pathEl({ catalog_id: 'star5' })]);
    expect(
      screen.getByTestId('canvas-element-kind-0').querySelector('option[disabled]')?.textContent,
    ).toBe(lookup(en, `${EDIT}.catalogNames.star5`));
  });

  it('`catalog_id` 가 없거나 모르는 값이면 일반 이름으로 떨어진다 — 그림은 그대로다', () => {
    useLocale('ko');
    const generic = lookup(ko, 'dashboard.canvas.elements.kindPath');
    renderEditor([pathEl({ catalog_id: undefined }), pathEl({ id: 'p2', catalog_id: 'nope' })]);
    for (const idx of [0, 1]) {
      const select = screen.getByTestId(`canvas-element-kind-${idx}`);
      expect(select.querySelector('option[disabled]')?.textContent).toBe(generic);
    }
  });

  it('원시형 넷의 이름은 008 이전과 같다 — 카탈로그가 그 자리를 건드리지 않는다', () => {
    useLocale('ko');
    renderEditor([
      { id: 'r1', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
    ]);
    const options = [...screen.getByTestId('canvas-element-kind-0').querySelectorAll('option')];
    expect(options.map((o) => o.textContent)).toEqual([
      lookup(ko, 'dashboard.canvas.elements.kindRect'),
      lookup(ko, 'dashboard.canvas.elements.kindEllipse'),
      lookup(ko, 'dashboard.canvas.elements.kindLine'),
      lookup(ko, 'dashboard.canvas.elements.kindText'),
    ]);
    // 경로 행이 아니므로 고를 수 없는 칸이 서지 않는다.
    expect(options.filter((o) => o.hasAttribute('disabled'))).toEqual([]);
  });
});
