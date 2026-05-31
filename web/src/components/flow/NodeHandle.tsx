// React Flow 커스텀 핸들 컴포넌트.
// 방향에 따라 색상을 다르게 표시하고, 포트 이름을 툴팁으로 제공한다.
//
// 2026-05-31: 플로우 표시 설정 (showPortStats / showPortNames) 에 따라 핸들
// 옆에 메시지 통계 수치와 포트 이름 텍스트를 표시한다.

import { Handle, Position } from '@xyflow/react';

import { cn } from '@/lib/utils/cn';

interface NodeHandleProps {
  /** 핸들 유형: source(출력) 또는 target(입력) */
  type: 'source' | 'target';
  /** 핸들 위치 */
  position: Position;
  /** 고유 핸들 ID */
  id: string;
  /** 포트 이름 (툴팁 + showName 활성 시 라벨로 사용) */
  label?: string;
  /** 에러 포트 여부 (빨간색으로 표시) */
  isError?: boolean;
  /** 같은 면에 여러 핸들이 있을 때 위치 오프셋 (퍼센트) */
  offset?: string;
  /** 2026-05-31: 본 핸들에 매핑되는 메시지 통계 수치. 값 있을 때만 옆에 표시. */
  statsCount?: number;
  /** 2026-05-31: 플로우 설정의 포트 이름 표시 토글. true 면 label 을 옆에 노출. */
  showName?: boolean;
}

/**
 * 커스텀 핸들 컴포넌트.
 * 입력은 파란색, 출력은 초록색, 에러는 빨간색으로 표시한다.
 * offset이 주어지면 Left/Right는 top, Top/Bottom은 left에 적용한다.
 */
export function NodeHandle({
  type,
  position,
  id,
  label,
  isError,
  offset,
  statsCount,
  showName,
}: NodeHandleProps) {
  const isTarget = type === 'target';
  const isVerticalSide = position === Position.Left || position === Position.Right;
  const style = offset
    ? isVerticalSide
      ? { top: offset }
      : { left: offset }
    : undefined;

  // 옆 라벨 (통계 / 이름) 위치 — handle 의 바깥쪽에 붙는다.
  // Left handle: 라벨이 핸들 더 왼쪽 (캔버스 outside) 으로 갈 수 없으므로 오른쪽 (노드 내부) 으로 표시.
  // Right handle: 라벨도 핸들 왼쪽 (노드 내부) 으로 표시.
  // 즉 좌우 핸들 공통적으로 라벨은 노드 카드 내부쪽.
  const showSidebar = (showName && label) || statsCount !== undefined;

  const sidebarPos: React.CSSProperties = {};
  if (position === Position.Left) {
    sidebarPos.left = '14px';
    sidebarPos.top = offset ?? '50%';
    sidebarPos.transform = 'translateY(-50%)';
  } else if (position === Position.Right) {
    sidebarPos.right = '14px';
    sidebarPos.top = offset ?? '50%';
    sidebarPos.transform = 'translateY(-50%)';
  } else if (position === Position.Top) {
    sidebarPos.top = '14px';
    sidebarPos.left = offset ?? '50%';
    sidebarPos.transform = 'translateX(-50%)';
  } else {
    sidebarPos.bottom = '14px';
    sidebarPos.left = offset ?? '50%';
    sidebarPos.transform = 'translateX(-50%)';
  }

  return (
    <>
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
      {showSidebar && (
        <span
          className="pointer-events-none absolute inline-flex items-center gap-1 whitespace-nowrap text-[9px] leading-none text-zinc-500 dark:text-zinc-400"
          style={sidebarPos}
        >
          {showName && label && <span className="font-medium">{label}</span>}
          {statsCount !== undefined && (
            <span className="tabular-nums">{statsCount.toLocaleString()}</span>
          )}
        </span>
      )}
    </>
  );
}
