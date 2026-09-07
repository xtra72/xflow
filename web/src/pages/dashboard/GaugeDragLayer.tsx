// 설정 미리보기에서 **현재값과 게이지를 끌어 옮기는** 레이어.
//
// 잡은 대상으로 무엇을 옮길지 갈린다 — 값 글자(`data-gauge-value-text`)를 잡으면 값이,
// 임계값 범례(`data-gauge-threshold-legend`)를 잡으면 범례가, 게이지 상자
// (`data-gauge-body`)를 잡으면 게이지 전체가 움직인다. 그 밖을 잡으면 아무 일도 없다:
// 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있어서, 아무 데나 잡아도
// 끌리면 그것들과 부딪힌다. 파이 드래그 레이어와 같은 규칙이다.
//
// 세 대상이 **같은 좌표계**를 쓴다 — 담는 상자 대비 백분율이다. 픽셀로 두면 미리보기와
// 실제 패널의 크기가 달라 같은 값이 다른 자리를 가리킨다.
//
// 값 글자는 한때 SVG `viewBox` 좌표였다. 도형과 같은 SVG 안에 있었기 때문인데, 그래서
// 도형을 줄이면 값이 따라 움직이고 캔버스 밖으로도 못 나갔다(보고된 결함). 값이 도형
// 밖 오버레이가 되면서 좌표계가 하나로 합쳐졌다.
//
// 대시보드에 놓인 패널은 이 레이어로 감싸지 않으므로 표시만 남고 끌리지 않는다.

import { useEffect, useRef } from 'react';

import { clampPercentOffset, pixelsToPercent } from './panels/charts/panelGeometry';
import { clampLegendOffsets, type LegendAnchor } from './panels/charts/legendOverlay';
import { clampFontSize, STAT_OFFSET_LIMIT } from './panels/charts/statLayout';
import { snapOffsetToGrid } from './panels/charts/panelEditAlign';
import {
  clampGroupDelta,
  EMPTY_SELECTION,
  nextSelection,
  type StatSelection,
} from './panels/charts/panelEditSelection';

/** 끌 수 있는 대상 하나. */
export interface GaugeDragTarget {
  offsetX: number;
  offsetY: number;
  /** 끄는 동안 계속 호출된다 — 미리보기가 즉시 따라와야 어디에 놓일지 보인다. */
  onChange: (next: { x: number; y: number }) => void;
  /** 크기 손잡이가 바꾸는 값(px 또는 %). 손잡이를 쓰지 않는 대상은 생략한다. */
  size?: number;
  onResize?: (next: number) => void;
  /** 크기의 하한·상한. 미지정이면 글자 크기 범위(6~160)로 죈다. */
  sizeRange?: { min: number; max: number };
  /**
   * 포인터 1px 이 크기 몇을 움직이는가(기본 1).
   *
   * px 단위 값(글자 크기)이나 백분율은 1:1 이 자연스럽지만, 값 글자의 크기는 **배율**
   * (0.3~3)이라 1:1 로 두면 조금만 끌어도 상한에 닿아 조절이 안 된다.
   */
  sizeStep?: number;
  /**
   * 세로 크기 — **주면 손잡이가 축을 나눈다**(가로 이동은 `size`, 세로 이동은 `sizeY`).
   *
   * 세로바 게이지처럼 사각형인 도형에만 준다. 원형·반원처럼 비율이 뜻을 갖는 도형은
   * 생략하고, 그러면 종전대로 대각선 투영으로 한 값을 키운다.
   */
  sizeY?: number;
  onResizeY?: (next: number) => void;
  sizeYRange?: { min: number; max: number };
}

type Kind = 'value' | 'legend' | 'body';

/**
 * 크기를 범위 안으로 죈다. 범위를 주지 않으면 글자 크기(6~160)로 본다 — 손잡이를 쓰는
 * 대상 중 범위를 밝히지 않는 것은 범례(글자 덩어리)뿐이다.
 */
function clampSize(
  raw: number,
  range: { min: number; max: number } | undefined,
  fallback: number,
): number {
  if (range) return Math.min(Math.max(raw, range.min), range.max);
  return clampFontSize(raw) ?? fallback;
}

export function GaugeDragLayer({
  enabled = true,
  snap = true,
  selection = EMPTY_SELECTION,
  onSelectionChange,
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
  /** 격자에 붙일지. 끄는 동안 Alt 를 누르면 이 값과 무관하게 잠시 꺼진다. */
  snap?: boolean;
  /** 고른 요소들. 끌면 이 전체가 함께 움직인다(값 글자는 빠진다). */
  selection?: StatSelection<Kind>;
  onSelectionChange?: (next: StatSelection<Kind>) => void;
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
    /** 자리를 옮기는가, 크기를 바꾸는가. */
    mode: 'move' | 'resize';
    startX: number;
    startY: number;
    baseX: number;
    baseY: number;
    baseSize: number;
    baseSizeY: number;
    rect: { left: number; top: number; width: number; height: number };
    /** 격자 스냅이 중심을 구하는 데 쓰는, 잡은 요소의 상자. */
    elRect: { left: number; top: number; width: number; height: number };
    /** 함께 움직이는 대상들의 시작 오프셋. 혼자일 때도 길이 1 인 무리다. */
    group: Array<{ kind: Kind; offsetX: number; offsetY: number }>;
    /** 범례 경로에서만 쓴다 — 가장자리까지 끌 수 있는 범위를 구하려면 제 크기가 필요하다. */
    legendBox: { anchor: LegendAnchor; width: number; height: number } | null;
  } | null>(null);

  // 최신 콜백·오프셋을 ref 로 안정화한다. document 리스너를 매 렌더 다시 달지 않는다.
  const stateRef = useRef({ value, legend, body });
  stateRef.current = { value, legend, body };
  // 리스너는 마운트 시 한 번만 달므로 최신 값을 ref 로 안정화한다.
  const snapRef = useRef(snap);
  snapRef.current = snap;

  // 프레임당 한 번만 반영한다. 포인터 이벤트는 기기에 따라 프레임보다 자주 오는데,
  // 그때마다 config 를 고치면 화면에 보이지도 않을 렌더가 쌓여 오히려 끊긴다.
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<
    | { mode: 'move'; items: Array<{ kind: Kind; x: number; y: number }> }
    | { mode: 'resize'; kind: Kind; size: number; sizeY?: number }
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
        if (next.sizeY !== undefined) stateRef.current[next.kind].onResizeY?.(next.sizeY);
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
        const t = stateRef.current[d.kind];
        const step = t.sizeStep ?? 1;
        if (t.sizeY !== undefined) {
          // 축을 나눈다 — 가로로 끌면 폭, 세로로 끌면 높이. 사각형 도형에서는 대각선
          // 투영이 뜻을 잃는다("가늘고 길게" 를 만들 수 없다).
          pendingRef.current = {
            mode: 'resize',
            kind: d.kind,
            size: clampSize(d.baseSize + dx * step, t.sizeRange, d.baseSize),
            sizeY: clampSize(d.baseSizeY + dy * step, t.sizeYRange, d.baseSizeY),
          };
        } else {
          // 대각선 투영 — 손잡이를 끈 거리만큼 커진다(통계·라인과 같은 규칙).
          pendingRef.current = {
            mode: 'resize',
            kind: d.kind,
            size: clampSize(d.baseSize + ((dx + dy) / 2) * step, t.sizeRange, d.baseSize),
          };
        }
        if (frameRef.current === null) frameRef.current = requestAnimationFrame(flush);
        return;
      }


      let next: { x: number; y: number };
      if (d.kind === 'legend' && d.legendBox) {
        // 범례도 백분율이다 — 픽셀로 두면 미리보기와 실제 패널의 크기가 달라 같은 값이
        // 다른 자리를 가리키고, 좁은 패널에서는 밖으로 나가 보이지 않는다. 죄기는 제
        // 모서리가 상자 가장자리에 닿는 곳까지 — 고정 상한은 여백을 남기고 멈춘다.
        next = clampLegendOffsets(
          d.legendBox.anchor,
          d.baseX + pixelsToPercent(dx, d.rect.width),
          d.baseY + pixelsToPercent(dy, d.rect.height),
          d.rect,
          d.legendBox,
        );
      } else {
        // 값 글자는 ±50% 까지 — 제 중심이 패널 어느 모서리에든 닿아야 한다(도형 영역에
        // 갇히지 않는 것이 이 요소의 요구다). 게이지 상자는 종전 ±40% 그대로다:
        // 영역을 꽉 채우는 그림이라 더 밀면 되돌릴 손잡이가 화면 밖으로 나간다.
        const limit = d.kind === 'value' ? STAT_OFFSET_LIMIT : undefined;
        next = {
          x: clampPercentOffset(d.baseX + pixelsToPercent(dx, d.rect.width), limit),
          y: clampPercentOffset(d.baseY + pixelsToPercent(dy, d.rect.height), limit),
        };
      }

      // 격자 스냅 — 백분율 대상에만 건다. Alt 로 잠시 끈다.
      if (snapRef.current && !e.altKey) {
        next = {
          x: snapOffsetToGrid(
            next.x,
            { offset: d.baseX, rectStart: d.elRect.left, rectLen: d.elRect.width },
            { start: d.rect.left, len: d.rect.width },
          ),
          y: snapOffsetToGrid(
            next.y,
            { offset: d.baseY, rectStart: d.elRect.top, rectLen: d.elRect.height },
            { start: d.rect.top, len: d.rect.height },
          ),
        };
      }

      // 혼자 끌 때는 무리 죄기를 걸지 않는다 — 대상마다 죄기 규칙이 다른데(범례는 제
      // 모서리까지, 게이지는 ±상한까지) 일률적인 상한을 덧씌우면 방금 계산한 규칙을
      // 덮어쓴다. 여럿이면 상대 배치를 지키기 위해 무리 전체가 갈 수 있는 만큼으로 죈다.
      const capped =
        d.group.length > 1
          ? clampGroupDelta(d.group, next.x - d.baseX, next.y - d.baseY)
          : { dx: next.x - d.baseX, dy: next.y - d.baseY };
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
    // 안쪽부터 본다 — 값 글자는 게이지 상자 **안**에 있으므로 순서를 뒤집으면 글자를
    // 잡아도 게이지 전체가 움직인다. 범례는 상자 밖이지만 순서를 못박아 두면 나중에
    // 배치가 바뀌어도 "범례를 잡았는데 게이지가 움직이는" 일이 없다.
    // 크기 손잡이를 가장 먼저 본다 — 손잡이는 대상 **안**에 있으므로 순서를 뒤집으면
    // 손잡이를 잡아도 대상이 통째로 움직인다.
    const resizeEl = target?.closest?.('[data-panel-resize]');
    const valueEl = resizeEl ? null : target?.closest?.('[data-gauge-value-text]');
    const legendEl =
      resizeEl || valueEl ? null : target?.closest?.('[data-gauge-threshold-legend]');
    const bodyEl =
      resizeEl || valueEl || legendEl ? null : target?.closest?.('[data-gauge-body]');
    const kind: Kind | null = resizeEl
      ? (resizeEl.getAttribute('data-panel-resize') as Kind)
      : valueEl
        ? 'value'
        : legendEl
          ? 'legend'
          : bodyEl
            ? 'body'
            : null;
    if (!kind) return;

    // Shift·Ctrl·Cmd 는 **고르기 전용** 조작이다 — 그 상태로 끌리면 무리에 넣으려다
    // 배치가 흐트러진다.
    const additive = e.shiftKey || e.ctrlKey || e.metaKey;
    const picked = nextSelection(selection, kind, additive);
    if (picked !== selection) onSelectionChange?.(picked);
    if (additive) return;

    // 값 글자는 SVG 안의 좌표라 그 SVG 를, 범례는 담긴 상자(게이지 영역)를, 게이지
    // 상자는 자기 자신을 기준 변으로 쓴다. 범례 자신을 기준으로 쓰면 작은 범례가 거의
    // 움직이지 못한다.
    //
    // 손잡이를 잡았을 때는 손잡이가 붙어 있는 대상을 거슬러 올라가 찾는다 — 크기 조절도
    // 같은 기준 상자를 써야 시작 크기와 이동량의 단위가 맞는다.
    const grabbedEl =
      valueEl ??
      legendEl ??
      bodyEl ??
      resizeEl!.closest('[data-gauge-threshold-legend]') ??
      resizeEl!.closest('[data-gauge-body]') ??
      // 값의 손잡이는 글자의 **형제**다(글자 폭을 알 수 없어 안에 둘 수 없다) —
      // 그래서 글자가 아니라 담는 상자로 거슬러 올라간다.
      resizeEl!.closest('[data-gauge-value-box]');
    if (!grabbedEl) return;
    // 값 글자와 범례는 담는 상자를, 게이지는 자기 자신을 기준 변으로 쓴다. 값·범례가
    // 제 상자를 기준으로 쓰면 작은 요소가 거의 움직이지 못한다.
    const box = kind === 'body' ? grabbedEl : grabbedEl.parentElement;
    const rect = box?.getBoundingClientRect();
    const legendRect = kind === 'legend' ? grabbedEl.getBoundingClientRect() : null;
    const elBox = grabbedEl.getBoundingClientRect();
    // 크기를 잴 수 없으면(레이아웃 전) 시작하지 않는다 — 0 으로 나눈 이동량이
    // 그림을 화면 밖으로 날린다.
    if (!rect || rect.width <= 0 || rect.height <= 0) return;
    e.preventDefault();
    const base = stateRef.current[kind];
    dragRef.current = {
      kind,
      mode: resizeEl ? 'resize' : 'move',
      startX: e.clientX,
      startY: e.clientY,
      baseX: base.offsetX,
      baseY: base.offsetY,
      baseSize: base.size ?? 0,
      baseSizeY: base.sizeY ?? 0,
      rect: { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
      elRect: { left: elBox.left, top: elBox.top, width: elBox.width, height: elBox.height },
      // 크기 조절은 잡은 대상 하나만 바꾼다 — 여러 대상에 같은 양을 더하는 것은 뜻이
      // 모호하다(글자 크기와 게이지 백분율이 섞인다).
      group: resizeEl
        ? [{ kind, offsetX: base.offsetX, offsetY: base.offsetY }]
        : [...picked].map((k) => ({
            kind: k,
            offsetX: stateRef.current[k].offsetX,
            offsetY: stateRef.current[k].offsetY,
          })),
      // 붙인 변은 범례가 스스로 알고 있다(`data-position`) — 설정을 다시 읽지 않는다.
      legendBox: legendRect
        ? {
            anchor: (grabbedEl.getAttribute('data-position') as LegendAnchor | null) ?? 'bottom',
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
