// 설정 미리보기에서 **현재값과 게이지를 끌어 옮기는** 레이어.
//
// 잡은 대상으로 무엇을 옮길지 갈린다 — 값 글자(`data-gauge-value-text`)를 잡으면 값이,
// 임계값 범례(`data-gauge-threshold-legend`)를 잡으면 범례가, 게이지 상자
// (`data-gauge-body`)를 잡으면 게이지 전체가 움직인다. 그 밖을 잡으면 아무 일도 없다:
// 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있어서, 아무 데나 잡아도
// 끌리면 그것들과 부딪힌다. 파이 드래그 레이어와 같은 규칙이다.
//
// **좌표계가 둘로 갈린다.** 값 글자는 SVG `viewBox` 좌표라 픽셀을 그 좌표로 환산해야
// 하고, 범례와 게이지 상자는 담는 상자 대비 **백분율**을 쓴다 — 픽셀로 두면 미리보기와
// 실제 패널의 크기가 달라 같은 값이 다른 자리를 가리킨다.
//
// 대시보드에 놓인 패널은 이 레이어로 감싸지 않으므로 표시만 남고 끌리지 않는다.

import { useEffect, useRef } from 'react';

import { clampOffset, parseViewBox, pixelsToViewBox } from './panels/gauge/valueDrag';
import { clampPercentOffset, pixelsToPercent } from './panels/charts/panelGeometry';
import { clampLegendOffsets, type LegendAnchor } from './panels/charts/legendOverlay';

/** 끌 수 있는 대상 하나. */
export interface GaugeDragTarget {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 미리보기가 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
}

type Kind = 'value' | 'legend' | 'body';

export function GaugeDragLayer({
  enabled = true,
  value,
  legend,
  body,
  children,
}: {
  /**
   * 끌 수 있는가. 대시보드 패널은 **배치 편집 모드일 때만** 켠다 — 늘 켜 두면 패널을
   * 옮기거나 크기를 바꾸려는 조작과 부딪힌다. 미리보기는 언제나 켠다.
   */
  enabled?: boolean;
  value: GaugeDragTarget;
  legend: GaugeDragTarget;
  body: GaugeDragTarget;
  children: React.ReactNode;
}): React.ReactElement {
  const hostRef = useRef<HTMLDivElement>(null);
  // 드래그 중에만 존재하는 상태. 시작 시점의 오프셋과 포인터 위치를 잡아 두고,
  // 이후에는 그 스냅샷 + 이동량으로만 계산한다 — 매 프레임 DOM 을 다시 재면
  // 리렌더와 얽혀 값이 떨린다(표 열 리사이즈와 같은 방식).
  const dragRef = useRef<{
    kind: Kind;
    startX: number;
    startY: number;
    baseX: number;
    baseY: number;
    rect: { width: number; height: number };
    /** 값 글자 경로에서만 쓴다(게이지 상자는 백분율이라 viewBox 가 필요 없다). */
    viewBox: { w: number; h: number } | null;
    /** 범례 경로에서만 쓴다 — 가장자리까지 끌 수 있는 범위를 구하려면 제 크기가 필요하다. */
    legendBox: { anchor: LegendAnchor; width: number; height: number } | null;
  } | null>(null);

  // 최신 콜백·오프셋을 ref 로 안정화한다. document 리스너를 매 렌더 다시 달지 않는다.
  const stateRef = useRef({ value, legend, body });
  stateRef.current = { value, legend, body };

  // 프레임당 한 번만 반영한다. 포인터 이벤트는 기기에 따라 프레임보다 자주 오는데,
  // 그때마다 config 를 고치면 화면에 보이지도 않을 렌더가 쌓여 오히려 끊긴다.
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<{ kind: Kind; x: number; y: number } | null>(null);

  useEffect(() => {
    const flush = (): void => {
      frameRef.current = null;
      const next = pendingRef.current;
      pendingRef.current = null;
      if (next) stateRef.current[next.kind].onChange({ x: next.x, y: next.y });
    };
    const onMove = (e: PointerEvent): void => {
      const d = dragRef.current;
      if (!d) return;
      const dx = e.clientX - d.startX;
      const dy = e.clientY - d.startY;
      if (d.kind === 'value' && d.viewBox) {
        const moved = pixelsToViewBox(dx, dy, d.rect, d.viewBox);
        pendingRef.current = {
          kind: 'value',
          x: clampOffset(d.baseX + moved.dx, d.viewBox.w),
          y: clampOffset(d.baseY + moved.dy, d.viewBox.h),
        };
      } else if (d.kind === 'legend' && d.legendBox) {
        // 범례도 백분율이다 — 픽셀로 두면 미리보기와 실제 패널의 크기가 달라 같은 값이
        // 다른 자리를 가리키고, 좁은 패널에서는 밖으로 나가 보이지 않는다. 죄기는 제
        // 모서리가 상자 가장자리에 닿는 곳까지 — 고정 상한은 여백을 남기고 멈춘다.
        const next = clampLegendOffsets(
          d.legendBox.anchor,
          d.baseX + pixelsToPercent(dx, d.rect.width),
          d.baseY + pixelsToPercent(dy, d.rect.height),
          d.rect,
          d.legendBox,
        );
        pendingRef.current = { kind: 'legend', x: next.x, y: next.y };
      } else {
        pendingRef.current = {
          kind: 'body',
          x: clampPercentOffset(d.baseX + pixelsToPercent(dx, d.rect.width)),
          y: clampPercentOffset(d.baseY + pixelsToPercent(dy, d.rect.height)),
        };
      }
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
    if (!enabled) return;
    const target = e.target as Element | null;
    // 안쪽부터 본다 — 값 글자는 게이지 상자 **안**에 있으므로 순서를 뒤집으면 글자를
    // 잡아도 게이지 전체가 움직인다. 범례는 상자 밖이지만 순서를 못박아 두면 나중에
    // 배치가 바뀌어도 "범례를 잡았는데 게이지가 움직이는" 일이 없다.
    const valueEl = target?.closest?.('[data-gauge-value-text]');
    const legendEl = valueEl ? null : target?.closest?.('[data-gauge-threshold-legend]');
    const bodyEl = valueEl || legendEl ? null : target?.closest?.('[data-gauge-body]');
    const kind: Kind | null = valueEl ? 'value' : legendEl ? 'legend' : bodyEl ? 'body' : null;
    if (!kind) return;

    // 값 글자는 SVG 안의 좌표라 그 SVG 를, 범례는 담긴 상자(게이지 영역)를, 게이지
    // 상자는 자기 자신을 기준 변으로 쓴다. 범례 자신을 기준으로 쓰면 작은 범례가 거의
    // 움직이지 못한다.
    const box =
      kind === 'value'
        ? valueEl!.closest('svg')
        : kind === 'legend'
          ? legendEl!.parentElement
          : bodyEl!;
    const viewBox =
      kind === 'value' ? parseViewBox(box?.getAttribute('viewBox')) : null;
    const rect = box?.getBoundingClientRect();
    const legendRect = kind === 'legend' ? legendEl!.getBoundingClientRect() : null;
    // 크기를 잴 수 없으면(레이아웃 전) 시작하지 않는다 — 0 으로 나눈 이동량이
    // 그림을 화면 밖으로 날린다.
    if (!rect || rect.width <= 0 || rect.height <= 0) return;
    if (kind === 'value' && !viewBox) return;
    e.preventDefault();
    const base = stateRef.current[kind];
    dragRef.current = {
      kind,
      startX: e.clientX,
      startY: e.clientY,
      baseX: base.offsetX,
      baseY: base.offsetY,
      rect: { width: rect.width, height: rect.height },
      viewBox,
      // 붙인 변은 범례가 스스로 알고 있다(`data-position`) — 설정을 다시 읽지 않는다.
      legendBox: legendRect
        ? {
            anchor: (legendEl!.getAttribute('data-position') as LegendAnchor | null) ?? 'bottom',
            width: legendRect.width,
            height: legendRect.height,
          }
        : null,
    };
  };

  return (
    <div
      ref={hostRef}
      className={
        enabled
          ? 'contents [&_[data-gauge-value-text]]:cursor-move [&_[data-gauge-threshold-legend]]:cursor-move [&_[data-gauge-body]]:cursor-move'
          : 'contents'
      }
      data-testid="gauge-drag-layer"
      onPointerDown={onPointerDown}
    >
      {children}
    </div>
  );
}
