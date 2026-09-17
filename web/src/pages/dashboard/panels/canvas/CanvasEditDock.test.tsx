// 캔버스 편집 도구 도크 시험 (SPEC-CANVAS-002 T9 후속 — 사용 시험: "도형 팔레트 크기가
// 너무 작음. 패널 설정에서만 사용할 것이므로, 패널 왼쪽에 영역 분할").
//
// 여기서 못박는 것 넷:
//   1) 도구는 **도크 자리가 있을 때만** 그려진다 — 자리를 내지 않는 대시보드 패널에는
//      도구가 없고, 스테이지 위에 떠 있던 띠도 남아 있지 않다.
//   2) 도크는 **패널 밖**에 선다 — 미리보기는 패널을 대시보드 크기로 렌더한 뒤 통째로
//      축소하므로, 도크가 패널 안에 있으면 그 미리보기가 대시보드와 다른 화면이 된다.
//   3) 도크의 도형 버튼은 목록 편집기가 쓰던 것과 **같은 생성 경로**(`appendElement`)를
//      부르고, 만든 것을 고른다(가정 A7). 목록 하단의 추가 버튼을 걷어낸 뒤로 이 버튼들이
//      **도형을 만드는 유일한 입구**이므로, 여기서 지는 무게가 그만큼 늘었다.
//   4) 크기와 이름 — 아이콘뿐이던 칸이 이름을 달고, 글자 크기는 주변 설정 화면을 따른다.
//   5) (2026-09-16) 도구 띠 — 도크의 절 일곱이 미리보기 제목 아래 가로 띠로 갔다. 띠와
//      도크는 **같은 조건**으로 나고 들며, 띠가 그 둘보다 앞(위)에 선다.
//
// jsdom 은 Tailwind 를 돌리지 않으므로 "얼마나 큰가" 는 렌더 결과로 잴 수 없다. 그래서
// 크기에 관한 주장만은 **소스를 훑는다** — `CanvasElementsEditor.test.tsx` §글자 크기가
// 세운 그 방식이며, 여기서 막으려는 것도 같다: 다시 작게 적는 일 그 자체다.

import { useState } from 'react';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

// i18n 은 키를 그대로 돌려준다(CanvasEditOverlay.test.tsx 선례).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { DEFAULT_CANVAS_SIZE, type CanvasElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { appendElement } from './canvasElementFactory';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';

afterEach(cleanup);

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 80 }, style: {} };
}

/**
 * 설정 미리보기의 형상을 흉내 낸다: 도크는 **축소되는 상자 밖**에 서고, 패널(여기서는
 * 오버레이)은 그 상자 안에 든다.
 *
 * `docked` 가 거짓이면 대시보드에 놓인 패널이다 — 도크 자리를 아무도 내지 않는다.
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
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <CanvasEditDockRegion enabled={docked}>{overlay}</CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

/** 도크 컴포넌트의 소스. 크기 주장은 이것을 훑는다(위 머리말). */
function dockSource(): string {
  return readFileSync(join(__dirname, 'CanvasEditDock.tsx'), 'utf-8');
}

/** 오버레이의 소스. "몸짓이 붙지 않았다" 는 주장은 렌더로 잴 수 없어 이것을 훑는다. */
function overlaySource(): string {
  return readFileSync(join(__dirname, 'CanvasEditOverlay.tsx'), 'utf-8');
}

/**
 * 도구 띠의 소스(2026-09-16). 도크에 있던 절 일곱이 이리로 갔으므로, **크기에 관한
 * 주장**이 따라와야 한다 — 파일 이름에 매인 가드는 내용이 옮겨 간 자리에서 빈 파일을
 * 지키며 조용히 무장 해제된다(바로 아래 `fieldSource()` 가 같은 이유로 생겼다).
 */
function toolbarSource(): string {
  return readFileSync(join(__dirname, 'CanvasEditToolbar.tsx'), 'utf-8');
}

/**
 * 배율 칸을 혼자 소유하는 컴포넌트의 소스(SPEC-CANVAS-006 M10).
 *
 * M9 의 환산 가드가 `dockSource()` 라는 **파일 이름**에 매여 있었으므로, 칸이 옮겨 가면
 * 그 가드도 함께 옮겨야 한다 — 옮기지 않으면 빈 파일을 지키며 **조용히 무장 해제된다**
 * (위험 R27 · R23 의 두 번째 얼굴).
 */
function fieldSource(): string {
  return readFileSync(join(__dirname, 'CanvasWorkspaceZoomField.tsx'), 'utf-8');
}

function zoomInput(): HTMLInputElement {
  return screen.getByTestId('canvas-workspace-zoom') as HTMLInputElement;
}

describe('도크는 패널 설정에서만 뜬다', () => {
  it('자리를 내면 도구가 그 자리에 그려진다', () => {
    render(<Harness initial={[]} />);

    const dock = screen.getByTestId('canvas-dock');
    expect(dock.contains(screen.getByTestId('canvas-dock-panel'))).toBe(true);
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.getByTestId(`canvas-palette-add-${kind}`)).toBeTruthy();
    }
  });

  it('자리를 내지 않으면(대시보드) 도구가 아예 없다 — 스테이지 위로 되돌아오지 않는다', () => {
    render(<Harness initial={[rect('el-1')]} docked={false} />);

    expect(screen.queryByTestId('canvas-dock')).toBeNull();
    expect(screen.queryByTestId('canvas-dock-panel')).toBeNull();
    expect(screen.queryByTestId('canvas-palette')).toBeNull();
    expect(screen.queryByTestId('canvas-palette-add-rect')).toBeNull();
    // 그래도 편집 표면 자체는 그대로다 — 옮긴 것은 팔레트뿐이고, 대시보드에서 기존 요소를
    // 끌고 크기를 조절하는 길은 종전과 같다.
    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
  });

  it('꺼진 도크는 감싸는 요소를 하나도 더하지 않는다 — 캔버스 아닌 패널의 DOM 이 그대로다', () => {
    const { container } = render(
      <CanvasEditDockRegion enabled={false}>
        <span data-testid="child">x</span>
      </CanvasEditDockRegion>,
    );

    expect(container.firstElementChild).toBe(screen.getByTestId('child'));
  });
});

// --- 도구 띠 (2026-09-16 — 사용자 결정: 도크의 절 일곱을 미리보기 제목 아래로) ---------
//
// 여기서 못박는 것 셋. 절들이 실제로 무엇을 하는가는 옮기기 전과 같은 시험들이 그대로
// 재고 있으므로(격자 토글 · 정렬 · 순서 · 배율 — 전부 전역 질의라 그릇을 묻지 않는다),
// 이 절이 지는 것은 **그릇 자체의 성질**이다.

describe('도구 띠는 도크와 **함께** 나고 든다', () => {
  it('자리를 내면 띠도 함께 서고, 절 여덟이 그 안에 있다', () => {
    render(<Harness initial={[]} />);

    const band = screen.getByTestId('canvas-toolbar-panel');
    expect(screen.getByTestId('canvas-toolbar').contains(band)).toBe(true);
    // 절의 **대표 컨트롤**이 전부 띠 안이다. 하나라도 도크에 남아 있으면 도구 한 벌이 두
    // 그릇에 흩어진 것이고, 그것은 옮김이 아니라 절반의 옮김이다.
    //
    // 일곱이 여덟이 된 것은 013 이 **변환** 절을 더했기 때문이다 — 옮겨 온 일곱에 새로
    // 선 하나다. 세어서 적는 것이 이 목록의 규율이므로 수도 함께 고친다.
    for (const id of [
      'canvas-workspace-zoom',
      'canvas-grid-toggle',
      'canvas-align-left',
      'canvas-order-front',
      'canvas-transform-flip-x',
      'canvas-group-create',
      'canvas-anchor-tool',
      'canvas-connector-tool-straight',
    ]) {
      expect(band.contains(screen.getByTestId(id)), id).toBe(true);
    }
    // 그리고 도크에 남은 것은 재료를 놓는 셋뿐이다.
    const dock = screen.getByTestId('canvas-dock-panel');
    for (const id of ['canvas-palette-add-rect', 'canvas-svg-import-group', 'canvas-scratchpad-save']) {
      expect(dock.contains(screen.getByTestId(id)), id).toBe(true);
    }
  });

  it('자리를 내지 않으면(대시보드) 띠도 없다 — 도크와 **같은 조건**이다', () => {
    // 조건이 갈라지면 띠만 있고 도크가 없는(또는 그 반대의) 표면이 생기고, 그때 그룹 ·
    // 앵커 · 연결선은 **어느 표면에서도** 닿지 않을 수 있다 — 006 이 배달한 그 결함이다.
    render(<Harness initial={[rect('el-1')]} docked={false} />);

    expect(screen.queryByTestId('canvas-toolbar')).toBeNull();
    expect(screen.queryByTestId('canvas-toolbar-panel')).toBeNull();
    expect(screen.queryByTestId('canvas-dock')).toBeNull();
    // 그 표면에서는 떠 있는 줄이 그 셋을 든다(불변식 I23) — 없어지는 것이 아니다.
    expect(screen.getByTestId('canvas-workspace-zoom-bar')).toBeTruthy();
  });

  it('띠는 미리보기 **제목 아래**다 — 도크·패널을 감싸지 않고 그 위에 선다', () => {
    // 자리를 담김이 아니라 **차례**로 잰다. 다이얼로그에서 이 지역은 제목 줄 바로 다음에
    // 통째로 꽂히므로, 지역 안에서 첫째인 것이 곧 "제목 바로 아래" 다.
    render(<Harness initial={[]} />);

    const band = screen.getByTestId('canvas-toolbar');
    const dock = screen.getByTestId('canvas-dock');
    const panel = screen.getByTestId('scaled-panel');
    // 감싸지 않는다 — 셋이 서로의 밖이다.
    expect(band.contains(dock)).toBe(false);
    expect(band.contains(panel)).toBe(false);
    expect(dock.contains(band)).toBe(false);
    // 그리고 띠가 **앞**이다(`DOCUMENT_POSITION_FOLLOWING` = 뒤에 온다).
    for (const later of [dock, panel]) {
      expect(band.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
    // 축소되는 상자 밖이라는 성질은 도크와 같다 — 안에 두면 미리보기가 대시보드와
    // 다른 화면이 된다(위 §미리보기 충실도).
    expect(panel.contains(screen.getByTestId('canvas-toolbar-panel'))).toBe(false);
  });
});

describe('도크는 축소되는 패널 **밖**에 선다 (미리보기 충실도)', () => {
  it('도구가 패널 상자 안에 들어 있지 않다', () => {
    // 패널 안에 두면 패널 자신의 레이아웃이 달라져 미리보기가 대시보드와 다른 화면이
    // 되고, 캔버스 스테이지도 도크 폭만큼 좁아진다.
    render(<Harness initial={[]} />);

    const panel = screen.getByTestId('scaled-panel');
    expect(panel.contains(screen.getByTestId('canvas-dock'))).toBe(false);
    expect(panel.contains(screen.getByTestId('canvas-dock-panel'))).toBe(false);
    expect(
      screen.getByTestId('canvas-edit-overlay').contains(screen.getByTestId('canvas-dock-panel')),
    ).toBe(false);
  });

  it('도크는 스테이지 위에 떠 있지 않다 — 절대 위치로 얹지 않는다', () => {
    // 떠 있는 띠였던 시절의 클래스(`absolute left-1 top-1`)가 되살아나면 그림을 다시
    // 가린다. 자리를 나눠 가진다는 것이 이 변경의 요지다.
    const source = dockSource();
    expect(source).toMatch(/const DOCK_CLASS =[\s\S]*?shrink-0/);
    expect(source).not.toMatch(/const DOCK_CLASS =[\s\S]*?absolute/);
  });
});

describe('도형 놓기는 공유 모듈과 한 경로다 — 이제 유일한 입구다 (가정 A7)', () => {
  it('도크가 낸 결과가 공유 모듈의 결과와 같다', () => {
    const before = [rect('el-1')];
    for (const kind of ['rect', 'ellipse', 'line', 'text'] as const) {
      cleanup();
      render(<Harness initial={before} />);
      fireEvent.click(screen.getByTestId(`canvas-palette-add-${kind}`));
      expect(liveElements()).toEqual(appendElement(before, kind).next);
    }
  });

  it('연속으로 놓아도 계단 오프셋이 겹치지 않는다 — 어느 버튼을 눌렀든 규칙이 같다', () => {
    render(<Harness initial={[]} />);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));

    const geos = liveElements().map((e) => JSON.stringify(e.geometry));
    expect(new Set(geos).size).toBe(2);
    expect(geos[1]).toBe(JSON.stringify({ x: 75, y: 65, w: 100, h: 80 }));
  });

  it('놓은 것이 곧바로 골라진다 — 다음 몸짓이 배치 드래그다', () => {
    render(<Harness initial={[rect('el-1')]} />);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));

    expect(screen.getByTestId('selection').textContent).toBe('el-2');
    // 골라졌으므로 그 자리에 외곽선이 붙는다.
    expect(screen.getByTestId('canvas-selection-el-2')).toBeTruthy();
  });
});

describe('도구는 이름을 달고 주변 설정 화면의 글자 크기를 쓴다', () => {
  it('도형 버튼이 아이콘 옆에 이름을 보인다', () => {
    render(<Harness initial={[]} />);

    const names: Record<string, string> = {
      rect: 'dashboard.canvas.edit.shapeRect',
      ellipse: 'dashboard.canvas.edit.shapeEllipse',
      line: 'dashboard.canvas.edit.shapeLine',
      text: 'dashboard.canvas.edit.shapeText',
    };
    for (const [kind, key] of Object.entries(names)) {
      expect(screen.getByTestId(`canvas-palette-add-${kind}`).textContent).toContain(key);
    }
  });

  it('보이는 이름이 생겨도 `aria-label` 은 남는다 — 하는 일을 말하는 것은 그쪽이다', () => {
    render(<Harness initial={[]} />);

    const button = screen.getByTestId('canvas-palette-add-ellipse');
    expect(button.getAttribute('aria-label')).toBe('dashboard.canvas.edit.paletteEllipse');
    expect(button.tagName).toBe('BUTTON');
    // 묶음 이름도 그대로다 — 스크린 리더가 이 한 벌을 하나로 읽는다.
    expect(screen.getByTestId('canvas-dock-panel').getAttribute('aria-label')).toBe(
      'dashboard.canvas.edit.paletteAria',
    );
    expect(screen.getByTestId('canvas-dock-panel').getAttribute('role')).toBe('group');
  });

  it('묶음마다 제목이 있고 그 제목이 묶음의 이름이 된다', () => {
    render(<Harness initial={[]} />);

    const shapes = screen.getByTestId('canvas-palette-add-rect').closest('section');
    expect(shapes).not.toBeNull();
    const labelledBy = shapes!.getAttribute('aria-labelledby');
    expect(labelledBy).not.toBeNull();
    expect(document.getElementById(labelledBy!)?.textContent).toBe(
      'dashboard.canvas.edit.dockShapes',
    );
  });

  it('9px · 10px 이 **두 파일 모두에** 없고 줄 버튼의 글자 크기가 주변과 같은 text-xs 다', () => {
    // **가드가 넓어졌다**(2026-09-16 — 도구 띠). 줄 버튼(`ROW_BUTTON_CLASS`)을 쓰던 절
    // 둘(격자 붙임 · 순서)이 띠로 갔고, 그 상수도 함께 갔다. 파일 이름에 매인 가드는 그
    // 순간 **빈 파일을 지킨다** — 006 M9 가 배율 칸을 옮기며 남긴 그 함정이다(위 §배율).
    // 그래서 이름을 좇아 옮기되 도크도 계속 훑는다: 막으려는 것은 파일이 아니라
    // "다시 작게 적는 일" 그 자체이고, 그 일은 두 파일 어디서나 일어날 수 있다.
    for (const [name, source] of [
      ['CanvasEditDock.tsx', dockSource()],
      ['CanvasEditToolbar.tsx', toolbarSource()],
    ] as const) {
      expect(source, name).not.toMatch(/text-\[9px\]/);
      expect(source, name).not.toMatch(/text-\[10px\]/);
    }
    expect(toolbarSource()).toMatch(/const ROW_BUTTON_CLASS =[\s\S]*?text-xs/);
  });
});

describe('격자 · 정렬 · 순서도 같은 자리로 옮겨 왔다', () => {
  it('격자 토글이 눌린 상태를 알리고, 꺼져 있으면 간격 고르개가 꺼진다', () => {
    render(<Harness initial={[]} />);

    const toggle = screen.getByTestId('canvas-grid-toggle');
    const step = screen.getByTestId('canvas-grid-step') as HTMLSelectElement;
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    expect(step.disabled).toBe(true);

    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    expect(step.disabled).toBe(false);
  });

  it('맞출 상대가 없으면 정렬이, 고른 것이 없으면 순서가 꺼져 있다', () => {
    render(<Harness initial={[rect('el-1')]} />);

    expect((screen.getByTestId('canvas-align-left') as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId('canvas-order-front') as HTMLButtonElement).disabled).toBe(true);

    // 하나를 놓으면 그것이 골라지므로 순서는 살아나고, 정렬은 여전히 상대가 없다.
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    expect((screen.getByTestId('canvas-order-front') as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByTestId('canvas-align-left') as HTMLButtonElement).disabled).toBe(true);
  });

  it('순서 버튼이 배열 순서를 실제로 바꾼다 — 옮겨 와서도 같은 일을 한다', () => {
    render(<Harness initial={[rect('el-1'), rect('el-2')]} />);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect')); // el-3 을 놓고 고른다

    fireEvent.click(screen.getByTestId('canvas-order-back'));
    expect(liveElements().map((e) => e.id)).toEqual(['el-3', 'el-1', 'el-2']);
  });
});

// --- 보기 배율 (SPEC-CANVAS-006 M9 · REQ-09 · AC-09 (AV)) ----------------
//
// 이 절이 재는 것은 **자리와 형상**이다. 값의 산술은 `canvasWorkspace.test.ts` 가 지고,
// 여기서는 그 산술에 닿는 길이 화면에 옳게 나 있는가를 잰다.

describe('작업 영역 묶음은 **띠**의 맨 앞에 서고 이름을 갖는다 (AC-09 (AV))', () => {
  it('띠의 첫 묶음이 작업 영역이고 `role="group"` · `aria-labelledby` 로 이름을 갖는다', () => {
    // **자리가 옮겨졌다**(2026-09-16 — 도구 띠). 006 이 이 묶음을 도크 맨 앞에 둔 근거는
    // "배율은 보이지 않을 때 손이 가는 컨트롤이라 스크롤해야 찾으면 그 순간 실패한다"
    // 였고, 그 근거는 **강해졌다**: 띠는 미리보기 제목 바로 아래라 스크롤이 아예 없다.
    // 그래서 단언을 지우지 않고 겨누는 그릇만 도크 → 띠로 바꾼다. 아래 두 줄이 그 이동을
    // 관계로 못박는다 — 배율은 띠의 첫 묶음, 도형은 도크의 첫 묶음.
    render(<Harness initial={[]} />);

    const sections = [...screen.getByTestId('canvas-toolbar-panel').querySelectorAll('section')];
    expect(sections.length).toBeGreaterThan(1);
    const [first] = sections as [HTMLElement];
    expect(first.contains(zoomInput())).toBe(true);
    expect(first.getAttribute('role')).toBe('group');
    const labelledBy = first.getAttribute('aria-labelledby');
    expect(labelledBy).not.toBeNull();
    expect(document.getElementById(labelledBy!)?.textContent).toBe(
      'dashboard.canvas.edit.dockView',
    );
    // 도형 묶음은 **도크의 맨 앞**이다 — 밀어내던 두 줄이 사라진 자리다.
    const dockSections = [...screen.getByTestId('canvas-dock-panel').querySelectorAll('section')];
    const [firstDock] = dockSections as [HTMLElement];
    expect(firstDock.contains(screen.getByTestId('canvas-palette-add-rect'))).toBe(true);
  });

  it('백분율 정수 입력이고 `aria-label` 과 `title` 이 같은 키에서 나온다', () => {
    render(<Harness initial={[]} />);
    const input = zoomInput();

    expect(input.type).toBe('number');
    expect(input.getAttribute('inputmode')).toBe('numeric');
    expect(input.getAttribute('aria-label')).toBe('dashboard.canvas.edit.workspaceZoom');
    expect(input.getAttribute('title')).toBe('dashboard.canvas.edit.workspaceZoom');
    expect(input.getAttribute('min')).toBe('25');
    expect(input.getAttribute('max')).toBe('100');
    expect(input.getAttribute('step')).toBe('1');
    // 기본값은 화면이 오늘 보여 주는 그 배율이다.
    expect(input.value).toBe('75');
  });

  it('제안 넷이 `<datalist>` 로 곁들여지고 옵션 글자가 번역된다', () => {
    render(<Harness initial={[]} />);

    const list = screen.getByTestId('canvas-workspace-zoom-suggestions');
    expect(zoomInput().getAttribute('list')).toBe(list.id);
    const options = [...list.querySelectorAll('option')];
    expect(options.map((o) => o.value)).toEqual(['25', '50', '75', '100']);
    // 벌거벗은 숫자는 그것이 백분율인지 캔버스 단위인지 말하지 않는다 — 바로 아래 칸이
    // 캔버스 단위를 받으므로 그 모호함이 실제 오해가 된다.
    for (const option of options) {
      expect(option.textContent).toContain('dashboard.canvas.edit.workspaceZoomOption');
    }
  });

  it('도움말이 **상시**이고 `aria-describedby` 로 이어져 있다', () => {
    // 조건부 고지는 기각했다 — 간격 25 에서 백분율 넷 가운데 셋이 깎이므로 사실상 상시
    // 뜨는 경고가 되고, 그것은 위험 R18 이 이미 이름을 붙인 실패다.
    render(<Harness initial={[]} />);

    const help = screen.getByTestId('canvas-workspace-zoom-hint');
    expect(help).toBeTruthy();
    const describedBy = zoomInput().getAttribute('aria-describedby');
    expect(describedBy).not.toBeNull();
    expect(help.textContent).toContain('dashboard.canvas.edit.workspaceZoomHint');

    // 격자를 켜고 끄고 간격을 바꿔도 사라지지 않는다 — 조건부가 아니라는 뜻이다.
    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
    fireEvent.change(screen.getByTestId('canvas-grid-step'), { target: { value: '50' } });
    expect(screen.getByTestId('canvas-workspace-zoom-hint')).toBeTruthy();
  });

  it('배율 칸은 붙임 토글에 **매여 있지 않다** — 그래서 격자 묶음이 아니라 제 묶음에 산다', () => {
    render(<Harness initial={[]} />);

    // 격자가 꺼진 채로도 살아 있다. 간격 칸은 그 반대다(끌 수 있는 토글에 매여 있다).
    expect(zoomInput().disabled).toBe(false);
    expect((screen.getByTestId('canvas-grid-step') as HTMLInputElement).disabled).toBe(true);
  });
});

describe('배율 칸이 실제로 값을 간다 (AC-09 (AS) · (AT))', () => {
  it('적은 백분율이 그대로 반영된다 — 되죄지 않는다', () => {
    render(<Harness initial={[]} />);

    fireEvent.change(zoomInput(), { target: { value: '50' } });
    expect(zoomInput().value).toBe('50');
    // 73% 는 간격 25 에서 기본값과 같은 그림을 내지만 **값 자체는 73 으로 남는다** —
    // 간격이 배율을 조용히 고치면 그것이 곧 I20 이 금지한 결합의 거울상이다(위험 R24).
    fireEvent.change(zoomInput(), { target: { value: '73' } });
    expect(zoomInput().value).toBe('73');
  });

  it('범위 밖은 죄이고, 읽을 수 없는 입력에는 지금 값이 그대로 남는다', () => {
    render(<Harness initial={[]} />);

    fireEvent.change(zoomInput(), { target: { value: '400' } });
    expect(zoomInput().value).toBe('100'); // 확대는 없다(불변식 I22)
    fireEvent.change(zoomInput(), { target: { value: '1' } });
    expect(zoomInput().value).toBe('25');
    fireEvent.change(zoomInput(), { target: { value: '' } });
    expect(zoomInput().value).toBe('25'); // 한 글자를 지우는 동안 화면이 무너지지 않는다
  });

  it('배율을 바꿔도 요소가 한 글자도 바뀌지 않는다 — 바뀌는 것은 시야뿐이다 (AC-09 (AS))', () => {
    render(<Harness initial={[rect('el-1')]} />);
    const before = screen.getByTestId('dump').textContent;

    fireEvent.change(zoomInput(), { target: { value: '50' } });

    expect(screen.getByTestId('dump').textContent).toBe(before);
  });

  it('다시 마운트하면 기본값으로 돌아간다 — 저장하지 않는다는 결정의 관측 가능한 얼굴이다', () => {
    // 돌아가는 값이 오늘 사용자가 보던 그 화면(75%)이라 초기화가 잃음이 아니라 되돌아옴
    // 으로 읽힌다(가정 A21). 그리고 범위가 마운트 하나이므로 두 캔버스 패널이 서로 다른
    // 배율을 가질 수 있다.
    render(<Harness initial={[]} />);
    fireEvent.change(zoomInput(), { target: { value: '50' } });
    expect(zoomInput().value).toBe('50');

    cleanup();
    render(<Harness initial={[]} />);

    expect(zoomInput().value).toBe('75');
  });

  it('간격을 바꿔도 배율이 한 글자도 바뀌지 않는다 (AC-09 (AU))', () => {
    render(<Harness initial={[]} />);
    fireEvent.change(zoomInput(), { target: { value: '50' } });

    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
    fireEvent.change(screen.getByTestId('canvas-grid-step'), { target: { value: '7' } });

    expect(zoomInput().value).toBe('50');
  });
});

describe('배율은 몸짓을 하나도 가져가지 않는다 (AC-09 (AV))', () => {
  it('도크에도 오버레이에도 휠 처리자가 없다', () => {
    // Ctrl/⌘+휠은 이 화면에서 이미 미리보기 확대의 것이고(`handlePreviewWheel`), 방향키는
    // 오버레이 안에서 이미 둘로 갈려 있다(선택 있음 → 요소 이동 / 없음 → 화면 이동).
    // 셋째 주인이 낄 자리가 없다.
    expect(dockSource()).not.toMatch(/onWheel|deltaY/);
    expect(overlaySource()).not.toMatch(/onWheel|deltaY/);
  });

  it('배율을 바꾸는 길은 그 칸뿐이다 — **셈이 아니라 형상**으로 잰다 (위험 R27)', () => {
    // M9 는 이것을 **셈**으로 지켰다(`setWorkspaceZoom` 이 정확히 둘: 선언 한 줄 + 도크로
    // 넘기는 prop 한 줄). 그러나 그 가드가 지키려던 문장은 셈이 아니라 **"몸짓이 붙지
    // 않았다"** 였고, M10 이 렌더 자리를 하나 늘리자(떠 있는 줄) 셈은 깨지는데 지키려던
    // 문장은 그대로 참이었다 — 이름·셈에 매인 가드가 제 주제를 따라가지 못해 죽는 그
    // 부류다(위험 R23 의 두 번째 얼굴 · R27).
    //
    // 그래서 형상으로 다시 쓴다: setter 는 **선언 한 줄과 렌더 prop 자리에만** 나타난다.
    // **단언은 약해지지 않고 넓어진다** — 렌더 자리가 몇으로 늘어도 죽지 않고, 처리자
    // 본문이나 인라인 화살표(`onKeyDown={() => setWorkspaceZoom(...)}`)에 한 번이라도
    // 나타나면 걸린다.
    const source = overlaySource();
    const lines = source.split('\n').filter((line) => line.includes('setWorkspaceZoom'));
    const declarations = lines.filter((line) => /^\s*const setWorkspaceZoom =/.test(line));
    const renders = lines.filter((line) => /^\s*onZoomChange=\{setWorkspaceZoom\}$/.test(line));
    expect(declarations.length).toBe(1);
    // 렌더 자리는 **둘 이상**이다(도크 + 줄). 이 수를 못박지 않는 것이 이 회차의 요점이다.
    expect(renders.length).toBeGreaterThanOrEqual(2);
    expect(lines.length).toBe(declarations.length + renders.length);

    // 그리고 **어떤 이벤트 처리자 본문에도** 없다 — 방향키는 이미 둘로 갈려 있고, 휠은
    // 미리보기 확대의 것이다. 자르는 끝을 함수의 닫는 줄로 잡는다: 파일 끝까지 자르면
    // 아래 JSX 의 prop 한 줄이 딸려 들어와 이 단언이 늘 실패한다(자를 자리를 틀리면
    // 시험이 제 이름과 다른 것을 잰다).
    for (const name of [
      'handleKeyDown',
      'handlePointerDown',
      'handlePointerMove',
      'handlePointerUp',
    ]) {
      const start = source.indexOf(`const ${name}`);
      expect(start, name).toBeGreaterThan(0);
      const end = source.indexOf('\n  };', start);
      expect(end, name).toBeGreaterThan(start);
      expect(source.slice(start, end), name).not.toMatch(/setWorkspaceZoom|onZoomChange/);
    }
    // 휠 · 드래그 · 전역 리스너는 아예 없다 — 몸짓을 가져갈 자리 자체가 없다.
    expect(source).not.toMatch(/onWheel|deltaY|addEventListener/);
  });

  it('백분율 환산이 나타나도 되는 파일은 **둘뿐**이다 (위험 R27 — 가드가 칸을 따라 옮겨 왔다)', () => {
    // M9 는 이 가드를 `dockSource()` 라는 **파일 이름**에 매어 두었다. M10 이 칸을
    // `CanvasWorkspaceZoomField.tsx` 로 옮기자 그 가드는 **빈 파일을 지키게 되어** 조용히
    // 무장 해제될 참이었다 — 깨지는 것이 아니라 조용해지는 부류라 더 위험하다(R23 의 두
    // 번째 얼굴). 그래서 옮기고, 같은 회차에 **범위를 넓힌다**.
    //
    // 환산이 나타나도 되는 파일은 `canvasWorkspace.ts`(산술의 주인)와 그 칸을 그리는
    // `CanvasWorkspaceZoomField.tsx` 둘뿐이며, 도크와 오버레이 어디에도 없다.
    expect(fieldSource()).toMatch(/workspaceZoomPercent/);
    expect(fieldSource()).toMatch(/clampWorkspaceZoom/);
    for (const [name, source] of [
      ['CanvasEditDock.tsx', dockSource()],
      ['CanvasEditOverlay.tsx', overlaySource()],
      ['CanvasWorkspaceZoomField.tsx', fieldSource()],
    ] as const) {
      expect(source, name).not.toMatch(/\* 100|\/ 100/);
    }
    // 그리고 도크와 오버레이는 환산 함수를 **부르지도** 않는다 — 값을 그대로 지나 보낼 뿐이다.
    for (const [name, source] of [
      ['CanvasEditDock.tsx', dockSource()],
      ['CanvasEditOverlay.tsx', overlaySource()],
    ] as const) {
      expect(source, name).not.toMatch(/workspaceZoomPercent|clampWorkspaceZoom/);
    }
  });

  it('칸의 소유자가 **하나**다 — 범위 · `aria` · 제안 넷이 한 파일에만 적혀 있다 (불변식 I24)', () => {
    // 두 벌이 되면 개명이나 범위 변경이 한쪽만 따라간다 — 0.11.0 의 격자 간격 목록과
    // 0.4.0 의 폭·높이 칸이 각각 그 결말을 보였다.
    expect(fieldSource()).toMatch(/data-testid="canvas-workspace-zoom"/);
    expect(fieldSource()).toMatch(/MIN_WORKSPACE_ZOOM/);
    expect(fieldSource()).toMatch(/MAX_WORKSPACE_ZOOM/);
    expect(fieldSource()).toMatch(/WORKSPACE_ZOOM_CHOICES/);
    expect(fieldSource()).toMatch(/data-testid="canvas-workspace-zoom-suggestions"/);
    for (const [name, source] of [
      ['CanvasEditDock.tsx', dockSource()],
      ['CanvasEditOverlay.tsx', overlaySource()],
    ] as const) {
      expect(source, name).not.toMatch(/data-testid="canvas-workspace-zoom"/);
      expect(source, name).not.toMatch(/WORKSPACE_ZOOM_CHOICES|MIN_WORKSPACE_ZOOM/);
      // 도크에는 **격자 간격**의 `<datalist>` 가 여전히 있다 — 겨누는 것은 배율의 것 하나다.
      expect(source, name).not.toMatch(/canvas-workspace-zoom-suggestions/);
    }
  });
});

describe('대시보드에 놓인 패널에는 **배율만** 있다 (AC-10 (AX) — 0.6.0 의 관측이 뒤집힌 자리)', () => {
  it('배율은 떠 있는 줄에 서고, 나머지 넷은 **여전히 없다**', () => {
    // **이 시험은 0.7.0 에서 겨눔이 뒤집혔다.** 0.6.0 은 "대시보드에는 배율 컨트롤이
    // 없다" 를 **관측**으로 적었고, 사용자가 그 자리에서 "0.75 를 바꿀 수 있게" 를 다시
    // 요구하면서 그 관측이 **결함 진술**이 되었다 — 표시 층 셋은 조건 없이 그려지는데
    // 그것을 다스릴 손잡이가 그 표면에 하나도 없었다(불변식 I23).
    //
    // 함께 단언된 넷은 **글자 그대로 살아남는다.** 0.7.0 이 고치는 것은 배율 하나이며,
    // 그 넷이 없는 것은 여전히 이름 붙은 미해결 비대칭이다(사용자가 도구 한 벌을 주는
    // 안 (b) 를 고르지 않았다). **이 단언을 함께 지우면 그 넷에 대한 회귀 가드가 사라진다.**
    render(<Harness initial={[rect('el-1')]} docked={false} />);

    // 뒤집힌 둘 — 이제 있다. 그리고 도크가 아니라 **줄** 안에 있다.
    const bar = screen.getByTestId('canvas-workspace-zoom-bar');
    expect(bar.contains(zoomInput())).toBe(true);
    expect(bar.contains(screen.getByTestId('canvas-workspace-zoom-hint'))).toBe(true);
    // 도크 본문이 대시보드로 온 것이 **아니다** — 온 것은 배율 하나다.
    expect(screen.queryByTestId('canvas-dock-panel')).toBeNull();

    // 살아남는 넷 — 여전히 없다.
    expect(screen.queryByTestId('canvas-grid-toggle')).toBeNull();
    expect(screen.queryByTestId('canvas-grid-step')).toBeNull();
    expect(screen.queryByTestId('canvas-align-left')).toBeNull();
    expect(screen.queryByTestId('canvas-order-front')).toBeNull();
    expect(screen.queryByTestId('canvas-palette-add-rect')).toBeNull();
  });

  it('도크가 있는 표면에는 줄이 서지 않는다 — 한 값에 살아 있는 컨트롤은 하나다 (불변식 I24)', () => {
    render(<Harness initial={[rect('el-1')]} />);

    expect(screen.queryByTestId('canvas-workspace-zoom-bar')).toBeNull();
    expect(screen.getAllByTestId('canvas-workspace-zoom').length).toBe(1);
    // **그릇이 바뀌었다**(2026-09-16 — 도구 띠): 그 하나가 사는 곳이 도크에서 띠로 갔다.
    // 재는 것은 그대로 "한 표면에 살아 있는 배율 칸은 하나" 이고, 도크에 **없다**는 것을
    // 함께 못박아 옮김이 복사가 아니었음을 남긴다.
    expect(screen.getByTestId('canvas-toolbar-panel').contains(zoomInput())).toBe(true);
    expect(screen.getByTestId('canvas-dock-panel').contains(zoomInput())).toBe(false);
  });
});
