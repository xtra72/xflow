// 도면 직접 배치 오버레이 — 드래그로 이동, 모서리 핸들로 크기 조절.
//
// 좌표 모델: 스테이지는 도면과 같은 종횡비를 갖고 센서 마커는 스테이지 정규화(0..1) 좌표다.
// 따라서 "도면을 옮긴다 = 스테이지를 옮긴다" 이고, 변형을 스테이지 박스에만 적용하면 마커는
// 좌표 재계산 없이 도면 위 같은 지점에 붙어 따라온다(stage.ts applyStageTransform).
//
// 이 컴포넌트는 변형 값만 만들어 올린다 — 실제 적용은 HeatmapPanel 이 스테이지 계산에서 한다.
// 배치 편집 중에만 마운트되며, 뷰어 렌더에는 존재하지 않는다.

import { useCallback, useRef, useState } from 'react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import type { StageBox, StageTransform } from './stage';

/** 크기 조절 핸들 위치(네 모서리). */
const HANDLES = ['nw', 'ne', 'sw', 'se'] as const;
type Handle = (typeof HANDLES)[number];

const HANDLE_CLASS: Record<Handle, string> = {
  nw: '-left-1 -top-1 cursor-nwse-resize',
  ne: '-right-1 -top-1 cursor-nesw-resize',
  sw: '-bottom-1 -left-1 cursor-nesw-resize',
  se: '-bottom-1 -right-1 cursor-nwse-resize',
};

/** 배율 하한/상한 — 스테이지가 사라지거나 터무니없이 커지는 것을 막는다. */
const MIN_SCALE = 0.1;
const MAX_SCALE = 10;

function clampScale(v: number): number {
  return v < MIN_SCALE ? MIN_SCALE : v > MAX_SCALE ? MAX_SCALE : v;
}

interface DragState {
  kind: 'move' | Handle;
  startX: number;
  startY: number;
  base: StageTransform;
}

export interface FloorPlanTransformOverlayProps {
  /** 현재(변형이 적용된) 스테이지 박스 — 아웃라인을 그릴 위치다. */
  stage: StageBox;
  /** 패널 본문 픽셀 크기 — 이동량을 비율로 환산하는 기준. */
  container: { width: number; height: number };
  /** 현재 변형 값. */
  transform: StageTransform | undefined;
  /** 변형 확정 콜백(드래그 중 연속 호출). */
  onChange: (next: StageTransform) => void;
  /** 변형 초기화 콜백(원래 맞춤으로 되돌림). */
  onReset: () => void;
}

/**
 * 도면 아웃라인 + 이동/크기조절 핸들.
 *
 * 이동은 컨테이너 대비 비율로 누적하므로 패널 크기가 바뀌어도 상대 위치가 유지된다.
 * 크기 조절은 잡은 모서리의 대각선 이동량을 현재 스테이지 폭으로 나눈 비율로 배율을 만든다 —
 * 배율은 중심 고정이라 어느 모서리를 잡아도 같은 방식으로 커지고 작아진다.
 */
export default function FloorPlanTransformOverlay({
  stage,
  container,
  transform,
  onChange,
  onReset,
}: FloorPlanTransformOverlayProps) {
  const { t } = useTranslation();
  const [drag, setDrag] = useState<DragState | null>(null);
  const dragRef = useRef<DragState | null>(null);

  const begin = useCallback(
    (kind: DragState['kind']) => (e: React.PointerEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const st: DragState = {
        kind,
        startX: e.clientX,
        startY: e.clientY,
        base: {
          offset_x: transform?.offset_x ?? 0,
          offset_y: transform?.offset_y ?? 0,
          scale: transform?.scale ?? 1,
        },
      };
      dragRef.current = st;
      setDrag(st);
      (e.target as Element).setPointerCapture?.(e.pointerId);
    },
    [transform],
  );

  const move = useCallback(
    (e: React.PointerEvent) => {
      const st = dragRef.current;
      if (!st) return;
      const dx = e.clientX - st.startX;
      const dy = e.clientY - st.startY;
      if (st.kind === 'move') {
        if (!(container.width > 0) || !(container.height > 0)) return;
        onChange({
          ...st.base,
          offset_x: (st.base.offset_x ?? 0) + dx / container.width,
          offset_y: (st.base.offset_y ?? 0) + dy / container.height,
        });
        return;
      }
      // 크기 조절: 바깥으로 끌면 커지고 안으로 끌면 작아진다. 잡은 모서리에 따라 부호가 바뀐다.
      if (!(stage.width > 0)) return;
      const signX = st.kind === 'ne' || st.kind === 'se' ? 1 : -1;
      const signY = st.kind === 'sw' || st.kind === 'se' ? 1 : -1;
      const delta = (dx * signX + dy * signY) / 2;
      const baseScale = st.base.scale ?? 1;
      // 현재 스테이지 폭 기준 비율 → 배율 증분. 중심 고정이라 양쪽이 같이 늘어난다.
      onChange({ ...st.base, scale: clampScale(baseScale + (delta * 2) / stage.width) });
    },
    [container.width, container.height, stage.width, onChange],
  );

  const end = useCallback(() => {
    dragRef.current = null;
    setDrag(null);
  }, []);

  return (
    <div
      data-testid="floorplan-transform-overlay"
      className="absolute z-[22]"
      style={{ left: stage.left, top: stage.top, width: stage.width, height: stage.height }}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
    >
      {/* 아웃라인 + 이동 히트영역. 내부를 잡고 끌면 도면이 움직인다. */}
      <div
        data-testid="floorplan-transform-body"
        onPointerDown={begin('move')}
        className={cn(
          'absolute inset-0 border-2 border-dashed border-amber-400/80',
          drag?.kind === 'move' ? 'cursor-grabbing' : 'cursor-grab',
        )}
      />
      {HANDLES.map((h) => (
        <span
          key={h}
          data-testid={`floorplan-transform-handle-${h}`}
          onPointerDown={begin(h)}
          className={cn(
            'absolute h-2.5 w-2.5 rounded-sm border border-white bg-amber-400 shadow',
            HANDLE_CLASS[h],
          )}
        />
      ))}
      <button
        type="button"
        data-testid="floorplan-transform-reset"
        onPointerDown={(e) => e.stopPropagation()}
        onClick={onReset}
        className="absolute -top-6 left-0 rounded bg-amber-400/90 px-1.5 py-0.5 text-[10px] leading-tight text-white shadow hover:bg-amber-500"
      >
        {t('dashboard.heatmap.transformReset')}
      </button>
    </div>
  );
}
