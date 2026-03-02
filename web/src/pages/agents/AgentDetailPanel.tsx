// 에이전트 상세 통계 패널.
// 행 확장 시 표시되며, 메시지 입출력, 에러 수, 업타임, 연결 상태를 보여준다.

import { useAgentStats } from '@/hooks/useAgent';
import { cn } from '@/lib/utils/cn';

interface AgentDetailPanelProps {
  agentId: string;
}

/** 통계 카드 항목 */
function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400">{label}</p>
      <p className="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{value}</p>
    </div>
  );
}

export default function AgentDetailPanel({ agentId }: AgentDetailPanelProps) {
  const { data: stats, isLoading } = useAgentStats(agentId);

  // 로딩 상태
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

  // 데이터 없음
  if (!stats) {
    return (
      <div className="p-4 text-sm text-gray-500 dark:text-gray-400">
        통계 데이터를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
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
  );
}
