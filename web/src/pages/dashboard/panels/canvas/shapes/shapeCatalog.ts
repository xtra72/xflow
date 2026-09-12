// 도형 카탈로그 30종 — 묶음 셋(일반 10 · 기본 12 · 화살표 8) (SPEC-CANVAS-008 M6).
//
// **순수 자료다.** React 도 i18n 도 DOM 도 부르지 않는다 — 이름은 키로만 들고 있고 그 키를
// 문자열로 바꾸는 일은 화면의 몫이다(그래야 로케일이 config 에 박히지 않는다). 그림 그리는
// 일도 하지 않는다: 여기 있는 것은 명령 목록뿐이고, 그리는 자는 `drawElement` 하나다.
//
// **카탈로그에 설비 심볼(밸브 · 펌프)을 넣지 않는다**(위험 R6). 경로는 통째로 하나의
// 스타일을 입으므로 규칙 표가 그 **안쪽을 지목하지 못한다**. 상태를 나르는 조립체는
// SPEC-CANVAS-004 의 `group` 이 맡기로 갈랐고, 008 이 파는 것은 **장식과 구조**다.
//
// **명령 목록은 값으로 복사되어 config 에 실린다**(참조가 아니다 — spec.md §경로 자료는
// 값인가 참조인가). 그래서 여기 있는 배열은 전부 얼려 둔다: 놓인 요소가 이 자료를 **가리키고**
// 있으면 카탈로그를 고칠 때 이미 놓인 도형이 조용히 달라진다. 얼림은 그 우회로를 막는
// 방어가 아니라 **읽는 사람에게 하는 선언**이고, 실제 방어는 `newPathElement` 가 명령마다
// 새 객체를 만드는 데에 있다.
//
// **왜 이 30종인가.** 사용자가 draw.io 화면을 내밀며 여섯 묶음 가운데 셋을 골랐다
// (순서도 · ER · UML 은 물렸다 — 그 셋의 본체는 도형이 아니라 **연결선**이고, 연결선 없이
// 상자만 주면 사용자는 `line` 으로 잇게 되며 그 선은 상자를 옮겨도 따라오지 않는다).
//
// **곡선을 요구하는 것은 정확히 7종**이고 나머지 23종은 `M`·`L`·`Z` 만으로 그려진다 —
// 가정 A2 가 그 산술로 `DrawContext2D` 에 더할 멤버를 둘로 못박았고, 이 모듈의 시험이
// 그 수를 **세어서** 확인한다(추정하지 않는다).
//
// **닫힘은 `Z` 가 정한다.** 열린 도형은 하나뿐이며(곡선 화살표) 그 사실이 씨앗 스타일을
// 가른다(`canvasElementFactory.newPathElement` — 닫히지 않은 윤곽을 채우면 그리지 않은
// 변이 생긴다).
//
// **부분 경로의 감김 방향은 하중을 받는다.** `fill()` 은 열린 부분 경로를 암묵적으로 닫고
// nonzero 규칙으로 채우므로, 겉 윤곽과 **반대로** 감긴 부분 경로는 구멍이 된다. 그것을
// 일부러 쓰는 곳이 하나(`process` 의 세로 홈 둘)이고, 실수로 그리 되면 안 되는 곳이
// 둘(`note` 의 접힘 선 · `cube` 의 모서리 선)이라 그 셋은 방향을 손으로 맞춰 두었다.
//
// @spec SPEC-CANVAS-008 REQ-02 · AC-03

import { PATH_LOCAL_EXTENT, type PathCommand } from './pathTypes';

// --- 묶음 -------------------------------------------------------------

/** 카탈로그 묶음 셋. 원시형 넷은 카탈로그가 아니므로 여기 없다(팔레트가 따로 낸다). */
export type ShapeGroupId = 'general' | 'basic' | 'arrow';

/** 카탈로그 도형 하나. */
export interface ShapeCatalogEntry {
  /**
   * 로케일 비의존 영문 id. **점을 쓰지 않는다** — 이 값이 그대로 `catalog_id` 로 config 에
   * 실리고, i18n 키의 마지막 마디가 되기 때문이다(프로젝트 규약 · `i18nKeyShape.test.ts`).
   */
  id: string;
  group: ShapeGroupId;
  /** 화면에 보이는 이름의 i18n 키. 이 모듈은 문자열을 갖지 않는다. */
  nameKey: string;
  /** 요소 상자 로컬 정수 명령(공칭 0..`PATH_LOCAL_EXTENT`). */
  path: readonly PathCommand[];
}

/** 한 묶음. */
export interface ShapeCatalogGroup {
  id: ShapeGroupId;
  titleKey: string;
  entries: readonly ShapeCatalogEntry[];
}

// --- 좌표 도우미 -------------------------------------------------------
//
// 상수 이름을 짧게 두는 것은 이 파일이 좌표표이기 때문이다 — 30종의 명령이 세로로 늘어서는
// 곳에서 함수 이름이 길면 좌표 자체가 읽히지 않는다.

const E = PATH_LOCAL_EXTENT;

/** `M` 한 개. */
const m = (x: number, y: number): PathCommand => ({ c: 'M', x, y });
/** `L` 한 개. */
const l = (x: number, y: number): PathCommand => ({ c: 'L', x, y });
/** `C` 한 개(3차 베지어). */
const c = (
  x1: number,
  y1: number,
  x2: number,
  y2: number,
  x: number,
  y: number,
): PathCommand => ({ c: 'C', x1, y1, x2, y2, x, y });
/** `Z` 한 개. */
const z = (): PathCommand => ({ c: 'Z' });

/** 다각형 하나를 `M`·`L`…·`Z` 로 편다(23종 가운데 대부분이 이 꼴이다). */
function poly(...pts: readonly (readonly [number, number])[]): PathCommand[] {
  const [head, ...rest] = pts;
  if (head === undefined) return [];
  return [m(head[0], head[1]), ...rest.map(([x, y]) => l(x, y)), z()];
}

// --- 일반 (10) ---------------------------------------------------------

/**
 * 둥근 사각형 — 모서리 반경 2000. **4분원을 3차 베지어로 근사한다**(`k = 4/3(√2−1)`,
 * 제어점 오프셋 `k·r ≈ 1105`). 최대 반경 오차 0.027% 로, 실사용 크기에서 장치 픽셀 하나보다
 * 훨씬 작다 — `arcTo` 를 들이지 않은 근거가 이 수다(spec.md).
 */
const roundedRect: PathCommand[] = [
  m(2000, 0),
  l(8000, 0),
  c(9105, 0, E, 895, E, 2000),
  l(E, 8000),
  c(E, 9105, 9105, E, 8000, E),
  l(2000, E),
  c(895, E, 0, 9105, 0, 8000),
  l(0, 2000),
  c(0, 895, 895, 0, 2000, 0),
  z(),
];

/**
 * 처리(미리 정의된 처리) — 사각형 몸통에 세로 홈 둘.
 *
 * 홈을 **반대 방향으로 감아** 진짜 구멍으로 둔다. 그냥 같은 방향으로 감으면 nonzero 규칙이
 * 둘을 합쳐 버려 그냥 사각형이 되고, 그러면 이 칸은 원시형 `rect` 와 화면에서 구분되지
 * 않는다 — 구분되지 않는 칸은 목록을 길게 할 뿐이다.
 */
const process: PathCommand[] = [
  ...poly([0, 0], [E, 0], [E, E], [0, E]),
  // 반대 감김(아래로 → 오른쪽 → 위로) = 구멍.
  ...poly([1000, 0], [1000, E], [1700, E], [1700, 0]),
  ...poly([8300, 0], [8300, E], [9000, E], [9000, 0]),
];

/** 문서 — 아래가 물결이다. 물결의 **올라오는 쪽**이 이 도형을 오목하게 만든다. */
const document_: PathCommand[] = [
  m(0, 0),
  l(E, 0),
  l(E, 7500),
  c(8333, E, 6667, 5000, 5000, 7500),
  c(3333, E, 1667, 5000, 0, 7500),
  z(),
];

/**
 * 원통 — 몸통 하나와 **위 타원 하나**, 둘 다 닫힌 부분 경로다.
 *
 * 타원을 겹쳐 두는 것에 뜻이 있다: 몸통과 **같은 방향**으로 감았으므로 채움은 둘의 합집합
 * (= 몸통)이라 실루엣이 달라지지 않고, 선은 앞쪽 테두리를 그려 원통으로 읽힌다. 타원을
 * 빼면 이 도형은 그냥 길쭉한 알약이다.
 */
const cylinder: PathCommand[] = [
  m(0, 1500),
  c(0, 672, 2239, 0, 5000, 0),
  c(7761, 0, E, 672, E, 1500),
  l(E, 8500),
  c(E, 9328, 7761, E, 5000, E),
  c(2239, E, 0, 9328, 0, 8500),
  z(),
  m(0, 1500),
  c(0, 672, 2239, 0, 5000, 0),
  c(7761, 0, E, 672, E, 1500),
  c(E, 2328, 7761, 3000, 5000, 3000),
  c(2239, 3000, 0, 2328, 0, 1500),
  z(),
];

/**
 * 정육면체 — 실루엣 육각형 + 모서리 선 둘(열린 부분 경로).
 *
 * 모서리 선의 감김 방향을 실루엣과 **맞춰** 적는다. `fill()` 이 열린 부분 경로를 암묵적으로
 * 닫으므로, 반대로 적으면 앞면 위쪽에 삼각형 구멍이 뚫린다 — 화면에서만 드러나는 부류다.
 */
const cube: PathCommand[] = [
  ...poly([0, 2500], [2500, 0], [E, 0], [E, 7500], [7500, E], [0, E]),
  m(E, 0),
  l(7500, 2500),
  l(0, 2500),
  m(7500, 2500),
  l(7500, E),
];

/** 카드 — 왼쪽 위 모서리를 **비스듬히 자른** 사각형. 자르기만 하므로 볼록하다. */
const card: PathCommand[] = poly([2500, 0], [E, 0], [E, E], [0, E], [0, 2500]);

/**
 * 메모 — 오른쪽 위 모서리가 접힌 종이.
 *
 * **실루엣은 볼록하다.** 모서리를 앞으로 접으면 종이의 그림자는 여전히 오각형이며, 접힘을
 * 구멍으로 파면 접힌 종이가 아니라 ㄱ 자로 잘린 종이가 된다. 그래서 접힘은 **선**으로만
 * 적고(열린 부분 경로), 그 선의 감김을 실루엣과 맞춰 구멍이 생기지 않게 한다.
 *
 * spec.md 는 이 도형을 오목 12종에 넣었으나 실측은 그렇지 않다 — 본 카탈로그의 오목 판정은
 * 시험이 **볼록 껍질로 재고**, 그 결과를 이 모듈이 아니라 시험이 소유한다.
 */
const note: PathCommand[] = [
  ...poly([0, 0], [7000, 0], [E, 3000], [E, E], [0, E]),
  m(E, 3000),
  l(7000, 3000),
  l(7000, 0),
];

/** 단계 — 뒤에 홈이 파이고 앞이 뾰족하다. 홈 꼭짓점이 오목을 만든다. */
const step: PathCommand[] = poly(
  [0, 0],
  [7500, 0],
  [E, 5000],
  [7500, E],
  [0, E],
  [2500, 5000],
);

/** 말풍선 — 둥근 몸통에 아래로 뻗은 꼬리. 꼬리 양옆이 오목 지역이다. */
const callout: PathCommand[] = [
  m(1200, 0),
  l(8800, 0),
  c(9463, 0, E, 537, E, 1200),
  l(E, 5800),
  c(E, 6463, 9463, 7000, 8800, 7000),
  l(4500, 7000),
  l(2000, E),
  l(2600, 7000),
  l(1200, 7000),
  c(537, 7000, 0, 6463, 0, 5800),
  l(0, 1200),
  c(0, 537, 537, 0, 1200, 0),
  z(),
];

/** 액터 — 머리(닫힌 타원)와 몸통(닫힌 다각형) 둘. 겨드랑이와 가랑이가 오목이다. */
const actor: PathCommand[] = [
  m(5000, 0),
  c(5828, 0, 6500, 672, 6500, 1500),
  c(6500, 2328, 5828, 3000, 5000, 3000),
  c(4172, 3000, 3500, 2328, 3500, 1500),
  c(3500, 672, 4172, 0, 5000, 0),
  z(),
  ...poly(
    [3800, 3800],
    [6200, 3800],
    [6200, 4600],
    [9200, 4600],
    [9200, 5400],
    [6200, 5400],
    [6200, 6600],
    [8000, E],
    [6800, E],
    [5000, 7000],
    [3200, E],
    [2000, E],
    [3800, 6600],
    [3800, 5400],
    [800, 5400],
    [800, 4600],
    [3800, 4600],
  ),
];

// --- 기본 (12) ---------------------------------------------------------

const triangle: PathCommand[] = poly([5000, 0], [E, E], [0, E]);

/** 직각삼각형 — **비대칭**이라 x·y 를 뒤바꾼 투영 결함이 여기서 드러난다(시험 규율 D2). */
const rightTriangle: PathCommand[] = poly([0, 0], [0, E], [E, E]);

const diamond: PathCommand[] = poly([5000, 0], [E, 5000], [5000, E], [0, 5000]);

const parallelogram: PathCommand[] = poly([2500, 0], [E, 0], [7500, E], [0, E]);

const trapezoid: PathCommand[] = poly([2500, 0], [7500, 0], [E, E], [0, E]);

/** 정오각형 — 외접원 위의 다섯 점을 상자에 맞춰 늘렸다. */
const pentagon: PathCommand[] = poly([5000, 0], [E, 3820], [8087, E], [1913, E], [0, 3820]);

const hexagon: PathCommand[] = poly(
  [2500, 0],
  [7500, 0],
  [E, 5000],
  [7500, E],
  [2500, E],
  [0, 5000],
);

const octagon: PathCommand[] = poly(
  [2929, 0],
  [7071, 0],
  [E, 2929],
  [E, 7071],
  [7071, E],
  [2929, E],
  [0, 7071],
  [0, 2929],
);

/** 십자 — 겨드랑이 넷이 **상자 안이면서 도형 밖**이다. 히트 판정이 갈리는 자리다. */
const cross: PathCommand[] = poly(
  [3500, 0],
  [6500, 0],
  [6500, 3500],
  [E, 3500],
  [E, 6500],
  [6500, 6500],
  [6500, E],
  [3500, E],
  [3500, 6500],
  [0, 6500],
  [0, 3500],
  [3500, 3500],
);

/** 사각별 — 바깥 꼭짓점 넷(변의 한가운데) · 안 꼭짓점 넷(대각선). */
const star4: PathCommand[] = poly(
  [5000, 0],
  [6344, 3656],
  [E, 5000],
  [6344, 6344],
  [5000, E],
  [3656, 6344],
  [0, 5000],
  [3656, 3656],
);

/**
 * 오각별 — 바깥 반지름 5000 · 안 반지름 1910 의 열 점을 상자에 맞춰 늘렸다.
 *
 * 좌표를 손으로 적어 둔다. 코드가 삼각함수로 지어내면 부호를 뒤집은 결함이 시험에서도
 * 똑같이 뒤집혀 통과한다 — 같은 셈을 두 번 쓰는 것은 세는 것이 아니다.
 */
const star5: PathCommand[] = poly(
  [5000, 0],
  [6180, 3820],
  [E, 3820],
  [6910, 6180],
  [8087, E],
  [5000, 7639],
  [1913, E],
  [3090, 6180],
  [0, 3820],
  [3821, 3820],
);

/** 구름 — 볼록한 혹 여섯이 이어지고 바닥은 `Z` 가 긋는 직선이다. */
const cloud: PathCommand[] = [
  m(2200, E),
  c(800, E, 0, 8900, 0, 7500),
  c(0, 6200, 900, 5100, 2100, 5000),
  c(2200, 3200, 3600, 1800, 5300, 1800),
  c(6800, 1800, 8000, 2800, 8400, 4200),
  c(9400, 4500, E, 5500, E, 6800),
  c(E, 8600, 8700, E, 7000, E),
  z(),
];

// --- 화살표 (8) --------------------------------------------------------

const arrowRight: PathCommand[] = poly(
  [0, 3000],
  [6000, 3000],
  [6000, 500],
  [E, 5000],
  [6000, 9500],
  [6000, 7000],
  [0, 7000],
);

const arrowLeftRight: PathCommand[] = poly(
  [0, 5000],
  [2500, 500],
  [2500, 3000],
  [7500, 3000],
  [7500, 500],
  [E, 5000],
  [7500, 9500],
  [7500, 7000],
  [2500, 7000],
  [2500, 9500],
);

const arrowUpDown: PathCommand[] = poly(
  [5000, 0],
  [9500, 2500],
  [7000, 2500],
  [7000, 7500],
  [9500, 7500],
  [5000, E],
  [500, 7500],
  [3000, 7500],
  [3000, 2500],
  [500, 2500],
);

/** 꺾인 화살표 — 아래에서 올라와 오른쪽으로 꺾인다. 안쪽 모서리가 오목이다. */
const arrowElbow: PathCommand[] = poly(
  [0, E],
  [0, 1000],
  [7000, 1000],
  [7000, 0],
  [E, 2500],
  [7000, 5000],
  [7000, 4000],
  [3000, 4000],
  [3000, E],
);

/**
 * 곡선 화살표 — **카탈로그에서 유일하게 열린 도형이다**(`Z` 가 없다).
 *
 * 그 사실이 두 자리에서 값을 한다. 하나, 채우면 그리지 않은 변이 생기므로 씨앗 스타일이
 * 선만 심는다(`newPathElement`). 둘, 그리기 기록에 `closePath` 가 **없어야** 하며 그것이
 * "언제나 닫는다" 는 결함을 잡는 유일한 고정 입력이다(시험 규율 D4).
 */
const arrowCurved: PathCommand[] = [
  m(800, 9500),
  c(1600, 3800, 4800, 1500, 9200, 2400),
  m(7600, 600),
  l(9200, 2400),
  l(7100, 4000),
];

/** 갈매기 — 뒤가 파인 띠. 단계와 위상은 같고 비례가 다르다. */
const chevron: PathCommand[] = poly(
  [1500, 0],
  [4500, 0],
  [9000, 5000],
  [4500, E],
  [1500, E],
  [6000, 5000],
);

/** 홈 화살표 — 꼬리에 V 홈이 파인 화살표. */
const arrowNotched: PathCommand[] = poly(
  [0, 3000],
  [6000, 3000],
  [6000, 500],
  [E, 5000],
  [6000, 9500],
  [6000, 7000],
  [0, 7000],
  [1500, 5000],
);

/** 화살촉 — 꼬리가 파인 다트. 파임이 없으면 삼각형과 같은 도형이 된다. */
const arrowHead: PathCommand[] = poly([0, 0], [E, 5000], [0, E], [3000, 5000]);

// --- 표 ----------------------------------------------------------------

/** 이름 키의 앞자리. **키 이름 안에 점을 넣지 않는다** — 마디로 나눈다. */
const NAME_KEY_PREFIX = 'dashboard.canvas.edit.catalogNames';

function entry(id: string, group: ShapeGroupId, path: PathCommand[]): ShapeCatalogEntry {
  return Object.freeze({
    id,
    group,
    nameKey: `${NAME_KEY_PREFIX}.${id}`,
    path: Object.freeze(path.map((cmd) => Object.freeze({ ...cmd }))),
  });
}

/**
 * 카탈로그 묶음 셋. **화면 차례가 곧 이 배열의 차례다** — 같은 것을 두 자리에서 다른
 * 순서로 내면 사용자가 두 목록을 따로 외워야 한다.
 */
export const SHAPE_GROUPS: readonly ShapeCatalogGroup[] = Object.freeze([
  Object.freeze({
    id: 'general' as const,
    titleKey: 'dashboard.canvas.edit.paletteGroupGeneral',
    entries: Object.freeze([
      entry('roundedRect', 'general', roundedRect),
      entry('process', 'general', process),
      entry('document', 'general', document_),
      entry('cylinder', 'general', cylinder),
      entry('cube', 'general', cube),
      entry('card', 'general', card),
      entry('note', 'general', note),
      entry('step', 'general', step),
      entry('callout', 'general', callout),
      entry('actor', 'general', actor),
    ]),
  }),
  Object.freeze({
    id: 'basic' as const,
    titleKey: 'dashboard.canvas.edit.paletteGroupBasic',
    entries: Object.freeze([
      entry('triangle', 'basic', triangle),
      entry('rightTriangle', 'basic', rightTriangle),
      entry('diamond', 'basic', diamond),
      entry('parallelogram', 'basic', parallelogram),
      entry('trapezoid', 'basic', trapezoid),
      entry('pentagon', 'basic', pentagon),
      entry('hexagon', 'basic', hexagon),
      entry('octagon', 'basic', octagon),
      entry('cross', 'basic', cross),
      entry('star4', 'basic', star4),
      entry('star5', 'basic', star5),
      entry('cloud', 'basic', cloud),
    ]),
  }),
  Object.freeze({
    id: 'arrow' as const,
    titleKey: 'dashboard.canvas.edit.paletteGroupArrow',
    entries: Object.freeze([
      entry('arrowRight', 'arrow', arrowRight),
      entry('arrowLeftRight', 'arrow', arrowLeftRight),
      entry('arrowUpDown', 'arrow', arrowUpDown),
      entry('arrowElbow', 'arrow', arrowElbow),
      entry('arrowCurved', 'arrow', arrowCurved),
      entry('chevron', 'arrow', chevron),
      entry('arrowNotched', 'arrow', arrowNotched),
      entry('arrowHead', 'arrow', arrowHead),
    ]),
  }),
]);

/** 묶음을 편 30종. 순회하는 시험이 한 종도 빠뜨리지 않게 하는 자리다. */
export const SHAPE_CATALOG: readonly ShapeCatalogEntry[] = Object.freeze(
  SHAPE_GROUPS.flatMap((g) => [...g.entries]),
);

/**
 * id 로 도형을 찾는다. **없으면 `undefined`** — 렌더는 이 함수를 부르지 않으므로(경로는
 * 값으로 실려 있다) 못 찾는 것이 그림을 망가뜨리지 않는다. 쓰는 곳은 요소 목록이 행에
 * 이름을 보일 때뿐이다.
 */
export function findShape(id: string | undefined): ShapeCatalogEntry | undefined {
  if (id === undefined) return undefined;
  return SHAPE_CATALOG.find((e) => e.id === id);
}
