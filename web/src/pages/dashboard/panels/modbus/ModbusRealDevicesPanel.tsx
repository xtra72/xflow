// SPEC-MODBUS-012 M2 (REQ-02): 실제 연결(upstream 백킹) 디바이스 목록 패널.
//
// list_devices 의 backed=true 항목만 실제 디바이스로 표시하고(REQ-02-01), 각 디바이스에 대해
// get_device_status.backing(게이트웨이→upstream 관측 메트릭)을 조회하여 모드/슬레이브 id/
// 요청·에러 수/평균 레이턴시를 표출한다(REQ-02-02). connected=false 는 degraded 로 구분한다(REQ-02-03).

import { HardDrive, PlugZap } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import {
  formatUnitLabel,
  useModbusDeviceStatus,
  useModbusGate,
  useModbusListDevices,
  type ModbusDeviceListItem,
} from './useModbusData';

interface ModbusRealDevicesPanelProps {
  title: string;
  config: Record<string, unknown>;
}

/** 에러율(%) 포맷. 요청 0 이면 0%. */
function errorRatePct(requests: number, errors: number): number {
  if (requests <= 0) return 0;
  return Math.round((errors / requests) * 1000) / 10;
}

/**
 * 실제 디바이스 한 행. 각 행이 자체적으로 get_device_status 를 폴링하여
 * backing 메트릭을 취득한다(행 단위 컴포넌트 → hooks 규약 안전).
 */
function RealDeviceRow({
  agentId,
  device,
  enabled,
}: {
  agentId: string;
  device: ModbusDeviceListItem;
  enabled: boolean;
}) {
  const { t } = useTranslation();
  const { status, isLoading } = useModbusDeviceStatus(agentId, device.unit_id, enabled);
  const backing = status?.backing ?? null;

  // backing 이 아직 없으면(로딩) list_devices 의 mode 로 임시 표기, connected 미상은 중립.
  const mode = backing?.mode || device.mode || '-';
  const connected = backing?.connected ?? true;
  const degraded = backing !== null && !connected;

  return (
    <div
      data-testid={`modbus-real-device-${device.unit_id}`}
      data-degraded={degraded ? 'true' : 'false'}
      className={cn(
        'flex items-center gap-2 rounded-md border px-2.5 py-2',
        degraded
          ? 'border-amber-300 bg-amber-50 dark:border-amber-500/40 dark:bg-amber-900/20'
          : 'border-(--color-border-default)',
      )}
    >
      {/* 연결 상태 점 */}
      <span
        data-testid={`modbus-real-status-${device.unit_id}`}
        data-status={degraded ? 'degraded' : 'online'}
        className={cn(
          'h-2.5 w-2.5 shrink-0 rounded-full',
          degraded ? 'bg-amber-500' : 'bg-green-500',
        )}
        title={degraded ? t('dashboard.modbus.degraded') : t('dashboard.modbus.connected')}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-sm font-medium text-(--color-text-primary)">
            {device.name || formatUnitLabel(device.unit_id)}
          </span>
          <span className="shrink-0 rounded bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] font-medium text-(--color-text-muted)">
            {mode}
          </span>
        </div>
        <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-(--color-text-muted)">
          <span title={t('dashboard.modbus.slaveId')}>
            {t('dashboard.modbus.slaveId')} {device.unit_id}
          </span>
          {isLoading && backing === null ? (
            <span>…</span>
          ) : (
            <>
              <span data-testid={`modbus-real-latency-${device.unit_id}`}>
                {t('dashboard.modbus.latency')} {(backing?.avg_latency_ms ?? 0).toFixed(1)}ms
              </span>
              <span data-testid={`modbus-real-errrate-${device.unit_id}`}>
                {t('dashboard.modbus.errRate')}{' '}
                {errorRatePct(backing?.request_count ?? 0, backing?.error_count ?? 0)}%
              </span>
              <span data-testid={`modbus-real-requests-${device.unit_id}`}>
                {t('dashboard.modbus.requests')} {backing?.request_count ?? 0}
              </span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

/** 실제 연결 디바이스 목록 패널. */
export default function ModbusRealDevicesPanel({ title, config }: ModbusRealDevicesPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  const { devices, isLoading, isError } = useModbusListDevices(gate.agentId, gate.enabled);
  const backedDevices = devices.filter((d) => d.backed === true);

  const icon = <PlugZap className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;

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
  if (backedDevices.length === 0) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice
          icon={<HardDrive className="h-6 w-6 text-(--color-text-muted)" />}
          message={t('dashboard.modbus.noBackedDevices')}
        />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={title} icon={icon}>
      <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto">
        {backedDevices.map((d) => (
          <RealDeviceRow key={d.unit_id} agentId={gate.agentId} device={d} enabled={gate.enabled} />
        ))}
      </div>
    </ModbusPanelFrame>
  );
}
