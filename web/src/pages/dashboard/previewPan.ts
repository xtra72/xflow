// 미리보기 이동(팬) — 순수 기하 + 상태 훅.
//
// `previewStage.ts` 의 이웃이다. 그 모듈이 "얼마나 축소해 보일까" 를 정하면 이 모듈은
// **"그 축소된 그림의 어디를 볼까"** 를 정한다. 둘을 한 파일에 두지 않은 이유는 하나가
// 순수 함수뿐인데 다른 하나는 포인터·키보드 상태를 들기 때문이고, 다른 곳에 두지 않은
// 이유는 줌과 팬이 **같은 한 상자**를 두고 하는 두 몸짓이기 때문이다.
//
// ## 왜 이 층에 있는가 — 줌과 같은 자리다
//
// 확대는 캔버스 전용이 아니다. 휠 확대 핸들러는 미리보기의 네 마운트 자리에 모두 붙어
// 있어 **패널 종류를 가리지 않는다**. 그래서 "확대하면 가장자리에 닿을 수 없다" 는 결함도
// 종류를 가리지 않으며, 그 답인 팬도 캔버스 안이 아니라 **줌과 같은 층**에 서야 한다.
// 캔버스 편집 안에 넣었다면 게이지·차트·히트맵 미리보기는 여전히 갇힌 채로 남는다.
//
// ## 몸짓의 소유권 — "아무도 가져가지 않은 것만 받는다"
//
// 미리보기 안에는 이미 제 몸짓을 가진 층이 여럿 있다: 캔버스 편집 오버레이(도형을 끈다),
// `PanelDragLayer`(게이지 값·범례를 끈다), 히트맵 마커·도면 변환, 도크. 이들이 몸짓을
// 가져갈 때 쓰는 표시는 이 저장소에 이미 둘 다 서 있다 —
//   - `stopPropagation()` : 이벤트가 위층에 **닿지도 않는다**(오버레이 히트 · 도크 · 도면).
//   - `preventDefault()`  : 닿기는 하되 **`defaultPrevented` 가 참**이다(`PanelDragLayer` ·
//                            히트맵 마커).
// 그래서 이 층의 규칙은 새 약속이 아니라 **이미 있는 그 두 표시를 읽는 것**이다:
//
//   > 팬은 fit 컨테이너까지 **소비되지 않은 채로 올라온** 몸짓만 받는다.
//
// 이 한 줄이 세 경우를 한꺼번에 정한다.
//   1. 캔버스에서 **도형을 잡으면** 오버레이가 끊으므로 팬은 시작하지 않는다.
//   2. 캔버스에서 **빈 자리를 주 버튼으로 잡아도** 오버레이가 가져가므로 팬은 시작하지
//      않는다 — 010 에서는 영역 선택이, 011 부터는 **오버레이 제 작업 영역 팬**이
//      가져간다(맨손 끌기와 조작키 끌기의 배정이 맞바뀌었다). 어느 쪽이든 이 층에 대한
//      답은 같다. 주 버튼이 아닌 누름은 종전대로 올라온다.
//   3. 캔버스가 아닌 패널은 아무도 가져가지 않으므로 표면 전체가 팬이다.
// 방향키도 같은 규칙이다 — 고른 도형이 있으면 오버레이가 옮기고 `preventDefault` 하므로
// 팬은 움직이지 않고, 고른 것이 없으면 오버레이가 흘려보내므로 팬이 화면을 옮긴다.
//
// **가운데 버튼은 그 규칙의 비상구다.** 캔버스 안에서는 1·2 가 모두 아래층의 것이라 주
// 버튼으로 화면을 옮길 길이 없다(010 이 2 를 가져가기 전에도 도형이 빽빽하면 빈 자리가
// 없어 같은 처지였다). 그래서 가운데 버튼만은 **캡처 단계**에서 가로채 아래층이 보기
// 전에 팬으로 돌린다(브라우저·CAD·도해 도구가 모두 쓰는 그 몸짓이다). 이것이 규칙의
// 예외가 아니라 규칙의 **명시적 우회로**인 이유는, 우회로가 하나뿐이고 그 하나가 다른 어떤
// 조작과도 겹치지 않기 때문이다.
//
// **가운데 버튼이 없는 손에는 방향키가 남는다.** 캔버스에서 고른 것이 없으면 오버레이가
// 방향키를 흘려보내므로 이 층이 받아 화면을 옮긴다(아래 §방향키). 빈 자리를 한 번 누르면
// 선택이 비고 초점이 오버레이의 탭 정거장에 앉으므로 그 상태는 한 번의 누름으로 닿는다.
// 그래도 좁은 길임을 감추지 않는다 — 주 버튼 끌기만큼 즉각적이지 않고, 이것이 010 이
// 빈 자리 끌기를 영역 선택에 내주며 치른 값이다.
//
// **011 이 그 값을 캔버스 안에서 돌려주었다.** 빈 자리 맨손 끌기는 이제 오버레이 제
// 팬이므로, 캔버스 안의 사용자는 이 층의 우회로를 찾지 않아도 화면을 옮긴다. 이 층의
// 규칙과 우회로는 한 글자도 바뀌지 않으며(캔버스 **밖**의 미리보기들이 여전히 이 층의
// 것이다), 바뀐 것은 캔버스 안에서 그 우회로를 쓸 일이 드물어졌다는 사실 하나다.
//
// ## 왜 좌표 보정이 필요 없는가
//
// 팬은 축소되는 상자에 **CSS `translate` 를 얹을 뿐**이다. `getBoundingClientRect()` 는
// 변환 **뒤**의 상자를 주므로 그 `left`/`top` 에 이동량이 이미 들어 있고, `width`/`height` 는
// 평행이동으로 달라지지 않는다. 캔버스 오버레이의 포인터 환산은 그 둘로만 이뤄지므로
// (`rect.width / stage.width` 와 `rect.left`) **보정항이 낄 자리가 없다** — 보정을 더하면
// 오히려 두 번 세는 것이 된다(SPEC-CANVAS-002 위험 R1 이 경고한 바로 그 형태).
//
// ## 프레임을 예약하지 않는다
//
// 이 모듈은 `requestAnimationFrame` 을 부르지 않고 요소 기하를 한 글자도 쓰지 않는다.
// 팬이 바꾸는 것은 상자의 `transform` 문자열 하나뿐이므로 캔버스 표면이 받는 props 는
// 그대로이고, 따라서 SPEC-CANVAS-001 REQ-05 의 유휴 정지가 그대로 유지된다.
//
// **DOM 무의존이 아니다**(React 훅이다). 다만 순수 기하 셋(`panBounds` · `clampPanOffset` ·
// `arrowPanDelta`)은 훅 밖에 두어 jsdom 없이 단위 시험된다.
//
// @spec SPEC-CANVAS-002 REQ-05

import { useEffect, useRef, useState, type PointerEvent, type KeyboardEvent } from 'react';

// --- 타입 ---------------------------------------------------------------

/** 화면 px 이동량. 축소된 상자에 그대로 `translate` 로 얹힌다. */
export interface PanOffset {
  x: number;
  y: number;
}

/** 축마다의 이동 상한(절댓값). 0 이면 그 축은 넘치지 않아 움직일 곳이 없다. */
export interface PanBounds {
  x: number;
  y: number;
}

/** 화면 px 크기 한 쌍 — 넘치는 상자와 그것을 담는 영역이 같은 형상을 쓴다. */
export interface PanSize {
  w: number;
  h: number;
}

// --- 상수 ---------------------------------------------------------------

const ORIGIN: PanOffset = { x: 0, y: 0 };
const NO_BOUNDS: PanBounds = { x: 0, y: 0 };

/**
 * 방향키 한 번의 이동량(화면 px).
 *
 * 24px 인 근거: 미리보기 영역은 대체로 500~800px 폭이므로 한 번 눌러 눈에 보이게
 * 움직이면서도 가장자리를 지나쳐 튀지 않는 크기가 그쯤이다. 더 작으면(4·8px) 끝까지
 * 가는 데 백 번을 눌러야 하고, 더 크면 원하는 자리에 세울 수 없다.
 */
export const PREVIEW_PAN_ARROW_PX = 24;

/**
 * Shift+방향키 한 번의 이동량(화면 px) — 잔걸음의 다섯 배다.
 *
 * Shift 를 고른 이유: Ctrl/Cmd 는 이미 휠 확대의 짝이고 Alt 는 OS 가 가져간다. 캔버스
 * 오버레이도 Shift+방향키를 쓰지만 그쪽은 **고른 도형이 있을 때**만 동작하고, 팬은
 * **고른 것이 없을 때**만 동작하므로 둘이 같은 순간에 뜻을 다투지 않는다.
 */
export const PREVIEW_PAN_ARROW_COARSE_PX = PREVIEW_PAN_ARROW_PX * 5;

/** 가운데 버튼(`PointerEvent.button`). 규칙의 명시적 우회로다(머리말 §몸짓의 소유권). */
const MIDDLE_BUTTON = 1;
/** 주 버튼. 오른쪽 버튼(2)은 상황 메뉴의 것이므로 팬이 가져가지 않는다. */
const PRIMARY_BUTTON = 0;

/** 방향키 → 화면 이동 방향. **끄는 것과 같은 뜻**이다 — 오른쪽 키는 패널을 오른쪽으로 옮긴다. */
const ARROW_SIGNS: Record<string, PanOffset> = {
  ArrowLeft: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 },
  ArrowUp: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 },
};

/** 보조기기가 읽을 단축키 이름. 캔버스 오버레이가 제 것을 알리는 방식과 같다. */
export const PREVIEW_PAN_KEY_SHORTCUTS =
  'ArrowUp ArrowDown ArrowLeft ArrowRight ' +
  'Shift+ArrowUp Shift+ArrowDown Shift+ArrowLeft Shift+ArrowRight';

// --- 순수 기하 -----------------------------------------------------------

/**
 * 한 축의 이동 상한.
 *
 * 상자는 영역 **가운데**에 놓이므로 양쪽으로 넘치는 양은 각각 `(상자 - 영역) / 2` 다.
 * 그만큼 옮기면 상자의 그 모서리가 영역의 모서리에 정확히 닿는다 — 그 이상은 그림이
 * 영역 안으로 물러나 빈 자리가 생기므로 허용하지 않는다.
 *
 * 넘치지 않으면 0 이다. 0 은 "움직일 곳이 없다" 는 뜻이고, 그 사실 하나가 잡을 커서도
 * 방향키 반응도 함께 끈다 — 눌러도 아무 일이 없는 조작은 고장으로 보인다.
 */
function axisBound(contentLen: number, areaLen: number): number {
  if (!Number.isFinite(contentLen) || !Number.isFinite(areaLen)) return 0;
  const over = contentLen - areaLen;
  return over > 0 ? over / 2 : 0;
}

/**
 * 넘치는 상자와 담는 영역에서 축마다의 이동 상한을 구한다.
 *
 * 상자를 아직 알 수 없으면(실측 전) 상한이 없다 — 그때는 배율도 모르므로 무엇을 얼마나
 * 옮길지 물을 수 없다.
 */
export function panBounds(content: PanSize | null, area: PanSize): PanBounds {
  if (content === null) return NO_BOUNDS;
  return { x: axisBound(content.w, area.w), y: axisBound(content.h, area.h) };
}

/** 이동량을 상한 안으로 죈다. 잴 수 없는 값(NaN/Infinity)은 0 으로 떨어뜨린다. */
function clampAxis(value: number, bound: number): number {
  if (!Number.isFinite(value)) return 0;
  if (!(bound > 0)) return 0;
  return Math.min(bound, Math.max(-bound, value));
}

/**
 * 이동량을 상한 안으로 죈다.
 *
 * **줌을 낮추면 상한이 줄어든다** — 그때 옛 이동량을 그대로 두면 그림이 영역 밖으로
 * 밀려나 빈 화면이 남는다. 그래서 이 함수는 끌 때뿐 아니라 **그릴 때마다** 지난다.
 */
export function clampPanOffset(offset: PanOffset, bounds: PanBounds): PanOffset {
  return { x: clampAxis(offset.x, bounds.x), y: clampAxis(offset.y, bounds.y) };
}

/**
 * 방향키 한 번의 이동량. 방향키가 아니면 `null` 이다.
 *
 * 부호는 **끄는 것과 같다** — 사용자가 하는 일은 "패널을 옮기는 것" 하나이고, 손으로
 * 옮기는 것과 키로 옮기는 것이 반대 방향이면 그것은 두 가지 조작이 된다.
 */
export function arrowPanDelta(key: string, shift: boolean): PanOffset | null {
  const sign = ARROW_SIGNS[key];
  if (sign === undefined) return null;
  const amount = shift ? PREVIEW_PAN_ARROW_COARSE_PX : PREVIEW_PAN_ARROW_PX;
  return { x: sign.x * amount, y: sign.y * amount };
}

// --- 훅 -----------------------------------------------------------------

/** fit 컨테이너에 그대로 펼치는 props 한 벌. */
export interface PreviewPanSurfaceProps {
  tabIndex?: number;
  role?: 'group';
  'aria-label'?: string;
  'aria-keyshortcuts'?: string;
  'data-testid': string;
  'data-can-pan': string;
  'data-panning': string;
  onPointerDownCapture: (event: PointerEvent<HTMLElement>) => void;
  onPointerDown: (event: PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: PointerEvent<HTMLElement>) => void;
  onPointerUp: (event: PointerEvent<HTMLElement>) => void;
  onPointerCancel: (event: PointerEvent<HTMLElement>) => void;
  onKeyDown: (event: KeyboardEvent<HTMLElement>) => void;
}

export interface PreviewPan {
  /** 상자에 얹을 화면 px 이동량. 넘치지 않으면 언제나 (0, 0) 이다. */
  offset: PanOffset;
  /** 옮길 곳이 있는가 — 넘치는 축이 하나라도 있는가. */
  canPan: boolean;
  /** 지금 끌고 있는가(커서 모양이 이 값으로 갈린다). */
  panning: boolean;
  /** 잡는 손 모양. 옮길 곳이 없으면 빈 문자열이라 커서가 달라지지 않는다. */
  cursorClass: string;
  surfaceProps: PreviewPanSurfaceProps;
}

interface PanDrag {
  pointerId: number;
  startX: number;
  startY: number;
  base: PanOffset;
}

/**
 * 축소된 미리보기를 끌어 옮긴다.
 *
 * `content` 는 상자가 **화면에서 차지하는** 크기(줌이 곱해진 뒤)이고 `area` 는 그것을
 * 담는 fit 컨테이너의 실측 크기다. 둘의 차이가 곧 옮길 수 있는 거리다.
 *
 * 돌려주는 `surfaceProps` 를 컨테이너에 그대로 펼치면 끝이다 — 부르는 쪽이 조립할 것이
 * 없어야 이 규칙이 한 곳에만 있다(`PanelSettingsDialog` 는 8천 줄이고, 그 파일에 조작
 * 규칙이 흩어지면 다음 사람이 그것을 찾지 못한다 — SPEC-CANVAS-002 위험 R5).
 *
 * @param labels 보조기기용 이름. i18n 을 이 모듈에 들이지 않으려고 문자열로 받는다 —
 *   순수 기하와 상태만 남기면 이 파일은 번역 트리와 무관해진다.
 */
export function usePreviewPan(
  content: PanSize | null,
  area: PanSize,
  labels: { surfaceAria: string },
): PreviewPan {
  const bounds = panBounds(content, area);
  const canPan = bounds.x > 0 || bounds.y > 0;

  const [rawOffset, setRawOffset] = useState<PanOffset>(ORIGIN);
  const [panning, setPanning] = useState(false);
  const dragRef = useRef<PanDrag | null>(null);

  // 그릴 때마다 죈다 — 줌을 낮추거나 영역이 넓어지면 상한이 **그 프레임에** 줄어들고,
  // 한 프레임이라도 늦으면 그림이 밖으로 나갔다 돌아오는 것이 눈에 보인다.
  const offset = clampPanOffset(rawOffset, bounds);
  // 효과가 상자가 아니라 **두 수**를 보게 한다 — 상자는 그릴 때마다 새로 만들어지므로
  // 의존성에 넣으면 매 렌더마다 도는 효과가 된다.
  const { x: offsetX, y: offsetY } = offset;

  // 죈 값을 상태에도 되돌려 놓는다. 그러지 않으면 축소했다가 다시 확대할 때 옛 이동량이
  // 되살아나 그림이 튄다 — 사용자가 "전체를 보려고" 축소한 그 순간에 자리는 이미 뜻을
  // 잃었다. 이미 죈 값을 다시 죄면 같은 값이므로 이 효과는 한 번 돌고 멈춘다.
  useEffect(() => {
    setRawOffset((prev) => (prev.x === offsetX && prev.y === offsetY ? prev : { x: offsetX, y: offsetY }));
  }, [offsetX, offsetY]);

  /** 끌기를 시작한다. 시작했으면 참. */
  const begin = (event: PointerEvent<HTMLElement>): boolean => {
    if (!canPan) return false;
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      base: offset,
    };
    setPanning(true);
    // 포인터가 영역을 벗어나도 이벤트가 계속 오게 한다. jsdom 에는 없는 API 다.
    event.currentTarget.setPointerCapture?.(event.pointerId);
    return true;
  };

  const finish = (event: PointerEvent<HTMLElement>): void => {
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    dragRef.current = null;
    setPanning(false);
  };

  /**
   * 가운데 버튼 — **캡처 단계**에서 가로챈다.
   *
   * 아래층(캔버스 오버레이)이 보기 **전**에 끊어야 도형 위에서도 화면이 옮겨진다. 이것이
   * 이 파일에서 유일하게 "소비되지 않은 것만 받는다" 규칙 위에 서는 자리이며, 그래서
   * 하나뿐이다.
   */
  const onPointerDownCapture = (event: PointerEvent<HTMLElement>): void => {
    if (event.button !== MIDDLE_BUTTON) return;
    if (!begin(event)) return;
    // 브라우저의 가운데 버튼 자동 스크롤과 아래층의 히트 테스트를 함께 막는다.
    event.preventDefault();
    event.stopPropagation();
  };

  /**
   * 주 버튼 — **아무도 가져가지 않은 몸짓만** 받는다(머리말 §몸짓의 소유권).
   *
   * `stopPropagation` 한 층의 누름은 여기 닿지도 않고, `preventDefault` 한 층의 누름은
   * 닿되 `defaultPrevented` 로 구분된다. 두 표시 모두 이 저장소에 이미 서 있던 것이라
   * 새로 약속할 것이 없다.
   */
  const onPointerDown = (event: PointerEvent<HTMLElement>): void => {
    if (event.button !== PRIMARY_BUTTON) return;
    if (event.defaultPrevented) return;
    if (!begin(event)) return;
    // 끄는 동안 글자가 선택되면 파란 하이라이트가 그림을 덮는다(`PanelDragLayer` 와 같은 이유).
    event.preventDefault();
  };

  const onPointerMove = (event: PointerEvent<HTMLElement>): void => {
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    setRawOffset(
      clampPanOffset(
        {
          x: drag.base.x + (event.clientX - drag.startX),
          y: drag.base.y + (event.clientY - drag.startY),
        },
        bounds,
      ),
    );
  };

  /**
   * 방향키 이동 — 드래그의 키보드 등가물이다.
   *
   * **실제로 옮겼을 때에만 소비한다.** 고른 도형이 있어 아래층이 이미 옮겼거나(그때는
   * `defaultPrevented` 다), 이미 끝에 닿아 한 픽셀도 움직이지 않았다면 이벤트를 그대로
   * 흘려보낸다 — 아무 일도 하지 않으면서 브라우저의 스크롤·초점 이동을 막는 것이 곧
   * 접근성 결함이다(캔버스 오버레이가 같은 규율을 따른다).
   */
  const onKeyDown = (event: KeyboardEvent<HTMLElement>): void => {
    if (!canPan) return;
    if (event.defaultPrevented) return;
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    const delta = arrowPanDelta(event.key, event.shiftKey);
    if (delta === null) return;
    const next = clampPanOffset({ x: offset.x + delta.x, y: offset.y + delta.y }, bounds);
    if (next.x === offset.x && next.y === offset.y) return;
    setRawOffset(next);
    event.preventDefault();
  };

  return {
    offset,
    canPan,
    panning,
    cursorClass: canPan ? (panning ? 'cursor-grabbing' : 'cursor-grab') : '',
    surfaceProps: {
      // 옮길 곳이 없으면 탭 차례에 끼지 않는다 — 눌러도 아무 일이 없는 정거장이 된다.
      tabIndex: canPan ? 0 : undefined,
      role: canPan ? 'group' : undefined,
      'aria-label': canPan ? labels.surfaceAria : undefined,
      'aria-keyshortcuts': canPan ? PREVIEW_PAN_KEY_SHORTCUTS : undefined,
      'data-testid': 'preview-pan-surface',
      'data-can-pan': canPan ? 'true' : 'false',
      'data-panning': panning ? 'true' : 'false',
      onPointerDownCapture,
      onPointerDown,
      onPointerMove,
      onPointerUp: finish,
      onPointerCancel: finish,
      onKeyDown,
    },
  };
}

/**
 * 상자에 얹을 `transform` 문자열.
 *
 * **이동량이 0 이면 배율만 낸다.** 그래야 팬을 쓰지 않는 흔한 경우의 DOM 이 이 기능이
 * 들어오기 전과 한 글자도 다르지 않다 — 쓰지 않는 기능이 화면에 흔적을 남기면 그 흔적이
 * 다음 사람에게 "여기 무언가 있다" 고 말한다.
 */
export function panTransform(offset: PanOffset, scaleX: number, scaleY: number): string {
  const scale = `scale(${scaleX}, ${scaleY})`;
  if (offset.x === 0 && offset.y === 0) return scale;
  return `translate(${offset.x}px, ${offset.y}px) ${scale}`;
}
