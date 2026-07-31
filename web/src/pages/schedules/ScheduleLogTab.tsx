// SPEC-SCHEDULE-VIEW-001 M6: 실행 로그 탭.
//
// useScheduleLogs 로 평탄한 로그 레코드(fire/result)를 조회하고 correlation_id 로
// fire+result 를 하나의 논리 행으로 병합(AC-16/AC-3)해 테이블로 렌더한다. result 가
// 없는 fire-only 그룹은 결과 컬럼 없이 표시한다(AC-15). 필터 바(schedule_id/rule_name/
// agent_id)는 apply-on-change 로 쿼리 파라미터를 좁힌다(AC-10). 로딩/에러/빈 상태를
// 명시 처리하며 빈 로그는 에러가 아닌 친절한 빈 상태로 표시한다(AC-10).
// AgentListPage/ScheduleManagementTab 레이아웃·색 토큰·한국어 문자열 관례를 따른다.

import { Fragment, useMemo, useState } from 'react';
import { AlertTriangle, ChevronDown, ChevronRight, ScrollText } from 'lucide-react';

import { useScheduleLogs } from '@/hooks/useScheduleLogs';
import type { ScheduleLogQuery } from '@/services/api/scheduleLogService';
import type { ScheduleLogTargetResult } from '@/types/schedule';
import { cn } from '@/lib/utils/cn';
import { formatDate } from '@/lib/utils/format';

import { mergeScheduleLogs, type MergedScheduleLog } from './scheduleLogMerge';

/** epoch ms → "YYYY-MM-DD HH:mm:ss". 0/무효 값은 '-'. */
function formatTriggerTime(ms: number): string {
  if (!ms || Number.isNaN(ms)) return '-';
  return formatDate(new Date(ms), 'long');
}

/** 대상별 결과 집계 요약(예: "ok=3/4"). */
function targetsSummary(targets: ScheduleLogTargetResult[]): string {
  const ok = targets.filter((t) => t.result === 'ok').length;
  return `ok=${ok}/${targets.length}`;
}

/** 결과 배지: ok=녹색, error=적색, 결과 없음(fire-only)=중립. */
function ResultBadge({ log }: { log: MergedScheduleLog }) {
  if (!log.hasResult) {
    return (
      <span
        className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-gray-700 dark:text-gray-400"
        data-testid={`log-result-pending-${log.correlationId}`}
      >
        결과 없음
      </span>
    );
  }
  const isOk = log.result === 'ok';
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
        isOk
          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
          : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
      )}
      data-testid={`log-result-${log.correlationId}`}
    >
      {log.result || '-'}
    </span>
  );
}

const inputCls =
  'min-w-40 flex-1 rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)';

export default function ScheduleLogTab() {
  // 필터 입력(apply-on-change): 각 입력이 즉시 쿼리 파라미터로 반영된다(AC-10).
  const [scheduleId, setScheduleId] = useState('');
  const [ruleName, setRuleName] = useState('');
  const [agentId, setAgentId] = useState('');

  const query = useMemo<ScheduleLogQuery>(
    () => ({
      scheduleId: scheduleId.trim() || undefined,
      ruleName: ruleName.trim() || undefined,
      agentId: agentId.trim() || undefined,
    }),
    [scheduleId, ruleName, agentId],
  );

  const { data, isLoading, isError, refetch } = useScheduleLogs(query);

  const rows = useMemo(() => mergeScheduleLogs(data ?? []), [data]);

  // 상세(대상별 결과) 펼침 상태(correlation_id Set).
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  function toggleExpand(correlationId: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(correlationId)) next.delete(correlationId);
      else next.add(correlationId);
      return next;
    });
  }

  return (
    <div className="space-y-3" data-testid="schedule-log-tab">
      {/* 필터 바: 스케줄 ID / 규칙 이름 / 에이전트 ID */}
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="text"
          value={scheduleId}
          onChange={(e) => setScheduleId(e.target.value)}
          placeholder="스케줄 ID"
          aria-label="스케줄 ID 필터"
          data-testid="log-filter-schedule-id"
          className={inputCls}
        />
        <input
          type="text"
          value={ruleName}
          onChange={(e) => setRuleName(e.target.value)}
          placeholder="규칙 이름"
          aria-label="규칙 이름 필터"
          data-testid="log-filter-rule-name"
          className={inputCls}
        />
        <input
          type="text"
          value={agentId}
          onChange={(e) => setAgentId(e.target.value)}
          placeholder="에이전트 ID"
          aria-label="에이전트 ID 필터"
          data-testid="log-filter-agent-id"
          className={inputCls}
        />
      </div>

      {/* 로딩 상태 */}
      {isLoading ? (
        <div className="space-y-2" data-testid="schedule-log-loading">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-10 animate-pulse rounded-md bg-(--color-bg-elevated)" />
          ))}
        </div>
      ) : isError ? (
        /* 에러 상태(빈 로그와 구분) */
        <div
          className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
          data-testid="schedule-log-error"
        >
          <AlertTriangle className="mx-auto h-8 w-8 text-red-400" aria-hidden="true" />
          <p className="mt-2 text-sm text-red-700 dark:text-red-400">
            실행 로그를 불러오지 못했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            다시 시도
          </button>
        </div>
      ) : rows.length === 0 ? (
        /* 빈 상태(AC-10): 에러 아님 */
        <div
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
          data-testid="schedule-log-empty"
        >
          <ScrollText className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" aria-hidden="true" />
          <p className="mt-4 text-sm text-(--color-text-muted)">표시할 실행 로그가 없습니다.</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated) text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                <th className="w-8 px-3 py-2" aria-label="상세 펼침" />
                <th className="px-3 py-2">실행 시각</th>
                <th className="px-3 py-2">스케줄 ID</th>
                <th className="px-3 py-2">규칙</th>
                <th className="px-3 py-2">에이전트</th>
                <th className="px-3 py-2">대상</th>
                <th className="px-3 py-2">동작</th>
                <th className="px-3 py-2">결과</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((log) => {
                // 상세는 대상별 결과(targets)가 있을 때만 펼칠 수 있다.
                const canExpand = log.hasResult && log.targets.length > 0;
                const isOpen = expanded.has(log.correlationId);
                // actor 가 declared 와 다르면 양쪽 노출(RD-6 불일치 가시성).
                const actorMismatch =
                  log.hasResult &&
                  log.actorAgentId !== '' &&
                  log.actorAgentId !== log.declaredAgentId;

                return (
                  <Fragment key={log.correlationId}>
                    <tr
                      data-testid={`log-row-${log.correlationId}`}
                      className="border-b border-(--color-border-default) last:border-0 hover:bg-(--color-bg-elevated)"
                    >
                      <td className="px-3 py-2 align-top">
                        {canExpand ? (
                          <button
                            type="button"
                            onClick={() => toggleExpand(log.correlationId)}
                            aria-label={isOpen ? '상세 접기' : '상세 펼치기'}
                            aria-expanded={isOpen}
                            data-testid={`log-expand-${log.correlationId}`}
                            className="rounded p-0.5 text-(--color-text-muted) hover:bg-(--color-bg-surface) hover:text-(--color-text-secondary)"
                          >
                            {isOpen ? (
                              <ChevronDown className="h-4 w-4" />
                            ) : (
                              <ChevronRight className="h-4 w-4" />
                            )}
                          </button>
                        ) : null}
                      </td>
                      <td className="px-3 py-2 align-top whitespace-nowrap tabular-nums text-(--color-text-secondary)">
                        {formatTriggerTime(log.triggerTime)}
                      </td>
                      <td className="px-3 py-2 align-top font-mono text-xs break-all text-(--color-text-primary)">
                        {log.scheduleId || '-'}
                      </td>
                      <td className="px-3 py-2 align-top text-(--color-text-secondary)">
                        {log.ruleName || '-'}
                      </td>
                      <td className="px-3 py-2 align-top text-(--color-text-secondary)">
                        <span className="font-mono text-xs">{log.declaredAgentId || '-'}</span>
                        {actorMismatch && (
                          <span
                            className="mt-0.5 flex items-center gap-1 text-[10px] text-amber-600 dark:text-amber-400"
                            data-testid={`log-actor-mismatch-${log.correlationId}`}
                            title="실제 실행 에이전트가 선언과 다릅니다"
                          >
                            <AlertTriangle className="h-3 w-3 shrink-0" />
                            <span className="font-mono break-all">실행: {log.actorAgentId}</span>
                          </span>
                        )}
                      </td>
                      {/* fire-only 는 결과 컬럼이 비어 있다(AC-15). */}
                      <td className="px-3 py-2 align-top break-all text-(--color-text-secondary)">
                        {log.hasResult ? log.target || '-' : '—'}
                      </td>
                      <td className="px-3 py-2 align-top text-(--color-text-secondary)">
                        {log.hasResult ? log.action || '-' : '—'}
                      </td>
                      <td className="px-3 py-2 align-top">
                        <div className="flex flex-col items-start gap-1">
                          <ResultBadge log={log} />
                          {log.hasResult && log.targets.length > 0 && (
                            <span className="text-[10px] tabular-nums text-(--color-text-muted)">
                              {targetsSummary(log.targets)}
                            </span>
                          )}
                        </div>
                      </td>
                    </tr>

                    {/* 대상별 결과 상세(펼침) */}
                    {canExpand && isOpen && (
                      <tr
                        data-testid={`log-targets-${log.correlationId}`}
                        className="border-b border-(--color-border-default) bg-(--color-bg-elevated)/50"
                      >
                        <td />
                        <td colSpan={7} className="px-3 py-2">
                          <table className="w-full text-xs">
                            <thead>
                              <tr className="text-left text-(--color-text-muted)">
                                <th className="py-1 pr-4 font-medium">대상</th>
                                <th className="py-1 pr-4 font-medium">결과</th>
                                <th className="py-1 font-medium">사유</th>
                              </tr>
                            </thead>
                            <tbody>
                              {log.targets.map((t, i) => (
                                <tr key={`${t.target}-${i}`}>
                                  <td className="py-1 pr-4 font-mono break-all text-(--color-text-secondary)">
                                    {t.target || '-'}
                                  </td>
                                  <td className="py-1 pr-4">
                                    <span
                                      className={cn(
                                        'inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium',
                                        t.result === 'ok'
                                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                                          : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
                                      )}
                                    >
                                      {t.result || '-'}
                                    </span>
                                  </td>
                                  <td className="py-1 break-all text-(--color-text-muted)">
                                    {t.reason || '-'}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
