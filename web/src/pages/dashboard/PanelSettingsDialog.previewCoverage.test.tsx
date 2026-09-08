// 모든 패널 타입이 미리보기를 그리는지 확인한다.
//
// 종전에는 타입마다 미리보기 분기를 손으로 달았고, 빠뜨린 타입은 설정을 열면 **빈 영역**만
// 보였다(agent-status · facility 계열 · trigger-config 등 12종). 화면을 열어 보기 전에는
// 알 수 없고 타입·린트·다른 테스트는 모두 통과하는 종류의 결함이라, 전 타입을 훑는다.
//
// 새 패널 타입이 생기면 아래 `PANEL_TYPE_SET` 이 **컴파일 오류**로 알려 준다 —
// 목록이 조용히 뒤처지면 정작 잡으려던 "빠뜨린 타입"을 검사하지 못한다.

import { describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import type { PanelType } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'flows', title: 'T', config: {} } as never,
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [
      {
        id: 'd',
        name: 'x',
        isDefault: true,
        panels: [storeMock.panel],
        layout: [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }],
      },
    ],
    updatePanelConfig: vi.fn(),
    updatePanelTitle: vi.fn(),
    setDashboardLayout: vi.fn(),
    dashboardGridCols: 12,
    dashboardGridWidth: 1376,
    dashboardShowGridLines: false,
    dashboardRefreshInterval: 5,
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (sel: (s: unknown) => unknown) => sel(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import PanelSettingsDialog from './PanelSettingsDialog';

/**
 * 전수 목록. `Record<PanelType, true>` 의 키로 두어 컴파일러가 유니온과의 일치를 강제한다
 * (stores/uiStore.panelSize.test.ts 와 같은 규율).
 */
const PANEL_TYPE_SET: Record<PanelType, true> = {
  'ac-control': true,
  'agent-status': true,
  'agents': true,
  'bar-chart': true,
  'custom-control': true,
  'device': true,
  'devices': true,
  'facility-device': true,
  'facility-group': true,
  'facility-line': true,
  'facility-schedule': true,
  'facility-station': true,
  'flows': true,
  'gauge': true,
  'graph-chart': true,
  'heatmap': true,
  'hvac-control': true,
  'logs': true,
  'modbus-bus-stats': true,
  'modbus-device-registers': true,
  'modbus-real-devices': true,
  'modbus-shared-registers': true,
  'modbus-summary-stats': true,
  'modbus-virtual-devices': true,
  'monitor-events': true,
  'monitor-logs': true,
  'monitor-metrics': true,
  'monitor-network': true,
  'monitor-stats': true,
  'outdoor-control': true,
  'pie-chart': true,
  'properties-grid': true,
  'resource': true,
  'stat': true,
  'sysmetrics-network': true,
  'sysmetrics-storage': true,
  'sysmetrics-system': true,
  'table': true,
  'text': true,
  'trigger-config': true,
};

const ALL_PANEL_TYPES = Object.keys(PANEL_TYPE_SET) as PanelType[];

describe('미리보기 — 모든 패널 타입이 그려진다', () => {
  it.each(ALL_PANEL_TYPES.map((t) => [t]))('%s', (type) => {
    storeMock.panel = { id: 'p1', type, title: 'T', config: {} } as never;

    render(
      <MemoryRouter>
        <QueryClientProvider client={inertQueryClient()}>
          <PanelSettingsDialog panelId="p1" onClose={() => {}} />
        </QueryClientProvider>
      </MemoryRouter>,
    );

    const stage = screen.getByTestId('preview-stage');
    // 비어 있으면 설정을 열어도 아무것도 보이지 않는다는 뜻이다.
    expect(stage.innerHTML.trim()).not.toBe('');
    cleanup();
  });
});
