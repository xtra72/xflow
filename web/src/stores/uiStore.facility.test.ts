// uiStore 설비 패널 등록 테스트 (SPEC-FACILITY-DASHBOARD-001 M5).
//
// createDefaultPanel / panelDefaultSize 는 모듈 비공개이므로 스토어 액션(addPanel)을 통해
// 간접 검증한다. 3종 설비 타입이 (1) 예상 기본 config 를 만들고 (2) 예상 그리드 크기를
// 부여하는지 확인한다 — 이는 exhaustive switch 완결성의 런타임 증거이기도 하다.

import { describe, it, expect } from 'vitest';

import { useUIStore, type PanelType } from './uiStore';

/** 현재 활성 대시보드 페이지. */
function activePage() {
  const s = useUIStore.getState();
  return s.dashboardPages.find((p) => p.id === s.activeDashboardId)!;
}

describe('uiStore 설비 패널 등록 (SPEC-FACILITY-DASHBOARD-001 M5)', () => {
  const cases: {
    type: PanelType;
    config: Record<string, unknown>;
    size: { w: number; h: number };
  }[] = [
    { type: 'facility-device', config: { agentId: '', deviceId: '' }, size: { w: 3, h: 5 } },
    { type: 'facility-station', config: { agentId: '', station: '', showStats: true, deviceLabelMode: 'placeIndex', offlineAsOff: false }, size: { w: 5, h: 6 } },
    {
      type: 'facility-line',
      config: { agentId: '', line: '', nodeSize: '2', offlineAsOff: false, stationsPerRow: 0 },
      size: { w: 8, h: 6 },
    },
  ];

  it.each(cases)('addPanel($type) → 기본 config + 그리드 크기', ({ type, config, size }) => {
    useUIStore.getState().addPanel(type);

    const page = activePage();
    const panel = page.panels[page.panels.length - 1]!;
    expect(panel.type).toBe(type);
    expect(panel.config).toEqual(config);
    expect(panel.title).toBeTruthy();

    const layout = page.layout.find((l) => l.i === panel.id)!;
    expect(layout.w).toBe(size.w);
    expect(layout.h).toBe(size.h);
  });

  it('addPanelWithConfig 로 대상 값이 기본값 위에 병합된다', () => {
    useUIStore
      .getState()
      .addPanelWithConfig('facility-station', { agentId: 'air-1', station: 's1' }, 'A역');

    const page = activePage();
    const panel = page.panels[page.panels.length - 1]!;
    expect(panel.type).toBe('facility-station');
    // 기본값(showStats:true, deviceLabelMode:'placeIndex') 위에 전달값이 병합된다.
    expect(panel.config).toEqual({ agentId: 'air-1', station: 's1', showStats: true, deviceLabelMode: 'placeIndex', offlineAsOff: false });
    expect(panel.title).toBe('A역');
  });
});
