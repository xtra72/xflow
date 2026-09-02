// 라인 차트 → 그래프 차트 이름 변경의 **읽기 경로** 검증.
//
// 이름만 바뀌었을 뿐 같은 패널이다. 저장된 대시보드가 옛 이름을 담고 있어도
// 그대로 열려야 하고, 그 경로가 끊기면 사용자는 패널이 통째로 사라진 것을 본다.

import { describe, expect, it } from 'vitest';

import { normalizePanelType, normalizePanels, type PanelConfig } from './uiStore';

const panel = (type: string, id = 'p1'): PanelConfig =>
  ({ id, type, title: 't', config: { x_label: '시간' } }) as unknown as PanelConfig;

describe('normalizePanelType', () => {
  it('옛 이름을 현재 이름으로 옮긴다', () => {
    expect(normalizePanelType('line-chart')).toBe('graph-chart');
  });

  it('현재 이름은 그대로 둔다', () => {
    expect(normalizePanelType('graph-chart')).toBe('graph-chart');
  });

  it('다른 패널 타입은 건드리지 않는다', () => {
    for (const t of ['bar-chart', 'pie-chart', 'stat', 'table', 'heatmap']) {
      expect(normalizePanelType(t)).toBe(t);
    }
  });

  it('모르는 값도 그대로 둔다 — 임의로 바꾸면 패널이 사라진다', () => {
    expect(normalizePanelType('future-chart')).toBe('future-chart');
  });
});

describe('normalizePanels', () => {
  it('옛 이름 패널만 옮기고 config 는 보존한다', () => {
    const out = normalizePanels([panel('line-chart'), panel('bar-chart', 'p2')]);
    expect(out[0]!.type).toBe('graph-chart');
    expect(out[0]!.config).toEqual({ x_label: '시간' });
    expect(out[1]!.type).toBe('bar-chart');
  });

  it('바꿀 것이 없으면 원본을 그대로 돌려준다 — 불필요한 재렌더를 만들지 않는다', () => {
    const input = [panel('graph-chart'), panel('stat', 'p2')];
    expect(normalizePanels(input)).toBe(input);
  });

  it('빈 목록도 안전하다', () => {
    expect(normalizePanels([])).toEqual([]);
  });
});
