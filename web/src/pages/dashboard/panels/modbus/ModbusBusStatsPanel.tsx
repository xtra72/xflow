// SPEC-MODBUS-012 M5 (REQ-06-04, AC-18): 버스 통계 패널(미니 차트).
//
// list_devices 합계 델타 + get_status.active_connections 를 프론트에서 폴 델타로 누적하여
// 4종 미니차트를 렌더한다: reads/min, writes/min, errors/min, active connections.
// 시계열은 백엔드에 없으므로(get_status 무이력) 프론트 세션-로컬 링버퍼로 재구성한다.
//
// 목업 대비 결정(confusion 관리):
//   - "CRC errors" → 백엔드 CRC 카운터 부재(Non-Goal) → error_count 델타 "errors/min"로 대체(§4 caveat).
//   - "bus latency" → 버스 전체 평균 레이턴시는 list_devices/get_status 에 없고(디바이스별 backing
//     조회 N회 fan-out 필요) 백엔드 미변경 제약상 집계 소스가 없다. 대신 REQ-06 이 명시한
//     "활성 커넥션 추이 미니차트"(get_status.active_connections)를 4번째 차트로 사용한다.
// recharts(기존 설치 의존성) 로 스파크라인을 그린다(신규 차트 라이브러리 없음, AC-21).

import { Activity } from 'lucide-react';
import { Line, LineChart, ResponsiveContainer, YAxis } from 'recharts';

import { useTranslation } from '@/lib/i18n';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import {
  useModbusBusSeries,
  useModbusGate,
  useModbusListDevices,
  useModbusStatus,
  type ModbusBusPoint,
} from './useModbusData';

interface ModbusBusStatsPanelProps {
  title: string;
  config: Record<string, unknown>;
}

interface MiniChartSpec {
  key: keyof Pick<
    ModbusBusPoint,
    'reads_per_min' | 'writes_per_min' | 'errors_per_min' | 'active_connections'
  >;
  labelKey: string;
  color: string;
  testid: string;
}

const CHARTS: MiniChartSpec[] = [
  { key: 'reads_per_min', labelKey: 'dashboard.modbus.readsPerMin', color: '#3b82f6', testid: 'reads' },
  { key: 'writes_per_min', labelKey: 'dashboard.modbus.writesPerMin', color: '#8b5cf6', testid: 'writes' },
  { key: 'errors_per_min', labelKey: 'dashboard.modbus.errorsPerMin', color: '#f59e0b', testid: 'errors' },
  {
    key: 'active_connections',
    labelKey: 'dashboard.modbus.activeConnections',
    color: '#10b981',
    testid: 'connections',
  },
];

/** 단일 미니차트(스파크라인 + 최신값). */
function MiniChart({
  spec,
  points,
  label,
}: {
  spec: MiniChartSpec;
  points: ModbusBusPoint[];
  label: string;
}) {
  const latest = points.length > 0 ? points[points.length - 1]![spec.key] : 0;
  const display =
    spec.key === 'active_connections' ? String(latest) : `${Math.round(latest * 10) / 10}`;

  return (
    <div
      data-testid={`modbus-bus-chart-${spec.testid}`}
      className="flex min-h-0 flex-col rounded-md border border-(--color-border-default) p-2"
    >
      <div className="flex items-baseline justify-between gap-1">
        <span className="truncate text-[10px] uppercase tracking-wide text-(--color-text-muted)">
          {label}
        </span>
        <span
          data-testid={`modbus-bus-value-${spec.testid}`}
          className="shrink-0 text-sm font-semibold text-(--color-text-primary)"
        >
          {display}
        </span>
      </div>
      <div className="mt-1 h-10 min-h-0 flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={points} margin={{ top: 2, right: 2, left: 2, bottom: 2 }}>
            <YAxis hide domain={[0, 'auto']} />
            <Line
              type="monotone"
              dataKey={spec.key}
              stroke={spec.color}
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

/** 버스 통계 패널. */
export default function ModbusBusStatsPanel({ title, config }: ModbusBusStatsPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  const { devices, isLoading, isError: devicesError } = useModbusListDevices(
    gate.agentId,
    gate.enabled,
  );
  const { status, isError: statusError } = useModbusStatus(gate.agentId, gate.enabled);
  const activeConnections = status?.active_connections ?? 0;
  const points = useModbusBusSeries(devices, activeConnections);

  const icon = <Activity className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;

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
  if (isLoading && devices.length === 0) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusSpinner />
      </ModbusPanelFrame>
    );
  }
  // exec 실패(에이전트 미실행/무응답) → 빈 차트 대신 명시적 안내(빈 blank 방지).
  if (devicesError || statusError) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.agentNotResponding')} />
      </ModbusPanelFrame>
    );
  }

  return (
    <ModbusPanelFrame title={title} icon={icon}>
      <div
        data-testid="modbus-bus-grid"
        className="grid min-h-0 flex-1 grid-cols-2 gap-2 overflow-y-auto"
      >
        {CHARTS.map((spec) => (
          <MiniChart key={spec.key} spec={spec} points={points} label={t(spec.labelKey)} />
        ))}
      </div>
    </ModbusPanelFrame>
  );
}
