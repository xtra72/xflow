// pieDrag 순수 모듈 테스트 — 끌어 옮긴 범례·파이가 패널 밖으로 나가지 않는지.

import { describe, it, expect } from 'vitest';

import { clampLegendOffset, legendTransform } from './pieDrag';

describe('clampLegendOffset', () => {
  it('기준 변의 절반을 넘지 않는다 — 더 밀면 다시 잡을 수 없다', () => {
    expect(clampLegendOffset(500, 200)).toBe(100);
    expect(clampLegendOffset(-500, 200)).toBe(-100);
  });

  it('범위 안이면 그대로 둔다', () => {
    expect(clampLegendOffset(37, 200)).toBe(37);
  });

  it('기준 변을 잴 수 없으면 0 — 죌 수 없는 값을 두면 범례가 사라진 것처럼 보인다', () => {
    expect(clampLegendOffset(37, 0)).toBe(0);
    expect(clampLegendOffset(37, Number.NaN)).toBe(0);
  });

  it('오프셋이 수가 아니면 0', () => {
    expect(clampLegendOffset(Number.NaN, 200)).toBe(0);
  });
});

describe('legendTransform', () => {
  it('하단은 가로만 되물린다 — 아래에 붙이고 가운데 정렬한다', () => {
    expect(legendTransform('bottom', 0, 0)).toBe('translate(calc(-50% + 0px), 0px)');
  });

  it('좌·우는 세로만 되물린다 — 옆에 붙이고 세로 가운데 정렬한다', () => {
    expect(legendTransform('left', 0, 0)).toBe('translate(0px, calc(-50% + 0px))');
    expect(legendTransform('right', 0, 0)).toBe('translate(0px, calc(-50% + 0px))');
  });

  it('끌어 옮긴 오프셋을 같은 transform 에 합친다 — 따로 쓰면 뒤가 앞을 지운다', () => {
    expect(legendTransform('bottom', 12, -4)).toBe('translate(calc(-50% + 12px), -4px)');
    expect(legendTransform('right', 12, -4)).toBe('translate(12px, calc(-50% + -4px))');
  });
});
