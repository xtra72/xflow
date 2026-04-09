// 에이전트 활성화 상태 배지 (SPEC-AGENT-005).
// enabled=false 인 경우에만 회색 "비활성화" 배지를 표시한다.
// enabled=true 또는 undefined (구버전 서버) 인 경우 아무 것도 렌더링하지 않아
// 기존 UI 에 노이즈를 주지 않는다.

import { cn } from '@/lib/utils/cn';

interface AgentEnabledBadgeProps {
  enabled?: boolean;
  className?: string;
}

export default function AgentEnabledBadge({ enabled, className }: AgentEnabledBadgeProps) {
  // 기본값(true) 또는 미지정 상태는 배지를 숨긴다.
  if (enabled !== false) {
    return null;
  }

  return (
    <span
      title="이 에이전트는 비활성화 상태이며, 데몬 재시작 시 자동 시작되지 않습니다."
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
        className,
      )}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-gray-400 dark:bg-gray-500" />
      비활성화
    </span>
  );
}
