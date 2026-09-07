// SPEC-DASHBOARD-002 (REQ-04, AC-04): PanelSettingsDialog 에이전트 상태 패널 설정.
//
// agent-status 패널 편집 시 (1) 전체 타입(필터 없음) 에이전트 재선택기가 노출되고,
// (2) 에이전트를 바꾸면 draft config.agentId 가 갱신되어 적용 시 저장되는지 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'agent-status', title: '에이전트 상태', config: { agentId: 'gw-1' } } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [{ id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] }],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

// 서로 다른 타입 3종 — 타입 필터가 없어야 전부 노출된다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'gw-1', name: '게이트웨이', type: 'modbus-gateway', status: 'running' },
        { id: 'xs-1', name: '설비', type: 'xsfm', status: 'running' },
        { id: 'mq-1', name: 'MQTT', type: 'mqtt-client', status: 'running' },
      ],
    },
  }),
  // 미리보기가 실제 AgentStatusPanel 을 그리면서 이 패널이 쓰는 훅까지 필요해졌다.
  useAgent: () => ({ data: undefined, isLoading: false }),
  useAgentStats: () => ({ data: undefined, isLoading: false }),
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.panel = { id: 'p1', type: 'agent-status', title: '에이전트 상태', config: { agentId: 'gw-1' } };
});

describe('PanelSettingsDialog 에이전트 상태 설정 (SPEC-DASHBOARD-002)', () => {
  it('AC-04-1: 전체 타입 에이전트 재선택기(필터 없음, 3종)를 노출한다', () => {
    render(
      // 미리보기가 실제 패널을 그리면서 조회 훅을 탄다 — 비활성 클라이언트로 감싼다.
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    const sel = screen.getByTestId('agent-status-settings-select') as HTMLSelectElement;
    expect(sel).toBeInTheDocument();
    // placeholder + 3종 = 4 (타입 필터 없음).
    expect(sel.options.length).toBe(4);
    expect(sel.value).toBe('gw-1');
  });

  it('AC-04-2: 에이전트 변경 시 draft config.agentId 가 갱신되어 적용 시 저장된다', () => {
    render(
      // 미리보기가 실제 패널을 그리면서 조회 훅을 탄다 — 비활성 클라이언트로 감싼다.
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    const sel = screen.getByTestId('agent-status-settings-select') as HTMLSelectElement;
    fireEvent.change(sel, { target: { value: 'xs-1' } });
    expect((screen.getByTestId('agent-status-settings-select') as HTMLSelectElement).value).toBe('xs-1');

    fireEvent.click(screen.getByRole('button', { name: 'dashboard.settings.apply' }));
    expect(storeMock.updatePanelConfig).toHaveBeenCalledTimes(1);
    const savedConfig = storeMock.updatePanelConfig.mock.calls[0]![1] as Record<string, unknown>;
    expect(savedConfig.agentId).toBe('xs-1');
  });
});
