// 설정 미리보기에서 **현재값을 끌어 옮기는** 레이어.
//
// 값 글자(`data-gauge-value-text`)를 잡았을 때만 드래그가 시작된다. 미리보기에는 패널
// 크기 조절·휠 확대 같은 다른 조작이 이미 있어서, 아무 데나 잡아도 끌리면 그것들과
// 부딪힌다. 글자 위에서만 시작하면 두 조작이 자리로 갈린다.
//
// 대시보드에 놓인 패널은 이 레이어로 감싸지 않으므로 값 글자에 표시만 남고 끌리지 않는다.

import { useEffect, useRef } from 'react';

import { clampOffset, parseViewBox, pixelsToViewBox } from './panels/gauge/valueDrag';

export function GaugeValueDragLayer({
  offsetX,
  offsetY,
  onChange,
  children,
}: {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 미리보기가 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
  children: React.ReactNode;
}): React.ReactElement {
  const hostRef = useRef<HTMLDivElement>(null);
  // 드래그 중에만 존재하는 상태. 시작 시점의 오프셋과 포인터 위치를 잡아 두고,
  // 이후에는 그 스냅샷 + 이동량으로만 계산한다 — 매 프레임 DOM 을 다시 재면
  // 리렌더와 얽혀 값이 떨린다(표 열 리사이즈와 같은 방식).
  const dragRef = useRef<{
    startX: number;
    startY: number;
    baseX: number;
    baseY: number;
    rect: { width: number; height: number };
    viewBox: { w: number; h: number };
  } | null>(null);

  // 최신 콜백·오프셋을 ref 로 안정화한다. document 리스너를 매 렌더 다시 달지 않는다.
  const stateRef = useRef({ offsetX, offsetY, onChange });
  stateRef.current = { offsetX, offsetY, onChange };

  // 프레임당 한 번만 반영한다. 포인터 이벤트는 기기에 따라 프레임보다 자주 오는데,
  // 그때마다 config 를 고치면 화면에 보이지도 않을 렌더가 쌓여 오히려 끊긴다.
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<{ x: number; y: number } | null>(null);

  useEffect(() => {
    const flush = (): void => {
      frameRef.current = null;
      const next = pendingRef.current;
      pendingRef.current = null;
      if (next) stateRef.current.onChange(next);
    };
    const onMove = (e: PointerEvent): void => {
      const d = dragRef.current;
      if (!d) return;
      const moved = pixelsToViewBox(e.clientX - d.startX, e.clientY - d.startY, d.rect, d.viewBox);
      pendingRef.current = {
        x: clampOffset(d.baseX + moved.dx, d.viewBox.w),
        y: clampOffset(d.baseY + moved.dy, d.viewBox.h),
      };
      if (frameRef.current === null) frameRef.current = requestAnimationFrame(flush);
    };
    const onUp = (): void => {
      dragRef.current = null;
      // 마지막 위치를 흘리지 않는다 — 프레임 대기 중에 손을 떼면 그 이동이 사라진다.
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current);
        flush();
      }
    };
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
    return () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    };
  }, []);

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>): void => {
    const target = e.target as Element | null;
    if (!target?.closest?.('[data-gauge-value-text]')) return;
    const svg = target.closest('svg');
    const viewBox = parseViewBox(svg?.getAttribute('viewBox'));
    const rect = svg?.getBoundingClientRect();
    // 크기를 잴 수 없으면(레이아웃 전) 시작하지 않는다 — 0 으로 나눈 이동량이
    // 값을 화면 밖으로 날린다.
    if (!viewBox || !rect || rect.width <= 0 || rect.height <= 0) return;
    e.preventDefault();
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      baseX: stateRef.current.offsetX,
      baseY: stateRef.current.offsetY,
      rect: { width: rect.width, height: rect.height },
      viewBox,
    };
  };

  return (
    <div
      ref={hostRef}
      className="contents [&_[data-gauge-value-text]]:cursor-move"
      data-testid="gauge-value-drag-layer"
      onPointerDown={onPointerDown}
    >
      {children}
    </div>
  );
}
