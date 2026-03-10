// React Flow 커스텀 핸들 컴포넌트.
// 방향에 따라 색상을 다르게 표시하고, 포트 이름을 툴팁으로 제공한다.

import { Handle, Position } from '@xyflow/react';

import { cn } from '@/lib/utils/cn';

interface NodeHandleProps {
  /** 핸들 유형: source(출력) 또는 target(입력) */
  type: 'source' | 'target';
  /** 핸들 위치 */
  position: Position;
  /** 고유 핸들 ID */
  id: string;
  /** 포트 이름 (툴팁으로 표시) */
  label?: string;
  /** 에러 포트 여부 (빨간색으로 표시) */
  isError?: boolean;
  /** 같은 면에 여러 핸들이 있을 때 위치 오프셋 (퍼센트) */
  offset?: string;
}

/**
 * 커스텀 핸들 컴포넌트.
 * 입력은 파란색, 출력은 초록색, 에러는 빨간색으로 표시한다.
 * offset이 주어지면 Left/Right는 top, Top/Bottom은 left에 적용한다.
 */
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
