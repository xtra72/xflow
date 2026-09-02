// SPEC-MODBUS-012 M4 (REQ-05, AC-12/13): 가상 디바이스 레지스터 맵 패널.
//
// config.unitId 로 지정된 가상 디바이스의 레지스터 맵을 get_device_status(unit_id=N)로 조회하여
// REQ-04 와 동일한 RegisterMapGrid(4영역 + 값 기반 셀 색상)로 렌더한다(AC-13 공유 로직).
// get_device_status 는 register_counts 를 반환하므로 degraded(정의−스냅샷 부족분)가 계산된다.

import { LayoutGrid } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import RegisterMapGrid from './RegisterMapGrid';
import { isRegisterMapEmpty, resolveAreaColumns } from './registerCellState';
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
  // config.areaColumns(영역별 열 수) → 영역 카드 내부 셀 그리드의 행당 셀 수(영역별).
  // 미설정/0 은 자동(flex-wrap). 구 단일 config.columns 는 전 영역 시드로 하위호환된다.
  const areaColumns = resolveAreaColumns(config);
  // config.areaLayoutColumns(영역 카드 배치 열 수) → 4개 영역 카드의 외곽 그리드 행당 카드 수.
  // 미설정/0 은 반응형 기본(1→2열). areaColumns 와는 독립적인 별개 설정이다.
  const layoutColumns = config.areaLayoutColumns as number | undefined;
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
  // 유닛은 선택됐으나 디바이스에 레지스터 영역이 전혀 없음 → 세그먼트 추가를 안내(제네릭 empty 대신).
  // (백엔드 get_device_status 는 공유-앨리어스/백킹 값을 포함한 유효 맵을 반환하므로, 이 분기는
  //  공유 세그먼트조차 없는 진짜 빈 디바이스에만 도달한다.)
  if (isRegisterMapEmpty(status.register_map, status.register_counts)) {
    return (
      <ModbusPanelFrame title={heading} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.deviceNoSegments')} />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={heading} icon={icon}>
      <RegisterMapGrid
        registerMap={status.register_map}
        registerCounts={status.register_counts}
        areaColumns={areaColumns}
        layoutColumns={layoutColumns}
      />
    </ModbusPanelFrame>
  );
}
