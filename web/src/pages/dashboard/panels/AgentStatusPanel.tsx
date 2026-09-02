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

import { Activity, AlertTriangle, Bot, CircleStop } from 'lucide-react';

import { useAgentDetailTarget, useAgentStatsTarget } from '@/hooks/useDetailTargets';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useTargetContext } from '@/lib/remote/TargetContext';
import type { AgentSummaryStat } from '@/types/agent';

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
function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
      <p className="text-xs font-medium text-(--color-text-muted)">{label}</p>
      <p className="mt-1 text-lg font-semibold text-(--color-text-primary)">{value}</p>
    </div>
  );
}

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
          <span className="shrink-0 text-xs text-(--color-text-muted)">{displayType}</span>
        </div>
        {/* 연결/에러 상태 배지(AgentPanel status 패턴 재사용) */}
        {status === 'error' ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-red-50 px-2 py-1 text-red-500 dark:bg-red-900/30 dark:text-red-400" title={t('dashboard.error')}>
            <AlertTriangle className="h-3.5 w-3.5" />
          </span>
        ) : isConnected ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-1 text-green-600 dark:bg-green-900/30 dark:text-green-400" title={t('dashboard.panel.connected')}>
            <Activity className="h-3.5 w-3.5" />
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-1 text-slate-400 dark:bg-slate-800 dark:text-slate-500" title={t('dashboard.panel.disconnected')}>
            <CircleStop className="h-3.5 w-3.5" />
          </span>
        )}
      </div>
      )}

      {/* 본문: 상태/통계 타일 */}
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
        {/* 상태 배지 행: status / enabled(옵셔널) */}
        <div className="flex flex-wrap gap-2">
          {status && (
            <span className="inline-flex items-center rounded-full bg-(--color-bg-elevated) px-2.5 py-1 text-xs font-medium text-(--color-text-secondary)">
              {t('dashboard.agentStatus.status')}: {status}
            </span>
          )}
          {enabled !== undefined && (
            <span
              className={cn(
                'inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium',
                enabled
                  ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
                  : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
              )}
            >
              {enabled ? t('dashboard.agentStatus.enabled') : t('dashboard.agentStatus.disabled')}
            </span>
          )}
        </div>

        {/* 뷰 분기(REQ-05/REQ-06): diagram → 인라인 SVG 흐름 다이어그램, tile → 기존 통계 타일. */}
        {viewMode === 'diagram' ? (
          <AgentStatusDiagram data={data} name={displayName} />
        ) : (
          <>
            {/* 공통 요약 통계 타일 */}
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <StatCard label={t('agents.detail.stats.totalIn')} value={data.messages_in.toLocaleString()} />
              <StatCard label={t('agents.detail.stats.totalOut')} value={data.messages_out.toLocaleString()} />
              <StatCard label={t('agents.detail.stats.error')} value={data.error_count.toLocaleString()} />
              <StatCard label={t('agents.detail.stats.uptime')} value={data.uptime ?? '-'} />
            </div>

            {/* EnhancedMessagesStats 요약(external/internal) — 존재 시에만. */}
            {messages && (
              <div>
                <p className="mb-2 text-xs font-medium text-(--color-text-muted)">{t('agents.detail.stats.messageDetail')}</p>
                <div className="grid grid-cols-2 gap-3">
                  <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
                    <p className="mb-2 text-xs font-semibold text-(--color-text-secondary)">{t('agents.detail.stats.external')}</p>
                    <div className="grid grid-cols-3 gap-2 text-xs">
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.received')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.external.received ?? 0).toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.sent')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.external.sent ?? 0).toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.error')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.external.errored ?? 0).toLocaleString()}</p>
                      </div>
                    </div>
                  </div>
                  <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
                    <p className="mb-2 text-xs font-semibold text-(--color-text-secondary)">{t('agents.detail.stats.internal')}</p>
                    <div className="grid grid-cols-3 gap-2 text-xs">
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.received')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.internal.received ?? 0).toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.sent')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.internal.sent ?? 0).toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="text-(--color-text-muted)">{t('agents.detail.field.error')}</p>
                        <p className="font-semibold text-(--color-text-primary)">{(messages.internal.errored ?? 0).toLocaleString()}</p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* 운영 통계: 드롭 메시지(공통, 타입 무관). */}
            <div>
              <p className="mb-2 text-xs font-medium text-(--color-text-muted)">{t('agents.detail.stats.operationStats')}</p>
              <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
                <StatCard label={t('agents.detail.stats.droppedMessages')} value={(data.dropped_messages ?? 0).toLocaleString()} />
              </div>
            </div>
          </>
        )}

        {/* 타입별 부가 통계(REQ-07): diagram·tile 두 뷰 공통. 부재 시 생략(graceful). */}
        <AgentSummaryStats summary={data.summary_stats} t={t} />
      </div>
    </div>
  );
}
