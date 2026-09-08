// panelEditSelection 단위 테스트.
//
// @spec SPEC-CHART-004 AC-37 / AC-38

import { describe, it, expect } from 'vitest';

import { EMPTY_SELECTION, clampGroupDelta, nextSelection } from './panelEditSelection';
import { PANEL_OFFSET_LIMIT } from './panelGeometry';
import { STAT_OFFSET_LIMIT } from './statLayout';

describe('nextSelection (AC-37)', () => {
  it('그냥 누르면 그것 하나만 고른다', () => {
    expect([...nextSelection(EMPTY_SELECTION, 'value', false)]).toEqual(['value']);
    expect([...nextSelection(new Set(['delta', 'stats']), 'value', false)]).toEqual(['value']);
  });

  it('이미 고른 것을 다시 누르면 선택이 유지된다 — 끌기를 시작하려는 것이다', () => {
    const cur = new Set(['value', 'delta'] as const);
    // 새 Set 을 만들지 않고 그대로 돌려준다(무리 끌기가 이어져야 한다).
    expect(nextSelection(cur, 'value', false)).toBe(cur);
  });

  it('Shift/Ctrl 이면 더한다', () => {
    expect([...nextSelection(new Set(['value']), 'delta', true)].sort()).toEqual([
      'delta',
      'value',
    ]);
  });

  it('Shift/Ctrl 로 이미 고른 것을 누르면 뺀다', () => {
    expect([...nextSelection(new Set(['value', 'delta']), 'value', true)]).toEqual(['delta']);
  });

  it('원본을 바꾸지 않는다', () => {
    const cur = new Set(['value'] as const);
    nextSelection(cur, 'delta', true);
    expect([...cur]).toEqual(['value']);
  });
});

describe('clampGroupDelta — 무리가 함께 멈춘다 (AC-38)', () => {
  // 상한은 요소의 성질이 정하므로 요소마다 밝힌다 — 아래 무리는 모두 글자 덩어리(±50)다.
  const text = (offsetX: number, offsetY = 0) => ({
    offsetX,
    offsetY,
    limit: STAT_OFFSET_LIMIT,
  });

  it('아무도 상한에 닿지 않으면 그대로다', () => {
    expect(clampGroupDelta([text(0, 0), text(10, -10)], 5, 5)).toEqual({ dx: 5, dy: 5 });
  });

  it('한 요소가 먼저 닿으면 무리 전체가 거기서 멈춘다', () => {
    // value 는 40 → +10 까지, delta 는 0 → +50 까지 갈 수 있다. 무리는 +10 이 한계다.
    expect(clampGroupDelta([text(40), text(0)], 30, 0).dx).toBe(10);
  });

  it('반대 방향도 같은 규칙이다', () => {
    expect(clampGroupDelta([text(-45), text(0)], -30, 0).dx).toBe(-5);
  });

  it('두 축을 따로 죈다 — 한 축이 막혔다고 다른 축이 멈추지 않는다', () => {
    expect(clampGroupDelta([text(50, 0)], 10, 10)).toEqual({ dx: 0, dy: 10 });
  });

  it('요소가 없으면 그대로 둔다', () => {
    expect(clampGroupDelta([], 10, 10)).toEqual({ dx: 10, dy: 10 });
  });

  it('상한을 넘겨 저장된 값에서도 더 벌어지지 않는다', () => {
    // 손으로 편집한 config 가 상한 밖에 있어도, 그 방향으로 더 가지는 않는다.
    expect(clampGroupDelta([text(60)], 5, 0).dx).toBeLessThanOrEqual(0);
  });

  it('상한을 밝히지 않은 요소는 ±40 으로 본다 — 영역을 채우는 그림이 안전한 기본값이다', () => {
    // 상한을 잊은 새 대상이 붙었을 때 조용히 틀리는 쪽은 느슨한 기본값이다.
    expect(clampGroupDelta([{ offsetX: 0, offsetY: 0 }], 90, 0).dx).toBe(PANEL_OFFSET_LIMIT);
  });

  it('상한이 다른 요소가 섞이면 각자의 상한을 쓴다 — 먼저 닿는 쪽이 무리를 멈춘다', () => {
    // 그림(±40)과 글자 덩어리(±50)를 함께 고른 상황. 그림이 35 에 있으므로 +5 가 한계다.
    const members = [
      { offsetX: 35, offsetY: 0, limit: PANEL_OFFSET_LIMIT },
      { offsetX: 0, offsetY: 0, limit: STAT_OFFSET_LIMIT },
    ];
    expect(clampGroupDelta(members, 30, 0).dx).toBe(5);
  });

  it('상한이 큰 요소가 이미 그 자리에 있어도 무리를 뒤로 밀지 않는다', () => {
    // 무리에 한 값을 씌우면 45 인 글자 덩어리의 허용치가 -5 가 되어, 앞으로 끌었는데
    // 무리가 뒤로 간다. 각자의 상한을 쓰면 그런 일이 없다.
    const members = [
      { offsetX: 0, offsetY: 0, limit: PANEL_OFFSET_LIMIT },
      { offsetX: 45, offsetY: 0, limit: STAT_OFFSET_LIMIT },
    ];
    expect(clampGroupDelta(members, 3, 0).dx).toBe(3);
  });
});
