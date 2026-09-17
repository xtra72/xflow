// 008 의 회귀 게이트 — 예산 경계 · 유휴 정지 · 출력 영역 밖 · I23 대조
// (SPEC-CANVAS-008 M12 · REQ-05 · REQ-07 · REQ-08 · AC-E3 · AC-E6 · AC-E9 · 불변식 J6).
//
// **경계는 "넘는 쪽" 만으로는 잴 수 없다.** M8 은 상한에서 거절하는 것을 쟀다. 그 시험만
// 있으면 `>=` 를 `>` 로, `>` 를 `>=` 로 바꾼 결함 가운데 **한쪽만** 드러난다 — 반대쪽
// (한 칸 아래는 받아들인다)을 재지 않으면 "아무것도 저장할 수 없다" 가 초록으로 통과한다.
// 그래서 여기서는 **정확히 50** 과 **정확히 49**, **정확히 256KB** 와 **256KB + 1 바이트**를
// 나란히 잰다.
//
// **유휴 정지는 008 에서 다시 재야 한다**(001 REQ-05 · AC-E3). 경로는 명령마다 그리기
// 호출을 내므로 요소 하나가 rect 의 열 배를 그린다. 그 사실이 프레임 예약과 **무관**하다는
// 것이 §명세의 할당 비용 논거를 떠받치는 성질이며, 001 의 시험은 경로 요소가 0개인 채로
// 그것을 쟀다 — 즉 **꺼져 있어서 통과하는 초록**이다.
//
// **출력 영역 밖은 006 이 다른 노드에 대해 못박은 성질이다**(AC-E6). 경로는 상자 기하를
// 쓰므로 그대로 성립해야 하고, `origin ≠ (0,0)` 에서 재지 않으면 원점 항을 빼먹은 결함이
// 보이지 않는다(시험 규율 D8).
//
// @spec SPEC-CANVAS-008 REQ-05 · REQ-07 · REQ-08 · AC-E3 · AC-E6 · AC-E9

import { useState } from 'react';

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import type { VisibilitySource } from '../charts/visiblePolling';

import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import CanvasSurface, { type FrameScheduler } from './CanvasSurface';
import { DEFAULT_CANVAS_SIZE, type CanvasElement, type PathElement } from './canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import { useScratchpadStore } from './scratchpad/scratchpadStore';
import {
  SCRATCHPAD_MAX_BYTES,
  SCRATCHPAD_MAX_ENTRIES,
  scratchpadBytes,
  type ScratchpadEntry,
} from './scratchpad/scratchpadTypes';
import { SHAPE_CATALOG } from './shapes/shapeCatalog';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

/** 비대칭 · 닫힘 — 대칭 도형은 축을 뒤바꾼 결함을 감춘다(D2). */
const TRIANGLE: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

function pathEl(id: string, over: Partial<PathElement> = {}): PathElement {
  return {
    id,
    kind: 'path',
    geometry: { x: 40, y: 30, w: 160, h: 90 },
    path: [...TRIANGLE],
    catalog_id: 'rightTriangle',
    style: { fill: '#123456' },
    ...over,
  };
}

function tinyRect(id = 'el-1'): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} };
}

function entry(id: string, elements: CanvasElement[] = [tinyRect()]): ScratchpadEntry {
  return { id, name: '', created: 1_700_000_000_000, origin: { x: 0, y: 0 }, elements };
}

beforeEach(() => {
  globalThis.localStorage.clear();
  useScratchpadStore.setState({ entries: [], notice: null });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
  globalThis.localStorage.clear();
});

// --- 예산 경계 -----------------------------------------------------------

describe('예산 경계는 **양쪽**을 잰다 (REQ-05 · AC-08)', () => {
  it('정확히 49개일 때는 받아들인다 — 한 칸 아래를 재지 않으면 "아무것도 못 넣는다" 가 통과한다', () => {
    useScratchpadStore.setState({
      entries: Array.from({ length: SCRATCHPAD_MAX_ENTRIES - 1 }, (_u, i) => entry(`sp-${i + 1}`)),
    });
    const result = useScratchpadStore.getState().saveEntry([tinyRect('el-9')]);
    expect(result.status).toBe('stored');
    expect(useScratchpadStore.getState().entries).toHaveLength(SCRATCHPAD_MAX_ENTRIES);
  });

  it('정확히 50개가 되면 거절한다 — 51번째는 없다', () => {
    useScratchpadStore.setState({
      entries: Array.from({ length: SCRATCHPAD_MAX_ENTRIES }, (_u, i) => entry(`sp-${i + 1}`)),
    });
    expect(useScratchpadStore.getState().saveEntry([tinyRect('el-9')]).status).toBe(
      'limit-entries',
    );
    expect(useScratchpadStore.getState().entries).toHaveLength(SCRATCHPAD_MAX_ENTRIES);
  });

  /**
   * 저장 뒤의 목록이 **정확히 `bytes` 바이트**가 되게 하는 요소를 짓는다.
   *
   * 채움 글자가 ASCII 라 JSON 도 UTF-8 도 한 글자에 1 바이트다 — 그래서 길이와 바이트가
   * 선형이고, 한 번 재서 차이만큼 늘리면 정확히 맞는다. `created` 는 아래에서 시각을
   * 얼려 자릿수를 고정한다(13자리).
   */
  function elementsOfExactly(bytes: number): CanvasElement[] {
    const build = (pad: number): CanvasElement[] => [
      { id: 'el-1', kind: 'text', geometry: { x: 0, y: 0 }, text: 'a'.repeat(pad), style: {} },
    ];
    const probe = 1000;
    const measured = scratchpadBytes([
      { ...entry('sp-1'), elements: build(probe) },
    ]);
    return build(probe + (bytes - measured));
  }

  it('저장 뒤 목록이 **정확히 256KB** 면 받아들인다 — 경계는 초과에서만 거절한다', () => {
    vi.spyOn(Date, 'now').mockReturnValue(1_700_000_000_000);
    const elements = elementsOfExactly(SCRATCHPAD_MAX_BYTES);
    const result = useScratchpadStore.getState().saveEntry(elements);

    // 경계 계산이 맞았는지 **먼저** 확인한다 — 어긋난 채로 통과하면 이 시험은 경계가
    // 아니라 그 언저리를 잰 것이 된다.
    expect(scratchpadBytes(useScratchpadStore.getState().entries)).toBe(SCRATCHPAD_MAX_BYTES);
    expect(result.status).toBe('stored');
  });

  it('한 바이트만 더 얹으면 거절한다', () => {
    vi.spyOn(Date, 'now').mockReturnValue(1_700_000_000_000);
    const result = useScratchpadStore.getState().saveEntry(
      elementsOfExactly(SCRATCHPAD_MAX_BYTES + 1),
    );
    expect(result.status).toBe('limit-bytes');
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });
});

// --- 저장소가 죽은 환경 ---------------------------------------------------

describe('저장소가 던져도 밖으로 나가지 않고 편집은 그대로 돈다 (REQ-05)', () => {
  it('`setItem` 이 언제나 던지는 환경에서 저장 · 이름 · 지우기가 전부 예외 없이 끝난다', () => {
    vi.spyOn(globalThis.localStorage.__proto__, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });

    expect(() => {
      const r = useScratchpadStore.getState().saveEntry([tinyRect()]);
      expect(r.status).toBe('volatile');
      const id = r.entry?.id ?? '';
      useScratchpadStore.getState().renameEntry(id, '이름');
      useScratchpadStore.getState().removeEntry(id);
    }).not.toThrow();
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });

  it('저장소가 죽어도 캔버스의 그림과 편집은 그대로다 — 도크가 서고 요소가 잡힌다', () => {
    vi.spyOn(globalThis.localStorage.__proto__, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });
    render(<EditHarness initial={[pathEl('p1')]} />);
    expect(screen.getByTestId('canvas-dock-panel')).toBeTruthy();
    expect(screen.getByTestId('canvas-scratchpad-dropzone')).toBeTruthy();
  });
});

// --- 손상 저장 자료 -------------------------------------------------------

describe('요소가 하나도 살아나지 않는 항목은 버리고 나머지는 살린다 (REQ-05)', () => {
  it('항목의 요소가 **전부** 파서에서 떨어지면 그 항목만 사라진다', () => {
    globalThis.localStorage.setItem(
      'xflow-canvas-scratchpad',
      JSON.stringify({
        version: 0,
        state: {
          entries: [
            // 살아남는 항목.
            { id: 'sp-1', name: 'ok', created: 1, origin: { x: 0, y: 0 }, elements: [tinyRect()] },
            // 요소가 전부 정체성을 잃었다 — id 없음 · 모르는 종류.
            {
              id: 'sp-2',
              name: 'dead',
              created: 2,
              origin: { x: 0, y: 0 },
              elements: [{ kind: 'rect' }, { id: 'x', kind: 'hologram' }],
            },
          ],
        },
      }),
    );

    // 재수화는 예외를 내지 않는다.
    expect(() => useScratchpadStore.persist.rehydrate()).not.toThrow();
    const ids = useScratchpadStore.getState().entries.map((e) => e.id);
    expect(ids).toEqual(['sp-1']);
  });
});

// --- 유휴 정지 (AC-E3) ----------------------------------------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;

class ImmediateResizeObserver {
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

/** 그리기 호출을 센다 — "그리기가 많다" 와 "프레임을 더 예약한다" 는 다른 축이다. */
function countingCtx() {
  const calls: string[] = [];
  return new Proxy({} as Record<string, unknown>, {
    get(_t, prop: string) {
      if (prop === '__calls') return calls;
      // 인자는 세지 않는다 — 여기서 재는 것은 **호출 수**다(인자는 `drawElement` 시험의 몫).
      return () => {
        calls.push(prop);
        return prop === 'measureText' ? { width: 0 } : undefined;
      };
    },
    set() {
      return true;
    },
  }) as unknown as CanvasRenderingContext2D & { __calls: string[] };
}

describe('경로가 있어도 유휴 정지가 그대로다 (AC-E3 · 001 REQ-05)', () => {
  let ctx: ReturnType<typeof countingCtx>;

  beforeEach(() => {
    ctx = countingCtx();
    vi.stubGlobal('ResizeObserver', ImmediateResizeObserver as unknown as typeof ResizeObserver);
    vi.stubGlobal('devicePixelRatio', 2);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx);
  });

  for (const count of [1, 20]) {
    it(`경로 ${count}개 · 트윈 없음 — 마운트에 한 장, 그 뒤로 예약이 없다`, () => {
      const clock = makeScheduler();
      const elements = Array.from({ length: count }, (_u, i) => pathEl(`p${i + 1}`));
      render(
        <CanvasSurface
          canvas={{ ...DEFAULT_CANVAS_SIZE }}
          elements={elements}
          targetStyles={{}}
          texts={{}}
          scheduler={clock.scheduler}
          visibilitySource={ALWAYS_VISIBLE}
        />,
      );
      expect(clock.requested).toBe(1);
      clock.flush(0);
      // 그렸음을 **먼저** 단언한다 — 아무것도 그리지 않았다면 "예약이 없다" 는 공허하다.
      expect(ctx.__calls.filter((c) => c === 'moveTo').length).toBeGreaterThanOrEqual(count);
      expect(clock.requested).toBe(1);
      expect(clock.pending).toBe(0);
    });
  }

  it('경로 스무 개는 rect 스무 개보다 훨씬 많이 그리지만 예약 계수는 **같다**', () => {
    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={{ ...DEFAULT_CANVAS_SIZE }}
        elements={Array.from({ length: 20 }, (_u, i) => pathEl(`p${i + 1}`))}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />,
    );
    clock.flush(0);
    const pathDraws = ctx.__calls.length;
    const pathFrames = clock.requested;
    cleanup();

    ctx = countingCtx();
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx);
    const clock2 = makeScheduler();
    render(
      <CanvasSurface
        canvas={{ ...DEFAULT_CANVAS_SIZE }}
        elements={Array.from({ length: 20 }, (_u, i) => tinyRect(`r${i + 1}`))}
        targetStyles={{}}
        texts={{}}
        scheduler={clock2.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />,
    );
    clock2.flush(0);

    expect(pathDraws).toBeGreaterThan(ctx.__calls.length);
    expect(pathFrames).toBe(clock2.requested);
    expect(clock2.pending).toBe(0);
  });
});

// --- 편집 표면 ------------------------------------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { width: 500, height: 400 },
};

function EditHarness({
  initial,
  docked = true,
}: {
  initial: readonly CanvasElement[];
  docked?: boolean;
}) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
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
    <I18nProvider>
      <CanvasEditSelectionContext value={state}>
        {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
      </CanvasEditSelectionContext>
    </I18nProvider>
  );
}

function stubOverlayRect(): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: PROJ.stage.width,
    height: PROJ.stage.height,
    right: PROJ.stage.width,
    bottom: PROJ.stage.height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

// --- 출력 영역 밖 (AC-E6) --------------------------------------------------

describe('경로는 출력 영역 밖에도 산다 (AC-E6 · REQ-08 · 006 계승)', () => {
  /**
   * 캔버스 밖으로 걸친 경로. 상자가 `x = -60` 이라 왼쪽 조각이 출력 영역 바깥이고,
   * 오른쪽 조각은 안에 있다 — 그 조각을 찍어 잡는다.
   */
  const OUTSIDE = pathEl('p1', { geometry: { x: -60, y: 30, w: 160, h: 90 } });

  it('음수 좌표의 경로가 골라진다 — 상자를 안쪽으로 죄지 않는다', () => {
    render(<EditHarness initial={[OUTSIDE]} />);
    stubOverlayRect();
    // 축척 0.5 → px 상자 (-30, 15, 80, 45). 왼쪽 조각이 출력 영역 **밖**이다.
    // 삼각형 내부는 로컬 `ly > lx` 이므로 px (12, 56) = 로컬 (5250, 9111) 을 찍는다.
    fireEvent(
      screen.getByTestId('canvas-edit-overlay'),
      new MouseEvent('pointerdown', { clientX: 12, clientY: 56, bubbles: true, cancelable: true }),
    );
    expect(screen.getByTestId('canvas-selection-p1')).toBeTruthy();
  });

  it('고른 뒤 방향키가 그 요소를 옮긴다 — 밖에 있어도 편집이 닿는다', () => {
    let seen: readonly CanvasNode[] = [];
    function Spy() {
      const [elements, setElements] = useState<readonly CanvasNode[]>([OUTSIDE]);
      seen = elements;
      const state = useCanvasEditSelectionState();
      return (
        <I18nProvider>
          <CanvasEditSelectionContext value={state}>
            <CanvasEditDockRegion enabled>
              <CanvasEditOverlay
                enabled
                elements={elements}
                projection={PROJ}
                textWidths={{}}
                onElementsChange={setElements}
              />
            </CanvasEditDockRegion>
          </CanvasEditSelectionContext>
        </I18nProvider>
      );
    }
    render(<Spy />);
    stubOverlayRect();
    fireEvent(
      screen.getByTestId('canvas-edit-overlay'),
      new MouseEvent('pointerdown', { clientX: 12, clientY: 56, bubbles: true, cancelable: true }),
    );
    // **손을 뗀 뒤에 누른다.** 끌기가 도는 동안 방향키가 먹으면 한 몸짓이 두 벌의 델타를
    // 요소에 얹는다 — 오버레이가 그 자리를 막고 있으며(실측), 그 성질을 여기 적어 둔다.
    fireEvent(
      screen.getByTestId('canvas-edit-overlay'),
      new MouseEvent('pointerup', { clientX: 12, clientY: 56, bubbles: true, cancelable: true }),
    );
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'ArrowLeft' });

    const moved = seen[0];
    expect(moved?.kind).toBe('path');
    // 더 바깥으로 나가는 방향인데도 죄이지 않는다(가정 A5).
    expect((moved as PathElement).geometry.x).toBeLessThan(-60);
  });
});

// --- I23 대조 (AC-E9 · 불변식 J6) -----------------------------------------

describe('표시 층과 그 손잡이의 렌더 조건이 같다 — 층→컨트롤 표 (AC-E9 · I23 · D10)', () => {
  /**
   * **층과 그 컨트롤을 한 표에 적고 두 표면을 갈아 끼운다.**
   *
   * 006 이 배달한 결함의 형상은 "조건 없이 그려지는 층 + 도크 뒤에만 있는 컨트롤" 이었고,
   * 각 층에도 각 컨트롤에도 제 시험이 있었지만 **둘을 나란히 본 시험이 없었다.**
   *
   * 008 의 답은 층을 하나도 더하지 않는 것이다 — 카탈로그도 서랍도 **컨트롤**이며 캔버스
   * 표면에 아무것도 그리지 않는다. 그래서 이 표의 `layer` 자리는 008 이 그리는 요소
   * (경로 요소의 외곽선 · 핸들)가 채우고, 그 컨트롤은 두 표면 **모두**에 있어야 한다.
   */
  const TABLE: ReadonlyArray<{
    name: string;
    /** 캔버스 표면에 그려지는 층의 testid(도크와 무관하게 서야 한다). */
    layer: string;
    /** 그 층을 다스리는 컨트롤의 testid(층과 **같은 조건**으로 서야 한다). */
    control: string;
  }> = [
    { name: '경로 요소의 선택 외곽선', layer: 'canvas-selection-p1', control: 'canvas-handle-se' },
  ];

  /** 도크 안에서만 사는 컨트롤 — 층을 만들지 않으므로 도크가 없으면 없어도 된다. */
  const DOCK_ONLY: readonly string[] = [
    'canvas-scratchpad-dropzone',
    'canvas-scratchpad-save',
    'canvas-palette-group-general',
    'canvas-palette-group-basic',
    'canvas-palette-group-arrow',
  ];

  function selectPath(): void {
    stubOverlayRect();
    fireEvent(
      screen.getByTestId('canvas-edit-overlay'),
      new MouseEvent('pointerdown', { clientX: 45, clientY: 55, bubbles: true, cancelable: true }),
    );
  }

  for (const docked of [true, false]) {
    it(`dockHost ${docked ? '!==' : '==='} null — 층이 서면 그 컨트롤도 선다`, () => {
      render(<EditHarness initial={[pathEl('p1')]} docked={docked} />);
      selectPath();
      for (const row of TABLE) {
        const layer = screen.queryByTestId(row.layer);
        const control = screen.queryByTestId(row.control);
        // **관계**를 잰다 — 둘 다 있거나 둘 다 없다. 한쪽만 있으면 I23 이 깨진 것이다.
        expect(layer !== null, `${row.name}: layer`).toBe(control !== null);
        // 그리고 008 의 경로 요소는 두 표면 **모두**에서 다스려진다.
        expect(layer, `${row.name}: layer(${docked})`).not.toBeNull();
        expect(control, `${row.name}: control(${docked})`).not.toBeNull();
      }
    });
  }

  it('도크 전용 컨트롤은 도크가 있을 때만 있고 — 그 "없음" 은 층을 남기지 않는다', () => {
    render(<EditHarness initial={[pathEl('p1')]} docked />);
    selectPath();
    for (const id of DOCK_ONLY) expect(screen.queryByTestId(id), id).not.toBeNull();
    cleanup();

    render(<EditHarness initial={[pathEl('p1')]} docked={false} />);
    selectPath();
    for (const id of DOCK_ONLY) expect(screen.queryByTestId(id), id).toBeNull();
    // 008 이 캔버스 표면에 더한 노드가 하나도 없다 — 드래그 강조도 목록도.
    const overlay = screen.getByTestId('canvas-edit-overlay');
    expect(overlay.querySelectorAll('[data-testid^="canvas-scratchpad-"]')).toHaveLength(0);
    expect(overlay.querySelectorAll('[data-testid^="canvas-catalog-"]')).toHaveLength(0);
  });
});

// --- 카탈로그 30종이 한 종도 빠짐없이 놓인다 --------------------------------

describe('카탈로그 30종 전량이 실제로 놓인다 (품질 게이트 Tested)', () => {
  it('30칸을 하나씩 누르면 경로 요소가 **30개** 붙고 id 도 명령도 겹치지 않는다', () => {
    // 요소 배열은 오버레이의 상태 안에 있으므로 **렌더마다 붙잡아** 마지막 값을 읽는다.
    // 이것을 하지 않고 카탈로그 정의만 다시 세면 시험 이름이 약속한 "놓인다" 를 아무도
    // 재지 않는다 — 이 SPEC 이 내내 겨눈 그 형상이다.
    let seen: readonly CanvasNode[] = [];
    function Spy() {
      const [elements, setElements] = useState<readonly CanvasNode[]>([]);
      seen = elements;
      const state = useCanvasEditSelectionState();
      return (
        <I18nProvider>
          <CanvasEditSelectionContext value={state}>
            <CanvasEditDockRegion enabled>
              <CanvasEditOverlay
                enabled
                elements={elements}
                projection={PROJ}
                textWidths={{}}
                onElementsChange={setElements}
              />
            </CanvasEditDockRegion>
          </CanvasEditSelectionContext>
        </I18nProvider>
      );
    }
    render(<Spy />);
    // 011 이후 `기본` 은 펼쳐진 채로 태어난다(REQ-01) — 무조건 누르면 **닫힌다.**
    // 여는 몸짓이 아니라 "열려 있다" 는 결과를 만든다.
    for (const id of ['general', 'basic', 'arrow']) {
      const head = screen.getByTestId(`canvas-palette-group-${id}`);
      if (head.getAttribute('aria-expanded') === 'false') fireEvent.click(head);
    }
    expect(SHAPE_CATALOG).toHaveLength(30);
    for (const shape of SHAPE_CATALOG) {
      fireEvent.click(screen.getByTestId(`canvas-catalog-add-${shape.id}`));
    }

    expect(seen).toHaveLength(30);
    // 한 종도 빠뜨리지 않았다 — 놓인 차례가 곧 카탈로그의 차례다.
    expect(seen.map((el) => (el as PathElement).catalog_id)).toEqual(
      SHAPE_CATALOG.map((s) => s.id),
    );
    expect(new Set(seen.map((el) => el.id)).size).toBe(30);
    for (const [i, el] of seen.entries()) {
      const shape = SHAPE_CATALOG[i];
      expect(el.kind, shape?.id).toBe('path');
      // **값이지 참조가 아니다** — 명령이 카탈로그의 그 배열이면 요소를 고칠 때 카탈로그가
      // 함께 바뀐다.
      expect((el as PathElement).path).toEqual(shape?.path);
      expect((el as PathElement).path).not.toBe(shape?.path);
    }
  });
});
