// 디바이스 온라인/오프라인 상태 배지 컴포넌트.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface DeviceStatusBadgeProps {
  online: boolean;
}

/** 디바이스 온라인/오프라인 상태 배지 */
export default function DeviceStatusBadge({ online }: DeviceStatusBadgeProps) {
  const { t } = useTranslation();
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        online
          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
          : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
      )}
    >
      <span
        className={cn('h-1.5 w-1.5 rounded-full', online ? 'bg-green-500' : 'bg-gray-400')}
        aria-hidden="true"
      />
      {online ? t('devices.status.online') : t('devices.status.offline')}
    </span>
  );
}
