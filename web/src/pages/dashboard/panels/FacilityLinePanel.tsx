// Facility 라인 패널 (SPEC-FACILITY-DASHBOARD-001 M4, REQ-FACDASH-001-01-*, 06-*).
//
// config 의 agentId + line 으로 로스터를 조회해 한 호선을 렌더한다(REQ-01-01):
//   (1) 라인도(line map): 레지스트리 order 순 역사 노드를 수평 연결선 위에 배치하고 폭이 넘치면
//       여러 행으로 접힌다(flex-wrap). 각 노드는 상태 점(정상/경고/오프라인/꺼짐) + 역사명이며,
//       상세(detail) 모드에서는 노드 아래에 기기별 <위치명>-<index> + 풍량 배지(1/2/3단·오프라인·꺼짐)를
//       나열한다. 심플(simple) 모드는 노드만 표시한다. 헤더에 범례 + 심플/상세 토글을 둔다.
//   (2) 역사별 간략 상태 목록(showStationStatus), (3) 라인 통계(showLineStats), (4) 라인 일괄 제어.
// 집계는 aggregateByLine + lineDiagramLayout + stationStatus + stationDeviceRows(롤업만, UB-001),
// fan-out 은 FacilityBulkControl 호출만. 표시 옵션(showStationStatus/showLineStats/nodeSize)은
// 스냅샷 config 로만 저장한다(UB-003, 스키마 변경 없음). 심플/상세는 런타임 상태(비영속)이다.
// 미등록/빈 호선은 안내 + 일괄 제어 비활성(REQ-01-06). 미분류(미등록 station) 기기는 별도 표기(UB-004).

import { useState } from 'react';

import { HardDrive } from 'lucide-react';

import { useFacilityRoster } from '@/hooks/useAirpurifierControl';
import {
  aggregateByLine,
  countStats,
  lineDiagramLayout,
  stationDeviceRows,
  stationStatus,
  type DeviceFanStatus,
  type LineDiagramNode,
  type StationStatus,
} from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { FacilityBulkControl, StatTiles } from './facilityShared';

interface FacilityLinePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 라인도 표시 모드(런타임, 비영속). 스크린샷 기준 기본값은 상세(detail). */
type LineMode = 'simple' | 'detail';

/** 역 정보 크기(config 옵션). 노드 + 텍스트 크기를 조절한다("역 정보 크기 조절"). */
type NodeSize = 'sm' | 'md' | 'lg';

function toNodeSize(value: unknown): NodeSize {
  return value === 'sm' || value === 'lg' ? value : 'md';
}

// ---- 색상 토큰 ----

/** 역사 노드 상태 점(round) 색상. normal 초록 · warning 주황 · offline 빨강 · off 흰/회색 테두리. */
const STATION_DOT: Record<StationStatus, string> = {
  normal: 'bg-green-500',
  warning: 'bg-orange-500',
  offline: 'bg-red-500',
  off: 'border border-(--color-border-strong) bg-(--color-bg-surface)',
  empty: 'border border-dashed border-(--color-border-default) bg-transparent',
};

/** 풍량 배지(square) 배경. 1단 노랑 · 2단 초록 · 3단 파랑 · 오프라인 빨강 · 꺼짐 흰/회색. */
const FAN_SWATCH: Record<DeviceFanStatus, string> = {
  fan1: 'bg-yellow-400',
  fan2: 'bg-green-500',
  fan3: 'bg-blue-500',
  offline: 'bg-red-500',
  off: 'border border-(--color-border-strong) bg-(--color-bg-surface)',
  unknown: 'bg-(--color-border-default)',
};

/** 풍량 배지 위 텍스트 색상(가독 대비). */
const FAN_TEXT: Record<DeviceFanStatus, string> = {
  fan1: 'text-yellow-950',
  fan2: 'text-white',
  fan3: 'text-white',
  offline: 'text-white',
  off: 'text-(--color-text-muted)',
  unknown: 'text-(--color-text-secondary)',
};

// ---- 크기 토큰 ----

interface SizeTokens {
  node: string;
  name: string;
  dot: string;
  row: string;
  badge: string;
  /** 인접 노드 사이 연결선(수평). mt 는 노드 헤더 높이에 맞춰 정렬. */
  connector: string;
}

const NODE_SIZE: Record<NodeSize, SizeTokens> = {
  sm: {
    node: 'min-w-28 max-w-40 px-2.5 py-2',
    name: 'text-xs',
    dot: 'h-2.5 w-2.5',
    row: 'text-[11px]',
    badge: 'min-w-6 px-1 py-0.5 text-[9px]',
    connector: 'mt-4 w-3',
  },
  md: {
    node: 'min-w-36 max-w-52 px-3 py-2.5',
    name: 'text-sm',
    dot: 'h-3 w-3',
    row: 'text-xs',
    badge: 'min-w-7 px-1.5 py-0.5 text-[10px]',
    connector: 'mt-5 w-4',
  },
  lg: {
    node: 'min-w-44 max-w-64 px-4 py-3.5',
    name: 'text-base',
    dot: 'h-3.5 w-3.5',
    row: 'text-sm',
    badge: 'min-w-8 px-2 py-1 text-xs',
    connector: 'mt-6 w-5',
  },
};

/** Facility 라인 패널. */
export default function FacilityLinePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilityLinePanelProps) {
  const { t } = useTranslation();
  const agentId = (config.agentId as string | undefined) ?? '';
  const line = config.line as string | undefined;
  const refreshMs = config.refreshMs as number | undefined;

  // 표시 옵션(config, 영속). 기본값: 표시 on / 크기 보통(md).
  const showStationStatus = (config.showStationStatus as boolean | undefined) ?? true;
  const showLineStats = (config.showLineStats as boolean | undefined) ?? true;
  const size = NODE_SIZE[toNodeSize(config.nodeSize)];

  // 심플/상세 토글(런타임, 비영속). 기본 상세(스크린샷 기준).
  const [mode, setMode] = useState<LineMode>('detail');

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);

  if (!line || !agentId) {
    return <Shell title={title}>{notice(t('dashboard.facility.notConfigured'))}</Shell>;
  }
  if (isLoading) {
    return (
      <Shell title={title}>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </Shell>
    );
  }
  if (isError) {
    return <Shell title={title}>{notice(t('dashboard.facility.loadError'))}</Shell>;
  }

  const agg = aggregateByLine(devices, stations);
  const lineSummary = agg.lines.find((l) => l.line === line);
  const diagram = lineDiagramLayout(line, stations, devices);
  const lineStats = lineSummary?.stats ?? countStats([]);
  const deviceCount = lineSummary?.deviceCount ?? 0;
  const unclassified = agg.unclassified.count;

  // 빈/미등록 호선: 레지스트리에 역사도 없고 기기도 없음(REQ-01-06).
  const noData = diagram.length === 0 && deviceCount === 0;
  if (noData) {
    return (
      <Shell title={title}>
        {notice(t('dashboard.facility.noData'))}
        {unclassified > 0 && <UnclassifiedNote count={unclassified} />}
      </Shell>
    );
  }

  return (
    <Shell title={title} line={line}>
      {/* (1) 라인도: 헤더(라벨 + 범례 + 심플/상세 토글) + 연결선 위 역사 노드(wrapping). */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.line.diagram')}
          </span>
          <ModeToggle mode={mode} onChange={setMode} />
        </div>

        <Legend />

        <div className="flex flex-wrap items-start gap-y-3 pb-1" data-testid="line-diagram">
          {diagram.map((node, idx) => (
            <div key={node.station} className="flex items-start">
              {idx > 0 && (
                <span
                  aria-hidden="true"
                  className={cn('h-px shrink-0 self-start bg-(--color-border-strong)', size.connector)}
                />
              )}
              <DiagramNode node={node} mode={mode} size={size} />
            </div>
          ))}
        </div>
      </div>

      {/* (2) 역사별 간략 상태(showStationStatus). */}
      {showStationStatus && (
        <div className="space-y-1">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.line.stations')}
          </span>
          <ul className="grid grid-cols-2 gap-1" data-testid="line-station-list">
            {diagram.map((node) => (
              <li
                key={node.station}
                className="rounded-lg border border-(--color-border-default) px-2.5 py-1.5 text-[11px]"
              >
                <p className="truncate font-medium text-(--color-text-primary)">{node.displayName}</p>
                <p className="mt-0.5 text-(--color-text-muted)">
                  {t('dashboard.facility.stat.total')} {node.summary.stats.total} ·{' '}
                  {t('dashboard.facility.stat.online')} {node.summary.stats.online} /{' '}
                  {node.summary.stats.offline} · {t('dashboard.facility.stat.powerOn')}{' '}
                  {node.summary.stats.powerOn} / {node.summary.stats.powerOff}
                </p>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* (3) 라인 통계(showLineStats). */}
      {showLineStats && (
        <div className="space-y-1">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.line.stats')}
          </span>
          <StatTiles stats={lineStats} />
        </div>
      )}

      {/* 미분류 표기(UB-004, REQ-05-03) */}
      {unclassified > 0 && <UnclassifiedNote count={unclassified} />}

      {/* (4) 라인 일괄 제어(line 셀렉터). 멤버 0 이면 비활성(REQ-06-03). */}
      <FacilityBulkControl agentId={agentId} selector={{ line }} memberCount={deviceCount} />
    </Shell>
  );
}

// ---- 심플/상세 토글 ----

function ModeToggle({ mode, onChange }: { mode: LineMode; onChange: (mode: LineMode) => void }) {
  const { t } = useTranslation();
  const items: { value: LineMode; labelKey: string; testId: string }[] = [
    { value: 'simple', labelKey: 'dashboard.facility.line.simple', testId: 'line-mode-simple' },
    { value: 'detail', labelKey: 'dashboard.facility.line.detail', testId: 'line-mode-detail' },
  ];
  return (
    <div
      role="group"
      data-testid="line-mode-toggle"
      className="inline-flex overflow-hidden rounded-md border border-(--color-border-default) text-[11px]"
    >
      {items.map((item) => {
        const active = mode === item.value;
        return (
          <button
            key={item.value}
            type="button"
            data-testid={item.testId}
            aria-pressed={active}
            onClick={() => onChange(item.value)}
            className={cn(
              'px-2.5 py-1 font-medium transition-colors',
              active
                ? 'bg-blue-600 text-white dark:bg-blue-500'
                : 'bg-(--color-bg-surface) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
            )}
          >
            {t(item.labelKey)}
          </button>
        );
      })}
    </div>
  );
}

// ---- 범례 ----

function Legend() {
  const { t } = useTranslation();
  const statusItems: StationStatus[] = ['normal', 'warning', 'offline', 'off'];
  const fanItems: DeviceFanStatus[] = ['fan1', 'fan2', 'fan3', 'offline'];
  return (
    <div
      data-testid="line-legend"
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-(--color-text-muted)"
    >
      <span className="font-medium text-(--color-text-secondary)">
        {t('dashboard.facility.line.legend')}
      </span>
      {statusItems.map((s) => (
        <span key={`st-${s}`} className="inline-flex items-center gap-1">
          <span className={cn('h-2.5 w-2.5 rounded-full', STATION_DOT[s])} aria-hidden="true" />
          {statusLabel(s, t)}
        </span>
      ))}
      <span className="h-3 w-px bg-(--color-border-default)" aria-hidden="true" />
      {fanItems.map((s) => (
        <span key={`fan-${s}`} className="inline-flex items-center gap-1">
          <span className={cn('h-2.5 w-2.5 rounded-sm', FAN_SWATCH[s])} aria-hidden="true" />
          {fanBadgeLabel(s, t)}
        </span>
      ))}
    </div>
  );
}

// ---- 라인도 노드 ----

function DiagramNode({
  node,
  mode,
  size,
}: {
  node: LineDiagramNode;
  mode: LineMode;
  size: SizeTokens;
}) {
  const { t } = useTranslation();
  const status = stationStatus(node.summary.stats);
  const rows = mode === 'detail' ? stationDeviceRows(node.summary.devices, node.places) : [];
  const statusText = statusLabel(status, t);
  return (
    <div
      data-testid="diagram-node"
      data-station={node.station}
      data-status={status}
      className={cn(
        'flex flex-col gap-2 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) shadow-sm',
        size.node,
      )}
    >
      <div className="flex items-center gap-1.5">
        <span
          data-testid="station-dot"
          className={cn('shrink-0 rounded-full', size.dot, STATION_DOT[status])}
          title={statusText}
          aria-label={statusText}
        />
        <span
          className={cn('min-w-0 truncate font-medium text-(--color-text-primary)', size.name)}
          title={node.displayName}
        >
          {node.displayName}
        </span>
      </div>

      {mode === 'detail' && rows.length > 0 && (
        <ul className={cn('flex flex-col gap-1', size.row)} data-testid="device-rows">
          {rows.map((row) => {
            const badgeText = fanBadgeLabel(row.fanStatus, t);
            return (
              <li
                key={row.device.device_id}
                data-testid="device-row"
                className="flex items-center justify-between gap-2"
              >
                <span className="min-w-0 truncate text-(--color-text-secondary)">
                  {row.placeLabel}-{row.device.index}
                </span>
                <span
                  data-testid="device-fan-badge"
                  data-fan={row.fanStatus}
                  title={badgeText}
                  className={cn(
                    'inline-flex shrink-0 items-center justify-center rounded-sm font-semibold tabular-nums',
                    size.badge,
                    FAN_SWATCH[row.fanStatus],
                    FAN_TEXT[row.fanStatus],
                  )}
                >
                  {badgeText}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// ---- 라벨 헬퍼 ----

/** 역사 상태 → i18n 라벨(empty 는 라벨 없음). */
function statusLabel(status: StationStatus, t: (k: string) => string): string {
  switch (status) {
    case 'normal':
      return t('dashboard.facility.status.normal');
    case 'warning':
      return t('dashboard.facility.status.warning');
    case 'offline':
      return t('dashboard.facility.status.offline');
    case 'off':
      return t('dashboard.facility.status.off');
    case 'empty':
      return '';
  }
}

/** 풍량 배지 상태 → i18n 라벨(오프라인/꺼짐은 status 라벨 재사용, unknown 은 '?'). */
function fanBadgeLabel(status: DeviceFanStatus, t: (k: string) => string): string {
  switch (status) {
    case 'fan1':
      return t('dashboard.facility.fan.fan1');
    case 'fan2':
      return t('dashboard.facility.fan.fan2');
    case 'fan3':
      return t('dashboard.facility.fan.fan3');
    case 'offline':
      return t('dashboard.facility.status.offline');
    case 'off':
      return t('dashboard.facility.status.off');
    case 'unknown':
      return '?';
  }
}

function UnclassifiedNote({ count }: { count: number }) {
  const { t } = useTranslation();
  return (
    <p
      data-testid="unclassified-note"
      className="rounded-lg bg-amber-50 px-2.5 py-1.5 text-[11px] text-amber-600 dark:bg-amber-900/20 dark:text-amber-400"
    >
      {t('dashboard.facility.unclassified')} <b className="tabular-nums">{count}</b>
    </p>
  );
}

// ---- 로컬 셸 ----

function Shell({ title, line, children }: { title: string; line?: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {line && <span className="shrink-0 truncate text-xs text-(--color-text-muted)">{line}</span>}
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto">{children}</div>
    </div>
  );
}

function notice(text: string) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center">
      <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
      <p className="text-xs text-(--color-text-muted)">{text}</p>
    </div>
  );
}
