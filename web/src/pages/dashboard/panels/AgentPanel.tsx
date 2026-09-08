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
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';
import {
  readListPanelStyle,
  readSummaryItems,
  readSummaryTileFont,
  resolveSummaryTileStyle,
  type SummaryItem,
} from './listPanelStyle';

/** 타일별 기본 색 — 배경을 직접 정하지 않았을 때만 쓰인다. */
const SUMMARY_TILE_CLASS: Record<SummaryItem, string> = {
  total: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  active: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  inactive: 'bg-(--color-bg-sunken) text-(--color-text-secondary)',
};

const SUMMARY_TILE_LABEL_KEY: Record<SummaryItem, string> = {
  total: 'dashboard.panel.total',
  active: 'dashboard.panel.active',
  inactive: 'dashboard.panel.inactive',
};

import { cn } from '@/lib/utils/cn';

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
  const titleStyle = usePanelTitleStyle();
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
  // 자리별 디자인(테이블 헤더·요소·요약 배지) 해석은 순수 모듈이 맡는다.
  const design = useMemo(() => readListPanelStyle(panelConfig?.config), [panelConfig?.config]);

  // 숨겨진 컬럼으로 정렬 중이면 기본(name)으로 fallback
  useEffect(() => {
    if (!visibleColumns.includes(sort.field as AgentColumnKey)) {
      setSort({ field: 'name', direction: 'asc' });
    }
  }, [visibleColumns, sort.field]);

  const show = (key: AgentColumnKey) => visibleColumns.includes(key);

  // 그릴 요약 타일과 순서. 미설정이면 셋 다 기본 순서로.
  const summaryItems = useMemo(
    () => readSummaryItems(panelConfig?.config),
    [panelConfig?.config],
  );

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
              // 타이틀 모양은 "타이틀 디자인" 한 곳이 정한다(플로우 현황과 같은 규칙).
              style={titleStyle}
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
          />
        </div>
      ) : (
        <>
          {design.showSummaryBadges && summaryItems.length > 0 && (
          <div className="mb-6 flex shrink-0 gap-3" data-testid="agent-summary-badges">
            {/*
              고른 타일만 고른 순서로 그린다. 타일마다 디자인을 따로 정할 수 있고,
              정하지 않은 것은 공통 배지 설정을 따른다.
            */}
            {summaryItems.map((item) => {
              const tile = resolveSummaryTileStyle(
                design.badgeStyle,
                readSummaryTileFont(panelConfig?.config, item),
              );
              return (
                <span
                  key={item}
                  data-testid={`agent-summary-${item}`}
                  className={cn(
                    'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium',
                    // 배경을 직접 정하지 않았으면 상태별 기본 색이 그대로 산다 —
                    // 색으로 상태를 읽던 단서를 뺏지 않는다.
                    !tile.hasOwnBackground && SUMMARY_TILE_CLASS[item],
                  )}
                  style={tile.style}
                >
                  {t(SUMMARY_TILE_LABEL_KEY[item])} {summary[item]}
                </span>
              );
            })}
          </div>
          )}

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
                          accentColor={design.headerAccent}
                        />
                      )}
                      {show('type') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={design.headerStyle}
                        >
                          {t('dashboard.col.type')}
                        </th>
                      )}
                      {show('status') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={design.headerStyle}
                        >
                          {t('dashboard.col.status')}
                        </th>
                      )}
                      {show('uptime') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={design.headerStyle}
                        >
                          {t('dashboard.col.uptime')}
                        </th>
                      )}
                      {show('messages') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={design.headerStyle}
                        >
                          {t('dashboard.col.messages')}
                        </th>
                      )}
                      {show('actions') && (
                        <th
                          className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={design.headerStyle}
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
                        // 글꼴·크기·굵기는 여기서 상속된다. 색은 안쪽 클래스가 이기므로
                        // 글자를 직접 담은 자리에 따로 건다(아래).
                        style={design.cellStyle}
                      >
                        {show('name') && (
                          <td
                            className="px-4 py-3 text-sm font-medium text-(--color-text-primary)"
                            style={design.cellStyle}
                          >
                            {agent.name}
                          </td>
                        )}
                        {show('type') && (
                          <td
                            className="px-4 py-3 text-sm text-(--color-text-secondary)"
                            style={design.cellStyle}
                          >
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
                                <span className="inline-flex items-center text-(--color-text-muted)" title={t('dashboard.panel.disconnected')}>
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
