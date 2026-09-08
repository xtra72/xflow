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
import { render, cleanup, act } from '@testing-library/react';

import type { VisibilitySource } from '../charts/visiblePolling';
import type { RectElement } from './canvasConfig';
import CanvasSurface, { type FrameScheduler } from './CanvasSurface';

// --- ResizeObserver 오버라이드(HeatmapCanvas.test 선례) -------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
let roInstances: { cb: RoCallback }[] = [];
let roDisconnects = 0;
let currentSize = { width: 100, height: 80 };

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

function rectEl(id: string, over: Partial<RectElement> = {}): RectElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: {}, ...over };
}

const LINEAR_300 = { duration_ms: 300, easing: 'linear' } as const;

beforeEach(() => {
  roInstances = [];
  roDisconnects = 0;
  currentSize = { width: 100, height: 80 };
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
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);

    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(200); // 100 × dpr 2
    expect(canvas.height).toBe(160); // 80 × dpr 2
    expect(canvas.style.width).toBe('100px');
    expect(canvas.style.height).toBe('80px');
  });

  it('한 프레임은 전체를 지우고 DPR 배율을 건 뒤 요소를 다시 그린다', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
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
    expect(ctxStub.calls.find((c) => c[0] === 'fillRect')?.slice(1)).toEqual([0, 0, 200, 160]);
    // 배경을 그린 뒤 DPR 배율을 세운다 — 이후 좌표는 CSS px 다.
    expect(ctxStub.calls.filter((c) => c[0] === 'setTransform').map((c) => c.slice(1))).toEqual([
      [1, 0, 0, 1, 0, 0],
      [2, 0, 0, 2, 0, 0],
    ]);
    // 정규화 (0,0,1,1) → CSS px 전체 (0,0,100,80).
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([0, 0, 100, 80]);
  });

  it('리사이즈되면 백킹 버퍼를 재계산하고 같은 상대 위치로 다시 그린다', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
        elements={[rectEl('a', { geometry: { x: 0.5, y: 0.5, w: 0.5, h: 0.5 } })]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(200);
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([50, 40, 50, 40]);

    ctxStub.calls.length = 0;
    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 50, height: 40 } }]);
    });
    clock.flush(16);

    expect(canvas.width).toBe(100); // 50 × dpr 2
    expect(canvas.height).toBe(80);
    // 요소는 화면상 같은 상대 위치(우하 사분면)에 남는다.
    expect(ctxStub.calls.find((c) => c[0] === 'rect')?.slice(1)).toEqual([25, 20, 25, 20]);
  });

  it('표시 크기가 0 이면 그리지 않는다(아직 그릴 수 없음 — 루프도 깨우지 않는다)', () => {
    currentSize = { width: 0, height: 0 };
    const clock = makeScheduler();
    render(
      <CanvasSurface
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
        elements={[rectEl('a')]}
        targetStyles={{ a: { fill: '#ff0000' } }}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={makeVisibility().source}
      />,
    );
    clock.flush(0);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(100);
    expect(canvas.height).toBe(80);
  });

  it('2D context 를 얻지 못하면 조용히 그리지 않는다(렌더 예외 없음)', () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const clock = makeScheduler();
    expect(() => {
      render(
        <CanvasSurface
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
      <CanvasSurface elements={[rectEl('a')]} targetStyles={{}} texts={{}} />,
    );
    view.unmount();
    expect(cancelSpy).toHaveBeenCalled();
  });

  it('className 을 주면 컨테이너 클래스를 대체한다', () => {
    const clock = makeScheduler();
    const { container } = render(
      <CanvasSurface
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
