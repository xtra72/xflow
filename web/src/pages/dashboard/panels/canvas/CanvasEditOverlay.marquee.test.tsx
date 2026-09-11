// 영역 선택(마키)이 **실제 몸짓으로 닿는가** (SPEC-CANVAS-009).
//
// `canvasMarquee.test.ts` 가 사각형 산술과 합집합을 값으로 못박았으므로, 이 파일이 재는
// 것은 그 값이 **오버레이의 포인터 경로를 타고 화면에 닿는가** 다. 둘을 한 파일에 두지
// 않는 이유는 시험 규율 D6 그 자체다 — 순수 산술은 jsdom 없이도 참이고, 닿음은 산술이
// 참이어도 거짓일 수 있다(006 M7 의 저술 여백이 그 증거다).
//
// ## 이 파일이 지키는 세 갈래
//
//   ① **경로**(D6 ②) — 처리자를 달지 않은 노드(`canvas-workspace-hit`)에 쏜 누름이
//      **진짜 버블링**으로 루트 처리자에 닿아 사각형을 세운다. jsdom 에서도 버블링은 진짜다.
//   ② **구조**(D6 ①) — 선 사각형의 상자가 계산값과 같고, `pointer-events-none` 이며,
//      루트의 **직계 자식**이다. "브라우저가 이것을 어떻게 칠할 것인가" 는 jsdom 이 답할 수
//      없으므로 그 답이 참일 **조건들**을 대신 못박는다. ①과 ②를 한 시험이 겸하지 않는다.
//   ③ **몸짓의 소유권** — 맨손 빈 지점 누름이 **여전히 소비되지 않는다**. 이것이 무너지면
//      `previewPan` 의 화면 이동이 통째로 죽는다(previewPan.ts §몸짓의 소유권 2).
//
// ## 고정 입력을 고른 방식
//
// 스테이지 200×100 · 캔버스 500×400 이므로 투영 축척은 **가로 0.4 · 세로 0.25** 로 갈린다
// (1:1 이면 캔버스 크기를 무시한 투영도 통과한다). 요소 넷의 px 상자를 수로 적는다.
//
//   | id   | 캔버스              | px 상자            | 사각형 (10,5)-(130,60) 과의 관계 |
//   |------|---------------------|--------------------|----------------------------------|
//   | a    | 50,40,100,80        | x 20..60  y 10..30 | **온전히 안**                    |
//   | c    | 400,320,200,200     | x 160..240 y 80..130| 완전히 밖                       |
//   | edge | 300,100,150,40      | x 120..180 y 25..35 | **변에 걸침**(오른쪽 50px 넘침)  |
//   | b    | 250,160,50,40       | x 100..120 y 40..50 | **온전히 안**                    |
//
// 네 함정을 피한 자리를 적어 둔다: 사각형이 **전부를 덮지 않고**(c·edge 가 빠진다),
// **아무것도 덮지 않지도 않으며**(a·b 가 든다), 크기가 **넷 다 다르고**(40×20 · 80×50 ·
// 60×10 · 20×10), 배열 한가운데(`c`)에 바깥 것이 있다. 네 방향 드래그는 §되돌려 끌기가 잰다.

import { useState } from 'react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

// i18n 은 키를 그대로 돌려준다(이웃 파일의 관용구 — 문구 자체는 아래 §i18n 이 JSON 으로 잰다).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import type { CanvasNode, GroupElement } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

function rect(id: string, geometry: BoxGeometry, visible?: boolean): CanvasElement {
  return { id, kind: 'rect', geometry, style: visible === undefined ? {} : { visible } };
}

/** 머리말의 표 그대로. `c`(밖의 것)가 배열 **한가운데**다. */
const FIXTURE: readonly CanvasElement[] = [
  rect('a', { x: 50, y: 40, w: 100, h: 80 }),
  rect('c', { x: 400, y: 320, w: 200, h: 200 }),
  rect('edge', { x: 300, y: 100, w: 150, h: 40 }),
  rect('b', { x: 250, y: 160, w: 50, h: 40 }),
];

/** 사각형의 두 모서리(스테이지 로컬 px = 화면 px, 축척 1 일 때). */
const FROM = { x: 10, y: 5 };
const TO = { x: 130, y: 60 };

/**
 * 어느 요소의 **집기 여유 상자에도 들지 않는** 자리. 여유는 사방 6px 이므로 네 요소가
 * 실제로 차지하는 면은 a(14..66, 4..36) · c(154..246, 74..136) · edge(114..186, 19..41) ·
 * b(94..126, 34..56) 이고, 아래 자리는 그 넷 밖이다. 눈으로 "멀어 보이는" 자리를 고르면
 * 여유 상자에 걸려 빈 지점 시험이 **엉뚱한 이유로** 빨개진다(실제로 한 번 그랬다).
 */
const EMPTY = { x: 5, y: 95 };

// --- 하네스 ---------------------------------------------------------------

interface HarnessProps {
  enabled?: boolean;
  elements: readonly CanvasNode[];
  stage?: StageSize;
  onElementsChange?: (next: CanvasNode[]) => void;
  onParentDown?: () => void;
}

function Harness({
  enabled = true,
  elements,
  stage = STAGE,
  onElementsChange = vi.fn(),
  onParentDown,
}: HarnessProps) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <div data-testid="parent" onPointerDown={onParentDown} className="relative">
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled={enabled}
            elements={elements}
            projection={{ stage, canvas: CANVAS }}
            textWidths={{}}
            onElementsChange={onElementsChange}
          />
        </CanvasEditDockRegion>
      </div>
    </CanvasEditSelectionContext>
  );
}

/**
 * 편집을 껐다 **다시 켤 수 있는** 하네스 — `enabled` 전이를 재는 자리에서만 쓴다.
 *
 * 다시 켜는 단추가 있는 것에 뜻이 있다. 꺼진 동안에는 오버레이가 `null` 을 돌려주므로
 * (`if (!enabled) return null`) 사각형이 DOM 에 없다는 사실만으로는 **상태가 거뒀는지**를
 * 알 수 없다 — 남아 있는 상태는 다시 켜는 순간에만 드러난다.
 */
function ToggleHarness({ elements }: { elements: readonly CanvasNode[] }) {
  const [enabled, setEnabled] = useState(true);
  return (
    <>
      <button type="button" data-testid="disable" onClick={() => setEnabled(false)}>
        off
      </button>
      <button type="button" data-testid="enable" onClick={() => setEnabled(true)}>
        on
      </button>
      <Harness enabled={enabled} elements={elements} />
    </>
  );
}

// --- 포인터 관용구 --------------------------------------------------------

/** jsdom 에 `PointerEvent` 가 없으므로 `MouseEvent` 로 만들고 타입만 포인터로 둔다. */
function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

/** 오버레이 루트에 직접 쏜다. 돌려받은 이벤트로 소비 여부를 잰다. */
function send(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  const evt = pointer(type, x, y, init);
  fireEvent(overlayRoot(), evt);
  return evt;
}

/** 처리자를 달지 **않은** 노드에 쏜다 — 도달은 오직 버블링으로만 이뤄진다(D6 ②). */
function sendToHit(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  const evt = pointer(type, x, y, init);
  fireEvent(screen.getByTestId('canvas-workspace-hit'), evt);
  return evt;
}

function stubOverlayRect(left = 0, top = 0, width = 200, height = 100): void {
  vi.spyOn(overlayRoot(), 'getBoundingClientRect').mockReturnValue({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect);
}

function selectionText(): string {
  return screen.getByTestId('selection').textContent ?? '';
}

/** 골라진 것들을 **순서 무관**으로 본다 — 선택은 Set 이라 순서에 뜻이 없다. */
function selected(): string[] {
  const text = selectionText();
  return text === '' ? [] : text.split(',').sort();
}

function marqueeEl(): HTMLElement | null {
  return screen.queryByTestId('canvas-marquee');
}

/** Shift 를 누른 채 사각형을 긋는다. 뗌은 부르는 쪽이 정한다. */
function dragMarquee(
  from: { x: number; y: number },
  ...points: readonly { x: number; y: number }[]
): void {
  send('pointerdown', from.x, from.y, { shiftKey: true });
  for (const p of points) send('pointermove', p.x, p.y, { shiftKey: true });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- ③ 몸짓의 소유권 (previewPan 이 살아 있다) ----------------------------

describe('맨손 빈 지점 누름은 종전 그대로 흘러간다 (AC-E3 · previewPan §몸짓의 소유권)', () => {
  it('맨손이면 선택만 비우고 **소비하지 않는다** — 이 한 줄이 화면 이동의 전제다', () => {
    const parent = vi.fn();
    render(<Harness elements={FIXTURE} onParentDown={parent} />);
    stubOverlayRect();
    send('pointerdown', 40, 20); // `a` 를 고른다
    send('pointerup', 40, 20);
    expect(selected()).toEqual(['a']);
    parent.mockClear();

    const evt = send('pointerdown', EMPTY.x, EMPTY.y); // 빈 자리

    expect(selected()).toEqual([]);
    // `previewPan.onPointerDown` 은 `defaultPrevented` 로 몸짓의 임자를 가린다.
    expect(evt.defaultPrevented).toBe(false);
    expect(parent).toHaveBeenCalledTimes(1);
    expect(marqueeEl()).toBeNull();
  });

  it('맨손 빈 지점에서 끌어도 사각형이 서지 않는다 — 그 몸짓은 위층의 것이다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    send('pointerdown', FROM.x, FROM.y);
    send('pointermove', TO.x, TO.y);

    expect(marqueeEl()).toBeNull();
    expect(selected()).toEqual([]);
  });

  it('Shift 로 시작하면 **우리 것이다** — 소비하고 위층에 닿지 않는다', () => {
    const parent = vi.fn();
    render(<Harness elements={FIXTURE} onParentDown={parent} />);
    stubOverlayRect();

    const evt = send('pointerdown', FROM.x, FROM.y, { shiftKey: true });

    expect(evt.defaultPrevented).toBe(true);
    expect(parent).not.toHaveBeenCalled();
  });

  // 셋을 **모두** 잰다. 하나만 재면 `event.shiftKey` 한 갈래만 읽는 구현이 통과한다 —
  // 히트 경로의 `additive` 는 처음부터 셋을 같은 자로 보았고, 여기서 갈라지면 "빈 자리에서는
  // Cmd 가 듣지 않는다" 가 된다.
  it.each([
    ['Shift', { shiftKey: true }],
    ['Ctrl', { ctrlKey: true }],
    ['Cmd', { metaKey: true }],
  ])('%s 로도 사각형이 선다 — 히트 경로의 `additive` 와 한 낱말이다', (_name, init) => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    send('pointerdown', FROM.x, FROM.y, init);
    send('pointermove', TO.x, TO.y, init);

    expect(marqueeEl()).not.toBeNull();
    expect(selected()).toEqual(['a', 'b']);
  });
});

// --- ② 구조 (D6 ①) -------------------------------------------------------

describe('선 사각형의 **형상** — 브라우저가 옳게 칠할 조건들 (D6 ①)', () => {
  it('상자가 두 모서리에서 나온 값과 같고, 장식이며, 포인터를 먹지 않는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    const el = marqueeEl();
    expect(el).not.toBeNull();
    expect(el!.style.left).toBe('10px');
    expect(el!.style.top).toBe('5px');
    expect(el!.style.width).toBe('120px');
    expect(el!.style.height).toBe('55px');
    // 장식이므로 이름이 없고, 위험 R8 의 가드가 지키는 그 문장을 만족한다.
    expect(el!.getAttribute('aria-hidden')).toBe('true');
    expect(el!.className).toContain('pointer-events-none');
  });

  it('루트의 **직계 자식**이다 — 좌표 기준이 선택 윤곽선과 같은 상자여야 한다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(marqueeEl()!.parentElement).toBe(overlayRoot());
    // 선택 윤곽선과 **같은 좌표 공간**이라는 것을 값으로 잇는다: 둘 다 스테이지 로컬 px 을
    // `left`/`top` 에 그대로 쓴다. `a` 의 px 상자는 (20,10,40,20) 이다.
    expect(screen.getByTestId('canvas-selection-a').style.left).toBe('20px');
  });

  it('되돌려 끌어도 같은 상자다 — 음수 크기를 화면에 내보내지 않는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(TO, FROM);

    const el = marqueeEl()!;
    expect(el.style.left).toBe('10px');
    expect(el.style.top).toBe('5px');
    expect(el.style.width).toBe('120px');
    expect(el.style.height).toBe('55px');
  });

  it('손을 떼면 사라지고, 취소해도 사라진다 — 남으면 조작할 수 없는 그림이 된다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);
    expect(marqueeEl()).not.toBeNull();
    send('pointerup', TO.x, TO.y);
    expect(marqueeEl()).toBeNull();
    // 고른 것은 **남는다** — 뗌이 몸짓의 결과를 무르지 않는다.
    expect(selected()).toEqual(['a', 'b']);

    dragMarquee(FROM, TO);
    send('pointercancel', TO.x, TO.y);
    expect(marqueeEl()).toBeNull();
    // 취소도 마지막 유효 결과를 확정한다(이동 드래그와 같은 규칙).
    expect(selected()).toEqual(['a', 'b']);
  });

  it('편집이 꺼지면 진행 중이던 사각형도 함께 거둔다', () => {
    render(<ToggleHarness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);
    expect(marqueeEl()).not.toBeNull();

    fireEvent.click(screen.getByTestId('disable'));

    expect(marqueeEl()).toBeNull();
    expect(selected()).toEqual([]);
  });

  it('다시 켜도 **옛 사각형이 되살아나지 않는다** — 상태까지 거둬야 참이다', () => {
    // 위 시험만으로는 모자라다: 꺼진 동안 오버레이는 `null` 을 돌려주므로 상태를 그대로
    // 들고 있어도 사각형이 보이지 않는다. 거두지 않은 상태는 **다시 켜는 순간** 조작할 수
    // 없는 그림으로 되살아나고, 그 결함은 편집을 껐다 켠 사람에게만 보인다.
    render(<ToggleHarness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);
    fireEvent.click(screen.getByTestId('disable'));

    fireEvent.click(screen.getByTestId('enable'));

    expect(marqueeEl()).toBeNull();
  });
});

// --- ① 경로 (D6 ②) -------------------------------------------------------

describe('저술 여백에서 시작한 사각형도 선다 (D6 ② · 006 M7 이 연 그 면)', () => {
  it('처리자를 달지 않은 닿는 면의 누름이 **버블링으로** 루트에 닿아 사각형을 세운다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    // 쏘는 자리는 루트가 **아니다** — 006 M7 이 고친 결함은 이 노드가 없을 때 누름이
    // 영영 닿지 않는 것이었다. 루트에 직접 쏜 시험은 닿음에 대해 아무것도 말하지 못한다.
    const evt = sendToHit('pointerdown', FROM.x, FROM.y, { shiftKey: true });

    expect(evt.defaultPrevented).toBe(true);
    expect(marqueeEl()).not.toBeNull();
  });

  it('사각형을 그은 뒤 루트가 **초점을 든다** — 그래야 방향키로 이어 옮길 수 있다', () => {
    // 마키는 누름을 `preventDefault` 하므로 브라우저의 기본 초점 이동이 함께 막힌다.
    // 손으로 옮기지 않으면 방금 감싸 고른 것을 방향키로 옮길 수 없고, 그것은 포인터로
    // 고른 사람과 키보드로 옮기려는 사람이 **같은 사람**이라는 T15 의 전제를 깬다.
    // (맨손 빈 지점 누름은 반대다 — 우리 조작이 아니므로 초점을 빼앗지 않는다.)
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(document.activeElement).toBe(overlayRoot());
  });

  it('맨손 빈 지점 누름은 여전히 초점을 빼앗지 않는다 — 우리 조작이 아니다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    send('pointerdown', EMPTY.x, EMPTY.y);

    expect(document.activeElement).not.toBe(overlayRoot());
  });

  it('좌표 기준은 여전히 **루트**다 — 닿는 면을 재면 축척과 원점이 함께 틀린다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    // 닿는 면에 **다른** 상자를 심는다. 구현이 `event.currentTarget` 대신 이 노드를 재면
    // 아래 기대값이 통째로 어긋난다(가정 A16).
    vi.spyOn(screen.getByTestId('canvas-workspace-hit'), 'getBoundingClientRect').mockReturnValue({
      left: 500,
      top: 500,
      width: 999,
      height: 999,
      right: 1499,
      bottom: 1499,
      x: 500,
      y: 500,
      toJSON: () => ({}),
    } as DOMRect);

    sendToHit('pointerdown', FROM.x, FROM.y, { shiftKey: true });
    sendToHit('pointermove', TO.x, TO.y, { shiftKey: true });

    expect(marqueeEl()!.style.left).toBe('10px');
    expect(selected()).toEqual(['a', 'b']);
  });
});

// --- 무엇이 골라지는가 -----------------------------------------------------

describe('사각형에 **온전히 든** 것만 골라진다 (결정 2 — 겹침이 아니라 포함)', () => {
  it('변에 걸친 것과 밖의 것은 빠진다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(selected()).toEqual(['a', 'b']);
    // 이름으로 한 번 더 적는다 — 겹침 판정으로 구현하면 `edge` 가 들어온다.
    expect(selectionText()).not.toContain('edge');
    expect(selectionText()).not.toContain('c');
  });

  it.each([
    ['왼쪽 위로', TO, FROM],
    ['오른쪽 위로', { x: FROM.x, y: TO.y }, { x: TO.x, y: FROM.y }],
    ['왼쪽 아래로', { x: TO.x, y: FROM.y }, { x: FROM.x, y: TO.y }],
  ])('%s 끌어도 같은 셋이 골라진다 (정규화)', (_name, from, to) => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    dragMarquee(from, to);

    expect(selected()).toEqual(['a', 'b']);
  });

  it('사각형을 줄이면 빠져나간 것이 **풀린다** — 매 이동마다 바탕에서 다시 센다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();

    // 넓게 → 좁게. 직전 결과에 얹는 구현이면 `b` 가 남는다.
    dragMarquee(FROM, TO, { x: 70, y: 40 });

    expect(selected()).toEqual(['a']);
  });

  it('보이지 않는 요소는 감싸도 골라지지 않는다 (`hitTest` 와 같은 규칙)', () => {
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 }), rect('ghost', { x: 55, y: 45, w: 20, h: 20 }, false)]}
      />,
    );
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(selected()).toEqual(['a']);
  });
});

// --- 합집합 (결정 4) -------------------------------------------------------

describe('사각형은 지금 선택에 **더한다** (히트 경로의 `additive` 와 같은 낱말)', () => {
  it('사각형 **밖**에 있던 선택은 그대로 남는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    send('pointerdown', 200, 105); // `c` 의 중심 — 사각형 밖이다
    send('pointerup', 200, 105);
    expect(selected()).toEqual(['c']);

    dragMarquee(FROM, TO);

    expect(selected()).toEqual(['a', 'b', 'c']);
  });

  it('사각형이 덮은 것이 **이미 골라져 있어도 풀리지 않는다** — 토글이 아니다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    send('pointerdown', 40, 20); // `a` 를 미리 고른다
    send('pointerup', 40, 20);
    expect(selected()).toEqual(['a']);

    dragMarquee(FROM, TO);

    // `nextSelection(.., true)` 를 요소마다 부르는 구현이면 여기서 `a` 가 **빠진다**.
    expect(selected()).toEqual(['a', 'b']);
  });

  it('Shift 로 빈 자리를 **누르기만** 하면 선택이 그대로다 (넓이 0 은 아무것도 더하지 않는다)', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    send('pointerdown', 40, 20);
    send('pointerup', 40, 20);

    send('pointerdown', EMPTY.x, EMPTY.y, { shiftKey: true });
    send('pointerup', EMPTY.x, EMPTY.y, { shiftKey: true });

    expect(selected()).toEqual(['a']);
  });
});

// --- 그룹 (결정 3) ---------------------------------------------------------

describe('그룹은 **제 상자**로 골라지고 키는 `nodeId` 다 (SPEC-CANVAS-004 · REQ-06)', () => {
  function group(id: string, geometry: BoxGeometry): GroupElement {
    return {
      id,
      kind: 'group',
      geometry,
      // 부품은 그룹 로컬 격자에 산다 — 사각형 판정은 이 배열을 보지 않는다.
      parts: [rect('p1', { x: 0, y: 0, w: 100, h: 100 }), rect('p2', { x: 200, y: 200, w: 100, h: 100 })],
    };
  }

  it('제 상자가 온전히 들면 **그룹이** 골라진다 — 부품 id 는 선택에 오지 않는다', () => {
    render(<Harness elements={[group('g', { x: 50, y: 40, w: 100, h: 80 })]} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(selected()).toEqual(['g']);
    expect(selectionText()).not.toContain('p1');
  });

  it('제 상자가 변에 걸치면 **부품이 안에 들어도** 골라지지 않는다', () => {
    // px 로 x 20..180 — 사각형의 오른쪽 변(130)을 넘는다. 부품 `p1` 은 그룹 왼쪽 위에
    // 있어 화면상 사각형 안이지만, 판정 대상은 그룹의 상자 하나다.
    render(<Harness elements={[group('g', { x: 50, y: 40, w: 400, h: 80 })]} />);
    stubOverlayRect();

    dragMarquee(FROM, TO);

    expect(selected()).toEqual([]);
  });
});

// --- 축소된 미리보기 (위험 R1 · AC-E9) ------------------------------------

describe('축소된 미리보기에서도 사각형이 손을 따라간다 (축척 0.5)', () => {
  // 축척 1 에서만 재면 `stagePoint` 를 건너뛴 구현이 통과한다 — 나눗셈이 항등이기 때문이다.
  const ZOOM_STAGE: StageSize = { width: 400, height: 200 };
  const ZOOM_RECT = { left: 50, top: 30, width: 200, height: 100 };

  /** 스테이지 로컬 px → 화면 px(정방향 변환). */
  function toScreen(x: number, y: number): { x: number; y: number } {
    return { x: ZOOM_RECT.left + x * 0.5, y: ZOOM_RECT.top + y * 0.5 };
  }

  it('화면 좌표를 스테이지 공간으로 되돌려 사각형과 선택을 낸다', () => {
    // 캔버스 500×400 → 스테이지 400×200 이므로 축척은 가로 0.8 · 세로 0.5 다.
    // `a`(캔버스 50,40,100,80)의 px 상자는 (40,20,80,40) → x 40..120, y 20..60.
    render(<Harness elements={FIXTURE} stage={ZOOM_STAGE} />);
    stubOverlayRect(ZOOM_RECT.left, ZOOM_RECT.top, ZOOM_RECT.width, ZOOM_RECT.height);

    const from = toScreen(20, 10);
    const to = toScreen(140, 70);
    send('pointerdown', from.x, from.y, { shiftKey: true });
    send('pointermove', to.x, to.y, { shiftKey: true });

    // 사각형은 **스테이지 공간**의 상자다 — 화면 px 을 그대로 쓰면 60px 이 나온다.
    expect(marqueeEl()!.style.left).toBe('20px');
    expect(marqueeEl()!.style.width).toBe('120px');
    expect(selected()).toEqual(['a']);
  });
});

// --- REQ-05 유휴 정지 -----------------------------------------------------

describe('사각형은 rAF 루프 밖의 DOM 이다 (REQ-05 · AC-E4)', () => {
  it('몸짓 내내 프레임을 **0 건** 요청하고 기하를 한 글자도 쓰지 않는다', () => {
    const emit = vi.fn();
    render(<Harness elements={FIXTURE} onElementsChange={emit} />);
    stubOverlayRect();
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    // 누름 → 여러 번의 이동 → 뗌. 이동 드래그였다면 합류 프레임이 잡혔을 자리다.
    dragMarquee(FROM, { x: 60, y: 30 }, { x: 100, y: 45 }, TO);
    send('pointerup', TO.x, TO.y, { shiftKey: true });

    expect(selected()).toEqual(['a', 'b']);
    // 루프를 깨우는 유일한 경로는 `elements` 변경이며, 마키는 그것을 만들지 않는다.
    expect(raf).not.toHaveBeenCalled();
    expect(emit).not.toHaveBeenCalled();
  });
});

// --- 불변식 I23 · 위험 R8 (사각형이 **살아 있는 동안** 잰다) --------------
//
// 이 절이 이 파일의 가장 무거운 배달물이다. `CanvasEditOverlay.test.tsx` 의 I23 시험은
// 마키가 **떠 있지 않은 상태**에서 열거하므로 이 새 자식을 한 번도 보지 못한다 — 그 초록은
// 마키에 대해 아무것도 말하지 않는다("컨트롤이 없어서 통과하는 시험은 엉뚱한 이유로
// 초록이다"). 그래서 같은 열거를 **사각형이 살아 있는 상태에서** 다시 돌린다.

describe('사각형은 표시 층이 아니라 **몸짓의 그림자**다 (불변식 I23 · 위험 R8)', () => {
  /** I23 시험이 쓰는 그 판정 — 이 노드나 그 자손이 무언가를 칠하는가. */
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

  /** `aria-hidden` 이면서 칠하는 루트의 직계 자식들. */
  function paintingChildren(): HTMLElement[] {
    return [...overlayRoot().children].filter(
      (child): child is HTMLElement =>
        child instanceof HTMLElement &&
        child.getAttribute('aria-hidden') === 'true' &&
        paintsSomething(child),
    );
  }

  it('사각형은 그 열거에 **실제로 걸린다** — 면제가 우연이 아니라 형상임을 먼저 못박는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);

    // 이 한 줄이 없으면 아래 면제는 **죽은 가드**다(걸리지도 않는 것을 면제해 봐야 뜻이 없다).
    expect(paintingChildren().map((el) => el.dataset.testid)).toContain('canvas-marquee');
  });

  it('걸린 채로 위험 R8 의 문장을 **만족한다** — 장식은 포인터를 먹지 않는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);

    // 우회하지 않는다: `aria-hidden` 을 떼거나 칠하지 않는 척해서 가드를 피하는 대신,
    // 가드가 요구하는 것을 그대로 준다.
    for (const child of paintingChildren()) {
      expect(child.className, child.dataset.testid).toContain('pointer-events-none');
    }
    expect(paintingChildren().length).toBeGreaterThan(0);
  });

  it('손을 떼면 **DOM 에서 사라진다** — 그것이 표시 층이 아니라는 형상 그 자체다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);
    expect(paintingChildren().map((el) => el.dataset.testid)).toContain('canvas-marquee');

    send('pointerup', TO.x, TO.y, { shiftKey: true });

    // 006 의 표시 층 셋은 **조건 없이** 그려져서 손잡이를 요구했다(I23). 이것은 몸짓이
    // 끝나는 순간 없어지므로 다스릴 지속 상태가 없고, 따라서 다스릴 컨트롤도 없다.
    expect(paintingChildren().map((el) => el.dataset.testid)).not.toContain('canvas-marquee');
    expect(marqueeEl()).toBeNull();
  });

  it('세 표시 층은 사각형이 떠 있는 동안에도 그대로다 — 이름 가드를 밟지 않는다', () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    dragMarquee(FROM, TO);

    for (const id of ['canvas-region-scrim', 'canvas-region-bounds', 'canvas-workspace-grid']) {
      expect(screen.getByTestId(id).className, id).toContain('pointer-events-none');
    }
    // 닿는 면은 여전히 포인터를 먹는다 — 사각형이 그 위에 얹혔다고 달라지지 않는다.
    expect(screen.getByTestId('canvas-workspace-hit').className).not.toContain(
      'pointer-events-none',
    );
  });
});

// --- i18n ------------------------------------------------------------------

describe('안내 문구가 두 언어에 있고 짝이 맞는다', () => {
  function editNs(messages: unknown): Record<string, unknown> {
    const root = messages as Record<string, Record<string, Record<string, unknown>>>;
    return root['dashboard']!['canvas']!['edit'] as Record<string, unknown>;
  }

  /** 문구에 든 치환자들. 순서는 뜻이 없고 **개수**가 뜻이 있다. */
  function placeholders(text: string): string[] {
    return [...text.matchAll(/\{[a-zA-Z]+\}/g)].map((m) => m[0]).sort();
  }

  it('`marqueeHint` 가 ko·en 양쪽에 있고 서로 다른 문구다', () => {
    const koText = editNs(ko)['marqueeHint'];
    const enText = editNs(en)['marqueeHint'];

    expect(typeof koText, 'ko').toBe('string');
    expect(typeof enText, 'en').toBe('string');
    expect((koText as string).length).toBeGreaterThan(0);
    expect((enText as string).length).toBeGreaterThan(0);
    // 한쪽을 복사해 두면 번역이 없는 것과 같다.
    expect(koText).not.toBe(enText);
  });

  it('치환자 **다중집합**이 두 언어에서 같다 — 기본 로케일이 ko 라 마운트만으로는 못 잰다', () => {
    // `I18nProvider` 를 세워 렌더하는 시험은 ko 만 보므로 en 의 결측·과잉 치환자를
    // 영영 보지 못한다. 그래서 두 JSON 을 **나란히** 비교한다.
    for (const key of ['keyboardHint', 'marqueeHint'] as const) {
      expect(placeholders(editNs(ko)[key] as string), `ko.${key}`).toEqual(
        placeholders(editNs(en)[key] as string),
      );
    }
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
    const text = editNs(ko)['marqueeHint'] as string;

    for (const word of ['문구', '트윈', '이징']) {
      expect(text.includes(word), `금지 어휘 "${word}"`).toBe(false);
    }
    // 몸짓을 실제로 설명한다 — 지우기만 하고 끝내지 않았다는 확인이다.
    expect(text).toContain('Shift');
  });

  it('설명문이 사각형 몸짓을 말한다 — 사각형은 `aria-hidden` 이라 이 문단이 유일한 통로다', () => {
    render(<Harness elements={FIXTURE} />);

    const id = overlayRoot().getAttribute('aria-describedby');
    const node = document.getElementById(id!);

    expect(node).toBeTruthy();
    // 이 파일의 i18n 대체는 키를 그대로 돌려주므로, 여기서 재는 것은 **이어져 있는가** 다
    // (문구 자체는 위 세 시험이 JSON 으로 잰다).
    expect(node!.textContent).toContain('dashboard.canvas.edit.keyboardHint');
    expect(node!.textContent).toContain('dashboard.canvas.edit.marqueeHint');
  });
});

// --- 이동 드래그가 한 줄도 바뀌지 않았다 ----------------------------------

describe('요소 위 누름은 종전 그대로다 (REQ-08 — 이 경로는 바뀌지 않는다)', () => {
  it('Shift+요소 누름은 여전히 **토글**이고 사각형을 세우지 않는다', async () => {
    render(<Harness elements={FIXTURE} />);
    stubOverlayRect();
    send('pointerdown', 40, 20); // `a`
    send('pointerup', 40, 20);

    send('pointerdown', 110, 45, { shiftKey: true }); // `b` 를 더한다
    expect(selected()).toEqual(['a', 'b']);
    expect(marqueeEl()).toBeNull();

    send('pointerdown', 110, 45, { shiftKey: true }); // 같은 것을 다시 — 빠진다
    expect(selected()).toEqual(['a']);
    expect(marqueeEl()).toBeNull();

    await act(async () => {});
  });

  it('맨손 요소 드래그는 여전히 기하를 쓴다 — 마키가 그 경로를 가로채지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={FIXTURE} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 40, 20);
    send('pointermove', 60, 30);
    await act(async () => {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    });

    expect(emit).toHaveBeenCalled();
    expect(marqueeEl()).toBeNull();
  });
});

// 투영 한 벌이 실제로 이 파일의 기대값을 낳는지 한 자리에서 못박는다(오타 방어).
describe('고정 입력의 산술', () => {
  it('머리말의 표가 투영과 일치한다', () => {
    expect(PROJ.stage.width / PROJ.canvas.width).toBe(0.4);
    expect(PROJ.stage.height / PROJ.canvas.height).toBe(0.25);
  });
});
