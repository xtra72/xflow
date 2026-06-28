// 개별 디바이스 대시보드 제어 패널.
// 특정 디바이스의 리모컨(제어 UI)을 실시간으로 표시한다.

import { Activity, HardDrive, Moon } from 'lucide-react';

import { useDeviceDetailTarget } from '@/hooks/useDetailTargets';
import { useDeviceRealtime } from '@/hooks/useDevice';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';

import { StatePropertiesSection } from '@/pages/devices/DeviceDetailPanel';

interface SingleDevicePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 개별 디바이스 제어 패널 (리모컨) */
export default function SingleDevicePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: SingleDevicePanelProps) {
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;
  const panelColor = config.panelColor as string | undefined;
  const accentElements = config.accentElements as Record<string, string | boolean> | undefined;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04/L03): 원격이면 device ref 를
  // 그 노드 기준으로 해석해 device.state(그룹 J SSE+폴백)로 실시간 상태를 취득한다.
  // 로컬은 기존 useDeviceRealtime 그대로(회귀 없음).
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localQuery = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteQuery = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteQuery.data : localQuery.data;
  const isLoading = remote ? remoteQuery.isLoading : localQuery.isLoading;

  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotConfigured')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-2 flex shrink-0 items-center justify-between">
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
          <span className="inline-flex items-center rounded-full bg-slate-100 px-2 py-1 text-slate-400 dark:bg-slate-800 dark:text-slate-500">
            <Moon className="h-3.5 w-3.5" aria-label={t('dashboard.panel.loadingAria')} />
          </span>
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
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotFound')}</p>
      </div>
    );
  }

  const properties = device.state?.properties;
  const hasProperties = properties && Object.keys(properties).length > 0;
  const acColor = (group: string): string | undefined => {
    if (!accentElements) return panelColor;
    const val = accentElements[group];
    if (val === false) return undefined;
    if (typeof val === 'string') return val;
    return panelColor;
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {/* 헤더: 이름 + 상태 배지 */}
      <div className="mb-3 flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className="truncate text-sm font-medium text-(--color-text-primary)"
            style={acColor('labels') ? { color: acColor('labels')! } : undefined}
          >
            {title}
          </span>
          <span className="text-xs text-(--color-text-muted)" style={acColor('labels') ? { color: `${acColor('labels')}80` } : undefined}>{device.protocol.toUpperCase()}</span>
        </div>
        <span className={cn(
          'inline-flex items-center gap-1 rounded-full px-2 py-1',
          device.online
            ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
            : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
        )}>
          {device.online
            ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
            : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
        </span>
      </div>

      {/* 제어 UI (full mode) */}
      {hasProperties ? (
        <div className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto">
          <StatePropertiesSection
            properties={properties}
            protocol={device.protocol}
            type={device.type}
            deviceId={device.id}
            accentColor={panelColor}
            accentElements={accentElements}
          />
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.noProperties')}</p>
        </div>
      )}
    </div>
  );
}
