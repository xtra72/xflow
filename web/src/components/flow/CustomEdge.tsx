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

/**
 * 커스텀 엣지 컴포넌트.
 * 선택된 엣지는 파란색으로 두껍게 표시하고, 호버 시 중간점에 삭제 버튼을 렌더링한다.
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

  return (
    <g
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={handleClick}
    >
      {/* 엣지 경로 */}
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        className={cn(
          'transition-all duration-150',
          selected ? '!stroke-blue-500' : '!stroke-zinc-300 dark:!stroke-zinc-600',
        )}
        style={{
          strokeWidth: selected ? 2.5 : 1.5,
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
