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

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import { edgeLinkName, isVirtualEdge } from '@/lib/flow/virtualLinks';
import { getConnectedElements } from '@/lib/flow/connectionFocus';

/**
 * 커스텀 엣지 컴포넌트.
 * 선택된 엣지는 파란색으로 두껍게 표시하고, 호버 시 중간점에 삭제 버튼을 렌더링한다.
 *
 * SPEC-LINK-001 / 가상 와이어 표시 규칙:
 *  - 숨김 모드(showVirtualWires=false): 가상 와이어 선을 완전히 그리지 않는다.
 *    연결은 노드 카드의 컴팩트 LinkIndicator 로만 표시된다.
 *  - 표시 모드(showVirtualWires=true): 가상 와이어를 점선(dotted) 으로 그려
 *    일반 실선 연결과 시각적으로 구분한다.
 *  - 클릭/그룹 하이라이트(groupHighlighted) 또는 엣지 선택(selected) 시에는
 *    두 모드 모두에서 가상 와이어를 또렷한 파란 dashed 로 드러내 강조한다
 *    (클릭으로 연결 확인하는 동작 유지).
 *
 * 연결 포커스는 "이미 보이는" 엣지에 대해서만 동작한다. 숨김 모드에서 가상
 * 엣지는 보이지 않으므로 포커스가 강조/흐림 대상으로 삼지 않는다. 표시 모드의
 * 가상 엣지는 점선 상태에서 포커스가 강조(파랑, 따라간 엣지)/흐림(opacity) 한다.
 * 비가상 엣지는 항상 실선이며 표시 규칙에 영향을 받지 않는다.
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
  const { t } = useTranslation();
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
  // focusDepth hop 이내 방향성 연결 집합(포커스 비활성 시 null).
  // 엣지 강조는 "따라간 엣지(edgeIds) 멤버십" 으로 판정한다 — 형제 엣지를 잘못
  // 강조하던 "양 끝이 집합에 듦" 규칙을 대체한다.
  const connected = useMemo(
    () =>
      focusActive ? getConnectedElements(edges, selectedNodeId, focusDepth) : null,
    [focusActive, edges, selectedNodeId, focusDepth],
  );
  // 이 엣지가 상류/하류 탐색에서 실제로 따라간(traversed) 엣지면 강조 대상이다.
  const edgeTraversed = connected !== null && connected.edgeIds.has(id);
  // 포커스 모드에서 따라가지 않은 엣지는 흐리게 처리한다(강조 엣지는 흐리지 않음).
  const focusEmphasized = focusActive && edgeTraversed;

  // ---- 선 강조/표시 규칙 ----
  // 클릭으로 연결 확인: 그룹 하이라이트 또는 엣지 선택 시 가상 와이어를 드러낸다.
  const revealedByClick = selected === true || groupHighlighted;
  // 강조(파란 선): 선택 / 그룹 하이라이트 / 포커스 강조 중 하나라도 해당하면 true.
  const emphasized = revealedByClick || focusEmphasized;
  // 포커스 모드에서 강조되지 않은 엣지는 흐리게 처리한다(클릭 강조 엣지는 제외).
  const focusDimmed = focusActive && !emphasized;

  // 숨김 모드의 가상 와이어는 클릭 강조가 없으면 선을 아예 그리지 않는다.
  const hiddenVirtual = virtual && !showVirtualWires && !revealedByClick;

  // 점선/대시 스타일 분류(렌더되는 선에 한함):
  //  - 클릭 강조된 가상 와이어: 또렷한 파란 dashed 로 드러낸다.
  //  - 표시 모드의 가상 와이어(클릭 강조 아님): dotted 로 실선과 구분한다.
  //  - 그 외(비가상): 실선.
  const virtualRevealedDashed = virtual && revealedByClick;
  const virtualShownDotted = virtual && showVirtualWires && !revealedByClick;

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

  // 숨김 모드의 가상 와이어는 클릭 강조가 없으면 선을 그리지 않는다(완전 숨김).
  // 연결은 노드 카드의 LinkIndicator 로만 표시된다. 포커스 모드여도 보이지 않는
  // 엣지는 강조/흐림 대상이 아니므로 그대로 렌더하지 않는다.
  if (hiddenVirtual) return null;

  return (
    <g
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={handleClick}
      className={cn(
        'transition-opacity duration-150',
        // 포커스 모드에서 따라가지 않은(강조 아님) 엣지는 흐리게 처리한다.
        focusDimmed && 'opacity-20',
      )}
    >
      {/* 엣지 경로.
          - 강조(선택/그룹 하이라이트/포커스): 또렷한 파란 선.
          - 클릭 강조된 가상 와이어: 파란 dashed 로 드러낸다.
          - 표시 모드의 가상 와이어: dotted 로 실선과 구분(포커스 시 파랑/흐림).
          - 그 외(비가상): 기존 회색 실선. */}
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        className={cn(
          'transition-all duration-150',
          // 강조 시 파란색, 그 외 회색.
          emphasized
            ? '!stroke-blue-500'
            : '!stroke-zinc-300 dark:!stroke-zinc-600',
        )}
        style={{
          strokeWidth: emphasized ? 2.5 : virtualShownDotted ? 1.25 : 1.5,
          // 대시 처리:
          //  - 클릭 강조된 가상 와이어: 또렷한 dashed("6 4") 로 드러낸다.
          //  - 표시 모드의 가상 와이어: dotted("2 4") 로 실선과 구분한다.
          //  - 그 외: 실선(undefined).
          strokeDasharray: virtualRevealedDashed
            ? '6 4'
            : virtualShownDotted
              ? '2 4'
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
            title={t('editor.edge.deleteTitle')}
          >
            <X className="h-3 w-3" />
          </button>
        </foreignObject>
      )}
    </g>
  );
}
