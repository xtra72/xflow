// 단일 에이전트(타입 무관) 상태·통계 대시보드 패널 (SPEC-DASHBOARD-002).
//
// 전체 `agents` 목록 패널(AgentPanel)과 달리, config.agentId 로 바인딩된 에이전트
// 하나의 타입-무관 공통 통계·상태를 실시간으로 표출한다. 사실상 에이전트 상세 화면의
// 통계 탭(AgentDetailPanel §StatsTab)을 대시보드 단일-에이전트 패널로 이식한 것이며,
// 신규 백엔드/엔드포인트 없이 기존 훅(useAgentStatsTarget + useAgentDetailTarget)만 재사용한다.
//
// 데이터 소스는 target(로컬|원격)에 따라 자동 전환된다:
//   - 로컬: 폴링(useAgentStats 위임, 5s).
//   - 원격: SSE 우선 + 폴백 폴링(그룹 J).
// 원격에서 name/type 등 상세가 제한될 수 있으므로 agentId/"-" 로 graceful 하게 대체 표기한다.
//
// 관측 전용 패널이다 — start/stop 등 제어 버튼은 노출하지 않는다(SPEC 비범위).

import { useMemo } from 'react';
import { Activity, AlertTriangle, Bot, CircleStop } from 'lucide-react';

import { useAgentDetailTarget, useAgentStatsTarget } from '@/hooks/useDetailTargets';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useTargetContext } from '@/lib/remote/TargetContext';
import type { AgentStatsInfo, AgentSummaryStat, EnhancedMessagesStats } from '@/types/agent';

import { readListPanelStyle } from './listPanelStyle';
import {
  mergeTileDesign,
  readTileDesign,
  readTileItems,
  resolveTileDesign,
  type ResolvedTileDesign,
  type TileDesign,
} from './tileSelection';
import { placeTiles, readTileGrid, type TileArea } from './tileLayout';

import AgentStatusDiagram from './AgentStatusDiagram';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';

interface AgentStatusPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 통계 타일 카드(StatsTab StatCard 패턴 이식). */
/** 설정이 없는 타일(타입별 부가 통계 등)이 쓰는 빈 디자인. */
const EMPTY_TILE_DESIGN: ResolvedTileDesign = {
  box: undefined,
  hasOwnBackground: false,
  labelStyle: undefined,
  valueStyle: undefined,
};

function StatCard({
  label,
  value,
  testId,
  design = EMPTY_TILE_DESIGN,
  area,
}: {
  label: string;
  value: string | number;
  testId?: string;
  /** 격자 안 자리. 없으면 흐름 배치(격자를 쓰지 않는 자리). */
  area?: TileArea;
  /** 타일별 디자인 — 상자·타이틀·값을 따로 얹는다. 설정이 없는 자리는 생략한다. */
  design?: ResolvedTileDesign;
}) {
  return (
    <div
      data-testid={testId}
      className={cn(
        'min-w-0 overflow-hidden rounded-lg border border-(--color-border-default) p-3',
        !design.hasOwnBackground && 'bg-(--color-bg-primary)',
      )}
      style={
        area
          ? {
              ...design.box,
              gridColumn: `${area.x} / span ${area.w}`,
              gridRow: `${area.y} / span ${area.h}`,
            }
          : design.box
      }
    >
      <p
        data-testid={testId ? `${testId}-label` : undefined}
        className="truncate text-xs font-medium text-(--color-text-muted)"
        style={design.labelStyle}
      >
        {label}
      </p>
      <p
        data-testid={testId ? `${testId}-value` : undefined}
        className="mt-1 truncate text-lg font-semibold text-(--color-text-primary)"
        style={design.valueStyle}
      >
        {value}
      </p>
    </div>
  );
}

/** 고를 수 있는 통계 타일. 순서는 기본 표시 순서다. */
const STAT_TILES = ['messagesIn', 'messagesOut', 'errors', 'uptime', 'dropped'] as const;
type StatTile = (typeof STAT_TILES)[number];

const STAT_TILE_LABEL_KEYS: Record<StatTile, string> = {
  messagesIn: 'agents.detail.stats.totalIn',
  messagesOut: 'agents.detail.stats.totalOut',
  errors: 'agents.detail.stats.error',
  uptime: 'agents.detail.stats.uptime',
  dropped: 'agents.detail.stats.droppedMessages',
};

/**
 * 값 색 규칙 판정에 쓰는 원값.
 *
 * 표시용 문자열("1,000")로는 숫자 비교가 성립하지 않는다 — 자릿점 때문에 "1,000" < "9" 가
 * 된다. 그래서 화면에 그리는 값과 규칙을 판정하는 값을 따로 둔다.
 */
const STAT_TILE_RAW: Record<StatTile, (d: AgentStatsInfo) => number | string> = {
  messagesIn: (d) => d.messages_in,
  messagesOut: (d) => d.messages_out,
  errors: (d) => d.error_count,
  uptime: (d) => d.uptime ?? '-',
  dropped: (d) => d.dropped_messages ?? 0,
};

/** 타일이 읽는 값 — 타일 이름과 통계 필드를 한곳에서 잇는다. */
const STAT_TILE_VALUE: Record<StatTile, (d: AgentStatsInfo) => string> = {
  messagesIn: (d) => d.messages_in.toLocaleString(),
  messagesOut: (d) => d.messages_out.toLocaleString(),
  errors: (d) => d.error_count.toLocaleString(),
  uptime: (d) => d.uptime ?? '-',
  dropped: (d) => (d.dropped_messages ?? 0).toLocaleString(),
};

/** 고를 수 있는 메시지 상세 타일. 외부/내부 두 묶음의 받음·보냄·오류. */
const MESSAGE_TILES = [
  'externalReceived',
  'externalSent',
  'externalErrored',
  'internalReceived',
  'internalSent',
  'internalErrored',
] as const;
type MessageTile = (typeof MESSAGE_TILES)[number];

/** 타일이 속한 묶음 — 카드는 묶음 단위로 그린다. */
const MESSAGE_TILE_GROUP: Record<MessageTile, MessageGroup> = {
  externalReceived: 'external',
  externalSent: 'external',
  externalErrored: 'external',
  internalReceived: 'internal',
  internalSent: 'internal',
  internalErrored: 'internal',
};

/** 카드 안에서 쓰는 짧은 라벨 — 묶음 이름은 카드 제목이 이미 말한다. */
const MESSAGE_TILE_LABEL_KEYS: Record<MessageTile, string> = {
  externalReceived: 'agents.detail.field.received',
  externalSent: 'agents.detail.field.sent',
  externalErrored: 'agents.detail.field.error',
  internalReceived: 'agents.detail.field.received',
  internalSent: 'agents.detail.field.sent',
  internalErrored: 'agents.detail.field.error',
};

/** 격자에 놓는 단위 — 묶음 카드. */
const MESSAGE_GROUPS = ['external', 'internal'] as const;
type MessageGroup = (typeof MESSAGE_GROUPS)[number];

const MESSAGE_GROUP_LABEL_KEYS: Record<MessageGroup, string> = {
  external: 'agents.detail.stats.external',
  internal: 'agents.detail.stats.internal',
};

/**
 * 설정 목록에서 쓰는 긴 라벨.
 *
 * 목록에는 카드 제목이 없어 짧은 라벨을 그대로 쓰면 "받음" 이 두 번 나와 어느 쪽인지
 * 가릴 수 없다.
 */
const MESSAGE_TILE_SETTING_LABEL_KEYS: Record<MessageTile, string> = {
  externalReceived: 'dashboard.settings.agentStatusOpt.msg.externalReceived',
  externalSent: 'dashboard.settings.agentStatusOpt.msg.externalSent',
  externalErrored: 'dashboard.settings.agentStatusOpt.msg.externalErrored',
  internalReceived: 'dashboard.settings.agentStatusOpt.msg.internalReceived',
  internalSent: 'dashboard.settings.agentStatusOpt.msg.internalSent',
  internalErrored: 'dashboard.settings.agentStatusOpt.msg.internalErrored',
};

const MESSAGE_TILE_VALUE: Record<MessageTile, (m: EnhancedMessagesStats) => number> = {
  externalReceived: (m) => m.external.received ?? 0,
  externalSent: (m) => m.external.sent ?? 0,
  externalErrored: (m) => m.external.errored ?? 0,
  internalReceived: (m) => m.internal.received ?? 0,
  internalSent: (m) => m.internal.sent ?? 0,
  internalErrored: (m) => m.internal.errored ?? 0,
};

/** 이 패널이 배지로 낼 수 있는 정보. */
const AGENT_BADGES = ['type', 'status', 'enabled'] as const;
type AgentBadge = (typeof AGENT_BADGES)[number];

const AGENT_BADGE_LABEL_KEYS: Record<AgentBadge, string> = {
  type: 'dashboard.settings.agentStatusOpt.badgeType',
  status: 'dashboard.agentStatus.status',
  enabled: 'dashboard.settings.agentStatusOpt.badgeEnabled',
};

/**
 * 영역별 기본 격자와 타일 크기.
 *
 * 통계는 8칸 폭에 2칸짜리 타일이라 한 줄에 넷이 들어간다. 메시지 상세는 4칸짜리 타일이라
 * 한 줄에 둘이다.
 */
const STAT_GRID_DEFAULT = { rows: 4, cols: 8 };
const STAT_TILE_SIZE = { w: 2, h: 2 };
const MESSAGE_GRID_DEFAULT = { rows: 2, cols: 8 };
const MESSAGE_TILE_SIZE = { w: 4, h: 2 };

export {
  AGENT_BADGES,
  AGENT_BADGE_LABEL_KEYS,
  MESSAGE_GROUPS,
  MESSAGE_GROUP_LABEL_KEYS,
  STAT_TILES,
  STAT_TILE_LABEL_KEYS,
  MESSAGE_TILES,
  MESSAGE_TILE_SETTING_LABEL_KEYS,
  STAT_GRID_DEFAULT,
  STAT_TILE_SIZE,
  MESSAGE_GRID_DEFAULT,
  MESSAGE_TILE_SIZE,
};
export type { StatTile, MessageTile, MessageGroup, AgentBadge };

/**
 * 타입별 부가 통계 요약(SPEC-DASHBOARD-003 REQ-07) — diagram·tile 두 뷰 공통.
 * summary_stats 부재 시 null 을 반환해 영역을 오류 없이 생략한다(graceful, AC-07-2).
 * 라벨은 안정적 key 를 i18n 매핑(`dashboard.agentStatus.summary.<key>`)하며, 미매핑 시 key 자체로 폴백(A7).
 */
function AgentSummaryStats({ summary, t }: { summary?: AgentSummaryStat[]; t: TranslationFn }) {
  if (!summary || summary.length === 0) return null;
  return (
    <div data-testid="agent-status-summary">
      <p className="mb-2 text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.agentStatus.summaryTitle')}
      </p>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {summary.map((s) => {
          const labelKey = `dashboard.agentStatus.summary.${s.key}`;
          const translated = t(labelKey);
          const label = translated === labelKey ? s.key : translated;
          const value = s.unit ? `${s.value.toLocaleString()} ${s.unit}` : s.value.toLocaleString();
          return <StatCard key={s.key} label={label} value={value} />;
        })}
      </div>
    </div>
  );
}

/** 단일 에이전트 상태·통계 패널 */
export default function AgentStatusPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: AgentStatusPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  // 상태 배지 글자 모양. 색을 비워 두면 상태별 의미색이 그대로 산다.
  const design = useMemo(() => readListPanelStyle(config), [config]);
  const agentId = (config.agentId as string | undefined) ?? '';

  // 타깃(로컬|원격)에 따라 데이터 소스가 전환된다. useTargetContext 미설정 시 로컬.
  // 실시간 통계(status/messages/error 등)와 식별 정보(name/type/enabled)는 서로 다른
  // 응답이므로 두 훅을 병행 취득한다(A3: stats 응답에는 name/type 이 없다).
  const target = useTargetContext();
  const stats = useAgentStatsTarget(target, agentId);
  const detail = useAgentDetailTarget(target, agentId, 'summary');

  // 상태 1: 미설정 — agentId 미선택(빈 화면 금지, 안내 표시).
  if (!agentId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <Bot className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.agentStatus.notConfigured')}</p>
      </div>
    );
  }

  // 상태 2: 로딩 — 통계/상세 취득 중.
  if (stats.isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        {showTitle && (
          <div className="mb-3 flex shrink-0 items-center gap-2">
            <Bot className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <span className="truncate text-sm font-medium text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
        </div>
      </div>
    );
  }

  // 상태 3: 무응답/에러 — 에이전트 미실행이거나 stats 취득 실패.
  if (stats.error || !stats.data) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <AlertTriangle className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.agentStatus.cannotLoad')}</p>
      </div>
    );
  }

  const data = stats.data;
  const info = detail.data;

  // 헤더 name/type: detail 훅에서 취득. 원격 제약 등으로 없으면 agentId/"-" 로 graceful 대체.
  const displayName = info?.name || agentId;
  const displayType = info?.type || '-';
  // status 는 stats 우선(실시간), 없으면 detail. enabled 는 detail 전용(옵셔널).
  const status = data.status || info?.status || '';
  const isConnected = data.connected === true;
  const enabled = info?.enabled;

  const messages = data.messages;

  // 출력 형식(REQ-06): config.viewMode 로 diagram|tile 전환. 미설정/미인식 값은 기본 'tile'
  // 로 폴백해 DASHBOARD-002 기존 동작을 보존한다(하위호환, AC-06-3).
  const viewMode = config.viewMode === 'diagram' ? 'diagram' : 'tile';
  // 그릴 통계 타일과 순서. 미설정이면 다섯 다 기본 순서로.
  // 모든 타일에 함께 걸리는 설정. 타일별 설정이 이 위를 덮는다.
  const commonDesign: TileDesign = {
    label_font: config.tileLabelFont,
    value_font: config.tileValueFont,
    bg: typeof config.tileBg === 'string' ? config.tileBg : undefined,
  };
  const tileDesign = (styles: unknown, item: string): TileDesign =>
    mergeTileDesign(commonDesign, readTileDesign(styles, item));

  const badgeItems = readTileItems(config.badgeItems, AGENT_BADGES);
  /** 배지 하나가 낼 글자. 값이 없으면 undefined — 빈 알약을 그리지 않는다. */
  const badgeText = (badge: AgentBadge): string | undefined => {
    switch (badge) {
      case 'type':
        return displayType !== '-' ? displayType : undefined;
      case 'status':
        return status ? `${t('dashboard.agentStatus.status')}: ${status}` : undefined;
      case 'enabled':
        return enabled === undefined
          ? undefined
          : enabled
            ? t('dashboard.agentStatus.enabled')
            : t('dashboard.agentStatus.disabled');
    }
  };

  const statTiles = readTileItems(config.statTiles, STAT_TILES);
  const statGrid = readTileGrid(config.statGrid, STAT_GRID_DEFAULT);
  const statAreas = placeTiles(
    statTiles,
    config.statTileAreas as Record<string, Partial<TileArea>> | undefined,
    statGrid,
    STAT_TILE_SIZE,
  );
  const messageGrid = readTileGrid(config.messageGrid, MESSAGE_GRID_DEFAULT);
  // 고른 값을 묶음별로 나눈다. 값이 하나도 없는 묶음은 카드 자체를 내지 않는다.
  const selectedMessageTiles = readTileItems(config.messageTiles, MESSAGE_TILES);
  const messageGroups = readTileItems(config.messageGroups, MESSAGE_GROUPS)
    .map(
      (group) =>
        [group, selectedMessageTiles.filter((tile) => MESSAGE_TILE_GROUP[tile] === group)] as const,
    )
    .filter(([, tiles]) => tiles.length > 0);
  const messageAreas = placeTiles(
    messageGroups.map(([group]) => group),
    config.messageTileAreas as Record<string, Partial<TileArea>> | undefined,
    messageGrid,
    MESSAGE_TILE_SIZE,
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {/* 헤더: name · type + 상태 배지 */}
      {showTitle && (
      <div className="mb-3 flex shrink-0 items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Bot className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-medium text-(--color-text-primary)" style={titleStyle} title={displayName}>
            {displayName}
          </span>
        </div>
        {/* 연결/에러 상태 배지(AgentPanel status 패턴 재사용) */}
        {status === 'error' ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-red-50 px-2 py-1 text-red-500 dark:bg-red-900/30 dark:text-red-400" title={t('dashboard.error')} style={design.badgeStyle}>
            <AlertTriangle className="h-3.5 w-3.5" />
          </span>
        ) : isConnected ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-1 text-green-600 dark:bg-green-900/30 dark:text-green-400" title={t('dashboard.panel.connected')} style={design.badgeStyle}>
            <Activity className="h-3.5 w-3.5" />
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-full bg-(--color-bg-sunken) px-2 py-1 text-(--color-text-muted)" title={t('dashboard.panel.disconnected')} style={design.badgeStyle}>
            <CircleStop className="h-3.5 w-3.5" />
          </span>
        )}
      </div>
      )}

      {/* 본문: 상태/통계 타일 */}
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
        {/*
          배지 — 타이틀 옆에 글자로 붙어 있던 종류를 여기로 옮겼다. 무엇을 낼지 고를 수
          있고 항목마다 모양을 정한다. 값이 없는 항목은 내지 않는다(빈 알약은 읽히지 않는다).
        */}
        {config.showBadges !== false && badgeItems.length > 0 && (
        <div className="flex flex-wrap gap-2" data-testid="agent-status-badges">
          {badgeItems.map((badge) => {
            const text = badgeText(badge);
            if (text === undefined) return null;
            const own = resolveTileDesign(readTileDesign(config.badgeStyles, badge), undefined);
            return (
              <span
                key={badge}
                data-testid={`agent-status-badge-${badge}`}
                className={cn(
                  'inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium',
                  !own.hasOwnBackground &&
                    (badge === 'enabled' && enabled
                      ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'bg-(--color-bg-elevated) text-(--color-text-secondary)'),
                )}
                style={{ ...design.badgeStyle, ...own.box, ...own.valueStyle }}
              >
                {text}
              </span>
            );
          })}
        </div>
        )}

        {/* 뷰 분기(REQ-05/REQ-06): diagram → 인라인 SVG 흐름 다이어그램, tile → 기존 통계 타일. */}
        {viewMode === 'diagram' ? (
          <AgentStatusDiagram data={data} name={displayName} />
        ) : (
          <>
            {/*
              통계 타일 — 고른 것만 고른 차례로. 종전에는 넷이 고정이고 드롭 메시지만
              "운영 통계" 라는 제목 아래 따로 있었는데, 타일 하나짜리 소제목은 고르고
              지우는 순간 자리가 비어 더 어색해진다. 한 무리로 합친다.
            */}
            {statTiles.length > 0 && (
              <div
                data-testid="agent-stat-grid"
                className="grid gap-3"
                style={{
                  gridTemplateColumns: `repeat(${statGrid.cols}, minmax(0, 1fr))`,
                  gridTemplateRows: `repeat(${statGrid.rows}, minmax(0, auto))`,
                }}
              >
                {statTiles.map((tile) => {
                  const raw = STAT_TILE_RAW[tile](data);
                  return (
                    <StatCard
                      key={tile}
                      testId={`agent-stat-${tile}`}
                      label={t(STAT_TILE_LABEL_KEYS[tile])}
                      value={STAT_TILE_VALUE[tile](data)}
                      // 값 색 규칙은 **원값**으로 판정한다 — 자릿점을 찍은 문자열("1,000")
                      // 로는 숫자 비교가 성립하지 않는다.
                      design={resolveTileDesign(tileDesign(config.statTileStyles, tile), raw)}
                      area={statAreas[tile]}
                    />
                  );
                })}
              </div>
            )}

            {/*
              메시지 상세 — 고른 값만 그린다. 값 여섯을 낱개로 고를 수 있게 하되 외부/내부
              묶음은 지킨다: 어느 쪽 수치인지가 곧 그 값의 뜻이라, 낱개로 흩으면 읽을 수 없다.
              한 묶음을 통째로 끄면 그 카드가 사라진다.
            */}
            {messages && messageGroups.length > 0 && (
              <div>
                <p className="mb-2 text-xs font-medium text-(--color-text-muted)">
                  {t('agents.detail.stats.messageDetail')}
                </p>
                {/*
                  카드 한 장이 묶음 하나(외부/내부)를 담고, 그 안에 고른 값이 들어간다.
                  어느 쪽 수치인지가 곧 그 값의 뜻이라 묶음을 흩으면 읽을 수 없다.
                  격자에 놓는 단위도 카드다 — 8x2 에 4x2 면 둘이 정확히 채운다.
                */}
                <div
                  data-testid="agent-message-grid"
                  className="grid gap-3"
                  style={{
                    gridTemplateColumns: `repeat(${messageGrid.cols}, minmax(0, 1fr))`,
                    gridTemplateRows: `repeat(${messageGrid.rows}, minmax(0, auto))`,
                  }}
                >
                  {messageGroups.map(([group, tiles]) => {
                    const design = tileDesign(config.messageTileStyles, group);
                    // 상자·타이틀은 값과 무관하다. 값 색 규칙은 값마다 따로 판정한다.
                    const chrome = resolveTileDesign(design, undefined);
                    const area = messageAreas[group];
                    return (
                      <div
                        key={group}
                        data-testid={`agent-message-group-${group}`}
                        className={cn(
                          'min-w-0 overflow-hidden rounded-lg border border-(--color-border-default) p-3',
                          !chrome.hasOwnBackground && 'bg-(--color-bg-primary)',
                        )}
                        style={{
                          ...chrome.box,
                          gridColumn: `${area.x} / span ${area.w}`,
                          gridRow: `${area.y} / span ${area.h}`,
                        }}
                      >
                        <p
                          data-testid={`agent-message-group-${group}-label`}
                          className="mb-2 truncate text-xs font-semibold text-(--color-text-secondary)"
                          style={chrome.labelStyle}
                        >
                          {t(MESSAGE_GROUP_LABEL_KEYS[group])}
                        </p>
                        <div
                          className="grid gap-2 text-xs"
                          style={{ gridTemplateColumns: `repeat(${tiles.length}, minmax(0, 1fr))` }}
                        >
                          {tiles.map((tile) => {
                            const raw = MESSAGE_TILE_VALUE[tile](messages);
                            return (
                              <div key={tile} data-testid={`agent-message-${tile}`} className="min-w-0">
                                <p className="truncate text-(--color-text-muted)">
                                  {t(MESSAGE_TILE_LABEL_KEYS[tile])}
                                </p>
                                <p
                                  data-testid={`agent-message-${tile}-value`}
                                  className="truncate font-semibold text-(--color-text-primary)"
                                  style={resolveTileDesign(design, raw).valueStyle}
                                >
                                  {raw.toLocaleString()}
                                </p>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

          </>
        )}

        {/* 타입별 부가 통계(REQ-07): diagram·tile 두 뷰 공통. 부재 시 생략(graceful). */}
        <AgentSummaryStats summary={data.summary_stats} t={t} />
      </div>
    </div>
  );
}
