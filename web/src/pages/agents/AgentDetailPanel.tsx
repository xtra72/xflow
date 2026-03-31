// 에이전트 상세 패널.
// 행 확장 시 표시되며, 통계 탭과 설정 탭으로 구성된다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, ChevronDown, ChevronRight, HardDrive, Lock, Pencil, Plus, RefreshCw, Save, Server, Trash2, X } from 'lucide-react';

import { useQueries, useQueryClient } from '@tanstack/react-query';

import { useAgent, useAgentStats, useConfigureAgent, useExecAgent } from '@/hooks/useAgent';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useFlows } from '@/hooks/useFlow';
import * as flowService from '@/services/api/flowService';
import { cn } from '@/lib/utils/cn';
import { getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import { getAgentConfigSchema } from '@/config/agentSchemas';
import type { ConfigSchema } from '@/types/node';
import { DynamicForm } from '@/components/property/DynamicForm';
import { FormField } from '@/components/property/FormField';
import DeviceStatusBadge from '@/pages/devices/DeviceStatusBadge';
import {
  getLogLevels,
  setComponentLogLevel,
  resetComponentLogLevel,
} from '@/services/api/monitorService';
import { useUIStore } from '@/stores/uiStore';

interface AgentDetailPanelProps {
  agentId: string;
  agentType: string;
}

type Tab = 'stats' | 'config' | 'devices' | 'topics' | 'store';

/** 통계 카드 항목 */
function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
      <p className="text-xs font-medium text-(--color-text-muted)">{label}</p>
      <p className="mt-1 text-lg font-semibold text-(--color-text-primary)">{value}</p>
    </div>
  );
}

/** 디바이스 탭을 표시하지 않는 에이전트 타입 */
const NO_DEVICES_TAB = new Set(['mqtt-client', 'logger', 'http', 'http-sender', 'influxdb', 'tsdb', 'store']);

/** 토픽 탭을 표시하는 에이전트 타입 */
const HAS_TOPICS_TAB = new Set(['mqtt-client']);

/** 저장소 탭을 표시하는 에이전트 타입 */
const HAS_STORE_TAB = new Set(['store']);

export default function AgentDetailPanel({ agentId, agentType }: AgentDetailPanelProps) {
  const showDevices = !NO_DEVICES_TAB.has(agentType);
  const showTopics = HAS_TOPICS_TAB.has(agentType);
  const showStore = HAS_STORE_TAB.has(agentType);

  const [tab, setTab] = useState<Tab>(showStore ? 'store' : 'stats');

  return (
    <div>
      {/* 탭 헤더 */}
      <div className="flex border-b border-(--color-border-default) px-4">
        <TabButton label="통계" active={tab === 'stats'} onClick={() => setTab('stats')} />
        <TabButton label="설정" active={tab === 'config'} onClick={() => setTab('config')} />
        {showTopics && <TabButton label="토픽" active={tab === 'topics'} onClick={() => setTab('topics')} />}
        {showStore && <TabButton label="저장소" active={tab === 'store'} onClick={() => setTab('store')} />}
        {showDevices && <TabButton label="디바이스" active={tab === 'devices'} onClick={() => setTab('devices')} />}
      </div>

      {/* 탭 컨텐츠 */}
      {tab === 'stats' && <StatsTab agentId={agentId} />}
      {tab === 'config' && <ConfigTab agentId={agentId} agentType={agentType} />}
      {tab === 'topics' && showTopics && <TopicsTab agentId={agentId} />}
      {tab === 'store' && showStore && <StoreTab agentId={agentId} />}
      {tab === 'devices' && showDevices && <DevicesTab agentId={agentId} agentType={agentType} />}
    </div>
  );
}

// ---- 탭 버튼 ----

function TabButton({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'px-4 py-2 text-sm font-medium transition-colors',
        active
          ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
          : 'text-(--color-text-muted) hover:text-(--color-text-secondary)',
      )}
    >
      {label}
    </button>
  );
}

// ---- 통계 탭 ----

function StatsTab({ agentId }: { agentId: string }) {
  const { data: stats, isLoading } = useAgentStats(agentId);
  const { data: agentDetail } = useAgent(agentId);

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        통계 데이터를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatCard label="수신 메시지" value={stats.messages_in.toLocaleString()} />
        <StatCard label="송신 메시지" value={stats.messages_out.toLocaleString()} />
        <StatCard label="에러 수" value={stats.error_count.toLocaleString()} />
        <StatCard label="업타임" value={stats.uptime ?? '-'} />
      </div>

      {/* 연결된 노드 */}
      <LinkedNodesSection agentId={agentId} agentName={agentDetail?.name} />
    </div>
  );
}

// ---- 연결 노드 섹션 ----

/** 에이전트에 연결된 플로우 노드 목록 */
function LinkedNodesSection({ agentId, agentName }: { agentId: string; agentName?: string }) {
  const { data: flowsData } = useFlows();
  const flows = flowsData?.data ?? [];

  // running 또는 loaded 상태인 플로우의 노드를 병렬 조회
  const activeFlows = useMemo(
    () => flows.filter((f) => f.status === 'running' || f.status === 'loaded'),
    [flows],
  );

  const nodeQueries = useQueries({
    queries: activeFlows.map((flow) => ({
      queryKey: ['flows', flow.id, 'nodes', 'linked', agentId],
      queryFn: () => flowService.getFlowNodes(flow.id),
      staleTime: 30_000,
      enabled: activeFlows.length > 0,
    })),
  });

  // agent_ref 또는 agent_id가 매칭되는 노드 필터링
  const linkedNodes = useMemo(() => {
    const result: { flowId: string; flowName: string; nodeId: string; nodeName: string; nodeType: string }[] = [];
    for (let i = 0; i < activeFlows.length; i++) {
      const flow = activeFlows[i]!;
      const nodes = nodeQueries[i]?.data;
      if (!nodes) continue;

      for (const node of nodes) {
        const cfg = node.config ?? {};
        const ref = cfg.agent_ref ?? cfg.agent_id;
        if (ref === agentId || ref === agentName) {
          result.push({
            flowId: flow.id,
            flowName: flow.name,
            nodeId: node.node_id,
            nodeName: node.name,
            nodeType: node.type,
          });
        }
      }
    }
    return result;
  }, [activeFlows, nodeQueries, agentId, agentName]);

  if (linkedNodes.length === 0) return null;

  return (
    <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
      <p className="mb-2 text-xs font-medium text-(--color-text-muted)">
        연결된 노드 ({linkedNodes.length})
      </p>
      <div className="space-y-1">
        {linkedNodes.map((n) => (
          <div
            key={`${n.flowId}-${n.nodeId}`}
            className="flex items-center justify-between rounded px-2 py-1 text-xs text-(--color-text-secondary)"
          >
            <span className="font-medium">{n.nodeName || n.nodeId}</span>
            <span className="text-(--color-text-muted)">
              {n.nodeType} · {n.flowName}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- 2열 설정 레이아웃 (NASA, Logger 공용) ----

/** 에이전트 타입별 좌측 컬럼 필드 및 컬럼 라벨 */
const TWO_COL_CONFIG: Record<string, { left: Set<string>; leftLabel: string; rightLabel: string }> = {
  'mqtt-client': {
    left: new Set(['broker', 'client_id', 'username', 'password', 'keep_alive_sec', 'connect_timeout_sec']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  'modbus-tcp': {
    left: new Set(['mode', 'read_mode', 'reconnect_interval', 'request_timeout', 'max_retries']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  'modbus-tcp-server': {
    left: new Set(['listen_address', 'listen_port']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  http: {
    left: new Set(['listen_addr', 'path', 'method']),
    leftLabel: '수신',
    rightLabel: '운영',
  },
  'http-sender': {
    left: new Set(['url', 'method', 'content_type']),
    leftLabel: '전송',
    rightLabel: '운영',
  },
  influxdb: {
    left: new Set(['url', 'token', 'org', 'bucket', 'version']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  logger: {
    left: new Set(['output', 'output_path', 'format', 'max_size', 'max_age', 'max_backups', 'compress']),
    leftLabel: '출력',
    rightLabel: '운영',
  },
  'samsung-nasa': {
    left: new Set(['transport_type', 'serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'tcp_addr']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  lgap: {
    left: new Set(['transport_type', 'serial_port', 'baud_rate', 'connect_timeout', 'read_timeout']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  lgcp: {
    left: new Set(['serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'read_timeout']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
};

function TwoColumnConfigLayout({
  data, schema, onChange, readOnly, agentType, logLevel,
}: {
  nodeId: string;
  data: Record<string, unknown>;
  schema: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
  agentType: string;
  logLevel?: { agentId: string; value: string; updating: boolean; onChangeLevel: (v: string) => void };
}) {
  const colConfig = TWO_COL_CONFIG[agentType];
  if (!colConfig) return null;

  const leftFields = schema.fields.filter((f) => colConfig.left.has(f.name));
  const rightFields = schema.fields.filter((f) => !colConfig.left.has(f.name));

  const filterVisible = (fields: typeof schema.fields) =>
    fields.filter((f) => !f.visibleWhen || data[f.visibleWhen.field] === f.visibleWhen.value);

  const handleChange = (fieldName: string, value: unknown) => {
    onChange({ ...data, [fieldName]: value });
  };

  return (
    <div className="grid grid-cols-2 gap-4">
      {/* 좌측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{colConfig.leftLabel}</h4>
        {filterVisible(leftFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
          />
        ))}
      </div>
      {/* 우측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{colConfig.rightLabel}</h4>
        {filterVisible(rightFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
          />
        ))}
        {logLevel && (
          <div className="space-y-1">
            <label
              htmlFor={`agent-log-${logLevel.agentId}`}
              className="block text-xs font-medium text-(--color-text-secondary)"
            >
              로그 레벨
            </label>
            <select
              id={`agent-log-${logLevel.agentId}`}
              value={logLevel.value}
              onChange={(e) => logLevel.onChangeLevel(e.target.value)}
              disabled={logLevel.updating}
              className={cn(
                'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
                'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
                'bg-(--color-bg-surface) text-(--color-text-primary)',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              <option value="">기본값</option>
              <option value="debug">DEBUG</option>
              <option value="info">INFO</option>
              <option value="warn">WARN</option>
              <option value="error">ERROR</option>
            </select>
          </div>
        )}
      </div>
    </div>
  );
}

// ---- 설정 탭 ----

function ConfigTab({ agentId, agentType }: { agentId: string; agentType: string }) {
  const { data: agent, isLoading } = useAgent(agentId);
  const configureAgent = useConfigureAgent();
  const addNotification = useUIStore((s) => s.addNotification);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Record<string, unknown>>({});

  // 컴포넌트별 로그 레벨 상태
  const [componentLogLevel, setComponentLogLevel_] = useState<string>('');
  const [isLogLevelUpdating, setIsLogLevelUpdating] = useState(false);

  // 컴포넌트 이름: observe 패키지에서 "agent.<type>.<name>" 형식으로 등록한다.
  const agentName = agent?.name ?? '';
  const componentKey = agentName ? `agent.${agentType}.${agentName}` : '';

  // 마운트 시 현재 에이전트의 로그 레벨 로드
  useEffect(() => {
    if (!componentKey) return;
    let cancelled = false;
    getLogLevels()
      .then((info) => {
        if (cancelled) return;
        const level = info.components[componentKey];
        setComponentLogLevel_(level ?? '');
      })
      .catch(() => {
        // 로드 실패 시 무시 (기본값 유지)
      });
    return () => {
      cancelled = true;
    };
  }, [componentKey]);

  /** 로그 레벨 변경 핸들러 */
  async function handleLogLevelChange(value: string) {
    const componentName = componentKey;
    setIsLogLevelUpdating(true);
    try {
      if (value === '') {
        await resetComponentLogLevel(componentName);
        setComponentLogLevel_('');
        addNotification({ type: 'success', message: '로그 레벨이 기본값으로 리셋되었습니다' });
      } else {
        await setComponentLogLevel(componentName, value);
        setComponentLogLevel_(value);
        addNotification({ type: 'success', message: `로그 레벨이 "${value.toUpperCase()}"로 변경되었습니다` });
      }
    } catch {
      addNotification({ type: 'error', message: '로그 레벨 변경에 실패했습니다' });
    } finally {
      setIsLogLevelUpdating(false);
    }
  }

  const config = agent?.config ?? {};
  const schema = getAgentConfigSchema(agentType);

  // 에이전트 데이터 로드 시 드래프트 초기화
  useEffect(() => {
    if (agent?.config) {
      setDraft(agent.config);
    }
  }, [agent?.config]);

  const handleEdit = useCallback(() => {
    setDraft(config);
    setEditing(true);
  }, [config]);

  const handleCancel = useCallback(() => {
    setDraft(config);
    setEditing(false);
  }, [config]);

  const handleSave = useCallback(async () => {
    try {
      await configureAgent.mutateAsync({ id: agentId, config: draft });
      setEditing(false);

      // transport 설정 변경 감지 시 재시작 알림
      const transportKeys = ['transport_type', 'serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'tcp_addr', 'tcp_address'];
      const changed = transportKeys.some((k) => String(config[k] ?? '') !== String(draft[k] ?? ''));
      if (changed) {
        addNotification({ type: 'info', message: '연결 설정이 변경되어 에이전트가 재시작됩니다' });
      } else {
        addNotification({ type: 'success', message: '설정이 저장되었습니다' });
      }
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  }, [agentId, draft, config, configureAgent, addNotification]);

  if (isLoading) {
    return (
      <div className="space-y-3 p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        에이전트 정보를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="p-4">
      {/* 액션 버튼 */}
      <div className="mb-3 flex items-center justify-end gap-2">
        {editing ? (
          <>
            <button
              type="button"
              onClick={handleCancel}
              disabled={configureAgent.isPending}
              className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              <X className="h-3.5 w-3.5" />
              취소
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={configureAgent.isPending}
              className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Save className="h-3.5 w-3.5" />
              {configureAgent.isPending ? '저장 중...' : '저장'}
            </button>
          </>
        ) : (
          <button
            type="button"
            onClick={handleEdit}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Pencil className="h-3.5 w-3.5" />
            편집
          </button>
        )}
      </div>

      {/* 에러 메시지 */}
      {configureAgent.isError && (
        <p className="mb-3 text-xs text-red-500 dark:text-red-400">
          설정 저장에 실패했습니다. 다시 시도해주세요.
        </p>
      )}

      {/* 연결 방식 변경 경고 (NASA) */}
      {editing && agentType === 'samsung-nasa' && config.transport_type !== draft.transport_type && (
        <div className="mb-3 flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-300">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          연결 방식 변경은 에이전트 재시작 후 적용됩니다.
        </div>
      )}

      {/* 즉시 적용 안내 (NASA 편집 모드) */}
      {editing && agentType === 'samsung-nasa' && config.transport_type === draft.transport_type && (
        <p className="mb-3 text-xs text-(--color-text-muted)">
          상태 확인 요청 간격, 부저, 알람 설정은 저장 즉시 적용됩니다.
        </p>
      )}

      {/* 설정 폼 */}
      {agentType in TWO_COL_CONFIG && schema ? (
        <TwoColumnConfigLayout
          nodeId={agentId}
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          readOnly={!editing}
          agentType={agentType}
          logLevel={{
            agentId,
            value: componentLogLevel,
            updating: isLogLevelUpdating,
            onChangeLevel: handleLogLevelChange,
          }}
        />
      ) : (
        <DynamicForm
          nodeId={agentId}
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          readOnly={!editing}
        />
      )}

      {/* 로그 레벨 설정 (NASA는 2열 레이아웃에 포함) */}
      {!(agentType in TWO_COL_CONFIG) && <div className="mt-4 rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
        <label
          htmlFor={`agent-log-level-${agentId}`}
          className="mb-1 block text-xs font-medium text-(--color-text-muted)"
        >
          로그 레벨
        </label>
        <select
          id={`agent-log-level-${agentId}`}
          value={componentLogLevel}
          onChange={(e) => handleLogLevelChange(e.target.value)}
          disabled={isLogLevelUpdating}
          className={cn(
            'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'bg-(--color-bg-surface) text-(--color-text-primary)',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <option value="">기본값(Default)</option>
          <option value="debug">DEBUG</option>
          <option value="info">INFO</option>
          <option value="warn">WARN</option>
          <option value="error">ERROR</option>
        </select>
      </div>}
    </div>
  );
}

// ---- Modbus TCP Server 디바이스 섹션 ----

/** list_devices 응답 내 개별 디바이스 */
interface ModbusDevice {
  unit_id: number;
  name: string;
  register_counts: {
    coils: number;
    discrete_inputs: number;
    holding_registers: number;
    input_registers: number;
  };
  status: string;
  stats: {
    read_count: number;
    write_count: number;
    error_count: number;
  };
}

/** get_device_status 응답 */
interface ModbusDeviceDetail {
  unit_id: number;
  name: string;
  register_counts: Record<string, number>;
  register_map: {
    coils?: Record<string, boolean>;
    discrete_inputs?: Record<string, boolean>;
    holding_registers?: Record<string, number>;
    input_registers?: Record<string, number>;
  };
  stats: {
    read_count: number;
    write_count: number;
    error_count: number;
    last_access?: string;
  };
  created_at?: string;
}

/** 레지스터 영역 라벨 (Modbus 기능 코드 포함) */
const REGISTER_AREA_LABELS: Record<string, string> = {
  coils: 'Coils (FC01/05)',
  discrete_inputs: 'Discrete Inputs (FC02)',
  holding_registers: 'Holding Registers (FC03/06)',
  input_registers: 'Input Registers (FC04)',
};

/** 레지스터 영역 순서 */
const REGISTER_AREA_ORDER = ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'] as const;

/** 레지스터 맵 테이블 컴포넌트 */
function RegisterMapTable({ registerMap }: { registerMap: ModbusDeviceDetail['register_map'] }) {
  const [expandedAreas, setExpandedAreas] = useState<Record<string, boolean>>({});

  const toggleArea = (area: string) => {
    setExpandedAreas((prev) => ({ ...prev, [area]: !prev[area] }));
  };

  // boolean 영역 (coils, discrete_inputs)
  const isBooleanArea = (area: string) => area === 'coils' || area === 'discrete_inputs';

  // 주소 정렬 (숫자 기준)
  const sortedEntries = (data: Record<string, unknown>) =>
    Object.entries(data).sort(([a], [b]) => Number(a) - Number(b));

  // 표시할 영역만 필터 (데이터가 있는 것만)
  const visibleAreas = REGISTER_AREA_ORDER.filter((area) => {
    const data = registerMap[area];
    return data && Object.keys(data).length > 0;
  });

  if (visibleAreas.length === 0) return null;

  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-(--color-text-muted)">레지스터 맵</p>
      {visibleAreas.map((area) => {
        const data = registerMap[area]!;
        const entries = sortedEntries(data);
        const isExpanded = expandedAreas[area] ?? false;
        const label = REGISTER_AREA_LABELS[area] ?? area;

        return (
          <div key={area} className="rounded border border-(--color-border-default)">
            {/* 영역 헤더 (클릭으로 접기/펼치기) */}
            <button
              type="button"
              onClick={() => toggleArea(area)}
              className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              {isExpanded ? (
                <ChevronDown className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              )}
              <span>{label}</span>
              <span className="ml-auto rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] font-normal text-(--color-text-muted)">
                {entries.length}
              </span>
            </button>

            {/* 레지스터 테이블 */}
            {isExpanded && (
              <div className="max-h-64 overflow-y-auto border-t border-(--color-border-default)">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="bg-(--color-bg-primary) text-left text-(--color-text-muted)">
                      <th className="px-2 py-1 font-medium">주소</th>
                      {isBooleanArea(area) ? (
                        <th className="px-2 py-1 font-medium">값</th>
                      ) : (
                        <>
                          <th className="px-2 py-1 font-medium">Dec</th>
                          <th className="px-2 py-1 font-medium">Hex</th>
                        </>
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {entries.map(([addr, value]) => (
                      <tr key={addr} className="hover:bg-(--color-bg-elevated)">
                        <td className="px-2 py-1 font-mono text-(--color-text-secondary)">{addr}</td>
                        {isBooleanArea(area) ? (
                          <td className="px-2 py-1">
                            <span
                              className={cn(
                                'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
                                value
                                  ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                                  : 'bg-(--color-bg-elevated) text-(--color-text-muted)',
                              )}
                            >
                              {value ? 'ON' : 'OFF'}
                            </span>
                          </td>
                        ) : (
                          <>
                            <td className="px-2 py-1 font-mono text-(--color-text-secondary)">
                              {typeof value === 'number' ? value : '-'}
                            </td>
                            <td className="px-2 py-1 font-mono text-(--color-text-muted)">
                              {typeof value === 'number'
                                ? `0x${value.toString(16).toUpperCase().padStart(4, '0')}`
                                : '-'}
                            </td>
                          </>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ModbusDevicesSection({ agentId }: { agentId: string }) {
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  // 디바이스 목록
  const [devices, setDevices] = useState<ModbusDevice[]>([]);
  const [isLoadingDevices, setIsLoadingDevices] = useState(true);

  // 상세 보기
  const [selectedUnitId, setSelectedUnitId] = useState<number | null>(null);
  const [deviceDetail, setDeviceDetail] = useState<ModbusDeviceDetail | null>(null);
  const [isLoadingDetail, setIsLoadingDetail] = useState(false);

  // 추가 모달
  const [showAddModal, setShowAddModal] = useState(false);
  const [addUnitId, setAddUnitId] = useState('');
  const [addName, setAddName] = useState('');
  const [isAdding, setIsAdding] = useState(false);

  // 레지스터 맵 폼 상태 (영역별 블록 배열, 빈 배열 = 비활성)
  type RegBlock = { start: string; count: string };
  const [addRegAreas, setAddRegAreas] = useState<Record<string, RegBlock[]>>({
    holding_registers: [{ start: '0', count: '100' }],
    input_registers: [],
    coils: [],
    discrete_inputs: [],
  });

  // 삭제 확인
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  // 디바이스 목록 로드
  const fetchDevices = useCallback(() => {
    setIsLoadingDevices(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'list_devices' } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { devices?: ModbusDevice[]; total?: number } };
          const list = result?.result?.devices ?? [];
          setDevices(list);
          setIsLoadingDevices(false);
        },
        onError: () => {
          setIsLoadingDevices(false);
          addNotification({ type: 'error', message: '디바이스 목록을 불러올 수 없습니다' });
        },
      },
    );
  }, [agentId, execAgent, addNotification]);

  useEffect(() => {
    fetchDevices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  // 디바이스 상세 로드
  const handleSelectDevice = useCallback((unitId: number) => {
    if (selectedUnitId === unitId) {
      setSelectedUnitId(null);
      setDeviceDetail(null);
      return;
    }
    setSelectedUnitId(unitId);
    setIsLoadingDetail(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'get_device_status', params: { unit_id: unitId } } },
      {
        onSuccess: (res) => {
          const result = res as { result?: ModbusDeviceDetail };
          setDeviceDetail(result?.result ?? null);
          setIsLoadingDetail(false);
        },
        onError: () => {
          setDeviceDetail(null);
          setIsLoadingDetail(false);
        },
      },
    );
  }, [agentId, selectedUnitId, execAgent]);

  // 디바이스 추가
  const handleAddDevice = useCallback(() => {
    const unitId = parseInt(addUnitId, 10);
    if (isNaN(unitId) || unitId < 1 || unitId > 247) {
      addNotification({ type: 'error', message: '유닛 ID는 1~247 범위여야 합니다' });
      return;
    }

    const params: Record<string, unknown> = { unit_id: unitId };
    if (addName.trim()) params.name = addName.trim();

    // 구조화된 레지스터 맵 조립 (영역당 다중 블록 지원)
    const regMap: Record<string, unknown> = {};
    for (const [area, blocks] of Object.entries(addRegAreas)) {
      if (!blocks || blocks.length === 0) continue;
      const parsed: { start_address: number; count: number }[] = [];
      for (const blk of blocks) {
        const start = parseInt(blk.start, 10);
        const cnt = parseInt(blk.count, 10);
        if (isNaN(start) || isNaN(cnt) || cnt <= 0) {
          addNotification({ type: 'error', message: `${REGISTER_AREA_LABELS[area] ?? area}: 올바른 주소와 개수를 입력하세요` });
          return;
        }
        parsed.push({ start_address: start, count: cnt });
      }
      // 블록 1개면 객체, 2개 이상이면 배열 (백엔드 호환)
      regMap[area] = parsed.length === 1 ? parsed[0] : parsed;
    }
    if (Object.keys(regMap).length === 0) {
      addNotification({ type: 'error', message: '최소 하나의 레지스터 영역을 활성화하세요' });
      return;
    }
    params.register_map = regMap;

    setIsAdding(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'add_device', params } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { success?: boolean; error?: string } };
          if (result?.result?.success === false) {
            addNotification({ type: 'error', message: result.result.error ?? '디바이스 추가 실패' });
          } else {
            addNotification({ type: 'success', message: `디바이스 (Unit ${unitId})가 추가되었습니다` });
            setShowAddModal(false);
            setAddUnitId('');
            setAddName('');
            setAddRegAreas({
              holding_registers: [{ start: '0', count: '100' }],
              input_registers: [],
              coils: [],
              discrete_inputs: [],
            });
            fetchDevices();
          }
          setIsAdding(false);
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          setIsAdding(false);
        },
      },
    );
  }, [agentId, addUnitId, addName, addRegAreas, execAgent, addNotification, fetchDevices]);

  // 디바이스 삭제
  const handleDeleteDevice = useCallback((unitId: number) => {
    execAgent.mutate(
      { id: agentId, req: { command: 'remove_device', params: { unit_id: unitId } } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { success?: boolean; error?: string } };
          if (result?.result?.success === false) {
            addNotification({ type: 'error', message: result.result.error ?? '디바이스 제거 실패' });
          } else {
            addNotification({ type: 'success', message: `디바이스 (Unit ${unitId})가 제거되었습니다` });
            if (selectedUnitId === unitId) {
              setSelectedUnitId(null);
              setDeviceDetail(null);
            }
            fetchDevices();
          }
          setDeleteTarget(null);
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 제거 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          setDeleteTarget(null);
        },
      },
    );
  }, [agentId, selectedUnitId, execAgent, addNotification, fetchDevices]);

  const canDelete = useMemo(() => devices.length > 1, [devices.length]);

  if (isLoadingDevices) {
    return (
      <div className="grid grid-cols-1 gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-32 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {devices.length}개 디바이스
        </span>
        <button
          type="button"
          onClick={() => setShowAddModal(true)}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-3.5 w-3.5" />
          디바이스 추가
        </button>
      </div>

      {/* 추가 모달 */}
      {showAddModal && (
        <div className="space-y-2 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800 dark:bg-blue-950">
          <div className="text-sm font-medium text-(--color-text-primary)">디바이스 추가</div>
          <div>
            <label htmlFor="modbus-add-unit-id" className="mb-1 block text-xs font-medium text-(--color-text-muted)">
              유닛 ID (1-247) *
            </label>
            <input
              id="modbus-add-unit-id"
              type="number"
              min={1}
              max={247}
              placeholder="1"
              value={addUnitId}
              onChange={(e) => setAddUnitId(e.target.value)}
              className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
            />
          </div>
          <div>
            <label htmlFor="modbus-add-name" className="mb-1 block text-xs font-medium text-(--color-text-muted)">
              이름 (선택)
            </label>
            <input
              id="modbus-add-name"
              type="text"
              placeholder="예: 센서 디바이스 1"
              value={addName}
              onChange={(e) => setAddName(e.target.value)}
              className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
            />
          </div>
          <div>
            <p className="mb-1.5 text-xs font-medium text-(--color-text-muted)">
              레지스터 맵
            </p>
            <div className="space-y-1.5">
              {REGISTER_AREA_ORDER.map((area) => {
                const blocks = addRegAreas[area] ?? [];
                const isActive = blocks.length > 0;
                const label = REGISTER_AREA_LABELS[area] ?? area;
                const defaultCount = area === 'coils' || area === 'discrete_inputs' ? '8' : '100';
                return (
                  <div
                    key={area}
                    className={cn(
                      'rounded-md border p-2 transition-colors',
                      isActive
                        ? 'border-blue-200 bg-blue-50/50 dark:border-blue-800 dark:bg-blue-950/30'
                        : 'border-(--color-border-default) bg-(--color-bg-primary)',
                    )}
                  >
                    <div className="flex items-center justify-between">
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={isActive}
                          onChange={(e) => {
                            setAddRegAreas((prev) => ({
                              ...prev,
                              [area]: e.target.checked ? [{ start: '0', count: defaultCount }] : [],
                            }));
                          }}
                          className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
                        />
                        <span className="text-xs font-medium text-(--color-text-secondary)">
                          {label}
                        </span>
                      </label>
                      {isActive && (
                        <button
                          type="button"
                          onClick={() => {
                            setAddRegAreas((prev) => {
                              const cur = prev[area] ?? [];
                              return { ...prev, [area]: [...cur, { start: '0', count: defaultCount }] };
                            });
                          }}
                          className="text-[10px] font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                        >
                          + 블록 추가
                        </button>
                      )}
                    </div>
                    {isActive && (
                      <div className="mt-1.5 space-y-1 pl-5">
                        {blocks.map((blk, idx) => (
                          <div key={idx} className="flex items-center gap-2">
                            <div className="flex items-center gap-1">
                              <span className="text-[10px] text-(--color-text-muted)">Start:</span>
                              <input
                                type="number"
                                min={0}
                                value={blk.start}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur[idx] = { start: val, count: cur[idx]?.count ?? defaultCount };
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="w-20 rounded border border-(--color-border-strong) px-1.5 py-0.5 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                              />
                            </div>
                            <div className="flex items-center gap-1">
                              <span className="text-[10px] text-(--color-text-muted)">Count:</span>
                              <input
                                type="number"
                                min={1}
                                value={blk.count}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur[idx] = { start: cur[idx]?.start ?? '0', count: val };
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="w-20 rounded border border-(--color-border-strong) px-1.5 py-0.5 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                              />
                            </div>
                            {blocks.length > 1 && (
                              <button
                                type="button"
                                onClick={() => {
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur.splice(idx, 1);
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="rounded p-0.5 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950"
                                title="블록 삭제"
                              >
                                <X className="h-3 w-3" />
                              </button>
                            )}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleAddDevice}
              disabled={!addUnitId.trim() || isAdding}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
            >
              {isAdding ? '추가 중...' : '추가'}
            </button>
            <button
              type="button"
              onClick={() => {
                setShowAddModal(false);
                setAddUnitId('');
                setAddName('');
                setAddRegAreas({
                  holding_registers: [{ start: '0', count: '100' }],
                  input_registers: [],
                  coils: [],
                  discrete_inputs: [],
                });
              }}
              className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              취소
            </button>
          </div>
        </div>
      )}

      {/* 삭제 확인 다이얼로그 */}
      {deleteTarget !== null && (
        <div className="flex items-center gap-3 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-950">
          <AlertTriangle className="h-4 w-4 flex-shrink-0 text-red-500" />
          <div className="flex-1">
            <p className="text-sm text-red-700 dark:text-red-300">
              Unit {deleteTarget} 디바이스를 삭제하시겠습니까?
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => handleDeleteDevice(deleteTarget)}
              className="rounded-md bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700"
            >
              삭제
            </button>
            <button
              type="button"
              onClick={() => setDeleteTarget(null)}
              className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              취소
            </button>
          </div>
        </div>
      )}

      {/* 디바이스 목록 */}
      {devices.length === 0 ? (
        <div className="p-6 text-center">
          <Server className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-(--color-text-muted)">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {devices.map((d) => (
            <div
              key={d.unit_id}
              className={cn(
                'cursor-pointer rounded-lg border bg-(--color-bg-surface) p-3 transition-colors',
                selectedUnitId === d.unit_id
                  ? 'border-blue-400 ring-1 ring-blue-400 dark:border-blue-500'
                  : 'border-(--color-border-default) hover:border-gray-300 dark:hover:border-gray-600',
              )}
              onClick={() => handleSelectDevice(d.unit_id)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  handleSelectDevice(d.unit_id);
                }
              }}
            >
              {/* 카드 헤더 */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="inline-flex h-6 min-w-[1.5rem] items-center justify-center rounded bg-(--color-bg-elevated) px-1.5 text-xs font-bold text-(--color-text-secondary)">
                    {d.unit_id}
                  </span>
                  <span className="text-sm font-medium text-(--color-text-primary)">
                    {d.name || `Device ${d.unit_id}`}
                  </span>
                </div>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
                      d.status === 'active'
                        ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                        : 'bg-(--color-bg-elevated) text-(--color-text-muted)',
                    )}
                  >
                    <span
                      className={cn(
                        'h-1.5 w-1.5 rounded-full',
                        d.status === 'active' ? 'bg-green-500' : 'bg-gray-400',
                      )}
                    />
                    {d.status === 'active' ? '활성' : '비활성'}
                  </span>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteTarget(d.unit_id);
                    }}
                    disabled={!canDelete}
                    className={cn(
                      'rounded p-1 transition-colors',
                      canDelete
                        ? 'text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950'
                        : 'cursor-not-allowed text-gray-300 dark:text-gray-600',
                    )}
                    title={canDelete ? '디바이스 삭제' : '마지막 디바이스는 삭제할 수 없습니다'}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>

              {/* 레지스터 영역 카운트 */}
              <div className="mt-2 grid grid-cols-2 gap-1">
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">Coils:</span> {d.register_counts.coils}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">DI:</span> {d.register_counts.discrete_inputs}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">HR:</span> {d.register_counts.holding_registers}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">IR:</span> {d.register_counts.input_registers}
                </div>
              </div>

              {/* 통계 요약 */}
              <div className="mt-2 flex items-center gap-3 border-t border-(--color-border-default) pt-2">
                <span className="flex items-center gap-1 text-[10px] text-(--color-text-muted)">
                  <Activity className="h-3 w-3" />
                  R:{d.stats.read_count} W:{d.stats.write_count}
                </span>
                {d.stats.error_count > 0 && (
                  <span className="text-[10px] text-red-500">
                    E:{d.stats.error_count}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 디바이스 상세 보기 */}
      {selectedUnitId !== null && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-4">
          <div className="mb-3 flex items-center justify-between">
            <h4 className="text-sm font-medium text-(--color-text-primary)">
              Unit {selectedUnitId} 상세 정보
            </h4>
            <button
              type="button"
              onClick={() => {
                setSelectedUnitId(null);
                setDeviceDetail(null);
              }}
              className="rounded p-1 text-gray-400 hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          {isLoadingDetail ? (
            <div className="space-y-2">
              <div className="h-4 w-1/3 animate-pulse rounded bg-(--color-bg-elevated)" />
              <div className="h-4 w-2/3 animate-pulse rounded bg-(--color-bg-elevated)" />
            </div>
          ) : deviceDetail ? (
            <div className="space-y-3">
              {/* 요청 통계 */}
              <div>
                <p className="mb-1 text-xs font-medium text-(--color-text-muted)">요청 통계</p>
                <div className="grid grid-cols-3 gap-2">
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">읽기</p>
                    <p className="text-sm font-semibold text-(--color-text-primary)">{deviceDetail.stats.read_count}</p>
                  </div>
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">쓰기</p>
                    <p className="text-sm font-semibold text-(--color-text-primary)">{deviceDetail.stats.write_count}</p>
                  </div>
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">에러</p>
                    <p className={cn(
                      'text-sm font-semibold',
                      deviceDetail.stats.error_count > 0 ? 'text-red-500' : 'text-(--color-text-primary)',
                    )}>{deviceDetail.stats.error_count}</p>
                  </div>
                </div>
              </div>

              {/* 레지스터 맵 정보 */}
              {deviceDetail.register_map && Object.keys(deviceDetail.register_map).length > 0 && (
                <RegisterMapTable registerMap={deviceDetail.register_map} />
              )}

              {/* 생성 시간 */}
              {deviceDetail.created_at && (
                <p className="text-xs text-(--color-text-muted)">
                  생성: {new Date(deviceDetail.created_at).toLocaleString('ko-KR')}
                </p>
              )}
            </div>
          ) : (
            <p className="text-xs text-(--color-text-muted)">
              상세 정보를 불러올 수 없습니다.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// ---- 토픽 탭 (MQTT) ----

/** 토픽 통계 타입 */
interface TopicStatEntry {
  topic: string;
  count: number;
  bytes: number;
  updated_at: string;
}

/** 바이트를 읽기 쉬운 단위로 변환 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  return `${val < 10 ? val.toFixed(1) : Math.round(val)} ${units[i]}`;
}

interface SubscribedTopicEntry {
  topic: string;
  qos: number;
  total_count: number;
  total_bytes: number;
  received_topics: TopicStatEntry[];
}

function TopicsTab({ agentId }: { agentId: string }) {
  const { data: agent, isLoading: agentLoading } = useAgent(agentId, 'full');
  const { data: stats } = useAgentStats(agentId);

  const state = agent?.state as {
    subscribed_topics?: SubscribedTopicEntry[];
    unmatched_topics?: TopicStatEntry[];
    pub_topics?: TopicStatEntry[];
    max_pub_topics?: number;
  } | undefined;

  const subscribedTopics = state?.subscribed_topics ?? [];
  const unmatchedTopics = state?.unmatched_topics ?? [];
  const pubTopics = state?.pub_topics ?? [];

  const totalReceivedCount = subscribedTopics.reduce(
    (sum, s) => sum + s.received_topics.length, 0,
  ) + unmatchedTopics.length;

  if (agentLoading) {
    return (
      <div className="p-4">
        <div className="h-32 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      {/* 통계 요약 */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatCard label="구독 토픽" value={subscribedTopics.length} />
        <StatCard label="수신 토픽" value={totalReceivedCount} />
        <StatCard label="발행 토픽" value={pubTopics.length} />
        <StatCard label="수신 메시지" value={stats?.messages_in.toLocaleString() ?? '-'} />
      </div>

      {/* 구독/수신 토픽 트리 */}
      <SubscriptionTree
        subscriptions={subscribedTopics}
        unmatchedTopics={unmatchedTopics}
      />

      {/* 발행 토픽 */}
      <TopicStatsTable
        title={`발행 토픽${state?.max_pub_topics ? ` (최대 ${state.max_pub_topics})` : ''}`}
        topics={pubTopics}
        emptyMessage="발행된 토픽이 없습니다."
      />
    </div>
  );
}

/** 구독/수신 토픽 트리 (접이식) */
function SubscriptionTree({
  subscriptions,
  unmatchedTopics,
}: {
  subscriptions: SubscribedTopicEntry[];
  unmatchedTopics: TopicStatEntry[];
}) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>(() => {
    // 기본: 모두 펼침
    const init: Record<string, boolean> = {};
    for (const s of subscriptions) {
      init[s.topic] = true;
    }
    if (unmatchedTopics.length > 0) {
      init['__unmatched__'] = true;
    }
    return init;
  });

  const toggle = (key: string) =>
    setExpanded((prev) => ({ ...prev, [key]: !prev[key] }));

  const allKeys = [
    ...subscriptions.map((s) => s.topic),
    ...(unmatchedTopics.length > 0 ? ['__unmatched__'] : []),
  ];
  const allExpanded = allKeys.every((k) => expanded[k]);

  const toggleAll = () => {
    const next: Record<string, boolean> = {};
    for (const k of allKeys) {
      next[k] = !allExpanded;
    }
    setExpanded(next);
  };

  if (subscriptions.length === 0 && unmatchedTopics.length === 0) {
    return (
      <div>
        <h4 className="mb-2 text-sm font-medium text-(--color-text-primary)">구독 토픽</h4>
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4 text-center">
          <p className="text-xs text-(--color-text-muted)">구독 중인 토픽이 없습니다.</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <h4 className="text-sm font-medium text-(--color-text-primary)">구독 / 수신 토픽</h4>
        <button
          onClick={toggleAll}
          className="text-xs text-(--color-text-muted) hover:text-(--color-text-primary)"
        >
          {allExpanded ? '모두 접기' : '모두 펼치기'}
        </button>
      </div>
      <div className="overflow-hidden rounded-lg border border-(--color-border-default)">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
              <th className="px-4 py-2 text-left text-xs font-medium text-(--color-text-muted)">토픽</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">메시지 수</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">데이터량</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">QoS</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">최근 활동</th>
            </tr>
          </thead>
          <tbody>
            {subscriptions.map((sub) => (
              <SubscriptionRow
                key={sub.topic}
                sub={sub}
                isExpanded={!!expanded[sub.topic]}
                onToggle={() => toggle(sub.topic)}
              />
            ))}
            {unmatchedTopics.length > 0 && (
              <UnmatchedRow
                topics={unmatchedTopics}
                isExpanded={!!expanded['__unmatched__']}
                onToggle={() => toggle('__unmatched__')}
              />
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/** 구독 패턴 행 + 하위 수신 토픽 행 */
function SubscriptionRow({
  sub,
  isExpanded,
  onToggle,
}: {
  sub: SubscribedTopicEntry;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const hasChildren = sub.received_topics.length > 0;
  const sorted = [...sub.received_topics].sort((a, b) => b.count - a.count);

  return (
    <>
      {/* 구독 패턴 행 */}
      <tr
        className={cn(
          'border-b border-(--color-border-default) bg-(--color-bg-elevated)/50',
          hasChildren && 'cursor-pointer hover:bg-(--color-bg-elevated)',
        )}
        onClick={hasChildren ? onToggle : undefined}
      >
        <td className="px-4 py-2 font-mono text-xs font-medium text-(--color-text-primary)">
          <span className="mr-1.5 inline-block w-3 text-center text-(--color-text-muted)">
            {hasChildren ? (isExpanded ? '\u25BC' : '\u25B6') : '\u00B7'}
          </span>
          {sub.topic}
          {hasChildren && (
            <span className="ml-2 text-(--color-text-muted)">({sub.received_topics.length})</span>
          )}
        </td>
        <td className="px-4 py-2 text-right text-xs font-medium text-(--color-text-primary)">
          {sub.total_count > 0 ? sub.total_count.toLocaleString() : '-'}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
          {sub.total_bytes > 0 ? formatBytes(sub.total_bytes) : '-'}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{sub.qos}</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
      </tr>
      {/* 수신 토픽 (자식) 행 */}
      {isExpanded && sorted.map((t, idx) => (
        <tr
          key={t.topic}
          className={cn(
            'border-b border-(--color-border-default) last:border-b-0',
            idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
          )}
        >
          <td className="py-2 pr-4 pl-10 font-mono text-xs text-(--color-text-secondary)">{t.topic}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
            {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
          </td>
        </tr>
      ))}
    </>
  );
}

/** 매칭되지 않은 수신 토픽 그룹 */
function UnmatchedRow({
  topics,
  isExpanded,
  onToggle,
}: {
  topics: TopicStatEntry[];
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const sorted = [...topics].sort((a, b) => b.count - a.count);
  const totalCount = topics.reduce((s, t) => s + t.count, 0);
  const totalBytes = topics.reduce((s, t) => s + t.bytes, 0);

  return (
    <>
      <tr
        className="cursor-pointer border-b border-(--color-border-default) bg-(--color-bg-elevated)/50 hover:bg-(--color-bg-elevated)"
        onClick={onToggle}
      >
        <td className="px-4 py-2 text-xs font-medium text-(--color-text-muted)">
          <span className="mr-1.5 inline-block w-3 text-center">
            {isExpanded ? '\u25BC' : '\u25B6'}
          </span>
          (기타 수신 토픽)
          <span className="ml-2">({topics.length})</span>
        </td>
        <td className="px-4 py-2 text-right text-xs font-medium text-(--color-text-primary)">
          {totalCount.toLocaleString()}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(totalBytes)}</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
      </tr>
      {isExpanded && sorted.map((t, idx) => (
        <tr
          key={t.topic}
          className={cn(
            'border-b border-(--color-border-default) last:border-b-0',
            idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
          )}
        >
          <td className="py-2 pr-4 pl-10 font-mono text-xs text-(--color-text-secondary)">{t.topic}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
            {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
          </td>
        </tr>
      ))}
    </>
  );
}

/** 토픽 통계 테이블 (발행 토픽용) */
function TopicStatsTable({
  title,
  topics,
  emptyMessage,
}: {
  title: string;
  topics: TopicStatEntry[];
  emptyMessage: string;
}) {
  const sorted = [...topics].sort((a, b) => b.count - a.count);

  return (
    <div>
      <h4 className="mb-2 text-sm font-medium text-(--color-text-primary)">{title}</h4>
      {sorted.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4 text-center">
          <p className="text-xs text-(--color-text-muted)">{emptyMessage}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                <th className="px-4 py-2 text-left text-xs font-medium text-(--color-text-muted)">토픽</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">메시지 수</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">데이터량</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">최근 활동</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((t, idx) => (
                <tr
                  key={t.topic}
                  className={cn(
                    'border-b border-(--color-border-default) last:border-b-0',
                    idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
                  )}
                >
                  <td className="px-4 py-2 font-mono text-xs text-(--color-text-primary)">{t.topic}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
                    {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---- 저장소 탭 ----

function formatTimeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return `${seconds}초 전`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}분 전`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}시간 전`;
  const days = Math.floor(hours / 24);
  return `${days}일 전`;
}

function StoreEntryRow({ entry, maxHistorySize, agentId }: { entry: Record<string, unknown>; maxHistorySize: number; agentId: string }) {
  const [expanded, setExpanded] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyData, setHistoryData] = useState<Array<{ value: unknown; timestamp: string }> | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const execAgent = useExecAgent();

  const valueStr = typeof entry.value === 'string' ? entry.value : JSON.stringify(entry.value);
  const truncated = valueStr.length > 60;
  const displayValue = truncated && !expanded ? valueStr.slice(0, 60) + '...' : valueStr;

  const updatedAt = entry.updated_at ? new Date(entry.updated_at as string) : null;
  const timeAgo = updatedAt ? formatTimeAgo(updatedAt) : '-';

  const stateHistoryCount = (entry.history_count as number) || 0;
  const historyCount = historyData !== null ? historyData.length : stateHistoryCount;
  const hasHistory = maxHistorySize > 0;

  const handleRowClick = useCallback(() => {
    if (!hasHistory) return;

    if (historyOpen) {
      setHistoryOpen(false);
      return;
    }

    setHistoryOpen(true);
    setHistoryLoading(true);
    execAgent.mutate(
      {
        id: agentId,
        req: {
          command: 'get_history',
          params: {
            key: entry.key as string,
            namespace: (entry.namespace as string) || 'default',
          },
        },
      },
      {
        onSuccess: (res) => {
          const data = res as unknown as Record<string, unknown>;
          const history = (data?.history as Array<{ value: unknown; timestamp: string }>) ?? [];
          setHistoryData(history);
          setHistoryLoading(false);
        },
        onError: () => {
          setHistoryData([]);
          setHistoryLoading(false);
        },
      },
    );
  }, [hasHistory, historyOpen, execAgent, agentId, entry.key, entry.namespace]);

  // 히스토리 확장 행의 colSpan 계산: key + value + ns + ttl + updated + (선택적 history 컬럼)
  const colSpan = 5 + (maxHistorySize > 0 ? 1 : 0);

  return (
    <>
      <tr
        className={cn('hover:bg-(--color-bg-secondary)/50', hasHistory && 'cursor-pointer')}
        onClick={handleRowClick}
      >
        <td className="px-3 py-2 font-mono text-xs text-(--color-text-primary) max-w-[200px] truncate" title={entry.key as string}>
          <span className="inline-flex items-center gap-1">
            {hasHistory && (
              <ChevronRight className={cn('h-3 w-3 text-(--color-text-muted) transition-transform', historyOpen && 'rotate-90')} />
            )}
            {entry.key as string}
          </span>
        </td>
        <td className="px-3 py-2 font-mono text-xs text-(--color-text-secondary) max-w-[300px]">
          <span
            className={truncated ? 'cursor-pointer hover:text-(--color-text-primary)' : ''}
            onClick={(e) => {
              if (truncated) {
                e.stopPropagation();
                setExpanded(!expanded);
              }
            }}
          >
            {displayValue}
          </span>
        </td>
        <td className="px-3 py-2 text-xs text-(--color-text-muted)">
          {(entry.namespace as string) || '-'}
        </td>
        <td className="px-3 py-2 text-xs text-(--color-text-muted)">
          {(entry.ttl as string) || '\u221E'}
        </td>
        {maxHistorySize > 0 && (
          <td className="px-3 py-2 text-xs text-(--color-text-muted)">
            {historyCount}
          </td>
        )}
        <td className="px-3 py-2 text-xs text-(--color-text-muted)" title={entry.updated_at as string}>
          {timeAgo}
        </td>
      </tr>
      {historyOpen && (
        <tr>
          <td colSpan={colSpan} className="bg-(--color-bg-secondary)/30 px-6 py-3">
            {historyLoading ? (
              <p className="text-xs text-(--color-text-muted)">로딩 중...</p>
            ) : historyData && historyData.length > 0 ? (
              <div className="space-y-1">
                <p className="text-xs font-medium text-(--color-text-muted) mb-2">
                  히스토리 ({historyData.length}건)
                </p>
                <div className="space-y-1">
                  {historyData.map((h, i) => (
                    <div key={i} className="flex items-baseline gap-3 text-xs">
                      <span className="text-(--color-text-muted) whitespace-nowrap">
                        {new Date(h.timestamp).toLocaleString('ko-KR')}
                      </span>
                      <span className="font-mono text-(--color-text-secondary)">
                        {typeof h.value === 'string' ? h.value : JSON.stringify(h.value)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <p className="text-xs text-(--color-text-muted)">히스토리가 없습니다</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

function StoreTab({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient();
  const { data: agent, isLoading } = useAgent(agentId, 'full');
  const [isRefreshing, setIsRefreshing] = useState(false);

  const entries = useMemo(() => {
    const state = agent?.state as { entries?: Array<Record<string, unknown>> } | undefined;
    return state?.entries ?? [];
  }, [agent?.state]);

  const totalKeys = (agent?.state as { total_keys?: number } | undefined)?.total_keys ?? 0;
  const totalHistoryEntries = (agent?.state as { total_history_entries?: number } | undefined)?.total_history_entries ?? 0;
  const maxHistorySize = (agent?.state as { max_history_size?: number } | undefined)?.max_history_size ?? 0;

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await queryClient.invalidateQueries({ queryKey: ['agents', agentId, 'full'] });
    setIsRefreshing(false);
  }, [queryClient, agentId]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8 text-(--color-text-muted)">
        로딩 중...
      </div>
    );
  }

  return (
    <div className="p-4 space-y-3">
      {/* 헤더 및 새로고침 버튼 */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-(--color-text-muted)">
          전체 <span className="font-semibold text-(--color-text-primary)">{totalKeys}</span>개 키
          {maxHistorySize > 0 && (
            <> · 히스토리 <span className="font-semibold text-(--color-text-primary)">{totalHistoryEntries}</span>건</>
          )}
        </p>
        <button
          type="button"
          onClick={handleRefresh}
          disabled={isRefreshing}
          className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-secondary) disabled:opacity-50"
        >
          <RefreshCw className={cn('h-3.5 w-3.5', isRefreshing && 'animate-spin')} />
          새로고침
        </button>
      </div>

      {/* 테이블 */}
      {entries.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-8 text-center text-sm text-(--color-text-muted)">
          저장된 데이터가 없습니다
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-secondary)">
                <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">키</th>
                <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">값</th>
                <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">네임스페이스</th>
                <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">TTL</th>
                {maxHistorySize > 0 && (
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">히스토리</th>
                )}
                <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">갱신</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default)">
              {entries.map((entry) => (
                <StoreEntryRow key={entry.key as string} entry={entry} maxHistorySize={maxHistorySize} agentId={agentId} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---- 디바이스 탭 ----

function DevicesTab({ agentId, agentType }: { agentId: string; agentType: string }) {
  // Modbus TCP Server: 전용 디바이스 섹션 사용
  if (agentType === 'modbus-tcp-server') {
    return <ModbusDevicesSection agentId={agentId} />;
  }

  const { data: agent } = useAgent(agentId);
  const { data, isLoading } = useDevicesRealtime(
    agent?.name ? { agent: agent.name } : undefined,
  );
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  const devices = data?.data ?? [];
  const isNasa = agentType === 'samsung-nasa';
  const isLgap = agentType === 'lgap';

  // 소스 정보 (list_devices 응답에서 획득)
  const [sourceMap, setSourceMap] = useState<Record<string, string>>({});
  useEffect(() => {
    if ((!isNasa && !isLgap) || !agent) return;
    execAgent.mutate(
      { id: agentId, req: { command: 'list_devices' } },
      {
        onSuccess: (res) => {
          const items = (res as { data?: Array<{ address?: string; source?: string }> })?.data;
          if (!Array.isArray(items)) return;
          const map: Record<string, string> = {};
          for (const item of items) {
            if (item.address) map[item.address] = item.source ?? 'bridge';
          }
          setSourceMap(map);
        },
      },
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId, agent?.name]);

  // 추가 폼 상태
  const [showAddForm, setShowAddForm] = useState(false);
  const [newAddress, setNewAddress] = useState('');
  const [newDeviceId, setNewDeviceId] = useState('');
  const [newDeviceType, setNewDeviceType] = useState('');
  const [newDeviceName, setNewDeviceName] = useState('');
  // LGAP 전용
  const [newZone, setNewZone] = useState('');
  const [newLgapDeviceId, setNewLgapDeviceId] = useState('');

  function handleAddDevice() {
    if (isLgap) {
      const zoneVal = parseInt(newZone.trim(), 0);
      if (isNaN(zoneVal) || zoneVal < 0 || zoneVal > 255) {
        addNotification({ type: 'error', message: '존 주소는 0~255 범위여야 합니다 (0x00~0xFF)' });
        return;
      }
      execAgent.mutate(
        {
          id: agentId,
          req: {
            command: 'add_device',
            params: {
              zone: zoneVal,
              ...(newLgapDeviceId.trim() && { device_id: newLgapDeviceId.trim() }),
              ...(newDeviceName.trim() && { name: newDeviceName.trim() }),
            },
          },
        },
        {
          onSuccess: () => {
            setShowAddForm(false);
            setNewZone('');
            setNewLgapDeviceId('');
            setNewDeviceName('');
            addNotification({ type: 'success', message: 'LGAP 디바이스가 추가되었습니다' });
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          },
        },
      );
      return;
    }

    if (!newAddress.trim()) return;
    execAgent.mutate(
      {
        id: agentId,
        req: {
          command: 'add_device',
          params: {
            address: newAddress.trim(),
            ...(newDeviceId.trim() && { device_id: newDeviceId.trim() }),
            ...(newDeviceType && { device_type: newDeviceType }),
            ...(newDeviceName.trim() && { name: newDeviceName.trim() }),
          },
        },
      },
      {
        onSuccess: () => {
          setShowAddForm(false);
          setNewAddress('');
          setNewDeviceId('');
          setNewDeviceType('');
          setNewDeviceName('');
          addNotification({ type: 'success', message: '디바이스가 추가되었습니다' });
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
        },
      },
    );
  }

  function handleRemoveDevice(deviceId: string, address?: string) {
    const params: Record<string, unknown> = {};
    if (deviceId) params.device_id = deviceId;
    else if (address) params.address = address;
    execAgent.mutate(
      { id: agentId, req: { command: 'remove_device', params } },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: '디바이스가 제거되었습니다' });
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 제거 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
        },
      },
    );
  }

  // 디바이스 ID에서 주소 부분 추출 (형식: "agentName:XX.XX.XX")
  function extractAddress(id: string): string {
    const parts = id.split(':');
    return parts.length > 1 ? parts.slice(1).join(':') : id;
  }

  // 주소를 소스맵과 매칭 (XX.XX.XX → XX XX XX 변환)
  function getSource(id: string): string {
    const addr = extractAddress(id);
    // dot-separated → space-separated 시도
    const spaced = addr.replace(/\./g, ' ');
    return sourceMap[spaced] ?? sourceMap[addr] ?? '';
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 p-4 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 추가 버튼 */}
      {(isNasa || isLgap) && (
        <div className="flex items-center justify-between">
          <span className="text-xs text-(--color-text-muted)">
            {devices.length}개 디바이스
          </span>
          <button
            type="button"
            onClick={() => setShowAddForm(!showAddForm)}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-3.5 w-3.5" />
            디바이스 추가
          </button>
        </div>
      )}

      {/* 디바이스 추가 다이얼로그 */}
      {showAddForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowAddForm(false)}>
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">디바이스 추가</h3>
            </div>
            <div className="space-y-3 p-4">
              {isLgap ? (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">존 주소</label>
                    <input
                      type="text"
                      placeholder="예: 0x11 또는 17"
                      value={newZone}
                      onChange={(e) => setNewZone(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                    <p className="mt-1 text-xs text-(--color-text-muted)">상위 니블=그룹, 하위 니블=유닛 (예: 0x11 = 그룹1 유닛1)</p>
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 ID (선택)</label>
                    <input
                      type="text"
                      placeholder="디바이스 ID"
                      value={newLgapDeviceId}
                      onChange={(e) => setNewLgapDeviceId(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                </>
              ) : (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">주소</label>
                    <input
                      type="text"
                      placeholder="예: 20 00 03"
                      value={newAddress}
                      onChange={(e) => setNewAddress(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 ID (선택)</label>
                    <input
                      type="text"
                      placeholder="디바이스 ID"
                      value={newDeviceId}
                      onChange={(e) => setNewDeviceId(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 종류</label>
                    <select
                      value={newDeviceType}
                      onChange={(e) => setNewDeviceType(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    >
                      <option value="">자동 감지</option>
                      <option value="indoor">실내기</option>
                      <option value="outdoor">실외기</option>
                    </select>
                  </div>
                </>
              )}
              {/* 이름 (공통, 선택) */}
              <div>
                <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">이름 (선택)</label>
                <input
                  type="text"
                  placeholder="디바이스 이름"
                  value={newDeviceName}
                  onChange={(e) => setNewDeviceName(e.target.value)}
                  className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                />
              </div>
              <p className="text-xs text-(--color-text-muted)">동적으로 추가된 디바이스는 에이전트 재시작 시 초기화됩니다.</p>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={() => setShowAddForm(false)}
                className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
              >
                취소
              </button>
              <button
                type="button"
                onClick={handleAddDevice}
                disabled={(isLgap ? !newZone.trim() : !newAddress.trim()) || execAgent.isPending}
                className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {execAgent.isPending ? '추가 중...' : '추가'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 디바이스 목록 (테이블) */}
      {devices.length === 0 ? (
        <div className="p-6 text-center">
          <HardDrive className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-(--color-text-muted)">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) text-left text-xs text-(--color-text-muted)">
                <th className="pb-2 pr-3 font-medium">이름</th>
                <th className="pb-2 pr-3 font-medium">ID</th>
                <th className="pb-2 pr-3 font-medium">종류</th>
                <th className="pb-2 pr-3 font-medium">연결</th>
                <th className="pb-2 pr-3 font-medium">소스</th>
                {(isNasa || isLgap) && <th className="pb-2 font-medium" />}
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default)">
              {devices.map((d) => {
                const source = getSource(d.id);
                const isConfig = source === 'config';
                return (
                  <tr key={d.id} className="text-(--color-text-primary)">
                    <td className="py-2 pr-3 font-medium">{d.name || extractAddress(d.id)}</td>
                    <td className="py-2 pr-3 text-xs text-(--color-text-muted) font-mono">{extractAddress(d.id)}</td>
                    <td className="py-2 pr-3 text-xs">{getDeviceTypeLabel(d.type)}</td>
                    <td className="py-2 pr-3"><DeviceStatusBadge online={d.online} /></td>
                    <td className="py-2 pr-3">
                      {source && (
                        <span
                          className={cn(
                            'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium',
                            isConfig
                              ? 'bg-(--color-bg-elevated) text-(--color-text-muted)'
                              : 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400',
                          )}
                        >
                          {isConfig && <Lock className="h-2.5 w-2.5" />}
                          {isConfig ? '설정' : '동적'}
                        </span>
                      )}
                    </td>
                    {(isNasa || isLgap) && (
                      <td className="py-2 text-right">
                        {!isConfig && source && (
                          <button
                            type="button"
                            onClick={() => handleRemoveDevice(d.name || '', extractAddress(d.id))}
                            disabled={execAgent.isPending}
                            className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 disabled:opacity-50 dark:hover:bg-red-950"
                            title="디바이스 제거"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        )}
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
