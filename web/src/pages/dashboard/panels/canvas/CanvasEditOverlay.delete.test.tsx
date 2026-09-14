// Delete·Backspace 로 고른 것을 지운다 (SPEC-CANVAS-010).
//
// 사용자가 말한 결함은 "키보드 백스페이스나 del 키로 삭제 안됨" 이었다. 오버레이의 키
// 처리자는 `ARROW_STEPS[event.key]` 를 보고 방향키가 아니면 그대로 돌아섰으므로, 지우는
// 길은 목록 편집기의 휴지통 단추 하나뿐이었다.
//
// ## 이 파일이 지키는 네 갈래
//
//   ① **지운다** — 고른 것들이 배열에서 빠지고, 여럿이어도 **한 번의 방출**이다.
//      N 번 방출하면 중간 배열이 화면에 잠깐 서고, 그 사이 프레임마다 config 가 쓰인다.
//   ② **우리 것이 아닌 키는 소비하지 않는다** — 고른 것이 없을 때 · 끄는 중 · 보조키가
//      붙은 Delete. 아무 일도 하지 않으면서 브라우저의 기본 동작을 막으면 그것이 결함이다
//      (이 저장소가 방향키에 대해 세운 그 규율 그대로다).
//   ③ **글자를 치는 중에는 지우지 않는다**(경로 · D6 ②). 도크는 DOM 상으로 패널 밖이지만
//      포털이라 **React 트리 상으로는 오버레이의 자식**이다 — 도크 안 칸에서 누른 키는
//      실제로 오버레이의 처리자까지 오른다(이 파일이 양성 대조로 그 사실을 함께 잰다).
//   ④ **규칙은 한 곳에 있다** — 목록 편집기의 휴지통과 이 키가 같은 순수 함수를 지난다.
//
// ## 되돌리기가 없다는 사실을 어떻게 다루는가
//
// **확인을 묻지 않는다.** 이 저장소가 이미 세운 판정은 "잃을 것이 없으면 묻지 않는다"
// 이고(`CanvasGroupTools` — 그룹 해제는 **규칙 행이 버려질 때만** 묻는다), 지우기가
// 없애는 것은 화면에 윤곽선이 둘린 **바로 그것들**이라 보이지 않게 잃는 것이 없다.
// 무엇보다 목록 편집기의 휴지통이 이미 묻지 않고 지운다 — 키에만 확인을 달면 "설정에서는
// 물어보는데 목록에서는 그냥 지워진다" 가 되며, 그 형상은 이 저장소가 이름을 붙여 금지한
// 것이다(`CanvasElementsEditor` §이 행이 다시 만들지 않는 것).
//
// @spec SPEC-CANVAS-010

import { useState } from 'react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import { GROUP_LOCAL_EXTENT, type CanvasNode, type GroupElement } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

/**
 * 셋 — 가운데 것이 **배열 한가운데**다. 끝의 것만 지우는 구현(`pop`·`slice(0,-1)`)이
 * 통과하지 못하게 하는 자리다.
 *
 * px 상자(축척 가로 0.4 · 세로 0.25): a = 20..60 × 10..30, b = 100..120 × 40..50,
 * c = 160..240 × 80..130.
 */
const FIXTURE: readonly CanvasElement[] = [
  rect('a', { x: 50, y: 40, w: 100, h: 80 }),
  rect('b', { x: 250, y: 160, w: 50, h: 40 }),
  rect('c', { x: 400, y: 320, w: 200, h: 200 }),
];

/** 각 요소의 중심(화면 px = 스테이지 px, 축척 1 일 때). */
const CENTER = {
  a: { x: 40, y: 20 },
  b: { x: 110, y: 45 },
  c: { x: 200, y: 105 },
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
      <CanvasEditDockRegion enabled>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={PROJ}
          textWidths={{}}
          onElementsChange={onElementsChange}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

function stubOverlayRect(): void {
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

/**
 * 도형을 고른다 — **뗌까지** 마친다. 놓지 않으면 끄는 중이라 키가 닿지 않는다.
 *
 * **고르기 자체가 한 번 방출한다.** 뗌은 넓이 0 짜리 이동을 확정하며 그 확정도 쓰기
 * 통로를 지나기 때문이다(`finishDrag` → `flush`). 이 파일이 재는 것은 **지우기의**
 * 방출이므로 고른 직후에 눈을 닦는다 — 닦지 않으면 `toHaveBeenCalledTimes(1)` 이
 * 고르기의 방출로 만족되어, 지우기에 대해 아무것도 재지 않으면서 초록이 된다.
 */
function pick(emit: ReturnType<typeof vi.fn>, at: { x: number; y: number }, additive = false): void {
  const init: MouseEventInit = { clientX: at.x, clientY: at.y, bubbles: true, cancelable: true };
  if (additive) init.shiftKey = true;
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', init));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', init));
  emit.mockClear();
}

function selectionText(): string {
  return screen.getByTestId('selection').textContent ?? '';
}

/** 방출된 배열의 id 들 — 순서가 뜻을 가지므로 정렬하지 않는다(배열 순서 = z-order). */
function emittedIds(emit: ReturnType<typeof vi.fn>, call = 0): string[] {
  return (emit.mock.calls[call]?.[0] as CanvasNode[]).map((el) => el.id);
}

function setup(elements: readonly CanvasNode[] = FIXTURE): ReturnType<typeof vi.fn> {
  const emit = vi.fn();
  render(<Harness elements={elements} onElementsChange={emit} />);
  stubOverlayRect();
  return emit;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- ① 지운다 -------------------------------------------------------------

describe('고른 것을 지운다 (Delete · Backspace)', () => {
  it.each(['Delete', 'Backspace'])('%s 로 고른 하나가 사라지고 나머지 순서는 그대로다', (key) => {
    const emit = setup();
    pick(emit, CENTER.b); // 배열 한가운데
    expect(selectionText()).toBe('b');

    fireEvent.keyDown(overlayRoot(), { key });

    expect(emit).toHaveBeenCalledTimes(1);
    expect(emittedIds(emit)).toEqual(['a', 'c']);
  });

  it('여럿을 골라도 **한 번의 방출**이다 — N 번이면 중간 배열이 화면에 선다', () => {
    const emit = setup();
    pick(emit, CENTER.a);
    pick(emit, CENTER.c, true);
    expect(selectionText().split(',').sort()).toEqual(['a', 'c']);

    fireEvent.keyDown(overlayRoot(), { key: 'Delete' });

    expect(emit).toHaveBeenCalledTimes(1);
    expect(emittedIds(emit)).toEqual(['b']);
  });

  it('지운 뒤 선택이 **빈다** — 없는 것을 가리키는 선택은 단추를 거짓으로 켠다', () => {
    const emit = setup();
    pick(emit, CENTER.a);

    fireEvent.keyDown(overlayRoot(), { key: 'Delete' });

    expect(emit).toHaveBeenCalledTimes(1);
    expect(selectionText()).toBe('');
  });

  it('그룹을 지우면 **부품도 함께 간다** — 배열 항목 하나가 곧 그 무리다', () => {
    // SPEC-CANVAS-004 0.4.0 이 일부러 고른 형상이다. 부품은 최상위 배열이 아니라 그룹
    // 항목 **안**에 살므로, 항목을 빼는 것 말고 따로 할 일이 없다.
    //
    // **고르는 몸짓은 004 와 같다**(SPEC-CANVAS-009 0.3.0). 009 0.2.0 은 부품 위의
    // 누름을 부품 선택에 걸어 그룹을 클릭으로 고를 길을 없앴고, 0.3.0 이 그것을 그림
    // 도구의 관용구로 되돌렸다 — **단일 클릭은 그룹, 더블클릭은 그 안.** 그래서 이
    // 시험은 004 때처럼 클릭 한 번으로 그룹을 고른다. 아래 §더블클릭이 진입 쪽을 잰다.
    const group: GroupElement = {
      id: 'g',
      kind: 'group',
      geometry: { x: 50, y: 40, w: 100, h: 80 },
      // 부품 기하는 **그룹 로컬 격자**(0..`GROUP_LOCAL_EXTENT` = 10000)다. 100×100 으로
      // 두면 상자의 1% 만 차지해 아래 중심 좌표가 잉크를 빗나가고, 이 시험은 "그룹이
      // 골라지지 않아서" 빨개진다 — 재려던 것과 다른 이유다.
      parts: [rect('p1', { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT })],
    };
    const emit = setup([group, rect('z', { x: 400, y: 320, w: 100, h: 60 })]);
    pick(emit, CENTER.a); // 그룹의 상자는 `a` 와 같은 자리다
    expect(selectionText()).toBe('g');

    fireEvent.keyDown(overlayRoot(), { key: 'Delete' });

    expect(emittedIds(emit)).toEqual(['z']);
  });

  it('더블클릭으로 들어가면 **부품**이 골라지고 Delete 는 아무것도 지우지 않는다 (SPEC-CANVAS-009 0.3.0)', () => {
    // 단일 클릭은 그룹을 고르고(위 시험), **더블클릭**이 그 안으로 들어간다. 들어간
    // 뒤의 Delete 는 **무동작**이다 — 부품 삭제는 009 의 범위가 아니며(빼는 길은 분리
    // 단추다), 조용히 그룹째 지우는 것은 사용자가 가리킨 것과 다른 것을 없애는 일이다.
    const group: GroupElement = {
      id: 'g',
      kind: 'group',
      geometry: { x: 50, y: 40, w: 100, h: 80 },
      parts: [rect('p1', { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT })],
    };
    const emit = setup([group, rect('z', { x: 400, y: 320, w: 100, h: 60 })]);
    pick(emit, CENTER.a);
    expect(selectionText(), '전제 — 첫 누름은 그룹을 고른다').toBe('g');
    pick(emit, CENTER.a); // 두 번째 누름 = 진입
    expect(selectionText()).toBe('g/p1');

    fireEvent.keyDown(overlayRoot(), { key: 'Delete' });

    expect(emit).not.toHaveBeenCalled();
  });

  it('소비한다 — Backspace 의 뒤로 가기가 그림을 지우면서 화면까지 떠나면 안 된다', () => {
    const emit = setup();
    pick(emit, CENTER.a);

    const evt = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true });
    fireEvent(overlayRoot(), evt);

    expect(evt.defaultPrevented).toBe(true);
  });
});

describe('지우기는 프레임을 예약하지 않는다 (REQ-05 · AC-E4)', () => {
  it('지우는 키가 rAF 를 **0 건** 요청한다 — 쓰기는 한 번이고 그 뒤로 루프가 없다', () => {
    // 방향키가 이미 이 규율을 따른다(합류 프레임은 **포인터** 이동이 쓰기를 모을 때만
    // 쓰인다). 지우기가 그 통로(`pendingRef`·`frameRef`)를 빌리면 쓸 것이 없는데도
    // 루프가 돌고, 그 결함은 화면이 아니라 전력계에만 드러난다.
    const emit = setup();
    pick(emit, CENTER.a);
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    fireEvent.keyDown(overlayRoot(), { key: 'Delete' });

    expect(emit).toHaveBeenCalledTimes(1);
    expect(raf).not.toHaveBeenCalled();
  });
});

// --- ② 우리 것이 아닌 키는 소비하지 않는다 --------------------------------

describe('우리 것이 아니면 소비하지 않는다 (방향키가 세운 그 규율)', () => {
  it('고른 것이 없으면 방출도 없고 소비도 없다', () => {
    const emit = setup();

    const evt = new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true });
    fireEvent(overlayRoot(), evt);

    expect(emit).not.toHaveBeenCalled();
    expect(evt.defaultPrevented).toBe(false);
  });

  it('끌고 있는 동안에는 **손이 이긴다** — 놓기 전에는 지우지 않는다', () => {
    const emit = setup();
    pick(emit, CENTER.a);
    // 다시 잡되 놓지 않는다.
    fireEvent(
      overlayRoot(),
      new MouseEvent('pointerdown', { clientX: CENTER.a.x, clientY: CENTER.a.y, bubbles: true, cancelable: true }),
    );

    const evt = new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true });
    fireEvent(overlayRoot(), evt);

    expect(emit).not.toHaveBeenCalled();
    expect(evt.defaultPrevented).toBe(false);
  });

  it('선택이 **이미 지워진 것만** 가리키면 방출도 소비도 없다', () => {
    // 이 갈래가 닿는 상태임을 먼저 만든다. 캔버스에서 고른 뒤 **목록 편집기에서** 그 행을
    // 지우면 선택에는 id 가 남고 배열에는 없다 — 이 파일의 다른 하네스는 배열이 고정이라
    // 그 상태를 만들 수 없어, 그대로 두면 `deleteSelection` 의 거짓 갈래가 한 번도 닿지
    // 않는 **검증되지 않은 가드**로 남는다.
    const emit = vi.fn();
    function Shrinking() {
      const [els, setEls] = useState<readonly CanvasNode[]>(FIXTURE);
      const state = useCanvasEditSelectionState();
      return (
        <CanvasEditSelectionContext value={state}>
          <span data-testid="selection">{[...state.selection].join(',')}</span>
          <button
            type="button"
            data-testid="drop-a"
            onClick={() => setEls(FIXTURE.filter((el) => el.id !== 'a'))}
          >
            drop
          </button>
          <CanvasEditDockRegion enabled>
            <CanvasEditOverlay
              enabled
              elements={els}
              projection={PROJ}
              textWidths={{}}
              onElementsChange={emit}
            />
          </CanvasEditDockRegion>
        </CanvasEditSelectionContext>
      );
    }
    render(<Shrinking />);
    stubOverlayRect();
    pick(emit, CENTER.a);
    fireEvent.click(screen.getByTestId('drop-a'));
    // 재려는 상태가 실제로 섰는지 먼저 단언한다 — 선택은 남고 배열에는 없다.
    expect(selectionText()).toBe('a');

    const evt = new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true });
    fireEvent(overlayRoot(), evt);

    expect(emit).not.toHaveBeenCalled();
    expect(evt.defaultPrevented).toBe(false);
  });

  it.each([
    ['Ctrl', { ctrlKey: true }],
    ['Cmd', { metaKey: true }],
    ['Alt', { altKey: true }],
  ])('%s+Delete 는 브라우저·OS 의 조작이다', (_name, mods) => {
    const emit = setup();
    pick(emit, CENTER.a);

    const evt = new KeyboardEvent('keydown', {
      key: 'Delete',
      bubbles: true,
      cancelable: true,
      ...mods,
    });
    fireEvent(overlayRoot(), evt);

    expect(emit).not.toHaveBeenCalled();
    expect(evt.defaultPrevented).toBe(false);
  });

  it('Shift+Delete 는 **우리 것이다** — 방향키의 Shift 와 달리 더 셀 격자가 없다', () => {
    // Shift 를 배제하면 Shift 를 짚은 채로 누른 Delete 가 조용히 아무 일도 하지 않는다.
    // 보조키 셋(Ctrl·Cmd·Alt)만 빼는 것은 방향키가 이미 쓰는 그 갈래다.
    const emit = setup();
    pick(emit, CENTER.a);

    fireEvent.keyDown(overlayRoot(), { key: 'Delete', shiftKey: true });

    expect(emittedIds(emit)).toEqual(['b', 'c']);
  });
});

// --- ③ 글자를 치는 중에는 지우지 않는다 (경로 · D6 ②) ---------------------

describe('도크 안에서 친 글자가 그림을 지우지 않는다 (포털 · D6 ②)', () => {
  it('도크는 **React 트리 상으로 오버레이의 자식**이라 키가 실제로 오른다 (양성 대조)', () => {
    // 이 대조가 없으면 아래 가드는 "닿지도 않는 것을 막았다" 로 초록일 수 있다.
    // 도크는 DOM 상으로 패널 **밖**이므로 네이티브 버블링으로는 오지 않는다 — 오는 길은
    // React 의 포털 전파 하나뿐이고, 그 길이 살아 있다는 것이 여기서 값으로 남는다.
    const emit = setup();
    pick(emit, CENTER.a);
    const dock = screen.getByTestId('canvas-dock-panel');
    expect(overlayRoot().contains(dock)).toBe(false); // DOM 상으로는 밖이다

    // 같은 키를 **루트**에 쏘면 지워진다 — 키 자체는 살아 있다.
    fireEvent.keyDown(overlayRoot(), { key: 'Backspace' });
    expect(emit).toHaveBeenCalledTimes(1);
  });

  it('도크의 글자 칸에서 누른 Backspace 는 그림을 건드리지 않는다', () => {
    const emit = setup();
    pick(emit, CENTER.a);

    fireEvent.keyDown(screen.getByTestId('canvas-dock-panel'), { key: 'Backspace' });

    expect(emit).not.toHaveBeenCalled();
    expect(selectionText()).toBe('a');
  });

  it('도크에서 누른 **방향키**도 요소를 옮기지 않는다 (같은 한 줄이 막는다)', () => {
    // 010 이전부터 열려 있던 자리다: 붙임 간격을 적으려고 수치 칸에서 방향키를 누르면
    // 수는 그대로이고 **요소가 움직였다**. 떠 있는 배율 줄은 같은 한 줄로 이미 막고 있었고
    // (`onKeyDown` 에서 `stopPropagation`), 도크에는 그 줄이 없었다.
    const emit = setup();
    pick(emit, CENTER.a);

    // 붙임을 켜 수치 칸을 **살린다** — 꺼진 칸에 쏜 시험은 엉뚱한 이유로 초록이다.
    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
    const field = screen.getByTestId('canvas-grid-step') as HTMLInputElement;
    expect(field.disabled).toBe(false);

    fireEvent.keyDown(field, { key: 'ArrowUp' });

    expect(emit).not.toHaveBeenCalled();
  });
});

// --- 경로 (D6 ②) — 처리자 없는 닿는 면 -----------------------------------

describe('닿는 면에서 누른 키도 **버블링으로** 루트에 닿는다 (D6 ②)', () => {
  it('처리자를 달지 않은 노드의 Delete 가 루트의 처리자까지 오른다', () => {
    const emit = setup();
    pick(emit, CENTER.a);

    fireEvent.keyDown(screen.getByTestId('canvas-workspace-hit'), { key: 'Delete' });

    expect(emittedIds(emit)).toEqual(['b', 'c']);
  });
});

// --- 구조 (D6 ①) ---------------------------------------------------------

describe('보조기기가 이 키를 **알 수 있다** (D6 ①)', () => {
  it('`aria-keyshortcuts` 가 Delete 와 Backspace 를 알린다', () => {
    setup();

    const keys = overlayRoot().getAttribute('aria-keyshortcuts') ?? '';

    expect(keys.split(/\s+/)).toContain('Delete');
    expect(keys.split(/\s+/)).toContain('Backspace');
    // 방향키가 밀려나지 않았다 — 더한 것이지 갈아 끼운 것이 아니다.
    expect(keys.split(/\s+/)).toContain('ArrowUp');
  });

  it('설명 문단이 지우기를 말한다 — 그 몸짓이 닿는 유일한 통로다', () => {
    setup();

    const id = overlayRoot().getAttribute('aria-describedby');
    const node = document.getElementById(id!);

    expect(node!.textContent).toContain('dashboard.canvas.edit.deleteHint');
  });
});

// --- ④ 규칙은 한 곳에 있다 ------------------------------------------------

describe('지우는 규칙은 **한 함수**다 (목록 편집기의 휴지통과 같은 길)', () => {
  it('두 자리 모두 그 함수를 들이고 **실제로 부른다**', () => {
    // 들여왔는지만 보면 인라인으로 되돌리고 import 를 남겨 두는 형태를 놓친다. 이웃한
    // 순서 이동 가드는 그 구멍을 eslint(`no-unused-vars`, error)에 맡겼고 실제로 잡히지만
    // — 확인했다 — 그러면 **이 시험 자체는 아무것도 말하지 못한다**. 부르는 자리까지
    // 보면 시험 하나가 제 힘으로 문을 막는다(돌연변이로 확인했다).
    for (const name of ['CanvasEditOverlay.tsx', 'CanvasElementsEditor.tsx']) {
      const source = readFileSync(join(__dirname, name), 'utf-8');
      expect(source, `${name}: 들여오기`).toMatch(/removeNodes[,\s}]/);
      expect(source, `${name}: 부르는 자리`).toMatch(/removeNodes\(/);
    }
  });

  it('배열에서 직접 빼는 두 번째 규칙이 없다', () => {
    // 이 가드가 지키는 것은 "지우는 형상이 하나" 이며, `moveElementTo` 가 순서 이동에 대해
    // 세운 그 규율과 같은 자다. 주석은 걷어내고 **코드만** 본다.
    for (const name of ['CanvasEditOverlay.tsx', 'CanvasElementsEditor.tsx']) {
      const code = readFileSync(join(__dirname, name), 'utf-8')
        .split('\n')
        .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
        .join('\n');
      expect(code, name).not.toMatch(/\.filter\(\([^)]*\)\s*=>\s*i\s*!==/);
    }
  });
});

// --- i18n -----------------------------------------------------------------

describe('안내 문구가 두 언어에 있고 짝이 맞는다', () => {
  function editNs(messages: unknown): Record<string, unknown> {
    const root = messages as Record<string, Record<string, Record<string, unknown>>>;
    return root['dashboard']!['canvas']!['edit'] as Record<string, unknown>;
  }

  /** 문구에 든 치환자들. 순서는 뜻이 없고 **개수**가 뜻이 있다. */
  function placeholders(text: string): string[] {
    return [...text.matchAll(/\{[a-zA-Z]+\}/g)].map((m) => m[0]).sort();
  }

  it('`deleteHint` 가 ko·en 양쪽에 있고 서로 다른 문구다', () => {
    const koText = editNs(ko)['deleteHint'];
    const enText = editNs(en)['deleteHint'];

    expect(typeof koText, 'ko').toBe('string');
    expect(typeof enText, 'en').toBe('string');
    expect((koText as string).length).toBeGreaterThan(0);
    expect(koText).not.toBe(enText);
  });

  it('치환자 **다중집합**이 두 언어에서 같다 — 기본 로케일이 ko 라 마운트만으로는 못 잰다', () => {
    expect(placeholders(editNs(ko)['deleteHint'] as string), 'ko.deleteHint').toEqual(
      placeholders(editNs(en)['deleteHint'] as string),
    );
  });

  it('키 이름에 점이 없다 — 이름에 점이 든 키는 어떤 조회 경로로도 닿지 않는다', () => {
    for (const [name, messages] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      for (const key of Object.keys(editNs(messages))) {
        expect(key.includes('.'), `${name}.edit: 이름에 점이 든 키 "${key}"`).toBe(false);
      }
    }
  });

  it('한국어 문구가 금지 어휘를 쓰지 않고 사용자의 낱말로 말한다', () => {
    const text = editNs(ko)['deleteHint'] as string;

    for (const word of ['문구', '트윈', '이징']) {
      expect(text.includes(word), `금지 어휘 "${word}"`).toBe(false);
    }
    // 몸짓을 실제로 설명한다 — 두 키 이름이 다 들어 있어야 한다.
    expect(text).toContain('Delete');
    expect(text).toContain('Backspace');
  });
});

// 고정 입력의 산술이 투영과 맞는지 한 자리에서 못박는다(오타 방어).
describe('고정 입력의 산술', () => {
  it('세 중심이 저마다 다른 요소를 맞힌다', () => {
    expect(PROJ.stage.width / PROJ.canvas.width).toBe(0.4);
    expect(PROJ.stage.height / PROJ.canvas.height).toBe(0.25);
    const emit = setup();
    pick(emit, CENTER.a);
    expect(selectionText()).toBe('a');
    pick(emit, CENTER.b);
    expect(selectionText()).toBe('b');
    pick(emit, CENTER.c);
    expect(selectionText()).toBe('c');
    // `pick` 이 눈을 닦았으므로 여기 남은 방출은 없다 — 고르기는 기하를 바꾸지 않는다.
    expect(emit).not.toHaveBeenCalled();
  });
});
