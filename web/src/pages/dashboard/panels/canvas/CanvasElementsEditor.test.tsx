// 캔버스 요소 목록 편집기 테스트 (SPEC-CANVAS-001 T8).
//
// 이 편집기가 지는 약속은 넷이고, 테스트의 무게중심도 거기에 둔다.
//   1. 목록 조작(삭제·순서)이 배열 순서 = z-order 를 그대로 바꾼다. **추가는 이제 이
//      편집기의 일이 아니다** — 도형을 만드는 자리는 미리보기 왼쪽 도크의 팔레트 하나이며,
//      그래서 여기서 만드는 경로를 지나야 하는 시험(씨앗 기하 · 계단 오프셋 · 렌더 이음매)
//      은 그 팔레트를 눌러 같은 상태에 닿는다(`setupPalette`).
//   2. 기하 칸이 **종류가 요구하는 축만** 낸다(rect/ellipse 는 x·y·w·h, line 은 두 끝점,
//      text 는 기준점).
//   3. 종류를 바꿔도 스타일·문구·바인딩·규칙이 살아남고 기하만 새 형상으로 옮겨진다.
//   4. 여기서 내보낸 config 를 `parseCanvasConfig` 로 읽으면 **같은 것**이 나온다.
//      가장 센 보증이다 — 편집기와 파서가 갈라지는 순간을 잡는다.
//   5. **여기서 만든 요소는 렌더 층이 실제로 칠한다.** 4번이 편집기↔파서 이음매를 지키듯
//      이것은 편집기↔렌더 이음매를 지킨다. 두 층이 각자 옳으면서 사이가 빈 적이 있다:
//      편집기는 `style: {}` 로 만들었고 렌더 층은 색 없는 요소를 (의도대로) 건너뛰어,
//      "사각형" 을 눌러도 캔버스가 빈 채였다. 어느 쪽 단위 테스트도 이를 볼 수 없었으므로
//      이 파일에서 `drawElement` 를 직접 불러 이음매를 건넌다. 만드는 자리가 도크로 옮겨
//      간 뒤에도 **건너는 이음매는 같다** — 누르는 버튼만 팔레트로 바뀐다.
//      **같은 이음매가 두 번 비었다.** 두 번째는 요소를 만드는 경로가 아니라 **문구를
//      적는** 경로였다: 도형의 문구 칸에 `{value}` 를 적어도 아무도 `textColor` 를 심지
//      않아 라벨이 그려지지 않았다(도형 라벨은 `fill` 로 폴백하지 않는다 — 폴백하면 라벨이
//      제 도형과 같은 색이 되므로 그 거절은 옳다). 그래서 이제 이 파일은 "추가 버튼을
//      눌렀다" 뿐 아니라 **"문구를 타이핑했다"** 도 렌더 층까지 끌고 간다.
//   6. 한 요소의 세부는 **접힌다.** 순번 · 요약 · 순서 · 삭제만 늘 보인다 — 종류는
//      저술이므로 머리줄이 아니라 스타일 탭의 첫 칸에 선다.
//   7. 펼친 몸통은 **탭 넷**(스타일 · 텍스트 · 배치 · 데이터)으로 갈리고, 열린 탭의 칸만
//      DOM 에 선다. 그래서 이 파일의 시험 대부분은 무엇을 재는지에 따라 탭을 먼저 연다
//      (`setup(..., { tab })` · `openTab`). 탭은 **재배치일 뿐**이라 저술 계약은 그대로이며,
//      그 사실 자체도 시험한다: 탭을 오간 편집이 누적되는가, 열린 탭이 config 에 실리지
//      않는가.
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

import { Profiler, useEffect, useState } from 'react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { StoreSourceConfig } from '../charts/chartChannelTypes';
import type { CanvasElement, CanvasElementKind, CanvasPanelConfig } from './canvasConfig';
import { DEFAULT_CANVAS_SIZE, parseCanvasConfig } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import { renderTextTemplate } from './canvasText';
import { drawElements, type DrawContext2D } from './drawElement';
import { SEED_COLOR, SEED_STROKE_WIDTH, SEED_TEXT_COLOR } from './canvasElementFactory';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasElementsEditor from './CanvasElementsEditor';
import {
  CanvasEditSelectionContext,
  CanvasLiveSeriesContext,
  useCanvasEditSelectionState,
  type CanvasLiveSeriesValue,
  type CanvasSeriesOption,
} from './canvasEditContext';
import {
  CanvasStageAspectContext,
  useCanvasStageAspectState,
} from './canvasStageAspect';

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
    geometry: { x: 50, y: 80, w: 150, h: 160 },
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

/** 요소 카드의 탭 넷. 화면 차례 그대로다. */
type TabName = 'style' | 'text' | 'arrange' | 'data';

/**
 * 탭 하나를 연다.
 *
 * 탭 상태는 **편집기 하나에 하나**(요소마다가 아니다)이므로, 어느 카드의 탭을 눌러도
 * 펼쳐진 카드가 모두 그 탭으로 따라온다 — 그래서 인자에 순번이 없다. 기본 탭은
 * `style` 이라 스타일 칸을 보는 시험은 이 함수를 부르지 않는다.
 */
function openTab(tab: TabName, idx = 0): void {
  fireEvent.click(screen.getByTestId(`canvas-element-tab-${tab}-${idx}`));
}

/**
 * 편집기를 그리고 onConfigChange 스파이를 돌려준다.
 *
 * 기본으로 모든 줄을 펼친다. 접힘 그 자체를 보는 시험만 `{ expand: false }` 로 끈다.
 * `{ tab }` 은 펼친 뒤 그 탭을 연다 — 카드가 넷으로 갈린 뒤로 "무엇을 보고 있는가" 는
 * 시험의 관심사가 아니라 **전제**이므로, 본문마다 클릭을 흩뿌리지 않고 여기서 받는다.
 */
function setup(
  config: Record<string, unknown>,
  opts: { expand?: boolean; tab?: TabName } = {},
) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  if (opts.expand !== false) expandAllRows();
  if (opts.tab !== undefined) openTab(opts.tab);
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
function setupStateful(
  initial: Record<string, unknown>,
  opts: { tab?: TabName } = {},
): { config: Record<string, unknown> } {
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
  if (opts.tab !== undefined) openTab(opts.tab);
  return live;
}

// --- 만드는 자리는 도크 팔레트다 -----------------------------------------
//
// 목록 하단의 종류별 추가 버튼은 걷어냈다(도크 팔레트와 **같은 함수**를 부르는 입구가
// 둘일 이유가 없다). 그래서 "만드는 경로를 지나야만 닿는 상태" 를 재는 시험들 — 씨앗
// 기하·계단 오프셋·편집기↔렌더 이음매 — 은 지우는 대신 **남은 입구로 옮겨** 같은 상태에
// 닿는다. 지우면 그 상태를 아무도 보지 않게 되는데, 결함이 살던 곳이 바로 거기다.

/** 팔레트 시험이 쓰는 투영. 칠하기 시험과 같은 치수다. */
const PALETTE_PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

/**
 * 도크 팔레트(+ 선택사항으로 목록 편집기)를 세우고 **살아 있는 요소 배열**을 돌려준다.
 *
 * 오버레이가 도구를 만들지만 그리는 자리는 도크이므로 `CanvasEditDockRegion` 이 함께
 * 있어야 팔레트가 DOM 에 나온다(설정 미리보기와 같은 형상이다).
 */
function setupPalette(
  initial: readonly CanvasElement[] = [],
  opts: { withList?: boolean } = {},
): { elements: CanvasElement[] } {
  const live = { elements: [...initial] };
  function Harness() {
    const [elements, setElements] = useState<CanvasElement[]>([...initial]);
    live.elements = elements;
    const state = useCanvasEditSelectionState();
    return (
      <CanvasEditSelectionContext value={state}>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={PALETTE_PROJ}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </CanvasEditDockRegion>
        {opts.withList === true && (
          <CanvasElementsEditor
            config={cfg(elements)}
            onConfigChange={(patch) => setElements(patch.elements as CanvasElement[])}
          />
        )}
      </CanvasEditSelectionContext>
    );
  }
  render(<Harness />);
  return live;
}

/** 팔레트에서 도형 하나를 놓는다. */
function place(kind: CanvasElementKind): void {
  fireEvent.click(screen.getByTestId(`canvas-palette-add-${kind}`));
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

  it('목록에는 종류별 추가 버튼이 없다 — 만드는 자리는 도크 팔레트 하나다', () => {
    // 같은 함수를 부르는 입구가 둘이면 화면에 같은 일을 하는 자리가 둘이 된다.
    // 팔레트가 이름과 누를 면적을 갖춘 뒤로는 이 줄이 남을 이유가 없었다.
    setup(cfg([rect()]), { expand: false });
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.queryByTestId(`canvas-element-add-${kind}`)).toBeNull();
    }
  });

  it('그 팔레트는 종류의 기본 기하를 가진 요소를 끝에 붙인다 (걷어낸 줄이 하던 일)', () => {
    // 걷어낸 것은 **입구**이지 경로가 아니다. 같은 `appendElement` 를 지나므로 씨앗
    // 기하·id 규칙도 그대로여야 한다 — 그 사실을 남은 입구에서 다시 잰다.
    const live = setupPalette();

    place('rect');
    expect(live.elements).toHaveLength(1);
    expect(live.elements[0]!.kind).toBe('rect');
    expect(live.elements[0]!.geometry).toEqual({ x: 50, y: 40, w: 100, h: 80 });
    expect(live.elements[0]!.id).toBe('el-1');

    cleanup();
    const line = setupPalette();
    place('line');
    expect(line.elements[0]!.geometry).toEqual({ x1: 50, y1: 200, x2: 450, y2: 200 });

    cleanup();
    const text = setupPalette();
    place('text');
    expect(text.elements[0]!.geometry).toEqual({ x: 250, y: 200 });

    cleanup();
    const ellipse = setupPalette();
    place('ellipse');
    expect(ellipse.elements[0]!.kind).toBe('ellipse');
  });

  it('신규 id 는 이미 쓰인 id 를 피해 결정적으로 붙는다', () => {
    const live = setupPalette([rect({ id: 'el-1' }), rect({ id: 'el-2' })]);
    place('rect');
    expect(live.elements[2]!.id).toBe('el-3');
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
    setup(cfg([rect(), rect({ id: 'e1', kind: 'ellipse' })]), { tab: 'arrange' });
    for (const axis of ['x', 'y', 'w', 'h']) {
      expect(screen.getByTestId(`canvas-element-geo-${axis}-0`)).toBeTruthy();
      expect(screen.getByTestId(`canvas-element-geo-${axis}-1`)).toBeTruthy();
    }
    expect(screen.queryByTestId('canvas-element-geo-x1-0')).toBeNull();
    expect((testid('canvas-element-geo-w-0') as HTMLInputElement).value).toBe('150');
  });

  it('line 은 두 끝점만 낸다', () => {
    setup(cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0, y1: 40, x2: 500, y2: 360 }, style: {} }]), { tab: 'arrange' });
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect(screen.getByTestId(`canvas-element-geo-${axis}-0`)).toBeTruthy();
    }
    expect(screen.queryByTestId('canvas-element-geo-w-0')).toBeNull();
    expect((testid('canvas-element-geo-y2-0') as HTMLInputElement).value).toBe('360');
  });

  it('text 는 기준점만 낸다', () => {
    setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 200, y: 240 }, style: {}, text: 'hi' }]), { tab: 'arrange' });
    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-geo-y-0')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-geo-w-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-geo-x1-0')).toBeNull();
  });

  it('기하 칸은 캔버스 밖의 값도 그대로 받는다 — 걸치는 배치도 뜻이 있다', () => {
    const spy = setup(cfg([rect()]), { tab: 'arrange' });
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '-250' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: -250, y: 80, w: 150, h: 160 });

    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '1250' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 1250, h: 160 });
  });

  it('빈 기하 칸은 0 으로 떨어진다 — NaN 이 들어가면 요소가 통째로 사라진다', () => {
    const spy = setup(cfg([rect()]), { tab: 'arrange' });
    fireEvent.change(testid('canvas-element-geo-y-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 0, w: 150, h: 160 });
  });

  it('line 과 text 의 기하 칸도 같은 경로로 편집된다', () => {
    const spy = setup(
      cfg([
        { id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} },
        { id: 't1', kind: 'text', geometry: { x: 250, y: 200 }, style: {} },
      ]),
    { tab: 'arrange' },
    );
    fireEvent.change(testid('canvas-element-geo-x2-0'), { target: { value: '375' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0, y1: 0, x2: 375, y2: 400 });

    fireEvent.change(testid('canvas-element-geo-y-1'), { target: { value: '100' } });
    expect(lastElements(spy)[1]!.geometry).toEqual({ x: 250, y: 100 });
  });
});

// --- 정수 좌표계 (SPEC-CANVAS-002 0.8.0) ---------------------------------
//
// 이 절이 고정하는 것은 **저장 왕복에 값이 바뀌지 않는다**(규율 2)의 정수판이다. 파서가
// 반올림하고 퇴화를 되살리므로, 편집기가 그보다 느슨한 값을 쓰면 사용자가 적은 것과
// 다시 열었을 때 보이는 것이 달라진다.

describe('CanvasElementsEditor — 기하 칸은 정수 칸이다', () => {
  it('위치·크기 칸의 step 이 1 이다 — 화살표 한 번이 곧 한 단위다', () => {
    setup(cfg([rect()]), { tab: 'arrange' });
    for (const axis of ['x', 'y', 'w', 'h']) {
      expect(testid(`canvas-element-geo-${axis}-0`).getAttribute('step')).toBe('1');
    }
  });

  it('소수를 적으면 반올림해 저술한다 — 파서가 어차피 반올림하므로 여기서 먼저 맞춘다', () => {
    const spy = setup(cfg([rect()]), { tab: 'arrange' });
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '10.4' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 10, y: 80, w: 150, h: 160 });

    fireEvent.change(testid('canvas-element-geo-y-0'), { target: { value: '10.6' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 11, w: 150, h: 160 });
  });

  it('편집기가 내보낸 기하는 파서를 지나도 그대로다(왕복 안정성)', () => {
    const spy = setup(cfg([rect()]), { tab: 'arrange' });
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '10.4' } });
    const written = lastElements(spy)[0]!.geometry;
    const reparsed = parseCanvasConfig(cfg(lastElements(spy))).elements[0]!.geometry;
    expect(reparsed).toEqual(written);
  });

  it('크기 칸만 하한을 갖는다 — 위치는 음수도 캔버스 밖도 합법이다', () => {
    setup(cfg([rect()]), { tab: 'arrange' });
    expect(testid('canvas-element-geo-w-0').getAttribute('min')).toBe('1');
    expect(testid('canvas-element-geo-h-0').getAttribute('min')).toBe('1');
    // 없는 하한을 적어 두면 그 자리가 곧 사용자 의도를 자르는 자리가 된다.
    expect(testid('canvas-element-geo-x-0').getAttribute('min')).toBeNull();
    expect(testid('canvas-element-geo-y-0').getAttribute('min')).toBeNull();
  });

  it('크기 칸을 비우거나 0 을 적어도 최소 크기로 남는다 — 요소가 사라지지 않는다', () => {
    // 0 을 그대로 쓰면 파서가 퇴화로 보고 씨앗 기하로 되살려, 칸을 비우는 순간 요소가
    // 화면 반대편으로 순간이동한다. 그 되살림은 옛 config 를 위한 것이지 이 조작을 위한
    // 것이 아니다.
    const spy = setup(cfg([rect()]), { tab: 'arrange' });
    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 1, h: 160 });

    fireEvent.change(testid('canvas-element-geo-h-0'), { target: { value: '0' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 150, h: 1 });

    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '-40' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 1, h: 160 });
  });

  it('선의 끝점 칸도 정수 칸이다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} }]),
    { tab: 'arrange' },
    );
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect(testid(`canvas-element-geo-${axis}-0`).getAttribute('step')).toBe('1');
      // 끝점은 자리이지 크기가 아니므로 하한이 없다.
      expect(testid(`canvas-element-geo-${axis}-0`).getAttribute('min')).toBeNull();
    }
    fireEvent.change(testid('canvas-element-geo-x2-0'), { target: { value: '300.5' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0, y1: 0, x2: 301, y2: 400 });
  });
});

describe('CanvasElementsEditor — 캔버스 크기 (모든 좌표의 분모)', () => {
  it('두 칸이 파싱된 크기를 보인다', () => {
    setup(cfg([rect()]));
    expect((testid('canvas-panel-width') as HTMLInputElement).value).toBe('500');
    expect((testid('canvas-panel-height') as HTMLInputElement).value).toBe('400');
  });

  it('크기가 없는 config(001 · 002 가 쓴 전부)도 기본값을 보인다', () => {
    setup({ ...cfg([rect()]) });
    expect((testid('canvas-panel-width') as HTMLInputElement).value).toBe('500');
  });

  it('정수 칸이다 — 좌표계와 같은 단위를 쓴다', () => {
    setup(cfg([rect()]));
    expect(testid('canvas-panel-width').getAttribute('step')).toBe('1');
    expect(testid('canvas-panel-height').getAttribute('step')).toBe('1');
  });

  it('바꾸면 canvas 패치가 나가고 다른 축은 그대로 실려 간다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-panel-width'), { target: { value: '800' } });
    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 800, height: 400 } });

    fireEvent.change(testid('canvas-panel-height'), { target: { value: '600' } });
    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 500, height: 600 } });
  });

  it('소수는 반올림하고 0 이하는 최소값으로 올린다 — 0 축은 모든 요소를 지운다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-panel-width'), { target: { value: '640.4' } });
    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 640, height: 400 } });

    fireEvent.change(testid('canvas-panel-width'), { target: { value: '0' } });
    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 1, height: 400 } });

    fireEvent.change(testid('canvas-panel-width'), { target: { value: '' } });
    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 1, height: 400 } });
  });

  // --- 패널 비율에 맞춤 (0.10.0) ---
  //
  // 축척이 하나가 된 뒤로 캔버스 비율과 패널 비율이 다르면 한 축에 여백이 남는다. 그것을
  // 없애는 단추가 이 두 칸 옆에 선다. **자동이 아니라 손으로** 누르는 것이 요점이다 —
  // 저장된 요소 좌표는 절대 캔버스 단위라, 높이가 바뀌면 사용자가 놓아 둔 자리의 뜻이
  // 달라지기 때문이다(`CanvasPanel` 시험 §조용한 이동 금지).

  /** 살아 있는 패널이 잰 상자를 내놓은 상태로 편집기를 세운다. */
  function setupWithStage(
    config: Record<string, unknown>,
    outer: { width: number; height: number },
  ): ReturnType<typeof vi.fn> {
    const onConfigChange = vi.fn();
    function Harness() {
      const state = useCanvasStageAspectState();
      // 패널이 하는 일과 같다: 잰 상자를 채널에 내놓는다.
      useEffect(() => state.publish(outer), [state]);
      return (
        <CanvasStageAspectContext value={state}>
          <CanvasElementsEditor config={config} onConfigChange={onConfigChange} />
        </CanvasStageAspectContext>
      );
    }
    render(<Harness />);
    return onConfigChange;
  }

  it('누르면 폭은 그대로 두고 높이를 패널 비율로 다시 계산한다', () => {
    // 사용자가 본 그 패널(1749×796)이다. round(500 × 796 / 1749) = 228.
    const spy = setupWithStage(cfg([rect()]), { width: 1749, height: 796 });
    fireEvent.click(testid('canvas-panel-size-fit'));

    expect(spy).toHaveBeenLastCalledWith({ canvas: { width: 500, height: 228 } });
    // 요소 좌표는 함께 가지 않는다 — 이 단추는 종이 모양만 바꾼다.
    expect(Object.keys(spy.mock.calls.at(-1)![0] as object)).toEqual(['canvas']);
  });

  it('무엇이 바뀔지 **누르기 전에** 적혀 있다', () => {
    setupWithStage(cfg([rect()]), { width: 1749, height: 796 });
    const title = testid('canvas-panel-size-fit').getAttribute('title') ?? '';
    expect(title).toContain('500×400');
    expect(title).toContain('500×228');
  });

  it('패널 비율을 아직 모르면 잠긴다 (곁에 미리보기가 없는 자리)', () => {
    setup(cfg([rect()]));
    expect((testid('canvas-panel-size-fit') as HTMLButtonElement).disabled).toBe(true);
  });

  it('이미 맞아 있으면 잠긴다 — 눌러도 화면이 그대로인 단추는 고장으로 보인다', () => {
    // 1000×800 은 500×400 과 같은 5:4 다.
    setupWithStage(cfg([rect()]), { width: 1000, height: 800 });
    expect((testid('canvas-panel-size-fit') as HTMLButtonElement).disabled).toBe(true);
  });

  it('잴 수 없는 상자에는 잠긴다 (0 · 비유한 — NaN 을 저장하지 않는다)', () => {
    for (const outer of [
      { width: 0, height: 0 },
      { width: 1749, height: 0 },
      { width: Number.NaN, height: 796 },
    ]) {
      cleanup();
      setupWithStage(cfg([rect()]), outer);
      expect(
        (testid('canvas-panel-size-fit') as HTMLButtonElement).disabled,
        `${outer.width}x${outer.height}`,
      ).toBe(true);
    }
  });

  it('크기를 바꿔도 **요소 좌표는 건드리지 않는다**', () => {
    // 함께 늘이면 정수 좌표에 반올림 오차가 쌓여 손으로 맞춰 둔 자리가 크기를 바꿀 때마다
    // 조금씩 어긋난다. 종이를 키운 것이지 그림을 키운 것이 아니다.
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-panel-width'), { target: { value: '1000' } });
    expect(Object.keys(spy.mock.calls.at(-1)![0] as object)).toEqual(['canvas']);
  });

  it('설명은 제목 뒤 `?` 에 있다 — 줄로 깔지 않는다', () => {
    setup(cfg([rect()]));
    expect(testid('canvas-panel-size-hint')).toBeTruthy();
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
    expect(el.geometry).toEqual({ x1: 50, y1: 80, x2: 450, y2: 200 });
  });

  it('rect → text: 기준점만 남는다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80 });
  });

  it('line → rect: 첫 끝점이 좌상단이 되고 크기는 기본값이다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 150, y1: 160, x2: 400, y2: 360 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'rect' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 150, y: 160, w: 100, h: 80 });
  });

  it('line → text: 첫 끝점이 정렬 기준점이 된다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 100, y1: 240, x2: 400, y2: 240 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 100, y: 240 });
  });

  it('text → text 처럼 형상이 이미 맞으면 기준점을 그대로 둔다', () => {
    const spy = setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 150, y: 360 }, style: {} }]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 150, y: 360 });
  });

  it('line → line 도 두 끝점을 그대로 둔다', () => {
    const spy = setup(
      cfg([{ id: 'l1', kind: 'line', geometry: { x1: 0, y1: 40, x2: 250, y2: 280 }, style: {} }]),
    );
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 0, y1: 40, x2: 250, y2: 280 });
  });

  it('text → line · text → rect 도 기준점을 살린다', () => {
    const spy = setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 350, y: 320 }, style: {} }]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 350, y1: 320, x2: 450, y2: 200 });

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 350, y: 320, w: 100, h: 80 });
  });

  it('rect → ellipse 처럼 형상이 같으면 기하가 그대로다', () => {
    const spy = setup(cfg([rich]));
    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 150, h: 160 });
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
    setup(cfg([rect()]), { tab: 'data' });
    const select = testid('canvas-element-binding-0') as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).toEqual(['', TEMP_ID, HUM_ID]);
    // 표시는 사람이 읽는 이름(alias)이고, 값은 동일성 키다 — 이름을 바꿔도 바인딩이 산다.
    expect(select.options[1]!.textContent).toBe('실습실 온도');
  });

  it('시리즈를 고르면 동일성 키로 바인딩하고 집계는 last 다', () => {
    const spy = setup(cfg([rect()]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-binding-0'), { target: { value: HUM_ID } });
    expect(lastElements(spy)[0]!.binding).toEqual({ series: HUM_ID, agg: 'last' });
  });

  it('"바인딩 없음"을 고르면 binding 키 자체가 사라진다 — 정적 도형은 합법이다', () => {
    const spy = setup(cfg([rect({ binding: { series: TEMP_ID, agg: 'last' } })]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-binding-0'), { target: { value: '' } });
    const el = lastElements(spy)[0]!;
    expect(el.binding).toBeUndefined();
    expect('binding' in el).toBe(false);
  });

  it('바인딩이 없으면 규칙 표를 읽기 전용으로 잠근다', () => {
    setup(cfg([rect(), rect({ id: 'b', binding: { series: TEMP_ID, agg: 'last' } })]), { tab: 'data' });
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
    { tab: 'data' },
    );
    fireEvent.click(testid('canvas-rule-delete-0'));
    const el = lastElements(spy)[0]!;
    expect(el.rules).toBeUndefined();
    expect('rules' in el).toBe(false);
  });

  it('소스에 없는 저장된 바인딩은 선택지로 되살려 조용히 잃지 않는다', () => {
    setup(cfg([rect({ binding: { series: 'ghost key ', agg: 'last' } })]), { tab: 'data' });
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
    },
    { tab: 'data' },
  );
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
    },
    { tab: 'data' },
  );
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values.length).toBeGreaterThan(1);
    expect(values[0]).toBe('');
  });

  it('소스가 없거나 data_source 가 없으면 "바인딩 없음" 하나만 낸다', () => {
    // data_source 미지정은 store 로 읽는다(기존 패널 config 하위호환 규약).
    setup({ elements: [rect()] }, { tab: 'data' });
    const values = Array.from((testid('canvas-element-binding-0') as HTMLSelectElement).options).map(
      (o) => o.value,
    );
    expect(values).toEqual(['']);
  });

  it('tsdb · sysmetrics 소스 블록이 비어 있어도 빈 목록으로 견딘다', () => {
    setup({ data_source: 'tsdb', elements: [rect()] }, { tab: 'data' });
    expect((testid('canvas-element-binding-0') as HTMLSelectElement).options).toHaveLength(1);

    cleanup();
    setup({ data_source: 'sysmetrics', elements: [rect()] }, { tab: 'data' });
    expect((testid('canvas-element-binding-0') as HTMLSelectElement).options).toHaveLength(1);
  });

  it('field 없는 store 시리즈도 동일성 키를 만든다 — 빈 metric 자리가 남는다', () => {
    setup(
      {
        data_source: 'store',
        store_source: { agent_name: 'a', series: [{ key: 'k' }], time_window_ms: 1, interval_ms: 1 },
        elements: [rect()],
      },
      { tab: 'data' },
    );
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
    },
    { tab: 'data' },
  );
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
    setup({ data_source: 'store', elements: [rect()] }, { tab: 'data' });

    expect(testid('canvas-element-binding-hint-0').textContent).toBe(
      'dashboard.canvas.elements.bindingNoSeries',
    );
  });

  it('tsdb · sysmetrics 소스가 비어 있을 때도 같은 안내가 뜬다', () => {
    setup({ data_source: 'tsdb', elements: [rect()] }, { tab: 'data' });
    expect(screen.getByTestId('canvas-element-binding-hint-0')).toBeTruthy();

    cleanup();
    setup({ data_source: 'sysmetrics', elements: [rect()] }, { tab: 'data' });
    expect(screen.getByTestId('canvas-element-binding-hint-0')).toBeTruthy();
  });

  it('시리즈가 있으면 안내는 나오지 않는다 — 고를 것이 있는데 하는 잔소리는 잡음이다', () => {
    setup(cfg([rect()]), { tab: 'data' });

    expect(screen.queryByTestId('canvas-element-binding-hint-0')).toBeNull();
  });

  it('목록이 비어도 저장된 바인딩이 되살아나 있으면 안내하지 않는다', () => {
    // 소스를 잠시 바꿨다 되돌리는 편집 도중의 자리다. 되살린 항목이 곧 고를 것이므로
    // "설정하세요" 는 그 자리에서 거짓말이 된다(`bindingOptionsFor` 계약).
    setup({
      data_source: 'store',
      elements: [rect({ binding: { series: 'temp value room=A', agg: 'last' } })],
    },
    { tab: 'data' },
  );

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

    // 글자 크기는 텍스트 탭이다 — 같은 `style` 키를 두 탭이 나눠 저술한다.
    openTab('text');
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

    openTab('text');
    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '18' } });
    expect(lastElements(spy)[0]!.style.fontSize).toBe(18);
  });

  it('열거형 스타일 칸의 "미지정"은 키를 지우고, 값을 고르면 키가 생긴다', () => {
    // 굵기·정렬은 텍스트 탭, 표시 여부는 스타일 탭이다. 둘을 오가며 재는 것이 곧 "탭을
    // 갈아도 저술이 끊기지 않는다" 를 함께 재는 자리가 된다.
    const spy = setup(cfg([rect({ style: { fontWeight: 'bold', align: 'center', visible: false } })]), {
      tab: 'text',
    });

    fireEvent.change(testid('canvas-element-font-weight-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ align: 'center', visible: false });

    fireEvent.change(testid('canvas-element-align-0'), { target: { value: 'right' } });
    expect(lastElements(spy)[0]!.style.align).toBe('right');

    openTab('style');
    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: 'show' } });
    expect(lastElements(spy)[0]!.style.visible).toBe(true);

    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ fontWeight: 'bold', align: 'center' });

    openTab('text');
    fireEvent.change(testid('canvas-element-font-weight-0'), { target: { value: 'normal' } });
    expect(lastElements(spy)[0]!.style.fontWeight).toBe('normal');

    fireEvent.change(testid('canvas-element-align-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.style).toEqual({ fontWeight: 'bold', visible: false });

    openTab('style');
    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: 'hide' } });
    expect(lastElements(spy)[0]!.style.visible).toBe(false);
  });

  it('범위 안의 불투명도는 그대로 실린다', () => {
    const spy = setup(cfg([rect()]));
    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '0.4' } });
    expect(lastElements(spy)[0]!.style.opacity).toBe(0.4);
  });

  it('문구 · 단위에 값을 넣으면 그대로 실린다', () => {
    const spy = setup(cfg([rect()]), { tab: 'text' });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{name}: {value}' } });
    expect(lastElements(spy)[0]!.text).toBe('{name}: {value}');

    // 단위는 값을 어떻게 읽는가의 일부라 데이터 탭의 숫자 스위치 옆에 있다.
    openTab('data');
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

    openTab('text');
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
    const spy = setup(cfg([rect({ text: 'x', unit: '°C' })]), { tab: 'text' });

    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '' } });
    let el = lastElements(spy)[0]!;
    expect('text' in el).toBe(false);
    expect(el.unit).toBe('°C');

    openTab('data');
    fireEvent.change(testid('canvas-element-unit-0'), { target: { value: '   ' } });
    el = lastElements(spy)[0]!;
    expect('unit' in el).toBe(false);
  });

  it('소수 자리는 비음수 정수로 죄고, 비우면 키가 사라진다', () => {
    const spy = setup(cfg([rect({ decimals: 2 })]), { tab: 'data' });

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '3.7' } });
    expect(lastElements(spy)[0]!.decimals).toBe(3);

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '-1' } });
    expect(lastElements(spy)[0]!.decimals).toBe(0);

    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '' } });
    expect('decimals' in lastElements(spy)[0]!).toBe(false);
  });

  it('문구 토큰 3종을 요소마다 제목 뒤 ? 도움말에 담아 명세를 찾지 않아도 되게 한다', () => {
    setup(cfg([rect()]), { tab: 'text' });
    expect(testid('canvas-element-token-help-0').textContent).toBe(
      'dashboard.canvas.elements.tokenHelp',
    );
  });
});

// --- 트윈 ---------------------------------------------------------------

describe('CanvasElementsEditor — 트윈', () => {
  it('요소 트윈 지속 시간을 넣으면 이징과 함께 덮어쓰기가 생긴다', () => {
    const spy = setup(cfg([rect()]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '500' } });
    expect(lastElements(spy)[0]!.tween).toEqual({ duration_ms: 500, easing: 'ease-out' });
  });

  it('요소 트윈 이징만 바꾸면 기존 지속 시간을 지킨다', () => {
    const spy = setup(cfg([rect({ tween: { duration_ms: 300, easing: 'linear' } })]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-tween-easing-0'), { target: { value: 'ease-in' } });
    expect(lastElements(spy)[0]!.tween).toEqual({ duration_ms: 300, easing: 'ease-in' });
  });

  it('지속 시간을 비우면 덮어쓰기 자체가 사라진다(= 패널 기본 사용)', () => {
    const spy = setup(cfg([rect({ tween: { duration_ms: 300, easing: 'linear' } })]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '' } });
    expect('tween' in lastElements(spy)[0]!).toBe(false);
  });

  it('덮어쓰기가 없는 요소의 이징만 건드리면 아무 트윈도 생기지 않는다', () => {
    const spy = setup(cfg([rect()]), { tab: 'data' });
    fireEvent.change(testid('canvas-element-tween-easing-0'), { target: { value: 'ease-in' } });
    expect('tween' in lastElements(spy)[0]!).toBe(false);
  });

  it('음수 지속 시간은 0(즉시 전환)으로 죈다', () => {
    const spy = setup(cfg([rect()]), { tab: 'data' });
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

  // 이 칸이 무엇을 하는지는 값이 **바뀌는 순간에만** 드러나므로 겉모습으로 읽히지 않는다.
  // 그래서 설명이 화면에 상주해야 한다(AC-E16) — 줄로 깔지 않고 제목 뒤 `?` 에 담되,
  // 설명 자체는 sr-only 로 항상 DOM 에 있어 스크린리더가 읽는다(`FieldHelp`).
  it('요소 전환 효과 제목의 ? 가 무엇이 바뀌는지와 0 의 뜻을 말한다 (AC-E16)', () => {
    setup(cfg([rect()]), { tab: 'data' });
    expect(testid('canvas-element-tween-hint-0').textContent).toBe(
      'dashboard.canvas.elements.tweenHint',
    );
  });

  it('패널 기본값 쪽 ? 에도 그 설명이 있다 — 같은 빈칸이 두 자리에서 다른 뜻이다 (AC-E16)', () => {
    setup(cfg([]));
    // 요소 쪽 빈칸은 "패널 기본 사용", 패널 쪽 빈칸은 "즉시 전환" 이므로 문장을 돌려 쓰지 않는다.
    expect(testid('canvas-panel-tween-hint').textContent).toBe(
      'dashboard.canvas.elements.panelTweenHint',
    );
    expect(testid('canvas-panel-tween-hint').textContent).not.toBe(
      'dashboard.canvas.elements.tweenHint',
    );
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
        { id: 'l1', kind: 'line', geometry: { x1: -100, y1: 200, x2: 700, y2: 200 }, style: {} },
        { id: 't1', kind: 'text', geometry: { x: 250, y: 200 }, style: { align: 'center' }, text: 'hi' },
      ]),
    );

    // 픽스처를 그대로 파싱하면 파서만 검사한다 — 실제로 편집해 편집기가 만든 배열을 본다.
    // 셋이 서로 다른 탭에 있으므로 이 왕복은 **탭을 오간 편집이 누적되는가**도 함께 잰다.
    openTab('arrange');
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '75' } });
    openTab('data');
    fireEvent.change(testid('canvas-element-unit-0'), { target: { value: 'kPa' } });
    openTab('text');
    fireEvent.change(testid('canvas-element-align-2'), { target: { value: 'right' } });
    expectRoundTrip(live.config);
  });

  it('죄인 극단값이 누적된 뒤에도 왕복에서 바뀌지 않는다', () => {
    const live = setupStateful(cfg([rect()]));

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '9' } });
    fireEvent.change(testid('canvas-element-stroke-width-0'), { target: { value: '-3' } });
    openTab('text');
    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '-8' } });
    openTab('data');
    fireEvent.change(testid('canvas-element-decimals-0'), { target: { value: '2.9' } });
    fireEvent.change(testid('canvas-element-tween-duration-0'), { target: { value: '-40' } });
    openTab('arrange');
    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '3.5' } });

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

  it('팔레트로 종류마다 하나씩 쌓은 뒤 목록에서 고쳐도 왕복에서 그대로다', () => {
    // 만드는 자리가 옮겨 갔어도 재는 것은 같다: **만들어진 것이 이 편집기를 지나
    // 파서까지 온전히 돌아오는가.** 그래서 팔레트로 쌓고 목록으로 고친 뒤 왕복한다.
    const live = setupPalette([], { withList: true });
    place('rect');
    place('ellipse');
    place('line');
    place('text');

    expect(live.elements.map((e) => e.kind)).toEqual(['rect', 'ellipse', 'line', 'text']);

    expandAllRows();
    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '0.5' } });

    expect(live.elements[0]!.style.opacity).toBe(0.5);
    expectRoundTrip(cfg(live.elements));
  });

  it('종류를 바꾸고 규칙까지 붙인 요소도 왕복에서 그대로다', () => {
    const live = setupStateful(cfg([rect({ binding: { series: TEMP_ID, agg: 'last' } })]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'text' } });
    openTab('text');
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{name} {value}{unit}' } });
    openTab('data');
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

/** 200×100 스테이지에 기본 캔버스(500×400)를 투영한다(`drawElement.test.ts` 와 같은 치수). */
const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

/**
 * 팔레트를 눌러 **화면이 실제로 만든** 요소를 꺼낸다.
 *
 * 목록 하단의 추가 버튼을 걷어낸 뒤 남은 유일한 입구다. 이음매를 건너는 이 시험들에서
 * 중요한 것은 "어느 버튼을 눌렀는가" 가 아니라 **사용자가 화면에서 만든 그 값**이 렌더
 * 층까지 살아 가는가이므로, 눌리는 버튼만 바꾸고 재는 것은 그대로 둔다.
 */
function addedElement(kind: CanvasElementKind): CanvasElement {
  const live = setupPalette();
  place(kind);
  const el = live.elements[0]!;
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
        geometry: { x1: 50, y1: 200, x2: 450, y2: 200 },
        style: style ?? { stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH },
      }
    : {
        id: 's1',
        kind,
        geometry: { x: 50, y: 40, w: 100, h: 80 },
        style: style ?? { fill: SEED_COLOR },
      };
}

/** 문구 칸에 **실제로 타이핑해** 편집기가 내보낸 요소를 꺼낸다. */
function typedLabel(
  kind: (typeof SHAPE_KINDS)[number],
  text: string,
  style?: CanvasElement['style'],
): CanvasElement {
  const spy = setup(cfg([shapeEl(kind, style)]), { tab: 'text' });
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
  drawElements(ctx, [el], {}, text === undefined ? {} : { [el.id]: text }, PROJ);
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
    const live = setupStateful(cfg([shapeEl('rect')]), { tab: 'text' });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{value}' } });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '' } });

    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el.text).toBeUndefined();
    expect(el.style.textColor).toBe(SEED_TEXT_COLOR);
  });

  it("kind:'text' 는 종전 그대로다 — 문구 요소는 textColor ?? fill 로 칠해진다", () => {
    const spy = setup(cfg([{ id: 't1', kind: 'text', geometry: { x: 250, y: 200 }, style: {} }]), {
      tab: 'text',
    });
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: '{value}' } });

    const el = lastElements(spy)[0]!;
    expect(el.text).toBe('{value}');
    expect(el.style.textColor).toBeUndefined();
  });
});

// --- 신규 요소의 자리 ----------------------------------------------------

describe('CanvasElementsEditor — 신규 요소는 겹치지 않는다', () => {
  /** 스테이지를 벗어났는지 본다(0..1 밖은 일부라도 화면 밖이다). */
  /** 기본 캔버스(500x400) 안에 온전히 들어 있는가. */
  function onCanvas(g: Record<string, number>): boolean {
    const right = (g.x ?? 0) + (g.w ?? 0);
    const bottom = (g.y ?? 0) + (g.h ?? 0);
    return (
      (g.x ?? 0) >= 0 &&
      (g.y ?? 0) >= 0 &&
      right <= DEFAULT_CANVAS_SIZE.width &&
      bottom <= DEFAULT_CANVAS_SIZE.height
    );
  }

  it('같은 종류를 세 번 더하면 세 자리가 모두 다르다', () => {
    const live = setupPalette();
    place('rect');
    place('rect');
    place('rect');

    const geos = live.elements.map((e) => JSON.stringify(e.geometry));
    expect(new Set(geos).size).toBe(3);
    expect(geos[0]).toBe(JSON.stringify({ x: 50, y: 40, w: 100, h: 80 }));
    expect(geos[1]).toBe(JSON.stringify({ x: 75, y: 65, w: 100, h: 80 }));
    expect(geos[2]).toBe(JSON.stringify({ x: 100, y: 90, w: 100, h: 80 }));
  });

  it('선과 문구는 여유가 있는 세로 축으로만 내려온다', () => {
    const live = setupPalette();
    place('line');
    place('line');
    place('text');
    place('text');

    const els = live.elements;
    // 가로는 이미 캔버스를 가로지르므로 건드리지 않는다.
    expect(els[0]!.geometry).toEqual({ x1: 50, y1: 200, x2: 450, y2: 200 });
    expect(els[1]!.geometry).toEqual({ x1: 50, y1: 225, x2: 450, y2: 225 });
    // 문구는 기준점에서 오른쪽으로 흐르므로 가로를 밀면 글자가 밖으로 나간다.
    expect(els[2]!.geometry).toEqual({ x: 250, y: 250 });
    expect(els[3]!.geometry).toEqual({ x: 250, y: 275 });
  });

  it('계단은 되감긴다 — 아홉 번을 더해도 캔버스 밖으로 행진하지 않는다', () => {
    const live = setupPalette();
    for (let i = 0; i < 9; i++) place('rect');

    const els = live.elements;
    expect(els).toHaveLength(9);
    for (const el of els) {
      expect(onCanvas(el.geometry as unknown as Record<string, number>)).toBe(true);
    }
    // 아홉 번째는 첫 번째 자리로 되감긴다(되감기 폭 8).
    expect(els[8]!.geometry).toEqual(els[0]!.geometry);
  });

  it('계단이 붙어도 좌표가 정수로 남는다 — 숫자 칸이 소수를 보이지 않는다', () => {
    const live = setupPalette();
    for (let i = 0; i < 4; i++) place('rect');

    const shown = live.elements.map((e) => (e.geometry as { x: number }).x);
    expect(shown).toEqual([50, 75, 100, 125]);
    for (const v of shown) expect(Number.isInteger(v)).toBe(true);
  });

  it('기존 요소의 좌표는 건드리지 않는다 — 캔버스 밖 저술은 합법이다', () => {
    const live = setupPalette([
      { id: 'far', kind: 'rect', geometry: { x: -250, y: 800, w: 1500, h: 1600 }, style: {} },
    ]);
    place('rect');

    expect(live.elements[0]!.geometry).toEqual({ x: -250, y: 800, w: 1500, h: 1600 });
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

    // 펼침의 자취는 **탭 줄**이다. 어느 탭이 열려 있든 몸통이 있으면 탭 줄이 있고,
    // 없으면 없다 — 칸 하나를 자취로 삼으면 그 칸이 다른 탭으로 옮겨 가는 날 이 시험은
    // 접힘이 깨진 것처럼 거짓말을 한다.
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByTestId('canvas-element-tabs-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-kind-0')).toBeTruthy();

    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByTestId('canvas-element-tabs-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-kind-0')).toBeNull();
  });

  it('접힌 줄에서도 순번 · 순서 이동 · 삭제는 그대로 닿는다', () => {
    // 머리줄에 남는 것은 **펼치지 않고도 되어야 하는 일** 뿐이다: 훑기(순번·요약) ·
    // 순서 조작 · 삭제. 종류는 그 목록에 없다 — 저술이므로 몸통(도형 묶음)에서 한다.
    const spy = setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]), { expand: false });

    expect(testid('canvas-element-order-0').textContent).toBe('1');
    expect((testid('canvas-element-move-down-0') as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByTestId('canvas-element-kind-0')).toBeNull();

    fireEvent.click(testid('canvas-element-move-down-0'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['b', 'a']);

    fireEvent.click(testid('canvas-element-delete-1'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['a']);
  });

  it('갓 놓은 요소의 줄은 펼쳐진 채로 붙는다 — 눌렀는데 아무 일도 없어 보이면 안 된다', () => {
    // 되먹임의 출처가 바뀌었다. 목록이 스스로 펼치던 자리를 걷어냈고, 지금은 팔레트가
    // **놓은 것을 고르며**(T9) 그 선택이 행을 펼친다(T10). 재는 것은 그대로다 —
    // 방금 만든 것이 화면에서 스스로를 소개하는가.
    setupPalette([rect({ id: 'old' })], { withList: true });
    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('false');

    place('rect');

    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('false');
    expect(testid('canvas-element-toggle-1').getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByTestId('canvas-element-tabs-1')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-tabs-0')).toBeNull();
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

  it('캔버스 선택이 있어도 목록에 추가 버튼이 되살아나지 않는다', () => {
    // 만드는 자리는 도크 팔레트 하나다. 선택 provider 가 있든 없든 그 사실은 같다.
    setup(cfg([]), { expand: false });
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.queryByTestId(`canvas-element-add-${kind}`)).toBeNull();
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

/**
 * 요소 하나를 캔버스에서 골라 둔 상태로 편집기를 그린다(그 행은 자동으로 펼쳐진다).
 *
 * 이 절이 재는 것은 좌표 칸이 **선택에 밀려나지 않는가** 이므로, 골라 둔 채로 그 칸이
 * 사는 배치 탭을 연다. 탭을 여는 것은 시험의 주제가 아니라 전제다.
 */
function setupPicked(elements: CanvasElement[], pick: string, tab: TabName = 'arrange') {
  const onConfigChange = vi.fn();
  render(
    <SelectionSpyHarness config={cfg(elements)} onConfigChange={onConfigChange} pick={pick} />,
  );
  fireEvent.click(testid('pick'));
  openTab(tab);
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
      [{ id: 'a', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} }],
      'a',
    );
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect((testid(`canvas-element-geo-${axis}-0`) as HTMLInputElement).disabled).toBe(false);
    }

    cleanup();
    setupPicked([{ id: 'a', kind: 'text', geometry: { x: 250, y: 200 }, style: {} }], 'a');
    for (const axis of ['x', 'y']) {
      expect((testid(`canvas-element-geo-${axis}-0`) as HTMLInputElement).disabled).toBe(false);
    }
    // 문구의 포인터 수단은 글자 크기 핸들 하나뿐이므로 그 등가물이 특히 남아 있어야 한다.
    openTab('text');
    expect((testid('canvas-element-font-size-0') as HTMLInputElement).disabled).toBe(false);
  });

  it('골라 둔 채로 타이핑하면 그대로 저술된다 — 회수 경로가 살아 있다', () => {
    const spy = setupPicked([rect({ id: 'a' })], 'a');
    spy.mockClear();

    // 캔버스 밖으로 전부 나간 요소를 되돌리는 그 조작이다(clamp 하지 않으므로 가능하다).
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '-250' } });

    expect(lastElements(spy)[0]!.geometry).toEqual({ x: -250, y: 80, w: 150, h: 160 });
  });

  it('순서·삭제도 캔버스 선택과 무관하게 목록에 그대로 남는다', () => {
    setupPicked([rect({ id: 'a' }), rect({ id: 'b' })], 'a');

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
    //
    // 배치 탭이 맨 앞/맨 뒤 보내기를 함께 내면서 들여오는 이름이 셋이 됐다. 셋 다 같은
    // 모듈에서 오고, 그 모듈 안에서 `bringToFront`·`sendToBack` 은 `moveElementTo` 로
    // 지어져 있다 — 그래서 네 동작이 지나는 규칙은 여전히 하나다.
    const source = readFileSync(join(__dirname, 'CanvasElementsEditor.tsx'), 'utf-8');
    expect(source).toMatch(
      /import \{ bringToFront, moveElementTo, sendToBack \} from '\.\/canvasEditArrange'/,
    );
    // 배열을 여기서 직접 자르지 않는다 — 두 번째 정렬 규칙이 생기는 자리가 그것이다.
    expect(source).not.toMatch(/\.splice\(/);
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

/**
 * 라이브 채널을 물린 채 편집기를 그리고 모든 줄을 펼친 뒤 **데이터 탭을 연다**.
 *
 * 이 절이 재는 것은 전부 바인딩 드롭다운이고 그것은 데이터 탭에 있다. 여기서 탭을 열지
 * 않으면 "안내가 뜨지 않는다" 류의 부재 단언이 **탭이 닫혀 있어서** 통과한다 — 참인 이유가
 * 재려던 것과 다른 통과는 통과가 아니다.
 */
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
  openTab('data');
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
    setup(cfg([rect()]), { tab: 'data' });

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

// --- 목록 행 → 캔버스 선택 (역방향 배선) -----------------------------------
//
// T10 이 놓은 변은 **캔버스 → 목록** 한 방향뿐이었다. 사용 시험이 되돌려 보낸 요구는 그
// 반대다: "요소 설정 클릭시 해당 컴포넌트 선택". 두 변이 같은 공유 컨텍스트를 지나므로
// 여기서 재는 것은 셋이다.
//   1. 행을 펼치면 **그 요소가 실제로 골라진다**(그리고 오버레이가 그것을 본다).
//   2. T10 의 자동 펼침 규칙 넷이 그대로 남는다 — 이미 있던 시험들이 그 몫을 지고 있으므로
//      여기서는 되풀이하지 않고, 새 변이 그 시험들을 깨지 않는 것으로 확인한다.
//   3. **진동이 없다.** 역방향 변(행 → 선택)이 순방향 변(선택 → 자동 펼침)을 도로 깨워
//      다시 선택을 부르는 고리가 생기면 렌더가 멎지 않는다. 커밋 수를 세는 것이 그 사실을
//      말하는 정직한 방법이다.

/** 목록 편집기와 **진짜 오버레이**를 한 provider 아래 함께 세운다. */
function ListAndOverlay({ elements }: { elements: CanvasElement[] }) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <CanvasEditOverlay
        enabled
        elements={elements}
        projection={{ stage: { width: 200, height: 100 }, canvas: { ...DEFAULT_CANVAS_SIZE } }}
        textWidths={{}}
        onElementsChange={() => {}}
      />
      <CanvasElementsEditor config={cfg(elements)} onConfigChange={() => {}} />
    </CanvasEditSelectionContext>
  );
}

describe('CanvasElementsEditor — 목록 행을 누르면 그 요소가 골라진다', () => {
  it('행을 펼치면 그 행이 선택 표시를 얻는다', () => {
    renderThree();
    expect(testid('canvas-element-0').getAttribute('data-selected')).toBeNull();

    fireEvent.click(testid('canvas-element-toggle-0'));

    expect(testid('canvas-element-0').getAttribute('data-selected')).toBe('true');
    expect(testid('canvas-element-1').getAttribute('data-selected')).toBeNull();
  });

  it('다음 행을 펼치면 선택이 그쪽으로 옮겨 간다 (단일 선택)', () => {
    renderThree();
    fireEvent.click(testid('canvas-element-toggle-0'));
    fireEvent.click(testid('canvas-element-toggle-1'));

    expect(testid('canvas-element-0').getAttribute('data-selected')).toBeNull();
    expect(testid('canvas-element-1').getAttribute('data-selected')).toBe('true');
  });

  it('오버레이가 그 선택을 본다 — 같은 컨텍스트 하나를 지난다', () => {
    // 목록과 캔버스가 각자 선택을 들면 "목록에서 고른 것" 과 "캔버스에서 고른 것" 이
    // 서로 다른 것을 가리킬 수 있다. 진짜 오버레이를 세워 그 한 벌임을 잰다.
    render(<ListAndOverlay elements={[rect({ id: 'a' }), rect({ id: 'b' })]} />);
    expect(screen.queryByTestId('canvas-selection-a')).toBeNull();

    fireEvent.click(testid('canvas-element-toggle-0'));

    expect(screen.getByTestId('canvas-selection-a')).toBeTruthy();
    expect(screen.queryByTestId('canvas-selection-b')).toBeNull();
    // 단일 선택이므로 오버레이의 손잡이도 그 요소에 붙는다(모서리 하나만 확인한다).
    expect(screen.getByTestId('canvas-handle-nw')).toBeTruthy();
  });

  it('접는 방향에서는 고르지 않는다 — 골랐다면 그 선택이 행을 도로 펼친다', () => {
    // `autoExpandedId` 는 선택에서 **파생**되므로(canvasEditContext), "접으면서 고른다" 는
    // 자기모순이다: 접기 단추가 접지 못하는 단추가 된다.
    renderThree();
    fireEvent.click(testid('canvas-element-toggle-0')); // 펼침 + 고름
    expect(isRowOpen(0)).toBe(true);

    fireEvent.click(testid('canvas-element-toggle-0')); // 접힘
    expect(isRowOpen(0)).toBe(false);
    // 선택은 남는다 — 접기가 캔버스의 손잡이까지 걷어 가지는 않는다.
    expect(testid('canvas-element-0').getAttribute('data-selected')).toBe('true');
  });

  it('provider 가 없어도 죽지 않는다 — 로컬 선택으로 떨어진다', () => {
    setup(cfg([rect({ id: 'a' })]), { expand: false });

    fireEvent.click(testid('canvas-element-toggle-0'));

    expect(isRowOpen(0)).toBe(true);
    expect(testid('canvas-element-0').getAttribute('data-selected')).toBe('true');
  });
});

// --- 진동 없음 (역방향 변이 순방향 변을 되깨우지 않는다) --------------------

/**
 * 커밋 횟수를 세면서 목록을 그린다.
 *
 * `Profiler` 는 이 부분 트리가 **실제로 커밋된 횟수**를 센다. 고리가 생겼다면 둘 중
 * 하나가 일어난다: React 가 "Maximum update depth exceeded" 로 던지거나(그러면 시험이
 * 그 자리에서 실패한다), 커밋 수가 상한 없이 늘어난다. 그래서 "클릭 한 번에 커밋 N회
 * 이하" 는 진동이 없다는 말을 재는 정직한 방식이다 — `fireEvent` 가 돌아온 시점에는
 * React 가 효과까지 모두 흘려보낸 뒤다.
 */
function renderCounted(elements: CanvasElement[]): { commits: () => number } {
  let count = 0;
  function Counted() {
    const state = useCanvasEditSelectionState();
    return (
      <CanvasEditSelectionContext value={state}>
        <Profiler id="editor" onRender={() => { count += 1; }}>
          <CanvasElementsEditor config={cfg(elements)} onConfigChange={() => {}} />
        </Profiler>
      </CanvasEditSelectionContext>
    );
  }
  render(<Counted />);
  return { commits: () => count };
}

describe('CanvasElementsEditor — 역방향 배선에 진동이 없다', () => {
  it('행을 펼쳐 고르는 한 번의 조작이 정해진 횟수 안에 가라앉는다', () => {
    const { commits } = renderCounted([rect({ id: 'a' }), rect({ id: 'b' })]);
    const base = commits();

    fireEvent.click(testid('canvas-element-toggle-0'));

    // 실제로는 둘이다: (1) 펼침 집합 + 선택이 함께 배치된 커밋, (2) `autoExpandedId` 를
    // 본 효과가 `canvasExpandedId` 를 옮긴 커밋. 고리가 있었다면 여기서 멎지 않는다.
    const afterFirst = commits() - base;
    expect(afterFirst).toBeGreaterThan(0);
    expect(afterFirst).toBeLessThanOrEqual(3);
  });

  it('이미 골라진 행을 다시 펼쳐도 선택 상태는 갈리지 않는다 (같은 참조를 돌려받는다)', () => {
    // `nextSelection` 이 이미 골라진 것에 대해 **같은 Set** 을 돌려주므로 provider 의
    // 상태가 바뀌지 않고, 따라서 순방향 변이 다시 깨어나지 않는다. 이것이 고리를 끊는
    // 자리다 — 매번 새 Set 을 만들면 여기서 커밋이 한 번 더 붙는다.
    const { commits } = renderCounted([rect({ id: 'a' })]);
    fireEvent.click(testid('canvas-element-toggle-0')); // 펼침 + 고름
    fireEvent.click(testid('canvas-element-toggle-0')); // 접힘 (고르지 않는다)
    const base = commits();

    fireEvent.click(testid('canvas-element-toggle-0')); // 다시 펼침 — 이미 골라져 있다

    // 펼침 집합이 갈린 커밋 하나뿐이다. 선택이 갈렸다면 효과 커밋이 하나 더 붙는다.
    expect(commits() - base).toBe(1);
    expect(isRowOpen(0)).toBe(true);
  });
});

// --- 위치 · 크기 · 도형 스타일 · 문구 스타일의 분리 --------------------------
//
// 한 줄에 늘어놓던 좌표와 스타일을 뜻이 다른 묶음으로 가른다. 여기서 재는 것은 **묶음의
// 종류별 비대칭**이다 — line 과 text 에는 크기 묶음이 없어야 하고(없는 것을 만들어
// 보이면 사용자는 그 칸을 찾다 못 찾는다), `fill` 은 그 종류에서 실제로 칠하는 자리에
// 서야 한다.
//
// 그 비대칭은 탭으로 갈린 뒤에도 그대로다 — 옮긴 것은 묶음이 서는 **자리**이지 어느
// 종류가 어느 묶음을 갖는가가 아니다. 그래서 아래 시험들은 해당 탭을 열고 같은 것을 잰다.

/** 이 행에 그려진 묶음 testid 들. */
function hasGroup(name: string, idx = 0): boolean {
  return screen.queryByTestId(`canvas-element-${name}-${idx}`) !== null;
}

describe('CanvasElementsEditor — 위치와 크기를 가른다', () => {
  it('rect 는 위치(x·y)와 크기(w·h) 두 묶음을 낸다', () => {
    setup(cfg([rect()]), { tab: 'arrange' });

    expect(hasGroup('position')).toBe(true);
    expect(hasGroup('size')).toBe(true);

    const position = testid('canvas-element-position-0');
    const size = testid('canvas-element-size-0');
    expect(position.contains(testid('canvas-element-geo-x-0'))).toBe(true);
    expect(position.contains(testid('canvas-element-geo-y-0'))).toBe(true);
    expect(size.contains(testid('canvas-element-geo-w-0'))).toBe(true);
    expect(size.contains(testid('canvas-element-geo-h-0'))).toBe(true);
    // 위치 묶음이 크기 칸을 물고 있으면 가른 뜻이 없다.
    expect(position.contains(testid('canvas-element-geo-w-0'))).toBe(false);
  });

  it('ellipse 도 같은 두 묶음이다', () => {
    setup(cfg([rect({ id: 'e', kind: 'ellipse' })]), { tab: 'arrange' });

    expect(hasGroup('position')).toBe(true);
    expect(hasGroup('size')).toBe(true);
  });

  it('line 은 두 끝점뿐이며 **크기 묶음을 만들지 않는다**', () => {
    setup(cfg([{ id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} }]), {
      tab: 'arrange',
    });

    expect(hasGroup('position')).toBe(true);
    expect(hasGroup('size')).toBe(false);
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect(testid('canvas-element-position-0').contains(testid(`canvas-element-geo-${axis}-0`))).toBe(
        true,
      );
    }
    expect(testid('canvas-element-position-0').textContent).toContain(
      'dashboard.canvas.elements.endpointsLabel',
    );
  });

  it('text 는 기준점뿐이며 크기 묶음이 없다 — 그 크기는 글자 크기다', () => {
    setup(cfg([{ id: 't', kind: 'text', geometry: { x: 250, y: 200 }, style: {} }]), {
      tab: 'arrange',
    });

    expect(hasGroup('position')).toBe(true);
    expect(hasGroup('size')).toBe(false);
    // 글자 크기는 텍스트 탭에 **한 자리에만** 있다 — 배치 탭에서는 찾을 수 없다.
    expect(screen.queryByTestId('canvas-element-font-size-0')).toBeNull();
    openTab('text');
    expect(screen.getByTestId('canvas-element-font-size-0')).toBeTruthy();
  });

  it('0..1 이라는 사실은 위치 묶음 제목 뒤 ? 가 계속 말한다', () => {
    setup(cfg([rect()]), { tab: 'arrange' });

    expect(testid('canvas-element-coord-help-0').textContent).toBe(
      'dashboard.canvas.elements.coordHint',
    );
  });

  // 설명을 `?` 뒤로 옮겼다고 **접근성이 내려가면 안 된다**: 설명은 sr-only 로 DOM 에
  // 상주하고(위 검사), 눈으로 보려면 키보드로 닿는 button 을 눌러 연다.
  it('좌표 설명의 ? 는 위치 묶음 안에 있고 눌러야 눈에 보인다', () => {
    setup(cfg([rect()]), { tab: 'arrange' });

    const group = testid('canvas-element-position-0');
    expect(group.contains(testid('canvas-element-coord-help-0'))).toBe(true);
    // 크기 묶음에는 같은 말을 두 번 두지 않는다.
    expect(testid('canvas-element-size-0').contains(testid('canvas-element-coord-help-0'))).toBe(
      false,
    );

    const button = group.querySelector('button[aria-expanded]');
    expect(button).toBeTruthy();
    expect(button!.getAttribute('aria-expanded')).toBe('false');

    fireEvent.click(button!);

    expect(button!.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('tooltip').textContent).toBe('dashboard.canvas.elements.coordHint');
  });

  it('가른 것은 표현뿐이다 — 좌표는 종전대로 죄이지 않고 그대로 저술된다', () => {
    const spy = setup(cfg([rect()]), { tab: 'arrange' });

    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '-250' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: -250, y: 80, w: 150, h: 160 });

    fireEvent.change(testid('canvas-element-geo-w-0'), { target: { value: '1000' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 80, w: 1000, h: 160 });

    // 비유한 입력은 여전히 0 으로 막힌다(NaN 이 기하에 들어가면 요소가 통째로 사라진다).
    fireEvent.change(testid('canvas-element-geo-y-0'), { target: { value: '' } });
    expect(lastElements(spy)[0]!.geometry).toEqual({ x: 50, y: 0, w: 150, h: 160 });
  });
});

describe('CanvasElementsEditor — 도형 스타일과 문구 스타일을 가른다', () => {
  it('도형에서는 채움·선이 스타일 탭에, 글자색·글자 크기·굵기·정렬이 텍스트 탭에 선다', () => {
    // 가름은 이제 **탭**이다. 그래서 "다른 묶음에 없다" 를 재던 자리가 "다른 탭을 열면
    // 아예 없다" 가 된다 — 열린 탭 하나만 그리므로 그것이 같은 말의 더 강한 형태다.
    setup(cfg([rect()]));

    const shape = testid('canvas-element-shape-style-0');
    for (const id of ['fill', 'stroke', 'stroke-width', 'opacity', 'visible']) {
      expect(shape.contains(testid(`canvas-element-${id}-0`))).toBe(true);
    }
    for (const id of ['text-color', 'font-size', 'font-weight', 'align']) {
      expect(screen.queryByTestId(`canvas-element-${id}-0`)).toBeNull();
    }

    openTab('text');
    const text = testid('canvas-element-text-style-0');
    for (const id of ['text-color', 'font-size', 'font-weight', 'align']) {
      expect(text.contains(testid(`canvas-element-${id}-0`))).toBe(true);
    }
    for (const id of ['fill', 'stroke', 'stroke-width', 'opacity', 'visible']) {
      expect(screen.queryByTestId(`canvas-element-${id}-0`)).toBeNull();
    }
  });

  it('도형에서는 라벨이 글자색만 쓴다는 사실을 텍스트 탭 제목 뒤 ? 가 말한다 (자동으로 심긴 색이 결함으로 읽히지 않게)', () => {
    setup(cfg([rect()]), { tab: 'text' });

    expect(testid('canvas-element-text-style-help-0').textContent).toBe(
      'dashboard.canvas.elements.textStyleHintShape',
    );
  });

  it('문구 요소에서는 채움색이 **텍스트 탭 쪽**으로 옮겨 간다 (textColor ?? fill)', () => {
    // 렌더 층은 `kind:'text'` 를 `textColor ?? fill` 로 칠한다(`drawElement`). 칠할 도형이
    // 없는 요소의 채움색을 "도형" 이라 부르면 화면이 거짓말을 한다.
    setup(cfg([{ id: 't', kind: 'text', geometry: { x: 0, y: 0 }, style: {} }]));

    // 스타일 탭에는 채움색이 없다 — 칠할 도형이 없기 때문이다.
    expect(testid('canvas-element-shape-style-0').contains(testid('canvas-element-stroke-0'))).toBe(
      true,
    );
    expect(screen.queryByTestId('canvas-element-fill-0')).toBeNull();

    openTab('text');
    const text = testid('canvas-element-text-style-0');
    expect(text.contains(testid('canvas-element-fill-0'))).toBe(true);
    // 글자색 바로 옆이다 — 그 둘이 한 값을 두고 폴백 관계이기 때문이다.
    expect(text.contains(testid('canvas-element-text-color-0'))).toBe(true);
    expect(testid('canvas-element-text-style-help-0').textContent).toBe(
      'dashboard.canvas.elements.textStyleHintText',
    );
  });

  it('가른 것은 표현뿐이다 — 어느 탭에 서든 같은 style 키로 저술된다', () => {
    const spy = setup(cfg([{ id: 't', kind: 'text', geometry: { x: 0, y: 0 }, style: {} }]), {
      tab: 'text',
    });

    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '18' } });
    expect(lastElements(spy)[0]!.style.fontSize).toBe(18);

    fireEvent.change(testid('canvas-element-align-0'), { target: { value: 'center' } });
    expect(lastElements(spy)[0]!.style.align).toBe('center');

    openTab('style');
    fireEvent.change(testid('canvas-element-visible-0'), { target: { value: 'hide' } });
    expect(lastElements(spy)[0]!.style.visible).toBe(false);
  });
});

// --- 글자 크기 (사용 시험: "하부 메뉴들의 폰트가 너무 작음") ------------------

describe('캔버스 편집기의 글자 크기는 주변 설정 화면을 따른다', () => {
  it('두 편집기 어디에도 9px 이 남지 않는다', () => {
    // 소스를 훑는 것이 정직한 방식이다: 클래스가 실제로 만드는 픽셀 크기는 jsdom 이
    // 계산해 주지 않으므로(Tailwind 를 돌리지 않는다) 렌더 결과로는 잴 수 없고,
    // 여기서 막으려는 것은 "다시 9px 을 적는 일" 그 자체다. 주변 설정 절
    // (`ChartPanelSections.tsx`)에는 9px 이 한 군데도 없다.
    for (const file of ['CanvasElementsEditor.tsx', 'CanvasRuleTableEditor.tsx']) {
      const source = readFileSync(join(__dirname, file), 'utf-8');
      expect(source).not.toMatch(/text-\[9px\]/);
      expect(source).not.toMatch(/text-\[10px\]/);
    }
  });

  it('입력 칸의 기본 글자 크기가 주변과 같은 text-xs 다', () => {
    for (const file of ['CanvasElementsEditor.tsx', 'CanvasRuleTableEditor.tsx']) {
      const source = readFileSync(join(__dirname, file), 'utf-8');
      expect(source).toMatch(/const INPUT_CLASS =[\s\S]*?text-xs/);
    }
  });
});

// --- 요소 카드의 묶음 배치 (SPEC-CANVAS-002 · AC-E17 · AC-E21) --------------
//
// 사용 시험이 돌려보낸 것은 결함이 아니라 **카드가 읽히지 않는다** 였다. 칸들이 제목과
// 한 줄에 섞여 흐르니 어디까지가 한 묶음인지 눈으로 끊기지 않았다. 그래서 이 절이 재는
// 것은 값이 아니라 **구조** 다: 묶음이 어떤 차례로 서는가, 제목이 제 줄에 서는가,
// 종류마다 어느 묶음이 없는가.
//
// 일곱을 한 두루마리로 쌓던 것을 **탭 넷**으로 갈랐다. 그래서 차례를 재는 자리도 넷이
// 된다 — 각 탭이 제 묶음만 내는가, 그리고 그 안의 차례가 뜻대로인가. 묶음의 종류별
// 비대칭(line·text 에 크기 묶음이 없다)은 배치 탭 안으로 옮겨 갔을 뿐 그대로다.

/** 카드에 설 수 있는 묶음 이름 — 화면에 서는 차례 그대로다. */
const GROUP_NAMES = [
  'zorder',
  'size',
  'position',
  'shape-style',
  'text-style',
  'data',
  'tween',
  'rules',
];

/** 색 스와치 칸 이름(도형 축 · 텍스트 축 양쪽에 나뉘어 선다). */
const SWATCH_NAMES = ['fill', 'stroke', 'text-color'];

/** 어떤 상자 안에 있는 testid 를 **DOM 차례대로** 뽑는다. */
function testIdsIn(root: HTMLElement, names: readonly string[], idx = 0): string[] {
  const wanted = new Set(names.map((n) => `canvas-element-${n}-${idx}`));
  return [...root.querySelectorAll('[data-testid]')]
    .map((n) => n.getAttribute('data-testid') ?? '')
    .filter((id) => wanted.has(id))
    .map((id) => id.slice('canvas-element-'.length, id.length - `-${idx}`.length));
}

/** 한 요소 카드에 선 묶음들을 화면 차례대로 낸다. */
function groupOrder(idx = 0): string[] {
  return testIdsIn(testid(`canvas-element-${idx}`), GROUP_NAMES, idx);
}

/** 한 묶음 안의 색 스와치를 화면 차례대로 낸다. */
function swatchOrder(group: string, idx = 0): string[] {
  return testIdsIn(testid(`canvas-element-${group}-${idx}`), SWATCH_NAMES, idx);
}

describe('CanvasElementsEditor — 요소 카드는 묶음을 탭 넷에 나눠 담는다', () => {
  it('rect 의 네 탭은 각각 제 묶음만 낸다', () => {
    setup(cfg([rect()]));

    // 스타일 탭 — 도형 하나.
    expect(groupOrder()).toEqual(['shape-style']);

    openTab('text');
    expect(groupOrder()).toEqual(['text-style']);

    // 배치 탭 — 순서 → 크기 → 위치. 얼마만큼인지를 정하고 나서 어디인지를 정한다.
    openTab('arrange');
    expect(groupOrder()).toEqual(['zorder', 'size', 'position']);

    // 데이터 탭 — 바인딩(숫자 스위치 포함) → 전환 효과 → 규칙.
    openTab('data');
    expect(groupOrder()).toEqual(['data', 'tween', 'rules']);
  });

  it('ellipse 도 같은 넷이다', () => {
    setup(cfg([rect({ id: 'e', kind: 'ellipse' })]));
    expect(groupOrder()).toEqual(['shape-style']);
    openTab('arrange');
    expect(groupOrder()).toEqual(['zorder', 'size', 'position']);
    openTab('data');
    expect(groupOrder()).toEqual(['data', 'tween', 'rules']);
  });

  it('line 의 배치 탭에는 크기 묶음이 없다 — 길이는 끝점에서 따라 나오는 값이다', () => {
    setup(cfg([{ id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} }]), {
      tab: 'arrange',
    });

    expect(groupOrder()).toEqual(['zorder', 'position']);
  });

  it('text 의 배치 탭에도 크기 묶음이 없다 — 그 크기는 텍스트 탭의 글자 크기다', () => {
    setup(cfg([{ id: 't', kind: 'text', geometry: { x: 250, y: 200 }, style: {} }]), {
      tab: 'arrange',
    });

    expect(groupOrder()).toEqual(['zorder', 'position']);

    openTab('text');
    expect(
      testid('canvas-element-text-style-0').contains(testid('canvas-element-font-size-0')),
    ).toBe(true);
  });

  it('묶음 제목은 **제 줄**에 서고 칸은 그 아래 줄에 선다', () => {
    // 제목과 칸이 한 흐름에 섞여 있으면, 칸이 넘쳐 다음 줄로 내려간 순간 그 칸은 아래
    // 묶음의 제목과 나란히 서고 화면은 "크기 · [W] [H] [테두리]" 처럼 읽힌다. 그래서
    // 재는 것은 글자가 아니라 **줄이 둘로 갈렸는가** 다: 제목 줄과 칸 줄이 서로 다른
    // 상자이고, 제목 줄에는 칸이 하나도 없으며, 칸은 전부 두 번째 줄에 있다.
    // 탭마다 열어 그 탭의 묶음을 잰다. 순서 묶음(zorder)은 칸이 아니라 단추를 이므로
    // 아래 "칸 줄에 input/select 가 있다" 단언의 대상이 아니다 — 그 묶음의 두 줄 형상은
    // 바로 아래 시험이 따로 잰다.
    setup(cfg([rect()]));
    const perTab: [TabName, string[]][] = [
      ['style', ['shape-style']],
      ['text', ['text-style']],
      ['arrange', ['size', 'position']],
      ['data', ['data', 'tween']],
    ];

    for (const [tab, groups] of perTab) {
      openTab(tab);
      for (const group of groups) {
      const box = testid(`canvas-element-${group}-0`);
      const rows = [...box.children];
      expect(rows).toHaveLength(2);

      const [heading, fields] = rows as [Element, Element];
      // 제목 줄: 글자와 (있다면) `?` 단추뿐 — 저술하는 칸은 하나도 없다.
      expect(heading.textContent).not.toBe('');
      expect(heading.querySelectorAll('input,select')).toHaveLength(0);
      // 칸 줄: 이 묶음의 칸이 **전부** 여기 있다.
      expect(fields.querySelectorAll('input,select').length).toBeGreaterThan(0);
      expect(box.querySelectorAll('input,select')).toHaveLength(
        fields.querySelectorAll('input,select').length,
      );
      }
    }
  });

  it('순서 묶음도 같은 두 줄이다 — 제목 줄에는 단추가 하나도 없다', () => {
    setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]), { tab: 'arrange' });

    const box = testid('canvas-element-zorder-0');
    const rows = [...box.children];
    expect(rows).toHaveLength(2);

    const [heading, fields] = rows as [Element, Element];
    expect(heading.textContent).toBe('dashboard.canvas.elements.zorderLabel');
    expect(heading.querySelectorAll('button')).toHaveLength(0);
    expect(fields.querySelectorAll('button')).toHaveLength(4);
  });

  it('세부는 여전히 접힌다 — 탭 줄까지 함께 접힌다', () => {
    setup(cfg([rect()]), { expand: false });

    expect(groupOrder()).toEqual([]);
    expect(testid('canvas-element-order-0').textContent).toBe('1');
    // 머리줄에 남는 저술 도구는 없다. 종류는 스타일 탭 안에 있으므로 함께 접힌다.
    expect(screen.getByTestId('canvas-element-toggle-0')).toBeTruthy();
    expect(screen.queryByTestId('canvas-element-kind-0')).toBeNull();
    // 탭 줄도 몸통이다 — 접힌 카드에 탭만 남으면 누를 것이 있는 접힌 카드가 된다.
    expect(screen.queryByTestId('canvas-element-tabs-0')).toBeNull();
  });
});

describe('CanvasElementsEditor — 색 스와치는 종류를 따라 자리를 옮긴다', () => {
  it('도형에서는 스타일 탭에 채우기 · 테두리 둘, 텍스트 탭에 글자색 하나다', () => {
    setup(cfg([rect()]));

    // 차례도 뜻이다 — 참고 화면의 차례를 따라 **채우기 다음 테두리**이며, 테두리 두께는
    // 제 색 바로 뒤에 붙는다(한 가지를 두 자리에서 만지지 않는다).
    expect(swatchOrder('shape-style')).toEqual(['fill', 'stroke']);

    openTab('text');
    expect(swatchOrder('text-style')).toEqual(['text-color']);
  });

  it('선도 같다 — 채우기는 선 자체에 뜻이 없어도 규칙 패치가 쓸 수 있는 축이다', () => {
    setup(cfg([{ id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 }, style: {} }]));
    expect(swatchOrder('shape-style')).toEqual(['fill', 'stroke']);
  });

  it("kind:'text' 에서는 채우기가 글자색 옆으로 간다 (textColor ?? fill)", () => {
    setup(cfg([{ id: 't', kind: 'text', geometry: { x: 0, y: 0 }, style: {} }]));

    expect(swatchOrder('shape-style')).toEqual(['stroke']);

    openTab('text');
    expect(swatchOrder('text-style')).toEqual(['text-color', 'fill']);
  });
});

describe('CanvasElementsEditor — 종류는 머리줄이 아니라 도형 묶음의 첫 칸이다', () => {
  /** 도형 묶음의 칸 이름들 — 화면 차례를 재기 위한 목록이다. */
  const SHAPE_FIELD_NAMES = ['kind', 'fill', 'stroke', 'stroke-width', 'opacity', 'visible'];

  it('종류 선택기는 도형 묶음 안에 서고 머리줄에는 없다', () => {
    setup(cfg([rect()]));

    const shape = testid('canvas-element-shape-style-0');
    expect(shape.contains(testid('canvas-element-kind-0'))).toBe(true);

    // 머리줄은 요소 카드의 첫 자식이다. 종류가 거기 남아 있으면 접힌 줄에서도 저술이
    // 가능해지고, "접으면 세부가 사라진다" 는 규율이 종류 하나에서만 깨진다.
    const header = testid('canvas-element-0').firstElementChild as HTMLElement;
    expect(header.contains(testid('canvas-element-toggle-0'))).toBe(true);
    expect(header.contains(testid('canvas-element-kind-0'))).toBe(false);
  });

  it('스타일 탭의 차례는 종류 · 채우기 · 테두리 · 두께 · 투명 · 표시 여부다', () => {
    // 차례가 곧 뜻이다: 무엇으로 그릴지(종류)를 먼저 정하고, 어떻게 그릴지(채우기·테두리·
    // 두께·투명)를 정한 다음, 그릴지 말지(표시 여부)를 끝에서 정한다. 가운데 넷의 차례는
    // 참고 화면을 따르며, 두께가 제 색 바로 뒤에 오는 것이 요점이다.
    setup(cfg([rect()]));

    expect(testIdsIn(testid('canvas-element-shape-style-0'), SHAPE_FIELD_NAMES)).toEqual([
      'kind',
      'fill',
      'stroke',
      'stroke-width',
      'opacity',
      'visible',
    ]);
  });

  it("kind:'text' 에서도 종류가 앞, 표시 여부가 뒤다 — 빠지는 것은 채우기뿐이다", () => {
    setup(cfg([{ id: 't', kind: 'text', geometry: { x: 0, y: 0 }, style: {} }]));

    expect(testIdsIn(testid('canvas-element-shape-style-0'), SHAPE_FIELD_NAMES)).toEqual([
      'kind',
      'stroke',
      'stroke-width',
      'opacity',
      'visible',
    ]);
  });

  it('머리줄은 순번 · 요약 토글 · 순서 이동 · 삭제를 그대로 지닌다', () => {
    setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]));

    const header = testid('canvas-element-0').firstElementChild as HTMLElement;
    for (const id of ['order', 'toggle', 'move-up', 'move-down', 'delete']) {
      expect(header.contains(testid(`canvas-element-${id}-0`))).toBe(true);
    }
  });

  it('새 자리에서 종류를 바꿔도 기하는 전과 같이 새 형상으로 다시 쓰인다', () => {
    // 자리만 옮겼다 — 부르는 함수(`withKind`)는 그대로다. 옮기다 `onChange` 를 다시
    // 적으면 기하 이관이 조용히 빠질 수 있으므로, 이음매를 여기서 한 번 더 건넌다.
    const spy = setup(cfg([rect()]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'line' } });
    expect(lastElements(spy)[0]!.kind).toBe('line');
    expect(lastElements(spy)[0]!.geometry).toEqual({ x1: 50, y1: 80, x2: 450, y2: 200 });
  });

  it('종류를 바꾼 뒤에도 그 줄은 펼친 채로 남는다 — 바꾼 칸이 손 밑에서 사라지면 안 된다', () => {
    // 종류가 몸통 안으로 들어온 뒤 새로 생긴 위험이다: 종류를 바꾸면 요소가 갈아끼워지는데
    // 펼침이 순번을 따라간다면 그 순간 줄이 접혀 방금 쓴 칸이 손 밑에서 사라진다.
    // 펼침은 요소 id 를 따라가므로(위 `toggleExpanded`) 그렇지 않다.
    setupStateful(cfg([rect()]));

    fireEvent.change(testid('canvas-element-kind-0'), { target: { value: 'ellipse' } });

    expect(testid('canvas-element-toggle-0').getAttribute('aria-expanded')).toBe('true');
    expect(testid('canvas-element-shape-style-0').contains(testid('canvas-element-kind-0'))).toBe(
      true,
    );
  });
});

// --- 요소 카드의 탭 (SPEC-CANVAS-002 · AC-E21) -----------------------------
//
// 탭은 **재배치**다. 그래서 이 절이 재는 것은 새 능력이 아니라 셋이다:
//   1. 열린 탭의 칸만 서고 다른 탭의 칸은 DOM 에 아예 없다(감춘 것이 아니라 그리지 않는다).
//   2. 탭을 갈아도 저술한 값이 살아 있다 — 제어 컴포넌트가 config 를 다시 읽어 그리므로,
//      갈아 낀 뒤 값이 비면 그것은 곧 저술이 config 에 닿지 않았다는 뜻이다.
//   3. 어느 탭이 열려 있는지는 **config 에 적히지 않는다**(펼침 여부와 같은 부류의 보기
//      상태다). 한 사람이 연 탭이 대시보드를 함께 보는 모두의 저장된 값이 되면 안 된다.

/** 단추 하나가 막혀 있는가. */
function tabDisabled(suffix: string): boolean {
  return (testid(`canvas-element-${suffix}`) as HTMLButtonElement).disabled;
}

/** 탭 단추 하나. */
function tabBtn(tab: TabName, idx = 0): HTMLButtonElement {
  return testid(`canvas-element-tab-${tab}-${idx}`) as HTMLButtonElement;
}

describe('CanvasElementsEditor — 요소 카드의 탭', () => {
  it('열린 탭의 칸만 서고 다른 탭의 칸은 DOM 에 없다', () => {
    setup(cfg([rect()]));

    // 스타일 탭(기본)
    expect(screen.getByTestId('canvas-element-kind-0')).toBeTruthy();
    for (const id of ['text-0', 'geo-x-0', 'binding-0']) {
      expect(screen.queryByTestId(`canvas-element-${id}`)).toBeNull();
    }

    openTab('text');
    expect(screen.getByTestId('canvas-element-text-0')).toBeTruthy();
    for (const id of ['kind-0', 'geo-x-0', 'binding-0']) {
      expect(screen.queryByTestId(`canvas-element-${id}`)).toBeNull();
    }

    openTab('arrange');
    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeTruthy();
    for (const id of ['kind-0', 'text-0', 'binding-0']) {
      expect(screen.queryByTestId(`canvas-element-${id}`)).toBeNull();
    }

    openTab('data');
    expect(screen.getByTestId('canvas-element-binding-0')).toBeTruthy();
    for (const id of ['kind-0', 'text-0', 'geo-x-0']) {
      expect(screen.queryByTestId(`canvas-element-${id}`)).toBeNull();
    }
  });

  it('탭을 갈아 끼워도 저술한 값이 그대로 보인다', () => {
    // 제어 컴포넌트의 회귀가 여기서 드러난다: 값이 config 로 올라가지 않았다면 탭을 떠났다
    // 돌아온 순간 칸이 원래 값으로 되돌아간다.
    setupStateful(cfg([rect()]));

    fireEvent.change(testid('canvas-element-opacity-0'), { target: { value: '0.25' } });
    openTab('text');
    fireEvent.change(testid('canvas-element-font-size-0'), { target: { value: '19' } });
    openTab('arrange');
    fireEvent.change(testid('canvas-element-geo-x-0'), { target: { value: '123' } });

    openTab('style');
    expect((testid('canvas-element-opacity-0') as HTMLInputElement).value).toBe('0.25');
    openTab('text');
    expect((testid('canvas-element-font-size-0') as HTMLInputElement).value).toBe('19');
    openTab('arrange');
    expect((testid('canvas-element-geo-x-0') as HTMLInputElement).value).toBe('123');
  });

  it('열린 탭은 요소마다가 아니라 편집기 하나에 하나다 (공유)', () => {
    // 좌표를 줄줄이 손보려고 요소를 옮겨 다닐 때 카드마다 다른 탭이 열려 있으면 매번 같은
    // 탭을 다시 골라야 한다. 하나로 두면 "배치를 보고 있다" 가 요소를 건너가도 유지된다.
    setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]));

    openTab('arrange', 1);

    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-geo-x-1')).toBeTruthy();
    expect(tabBtn('arrange', 0).getAttribute('aria-selected')).toBe('true');
    expect(tabBtn('arrange', 1).getAttribute('aria-selected')).toBe('true');
  });

  it('어느 탭을 열었는지는 config 에 적히지 않는다', () => {
    const spy = setup(cfg([rect()]));

    openTab('arrange');
    openTab('data');
    openTab('text');

    // 탭만 눌렀을 때는 패치 자체가 나가지 않는다.
    expect(spy).not.toHaveBeenCalled();

    // 저술이 일어난 뒤에도 패치에는 요소 배열뿐이다 — 탭 이름이 실려 나갈 자리가 없다.
    fireEvent.change(testid('canvas-element-text-0'), { target: { value: 'hi' } });
    const patch = lastPatch(spy);
    expect(Object.keys(patch)).toEqual(['elements']);
    expect(JSON.stringify(patch)).not.toContain('tab');
    expect(lastElements(spy)[0]!.text).toBe('hi');
  });

  it('진짜 탭이다 — 역할 · 선택 표시 · 판 연결이 갖춰져 있다', () => {
    setup(cfg([rect()]));

    const list = testid('canvas-element-tabs-0');
    expect(list.getAttribute('role')).toBe('tablist');
    expect(list.getAttribute('aria-label')).toBe('dashboard.canvas.elements.tabsAria');

    const panel = testid('canvas-element-tabpanel-0');
    expect(panel.getAttribute('role')).toBe('tabpanel');

    for (const tab of ['style', 'text', 'arrange', 'data'] as const) {
      const btn = tabBtn(tab);
      expect(btn.getAttribute('role')).toBe('tab');
      // 판을 가리키는 손가락이 있어야 스크린리더가 탭과 내용을 잇는다.
      expect(btn.getAttribute('aria-controls')).toBe(panel.id);
      expect(btn.getAttribute('aria-selected')).toBe(tab === 'style' ? 'true' : 'false');
      // roving tabindex — 탭 줄 전체가 하나의 정지점이고 내부 이동은 화살표가 맡는다.
      expect(btn.tabIndex).toBe(tab === 'style' ? 0 : -1);
    }
    // 열린 탭이 판의 이름표다.
    expect(panel.getAttribute('aria-labelledby')).toBe(tabBtn('style').id);
  });

  it('좌우 화살표로 옮겨 다니고 양끝에서 순환한다 — 초점도 함께 간다', () => {
    setup(cfg([rect()]));

    tabBtn('style').focus();
    fireEvent.keyDown(tabBtn('style'), { key: 'ArrowRight' });
    expect(tabBtn('text').getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(tabBtn('text'));
    expect(screen.getByTestId('canvas-element-text-0')).toBeTruthy();

    fireEvent.keyDown(tabBtn('text'), { key: 'ArrowRight' });
    fireEvent.keyDown(tabBtn('arrange'), { key: 'ArrowRight' });
    expect(tabBtn('data').getAttribute('aria-selected')).toBe('true');

    // 끝에서 한 번 더 — 처음으로 돌아온다(WAI-ARIA tabs 패턴).
    fireEvent.keyDown(tabBtn('data'), { key: 'ArrowRight' });
    expect(tabBtn('style').getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(tabBtn('style'));

    // 왼쪽도 같다 — 처음에서 왼쪽이면 끝으로 감긴다.
    fireEvent.keyDown(tabBtn('style'), { key: 'ArrowLeft' });
    expect(tabBtn('data').getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(tabBtn('data'));
  });

  it('다른 키는 탭을 옮기지 않는다 — 위/아래는 이 줄의 것이 아니다', () => {
    setup(cfg([rect()]));

    fireEvent.keyDown(tabBtn('style'), { key: 'ArrowDown' });
    fireEvent.keyDown(tabBtn('style'), { key: 'Enter' });

    expect(tabBtn('style').getAttribute('aria-selected')).toBe('true');
  });

  it('초점 테두리가 보인다 — 화살표가 어디에 닿았는지 알 수 있는 유일한 표시다', () => {
    setup(cfg([rect()]));
    expect(tabBtn('arrange').className).toMatch(/focus-visible:ring/);
  });
});

// --- 배치 탭의 순서(z-order) 네 동작 (SPEC-CANVAS-002 · AC-E21) -------------
//
// 넷은 새 기능이 아니라 **이미 있던 동작의 노출**이다: 규칙은 여전히
// `canvasEditArrange.moveElementTo` 하나이고, 맨 앞/맨 뒤는 그 함수로 지어진
// `bringToFront`/`sendToBack` 이다(같은 모듈 §z-order). 그래서 여기서 재는 것은 네 단추가
// **같은 규칙을 지나 같은 결과에 닿는가** 이며, 머리줄의 위/아래 이동과 결과가 갈릴 수
// 없다는 사실을 마지막 시험이 못박는다.

describe('CanvasElementsEditor — 배치 탭의 순서 네 동작', () => {
  const three = () => cfg([rect({ id: 'a' }), rect({ id: 'b' }), rect({ id: 'c' })]);

  it('맨 앞으로 보내면 배열 끝(= 맨 위)으로 간다', () => {
    const spy = setup(three(), { tab: 'arrange' });
    fireEvent.click(testid('canvas-element-zorder-front-0'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['b', 'c', 'a']);
  });

  it('맨 뒤로 보내면 배열 앞(= 맨 아래)으로 간다', () => {
    const spy = setup(three(), { tab: 'arrange' });
    fireEvent.click(testid('canvas-element-zorder-back-2'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['c', 'a', 'b']);
  });

  it('한 칸 앞으로 · 한 칸 뒤로는 이웃과 자리를 바꾼다', () => {
    const spy = setup(three(), { tab: 'arrange' });

    fireEvent.click(testid('canvas-element-zorder-forward-0'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['b', 'a', 'c']);

    fireEvent.click(testid('canvas-element-zorder-backward-2'));
    expect(lastElements(spy).map((e) => e.id)).toEqual(['a', 'c', 'b']);
  });

  it('갈 곳이 없는 단추는 막힌다 — 눌러도 아무 일도 없는 단추를 남기지 않는다', () => {
    setup(three(), { tab: 'arrange' });

    // 맨 아래 요소: 뒤로 갈 곳이 없다.
    expect(tabDisabled('zorder-back-0')).toBe(true);
    expect(tabDisabled('zorder-backward-0')).toBe(true);
    expect(tabDisabled('zorder-front-0')).toBe(false);
    expect(tabDisabled('zorder-forward-0')).toBe(false);

    // 맨 위 요소: 앞으로 갈 곳이 없다.
    expect(tabDisabled('zorder-front-2')).toBe(true);
    expect(tabDisabled('zorder-forward-2')).toBe(true);
    expect(tabDisabled('zorder-back-2')).toBe(false);
    expect(tabDisabled('zorder-backward-2')).toBe(false);
  });

  it('머리줄의 위/아래 이동과 같은 결과에 닿는다 — 규칙이 하나이기 때문이다', () => {
    // 두 입구가 있는 것은 뜻이 있다: 머리줄의 둘은 **접힌 채로** 목록을 훑으며 쓰는 것이고,
    // 배치 탭의 넷은 카드를 펼쳐 한 요소를 다루는 동안의 온전한 벌이다. 두 입구가 같은
    // 함수를 지나므로 어느 쪽으로 눌렀는지에 따라 결과가 달라질 수 없다.
    const viaTab = setup(three(), { tab: 'arrange' });
    fireEvent.click(testid('canvas-element-zorder-forward-0'));
    const byTab = lastElements(viaTab).map((e) => e.id);

    cleanup();

    const viaHeader = setup(three(), { expand: false });
    fireEvent.click(testid('canvas-element-move-down-0'));
    expect(lastElements(viaHeader).map((e) => e.id)).toEqual(byTab);
  });

  it('순서를 바꿔도 그 요소의 카드는 펼친 채로 남는다 — 방금 누른 단추가 손 밑에서 사라지지 않는다', () => {
    // 펼침은 순번이 아니라 요소 id 를 따라간다. 순서 조작이 카드를 접으면 연달아 누르는
    // 동안 단추가 다른 요소의 것으로 바뀐다.
    setupStateful(three(), { tab: 'arrange' });

    fireEvent.click(testid('canvas-element-zorder-front-0'));

    // a 가 끝으로 갔다 — 그 자리(순번 3)의 카드가 여전히 펼쳐져 있어야 한다.
    expect(testid('canvas-element-2').getAttribute('data-element-id')).toBe('a');
    expect(testid('canvas-element-toggle-2').getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByTestId('canvas-element-zorder-front-2')).toBeTruthy();
  });
});

// --- 숫자 스위치 (SPEC-CANVAS-002 · AC-E17) -------------------------------

describe('CanvasElementsEditor — 숫자 스위치', () => {
  it('기본은 켜짐이다 — config 에 키가 없는 기존 요소도 숫자로 읽는다', () => {
    setup(cfg([rect()]), { tab: 'data' });
    expect((testid('canvas-element-numeric-0') as HTMLInputElement).checked).toBe(true);
  });

  it('끄면 numeric:false 가 실린다', () => {
    const spy = setup(cfg([rect()]), { tab: 'data' });

    fireEvent.click(testid('canvas-element-numeric-0'));

    expect(lastElements(spy)[0]!.numeric).toBe(false);
  });

  it('다시 켜면 **키 자체가 사라진다** — 기본값은 부재로 적는다(규율 1)', () => {
    const live = setupStateful(cfg([rect({ numeric: false } as Partial<CanvasElement>)]), { tab: 'data' });
    expect((testid('canvas-element-numeric-0') as HTMLInputElement).checked).toBe(false);

    fireEvent.click(testid('canvas-element-numeric-0'));

    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el).not.toHaveProperty('numeric');
  });

  it('켜져 있으면 소수 자리 · 단위 칸이 함께 선다', () => {
    setup(cfg([rect()]), { tab: 'data' });

    expect(screen.getByTestId('canvas-element-numeric-fields-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-decimals-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-unit-0')).toBeTruthy();
  });

  it('끄면 그 두 칸이 사라진다 — 만져도 아무 일도 없는 칸을 남기지 않는다', () => {
    setup(cfg([rect({ numeric: false, decimals: 2, unit: '℃' } as Partial<CanvasElement>)]), { tab: 'data' });

    expect(screen.queryByTestId('canvas-element-numeric-fields-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-decimals-0')).toBeNull();
    expect(screen.queryByTestId('canvas-element-unit-0')).toBeNull();
  });

  it('감춘다고 값을 지우지는 않는다 — 다시 켜면 적어 둔 소수 자리 · 단위가 그대로다', () => {
    const live = setupStateful(
      cfg([rect({ numeric: false, decimals: 2, unit: '℃' } as Partial<CanvasElement>)]),
    { tab: 'data' },
    );

    fireEvent.click(testid('canvas-element-numeric-0'));

    expect((testid('canvas-element-decimals-0') as HTMLInputElement).value).toBe('2');
    expect((testid('canvas-element-unit-0') as HTMLInputElement).value).toBe('℃');
    const el = (live.config.elements as CanvasElement[])[0]!;
    expect(el.decimals).toBe(2);
    expect(el.unit).toBe('℃');
  });

  it('스위치와 그 설명은 데이터 탭의 바인딩 묶음 안에 선다 — 값을 어떻게 읽는가는 무엇을 읽는가와 한 결정이다', () => {
    // 한때 텍스트 묶음에 있었다. 그때의 근거("읽은 값이 글자가 된다")보다 지금의 근거가
    // 가깝다 — 이 스위치가 정하는 것은 글자의 모양이 아니라 **판독값을 어떤 값으로 읽을
    // 것인가**이고, 그 값을 어디서 받는지를 정하는 칸이 바로 위에 있다.
    setup(cfg([rect()]), { tab: 'data' });

    const group = testid('canvas-element-data-0');
    expect(group.contains(testid('canvas-element-binding-0'))).toBe(true);
    expect(group.contains(testid('canvas-element-numeric-0'))).toBe(true);
    expect(group.contains(testid('canvas-element-numeric-fields-0'))).toBe(true);
    expect(testid('canvas-element-numeric-help-0').textContent).toBe(
      'dashboard.canvas.elements.numericHint',
    );

    // 텍스트 탭에는 없다 — 같은 스위치가 두 자리에 서면 어느 쪽이 진짜인지 알 수 없다.
    openTab('text');
    expect(screen.queryByTestId('canvas-element-numeric-0')).toBeNull();
  });

  it('요소마다 따로다 — 한 줄을 꺼도 옆 줄은 켜진 채다', () => {
    const spy = setup(cfg([rect({ id: 'a' }), rect({ id: 'b' })]), { tab: 'data' });

    fireEvent.click(testid('canvas-element-numeric-1'));

    const els = lastElements(spy);
    expect(els[0]).not.toHaveProperty('numeric');
    expect(els[1]!.numeric).toBe(false);
  });

  it('왕복에서 그대로다 — 끈 요소를 파서로 다시 읽어도 같은 것이 나온다', () => {
    const live = setupStateful(cfg([rect({ text: '{value}', unit: '℃' } as Partial<CanvasElement>)]), { tab: 'data' });

    fireEvent.click(testid('canvas-element-numeric-0'));

    const parsed = parseCanvasConfig(live.config);
    expect(parsed.elements[0]!.numeric).toBe(false);
    expect(parseCanvasConfig(parsed)).toEqual(parsed);
  });
});

describe('CanvasElementsEditor — 숫자를 끄면 규칙 표가 그 사실을 말한다', () => {
  it('켜져 있을 때는 잔소리하지 않는다', () => {
    setup(cfg([rect()]), { tab: 'data' });
    expect(screen.queryByTestId('canvas-element-rules-nonnumeric-help-0')).toBeNull();
  });

  it('끄면 규칙 묶음 제목 뒤 ? 가 왜 비교 행이 일치하지 않는지 말한다', () => {
    // 값이 오고 있는데도 스칼라 행이 전부 빗나가는 상태는 사용자에게 **고장으로 보인다**.
    // 화면이 스스로 이유를 말하지 않으면 그 다음 보고서는 "규칙이 안 먹는다" 가 된다.
    setup(cfg([rect({ numeric: false } as Partial<CanvasElement>)]), { tab: 'data' });

    const help = testid('canvas-element-rules-nonnumeric-help-0');
    expect(help.textContent).toBe('dashboard.canvas.elements.rulesNonNumericHint');
    expect(testid('canvas-element-rules-0').contains(help)).toBe(true);
  });

  it('그 설명은 줄로 깔지 않고 눌러야 보인다 — 이 화면 전체의 규칙이다', () => {
    setup(cfg([rect({ numeric: false } as Partial<CanvasElement>)]), { tab: 'data' });

    const heading = testid('canvas-element-rules-0').firstElementChild!;
    const button = heading.querySelector('button[aria-expanded]');
    expect(button).toBeTruthy();
    expect(button!.getAttribute('aria-expanded')).toBe('false');

    fireEvent.click(button!);
    expect(screen.getByRole('tooltip').textContent).toBe(
      'dashboard.canvas.elements.rulesNonNumericHint',
    );
  });
});
