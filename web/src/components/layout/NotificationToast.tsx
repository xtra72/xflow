// 알림 토스트 컴포넌트.
// uiStore의 notifications를 화면 우측 상단에 표시하며, 3초 후 자동으로 사라진다.

import { useEffect } from 'react';
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useUIStore, type Notification } from '@/stores/uiStore';

const ICON_MAP = {
  info: Info,
  success: CheckCircle2,
  warning: AlertTriangle,
  error: XCircle,
} as const;

const STYLE_MAP: Record<Notification['type'], string> = {
  info: 'border-blue-200 bg-blue-50 text-blue-800 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-200',
  success:
    'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200',
  warning:
    'border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
  error:
    'border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-200',
};

const AUTO_DISMISS_MS = 3000;

export function NotificationToast() {
  const notifications = useUIStore((s) => s.notifications);
  const dismissNotification = useUIStore((s) => s.dismissNotification);

  // 최근 5개만 표시
  const visible = notifications.slice(-5);

  return (
    <div className="pointer-events-none fixed right-4 top-4 z-50 flex flex-col gap-2">
      {visible.map((n) => (
        <ToastItem key={n.id} notification={n} onDismiss={dismissNotification} />
      ))}
    </div>
  );
}

function ToastItem({
  notification,
  onDismiss,
}: {
  notification: Notification;
  onDismiss: (id: string) => void;
}) {
  const Icon = ICON_MAP[notification.type];

  useEffect(() => {
    const timer = setTimeout(() => onDismiss(notification.id), AUTO_DISMISS_MS);
    return () => clearTimeout(timer);
  }, [notification.id, onDismiss]);

  return (
    <div
      className={cn(
        'pointer-events-auto flex items-start gap-2 rounded-lg border px-3 py-2 shadow-lg',
        'animate-in slide-in-from-right-5 fade-in duration-200',
        'min-w-[260px] max-w-[360px]',
        STYLE_MAP[notification.type],
      )}
    >
      <Icon className="mt-0.5 h-4 w-4 shrink-0" />
      <span className="flex-1 text-sm">{notification.message}</span>
      <button
        type="button"
        onClick={() => onDismiss(notification.id)}
        className="shrink-0 rounded p-0.5 opacity-60 hover:opacity-100"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
