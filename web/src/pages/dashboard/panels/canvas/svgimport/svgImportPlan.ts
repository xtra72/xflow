// 가져오기 계획 — viewBox · 상자 · 로컬 정규화 · 분할 · 상한 (SPEC-CANVAS-007 M6 · M7).
//
// **`viewBox` 를 축마다 독립으로 로컬 격자에 앉힌다.** 그러면 그림이 일그러진다 —
// **요소 상자의 종횡비를 문서의 종횡비로 정하지 않는다면.** 그래서 정한다: 가져온 그림의
// 상자는 문서 종횡비를 지킨다. 로컬 격자의 정사각성과 상자의 비정사각성이 정확히 상쇄되어
// 화면에 나오는 것이 원본 비율이다.
//
// **기각 — 종횡비를 지켜 로컬 격자 안에 레터박스한다.** 그러면 로컬 좌표 안에 **죽은
// 여백**이 박히고, 요소 상자의 8핸들이 그림에 닿지 않는 자리에 선다. 상자를 잡아 늘여도
// 여백이 함께 늘어나므로 사용자는 "왜 손잡이가 그림에서 떨어져 있는가" 를 영영 고칠 수 없다.
//
// **그래서 문서의 `preserveAspectRatio` 를 읽지 않는다.** 그 속성은 뷰포트가 viewBox 를
// 레터박스하는 규칙인데 007 에는 뷰포트가 없고, **상자의 종횡비를 문서에 맞추는 것이 그
// 속성이 하려던 일을 이미 한 것**이다.
//
// **나눗셈 자리를 늘리지 않는다**(불변식 K8). 로컬 → px 의 나눗셈은 여전히
// `canvasGeometry.projectPathPoints` 하나이고, 이 모듈이 하는 것은 **곱셈**이다.
// 반올림은 `canvasConfig.coordinate()` 를 그대로 쓴다 — 두 번째 수치 규율을 만들지 않는다.
//
// **로컬 좌표를 clamp 하지 않는다.** 도형의 잉크가 상자 밖으로 나가는 경우가 실재하고
// (선 두께 · 문서 밖 좌표), 008 이 말풍선 꼬리에 대해 이미 그렇게 정했다.
//
// **두 변 모두 `MIN_ELEMENT_EXTENT` 이상을 보장한다**(REQ-06 · 가정 A15). 가로선 하나의
// 바운딩 박스는 `h = 0` 이고, 그 상자를 저장하면 **파서가 기하를 통째로
// `DEFAULT_BOX_GEOMETRY` 로 갈아 끼운다**(실측 `isDegenerateBox` → `parseBoxGeometry`) —
// "저장할 땐 맞고 다시 열면 딴 자리" 가 되는 자료 손상이다.
//
// **요소마다 제 기하의 상자를 준다**(결함 D3 정정). 007 이 처음 배달한 것은 상자 하나를
// 온 그림이 함께 쓰는 것이었고, 그 근거는 "도형마다 주면 정수 반올림으로 최대 0.5 단위씩
// 어긋난다" 였다. 근거는 참이지만 대가가 더 크다 — 큰 문서 구석의 작은 별을 고르면 손잡이
// 여덟이 문서 가장자리에 서고(I23 의 반대), 선택 윤곽이 그림 전부를 두르며, **정렬이 죽는다**
// (`alignDeltas` 가 요소 상자로 맞추는데 상자가 전부 같으면 델타가 전부 0 이다). 왜 뒤집는지와
// 무엇을 어떻게 재는지는 `svgImportBox` 가 소유한다.
//
// **다만 한 도형이 상한 때문에 나뉜 조각들은 여전히 같은 상자를 쓴다.** 도넛의 바깥과 구멍은
// 한 도형이었고 조각마다 상자를 주면 구멍이 반올림만큼 어긋나 도넛이 어그러진다 — 그래서
// 상자는 **나누기 전의 도형**에서 잰다(AC-06 이 요구하는 깊은 비교가 곧 이것이다).
//
// **계단 오프셋은 여전히 무리 전체에 한 번만 더해진다**(REQ-06). 상자가 여럿이 되었으므로
// "더할 자리가 없다" 는 형상 논증은 사라졌고, 대신 요소를 만드는 입구가 **하나의 오프셋을
// 모든 상자에** 더한다 — 같은 값을 더하므로 상대 배치가 보존된다.
//
// **상한 셋은 서로 다른 층을 죄지만 함께 정해진 수다.** 요소 수만 죄면 1024개가 전부 256
// 명령일 때 262,144 명령이 되어 예산을 통째로 먹는다. 그래서 **명령 총수를 함께** 죈다.
// 넘으면 **거절이다 — 앞부분만 가져오지 않는다**(§결정 7).
//
// @spec SPEC-CANVAS-007 REQ-02 · REQ-03 · REQ-05 · REQ-06 · AC-05 · AC-06 · AC-E8

import {
  coordinate,
  isDegenerateLine,
  MIN_ELEMENT_EXTENT,
  type BoxGeometry,
  type CanvasSize,
  type ElementStyle,
  type LineGeometry,
  type PointGeometry,
} from '../canvasConfig';
import { clampCanvasFontSize } from '../canvasEditGeometry';
import { MAX_PATH_COMMANDS, PATH_LOCAL_EXTENT, type PathCommand } from '../shapes/pathTypes';

import { readSvgDocument, type SvgDocumentReader, type ViewBox } from './svgDocument';
import {
  DEFAULT_IMPORT_LIMITS,
  MAX_IMPORT_FILE_BYTES,
  type SvgImportLimits,
  type ImportNote,
  type ImportReport,
  type ImportRefusal,
  type ImportedNative,
  type ImportedShape,
  type ImportedText,
} from './svgImportTypes';
import {
  placePoint,
  placeShape,
  tightCommandBounds,
  viewBoxBounds,
  type DocumentPlacement,
} from './svgImportBox';
import {
  applyEvenOddWinding,
  commandBounds,
  splitByCommandLimit,
  unionBounds,
  type Bounds,
} from './svgImportSplit';
import { mergeNotes } from './svgStyle';

/**
 * 가져온 그림이 캔버스에서 차지하는 비율(긴 축 기준).
 *
 * 1 이 아닌 이유: 상자를 캔버스에 꽉 채우면 8핸들이 캔버스 가장자리에 붙어 **집기 어렵고**,
 * 사용자가 가져온 그림 옆에 무언가를 놓을 자리가 없다. 0.8 은 네 변에 각각 캔버스의 10%
 * 여백을 남긴다 — 기본 캔버스(500×400)에서 가로 50 · 세로 40 단위이며, `HIT_TOLERANCE_PX`
 * (6)보다 넉넉하다.
 */
export const IMPORT_BOX_FILL = 0.8;

/** 요소 하나가 될 준비가 끝난 도형. 좌표는 **제 상자의 로컬 정수**다. */
export interface ImportedPathSpec {
  /** 갈래표. 세 spec 이 한 배열에 **문서 순서대로** 섞여 살므로 판별자가 필요하다. */
  readonly kind: 'path';
  readonly commands: readonly PathCommand[];
  /**
   * 이 도형의 기하가 차지하는 최소 영역. **계단 오프셋은 아직 더해지지 않았다**(M8 의 몫).
   *
   * 한 도형이 상한 때문에 나뉘었으면 그 조각들은 **같은 값**을 든다 — 상자를 나누기 전의
   * 도형에서 재기 때문이다.
   */
  readonly box: BoxGeometry;
  readonly style: ElementStyle;
  /** SVG 가 칠을 한 마디라도 말했는가 — 아니면 008 의 `pathSeedStyle` 이 선다. */
  readonly hasOwnStyle: boolean;
}

/**
 * 요소 하나가 될 준비가 끝난 **사각형 또는 타원**.
 *
 * `ImportedPathSpec` 과 달리 `commands` 가 없다 — 캔버스의 `rect`·`ellipse` 는 상자 하나로
 * 다 그려지므로 실을 명령이 없다. 그 없음이 예산에서 곧바로 값이 된다(`MAX_IMPORT_COMMANDS`
 * 를 한 칸도 먹지 않는다).
 *
 * 둘을 한 타입에 둔 것은 **기하가 같기** 때문이다(`BoxGeometry`). 요소가 될 때 `kind` 를
 * 그대로 옮겨 적는 것 말고 다른 차이가 없으므로, 갈라 두면 똑같은 줄이 두 벌 생긴다.
 */
export interface ImportedBoxSpec {
  readonly kind: 'rect' | 'ellipse';
  /** 요소 상자. **계단 오프셋은 아직 더해지지 않았다**(M8 의 몫). */
  readonly box: BoxGeometry;
  readonly style: ElementStyle;
  /** SVG 가 칠을 한 마디라도 말했는가 — 아니면 씨앗 스타일이 선다. */
  readonly hasOwnStyle: boolean;
}

/**
 * 요소 하나가 될 준비가 끝난 **선**. 두 끝점(정수 캔버스 단위)을 그대로 든다.
 *
 * 상자 갈래와 갈라 두는 것은 기하가 다르기 때문이다 — `LineGeometry` 는 두 끝점이고
 * 거기에는 "좌상단과 크기" 가 없다. 한 타입에 담으면 `kind` 로 기하를 좁히지 못해
 * 요소를 만드는 자리가 형상 검사(`'w' in geo`)를 하게 되고, 그것은 `Geometry` 합집합이
 * `kind` 로 판별되게 세워 둔 001 의 규율을 되돌리는 일이다.
 */
export interface ImportedLineSpec {
  readonly kind: 'line';
  /** 두 끝점. **계단 오프셋은 아직 더해지지 않았다**(M8 의 몫). */
  readonly line: LineGeometry;
  readonly style: ElementStyle;
  readonly hasOwnStyle: boolean;
}

/**
 * 요소가 될 도형 하나 — 경로이거나 원시형이다. **한 배열에 문서 순서대로 섞인다.**
 *
 * 갈래마다 배열을 따로 내는 안을 기각한다. 배열 순서가 이 패널의 **유일한 z-order** 이므로
 * (001), 원시형을 뒤에 몰면 문서에서 배경이던 `<rect>` 가 그 위에 그려지던 `<path>` 를
 * 덮는다 — 007 이 문구에 대해 치르고 `textOrderChanged` 로 보고한 그 대가이며, 배경 사각형
 * 에서는 훨씬 크다(이름표가 가려지는 것과 그림 전체가 덮이는 것은 다른 일이다). 한 배열이면
 * 그 대가가 아예 발생하지 않으므로 보고할 것도 없다.
 */
export type ImportedShapeSpec = ImportedPathSpec | ImportedBoxSpec | ImportedLineSpec;

/**
 * 요소 하나가 될 준비가 끝난 문구. 좌표는 **캔버스 단위 기준점**이다.
 *
 * 경로와 달리 로컬 정규화가 없다 — 문구 요소의 기하는 상자가 아니라 점 하나이고
 * (`PointGeometry`), 점에는 나눌 격자가 없다. 그래서 이 층에서 이미 캔버스 단위다.
 */
export interface ImportedTextSpec {
  /** 정렬 기준점(정수 캔버스 단위). **계단 오프셋은 아직 더해지지 않았다**(M8 의 몫). */
  readonly at: PointGeometry;
  /** 한 줄로 편 글자. 상한을 넘었으면 이미 잘려 있다. */
  readonly text: string;
  /** `fontSize` 가 **여기서 px 가 된다** — 문서 축척을 곱하고 패널 범위로 죈 뒤다. */
  readonly style: ElementStyle;
  /** 원본이 칠을 한 마디라도 말했는가 — 아니면 002 의 문구 씨앗 색이 선다. */
  readonly hasOwnStyle: boolean;
}

/** 계획의 산출. **예외가 아니라 값으로 실패가 돌아온다**(REQ-07). */
export type SvgImportPlan =
  | {
      readonly ok: true;
      /**
       * 도형마다 제 상자를 든다. **무리가 함께 쓰는 상자는 없다** — 문서 틀은 이 층 안에서
       * 살다 죽고(`fitBox`), 밖으로 나가는 것은 요소가 될 도형들뿐이다.
       */
      readonly shapes: readonly ImportedShapeSpec[];
      /**
       * 문구들. **도형과 갈라 나간다** — 요소가 될 때 상자가 아니라 점 위에 서고, 하나는
       * 명령을, 다른 하나는 글자를 예산에서 먹는다.
       */
      readonly texts: readonly ImportedTextSpec[];
      readonly report: ImportReport;
    }
  | { readonly ok: false; readonly refusal: ImportRefusal };

// --- 바운딩 박스 ---------------------------------------------------------

// `commandBounds` 는 `svgImportSplit` 이 소유한다 — 포함 판정과 `viewBox` 폴백이 **같은**
// 상자를 봐야 하고, 둘이 갈라지면 "무리로는 붙어 있는데 폴백 상자로는 떨어져 있다" 가
// 생긴다. 계획 층은 그것을 그대로 다시 내보내 호출부가 두 모듈을 다 알 필요가 없게 한다.
export { commandBounds } from './svgImportSplit';

/** 도형 전부의 합집합 바운딩 박스. */
function shapesBounds(shapes: readonly ImportedShape[]): Bounds | undefined {
  let union: Bounds | undefined;
  for (const shape of shapes) union = unionBounds(union, commandBounds(shape.commands));
  return union;
}

// --- viewBox 폴백 -------------------------------------------------------

/** 폴백의 결과 — 어느 단에서 정해졌는지 값으로 말한다. */
export type ViewBoxOutcome =
  | { readonly ok: true; readonly viewBox: ViewBox }
  | { readonly ok: false; readonly reason: 'degenerateViewBox' | 'emptyDocument' };

/**
 * `viewBox` 3단 폴백. **어느 경우에도 브라우저에게 크기를 묻지 않는다** —
 * `<img>` 를 만들지 않고 `naturalWidth` 를 읽지 않는다(REQ-02).
 *
 * 1. `viewBox` 속성.
 * 2. `width`/`height` 속성 — **단위 없는 수 또는 `px` 만**(`%` 는 크기가 아니다).
 * 3. **변환을 다 녹인 뒤 모든 도형의 합집합 바운딩 박스.**
 *
 * 그 합집합이 한 축이라도 퇴화(길이 0)하면 **거절한다** — 가로선 하나뿐인 문서에는 담을
 * 종횡비가 없고, 지어내면 그것은 우리가 고른 값이지 문서의 값이 아니다.
 */
export function resolveViewBox(
  declared: ViewBox | undefined,
  size: { readonly width: number; readonly height: number } | undefined,
  shapes: readonly ImportedShape[],
): ViewBoxOutcome {
  if (declared !== undefined) return { ok: true, viewBox: declared };
  if (size !== undefined) {
    return { ok: true, viewBox: { minX: 0, minY: 0, width: size.width, height: size.height } };
  }
  const union = shapesBounds(shapes);
  if (union === undefined) return { ok: false, reason: 'emptyDocument' };
  const width = union.maxX - union.minX;
  const height = union.maxY - union.minY;
  if (!(width > 0) || !(height > 0)) return { ok: false, reason: 'degenerateViewBox' };
  return { ok: true, viewBox: { minX: union.minX, minY: union.minY, width, height } };
}

// --- 상자와 로컬 정규화 -------------------------------------------------

/**
 * 문서 종횡비를 지켜 캔버스 안에 담은 상자. **계단 오프셋은 더하지 않는다** —
 * 그것은 요소를 만드는 입구의 몫이고, 입구는 하나다(불변식 K9).
 *
 * 두 변 모두 `MIN_ELEMENT_EXTENT` 이상을 보장한다. 이 한 줄이 없으면 아주 납작한
 * `viewBox`(`0 0 1000 1`)가 높이 0 짜리 상자를 내고, 그 상자는 **저장 왕복에서 기하
 * 통째로** `DEFAULT_BOX_GEOMETRY` 로 갈아 끼워진다(가정 A15).
 */
export function fitBox(viewBox: ViewBox, canvas: CanvasSize): BoxGeometry {
  const scale = Math.min(
    (canvas.width * IMPORT_BOX_FILL) / viewBox.width,
    (canvas.height * IMPORT_BOX_FILL) / viewBox.height,
  );
  const w = Math.max(MIN_ELEMENT_EXTENT, coordinate(viewBox.width * scale, MIN_ELEMENT_EXTENT));
  const h = Math.max(MIN_ELEMENT_EXTENT, coordinate(viewBox.height * scale, MIN_ELEMENT_EXTENT));
  return {
    x: coordinate((canvas.width - w) / 2, 0),
    y: coordinate((canvas.height - h) / 2, 0),
    w,
    h,
  };
}

/**
 * 사용자 단위 → 경로 로컬 정수. **축마다 독립이다**(위 머리말).
 *
 * ```
 * local.x = round( (user.x − minX) × PATH_LOCAL_EXTENT / vbW )
 * local.y = round( (user.y − minY) × PATH_LOCAL_EXTENT / vbH )
 * ```
 */
export function toLocalCommands(
  commands: readonly PathCommand[],
  viewBox: ViewBox,
): PathCommand[] {
  const localX = (x: number): number =>
    coordinate(((x - viewBox.minX) * PATH_LOCAL_EXTENT) / viewBox.width, 0);
  const localY = (y: number): number =>
    coordinate(((y - viewBox.minY) * PATH_LOCAL_EXTENT) / viewBox.height, 0);
  return commands.map((cmd) => {
    switch (cmd.c) {
      case 'Z':
        return { c: 'Z' };
      case 'M':
      case 'L':
        return { c: cmd.c, x: localX(cmd.x), y: localY(cmd.y) };
      case 'C':
        return {
          c: 'C',
          x1: localX(cmd.x1),
          y1: localY(cmd.y1),
          x2: localX(cmd.x2),
          y2: localY(cmd.y2),
          x: localX(cmd.x),
          y: localY(cmd.y),
        };
    }
  });
}

/**
 * 선 두께를 `상자 ÷ viewBox` 비로 옮긴다.
 *
 * **새 규율을 만들지 않는 것이 요점이다.** `strokeWidth` 는 투영을 지나지 않는 원시
 * px 이고(실측: `ctx.lineWidth = resolveStrokeWidth(style.strokeWidth)` 에 축척이 곱해지지
 * 않는다), 그것은 이 패널의 **모든 요소가 이미 가진 성질**이다. 축척 1 에서 정확하고
 * 다른 축척에서는 다른 모든 요소와 **같은 방식으로** 어긋난다.
 *
 * 상자 종횡비를 문서 종횡비로 맞춘 덕에 이 비는 **두 축에서 같다** — 비균등 배율은 문서
 * **안쪽**의 `transform` 에서만 생기고, 그때는 문서 층이 이미 `√|det|` 를 곱하고 근사로
 * 보고했다.
 */
function scaleStroke(style: ElementStyle, ratio: number): ElementStyle {
  if (style.strokeWidth === undefined) return style;
  return { ...style, strokeWidth: style.strokeWidth * ratio };
}

// --- 예산 추정 ----------------------------------------------------------

/**
 * 산출이 config 에서 차지할 바이트의 추정.
 *
 * 손으로 센 상수(명령당 27~67B)를 쓰지 않고 **실제 직렬화 길이를 잰다** — 상수는 명령
 * 형상이 바뀌는 날 조용히 틀리고, 그 틀림은 "저장이 413 으로 실패한다" 로만 드러난다
 * (위험 R6). id 는 아직 없으므로 자리를 채워 센다.
 *
 * **문자 수가 아니라 UTF-8 바이트다**(결함 B 정정). 경로의 직렬화는 숫자와 따옴표뿐이라
 * 두 수가 같지만, 문구는 사용자의 글자를 그대로 싣는다 — 한글 한 자가 3바이트이므로
 * `length` 로 재면 추정이 실제의 1/3 이 되고, 화면이 "약 24KB" 라고 말한 가져오기가
 * 저장에서 57KB 를 먹는다. 파일 상한이 `TextEncoder` 를 쓰는 이유가 그대로 여기에도 있다.
 */
export function estimateBytes(
  shapes: readonly ImportedShapeSpec[],
  texts: readonly ImportedTextSpec[] = [],
): number {
  let total = 2; // 배열의 대괄호 둘.
  for (const shape of shapes) {
    // **요소가 실제로 실릴 형상 그대로 센다.** 원시형에는 `path` 칸이 아예 없고 선의 기하는
    // 네 칸 모두 이름이 다르다 — 셋을 한 형상으로 뭉뚱그려 세면 추정이 형상마다 어긋난다.
    total += byteLength(
      JSON.stringify(
        shape.kind === 'path'
          ? {
              id: 'el-00',
              kind: 'path',
              geometry: shape.box,
              path: shape.commands,
              style: shape.style,
            }
          : {
              id: 'el-00',
              kind: shape.kind,
              geometry: shape.kind === 'line' ? shape.line : shape.box,
              style: shape.style,
            },
      ),
    );
  }
  // **문구도 센다.** 명령을 하나도 나르지 않으므로 명령 상한 아래를 그냥 지나가지만,
  // 제 문자열로 예산을 먹는 것은 경로와 똑같다 — 세지 않으면 추정이 "저장이 413 으로
  // 실패한다" 는 그 자리에서만 틀렸음이 드러난다(위험 R6).
  for (const text of texts) {
    total += byteLength(
      JSON.stringify({
        id: 'el-00',
        kind: 'text',
        geometry: text.at,
        text: text.text,
        style: text.style,
      }),
    );
  }
  // 원소 **사이**의 쉼표는 `n − 1` 개다. `n` 개로 세면 빈 배열이 3바이트가 되고, 그
  // 한 바이트의 어긋남이 "추정이 실제 직렬화를 재는가" 를 재는 시험을 통과시킨다.
  return total + Math.max(0, shapes.length + texts.length - 1);
}

// --- 입구 --------------------------------------------------------------

export interface PlanSvgImportOptions {
  /** 문서를 읽는 함수. **주입 가능하다** — 바이트 상한이 파싱 **전에** 있음을 시험이 잰다. */
  readonly readDocument?: SvgDocumentReader;
  /**
   * 요소 · 명령 상한. 없으면 이 모듈의 컴파일 기본값이 선다.
   *
   * **상한이 인자로 들어오는 것이 §결정 14 의 형상이다** — 운영자가 설정한 수를 서버가
   * 내려 주고 화면 층이 여기로 넘긴다. 이 함수는 여전히 순수하다: 상한을 스스로 물어보지
   * 않으므로 시험이 어떤 상한으로든 계획을 재현할 수 있다.
   */
  readonly limits?: SvgImportLimits;
}

/**
 * 문자열 하나를 놓을 준비가 끝난 계획으로.
 *
 * **바이트 상한을 파싱보다 먼저 본다.** 파싱 뒤로 옮기면 내부 엔티티 확장 폭탄이 먼저
 * 터진다 — 브라우저가 그것을 막는다고 알려져 있으나 본 SPEC 은 실측하지 않았다(가정 A8 ·
 * 위험 R10). 실측하지 않은 방어에 기대지 않는 유일한 길이 파싱 전에 자르는 것이다.
 */
export function planSvgImport(
  text: string,
  canvas: CanvasSize,
  options: PlanSvgImportOptions = {},
): SvgImportPlan {
  const limits = options.limits ?? DEFAULT_IMPORT_LIMITS;
  const bytes = byteLength(text);
  if (bytes > MAX_IMPORT_FILE_BYTES) {
    return {
      ok: false,
      refusal: { reason: 'fileTooLarge', actual: bytes, limit: MAX_IMPORT_FILE_BYTES },
    };
  }

  const read = options.readDocument ?? readSvgDocument;
  const outcome = read(text);
  if (!outcome.ok) return { ok: false, refusal: outcome.refusal };
  const { shapes, texts, notes, viewBox, size } = outcome.document;

  const resolved = resolveViewBox(viewBox, size, shapes);
  if (!resolved.ok) {
    return { ok: false, refusal: { reason: resolved.reason, actual: 0, limit: 0 } };
  }

  // 문서 틀 — 온 그림이 캔버스 안에 놓이는 자리이자 **모든 요소가 공유하는 축척의 출처**다.
  // 이 값은 이 함수 안에서 살다 죽는다: 밖으로 나가는 상자는 도형마다의 것뿐이다.
  const doc: DocumentPlacement = { box: fitBox(resolved.viewBox, canvas), viewBox: resolved.viewBox };
  const strokeRatio = doc.box.w / resolved.viewBox.width;
  const specs: ImportedShapeSpec[] = [];
  const extra: ImportNote[] = [];

  for (const shape of shapes) {
    // **원시형이 먼저다.** 태그가 스스로 무엇인지 말했고 그 말을 캔버스가 그대로 담을 수
    // 있으면, 명령으로 옮겨 적을 이유가 없다. 담지 못하면(`undefined`) 아래로 떨어져
    // 경로가 된다 — 같은 도형의 두 표현이 `ImportedShape` 에 함께 실려 있기 때문에
    // 이 되돌아감에 특례가 필요 없다.
    if (shape.native !== undefined) {
      const spec = planNative(shape.native, shape, doc, strokeRatio);
      if (spec !== undefined) {
        specs.push(spec);
        continue;
      }
    }
    // **감김 뒤집기가 먼저다.** 나눈 뒤에 뒤집으면 무리 밖의 형제를 볼 수 없어 깊이를
    // 잘못 세고, 그때 도넛의 구멍이 채워진다.
    const winded = shape.evenOdd ? applyEvenOddWinding(shape.commands) : shape.commands;
    const pieces = splitByCommandLimit(winded, MAX_PATH_COMMANDS);
    if (pieces === undefined) {
      // 무리로 나눠도 상한을 넘는다 — **자르지 않고 그 도형을 거절한다**(위험 R4).
      extra.push({ kind: 'dropped', reason: 'commandLimitDropped', count: 1 });
      continue;
    }
    // **나누기 전의 도형에서 잰다.** 조각들(도넛의 바깥과 구멍)은 한 도형이었으므로 같은
    // 상자와 같은 기준 틀을 써야 한다 — 조각마다 재면 구멍이 반올림만큼 어긋난다.
    const placement = placeShape(tightCommandBounds(winded) ?? viewBoxBounds(doc.viewBox), doc);
    for (const piece of pieces) {
      specs.push({
        kind: 'path',
        commands: toLocalCommands(piece, placement.frame),
        box: placement.box,
        style: scaleStroke(shape.style, strokeRatio),
        hasOwnStyle: shape.hasOwnStyle,
      });
    }
  }

  // **문구도 요소 상한을 먹는다.** 상한이 죄는 것은 "경로 몇 개" 가 아니라 config 에 실릴
  // 요소 수이므로, 문구를 빼고 세면 상한을 넘는 요소가 조용히 놓인다.
  const textSpecs = planTexts(texts, doc, strokeRatio, extra);

  // **넘으면 거절이다. 앞부분만 가져오지 않는다.** 도구가 내는 문서 순서는 배경→전경이라
  // 앞쪽은 대개 배경 조각들이고, 그 절단은 "설명 없는 틀린 그림" 이다.
  const elementCount = specs.length + textSpecs.length;
  if (elementCount > limits.maxElements) {
    return {
      ok: false,
      refusal: { reason: 'tooManyElements', actual: elementCount, limit: limits.maxElements },
    };
  }
  const commandTotal = countCommands(specs);
  if (commandTotal > limits.maxCommands) {
    return {
      ok: false,
      refusal: { reason: 'tooManyCommands', actual: commandTotal, limit: limits.maxCommands },
    };
  }

  return {
    ok: true,
    shapes: specs,
    texts: textSpecs,
    report: buildReport(specs, textSpecs, [...notes, ...extra]),
  };
}

/**
 * 원시 도형 하나를 놓을 준비가 끝난 값으로. **못 놓으면 `undefined`**(호출부가 경로로 떨어진다).
 *
 * **사용자 단위 → 캔버스 단위의 규칙을 새로 만들지 않는다.** 상자는 `placeShape` 를,
 * 선의 두 끝점은 `placePoint` 를 지난다 — 경로의 상자가 지나는 그 함수이고 문구의 기준점이
 * 지나는 그 함수다(007 0.2.0 이 `placeShape` 를 `placePoint` 위에 세워 규칙을 하나로 만든
 * 뒤로 자리는 하나다). 여기서 `doc.box.x + (x − minX) × 축척` 을 다시 적으면 그 통일이
 * 그날로 깨진다.
 *
 * **상자를 `tightCommandBounds` 로 재지 않는다.** 원시형의 상자는 **태그가 말한 수 그 자체**
 * 이므로 잴 것이 없다 — `<rect>` 의 상자는 `x`·`y`·`width`·`height` 이고 `<ellipse>` 의 상자는
 * `cx±rx`·`cy±ry` 다. 명령으로 옮겨 잰 값과 견주면 사각형은 정확히 같고 타원은 **다르다**:
 * 4분원의 3차 근사는 반경을 최대 0.027% 안쪽으로 스치므로, 명령에서 잰 상자는 그만큼 작다.
 * 태그가 말한 수를 쓰는 쪽이 **더 정확하며** 그것이 이 치환의 값 가운데 하나다.
 * `strokeWidth` 는 여전히 상자에 들지 않는다(`svgImportBox` §결정 1 — 잉크까지 넓히면
 * "손잡이가 잉크를 두른다" 가 축척 1 에서만 참이 된다).
 *
 * **퇴화한 선은 되돌린다.** 두 끝점이 캔버스 정수로 같은 칸에 떨어지면 저장 왕복에서
 * 파서가 기하를 통째로 `DEFAULT_LINE_GEOMETRY` 로 갈아 끼운다(실측 `isDegenerateLine` →
 * `parseLineGeometry`) — 캔버스 한복판을 가로지르는 400 단위짜리 선이 나타난다. 상자 갈래는
 * 이 되돌림이 필요 없다: `placeShape` 의 `axisSpan` 이 **사용자 단위에서 먼저** 넓혀 두어
 * 두 변이 반올림 뒤에도 `MIN_ELEMENT_EXTENT` 이상임을 이미 보장한다. 판정을 캔버스 정수
 * **뒤에** 두는 것이 요점이다 — 사용자 단위로는 다른 두 점이 같은 칸에 떨어질 수 있다.
 */
function planNative(
  native: ImportedNative,
  shape: ImportedShape,
  doc: DocumentPlacement,
  ratio: number,
): ImportedShapeSpec | undefined {
  const style = scaleStroke(shape.style, ratio);
  if (native.kind === 'line') {
    const from = placePoint(native.x1, native.y1, doc);
    const to = placePoint(native.x2, native.y2, doc);
    const line: LineGeometry = { x1: from.x, y1: from.y, x2: to.x, y2: to.y };
    if (isDegenerateLine(line)) return undefined;
    return { kind: 'line', line, style, hasOwnStyle: shape.hasOwnStyle };
  }
  return {
    kind: native.kind,
    box: placeShape(native, doc).box,
    style,
    hasOwnStyle: shape.hasOwnStyle,
  };
}

/**
 * 문구들을 놓을 준비가 끝난 값으로.
 *
 * **글자 크기가 여기서 px 가 된다.** 문서 층은 사용자 단위를 냈고, 이 곱셈은 선 두께가
 * 지나는 **그 비**(`상자 ÷ viewBox`)를 그대로 쓴다 — 크기도 두께도 투영을 지나지 않는 화면
 * 양이므로 축척 1 에서 정확하고 다른 축척에서는 다른 모든 요소와 **같은 방식으로** 어긋난다.
 * 두 번째 비를 지어내면 글자만 도형과 다른 배율로 커진다.
 *
 * **죈 사실을 보고한다.** 패널의 범위(6..160px)는 파서보다 좁고(`canvasEditGeometry`
 * §CANVAS_FONT_SIZE_MIN), 그 죔은 문서가 말한 크기와 화면의 크기가 갈라지는 유일한 자리다.
 */
function planTexts(
  texts: readonly ImportedText[],
  doc: DocumentPlacement,
  ratio: number,
  extra: ImportNote[],
): ImportedTextSpec[] {
  const out: ImportedTextSpec[] = [];
  for (const text of texts) {
    const scaled = text.fontSizeUserUnits * ratio;
    const fontSize = clampCanvasFontSize(scaled);
    if (fontSize !== scaled) extra.push({ kind: 'approximated', reason: 'textSizeClamped', count: 1 });
    out.push({
      at: placePoint(text.x, text.y, doc),
      text: text.text,
      style: { ...text.style, fontSize },
      hasOwnStyle: text.hasOwnStyle,
    });
  }
  return out;
}

/**
 * 명령 총수. **원시형은 한 칸도 먹지 않는다** — 상자 하나로 다 그려지므로 실을 명령이 없다.
 *
 * 상한 검사와 보고가 **같은 함수**를 지난다. 두 곳이 각자 세면 어느 날 한쪽만 원시형을
 * 알아보게 되고, 그때 화면이 말하는 수와 상한이 죄는 수가 갈라진다.
 */
function countCommands(specs: readonly ImportedShapeSpec[]): number {
  return specs.reduce((sum, spec) => sum + (spec.kind === 'path' ? spec.commands.length : 0), 0);
}

/**
 * 보고 한 벌.
 *
 * **`shapes` 는 여전히 "요소가 된 도형 수" 다.** 경로와 원시형을 갈라 세지 않는 것에 뜻이
 * 있다 — 사용자가 읽는 문장은 "도형 N개" 이고, 그 N 이 갑자기 경로만 뜻하면 `<rect>` 만 든
 * 문서에서 "도형 0개" 가 나온다. 갈래는 요소 목록이 줄마다 이름으로 말한다(사각형 · 타원 ·
 * 선 · 경로).
 *
 * **원시형이 되었다는 사실은 보고에 오르지 않는다.** 보고의 갈래는 `approximated` 와
 * `dropped` 둘뿐이고 이것은 어느 쪽도 아니다 — 근사한 것이 없고(치환은 정확하다) 버린 것도
 * 없다. "경로로 남았습니다" 를 적는 안도 기각한다: `<path>` 와 `<polygon>` 이 든 거의 모든
 * 파일에서 울려 보고가 아무것도 말하지 않게 되는 그 부류다(위험 R7 · `<style>` 이 이미
 * 같은 판단을 지났다).
 */
function buildReport(
  specs: readonly ImportedShapeSpec[],
  texts: readonly ImportedTextSpec[],
  notes: readonly ImportNote[],
): ImportReport {
  return {
    shapes: specs.length,
    texts: texts.length,
    commands: countCommands(specs),
    estimatedBytes: estimateBytes(specs, texts),
    notes: mergeNotes(notes),
  };
}

/**
 * UTF-8 바이트 길이. **문자 수가 아니다** — 한글이 든 SVG 는 문자당 3바이트이므로
 * `length` 로 재면 상한이 세 배 헐거워진다.
 */
function byteLength(text: string): number {
  return new TextEncoder().encode(text).length;
}
