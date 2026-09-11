// 캔버스 표면 컴포넌트 — <canvas> + rAF 루프 + DPR/ResizeObserver + 가시성 게이팅
// (SPEC-CANVAS-001 T10).
//
// 이 컴포넌트가 지는 책임은 셋이며, 셋 다 인수 기준에 묶여 있다.
//   1) 백킹 버퍼(AC-E5) — 표시 크기·devicePixelRatio 가 바뀌면 버퍼를 다시 잡고 다시 그린다.
//      캔버스 좌표를 새 크기로 재투영하므로 요소는 화면상 같은 상대 위치에 남는다.
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
// **SPEC-CANVAS-002 가 이 컴포넌트에 더한 것은 선택 prop 하나뿐이다**(T3): 캔버스 위에
// DOM 층을 얹는 `overlay` 렌더 prop. **넘기지 않으면 오버레이 때문에 생기는 것이 하나도
// 없다**(AC-E1) — 추가 렌더도, 추가 프레임도, 오버레이가 만드는 DOM 노드도 없다.
//
// **0.9.0 이 더한 넷째 책임: 그리는 영역을 격자 칸에 맞춘다**(사용 시험 "격자가 일정하지
// 않음" 의 세 번째 회차). 잰 상자를 그대로 투영에 쓰면 격자 한 칸이 소수 px 가 되고, 소수
// 자리에서 시작하는 1px 선은 두 픽셀에 걸쳐 칠해져 선마다 굵기가 달라 보인다(자세한 산술은
// `canvasGeometry` §그리는 영역의 격자 정렬). 그래서 잰 상자에서 한 칸 미만의 자투리를 뺀
// **안쪽 상자**를 짓고, 캔버스와 오버레이를 그 안에 함께 넣는다. 투영·붙임·격자가 모두 그
// 상자를 쓰므로 셋이 갈라질 자리가 없고, 오버레이가 제 상자로 재는 값과 `projection.stage`
// 가 **같은 노드**라 그 사이에 보정 산술이 낄 자리도 없다.
//
// **006 이 더한 다섯째 책임: 상자를 둘로 짓는다**(SPEC-CANVAS-006 M2). 0.9.0 의 안쪽 상자
// 하나가 저술 공간이자 출력 공간이었으므로, 캔버스 밖에 놓인 요소는 저장은 되지만 화면에서
// 사라졌다(비트맵이 제 상자 밖의 그리기를 버린다). 006 은 편집 중일 때 **작업 영역**
// (`canvas-workspace` — 잰 상자 전부)과 그 안의 **패널 출력 영역**(`canvas-stage` — 오늘의
// 그 상자)을 따로 짓고, `<canvas>` 를 바깥쪽인 작업 영역에 올린다. 그것이 이 SPEC 의 유일한
// 구조 변경이며, 출력 영역 밖의 그림이 비트맵 안으로 들어오는 이유 전부다.
//
// 지켜 낸 것이 결정적이다. **오버레이는 여전히 `canvas-stage` 안에 살고 `projection.stage`
// 는 여전히 그 상자의 크기다** — 오버레이가 제 `getBoundingClientRect()` 로 재는 상자와
// 투영이 쓰는 크기가 **같은 노드**라는 성질이 그대로이므로, 둘 사이에 보정 산술이 낄 자리가
// 006 이후에도 존재하지 않는다(위험 R1). 그래서 히트 테스트 · 핸들 좌표 · 정렬 · 붙임 ·
// 포인터 환산이 한 줄도 바뀌지 않았다.
//
// 비트맵의 원점 이동은 **이미 있던 한 줄**이 진다: `drawFrame` 은 DPR 배율을 세우려고
// `ctx.setTransform(scale,0,0,scale,0,0)` 을 이미 부르고 있었고, 006 은 그 마지막 두 인자
// (평행이동)에 출력 영역의 px 자리를 실을 뿐이다. `drawElement.ts` 도 `DrawContext2D` 도
// 한 글자도 바뀌지 않는다.
//
// **`workspace` 를 넘기지 않으면 002 와 동작이 완전히 같다**(AC-E1). 그때 `workspaceBox` 는
// `stageLattice` 결과를 그대로 옮겨 담아 작업 영역과 출력 영역이 겹치고 원점이 (0,0) 이므로
// 평행이동도 0 이다. 상자를 **조건부로 없애지 않는** 것에 뜻이 있다 — 갈래가 둘이면 편집
// 토글마다 DOM 이 갈아엎히고, 그때 `<canvas>` 가 다시 만들어져 마지막 프레임이 사라진다.
//
// **포인터 통과 슬롯은 두지 않는다.** T3 은 설계를 그대로 옮겨 `onCanvasPointer*` 넷을
// `<canvas>` 에 달아 두었으나, T5 가 오버레이를 지으면서 포인터를 **오버레이 루트**에서
// 받기로 결론이 났다 — 초점을 받는 핸들(T8)과 팔레트(T9)가 어차피 그 층 안의 진짜 DOM
// 요소여서 좌표 기준이 하나로 유지되고, 칠해진 픽셀인 캔버스에는 잡을 노드가 없다. 그래서
// 넷은 **호출부가 하나도 없는 API** 로 남았고, 부르는 곳이 없는 통과 슬롯은 다음 사람에게
// "여기로도 포인터가 들어온다" 고 거짓말한다. SPEC-CANVAS-004 가 이 파일을 다시 고칠
// 예정이므로(위험 R4) 그 거짓말을 물려주지 않는다. AC-E3("빈 지점 누름을 소비하지
// 않는다")은 약해지지 않는다 — 그 인수 기준이 요구하는 것은 **어느 노드가 받는가**가
// 아니라 **소비하지 않는다**이며, 그 책임은 오버레이가 진다.
//
// 002 가 편집기를 캔버스에 칠하지 않고 DOM 오버레이로 둔 결정적 이유가 위 3번(루프 규율)
// 이다. 선택·호버·핸들을 rAF 루프 안에 칠하면 편집기 상태가 001 이 유휴로 만들려고 지은
// 그 루프 안으로 들어와, "트윈이 없으면 프레임을 예약하지 않는다" 에 예외가 생긴다.
// 그래서 이 파일에서 **프레임을 예약하는 경로는 001 과 똑같이 셋뿐이다**: (1) 데이터·크기·
// 배경 props 변경, (2) 다시 보이게 됨, (3) 진행 중인 트윈의 다음 장. 새 prop 은 어느
// 효과의 의존성에도 들어가지 않고, 실측 폭 장부도 state 가 아니라 ref 라 재렌더를 낳지
// 않는다(REQ-05 · AC-E4).
//
// @spec SPEC-CANVAS-001 · SPEC-CANVAS-002 (T3 — 오버레이 슬롯) · SPEC-CANVAS-006 (M2 — 두 상자)

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { documentVisibility, type VisibilitySource } from '../charts/visiblePolling';
import type { CanvasSize, TweenSpec } from './canvasConfig';
import { CANVAS_GRID_STEP_UNITS } from './canvasEditArrange';
import {
  computeBackingSize,
  type CanvasProjection,
  type StageSize,
} from './canvasGeometry';
import type { ResolvedStyle } from './canvasRules';
import { CanvasStageGridContext, type CanvasStageGrid } from './canvasStageGrid';
import { beginTween, retargetTween, sampleTween, type TweenState } from './canvasTween';
import { DEFAULT_WORKSPACE_ZOOM, workspaceBox } from './canvasWorkspace';
import { clearSurface, drawElements, type DrawContext2D } from './drawElement';
import { walkDrawables } from './group/frameKey';
import type { CanvasNode } from './group/groupTypes';

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

// --- 오버레이 슬롯 (SPEC-CANVAS-002 T3) ---------------------------------

/**
 * 오버레이 렌더 prop 이 받는 것. **표면이 이미 잰 값들뿐이며 오버레이가 다시 잴 것은 없다.**
 *
 * 이 형상이 SPEC-CANVAS-002 위험 R1("핸들이 도형에서 미끄러진다")의 대책 그 자체다.
 * 스테이지 크기의 두 번째 측정원이 생기거나 투영이 두 벌이 되면 핸들과 도형이 어긋나고,
 * 그 어긋남은 "가끔 어긋난다" 로만 보고되어 원인을 찾기 어렵다. 그래서 오버레이는 크기를
 * **스스로 재지 않고** 이 값을 받아 쓰며, 좌표 투영도 `canvasGeometry` 의 같은 함수를 쓴다.
 */
export interface CanvasOverlayContext {
  /**
   * 프레임이 투영에 쓰는 **바로 그 한 벌** — 표면이 그리는 영역의 CSS px 크기와 config 가
   * 정한 캔버스 단위 크기다(측정원이 하나다 — AC-E2).
   *
   * `stage` 는 `ResizeObserver` 가 잰 값을 격자 칸에 맞춘 값이다(`stageLattice`). 그 값은
   * 표면이 **실제로 지은 상자**의 크기이기도 하다 — 캔버스와 이 오버레이가 그 상자 안에
   * 함께 살므로, 오버레이가 제 `getBoundingClientRect()` 로 재는 상자와 여기 실린 크기가
   * 같은 상자를 가리킨다(축척 보정 말고는 사이에 낄 산술이 없다 — 위험 R1).
   *
   * 둘을 묶어 넘기는 것에 뜻이 있다. 정수 좌표계에서 투영은 두 크기를 **모두** 요구하므로,
   * 하나만 넘기면 받는 쪽이 나머지 하나를 스스로 구하게 되고 그 자리가 곧 두 번째 출처다.
   */
  projection: CanvasProjection;
  /**
   * **직전에 그린 프레임**이 잰 글자 폭(`kind:'text'` 요소 id → CSS px). `drawElements` 가
   * 프레임마다 이미 재던 값을 그대로 흘려보낸 것이라 두 번째 측정원이 아니다.
   *
   * 최악의 지연은 한 프레임이며, 그 지연이 틀리게 할 수 있는 것은 **텍스트 상자의 폭
   * 하나뿐**이다(도형은 투영만으로 정해진다). 아직 한 프레임도 그리지 않았으면 비어 있고,
   * 받는 쪽은 기준점 둘레 여유 상자로 폴백한다(AC-E7).
   */
  textWidths: Record<string, number>;
}

export interface CanvasSurfaceProps {
  /**
   * 배열 순서 = 그리기 순서(뒤가 위).
   *
   * SPEC-CANVAS-004 M3 — 원소 타입이 `CanvasNode` 로 넓어졌다. 그룹은 그릴 도형이 없고
   * 제 **부품**이 그려지므로, 이 층이 드는 트윈 장부도 부품에 대해서는 복합 키를 쓴다.
   * 최상위 원소의 키는 002 와 **바이트 동일**하다(불변식 G11).
   */
  elements: CanvasNode[];
  /**
   * 캔버스 좌표계의 크기(정수 단위). 요소 기하가 이 단위로 적혀 있으므로, 투영은 축마다
   * `스테이지 px / 이 값` 을 곱한다. 값이 바뀌면 같은 요소가 다른 자리에 그려지므로
   * 프레임을 예약하는 props 축이다(아래 효과의 의존성 목록).
   */
  canvas: CanvasSize;
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
  /**
   * 캔버스 **위에** 얹을 DOM 층(SPEC-CANVAS-002 T3). 선택 외곽선·핸들·팔레트가 여기 산다.
   *
   * 미지정이면 아무것도 렌더하지 않는다 — 추가 DOM 노드조차 생기지 않는다(AC-E1).
   * 캔버스에 칠하지 않으므로 이 층이 무엇을 그리든 rAF 루프를 깨우지 않는다(REQ-05).
   */
  overlay?: (ctx: CanvasOverlayContext) => ReactNode;
  /**
   * 잰 **바깥** 상자를 알린다(0.10.0). 크기가 실제로 달라진 뒤 효과에서만 부르며, 렌더
   * 중에는 부르지 않는다 — 렌더 단계에서 남의 상태를 갈면 React 가 그 렌더를 버린다.
   *
   * 알리는 것이 `stage`(맞춘 영역)가 아니라 **바깥** 상자인 것에 뜻이 있다. 맞춘 영역은
   * 이미 캔버스 비율이므로 그것으로는 "캔버스 비율과 패널 비율이 얼마나 다른가" 를 알 수
   * 없다 — 여백을 없애려는 쪽(`canvasStageAspect`)이 봐야 하는 것은 여백을 만든 그 상자다.
   *
   * 미지정이면 이 효과는 아무것도 하지 않는다. 프레임을 예약하지 않으므로 유휴 정지에
   * 영향이 없다(REQ-05 · AC-E4).
   */
  onStageMeasured?: (outer: StageSize) => void;
  /**
   * 편집 중인가 — 캔버스 비트맵이 패널 출력 영역보다 **넓은 작업 영역**을 덮는가
   * (SPEC-CANVAS-006 M2).
   *
   * **기본값 `false` 는 002 와 완전히 동일한 동작이다**(AC-E1). 그때 작업 영역과 출력
   * 영역이 겹치고 캔버스 좌표 원점이 (0,0) 이라 `setTransform` 의 평행이동 두 인자가 0 이며,
   * 그리는 결과 · 프레임 예약 횟수 · 트윈 동작이 종전과 같다.
   *
   * 작업 영역 전용 토글을 만들지 않고 편집 게이팅에 얹는 것에 뜻이 있다 — 세 겹 게이팅이
   * 이미 "지금 편집 중인가" 를 소유하고, 넷째 개념이 생기면 사용자가 패널마다 다른 규칙을
   * 배운다. 이 값이 바뀌면 **그리는 상자가 실제로 달라지므로** 프레임이 한 장 필요한데,
   * 그것은 새 깨우기 경로가 아니라 종전의 props 변경 경로 그대로다(REQ-05 · AC-E4).
   */
  workspace?: boolean;
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
  canvas,
  targetStyles,
  texts,
  panelTween,
  background,
  visibilitySource = documentVisibility,
  scheduler = DEFAULT_SCHEDULER,
  className,
  overlay,
  onStageMeasured,
  workspace = false,
}: CanvasSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const ctxRef = useRef<DrawContext2D | null>(null);

  /** **바깥** 상자의 CSS px 크기. ResizeObserver 가 갱신한다(격자에 맞추기 전 값이다). */
  const [outer, setOuter] = useState<StageSize>({ width: 0, height: 0 });

  /**
   * 그리는 영역을 맞출 격자 간격(정수 캔버스 단위).
   *
   * 상태의 주인이 표면인 근거는 `canvasStageGrid.ts` §왜 상태의 주인이 표면인가에 있다.
   * 기본값은 격자 어휘의 주인(`canvasEditArrange`)이 가진 그 값이다 — 여기서 25 를 다시
   * 적으면 같은 뜻의 수가 두 곳에 살게 되고, 한쪽만 바뀌는 날 화면과 붙임이 갈라진다.
   * 편집 도크에서 간격을 고르면 오버레이가 위 컨텍스트로 이 상태를 갈고, 그리는 영역이
   * 새 칸에 다시 맞춰진다(크기가 실제로 달라지므로 그때는 프레임이 한 장 필요하다 —
   * 리사이즈와 같은 부류이며, 선택·호버·격자 토글은 여전히 0 건이다).
   */
  const [gridStep, setGridStep] = useState<number>(CANVAS_GRID_STEP_UNITS);

  /**
   * 보기 배율(분수) — 편집기가 작업 영역을 보여 주는 축척(SPEC-CANVAS-006 M9 · REQ-09).
   *
   * 주인이 표면인 근거는 위 `gridStep` 과 **한 글자도 다르지 않다**: 이 값이 그리는 상자를
   * 정하고 그 상자는 표면의 것이다. 저장하지 않는 표시 상태라는 것도 같다(가정 A21) —
   * 저장하면 config 스키마가 넓어지고, 한 사람의 시야 취향이 그 대시보드를 보는 모든
   * 사람에게 실려 간다. 범위는 **표면 하나·마운트 하나**이므로 두 캔버스 패널이 서로 다른
   * 배율을 가질 수 있고, 다이얼로그를 닫으면 기본값으로 돌아간다.
   *
   * 배율을 바꾸면 그리는 상자가 실제로 달라지므로 프레임이 **한 장** 필요하다 — 아래
   * `geometry` 의 의존성을 지나는 **종전의 props 변경 경로 그대로**이며(격자 간격 변경과
   * 같은 부류) 새 깨우기 경로가 아니다(REQ-05 · AC-E4).
   */
  const [zoom, setZoom] = useState<number>(DEFAULT_WORKSPACE_ZOOM);

  /**
   * 바깥 상자에서 파생한 상자 한 벌(`canvasWorkspace.workspaceBox`). **투영·상자·격자가
   * 함께 보는 단 한 벌**이다.
   *
   * 의존성을 객체가 아니라 다섯 수치로 적는 것에 뜻이 있다 — props 로 온 객체는 부모가
   * 렌더할 때마다 새 신원일 수 있고, 그 신원이 여기 들어오면 렌더마다 새 상자가 나와
   * 아래 예약 효과가 프레임을 계속 깨운다(AC-E4 가 금지한 바로 그것이다).
   */
  const geometry = useMemo(
    () =>
      workspaceBox(
        { width: outer.width, height: outer.height },
        { width: canvas.width, height: canvas.height },
        gridStep,
        workspace,
        zoom,
      ),
    [outer.width, outer.height, canvas.width, canvas.height, gridStep, workspace, zoom],
  );
  /**
   * **패널 출력 영역**. 이 아래에서 "스테이지" 는 언제나 이 값이며, 그 뜻은 006 전후로
   * 한 글자도 바뀌지 않았다 — 대시보드에서 실제로 보이게 될 그 사각형이다.
   */
  const stage = geometry.stage;

  /**
   * 오버레이 슬롯 둘레에 펴는 격자 한 벌(`canvasStageGrid.ts`). 값이 같으면 신원도 같아야
   * 헛 렌더가 없다.
   *
   * 006 이 `origin` 과 `box` 를 함께 싣는다. 격자는 **작업 영역 전체**(`box`)에 그려지되
   * 선은 **출력 영역의 원점**(`origin`)에서 시작해야 하는데(REQ-04 · 위험 R3), 둘 다 상자를
   * 지으며 이미 손에 든 값이다. 내려보내지 않으면 오버레이가 같은 상자를 **다시 파생**하게
   * 되고 그것이 곧 두 번째 측정원이다(위험 R1 · 불변식 I10). 이 컨텍스트가 존재하는 이유
   * ("상자를 짓는 쪽이 필요한 것을 아래로 내린다")에 그대로 들어맞는 값 둘이다.
   */
  const stageGrid = useMemo<CanvasStageGrid>(
    () => ({
      step: gridStep,
      setStep: setGridStep,
      zoom,
      setZoom,
      cell: geometry.cell,
      origin: geometry.origin,
      box: geometry.box,
    }),
    [gridStep, zoom, geometry],
  );

  /**
   * 프레임 콜백은 예약된 시점의 클로저를 들고 늦게 실행된다. 그때 옛 props 를 보면
   * 데이터가 바뀐 프레임이 옛 그림을 그린다 — 그래서 최신 props 를 ref 한 곳에 모은다.
   */
  const latest = useRef({
    elements,
    canvas,
    targetStyles,
    texts,
    panelTween,
    background,
    stage,
    box: geometry.box,
    origin: geometry.origin,
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
  /**
   * 직전 프레임이 잰 글자 폭 장부(SPEC-CANVAS-002 T3).
   *
   * **state 가 아니라 ref 인 것이 핵심이다.** state 로 두면 프레임마다 재렌더가 돌고, 그
   * 재렌더가 다시 프레임을 예약해 001 이 금지 조항으로 못박은 유휴 정지가 무너진다
   * (REQ-05 · AC-E4). ref 는 그리기의 부산물을 오버레이 쪽으로 흘려보내기만 하고 루프를
   * 건드리지 않는다. 그 대가로 오버레이가 보는 폭은 **직전 프레임**의 값이지만, 그것이
   * 명세가 허용한 정확한 지연이다(최악 한 프레임, 틀릴 수 있는 것은 텍스트 상자의 폭뿐).
   */
  const textWidthsRef = useRef<Record<string, number>>({});

  // props → ref 동기화. 이 훅이 가장 먼저 선언되어 있어야 아래 효과들이 최신값을 본다.
  useEffect(() => {
    latest.current = {
      elements,
      canvas,
      targetStyles,
      texts,
      panelTween,
      background,
      stage,
      box: geometry.box,
      origin: geometry.origin,
      scheduler,
    };
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

      // **그리는 쪽과 같은 순회**를 쓴다(`walkDrawables`). 순회가 둘이 되면 키 집합이
      // 갈라지고, 그 어긋남은 "어떤 부품만 트윈되지 않는다" 로만 보인다.
      for (const { key, element: el } of walkDrawables(current.elements)) {
        seen.add(key);
        const target = current.targetStyles[key] ?? el.style;
        const live = tweens.get(key);
        let state: TweenState<ResolvedStyle>;
        if (live === undefined) {
          state = beginTween(target, target, undefined, nowMs);
        } else if (sameStyle(live.to, target)) {
          state = live;
        } else {
          state = retargetTween(live, target, el.tween ?? current.panelTween, nowMs);
        }
        tweens.set(key, state);

        const sampled = sampleTween(state, nowMs);
        styles[key] = sampled.style;
        if (!sampled.done) allDone = false;
      }

      // 사라진 요소의 장부를 정리한다(Map 은 순회 중 삭제가 안전하다). 그룹이 삭제되거나
      // **해제되면** 그 부품의 복합 키도 여기서 함께 사라진다 — 키가 `그룹id/부품id` 라
      // 그룹이 없어지는 순간 어느 순회도 그 키를 내지 않기 때문이다.
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
      // 지역 이름이 `canvas` 가 아닌 것에 뜻이 있다 — 이 컴포넌트에는 같은 이름의 prop
      // (캔버스 **좌표계 크기**)이 있고, 둘이 가려지면 어느 쪽을 쓰는지 읽어서 알 수 없다.
      const surface = canvasRef.current;
      if (surface === null) return true;
      if (ctxRef.current === null) ctxRef.current = surface.getContext('2d');
      const ctx = ctxRef.current;
      if (ctx === null) return true;

      const {
        stage: size,
        box: area,
        origin,
        background: bg,
        texts: labels,
        elements: els,
        canvas: units,
      } = latest.current;
      const dpr =
        typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1;
      // 비트맵이 덮는 것은 **작업 영역**이다(006). 편집이 꺼져 있으면 작업 영역과 출력
      // 영역이 겹치므로 이 값은 종전과 같다. 메모리 비용도 사실상 그대로다 — 늘어나는
      // 것은 0.9.0 이 여백으로 내주던 자투리 몫뿐이다.
      const backing = computeBackingSize(area.width, area.height, dpr);
      if (backing.width === 0 || backing.height === 0) return true;

      // 백킹 버퍼는 값이 바뀔 때만 쓴다 — canvas.width 대입은 표면을 지우는 부수효과가 있다.
      if (surface.width !== backing.width) surface.width = backing.width;
      if (surface.height !== backing.height) surface.height = backing.height;
      surface.style.width = `${area.width}px`;
      surface.style.height = `${area.height}px`;

      const { styles, allDone } = advance(nowMs);

      clearSurface(ctx, backing, bg);
      // 이후 그리기는 CSS px 좌표계에서 이뤄진다(캔버스 좌표 ÷ 캔버스 크기 × 표시 크기).
      //
      // 마지막 두 인자가 평행이동이며, 006 은 거기에 **출력 영역의 자리**를 싣는다. 그래서
      // 캔버스 좌표 (0,0) 이 작업 영역 안의 `origin` 에 떨어지고, 음수 좌표의 요소가 비트맵
      // 안으로 들어와 실제로 그려진다. `drawElements` 에는 오늘 그대로 `{ stage, canvas }` 를
      // 넘기므로 `drawElement.ts` 와 `DrawContext2D` 는 한 글자도 바뀌지 않는다.
      ctx.setTransform(
        backing.scale,
        0,
        0,
        backing.scale,
        origin.x * backing.scale,
        origin.y * backing.scale,
      );
      // 반환값은 이 프레임이 잰 글자 폭이다 — 재는 곳이 늘어난 것이 아니라, 원래 재던
      // 값을 오버레이 쪽으로 흘려보낼 뿐이다(측정은 여전히 프레임당 1회).
      textWidthsRef.current = drawElements(ctx, els, styles, labels, { stage: size, canvas: units });
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
  //
  // 재는 것은 **바깥** 상자다. 안쪽 상자(그리는 영역)는 이 값에서 파생되므로 관찰하면
  // 제 꼬리를 물게 된다 — 파생된 크기를 다시 재어 다시 파생시키는 고리가 된다.
  useEffect(() => {
    const el = containerRef.current;
    if (el === null || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const width = Math.floor(entry.contentRect.width);
        const height = Math.floor(entry.contentRect.height);
        // 같은 크기면 상태를 갈지 않는다 — 새 객체를 넣으면 매 관찰마다 재렌더가 돈다.
        setOuter((prev) => (prev.width === width && prev.height === height ? prev : { width, height }));
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
  //
  // 크기 축은 잰 값이 아니라 **파생한 상자 한 벌**(`geometry`)이다. 그래서 바깥 상자가 1px
  // 흔들려도 그리는 영역이 같으면 프레임이 돌지 않는다 — 그릴 것이 정말로 달라졌을 때만
  // 깨운다. 006 이 그 자리를 `lattice` 에서 `geometry` 로 갈아 끼웠을 뿐 경로는 그대로이며,
  // 편집 토글이 프레임을 한 장 부르는 것도 이 **종전의 props 변경 경로**다(AC-E4).
  useEffect(() => {
    scheduleFrame();
  }, [elements, canvas, targetStyles, texts, panelTween, background, geometry, scheduleFrame]);

  // 잰 바깥 상자를 밖으로 알린다(0.10.0). **프레임을 예약하지 않는다** — 이 효과가 하는
  // 일은 통보 하나뿐이고, 어느 그리기 경로의 의존성에도 들어가지 않는다(AC-E4).
  //
  // 알리는 축이 `outer` 인 것은 위 prop 주석의 이유 그대로다. 크기가 실제로 달라졌을 때만
  // 도는데, `setOuter` 가 같은 값이면 상태를 갈지 않기 때문이다(위 ResizeObserver 효과).
  useEffect(() => {
    onStageMeasured?.({ width: outer.width, height: outer.height });
  }, [onStageMeasured, outer.width, outer.height]);

  // 언마운트 시 예약된 프레임 정리.
  useEffect(() => cancelFrame, [cancelFrame]);

  return (
    <div
      ref={containerRef}
      // `overflow-hidden` 은 006 이 더한다. 그러면 출력 영역 밖 요소의 **DOM 손잡이**가
      // 비트맵의 클리핑과 **같은 자리**에서 잘린다. 편집이 꺼져 있으면 선택도 손잡이도 없고
      // 작업 영역이 잰 상자를 넘지도 않으므로 이 한 줄로 달라지는 것은 없다.
      className={
        className === undefined ? 'relative min-h-0 w-full flex-1 overflow-hidden' : className
      }
    >
      {/*
        **상자 둘 다 진짜 DOM 상자다**(0.9.0 이 세우고 006 이 하나를 더했다). 자투리든
        저술 여백이든 산술로만 다루는 길(한 상자가 바깥을 덮고 좌표에 오프셋을 더하는 길)도
        있었지만 그것은 축척 결함(AC-E9)과 같은 부류의 함정이다 — 오버레이는 포인터를 제
        `getBoundingClientRect()` 로 받는데, 그 상자가 스테이지와 다른 상자가 되는 순간 모든
        좌표에 조용한 오프셋이 실린다. 상자를 실제로 지어 두면 그 오프셋이 **존재할 수 없다**.

        바깥쪽 `canvas-workspace` 는 **작업 영역**이다. 편집 중에는 잰 상자 전부이고, 편집이
        꺼져 있으면 0.9.0 의 그 안쪽 상자와 크기·자리가 같다(그때 아래 `canvas-stage` 가
        (0,0) 에 겹쳐 오늘과 같은 그림이 된다). 두 자리 모두 **정수** px 다 — 소수 자리에
        두면 안쪽의 모든 선이 다시 소수에서 시작해 이 변경이 하려던 일이 통째로 무산된다.
      */}
      <div
        data-testid="canvas-workspace"
        className="absolute"
        style={{
          left: geometry.offset.x,
          top: geometry.offset.y,
          width: geometry.box.width,
          height: geometry.box.height,
        }}
      >
        <canvas
          ref={canvasRef}
          data-testid="canvas-surface"
          // 캔버스는 **작업 영역**을 덮는다(006). 그래야 출력 영역 밖의 그림이 비트맵 안에
          // 들어와 실제로 그려진다.
          //
          // 포인터 리스너를 달지 않는다 — 표면은 포인터로 아무것도 하지 않으며, 편집
          // 포인터는 아래 오버레이 층의 루트가 받는다(머리말 §포인터 통과 슬롯).
          className="block h-full w-full"
        />
        {/*
          안쪽 `canvas-stage` 는 **패널 출력 영역**이며 0.9.0 의 그 상자 그대로다. 오버레이는
          여전히 이 안에 `absolute inset-0` 로 살고 `projection.stage` 는 여전히 이 상자의
          크기이므로, 둘이 **같은 노드**라는 성질이 006 이후에도 그대로다(위험 R1).

          미지정이면 옵셔널 호출이 인자 평가조차 건너뛰고 `undefined` 를 렌더하므로 오버레이
          때문에 생기는 DOM 노드는 하나도 없다(AC-E1).

          투영 한 벌은 프레임이 쓰는 그 값들을 그대로 넘긴다 — 오버레이용 두 번째 측정을
          만들지 않기 위해서다(AC-E2). 폭은 직전 프레임의 장부다. 격자 한 벌은 props 가
          아니라 컨텍스트로 가는데, 사이에 있는 `CanvasPanel` 이 슬롯의 값 가운데 둘만
          골라 넘기기 때문이다(`canvasStageGrid.ts` §왜 컨텍스트인가).
        */}
        <div
          data-testid="canvas-stage"
          className="absolute"
          style={{
            left: geometry.origin.x,
            top: geometry.origin.y,
            width: stage.width,
            height: stage.height,
          }}
        >
          <CanvasStageGridContext value={stageGrid}>
            {overlay?.({
              projection: { stage, canvas },
              textWidths: textWidthsRef.current,
            })}
          </CanvasStageGridContext>
        </div>
      </div>
    </div>
  );
}
