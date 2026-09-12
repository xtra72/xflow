// 요소 종류 두 이름의 경계 (SPEC-CANVAS-008 M5).
//
// 008 이 다섯 번째 종류를 들이면서 `CanvasElementKind` 한 이름이 겸하던 두 뜻을 갈랐다:
//
//   - `CanvasPrimitiveKind` — 사용자가 **직접 고를 수 있는** 넷. 팔레트가 내는 것, 종류
//     바꾸기가 받는 것, `newElement` 가 만드는 것.
//   - `CanvasElementKind` — 요소가 **가질 수 있는** 다섯. `CanvasElement['kind']` 와 같다.
//
// 겸직을 그대로 두었다면 어느 쪽으로 넓히든 잘못됐다. 넷으로 두면 다섯 번째 요소의 `kind`
// 를 받는 자리들이 조용히 넷 중 하나로 읽고, 다섯으로 두면 팔레트와 종류 바꾸기가 경로를
// **고를 수 있는 것**으로 내놓는다(REQ-07 이 금지한 바로 그것).
//
// 이 파일이 재는 것은 그 경계 둘이다:
//   ① 넓은 쪽이 `CanvasElement['kind']` 와 **정확히 같은 집합**인가(총망라 — 여섯 번째
//      종류가 들어오면 아래 Record 가 컴파일에서 운다)
//   ② 좁은 쪽이 화면에서 **여전히 넷**인가(AC-E10 — 팔레트 칸이 다섯이 되지 않았는가)
//
// @spec SPEC-CANVAS-008 REQ-07 · AC-E10

import { useState } from 'react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import {
  DEFAULT_CANVAS_SIZE,
  type CanvasElement,
  type CanvasElementKind,
  type CanvasPrimitiveKind,
} from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';

afterEach(cleanup);

// --- ① 넓은 쪽의 총망라 --------------------------------------------------

/**
 * 다섯 종류 전부를 키로 갖는 표. **`Record` 라서 총망라를 요구한다** — 여섯 번째 종류가
 * `CanvasElementKind` 에 들어오는 순간 이 선언이 컴파일에서 운다. 배열로 적었다면 울지
 * 않았을 것이고, 그 침묵이 008 이 처음부터 겨눈 부류다.
 */
const ALL_KINDS: Record<CanvasElementKind, true> = {
  rect: true,
  ellipse: true,
  line: true,
  text: true,
  path: true,
};

/** 원시형 넷. 같은 이유로 `Record` 다 — 다섯 번째 원시형이 들어오면 여기가 운다. */
const PRIMITIVE_KINDS: Record<CanvasPrimitiveKind, true> = {
  rect: true,
  ellipse: true,
  line: true,
  text: true,
};

describe('두 종류 이름의 경계', () => {
  it('넓은 쪽이 다섯이고 좁은 쪽이 넷이다', () => {
    expect(Object.keys(ALL_KINDS).sort()).toEqual(['ellipse', 'line', 'path', 'rect', 'text']);
    expect(Object.keys(PRIMITIVE_KINDS).sort()).toEqual(['ellipse', 'line', 'rect', 'text']);
  });

  it('둘의 차이는 **경로 하나뿐**이다', () => {
    const extra = Object.keys(ALL_KINDS).filter((k) => !(k in PRIMITIVE_KINDS));
    expect(extra).toEqual(['path']);
  });

  it('요소 합집합의 `kind` 가 넓은 쪽에 **전부** 든다 — 읽히지 못하는 종류가 없다', () => {
    const samples: CanvasElement[] = [
      { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: {} },
      { id: 'e', kind: 'ellipse', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: {} },
      { id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 1, y2: 1 }, style: {} },
      { id: 't', kind: 'text', geometry: { x: 0, y: 0 }, style: {} },
      { id: 'p', kind: 'path', geometry: { x: 0, y: 0, w: 1, h: 1 }, path: [], style: {} },
    ];
    // 아래 대입이 곧 형상 판정이다 — `el.kind` 가 `CanvasElementKind` 보다 넓으면 컴파일에서
    // 운다. 그리고 다섯 표본이 다섯 키를 **전부** 덮는지 수로도 확인한다.
    const seen = new Set<CanvasElementKind>(samples.map((el) => el.kind));
    expect(seen.size).toBe(Object.keys(ALL_KINDS).length);
  });
});

// --- ② 팔레트는 여전히 넷이다 (AC-E10) -----------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

function Harness() {
  const [elements, setElements] = useState<readonly CanvasNode[]>([]);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <CanvasEditDockRegion enabled>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={PROJ}
          textWidths={{}}
          onElementsChange={setElements}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

describe('팔레트는 넷을 그대로 낸다 (AC-E10)', () => {
  it('칸이 **정확히 넷**이고 이름도 차례도 그대로다', () => {
    render(<Harness />);
    // "넷이 있다" 가 아니라 "넷뿐이다" 를 잰다. 앞의 문장은 다섯 번째 칸이 생겨도 참이다.
    const ids = screen
      .getAllByTestId(/^canvas-palette-add-/)
      .map((node) => (node.dataset.testid ?? '').replace('canvas-palette-add-', ''));
    expect(ids).toEqual(['rect', 'ellipse', 'line', 'text']);
  });

  it('경로 칸이 없다 — 다섯 번째 종류가 팔레트로 새 나오지 않았다', () => {
    render(<Harness />);
    expect(screen.queryByTestId('canvas-palette-add-path')).toBeNull();
  });
});
