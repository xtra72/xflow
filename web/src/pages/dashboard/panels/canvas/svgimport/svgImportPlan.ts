// 가져오기 계획 — viewBox · 상자 · 로컬 정규화 (SPEC-CANVAS-007 M6).
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
// **모든 산출 요소가 같은 상자를 쓴다.** 도형마다 제 잉크의 바운딩 박스를 주는 안을
// 기각한다: 상자 좌표는 정수 캔버스 단위로 반올림되므로 도형마다 최대 0.5 단위씩 **서로
// 어긋난다.** 상자를 공유하면 로컬 좌표가 문서의 뜻을 그대로 유지해 어긋남이 **0** 이고,
// 히트는 어차피 윤곽으로 하므로 겹친 상자가 선택을 흐리지 않으며, **여덟 핸들이 똑같이
// 서는 것 자체가 "이것들은 한 그림이었다" 는 눈에 보이는 표시**가 된다. 계단 오프셋이
// 무리 전체에 한 번만 더해지는 것(REQ-06)도 이 결정의 귀결이다 — 상자가 하나이므로
// 요소마다 더할 자리가 애초에 없다.
//
// @spec SPEC-CANVAS-007 REQ-02 · REQ-03 · REQ-05 · REQ-06 · AC-05 · AC-E8

import {
  coordinate,
  MIN_ELEMENT_EXTENT,
  type BoxGeometry,
  type CanvasSize,
  type ElementStyle,
} from '../canvasConfig';
import { PATH_LOCAL_EXTENT, type PathCommand } from '../shapes/pathTypes';

import { readSvgDocument, type SvgDocumentReader, type ViewBox } from './svgDocument';
import {
  MAX_IMPORT_FILE_BYTES,
  type ImportNote,
  type ImportReport,
  type ImportRefusal,
  type ImportedShape,
} from './svgImportTypes';
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

/** 요소 하나가 될 준비가 끝난 도형. 좌표는 **로컬 정수**이며 상자는 무리가 함께 쓴다. */
export interface ImportedPathSpec {
  readonly commands: readonly PathCommand[];
  readonly style: ElementStyle;
  /** SVG 가 칠을 한 마디라도 말했는가 — 아니면 008 의 `pathSeedStyle` 이 선다. */
  readonly hasOwnStyle: boolean;
}

/** 계획의 산출. **예외가 아니라 값으로 실패가 돌아온다**(REQ-07). */
export type SvgImportPlan =
  | {
      readonly ok: true;
      /** 무리 전체가 함께 쓰는 상자. **계단 오프셋은 아직 더해지지 않았다**(M8 의 몫). */
      readonly box: BoxGeometry;
      readonly shapes: readonly ImportedPathSpec[];
      readonly report: ImportReport;
    }
  | { readonly ok: false; readonly refusal: ImportRefusal };

// --- 바운딩 박스 --------------------------------------------------------

interface Bounds {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

/**
 * 명령 목록의 바운딩 박스 — **제어점까지 함께 센다.**
 *
 * 3차 베지어의 실제 극값은 제어점 볼록 껍질 **안**에 있으므로 이 상자는 참값의 상위집합이다.
 * 극값을 풀어 정확한 상자를 구하는 안을 기각한다: 이 상자가 쓰이는 자리는 `viewBox` 가
 * 없는 문서의 **폴백** 하나뿐이고, 그 자리에서 여백이 조금 넓은 것은 그림을 망치지 않는다.
 * 반대로 상위집합이 아니면 잉크가 상자 밖으로 나간다.
 */
export function commandBounds(commands: readonly PathCommand[]): Bounds | undefined {
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
  for (const cmd of commands) {
    switch (cmd.c) {
      case 'Z':
        break;
      case 'M':
      case 'L':
        include(cmd.x, cmd.y);
        break;
      case 'C':
        include(cmd.x1, cmd.y1);
        include(cmd.x2, cmd.y2);
        include(cmd.x, cmd.y);
        break;
    }
  }
  return bounds;
}

/** 도형 전부의 합집합 바운딩 박스. */
function unionBounds(shapes: readonly ImportedShape[]): Bounds | undefined {
  let union: Bounds | undefined;
  for (const shape of shapes) {
    const bounds = commandBounds(shape.commands);
    if (bounds === undefined) continue;
    union =
      union === undefined
        ? bounds
        : {
            minX: Math.min(union.minX, bounds.minX),
            minY: Math.min(union.minY, bounds.minY),
            maxX: Math.max(union.maxX, bounds.maxX),
            maxY: Math.max(union.maxY, bounds.maxY),
          };
  }
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
  const union = unionBounds(shapes);
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
 */
export function estimateBytes(shapes: readonly ImportedPathSpec[], box: BoxGeometry): number {
  let total = 2; // 배열의 대괄호 둘.
  for (const shape of shapes) {
    total += JSON.stringify({
      id: 'el-00',
      kind: 'path',
      geometry: box,
      path: shape.commands,
      style: shape.style,
    }).length;
  }
  // 원소 **사이**의 쉼표는 `n − 1` 개다. `n` 개로 세면 빈 배열이 3바이트가 되고, 그
  // 한 바이트의 어긋남이 "추정이 실제 직렬화를 재는가" 를 재는 시험을 통과시킨다.
  return total + Math.max(0, shapes.length - 1);
}

// --- 입구 --------------------------------------------------------------

export interface PlanSvgImportOptions {
  /** 문서를 읽는 함수. **주입 가능하다** — 바이트 상한이 파싱 **전에** 있음을 시험이 잰다. */
  readonly readDocument?: SvgDocumentReader;
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
  const { shapes, notes, viewBox, size } = outcome.document;

  const resolved = resolveViewBox(viewBox, size, shapes);
  if (!resolved.ok) {
    return { ok: false, refusal: { reason: resolved.reason, actual: 0, limit: 0 } };
  }

  const box = fitBox(resolved.viewBox, canvas);
  const strokeRatio = box.w / resolved.viewBox.width;
  const specs: ImportedPathSpec[] = shapes.map((shape) => ({
    commands: toLocalCommands(shape.commands, resolved.viewBox),
    style: scaleStroke(shape.style, strokeRatio),
    hasOwnStyle: shape.hasOwnStyle,
  }));

  return { ok: true, box, shapes: specs, report: buildReport(specs, box, notes) };
}

function buildReport(
  specs: readonly ImportedPathSpec[],
  box: BoxGeometry,
  notes: readonly ImportNote[],
): ImportReport {
  return {
    shapes: specs.length,
    commands: specs.reduce((sum, spec) => sum + spec.commands.length, 0),
    estimatedBytes: estimateBytes(specs, box),
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
