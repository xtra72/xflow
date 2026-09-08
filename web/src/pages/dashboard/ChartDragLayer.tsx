// 라인 차트의 **범례와 그림 상자를 끌어 옮기는** 레이어.
//
// 패널 드래그 레이어(`PanelDragLayer`)와 같은 구조다 — 잡은 대상으로 무엇이 움직일지 갈리고, 정해진 대상
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

import {
  PANEL_OFFSET_LIMIT,
  clampPercentOffset,
  pixelsToPercent,
} from './panels/charts/panelGeometry';
import {
  LEGEND_OFFSET_SAFETY_LIMIT,
  clampInlineLegendOffset,
} from './panels/charts/legendOverlay';
import { clampFontSize } from './panels/charts/statLayout';
import { snapOffsetToGrid } from './panels/charts/panelEditAlign';
import {
  clampGroupDelta,
  EMPTY_SELECTION,
  nextSelection,
  type PanelSelection,
} from './panels/charts/panelEditSelection';

/** 끌 수 있는 대상 하나. */
export interface ChartDragTarget {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
  /** 크기 손잡이가 바꾸는 값(px 또는 %). 손잡이를 쓰지 않는 대상은 생략한다. */
  size?: number;
  onResize?: (next: number) => void;
  /** 크기의 하한·상한. 미지정이면 글자 크기 범위(6~160)로 죈다. */
  sizeRange?: { min: number; max: number };
}



type Kind = 'legend' | 'plot';

/**
 * 대상별 오프셋 상한(백분율 포인트) — **읽는 쪽과 같은 값이어야 한다.**
 *
 * 그림 상자는 `readPanelOffset` 이 ±40 으로 읽고, 범례는 `clampStoredLegendOffset` 이
 * ±50 으로 읽는다. 쓰는 쪽이 더 느슨하면 끄는 동안에는 따라오다가 다음에 config 를 읽는
 * 순간 되돌아간다 — 특히 격자 스냅은 죄기 뒤에 값을 반 칸까지 도로 밀어내므로, 스냅에도
 * 같은 값을 넘겨야 한다.
 */
const OFFSET_LIMIT: Record<Kind, number> = {
  plot: PANEL_OFFSET_LIMIT,
  legend: LEGEND_OFFSET_SAFETY_LIMIT,
};

export function ChartDragLayer({
  enabled = true,
  snap = true,
  selection = EMPTY_SELECTION,
  onSelectionChange,
  legend,
  plot,
  children,
}: {
  /** 끌 수 있는가. 대시보드 패널은 배치 편집 모드일 때만 켠다. 미리보기는 언제나 켠다. */
  enabled?: boolean;
  /** 격자에 붙일지. 끄는 동안 Alt 를 누르면 이 값과 무관하게 잠시 꺼진다. */
  snap?: boolean;
  /** 고른 요소들. 끌면 이 전체가 함께 움직인다. */
  selection?: PanelSelection<Kind>;
  onSelectionChange?: (next: PanelSelection<Kind>) => void;
  legend: ChartDragTarget;
  plot: ChartDragTarget;
  children: React.ReactNode;
}): React.ReactElement {
  // 드래그 중에만 존재하는 상태. 시작 시점의 값을 잡아 두고 이후에는 그 스냅샷 + 이동량
  // 으로만 계산한다 — 매 프레임 DOM 을 다시 재면 리렌더와 얽혀 값이 떨린다.
  const dragRef = useRef<{
    kind: Kind;
    /** 자리를 옮기는가, 크기를 바꾸는가. */
    mode: 'move' | 'resize';
    startX: number;
    startY: number;
    base: { x: number; y: number };
    baseSize: number;
    container: { left: number; top: number; width: number; height: number };
    /** 범례 경로에서만 쓴다 — 남은 여백을 구하려면 지금 그려진 자리가 필요하다. */
    content: { left: number; top: number; width: number; height: number } | null;
    /** 격자 스냅이 중심을 구하는 데 쓰는, 잡은 요소의 상자. */
    elRect: { left: number; top: number; width: number; height: number };
    /** 함께 움직이는 요소들의 시작 오프셋. 혼자일 때도 길이 1 인 무리다. */
    group: Array<{ kind: Kind; offsetX: number; offsetY: number; limit: number }>;
  } | null>(null);

  const stateRef = useRef({ legend, plot });
  stateRef.current = { legend, plot };
  // 리스너는 마운트 시 한 번만 달므로 최신 값을 ref 로 안정화한다.
  const snapRef = useRef(snap);
  snapRef.current = snap;

  // 프레임당 한 번만 반영한다. 포인터 이벤트는 기기에 따라 프레임보다 자주 오는데,
  // 그때마다 config 를 고치면 화면에 보이지도 않을 렌더가 쌓여 오히려 끊긴다.
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<
    | { mode: 'move'; items: Array<{ kind: Kind; x: number; y: number }> }
    | { mode: 'resize'; kind: Kind; size: number }
    | null
  >(null);

  useEffect(() => {
    const flush = (): void => {
      frameRef.current = null;
      const next = pendingRef.current;
      pendingRef.current = null;
      if (!next) return;
      if (next.mode === 'resize') {
        stateRef.current[next.kind].onResize?.(next.size);
        return;
      }
      for (const item of next.items) {
        stateRef.current[item.kind].onChange({ x: item.x, y: item.y });
      }
    };
    const onMove = (e: PointerEvent): void => {
      const d = dragRef.current;
      if (!d) return;
      const dx = e.clientX - d.startX;
      const dy = e.clientY - d.startY;

      if (d.mode === 'resize') {
        // 대각선 투영 — 손잡이를 끈 거리만큼 커진다(통계와 같은 규칙).
        const t = stateRef.current[d.kind];
        const raw = d.baseSize + (dx + dy) / 2;
        const size = t.sizeRange
          ? Math.min(Math.max(raw, t.sizeRange.min), t.sizeRange.max)
          : (clampFontSize(raw) ?? d.baseSize);
        pendingRef.current = { mode: 'resize', kind: d.kind, size };
        if (frameRef.current === null) frameRef.current = requestAnimationFrame(flush);
        return;
      }

      const raw = {
        x: d.base.x + pixelsToPercent(dx, d.container.width),
        y: d.base.y + pixelsToPercent(dy, d.container.height),
      };
      // 죄기 규칙은 대상마다 다르다 — 범례는 지금 그려진 자리에서 남은 여백까지,
      // 그림 상자는 ±상한까지(머리말 참조).
      let next = d.content
        ? clampInlineLegendOffset(d.base, raw, d.container, d.content)
        : { x: clampPercentOffset(raw.x), y: clampPercentOffset(raw.y) };

      // 격자 스냅 — 두 대상 모두 백분율이라 같은 규칙을 쓴다. Alt 로 잠시 끈다.
      if (snapRef.current && !e.altKey) {
        next = {
          x: snapOffsetToGrid(
            next.x,
            { offset: d.base.x, rectStart: d.elRect.left, rectLen: d.elRect.width },
            { start: d.container.left, len: d.container.width },
            OFFSET_LIMIT[d.kind],
          ),
          y: snapOffsetToGrid(
            next.y,
            { offset: d.base.y, rectStart: d.elRect.top, rectLen: d.elRect.height },
            { start: d.container.top, len: d.container.height },
            OFFSET_LIMIT[d.kind],
          ),
        };
      }

      // 혼자 끌 때는 무리 죄기를 걸지 않는다 — 대상마다 죄기 규칙이 다른데(범례는
      // 남은 여백까지, 그림은 ±상한까지) 일률적인 상한을 덧씌우면 방금 계산한 규칙을
      // 덮어쓴다. 실제로 범례가 갈 수 있는 자리에서 미리 멈췄다.
      //
      // 여럿이면 상대 배치를 지키기 위해 무리 전체가 갈 수 있는 만큼으로 죈다.
      const capped =
        d.group.length > 1
          ? clampGroupDelta(d.group, next.x - d.base.x, next.y - d.base.y)
          : { dx: next.x - d.base.x, dy: next.y - d.base.y };
      pendingRef.current = {
        mode: 'move',
        items: d.group.map((m) => ({
          kind: m.kind,
          x: m.offsetX + capped.dx,
          y: m.offsetY + capped.dy,
        })),
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
    // 크기 손잡이를 가장 먼저 본다 — 손잡이는 대상 **안**에 있으므로 순서를 뒤집으면
    // 손잡이를 잡아도 대상이 통째로 움직인다.
    const resizeEl = target?.closest?.('[data-panel-resize]');
    // 범례를 먼저 본다 — 범례는 그림 상자 밖에 있지만, 순서를 못박아 두면 나중에 배치가
    // 바뀌어도 "범례를 잡았는데 그림이 움직이는" 일이 없다.
    const legendEl = resizeEl ? null : target?.closest?.('[data-chart-legend]');
    const plotEl = resizeEl || legendEl ? null : target?.closest?.('[data-chart-plot-area]');
    const kind: Kind | null = resizeEl
      ? (resizeEl.getAttribute('data-panel-resize') as Kind)
      : legendEl
        ? 'legend'
        : plotEl
          ? 'plot'
          : null;
    if (!kind) return;

    // Shift·Ctrl·Cmd 는 **고르기 전용** 조작이다 — 그 상태로 끌리면 무리에 넣으려다
    // 배치가 흐트러진다.
    const additive = e.shiftKey || e.ctrlKey || e.metaKey;
    const picked = nextSelection(selection, kind, additive);
    if (picked !== selection) onSelectionChange?.(picked);
    if (additive) return;

    // 죄기 기준은 **차트와 범례가 함께 들어 있는 본문**이다. 대상 자신을 쓰면 거의 움직이지
    // 못하고, 화면 전체를 쓰면 패널 밖으로 나간다. 부모를 그냥 쓰지 않는 이유는 배치마다
    // 부모가 다르기 때문이다 — 본문에 붙은 표식을 거슬러 올라가 찾는다.
    const grabbed = (resizeEl ?? legendEl ?? plotEl)!;
    const boundsEl = grabbed.closest('[data-chart-legend-bounds]') ?? grabbed.parentElement;
    const box = boundsEl?.getBoundingClientRect();
    if (!box || box.width <= 0 || box.height <= 0) return;

    // 범례에서는 잡은 요소(변위가 붙는 자리)와 **재는 요소**가 다르다. 잡은 요소는 flex
    // 형제라 교차축으로 늘어나 있어서, 그것을 재면 늘어난 축의 남은 여백이 0이 되어 그
    // 방향으로는 한 픽셀도 못 움직인다. 그림 상자는 영역을 꽉 채우므로 잴 것이 없다 —
    // ±상한으로 죈다.
    const contentEl = legendEl?.querySelector('[data-chart-legend-content]') ?? legendEl;
    const content = contentEl?.getBoundingClientRect();
    // 격자 스냅의 기준이 되는 잡은 요소의 상자.
    const elBox = grabbed.getBoundingClientRect();

    e.preventDefault();
    dragRef.current = {
      kind,
      mode: resizeEl ? 'resize' : 'move',
      startX: e.clientX,
      startY: e.clientY,
      base: { x: stateRef.current[kind].offsetX, y: stateRef.current[kind].offsetY },
      baseSize: stateRef.current[kind].size ?? 0,
      container: { left: box.left, top: box.top, width: box.width, height: box.height },
      elRect: { left: elBox.left, top: elBox.top, width: elBox.width, height: elBox.height },
      // 크기 조절은 잡은 대상 하나만 바꾼다 — 여러 대상에 같은 양을 더하는 것은 뜻이
      // 모호하다(글자 크기와 그림 백분율이 섞인다).
      // 상한은 대상마다 다르므로 무리에 한 값을 씌우지 않고 각자의 것을 들려 보낸다.
      group: resizeEl
        ? [
            {
              kind,
              offsetX: stateRef.current[kind].offsetX,
              offsetY: stateRef.current[kind].offsetY,
              limit: OFFSET_LIMIT[kind],
            },
          ]
        : [...picked].map((k) => ({
            kind: k,
            offsetX: stateRef.current[k].offsetX,
            offsetY: stateRef.current[k].offsetY,
            limit: OFFSET_LIMIT[k],
          })),
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
