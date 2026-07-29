// 설비 패널 영속 정합 테스트 (SPEC-FACILITY-DASHBOARD-001 M6, REQ-04-05 / UB-003).
//
// 3종 설비 패널이 기존 대시보드 스냅샷 스키마(SPEC-DASHBOARD-001)를 그대로 타는지
// 검증한다. 실제 영속 경로(store → collectActivePayload → JSON PUT)를 미러링해
// (1) 각 패널의 type+config 가 JSON 왕복에서 정확히 보존되고,
// (2) payload/스냅샷 최상위 스키마에 설비 전용 신규 필드가 새지 않음(config 내부 확장 한정)을
// 확인한다.

import { describe, it, expect, beforeEach } from 'vitest';

import {
  useUIStore,
  collectActivePayload,
  type PanelConfig,
  type PanelType,
} from '@/stores/uiStore';
import type { DashboardPayload, DashboardSnapshot } from '@/types/dashboard';

/** DashboardPayload 의 알려진 최상위 키 집합(SPEC-DASHBOARD-001 스키마). */
const KNOWN_PAYLOAD_KEYS = [
  'activeDashboardId',
  'dashboardGridCols',
  'dashboardPages',
  'dashboardRefreshInterval',
  'dashboardShowGridLines',
  'deviceGridLayout',
].sort();

/** DashboardSnapshot 의 알려진 최상위 키 집합(서버 스냅샷 래퍼). */
const KNOWN_SNAPSHOT_KEYS = ['owner', 'payload', 'scope', 'updatedAt', 'version'].sort();

/** 활성 페이지에서 특정 type 의 패널을 찾는다. */
function panelOfType(payload: DashboardPayload, type: PanelType): PanelConfig | undefined {
  const page = payload.dashboardPages.find((p) => p.id === payload.activeDashboardId);
  return page?.panels.find((pl) => pl.type === type);
}

// 3종 설비 패널의 현실적 config (agentId + 대상 + 표시옵션). facility-station/line 은
// addPanelWithConfig 병합 시 기본 표시옵션(showStats / nodeSize / offlineAsOff / stationsPerRow)이 함께 저장된다.
const facilityPanels: { type: PanelType; config: Record<string, unknown>; title: string }[] = [
  { type: 'facility-device', config: { agentId: 'air-1', deviceId: 'dev-42', refreshMs: 5000 }, title: '설비 기기 A' },
  {
    type: 'facility-station',
    config: { agentId: 'air-1', station: 'ST-101', refreshMs: 10000, showStats: true, deviceLabelMode: 'placeIndex', offlineAsOff: false },
    title: '강남역',
  },
  {
    type: 'facility-line',
    config: { agentId: 'air-1', line: '2호선', nodeSize: '2', offlineAsOff: false, stationsPerRow: 0 },
    title: '2호선',
  },
];

beforeEach(() => {
  // 실제 생성 경로(addPanelWithConfig)로 3종 패널을 활성 대시보드에 추가한다.
  const add = useUIStore.getState().addPanelWithConfig;
  for (const { type, config, title } of facilityPanels) {
    add(type, config, title);
  }
});

describe('설비 패널 영속 정합 (SPEC-FACILITY-DASHBOARD-001 REQ-04-05, UB-003)', () => {
  it('type+config 가 스냅샷 JSON 왕복에서 정확히 보존된다', () => {
    // 실제 서버 PUT 페이로드 수집 경로.
    const payload = collectActivePayload(useUIStore.getState());
    // 서버가 부여하는 메타는 스텁하되, 영속되는 스냅샷 형태를 그대로 미러링.
    const snapshot: DashboardSnapshot = {
      scope: 'global',
      owner: null,
      version: 3,
      updatedAt: 1_700_000_000_000,
      payload,
    };

    // 스냅샷이 SQLite 로 저장되었다 복원되는 것을 JSON 왕복으로 재현.
    const restored: DashboardSnapshot = JSON.parse(JSON.stringify(snapshot));

    for (const { type, config, title } of facilityPanels) {
      const panel = panelOfType(restored.payload, type);
      expect(panel, `복원된 페이로드에 ${type} 패널이 있어야 한다`).toBeDefined();
      // type 은 정확히 보존.
      expect(panel!.type).toBe(type);
      // config 는 addPanelWithConfig 가 만든 것 그대로(부분 병합 없음, 알 수 없는 키 없음).
      expect(panel!.config).toEqual(config);
      // 제목도 그대로.
      expect(panel!.title).toBe(title);
    }
  });

  it('스냅샷/페이로드 최상위 스키마에 설비 전용 신규 필드가 새지 않는다(UB-003)', () => {
    const payload = collectActivePayload(useUIStore.getState());
    const snapshot: DashboardSnapshot = {
      scope: 'global',
      owner: null,
      version: 3,
      updatedAt: 1_700_000_000_000,
      payload,
    };
    const restored: DashboardSnapshot = JSON.parse(JSON.stringify(snapshot));

    // 스냅샷 최상위 키 = 알려진 5개 그대로.
    expect(Object.keys(restored).sort()).toEqual(KNOWN_SNAPSHOT_KEYS);
    // payload 최상위 키 = 알려진 6개 그대로 (설비 패널을 추가해도 확장은 config 내부로 한정).
    expect(Object.keys(restored.payload).sort()).toEqual(KNOWN_PAYLOAD_KEYS);

    // 각 설비 패널 항목의 최상위 키도 기존 PanelConfig 스키마(id/type/title/config)로 한정.
    for (const { type } of facilityPanels) {
      const panel = panelOfType(restored.payload, type)!;
      expect(Object.keys(panel).sort()).toEqual(['config', 'id', 'title', 'type']);
    }
  });

  it('설비 확장은 패널 config 내부 키로만 이루어진다(config-only 확장)', () => {
    const payload = collectActivePayload(useUIStore.getState());
    // 설비 전용 식별 필드(agentId/station/line/deviceId)는 오직 panel.config 안에만 존재.
    const device = panelOfType(payload, 'facility-device')!;
    const station = panelOfType(payload, 'facility-station')!;
    const line = panelOfType(payload, 'facility-line')!;

    expect(device.config).toMatchObject({ agentId: 'air-1', deviceId: 'dev-42' });
    expect(station.config).toMatchObject({ agentId: 'air-1', station: 'ST-101' });
    expect(line.config).toMatchObject({ agentId: 'air-1', line: '2호선' });

    // 최상위 payload 에는 agentId/station/line/deviceId 가 존재하지 않는다.
    const payloadKeys = Object.keys(payload);
    for (const leaked of ['agentId', 'station', 'line', 'deviceId']) {
      expect(payloadKeys).not.toContain(leaked);
    }
  });
});
