// 시스템 상태 요약 위젯.
// 플로우 상태별(Running/Stopped/Error) 개수를 표시한다.

import { Activity, AlertTriangle, CircleStop, FileText, Rocket } from 'lucide-react';

import type { FlowInfo } from '@/types/flow';

/** 상태별 색상 및 아이콘 매핑 */
const STATUS_CONFIG: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  running: {
    label: '실행 중',
    color: 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30',
    icon: <Activity className="h-5 w-5" />,
  },
  stopped: {
    label: '중지됨',
    color: 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-700/30',
    icon: <CircleStop className="h-5 w-5" />,
  },
  error: {
    label: '오류',
    color: 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-900/30',
    icon: <AlertTriangle className="h-5 w-5" />,
  },
  stored: {
    label: '저장됨',
    color: 'text-blue-600 bg-blue-100 dark:text-blue-400 dark:bg-blue-900/30',
    icon: <FileText className="h-5 w-5" />,
  },
  loaded: {
    label: '탑재됨',
    color: 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-900/30',
    icon: <Rocket className="h-5 w-5" />,
  },
};

interface SystemStatusWidgetProps {
  flows: FlowInfo[];
}

/** 플로우 상태별 집계를 카드 형태로 보여주는 위젯 */
export default function SystemStatusWidget({ flows }: SystemStatusWidgetProps) {
  // 상태별 플로우 수를 집계한다
  const statusCounts: Record<string, number> = {};
  for (const flow of flows) {
    const status = flow.status ?? 'stored';
    statusCounts[status] = (statusCounts[status] ?? 0) + 1;
  }

  // 표시할 주요 상태 목록
  const displayStatuses = ['running', 'stopped', 'error', 'stored', 'loaded'];

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
        시스템 상태
      </h3>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        {displayStatuses.map((status) => {
          const config = STATUS_CONFIG[status];
          const count = statusCounts[status] ?? 0;
          if (!config) return null;

          return (
            <div
              key={status}
              className="flex flex-col items-center rounded-md border border-gray-200 p-3 dark:border-gray-700"
            >
              <div className={`mb-1 rounded-full p-2 ${config.color}`}>
                {config.icon}
              </div>
              <span className="text-2xl font-bold text-gray-900 dark:text-white">
                {count}
              </span>
              <span className="text-xs text-gray-500 dark:text-gray-400">
                {config.label}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
