// 시스템 이벤트 타임라인 대시보드 패널.
//
// `config.items` 는 이벤트 유형 집합이며, `all` 이 포함되거나 비어 있으면 유형
// 필터 없이 전부 보여준다.

import { useMemo } from 'react';

import EventTimeline, { type EventType } from '@/pages/monitoring/EventTimeline';
import { useMonitorStream } from '@/pages/monitoring/monitorStream';

import { usePanelTitleVisible } from '../../panelChromeContext';
import { readPanelItems } from './monitorPanelConfig';

/** 이벤트 항목 키는 EventType 과 1:1 이다 ('all' 만 예외). */
const ITEM_TYPES: EventType[] = ['status_change', 'deployment', 'error', 'system'];

interface MonitorEventsPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function MonitorEventsPanel({ title, config }: MonitorEventsPanelProps) {
  const showTitle = usePanelTitleVisible();
  const items = useMemo(() => readPanelItems('events', config), [config]);
  const { events } = useMonitorStream();

  const types = useMemo(() => {
    if (items.length === 0 || items.includes('all')) return null;
    const set = new Set<EventType>(items.filter((k): k is EventType =>
      ITEM_TYPES.includes(k as EventType),
    ));
    return set.size > 0 ? set : null;
  }, [items]);

  const filtered = useMemo(
    () => (types === null ? events : events.filter((e) => types.has(e.type))),
    [events, types],
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="monitor-events-panel">
        <EventTimeline events={filtered} title={showTitle ? title : undefined} />
      </div>
    </div>
  );
}
