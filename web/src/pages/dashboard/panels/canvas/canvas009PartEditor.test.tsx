// 목록에서 부품 속성을 고친다 (SPEC-CANVAS-009 M5).
//
// ## 이 파일이 겨누는 이음매
//
// 부품 카드는 **캔버스 단위**로 말하고 부품은 **로컬 격자**에 저장된다. 그 사이를 읽는
// 쪽(`partInCanvasUnits`)과 쓰는 쪽(`writePartFromCanvasUnits`)이 갈라지면, 화면에는 맞는
// 숫자가 보이는데 저장되는 값이 다른 상태가 된다 — 그 어긋남은 저장 왕복을 견디고
// **다시 열었을 때만** 드러난다. 그래서 이 파일은 칸을 실제로 고치고 방출된 config 의
// 저장 좌표를 본다.
//
// ## "같은 컨트롤" 을 어떻게 재는가
//
// 눈으로 닮았는지가 아니라 **같은 함수를 지나는지**를 잰다: 두 칸에 같은 값을 넣고 같은
// 규칙으로 죄이는지 본다(AC-20 · AC-39 와 같은 규율). 다시 지은 컨트롤은 이 시험을
// 우연히 통과할 수 없다 — 클램프·부재 처리·반올림이 전부 일치해야 하기 때문이다.
//
// @spec SPEC-CANVAS-009 REQ-04 · AC-19 ~ AC-23

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement } from './canvasConfig';
import CanvasElementsEditor from './CanvasElementsEditor';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
  type CanvasEditSelectionValue,
} from './canvasEditContext';
import { GROUP_LOCAL_EXTENT, isGroup, type CanvasNode, type GroupElement } from './group/groupTypes';

afterEach(cleanup);

// --- 고정 입력 -------------------------------------------------------------
//
// 그룹 상자 (100,100)-(300,300) — 한 변 200 이라 로컬 5000 이 캔버스 100 에 정확히 대응한다
// (AC-21 이 든 그 상자다). `mid` 는 격자 한가운데이므로 캔버스 단위로 200 근방이어야 하고
// 로컬 값 5000 은 화면에 나오면 안 된다.

const HALF = GROUP_LOCAL_EXTENT / 2;

const PARTS: readonly CanvasElement[] = [
  { id: 'mid', kind: 'rect', geometry: { x: HALF, y: HALF, w: 2000, h: 2000 }, style: {} },
  { id: 'edge', kind: 'line', geometry: { x1: 0, y1: 0, x2: HALF, y2: HALF }, style: {} },
  { id: 'label', kind: 'text', geometry: { x: HALF, y: 9200 }, style: {}, text: '온도' },
];

function groupNode(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 100, y: 100, w: 200, h: 200 },
    parts: PARTS.map((p) => ({ ...p })),
    ...over,
  };
}

function rectNode(id = 'r1'): Record<string, unknown> {
  return { id, kind: 'rect', geometry: { x: 50, y: 80, w: 150, h: 160 }, style: {} };
}

function cfg(elements: readonly unknown[]): Record<string, unknown> {
  return { channel_name: '', data_source: 'store', canvas: { width: 500, height: 400 }, elements };
}

// --- 하네스 ---------------------------------------------------------------

function setup(config: Record<string, unknown> = cfg([rectNode(), groupNode()])) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  return onConfigChange;
}

/** 선택 상태까지 들여다보는 하네스(AC-23 용). */
function setupWithSelection(config: Record<string, unknown>) {
  const live: { selection: Set<string> } = { selection: new Set() };
  function Harness() {
    const state: CanvasEditSelectionValue = useCanvasEditSelectionState();
    live.selection = new Set(state.selection);
    return (
      <CanvasEditSelectionContext value={state}>
        <CanvasElementsEditor config={config} onConfigChange={() => {}} />
      </CanvasEditSelectionContext>
    );
  }
  render(<Harness />);
  return live;
}

function lastNodes(spy: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
  return patch.elements as CanvasNode[];
}

function partIn(nodes: readonly CanvasNode[], partId: string): CanvasElement {
  const g = nodes.find(isGroup) as GroupElement;
  expect(g, '그룹이 배열에 남아 있다').toBeDefined();
  const p = g.parts.find((x) => x.id === partId);
  expect(p, partId).toBeDefined();
  return p!;
}

/** 그룹 행과 부품 카드를 함께 편다. 전제(접혀 있었다)를 함께 단언한다. */
function openPart(groupIdx: number, partIdx: number): void {
  const groupToggle = screen.getByTestId(`canvas-group-row-toggle-${groupIdx}`);
  expect(groupToggle, '전제 — 그룹 행은 접힌 채로 태어난다').toHaveAttribute(
    'aria-expanded',
    'false',
  );
  fireEvent.click(groupToggle);
  const partToggle = screen.getByTestId(`canvas-group-row-part-toggle-${groupIdx}-${partIdx}`);
  expect(partToggle, '전제 — 부품 카드도 접힌 채로 태어난다').toHaveAttribute(
    'aria-expanded',
    'false',
  );
  fireEvent.click(partToggle);
}

// --- 겉모습 칸이 선다 (AC-19) ----------------------------------------------

describe('부품 행이 겉모습 칸을 낸다 (AC-19)', () => {
  it('펼치면 채우기 · 테두리 · 불투명도 칸이 있다', () => {
    setup();
    openPart(1, 0);
    for (const id of [
      'canvas-part-fill-1-0',
      'canvas-part-stroke-1-0',
      'canvas-part-opacity-1-0',
      'canvas-part-stroke-width-1-0',
      'canvas-part-visible-1-0',
    ]) {
      expect(screen.queryByTestId(id), id).not.toBeNull();
    }
  });

  it('문구 · 바인딩 칸도 함께 선다 (REQ-04 의 셋)', () => {
    setup();
    openPart(1, 2);
    expect(screen.queryByTestId('canvas-part-text-1-2')).not.toBeNull();
    expect(screen.queryByTestId('canvas-part-binding-1-2')).not.toBeNull();
  });

  it('접으면 사라진다 — 펼침이 실제로 무언가를 가른다', () => {
    setup();
    openPart(1, 0);
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-0'));
    expect(screen.queryByTestId('canvas-part-body-1-0')).toBeNull();
  });
});

// --- 같은 컨트롤 (AC-20) ---------------------------------------------------

describe('최상위 요소 행과 **같은 컨트롤**이다 (AC-20)', () => {
  it('불투명도 칸의 형상이 같다', () => {
    setup();
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    openPart(1, 0);

    const element = screen.getByTestId('canvas-element-opacity-0');
    const part = screen.getByTestId('canvas-part-opacity-1-0');
    for (const attr of ['type', 'step', 'min', 'max', 'placeholder']) {
      expect(part.getAttribute(attr), attr).toBe(element.getAttribute(attr));
    }
  });

  it('같은 클램프를 지난다 — 두 칸에 1.5 를 넣으면 둘 다 1 이다', () => {
    const spy = setup();
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    fireEvent.change(screen.getByTestId('canvas-element-opacity-0'), { target: { value: '1.5' } });
    expect(lastNodes(spy)[0]!.style?.opacity).toBe(1);

    openPart(1, 0);
    fireEvent.change(screen.getByTestId('canvas-part-opacity-1-0'), { target: { value: '1.5' } });
    expect(partIn(lastNodes(spy), 'mid').style.opacity).toBe(1);
  });

  it('빈 칸은 **부재**다 — 키 자체가 지워지고 `0` 으로 눌러앉지 않는다', () => {
    // 저술된 값에서 출발한다. 이 편집기는 제 상태를 들지 않고 config 를 되받아 그리므로
    // (한 번 방출한 뒤에도 화면의 값은 그대로다), 비우는 몸짓을 재려면 **처음부터 값이
    // 있어야** 한다 — 그러지 않으면 두 번째 `change` 가 같은 값이라 이벤트조차 나지 않고
    // 시험은 첫 방출을 보며 엉뚱한 이유로 빨개진다.
    const withOpacity = groupNode({
      parts: PARTS.map((p) => (p.id === 'mid' ? { ...p, style: { opacity: 0.5 } } : { ...p })),
    });
    const spy = setup(cfg([rectNode(), withOpacity]));
    openPart(1, 0);
    expect(screen.getByTestId('canvas-part-opacity-1-0')).toHaveValue(0.5);
    fireEvent.change(screen.getByTestId('canvas-part-opacity-1-0'), { target: { value: '' } });
    expect('opacity' in partIn(lastNodes(spy), 'mid').style).toBe(false);
  });

  it('새 색 고르개를 짓지 않는다 — 요소 행과 같은 컴포넌트다', () => {
    setup();
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    openPart(1, 0);
    const element = screen.getByTestId('canvas-element-fill-0');
    const part = screen.getByTestId('canvas-part-fill-1-0');
    expect(part.tagName).toBe(element.tagName);
    expect(part.className).toBe(element.className);
  });
});

// --- 기하가 캔버스 단위로 말한다 (AC-21 · AC-22) ---------------------------

describe('기하 칸이 캔버스 단위로 말한다 (AC-21)', () => {
  it('격자 한가운데 부품이 200 근방으로 보인다 — 로컬 5000 이 아니다', () => {
    setup();
    openPart(1, 0);
    // 로컬 5000 → 100 + 5000/10000 × 200 = 200.
    expect(screen.getByTestId('canvas-part-geo-x-1-0')).toHaveValue(200);
    expect(screen.getByTestId('canvas-part-geo-y-1-0')).toHaveValue(200);
    // 로컬 2000 → 2000/10000 × 200 = 40.
    expect(screen.getByTestId('canvas-part-geo-w-1-0')).toHaveValue(40);
  });

  it('선 부품은 끝점 넷이 캔버스 단위다', () => {
    setup();
    openPart(1, 1);
    expect(screen.getByTestId('canvas-part-geo-x1-1-1')).toHaveValue(100);
    expect(screen.getByTestId('canvas-part-geo-x2-1-1')).toHaveValue(200);
  });

  it('고친 값이 **로컬 좌표**로 저장된다 — 읽기와 쓰기가 같은 자를 쓴다', () => {
    const spy = setup();
    openPart(1, 0);
    // 캔버스 220 → 로컬 (220 - 100) / 200 × 10000 = 6000.
    fireEvent.change(screen.getByTestId('canvas-part-geo-x-1-0'), { target: { value: '220' } });
    expect((partIn(lastNodes(spy), 'mid').geometry as BoxGeometry).x).toBe(6000);
  });

  it('기하를 고쳐도 그룹 상자는 그대로다 (004 A16)', () => {
    const spy = setup();
    openPart(1, 0);
    fireEvent.change(screen.getByTestId('canvas-part-geo-x-1-0'), { target: { value: '220' } });
    expect(lastNodes(spy).find(isGroup)!.geometry).toEqual({ x: 100, y: 100, w: 200, h: 200 });
  });

  it('겉모습을 고쳐도 저장 좌표는 **로컬 그대로**다 — 되돌림이 한 번만 일어난다', () => {
    // 읽은 캔버스 단위를 그대로 되쓰면서 로컬로 되돌리지 않으면, 색만 바꿔도 좌표가
    // 200 으로 눌러앉는다. 그 결함은 색을 바꾸는 순간 도형이 뛰는 것으로만 보인다.
    const spy = setup();
    openPart(1, 0);
    fireEvent.change(screen.getByTestId('canvas-part-opacity-1-0'), { target: { value: '0.5' } });
    expect(partIn(lastNodes(spy), 'mid').geometry).toEqual({
      x: HALF,
      y: HALF,
      w: 2000,
      h: 2000,
    });
  });

  it('부품 id 가 복합 키로 눌러앉지 않는다 — 읽기가 실어 보낸 키를 쓰기가 되돌린다', () => {
    const spy = setup();
    openPart(1, 0);
    fireEvent.change(screen.getByTestId('canvas-part-opacity-1-0'), { target: { value: '0.5' } });
    expect((lastNodes(spy).find(isGroup) as GroupElement).parts.map((p) => p.id)).toEqual([
      'mid',
      'edge',
      'label',
    ]);
  });
});

describe('그룹 로컬 격자가 화면에 없다 (AC-22)', () => {
  it('세 부품을 모두 펼쳐도 로컬 좌표값이 어느 칸에도 없다', () => {
    setup();
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    for (const pIdx of [0, 1, 2]) {
      fireEvent.click(screen.getByTestId(`canvas-group-row-part-toggle-1-${pIdx}`));
    }
    const shown = [...document.querySelectorAll('input')].map((el) =>
      Number((el as HTMLInputElement).value),
    );
    for (const local of [HALF, 2000, 9200]) {
      expect(shown, String(local)).not.toContain(local);
    }
  });
});

// --- 부품 행을 누르면 부품이 골라진다 (AC-23) ------------------------------

describe('부품 행을 누르면 **부품**이 골라진다 (AC-23 — 004 에서 뒤집힘)', () => {
  it('선택 상태에 복합 키가 들어간다', () => {
    const live = setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-1'));
    expect([...live.selection]).toEqual(['grp-1/edge']);
  });

  it('접는 방향에서도 고른다 — 부품의 자동 펼침은 그룹 행을 향하므로 되돌아오지 않는다', () => {
    const live = setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-1'));
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-1'));
    expect([...live.selection]).toEqual(['grp-1/edge']);
    expect(screen.queryByTestId('canvas-part-body-1-1')).toBeNull();
  });

  it('고른 부품의 행이 강조된다 — 어느 줄이 골라졌는지 목록이 말한다', () => {
    setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-1'));
    expect(screen.getByTestId('canvas-group-row-part-1-1')).toHaveAttribute(
      'data-selected',
      'true',
    );
    expect(screen.getByTestId('canvas-group-row-part-1-0')).not.toHaveAttribute('data-selected');
  });
});
