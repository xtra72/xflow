// CanvasSurface 렌더 루프 테스트 (SPEC-CANVAS-001 T10).
//
// 2D context 획득은 `HeatmapCanvas.test.tsx` 선례를 그대로 따른다 — jsdom 은 canvas 2D
// context 를 주지 않으므로 `HTMLCanvasElement.prototype.getContext` 를 기록 스텁으로
// 갈아끼운다. 그 위에 이 컴포넌트가 주입 가능하게 만든 두 축을 얹는다.
//   - `FrameScheduler`: 가짜 시계로 프레임을 한 장씩 민다(fake timer 없이 결정적).
//   - `VisibilitySource`: 가시성을 테스트가 직접 뒤집는다(visiblePolling 재사용).
//
// 덮는 인수 기준: AC-E5(리사이즈/DPR 재계산), AC-E6(유휴 정지 · 비가시 무예약 · 재가시
// 즉시 1프레임), AC-03(트윈 도중 목표 변경 시 값이 튀지 않음).

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup, act, fireEvent, screen } from '@testing-library/react';

import type { VisibilitySource } from '../charts/visiblePolling';
import type { CanvasSize, RectElement } from './canvasConfig';
import CanvasSurface, {
  type CanvasOverlayContext,
  type FrameScheduler,
} from './CanvasSurface';
import { useCanvasStageGrid, type CanvasStageGrid } from './canvasStageGrid';
import { DEFAULT_WORKSPACE_ZOOM } from './canvasWorkspace';

// --- ResizeObserver 오버라이드(HeatmapCanvas.test 선례) -------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
let roInstances: { cb: RoCallback }[] = [];
let roDisconnects = 0;
let currentSize = { width: 96, height: 64 };

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
    roInstances.push(this);
  }
  observe() {
    this.cb([{ contentRect: { width: currentSize.width, height: currentSize.height } }]);
  }
  unobserve() {}
  disconnect() {
    roDisconnects += 1;
  }
}

// --- 기록 context 스텁 ---------------------------------------------------

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
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * 10 }),
    // 속성은 접근자로 두어 대입 시점을 기록한다(어떤 색으로 칠했는지가 관찰 대상이다).
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

/** 기록된 호출 이름 목록. */
function ops(): string[] {
  return ctxStub.calls.map((c) => c[0]);
}
/** 마지막으로 대입된 fillStyle(= 마지막으로 칠한 색). */
function lastFill(): unknown {
  return ctxStub.calls.filter((c) => c[0] === 'fillStyle').at(-1)?.[1];
}

// --- 가짜 프레임 예약기 --------------------------------------------------

function makeScheduler() {
  let nextHandle = 1;
  let requested = 0;
  const cancelled: number[] = [];
  const pending = new Map<number, (nowMs: number) => void>();
  const scheduler: FrameScheduler = {
    request(cb) {
      requested += 1;
      const handle = nextHandle++;
      pending.set(handle, cb);
      return handle;
    },
    cancel(handle) {
      cancelled.push(handle);
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
    cancelled,
    /** 예약된 프레임을 주어진 시각으로 실행한다. 실행 중 새로 예약된 것은 다음 flush 몫이다. */
    flush(nowMs: number) {
      const due = [...pending.values()];
      pending.clear();
      act(() => {
        for (const cb of due) cb(nowMs);
      });
    },
  };
}

// --- 가짜 가시성 ---------------------------------------------------------

function makeVisibility(initial = true) {
  let visible = initial;
  const subscribers = new Set<() => void>();
  const source: VisibilitySource = {
    isVisible: () => visible,
    subscribe(onChange) {
      subscribers.add(onChange);
      return () => {
        subscribers.delete(onChange);
      };
    },
  };
  return {
    source,
    get subscriberCount() {
      return subscribers.size;
    },
    set(next: boolean) {
      visible = next;
      act(() => {
        for (const onChange of subscribers) onChange();
      });
    },
  };
}

// --- 고정 입력 -----------------------------------------------------------

/**
 * 대표 캔버스(240x160) — 스테이지(96x64)의 두 배 반이라 축척이 두 축 모두 0.4 다.
 *
 * 1:1 로 두지 않는 것에 뜻이 있다: 축척이 1 이면 캔버스 크기를 아예 무시한 투영도 이
 * 파일의 기대값을 통과한다. 대신 **두 축의 값이 서로 다르다**(240 ≠ 160, 96 ≠ 64) —
 * 축을 뒤바꾼 계산은 거기서 드러난다.
 *
 * **두 비율이 같은 것에 뜻이 있다**(0.10.0). 표면은 축척 하나로 그리는 영역을 맞추므로
 * (`stageLattice`), 비율이 다르면 한 축에 여백이 생겨 아래 기대값들이 전부 그 여백 계산을
 * 안고 가게 된다. 240:160 과 96:64 는 둘 다 3:2 라 여백이 0 이고, 기본 간격 25 에서 한
 * 칸은 96×25/240 = 64×25/160 = 10px 로 두 축이 같은 정수다. 비율이 다를 때의 여백과
 * 맞추는 산술 자체는 따로 시험한다(§그리는 영역 · `canvasGeometry.test.ts`).
 */
const CANVAS: CanvasSize = { width: 240, height: 160 };

function rectEl(id: string, over: Partial<RectElement> = {}): RectElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 240, h: 160 }, style: {}, ...over };
}

const LINEAR_300 = { duration_ms: 300, easing: 'linear' } as const;

beforeEach(() => {
  roInstances = [];
  roDisconnects = 0;
  currentSize = { width: 96, height: 64 };
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

describe('CanvasSurface — 백킹 버퍼와 첫 프레임 (AC-E5)', () => {
  it('표시 크기 × devicePixelRatio 로 백킹 버퍼를 잡고 CSS 크기를 별도로 세운다', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(192); // 96 × dpr 2
    expect(canvas.height).toBe(128); // 64 × dpr 2
    expect(canvas.style.width).toBe('96px');
    expect(canvas.style.height).toBe('64px');
  });

  it('한 프레임은 전체를 지우고 DPR 배율을 건 뒤 요소를 다시 그린다', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        background="#101010"
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    expect(ops()).toContain('clearRect');
    // 배경색이 있으면 지운 뒤 칠한다.
    expect(ctxStub.calls.find((c) => c[0] === 'fillRect')?.slice(1)).toEqual([0, 0, 192, 128]);
    // 배경을 그린 뒤 DPR 배율을 세운다 — 이후 좌표는 CSS px 다.
    expect(ctxStub.calls.filter((c) => c[0] === 'setTransform').map((c) => c.slice(1))).toEqual([
      [1, 0, 0, 1, 0, 0],
      [2, 0, 0, 2, 0, 0],
    ]);
    // 캔버스 (0,0,240,160) = 캔버스 전체 → CSS px 전체 (0,0,96,64).
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 96, 64]);
  });

  it('리사이즈되면 백킹 버퍼를 재계산하고 같은 상대 위치로 다시 그린다', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a', { geometry: { x: 120, y: 80, w: 120, h: 80 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(192);
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([48, 32, 48, 32]);

    ctxStub.calls.length = 0;
    act(() => {
      // 48x32 도 같은 3:2 라 여백이 없다(48×25/240 = 32×25/160 = 5px 칸).
      roInstances[0]!.cb([{ contentRect: { width: 48, height: 32 } }]);
    });
    clock.flush(16);

    expect(canvas.width).toBe(96); // 48 × dpr 2
    expect(canvas.height).toBe(64);
    // 요소는 화면상 같은 상대 위치(우하 사분면)에 남는다.
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([24, 16, 24, 16]);
  });

  it('표시 크기가 0 이면 그리지 않는다(아직 그릴 수 없음 — 루프도 깨우지 않는다)', () => {
    currentSize = { width: 0, height: 0 };
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    expect(ops()).not.toContain('clearRect');
    expect(clock.pending).toBe(0);
  });

  it('devicePixelRatio 를 믿을 수 없으면 배율 1 로 떨어뜨린다', () => {
    vi.stubGlobal('devicePixelRatio', 0);
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(96);
    expect(canvas.height).toBe(64);
  });

  it('2D context 를 얻지 못하면 조용히 그리지 않는다(렌더 예외 없음)', () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const clock = makeScheduler();
    expect(() => {
      render(
        <CanvasSurface
          canvas={CANVAS}
        elements={[rectEl('a')]}
          targetStyles={{ a: { fill: '#ff0000' } }}
          texts={{}}
          scheduler={clock.scheduler}
          visibilitySource={makeVisibility().source}
        />,
      );
      clock.flush(0);
    }).not.toThrow();
    expect(ctxStub.calls).toHaveLength(0);
  });

  it('ResizeObserver 가 없는 환경에서도 렌더가 깨지지 않는다', () => {
    vi.stubGlobal('ResizeObserver', undefined);
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    expect(container.querySelector('canvas')).not.toBeNull();
    expect(ops()).not.toContain('clearRect');
  });
});

describe('CanvasSurface — 유휴 정지 (AC-E6)', () => {
  it('모든 트윈이 끝난 프레임 뒤에는 다음 프레임을 예약하지 않는다', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    // 마운트가 요청한 한 장뿐이다(첫 등장은 트윈 없이 곧바로 목표).
    expect(clock.requested).toBe(1);
    clock.flush(0);
    expect(clock.requested).toBe(1);
    expect(clock.pending).toBe(0);
  });

  it('트윈이 도는 동안에만 프레임을 이어 예약하고 끝나면 멈춘다', () => {
    const clock = makeScheduler();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(1000);
    expect(clock.requested).toBe(1);

    // 목표가 바뀌면 루프가 깨어난다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    expect(clock.requested).toBe(2);

    clock.flush(1000); // 진행도 0 — 아직 끝나지 않았으니 다음 장을 예약한다.
    expect(clock.requested).toBe(3);
    clock.flush(1150); // 진행도 0.5.
    expect(clock.requested).toBe(4);
    clock.flush(1400); // 지속 시간 경과 → 완료 → 예약 중단(유휴).
    expect(clock.requested).toBe(4);
    expect(clock.pending).toBe(0);
  });

  it('props 가 그대로면 아무것도 그리지 않아 마지막 프레임이 남는다(AC-E4)', () => {
    const clock = makeScheduler();
    const elements = [rectEl('a')];
    const targetStyles = { a: { fill: '#ff0000' } };
    const texts = {};
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={targetStyles}
        texts={texts}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(0);
    ctxStub.calls.length = 0;

    // 같은 참조로 재렌더 — 폴링이 실패해 새 데이터가 없는 상황이다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={targetStyles}
        texts={texts}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(16);
    expect(ctxStub.calls).toHaveLength(0);
  });
});

describe('CanvasSurface — 가시성 게이팅 (AC-E6)', () => {
  it('보이지 않는 동안에는 프레임을 예약하지 않고, 다시 보이면 한 장을 즉시 그린다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(0);
    expect(clock.requested).toBe(1);

    visibility.set(false);
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    // 데이터가 바뀌었어도 보이지 않으면 예약하지 않는다.
    expect(clock.requested).toBe(1);
    expect(clock.pending).toBe(0);

    ctxStub.calls.length = 0;
    visibility.set(true);
    expect(clock.requested).toBe(2);
    clock.flush(100);
    // 다시 보이자마자 최신 상태를 반영한다.
    expect(lastFill()).toBe('#ffffff');
  });

  it('보이지 않게 되면 진행 중이던 루프의 예약도 취소한다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(1000);
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(1000); // 트윈 진행 중 → 다음 장이 예약돼 있다.
    expect(clock.pending).toBe(1);

    visibility.set(false);
    expect(clock.pending).toBe(0);
    expect(clock.cancelled).toHaveLength(1);
  });
});

describe('CanvasSurface — 트윈 리타깃 (AC-03)', () => {
  it('보간 도중 목표가 바뀌면 현재 보간 중인 값에서 다시 트윈한다(값이 튀지 않는다)', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const black = (
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />
    );
    const white = (
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />
    );

    const view = render(black);
    clock.flush(1000);
    expect(lastFill()).toBe('#000000');

    view.rerender(white);
    clock.flush(1000);
    clock.flush(1150); // 절반 진행.
    const midway = lastFill();
    expect(midway).not.toBe('#000000');
    expect(midway).not.toBe('#ffffff');

    // 같은 시각에 목표를 되돌린다 → 리타깃은 현재 보간값에서 출발해야 한다.
    view.rerender(black);
    clock.flush(1150);
    expect(lastFill()).toBe(midway);

    // 이어지는 프레임은 원래 목표(검정)로 되돌아간다.
    clock.flush(1500);
    expect(lastFill()).toBe('#000000');
  });

  it('요소의 tween 이 패널 기본 트윈을 덮어쓴다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const element = rectEl('a', { tween: { duration_ms: 0, easing: 'linear' } });
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[element]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(1000);
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[element]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(1000);
    // duration 0 → 즉시 전환이며 루프를 깨우지 않는다.
    expect(lastFill()).toBe('#ffffff');
    expect(clock.pending).toBe(0);
  });

  it('사라진 요소의 트윈 장부는 정리되고 남은 요소만 그린다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a'), rectEl('b')]}
        targetStyles={{ a: { fill: '#ff0000' }, b: { fill: '#00ff00' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(0);
    expect(ctxStub.calls.filter((c) => c[0] === 'rect')).toHaveLength(2);

    ctxStub.calls.length = 0;
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('b')]}
        targetStyles={{ b: { fill: '#00ff00' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(16);
    expect(ctxStub.calls.filter((c) => c[0] === 'rect')).toHaveLength(1);
    expect(lastFill()).toBe('#00ff00');
  });

  it('targetStyles 에 없는 요소는 자기 기본 스타일로 그려진다(AC-E2/AC-E3)', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a', { style: { fill: '#abcdef' } })]}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    expect(lastFill()).toBe('#abcdef');
  });
});

describe('CanvasSurface — 수명주기와 기본 주입값', () => {
  it('언마운트 시 예약된 프레임·관찰자·가시성 구독을 모두 정리한다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    expect(clock.pending).toBe(1);
    expect(visibility.subscriberCount).toBe(1);

    view.unmount();
    expect(clock.pending).toBe(0);
    expect(clock.cancelled).toHaveLength(1);
    expect(visibility.subscriberCount).toBe(0);
    expect(roDisconnects).toBe(1);
  });

  it('예약기·가시성을 주입하지 않으면 브라우저 기본(requestAnimationFrame/document)을 쓴다', async () => {
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
      />,
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(ops()).toContain('clearRect');
    expect(lastFill()).toBe('#ff0000');
  });

  it('기본 예약기에서도 언마운트가 예약을 취소한다', () => {
    const cancelSpy = vi.spyOn(globalThis, 'cancelAnimationFrame');
    const view = render(
      <CanvasSurface canvas={CANVAS} elements={[rectEl('a')]} targetStyles={{}} texts={{}} />,
    );
    view.unmount();
    expect(cancelSpy).toHaveBeenCalled();
  });

  it('className 을 주면 컨테이너 클래스를 대체한다', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[]}
        targetStyles={{}}
        texts={{}}
        className="h-40 w-40"
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    expect((container.firstElementChild as HTMLElement).className).toBe('h-40 w-40');
  });
});

// --- 무동작 보장 (SPEC-CANVAS-002 T3 · AC-E1) ----------------------------
//
// **이 블록은 002 의 오버레이 슬롯보다 먼저 쓰였다.** 002 는 `CanvasSurface` 에 선택
// prop(`overlay` 렌더 prop)을 더하지만, 그것을 넘기지 않았을 때의 동작은 001 과 **완전히
// 같아야 한다**(acceptance.md AC-E1). 그 "같음" 을 나중에 눈으로 비교할 수는 없으므로,
// prop 을 더하기 전에 001 의 현재 동작을 여기에 못박아 둔다. 아래 다섯 축은 그대로 002 의
// 회귀 게이트다 — 하나라도 깨지면 prop 추가가 아니라 루프 배선이 잘못된 것이다.
//
// 위 describe 들과 일부 단언이 겹치는 것은 의도적이다. 저쪽은 001 의 기능을 검증하고,
// 이쪽은 **"슬롯을 쓰지 않으면 아무 일도 없다"** 는 002 의 계약 하나를 검증한다.

describe('CanvasSurface — 무동작 보장: 오버레이 슬롯 미사용 시 001 과 동일 (AC-E1)', () => {
  it('그리는 상자 안에는 <canvas> 하나뿐이다(오버레이가 만드는 노드가 없다)', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    // 컨테이너 → **작업 영역** → 캔버스 + **출력 영역**. 006 이 상자를 하나 더 지었으므로
    // 캔버스를 담은 상자는 이제 `canvas-workspace` 다(006 이전에는 `canvas-stage` 였다).
    // 상자 둘 다 0.9.0/006 이 자투리와 저술 여백을 실제 DOM 으로 만든 것이며 **오버레이와
    // 무관하게 언제나** 있다(슬롯을 써도 안 써도 같다). 그래서 이 시험이 재는 것은 상자의
    // 수가 아니라 **오버레이 때문에 생기는 노드의 수**이고, 그것은 006 이후에도 0 개다.
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.children).toHaveLength(1);
    const area = screen.getByTestId('canvas-workspace');
    expect(wrapper.children[0]).toBe(area);
    // 작업 영역이 드는 것은 캔버스와 출력 영역 **둘뿐**이다 — 셋째가 생기면 여기서 걸린다.
    expect(area.children).toHaveLength(2);
    expect(area.children[0]!.tagName).toBe('CANVAS');
    const box = screen.getByTestId('canvas-stage');
    expect(area.children[1]).toBe(box);
    // 그리고 출력 영역은 **비어 있다** — 슬롯을 쓰지 않으면 옵셔널 호출이 인자 평가조차
    // 건너뛰므로 오버레이가 만드는 노드가 하나도 없다. 이 한 줄이 AC-E1 의 무동작 보장이다.
    expect(box.children).toHaveLength(0);
    expect(screen.queryByTestId('canvas-overlay')).toBeNull();
  });

  it('한 프레임의 그리기 호출 순서와 인자가 001 과 같다', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[
          rectEl('r', { geometry: { x: 0, y: 0, w: 120, h: 80 } }),
          { id: 't', kind: 'text', geometry: { x: 120, y: 80 }, style: {} },
        ]}
        targetStyles={{
          r: { fill: '#ff0000', textColor: '#ffffff' },
          t: { textColor: '#000000' },
        }}
        texts={{ r: 'ab', t: 'cd' }}
        background="#101010"
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    expect(ops()).toEqual([
      // clearSurface — 항등 변환 → 지우기 → 배경 칠하기.
      'setTransform',
      'clearRect',
      'fillStyle',
      'fillRect',
      // DPR 배율(이후 좌표는 CSS px).
      'setTransform',
      // rect 요소 + 그 라벨.
      'save',
      'beginPath',
      'rect',
      'fillStyle',
      'fill',
      'fillStyle',
      'fillText',
      'restore',
      // text 요소(라벨 없음 — 글자가 곧 기하다).
      'save',
      'fillStyle',
      'fillText',
      'restore',
    ]);
    expect(ctxStub.calls.filter((c) => c[0] === 'fillText').map((c) => c.slice(1))).toEqual([
      ['ab', 24, 16], // rect 중심 (24,16), align 미지정(left) → 원점 그대로.
      ['cd', 48, 32], // 기준점 (48,32).
    ]);
  });

  it('프레임 예약 계수가 001 과 같다: 마운트 1회, 유휴 진입 뒤 추가 예약 없음', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    expect(clock.requested).toBe(1);
    clock.flush(0);
    expect(clock.requested).toBe(1);
    expect(clock.pending).toBe(0);
  });

  it('트윈이 도는 동안의 예약 계수와 유휴 정지 시점이 001 과 같다', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(1000);
    expect(clock.requested).toBe(1);

    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        panelTween={LINEAR_300}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    expect(clock.requested).toBe(2);
    clock.flush(1000);
    expect(clock.requested).toBe(3);
    clock.flush(1400); // 지속 시간 경과 → 완료 → 유휴.
    expect(clock.requested).toBe(3);
    expect(clock.pending).toBe(0);
  });

  it('가시성 게이팅이 001 과 같다: 비가시 무예약, 재가시 즉시 한 장', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#000000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(0);
    expect(clock.requested).toBe(1);

    visibility.set(false);
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ffffff' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    expect(clock.requested).toBe(1);
    expect(clock.pending).toBe(0);

    visibility.set(true);
    expect(clock.requested).toBe(2);
  });

  it('언마운트 정리가 001 과 같다: 예약 취소 · 구독 해제 · 관찰자 해제', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    expect(clock.pending).toBe(1);
    expect(visibility.subscriberCount).toBe(1);

    view.unmount();
    expect(clock.pending).toBe(0);
    expect(clock.cancelled).toHaveLength(1);
    expect(visibility.subscriberCount).toBe(0);
    expect(roDisconnects).toBe(1);
  });

  it('props 가 그대로면 프레임도 그리기도 없다(마지막 프레임 보존)', () => {
    const clock = makeScheduler();
    const elements = [rectEl('a')];
    const targetStyles = { a: { fill: '#ff0000' } };
    const texts = {};
    const visibility = makeVisibility();
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={targetStyles}
        texts={texts}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    clock.flush(0);
    ctxStub.calls.length = 0;
    const before = clock.requested;

    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={targetStyles}
        texts={texts}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
      />,
    );
    expect(clock.requested).toBe(before);
    clock.flush(16);
    expect(ctxStub.calls).toHaveLength(0);
  });
});

// --- 오버레이 슬롯 (SPEC-CANVAS-002 T3) ----------------------------------
//
// 위의 무동작 블록이 "쓰지 않으면 아무 일도 없다" 를 지켰다면, 이 블록은 "쓰면 정확히
// 이만큼만 일어난다" 를 지킨다. 검증 축은 셋이다.
//   1) 측정원이 하나다 — 오버레이가 받는 스테이지는 프레임이 투영에 쓰는 그 값이다(AC-E2).
//   2) 폭 장부는 직전 프레임의 값이며 두 번째 측정원이 아니다(AC-E7).
//   3) **새 prop 은 루프를 깨우지 않는다** — 프레임 예약 경로는 001 의 셋 그대로다(AC-E4).

describe('CanvasSurface — 오버레이 슬롯 (SPEC-CANVAS-002)', () => {
  /** 오버레이가 받은 컨텍스트를 순서대로 모은다(렌더마다 한 번 호출된다). */
  function makeOverlaySpy() {
    const seen: CanvasOverlayContext[] = [];
    return {
      seen,
      get last() {
        return seen.at(-1);
      },
      render: (ctx: CanvasOverlayContext) => {
        seen.push(ctx);
        return <div data-testid="canvas-overlay" />;
      },
    };
  }

  it('오버레이는 캔버스 **뒤(=위)** 에 칠해지고 출력 영역 상자 안에 산다', () => {
    const clock = makeScheduler();
    const spy = makeOverlaySpy();
    const { container } = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    clock.flush(0);

    // 006 이전에는 "캔버스의 **뒤 형제**" 라는 모양이 곧 이 성질이었다. 006 이 캔버스를
    // 작업 영역으로 올리면서 둘은 더 이상 형제가 아니지만, 이 시험이 지키던 것은 배치가
    // 아니라 **칠하는 순서**(오버레이가 캔버스 위에 온다)와 **오버레이가 사는 상자**다.
    // 그래서 모양이 아니라 성질을 단언한다 — 006 이후 그 성질은 형제 순서가 아니라 중첩을
    // 통해 성립한다: 작업 영역 안에서 `canvas-stage` 가 `<canvas>` **뒤에** 오고 오버레이는
    // 그 안에 산다.
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.children).toHaveLength(1);
    const area = wrapper.children[0] as HTMLElement;
    expect(area).toBe(screen.getByTestId('canvas-workspace'));

    const surface = screen.getByTestId('canvas-surface');
    const box = screen.getByTestId('canvas-stage');
    const overlayNode = screen.getByTestId('canvas-overlay');

    // 캔버스는 작업 영역이 직접 들고, 오버레이는 **출력 영역** 안에 산다 — 오버레이가 제
    // `getBoundingClientRect()` 로 재는 상자와 `projection.stage` 가 같은 노드가 되려면
    // 오버레이가 바로 그 상자 안에 있어야 한다(0.9.0 · 위험 R1).
    expect(area.children).toHaveLength(2);
    expect(area.children[0]).toBe(surface);
    expect(area.children[1]).toBe(box);
    expect(box.children).toHaveLength(1);
    expect(box.children[0]).toBe(overlayNode);
    // 캔버스는 그 상자 **밖**이다 — 안에 있으면 006 이 하려던 일(그림이 출력 영역 밖으로
    // 나간다)이 통째로 사라진다.
    expect(box.contains(surface)).toBe(false);

    // **칠하는 순서**: 문서 순서로 오버레이가 캔버스보다 뒤다. 두 층은 같은 쌓임 맥락 안의
    // 흐름 위에 있으므로 뒤에 오는 쪽이 위에 칠해진다. 서로를 품지 않는 두 노드이므로
    // 반환값은 마스크가 아니라 `FOLLOWING` 하나다 — 순서가 뒤집히면 `PRECEDING`(2) 이
    // 나와 여기서 걸린다.
    expect(surface.compareDocumentPosition(overlayNode)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });

  it('오버레이가 받는 스테이지는 프레임이 투영에 쓰는 그 값이다(측정원이 하나다)', () => {
    const clock = makeScheduler();
    const spy = makeOverlaySpy();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a', { geometry: { x: 0, y: 0, w: 240, h: 160 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    clock.flush(0);

    // 캔버스 전체를 덮는 상자이므로 투영된 px 상자가 곧 스테이지 크기다.
    const projected = ctxStub.calls.find((c) => c[0] === 'rect')!.slice(1);
    expect(spy.last!.projection.stage).toEqual({ width: projected[2], height: projected[3] });
    expect(spy.last!.projection.stage).toEqual({ width: 96, height: 64 });
  });

  it('오버레이는 캔버스 단위 크기도 함께 받는다 — 투영에는 두 크기가 모두 필요하다', () => {
    // 스테이지만 넘기면 받는 쪽이 나머지 하나를 스스로 구하게 되고, 그 자리가 곧 두 번째
    // 출처다(위험 R1). 그래서 표면이 **한 벌로 묶어** 넘긴다.
    const clock = makeScheduler();
    const spy = makeOverlaySpy();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    clock.flush(0);

    expect(spy.last!.projection.canvas).toEqual(CANVAS);
    expect(spy.last!.projection.canvas).not.toEqual(spy.last!.projection.stage);
  });

  it('캔버스 크기가 바뀌면 같은 요소가 새 축척으로 다시 그려진다', () => {
    // 캔버스 크기는 모든 좌표의 분모이므로, 바뀌면 그림이 달라진다 — 프레임을 예약하는
    // props 축이라는 뜻이다(선택·호버 같은 편집 상태와 다른 부류다).
    const clock = makeScheduler();
    const props = {
      elements: [rectEl('a', { geometry: { x: 0, y: 0, w: 120, h: 80 } })],
      targetStyles: { a: { fill: '#ff0000' } },
      texts: {},
      scheduler: clock.scheduler,
      visibilitySource: makeVisibility().source,
    };
    const { rerender } = render(<CanvasSurface canvas={CANVAS} {...props} />);
    clock.flush(0);
    // 240x160 캔버스의 절반 → 스테이지(96x64)의 절반.
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 48, 32]);

    ctxStub.calls.length = 0;
    rerender(<CanvasSurface canvas={{ width: 120, height: 80 }} {...props} />);
    clock.flush(16);
    // 같은 좌표가 이제 캔버스 전체다 → 스테이지 전체를 덮는다.
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 96, 64]);
  });

  it('패널 크기가 바뀌면 오버레이의 스테이지도 같은 새 값으로 따라온다(AC-E2)', () => {
    const clock = makeScheduler();
    const spy = makeOverlaySpy();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a', { geometry: { x: 0, y: 0, w: 240, h: 160 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    clock.flush(0);
    expect(spy.last!.projection.stage).toEqual({ width: 96, height: 64 });

    ctxStub.calls.length = 0;
    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 48, height: 32 } }]);
    });
    clock.flush(16);

    const projected = ctxStub.calls.find((c) => c[0] === 'rect')!.slice(1);
    expect(spy.last!.projection.stage).toEqual({ width: 48, height: 32 });
    expect(spy.last!.projection.stage).toEqual({ width: projected[2], height: projected[3] });
  });

  it('폭 장부는 첫 프레임 전에는 비어 있고, 그 뒤에는 직전 프레임이 잰 값이다(AC-E7)', () => {
    const clock = makeScheduler();
    const spy = makeOverlaySpy();
    const elements = [{ id: 't', kind: 'text' as const, geometry: { x: 100, y: 80 }, style: {} }];
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={{ t: { textColor: '#000000' } }}
        texts={{ t: 'ab' }}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    // 아직 한 프레임도 그리지 않았다 — 받는 쪽이 폴백할 수 있게 빈 장부다.
    expect(spy.seen[0]!.textWidths).toEqual({});

    clock.flush(0);
    // 그리기는 재렌더를 낳지 않는다(ref 다) — 다음 렌더가 직전 프레임의 장부를 본다.
    view.rerender(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={{ t: { textColor: '#111111' } }}
        texts={{ t: 'ab' }}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={spy.render}
      />,
    );
    // 스텁의 measureText 는 글자 수 × 10 이다.
    expect(spy.last!.textWidths).toEqual({ t: 20 });
  });

  // 표면은 포인터 리스너를 **아예 달지 않는다**(002 가 T3 의 통과 슬롯 넷을 걷어냈다 —
  // 부르는 곳이 하나도 없었고, 편집 포인터는 오버레이 루트가 받는다). 그래서 이 성질은
  // "핸들러를 주지 않았을 때" 가 아니라 **언제나** 성립하는 무조건적 보장이 되었다.
  it('캔버스 포인터 이벤트는 아무 일도 하지 않는다 (리스너가 없다)', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={CANVAS}
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    ctxStub.calls.length = 0;
    const before = clock.requested;

    const canvas = screen.getByTestId('canvas-surface');
    expect(() => {
      fireEvent.pointerDown(canvas, { pointerId: 1 });
      fireEvent.pointerMove(canvas, { pointerId: 1 });
      fireEvent.pointerUp(canvas, { pointerId: 1 });
      fireEvent.pointerCancel(canvas, { pointerId: 1 });
    }).not.toThrow();

    expect(clock.requested).toBe(before);
    expect(ctxStub.calls).toHaveLength(0);
  });

  it('오버레이만 새 신원으로 바뀌면 프레임을 추가로 예약하지 않는다(AC-E4)', () => {
    const clock = makeScheduler();
    const visibility = makeVisibility();
    const elements = [rectEl('a')];
    const targetStyles = { a: { fill: '#ff0000' } };
    const texts = {};
    const view = render(
      <CanvasSurface
        canvas={CANVAS}
        elements={elements}
        targetStyles={targetStyles}
        texts={texts}
        scheduler={clock.scheduler}
        visibilitySource={visibility.source}
        overlay={() => <div data-testid="canvas-overlay" />}
      />,
    );
    clock.flush(0);
    const before = clock.requested;
    ctxStub.calls.length = 0;

    // 렌더마다 새 함수가 만들어지는 것이 실제 사용 형태다 — 그것이 루프를 깨우면 안 된다.
    for (const _ of [0, 1, 2]) {
      view.rerender(
        <CanvasSurface
          canvas={CANVAS}
        elements={elements}
          targetStyles={targetStyles}
          texts={texts}
          scheduler={clock.scheduler}
          visibilitySource={visibility.source}
          overlay={() => <div data-testid="canvas-overlay" />}
        />,
      );
    }

    expect(clock.requested).toBe(before);
    expect(clock.pending).toBe(0);
    expect(ctxStub.calls).toHaveLength(0);
  });
});

// --- 그리는 영역은 격자 칸의 정수배다 (0.9.0 · 사용 시험 "격자가 일정하지 않음") ---
//
// 위 절들은 픽스처가 이미 칸에 맞아 있어 "맞추기" 를 재지 못한다(그것이 그 픽스처를 고른
// 이유다 — 다른 계약을 재는 시험이 이 산술에 흔들리면 안 된다). 여기서는 반대로 **나누어
// 떨어지지 않는** 크기를 넣어 맞추기 자체를 본다. 사용자가 실제로 본 1749×796 이다.
//
// 재는 것은 상자다: 표면이 자투리를 산술이 아니라 **진짜 DOM 상자**로 만들었는가.
// 그것이 이 변경의 핵심 결정이다 — 상자를 지어 두면 오버레이가 제
// `getBoundingClientRect()` 로 재는 상자와 `projection.stage` 가 같은 노드라, 그 사이에
// 조용한 오프셋이 낄 자리가 **존재할 수 없다**(위험 R1 · AC-E9 와 같은 부류의 함정).

/** 기본 캔버스(500x400) — 이 절만 쓴다. 25 단위 격자면 20 x 16 칸이다. */
const WIDE_CANVAS: CanvasSize = { width: 500, height: 400 };

function stageBox(): HTMLElement {
  return screen.getByTestId('canvas-stage');
}

/** 작업 영역 상자(006 M2). 편집이 꺼져 있으면 출력 영역과 크기·자리가 같다. */
function areaBox(): HTMLElement {
  return screen.getByTestId('canvas-workspace');
}

/** `'384px'` → `384`. 두 상자의 자리를 더해 **화면상 자리**를 구할 때 쓴다. */
function px(value: string): number {
  return Number.parseFloat(value);
}

describe('CanvasSurface — 그리는 영역 (0.9.0)', () => {
  function renderAt(width: number, height: number, overlay?: (ctx: CanvasOverlayContext) => React.ReactNode) {
    currentSize = { width, height };
    const clock = makeScheduler();
    const view = render(
      <CanvasSurface
        canvas={WIDE_CANVAS}
        elements={[rectEl('a', { geometry: { x: 0, y: 0, w: 500, h: 400 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        overlay={overlay}
      />,
    );
    clock.flush(0);
    return { clock, view };
  }

  it('나누어떨어지지 않는 상자(1749x796)를 칸의 정수배로 줄인다', () => {
    renderAt(1749, 796);

    // '정확한' 칸은 가로 87.45 · 세로 49.75px 였다. 축척이 하나이므로 작은 쪽을 내림한
    // 49px 를 두 축에 함께 쓴다 — 20칸 × 16칸.
    expect(stageBox().style.width).toBe('980px');
    expect(stageBox().style.height).toBe('784px');
  });

  it('그 상자에서 한 칸이 **정사각형**이다 — 도형이 일그러지지 않는 근거다', () => {
    renderAt(1749, 796);

    // 상자 크기 ÷ 칸 수. 두 값이 같아야 캔버스 단위의 정사각형이 화면에서도 정사각형이다.
    const width = Number.parseFloat(stageBox().style.width);
    const height = Number.parseFloat(stageBox().style.height);
    expect(width / (WIDE_CANVAS.width / 25)).toBe(height / (WIDE_CANVAS.height / 25));
  });

  it('자투리는 양쪽에 나눈 **정수** 자리가 된다 — 소수면 안쪽 선이 다시 소수다', () => {
    renderAt(1749, 796);

    // 비율이 다르므로 여백은 가로에 몰린다((1749-980)/2 내림). 그 여백은 캔버스 크기를
    // 패널 비율에 맞추면 사라진다(`fitCanvasSizeToStage` · `CanvasPanel` 자동 맞춤).
    //
    // 006 은 이 **자리를 바깥쪽 상자로 올렸을 뿐** 수를 바꾸지 않았다 — 편집이 꺼진 갈래에서
    // 작업 영역은 0.9.0 의 그 안쪽 상자와 크기·자리가 그대로 같다(`workspaceBox` 무동작 갈래).
    expect(areaBox().style.left).toBe('384px');
    expect(areaBox().style.top).toBe('6px'); // (796-784)/2

    // 그리고 읽는 사람이 실제로 신경 쓰는 성질은 이것이다 — **출력 영역의 화면상 자리가
    // 상자를 하나 더 지었어도 옮겨지지 않았다.** 중첩된 자리는 두 상자의 합이고, 무동작
    // 갈래에서 원점이 (0,0) 이므로 합은 006 이전의 그 값 그대로여야 한다. 중첩이 스테이지를
    // 화면에서 한 픽셀이라도 밀면 여기서 걸린다.
    expect(px(areaBox().style.left) + px(stageBox().style.left)).toBe(384);
    expect(px(areaBox().style.top) + px(stageBox().style.top)).toBe(6);
    // 두 자리 모두 **정수** CSS px 다 — 소수면 안쪽 선이 다시 소수에서 시작한다.
    for (const value of [areaBox().style.left, areaBox().style.top, stageBox().style.left, stageBox().style.top]) {
      expect(Number.isInteger(px(value))).toBe(true);
    }
  });

  it('캔버스와 오버레이가 **그 상자 안에** 함께 산다 (측정원이 하나다)', () => {
    const seen: CanvasOverlayContext[] = [];
    renderAt(1749, 796, (ctx) => {
      seen.push(ctx);
      return <div data-testid="canvas-overlay" />;
    });
    const spy = { last: seen.at(-1) };

    // 006 이 캔버스를 작업 영역으로 올렸으므로 "같은 상자 안" 은 이제 **작업 영역**이다.
    // 그러나 이 시험의 알맹이는 배치가 아니라 **측정원이 하나**라는 성질이고, 그것은 아래
    // 마지막 세 줄이 통째로 진다 — 오버레이가 사는 상자의 style 크기가 곧 `projection.stage`
    // 다. 둘이 갈라지는 순간 포인터에 조용한 오프셋이 실린다(위험 R1).
    const area = areaBox();
    expect(area.contains(screen.getByTestId('canvas-surface'))).toBe(true);
    expect(area.contains(screen.getByTestId('canvas-overlay'))).toBe(true);

    const box = stageBox();
    // 오버레이가 사는 상자는 **출력 영역**이다(캔버스는 그 바깥이다).
    expect(box.contains(screen.getByTestId('canvas-overlay'))).toBe(true);
    expect(box.contains(screen.getByTestId('canvas-surface'))).toBe(false);
    // 오버레이가 받는 스테이지 = 그 상자의 크기. 둘이 다르면 포인터에 오프셋이 실린다.
    expect(spy.last!.projection.stage).toEqual({ width: 980, height: 784 });
    expect(`${spy.last!.projection.stage.width}px`).toBe(box.style.width);
    expect(`${spy.last!.projection.stage.height}px`).toBe(box.style.height);
  });

  it('캔버스도 그 크기로 잡힌다 — 그리는 좌표와 상자가 같은 영역이다', () => {
    renderAt(1749, 796);

    const canvas = screen.getByTestId('canvas-surface') as HTMLCanvasElement;
    expect(canvas.style.width).toBe('980px');
    expect(canvas.style.height).toBe('784px');
    // 캔버스 전체를 덮는 요소 → 맞춘 영역 전체를 덮는다.
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 980, 784]);
  });

  it('바깥 상자가 한 칸 안에서 흔들려도 그리는 영역은 그대로다', () => {
    // 796 과 799 는 같은 49px 칸을 고르므로 영역은 둘 다 980×784 다. 잰 값이 아니라
    // **맞춘 영역**이 그림을 정하므로, 레이아웃이 1px 씩 흔들려도 도형이 움직이지 않는다.
    const { clock } = renderAt(1749, 796);
    ctxStub.calls.length = 0;

    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 1750, height: 799 } }]);
    });
    clock.flush(16);

    expect(stageBox().style.width).toBe('980px');
    expect(stageBox().style.height).toBe('784px');
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 980, 784]);
  });

  it('아주 작은 패널에서도 그림이 사라지지 않는다 (한 칸이 1px 미만)', () => {
    // 한 칸이 0.5px 이라 내림하면 0 이다. 0 을 곱하면 영역이 통째로 사라지므로
    // 그때는 맞추지 않고 종전 그대로 그린다.
    renderAt(10, 8);

    expect(stageBox().style.width).toBe('10px');
    expect(stageBox().style.height).toBe('8px');
    expect(stageBox().style.left).toBe('0px');
    const rect = ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1);
    expect(rect).toEqual([0, 0, 10, 8]);
  });

  it('아직 재지 못한 상자(0)에서는 그리지 않고 NaN 도 내지 않는다', () => {
    const { clock } = renderAt(0, 0);

    expect(stageBox().style.width).toBe('0px');
    expect(stageBox().style.height).toBe('0px');
    expect(ops()).not.toContain('clearRect');
    expect(clock.pending).toBe(0);
  });
});

// --- 잰 바깥 상자를 밖으로 알린다 (0.10.0) -------------------------------
//
// 편집기의 "패널 비율에 맞춤" 이 이 값을 쓴다. 알리는 것이 **바깥** 상자인 것이 요점이다 —
// 맞춘 영역(`projection.stage`)은 이미 캔버스 비율이라 그것에 맞추면 언제나 무동작이고,
// 여백을 없애려면 여백을 만든 그 상자를 봐야 한다.

describe('CanvasSurface — 잰 상자 알림 (0.10.0)', () => {
  function renderMeasured(width: number, height: number) {
    currentSize = { width, height };
    const clock = makeScheduler();
    const onStageMeasured = vi.fn();
    render(
      <CanvasSurface
        canvas={WIDE_CANVAS}
        elements={[rectEl('a', { geometry: { x: 0, y: 0, w: 500, h: 400 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        onStageMeasured={onStageMeasured}
      />,
    );
    clock.flush(0);
    return { clock, onStageMeasured };
  }

  it('알리는 값은 **잰 바깥 상자**다 — 맞춘 영역이 아니다', () => {
    const { onStageMeasured } = renderMeasured(1749, 796);

    // 맞춘 영역은 980×784 다. 그 값을 알리면 받는 쪽이 제 꼬리를 물게 된다.
    expect(onStageMeasured).toHaveBeenLastCalledWith({ width: 1749, height: 796 });
    expect(screen.getByTestId('canvas-stage').style.width).toBe('980px');
  });

  it('크기가 바뀌면 새 값으로 다시 알린다', () => {
    const { clock, onStageMeasured } = renderMeasured(1749, 796);

    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 800, height: 600 } }]);
    });
    clock.flush(16);

    expect(onStageMeasured).toHaveBeenLastCalledWith({ width: 800, height: 600 });
  });

  it('이 알림은 프레임을 예약하지 않는다 — 유휴 정지가 그대로다 (AC-E4)', () => {
    const { clock, onStageMeasured } = renderMeasured(1749, 796);
    // 첫 프레임 뒤 유휴에 들었다. 마운트 때는 아직 재기 전인 0 과 잰 값이 차례로 나가므로
    // (아래 §첫 통보) 여기서부터의 증가만 센다.
    expect(clock.pending).toBe(0);
    const before = clock.requested;
    const notified = onStageMeasured.mock.calls.length;

    // 같은 크기를 다시 통보한다 — 상태가 갈리지 않으므로 알림도 프레임도 없다.
    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 1749, height: 796 } }]);
    });

    expect(clock.requested).toBe(before);
    expect(clock.pending).toBe(0);
    expect(onStageMeasured.mock.calls.length).toBe(notified);
  });

  it('아직 재지 못한 상자(0)도 그대로 알린다 — 판단은 받는 쪽의 몫이다', () => {
    const { onStageMeasured } = renderMeasured(0, 0);
    expect(onStageMeasured).toHaveBeenLastCalledWith({ width: 0, height: 0 });
  });

  it('첫 통보는 **재기 전의 0** 이다 — 받는 쪽은 그것을 보고 아무것도 하지 않는다', () => {
    // `ResizeObserver` 가 처음 부르기 전 상태가 0 이며, 그 값도 숨기지 않고 알린다.
    // 숨기면 받는 쪽이 "아직 모른다" 와 "0 이다" 를 구분할 수 없다.
    const { onStageMeasured } = renderMeasured(1749, 796);
    expect(onStageMeasured.mock.calls[0]![0]).toEqual({ width: 0, height: 0 });
    expect(onStageMeasured).toHaveBeenLastCalledWith({ width: 1749, height: 796 });
  });
});

// --- 작업 영역과 출력 영역 (SPEC-CANVAS-006 M2) --------------------------
//
// 006 이 표면에 더한 다섯째 책임 하나만 잰다: **상자를 둘로 짓고, 비트맵을 바깥쪽 상자에
// 올리고, 캔버스 좌표 원점을 이미 있던 `setTransform` 한 줄에 싣는다.** 격자 위상(M3) ·
// 경계와 흐림(M4~) 은 이 절의 몫이 아니다.
//
// 고정 입력을 여기 한 번 적고 이유를 함께 남긴다 — acceptance.md §시험 규율의 다섯 함정과
// 006 이 더한 넷을 이 절이 정면으로 받는다.
//   - 바깥 상자 **1749 × 796** — 정사각형이 아니고(0.8.0 (O)), 캔버스 비율 5:4 와도 다르며
//     (0.10.0 (V)), 칸으로 나누어떨어지지도 않는다(0.9.0 (R)). 사용자가 반 칸을 실제로 본
//     그 상자다. 정사각형이나 비율이 같은 상자를 넣으면 여백이 0 이라 원점이 (0,0) 이 되고,
//     그러면 **원점 이동이 아예 없는 코드도 이 절 전부를 통과한다.**
//   - 캔버스 **500 × 400**, 간격 **25**.
//   - 그 조합에서 나오는 수: 꺼짐 → 영역 980×784 · 자리 (384, 6) · 칸 49.
//     켜짐 → 줄인 상자 1311×597 · 영역 740×592 · 원점 (504, 102) · 칸 37.
//   - **원점 (504, 102) 는 칸 37 의 배수가 아니다**(504 = 37×13 + 23, 102 = 37×2 + 28). D3 이
//     요구하는 조합이 이것이며, 그 사실을 아래에서 수로 단언해 다음 사람이 축소 비율을
//     편한 값으로 갈아 끼우면 곧바로 걸리게 한다.
//   - 요소 셋은 **안에 하나 · 완전히 밖에 하나(음수 좌표) · 경계에 걸친 하나**다(D2).
//     전부 안에 있는 고정 입력은 원점 이동이 없어도 통과한다.
//   - 축척은 두 갈래 모두 1 이 아니다(꺼짐 1.96 · 켜짐 1.48). 축척 1 이면 나눗셈이 항등이라
//     투영을 아예 하지 않는 코드도 통과한다(0.2.0 결함 B).
//   - **꺼진 갈래를 따로 잰다**(D1). 켠 시험만 두면 무동작 갈래가 한 번도 돌지 않고,
//     기본값이 `true` 로 뒤집혀도 아무도 모른다.

describe('CanvasSurface — 작업 영역과 출력 영역 (SPEC-CANVAS-006 M2)', () => {
  /** 안에 하나 · 완전히 밖에 하나(음수 좌표) · 경계에 걸친 하나 (D2). */
  const OUTSIDE_ELEMENTS = [
    rectEl('in', { geometry: { x: 100, y: 100, w: 100, h: 100 } }),
    rectEl('out', { geometry: { x: -100, y: -50, w: 100, h: 100 } }),
    rectEl('edge', { geometry: { x: 450, y: 350, w: 100, h: 100 } }),
  ];
  /** 신원이 고정된 props — 재렌더에서 그대로 다시 쓴다(아래 `element` 주석 참고). */
  const TARGET_STYLES = {
    in: { fill: '#ff0000' },
    out: { fill: '#00ff00' },
    edge: { fill: '#0000ff' },
  };
  const NO_TEXTS = {};

  function renderSurface(opts: {
    workspace: boolean;
    width?: number;
    height?: number;
  }) {
    currentSize = { width: opts.width ?? 1749, height: opts.height ?? 796 };
    const clock = makeScheduler();
    const seen: CanvasOverlayContext[] = [];
    const visibility = makeVisibility().source;
    const overlay = (ctx: CanvasOverlayContext) => {
      seen.push(ctx);
      return <div data-testid="canvas-overlay" />;
    };
    // 요소를 함수로 짓는 것에 뜻이 있다 — 아래 토글 시험이 **`workspace` 하나만** 바꾼
    // 재렌더를 하려면 나머지 props 가 신원까지 그대로여야 한다. 객체 리터럴을 재렌더에서
    // 다시 쓰면 `targetStyles` 의 새 신원이 프레임을 부르고, 그러면 그 시험은 토글이 아니라
    // **리터럴이 부른 프레임**을 세면서 초록이 된다(틀린 이유로 통과하는 초록).
    const element = (workspace: boolean) => (
      <CanvasSurface
        canvas={WIDE_CANVAS}
        elements={OUTSIDE_ELEMENTS}
        targetStyles={TARGET_STYLES}
        texts={NO_TEXTS}
        scheduler={clock.scheduler}
        visibilitySource={visibility}
        workspace={workspace}
        overlay={overlay}
      />
    );
    const view = render(element(opts.workspace));
    clock.flush(0);
    return {
      clock,
      view,
      seen,
      last: () => seen.at(-1)!,
      /** `workspace` **하나만** 뒤집어 다시 렌더한다(나머지는 신원까지 동일하다). */
      toggleWorkspace(next: boolean) {
        act(() => {
          view.rerender(element(next));
        });
      },
    };
  }

  /** 이 프레임이 그린 rect 들의 인자(스테이지 px). 순서는 요소 순서다. */
  function rects(): unknown[][] {
    return ctxStub.calls.filter((c) => c[0] === 'rect').map((c) => c.slice(1));
  }
  /** 이 프레임이 세운 좌표계들. 첫째는 `clearSurface` 의 항등 되돌림이다. */
  function transforms(): unknown[][] {
    return ctxStub.calls.filter((c) => c[0] === 'setTransform').map((c) => c.slice(1));
  }
  function surfaceEl(): HTMLCanvasElement {
    return screen.getByTestId('canvas-surface') as HTMLCanvasElement;
  }

  it('꺼진 갈래(기본값)에서는 두 상자가 겹치고 원점이 (0,0) 이다 — 픽셀이 006 이전과 같다', () => {
    // D1: 이 시험이 없으면 무동작 갈래가 한 번도 돌지 않는다. 006 은 갈래를 `workspaceBox`
    // **한 함수** 안에 두었으므로(AC-E1) 여기서 재는 것은 그 갈래가 정말 오늘과 같은가다.
    renderSurface({ workspace: false });

    // 상자가 하나 늘었지만 크기·자리가 겹쳐 화면은 그대로다.
    expect(areaBox().style.width).toBe('980px');
    expect(areaBox().style.height).toBe('784px');
    expect(stageBox().style.width).toBe(areaBox().style.width);
    expect(stageBox().style.height).toBe(areaBox().style.height);
    // 원점 (0,0) — 출력 영역이 작업 영역의 왼쪽 위에 딱 겹친다.
    expect(stageBox().style.left).toBe('0px');
    expect(stageBox().style.top).toBe('0px');
    // 자투리 자리는 006 이전 그대로 (384, 6) 이고, 이제 바깥쪽 상자가 그것을 진다.
    expect(areaBox().style.left).toBe('384px');
    expect(areaBox().style.top).toBe('6px');

    // 평행이동 두 인자가 **0** 이다 — 원점이 (0,0) 이므로 좌표계가 006 이전과 같다.
    expect(transforms()).toEqual([
      [1, 0, 0, 1, 0, 0],
      [2, 0, 0, 2, 0, 0],
    ]);
    // 비트맵도 CSS 크기도 006 이전의 그 값이다(dpr 2).
    expect(surfaceEl().style.width).toBe('980px');
    expect(surfaceEl().style.height).toBe('784px');
    expect(surfaceEl().width).toBe(1960);
    expect(surfaceEl().height).toBe(1568);
    // 축척 980/500 = 1.96 (1 이 아니다). 그림도 006 이전과 한 픽셀도 다르지 않다.
    expect(rects()).toEqual([
      [196, 196, 196, 196],
      [-196, -98, 196, 196],
      [882, 686, 196, 196],
    ]);
  });

  it('켠 갈래는 상자를 둘로 짓는다 — 작업 영역은 잰 상자 전부, 출력 영역은 그 안 가운데다', () => {
    renderSurface({ workspace: true });

    // D4 를 **먼저** 단언한다: 두 상자가 실제로 다르지 않으면 006 이 하는 일이 사라지고
    // 아래 나머지가 전부 우연히 통과한다(축소 비율 1.0 이 감추는 그 형상이다).
    expect(areaBox().style.width).not.toBe(stageBox().style.width);
    expect(areaBox().style.height).not.toBe(stageBox().style.height);

    // 작업 영역 = 잰 바깥 상자 전부, 자리는 (0,0).
    expect(areaBox().style.width).toBe('1749px');
    expect(areaBox().style.height).toBe('796px');
    expect(areaBox().style.left).toBe('0px');
    expect(areaBox().style.top).toBe('0px');

    // 출력 영역 = 줄인 상자(1311×597)에 격자를 맞춘 740×592 가 가운데에 선다.
    expect(stageBox().style.width).toBe('740px');
    expect(stageBox().style.height).toBe('592px');
    expect(stageBox().style.left).toBe('504px');
    expect(stageBox().style.top).toBe('102px');

    // 원점이 **정수** CSS px 다 — 소수면 안쪽의 모든 선이 다시 소수에서 시작한다.
    expect(Number.isInteger(px(stageBox().style.left))).toBe(true);
    expect(Number.isInteger(px(stageBox().style.top))).toBe(true);
    // 그리고 사방 여백이 축마다 1px 이내로 고르다(내림 때문에 0 이 아니라 한 단위 이내다).
    expect(Math.abs(1749 - 740 - 2 * 504)).toBeLessThanOrEqual(1);
    expect(Math.abs(796 - 592 - 2 * 102)).toBeLessThanOrEqual(1);

    // 축척은 **하나**다 — 출력 영역의 종횡비가 캔버스와 같아야 도형이 일그러지지 않는다.
    const cell = 740 / (WIDE_CANVAS.width / 25);
    expect(cell).toBe(592 / (WIDE_CANVAS.height / 25));
    expect(cell).toBe(37);
    // D3: 그 원점은 한 칸의 **배수가 아니다**. 배수인 고정 입력에서는 격자를 작업 영역의
    // 왼쪽 위에 앉힌 결함과 출력 영역의 원점에 앉힌 옳은 구현이 같은 자리를 낸다.
    expect(px(stageBox().style.left) % cell).toBe(23);
    expect(px(stageBox().style.top) % cell).toBe(28);
  });

  it('출력 영역의 크기가 곧 `projection.stage` 다 — 두 상자가 **다를 때에도** 그렇다', () => {
    // 꺼진 갈래에서는 작업 영역과 출력 영역이 같은 값이라, 표면이 실수로 작업 영역을
    // 넘겨도 이 단언이 통과한다(두 값이 우연히 같아지는 형상 — 0.3.0 결함 D 의 규율).
    // 그래서 이 게이트는 **켠 갈래**에서 돌린다.
    const { last } = renderSurface({ workspace: true });

    expect(last().projection.stage).toEqual({ width: 740, height: 592 });
    expect(`${last().projection.stage.width}px`).toBe(stageBox().style.width);
    expect(`${last().projection.stage.height}px`).toBe(stageBox().style.height);
    // 작업 영역이 아니다 — 넘겼다면 오버레이가 제 상자로 재는 값과 갈라져 포인터에
    // 조용한 오프셋이 실린다(위험 R1).
    expect(`${last().projection.stage.width}px`).not.toBe(areaBox().style.width);
    // 투영 한 벌은 여전히 두 칸이며 원점을 싣지 않는다(AC-01).
    expect(Object.keys(last().projection).sort()).toEqual(['canvas', 'stage']);
    expect(last().projection.canvas).toEqual(WIDE_CANVAS);
    // 오버레이는 **출력 영역** 안에 산다(캔버스는 그 바깥이다).
    expect(stageBox().contains(screen.getByTestId('canvas-overlay'))).toBe(true);
    expect(stageBox().contains(surfaceEl())).toBe(false);
    expect(areaBox().contains(surfaceEl())).toBe(true);
  });

  it('비트맵은 출력 영역이 아니라 **작업 영역**을 덮는다 (`computeBackingSize` 가 받는 상자)', () => {
    // dpr 을 정수 2 가 아니라 **1.5** 로 둔다 — 정수 dpr 은 반올림 자리를 감춘다.
    vi.stubGlobal('devicePixelRatio', 1.5);
    renderSurface({ workspace: true });

    // CSS 크기가 작업 영역이다. 출력 영역(740×592)이면 그 밖의 그림이 통째로 잘린다.
    expect(surfaceEl().style.width).toBe('1749px');
    expect(surfaceEl().style.height).toBe('796px');
    expect(surfaceEl().style.width).not.toBe(stageBox().style.width);
    expect(surfaceEl().style.height).not.toBe(stageBox().style.height);
    // 백킹 버퍼도 작업 영역 × dpr 이다(1749×1.5 = 2623.5 → 2624 · 796×1.5 = 1194).
    expect(surfaceEl().width).toBe(2624);
    expect(surfaceEl().height).toBe(1194);
    // 지우는 것도 작업 영역 전체다 — 출력 영역만 지우면 밖의 지난 프레임이 남는다.
    expect(ctxStub.calls.find((c) => c[0] === 'clearRect')?.slice(1)).toEqual([0, 0, 2624, 1194]);
  });

  it('원점은 이미 있던 `setTransform` **그 한 줄**에 실린다 — 좌표계를 새로 건너지 않는다', () => {
    renderSurface({ workspace: true });

    // 한 프레임의 좌표계는 둘뿐이다: `clearSurface` 가 소유한 항등 되돌림과, 그리기 좌표계
    // **하나**. 006 은 둘째의 마지막 두 인자(평행이동)에 원점을 실을 뿐 셋째를 만들지
    // 않는다 — 셋째가 생기면 그것이 곧 새로운 좌표계 건넘이고, 보정 산술이 낄 자리다.
    expect(transforms()).toEqual([
      [1, 0, 0, 1, 0, 0],
      [2, 0, 0, 2, 504 * 2, 102 * 2],
    ]);
    expect(transforms()).toHaveLength(2);
  });

  it('출력 영역 **밖**의 요소가 비트맵 **안**으로 들어와 그려진다 (D2)', () => {
    renderSurface({ workspace: true });

    // 투영은 한 글자도 바뀌지 않았다 — 밖의 요소는 여전히 **음수** 스테이지 px 다.
    // (여기서 값이 양수로 접혀 있으면 누군가 투영에 원점을 더한 것이고, 그것이 곧 두 번
    //  세는 결함이다.)
    expect(rects()).toEqual([
      [148, 148, 148, 148],
      [-148, -74, 148, 148],
      [666, 518, 148, 148],
    ]);

    // 그 음수를 비트맵 안으로 들이는 것은 좌표계의 평행이동이다. 두 층(투영 · 좌표계)을
    // **함께** 세워야 이음매가 덮인다(0.4.0 결함 E — 한 층만 본 시험은 통과한다).
    const [, draw] = transforms();
    const [scale, , , , tx, ty] = draw as number[];
    const outside = rects()[1] as number[];
    // 밖의 요소가 장치 픽셀에서 실제로 비트맵 안에 떨어진다.
    expect(outside[0]! * scale! + tx!).toBe(712); // -148×2 + 1008
    expect(outside[1]! * scale! + ty!).toBe(56); // -74×2 + 204
    expect(outside[0]! * scale! + tx!).toBeGreaterThanOrEqual(0);
    expect(outside[1]! * scale! + ty!).toBeGreaterThanOrEqual(0);
    // 경계에 걸친 요소는 출력 영역을 **넘어서지만** 비트맵 안에 남는다.
    const edge = rects()[2] as number[];
    expect(edge[0]! + edge[2]!).toBeGreaterThan(740);
    expect((edge[0]! + edge[2]!) * scale! + tx!).toBeLessThanOrEqual(surfaceEl().width);
  });

  it('편집 토글은 프레임을 **한 장** 예약하고 다시 유휴로 돌아간다 (AC-E4)', () => {
    // 그리는 상자가 실제로 바뀌므로 한 장이 필요하다. 중요한 것은 그것이 **종전의 props
    // 변경 경로**(`geometry` 의존성) 그대로이지 새 깨우기 경로가 아니라는 것이다.
    //
    // 재렌더에서 바뀌는 것은 `workspace` **하나뿐**이고 나머지 props 는 신원까지 그대로다
    // (`renderSurface.toggleWorkspace`). 리터럴을 다시 쓰면 `targetStyles` 의 새 신원이
    // 프레임을 부르고, 그러면 이 시험은 토글이 아니라 그 리터럴을 세면서 초록이 된다.
    const { clock, toggleWorkspace } = renderSurface({ workspace: false });
    expect(clock.pending).toBe(0); // 유휴에 들었음을 **먼저** 단언한다(꺼져 있어서 통과하는 초록 방지)
    const before = clock.requested;

    toggleWorkspace(true);

    expect(clock.requested).toBe(before + 1);
    clock.flush(16);
    // 진행 중 트윈이 없으므로 그 한 장 뒤에 루프는 다시 유휴다.
    expect(clock.pending).toBe(0);
    // 그리고 상자는 정말로 바뀌었다 — 바뀌지 않았다면 이 시험은 틀린 이유로 통과한 것이다.
    expect(stageBox().style.width).toBe('740px');
  });

  it('`overflow-hidden` 이 컨테이너에 있다 — DOM 손잡이가 비트맵과 같은 자리에서 잘린다', () => {
    const { view } = renderSurface({ workspace: true });

    const wrapper = view.container.firstElementChild as HTMLElement;
    expect(wrapper.className.split(/\s+/)).toContain('overflow-hidden');
    // 두 상자가 `absolute` 로 앉을 기준이 되는 `relative` 도 함께 있어야 한다.
    expect(wrapper.className.split(/\s+/)).toContain('relative');
  });

  it('퇴화: 아직 재지 못한 상자(0)는 켠 갈래에서도 그리지 않고 NaN 도 내지 않는다', () => {
    const { clock } = renderSurface({ workspace: true, width: 0, height: 0 });

    expect(areaBox().style.width).toBe('0px');
    expect(areaBox().style.height).toBe('0px');
    expect(stageBox().style.width).toBe('0px');
    expect(stageBox().style.height).toBe('0px');
    expect(stageBox().style.left).toBe('0px');
    expect(stageBox().style.top).toBe('0px');
    expect(ops()).not.toContain('clearRect');
    expect(clock.pending).toBe(0);
  });

  it('퇴화: 극단적으로 납작한 상자에서도 축척은 하나이고 출력 영역이 작업 영역을 넘지 않는다', () => {
    // 2000×50 → 줄인 상자 1500×37 → 한 칸 2px → 출력 영역 40×32, 원점 (980, 9).
    // 한 칸이 1px 미만이 아니므로 정수화가 살아 있고, 그래도 상자가 뒤집히지 않는다.
    renderSurface({ workspace: true, width: 2000, height: 50 });

    expect(areaBox().style.width).toBe('2000px');
    expect(areaBox().style.height).toBe('50px');
    expect(stageBox().style.width).toBe('40px');
    expect(stageBox().style.height).toBe('32px');
    expect(stageBox().style.left).toBe('980px');
    expect(stageBox().style.top).toBe('9px');
    // 줄이는 방향이 뒤집히지 않는다.
    expect(px(stageBox().style.width)).toBeLessThanOrEqual(px(areaBox().style.width));
    expect(px(stageBox().style.height)).toBeLessThanOrEqual(px(areaBox().style.height));
    // 축척이 하나다 — 칸이 두 축 모두 2px 다.
    expect(40 / (WIDE_CANVAS.width / 25)).toBe(32 / (WIDE_CANVAS.height / 25));
    // NaN 이 상자에도 좌표계에도 번지지 않는다.
    for (const value of transforms().flat()) {
      expect(Number.isFinite(value as number)).toBe(true);
    }
    expect(rects()).toEqual([
      [8, 8, 8, 8],
      [-8, -4, 8, 8],
      [36, 28, 8, 8],
    ]);
  });
});

// --- 격자 컨텍스트가 원점과 작업 영역을 나른다 (SPEC-CANVAS-006 M3) ---
//
// M2 가 상자 둘을 **DOM 으로** 지었다면, M3 은 그 두 상자에서 나온 값 둘(`origin` · `box`)을
// **오버레이가 쓸 수 있게** 아래로 내린다. 재는 것은 그 통로 하나다 — 격자를 실제로
// 옮기는 것은 M5·M6 의 몫이고 이 절은 값이 닿는가만 본다.
//
// **이 절은 표면과 진짜 컨텍스트 소비자를 함께 세운다**(0.4.0 결함 E 가 세운 이음매 규율).
// 표면이 무엇을 실었는지만 보면(예: `stageGrid` 를 직접 만들어 단언) 컨텍스트가 실제로
// 오버레이 자리에 닿는지는 통째로 빠지고, 컨텍스트를 소비자 쪽에서만 보면 표면이 무엇을
// 실었는지가 빠진다. 그래서 소비자는 **진짜 훅**(`useCanvasStageGrid`)을 부르고, 그 값을
// 표면이 실제로 지은 **DOM 상자**와 맞대어 본다.
//
// 고정 입력은 위 M2 절과 같은 1749×796 × 500×400 이며, 그 이유도 같다. 이 절이 특히
// 기대는 함정 둘을 다시 적는다.
//   - **`workspace={false}` 에서는 `box === stage` 이고 `origin === (0,0)` 이다.** 그러므로
//     끈 갈래에서만 재는 시험은 두 값을 **혼동한 구현도 통과시킨다**(`box` 자리에 `stage`
//     를 실어도, `origin` 을 아예 싣지 않아도 같은 값이 나온다). 이 절의 본 시험은 반드시
//     **켠 갈래**에서 돈다(D1 · D4).
//   - 켠 갈래의 원점 **(504, 102) 는 칸 37 의 배수가 아니다**(504 = 37×13 + 23,
//     102 = 37×2 + 28). 배수인 조합에서는 "작업 영역의 왼쪽 위" 와 "출력 영역의 원점" 이
//     같은 자리라, 격자를 어디에 앉히든 시험이 실패할 수 없다(D3).

describe('CanvasSurface — 격자 컨텍스트의 원점과 작업 영역 (SPEC-CANVAS-006 M3)', () => {
  /** 표면이 편 격자 한 벌을 **진짜 훅으로** 받아 기록하는 소비자. */
  function renderWithConsumer(opts: { workspace: boolean; width?: number; height?: number }) {
    currentSize = { width: opts.width ?? 1749, height: opts.height ?? 796 };
    const clock = makeScheduler();
    const seen: (CanvasStageGrid | null)[] = [];

    function GridProbe() {
      seen.push(useCanvasStageGrid());
      return <div data-testid="canvas-overlay" />;
    }

    const view = render(
      <CanvasSurface
        canvas={WIDE_CANVAS}
        elements={[rectEl('a', { geometry: { x: -100, y: -50, w: 100, h: 100 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
        workspace={opts.workspace}
        overlay={() => <GridProbe />}
      />,
    );
    clock.flush(0);
    return { clock, view, grid: () => seen.at(-1)! };
  }

  it('켠 갈래: 원점과 작업 영역이 표면이 지은 두 상자와 **같은 값**으로 내려온다', () => {
    const { grid } = renderWithConsumer({ workspace: true });

    // 먼저 두 상자가 **실제로 다름**을 못박는다(D4). 같으면 아래 단언 전부가 뜻을 잃는다.
    expect(areaBox().style.width).not.toBe(stageBox().style.width);
    expect(stageBox().style.left).not.toBe('0px');

    const g = grid();
    expect(g).not.toBeNull();
    // 원점 = 출력 영역 상자가 앉은 자리. 오버레이가 다시 파생하지 않고 이 값을 쓴다.
    expect(g.origin).toEqual({ x: 504, y: 102 });
    expect(g.origin.x).toBe(px(stageBox().style.left));
    expect(g.origin.y).toBe(px(stageBox().style.top));
    // 작업 영역 = 격자가 덮어야 할 상자. 잰 바깥 상자 전부다.
    expect(g.box).toEqual({ width: 1749, height: 796 });
    expect(g.box.width).toBe(px(areaBox().style.width));
    expect(g.box.height).toBe(px(areaBox().style.height));
    // 종전 세 칸도 그대로다 — 칸은 두 축이 같은 정수다(축척이 하나다).
    expect(g.cell).toEqual({ x: 37, y: 37 });
    expect(g.step).toBe(25);
    expect(typeof g.setStep).toBe('function');
  });

  it('켠 갈래의 원점은 한 칸의 배수가 **아니다** — 격자 위상 시험이 실패할 수 있는 조합이다', () => {
    // D3: 이 수가 배수가 되는 순간 M5·M6 의 위상 시험은 원점을 무시한 구현도 통과시킨다.
    // 그래서 그 시험이 서기 **전에** 이 조합을 여기서 못박는다.
    const { grid } = renderWithConsumer({ workspace: true });

    const g = grid();
    expect(g.origin.x % g.cell.x).toBe(23); // 504 = 37×13 + 23
    expect(g.origin.y % g.cell.y).toBe(28); // 102 = 37×2 + 28
    expect(g.origin.x % g.cell.x).not.toBe(0);
    expect(g.origin.y % g.cell.y).not.toBe(0);
  });

  it('켠 갈래: 작업 영역이 출력 영역보다 **넓다** — 두 칸을 맞바꾼 구현이 여기서 걸린다', () => {
    // `box` 자리에 `stage` 를 실은 구현은 끈 갈래에서 두 값이 같아 통과하지만 여기서 걸린다.
    const { grid } = renderWithConsumer({ workspace: true });

    const g = grid();
    expect(g.box.width).toBeGreaterThan(px(stageBox().style.width));
    expect(g.box.height).toBeGreaterThan(px(stageBox().style.height));
  });

  it('끈 갈래: 원점이 (0,0) 이고 작업 영역이 출력 영역과 같다 — 006 이전과 같은 값이다', () => {
    // D1: 끈 갈래도 따로 잰다. 여기서만 재면 위 세 시험이 못 잡는 혼동이 남지만,
    // 여기를 빼면 무동작 갈래가 한 번도 돌지 않는다. 둘 다 있어야 갈래 둘이 덮인다.
    const { grid } = renderWithConsumer({ workspace: false });

    const g = grid();
    expect(g.origin).toEqual({ x: 0, y: 0 });
    expect(g.box).toEqual({ width: 980, height: 784 });
    expect(g.box.width).toBe(px(stageBox().style.width));
    expect(g.box.height).toBe(px(stageBox().style.height));
    expect(g.cell).toEqual({ x: 49, y: 49 });
  });

  it('퇴화: 아직 재지 못한 상자(0)에서도 두 칸이 NaN 없이 0 으로 내려온다', () => {
    const { grid } = renderWithConsumer({ workspace: true, width: 0, height: 0 });

    const g = grid();
    expect(g.origin).toEqual({ x: 0, y: 0 });
    expect(g.box).toEqual({ width: 0, height: 0 });
    for (const value of [g.origin.x, g.origin.y, g.box.width, g.box.height]) {
      expect(Number.isFinite(value)).toBe(true);
    }
  });

  // --- 보기 배율 (SPEC-CANVAS-006 M9 · REQ-09) ---------------------------
  //
  // 상태의 주인이 표면이라는 것은 **컨텍스트에 실려 내려오는가**로만 확인된다. 오버레이가
  // 제 상태로 들고 있으면 상자를 짓는 쪽이 남의 상태를 되물어야 하고, 그 되묻는 자리가 곧
  // 두 번째 출처다(위험 R1 · 불변식 I10).

  it('배율이 격자 한 벌에 실려 내려온다 — 주인은 상자를 짓는 쪽이다', () => {
    const { grid } = renderWithConsumer({ workspace: true });

    const g = grid();
    expect(g.zoom).toBe(DEFAULT_WORKSPACE_ZOOM);
    expect(typeof g.setZoom).toBe('function');
    // `step`/`setStep` 과 **같은 짝**이다 — 두 값이 같은 자리에 함께 산다.
    expect(typeof g.setStep).toBe('function');
  });

  it('배율을 바꾸면 상자가 다시 지어지고 프레임이 **한 장** 예약된다 (AC-E4 · AC-09 (AS))', () => {
    // 그리는 상자가 실제로 달라지므로 한 장이 필요하다. 중요한 것은 그것이 **종전의 props
    // 변경 경로**(`geometry` 의존성) 그대로이지 새 깨우기 경로가 아니라는 것이다 —
    // 격자 간격 변경과 같은 부류다.
    const { clock, grid } = renderWithConsumer({ workspace: true });
    expect(clock.pending).toBe(0); // 유휴에 들었음을 **먼저** 단언한다
    const before = clock.requested;
    // 기본 배율(0.75)의 상자를 먼저 못박는다 — 바뀌지 않으면 이 시험은 틀린 이유로 통과한다.
    expect(stageBox().style.width).toBe('740px');
    expect(stageBox().style.left).toBe('504px');

    act(() => {
      grid().setZoom(0.5);
    });

    expect(clock.requested).toBe(before + 1);
    clock.flush(16);
    expect(clock.pending).toBe(0);

    // 칸 · 상자 · 원점 셋이 **모두** 달라졌다(시험 규율 D8 — 셋 다 달라야 배선을 잰다).
    const g = grid();
    expect(g.zoom).toBe(0.5);
    expect(g.cell).toEqual({ x: 24, y: 24 });
    expect(g.origin).toEqual({ x: 634, y: 206 });
    expect(stageBox().style.width).toBe('480px');
    expect(stageBox().style.height).toBe('384px');
    expect(stageBox().style.left).toBe('634px');
    // 작업 영역은 그대로 잰 상자 전부다 — 배율은 그 상자를 넘지 않는다(불변식 I22).
    expect(g.box).toEqual({ width: 1749, height: 796 });
    expect(areaBox().style.width).toBe('1749px');
  });
});
