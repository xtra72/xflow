// 노드 등록 상태 배지 (SPEC-REMOTE-001 M5, G01).
//
// pending/approved/rejected/revoked 4개 상태를 색상 배지로 표시한다.
// 알 수 없는 상태 값은 중립(회색) 배지로 그대로 노출한다.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { RegistrationStatus } from '@/types/remote';

interface NodeStatusBadgeProps {
  /** 등록 상태 (백엔드 문자열). 알 수 없는 값일 수 있다. */
  status: string;
  /** 추가 className. */
  className?: string;
}

/** 알려진 상태별 배지 스타일 (라이트/다크). */
const STATUS_STYLE: Record<RegistrationStatus, string> = {
  pending:
    'border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
  approved:
    'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200',
  rejected:
    'border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-200',
  revoked:
    'border-gray-200 bg-gray-100 text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300',
};

/** 중립(알 수 없는 상태) 스타일. */
const NEUTRAL_STYLE =
  'border-gray-200 bg-gray-100 text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300';

/** status 가 알려진 RegistrationStatus 인지 판별한다. */
function isKnownStatus(status: string): status is RegistrationStatus {
  return status in STATUS_STYLE;
}

/**
 * 노드 등록 상태를 색상 배지로 표시한다.
 * i18n 키 `remote.status.<status>` 로 번역하며, 미정의 상태는 원문 그대로 노출한다.
 */
export function NodeStatusBadge({
  status,
  className,
}: NodeStatusBadgeProps): React.JSX.Element {
  const { t } = useTranslation();
  const known = isKnownStatus(status);
  const label = known ? t(`remote.status.${status}`) : status;
  const style = known ? STATUS_STYLE[status] : NEUTRAL_STYLE;

  return (
    <span
      data-testid="node-status-badge"
      data-status={status}
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium',
        style,
        className,
      )}
    >
      {label}
    </span>
  );
}
