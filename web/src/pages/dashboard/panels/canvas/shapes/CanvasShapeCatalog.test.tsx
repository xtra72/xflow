// 도형 카탈로그 팔레트 시험 (SPEC-CANVAS-008 M7).
//
// 이 파일이 겨누는 함정 셋.
//
// **하나 — 키 메아리 i18n**(시험 규율 D7). 이 저장소의 도크 시험들은 `t` 를 키 그대로
// 돌려주도록 대체해 왔고, 그 아래에서는 치환자 결함이 **드러나지 않는다** — 돌려받은
// 문자열에 `{shape}` 가 애초에 없기 때문이다. 그래서 여기서는 **진짜 번역**(`I18nProvider`
// + `ko.json`)으로 렌더하고, 치환자를 **두 번** 말하는 문구가 **두 자리 모두** 바뀌었는지
// 잰다. `replace` 는 첫 자리만 바꾼다.
//
// **둘 — 한 표면만 재는 시험**(시험 규율 D10 · 불변식 I23). 006 은 표시 층 셋을 조건 없이
// 그리면서 컨트롤 전부를 도크 뒤에 두어 결함을 배달했고, **어떤 시험도 울리지 않았다 — 두
// 조건이 같은 자리에서 비교된 적이 없었기 때문**이다. 그래서 아래 시험은 `docked` 를
// **갈아 끼우며** 관계를 잰다: 008 이 더한 노드 가운데 도크 밖에 있는 것이 하나도 없는가.
//
// **셋 — 그리지 않는 미리보기.** 칸에 `<canvas>` 가 있다는 것만 재면 그 캔버스가 텅 비어
// 있어도 초록이다. 그래서 2D context 를 기록 스텁으로 갈아 끼우고 **실제 렌더 경로가
// 무엇을 그렸는지** 잰다(REQ-06 — 미리보기는 `drawElements` 를 그대로 부른다).
//
// @spec SPEC-CANVAS-008 REQ-02 · REQ-06 · AC-03 · AC-E9 · AC-E10

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import ko from '@/lib/i18n/ko.json';

import { CanvasEditDockRegion } from '../CanvasEditDock';
import CanvasEditOverlay from '../CanvasEditOverlay';
import { DEFAULT_CANVAS_SIZE, type CanvasElement, type PathElement } from '../canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import type { CanvasProjection } from '../canvasGeometry';
import type { DrawContext2D } from '../drawElement';
import { PALETTE_STORAGE_KEY } from './paletteGroups';
import { SHAPE_CATALOG, SHAPE_GROUPS, findShape } from './shapeCatalog';
import type { CanvasNode } from '../group/groupTypes';

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 80 }, style: {} };
}

/**
 * 설정 미리보기의 형상을 흉내 낸다(`CanvasEditDock.test.tsx` 의 그것과 같은 꼴). 다른 것은
 * **i18n 을 대체하지 않는다**는 한 줄이며, 그 한 줄이 D7 을 재는 자리다.
 */
function Harness({
  initial,
  docked = true,
}: {
  initial: readonly CanvasElement[];
  docked?: boolean;
}) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  const overlay = (
    <div data-testid="scaled-panel">
      <CanvasEditOverlay
        enabled
        elements={elements}
        projection={PROJ}
        textWidths={{}}
        onElementsChange={setElements}
      />
    </div>
  );
  return (
    <I18nProvider>
      <CanvasEditSelectionContext value={state}>
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <span data-testid="dump">{JSON.stringify(elements)}</span>
        <CanvasEditDockRegion enabled={docked}>{overlay}</CanvasEditDockRegion>
      </CanvasEditSelectionContext>
    </I18nProvider>
  );
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

function selection(): string {
  return screen.getByTestId('selection').textContent ?? '';
}

/** 묶음 하나의 접힘을 **뒤집는다**. */
function toggleGroup(id: string): void {
  fireEvent.click(screen.getByTestId(`canvas-palette-group-${id}`));
}

/**
 * 묶음 하나를 **편다**. 이미 펴져 있으면 아무 일도 하지 않는다.
 *
 * 011 이 `기본` 을 펼친 채로 태어나게 했으므로(REQ-01) 무조건 누르는 몸짓은 그 묶음을
 * **닫는다**. "열고 잰다" 는 뜻을 몸짓이 아니라 결과로 적는다.
 */
function openGroup(id: string): void {
  const head = screen.getByTestId(`canvas-palette-group-${id}`);
  if (head.getAttribute('aria-expanded') === 'false') fireEvent.click(head);
}

// --- 미리보기 기록 스텁 --------------------------------------------------

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  return {
    calls,
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {
      calls.push(['beginPath']);
    },
    // 011 이 이 셋을 **무동작에서 기록으로** 바꿨다. 카탈로그 30종은 전부 `kind:'path'` 라
    // 세 함수를 한 번도 부르지 않으므로 기존 카탈로그 단언은 한 자도 달라지지 않고, 새로
    // 들어온 원시형 넷(rect·ellipse·text)은 이 셋을 지나지 않으면 잴 수가 없다.
    rect(x, y, w, h) {
      calls.push(['rect', x, y, w, h]);
    },
    ellipse(cx, cy, rx, ry) {
      calls.push(['ellipse', cx, cy, rx, ry]);
    },
    moveTo(x, y) {
      calls.push(['moveTo', x, y]);
    },
    lineTo(x, y) {
      calls.push(['lineTo', x, y]);
    },
    closePath() {
      calls.push(['closePath']);
    },
    bezierCurveTo(a, b, c, d, e, f) {
      calls.push(['bezierCurveTo', a, b, c, d, e, f]);
    },
    stroke() {
      calls.push(['stroke']);
    },
    fill() {
      calls.push(['fill']);
    },
    fillText(text, x, y) {
      calls.push(['fillText', text, x, y]);
    },
    measureText(text: string) {
      return { width: text.length * 10 };
    },
    clearRect() {
      calls.push(['clearRect']);
    },
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

/** 미리보기 캔버스마다 기록기를 하나씩 붙인다(도형 id 로 찾는다). */
function stubPreviewContexts(): Map<string, Recorder> {
  const byId = new Map<string, Recorder>();
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function (
    this: HTMLCanvasElement,
  ) {
    // 두 접두사를 모두 벗긴다 — 011 이후 원시형 넷도 같은 미리보기 부품을 쓰므로
    // (`canvas-palette-preview-*`), 카탈로그만 벗기면 넷의 기록을 집을 수 없다.
    const id = (this.dataset.testid ?? '')
      .replace('canvas-catalog-preview-', '')
      .replace('canvas-palette-preview-', '');
    const recorder = makeRecorder();
    byId.set(id, recorder);
    return recorder as unknown as CanvasRenderingContext2D;
  } as unknown as typeof HTMLCanvasElement.prototype.getContext);
  return byId;
}

beforeEach(() => {
  localStorage.clear();
  // jsdom 은 2D context 를 주지 않고 **부르는 자리마다 시끄럽게 운다**. 기본값을 `null` 로
  // 세워 두면 그 소음이 사라지고, 미리보기가 컨텍스트 없이도 서는지가 덤으로 재어진다.
  // 실제로 무엇을 그리는지 재는 시험은 아래에서 이 대체를 다시 갈아 끼운다.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

// --- 오늘의 팔레트가 그대로 있다 -----------------------------------------

describe('팔레트의 오늘이 그대로 있다 (AC-E10 · 회귀 · SPEC-CANVAS-011 REQ-01)', () => {
  it('원시형 넷은 같은 이름 · 같은 차례로 `기본` 묶음 맨 앞에 서고 나머지 둘은 접혀 있다', () => {
    render(<Harness initial={[]} />);

    // 이름과 **차례**를 함께 잰다. 이름만 재면 순서가 뒤바뀌어도 통과한다.
    const kinds = ['rect', 'ellipse', 'line', 'text'];
    const buttons = kinds.map((k) => screen.getByTestId(`canvas-palette-add-${k}`));
    const dock = screen.getByTestId('canvas-dock-panel');
    const all = [...dock.querySelectorAll('[data-testid^="canvas-palette-add-"]')];
    expect(all).toEqual(buttons);

    // **뒤집힌 단언 (SPEC-CANVAS-011 REQ-01 · AC-01).** 008 에서는 카탈로그 묶음 셋이 모두 접혀 있었다.
    // 011 이 원시형 넷을 `기본` 으로 옮기면서 그 묶음이 펼쳐진 채로 태어난다 — 008 이 위험
    // R10 의 답으로 세운 "자주 쓰는 넷이 열자마자 보인다" 를 자리만 옮겨 지키는 것이다.
    expect(screen.getByTestId('canvas-palette-group-basic').getAttribute('aria-expanded')).toBe(
      'true',
    );
    for (const group of SHAPE_GROUPS.filter((g) => g.id !== 'basic')) {
      expect(screen.getByTestId(`canvas-palette-group-${group.id}`)).toBeTruthy();
      expect(screen.queryByTestId(`canvas-palette-group-body-${group.id}`)).toBeNull();
      expect(
        screen.getByTestId(`canvas-palette-group-${group.id}`).getAttribute('aria-expanded'),
      ).toBe('false');
    }
    // **뒤집힌 단언 (SPEC-CANVAS-011 REQ-01 · AC-01).** 008 의 `원시형` 묶음은 **없다.**
    expect(screen.queryByTestId('canvas-palette-group-primitive')).toBeNull();

    // 접힌 묶음은 자식을 **아예 그리지 않는다** — 열릴 때 만들어지는 미리보기가 `기본` 몫뿐이다.
    const basicCells = SHAPE_GROUPS.find((g) => g.id === 'basic')?.entries.length ?? 0;
    expect(screen.queryAllByTestId(/^canvas-catalog-add-/)).toHaveLength(basicCells);
    expect(document.querySelectorAll('[data-testid^="canvas-catalog-preview-"]')).toHaveLength(
      basicCells,
    );
  });

  it('`기본` 묶음 몸통의 **첫 네 칸**이 사각형 · 타원 · 선 · 텍스트다 (011 AC-02)', () => {
    render(<Harness initial={[]} />);

    // 몸통 안에서 잰다 — 도크 전체로 재면 넷이 `기본` 밖에 서 있어도 통과한다.
    const body = screen.getByTestId('canvas-palette-group-body-basic');
    const ids = [
      ...body.querySelectorAll<HTMLElement>(
        '[data-testid^="canvas-palette-add-"],[data-testid^="canvas-catalog-add-"]',
      ),
    ].map((node) => node.dataset.testid ?? '');

    expect(ids.slice(0, 4)).toEqual([
      'canvas-palette-add-rect',
      'canvas-palette-add-ellipse',
      'canvas-palette-add-line',
      'canvas-palette-add-text',
    ]);
    // 그 뒤로 카탈로그 12종이 이어진다 — 넷은 격자 **위**가 아니라 격자의 첫 칸들이다.
    expect(ids).toHaveLength(4 + (SHAPE_GROUPS.find((g) => g.id === 'basic')?.entries.length ?? 0));
    expect(ids[4]).toBe('canvas-catalog-add-triangle');
  });

  it('그 네 칸이 카탈로그 칸과 **같은 격자의 자식**이다 — 한 묶음에 배치는 하나다', () => {
    render(<Harness initial={[]} />);

    // 차례만 재면 넷이 제 상자에 세로로 쌓여 있어도 통과한다 — 실제로 배달된 결함이 그것
    // 이었다(줄 버튼 넷 위, 2열 격자 서른 아래). 여기서 재는 것은 **부모가 하나**라는 사실이다.
    const body = screen.getByTestId('canvas-palette-group-body-basic');
    const cells = [
      ...body.querySelectorAll<HTMLElement>(
        '[data-testid^="canvas-palette-add-"],[data-testid^="canvas-catalog-add-"]',
      ),
    ];
    const parents = new Set(cells.map((node) => node.parentElement));
    expect(parents.size, '칸들의 부모가 둘 이상이다 — 격자가 갈라졌다').toBe(1);

    // 그 하나가 실제로 2열 격자다. 부모가 하나여도 그것이 세로 상자면 눈에는 여전히 줄이다.
    const grid = cells[0]?.parentElement;
    expect(grid?.className).toContain('grid-cols-2');

    // 겉모습도 한 벌이다 — 원시형 칸과 카탈로그 칸이 같은 클래스를 든다(`CELL_CLASS` 공유).
    const primitive = screen.getByTestId('canvas-palette-add-rect');
    const catalog = screen.getByTestId('canvas-catalog-add-triangle');
    expect(primitive.className).toBe(catalog.className);
  });

  it('네 칸도 카탈로그 칸과 **같은 미리보기 캔버스**를 든다 — glyph 가 아니다 (011)', () => {
    render(<Harness initial={[]} />);

    const catalogCanvas = screen
      .getByTestId('canvas-catalog-add-triangle')
      .querySelector('canvas');
    expect(catalogCanvas, '카탈로그 칸에 미리보기가 없다').not.toBeNull();

    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      const cell = screen.getByTestId(`canvas-palette-add-${kind}`);

      // ① 그림이 `<canvas>` 다. 011 이전에는 lucide `<svg>` 였고, 그래서 한 격자 안에
      //    윤곽선 글리프와 파란 도형이라는 두 벌의 잉크가 서 있었다.
      const canvas = cell.querySelector('canvas');
      expect(canvas, `${kind}: 미리보기 캔버스가 없다`).not.toBeNull();
      expect(cell.querySelector('svg'), `${kind}: glyph 가 남아 있다`).toBeNull();

      // ② 그 캔버스가 카탈로그의 것과 **같은 자리**다 — 뒷면 크기도 CSS 크기도.
      expect(canvas?.getAttribute('width'), kind).toBe(catalogCanvas?.getAttribute('width'));
      expect(canvas?.getAttribute('height'), kind).toBe(catalogCanvas?.getAttribute('height'));
      expect(canvas?.getAttribute('style'), kind).toBe(catalogCanvas?.getAttribute('style'));

      // ③ 장식은 이름을 나르지 않는다 — 008 의 a11y 가드가 훑는 그 속성이다.
      expect(canvas?.getAttribute('aria-hidden'), kind).toBe('true');
      expect(canvas?.getAttribute('data-testid'), kind).toBe(`canvas-palette-preview-${kind}`);
    }
  });

  it('그 넷이 실제로 **무엇을 그린다** — 빈 캔버스가 아니다 (011 · 시험 규율 셋)', () => {
    // 캔버스가 있다는 것만 재면 텅 빈 채로도 초록이다(이 파일 머리말의 함정 셋). 종류마다
    // 제 그리기 갈래를 지났는지 **기록으로** 잰다.
    const byId = stubPreviewContexts();
    render(<Harness initial={[]} />);

    const expected: Record<string, string> = {
      rect: 'rect',
      ellipse: 'ellipse',
      line: 'lineTo',
      text: 'fillText',
    };
    for (const [kind, call] of Object.entries(expected)) {
      const rec = byId.get(kind);
      expect(rec, `${kind} 미리보기가 그리지 않았다`).toBeDefined();
      if (rec === undefined) continue;
      const names = rec.calls.map((c) => c[0]);
      expect(names, `${kind} 의 그리기 갈래`).toContain(call);
    }

    // 문구 칸은 `T` 하나를 칸 한가운데에 놓는다 — 로케일과 무관한 그림이다(011 사용자 결정).
    const text = byId.get('text');
    const fillText = text?.calls.find((c) => c[0] === 'fillText');
    expect(fillText?.[1], '문구 미리보기가 그린 글자').toBe('T');
  });

  it('`기본` 묶음을 접으면 원시형 넷이 사라지고, 다시 펴면 넷이 돌아온다', () => {
    render(<Harness initial={[]} />);

    toggleGroup('basic');
    expect(screen.queryByTestId('canvas-palette-add-rect')).toBeNull();

    toggleGroup('basic');
    expect(screen.getByTestId('canvas-palette-add-rect')).toBeTruthy();
  });
});

// --- 카탈로그를 편다 ------------------------------------------------------

describe('카탈로그 묶음을 펴면 30칸이 선다 (REQ-02)', () => {
  it('묶음마다 제 정원의 칸이 서고, 셋을 다 펴면 30칸이다', () => {
    render(<Harness initial={[]} />);

    for (const group of SHAPE_GROUPS) {
      openGroup(group.id);
      const body = screen.getByTestId(`canvas-palette-group-body-${group.id}`);
      const cells = body.querySelectorAll('[data-testid^="canvas-catalog-add-"]');
      expect(cells.length, `${group.id} 묶음의 칸 수`).toBe(group.entries.length);
    }

    // 30종이 **한 종도 빠짐없이** 있다. 수만 재면 같은 도형이 두 번 서도 통과한다.
    for (const entry of SHAPE_CATALOG) {
      expect(screen.getByTestId(`canvas-catalog-add-${entry.id}`), entry.id).toBeTruthy();
    }
    expect(screen.queryAllByTestId(/^canvas-catalog-add-/)).toHaveLength(30);
  });

  it('접힘 상태는 기기 지역에 남고 다음 렌더가 그것을 읽는다 (REQ-06)', () => {
    render(<Harness initial={[]} />);
    toggleGroup('general');
    // **뒤집힌 단언 (SPEC-CANVAS-011 REQ-01).** 008 에서는 `primitive` 를 접었다. 그 묶음이
    // 없어졌으므로 원시형 넷을 품은 `기본` 을 접는다 — 재는 것(펴고 접은 둘이 그대로 돌아
    // 온다)은 같다.
    toggleGroup('basic');

    const stored = JSON.parse(localStorage.getItem(PALETTE_STORAGE_KEY) ?? '{}') as Record<
      string,
      boolean
    >;
    expect(stored.general).toBe(false);
    expect(stored.basic).toBe(true);
    expect(stored.primitive).toBeUndefined();

    cleanup();
    render(<Harness initial={[]} />);
    // 편 묶음은 펴진 채로, 접은 묶음은 접힌 채로 돌아온다.
    expect(screen.getByTestId('canvas-palette-group-body-general')).toBeTruthy();
    expect(screen.queryByTestId('canvas-palette-add-rect')).toBeNull();
  });

  it('저장소가 던져도 팔레트는 선다 — 잃는 것은 접힘 상태뿐이다 (REQ-06)', () => {
    // 시크릿 창 · 용량 초과 · 접근 불가. 여기서 예외가 밖으로 나가면 잃는 것은 접힘 상태가
    // 아니라 팔레트 전체다.
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('접근 불가');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('용량 초과');
    });

    render(<Harness initial={[]} />);
    expect(screen.getByTestId('canvas-palette-add-rect')).toBeTruthy();
    // 접힌 채 태어나는 묶음으로 잰다 — `기본` 은 011 이후 이미 펴져 있어 여는 몸짓이 없다.
    toggleGroup('general');
    expect(screen.getByTestId('canvas-palette-group-body-general')).toBeTruthy();
  });
});

// --- 놓기 ----------------------------------------------------------------

describe('카탈로그에서 놓은 도형은 일반 요소와 구별되지 않는다 (AC-03)', () => {
  it('배열 끝에 경로 요소가 붙고, id · 계단 · 씨앗 스타일이 원시형의 그 규칙이다', () => {
    render(<Harness initial={[rect('el-1')]} />);
    openGroup('basic');
    fireEvent.click(screen.getByTestId('canvas-catalog-add-star5'));

    const elements = liveElements();
    expect(elements).toHaveLength(2);
    const created = elements[1] as PathElement;
    // 끝에 붙는 것이 곧 맨 위다(배열 순서가 001 의 유일한 z-order 다).
    expect(created.kind).toBe('path');
    expect(created.id).toBe('el-2');
    expect(created.catalog_id).toBe('star5');
    // 계단은 `seedOffset(1) = 25` — 씨앗 상자에 그만큼 더한 자리다.
    expect(created.geometry).toEqual({ x: 75, y: 65, w: 100, h: 80 });
    // 닫힌 도형이므로 `rect` 의 씨앗 스타일 그대로다.
    expect(created.style).toEqual({ fill: '#3b82f6' });
    // 놓자마자 골라진다 — 다음 몸짓이 곧 배치 드래그가 된다.
    expect(selection()).toBe('el-2');
  });

  it('명령 목록은 카탈로그의 값이되 **그 배열이 아니다** — 값이지 참조가 아니다', () => {
    render(<Harness initial={[]} />);
    openGroup('arrow');
    fireEvent.click(screen.getByTestId('canvas-catalog-add-arrowRight'));

    const created = liveElements()[0] as PathElement;
    const source = findShape('arrowRight');
    expect(source).toBeDefined();
    if (source === undefined) return;
    expect(created.path).toEqual(source.path);
    // 카탈로그의 명령은 얼려 있다. 요소가 그것을 **가리키고** 있으면 여기서 던진다.
    expect(() => {
      const first = created.path[0];
      if (first !== undefined && first.c === 'M') first.x = 1;
    }).not.toThrow();
  });

  it('열린 도형은 선만 심고 채우지 않는다 — 채우면 저술한 적 없는 변이 생긴다', () => {
    render(<Harness initial={[]} />);
    openGroup('arrow');
    fireEvent.click(screen.getByTestId('canvas-catalog-add-arrowCurved'));

    const created = liveElements()[0] as PathElement;
    expect(created.style.fill).toBeUndefined();
    expect(created.style.stroke).toBe('#3b82f6');
    expect(created.style.strokeWidth).toBe(2);
  });

  it('연달아 놓으면 id 도 자리도 겹치지 않는다', () => {
    render(<Harness initial={[]} />);
    openGroup('basic');
    fireEvent.click(screen.getByTestId('canvas-catalog-add-triangle'));
    fireEvent.click(screen.getByTestId('canvas-catalog-add-hexagon'));

    const [a, b] = liveElements() as [PathElement, PathElement];
    expect(a.id).not.toBe(b.id);
    expect(a.geometry).not.toEqual(b.geometry);
  });
});

// --- 미리보기 ------------------------------------------------------------

describe('미리보기는 실제 렌더 경로다 (REQ-06)', () => {
  it('칸의 캔버스가 그 도형의 명령을 그대로 그린다', () => {
    const byId = stubPreviewContexts();
    render(<Harness initial={[]} />);
    openGroup('basic');

    const star = byId.get('star5');
    expect(star, 'star5 미리보기가 그리지 않았다').toBeDefined();
    if (star === undefined) return;
    const names = star.calls.map((c) => c[0]);
    // 표면을 먼저 지우고(별도 그리기 코드가 아니라 `clearSurface`), 그 다음 그린다.
    expect(names[0]).toBe('clearRect');
    expect(names).toContain('beginPath');
    expect(names).toContain('moveTo');
    expect(names).toContain('closePath');
    // 별은 열 점이다 — 명령 수만큼 `lineTo` 가 남는다.
    expect(names.filter((n) => n === 'lineTo')).toHaveLength(9);
    // 닫힌 도형의 씨앗은 채움이다.
    expect(names).toContain('fill');
    expect(names).not.toContain('stroke');

    // 미리보기 상자는 **정사각**이다. 카탈로그의 명령은 정사각 로컬 격자 위에서 그려졌으므로
    // 직사각 상자에 넣으면 30종 전부가 한 축으로 눌린 채 보인다. 별의 로컬 상자가 두 축 모두
    // 0..10000 이라, 그린 좌표의 가로 폭과 세로 폭이 같아야 한다.
    const xs: number[] = [];
    const ys: number[] = [];
    for (const [name, ...args] of star.calls) {
      if (name !== 'moveTo' && name !== 'lineTo') continue;
      xs.push(args[0] as number);
      ys.push(args[1] as number);
    }
    const spanX = Math.max(...xs) - Math.min(...xs);
    const spanY = Math.max(...ys) - Math.min(...ys);
    expect(spanX, '미리보기 상자가 정사각이 아니다').toBeCloseTo(spanY, 6);
  });

  it('곡선 도형은 미리보기에서도 곡선이고, 열린 도형은 선으로만 그려진다 (D4)', () => {
    const byId = stubPreviewContexts();
    render(<Harness initial={[]} />);
    openGroup('general');
    openGroup('arrow');

    const rounded = byId.get('roundedRect');
    expect(rounded).toBeDefined();
    if (rounded !== undefined) {
      const names = rounded.calls.map((c) => c[0]);
      expect(names).toContain('bezierCurveTo');
      expect(names).toContain('closePath');
    }

    const curved = byId.get('arrowCurved');
    expect(curved).toBeDefined();
    if (curved !== undefined) {
      const names = curved.calls.map((c) => c[0]);
      expect(names).toContain('bezierCurveTo');
      // 열린 도형이므로 닫지 않고, 씨앗이 선뿐이라 채우지 않는다.
      expect(names).not.toContain('closePath');
      expect(names).not.toContain('fill');
      expect(names).toContain('stroke');
    }
  });

  it('2D context 를 얻지 못해도 팔레트는 선다', () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    render(<Harness initial={[]} />);
    openGroup('basic');
    expect(screen.getByTestId('canvas-catalog-add-star5')).toBeTruthy();
  });
});

// --- i18n --------------------------------------------------------------

describe('이름은 진짜 번역으로 붙는다 (REQ-06 · 시험 규율 D7)', () => {
  it('치환자를 두 번 말하는 문구가 **두 자리 모두** 바뀐다', () => {
    render(<Harness initial={[]} />);
    openGroup('basic');

    const template = ko.dashboard.canvas.edit.paletteShapeAria;
    // 고정 입력이 스스로 무엇을 재는지 먼저 단언한다 — 문구에서 치환자가 하나로 줄면
    // 이 시험은 `replace` 결함을 더 이상 잡지 못하며, 그 사실이 여기서 드러나야 한다.
    expect(template.split('{shape}').length - 1, '치환자가 둘이 아니다').toBe(2);

    const name = ko.dashboard.canvas.edit.catalogNames.star5;
    const label = screen.getByTestId('canvas-catalog-add-star5').getAttribute('aria-label') ?? '';
    // `replace` 였다면 뒤쪽에 `{shape}` 가 벌거벗은 채 남는다.
    expect(label).not.toContain('{shape}');
    expect(label.split(name).length - 1, '이름이 두 번 들어 있지 않다').toBe(2);
    expect(label).toBe(template.replaceAll('{shape}', name));
    // 접근성 이름이 보이는 라벨을 포함한다(WCAG 2.5.3).
    expect(screen.getByTestId('canvas-catalog-add-star5').textContent).toContain(name);
  });

  it('묶음 머리는 번역된 제목을 보이고 화면에 원문 키가 뜨지 않는다', () => {
    render(<Harness initial={[]} />);
    // **뒤집힌 단언 (SPEC-CANVAS-011 REQ-01 · AC-01).** 008 의 `원시형` 머리는 없다.
    // 그 i18n 키(`paletteGroupPrimitive`)는 로케일 파일에 **남겨 두었다** — 소비처만 지운다.
    const titles = {
      basic: ko.dashboard.canvas.edit.paletteGroupBasic,
      general: ko.dashboard.canvas.edit.paletteGroupGeneral,
      arrow: ko.dashboard.canvas.edit.paletteGroupArrow,
    };
    for (const [id, title] of Object.entries(titles)) {
      const head = screen.getByTestId(`canvas-palette-group-${id}`);
      expect(head.textContent, id).toBe(title);
      expect(head.textContent, id).not.toContain('dashboard.canvas');
    }
  });
});

// --- 두 표면 -------------------------------------------------------------

describe('그려지는 자리에 손잡이가 닿는다 (AC-E9 · 불변식 I23 · 시험 규율 D10)', () => {
  it('008 이 더한 노드는 **전부** 도크 안에 있고 캔버스 표면에는 하나도 없다', () => {
    // 한 표면만 재면 006 을 통과시킨 그 형상이 그대로 돌아온다. `docked` 를 갈아 끼운다.
    for (const docked of [true, false] as const) {
      cleanup();
      render(<Harness initial={[rect('el-1')]} docked={docked} />);
      // 카탈로그를 열어 둔다 — 접힌 채로 재면 "칸이 없다" 가 두 표면에서 똑같이 참이라
      // 이 시험이 아무것도 재지 못한다.
      if (docked) openGroup('basic');

      const added = [
        ...document.querySelectorAll<HTMLElement>(
          '[data-testid^="canvas-palette-group-"],[data-testid^="canvas-catalog-"]',
        ),
      ];
      const overlayRoot = screen.getByTestId('canvas-edit-overlay');
      const dock = screen.queryByTestId('canvas-dock-panel');

      // ① 덮임 — 008 이 더한 노드는 하나도 빠짐없이 도크 안이다.
      for (const node of added) {
        expect(dock, `@docked=${docked}`).not.toBeNull();
        expect(dock?.contains(node), `${node.dataset.testid} 가 도크 밖이다`).toBe(true);
        // ② 캔버스 표면 위에는 아무것도 더하지 않는다 — 다스릴 수 없는 층을 만들지 않는다.
        expect(overlayRoot.contains(node), `${node.dataset.testid} 가 표면 위에 있다`).toBe(false);
      }

      // ③ 관계 — 도크의 유무와 카탈로그의 유무가 **같은 한 값**이다.
      expect(added.length > 0, `@docked=${docked} 의 카탈로그 유무`).toBe(dock !== null);
    }
  });

  it('대시보드 표면에는 묶음 머리도 · 칸도 · 미리보기도 없다', () => {
    render(<Harness initial={[rect('el-1')]} docked={false} />);

    for (const group of SHAPE_GROUPS.map((g) => g.id)) {
      expect(screen.queryByTestId(`canvas-palette-group-${group}`), group).toBeNull();
    }
    expect(screen.queryAllByTestId(/^canvas-catalog-/)).toHaveLength(0);
    // 그래도 놓인 경로 도형을 다스리는 손잡이는 그 표면에도 있다 — 팔레트가 없는 것이
    // "다스릴 수 없는 층" 을 만들지 않는 근거다(AC-E9 ③).
    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
  });
});
