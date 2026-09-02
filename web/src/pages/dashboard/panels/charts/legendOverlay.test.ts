// legendOverlay 순수 모듈 테스트 — 겹쳐 뜨는 범례의 자리 계산.
//
// 오프셋이 백분율인 것이 이 모듈의 핵심이다. 픽셀이던 시절에는 설정 미리보기(≈1870px)와
// 대시보드 패널(≈1500px 이하)에서 같은 값이 다른 자리를 가리켰고, 좁은 패널에서는 범례가
// 밖으로 나가 보이지 않았다.

import { describe, it, expect } from 'vitest';

import {
  clampInlineLegendOffset,
  clampLegendOffsets,
  clampStoredLegendOffset,
  inlineLegendOffsetStyle,
  LEGEND_OFFSET_SAFETY_LIMIT,
  legendOffsetBounds,
  legendOverlayStyle,
} from './legendOverlay';

describe('legendOverlayStyle', () => {
  it('위·아래는 가로 가운데 정렬 — 되물림은 transform 이 맡는다', () => {
    expect(legendOverlayStyle('top', 0, 0)).toEqual({
      left: 'calc(50% + 0%)',
      top: '0%',
      transform: 'translateX(-50%)',
    });
    expect(legendOverlayStyle('bottom', 0, 0)).toEqual({
      left: 'calc(50% + 0%)',
      bottom: '0%',
      transform: 'translateX(-50%)',
    });
  });

  it('좌·우는 세로 가운데 정렬', () => {
    expect(legendOverlayStyle('left', 0, 0)).toEqual({
      left: '0%',
      top: 'calc(50% + 0%)',
      transform: 'translateY(-50%)',
    });
    expect(legendOverlayStyle('right', 0, 0)).toEqual({
      right: '0%',
      top: 'calc(50% + 0%)',
      transform: 'translateY(-50%)',
    });
  });

  it('오프셋은 담는 상자 기준 백분율로 들어간다 — transform 백분율(요소 자신 기준)이 아니다', () => {
    const s = legendOverlayStyle('bottom', 12, -8);
    expect(s.left).toBe('calc(50% + 12%)');
    // 아래에 붙인 축은 반대로 자라므로 부호를 뒤집어 넣는다(y 가 크면 화면상 아래).
    expect(s.bottom).toBe('8%');
  });

  it('부호는 화면 방향으로 통일된다 — x 가 크면 오른쪽, y 가 크면 아래', () => {
    // 오른쪽에 붙였을 때: x 가 크면(오른쪽) `right` 는 줄어야 한다.
    expect(legendOverlayStyle('right', 10, 0).right).toBe('-10%');
    // 위에 붙였을 때: y 가 크면(아래) `top` 이 커진다.
    expect(legendOverlayStyle('top', 0, 10).top).toBe('10%');
  });
});

describe('legendOffsetBounds', () => {
  const box = { width: 1000, height: 500 };

  it('가운데 정렬 축은 모서리가 가장자리에 닿는 곳까지 — 고정 상한처럼 여백을 남기지 않는다', () => {
    // 폭 100 짜리 범례 → 중심이 ±45% 까지 가면 모서리가 정확히 가장자리에 닿는다.
    const b = legendOffsetBounds('bottom', box, { width: 100, height: 40 });
    expect(b.maxX).toBeCloseTo(45);
    expect(b.minX).toBeCloseTo(-45);
  });

  it('붙인 축은 그 변이 0 — 한쪽으로만 움직인다', () => {
    const b = legendOffsetBounds('bottom', box, { width: 100, height: 50 });
    // 아래에 붙였으므로 아래로는 더 갈 수 없고(0), 위로는 높이만큼 뺀 만큼 간다.
    expect(b.maxY).toBe(0);
    expect(b.minY).toBeCloseTo(-90);
  });

  it('위에 붙이면 부호가 뒤집힌다', () => {
    const b = legendOffsetBounds('top', box, { width: 100, height: 50 });
    expect(b.minY).toBe(0);
    expect(b.maxY).toBeCloseTo(90);
  });

  it('좌·우 배치는 세로가 가운데 정렬, 가로가 붙인 축이다', () => {
    const left = legendOffsetBounds('left', box, { width: 100, height: 50 });
    expect(left.minX).toBe(0);
    expect(left.maxX).toBeCloseTo(90);
    expect(left.maxY).toBeCloseTo(45);

    const right = legendOffsetBounds('right', box, { width: 100, height: 50 });
    expect(right.maxX).toBe(0);
    expect(right.minX).toBeCloseTo(-90);
  });

  it('범례가 상자보다 크거나 상자를 잴 수 없으면 움직일 여지가 없다', () => {
    expect(legendOffsetBounds('bottom', box, { width: 2000, height: 40 }).maxX).toBe(0);
    expect(legendOffsetBounds('bottom', { width: 0, height: 0 }, { width: 10, height: 10 })).toEqual(
      { minX: 0, maxX: 0, minY: 0, maxY: 0 },
    );
  });
});

describe('clampLegendOffsets', () => {
  const box = { width: 1000, height: 500 };
  const legend = { width: 100, height: 50 };

  it('범위를 넘기면 가장자리에 붙인다', () => {
    expect(clampLegendOffsets('bottom', 999, 999, box, legend)).toEqual({ x: 45, y: 0 });
  });

  it('범위 안이면 그대로 둔다', () => {
    expect(clampLegendOffsets('bottom', 12, -20, box, legend)).toEqual({ x: 12, y: -20 });
  });

  it('수가 아니면 0', () => {
    expect(clampLegendOffsets('bottom', Number.NaN, Number.NaN, box, legend)).toEqual({
      x: 0,
      y: 0,
    });
  });
});

describe('clampStoredLegendOffset', () => {
  it('저장값에는 성긴 안전 상한만 건다 — 패널이 줄어도 통째로 사라지지 않게', () => {
    expect(clampStoredLegendOffset(999)).toBe(LEGEND_OFFSET_SAFETY_LIMIT);
    expect(clampStoredLegendOffset(-999)).toBe(-LEGEND_OFFSET_SAFETY_LIMIT);
    expect(clampStoredLegendOffset(12)).toBe(12);
  });

  it('수가 아니면 0 — 구 config 에는 이 키가 없다', () => {
    expect(clampStoredLegendOffset(undefined)).toBe(0);
    expect(clampStoredLegendOffset('12')).toBe(0);
  });
});

describe('inlineLegendOffsetStyle — 흐름에 남는 범례', () => {
  it('변위가 없으면 스타일을 붙이지 않는다 — 저장된 대시보드의 그림이 변하지 않는다', () => {
    expect(inlineLegendOffsetStyle(0, 0)).toBeUndefined();
  });

  it('상대 배치로 민다 — 자리를 새로 잡지 않고 그 자리에서 밀기만 한다', () => {
    expect(inlineLegendOffsetStyle(10, -5)).toEqual({
      position: 'relative',
      left: '10%',
      top: '-5%',
    });
  });
});

describe('clampInlineLegendOffset', () => {
  // 상자 1000×500, 범례는 그 안 (100,400) 자리에 200×50 으로 그려져 있다.
  const container = { left: 0, top: 0, width: 1000, height: 500 };
  const legend = { left: 100, top: 400, width: 200, height: 50 };
  const base = { x: 0, y: 0 };

  it('남은 여백 안에서는 그대로 통과한다', () => {
    expect(clampInlineLegendOffset(base, { x: 5, y: -10 }, container, legend)).toEqual({
      x: 5,
      y: -10,
    });
  });

  it('왼쪽 끝을 넘지 않는다 — 범례 왼면이 상자 왼면에 닿는 곳이 한계다', () => {
    // 여백 -100px / 1000px = -10%.
    expect(clampInlineLegendOffset(base, { x: -50, y: 0 }, container, legend).x).toBe(-10);
  });

  it('오른쪽 끝을 넘지 않는다', () => {
    // 오른쪽 여백 1000 - 300 = 700px → 70%.
    expect(clampInlineLegendOffset(base, { x: 200, y: 0 }, container, legend).x).toBe(70);
  });

  it('아래쪽도 같은 규칙이다', () => {
    // 아래 여백 500 - 450 = 50px / 500px = 10%.
    expect(clampInlineLegendOffset(base, { x: 0, y: 99 }, container, legend).y).toBe(10);
  });

  it('이미 밀려 있으면 그 자리에서 남은 만큼만 더 간다', () => {
    // base 20% 에서 오른쪽 여백이 70% 남았으므로 총 90% 까지.
    const moved = { x: 20, y: 0 };
    expect(clampInlineLegendOffset(moved, { x: 500, y: 0 }, container, legend).x).toBe(90);
  });

  it('범례가 상자보다 크면 움직이지 않는다 — 죌 수 없는 값을 통과시키면 사라진다', () => {
    const huge = { left: -50, top: -20, width: 1200, height: 600 };
    expect(clampInlineLegendOffset({ x: 3, y: 4 }, { x: 99, y: 99 }, container, huge)).toEqual({
      x: 3,
      y: 4,
    });
  });

  it('상자를 잴 수 없으면 제자리다', () => {
    const zero = { left: 0, top: 0, width: 0, height: 0 };
    expect(clampInlineLegendOffset({ x: 7, y: 8 }, { x: 1, y: 2 }, zero, legend)).toEqual({
      x: 7,
      y: 8,
    });
  });

  it('NaN 은 제자리로 떨어뜨린다', () => {
    expect(clampInlineLegendOffset({ x: 2, y: 3 }, { x: NaN, y: NaN }, container, legend)).toEqual({
      x: 2,
      y: 3,
    });
  });
});
