// 앵커 도구 — 몸짓과 두 표면 (SPEC-CANVAS-011 M3'b).
//
// ## 이 파일이 겨누는 이음매
//
// 011 이전의 오버레이에는 **도구가 없었다.** 포인터 경로는 언제나 "고르기" 한 뜻으로 돌았고,
// 더블클릭의 뜻은 009 가 정한 하나(그룹 진입)뿐이었다. M3'b 는 그 경로에 **뜻이 둘인 순간**을
// 만든다 — 그리고 그 둘을 가르는 것은 대상이 아니라 **도구**다.
//
// 그래서 여기서 재는 쌍이 넷이다:
//
//   도구 꺼짐 + 도형    → 아무 일도 없다            (AC-26)
//   도구 꺼짐 + 부품    → 009 그대로 그룹 진입      (AC-27)
//   도구 켜짐 + 도형    → 앵커가 는다              (AC-17)
//   도구 켜짐 + 부품    → **그룹**에 앵커가 늘고 진입하지 않는다  ← AC 가 이름 적지 않은 자리
//
// 넷째가 이 파일의 이유다. 셋만 재면 "도구가 켜져 있어도 부품 위에서는 옛 뜻이 산다" 는
// 구현이 전량 초록으로 통과하고, 그 어긋남은 그룹 위에서 앵커를 놓으려는 손에만 보인다.
//
// ## 고정 입력의 산술 (009 의 그 좌표를 그대로 쓴다)
//
// 스테이지 200×100 · 캔버스 500×400 → 축척 가로 0.4 · 세로 0.25.
//
//   r1   캔버스 (100,100)-(300,300)  → px  40..120 × 25..75   중심 px (80,50)
//   l1   캔버스 (400,40)-(480,40)    → px 160..192 × 10
//   t1   캔버스 (40,360)             → px (16,90)
//   grp-1 캔버스 (50,40)-(150,120)   → px  20..60  × 10..30
//     body 로컬 (1000,1000)-(5000,5000) → 캔버스 (60,48)-(100,80) → px 24..40 × 12..20
//
// 넷의 히트 상자가 여유 6px 을 부풀린 뒤에도 **서로 닿지 않는다** — 닿으면 "앵커가 늘었다"
// 가 엉뚱한 요소에서 참이 되어 시험이 옳은 이유로 초록이 되지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-02 · REQ-02' · AC-17 · AC-23 · AC-24 · AC-26 · AC-27 · AC-60 · AC-61

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { CanvasElement, CanvasSize } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import { FIXED_ANCHOR_IDS } from './connector/anchors';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

// --- 고정 입력 -------------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

const R1: CanvasElement = {
  id: 'r1',
  kind: 'rect',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  style: {},
};

const L1: CanvasElement = {
  id: 'l1',
  kind: 'line',
  geometry: { x1: 400, y1: 40, x2: 480, y2: 40 },
  style: {},
};

const T1: CanvasElement = {
  id: 't1',
  kind: 'text',
  geometry: { x: 40, y: 360 },
  style: {},
  text: 'abc',
};

function group(): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 50, y: 40, w: 100, h: 80 },
    parts: [
      { id: 'body', kind: 'rect', geometry: { x: 1000, y: 1000, w: 4000, h: 4000 }, style: {} },
      { id: 'head', kind: 'rect', geometry: { x: 5000, y: 5000, w: 4000, h: 4000 }, style: {} },
    ],
  };
}

const ALL: readonly CanvasNode[] = [R1, L1, T1, group()];

/** 화면 px 지점들. 위 머리말의 산술에서 나온 값이다. */
const AT = {
  rect: { x: 80, y: 50 },
  line: { x: 176, y: 10 },
  text: { x: 16, y: 90 },
  body: { x: 32, y: 16 },
  empty: { x: 130, y: 95 },
} as const;

// --- 하네스 ---------------------------------------------------------------

let live: readonly CanvasNode[] = [];

/** 두 표면을 **같은 하네스**로 세운다 — `docked` 하나만 갈아 끼운다(004 의 시험 규율 E-J). */
function Harness({ docked, initial }: { docked: boolean; initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const [enabled, setEnabled] = useState(true);
  live = elements;
  const state = useCanvasEditSelectionState();
  const overlay = (
    <CanvasEditOverlay
      enabled={enabled}
      elements={elements}
      projection={PROJ}
      textWidths={{}}
      onElementsChange={setElements}
    />
  );
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].sort().join(',')}</span>
      <button type="button" data-testid="disable" onClick={() => setEnabled(false)}>
        disable
      </button>
      {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
    </CanvasEditSelectionContext>
  );
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

function setup(initial: readonly CanvasNode[] = ALL, docked = false): void {
  live = initial;
  render(<Harness docked={docked} initial={initial} />);
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
}

function at(p: { x: number; y: number }): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true };
}

function click(p: { x: number; y: number }): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', at(p)));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/** 같은 자리를 연달아 두 번 누른다 — 009 의 `doubleClick` 과 **같은 몸짓**이다. */
function doubleClick(p: { x: number; y: number }): void {
  click(p);
  click(p);
}

function toolButton(): HTMLElement {
  return screen.getByTestId('canvas-anchor-tool');
}

/** 앵커 도구를 켠다 — **켜졌음을 단언한다.** 안 켜지면 아래 시험이 옳은 이유로 돌지 않는다. */
function enableTool(): void {
  fireEvent.click(toolButton());
  expect(toolButton().getAttribute('aria-pressed'), 'precondition: 도구가 켜졌다').toBe('true');
}

function node(id: string): CanvasNode {
  const found = live.find((n) => n.id === id);
  expect(found, `${id} 가 배열에 있다`).toBeDefined();
  return found!;
}

function anchorsOf(id: string): readonly { id: string; x: number; y: number }[] {
  const found = node(id);
  // 연결선에는 `anchors` 가 없다(SPEC-CANVAS-011 M4). 이 시험의 장면에는 오지 않으므로
  // 빈 목록이 아니라 던지기로 두어, 형상이 바뀌면 조용히 "앵커가 없다" 로 통과하지 않는다.
  if (isConnector(found)) throw new Error(`${id} 는 앵커를 가질 수 없다`);
  return found.anchors ?? [];
}

function selected(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

function dotIds(): string[] {
  return screen
    .queryAllByTestId(/^canvas-anchor-dot-/)
    .map((el) => el.getAttribute('data-testid')!.replace('canvas-anchor-dot-', ''));
}

function dotAt(nodeId: string, anchorId: string): { x: number; y: number } {
  const el = screen.getByTestId(`canvas-anchor-dot-${nodeId}-${anchorId}`);
  return { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  live = [];
});

// --- 두 표면 (AC-61) -------------------------------------------------------

describe('도구가 **두 표면에 함께** 선다 (AC-61 · 불변식 I23 · I24)', () => {
  it.each([true, false])('docked=%s 에서 앵커 토글이 DOM 에 있다', (docked) => {
    setup(ALL, docked);
    expect(toolButton()).toBeTruthy();
    expect(toolButton().getAttribute('aria-pressed')).toBe('false');
  });

  it.each([true, false])('docked=%s 에서 토글이 **하나뿐**이다 — 두 벌이 되지 않았다', (docked) => {
    setup(ALL, docked);
    expect(screen.queryAllByTestId('canvas-anchor-tool')).toHaveLength(1);
  });

  it.each([true, false])('docked=%s 에서 켜면 앵커 점이 실제로 선다', (docked) => {
    setup(ALL, docked);
    expect(dotIds()).toHaveLength(0);
    enableTool();
    expect(dotIds().length).toBeGreaterThan(0);
  });

  it('다시 누르면 꺼진다 — 끄는 단추를 따로 두지 않는다', () => {
    setup();
    enableTool();
    fireEvent.click(toolButton());
    expect(toolButton().getAttribute('aria-pressed')).toBe('false');
    expect(dotIds()).toHaveLength(0);
  });
});

// --- 도구가 꺼져 있을 때 (AC-26 · AC-27) ------------------------------------

describe('도구가 꺼져 있으면 009 가 한 글자도 바뀌지 않는다', () => {
  it('도형 위 더블클릭에 앵커가 생기지 않는다 (AC-26)', () => {
    setup();
    // **먼저 그 좌표가 실제로 요소를 맞힘을 확인한다.** 빗나간 좌표로 "안 생겼다" 를
    // 재면 이 시험은 틀린 이유로 초록이 된다.
    click(AT.rect);
    expect(selected(), 'precondition: r1 을 맞혔다').toEqual(['r1']);

    doubleClick(AT.rect);
    expect(anchorsOf('r1')).toEqual([]);
    // **값으로** 견준다. 누르고 떼는 몸짓 자체는 009 이전부터 기하를 같은 값으로 다시
    // 흘릴 수 있어(빈 드래그의 확정) 참조가 갈린다 — 그것은 이 밀레스톤이 만든 변화가
    // 아니고, 여기서 재려는 것은 "앵커가 생기지 않았고 다른 것도 달라지지 않았다" 이다.
    expect(node('r1')).toEqual(R1);
  });

  it('그룹 부품 위 더블클릭은 **그룹 진입**이다 (AC-27)', () => {
    setup();
    click(AT.body);
    expect(selected(), 'precondition: 단일 클릭은 그룹이다').toEqual(['grp-1']);

    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1/body']);
    expect(anchorsOf('grp-1')).toEqual([]);
  });

  it('앵커 점이 하나도 그려지지 않는다 (REQ-02-b)', () => {
    setup();
    expect(dotIds()).toEqual([]);
  });
});

// --- 도구가 켜졌을 때: 더하기 (AC-17) ---------------------------------------

describe('도형 위 더블클릭이 앵커를 **더한다** (AC-17)', () => {
  it('상자형 요소의 `anchors` 가 하나 는다', () => {
    setup();
    click(AT.rect);
    expect(selected(), 'precondition: r1 을 맞혔다').toEqual(['r1']);
    enableTool();

    doubleClick(AT.rect);
    expect(anchorsOf('r1')).toHaveLength(1);
  });

  it('저장 좌표가 **로컬 격자**다 — 캔버스 값이 아니다 (AC-18 의 화면 쪽 확인)', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    // px (80,50) → 캔버스 (200,200) → 상자 (100,100)-(300,300) 안에서 한가운데.
    expect(anchorsOf('r1')[0]).toEqual({ id: 'a1', x: 5000, y: 5000 });
  });

  it('둘째 앵커는 `a2` 다 — 다른 자리에 놓으면 둘이 함께 남는다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    doubleClick({ x: AT.rect.x + 20, y: AT.rect.y });
    expect(anchorsOf('r1').map((a) => a.id)).toEqual(['a1', 'a2']);
  });

  it('한 번 누르는 것만으로는 생기지 않는다 — 몸짓은 여전히 **두 번째** 누름이다', () => {
    setup();
    enableTool();
    click(AT.rect);
    expect(anchorsOf('r1')).toEqual([]);
  });

  it('빈 자리를 사이에 두면 사슬이 끊겨 생기지 않는다', () => {
    setup();
    enableTool();
    click(AT.rect);
    click(AT.empty);
    click(AT.rect);
    expect(anchorsOf('r1')).toEqual([]);
  });

  it('선택을 건드리지 않는다 — 앵커를 놓는 일은 고르는 일이 아니다', () => {
    setup();
    click(AT.rect);
    enableTool();
    doubleClick(AT.rect);
    expect(selected()).toEqual(['r1']);
  });
});

// --- 도구가 켜졌을 때: 빼기 (AC-23) -----------------------------------------

describe('앵커 위 더블클릭이 그것을 **뺀다** (AC-23)', () => {
  it('같은 자리를 다시 더블클릭하면 `anchors` 가 준다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    expect(anchorsOf('r1'), 'precondition: 하나 놓였다').toHaveLength(1);

    doubleClick(AT.rect);
    expect(anchorsOf('r1')).toEqual([]);
  });

  it('마지막 하나를 빼면 **키 자체가 없다** (AC-25 의 화면 쪽 확인)', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    doubleClick(AT.rect);
    expect('anchors' in node('r1')).toBe(false);
  });

  it('보이는 점보다 좁게 집히지 않는다 — 점 가장자리도 빠진다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    // 점은 지름 8px 이고 오차는 9px 이다. 4px 은 그 점 **안**이므로 여기서 하나가 늘면
    // 사용자가 보는 점 위를 눌렀는데 옆에 새 앵커가 서는 화면이 된다.
    doubleClick({ x: AT.rect.x + 4, y: AT.rect.y });
    expect(anchorsOf('r1')).toEqual([]);
  });

  it('오차 **밖**에서는 곁에 새 앵커가 선다 — 빼기가 더하기를 삼키지 않는다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    doubleClick({ x: AT.rect.x + 12, y: AT.rect.y });
    expect(anchorsOf('r1')).toHaveLength(2);
  });

  it('그려진 점의 자리가 곧 집히는 자리다 — 둘이 같은 함수에서 나온다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    expect(dotAt('r1', 'a1')).toEqual(AT.rect);
  });

  it('세 번 눌러도 홀짝으로 갈리지 않는다 — 몸짓을 먹으면 사슬이 끊긴다', () => {
    setup();
    enableTool();
    click(AT.rect);
    click(AT.rect); // 여기서 하나 놓인다
    click(AT.rect); // 사슬이 끊겼으므로 이 누름은 첫 누름이다
    expect(anchorsOf('r1')).toHaveLength(1);
  });
});

// --- 선·텍스트 (AC-24) ------------------------------------------------------

describe('선과 텍스트에는 더할 수 없고, 그 사실을 화면이 말한다 (AC-24)', () => {
  it.each([
    ['l1', AT.line],
    ['t1', AT.text],
  ])('%s 위의 더블클릭에 `anchors` 가 생기지 않는다', (id, point) => {
    setup();
    click(point);
    expect(selected(), `precondition: ${id} 을 맞혔다`).toEqual([id]);
    enableTool();

    expect(() => doubleClick(point)).not.toThrow();
    expect(anchorsOf(id)).toEqual([]);
    expect('anchors' in node(id)).toBe(false);
  });

  it('거절 안내가 뜬다 — **조용한 무효가 아니다**', () => {
    setup();
    enableTool();
    expect(screen.queryByTestId('canvas-anchor-refusal')).toBeNull();

    doubleClick(AT.line);
    const notice = screen.getByTestId('canvas-anchor-refusal');
    expect(notice.getAttribute('role')).toBe('status');
    expect(notice.textContent).toBe('dashboard.canvas.edit.anchorRefusalNotBoxed');
  });

  it('도구를 끄면 안내가 걷힌다 — 그 도구에 대한 말이기 때문이다', () => {
    setup();
    enableTool();
    doubleClick(AT.line);
    expect(screen.getByTestId('canvas-anchor-refusal')).toBeTruthy();

    fireEvent.click(toolButton());
    expect(screen.queryByTestId('canvas-anchor-refusal')).toBeNull();
  });
});

// --- 이음매: 도구 켜짐 + 그룹 부품 -------------------------------------------

describe('도구가 켜져 있으면 부품 위 더블클릭도 **앵커**다 (009 와의 이음매)', () => {
  it('앵커는 부품이 아니라 **그룹**에 선다 (A3 — 부품은 앵커를 내지 않는다)', () => {
    setup();
    click(AT.body);
    expect(selected(), 'precondition: 부품을 맞혔다').toEqual(['grp-1']);
    enableTool();

    doubleClick(AT.body);
    expect(anchorsOf('grp-1')).toHaveLength(1);
  });

  it('부품의 저장 좌표는 한 자리도 바뀌지 않는다', () => {
    setup();
    enableTool();
    doubleClick(AT.body);
    const g = node('grp-1') as GroupElement;
    expect(g.parts.map((p) => p.geometry)).toEqual([
      { x: 1000, y: 1000, w: 4000, h: 4000 },
      { x: 5000, y: 5000, w: 4000, h: 4000 },
    ]);
    for (const part of g.parts) expect('anchors' in part).toBe(false);
  });

  it('**그룹 안으로 들어가지 않는다** — 두 몸짓이 섞이지 않는 자리다 (REQ-05-c)', () => {
    setup();
    click(AT.body);
    enableTool();
    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1']);
  });

  it('도구를 끄면 같은 자리가 **도로 그룹 진입**이다 — 뜻을 가르는 것은 도구다', () => {
    setup();
    enableTool();
    doubleClick(AT.body);
    expect(anchorsOf('grp-1'), 'precondition: 켜진 동안에는 앵커였다').toHaveLength(1);

    fireEvent.click(toolButton());
    doubleClick(AT.body);
    expect(selected()).toEqual(['grp-1/body']);
    // 껐으므로 더 늘지 않는다.
    expect(anchorsOf('grp-1')).toHaveLength(1);
  });
});

// --- 앵커 점이 그려지는 범위 (REQ-02-b · AC-60) -----------------------------

describe('앵커 점은 **최상위 전부**에, 그리고 **편집 중에만** 선다', () => {
  it('네 노드가 각각 고정 아홉을 낸다 — 고른 것에만 서지 않는다', () => {
    setup();
    enableTool();
    expect(dotIds()).toHaveLength(ALL.length * FIXED_ANCHOR_IDS.length);
    for (const el of ALL) {
      for (const id of FIXED_ANCHOR_IDS) {
        const dot = screen.getByTestId(`canvas-anchor-dot-${el.id}-${id}`);
        expect(dot, `${el.id}/${id}`).toBeTruthy();
      }
    }
  });

  it('임의 앵커를 더하면 그 노드만 하나 는다', () => {
    setup();
    enableTool();
    doubleClick(AT.rect);
    expect(dotIds()).toHaveLength(ALL.length * FIXED_ANCHOR_IDS.length + 1);
    expect(screen.getByTestId('canvas-anchor-dot-r1-a1')).toBeTruthy();
  });

  it('부품에는 서지 않는다 (A3)', () => {
    setup();
    enableTool();
    for (const partId of ['body', 'head']) {
      expect(screen.queryByTestId(`canvas-anchor-dot-${partId}-nw`), partId).toBeNull();
      expect(screen.queryByTestId(`canvas-anchor-dot-grp-1/${partId}-nw`), partId).toBeNull();
    }
  });

  it('표시 전용으로 넘어가면 DOM 에서 사라진다 (AC-60)', () => {
    setup();
    enableTool();
    expect(dotIds().length, 'precondition: 지금은 보인다').toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId('disable'));
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    expect(dotIds()).toEqual([]);
    expect(screen.queryAllByTestId('canvas-anchor-tool')).toHaveLength(0);
  });

  it('점은 **표식이다** — 단추가 아니고 포인터를 먹지 않는다 (AC-28 · 위험 R8)', () => {
    setup();
    enableTool();
    for (const el of screen.queryAllByTestId(/^canvas-anchor-dot-/)) {
      expect(el.tagName, el.dataset.testid).toBe('DIV');
      expect(el.getAttribute('aria-hidden'), el.dataset.testid).toBe('true');
      expect(el.className, el.dataset.testid).toContain('pointer-events-none');
    }
  });

  it('점은 오버레이 루트의 **직계 자식**이다 — 손잡이·윤곽선과 같은 좌표계에 산다', () => {
    setup();
    enableTool();
    const children = new Set(overlayRoot().children);
    for (const el of screen.queryAllByTestId(/^canvas-anchor-dot-/)) {
      expect(children.has(el), el.dataset.testid).toBe(true);
    }
  });
});
