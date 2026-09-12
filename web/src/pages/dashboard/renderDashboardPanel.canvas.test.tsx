// renderDashboardPanel 캔버스 패널 디스패치 검증 (SPEC-CANVAS-001 REQ-01 / T3).
//
// switch(panel.type) 가 'canvas' 를 CanvasPanel 로 보내고 panelId/title/config 를 그대로
// 넘기는지 본다. config 를 파싱한 view 로 바꿔 넘기면 데이터 소스 판정이 사라지므로
// (CanvasPanel 규율 2) "원본 그대로" 를 함께 잠근다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

vi.mock('./panels/canvas/CanvasPanel', () => ({
  default: (p: { panelId: string; title?: string; config: Record<string, unknown> }) => (
    <div
      data-testid="stub-canvas"
      data-panelid={p.panelId}
      data-title={p.title}
      data-cfg={JSON.stringify(p.config)}
    />
  ),
}));

import { renderDashboardPanel } from './renderDashboardPanel';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

describe('renderDashboardPanel 캔버스 패널 디스패치', () => {
  const config = { data_source: 'store', elements: [{ id: 'e1', kind: 'rect' }] };
  const panel: PanelConfig = { id: 'p-canvas', type: 'canvas', title: '계통도', config };

  it('canvas → CanvasPanel 로 디스패치한다', () => {
    render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

    const el = screen.getByTestId('stub-canvas');
    expect(el).toHaveAttribute('data-panelid', 'p-canvas');
    expect(el).toHaveAttribute('data-title', '계통도');
  });

  it('config 를 원본 그대로 넘긴다', () => {
    render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

    expect(screen.getByTestId('stub-canvas')).toHaveAttribute('data-cfg', JSON.stringify(config));
  });
});
