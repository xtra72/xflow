// 시스템 이벤트 타임라인 컴포넌트.
// 상태 변경, 배포, 에러 등 시스템 이벤트를 시간순(최신 먼저)으로 표시한다.

import {
  AlertTriangle,
  CheckCircle,
  Info,
  Rocket,
  Server,
} from 'lucide-react';

import { formatDate } from '@/lib/utils/format';

/** 이벤트 유형 */
export type EventType = 'status_change' | 'deployment' | 'error' | 'system';

/** 단일 시스템 이벤트 */
export interface SystemEvent {
  id: string;
  type: EventType;
  message: string;
  timestamp: string;
  details?: string;
}

// 최대 표시 이벤트 수
const MAX_EVENTS = 100;

/** 이벤트 유형별 아이콘과 색상 설정 */
const EVENT_CONFIG: Record<
  EventType,
  { icon: React.ComponentType<{ className?: string }>; color: string; bg: string }
> = {
  status_change: {
    icon: CheckCircle,
    color: 'text-blue-500',
    bg: 'bg-blue-100 dark:bg-blue-900/30',
  },
  deployment: {
    icon: Rocket,
    color: 'text-green-500',
    bg: 'bg-green-100 dark:bg-green-900/30',
  },
  error: {
    icon: AlertTriangle,
    color: 'text-red-500',
    bg: 'bg-red-100 dark:bg-red-900/30',
  },
  system: {
    icon: Server,
    color: 'text-gray-500',
    bg: 'bg-gray-100 dark:bg-gray-700/30',
  },
};

interface EventTimelineProps {
  events: SystemEvent[];
}

/** 시스템 이벤트 타임라인 */
export default function EventTimeline({ events }: EventTimelineProps) {
  // 최신순 정렬, 최대 100개 표시
  const sorted = [...events]
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
    .slice(0, MAX_EVENTS);

  if (sorted.length === 0) {
    return (
      <div className="bg-(--color-bg-surface) rounded-lg shadow p-8 text-center">
        <Info className="mx-auto h-8 w-8 text-gray-400 mb-2" />
        <p className="text-sm text-(--color-text-muted)">
          아직 수신된 이벤트가 없습니다.
        </p>
      </div>
    );
  }

  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow p-4">
      <div className="space-y-0">
        {sorted.map((event, idx) => {
          const config = EVENT_CONFIG[event.type] ?? EVENT_CONFIG.system;
          const Icon = config.icon;
          const isLast = idx === sorted.length - 1;

          return (
            <div key={event.id} className="flex gap-3">
              {/* 타임라인 라인 + 아이콘 */}
              <div className="flex flex-col items-center">
                <div
                  className={`flex items-center justify-center w-8 h-8 rounded-full shrink-0 ${config.bg}`}
                >
                  <Icon className={`w-4 h-4 ${config.color}`} />
                </div>
                {!isLast && (
                  <div className="w-px flex-1 bg-(--color-border-default) my-1" />
                )}
              </div>

              {/* 이벤트 내용 */}
              <div className={`pb-4 ${isLast ? '' : ''}`}>
                <div className="flex items-baseline gap-2">
                  <span className="text-sm font-medium text-(--color-text-primary)">
                    {event.message}
                  </span>
                </div>
                <p className="text-xs text-(--color-text-muted) mt-0.5">
                  {formatDate(event.timestamp, 'long')}
                </p>
                {event.details && (
                  <p className="text-xs text-(--color-text-secondary) mt-1 bg-(--color-bg-sunken) rounded px-2 py-1">
                    {event.details}
                  </p>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
