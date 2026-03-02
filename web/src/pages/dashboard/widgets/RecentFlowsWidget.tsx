// 최근 활성 플로우 목록 위젯.
// updated_at 기준으로 최근 10개 플로우를 표시하고
// 클릭 시 에디터 페이지로 이동한다.

import { Workflow } from 'lucide-react';
import { useNavigate } from 'react-router';

import { formatDate } from '@/lib/utils/format';
import type { FlowInfo } from '@/types/flow';

/** 플로우 상태에 따른 뱃지 스타일 */
const STATUS_BADGE: Record<string, string> = {
  Running: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  Stopped: 'bg-gray-100 text-gray-700 dark:bg-gray-700/30 dark:text-gray-400',
  Error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  Draft: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  Deployed: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
};

/** 상태 한글 레이블 */
const STATUS_LABEL: Record<string, string> = {
  Running: '실행 중',
  Stopped: '중지됨',
  Error: '오류',
  Draft: '초안',
  Deployed: '배포됨',
};

interface RecentFlowsWidgetProps {
  flows: FlowInfo[];
}

/** 최근 업데이트된 플로우 10개를 목록으로 표시하는 위젯 */
export default function RecentFlowsWidget({ flows }: RecentFlowsWidgetProps) {
  const navigate = useNavigate();

  // updated_at 기준 내림차순 정렬 후 최대 10개 선택
  const recentFlows = [...flows]
    .sort((a, b) => {
      const dateA = a.updated_at ?? a.created_at ?? '';
      const dateB = b.updated_at ?? b.created_at ?? '';
      return dateB.localeCompare(dateA);
    })
    .slice(0, 10);

  /** 플로우 클릭 시 에디터로 이동 */
  const handleClick = (flowId: string) => {
    navigate(`/editor/${flowId}`);
  };

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
        최근 플로우
      </h3>

      {recentFlows.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          등록된 플로우가 없습니다.
        </p>
      ) : (
        <ul className="divide-y divide-gray-200 dark:divide-gray-700">
          {recentFlows.map((flow) => {
            const badgeClass = STATUS_BADGE[flow.status] ?? STATUS_BADGE['Draft'];
            const label = STATUS_LABEL[flow.status] ?? flow.status;
            const timeStr = flow.updated_at ?? flow.created_at;

            return (
              <li key={flow.id}>
                <button
                  type="button"
                  onClick={() => handleClick(flow.id)}
                  className="flex w-full items-center gap-3 px-1 py-3 text-left transition-colors hover:bg-gray-50 dark:hover:bg-gray-700/50"
                >
                  <Workflow className="h-4 w-4 shrink-0 text-gray-400" />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium text-gray-900 dark:text-white">
                    {flow.name}
                  </span>
                  <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${badgeClass}`}>
                    {label}
                  </span>
                  {timeStr && (
                    <span className="shrink-0 text-xs text-gray-400 dark:text-gray-500">
                      {formatDate(timeStr, 'relative')}
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
