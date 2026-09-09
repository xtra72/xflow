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

import type { CanvasElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { appendElement } from './canvasElementFactory';
import type { StageSize } from './canvasGeometry';

afterEach(cleanup);

const STAGE: StageSize = { width: 200, height: 100 };

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }, style: {} };
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
  const [elements, setElements] = useState<readonly CanvasElement[]>(initial);
  const state = useCanvasEditSelectionState();
  const overlay = (
    <div data-testid="scaled-panel">
      <CanvasEditOverlay
        enabled
        elements={elements}
        stage={STAGE}
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
    expect(geos[1]).toBe(JSON.stringify({ x: 0.15, y: 0.15, w: 0.2, h: 0.2 }));
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

  it('9px · 10px 이 없고 줄 버튼의 글자 크기가 주변과 같은 text-xs 다', () => {
    const source = dockSource();
    expect(source).not.toMatch(/text-\[9px\]/);
    expect(source).not.toMatch(/text-\[10px\]/);
    expect(source).toMatch(/const ROW_BUTTON_CLASS =[\s\S]*?text-xs/);
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
