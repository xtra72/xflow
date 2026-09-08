// 캔버스 표면 컴포넌트 — <canvas> + rAF 루프 + DPR/ResizeObserver + 가시성 게이팅
// (SPEC-CANVAS-001 T10).
//
// 이 컴포넌트가 지는 책임은 셋이며, 셋 다 인수 기준에 묶여 있다.
//   1) 백킹 버퍼(AC-E5) — 표시 크기·devicePixelRatio 가 바뀌면 버퍼를 다시 잡고 다시 그린다.
//      정규화 좌표를 새 크기로 재투영하므로 요소는 화면상 같은 상대 위치에 남는다.
//   2) 트윈 장부(AC-03) — 요소별 `TweenState` 를 들고, 목표가 바뀌면 **진행 중인 값에서**
//      다시 트윈한다(retarget). 그래서 값이 튀지 않는다.
//   3) 루프 규율(AC-E6) — 모든 트윈이 끝난 프레임 뒤에는 다음 프레임을 예약하지 않고
//      (유휴 정지), 보이지 않는 동안에는 아예 예약하지 않는다(가시성 게이팅).
//
// **주입 가능한 두 축**이 이 컴포넌트의 테스트 가능성 전부다. 가시성은 이미 코드베이스에
// 있는 `VisibilitySource`(visiblePolling.ts)를 그대로 재사용하고, 프레임 예약은 같은
// 모양의 `FrameScheduler` 를 새로 둔다. 둘 다 기본값이 브라우저 구현이므로 운영 경로는
// 평범하고, 테스트는 가짜 시계로 프레임을 한 장씩 밀어 볼 수 있다(fake timer 불필요).
//
// 한 프레임 = 전체 다시 그리기다(§렌더 루프 3). 부분 무효화(dirty rect)는 하지 않는다.
// 반대로 **props 가 그대로면 아무것도 그리지 않는다** — 폴링이 실패해도 마지막 프레임이
// 그대로 남아 있어야 하기 때문이다(AC-E4).
//
// @spec SPEC-CANVAS-001

import { useCallback, useEffect, useRef, useState } from 'react';

import { documentVisibility, type VisibilitySource } from '../charts/visiblePolling';
import type { CanvasElement, TweenSpec } from './canvasConfig';
import { computeBackingSize, type StageSize } from './canvasGeometry';
import type { ResolvedStyle } from './canvasRules';
import { beginTween, retargetTween, sampleTween, type TweenState } from './canvasTween';
import { clearSurface, drawElements, type DrawContext2D } from './drawElement';

// --- 주입 지점 -----------------------------------------------------------

/**
 * 프레임 예약기. 기본 구현은 `requestAnimationFrame` 이며, 테스트는 이 자리에 가짜를
 * 넣어 프레임을 원하는 시각으로 한 장씩 진행시킨다 — `VisibilitySource` 가 가시성에
 * 대해 해 주는 일과 같은 것을 시간에 대해 한다.
 */
export interface FrameScheduler {
  request(cb: (nowMs: number) => void): number;
  cancel(handle: number): void;
}

/** 브라우저 기본 예약기. */
const DEFAULT_SCHEDULER: FrameScheduler = {
  request: (cb) => requestAnimationFrame(cb),
  cancel: (handle) => cancelAnimationFrame(handle),
};

export interface CanvasSurfaceProps {
  /** 배열 순서 = 그리기 순서(뒤가 위). */
  elements: CanvasElement[];
  /** 요소 id → 규칙 평가가 확정한 목표 스타일. 없으면 요소의 기본 스타일을 쓴다. */
  targetStyles: Record<string, ResolvedStyle>;
  /** 요소 id → 토큰 치환이 끝난 문구. 없으면 요소의 기본 문구를 쓴다. */
  texts: Record<string, string | undefined>;
  /** 패널 기본 트윈. 요소의 `tween` 이 있으면 그쪽이 이긴다. */
  panelTween?: TweenSpec;
  /** 캔버스 배경색. 미지정이면 지우기만 해 패널 표면색이 비친다. */
  background?: string;
  /** 가시성 판정. 기본은 document. */
  visibilitySource?: VisibilitySource;
  /** 프레임 예약기. 기본은 requestAnimationFrame. */
  scheduler?: FrameScheduler;
  className?: string;
}

// --- 스타일 비교 ---------------------------------------------------------

/** `ResolvedStyle` 의 전 필드. 목표가 바뀌었는지 판정하는 축이다. */
const STYLE_KEYS = [
  'fill',
  'stroke',
  'strokeWidth',
  'opacity',
  'fontSize',
  'fontWeight',
  'textColor',
  'align',
  'visible',
  'text',
] as const;

/**
 * 두 스타일이 같은가(얕은 비교).
 *
 * 규칙 평가는 프레임마다 **새 객체**를 낸다(순수 함수라 그렇다). 그래서 참조 비교로는
 * 목표가 바뀌었는지 알 수 없고, 매 프레임 트윈을 다시 시작해 애니메이션이 영원히 처음
 * 상태에 머문다. 값 비교가 필요한 이유다.
 */
function sameStyle(a: ResolvedStyle, b: ResolvedStyle): boolean {
  return STYLE_KEYS.every((key) => a[key] === b[key]);
}

// --- 컴포넌트 -----------------------------------------------------------

export default function CanvasSurface({
  elements,
  targetStyles,
  texts,
  panelTween,
  background,
  visibilitySource = documentVisibility,
  scheduler = DEFAULT_SCHEDULER,
  className,
}: CanvasSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const ctxRef = useRef<DrawContext2D | null>(null);

  /** 표시 영역 CSS px 크기. ResizeObserver 가 갱신한다. */
  const [display, setDisplay] = useState<StageSize>({ width: 0, height: 0 });

  /**
   * 프레임 콜백은 예약된 시점의 클로저를 들고 늦게 실행된다. 그때 옛 props 를 보면
   * 데이터가 바뀐 프레임이 옛 그림을 그린다 — 그래서 최신 props 를 ref 한 곳에 모은다.
   */
  const latest = useRef({
    elements,
    targetStyles,
    texts,
    panelTween,
    background,
    display,
    scheduler,
  });

  /** 요소 id → 진행 중인 트윈. 요소가 사라지면 함께 지운다. */
  const tweensRef = useRef(new Map<string, TweenState<ResolvedStyle>>());
  /** 예약된 프레임 핸들. null 이면 예약 없음(= 유휴). */
  const frameRef = useRef<number | null>(null);
  /** 현재 가시성. 프레임 콜백이 동기적으로 읽어야 해서 state 가 아니라 ref 다. */
  const visibleRef = useRef(true);
  /** 프레임 콜백의 최신 구현. 예약 함수와 서로를 부르는 순환을 여기서 끊는다. */
  const frameHandlerRef = useRef<(nowMs: number) => void>(() => {});

  // props → ref 동기화. 이 훅이 가장 먼저 선언되어 있어야 아래 효과들이 최신값을 본다.
  useEffect(() => {
    latest.current = { elements, targetStyles, texts, panelTween, background, display, scheduler };
  });

  /**
   * 트윈 장부를 `nowMs` 로 전진시키고 이번 프레임에 그릴 스타일을 낸다.
   *
   * 세 갈래다.
   * - **첫 등장**: 트윈 없이 곧바로 목표에서 시작한다(`from === to`). 마운트가 전이처럼
   *   보이면 안 되고, 그런 트윈은 첫 샘플에서 끝나므로 루프도 깨우지 않는다.
   * - **목표 그대로**: 진행 중인 트윈을 그대로 이어 샘플한다.
   * - **목표 변경**: `retargetTween` 으로 **현재 보간 중인 값에서** 새 목표로 다시
   *   출발한다(AC-03 "값이 튀지 않는다"). 요소의 `tween` 이 패널 기본을 덮어쓴다.
   */
  const advance = useCallback(
    (nowMs: number): { styles: Record<string, ResolvedStyle>; allDone: boolean } => {
      const current = latest.current;
      const tweens = tweensRef.current;
      const styles: Record<string, ResolvedStyle> = {};
      const seen = new Set<string>();
      let allDone = true;

      for (const el of current.elements) {
        seen.add(el.id);
        const target = current.targetStyles[el.id] ?? el.style;
        const live = tweens.get(el.id);
        let state: TweenState<ResolvedStyle>;
        if (live === undefined) {
          state = beginTween(target, target, undefined, nowMs);
        } else if (sameStyle(live.to, target)) {
          state = live;
        } else {
          state = retargetTween(live, target, el.tween ?? current.panelTween, nowMs);
        }
        tweens.set(el.id, state);

        const sampled = sampleTween(state, nowMs);
        styles[el.id] = sampled.style;
        if (!sampled.done) allDone = false;
      }

      // 사라진 요소의 장부를 정리한다(Map 은 순회 중 삭제가 안전하다).
      for (const id of tweens.keys()) {
        if (!seen.has(id)) tweens.delete(id);
      }
      return { styles, allDone };
    },
    [],
  );

  /**
   * 한 프레임을 그린다. 돌려주는 값은 "더 그릴 것이 없는가"(= 루프를 멈춰도 되는가)다.
   *
   * 백킹 버퍼 축이 0 이면 **아무것도 하지 않고** 끝난 것으로 본다(장부도 전진시키지
   * 않는다) — 크기 0 은 "아직 그릴 수 없음"이지 "그릴 것이 없음"이 아니므로, 이 상태에서
   * 프레임을 계속 예약하면 보이지도 않는 그림을 위해 루프가 돈다. 크기가 잡히면
   * 리사이즈 효과가 다시 한 프레임을 요청한다.
   */
  const drawFrame = useCallback(
    (nowMs: number): boolean => {
      const canvas = canvasRef.current;
      if (canvas === null) return true;
      if (ctxRef.current === null) ctxRef.current = canvas.getContext('2d');
      const ctx = ctxRef.current;
      if (ctx === null) return true;

      const { display: size, background: bg, texts: labels, elements: els } = latest.current;
      const dpr =
        typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1;
      const backing = computeBackingSize(size.width, size.height, dpr);
      if (backing.width === 0 || backing.height === 0) return true;

      // 백킹 버퍼는 값이 바뀔 때만 쓴다 — canvas.width 대입은 표면을 지우는 부수효과가 있다.
      if (canvas.width !== backing.width) canvas.width = backing.width;
      if (canvas.height !== backing.height) canvas.height = backing.height;
      canvas.style.width = `${size.width}px`;
      canvas.style.height = `${size.height}px`;

      const { styles, allDone } = advance(nowMs);

      clearSurface(ctx, backing, bg);
      // 이후 그리기는 CSS px 좌표계에서 이뤄진다(정규화 좌표 × 표시 크기).
      ctx.setTransform(backing.scale, 0, 0, backing.scale, 0, 0);
      drawElements(ctx, els, styles, labels, size);
      return allDone;
    },
    [advance],
  );

  /** 프레임 한 장을 예약한다. 이미 예약돼 있거나 보이지 않으면 아무것도 하지 않는다. */
  const scheduleFrame = useCallback(() => {
    if (frameRef.current !== null || !visibleRef.current) return;
    frameRef.current = latest.current.scheduler.request((nowMs) => frameHandlerRef.current(nowMs));
  }, []);

  /** 예약된 프레임을 취소한다. */
  const cancelFrame = useCallback(() => {
    if (frameRef.current === null) return;
    latest.current.scheduler.cancel(frameRef.current);
    frameRef.current = null;
  }, []);

  // 프레임 콜백 본체. 유휴 정지 판정이 여기 있다(AC-E6).
  useEffect(() => {
    frameHandlerRef.current = (nowMs) => {
      frameRef.current = null;
      if (!drawFrame(nowMs)) scheduleFrame();
    };
  }, [drawFrame, scheduleFrame]);

  // 표시 영역 추적(AC-E5). HeatmapCanvas 의 ResizeObserver 규율을 그대로 따른다.
  useEffect(() => {
    const el = containerRef.current;
    if (el === null || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const width = Math.floor(entry.contentRect.width);
        const height = Math.floor(entry.contentRect.height);
        // 같은 크기면 상태를 갈지 않는다 — 새 객체를 넣으면 매 관찰마다 재렌더가 돈다.
        setDisplay((prev) => (prev.width === width && prev.height === height ? prev : { width, height }));
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // 가시성 게이팅(AC-E6). 보이지 않으면 예약을 끊고, 다시 보이면 한 프레임을 즉시 그린다.
  useEffect(() => {
    visibleRef.current = visibilitySource.isVisible();
    const unsubscribe = visibilitySource.subscribe(() => {
      const visible = visibilitySource.isVisible();
      visibleRef.current = visible;
      if (visible) scheduleFrame();
      else cancelFrame();
    });
    return unsubscribe;
  }, [visibilitySource, scheduleFrame, cancelFrame]);

  // 데이터·크기·배경이 바뀌면 한 프레임을 요청한다. 바뀌지 않으면 아무것도 그리지 않아
  // 마지막 프레임이 그대로 남는다(AC-E4).
  useEffect(() => {
    scheduleFrame();
  }, [elements, targetStyles, texts, panelTween, background, display, scheduleFrame]);

  // 언마운트 시 예약된 프레임 정리.
  useEffect(() => cancelFrame, [cancelFrame]);

  return (
    <div
      ref={containerRef}
      className={className === undefined ? 'relative min-h-0 w-full flex-1' : className}
    >
      <canvas ref={canvasRef} data-testid="canvas-surface" className="block h-full w-full" />
    </div>
  );
}
