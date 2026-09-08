// 미리보기 위에 얹는 패널 크기 조절 오버레이.
//
// 대시보드에서 패널 우하단을 끄는 것과 같은 조작을 설정 화면에서도 할 수 있게 한다.
// 그리드(칼럼 수·셀 크기·마진)는 **현재 대시보드 설정을 그대로** 따르므로, 여기서 맞춘
// 크기가 대시보드에서 그대로 재현된다.
//
// 가이드 라인은 이 오버레이가 아니라 패널 **뒤**의 PanelGridBackdrop 이 그린다 —
// 위에 그리면 패널의 실제 모양이 가려진다. 여기서는 패널 경계선과 조작만 담당한다.
//
// 프레임 자체는 포인터를 받지 않는다(pointer-events-none) — 히트맵 마커 배치처럼
// 미리보기 안에서 이뤄지는 드래그를 가리지 않기 위함이며, 우하단 손잡이만 예외다.

import { useCallback, useEffect, useRef, useState } from 'react';

import { cn } from '@/lib/utils/cn';
import { resizeGridSize, type GridSize } from './previewGridSize';

interface PanelResizeOverlayProps {
  /** 현재(편집 중) 그리드 크기 */
  size: GridSize;
/**
   * 미리보기 상자의 화면 크기(px) — 프레임이 미리보기와 정확히 겹치게 한다.
   * 아직 실측 전이면 null 이며, 이때는 종횡비 기반 CSS 로 그린다(첫 페인트에서
   * 프레임이 뒤늦게 나타나지 않도록).
   */
  box: { w: number; h: number } | null;
  /** 패널의 픽셀 종횡비 — box 미실측 시 CSS 폴백에 쓴다 */
  aspect: number;
  /** 대시보드 칼럼 수 */
  cols: number;
  /** 대시보드 셀 한 변(px). 0 이면 미측정 */
  cell: number;
  minW?: number;
  minH?: number;
  /** 크기가 바뀔 때마다 호출된다(드래그 중 연속 호출) */
  onChange: (next: GridSize) => void;
}

export function PanelResizeOverlay({
  size,
  box,
  aspect,
  cols,
  cell,
  minW,
  minH,
  onChange,
}: PanelResizeOverlayProps) {
  const [dragging, setDragging] = useState(false);
  const frameRef = useRef<HTMLDivElement | null>(null);

  // 드래그 시작 시점의 좌표와 크기 — 이동 중 갱신되는 size 를 기준으로 삼으면
  // 누적 오차가 생기므로 시작값을 고정해 둔다.
  const startRef = useRef<{ x: number; y: number; size: GridSize; boxW: number; boxH: number }>({
    x: 0,
    y: 0,
    size,
    boxW: 0,
    boxH: 0,
  });

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      // 배율 환산의 기준 크기. 실측값이 있으면 그것을, 없으면 프레임의 실제 렌더
      // 크기를 읽는다(CSS 종횡비로 그려진 경우).
      const rect = frameRef.current?.getBoundingClientRect();
      startRef.current = {
        x: e.clientX,
        y: e.clientY,
        size,
        boxW: box?.w ?? rect?.width ?? 0,
        boxH: box?.h ?? rect?.height ?? 0,
      };
      setDragging(true);
    },
    [size, box],
  );

  useEffect(() => {
    if (!dragging) return;

    const onMove = (e: MouseEvent) => {
      const start = startRef.current;
      onChange(
        resizeGridSize({
          start: start.size,
          dxPx: e.clientX - start.x,
          dyPx: e.clientY - start.y,
          boxW: start.boxW,
          boxH: start.boxH,
          cell,
          bounds: { cols, minW, minH },
        }),
      );
    };
    const onUp = () => setDragging(false);

    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  }, [dragging, cell, cols, minW, minH, onChange]);

  return (
    // z-10 이 필요하다. 미리보기 패널은 transform 으로 축소되는데, transform 은
    // 스택 컨텍스트를 만들어 z-index:auto 인 이 오버레이보다 **위에** 그려진다.
    // 패널 배경이 불투명하므로 z-index 없이는 프레임도 그리드도 가려 보이지 않는다.
    <div
      data-testid="panel-resize-overlay"
      className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center"
    >
      <div
        ref={frameRef}
        className={cn(
          'relative outline outline-1 outline-blue-400/70',
          // 실측 전 CSS 폴백에서만 영역 안으로 가둔다. 실측된 뒤에는 패널 경계와
          // 정확히 겹쳐야 하므로 가두지 않는다 — 채움 모드에서 넘치는 건 의도된 것이고,
          // 손잡이는 줌을 낮추거나 맞춤 모드로 바꿔 잡는다.
          box === null && 'max-h-full max-w-full',
        )}
        style={
          box !== null
            ? { width: `${box.w}px`, height: `${box.h}px` }
            : { aspectRatio: `${aspect} / 1`, width: '100%', height: '100%' }
        }
        data-testid="panel-resize-frame"
      >
        {/* 크기 배지 */}
        <span
          data-testid="panel-resize-size-badge"
          className={cn(
            'absolute left-0 top-0 bg-blue-500/80 px-1 py-px text-[9px] leading-tight text-white tabular-nums',
            dragging && 'bg-blue-600',
          )}
        >
          {size.w} × {size.h}
        </span>

        {/* 우하단 손잡이 — 대시보드의 resizeConfig handles: ['se'] 와 같은 자리다. */}
        <button
          type="button"
          data-testid="panel-resize-handle"
          aria-label="패널 크기 조절"
          onMouseDown={handleMouseDown}
          className={cn(
            'pointer-events-auto absolute -bottom-1 -right-1 h-4 w-4 cursor-se-resize rounded-sm',
            'border border-blue-500 bg-(--color-bg-elevated)',
            dragging ? 'bg-blue-500' : 'hover:bg-blue-500/30',
          )}
        />
      </div>
    </div>
  );
}
