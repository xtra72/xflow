// 006 이 더한 화면의 **문구와 보조기기** — 진짜 번역 문구로 재는 자리 (SPEC-CANVAS-006 M11).
//
// 왜 파일을 나누는가: 이웃한 두 시험 파일(`CanvasEditOverlay.test.tsx` ·
// `CanvasEditDock.test.tsx`)은 i18n 을 **키를 그대로 돌려주는** 것으로 대체한다. 그 대체
// 아래에서는 `t('...keyboardHint')` 가 언제나 그 키 문자열이므로 **문구가 무엇을 말하는가**
// 를 물을 수 없고, 치환자(`{percent}`)가 빈 자리를 만드는 결함도 드러나지 않는다
// (acceptance.md §시험 규율 "진짜 번역 문구로 잰다" · AC-E22 가 세운 그 규율).
// `vi.mock` 은 파일 단위이므로 대체를 달리하려면 파일을 나누는 수밖에 없다
// (`CanvasEditDock.gridStep.test.tsx` 가 M9 에서 같은 이유로 갈라져 나갔다).
//
// 그리고 이 파일은 **두 표면을 모두** 세운다(시험 규율 D9). 006 이 실제로 물린 자리가
// 그것이다 — M9 의 시험은 전부 `docked` 표면에서 돌았고, 대시보드 표면에 컨트롤이 하나도
// 서지 않는다는 사실이 전량 green 인 채로 사용자에게 발견되었다.

import { cleanup, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

vi.mock('@/lib/i18n', async () => {
  const messages = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const lookup = (key: string): string => {
    const value = key
      .split('.')
      .reduce<unknown>(
        (node, seg) =>
          node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
        messages,
      );
    return typeof value === 'string' ? value : key;
  };
  return { useTranslation: () => ({ t: lookup }) };
});

import { DEFAULT_CANVAS_SIZE } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';

afterEach(cleanup);

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 160 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

/**
 * 006 이 더한 키 전부 — **두 네임스페이스에 걸쳐 있다.**
 *
 * `edit` 은 편집 표면(도크 · 줄 · 오버레이)의 것이고, `elements` 는 목록 편집기의 것이다.
 * 한 네임스페이스만 훑는 목록은 `panelSizeDerived` 를 놓친다(M8 이 그 자리에 넣었다).
 */
const ADDED_KEYS = {
  edit: ['dockView', 'workspaceZoomBar', 'workspaceZoom', 'workspaceZoomOption', 'workspaceZoomHint'],
  elements: ['panelSizeDerived'],
} as const;

/** 006 이 걷어낸 키 — 양쪽 언어에서 사라졌어야 한다(AC-07 (AN)). 전부 `elements` 의 것이다. */
const REMOVED_KEYS = {
  edit: [],
  elements: [
    'panelWidthAria',
    'panelHeightAria',
    'panelSizeFit',
    'panelSizeFitAria',
    'panelSizeFitTitle',
    'panelSizeFitNone',
  ],
} as const;

type Namespace = keyof typeof ADDED_KEYS;

function namespaceOf(messages: typeof ko | typeof en, ns: Namespace): Record<string, unknown> {
  return (
    messages as unknown as {
      dashboard: { canvas: Record<Namespace, Record<string, unknown>> };
    }
  ).dashboard.canvas[ns];
}

function editNamespace(messages: typeof ko | typeof en): Record<string, unknown> {
  return namespaceOf(messages, 'edit');
}

/**
 * `docked` 면 도크가 서는 표면, 아니면 대시보드 표면(줄이 서는 자리)이다.
 *
 * 표면을 인자로 받는 것이 이 파일의 요점이다 — 한 표면만 재는 시험은 D9 를 만족시키지
 * 못한다.
 */
function Harness({ docked }: { docked: boolean }): React.JSX.Element {
  const [elements, setElements] = useState<readonly CanvasNode[]>([]);
  const selection = useCanvasEditSelectionState();
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
    <CanvasEditSelectionContext value={selection}>
      {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
    </CanvasEditSelectionContext>
  );
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

/** 루트가 `aria-describedby` 로 가리키는 그 문단 — 보조기기가 실제로 읽는 글자다. */
function surfaceDescription(): HTMLElement {
  const id = overlayRoot().getAttribute('aria-describedby');
  expect(id, 'aria-describedby').toBeTruthy();
  const node = document.getElementById(id!);
  expect(node, `#${id}`).toBeTruthy();
  return node!;
}

// --- 편집 표면의 설명문 (M11) --------------------------------------------

describe('편집 표면의 설명문이 006 이 그린 것을 말한다 (M11)', () => {
  it('방향키만이 아니라 **출력 영역 밖**과 **흐림·경계의 뜻**을 함께 말한다', () => {
    // 006 이 화면에 더한 것 셋(저술 여백 · 흐림 · 경계)은 전부 `aria-hidden` 인 장식이다.
    // 그러므로 보조기기 사용자에게 그 셋은 **존재하지 않는다** — 유일하게 남는 통로가
    // 이 설명문이고, 그것이 002 의 문구 그대로면 006 은 눈으로 보는 사람에게만 배달된
    // 것이다.
    render(<Harness docked={false} />);

    const text = surfaceDescription().textContent ?? '';
    // 002 가 말하던 것은 그대로 남는다 — 이 회차는 지우지 않고 더한다.
    expect(text).toContain('방향키');
    expect(text).toContain('Shift');
    // 006 이 더하는 것 (1) — 밖에 놓을 수 있고, 그 밖은 패널에 보이지 않는다.
    expect(text).toContain('출력 영역');
    expect(text).toContain('보이지 않습니다');
    // 006 이 더하는 것 (2) — 흐림과 경계가 무엇을 뜻하는가.
    expect(text).toContain('경계');
    // 키가 그대로 새어 나오지 않는다(대체가 진짜 문구를 돌려준다는 사실의 확인).
    expect(text).not.toContain('dashboard.canvas.edit');
    expect(text).not.toMatch(/\{[a-zA-Z]+\}/);
  });

  it('두 언어가 **같은 셋**을 말한다 — 한쪽만 손보면 다른 쪽이 002 에 머문다', () => {
    // 문구를 더할 때 한 언어만 고치는 것이 이 저장소의 실제 실패 방식이다
    // (`gridStepPartial` 이 M9 에서 같은 가드를 세웠다).
    const hints = {
      ko: editNamespace(ko).keyboardHint as string,
      en: editNamespace(en).keyboardHint as string,
    };
    expect(hints.ko).toContain('출력 영역');
    expect(hints.ko).toContain('경계');
    expect(hints.en).toContain('output region');
    expect(hints.en).toContain('border');
    // 그리고 실제로 **번역**이다 — 한쪽을 복사해 두면 번역이 없는 것과 같다.
    expect(hints.ko).not.toBe(hints.en);
    // 002 의 문장이 잘려 나가지 않았다.
    for (const hint of Object.values(hints)) expect(hint).toMatch(/Shift/);
  });

  it('설명문은 눈에 보이지 않고, 두 표면 **모두**에 선다', () => {
    // 도크가 없는 표면에서 설명문이 사라지면 대시보드에서 편집하는 사람에게 006 의
    // 안내가 통째로 없다 — M10 이 고친 바로 그 부류의 결함이다(불변식 I23).
    for (const docked of [true, false] as const) {
      cleanup();
      render(<Harness docked={docked} />);
      const node = surfaceDescription();
      expect(node.className, `@docked=${docked}`).toContain('sr-only');
      expect((node.textContent ?? '').length, `@docked=${docked}`).toBeGreaterThan(0);
    }
  });
});

// --- 치환자 (M11) ---------------------------------------------------------

describe('치환자가 빈 자리를 만들지 않는다 (M11 · 진짜 문구로만 잴 수 있다)', () => {
  it('배율 제안 넷이 진짜 문구로 채워지고 `{percent}` 가 남지 않는다 — **두 표면 모두**', () => {
    for (const docked of [true, false] as const) {
      cleanup();
      render(<Harness docked={docked} />);

      const options = [
        ...screen.getByTestId('canvas-workspace-zoom-suggestions').querySelectorAll('option'),
      ];
      expect(options.map((o) => o.value), `@docked=${docked}`).toEqual(['25', '50', '75', '100']);
      for (const option of options) {
        const text = option.textContent ?? '';
        expect(text, `@docked=${docked}`).not.toContain('{percent}');
        expect(text, `@docked=${docked}`).toContain(option.value);
        // 벌거벗은 숫자가 아니다 — 단위가 글자로 붙어 있다.
        expect(text.replace(/[0-9]/g, '').trim().length, `@docked=${docked}`).toBeGreaterThan(0);
      }
    }
  });

  it('006 이 그리는 어떤 글자에도 치환자가 남지 않는다 — **두 표면 모두**', () => {
    // 개별 문구를 이름으로 세는 대신 표면 전체를 훑는다. 다음 사람이 치환자를 가진
    // 문구를 하나 더 그려 넣고 채우기를 잊으면 여기서 걸린다.
    for (const docked of [true, false] as const) {
      cleanup();
      render(<Harness docked={docked} />);
      const body = document.body.textContent ?? '';
      expect(body, `@docked=${docked}`).not.toMatch(/\{[a-zA-Z]+\}/);
      expect(body, `@docked=${docked}`).not.toContain('dashboard.canvas.edit');
    }
  });
});

// --- 006 이 더하고 걷어낸 키 (M11) ---------------------------------------

describe('006 의 키가 양쪽 언어에 있고 이름에 점이 없다 (M11)', () => {
  it('더한 여섯이 두 언어에 모두 있고, 걷어낸 여섯은 두 언어에서 사라졌다', () => {
    for (const [name, messages] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      for (const ns of ['edit', 'elements'] as const) {
        const tree = namespaceOf(messages, ns);
        for (const key of ADDED_KEYS[ns]) {
          expect(typeof tree[key], `${name}.${ns}.${key}`).toBe('string');
          expect((tree[key] as string).length, `${name}.${ns}.${key}`).toBeGreaterThan(0);
        }
        for (const key of REMOVED_KEYS[ns]) {
          expect(tree[key], `${name}.${ns}.${key} 는 걷어냈다`).toBeUndefined();
        }
      }
    }
  });

  it('키 **이름 자체**에 점이 없다 — 이름에 점이 든 키는 어떤 조회 경로로도 닿지 않는다', () => {
    // `expect('workspaceZoom').not.toContain('.')` 은 항진 명제다(적어 둔 글자를 다시
    // 읽을 뿐이다). 실제 객체의 키를 훑어야 `"workspace.zoom": ...` 을 적는 순간 걸린다.
    for (const [name, messages] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      for (const ns of ['edit', 'elements'] as const) {
        for (const key of Object.keys(namespaceOf(messages, ns))) {
          expect(key.includes('.'), `${name}.${ns}: 이름에 점이 든 키 "${key}"`).toBe(false);
        }
      }
    }
  });

  it('두 언어가 서로 다른 문구다 — 한쪽을 복사해 두면 번역이 없는 것과 같다', () => {
    for (const ns of ['edit', 'elements'] as const) {
      for (const key of ADDED_KEYS[ns]) {
        // `workspaceZoomOption` 은 `{percent}%` 라는 숫자·기호가 대부분이라 두 언어가
        // 겹칠 여지가 있다 — 그래서 이 단언에서 뺀다. 대신 위에서 치환자를 잰다.
        if (key === 'workspaceZoomOption') continue;
        expect(namespaceOf(ko, ns)[key], `${ns}.${key}`).not.toBe(namespaceOf(en, ns)[key]);
      }
    }
  });
});

// --- 장식과 이름 (M11 · 보조기기) ----------------------------------------

describe('장식은 이름을 갖지 않고, 컨트롤은 진짜 이름을 갖는다 (M11)', () => {
  const DECORATION = [
    'canvas-workspace-grid',
    'canvas-region-scrim',
    'canvas-region-bounds',
  ] as const;

  it('표시 층 셋은 `aria-hidden` 이고 **알릴 글자가 없다** — 두 표면 모두', () => {
    // M10 의 형상 가드는 "`aria-hidden` 이면서 칠하는 층은 포인터를 먹지 않는다" 를
    // 지킨다. 그것은 **포인터**의 성질이다. 여기서 재는 것은 **보조기기**의 성질이다:
    // 그 셋이 이름도 글자도 갖지 않아 스크린 리더가 읽을 것이 없다는 사실.
    for (const docked of [true, false] as const) {
      cleanup();
      render(<Harness docked={docked} />);
      for (const id of DECORATION) {
        const node = screen.getByTestId(id);
        expect(node.getAttribute('aria-hidden'), `${id}@${docked}`).toBe('true');
        expect(node.getAttribute('aria-label'), `${id}@${docked}`).toBeNull();
        expect((node.textContent ?? '').trim(), `${id}@${docked}`).toBe('');
      }
    }
  });

  it('배율 줄은 이름을 가진 컨트롤이라 `aria-hidden` 이 **아니고**, 그 이름이 진짜 문구다', () => {
    render(<Harness docked={false} />);

    const bar = screen.getByTestId('canvas-workspace-zoom-bar');
    expect(bar.getAttribute('aria-hidden')).toBeNull();
    expect(bar.getAttribute('role')).toBe('group');
    // 키가 새어 나오면 이름이 없는 것과 같다.
    expect(bar.getAttribute('aria-label')).toBe(editNamespace(ko).workspaceZoomBar);
    expect(bar.getAttribute('aria-label')).not.toContain('dashboard.');

    // 도움말은 **상시** 문구이고 `aria-describedby` 로 이어진다 — 클릭 팝오버가 아니다
    // (팝오버는 표면 컨테이너의 `overflow-hidden` 에 잘린다).
    const input = within(bar).getByTestId('canvas-workspace-zoom');
    const hint = screen.getByTestId('canvas-workspace-zoom-hint');
    expect(input.getAttribute('aria-describedby')).toBe(hint.id);
    expect(hint.className).toContain('sr-only');
    expect(hint.textContent).toBe(editNamespace(ko).workspaceZoomHint);
    expect(input.getAttribute('aria-label')).toBe(editNamespace(ko).workspaceZoom);
    expect(input.getAttribute('title')).toBe(editNamespace(ko).workspaceZoom);
  });
});

// --- 치환의 형상 (M11) ----------------------------------------------------

describe('치환은 `replaceAll` 이다 — `replace` 는 둘째 치환자를 화면에 남긴다 (M11)', () => {
  it('같은 치환자가 둘인 문구에서 두 함수의 결과가 갈린다 — 그 산술을 수로 적는다', () => {
    // 이 저장소는 그 형상을 **이미 갖고 있다**: `gridStepPartial` 의 `{step}` 이 둘이라
    // 도크가 `replaceAll` 을 쓴다. 배율 문구의 치환자는 오늘 하나뿐이라 두 함수가 같은
    // 답을 내지만, 그것은 **문구의 성질**이지 코드의 성질이 아니다 — 번역을 손보다
    // 치환자를 하나 더 넣는 순간 침묵이 깨진다.
    const twice = '{p} 에서 {p} 로';
    expect(twice.replace('{p}', '1')).toContain('{p}');
    expect(twice.replaceAll('{p}', '1')).not.toContain('{p}');
    // 그리고 실제로 둘인 문구가 있다.
    for (const messages of [ko, en]) {
      const partial = editNamespace(messages).gridStepPartial as string;
      expect(partial.match(/\{step\}/g)?.length).toBeGreaterThanOrEqual(2);
    }
  });

  it('006 이 치환하는 자리는 `replaceAll` 로 적혀 있다 (형상 가드)', async () => {
    // **겨누는 범위는 006 이 소유한 파일 하나다.** `CanvasEditDock.tsx` 에는 002 가
    // 0.11.0 에 넣은 `gridStepOption` 의 `.replace('{step}', …)` 가 아직 남아 있는데,
    // 그 문구도 치환자가 하나뿐이라 오늘 결과가 같다 — 같은 잠재 함정이되 **006 이 더한
    // 문구가 아니므로** 이 회차의 쓸이 범위 밖이다(M11 은 M1~M10 이 더한 문구를 쓴다).
    // 범위를 넓혀 남의 파일을 고치는 대신, 그 사실을 여기 이름으로 적어 남긴다.
    const { readFileSync } = await import('node:fs');
    const { join } = await import('node:path');
    const source = readFileSync(join(__dirname, 'CanvasWorkspaceZoomField.tsx'), 'utf-8');
    // 치환자를 문자열로 넘기는 `replace(` 가 한 자리도 없다. `replaceAll` 은 걸리지
    // 않는다 — `replace(` 뒤에 곧바로 여는 따옴표와 `{` 가 오는 형상만 겨눈다.
    expect(source.match(/\.replace\(\s*'\{/g)).toBeNull();
    expect(source).toMatch(/\.replaceAll\(\s*\n?\s*'\{percent\}'/);
  });
});
