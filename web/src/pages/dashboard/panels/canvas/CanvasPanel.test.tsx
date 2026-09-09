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

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { width: 200, height: 100 } }]);
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
      geometry: { x: 0, y: 0, w: 0.5, h: 0.5 },
      style: { fill: '#888888', stroke: '#000000', strokeWidth: 2 },
      binding: { series: sid('tank.level'), agg: 'last' },
      rules: [
        { op: 'gt', value: 80, patch: { fill: '#ff0000' } },
        { op: 'gt', value: 50, patch: { fill: '#ffff00', strokeWidth: 4 } },
      ],
    },
    { id: 'e1', kind: 'ellipse', geometry: { x: 0.5, y: 0, w: 0.3, h: 0.3 }, style: { fill: '#00ff00' } },
    { id: 'l1', kind: 'line', geometry: { x1: 0, y1: 1, x2: 1, y2: 1 }, style: { stroke: '#0000ff', strokeWidth: 1 } },
    { id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: { textColor: '#111111' }, text: 'label' },
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
            geometry: { x: 0.5, y: 0.5 },
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
            geometry: { x: 0.5, y: 0.5 },
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
      makeConfig([{ id: 'a', kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: {} }]),
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
      geometry: { x: 0, y: 0, w: 1, h: 1 },
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
          geometry: { x: 0, y: 0, w: 1, h: 1 },
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
      geometry: { x: 0, y: 0, w: 1, h: 1 },
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
          geometry: { x: 0, y: 0, w: 1, h: 1 },
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
        { id: 'plain', kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: { fill: '#cccccc' } },
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
          geometry: { x: 0, y: 0, w: 1, h: 1 },
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
          geometry: { x: 0, y: 0, w: 1, h: 1 },
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
            geometry: { x: 0, y: 0, w: 1, h: 1 },
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
            geometry: { x: 0, y: 0, w: 1, h: 1 },
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

/** 편집 배선을 켠 채 패널을 렌더한다. 프레임은 더 예약할 것이 없을 때까지 밀어 둔다. */
function renderEditablePanel(
  elements: unknown[],
  opts: { onConfigChange?: (patch: Record<string, unknown>) => void; forceEdit?: boolean } = {},
) {
  const clock = makeScheduler();
  const view = render(
    // 편집 도구(팔레트·격자·정렬·순서)는 패널 **밖**의 도크 자리에만 그려진다. 설정
    // 미리보기가 그 자리를 내므로, 도구를 보는 시험은 같은 형상을 흉내 낸다.
    <CanvasEditDockRegion enabled>
      <CanvasPanel
        panelId="p1"
        config={makeConfig(elements)}
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

/** 스테이지 200×100 위의 사각형 하나. px 상자는 (20, 10, 40, 20) 이다. */
const EDIT_RECT = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 },
  style: { fill: '#888888' },
};

/** config 를 실제로 갱신하는 숙주 — 드래그의 깨우기 경로를 끝까지 잇는다. */
function StatefulHost({ clock }: { clock: ReturnType<typeof makeScheduler> }) {
  const [cfg, setCfg] = useState<Record<string, unknown>>(() => makeConfig([EDIT_RECT]));
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
    renderEditablePanel([EDIT_RECT], { onConfigChange: vi.fn(), forceEdit: true });

    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
    expect(screen.queryByTestId('canvas-edit-toggle')).toBeNull();
  });
});

describe('CanvasPanel — 드래그가 onConfigChange 로 흘러간다 (AC-03)', () => {
  it('요소를 끌면 elements 패치가 나간다 (001 이 받아만 두었던 자리다)', async () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([EDIT_RECT], { onConfigChange, forceEdit: true });

    sendToOverlay('pointerdown', 30, 15);
    sendToOverlay('pointermove', 50, 25);
    await nextBrowserFrame();

    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as { elements: Array<{ id: string; geometry: unknown }> };
    expect(Object.keys(patch)).toEqual(['elements']);
    expect(patch.elements.find((el) => el.id === 'a')!.geometry).toEqual({
      x: 0.2,
      y: 0.2,
      w: 0.2,
      h: 0.2,
    });
  });

  it('편집이 꺼져 있으면 같은 누름이 아무것도 바꾸지 않는다', async () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([EDIT_RECT], { onConfigChange });

    // 오버레이가 아예 없으므로 누름은 캔버스로 가고 아무 일도 일어나지 않는다.
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
    fireEvent(screen.getByTestId('canvas-surface'), panelPointer('pointerdown', 30, 15));
    fireEvent(screen.getByTestId('canvas-surface'), panelPointer('pointermove', 50, 25));
    await nextBrowserFrame();

    expect(onConfigChange).not.toHaveBeenCalled();
  });
});

describe('CanvasPanel — 편집기가 유휴 정지를 깨지 않는다 (AC-E4)', () => {
  it('선택과 호버는 프레임을 단 한 건도 요청하지 않는다', () => {
    const { clock } = renderEditablePanel(
      [EDIT_RECT, { ...EDIT_RECT, id: 'b', geometry: { x: 0.5, y: 0.5, w: 0.2, h: 0.2 } }],
      { onConfigChange: vi.fn(), forceEdit: true },
    );
    // 유휴에 들었다 — 여기서부터의 요청은 전부 편집기 탓이다.
    expect(clock.pending).toBe(0);
    const before = clock.requested;

    // 고르고, 다른 요소로 옮기고, 핸들 자리를 지나간다(호버).
    sendToOverlay('pointerdown', 30, 15);
    sendToOverlay('pointerup', 30, 15);
    sendToOverlay('pointerdown', 110, 55);
    sendToOverlay('pointerup', 110, 55);
    sendToOverlay('pointermove', 120, 60);
    sendToOverlay('pointermove', 40, 20);

    expect(screen.getByTestId('canvas-selection-b')).toBeTruthy();
    expect(clock.requested).toBe(before);
    expect(clock.pending).toBe(0);
  });

  it('끌어서 기하가 바뀌면 종전의 props 변경 경로로 프레임이 예약된다', async () => {
    const clock = makeScheduler();
    render(<StatefulHost clock={clock} />);
    for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
    const before = clock.requested;

    sendToOverlay('pointerdown', 30, 15);
    sendToOverlay('pointermove', 50, 25);
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
    });
    expect(clock.pending).toBe(0);

    // 고르기까지가 전제다(핸들은 하나만 골랐을 때 뜬다).
    sendToOverlay('pointerdown', 30, 15);
    sendToOverlay('pointerup', 30, 15);
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
    sendToOverlay('pointerdown', 30, 15);
    sendToOverlay('pointerup', 30, 15);
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
    renderEditablePanel([], { onConfigChange: vi.fn(), forceEdit: true });

    expect(screen.getByTestId('canvas-empty')).toBeTruthy();
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.getByTestId(`canvas-palette-add-${kind}`)).toBeTruthy();
    }
  });

  it('요소 0개에서 팔레트를 누르면 첫 요소가 config 로 흘러간다', () => {
    const onConfigChange = vi.fn();
    renderEditablePanel([], { onConfigChange, forceEdit: true });

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

/** 그 팬아웃을 낳는 config — store 참조는 **하나**뿐이다(태그 필터 한 줄). */
function fanoutConfig(elements: unknown[]): Record<string, unknown> {
  return {
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
  geometry: { x: 0.5, y: 0.5 },
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

/** 숙주를 렌더하고 첫 행을 펼친다(세부는 기본이 접힘이다). */
function renderBindingHost() {
  const clock = makeScheduler();
  const view = render(<BindingHost clock={clock} />);
  for (let i = 0; i < 5 && clock.pending > 0; i++) clock.flush(i * 16);
  fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
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
      const cfg = { data_source: 'store', elements: [VALUE_LABEL] };
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
