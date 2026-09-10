// 경로 요소의 편집 이음매 — 목록 편집기 (SPEC-CANVAS-008 M5).
//
// M1 이 실측한 침묵 지점 가운데 **목록 편집기가 소유한 것**을 여기서 잰다: 기하 칸의 렌더
// 조건 셋(`rect|ellipse` · `line` · `text`)은 어느 것도 경로에 걸리지 않아, 경로 행에는
// **기하 칸이 하나도 없었다.** 컴파일러가 울 자리가 아니다 — 조건이 그냥 전부 거짓일 뿐이다.
//
// 그것이 가벼운 결함이 아닌 이유: 이 목록은 **캔버스 밖으로 나간 요소를 되찾는 마지막
// 경로**다. 좌표를 clamp 하지 않기로 한 규율(가정 A5)이 성립하려면 수치 입력이 반드시
// 있어야 한다.
//
// 함께 재는 것 둘:
//   - 종류 칸이 경로의 **이름을 말한다**(`ELEMENT_KINDS` 에는 없지만 `KIND_LABEL_KEY` 에는
//     있다 — AC-E10). 없으면 `select` 의 값이 어느 `option` 과도 맞지 않아 브라우저가 첫
//     칸(사각형)을 보여 주고, 그것은 거짓말이다.
//   - 경로에서 다른 종류로 바꾸는 길은 **열려 있고**(REQ-07), 바꿀 때 경로 전용 자료는
//     **따라가지 않는다**.
//
// i18n 은 키 통과 스텁이므로 개별 칸은 `data-testid` 로 집는다(기존 파일의 관용구).
//
// @spec SPEC-CANVAS-008 REQ-02 · REQ-07 · AC-E10 · D1

import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { CanvasElement, PathElement } from './canvasConfig';
import { DEFAULT_CANVAS_SIZE, parseCanvasConfig } from './canvasConfig';
import CanvasElementsEditor from './CanvasElementsEditor';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';

// --- 고정 입력 -----------------------------------------------------------

/** 비대칭 삼각형 — 대칭 도형은 축을 뒤바꾼 결함을 감춘다. */
const TRIANGLE: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

/** 원점 ≠ 0 · `x ≠ y` · 비정사각(D2·D3). 게다가 **캔버스 밖**으로 걸쳐 있다. */
function pathEl(overrides: Partial<PathElement> = {}): PathElement {
  return {
    id: 'p1',
    kind: 'path',
    geometry: { x: -37, y: 61, w: 160, h: 90 },
    path: [...TRIANGLE],
    catalog_id: 'rightTriangle',
    style: { fill: '#123456', opacity: 0.5 },
    text: '{value}',
    numeric: true,
    ...overrides,
  };
}

function cfg(elements: readonly CanvasElement[]): Record<string, unknown> {
  return { canvas: { ...DEFAULT_CANVAS_SIZE }, elements: [...elements] };
}

function setup(elements: readonly CanvasElement[], tab?: 'style' | 'arrange') {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={cfg(elements)} onConfigChange={onConfigChange} />);
  for (const btn of screen.queryAllByTestId(/^canvas-element-toggle-\d+$/)) fireEvent.click(btn);
  if (tab !== undefined) fireEvent.click(screen.getByTestId(`canvas-element-tab-${tab}-0`));
  return onConfigChange;
}

function lastElements(spy: ReturnType<typeof vi.fn>): CanvasElement[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
  return patch.elements as CanvasElement[];
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 기하 칸 --------------------------------------------------------------

describe('경로 행에도 기하 칸이 선다 (D1)', () => {
  it('rect 와 **같은 여섯 칸**(w·h·x·y)이 있다 — 하나도 없던 자리다', () => {
    setup([pathEl()], 'arrange');
    for (const axis of ['w', 'h', 'x', 'y']) {
      expect(screen.getByTestId(`canvas-element-geo-${axis}-0`), axis).toBeTruthy();
    }
    expect(screen.getByTestId('canvas-element-size-0')).toBeTruthy();
    expect(screen.getByTestId('canvas-element-position-0')).toBeTruthy();
  });

  it('칸에 지금 값이 실려 있다 — 음수 좌표도 그대로 보인다(캔버스 밖 저술)', () => {
    setup([pathEl()], 'arrange');
    const value = (axis: string) =>
      (screen.getByTestId(`canvas-element-geo-${axis}-0`) as HTMLInputElement).value;
    expect(value('x')).toBe('-37');
    expect(value('y')).toBe('61');
    expect(value('w')).toBe('160');
    expect(value('h')).toBe('90');
  });

  it('칸을 고치면 그 축만 바뀌고 **명령 목록과 출처는 살아남는다**', () => {
    const spy = setup([pathEl()], 'arrange');
    fireEvent.change(screen.getByTestId('canvas-element-geo-x-0'), { target: { value: '5' } });

    const out = lastElements(spy)[0] as PathElement;
    expect(out.geometry).toEqual({ x: 5, y: 61, w: 160, h: 90 });
    expect(out.path).toEqual(TRIANGLE);
    expect(out.catalog_id).toBe('rightTriangle');
  });

  it('선·문구의 칸은 서지 않는다 — 경로는 상자이지 선도 점도 아니다', () => {
    setup([pathEl()], 'arrange');
    for (const axis of ['x1', 'y1', 'x2', 'y2']) {
      expect(screen.queryByTestId(`canvas-element-geo-${axis}-0`), axis).toBeNull();
    }
  });
});

// --- 종류 칸 (AC-E10) -----------------------------------------------------

describe('종류 칸이 경로의 이름을 말한다 (AC-E10)', () => {
  it('지금 종류가 `path` 로 보이고 그 이름이 붙는다', () => {
    setup([pathEl()], 'style');
    const select = screen.getByTestId('canvas-element-kind-0') as HTMLSelectElement;
    // 이름이 없으면 브라우저가 첫 `option`(사각형)을 골라 보여 준다 — "이 줄은 사각형이다"
    // 라는 거짓말이며, 그것이 이 단언의 표적이다.
    expect(select.value).toBe('path');
    const current = [...select.options].find((o) => o.value === 'path');
    expect(current?.textContent).toBe('dashboard.canvas.elements.kindPath');
  });

  it('경로 칸은 **고를 수 없다** — 무엇으로부터 경로를 지어낼지에 답이 없다(REQ-07)', () => {
    setup([pathEl()], 'style');
    const select = screen.getByTestId('canvas-element-kind-0') as HTMLSelectElement;
    expect([...select.options].find((o) => o.value === 'path')?.disabled).toBe(true);
  });

  it('rect 행에는 그 칸이 **없다** — 선택지는 여전히 넷이다', () => {
    setup([{ id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} }], 'style');
    const select = screen.getByTestId('canvas-element-kind-0') as HTMLSelectElement;
    expect([...select.options].map((o) => o.value)).toEqual(['rect', 'ellipse', 'line', 'text']);
  });
});

// --- 경로에서 다른 종류로 ------------------------------------------------

describe('경로에서 다른 종류로 바꾸는 길은 열려 있다 (REQ-07)', () => {
  it('사각형으로 바꾸면 상자가 그대로 살아난다', () => {
    const spy = setup([pathEl()], 'style');
    fireEvent.change(screen.getByTestId('canvas-element-kind-0'), { target: { value: 'rect' } });

    const out = lastElements(spy)[0]!;
    expect(out.kind).toBe('rect');
    expect(out.geometry).toEqual({ x: -37, y: 61, w: 160, h: 90 });
  });

  it('스타일 · 문구 · 수치 해석은 그대로 남는다', () => {
    const spy = setup([pathEl()], 'style');
    fireEvent.change(screen.getByTestId('canvas-element-kind-0'), { target: { value: 'rect' } });

    const out = lastElements(spy)[0]!;
    expect(out.style).toEqual({ fill: '#123456', opacity: 0.5 });
    expect(out.text).toBe('{value}');
    expect(out.numeric).toBe(true);
  });

  it('**경로 전용 자료는 따라가지 않는다** — 사각형이 명령 목록을 업고 다니지 않는다', () => {
    // 전개(`...rest`)는 초과 속성 검사를 지나지 않으므로 타입이 이것을 막지 못한다. 파서는
    // 다음에 읽을 때 조용히 버리므로 **아무도 모르는 채로 snapshot 바이트만 먹는다.**
    const spy = setup([pathEl()], 'style');
    fireEvent.change(screen.getByTestId('canvas-element-kind-0'), { target: { value: 'rect' } });

    const out = lastElements(spy)[0]! as unknown as Record<string, unknown>;
    expect(out).not.toHaveProperty('path');
    expect(out).not.toHaveProperty('catalog_id');
  });

  it('선으로 바꾸면 좌상단이 첫 끝점이 된다(기존 규율 그대로)', () => {
    const spy = setup([pathEl()], 'style');
    fireEvent.change(screen.getByTestId('canvas-element-kind-0'), { target: { value: 'line' } });

    const out = lastElements(spy)[0]!;
    expect(out.kind).toBe('line');
    expect(out.geometry).toMatchObject({ x1: -37, y1: 61 });
  });
});

// --- 편집기 ↔ 파서 이음매 -------------------------------------------------

describe('편집기가 내보낸 것을 파서가 그대로 읽는다', () => {
  it('경로 요소를 담은 패치가 왕복을 지난다', () => {
    // 두 층이 각자 옳으면서 사이가 빈 적이 있다(이 파일 이웃의 머리말). 경로는 그 이음매를
    // 지나는 다섯 번째 종류이므로 여기서도 건넌다.
    const spy = setup([pathEl()], 'arrange');
    fireEvent.change(screen.getByTestId('canvas-element-geo-w-0'), { target: { value: '250' } });

    const patched = cfg(lastElements(spy));
    const parsed = parseCanvasConfig(JSON.parse(JSON.stringify(patched)));
    const el = parsed.elements[0] as PathElement;
    expect(el.kind).toBe('path');
    expect(el.geometry).toEqual({ x: -37, y: 61, w: 250, h: 90 });
    expect(el.path).toEqual(TRIANGLE);
    expect(el.catalog_id).toBe('rightTriangle');
  });
});
