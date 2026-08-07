// 센서 배치 좌표 변환 순수 함수 (SPEC-HEATMAP-PANEL-002 T5).
//
// 드래그 배치 에디터의 좌표계 핵심. 세 레이어(배경/히트맵/마커)가 공유하는 정규화 좌표(0..1)와
// 컨테이너 실측 픽셀 좌표 사이를 변환한다. DOM 에 의존하지 않도록 rect 를 인자로 받아 순수 함수로
// 유지한다(단위 테스트 필수 — 경계값/도면밖 clamp/스냅).
//
// 좌표계 원칙:
//   - 저장 표현은 항상 정규화(0..1). 리사이즈/도면 교체에 불변(A4, AC-E4).
//   - 포인터 → 정규화 변환은 [0,1] 로 clamp 해 도면 밖 좌표 저장을 금지한다(REQ-04, AC-E2, R5).
//   - 픽셀 배치(fromNormalized)는 표시 시점 파생이다(R4).
//
// @spec SPEC-HEATMAP-PANEL-002

/** 정규화 좌표(0..1). */
export interface NormalizedPos {
  x: number;
  y: number;
}

/** 컨테이너 실측 사각형(getBoundingClientRect 부분집합). */
export interface ContainerRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * 값을 [0,1] 로 clamp 한다. 비유한(NaN/Infinity)은 0 으로 방어한다(AC-E2, R5).
 * 좌표가 [0,1] 범위를 벗어나 저장되지 않도록 하는 최종 방어선이다.
 */
export function clamp01(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/**
 * 포인터 client 좌표를 컨테이너 rect 기준 정규화 좌표(0..1)로 변환한다. 결과는 [0,1] 로
 * clamp 되어 도면 경계 밖 포인터도 절대 범위 밖으로 저장되지 않는다(AC-E2). width/height 가
 * 0 이하면 0 으로 폴백한다(0 나눗셈 방어).
 */
export function toNormalized(
  clientX: number,
  clientY: number,
  rect: ContainerRect,
): NormalizedPos {
  const nx = rect.width > 0 ? (clientX - rect.left) / rect.width : 0;
  const ny = rect.height > 0 ? (clientY - rect.top) / rect.height : 0;
  return { x: clamp01(nx), y: clamp01(ny) };
}

/**
 * 정규화 좌표를 컨테이너 표시 크기 기준 픽셀 배치(left/top)로 파생한다(표시 시점, R4).
 * 좌표는 clamp 되어 마커가 컨테이너 밖으로 나가지 않는다.
 */
export function fromNormalized(
  pos: NormalizedPos,
  rect: { width: number; height: number },
): { left: number; top: number } {
  return {
    left: clamp01(pos.x) * rect.width,
    top: clamp01(pos.y) * rect.height,
  };
}

/**
 * 그리드 스냅을 적용한다. snap 이 falsy/비유한/0 이하면 원본 그대로 반환한다(no-op). 유효하면
 * 각 축을 가장 가까운 snap 배수로 반올림하고 [0,1] 로 clamp 한다.
 */
export function applySnap(pos: NormalizedPos, snap?: number): NormalizedPos {
  if (!snap || !Number.isFinite(snap) || snap <= 0) return pos;
  const snapAxis = (v: number) => clamp01(Math.round(v / snap) * snap);
  return { x: snapAxis(pos.x), y: snapAxis(pos.y) };
}
