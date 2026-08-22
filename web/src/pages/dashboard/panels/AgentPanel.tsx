// 에이전트 패널 컴포넌트.
// 상단에 에이전트 상태 요약, 하단에 에이전트 리스트 테이블을 표시한다.

import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Bot,
  CircleStop,
  Pause,
  Play,
  RotateCcw,
} from 'lucide-react';
import { Link } from 'react-router';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useAgents } from '@/hooks';
import { useAgentActionsTarget } from '@/hooks/useResourceActions';
import { useAgentsTarget } from '@/hooks/useResourceTargets';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import {
  useUIStore,
  type AgentColumnKey,
  type PanelConfig,
} from '@/stores/uiStore';
import { startAgent, stopAgent, restartAgent } from '@/services/api/agentService';
import { usePanelTitleVisible } from '../panelChromeContext';

/** 전체 AgentColumnKey 기본 목록 */
const ALL_AGENT_COLUMNS: AgentColumnKey[] = ['name', 'type', 'status', 'uptime', 'messages', 'actions'];

interface AgentPanelProps {
  /** 패널 설정 (멀티-대시보드 모델에서 전달) */
  panelConfig?: PanelConfig;
}

/** 에이전트 패널 - 자체적으로 useAgents 훅으로 데이터 관리 */
export default function AgentPanel({ panelConfig }: AgentPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const queryClient = useQueryClient();
  const refreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04): 원격이면 노드 미러 목록을
  // 소스로 쓴다(useAgentsTarget). 로컬은 기존 useAgents 그대로(회귀 없음).
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localQuery = useAgents(undefined, refreshMs);
  const remoteQuery = useAgentsTarget(target);
  const agents = useMemo(
    () => (remote ? (remoteQuery.data?.data ?? []) : (localQuery.data?.data ?? [])),
    [remote, remoteQuery.data, localQuery.data],
  );
  const isLoading = remote ? remoteQuery.isLoading : localQuery.isLoading;

  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 패널 설정 (멀티-대시보드 패널 config에서 읽기)
  const title = panelConfig?.title ?? t('dashboard.agents');
  const visibleColumns = useMemo(
    () => (panelConfig?.config?.visibleColumns as AgentColumnKey[]) ?? [...ALL_AGENT_COLUMNS],
    [panelConfig?.config?.visibleColumns],
  );
  const panelColor = panelConfig?.config?.panelColor as string | undefined;
  const accentElements = (panelConfig?.config?.accentElements as Record<string, string | boolean>) ?? {};
  const acColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };

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
      (a) => a.connected === true,
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

  // 에이전트 액션 뮤테이션(로컬). 원격은 그룹 D 명령으로 라우팅한다(REQ-L12/J12).
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

  // 원격 라이프사이클: 그룹 D 명령(REQ-L12, start/stop/restart 지원). 게이팅 적용.
  const remoteActions = useAgentActionsTarget(target);
  const gating = useTargetGating(target);

  /** 에이전트 상태에 따른 액션 버튼 렌더링 */
  const renderActionButton = (agent: (typeof agents)[number]) => {
    const isConnected = agent.connected === true;

    // 원격: 그룹 D 명령 + 게이팅. 로컬은 기존 동작 불변.
    if (remote) {
      const action =
        agent.status === 'error' ? 'restart' : isConnected ? 'stop' : 'start';
      const Icon = action === 'restart' ? RotateCcw : action === 'stop' ? Pause : Play;
      const labelSuffix = action === 'restart' ? t('dashboard.action.restart') : action === 'stop' ? t('dashboard.action.stop') : t('dashboard.action.start');
      const disabled =
        !remoteActions.supports(action) ||
        !gating.canControl() ||
        remoteActions.pending[action] === true;
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            void remoteActions.perform(action, agent.id).catch(() => {});
          }}
          disabled={disabled}
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${agent.name} ${labelSuffix}`}
        >
          <Icon className="h-4 w-4" />
        </button>
      );
    }

    const isPending =
      startMutation.isPending || stopMutation.isPending || restartMutation.isPending;

    if (agent.status === 'error') {
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            restartMutation.mutate(agent.id);
          }}
          disabled={isPending}
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${agent.name} ${t('dashboard.action.restart')}`}
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
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${agent.name} ${t('dashboard.action.stop')}`}
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
        className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
        aria-label={`${agent.name} ${t('dashboard.action.start')}`}
      >
        <Play className="h-4 w-4" />
      </button>
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-6 shadow">
      {/* 헤더: 타이틀 + 설정 */}
      {showTitle && (
        <div className="mb-4 flex shrink-0 items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <Bot className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <h3
              className="truncate text-lg font-semibold text-(--color-text-primary)"
              style={acColor('header') ? { color: acColor('header')! } : undefined}
            >
              {title}
            </h3>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div
            className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600"
            style={acColor('header') ? { borderTopColor: acColor('header')! } : undefined}
          />
        </div>
      ) : (
        <>
          <div className="mb-6 flex shrink-0 gap-3">
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-blue-100 px-3 py-1 text-sm font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              {t('dashboard.panel.total')} {summary.total}
            </span>
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-green-100 px-3 py-1 text-sm font-medium text-green-700 dark:bg-green-900/30 dark:text-green-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              {t('dashboard.panel.active')} {summary.active}
            </span>
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-sm font-medium text-gray-600 dark:bg-gray-700/30 dark:text-gray-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              {t('dashboard.panel.inactive')} {summary.inactive}
            </span>
          </div>

          {/* 에이전트 리스트 테이블 */}
          {sortedAgents.length === 0 ? (
            <p className="text-sm text-(--color-text-muted)">
              {t('dashboard.agentPanel.empty')}
            </p>
          ) : (
            <div className="min-h-0 flex-1 overflow-y-auto">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-(--color-border-default)">
                      {show('name') && (
                        <SortableHeader
                          label={t('dashboard.col.name')}
                          field="name"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                          accentColor={acColor('table') ?? panelColor}
                        />
                      )}
                      {show('type') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.type')}
                        </th>
                      )}
                      {show('status') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.status')}
                        </th>
                      )}
                      {show('uptime') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.uptime')}
                        </th>
                      )}
                      {show('messages') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.messages')}
                        </th>
                      )}
                      {show('actions') && (
                        <th
                          className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.actions')}
                        </th>
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {sortedAgents.map((agent) => (
                      <tr
                        key={agent.id}
                        className="transition-colors hover:bg-(--color-bg-elevated)"
                      >
                        {show('name') && (
                          <td className="px-4 py-3 text-sm font-medium text-(--color-text-primary)">
                            {agent.name}
                          </td>
                        )}
                        {show('type') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-secondary)">
                            {agent.type}
                          </td>
                        )}
                        {show('status') && (
                          <td className="px-4 py-3">
                            {(() => {
                              if (agent.status === 'error') {
                                return (
                                  <span className="inline-flex items-center text-red-600 dark:text-red-400" title={t('dashboard.error')}>
                                    <AlertTriangle className="h-4 w-4" />
                                  </span>
                                );
                              }
                              const isConnected = agent.connected === true;
                              return isConnected ? (
                                <span className="inline-flex items-center text-green-600 dark:text-green-400" title={t('dashboard.panel.connected')}>
                                  <Activity className="h-4 w-4" />
                                </span>
                              ) : (
                                <span className="inline-flex items-center text-gray-400 dark:text-gray-500" title={t('dashboard.panel.disconnected')}>
                                  <CircleStop className="h-4 w-4" />
                                </span>
                              );
                            })()}
                          </td>
                        )}
                        {show('uptime') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-muted)">
                            {agent.uptime ?? '-'}
                          </td>
                        )}
                        {show('messages') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-muted)">
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

              {/* 더 보기 링크 — 원격은 로컬 `/agents` 로 이탈하므로 숨긴다. */}
              {!remote && agents.length > 10 && (
                <div className="mt-4 text-right">
                  <Link
                    to="/agents"
                    className="inline-flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                  >
                    {t('dashboard.panel.more')}
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
