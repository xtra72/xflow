// 패널 요소를 **끌어 옮기고 모서리 핸들로 크기를 바꾸는** 레이어.
//
// 통계·바·파이가 함께 쓴다. 끌 대상은 패널마다 다르므로(통계 3 · 파이 2 · 바 2)
// 고정 prop 이 아니라 **대상 맵**을 받는다.
//
// 잡은 대상으로 무엇이 움직일지 갈린다 — `data-panel-resize` 를 잡으면 그 요소의 글자
// 크기가, `data-panel-drag` 를 잡으면 그 요소의 자리가 바뀐다. 그 밖을 잡으면 아무 일도
// 없다: 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있어서, 아무 데나
// 잡아도 끌리면 그것들과 부딪힌다(게이지·파이와 같은 규칙).
//
// 게이지와 달리 **좌표계가 하나다.** 자리는 전부 기준 상자(`data-panel-bounds`) 대비
// 백분율이고, 크기만 절대 px 이다. 픽셀 자리를 쓰면 미리보기와 실제 패널의 크기가 달라
// 같은 값이 다른 자리를 가리킨다.
//
// 대시보드에 놓인 패널은 편집이 꺼져 있으면 표시만 남고 끌리지 않는다.
//
// @spec SPEC-CHART-004 §2.1 [U1] / §2.2 [U2]

import { useEffect, useRef } from 'react';

import { clampFontSize } from './panels/charts/statLayout';
import { snapOffsetToGrid } from './panels/charts/panelEditAlign';
import {
  clampGroupDelta,
  EMPTY_SELECTION,
  nextSelection,
  type PanelSelection,
} from './panels/charts/panelEditSelection';
import { clampPercentOffset, pixelsToPercent } from './panels/charts/panelGeometry';

/** 끌 수 있는 요소 하나. */
export interface PanelDragTarget {
  /** 현재 가로 오프셋(백분율 포인트). */
  offsetX: number;
  /** 현재 세로 오프셋(백분율 포인트). */
  offsetY: number;
  /** 현재 글자 크기(px). 크기 핸들의 시작점이다. */
  fontSize: number;
  /** 끄는 동안 계속 호출된다 — 미리보기가 즉시 따라와야 어디에 놓일지 보인다. */
  onMove: (next: { x: number; y: number }) => void;
  onResize: (fontSize: number) => void;
  /**
   * 오프셋 상한(백분율 포인트). 미지정이면 ±40(`PANEL_OFFSET_LIMIT`).
   *
   * **상한은 레이어가 아니라 요소의 성질이 정한다.** 이 레이어는 성질이 다른 요소를
   * 함께 나른다 — 통계 세 줄과 범례는 가운데에서 시작하는 작은 글자 덩어리라 모서리에
   * 닿으려면 축마다 50 이 필요하고(`STAT_OFFSET_LIMIT`), 파이 그림과 바 그림 영역은
   * 영역을 가득 채우므로 40 만 넘어도 절반이 잘린다(`PANEL_OFFSET_LIMIT`).
   *
   * 기본값을 느슨한 쪽(50)이 아니라 **40** 으로 두는 이유: 이 값은 읽는 쪽의 상한과
   * 같아야 한다. 쓰기가 더 느슨하면 끄는 동안에는 50 까지 따라오다가 다음에 config 를
   * 읽는 순간 40 으로 되돌아가, 방금 맞춘 자리가 소리 없이 튄다(실제로 그렇게 보고됐다).
   * 상한을 밝히지 않은 새 대상이 붙었을 때 조용히 틀리는 쪽은 느슨한 기본값이다.
   */
  limit?: number;
  /** 크기의 하한·상한. 미지정이면 글자 크기 범위(6~160)로 죈다. */
  sizeRange?: { min: number; max: number };
  /**
   * 세로 크기 — **주면 손잡이가 축을 나눈다**(가로 이동은 `fontSize`, 세로 이동은 `sizeY`).
   *
   * 사각형 도형에만 준다. 바 차트가 그렇다 — 가로는 막대 굵기(px), 세로는 그림 영역
   * 배율(%)이다. 파이처럼 비율이 뜻을 갖는 도형은 생략하고, 그러면 종전대로 대각선
   * 투영으로 한 값을 키운다.
   *
   * 두 축의 단위가 서로 달라도 되므로 범위도 축마다 따로 받는다.
   */
  sizeY?: number;
  onResizeY?: (next: number) => void;
  sizeYRange?: { min: number; max: number };
}

/**
 * 크기를 범위 안으로 죈다. 범위를 주지 않으면 글자 크기(6~160)로 본다 — 손잡이를 쓰는
 * 대상 중 범위를 밝히지 않는 것은 글자 덩어리(통계 세 줄·범례)뿐이다.
 */
function clampSize(
  raw: number,
  range: { min: number; max: number } | undefined,
  fallback: number,
): number {
  if (range) return Math.min(Math.max(raw, range.min), range.max);
  return clampFontSize(raw) ?? fallback;
}

type Mode = 'move' | 'resize';

/**
 * 편집 중 요소에 붙는 CSS 클래스 — **잡을 수 있는 자리를 화면에 보여 준다.**
 *
 * 커서 모양만으로는 부족하다. 통계 패널은 글자만 있는 화면이라, 대상 위에 포인터를
 * 올리기 전에는 무엇이 잡히는지 알 수 없다. 점선 윤곽이 각 요소의 상자를 그려
 * "이 덩어리가 하나의 단위" 라는 사실을 먼저 보여 준다.
 *
 * 윤곽은 `outline` 으로 그린다 — `border` 는 상자 크기를 바꿔 편집을 켜는 순간 배치가
 * 흔들리고, 그 흔들림이 사용자가 맞춘 자리를 어긋나 보이게 한다.
 */
export const PANEL_EDIT_OUTLINE_CLASS =
  'outline-dashed outline-1 outline-offset-4 outline-blue-400/40 rounded-sm';

/**
 * 고른 요소의 윤곽 — 실선이고 더 진하다.
 *
 * 점선/실선과 진하기를 **함께** 바꾼다. 색만 바꾸면 흐린 화면이나 색각 이상에서 두
 * 상태가 구분되지 않는다.
 */
export const PANEL_SELECTED_OUTLINE_CLASS =
  'outline outline-2 outline-offset-4 outline-blue-500 rounded-sm';

/**
 * 요소 우하단의 크기 핸들.
 *
 * 레이어가 아니라 **요소가** 그린다 — 핸들의 자리는 그 요소의 상자에 붙어야 하는데,
 * 레이어는 요소가 어디에 어떤 크기로 놓였는지 모른다.
 */
export function PanelResizeHandle<K extends string>({
  kind,
  enabled,
  label,
  className,
}: {
  kind: K;
  enabled: boolean;
  /** 스크린리더용 이름. 요소마다 다르다. */
  label: string;
  /**
   * 놓을 자리를 바꾼다. 기본은 상자 **밖** 모서리(`-bottom-2 -right-2`)지만, 담는 요소가
   * 넘침을 자르면(`overflow-auto` 인 범례 등) 밖으로 나간 손잡이가 잘려 보이지 않는다.
   * 그런 요소는 안쪽 모서리로 옮겨 준다.
   */
  className?: string;
}): React.ReactElement | null {
  if (!enabled) return null;
  return (
    <span
      data-panel-resize={kind}
      data-testid={`panel-resize-${kind}`}
      role="slider"
      aria-label={label}
      aria-valuenow={0}
      tabIndex={-1}
      className={
        className ??
        'absolute -bottom-2 -right-2 h-3 w-3 cursor-nwse-resize rounded-sm border border-white bg-blue-500 shadow'
      }
    />
  );
}

export function PanelDragLayer<K extends string>({
  enabled = true,
  snap = true,
  selection,
  onSelectionChange,
  targets,
  children,
}: {
  /**
   * 끌 수 있는가. 대시보드 패널은 **배치 편집 모드일 때만** 켠다 — 늘 켜 두면 패널을
   * 옮기거나 크기를 바꾸려는 조작과 부딪힌다. 미리보기는 언제나 켠다.
   */
  enabled?: boolean;
  /** 격자에 붙일지. 끄는 동안 Alt 를 누르면 이 값과 무관하게 잠시 꺼진다. */
  snap?: boolean;
  /** 지금 고른 요소들. 끌면 이 전체가 함께 움직인다. */
  selection: PanelSelection<K>;
  onSelectionChange: (next: PanelSelection<K>) => void;
  /**
   * 끌 수 있는 대상들. 키가 곧 `data-panel-drag` / `data-panel-resize` 표식의 값이다.
   *
   * 고정 prop 이 아니라 맵인 이유: 패널마다 대상 수와 이름이 다르다. 맵이면 이 레이어가
   * 대상 목록을 알 필요가 없고, 패널이 늘어도 여기를 고치지 않는다.
   */
  targets: Readonly<Record<K, PanelDragTarget>>;
  children: React.ReactNode;
}): React.ReactElement {
  // 드래그 중에만 존재하는 상태. 시작 시점의 값과 포인터 위치를 잡아 두고, 이후에는
  // 그 스냅샷 + 이동량으로만 계산한다 — 매 프레임 DOM 을 다시 재면 리렌더와 얽혀
  // 값이 떨린다(게이지와 같은 방식).
  const dragRef = useRef<{
    kind: K;
    mode: Mode;
    startX: number;
    startY: number;
    baseX: number;
    baseY: number;
    baseSize: number;
    baseSizeY: number;
    rect: { left: number; top: number; width: number; height: number };
    /** 잡은 요소의 상자 — 격자 스냅이 중심을 구하는 데 쓴다. */
    elRect: { left: number; top: number; width: number; height: number };
    /**
     * 함께 움직이는 요소들의 시작 오프셋.
     *
     * 잡은 것 하나만 끌 때도 길이 1 인 무리다 — 갈래를 나누면 두 경로가 서로 다르게
     * 죄어져 "혼자일 때와 여럿일 때 상한이 다르다" 가 된다.
     */
    group: Array<{ kind: K; offsetX: number; offsetY: number; limit?: number }>;
  } | null>(null);

  // 최신 콜백·값을 ref 로 안정화한다. document 리스너를 매 렌더 다시 달지 않는다.
  const stateRef = useRef(targets);
  stateRef.current = targets;
  // `snap` 도 같은 이유로 ref 에 둔다 — document 리스너는 마운트 시 한 번만 단다.
  const snapRef = useRef(snap);
  snapRef.current = snap;

  // 프레임당 한 번만 반영한다. 포인터 이벤트는 기기에 따라 프레임보다 자주 오는데,
  // 그때마다 config 를 고치면 화면에 보이지도 않을 렌더가 쌓여 오히려 끊긴다.
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<
    | { mode: 'move'; items: Array<{ kind: K; x: number; y: number }> }
    | { kind: K; mode: 'resize'; size: number; sizeY?: number }
    | null
  >(null);

  useEffect(() => {
    const flush = (): void => {
      frameRef.current = null;
      const next = pendingRef.current;
      pendingRef.current = null;
      if (!next) return;
      if (next.mode === 'resize') {
        stateRef.current[next.kind].onResize(next.size);
        if (next.sizeY !== undefined) stateRef.current[next.kind].onResizeY?.(next.sizeY);
        return;
      }
      for (const item of next.items) {
        stateRef.current[item.kind].onMove({ x: item.x, y: item.y });
      }
    };
    const onMove = (e: PointerEvent): void => {
      const d = dragRef.current;
      if (!d) return;
      const dx = e.clientX - d.startX;
      const dy = e.clientY - d.startY;
      if (d.mode === 'resize') {
        // 대각선 **투영**을 px 에 더한다 — 손잡이를 오른쪽 아래로 끈 거리만큼 글자가
        // 커진다(spec.md §7 OQ1). `dx + dy` 를 그대로 쓰면 대각선으로 끌 때 포인터보다
        // 두 배 빨리 커져 조작이 튄다.
        //
        // 소수 px 을 죄지 않는다 — 정수로 반올림하면 포인터를 천천히 움직여도 크기가
        // 계단으로 뛴다. 브라우저는 소수 `font-size` 를 그대로 그린다.
        const t = stateRef.current[d.kind];
        pendingRef.current =
          t.sizeY !== undefined
            ? {
                // 사각형 도형은 축을 나눈다 — 가로로 끌면 폭, 세로로 끌면 높이.
                // 대각선 투영으로는 "가늘고 길게" 를 만들 수 없다.
                kind: d.kind,
                mode: 'resize',
                size: clampSize(d.baseSize + dx, t.sizeRange, d.baseSize),
                sizeY: clampSize(d.baseSizeY + dy, t.sizeYRange, d.baseSizeY),
              }
            : {
                kind: d.kind,
                mode: 'resize',
                size: clampSize(d.baseSize + (dx + dy) / 2, t.sizeRange, d.baseSize),
              };
      } else {
        // 이동량은 **잡은 요소**가 정한다 — 무리는 그 값을 그대로 받아 상대 배치를
        // 유지한다. 요소마다 따로 계산하면 격자 스냅이 저마다 다른 자리에 붙어 대형이
        // 무너진다.
        // 상한은 **잡은 요소**의 것을 쓴다(미지정이면 ±40). 한 값으로 묶으면 글자
        // 덩어리와 영역을 채우는 그림 중 한쪽이 반드시 읽는 쪽과 어긋난다.
        const limit = stateRef.current[d.kind].limit;
        let x = clampPercentOffset(d.baseX + pixelsToPercent(dx, d.rect.width), limit);
        let y = clampPercentOffset(d.baseY + pixelsToPercent(dy, d.rect.height), limit);
        // Alt 를 누르고 있으면 격자를 잠시 끈다 — 정밀 조정이 막히면 격자가 오히려
        // 방해가 된다. 스냅 기준은 요소의 중심이며 계산은 `statAlign` 이 소유한다.
        if (snapRef.current && !e.altKey) {
          // 스냅에도 같은 상한을 넘긴다 — 스냅은 중심을 격자로 끌어당기므로 죄기 뒤에
          // 값을 반 칸까지 도로 밀어낼 수 있고, 여기서 상한이 느슨하면 방금 죈 값이 풀린다.
          x = snapOffsetToGrid(
            x,
            { offset: d.baseX, rectStart: d.elRect.left, rectLen: d.elRect.width },
            { start: d.rect.left, len: d.rect.width },
            limit,
          );
          y = snapOffsetToGrid(
            y,
            { offset: d.baseY, rectStart: d.elRect.top, rectLen: d.elRect.height },
            { start: d.rect.top, len: d.rect.height },
            limit,
          );
        }
        // 무리 전체가 갈 수 있는 만큼으로 이동량을 죈다 — 한 요소가 상한에 닿으면
        // 다 같이 멈춰야 상대 배치가 유지된다. 상한은 요소마다 다르므로 무리에 한 값을
        // 씌우지 않고 각자의 것을 들려 보낸다(무리에는 그림과 범례가 섞일 수 있다).
        const capped = clampGroupDelta(d.group, x - d.baseX, y - d.baseY);
        pendingRef.current = {
          mode: 'move',
          items: d.group.map((m) => ({
            kind: m.kind,
            x: m.offsetX + capped.dx,
            y: m.offsetY + capped.dy,
          })),
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
    const el = e.target as Element | null;
    // 핸들을 먼저 본다 — 핸들은 요소 **안**에 있으므로 순서를 뒤집으면 핸들을 잡아도
    // 요소가 통째로 움직인다.
    const resizeEl = el?.closest?.('[data-panel-resize]');
    const moveEl = resizeEl ? null : el?.closest?.('[data-panel-drag]');
    const host = resizeEl ?? moveEl;
    if (!host) {
      // 빈 자리를 누르면 선택을 푼다. 기준 상자 밖(툴바 등)은 건드리지 않는다 —
      // 단추를 누를 때마다 선택이 풀리면 정렬을 이어서 쓸 수 없다.
      if (el?.closest?.('[data-panel-bounds]')) onSelectionChange(EMPTY_SELECTION);
      return;
    }
    const kind = (resizeEl?.getAttribute('data-panel-resize') ??
      moveEl?.getAttribute('data-panel-drag')) as K | null;
    if (!kind) return;

    // Shift·Ctrl·Cmd 는 **고르기 전용** 조작이다 — 그 상태로 끌리면 무리에 넣으려다
    // 배치가 흐트러진다.
    const additive = e.shiftKey || e.ctrlKey || e.metaKey;
    const picked = nextSelection(selection, kind, additive);
    if (picked !== selection) onSelectionChange(picked);
    if (additive) return;

    // 자리는 **기준 상자** 대비 백분율이다. 요소 자신을 기준으로 쓰면 작은 요소가 거의
    // 움직이지 못하고, 큰 요소는 한 번에 화면을 가로지른다.
    const bounds = host.closest('[data-panel-bounds]');
    const rect = bounds?.getBoundingClientRect();
    // 크기를 잴 수 없으면(레이아웃 전) 시작하지 않는다 — 0 으로 나눈 이동량이 요소를
    // 화면 밖으로 날린다.
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
      baseSize: base.fontSize,
      baseSizeY: base.sizeY ?? 0,
      rect: { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
      elRect: (() => {
        const r = (moveEl ?? host).getBoundingClientRect();
        return { left: r.left, top: r.top, width: r.width, height: r.height };
      })(),
      // 크기 조절은 잡은 요소 하나만 바꾼다 — 여러 요소에 같은 px 을 더하는 것은 뜻이
      // 모호하다(글자 크기가 제각각이면 어떤 것을 기준으로 삼는지 알 수 없다).
      group: resizeEl
        ? [{ kind, offsetX: base.offsetX, offsetY: base.offsetY, limit: base.limit }]
        : [...picked].map((k) => ({
            kind: k,
            offsetX: stateRef.current[k].offsetX,
            offsetY: stateRef.current[k].offsetY,
            limit: stateRef.current[k].limit,
          })),
    };
  };

  return (
    <div
      className={
        enabled
          ? // 끄는 동안 글자가 선택되면 파란 하이라이트가 덮여 어디에 놓이는지 보이지 않는다.
            'contents [&_[data-panel-drag]]:cursor-move [&_[data-panel-drag]]:select-none'
          : 'contents'
      }
      data-testid="panel-drag-layer"
      onPointerDown={onPointerDown}
    >
      {children}
    </div>
  );
}
