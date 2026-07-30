// 대시보드 패널 렌더러 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L09).
//
// `DashboardPage` 의 패널 switch 를 공유 함수로 추출한 것이다. 로컬 뷰와 원격
// 읽기 전용 뷰가 동일 코드로 패널을 렌더하여 패널 컴포넌트 중복을 피한다(A16).
//
// target 은 TargetProvider(상위 DashboardPage 가 감싼다)로 각 패널에 전파되므로
// renderPanel 시그니처는 변경하지 않는다 — 패널은 useTargetContext()로 target 을
// 읽어 데이터 소스를 전환한다(REQ-L09). target 미지정/local 시 패널 렌더는 로컬과
// 바이트 동일하다(회귀 0).

import {
  Hash,
  Gauge,
  LineChart,
  BarChart3,
  PieChart,
  Type,
  Table,
  Gamepad2,
} from 'lucide-react';

import type { PanelConfig, PanelType } from '@/stores/uiStore';
import type { FlowInfo } from '@/types/flow';

import AgentPanel from './panels/AgentPanel';
import DevicePanel from './panels/DevicePanel';
import FlowPanel from './panels/FlowPanel';
import LogPanel from './panels/LogPanel';
import SingleDevicePanel from './panels/SingleDevicePanel';
import AcControlPanel from './panels/AcControlPanel';
import GaugePanel from './panels/GaugePanel';
import PropertiesGridPanel from './panels/PropertiesGridPanel';
import HvacControlPanel from './panels/HvacControlPanel';
import OutdoorControlPanel from './panels/OutdoorControlPanel';
import FacilityDevicePanel from './panels/FacilityDevicePanel';
import FacilityLinePanel from './panels/FacilityLinePanel';
import FacilityGroupPanel from './panels/FacilityGroupPanel';
import StatPanel from './panels/charts/StatPanel';
import LineChartPanel from './panels/charts/LineChartPanel';
import BarChartPanel from './panels/charts/BarChartPanel';
import PieChartPanel from './panels/charts/PieChartPanel';
import TablePanel from './panels/charts/TablePanel';
import ResourceWidget from './widgets/ResourceWidget';

/** 패널 타입별 아이콘 매핑(플레이스홀더 패널용). */
const PANEL_TYPE_ICONS: Partial<Record<PanelType, React.ReactNode>> = {
  stat: <Hash className="h-6 w-6 text-(--color-text-muted)" />,
  gauge: <Gauge className="h-6 w-6 text-(--color-text-muted)" />,
  'line-chart': <LineChart className="h-6 w-6 text-(--color-text-muted)" />,
  'bar-chart': <BarChart3 className="h-6 w-6 text-(--color-text-muted)" />,
  'pie-chart': <PieChart className="h-6 w-6 text-(--color-text-muted)" />,
  text: <Type className="h-6 w-6 text-(--color-text-muted)" />,
  table: <Table className="h-6 w-6 text-(--color-text-muted)" />,
  'custom-control': <Gamepad2 className="h-6 w-6 text-(--color-text-muted)" />,
};

/** 패널 config/title 변경 콜백 쌍. */
export interface PanelChangeHandlers {
  onConfigChange: (config: Record<string, unknown>) => void;
  onTitleChange: (title: string) => void;
}

/**
 * 패널 타입에 따라 적절한 위젯 컴포넌트를 렌더링한다.
 *
 * @param panel - 패널 설정.
 * @param flowsList - flows 패널용 로컬 플로우 목록(원격은 패널이 자체 소스로 전환).
 * @param metricsData - resource 위젯용 로컬 메트릭(원격은 위젯이 자체 소스로 전환).
 * @param refreshMs - devices 패널 폴링 주기.
 * @param handlers - config/title 변경 콜백(읽기 전용 뷰는 no-op 전달).
 */
export function renderDashboardPanel(
  panel: PanelConfig,
  flowsList: FlowInfo[],
  metricsData: Record<string, unknown> | undefined,
  refreshMs: number,
  handlers: (panelId: string) => PanelChangeHandlers,
): React.ReactNode {
  const { onConfigChange: onCfg, onTitleChange: onTitle } = handlers(panel.id);

  switch (panel.type) {
    case 'flows':
      return <FlowPanel flows={flowsList} panelConfig={panel} />;
    case 'agents':
      return <AgentPanel panelConfig={panel} />;
    case 'resource':
      return <ResourceWidget metrics={metricsData} panelConfig={panel} />;
    case 'devices':
      return (
        <DevicePanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          refreshMs={refreshMs}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'device':
      return (
        <SingleDevicePanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'logs':
      return (
        <LogPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'ac-control':
      return (
        <AcControlPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'hvac-control':
      return (
        <HvacControlPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'outdoor-control':
      return (
        <OutdoorControlPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    // SPEC-FACILITY-DASHBOARD-001 M5: 설비 패널 3종 (device-panel 케이스와 동일 디스패치).
    case 'facility-device':
      return (
        <FacilityDevicePanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    // 레거시 facility-station 은 facility-group 의 별칭으로 디스패치한다(하위호환).
    // FacilityGroupPanel 이 config.station → "station:<code>" 로 groupId 를 파생하므로
    // 기존 저장된 역사 패널({type:'facility-station', config:{station}})이 그대로 렌더된다.
    case 'facility-station':
      return (
        <FacilityGroupPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'facility-line':
      return (
        <FacilityLinePanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    // SPEC-XSFM-GROUP-001 M7: 설비 그룹 패널.
    case 'facility-group':
      return (
        <FacilityGroupPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'gauge':
      return (
        <GaugePanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    case 'properties-grid':
      return (
        <PropertiesGridPanel
          panelId={panel.id}
          title={panel.title}
          config={panel.config}
          onConfigChange={onCfg}
          onTitleChange={onTitle}
        />
      );
    // SPEC-CHART-001 M4: 5종 차트 패널
    case 'stat':
      return <StatPanel panelId={panel.id} config={panel.config} />;
    case 'line-chart':
      return <LineChartPanel panelId={panel.id} title={panel.title} config={panel.config} />;
    case 'bar-chart':
      return <BarChartPanel panelId={panel.id} config={panel.config} />;
    case 'pie-chart':
      return <PieChartPanel panelId={panel.id} config={panel.config} />;
    case 'table':
      return <TablePanel panelId={panel.id} config={panel.config} />;
    // 잔여 플레이스홀더 패널 타입들 (text, custom-control)
    case 'text':
    case 'custom-control':
      return (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 rounded-lg bg-(--color-bg-surface) p-6 shadow">
          {PANEL_TYPE_ICONS[panel.type] ?? null}
          <span className="text-sm font-medium text-(--color-text-primary)">{panel.title}</span>
          <span className="text-xs text-(--color-text-muted)">{panel.type}</span>
        </div>
      );
    default:
      return (
        <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg bg-(--color-bg-surface) p-6 shadow">
          <span className="text-sm text-(--color-text-muted)">{panel.title}</span>
        </div>
      );
  }
}
