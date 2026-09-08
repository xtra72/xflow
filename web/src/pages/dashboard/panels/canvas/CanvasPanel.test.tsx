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
import { render, cleanup, act } from '@testing-library/react';

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
import CanvasPanel from './CanvasPanel';
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
  it('빈 상태 안내를 그리고 캔버스 표면을 만들지 않는다', () => {
    const { getByTestId, queryByTestId, container } = renderPanel(makeConfig([]));

    expect(getByTestId('canvas-empty').textContent).toContain('dashboard.canvas.emptyState');
    expect(queryByTestId('canvas-surface')).toBeNull();
    expect(container.querySelector('canvas')).toBeNull();
  });

  it('config 가 아예 손상되어도 예외 없이 빈 상태로 떨어진다', () => {
    const { getByTestId } = renderPanel({ elements: 'not-an-array' });
    expect(getByTestId('canvas-empty')).toBeTruthy();
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
