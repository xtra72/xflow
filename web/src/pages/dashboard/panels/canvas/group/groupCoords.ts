// 그룹 로컬 좌표 변환 (SPEC-CANVAS-004 M2).
//
// 묶기는 **캔버스 단위 → 그룹 로컬 정수**, 풀기는 그 역이다. 두 방향 다 새 산술이
// 아니다 — `canvasGeometry` 의 투영/역투영을 `localProjection(상자)` 로 다시 매개변수화한
// 것이며, `로컬 ÷ EXTENT × 상자변` 이라는 **글자는 이 파일에 한 번도 나오지 않는다**
// (불변식 G2 · 008 불변식 J3).
//
// ## 왜 `project*In` 에 **캔버스 단위 상자**를 넣는가
//
// `projectBoxIn` 의 인자 이름은 `PxBox` 지만 그 산술에는 단위가 없다. `ellipseParams` 가
// 같은 성질을 먼저 적어 두었다 — "캔버스 단위 박스든 투영된 px 박스든 같은 산술이라 어느
// 쪽에도 쓸 수 있다". 상자와 로컬 좌표가 **같은 공간**에 있기만 하면 되므로, 그룹의 캔버스
// 단위 상자를 넣으면 캔버스 단위가 나온다. 그래서 그리는 쪽(px)과 푸는 쪽(캔버스 단위)이
// **같은 함수**를 지나고, 둘이 갈라질 자리가 없다.
//
// ## 왕복 오차의 상한을 감추지 않는다 (가정 A15)
//
// 두 방향 모두 정수로 반올림하므로 왕복 오차는 **`상자변 ÷ (2 × EXTENT)` 캔버스 단위**를
// 넘지 않는다. 기본 캔버스 500×400 에서 ≤ 0.025 단위라 정수 반올림이 원본을 **정확히**
// 되살리고, 한 변이 `EXTENT` 를 넘는 그룹에서만 왕복이 손실이다. 그 사실을 시험이
// **양쪽 다** 단언한다(시험 규율 E-G).
//
// **이 모듈은 DOM 을 모른다.** 순수 산술뿐이다.
//
// @spec SPEC-CANVAS-004 REQ-07

import {
  coordinate,
  MIN_ELEMENT_EXTENT,
  type BoxGeometry,
  type Geometry,
  type LineGeometry,
  type PointGeometry,
} from '../canvasConfig';
import {
  localProjection,
  projectBoxIn,
  projectLineIn,
  projectPointIn,
  unprojectBox,
  unprojectPoint,
  type CanvasBox,
  type CanvasPoint,
} from '../canvasGeometry';

/** 좌표 하나를 정수로 죈다. 001 의 `coordinate` 를 그대로 쓴다 — 반올림 규율은 하나다. */
function round(v: number): number {
  return coordinate(v, 0);
}

// --- 절대 → 로컬 (묶기) ----------------------------------------------------
//
// 자리(x·y)에서는 원점을 빼고 길이(w·h)에서는 빼지 않는다. **그것이 상자와 자리의
// 차이의 전부**이며, 그 하나를 여기서 적고 나머지 축척은 역투영이 맡는다.
//
// `unproject*In` 을 `canvasGeometry` 에 **더하지 않는 것**에 뜻이 있다(가정 A18). 004 는
// 부품에 핸들을 주지 않으므로 포인터를 로컬로 되돌릴 일이 없고, 그 함수가 공개 API 로
// 서면 "그린 자리와 잡히는 자리가 같다" 는 훨씬 비싼 불변식을 지켜야 한다. 여기 있는 것은
// 포인터가 아니라 **캔버스 단위 저술값**을 옮기는 산술이다(불변식 G1 — 묶기/풀기는 캔버스
// 단위 안에서만 산다).

/** rect/ellipse/path 상자를 그룹 로컬 정수 격자로 옮긴다. */
export function toLocalBox(geo: BoxGeometry, box: CanvasBox): BoxGeometry {
  const local = unprojectBox(
    { x: geo.x - box.x, y: geo.y - box.y, w: geo.w, h: geo.h },
    localProjection(box),
  );
  return { x: round(local.x), y: round(local.y), w: round(local.w), h: round(local.h) };
}

/** 선 기하를 그룹 로컬 정수 격자로 옮긴다(끝점 둘 다 자리이므로 둘 다 원점을 뺀다). */
export function toLocalLine(geo: LineGeometry, box: CanvasBox): LineGeometry {
  const proj = localProjection(box);
  const p1 = unprojectPoint({ x: geo.x1 - box.x, y: geo.y1 - box.y }, proj);
  const p2 = unprojectPoint({ x: geo.x2 - box.x, y: geo.y2 - box.y }, proj);
  return { x1: round(p1.x), y1: round(p1.y), x2: round(p2.x), y2: round(p2.y) };
}

/** 문구 기준점을 그룹 로컬 정수 격자로 옮긴다. */
export function toLocalPoint(geo: PointGeometry, box: CanvasBox): PointGeometry {
  const p = unprojectPoint({ x: geo.x - box.x, y: geo.y - box.y }, localProjection(box));
  return { x: round(p.x), y: round(p.y) };
}

// --- 로컬 → 절대 (풀기) ----------------------------------------------------

/** 그룹 로컬 상자를 캔버스 단위 정수로 되돌린다. */
export function toAbsoluteBox(geo: BoxGeometry, box: CanvasBox): BoxGeometry {
  const abs = projectBoxIn(geo, box);
  return { x: round(abs.x), y: round(abs.y), w: round(abs.w), h: round(abs.h) };
}

/** 그룹 로컬 선을 캔버스 단위 정수로 되돌린다. */
export function toAbsoluteLine(geo: LineGeometry, box: CanvasBox): LineGeometry {
  const abs = projectLineIn(geo, box);
  return { x1: round(abs.x1), y1: round(abs.y1), x2: round(abs.x2), y2: round(abs.y2) };
}

// --- 되돌림 한 쌍: 저장할 것인가, 그릴 것인가 -------------------------------
//
// 아래 둘은 **같은 산술**을 지나고 마지막 한 가지에서만 갈린다 — 정수화 여부다. 둘이
// 존재하는 까닭은 되돌린 값을 **누가 받느냐**가 다르기 때문이다.
//
//   - `toAbsolutePointRounded` → **저장되는 기하**로 간다(그룹 풀기 · 부품 분리). 캔버스
//     좌표계가 정수라 소수 자리를 담을 곳이 없고, 남겨 두면 격자·붙임·수치 칸이 저마다
//     다른 반올림을 하게 된다(001 `coordinate()` 의 근거 그대로).
//   - `toAbsolutePointExact` → **파생되는 자리**로 간다(앵커 · 연결선 끝점). 저장되지
//     않으므로 담을 곳의 제약이 없고, 정수화하면 그 자리가 **그려지는 자리에서 밀린다.**
//
// **반올림은 산술의 성질이 아니라 쓰는 쪽의 성질이다.** 그 사실을 이름에 적어 두지 않으면
// 호출부에서 구분이 서지 않는다 — 011 이 실제로 그 함정을 밟았고, 재어 보니 별의 꼭지점에
// 붙인 앵커가 그려지는 꼭지점에서 **축마다 최대 0.5 캔버스 단위**(기본 축척에서 화면
// 1 px) 밀렸다. 산술은 정확했고(잔차 2.8e-14) 어긋남은 전부 정수화였다.
//
// 반환 타입이 갈리는 것도 같은 말이다. 정수 쪽은 `PointGeometry`(기하)를, 정확 쪽은
// `CanvasPoint`(자리)를 낸다 — `canvasGeometry` 가 그 둘을 "형상은 같고 뜻이 다르다
// (자리 vs 기하)" 로 갈라 둔 그 구분이며, 그래서 파생값을 기하 쓰기 통로에 잘못 흘리면
// 타입이 먼저 운다.
//
// 되돌림의 **반대 방향은 한 벌뿐이다.** `toLocalPoint` 는 정수화를 유지한다 — 로컬 좌표는
// 저장되는 값이고(008 이 JSON 크기로 그 정수 격자를 고른 근거가 그대로 선다), 꼭지점
// 붙임의 "정확히" 는 환산이 아니라 **경로 명령의 좌표를 그대로 저장하는 데서** 나온다.

/** 그룹 로컬 기준점을 캔버스 단위 **정수 기하**로 되돌린다(저장되는 값). */
export function toAbsolutePointRounded(geo: PointGeometry, box: CanvasBox): PointGeometry {
  const abs = projectPointIn(geo, box);
  return { x: round(abs.x), y: round(abs.y) };
}

/** 로컬 기준점을 캔버스 단위 **자리**로 되돌린다 — 정수화하지 않는다(파생되는 값). */
export function toAbsolutePointExact(geo: PointGeometry, box: CanvasBox): CanvasPoint {
  const abs = projectPointIn(geo, box);
  return { x: abs.x, y: abs.y };
}

// --- 기하 형상별 갈래 ------------------------------------------------------
//
// `moveGeometry` 가 세운 오버로드 형상을 그대로 따른다 — 종류로 좁힌 호출부는 좁혀진
// 기하를 돌려받고, 좁히지 않은 호출부는 합집합을 받는다.

export function toLocalGeometry(geo: BoxGeometry, box: CanvasBox): BoxGeometry;
export function toLocalGeometry(geo: LineGeometry, box: CanvasBox): LineGeometry;
export function toLocalGeometry(geo: PointGeometry, box: CanvasBox): PointGeometry;
export function toLocalGeometry(geo: Geometry, box: CanvasBox): Geometry;
export function toLocalGeometry(geo: Geometry, box: CanvasBox): Geometry {
  if ('w' in geo) return toLocalBox(geo, box);
  if ('x1' in geo) return toLocalLine(geo, box);
  return toLocalPoint(geo, box);
}

export function toAbsoluteGeometry(geo: BoxGeometry, box: CanvasBox): BoxGeometry;
export function toAbsoluteGeometry(geo: LineGeometry, box: CanvasBox): LineGeometry;
export function toAbsoluteGeometry(geo: PointGeometry, box: CanvasBox): PointGeometry;
export function toAbsoluteGeometry(geo: Geometry, box: CanvasBox): Geometry;
export function toAbsoluteGeometry(geo: Geometry, box: CanvasBox): Geometry {
  if ('w' in geo) return toAbsoluteBox(geo, box);
  if ('x1' in geo) return toAbsoluteLine(geo, box);
  return toAbsolutePointRounded(geo, box);
}

// --- 퇴화 상자 넓히기 ------------------------------------------------------

/**
 * 한 축을 **가운데를 지키며** 최소 크기 이상으로 넓힌다.
 *
 * **`toLocal` 을 부르기 **전에** 일어나야 한다.** 가로선 둘만 골라 묶으면 상자 높이가 0 이고,
 * 그때 나누는 수가 0 이라 모든 부품의 로컬 y 가 0 으로 내려앉아 **상자 왼쪽 위 모서리에
 * 찌부러진다.** 그 결함은 예외를 내지 않고 저장 왕복을 견디며 **화면으로만** 드러난다 —
 * 007 0.2.0 이 같은 함정에서 같은 결론에 도달했고 004 가 그 교훈을 인용으로 물려받는다.
 *
 * **양쪽으로 같은 양을 넓히는 것**이 요점이다. 한쪽만 넓히면 가운데가 반 칸 어긋나고,
 * 캔버스 좌표가 정수라 그 반 칸은 반올림에서 사라져 부품이 다시 모서리로 간다. 양쪽에
 * `MIN_ELEMENT_EXTENT` 씩 더하면 넓힌 길이가 짝수라 **가운데가 정수에 남는다** —
 * 그래서 퇴화 축의 부품 로컬 좌표가 `EXTENT / 2` 가 된다(모서리가 아니라 한가운데다).
 */
function widenAxis(origin: number, span: number): readonly [number, number] {
  if (span >= MIN_ELEMENT_EXTENT) return [origin, span];
  const safe = span > 0 ? Math.round(span) : 0;
  const center = origin + safe / 2;
  const widened = safe + 2 * MIN_ELEMENT_EXTENT;
  return [round(center - widened / 2), widened];
}

/**
 * 퇴화한 변을 가운데를 지키며 넓힌 상자. 두 변 모두 멀쩡하면 **받은 값을 그대로 돌려준다**
 * (같은 참조는 아니지만 값이 같다 — 넓히기가 멀쩡한 상자를 건드리지 않는다는 뜻이다).
 */
export function widenDegenerateBox(box: CanvasBox): CanvasBox {
  const [x, w] = widenAxis(box.x, box.w);
  const [y, h] = widenAxis(box.y, box.h);
  return { x, y, w, h };
}
