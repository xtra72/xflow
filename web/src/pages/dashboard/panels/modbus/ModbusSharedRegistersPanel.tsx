// SPEC-MODBUS-012 M4 (REQ-04, AC-09~11/11b): 공유 레지스터 맵 패널.
//
// modbus-gateway 의 unit 0 공유 컨테이너 레지스터 맵을 get_map(unit_id=0)으로 조회하여
// 4영역 그리드(POINTS/ACTIVE/DEGRADED + 셀 색상)로 렌더한다. get_map 은 register_counts 를
// 반환하지 않으므로(스냅샷만) 정의 개수를 스냅샷 present 개수로 유도한다(공유 맵 degraded 0).

import { Grid3x3 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import RegisterMapGrid from './RegisterMapGrid';
import { countsFromSnapshot, isRegisterMapEmpty, resolveAreaColumns } from './registerCellState';
import { useModbusGate, useModbusRegisterMap } from './useModbusData';

interface ModbusSharedRegistersPanelProps {
  title: string;
  config: Record<string, unknown>;
}

/** 공유 컨테이너(unit 0) 고정. */
const SHARED_UNIT_ID = 0;

/** 공유 레지스터 맵 패널. */
export default function ModbusSharedRegistersPanel({
  title,
  config,
}: ModbusSharedRegistersPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  // config.areaColumns(영역별 열 수) → 영역 카드 내부 셀 그리드의 행당 셀 수(영역별).
  // 미설정/0 은 자동(flex-wrap). 구 단일 config.columns 는 전 영역 시드로 하위호환된다.
  const areaColumns = resolveAreaColumns(config);
  const { registerMap, isLoading, isError } = useModbusRegisterMap(
    gate.agentId,
    SHARED_UNIT_ID,
    gate.enabled,
  );

  const icon = <Grid3x3 className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;

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
  if (isLoading && !registerMap) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusSpinner />
      </ModbusPanelFrame>
    );
  }
  // 조회 에러(공유 컨테이너 미구성 ErrNoSharedContainer 포함)/빈 스냅샷 → 안내(AC-11b).
  const counts = registerMap ? countsFromSnapshot(registerMap) : undefined;
  if (isError || !registerMap || !counts || isRegisterMapEmpty(registerMap, counts)) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.noSharedContainer')} />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={title} icon={icon}>
      <RegisterMapGrid registerMap={registerMap} registerCounts={counts} areaColumns={areaColumns} />
    </ModbusPanelFrame>
  );
}
