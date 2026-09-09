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

    // 컨테이너 → 그리는 상자 → 캔버스. 가운데 상자는 0.9.0 이 격자 자투리를 실제
    // DOM 으로 만든 것이며 **오버레이와 무관하게 언제나** 있다(슬롯을 써도 안 써도 같다).
    // AC-E1 이 금지하는 것은 "오버레이 때문에 생기는 노드" 이고, 그것은 여전히 0 개다.
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.children).toHaveLength(1);
    const box = screen.getByTestId('canvas-stage');
    expect(wrapper.children[0]).toBe(box);
    expect(box.children).toHaveLength(1);
    expect(box.children[0]!.tagName).toBe('CANVAS');
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

  it('오버레이는 캔버스 뒤 형제로 **그리는 상자 안에** 렌더된다', () => {
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

    // 같은 상자 안의 형제라는 사실이 이 시험의 전부다 — 오버레이가 제
    // `getBoundingClientRect()` 로 재는 상자와 `projection.stage` 가 같은 노드가 되려면
    // 둘이 **그리는 상자 안에** 함께 있어야 한다(0.9.0 · 위험 R1).
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.children).toHaveLength(1);
    const box = wrapper.children[0] as HTMLElement;
    expect(box).toBe(screen.getByTestId('canvas-stage'));
    expect(box.children).toHaveLength(2);
    expect(box.children[0]!.tagName).toBe('CANVAS');
    expect(box.children[1]).toBe(screen.getByTestId('canvas-overlay'));
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
    expect(stageBox().style.left).toBe('384px');
    expect(stageBox().style.top).toBe('6px'); // (796-784)/2
  });

  it('캔버스와 오버레이가 **그 상자 안에** 함께 산다 (측정원이 하나다)', () => {
    const seen: CanvasOverlayContext[] = [];
    renderAt(1749, 796, (ctx) => {
      seen.push(ctx);
      return <div data-testid="canvas-overlay" />;
    });
    const spy = { last: seen.at(-1) };

    const box = stageBox();
    expect(box.contains(screen.getByTestId('canvas-surface'))).toBe(true);
    expect(box.contains(screen.getByTestId('canvas-overlay'))).toBe(true);
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
