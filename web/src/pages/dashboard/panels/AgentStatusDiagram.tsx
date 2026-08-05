// SPEC-DASHBOARD-003 (REQ-05): 메시지 흐름 다이어그램(인라인 SVG).
//
// 외부 ↔ 에이전트 ↔ 내부 노드를 화살표로 잇는다. 방향은 화살표로만 나타내고 라벨에는
// 숫자만 표기하며(in/out 접두어 없음), 오류는 수신(in) 라벨 아래에 빨간색 숫자로 둔다.
// buffer(pending/capacity)·uptime 은 에이전트 박스 내부 하단 좌/우 코너에, 드롭 메시지는
// 아래쪽 "쓰레기통"(DB 실린더)으로 분리해 하향 화살표 + 카운트로 표기한다.
// 외부 차트 라이브러리 없이 자족 인라인 SVG로 구현한다(RegisterMapGrid 자족 규약 참조).
//
// messages(EnhancedMessagesStats) 부재 시 flat 필드(messages_in/out, error_count)로 폴백해
// 단일 외부↔에이전트 흐름만 오류 없이 표기하고 내부 노드·화살표는 생략한다(§S3, AC-05-3).

import { useTranslation } from '@/lib/i18n';
import type { AgentStatsInfo } from '@/types/agent';

interface AgentStatusDiagramProps {
  data: AgentStatsInfo;
  /** 중앙 에이전트 노드에 표기할 실제 타이틀(부재 시 일반 라벨로 폴백). */
  name?: string;
}

/** 화살표 한 개(선 + 화살촉). 화살촉은 끝점(x2,y2)에 direction 방향으로 그린다. */
function Arrow({
  x1,
  y1,
  x2,
  y2,
  direction,
  className,
}: {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  direction: 'right' | 'left' | 'down';
  className: string;
}) {
  const head =
    direction === 'right'
      ? `${x2},${y2} ${x2 - 8},${y2 - 4} ${x2 - 8},${y2 + 4}`
      : direction === 'left'
        ? `${x2},${y2} ${x2 + 8},${y2 - 4} ${x2 + 8},${y2 + 4}`
        : `${x2},${y2} ${x2 - 4},${y2 - 8} ${x2 + 4},${y2 - 8}`;
  return (
    <>
      <line x1={x1} y1={y1} x2={x2} y2={y2} strokeWidth={2} className={className} />
      <polygon points={head} className={className} />
    </>
  );
}

/** 안전 정수화(undefined → 0). */
function n(v: number | undefined): number {
  return v ?? 0;
}

/** 단일 에이전트 메시지 흐름 다이어그램(자족 인라인 SVG). */
export default function AgentStatusDiagram({ data, name }: AgentStatusDiagramProps) {
  const { t } = useTranslation();

  // 중앙 노드 라벨: 실제 에이전트 타이틀(name) 우선, 부재 시 기존 일반 라벨로 폴백.
  // 노드 폭 제약상 과도한 길이는 말줄임하고, 전체명은 <title> 툴팁으로 제공한다.
  const agentFullLabel = name?.trim() || t('dashboard.agentStatus.diagram.agent');
  const agentLabel = agentFullLabel.length > 16 ? `${agentFullLabel.slice(0, 15)}…` : agentFullLabel;

  const messages = data.messages;
  const hasInternal = !!messages;

  // 외부↔에이전트 흐름: 중첩(external) 우선, 없으면 flat 필드 폴백(§S3, AC-05-3).
  const extIn = messages ? n(messages.external.received) : n(data.messages_in);
  const extOut = messages ? n(messages.external.sent) : n(data.messages_out);
  const extErr = messages ? n(messages.external.errored) : n(data.error_count);

  // 에이전트↔내부 흐름: 중첩 존재 시에만 표기.
  const intIn = n(messages?.internal.received);
  const intOut = n(messages?.internal.sent);
  const intErr = n(messages?.internal.errored);

  // 에이전트 노드 테두리 색상(배지 팔레트 재사용): error→red, connected→green, 그 외→기본.
  const agentStroke =
    data.status === 'error'
      ? 'stroke-red-400 dark:stroke-red-500'
      : data.connected
        ? 'stroke-green-400 dark:stroke-green-500'
        : 'stroke-(--color-border-strong)';

  const darkNode = 'fill-slate-500 stroke-slate-600 dark:fill-slate-600 dark:stroke-slate-500';
  const darkNodeText = 'fill-white text-[12px] font-semibold';
  const agentBox = 'fill-(--color-bg-primary)';
  const agentName = 'fill-(--color-text-primary) text-[14px] font-bold';
  const flowStroke = 'stroke-(--color-text-primary) fill-(--color-text-primary)';
  const countText = 'fill-(--color-text-primary) text-[11px] font-semibold';
  const errText = 'fill-red-500 text-[11px] font-semibold';
  const metaText = 'fill-(--color-text-muted) text-[10px]';
  const cylinder = 'fill-(--color-bg-primary) stroke-(--color-border-strong)';
  const trashLabel = 'fill-(--color-text-secondary) text-[10px]';

  const buffer = data.buffer;

  return (
    <svg
      data-testid="agent-status-diagram"
      viewBox="0 0 480 290"
      className="h-auto w-full"
      role="img"
      aria-label={t('dashboard.agentStatus.diagram.title')}
    >
      {/* 노드: 외부(진한 회색) */}
      <rect x={24} y={76} width={92} height={104} rx={10} strokeWidth={1.5} className={darkNode} />
      <text x={70} y={132} textAnchor="middle" className={darkNodeText}>
        {t('dashboard.agentStatus.diagram.external')}
      </text>

      {/* 노드: 에이전트(흰 박스 + 상태 테두리) */}
      <rect x={168} y={58} width={144} height={140} rx={14} strokeWidth={2} className={`${agentBox} ${agentStroke}`} />
      <text x={240} y={122} textAnchor="middle" className={agentName} data-testid="agent-status-diagram-agent">
        {agentLabel}
        <title>{agentFullLabel}</title>
      </text>
      {/* 박스 내부 하단: buffer(좌) / uptime(우) */}
      <text x={182} y={186} className={metaText} data-testid="agent-status-diagram-buffer">
        {n(buffer?.pending).toLocaleString()}/{n(buffer?.capacity).toLocaleString()}
      </text>
      <text x={298} y={186} textAnchor="end" className={metaText} data-testid="agent-status-diagram-uptime">
        {data.uptime ?? '-'}
      </text>

      {/* 외부: 상단 화살표(에이전트 → 외부, 송신/out) */}
      <Arrow x1={168} y1={106} x2={116} y2={106} direction="left" className={flowStroke} />
      <text x={142} y={98} textAnchor="middle" className={countText} data-testid="agent-status-arrow-external-out">
        {extOut.toLocaleString()}
      </text>

      {/* 외부: 하단 화살표(외부 → 에이전트, 수신/in) + 오류(빨강) */}
      <Arrow x1={116} y1={150} x2={168} y2={150} direction="right" className={flowStroke} />
      <text x={142} y={142} textAnchor="middle" className={countText} data-testid="agent-status-arrow-external-in">
        {extIn.toLocaleString()}
      </text>
      <text x={142} y={162} textAnchor="middle" className={errText} data-testid="agent-status-arrow-external-err">
        {extErr.toLocaleString()}
      </text>

      {hasInternal && (
        <>
          {/* 노드: 내부(진한 회색) */}
          <rect x={364} y={76} width={92} height={104} rx={10} strokeWidth={1.5} className={darkNode} />
          <text x={410} y={132} textAnchor="middle" className={darkNodeText}>
            {t('dashboard.agentStatus.diagram.internal')}
          </text>

          {/* 내부: 상단 화살표(에이전트 → 내부, 송신/out) */}
          <Arrow x1={312} y1={106} x2={364} y2={106} direction="right" className={flowStroke} />
          <text x={338} y={98} textAnchor="middle" className={countText} data-testid="agent-status-arrow-internal-out">
            {intOut.toLocaleString()}
          </text>

          {/* 내부: 하단 화살표(내부 → 에이전트, 수신/in) + 오류(빨강) */}
          <Arrow x1={364} y1={150} x2={312} y2={150} direction="left" className={flowStroke} />
          <text x={338} y={142} textAnchor="middle" className={countText} data-testid="agent-status-arrow-internal-in">
            {intIn.toLocaleString()}
          </text>
          <text x={338} y={162} textAnchor="middle" className={errText} data-testid="agent-status-arrow-internal-err">
            {intErr.toLocaleString()}
          </text>
        </>
      )}

      {/* 드롭 메시지: 에이전트 → 쓰레기통(DB 실린더) 하향 흐름 */}
      <Arrow x1={240} y1={198} x2={240} y2={232} direction="down" className={flowStroke} />
      <text x={252} y={218} className={countText} data-testid="agent-status-diagram-dropped">
        {n(data.dropped_messages).toLocaleString()}
      </text>
      {/* 실린더: 상단 림 + 몸통(측면 + 하단 곡선) */}
      <path d="M222 236 V256 A18 5 0 0 0 258 256 V236" strokeWidth={1.5} className={cylinder} />
      <ellipse cx={240} cy={236} rx={18} ry={5} strokeWidth={1.5} className={cylinder} />
      <text x={240} y={276} textAnchor="middle" className={trashLabel}>
        {t('dashboard.agentStatus.diagram.dropped')}
      </text>
    </svg>
  );
}
