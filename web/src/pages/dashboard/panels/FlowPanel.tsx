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
  Pause,
  Play,
  Rocket,
  RotateCcw,
} from 'lucide-react';
import { Link } from 'react-router';

import PanelSettingsDropdown, { type ColumnOption } from '@/components/common/PanelSettingsDropdown';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { formatDate } from '@/lib/utils/format';
import FlowStatusBadge from '@/pages/flows/FlowStatusBadge';
import { startFlow, stopFlow, restartFlow } from '@/services/api/flowService';
import {
  useUIStore,
  type FlowColumnKey,
} from '@/stores/uiStore';
import type { FlowInfo } from '@/types/flow';

/** 상태별 색상 및 아이콘 매핑 */
const STATUS_CONFIG: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  running: {
    label: '실행 중',
    color: 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30',
    icon: <Activity className="h-4 w-4" />,
  },
  stopped: {
    label: '중지됨',
    color: 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-700/30',
    icon: <CircleStop className="h-4 w-4" />,
  },
  error: {
    label: '오류',
    color: 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-900/30',
    icon: <AlertTriangle className="h-4 w-4" />,
  },
  stored: {
    label: '저장됨',
    color: 'text-blue-600 bg-blue-100 dark:text-blue-400 dark:bg-blue-900/30',
    icon: <FileText className="h-4 w-4" />,
  },
  loaded: {
    label: '탑재됨',
    color: 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-900/30',
    icon: <Rocket className="h-4 w-4" />,
  },
};

/** 표시할 주요 상태 목록 */
const DISPLAY_STATUSES = ['running', 'stopped', 'error', 'stored', 'loaded'] as const;

/** 컬럼 옵션 (설정 드롭다운용) */
const FLOW_COLUMN_OPTIONS: ColumnOption<FlowColumnKey>[] = [
  { key: 'name', label: '이름' },
  { key: 'status', label: '상태' },
  { key: 'node_count', label: '노드 수' },
  { key: 'updated_at', label: '업타임' },
  { key: 'actions', label: '액션' },
];

interface FlowPanelProps {
  flows: FlowInfo[];
}

/** 플로우 상태 요약 + 플로우 리스트 테이블 패널 */
export default function FlowPanel({ flows }: FlowPanelProps) {
  const queryClient = useQueryClient();
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 패널 설정
  const title = useUIStore((s) => s.flowPanelTitle);
  const visibleColumns = useUIStore((s) => s.flowVisibleColumns);
  const setTitle = useUIStore((s) => s.setFlowPanelTitle);
  const setVisibleColumns = useUIStore((s) => s.setFlowVisibleColumns);

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

  // 플로우 액션 뮤테이션
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

  /** 플로우 상태에 따른 액션 버튼 렌더링 */
  const renderActionButton = (flow: FlowInfo) => {
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
          className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
          aria-label={`${flow.name} 중지`}
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
          className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
          aria-label={`${flow.name} 재시작`}
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
        className="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
        aria-label={`${flow.name} 시작`}
      >
        <Play className="h-4 w-4" />
      </button>
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      {/* 헤더: 타이틀 + 설정 */}
      <div className="mb-4 flex shrink-0 items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          {title}
        </h3>
        <PanelSettingsDropdown
          title={title}
          onTitleChange={setTitle}
          columns={FLOW_COLUMN_OPTIONS}
          visibleColumns={visibleColumns}
          onColumnsChange={setVisibleColumns}
        />
      </div>

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
            >
              {config.icon}
              {config.label} {count}
            </span>
          );
        })}
      </div>

      {/* 플로우 리스트 테이블 */}
      {sortedFlows.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          등록된 플로우가 없습니다.
        </p>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto">
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
                  {show('status') && (
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      상태
                    </th>
                  )}
                  {show('node_count') && (
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      노드 수
                    </th>
                  )}
                  {show('updated_at') && (
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      업타임
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
                {sortedFlows.map((flow) => {
                  const timeStr = flow.updated_at ?? flow.created_at;

                  return (
                    <tr
                      key={flow.id}
                      className="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700/50"
                    >
                      {show('name') && (
                        <td className="px-4 py-3">
                          <Link
                            to={`/editor/${flow.id}`}
                            className="text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                          >
                            {flow.name}
                          </Link>
                        </td>
                      )}
                      {show('status') && (
                        <td className="px-4 py-3">
                          <FlowStatusBadge status={flow.status} />
                        </td>
                      )}
                      {show('node_count') && (
                        <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                          {flow.node_count}
                        </td>
                      )}
                      {show('updated_at') && (
                        <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
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

          {/* 더 보기 링크 */}
          {flows.length > 10 && (
            <div className="mt-4 text-right">
              <Link
                to="/flows"
                className="inline-flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
              >
                더 보기
                <ArrowRight className="h-4 w-4" />
              </Link>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
