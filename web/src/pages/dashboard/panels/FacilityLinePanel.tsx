// Facility 라인 패널 (SPEC-FACILITY-DASHBOARD-001 M4, REQ-FACDASH-001-01-*, 06-*).
//
// config 의 agentId + line 으로 로스터를 조회해 한 호선의 네 영역을 렌더한다(REQ-01-01):
//   (1) 라인도(line diagram): 레지스트리 order 순 역사 노드 + 상태 배지(온라인/오프라인·전원·결함),
//   (2) 역사별 간략 상태, (3) 라인 통계(StatTiles), (4) 라인 일괄 제어(line 셀렉터 fan-out).
// 집계는 aggregateByLine + lineDiagramLayout(롤업만, UB-001), fan-out 은 FacilityBulkControl 호출만.
// 미등록/빈 호선은 안내 + 일괄 제어 비활성(REQ-01-06). 미분류(미등록 station) 기기는 별도 표기(UB-004).

import { AlertTriangle, HardDrive } from 'lucide-react';

import { useFacilityRoster } from '@/hooks/useAirpurifierControl';
import {
  aggregateByLine,
  countStats,
  lineDiagramLayout,
  type LineDiagramNode,
} from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { FacilityBulkControl, StatTiles } from './facilityShared';

interface FacilityLinePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
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
      {/* (1) 라인도 */}
      <div className="space-y-1">
        <span className="text-xs font-semibold text-(--color-text-secondary)">
          {t('dashboard.facility.line.diagram')}
        </span>
        <div
          className="flex items-stretch gap-1 overflow-x-auto pb-1"
          data-testid="line-diagram"
        >
          {diagram.map((node, idx) => (
            <DiagramNode key={node.station} node={node} showConnector={idx > 0} />
          ))}
        </div>
      </div>

      {/* (2) 역사별 간략 상태 */}
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

      {/* (3) 라인 통계 */}
      <div className="space-y-1">
        <span className="text-xs font-semibold text-(--color-text-secondary)">
          {t('dashboard.facility.line.stats')}
        </span>
        <StatTiles stats={lineStats} />
      </div>

      {/* 미분류 표기(UB-004, REQ-05-03) */}
      {unclassified > 0 && <UnclassifiedNote count={unclassified} />}

      {/* (4) 라인 일괄 제어(line 셀렉터). 멤버 0 이면 비활성(REQ-06-03). */}
      <FacilityBulkControl agentId={agentId} selector={{ line }} memberCount={deviceCount} />
    </Shell>
  );
}

// ---- 라인도 노드 ----

function DiagramNode({ node, showConnector }: { node: LineDiagramNode; showConnector: boolean }) {
  const { t } = useTranslation();
  const { online, offline, powerOn, powerOff } = node.summary.stats;
  const fault = offline > 0; // 로스터에 결함 필드가 없어 오프라인을 결함 지시자로 대용.
  return (
    <div className="flex items-center gap-1">
      {showConnector && <span className="h-px w-3 shrink-0 bg-(--color-border-strong)" aria-hidden="true" />}
      <div className="flex min-w-20 flex-col items-center gap-1 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5">
        <span className="max-w-24 truncate text-[11px] font-medium text-(--color-text-primary)" title={node.displayName}>
          {node.displayName}
        </span>
        <div className="flex items-center gap-1 text-[10px] tabular-nums">
          <span className="text-blue-500" title={t('dashboard.facility.stat.online')}>
            {online}
          </span>
          <span className="text-(--color-text-muted)">/</span>
          <span className="text-slate-400" title={t('dashboard.facility.stat.offline')}>
            {offline}
          </span>
          <span className="text-(--color-text-muted)">·</span>
          <span className="text-green-600 dark:text-green-400" title={t('dashboard.facility.stat.powerOn')}>
            {powerOn}
          </span>
          <span className="text-(--color-text-muted)">/</span>
          <span className="text-(--color-text-muted)" title={t('dashboard.facility.stat.powerOff')}>
            {powerOff}
          </span>
          {fault && (
            <AlertTriangle
              className="h-3 w-3 text-amber-500"
              aria-label={t('dashboard.facility.fault')}
              data-testid="diagram-fault"
            />
          )}
        </div>
      </div>
    </div>
  );
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
