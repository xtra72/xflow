// 그룹 진입과 부품 핸들 — 단일 클릭은 그룹, 더블클릭은 그 안 (SPEC-CANVAS-009 M3).
//
// ## 이 파일이 겨누는 이음매
//
// 004 는 히트 테스트에서 **이미** `{ nodeId, partId }` 를 돌려주고 있었고, 오버레이가 그
// `partId` 를 **버렸다**. 009 가 그 값을 살렸지만, 0.2.0 은 그것을 **단일 클릭**에 걸어
// 두어 그룹을 클릭으로 고를 길을 없애 버렸다(사용자 보고: "그룹핑 객체가 선택되지 않음").
//
// 0.3.0 은 그림 도구의 관용구로 옮긴다 — **단일 클릭은 그룹, 더블클릭은 그 안.** 한 번
// 들어간 뒤에는 그 그룹 안에서 단일 클릭도 부품을 고르고(머무름), 빈 곳이나 다른 요소를
// 누르면 빠져나온다. "안에 있음" 은 별도 상태가 아니라 **선택에서 파생**된다.
//
// ## 고정 입력의 산술
//
// 스테이지 200×100 · 캔버스 500×400 이므로 축척은 가로 0.4 · 세로 0.25 다.
//
//   그룹 상자  캔버스 (50,40)-(150,120)   → px 20..60 × 10..30
//   body 로컬 (1000,1000)-(5000,5000)     → 캔버스 (60,48)-(100,80)   → px 24..40 × 12..20
//   head 로컬 (5000,5000)-(9000,9000)     → 캔버스 (100,80)-(140,112) → px 40..56 × 20..28
//
// **부품 상자가 그룹 상자와 한 모서리도 겹치지 않게** 골랐다. 겹치면 "핸들이 부품 상자에
// 섰다" 와 "그룹 상자에 섰다" 가 같은 좌표를 내어 AC-10 이 옳은 이유로 초록이 되지 않는다.
//
// @spec SPEC-CANVAS-009 REQ-01 · REQ-02 · REQ-03 · AC-05 ~ AC-12 · AC-40 · AC-41 · AC-43

import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize, Geometry } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import { type CanvasNode, type GroupElement } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

/**
 * 노드의 기하. 연결선에는 기하가 없으므로(SPEC-CANVAS-011 M4) 좁혀 읽는다 — 이 시험의
 * 장면에는 연결선이 오지 않으며, 그때는 `undefined` 라 단언이 조용히 통과하지 않는다.
 */
function geometryOf(node: CanvasNode): Geometry | undefined {
  return isConnector(node) ? undefined : node.geometry;
}


// --- 고정 입력 -------------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 50, y: 40, w: 100, h: 80 },
    parts: [
      rect('body', { x: 1000, y: 1000, w: 4000, h: 4000 }),
      rect('head', { x: 5000, y: 5000, w: 4000, h: 4000 }),
    ],
    ...over,
  };
}

/** 최상위 형제 — px 160..200 × 80..95. 마키와 최상위 선택이 닿는 자리다. */
function sibling(): CanvasElement {
  return rect('el-3', { x: 400, y: 320, w: 100, h: 60 });
}

/** 화면 px 중심들. 위 머리말의 산술에서 나온 값이다. */
const AT = {
  body: { x: 32, y: 16 },
  head: { x: 48, y: 24 },
  /** 그룹 상자 **안**이지만 어느 부품에도 닿지 않는 지점(AC-09). */
  groupGap: { x: 58, y: 12 },
  sibling: { x: 180, y: 88 },
  /** 어느 요소에도 닿지 않는 바깥 지점. */
  empty: { x: 5, y: 95 },
  /** 그룹 상자의 **왼쪽 위 바깥** — 여기서 아래로 그으면 그룹만 온전히 감싼다. */
  aboveGroup: { x: 5, y: 5 },
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
      <span data-testid="auto-expanded">{state.autoExpandedId ?? ''}</span>
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
    left: 0,
    top: 0,
    width: STAGE.width,
    height: STAGE.height,
    right: STAGE.width,
    bottom: STAGE.height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
  return emit;
}

function at(p: { x: number; y: number }): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true };
}

function press(target: HTMLElement, p: { x: number; y: number }): void {
  fireEvent(target, new MouseEvent('pointerdown', at(p)));
}

/** 누르고 뗀다 — 놓지 않으면 끄는 중이라 다음 몸짓이 닿지 않는다. */
function click(p: { x: number; y: number }): void {
  press(overlayRoot(), p);
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/**
 * 같은 자리를 연달아 두 번 누른다 — **그룹 안으로 들어가는 몸짓**이다.
 *
 * 두 누름이 같은 프레임 안에서 일어나므로 판정 창(400ms) 안이고 좌표도 같다. 실제
 * 브라우저에서도 두 번의 빠른 누름이 곧 더블클릭이므로, 이 하네스가 지어낸 몸짓이 아니다.
 */
function doubleClick(p: { x: number; y: number }): void {
  click(p);
  click(p);
}

/**
 * 빈 자리에서 사각형을 그어 **최상위 원소**를 고른다(그룹 자신을 고르는 유일한 몸짓).
 *
 * **Ctrl 을 짚는다**(SPEC-CANVAS-011). 이 파일이 재는 것은 009 의 선택 규칙이지 사각형을
 * 시작하는 몸짓이 아니다 — 011 이 맨손 끌기를 팬에 주었으므로, 이 도우미는 그 몸짓의
 * 새 이름을 쓸 뿐 기대값은 한 글자도 바뀌지 않는다.
 */
function marqueeSelect(from: { x: number; y: number }, to: { x: number; y: number }): void {
  const ctrl = { ...at(from), ctrlKey: true };
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', ctrl));
  fireEvent(overlayRoot(), new MouseEvent('pointermove', { ...at(to), ctrlKey: true }));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', { ...at(to), ctrlKey: true }));
}

function selected(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

function handleIds(): string[] {
  return screen
    .queryAllByTestId(/^canvas-handle-/)
    .map((el) => el.getAttribute('data-testid')!.replace('canvas-handle-', ''));
}

function handleAt(id: string): { x: number; y: number } {
  const el = screen.getByTestId(`canvas-handle-${id}`);
  return { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
}

function lastNodes(emit: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(emit).toHaveBeenCalled();
  return emit.mock.calls[emit.mock.calls.length - 1]![0] as CanvasNode[];
}

function partsOf(nodes: readonly CanvasNode[]): readonly CanvasElement[] {
  const g = nodes.find((n) => n.id === 'grp-1') as GroupElement;
  expect(g, '그룹이 배열에 남아 있다').toBeDefined();
  return g.parts;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 선택 (AC-05 ~ AC-09) --------------------------------------------------

describe('단일 클릭은 **그룹**을 고른다 (AC-05 · 사용자 보고 재현)', () => {
  it('부품 위를 한 번 누르면 그룹이 골라진다 — 부품이 아니다', () => {
    setup();
    click(AT.body);
    expect(selected()).toEqual(['grp-1']);
  });

  it('어느 부품 위를 누르든 같다 — 그룹은 부품 잉크로 잡히지만 고르는 것은 그룹이다', () => {
    setup();
    click(AT.head);
    expect(selected()).toEqual(['grp-1']);
  });

  it('그룹을 고른 뒤 끌면 **그룹 전체**가 옮겨진다 — 부품 좌표는 그대로다', () => {
    const emit = setup();
    press(overlayRoot(), AT.body);
    fireEvent(overlayRoot(), new MouseEvent('pointermove', at({ x: AT.body.x + 4, y: AT.body.y })));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', at({ x: AT.body.x + 4, y: AT.body.y })));

    const nodes = lastNodes(emit);
    const g = nodes.find((n) => n.id === 'grp-1') as GroupElement;
    // 캔버스 4px → 10 단위. 그룹 상자만 움직인다.
    expect(g.geometry).toEqual({ x: 60, y: 40, w: 100, h: 80 });
    expect(g.parts[0]!.geometry).toEqual({ x: 1000, y: 1000, w: 4000, h: 4000 });
  });
});

describe('더블클릭은 **그 부품**으로 들어간다 (AC-05)', () => {
  it('두 번 누르면 선택 상태에 복합 키가 들어간다', () => {
    setup();
    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1/body']);
  });

  it('다른 부품을 더블클릭하면 그 부품이 골라진다', () => {
    setup();
    doubleClick(AT.head);
    expect(selected()).toEqual(['grp-1/head']);
  });

  it('최상위 요소의 더블클릭은 아무것도 바꾸지 않는다 — 들어갈 안이 없다', () => {
    setup();
    doubleClick(AT.sibling);
    expect(selected()).toEqual(['el-3']);
  });

  it('서로 다른 부품을 빠르게 연달아 누른 것은 더블클릭이 아니다', () => {
    // 사용자가 한 일은 "이것, 그리고 저것" 이지 "이 안으로" 가 아니다. 판정이 시각과
    // 좌표만 본다면 이 둘이 진입으로 읽힌다.
    setup();
    click(AT.body);
    click(AT.head);
    expect(selected()).toEqual(['grp-1']);
  });

  it('빈 곳을 거치면 연타 사슬이 끊긴다', () => {
    setup();
    click(AT.body);
    click(AT.empty);
    click(AT.body);
    expect(selected()).toEqual(['grp-1']);
  });
});

describe('한 번 들어가면 그 그룹 안에 **머무른다**', () => {
  it('부품이 골라진 동안에는 단일 클릭도 같은 그룹의 부품을 고른다', () => {
    setup();
    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1/body']);
    click(AT.head);
    expect(selected()).toEqual(['grp-1/head']);
  });

  it('그룹 밖을 누르면 빠져나온다', () => {
    setup();
    doubleClick(AT.body);
    click(AT.sibling);
    expect(selected()).toEqual(['el-3']);
    // 빠져나온 뒤에는 단일 클릭이 다시 그룹을 고른다.
    click(AT.body);
    expect(selected()).toEqual(['grp-1']);
  });

  it('빈 곳을 누르면 빠져나온다', () => {
    setup();
    doubleClick(AT.body);
    click(AT.empty);
    expect(selected()).toEqual([]);
    click(AT.head);
    expect(selected()).toEqual(['grp-1']);
  });
});

describe('최상위 선택 키가 004 와 바이트 동일하다 (AC-06)', () => {
  it('최상위 요소를 누르면 정확히 `el-3` 이다 — 접두사도 구분자도 붙지 않는다', () => {
    setup();
    click(AT.sibling);
    // **이 단언이 009 전체의 안전줄이다.** 여기가 깨지면 001·002·004 의 선택 시험이
    // 전부 흔들린다.
    expect(selected()).toEqual(['el-3']);
  });

  it('마키로 고른 그룹의 키도 평평하다', () => {
    setup();
    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    expect(selected()).toEqual(['grp-1']);
  });

  it('단일 클릭으로 고른 그룹의 키도 평평하다 — 진입 전에는 복합 키가 없다', () => {
    setup();
    click(AT.body);
    expect(selected()).toEqual(['grp-1']);
    expect(selected()[0]).not.toContain('/');
  });
});

describe('부품과 그룹이 동시에 선택되지 않는다 (AC-07)', () => {
  it('부품이 골라지면 그 그룹의 id 는 선택에 없다', () => {
    setup();
    doubleClick(AT.body);
    expect(selected()).not.toContain('grp-1');
  });

  it('그룹이 골라져 있다가 더블클릭으로 들어가면 그룹이 빠진다', () => {
    setup();
    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    expect(selected()).toEqual(['grp-1']);
    doubleClick(AT.head);
    expect(selected()).toEqual(['grp-1/head']);
  });
});

describe('부품 선택은 하나뿐이다 (AC-08)', () => {
  it('안에서 다른 부품을 누르면 부품 키가 정확히 하나 남는다', () => {
    setup();
    doubleClick(AT.body);
    click(AT.head);
    expect(selected()).toEqual(['grp-1/head']);
  });

  it('안에서는 Shift 를 눌러도 더해지지 않는다 — 부품 다중 선택은 뜻이 정의되지 않았다', () => {
    setup();
    doubleClick(AT.body);
    fireEvent(overlayRoot(), new MouseEvent('pointerdown', { ...at(AT.head), shiftKey: true }));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', { ...at(AT.head), shiftKey: true }));
    expect(selected()).toEqual(['grp-1/head']);
  });

  it('최상위 요소의 Shift 더하기는 그대로다 — 뒤집힌 것은 부품뿐이다', () => {
    setup([group(), sibling(), rect('el-9', { x: 400, y: 40, w: 100, h: 60 })]);
    click(AT.sibling);
    fireEvent(overlayRoot(), new MouseEvent('pointerdown', { ...at({ x: 180, y: 15 }), shiftKey: true }));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', { ...at({ x: 180, y: 15 }), shiftKey: true }));
    expect(selected().sort()).toEqual(['el-3', 'el-9']);
  });
});

describe('그룹 상자의 빈 곳은 부품을 선택하지 않는다 (AC-09 · 004 AC-E11)', () => {
  it('어느 부품에도 닿지 않는 지점은 아무것도 고르지 않는다', () => {
    setup();
    click(AT.groupGap);
    expect(selected()).toEqual([]);
  });
});

// --- 핸들 (AC-10 ~ AC-12) --------------------------------------------------

describe('선택된 부품에 핸들이 선다 (AC-10)', () => {
  it('여덟 자리가 **부품의 투영 상자**에 놓인다', () => {
    setup();
    doubleClick(AT.body);
    expect(handleIds()).toHaveLength(8);
    // body 의 투영 상자는 px 24..40 × 12..20 이다.
    expect(handleAt('nw')).toEqual({ x: 24, y: 12 });
    expect(handleAt('se')).toEqual({ x: 40, y: 20 });
  });

  it('그룹 상자가 아니다 — 두 상자가 한 모서리도 겹치지 않는 고정 입력이다', () => {
    setup();
    doubleClick(AT.body);
    // 그룹 상자는 px 20..60 × 10..30 이다. 그 모서리가 나오면 선택을 잘못 읽은 것이다.
    expect(handleAt('nw')).not.toEqual({ x: 20, y: 10 });
    expect(handleAt('se')).not.toEqual({ x: 60, y: 30 });
  });
});

describe('그룹만 선택되면 핸들은 그룹 상자에 선다 (AC-11)', () => {
  it('004 와 동일하게 그룹 상자의 여덟 자리다', () => {
    setup();
    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    expect(handleIds()).toHaveLength(8);
    expect(handleAt('nw')).toEqual({ x: 20, y: 10 });
    expect(handleAt('se')).toEqual({ x: 60, y: 30 });
  });
});

describe('핸들이 두 상자에 동시에 서지 않는다 (AC-12)', () => {
  it('어떤 선택 상태에서든 한 벌만 나온다', () => {
    setup();
    for (const step of [() => doubleClick(AT.body), () => click(AT.head), () => click(AT.sibling)]) {
      step();
      const ids = handleIds();
      expect(new Set(ids).size, ids.join(',')).toBe(ids.length);
      expect(ids.length).toBe(8);
    }
  });

  it('아무것도 고르지 않으면 핸들이 없다', () => {
    setup();
    click(AT.empty);
    expect(handleIds()).toEqual([]);
  });
});

// --- 목록 연동 (M3 작업 3) --------------------------------------------------

describe('부품을 고르면 목록이 **그 그룹 행**을 펼친다', () => {
  it('자동 펼침 id 가 그룹 id 다 — 복합 키를 그대로 내려보내면 어느 행과도 만나지 못한다', () => {
    setup();
    doubleClick(AT.body);
    expect(screen.getByTestId('auto-expanded').textContent).toBe('grp-1');
  });

  it('최상위 요소에서는 그 요소의 id 그대로다', () => {
    setup();
    click(AT.sibling);
    expect(screen.getByTestId('auto-expanded').textContent).toBe('el-3');
  });
});

// --- 드래그가 저장 좌표에 닿는다 (REQ-03 이음매) ---------------------------

describe('부품을 끌면 **저장 좌표**가 바뀐다 (히트 → 선택 → 역투영 → 저장)', () => {
  it('가로 4px 이동이 로컬 격자에서 1000 만큼이다', () => {
    // px 4 → 캔버스 4 / 0.4 = 10 단위 → 로컬 10 / 100 × 10000 = 1000.
    const emit = setup();
    doubleClick(AT.body); // 먼저 그룹 안으로 들어간다
    press(overlayRoot(), AT.body);
    fireEvent(overlayRoot(), new MouseEvent('pointermove', at({ x: AT.body.x + 4, y: AT.body.y })));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', at({ x: AT.body.x + 4, y: AT.body.y })));

    const parts = partsOf(lastNodes(emit));
    expect(parts[0]!.geometry).toEqual({ x: 2000, y: 1000, w: 4000, h: 4000 });
    // 형제 부품은 그대로다.
    expect(parts[1]!.geometry).toEqual({ x: 5000, y: 5000, w: 4000, h: 4000 });
  });

  it('그룹 상자는 한 자리도 바뀌지 않는다 (AC-14 · 004 A16)', () => {
    const emit = setup();
    doubleClick(AT.body);
    press(overlayRoot(), AT.body);
    fireEvent(overlayRoot(), new MouseEvent('pointermove', at({ x: AT.body.x + 4, y: AT.body.y })));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', at({ x: AT.body.x + 4, y: AT.body.y })));

    const g = lastNodes(emit).find((n) => n.id === 'grp-1')!;
    expect(geometryOf(g)).toEqual({ x: 50, y: 40, w: 100, h: 80 });
  });

  it('방향키도 같은 통로를 지난다 — 끌었을 때와 갈리지 않는다', () => {
    const emit = setup();
    doubleClick(AT.body);
    emit.mockClear();
    fireEvent.keyDown(overlayRoot(), { key: 'ArrowRight' });

    const parts = partsOf(lastNodes(emit));
    // 방향키 한 번은 **캔버스 1 단위**다(`ARROW_STEPS`) → 로컬 1 / 100 × 10000 = 100.
    expect((parts[0]!.geometry as BoxGeometry).x).toBe(1100);
  });
});

// --- 회귀: 뒤집지 않은 것 (AC-43) ------------------------------------------

describe('그룹 크기 조절이 부품 저장 좌표를 바꾸지 않는다 (AC-43 · 004 A17)', () => {
  it('그룹을 골라 `se` 핸들로 늘려도 두 부품의 로컬 좌표가 그대로다', () => {
    const emit = setup();
    marqueeSelect(AT.aboveGroup, { x: 80, y: 50 });
    const handle = screen.getByTestId('canvas-handle-se');
    fireEvent(handle, new MouseEvent('pointerdown', at({ x: 60, y: 30 })));
    fireEvent(overlayRoot(), new MouseEvent('pointermove', at({ x: 80, y: 45 })));
    fireEvent(overlayRoot(), new MouseEvent('pointerup', at({ x: 80, y: 45 })));

    const nodes = lastNodes(emit);
    const g = nodes.find((n) => n.id === 'grp-1') as GroupElement;
    // 전제 — 상자는 실제로 커졌다. 안 커졌다면 아래 "부품 불변" 이 아무것도 재지 않는다.
    expect(g.geometry.w).toBeGreaterThan(100);
    expect(g.parts[0]!.geometry).toEqual({ x: 1000, y: 1000, w: 4000, h: 4000 });
    expect(g.parts[1]!.geometry).toEqual({ x: 5000, y: 5000, w: 4000, h: 4000 });
  });
});

// --- 견고성 (AC-40 · AC-41) ------------------------------------------------

describe('견고성 — 없는 것을 가리켜도 예외가 아니다 (REQ-07)', () => {
  it('부품 0 개 그룹의 상자 안을 눌러도 예외가 나지 않는다 (AC-41)', () => {
    setup([group({ parts: [] }), sibling()]);
    expect(() => click(AT.body)).not.toThrow();
    expect(selected()).toEqual([]);
  });

  it('그룹이 부품 0 개로 갈려도 화면이 무너지지 않는다 (AC-40 — 없는 부품 키가 선택에 남는다)', () => {
    const emit = vi.fn();
    const { rerender } = render(<Harness elements={[group(), sibling()]} onElementsChange={emit} />);
    vi.spyOn(overlayRoot(), 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, width: STAGE.width, height: STAGE.height,
      right: STAGE.width, bottom: STAGE.height, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1/body']);

    // 부품이 사라진 배열로 다시 그린다 — 선택에는 `grp-1/body` 가 남아 있다.
    expect(() =>
      rerender(<Harness elements={[group({ parts: [] }), sibling()]} onElementsChange={emit} />),
    ).not.toThrow();
    // 가리킬 것이 없으므로 핸들도 윤곽선도 서지 않는다(빈 선택으로 강등된 것과 같다).
    expect(handleIds()).toEqual([]);
    expect(screen.queryByTestId('canvas-selection-grp-1/body')).toBeNull();
  });

  it('그룹 자체가 사라져도 예외가 아니다', () => {
    const emit = vi.fn();
    const { rerender } = render(<Harness elements={[group(), sibling()]} onElementsChange={emit} />);
    vi.spyOn(overlayRoot(), 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, width: STAGE.width, height: STAGE.height,
      right: STAGE.width, bottom: STAGE.height, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
    doubleClick(AT.head);
    expect(() => rerender(<Harness elements={[sibling()]} onElementsChange={emit} />)).not.toThrow();
    expect(handleIds()).toEqual([]);
  });
});
