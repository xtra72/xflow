// 화면 층이 쓰는 순수 조각들 — 문구 키 표 · 채우기 · 미리보기 크기 · 파일 읽기
// (SPEC-CANVAS-007 M9 · M10).
//
// **컴포넌트 파일에서 갈라 낸 이유는 둘이다.** 하나는 규율이다 — 이 저장소의 lint 는 컴포넌트
// 파일이 컴포넌트 아닌 것을 내보내면 경고한다(`react-refresh/only-export-components`), 그리고
// 그 경고는 옳다: 여기 있는 것들은 렌더 없이 시험되는 순수 함수와 상수이므로 DOM 을 세우지
// 않고 잰다. 다른 하나는 이 SPEC 이 내내 지킨 층 가르기다(산술과 DOM 을 섞지 않는다).
//
// **사유 → 문구 키 표를 리터럴 `Record` 로 두는 것이 이 파일의 요점이다.** 사유 문자열을
// 그대로 키로 조립하는 안(`` `${EDIT}.importNote${reason}` ``)을 기각한다: 오타 난 키는 화면에
// 원문 키를 내고 시험은 초록으로 통과한다. 리터럴 표는 총망라를 **컴파일러가** 요구하므로,
// 사유를 하나 더하면 표를 채우지 않고는 빌드가 서지 않는다 — `svgImportTypes` 가 사유를
// 문자열이 아니라 합집합으로 둔 그 결정의 짝이다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04

import type { CanvasSize } from '../canvasConfig';

import type { ImportNoteReason, ImportRefusal, ImportRefusalReason } from './svgImportTypes';

/** 문구 키의 앞자리. 키 이름 **안에는 점을 넣지 않는다**(프로젝트 규약 · `i18nKeyShape.test.ts`). */
export const EDIT = 'dashboard.canvas.edit';

/** 보고 사유 → 문구 키. 총망라를 컴파일러가 요구한다(위 머리말). */
export const NOTE_KEYS: Readonly<Record<ImportNoteReason, string>> = {
  gradientToSolid: `${EDIT}.importNoteGradientToSolid`,
  paintUnresolved: `${EDIT}.importNotePaintUnresolved`,
  opacityUnfoldable: `${EDIT}.importNoteOpacityUnfoldable`,
  nonUniformStrokeScale: `${EDIT}.importNoteNonUniformStrokeScale`,
  evenOddWinding: `${EDIT}.importNoteEvenOddWinding`,
  groupOpacityPerChild: `${EDIT}.importNoteGroupOpacityPerChild`,
  preserveAspectRatioIgnored: `${EDIT}.importNotePreserveAspectRatioIgnored`,
  rootTransformIgnored: `${EDIT}.importNoteRootTransformIgnored`,
  useSizeIgnored: `${EDIT}.importNoteUseSizeIgnored`,
  textDropped: `${EDIT}.importNoteTextDropped`,
  imageDropped: `${EDIT}.importNoteImageDropped`,
  foreignObjectDropped: `${EDIT}.importNoteForeignObjectDropped`,
  nestedSvgDropped: `${EDIT}.importNoteNestedSvgDropped`,
  styleRuleDropped: `${EDIT}.importNoteStyleRuleDropped`,
  filterDropped: `${EDIT}.importNoteFilterDropped`,
  clipPathDropped: `${EDIT}.importNoteClipPathDropped`,
  maskDropped: `${EDIT}.importNoteMaskDropped`,
  dashArrayDropped: `${EDIT}.importNoteDashArrayDropped`,
  hiddenDropped: `${EDIT}.importNoteHiddenDropped`,
  commandLimitDropped: `${EDIT}.importNoteCommandLimitDropped`,
  useDepthDropped: `${EDIT}.importNoteUseDepthDropped`,
  externalRefDropped: `${EDIT}.importNoteExternalRefDropped`,
  degenerateTransformDropped: `${EDIT}.importNoteDegenerateTransformDropped`,
};

/** 거절 사유 → 문구 키. 같은 규율. */
export const REFUSAL_KEYS: Readonly<Record<ImportRefusalReason, string>> = {
  fileTooLarge: `${EDIT}.importRefusedFileTooLarge`,
  unreadable: `${EDIT}.importRefusedUnreadable`,
  notSvg: `${EDIT}.importRefusedNotSvg`,
  emptyDocument: `${EDIT}.importRefusedEmptyDocument`,
  degenerateViewBox: `${EDIT}.importRefusedDegenerateViewBox`,
  tooManyElements: `${EDIT}.importRefusedTooManyElements`,
  tooManyCommands: `${EDIT}.importRefusedTooManyCommands`,
};

/**
 * 바이트를 KB 로. **0KB 라고 말하지 않는다** — 작은 파일도 자리를 차지하며, "0KB 는 상한
 * 2048KB 를 넘습니다" 같은 문장이 나오면 화면이 거짓말을 한다.
 */
export function kb(bytes: number): number {
  return Math.max(1, Math.round(bytes / 1024));
}

/**
 * 거절 문구를 채운다.
 *
 * **`replaceAll` 이다.** 상한 문구 셋이 상한을 **두 번** 말하므로(`{limit}` · `{limitKb}`),
 * `replace` 는 첫 자리만 바꾸고 뒤쪽에 치환자가 벌거벗은 채 남는다. 이 저장소가 이미 두 번
 * 물린 자리다(도크의 `gridStepPartial` · 서랍의 한도 안내 · 위험 R14).
 *
 * 네 이름을 **모두** 갈아 끼우는 것에 뜻이 있다 — 사유마다 쓰는 이름이 다르고(바이트는 KB,
 * 나머지는 개수), 사유별로 치환 표를 갈라 두면 새 사유가 어느 표에도 들지 않은 채 통과한다.
 */
export function refusalText(template: string, refusal: ImportRefusal): string {
  return template
    .replaceAll('{actual}', String(refusal.actual))
    .replaceAll('{limit}', String(refusal.limit))
    .replaceAll('{actualKb}', String(kb(refusal.actual)))
    .replaceAll('{limitKb}', String(kb(refusal.limit)));
}

/**
 * 미리보기 상자의 최대 변(CSS px). 도크 폭이 `w-44`(176px)이고 좌우 `p-2` 가 16px 을 먹으므로
 * 152 가 남는 폭 그대로다. 세로도 같은 수로 죄어 아주 납작하거나 아주 긴 캔버스가 도크를
 * 밀어내지 못하게 한다.
 */
export const PREVIEW_MAX_PX = 152;

/**
 * 캔버스 종횡비를 **지켜** 최대 변 안에 담은 미리보기 크기.
 *
 * 고정 크기 상자에 캔버스를 밀어 넣는 안을 기각한다 — 그러면 500×400 이 아닌 캔버스에서
 * 미리보기만 눌려 보이고 사용자는 "가져오기가 그림을 일그러뜨렸다" 고 읽는다. 상자 종횡비를
 * 문서 종횡비로 맞춘 §결정 3 이 지키려던 바로 그 성질이 미리보기에서 깨지면 그 결정이
 * 화면에서 반증된다.
 */
export function fitPreviewSize(canvas: CanvasSize): { width: number; height: number } {
  const w = canvas.width > 0 ? canvas.width : 1;
  const h = canvas.height > 0 ? canvas.height : 1;
  const scale = Math.min(PREVIEW_MAX_PX / w, PREVIEW_MAX_PX / h);
  return {
    width: Math.max(1, Math.round(w * scale)),
    height: Math.max(1, Math.round(h * scale)),
  };
}

/**
 * `File` 하나를 문자열로.
 *
 * **jsdom 의 `File` 에는 `text()` 가 없다**(측정: `typeof file.text === 'undefined'`) — 그 사실을
 * 모른 채 `File.text()` 에 기대면 시험 환경에서만 죽는 코드가 된다. `FileReader` 는 jsdom 에도
 * 있으므로 그것을 쓰고, 그 자리를 **주입 가능하게** 둔다(`heatmap/imageAsset.ts` 의 형상).
 *
 * 실패는 **값이 아니라 거부된 약속**으로 돌아온다. 이 함수의 실패는 파싱 실패와 성질이
 * 다르며(문서가 나쁜 것이 아니라 읽지 못한 것이다) 호출부가 그 둘을 다른 문구로 말한다.
 */
export function readSvgFileText(
  file: File,
  reader: FileReader = new FileReader(),
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    reader.onload = () => {
      const result = reader.result;
      if (typeof result === 'string') resolve(result);
      else reject(new Error('FileReader 가 문자열을 반환하지 않았습니다'));
    };
    reader.onerror = () => reject(reader.error ?? new Error('FileReader 가 파일을 읽지 못했습니다'));
    reader.readAsText(file);
  });
}
