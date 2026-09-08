// 캔버스 요소 목록 편집기 테스트 (SPEC-CANVAS-001 T8).
//
// 이 편집기가 지는 약속은 넷이고, 테스트의 무게중심도 거기에 둔다.
//   1. 목록 조작(추가·삭제·순서)이 배열 순서 = z-order 를 그대로 바꾼다.
//   2. 기하 칸이 **종류가 요구하는 축만** 낸다(rect/ellipse 는 x·y·w·h, line 은 두 끝점,
//      text 는 기준점).
//   3. 종류를 바꿔도 스타일·문구·바인딩·규칙이 살아남고 기하만 새 형상으로 옮겨진다.
//   4. 여기서 내보낸 config 를 `parseCanvasConfig` 로 읽으면 **같은 것**이 나온다.
//      가장 센 보증이다 — 편집기와 파서가 갈라지는 순간을 잡는다.
//   5. **여기서 만든 요소는 렌더 층이 실제로 칠한다.** 4번이 편집기↔파서 이음매를 지키듯
//      이것은 편집기↔렌더 이음매를 지킨다. 두 층이 각자 옳으면서 사이가 빈 적이 있다:
//      편집기는 `style: {}` 로 만들었고 렌더 층은 색 없는 요소를 (의도대로) 건너뛰어,
//      "사각형" 을 눌러도 캔버스가 빈 채였다. 어느 쪽 단위 테스트도 이를 볼 수 없었으므로
//      이 파일에서 `drawElement` 를 직접 불러 이음매를 건넌다.
//      **같은 이음매가 두 번 비었다.** 두 번째는 요소를 만드는 경로가 아니라 **문구를
//      적는** 경로였다: 도형의 문구 칸에 `{value}` 를 적어도 아무도 `textColor` 를 심지
//      않아 라벨이 그려지지 않았다(도형 라벨은 `fill` 로 폴백하지 않는다 — 폴백하면 라벨이
//      제 도형과 같은 색이 되므로 그 거절은 옳다). 그래서 이제 이 파일은 "추가 버튼을
//      눌렀다" 뿐 아니라 **"문구를 타이핑했다"** 도 렌더 층까지 끌고 간다.
//   6. 한 요소의 세부 여섯 줄은 **접힌다.** 순번 · 종류 · 순서 · 삭제만 늘 보인다.
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

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { useState } from 'react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { StoreSourceConfig } from '../charts/chartChannelTypes';
import type { CanvasElement, CanvasElementKind, CanvasPanelConfig } from './canvasConfig';
import { parseCanvasConfig } from './canvasConfig';
import type { StageSize } from './canvasGeometry';
import { renderTextTemplate } from './canvasText';
import { drawElements, type DrawContext2D } from './drawElement';
import { SEED_COLOR, SEED_STROKE_WIDTH, SEED_TEXT_COLOR } from './canvasElementFactory';
import CanvasElementsEditor from './CanvasElementsEditor';
import {
  CanvasEditSelectionContext,
  CanvasLiveSeriesContext,
  useCanvasEditSelectionState,
  type CanvasLiveSeriesValue,
  type CanvasSeriesOption,
} from './canvasEditContext';

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

/**
 * 렌더된 모든 요소 줄을 펼친다.
 *
 * 세부 여섯 줄은 이제 기본이 접힘이라, 그 줄들을 보는 시험은 먼저 펼쳐야 한다. 펼치기는
 * 각 시험의 관심사가 아니라 **전제**이므로 여기 한 곳에 둔다 — 시험 본문마다 클릭을
 * 흩뿌리면 무엇을 시험하는 파일인지가 흐려진다.
 */
function expandAllRows(): void {
  for (const btn of screen.queryAllByTestId(/^canvas-element-toggle-\d+$/)) {
    fireEvent.click(btn);
  }
}

/**
 * 편집기를 그리고 onConfigChange 스파이를 돌려준다.
 *
 * 기본으로 모든 줄을 펼친다. 접힘 그 자체를 보는 시험만 `{ expand: false }` 로 끈다.
 */
function setup(config: Record<string, unknown>, opts: { expand?: boolean } = {}) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  if (opts.expand !== false) expandAllRows();
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
  expandAllRows();
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

// --- 결함 C(사용 시험): 고를 것이 없으면 어디로 가야 하는지 말한다 --------
//
// "바인딩 없음" 하나만 든 상자는 사용자에게 **아무것도 설명하지 않는다.** 목록이 비는
// 정상적인 이유는 하나뿐이므로(데이터 소스에 시리즈가 아직 없다) 그 한 문장을 말한다.
// 히트맵의 `heatmapNoSensors` 가 같은 자리에서 같은 일을 하는 이 저장소의 관용구다.

describe('바인딩 선택지가 비면 데이터 소스로 안내한다', () => {
  it('시리즈가 하나도 없으면 안내가 뜬다', () => {
    setup({ data_source: 'store', elements: [rect()] });

    expect(testid('canvas-element-binding-hint-0').textContent).toBe(
      'dashboard.canvas.elements.bindingNoSeries',
    );
  });

  it('tsdb · sysmetrics 소스가 비어 있을 때도 같은 안내가 뜬다', () => {
    setup({ data_source: 'tsdb', elements: [rect()] });
    expect(screen.getByTestId('canvas-element-binding-hint-0')).toBeTruthy();

    cleanup();
    setup({ data_source: 'sysmetrics', elements: [rect()] });
    expect(screen.getByTestId('canvas-element-binding-hint-0')).toBeTruthy();
  });

  it('시리즈가 있으면 안내는 나오지 않는다 — 고를 것이 있는데 하는 잔소리는 잡음이다', () => {
    setup(cfg([rect()]));

    expect(screen.queryByTestId('canvas-element-binding-hint-0')).toBeNull();
  });

  it('목록이 비어도 저장된 바인딩이 되살아나 있으면 안내하지 않는다', () => {
    // 소스를 잠시 바꿨다 되돌리는 편집 도중의 자리다. 되살린 항목이 곧 고를 것이므로
    // "설정하세요" 는 그 자리에서 거짓말이 된다(`bindingOptionsFor` 계약).
    setup({
      data_source: 'store',
      elements: [rect({ binding: { series: 'temp value room=A', agg: 'last' } })],
    });

    expect(screen.queryByTestId('canvas-element-binding-hint-0')).toBeNull();
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

// --- 편집기 ↔ 렌더 이음매 -------------------------------------------------

/**
 * 칠하기 호출만 기록하는 최소 2D context.
 *
 * `drawElement.test.ts` 의 기록 스텁과 같은 수법이되(구조 인터페이스 `DrawContext2D` 를
 * 만족하는 가짜를 넘긴다) 여기서 물을 것은 **"무엇이든 칠해졌는가"** 하나뿐이라 경로·
 * 인자까지 받아 적지 않는다. 그 검사는 저쪽 파일의 몫이다.
 */
interface PaintRecorder extends DrawContext2D {
  readonly painted: string[];
}

function makePaintRecorder(): PaintRecorder {
  const painted: string[] = [];
  return {
    painted,
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {},
    rect() {},
    ellipse() {},
    moveTo() {},
    lineTo() {},
    stroke() {
      painted.push('stroke');
    },
    fill() {
      painted.push('fill');
    },
    fillText(text: string) {
      painted.push(`fillText:${text}`);
    },
    measureText(text: string) {
      return { width: text.length * 8 };
    },
    clearRect() {},
    fillRect() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'middle',
  };
}

/** 200×100 스테이지(`drawElement.test.ts` 와 같은 치수 — 정규화 좌표가 정수 px 로 떨어진다). */
const STAGE: StageSize = { width: 200, height: 100 };

/** 추가 버튼을 눌러 **편집기가 실제로 만든** 요소를 꺼낸다. */
function addedElement(kind: CanvasElementKind): CanvasElement {
  const spy = setup(cfg([]));
  fireEvent.click(testid(`canvas-element-add-${kind}`));
  const el = lastElements(spy)[0]!;
  cleanup();
  return el;
}

/** 라벨이 붙을 수 있는 도형 3종. `kind:'text'` 는 색 규칙이 달라 여기 들지 않는다. */
const SHAPE_KINDS = ['rect', 'ellipse', 'line'] as const;

/**
 * 씨앗이 심는 것과 **같은 스타일**을 입은 도형 하나. 글자색은 없다 — 사용자가 도형을
 * 더한 직후의 상태가 정확히 이것이고, 결함이 살던 자리도 여기다.
 */
function shapeEl(
  kind: (typeof SHAPE_KINDS)[number],
  style?: CanvasElement['style'],
): CanvasElement {
  return kind === 'line'
    ? {
        id: 's1',
        kind: 'line',
        geometry: { x1: 0.1, y1: 0.5, x2: 0.9, y2: 0.5 },
        style: style ?? { stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH },
      }
    : {
        id: 's1',
        kind,
        geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 },
        style: style ?? { fill: SEED_COLOR },
      };
}

/** 문구 칸에 **실제로 타이핑해** 편집기가 내보낸 요소를 꺼낸다. */
function typedLabel(
  kind: (typeof SHAPE_KINDS)[number],
  text: string,
  style?: CanvasElement['style'],
): CanvasElement {
  const spy = setup(cfg([shapeEl(kind, style)]));
  fireEvent.change(testid('canvas-element-text-0'), { target: { value: text } });
  const el = lastElements(spy)[0]!;
  cleanup();
  return el;
}

/** 요소 하나를 그리고 기록된 칠하기 호출을 돌려준다. */
function paintCallsFor(el: CanvasElement, text?: string): string[] {
  const ctx = makePaintRecorder();
  // 스타일·문구 맵을 비워 넘기면 `drawElements` 는 요소 자신의 값으로 떨어진다 —
  // 바인딩도 규칙도 없는 갓 만든 요소가 실제로 지나는 경로가 그것이다.
  drawElements(ctx, [el], {}, text === undefined ? {} : { [el.id]: text }, STAGE);
  return ctx.painted;
}

describe('CanvasElementsEditor — 만든 요소는 실제로 칠해진다', () => {
  /**
   * 이 파일이 `drawElement` 를 부르는 것은 층 경계를 넘는 일이며, **그것이 요점이다.**
   * 편집기와 렌더 층은 각자 100% 덮여 있으면서도 사이가 비어 있었다: 편집기는 색 없는
   * 요소를 만들었고, 렌더 층은 색 없는 요소를 (설계대로) 건너뛰었다. 두 계약 모두 옳은데
   * 화면만 비었다. 그 빈자리를 재는 자는 이 시험뿐이다.
   */
  it('네 종류 모두 한 번 이상 칠한다 — 추가했는데 빈 캔버스가 나오지 않는다', () => {
    for (const kind of ['rect', 'ellipse', 'line', 'text'] as const) {
      const painted = paintCallsFor(addedElement(kind));
      expect(painted.length, `${kind} 는 아무것도 칠하지 않았다`).toBeGreaterThan(0);
    }
  });

  it('종류마다 그 종류를 보이게 하는 칠하기가 실제로 일어난다', () => {
    expect(paintCallsFor(addedElement('rect'))).toContain('fill');
    expect(paintCallsFor(addedElement('ellipse'))).toContain('fill');
    // 선은 열린 경로라 채움이 뜻이 없다 — 색만으로도 부족하고 두께가 함께 있어야 그어진다.
    expect(paintCallsFor(addedElement('line'))).toEqual(['stroke']);
    expect(paintCallsFor(addedElement('text')).some((c) => c.startsWith('fillText:'))).toBe(true);
  });

  it('문구 요소는 바인딩이 없을 때의 치환 결과로도 칠해진다', () => {
    // `CanvasPanel.buildCanvasFrame` 이 지나는 길이다 — 바인딩이 없으면 `{value}` 가
    // 결측 표기로, `{name}`·`{unit}` 이 빈 문자열로 접힌다. 접힌 뒤에도 그릴 글자가
    // 남아야 "문구 추가" 가 화면에 무언가를 낸다.
    const el = addedElement('text');
    const resolved = renderTextTemplate(el.text ?? '', {});
    expect(resolved.trim()).not.toBe('');
    expect(paintCallsFor(el, resolved).some((c) => c.startsWith('fillText:'))).toBe(true);
  });

  it('심어 둔 색은 2D context 가 받을 수 있는 실제 색 문자열이다', () => {
    // CSS 변수(`var(--...)`)는 canvas 2D 가 해석하지 못한다 — 히트맵 프리셋과 같은 이유로
    // hex 리터럴로 못박는다.
    const hex = /^#[0-9a-f]{6}$/i;
    expect(addedElement('rect').style.fill).toMatch(hex);
    expect(addedElement('ellipse').style.fill).toMatch(hex);
    expect(addedElement('line').style.stroke).toMatch(hex);
    expect(addedElement('line').style.strokeWidth).toBeGreaterThan(0);
    expect(addedElement('text').style.textColor).toMatch(hex);
  });

  it('심어 둔 문구는 토큰 3종을 그대로 보여 문구 칸이 곧 사용법이 된다', () => {
    const text = addedElement('text').text ?? '';
    expect(text).toContain('{value}');
    expect(text).toContain('{name}');
    expect(text).toContain('{unit}');
  });

  /**
   * 같은 이음매의 두 번째 구멍이다. 앞의 시험들은 **추가 버튼**이 낸 요소가 칠해지는지만
   * 재고 있었고, 그 사이로 이것이 빠져나갔다: 도형에 **문구를 적는** 경로에는 아무도
   * 색을 심지 않아, 사용자가 시리즈를 묶고 `{value}` 를 적어도 화면에는 아무것도 나오지
   * 않았다(라벨은 `textColor` 로만 칠해지고 `fill` 로 폴백하지 않는다 — 폴백하면 라벨이
   * 제 도형과 같은 색이 되므로 그 거절은 옳다).
   *
   * 그래서 여기서는 **실제 편집기 UI 에 문구를 타이핑해** 나온 요소를 그대로 `drawElements`
   * 에 넣는다. 편집기만 보면 "값이 잘 실렸다" 로, 렌더 층만 보면 "색 없는 라벨은 안 그린다"
   * 로 각각 통과하므로, 두 층을 잇지 않고서는 이 결함을 볼 수 없다.
   */
  it('도형에 문구를 적으면 렌더 층이 그 글자를 실제로 칠한다', () => {
    for (const kind of SHAPE_KINDS) {
      const el = typedLabel(kind, '{value}');
      // 바인딩이 없는 갓 적은 라벨이 실제로 지나는 길이다(`CanvasPanel.buildCanvasFrame`).
      const painted = paintCallsFor(el, renderTextTemplate(el.text ?? '', {}));
      expect(
        painted.some((c) => c.startsWith('fillText:')),
        `${kind} 에 문구를 적었는데 라벨이 칠해지지 않았다`,
      ).toBe(true);
    }
  });

  it('그 글자색은 config 에 실려 편집기의 글자색 칸에도 뜬다', () => {
    // 렌더 층이 색을 지어내는 대신 저술 시점에 심는 이유다 — 지어낸 색은 화면에만 있어
    // 사용자가 갈아입힐 수 없다.
    for (const kind of SHAPE_KINDS) {
      expect(typedLabel(kind, '{value}').style.textColor, `${kind}`).toBe(SEED_TEXT_COLOR);
    }
  });

  it('사용자가 고른 글자색은 문구를 적어도 덮이지 않는다', () => {
    const el = typedLabel('rect', '{value}', { fill: SEED_COLOR, textColor: '#ff0000' });
    expect(el.style.textColor).toBe('#ff0000');
  });

  it('문구를 지워도 그때 심긴 색은 남는다 — 다시 적을 때 색을 잃지 않는다', () => {
    const live = setupStateful(cfg([shapeEl('rect')]));
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{value}' } });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '' } });

    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el.text).toBeUndefined();
    expect(el.style.textColor).toBe(SEED_TEXT_COLOR);
  });

  it("kind:'text' 는 종전 그대로다 — 문구 요소는 textColor ?? fill 로 칠해진다", () => {
    const spy = setup(
      cfg([{ id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{value}' } });

    const el = lastElements(spy)[0]!;
    expect(el.text).toBe('{value}');
    expect(el.style.textColor).toBeUndefined();
  });
});

// --- 신규 요소의 자리 ----------------------------------------------------

describe('CanvasElementsEditor — 신규 요소는 겹치지 않는다', () => {
  /** 스테이지를 벗어났는지 본다(0..1 밖은 일부라도 화면 밖이다). */
  function onStage(g: Record<string, number>): boolean {
    return Object.values(g).every((v) => v >= 0 && v <= 1);
  }

  it('같은 종류를 세 번 더하면 세 자리가 모두 다르다', () => {
    const live = setupStateful(cfg([]));
    fireEvent.click(testid('canvas-element-add-rect'));
    fireEvent.click(testid('canvas-element-add-rect'));
    fireEvent.click(testid('canvas-element-add-rect'));

    const geos = (live.config.elements as CanvasElement[]).map((e) => JSON.stringify(e.geometry));
    expect(new Set(geos).size).toBe(3);
    expect(geos[0]).toBe(JSON.stringify({ x: 0.1, y: 0.1, w: 0.2, h: 0.2 }));
    expect(geos[1]).toBe(JSON.stringify({ x: 0.15, y: 0.15, w: 0.2, h: 0.2 }));
    expect(geos[2]).toBe(JSON.stringify({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 }));
  });

  it('선과 문구는 여유가 있는 세로 축으로만 내려온다', () => {
    const live = setupStateful(cfg([]));
    fireEvent.click(testid('canvas-element-add-line'));
    fireEvent.click(testid('canvas-element-add-line'));
    fireEvent.click(testid('canvas-element-add-text'));
    fireEvent.click(testid('canvas-element-add-text'));

    const els = live.config.elements as CanvasElement[];
    // 가로는 이미 스테이지를 가로지르므로 건드리지 않는다.
    expect(els[0]!.geometry).toEqual({ x1: 0.1, y1: 0.5, x2: 0.9, y2: 0.5 });
    expect(els[1]!.geometry).toEqual({ x1: 0.1, y1: 0.55, x2: 0.9, y2: 0.55 });
    // 문구는 기준점에서 오른쪽으로 흐르므로 가로를 밀면 글자가 밖으로 나간다.
    expect(els[2]!.geometry).toEqual({ x: 0.5, y: 0.6 });
    expect(els[3]!.geometry).toEqual({ x: 0.5, y: 0.65 });
  });

  it('계단은 되감긴다 — 아홉 번을 더해도 스테이지 밖으로 행진하지 않는다', () => {
    const live = setupStateful(cfg([]));
    for (let i = 0; i < 9; i++) fireEvent.click(testid('canvas-element-add-rect'));

    const els = live.config.elements as CanvasElement[];
    expect(els).toHaveLength(9);
    for (const el of els) {
      expect(onStage(el.geometry as unknown as Record<string, number>)).toBe(true);
    }
    // 아홉 번째는 첫 번째 자리로 되감긴다(되감기 폭 8).
    expect(els[8]!.geometry).toEqual(els[0]!.geometry);
  });

  it('계단이 붙어도 좌표에 부동소수 찌꺼기가 남지 않는다', () => {
    const live = setupStateful(cfg([]));
    for (let i = 0; i < 4; i++) fireEvent.click(testid('canvas-element-add-rect'));

    // 0.1 + 0.15 를 그대로 두면 0.25000000000000006 이 숫자 칸에 그대로 뜬다.
    const shown = (live.config.elements as CanvasElement[]).map(
      (e) => (e.geometry as { x: number }).x,
    );
    expect(shown).toEqual([0.1, 0.15, 0.2, 0.25]);
  });

  it('기존 요소의 좌표는 건드리지 않는다 — 스테이지 밖 저술은 합법이다', () => {
    const live = setupStateful(
      cfg([{ id: 'far', kind: 'rect', geometry: { x: -0.5, y: 2, w: 3, h: 4 }, style: {} }]),
    );
    fireEvent.click(testid('canvas-element-add-rect'));

    const els = live.config.elements as CanvasElement[];
    expect(els[0]!.geometry).toEqual({ x: -0.5, y: 2, w: 3, h: 4 });
  });
});

// --- 접기 ---------------------------------------------------------------

describe('CanvasElementsEditor — 요소 줄 접기', () => {
  it('세부 여섯 줄은 펼치기 전에는 그려지지 않는다', () => {
    setup(cfg([rect()]), { expand: false });

    expect(screen.queryByTestId('canvas-element-geo-x-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-fill-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-text-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-binding-0')).toBeNull();
    expect(screen.queryByTestId('canvas-rule-add')).toBeNull();
  });

  it('머리줄을 누르면 펼쳐지고 다시 누르면 접힌다', () => {
    setup(cfg([rect()]), { expand: false });
    const toggle = testid('canvas-element-toggle-0');

    expect(toggle.getAttribute('aria-expanded')).toBe('false');

    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeTruthy();

    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByTestId('canvas-element-geo-x-0')).toBeNull();
  });

  it('접힌 줄에서도 순번 · 종류 · 순서 이동 · 삭제는 그대로 닿는다', () => {
    const spy = setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]), { expand: false });

    expect(testid('canvas-element-order-0').textContent).toBe('1');
    expect((testid('canvas-element-move-down-0') as HTMLButtonElement).disabled).toBe(false);

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });
    expect(lastElements(spy)[0]!.kind).toBe('ellipse');

    fireEvent.click(testid('canvas-element-move-down-0'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['b', 'a']);

    fireEvent.click(testid('canvas-element-delete-1'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['a']);
  });

  it('갓 더한 요소는 펼쳐진 채로 붙는다 — 눌렀는데 아무 일도 없어 보이면 안 된다', () => {
    setupStateful(cfg([rect({ id: 'old' })]));
    // 하네스가 기존 줄을 펼쳤으므로 먼저 되접어 새 줄만 남긴다.
    fireEvent.click(testid('canvas-element-toggle-0'));
    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('false');

    fireEvent.click(testid('canvas-element-add-rect'));

    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('false');
    expect(testid('canvas-element-toggle-1').getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByTestId('canvas-element-geo-x-1')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-geo-x-0')).toBeNull();
  });

  it('펼침은 순번이 아니라 요소를 따라간다 — 순서를 바꿔도 열린 줄은 그 요소다', () => {
    setupStateful(cfg([rect({ id: 'a' }), rect({ id: 'b' })]));
    fireEvent.click(testid('canvas-element-toggle-1')); // b 만 접는다

    fireEvent.click(testid('canvas-element-move-down-0')); // a 를 뒤로 → [b, a]

    expect(testid('canvas-element-0').getAttribute('data-element-id')).toBe('b');
    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('false');
    expect(testid('canvas-element-toggle-1').getAttribute('aria-expanded')).toBe('true');
  });

  it('접힌 줄은 무엇이 접혀 있는지 한 줄로 알린다', () => {
    setup(
      cfg([
        rect({ id: 'plain' }),
        rect({
          id: 'live',
          binding: { series: TEMP_ID, agg: 'last' },
          rules: [
            { op: 'gt', value: 1, patch: {} },
            { op: 'lt', value: 0, patch: {} },
          ],
        }),
      ]),
      { expand: false },
    );

    expect(testid('canvas-element-toggle-0').textContent).toBe(
      'plain · dashboard.canvas.elements.summaryStatic',
    );
    // 스텁 t 는 키를 그대로 돌려주므로 `{count}` 자리가 채워지지 않는다(머리말의 `{index}`
    // 와 같은 사정이다). 여기서 볼 것은 "규칙이 있는 줄만 규칙 칸을 낸다" 는 갈래다.
    expect(testid('canvas-element-toggle-1').textContent).toBe(
      'live · dashboard.canvas.elements.summaryBound · dashboard.canvas.elements.summaryRules',
    );
  });

  it('펼쳐도 같은 요약이 남는다 — 머리줄 폭이 바뀌면 옆 버튼이 손 밑에서 움직인다', () => {
    setup(cfg([rect({ id: 'plain' })]));
    expect(testid('canvas-element-toggle-0').textContent).toBe(
      'plain · dashboard.canvas.elements.summaryStatic',
    );
  });
});


// --- 캔버스 선택 → 속성 편집 연동 (SPEC-CANVAS-002 T10 · AC-06) ------------

/**
 * 캔버스 선택을 손으로 조종할 수 있는 하네스.
 *
 * 실제로는 오버레이가 `setSelection` 을 부르지만, 여기서 재려는 것은 **목록 편집기가 그
 * 선택에 어떻게 반응하는가** 하나다. 오버레이까지 끌고 오면 히트 테스트·스테이지 크기가
 * 시험의 전제가 되어 무엇이 깨졌는지 알기 어려워진다(오버레이 쪽 계약은
 * `CanvasEditOverlay.test.tsx` 가 이미 잰다).
 */
function SelectionHarness({ config }: { config: Record<string, unknown> }) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <button
        type="button"
        data-testid="pick-a"
        onClick={() => state.setSelection(new Set(['a']))}
      />
      <button
        type="button"
        data-testid="pick-b"
        onClick={() => state.setSelection(new Set(['b']))}
      />
      <button
        type="button"
        data-testid="pick-ab"
        onClick={() => state.setSelection(new Set(['a', 'b']))}
      />
      <button type="button" data-testid="pick-none" onClick={() => state.setSelection(new Set())} />
      <CanvasElementsEditor config={config} onConfigChange={() => {}} />
    </CanvasEditSelectionContext>
  );
}

/** 행이 펼쳐져 있는가 — 토글의 `aria-expanded` 가 곧 그 사실이다. */
function isRowOpen(idx: number): boolean {
  return testid(`canvas-element-toggle-${idx}`).getAttribute('aria-expanded') === 'true';
}

/** 세 요소(a · b · c)를 그린다. 선택 대상 둘과, 사용자가 손으로 펼칠 하나. */
function renderThree(): void {
  render(<SelectionHarness config={cfg([rect({ id: 'a' }), rect({ id: 'b' }), rect({ id: 'c' })])} />);
}

describe('CanvasElementsEditor — 캔버스 선택이 목록을 조종한다 (AC-06)', () => {
  it('요소 하나가 선택되면 그 행만 펼쳐진다', () => {
    renderThree();
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, false, false]);

    fireEvent.click(testid('pick-a'));
    expect([0, 1, 2].map(isRowOpen)).toEqual([true, false, false]);
  });

  it('그 행을 시야로 스크롤한다', () => {
    // `scrollIntoView` 는 jsdom 에 없다 — 실제 브라우저와 같은 이름으로 심어 본다.
    const scrollIntoView = vi.fn();
    Element.prototype.scrollIntoView = scrollIntoView as unknown as Element['scrollIntoView'];
    try {
      renderThree();
      fireEvent.click(testid('pick-b'));

      expect(scrollIntoView).toHaveBeenCalledTimes(1);
      // 스크롤된 것은 **그 요소의 행**이다.
      expect(scrollIntoView.mock.instances[0]).toBe(testid('canvas-element-1'));
    } finally {
      delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
    }
  });

  it('캔버스가 펼친 행은 **하나뿐**이다 — 다음 선택 때 이전 것이 도로 접힌다', () => {
    renderThree();
    fireEvent.click(testid('pick-a'));
    fireEvent.click(testid('pick-b'));
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, true, false]);
  });

  it('사용자가 손으로 펼친 행은 캔버스가 접지 않는다', () => {
    renderThree();
    // c 를 손으로 펼쳐 둔다.
    fireEvent.click(testid('canvas-element-toggle-2'));
    expect(isRowOpen(2)).toBe(true);

    fireEvent.click(testid('pick-a'));
    fireEvent.click(testid('pick-b'));

    // 캔버스가 펼친 것은 갈렸지만 손으로 펼친 c 는 그대로다.
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, true, true]);
  });

  it('둘 이상 선택되면 **아무 행도** 자동으로 펼쳐지지 않는다', () => {
    // 이 규칙이 없으면 다중 선택 한 번에 화면이 다시 "설정이 모두 펼쳐진" 상태가 된다.
    renderThree();
    fireEvent.click(testid('pick-a'));
    expect(isRowOpen(0)).toBe(true);

    fireEvent.click(testid('pick-ab'));
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, false, false]);
  });

  it('선택이 비면 캔버스가 펼친 행도 접힌다', () => {
    renderThree();
    fireEvent.click(testid('pick-a'));
    fireEvent.click(testid('pick-none'));
    expect(isRowOpen(0)).toBe(false);
  });

  it('둘 이상 선택되면 표시만 남는다', () => {
    renderThree();
    fireEvent.click(testid('pick-ab'));
    expect(testid('canvas-element-0').getAttribute('data-selected')).toBe('true');
    expect(testid('canvas-element-1').getAttribute('data-selected')).toBe('true');
    expect(testid('canvas-element-2').getAttribute('data-selected')).toBeNull();
  });

  it('캔버스가 펼친 행도 손으로 접을 수 있다', () => {
    // 접을 수 없으면 사용자는 되돌릴 방법이 없는 상태를 손에 쥔다.
    renderThree();
    fireEvent.click(testid('pick-a'));
    expect(isRowOpen(0)).toBe(true);

    fireEvent.click(testid('canvas-element-toggle-0'));
    expect(isRowOpen(0)).toBe(false);
  });

  it('캔버스가 펼친 행을 손으로 접어도 다른 행의 자동 펼침은 그대로 동작한다', () => {
    renderThree();
    fireEvent.click(testid('pick-a'));
    fireEvent.click(testid('canvas-element-toggle-0'));
    fireEvent.click(testid('pick-b'));
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, true, false]);
  });

  it('provider 가 없어도 동작한다 — 대시보드에 놓인 패널 곁에는 목록 편집기가 없다', () => {
    // 컨텍스트 기본값이 `null` 이라 소비 훅이 로컬 선택으로 떨어지고, 자동 펼침은
    // 언제나 없는 것이 된다. 접고 펴기는 종전대로다.
    setup(cfg([rect({ id: 'a' })]), { expand: false });
    expect(isRowOpen(0)).toBe(false);
    expect(testid('canvas-element-0').getAttribute('data-selected')).toBeNull();

    fireEvent.click(testid('canvas-element-toggle-0'));
    expect(isRowOpen(0)).toBe(true);
  });

  it('목록 하단의 추가 버튼 4개는 그대로 남는다 — 팔레트가 대체하지 않는다', () => {
    // 포인터를 쓰지 않는 경로이자 이미 테스트에 묶인 표면이다(REQ-01 · AC-05).
    setup(cfg([]), { expand: false });
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.getByTestId(`canvas-element-add-${kind}`)).toBeTruthy();
    }
  });
});

// --- 드래그가 유일한 수단이 아니다 (SPEC-CANVAS-002 T15 · REQ-01 · 위험 R10) ----
//
// 002 는 캔버스 위 드래그를 들여왔다. 그 자체는 이득이지만, **수치 입력을 밀어내면**
// 포인터를 쓰지 못하는 사용자가 패널을 저술할 수 없게 되어 002 는 001 보다 나빠진다.
// 그래서 REQ-01(항상)과 REQ-05(금지)가 같은 것을 양쪽에서 못박았고, 여기서는 그것을
// **가정하지 않고 잰다** — 캔버스가 요소를 골라 둔 상태에서도 칸이 그대로 있고, 잠기지
// 않았으며, 실제로 값을 쓸 수 있는지까지 본다.
//
// 스테이지 밖으로 전부 나간 요소의 회수 경로가 이 칸이기도 하다(AC-E5).

/** 선택을 손으로 조종하면서 **패치도 받아 보는** 하네스. */
function SelectionSpyHarness({
  config,
  onConfigChange,
  pick,
}: {
  config: Record<string, unknown>;
  onConfigChange: (patch: Record<string, unknown>) => void;
  pick: string;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <button
        type="button"
        data-testid="pick"
        onClick={() => state.setSelection(new Set([pick]))}
      />
      <CanvasElementsEditor config={config} onConfigChange={onConfigChange} />
    </CanvasEditSelectionContext>
  );
}

/** 요소 하나를 캔버스에서 골라 둔 상태로 편집기를 그린다(그 행은 자동으로 펼쳐진다). */
function setupPicked(elements: CanvasElement[], pick: string) {
  const onConfigChange = vi.fn();
  render(
    <SelectionSpyHarness config={cfg(elements)} onConfigChange={onConfigChange} pick={pick} />,
  );
  fireEvent.click(testid('pick'));
  return onConfigChange;
}

describe('CanvasElementsEditor — 캔버스 선택이 수치 입력을 밀어내지 않는다 (REQ-01 · 위험 R10)', () => {
  it('사각형을 캔버스에서 골라도 x·y·w·h 칸이 그대로 있고 잠기지 않는다', () => {
    setupPicked([rect({ id: 'a' })], 'a');

    for (const axis of ['x', 'y', 'w', 'h']) {
      const input = testid(`canvas-element-geo-${axis}-0`) as HTMLInputElement;
      expect(input.tagName).toBe('INPUT');
      expect(input.disabled).toBe(false);
      expect(input.readOnly).toBe(false);
    }
  });

  it('선과 문구도 자기 축의 칸을 그대로 낸다', () => {
    setupPicked(
      [{ id: 'a', kind: 'line', geometry: { x1: 0, y1: 0, x2: 1, y2: 1 }, style: {} }],
      'a',
    );
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect((testid(`canvas-element-geo-${axis}-0`) as HTMLInputElement).disabled).toBe(false);
    }

    cleanup();
    setupPicked([{ id: 'a', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: {} }], 'a');
    for (const axis of ['x', 'y']) {
      expect((testid(`canvas-element-geo-${axis}-0`) as HTMLInputElement).disabled).toBe(false);
    }
    // 문구의 포인터 수단은 글자 크기 핸들 하나뿐이므로 그 등가물이 특히 남아 있어야 한다.
    expect((testid('canvas-element-font-size-0') as HTMLInputElement).disabled).toBe(false);
  });

  it('골라 둔 채로 타이핑하면 그대로 저술된다 — 회수 경로가 살아 있다', () => {
    const spy = setupPicked([rect({ id: 'a' })], 'a');
    spy.mockClear();

    // 스테이지 밖으로 전부 나간 요소를 되돌리는 그 조작이다(clamp 하지 않으므로 가능하다).
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '-0.5' } });

    expect(lastElements(spy)[0]!.geometry).toEqual({ x: -0.5, y: 0.2, w: 0.3, h: 0.4 });
  });

  it('추가·순서·삭제도 캔버스 선택과 무관하게 목록에 그대로 남는다', () => {
    setupPicked([rect({ id: 'a' }), rect({ id: 'b' })], 'a');

    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect((testid(`canvas-element-add-${kind}`) as HTMLButtonElement).disabled).toBe(false);
    }
    // 첫 행의 아래로 이동과 삭제는 언제나 쓸 수 있다(위로 이동만 양 끝에서 잠긴다).
    expect((testid('canvas-element-move-down-0') as HTMLButtonElement).disabled).toBe(false);
    expect((testid('canvas-element-delete-0') as HTMLButtonElement).disabled).toBe(false);
  });
});

// --- 정렬 규칙 단일화 가드 (REQ-04) ----------------------------------------

describe('순서 이동 규칙은 한 곳에만 있다 (REQ-04)', () => {
  it('목록 편집기의 순서 이동도 이 모듈의 `moveElementTo` 를 지난다 (REQ-04)', () => {
    // 001 의 z-order 수단은 배열 순서 하나뿐이므로 정렬 규칙도 하나여야 한다. 목록
    // 편집기가 인라인 splice 를 한 벌 더 들고 있던 동안에는 그 규율이 주석일 뿐이었고,
    // "목록에서 눌렀는가 캔버스에서 눌렀는가" 에 따라 결과가 갈릴 자리가 남아 있었다.
    //
    // 들여왔는지만 보면 인라인으로 되돌리고 import 를 남겨 두는 형태를 놓칠 것 같지만,
    // 그때는 쓰이지 않는 import 가 되어 eslint(`no-unused-vars`, error)가 잡는다 —
    // 둘이 짝을 이뤄야 이 가드가 실제로 문을 막는다.
    const source = readFileSync(join(__dirname, 'CanvasElementsEditor.tsx'), 'utf-8');
    expect(source).toMatch(/import \{ moveElementTo \} from '\.\/canvasEditArrange'/);
  });
});

// --- 결함 D: 살아 있는 패널이 낸 키 집합이 config 추측을 이긴다 -----------
//
// 편집기가 config 만 보고 만든 목록은 **추측**이다 — 참조 하나가 태그로 여러 컬럼으로
// 펼쳐지면 그 추측은 패널이 실제로 쓰는 키와 갈라지고, 사용자가 고른 값은 어느 판독값과도
// 만나지 못한다. 그래서 곁에 미리보기가 있으면 그쪽이 낸 것을 그대로 쓴다.
//
// 두 층이 실제로 이어지는지(고른 값이 진짜 그려지는지)는 `CanvasPanel.test.tsx` 가
// 두 컴포넌트를 함께 세워 잰다. 여기서는 편집기 쪽 계약만 잠근다.

/** 패널이 이미 키 집합을 내놓은 상태를 흉내 낸 provider 값. */
function liveValue(options: CanvasSeriesOption[]): CanvasLiveSeriesValue {
  return { options, publish: () => {} };
}

/** 라이브 채널을 물린 채 편집기를 그리고 모든 줄을 펼친다. */
function setupWithLive(
  config: Record<string, unknown>,
  options: CanvasSeriesOption[],
): ReturnType<typeof vi.fn> {
  const onConfigChange = vi.fn();
  render(
    <CanvasLiveSeriesContext value={liveValue(options)}>
      <CanvasElementsEditor config={config} onConfigChange={onConfigChange} />
    </CanvasLiveSeriesContext>,
  );
  expandAllRows();
  return onConfigChange;
}

/** 바인딩 드롭다운의 선택지 값 목록. */
function bindingValues(): string[] {
  return Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
    (o) => o.value,
  );
}

const FANOUT: CanvasSeriesOption[] = [
  { id: 'temp · value{room=A}', label: 'temp · value{room=A}' },
  { id: 'temp · value{room=B}', label: 'temp · value{room=B}' },
];

describe('CanvasElementsEditor — 살아 있는 시리즈가 config 추측을 이긴다 (결함 D)', () => {
  it('패널이 낸 키 집합이 있으면 그것만 낸다 (config 로 뽑은 동일성 키는 나오지 않는다)', () => {
    setupWithLive(cfg([rect()]), FANOUT);

    expect(bindingValues()).toEqual(['', FANOUT[0]!.id, FANOUT[1]!.id]);
    expect(bindingValues()).not.toContain(TEMP_ID);
  });

  it('고른 값은 그 키 그대로 올라간다 (집계는 여전히 last 다)', () => {
    const spy = setupWithLive(cfg([rect()]), FANOUT);

    fireEvent.change(testid('canvas-element-binding-0'), { target: { value: FANOUT[1]!.id } });

    expect(lastElements(spy)[0]!.binding).toEqual({ series: FANOUT[1]!.id, agg: 'last' });
  });

  it('표시 이름은 패널이 준 것을 그대로 쓴다 (훅이 정한 이름 하나뿐이다)', () => {
    setupWithLive(cfg([rect()]), [{ id: 'k', label: '실습실 온도' }]);

    const select = testid('canvas-element-binding-0') as HTMLSelectElement;
    expect(select.options[1]!.value).toBe('k');
    expect(select.options[1]!.textContent).toBe('실습실 온도');
  });

  it('아직 아무것도 내놓지 않았으면 config 목록으로 떨어진다 (첫 조회 전)', () => {
    setupWithLive(cfg([rect()]), []);

    expect(bindingValues()).toEqual(['', TEMP_ID, HUM_ID]);
  });

  it('provider 가 아예 없어도 config 목록으로 동작한다 (편집기 단독 렌더)', () => {
    setup(cfg([rect()]));

    expect(bindingValues()).toEqual(['', TEMP_ID, HUM_ID]);
  });

  it('저장된 바인딩이 라이브 목록에 없으면 되살린다 (되살리기 규칙은 그대로다)', () => {
    setupWithLive(cfg([rect({ binding: { series: 'ghost key ', agg: 'last' } })]), FANOUT);

    const select = testid('canvas-element-binding-0') as HTMLSelectElement;
    expect(select.value).toBe('ghost key ');
    expect(bindingValues()).toEqual(['', FANOUT[0]!.id, FANOUT[1]!.id, 'ghost key ']);
  });

  it('라이브 시리즈가 있으면 config 소스가 비어도 안내는 뜨지 않는다', () => {
    // 태그 팬아웃처럼 config 만으로는 목록을 세울 수 없는 자리다. 고를 것이 있으므로
    // "데이터 소스를 먼저 설정하라" 는 그 자리에서 거짓말이 된다.
    setupWithLive({ data_source: 'store', elements: [rect()] }, FANOUT);

    expect(screen.queryByTestId('canvas-element-binding-hint-0')).toBeNull();
  });

  it('라이브도 config 도 비면 안내가 그대로 뜬다 (AC-E11 회귀)', () => {
    setupWithLive({ data_source: 'store', elements: [rect()] }, []);

    expect(testid('canvas-element-binding-hint-0').textContent).toBe(
      'dashboard.canvas.elements.bindingNoSeries',
    );
  });
});
