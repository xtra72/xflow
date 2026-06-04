// SPEC-SUBFLOW-001 그룹 B: 플로우 레벨 경계 포트 합성 노드.
//
// 캔버스 좌측(입력) / 우측(출력) 경계에 고정되는 슬림 카드로, 플로우 레벨 포트를
// 핸들로 노출한다(REQ-SUBFLOW-B01/B02). 핸들 id = 포트 이름이므로, 이 노드로 연결한
// 엣지는 sourceHandle/targetHandle = 포트 이름(센티넬 규약)을 운반한다.
//
//   - 입력 경계(__flow_input__) : source 핸들(우측). 사용자가 여기서 내부 노드로 와이어.
//   - 출력 경계(__flow_output__): target 핸들(좌측). 내부 노드에서 여기로 와이어.
//
// 경계 노드는 비-노드 엔티티이므로(REQ-SUBFLOW-B04) 설정 패널이 없고, 일반 노드
// 삭제 대상에서 제외된다(스토어 guard + selectable/deletable=false). 포트 추가/이름/
// 삭제는 포트 관리 패널(FlowPortPanel)에서 수행한다.

import { Handle, Position, type NodeProps } from '@xyflow/react';
import { ArrowRightToLine, ArrowLeftToLine } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import type { FlowBoundaryNodeData } from '@/lib/flow/boundary';

/**
 * 플로우 경계 노드(입력/출력) 렌더러.
 * data.direction 으로 좌/우 및 핸들 유형(source/target)을 결정하고,
 * data.ports(포트 이름 배열)로 핸들을 1개씩 그린다.
 */
export function FlowBoundaryNode({ data }: NodeProps) {
  const { direction, ports } = data as FlowBoundaryNodeData;
  const isInput = direction === 'input';

  // 입력 경계: 우측 source 핸들(내부 노드로 보냄).
  // 출력 경계: 좌측 target 핸들(내부 노드에서 받음).
  const handleType = isInput ? 'source' : 'target';
  const handlePosition = isInput ? Position.Right : Position.Left;

  const title = isInput ? '플로우 입력' : '플로우 출력';
  const Icon = isInput ? ArrowRightToLine : ArrowLeftToLine;
  const emptyHint = isInput
    ? '입력 포트 없음'
    : '출력 포트 없음';

  return (
    <div
      className={cn(
        'min-w-[120px] rounded-lg border-2 border-dashed bg-white/90 px-2 py-2 shadow-sm',
        'dark:bg-zinc-900/90',
        isInput
          ? 'border-blue-400 dark:border-blue-500/60'
          : 'border-emerald-400 dark:border-emerald-500/60',
      )}
    >
      {/* 헤더 */}
      <div
        className={cn(
          'mb-1.5 flex items-center gap-1 text-[11px] font-semibold uppercase tracking-wide',
          isInput
            ? 'text-blue-600 dark:text-blue-300'
            : 'text-emerald-600 dark:text-emerald-300',
        )}
      >
        <Icon className="h-3 w-3 shrink-0" />
        <span className="truncate">{title}</span>
      </div>

      {ports.length === 0 ? (
        <div className="px-1 py-0.5 text-[11px] italic text-zinc-400">
          {emptyHint}
        </div>
      ) : (
        <div className="flex flex-col gap-1.5">
          {ports.map((portName) => (
            <div
              key={portName}
              className={cn(
                'relative flex items-center rounded px-1.5 py-1 text-xs',
                'bg-zinc-50 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200',
                // 입력 경계는 라벨을 좌측, 핸들을 우측. 출력 경계는 반대.
                isInput ? 'justify-start pr-3' : 'justify-end pl-3',
              )}
            >
              <span className="truncate" title={portName}>
                {portName}
              </span>
              <Handle
                type={handleType}
                position={handlePosition}
                id={portName}
                title={portName}
                className={cn(
                  '!h-3 !w-3 !rounded-full !border-2 !border-white dark:!border-zinc-800',
                  isInput
                    ? '!bg-blue-500 hover:!bg-blue-400'
                    : '!bg-emerald-500 hover:!bg-emerald-400',
                )}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
