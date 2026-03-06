// 에이전트 상세 패널.
// 행 확장 시 표시되며, 통계 탭과 설정 탭으로 구성된다.

import { useCallback, useEffect, useState } from 'react';
import { Pencil, Save, X } from 'lucide-react';

import { useAgent, useAgentStats, useConfigureAgent } from '@/hooks/useAgent';
import { cn } from '@/lib/utils/cn';
import { getAgentConfigSchema } from '@/config/agentSchemas';
import { DynamicForm } from '@/components/property/DynamicForm';
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

type Tab = 'stats' | 'config';

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
      </div>

      {/* 탭 컨텐츠 */}
      {tab === 'stats' && <StatsTab agentId={agentId} />}
      {tab === 'config' && <ConfigTab agentId={agentId} agentType={agentType} />}
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
