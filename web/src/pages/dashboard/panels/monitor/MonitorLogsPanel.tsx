// 시스템 로그 대시보드 패널.
//
// 모니터링 페이지와 같은 LogViewer 를 그대로 쓴다. `config.items` 는 레벨 프리셋
// 집합이며, `all` 이 포함되거나 비어 있으면 레벨 필터 없이 전부 보여준다.
// 뷰어 안에서 추가로 거는 필터(소스/컴포넌트/페이지네이션)는 그대로 동작한다.

import { useMemo } from 'react';
import LogViewer, { type LogLevel } from '@/pages/monitoring/LogViewer';
import { useMonitorStream } from '@/pages/monitoring/monitorStream';
import { useLogComponentNavigate } from '@/pages/monitoring/useLogComponentNavigate';

import { usePanelTitleVisible } from '../../panelChromeContext';
import { readPanelItems } from './monitorPanelConfig';

/** 로그 항목 키 → 로그 레벨 */
const ITEM_LEVEL: Record<string, LogLevel> = {
  error: 'ERROR',
  warn: 'WARN',
  info: 'INFO',
  debug: 'DEBUG',
};

interface MonitorLogsPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function MonitorLogsPanel({ title, config }: MonitorLogsPanelProps) {
  const showTitle = usePanelTitleVisible();
  const items = useMemo(() => readPanelItems('logs', config), [config]);
  const { logs } = useMonitorStream();
  const navigateToComponent = useLogComponentNavigate();

  // 'all' 이거나 아무것도 고르지 않았으면 필터를 걸지 않는다.
  const levels = useMemo(() => {
    if (items.length === 0 || items.includes('all')) return null;
    const set = new Set<LogLevel>();
    for (const key of items) {
      const level = ITEM_LEVEL[key];
      if (level) set.add(level);
    }
    return set.size > 0 ? set : null;
  }, [items]);

  const entries = useMemo(
    () => (levels === null ? logs : logs.filter((e) => levels.has(e.level))),
    [logs, levels],
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="monitor-logs-panel">
        <LogViewer
          entries={entries}
          title={showTitle ? title : undefined}
          onSelectComponent={navigateToComponent}
        />
      </div>
    </div>
  );
}
