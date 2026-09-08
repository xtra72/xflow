// 미리보기 스테이지 기하 검증.
//
// 잠그는 것: 패널이 그리드 단위 수만큼(마진 포함) 자리를 차지하고, 채움/맞춤이
// 각각 영역을 채우거나 전부 보이게 하며, 그리드 간격이 대시보드와 같은 비율로
// 함께 축소되는 것.

import { describe, expect, it } from 'vitest';

import { GRID_MARGIN_PX } from './gridGeometry';
import {
  NOMINAL_CELL_PX,
  computeFitScale,
  computePreviewStage,
  initialPreviewZoom,
} from './previewStage';

const CELL = 100;
const base = { cell: CELL, areaW: 800, areaH: 600, mode: 'fit' as const, zoom: 1 };

describe('computeFitScale', () => {
  it('패널 전체가 영역에 들어가는 배율을 준다', () => {
    const scale = computeFitScale({ ...base, w: 24, h: 8 })!;
    const stage = computePreviewStage({ ...base, w: 24, h: 8, zoom: scale })!;

    expect(stage.screenW).toBeLessThanOrEqual(base.areaW + 1e-6);
    expect(stage.screenH).toBeLessThanOrEqual(base.areaH + 1e-6);
  });

  it('영역보다 작은 패널은 1 을 넘는 배율을 준다 — 하한을 좁히지 않는다', () => {
    expect(computeFitScale({ ...base, w: 2, h: 2 })!).toBeGreaterThan(1);
  });

  it('크기를 모르면 null', () => {
    expect(computeFitScale({ ...base, w: 0, h: 3 })).toBeNull();
    expect(computeFitScale({ ...base, w: 4, h: 3, areaW: 0 })).toBeNull();
  });
});

describe('computePreviewStage', () => {
  it('패널은 그리드 단위 수만큼 자리를 차지한다 — 내부 마진 포함', () => {
    const stage = computePreviewStage({ ...base, w: 4, h: 3 })!;
    expect(stage.pxW).toBe(4 * CELL + GRID_MARGIN_PX * 3);
    expect(stage.pxH).toBe(3 * CELL + GRID_MARGIN_PX * 2);
  });

  it('맞춤 100% 는 대시보드와 같은 크기다 — 영역에 맞추지 않는다', () => {
    const stage = computePreviewStage({ ...base, w: 24, h: 8 })!;
    expect(stage.scaleX).toBe(1);
    expect(stage.scaleY).toBe(1);
    // 영역(800×600)보다 크지만 줄이지 않는다 — 실제 크기를 그대로 보여준다.
    expect(stage.screenW).toBe(stage.pxW);
    expect(stage.screenH).toBe(stage.pxH);
    expect(stage.screenW).toBeGreaterThan(base.areaW);
  });

  it('맞춤에서 줌을 전체 배율까지 낮추면 영역 안에 들어온다', () => {
    const fitScale = computeFitScale({ ...base, w: 24, h: 8 })!;
    const stage = computePreviewStage({ ...base, w: 24, h: 8, zoom: fitScale })!;

    expect(stage.screenW).toBeLessThanOrEqual(base.areaW + 1e-6);
    expect(stage.screenH).toBeLessThanOrEqual(base.areaH + 1e-6);
    // 한 축은 영역에 꼭 맞는다(둘 다 남으면 더 키울 수 있다는 뜻).
    const touches =
      Math.abs(stage.screenW - base.areaW) < 1e-6 || Math.abs(stage.screenH - base.areaH) < 1e-6;
    expect(touches).toBe(true);
  });

  it('채움은 영역을 정확히 채운다 — 넘치지도 남기지도 않는다', () => {
    const stage = computePreviewStage({ ...base, w: 24, h: 8, mode: 'fill' })!;
    // 우하단 손잡이가 영역 모서리에 놓이려면 화면 크기가 영역과 같아야 한다.
    expect(stage.screenW).toBeCloseTo(base.areaW, 6);
    expect(stage.screenH).toBeCloseTo(base.areaH, 6);
  });

  it('채움은 축마다 배율이 다르다 — 묶어 확대하면 한쪽이 영역을 넘친다', () => {
    const stage = computePreviewStage({ ...base, w: 24, h: 8, mode: 'fill' })!;
    expect(stage.scaleX).not.toBeCloseTo(stage.scaleY, 6);
  });

  it('맞춤은 두 축의 배율이 같다 — 형태가 보존된다', () => {
    const stage = computePreviewStage({ ...base, w: 24, h: 8, zoom: 0.3 })!;
    expect(stage.scaleX).toBeCloseTo(stage.scaleY, 10);
  });

  it.each([['fit' as const], ['fill' as const]])(
    '%s: 셀과 간격이 화면 크기를 정확히 메운다 — 대시보드와 같은 비율',
    (mode) => {
      const stage = computePreviewStage({ ...base, w: 24, h: 8, mode, zoom: 0.4 })!;
      // 셀 w개 + 간격 (w-1)개 = 화면 폭 (높이도 같은 식)
      expect(stage.cellW * 24 + stage.gapX * 23).toBeCloseTo(stage.screenW, 6);
      expect(stage.cellH * 8 + stage.gapY * 7).toBeCloseTo(stage.screenH, 6);
      // 셀:간격 비는 대시보드와 같다.
      expect(stage.cellW / stage.gapX).toBeCloseTo(CELL / GRID_MARGIN_PX, 10);
      expect(stage.cellH / stage.gapY).toBeCloseTo(CELL / GRID_MARGIN_PX, 10);
    },
  );

  it('줌은 두 축 배율에 곱해진다 — 영역보다 큰 패널도 줄여 전체를 볼 수 있다', () => {
    const fill = computePreviewStage({ ...base, w: 24, h: 8, mode: 'fill' })!;
    const zoomedOut = computePreviewStage({ ...base, w: 24, h: 8, mode: 'fill', zoom: 0.5 })!;

    expect(zoomedOut.scaleX).toBeCloseTo(fill.scaleX * 0.5, 10);
    expect(zoomedOut.scaleY).toBeCloseTo(fill.scaleY * 0.5, 10);
    expect(zoomedOut.screenW).toBeCloseTo(fill.screenW * 0.5, 6);
  });

  it('셀 크기를 모르면 기준 셀로 대체한다 — 비율은 유지된다', () => {
    const stage = computePreviewStage({ ...base, w: 4, h: 3, cell: 0 })!;
    expect(stage.pxW).toBe(4 * NOMINAL_CELL_PX + GRID_MARGIN_PX * 3);
  });

  it.each([
    ['그리드 단위가 0', { w: 0, h: 3 }],
    ['높이가 0', { w: 4, h: 0 }],
    ['영역 미실측', { w: 4, h: 3, areaW: 0 }],
  ])('%s 이면 렌더를 미룬다', (_label, patch) => {
    expect(computePreviewStage({ ...base, ...patch })).toBeNull();
  });
});

describe('initialPreviewZoom', () => {
  it('맞춤: 영역보다 큰 패널은 전체가 보이는 배율에서 시작한다', () => {
    // 100% 로 열면 가운데 일부만 보여 패널 모양을 알 수 없다.
    expect(initialPreviewZoom('fit', 0.22)).toBeCloseTo(0.22, 10);
  });

  it('맞춤: 영역보다 작은 패널은 확대하지 않는다 — 100% 가 실제 크기다', () => {
    expect(initialPreviewZoom('fit', 1.8)).toBe(1);
  });

  it('채움: 100% 가 이미 영역에 꼭 맞으므로 1 이다', () => {
    expect(initialPreviewZoom('fill', 0.22)).toBe(1);
  });

  it('배율을 모르면(실측 전) 1 이다', () => {
    expect(initialPreviewZoom('fit', null)).toBe(1);
  });
});
