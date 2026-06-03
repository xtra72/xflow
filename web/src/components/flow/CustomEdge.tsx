// React Flow 커스텀 엣지 컴포넌트.
// 부드러운 베지어 곡선으로 연결선을 표시하고, 호버 시 삭제 버튼을 제공한다.

import { useState, useCallback, useMemo } from 'react';
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
import { getConnectedNodeIds } from '@/lib/flow/connectionFocus';

/**
 * 커스텀 엣지 컴포넌트.
 * 선택된 엣지는 파란색으로 두껍게 표시하고, 호버 시 중간점에 삭제 버튼을 렌더링한다.
 *
 * SPEC-LINK-001 / Feature 1(개선): `virtual=true` 인 엣지는 더 이상 완전히
 * 숨기지 않는다. 가상 와이어 표시 토글이 꺼져 있어도(숨김 모드) 연결을 희미하게
 * 추적할 수 있도록 항상 옅은 점선으로 그린다. 토글이 켜지면 일반 연결선(실선)
 * 으로 그린다. 선택/그룹 하이라이트/포커스 강조 시에는 옅은 점선보다 더 또렷한
 * 파란 선으로 강조해 항상 표시되는 옅은 점선들 사이에서도 두드러지게 한다.
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

  // Feature 1: 가상 와이어 표시 토글. 켜면 가상 엣지도 일반 연결선처럼 그린다.
  const showVirtualWires = useEditorStore((s) => s.showVirtualWires);

  // Feature 2: 연결 포커스. 토글이 켜지고 단일 노드가 선택되면, 선택 노드로부터
  // focusDepth hop 이내의 연결 집합을 계산해 이 엣지의 강조/흐림을 결정한다.
  const focusOn = useEditorStore((s) => s.focusConnectionsOnSelect);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const focusDepth = useEditorStore((s) => s.focusDepth);
  // 연결 집합 계산에는 전체 엣지가 필요하다(BFS 다중 hop).
  const edges = useEditorStore((s) => s.edges);

  const virtual = edge ? isVirtualEdge(edge) : false;
  const linkName = edge ? edgeLinkName(edge) : '';
  // 가상 링크가 하이라이트 대상인지: 같은 이름 그룹이 활성화된 경우(빈 이름 제외).
  const groupHighlighted =
    virtual && linkName !== '' && highlightedLinkName === linkName;

  // 연결 포커스 활성 여부: 토글 ON + 단일 노드 선택 시에만 흐림/강조를 적용한다.
  const focusActive = focusOn && selectedNodeId !== null;
  // focusDepth hop 이내 연결 노드 집합(포커스 비활성 시 null).
  const connectedNodeIds = useMemo(
    () =>
      focusActive ? getConnectedNodeIds(edges, selectedNodeId, focusDepth) : null,
    [focusActive, edges, selectedNodeId, focusDepth],
  );
  // 이 엣지의 양 끝이 모두 연결 집합 안에 있으면 강조 대상이다(다중 hop 일반화).
  // depth=1 에서도 선택↔이웃 엣지를 그대로 강조한다.
  const bothEndpointsInSet =
    connectedNodeIds !== null &&
    edge !== undefined &&
    connectedNodeIds.has(edge.source) &&
    connectedNodeIds.has(edge.target);
  // 포커스 모드에서 양 끝이 모두 집합에 들지 않는 엣지는 흐리게 처리한다.
  const focusDimmed = focusActive && !bothEndpointsInSet;
  // 포커스 모드에서 양 끝이 모두 집합에 드는 엣지는 강조한다.
  const focusEmphasized = focusActive && bothEndpointsInSet;

  // ---- 선 강조/스타일 결정 (Feature 1 개선) ----
  // 강조(파란 선): 선택 / 그룹 하이라이트 / 포커스 강조 중 하나라도 해당하면 true.
  const emphasized = selected === true || groupHighlighted || focusEmphasized;
  // 옅은 점선(숨김 모드의 가상 와이어): 가상이고, 표시 토글이 꺼져 있고,
  // 강조 상태가 아닐 때. 사용자가 연결을 희미하게 추적할 수 있게 한다.
  const faintDotted = virtual && !showVirtualWires && !emphasized;
  // 강조된 가상 와이어는 점선으로 표시해 "원래 숨겨진 선"임을 구분한다.
  // 단, 가상 와이어 표시 토글로 항상 실선 표시 중이거나 명시적으로 선택된
  // 경우(엣지 선택)에는 실선으로 둔다.
  const emphasizedVirtualDashed =
    emphasized && virtual && !showVirtualWires && selected !== true;

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

  // Feature 1(개선): 가상 엣지도 더 이상 완전히 숨기지 않는다 — 숨김 모드에서는
  // 옅은 점선으로 항상 렌더해 연결을 희미하게 추적할 수 있게 한다(early return 제거).

  return (
    <g
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={handleClick}
      className={cn(
        'transition-opacity duration-150',
        // Feature 2: 포커스 모드에서 양 끝이 집합에 들지 않는 엣지는 흐리게 처리한다.
        focusDimmed && 'opacity-20',
        // Feature 1(개선): 숨김 모드의 가상 와이어는 옅은 점선으로 de-emphasize 한다.
        // (포커스 흐림과 동시 적용 시 더 흐려지지만 의도된 동작이다.)
        faintDotted && 'opacity-40',
      )}
    >
      {/* 엣지 경로.
          - 강조(선택/그룹 하이라이트/포커스): 또렷한 파란 선.
          - 숨김 모드의 가상 와이어(faintDotted): 옅은(muted) 점선.
          - 그 외(비가상/표시 토글 ON): 기존 회색 실선. */}
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        className={cn(
          'transition-all duration-150',
          // 강조 시 파란색. 숨김 모드 가상 와이어는 더 옅은 muted 색으로 둔다.
          emphasized
            ? '!stroke-blue-500'
            : faintDotted
              ? '!stroke-zinc-300 dark:!stroke-zinc-700'
              : '!stroke-zinc-300 dark:!stroke-zinc-600',
        )}
        style={{
          strokeWidth: emphasized ? 2.5 : faintDotted ? 1 : 1.5,
          // 점선 처리:
          //  - 숨김 모드 가상 와이어(faintDotted): 촘촘한 dotted 로 약하게 표시.
          //  - 강조된 가상 와이어: 더 또렷한 dashed 로 "숨겨진 선" 임을 구분.
          //  - 그 외: 실선.
          strokeDasharray: faintDotted
            ? '2 4'
            : emphasizedVirtualDashed
              ? '6 4'
              : undefined,
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
