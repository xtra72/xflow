// 에이전트 상세 패널.
// 행 확장 시 표시되며, 통계 탭과 설정 탭으로 구성된다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, ChevronDown, ChevronRight, HardDrive, Lock, Pencil, Plus, Save, Server, Trash2, X } from 'lucide-react';

import { useAgent, useAgentStats, useConfigureAgent, useExecAgent } from '@/hooks/useAgent';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import { getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import { getAgentConfigSchema } from '@/config/agentSchemas';
import { DynamicForm } from '@/components/property/DynamicForm';
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

type Tab = 'stats' | 'config' | 'devices';

/** 통계 카드 항목 */
function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400">{label}</p>
      <p className="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{value}</p>
    </div>
  );
}

export default function AgentDetailPanel({ agentId, agentType }: AgentDetailPanelProps) {
  const [tab, setTab] = useState<Tab>('stats');

  return (
    <div>
      {/* 탭 헤더 */}
      <div className="flex border-b border-gray-200 px-4 dark:border-gray-700">
        <TabButton label="통계" active={tab === 'stats'} onClick={() => setTab('stats')} />
        <TabButton label="설정" active={tab === 'config'} onClick={() => setTab('config')} />
        <TabButton label="디바이스" active={tab === 'devices'} onClick={() => setTab('devices')} />
      </div>

      {/* 탭 컨텐츠 */}
      {tab === 'stats' && <StatsTab agentId={agentId} />}
      {tab === 'config' && <ConfigTab agentId={agentId} agentType={agentType} />}
      {tab === 'devices' && <DevicesTab agentId={agentId} agentType={agentType} />}
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
          : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300',
      )}
    >
      {label}
    </button>
  );
}

// ---- 통계 탭 ----

function StatsTab({ agentId }: { agentId: string }) {
  const { data: stats, isLoading } = useAgentStats(agentId);
  const addNotification = useUIStore((s) => s.addNotification);

  // 컴포넌트별 로그 레벨 상태
  const [componentLogLevel, setComponentLogLevel_] = useState<string>('');
  const [isLogLevelUpdating, setIsLogLevelUpdating] = useState(false);

  // 마운트 시 현재 에이전트의 로그 레벨 로드
  useEffect(() => {
    let cancelled = false;
    getLogLevels()
      .then((info) => {
        if (cancelled) return;
        const key = `agent.${agentId}`;
        const level = info.components[key];
        setComponentLogLevel_(level ?? '');
      })
      .catch(() => {
        // 로드 실패 시 무시 (기본값 유지)
      });
    return () => {
      cancelled = true;
    };
  }, [agentId]);

  /** 로그 레벨 변경 핸들러 */
  async function handleLogLevelChange(value: string) {
    const componentName = `agent.${agentId}`;
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

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-lg bg-gray-200 dark:bg-gray-700"
          />
        ))}
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="p-4 text-sm text-gray-500 dark:text-gray-400">
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
        <div className="col-span-2 md:col-span-4">
          <div className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
            <span className="text-xs font-medium text-gray-500 dark:text-gray-400">연결 상태</span>
            <span
              className={cn(
                'inline-flex items-center gap-1 text-sm font-medium',
                stats.connected
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-gray-500 dark:text-gray-400',
              )}
            >
              <span
                className={cn(
                  'h-2 w-2 rounded-full',
                  stats.connected ? 'bg-green-500' : 'bg-gray-400',
                )}
              />
              {stats.connected ? '연결됨' : '연결 해제'}
            </span>
          </div>
        </div>
      </div>

      {/* 로그 레벨 설정 */}
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
        <label
          htmlFor={`agent-log-level-${agentId}`}
          className="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400"
        >
          로그 레벨
        </label>
        <select
          id={`agent-log-level-${agentId}`}
          value={componentLogLevel}
          onChange={(e) => handleLogLevelChange(e.target.value)}
          disabled={isLogLevelUpdating}
          className={cn(
            'block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'dark:border-gray-600 dark:bg-gray-700 dark:text-white',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <option value="">기본값(Default)</option>
          <option value="debug">DEBUG</option>
          <option value="info">INFO</option>
          <option value="warn">WARN</option>
          <option value="error">ERROR</option>
        </select>
      </div>
    </div>
  );
}

// ---- 설정 탭 ----

function ConfigTab({ agentId, agentType }: { agentId: string; agentType: string }) {
  const { data: agent, isLoading } = useAgent(agentId);
  const configureAgent = useConfigureAgent();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Record<string, unknown>>({});

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
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  }, [agentId, draft, configureAgent]);

  if (isLoading) {
    return (
      <div className="space-y-3 p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
        ))}
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="p-4 text-sm text-gray-500 dark:text-gray-400">
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
              className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
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
            className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
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

      {/* 설정 폼 */}
      <DynamicForm
        nodeId={agentId}
        data={editing ? draft : config}
        schema={schema}
        onChange={setDraft}
        readOnly={!editing}
      />
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
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400">레지스터 맵</p>
      {visibleAreas.map((area) => {
        const data = registerMap[area]!;
        const entries = sortedEntries(data);
        const isExpanded = expandedAreas[area] ?? false;
        const label = REGISTER_AREA_LABELS[area] ?? area;

        return (
          <div key={area} className="rounded border border-gray-200 dark:border-gray-600">
            {/* 영역 헤더 (클릭으로 접기/펼치기) */}
            <button
              type="button"
              onClick={() => toggleArea(area)}
              className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-xs font-medium text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              {isExpanded ? (
                <ChevronDown className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              )}
              <span>{label}</span>
              <span className="ml-auto rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-normal text-gray-500 dark:bg-gray-600 dark:text-gray-400">
                {entries.length}
              </span>
            </button>

            {/* 레지스터 테이블 */}
            {isExpanded && (
              <div className="max-h-64 overflow-y-auto border-t border-gray-200 dark:border-gray-600">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="bg-gray-50 text-left text-gray-500 dark:bg-gray-700 dark:text-gray-400">
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
                  <tbody className="divide-y divide-gray-100 dark:divide-gray-600">
                    {entries.map(([addr, value]) => (
                      <tr key={addr} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                        <td className="px-2 py-1 font-mono text-gray-700 dark:text-gray-300">{addr}</td>
                        {isBooleanArea(area) ? (
                          <td className="px-2 py-1">
                            <span
                              className={cn(
                                'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
                                value
                                  ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                                  : 'bg-gray-100 text-gray-500 dark:bg-gray-600 dark:text-gray-400',
                              )}
                            >
                              {value ? 'ON' : 'OFF'}
                            </span>
                          </td>
                        ) : (
                          <>
                            <td className="px-2 py-1 font-mono text-gray-700 dark:text-gray-300">
                              {typeof value === 'number' ? value : '-'}
                            </td>
                            <td className="px-2 py-1 font-mono text-gray-500 dark:text-gray-400">
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
            className="h-32 animate-pulse rounded-lg bg-gray-200 dark:bg-gray-700"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-gray-500 dark:text-gray-400">
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
          <div className="text-sm font-medium text-gray-900 dark:text-white">디바이스 추가</div>
          <div>
            <label htmlFor="modbus-add-unit-id" className="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
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
              className="block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
            />
          </div>
          <div>
            <label htmlFor="modbus-add-name" className="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              이름 (선택)
            </label>
            <input
              id="modbus-add-name"
              type="text"
              placeholder="예: 센서 디바이스 1"
              value={addName}
              onChange={(e) => setAddName(e.target.value)}
              className="block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
            />
          </div>
          <div>
            <p className="mb-1.5 text-xs font-medium text-gray-600 dark:text-gray-400">
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
                        : 'border-gray-200 bg-gray-50/50 dark:border-gray-700 dark:bg-gray-800/30',
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
                        <span className="text-xs font-medium text-gray-700 dark:text-gray-300">
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
                              <span className="text-[10px] text-gray-500 dark:text-gray-400">Start:</span>
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
                                className="w-20 rounded border border-gray-300 px-1.5 py-0.5 font-mono text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                              />
                            </div>
                            <div className="flex items-center gap-1">
                              <span className="text-[10px] text-gray-500 dark:text-gray-400">Count:</span>
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
                                className="w-20 rounded border border-gray-300 px-1.5 py-0.5 font-mono text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
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
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300"
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
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300"
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
          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {devices.map((d) => (
            <div
              key={d.unit_id}
              className={cn(
                'cursor-pointer rounded-lg border bg-white p-3 transition-colors dark:bg-gray-800',
                selectedUnitId === d.unit_id
                  ? 'border-blue-400 ring-1 ring-blue-400 dark:border-blue-500'
                  : 'border-gray-200 hover:border-gray-300 dark:border-gray-700 dark:hover:border-gray-600',
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
                  <span className="inline-flex h-6 min-w-[1.5rem] items-center justify-center rounded bg-gray-100 px-1.5 text-xs font-bold text-gray-700 dark:bg-gray-700 dark:text-gray-300">
                    {d.unit_id}
                  </span>
                  <span className="text-sm font-medium text-gray-900 dark:text-white">
                    {d.name || `Device ${d.unit_id}`}
                  </span>
                </div>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
                      d.status === 'active'
                        ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                        : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
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
                <div className="text-[10px] text-gray-500 dark:text-gray-400">
                  <span className="font-medium">Coils:</span> {d.register_counts.coils}
                </div>
                <div className="text-[10px] text-gray-500 dark:text-gray-400">
                  <span className="font-medium">DI:</span> {d.register_counts.discrete_inputs}
                </div>
                <div className="text-[10px] text-gray-500 dark:text-gray-400">
                  <span className="font-medium">HR:</span> {d.register_counts.holding_registers}
                </div>
                <div className="text-[10px] text-gray-500 dark:text-gray-400">
                  <span className="font-medium">IR:</span> {d.register_counts.input_registers}
                </div>
              </div>

              {/* 통계 요약 */}
              <div className="mt-2 flex items-center gap-3 border-t border-gray-100 pt-2 dark:border-gray-700">
                <span className="flex items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400">
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
        <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800">
          <div className="mb-3 flex items-center justify-between">
            <h4 className="text-sm font-medium text-gray-900 dark:text-white">
              Unit {selectedUnitId} 상세 정보
            </h4>
            <button
              type="button"
              onClick={() => {
                setSelectedUnitId(null);
                setDeviceDetail(null);
              }}
              className="rounded p-1 text-gray-400 hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          {isLoadingDetail ? (
            <div className="space-y-2">
              <div className="h-4 w-1/3 animate-pulse rounded bg-gray-200 dark:bg-gray-600" />
              <div className="h-4 w-2/3 animate-pulse rounded bg-gray-200 dark:bg-gray-600" />
            </div>
          ) : deviceDetail ? (
            <div className="space-y-3">
              {/* 요청 통계 */}
              <div>
                <p className="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">요청 통계</p>
                <div className="grid grid-cols-3 gap-2">
                  <div className="rounded border border-gray-200 bg-white p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <p className="text-xs text-gray-500 dark:text-gray-400">읽기</p>
                    <p className="text-sm font-semibold text-gray-900 dark:text-white">{deviceDetail.stats.read_count}</p>
                  </div>
                  <div className="rounded border border-gray-200 bg-white p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <p className="text-xs text-gray-500 dark:text-gray-400">쓰기</p>
                    <p className="text-sm font-semibold text-gray-900 dark:text-white">{deviceDetail.stats.write_count}</p>
                  </div>
                  <div className="rounded border border-gray-200 bg-white p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <p className="text-xs text-gray-500 dark:text-gray-400">에러</p>
                    <p className={cn(
                      'text-sm font-semibold',
                      deviceDetail.stats.error_count > 0 ? 'text-red-500' : 'text-gray-900 dark:text-white',
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
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  생성: {new Date(deviceDetail.created_at).toLocaleString('ko-KR')}
                </p>
              )}
            </div>
          ) : (
            <p className="text-xs text-gray-500 dark:text-gray-400">
              상세 정보를 불러올 수 없습니다.
            </p>
          )}
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

  // 소스 정보 (list_devices 응답에서 획득)
  const [sourceMap, setSourceMap] = useState<Record<string, string>>({});
  useEffect(() => {
    if (!isNasa || !agent) return;
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

  function handleAddDevice() {
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
          },
        },
      },
      {
        onSuccess: () => {
          setShowAddForm(false);
          setNewAddress('');
          setNewDeviceId('');
          setNewDeviceType('');
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
            className="h-20 animate-pulse rounded-lg bg-gray-200 dark:bg-gray-700"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 추가 버튼 */}
      {isNasa && (
        <div className="flex items-center justify-between">
          <span className="text-xs text-gray-500 dark:text-gray-400">
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

      {/* 추가 폼 */}
      {showAddForm && (
        <div className="space-y-2 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800 dark:bg-blue-950">
          <input
            type="text"
            placeholder="주소 (예: 20 00 03)"
            value={newAddress}
            onChange={(e) => setNewAddress(e.target.value)}
            className="block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
          />
          <input
            type="text"
            placeholder="디바이스 ID (선택)"
            value={newDeviceId}
            onChange={(e) => setNewDeviceId(e.target.value)}
            className="block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
          />
          <select
            value={newDeviceType}
            onChange={(e) => setNewDeviceType(e.target.value)}
            className="block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
          >
            <option value="">자동 감지</option>
            <option value="indoor">실내기</option>
            <option value="outdoor">실외기</option>
          </select>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleAddDevice}
              disabled={!newAddress.trim() || execAgent.isPending}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
            >
              {execAgent.isPending ? '추가 중...' : '추가'}
            </button>
            <button
              type="button"
              onClick={() => setShowAddForm(false)}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300"
            >
              취소
            </button>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400">
            동적으로 추가된 디바이스는 에이전트 재시작 시 초기화됩니다.
          </p>
        </div>
      )}

      {/* 디바이스 목록 */}
      {devices.length === 0 ? (
        <div className="p-6 text-center">
          <HardDrive className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3">
          {devices.map((d) => {
            const source = getSource(d.id);
            const isConfig = source === 'config';
            return (
              <div
                key={d.id}
                className="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800"
              >
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-gray-900 dark:text-white">
                    {d.name || d.id}
                  </span>
                  <div className="flex items-center gap-1.5">
                    {source && (
                      <span
                        className={cn(
                          'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium',
                          isConfig
                            ? 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                            : 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400',
                        )}
                      >
                        {isConfig && <Lock className="h-2.5 w-2.5" />}
                        {isConfig ? '설정' : '동적'}
                      </span>
                    )}
                    <DeviceStatusBadge online={d.online} />
                  </div>
                </div>
                <div className="mt-1 flex items-center justify-between">
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {getDeviceTypeLabel(d.type)} &middot; {d.protocol.toUpperCase()}
                  </p>
                  {isNasa && !isConfig && source && (
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
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
