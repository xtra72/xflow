// SVG 가져오기의 자료와 상한 (SPEC-CANVAS-007 M1).
//
// **이 모듈은 순수 자료다** — 함수도 DOM 도 없다. 여기 적힌 타입은 전부 `svgimport/` 안에서
// 살다 죽으며 config 로 나가지 않는다. 나가는 것은 `CanvasElement[]` 하나뿐이고, 그 타입은
// 008 이 이미 배달한 것을 한 글자도 바꾸지 않는다(불변식 K12).
//
// **상한 넷을 여기 모아 둔 이유.** 셋(파일 바이트 · 요소 수 · 명령 총수)은 서로 다른 층이
// 읽지만 **함께 정해진 수**다 — 요소 수만 죄면 64개가 전부 256 명령일 때 16,384 명령이
// 되어 예산을 통째로 먹는다. 한 파일에 두면 그 관계가 눈에 보인다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04

import type { ElementStyle } from '../canvasConfig';
import type { PathCommand } from '../shapes/pathTypes';

// --- 상한 ---------------------------------------------------------------

/**
 * 파싱 **전에** 자르는 입력 바이트 상한.
 *
 * 파싱 뒤로 옮기면 내부 엔티티 확장 폭탄(`<!ENTITY a "AAAA">` 를 겹쳐 쓰는 문서)이
 * 먼저 터진다 — 브라우저가 그것을 막는다고 알려져 있으나 본 SPEC 은 그것을 **실측하지
 * 않았다**(가정 A8). 실측하지 않은 방어에 기대지 않는 유일한 길이 파싱 전에 자르는
 * 것이다(위험 R10).
 *
 * 2 MiB 인 이유: 실사용 SVG 아이콘·도면은 수십~수백 KB이고, 그 열 배 이상을 남겨도
 * 파싱 시간이 눈에 띄지 않는다.
 */
export const MAX_IMPORT_FILE_BYTES = 2 * 1024 * 1024;

/**
 * 한 번의 가져오기가 만들 수 있는 요소 수의 상한.
 *
 * 넘으면 **거절한다 — 앞의 64개만 가져오지 않는다.** 도구가 내는 문서 순서는 배경→전경
 * 이라 앞의 64개는 대개 배경 조각들이고, 그 절단은 "설명 없는 틀린 그림" 이다.
 */
export const MAX_IMPORT_ELEMENTS = 64;

/**
 * 한 번의 가져오기가 만들 수 있는 명령 **총수**의 상한.
 *
 * 산술(명령 하나의 실제 직렬화 형상에서 셈):
 * ```
 * {"c":"M","x":1234,"y":5678}                                         = 27 B
 * {"c":"L","x":1234,"y":5678}                                         = 27 B
 * {"c":"C","x1":1234,"y1":5678,"x2":1234,"y2":5678,"x":1234,"y":5678} = 67 B
 * {"c":"Z"}                                                           =  9 B
 * 현실적 혼합(L 60% · C 30% · M/Z 10%, 쉼표 포함) ≈ 39 B/명령
 *   → 640 × 39                    ≈ 25.0 KB
 * 요소 한 건의 명령 밖 부분(id · kind · geometry · style) ≈ 135 B
 *   → 64 × 135                    ≈  8.6 KB
 * 합                              ≈ 33.6 KB = 256KB 예산의 약 13.1%
 * ```
 *
 * **최악은 실측이다**(문서 → 계획 → 요소를 실제로 지나 `JSON.stringify` 의 UTF-8 바이트를
 * 잰 값). 손으로 센 수를 그대로 두지 않는 이유는 이 파일이 이미 적은 그것이다 — 상수는
 * 형상이 바뀌는 날 조용히 틀린다.
 * ```
 * 경로 64개 × 10 C-명령(명령 640 전부)        = 49,971 B = 19.06%
 * 문구 64개 × 256자 한글                      = 57,370 B = 21.88%
 * 문구 64개 × 256자 ASCII                     = 23,514 B =  8.97%
 * **최악** 경로 3개(명령 640 전부) + 문구 61개(256자 한글)
 *                                             = 97,465 B = 37.18%
 * ```
 * 마지막 줄이 최악인 이유가 두 상한의 관계를 드러낸다: 명령을 **가장 적은 경로에 몰면**
 * 요소 자리가 남고, 남은 자리를 글자로 채울 수 있다. 그 문서는 66KB 짜리 SVG 이며 —
 * 이름표 61개가 저마다 한글 256자인, 실사용에 없는 형상이다. 그래도 **적어 둔다**:
 * 예산이 무엇으로 차는지를 모른 채 상한을 다시 정하는 것이 가장 나쁘다(위험 R6).
 * **이 수가 죄는 것은 저장 크기만이 아니다.** 빗나간 클릭 한 번이 모든 경로 요소를
 * 평탄화하므로(`hitTest` 가 처음 맞는 것에서 멈춘다 — 실측), 명령 총수는 포인터 사건당
 * 비용의 상한이기도 하다. 그 아래를 받쳐 줄 두 번째 죔쇠(공간 색인·캐시)가 이 저장소에
 * 없다(위험 R5).
 */
export const MAX_IMPORT_COMMANDS = 640;

/**
 * 문구 요소 하나가 실을 수 있는 **글자 수**의 상한.
 *
 * 명령 상한이 죄지 못하는 축이 이것이다 — 문구 요소는 명령을 하나도 나르지 않으므로
 * `MAX_IMPORT_COMMANDS` 아래를 자유롭게 지나가고, 그동안 제 문자열로 예산을 먹는다.
 * 상한이 없으면 2MiB 짜리 문서의 글자가 거의 그대로 config 로 흘러 들어가 저장이 413 으로
 * 실패한다(위험 R6 와 같은 부류이며, 다른 축이다).
 *
 * 256 인 이유: 도면의 이름표는 "회의실 A" · "3층 기계실" 처럼 짧고, 그 열 배 이상을 남겨도
 * 문구만의 최악(64 × 256자 한글)이 **실측 57,370 B**(256KB 예산의 21.88%)로 경로 쪽 최악
 * (49,971 B · 19.06%)과 같은 자리에 선다. **넘으면 자르고 보고한다** — 이름표 하나 때문에
 * 문서 전체를 거절하는 것은 대가가 크다.
 *
 * 둘을 섞은 최악은 그보다 크다(97,465 B · 37.18% — `MAX_IMPORT_COMMANDS` 머리말의 실측표).
 * 그 수를 여기서 더 죄지 않는 것은 **화면이 놓기 전에 그 크기를 말하기 때문**이다: 추정이
 * UTF-8 바이트를 세므로(`estimateBytes`) 사용자는 "약 96KB" 를 읽고 판단할 수 있다.
 */
export const MAX_IMPORT_TEXT_LENGTH = 256;

/**
 * `<use>` 전개의 깊이 상한.
 *
 * 순환 검출과 **둘 다** 둔다 — 순환이 아닌 깊은 사슬(도구가 낸 중첩 심볼)도 멈춰야 하고,
 * 깊이 상한만으로는 상한 안에서 도는 순환의 비용이 지수로 커진다.
 */
export const MAX_USE_DEPTH = 4;

/**
 * 호 한 조각의 최대 중심각(라디안).
 *
 * `k = (4/3)·tan(δ/4)` 에서 `δ = π/2` 일 때 최대 반경 오차가 `0.027% × max(rx, ry)` 다.
 * **45° 로 더 잘게 자르는 안을 기각한다** — 오차는 대략 `δ⁶` 로 줄어 1/64 이 되지만
 * **명령 수가 두 배**가 되고 그 수는 `MAX_PATH_COMMANDS` 라는 죔쇠를 직접 먹는다.
 * 90° 의 오차가 이미 평탄화 허용 오차(`FLATTEN_TOLERANCE_PX`)의 1/34 이므로 살 이유가 없다
 * (불변식 K5).
 */
export const ARC_SEGMENT_MAX_RAD = Math.PI / 2;

// --- 보고 --------------------------------------------------------------

/**
 * 보고의 갈래. **`'imported'` 갈래를 두지 않는 것이 요점이다** — 들어온 것은 개수로
 * 요약되지 항목으로 나열되지 않는다. 그리고 `<defs>` 처럼 **원래 그려지지 않는 것**은
 * 어느 갈래에도 오르지 않는다(그리지 않는 것을 버렸다고 말하면 보고가 잡음이 된다).
 */
export type ImportNoteKind = 'approximated' | 'dropped';

/**
 * 보고 항목의 사유. **문자열이 아니라 합집합인 이유**: 오타 난 사유는 화면에 키가 그대로
 * 나오고 시험은 초록으로 통과한다. 이 저장소가 i18n 에서 이미 물린 부류다.
 *
 * 이 값은 i18n 키가 **아니다** — 키 이름 안에 점을 넣지 않는 규율을 지키기 위해, 화면
 * 층(M9·M10)이 이 사유를 제 키로 옮긴다.
 */
export type ImportNoteReason =
  // --- 근사됨 ---
  /** `fill="url(#g)"` → 참조된 그라디언트의 첫 `stop-color` 단색. */
  | 'gradientToSolid'
  /** 색을 알 수 없어(`currentColor` 등) 씨앗 색으로 떨어뜨렸다. */
  | 'paintUnresolved'
  /** 알파를 색 문자열에 접을 수 없는 색 표기(이름 색 등)라 알파가 떨어졌다. */
  | 'opacityUnfoldable'
  /** 비균등 배율 아래의 선 두께를 `√|det|` 스칼라로 옮겼다. */
  | 'nonUniformStrokeScale'
  /** `fill-rule="evenodd"` 를 포함 관계 기반 감김 뒤집기로 옮겼다(성공해도 오른다). */
  | 'evenOddWinding'
  /** 그룹 `opacity` 를 자식마다 곱했다 — 겹친 자리가 진해진다. */
  | 'groupOpacityPerChild'
  /** 문서의 `preserveAspectRatio` 를 읽지 않았다. */
  | 'preserveAspectRatioIgnored'
  /** 최외곽 `<svg>` 의 `transform` 을 적용하지 않았다(SVG 1.1 이 정의하지 않는다). */
  | 'rootTransformIgnored'
  /** `<use>` 의 `width`/`height` 를 무시했다. */
  | 'useSizeIgnored'
  /**
   * 문서가 말한 활자를 그대로 옮기지 못했다 — 패널은 제 글꼴 하나로 그린다.
   *
   * `font-family` · `font-style` · `letter-spacing` · `text-decoration` · `writing-mode` ·
   * `textLength` 를 말했거나, `font-size` 의 단위를 읽지 못했거나, `dominant-baseline` ·
   * `alignment-baseline` 이 가운데가 아닌 값일 때 문구 하나마다 한 번 오른다.
   */
  | 'textFontIgnored'
  /** 옮긴 글자 크기가 패널의 범위(6..160px) 밖이라 죄였다. */
  | 'textSizeClamped'
  /** 문구 안에 `{value}` · `{name}` · `{unit}` 이 있어 패널이 그것을 치환한다. */
  | 'textTemplateToken'
  /** 문구를 문서에서 뒤따르던 도형들 **위**로 옮겼다 — 배열 순서가 유일한 z-order 다. */
  | 'textOrderChanged'
  // --- 버림 ---
  /**
   * 글자를 요소로 세우지 못했다 — `<textPath>` 처럼 **곧은 한 줄이 아닌** 글자다.
   *
   * `<text>` 가 요소로 들어오게 된 뒤로 이 사유는 "글자를 지원하지 않는다" 가 아니라
   * "이 글자는 옮길 길이 없다" 를 뜻한다.
   */
  | 'textDropped'
  /** `MAX_IMPORT_TEXT_LENGTH` 를 넘어 잘린 문구. */
  | 'textTruncated'
  /** `<tspan>` 이 제 자리(`x`/`y`/`dx`/`dy`)나 제 스타일을 말했다 — 내용만 한 줄로 남는다. */
  | 'tspanDropped'
  | 'imageDropped'
  | 'foreignObjectDropped'
  | 'nestedSvgDropped'
  | 'styleRuleDropped'
  | 'filterDropped'
  | 'clipPathDropped'
  | 'maskDropped'
  /** 점선이 실선이 되는 것은 "조금 다른" 것이 아니라 다른 그림이다. */
  | 'dashArrayDropped'
  /** `display:none` · `visibility:hidden` — 요소를 만들지 않는다. */
  | 'hiddenDropped'
  /** 감김 무리로 나눈 뒤에도 `MAX_PATH_COMMANDS` 를 넘는 도형. */
  | 'commandLimitDropped'
  /** `<use>` 깊이 상한 또는 순환에서 끊긴 가지. */
  | 'useDepthDropped'
  /** 외부 문서 참조 — 가져오면 "브라우저를 떠나지 않는다" 가 깨진다. */
  | 'externalRefDropped'
  /** `|det| = 0` — 그려질 것이 없는 퇴화 변환. */
  | 'degenerateTransformDropped';

/** 보고 한 줄. **개수를 반드시 든다** — "필터가 있습니다" 가 아니라 "필터 3개". */
export interface ImportNote {
  readonly kind: ImportNoteKind;
  readonly reason: ImportNoteReason;
  readonly count: number;
}

/** 놓기 전에 화면이 말하는 것 전부. */
export interface ImportReport {
  /** 들어옴 — 요소가 된 도형 수. */
  readonly shapes: number;
  /** 들어옴 — 요소가 된 문구 수. 도형과 **갈라 세는** 것은 둘이 나르는 것이 다르기
   * 때문이다: 도형은 명령을, 문구는 글자를 나른다. 합쳐 세면 "명령 0개인 요소 5개" 가
   * 화면에서 설명 없는 수가 된다. */
  readonly texts: number;
  /** 들어옴 — 명령 총수. */
  readonly commands: number;
  /** 들어옴 — 직렬화 추정 바이트. */
  readonly estimatedBytes: number;
  /** 근사됨 + 버림. 갈래별로 갈라 보이는 것은 화면의 몫이다. */
  readonly notes: readonly ImportNote[];
}

/** 가져오기 자체가 서지 않는 경우의 값. **예외가 아니라 값으로 돌아온다**(REQ-07). */
export interface ImportRefusal {
  readonly reason: ImportRefusalReason;
  /** 실제 수. 상한과 **함께** 말해야 사용자가 얼마나 줄일지 안다. */
  readonly actual: number;
  readonly limit: number;
}

export type ImportRefusalReason =
  | 'fileTooLarge'
  /**
   * 파일을 **읽지** 못했다(`FileReader` 실패 — 읽는 도중 지워짐 · 권한 · 장치 오류).
   *
   * `notSvg` 와 합치지 않는 것에 뜻이 있다: 하나는 **문서가 나쁘다**이고 다른 하나는 **문서를
   * 보지 못했다**이며, 사용자가 할 일이 다르다(고쳐 내보내기 vs 다시 고르기). 합치면 멀쩡한
   * 파일을 두고 "SVG 가 아닙니다" 를 읽는다.
   *
   * 화면 층만 이 사유를 낸다 — 파싱 층은 이미 문자열을 받으므로 읽기에 실패할 자리가 없다.
   */
  | 'unreadable'
  | 'notSvg'
  | 'emptyDocument'
  | 'degenerateViewBox'
  | 'tooManyElements'
  | 'tooManyCommands';

// --- 도형 --------------------------------------------------------------

/**
 * 사용자 단위 좌표의 도형 하나. **아직 요소가 아니다.**
 *
 * `commands` 가 `PathCommand[]` 인데 좌표가 **로컬 정수가 아니라 사용자 단위 실수**라는
 * 것이 이 타입에서 가장 잘 오해되는 자리다. 그 타입을 재사용하는 것은 어휘가 같기
 * 때문이고(`M`·`L`·`C`·`Z`), 로컬 정수로 옮기는 곱셈·반올림은 **한 곳**(M6 의 로컬
 * 정규화)에서만 일어난다. 두 번째 명령 타입을 만드는 안을 기각한다 — 그러면 축약 산술이
 * 두 타입을 오가며 변환 함수가 하나 더 생긴다.
 */
export interface ImportedShape {
  /** 사용자 단위. 조상 `transform` 은 **이미 좌표에 녹아 있다**. */
  readonly commands: readonly PathCommand[];
  /** 알파는 색 문자열에 접혀 있다. */
  readonly style: ElementStyle;
  /** `Z` 가 하나라도 있는가. */
  readonly closed: boolean;
  /** SVG 가 그림 스타일을 한 마디라도 말했는가. 아니면 008 의 `pathSeedStyle` 이 선다. */
  readonly hasOwnStyle: boolean;
  /** `fill-rule="evenodd"` 인가 — 감김 무리 뒤집기의 입력. */
  readonly evenOdd: boolean;
}

/**
 * 사용자 단위 좌표의 문구 하나. **아직 요소가 아니다.**
 *
 * `commands` 가 아니라 `x`/`y` 한 점을 드는 것이 이 타입의 요점이다 — 캔버스의 문구 요소는
 * `PointGeometry`(정렬 기준점) 위에 서고, 상자를 갖지 않는다. 글자의 **폭과 높이는 이 층에서
 * 잴 수 없다**: `getBBox` 는 jsdom 에 없고(불변식 K4) `measureText` 는 2D context 를 요구하며,
 * 그 둘 없이 상자를 지어내면 그것은 문서의 값이 아니라 우리가 고른 값이다.
 *
 * 그래서 문구는 `viewBox` 폴백의 합집합에도 **들지 않는다** — 잴 수 없는 것으로 문서의
 * 크기를 정할 수 없다.
 */
export interface ImportedText {
  /** 사용자 단위 정렬 기준점. 조상 `transform` 은 **이미 좌표에 녹아 있다**. */
  readonly x: number;
  readonly y: number;
  /** 한 줄로 편 글자. 공백은 SVG 의 `xml:space` 규칙으로 이미 정규화되어 있다. */
  readonly text: string;
  /**
   * 알파가 색에 접힌 스타일 — `textColor` · `opacity` · `fontWeight` · `align`.
   *
   * **`fontSize` 는 여기 없다.** 그 값은 아직 사용자 단위이고 `ElementStyle.fontSize` 는
   * px 라, 한 필드에 담는 순간 계획 층이 축척을 두 번 곱하거나 한 번도 곱하지 않는 길이 열린다.
   */
  readonly style: ElementStyle;
  /** **사용자 단위** 글자 크기. 조상 `transform` 의 배율은 이미 곱해져 있다. */
  readonly fontSizeUserUnits: number;
  /** SVG 가 칠을 한 마디라도 말했는가. 아니면 002 의 문구 씨앗 색이 선다. */
  readonly hasOwnStyle: boolean;
}
