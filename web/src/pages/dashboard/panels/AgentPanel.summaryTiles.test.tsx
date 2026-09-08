// 에이전트 현황 패널의 요약 타일 — 표시 선택과 항목별 디자인.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

vi.mock('@/hooks', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'a-1', name: '게이트웨이', type: 'modbus-gateway', status: 'running', connected: true },
        { id: 'a-2', name: '센서', type: 'chirpstack', status: 'stopped', connected: false },
      ],
    },
    isLoading: false,
  }),
}));

vi.mock('@/hooks/useResourceTargets', () => ({
  useAgentsTarget: () => ({ data: undefined, isLoading: false }),
}));

import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import type { PanelConfig } from '@/stores/uiStore';
import AgentPanel from './AgentPanel';

function renderPanel(config: Record<string, unknown>) {
  const panelConfig = { id: 'p1', type: 'agents', title: '에이전트 현황', config } as PanelConfig;
  render(
    <QueryClientProvider client={inertQueryClient()}>
      <AgentPanel panelConfig={panelConfig} />
    </QueryClientProvider>,
  );
}

describe('요약 타일 표시 선택', () => {
  it('미설정이면 셋 다 기본 순서로 낸다', () => {
    renderPanel({});
    for (const item of ['total', 'active', 'inactive']) {
      expect(screen.getByTestId(`agent-summary-${item}`)).toBeInTheDocument();
    }
  });

  it('고른 것만 낸다 — 종전에는 셋을 한꺼번에 켜고 끄는 것뿐이었다', () => {
    renderPanel({ summaryItems: ['active'] });
    expect(screen.getByTestId('agent-summary-active')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-summary-total')).not.toBeInTheDocument();
    expect(screen.queryByTestId('agent-summary-inactive')).not.toBeInTheDocument();
  });

  it('고른 차례대로 그린다', () => {
    renderPanel({ summaryItems: ['inactive', 'total'] });
    const rendered = Array.from(
      screen.getByTestId('agent-summary-badges').children,
    ).map((el) => el.getAttribute('data-testid'));
    expect(rendered).toEqual(['agent-summary-inactive', 'agent-summary-total']);
  });

  it('전부 끄면 배지 줄 자체가 없다', () => {
    renderPanel({ summaryItems: [] });
    expect(screen.queryByTestId('agent-summary-badges')).not.toBeInTheDocument();
  });

  it('배지 전체 끄기가 여전히 이긴다', () => {
    renderPanel({ showSummaryBadges: false, summaryItems: ['active'] });
    expect(screen.queryByTestId('agent-summary-badges')).not.toBeInTheDocument();
  });
});

describe('항목별 타일 디자인', () => {
  it('고른 타일에만 걸린다', () => {
    renderPanel({ summaryStyles: { active: { size: 24 } } });
    expect(screen.getByTestId('agent-summary-active')).toHaveStyle({ fontSize: '24px' });
    expect(screen.getByTestId('agent-summary-total')).not.toHaveStyle({ fontSize: '24px' });
  });

  it('배경색을 정하지 않으면 타일별 기본 색이 그대로 산다', () => {
    renderPanel({});
    expect(screen.getByTestId('agent-summary-active').className).toContain('bg-green-100');
  });

  it('배경색을 정하면 기본 색 클래스를 걷어낸다 — 두 색이 겹치면 어느 쪽이 보이는지 알 수 없다', () => {
    renderPanel({ summaryStyles: { active: { bg: '#111111' } } });
    const tile = screen.getByTestId('agent-summary-active');
    expect(tile.className).not.toContain('bg-green-100');
    expect(tile).toHaveStyle({ backgroundColor: '#111111' });
  });
});
