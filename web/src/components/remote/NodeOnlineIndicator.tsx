// 노드/자원의 online/offline 상태 인디케이터 (SPEC-REMOTE-001 M5, G01).
//
// 상태 점(dot) + 라벨로 구성된다. 접근성을 위해 role="status" 와
// aria-label 을 부여하며, 색상에만 의존하지 않도록 텍스트 라벨을 함께 노출한다.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface NodeOnlineIndicatorProps {
  /** 라이브 연결 상태. */
  online: boolean;
  /** 라벨 텍스트 표시 여부 (false 면 점만 표시). 기본 true. */
  showLabel?: boolean;
  /** 추가 className. */
  className?: string;
}

/**
 * online/offline 을 점 + 라벨로 표시한다.
 * online=true → 초록, false → 회색.
 */
export function NodeOnlineIndicator({
  online,
  showLabel = true,
  className,
}: NodeOnlineIndicatorProps): React.JSX.Element {
  const { t } = useTranslation();
  const label = online ? t('remote.online') : t('remote.offline');

  return (
    <span
      role="status"
      aria-label={label}
      data-testid="node-online-indicator"
      data-online={online}
      className={cn('inline-flex items-center gap-1.5 text-sm', className)}
    >
      <span
        aria-hidden="true"
        className={cn(
          'h-2 w-2 shrink-0 rounded-full',
          online
            ? 'bg-green-500 dark:bg-green-400'
            : 'bg-gray-400 dark:bg-gray-500',
        )}
      />
      {showLabel && (
        <span
          className={
            online
              ? 'text-green-700 dark:text-green-400'
              : 'text-(--color-text-muted)'
          }
        >
          {label}
        </span>
      )}
    </span>
  );
}
