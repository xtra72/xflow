// 디바이스 목록 테이블의 단일 컬럼 셀 렌더러.
//
// 디바이스 탭(DeviceListPage)과 대시보드 디바이스 패널(DevicePanel)이 같은 컬럼
// 집합(useDeviceColumns 의 DeviceListColumnKey)을 같은 모양으로 렌더하도록 이 셀
// 하나만 공유한다. 각자 셀 렌더를 갖고 있으면 한쪽만 컬럼이 추가되거나 배지 색이
// 달라져 두 화면의 표시가 조용히 갈라진다.

import { Lock } from 'lucide-react';

import type { TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { getDeviceDisplayName, getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import type { DeviceListColumnKey } from '@/hooks/useDeviceColumns';
import type { DeviceInfo } from '@/types/device';

import DeviceStatusBadge from './DeviceStatusBadge';
import { PROTOCOL_COLORS, formatRelativeTime, sourceVariant } from './deviceDisplay';

/** 단일 컬럼 셀 렌더 (컬럼 키별). */
export function DeviceCell({
  column,
  device,
  t,
}: {
  column: DeviceListColumnKey;
  device: DeviceInfo;
  t: TranslationFn;
}) {
  switch (column) {
    case 'name':
      return (
        <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-(--color-text-primary)">
          {getDeviceDisplayName(device)}
        </td>
      );
    case 'id': {
      // uid 우선. 공간이 남으면 전체 표시 — 잘라내지 않고 한 줄로 노출(전체값 툴팁 유지).
      const idValue = device.uid || device.id;
      return (
        <td className="px-4 py-3">
          <span
            title={idValue}
            className="block whitespace-nowrap font-mono text-xs text-(--color-text-muted)"
          >
            {idValue || '-'}
          </span>
        </td>
      );
    }
    case 'type':
      return (
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {getDeviceTypeLabel(device.type)}
        </td>
      );
    case 'protocol': {
      const protocolColor =
        PROTOCOL_COLORS[device.protocol] ?? 'bg-(--color-bg-elevated) text-(--color-text-muted)';
      return (
        <td className="whitespace-nowrap px-4 py-3">
          <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', protocolColor)}>
            {device.protocol.toUpperCase()}
          </span>
        </td>
      );
    }
    case 'status':
      return (
        <td className="whitespace-nowrap px-4 py-3">
          <DeviceStatusBadge online={device.online} />
        </td>
      );
    case 'agent':
      return (
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {device.agent_name}
        </td>
      );
    case 'source': {
      const variant = sourceVariant(device.source);
      return (
        <td className="whitespace-nowrap px-4 py-3">
          {!variant ? (
            <span className="text-xs text-(--color-text-muted)">-</span>
          ) : (
            <span
              className={cn(
                'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium',
                variant.manual
                  ? 'bg-(--color-bg-elevated) text-(--color-text-muted)'
                  : 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400',
              )}
              title={variant.manual ? t('devices.source.manualTitle') : t('devices.source.autoTitle')}
            >
              {variant.manual && <Lock className="h-2.5 w-2.5" />}
              {variant.labelKey ? t(variant.labelKey) : variant.rawLabel}
            </span>
          )}
        </td>
      );
    }
    case 'last_seen':
      return (
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatRelativeTime(device.last_seen, t)}
        </td>
      );
    default:
      return null;
  }
}
