// 설정 미리보기에서 **파이와 범례를 끌어 옮기는** 레이어.
//
// 잡은 대상으로 무엇을 옮길지 갈린다 — 범례(`data-pie-legend`)를 잡으면 범례가,
// 차트 영역(`data-pie-chart-area`)을 잡으면 파이 중심이 움직인다. 그 밖을 잡으면
// 아무 일도 없다: 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있어서,
// 아무 데나 잡아도 끌리면 그것들과 부딪힌다(게이지 값 드래그와 같은 규칙).
//
// 두 대상을 **한 레이어**가 맡는 이유는 겹침 때문이다. 범례는 차트 영역 밖에 있지만
// 레이어를 두 겹으로 쌓으면 이벤트가 위로 타고 올라가 둘이 함께 움직인다.
//
// 대시보드에 놓인 패널은 이 레이어로 감싸지 않으므로 오프셋만큼 밀린 자리에 그려질 뿐
// 끌리지 않는다.

import { useEffect, useRef } from 'react';

import { clampPercentOffset, pixelsToPercent } from './panels/charts/panelGeometry';
import { clampLegendOffsets, type LegendAnchor } from './panels/charts/legendOverlay';

/** 끌 수 있는 대상 하나. */
export interface PieDragTarget {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 미리보기가 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
}

type Kind = 'legend' | 'chart';

export function PieDragLayer({
  enabled = true,
  legend,
  chart,
  children,
}: {
  /**
   * 끌 수 있는가. 대시보드 패널은 **배치 편집 모드일 때만** 켠다 — 늘 켜 두면 패널을
   * 옮기거나 크기를 바꾸려는 조작과 부딪힌다. 미리보기는 언제나 켠다.
   */
  enabled?: boolean;
  legend: PieDragTarget;
  chart: PieDragTarget;
  children: React.ReactNode;
}): React.ReactElement {
  // 드래그 중에만 존재하는 상태. 시작 시점의 오프셋과 포인터 위치를 잡아 두고 이후에는
  // 그 스냅샷 + 이동량으로만 계산한다 — 매 프레임 DOM 을 다시 재면 리렌더와 얽혀 값이
  // 떨린다(게이지 값 드래그와 같은 방식).
  const dragRef = useRef<{
    kind: Kind;
    startX: number;
    startY: number;
    baseX: number;
    baseY: number;
    span: { w: number; h: number };
    /** 범례 경로에서만 쓴다 — 가장자리까지 끌 수 있는 범위를 구하려면 제 크기가 필요하다. */
    legendBox: { anchor: LegendAnchor; width: number; height: number } | null;
  } | null>(null);

  const stateRef = useRef({ legend, chart });
  stateRef.current = { legend, chart };

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
      pendingRef.current =
        d.kind === 'legend'
          ? (() => {
              // 범례도 백분율이다 — 픽셀로 두면 미리보기와 실제 패널의 폭이 달라 같은
              // 값이 다른 자리를 가리키고, 좁은 패널에서는 밖으로 나가 보이지 않는다.
              // 죄기는 **제 모서리가 상자 가장자리에 닿는 곳**까지 — 고정 상한으로 죄면
              // 여백이 남았는데도 멈춘다.
              const next = clampLegendOffsets(
                d.legendBox!.anchor,
                d.baseX + pixelsToPercent(dx, d.span.w),
                d.baseY + pixelsToPercent(dy, d.span.h),
                { width: d.span.w, height: d.span.h },
                d.legendBox!,
              );
              return { kind: 'legend' as const, x: next.x, y: next.y };
            })()
          : {
              // 파이 중심도 같은 규칙이다(recharts 가 `cx`/`cy` 를 백분율로 받는다).
              kind: 'chart',
              x: clampPercentOffset(d.baseX + pixelsToPercent(dx, d.span.w)),
              y: clampPercentOffset(d.baseY + pixelsToPercent(dy, d.span.h)),
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
    if (!enabled) return;
    const target = e.target as Element | null;
    // 범례를 먼저 본다 — 범례가 차트 영역 안에 놓이는 배치는 없지만, 순서를 못박아
    // 두면 나중에 배치가 바뀌어도 "범례를 잡았는데 파이가 움직이는" 일이 없다.
    const legendEl = target?.closest?.('[data-pie-legend]');
    const chartEl = legendEl ? null : target?.closest?.('[data-pie-chart-area]');
    const kind: Kind | null = legendEl ? 'legend' : chartEl ? 'chart' : null;
    if (!kind) return;

    // 죄기 기준은 두 경우 모두 **패널 본문**이다. 대상 자신의 크기를 쓰면 작은 범례가
    // 거의 움직이지 못하고, 화면 전체를 쓰면 패널 밖으로 나간다. 차트 영역은 자기
    // 자신이 기준 변이므로 그대로 쓴다.
    const rect = (kind === 'legend' ? legendEl!.parentElement : chartEl!)?.getBoundingClientRect();
    if (!rect || rect.width <= 0 || rect.height <= 0) return;
    e.preventDefault();
    const base = stateRef.current[kind];
    // 붙인 변은 범례가 스스로 알고 있다(`data-position`) — 설정을 다시 읽지 않는다.
    const legendRect = kind === 'legend' ? legendEl!.getBoundingClientRect() : null;
    dragRef.current = {
      kind,
      startX: e.clientX,
      startY: e.clientY,
      baseX: base.offsetX,
      baseY: base.offsetY,
      span: { w: rect.width, h: rect.height },
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
      className={
        enabled
          ? 'contents [&_[data-pie-legend]]:cursor-move [&_[data-pie-chart-area]]:cursor-move'
          : 'contents'
      }
      data-testid="pie-drag-layer"
      onPointerDown={onPointerDown}
    >
      {children}
    </div>
  );
}
