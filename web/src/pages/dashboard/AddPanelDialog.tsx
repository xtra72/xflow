// 패널 추가 다이얼로그.
// 카테고리 탭 기반으로 패널 유형을 선택하여 활성 대시보드에 추가한다.
// 'device', 'ac-control', 'hvac-control', 'custom-control' 유형 선택 시
// 디바이스 목록을 표시하여 개별 디바이스를 선택한다.

import { useEffect, useMemo, useState } from 'react';
import {
  GitBranch,
  Bot,
  Activity,
  HardDrive,
  ScrollText,
  Hash,
  TrendingUp,
  BarChart2,
  PieChart,
  FileText,
  Cpu,
  Thermometer,
  Wind,
  Settings,
  X,
  ArrowLeft,
  Wifi,
  WifiOff,
  Search,
  CircleDot,
  Table,
  LayoutGrid,
  Route,
  Fan,
  Layers,
  AlarmClock,
  CalendarClock,
  Grid3x3,
  PlugZap,
  Gauge,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { useUIStore, type PanelType } from '@/stores/uiStore';
import { useDevices } from '@/hooks/useDevice';
import { useAgents } from '@/hooks/useAgent';
import { useModbusListDevices, formatUnitLabel } from '@/pages/dashboard/panels/modbus/useModbusData';
import { useStations, useXsfmDevices } from '@/hooks/useStation';
import { useGroups } from '@/hooks/useGroups';
import { useNodeTypeInstances } from '@/hooks/useNodeTypeInstances';
import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { getDeviceDisplayName, getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import { buildDefaultStoreSource } from '@/pages/dashboard/panels/charts/chartChannelTypes';

// ---- 차트 패널 공통 ----

// 채널 이름 입력 스텝은 없어졌다. 차트 계열(통계/게이지/라인/바/파이/테이블)은 모두 기본
// config 가 `data_source: 'store'` 이므로(`uiStore.createDefaultPanel`) 생성 시 채널을 묻지
// 않고 곧바로 추가되며, 채널/Store/TSDB 전환은 패널 설정의 데이터 소스 섹션에서 한다.
// 렌더 경로는 세 소스를 모두 지원한다(`usePanelSeriesData`).

// ---- 패널 유형 정의 ----

/** 패널 유형 옵션 */
interface PanelOption {
  type: PanelType;
  icon: LucideIcon;
  /** 라벨 i18n 키 (렌더 시 t() 로 변환) */
  labelKey: string;
  /** 설명 i18n 키 (렌더 시 t() 로 변환) */
  descriptionKey: string;
  /** 디바이스 선택 스텝이 필요한 유형 */
  needsDevice?: boolean;
  /** 설비(에이전트 + 라인/역사/기기) 선택 스텝이 필요한 유형 (SPEC-FACILITY-DASHBOARD-001 M5) */
  needsFacility?: boolean;
  /** trigger 노드(플로우 + 노드) 선택 스텝이 필요한 유형 (SPEC-TRIGGER-PANEL-001 M2) */
  needsTriggerNode?: boolean;
  /** 설비 제어 예약(trigger 노드 + xsfm 에이전트) 선택 스텝이 필요한 유형 (SPEC-TRIGGER-SCHED-001 M2) */
  needsFacilitySchedule?: boolean;
  /** modbus-gateway 에이전트 선택 스텝이 필요한 유형 (SPEC-MODBUS-012 M1). 가상 레지스터 맵은 unit 2차 선택. */
  needsAgent?: boolean;
  /** 전체 타입(필터 없음) 에이전트 선택 스텝이 필요한 유형 (SPEC-DASHBOARD-002). */
  needsAgentStatus?: boolean;
  /**
   * 차트 채널 선택 스텝을 건너뛰고 곧바로 추가할 prefilled config.
   * 다채널 비교 등 단일 채널 입력만으로 부족한 프리셋용.
   */
  presetConfig?: Record<string, unknown>;
}

/** 카테고리 정의 (안정적인 식별자, 표시 라벨은 t() 로 변환) */
type Category = 'status' | 'chart' | 'data' | 'content' | 'etc';

const CATEGORIES: Category[] = ['status', 'chart', 'data', 'content', 'etc'];

/** 카테고리 표시 라벨 키 */
const CATEGORY_LABEL_KEY: Record<Category, string> = {
  status: 'dashboard.panelCategories.status',
  chart: 'dashboard.panelCategories.chart',
  data: 'dashboard.panelCategories.data',
  content: 'dashboard.panelCategories.content',
  etc: 'dashboard.panelCategories.etc',
};

/**
 * 카테고리 안의 하위 그룹.
 *
 * `labelKey` 가 없으면 제목 없이 옵션만 늘어놓는다(평면 카테고리). 콘텐트처럼 성격이
 * 갈리는 카테고리만 제목 있는 그룹으로 나눈다 — 모든 카테고리에 제목을 강제하면 항목이
 * 몇 개뿐인 카테고리에서 제목이 목록보다 커진다.
 */
interface PanelGroup {
  labelKey?: string;
  options: PanelOption[];
}

/** 카테고리별 패널 옵션 */
const PANEL_GROUPS_BY_CATEGORY: Record<Category, PanelGroup[]> = {
  // 상태: 무엇이 돌고 있고 어떤 상태인지 보여주는 패널.
  status: [
    {
      options: [
        { type: 'flows', icon: GitBranch, labelKey: 'dashboard.panelTypes.flows', descriptionKey: 'dashboard.addPanel.descriptions.flows' },
        { type: 'agents', icon: Bot, labelKey: 'dashboard.panelTypes.agents', descriptionKey: 'dashboard.addPanel.descriptions.agents' },
        // SPEC-DASHBOARD-002: 단일 에이전트(타입 무관) 상태·통계 패널. 전체 타입 에이전트 선택 스텝을 거친다.
        { type: 'agent-status', icon: Bot, labelKey: 'dashboard.panelTypes.agentStatus', descriptionKey: 'dashboard.addPanel.descriptions.agentStatus', needsAgentStatus: true },
        { type: 'devices', icon: HardDrive, labelKey: 'dashboard.panelTypes.devices', descriptionKey: 'dashboard.addPanel.descriptions.devices' },
        // 속성 그리드 → '디바이스 상태'. 한 디바이스의 상태 속성을 그리드로 보는 패널이므로
        // 상태 카테고리에 속한다(기타에 있으면 성격이 드러나지 않는다).
        { type: 'properties-grid', icon: LayoutGrid, labelKey: 'dashboard.addPanel.labels.propertiesGrid', descriptionKey: 'dashboard.addPanel.descriptions.propertiesGrid', needsDevice: true },
      ],
    },
  ],
  chart: [
    {
      options: [
        { type: 'stat', icon: Hash, labelKey: 'dashboard.panelTypes.stat', descriptionKey: 'dashboard.addPanel.descriptions.stat' },
        { type: 'gauge', icon: CircleDot, labelKey: 'dashboard.panelTypes.gauge', descriptionKey: 'dashboard.addPanel.descriptions.gauge' },
        // 라인 차트는 store 소스로 바로 추가된다 — 설정에서 에이전트 + 키/태그만 지정하면 된다.
        // 형상은 공용 팩토리 하나에서 나오므로 통계/게이지/바/파이의 기본 config 와 항상 같다.
        //
        // 채널 기반 항목 2종(단일 채널 · 다채널 비교)은 목록에서 제거됐다. 채널 경로 자체는
        // 남아 있으므로(설정의 데이터 소스 토글 · `channels` 배열) 기존 패널은 그대로 동작하고,
        // 새로 만든 라인 차트도 설정에서 채널로 되돌릴 수 있다.
        {
          type: 'graph-chart',
          icon: TrendingUp,
          labelKey: 'dashboard.panelTypes.lineChart',
          descriptionKey: 'dashboard.addPanel.descriptions.lineChart',
          presetConfig: {
            data_source: 'store',
            store_source: buildDefaultStoreSource(),
          },
        },
        { type: 'bar-chart', icon: BarChart2, labelKey: 'dashboard.panelTypes.barChart', descriptionKey: 'dashboard.addPanel.descriptions.barChart' },
        { type: 'pie-chart', icon: PieChart, labelKey: 'dashboard.panelTypes.pieChart', descriptionKey: 'dashboard.addPanel.descriptions.pieChart' },
        // SPEC-HEATMAP-PANEL-001 (MVP): store 태그 바인딩 온도 히트맵. 채널 스텝 없이 기본 config 로 추가된다.
        { type: 'heatmap', icon: Thermometer, labelKey: 'dashboard.addPanel.labels.heatmap', descriptionKey: 'dashboard.addPanel.descriptions.heatmap' },
      ],
    },
  ],
  // 데이터: 시리즈를 표 형태로 훑어보는 패널.
  data: [
    {
      options: [
        // 테이블은 통계/게이지/바/파이와 같이 store 기본 소스로 즉시 추가된다
        // (`uiStore.createDefaultPanel`). 채널/Store/TSDB 전환은 패널 설정의 데이터 소스
        // 섹션에서 한다 — 렌더 경로는 이미 3종을 모두 지원한다(`usePanelSeriesData`).
        { type: 'table', icon: Table, labelKey: 'dashboard.panelTypes.table', descriptionKey: 'dashboard.addPanel.descriptions.table' },
      ],
    },
  ],
  // 콘텐트: 도메인별 하위 그룹으로 나눈다(성격이 서로 멀어 한 줄로 늘어놓으면 찾기 어렵다).
  content: [
    {
      labelKey: 'dashboard.addPanel.groups.hvacr',
      options: [
        { type: 'ac-control', icon: Thermometer, labelKey: 'dashboard.panelTypes.acControl', descriptionKey: 'dashboard.addPanel.descriptions.acControl', needsDevice: true },
        { type: 'hvac-control', icon: Wind, labelKey: 'dashboard.panelTypes.hvacControl', descriptionKey: 'dashboard.addPanel.descriptions.hvacControl', needsDevice: true },
        { type: 'outdoor-control', icon: Cpu, labelKey: 'dashboard.addPanel.labels.outdoorControl', descriptionKey: 'dashboard.addPanel.descriptions.outdoorControl', needsDevice: true },
      ],
    },
    {
      // SPEC-FACILITY-DASHBOARD-001 M5 / SPEC-XSFM-GROUP-001 M7: 설비 패널.
      // 역사(facility-station)는 그룹(facility-group)의 한 종류로 흡수되어 더 이상 별도 생성
      // 타입이 아니다(그룹 선택기에서 역사 그룹을 고른다).
      labelKey: 'dashboard.addPanel.groups.facility',
      options: [
        { type: 'facility-line', icon: Route, labelKey: 'dashboard.panelTypes.facilityLine', descriptionKey: 'dashboard.addPanel.descriptions.facilityLine', needsFacility: true },
        { type: 'facility-group', icon: Layers, labelKey: 'dashboard.panelTypes.facilityGroup', descriptionKey: 'dashboard.addPanel.descriptions.facilityGroup', needsFacility: true },
        { type: 'facility-device', icon: Fan, labelKey: 'dashboard.panelTypes.facilityDevice', descriptionKey: 'dashboard.addPanel.descriptions.facilityDevice', needsFacility: true },
        // SPEC-TRIGGER-SCHED-001 M2: 설비 제어 예약 패널(규칙 테이블 + 모달). trigger-config 와 공존(RD-5).
        { type: 'facility-schedule', icon: CalendarClock, labelKey: 'dashboard.panelTypes.facilitySchedule', descriptionKey: 'dashboard.addPanel.descriptions.facilitySchedule', needsFacilitySchedule: true },
      ],
    },
    {
      // SPEC-MODBUS-012 M1: MODBUS Gateway 패널(모두 modbus-gateway 에이전트에 바인딩).
      labelKey: 'dashboard.addPanel.groups.modbus',
      options: [
        // 실제 디바이스 → '디바이스 상태'. 상류(upstream) 연결 디바이스의 상태를 보는 패널이라
        // 다른 MODBUS Gateway 패널들과 같은 그룹에 둔다.
        { type: 'modbus-real-devices', icon: PlugZap, labelKey: 'dashboard.panelTypes.modbusRealDevices', descriptionKey: 'dashboard.addPanel.descriptions.modbusRealDevices', needsAgent: true },
        { type: 'modbus-virtual-devices', icon: Cpu, labelKey: 'dashboard.panelTypes.modbusVirtualDevices', descriptionKey: 'dashboard.addPanel.descriptions.modbusVirtualDevices', needsAgent: true },
        { type: 'modbus-shared-registers', icon: Grid3x3, labelKey: 'dashboard.panelTypes.modbusSharedRegisters', descriptionKey: 'dashboard.addPanel.descriptions.modbusSharedRegisters', needsAgent: true },
        { type: 'modbus-device-registers', icon: LayoutGrid, labelKey: 'dashboard.panelTypes.modbusDeviceRegisters', descriptionKey: 'dashboard.addPanel.descriptions.modbusDeviceRegisters', needsAgent: true },
        { type: 'modbus-bus-stats', icon: Activity, labelKey: 'dashboard.panelTypes.modbusBusStats', descriptionKey: 'dashboard.addPanel.descriptions.modbusBusStats', needsAgent: true },
        { type: 'modbus-summary-stats', icon: Gauge, labelKey: 'dashboard.panelTypes.modbusSummaryStats', descriptionKey: 'dashboard.addPanel.descriptions.modbusSummaryStats', needsAgent: true },
      ],
    },
  ],
  // 기타: 위 분류에 배정되지 않은 나머지. 목록에서 빠지면 새로 만들 수 없으므로 여기에 모은다.
  etc: [
    {
      options: [
        { type: 'text', icon: FileText, labelKey: 'dashboard.panelTypes.text', descriptionKey: 'dashboard.addPanel.descriptions.text' },
        { type: 'device', icon: HardDrive, labelKey: 'dashboard.panelTypes.device', descriptionKey: 'dashboard.addPanel.descriptions.device', needsDevice: true },
        // SPEC-TRIGGER-PANEL-001 M2: trigger 노드 스케줄/페이로드 설정 패널.
        { type: 'trigger-config', icon: AlarmClock, labelKey: 'dashboard.panelTypes.triggerConfig', descriptionKey: 'dashboard.addPanel.descriptions.triggerConfig', needsTriggerNode: true },
        { type: 'custom-control', icon: Settings, labelKey: 'dashboard.panelTypes.customControl', descriptionKey: 'dashboard.addPanel.descriptions.customControl', needsDevice: true },
        // 시스템 카테고리에서 옮겨 왔다. 나머지 모니터링 패널과 달리 xflowd 런타임 지표가
        // 아니라 로그를 읽는 뷰어라, 호스트 지표만 남은 시스템 카테고리에 두면 성격이 어긋난다.
        { type: 'monitor-logs', icon: ScrollText, labelKey: 'dashboard.panelTypes.monitorLogs', descriptionKey: 'dashboard.addPanel.descriptions.monitorLogs' },
      ],
    },
  ],
};

/** 전체 패널 옵션 (검색용) — 카테고리·하위 그룹을 모두 평탄화한다. */
const ALL_PANEL_OPTIONS: PanelOption[] = Object.values(PANEL_GROUPS_BY_CATEGORY)
  .flat()
  .flatMap((g) => g.options);

// ---- 컴포넌트 ----

interface AddPanelDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 패널 추가 모달 */
export default function AddPanelDialog({ open, onClose }: AddPanelDialogProps) {
  const addPanel = useUIStore((s) => s.addPanel);
  const addPanelWithConfig = useUIStore((s) => s.addPanelWithConfig);
  const [step, setStep] = useState<
    | 'type'
    | 'device'
    | 'facility'
    | 'trigger-node'
    | 'facility-schedule'
    | 'modbus-agent'
    | 'agent-status-agent'
  >('type');
  const [selectedType, setSelectedType] = useState<PanelType | null>(null);

  // 다이얼로그 닫힐 때 상태 초기화
  useEffect(() => {
    if (!open) {
      setStep('type');
      setSelectedType(null);
    }
  }, [open]);

  // ESC 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (
          step === 'device' ||
          step === 'facility' ||
          step === 'trigger-node' ||
          step === 'facility-schedule' ||
          step === 'modbus-agent' ||
          step === 'agent-status-agent'
        ) {
          setStep('type');
          setSelectedType(null);
        } else {
          onClose();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, step, onClose]);

  // 패널 유형 선택 처리
  const handleSelect = (option: PanelOption) => {
    if (option.needsDevice) {
      setSelectedType(option.type);
      setStep('device');
      return;
    }
    if (option.needsFacility) {
      setSelectedType(option.type);
      setStep('facility');
      return;
    }
    if (option.needsTriggerNode) {
      setSelectedType(option.type);
      setStep('trigger-node');
      return;
    }
    if (option.needsFacilitySchedule) {
      setSelectedType(option.type);
      setStep('facility-schedule');
      return;
    }
    // SPEC-MODBUS-012 M1: modbus-gateway 에이전트 선택 스텝(가상 레지스터 맵은 unit 2차 선택).
    if (option.needsAgent) {
      setSelectedType(option.type);
      setStep('modbus-agent');
      return;
    }
    // SPEC-DASHBOARD-002: 전체 타입(필터 없음) 에이전트 선택 스텝.
    if (option.needsAgentStatus) {
      setSelectedType(option.type);
      setStep('agent-status-agent');
      return;
    }
    if (option.presetConfig) {
      // 프리셋이 소스 설정을 미리 주므로 그대로 추가한다.
      addPanelWithConfig(option.type, option.presetConfig);
      onClose();
      return;
    }
    addPanel(option.type);
    onClose();
  };

  // 디바이스 선택 처리
  const handleDeviceSelect = (deviceId: string, deviceName: string) => {
    if (!selectedType) return;
    addPanelWithConfig(selectedType, { deviceId }, deviceName);
    onClose();
  };

  // 설비 대상 선택 완료 처리 (에이전트 + 라인/역사/기기).
  const handleFacilityConfirm = (config: Record<string, unknown>, title?: string) => {
    if (!selectedType) return;
    addPanelWithConfig(selectedType, config, title);
    onClose();
  };

  // trigger 노드 선택 완료 처리 (SPEC-TRIGGER-PANEL-001 M2).
  // 선택한 flow/node 를 패널 config 에 저장하고, 기본 타이틀은 노드/플로우 이름으로 한다.
  const handleTriggerNodeSelect = (
    flowId: string,
    nodeId: string,
    title: string,
  ) => {
    addPanelWithConfig('trigger-config', { flowId, nodeId }, title);
    onClose();
  };

  // 설비 제어 예약 대상 선택 완료 처리 (SPEC-TRIGGER-SCHED-001 M2).
  // trigger 노드({flowId,nodeId}) + xsfm 에이전트(agentId, 선택)를 config 로 저장한다.
  const handleFacilityScheduleConfirm = (
    flowId: string,
    nodeId: string,
    agentId: string,
    title: string,
  ) => {
    addPanelWithConfig('facility-schedule', { flowId, nodeId, agentId }, title);
    onClose();
  };

  // MODBUS Gateway 패널 대상 선택 완료 처리 (SPEC-MODBUS-012 M1).
  // 선택한 modbus-gateway 에이전트(+ 가상 레지스터 맵은 unitId)를 config 로 저장한다.
  const handleModbusConfirm = (config: Record<string, unknown>, title?: string) => {
    if (!selectedType) return;
    addPanelWithConfig(selectedType, config, title);
    onClose();
  };

  // 에이전트 상태 패널 대상 선택 완료 처리 (SPEC-DASHBOARD-002).
  // 전체 타입 중 선택한 에이전트의 { agentId } 를 config 로 저장하고, 기본 타이틀은 에이전트 이름으로 한다.
  const handleAgentStatusConfirm = (agentId: string, title?: string) => {
    addPanelWithConfig('agent-status', { agentId }, title);
    onClose();
  };

  // sysmetrics 패널 대상 선택 완료 처리 (SPEC-SYSMETRICS-PANEL-001 M5).
  // 정본은 agent_id 이고 agent_name 은 표시용 스냅샷이다 — 이름만 저장하면
  // 에이전트를 리네임했을 때 패널이 조용히 끊긴다.
  if (!open) return null;

  // 모달 → 페이지: 배경 오버레이 제거, AppLayout 콘텐츠 영역을 채우는 전체화면
  // 페이지 컨테이너로 렌더한다. 위저드 카드는 가독성을 위해 중앙 정렬한다.
  return (
    <div className="flex h-full w-full flex-col overflow-y-auto">
      <div
        className="mx-auto w-full max-w-2xl overflow-hidden rounded-lg border border-(--color-border-default) bg-(--color-bg-surface)"
        aria-labelledby="add-panel-dialog-title"
      >
        {step === 'type' && <TypeStep onSelect={handleSelect} onClose={onClose} />}
        {step === 'device' && (
          <DeviceStep
            onSelect={handleDeviceSelect}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'facility' && selectedType && (
          <FacilityStep
            panelType={selectedType}
            onConfirm={handleFacilityConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'trigger-node' && (
          <TriggerNodeStep
            onSelect={handleTriggerNodeSelect}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'facility-schedule' && (
          <FacilityScheduleStep
            onConfirm={handleFacilityScheduleConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'modbus-agent' && selectedType && (
          <ModbusAgentStep
            panelType={selectedType}
            onConfirm={handleModbusConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'agent-status-agent' && (
          <AgentStatusAgentStep
            onConfirm={handleAgentStatusConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
      </div>
    </div>
  );
}

// ---- Step 1: 카테고리 기반 패널 유형 선택 ----

function TypeStep({
  onSelect,
  onClose,
}: {
  onSelect: (option: PanelOption) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [activeCategory, setActiveCategory] = useState<Category>('status');
  const [searchQuery, setSearchQuery] = useState('');

  // 화면에 그릴 그룹 목록. 검색 중에는 카테고리·하위 그룹을 무시하고 전체에서 필터링한
  // 결과를 제목 없는 그룹 하나로 돌려준다 — 검색은 "어느 카테고리에 있는지 모를 때" 쓰는
  // 것이므로 그룹 제목이 오히려 결과를 흩어 놓는다.
  const groups = useMemo<PanelGroup[]>(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return PANEL_GROUPS_BY_CATEGORY[activeCategory];
    }
    const hits = ALL_PANEL_OPTIONS.filter(
      (opt) =>
        t(opt.labelKey).toLowerCase().includes(query) ||
        t(opt.descriptionKey).toLowerCase().includes(query),
    );
    return hits.length > 0 ? [{ options: hits }] : [];
  }, [activeCategory, searchQuery, t]);

  const isSearching = searchQuery.trim().length > 0;
  const isEmpty = groups.every((g) => g.options.length === 0);

  return (
    <>
      {/* 헤더 */}
      <div className="border-b border-(--color-border-default) px-5 pt-4 pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {/* 페이지 뒤로가기: 대시보드로 복귀 */}
            <button
              type="button"
              onClick={onClose}
              className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
              aria-label={t('dashboard.addPanel.backAria')}
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <h2
              id="add-panel-dialog-title"
              className="text-lg font-semibold text-(--color-text-primary)"
            >
              {t('dashboard.addPanel.title')}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.closeAria')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 검색 입력 */}
        <div className="relative mt-3">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-(--color-text-muted)" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('dashboard.addPanel.searchPlaceholder')}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) py-2 pr-3 pl-9 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
          />
        </div>

        {/* 카테고리 탭 (검색 중이 아닐 때만 표시) */}
        {!isSearching && (
          <div className="mt-3 flex gap-1">
            {CATEGORIES.map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => setActiveCategory(cat)}
                className={cn(
                  'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  activeCategory === cat
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
                )}
              >
                {t(CATEGORY_LABEL_KEY[cat])}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* 패널 유형 카드 그리드 */}
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {isEmpty ? (
          <p
            data-testid="add-panel-empty"
            className="py-8 text-center text-sm text-(--color-text-muted)"
          >
            {/* 검색 결과 없음과 "이 카테고리에 항목이 없음" 은 원인이 달라 문구를 나눈다. */}
            {t(isSearching ? 'dashboard.addPanel.noResults' : 'dashboard.addPanel.emptyCategory')}
          </p>
        ) : (
          <div className="space-y-4">
            {groups.map((group, gi) => (
              <div key={group.labelKey ?? `g${gi}`} data-testid="add-panel-group">
                {group.labelKey && (
                  <p
                    data-testid="add-panel-group-label"
                    className="mb-2 text-xs font-semibold text-(--color-text-muted)"
                  >
                    {t(group.labelKey)}
                  </p>
                )}
                <div className="grid grid-cols-2 gap-3">
                  {group.options.map((option) => {
                    const Icon = option.icon;
                    return (
                      <button
                        key={`${option.type}:${option.labelKey}`}
                        type="button"
                        onClick={() => onSelect(option)}
                        className="flex items-start gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
                      >
                        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-(--color-bg-elevated)">
                          <Icon className="h-4 w-4 text-(--color-text-secondary)" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold text-(--color-text-primary)">
                            {t(option.labelKey)}
                          </p>
                          <p className="mt-0.5 text-xs leading-relaxed text-(--color-text-muted)">
                            {t(option.descriptionKey)}
                          </p>
                        </div>
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step 2: 디바이스 선택 ----

function DeviceStep({
  onSelect,
  onBack,
  onClose,
}: {
  onSelect: (deviceId: string, deviceName: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { data: devicesData, isLoading } = useDevices();
  const devices = devicesData?.data ?? [];

  return (
    <>
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('dashboard.addPanel.selectDevice')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
          </div>
        ) : devices.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            {t('dashboard.addPanel.noDevices')}
          </p>
        ) : (
          <div className="space-y-2">
            {devices.map((device) => (
              <button
                key={device.uid ?? device.id}
                type="button"
                onClick={() => onSelect(device.id, getDeviceDisplayName(device))}
                className="flex w-full items-center gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
              >
                <span
                  className={cn(
                    'h-2.5 w-2.5 shrink-0 rounded-full',
                    device.online ? 'bg-green-500' : 'bg-gray-400',
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-(--color-text-primary)">
                    {getDeviceDisplayName(device)}
                  </p>
                  <p className="text-xs text-(--color-text-muted)">
                    {getDeviceTypeLabel(device.type)} &middot; {device.protocol.toUpperCase()}
                  </p>
                </div>
                {device.online ? (
                  <Wifi className="h-4 w-4 shrink-0 text-green-500" />
                ) : (
                  <WifiOff className="h-4 w-4 shrink-0 text-gray-400" />
                )}
              </button>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step: 설비 대상 선택 (SPEC-FACILITY-DASHBOARD-001 M5) ----

/** 설비 패널 타입 → config 대상 키. 역사(facility-station)는 그룹으로 흡수되어 생성 경로에서 제외된다. */
const FACILITY_TARGET_KEY: Record<string, 'deviceId' | 'line' | 'groupId'> = {
  'facility-device': 'deviceId',
  'facility-line': 'line',
  'facility-group': 'groupId',
};

/** 대상 셀렉트 라벨/placeholder i18n 키(패널 타입별). */
const FACILITY_TARGET_I18N: Record<string, { label: string; placeholder: string }> = {
  'facility-device': { label: 'dashboard.settings.device', placeholder: 'dashboard.settings.selectDevice' },
  'facility-line': { label: 'dashboard.settings.line', placeholder: 'dashboard.settings.selectLine' },
  'facility-group': { label: 'dashboard.settings.group', placeholder: 'dashboard.settings.selectGroup' },
};

/** 대상 옵션 한 건(값 + 표시 라벨 + 선택 시 기본 타이틀). title 미지정 시 label 을 타이틀로 쓴다. */
interface FacilityTargetOption {
  value: string;
  label: string;
  title?: string;
}

/**
 * 설비 대상 선택 스텝. 에이전트(xsfm)를 먼저 고르고, 패널 타입에 따라 라인/그룹/기기 대상을 고른다.
 * 대상 조회는 기존 useStations / useXsfmDevices / useGroups 를 재사용한다(UB-001, 재구현 없음).
 * 완료 시 addPanelWithConfig 로 { agentId, <대상키> } + 대상 이름 기본 타이틀을 전달한다.
 *
 * 설비 그룹 패널(facility-group)은 역사·라인·커스텀 그룹을 한 셀렉터에 나열하며(type 라벨 + 멤버 수),
 * 선택 시 config.groupId 로 저장한다. 역사(facility-station)는 그룹으로 흡수되어 생성 경로에서 제외된다.
 */
function FacilityStep({
  panelType,
  onConfirm,
  onBack,
  onClose,
}: {
  panelType: PanelType;
  onConfirm: (config: Record<string, unknown>, title?: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [agentId, setAgentId] = useState('');
  const [target, setTarget] = useState('');

  const { data: agentsResult } = useAgents();
  const airAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm'),
    [agentsResult],
  );

  const { data: stations, isLoading: stationsLoading } = useStations(agentId);
  const { data: devices, isLoading: devicesLoading } = useXsfmDevices(agentId);
  const { data: groups, isLoading: groupsLoading } = useGroups(agentId);

  const targetKey = FACILITY_TARGET_KEY[panelType] ?? 'deviceId';
  const targetI18n = FACILITY_TARGET_I18N[panelType] ?? FACILITY_TARGET_I18N['facility-device']!;

  // 패널 타입별 대상 옵션 계산. 그룹은 label 에 type + 멤버 수를 함께 보여주되 title 은 그룹명만 쓴다.
  const targetOptions: FacilityTargetOption[] = useMemo(() => {
    if (panelType === 'facility-device') {
      return (devices ?? []).map((d) => ({ value: d.device_id, label: d.name || d.device_id }));
    }
    if (panelType === 'facility-group') {
      return (groups ?? []).map((g) => ({
        value: g.id,
        label: `${g.name} · ${t(`dashboard.facility.group.type.${g.type}`)} · ${g.member_count}`,
        title: g.name,
      }));
    }
    // facility-line: 로스터의 distinct line 값.
    const lines = Array.from(new Set((stations ?? []).map((s) => s.line).filter(Boolean)));
    return lines.map((l) => ({ value: l, label: l }));
  }, [panelType, devices, stations, groups, t]);

  const targetLoading =
    panelType === 'facility-device'
      ? devicesLoading
      : panelType === 'facility-group'
        ? groupsLoading
        : stationsLoading;
  const canSave = agentId.length > 0 && target.length > 0;

  const handleConfirm = () => {
    if (!canSave) return;
    const selected = targetOptions.find((o) => o.value === target);
    // 기본 타이틀: 그룹은 그룹명(title), 그 외는 표시 라벨.
    onConfirm({ agentId, [targetKey]: target }, selected?.title ?? selected?.label);
  };

  return (
    <>
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t(targetI18n.label)}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* 본문 */}
      <div className="space-y-4 px-5 py-4">
        {/* 에이전트 선택 */}
        <div>
          <label
            htmlFor="facility-agent-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.settings.agent')} <span className="text-red-500">*</span>
          </label>
          <select
            id="facility-agent-select"
            data-testid="facility-agent-select"
            value={agentId}
            onChange={(e) => {
              setAgentId(e.target.value);
              setTarget('');
            }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t('dashboard.settings.selectAgent')}</option>
            {airAgents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>

        {/* 대상(라인/그룹/기기) 선택. */}
        <div>
          <label
            htmlFor="facility-target-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t(targetI18n.label)} <span className="text-red-500">*</span>
          </label>
          <select
            id="facility-target-select"
            data-testid="facility-target-select"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            disabled={!agentId || targetLoading}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
          >
            <option value="">{t(targetI18n.placeholder)}</option>
            {targetOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 푸터 */}
      <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
        <button
          type="button"
          onClick={onBack}
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
        >
          {t('dashboard.addPanel.previous')}
        </button>
        <button
          type="button"
          data-testid="facility-save"
          onClick={handleConfirm}
          disabled={!canSave}
          className={cn(
            'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
            canSave
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'cursor-not-allowed bg-gray-300 text-gray-500 dark:bg-gray-700 dark:text-gray-500',
          )}
        >
          {t('dashboard.addPanel.save')}
        </button>
      </div>
    </>
  );
}

// ---- Step: trigger 노드 선택 (SPEC-TRIGGER-PANEL-001 M2) ----

/**
 * trigger 노드 선택 스텝. running 플로우의 trigger 노드 인스턴스를
 * useNodeTypeInstances('trigger') 로 열거하여 피커로 제시한다(REQ-02-02).
 * 선택 시 { flowId, nodeId } 를 패널 config 로 저장하고(REQ-02-03), 기본 타이틀은
 * 노드/플로우 이름으로 한다. 설비 스텝(FacilityStep)을 flow 노드 타겟으로 미러링한 것이다.
 */
function TriggerNodeStep({
  onSelect,
  onBack,
  onClose,
}: {
  onSelect: (flowId: string, nodeId: string, title: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { instances, isLoading } = useNodeTypeInstances('trigger');

  return (
    <>
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('dashboard.addPanel.selectTriggerNode')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
          </div>
        ) : instances.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            {t('dashboard.addPanel.noTriggerNodes')}
          </p>
        ) : (
          <div className="space-y-2">
            {instances.map((inst) => (
              <button
                key={`${inst.flowId}:${inst.nodeId}`}
                type="button"
                data-testid={`trigger-node-option-${inst.flowId}-${inst.nodeId}`}
                onClick={() =>
                  onSelect(inst.flowId, inst.nodeId, inst.nodeName || inst.flowName)
                }
                className="flex w-full items-center gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
              >
                <AlarmClock className="h-4 w-4 shrink-0 text-blue-500" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-(--color-text-primary)">
                    {inst.nodeName}
                  </p>
                  <p className="text-xs text-(--color-text-muted)">{inst.flowName}</p>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step: 설비 제어 예약 대상 선택 (SPEC-TRIGGER-SCHED-001 M2) ----

/**
 * 설비 제어 예약 패널 대상 선택 스텝. trigger 노드(필수)를
 * useNodeTypeInstances('trigger') 로 열거하고, TARGET 열거용 xsfm 에이전트(선택)를
 * useAgents 로 고른다. 에이전트는 미지정 가능하며(자유 입력 폴백), 노드 선택 시
 * { flowId, nodeId, agentId } 를 패널 config 로 저장한다(REQ-SCHED-02/04).
 */
function FacilityScheduleStep({
  onConfirm,
  onBack,
  onClose,
}: {
  onConfirm: (flowId: string, nodeId: string, agentId: string, title: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { instances, isLoading } = useNodeTypeInstances('trigger');
  const { data: agentsResult } = useAgents();
  const xsfmAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm'),
    [agentsResult],
  );
  const [agentId, setAgentId] = useState('');

  return (
    <>
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('dashboard.addPanel.selectFacilitySchedule')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {/* 설비 에이전트(선택) */}
        <div className="mb-4">
          <label
            htmlFor="facility-schedule-agent-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.settings.agent')}
          </label>
          <select
            id="facility-schedule-agent-select"
            data-testid="facility-schedule-agent-select"
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t('dashboard.addPanel.facilityScheduleAgentOptional')}</option>
            {xsfmAgents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>

        {/* trigger 노드(필수) */}
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
          </div>
        ) : instances.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            {t('dashboard.addPanel.noTriggerNodes')}
          </p>
        ) : (
          <div className="space-y-2">
            {instances.map((inst) => (
              <button
                key={`${inst.flowId}:${inst.nodeId}`}
                type="button"
                data-testid={`facility-schedule-node-${inst.flowId}-${inst.nodeId}`}
                onClick={() =>
                  onConfirm(inst.flowId, inst.nodeId, agentId, inst.nodeName || inst.flowName)
                }
                className="flex w-full items-center gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
              >
                <CalendarClock className="h-4 w-4 shrink-0 text-blue-500" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-(--color-text-primary)">{inst.nodeName}</p>
                  <p className="text-xs text-(--color-text-muted)">{inst.flowName}</p>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step: MODBUS Gateway 에이전트 선택 (SPEC-MODBUS-012 M1) ----

/**
 * MODBUS Gateway 패널 대상 선택 스텝. modbus-gateway 에이전트를 필터해 제시하고
 * (FacilityStep 의 xsfm 필터를 gateway 로 미러링), 완료 시 { agentId } 를 config 로 저장한다.
 * 가상 디바이스 레지스터 맵(modbus-device-registers)만 2차로 대상 unit 을 선택해 unitId 를 추가한다
 * (선택된 에이전트에 list_devices 를 조회).
 */
function ModbusAgentStep({
  panelType,
  onConfirm,
  onBack,
  onClose,
}: {
  panelType: PanelType;
  onConfirm: (config: Record<string, unknown>, title?: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [agentId, setAgentId] = useState('');
  const [unitId, setUnitId] = useState('');

  const { data: agentsResult } = useAgents();
  const gatewayAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'modbus-gateway'),
    [agentsResult],
  );

  // 가상 디바이스 레지스터 맵만 unit 2차 선택. 그 외 5종은 에이전트만 선택한다.
  const needsUnit = panelType === 'modbus-device-registers';
  const { devices } = useModbusListDevices(agentId, needsUnit && agentId.length > 0);

  const selectedAgentName = gatewayAgents.find((a) => a.id === agentId)?.name;
  const canSave = agentId.length > 0 && (!needsUnit || unitId.length > 0);

  const handleConfirm = () => {
    if (!canSave) return;
    const config = needsUnit ? { agentId, unitId: Number(unitId) } : { agentId };
    onConfirm(config, selectedAgentName);
  };

  return (
    <>
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('dashboard.addPanel.selectModbusGateway')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* 본문 */}
      <div className="space-y-4 px-5 py-4">
        {/* 에이전트 선택(modbus-gateway 만) */}
        <div>
          <label
            htmlFor="modbus-agent-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.settings.agent')} <span className="text-red-500">*</span>
          </label>
          <select
            id="modbus-agent-select"
            data-testid="modbus-agent-select"
            value={agentId}
            onChange={(e) => {
              setAgentId(e.target.value);
              setUnitId('');
            }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t('dashboard.settings.selectAgent')}</option>
            {gatewayAgents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>

        {/* 대상 unit 선택(가상 디바이스 레지스터 맵 전용, 2차 스텝) */}
        {needsUnit && (
          <div>
            <label
              htmlFor="modbus-unit-select"
              className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
            >
              {t('dashboard.modbus.selectUnit')} <span className="text-red-500">*</span>
            </label>
            <select
              id="modbus-unit-select"
              data-testid="modbus-unit-select"
              value={unitId}
              onChange={(e) => setUnitId(e.target.value)}
              disabled={!agentId}
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
            >
              <option value="">{t('dashboard.modbus.selectUnitPlaceholder')}</option>
              {devices.map((d) => (
                <option key={d.unit_id} value={String(d.unit_id)}>
                  {formatUnitLabel(d.unit_id)} · {d.name || formatUnitLabel(d.unit_id)}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* 푸터 */}
      <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
        <button
          type="button"
          onClick={onBack}
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
        >
          {t('dashboard.addPanel.previous')}
        </button>
        <button
          type="button"
          data-testid="modbus-save"
          onClick={handleConfirm}
          disabled={!canSave}
          className={cn(
            'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
            canSave
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'cursor-not-allowed bg-gray-300 text-gray-500 dark:bg-gray-700 dark:text-gray-500',
          )}
        >
          {t('dashboard.addPanel.save')}
        </button>
      </div>
    </>
  );
}

// ---- Step: 에이전트 상태 패널 대상 선택 (SPEC-DASHBOARD-002) ----

/**
 * 에이전트 상태 패널 대상 선택 스텝. ModbusAgentStep 을 미러링하되 타입 필터를 제거해
 * 전체 연결 에이전트를 제시한다(타입 무관 단일 에이전트 바인딩). 완료 시 { agentId } 를
 * config 로 저장하고 기본 타이틀은 에이전트 이름으로 한다.
 */
function AgentStatusAgentStep({
  onConfirm,
  onBack,
  onClose,
  agentType,
  titleKey = 'dashboard.addPanel.selectAgentStatus',
  testIdPrefix = 'agent-status',
}: {
  onConfirm: (agentId: string, title?: string) => void;
  onBack: () => void;
  onClose: () => void;
  /**
   * 목록을 좁힐 에이전트 타입. 생략하면 전체를 제시한다.
   *
   * 유형마다 불리언 플래그(needsAgent / needsAgentStatus / ...)를 늘리는 대신
   * 필터를 파라미터로 받는다 — 플래그 방식은 세 번째부터 무너진다.
   */
  agentType?: string;
  /** 헤더 제목 i18n 키 */
  titleKey?: string;
  /** 테스트 훅 접두사 (select / save 버튼) */
  testIdPrefix?: string;
}) {
  const { t } = useTranslation();
  const [agentId, setAgentId] = useState('');

  const { data: agentsResult } = useAgents();
  // agentType 이 없으면 전체 연결 에이전트를 제시한다(ModbusAgentStep 과의 핵심 차이).
  const agents = useMemo(() => {
    const all = agentsResult?.data ?? [];
    return agentType ? all.filter((a) => a.type === agentType) : all;
  }, [agentsResult, agentType]);

  const selectedAgentName = agents.find((a) => a.id === agentId)?.name;
  const canSave = agentId.length > 0;

  const handleConfirm = () => {
    if (!canSave) return;
    onConfirm(agentId, selectedAgentName);
  };

  return (
    <>
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t(titleKey)}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* 본문: 전체 타입 에이전트 선택 */}
      <div className="space-y-4 px-5 py-4">
        <div>
          <label
            htmlFor={`${testIdPrefix}-select`}
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.settings.agent')} <span className="text-red-500">*</span>
          </label>
          <select
            id={`${testIdPrefix}-select`}
            data-testid={`${testIdPrefix}-select`}
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t('dashboard.settings.selectAgent')}</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} ({a.type})
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 푸터 */}
      <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
        <button
          type="button"
          onClick={onBack}
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
        >
          {t('dashboard.addPanel.previous')}
        </button>
        <button
          type="button"
          data-testid={`${testIdPrefix}-save`}
          onClick={handleConfirm}
          disabled={!canSave}
          className={cn(
            'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
            canSave
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'cursor-not-allowed bg-gray-300 text-gray-500 dark:bg-gray-700 dark:text-gray-500',
          )}
        >
          {t('dashboard.addPanel.save')}
        </button>
      </div>
    </>
  );
}
