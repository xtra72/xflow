// React Flow 커스텀 노드 컴포넌트.
// 노드 유형에 따른 아이콘, 상태 표시 점, 입출력 핸들을 렌더링한다.
// 필수 설정이 누락된 노드는 좌측 상단에 경고 뱃지를 표시한다.

import { memo, useCallback, useMemo } from 'react';
import { Position, type NodeProps } from '@xyflow/react';
import {
  AlertTriangle,
  ArrowDownToLine,
  ArrowUpFromLine,
  Bug,
  Cable,
  Cog,
  Database,
  GitBranch,
  Power,
  PowerOff,
  ShieldAlert,
  Sparkles,
  type LucideIcon,
} from 'lucide-react';

import { useParams } from 'react-router';

import { getRequiredFieldErrors } from '@/config/nodeSchemas';
import { useNodeRuntimeStats } from '@/contexts/RuntimeStatsContext';
import { cn } from '@/lib/utils/cn';
import { configureNode } from '@/services/api/nodeService';
import { useEditorStore } from '@/stores/editorStore';
import { DEFAULT_FLOW_DISPLAY_SETTINGS, useUIStore } from '@/stores/uiStore';
import { APIError } from '@/types/api';
import { NodeHandle } from './NodeHandle';

/** 카테고리별 아이콘 매핑 */
const CATEGORY_ICONS: Record<string, LucideIcon> = {
  processing: Cog,
  routing: GitBranch,
  io: Cable,
  error: ShieldAlert,
  debug: Bug,
  storage: Database,
  // 레거시 호환
  input: ArrowDownToLine,
  output: ArrowUpFromLine,
  process: Cog,
  bridge: Cable,
  special: Sparkles,
};

/** 노드 data에 전달되는 속성 */
interface CustomNodeData {
  label: string;
  nodeType: string;
  category: string;
  icon?: string;
  status?: string;
  enabled?: boolean;
  ports?: { name: string; direction: 'input' | 'output' | 'error' }[];
  /** v0.18.9: output 노드 전용 — 출력 활성화 여부. 카드 우상단 ON/OFF 버튼과 연동. */
  output_enabled?: boolean;
  [key: string]: unknown;
}

/**
 * 커스텀 노드 컴포넌트.
 * 카테고리별 아이콘, 라벨, 상태 표시, 입출력 핸들을 포함하는 카드 형태로 표시한다.
 */
function CustomNodeComponent({ id, data, selected }: NodeProps) {
  const nodeData = data as CustomNodeData;
  const disabled = nodeData.enabled === false;
  const stats = useNodeRuntimeStats(id);
  const Icon = CATEGORY_ICONS[nodeData.category] ?? Cog;

  // 2026-05-31: 플로우 단위 표시 설정 (showPortStats / showPortNames).
  // 노드 단위 inMessages / outMessages 만 backend 가 제공하므로, input 핸들에
  // inMessages, output 핸들에 outMessages 합산값을 분배 (per-port backend API
  // 도입 시 정확한 분배로 교체).
  const { flowId } = useParams();
  const displaySettings = useUIStore(
    (s) => (flowId ? s.flowDisplaySettings[flowId] : undefined) ?? DEFAULT_FLOW_DISPLAY_SETTINGS,
  );
  const inMessages = stats?.inMessages;
  const outMessages = stats?.outMessages;

  // 포트 이름 → 포트별 런타임 통계 조회 맵 (방향까지 키에 포함).
  // output 포트의 delivered / 큐 적체량(messages - delivered) 표시에 사용한다.
  const portStatByKey = useMemo(() => {
    const map = new Map<string, { messages: number; delivered: number }>();
    for (const p of stats?.ports ?? []) {
      map.set(`${p.direction}:${p.name}`, {
        messages: p.messages,
        delivered: p.delivered,
      });
    }
    return map;
  }, [stats]);

  // v0.18.9: output 노드 전용 — 출력 ON/OFF 토글. 패널을 펼치지 않아도
  // 노드 카드에서 직접 토글 가능.
  const isOutputNode = nodeData.nodeType === 'output';
  const outputEnabled = nodeData.output_enabled !== false; // default true
  const updateNodeData = useEditorStore((s) => s.updateNodeData);
  const currentFlowId = useEditorStore((s) => s.currentFlowId);
  const addNotification = useUIStore((s) => s.addNotification);
  const toggleOutput = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      const next = !outputEnabled;

      // 1) 에디터 상태 갱신 — 저장 시 직렬화되어 다음 배포에 반영된다.
      updateNodeData(id, { output_enabled: next });

      // 2) 실행 중 플로우에 즉시(라이브) 적용 — 저장/재배포 없이 반영.
      //    fire-and-forget: UI 토글은 이미 완료되었으므로 await 하지 않는다.
      if (currentFlowId) {
        configureNode(currentFlowId, id, { output_enabled: next }).catch(
          (err: unknown) => {
            // 플로우가 실행 중이 아니거나 노드를 찾을 수 없으면 404.
            // 에디터 상태 변경은 다음 배포에서 반영되므로 조용히 무시한다.
            if (err instanceof APIError && err.status === 404) {
              return;
            }
            // 그 외 오류(네트워크/서버 장애 등) 는 사용자에게 경고로 알린다.
            addNotification({
              type: 'warning',
              message: '출력 설정을 실행 중 플로우에 즉시 적용하지 못했습니다. 저장 후 다시 배포하면 반영됩니다.',
            });
          },
        );
      }
    },
    [id, outputEnabled, updateNodeData, currentFlowId, addNotification],
  );

  // 필수 필드 누락 검사 (노드 카드에 경고 뱃지 표시용).
  // bridge 처럼 agent_type 에 따라 스키마가 달라지는 노드는 nodeData.agent_type 을 그대로 사용한다.
  const missingRequired = useMemo(() => {
    return getRequiredFieldErrors(
      nodeData.nodeType,
      nodeData as unknown as Record<string, unknown>,
      nodeData.agent_type as string | undefined,
    );
  }, [nodeData]);
  const hasValidationError = missingRequired.length > 0;
  const validationTooltip = hasValidationError
    ? `설정 필요:\n${missingRequired.map((e) => `• ${e.label}`).join('\n')}`
    : undefined;

  // 입력/출력/에러 포트 분리
  const inputPorts = nodeData.ports?.filter((p) => p.direction === 'input') ?? [];
  const outputPorts = nodeData.ports?.filter((p) => p.direction === 'output') ?? [];
  const errorPorts = nodeData.ports?.filter((p) => p.direction === 'error') ?? [];
  // 오른쪽 면에 배치되는 전체 포트 수 (출력 + 에러)
  const rightPorts = [...outputPorts, ...errorPorts];

  return (
    <div
      className={cn(
        'relative flex flex-col rounded-lg border bg-white px-3 py-2 shadow-sm',
        'dark:bg-zinc-900 dark:border-zinc-700',
        // 2026-05-31: min-h 로 카드 크기를 고정하고 포트 row 들이 vertical center
        // 정렬되도록 한다. 1 port 와 2 port 노드의 첫 포트 위치가 시각적으로
        // 일치 — 헤더 바로 아래가 아니라 카드의 center 영역에서 균등 분포.
        'min-h-[88px] min-w-[160px] transition-shadow duration-150',
        selected
          ? 'ring-2 ring-blue-500 border-blue-500 shadow-md'
          : 'border-zinc-200 hover:shadow-md',
        // 필수 설정이 누락된 경우 호박색 테두리로 시각화 (선택 상태가 우선)
        !selected && hasValidationError && 'border-amber-400 dark:border-amber-600',
        disabled && 'opacity-45',
      )}
    >
      {/* 2026-05-31: 우측 상단 상태 표시 점 제거 — 노드 border 색상이 동등 역할 담당. */}

      {/* 필수 설정 누락 경고 뱃지 (좌측 상단) */}
      {hasValidationError && (
        <div
          className={cn(
            'absolute -top-1.5 -left-1.5 flex h-4 w-4 items-center justify-center rounded-full',
            'bg-amber-400 text-white shadow-sm dark:bg-amber-500',
            'ring-1 ring-white dark:ring-zinc-800',
          )}
          title={validationTooltip}
          aria-label={validationTooltip}
        >
          <AlertTriangle className="h-2.5 w-2.5" />
        </div>
      )}

      {/* 아이콘 + 라벨 */}
      <div className="flex items-center gap-2">
        <div className="flex-shrink-0 rounded-md bg-zinc-100 p-1.5 dark:bg-zinc-800">
          <Icon className="h-4 w-4 text-zinc-600 dark:text-zinc-300" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">
            {nodeData.label}
          </p>
          <p className="truncate text-[10px] text-zinc-400">
            {nodeData.nodeType}
          </p>
        </div>
        {/* v0.18.9: output 노드 ON/OFF 토글 버튼 */}
        {isOutputNode && (
          <button
            type="button"
            onClick={toggleOutput}
            onMouseDown={(e) => e.stopPropagation()}
            className={cn(
              'flex-shrink-0 rounded-md p-1 transition-colors',
              outputEnabled
                ? 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/40 dark:text-emerald-400 dark:hover:bg-emerald-900/60'
                : 'bg-zinc-100 text-zinc-400 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-500 dark:hover:bg-zinc-700',
            )}
            title={outputEnabled ? '출력 ON — 클릭하여 OFF' : '출력 OFF — 클릭하여 ON'}
            aria-label={outputEnabled ? '출력 비활성화' : '출력 활성화'}
          >
            {outputEnabled ? (
              <Power className="h-3 w-3" />
            ) : (
              <PowerOff className="h-3 w-3" />
            )}
          </button>
        )}
      </div>

      {/* 2026-05-31 (재구성): 헤더 (아이콘 + 라벨) 아래로 포트 row 영역 분리.
          핸들은 row 의 좌/우 가장자리에 stick (top:50%). row 단위 inline 으로
          라벨/통계 표시 — 헤더와 겹치지 않음.
          flex-1 + justify-center 로 row 들을 카드의 남은 vertical 공간 안에서
          center 정렬 — 포트 수와 무관하게 시각적으로 균형 잡힘. */}
      {(inputPorts.length > 0 || rightPorts.length > 0 || stats) && (
        <div className="mt-1.5 flex flex-1 flex-col justify-center border-t border-zinc-100 pt-1.5 dark:border-zinc-700">
          {/* 노드 단위 누계 통계 — 포트별 통계 표시 비활성 시에만 노출 */}
          {stats && !displaySettings.showPortStats && (
            <div className="mb-1 flex items-center gap-2 text-[10px] text-zinc-400">
              <span className="inline-flex items-center gap-0.5" title="입력">
                <ArrowDownToLine className="h-2.5 w-2.5" />
                {stats.inMessages.toLocaleString()}
              </span>
              <span className="inline-flex items-center gap-0.5" title="출력">
                <ArrowUpFromLine className="h-2.5 w-2.5" />
                {stats.outMessages.toLocaleString()}
              </span>
            </div>
          )}

          {/* 2026-05-31 (재구성 v2): grid 2-column 으로 input/output 을 같은 row 에
              정렬. row 수 = max(input count, right side count). 비어 있는 cell
              도 row 높이 유지하여 좌/우 핸들이 vertical center 정렬. */}
          <div className="-mx-3 flex flex-col gap-0.5">
            {Array.from({
              length: Math.max(inputPorts.length, rightPorts.length),
            }).map((_, rowIdx) => {
              const inPort = inputPorts[rowIdx];
              const rightPort = rightPorts[rowIdx];
              const isErrorRow = rightPort
                ? rowIdx >= outputPorts.length
                : false;
              return (
                <div
                  key={`row-${rowIdx}`}
                  className="grid grid-cols-2 gap-1"
                >
                  {/* 입력 cell (좌측 핸들 + 라벨/통계 inline) */}
                  <div className="relative flex h-5 items-center pl-3.5 pr-1">
                    {inPort && (
                      <>
                        <NodeHandle
                          type="target"
                          position={Position.Left}
                          id={inPort.name}
                          label={inPort.name}
                        />
                        <span className="inline-flex items-center gap-1.5 text-[9px] leading-none text-zinc-500 dark:text-zinc-400">
                          {displaySettings.showPortNames && (
                            <span className="font-medium">{inPort.name}</span>
                          )}
                          {displaySettings.showPortStats &&
                            (() => {
                              // 포트별 통계가 있으면 해당 포트의 messages,
                              // 없으면 첫 행에 한해 노드 단위 inMessages 합산값으로 폴백.
                              const portStat = portStatByKey.get(
                                `input:${inPort.name}`,
                              );
                              const count =
                                portStat?.messages ??
                                (rowIdx === 0 ? inMessages : undefined);
                              if (count === undefined) return null;
                              return (
                                <span
                                  className="tabular-nums"
                                  title={`입력 ${count.toLocaleString()}건`}
                                >
                                  {count.toLocaleString()}
                                </span>
                              );
                            })()}
                        </span>
                      </>
                    )}
                  </div>
                  {/* 출력/에러 cell (라벨/통계 inline + 우측 핸들) */}
                  <div className="relative flex h-5 items-center justify-end pl-1 pr-3.5">
                    {rightPort && (
                      <>
                        <span
                          className={cn(
                            'inline-flex items-center gap-1.5 text-[9px] leading-none',
                            isErrorRow
                              ? 'text-red-500 dark:text-red-400'
                              : 'text-zinc-500 dark:text-zinc-400',
                          )}
                        >
                          {!isErrorRow &&
                            displaySettings.showPortStats &&
                            (() => {
                              // output 포트: 실제 전달(delivered)을 주 카운트로 표시하고,
                              // 큐 적체량(messages - delivered)이 있으면 "+N" 뱃지로 노출.
                              // 포트별 통계가 없으면 첫 행에 한해 노드 단위 outMessages
                              // 합산값으로 폴백(delivered 정보 없음 → emit 기준 표시).
                              const portStat = portStatByKey.get(
                                `output:${rightPort.name}`,
                              );
                              if (portStat) {
                                const pending = Math.max(
                                  0,
                                  portStat.messages - portStat.delivered,
                                );
                                return (
                                  <span
                                    className="inline-flex items-center gap-0.5"
                                    title={`전달 ${portStat.delivered.toLocaleString()}건 / 생성 ${portStat.messages.toLocaleString()}건${
                                      pending > 0
                                        ? ` (큐 적체 ${pending.toLocaleString()}건)`
                                        : ''
                                    }`}
                                  >
                                    <span className="tabular-nums">
                                      {portStat.delivered.toLocaleString()}
                                    </span>
                                    {pending > 0 && (
                                      <span className="rounded-sm bg-amber-100 px-0.5 font-medium tabular-nums text-amber-700 dark:bg-amber-900/40 dark:text-amber-400">
                                        +{pending.toLocaleString()}
                                      </span>
                                    )}
                                  </span>
                                );
                              }
                              if (rowIdx === 0 && outMessages !== undefined) {
                                return (
                                  <span
                                    className="tabular-nums"
                                    title={`출력 ${outMessages.toLocaleString()}건`}
                                  >
                                    {outMessages.toLocaleString()}
                                  </span>
                                );
                              }
                              return null;
                            })()}
                          {displaySettings.showPortNames && (
                            <span className="font-medium">{rightPort.name}</span>
                          )}
                        </span>
                        <NodeHandle
                          type="source"
                          position={Position.Right}
                          id={rightPort.name}
                          label={rightPort.name}
                          isError={isErrorRow}
                        />
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

export const CustomNode = memo(CustomNodeComponent);
