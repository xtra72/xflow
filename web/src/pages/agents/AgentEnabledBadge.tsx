// 에이전트 활성화 상태 배지 (SPEC-AGENT-005).
// enabled=false 인 경우에만 회색 "비활성화" 배지를 표시한다.
// enabled=true 또는 undefined (구버전 서버) 인 경우 아무 것도 렌더링하지 않아
// 기존 UI 에 노이즈를 주지 않는다.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface AgentEnabledBadgeProps {
  enabled?: boolean;
  className?: string;
}

export default function AgentEnabledBadge({ enabled, className }: AgentEnabledBadgeProps) {
  const { t } = useTranslation();

  // 기본값(true) 또는 미지정 상태는 배지를 숨긴다.
  if (enabled !== false) {
    return null;
  }

  return (
    <span
      title={t('agents.badge.disabledTooltip')}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        'bg-(--color-bg-sunken) text-(--color-text-secondary)',
        className,
      )}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-(--color-status-stopped)" />
      {t('agents.badge.disabled')}
    </span>
  );
}
