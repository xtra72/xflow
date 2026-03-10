// 에이전트 패널 컴포넌트.
// 상단에 에이전트 상태 요약, 하단에 에이전트 리스트 테이블을 표시한다.

import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  ArrowRight,
  Pause,
  Play,
  RotateCcw,
} from 'lucide-react';
import { Link } from 'react-router';

import PanelSettingsDropdown, { type ColumnOption } from '@/components/common/PanelSettingsDropdown';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useAgents } from '@/hooks';
import AgentStatusBadge from '@/pages/agents/AgentStatusBadge';
import {
  useUIStore,
  type AgentColumnKey,
} from '@/stores/uiStore';
import { startAgent, stopAgent, restartAgent } from '@/services/api/agentService';

/** 컬럼 옵션 (설정 드롭다운용) */
const AGENT_COLUMN_OPTIONS: ColumnOption<AgentColumnKey>[] = [
  { key: 'name', label: '이름' },
  { key: 'type', label: '타입' },
  { key: 'status', label: '상태' },
  { key: 'uptime', label: '업타임' },
  { key: 'messages', label: '메시지 IN/OUT' },
  { key: 'actions', label: '액션' },
];

/** 에이전트 패널 - 자체적으로 useAgents 훅으로 데이터 관리 */
export default function AgentPanel() {
  const queryClient = useQueryClient();
  const refreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;
  const { data, isLoading } = useAgents(undefined, refreshMs);
  const agents = data?.data ?? [];

  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 패널 설정
  const title = useUIStore((s) => s.agentPanelTitle);
  const visibleColumns = useUIStore((s) => s.agentVisibleColumns);
  const setTitle = useUIStore((s) => s.setAgentPanelTitle);
  const setVisibleColumns = useUIStore((s) => s.setAgentVisibleColumns);

  // 숨겨진 컬럼으로 정렬 중이면 기본(name)으로 fallback
  useEffect(() => {
    if (!visibleColumns.includes(sort.field as AgentColumnKey)) {
      setSort({ field: 'name', direction: 'asc' });
    }
  }, [visibleColumns, sort.field]);

  const show = (key: AgentColumnKey) => visibleColumns.includes(key);

  // 상태 요약 집계
  const summary = useMemo(() => {
    const total = agents.length;
    const active = agents.filter(
      (a) => a.connected === true || a.status === 'running',
    ).length;
    const inactive = total - active;
    return { total, active, inactive };
  }, [agents]);

  // 정렬된 에이전트 목록 (최대 10개)
  const sortedAgents = useMemo(() => {
    const sorted = [...agents].sort((a, b) => {
      let valA: string | number;
      let valB: string | number;

      switch (sort.field) {
        case 'name':
          valA = a.name.toLowerCase();
          valB = b.name.toLowerCase();
          break;
        case 'type':
          valA = a.type.toLowerCase();
          valB = b.type.toLowerCase();
          break;
        case 'status':
          valA = a.connected ? '1' : '0';
          valB = b.connected ? '1' : '0';
          break;
        default:
          valA = a.name.toLowerCase();
          valB = b.name.toLowerCase();
      }

      if (valA < valB) return sort.direction === 'asc' ? -1 : 1;
      if (valA > valB) return sort.direction === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted.slice(0, 10);
  }, [agents, sort]);

  /** 정렬 변경 핸들러 */
  const handleSort = (field: string) => {
    setSort((prev) => ({
      field,
      direction: prev.field === field && prev.direction === 'asc' ? 'desc' : 'asc',
    }));
  };

  // 에이전트 액션 뮤테이션
  const startMutation = useMutation({
    mutationFn: startAgent,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  });

  const stopMutation = useMutation({
    mutationFn: stopAgent,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  });

  const restartMutation = useMutation({
    mutationFn: restartAgent,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  });

  /** 에이전트 상태에 따른 액션 버튼 렌더링 */
  const renderActionButton = (agent: (typeof agents)[number]) => {
    const isPending =
      startMutation.isPending || stopMutation.isPending || restartMutation.isPending;
    const isConnected = agent.connected === true || agent.status === 'running';

    if (agent.status === 'error') {
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            restartMutation.mutate(agent.id);
          }}
          disabled={isPending}
          className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
          aria-label={`${agent.name} 재시작`}
        >
          <RotateCcw className="h-4 w-4" />
        </button>
      );
    }

    if (isConnected) {
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            stopMutation.mutate(agent.id);
          }}
          disabled={isPending}
          className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
          aria-label={`${agent.name} 중지`}
        >
          <Pause className="h-4 w-4" />
        </button>
      );
    }

    return (
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          startMutation.mutate(agent.id);
        }}
        disabled={isPending}
        className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
        aria-label={`${agent.name} 시작`}
      >
        <Play className="h-4 w-4" />
      </button>
    );
  };

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      {/* 헤더: 타이틀 + 설정 */}
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          {title}
        </h3>
        <PanelSettingsDropdown
          title={title}
          onTitleChange={setTitle}
          columns={AGENT_COLUMN_OPTIONS}
          visibleColumns={visibleColumns}
          onColumnsChange={setVisibleColumns}
        />
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
        </div>
      ) : (
        <>
          <div className="mb-6 flex gap-3">
            <span className="inline-flex items-center gap-1.5 rounded-full bg-blue-100 px-3 py-1 text-sm font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
              전체 {summary.total}
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-green-100 px-3 py-1 text-sm font-medium text-green-700 dark:bg-green-900/30 dark:text-green-400">
              활성 {summary.active}
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-sm font-medium text-gray-600 dark:bg-gray-700/30 dark:text-gray-400">
              비활성 {summary.inactive}
            </span>
          </div>

          {/* 에이전트 리스트 테이블 */}
          {sortedAgents.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">
              등록된 에이전트가 없습니다.
            </p>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-gray-200 dark:border-gray-700">
                      {show('name') && (
                        <SortableHeader
                          label="이름"
                          field="name"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                        />
                      )}
                      {show('type') && (
                        <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                          타입
                        </th>
                      )}
                      {show('status') && (
                        <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                          상태
                        </th>
                      )}
                      {show('uptime') && (
                        <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                          업타임
                        </th>
                      )}
                      {show('messages') && (
                        <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                          메시지 IN/OUT
                        </th>
                      )}
                      {show('actions') && (
                        <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                          액션
                        </th>
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                    {sortedAgents.map((agent) => (
                      <tr
                        key={agent.id}
                        className="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700/50"
                      >
                        {show('name') && (
                          <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">
                            {agent.name}
                          </td>
                        )}
                        {show('type') && (
                          <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                            {agent.type}
                          </td>
                        )}
                        {show('status') && (
                          <td className="px-4 py-3">
                            <AgentStatusBadge connected={agent.connected} status={agent.status} />
                          </td>
                        )}
                        {show('uptime') && (
                          <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                            {agent.uptime ?? '-'}
                          </td>
                        )}
                        {show('messages') && (
                          <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                            {agent.stats?.messages_in ?? 0} / {agent.stats?.messages_out ?? 0}
                          </td>
                        )}
                        {show('actions') && (
                          <td className="px-4 py-3 text-right">
                            {renderActionButton(agent)}
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {/* 더 보기 링크 */}
              {agents.length > 10 && (
                <div className="mt-4 text-right">
                  <Link
                    to="/agents"
                    className="inline-flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                  >
                    더 보기
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
