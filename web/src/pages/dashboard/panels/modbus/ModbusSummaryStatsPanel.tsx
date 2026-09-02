// SPEC-MODBUS-012 M5 (REQ-06-03, AC-17): 종합 통계 패널(상단 바 스타일).
//
// get_status + list_devices + list_clients 를 조합하여 종합 통계 타일을 표출한다:
// poll cycle, throughput(req/s), clients, active connections, errors(rate), uptime,
// 총 디바이스, 총 reads/writes/errors. throughput 은 버스 델타 시계열의 최신 rate 로 계산한다.

import { Gauge } from 'lucide-react';
import type { ReactNode } from 'react';

import { useTranslation } from '@/lib/i18n';
import { useUIStore } from '@/stores/uiStore';

import { ModbusNotice, ModbusPanelFrame, ModbusSpinner } from './panelChrome';
import {
  formatUptime,
  sumDeviceTotals,
  useModbusBusSeries,
  useModbusGate,
  useModbusListClients,
  useModbusListDevices,
  useModbusStatus,
} from './useModbusData';

interface ModbusSummaryStatsPanelProps {
  title: string;
  config: Record<string, unknown>;
}

/** 오류율(%) — errors / (reads+writes+errors). */
function errorRatePct(reads: number, writes: number, errors: number): number {
  const total = reads + writes + errors;
  if (total <= 0) return 0;
  return Math.round((errors / total) * 1000) / 10;
}

/** 종합 통계 타일. */
function StatTile({
  testid,
  label,
  value,
  tone,
}: {
  testid: string;
  label: string;
  value: ReactNode;
  tone?: 'default' | 'danger';
}) {
  return (
    <div
      data-testid={testid}
      // 그리드 트랙(1fr)을 채우도록 min-w 를 제거하고 폭 100%로 늘린다(패널 폭에 맞춰 균등 신축).
      className="flex min-w-0 flex-col rounded-md border border-(--color-border-default) px-3 py-2"
    >
      <span className="truncate text-[10px] uppercase tracking-wide text-(--color-text-muted)">
        {label}
      </span>
      <span
        className={
          tone === 'danger'
            ? 'truncate text-lg font-semibold text-amber-600 dark:text-amber-400'
            : 'truncate text-lg font-semibold text-(--color-text-primary)'
        }
      >
        {value}
      </span>
    </div>
  );
}

/** 종합 통계 패널. */
export default function ModbusSummaryStatsPanel({ title, config }: ModbusSummaryStatsPanelProps) {
  const { t } = useTranslation();
  const gate = useModbusGate(config);
  // config.columns(≥1) → 행당 고정 타일 수. 미설정/0 은 auto-fit 반응형(기본).
  const columns =
    typeof config.columns === 'number' && config.columns > 0 ? config.columns : undefined;
  const pollCycle = useUIStore((s) => s.dashboardRefreshInterval);
  const { status, isLoading: statusLoading, isError: statusError } = useModbusStatus(
    gate.agentId,
    gate.enabled,
  );
  const { devices, isError: devicesError } = useModbusListDevices(gate.agentId, gate.enabled);
  const { clients } = useModbusListClients(gate.agentId, gate.enabled);

  const activeConnections = status?.active_connections ?? 0;
  const series = useModbusBusSeries(devices, activeConnections);
  const totals = sumDeviceTotals(devices);
  const last = series.length > 0 ? series[series.length - 1] : undefined;
  // throughput(req/s) = 최신 버스 델타(reads+writes per min) / 60. 기준선 이전이면 0.
  const throughput = last ? Math.round(((last.reads_per_min + last.writes_per_min) / 60) * 10) / 10 : 0;

  const icon = <Gauge className="h-4 w-4 shrink-0 text-(--color-text-muted)" />;

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
  if (statusLoading && !status) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusSpinner />
      </ModbusPanelFrame>
    );
  }
  // exec 실패(에이전트 미실행/무응답) → 0 값 타일 대신 명시적 안내(오해 소지 방지).
  if ((statusError || devicesError) && !status) {
    return (
      <ModbusPanelFrame title={title} icon={icon}>
        <ModbusNotice message={t('dashboard.modbus.agentNotResponding')} />
      </ModbusPanelFrame>
    );
  }

  const errPct = errorRatePct(totals.reads, totals.writes, totals.errors);

  return (
    <ModbusPanelFrame title={title} icon={icon}>
      <div
        data-testid="modbus-summary-bar"
        // columns 설정 시 행당 고정 열(repeat(N, 1fr)). 미설정 시 auto-fit 그리드로 타일이 패널
        // 폭에 맞춰 균등 신축(1fr)하고, 폭이 좁아지면 열을 줄여 리플로우한다(기본 동작).
        className="grid min-h-0 flex-1 content-start gap-2 overflow-y-auto"
        style={{
          gridTemplateColumns: columns
            ? `repeat(${columns}, minmax(0, 1fr))`
            : 'repeat(auto-fit, minmax(92px, 1fr))',
        }}
      >
        <StatTile
          testid="modbus-summary-pollCycle"
          label={t('dashboard.modbus.pollCycle')}
          value={`${pollCycle}s`}
        />
        <StatTile
          testid="modbus-summary-throughput"
          label={t('dashboard.modbus.throughput')}
          value={`${throughput}/s`}
        />
        <StatTile
          testid="modbus-summary-connections"
          label={t('dashboard.modbus.connections')}
          value={activeConnections}
        />
        <StatTile
          testid="modbus-summary-clients"
          label={t('dashboard.modbus.clients')}
          value={clients.length}
        />
        <StatTile
          testid="modbus-summary-devices"
          label={t('dashboard.modbus.totalDevices')}
          value={devices.length}
        />
        <StatTile
          testid="modbus-summary-reads"
          label={t('dashboard.modbus.totalReads')}
          value={totals.reads}
        />
        <StatTile
          testid="modbus-summary-writes"
          label={t('dashboard.modbus.totalWrites')}
          value={totals.writes}
        />
        <StatTile
          testid="modbus-summary-errors"
          label={t('dashboard.modbus.totalErrors')}
          value={`${totals.errors} (${errPct}%)`}
          tone={totals.errors > 0 ? 'danger' : 'default'}
        />
        <StatTile
          testid="modbus-summary-uptime"
          label={t('dashboard.modbus.uptime')}
          value={formatUptime(status?.uptime_seconds ?? 0)}
        />
      </div>
    </ModbusPanelFrame>
  );
}
