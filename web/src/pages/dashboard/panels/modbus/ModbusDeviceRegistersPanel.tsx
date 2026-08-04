// SPEC-MODBUS-012 M4 (REQ-05, AC-12/13): 가상 디바이스 레지스터 맵 패널.
//
// config.unitId 로 지정된 가상 디바이스의 레지스터 맵을 get_device_status(unit_id=N)로 조회하여
// REQ-04 와 동일한 RegisterMapGrid(4영역 + 값 기반 셀 색상)로 렌더한다(AC-13 공유 로직).
// get_device_status 는 register_counts 를 반환하므로 degraded(정의−스냅샷 부족분)가 계산된다.

import { LayoutGrid } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import RegisterMapGrid from './RegisterMapGrid';
import { isRegisterMapEmpty } from './registerCellState';
import { formatUnitLabel, useModbusDeviceStatus, useModbusGate } from './useModbusData';

interface ModbusDeviceRegistersPanelProps {
  title: string;
  config: Record<string, unknown>;
}

/** 가상 디바이스 레지스터 맵 패널. */
export default function ModbusDeviceRegistersPanel({
  title,
  config,
}: ModbusDeviceRegistersPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  const unitId = (config.unitId as number | undefined) ?? 0;
  const { status, isLoading, isError } = useModbusDeviceStatus(gate.agentId, unitId, gate.enabled);

  const icon = <LayoutGrid className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;
  const heading = status?.name
    ? `${title} · ${status.name}`
    : `${title} · ${formatUnitLabel(unitId)}`;

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
  if (isLoading && !status) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusSpinner />
      </ModbusPanelFrame>
    );
  }
  if (isError || !status) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.loadError')} />
      </ModbusPanelFrame>
    );
  }
  if (isRegisterMapEmpty(status.register_map, status.register_counts)) {
    return (
      <ModbusPanelFrame title={heading} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.empty')} />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={heading} icon={icon}>
      <RegisterMapGrid
        registerMap={status.register_map}
        registerCounts={status.register_counts}
      />
    </ModbusPanelFrame>
  );
}
