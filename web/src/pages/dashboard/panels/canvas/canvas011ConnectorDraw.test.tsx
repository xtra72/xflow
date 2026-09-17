// 긋는 몸짓 — 앵커에서 앵커로 (SPEC-CANVAS-011 M8 · AC-57~AC-61 · AC-31).
//
// ## 이 파일이 겨누는 이음매
//
// M8 은 011 에서 **연결선이 처음 생기는 자리**다. 그 전까지 연결선은 손으로 적은 config
// 에서만 왔고, 그래서 M4~M7 의 시험은 전부 "이미 있는 선" 을 재었다. 여기서부터는 만드는
// 쪽이 있으므로 새 실패 형상이 하나 열린다 — **만든 것이 그려지지 않는다.**
//
// 그 형상은 이 저장소가 **한 번 배달한 적이 있다**(SPEC-CANVAS-001). 편집기가 요소를
// `style: {}` 로 만들었고, 렌더는 색을 지어내기를 옳게 거절했으며, 두 파일은 각각
// 100% 로 초록이었다. 새 요소는 조용히 아무것도 그리지 않았다. `paintStroke` 가 색과
// **양수 두께**를 둘 다 요구하므로 연결선은 그 함정을 그대로 물려받는다.
//
// 그래서 이 파일의 중심 단언은 "배열이 하나 늘었다" 가 **아니라** §씨앗 스타일의
// "방금 그은 선이 실제로 `stroke` 를 낸다" 이다. 층을 건너지 않는 시험으로는 이 결함이
// 덮이지 않는다 — 몸짓 쪽도 렌더 쪽도 저마다 옳기 때문이다.
//
// ## 고정 입력의 산술 (011 M3'b 의 그 좌표 규율 그대로)
//
// 스테이지 200×100 · 캔버스 500×400 → 축척 가로 0.4 · 세로 0.25.
//
//   r1  캔버스 (100,100)-(300,300) → px 40..120 × 25..75
//         e  캔버스 (300,200) → px (120,50)      nw 캔버스 (100,100) → px (40,25)
//         c  캔버스 (200,200) → px ( 80,50)
//   r2  캔버스 (400, 40)-(480,120) → px 160..192 × 10..30
//         w  캔버스 (400, 80) → px (160,20)
//
// 두 무리의 앵커는 서로 **집는 오차(9px)의 네 배 넘게** 떨어져 있다 — 가까우면 "r2 의 w
// 에서 놓았다" 가 옳은 이유로 참이 되지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-03 · REQ-04 · AC-31 · AC-57 · AC-58 · AC-59 · AC-60 · AC-61

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { parseCanvasConfig, type CanvasElement, type CanvasSize } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { projectPoint, type CanvasProjection, type StageSize } from './canvasGeometry';
import { hitTest } from './canvasHitTest';
import { drawElements, type DrawContext2D } from './drawElement';
import { CANVAS_TOOLS, TOOL_CONNECTOR_ROUTE } from './connector/canvasTools';
import {
  isConnector,
  type ConnectorElement,
  type ConnectorEnd,
} from './connector/connectorTypes';
import { addAnchorAt, removeAnchor } from './connector/anchors';
import { resolveConnector } from './connector/resolveConnector';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/**
 * 연결선을 긋는 도구들 — **표에서 파생시킨다.** 넷을 손으로 적으면 다섯째가 늘 때 이
 * 파일이 조용히 넷만 재고 지나간다.
 */
const CONNECTOR_TOOLS = CANVAS_TOOLS.filter((tool) => TOOL_CONNECTOR_ROUTE[tool] !== null);

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

const R1: CanvasElement = {
  id: 'r1',
  kind: 'rect',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  style: { fill: '#111' },
};

const R2: CanvasElement = {
  id: 'r2',
  kind: 'rect',
  geometry: { x: 400, y: 40, w: 80, h: 80 },
  style: { fill: '#222' },
};

const ALL: readonly CanvasNode[] = [R1, R2];

/** 화면 px 지점들 — 위 머리말의 산술에서 나온 값이다. */
const AT = {
  /** r1 의 `e` 앵커. */
  r1East: { x: 120, y: 50 },
  /** r1 의 `nw` 앵커. */
  r1NorthWest: { x: 40, y: 25 },
  /** r1 의 `c` 앵커. */
  r1Center: { x: 80, y: 50 },
  /** r2 의 `w` 앵커. */
  r2West: { x: 160, y: 20 },
  /** r1 의 **몸통**이되 아홉 자리 어디에서도 9px 넘게 떨어진 점. */
  r1Body: { x: 100, y: 37 },
  /** 어느 요소에도 어느 앵커에도 닿지 않는 자리. */
  empty: { x: 130, y: 95 },
} as const;

// --- 하네스 ---------------------------------------------------------------

let live: readonly CanvasNode[] = [];

/** 두 표면을 **같은 하네스**로 세운다 — `docked` 하나만 갈아 끼운다(004 의 시험 규율 E-J). */
function Harness({ docked, initial }: { docked: boolean; initial: readonly CanvasNode[] }) {
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
      <span data-testid="selection">{[...state.selection].sort().join(',')}</span>
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

type Pt = { x: number; y: number };

function at(p: Pt): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true, button: 0 };
}

function down(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', at(p)));
}
function move(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointermove', at(p)));
}
function up(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/** 누르고 · 끌고 · 놓는다 — 이 파일이 재는 그 한 몸짓. */
function draw(from: Pt, to: Pt): void {
  down(from);
  move(to);
  up(to);
}

function click(p: Pt): void {
  down(p);
  up(p);
}

function toolButton(id: string): HTMLElement {
  return screen.getByTestId(`canvas-connector-tool-${id}`);
}

/** 도구를 켠다 — **켜졌음을 단언한다.** 안 켜지면 아래 시험이 옳은 이유로 돌지 않는다. */
function enableTool(id = 'straight'): void {
  fireEvent.click(toolButton(id));
  expect(toolButton(id).getAttribute('aria-pressed'), 'precondition: 도구가 켜졌다').toBe('true');
}

function connectors(): ConnectorElement[] {
  return live.filter((n): n is ConnectorElement => isConnector(n));
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

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  live = [];
});

// --- AC-61: 도구가 두 표면에 함께 선다 --------------------------------------

describe('연결선 도구 넷이 **두 표면에 함께** 선다 (AC-61 · 불변식 I23 · I24)', () => {
  it.each([true, false])('docked=%s 에서 단추 넷이 모두 DOM 에 있다', (docked) => {
    setup(ALL, docked);
    for (const id of CONNECTOR_TOOLS) {
      expect(toolButton(id), id).toBeTruthy();
      expect(toolButton(id).getAttribute('aria-pressed'), id).toBe('false');
    }
  });

  it.each([true, false])('docked=%s 에서 단추가 종류마다 **하나뿐**이다', (docked) => {
    setup(ALL, docked);
    for (const id of CONNECTOR_TOOLS) {
      expect(screen.queryAllByTestId(`canvas-connector-tool-${id}`), id).toHaveLength(1);
    }
  });

  it('넷이 서로 **배타적**이다 — 많아야 하나가 눌려 있다', () => {
    setup();
    for (const id of CONNECTOR_TOOLS) {
      fireEvent.click(toolButton(id));
      const pressed = CONNECTOR_TOOLS.filter(
        (other) => toolButton(other).getAttribute('aria-pressed') === 'true',
      );
      expect(pressed, id).toEqual([id]);
    }
  });

  it('다시 누르면 꺼진다 — 끄는 단추를 따로 두지 않는다', () => {
    setup();
    enableTool();
    fireEvent.click(toolButton('straight'));
    expect(toolButton('straight').getAttribute('aria-pressed')).toBe('false');
  });
});

// --- REQ-02-b · AC-60: 앵커는 도구가 켜진 동안에만 -------------------------

describe('연결선 도구가 앵커를 보인다 (REQ-02-b · AC-60)', () => {
  it('꺼져 있으면 점이 하나도 없다', () => {
    setup();
    expect(dotIds()).toEqual([]);
  });

  it.each([...CONNECTOR_TOOLS])('%s 를 켜면 최상위 **전부**에 점이 선다', (id) => {
    setup();
    enableTool(id);
    // 고른 것에만 세우면 출발 앵커를 고르는 순간 도착 앵커가 사라진다 — 잇는 일은 요소
    // 둘 사이에서 일어나므로 그 화면으로는 몸짓 자체가 성립하지 않는다.
    expect(dotIds()).toContain('r1-e');
    expect(dotIds()).toContain('r2-w');
  });

  it('표시 전용 패널에는 앵커가 **DOM 에 없다** (AC-60)', () => {
    // 편집이 꺼진 표면에는 이 층 자체가 서지 않는다. 도구를 켤 길조차 없으므로 "도구가
    // 켜진 채로 편집이 꺼지면?" 이라는 상태가 표현 불가능하다.
    render(
      <CanvasEditOverlay
        enabled={false}
        elements={ALL}
        projection={PROJ}
        textWidths={{}}
        onElementsChange={() => {}}
      />,
    );
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    expect(screen.queryAllByTestId(/^canvas-anchor-dot-/)).toHaveLength(0);
  });
});

// --- AC-57: 앵커에서 앵커로 ------------------------------------------------

describe('앵커에서 앵커로 그으면 연결선이 생긴다 (AC-57)', () => {
  it('배열에 연결선이 **하나** 늘고 두 끝이 각각을 참조한다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);

    expect(live).toHaveLength(3);
    expect(connectors()).toHaveLength(1);
    expect(connectors()[0]!.from).toEqual({ el: 'r1', a: 'e' });
    expect(connectors()[0]!.to).toEqual({ el: 'r2', a: 'w' });
  });

  it('도형 둘은 **한 글자도** 달라지지 않는다 — 잇는 일은 기하를 쓰지 않는다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(live.filter((n) => !isConnector(n))).toEqual([R1, R2]);
  });

  it.each([...CONNECTOR_TOOLS])('%s 도구는 제 `route` 를 실어 만든다', (id) => {
    setup();
    enableTool(id);
    draw(AT.r1East, AT.r2West);
    expect(connectors()[0]!.route).toBe(id);
  });

  it('중간점을 심지 않는다 — 쓰지 않은 키가 생기지 않는다', () => {
    setup();
    enableTool('curve');
    draw(AT.r1East, AT.r2West);
    expect('points' in connectors()[0]!).toBe(false);
  });

  it('도구가 꺼져 있으면 앵커 자리를 눌러도 그어지지 않는다', () => {
    setup();
    draw(AT.r1East, AT.r2West);
    expect(connectors()).toEqual([]);
  });

  it('중심 앵커에서도 그어진다 — 중심은 자르지 않는 자리다 (A12 · AC-16)', () => {
    setup();
    enableTool();
    draw(AT.r1Center, AT.r2West);
    expect(connectors()[0]!.from).toEqual({ el: 'r1', a: 'c' });
  });

  it('참조된 요소를 옮기면 끝점이 **따라간다** (REQ-03-a)', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    const before = resolveConnector(connectors()[0]!, live, PROJ, {})!;

    const moved = live.map((n) =>
      n.id === 'r2' ? ({ ...R2, geometry: { ...R2.geometry, x: 460 } } as CanvasNode) : n,
    );
    const after = resolveConnector(connectors()[0]!, moved, PROJ, {})!;

    expect(after[0]).toEqual(before[0]);
    expect(after[1]).toEqual({ x: 460, y: 80 });
    expect(after[1]).not.toEqual(before[1]);
  });
});

// --- AC-58: 빈 곳에서 놓으면 자유 끝점 --------------------------------------

describe('빈 곳에서 놓으면 **아무것도 생기지 않는다** (016 이 AC-58 을 뒤집는다)', () => {
  // ## 뒤집은 조항이며 지우지 않는다
  //
  // 011 AC-58 은 "빈 곳에서 놓으면 자유 끝점이다" 였다. 그 표현이 실제로 낳은 것은 **도형
  // 에서 떨어져 허공에 꽂힌 선**이고, 도형을 옮기면 그 선은 따라오지 않고 옛 자리에 남는다
  // — 011 이 붙은 끝을 참조로 둔 바로 그 이유가 자유 끝에서는 지켜지지 않았다.
  //
  // 016 은 **저술 경로에서** 자유 끝을 없앤다. 자료형의 자유 갈래는 그대로 남으므로
  // (REQ-03) 016 이전에 저장된 선은 여전히 그려지고 잡힌다 — 아래 마지막 시험이 그 사실을
  // 따로 붙든다.

  it('빈 곳에서 놓으면 선이 만들어지지 않는다 (REQ-01)', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.empty);
    expect(connectors()).toHaveLength(0);
  });

  it('도형 몸통 위에서 놓아도 앵커가 아니면 만들어지지 않는다', () => {
    // 잡는 것은 **앵커**이지 요소가 아니다 — 011 이 이 자리에 적어 둔 그 문장은 그대로
    // 참이고, 016 은 그 뒤의 처분만 바꾼다(자유 끝 대신 **아무것도 없음**).
    setup();
    enableTool();
    draw(AT.r1East, AT.r1Body);
    expect(connectors()).toHaveLength(0);
  });

  it('안내를 띄우지 않는다 — 몸짓을 그만둔 것이지 거절당한 것이 아니다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.empty);
    // 같은 함수가 "같은 앵커에서 놓으면 아무것도 만들지 않는다" 에 대해 이미 적어 둔
    // 그 판단이다. 거절 안내가 뜨면 사용자는 하지 않기로 한 일을 두고 꾸중을 듣는다.
    expect(screen.queryByTestId('canvas-anchor-refusal')).toBeNull();
  });

  it('**옛 저술의 자유 끝은 살아 있다** (REQ-03)', () => {
    // 자료형을 지우지 않는 것이 016 의 절반이다. 지우면 016 이전에 저장된 대시보드의
    // 자유 끝 연결선이 읽는 순간 사라지고, 그것이 011 REQ-08 이 "가장 나쁜 실패" 로
    // 이름 적은 형상이다.
    const legacy = {
      id: 'c-legacy',
      kind: 'connector',
      from: { el: 'r1', a: 'e' },
      to: { x: 325, y: 380 },
      route: 'straight',
    } as const;
    const parsed = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [{ ...R1 }, { ...legacy }],
    }).elements;
    const line = parsed.find((n) => n.id === 'c-legacy');
    expect(line).toBeDefined();
    expect(isConnector(line!) && line.to).toEqual({ x: 325, y: 380 });
    // 그리고 **해석된다** — 그려지고 잡히는 선이라는 뜻이다.
    expect(resolveConnector(line as never, parsed, PROJ, {})).toBeDefined();
  });
});

// --- AC-59: 만든 것이 선택된다 ---------------------------------------------

describe('만든 것이 곧바로 선택된다 (AC-59)', () => {
  it('그은 연결선 하나만 골라져 있다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(selected()).toEqual([connectors()[0]!.id]);
  });

  it('선택 키가 **평평하다** — 구분자가 없다 (AC-66 · 009 의 선택 모델 불변)', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(selected()[0]).not.toContain('/');
    expect(selected()[0]).toBe(connectors()[0]!.id);
  });

  it('연결선이 골라져도 8핸들이 서지 않는다 (AC-63 의 오늘 잴 수 있는 절반)', () => {
    // 연결선에는 늘릴 상자가 없다(REQ-07). `nodeForKey` 가 연결선 키를 풀지 않으므로
    // 손잡이도 윤곽선도 서지 않는다 — M9 가 제 손잡이를 세울 때까지 그 자리는 비어 있다.
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(screen.queryByTestId('canvas-handle-nw')).toBeNull();
    expect(screen.queryByTestId(`canvas-selection-${connectors()[0]!.id}`)).toBeNull();
  });
});

// --- 같은 앵커에서 놓기 ----------------------------------------------------

describe('누른 앵커에서 그대로 놓으면 **아무것도 만들지 않는다**', () => {
  it('길이 0 인 선이 배열에 남지 않는다', () => {
    // 두 끝이 같은 점인 선은 그려도 보이지 않고 잉크가 없어 잡히지도 않는다 — 배열에만
    // 있고 화면에는 없는 노드는 011 이 REQ-08 에서 "가장 나쁜 실패" 로 이름 적은 형상이다.
    setup();
    enableTool();
    draw(AT.r1East, AT.r1East);
    expect(connectors()).toEqual([]);
  });

  it('선택도 건드리지 않는다 — 몸짓을 그만둔 것이지 무언가를 고른 것이 아니다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r1East);
    expect(selected()).toEqual([]);
  });

  it('같은 **요소**의 다른 앵커는 막지 않는다 — 대각선은 볼 수 있는 선이다', () => {
    setup();
    enableTool();
    draw(AT.r1NorthWest, AT.r1East);
    expect(connectors()).toHaveLength(1);
    expect(connectors()[0]!.from).toEqual({ el: 'r1', a: 'nw' });
    expect(connectors()[0]!.to).toEqual({ el: 'r1', a: 'e' });
  });
});

// --- 앵커가 아닌 누름은 **종전의 뜻 그대로** --------------------------------

describe('도구가 켜져 있어도 앵커가 아닌 누름은 옛 뜻으로 지나간다', () => {
  it('도형 몸통을 누르면 **고르기**다 — 긋기가 시작되지 않는다', () => {
    // M3'b 가 앵커 도구에 대해 정한 형상이다: 도구는 **한 몸짓의 뜻**만 갈아 끼우고
    // 나머지는 건드리지 않는다. 막아 두면 잇는 동안 도형을 옮길 수도 골라 볼 수도 없어,
    // 사용자가 잇기 전후로 도구를 계속 껐다 켜게 된다.
    setup();
    enableTool();
    click(AT.r1Body);
    expect(selected()).toEqual(['r1']);
    expect(connectors()).toEqual([]);
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
  });

  it('몸통을 잡아 끌면 종전대로 **옮겨진다**', () => {
    setup();
    enableTool();
    down(AT.r1Body);
    move({ x: AT.r1Body.x + 20, y: AT.r1Body.y });
    up({ x: AT.r1Body.x + 20, y: AT.r1Body.y });
    expect(connectors()).toEqual([]);
    // 이동은 rAF 로 모이므로 값까지는 여기서 재지 않는다 — 여기서 재는 것은 **몸짓의 뜻**
    // 이 갈리지 않았다는 사실이며, 고른 것이 도형이라는 점이 그것을 말한다.
    expect(selected()).toEqual(['r1']);
  });

  it('빈 자리를 맨손으로 끌면 종전대로 **팬**이다 — 도구가 그 뜻을 갈아 끼우지 않는다', () => {
    // **뒤집힌 시험이다.** 종전 문장: "빈 자리를 끌면 종전대로 **마키**다". 이 절이
    // 지키는 것은 "도구는 **앵커 위 한 몸짓**의 뜻만 갈아 끼우고 나머지는 건드리지
    // 않는다" 이고, 그 문장은 그대로다 — 011 이 뒤집은 것은 **도구 밖의 종전 뜻**이다
    // (빈 자리 맨손 끌기가 사각형에서 팬으로 바뀌었다). 사각형은 Ctrl 로 옮겨 앉았고,
    // 아래 시험이 그 갈래도 도구에 먹히지 않음을 함께 잰다.
    setup();
    enableTool();
    down(AT.empty);
    move({ x: 45, y: 30 });
    expect(screen.queryByTestId('canvas-marquee')).toBeNull();
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
    up({ x: 45, y: 30 });
    expect(connectors()).toEqual([]);
  });

  it('빈 자리를 Ctrl 로 끌면 종전대로 **마키**다', () => {
    setup();
    enableTool();
    const ctrl = { ctrlKey: true };
    fireEvent(overlayRoot(), new MouseEvent('pointerdown', { ...at(AT.empty), ...ctrl }));
    fireEvent(overlayRoot(), new MouseEvent('pointermove', { ...at({ x: 45, y: 30 }), ...ctrl }));
    expect(screen.queryByTestId('canvas-marquee')).not.toBeNull();
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
    fireEvent(overlayRoot(), new MouseEvent('pointerup', { ...at({ x: 45, y: 30 }), ...ctrl }));
    expect(connectors()).toEqual([]);
  });
});

// --- 미리보기 --------------------------------------------------------------

describe('긋는 동안 미리보기가 선다', () => {
  it('누른 뒤 움직이면 뜨고, 놓으면 사라진다', () => {
    setup();
    enableTool();
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
    down(AT.r1East);
    move(AT.r2West);
    expect(screen.queryByTestId('canvas-connector-preview')).not.toBeNull();
    up(AT.r2West);
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
  });

  it('고정된 끝이 **점의 자리**다 — 포인터가 내려앉은 자리가 아니다', () => {
    setup();
    enableTool();
    // 앵커에서 4px 빗나간 자리를 누른다. 오차(9px) 안이므로 집히되, 미리보기는 점에서
    // 시작해야 한다 — 만들어질 선이 점에서 시작하기 때문이다.
    down({ x: AT.r1East.x + 4, y: AT.r1East.y });
    move(AT.r2West);
    const preview = screen.getByTestId('canvas-connector-preview');
    expect(Number.parseFloat(preview.style.left)).toBe(AT.r1East.x);
    expect(Number.parseFloat(preview.style.top)).toBe(AT.r1East.y);
  });

  it('길이가 두 점 사이 거리다', () => {
    setup();
    enableTool();
    down(AT.r1East);
    move({ x: AT.r1East.x + 30, y: AT.r1East.y + 40 });
    expect(screen.getByTestId('canvas-connector-preview').style.width).toBe('50px');
  });

  it('**장식이며 포인터를 먹지 않는다** (위험 R8 가드의 그 문장)', () => {
    // 칠하면서 `aria-hidden` 인 자식은 `pointer-events-none` 이어야 한다. `<div>` 인 것이
    // 그 사실의 절반이다 — `<svg>` 였다면 그 가드의 `instanceof HTMLElement` 에 잡히지
    // 않아 조용히 면제되었을 것이다.
    setup();
    enableTool();
    down(AT.r1East);
    move(AT.r2West);
    const preview = screen.getByTestId('canvas-connector-preview');
    expect(preview).toBeInstanceOf(HTMLElement);
    expect(preview.getAttribute('aria-hidden')).toBe('true');
    expect(preview.className).toContain('pointer-events-none');
  });

  it('취소는 아무것도 만들지 않는다 — 끊긴 몸짓은 "놓았다" 가 아니다', () => {
    setup();
    enableTool();
    down(AT.r1East);
    move(AT.r2West);
    fireEvent(overlayRoot(), new MouseEvent('pointercancel', at(AT.r2West)));
    expect(connectors()).toEqual([]);
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
  });
});

// --- 씨앗 스타일: **층을 건너는** 단언 --------------------------------------
//
// 이 절이 이 파일의 이유다(머리말 참조). 몸짓이 만든 **그 배열**을 렌더 층에 그대로
// 먹이고 캔버스 호출을 본다 — 두 층 사이에 시험이 지은 값이 하나도 끼지 않는다.

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

/** 001 의 기록 스텁(속성 대입도 기록한다 — 그래야 "색을 심었다" 가 재어진다). */
function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  const rec =
    (op: string) =>
    (...args: unknown[]): void => {
      calls.push([op, ...args]);
    };
  let fillStyle: string | CanvasGradient | CanvasPattern = '';
  let strokeStyle: string | CanvasGradient | CanvasPattern = '';
  let lineWidth = 1;
  let globalAlpha = 1;
  let font = '';
  let textAlign: CanvasTextAlign = 'left';
  let textBaseline: CanvasTextBaseline = 'alphabetic';
  return {
    calls,
    save: rec('save'),
    restore: rec('restore'),
    setTransform: rec('setTransform'),
    beginPath: rec('beginPath'),
    rect: rec('rect'),
    ellipse: rec('ellipse'),
    moveTo: rec('moveTo'),
    lineTo: rec('lineTo'),
    closePath: rec('closePath'),
    bezierCurveTo: rec('bezierCurveTo'),
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * 10 }),
    get fillStyle() {
      return fillStyle;
    },
    set fillStyle(v) {
      fillStyle = v;
      calls.push(['fillStyle', v]);
    },
    get strokeStyle() {
      return strokeStyle;
    },
    set strokeStyle(v) {
      strokeStyle = v;
      calls.push(['strokeStyle', v]);
    },
    get lineWidth() {
      return lineWidth;
    },
    set lineWidth(v) {
      lineWidth = v;
      calls.push(['lineWidth', v]);
    },
    get globalAlpha() {
      return globalAlpha;
    },
    set globalAlpha(v) {
      globalAlpha = v;
      calls.push(['globalAlpha', v]);
    },
    get font() {
      return font;
    },
    set font(v) {
      font = v;
      calls.push(['font', v]);
    },
    get textAlign() {
      return textAlign;
    },
    set textAlign(v) {
      textAlign = v;
      calls.push(['textAlign', v]);
    },
    get textBaseline() {
      return textBaseline;
    },
    set textBaseline(v) {
      textBaseline = v;
      calls.push(['textBaseline', v]);
    },
  };
}

/** 배열을 **그대로** 렌더에 먹인다. 스타일 표는 비운다 — 저술만 본다. */
function renderNodes(nodes: readonly CanvasNode[]): Recorder {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  return ctx;
}

/** 몸짓이 방금 만든 그 배열. */
function renderLive(): Recorder {
  return renderNodes(live);
}

function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

describe('방금 그은 선이 **실제로 칠해진다** (SPEC-CANVAS-001 이 배달한 그 함정)', () => {
  it('`stroke` 호출이 난다 — 배열이 늘었다는 것만으로는 부족하다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    // 고정 입력의 도형 둘은 **채우기만** 하므로 `stroke` 를 하나도 내지 않는다. 그래서
    // 이 단언이 참이면 그 호출을 낸 것은 연결선이다.
    expect(ops(renderLive())).toContain('stroke');
  });

  it('색과 **양수 두께**가 둘 다 대입된다 — `paintStroke` 가 둘을 함께 요구한다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    const ctx = renderLive();
    const stroke = ctx.calls.find((c) => c[0] === 'strokeStyle');
    const width = ctx.calls.find((c) => c[0] === 'lineWidth');
    expect(typeof stroke?.[1]).toBe('string');
    expect((stroke?.[1] as string).length).toBeGreaterThan(0);
    expect(width?.[1]).toBeGreaterThan(0);
  });

  it('두 끝이 **참조가 풀린 자리**에 그어진다 — 보이는 선과 저술이 어긋나지 않는다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    const ctx = renderLive();
    const move0 = ctx.calls.find((c) => c[0] === 'moveTo');
    const line0 = ctx.calls.find((c) => c[0] === 'lineTo');
    const from = projectPoint({ x: 300, y: 200 }, PROJ);
    const to = projectPoint({ x: 400, y: 80 }, PROJ);
    expect(move0?.slice(1)).toEqual([from.x, from.y]);
    expect(line0?.slice(1)).toEqual([to.x, to.y]);
  });

  it.each([...CONNECTOR_TOOLS])('%s 로 그은 선도 마찬가지다 — 넷이 같은 씨앗을 입는다', (id) => {
    // 중간점이 없는 동안 넷은 **같은 그림**이다(REQ-04-b · AC-50). 씨앗이 갈리면 그
    // 사실이 거짓이 되고, 그 차이는 "자유선만 안 보인다" 같은 모양으로만 보고된다.
    setup();
    enableTool(id);
    draw(AT.r1East, AT.r2West);
    expect(ops(renderLive())).toContain('stroke');
  });

  it('앵커 둘로 끝난 선이 칠해진다 (016 이 자유 끝 갈래를 걷었다)', () => {
    // 011 은 여기서 **자유 끝으로 끝난 선**을 칠했다. 016 이후 저술 경로는 그런 선을
    // 만들지 않으므로 같은 사실을 앵커 둘로 잰다 — 지키는 것은 "방금 그은 선이 실제로
    // 칠해진다"(001 이 배달한 그 함정)이고, 그 사실은 끝의 종류와 무관하다.
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(ops(renderLive())).toContain('stroke');
  });

  it('그은 선은 **잉크로 잡힌다** — 그린 자리와 잡히는 자리가 같다 (REQ-07-b)', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    const id = connectors()[0]!.id;
    // 두 앵커의 px 중점. 도형 둘의 상자 어디에도 들지 않는 자리다.
    const mid = { x: (120 + 160) / 2, y: (50 + 20) / 2 };
    expect(hitTest(live, mid, PROJ, {})).toEqual({ nodeId: id });
  });
});

// --- AC-31: 앵커를 빼면 연결이 끊긴다 ---------------------------------------
//
// AC-31 은 M3' 가 적어 두고 **오늘 처음 잴 수 있게 된** 조항이다. 그 전에는 연결선을 만드는
// 길이 없어 "그 앵커를 가리키는 연결선" 이라는 전제를 세울 수 없었다.
//
// 재는 것은 넷이다 — 앵커를 뺀 뒤에도 (①) 해석이 `undefined` 이고, (②) 그려지지 않고,
// (③) 잡히지 않고, (④) **config 에서 사라지지 않는다.** 넷째가 이 조항의 무게다:
// 조용히 사라지는 저술이 008 이 이름 적은 가장 나쁜 실패이므로, 그리지 않되 **버리지
// 않는다**(REQ-08).
//
// ## 빼는 몸짓과 빼는 함수를 **나눠서** 잰다
//
// 아래 §빼는 몸짓이 오버레이의 더블클릭으로 실제 제거를 재고, §끊긴 뒤는 그 몸짓이 부르는
// 순수 함수(`removeAnchor`)를 **연결선이 걸린 배열**에 적용해 세 소비자를 잰다. 나눈 것은
// 편의가 아니라 **오늘 관측된 사실** 때문이다 — 아래 §닿지 않는 자리에 그 사실과 근거를
// 적어 두었다.

/** 도형 위 같은 자리를 연달아 두 번 누른다 — 009 의 그 몸짓 그대로다. */
function doubleClick(p: Pt): void {
  click(p);
  click(p);
}

function anchorToolButton(): HTMLElement {
  return screen.getByTestId('canvas-anchor-tool');
}

/** r1 의 `anchors` — 연결선에는 그 필드가 없으므로 그 경우는 던진다. */
function anchorsOfR1(): readonly { id: string; x: number; y: number }[] {
  const host = live.find((n) => n.id === 'r1')!;
  if (isConnector(host)) throw new Error('r1 이 연결선일 수 없다');
  return host.anchors ?? [];
}

describe('빼는 몸짓은 그대로 산다 (REQ-02\'-c · AC-23)', () => {
  it('앵커 도구로 더블클릭하면 더해지고, 한 번 더 하면 빠진다', () => {
    setup();
    fireEvent.click(anchorToolButton());
    doubleClick(AT.r1Body);
    expect(anchorsOfR1()).toHaveLength(1);
    doubleClick(AT.r1Body);
    expect(anchorsOfR1()).toEqual([]);
  });
});

/**
 * r1 에 임의 앵커를 세우고 **그 앵커를 가리키는 연결선**을 그은 배열.
 *
 * 연결선은 진짜 몸짓이 만든 그것이다 — 손으로 지은 노드를 끼우면 M8 이 실제로 무엇을
 * 저장하는지가 이 절의 전제에서 빠진다.
 */
function connectedScene(): { anchorId: string; connectorId: string; nodes: readonly CanvasNode[] } {
  setup();
  fireEvent.click(anchorToolButton());
  doubleClick(AT.r1Body);
  const anchors = anchorsOfR1();
  expect(anchors, 'precondition: 임의 앵커가 하나 섰다').toHaveLength(1);

  fireEvent.click(anchorToolButton()); // 앵커 도구를 끈다
  enableTool();
  draw(AT.r1Body, AT.r2West);
  expect(connectors(), 'precondition: 연결선이 하나 그어졌다').toHaveLength(1);
  expect(connectors()[0]!.from).toEqual({ el: 'r1', a: anchors[0]!.id });

  return { anchorId: anchors[0]!.id, connectorId: connectors()[0]!.id, nodes: [...live] };
}

/** 그 앵커를 뺀 배열 — 몸짓이 부르는 **그 순수 함수**를 지난다. */
function withoutAnchor(nodes: readonly CanvasNode[], anchorId: string): CanvasNode[] {
  return nodes.map((n) => (n.id === 'r1' && !isConnector(n) ? removeAnchor(n, anchorId) : n));
}

/** 그 연결선의 잉크 위 한 점(px) — 두 끝의 중점이다. */
const INK_MID = { x: (AT.r1Body.x + AT.r2West.x) / 2, y: (AT.r1Body.y + AT.r2West.y) / 2 };

describe("임의 앵커를 빼면 그 연결선이 **끊긴 연결**이 된다 (AC-31 · REQ-02'-e · REQ-08)", () => {
  it('빼기 전에는 멀쩡히 해석되고 그려지고 잡힌다 — 켜져 있음', () => {
    const { connectorId, nodes } = connectedScene();
    expect(resolveConnector(connectors()[0]!, nodes, PROJ, {})).toBeDefined();
    expect(ops(renderNodes(nodes))).toContain('stroke');
    expect(hitTest(nodes, INK_MID, PROJ, {})?.nodeId).toBe(connectorId);
  });

  it('① 해석이 `undefined` 다 — 예외가 아니다', () => {
    const { anchorId, nodes } = connectedScene();
    const broken = withoutAnchor(nodes, anchorId);
    // 중심이나 아무 고정 앵커로 떨어뜨리지 **않는다**. 떨어뜨리면 사용자가 그어 둔 선이
    // 예고 없이 다른 자리로 옮겨 앉고, 그 이동은 되돌릴 손잡이가 없다.
    expect(resolveConnector(connectors()[0]!, broken, PROJ, {})).toBeUndefined();
  });

  it('② 그려지지 않는다 — 캔버스 호출이 하나도 나지 않는다 (AC-52)', () => {
    const { anchorId, nodes } = connectedScene();
    // 고정 입력의 도형 둘은 **채우기만** 하므로 이 이름들을 낼 수 있는 것은 연결선뿐이다.
    const drawn = ops(renderNodes(withoutAnchor(nodes, anchorId))).filter((op) =>
      ['moveTo', 'lineTo', 'bezierCurveTo', 'stroke', 'strokeStyle', 'lineWidth'].includes(op),
    );
    expect(drawn).toEqual([]);
  });

  it('③ 잡히지 않는다 — 그리지 않는 것이 잡히면 볼 수 없는 것이 골라진다', () => {
    const { anchorId, nodes } = connectedScene();
    expect(hitTest(withoutAnchor(nodes, anchorId), INK_MID, PROJ, {})).toBeUndefined();
  });

  it('④ **config 에서 사라지지 않는다** — 그리지 않되 버리지 않는다 (REQ-08)', () => {
    const { anchorId, connectorId, nodes } = connectedScene();
    const before = nodes.find((n) => n.id === connectorId)!;
    const broken = withoutAnchor(nodes, anchorId);

    const kept = broken.filter((n): n is ConnectorElement => isConnector(n));
    expect(kept).toHaveLength(1);
    // 끝점도 **한 글자도** 달라지지 않는다.
    expect(kept[0]!).toEqual(before);
    expect(kept[0]!.from).toEqual({ el: 'r1', a: anchorId });
  });

  it('앵커를 **도로 세우면** 다시 그려진다 — 끊김은 회복 가능한 상태다', () => {
    const { anchorId, nodes } = connectedScene();
    const broken = withoutAnchor(nodes, anchorId);
    expect(resolveConnector(connectors()[0]!, broken, PROJ, {})).toBeUndefined();

    const restored = broken.map((n) =>
      n.id === 'r1' && !isConnector(n)
        ? addAnchorAt(n, { x: 250, y: 148 }, anchorId)
        : n,
    );
    expect(resolveConnector(connectors()[0]!, restored, PROJ, {})).toBeDefined();
    expect(ops(renderNodes(restored))).toContain('stroke');
  });
});

// --- 닿지 않던 자리 — **M9 가 열었다** ---------------------------------------
//
// M8 은 이 두 단언을 "오늘 관측된 사실" 로 붙들어 두고, 고치는 자리가 M9 라고 적었다:
// "연결선을 고르고 그 손잡이를 다루는 일이 M9 의 몫이고, 그때 잉크 위의 누름이 무엇을
// 겨누는가가 한 번에 정해진다. 그 결정이 서면 이 단언은 뒤집히며, 뒤집히는 것이 옳다."
//
// **M9 가 그 결정을 세웠다**: 앵커 도구가 켜진 동안 오차 안의 앵커가 잉크를 이긴다
// (`CanvasEditOverlay` §앵커 도구가 켜진 동안 앵커가 잉크를 이긴다). 근거는 M3'b 가
// 이미 세운 문장이다 — 몸짓의 뜻을 가르는 것은 대상이 아니라 **도구**이고, 앵커에 무엇이
// 붙어 있다는 이유로 그 앵커에 닿지 못하는 것은 그 규칙의 구멍이다.
//
// 그래서 절 이름과 첫 단언을 **뒤집는다.** 시험을 지우지 않는 것에 뜻이 있다: 지우면
// "선이 걸린 앵커도 뺄 수 있다" 를 재는 자리가 아무 데도 없어지고, 이 성질은 히트 순서가
// 바뀌는 날 조용히 되돌아간다(009 가 004 의 두 단언에 대해 한 그대로다).

describe('연결선이 걸린 임의 앵커도 **앵커 도구로 뺀다** (M9 가 뒤집은 단언)', () => {
  it('그 자리의 더블클릭은 잉크가 아니라 **앵커**를 겨눈다', () => {
    const { connectorId } = connectedScene();
    fireEvent.click(anchorToolButton());
    // **잉크를 맞힌다는 전제는 그대로 참이다.** 달라진 것은 히트가 아니라 그 히트를
    // 무엇으로 읽는가이며, 그 전제가 깨지면 이 시험은 틀린 이유로 초록이 된다.
    expect(hitTest(live, AT.r1Body, PROJ, {})?.nodeId, 'precondition: 잉크를 맞혔다').toBe(
      connectorId,
    );

    doubleClick(AT.r1Body);
    expect(anchorsOfR1()).toEqual([]);
  });

  it('그래도 **연결선은 버려지지 않는다** — 끊긴 연결이 될 뿐이다 (REQ-08)', () => {
    // 뺐다고 선을 함께 지우지 않는다. 조용히 사라지는 저술이 008 이 이름 적은 가장 나쁜
    // 실패이고, 앵커를 도로 세우면 그 선은 다시 그려진다(위 §끊긴 뒤).
    const { connectorId } = connectedScene();
    fireEvent.click(anchorToolButton());
    doubleClick(AT.r1Body);

    expect(anchorsOfR1()).toEqual([]);
    expect(connectors().map((c) => c.id)).toEqual([connectorId]);
    expect(resolveConnector(connectors()[0]!, live, PROJ, {})).toBeUndefined();
  });

  it('연결선이 없어도 같은 자리에서 그대로 빠진다 — 잉크가 가리지 않던 길도 산다', () => {
    setup();
    fireEvent.click(anchorToolButton());
    doubleClick(AT.r1Body);
    expect(anchorsOfR1()).toHaveLength(1);
    doubleClick(AT.r1Body);
    expect(anchorsOfR1()).toEqual([]);
  });

  it('**고르기 도구에서는 잉크가 종전대로 이긴다** — 우선순위는 도구에 매여 있다', () => {
    // 도구를 켜지 않은 채 같은 자리를 누르면 골라지는 것은 연결선이다. 이 단언이 없으면
    // M9 의 우선순위가 도구와 무관하게 퍼졌는지 알 길이 없고, 그때 연결선을 눌러 고르는
    // 길이 막혀 손잡이 자체가 설 수 없다.
    const { connectorId } = connectedScene();
    // `connectedScene` 은 직선 도구를 켠 채로 끝난다. 끄지 않으면 앵커 위의 누름이 **긋는
    // 몸짓**으로 읽혀 이 시험이 재려는 갈래에 닿지도 않는다.
    fireEvent.click(toolButton('straight'));
    expect(toolButton('straight').getAttribute('aria-pressed'), 'precondition: 도구가 꺼졌다').toBe(
      'false',
    );

    doubleClick(AT.r1Body);
    expect(selected()).toEqual([connectorId]);
    expect(anchorsOfR1()).toHaveLength(1);
  });
});

// --- 저장 왕복 -------------------------------------------------------------

describe('그은 선의 형상이 저술로서 온전하다', () => {
  it('끝점 둘이 **붙은 끝**이다 (016 K1)', () => {
    // 011 은 "두 갈래 중 하나씩" 을 보았다 — 그때는 자유 끝이 저술될 수 있었다. 016 이후
    // 저술 경로가 내는 끝은 **언제나 붙은 끝**이며, 아래 단언은 그 더 좁은 사실을 잰다.
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    const c = connectors()[0]!;
    const ends: ConnectorEnd[] = [c.from, c.to];
    for (const end of ends) {
      const keys = Object.keys(end).sort();
      expect(keys.join(','), JSON.stringify(end)).toBe('a,el');
    }
  });

  it('id 가 배열의 다른 노드와 겹치지 않는다', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    draw(AT.r1NorthWest, AT.r2West);
    const ids = live.map((n) => n.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('그은 것이 배열 **끝**에 온다 — 방금 그은 선이 맨 위다(001 의 z-order)', () => {
    setup();
    enableTool();
    draw(AT.r1East, AT.r2West);
    expect(live[live.length - 1]!.id).toBe(connectors()[0]!.id);
  });
});

// --- 펜 커서 (사용자 신고 2026-09-16) ----------------------------------------

/** 지금 루트가 쓰는 커서. 빈 문자열이면 브라우저 기본이다. */
function cursor(): string {
  return overlayRoot().style.cursor;
}

function isPen(): boolean {
  return cursor().startsWith('url(');
}

function enableAnchorTool(): void {
  const button = screen.getByTestId('canvas-anchor-tool');
  fireEvent.click(button);
  expect(button.getAttribute('aria-pressed'), 'precondition: 앵커 도구가 켜졌다').toBe('true');
}

describe('선을 그을 수 있는 자리에서 커서가 **펜**이 된다', () => {
  // 신고는 둘이었다 — **앵커 위**에서, 그리고 **긋는 동안**. 그 둘이 이 절의 전부다.
  //
  // 커서가 루트에 사는 까닭은 앵커 점이 **표식**이기 때문이다(`ANCHOR_DOT_CLASS` —
  // `pointer-events-none`). 점에서 커서를 내려면 점을 단추로 바꿔야 하고, 그러면 AC-28 이
  // 막는 "더블클릭 판정이 둘" 이 DOM 층에서 되살아난다.

  it('도구가 꺼져 있으면 앵커 위에서도 펜이 아니다', () => {
    // 고르기 도구에서 앵커를 누르면 나는 것은 선이 아니라 고르기다. 그때 펜을 보이면
    // 화면이 지키지 못할 약속을 한다.
    setup();
    move(AT.r1East);
    expect(isPen()).toBe(false);
  });

  it.each(CONNECTOR_TOOLS)('%s 도구에서 앵커 위면 펜이다', (tool) => {
    setup();
    enableTool(tool);
    move(AT.r1East);
    expect(isPen()).toBe(true);
  });

  it('같은 도구라도 앵커가 아닌 자리는 펜이 아니다 — 몸통도 빈 자리도', () => {
    // 도구가 켜져 있어도 앵커 밖의 누름은 **종전의 뜻 그대로**다(고르기 · 마키).
    setup();
    enableTool();
    for (const point of [AT.r1Body, AT.empty]) {
      move(point);
      expect(isPen(), `${point.x},${point.y}`).toBe(false);
    }
  });

  it('앵커를 벗어나면 도로 기본이다 — 켜진 채로 남지 않는다', () => {
    setup();
    enableTool();
    move(AT.r1East);
    expect(isPen(), 'precondition: 앵커 위에서 펜이다').toBe(true);
    move(AT.empty);
    expect(isPen()).toBe(false);
  });

  it('**앵커 도구**에서는 펜이 아니다 — 그 점을 눌러 나는 것은 선이 아니다', () => {
    // 앵커 도구도 앵커를 보여 주지만(`TOOL_SHOWS_ANCHORS`) 그 누름이 만드는 것은 앵커다
    // (REQ-02'). 커서는 `TOOL_CONNECTOR_ROUTE` 를 읽고, 그것은 누름이 잇기를 시작할지
    // 정하는 **그 표**다 — 두 자가 갈리면 보이는 약속과 일어나는 일이 갈린다.
    setup();
    enableAnchorTool();
    expect(dotIds().length, 'precondition: 앵커 점은 보인다').toBeGreaterThan(0);
    move(AT.r1East);
    expect(isPen()).toBe(false);
  });

  it('도구를 끄면 **손을 움직이기 전에** 펜이 사라진다', () => {
    // 판정은 이동에서만 도는데 도구는 단추로 바뀐다. 렌더가 표를 한 번 더 읽지 않으면
    // 끈 뒤에도 손이 움직일 때까지 펜이 남는다.
    setup();
    enableTool();
    move(AT.r1East);
    expect(isPen(), 'precondition: 펜이다').toBe(true);

    fireEvent.click(toolButton('straight'));
    expect(toolButton('straight').getAttribute('aria-pressed')).toBe('false');
    expect(isPen()).toBe(false);
  });
});

describe('긋는 동안에는 커서가 **펜으로 남는다**', () => {
  it('앵커를 벗어나 빈 자리로 끌어도 기본으로 돌아가지 않는다', () => {
    // 이 한 줄이 신고의 둘째 절반이다. 자리를 계속 물으면 앵커를 벗어나는 순간 커서가
    // 바뀌는데, 긋는 중이라는 사실 자체가 "여기는 선을 긋는 자리" 다.
    setup();
    enableTool();
    down(AT.r1East);
    move(AT.empty);
    expect(isPen()).toBe(true);
  });

  it('다른 앵커 위를 지날 때도 그대로다 — 몸짓 안에서 커서가 깜빡이지 않는다', () => {
    setup();
    enableTool();
    down(AT.r1East);
    for (const point of [AT.empty, AT.r2West, AT.r1Body, AT.r2West]) {
      move(point);
      expect(isPen(), `${point.x},${point.y}`).toBe(true);
    }
  });

  it('놓으면 그 자리가 답한다 — 빈 자리에서 끝내도 커서는 기본으로 돌아온다', () => {
    // 016 이후 빈 자리에서 끝내면 **선이 생기지 않는다.** 그래도 커서는 돌아와야 한다 —
    // 몸짓은 끝났고, 커서가 펜으로 남으면 사용자는 아직 긋는 중이라고 읽는다. 011 의
    // 전제("선 하나가 그어졌다")를 그 사실로 바꾼다.
    setup();
    enableTool();
    draw(AT.r1East, AT.empty);
    expect(connectors(), 'precondition: 016 은 빈 자리에서 만들지 않는다').toHaveLength(0);
    expect(isPen()).toBe(false);
  });
});

describe('펜 그림이 지키는 것 셋', () => {
  const penValue = (): string => {
    setup();
    enableTool();
    move(AT.r1East);
    return cursor();
  };

  it('그림이 SVG 데이터 URI 하나다 — 파일을 더 받아 오지 않는다', () => {
    expect(penValue()).toContain('data:image/svg+xml,');
  });

  it('핫스팟이 **펜촉**이다 — 몸통 가운데가 아니다', () => {
    // 펜은 촉으로 긋는다. 가운데를 잡으면 그림과 실제로 집히는 자리가 어긋나고, 그
    // 어긋남은 이 표면이 손잡이 자리에 대해 막아 둔 그것과 같은 종류다(AC-E2).
    const value = penValue();
    const hotspot = value.match(/\)\s+(\d+)\s+(\d+)\s*,/);
    expect(hotspot, '핫스팟 두 수가 url() 뒤에 온다').not.toBeNull();
    const [x, y] = [Number(hotspot![1]), Number(hotspot![2])];
    // 24×24 그림의 왼쪽 아래 — 그 자리에 촉이 있다.
    expect(x).toBeLessThan(6);
    expect(y).toBeGreaterThan(18);
    expect(y).toBeLessThan(24);
  });

  it('그림을 못 그려도 뜻이 남는다 — 낱말 대체가 붙어 있다', () => {
    // SVG 커서를 그리지 못하는 브라우저에서 이 낱말이 없으면 커서가 기본 화살표로
    // 돌아가 아무 말도 하지 않는다.
    expect(penValue().trim()).toMatch(/,\s*crosshair$/);
  });

  it('어느 바탕에서도 보인다 — 칠과 흰 테두리를 함께 든다', () => {
    // 한쪽만 두면 그 반대 바탕에서 커서가 사라지고, 캔버스 바탕은 저술하는 사람이 정한다.
    const value = penValue();
    expect(value, '선택 파랑 칠').toContain('%233b82f6');
    expect(value, '흰 테두리').toContain("stroke='%23ffffff'");
  });
});
