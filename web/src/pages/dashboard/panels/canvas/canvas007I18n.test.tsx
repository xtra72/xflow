// 007 이 더한 문구 전량의 형상 가드 — 양쪽 로케일 · 접근성 (SPEC-CANVAS-007 M10 · E13 · E14).
//
// 이 파일이 겨누는 구멍 셋. 셋 다 **전량 초록인 채로** 화면에 원문 키나 벌거벗은 치환자를
// 배달할 수 있는 자리다.
//
// **하나 — 기본 로케일이 `ko` 다**(실측 `lib/i18n/index.ts` §`DEFAULT_LOCALE`). `I18nProvider`
// 를 그냥 세우면 **언제나 한국어**라, `en.json` 에서 문구 하나가 빠지거나 치환자가 하나 빠져
// 있어도 오늘의 렌더 시험 전량이 초록이다. 여기서는 **로케일을 갈아 끼우며** 같은 단언을
// 양쪽에서 돌리고, 신규 키 44개에 대해 **두 파일의 치환자 다중 집합이 같음**을 단언한다.
//
// **둘 — 동적 키는 `i18nKeyShape.test.ts` 의 눈에 보이지 않는다.** 그 가드의 정규식은
// `t('리터럴')` 만 잡는데(`T_CALL`), 007 의 보고 사유 23종과 거절 사유 7종은 `t(NOTE_KEYS[r])`
// 로 불리므로 그 가드를 **지나가지 않는다**. 여기서는 **표에서 유도해** 전량을 센다 —
// 사유가 늘면 가드가 저절로 넓어진다.
//
// **셋 — `replace` 대 `replaceAll`**(위험 R14). 007 의 문구 셋이 상한을 **두 번** 말한다.
// 그 성질은 시험이 지키지 않으면 조용히 사라진다 — 번역을 다듬다 둘째 자리를 지우면
// `replaceAll` 을 `replace` 로 되돌려도 아무 시험이 울지 않는다. 그래서 "두 번 말한다" 자체를
// 고정 입력이 아니라 **가드**로 둔다.
//
// **끌기가 없다.** 007 은 파일을 캔버스 위로 끌어놓는 길을 두지 않으므로 WCAG 2.2 SC 2.5.7
// (끌기 동작)의 "포인터 끌기 없이도 되는 등가물" 요구가 **발생하지 않는다**. 그 사실을
// 아래 시험이 함께 적는다 — 적지 않으면 훗날 드롭을 더하는 변경이 "접근성은 그대로" 라고
// 잘못 읽힌다.
//
// @spec SPEC-CANVAS-007 REQ-04 · AC-07 · AC-E11 · E13 · E14

import { useState } from 'react';

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { DEFAULT_CANVAS_SIZE, type CanvasElement } from './canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import { NOTE_KEYS, REFUSAL_KEYS } from './svgimport/svgImportPresent';
import { MAX_IMPORT_ELEMENTS } from './svgimport/svgImportTypes';
import type { CanvasNode } from './group/groupTypes';

// --- 키 목록 -------------------------------------------------------------

const EDIT = 'dashboard.canvas.edit';

/**
 * 007 이 `dashboard.canvas.edit` 아래에 더한 **구조 문구** 전량.
 *
 * 리터럴 목록인 것에 뜻이 있다 — 코드에서 유도하면 "코드가 부르지 않는 키" 를 셀 수 없고,
 * 문구를 지우면서 호출부를 함께 지운 변경이 가드를 **줄이면서** 초록으로 남는다. 이 목록은
 * 손으로 적어야 그때 diff 에 드러난다(008 이 같은 자리에서 한 판단).
 */
const STRUCTURE_KEYS: readonly string[] = [
  'dockImport',
  'importGroup',
  'importPick',
  'importPickAgain',
  'importHint',
  'importSummary',
  'importNotesToggle',
  'importNotesApproximated',
  'importNotesDropped',
  'importNoteItem',
  'importPlace',
  'importCancel',
  'importNothing',
  'importCommandLimitHint',
].map((k) => `${EDIT}.${k}`);

/** 사유 문구 37종 — **표에서 유도한다**(사유가 늘면 가드도 늘어난다). */
const REASON_KEYS: readonly string[] = [...Object.values(NOTE_KEYS), ...Object.values(REFUSAL_KEYS)];

const ALL_007_KEYS: readonly string[] = [...STRUCTURE_KEYS, ...REASON_KEYS];

/**
 * 치환자를 **두 번 이상** 말하는 문구와 그 횟수.
 *
 * 007 이 이 형상을 일부러 만든 자리다(plan.md M10 · 위험 R14). 횟수를 여기 적어 두면 번역을
 * 다듬다 둘째 자리를 지우는 변경이 **그 자리에서** 울린다.
 */
const REPEATED_TOKENS: ReadonlyArray<readonly [key: string, token: string, times: number]> = [
  [REFUSAL_KEYS.tooManyElements, '{limit}', 2],
  [REFUSAL_KEYS.tooManyCommands, '{limit}', 2],
  [REFUSAL_KEYS.fileTooLarge, '{limitKb}', 2],
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

/** 문구 안의 `{…}` 치환자 다중 집합. 정렬해 두어 비교가 순서에 흔들리지 않는다. */
function tokensOf(text: string): string[] {
  return [...text.matchAll(/\{[a-zA-Z0-9_]+\}/g)].map((m) => m[0]).sort();
}

// --- 형상 가드 (DOM 없이) -------------------------------------------------

describe('007 이 더한 문구는 ko · en 양쪽에 있다 (E13 · 품질 게이트 Unified)', () => {
  it('키 51개가 양쪽 로케일에서 **비어 있지 않은 문자열**로 잡힌다', () => {
    // 켜져 있음을 먼저 단언한다 — 목록이 비면 아래 순회가 0회 돌고 초록이 된다.
    expect(STRUCTURE_KEYS).toHaveLength(14);
    expect(REASON_KEYS).toHaveLength(37);
    expect(ALL_007_KEYS).toHaveLength(51);

    const missing: string[] = [];
    for (const key of ALL_007_KEYS) {
      for (const [name, tree] of Object.entries(LOCALES)) {
        const text = lookup(tree, key);
        if (text === undefined || text === '') missing.push(`${name}: ${key}`);
      }
    }
    expect(missing).toEqual([]);
  });

  it('사유 37종은 **동적 키라서** 기존 가드를 지나지 않는다 — 여기서 센다', () => {
    // `i18nKeyShape.test.ts` 의 `T_CALL` 은 `t('리터럴')` 만 잡고, 사유 문구는 `t(NOTE_KEYS[r])`
    // 로 불린다. 그 구멍을 이 시험이 메운다.
    for (const key of REASON_KEYS) {
      expect(lookup(ko, key), `ko: ${key}`).toBeTypeOf('string');
      expect(lookup(en, key), `en: ${key}`).toBeTypeOf('string');
    }
  });

  it('키 이름 안에 점이 없다 — 어떤 경로로도 닿지 않는 키를 만들지 않는다', () => {
    for (const key of ALL_007_KEYS) {
      expect(key.split('.').filter((s) => s === '')).toEqual([]);
      expect(key.split('.')).toHaveLength(4);
    }
  });

  it('두 로케일의 **치환자 다중 집합이 같다** — 한쪽에서 하나가 빠지면 빨개진다', () => {
    const mismatched: string[] = [];
    for (const key of ALL_007_KEYS) {
      const koTokens = tokensOf(lookup(ko, key) ?? '');
      const enTokens = tokensOf(lookup(en, key) ?? '');
      if (JSON.stringify(koTokens) !== JSON.stringify(enTokens)) {
        mismatched.push(`${key}: ko=${koTokens.join(',')} en=${enTokens.join(',')}`);
      }
    }
    expect(mismatched).toEqual([]);
    // 켜져 있음: 치환자를 가진 문구가 실제로 있다(전부 없으면 위 비교가 무력하다).
    expect(ALL_007_KEYS.filter((k) => tokensOf(lookup(ko, k) ?? '').length > 0).length).toBeGreaterThanOrEqual(6);
  });

  it('치환자를 **두 번** 말하는 문구가 양쪽에서 그대로 두 번이다 (E14 고정 입력의 가드)', () => {
    for (const [key, token, times] of REPEATED_TOKENS) {
      for (const [name, tree] of Object.entries(LOCALES) as [LocaleName, unknown][]) {
        const text = lookup(tree, key);
        expect(text, `${name}: ${key}`).toBeTypeOf('string');
        expect(tokensOf(text ?? '').filter((t) => t === token), `${name}: ${key}`).toHaveLength(times);
      }
    }
  });
});

// --- 고정 입력 (렌더) -----------------------------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

const VIEW_BOX = 'viewBox="-13 7 317 181"';

/** 버림 셋과 근사 하나가 실제로 들어 있는 문서 — 보고가 켜져 있어야 문구가 재어진다. */
const NOISY =
  `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}>` +
  '<style>.a{fill:red}</style>' +
  '<text x="1" y="2">버려짐</text>' +
  '<image href="a.png" x="0" y="0" width="4" height="4"/>' +
  '<path d="M0 0 L10 10 L0 10 Z" fill="#c0392b" fill-rule="evenodd"/>' +
  '</svg>';

/** 요소 상한을 실제로 넘는 문서 — 넘지 않으면 상한 문구가 렌더되지 않는다. */
const OVER_LIMIT =
  `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}>` +
  Array.from(
    { length: MAX_IMPORT_ELEMENTS + 36 },
    (_v, i) => `<rect x="${i % 20}" y="12" width="3" height="3" fill="#c0392b"/>`,
  ).join('') +
  '</svg>';

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

async function chooseDoc(text: string, ready = true): Promise<void> {
  // **이미 펴져 있으면 다시 누르지 않는다** — 누르면 접혀서 파일 입력이 사라진다.
  if (screen.queryByTestId('canvas-svg-import-group-body') === null) {
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
  }
  fireEvent.change(screen.getByTestId('canvas-svg-import-file'), {
    target: { files: [new File([text], 'a.svg', { type: 'image/svg+xml' })] },
  });
  await waitFor(() =>
    expect(
      screen.getByTestId(ready ? 'canvas-svg-import-summary' : 'canvas-svg-import-refused'),
    ).toBeTruthy(),
  );
}

beforeEach(() => {
  globalThis.localStorage.clear();
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.localStorage.clear();
});

// --- 두 로케일에서 같은 단언 (E13) ----------------------------------------

for (const locale of ['ko', 'en'] as const) {
  describe(`\`${locale}\` 로케일에서 화면이 원문 키도 벌거벗은 치환자도 내지 않는다 (E13)`, () => {
    it('묶음 머리 · 안내 · 파일 고르기가 **번역된 글자**를 낸다', () => {
      useLocale(locale);
      render(<Harness initial={[]} />);
      const head = screen.getByTestId('canvas-svg-import-group');
      // 키가 그대로 새는 형상은 점이 든 문자열이다 — 그것을 직접 막는다.
      expect(head.textContent ?? '').not.toContain('dashboard.');
      expect(head.textContent ?? '').not.toBe('');
      expect(head.textContent).toBe(lookup(LOCALES[locale], `${EDIT}.importGroup`));

      fireEvent.click(head);
      const pick = screen.getByTestId('canvas-svg-import-pick');
      expect(pick.textContent).toBe(lookup(LOCALES[locale], `${EDIT}.importPick`));
    });

    it('요약과 갈래 목록이 개수를 담고 치환자를 남기지 않는다', async () => {
      useLocale(locale);
      render(<Harness initial={[]} />);
      await chooseDoc(NOISY);

      const summary = screen.getByTestId('canvas-svg-import-summary');
      expect(summary.textContent ?? '').not.toContain('{');
      expect(summary.textContent ?? '').not.toContain('dashboard.');
      // 도형 1개 · 명령 4개 — 켜져 있음을 수로 못박는다.
      expect(summary.textContent).toContain('1');
      expect(summary.textContent).toContain('4');

      const toggle = screen.getByTestId('canvas-svg-import-notes-toggle');
      expect(toggle.textContent ?? '').not.toContain('{');
      fireEvent.click(toggle);

      // 사유가 `textDropped` 에서 `textOrderChanged` 로 바뀌었다(결함 B 정정) — 이 문서의
      // `<text>` 는 이제 요소가 되고, 뒤따르는 `<path>` 위로 올라선 것이 보고에 오른다.
      const item = screen.getByTestId('canvas-svg-import-note-textOrderChanged');
      expect(item.textContent ?? '').not.toContain('{');
      expect(item.textContent ?? '').not.toContain('dashboard.');
      // 사유 이름과 개수가 **둘 다** 들어 있다 — 하나만 있으면 "필터가 있습니다" 로 퇴화한다.
      expect(item.textContent).toContain(lookup(LOCALES[locale], NOTE_KEYS.textOrderChanged) ?? 'x');
      expect(item.textContent).toContain('1');
    });

    it('상한 안내가 상한을 **두 자리 모두** 바꾼다 (E14 · `replace` 였다면 뒤가 남는다)', async () => {
      useLocale(locale);
      render(<Harness initial={[]} />);
      await chooseDoc(OVER_LIMIT, false);

      const line = screen.getByTestId('canvas-svg-import-refused');
      const text = line.textContent ?? '';
      expect(text).not.toContain('{');
      expect(text).not.toContain('dashboard.');
      // 고정 입력이 그 성질을 실제로 갖는지 먼저 잰다.
      const template = lookup(LOCALES[locale], REFUSAL_KEYS.tooManyElements) ?? '';
      expect([...template.matchAll(/\{limit\}/g)]).toHaveLength(2);
      // 상한이 **두 번** 나온다 — 첫 자리만 바꿨다면 하나뿐이다.
      expect([...text.matchAll(new RegExp(String(MAX_IMPORT_ELEMENTS), 'g'))].length).toBe(2);
      expect(text).toContain(String(MAX_IMPORT_ELEMENTS + 36));
    });
  });
}

// --- 접근성 ---------------------------------------------------------------

describe('접근성 (M10 · WCAG 2.2)', () => {
  it('파일 고르기의 **접근 이름**이 보이는 라벨과 같은 것을 가리킨다 (SC 2.5.3)', () => {
    useLocale('ko');
    render(<Harness initial={[]} />);
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));

    const input = screen.getByTestId('canvas-svg-import-file');
    const label = screen.getByTestId('canvas-svg-import-pick');
    const name = input.getAttribute('aria-label') ?? '';
    expect(name).not.toBe('');
    // 보이는 글자가 곧 접근 이름이다 — 음성 조작이 보이는 대로 통한다.
    expect(label.textContent).toContain(name);
    // 화면에서만 숨는다 — `hidden`/`display:none` 은 키보드 초점을 함께 빼앗는다.
    expect(input.className).toContain('sr-only');
    expect(input.hasAttribute('hidden')).toBe(false);
  });

  it('접히는 두 자리가 `aria-expanded` 로 상태를 알리고, 편 뒤에만 `aria-controls` 를 든다', async () => {
    useLocale('ko');
    render(<Harness initial={[]} />);
    const head = screen.getByTestId('canvas-svg-import-group');
    expect(head.getAttribute('aria-expanded')).toBe('false');
    // 없는 id 를 가리키는 `aria-controls` 는 보조기술에게 거짓말이다.
    expect(head.getAttribute('aria-controls')).toBeNull();
    fireEvent.click(head);
    expect(head.getAttribute('aria-expanded')).toBe('true');
    expect(head.getAttribute('aria-controls')).toBe(
      screen.getByTestId('canvas-svg-import-group-body').id,
    );

    await chooseDoc(NOISY);
    const toggle = screen.getByTestId('canvas-svg-import-notes-toggle');
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(toggle.getAttribute('aria-controls')).toBeNull();
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(toggle.getAttribute('aria-controls')).toBe(screen.getByTestId('canvas-svg-import-notes').id);
  });

  it('거절 사유와 "가져올 것 없음" 이 `role="status"` 로 알려진다 — 초점을 빼앗지 않는다', async () => {
    useLocale('ko');
    render(<Harness initial={[]} />);
    await chooseDoc(OVER_LIMIT, false);
    expect(screen.getByTestId('canvas-svg-import-refused').getAttribute('role')).toBe('status');

    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), {
      target: {
        files: [
          new File(
            [`<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}><defs><linearGradient id="g"/></defs></svg>`],
            'b.svg',
          ),
        ],
      },
    });
    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-empty')).toBeTruthy());
    expect(screen.getByTestId('canvas-svg-import-empty').getAttribute('role')).toBe('status');
  });

  it('끌기로만 되는 조작이 **하나도 없다** — SC 2.5.7 의 등가물 요구가 발생하지 않는다', async () => {
    useLocale('ko');
    render(<Harness initial={[]} />);
    await chooseDoc(NOISY);
    fireEvent.click(screen.getByTestId('canvas-svg-import-notes-toggle'));

    const body = screen.getByTestId('canvas-svg-import-group-body');
    // 007 이 세운 노드 가운데 끌 수 있는 것이 없다. 있으면 그 조작에는 끌지 않는 등가물이
    // 함께 있어야 하고, 그 사실을 이 줄이 미래의 변경에게 말한다.
    expect(body.querySelectorAll('[draggable="true"]')).toHaveLength(0);
    expect(body.querySelectorAll('[data-drop-target]')).toHaveLength(0);
    // 켜져 있음: 준비됨 상태에는 실제로 눌리는 컨트롤이 여럿 있다(빈 화면을 재고 있지 않다).
    expect(body.querySelectorAll('button').length).toBeGreaterThanOrEqual(3);

    // 파일은 **고르는** 것이지 끄는 것이 아니다 — 대기 상태의 입구가 `type="file"` 하나다.
    fireEvent.click(screen.getByTestId('canvas-svg-import-cancel'));
    expect(screen.getByTestId('canvas-svg-import-file').getAttribute('type')).toBe('file');
  });
});
