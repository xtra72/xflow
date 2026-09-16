// 캔버스 패널 config 타입 + 관용 파서 (SPEC-CANVAS-001 T1).
//
// 캔버스 패널은 사용자가 저술한 도형 목록(elements)을 **정수 캔버스 좌표**로 들고 있다가
// Canvas 2D 로 그린다. 각 요소는 시리즈에 바인딩되어, 조건 규칙 표(first-match-wins)의
// 결과로 스타일·문구가 바뀐다(REQ-02/REQ-03/REQ-04).
//
// ## 좌표계가 정수인 이유 (SPEC-CANVAS-002 0.8.0)
//
// 001~002 는 좌표를 정규화(0..1) 분수로 들었다. 그 모델에서 격자는 **백분율**로만 말할 수
// 있었고, 백분율은 제 축 길이에 대한 값이라 정사각 칸을 얻으려면 세로 백분율을 스테이지
// 종횡비에서 파생해야 했다 — 1749×796 스테이지에서 그 값은 21.97% 가 되어 마지막 줄이
// **반 칸**으로 잘렸다. 자투리 칸은 계산 실수가 아니라 그 모델의 필연이었다.
//
// 캔버스 크기를 **정수 두 개**(기본 500 × 400)로 정하고 요소 기하를 그 단위의 정수로 두면
// 자투리가 사라진다: 10 단위 격자는 50 × 40 칸으로 **나머지 없이** 떨어진다. 백분율은
// 사라지고(화면에 그릴 때만 `step / 캔버스 축 길이 × 100` 으로 한 번 환산된다), 그와 함께
// 도형 팔레트의 "비율" 어휘도 사라진다.
//
// 좌표는 여전히 **clamp 하지 않는다** — 캔버스 밖 저술은 합법이다(가정 A5). 정수인 것과
// 범위가 있는 것은 다른 축이며, 여기서 죄는 것은 **자릿수뿐**이다.
//
// config 는 기존 패널과 동일하게 Go 쪽에서 불투명 JSON 으로 영속되므로 백엔드 변경이
// 없다. 그래서 스키마가 002/003 을 거치며 자라도 구버전 패널이 깨지지 않으려면 읽기
// 경로에 관용 파서가 있어야 한다 — `parseCanvasConfig` 는 결측/손상/구버전 입력을
// **예외 없이** 기본값으로 보정한다(§위험 R3, `parseHeatmapConfig` 패턴).
//
// 데이터 소스 축에 대하여: `usePanelSeriesData` 는 파싱된 view 가 아니라 **원본 config
// 레코드**를 그대로 받는다(HeatmapPanel 선례). 다중 소스 배열(`sources[]`)·`data_sources`
// 같은 필드는 그 계약이 소유하므로 여기서는 보존하지 않는다 — 이 파서가 돌려주는 것은
// 캔버스가 그리기 위해 읽는 필드의 view 다.
//
// @spec SPEC-CANVAS-001

import {
  buildDefaultStoreSource,
  type ChartDataSourceKind,
  type ChartPanelConfigBase,
  type StoreSourceConfig,
  type TsdbSourceConfig,
} from '../charts/chartChannelTypes';
import {
  DEFAULT_PATH,
  MAX_PATH_COMMANDS,
  type PathCommand,
} from './shapes/pathTypes';
// 파선 무늬의 이름 넷과 그 판별(SPEC-CANVAS-012 M2). 아무것도 들이지 않는 잎이라 순환이
// 없고, 파서와 렌더가 **같은 표**를 본다 — 표를 `drawElement` 에 두면 파서가 유효한 이름을
// 판정하려고 렌더 모듈을 들이게 된다.
import { isStrokeDash, type StrokeDash } from './strokeDash';
// 각도의 정규화와 저장 규율(SPEC-CANVAS-014). 아무것도 들이지 않는 잎이라 순환이 없다.
import { storableDegrees } from './canvasRotation';
// 임의 앵커의 자료형. 잎 모듈(`shapes/pathTypes` 밖을 들이지 않는다)이라 순환이 없다.
import type { CustomAnchor } from './connector/anchorTypes';
// 연결선의 자료형과 그 판별(SPEC-CANVAS-011 M4). 저쪽이 이 파일에서 **타입만** 가져가므로
// 값(`CONNECTOR_KIND` · `isConnectorKind`)을 들여도 실행 시각 순환이 생기지 않는다.
import {
  CONNECTOR_KIND,
  DEFAULT_CONNECTOR_ROUTE,
  isConnectorKind,
  MAX_CONNECTOR_POINTS,
  routeHosting,
  type ConnectorElement,
  type ConnectorEnd,
  type ConnectorRoute,
} from './connector/connectorTypes';
// 타입만 가져온다 — 런타임 의존이 없으므로 `group/groupTypes.ts` 와의 순환이 생기지
// 않는다(저쪽도 이 파일에서 타입만 가져간다).
import type { CanvasNode, GroupElement, SymbolStamp } from './group/groupTypes';

// --- 기본값 상수 ---------------------------------------------------------

/** `{value}` 치환 시 기본 소수 자리(명세 §문구 템플릿). */
export const DEFAULT_DECIMALS = 1;
/** 선 두께 기본값(px). 손상된 strokeWidth 가 들어왔을 때의 폴백이기도 하다. */
export const DEFAULT_STROKE_WIDTH = 1;
/** 글자 크기 기본값(px). 손상된 fontSize 가 들어왔을 때의 폴백이기도 하다. */
export const DEFAULT_FONT_SIZE = 14;
/** 불투명도 기본값. 손상된 opacity 가 들어왔을 때의 폴백이다. */
export const DEFAULT_OPACITY = 1;
/** 신규 패널의 기본 트윈 지속 시간(ms). */
export const DEFAULT_TWEEN_DURATION_MS = 300;
/** 신규 패널의 기본 이징. */
export const DEFAULT_TWEEN_EASING: TweenEasing = 'ease-out';

/**
 * 신규 패널의 캔버스 크기(정수 캔버스 단위).
 *
 * 500 × 400 인 것에 두 가지 뜻이 있다. 하나는 **가로가 조금 긴 판**이라 대시보드 패널의
 * 흔한 모양에 가깝다는 것이고, 다른 하나는 두 축이 **100 의 배수**라는 것이다 — 고를 수
 * 있는 격자 간격이 모두 100 의 약수이므로(`CANVAS_GRID_STEP_CHOICES`), 기본 크기에서는
 * 어떤 간격을 골라도 두 축이 나머지 없이 떨어진다.
 */
export const DEFAULT_CANVAS_SIZE: Readonly<CanvasSize> = { width: 500, height: 400 };

/**
 * 캔버스 한 축의 최소 길이(단위). 0 이하는 투영이 성립하지 않는다(0 으로 나눈다).
 * 파서는 그런 값을 받으면 기본 크기로 떨어뜨린다.
 */
export const MIN_CANVAS_DIMENSION = 1;

/**
 * 캔버스 한 축의 최대 길이(단위). 좌표가 이보다 커진다고 그림이 달라지지는 않지만
 * (투영은 비율이다), 편집기의 수치 칸이 감당할 자릿수에 상한이 있어야 한다.
 */
export const MAX_CANVAS_DIMENSION = 100000;

/**
 * 도형이 가질 수 있는 **최소 크기**(단위) — 곧 "퇴화" 의 경계다.
 *
 * 정수 좌표계에서 폭·높이 0 은 화면에서 사라진다는 뜻이고, 사라진 요소는 사용자가
 * **찾을 수 없어 고칠 수도 없다**. 그래서 읽는 쪽(파서)은 퇴화 기하를 씨앗 기하로
 * 대체하고, 쓰는 쪽(`patchNodeGeometry`)은 아예 만들지 않는다. 두 규율이 같은 상수를
 * 보므로 "읽을 때는 살아나는데 끌면 다시 사라진다" 가 생기지 않는다.
 */
export const MIN_ELEMENT_EXTENT = 1;

/**
 * rect/ellipse 기하가 손상·퇴화했을 때 대신 쓰는 좌상단+크기(정수 캔버스 단위).
 *
 * 기본 캔버스(500×400)의 10% 자리에서 20% 크기 — 정규화 시절의 `0.1/0.1/0.2/0.2` 를 그
 * 크기로 옮겨 적은 값이라 화면에 나타나는 모습이 종전과 같다.
 */
export const DEFAULT_BOX_GEOMETRY: Readonly<BoxGeometry> = { x: 50, y: 40, w: 100, h: 80 };
/** line 기하가 손상·퇴화했을 때 대신 쓰는 두 끝점(정수 캔버스 단위). */
export const DEFAULT_LINE_GEOMETRY: Readonly<LineGeometry> = { x1: 50, y1: 200, x2: 450, y2: 200 };
/** text 기하가 손상됐을 때 대신 쓰는 정렬 기준점(정수 캔버스 단위). */
export const DEFAULT_POINT_GEOMETRY: Readonly<PointGeometry> = { x: 250, y: 200 };

// --- 타입 ---------------------------------------------------------------

/**
 * 사용자가 **직접 고를 수 있는** 원시형 4종(001 REQ-02).
 *
 * 팔레트가 내는 것도, 목록 편집기의 "종류 바꾸기" 가 받는 것도 이 넷이다. 경로가 여기
 * 없는 것이 008 REQ-07 의 금지 조항("`path` 를 종류 바꾸기 선택지에 넣지 않는다")이며,
 * 그 금지를 **타입으로** 세워 두면 다섯 번째 단추나 다섯 번째 선택지가 실수로 생길 수
 * 없다 — 배열에 한 줄 더하는 것만으로는 컴파일러가 울지 않지만, 그 배열의 원소 타입이
 * 이것이면 운다.
 *
 * 경로를 무엇으로부터 지어낼지에 대한 답이 없다는 것이 그 금지의 근거다(어떤 명령
 * 목록을 발명할 것인가). 반대 방향 — 경로에서 다른 종류로 바꾸는 길은 열려 있다.
 */
export type CanvasPrimitiveKind = 'rect' | 'ellipse' | 'line' | 'text';

/**
 * **모든** 요소 종류. `CanvasElement['kind']` 와 같은 집합이어야 한다.
 *
 * 원시형 넷과 갈라 둔 것이 008 이 이 타입에 한 일의 전부다. 종전에는 한 이름이 두 뜻을
 * 겸했다 — "요소가 가질 수 있는 종류" 와 "사용자가 고를 수 있는 종류". 다섯 번째 종류가
 * 들어오는 순간 그 겸직은 침묵을 만든다: 이름이 넷을 뜻하는 채로 다섯 번째 요소의
 * `kind` 를 받으면, 그 요소를 다루는 자리들이 타입 오류 없이 넷 중 하나로 읽힌다.
 */
export type CanvasElementKind = CanvasPrimitiveKind | 'path';

/** 비교 연산자 집합(명세 §비교 연산자 집합). `nodata` 는 결측 판정이다. */
export type RuleOp = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne' | 'between' | 'nodata';

/** 트윈 이징 4종(명세 §트윈 명세). */
export type TweenEasing = 'linear' | 'ease-in' | 'ease-out' | 'ease-in-out';

/** 글자 굵기. 보간이 성립하지 않아 트윈 없이 즉시 전환되는 속성이다. */
export type ElementFontWeight = 'normal' | 'bold';

/** 텍스트 정렬 기준. `measureText` 로 잰 폭을 이 기준으로 배치한다(REQ-02). */
export type ElementAlign = 'left' | 'center' | 'right';

/** 바인딩 집계. 001 은 최신값만 쓴다 — 다른 집계는 후속 SPEC. */
export type ElementBindingAgg = 'last';

/**
 * 캔버스 좌표계의 크기(정수 단위). 요소 기하는 **이 단위의 정수**이며, 화면 px 로 가는
 * 길은 `canvasGeometry` 의 투영 하나뿐이다(축마다 `스테이지 px / 캔버스 단위` 를 곱한다).
 */
export interface CanvasSize {
  width: number;
  height: number;
}

/** rect | ellipse 기하 — 좌상단 + 크기. 모두 정수 캔버스 좌표. */
export interface BoxGeometry {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** line 기하 — 두 끝점. 모두 정수 캔버스 좌표. */
export interface LineGeometry {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** text 기하 — 정렬 기준점. 정수 캔버스 좌표. */
export interface PointGeometry {
  x: number;
  y: number;
}

/**
 * 도형별 기하 합집합. 어느 갈래인지는 `CanvasElement.kind` 가 판별한다 —
 * 요소를 `kind` 로 좁히면 `geometry` 도 함께 좁혀지므로 소비 측(canvasGeometry.ts /
 * drawElement.ts)이 `'w' in geo` 같은 형상 검사를 하지 않아도 된다.
 */
export type Geometry = BoxGeometry | LineGeometry | PointGeometry;

/**
 * 기하가 **저장된 상자**인가 — 011 A11("임의 앵커는 상자형에만 선다")의 판정이다.
 *
 * 코드베이스의 관용은 `'w' in geo` 를 쓰는 자리에 바로 적는 것이고, 여기만 이름을 갖는
 * 데에는 이유가 둘 있다. 하나는 A11 이 **하나의 규칙**이라 이름이 있어야 쓰기 경로와
 * 파서가 같은 것을 말한다고 보일 수 있다는 것. 다른 하나는 `connector/anchors.ts` 가
 * 자리 이름 리터럴을 `'c'` 하나로 못박은 가드(AC-11) 아래 있어 그 파일 안에서는
 * `'w'` 라는 **글자**를 적을 수 없다는 것이다 — 가드를 피하려고 이름을 짓는 것이 아니라,
 * 가드가 그 파일을 자리 이름 전용으로 못박아 두었으므로 형상 판정은 밖에 산다.
 *
 * 문구(`PointGeometry`)와 선(`LineGeometry`)이 여기서 떨어지는 것이 A11 의 전부다.
 */
export function isBoxGeometry(geo: Geometry): geo is BoxGeometry {
  return 'w' in geo;
}

/**
 * 요소의 기본 스타일. 모든 필드가 옵셔널이며, 미지정은 "렌더측 기본" 을 뜻한다
 * (히트맵 `ContourLineStyle` 선례). 파서는 미지정 필드를 만들어 채우지 않는다 —
 * 채우면 "지정 안 함" 과 "값이 우연히 기본값과 같음" 이 구분되지 않는다.
 */
export interface ElementStyle {
  fill?: string;
  stroke?: string;
  /** 선 두께(px). */
  strokeWidth?: number;
  /**
   * 파선 무늬 (SPEC-CANVAS-012 REQ-04).
   *
   * `strokeWidth` 바로 뒤가 자리다 — 둘 다 **선을 어떻게 긋는가**를 정하고, 무늬가 두께의
   * 배수로 나오므로(`strokeDash.dashPattern`) 읽는 사람이 둘을 함께 본다.
   *
   * **미지정과 `'solid'` 는 다른 값이다**(같은 그림이지만). 그 구분은 `visible` 이 3지
   * 선택인 것과 같은 판단이다 — 잎 모듈 `strokeDash.ts` 의 `StrokeDash` 주석 참조.
   */
  strokeDash?: StrokeDash;
  /** 0..1. */
  opacity?: number;
  /** 글자 크기(px). */
  fontSize?: number;
  fontWeight?: ElementFontWeight;
  textColor?: string;
  align?: ElementAlign;
  /** 기본 true(미지정 = 보임). */
  visible?: boolean;
}

/** 요소가 참조하는 시리즈. `series` 는 로케일 비의존 동일성 키다(REQ-03). */
export interface ElementBinding {
  series: string;
  agg: ElementBindingAgg;
}

/** 규칙 행의 임계값. `between` 만 2원소 튜플이다. */
export type RuleValue = number | [number, number];

/**
 * 일치한 행이 기본 스타일 위에 덮어쓰는 패치. 명세가 든 패치 가능 속성은
 * fill · stroke · strokeWidth · opacity · textColor · fontWeight · visible · text 이며,
 * 타입은 그보다 넓은 `Partial<ElementStyle>` 이다 — 좁히면 003 이 기하·정렬을 패치
 * 대상에 넣을 때 타입을 다시 바꿔야 한다. 설정 UI 가 노출 범위를 좁히는 축이다.
 */
export type StylePatch = Partial<ElementStyle> & { text?: string };

/** 규칙 표의 한 행. 표의 위에서부터 처음 일치한 행 하나만 이긴다(REQ-04). */
export interface RuleRow {
  op: RuleOp;
  /** `nodata` 행에서는 평가에 쓰이지 않는다(파서가 0 으로 정규화한다). */
  value: RuleValue;
  patch: StylePatch;
}

/** 상태 전이 트윈 명세. `duration_ms: 0` 이면 즉시 전환이며 루프를 깨우지 않는다. */
export interface TweenSpec {
  duration_ms: number;
  easing: TweenEasing;
}

/** 도형 종류와 무관한 요소 공통 필드. */
export interface CanvasElementBase {
  /** 안정 식별자(규칙/설정 UI 가 참조). 배열 안에서 유일하다. */
  id: string;
  /** 기본 스타일. */
  style: ElementStyle;
  /** 기본 문구 템플릿. kind:'text' 는 필수이나 파서는 결측을 탈락 사유로 보지 않는다. */
  text?: string;
  /**
   * 바인딩 판독값을 **숫자로 읽을 것인가**.
   *
   * `true`(또는 **부재**)면 판독값을 숫자로 보고 `decimals` 자리로 반올림한 뒤 `{unit}` 을
   * 붙인다 — 001 이래의 유일한 동작이다. `false` 면 받은 값을 **문자열 그대로** 내보낸다:
   * 반올림도 단위도 걸리지 않으므로 `decimals` · `unit` 은 그 상태에서 뜻이 없다.
   *
   * **부재가 곧 숫자다.** 그래서 이 필드가 없는 기존 config 는 한 픽셀도 달라지지 않는다.
   * 그 규칙은 아래 `isNumericElement` 한 곳에만 적혀 있어야 한다 — 소비 측이 저마다
   * `!el.numeric` 처럼 참 판정을 하면 부재가 조용히 "숫자 아님" 으로 뒤집힌다.
   */
  numeric?: boolean;
  /** `{value}` 치환 시 소수 자리. `numeric: false` 에서는 쓰이지 않는다. 미지정이면 `DEFAULT_DECIMALS`. */
  decimals?: number;
  /** `{unit}` 치환 값. `numeric: false` 에서는 쓰이지 않는다. */
  unit?: string;
  /** 없으면 정적 도형(규칙 평가 대상이 아니다). */
  binding?: ElementBinding;
  /** 위에서부터 첫 일치 승리. 유효 행이 하나도 없으면 미지정과 같다. */
  rules?: RuleRow[];
  /** 패널 기본 트윈을 덮어쓴다. */
  tween?: TweenSpec;
  /**
   * 임의 앵커. 좌표는 **요소 로컬 정수 격자**(0..`ANCHOR_LOCAL_EXTENT`)다(A10).
   *
   * **상자형(rect · ellipse · path)에만 선다**(A11). 선에는 상자가 없고, 문구의 상자는
   * 글자를 재어 나온 값이라 그 위에 저술값을 얹으면 앵커가 측정에 딸려 조용히 미끄러진다.
   * 그런데 이 필드는 공통 베이스에 있으므로 **타입으로는 선·문구에도 앉는다** — 그
   * 구멍을 파서(`parseElement` 의 상자형 갈래에서만 붙인다)와 쓰기 경로
   * (`addAnchorAt` 의 상자 판정)가 **양쪽에서** 막는다. 한쪽만 막으면 다른 쪽으로 들어온
   * 값이 한 번은 그려지고 다음 읽기에서 사라진다.
   */
  anchors?: CustomAnchor[];
  /**
   * 회전 각도 — **정수 도**, `[0, 360)`, 양수는 시계 방향 (SPEC-CANVAS-014 REQ-01).
   *
   * **0 은 저장되지 않는다**(§결정 7). 미지정과 0 은 같은 그림이고, 파서가 0 을 만들어
   * 채우면 왕복 한 번에 모든 요소가 키를 하나씩 얻는다.
   *
   * **`kind:'line'` 에는 서지 않는다.** 선의 임의 각도는 두 끝점으로 표현되므로 필드를
   * 주면 같은 그림을 두 가지로 적을 수 있게 된다 — `anchors` 가 상자형에만 서는 것(A11)과
   * 같은 규율이고, 그 금지도 같은 자리(파서의 갈래)가 진다.
   *
   * 라디안이 아닌 까닭은 `canvasRotation.ts` 머리말이 진다.
   */
  rotation?: number;
}

/** 사각형 요소. */
export interface RectElement extends CanvasElementBase {
  kind: 'rect';
  geometry: BoxGeometry;
}

/** 타원 요소. */
export interface EllipseElement extends CanvasElementBase {
  kind: 'ellipse';
  geometry: BoxGeometry;
}

/** 선 요소. */
export interface LineElement extends CanvasElementBase {
  kind: 'line';
  geometry: LineGeometry;
}

/** 문구 요소. */
export interface TextElement extends CanvasElementBase {
  kind: 'text';
  geometry: PointGeometry;
}

/**
 * 경로 요소 — 임의의 닫힌(또는 열린) 윤곽 하나(SPEC-CANVAS-008 REQ-01).
 *
 * **기하가 `BoxGeometry` 인 것이 이 모델의 절반이다.** rect 와 같은 형상이므로 8핸들 크기
 * 조절 · 정렬 · 붙임 · 무리 이동 · 방향키 미세 이동이 한 줄도 바뀌지 않고 경로에 걸린다 —
 * 그 전부가 이미 `BoxGeometry` 위에 서 있다. 대가는 하나뿐이고 숨기지 않는다: 비균등하게
 * 늘리면 윤곽이 함께 일그러진다(원이 타원이 되듯).
 *
 * 한 요소가 **하나의 윤곽**이고 그 윤곽 전체가 **하나의 스타일**을 입는다. 그래서 규칙 표는
 * 경로의 **안쪽을 지목하지 못한다** — 상태를 나르는 조립체는 004 의 `group` 이 맡는다.
 */
export interface PathElement extends CanvasElementBase {
  kind: 'path';
  /** 정수 캔버스 단위. rect 와 **같은** 형상이다. */
  geometry: BoxGeometry;
  /** 요소 상자 로컬 정수(공칭 0..`PATH_LOCAL_EXTENT`). clamp 하지 않는다. */
  path: PathCommand[];
  /**
   * 어느 카탈로그 도형에서 나왔는가. **표시·감사용이며 렌더 경로는 읽지 않는다** —
   * 결측이거나 모르는 값이어도 그림은 완전하다(004 REQ-02 의 `symbol` 출처 기록과 같은 규율).
   */
  catalog_id?: string;
}

/** 캔버스 요소 — `kind` 로 판별하는 합집합. */
export type CanvasElement =
  | RectElement
  | EllipseElement
  | LineElement
  | TextElement
  | PathElement;

/**
 * 캔버스 패널 config. 데이터 소스 축은 기존 차트 패널과 동일한
 * `ChartPanelConfigBase` 를 그대로 확장한다(신규 데이터 경로 없음, REQ-03).
 */
export interface CanvasPanelConfig extends ChartPanelConfigBase {
  /**
   * 캔버스 좌표계의 크기(정수 단위). **파싱된 view 에는 언제나 있다** — 투영이 이 값
   * 없이는 성립하지 않으므로 "미지정" 이라는 상태를 만들지 않는다(결측 config 는 파서가
   * `DEFAULT_CANVAS_SIZE` 로 채운다).
   */
  canvas: CanvasSize;
  /** 캔버스 배경색. 미지정이면 패널 표면색. */
  background?: string;
  /** 패널 기본 트윈. 요소가 덮어쓸 수 있다. */
  tween?: TweenSpec;
  /**
   * 배열 순서 = 그리기 순서(뒤가 위). 001 의 유일한 z-order 수단.
   *
   * **원소 타입이 `CanvasNode` 로 넓어진 것이 004 가 이 형상에 한 일의 전부다**(REQ-01).
   * `CanvasElement` 유니온은 한 글자도 바뀌지 않으며, 그룹은 그 유니온 **바깥**에서
   * 합쳐진다 — 그래야 `parts: CanvasElement[]` 가 그룹 중첩을 타입으로 막는다(A6).
   */
  elements: CanvasNode[];
}

// --- 원시 값 보정 도우미 -------------------------------------------------

/** 유한 숫자인지 확인한다(NaN/Infinity 방어). */
function isFiniteNumber(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

/** 객체가 아니면 빈 레코드로 본다(필드 접근을 예외 없이 만드는 죔쇠). */
function asRecord(raw: unknown): Record<string, unknown> {
  return raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
}

/** 값을 [0,1] 로 clamp 한다. */
function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/**
 * 유한 숫자면 **정수로 반올림**해서, 아니면 폴백. 기하 좌표 보정에 쓴다.
 *
 * 반올림이 이 함수가 하는 일의 전부다 — **범위는 죄지 않는다**. 캔버스 밖 좌표는 합법인
 * 저술이며(가정 A5), 잘라내면 사용자 의도가 조용히 바뀐다. 정수화는 그와 다른 축이다:
 * 좌표계 자체가 정수라 소수 자리는 저장할 곳이 없고, 남겨 두면 격자·붙임·수치 칸이
 * 저마다 다른 반올림을 하게 된다.
 *
 * **내보내는 이유**: SVG 가져오기(007)가 사용자 단위를 경로 로컬 정수로 옮길 때 같은
 * 규율을 써야 한다. 그 모듈이 제 반올림을 지어내면 이 파일에 **두 번째 수치 규율**이
 * 생기고, 둘이 갈라지는 날 "읽을 때와 쓸 때가 다르다" 가 시작된다.
 */
export function coordinate(v: unknown, fallback: number): number {
  return isFiniteNumber(v) ? Math.round(v) : fallback;
}

/**
 * 캔버스 한 축의 길이. 유한 정수 양수만 통과하며, 그 밖은 **기본 크기로 떨어뜨린다**.
 *
 * 0 이하를 폴백으로 보내는 것이 요점이다 — 0 축은 투영에서 0 으로 나누는 자리이고,
 * 그 결과(NaN)는 모든 요소를 화면에서 지운다.
 */
function dimension(v: unknown, fallback: number): number {
  if (!isFiniteNumber(v)) return fallback;
  const n = Math.round(v);
  if (n < MIN_CANVAS_DIMENSION) return fallback;
  return Math.min(n, MAX_CANVAS_DIMENSION);
}

/**
 * 캔버스 크기. 결측·손상은 축마다 따로 기본값으로 떨어진다 — 한 축이 깨졌다고 멀쩡한
 * 다른 축까지 버리면 사용자가 정한 값이 이유 없이 사라진다.
 */
export function parseCanvasSize(raw: unknown): CanvasSize {
  const c = asRecord(raw);
  return {
    width: dimension(c.width, DEFAULT_CANVAS_SIZE.width),
    height: dimension(c.height, DEFAULT_CANVAS_SIZE.height),
  };
}

/**
 * 폭·높이가 화면에서 사라지는 크기인가(퇴화).
 *
 * 정수 좌표계에서 0 은 "아주 얇음" 이 아니라 **없음**이다. 음수도 함께 잡는다 — 001 의
 * 렌더 층은 음수 크기 박스를 견디지만(읽기 경로의 견고성), 그 견고성은 그려질 무언가가
 * 있을 때의 이야기이고 여기서 걸러지는 것은 애초에 그려지지 않는 값이다.
 */
export function isDegenerateBox(g: BoxGeometry): boolean {
  return !(g.w >= MIN_ELEMENT_EXTENT) || !(g.h >= MIN_ELEMENT_EXTENT);
}

/** 두 끝점이 같은 선인가(퇴화). 길이 0 선은 어떤 두께로도 그려지지 않는다. */
export function isDegenerateLine(g: LineGeometry): boolean {
  return g.x1 === g.x2 && g.y1 === g.y2;
}

/** 값이 아예 없는가(부재). null 도 부재로 본다 — JSON 왕복에서 흔한 형태다. */
function isAbsent(v: unknown): boolean {
  return v === undefined || v === null;
}

/** 비어 있지 않은 문자열만 통과시킨다. 빈 문자열은 "지정 안 함" 과 같다. */
function optionalString(v: unknown): string | undefined {
  return typeof v === 'string' && v.trim() !== '' ? v : undefined;
}

/**
 * 부재는 미지정(undefined)으로, **있는데 손상된 값**(비유한·음수·타입 불일치)은
 * 폴백으로 보정한다. 손상 값을 조용히 지우면 사용자가 화면에서 원인을 볼 수 없고,
 * 부재를 기본값으로 채우면 "지정 안 함" 이 사라진다 — 두 경우를 갈라 두는 이유다.
 */
function optionalNonNegative(v: unknown, fallback: number): number | undefined {
  if (isAbsent(v)) return undefined;
  return isFiniteNumber(v) && v >= 0 ? v : fallback;
}

/** 부재는 미지정, 유한 숫자는 0..1 clamp, 손상 값은 기본 불투명도로 보정한다. */
function optionalOpacity(v: unknown): number | undefined {
  if (isAbsent(v)) return undefined;
  return isFiniteNumber(v) ? clamp01(v) : DEFAULT_OPACITY;
}

// --- 부분 파서 -----------------------------------------------------------

/**
 * 기하 손상 정책: **요소를 버리지 않고 기본 기하로 대체한다**(필드 단위 폴백).
 *
 * 좌표 한 칸이 손상됐다고 요소를 버리면 그 요소의 스타일·규칙·바인딩이 저장 왕복
 * 한 번에 사라진다 — 사용자가 되돌릴 수 없는 손실이다. 기본 기하로 대체하면 요소가
 * 화면에 남아 사용자가 보고 고칠 수 있다. 요소를 버리는 것은 **정체성이 없을 때**
 * (id 결측·미지 kind)로 한정한다. 좌표는 캔버스 안으로 clamp 하지 않는다 — 캔버스 밖으로
 * 일부 걸치는 배치도 뜻이 있는 저술이며, 잘라내면 사용자 의도가 조용히 바뀐다.
 *
 * ## 퇴화는 필드 폴백만으로 막을 수 없다 (정수 좌표계)
 *
 * 필드 폴백은 **결측·비유한**만 잡는다. 정수 좌표계에서는 그 그물을 빠져나가는 두 번째
 * 경로가 있다: 값이 멀쩡한 숫자인데 **반올림하면 0 이 되는** 경우다. 옛 0..1 좌표가 정확히
 * 그렇다 — `Math.round(0.2)` 는 0 이므로 옛 사각형은 크기 0 짜리 **보이지 않는** 요소가
 * 된다. 자리가 틀린 요소는 사용자가 찾아서 고칠 수 있지만 보이지 않는 요소는 그럴 수
 * 없으므로, 보이지 않는 편이 더 나쁘다. 그래서 반올림 **뒤에** 한 번 더 보고, 퇴화했으면
 * 기하 전체를 씨앗 기하로 바꾼다(필드 하나만 고치면 나머지 옛 값과 뒤섞여 아무 데도 아닌
 * 자리가 나온다).
 */
function parseBoxGeometry(raw: unknown): BoxGeometry {
  const g = asRecord(raw);
  const box: BoxGeometry = {
    x: coordinate(g.x, DEFAULT_BOX_GEOMETRY.x),
    y: coordinate(g.y, DEFAULT_BOX_GEOMETRY.y),
    w: coordinate(g.w, DEFAULT_BOX_GEOMETRY.w),
    h: coordinate(g.h, DEFAULT_BOX_GEOMETRY.h),
  };
  return isDegenerateBox(box) ? { ...DEFAULT_BOX_GEOMETRY } : box;
}

/** line 기하. 손상 필드는 기본 끝점으로, 길이 0 은 기본 선으로 대체한다(위 정책 동일). */
function parseLineGeometry(raw: unknown): LineGeometry {
  const g = asRecord(raw);
  const line: LineGeometry = {
    x1: coordinate(g.x1, DEFAULT_LINE_GEOMETRY.x1),
    y1: coordinate(g.y1, DEFAULT_LINE_GEOMETRY.y1),
    x2: coordinate(g.x2, DEFAULT_LINE_GEOMETRY.x2),
    y2: coordinate(g.y2, DEFAULT_LINE_GEOMETRY.y2),
  };
  return isDegenerateLine(line) ? { ...DEFAULT_LINE_GEOMETRY } : line;
}

/**
 * text 기하. 손상 필드는 기본 기준점으로 대체한다(위 정책 동일).
 *
 * **퇴화 검사가 없는 것이 옳다** — 기준점에는 크기가 없어 0 이 될 수 있는 넓이가 없다.
 * 옛 0..1 좌표는 여기서 캔버스의 왼쪽 위 모서리 근처(0 또는 1 단위)로 반올림되는데,
 * 그것은 자리가 틀린 것이지 사라진 것이 아니므로 사용자가 보고 고칠 수 있다.
 */
function parsePointGeometry(raw: unknown): PointGeometry {
  const g = asRecord(raw);
  return {
    x: coordinate(g.x, DEFAULT_POINT_GEOMETRY.x),
    y: coordinate(g.y, DEFAULT_POINT_GEOMETRY.y),
  };
}

/**
 * 스타일을 파싱한다. enum 은 화이트리스트로 좁히고(미인정 값은 미지정으로 떨어진다),
 * 수치는 위 도우미의 "부재 = 미지정 / 손상 = 폴백" 규율을 따른다.
 */
function parseStyle(raw: unknown): ElementStyle {
  const out: ElementStyle = {};
  if (!raw || typeof raw !== 'object') return out;
  const s = raw as Record<string, unknown>;

  const fill = optionalString(s.fill);
  if (fill !== undefined) out.fill = fill;
  const stroke = optionalString(s.stroke);
  if (stroke !== undefined) out.stroke = stroke;
  const textColor = optionalString(s.textColor);
  if (textColor !== undefined) out.textColor = textColor;

  const strokeWidth = optionalNonNegative(s.strokeWidth, DEFAULT_STROKE_WIDTH);
  if (strokeWidth !== undefined) out.strokeWidth = strokeWidth;
  const fontSize = optionalNonNegative(s.fontSize, DEFAULT_FONT_SIZE);
  if (fontSize !== undefined) out.fontSize = fontSize;
  const opacity = optionalOpacity(s.opacity);
  if (opacity !== undefined) out.opacity = opacity;

  // 모르는 값은 **키만** 버린다(012 REQ-06 · AC-09) — 요소를 통째로 떨어뜨리지 않는 것이
  // 001 이래의 파서 규율이고, 판별은 잎 모듈 한 자리에서만 이뤄진다.
  if (isStrokeDash(s.strokeDash)) out.strokeDash = s.strokeDash;

  if (s.fontWeight === 'bold' || s.fontWeight === 'normal') out.fontWeight = s.fontWeight;
  if (s.align === 'left' || s.align === 'center' || s.align === 'right') out.align = s.align;
  if (typeof s.visible === 'boolean') out.visible = s.visible;

  return out;
}

/**
 * 규칙 패치를 파싱한다. 스타일 축은 `parseStyle` 과 같은 규율이고, 문구는 빈 문자열도
 * 뜻이 있어(라벨 지우기) 길이 검사 없이 문자열이면 통과시킨다.
 */
function parsePatch(raw: unknown): StylePatch {
  const patch: StylePatch = parseStyle(raw);
  if (raw && typeof raw === 'object') {
    const t = (raw as Record<string, unknown>).text;
    if (typeof t === 'string') patch.text = t;
  }
  return patch;
}

/** 규칙 행 1건. 연산자·임계값이 성립하지 않으면 그 행을 버린다(null). */
function parseRuleRow(raw: unknown): RuleRow | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const r = raw as Record<string, unknown>;
  const op = r.op;

  // 결측 판정 행. value 는 평가에 쓰이지 않으므로 형상 유지를 위해 0 으로 정규화한다.
  if (op === 'nodata') {
    return { op, value: 0, patch: parsePatch(r.patch) };
  }
  // 구간 행. 2원소 유한 숫자 튜플이 아니면 판정이 성립하지 않으므로 버린다.
  if (op === 'between') {
    const v = r.value;
    if (!Array.isArray(v) || v.length !== 2) return null;
    if (!isFiniteNumber(v[0]) || !isFiniteNumber(v[1])) return null;
    // 순서는 저술 그대로 둔다 — 정렬은 평가기(canvasRules)의 몫이다.
    return { op, value: [v[0], v[1]], patch: parsePatch(r.patch) };
  }
  if (op === 'gt' || op === 'gte' || op === 'lt' || op === 'lte' || op === 'eq' || op === 'ne') {
    if (!isFiniteNumber(r.value)) return null;
    return { op, value: r.value, patch: parsePatch(r.patch) };
  }
  // 미지 연산자는 화면에서 조용히 다른 판정으로 바뀌는 것보다 버리는 편이 안전하다.
  return null;
}

/**
 * 규칙 표. 배열이 아니면 미지정이며, 유효 행이 하나도 남지 않아도 미지정이다 —
 * 빈 표와 표 없음은 렌더에서 완전히 같은 상태라 죽은 필드를 남길 이유가 없다.
 */
function parseRules(raw: unknown): RuleRow[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const rows: RuleRow[] = [];
  for (const item of raw) {
    const row = parseRuleRow(item);
    if (row) rows.push(row);
  }
  return rows.length > 0 ? rows : undefined;
}

/** 바인딩. 시리즈 키가 없으면 정적 도형이다. 001 의 집계는 'last' 뿐이다. */
function parseBinding(raw: unknown): ElementBinding | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const b = raw as Record<string, unknown>;
  const series = optionalString(b.series);
  if (series === undefined) return undefined;
  return { series, agg: 'last' };
}

/**
 * 트윈. 객체가 아니면 미지정(즉시 전환)이다. 지속 시간은 유한 비음수로 죄고,
 * 손상 값은 0(즉시 전환)으로 떨어뜨린다 — 임의의 기본 지속 시간을 넣으면 사용자가
 * 설정하지 않은 애니메이션이 생겨 루프가 깨어난다(REQ-05 유휴 정지).
 */
function parseTween(raw: unknown): TweenSpec | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const t = raw as Record<string, unknown>;
  const duration = isFiniteNumber(t.duration_ms) && t.duration_ms > 0 ? t.duration_ms : 0;
  const easing: TweenEasing =
    t.easing === 'ease-in' || t.easing === 'ease-out' || t.easing === 'ease-in-out'
      ? t.easing
      : 'linear';
  return { duration_ms: duration, easing };
}

/**
 * 경로 로컬 좌표 하나. 유한하지 않으면 `null` — **그 명령을 버린다**는 신호다.
 *
 * 기하 좌표와 규율이 갈리는 유일한 지점이다. 기하는 손상 필드를 기본값으로 **채우지만**
 * (요소가 화면에서 사라지지 않아야 하므로) 명령 하나의 좌표에는 채울 기본값이 없다 —
 * 지어낸 좌표를 끼워 넣으면 윤곽이 엉뚱한 곳으로 튀어 사용자가 무엇을 고쳐야 할지 모른다.
 * 그 명령만 빠지면 나머지 윤곽은 읽던 대로 이어진다.
 *
 * 반올림은 `coordinate()` 를 그대로 쓴다. 위 유한성 관문 때문에 폴백 인자에는 닿지
 * 않지만, **반올림 규율이 적히는 자리를 둘로 만들지 않는 것**이 이 재사용의 요점이다.
 */
function localCoordinate(v: unknown): number | null {
  if (!isFiniteNumber(v)) return null;
  return coordinate(v, 0);
}

/**
 * 경로 명령 1건. 성립하지 않으면 `null`(그 명령만 버린다).
 *
 * 모르는 명령 문자를 **버리는** 것이 규칙 행(`parseRuleRow`)의 미지 연산자 정책과 같다 —
 * 조용히 다른 명령으로 바꿔 읽는 것보다 빠뜨리는 편이 안전하다.
 */
function parsePathCommand(raw: unknown): PathCommand | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const r = raw as Record<string, unknown>;
  switch (r.c) {
    case 'Z':
      return { c: 'Z' };
    case 'M':
    case 'L': {
      const x = localCoordinate(r.x);
      const y = localCoordinate(r.y);
      if (x === null || y === null) return null;
      return { c: r.c, x, y };
    }
    case 'C': {
      const x1 = localCoordinate(r.x1);
      const y1 = localCoordinate(r.y1);
      const x2 = localCoordinate(r.x2);
      const y2 = localCoordinate(r.y2);
      const x = localCoordinate(r.x);
      const y = localCoordinate(r.y);
      if (x1 === null || y1 === null || x2 === null || y2 === null || x === null || y === null) {
        return null;
      }
      return { c: 'C', x1, y1, x2, y2, x, y };
    }
    default:
      return null;
  }
}

/** 씨앗 경로의 **사본**. 얼려 둔 원본을 흘려보내면 쓰는 쪽이 전역을 오염시킨다. */
function seedPath(): PathCommand[] {
  return DEFAULT_PATH.map((cmd) => ({ ...cmd }));
}

/**
 * 경로 명령 목록. **예외를 던지지 않으며 요소를 버리지도 않는다**(REQ-07).
 *
 * 씨앗으로 떨어지는 경우 셋과 그 이유:
 *   1. 배열이 아니다 — 읽을 것이 없다.
 *   2. 유효 명령이 하나도 남지 않았다 — 그릴 것이 없어 요소가 화면에서 사라진다.
 *   3. **첫 유효 명령이 `M` 이 아니다** — 앞에 `M(0,0)` 을 세우지 않는다. 세우면 저술한
 *      적 없는 변이 하나 생겨 화면에 정체 모를 형상이 나오고, 사용자는 그것이 제 저술인지
 *      파서가 지어낸 것인지 구분할 수 없다.
 *
 * 상한을 넘으면 **앞에서부터 상한까지만** 살린다(전부 버리면 도형이 사라진다). 세는 것은
 * 살아남은 명령이므로, 손상된 거대 배열도 한 프레임을 삼키지 않는다.
 */
function parsePathCommands(raw: unknown): PathCommand[] {
  if (!Array.isArray(raw)) return seedPath();
  const out: PathCommand[] = [];
  for (const item of raw) {
    const cmd = parsePathCommand(item);
    if (!cmd) continue;
    out.push(cmd);
    if (out.length >= MAX_PATH_COMMANDS) break;
  }
  if (out.length === 0) return seedPath();
  return out[0]?.c === 'M' ? out : seedPath();
}

/**
 * 임의 앵커 1건. 성립하지 않으면 `null`(그 **항목만** 버린다 — AC-29).
 *
 * 좌표 규율은 경로 명령과 같다(`localCoordinate`): 손상 좌표에는 채울 기본값이 없으므로
 * 지어내지 않고 항목을 뺀다. 0 으로 채우면 앵커가 상자 왼쪽 위 모서리로 조용히 이사하고,
 * 그 자리에 붙은 연결선은 예외 없이 엉뚱한 곳을 가리킨다.
 *
 * `id` 는 정체성이다. 없으면 연결선이 가리킬 수 없으므로 그 항목은 뜻이 없다.
 */
function parseCustomAnchor(raw: unknown): CustomAnchor | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const a = raw as Record<string, unknown>;
  const id = optionalString(a.id);
  if (id === undefined) return null;
  const x = localCoordinate(a.x);
  const y = localCoordinate(a.y);
  if (x === null || y === null) return null;
  return { id, x, y };
}

/**
 * 임의 앵커 목록. **예외를 던지지 않으며 요소를 버리지도 않는다**(REQ-07 규율).
 *
 * 배열이 아니면 미지정이고, 손상 항목은 그것만 빠지며(AC-29), id 중복은 **먼저 온 것이
 * 이긴다**(AC-30 — `parseElements` 와 같은 규율이다. 연결선이 id 로 앵커를 지목하므로
 * 중복이 남으면 어느 쪽을 가리키는지 정할 수 없다).
 *
 * 살아남은 항목이 없으면 **미지정으로 떨어뜨린다.** `[]` 를 남기면 "쓴 적 없음" 과
 * "다 지웠음" 이 구분되지 않고, 저장 왕복에 `anchors` 키가 새로 생긴다(AC-25).
 */
function parseCustomAnchors(raw: unknown): CustomAnchor[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out: CustomAnchor[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    const anchor = parseCustomAnchor(item);
    if (!anchor) continue;
    if (seen.has(anchor.id)) continue;
    seen.add(anchor.id);
    out.push(anchor);
  }
  return out.length > 0 ? out : undefined;
}

/** 요소 1건. 정체성(id · kind)이 성립하지 않으면 버린다(null). */
function parseElement(raw: unknown): CanvasElement | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const e = raw as Record<string, unknown>;

  const id = optionalString(e.id);
  if (id === undefined) return null;

  const kind = e.kind;
  // `'path'` 가 이 줄에 없으면 저장된 경로 요소는 **읽을 때 조용히 사라진다** — 예외도
  // 경고도 없이 사용자의 저술이 없어지는, 008 에서 가장 나쁜 실패다(위험 R2).
  if (
    kind !== 'rect' &&
    kind !== 'ellipse' &&
    kind !== 'line' &&
    kind !== 'text' &&
    kind !== 'path'
  ) {
    return null;
  }

  const decimals =
    isFiniteNumber(e.decimals) && e.decimals >= 0 ? Math.trunc(e.decimals) : undefined;
  // 불리언이 아닌 값(문자열 "false" · 0 · null …)은 **부재로 떨어뜨린다** — 손상 입력을
  // 임의로 참·거짓으로 읽으면 기존 대시보드의 표기가 조용히 뒤집힌다. 부재 = 숫자이므로
  // 그 폴백은 언제나 종전 동작이다.
  const numeric = typeof e.numeric === 'boolean' ? e.numeric : undefined;
  const unit = optionalString(e.unit);
  const binding = parseBinding(e.binding);
  const rules = parseRules(e.rules);
  const tween = parseTween(e.tween);

  const base: CanvasElementBase = {
    id,
    style: parseStyle(e.style),
    // 문구는 빈 문자열도 뜻이 있다(라벨 없음) — 길이 검사 없이 문자열이면 보존한다.
    ...(typeof e.text === 'string' ? { text: e.text } : {}),
    ...(numeric !== undefined ? { numeric } : {}),
    ...(decimals !== undefined ? { decimals } : {}),
    ...(unit !== undefined ? { unit } : {}),
    ...(binding !== undefined ? { binding } : {}),
    ...(rules !== undefined ? { rules } : {}),
    ...(tween !== undefined ? { tween } : {}),
  };

  // 임의 앵커는 **상자형 갈래에서만** 붙는다(A11 · AC-24). 선·문구 갈래에 이 조각이
  // 없는 것이 그 금지의 전부이며, 손으로 고친 config 가 문구에 `anchors` 를 적어 두어도
  // 읽는 순간 사라진다 — 남겨 두면 그 값이 한 번은 그려지고 글자 폭이 바뀌는 날 조용히
  // 미끄러진다.
  const anchors = parseCustomAnchors(e.anchors);
  const withAnchors = anchors !== undefined ? { anchors } : {};

  // 회전도 **갈래에서만** 붙는다(014 §결정 1 · AC-05). 선에 이 조각이 없는 것이 그 금지의
  // 전부다 — 선의 각도는 두 끝점이 이미 말하므로, 필드를 남겨 두면 같은 그림을 두 가지로
  // 적을 수 있게 되고 그때 어느 쪽이 참인지 화면이 답하지 못한다.
  //
  // **0 은 키를 만들지 않는다**(`storableDegrees`) — 014 이전 저장이 바이트 동일하게
  // 왕복하는 것이 그 조각의 값이다.
  const rotation = storableDegrees(e.rotation);
  const withRotation = rotation !== undefined ? { rotation } : {};

  switch (kind) {
    case 'rect':
      return {
        ...base,
        kind,
        geometry: parseBoxGeometry(e.geometry),
        ...withAnchors,
        ...withRotation,
      };
    case 'ellipse':
      return {
        ...base,
        kind,
        geometry: parseBoxGeometry(e.geometry),
        ...withAnchors,
        ...withRotation,
      };
    case 'line':
      return { ...base, kind, geometry: parseLineGeometry(e.geometry) };
    case 'path': {
      // `catalog_id` 는 **표시·감사용**이다. 렌더가 읽지 않으므로 결측이어도 그림은
      // 완전하고(REQ-01), 카탈로그에 없는 id 여도 그대로 보존한다 — 값으로 저장한 이상
      // "정의를 못 찾은 경로" 라는 실패 모드는 만들지 않는다.
      const catalogId = optionalString(e.catalog_id);
      return {
        ...base,
        kind,
        geometry: parseBoxGeometry(e.geometry),
        path: parsePathCommands(e.path),
        ...(catalogId !== undefined ? { catalog_id: catalogId } : {}),
        ...withAnchors,
        ...withRotation,
      };
    }
    default:
      return { ...base, kind, geometry: parsePointGeometry(e.geometry), ...withRotation };
  }
}

/**
 * 요소 목록. 배열이 아니면 빈 목록이고, 정체성이 없는 항목은 버린다.
 * id 중복은 **먼저 온 것이 이긴다** — 규칙·설정 UI 가 id 로 요소를 지목하므로
 * 중복이 남으면 어느 쪽을 가리키는지 정할 수 없다.
 *
 * **내보내는 이유는 스크래치패드다**(SPEC-CANVAS-008 REQ-05). 저장된 항목의 몸체도
 * `CanvasElement[]` 이므로 그것을 읽는 파서가 둘이 되면 "config 에서는 살아나는데
 * 서랍에서는 사라지는 요소" 가 생긴다. 두 번째 요소 파서를 만들지 않으려고 이 한
 * 함수를 공유한다 — 형상이 아니라 **같은 코드**를 나눠 쓴다.
 */
export function parseElements(raw: unknown): CanvasElement[] {
  if (!Array.isArray(raw)) return [];
  const out: CanvasElement[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    const el = parseElement(item);
    if (!el) continue;
    if (seen.has(el.id)) continue;
    seen.add(el.id);
    out.push(el);
  }
  return out;
}

/**
 * 그룹의 출처 기록. 두 문자열이 다 있어야 뜻이 있으므로 한쪽만 있으면 미지정이다.
 * 렌더가 읽지 않는 값이라 손상은 그림에 닿지 않는다(REQ-05 · AC-E1).
 */
function parseSymbolStamp(raw: unknown): SymbolStamp | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const s = raw as Record<string, unknown>;
  const catalogId = optionalString(s.catalog_id);
  const version = optionalString(s.version);
  if (catalogId === undefined || version === undefined) return undefined;
  return { catalog_id: catalogId, version };
}

/**
 * **선택 필드로서의** 스타일. 그룹(004)과 연결선(011)이 함께 쓴다.
 *
 * **빈 객체를 만들어 채우지 않는다** — `{}` 를 넣어 두면 "캐스케이드 미사용" 과 "빈
 * 캐스케이드" 가 구분되지 않고, 설정 UI 가 두 상태를 다르게 보여 사용자를 혼란시킨다
 * (§파서 생존). 001 의 `parseStyle` 을 **그대로 재사용**하고 결과가 비었을 때만 미지정으로
 * 떨어뜨린다 — 스타일 파서가 둘이 되지 않는다.
 *
 * 요소(001)와 갈리는 자리는 여기 하나뿐이다: 요소의 `style` 은 **언제나 있는** 필드라
 * `parseStyle` 을 곧바로 부른다.
 */
function parseOptionalStyle(raw: unknown): ElementStyle | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const style = parseStyle(raw);
  return Object.keys(style).length > 0 ? style : undefined;
}

/**
 * 그룹 1건. 정체성(`id`)이 성립하지 않으면 버린다(001 의 요소 규칙과 같다).
 *
 * **`parts` 는 001 의 `parseElements` 를 그대로 부른다.** 그 함수는 `parseElement` 만
 * 부르고 `parseElement` 의 `kind` 화이트리스트에는 `'group'` 이 없으므로, `parts` 안에
 * 섞여 들어온 그룹 항목은 **형상 덕에 저절로** 버려진다(A6 · AC-E2) — 런타임 깊이 검사가
 * 필요 없는 이유다. 부품 id 중복도 그 함수가 이미 "먼저 온 것이 이긴다" 로 다룬다.
 *
 * `parts` 가 배열이 아니면 빈 배열이다 — **빈 그룹은 오류가 아니라 아무것도 그리지 않는
 * 그룹**이다(REQ-05).
 */
function parseGroup(e: Record<string, unknown>): GroupElement | null {
  const id = optionalString(e.id);
  if (id === undefined) return null;

  const binding = parseBinding(e.binding);
  const style = parseOptionalStyle(e.style);
  const rules = parseRules(e.rules);
  const tween = parseTween(e.tween);
  const symbol = parseSymbolStamp(e.symbol);
  // 그룹도 상자형이다(A11) — `geometry` 가 저장된 상자이므로 로컬 격자가 그대로 성립한다.
  // 그룹은 `CanvasElement` 가 아니라서 위 `parseElement` 의 조각이 닿지 않으므로 여기서
  // **같은 파서**를 부른다(앵커 파서가 둘이 되지 않는다).
  const anchors = parseCustomAnchors(e.anchors);
  // 같은 사정으로 회전도 여기서 **같은 규율**을 부른다(014 §결정 7 — 0 은 키를 만들지 않는다).
  const rotation = storableDegrees(e.rotation);

  return {
    id,
    kind: 'group',
    geometry: parseBoxGeometry(e.geometry),
    parts: parseElements(e.parts),
    ...(anchors !== undefined ? { anchors } : {}),
    ...(rotation !== undefined ? { rotation } : {}),
    ...(binding !== undefined ? { binding } : {}),
    ...(style !== undefined ? { style } : {}),
    ...(rules !== undefined ? { rules } : {}),
    ...(tween !== undefined ? { tween } : {}),
    ...(symbol !== undefined ? { symbol } : {}),
  };
}

/**
 * 자유 끝점 · 중간점 1건. 좌표 한쪽이라도 손상됐으면 `null`이다.
 *
 * 좌표 규율은 경로 명령(`localCoordinate`) · 임의 앵커(`parseCustomAnchor`)와 **같다**:
 * 자리를 뜻하는 좌표에는 채울 기본값이 없으므로 지어내지 않는다. 0 으로 채우면 점이
 * 캔버스 왼쪽 위 모서리로 조용히 이사하고, 사용자는 저술한 적 없는 꺾임을 보게 된다.
 * 반올림은 `coordinate()` 를 지난다 — 유한성 관문 때문에 폴백 인자에는 닿지 않지만,
 * **반올림 규율이 적히는 자리를 둘로 만들지 않는 것**이 이 재사용의 요점이다.
 *
 * 경로 명령과 달리 **로컬 격자가 아니라 캔버스 단위 절대 정수**다. 연결선에는 담을 상자가
 * 없어 기준 삼을 로컬 격자가 없기 때문이다(`connectorTypes` §중간점이 절대 좌표인 이유).
 */
function parseAbsolutePoint(raw: unknown): PointGeometry | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const p = raw as Record<string, unknown>;
  if (!isFiniteNumber(p.x) || !isFiniteNumber(p.y)) return null;
  return { x: coordinate(p.x, 0), y: coordinate(p.y, 0) };
}

/**
 * 연결선의 한 끝. 성립하지 않으면 `null`이고, 그때 **노드 자체가 버려진다**(AC-38).
 *
 * 끝이 없는 연결선은 잇는 것이 없으므로 뜻이 없다 — id 가 없는 요소를 버리는 001 의 규율
 * 과 같은 자리다. 기본 끝점을 지어 넣지 않는 이유도 같다: 캔버스 어딘가에 사용자가 긋지
 * 않은 선이 나타나고, 그것이 제 저술인지 파서가 지어낸 것인지 구분할 길이 없다.
 *
 * **붙은 끝을 먼저 본다.** 두 형태가 한 객체에 섞여 들어오면(`{el,a,x,y}`) 참조가 이긴다 —
 * 참조는 사용자가 앵커를 골랐다는 사실이고 좌표는 그 결과의 흔적일 뿐이므로, 좌표를
 * 택하면 붙여 둔 선이 저장 왕복 한 번에 떨어져 나온다.
 */
function parseConnectorEnd(raw: unknown): ConnectorEnd | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const e = raw as Record<string, unknown>;
  const el = optionalString(e.el);
  const a = optionalString(e.a);
  if (el !== undefined && a !== undefined) return { el, a };
  return parseAbsolutePoint(raw);
}

/**
 * 중간점 목록. 손상 항목은 **그것만** 빠진다(경로 명령 · 임의 앵커와 같은 규율).
 *
 * 살아남은 점이 없으면 **미지정으로 떨어뜨린다.** `[]` 를 남기면 "찍은 적 없음" 과 "다
 * 지웠음" 이 구분되지 않고, 저장 왕복에 없던 `points` 키가 생긴다(AC-25 와 같은 규율).
 *
 * 상한은 008 의 경로 명령과 **같은 수**를 쓴다. 두 값은 같은 개념("한 도형이 들 수 있는
 * 점의 수")이고, 자유선(M11)이 궤적을 받는 쪽이므로 상한 없이 두면 손이 움직인 만큼
 * config 가 자란다. 넘치면 **앞에서부터** 살린다 — 전부 버리면 선이 사라진다.
 *
 * M11 부터 그 수를 이름으로 든다(`MAX_CONNECTOR_POINTS`). 값은 한 자리도 달라지지 않지만,
 * 궤적을 **받는** 쪽이 같은 이름을 보므로 저술의 상한과 파싱의 상한이 갈릴 수 없다 —
 * 갈리면 그어 놓은 선이 저장 왕복에서 조용히 잘린다.
 */
function parseConnectorPoints(raw: unknown): PointGeometry[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out: PointGeometry[] = [];
  for (const item of raw) {
    const point = parseAbsolutePoint(item);
    if (!point) continue;
    out.push(point);
    if (out.length >= MAX_CONNECTOR_POINTS) break;
  }
  return out.length > 0 ? out : undefined;
}

/**
 * 그리는 법. 모르는 값은 **직선으로 떨어진다**(AC-37).
 *
 * 규칙 행의 미지 연산자나 경로의 미지 명령처럼 **버리지 않는** 것에 뜻이 있다: 여기서
 * 노드를 버리면 사용자의 연결이 오타 하나에 사라지는데, 직선은 점이 없어도 그림이 완전한
 * 유일한 갈래이므로 어떤 `points` 상태에서도 화면이 답을 낸다. 그리는 법은 사용자가
 * 한 번 더 고르면 되지만 사라진 선은 되돌릴 길이 없다.
 */
function parseConnectorRoute(raw: unknown): ConnectorRoute {
  return raw === 'elbow' || raw === 'curve' || raw === 'free' || raw === 'straight'
    ? raw
    : DEFAULT_CONNECTOR_ROUTE;
}

/**
 * 연결선 1건. 정체성(`id` · 두 끝)이 성립하지 않으면 버린다.
 *
 * 요소 · 그룹보다 버리는 문턱이 하나 높다: 그 둘은 id 만 있으면 살지만 연결선은 **끝
 * 둘**까지 있어야 산다(AC-38). 근거는 기하 손상 정책과 같은 자리에 있다 — 요소는 기하가
 * 깨져도 기본 기하로 세워 두면 사용자가 화면에서 보고 고칠 수 있지만, 끝이 없는 연결선을
 * 세우려면 두 자리를 **지어내야** 하고 그것은 저술한 적 없는 선이 된다.
 *
 * 스타일 · 규칙 · 바인딩 · 트윈은 001 의 파서를 **그대로** 부른다. 캐스케이드 파서가 둘이
 * 되면 "요소에서는 먹는 규칙이 연결선에서는 안 먹는" 자리가 생긴다. `style` 은 그룹과
 * 같이 빈 객체를 만들어 채우지 않는다(`parseOptionalStyle`).
 *
 * ## 점이 살아남으면 `route` 는 **그 점이 사는 갈래**다 (`routeHosting`)
 *
 * 손으로 적은 파일은 `route: 'straight'` 와 `points` 를 함께 적을 수 있다. 그 둘은 같은
 * 자리에 설 수 없다 — `connectorPath` 가 직선을 점 목록 그대로 이어 그리므로, 그대로
 * 받아들이면 "직선이라고 적혀 있는데 폴리라인으로 그려지는 선" 이 화면에 남고 REQ-04 가
 * 그 순간 거짓이 된다.
 *
 * 버리는 쪽(점을 떨어뜨린다)이 아니라 **올리는 쪽**을 고른다. 위 `parseConnectorRoute` 가
 * 모르는 `route` 를 버리지 않고 직선으로 떨어뜨리는 그 근거와 같은 방향이다 — 사용자가
 * 찍어 둔 꺾임은 되돌릴 길이 없지만, 그리는 법은 한 번 더 고르면 된다.
 *
 * 저술하는 쪽(`insertPointAt` · `appendConnector`)이 지나는 **그 함수**를 여기서도 지나므로
 * 저장 왕복에 값이 달라지지 않는다: 승격해서 저장한 `elbow` 는 그대로 `elbow` 로 돌아온다.
 */
function parseConnector(e: Record<string, unknown>): ConnectorElement | null {
  const id = optionalString(e.id);
  if (id === undefined) return null;

  const from = parseConnectorEnd(e.from);
  const to = parseConnectorEnd(e.to);
  if (from === null || to === null) return null;

  const points = parseConnectorPoints(e.points);
  const style = parseOptionalStyle(e.style);
  const binding = parseBinding(e.binding);
  const rules = parseRules(e.rules);
  const tween = parseTween(e.tween);

  return {
    id,
    kind: CONNECTOR_KIND,
    from,
    to,
    route: routeHosting(parseConnectorRoute(e.route), points?.length ?? 0),
    ...(points !== undefined ? { points } : {}),
    ...(style !== undefined ? { style } : {}),
    ...(binding !== undefined ? { binding } : {}),
    ...(rules !== undefined ? { rules } : {}),
    ...(tween !== undefined ? { tween } : {}),
  };
}

/**
 * 최상위 노드 1건 — 그룹이면 그룹으로, 연결선이면 연결선으로, 아니면 001 의 요소 파서로.
 *
 * **갈래가 `parseElement` 안이 아니라 그 위 한 층에 서는 것이 이 형상의 요점이다**
 * (§파서 생존). `parseElement` 는 `CanvasElement` 를 내는 계약이고 008 이 `'path'` 갈래를
 * 더하며 그 계약을 다시 못박았다. 그룹을 그 안에 넣으면 반환 타입이 넓어져 계약이 깨지고,
 * `parts` 안의 그룹을 **런타임으로** 걸러야 한다. 한 층 위에 두면 그 검사가 사라진다.
 *
 * **011 이 이 함수에 더한 것은 한 줄이다.** `parseElement` 는 한 글자도 바뀌지 않았고,
 * 그래서 그 함수의 `kind` 화이트리스트를 세는 출시된 가드가 그대로 산다(AC-34 · AC-35).
 * 같은 형상 덕에 `parts` 에 섞여 들어온 연결선도 **저절로** 버려진다 — 그룹 안에 연결선이
 * 올 수 없다는 사실에 런타임 검사가 필요 없다(A3).
 */
function parseNode(raw: unknown): CanvasNode | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const e = raw as Record<string, unknown>;
  if (e.kind === 'group') return parseGroup(e);
  if (isConnectorKind(e.kind)) return parseConnector(e);
  return parseElement(raw);
}

/**
 * 최상위 노드 목록. 규율은 `parseElements` 와 **한 글자도 다르지 않다** — 배열이 아니면
 * 빈 목록이고, 정체성이 없는 항목은 버리며, id 중복은 먼저 온 것이 이긴다.
 *
 * 그룹과 요소가 **같은 id 공간**을 나눠 쓴다. 선택 키도 프레임 키도 최상위에서는 `nodeId`
 * 하나이므로, 둘을 갈라 두면 "그룹 `a` 와 요소 `a` 가 동시에 있는" 상태가 생긴다.
 */
export function parseNodes(raw: unknown): CanvasNode[] {
  if (!Array.isArray(raw)) return [];
  const out: CanvasNode[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    const node = parseNode(item);
    if (!node) continue;
    if (seen.has(node.id)) continue;
    seen.add(node.id);
    out.push(node);
  }
  return out;
}

/**
 * 이 요소가 판독값을 **숫자로 읽는가**(`CanvasElementBase.numeric` 의 유일한 해석기).
 *
 * `false` 일 때만 거짓이다 — **부재는 숫자**다. 이 한 줄이 "이 필드가 없는 기존 config 가
 * 종전과 똑같이 그려진다" 는 보증의 전부이며, 그래서 참 판정(`!!el.numeric`)이 아니라
 * **거짓 동일성 판정**(`!== false`)으로 적는다. 소비 측(패널·편집기·문구 치환)이 저마다
 * 판정을 적으면 그중 하나가 참 판정으로 적히는 순간 부재가 조용히 뒤집힌다.
 */
export function isNumericElement(el: Pick<CanvasElementBase, 'numeric'>): boolean {
  return el.numeric !== false;
}

/** 데이터 소스 종류. 미인정 값은 미지정으로 떨어뜨린다(소스 판정 계약이 idle 로 방어). */
function parseDataSource(raw: unknown): ChartDataSourceKind | undefined {
  return raw === 'store' || raw === 'tsdb' || raw === 'sysmetrics' ? raw : undefined;
}

// --- 공개 API ------------------------------------------------------------

/**
 * 신규 캔버스 패널의 기본 config.
 *
 * 요소 0개로 시작한다 — 빈 캔버스는 오류가 아니라 안내 상태다(REQ-05). 데이터 소스는
 * 다른 패널과 같은 기본 store 소스(비활성: 에이전트·시리즈 미선택)이며, `channel_name`
 * 키는 비워 둔 채 남긴다(기존 패널 기본값 규약 동일).
 */
export function buildDefaultCanvasConfig(): CanvasPanelConfig {
  return {
    channel_name: '',
    data_source: 'store',
    store_source: buildDefaultStoreSource(),
    canvas: { ...DEFAULT_CANVAS_SIZE },
    tween: { duration_ms: DEFAULT_TWEEN_DURATION_MS, easing: DEFAULT_TWEEN_EASING },
    elements: [],
  };
}

/**
 * 캔버스 패널 config 를 파싱한다.
 *
 * 어떤 입력(null·숫자·문자열·배열·손상 객체)에도 **예외를 던지지 않는다**. 알 수 없는
 * 필드는 보존하지 않고 무시하며, 결측 필드는 기본값으로 채운다(REQ-01/REQ-05, §위험 R3).
 */
export function parseCanvasConfig(raw: unknown): CanvasPanelConfig {
  // 배열도 객체지만 config 형상이 아니다 — 필드 접근이 전부 undefined 가 되어 기본값이 된다.
  const cfg = raw && typeof raw === 'object' && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : {};

  const background = optionalString(cfg.background);
  const dataSource = parseDataSource(cfg.data_source);
  const tween = parseTween(cfg.tween);

  return {
    // 채널 키는 문자열이 아니면 빈 문자열로 둔다 — 키가 사라지면 설정 화면이 편집할
    // 필드를 잃는다(uiStore.createDefaultPanel 의 `channel_name: ''` 주석과 같은 이유).
    channel_name: typeof cfg.channel_name === 'string' ? cfg.channel_name : '',
    ...(dataSource !== undefined ? { data_source: dataSource } : {}),
    // 소스 블록은 형상을 그대로 신뢰한다(히트맵 parseStoreSource 선례) — 무효 필드는
    // 조회 훅이 idle 로 방어하며, 여기서 손대면 사용자 설정이 조용히 사라진다.
    ...(cfg.store_source && typeof cfg.store_source === 'object'
      ? { store_source: cfg.store_source as StoreSourceConfig }
      : {}),
    ...(cfg.tsdb_source && typeof cfg.tsdb_source === 'object'
      ? { tsdb_source: cfg.tsdb_source as TsdbSourceConfig }
      : {}),
    ...(background !== undefined ? { background } : {}),
    ...(tween !== undefined ? { tween } : {}),
    // 크기는 **언제나** 있다(위 `CanvasPanelConfig.canvas` 주석) — 옵셔널로 두면 투영하는
    // 자리마다 "없으면 기본" 을 적게 되고, 그중 하나가 다른 기본을 적는 순간 갈라진다.
    canvas: parseCanvasSize(cfg.canvas),
    elements: parseNodes(cfg.elements),
  };
}
