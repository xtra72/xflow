// 디바이스 속성 그리드 패널.
// 디바이스의 속성을 설정 가능한 컬럼 수와 항목 선택으로 표시한다.

import { Activity, HardDrive, Moon } from 'lucide-react';

import { useDeviceRealtime } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import { getPropertyLabel, sortProperties, formatPropertyValue } from '@/lib/utils/deviceLabels';

interface PropertiesGridPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function PropertiesGridPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: PropertiesGridPanelProps) {
  const deviceId = config.deviceId as string | undefined;
  const gridCols = (config.gridCols as number | undefined) ?? 3;
  const visibleProperties = (config.visibleProperties as string[] | undefined) ?? [];
  const panelColor = config.panelColor as string | undefined;
  const accentElements = config.accentElements as Record<string, string | boolean> | undefined;

  const { data: device, isLoading } = useDeviceRealtime(deviceId ?? '');

  const acColor = (group: string): string | undefined => {
    if (!accentElements) return panelColor;
    const val = accentElements[group];
    if (val === false) return undefined;
    if (typeof val === 'string') return val;
    return panelColor;
  };

  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">디바이스가 설정되지 않았습니다.</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-2 flex shrink-0 items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-gray-300" />
          <span className="text-sm font-medium text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex flex-1 items-center justify-center">
          <div
            className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600"
            style={panelColor ? { borderTopColor: panelColor } : undefined}
          />
        </div>
      </div>
    );
  }

  if (!device) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">디바이스를 찾을 수 없습니다.</p>
      </div>
    );
  }

  const properties = device.state?.properties;
  if (!properties || Object.keys(properties).length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-3 flex shrink-0 items-center justify-between">
          <span className="text-sm font-medium text-(--color-text-primary)">{title}</span>
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            device.online
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
          )}>
            {device.online
              ? <span title="가동 중"><Activity className="h-3.5 w-3.5" aria-label="가동 중" /></span>
              : <span title="대기"><Moon className="h-3.5 w-3.5" aria-label="대기" /></span>}
          </span>
        </div>
        <div className="flex flex-1 items-center justify-center">
          <p className="text-xs text-(--color-text-muted)">속성 정보가 없습니다.</p>
        </div>
      </div>
    );
  }

  // 표시할 속성 필터링 (visibleProperties가 비어있으면 전체 표시)
  let entries = sortProperties(Object.entries(properties));
  if (visibleProperties.length > 0) {
    const allowed = new Set(visibleProperties);
    entries = entries.filter(([key]) => allowed.has(key));
  }

  const colClass =
    gridCols === 1 ? 'grid-cols-1'
    : gridCols === 2 ? 'grid-cols-2'
    : gridCols === 4 ? 'grid-cols-4'
    : gridCols === 5 ? 'grid-cols-5'
    : gridCols >= 6 ? 'grid-cols-6'
    : 'grid-cols-3';

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {/* 헤더 */}
      <div className="mb-3 flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className="text-sm font-medium text-(--color-text-primary)"
            style={acColor('labels') ? { color: acColor('labels')! } : undefined}
          >
            {title}
          </span>
          <span className="text-xs text-(--color-text-muted)" style={acColor('labels') ? { color: `${acColor('labels')}80` } : undefined}>
            {device.protocol.toUpperCase()}
          </span>
        </div>
        <span className={cn(
          'inline-flex items-center gap-1 rounded-full px-2 py-1',
          device.online
            ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
            : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
        )}>
          {device.online
            ? <span title="가동 중"><Activity className="h-3.5 w-3.5" aria-label="가동 중" /></span>
            : <span title="대기"><Moon className="h-3.5 w-3.5" aria-label="대기" /></span>}
        </span>
      </div>

      {/* 속성 그리드 */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className={cn('grid gap-3', colClass)}>
          {entries.map(([key, value]) => (
            <div
              key={key}
              className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2"
              style={acColor('borders') ? { borderColor: `${acColor('borders')}30` } : undefined}
            >
              <p className="text-xs text-(--color-text-muted)" style={acColor('labels') ? { color: acColor('labels')! } : undefined}>
                {getPropertyLabel(key, device.protocol, device.type)}
              </p>
              <p className="mt-0.5 text-sm font-medium text-(--color-text-primary)">
                {formatPropertyValue(key, value)}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
