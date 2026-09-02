// 배치 되돌리기 — 판정과 동작이 같은 키 집합을 봐야 한다.
//
// 둘이 갈리면 두 방향으로 틀린다: 판정이 빠뜨린 키는 버튼이 안 뜬 채로 남고, 동작이
// 빠뜨린 키는 눌러도 지워지지 않는다. 그래서 두 함수를 짝지어 검사한다.

import { describe, it, expect } from 'vitest';

import { chartLayoutResetPatch, isChartLayoutDirty } from './chartLayout';

describe('isChartLayoutDirty', () => {
  it('건드리지 않은 패널은 되돌릴 것이 없다', () => {
    expect(isChartLayoutDirty(undefined)).toBe(false);
    expect(isChartLayoutDirty({})).toBe(false);
  });

  it('그림 상자의 크기·자리를 본다', () => {
    expect(isChartLayoutDirty({ plot_size: 80 })).toBe(true);
    expect(isChartLayoutDirty({ plot_offset_x: 10 })).toBe(true);
    expect(isChartLayoutDirty({ plot_offset_y: -5 })).toBe(true);
  });

  it('범례의 자리도 본다', () => {
    expect(isChartLayoutDirty({ legend: { offset_x: 12 } })).toBe(true);
    expect(isChartLayoutDirty({ legend: { offset_y: -3 } })).toBe(true);
  });

  it('배치가 아닌 값에는 반응하지 않는다 — 글꼴을 고쳤다고 배치 초기화가 뜨면 안 된다', () => {
    expect(isChartLayoutDirty({ legend: { font_size: 16, show_last_value: true } })).toBe(false);
    expect(isChartLayoutDirty({ graph_style: 'area', y_label: '온도' })).toBe(false);
  });

  it('자리 0 은 기본값이다', () => {
    expect(isChartLayoutDirty({ plot_offset_x: 0, legend: { offset_y: 0 } })).toBe(false);
  });
});

describe('chartLayoutResetPatch', () => {
  it('그림 상자의 세 키를 지운다', () => {
    const patch = chartLayoutResetPatch({ plot_size: 60, plot_offset_x: 9, plot_offset_y: 8 });
    expect(patch.plot_size).toBeUndefined();
    expect(patch.plot_offset_x).toBeUndefined();
    expect(patch.plot_offset_y).toBeUndefined();
    expect('plot_size' in patch).toBe(true);
  });

  it('범례는 자리만 뺀다 — 통째로 지우면 글꼴까지 초기화된다', () => {
    const patch = chartLayoutResetPatch({
      legend: { offset_x: 12, offset_y: -4, font_size: 16, show_last_value: true },
    });
    expect(patch.legend).toEqual({ font_size: 16, show_last_value: true });
  });

  it('남는 것이 없으면 범례 오브젝트 자체를 지운다', () => {
    expect(chartLayoutResetPatch({ legend: { offset_x: 12 } }).legend).toBeUndefined();
  });

  it('범례가 없던 패널도 그대로 처리한다', () => {
    expect(chartLayoutResetPatch({ plot_size: 60 }).legend).toBeUndefined();
  });

  it('되돌린 결과는 더 이상 dirty 가 아니다 — 판정과 동작이 같은 키를 본다', () => {
    const before = {
      plot_size: 60,
      plot_offset_x: 9,
      plot_offset_y: 8,
      legend: { offset_x: 12, offset_y: -4, font_size: 16 },
    };
    const after = { ...before, ...chartLayoutResetPatch(before) };
    expect(isChartLayoutDirty(after)).toBe(false);
  });
});
