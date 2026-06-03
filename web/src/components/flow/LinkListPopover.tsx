// 가상(네임드) 링크 팝오버 목록 (SPEC-LINK-001).
//
// 포트 옆 컴팩트 인디케이터(LinkIndicator)를 클릭하면 노드 "바깥"에 떠오르는
// 플로팅 목록이다. 출력 포트는 노드의 오른쪽, 입력 포트는 노드의 왼쪽에 뜬다.
//
// React Flow `NodeToolbar` 로 렌더한다. NodeToolbar 는 내부적으로 노드의 화면
// 좌표 + 뷰포트 transform(getNodeToolbarTransform)을 사용해 위치를 계산하고
// 포털로 캔버스 위에 올리므로, 줌/팬/노드 이동에 자동으로 따라가고 캔버스에
// 클리핑되지 않는다(별도 좌표 계산/포털 불필요).
//
// 각 항목 선택 시 기존 이름 그룹 하이라이트(setHighlightedLinkName)를 재사용해
// 같은 이름의 숨겨진 와이어(점선)를 일시 표시하고 상대 끝점을 강조한다.

import { forwardRef } from 'react';
import { NodeToolbar, Position, type Align } from '@xyflow/react';

import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import type { LinkListEntry } from '@/lib/flow/virtualLinks';

interface LinkListPopoverProps {
  /** 팝오버가 anchor 될 노드 id. */
  nodeId: string;
  /** 목록 방향: 출력(오른쪽) 또는 입력(왼쪽). */
  direction: 'output' | 'input';
  /** 클릭된 포트 이름(헤더 부가 표시용). */
  port: string;
  /** 노드 세로 기준 정렬(클릭한 포트 위치 근처로 맞춤). */
  align: Align;
  /** 표시할 목록 항목들(이름 그룹 단위). */
  entries: LinkListEntry[];
  /** 항목 선택 후 닫기 콜백. */
  onClose: () => void;
}

/** 빈 이름 표시용 폴백 라벨. */
const UNNAMED_LABEL = '(이름 없음)';

/**
 * 노드 측면에 떠오르는 가상 링크 목록 팝오버.
 * forwardRef 로 내부 컨테이너 ref 를 노출해 바깥-클릭 판정에 사용한다.
 */
export const LinkListPopover = forwardRef<HTMLDivElement, LinkListPopoverProps>(
  function LinkListPopover(
    { nodeId, direction, port, align, entries, onClose },
    ref,
  ) {
    const highlightedLinkName = useEditorStore((s) => s.highlightedLinkName);
    const setHighlightedLinkName = useEditorStore(
      (s) => s.setHighlightedLinkName,
    );

    const isOutput = direction === 'output';
    const header = isOutput ? '출력 링크' : '입력 링크';
    // 출력 포트는 오른쪽, 입력 포트는 왼쪽에 띄운다.
    const toolbarPosition = isOutput ? Position.Right : Position.Left;

    return (
      <NodeToolbar
        nodeId={nodeId}
        isVisible
        position={toolbarPosition}
        align={align}
        offset={8}
      >
        <div
          ref={ref}
          // 노드 바깥 플로팅 카드. 캔버스 줌과 무관하게 일정 크기를 유지한다.
          className={cn(
            'nodrag nowheel min-w-[180px] max-w-[260px] rounded-md border bg-white p-1 shadow-lg',
            'border-zinc-200 dark:border-zinc-700 dark:bg-zinc-900',
            'text-zinc-700 dark:text-zinc-200',
          )}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => e.stopPropagation()}
        >
          {/* 헤더: 방향 + 포트 이름 */}
          <div className="flex items-center justify-between gap-2 px-1.5 pb-1 pt-0.5">
            <span className="text-[10px] font-semibold text-zinc-500 dark:text-zinc-400">
              {header}
            </span>
            <span className="max-w-[120px] truncate text-[10px] text-zinc-400">
              {port}
            </span>
          </div>

          {/* 항목 목록: 이름 그룹별 1행. 상대 끝점은 노드:포트 로 표시한다. */}
          <ul className="flex flex-col gap-px">
            {entries.map((entry) => {
              const display = entry.name === '' ? UNNAMED_LABEL : entry.name;
              const highlighted =
                entry.name !== '' && highlightedLinkName === entry.name;

              const handleSelect = (e: React.MouseEvent) => {
                e.stopPropagation();
                // 빈 이름은 그룹 식별이 불가하므로 하이라이트하지 않는다.
                if (entry.name !== '') {
                  setHighlightedLinkName(entry.name);
                }
                onClose();
              };

              return (
                <li key={`${entry.port}-${entry.name}`}>
                  <button
                    type="button"
                    onClick={handleSelect}
                    title={`${header} ${display}`}
                    className={cn(
                      'flex w-full flex-col items-stretch gap-0.5 rounded px-1.5 py-1 text-left',
                      'transition-colors duration-100',
                      highlighted
                        ? 'bg-blue-100 dark:bg-blue-900/40'
                        : 'hover:bg-zinc-100 dark:hover:bg-zinc-800',
                    )}
                  >
                    {/* 링크 이름 */}
                    <span
                      className={cn(
                        'truncate text-[11px] font-medium',
                        highlighted
                          ? 'text-blue-700 dark:text-blue-300'
                          : 'text-zinc-700 dark:text-zinc-200',
                      )}
                    >
                      {display}
                    </span>
                    {/* 상대 끝점: 출력은 "→ 노드:포트", 입력은 "노드:포트 →" */}
                    <span className="flex flex-col gap-0.5">
                      {entry.counterparts.map((cp) => (
                        <span
                          key={cp.edgeId}
                          className="truncate text-[9px] leading-tight text-zinc-400"
                        >
                          {isOutput
                            ? `→ ${cp.nodeLabel}:${cp.port}`
                            : `${cp.nodeLabel}:${cp.port} →`}
                        </span>
                      ))}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      </NodeToolbar>
    );
  },
);
