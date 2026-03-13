// 에이전트 상세 패널.
// 행 확장 시 표시되며, 통계 탭과 설정 탭으로 구성된다.

import { useCallback, useEffect, useState } from 'react';
import { HardDrive, Lock, Pencil, Plus, Save, Trash2, X } from 'lucide-react';

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

// ---- 디바이스 탭 ----

function DevicesTab({ agentId, agentType }: { agentId: string; agentType: string }) {
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
