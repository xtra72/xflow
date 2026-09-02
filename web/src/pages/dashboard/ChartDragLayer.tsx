// 라인 차트의 **범례와 그림 상자를 끌어 옮기는** 레이어.
//
// 파이(`PieDragLayer`)와 같은 구조다 — 잡은 대상으로 무엇이 움직일지 갈리고, 정해진 대상
// 밖을 잡으면 아무 일도 없다(패널 크기 조절·휠 확대 같은 다른 조작과 부딪히지 않기 위해).
// 두 대상을 **한 레이어**가 맡는 이유도 같다: 레이어를 두 겹으로 쌓으면 이벤트가 위로
// 타고 올라가 둘이 함께 움직인다.
//
// 죄기 규칙이 대상마다 다르다.
//   - 그림 상자: 파이 중심과 같은 ±상한(`clampPercentOffset`). 상자가 영역을 꽉 채우므로
//     "가장자리에 닿는 곳" 이라는 기준이 성립하지 않는다.
//   - 범례: 흐름 안에 남아 상대 변위만 얹으므로 "지금 그려진 자리에서 상자 밖으로 나가지
//     않는 만큼" 이 한계다(`clampInlineLegendOffset`). 파이 범례처럼 어느 변에 붙어 뜨는
//     것이 아니라 변별 규칙(`clampLegendOffsets`)은 쓰지 않는다.
//
// 대시보드에 놓인 패널은 배치 편집 모드일 때만 이 레이어가 켜지고, 꺼져 있으면 저장된
// 변위만큼 밀린 자리에 그려질 뿐 끌리지 않는다.

import { useEffect, useRef } from 'react';

import { clampPercentOffset, pixelsToPercent } from './panels/charts/panelGeometry';
import { clampInlineLegendOffset } from './panels/charts/legendOverlay';

/** 끌 수 있는 대상 하나. */
export interface ChartDragTarget {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
}

type Kind = 'legend' | 'plot';

export function ChartDragLayer({
  enabled = true,
  legend,
  plot,
  children,
}: {
  /** 끌 수 있는가. 대시보드 패널은 배치 편집 모드일 때만 켠다. 미리보기는 언제나 켠다. */
  enabled?: boolean;
  legend: ChartDragTarget;
  plot: ChartDragTarget;
  children: React.ReactNode;
}): React.ReactElement {
  // 드래그 중에만 존재하는 상태. 시작 시점의 값을 잡아 두고 이후에는 그 스냅샷 + 이동량
  // 으로만 계산한다 — 매 프레임 DOM 을 다시 재면 리렌더와 얽혀 값이 떨린다.
  const dragRef = useRef<{
    kind: Kind;
    startX: number;
    startY: number;
    base: { x: number; y: number };
    container: { left: number; top: number; width: number; height: number };
    /** 범례 경로에서만 쓴다 — 남은 여백을 구하려면 지금 그려진 자리가 필요하다. */
    content: { left: number; top: number; width: number; height: number } | null;
  } | null>(null);

  const stateRef = useRef({ legend, plot });
  stateRef.current = { legend, plot };

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
      const raw = {
        x: d.base.x + pixelsToPercent(e.clientX - d.startX, d.container.width),
        y: d.base.y + pixelsToPercent(e.clientY - d.startY, d.container.height),
      };
      const next = d.content
        ? clampInlineLegendOffset(d.base, raw, d.container, d.content)
        : { x: clampPercentOffset(raw.x), y: clampPercentOffset(raw.y) };
      pendingRef.current = { kind: d.kind, ...next };
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
    // 범례를 먼저 본다 — 범례는 그림 상자 밖에 있지만, 순서를 못박아 두면 나중에 배치가
    // 바뀌어도 "범례를 잡았는데 그림이 움직이는" 일이 없다.
    const legendEl = target?.closest?.('[data-chart-legend]');
    const plotEl = legendEl ? null : target?.closest?.('[data-chart-plot-area]');
    const kind: Kind | null = legendEl ? 'legend' : plotEl ? 'plot' : null;
    if (!kind) return;

    // 죄기 기준은 **차트와 범례가 함께 들어 있는 본문**이다. 대상 자신을 쓰면 거의 움직이지
    // 못하고, 화면 전체를 쓰면 패널 밖으로 나간다. 부모를 그냥 쓰지 않는 이유는 배치마다
    // 부모가 다르기 때문이다 — 본문에 붙은 표식을 거슬러 올라가 찾는다.
    const grabbed = (legendEl ?? plotEl)!;
    const boundsEl = grabbed.closest('[data-chart-legend-bounds]') ?? grabbed.parentElement;
    const box = boundsEl?.getBoundingClientRect();
    if (!box || box.width <= 0 || box.height <= 0) return;

    // 범례에서는 잡은 요소(변위가 붙는 자리)와 **재는 요소**가 다르다. 잡은 요소는 flex
    // 형제라 교차축으로 늘어나 있어서, 그것을 재면 늘어난 축의 남은 여백이 0이 되어 그
    // 방향으로는 한 픽셀도 못 움직인다. 그림 상자는 영역을 꽉 채우므로 잴 것이 없다 —
    // ±상한으로 죈다.
    const contentEl = legendEl?.querySelector('[data-chart-legend-content]') ?? legendEl;
    const content = contentEl?.getBoundingClientRect();

    e.preventDefault();
    dragRef.current = {
      kind,
      startX: e.clientX,
      startY: e.clientY,
      base: { x: stateRef.current[kind].offsetX, y: stateRef.current[kind].offsetY },
      container: { left: box.left, top: box.top, width: box.width, height: box.height },
      content: content
        ? {
            left: content.left,
            top: content.top,
            width: content.width,
            height: content.height,
          }
        : null,
    };
  };

  return (
    <div
      className={
        enabled
          ? 'contents [&_[data-chart-legend]]:cursor-move [&_[data-chart-plot-area]]:cursor-move'
          : 'contents'
      }
      data-testid="chart-drag-layer"
      onPointerDown={onPointerDown}
    >
      {children}
    </div>
  );
}
