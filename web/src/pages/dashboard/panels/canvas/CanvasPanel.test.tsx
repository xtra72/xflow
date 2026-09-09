// CanvasPanel 데이터 바인딩 + 견고성 테스트 (SPEC-CANVAS-001 T7/T11).
//
// 덮는 인수 기준: AC-01(4종 렌더 + 첫 일치 행의 채움색), AC-04(문구 토큰 치환),
// AC-E1(요소 0개), AC-E2(시리즈 결측), AC-E3(규칙 미일치), AC-E4(폴링 실패 시 마지막
// 프레임 유지 + 오류 배지), 정적(비바인딩) 요소, NaN/Infinity 방어.
//
// **Provider 를 요구하지 않는다.** `HeatmapPanel.test.tsx` 선례를 그대로 따라 조회
// **전송 계층**(useStoreChartData / useTsdbChartData)만 모킹하고 i18n 은 키를 그대로
// 돌려주도록 바꾼다. 그래서 이 파일은 `usePanelSeriesData` 를 **진짜로** 실행한다 —
// 패널이 원본 config 를 그 계약에 넘긴다는 T7 의 요점이 모킹으로 가려지지 않는다.
// (`inertQueryClient` 는 `QueryClientProvider` 로 감싸야 하는 설정 다이얼로그 쪽 선례이며,
// 패널 단위 테스트는 전송 계층을 걷어내 provider 자체를 불필요하게 만든다.)
//
// 그리기 관찰은 `CanvasSurface.test.tsx` 와 같은 기록 context 스텁 + 가짜 프레임 예약기다.
// 실제 rAF 는 돌지 않는다.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup, act, fireEvent, screen } from '@testing-library/react';

import { useState } from 'react';

import { useUIStore } from '@/stores/uiStore';

import type { UseStoreChartDataResult } from '../charts/useStoreChartData';
import type { ChartEntry } from '../charts/chartChannelTypes';
import type { VisibilitySource } from '../charts/visiblePolling';

// --- 전송 계층 모킹(HeatmapPanel.test.tsx 선례) --------------------------

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

const storeMock: { current: UseStoreChartDataResult } = { current: idleResult() };
vi.mock('../charts/useStoreChartData', () => ({
  useStoreChartData: () => storeMock.current,
}));
vi.mock('../charts/useTsdbChartData', () => ({
  useTsdbChartData: () => storeMock.current,
}));

// i18n 은 키를 그대로 돌려준다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { storeSeriesId } from '../charts/chartChannelTypes';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasPanel from './CanvasPanel';
import CanvasElementsEditor from './CanvasElementsEditor';
import {
  CanvasEditSelectionContext,
  CanvasLiveSeriesContext,
  useCanvasEditSelectionState,
  useCanvasLiveSeriesState,
} from './canvasEditContext';
import type { FrameScheduler } from './CanvasSurface';

// --- ResizeObserver 오버라이드 ------------------------------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;

/**
 * 지금 통보할 바깥 상자. 대부분의 시험은 기본값을 쓰고, 리사이즈를 다루는 시험만 이 값을
 * 갈거나 아래 `panelObservers` 로 새 크기를 직접 통보한다.
 */
let panelOuter = { width: 200, height: 160 };
/** 살아 있는 관찰자들 — 리사이즈를 시험이 직접 일으킬 통로다. */
let panelObservers: TriggeringResizeObserver[] = [];

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
    panelObservers.push(this);
  }
  observe() {
    // 200x160 은 기본 캔버스(500x400)와 **같은 5:4** 이고 기본 격자 칸(25 단위)에도 이미
    // 맞아 있다: 200×25/500 = 160×25/400 = 10px 이라 두 축 모두 나머지가 0 이다. 표면은
    // 축척 하나로 그리는 영역을 그 칸의 정수배로 줄이므로(0.10.0 · `stageLattice`), 비율이
    // 어긋나거나 칸에 맞지 않는 크기를 쓰면 이 파일의 좌표 기대값이 "줄인 만큼" 을 함께
    // 지고 가게 된다 — 그 산술은 `canvasGeometry.test.ts` 와 `CanvasSurface.test.tsx` 가
    // 따로 시험한다. 두 축의 축척은 0.4 로 같고, 그래서 화면 px 는 두 축 모두 2.5 단위다.
    this.cb([{ contentRect: { ...panelOuter } }]);
  }
  unobserve() {}
  disconnect() {}
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
/** 도형 채움 직전에 대입된 색만 모은다(`fillStyle` → `fill` 쌍). */
function shapeFills(): unknown[] {
  const out: unknown[] = [];
  ctxStub.calls.forEach((c, i) => {
    if (c[0] === 'fillStyle' && ctxStub.calls[i + 1]?.[0] === 'fill') out.push(c[1]);
  });
  return out;
}
/** 그려진 문구 목록. */
function drawnTexts(): string[] {
  return ctxStub.calls.filter((c) => c[0] === 'fillText').map((c) => c[1] as string);
}

// --- 가짜 프레임 예약기 --------------------------------------------------

function makeScheduler() {
  let nextHandle = 1;
  // 프레임 **요청 횟수**. 001 의 유휴 정지가 편집기 때문에 깨지지 않았는지 재는 눈이다
  // (SPEC-CANVAS-002 AC-E4 — 선택·호버는 프레임을 0 건 요청해야 한다).
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

/** 항상 보이는 가시성 원천(문서 상태와 무관하게 결정적). */
const ALWAYS_VISIBLE: VisibilitySource = {
  isVisible: () => true,
  subscribe: () => () => {},
};

// --- 고정 입력 -----------------------------------------------------------

/** 시리즈 동일성 키(metric/tags 없는 시리즈). 바인딩이 참조하는 공간이다. */
function sid(key: string): string {
  return storeSeriesId(key, '', {});
}

/** 값 하나짜리 타임라인. */
function reading(...values: Array<number | null>): ChartEntry[] {
  return values.map((value, i) => ({ timestamp: i + 1, value }));
}

/** store 소스가 붙은 캔버스 config. */
function makeConfig(
  elements: unknown[],
  seriesKeys: string[] = ['tank.level'],
  extras: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    data_source: 'store',
    store_source: {
      agent_name: 'a',
      namespace: 'default',
      selection_mode: 'keys',
      series: seriesKeys.map((k) => ({ key: k })),
      time_window_ms: 1000,
      interval_ms: 1000,
      aggregation: 'last',
    },
    elements,
    ...extras,
  };
}

/** 조회 결과를 컬럼 이름 → 타임라인으로 세운다. */
function setSeries(
  columns: Record<string, ChartEntry[]>,
  status: UseStoreChartDataResult['status'] = 'connected',
) {
  storeMock.current = {
    ...idleResult(),
    seriesNames: Object.keys(columns),
    seriesEntries: new Map(Object.entries(columns)),
    status,
  };
}

/** 패널을 렌더하고 첫 프레임을 그린다. */
function renderPanel(config: Record<string, unknown>, title?: string) {
  const clock = makeScheduler();
  const view = render(
    <CanvasPanel
      panelId="p1"
      title={title}
      config={config}
      scheduler={clock.scheduler}
      visibilitySource={ALWAYS_VISIBLE}
    />,
  );
  clock.flush(0);
  return { ...view, clock, config };
}

beforeEach(() => {
  storeMock.current = idleResult();
  panelOuter = { width: 200, height: 160 };
  panelObservers = [];
  ctxStub = makeCtxStub();
  vi.stubGlobal('ResizeObserver', TriggeringResizeObserver as unknown as typeof ResizeObserver);
  vi.stubGlobal('devicePixelRatio', 1);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    ctxStub as unknown as CanvasRenderingContext2D,
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// --- AC-01 ---------------------------------------------------------------

describe('CanvasPanel — 도형 구성 렌더 + 규칙 적용 (AC-01)', () => {
  const elements = [
    {
      id: 'tank',
      kind: 'rect',
      geometry: { x: 0, y: 0, w: 250, h: 200 },
      style: { fill: '#888888', stroke: '#000000', strokeWidth: 2 },
      binding: { series: sid('tank.level'), agg: 'last' },
      rules: [
        { op: 'gt', value: 80, patch: { fill: '#ff0000' } },
        { op: 'gt', value: 50, patch: { fill: '#ffff00', strokeWidth: 4 } },
      ],
    },
    { id: 'e1', kind: 'ellipse', geometry: { x: 250, y: 0, w: 150, h: 120 }, style: { fill: '#00ff00' } },
    { id: 'l1', kind: 'line', geometry: { x1: 0, y1: 400, x2: 500, y2: 400 }, style: { stroke: '#0000ff', strokeWidth: 1 } },
    { id: 't1', kind: 'text', geometry: { x: 250, y: 200 }, style: { textColor: '#111111' }, text: 'label' },
  ];

  it('4종 요소를 배열 순서대로 그리고, 바인딩 최신값 90 에 첫 일치 행의 채움색을 쓴다', () => {
    setSeries({ 'tank.level': reading(10, 90) });
    renderPanel(makeConfig(elements));

    // 도형 원시형 4종이 모두 캔버스에 나갔다.
    expect(ops()).toContain('rect');
    expect(ops()).toContain('ellipse');
    expect(ops()).toContain('moveTo');
    expect(ops()).toContain('lineTo');
    expect(drawnTexts()).toContain('label');

    // 배열 순서(뒤가 위) — rect 가 ellipse 보다 먼저 나간다.
    expect(ops().indexOf('rect')).toBeLessThan(ops().indexOf('ellipse'));

    // 첫 일치 행([gt 80])의 빨강. 두 번째 행의 노랑은 평가되지 않는다.
    expect(shapeFills()).toContain('#ff0000');
    expect(shapeFills()).not.toContain('#ffff00');
  });

  it('첫 일치 행이 이겨 두 번째 행의 선 두께 패치가 섞이지 않는다 (first-match-wins)', () => {
    setSeries({ 'tank.level': reading(90) });
    renderPanel(makeConfig(elements));

    // [gt 50] 행의 strokeWidth 4 가 아니라 기본 스타일의 2 가 마지막으로 설정된 선 두께다.
    const widths = ctxStub.calls.filter((c) => c[0] === 'stroke');
    expect(widths.length).toBeGreaterThan(0);
    expect(ctxStub.lineWidth).not.toBe(4);
  });

  it('제목이 있으면 타이틀 바를 그린다', () => {
    setSeries({ 'tank.level': reading(90) });
    const { getByTestId } = renderPanel(makeConfig(elements), '탱크 계통도');
    expect(getByTestId('canvas-title').textContent).toContain('탱크 계통도');
  });

  it('제목이 없으면 타이틀 바를 그리지 않는다', () => {
    setSeries({ 'tank.level': reading(90) });
    const { queryByTestId } = renderPanel(makeConfig(elements));
    expect(queryByTestId('canvas-title')).toBeNull();
  });
});

// --- AC-04 ---------------------------------------------------------------

describe('CanvasPanel — 문구 템플릿 토큰 치환 (AC-04)', () => {
  it('{name} · {value} · {unit} 을 단순 치환하고 미지 토큰은 원문으로 둔다', () => {
    // 표시명은 컬럼 이름(alias 미지정이면 훅이 정한 이름)이며 로케일에 의존하지 않는다.
    setSeries({ 실외기: reading(23.456) });
    renderPanel(
      makeConfig(
        [
          {
            id: 'label',
            kind: 'text',
            geometry: { x: 250, y: 200 },
            style: { textColor: '#000000' },
            text: '{name}: {value}{unit} {foo}',
            decimals: 1,
            unit: '℃',
            binding: { series: sid('outdoor'), agg: 'last' },
          },
        ],
        ['outdoor'],
      ),
    );

    expect(drawnTexts()).toContain('실외기: 23.5℃ {foo}');
  });

  it('일치한 규칙 행의 문구 패치가 기본 문구 템플릿을 대신한다', () => {
    setSeries({ pump: reading(1) });
    renderPanel(
      makeConfig(
        [
          {
            id: 'p',
            kind: 'text',
            geometry: { x: 250, y: 200 },
            style: { textColor: '#000000' },
            text: '기본 {value}',
            decimals: 0,
            binding: { series: sid('pump.run'), agg: 'last' },
            rules: [{ op: 'eq', value: 1, patch: { text: '가동 {value}' } }],
          },
        ],
        ['pump.run'],
      ),
    );

    expect(drawnTexts()).toContain('가동 1');
    expect(drawnTexts()).not.toContain('기본 1');
  });
});

// --- AC-E1 ---------------------------------------------------------------

describe('CanvasPanel — 요소 0개 (AC-E1)', () => {
  // **이 시험의 주장이 002 사용 시험에서 뒤집혔다.** 001 이 쓴 원래 문장은 "빈 상태 안내를
  // 그리고 캔버스 표면을 만들지 않는다" 였고, 그때는 옳았다 — 패널이 렌더 전용(가정 A7)이라
  // 요소가 0개면 표면에 그릴 것도 누를 것도 없었기 때문이다. 002 가 그 표면 위에 도형
  // 팔레트를 얹으면서 전제가 깨졌다: 표면이 없으면 오버레이도 팔레트도 없어, **첫 도형을
  // 놓아야 할 바로 그 순간에 놓을 곳이 사라진다.** 그래서 "만들지 않는다" 를 "표면 위에
  // 겹친다" 로 **의도적으로** 바꾼다. AC-E1 이 실제로 요구하는 것(안내가 있고 렌더 예외가
  // 없다)은 그대로 성립하며, 오히려 AC-E9 가 그 위에 얹힌다.
  it('빈 상태 안내를 표면 위에 겹쳐 그린다 (표면을 대신하지 않는다 — AC-E9)', () => {
    const { getByTestId, container } = renderPanel(makeConfig([]));

    expect(getByTestId('canvas-empty').textContent).toContain('dashboard.canvas.emptyState');
    // 표면은 살아 있다 — 002 의 팔레트가 얹힐 자리다.
    expect(getByTestId('canvas-surface')).toBeTruthy();
    expect(container.querySelector('canvas')).toBeTruthy();
  });

  it('빈 안내는 포인터를 먹지 않는다 (팔레트·캔버스 누름을 가리는 유리판이 아니다)', () => {
    const { getByTestId } = renderPanel(makeConfig([]));
    expect(getByTestId('canvas-empty').className).toContain('pointer-events-none');
  });

  it('config 가 아예 손상되어도 예외 없이 빈 상태로 떨어진다', () => {
    const { getByTestId } = renderPanel({ elements: 'not-an-array' });
    expect(getByTestId('canvas-empty')).toBeTruthy();
  });

  it('요소가 생기면 안내가 사라진다', () => {
    const { queryByTestId } = renderPanel(
      makeConfig([{ id: 'a', kind: 'rect', geometry: { x: 0, y: 0, w: 500, h: 400 }, style: {} }]),
    );
    expect(queryByTestId('canvas-empty')).toBeNull();
  });
});

// --- AC-E2 ---------------------------------------------------------------

describe('CanvasPanel — 바인딩 시리즈 결측 (AC-E2)', () => {
  const missingElements = [
    {
      id: 'r',
      kind: 'rect',
      geometry: { x: 0, y: 0, w: 500, h: 400 },
      style: { fill: '#cccccc', textColor: '#000000' },
      text: '{value}',
      binding: { series: sid('sensor.a'), agg: 'last' },
    },
  ];

  it('시리즈가 아예 없으면 기본 스타일로 그려지고 {value} 는 결측 표기가 된다', () => {
    setSeries({});
    renderPanel(makeConfig(missingElements, ['sensor.a']));

    expect(shapeFills()).toContain('#cccccc');
    expect(drawnTexts()).toContain('-');
  });

  it('컬럼은 있으나 타임라인이 비면(값 없음) 역시 결측으로 다룬다', () => {
    // seriesNames 에는 있는데 seriesEntries 에 타임라인이 없는 상태.
    storeMock.current = {
      ...idleResult(),
      seriesNames: ['sensor.a'],
      status: 'connected',
    };
    renderPanel(makeConfig(missingElements, ['sensor.a']));

    expect(shapeFills()).toContain('#cccccc');
    expect(drawnTexts()).toContain('-');
  });

  it('nodata 행은 결측에서 정상적으로 일치한다', () => {
    setSeries({ 'sensor.a': reading(null) });
    renderPanel(
      makeConfig(
        [
          {
            ...missingElements[0],
            rules: [
              { op: 'nodata', patch: { fill: '#999999' } },
              { op: 'gt', value: 0, patch: { fill: '#ff0000' } },
            ],
          },
        ],
        ['sensor.a'],
      ),
    );

    expect(shapeFills()).toContain('#999999');
    expect(shapeFills()).not.toContain('#ff0000');
  });
});

// --- AC-E3 ---------------------------------------------------------------

describe('CanvasPanel — 일치하는 규칙 없음 (AC-E3)', () => {
  it('기본 스타일과 기본 문구를 그대로 쓴다(오류가 아니다)', () => {
    setSeries({ 'tank.level': reading(10) });
    renderPanel(
      makeConfig([
        {
          id: 'r',
          kind: 'rect',
          geometry: { x: 0, y: 0, w: 500, h: 400 },
          style: { fill: '#cccccc', textColor: '#000000' },
          text: '기본 {value}',
          decimals: 0,
          binding: { series: sid('tank.level'), agg: 'last' },
          rules: [
            { op: 'gt', value: 80, patch: { fill: '#ff0000' } },
            { op: 'gt', value: 50, patch: { fill: '#ffff00' } },
          ],
        },
      ]),
    );

    expect(shapeFills()).toEqual(['#cccccc']);
    expect(drawnTexts()).toContain('기본 10');
  });
});

// --- AC-E4 ---------------------------------------------------------------

describe('CanvasPanel — 폴링 실패 (AC-E4)', () => {
  const errElements = [
    {
      id: 'r',
      kind: 'rect',
      geometry: { x: 0, y: 0, w: 500, h: 400 },
      style: { fill: '#cccccc', textColor: '#000000' },
      text: '{value}',
      decimals: 0,
      binding: { series: sid('tank.level'), agg: 'last' },
      rules: [{ op: 'gt', value: 80, patch: { fill: '#ff0000' } }],
    },
  ];

  it('오류가 나도 마지막 성공 프레임을 다시 그리지 않고 배지만 덧붙인다', () => {
    setSeries({ 'tank.level': reading(90) });
    const config = makeConfig(errElements);
    const clock = makeScheduler();
    // 매번 **새 엘리먼트**를 만든다 — 같은 엘리먼트 참조를 다시 넘기면 React 가 재렌더를
    // 건너뛰어(엘리먼트 동일성 bailout) 훅 결과 변화가 반영되지 않는다.
    const panel = () => (
      <CanvasPanel
        panelId="p1"
        config={config}
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />
    );
    const { rerender, queryByTestId, getByTestId } = render(panel());
    clock.flush(0);

    expect(shapeFills()).toContain('#ff0000');
    expect(drawnTexts()).toContain('90');
    expect(queryByTestId('canvas-error')).toBeNull();

    // 다음 폴링이 실패해 조회 결과가 통째로 비워진 상태로 바뀐다.
    ctxStub.calls.length = 0;
    storeMock.current = { ...idleResult(), status: 'error', errorReason: 'network' };
    rerender(panel());
    clock.flush(16);

    // 표면은 살아 있고, 목표가 그대로라 프레임 자체가 예약되지 않는다 = 마지막 그림 유지.
    expect(getByTestId('canvas-surface')).toBeTruthy();
    expect(ops()).not.toContain('clearRect');
    expect(shapeFills()).toEqual([]);
    // 오류 배지만 덧붙는다.
    expect(getByTestId('canvas-error').textContent).toContain('dashboard.canvas.error');
  });

  it('다음 주기가 성공하면 새 값으로 회복하고 배지를 거둔다', () => {
    setSeries({ 'tank.level': reading(90) });
    const config = makeConfig(errElements);
    const clock = makeScheduler();
    const panel = () => (
      <CanvasPanel
        panelId="p1"
        config={config}
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />
    );
    const { rerender, queryByTestId } = render(panel());
    clock.flush(0);

    storeMock.current = { ...idleResult(), status: 'error' };
    rerender(panel());
    clock.flush(16);

    ctxStub.calls.length = 0;
    setSeries({ 'tank.level': reading(10) });
    rerender(panel());
    clock.flush(32);

    expect(shapeFills()).toContain('#cccccc');
    expect(drawnTexts()).toContain('10');
    expect(queryByTestId('canvas-error')).toBeNull();
  });
});

// --- 정적 요소 / 값 방어 / 키 공간 ---------------------------------------

describe('CanvasPanel — 정적 요소와 값 방어', () => {
  it('바인딩이 없는 요소는 규칙을 평가하지 않고 기본 스타일로 그린다', () => {
    setSeries({ 'tank.level': reading(90) });
    renderPanel(
      makeConfig([
        {
          id: 'static',
          kind: 'rect',
          geometry: { x: 0, y: 0, w: 500, h: 400 },
          style: { fill: '#cccccc', textColor: '#000000' },
          text: '정적 {value}',
          // 바인딩이 없으므로 이 행은 절대 평가되지 않는다.
          rules: [{ op: 'gt', value: 0, patch: { fill: '#ff0000' } }],
        },
      ]),
    );

    expect(shapeFills()).toEqual(['#cccccc']);
    // 값이 없으므로 {value} 는 결측 표기다.
    expect(drawnTexts()).toContain('정적 -');
  });

  it('문구가 없는 요소는 아무 글자도 그리지 않는다', () => {
    setSeries({ 'tank.level': reading(90) });
    renderPanel(
      makeConfig([
        { id: 'plain', kind: 'rect', geometry: { x: 0, y: 0, w: 500, h: 400 }, style: { fill: '#cccccc' } },
      ]),
    );

    expect(drawnTexts()).toEqual([]);
  });

  it('NaN / Infinity 값은 결측으로 다뤄 규칙과 문구에 흘려보내지 않는다', () => {
    setSeries({ 'tank.level': reading(Number.NaN, Number.POSITIVE_INFINITY) });
    renderPanel(
      makeConfig([
        {
          id: 'r',
          kind: 'rect',
          geometry: { x: 0, y: 0, w: 500, h: 400 },
          style: { fill: '#cccccc', textColor: '#000000' },
          text: '{value}',
          binding: { series: sid('tank.level'), agg: 'last' },
          rules: [
            { op: 'gt', value: 0, patch: { fill: '#ff0000' } },
            { op: 'nodata', patch: { fill: '#999999' } },
          ],
        },
      ]),
    );

    // gt 는 일치하지 않고 nodata 가 이긴다. Infinity 가 화면에 찍히지 않는다.
    expect(shapeFills()).toContain('#999999');
    expect(drawnTexts()).toContain('-');
    expect(drawnTexts().join('')).not.toContain('Infinity');
  });

  it('맨 끝 빈 버킷을 건너뛰고 마지막 유한값을 읽는다', () => {
    setSeries({ 'tank.level': reading(90, null, null) });
    renderPanel(
      makeConfig([
        {
          id: 'r',
          kind: 'rect',
          geometry: { x: 0, y: 0, w: 500, h: 400 },
          style: { fill: '#cccccc', textColor: '#000000' },
          text: '{value}',
          decimals: 0,
          binding: { series: sid('tank.level'), agg: 'last' },
        },
      ]),
    );

    expect(drawnTexts()).toContain('90');
  });

  it('컬럼 수와 config 시리즈 수가 어긋나면 조회 이름이 곧 동일성 키다', () => {
    // 한 key 가 두 컬럼으로 펼쳐진 경우(aligned=false) — 히트맵과 같은 graceful degrade.
    setSeries({ 'tank.level a': reading(90), 'tank.level b': reading(10) });
    renderPanel(
      makeConfig(
        [
          {
            id: 'r',
            kind: 'rect',
            geometry: { x: 0, y: 0, w: 500, h: 400 },
            style: { fill: '#cccccc', textColor: '#000000' },
            text: '{name}={value}',
            decimals: 0,
            binding: { series: 'tank.level b', agg: 'last' },
          },
        ],
        ['tank.level'],
      ),
    );

    expect(drawnTexts()).toContain('tank.level b=10');
  });

  it('같은 동일성 키가 두 컬럼으로 오면 첫 컬럼이 이긴다', () => {
    setSeries({ first: reading(90), second: reading(10) });
    renderPanel(
      makeConfig(
        [
          {
            id: 'r',
            kind: 'rect',
            geometry: { x: 0, y: 0, w: 500, h: 400 },
            style: { fill: '#cccccc', textColor: '#000000' },
            text: '{value}',
            decimals: 0,
            binding: { series: sid('dup'), agg: 'last' },
          },
        ],
        // 같은 (key, field, tags) 두 건 → 동일성 키가 겹친다.
        ['dup', 'dup'],
      ),
    );

    expect(drawnTexts()).toContain('90');
  });
});

// --- SPEC-CANVAS-002 T6: 캔버스 내 시각 편집 배선 -------------------------
//
// 001 은 `onConfigChange` 를 **받아만 두었다**. 여기서 그 자리가 살아난다.
// 덮는 인수 기준: AC-03(드래그가 config 로 흘러간다), AC-07(세 겹 게이팅),
// AC-E4(선택·호버는 프레임을 0 건 요청한다 — 001 의 유휴 정지 보존).

/**
 * **고정점** 고정 입력 — `canvas === outer`(acceptance.md §시험 규율 D7).
 *
 * 편집을 켠 시험은 이 짝을 써야 한다. M8 이 편집 중 캔버스 크기를 잰 상자 **그 자체**로
 * 유도하므로(항등이다), 고정점이 아닌 짝으로 편집을 켜면 시험이 적어 둔 캔버스가
 * **첫 측정에서 덮이고** 그 뒤의 좌표 기대값이 전부 다른 캔버스 위에서 계산된다 —
 * 초록인데 제 이름과 다른 것을 재는 시험이 된다.
 *
 * 그리고 이 함정은 피할 수 없다: `usePanelEditMode` 의 `canEdit` 가 곧 `onConfigChange`
 * 존재이므로 **"편집은 켜되 쓰지는 않는" 형상이 존재하지 않는다.**
 */
function fixedPointCanvas(): { width: number; height: number } {
  return { ...panelOuter };
}

/** 편집 배선을 켠 채 패널을 렌더한다. 프레임은 더 예약할 것이 없을 때까지 밀어 둔다. */
function renderEditablePanel(
  elements: unknown[],
  opts: {
    onConfigChange?: (patch: Record<string, unknown>) => void;
    forceEdit?: boolean;
    /** 저술된 캔버스 크기. 편집을 켜는 시험은 **고정점**을 넘긴다(D7). */
    canvas?: { width: number; height: number };
  } = {},
) {
  const clock = makeScheduler();
  const view = render(
    // 편집 도구(팔레트·격자·정렬·순서)는 패널 **밖**의 도크 자리에만 그려진다. 설정
    // 미리보기가 그 자리를 내므로, 도구를 보는 시험은 같은 형상을 흉내 낸다.
    <CanvasEditDockRegion enabled>
      <CanvasPanel
        panelId="p1"
        config={makeConfig(
          elements,
          undefined,
          opts.canvas === undefined ? {} : { canvas: opts.canvas },
        )}
        onConfigChange={opts.onConfigChange}
        forceEdit={opts.forceEdit}
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />
    </CanvasEditDockRegion>,
  );
  // 표면이 유휴에 들 때까지 민다 — 유휴가 아닌 상태에서 프레임을 세면 무엇을 세는지 흐려진다.
  for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
  return { ...view, clock };
}

/** 좌표를 실제로 실어 나르는 포인터 이벤트(jsdom 에는 PointerEvent 가 없다). */
function panelPointer(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, {
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
    ...init,
  });
}

/** 오버레이 루트에 포인터 이벤트를 보낸다. */
function sendToOverlay(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  const evt = panelPointer(type, x, y, init);
  fireEvent(screen.getByTestId('canvas-edit-overlay'), evt);
  return evt;
}

/** 오버레이의 합류 프레임을 기다린다(표면의 가짜 예약기와는 다른 축이다). */
async function nextBrowserFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

/**
 * 사각형 하나 — 기본 캔버스(500x400) 좌표로 (50, 40, 100, 80).
 *
 * config 에 `canvas` 키가 없으므로 파서가 기본 크기를 채운다(001·002 가 쓴 config 와
 * 같은 형상이다). 스테이지 200×100 에 투영하면 px 상자는 (20, 10, 40, 20) 이다.
 */
const EDIT_RECT = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 50, y: 40, w: 100, h: 80 },
  style: { fill: '#888888' },
};

/** config 를 실제로 갱신하는 숙주 — 드래그의 깨우기 경로를 끝까지 잇는다. */
function StatefulHost({ clock }: { clock: ReturnType<typeof makeScheduler> }) {
  // 편집을 켜는 숙주이므로 **고정점**을 쓴다(D7) — 그러지 않으면 첫 측정이 캔버스를
  // 덮어 아래 시험들의 화면 px 가 다른 캔버스 위에서 계산된다.
  const [cfg, setCfg] = useState<Record<string, unknown>>(() =>
    makeConfig([EDIT_RECT], undefined, { canvas: fixedPointCanvas() }),
  );
  return (
    <CanvasEditDockRegion enabled>
      <CanvasPanel
        panelId="p1"
        config={cfg}
        onConfigChange={(patch) => setCfg((prev) => ({ ...prev, ...patch }))}
        forceEdit
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />
    </CanvasEditDockRegion>
  );
}

describe('CanvasPanel — 편집 게이팅 세 겹 (AC-07)', () => {
  afterEach(() => {
    // 이 훅은 파일 최상단의 `cleanup()` **보다 먼저** 돈다(등록 역순). 아직 마운트된
    // 패널이 스토어를 구독하고 있으므로 act 로 감싸지 않으면 React 가 경고한다.
    act(() => {
      useUIStore.setState({ dashboardEditMode: false });
    });
  });

  it('config 를 쓸 콜백이 없으면 편집도 토글도 없다 (끌어도 저장할 곳이 없다)', () => {
    useUIStore.setState({ dashboardEditMode: true });
    renderEditablePanel([EDIT_RECT]);

    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    expect(screen.queryByTestId('canvas-edit-toggle')).toBeNull();
  });

  it('대시보드 편집모드가 꺼져 있으면 표시 전용이다 (읽기 전용 뷰)', () => {
    renderEditablePanel([EDIT_RECT], { onConfigChange: vi.fn() });

    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    expect(screen.queryByTestId('canvas-edit-toggle')).toBeNull();
  });

  it('편집모드에서는 토글이 나오고, 켜야 오버레이가 생긴다', () => {
    useUIStore.setState({ dashboardEditMode: true });
    renderEditablePanel([EDIT_RECT], { onConfigChange: vi.fn() });

    const toggle = screen.getByTestId('canvas-edit-toggle');
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();

    fireEvent.click(toggle);
    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
  });

  it('forceEdit 자리(설정 미리보기)는 항상 편집이며 토글을 감춘다', () => {
    renderEditablePanel([EDIT_RECT], {
      onConfigChange: vi.fn(),
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
    expect(screen.queryByTestId('canvas-edit-toggle')).toBeNull();
  });
});

describe('CanvasPanel — 드래그가 onConfigChange 로 흘러간다 (AC-03)', () => {
  it('요소를 끌면 elements 패치가 나간다 (001 이 받아만 두었던 자리다)', async () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([EDIT_RECT], {
      onConfigChange,
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    sendToOverlay('pointerdown', 40, 40);
    sendToOverlay('pointermove', 76, 76);
    await nextBrowserFrame();

    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as { elements: Array<{ id: string; geometry: unknown }> };
    expect(Object.keys(patch)).toEqual(['elements']);
    // 화면에서 (36,36)px 끌었다 — **편집 중** 스테이지 144×115.2 위의 그 길이는 캔버스
    // (200×160) 단위로 (50, 50) 이다(두 축이 같은 축척 0.72 다). 자리는 정수로 저장된다.
    //
    // 006 M8 이 편집 중 캔버스 크기를 잰 상자 그 자체로 유도하므로, 이 시험은 **고정점**
    // 고정 입력(`canvas === outer === 200×160`)을 쓴다(D7). 그러지 않으면 첫 측정이
    // 캔버스를 덮어 아래 기대값이 시험이 적어 둔 것과 **다른 캔버스** 위에서 계산된다.
    // 화면 px 는 그 축척(0.72)을 따라 옮겨 갔을 뿐이며(21→40 · 35→76), 이 시험이 지키는
    // 단언 — 끈 결과가 `elements` 패치 하나로 나가고, 그 값이 정수이며, 두 축이 **같은
    // 양**만큼 움직인다 — 은 한 글자도 달라지지 않았다. 축척이 바뀌는 것 자체는 REQ-03 이
    // 명시한 성질이다("바뀌는 것은 투영 축척뿐이고 좌표는 그대로다").
    expect(patch.elements.find((el) => el.id === 'a')!.geometry).toEqual({
      x: 100,
      y: 90,
      w: 100,
      h: 80,
    });
  });

  it('편집이 꺼져 있으면 같은 누름이 아무것도 바꾸지 않는다', async () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([EDIT_RECT], { onConfigChange });

    // 오버레이가 아예 없으므로 누름은 캔버스로 가고 아무 일도 일어나지 않는다.
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    fireEvent(screen.getByTestId('canvas-surface'), panelPointer('pointerdown', 21, 21));
    fireEvent(screen.getByTestId('canvas-surface'), panelPointer('pointermove', 35, 35));
    await nextBrowserFrame();

    expect(onConfigChange).not.toHaveBeenCalled();
  });
});

describe('CanvasPanel — 편집기가 유휴 정지를 깨지 않는다 (AC-E4)', () => {
  it('선택과 호버는 프레임을 단 한 건도 요청하지 않는다', () => {
    const { clock } = renderEditablePanel(
      [EDIT_RECT, { ...EDIT_RECT, id: 'b', geometry: { x: 250, y: 200, w: 100, h: 80 } }],
      { onConfigChange: vi.fn(), forceEdit: true, canvas: fixedPointCanvas() },
    );
    // 유휴에 들었다 — 여기서부터의 요청은 전부 편집기 탓이다.
    expect(clock.pending).toBe(0);
    const before = clock.requested;

    // 고르고, 다른 요소로 옮기고, 핸들 자리를 지나간다(호버).
    //
    // 고정점 고정 입력(`canvas === outer === 200×160`, 축척 0.72)에서 겨냥하는 대상은
    // 그대로다: (40,40) 은 요소 a 의 몸통(px 36..108 × 28.8..86.4), (200,160) 은 요소 b 의
    // 몸통(px 180..252 × 144..201.6 — 출력 영역 **밖**의 저술 여백이다), (150,120) 은
    // 아무것도 없는 자리, (50,50) 은 다시 a 의 몸통이다. 이 시험이 지키는 단언(선택도
    // 호버도 프레임을 **0 건** 요청한다)은 화면 자리와 무관하다.
    sendToOverlay('pointerdown', 40, 40);
    sendToOverlay('pointerup', 40, 40);
    sendToOverlay('pointerdown', 200, 160);
    sendToOverlay('pointerup', 200, 160);
    sendToOverlay('pointermove', 150, 120);
    sendToOverlay('pointermove', 50, 50);

    expect(screen.getByTestId('canvas-selection-b')).toBeTruthy();
    expect(clock.requested).toBe(before);
    expect(clock.pending).toBe(0);
  });

  it('끌어서 기하가 바뀌면 종전의 props 변경 경로로 프레임이 예약된다', async () => {
    const clock = makeScheduler();
    render(<StatefulHost clock={clock} />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    const before = clock.requested;

    sendToOverlay('pointerdown', 40, 40);
    sendToOverlay('pointermove', 76, 76);
    // 쓰기가 config 를 갈면 `elements` 참조가 바뀐다 — 001 과 **같은** 깨우기 경로다.
    await nextBrowserFrame();

    expect(clock.requested).toBeGreaterThan(before);
  });
});

// --- SPEC-CANVAS-002 T16: 편집 표면 전량이 유휴 정지를 지킨다 --------------
//
// 위 블록이 "선택과 호버" 를 쟀다면 여기서는 **편집 표면의 나머지 조작 전부**를 같은 눈
// (주입된 `FrameScheduler`)으로 잰다 — 격자 토글 · 팔레트 호버·초점 · 핸들 초점. 셋 다
// DOM/CSS 뿐이라 캔버스 props 를 건드리지 않으므로 프레임이 0 건이어야 하고, 하나라도
// 새면 그것은 오버레이가 001 의 루프 안으로 들어왔다는 신호다(REQ-05 · 위험 R3).
//
// 방향키 이동(T15)은 반대로 **예약되어야** 한다. 다만 그 이유는 "편집기라서" 가 아니라
// **`elements` 가 실제로 바뀌었기 때문**이며, 그것은 설정 다이얼로그에서 수치를 고칠 때
// 오늘도 일어나는 바로 그 경로다.

describe('CanvasPanel — 편집 표면 조작 전량이 유휴 정지를 지킨다 (AC-E4 · T16)', () => {
  it('격자 토글 · 팔레트 호버·초점 · 핸들 초점은 프레임 요청이 0 건이다', () => {
    const { clock } = renderEditablePanel([EDIT_RECT], {
      onConfigChange: vi.fn(),
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });
    expect(clock.pending).toBe(0);

    // 고르기까지가 전제다(핸들은 하나만 골랐을 때 뜬다).
    sendToOverlay('pointerdown', 40, 40);
    sendToOverlay('pointerup', 40, 40);
    expect(screen.getByTestId('canvas-selection-a')).toBeTruthy();
    const before = clock.requested;

    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
    fireEvent.pointerOver(screen.getByTestId('canvas-palette-add-rect'));
    fireEvent.mouseOver(screen.getByTestId('canvas-palette-add-rect'));
    screen.getByTestId('canvas-palette-add-rect').focus();
    screen.getByTestId('canvas-handle-se').focus();
    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
    // 소비하지 않는 키(보조키가 붙은 방향키)도 마찬가지다.
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), {
      key: 'ArrowRight',
      ctrlKey: true,
    });

    expect(clock.requested).toBe(before);
    expect(clock.pending).toBe(0);
  });

  it('방향키로 옮기면 종전의 props 변경 경로로 프레임이 예약된다 (새 깨우기 경로가 아니다)', () => {
    const clock = makeScheduler();
    render(<StatefulHost clock={clock} />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);

    // 고르고 손을 뗀다 — 놓기 자체가 제자리 확정 쓰기를 한 번 내므로 그 프레임까지 민다.
    sendToOverlay('pointerdown', 40, 40);
    sendToOverlay('pointerup', 40, 40);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    const before = clock.requested;

    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), {
      key: 'ArrowRight',
      shiftKey: true,
    });

    // 한 번 눌러 한 번 예약이다 — 합류 프레임을 따로 잡지 않는다.
    expect(clock.requested).toBe(before + 1);
    // 그 프레임을 그리고 나면 진행 중 트윈이 없으므로 루프는 다시 유휴로 돌아간다.
    clock.flush(100);
    expect(clock.pending).toBe(0);
  });
});

// --- 결함 A(사용 시험): 요소 0개일 때 팔레트가 사라지지 않는다 (AC-E9) ----
//
// 001 의 빈 상태 분기는 표면을 **대신했고**, 002 는 팔레트를 그 표면의 오버레이 슬롯에
// 얹었다. 두 결정이 만나 "첫 도형을 놓을 수 없다" 가 되었다 — 어느 쪽 시험도 혼자서는
// 그것을 볼 수 없었다. 그래서 둘이 만나는 지점을 여기서 못박는다.

describe('CanvasPanel — 요소가 0개여도 팔레트로 첫 도형을 놓을 수 있다 (AC-E9)', () => {
  it('요소 0개 + 편집에서 팔레트 버튼 4개가 모두 떠 있고 안내도 함께 보인다', () => {
    renderEditablePanel([], {
      onConfigChange: vi.fn(),
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    expect(screen.getByTestId('canvas-empty')).toBeTruthy();
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.getByTestId(`canvas-palette-add-${kind}`)).toBeTruthy();
    }
  });

  it('요소 0개에서 팔레트를 누르면 첫 요소가 config 로 흘러간다', () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([], {
      onConfigChange,
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));

    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as { elements: Array<{ kind: string }> };
    expect(patch.elements).toHaveLength(1);
    expect(patch.elements[0]!.kind).toBe('rect');
  });
});

// --- 결함 D(사용 시험): 고른 시리즈가 실제로 값을 낸다 --------------------
//
// 목록 편집기가 내던 바인딩 값과 패널이 판독값을 찾던 키가 **서로 다른 공간**이었다.
// 편집기는 config 만 보고 언제나 `storeSeriesId(...)` 를 냈고, 패널은 조회 컬럼 수가
// config 의 참조 수와 같을 때만 그 동일성 키를 쓰고 **아닐 때는 조회 이름**을 썼다.
// 참조 하나가 태그로 여러 컬럼으로 펼쳐지는 흔한 경우가 정확히 그 "아닐 때" 이므로,
// 사용자가 고른 키는 어느 판독값과도 만나지 못하고 `{value}` 가 결측 표기로 남았다.
//
// **이 결함이 기존 시험 전부를 빠져나간 이유**: 바인딩을 다룬 모든 고정 입력이
// 컬럼 수 == 참조 수(1:1)였다. 그 형상에서는 두 공간이 우연히 같으므로 결함이 없다.
// 그래서 아래 시험의 고정 입력은 **참조 1개 → 컬럼 3개(비정렬)** 다.

/** 참조 하나가 컬럼 셋으로 펼쳐지는 조회 결과(태그 팬아웃). 이름은 훅이 정한 표기다. */
const FANOUT_COLUMNS = ['temp · value{room=A}', 'temp · value{room=B}', 'temp · value{room=C}'];

/**
 * 그 팬아웃을 낳는 config — store 참조는 **하나**뿐이다(태그 필터 한 줄).
 *
 * 이 숙주들은 전부 편집을 켠 채(`forceEdit`) 패치를 되먹이므로 **고정점**을 쓴다(D7) —
 * 그러지 않으면 첫 측정이 캔버스를 잰 상자로 덮어, 시험이 적어 둔 500×400 위에서
 * 계산한 척하는 초록이 된다.
 */
function fanoutConfig(elements: unknown[]): Record<string, unknown> {
  return {
    canvas: { ...panelOuter },
    data_source: 'store',
    store_source: {
      agent_name: 'a',
      namespace: 'default',
      selection_mode: 'keys',
      series: [{ key: 'temp', field: 'value', tags: { room: '*' } }],
      time_window_ms: 1000,
      interval_ms: 1000,
      aggregation: 'last',
    },
    elements,
  };
}

/** 편집기가 config 만 보고 냈던(=결함이 있던) 키. 어느 판독값과도 만나지 못한다. */
const CONFIG_DERIVED_KEY = storeSeriesId('temp', 'value', { room: '*' });

/** `{value}` 만 찍는 텍스트 요소 하나. 바인딩은 아직 없다 — 사용자가 곧 고른다. */
const VALUE_LABEL = {
  id: 'v',
  kind: 'text',
  geometry: { x: 250, y: 200 },
  style: { textColor: '#000000' },
  text: '{value}',
  decimals: 0,
};

/**
 * 설정 다이얼로그와 **같은 형상**: 미리보기 패널과 목록 편집기가 한 config 와
 * 두 컨텍스트를 나눠 쓴다. 결함이 두 컴포넌트 **사이**에 있었으므로 어느 한쪽만
 * 렌더하는 시험으로는 볼 수 없다.
 */
function BindingHost({ clock }: { clock: ReturnType<typeof makeScheduler> }) {
  const [cfg, setCfg] = useState<Record<string, unknown>>(() => fanoutConfig([VALUE_LABEL]));
  const patch = (p: Record<string, unknown>) => setCfg((prev) => ({ ...prev, ...p }));
  const selection = useCanvasEditSelectionState();
  const liveSeries = useCanvasLiveSeriesState();
  return (
    <CanvasLiveSeriesContext value={liveSeries}>
      <CanvasEditSelectionContext value={selection}>
        <CanvasPanel
          panelId="p1"
          config={cfg}
          onConfigChange={patch}
          forceEdit
          scheduler={clock.scheduler}
          visibilitySource={ALWAYS_VISIBLE}
        />
        <CanvasElementsEditor config={cfg} onConfigChange={patch} />
      </CanvasEditSelectionContext>
    </CanvasLiveSeriesContext>
  );
}

/**
 * 숙주를 렌더하고 첫 행을 펼친 뒤 **데이터 탭을 연다**.
 *
 * 세부는 기본이 접힘이고, 펼친 카드는 탭 넷으로 갈려 열린 탭 하나만 그린다. 이 절이 재는
 * 것은 전부 바인딩 드롭다운이며 그것은 데이터 탭에 있다 — 열지 않으면 "안내가 뜨지
 * 않는다" 류의 부재 단언이 탭이 닫혀 있어서 통과한다.
 */
function renderBindingHost() {
  const clock = makeScheduler();
  const view = render(<BindingHost clock={clock} />);
  for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
  fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
  fireEvent.click(screen.getByTestId('canvas-element-tab-data-0'));
  return { ...view, clock };
}

/** 바인딩 드롭다운의 선택지 값 목록. */
function bindingValues(): string[] {
  const select = screen.getByTestId('canvas-element-binding-0') as HTMLSelectElement;
  return Array.from(select.options).map((o) => o.value);
}

describe('CanvasPanel ↔ CanvasElementsEditor — 고른 시리즈가 값을 낸다 (결함 D)', () => {
  it('참조 1개가 컬럼 3개로 펼쳐지면 편집기는 **판독값 키 3개**를 낸다', () => {
    setSeries({
      [FANOUT_COLUMNS[0]!]: reading(11),
      [FANOUT_COLUMNS[1]!]: reading(22),
      [FANOUT_COLUMNS[2]!]: reading(33),
    });
    renderBindingHost();

    expect(bindingValues()).toEqual(['', ...FANOUT_COLUMNS]);
    // config 만 보고 낸 키는 이 목록에 없다 — 있으면 고른 순간 값을 잃는다.
    expect(bindingValues()).not.toContain(CONFIG_DERIVED_KEY);
  });

  it('그 선택지를 고르면 그 컬럼의 값이 실제로 `{value}` 로 그려진다', () => {
    setSeries({
      [FANOUT_COLUMNS[0]!]: reading(11),
      [FANOUT_COLUMNS[1]!]: reading(22),
      [FANOUT_COLUMNS[2]!]: reading(33),
    });
    const { clock } = renderBindingHost();

    // 아직 바인딩이 없으므로 결측 표기다.
    expect(drawnTexts()).toContain('-');

    // 가운데 컬럼을 고른다 — 첫 컬럼을 고르면 "정렬됐을 때와 우연히 같은" 자리라
    // 결함을 통과시킬 수 있다.
    fireEvent.change(screen.getByTestId('canvas-element-binding-0'), {
      target: { value: FANOUT_COLUMNS[1]! },
    });
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(100 + i * 16);

    expect(drawnTexts()).toContain('22');
  });

  it('컬럼 수와 참조 수가 1:1 이면 종전대로 동일성 키를 낸다 (정렬 경로 회귀)', () => {
    // 참조 1개 · 컬럼 1개. 이 형상에서 패널은 조회 이름이 아니라 동일성 키로 키잉한다.
    setSeries({ 'temp · value{room=A}': reading(42) });
    const clock = makeScheduler();
    function Aligned() {
      const [cfg, setCfg] = useState<Record<string, unknown>>(() =>
        fanoutConfig([{ ...VALUE_LABEL }]),
      );
      const patch = (p: Record<string, unknown>) => setCfg((prev) => ({ ...prev, ...p }));
      const liveSeries = useCanvasLiveSeriesState();
      return (
        <CanvasLiveSeriesContext value={liveSeries}>
          <CanvasPanel
            panelId="p1"
            config={cfg}
            onConfigChange={patch}
            forceEdit
            scheduler={clock.scheduler}
            visibilitySource={ALWAYS_VISIBLE}
          />
          <CanvasElementsEditor config={cfg} onConfigChange={patch} />
        </CanvasLiveSeriesContext>
      );
    }
    render(<Aligned />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    fireEvent.click(screen.getByTestId('canvas-element-tab-data-0'));

    expect(bindingValues()).toEqual(['', CONFIG_DERIVED_KEY]);

    fireEvent.change(screen.getByTestId('canvas-element-binding-0'), {
      target: { value: CONFIG_DERIVED_KEY },
    });
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(100 + i * 16);

    expect(drawnTexts()).toContain('42');
  });

  it('조회가 아직 비어 있으면 config 로 뽑은 목록으로 떨어진다 (첫 조회 전)', () => {
    // 미리보기가 아직 아무 컬럼도 받지 못한 순간. 목록이 통째로 비면 사용자는 고를
    // 것이 없다고 읽으므로, 최선 추정이라도 내놓는다.
    renderBindingHost();
    expect(bindingValues()).toEqual(['', CONFIG_DERIVED_KEY]);
  });

  it('시리즈가 살아 있으면 "고를 것이 없다" 안내는 뜨지 않는다', () => {
    setSeries({
      [FANOUT_COLUMNS[0]!]: reading(11),
      [FANOUT_COLUMNS[1]!]: reading(22),
      [FANOUT_COLUMNS[2]!]: reading(33),
    });
    renderBindingHost();

    expect(bindingValues()).toContain(FANOUT_COLUMNS[0]!);
    expect(screen.queryByTestId('canvas-element-binding-hint-0')).toBeNull();
  });

  it('소스도 조회도 비면 안내가 뜬다 (진짜로 바인딩할 것이 없다)', () => {
    const clock = makeScheduler();
    function Bare() {
      const liveSeries = useCanvasLiveSeriesState();
      const cfg = { canvas: { ...panelOuter }, data_source: 'store', elements: [VALUE_LABEL] };
      return (
        <CanvasLiveSeriesContext value={liveSeries}>
          <CanvasPanel
            panelId="p1"
            config={cfg}
            onConfigChange={() => {}}
            forceEdit
            scheduler={clock.scheduler}
            visibilitySource={ALWAYS_VISIBLE}
          />
          <CanvasElementsEditor config={cfg} onConfigChange={() => {}} />
        </CanvasLiveSeriesContext>
      );
    }
    render(<Bare />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    fireEvent.click(screen.getByTestId('canvas-element-tab-data-0'));

    expect(bindingValues()).toEqual(['']);
    expect(screen.getByTestId('canvas-element-binding-hint-0')).toBeTruthy();
  });

  it('발행은 값이 같으면 다시 일어나지 않는다 (폴링마다 렌더가 도는 고리를 막는다)', () => {
    setSeries({ [FANOUT_COLUMNS[0]!]: reading(11) });
    let editorRenders = 0;
    const clock = makeScheduler();

    function CountingEditor(props: {
      config: Record<string, unknown>;
      onConfigChange: (p: Record<string, unknown>) => void;
    }) {
      editorRenders += 1;
      return <CanvasElementsEditor {...props} />;
    }

    function Host() {
      const [cfg, setCfg] = useState<Record<string, unknown>>(() =>
        fanoutConfig([VALUE_LABEL]),
      );
      const patch = (p: Record<string, unknown>) => setCfg((prev) => ({ ...prev, ...p }));
      const liveSeries = useCanvasLiveSeriesState();
      return (
        <CanvasLiveSeriesContext value={liveSeries}>
          <CanvasPanel
            panelId="p1"
            config={cfg}
            onConfigChange={patch}
            forceEdit
            scheduler={clock.scheduler}
            visibilitySource={ALWAYS_VISIBLE}
          />
          <CountingEditor config={cfg} onConfigChange={patch} />
        </CanvasLiveSeriesContext>
      );
    }

    const { rerender } = render(<Host />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    const afterFirstPublish = editorRenders;

    // 같은 값을 다시 조회한 것과 같은 상황: 판독값 맵은 새 참조지만 값은 그대로다.
    setSeries({ [FANOUT_COLUMNS[0]!]: reading(11) });
    act(() => {
      rerender(<Host />);
    });

    // 값이 같으므로 발행이 상태를 갈지 않았고, 편집기는 부모 렌더 한 번만큼만 돌았다.
    expect(editorRenders).toBe(afterFirstPublish + 1);
  });
});

// --- 숫자 스위치 (SPEC-CANVAS-002 · AC-E17) -------------------------------
//
// 값이 화면까지 오는 **전체 길**을 건너는 시험이다. 단위 시험(`canvasText.test.ts`)은
// 치환기 하나만 보고, 파서 시험은 키 하나만 본다 — 그 사이에 `latestFiniteValue` 가
// 있고, 그 함수는 숫자가 아닌 판독값을 **버린다**. 그래서 "끄면 문자열이 나온다" 는
// 오직 이 자리에서만 거짓이 될 수 있었다.
//
// 고정 입력의 값이 `"ON"` 인 것이 이 시험의 전부다. 숫자 고정 입력(`23.456`)으로는
// 켠 상태와 끈 상태의 차이가 반올림 여부로만 나타나, 판독 단계에서 값이 사라지는
// 결함을 그대로 통과시킨다.

/** 원시 판독값 타임라인 — 숫자가 아닌 값도 그대로 담는다(ChartEntry.value 는 unknown 이다). */
function rawReading(...values: unknown[]): ChartEntry[] {
  return values.map((value, i) => ({ timestamp: i + 1, value }));
}

/** 텍스트 요소 하나. `numeric` 을 부재로 두면 종전 config 와 같은 형상이다. */
function labelElement(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'lbl',
    kind: 'text',
    geometry: { x: 250, y: 200 },
    style: { textColor: '#000000' },
    text: '{value}{unit}',
    decimals: 1,
    unit: '℃',
    binding: { series: sid('pump.state'), agg: 'last' },
    ...over,
  };
}

describe('CanvasPanel — 숫자를 끄면 받은 값이 글자 그대로 나온다', () => {
  it('숫자가 아닌 판독값 "ON" 이 {value} 까지 살아 온다', () => {
    setSeries({ pump: rawReading('ON') });
    renderPanel(makeConfig([labelElement({ numeric: false })], ['pump.state']));

    expect(drawnTexts()).toContain('ON');
  });

  it('그때 소수 자리도 단위도 걸리지 않는다', () => {
    setSeries({ pump: rawReading('23.456') });
    renderPanel(
      makeConfig([labelElement({ numeric: false, decimals: 1, unit: '℃' })], ['pump.state']),
    );

    // 문자열이므로 반올림되지 않고, 단위도 붙지 않는다.
    expect(drawnTexts()).toContain('23.456');
    expect(drawnTexts()).not.toContain('23.5℃');
  });

  it('같은 판독값이 숫자를 켠 채로는 결측 표기로 남는다 — 스위치가 그 차이의 전부다', () => {
    setSeries({ pump: rawReading('ON') });
    renderPanel(makeConfig([labelElement()], ['pump.state']));

    // 숫자 경로는 비숫자를 버린다(`latestFiniteValue`). 종전 동작이며 바뀌지 않았다.
    expect(drawnTexts()).toContain('-℃');
    expect(drawnTexts()).not.toContain('ON');
  });

  it('숫자를 끈 요소도 값이 없으면 결측 표기다', () => {
    setSeries({ pump: rawReading(null) });
    renderPanel(makeConfig([labelElement({ numeric: false })], ['pump.state']));

    expect(drawnTexts()).toContain('-');
  });

  it('한 캔버스 안에서 요소마다 다르게 읽는다 — 스위치는 요소 축이다', () => {
    setSeries({ pump: rawReading('ON') });
    renderPanel(
      makeConfig(
        [
          labelElement({ id: 'raw', numeric: false, text: 'A{value}' }),
          labelElement({ id: 'num', text: 'B{value}' }),
        ],
        ['pump.state'],
      ),
    );

    expect(drawnTexts()).toContain('AON');
    expect(drawnTexts()).toContain('B-');
  });
});

describe('CanvasPanel — numeric 부재는 숫자다 (하위 호환 가드)', () => {
  // 이 파일의 나머지 시험 전부가 이 필드 없이 쓰였고 전량 green 이라는 사실이 이미
  // 하위 호환의 몸통이다. 아래는 그 성질을 **한 자리에서 못박아** 두는 자물쇠다 —
  // 판정을 참 판정(`!!el.numeric`)으로 잘못 적으면 부재가 거짓이 되어 기존 대시보드가
  // 통째로 문자열 표기로 뒤집히는데, 그 회귀는 다른 어떤 시험도 이름으로 부르지 않는다.
  it('키가 없는 요소는 종전대로 숫자로 서식된다(반올림 + 단위)', () => {
    setSeries({ pump: rawReading(23.456) });
    const { config } = renderPanel(makeConfig([labelElement()], ['pump.state']));

    expect(drawnTexts()).toContain('23.5℃');
    // 고정 입력에 그 키가 실제로 없다 — "부재" 를 재고 있음을 스스로 확인한다.
    expect((config.elements as Record<string, unknown>[])[0]).not.toHaveProperty('numeric');
  });

  it('numeric:true 를 명시한 결과와 한 글자도 다르지 않다', () => {
    setSeries({ pump: rawReading(23.456) });
    renderPanel(makeConfig([labelElement({ id: 'absent', text: '{value}{unit}' })], ['pump.state']));
    const withoutFlag = drawnTexts();

    cleanup();
    ctxStub = makeCtxStub();
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      ctxStub as unknown as CanvasRenderingContext2D,
    );
    setSeries({ pump: rawReading(23.456) });
    renderPanel(
      makeConfig([labelElement({ id: 'explicit', numeric: true, text: '{value}{unit}' })], ['pump.state']),
    );

    expect(drawnTexts()).toEqual(withoutFlag);
  });
});

describe('CanvasPanel — 숫자를 끄면 규칙은 "값 없음" 행만 남는다', () => {
  /** 규칙 두 줄을 단 사각형 — 비교 행 하나와 결측 행 하나. */
  function ruledRect(over: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      id: 'box',
      kind: 'rect',
      geometry: { x: 0, y: 0, w: 500, h: 400 },
      style: { fill: '#888888' },
      binding: { series: sid('pump.state'), agg: 'last' },
      rules: [
        { op: 'gt', value: 10, patch: { fill: '#ff0000' } },
        { op: 'nodata', value: 0, patch: { fill: '#0000ff' } },
      ],
      ...over,
    };
  }

  it('숫자를 켠 채로는 비교 행이 이긴다(종전 동작)', () => {
    setSeries({ pump: rawReading(50) });
    renderPanel(makeConfig([ruledRect()], ['pump.state']));

    expect(shapeFills()).toContain('#ff0000');
  });

  it('숫자를 끄면 비교 행은 일치하지 않고 결측 행이 남는다', () => {
    // 값은 여전히 오고 있다(50). 끈 것은 "숫자로 읽는다" 이지 "데이터가 없다" 가 아니다 —
    // 그럼에도 비교할 수가 없으므로 스칼라 행은 전부 빗나간다. 이 결과가 조용하면 사용자는
    // 고장으로 읽으므로, 편집기가 규칙 표 제목 뒤 `?` 로 이유를 말한다.
    setSeries({ pump: rawReading(50) });
    renderPanel(makeConfig([ruledRect({ numeric: false })], ['pump.state']));

    expect(shapeFills()).not.toContain('#ff0000');
    expect(shapeFills()).toContain('#0000ff');
  });

  it('결측 행조차 없으면 기본 스타일 그대로다 — 요소가 사라지지 않는다', () => {
    setSeries({ pump: rawReading('ON') });
    renderPanel(
      makeConfig(
        [ruledRect({ numeric: false, rules: [{ op: 'gt', value: 10, patch: { fill: '#ff0000' } }] })],
        ['pump.state'],
      ),
    );

    expect(shapeFills()).toContain('#888888');
  });
});

// --- 캔버스 크기는 편집 중 패널 크기다 (SPEC-CANVAS-006 M8 · REQ-07) ------
//
// **0.10.0 의 규칙이 여기서 뒤집힌다.** 그때 이 자리가 지키던 성질은 둘이고 서로 반대
// 방향이었다: (1) 아무것도 놓이지 않은 새 패널에서만 **한 번** 맞춘다, (2) 그 뒤로는
// 패널 크기가 아무리 바뀌어도 저장된 캔버스 크기를 건드리지 않는다. 그 제한의 근거는
// 하나뿐이었다 — 저장된 요소 좌표는 절대 캔버스 단위라, 캔버스가 바뀌면 사용자가 y=200 에
// 둔 도형이 가리키는 자리가 **조용히** 달라진다.
//
// **006 이 그 침묵을 없앴다.** M2 가 출력 영역 밖의 그림을 살렸고, M6 이 어디까지 패널에
// 나오는지를 경계와 흐림으로 그렸으며, M7 이 그 바깥에서 시작한 누름을 편집 처리자에
// 닿게 했다. 그래서 뜻이 달라진 요소는 사각형 안에 남거나 **눈에 보이게** 밖으로 나가고,
// 나가도 **끌어서 되돌릴 수 있다**. 위험했던 것은 움직임이 아니라 침묵이었고, 침묵이
// 사라졌으므로 이 유도가 상시가 된다(REQ-07 · 불변식 I15).
//
// 이 절이 지는 성질은 셋으로 갈린다(acceptance.md AC-07).
//   (AK) **표시와 유도가 한 게이트를 지난다** — 순서를 일정이 아니라 형상으로 지킨다.
//   (AL) **상자가 바뀌면 크기가 따라온다** — 두 축 모두, 출처를 묻지 않는다.
//   (AM) **쓰지 않아야 할 때는 쓰지 않는다** — 이 절에서 가장 무거운 갈래다.
//
// 그리고 이 절은 **걷어낸 "패널 비율에 맞춤" 단추의 단언 둘을 물려받는다**(AC-07 (AN)):
// `canvas` 패치 하나만 낼 것 · 요소 좌표를 함께 옮기지 않을 것. 셋째였던 **"폭 불변"**
// 은 옮겨 오지 않고 **폐기된다** — 두 축이 모두 유도되므로 그 단언은 이제 거짓이다.

/** 캔버스 크기 패치만 골라낸다(다른 패치와 섞이지 않게 한다). */
function canvasPatches(spy: ReturnType<typeof vi.fn>): Array<{ width: number; height: number }> {
  return spy.mock.calls
    .map((c) => (c[0] as { canvas?: { width: number; height: number } }).canvas)
    .filter((c): c is { width: number; height: number } => c !== undefined);
}

/** 패널 크기를 바꾼다(레이아웃 변경 — 대시보드에서 패널을 늘이는 일). */
function resizePanelTo(width: number, height: number): void {
  panelOuter = { width, height };
  act(() => {
    for (const ro of panelObservers) ro.cb([{ contentRect: { width, height } }]);
  });
}

/**
 * 패치를 **실제로 되먹이는** 숙주. 유도의 수렴을 재려면 이것이 있어야 한다.
 *
 * 패치를 삼키는 `vi.fn()` 숙주에서는 config 가 영영 그대로라, "두 번째 측정이 아무것도
 * 쓰지 않는다" 를 잴 수 없다 — 유도가 저장값을 되먹는 구현조차 통과한다.
 */
function StatefulSizeHost({
  clock,
  elements = [],
  canvas,
  onPatch,
}: {
  clock: ReturnType<typeof makeScheduler>;
  elements?: unknown[];
  canvas?: { width: number; height: number };
  onPatch: (patch: Record<string, unknown>) => void;
}) {
  const [cfg, setCfg] = useState<Record<string, unknown>>(() =>
    makeConfig(elements, undefined, canvas === undefined ? {} : { canvas }),
  );
  return (
    <CanvasEditDockRegion enabled>
      <CanvasPanel
        panelId="p1"
        config={cfg}
        onConfigChange={(patch) => {
          onPatch(patch);
          setCfg((prev) => ({ ...prev, ...patch }));
        }}
        forceEdit
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />
    </CanvasEditDockRegion>
  );
}

function renderStatefulSize(opts: {
  elements?: unknown[];
  canvas?: { width: number; height: number };
}) {
  const spy = vi.fn();
  const clock = makeScheduler();
  const view = render(<StatefulSizeHost clock={clock} onPatch={spy} {...opts} />);
  for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
  return { ...view, clock, spy };
}

describe('CanvasPanel — 표시와 유도가 한 게이트를 지난다 (AC-07 (AK) · I15 · R13)', () => {
  it('유도가 실제로 도는 상태에서는 **출력 영역 경계가 DOM 에 있다**', () => {
    // 형상을 재는 단언이다. 유도와 표시를 다른 게이트로 가르는 구현은 이 짝을 깨뜨리고,
    // 그때 되살아나는 것이 0.10.0 이 경고한 그 회귀다 — 크기가 바뀌어 모든 요소의 뜻이
    // 달라지는데 화면에 아무 단서가 없는 상태.
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([EDIT_RECT], { onConfigChange, forceEdit: true });

    // 유도가 정말 돌았다(꺼진 채로 잰 "유도 시험" 은 틀린 이유로 통과한다).
    expect(canvasPatches(onConfigChange)).toEqual([{ width: 1749, height: 796 }]);
    // 그 상태에서 경계와 흐림이 함께 서 있다.
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();
    expect(screen.getByTestId('canvas-region-scrim')).toBeTruthy();
  });

  it('경계가 없는 상태에서는 상자를 아무리 흔들어도 패치가 **0 건**이다', () => {
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([EDIT_RECT], { onConfigChange });

    expect(screen.queryByTestId('canvas-region-bounds')).toBeNull();
    resizePanelTo(900, 900);
    resizePanelTo(400, 1200);

    expect(canvasPatches(onConfigChange)).toEqual([]);
  });
});

describe('CanvasPanel — 상자가 바뀌면 크기가 따라온다 (AC-07 (AL) · REQ-07)', () => {
  it('두 축이 모두 잰 바깥 상자 **그 자체**로 쓰인다 — 어떤 축척도 곱하지 않는다', () => {
    // 고정 입력은 **고정점이 아닌** 짝이어야 한다(D7) — 이미 맞아 있으면 유도가 무동작이라
    // 이 시험이 실패할 수 없다. 저장된 캔버스는 기본값 500×400 이고 상자는 1749×796 이다.
    //
    // 0.4.0 의 폐기된 규칙(`floor(outer × 0.75)`)이 되살아나면 여기서 1311×597 이 나온다.
    // 그 규칙에서는 캔버스가 영원히 패널의 0.75 배라 REQ-07 이 제 이름("캔버스 크기 =
    // 패널 크기")을 지킬 수 없었고, 사용자가 화면을 보고 정확히 그것을 잡아냈다(위험 R21).
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([], { onConfigChange, forceEdit: true });

    expect(canvasPatches(onConfigChange)).toEqual([{ width: 1749, height: 796 }]);
  });

  it('요소가 있어도 유도가 돌고, **요소 좌표는 함께 가지 않는다** (걷어낸 단추의 단언)', () => {
    // 0.10.0 은 "요소가 하나라도 있으면 맞추지 않는다" 로 막았고, 그 근거("뜻이 조용히
    // 달라진다")가 006 에서 사라졌다. 대신 살아남는 것이 이 단언이다 — 종이를 고친 것이지
    // 그림을 고친 것이 아니다.
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([EDIT_RECT], { onConfigChange, forceEdit: true });

    expect(canvasPatches(onConfigChange)).toEqual([{ width: 1749, height: 796 }]);
    // 패치에 `elements` 가 실려 있으면 요소 좌표를 함께 옮긴 것이다.
    for (const call of onConfigChange.mock.calls) {
      expect(Object.keys(call[0] as object)).toEqual(['canvas']);
    }
  });

  it('트리거의 **출처를 묻지 않는다** — 타일 드래그든 창 크기 변경이든 같은 규칙이다', () => {
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT] });

    // 첫 측정(200×160) 뒤 이어지는 리사이즈 둘. 어느 것이 무엇 때문인지 묻지 않는다.
    expect(canvasPatches(spy)).toEqual([{ width: 200, height: 160 }]);
    resizePanelTo(1749, 796);
    resizePanelTo(600, 1200);

    expect(canvasPatches(spy)).toEqual([
      { width: 200, height: 160 },
      { width: 1749, height: 796 },
      { width: 600, height: 1200 },
    ]);
  });

  it('리사이즈 한 번은 많아야 **한 건**이고, 같은 상자를 다시 재면 **0 건**이다 (I19)', () => {
    // 유도가 저장값을 입력으로 먹으면 리사이즈 한 번이 최대 세 번의 연쇄 쓰기가 된다
    // (spec.md §기각한 안 — 출력 영역을 그대로 받아 적는다). 되먹이는 숙주라야 그 고리가
    // 실제로 돌 수 있고, 그래서 이 시험은 `vi.fn()` 이 아니라 `StatefulSizeHost` 를 쓴다.
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT] });
    expect(canvasPatches(spy)).toHaveLength(1);

    resizePanelTo(1749, 796);
    expect(canvasPatches(spy)).toHaveLength(2);

    // 같은 상자를 다시 통보한다 — 유도가 잰 상자만의 함수라 같은 값이 나오고, 받은 객체를
    // 그대로 돌려주므로 부르는 쪽의 참조 비교가 쓰기를 삼킨다.
    resizePanelTo(1749, 796);
    resizePanelTo(1749, 796);
    expect(canvasPatches(spy)).toHaveLength(2);
  });

  it('상자가 **1px** 만 바뀌어도 유도값이 1px 바뀌어 쓰기가 한 건 난다 (항등의 대가)', () => {
    // 0.4.0 까지는 `floor(outer × 0.75)` 가 일부 1px 변화를 삼켰다. 그 흡수는 **보장이
    // 아니었고**, 0.5.0 은 흡수를 아예 포기하는 대신 **상한**(측정당 많아야 한 건)만 지킨다.
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT] });
    const before = canvasPatches(spy).length;

    resizePanelTo(201, 160);

    expect(canvasPatches(spy)).toHaveLength(before + 1);
    expect(canvasPatches(spy).at(-1)).toEqual({ width: 201, height: 160 });
  });

  it('저장된 `canvas` 를 아무 값으로 놓아도 유도값이 같다 — 저장값은 입력이 아니다 (I19)', () => {
    // 이 단언이 실패하면 "출력 영역을 그대로 받아 적는" 기각된 안으로 되돌아간 것이다
    // (위험 R20). 저장값이 계산에 들어가면 두 짝이 서로 다른 유도값을 낸다.
    panelOuter = { width: 1749, height: 796 };
    for (const stored of [
      { width: 333, height: 222 },
      { width: 1, height: 1 },
      { width: 99999, height: 3 },
    ]) {
      cleanup();
      const onConfigChange = vi.fn();
      renderEditablePanel([EDIT_RECT], { onConfigChange, forceEdit: true, canvas: stored });
      expect(canvasPatches(onConfigChange), `${stored.width}x${stored.height}`).toEqual([
        { width: 1749, height: 796 },
      ]);
    }
  });

  it('격자 간격을 바꿔도 캔버스 크기가 **한 글자도** 바뀌지 않는다 (I20 · R20)', () => {
    // `gridStep` 은 표면이 `useState` 로 든 **저장되지 않는 표시 상태**다. 그것이 유도의
    // 입력이 되면 저장되지 않는 값이 저장되는 값을 고치게 되고, 같은 패널을 다른 간격으로
    // 연 두 사람이 캔버스 크기를 두고 다툰다.
    //
    // 되먹이는 숙주라야 이 가드가 문다: 유도가 `floor(outer ÷ step) × step` 이었다면
    // 수렴값이 1725×775 이고 간격을 10 으로 바꾸는 순간 1740×790 으로 다시 쓰인다.
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT], canvas: { width: 500, height: 400 } });
    resizePanelTo(1749, 796);
    const before = canvasPatches(spy).length;

    fireEvent.change(screen.getByTestId('canvas-grid-step'), { target: { value: '10' } });

    expect(canvasPatches(spy)).toHaveLength(before);
  });
});

describe('CanvasPanel — 쓰지 않아야 할 때는 쓰지 않는다 (AC-07 (AM) · 위험 R15 · D5)', () => {
  it('편집 중이 아니면 맞추지 않는다 — 보기만 하는 패널은 config 를 쓰지 않는다', () => {
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([], { onConfigChange });

    expect(canvasPatches(onConfigChange)).toEqual([]);
  });

  it('쓸 곳이 없으면(`onConfigChange` 부재) 상자가 아무리 바뀌어도 쓰지 않는다', () => {
    // 두 게이트가 하나로 겹친다(`canEdit` 가 곧 `onConfigChange` 존재). 그럼에도 둘 다
    // 잰다 — 한쪽 형상이 바뀌는 날 다른 쪽이 남아 있어야 한다.
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([], { forceEdit: true });

    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    // 여기서 예외가 나지 않는다는 사실 자체가 단언이다(쓸 곳이 없는 경로가 살아 있다).
    resizePanelTo(900, 900);
  });

  it('아직 재지 못한 상자(0)에서는 맞추지 않고 NaN 도 내지 않는다', () => {
    const onConfigChange = vi.fn();
    panelOuter = { width: 0, height: 0 };
    renderEditablePanel([], { onConfigChange, forceEdit: true });

    expect(canvasPatches(onConfigChange)).toEqual([]);
  });

  it('유도한 축이 1 미만이면 저장값을 그대로 둔다 — "모르면 손대지 않는다"', () => {
    const onConfigChange = vi.fn();
    panelOuter = { width: 1749, height: 0 };
    renderEditablePanel([], { onConfigChange, forceEdit: true });

    expect(canvasPatches(onConfigChange)).toEqual([]);
  });
});

describe('CanvasPanel — 리사이즈는 종이만 고치고 그림은 고치지 않는다 (AC-07 (AL) · AC-E20 (X))', () => {
  it('패널을 여러 번 늘여도 **요소 좌표는 한 글자도 바뀌지 않는다**', () => {
    // 0.10.0 은 이 자리에서 "저장된 캔버스 크기가 그대로다" 를 단언했다. 006 이 그 문장을
    // 뒤집었으므로(캔버스는 이제 상자를 따라간다), 살아남는 성질은 **요소 좌표 불변**이다 —
    // 그리고 그것이 이 유도를 안전하게 만드는 조건 그 자체다.
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT] });

    resizePanelTo(1749, 796);
    resizePanelTo(600, 1200);
    resizePanelTo(320, 240);

    expect(canvasPatches(spy).length).toBeGreaterThan(0);
    for (const call of spy.mock.calls) {
      expect(Object.keys(call[0] as object)).toEqual(['canvas']);
    }
  });

  it('그래서 그림이 **캔버스에 대해** 작아진다 — 그 대가를 수로 적어 둔다 (위험 R19)', () => {
    // 요소 좌표가 그대로인 채 캔버스만 커지므로, 옛 그림이 차지하는 몫이 줄어든다.
    // 사용자는 이 대가를 이미 받아들였고(spec.md §옛 패널을 편집으로 열면 무엇이 보이는가),
    // 여기서는 그것이 **조용하지 않다**는 것만 못박는다: 유도가 돈 그 상태에 경계가 있다.
    const { spy } = renderStatefulSize({ elements: [EDIT_RECT], canvas: { width: 500, height: 400 } });
    resizePanelTo(1749, 796);

    expect(canvasPatches(spy).at(-1)).toEqual({ width: 1749, height: 796 });
    // 500 ÷ 1749 = 28.6% · 400 ÷ 796 = 50.3% (spec.md 가 적은 그 수다).
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();
  });
});

// --- 작업 영역은 편집 게이팅에 얹힌다 (SPEC-CANVAS-006 M4) ---
//
// M2·M3 이 표면에 `workspace` 축을 세웠고, 여기서 그 축이 **패널의 편집 게이트에** 묶인다.
// 재는 것은 배선 하나다: `workspace` 가 정말 `edit.active` 를 나르는가, 그리고 편집이
// 꺼진 자리에서는 006 이전과 값이 같은가.
//
// 고정 입력은 **1749 × 796** 이다. 이 파일의 기본 상자 200×160 은 **작업 영역이 켜졌는지
// 꺼졌는지 구별이 흐리다**(꺼진 갈래에서 자투리가 0 이라 두 상자가 같은 자리에서 시작한다).
// 1749×796 은 정사각형도 아니고(0.8.0 (O)) 칸으로 나누어떨어지지도 않아(0.9.0 (R)) 두
// 갈래가 서로 다른 수를 낸다.
//
// **M8 이 두 갈래의 고정 입력을 갈랐다.** 편집을 켠 시험은 이제 **고정점**을 써야 한다
// (D7 — `canvas === outer === 1749×796`). 유도가 항등이라 편집을 켜는 순간 캔버스가 잰
// 상자로 덮이므로, 고정점이 아닌 짝을 쓰면 이 절이 재는 상자 수가 그 유도의 결과를 함께
// 지고 간다. 편집을 **끈** 시험은 종전 그대로 캔버스 500×400 을 쓴다 — 그 갈래에서는
// 캔버스가 상자에서 유도되지 않으므로 "캔버스와 비율이 다른 상자"(0.10.0 (V))가 여전히
// 만들어지고, 그 규율의 자리가 바로 여기다.
//   - 꺼짐(캔버스 500×400) → 상자 하나뿐인 것과 같다: 영역 980×784 · 자리 (384, 6) · 원점 (0,0).
//   - 켜짐(캔버스 1749×796, 고정점) → 작업 영역 1749×796 · 축소 상자 1311×597 · 칸 18 ·
//     축척 **0.72** · 출력 영역 **1259.28 × 573.12** · 원점 **(244, 111)**.
//
// 원점 (244, 111) 은 칸 18 의 배수가 **아니고**(244 % 18 = 10, 111 % 18 = 3) 격자 상자
// 폭·높이도 칸의 배수가 **아니다**(1749 % 18 = 3, 796 % 18 = 4) — D3 과 위험 R17 의 시험
// 조건을 유도 뒤에도 그대로 만족한다.
//
// 요소를 하나 두는 것에도 뜻이 있다 — 요소가 0개면 빈 상태 안내가 겹쳐 서고, 이 절이
// 재려는 것은 그 안내가 아니라 상자 둘이다.

describe('CanvasPanel — 작업 영역은 편집 게이팅에 얹힌다 (SPEC-CANVAS-006 M4)', () => {
  /** `'384px'` → `384`. */
  const px = (v: string): number => Number.parseFloat(v);
  const workspaceBoxEl = () => screen.getByTestId('canvas-workspace');
  const stageBoxEl = () => screen.getByTestId('canvas-stage');

  it('편집이 켜지면 표면이 상자를 둘로 짓는다 — `workspace` 가 `edit.active` 를 나른다', () => {
    panelOuter = { width: 1749, height: 796 };
    const onConfigChange = vi.fn();
    renderEditablePanel([EDIT_RECT], {
      onConfigChange,
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    // 편집이 정말 켜져 있다(꺼진 채로 잰 "작업 영역 시험" 은 틀린 이유로 통과한다).
    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
    // 그리고 이 짝이 정말 **고정점**이다 — 첫 측정에 `canvas` 패치가 0 건이어야 아래 수가
    // 시험이 적어 둔 그 캔버스 위에서 계산된 값이다(D7).
    expect(canvasPatches(onConfigChange)).toEqual([]);

    // 작업 영역은 잰 상자 전부다.
    expect(workspaceBoxEl().style.width).toBe('1749px');
    expect(workspaceBoxEl().style.height).toBe('796px');
    expect(workspaceBoxEl().style.left).toBe('0px');
    expect(workspaceBoxEl().style.top).toBe('0px');
    // 출력 영역은 그 안에 가운데로 앉고, 두 상자는 **실제로 다르다**.
    expect(stageBoxEl().style.width).toBe('1259.28px');
    expect(stageBoxEl().style.height).toBe('573.12px');
    expect(stageBoxEl().style.left).toBe('244px');
    expect(stageBoxEl().style.top).toBe('111px');
    expect(px(stageBoxEl().style.width)).toBeLessThan(px(workspaceBoxEl().style.width));
  });

  it('편집이 꺼지면 두 상자가 겹친다 — 대시보드의 그림이 006 이전과 같다', () => {
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([EDIT_RECT], { onConfigChange: vi.fn() });

    // 편집이 정말 꺼져 있다.
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();

    // 006 이전의 그 상자 하나와 크기·자리가 같고, 출력 영역이 그 왼쪽 위에 겹친다.
    expect(workspaceBoxEl().style.width).toBe('980px');
    expect(workspaceBoxEl().style.height).toBe('784px');
    expect(workspaceBoxEl().style.left).toBe('384px');
    expect(workspaceBoxEl().style.top).toBe('6px');
    expect(stageBoxEl().style.width).toBe(workspaceBoxEl().style.width);
    expect(stageBoxEl().style.height).toBe(workspaceBoxEl().style.height);
    expect(stageBoxEl().style.left).toBe('0px');
    expect(stageBoxEl().style.top).toBe('0px');
  });

  it('작업 영역과 편집 층은 **같은 게이트**를 지난다 — 넷째 토글이 없다', () => {
    // 형상으로 재는 단언이다: 편집 층이 있는 상태에서만 두 상자가 갈리고, 없는 상태에서는
    // 갈리지 않는다. 작업 영역에 제 토글을 따로 두는 구현은 이 짝을 깨뜨린다.
    panelOuter = { width: 1749, height: 796 };

    const split = (): boolean =>
      workspaceBoxEl().style.width !== stageBoxEl().style.width ||
      stageBoxEl().style.left !== '0px';

    renderEditablePanel([EDIT_RECT], {
      onConfigChange: vi.fn(),
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });
    expect(screen.queryByTestId('canvas-edit-overlay')).not.toBeNull();
    expect(split()).toBe(true);

    cleanup();

    renderEditablePanel([EDIT_RECT], { onConfigChange: vi.fn() });
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    expect(split()).toBe(false);
  });

  it('패널은 표면에 `className` 을 넘기지 않는다 — `overflow-hidden` 이 살아 있다', () => {
    // 표면의 `overflow-hidden` 은 **기본 `className` 에만** 있고, `className` 을 넘기면
    // 통째로 갈린다(종전 동작이며 표면 시험이 그 형상을 이미 못박고 있다). 그래서 이 한
    // 줄이 없으면, 패널에 `className` 을 넘기는 날 출력 영역 밖 손잡이의 클리핑 보장이
    // **조용히** 사라진다 — 어느 표면 시험도 그것을 잡지 못한다(패널이 무엇을 넘기는지는
    // 표면의 관심 밖이다).
    panelOuter = { width: 1749, height: 796 };
    renderEditablePanel([EDIT_RECT], {
      onConfigChange: vi.fn(),
      forceEdit: true,
      canvas: fixedPointCanvas(),
    });

    const surfaceRoot = workspaceBoxEl().parentElement as HTMLElement;
    expect(surfaceRoot.className.split(/\s+/)).toContain('overflow-hidden');
    expect(surfaceRoot.className.split(/\s+/)).toContain('relative');
  });
});
