// SPEC-DASHBOARD-003 (REQ-05): 메시지 흐름 다이어그램(인라인 SVG).
//
// External → 에이전트 → Internal 노드를 화살표로 잇고, 각 화살표에 방향별 in/out/error
// 카운트를, 보조로 dropped/buffer/uptime 을 표기한다. 외부 차트 라이브러리 없이 자족
// 인라인 SVG(<svg><rect/line/polygon/text/>)로 구현한다(RegisterMapGrid 자족 규약 참조).
//
// messages(EnhancedMessagesStats) 부재 시 flat 필드(messages_in/out, error_count)로 폴백해
// 단일 External↔Agent 흐름만 오류 없이 표기한다(§S3 graceful, AC-05-3).

import { useTranslation } from '@/lib/i18n';
import type { AgentStatsInfo } from '@/types/agent';

interface AgentStatusDiagramProps {
  data: AgentStatsInfo;
}

/** 화살표 한 개(선 + 화살촉). direction 은 'right'|'left'. */
function Arrow({
  x1,
  x2,
  y,
  direction,
  className,
}: {
  x1: number;
  x2: number;
  y: number;
  direction: 'right' | 'left';
  className: string;
}) {
  // 화살촉 좌표(끝점 기준 삼각형).
  const head =
    direction === 'right'
      ? `${x2},${y} ${x2 - 6},${y - 3} ${x2 - 6},${y + 3}`
      : `${x2},${y} ${x2 + 6},${y - 3} ${x2 + 6},${y + 3}`;
  return (
    <>
      <line x1={x1} y1={y} x2={x2} y2={y} strokeWidth={1.5} className={className} />
      <polygon points={head} className={className} />
    </>
  );
}

/** 안전 정수화(undefined → 0). */
function n(v: number | undefined): number {
  return v ?? 0;
}

/** 단일 에이전트 메시지 흐름 다이어그램(자족 인라인 SVG). */
export default function AgentStatusDiagram({ data }: AgentStatusDiagramProps) {
  const { t } = useTranslation();

  const messages = data.messages;
  const hasInternal = !!messages;

  // External↔Agent 흐름: 중첩(external) 우선, 없으면 flat 필드 폴백(§S3, AC-05-3).
  const extIn = messages ? n(messages.external.received) : n(data.messages_in);
  const extOut = messages ? n(messages.external.sent) : n(data.messages_out);
  const extErr = messages ? n(messages.external.errored) : n(data.error_count);

  // Agent↔Internal 흐름: 중첩 존재 시에만 표기.
  const intIn = n(messages?.internal.received);
  const intOut = n(messages?.internal.sent);
  const intErr = n(messages?.internal.errored);

  // 에이전트 노드 상태 색상(배지 팔레트 재사용): error→red, connected→green, 그 외→slate.
  const agentFill =
    data.status === 'error'
      ? 'fill-red-100 stroke-red-400 dark:fill-red-900/40 dark:stroke-red-500'
      : data.connected
        ? 'fill-green-100 stroke-green-400 dark:fill-green-900/40 dark:stroke-green-500'
        : 'fill-slate-100 stroke-slate-300 dark:fill-slate-800 dark:stroke-slate-600';

  const nodeText = 'fill-(--color-text-primary) text-[10px] font-semibold';
  const labelText = 'fill-(--color-text-muted) text-[9px]';
  const nodeBox = 'fill-(--color-bg-primary) stroke-(--color-border-default)';
  const flowStroke = 'stroke-(--color-border-strong) fill-(--color-border-strong)';
  const errText = 'fill-red-500 text-[9px]';

  const buffer = data.buffer;

  return (
    <svg
      data-testid="agent-status-diagram"
      viewBox="0 0 340 190"
      className="h-auto w-full"
      role="img"
      aria-label={t('dashboard.agentStatus.diagram.title')}
    >
      {/* 노드: External */}
      <rect x={8} y={64} width={78} height={44} rx={6} strokeWidth={1} className={nodeBox} />
      <text x={47} y={90} textAnchor="middle" className={nodeText}>
        {t('dashboard.agentStatus.diagram.external')}
      </text>

      {/* 노드: Agent(상태 색상) */}
      <rect x={131} y={54} width={78} height={64} rx={6} strokeWidth={1.5} className={agentFill} />
      <text x={170} y={90} textAnchor="middle" className={nodeText}>
        {t('dashboard.agentStatus.diagram.agent')}
      </text>

      {/* External → Agent (수신/in) */}
      <Arrow x1={86} x2={131} y={76} direction="right" className={flowStroke} />
      <text x={108} y={71} textAnchor="middle" className={labelText} data-testid="agent-status-arrow-external-in">
        {t('dashboard.agentStatus.diagram.in')} {extIn.toLocaleString()}
      </text>

      {/* Agent → External (송신/out) */}
      <Arrow x1={131} x2={86} y={96} direction="left" className={flowStroke} />
      <text x={108} y={108} textAnchor="middle" className={labelText} data-testid="agent-status-arrow-external-out">
        {t('dashboard.agentStatus.diagram.out')} {extOut.toLocaleString()}
      </text>

      {/* External 오류(err) */}
      <text x={108} y={120} textAnchor="middle" className={errText} data-testid="agent-status-arrow-external-err">
        {t('dashboard.agentStatus.diagram.err')} {extErr.toLocaleString()}
      </text>

      {hasInternal && (
        <>
          {/* 노드: Internal */}
          <rect x={254} y={64} width={78} height={44} rx={6} strokeWidth={1} className={nodeBox} />
          <text x={293} y={90} textAnchor="middle" className={nodeText}>
            {t('dashboard.agentStatus.diagram.internal')}
          </text>

          {/* Agent → Internal (송신/out) */}
          <Arrow x1={209} x2={254} y={76} direction="right" className={flowStroke} />
          <text x={231} y={71} textAnchor="middle" className={labelText} data-testid="agent-status-arrow-internal-out">
            {t('dashboard.agentStatus.diagram.out')} {intOut.toLocaleString()}
          </text>

          {/* Internal → Agent (수신/in) */}
          <Arrow x1={254} x2={209} y={96} direction="left" className={flowStroke} />
          <text x={231} y={108} textAnchor="middle" className={labelText} data-testid="agent-status-arrow-internal-in">
            {t('dashboard.agentStatus.diagram.in')} {intIn.toLocaleString()}
          </text>

          {/* Internal 오류(err) */}
          <text x={231} y={120} textAnchor="middle" className={errText} data-testid="agent-status-arrow-internal-err">
            {t('dashboard.agentStatus.diagram.err')} {intErr.toLocaleString()}
          </text>
        </>
      )}

      {/* 보조 지표: dropped / buffer / uptime (AC-05-2). */}
      <text x={8} y={150} className={labelText} data-testid="agent-status-diagram-dropped">
        {t('agents.detail.stats.droppedMessages')}: {n(data.dropped_messages).toLocaleString()}
      </text>
      <text x={8} y={165} className={labelText} data-testid="agent-status-diagram-buffer">
        {t('dashboard.agentStatus.diagram.buffer')}: {n(buffer?.pending).toLocaleString()}/
        {n(buffer?.capacity).toLocaleString()}
      </text>
      <text x={8} y={180} className={labelText} data-testid="agent-status-diagram-uptime">
        {t('agents.detail.stats.uptime')}: {data.uptime ?? '-'}
      </text>
    </svg>
  );
}
