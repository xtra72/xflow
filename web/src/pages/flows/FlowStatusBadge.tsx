// 플로우 상태 뱃지 컴포넌트.
// 플로우의 현재 상태를 색상 코딩된 뱃지로 표시한다.

import { cn } from '@/lib/utils/cn';

interface FlowStatusBadgeProps {
  status: string;
}

/** 상태별 색상 매핑 */
const statusStyles: Record<string, { bg: string; text: string; dot: string }> = {
  Running: {
    bg: 'bg-green-100 dark:bg-green-900/30',
    text: 'text-green-700 dark:text-green-400',
    dot: 'bg-green-500',
  },
  Stopped: {
    bg: 'bg-gray-100 dark:bg-gray-700',
    text: 'text-gray-500 dark:text-gray-400',
    dot: 'bg-gray-400',
  },
  Error: {
    bg: 'bg-red-100 dark:bg-red-900/30',
    text: 'text-red-700 dark:text-red-400',
    dot: 'bg-red-500',
  },
  Draft: {
    bg: 'bg-blue-100 dark:bg-blue-900/30',
    text: 'text-blue-700 dark:text-blue-400',
    dot: 'bg-blue-500',
  },
  Deployed: {
    bg: 'bg-yellow-100 dark:bg-yellow-900/30',
    text: 'text-yellow-700 dark:text-yellow-400',
    dot: 'bg-yellow-500',
  },
};

/** 기본 스타일 (알 수 없는 상태) */
const defaultStyle = {
  bg: 'bg-gray-100 dark:bg-gray-700',
  text: 'text-gray-500 dark:text-gray-400',
  dot: 'bg-gray-400',
};

/**
 * 플로우 상태 뱃지.
 * 상태에 따라 색상이 다른 뱃지를 렌더링한다.
 */
export default function FlowStatusBadge({ status }: FlowStatusBadgeProps) {
  const style = statusStyles[status] ?? defaultStyle;

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        style.bg,
        style.text,
      )}
    >
      <span className={cn('h-1.5 w-1.5 rounded-full', style.dot)} />
      {status}
    </span>
  );
}
