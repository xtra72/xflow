// Facility 라인 패널 (SPEC-FACILITY-DASHBOARD-001 M4, REQ-FACDASH-001-01-*, 06-*).
//
// config 의 agentId + line 으로 로스터를 조회해 한 호선을 렌더한다(REQ-01-01):
//   (1) 노선도(metro line map): 레지스트리 order 순 역사 노드를 연속된 수평 노선 위에 역 마커로
//       배치한다. 한 행이 패널 전체 폭을 균등하게 채우도록(justify-between) 역들을 펼치고, 행을
//       가로지르는 연결선(row-spanning line, inset-x-0)이 좌우 끝까지 이어져 연속 노선처럼 보인다.
//       한 행당 역사 수(밀도)는 config.stationsPerRow 로 사용자가 직접 정하고(그 N개가 전체 폭에
//       균등 분산), 미지정/무효면 nodeSize 기반 자동 균등 배분으로 폴백한다. 각 노드는 선 위의 상태 점(정상/경고/
//       오프라인/꺼짐) + 그 아래 역사명이며, 상세(detail) 모드에서는 역 아래로 기기별 <위치명>-<index>
//       + 풍량 배지(1/2/3단·오프라인·꺼짐)를 매단다. 심플(simple) 모드는 마커+역사명만 표시한다.
//       헤더에 범례 + 심플/상세 토글을 둔다.
//   (2) 역사별 간략 상태 목록(showStationStatus), (3) 라인 통계(showLineStats), (4) 라인 일괄 제어.
// 집계는 aggregateByLine + lineDiagramLayout + stationStatus + stationDeviceRows(롤업만, UB-001),
// fan-out 은 FacilityBulkControl 호출만. 표시 옵션(showStationStatus/showLineStats/nodeSize/stationsPerRow)은
// 스냅샷 config 로만 저장한다(UB-003, 스키마 변경 없음). 심플/상세는 런타임 상태(비영속)이다.
// 미등록/빈 호선은 안내 + 일괄 제어 비활성(REQ-01-06). 미분류(미등록 station) 기기는 별도 표기(UB-004).

import { useState } from 'react';

import { HardDrive } from 'lucide-react';

import { useFacilityRoster } from '@/hooks/useAirpurifierControl';
import {
  aggregateByLine,
  countStats,
  displayDeviceFanStatus,
  displayStationStatus,
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

/**
 * 역 정보 크기(config 옵션, 5단계). 노드 + 텍스트 크기를 조절한다("역 정보 크기 조절").
 * 레벨 '3' 이 기존 lg(크게), 그 위로 '4'/'5' 를 추가한 5단계이다. 기본은 레벨 '2'(기존 md).
 */
type NodeSize = '1' | '2' | '3' | '4' | '5';

/**
 * config 의 nodeSize 값을 5단계 키로 정규화한다. 레거시 sm/md/lg 값은 각각 '1'/'2'/'3' 으로
 * 매핑해 하위 호환한다(UB-003). 미지정/불명은 기본 레벨 '2'.
 */
function toNodeSize(value: unknown): NodeSize {
  switch (value) {
    case '1':
    case '2':
    case '3':
    case '4':
    case '5':
      return value;
    case 'sm':
      return '1';
    case 'md':
      return '2';
    case 'lg':
      return '3';
    default:
      return '2';
  }
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
  /** 노드 열(column) 폭 + 좌우 여백 + 상단 여백(점이 선 위에 놓이도록). */
  node: string;
  name: string;
  dot: string;
  row: string;
  badge: string;
  /**
   * 노선(metro line): 행을 가로지르는 row-spanning 연결선(inset-x-0)이 좌우 끝까지 이어져 연속
   * 노선이 된다. lineTop 은 점(dot)의 세로 중심에 선을 맞추는 절대 top, line 은 선 두께이다.
   */
  lineTop: string;
  line: string;
}

const NODE_SIZE: Record<NodeSize, SizeTokens> = {
  '1': {
    node: 'min-w-24 max-w-40 px-2 pt-1.5',
    name: 'text-xs',
    dot: 'h-2.5 w-2.5',
    row: 'text-[11px]',
    badge: 'min-w-6 px-1 py-0.5 text-[9px]',
    lineTop: 'top-[11px]', // pt-1.5(6px) + dot 10px/2
    line: 'h-px',
  },
  '2': {
    node: 'min-w-32 max-w-52 px-3 pt-1.5',
    name: 'text-sm',
    dot: 'h-3 w-3',
    row: 'text-xs',
    badge: 'min-w-7 px-1.5 py-0.5 text-[10px]',
    lineTop: 'top-[12px]', // pt-1.5(6px) + dot 12px/2
    line: 'h-0.5',
  },
  '3': {
    node: 'min-w-44 max-w-64 px-4 pt-2',
    name: 'text-base',
    dot: 'h-3.5 w-3.5',
    row: 'text-sm',
    badge: 'min-w-8 px-2 py-1 text-xs',
    lineTop: 'top-[15px]', // pt-2(8px) + dot 14px/2
    line: 'h-0.5',
  },
  '4': {
    node: 'min-w-52 max-w-72 px-5 pt-2.5',
    name: 'text-lg',
    dot: 'h-4 w-4',
    row: 'text-base',
    badge: 'min-w-9 px-2.5 py-1 text-sm',
    lineTop: 'top-[18px]', // pt-2.5(10px) + dot 16px/2
    line: 'h-0.5',
  },
  '5': {
    node: 'min-w-64 max-w-80 px-6 pt-3',
    name: 'text-xl',
    dot: 'h-5 w-5',
    row: 'text-lg',
    badge: 'min-w-10 px-3 py-1.5 text-base',
    lineTop: 'top-[22px]', // pt-3(12px) + dot 20px/2
    line: 'h-1',
  },
};

/**
 * 노드 크기별 한 행 최대 역사 수. 노드가 클수록 한 행에 적게 배치한다(B2 균등 배분의 상한).
 * 실제 배치는 이 상한으로 필요한 행 수를 구한 뒤 balancedRows 로 행별 수를 고르게 맞춘다.
 */
const ROW_MAX: Record<NodeSize, number> = { '1': 10, '2': 8, '3': 6, '4': 5, '5': 4 };

/**
 * order 정렬된 노드 배열을 균등한 행으로 분배한다(B2 라인 균등 배분). maxPerRow 로 필요한 행
 * 수(rowCount)를 구한 뒤, 행별 개수를 ceil(n/rowCount) 로 고르게 맞춘다(예: 13, max 8 → 2행 7+6;
 * 10, max 8 → 2행 5+5). 그리디 flex-wrap 과 달리 마지막 행이 비지 않는다. 순서는 보존한다.
 */
function balancedRows<T>(items: T[], maxPerRow: number): T[][] {
  const n = items.length;
  if (n === 0) return [];
  const rowCount = Math.ceil(n / Math.max(1, maxPerRow));
  const perRow = Math.ceil(n / rowCount);
  const rows: T[][] = [];
  for (let i = 0; i < n; i += perRow) {
    rows.push(items.slice(i, i + perRow));
  }
  return rows;
}

/**
 * order 정렬된 노드 배열을 정확히 size 개씩 행으로 나눈다(사용자 지정 stationsPerRow). 마지막
 * 행만 더 적을 수 있다(예: 13, size 4 → 4/4/4/1). balancedRows(자동 균등)와 달리 각 행의 개수를
 * 사용자가 직접 정한다. 순서는 보존한다.
 */
function chunkRows<T>(items: T[], size: number): T[][] {
  const rows: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    rows.push(items.slice(i, i + size));
  }
  return rows;
}

/**
 * config 의 stationsPerRow(1줄당 역사 수) 를 정규화한다. 1 이상의 정수만 유효하며, 미지정/0/음수/
 * 비정수/불명은 null(자동 폴백 = nodeSize 기반 ROW_MAX)을 반환한다(하위 호환 UB-003). 숫자 문자열도
 * 허용한다(설정 입력이 문자열로 저장되는 경우 대비).
 */
function toStationsPerRow(value: unknown): number | null {
  const n =
    typeof value === 'number' ? value : typeof value === 'string' && value.trim() !== '' ? Number(value) : NaN;
  if (!Number.isFinite(n) || !Number.isInteger(n) || n < 1) return null;
  return n;
}

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

  // 표시 옵션(config, 영속). 기본값: 표시 on / 크기 레벨2(기존 md) / offlineAsOff off.
  const showStationStatus = (config.showStationStatus as boolean | undefined) ?? true;
  const showLineStats = (config.showLineStats as boolean | undefined) ?? true;
  const offlineAsOff = (config.offlineAsOff as boolean | undefined) ?? false;
  const nodeSize = toNodeSize(config.nodeSize);
  const size = NODE_SIZE[nodeSize];
  // 사용자 지정 1줄당 역사 수(config, 영속). 무효/미지정이면 null → nodeSize 기반 자동 배분 폴백.
  const stationsPerRow = toStationsPerRow(config.stationsPerRow);

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

  // 행 분배: stationsPerRow 지정 시 정확 청킹(사용자 제어), 미지정/무효 시 nodeSize 기반 자동 균등 배분.
  const rows = stationsPerRow ? chunkRows(diagram, stationsPerRow) : balancedRows(diagram, ROW_MAX[nodeSize]);

  return (
    <Shell title={title} line={line}>
      {/* (1) 라인도: 헤더(라벨 + [일괄 제어] + 심플/상세 토글) + 균등 배분된 역사 노드 행. */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.line.diagram')}
          </span>
          {/* B3: 헤더 우측 — 일괄 제어(OFF/풍량)를 심플/상세 토글 왼쪽에 둔다. 멤버 0 이면 비활성. */}
          <div className="flex items-center gap-2">
            <FacilityBulkControl agentId={agentId} selector={{ line }} memberCount={deviceCount} />
            <ModeToggle mode={mode} onChange={setMode} />
          </div>
        </div>

        {/* 노선도(metro line map): 노드를 행으로 나누고, 각 행은 패널 전체 폭을 균등하게 채우도록
            (justify-between) 역들을 펼친다. 행을 가로지르는 row-spanning 연결선(inset-x-0)이 좌우
            끝까지 이어져 연속 노선처럼 보인다. stationsPerRow 로 한 행당 역사 수(밀도)를 정하면 그 N개가
            전체 폭에 균등 분산된다. */}
        <div className="flex flex-col gap-y-6 pb-1" data-testid="line-diagram">
          {rows.map((rowNodes, ri) => (
            <div
              key={ri}
              data-testid="line-row"
              className="relative flex items-start justify-between"
            >
              {/* 행을 가로지르는 row-spanning 연결선(inset-x-0): 점(dot) 세로 중심에 맞춰 좌우 끝까지
                  이어진다(z-0, 노드/점 뒤). */}
              <span
                aria-hidden="true"
                data-testid="line-connector"
                className={cn('absolute inset-x-0 z-0 bg-(--color-border-strong)', size.lineTop, size.line)}
              />
              {rowNodes.map((node) => (
                <DiagramNode
                  key={node.station}
                  node={node}
                  mode={mode}
                  size={size}
                  offlineAsOff={offlineAsOff}
                />
              ))}
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

      {/* B4: 범례는 패널 하단 우측에 둔다. */}
      <div className="mt-auto flex justify-end pt-1">
        <Legend offlineAsOff={offlineAsOff} />
      </div>
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

function Legend({ offlineAsOff }: { offlineAsOff: boolean }) {
  const { t } = useTranslation();
  // offlineAsOff 켜짐: 오프라인이 꺼짐(off)으로 표시되므로 범례에서 오프라인 항목을 뺀다.
  const statusItems: StationStatus[] = offlineAsOff
    ? ['normal', 'warning', 'off']
    : ['normal', 'warning', 'offline', 'off'];
  const fanItems: DeviceFanStatus[] = offlineAsOff
    ? ['fan1', 'fan2', 'fan3']
    : ['fan1', 'fan2', 'fan3', 'offline'];
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
  offlineAsOff,
}: {
  node: LineDiagramNode;
  mode: LineMode;
  size: SizeTokens;
  offlineAsOff: boolean;
}) {
  const { t } = useTranslation();
  // 표시 상태(offlineAsOff 매핑 적용). data-status 는 표시 상태를 반영한다.
  const status = displayStationStatus(stationStatus(node.summary.stats), offlineAsOff);
  const rows = mode === 'detail' ? stationDeviceRows(node.summary.devices, node.places) : [];
  const statusText = statusLabel(status, t);
  return (
    <div
      data-testid="diagram-node"
      data-station={node.station}
      data-status={status}
      className={cn('relative flex flex-col items-center', size.node)}
    >
      {/* 역 마커(점): 행을 가로지르는 연결선(row-spanning) 위에 놓이며(z-10) 배경색 링으로 선과
          분리해 metro 정거장처럼 보이게 한다. */}
      <span
        data-testid="station-dot"
        className={cn(
          'relative z-10 shrink-0 rounded-full ring-2 ring-(--color-bg-surface)',
          size.dot,
          STATION_DOT[status],
        )}
        title={statusText}
        aria-label={statusText}
      />

      {/* 역사명: 점 아래에 매단다. */}
      <span
        className={cn('mt-1.5 max-w-full truncate text-center font-medium text-(--color-text-primary)', size.name)}
        title={node.displayName}
      >
        {node.displayName}
      </span>

      {/* 상세 모드: 역 아래로 기기 행 목록을 매단다. */}
      {mode === 'detail' && rows.length > 0 && (
        <ul
          className={cn(
            'mt-2 flex w-full flex-col gap-1 border-t border-(--color-border-default) pt-1.5',
            size.row,
          )}
          data-testid="device-rows"
        >
          {rows.map((row) => {
            const fanStatus = displayDeviceFanStatus(row.fanStatus, offlineAsOff);
            const badgeText = fanBadgeLabel(fanStatus, t);
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
                  data-fan={fanStatus}
                  title={badgeText}
                  className={cn(
                    'inline-flex shrink-0 items-center justify-center rounded-sm font-semibold tabular-nums',
                    size.badge,
                    FAN_SWATCH[fanStatus],
                    FAN_TEXT[fanStatus],
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
