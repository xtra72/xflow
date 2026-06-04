// SPEC-SUBFLOW-001 M4: 실제 노드 전체를 감싸는 "영역(바운딩 박스)" 표시 합성 노드.
//
// 경계 포트(__flow_input__ / __flow_output__)가 붙는 영역을 사용자에게 보여주기 위한
// 배경 사각형이다. 실제 노드들의 바운딩 박스를 따라가며(노드 이동 시 함께 갱신),
// 옅은 점선 테두리 + 매우 옅은 채움으로만 그린다.
//
// 비-노드 엔티티이므로(boundary.ts isSyntheticNode) 저장 `nodes` 에서 제외되고,
// 바운딩 박스 계산에서도 제외된다(피드백 루프 방지). 비선택·비삭제·비드래그이며
// zIndex<0 으로 다른 노드 아래에 깔린다. 포인터 이벤트도 받지 않아 상호작용을 막는다.

import type { NodeProps } from '@xyflow/react';

import type { FlowAreaNodeData } from '@/lib/flow/boundary';

/**
 * 영역 표시 노드 렌더러.
 * data.width / data.height 크기의 옅은 점선 사각형을 그린다(상호작용 없음).
 */
export function FlowAreaNode({ data }: NodeProps) {
  const { width, height } = data as FlowAreaNodeData;

  return (
    <div
      // 순수 표시용: 클릭/드래그 등 모든 포인터 이벤트를 통과시킨다.
      style={{ width, height, pointerEvents: 'none' }}
      className="rounded-2xl border-2 border-dashed border-zinc-300/70 bg-zinc-400/5 dark:border-zinc-600/60 dark:bg-zinc-400/5"
      aria-hidden="true"
    />
  );
}
