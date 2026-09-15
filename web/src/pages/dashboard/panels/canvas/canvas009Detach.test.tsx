// 부품 분리 단추 — 캔버스에서 고른 부품 하나가 최상위로 올라온다 (SPEC-CANVAS-009 M6).
//
// ## 이 파일이 겨누는 이음매
//
// `groupOps.detach.test.ts` 는 순수 함수를 잰다. 여기서 재는 것은 **그 함수에 닿기까지의
// 길**이다 — 캔버스에서 부품을 고르고, 단추가 켜지고, 확인을 지나고, 배열이 바뀐다.
// 순수 함수가 100% 여도 그 사이가 끊겨 있으면 화면에서는 아무 일도 일어나지 않는다.
//
// ## 활성 조건이 서로 배타적이라는 것
//
// 그룹 해제와 부품 분리는 같은 덩어리를 다루고 범위만 다르다. 한 선택에서 둘이 함께
// 켜지면 사용자는 무엇이 사라지는지 예측할 수 없다 — 부품과 그룹이 동시에 선택되지
// 않는다는 REQ-01-a 가 그 배타성을 형상으로 보장하며, 이 파일이 그것을 화면에서 잰다.
//
// @spec SPEC-CANVAS-009 REQ-05 · AC-24 ~ AC-25 · AC-31 ~ AC-32

import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize, Geometry, RuleRow } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import { isGroup, type CanvasNode, type GroupElement } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

/**
 * 노드의 기하. 연결선에는 기하가 없으므로(SPEC-CANVAS-011 M4) 좁혀 읽는다 — 이 시험의
 * 장면에는 연결선이 오지 않으며, 그때는 `undefined` 라 단언이 조용히 통과하지 않는다.
 */
function geometryOf(node: CanvasNode): Geometry | undefined {
  return isConnector(node) ? undefined : node.geometry;
}


// --- 고정 입력 -------------------------------------------------------------
//
// 좌표 산술은 `canvas009PartSelection.test.tsx` 머리말과 같다(스테이지 200×100 ·
// 캔버스 500×400 → 축척 0.4 · 0.25).

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

const NODATA_ROW: RuleRow = { op: 'nodata', value: 0, patch: {} };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

/** 부품 셋 — 하나를 빼도 그룹이 살아남는다(둘 이하면 `detachPart` 가 통째로 푼다). */
function group(over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 50, y: 40, w: 100, h: 80 },
    parts: [
      rect('body', { x: 1000, y: 1000, w: 4000, h: 4000 }),
      rect('head', { x: 5000, y: 5000, w: 4000, h: 4000 }),
      rect('tail', { x: 100, y: 8000, w: 500, h: 500 }),
    ],
    ...over,
  };
}

function sibling(): CanvasElement {
  return rect('el-3', { x: 400, y: 320, w: 100, h: 60 });
}

const AT = {
  body: { x: 32, y: 16 },
  sibling: { x: 180, y: 88 },
  aboveGroup: { x: 5, y: 5 },
  empty: { x: 5, y: 95 },
} as const;

// --- 하네스 ---------------------------------------------------------------

function Harness({
  elements,
  onElementsChange,
}: {
  elements: readonly CanvasNode[];
  onElementsChange: (next: CanvasNode[]) => void;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <CanvasEditOverlay
        enabled
        elements={elements}
        projection={PROJ}
        textWidths={{}}
        onElementsChange={onElementsChange}
      />
    </CanvasEditSelectionContext>
  );
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

function setup(elements: readonly CanvasNode[] = [group(), sibling()]) {
  const emit = vi.fn();
  render(<Harness elements={elements} onElementsChange={emit} />);
  vi.spyOn(overlayRoot(), 'getBoundingClientRect').mockReturnValue({
    left: 0, top: 0, width: STAGE.width, height: STAGE.height,
    right: STAGE.width, bottom: STAGE.height, x: 0, y: 0, toJSON: () => ({}),
  } as DOMRect);
  return emit;
}

function at(p: { x: number; y: number }): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true };
}

function click(p: { x: number; y: number }): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', at(p)));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/**
 * 그룹 안으로 들어간다 — 부품을 고르는 몸짓은 **더블클릭**이다(SPEC-CANVAS-009 0.3.0).
 *
 * 단일 클릭은 그룹을 고르므로, 분리 단추를 켜려면 먼저 안으로 들어가야 한다.
 */
function enterPart(p: { x: number; y: number }): void {
  click(p);
  click(p);
}

function marqueeSelect(from: { x: number; y: number }, to: { x: number; y: number }): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', at(from)));
  fireEvent(overlayRoot(), new MouseEvent('pointermove', at(to)));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(to)));
}

function detachButton(): HTMLButtonElement {
  return screen.getByTestId('canvas-group-detach') as HTMLButtonElement;
}

function ungroupButton(): HTMLButtonElement {
  return screen.getByTestId('canvas-group-ungroup') as HTMLButtonElement;
}

function lastNodes(emit: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(emit).toHaveBeenCalled();
  return emit.mock.calls[emit.mock.calls.length - 1]![0] as CanvasNode[];
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 활성 조건 (AC-32) -----------------------------------------------------

describe('분리 단추는 부품이 골라졌을 때만 켜진다', () => {
  it('아무것도 고르지 않으면 꺼져 있다', () => {
    setup();
    expect(detachButton()).toBeDisabled();
  });

  it('부품을 고르면 켜진다', () => {
    setup();
    enterPart(AT.body);
    expect(detachButton()).toBeEnabled();
  });

  it('최상위 요소를 고르면 꺼진다 — 최상위 요소에는 분리가 없다 (AC-32)', () => {
    setup();
    click(AT.sibling);
    expect(detachButton()).toBeDisabled();
  });

  it('부품 위를 **한 번만** 누르면 꺼진 채다 — 그때 골라진 것은 그룹이다', () => {
    setup();
    click(AT.body);
    expect(detachButton()).toBeDisabled();
    expect(ungroupButton()).toBeEnabled();
  });

  it('그룹을 고르면 꺼진다 — 그때 켜지는 것은 그룹 해제다', () => {
    setup();
    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    expect(detachButton()).toBeDisabled();
    expect(ungroupButton()).toBeEnabled();
  });

  it('둘이 **함께 켜지지 않는다** — 한 선택에서 다루는 범위는 하나다', () => {
    setup();
    for (const step of [
      () => enterPart(AT.body),
      () => marqueeSelect(AT.aboveGroup, { x: 80, y: 50 }),
      () => click(AT.sibling),
      () => click(AT.empty),
    ]) {
      step();
      const both = !detachButton().disabled && !ungroupButton().disabled;
      expect(both, '분리와 해제가 동시에 켜졌다').toBe(false);
    }
  });
});

// --- 실제로 뺀다 (AC-24 · AC-25) -------------------------------------------

describe('누르면 부품이 최상위로 올라온다', () => {
  it('그룹의 부품이 하나 줄고 새 요소가 **그룹 바로 뒤**에 선다', () => {
    const emit = setup();
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());

    const nodes = lastNodes(emit);
    expect(nodes.map((n) => n.id)[0]).toBe('grp-1');
    // 자리 1 이 올라온 요소, 자리 2 가 형제다.
    expect(nodes).toHaveLength(3);
    expect(nodes[2]!.id).toBe('el-3');

    const g = nodes.find(isGroup)!;
    expect(g.parts.map((p) => p.id)).toEqual(['head', 'tail']);
  });

  it('올라온 요소의 기하가 **캔버스 절대 좌표**다 — 자리가 움직이지 않는다', () => {
    const emit = setup();
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());

    // 로컬 (1000,1000)-(5000,5000) → 캔버스 (60,48)-(100,80).
    expect(geometryOf(lastNodes(emit)[1]!)).toEqual({ x: 60, y: 48, w: 40, h: 32 });
  });

  it('뺀 것이 선택으로 남는다 — 방금 뺀 것을 곧바로 끌 수 있어야 한다', () => {
    const emit = setup();
    enterPart(AT.body);
    fireEvent.click(detachButton());

    const liftedId = (emit.mock.calls[emit.mock.calls.length - 1]![0] as CanvasNode[])[1]!.id;
    expect(screen.getByTestId('selection').textContent).toBe(liftedId);
  });
});

// --- 규칙 손실 확인 (AC-31) ------------------------------------------------

describe('규칙이 걸린 그룹에서는 **먼저 묻는다** (AC-31 · REQ-05-c)', () => {
  it('규칙이 없으면 묻지 않고 곧바로 뺀다', () => {
    const emit = setup();
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());

    expect(screen.queryByTestId('canvas-group-detach-ask')).toBeNull();
    expect(emit).toHaveBeenCalledTimes(1);
  });

  it('규칙이 있으면 확인을 띄우고 **아직 빼지 않는다**', () => {
    const emit = setup([group({ rules: [NODATA_ROW] }), sibling()]);
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());

    expect(screen.getByTestId('canvas-group-detach-ask')).not.toBeNull();
    expect(emit).not.toHaveBeenCalled();
  });

  it('확인하면 뺀다', () => {
    const emit = setup([group({ rules: [NODATA_ROW] }), sibling()]);
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());
    fireEvent.click(screen.getByTestId('canvas-group-ungroup-yes'));

    expect(emit).toHaveBeenCalledTimes(1);
    expect(lastNodes(emit).find(isGroup)!.parts).toHaveLength(2);
  });

  it('거절하면 아무 일도 없고 확인이 사라진다', () => {
    const emit = setup([group({ rules: [NODATA_ROW] }), sibling()]);
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());
    fireEvent.click(screen.getByTestId('canvas-group-ungroup-no'));

    expect(emit).not.toHaveBeenCalled();
    expect(screen.queryByTestId('canvas-group-detach-ask')).toBeNull();
  });

  it('확인 중에 선택이 바뀌면 확인이 거둬진다 — 묻지 않은 것이 빠지면 안 된다', () => {
    const emit = setup([group({ rules: [NODATA_ROW] }), sibling()]);
    enterPart(AT.body);
    fireEvent.click(detachButton());
    expect(screen.getByTestId('canvas-group-detach-ask')).not.toBeNull();

    emit.mockClear();
    click(AT.sibling);
    expect(screen.queryByTestId('canvas-group-detach-ask')).toBeNull();
  });

  it('확인 줄은 **하나뿐**이다 — 해제 확인과 분리 확인이 함께 뜨지 않는다', () => {
    setup([group({ rules: [NODATA_ROW] }), sibling()]);
    enterPart(AT.body);
    fireEvent.click(detachButton());
    expect(screen.queryByTestId('canvas-group-ungroup-ask')).toBeNull();
    expect(screen.queryByTestId('canvas-group-detach-ask')).not.toBeNull();

    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    fireEvent.click(ungroupButton());
    expect(screen.queryByTestId('canvas-group-detach-ask')).toBeNull();
    expect(screen.queryByTestId('canvas-group-ungroup-ask')).not.toBeNull();
  });
});

// --- 그룹이 사라지는 갈래가 화면에서도 같다 (AC-29 · AC-30) ----------------

describe('남을 부품이 하나뿐이면 그룹째 풀린다', () => {
  it('부품 2 개 그룹에서 하나를 빼면 그룹 행이 사라지고 둘 다 올라온다', () => {
    const emit = setup([
      group({ parts: [rect('body', { x: 1000, y: 1000, w: 4000, h: 4000 }), rect('head', { x: 5000, y: 5000, w: 4000, h: 4000 })] }),
      sibling(),
    ]);
    enterPart(AT.body);
    emit.mockClear();
    fireEvent.click(detachButton());

    const nodes = lastNodes(emit);
    expect(nodes.find(isGroup)).toBeUndefined();
    expect(nodes).toHaveLength(3);
  });
});
