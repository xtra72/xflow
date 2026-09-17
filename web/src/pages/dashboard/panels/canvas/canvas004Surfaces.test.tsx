// 묶기 · 풀기의 배선과 **두 표면**의 컨트롤 (SPEC-CANVAS-004 M6 · AC-11 · AC-12 · AC-13 ·
// AC-E12 · 불변식 G7 · 시험 규율 E-J).
//
// ## 이 파일이 겨누는 구멍
//
// **한 표면만 재는 시험은 아무것도 증명하지 않는다**(E-J). 006 이 정확히 그 함정으로 결함
// 하나를 배달했다 — 표시 층 셋은 조건 없이 그려지는데 컨트롤 전부가 `dockHost !== null`
// 뒤에 있었고, 그 사실을 **전량 green 인 스위트가 잡지 못한 이유는 두 조건이 같은 자리에서
// 비교된 적이 없어서다.** 도크 하네스에서만 재면 대시보드에 단추가 없어도 초록이다.
//
// 그래서 이 파일의 거의 모든 시험이 `for (const docked of [true, false])` 한 줄을 두른다.
// 그리고 **관계**를 잰다: 그룹이 그려지고 골라지는 모든 표면에서 그룹/그룹해제 컨트롤이
// 적어도 하나 DOM 에 있는가.
//
// **"없음" 을 단언할 때는 그 없음이 옳다는 근거를 함께 적는다**(006 시험 규율 D9). 대시보드
// 표면에 팔레트 · 정렬 · 순서가 없는 것은 **여전히 옳다** — 그 넷을 대시보드에 주는 안은
// 006 에서 사용자에게 제시되었고 고르지 않았다. 그룹이 없는 것은 옳지 않다 — 그룹은 두
// 표면 모두에서 **그려지고 선택되기** 때문이다(가정 A21 · 불변식 I23).
//
// ## 고정 입력이 지키는 것 (시험 규율 E-A · E-B · E-C · E-D · E-L)
//
// 묶을 셋의 상자는 **서로 다르고 겹치지 않으며**, 하나(`t`)는 합집합 상자의 가장자리에
// **닿지 않는다** — 가장자리에 닿는 부품만 있으면 로컬 좌표가 전부 `0` 또는 `EXTENT` 라
// 축척 산술이 관측되지 않는다(E-A). 합집합 상자는 `{ x: 73, y: 41, w: 317, h: 181 }` 이라
// 원점이 0 이 아니고(E-D) 정사각이 아니며(E-B) 두 변 모두 `GROUP_LOCAL_EXTENT` 를
// 나누어떨어뜨리지 않는다(E-C). 셋 가운데 하나는 **문구 요소**다(E-L).
//
// 산술 자체는 여기서 재지 않는다 — `group/groupOps.test.ts` 의 51 시험이 그 몫이고, 이
// 파일은 **화면이 그 함수를 부르는가**를 잰다. 다만 고정 입력이 항등이면 "부르지 않는
// 구현" 도 통과하므로, 위 넷을 여기서도 갖춘다.
//
// @spec SPEC-CANVAS-004 REQ-07 · REQ-08

import { useState } from 'react';

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { DEFAULT_CANVAS_SIZE, type CanvasElement, type Geometry } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import { GROUP_LOCAL_EXTENT, isGroup, type CanvasNode, type GroupElement } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

/**
 * 노드의 기하. 연결선에는 기하가 없으므로(SPEC-CANVAS-011 M4) 좁혀 읽는다 — 이 시험의
 * 장면에는 연결선이 오지 않으며, 그때는 `undefined` 라 단언이 조용히 통과하지 않는다.
 */
function geometryOf(node: CanvasNode): Geometry | undefined {
  return isConnector(node) ? undefined : node.geometry;
}

import { useScratchpadStore } from './scratchpad/scratchpadStore';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 160 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

// --- 고정 입력 -----------------------------------------------------------

/** 왼쪽 위 — 합집합 상자의 **왼쪽 변과 위 변**에 닿는다. */
const A: CanvasElement = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 73, y: 41, w: 120, h: 60 },
  style: {},
};

/** 오른쪽 아래 — **오른쪽 변과 아래 변**에 닿는다. A 와 겹치지 않는다. */
const B: CanvasElement = {
  id: 'b',
  kind: 'ellipse',
  geometry: { x: 250, y: 150, w: 140, h: 72 },
  style: {},
};

/**
 * 안쪽에 떠 있는 **문구** 부품 — 어느 변에도 닿지 않는다(E-A) 그리고 문구다(E-L).
 *
 * 폭이 없는 것이 뜻이 있다: `elementsBounds` 가 문구를 넓이 0 으로 보므로(008 이 정한 규칙)
 * **묶기가 폰트에 따라 다른 상자를 내지 않는다.**
 */
const T: CanvasElement = {
  id: 't',
  kind: 'text',
  geometry: { x: 200, y: 120 },
  style: {},
  text: 'label',
};

/** 묶지 않고 남는 것 — 묶기가 남의 기하를 건드리지 않았음을 재기 위한 대조군이다. */
const OUT: CanvasElement = {
  id: 'out',
  kind: 'rect',
  geometry: { x: 420, y: 300, w: 40, h: 30 },
  style: {},
};

/** 합집합 상자 — 원점 ≠ 0 · 비정사각 · 10000 을 나누어떨어뜨리지 않는다. */
const UNION = { x: 73, y: 41, w: 317, h: 181 } as const;

/** 규칙 표 두 행을 든 그룹. 풀면 그 둘이 **버려진다**(가정 A19). */
function groupWithRules(id = 'grp-1'): GroupElement {
  return {
    id,
    kind: 'group',
    geometry: { ...UNION },
    parts: [
      { id: 'p-a', kind: 'rect', geometry: { x: 0, y: 0, w: 3785, h: 3315 }, style: {} },
      { id: 'p-t', kind: 'text', geometry: { x: 4006, y: 4365 }, style: {}, text: 'label' },
    ],
    binding: { series: 's1', agg: 'last' },
    rules: [
      { op: 'nodata', value: 0, patch: { fill: '#111111' } },
      { op: 'gt', value: 10, patch: { fill: '#222222' } },
    ],
  };
}

/** 규칙이 **없는** 그룹 — 풀기가 묻지 않아야 하는 쪽이다. */
function groupWithoutRules(id = 'grp-2'): GroupElement {
  const g = groupWithRules(id);
  delete g.rules;
  return g;
}

// --- 하네스 -------------------------------------------------------------

let live: readonly CanvasNode[] = [];

/**
 * 두 표면을 **같은 하네스**로 세운다 — `docked` 하나만 갈아 끼운다(E-J).
 *
 * 선택은 컨트롤이 아니라 **하네스의 단추**로 세운다. 포인터로 고르는 길은 M4 의 히트
 * 시험이 이미 소유하고 있고, 여기서 재려는 것은 "무엇이 골라졌을 때 어떤 단추가 사는가"
 * 이지 "어디를 누르면 골라지는가" 가 아니다 — 섞으면 히트가 어긋났을 때 이 파일이 함께
 * 빨개져 원인을 가린다.
 */
function Harness({
  docked,
  initial,
  select = [],
  select2 = [],
}: {
  docked: boolean;
  initial: readonly CanvasNode[];
  select?: readonly string[];
  /** **같은 마운트 안에서** 선택을 한 번 더 갈아 끼우는 길(다시 마운트하면 못 재는 것이 있다). */
  select2?: readonly string[];
}) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  live = elements;
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
    <CanvasEditSelectionContext value={state}>
      <button type="button" data-testid="pick" onClick={() => state.setSelection(new Set(select))}>
        pick
      </button>
      <button type="button" data-testid="pick2" onClick={() => state.setSelection(new Set(select2))}>
        pick2
      </button>
      <span data-testid="selection">{[...state.selection].sort().join(',')}</span>
      {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
    </CanvasEditSelectionContext>
  );
}

/** 고정 입력을 세우고 **선택을 실제로 세웠음을 단언한다** — 빈 선택은 초록의 거짓 근거다. */
function setup(opts: {
  docked: boolean;
  initial: readonly CanvasNode[];
  select?: readonly string[];
  select2?: readonly string[];
}): void {
  live = [];
  render(
    <Harness
      docked={opts.docked}
      initial={opts.initial}
      select={opts.select}
      select2={opts.select2}
    />,
  );
  fireEvent.click(screen.getByTestId('pick'));
  const want = [...(opts.select ?? [])].sort().join(',');
  expect(screen.getByTestId('selection').textContent, 'precondition: 선택').toBe(want);
}

function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

function send(type: string, x: number, y: number, init: MouseEventInit = {}): void {
  fireEvent(screen.getByTestId('canvas-edit-overlay'), pointer(type, x, y, init));
}

/** 오버레이의 화면 상자를 심는다(jsdom 은 레이아웃을 하지 않는다). 축척 1 이 아니다. */
function stubOverlayRect(width = PROJ.stage.width, height = PROJ.stage.height): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width,
    height,
    right: width,
    bottom: height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

/** 이동은 프레임당 한 번으로 모인다 — 프레임을 기다려야 실제와 같은 시점이다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

function groupButton(): HTMLButtonElement {
  return screen.getByTestId('canvas-group-create') as HTMLButtonElement;
}

function ungroupButton(): HTMLButtonElement {
  return screen.getByTestId('canvas-group-ungroup') as HTMLButtonElement;
}

const SURFACES = [true, false] as const;

/** 표면 이름 — 실패 메시지가 어느 쪽인지 말하게 한다. */
function surfaceName(docked: boolean): string {
  return docked ? '설정(도크)' : '대시보드(줄)';
}

// --- ① 두 표면 모두에 컨트롤이 선다 (AC-E12 · 불변식 G7) -------------------

describe('그룹 컨트롤이 두 표면 모두에 선다 (AC-E12 · 불변식 I23 · 시험 규율 E-J)', () => {
  it('①덮임 — 어느 표면에서도 그룹·그룹해제 단추가 **정확히 하나씩** 있고 누를 수 있다', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [A, B, T, OUT], select: ['a', 'b'] });

      // "있다" 가 아니라 "하나뿐이다" 를 잰다 — 두 자리에 동시에 서는 것이 I24 위반이다.
      expect(screen.getAllByTestId('canvas-group-create'), surfaceName(docked)).toHaveLength(1);
      expect(screen.getAllByTestId('canvas-group-ungroup'), surfaceName(docked)).toHaveLength(1);
      // **꺼져 있으면 있으나 마나다.** 둘을 골랐으므로 묶기는 켜져 있어야 한다.
      expect(groupButton().disabled, surfaceName(docked)).toBe(false);
      expect(groupButton().tagName, surfaceName(docked)).toBe('BUTTON');
    }
  });

  it('②자리 — 도크 표면에서는 **띠** 안에, 대시보드 표면에서는 떠 있는 줄 안에 산다', () => {
    // 담김 관계를 잰다. "어딘가에 있다" 만 재면 띠 밖 · 줄 밖에 떠 있어도 초록이다.
    //
    // **그릇이 바뀌었다**(2026-09-16 — 도구 띠). 그룹 절은 도크에서 미리보기 제목 아래
    // 가로 띠로 갔다 — 묶기는 선택 위에서 도는 연산이라 정렬 · 순서와 한 부류이고, 그
    // 셋이 함께 옮겨졌기 때문이다. 재는 것(두 표면에 하나씩, 제 그릇 안에)은 그대로다.
    cleanup();
    setup({ docked: true, initial: [A, B], select: ['a', 'b'] });
    expect(screen.getByTestId('canvas-toolbar-panel').contains(groupButton())).toBe(true);
    expect(screen.queryByTestId('canvas-workspace-zoom-bar')).toBeNull();

    cleanup();
    setup({ docked: false, initial: [A, B], select: ['a', 'b'] });
    expect(screen.getByTestId('canvas-workspace-zoom-bar').contains(groupButton())).toBe(true);
    // 도크가 대시보드로 온 것이 아니다 — 온 것은 그룹 컨트롤과 배율뿐이다.
    expect(screen.queryByTestId('canvas-dock-panel')).toBeNull();
  });

  it('③관계 — 대시보드에 팔레트·정렬·순서가 **없는 것은 옳고**, 그룹이 없는 것은 옳지 않다', () => {
    // 006 이 사용자에게 제시하고 고르지 않은 그 넷은 여전히 대시보드에 없다. 그 없음의
    // 근거는 "고르지 않았다" 이지 "닿지 않아도 된다" 가 아니다 — 그리고 그룹은 다르다:
    // 그룹은 두 표면 모두에서 **그려지고 선택되므로** 손잡이가 닿아야 한다(I23).
    setup({ docked: false, initial: [A, B], select: ['a', 'b'] });
    expect(screen.queryByTestId('canvas-palette-add-rect')).toBeNull();
    expect(screen.queryByTestId('canvas-align-left')).toBeNull();
    expect(screen.queryByTestId('canvas-order-front')).toBeNull();
    // 그런데 그룹은 있다.
    expect(screen.queryByTestId('canvas-group-create')).not.toBeNull();
  });

  it('④선택 윤곽 — 그룹은 두 표면 모두에서 **골라지고 윤곽이 선다**(손잡이가 닿아야 하는 근거)', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [groupWithRules(), OUT], select: ['grp-1'] });
      expect(screen.queryByTestId('canvas-selection-grp-1'), surfaceName(docked)).not.toBeNull();
      // 8핸들이 **그룹 상자**에 선다(REQ-08). 부품에는 서지 않는다(가정 A18).
      expect(screen.queryByTestId('canvas-handle-nw'), surfaceName(docked)).not.toBeNull();
      expect(screen.queryByTestId('canvas-handle-p-a'), surfaceName(docked)).toBeNull();
    }
  });
});

// --- ② I23 의 표에 줄이 필요한가 -----------------------------------------

/**
 * 이 노드나 그 자손이 **무언가를 칠하는가**. 006 M10 의 판정을 그대로 옮겼다 — 같은 질문에
 * 다른 자를 대면 두 시험이 다른 답을 내고, 그때 어느 쪽이 참인지 알 길이 없다.
 */
function paintsSomething(el: HTMLElement): boolean {
  const nodes: HTMLElement[] = [el, ...el.querySelectorAll<HTMLElement>('*')];
  return nodes.some(
    (n) =>
      n.style.backgroundColor !== '' ||
      n.style.backgroundImage !== '' ||
      n.style.borderColor !== '' ||
      n.style.boxShadow !== '' ||
      /(?:^|\s)(?:border|bg-)/.test(n.className),
  );
}

/** `aria-hidden` 이면서 칠하는 오버레이 **직속 자식**의 이름들(선택 그림자는 뺀다). */
function paintingLayerIds(): string[] {
  const root = screen.getByTestId('canvas-edit-overlay');
  return [...root.children]
    .filter(
      (child): child is HTMLElement =>
        child instanceof HTMLElement &&
        child.getAttribute('aria-hidden') === 'true' &&
        paintsSomething(child),
    )
    .map((child) => child.dataset.testid ?? '(이름 없음)')
    .filter((id) => !id.startsWith('canvas-selection-'))
    .sort();
}

describe('I23 의 층→컨트롤 표에 줄이 필요한가 — **재서 답한다**', () => {
  it('그룹이 있든 없든 칠하는 `aria-hidden` 직속 자식 집합이 **같다** — 그래서 새 줄이 없다', () => {
    // 이 파일이 가장 조용히 틀릴 수 있는 자리다. "그룹은 캔버스에 칠해지니 표와 무관하다"
    // 는 **추론**이고, 006 이 물린 것은 정확히 그런 추론이었다. 그래서 유추 대신 **잰다**:
    // 그룹이 DOM 에 새 칠하는 층을 더하는가. 더하지 않으면 표는 그대로다.
    //
    // 그룹의 그림은 `<canvas>` 픽셀이고, 그룹이 더하는 DOM 은 (a) 선택 윤곽(이미 표에서
    // 면제된 `canvas-selection-*`) 과 (b) 이름을 가진 컨트롤(`aria-hidden` 이 아니다)뿐이다.
    // 아래 단언이 그 둘을 **관측으로** 확인한다.
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [A, B, OUT], select: [] });
      const withoutGroup = paintingLayerIds();

      cleanup();
      setup({ docked, initial: [groupWithRules(), OUT], select: ['grp-1'] });
      const withGroup = paintingLayerIds();

      expect(withGroup, surfaceName(docked)).toEqual(withoutGroup);
      // 켜져 있음을 먼저 단언한다 — 둘 다 빈 배열이면 위 비교가 아무것도 재지 않는다.
      expect(withGroup.length, `${surfaceName(docked)}: 칠하는 층이 0 이다`).toBeGreaterThan(0);
    }
  });

  it('그룹 컨트롤은 `aria-hidden` 이 아니다 — 이름을 가진 컨트롤이라 표의 대상이 아니다', () => {
    setup({ docked: false, initial: [A, B], select: ['a', 'b'] });
    expect(groupButton().getAttribute('aria-hidden')).toBeNull();
    expect(groupButton().getAttribute('aria-label')).not.toBe('');
  });
});

// --- ③ 묶기 (AC-11) ------------------------------------------------------

describe('고른 것들을 묶는다 (AC-11 · 두 표면 모두)', () => {
  it('셋을 묶으면 그룹 하나가 **마지막 것의 자리**에 서고 남은 요소는 한 자리도 바뀌지 않는다', () => {
    for (const docked of SURFACES) {
      cleanup();
      // 배열 순서: a · out · b · t. 고른 셋(a·b·t) 가운데 **마지막**은 `t`(index 3).
      setup({ docked, initial: [A, OUT, B, T], select: ['a', 'b', 't'] });
      fireEvent.click(groupButton());

      expect(live.map((n) => n.kind), surfaceName(docked)).toEqual(['rect', 'group']);
      const group = live[1] as GroupElement;
      expect(group.geometry, surfaceName(docked)).toEqual(UNION);
      expect(group.parts.map((p) => p.id), surfaceName(docked)).toEqual(['a', 'b', 't']);

      // 남은 최상위 요소의 기하가 그대로다 — 묶기가 남의 좌표를 건드리지 않았다.
      expect(live[0], surfaceName(docked)).toEqual(OUT);

      // 가장자리에 닿지 않는 부품의 로컬 좌표가 **0 도 EXTENT 도 아니다**(E-A).
      // 이 한 줄이 "원점 빼기와 축척 곱하기를 둘 다 잊은 구현" 을 떨어뜨린다.
      const text = group.parts[2]!;
      expect(text.kind, surfaceName(docked)).toBe('text');
      const local = text.geometry as { x: number; y: number };
      for (const v of [local.x, local.y]) {
        expect(v, surfaceName(docked)).toBeGreaterThan(0);
        expect(v, surfaceName(docked)).toBeLessThan(GROUP_LOCAL_EXTENT);
      }

      // 선택은 **새 그룹 하나**다 — 놓은 것을 바로 끌 수 있어야 한다.
      expect(screen.getByTestId('selection').textContent, surfaceName(docked)).toBe(group.id);
    }
  });
});

// --- ④ 묶기의 거절 둘 (AC-12) -------------------------------------------

describe('묶기의 거절 둘 (AC-12 · 두 표면 모두)', () => {
  it('하나만 골랐으면 단추가 **꺼져 있다** — 눌러도 아무 일이 없는 단추를 두지 않는다', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [A, B], select: ['a'] });
      expect(groupButton().disabled, surfaceName(docked)).toBe(true);
      // 꺼진 단추는 안내도 내지 않는다 — 말할 거절이 아직 없다.
      expect(screen.queryByTestId('canvas-group-refusal'), surfaceName(docked)).toBeNull();
    }
  });

  it('그룹이 섞여 있으면 **거절하고 화면이 이유를 말하며** 배열이 한 바이트도 바뀌지 않는다', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [groupWithRules(), A, B], select: ['grp-1', 'a'] });
      const before = live;
      fireEvent.click(groupButton());

      // **같은 참조**다 — "값이 같다" 보다 강한 단언이며, 헛된 렌더도 없었다는 뜻이다.
      expect(live, surfaceName(docked)).toBe(before);
      const notice = screen.getByTestId('canvas-group-refusal');
      expect(notice.textContent, surfaceName(docked)).toBe(
        'dashboard.canvas.edit.groupRefusalNested',
      );
      // 그룹 안에 그룹이 만들어지지 않았다.
      expect(live.filter(isGroup)[0]!.parts.some(isGroup as never), surfaceName(docked)).toBe(false);
    }
  });

  it('거절 안내는 **선택이 바뀌면 걷힌다** — 지난 선택에 대한 말이 지금 선택을 두고 하는 말로 읽히지 않는다', () => {
    // **같은 마운트 안에서** 갈아 끼워야 한다. 다시 마운트해 재면 상태가 어차피 새로
    // 태어나므로, 안내를 거두는 코드를 통째로 지워도 초록이다 — 그 형상이 곧 죽은 가드다.
    setup({ docked: true, initial: [groupWithRules(), A, B], select: ['grp-1', 'a'], select2: ['a', 'b'] });
    fireEvent.click(groupButton());
    expect(screen.queryByTestId('canvas-group-refusal')).not.toBeNull();

    fireEvent.click(screen.getByTestId('pick2'));
    expect(screen.getByTestId('selection').textContent).toBe('a,b');
    expect(screen.queryByTestId('canvas-group-refusal')).toBeNull();
  });

  it('선택에 **배열에 없는 id** 가 남아 있으면 묶기는 `tooFew` 로 거절한다 (죽은 갈래가 아니다)', () => {
    // 단추의 활성 조건은 `selection.size >= 2` 이고 `groupNodes` 의 거절 조건은 **배열에서
    // 실제로 찾은 것**을 센다. 목록 편집기에서 지운 요소의 id 가 선택에 남는 일이 실제로
    // 있으므로(오버레이가 그 사실을 제 주석에 적었다) 이 갈래는 닿는다.
    setup({ docked: true, initial: [A, B], select: ['a', 'ghost'] });
    expect(groupButton().disabled).toBe(false);
    const before = live;
    fireEvent.click(groupButton());
    expect(live).toBe(before);
    expect(screen.getByTestId('canvas-group-refusal').textContent).toBe(
      'dashboard.canvas.edit.groupRefusalTooFew',
    );
  });
});

// --- ⑤ 풀기와 그 안내 (AC-13 · REQ-07) ----------------------------------

describe('그룹을 푼다 — 규칙을 잃는다는 사실을 **잃기 전에** 말한다 (AC-13 · 두 표면 모두)', () => {
  it('규칙이 있으면 먼저 묻고, 확인 전에는 배열이 그대로다', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [groupWithRules(), OUT], select: ['grp-1'] });
      const before = live;

      fireEvent.click(ungroupButton());
      const ask = screen.getByTestId('canvas-group-ungroup-ask');
      // 문구가 **버려지는 행의 수**를 나른다 — 판정은 M5 의 함수 하나가 소유한다.
      expect(ask.textContent, surfaceName(docked)).toContain('groupUngroupAsk');
      // 아직 아무것도 풀리지 않았다.
      expect(live, surfaceName(docked)).toBe(before);
      expect(live.filter(isGroup), surfaceName(docked)).toHaveLength(1);

      fireEvent.click(screen.getByTestId('canvas-group-ungroup-yes'));
      expect(live.map((n) => n.kind), surfaceName(docked)).toEqual(['rect', 'text', 'rect']);
      // 풀린 부품 **전부**가 선택으로 남는다.
      expect(screen.getByTestId('selection').textContent, surfaceName(docked)).not.toBe('');
      expect(screen.getByTestId('selection').textContent!.split(','), surfaceName(docked)).toHaveLength(2);
      expect(screen.queryByTestId('canvas-group-ungroup-ask'), surfaceName(docked)).toBeNull();
    }
  });

  it('"풀지 않기" 를 고르면 안내만 걷히고 그룹은 그대로다', () => {
    setup({ docked: false, initial: [groupWithRules(), OUT], select: ['grp-1'] });
    const before = live;
    fireEvent.click(ungroupButton());
    fireEvent.click(screen.getByTestId('canvas-group-ungroup-no'));
    expect(screen.queryByTestId('canvas-group-ungroup-ask')).toBeNull();
    expect(live).toBe(before);
  });

  it('규칙이 **없으면 묻지 않는다** — 잃을 것이 없는 풀기에 걸음을 붙이지 않는다 (REQ-07)', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [groupWithoutRules(), OUT], select: ['grp-2'] });
      fireEvent.click(ungroupButton());
      // 안내 없이 **곧바로** 풀렸다. 이 한 쌍이 "언제나 묻는 구현" 과 "안내가 안 뜨는
      // 구현" 을 동시에 떨어뜨린다.
      expect(screen.queryByTestId('canvas-group-ungroup-ask'), surfaceName(docked)).toBeNull();
      expect(live.filter(isGroup), surfaceName(docked)).toHaveLength(0);
      expect(live.map((n) => n.kind), surfaceName(docked)).toEqual(['rect', 'text', 'rect']);
    }
  });

  it('풀 그룹이 없으면 단추가 꺼지고, 둘 이상 골라도 꺼진다 (무엇을 푸는가에 답이 없다)', () => {
    for (const docked of SURFACES) {
      cleanup();
      setup({ docked, initial: [A, B], select: ['a', 'b'] });
      expect(ungroupButton().disabled, `${surfaceName(docked)}: 그룹이 아닌 둘`).toBe(true);

      cleanup();
      setup({ docked, initial: [groupWithRules(), groupWithRules('grp-9')], select: ['grp-1', 'grp-9'] });
      expect(ungroupButton().disabled, `${surfaceName(docked)}: 그룹 둘`).toBe(true);

      cleanup();
      setup({ docked, initial: [groupWithRules(), OUT], select: ['grp-1'] });
      expect(ungroupButton().disabled, `${surfaceName(docked)}: 그룹 하나`).toBe(false);
    }
  });

  it('묻는 중에 풀 대상이 사라지면 확인이 걷힌다 — 묻지 않은 것이 풀리지 않는다', () => {
    // 여기도 **같은 마운트 안에서** 갈아 끼운다. 다시 마운트하면 확인 상태가 어차피 새로
    // 태어나 가드가 아무것도 지키지 않는다.
    setup({ docked: true, initial: [groupWithRules(), OUT], select: ['grp-1'], select2: [] });
    fireEvent.click(ungroupButton());
    expect(screen.queryByTestId('canvas-group-ungroup-ask')).not.toBeNull();

    fireEvent.click(screen.getByTestId('pick2'));
    expect(screen.getByTestId('selection').textContent).toBe('');
    expect(screen.queryByTestId('canvas-group-ungroup-ask')).toBeNull();
    expect(ungroupButton().disabled).toBe(true);
  });
});

// --- ⑦ 서랍의 저장 형상은 한 바이트도 넓어지지 않는다 (가정 A20) ----------

describe('그룹은 서랍에 들어가지 않는다 — 저장 형상은 `CanvasElement[]` 그대로다', () => {
  it('그룹과 요소를 함께 골라 저장하면 **요소만** 들어간다', () => {
    // 가드를 지우고 단언(`as CanvasElement[]`)으로 뚫으면 타입은 조용하고 그룹이 서랍에
    // 눌러앉는다 — 그 항목은 `localStorage` 로 영속되어 다음 세션까지 살아남는다.
    useScratchpadStore.setState({ entries: [], notice: null });
    setup({ docked: true, initial: [groupWithRules(), A], select: ['grp-1', 'a'] });
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    const entries = useScratchpadStore.getState().entries;
    expect(entries).toHaveLength(1);
    expect(entries[0]!.elements.map((el) => el.id)).toEqual(['a']);
    expect(entries[0]!.elements.some((el) => (el as { kind: string }).kind === 'group')).toBe(false);
  });

  it('그룹만 골라 저장하면 서랍이 **이미 가진** 빈 선택 안내가 뜬다 (새 문구를 만들지 않는다)', () => {
    useScratchpadStore.setState({ entries: [], notice: null });
    setup({ docked: true, initial: [groupWithRules(), A], select: ['grp-1'] });
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    expect(useScratchpadStore.getState().entries).toHaveLength(0);
    expect(useScratchpadStore.getState().notice?.status).toBe('empty');
  });
});

// --- ⑧ 그룹 상자는 손잡이로 실제로 잡힌다 (REQ-08) ------------------------

describe('그룹 8핸들이 보이기만 하는 것이 아니라 **잡힌다**', () => {
  it('se 손잡이를 끌면 그룹 상자가 커지고 **부품의 저장 좌표는 한 자리도 바뀌지 않는다**', async () => {
    // `handleDragState` 의 상자 갈래에서 그룹을 빠뜨리면 손잡이는 서고 자리도 잡히는데
    // **드래그가 시작되지 않는다** — 보이는데 잡히지 않는 그룹이 되고, 그 증상은 화면에서만
    // 드러난다. 위 ④(손잡이가 선다)는 그 상태에서도 초록이므로 이 시험이 따로 서 있다.
    setup({ docked: false, initial: [groupWithRules(), OUT], select: ['grp-1'] });
    stubOverlayRect();
    const before = live.filter(isGroup)[0]!;

    const handle = screen.getByTestId('canvas-handle-se');
    const from = {
      x: Number.parseFloat(handle.style.left),
      y: Number.parseFloat(handle.style.top),
    };
    fireEvent(handle, pointer('pointerdown', from.x, from.y));
    send('pointermove', from.x + 20, from.y + 16);
    await nextFrame();

    const after = live.filter(isGroup)[0]!;
    expect(after.geometry).not.toEqual(before.geometry);
    expect(after.geometry.w).toBeGreaterThan(before.geometry.w);
    expect(after.geometry.h).toBeGreaterThan(before.geometry.h);
    // 가정 A17 — 그룹 크기 조절은 부품의 저장 좌표를 바꾸지 않는다(008 J2 와 같은 자리).
    expect(after.parts).toEqual(before.parts);
  });
});

// --- ⑥ 묶기 → 풀기 왕복이 화면을 지난다 ----------------------------------

describe('묶었다가 곧바로 푸는 몸짓 (AC-14 의 화면 쪽 — 산술은 groupOps 가 잰다)', () => {
  it('상자 변이 작은 묶음은 왕복 뒤 기하가 **정수까지 원본과 같다**', () => {
    setup({ docked: true, initial: [A, B, T, OUT], select: ['a', 'b', 't'] });
    fireEvent.click(groupButton());
    const groupId = live.filter(isGroup)[0]!.id;

    // 방금 만든 그룹이 선택으로 서 있으므로 그대로 풀 수 있다(규칙이 없으니 묻지 않는다).
    expect(screen.getByTestId('selection').textContent).toBe(groupId);
    fireEvent.click(ungroupButton());

    const byKind = new Map(live.map((n) => [n.kind, n] as const));
    expect(live).toHaveLength(4);
    expect(geometryOf(byKind.get('ellipse')!)).toEqual(B.geometry);
    expect(geometryOf(byKind.get('text')!)).toEqual(T.geometry);
  });
});
