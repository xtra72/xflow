// SPEC-MODBUS-012 M2 (REQ-03): 가상 디바이스 목록 패널.
//
// modbus-gateway 에이전트의 list_devices 를 폴링하여 가상 디바이스(U01~)를
// unit_id, name, 영역 배지(CO/DI/IR/HR), active/stale 상태와 함께 나열한다.
// active/stale 은 폴링 간 stats(read/write/error) 델타로 판정한다(REQ-03-02).

import { useEffect, useRef, useState } from 'react';
import { Activity, CircleStop, Cpu } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import {
  AREA_BADGES,
  formatUnitLabel,
  useModbusGate,
  useModbusListDevices,
  type ModbusDeviceListItem,
} from './useModbusData';

interface ModbusVirtualDevicesPanelProps {
  title: string;
  config: Record<string, unknown>;
}

/** stats 합계(read+write+error) — 접근 발생 여부 판정용. */
function accessTotal(d: ModbusDeviceListItem): number {
  return d.stats.read_count + d.stats.write_count + d.stats.error_count;
}

/** 가상 디바이스 목록 패널. */
export default function ModbusVirtualDevicesPanel({
  title,
  config,
}: ModbusVirtualDevicesPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  const { devices, isLoading, isError } = useModbusListDevices(gate.agentId, gate.enabled);

  // 폴링 간 stats 델타로 active(최근 접근)/stale(무접근) 판정(REQ-03-02).
  // 델타 계산은 primitive 시그니처를 의존성으로 사용해 배열 참조 변경에 의한 무한 렌더를 막는다.
  const prevTotals = useRef<Map<number, number>>(new Map());
  const [activeMap, setActiveMap] = useState<Record<number, boolean>>({});
  const totalSig = devices.map((d) => `${d.unit_id}:${accessTotal(d)}`).join('|');

  useEffect(() => {
    const map = prevTotals.current;
    const next: Record<number, boolean> = {};
    for (const d of devices) {
      const total = accessTotal(d);
      const prev = map.get(d.unit_id);
      next[d.unit_id] = prev !== undefined && total > prev;
      map.set(d.unit_id, total);
    }
    setActiveMap(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [totalSig]);

  const icon = <Cpu className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;

  if (!gate.bound) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.notConfigured')} />
      </ModbusPanelFrame>
    );
  }
  if (gate.remote) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.remoteUnavailable')} />
      </ModbusPanelFrame>
    );
  }
  if (isLoading) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusSpinner />
      </ModbusPanelFrame>
    );
  }
  if (isError) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.loadError')} />
      </ModbusPanelFrame>
    );
  }
  if (devices.length === 0) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.empty')} />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={title} icon={icon}>
      <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto">
        {devices.map((d) => {
          const isActive = activeMap[d.unit_id] === true;
          return (
            <div
              key={d.unit_id}
              data-testid={`modbus-virtual-device-${d.unit_id}`}
              className="flex items-center gap-2 rounded-md border border-(--color-border-default) px-2.5 py-2"
            >
              <span className="font-mono text-xs font-semibold text-(--color-text-secondary)">
                {formatUnitLabel(d.unit_id)}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm text-(--color-text-primary)">
                {d.name || formatUnitLabel(d.unit_id)}
              </span>
              {/* 영역 배지(정의 개수>0 인 영역만) */}
              <span className="flex shrink-0 gap-1">
                {AREA_BADGES.filter((b) => d.register_counts[b.key] > 0).map((b) => (
                  <span
                    key={b.key}
                    data-testid={`modbus-area-badge-${d.unit_id}-${b.key}`}
                    className="rounded bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] font-medium text-(--color-text-muted)"
                  >
                    {b.label}
                  </span>
                ))}
              </span>
              {/* active/stale 상태 */}
              <span
                data-testid={`modbus-virtual-status-${d.unit_id}`}
                data-status={isActive ? 'active' : 'stale'}
                className={cn(
                  'inline-flex shrink-0 items-center',
                  isActive
                    ? 'text-green-600 dark:text-green-400'
                    : 'text-(--color-text-muted)',
                )}
                title={isActive ? t('dashboard.modbus.active') : t('dashboard.modbus.stale')}
              >
                {isActive ? (
                  <Activity className="h-4 w-4" aria-label={t('dashboard.modbus.active')} />
                ) : (
                  <CircleStop className="h-4 w-4" aria-label={t('dashboard.modbus.stale')} />
                )}
              </span>
            </div>
          );
        })}
      </div>
    </ModbusPanelFrame>
  );
}
