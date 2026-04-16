// 패널 상세 설정 다이얼로그.
// 편집 모드에서 패널별 설정(타이틀, 색상, 컬럼/메트릭 가시성, 타입별 설정)을 관리한다.

import { useCallback, useEffect, useState } from 'react';
import { ArrowUpDown, Check, Fan, Gauge, Minus, Pipette, Plus, Power, Snowflake, Thermometer, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

import { useDevices, useDeviceRealtime } from '@/hooks/useDevice';
import { useFlows } from '@/hooks/useFlow';
import type { GaugeType } from './panels/GaugePanel';
import { getDeviceTypeLabel, getPropertyLabel } from '@/lib/utils/deviceLabels';
import {
  ChartChannelSection,
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
} from './ChartPanelSections';
import {
  useUIStore,
  type PanelConfig,
  type FlowColumnKey,
  type AgentColumnKey,
  type DeviceColumnKey,
  type MetricKey,
  ALL_FLOW_COLUMNS,
  ALL_AGENT_COLUMNS,
  ALL_DEVICE_COLUMNS,
  ALL_METRIC_KEYS,
} from '@/stores/uiStore';

// ---- 차트 패널 공통 (SPEC-CHART-001 M5) ----

/** 차트 계열 패널 타입 집합 (REQ-M5-03) */
const CHART_PANEL_TYPES = new Set(['stat', 'line-chart', 'bar-chart', 'pie-chart', 'table']);

/** 패널 색상 프리셋 */
const COLOR_PRESETS = [
  '#3b82f6', // blue
  '#10b981', // emerald
  '#f59e0b', // amber
  '#ef4444', // red
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#f97316', // orange
  '#64748b', // slate
  '#0f172a', // dark
];

/** 컬럼 라벨 매핑 */
const FLOW_COLUMN_LABELS: Record<FlowColumnKey, string> = {
  name: '이름',
  status: '상태',
  node_count: '노드 수',
  updated_at: '수정일',
  actions: '액션',
};

const AGENT_COLUMN_LABELS: Record<AgentColumnKey, string> = {
  name: '이름',
  type: '타입',
  status: '상태',
  uptime: '업타임',
  messages: '메시지 IN/OUT',
  actions: '액션',
};

const DEVICE_COLUMN_LABELS: Record<DeviceColumnKey, string> = {
  name: '이름',
  type: '타입',
  status: '상태',
  agent: '에이전트',
  last_seen: '최근 통신',
};

const METRIC_LABELS: Record<MetricKey, string> = {
  cpu: 'CPU',
  memory: '메모리',
  throughput: '처리량',
  errorRate: '에러율',
};

interface PanelSettingsDialogProps {
  panelId: string | null;
  onClose: () => void;
}

/** 패널 상세 설정 모달 */
export default function PanelSettingsDialog({ panelId, onClose }: PanelSettingsDialogProps) {
  const activePage = useUIStore((s) =>
    s.dashboardPages.find((p) => p.id === s.activeDashboardId),
  );
  const updatePanelConfig = useUIStore((s) => s.updatePanelConfig);
  const updatePanelTitle = useUIStore((s) => s.updatePanelTitle);

  const panel = activePage?.panels.find((p) => p.id === panelId) ?? null;

  // 악센트 그룹 선택 상태 (좌측 컬럼에 컨트롤 표시용)
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null);

  // 악센트 관련 파생값
  const accentElements = (panel?.config?.accentElements as Record<string, string | boolean>) ?? {};
  const panelColor = panel?.config?.panelColor as string | undefined;
  const effectiveColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };
  const getSubProp = (group: string, prop: string): string | undefined => {
    const val = accentElements[`${group}.${prop}`];
    return typeof val === 'string' ? val : undefined;
  };

  // 패널 변경 시 선택 그룹 초기화
  useEffect(() => {
    setSelectedGroup(null);
  }, [panelId]);

  // 악센트 라벨 결정
  const accentLabels = panel?.type === 'device' || panel?.type === 'ac-control' || panel?.type === 'hvac-control' || panel?.type === 'properties-grid'
    ? ACCENT_ELEMENT_LABELS
    : panel?.type === 'resource'
    ? RESOURCE_ACCENT_LABELS
    : panel?.type === 'logs'
    ? LOG_ACCENT_LABELS
    : panel?.type === 'gauge'
    ? GAUGE_ACCENT_LABELS
    : LIST_ACCENT_LABELS;

  // ESC 키로 닫기
  useEffect(() => {
    if (!panel) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [panel, onClose]);

  // 배경 클릭 시 닫기
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  if (!panel) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="panel-settings-dialog-title"
    >
      <div className="mx-4 flex h-[min(620px,85vh)] w-full max-w-[740px] flex-col rounded-2xl bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex shrink-0 items-center justify-between px-5 pt-4 pb-3">
          <h2
            id="panel-settings-dialog-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            패널 설정
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-4.5 w-4.5" />
          </button>
        </div>

        <div className="border-t border-(--color-border-default)" />

        {/* 설정 내용 - 2컬럼 레이아웃 */}
        <div className="flex min-h-0 flex-1 gap-6 px-5 pb-5 pt-4">
          {/* 좌측 컬럼: 설정 + 악센트 컨트롤 */}
          <div className="w-[260px] shrink-0 space-y-5 overflow-y-auto pr-1">
            {/* 공통: 타이틀 */}
            <TitleSection panel={panel} onTitleChange={(t) => updatePanelTitle(panel.id, t)} />

            {/* 디바이스 선택 (device/ac-control/hvac-control/properties-grid) */}
            {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control' || panel.type === 'properties-grid') && (
              <DeviceSection
                panel={panel}
                onConfigChange={(c) => updatePanelConfig(panel.id, c)}
              />
            )}

            {/* 타입별 설정 */}
            {panel.type === 'flows' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <ColumnsSection<FlowColumnKey>
                  allColumns={[...ALL_FLOW_COLUMNS]}
                  labels={FLOW_COLUMN_LABELS}
                  visibleColumns={(panel.config?.visibleColumns as FlowColumnKey[]) ?? [...ALL_FLOW_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </>
            )}
            {panel.type === 'agents' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <ColumnsSection<AgentColumnKey>
                  allColumns={[...ALL_AGENT_COLUMNS]}
                  labels={AGENT_COLUMN_LABELS}
                  visibleColumns={(panel.config?.visibleColumns as AgentColumnKey[]) ?? [...ALL_AGENT_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </>
            )}
            {panel.type === 'devices' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <ColumnsSection<DeviceColumnKey>
                  allColumns={[...ALL_DEVICE_COLUMNS]}
                  labels={DEVICE_COLUMN_LABELS}
                  visibleColumns={(panel.config?.visibleColumns as DeviceColumnKey[]) ?? [...ALL_DEVICE_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </>
            )}
            {panel.type === 'resource' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <ResourceSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'logs' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <LogsSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'gauge' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <GaugeSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'properties-grid' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <PropertiesGridSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}

            {/* 차트 패널 공통: channel_name (SPEC-CHART-001 REQ-M5-04) */}
            {CHART_PANEL_TYPES.has(panel.type) && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <ChartChannelSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}

            {/* 차트 타입별 세부 설정 (SPEC-CHART-001 §4.2.2 / REQ-M5-03) */}
            {panel.type === 'stat' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <StatChartSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'line-chart' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <LineChartSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'bar-chart' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <BarChartSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'pie-chart' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <PieChartSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}
            {panel.type === 'table' && (
              <>
                <div className="border-t border-(--color-border-default)" />
                <TableChartSection
                  panel={panel}
                  onConfigChange={(c) => updatePanelConfig(panel.id, c)}
                />
              </>
            )}

          </div>

          {/* 우측 컬럼: 프리뷰 + 악센트 컨트롤 */}
          <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto">
            <label className="shrink-0 text-xs font-medium text-(--color-text-muted)">패널 스타일 미리보기</label>
            {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control') && (
              <NasaMiniPreview
                selectedGroup={selectedGroup}
                onSelectGroup={setSelectedGroup}
                effectiveColor={effectiveColor}
                getSubProp={getSubProp}
                panelColor={panelColor}
              />
            )}
            {panel.type === 'properties-grid' && (
              <GridMiniPreview
                selectedGroup={selectedGroup}
                onSelectGroup={setSelectedGroup}
                effectiveColor={effectiveColor}
                panelColor={panelColor}
                gridCols={(panel.config?.gridCols as number | undefined) ?? 3}
              />
            )}
            {(panel.type === 'flows' || panel.type === 'agents' || panel.type === 'devices') && (
              <ListMiniPreview
                selectedGroup={selectedGroup}
                onSelectGroup={setSelectedGroup}
                effectiveColor={effectiveColor}
                panelColor={panelColor}
                variant={panel.type as 'flows' | 'agents' | 'devices'}
              />
            )}
            {panel.type === 'resource' && (
              <ResourceMiniPreview
                selectedGroup={selectedGroup}
                onSelectGroup={setSelectedGroup}
                effectiveColor={effectiveColor}
                panelColor={panelColor}
              />
            )}
            {panel.type === 'logs' && (
              <LogMiniPreview
                selectedGroup={selectedGroup}
                onSelectGroup={setSelectedGroup}
                effectiveColor={effectiveColor}
                panelColor={panelColor}
              />
            )}
            {panel.type === 'gauge' && (
              <GaugeMiniPreview panel={panel} />
            )}
            {/* 악센트 그룹 컨트롤 (프리뷰에서 선택 시 표시) */}
            {selectedGroup && (
              <AccentGroupControls
                selected={selectedGroup}
                labels={accentLabels}
                accentElements={accentElements}
                panelColor={panelColor}
                onChange={(elements) => updatePanelConfig(panel.id, { accentElements: elements })}
                onPanelColorChange={(color) => updatePanelConfig(panel.id, { panelColor: color })}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

/** 타이틀 입력 섹션 */
function TitleSection({
  panel,
  onTitleChange,
}: {
  panel: PanelConfig;
  onTitleChange: (title: string) => void;
}) {
  const [draft, setDraft] = useState(panel.title);

  useEffect(() => {
    setDraft(panel.title);
  }, [panel.title]);

  const handleBlur = () => {
    const trimmed = draft.trim();
    if (trimmed && trimmed !== panel.title) {
      onTitleChange(trimmed);
    } else {
      setDraft(panel.title);
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        타이틀
      </label>
      <input
        type="text"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={handleBlur}
        onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}

/** 색상 선택 섹션 */
/** 컬럼 가시성 토글 섹션 (flows, agents 공용) */
function ColumnsSection<T extends string>({
  allColumns,
  labels,
  visibleColumns,
  onChange,
}: {
  allColumns: T[];
  labels: Record<T, string>;
  visibleColumns: T[];
  onChange: (cols: T[]) => void;
}) {
  const toggle = (key: T) => {
    if (visibleColumns.includes(key)) {
      if (visibleColumns.length > 1) {
        onChange(visibleColumns.filter((c) => c !== key));
      }
    } else {
      onChange([...visibleColumns, key]);
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        표시 항목
      </label>
      <div className="space-y-1">
        {allColumns.map((key) => (
          <label
            key={key}
            className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
          >
            <input
              type="checkbox"
              checked={visibleColumns.includes(key)}
              onChange={() => toggle(key)}
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm text-(--color-text-primary)">{labels[key]}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

/** 리소스 패널 전용 설정 (메트릭 + 그리드 열 수) */
function ResourceSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const visibleMetrics = (panel.config?.visibleMetrics as MetricKey[]) ?? [...ALL_METRIC_KEYS];
  const gridCols = (panel.config?.gridCols as number | undefined) ?? visibleMetrics.length;

  const toggleMetric = (key: MetricKey) => {
    if (visibleMetrics.includes(key)) {
      if (visibleMetrics.length > 1) {
        onConfigChange({ visibleMetrics: visibleMetrics.filter((m) => m !== key) });
      }
    } else {
      onConfigChange({ visibleMetrics: [...visibleMetrics, key] });
    }
  };

  return (
    <>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          표시 메트릭
        </label>
        <div className="space-y-1">
          {ALL_METRIC_KEYS.map((key) => (
            <label
              key={key}
              className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
            >
              <input
                type="checkbox"
                checked={visibleMetrics.includes(key)}
                onChange={() => toggleMetric(key)}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="text-sm text-(--color-text-primary)">{METRIC_LABELS[key]}</span>
            </label>
          ))}
        </div>
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          열 수
        </label>
        <div className="flex gap-1">
          {[1, 2, 3, 4].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => onConfigChange({ gridCols: n })}
              className={`flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors ${
                gridCols === n
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80'
              }`}
            >
              {n}
            </button>
          ))}
        </div>
      </div>
    </>
  );
}

/** 디바이스 제어 패널 전용 설정 (디바이스 선택) */
function DeviceSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const currentDeviceId = panel.config?.deviceId as string | undefined;
  const { data: devicesData, isLoading } = useDevices();
  const allDevices = devicesData?.data ?? [];

  // ac-control/hvac-control 패널은 실외기(outdoor)를 제외한다
  const devices = (panel.type === 'ac-control' || panel.type === 'hvac-control')
    ? allDevices.filter((d) => d.type !== 'outdoor')
    : allDevices;

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        디바이스
      </label>
      {isLoading ? (
        <div className="flex items-center justify-center py-4">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
        </div>
      ) : devices.length === 0 ? (
        <p className="py-2 text-sm text-(--color-text-muted)">등록된 디바이스가 없습니다.</p>
      ) : (
        <select
          value={currentDeviceId ?? ''}
          onChange={(e) => onConfigChange({ deviceId: e.target.value || undefined })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="">선택하세요</option>
          {devices.map((device) => (
            <option key={device.id} value={device.id}>
              {device.name || device.id} ({getDeviceTypeLabel(device.type)})
            </option>
          ))}
        </select>
      )}
    </div>
  );
}

/** 속성 그리드 패널 전용 설정 (열 수 + 표시 항목) */
function PropertiesGridSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const gridCols = (panel.config?.gridCols as number | undefined) ?? 3;
  const visibleProperties = (panel.config?.visibleProperties as string[] | undefined) ?? [];
  const deviceId = panel.config?.deviceId as string | undefined;
  const { data: deviceData } = useDeviceRealtime(deviceId ?? '');
  const allKeys = deviceData?.state?.properties ? Object.keys(deviceData.state.properties) : [];
  const protocol = deviceData?.protocol ?? '';
  const type = deviceData?.type ?? '';

  const toggleProperty = (key: string) => {
    if (visibleProperties.includes(key)) {
      onConfigChange({ visibleProperties: visibleProperties.filter((k) => k !== key) });
    } else {
      onConfigChange({ visibleProperties: [...visibleProperties, key] });
    }
  };

  const isAllSelected = visibleProperties.length === 0;

  return (
    <>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          열 수
        </label>
        <div className="flex gap-1">
          {[1, 2, 3, 4, 5, 6].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => onConfigChange({ gridCols: n })}
              className={`flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors ${
                gridCols === n
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80'
              }`}
            >
              {n}
            </button>
          ))}
        </div>
      </div>
      {allKeys.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            표시 항목
          </label>
          <div className="space-y-1">
            <label
              className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
            >
              <input
                type="checkbox"
                checked={isAllSelected}
                onChange={() => onConfigChange({ visibleProperties: [] })}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="text-sm font-medium text-(--color-text-primary)">전체</span>
            </label>
            {allKeys.map((key) => (
              <label
                key={key}
                className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
              >
                <input
                  type="checkbox"
                  checked={isAllSelected || visibleProperties.includes(key)}
                  onChange={() => {
                    if (isAllSelected) {
                      // "전체" 해제 → 이 항목만 제외
                      onConfigChange({ visibleProperties: allKeys.filter((k) => k !== key) });
                    } else {
                      toggleProperty(key);
                    }
                  }}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-(--color-text-primary)">{getPropertyLabel(key, protocol, type)}</span>
              </label>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

/** 로그 패널 전용 설정 (최대 줄 수) */
function LogsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const maxLines = (panel.config?.maxLines as number) || 100;
  const [draft, setDraft] = useState(String(maxLines));

  useEffect(() => {
    setDraft(String(maxLines));
  }, [maxLines]);

  const handleBlur = () => {
    const n = parseInt(draft, 10);
    if (!isNaN(n) && n > 0 && n !== maxLines) {
      onConfigChange({ maxLines: n });
    } else {
      setDraft(String(maxLines));
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        최대 줄 수
      </label>
      <input
        type="number"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={handleBlur}
        onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        min={10}
        max={10000}
        className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}

/** 악센트 적용 요소 그룹 (디바이스 리모컨) */
const ACCENT_ELEMENT_LABELS: Record<string, string> = {
  _base: '전체 색상',
  temperature: '온도 표시',
  controls: '제어 버튼',
  labels: '라벨/텍스트',
  borders: '테두리/구분선',
  indicators: '상태 표시',
};

/** 리스트 패널 (flows/agents/devices) 악센트 그룹 */
const LIST_ACCENT_LABELS: Record<string, string> = {
  _base: '전체 색상',
  header: '타이틀',
  badges: '요약 배지',
  table: '테이블 헤더',
};

/** 리소스 패널 악센트 그룹 */
const RESOURCE_ACCENT_LABELS: Record<string, string> = {
  _base: '전체 색상',
  header: '타이틀',
  cpu: 'CPU 카드',
  memory: '메모리 카드',
  throughput: '처리량 카드',
  errorRate: '에러율 카드',
};

/** 로그 패널 악센트 그룹 */
const LOG_ACCENT_LABELS: Record<string, string> = {
  _base: '전체 색상',
  header: '타이틀',
  levels: '레벨 배지',
  timestamp: '타임스탬프',
  source: '소스 라벨',
};

/** 게이지 패널 악센트 그룹 */
const GAUGE_ACCENT_LABELS: Record<string, string> = {
  _base: '전체 색상',
  header: '타이틀',
  arc: '게이지 호',
  value: '값 텍스트',
};

/** 게이지 유형 메타 */
const GAUGE_TYPE_META: { type: GaugeType; label: string; icon: string }[] = [
  { type: 'simple', label: '심플', icon: 'O' },
  { type: 'half', label: '반원', icon: 'U' },
  { type: 'multi-ring', label: '멀티링', icon: '(O)' },
  { type: 'needle', label: '니들', icon: '>' },
  { type: 'needle-rainbow', label: '니들 RB', icon: '>>' },
  { type: 'vertical-bar', label: '세로 바', icon: '|' },
  { type: 'half-rainbow', label: '반원 RB', icon: 'U+' },
];

/** 데이터 소스 바인딩 */
interface DataSourceBinding {
  sourceType: 'resource' | 'flow';
  resource?: string;
  flowId?: string;
  dataField?: string;
}

/** 연속 컬러 테마 프리셋 */
const COLOR_THEME_PRESETS = [
  { id: 'green-red', label: '초록-노랑-빨강', colors: ['#10b981', '#f59e0b', '#ef4444'] },
  { id: 'blue-purple', label: '파랑-보라', colors: ['#3b82f6', '#8b5cf6', '#a855f7'] },
  { id: 'cyan-blue', label: '시안-파랑', colors: ['#06b6d4', '#3b82f6', '#1e40af'] },
];

/** 서브 속성용 색상 프리셋 (흰/검 포함) */
const SUB_COLOR_PRESETS = ['#ffffff', '#000000', ...COLOR_PRESETS];

/** 라운드 프리셋 */
const RADIUS_PRESETS = [
  { value: '0', label: '각진' },
  { value: '4', label: '약간' },
  { value: '8', label: '보통' },
  { value: '9999', label: '원형' },
];

/** 서브 속성 색상 팔레트 행 */
function SubColorRow({
  label,
  color,
  onColorChange,
}: {
  label: string;
  color: string | undefined;
  onColorChange: (c: string | undefined) => void;
}) {
  return (
    <div>
      <div className="mb-1 flex items-center gap-1.5">
        <span className="text-[11px] text-(--color-text-muted)">{label}</span>
        {color && (
          <span
            className="h-2.5 w-2.5 rounded-full border border-gray-200 dark:border-gray-600"
            style={{ backgroundColor: color }}
          />
        )}
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          {SUB_COLOR_PRESETS.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => onColorChange(c)}
              className={cn(
                'relative h-5 w-5 rounded-full transition-transform hover:scale-110',
                c === '#ffffff' && color !== c && 'ring-1 ring-gray-200 dark:ring-gray-600',
              )}
              style={{ backgroundColor: c }}
              aria-label={`${label} ${c}`}
            >
              {color === c && <Check className={cn('absolute inset-0 m-auto h-3 w-3 drop-shadow', c === '#ffffff' ? 'text-gray-700' : 'text-white')} />}
            </button>
          ))}
          <button
            type="button"
            onClick={() => onColorChange(undefined)}
            className={cn(
              'flex h-5 w-5 items-center justify-center rounded-full border-2 transition-transform hover:scale-110',
              !color
                ? 'border-blue-500 bg-(--color-bg-surface) text-(--color-text-secondary)'
                : 'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-muted)',
            )}
            title="기본"
          >
            <X className="h-2.5 w-2.5" />
          </button>
        </div>
        <div className="flex items-center gap-1.5">
          <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md bg-(--color-bg-elevated) transition-colors hover:bg-(--color-border-default)">
            <Pipette className="h-3.5 w-3.5 text-(--color-text-muted)" />
            <input
              type="color"
              value={color ?? '#3b82f6'}
              onChange={(e) => onColorChange(e.target.value)}
              className="absolute inset-0 cursor-pointer opacity-0"
            />
          </label>
          <div className="flex h-6 items-center gap-px rounded-md bg-(--color-bg-elevated) px-1.5 text-[11px] font-mono text-(--color-text-secondary)">
            <span className="text-(--color-text-muted)">#</span>
            <input type="text" value={(color ?? '#3b82f6').replace('#', '').toUpperCase()}
              onChange={(e) => { const v = e.target.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6); if (v.length === 6) onColorChange(`#${v}`); }}
              className="w-14 bg-transparent text-center outline-none" maxLength={6} />
            <span className="mx-1 h-3 w-px bg-(--color-border-default)" />
            <span className="text-(--color-text-muted)">100%</span>
          </div>
        </div>
      </div>
    </div>
  );
}

/** 범용 악센트 그룹 컨트롤 패널 — 체크박스 + 색상 팔레트 */
function AccentGroupControls({
  selected,
  labels,
  accentElements,
  panelColor,
  onChange,
  onPanelColorChange,
}: {
  selected: string;
  labels: Record<string, string>;
  accentElements: Record<string, string | boolean>;
  panelColor: string | undefined;
  onChange: (elements: Record<string, string | boolean>) => void;
  onPanelColorChange?: (color: string | undefined) => void;
}) {
  // _base 그룹은 panelColor를 직접 제어
  const isBase = selected === '_base';
  const isEnabled = isBase ? true : accentElements[selected] !== false;
  const gc = isBase ? panelColor : (typeof accentElements[selected] === 'string' ? (accentElements[selected] as string) : undefined);

  // 서브 속성 접근
  const getSubProp = (group: string, prop: string): string | undefined => {
    const val = accentElements[`${group}.${prop}`];
    return typeof val === 'string' ? val : undefined;
  };
  const setSubProp = (group: string, prop: string, value: string | undefined) => {
    const key = `${group}.${prop}`;
    if (value === undefined) {
      const next = { ...accentElements };
      delete next[key];
      onChange(next);
    } else {
      onChange({ ...accentElements, [key]: value });
    }
  };
  const toggle = () => {
    if (isBase) return; // _base는 항상 활성
    onChange({ ...accentElements, [selected]: isEnabled ? false : true });
  };
  const setColor = (color: string | undefined) => {
    if (isBase) {
      onPanelColorChange?.(color);
    } else {
      onChange({ ...accentElements, [selected]: color ?? true });
    }
  };

  const hasSubProps = selected === 'labels';

  return (
    <div className="mt-3 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
      <div className="mb-2 flex items-center gap-2">
        {!isBase && <input type="checkbox" checked={isEnabled} onChange={toggle} className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500" />}
        <span className="text-sm font-medium text-(--color-text-primary)">{labels[selected]}</span>
        {isEnabled && hasSubProps ? (
          <div className="ml-auto flex gap-1">
            {getSubProp('labels', 'bg') && <span className="h-3 w-3 rounded-sm border border-white/50" style={{ backgroundColor: getSubProp('labels', 'bg') }} title="배경" />}
            {getSubProp('labels', 'text') && <span className="h-3 w-3 rounded-sm border border-white/50" style={{ backgroundColor: getSubProp('labels', 'text') }} title="글자" />}
          </div>
        ) : isEnabled && gc ? (
          <span className="ml-auto h-3.5 w-3.5 rounded-full border border-white/50" style={{ backgroundColor: gc }} />
        ) : isEnabled ? (
          <span className="ml-auto text-[10px] text-(--color-text-muted)">패널 색상</span>
        ) : null}
      </div>

      {isEnabled && hasSubProps ? (
        <div className="space-y-3">
          <SubColorRow label="배경" color={getSubProp('labels', 'bg')} onColorChange={(c) => setSubProp('labels', 'bg', c)} />
          <SubColorRow label="글자" color={getSubProp('labels', 'text')} onColorChange={(c) => setSubProp('labels', 'text', c)} />
          <div>
            <span className="mb-1 block text-[11px] text-(--color-text-muted)">라운드</span>
            <div className="flex gap-1">
              {RADIUS_PRESETS.map((r) => (
                <button key={r.value} type="button"
                  onClick={() => setSubProp('labels', 'radius', getSubProp('labels', 'radius') === r.value ? undefined : r.value)}
                  className={cn('rounded px-2.5 py-1 text-[11px] font-medium transition-colors',
                    getSubProp('labels', 'radius') === r.value ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                      : 'bg-(--color-bg-surface) text-(--color-text-secondary) hover:bg-(--color-bg-surface)/80')}
                >{r.label}</button>
              ))}
            </div>
          </div>
        </div>
      ) : isEnabled ? (
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5">
            {COLOR_PRESETS.map((color) => (
              <button key={color} type="button" onClick={() => setColor(color)}
                className="relative h-5 w-5 rounded-full transition-transform hover:scale-110"
                style={{ backgroundColor: color }} aria-label={`${labels[selected]} ${color}`}>
                {gc === color && <Check className="absolute inset-0 m-auto h-3 w-3 text-white drop-shadow" />}
              </button>
            ))}
            <button type="button" onClick={() => setColor(undefined)}
              className={cn('flex h-5 w-5 items-center justify-center rounded-full border-2 transition-transform hover:scale-110',
                !gc ? 'border-blue-500 bg-(--color-bg-surface) text-(--color-text-secondary)' : 'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-muted)')}
              title="패널 색상"><X className="h-2.5 w-2.5" /></button>
          </div>
          <div className="flex items-center gap-1.5">
            <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md bg-(--color-bg-elevated) transition-colors hover:bg-(--color-border-default)">
              <Pipette className="h-3.5 w-3.5 text-(--color-text-muted)" />
              <input type="color" value={gc ?? panelColor ?? '#3b82f6'} onChange={(e) => setColor(e.target.value)} className="absolute inset-0 cursor-pointer opacity-0" />
            </label>
            <div className="flex h-6 items-center gap-px rounded-md bg-(--color-bg-elevated) px-1.5 text-[11px] font-mono text-(--color-text-secondary)">
              <span className="text-(--color-text-muted)">#</span>
              <input type="text" value={(gc ?? panelColor ?? '#3b82f6').replace('#', '').toUpperCase()}
                onChange={(e) => { const v = e.target.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6); if (v.length === 6) setColor(`#${v}`); }}
                className="w-14 bg-transparent text-center outline-none" maxLength={6} />
              <span className="mx-1 h-3 w-px bg-(--color-border-default)" />
              <span className="text-(--color-text-muted)">100%</span>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** 게이지 패널 전용 설정 섹션 */
function GaugeSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const config = panel.config ?? {};
  const gaugeType = (config.gaugeType as GaugeType) ?? 'simple';
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const unit = (config.unit as string) ?? '%';
  const dataSources = (config.dataSources as DataSourceBinding[]) ?? [{ sourceType: 'resource', resource: 'cpu' }];
  const colorMode = (config.colorMode as 'individual' | 'continuous') ?? 'individual';
  const colorTheme = (config.colorTheme as string) ?? 'green-red';
  const thresholds = (config.thresholds as { name: string; color: string; from: number; to: number }[]) ?? [
    { name: '정상', color: '#10b981', from: 0, to: 60 },
    { name: '주의', color: '#f59e0b', from: 60, to: 80 },
    { name: '위험', color: '#ef4444', from: 80, to: 100 },
  ];

  // 플로우 목록 (데이터 소스 선택용)
  const { data: flowsData } = useFlows();
  const flows = flowsData?.data ?? [];

  // 로컬 드래프트
  const [minDraft, setMinDraft] = useState(String(min));
  const [maxDraft, setMaxDraft] = useState(String(max));
  const [unitDraft, setUnitDraft] = useState(unit);

  useEffect(() => { setMinDraft(String(min)); }, [min]);
  useEffect(() => { setMaxDraft(String(max)); }, [max]);
  useEffect(() => { setUnitDraft(unit); }, [unit]);

  const commitRange = () => {
    const nMin = parseFloat(minDraft);
    const nMax = parseFloat(maxDraft);
    if (!isNaN(nMin) && !isNaN(nMax)) {
      onConfigChange({ min: nMin, max: nMax });
    }
  };

  const updateDataSource = (index: number, patch: Partial<DataSourceBinding>) => {
    const next = dataSources.map((ds, i) => (i === index ? { ...ds, ...patch } : ds));
    onConfigChange({ dataSources: next });
  };

  const addDataSource = () => {
    onConfigChange({ dataSources: [...dataSources, { sourceType: 'resource' as const, resource: 'cpu' }] });
  };

  const removeDataSource = (index: number) => {
    if (dataSources.length <= 1) return;
    onConfigChange({ dataSources: dataSources.filter((_, i) => i !== index) });
  };

  const updateThreshold = (index: number, patch: Partial<{ name: string; color: string; from: number; to: number }>) => {
    const next = thresholds.map((t, i) => (i === index ? { ...t, ...patch } : t));
    onConfigChange({ thresholds: next });
  };

  const addThreshold = () => {
    const lastTo = thresholds.length > 0 ? thresholds[thresholds.length - 1]!.to : 0;
    onConfigChange({
      thresholds: [...thresholds, { name: '', color: '#64748b', from: lastTo, to: max }],
    });
  };

  const removeThreshold = (index: number) => {
    onConfigChange({ thresholds: thresholds.filter((_, i) => i !== index) });
  };

  return (
    <div className="space-y-4">
      {/* A. 게이지 유형 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          게이지 유형
        </label>
        <div className="grid grid-cols-4 gap-1">
          {GAUGE_TYPE_META.map(({ type, label }) => (
            <button
              key={type}
              type="button"
              onClick={() => onConfigChange({ gaugeType: type })}
              className={cn(
                'flex flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] font-medium transition-colors',
                gaugeType === type
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
              )}
            >
              <GaugeTypeIcon type={type} size={18} active={gaugeType === type} />
              <span className="leading-tight">{label}</span>
            </button>
          ))}
        </div>
      </div>

      {/* B. 값 범위 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          값 범위
        </label>
        <div className="flex items-center gap-2">
          <input
            type="number"
            value={minDraft}
            onChange={(e) => setMinDraft(e.target.value)}
            onBlur={commitRange}
            onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2.5 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            placeholder="최소"
          />
          <span className="shrink-0 text-xs text-(--color-text-muted)">~</span>
          <input
            type="number"
            value={maxDraft}
            onChange={(e) => setMaxDraft(e.target.value)}
            onBlur={commitRange}
            onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2.5 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            placeholder="최대"
          />
        </div>
      </div>

      {/* C. 단위 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          단위
        </label>
        <input
          type="text"
          value={unitDraft}
          onChange={(e) => setUnitDraft(e.target.value)}
          onBlur={() => { if (unitDraft !== unit) onConfigChange({ unit: unitDraft }); }}
          onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          placeholder="%"
        />
      </div>

      {/* D. 값 지정 (데이터 소스) */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          값 지정
        </label>
        <div className="space-y-1.5">
          {dataSources.map((ds, idx) => (
            <div key={idx} className="flex items-center gap-1.5">
              <select
                value={ds.sourceType}
                onChange={(e) => updateDataSource(idx, { sourceType: e.target.value as 'resource' | 'flow', resource: undefined, flowId: undefined, dataField: undefined })}
                className="w-[72px] shrink-0 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              >
                <option value="resource">리소스</option>
                <option value="flow">플로우</option>
              </select>
              {ds.sourceType === 'resource' ? (
                <select
                  value={ds.resource ?? 'cpu'}
                  onChange={(e) => updateDataSource(idx, { resource: e.target.value })}
                  className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                >
                  <option value="cpu">CPU 사용률</option>
                  <option value="memory">메모리 사용률</option>
                </select>
              ) : (
                <div className="flex min-w-0 flex-1 gap-1">
                  <select
                    value={ds.flowId ?? ''}
                    onChange={(e) => updateDataSource(idx, { flowId: e.target.value || undefined })}
                    className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  >
                    <option value="">플로우 선택</option>
                    {flows.map((f) => (
                      <option key={f.id} value={f.id}>{f.name || f.id}</option>
                    ))}
                  </select>
                </div>
              )}
              {gaugeType === 'multi-ring' && dataSources.length > 1 && (
                <button
                  type="button"
                  onClick={() => removeDataSource(idx)}
                  className="shrink-0 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-red-500"
                  aria-label="데이터 소스 삭제"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          ))}
          {gaugeType === 'multi-ring' && (
            <button
              type="button"
              onClick={addDataSource}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
            >
              <Plus className="h-3 w-3" />
              데이터 소스 추가
            </button>
          )}
        </div>
      </div>

      {/* E. 임계값 및 컬러 설정 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          임계값 및 컬러
        </label>
        {/* 모드 토글 */}
        <div className="mb-2 flex gap-1">
          <button
            type="button"
            onClick={() => onConfigChange({ colorMode: 'individual' })}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
              colorMode === 'individual'
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
            )}
          >
            개별 지정
          </button>
          <button
            type="button"
            onClick={() => onConfigChange({ colorMode: 'continuous' })}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
              colorMode === 'continuous'
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
            )}
          >
            연속 컬러
          </button>
        </div>

        {colorMode === 'individual' ? (
          <div className="space-y-1.5">
            {thresholds.map((t, idx) => (
              <div key={idx} className="flex items-center gap-1">
                <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors hover:opacity-80">
                  <span
                    className="h-4 w-4 rounded-sm border border-gray-200 dark:border-gray-600"
                    style={{ backgroundColor: t.color }}
                  />
                  <input
                    type="color"
                    value={t.color}
                    onChange={(e) => updateThreshold(idx, { color: e.target.value })}
                    className="absolute inset-0 cursor-pointer opacity-0"
                  />
                </label>
                <input
                  type="text"
                  value={t.name}
                  onChange={(e) => updateThreshold(idx, { name: e.target.value })}
                  placeholder="이름"
                  className="w-[52px] shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <input
                  type="number"
                  value={t.from}
                  onChange={(e) => updateThreshold(idx, { from: parseFloat(e.target.value) || 0 })}
                  className="w-[40px] shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <span className="text-[10px] text-(--color-text-muted)">~</span>
                <input
                  type="number"
                  value={t.to}
                  onChange={(e) => updateThreshold(idx, { to: parseFloat(e.target.value) || 0 })}
                  className="w-[40px] shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <button
                  type="button"
                  onClick={() => removeThreshold(idx)}
                  className="shrink-0 rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
                  aria-label="임계값 삭제"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={addThreshold}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
            >
              <Plus className="h-3 w-3" />
              임계값 추가
            </button>
          </div>
        ) : (
          <div className="flex gap-1.5">
            {COLOR_THEME_PRESETS.map((theme) => (
              <button
                key={theme.id}
                type="button"
                onClick={() => onConfigChange({ colorTheme: theme.id })}
                className={cn(
                  'flex flex-1 flex-col items-center gap-1 rounded-md px-2 py-2 transition-colors',
                  colorTheme === theme.id
                    ? 'ring-2 ring-blue-500 ring-inset bg-blue-50/50 dark:bg-blue-900/20'
                    : 'bg-(--color-bg-elevated) hover:bg-(--color-bg-elevated)/80',
                )}
              >
                <div className="flex gap-0.5">
                  {theme.colors.map((c, i) => (
                    <span key={i} className="h-3 w-3 rounded-full" style={{ backgroundColor: c }} />
                  ))}
                </div>
                <span className="text-[9px] text-(--color-text-muted) leading-tight">{theme.label}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** 게이지 유형 아이콘 (SVG 미니 아이콘) */
function GaugeTypeIcon({ type, size = 18, active }: { type: GaugeType; size?: number; active?: boolean }) {
  const color = active ? '#2563eb' : '#94a3b8';
  const s = size;
  const c = s / 2;
  const r = s / 2 - 2;

  switch (type) {
    case 'simple':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={2.5} strokeDasharray={`${r * Math.PI * 1.5} ${r * Math.PI * 2}`} strokeLinecap="round" />
        </svg>
      );
    case 'half':
      return (
        <svg width={s} height={s * 0.65} viewBox={`0 0 ${s} ${s * 0.65}`}>
          <path d={`M 2 ${s * 0.6} A ${r} ${r} 0 0 1 ${s - 2} ${s * 0.6}`} fill="none" stroke={color} strokeWidth={2.5} strokeLinecap="round" />
        </svg>
      );
    case 'multi-ring':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={1.5} strokeDasharray={`${r * Math.PI * 1.2} 100`} opacity={0.4} />
          <circle cx={c} cy={c} r={r - 3} fill="none" stroke={color} strokeWidth={1.5} strokeDasharray={`${(r - 3) * Math.PI * 1.5} 100`} />
        </svg>
      );
    case 'needle':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={1.5} opacity={0.3} />
          <line x1={c} y1={c} x2={c + r * 0.7} y2={c - r * 0.3} stroke={color} strokeWidth={1.5} strokeLinecap="round" />
          <circle cx={c} cy={c} r={1.5} fill={color} />
        </svg>
      );
    case 'needle-rainbow':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <path d={`M ${c - r} ${c} A ${r} ${r} 0 0 1 ${c + r} ${c}`} fill="none" stroke="#10b981" strokeWidth={2} />
          <path d={`M ${c - r * 0.7} ${c - r * 0.7} A ${r} ${r} 0 0 1 ${c + r} ${c}`} fill="none" stroke="#f59e0b" strokeWidth={2} />
          <line x1={c} y1={c} x2={c + r * 0.5} y2={c - r * 0.5} stroke={color} strokeWidth={1.5} strokeLinecap="round" />
          <circle cx={c} cy={c} r={1.5} fill={color} />
        </svg>
      );
    case 'vertical-bar':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <rect x={s * 0.3} y={2} width={s * 0.4} height={s - 4} rx={2} fill="none" stroke={color} strokeWidth={1.5} opacity={0.3} />
          <rect x={s * 0.3} y={s * 0.4} width={s * 0.4} height={s * 0.6 - 2} rx={2} fill={color} opacity={0.7} />
        </svg>
      );
    case 'half-rainbow':
      return (
        <svg width={s} height={s * 0.65} viewBox={`0 0 ${s} ${s * 0.65}`}>
          <path d={`M 2 ${s * 0.6} A ${r} ${r} 0 0 1 ${s * 0.4} ${s * 0.1}`} fill="none" stroke="#10b981" strokeWidth={2.5} strokeLinecap="round" />
          <path d={`M ${s * 0.4} ${s * 0.1} A ${r} ${r} 0 0 1 ${s - 2} ${s * 0.6}`} fill="none" stroke="#ef4444" strokeWidth={2.5} strokeLinecap="round" />
        </svg>
      );
    default:
      return <Gauge className="text-current" style={{ width: s, height: s }} />;
  }
}

/** 게이지 미니 프리뷰 (우측 컬럼) */
function GaugeMiniPreview({ panel }: { panel: PanelConfig }) {
  const config = panel.config ?? {};
  const gaugeType = (config.gaugeType as GaugeType) ?? 'simple';
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const unit = (config.unit as string) ?? '%';
  const thresholds = (config.thresholds as { name: string; color: string; from: number; to: number }[]) ?? [];
  const value = 65; // 프리뷰용 고정값

  const ratio = max > min ? (value - min) / (max - min) : 0;
  const activeColor = thresholds.find((t) => value >= t.from && value < t.to)?.color ?? '#3b82f6';

  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-(--color-border-default) bg-(--color-bg-elevated) p-4">
      <span className="mb-2 text-[10px] font-medium text-(--color-text-muted)">
        {GAUGE_TYPE_META.find((m) => m.type === gaugeType)?.label ?? gaugeType}
      </span>
      <svg width={140} height={gaugeType === 'vertical-bar' ? 110 : 90} viewBox={gaugeType === 'vertical-bar' ? '0 0 140 110' : '0 0 140 90'}>
        {gaugeType === 'simple' && (() => {
          const cx = 70, cy = 50, r = 35;
          const circumference = 2 * Math.PI * r;
          const dashLen = circumference * ratio;
          return (
            <g>
              <circle cx={cx} cy={cy} r={r} fill="none" stroke="currentColor" strokeWidth={6} opacity={0.1} />
              <circle cx={cx} cy={cy} r={r} fill="none" stroke={activeColor} strokeWidth={6}
                strokeDasharray={`${dashLen} ${circumference}`}
                strokeLinecap="round" transform={`rotate(-90 ${cx} ${cy})`} />
              <text x={cx} y={cy - 2} textAnchor="middle" className="fill-(--color-text-primary)" fontSize={18} fontWeight={600}>{value}</text>
              <text x={cx} y={cy + 14} textAnchor="middle" className="fill-(--color-text-muted)" fontSize={10}>{unit}</text>
            </g>
          );
        })()}
        {gaugeType === 'half' && (() => {
          const cx = 70, cy = 70, r = 45;
          const halfCirc = Math.PI * r;
          const dashLen = halfCirc * ratio;
          return (
            <g>
              <path d={`M ${cx - r} ${cy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none" stroke="currentColor" strokeWidth={6} opacity={0.1} />
              <path d={`M ${cx - r} ${cy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none" stroke={activeColor} strokeWidth={6}
                strokeDasharray={`${dashLen} ${halfCirc}`} strokeLinecap="round" />
              <text x={cx} y={cy - 10} textAnchor="middle" className="fill-(--color-text-primary)" fontSize={20} fontWeight={600}>{value}</text>
              <text x={cx} y={cy + 4} textAnchor="middle" className="fill-(--color-text-muted)" fontSize={10}>{unit}</text>
            </g>
          );
        })()}
        {gaugeType === 'multi-ring' && (() => {
          const cx = 70, cy = 50;
          const rings = [
            { r: 38, ratio: 0.75, color: '#3b82f6' },
            { r: 28, ratio: 0.55, color: '#10b981' },
            { r: 18, ratio: 0.9, color: '#f59e0b' },
          ];
          return (
            <g>
              {rings.map((ring, i) => {
                const circumference = 2 * Math.PI * ring.r;
                return (
                  <g key={i}>
                    <circle cx={cx} cy={cy} r={ring.r} fill="none" stroke="currentColor" strokeWidth={5} opacity={0.08} />
                    <circle cx={cx} cy={cy} r={ring.r} fill="none" stroke={ring.color} strokeWidth={5}
                      strokeDasharray={`${circumference * ring.ratio} ${circumference}`}
                      strokeLinecap="round" transform={`rotate(-90 ${cx} ${cy})`} />
                  </g>
                );
              })}
            </g>
          );
        })()}
        {(gaugeType === 'needle' || gaugeType === 'needle-rainbow') && (() => {
          const cx = 70, cy = 65, r = 45;
          const angle = -180 + ratio * 180;
          const rad = (angle * Math.PI) / 180;
          const nx = cx + (r - 10) * Math.cos(rad);
          const ny = cy + (r - 10) * Math.sin(rad);
          return (
            <g>
              <path d={`M ${cx - r} ${cy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none"
                stroke={gaugeType === 'needle-rainbow' ? 'url(#rainbow-grad)' : 'currentColor'} strokeWidth={6} opacity={gaugeType === 'needle-rainbow' ? 0.8 : 0.1} />
              {gaugeType === 'needle-rainbow' && (
                <defs>
                  <linearGradient id="rainbow-grad" x1="0%" y1="0%" x2="100%" y2="0%">
                    <stop offset="0%" stopColor="#10b981" />
                    <stop offset="50%" stopColor="#f59e0b" />
                    <stop offset="100%" stopColor="#ef4444" />
                  </linearGradient>
                </defs>
              )}
              <line x1={cx} y1={cy} x2={nx} y2={ny} stroke={activeColor} strokeWidth={2} strokeLinecap="round" />
              <circle cx={cx} cy={cy} r={3} fill={activeColor} />
              <text x={cx} y={cy - 20} textAnchor="middle" className="fill-(--color-text-primary)" fontSize={16} fontWeight={600}>{value}{unit}</text>
            </g>
          );
        })()}
        {gaugeType === 'vertical-bar' && (() => {
          const bx = 55, by = 5, bw = 30, bh = 90;
          const fillH = bh * ratio;
          return (
            <g>
              <rect x={bx} y={by} width={bw} height={bh} rx={4} fill="currentColor" opacity={0.08} />
              <rect x={bx} y={by + bh - fillH} width={bw} height={fillH} rx={4} fill={activeColor} opacity={0.8} />
              <text x={bx + bw / 2} y={by + bh + 14} textAnchor="middle" className="fill-(--color-text-primary)" fontSize={12} fontWeight={600}>{value}{unit}</text>
            </g>
          );
        })()}
        {gaugeType === 'half-rainbow' && (() => {
          const cx = 70, cy = 70, r = 45;
          return (
            <g>
              <defs>
                <linearGradient id="half-rb-grad" x1="0%" y1="0%" x2="100%" y2="0%">
                  <stop offset="0%" stopColor="#10b981" />
                  <stop offset="50%" stopColor="#f59e0b" />
                  <stop offset="100%" stopColor="#ef4444" />
                </linearGradient>
              </defs>
              <path d={`M ${cx - r} ${cy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none" stroke="url(#half-rb-grad)" strokeWidth={6} strokeLinecap="round" />
              <text x={cx} y={cy - 10} textAnchor="middle" className="fill-(--color-text-primary)" fontSize={20} fontWeight={600}>{value}</text>
              <text x={cx} y={cy + 4} textAnchor="middle" className="fill-(--color-text-muted)" fontSize={10}>{unit}</text>
            </g>
          );
        })()}
      </svg>
      <div className="mt-1 flex gap-2">
        <span className="text-[10px] text-(--color-text-muted)">범위: {min} ~ {max}</span>
      </div>
    </div>
  );
}

/** NASA 리모컨 미니 프리뷰 - 클릭으로 악센트 그룹 선택 */
function NasaMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  getSubProp,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (group: string) => void;
  effectiveColor: (group: string) => string | undefined;
  getSubProp: (group: string, prop: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const zoneClass = (group: string, extra?: string) =>
    cn(
      'cursor-pointer transition-all relative',
      selectedGroup === group
        ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15'
        : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10',
      extra,
    );

  const tag = (group: string, label: string) =>
    selectedGroup === group ? (
      <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">
        {label}
      </span>
    ) : null;

  const divider = (
    <div
      className={cn(
        'mx-3 cursor-pointer border-t transition-all',
        selectedGroup === 'borders' ? 'border-blue-500 border-t-2' : 'border-gray-100 dark:border-gray-700',
      )}
      style={selectedGroup !== 'borders' && effectiveColor('borders') ? { borderColor: `${effectiveColor('borders')}30` } : undefined}
      onClick={() => onSelectGroup('borders')}
    />
  );

  const labelBg = getSubProp('labels', 'bg');
  const labelText = getSubProp('labels', 'text') ?? effectiveColor('labels');
  const labelRadius = getSubProp('labels', 'radius');

  return (
    <div
      className={cn(
        'overflow-hidden rounded-2xl text-xs',
        selectedGroup === 'borders' ? 'ring-2 ring-blue-500' : 'ring-1 ring-(--color-border-default)',
      )}
      style={selectedGroup !== 'borders' && effectiveColor('borders') ? { boxShadow: `inset 0 0 0 1px ${effectiveColor('borders')}40` } : undefined}
    >
      {/* 전체 색상 - _base */}
      <div
        className={zoneClass('_base', 'flex items-center gap-1.5 px-3 py-1')}
        onClick={() => onSelectGroup('_base')}
      >
        {tag('_base', '전체 색상')}
        <span
          className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }}
        />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? '기본'}</span>
      </div>

      {/* 헤더: 아이콘+타이틀 | 상태뱃지+전원 — indicators */}
      <div className={zoneClass('indicators')} onClick={() => onSelectGroup('indicators')}>
        {tag('indicators', '상태 표시')}
        <div className="flex items-center justify-between px-3 py-1.5">
          <div className="flex items-center gap-1.5">
            <Snowflake
              className="h-3.5 w-3.5 text-blue-500"
              style={effectiveColor('indicators') ? { color: effectiveColor('indicators')! } : undefined}
            />
            <span className="text-[11px] font-bold text-(--color-text-primary)">living-room</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span
              className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-1.5 py-0.5 text-[9px] font-medium text-blue-500 dark:bg-blue-900/30 dark:text-blue-400"
              style={effectiveColor('indicators') ? { color: effectiveColor('indicators')!, backgroundColor: `${effectiveColor('indicators')}15` } : undefined}
            >
              <span className="h-1 w-1 rounded-full bg-blue-500" style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined} />
              가동 중
            </span>
            <span
              className="flex h-5 w-5 items-center justify-center rounded-md bg-blue-500 text-white"
              style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined}
            >
              <Power className="h-3 w-3" />
            </span>
          </div>
        </div>
      </div>

      {/* 현재 온도 — temperature */}
      <div className={zoneClass('temperature')} onClick={() => onSelectGroup('temperature')}>
        {tag('temperature', '온도 표시')}
        <div className="flex flex-col items-center py-2">
          <div className="flex items-end">
            <span
              className="text-3xl font-light text-blue-600"
              style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined}
            >23.7</span>
            <span
              className="text-sm text-blue-600"
              style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined}
            >°C</span>
          </div>
          <span
            className="text-[9px] text-blue-300"
            style={effectiveColor('temperature') ? { color: `${effectiveColor('temperature')}60` } : undefined}
          >현재 온도</span>
        </div>
        {/* 설정 온도 */}
        <div className="flex items-center justify-center gap-1.5 pb-2">
          <Thermometer className="h-3 w-3 text-(--color-text-muted)" style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined} />
          <span className="flex h-4.5 w-4.5 items-center justify-center rounded bg-(--color-bg-elevated)"><Minus className="h-2.5 w-2.5 text-(--color-text-muted)" /></span>
          <span className="text-[10px] font-semibold text-(--color-text-primary)">설정 24°C</span>
          <span className="flex h-4.5 w-4.5 items-center justify-center rounded bg-(--color-bg-elevated)"><Plus className="h-2.5 w-2.5 text-(--color-text-muted)" /></span>
        </div>
      </div>

      {divider}

      {/* 모드 선택 (5버튼) — labels */}
      <div className={zoneClass('labels')} onClick={() => onSelectGroup('labels')}>
        {tag('labels', '모드/라벨')}
        <div className="flex gap-1 px-3 py-2">
          {[
            { label: '냉방', icon: <Snowflake className="h-3 w-3" />, active: true },
            { label: '난방', active: false },
            { label: '자동', active: false },
            { label: '제습', active: false },
            { label: '팬', active: false },
          ].map(({ label, icon, active }) => (
            <span
              key={label}
              className={cn(
                'flex flex-1 flex-col items-center justify-center gap-0.5 py-1 text-[8px] font-medium',
                labelRadius == null && 'rounded-lg',
                active && !labelBg && !labelText
                  ? 'bg-blue-600 text-white'
                  : active ? 'text-white' : 'text-(--color-text-muted) ring-1 ring-(--color-border-default)',
              )}
              style={{
                ...(active && labelBg ? { backgroundColor: labelBg } : {}),
                ...(active && labelText ? { color: labelText } : {}),
                ...(labelRadius != null ? { borderRadius: `${labelRadius}px` } : {}),
              }}
            >
              {icon}
              {label}
            </span>
          ))}
        </div>
      </div>

      {/* 풍량 — controls */}
      <div className={zoneClass('controls')} onClick={() => onSelectGroup('controls')}>
        {tag('controls', '제어 버튼')}
        <div className="flex items-center gap-1.5 px-3 py-1.5">
          <Fan
            className="h-3 w-3 shrink-0 text-blue-600"
            style={effectiveColor('controls') ? { color: effectiveColor('controls')! } : undefined}
          />
          <span
            className="text-[10px] font-semibold text-blue-600"
            style={effectiveColor('controls') ? { color: effectiveColor('controls')! } : undefined}
          >풍량</span>
          {['자동', '약', '중', '강'].map((s, i) => (
            <span
              key={s}
              className={cn(
                'flex-1 rounded-md py-0.5 text-center text-[9px] font-medium',
                i === 0
                  ? 'bg-blue-50 text-blue-600 ring-1 ring-blue-500 dark:bg-blue-900/30'
                  : 'text-(--color-text-muted) ring-1 ring-(--color-border-default)',
              )}
              style={i === 0 && effectiveColor('controls') ? { color: effectiveColor('controls')!, borderColor: effectiveColor('controls')!, backgroundColor: `${effectiveColor('controls')}10` } : undefined}
            >
              {s}
            </span>
          ))}
        </div>
      </div>

      {divider}

      {/* 하단: 스윙 + 필터 — indicators */}
      <div className={zoneClass('indicators', 'rounded-b-2xl')} onClick={() => onSelectGroup('indicators')}>
        <div className="flex items-center gap-3 px-3 py-1.5">
          <span
            className="flex items-center gap-0.5 text-[10px] text-(--color-text-muted)"
            style={effectiveColor('indicators') ? { color: effectiveColor('indicators')! } : undefined}
          >
            <ArrowUpDown className="h-3 w-3" />
            스윙 ON
          </span>
          <span className="flex items-center gap-0.5 text-[10px] text-amber-500">
            필터 정상
          </span>
        </div>
      </div>
    </div>
  );
}

/** 리스트 패널 미니 프리뷰 (flows / agents / devices) */
function ListMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
  variant,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
  variant: 'flows' | 'agents' | 'devices';
}) {
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const titles = { flows: '플로우 현황', agents: '에이전트 현황', devices: '디바이스 목록' };
  const badgeLabels = variant === 'flows'
    ? [{ l: '실행중 3', c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }, { l: '중지 2', c: 'bg-gray-100 text-gray-500 dark:bg-gray-700/30 dark:text-gray-400' }]
    : variant === 'agents'
    ? [{ l: '전체 5', c: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' }, { l: '활성 3', c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }]
    : [{ l: '전체 8', c: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' }, { l: '온라인 6', c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }];

  return (
    <div className="overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', '전체 색상')}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? '기본'}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', '타이틀')}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {titles[variant]}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 요약 배지 */}
      <div className={zone('badges', 'flex gap-1.5 px-3 py-2')} onClick={() => onSelectGroup('badges')}>
        {tag('badges', '배지')}
        {badgeLabels.map((b) => (
          <span key={b.l} className={cn('rounded-full px-1.5 py-0.5 text-[10px] font-medium', b.c)}
            style={effectiveColor('badges') ? { backgroundColor: `${effectiveColor('badges')}20`, color: effectiveColor('badges')! } : undefined}>
            {b.l}
          </span>
        ))}
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 테이블 헤더 */}
      <div className={zone('table', 'px-3 py-2')} onClick={() => onSelectGroup('table')}>
        {tag('table', '헤더')}
        <div className="flex gap-4 text-[10px] font-medium uppercase tracking-wider text-(--color-text-muted)"
          style={effectiveColor('table') ? { color: effectiveColor('table')! } : undefined}>
          <span className="flex-1">이름</span><span>상태</span><span>업데이트</span>
        </div>
      </div>
      {/* 더미 행 */}
      <div className="border-t border-gray-100 px-3 py-1.5 dark:border-gray-700">
        <div className="flex gap-4 text-[10px] text-(--color-text-muted)">
          <span className="flex-1 text-blue-500">sample-1</span><span>●</span><span>2분 전</span>
        </div>
      </div>
      <div className="border-t border-gray-100 px-3 py-1.5 dark:border-gray-700">
        <div className="flex gap-4 text-[10px] text-(--color-text-muted)">
          <span className="flex-1 text-blue-500">sample-2</span><span>○</span><span>5분 전</span>
        </div>
      </div>
    </div>
  );
}

/** 리소스 패널 미니 프리뷰 */
/** 속성 그리드 미니 프리뷰 */
function GridMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
  gridCols,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
  gridCols: number;
}) {
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const items = [
    { key: 'power', label: '전원', value: 'ON' },
    { key: 'mode', label: '운전 모드', value: 'cooling' },
    { key: 'target_temp', label: '설정 온도', value: '24°C' },
    { key: 'current_temp', label: '현재 온도', value: '25.5°C' },
    { key: 'fan_speed', label: '풍량', value: 'auto' },
    { key: 'valve_open', label: '밸브 개도', value: 'ON' },
  ];

  const cols = Math.min(gridCols, 3);
  const colClass = cols === 1 ? 'grid-cols-1' : cols === 2 ? 'grid-cols-2' : 'grid-cols-3';

  return (
    <div className="overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', '전체 색상')}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? '기본'}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 인디케이터 + 라벨 */}
      <div className={zone('labels', 'flex items-center gap-2 px-3 py-2')} onClick={() => onSelectGroup('labels')}>
        {tag('labels', '라벨')}
        <span className="h-2 w-2 rounded-full bg-green-500" style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined} />
        <span className="text-sm font-medium text-(--color-text-primary)" style={effectiveColor('labels') ? { color: effectiveColor('labels')! } : undefined}>
          속성 그리드
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 속성 카드 그리드 */}
      <div className={cn('grid gap-2 p-3', colClass)}>
        {items.slice(0, Math.min(items.length, cols * 2)).map((item) => (
          <div
            key={item.key}
            className={zone('borders', 'rounded-lg border border-(--color-border-default) px-2 py-1.5')}
            onClick={() => onSelectGroup('borders')}
            style={effectiveColor('borders') ? { borderColor: `${effectiveColor('borders')}30` } : undefined}
          >
            {tag('borders', '테두리')}
            <p className="text-[10px] text-(--color-text-muted)" style={effectiveColor('labels') ? { color: effectiveColor('labels')! } : undefined}>
              {item.label}
            </p>
            <p className="mt-0.5 text-xs font-medium text-(--color-text-primary)">{item.value}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function ResourceMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const metrics = [
    { key: 'cpu', label: 'CPU', value: '45.2%', defaultColor: '#3b82f6' },
    { key: 'memory', label: 'MEM', value: '62.8%', defaultColor: '#8b5cf6' },
    { key: 'throughput', label: 'MSG/S', value: '1,234', defaultColor: '#10b981' },
    { key: 'errorRate', label: 'ERR', value: '0.3%', defaultColor: '#ef4444' },
  ];

  return (
    <div className="overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', '전체 색상')}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? '기본'}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', '타이틀')}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          프로세스 리소스
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 메트릭 카드 - 각 카드가 독립 악센트 그룹 */}
      <div className="grid grid-cols-2 gap-2 p-3">
        {metrics.map((m) => {
          const cardColor = effectiveColor(m.key) ?? m.defaultColor;
          return (
            <div
              key={m.key}
              className={zone(m.key, 'rounded-md border border-(--color-border-default) p-2 text-center')}
              onClick={() => onSelectGroup(m.key)}
            >
              {tag(m.key, m.label)}
              <div className="mb-1 inline-flex items-center gap-1">
                <span className="text-[10px] text-(--color-text-muted)" style={effectiveColor(m.key) ? { color: cardColor } : undefined}>{m.label}</span>
              </div>
              <div className="text-base font-bold">
                <span style={{ color: cardColor }}>{m.value}</span>
              </div>
              <div className="mt-1 flex items-end gap-px h-4">
                {[3,5,4,6,8,7,5,6].map((h, i) => (
                  <div key={i} className="flex-1 rounded-sm" style={{ height: `${h * 2}px`, backgroundColor: cardColor, opacity: 0.6 }} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** 로그 패널 미니 프리뷰 */
function LogMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const rows = [
    { time: '14:23:01', level: 'INFO', lvCls: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400', src: 'mqtt', msg: 'connected to broker' },
    { time: '14:23:05', level: 'WARN', lvCls: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400', src: 'flow', msg: 'retry attempt 3' },
    { time: '14:23:08', level: 'ERR', lvCls: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400', src: 'nasa', msg: 'timeout on device A1' },
  ];

  return (
    <div className="overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', '전체 색상')}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? '기본'}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', '타이틀')}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          시스템 로그
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 로그 행 */}
      <div className="bg-(--color-bg-sunken) font-mono">
        {rows.map((r, i) => (
          <div key={i} className="flex items-center gap-1.5 border-b border-(--color-border-subtle) px-2 py-1">
            <span className={zone('timestamp', 'shrink-0 text-[9px]')} onClick={(e) => { e.stopPropagation(); onSelectGroup('timestamp'); }}>
              {i === 0 && tag('timestamp', '시간')}
              <span style={effectiveColor('timestamp') ? { color: effectiveColor('timestamp')! } : undefined} className="text-(--color-text-muted)">{r.time}</span>
            </span>
            <span className={zone('levels', 'shrink-0')} onClick={(e) => { e.stopPropagation(); onSelectGroup('levels'); }}>
              {i === 0 && tag('levels', '레벨')}
              <span className={cn('rounded px-1 py-px text-[8px] font-semibold', r.lvCls)}
                style={effectiveColor('levels') ? { backgroundColor: `${effectiveColor('levels')}20`, color: effectiveColor('levels')! } : undefined}>
                {r.level}
              </span>
            </span>
            <span className={zone('source', 'shrink-0')} onClick={(e) => { e.stopPropagation(); onSelectGroup('source'); }}>
              {i === 0 && tag('source', '소스')}
              <span className="text-[9px] text-purple-600 dark:text-purple-400" style={effectiveColor('source') ? { color: effectiveColor('source')! } : undefined}>{r.src}</span>
            </span>
            <span className="flex-1 truncate text-[9px] text-(--color-text-primary)">{r.msg}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
