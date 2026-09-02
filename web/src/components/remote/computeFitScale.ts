// 고정 캔버스 스케일-투-핏 배율 계산 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M08, OQ-M4).
//
// 컴포넌트 파일(FixedCanvasScaler.tsx)에서 분리한 순수 함수 모듈이다. 컴포넌트 파일이
// 컴포넌트만 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

/**
 * 종횡비 보존 스케일-투-핏 배율을 계산한다(REQ-M08, OQ-M4).
 *
 * `scale = min(availW/canvasW, availH/canvasH)` — 가용 영역에 캔버스를 종횡비
 * 보존하며 맞춘 배율이다. 가용 영역과 캔버스의 종횡비가 다르면 남는 축에
 * 레터박스/필러박스 여백이 생긴다(stretch/crop 없음).
 *
 * 방어:
 *   - 캔버스 폭/높이가 0 이하이면 1 을 반환한다(폴백은 호출자 책임).
 *   - 가용 영역이 0 이하이면 1 을 반환한다(측정 전 상태 — 후속 측정으로 교체).
 *
 * @returns 양수 배율(0 < scale). 1:1 이상으로 확대될 수도, 축소될 수도 있다.
 */
export function computeFitScale(
  availW: number,
  availH: number,
  canvasW: number,
  canvasH: number,
): number {
  if (canvasW <= 0 || canvasH <= 0) return 1;
  if (availW <= 0 || availH <= 0) return 1;
  return Math.min(availW / canvasW, availH / canvasH);
}
