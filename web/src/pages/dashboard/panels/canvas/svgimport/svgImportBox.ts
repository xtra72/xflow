// 요소 상자 — 도형마다 제 기하가 차지하는 최소 영역 (SPEC-CANVAS-007 · 결함 D3 정정).
//
// **007 이 배달한 것은 상자 하나를 온 그림이 함께 쓰는 것이었다.** 그 결정의 근거는 정수
// 반올림이었다 — 도형마다 상자를 주면 서로 최대 0.5 단위 어긋난다는 것. 근거 자체는 참이지만
// 대가가 그보다 크다는 것이 쓰고 나서 드러났다.
//
//   - 큰 문서 구석의 작은 별 하나를 고르면 **여덟 손잡이가 문서 가장자리에 선다** — 제
//     잉크에서 수백 단위 떨어진 자리다. I23("그려지는 자리에 손잡이가 닿는다")의 정확한
//     반대이며, 007 이 스스로 인용한 그 원리를 놓인 요소가 어긴다.
//   - 선택 윤곽이 그 도형이 아니라 **가져온 것 전부**를 두른다.
//   - **정렬이 아무 일도 하지 않는다.** `alignDeltas` 는 요소 상자로 맞추는데 상자가 전부
//     같으므로 델타가 전부 0 이다 — 가져온 요소들끼리는 정렬 단추가 죽은 단추다.
//
// 0.5 단위의 어긋남은 기본 캔버스(500×400)에서 0.1% 이고 화면에서 보이지 않는다. 손잡이가
// 그림 밖에 서는 것과 정렬이 죽는 것은 보인다. **그래서 뒤집는다.**
//
// **상자만 좁히지 않는다 — 명령을 제 상자의 로컬 격자로 다시 정규화한다.** 상자만 좁히면
// 좌표는 여전히 문서 격자를 가리키므로 그림이 상자 밖으로 통째로 튀어 나간다. 이 모듈이
// 내놓는 `frame` 이 그 재정규화의 기준이고, **상자와 같은 수에서 나온다** — 둘을 따로
// 계산하면 어느 날 한쪽만 고쳐져 "상자는 여기, 그림은 저기" 가 된다.
//
// --- 결정 1 — 상자는 **기하**의 바운딩 박스이지 잉크의 것이 아니다 ------------------
//
// 선 두께의 절반만큼 잉크가 윤곽 밖으로 번진다. 그만큼 상자를 넓히는 안을 기각한다:
// `strokeWidth` 는 이 패널에서 **투영을 지나지 않는 원시 px** 이므로(실측 — `drawElement` 의
// `ctx.lineWidth = resolveStrokeWidth(style.strokeWidth)` 에 축척이 곱해지지 않는다), 상자를
// 잉크까지 넓히면 "손잡이가 잉크를 두른다" 는 약속이 **축척 1 에서만** 참이 되고 다른 모든
// 축척에서 거짓이 된다. 지킬 수 없는 약속을 상자에 새기지 않는다. 게다가 008 이 저술한
// 경로는 상자가 기하를 두르고 잉크가 그 밖으로 나가며(SPEC §로컬 정규화 "clamp 하지
// 않는다"), 가져온 경로만 다르게 하면 같은 종류의 요소에 규칙이 둘이 된다.
//
// **그래서 손잡이는 기하를 두른다** — 두꺼운 선의 잉크는 상자 밖으로 `strokeWidth/2` 만큼
// 나간다. 008 의 경로와 **똑같이** 나간다.
//
// --- 결정 3 — 곡선의 극값을 푼다. 제어점 볼록 껍질로 재지 않는다 -------------------
//
// `svgImportSplit.commandBounds` 는 제어점까지 세는 **상위집합**이고, 그 방향이 옳은 자리가
// 둘 있다(무리 포함 판정 · `viewBox` 폴백 — 크게 묶는 쪽의 틀림만 화면이 말한다). 요소
// 상자는 정반대다: 상위집합은 곧 "손잡이가 잉크에서 떨어진다" 이며 그것이 지금 고치는 결함
// 자체다. 얼마나 떨어지는가는 작지 않다 — `M 0 0 C 100 0 100 100 0 100` 의 껍질은 x 를
// 100 까지 세지만 곡선의 실제 최대는 `t = 1/2` 의 **75** 다(33% 헐거운 상자).
//
// 그래서 도함수의 이차식 `a t² + b t + c = 0` 을 풀어 `(0, 1)` 안의 근에서 곡선을 평가한다.
// 표본으로 훑는 안을 기각한다 — 최대를 성기게 훑으면 **언제나 낙관적으로** 읽혀(참 최대는
// 표본 사이에 있다) 상자가 잉크보다 작아지고, 그 틀림은 "손잡이가 잉크 안쪽에 선다" 로
// 조용히 남는다. 근은 축마다 많아야 둘이고 산술은 닫혀 있다.
//
// **그리고 `commandBounds` 는 한 글자도 고치지 않는다.** 두 쓰임이 요구하는 안전한 방향이
// 서로 반대이므로 함수도 둘이어야 한다.
//
// @spec SPEC-CANVAS-007 REQ-06 · AC-05 · AC-06 · AC-E8

import { coordinate, MIN_ELEMENT_EXTENT, type BoxGeometry } from '../canvasConfig';
import type { PathCommand } from '../shapes/pathTypes';

import type { ViewBox } from './svgDocument';
import type { Bounds } from './svgImportSplit';

// --- 3차 베지어의 극값 ---------------------------------------------------

/** `B(t)` 한 축. */
function cubicAt(t: number, p0: number, p1: number, p2: number, p3: number): number {
  const u = 1 - t;
  return u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3;
}

/**
 * `B'(t) = 0` 의 근 중 **구간 안**의 것들. 곡선의 끝점은 여기 들지 않는다 — 끝점은 명령
 * 자체가 나르므로 호출부가 이미 세었고, 여기서 다시 세면 두 곳이 같은 점을 책임진다.
 *
 * ```
 * B'(t)/3 = a t² + b t + c,  a = −P0 + 3P1 − 3P2 + P3
 *                            b = 2(P0 − 2P1 + P2)
 *                            c = P1 − P0
 * ```
 *
 * **`a === 0` 을 따로 가른다.** 세 점이 고르게 놓인 곡선(도구가 직선을 3차로 낸 흔한 형태)
 * 에서 이차항이 사라지고, 그때 `(−b ± √D)/2a` 는 0/0 이라 근을 통째로 잃는다. 그 갈래는
 * 시험이 실제로 물었다(뮤테이션 2).
 *
 * **가드는 `inside` 하나다.** 0 으로 나눈 `±∞`, `√(음수)` 의 `NaN`, 구간 밖의 근이 **같은
 * 비교 하나로** 함께 걸리므로 `b === 0` 도 `disc >= 0` 도 따로 막지 않는다. 막아 보았고
 * 어느 시험도 물지 않았다 — 물지 않는 가드는 다음 사람에게 "여기로 올 수도 있다" 고
 * 거짓말하므로 지운다(이 저장소가 `svgPathData` 에서 이미 적어 둔 규율이다). 끝점
 * `t = 0`·`t = 1` 도 이 비교에서 빠진다.
 */
function cubicExtremaTimes(p0: number, p1: number, p2: number, p3: number): number[] {
  const a = -p0 + 3 * p1 - 3 * p2 + p3;
  const b = 2 * (p0 - 2 * p1 + p2);
  const c = p1 - p0;
  const inside = (t: number): boolean => t > 0 && t < 1;
  if (a === 0) return [-c / b].filter(inside);
  const root = Math.sqrt(b * b - 4 * a * c);
  return [(-b + root) / (2 * a), (-b - root) / (2 * a)].filter(inside);
}

/**
 * 명령 목록의 **참** 바운딩 박스 — 곡선의 극값을 풀어 잰다.
 *
 * `commandBounds` 와 형상은 같고 답이 다르다(상위집합이 아니라 참값). 위 머리말 §결정 3 이
 * 둘을 왜 가르는지 적는다.
 *
 * 비유한 좌표는 `commandBounds` 와 **같은 규율로** 건너뛴다 — 좌표 하나가 망가졌다고 도형
 * 전체의 상자를 잃지 않는다. 전부 망가졌으면 `undefined` 이고, 그때 무엇을 쓸지는 계획
 * 층이 정한다.
 */
export function tightCommandBounds(commands: readonly PathCommand[]): Bounds | undefined {
  let bounds: Bounds | undefined;
  const include = (x: number, y: number): void => {
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    if (bounds === undefined) {
      bounds = { minX: x, minY: y, maxX: x, maxY: y };
      return;
    }
    bounds.minX = Math.min(bounds.minX, x);
    bounds.minY = Math.min(bounds.minY, y);
    bounds.maxX = Math.max(bounds.maxX, x);
    bounds.maxY = Math.max(bounds.maxY, y);
  };

  let cursor: { x: number; y: number } | undefined;
  let subPathStart: { x: number; y: number } | undefined;
  for (const cmd of commands) {
    switch (cmd.c) {
      case 'Z':
        // 닫는 선은 새 점을 만들지 않는다(끝이 이미 센 시작점이다). 다만 **현재 점은
        // 시작점으로 돌아간다** — `Z` 뒤에 이어지는 곡선의 `P0` 가 그것이며, 그 사실을
        // 놓치면 그 곡선의 극값을 엉뚱한 `P0` 로 푼다.
        cursor = subPathStart;
        break;
      case 'M':
        include(cmd.x, cmd.y);
        cursor = { x: cmd.x, y: cmd.y };
        subPathStart = cursor;
        break;
      case 'L':
        include(cmd.x, cmd.y);
        cursor = { x: cmd.x, y: cmd.y };
        break;
      case 'C': {
        include(cmd.x, cmd.y);
        if (cursor === undefined) {
          // `M` 없이 시작한 목록 — `P0` 가 없어 극값을 풀 자리가 없다. 제어점까지 세는
          // 상위집합으로 떨어진다(`commandBounds` 와 같은 답). 문서 층을 지나온 목록은
          // 언제나 `M` 으로 열리므로 이 가지에 닿는 길은 이 함수를 직접 부르는 것뿐이고,
          // 시험이 그렇게 부른다.
          include(cmd.x1, cmd.y1);
          include(cmd.x2, cmd.y2);
        } else {
          const from = cursor;
          const times = [
            ...cubicExtremaTimes(from.x, cmd.x1, cmd.x2, cmd.x),
            ...cubicExtremaTimes(from.y, cmd.y1, cmd.y2, cmd.y),
          ];
          // **곡선 위의 진짜 점을 넣는다** — 축마다 구한 `t` 라도 그 자리의 두 좌표를 함께
          // 넣는 편이 축을 갈라 넣는 것보다 싸고, 상자가 곡선 밖으로 나갈 자리가 없다.
          for (const t of times) {
            include(
              cubicAt(t, from.x, cmd.x1, cmd.x2, cmd.x),
              cubicAt(t, from.y, cmd.y1, cmd.y2, cmd.y),
            );
          }
        }
        cursor = { x: cmd.x, y: cmd.y };
        break;
      }
    }
  }
  return bounds;
}

// --- 문서 틀 안의 요소 자리 ---------------------------------------------

/** 문서 전체가 놓이는 틀 — `fitBox` 가 정한 상자와 그것이 담는 사용자 좌표 상자. */
export interface DocumentPlacement {
  readonly box: BoxGeometry;
  readonly viewBox: ViewBox;
}

/** 도형 하나의 자리. **두 값이 같은 수에서 나온다**(위 머리말). */
export interface ShapePlacement {
  /** 캔버스 단위 요소 상자. 계단 오프셋은 아직 더해지지 않았다(요소를 만드는 입구의 몫). */
  readonly box: BoxGeometry;
  /** 로컬 정규화의 기준 틀(사용자 단위). `toLocalCommands` 가 이것을 받는다. */
  readonly frame: ViewBox;
}

/**
 * 한 축의 기준 구간. **퇴화를 죄는 자리는 여기 하나다.**
 *
 * 가로선 하나의 바운딩 박스는 `h = 0` 이고, 그 상자를 저장하면 파서가 기하를 **통째로**
 * `DEFAULT_BOX_GEOMETRY` 로 갈아 끼운다(실측 `isDegenerateBox` → `parseBoxGeometry`) —
 * "저장할 땐 맞고 다시 열면 딴 자리" 라는 자료 손상이다(가정 A15 · REQ-06).
 *
 * M6 는 문서 상자에 대해 그것을 `Math.max(MIN_ELEMENT_EXTENT, coordinate(...))` 로 죄었다.
 * **요소마다에는 그 방식이 모자라다** — 상자만 1 로 밀어 올리면 로컬 정규화의 나누는 수는
 * 여전히 0 이고, 그러면 좌표가 `±∞` 를 지나 폴백 0 으로 내려앉아 도형이 상자 왼쪽 위
 * 모서리에 찌부러진다. 그래서 **사용자 단위에서 먼저 넓힌다**: 투영하면 정확히
 * `MIN_ELEMENT_EXTENT` 가 되는 길이가 하한이고, 상자 폭과 정규화의 나누는 수가 **그 한
 * 값에서 함께** 나온다. 그래서 이 함수 뒤에 `Math.max` 를 한 번 더 두지 않는다 — 넓힌 뒤의
 * `extent × scale` 은 1 이상이므로 그 죔쇠는 죽은 가지가 된다.
 *
 * **가운데를 지키며 넓힌다.** 한쪽으로만 밀면 선이 제 상자 안에서 위나 왼쪽에 붙고,
 * 손잡이 여덟이 그 선을 감싸지 않는다.
 */
function axisSpan(lo: number, hi: number, scale: number): { min: number; extent: number } {
  const floor = MIN_ELEMENT_EXTENT / scale;
  const measured = hi - lo;
  // `NaN` 은 이 비교에서 거짓이므로 넓히는 쪽으로 간다 — 잴 수 없는 축은 최소 크기다.
  if (measured >= floor) return { min: lo, extent: measured };
  return { min: (lo + hi) / 2 - floor / 2, extent: floor };
}

/**
 * 도형의 사용자 단위 바운딩 박스를 문서 틀 안의 요소 자리로 옮긴다.
 *
 * 축척은 **문서 틀의 것**이다(`docBox.w / viewBox.width`) — 도형마다 제 축척을 지어내면
 * 도형들이 서로 다른 배율로 놓여 문서의 배치가 흩어진다. 이 함수가 도형마다 바꾸는 것은
 * **원점과 길이**뿐이고 축척은 문서가 정한 하나다. 그래서 상대 배치가 보존된다.
 *
 * 반올림은 `coordinate()` 를 그대로 쓴다 — 두 번째 수치 규율을 만들지 않는다.
 */
export function placeShape(bounds: Bounds, doc: DocumentPlacement): ShapePlacement {
  const scaleX = doc.box.w / doc.viewBox.width;
  const scaleY = doc.box.h / doc.viewBox.height;
  const x = axisSpan(bounds.minX, bounds.maxX, scaleX);
  const y = axisSpan(bounds.minY, bounds.maxY, scaleY);
  return {
    box: {
      x: coordinate(doc.box.x + (x.min - doc.viewBox.minX) * scaleX, doc.box.x),
      y: coordinate(doc.box.y + (y.min - doc.viewBox.minY) * scaleY, doc.box.y),
      w: coordinate(x.extent * scaleX, MIN_ELEMENT_EXTENT),
      h: coordinate(y.extent * scaleY, MIN_ELEMENT_EXTENT),
    },
    frame: { minX: x.min, minY: y.min, width: x.extent, height: y.extent },
  };
}

/**
 * `viewBox` 를 상자로. **잴 수 없는 도형이 떨어질 자리**다 — 좌표가 전부 비유한이면 잴
 * 바운딩 박스가 없고, 그때 문서 틀을 그대로 쓰면 007 이 배달했던 그 자리에 놓인다(요소를
 * 잃지 않는다). `placeShape` 에 특례를 두지 않고 **입력으로** 처리하는 것이 요점이다.
 */
export function viewBoxBounds(viewBox: ViewBox): Bounds {
  return {
    minX: viewBox.minX,
    minY: viewBox.minY,
    maxX: viewBox.minX + viewBox.width,
    maxY: viewBox.minY + viewBox.height,
  };
}
