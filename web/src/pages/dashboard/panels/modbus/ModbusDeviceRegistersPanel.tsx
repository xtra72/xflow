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
  const rawUnitId = config.unitId as number | undefined;
  // unit 0 은 공유 컨테이너(서빙 디바이스 아님) → get_device_status(0) 은 의미 있는 결과가 없다.
  // config.unitId 미설정(생성 시 가상 디바이스 없음/설정에서 에이전트 변경으로 초기화)이거나 0 이면
  // "유닛 미선택"으로 보고 명시적 안내를 표시한다(제네릭 empty/loadError 로 빠지지 않도록).
  const unitId = rawUnitId ?? 0;
  const unitSelected = rawUnitId !== undefined && rawUnitId > 0;
  const { status, isLoading, isError } = useModbusDeviceStatus(
    gate.agentId,
    unitId,
    gate.enabled && unitSelected,
  );

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
  // 대상 유닛 미선택(unitId 미설정 또는 0=공유 컨테이너) → 설정에서 유닛 선택을 안내한다.
  if (!unitSelected) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.unitNotSelected')} />
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
