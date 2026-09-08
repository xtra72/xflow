// panelEditAlign 단위 테스트.
//
// @spec SPEC-CHART-004 AC-33 / AC-34 / AC-35

import { describe, it, expect } from 'vitest';

import {
  GRID_STEP_PERCENT,
  computeAlignPatches,
  snapOffsetToGrid,
  type PanelElementBox,
} from './panelEditAlign';
import { PANEL_OFFSET_LIMIT } from './panelGeometry';
import { STAT_OFFSET_LIMIT } from './statLayout';

/** 기준 상자 200×100, 화면 원점에 놓인 상황. */
const BOUNDS = { left: 0, top: 0, width: 200, height: 100 };

function box(over: Partial<PanelElementBox> = {}): PanelElementBox {
  return {
    kind: 'value',
    rect: { left: 0, top: 0, width: 50, height: 20 },
    offsetX: 0,
    offsetY: 0,
    ...over,
  };
}

describe('snapOffsetToGrid — 중심을 격자에 맞춘다 (AC-33)', () => {
  it('격자 간격은 10% 다', () => {
    expect(GRID_STEP_PERCENT).toBe(10);
  });

  /** 폭 200 상자에서 흐름상 중심이 50%(=100px)인 요소. */
  const centered = { offset: 0, rectStart: 75, rectLen: 50 };
  const BOUNDS_X = { start: 0, len: 200 };

  it('중심이 격자에 이미 있으면 오프셋이 그대로다', () => {
    expect(snapOffsetToGrid(0, centered, BOUNDS_X)).toBe(0);
  });

  it('가까운 격자선으로 끌어당긴다', () => {
    // 새 오프셋 3%p → 중심 53% → 가장 가까운 격자 50% 로 되붙는다.
    expect(snapOffsetToGrid(3, centered, BOUNDS_X)).toBeCloseTo(0, 6);
    // 12%p → 중심 62% → 60% 로 붙어 오프셋 10%p.
    expect(snapOffsetToGrid(12, centered, BOUNDS_X)).toBeCloseTo(10, 6);
  });

  it('잡는 순간의 오프셋으로 흐름 자리를 되짚는다 — 이동량을 두 번 반영하지 않는다', () => {
    // 잡을 때 이미 12%p 가 반영된 상자(left 99). 새 오프셋도 12%p 면 제자리이므로
    // 중심 62% → 60% 로 2%p 만 되돌린다.
    const base = { offset: 12, rectStart: 99, rectLen: 50 };
    expect(snapOffsetToGrid(12, base, BOUNDS_X)).toBeCloseTo(10, 6);
    // 잡을 때의 오프셋을 0 으로 잘못 넘기면 이동량이 두 번 반영되어 다른 답이 나온다.
    expect(snapOffsetToGrid(12, { ...base, offset: 0 }, BOUNDS_X)).not.toBeCloseTo(10, 6);
  });

  it('기준 상자를 잴 수 없으면 그대로 둔다', () => {
    expect(snapOffsetToGrid(12, centered, { start: 0, len: 0 })).toBe(12);
  });

  it('상한을 밝히지 않으면 ±40 을 넘기지 않는다', () => {
    expect(
      snapOffsetToGrid(48, { offset: 48, rectStart: 400, rectLen: 50 }, BOUNDS_X),
    ).toBeLessThanOrEqual(PANEL_OFFSET_LIMIT);
  });

  it('넘겨받은 상한을 쓴다 — 글자 덩어리는 ±50 까지 간다', () => {
    expect(
      snapOffsetToGrid(48, { offset: 48, rectStart: 400, rectLen: 50 }, BOUNDS_X, STAT_OFFSET_LIMIT),
    ).toBeLessThanOrEqual(STAT_OFFSET_LIMIT);
  });

  it('격자로 끌어당긴 값도 상한을 넘지 않는다 — 스냅이 죄기를 되돌리지 못한다', () => {
    // 흐름상 시작 자리 71, 폭 50 인 요소를 38%p 만큼 끈 상황: 중심이 86% 라 90% 로 붙어
    // 오프셋이 42%p 가 된다. 그림(±40)이면 40 에서 멈춰야 하고, 글자 덩어리(±50)면
    // 42 를 그대로 쓴다. 여기서 상한을 흘리면 파이 그림이 40 을 넘겨 저장된다.
    const base = { offset: 0, rectStart: 71, rectLen: 50 };
    expect(snapOffsetToGrid(38, base, BOUNDS_X)).toBeCloseTo(PANEL_OFFSET_LIMIT, 6);
    expect(snapOffsetToGrid(38, base, BOUNDS_X, STAT_OFFSET_LIMIT)).toBeCloseTo(42, 6);
  });
});

describe('computeAlignPatches — 요소끼리 맞춘다 (AC-34)', () => {
  /** 가로로 어긋난 세 요소. left 20 / 60 / 100, 폭 50 / 30 / 40. */
  const three: PanelElementBox[] = [
    box({ kind: 'value', rect: { left: 20, top: 0, width: 50, height: 20 } }),
    box({ kind: 'delta', rect: { left: 60, top: 30, width: 30, height: 14 } }),
    box({ kind: 'stats', rect: { left: 100, top: 60, width: 40, height: 14 } }),
  ];

  it('start 는 가장 앞선 변에 맞춘다', () => {
    const patches = computeAlignPatches(three, BOUNDS, 'horizontal', 'start');
    // 바깥 상자의 왼쪽은 20. 각자 그만큼 왼쪽으로 간다(폭 200 기준 백분율).
    expect(patches).toEqual([
      { kind: 'value', offsetX: 0 },
      { kind: 'delta', offsetX: -20 },
      { kind: 'stats', offsetX: -40 },
    ]);
  });

  it('end 는 가장 뒤선 변에 맞춘다', () => {
    const patches = computeAlignPatches(three, BOUNDS, 'horizontal', 'end');
    // 바깥 상자의 오른쪽은 140(=100+40). 각자 오른쪽 변을 거기 맞춘다.
    expect(patches).toEqual([
      { kind: 'value', offsetX: 35 }, // 140-50=90, 90-20=70px = 35%
      { kind: 'delta', offsetX: 25 }, // 140-30=110, 110-60=50px = 25%
      { kind: 'stats', offsetX: 0 },
    ]);
  });

  it('center 는 바깥 상자의 가운데에 맞춘다 — 가장 왼쪽 요소가 기준이 아니다', () => {
    const patches = computeAlignPatches(three, BOUNDS, 'horizontal', 'center');
    // 바깥 상자 20~140, 가운데 80.
    expect(patches).toEqual([
      { kind: 'value', offsetX: 17.5 }, // 80-25=55, 55-20=35px
      { kind: 'delta', offsetX: 2.5 }, // 80-15=65, 65-60=5px
      { kind: 'stats', offsetX: -20 }, // 80-20=60, 60-100=-40px
    ]);
  });

  it('세로 축은 offsetY 만 담는다 — 가로 오프셋을 건드리지 않는다', () => {
    const patches = computeAlignPatches(three, BOUNDS, 'vertical', 'start');
    expect(patches[0]).toEqual({ kind: 'value', offsetY: 0 });
    expect(patches[1]).toEqual({ kind: 'delta', offsetY: -30 }); // 30px / 높이 100
    for (const p of patches) expect(p).not.toHaveProperty('offsetX');
  });

  it('이미 놓인 오프셋 위에서 계산한다', () => {
    const shifted = [
      box({ kind: 'value', rect: { left: 20, top: 0, width: 50, height: 20 }, offsetX: 10 }),
      box({ kind: 'delta', rect: { left: 60, top: 30, width: 30, height: 14 }, offsetX: -5 }),
    ];
    const patches = computeAlignPatches(shifted, BOUNDS, 'horizontal', 'start');
    // value 는 이미 왼쪽 끝이라 그대로, delta 는 -5 에서 20px(=10%p) 더 왼쪽으로.
    expect(patches).toEqual([
      { kind: 'value', offsetX: 10 },
      { kind: 'delta', offsetX: -25 },
    ]);
  });

  it('요소가 밝힌 상한으로 죈다 — 글자 덩어리는 ±50', () => {
    const far = [
      box({ kind: 'value', rect: { left: 0, top: 0, width: 10, height: 10 } }),
      box({
        kind: 'delta',
        rect: { left: 190, top: 0, width: 10, height: 10 },
        offsetX: 40,
        limit: STAT_OFFSET_LIMIT,
      }),
    ];
    const patches = computeAlignPatches(far, BOUNDS, 'horizontal', 'start');
    expect(patches[1]!.offsetX).toBe(-STAT_OFFSET_LIMIT);
  });

  it('상한을 밝히지 않으면 ±40 으로 죈다 — 영역을 채우는 그림이 안전한 기본값이다', () => {
    // 파이 그림·바 그림 영역·게이지 상자는 읽는 쪽(`readPanelOffset`)이 ±40 으로 읽는다.
    // 정렬이 더 느슨하게 죄면 맞춘 자리가 다음에 읽힐 때 되돌아간다.
    const far = [
      box({ kind: 'chart', rect: { left: 0, top: 0, width: 10, height: 10 } }),
      box({ kind: 'legend', rect: { left: 190, top: 0, width: 10, height: 10 }, offsetX: 40 }),
    ];
    const patches = computeAlignPatches(far, BOUNDS, 'horizontal', 'start');
    expect(patches[1]!.offsetX).toBe(-PANEL_OFFSET_LIMIT);
  });

  it('상한이 다른 요소가 섞여도 각자의 상한을 쓴다', () => {
    // 같은 정렬 한 번에 그림(±40)과 범례(±50)가 함께 걸린다.
    const far = [
      box({ kind: 'chart', rect: { left: 0, top: 0, width: 10, height: 10 } }),
      box({ kind: 'chart2', rect: { left: 190, top: 0, width: 10, height: 10 }, offsetX: 40 }),
      box({
        kind: 'legend',
        rect: { left: 190, top: 0, width: 10, height: 10 },
        offsetX: 40,
        limit: STAT_OFFSET_LIMIT,
      }),
    ];
    const patches = computeAlignPatches(far, BOUNDS, 'horizontal', 'start');
    expect(patches[1]!.offsetX).toBe(-PANEL_OFFSET_LIMIT);
    expect(patches[2]!.offsetX).toBe(-STAT_OFFSET_LIMIT);
  });

  it('요소가 2개 미만이면 맞출 상대가 없다', () => {
    expect(computeAlignPatches([box()], BOUNDS, 'horizontal', 'start')).toEqual([]);
    expect(computeAlignPatches([], BOUNDS, 'horizontal', 'start')).toEqual([]);
  });

  it('기준 상자를 잴 수 없으면 아무것도 하지 않는다', () => {
    const patches = computeAlignPatches(three, { ...BOUNDS, width: 0 }, 'horizontal', 'start');
    expect(patches).toEqual([]);
  });
});
