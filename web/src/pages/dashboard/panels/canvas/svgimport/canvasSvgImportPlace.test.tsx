// 놓으면 일반 요소가 된다 — 도크 · 오버레이 · 생성 입구를 한 시험에서 함께 잰다
// (SPEC-CANVAS-007 M9 · REQ-06 · REQ-08 · AC-09).
//
// **층을 따로 재면 이음매가 덮이지 않는다.** 계획도 생성 입구도 제 시험이 있으나, "도크에서
// 고른 파일이 오버레이의 배열에 닿는가" 는 둘 중 어느 시험도 재지 않는다 — 이 저장소가
// 편집기↔렌더러에서 이미 물린 부류다. 여기서는 **파일 입력에서 시작해 요소 배열에서 끝난다.**
//
// 축척은 스테이지 250 × 200 · 캔버스 500 × 400 이라 **0.5** 다. 단위 축척은 나눗셈이 항등이라
// 투영 결함을 통째로 숨긴다(시험 규율).
//
// **파일 읽기를 갈아 끼우지 않는다.** 도크가 세우는 `CanvasSvgImport` 에는 주입 지점이 없고,
// 그것이 옳다 — 여기서 재려는 것은 **실제 배선**이다. jsdom 의 `FileReader` 는 비동기이므로
// `waitFor` 로 기다린다.
//
// @spec SPEC-CANVAS-007 REQ-06 · REQ-08 · AC-09

import { useState } from 'react';

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { CanvasEditDockRegion } from '../CanvasEditDock';
import CanvasEditOverlay from '../CanvasEditOverlay';
import type { CanvasElement, CanvasSize, PathElement } from '../canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import type { StageSize } from '../canvasGeometry';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 250, height: 200 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 · 안 나누어떨어짐 (시험 규율 E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

/**
 * 도형 **둘**. 하나는 `viewBox` 를 가득 채우고(어느 점을 찍어도 맞는다) 다른 하나는 그 안에
 * 든다. 하나뿐이면 "무리로 움직인다" 와 "하나가 움직인다" 가 구별되지 않는다.
 */
const DOC =
  `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}>` +
  '<rect x="-13" y="7" width="317" height="181" fill="#c0392b"/>' +
  '<rect x="40" y="40" width="60" height="40" fill="#145a32"/>' +
  '</svg>';

/**
 * 이미 놓인 요소 **셋**. 계단 오프셋이 0 이 아닌 자리에서 재기 위한 것이다 — 0 이면
 * "무리 전체에 한 번" 과 "요소마다 한 번" 이 같은 답을 낸다.
 */
const SEED: readonly CanvasElement[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 10, y: 10, w: 20, h: 20 }, style: { fill: '#a' } },
  { id: 'el-2', kind: 'rect', geometry: { x: 40, y: 10, w: 20, h: 20 }, style: { fill: '#b' } },
  { id: 'el-3', kind: 'rect', geometry: { x: 70, y: 10, w: 20, h: 20 }, style: { fill: '#c' } },
];

// --- 하네스 ---------------------------------------------------------------

let changes = 0;

function Harness({ docked = true }: { docked?: boolean }) {
  const [elements, setElements] = useState<readonly CanvasElement[]>(SEED);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <CanvasEditDockRegion enabled={docked}>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={{ stage: STAGE, canvas: CANVAS }}
          textWidths={{}}
          onElementsChange={(next) => {
            changes += 1;
            setElements(next);
          }}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

function liveSelection(): string[] {
  const raw = screen.getByTestId('selection').textContent ?? '';
  return raw === '' ? [] : raw.split(',');
}

function stubOverlayRect(): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: STAGE.width,
    height: STAGE.height,
    right: STAGE.width,
    bottom: STAGE.height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

function send(type: string, x: number, y: number, init: MouseEventInit = {}): void {
  fireEvent(
    screen.getByTestId('canvas-edit-overlay'),
    new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init }),
  );
}

/** 묶음을 펴고 문서를 골라 준비 상태까지 간다. **진짜 `FileReader` 를 지난다.** */
async function chooseDoc(text = DOC): Promise<void> {
  fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
  fireEvent.change(screen.getByTestId('canvas-svg-import-file'), {
    target: { files: [new File([text], 'a.svg', { type: 'image/svg+xml' })] },
  });
  await waitFor(() => expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy());
}

beforeEach(() => {
  changes = 0;
  localStorage.clear();
  // jsdom 은 2D context 를 주지 않고 부르는 자리마다 시끄럽게 운다.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

// --- 놓기 -----------------------------------------------------------------

describe('AC-09 — 놓으면 일반 요소가 된다 (REQ-06)', () => {
  it('요소들이 배열 **끝에 문서 순서대로** 붙고 앞의 셋은 그대로다', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    const els = liveElements();
    expect(els).toHaveLength(5);
    expect(els.slice(0, 3)).toEqual(SEED);
    // 배열 끝 = 맨 위 = 문서의 나중 — 두 순서가 일치한다(REQ-06).
    expect(els.slice(3).map((el) => el.kind)).toEqual(['path', 'path']);
    // 문서에서 뒤에 있던 초록 사각이 배열에서도 뒤에 있다.
    expect((els[3] as PathElement).style.fill).toBe('#c0392b');
    expect((els[4] as PathElement).style.fill).toBe('#145a32');
  });

  it('새 id 가 `canvasElementFactory` 의 규칙을 따른다 — 겹치지 않는다', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    const ids = liveElements().map((el) => el.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.slice(3)).toEqual(['el-4', 'el-5']);
  });

  it('계단 오프셋이 **무리 전체에 한 번**이다 — 요소마다 더하면 그림이 흩어진다', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    const created = liveElements().slice(3) as PathElement[];
    // 값이 같다 — 조각마다 상자를 지었으면 정수 반올림이 서로 어긋난다.
    expect(created[0]?.geometry).toEqual(created[1]?.geometry);
    // 그리고 계단이 실제로 **0 이 아니다**(이미 놓인 셋 때문) — 0 이면 이 시험이 무력하다.
    expect(created[0]?.geometry.x).not.toBe(50);
    // 상자 종횡비가 문서의 그것(317 : 181)이다.
    const g = created[0]!.geometry;
    expect(g.w / g.h).toBeCloseTo(317 / 181, 1);
  });

  it('새로 만들어진 id **전부**가 선택으로 선다 (REQ-06)', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));
    expect(liveSelection().sort()).toEqual(['el-4', 'el-5']);
  });

  it('그 선택을 끌면 **한 덩어리로** 움직인다 — 같은 캔버스 델타', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));
    stubOverlayRect();

    const before = (liveElements().slice(3) as PathElement[]).map((el) => ({ ...el.geometry }));
    // 상자 안의 한 점 — 첫 도형이 `viewBox` 를 가득 채우므로 가운데는 반드시 맞는다.
    const g = before[0]!;
    const cx = (g.x + g.w / 2) * 0.5;
    const cy = (g.y + g.h / 2) * 0.5;
    // 이미 고른 것을 다시 잡으면 무리가 유지된다.
    send('pointerdown', cx, cy);
    send('pointermove', cx + 20, cy + 10);
    send('pointerup', cx + 20, cy + 10);

    const after = (liveElements().slice(3) as PathElement[]).map((el) => el.geometry);
    expect(after).toHaveLength(2);
    const deltas = after.map((geo, i) => ({
      dx: geo.x - before[i]!.x,
      dy: geo.y - before[i]!.y,
    }));
    // 켜져 있음을 먼저 잰다 — 안 움직였으면 "같은 델타" 가 `0 === 0` 이 된다.
    expect(Math.abs(deltas[0]!.dx)).toBeGreaterThan(0);
    expect(deltas[1]).toEqual(deltas[0]);
    // 고르지 않은 셋은 그대로다.
    expect(liveElements().slice(0, 3)).toEqual(SEED);
  });

  it('가져온 요소가 카탈로그 경로와 **구별되지 않는다** (REQ-08)', async () => {
    render(<Harness />);
    // 카탈로그에서 하나 놓고 — 같은 화면에서 견준다.
    fireEvent.click(screen.getByTestId('canvas-palette-group-general'));
    const catalogButton = screen
      .getByTestId('canvas-dock-panel')
      .querySelector('[data-testid^="canvas-catalog-add-"]');
    expect(catalogButton).not.toBeNull();
    fireEvent.click(catalogButton as HTMLElement);

    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    const els = liveElements();
    const fromCatalog = els[3] as PathElement;
    const fromImport = els[4] as PathElement;
    expect(fromCatalog.kind).toBe('path');
    expect(fromImport.kind).toBe('path');
    // 필드 이름 집합이 `catalog_id` 하나만 다르다 — 009 가 출처별 분기를 갖지 않아도 된다.
    const catalogKeys = Object.keys(fromCatalog).sort();
    const importKeys = Object.keys(fromImport).sort();
    expect(catalogKeys.filter((k) => k !== 'catalog_id')).toEqual(importKeys);
    // 가져온 요소에는 카탈로그가 없다 — 예약값을 심으면 한 필드가 두 뜻을 갖는다.
    expect(importKeys).not.toContain('catalog_id');
  });

  it('가져온 요소를 고르면 **8핸들**이 붙는다 — 카탈로그 경로와 같은 컨트롤이다', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));
    stubOverlayRect();

    const created = liveElements().slice(3) as PathElement[];
    // 놓은 직후에는 **둘 다** 골라져 있고, 8핸들은 하나만 골랐을 때 붙는다(오버레이의 규칙).
    // 그래서 먼저 빈 자리를 눌러 선택을 비우고 큰 도형 **하나만** 다시 잡는다.
    expect(liveSelection()).toHaveLength(2);
    expect(screen.queryByTestId('canvas-handle-se')).toBeNull();

    // 상자 밖이면서 씨앗 셋에서도 먼 자리 — `HIT_TOLERANCE_PX`(6) 안에 아무것도 없다.
    send('pointerdown', 240, 20);
    expect(liveSelection()).toEqual([]);

    // 큰 도형만 덮는 자리(작은 초록 사각은 상자의 x 0.17~0.36 · y 0.18~0.40 에 있다).
    const g = created[0]!.geometry;
    send('pointerdown', (g.x + g.w * 0.8) * 0.5, (g.y + g.h * 0.8) * 0.5);
    expect(liveSelection()).toEqual([created[0]!.id]);

    expect(screen.getByTestId(`canvas-selection-${created[0]!.id}`)).toBeTruthy();
    for (const h of ['nw', 'n', 'ne', 'w', 'e', 'sw', 's', 'se']) {
      expect(screen.queryByTestId(`canvas-handle-${h}`), h).not.toBeNull();
    }
  });
});

// --- 취소 -----------------------------------------------------------------

describe('취소는 무동작이다 (REQ-06 · 불변식 K15)', () => {
  it('`onElementsChange` 가 한 번도 불리지 않고 배열이 그대로다', async () => {
    render(<Harness />);
    await chooseDoc();
    expect(changes).toBe(0);
    fireEvent.click(screen.getByTestId('canvas-svg-import-cancel'));

    expect(changes).toBe(0);
    expect(liveElements()).toEqual(SEED);
    expect(liveSelection()).toEqual([]);
  });

  it('거절된 파일도 배열을 건드리지 않는다', async () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), {
      target: { files: [new File(['<note xmlns="urn:x"/>'], 'a.svg')] },
    });
    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-refused')).toBeTruthy());
    expect(changes).toBe(0);
    expect(liveElements()).toEqual(SEED);
  });
});
