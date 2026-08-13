// 디바이스 속성 그리드 패널.
// 디바이스의 속성을 설정 가능한 컬럼 수와 항목 선택으로 표시한다.

import { Activity, HardDrive, Moon } from 'lucide-react';

import { useDeviceDetailTarget } from '@/hooks/useDetailTargets';
import { useDeviceRealtime } from '@/hooks/useDevice';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';
import { getPropertyLabel, sortProperties, formatPropertyValue, expandMeasurementEntries, excludeDedicatedSectionKeys } from '@/lib/utils/deviceLabels';
import { formatEpochMs, formatRelativeEpochMs } from '@/lib/utils/format';

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
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;
  const gridCols = (config.gridCols as number | undefined) ?? 3;
  const visibleProperties = (config.visibleProperties as string[] | undefined) ?? [];
  const panelColor = config.panelColor as string | undefined;
  const accentElements = config.accentElements as Record<string, string | boolean> | undefined;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04): 원격이면 device.state(그룹 J)로
  // 그 노드 디바이스의 속성을 읽는다(REQ-L03). 로컬은 기존 useDeviceRealtime 그대로.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localDevice = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteDevice = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteDevice.data : localDevice.data;
  const isLoading = remote ? remoteDevice.isLoading : localDevice.isLoading;

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
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotConfigured')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-2 flex shrink-0 items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-gray-300" />
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
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
  if (!properties || Object.keys(properties).length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-3 flex shrink-0 items-center justify-between">
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
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
        <div className="flex flex-1 items-center justify-center">
          <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.noProperties')}</p>
        </div>
      </div>
    );
  }

  // 전원 OFF 시 운전 계열 속성은 정규화된 기본값이라 실제 값이 아니므로 '-' 로 표시.
  const powerOff = properties['power'] === false;

  // 표시할 속성 필터링 (visibleProperties가 비어있으면 전체 표시).
  // 필터는 원본 속성 키 기준이므로 'measurements' 를 선택하면 측정치 전체가 표시된다.
  //
  // gateways 는 여기서 제외한다. 이 패널은 사용자가 컬럼 수와 표시 항목을 고르는
  // "key/value 카드 N열" 그리드라, (디바이스, 게이트웨이) 쌍 여러 건짜리 표를 끼워 넣으면
  // 사용자가 지정한 레이아웃이 깨진다. 제외하지 않으면 객체 폴백으로 JSON 덩어리가
  // 표시되므로 제외 자체는 필수다. 링크별 상세는 디바이스 상세 패널의 게이트웨이
  // 섹션과 에이전트 게이트웨이 탭에서 본다.
  let filtered = excludeDedicatedSectionKeys(sortProperties(Object.entries(properties)));
  if (visibleProperties.length > 0) {
    const allowed = new Set(visibleProperties);
    filtered = filtered.filter(([key]) => allowed.has(key));
  }
  // measurements 는 측정치별 개별 카드로 펼친다(측정치마다 갱신 시각이 다르다).
  const entries = expandMeasurementEntries(filtered);

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
            className="truncate text-sm font-medium text-(--color-text-primary)"
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
            ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
            : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
        </span>
      </div>

      {/* 속성 그리드 */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className={cn('grid gap-3', colClass)}>
          {entries.map(({ id, key, value, timeMs }) => (
            <div
              key={id}
              className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2"
              style={acColor('borders') ? { borderColor: `${acColor('borders')}30` } : undefined}
            >
              <p className="text-xs text-(--color-text-muted)" style={acColor('labels') ? { color: acColor('labels')! } : undefined}>
                {getPropertyLabel(key, device.protocol, device.type)}
              </p>
              <p className="mt-0.5 text-sm font-medium text-(--color-text-primary)">
                {formatPropertyValue(key, value, { powerOff })}
              </p>
              {/* 측정치별 갱신 시각. 값과 경쟁하지 않도록 작고 흐리게, 상대 시간으로 표시하고
                  정확한 시각은 title(hover)로 제공한다. */}
              {timeMs !== undefined && (
                <p className="mt-0.5 text-[10px] text-(--color-text-muted)" title={formatEpochMs(timeMs)}>
                  {formatRelativeEpochMs(timeMs)}
                </p>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
