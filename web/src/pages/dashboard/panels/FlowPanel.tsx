// 플로우 패널 컴포넌트.
// 상단에 상태별 요약 뱃지, 하단에 플로우 리스트 테이블을 표시한다.

import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  CircleStop,
  FileText,
  GitBranch,
  Pause,
  Play,
  Rocket,
  RotateCcw,
} from 'lucide-react';
import { Link } from 'react-router';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useFlowActionsTarget } from '@/hooks/useResourceActions';
import { useFlowsTarget } from '@/hooks/useResourceTargets';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { usePanelTitleVisible } from '../panelChromeContext';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { formatDate } from '@/lib/utils/format';
import { startFlow, stopFlow, restartFlow } from '@/services/api/flowService';
import {
  type FlowColumnKey,
  type PanelConfig,
} from '@/stores/uiStore';
import type { FlowInfo } from '@/types/flow';

/** 상태별 색상 및 아이콘 매핑 (label 은 i18n 키, 렌더 시 t(key) 로 변환) */
const STATUS_CONFIG: Record<string, { labelKey: string; color: string; icon: React.ReactNode }> = {
  running: {
    labelKey: 'dashboard.running',
    color: 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30',
    icon: <Activity className="h-4 w-4" />,
  },
  stopped: {
    labelKey: 'dashboard.stopped',
    color: 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-700/30',
    icon: <CircleStop className="h-4 w-4" />,
  },
  error: {
    labelKey: 'dashboard.error',
    color: 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-900/30',
    icon: <AlertTriangle className="h-4 w-4" />,
  },
  stored: {
    labelKey: 'dashboard.stored',
    color: 'text-blue-600 bg-blue-100 dark:text-blue-400 dark:bg-blue-900/30',
    icon: <FileText className="h-4 w-4" />,
  },
  loaded: {
    labelKey: 'dashboard.loaded',
    color: 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-900/30',
    icon: <Rocket className="h-4 w-4" />,
  },
};

/** 표시할 주요 상태 목록 */
const DISPLAY_STATUSES = ['running', 'stopped', 'error', 'stored', 'loaded'] as const;

/** 전체 FlowColumnKey 기본 목록 */
const ALL_FLOW_COLUMNS: FlowColumnKey[] = ['name', 'status', 'node_count', 'updated_at', 'actions'];

interface FlowPanelProps {
  flows: FlowInfo[];
  /** 패널 설정 (멀티-대시보드 모델에서 전달) */
  panelConfig?: PanelConfig;
}

/** 플로우 상태 요약 + 플로우 리스트 테이블 패널 */
export default function FlowPanel({ flows: localFlows, panelConfig }: FlowPanelProps) {
  const showTitle = usePanelTitleVisible();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04): 원격이면 노드 미러 목록을
  // 소스로 쓴다(useFlowsTarget — 그룹 J/E 재사용). 로컬은 prop 의 flows 를 그대로
  // 사용해 회귀 없이 동일 렌더한다.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const remoteFlows = useFlowsTarget(target);
  const flows = useMemo<FlowInfo[]>(
    () => (remote ? (remoteFlows.data?.data ?? []) : localFlows),
    [remote, remoteFlows.data, localFlows],
  );

  // 패널 설정 (멀티-대시보드 패널 config에서 읽기)
  const title = panelConfig?.title ?? t('dashboard.panelTypes.flows');
  const visibleColumns = useMemo(
    () => (panelConfig?.config?.visibleColumns as FlowColumnKey[]) ?? [...ALL_FLOW_COLUMNS],
    [panelConfig?.config?.visibleColumns],
  );
  const panelColor = panelConfig?.config?.panelColor as string | undefined;
  const accentElements = (panelConfig?.config?.accentElements as Record<string, string | boolean>) ?? {};

  /** accentElements 그룹별 유효 색상 */
  const acColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };

  // 숨겨진 컬럼으로 정렬 중이면 기본(name)으로 fallback
  useEffect(() => {
    if (!visibleColumns.includes(sort.field as FlowColumnKey)) {
      setSort({ field: 'name', direction: 'asc' });
    }
  }, [visibleColumns, sort.field]);

  const show = (key: FlowColumnKey) => visibleColumns.includes(key);

  // 상태별 플로우 수 집계
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const flow of flows) {
      const status = flow.status ?? 'stored';
      counts[status] = (counts[status] ?? 0) + 1;
    }
    return counts;
  }, [flows]);

  // 정렬된 플로우 목록 (최대 10개)
  const sortedFlows = useMemo(() => {
    const sorted = [...flows].sort((a, b) => {
      let valA: string | number;
      let valB: string | number;

      switch (sort.field) {
        case 'name':
          valA = a.name.toLowerCase();
          valB = b.name.toLowerCase();
          break;
        case 'status':
          valA = a.status;
          valB = b.status;
          break;
        case 'node_count':
          valA = a.node_count;
          valB = b.node_count;
          break;
        case 'updated_at':
          valA = a.updated_at ?? a.created_at ?? '';
          valB = b.updated_at ?? b.created_at ?? '';
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
  }, [flows, sort]);

  /** 정렬 변경 핸들러 */
  const handleSort = (field: string) => {
    setSort((prev) => ({
      field,
      direction: prev.field === field && prev.direction === 'asc' ? 'desc' : 'asc',
    }));
  };

  // 플로우 액션 뮤테이션(로컬). 원격은 그룹 D 명령으로 라우팅한다(REQ-L12/J12).
  const startMutation = useMutation({
    mutationFn: startFlow,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['flows'] }),
  });

  const stopMutation = useMutation({
    mutationFn: stopFlow,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['flows'] }),
  });

  const restartMutation = useMutation({
    mutationFn: restartFlow,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['flows'] }),
  });

  // 원격 라이프사이클: 그룹 D 명령(REQ-L12). 노드 ready 가 아니면 게이팅된다.
  const remoteActions = useFlowActionsTarget(target);
  const gating = useTargetGating(target);

  /** 플로우 상태에 따른 액션 버튼 렌더링 */
  const renderActionButton = (flow: FlowInfo) => {
    // 원격: 그룹 D 명령 + 게이팅(노드 승인∧온라인 ∧ 자원 online). 미지원 액션
    // (restart)은 supports=false 로 비활성. 로컬은 기존 동작 불변.
    if (remote) {
      const action =
        flow.status === 'running' ? 'stop' : flow.status === 'error' ? 'restart' : 'start';
      const Icon = action === 'stop' ? Pause : action === 'restart' ? RotateCcw : Play;
      const labelSuffix = action === 'stop' ? t('dashboard.action.stop') : action === 'restart' ? t('dashboard.action.restart') : t('dashboard.action.start');
      const supported = remoteActions.supports(action);
      const canControl = gating.canControl(
        flow.status === 'running' || flow.status === 'error' || flow.status === 'stopped',
      );
      const disabled = !supported || !canControl || remoteActions.pending[action] === true;
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            void remoteActions.perform(action, flow.id).catch(() => {});
          }}
          disabled={disabled}
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${flow.name} ${labelSuffix}`}
        >
          <Icon className="h-4 w-4" />
        </button>
      );
    }

    const isPending =
      startMutation.isPending || stopMutation.isPending || restartMutation.isPending;

    if (flow.status === 'running') {
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            stopMutation.mutate(flow.id);
          }}
          disabled={isPending}
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${flow.name} ${t('dashboard.action.stop')}`}
        >
          <Pause className="h-4 w-4" />
        </button>
      );
    }

    if (flow.status === 'error') {
      return (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            restartMutation.mutate(flow.id);
          }}
          disabled={isPending}
          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
          aria-label={`${flow.name} ${t('dashboard.action.restart')}`}
        >
          <RotateCcw className="h-4 w-4" />
        </button>
      );
    }

    // stopped, stored, loaded
    return (
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          startMutation.mutate(flow.id);
        }}
        disabled={isPending}
        className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
        aria-label={`${flow.name} ${t('dashboard.action.start')}`}
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
            <GitBranch className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <h3
              className="truncate text-lg font-semibold text-(--color-text-primary)"
              style={acColor('header') ? { color: acColor('header')! } : undefined}
            >
              {title}
            </h3>
          </div>
        </div>
      )}

      {/* 상태별 요약 */}
      <div className="mb-6 flex shrink-0 flex-wrap gap-2">
        {DISPLAY_STATUSES.map((status) => {
          const config = STATUS_CONFIG[status];
          const count = statusCounts[status] ?? 0;
          if (!config) return null;

          return (
            <span
              key={status}
              className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium ${config.color}`}
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              {config.icon}
              {t(config.labelKey)} {count}
            </span>
          );
        })}
      </div>

      {/* 플로우 리스트 테이블 */}
      {sortedFlows.length === 0 ? (
        <p className="text-sm text-(--color-text-muted)">
          {t('dashboard.flowPanel.empty')}
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
                  {show('status') && (
                    <th
                      className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                      style={acColor('table') ? { color: acColor('table')! } : undefined}
                    >
                      {t('dashboard.col.status')}
                    </th>
                  )}
                  {show('node_count') && (
                    <th
                      className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                      style={acColor('table') ? { color: acColor('table')! } : undefined}
                    >
                      {t('dashboard.col.nodeCount')}
                    </th>
                  )}
                  {show('updated_at') && (
                    <th
                      className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                      style={acColor('table') ? { color: acColor('table')! } : undefined}
                    >
                      {t('dashboard.col.uptime')}
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
                {sortedFlows.map((flow) => {
                  const timeStr = flow.updated_at ?? flow.created_at;

                  return (
                    <tr
                      key={flow.id}
                      className="transition-colors hover:bg-(--color-bg-elevated)"
                    >
                      {show('name') && (
                        <td className="px-4 py-3">
                          {remote ? (
                            // 원격: 에디터 딥링크는 노드 컨텍스트 밖이므로 단순 텍스트로
                            // 표시한다(편집은 노드 대시보드 플로우 서브탭 — REQ-L12).
                            <span className="text-sm font-medium text-(--color-text-primary)">
                              {flow.name}
                            </span>
                          ) : (
                            <Link
                              to={`/editor/${flow.id}`}
                              className="text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                            >
                              {flow.name}
                            </Link>
                          )}
                        </td>
                      )}
                      {show('status') && (
                        <td className="px-4 py-3">
                          {(() => {
                            const cfg = STATUS_CONFIG[flow.status];
                            return cfg ? (
                              <span className={`inline-flex items-center ${cfg.color.split(' ').filter(c => c.startsWith('text-')).join(' ')}`} title={t(cfg.labelKey)}>
                                {cfg.icon}
                              </span>
                            ) : (
                              <span className="text-sm text-(--color-text-muted)">{flow.status}</span>
                            );
                          })()}
                        </td>
                      )}
                      {show('node_count') && (
                        <td className="px-4 py-3 text-sm text-(--color-text-secondary)">
                          {flow.node_count}
                        </td>
                      )}
                      {show('updated_at') && (
                        <td className="px-4 py-3 text-sm text-(--color-text-muted)">
                          {timeStr ? formatDate(timeStr, 'relative') : '-'}
                        </td>
                      )}
                      {show('actions') && (
                        <td className="px-4 py-3 text-right">
                          {renderActionButton(flow)}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* 더 보기 링크 — 원격은 로컬 `/flows` 로 이탈하므로 숨긴다(노드 서브탭 사용). */}
          {!remote && flows.length > 10 && (
            <div className="mt-4 text-right">
              <Link
                to="/flows"
                className="inline-flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
              >
                {t('dashboard.panel.more')}
                <ArrowRight className="h-4 w-4" />
              </Link>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
