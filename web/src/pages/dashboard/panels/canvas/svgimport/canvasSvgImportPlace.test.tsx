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
import { parseScratchpadEntry, type ScratchpadEntry } from '../scratchpad/scratchpadTypes';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import type { StageSize } from '../canvasGeometry';
import { planSvgImport } from './svgImportPlan';

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
/** 씨앗 첫 요소의 왼쪽 변. 정렬 시험이 이 수를 표적으로 쓴다. */
const SEED_LEFT = 10;

const SEED: readonly CanvasElement[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: SEED_LEFT, y: 10, w: 20, h: 20 }, style: { fill: '#a' } },
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
    const planned = planSvgImport(DOC, CANVAS);
    expect(planned.ok).toBe(true);
    if (!planned.ok) return;
    // **상자는 도형마다 다르되 계단은 하나다**(결함 D3 정정). 옛 시험은 "두 기하가 같다" 로
    // 계단을 재었고, 그것은 상자가 전부 같다는 우연에 기댄 단언이었다. 재야 하는 것은 값이
    // 아니라 **델타**이며, 그 편이 강하다 — 상자가 저마다 달라도 계단만을 잰다.
    const deltas = created.map((el, i) => ({
      dx: el.geometry.x - planned.shapes[i]!.box.x,
      dy: el.geometry.y - planned.shapes[i]!.box.y,
    }));
    for (const d of deltas) expect(d).toEqual(deltas[0]);
    // 그리고 계단이 실제로 **0 이 아니다**(이미 놓인 셋 때문) — 0 이면 이 시험이 무력하다.
    expect(deltas[0]!.dx).toBeGreaterThan(0);
    expect(deltas[0]!.dx).toBe(deltas[0]!.dy);
    // 켜져 있음: 두 상자가 **다르다**. 둘째 사각(60×40)은 문서(317×181)의 일부만 차지한다.
    expect(created[1]!.geometry.w).toBeLessThan(created[0]!.geometry.w);
    // 첫 도형이 `viewBox` 를 가득 채우므로 그 상자가 문서 종횡비(317 : 181)를 든다.
    const g = created[0]!.geometry;
    expect(g.w / g.h).toBeCloseTo(317 / 181, 1);
    // 둘째 도형은 제 종횡비(60 : 40)를 든다 — 문서의 것이 아니다. 이 한 줄이 없으면
    // 상자를 여전히 문서 것으로 주는 결함이 위 단언들을 전부 통과한다.
    expect(created[1]!.geometry.w / created[1]!.geometry.h).toBeCloseTo(60 / 40, 1);
  });

  it('가져온 요소들끼리도 정렬이 **움직인다** — 상자가 도형마다이기 때문이다 (결함 D3)', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    // 놓자마자 새 id 전부가 선택이다(REQ-06) — 그래서 고르는 몸짓 없이 정렬이 곧바로 선다.
    const created = liveElements().slice(3) as PathElement[];
    expect(liveSelection().sort()).toEqual(created.map((el) => el.id).sort());
    const before = created.map((el) => el.geometry.x);
    // 켜져 있음: 두 왼쪽 변이 애초에 다르다. **공유 상자 시절에는 같았고**, 그래서
    // `alignDeltas` 의 델타가 전부 0 이라 정렬 단추가 살아 있는 채로 죽어 있었다.
    expect(before[0]).not.toBe(before[1]);

    const align = screen.getByTestId('canvas-align-left') as HTMLButtonElement;
    expect(align.disabled).toBe(false);
    fireEvent.click(align);

    const after = (liveElements().slice(3) as PathElement[]).map((el) => el.geometry.x);
    // 두 왼쪽 변이 맞았다. 그리고 **실제로 움직였다** — 맞춰졌다는 단언만으로는 "애초에
    // 같아서 아무 일도 없었다" 를 구분하지 못한다(그것이 이 결함의 형상이었다).
    expect(after[0]).toBe(after[1]);
    expect(after).not.toEqual(before);
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

  it('정렬 · 순서 단추가 가져온 요소에도 **그대로 닿는다** (AC-09)', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    stubOverlayRect();
    // **가져온 요소 하나와 씨앗 요소 하나**를 함께 고른다 — 이 시험이 재는 것은 "정렬이
    // 출처를 구분하지 않는가" 이므로 두 출처가 섞여 있어야 한다. 가져온 것들끼리의 정렬은
    // 바로 위 시험이 따로 잰다(그것이 결함 D3 의 세 번째 증상이었다).
    send('pointerdown', 240, 20); // 빈 자리 — 선택을 비운다
    const created = liveElements().slice(3) as PathElement[];
    const g = created[0]!.geometry;
    send('pointerdown', (g.x + g.w * 0.8) * 0.5, (g.y + g.h * 0.8) * 0.5); // 가져온 큰 도형
    send('pointerdown', 10, 10, { shiftKey: true }); // 씨앗 el-1 (px 5..15 × 5..15)
    expect(liveSelection().sort()).toEqual(['el-1', created[0]!.id]);

    const align = screen.getByTestId('canvas-align-left') as HTMLButtonElement;
    expect(align.disabled).toBe(false);
    // 켜져 있음: 두 상자의 x 가 애초에 다르다(같으면 정렬이 아무것도 하지 않는다).
    expect(g.x).not.toBe(SEED_LEFT);
    fireEvent.click(align);

    const moved = (liveElements().find((el) => el.id === created[0]!.id) as PathElement).geometry;
    // 가져온 요소가 씨앗 요소의 왼쪽 변에 맞춰졌다 — 정렬이 출처를 구분하지 않는다.
    expect(moved.x).toBe(SEED_LEFT);
    // 함께 고르지 않은 나머지 가져온 요소는 **제 자리 그대로**다. 옛 시험은 이 자리를
    // `g.x`(첫 요소의 x)와 견주었는데, 그것이 통했던 것은 두 상자가 같았기 때문이다 —
    // 상자가 도형마다인 지금 견줄 것은 **그 요소 자신의 놓인 자리**다.
    expect((liveElements().find((el) => el.id === created[1]!.id) as PathElement).geometry.x).toBe(
      created[1]!.geometry.x,
    );

    // 순서도 관측된다: 뒤로 보내면 배열 앞으로 간다(배열 순서가 유일한 z-order 다).
    const idsBefore = liveElements().map((el) => el.id);
    const picked = liveSelection().sort();
    fireEvent.click(screen.getByTestId('canvas-order-back'));
    const idsAfter = liveElements().map((el) => el.id);
    expect(idsAfter).not.toEqual(idsBefore);
    // 고른 둘(씨앗 하나 · 가져온 하나)이 나란히 맨 앞에 섰다 — 순서 단추도 출처를 구분하지
    // 않는다. 켜져 있음: 그 둘 가운데 하나는 가져온 요소다.
    expect(idsAfter.slice(0, 2).sort()).toEqual(picked);
    expect(picked).toContain(created[0]!.id);
    // 요소가 사라지거나 늘지 않았다.
    expect(new Set(idsAfter)).toEqual(new Set(idsBefore));
  });

  it('서랍에 넣어 왕복시켜도 명령과 스타일이 **깊은 비교로 같다** (AC-E12 · REQ-08)', async () => {
    render(<Harness />);
    await chooseDoc();
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    const created = liveElements().slice(3) as PathElement[];
    expect(created).toHaveLength(2);
    // 008 의 서랍 항목 형상 그대로 담아 **직렬화 왕복**을 시킨다. 가져온 요소가 카탈로그
    // 경로와 구별되지 않는다는 REQ-08 의 기계적 확인이다 — 구별되었다면 파서가 무언가를
    // 잃거나 기본값으로 갈아 끼운다.
    const entry: ScratchpadEntry = {
      id: 'sp-1',
      name: '가져온 그림',
      created: 0,
      origin: { x: created[0]!.geometry.x, y: created[0]!.geometry.y },
      elements: created,
    };
    const reopened = parseScratchpadEntry(JSON.parse(JSON.stringify(entry)) as unknown);
    expect(reopened).not.toBeNull();
    expect(reopened?.elements).toHaveLength(2);
    for (const [i, el] of (reopened?.elements ?? []).entries()) {
      expect(el.kind, `#${i}`).toBe('path');
      expect((el as PathElement).path, `#${i}`).toEqual(created[i]!.path);
      expect(el.style, `#${i}`).toEqual(created[i]!.style);
      expect(el.geometry, `#${i}`).toEqual(created[i]!.geometry);
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
