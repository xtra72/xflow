// 보기 팬 — **표면과 오버레이 사이의 이음매**를 재는 자리 (SPEC-CANVAS-006 영역 ·
// 사용자 신고 2026-09-16 "줌 확대시 보이는 패널 영역 이동 가능. 마우스를 누른 상태에서 이동").
//
// ## 왜 이 파일이 따로 있는가
//
// `canvasWorkspace.test.ts` 가 팬의 **산술**을 값으로 못박았고, `CanvasEditOverlay.test.tsx`
// 가 키와 커서의 **국소 동작**을 잰다. 둘 다 초록이어도 팬은 죽어 있을 수 있다 — 팬이
// 화면을 옮기려면 오버레이가 잰 손짓이 컨텍스트를 타고 표면의 상자 산술에 닿아 **진짜 DOM
// 상자**(`canvas-stage`)를 옮겨야 하고, 그 층 건너는 경로는 어느 쪽 단위 시험에도 들어 있지
// 않다(이 저장소가 편집기↔렌더러 이음매에서 이미 한 번 데인 자리다).
//
// 그래서 이 파일은 **진짜 `CanvasSurface` 안에 진짜 `CanvasEditOverlay` 를 넣고** 손짓을
// 쏜 뒤, 옮겨졌는지를 **DOM 상자의 `left`/`top`** 으로 잰다. 값이 아니라 픽셀이 증인이다.
//
// ## 고정 입력
//
// 006 이 이미 못박아 둔 조합을 그대로 쓴다(`CanvasSurface.test.tsx` §격자 컨텍스트).
//   - 잰 바깥 상자 **1749 × 796**, 캔버스 **500 × 400**, 간격 25, 기본 배율 0.75
//   - 그 조합에서 줄인 상자 1311 × 597 · 칸 37 · 출력 영역 **740 × 592** · 원점 **(504, 102)**
//   - 따라서 투영 축척은 740 / 500 = **1.48** 이다(두 축이 같다 — 축척은 하나다).
//
// 원점이 (0,0) 이 아닌 것이 요점이다 — 0 이면 "팬만큼 옮겼다" 와 "팬을 그대로 원점으로
// 썼다" 가 같은 수를 내어 시험이 실패할 수 없다.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup, act, fireEvent, screen } from '@testing-library/react';

// i18n 은 키를 그대로 돌려준다(이웃 시험 파일들의 관용구).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { CanvasSize, RectElement } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import CanvasSurface, { type FrameScheduler } from './CanvasSurface';

// --- ResizeObserver · 2D context 스텁 (`CanvasSurface.test.tsx` 선례) -----

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
let currentSize = { width: 1749, height: 796 };

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { width: currentSize.width, height: currentSize.height } }]);
  }
  unobserve() {}
  disconnect() {}
}

/** jsdom 은 2D context 를 주지 않는다. 이 파일은 칠한 결과를 보지 않으므로 빈 스텁이면 된다. */
function makeCtxStub() {
  const noop = () => {};
  return new Proxy(
    { measureText: () => ({ width: 0 }), canvas: { width: 0, height: 0 } },
    {
      get: (target, key) =>
        key in target ? (target as Record<string, unknown>)[key as string] : noop,
      set: () => true,
    },
  );
}

function makeScheduler() {
  const pending = new Map<number, (nowMs: number) => void>();
  let nextHandle = 1;
  const scheduler: FrameScheduler = {
    request(cb) {
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
    flush(nowMs: number) {
      const due = [...pending.values()];
      pending.clear();
      act(() => {
        for (const cb of due) cb(nowMs);
      });
    },
  };
}

// --- 고정 입력 -----------------------------------------------------------

const CANVAS: CanvasSize = { width: 500, height: 400 };
/** 기본 배율 0.75 에서의 원점(006 이 못박은 수). */
const HOME = { x: 504, y: 102 };
/** 그 배율에서의 출력 영역. 죔이 지키는 중심을 이 크기로 구한다. */
const STAGE = { width: 740, height: 592 };
/** 투영 축척 — 두 축이 같다. 캔버스 좌표를 화면 px 로 옮길 때 쓴다. */
const SCALE = STAGE.width / 500;

/**
 * 캔버스 **밖** 왼쪽에 놓인 요소. 저술은 합법이고(좌표를 죄지 않는다) 팬이 없으면 닿지
 * 못하는 그 요소다 — 이 기능이 존재하는 이유의 화신이다.
 */
function outsideRect(): RectElement {
  // x = -500 인 것에 뜻이 있다. 저술 여백은 원점만큼(504px)이고 축척이 1.48 이므로, 캔버스
  // 좌표 -340 까지는 **팬 없이도** 여백 안에 보인다 — 그보다 가까운 자리를 고르면 이
  // 파일의 시험이 팬 없이도 통과한다(-500 은 화면에서 원점보다 740px 왼쪽이다).
  return { id: 'far', kind: 'rect', geometry: { x: -500, y: 40, w: 120, h: 80 }, style: {} };
}

// --- 하네스 ---------------------------------------------------------------

interface RenderOpts {
  workspace?: boolean;
}

function renderSeam(opts: RenderOpts = {}) {
  const clock = makeScheduler();
  const element = (workspace: boolean) => (
    <CanvasSurface
      canvas={CANVAS}
      elements={[outsideRect()]}
      targetStyles={{ far: { fill: '#ff0000' } }}
      texts={{}}
      scheduler={clock.scheduler}
      visibilitySource={{ isVisible: () => true, subscribe: () => () => {} }}
      workspace={workspace}
      overlay={(ctx) => (
        <CanvasEditOverlay
          enabled
          elements={[outsideRect()]}
          projection={ctx.projection}
          textWidths={ctx.textWidths}
          onElementsChange={vi.fn()}
        />
      )}
    />
  );
  const view = render(element(opts.workspace ?? true));
  clock.flush(0);
  return {
    clock,
    /** `workspace` **하나만** 뒤집어 다시 렌더한다 — 편집을 껐다 켜는 그 전이다. */
    setWorkspace: (next: boolean) => {
      view.rerender(element(next));
      clock.flush(0);
    },
  };
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

function stageBox(): HTMLElement {
  return screen.getByTestId('canvas-stage');
}

/** `'244px'` → `244`. 팬이 옮겼는지를 이 수로 잰다. */
function px(value: string): number {
  return Number.parseFloat(value);
}

function stageAt(): { x: number; y: number } {
  return { x: px(stageBox().style.left), y: px(stageBox().style.top) };
}

/** jsdom 에 `PointerEvent` 가 없으므로 `MouseEvent` 로 만들고 타입만 포인터로 둔다. */
function send(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  const evt = new MouseEvent(type, {
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  fireEvent(overlayRoot(), evt);
  return evt;
}

/** Space 를 짚는다. 돌려받은 값은 **소비했는가** 다(페이지 스크롤을 막았는가). */
function holdSpace(): boolean {
  return !fireEvent.keyDown(overlayRoot(), { key: ' ' });
}

function releaseSpace(): void {
  fireEvent.keyUp(overlayRoot(), { key: ' ' });
}

/** Space 를 짚은 채 끌고 손을 뗀다 — 사용자가 하는 그 몸짓 전부다. */
function spaceDrag(from: { x: number; y: number }, to: { x: number; y: number }): void {
  holdSpace();
  send('pointerdown', from.x, from.y);
  send('pointermove', to.x, to.y);
  send('pointerup', to.x, to.y);
  releaseSpace();
}

beforeEach(() => {
  currentSize = { width: 1749, height: 796 };
  vi.stubGlobal('ResizeObserver', TriggeringResizeObserver as unknown as typeof ResizeObserver);
  vi.stubGlobal('devicePixelRatio', 1);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    makeCtxStub() as unknown as CanvasRenderingContext2D,
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// --- 이음매 ---------------------------------------------------------------

describe('보기 팬 — 손짓이 표면의 상자를 실제로 옮긴다 (이음매)', () => {
  it('Space 를 짚고 끌면 출력 영역 상자가 **끈 만큼** 옮겨진다', () => {
    renderSeam();

    // 먼저 출발 자리를 못박는다 — 이 수가 (0,0) 이면 아래 단언이 "팬을 원점으로 그대로
    // 쓴 구현" 도 통과시킨다.
    expect(stageAt()).toEqual(HOME);

    spaceDrag({ x: 600, y: 300 }, { x: 700, y: 340 });

    expect(stageAt()).toEqual({ x: HOME.x + 100, y: HOME.y + 40 });
  });

  it('캔버스 **밖**에 놓인 요소가 그 이동으로 작업 영역 안에 들어온다', () => {
    renderSeam();

    // 요소는 캔버스 x = -500 에 있으므로 화면에서는 원점보다 740px 왼쪽이다. 출발 원점이
    // 504 이므로 그 자리는 작업 영역 밖(음수)이다 — **비트맵에 들지 않아 보이지 않는다**.
    const screenXOf = (canvasX: number) => stageAt().x + canvasX * SCALE;
    expect(screenXOf(-500)).toBeLessThan(0);

    // 오른쪽으로 400px 민다. 그만큼 원점이 오른쪽으로 가므로 밖의 요소가 상자 안으로 든다.
    spaceDrag({ x: 400, y: 300 }, { x: 800, y: 300 });

    expect(screenXOf(-500)).toBeGreaterThan(0);
  });

  it('Space 없이 빈 자리를 끌어도 **같은 만큼 옮겨진다** — 011 이 맨손을 팬에 주었다', () => {
    // **뒤집힌 시험이다.** 종전 문장: "Space 없이 같은 곳을 끌면 상자가 꿈쩍도 하지
    // 않는다 — 그 몸짓에는 임자가 있다". 그 임자는 010 의 영역 선택이었고, 011 이
    // 잦기를 근거로 둘을 맞바꿨다(영역 선택은 Ctrl/Cmd 로 갔다). 지우지 않고 뒤집어
    // 두는 것은, 맨손 끌기가 다시 멈춰 서는 날 그것이 회귀임을 이 자리가 말해야 하기
    // 때문이다.
    renderSeam();

    send('pointerdown', 600, 300);
    send('pointermove', 700, 340);
    send('pointerup', 700, 340);

    expect(stageAt()).toEqual({ x: HOME.x + 100, y: HOME.y + 40 });
  });

  it('Ctrl 을 짚고 끌면 상자가 **꿈쩍도 하지 않는다** — 그 몸짓은 영역 선택의 것이다', () => {
    // 위 시험의 짝이다. 011 이 맞바꾼 것이 **배정**이지 경계가 아님을 이 자리가 지킨다 —
    // 팬이 조작키 갈래까지 먹으면 캔버스 안에서 감싸 고르는 길이 사라진다.
    renderSeam();

    send('pointerdown', 600, 300, { ctrlKey: true });
    send('pointermove', 700, 340, { ctrlKey: true });
    send('pointerup', 700, 340, { ctrlKey: true });

    expect(stageAt()).toEqual(HOME);
  });

  // **잉크 위의 맨손 끌기가 여전히 이동이라는 경계는 여기서 재지 않는다.** 이 하네스는
  // 루트의 상자를 스텁하지 않으므로(jsdom 은 0 을 돌려준다) 히트 테스트가 성립하지
  // 않는다 — 여기서 재면 "요소를 맞히지 못해 초록" 인 시험이 된다. 그 경계는 상자를
  // 스텁하는 이웃 파일들이 잰다(`CanvasEditOverlay.test.tsx` §이동 드래그 ·
  // `CanvasEditOverlay.marquee.test.tsx` §요소 위 누름은 종전 그대로다).

  it('편집을 끄면 팬이 화면에서 사라지고, 다시 켜면 **보던 자리로 돌아온다**', () => {
    const { setWorkspace } = renderSeam();
    spaceDrag({ x: 600, y: 300 }, { x: 700, y: 300 });
    expect(stageAt().x).toBe(HOME.x + 100);

    // 꺼진 갈래는 팬을 보지 않는다 — 두 상자가 겹쳐 006 이전의 그림이 된다.
    setWorkspace(false);
    expect(stageAt()).toEqual({ x: 0, y: 0 });

    // **되돌릴 상태를 따로 두지 않았으므로** 다시 켜면 그대로다. 잃음이 아니라 되돌아옴이다.
    setWorkspace(true);
    expect(stageAt().x).toBe(HOME.x + 100);
  });
});

describe('보기 팬 — 몸짓의 규칙', () => {
  it('Space 를 **끄는 도중에 떼어도** 팬은 손을 뗄 때까지 이어진다', () => {
    renderSeam();

    holdSpace();
    send('pointerdown', 600, 300);
    send('pointermove', 650, 300);
    // 여기서 Space 를 뗀다 — 몸짓의 뜻은 **잡는 순간** 한 번만 정해졌다.
    releaseSpace();
    send('pointermove', 700, 300);
    send('pointerup', 700, 300);

    expect(stageAt().x).toBe(HOME.x + 100);
  });

  it('초점이 떠나면 짚은 상태가 풀린다 — 짚은 채로 굳지 않는다', () => {
    // **증인이 바뀐 시험이다**(011). 종전에는 짚음이 풀렸다는 것을 루트의 커서로 쟀고
    // (`style.cursor` 가 `grab` 이 아니게 된다), 011 에서 편 손은 **기본 커서**가 되었다 —
    // 맨손 누름이 어디서나 팬이거나 이동이기 때문이다. 그래서 같은 사실을 자식까지 덮는
    // 후손 변형으로 잰다: 그 덮음은 "손잡이마저 잡히지 않는다" 는 뜻이라 여전히 짚은
    // 동안에만 참이다.
    //
    // 뒤의 절반은 **잴 것이 없어졌다**. 종전: "그 뒤의 누름은 다시 **고르기**다(팬이
    // 아니다)". 011 에서 빈 자리의 맨손 누름은 짚든 짚지 않든 팬이므로 그 절반은 짚음에
    // 대해 아무것도 말하지 못한다 — 갈리는 자리는 **잉크 위**뿐이고, 이 하네스는 히트
    // 테스트가 성립하지 않아 그것을 잴 수 없다(위 절 끝의 주석). 거짓 증인을 남기느니
    // 뺀다.
    renderSeam();

    holdSpace();
    expect(overlayRoot().className).toContain('[&_*]:cursor-grab');
    fireEvent.blur(overlayRoot());
    expect(overlayRoot().className).not.toContain('[&_*]:cursor-grab');
  });

  it('커서가 둘을 차례로 말한다 — 편 손 · 쥔 손, 그리고 다시 편 손', () => {
    // **뒤집힌 시험이다**(011). 종전 문장: "커서가 셋을 차례로 말한다 — 편 손 · 쥔 손 ·
    // **놓은 손**", 그리고 기대값 셋 중 둘이 빈 문자열(브라우저 기본)이었다. 011 에서
    // 맨손 끌기가 팬이 되었으므로 **쉬고 있는 커서 자체가 편 손**이다 — 손을 켜는 규칙을
    // 둘로 두지 않겠다는 것이 그 선택이고(`CanvasEditOverlay` §커서의 우선순위), 그래서
    // 기본 커서가 없어지는 대신 짚음은 **후손 변형**으로만 제 몫을 말한다.
    renderSeam();

    expect(overlayRoot().style.cursor).toBe('grab');
    // 쉬는 동안에는 자식을 덮지 않는다 — 그 자리에서 손잡이는 실제로 잡히므로 손잡이의
    // 커서가 거짓말이 아니다.
    expect(overlayRoot().className).not.toContain('[&_*]:cursor-grab');

    holdSpace();
    expect(overlayRoot().style.cursor).toBe('grab');
    // 짚은 동안에는 손잡이마저 잡히지 않으므로 자식까지 덮어야 커서가 거짓말하지 않는다.
    expect(overlayRoot().className).toContain('[&_*]:cursor-grab');

    send('pointerdown', 600, 300);
    expect(overlayRoot().style.cursor).toBe('grabbing');
    expect(overlayRoot().className).toContain('[&_*]:cursor-grabbing');

    send('pointerup', 600, 300);
    releaseSpace();
    expect(overlayRoot().style.cursor).toBe('grab');
    expect(overlayRoot().className).not.toContain('[&_*]:cursor-grab');
  });

  it('Space+방향키가 같은 일을 한다 — 팬이 포인터 전용 기능이 아니다', () => {
    renderSeam();

    holdSpace();
    fireEvent.keyDown(overlayRoot(), { key: 'ArrowRight' });
    fireEvent.keyDown(overlayRoot(), { key: 'ArrowDown' });
    releaseSpace();

    // 한 번에 24px — 부호는 **끄는 것과 같다**(오른쪽 키는 그림을 오른쪽으로 민다).
    expect(stageAt()).toEqual({ x: HOME.x + 24, y: HOME.y + 24 });
  });

  it('Space 를 짚지 않은 방향키는 **종전 그대로** 이 층을 지나간다', () => {
    renderSeam();

    fireEvent.keyDown(overlayRoot(), { key: 'ArrowRight' });

    expect(stageAt()).toEqual(HOME);
  });
});

describe('보기 팬 — 죔이 지키는 것 (출력 영역의 중심)', () => {
  it('아무리 멀리 끌어도 출력 영역의 **중심이 상자 안에** 남는다', () => {
    renderSeam();

    // 상자 폭의 세 배를 민다 — 죄지 않으면 출력 영역이 통째로 화면을 떠난다.
    spaceDrag({ x: 100, y: 100 }, { x: 100 + 1749 * 3, y: 100 + 796 * 3 });

    const at = stageAt();
    const centre = { x: at.x + STAGE.width / 2, y: at.y + STAGE.height / 2 };
    expect(centre.x).toBeLessThanOrEqual(1749);
    expect(centre.y).toBeLessThanOrEqual(796);
    // 그리고 실제로 **멀리 갔다** — 죔이 팬을 통째로 죽인 것이 아니다.
    expect(at.x).toBeGreaterThan(HOME.x);
  });

  it('넘겨 민 만큼이 **다음 손짓에 빚으로 남지 않는다** — 새 몸짓은 죈 값에서 시작한다', () => {
    renderSeam();

    // 상한을 한참 넘겨 민다. 한 몸짓 **안에서는** 손이 상한 밖에 있는 동안 그림이 서
    // 있는데, 그것은 죔의 성질이지 결함이 아니다(끝에 닿았다는 사실이 그렇게 보인다).
    spaceDrag({ x: 100, y: 300 }, { x: 100 + 5000, y: 300 });
    const stuck = stageAt().x;
    expect(stuck).toBeGreaterThan(HOME.x);

    // **새 몸짓**은 죈 값에서 다시 잰다. 날값을 쌓아 두는 구현이라면 여기서 5000px 을 다
    // 갚기 전까지 꿈쩍하지 않는다 — 사용자에게는 "왼쪽으로는 안 끌린다" 로 보인다.
    spaceDrag({ x: 500, y: 300 }, { x: 470, y: 300 });
    expect(stageAt().x).toBe(stuck - 30);
  });
});
