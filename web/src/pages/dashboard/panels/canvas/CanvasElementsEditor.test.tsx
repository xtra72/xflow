// 캔버스 요소 목록 편집기 테스트 (SPEC-CANVAS-001 T8).
//
// 이 편집기가 지는 약속은 넷이고, 테스트의 무게중심도 거기에 둔다.
//   1. 목록 조작(추가·삭제·순서)이 배열 순서 = z-order 를 그대로 바꾼다.
//   2. 기하 칸이 **종류가 요구하는 축만** 낸다(rect/ellipse 는 x·y·w·h, line 은 두 끝점,
//      text 는 기준점).
//   3. 종류를 바꿔도 스타일·문구·바인딩·규칙이 살아남고 기하만 새 형상으로 옮겨진다.
//   4. 여기서 내보낸 config 를 `parseCanvasConfig` 로 읽으면 **같은 것**이 나온다.
//      넷 중 이것이 가장 센 보증이다 — 편집기와 파서가 갈라지는 순간을 잡는다.
//
// i18n 은 `CanvasRuleTableEditor.test.tsx` 선례대로 키 통과 스텁으로 갈아끼운다. 그래서
// 개별 컨트롤은 aria-label 이 아니라 `data-testid` 로 집는다(스텁 t 는 `{index}` 를
// 채워 주지 않는다).
//
// `ColorSwatchButton` 은 실물을 쓴다 — 색 칸이 실제로 열리는지가 아니라 편집기가 그
// 계약(`color`/`onChange`/`ariaLabel`/`testId`)을 지키는지를 보려는 것이고, 스텁으로
// 바꾸면 그 계약이 검사되지 않는다. 다만 그 컴포넌트도 `useTranslation` 을 쓰므로 위
// 스텁이 함께 덮는다.
//
// 남은 미도달 분기 세 갈래는 **jsdom 에서 도달할 수 없는 방어**이며 죽은 코드가 아니다.
// `CanvasRuleTableEditor.test.tsx` 가 같은 이유로 남긴 것과 같은 목록이다.
//   - `parseCoordinate` / `parseOptionalNumber` 의 비유한 갈래: jsdom 의 number 입력은
//     `1e999` 같은 문자열을 빈 값으로 소독해 버려 Infinity 가 함수까지 닿지 않는다.
//     실제 브라우저는 그 문자열을 그대로 넘기므로 가드는 유지한다 — NaN 이 기하에
//     들어가면 그 요소가 화면에서 통째로 사라진다.
//   - `moveAt` 의 범위 가드: 양 끝 요소의 바깥쪽 이동 버튼이 disabled 라 클릭이 발생하지
//     않는다(규칙 표 · `AcControlThresholdsSection` 과 같은 형태의 가드다).
//
// @spec SPEC-CANVAS-001

import { useState } from 'react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { StoreSourceConfig } from '../charts/chartChannelTypes';
import type { CanvasElement, CanvasPanelConfig } from './canvasConfig';
import { parseCanvasConfig } from './canvasConfig';
import CanvasElementsEditor from './CanvasElementsEditor';

afterEach(cleanup);

// --- 픽스처 -------------------------------------------------------------

/** store 소스 두 줄. 동일성 키는 `storeSeriesId` 의 `"<key> <metric> <k=v,...>"` 형식이다. */
const STORE_SOURCE: StoreSourceConfig = {
  agent_name: 'a1',
  series: [
    { key: 'temp', field: 'value', tags: { room: 'A' }, alias: '실습실 온도' },
    { key: 'hum', field: 'value' },
  ],
  time_window_ms: 60_000,
  interval_ms: 1_000,
  aggregation: 'average',
};

const TEMP_ID = 'temp value room=A';
const HUM_ID = 'hum value ';

/** 기본 config — store 소스 + 주어진 요소들. */
function cfg(elements: CanvasElement[], extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    channel_name: '',
    data_source: 'store',
    store_source: STORE_SOURCE,
    elements,
    ...extra,
  };
}

/** 사각형 요소 하나. */
function rect(over: Partial<CanvasElement> = {}): CanvasElement {
  return {
    id: 'r1',
    kind: 'rect',
    geometry: { x: 0.1, y: 0.2, w: 0.3, h: 0.4 },
    style: {},
    ...over,
  } as CanvasElement;
}

/** 편집기를 그리고 onConfigChange 스파이를 돌려준다. */
function setup(config: Record<string, unknown>) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  return onConfigChange;
}

const testid = (id: string): HTMLElement => screen.getByTestId(id);

/** 마지막 패치에서 요소 배열을 꺼낸다. */
function lastElements(spy: ReturnType<typeof vi.fn>): CanvasElement[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
  return patch.elements as CanvasElement[];
}

/** 마지막 패치 자체(요소 밖의 축을 볼 때). */
function lastPatch(spy: ReturnType<typeof vi.fn>): Record<string, unknown> {
  expect(spy).toHaveBeenCalled();
  return spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
}

/**
 * 다이얼로그와 **같은 형상**으로 편집기를 감싼다 — 패치를 최상위 얕은 병합으로 누적하고
 * 그 결과를 다시 `config` 로 내려보낸다(`useDraftPanelConfig.patchConfig` 규약).
 *
 * 왕복 시험에는 스파이가 아니라 이 하네스가 필요하다. 편집기는 제어 컴포넌트라 config 가
 * 되돌아오지 않으면 매 편집이 **원본에서** 다시 계산되고, 그러면 연속 편집 중 마지막
 * 하나만 패치에 남는다 — 앞선 편집이 왕복 단언을 통과한 적이 없게 된다.
 */
function setupStateful(initial: Record<string, unknown>): { config: Record<string, unknown> } {
  const live = { config: initial };
  function Harness() {
    const [config, setConfig] = useState(initial);
    live.config = config;
    return (
      <CanvasElementsEditor
        config={config}
        onConfigChange={(patch) => setConfig((prev) => ({ ...prev, ...patch }))}
      />
    );
  }
  render(<Harness />);
  return live;
}

// --- 목록 조작 -----------------------------------------------------------

describe('CanvasElementsEditor — 요소 목록', () => {
  it('요소가 없으면 빈 상태 안내를 보이고 행을 그리지 않는다', () => {
    setup(cfg([]));
    expect(screen.getByTestId('canvas-element-empty')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-0')).toBeNull();
  });

  it('요소를 배열 순서대로 그리고 1-기반 순번을 보인다', () => {
    setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]));
    expect(testid('canvas-element-order-0').textContent).toBe('1');
    expect(testid('canvas-element-order-1').textContent).toBe('2');
    expect(testid('canvas-element-0').getAttribute('data-element-id')).toBe('a');
    expect(testid('canvas-element-1').getAttribute('data-element-id')).toBe('b');
  });

  it('배열 순서가 곧 앞뒤 순서라는 사실을 화면에 적는다', () => {
    setup(cfg([]));
    expect(screen.getByText('dashboard.canvas.elements.orderHint')).toBeTruthy();
  });

  it('종류별 추가 버튼이 그 종류의 기본 기하를 가진 요소를 끝에 붙인다', () => {
    const spy = setup(cfg([]));

    fireEvent.click(testid('canvas-element-add-rect'));
    let next = lastElements(spy);
    expect(next).toHaveLength(1);
    expect(next[0]!.kind).toBe('rect');
    expect(next[0]!.geometry).toEqual({ x: 0.1, y: 0.1, w: 0.2, h: 0.2 });
    expect(next[0]!.id).toBe('el-1');

    cleanup();
    const spy2 = setup(cfg([]));
    fireEvent.click(testid('canvas-element-add-line'));
    next = lastElements(spy2);
    expect(next[0]!.kind).toBe('line');
    expect(next[0]!.geometry).toEqual({ x1: 0.1, y1: 0.5, x2: 0.9, y2: 0.5 });

    cleanup();
    const spy3 = setup(cfg([]));
    fireEvent.click(testid('canvas-element-add-text'));
    expect(lastElements(spy3)[0]!.geometry).toEqual({ x: 0.5, y: 0.5 });

    cleanup();
    const spy4 = setup(cfg([]));
    fireEvent.click(testid('canvas-element-add-ellipse'));
    expect(lastElements(spy4)[0]!.kind).toBe('ellipse');
  });

  it('신규 id 는 이미 쓰인 id 를 피해 결정적으로 붙는다', () => {
    const spy = setup(cfg([rect({ id: 'el-1' }), rect({ id: 'el-2' })]));
    fireEvent.click(testid('canvas-element-add-rect'));
    expect(lastElements(spy)[2]!.id).toBe('el-3');
  });

  it('요소를 지우면 그 요소만 빠진다', () => {
    const spy = setup(cfg([rect({ id: 'a' }), rect({ id: 'b' }), rect({ id: 'c' })]));
    fireEvent.click(testid('canvas-element-delete-1'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['a', 'c']);
  });

  it('위/아래 이동이 배열 순서(= 앞뒤 순서)를 바꾼다', () => {
    const spy = setup(cfg([rect({ id: 'a' }), rect({ id: 'b' }), rect({ id: 'c' })]));

    fireEvent.click(testid('canvas-element-move-down-0'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['b', 'a', 'c']);

    fireEvent.click(testid('canvas-element-move-up-2'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['a', 'c', 'b']);
  });

  it('양 끝 요소의 바깥쪽 이동 버튼은 막혀 있다', () => {
    setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]));
    expect((testid('canvas-element-move-up-0') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-element-move-down-1') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-element-move-down-0') as HTMLButtonElement).disabled).toBe(false);
  });
});

// --- 종류별 기하 ---------------------------------------------------------

describe('CanvasElementsEditor — 종류별 기하 칸', () => {
  it('rect · ellipse 는 x · y · w · h 를 낸다', () => {
    setup(cfg([rect(), rect({ id: 'e1', kind: 'ellipse' })]));
    for (const axis of ['x', 'y', 'w', 'h']) {
      expect(screen.getByTestId(`canvas-element-geo-${axis}-0`)).toBeTruthy();
      expect(screen.getByTestId(`canvas-element-geo-${axis}-1`)).toBeTruthy();
    }
    expect(screen.queryByTestId('canvas-element-geo-x1-0')).toBeNull();
    expect((testid('canvas-element-geo-w-0') as HTMLInputElement).value).toBe('0.3');
  });

  it('line 은 두 끝점만 낸다', () => {
    setup(cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0.1, x2: 1, y2: 0.9 }, style: {} }]));
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect(screen.getByTestId(`canvas-element-geo-${axis}-0`)).toBeTruthy();
    }
    expect(screen.queryByTestId('canvas-element-geo-w-0')).toBeNull();
    expect((testid('canvas-element-geo-y2-0') as HTMLInputElement).value).toBe('0.9');
  });

  it('text 는 기준점만 낸다', () => {
    setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 0.4, y: 0.6 }, style: {}, text: 'hi' }]));
    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-geo-y-0')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-geo-w-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-geo-x1-0')).toBeNull();
  });

  it('기하 칸은 0..1 밖의 값도 그대로 받는다 — 스테이지에 걸치는 배치도 뜻이 있다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '-0.5' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: -0.5, y: 0.2, w: 0.3, h: 0.4 });

    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '2.5' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.1, y: 0.2, w: 2.5, h: 0.4 });
  });

  it('빈 기하 칸은 0 으로 떨어진다 — NaN 이 들어가면 요소가 통째로 사라진다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-geo-y-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.1, y: 0, w: 0.3, h: 0.4 });
  });

  it('line 과 text 의 기하 칸도 같은 경로로 편집된다', () => {
    const spy = setup(
      cfg([
        { id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0, x2: 1, y2: 1 }, style: {} },
        { id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: {} },
      ]),
    );
    fireEvent.change(testid('canvas-element-geo-x2-0'), { target: { value: '0.75' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0, y1: 0, x2: 0.75, y2: 1 });

    fireEvent.change(testid('canvas-element-geo-y-1'), { target: { value: '0.25' } });
    expect(lastElements(spy)[1]!.geometry).toEqual({ x: 0.5, y: 0.25 });
  });
});

// --- 종류 변경 -----------------------------------------------------------

describe('CanvasElementsEditor — 종류 변경', () => {
  /** 스타일·문구·바인딩·규칙을 모두 가진 요소. 종류를 바꿔도 이것들이 살아야 한다. */
  const rich = rect({
    id: 'rich',
    style: { fill: '#ff0000', opacity: 0.5 },
    text: '{value} {unit}',
    decimals: 2,
    unit: '°C',
    binding: { series: TEMP_ID, agg: 'last' },
    rules: [{ op: 'gt', value: 90, patch: { fill: '#00ff00' } }],
    tween: { duration_ms: 200, easing: 'linear' },
  });

  it('rect → line: 기준점을 살리고 대응 없는 칸만 기본값으로 채운다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });

    const el = lastElements(spy)[0]!;
    expect(el.kind).toBe('line');
    expect(el.geometry).toEqual({ x1: 0.1, y1: 0.2, x2: 0.9, y2: 0.5 });
  });

  it('rect → text: 기준점만 남는다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.1, y: 0.2 });
  });

  it('line → rect: 첫 끝점이 좌상단이 되고 크기는 기본값이다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0.3, y1: 0.4, x2: 0.8, y2: 0.9 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'rect' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.3, y: 0.4, w: 0.2, h: 0.2 });
  });

  it('line → text: 첫 끝점이 정렬 기준점이 된다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0.2, y1: 0.6, x2: 0.8, y2: 0.6 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.2, y: 0.6 });
  });

  it('text → text 처럼 형상이 이미 맞으면 기준점을 그대로 둔다', () => {
    const spy = setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 0.3, y: 0.9 }, style: {} }]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.3, y: 0.9 });
  });

  it('line → line 도 두 끝점을 그대로 둔다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0.1, x2: 0.5, y2: 0.7 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0, y1: 0.1, x2: 0.5, y2: 0.7 });
  });

  it('text → line · text → rect 도 기준점을 살린다', () => {
    const spy = setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 0.7, y: 0.8 }, style: {} }]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0.7, y1: 0.8, x2: 0.9, y2: 0.5 });

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.7, y: 0.8, w: 0.2, h: 0.2 });
  });

  it('rect → ellipse 처럼 형상이 같으면 기하가 그대로다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 0.1, y: 0.2, w: 0.3, h: 0.4 });
  });

  it('종류를 바꿔도 스타일 · 문구 · 바인딩 · 규칙 · 트윈은 잃지 않는다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });

    const el = lastElements(spy)[0]!;
    expect(el.id).toBe('rich');
    expect(el.style).toEqual({ fill: '#ff0000', opacity: 0.5 });
    expect(el.text).toBe('{value} {unit}');
    expect(el.decimals).toBe(2);
    expect(el.unit).toBe('°C');
    expect(el.binding).toEqual({ series: TEMP_ID, agg: 'last' });
    expect(el.rules).toEqual([{ op: 'gt', value: 90, patch: { fill: '#00ff00' } }]);
    expect(el.tween).toEqual({ duration_ms: 200, easing: 'linear' });
  });
});

// --- 바인딩 --------------------------------------------------------------

describe('CanvasElementsEditor — 바인딩 선택', () => {
  it('store 소스의 시리즈를 동일성 키 공간으로 낸다', () => {
    setup(cfg([rect()]));
    const select = testid('canvas-element-binding-0') as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).toEqual(['', TEMP_ID, HUM_ID]);
    // 표시는 사람이 읽는 이름(alias)이고, 값은 동일성 키다 — 이름을 바꿔도 바인딩이 산다.
    expect(select.options[1]!.textContent).toBe('실습실 온도');
  });

  it('시리즈를 고르면 동일성 키로 바인딩하고 집계는 last 다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-binding-0'), { target: { value: HUM_ID } });
    expect(lastElements(spy)[0]!.binding).toEqual({ series: HUM_ID, agg: 'last' });
  });

  it('"바인딩 없음"을 고르면 binding 키 자체가 사라진다 — 정적 도형은 합법이다', () => {
    const spy = setup(cfg([rect({ binding: { series: TEMP_ID, agg: 'last' } })]));
    fireEvent.change(testid('canvas-element-binding-0'), { target: { value: '' } });
    const el = lastElements(spy)[0]!;
    expect(el.binding).toBeUndefined();
    expect('binding' in el).toBe(false);
  });

  it('바인딩이 없으면 규칙 표를 읽기 전용으로 잠근다', () => {
    setup(cfg([rect(), rect({ id: 'b', binding: { series: TEMP_ID, agg: 'last' } })]));
    // disabled 는 규칙 표의 안내 문구와 "규칙 추가" 버튼 상태로 드러난다.
    const hints = screen.getAllByTestId('canvas-rule-disabled-hint');
    expect(hints).toHaveLength(1);
    const addButtons = screen.getAllByTestId('canvas-rule-add') as HTMLButtonElement[];
    expect(addButtons[0]!.disabled).toBe(true);
    expect(addButtons[1]!.disabled).toBe(false);
  });

  it('규칙 표의 변경은 그 요소의 rules 로 올라가고, 표를 비우면 키가 사라진다', () => {
    const spy = setup(
      cfg([
        rect({
          binding: { series: TEMP_ID, agg: 'last' },
          rules: [{ op: 'gt', value: 1, patch: {} }],
        }),
      ]),
    );
    fireEvent.click(testid('canvas-rule-delete-0'));
    const el = lastElements(spy)[0]!;
    expect(el.rules).toBeUndefined();
    expect('rules' in el).toBe(false);
  });

  it('소스에 없는 저장된 바인딩은 선택지로 되살려 조용히 잃지 않는다', () => {
    setup(cfg([rect({ binding: { series: 'ghost key ', agg: 'last' } })]));
    const select = testid('canvas-element-binding-0') as HTMLSelectElement;
    expect(select.value).toBe('ghost key ');
    expect(Array.from(select.options).map((o) => o.value)).toContain('ghost key ');
  });

  it('tsdb 소스에서는 조회 이름 공간을 낸다 — CanvasPanel 의 폴백과 같은 공간이다', () => {
    setup({
      channel_name: '',
      data_source: 'tsdb',
      tsdb_source: {
        backend: 'influxdb',
        agent_name: 'a',
        series: [{ key: 'm1', field: 'v', alias: 'Alpha' }, { key: 'm2', field: 'v' }],
        time_window_ms: 1000,
        interval_ms: 100,
      },
      elements: [rect()],
    });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values).toEqual(['', 'Alpha', 'm2 · v']);
  });

  it('sysmetrics 소스도 store 형상으로 옮긴 이름 공간을 낸다', () => {
    setup({
      channel_name: '',
      data_source: 'sysmetrics',
      sysmetrics_source: {
        agent_id: 'ag1',
        agent_name: 'a',
        series: [{ key: 'cpu.usage_percent' }],
        time_window_ms: 1000,
        interval_ms: 100,
      },
      elements: [rect()],
    });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values.length).toBeGreaterThan(1);
    expect(values[0]).toBe('');
  });

  it('소스가 없거나 data_source 가 없으면 "바인딩 없음" 하나만 낸다', () => {
    // data_source 미지정은 store 로 읽는다(기존 패널 config 하위호환 규약).
    setup({ elements: [rect()] });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values).toEqual(['']);
  });

  it('tsdb · sysmetrics 소스 블록이 비어 있어도 빈 목록으로 견딘다', () => {
    setup({ data_source: 'tsdb', elements: [rect()] });
    expect((testid('canvas-element-binding-0') as HTMLSelectElement).options).toHaveLength(1);

    cleanup();
    setup({ data_source: 'sysmetrics', elements: [rect()] });
    expect((testid('canvas-element-binding-0') as HTMLSelectElement).options).toHaveLength(1);
  });

  it('field 없는 store 시리즈도 동일성 키를 만든다 — 빈 metric 자리가 남는다', () => {
    setup({
      data_source: 'store',
      store_source: { agent_name: 'a', series: [{ key: 'k' }], time_window_ms: 1, interval_ms: 1 },
      elements: [rect()],
    });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values).toEqual(['', 'k  ']);
  });

  it('같은 동일성 키가 두 번 나오면 앞선 것만 남는다', () => {
    setup({
      channel_name: '',
      data_source: 'store',
      store_source: {
        agent_name: 'a',
        series: [
          { key: 'k', field: 'v' },
          { key: 'k', field: 'v' },
        ],
        time_window_ms: 1,
        interval_ms: 1,
      },
      elements: [rect()],
    });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values).toEqual(['', 'k v ']);
  });
});

// --- 옵셔널 필드의 부재 규율 ---------------------------------------------

describe('CanvasElementsEditor — 빈 칸은 부재다', () => {
  it('스타일 수치 칸을 비우면 그 키가 사라진다', () => {
    const spy = setup(cfg([rect({ style: { strokeWidth: 3, opacity: 0.5, fontSize: 20 } })]));

    fireEvent.change(testid('canvas-element-stroke-width-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ opacity: 0.5, fontSize: 20 });

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ strokeWidth: 3, fontSize: 20 });

    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ strokeWidth: 3, opacity: 0.5 });
  });

  it('스타일 수치는 파서가 죄는 범위 안으로 먼저 죈다', () => {
    const spy = setup(cfg([rect()]));

    fireEvent.change(testid('canvas-element-stroke-width-0'), { target: { value: '-4' } });
    expect(lastElements(spy)[0]!.style.strokeWidth).toBe(0);

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '3' } });
    expect(lastElements(spy)[0]!.style.opacity).toBe(1);

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '-1' } });
    expect(lastElements(spy)[0]!.style.opacity).toBe(0);

    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '18' } });
    expect(lastElements(spy)[0]!.style.fontSize).toBe(18);
  });

  it('열거형 스타일 칸의 "미지정"은 키를 지우고, 값을 고르면 키가 생긴다', () => {
    const spy = setup(cfg([rect({ style: { fontWeight: 'bold', align: 'center', visible: false } })]));

    fireEvent.change(testid('canvas-element-font-weight-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ align: 'center', visible: false });

    fireEvent.change(testid('canvas-element-align-0'), { target: { value: 'right' } });
    expect(lastElements(spy)[0]!.style.align).toBe('right');

    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: 'show' } });
    expect(lastElements(spy)[0]!.style.visible).toBe(true);

    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ fontWeight: 'bold', align: 'center' });

    fireEvent.change(testid('canvas-element-font-weight-0'), { target: { value: 'normal' } });
    expect(lastElements(spy)[0]!.style.fontWeight).toBe('normal');

    fireEvent.change(testid('canvas-element-align-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ fontWeight: 'bold', visible: false });

    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: 'hide' } });
    expect(lastElements(spy)[0]!.style.visible).toBe(false);
  });

  it('범위 안의 불투명도는 그대로 실린다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '0.4' } });
    expect(lastElements(spy)[0]!.style.opacity).toBe(0.4);
  });

  it('문구 · 단위에 값을 넣으면 그대로 실린다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{name}: {value}' } });
    expect(lastElements(spy)[0]!.text).toBe('{name}: {value}');

    fireEvent.change(testid('canvas-element-unit-0'), { target: { value: 'kW' } });
    expect(lastElements(spy)[0]!.unit).toBe('kW');
  });

  it('색 칸에서 고른 색이 스타일 키가 되고, "미설정"은 그 키를 지운다', () => {
    const spy = setup(cfg([rect()]));

    // 실물 ColorSwatchButton 을 쓴다 — 스와치를 열고 팔레트에서 고르는 경로가
    // 편집기가 지키는 계약(color/onChange/ariaLabel/testId)을 실제로 검사한다.
    fireEvent.click(testid('canvas-element-fill-0'));
    fireEvent.click(screen.getByLabelText('#3b82f6'));
    expect(lastElements(spy)[0]!.style).toEqual({ fill: '#3b82f6' });

    fireEvent.click(testid('canvas-element-stroke-0'));
    fireEvent.click(screen.getByLabelText('#ef4444'));
    expect(lastElements(spy)[0]!.style).toEqual({ stroke: '#ef4444' });

    fireEvent.click(testid('canvas-element-text-color-0'));
    fireEvent.click(screen.getByLabelText('#10b981'));
    expect(lastElements(spy)[0]!.style).toEqual({ textColor: '#10b981' });
  });

  it('배경색 스와치의 "미설정"은 background 를 지운다', () => {
    const spy = setup(cfg([], { background: '#101010' }));
    fireEvent.click(testid('canvas-panel-background'));
    fireEvent.click(screen.getByLabelText('dashboard.colorSwatch.defaultAria'));
    expect(lastPatch(spy)).toEqual({ background: undefined });
  });

  it('문구 · 단위를 비우면 그 키가 사라진다(공백만 있는 값도 같다)', () => {
    const spy = setup(cfg([rect({ text: 'x', unit: '°C' })]));

    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '' } });
    let el = lastElements(spy)[0]!;
    expect('text' in el).toBe(false);
    expect(el.unit).toBe('°C');

    fireEvent.change(testid('canvas-element-unit-0'), { target: { value: '   ' } });
    el = lastElements(spy)[0]!;
    expect('unit' in el).toBe(false);
  });

  it('소수 자리는 비음수 정수로 죄고, 비우면 키가 사라진다', () => {
    const spy = setup(cfg([rect({ decimals: 2 })]));

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '3.7' } });
    expect(lastElements(spy)[0]!.decimals).toBe(3);

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '-1' } });
    expect(lastElements(spy)[0]!.decimals).toBe(0);

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '' } });
    expect('decimals' in lastElements(spy)[0]!).toBe(false);
  });

  it('문구 토큰 3종을 요소마다 옆에 적어 명세를 찾지 않아도 되게 한다', () => {
    setup(cfg([rect()]));
    expect(testid('canvas-element-token-help-0').textContent).toBe(
      'dashboard.canvas.elements.tokenHelp',
    );
  });
});

// --- 트윈 ---------------------------------------------------------------

describe('CanvasElementsEditor — 트윈', () => {
  it('요소 트윈 지속 시간을 넣으면 이징과 함께 덮어쓰기가 생긴다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '500' } });
    expect(lastElements(spy)[0]!.tween).toEqual({ duration_ms: 500, easing: 'ease-out' });
  });

  it('요소 트윈 이징만 바꾸면 기존 지속 시간을 지킨다', () => {
    const spy = setup(cfg([rect({ tween: { duration_ms: 300, easing: 'linear' } })]));
    fireEvent.change(testid('canvas-element-tween-easing-0'), { target: { value: 'ease-in' } });
    expect(lastElements(spy)[0]!.tween).toEqual({ duration_ms: 300, easing: 'ease-in' });
  });

  it('지속 시간을 비우면 덮어쓰기 자체가 사라진다(= 패널 기본 사용)', () => {
    const spy = setup(cfg([rect({ tween: { duration_ms: 300, easing: 'linear' } })]));
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '' } });
    expect('tween' in lastElements(spy)[0]!).toBe(false);
  });

  it('덮어쓰기가 없는 요소의 이징만 건드리면 아무 트윈도 생기지 않는다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-tween-easing-0'), { target: { value: 'ease-in' } });
    expect('tween' in lastElements(spy)[0]!).toBe(false);
  });

  it('음수 지속 시간은 0(즉시 전환)으로 죈다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '-100' } });
    expect(lastElements(spy)[0]!.tween).toEqual({ duration_ms: 0, easing: 'ease-out' });
  });

  it('패널 기본 트윈과 배경색은 요소 밖의 축으로 올라간다', () => {
    const spy = setup(cfg([], { tween: { duration_ms: 300, easing: 'ease-out' } }));

    fireEvent.change(testid('canvas-panel-tween-duration'), { target: { value: '800' } });
    expect(lastPatch(spy)).toEqual({ tween: { duration_ms: 800, easing: 'ease-out' } });

    fireEvent.change(testid('canvas-panel-tween-easing'), { target: { value: 'linear' } });
    expect(lastPatch(spy)).toEqual({ tween: { duration_ms: 300, easing: 'linear' } });

    fireEvent.change(testid('canvas-panel-tween-duration'), { target: { value: '' } });
    expect(lastPatch(spy)).toEqual({ tween: undefined });
  });

  it('패널 기본 트윈이 없을 때 이징만 건드려도 트윈이 생기지 않는다', () => {
    const spy = setup(cfg([]));
    fireEvent.change(testid('canvas-panel-tween-easing'), { target: { value: 'linear' } });
    expect(lastPatch(spy)).toEqual({ tween: undefined });
  });

  it('배경색 스와치를 낸다', () => {
    setup(cfg([], { background: '#101010' }));
    expect(testid('canvas-panel-background')).toBeTruthy();
  });
});

// --- 왕복 ---------------------------------------------------------------

describe('CanvasElementsEditor — parseCanvasConfig 왕복', () => {
  /**
   * 편집기가 내보낸 config 를 파서로 읽으면 **같은 것**이 나와야 한다.
   *
   * 이 단언이 편집기와 파서가 갈라지는 순간을 잡는다. 갈라지면 사용자가 저장 버튼을
   * 한 번 누른 것만으로 화면의 값이 바뀐다 — 편집기가 파서가 죄는 범위 밖의 값을
   * 내보낼 때 실제로 그렇게 된다(음수 두께, 1 넘는 불투명도, 소수 자리의 소수점).
   *
   * 편집은 반드시 **누적 하네스**(`setupStateful`) 위에서 한다. 스파이만 쓰면 편집기가
   * 제어 컴포넌트라 매 편집이 원본에서 다시 계산되어, 연속 편집 중 마지막 하나만
   * 검사된다 — 그러면 이 단언이 조용히 힘을 잃는다.
   */
  function expectRoundTrip(config: Record<string, unknown>): CanvasPanelConfig {
    const parsed = parseCanvasConfig(config);
    expect(parsed.elements).toEqual(config.elements);
    return parsed;
  }

  it('요소 편집 결과가 파싱 왕복에서 그대로 남는다', () => {
    const live = setupStateful(
      cfg([
        rect({
          id: 'r1',
          style: { fill: '#ff0000', strokeWidth: 2, opacity: 0.5, fontWeight: 'bold', visible: true },
          text: '{value}{unit}',
          decimals: 1,
          unit: '°C',
          binding: { series: TEMP_ID, agg: 'last' },
          rules: [
            { op: 'gt', value: 80, patch: { fill: '#ffaa00' } },
            { op: 'between', value: [10, 20], patch: { text: '정상' } },
            { op: 'nodata', value: 0, patch: { visible: false } },
          ],
          tween: { duration_ms: 250, easing: 'ease-in' },
        }),
        { id: 'l1', kind: 'line', geometry: { x1: -0.2, y1: 0.5, x2: 1.4, y2: 0.5 }, style: {} },
        { id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: { align: 'center' }, text: 'hi' },
      ]),
    );

    // 픽스처를 그대로 파싱하면 파서만 검사한다 — 실제로 편집해 편집기가 만든 배열을 본다.
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '0.15' } });
    fireEvent.change(testid('canvas-element-unit-0'), { target: { value: 'kPa' } });
    fireEvent.change(testid('canvas-element-align-2'), { target: { value: 'right' } });
    expectRoundTrip(live.config);
  });

  it('죄인 극단값이 누적된 뒤에도 왕복에서 바뀌지 않는다', () => {
    const live = setupStateful(cfg([rect()]));

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '9' } });
    fireEvent.change(testid('canvas-element-stroke-width-0'), { target: { value: '-3' } });
    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '-8' } });
    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '2.9' } });
    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '3.5' } });
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '-40' } });

    // 편집이 실제로 누적됐는지 먼저 확인한다 — 하나만 남았다면 아래 왕복 단언은 허수다.
    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el.style).toEqual({ opacity: 1, strokeWidth: 0, fontSize: 0 });
    expect(el.decimals).toBe(2);
    expect(el.tween).toEqual({ duration_ms: 0, easing: 'ease-out' });

    expectRoundTrip(live.config);
  });

  it('패널 축(배경 · 기본 트윈)도 왕복에서 바뀌지 않는다', () => {
    const live = setupStateful(cfg([], { background: '#0a0a0a' }));
    fireEvent.change(testid('canvas-panel-tween-duration'), { target: { value: '450' } });
    fireEvent.change(testid('canvas-panel-tween-easing'), { target: { value: 'ease-in-out' } });

    const parsed = expectRoundTrip(live.config);
    expect(parsed.tween).toEqual({ duration_ms: 450, easing: 'ease-in-out' });
    expect(parsed.background).toBe('#0a0a0a');
  });

  it('신규 요소를 종류마다 하나씩 쌓아도 왕복에서 그대로다', () => {
    const live = setupStateful(cfg([]));
    fireEvent.click(testid('canvas-element-add-rect'));
    fireEvent.click(testid('canvas-element-add-ellipse'));
    fireEvent.click(testid('canvas-element-add-line'));
    fireEvent.click(testid('canvas-element-add-text'));

    expect((live.config.elements as CanvasElement[]).map((e) => e.kind)).toEqual([
      'rect',
      'ellipse',
      'line',
      'text',
    ]);
    expectRoundTrip(live.config);
  });

  it('종류를 바꾸고 규칙까지 붙인 요소도 왕복에서 그대로다', () => {
    const live = setupStateful(cfg([rect({ binding: { series: TEMP_ID, agg: 'last' } })]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{name} {value}{unit}' } });
    fireEvent.click(testid('canvas-rule-add'));
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'between' } });
    fireEvent.change(testid('canvas-rule-value-high-0'), { target: { value: '42' } });

    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el.kind).toBe('text');
    expect(el.rules).toEqual([{ op: 'between', value: [0, 42], patch: {} }]);
    expectRoundTrip(live.config);
  });
});
