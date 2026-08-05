// SPEC-DASHBOARD-002 (REQ-01, AC-01-4): renderDashboardPanel agent-status 디스패치.
//
// switch(panel.type) 가 'agent-status' 를 AgentStatusPanel 로 디스패치하고
// panelId/title/config props 를 전달하는지 검증한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

vi.mock('./panels/AgentStatusPanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div
      data-testid="stub-agent-status"
      data-panelid={props.panelId}
      data-title={props.title}
      data-cfg={JSON.stringify(props.config)}
    />
  ),
}));

import { renderDashboardPanel } from './renderDashboardPanel';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

describe('renderDashboardPanel agent-status 디스패치 (SPEC-DASHBOARD-002)', () => {
  it('agent-status → AgentStatusPanel 로 디스패치하고 panelId/title/config 를 전달한다', () => {
    const panel: PanelConfig = {
      id: 'p-agent-status',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
    render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

    const el = screen.getByTestId('stub-agent-status');
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('data-panelid', 'p-agent-status');
    expect(el).toHaveAttribute('data-title', '에이전트 상태');
    expect(el).toHaveAttribute('data-cfg', JSON.stringify({ agentId: 'a-1' }));
  });
});
