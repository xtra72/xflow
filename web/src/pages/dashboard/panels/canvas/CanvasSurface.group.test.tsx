// 그룹이 있을 때의 렌더 루프 — 네 표면의 키와 유휴 정지 (SPEC-CANVAS-004 M3).
//
// 001 의 `CanvasSurface.test.tsx` 하네스를 그대로 쓴다(가짜 시계 · 기록 context 스텁 ·
// 주입 가시성). 재는 것은 셋이다:
//   ① **네 표면이 같은 키를 본다** — 목표 스타일 · 문구 · 트윈 장부 · 글자 폭 장부.
//      최상위 원소에서는 002 와 바이트 동일하고, 부품에서만 복합 키다(불변식 G11).
//   ② **유휴 정지가 유지된다**(REQ-05 · G9). 부품이 늘어도 트윈이 없으면 프레임을
//      예약하지 않고, 트윈이 끝나면 멈춘다.
//   ③ **그룹이 사라지면 그 부품의 장부도 함께 청소된다** — 키가 `그룹id/부품id` 라
//      그룹이 없어지는 순간 어느 순회도 그 키를 내지 않는다.
//
// @spec SPEC-CANVAS-004 REQ-03 · REQ-05 · AC-04 · AC-E6

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, render } from '@testing-library/react';

import type { VisibilitySource } from '../charts/visiblePolling';
import type { CanvasElement, CanvasSize } from './canvasConfig';
import CanvasSurface, { type CanvasOverlayContext, type FrameScheduler } from './CanvasSurface';
import type { CanvasNode, GroupElement } from './group/groupTypes';

// --- 하네스 ---------------------------------------------------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { width: 96, height: 64 } }]);
  }
  unobserve() {}
  disconnect() {}
}

type Recorded = [string, ...unknown[]];

function makeCtxStub() {
  const calls: Recorded[] = [];
  const rec =
    (op: string) =>
    (...args: unknown[]) => {
      calls.push([op, ...args]);
    };
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
    _fillStyle: '' as unknown,
    get fillStyle() {
      return this._fillStyle;
    },
    set fillStyle(v: unknown) {
      this._fillStyle = v;
      calls.push(['fillStyle', v]);
    },
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
  };
}

let ctxStub: ReturnType<typeof makeCtxStub>;

function makeScheduler() {
  let nextHandle = 1;
  let requested = 0;
  const pending = new Map<number, (nowMs: number) => void>();
  const scheduler: FrameScheduler = {
    request(cb) {
      requested += 1;
      const handle = nextHandle++;
      pending.set(handle, cb);
      return handle;
    },
    cancel(handle) {
      pending.delete(handle);
    },
  };
  return {
    scheduler,
    get requested() {
      return requested;
    },
    get pending() {
      return pending.size;
    },
    flush(nowMs: number) {
      const due = [...pending.values()];
      pending.clear();
      act(() => {
        for (const cb of due) cb(nowMs);
      });
    },
  };
}

const ALWAYS_VISIBLE: VisibilitySource = {
  isVisible: () => true,
  subscribe: () => () => {},
};

/** 캔버스 240×160 · 스테이지 96×64 — 두 축이 3:2 라 여백이 0 이고 축척이 0.4 다. */
const CANVAS: CanvasSize = { width: 240, height: 160 };

function rectEl(id: string, over: Partial<CanvasElement> = {}): CanvasElement {
  return {
    id,
    kind: 'rect',
    geometry: { x: 0, y: 0, w: 120, h: 80 },
    style: {},
    ...over,
  } as CanvasElement;
}

/** 부품 셋 — 상자가 서로 다르고 하나는 안쪽에 떠 있으며 문구가 하나 있다(E-A · E-B · E-L). */
function group(over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 20, y: 10, w: 173, h: 91 },
    parts: [
      { id: 'body', kind: 'rect', style: { fill: '#111' }, geometry: { x: 0, y: 0, w: 3000, h: 2000 } },
      {
        id: 'stem',
        kind: 'ellipse',
        style: { fill: '#222' },
        geometry: { x: 4100, y: 3300, w: 1700, h: 900 },
      },
      {
        id: 'label',
        kind: 'text',
        style: { fill: '#333' },
        geometry: { x: 5000, y: 9200 },
        text: 'abcd',
      },
    ],
    ...over,
  };
}

let overlaySeen: CanvasOverlayContext | undefined;

function renderSurface(
  nodes: CanvasNode[],
  props: Partial<React.ComponentProps<typeof CanvasSurface>> = {},
) {
  const clock = makeScheduler();
  // **매번 새 엘리먼트를 짓는다.** 같은 엘리먼트 참조로 `rerender` 하면 React 가 그
  // 렌더를 건너뛰어 오버레이가 다시 불리지 않는다(그러면 아래 `settle` 이 아무 일도
  // 하지 않으면서 초록이 된다 — 죽은 도우미가 될 뻔한 자리다).
  const make = () => (
    <CanvasSurface
      canvas={CANVAS}
      elements={nodes}
      targetStyles={{}}
      texts={{}}
      visibilitySource={ALWAYS_VISIBLE}
      scheduler={clock.scheduler}
      overlay={(ctx) => {
        overlaySeen = ctx;
        return null;
      }}
      {...props}
    />
  );
  const view = render(make());
  return {
    clock,
    view,
    /**
     * 그리기는 재렌더를 낳지 않으므로(폭 장부가 state 가 아니라 ref 다 — REQ-05 유휴
     * 정지), 오버레이가 **직전 프레임**의 장부를 보려면 렌더가 한 번 더 돌아야 한다.
     * 001 의 `CanvasSurface.test.tsx` 가 같은 자리에서 같은 일을 한다.
     */
    settle() {
      view.rerender(make());
    },
  };
}

beforeEach(() => {
  overlaySeen = undefined;
  ctxStub = makeCtxStub();
  vi.stubGlobal('ResizeObserver', TriggeringResizeObserver as unknown as typeof ResizeObserver);
  vi.stubGlobal('devicePixelRatio', 2);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    ctxStub as unknown as CanvasRenderingContext2D,
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// --- ① 네 표면의 키 (AC-04 · G11) ------------------------------------------

describe('네 표면이 같은 키를 본다 (AC-04 · 불변식 G11)', () => {
  it('글자 폭 장부(넷째 표면)가 부품에 복합 키를, 최상위에 평평한 키를 쓴다', () => {
    const top = rectEl('txt-1', {
      kind: 'text',
      geometry: { x: 10, y: 10 },
      text: 'xy',
      // 색이 없으면 그리지 않으므로 재지도 않는다(001 의 규율) — 장부에 담기려면 칠해야 한다.
      style: { fill: '#000000' },
    }) as CanvasElement;
    const { clock, settle } = renderSurface([top, group()]);
    clock.flush(0);
    settle();
    expect(overlaySeen?.textWidths).toBeDefined();
    expect(Object.keys(overlaySeen?.textWidths ?? {}).sort()).toEqual(['grp-1/label', 'txt-1']);
    // **실측 폭**이다 — 폴백과 구분되는 값(문구 길이 × 10)을 쓴다.
    expect(overlaySeen?.textWidths['grp-1/label']).toBe(40);
    expect(overlaySeen?.textWidths['txt-1']).toBe(20);
  });

  it('목표 스타일(첫째 표면)이 부품에 복합 키로 걸린다', () => {
    const { clock } = renderSurface([group()], {
      targetStyles: { 'grp-1/body': { fill: '#ff0000' } },
    });
    clock.flush(0);
    expect(ctxStub.calls.some((c) => c[0] === 'fillStyle' && c[1] === '#ff0000')).toBe(true);
  });

  it('문구(둘째 표면)도 복합 키로 걸린다', () => {
    const { clock } = renderSurface([group()], { texts: { 'grp-1/label': 'replaced' } });
    clock.flush(0);
    expect(ctxStub.calls.find((c) => c[0] === 'fillText')?.[1]).toBe('replaced');
  });

  it('최상위 원소의 키는 002 와 **바이트 동일**하다 — 평평한 `el.id`', () => {
    const { clock } = renderSurface([rectEl('rect-1')], {
      targetStyles: { 'rect-1': { fill: '#00ff00' } },
    });
    clock.flush(0);
    expect(ctxStub.calls.some((c) => c[0] === 'fillStyle' && c[1] === '#00ff00')).toBe(true);
  });

  it('평평한 키로 들어온 부품 스타일은 무시된다 (E-I — 교차 오염 방어)', () => {
    const { clock } = renderSurface([group()], { targetStyles: { body: { fill: '#ff0000' } } });
    clock.flush(0);
    expect(ctxStub.calls.some((c) => c[0] === 'fillStyle' && c[1] === '#ff0000')).toBe(false);
  });
});

// --- ② 트윈 장부(셋째 표면)와 유휴 정지 (REQ-05 · G9) ----------------------

describe('트윈 장부가 복합 키를 쓰고 유휴 정지가 유지된다 (REQ-05 · G9)', () => {
  it('트윈할 것이 없으면 첫 프레임 뒤 **프레임을 예약하지 않는다** (AC-E6)', () => {
    const { clock } = renderSurface([rectEl('r'), group()]);
    clock.flush(0);
    expect(clock.pending).toBe(0);
  });

  it('부품 트윈이 시작되면 프레임이 돌고, 끝나면 **멈춘다**', () => {
    const nodes: CanvasNode[] = [group()];
    const { clock, view } = renderSurface(nodes, {
      panelTween: { duration_ms: 300, easing: 'linear' },
      targetStyles: { 'grp-1/body': { fill: '#000000' } },
    });
    clock.flush(0);
    expect(clock.pending).toBe(0);

    // 목표를 바꾸면 그 **부품 하나**의 트윈이 시작된다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={nodes}
        targetStyles={{ 'grp-1/body': { fill: '#ffffff' } }}
        texts={{}}
        panelTween={{ duration_ms: 300, easing: 'linear' }}
        visibilitySource={ALWAYS_VISIBLE}
        scheduler={clock.scheduler}
      />,
    );
    expect(clock.pending).toBeGreaterThan(0);
    clock.flush(100);
    expect(clock.pending).toBeGreaterThan(0);
    // 창을 넘기면 멈춘다 — 부품이 여럿이어도 **깨어 있는 창의 길이는 늘지 않는다**.
    clock.flush(1000);
    expect(clock.pending).toBe(0);
  });

  it('그룹이 사라지면 그 부품의 장부도 함께 청소된다', () => {
    const withGroup: CanvasNode[] = [rectEl('r'), group()];
    const { clock, view } = renderSurface(withGroup, {
      panelTween: { duration_ms: 300, easing: 'linear' },
    });
    clock.flush(0);

    // 그룹을 걷어 낸다. 청소가 없으면 `grp-1/body` 항목이 장부에 남는다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('r')]}
        targetStyles={{}}
        texts={{}}
        panelTween={{ duration_ms: 300, easing: 'linear' }}
        visibilitySource={ALWAYS_VISIBLE}
        scheduler={clock.scheduler}
        overlay={(ctx) => {
          overlaySeen = ctx;
          return null;
        }}
      />,
    );
    clock.flush(10);

    // 그룹이 되돌아오면 **첫 등장**으로 다시 시작한다(트윈 없이 곧바로 목표에서).
    // 장부가 남아 있었다면 옛 값에서 새 목표로 트윈이 시작되어 프레임이 예약된다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={withGroup}
        targetStyles={{ 'grp-1/body': { fill: '#123456' } }}
        texts={{}}
        panelTween={{ duration_ms: 300, easing: 'linear' }}
        visibilitySource={ALWAYS_VISIBLE}
        scheduler={clock.scheduler}
        overlay={(ctx) => {
          overlaySeen = ctx;
          return null;
        }}
      />,
    );
    clock.flush(20);
    expect(clock.pending).toBe(0);
    // 그리고 목표 색이 **즉시** 칠해진다(보간 중간값이 아니다).
    expect(ctxStub.calls.some((c) => c[0] === 'fillStyle' && c[1] === '#123456')).toBe(true);
  });

  it('그룹 안 문구 부품의 폭도 트윈 없이 한 프레임에 잡힌다', () => {
    const { clock, settle } = renderSurface([group()]);
    clock.flush(0);
    expect(clock.pending).toBe(0);
    settle();
    expect(overlaySeen?.textWidths['grp-1/label']).toBe(40);
  });
});
