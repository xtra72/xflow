// React Flow 커스텀 엣지 컴포넌트.
// 부드러운 베지어 곡선으로 연결선을 표시하고, 호버 시 삭제 버튼을 제공한다.

import { useState, useCallback } from 'react';
import {
  BaseEdge,
  getBezierPath,
  useReactFlow,
  type EdgeProps,
} from '@xyflow/react';
import { X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import { edgeLinkName, isVirtualEdge } from '@/lib/flow/virtualLinks';

/**
 * 커스텀 엣지 컴포넌트.
 * 선택된 엣지는 파란색으로 두껍게 표시하고, 호버 시 중간점에 삭제 버튼을 렌더링한다.
 *
 * SPEC-LINK-001: `virtual=true` 인 엣지는 평소 긴 연결선을 그리지 않는다(REQ-LINK-020).
 * 단, 해당 엣지가 선택되었거나 같은 이름 그룹이 하이라이트된 경우에는 연결성을
 * 보여주기 위해 선을 일시적으로(강조하여) 그린다(REQ-LINK-024).
 */
export function CustomEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  selected,
  markerEnd,
}: EdgeProps) {
  const [hovered, setHovered] = useState(false);
  const { deleteElements } = useReactFlow();
  const selectEdge = useEditorStore((s) => s.selectEdge);

  // 이 엣지의 가상화/이름은 스토어의 최상위 속성(`virtual`/`name`)에서 읽는다.
  // React Flow 의 EdgeProps 는 `data` 만 노출하므로 스토어 조회로 보강한다.
  const edge = useEditorStore((s) => s.edges.find((e) => e.id === id));
  const highlightedLinkName = useEditorStore((s) => s.highlightedLinkName);

  const virtual = edge ? isVirtualEdge(edge) : false;
  const linkName = edge ? edgeLinkName(edge) : '';
  // 가상 링크가 하이라이트 대상인지: 같은 이름 그룹이 활성화된 경우(빈 이름 제외).
  const groupHighlighted =
    virtual && linkName !== '' && highlightedLinkName === linkName;
  // 가상 엣지를 화면에 그릴지 여부: 비가상은 항상 그린다(하위 호환, REQ-LINK-025).
  // 가상은 선택되었거나 같은 이름 그룹이 하이라이트된 경우에만 그린다.
  const showPath = !virtual || selected === true || groupHighlighted;

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  // 엣지 삭제 처리
  const handleDelete = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      deleteElements({ edges: [{ id }] });
    },
    [id, deleteElements],
  );

  // 엣지 클릭 시 선택
  const handleClick = useCallback(() => {
    selectEdge(id);
  }, [id, selectEdge]);

  // SPEC-LINK-001: 가상 엣지가 숨김 상태이면 캔버스에 아무것도 그리지 않는다.
  // 양 끝 배지는 CustomNode 의 포트 영역에서 렌더한다(decision #3).
  if (!showPath) {
    return null;
  }

  return (
    <g
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={handleClick}
    >
      {/* 엣지 경로. 가상 링크가 하이라이트로 일시 표시될 때는 점선으로 강조한다. */}
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        className={cn(
          'transition-all duration-150',
          selected || groupHighlighted
            ? '!stroke-blue-500'
            : '!stroke-zinc-300 dark:!stroke-zinc-600',
        )}
        style={{
          strokeWidth: selected || groupHighlighted ? 2.5 : 1.5,
          // 가상 링크를 일시 표시할 때는 점선으로 "원래 숨겨진 선"임을 구분한다.
          strokeDasharray: groupHighlighted && !selected ? '6 4' : undefined,
        }}
      />

      {/* 호버 시 삭제 버튼 */}
      {hovered && (
        <foreignObject
          x={labelX - 10}
          y={labelY - 10}
          width={20}
          height={20}
          className="pointer-events-auto overflow-visible"
        >
          <button
            type="button"
            onClick={handleDelete}
            className={cn(
              'flex h-5 w-5 items-center justify-center rounded-full',
              'bg-red-500 text-white shadow-sm',
              'hover:bg-red-600 transition-colors duration-100',
            )}
            title="연결 삭제"
          >
            <X className="h-3 w-3" />
          </button>
        </foreignObject>
      )}
    </g>
  );
}
