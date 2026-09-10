// 경로 명령 어휘와 그 상수 (SPEC-CANVAS-008 M1).
//
// 카탈로그 30종 가운데 23종은 다각형이고 7종이 곡선이다. 그 전부를 적는 데 필요한 명령은
// 넷뿐이다 — `M`(옮김) · `L`(직선) · `C`(3차 베지어) · `Z`(닫음). 2차 베지어를 두지 않는
// 것은 3차가 그것을 **정확히** 표현하기 때문이고(`c1 = p0 + 2/3(q−p0)`), 호(arc)를 두지
// 않는 것은 4분원이 3차 베지어로 최대 반경 오차 0.027% 에 근사되며 `arcTo` 의 접선 의미가
// "모서리를 적는 두 번째 방법" 을 만들기 때문이다(spec.md §DrawContext2D 에 더하는 것은 둘).
//
// **이 모듈은 DOM 을 모른다.** 순수 자료와 숫자뿐이라 jsdom 없이 전량 단위 시험된다.
// 좌표를 요소 상자 **로컬**로 두는 것이 이 어휘의 절반이다 — 상자를 늘리면 도형이 함께
// 늘어나고, 캔버스 좌표계(정수 단위)와 섞이지 않는다. 무엇보다 경로 명령은 기하가 아니므로
// 기하 쓰기 통로(`patchNodeGeometry`)를 지나지 않는다(불변식 J2).
//
// @spec SPEC-CANVAS-008

/**
 * 경로 명령 한 개. 좌표는 **요소 상자 로컬 정수**(공칭 0..`PATH_LOCAL_EXTENT`)다.
 *
 * 판별자를 `c` 한 글자로 둔 것은 config 크기 때문이다 — 명령 하나가 8~12개씩 붙고 한
 * 대시보드의 snapshot 예산이 256KB 이므로, 키 이름 한 글자가 실제로 값을 한다.
 */
export type PathCommand =
  | { c: 'M'; x: number; y: number }
  | { c: 'L'; x: number; y: number }
  | { c: 'C'; x1: number; y1: number; x2: number; y2: number; x: number; y: number }
  | { c: 'Z' };

/**
 * 경로 로컬 격자. 상자 한 변이 이 값에 대응한다.
 *
 * **정규화 분수(0..1)를 기각한 자리다.** `canvasConfig` 의 좌표 규율은 오늘 하나뿐이고
 * (유한하면 반올림, 아니면 폴백 — `coordinate()`), 분수를 들이면 그 파일에 두 번째 수치
 * 규율이 생긴다. 정수 격자 위에서는 그 함수를 그대로 재사용한다.
 *
 * 10000 인 이유: 양자화 오차가 `상자 길이 ÷ 10000` 캔버스 단위라, 100 단위짜리 도형에서
 * 0.01 단위 — 캔버스 자체의 양자(1 단위)보다 두 자릿수 작다. JSON 크기도 정수가 작다
 * (`552` 세 자 vs `0.5523` 여섯 자).
 */
export const PATH_LOCAL_EXTENT = 10000;

/**
 * 한 요소가 가질 수 있는 명령 수의 상한.
 *
 * 손상되거나 거대한 목록 하나가 한 프레임을 삼키지 않게 하는 죔쇠다(REQ-07). 카탈로그
 * 30종 가운데 가장 긴 것도 수십 개이므로 256 은 넉넉한 여유다.
 */
export const MAX_PATH_COMMANDS = 256;

/**
 * 씨앗 경로 — 로컬 격자를 가득 채우는 단위 사각형.
 *
 * 손상된 명령 목록을 만났을 때 **요소를 버리지 않고** 이것으로 대체한다. 001 의 기하 손상
 * 정책과 같은 규율이다: 버리는 것은 정체성이 없을 때뿐이고, 보이지 않는 요소는 사용자가
 * 찾을 수 없어 고칠 수도 없다.
 *
 * 얼려 둔 것은 값이지 방어가 아니다 — 읽는 쪽은 언제나 **사본**을 만들어 쓴다.
 */
export const DEFAULT_PATH: readonly PathCommand[] = Object.freeze([
  Object.freeze({ c: 'M', x: 0, y: 0 }),
  Object.freeze({ c: 'L', x: PATH_LOCAL_EXTENT, y: 0 }),
  Object.freeze({ c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT }),
  Object.freeze({ c: 'L', x: 0, y: PATH_LOCAL_EXTENT }),
  Object.freeze({ c: 'Z' }),
] as const);
