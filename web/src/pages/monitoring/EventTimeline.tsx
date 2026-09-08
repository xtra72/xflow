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
import { useTranslation } from '@/lib/i18n';

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
    color: 'text-(--color-text-muted)',
    bg: 'bg-(--color-bg-sunken)',
  },
};

interface EventTimelineProps {
  events: SystemEvent[];
  /**
   * 헤더 제목. 지정하면 제목 + 건수 헤더가 붙는다.
   * 모니터링 보드가 유형별 타임라인을 여러 개 배치할 때 서로를 구분하기 위해 쓴다.
   */
  title?: string;
}

/** 시스템 이벤트 타임라인 */
export default function EventTimeline({ events, title }: EventTimelineProps) {
  const { t } = useTranslation();
  // 최신순 정렬, 최대 100개 표시
  const sorted = [...events]
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
    .slice(0, MAX_EVENTS);

  if (sorted.length === 0) {
    return (
      <div className="bg-(--color-bg-surface) rounded-lg shadow">
        {title && <TimelineHeader title={title} count={0} unit={t('monitoring.countUnit')} />}
        <div className="p-8 text-center">
          <Info className="mx-auto h-8 w-8 text-(--color-text-muted) mb-2" />
          <p className="text-sm text-(--color-text-muted)">{t('monitoring.noEvents')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow">
      {title && (
        <TimelineHeader title={title} count={events.length} unit={t('monitoring.countUnit')} />
      )}
      <div className="space-y-0 p-4">
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
                    {/* 메시지 없는 이벤트는 스트림이 빈 문자열로 담아 두므로 여기서 채운다. */}
                    {event.message || t('monitoring.systemEvent')}
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

/** 타임라인 헤더 (제목 + 건수) — title 이 주어진 경우에만 렌더된다. */
function TimelineHeader({
  title,
  count,
  unit,
}: {
  title: string;
  count: number;
  unit: string;
}) {
  return (
    <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
      <h3 className="text-sm font-semibold text-(--color-text-primary)">{title}</h3>
      <span className="text-xs text-(--color-text-muted)">
        {`${count.toLocaleString()}${unit}`}
      </span>
    </div>
  );
}
