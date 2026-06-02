// React Flow 커스텀 핸들 컴포넌트.
//
// 2026-05-31 (재구성): NodeHandle 은 핸들 도형만 담당한다. 라벨/통계는 caller
// 측 (CustomNode) 의 row 컨테이너가 inline 으로 표시한다. 이전 sidebar absolute
// 라벨 패턴은 헤더 영역과 겹치는 문제로 폐기.
//
// position 은 Left/Right 만 지원하고 row 단위 레이아웃의 row center 에 stick
// 시킨다 (top:50% + 좌/우 가장자리). 기존 offset prop 은 보존 (다중 포트 row
// 없이 단일 노드에서 핸들 위치를 직접 지정해야 하는 케이스 — 호환).

import { Handle, Position } from '@xyflow/react';

import { cn } from '@/lib/utils/cn';

interface NodeHandleProps {
  /** 핸들 유형: source(출력) 또는 target(입력) */
  type: 'source' | 'target';
  /** 핸들 위치 */
  position: Position;
  /** 고유 핸들 ID */
  id: string;
  /** 툴팁 텍스트 (마우스 hover 시 노출) */
  label?: string;
  /** 에러 포트 여부 (빨간색으로 표시) */
  isError?: boolean;
  /** 같은 면에 여러 핸들이 있을 때 위치 오프셋 (퍼센트). row 레이아웃 미사용 시. */
  offset?: string;
}

export function NodeHandle({ type, position, id, label, isError, offset }: NodeHandleProps) {
  const isTarget = type === 'target';
  const isVerticalSide = position === Position.Left || position === Position.Right;
  const style = offset
    ? isVerticalSide
      ? { top: offset }
      : { left: offset }
    : undefined;

  return (
    <Handle
      type={type}
      position={position}
      id={id}
      title={label}
      style={style}
      className={cn(
        '!w-3 !h-3 !rounded-full !border-2 !border-white dark:!border-zinc-800',
        'transition-colors duration-150',
        isError
          ? '!bg-red-500 hover:!bg-red-400'
          : isTarget
            ? '!bg-blue-500 hover:!bg-blue-400'
            : '!bg-emerald-500 hover:!bg-emerald-400',
      )}
    />
  );
}
