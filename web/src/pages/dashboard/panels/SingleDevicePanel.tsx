// 개별 디바이스 대시보드 제어 패널.
// 특정 디바이스의 리모컨(제어 UI)을 실시간으로 표시한다.

import { HardDrive } from 'lucide-react';

import { useDeviceRealtime } from '@/hooks/useDevice';
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
  const deviceId = config.deviceId as string | undefined;
  const panelColor = config.panelColor as string | undefined;
  const accentElements = config.accentElements as Record<string, string | boolean> | undefined;
  const { data: device, isLoading } = useDeviceRealtime(deviceId ?? '');

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
      {/* 헤더: 상태 점 + 이름 + 설정 */}
      <div className="mb-3 flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'h-2 w-2 shrink-0 rounded-full',
              device.online ? 'bg-green-500' : 'bg-gray-400',
            )}
            style={device.online && acColor('indicators') ? { backgroundColor: acColor('indicators')! } : undefined}
          />
          <span
            className="text-sm font-medium text-(--color-text-primary)"
            style={acColor('labels') ? { color: acColor('labels')! } : undefined}
          >
            {title}
          </span>
          <span className="text-xs text-(--color-text-muted)" style={acColor('labels') ? { color: `${acColor('labels')}80` } : undefined}>{device.protocol.toUpperCase()}</span>
        </div>
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
          <p className="text-xs text-(--color-text-muted)">속성 정보가 없습니다.</p>
        </div>
      )}
    </div>
  );
}
