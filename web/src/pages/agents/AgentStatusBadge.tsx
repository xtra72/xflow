// 에이전트 연결 상태 배지.
// connected 여부에 따라 초록색(연결됨) 또는 회색(연결 해제) 배지를 표시한다.

import { cn } from '@/lib/utils/cn';

interface AgentStatusBadgeProps {
  connected?: boolean;
}

export default function AgentStatusBadge({ connected }: AgentStatusBadgeProps) {
  const isConnected = connected === true;

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        isConnected
          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
          : 'bg-(--color-bg-sunken) text-(--color-text-muted)',
      )}
    >
      <span
        className={cn(
          'h-1.5 w-1.5 rounded-full',
          isConnected ? 'bg-green-500 dark:bg-green-400' : 'bg-(--color-status-stopped)',
        )}
      />
      {isConnected ? '연결됨' : '연결 해제'}
    </span>
  );
}
