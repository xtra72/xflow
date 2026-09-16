// 그룹 투명도 — 편집 칸에서 저장을 지나 **렌더 곱셈까지** (SPEC-CANVAS-009 M1).
//
// ## 이 파일이 겨누는 이음매
//
// 009 spec.md 는 "③ 은 이미 동작한다 — `bakeStyle` 이 곱해 굽고 있다" 고 적었다. **실측은
// 반대였다.** `bakeStyle` 의 호출자는 `ungroupNode` 하나뿐이었고, `buildCanvasFrame` 은
// 그룹을 통째로 건너뛰어(`if (isGroup(el)) continue;`) 부품의 복합 키 항목을 **하나도**
// 내지 않았다. 그래서 `drawElements` 의 `styles[key] ?? element.style` 폴백이 조용히 일했고,
// 그룹의 `style` 은 저장은 되지만 화면에 닿지 않았다.
//
// 그 구멍은 **편집기에도 렌더러에도 없다.** 두 층이 각각 100% 여도 그 사이는 덮이지
// 않는다 — 이 저장소가 이미 겪은 그 함정이다. 그래서 이 파일의 시험은 **층을 건넌다**:
// 편집 칸에 숫자를 넣고(목록), 그 값이 config 로 나가고(저장), 같은 config 로 패널을
// 그렸을 때 캔버스에 대입된 `globalAlpha` 를 본다(렌더).
//
// ## 왜 `globalAlpha` 를 기록하는 스텁이 따로 필요한가
//
// `CanvasPanel.test.tsx` 의 스텁은 `globalAlpha` 를 **평범한 속성**으로 둔다. 대입은 되지만
// 남지 않으므로, 마지막 값 하나만 읽히고 "어느 도형에 얼마가 걸렸는가" 를 말하지 못한다.
// 여기서는 대입을 기록하고 `fill` 과 짝지어 도형별 실효 투명도를 읽는다.
//
// @spec SPEC-CANVAS-009 REQ-06 · AC-34 ~ AC-39

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { UseStoreChartDataResult } from '../charts/useStoreChartData';
import type { VisibilitySource } from '../charts/visiblePolling';

function idleResult(): UseStoreChartDataResult {
  return {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  };
}

vi.mock('../charts/useStoreChartData', () => ({
  useStoreChartData: () => idleResult(),
}));
vi.mock('../charts/useTsdbChartData', () => ({
  useTsdbChartData: () => idleResult(),
}));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import CanvasPanel from './CanvasPanel';
import CanvasElementsEditor from './CanvasElementsEditor';
import type { FrameScheduler } from './CanvasSurface';
import type { CanvasNode } from './group/groupTypes';

afterEach(cleanup);

// --- 기록 context 스텁 — `globalAlpha` 를 **남긴다** -------------------------

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
    strokeStyle: '',
    lineWidth: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
    _fillStyle: '' as unknown,
    get fillStyle() {
      return this._fillStyle;
    },
    set fillStyle(v: unknown) {
      this._fillStyle = v;
      calls.push(['fillStyle', v]);
    },
    _globalAlpha: 1,
    get globalAlpha() {
      return this._globalAlpha;
    },
    set globalAlpha(v: number) {
      this._globalAlpha = v;
      calls.push(['globalAlpha', v]);
    },
  };
}

let ctxStub: ReturnType<typeof makeCtxStub>;

/**
 * 채움색 → 그 도형이 칠해진 순간의 `globalAlpha`.
 *
 * `fill` 호출을 기준으로 **직전에 마지막으로 대입된** 두 값을 짝짓는다. 마지막 값 하나만
 * 읽으면 도형이 둘 이상일 때 어느 것의 값인지 알 수 없다.
 */
function alphaByFill(): Map<unknown, number> {
  const out = new Map<unknown, number>();
  let alpha = 1;
  let fill: unknown = '';
  for (const call of ctxStub.calls) {
    if (call[0] === 'globalAlpha') alpha = call[1] as number;
    else if (call[0] === 'fillStyle') fill = call[1];
    else if (call[0] === 'fill') out.set(fill, alpha);
  }
  return out;
}

// --- 가짜 프레임 예약기 ----------------------------------------------------

function makeScheduler() {
  let nextHandle = 1;
  const pending = new Map<number, (nowMs: number) => void>();
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

const ALWAYS_VISIBLE: VisibilitySource = {
  isVisible: () => true,
  subscribe: () => () => {},
};

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { width: 200, height: 160 } }]);
  }
  unobserve() {}
  disconnect() {}
}

beforeEach(() => {
  ctxStub = makeCtxStub();
  vi.stubGlobal('ResizeObserver', TriggeringResizeObserver as unknown as typeof ResizeObserver);
  vi.stubGlobal('devicePixelRatio', 1);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    ctxStub as unknown as CanvasRenderingContext2D,
  );
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// --- 고정 입력 -------------------------------------------------------------
//
// 부품 둘은 **채움색이 서로 다르다** — 같으면 `alphaByFill` 이 둘을 한 칸으로 접어
// "한쪽만 곱해졌다" 를 잡지 못한다.

function groupNode(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    parts: [
      { id: 'body', kind: 'rect', geometry: { x: 0, y: 0, w: 3000, h: 2000 }, style: { fill: '#aaaaaa', opacity: 0.4 } },
      { id: 'stem', kind: 'ellipse', geometry: { x: 4100, y: 3300, w: 1700, h: 900 }, style: { fill: '#bbbbbb' } },
    ],
    ...over,
  };
}

function rectNode(id = 'r1'): Record<string, unknown> {
  return { id, kind: 'rect', geometry: { x: 50, y: 80, w: 150, h: 160 }, style: { fill: '#cccccc' } };
}

function cfg(elements: readonly unknown[]): Record<string, unknown> {
  return { channel_name: '', data_source: 'store', canvas: { width: 500, height: 400 }, elements };
}

function renderPanel(config: Record<string, unknown>) {
  const clock = makeScheduler();
  render(
    <CanvasPanel
      panelId="p1"
      config={config}
      scheduler={clock.scheduler}
      visibilitySource={ALWAYS_VISIBLE}
    />,
  );
  clock.flush(0);
}

function setupEditor(config: Record<string, unknown>) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  return onConfigChange;
}

function lastNodes(spy: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
  return patch.elements as CanvasNode[];
}

/** 그룹 행을 펼친다. 전제(접혀 있었다)를 함께 단언한다. */
function expandGroup(idx: number): void {
  const toggle = screen.getByTestId(`canvas-group-row-toggle-${idx}`);
  expect(toggle, '전제 — 그룹 행은 접힌 채로 태어난다').toHaveAttribute('aria-expanded', 'false');
  fireEvent.click(toggle);
}

// --- 편집 칸 (AC-34 ~ AC-36 · AC-39) ---------------------------------------

describe('그룹 행에 투명도 칸이 선다 (AC-34)', () => {
  it('펼치면 투명도 입력 칸이 있다', () => {
    setupEditor(cfg([rectNode(), groupNode()]));
    // 전제 — 접힌 동안에는 없다. 이 단언이 없으면 아래 "있다" 가 접힘과 무관해진다.
    expect(screen.queryByTestId('canvas-group-opacity-1')).toBeNull();
    expandGroup(1);
    expect(screen.getByTestId('canvas-group-opacity-1')).not.toBeNull();
  });

  it('빈 칸으로 태어난다 — 저술하지 않은 투명도는 `1` 이 아니라 **부재**다', () => {
    setupEditor(cfg([groupNode()]));
    expandGroup(0);
    expect(screen.getByTestId('canvas-group-opacity-0')).toHaveValue(null);
  });

  it('저술된 값이 칸에 **백분율로** 보인다 (012 M5 · AC-24)', () => {
    // 012 가 칸의 눈금을 0..1 에서 0~100% 로 옮겼다. **저장은 0.5 그대로이고** 보이는
    // 수만 50 이다 — 아래 §왕복이 그 사실을 따로 못박는다.
    setupEditor(cfg([groupNode({ style: { opacity: 0.5 } })]));
    expandGroup(0);
    expect(screen.getByTestId('canvas-group-opacity-0')).toHaveValue(50);
  });
});

describe('입력이 그룹 스타일에 기록된다 (AC-35 · AC-36)', () => {
  it('50(%) 을 넣으면 `style.opacity` 가 0.5 다 (012 M5 · AC-25)', () => {
    // **칸은 백분율, 저장은 0..1.** 이 한 줄이 012 §결정 4 의 전부다.
    const spy = setupEditor(cfg([rectNode(), groupNode()]));
    expandGroup(1);
    fireEvent.change(screen.getByTestId('canvas-group-opacity-1'), { target: { value: '50' } });

    const nodes = lastNodes(spy);
    expect(nodes[1]!.style?.opacity).toBe(0.5);
    // 그룹의 나머지는 그대로다 — 투명도 쓰기가 부품을 건드리지 않는다.
    expect((nodes[1] as { parts: unknown[] }).parts).toHaveLength(2);
    // 형제도 그대로다.
    expect(nodes[0]!.id).toBe('r1');
  });

  it('150(%) 은 1 로, -50(%) 은 0 으로 죈다 (012 AC-27)', () => {
    // 죄는 **뜻**은 그대로이고 눈금만 옮겼다 — 범위 밖 입력이 저장에 새지 않는다.
    for (const [raw, expected] of [
      ['150', 1],
      ['-50', 0],
    ] as const) {
      const spy = setupEditor(cfg([groupNode()]));
      expandGroup(0);
      fireEvent.change(screen.getByTestId('canvas-group-opacity-0'), { target: { value: raw } });
      expect(lastNodes(spy)[0]!.style?.opacity, raw).toBe(expected);
      cleanup();
    }
  });

  it('칸을 비우면 키 자체가 사라진다 — 부재와 `0` 은 다른 뜻이다', () => {
    const spy = setupEditor(cfg([groupNode({ style: { opacity: 0.5 } })]));
    expandGroup(0);
    fireEvent.change(screen.getByTestId('canvas-group-opacity-0'), { target: { value: '' } });
    expect('opacity' in (lastNodes(spy)[0]!.style ?? {})).toBe(false);
  });
});

describe('최상위 요소 행의 칸과 **같은 컨트롤**이다 (AC-39)', () => {
  it('같은 `<input type=number>` · 같은 하한·상한이다', () => {
    setupEditor(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    expandGroup(1);

    const elementField = screen.getByTestId('canvas-element-opacity-0');
    const groupField = screen.getByTestId('canvas-group-opacity-1');
    for (const attr of ['type', 'step', 'min', 'max']) {
      expect(groupField.getAttribute(attr), attr).toBe(elementField.getAttribute(attr));
    }
    // 같은 i18n 키를 쓴다 — 새 키를 짓지 않았다는 사실을 자리표시자가 말한다.
    expect(groupField.getAttribute('placeholder')).toBe(elementField.getAttribute('placeholder'));
  });

  it('같은 클램프를 지난다 — 두 칸에 150(%) 을 넣으면 둘 다 1 이다', () => {
    // 012 M5 이후 그 "같은 클램프" 는 `opacityPercent.percentInputToOpacity` 한 쌍이다.
    // 두 칸이 같은 함수를 지나는 것이 AC-28 이고, 이 단언이 그 결과를 본다.
    const spy = setupEditor(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    fireEvent.change(screen.getByTestId('canvas-element-opacity-0'), { target: { value: '150' } });
    expect(lastNodes(spy)[0]!.style?.opacity).toBe(1);

    expandGroup(1);
    fireEvent.change(screen.getByTestId('canvas-group-opacity-1'), { target: { value: '150' } });
    expect(lastNodes(spy)[1]!.style?.opacity).toBe(1);
  });
});

// --- 렌더 곱셈 (AC-37 · AC-38) ---------------------------------------------
//
// 여기가 009 가 실제로 메운 구멍이다. 위 편집 칸만으로는 값이 저장될 뿐 화면에 닿지 않았다.

describe('렌더가 그룹 투명도와 부품 투명도를 **곱해** 적용한다 (AC-37)', () => {
  it('그룹 0.5 · 부품 0.4 → 실효 0.2', () => {
    renderPanel(cfg([groupNode({ style: { opacity: 0.5 } })]));
    expect(alphaByFill().get('#aaaaaa')).toBeCloseTo(0.2, 10);
  });

  it('같은 그룹의 다른 부품은 제 값으로 곱해진다 — 한 값이 모두를 덮지 않는다', () => {
    renderPanel(cfg([groupNode({ style: { opacity: 0.5 } })]));
    const alphas = alphaByFill();
    // `stem` 은 제 투명도가 없으므로 그룹 값만 걸린다(AC-38 과 같은 규칙).
    expect(alphas.get('#bbbbbb')).toBeCloseTo(0.5, 10);
    expect(alphas.get('#aaaaaa')).toBeCloseTo(0.2, 10);
  });
});

describe('그룹 투명도만 있어도 적용된다 (AC-38)', () => {
  it('부품에 투명도가 없으면 그룹 값이 그대로 실효 투명도다', () => {
    renderPanel(
      cfg([
        groupNode({
          style: { opacity: 0.5 },
          parts: [
            { id: 'solo', kind: 'rect', geometry: { x: 0, y: 0, w: 3000, h: 2000 }, style: { fill: '#123456' } },
          ],
        }),
      ]),
    );
    expect(alphaByFill().get('#123456')).toBeCloseTo(0.5, 10);
  });
});

describe('뒤집지 않은 것 — 최상위 경로는 004 와 같다', () => {
  it('최상위 요소의 투명도는 제 값 하나뿐이다 (그룹이 없으니 곱할 것도 없다)', () => {
    renderPanel(cfg([{ ...rectNode(), style: { fill: '#cccccc', opacity: 0.25 } }]));
    expect(alphaByFill().get('#cccccc')).toBeCloseTo(0.25, 10);
  });

  it('그룹에 투명도가 없으면 부품은 제 값 그대로다 — 구운 결과가 값을 지어내지 않는다', () => {
    renderPanel(cfg([groupNode()]));
    const alphas = alphaByFill();
    expect(alphas.get('#aaaaaa')).toBeCloseTo(0.4, 10);
    // 양쪽 미지정이면 `1` 을 지어 넣지 않는다 — 렌더 기본값과 구별되지 않아야 한다(G12).
    expect(alphas.get('#bbbbbb')).toBeCloseTo(1, 10);
  });

  it('그룹 밖 형제는 그룹 투명도에 물들지 않는다', () => {
    renderPanel(cfg([groupNode({ style: { opacity: 0.5 } }), rectNode()]));
    expect(alphaByFill().get('#cccccc')).toBeCloseTo(1, 10);
  });
});
