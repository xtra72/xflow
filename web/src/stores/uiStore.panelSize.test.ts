// 패널 생성 시점 기본 그리드 크기 테스트.
//
// `panelDefaultSize` / `PANEL_DEFAULT_SIZES` 는 모듈 비공개이므로 스토어 액션(addPanel)을
// 통해 관측한다. 잠그는 것은 둘이다:
//
//   1. **전수성** — 모든 `PanelType` 이 표에 있어야 한다. 종전에는 표에 없는 타입이
//      조용히 일괄 폴백(5×4)으로 태어났고, 그 폴백이 맞는지 아무도 확인하지 않았다.
//      타입 수준에서 `Record<PanelType, …>` 가 강제하지만, 새 타입을 추가하며 폴백
//      값을 복사해 넣는 식으로 우회할 수 있으므로 값 수준에서도 확인한다.
//   2. **하한** — 헤더를 빼고 나면 내용이 들어갈 자리가 없는 크기로 태어나지 않는다.

import { beforeEach, describe, expect, it } from 'vitest';

import { useUIStore, type PanelType } from './uiStore';

/**
 * 표에 실려야 하는 모든 패널 타입.
 *
 * `Record<PanelType, true>` 의 키로 두어 **컴파일러가 유니온과의 일치를 강제**한다.
 * 배열 리터럴로 두면 새 타입이 생겨도 이 목록이 조용히 뒤처져, 정작 잡으려던
 * "표에 없는 타입" 을 검사하지 못한다.
 */
const PANEL_TYPE_SET: Record<PanelType, true> = {
  'flows': true,
  'agents': true,
  'resource': true,
  'devices': true,
  'device': true,
  'logs': true,
  'stat': true,
  'gauge': true,
  'graph-chart': true,
  'bar-chart': true,
  'pie-chart': true,
  'text': true,
  'table': true,
  'ac-control': true,
  'hvac-control': true,
  'custom-control': true,
  'outdoor-control': true,
  'properties-grid': true,
  'facility-line': true,
  'facility-station': true,
  'facility-device': true,
  'facility-group': true,
  'trigger-config': true,
  'facility-schedule': true,
  'modbus-real-devices': true,
  'modbus-virtual-devices': true,
  'modbus-shared-registers': true,
  'modbus-device-registers': true,
  'modbus-bus-stats': true,
  'modbus-summary-stats': true,
  'agent-status': true,
  'heatmap': true,
  'monitor-stats': true,
  'monitor-metrics': true,
  'monitor-network': true,
  'monitor-logs': true,
  'monitor-events': true,
  'sysmetrics-system': true,
  'sysmetrics-network': true,
  'sysmetrics-storage': true,
};

const ALL_PANEL_TYPES = Object.keys(PANEL_TYPE_SET) as PanelType[];

function activePage() {
  const s = useUIStore.getState();
  return s.dashboardPages.find((p) => p.id === s.activeDashboardId)!;
}

/** 타입 하나를 추가하고 그 레이아웃 항목을 돌려준다. */
function addAndRead(type: PanelType): { w: number; h: number; minW?: number; minH?: number } {
  useUIStore.getState().addPanel(type);
  const page = activePage();
  const panel = page.panels[page.panels.length - 1]!;
  return page.layout.find((l) => l.i === panel.id)!;
}

describe('패널 생성 기본 크기', () => {
  beforeEach(() => {
    useUIStore.setState({
      dashboardPages: [{ id: 'p', name: 'p', isDefault: true, panels: [], layout: [] }],
      activeDashboardId: 'p',
    });
  });

  it.each(ALL_PANEL_TYPES)('%s 는 표에 실려 있다 (일괄 폴백으로 태어나지 않는다)', (type) => {
    const layout = addAndRead(type);
    expect(layout.w).toBeGreaterThan(0);
    expect(layout.h).toBeGreaterThan(0);
  });

  // 셀은 정사각이고 헤더가 위쪽 한 조각을 먹는다. 높이 1칸짜리는 헤더를 빼면 값 한 줄도
  // 온전히 들어가지 않으므로, 어떤 타입도 1칸 높이로 태어나지 않는다. 폭 3칸은 어느
  // 패널이든 라벨과 값이 한 줄에 들어가는 하한이다.
  it.each(ALL_PANEL_TYPES)('%s 는 헤더를 빼고도 내용이 들어갈 크기다', (type) => {
    const layout = addAndRead(type);
    expect(layout.h).toBeGreaterThanOrEqual(2);
    expect(layout.w).toBeGreaterThanOrEqual(3);
  });

  // 폭이 열 수를 넘으면 react-grid-layout 이 강제로 줄여 배치가 흐트러진다.
  it.each(ALL_PANEL_TYPES)('%s 는 기본 10열 그리드를 넘지 않는다', (type) => {
    expect(addAndRead(type).w).toBeLessThanOrEqual(10);
  });

  it.each(ALL_PANEL_TYPES)('%s 의 minW/minH 는 기본 크기를 넘지 않는다', (type) => {
    const layout = addAndRead(type);
    expect(layout.minW ?? 0).toBeLessThanOrEqual(layout.w);
    expect(layout.minH ?? 0).toBeLessThanOrEqual(layout.h);
  });

  // 종전에 좁다고 지적된 타입들 — 값을 되돌리면 여기서 걸린다.
  it.each([
    { type: 'stat' as PanelType, w: 3, h: 2 },
    { type: 'gauge' as PanelType, w: 3, h: 3 },
    { type: 'graph-chart' as PanelType, w: 6, h: 4 },
    { type: 'bar-chart' as PanelType, w: 5, h: 4 },
    { type: 'pie-chart' as PanelType, w: 4, h: 4 },
    { type: 'table' as PanelType, w: 7, h: 5 },
    { type: 'sysmetrics-system' as PanelType, w: 6, h: 3 },
    { type: 'sysmetrics-network' as PanelType, w: 6, h: 5 },
    { type: 'sysmetrics-storage' as PanelType, w: 5, h: 4 },
  ])('$type 은 $w×$h 로 태어난다', ({ type, w, h }) => {
    expect(addAndRead(type)).toMatchObject({ w, h });
  });
});
