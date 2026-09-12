// SVG 가져오기 묶음 — 도크 안의 파일 고르기 · 미리보기 · 보고 · 놓기 (SPEC-CANVAS-007 M9).
//
// **이 파일은 도크 안에서만 산다.** 도크 자리(`canvasEditDockHost`)가 없으면 오버레이가 도크
// 본문을 아예 그리지 않고 이 컴포넌트는 그 본문의 자식이다 — 대시보드에 놓인 패널에는 파일
// 입력도 미리보기도 보고도 **하나도 없다**(불변식 K10 · 006 I23).
//
// 그 "없음" 이 006 이 배달한 결함의 되풀이가 아닌 이유를 이름으로 적는다: 가져오기는 **표시
// 층이 아니라 컨트롤**이고 캔버스 표면에 아무것도 그리지 않으므로 **다스릴 층 자체를 만들지
// 않는다.** 006 이 깬 형상은 "조건 없이 그려지는 층 + 도크 뒤에만 있는 손잡이" 였는데, 여기
// 에는 그 조합이 만들어질 자리가 없다. 놓인 경로 요소를 다스리는 컨트롤(선택 · 드래그 ·
// 8핸들 · 정렬 · 순서)은 006 M7 이 넓혀 둔 그대로 대시보드에서도 닿는다.
//
// **그래서 캔버스 위로 파일을 끌어놓지 않는다**(spec.md §결정 6 · §I23). 드롭을 받으면 드롭
// 강조가 **다스릴 수 없는 표시 층**이 되어 006 의 결함이 그대로 재현된다. 이 금지는 편의의
// 문제가 아니라 I23 의 직접 귀결이며, 덤으로 WCAG 2.2 SC 2.5.7(끌기 동작)의 등가물 요구도
// 발생하지 않는다 — 파일 고르기는 끌기가 아니다.
//
// **미리보기는 실제 렌더 경로다**(004 §심볼 고르기 UI 가 세우고 008 이 카탈로그·서랍에 적용한
// 그 규율). 별도 썸네일도 별도 그리기 코드도 만들지 않고 `appendImportedElements` 로 **놓을
// 그 요소들을 그대로 만들어** `drawElements` 에 넘긴다. 그래서 씨앗 스타일 판정(닫힘이면 채움,
// 열림이면 선)도 한 벌이고, "미리보기는 멀쩡한데 놓으면 다른 그림" 이 생길 자리가 없다.
//
// **보고 요약 한 줄은 접히지 않는다**(REQ-04). 갈래별 목록만 접힌다. 토스트로 띄우고 사라지게
// 하는 안을 기각한다 — 사용자가 [놓기] 를 누르기 **전에** 읽어야 하는 정보인데, 사라지는 것은
// 결정 **뒤에** 읽힌다.
//
// **묶음은 기본으로 접혀 있다**(AC-E1). 도크 폭이 `w-44` 고정이라 펼쳐 두면 자주 쓰는 도형
// 넷이 스크롤 아래로 밀린다 — 008 이 카탈로그 묶음 셋에 대해, 006 이 배율 칸에 대해 한 그
// 판단이다.
//
// **접힘 상태를 저장소에 얹지 않는다.** 팔레트 묶음 넷은 `xflow-canvas-palette` 에 남지만
// 가져오기는 **한 번 쓰고 닫는 일**이라 다음 세션까지 펴 둘 이유가 없고, 얹으면 그 저장 형상
// (`PaletteGroupId` 넷)이 다섯이 되어 008 이 세운 표를 넓히게 된다.
//
// **파일 읽기는 주입 가능하다**(`heatmap/imageAsset.ts` 의 형상). jsdom 의 `File` 에는
// `text()` 가 **없다**(측정: `typeof file.text === 'undefined'`) — 그 사실을 모른 채 `File.text()`
// 에 기대면 시험 환경에서만 죽는 코드가 된다. 기본 구현은 `FileReader.readAsText` 이고 시험은
// 그 자리를 갈아 끼운다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04 · REQ-06 · REQ-07 · AC-07 · AC-09 · AC-E11

import { useEffect, useId, useMemo, useRef, useState } from 'react';

import { ChevronDown, ChevronRight, Check, FileUp, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { getDashboardLimits, type DashboardLimits } from '@/services/api/dashboardLimitsService';

import type { CanvasElement, CanvasSize } from '../canvasConfig';
import { appendImportedElements } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import { clearSurface, drawElements, type DrawContext2D } from '../drawElement';
import {
  EDIT,
  NOTE_KEYS,
  REFUSAL_KEYS,
  fitPreviewSize,
  kb,
  readSvgFileText,
  refusalText,
} from './svgImportPresent';
import { planSvgImport, type ImportedShapeSpec, type ImportedTextSpec } from './svgImportPlan';
import {
  MAX_IMPORT_FILE_BYTES,
  resolveImportLimits,
  type ImportNote,
  type ImportRefusal,
  type ImportReport,
} from './svgImportTypes';

// --- 겉모습 --------------------------------------------------------------

/** 묶음 머리 — 팔레트 묶음과 **같은 껍데기**다. 하나만 다르면 "이건 왜 접히지" 를 묻게 된다. */
const GROUP_HEAD_CLASS =
  'flex w-full items-center gap-1 rounded px-1 py-1 text-left text-[11px] font-medium ' +
  'text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/** 도크의 줄 버튼과 같은 눈금 — 한 도구의 여러 표현이 같은 크기를 쓴다. */
const ROW_BUTTON_CLASS =
  'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300 ' +
  'disabled:pointer-events-none disabled:opacity-40';

/** 파일 고르기 — `<label>` 이 버튼처럼 보인다. 안의 `<input>` 은 화면에서만 숨는다. */
const PICK_LABEL_CLASS =
  'flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus-within:outline-none focus-within:ring-2 focus-within:ring-blue-300';

const SECTION_TITLE_CLASS = 'px-1 pb-1 text-[11px] font-medium text-(--color-text-muted)';

const SUMMARY_CLASS = 'px-1 text-[11px] leading-tight text-(--color-text-secondary)';

const HINT_CLASS = 'px-1 text-[11px] leading-tight text-(--color-text-muted)';

const NOTE_LIST_CLASS = 'flex flex-col gap-0.5 px-1 pb-0.5';

const NOTE_HEAD_CLASS = 'text-[11px] font-medium text-(--color-text-muted)';

const NOTE_ITEM_CLASS = 'text-[11px] leading-tight text-(--color-text-muted)';

const ICON_CLASS = 'h-4 w-4 shrink-0';

const CHEVRON_CLASS = 'h-3 w-3 shrink-0';

// --- 미리보기 ------------------------------------------------------------

/**
 * 뒷면 배율. DPR 을 읽지 않고 2 로 고정하는 것은 이 칸이 **그림이 아니라 확인용**이기
 * 때문이다 — 표면(`CanvasSurface`)이 DPR 을 읽는 것과 달리 여기서는 선명도 한 눈금이면
 * 족하고, DPR 을 읽으면 이 파일이 표면의 그 배선을 한 벌 더 갖게 된다(008 카탈로그와 같은 판단).
 */
const PREVIEW_SCALE = 2;

/**
 * 놓을 요소 그대로를 그린다.
 *
 * 2D context 를 얻지 못하면 **조용히 빈 칸으로 남는다.** jsdom 이 그 자리이고(시험 환경),
 * 실제 브라우저에서도 컨텍스트 소진 같은 이유로 `null` 이 올 수 있다. 미리보기 하나가 가져오기
 * 전체를 무너뜨리지 않아야 한다.
 */
function ImportPreview({
  elements,
  canvas,
}: {
  /** 미리보기가 그리는 것 — **경로만이 아니다**(결함 B 정정: 문구도 요소로 들어온다). */
  elements: readonly CanvasElement[];
  canvas: CanvasSize;
}): React.ReactElement {
  const ref = useRef<HTMLCanvasElement | null>(null);
  const size = fitPreviewSize(canvas);
  const projection: CanvasProjection = {
    stage: { width: size.width * PREVIEW_SCALE, height: size.height * PREVIEW_SCALE },
    canvas,
  };

  useEffect(() => {
    const node = ref.current;
    // **과잉 가드다**(마운트된 `<canvas>` 의 ref 는 효과가 도는 시점에 언제나 있다). 그럼에도
    // 두는 것은 타입이 `null` 을 담기 때문이며, 008 의 카탈로그 미리보기가 같은 자리에 같은
    // 줄을 둔다 — 두 미리보기가 다른 모양을 하면 다음 사람이 어느 쪽이 옳은지 묻게 된다.
    if (node === null) return;
    const ctx = node.getContext('2d') as DrawContext2D | null;
    if (ctx === null) return;
    clearSurface(ctx, { ...projection.stage, scale: PREVIEW_SCALE });
    // 규칙이 붙지 않은 채로 그린다 — 가져온 요소에는 아직 데이터가 매이지 않았으므로
    // `drawElements` 가 요소 자신의 스타일로 떨어진다(`styles[id] ?? el.style`).
    drawElements(ctx, elements, {}, {}, projection);
  });

  return (
    <canvas
      ref={ref}
      data-testid="canvas-svg-import-preview"
      width={projection.stage.width}
      height={projection.stage.height}
      style={{ width: size.width, height: size.height }}
      // 그림은 이름을 나르지 않는다 — 무엇이 들어오는지는 아래 요약 한 줄이 말한다.
      aria-hidden="true"
      className="mx-auto rounded border border-(--color-border-default)"
    />
  );
}

// --- 상태 ---------------------------------------------------------------

/** 준비된 가져오기 하나 — 계획의 성공 갈래 그대로다. */
interface ReadyImport {
  /** 도형마다 제 상자를 든다 — 무리가 함께 쓰는 상자는 없다(결함 D3 정정). */
  readonly shapes: readonly ImportedShapeSpec[];
  /** 문구들. **도형이 하나도 없어도 이쪽이 있으면 놓을 것이 있다**(결함 B 정정). */
  readonly texts: readonly ImportedTextSpec[];
  readonly report: ImportReport;
}

/**
 * 놓을 것의 수. **도형만 세면 글자뿐인 문서에서 단추가 죽는다** — 그 문서는 가져올 것이
 * 없는 문서가 아니라 가져올 것이 **글자인** 문서다.
 */
function placeableCount(ready: ReadyImport): number {
  return ready.shapes.length + ready.texts.length;
}

type ImportState =
  | { readonly phase: 'idle' }
  | { readonly phase: 'ready'; readonly ready: ReadyImport }
  | { readonly phase: 'refused'; readonly refusal: ImportRefusal };

// --- 묶음 ---------------------------------------------------------------

export interface CanvasSvgImportProps {
  /** 캔버스 좌표계 크기 — 상자를 담을 자리이자 미리보기의 좌표계다. */
  canvas: CanvasSize;
  /**
   * 놓는다. **요소를 만드는 일은 이 컴포넌트가 하지 않는다**(불변식 K9) — 만드는 입구는
   * `canvasElementFactory` 하나이고, 놓은 뒤의 선택은 오버레이가 소유한다.
   */
  onPlace: (
    shapes: readonly ImportedShapeSpec[],
    texts: readonly ImportedTextSpec[],
  ) => void;
  /** 파일을 문자열로 읽는 함수. 시험이 갈아 끼운다(기본은 `FileReader`). */
  readFile?: (file: File) => Promise<string>;
  /**
   * 서버가 적용 중인 상한을 묻는 함수. 시험이 갈아 끼운다(@SPEC:SPEC-CANVAS-007 §결정 14).
   *
   * `readFile` 과 **같은 형상으로** 주입받는 것이 요점이다 — 이 컴포넌트는 이미 바깥
   * 세계를 인자로 받는 규율을 갖고 있고, 그 규율에 하나를 더하는 것이 새 기구를
   * 들이는 것보다 싸다.
   */
  fetchLimits?: () => Promise<DashboardLimits>;
}

/**
 * 가져오기 묶음 하나. 상태 셋(대기 · 준비됨 · 거절됨)을 갖되 **요소도 선택도 갖지 않는다** —
 * 그 둘은 오버레이의 것이고, 이 컴포넌트가 내놓는 것은 "놓을 준비가 된 계획" 하나다.
 */
export function CanvasSvgImport({
  canvas,
  onPlace,
  readFile = readSvgFileText,
  fetchLimits = getDashboardLimits,
}: CanvasSvgImportProps): React.ReactElement {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(true);
  const [notesOpen, setNotesOpen] = useState(false);
  const [state, setState] = useState<ImportState>({ phase: 'idle' });
  const bodyId = useId();
  const titleId = useId();
  const notesId = useId();

  /**
   * 미리보기가 그릴 요소 — **놓을 그것 그대로**다. 빈 배열에 붙이므로 계단 오프셋이 0 이고
   * (`seedOffset(0)`), 그래서 미리보기의 자리가 곧 놓았을 때의 자리다(이미 놓인 요소가 있으면
   * 계단만큼 밀린다 — 그 차이는 한 자리 수 단위이고 미리보기의 목적은 자리가 아니라 그림이다).
   */
  const preview = useMemo(() => {
    if (state.phase !== 'ready') return [];
    // **놓을 그것 그대로** — 도형과 문구를 같은 입구에서 만들고, 그 입구가 정한 순서
    // (도형 먼저, 문구가 그 위)를 미리보기도 그대로 그린다.
    const made = appendImportedElements([], state.ready.shapes, state.ready.texts);
    return [...made.created, ...made.createdTexts];
  }, [state]);

  const pick = async (file: File | undefined, input: HTMLInputElement): Promise<void> => {
    // 같은 파일을 다시 골라도 `change` 가 나도록 값을 비운다 — 비우지 않으면 취소한 뒤 같은
    // 파일을 다시 고르는 동작이 조용히 아무 일도 하지 않는다.
    input.value = '';
    if (file === undefined) return;
    // **바이트 상한을 읽기보다 먼저 본다.** `planSvgImport` 도 같은 상한을 보지만 그것은 이미
    // 문자열이 된 뒤다 — 2GB 짜리 파일을 메모리에 올린 다음 거절하는 것은 거절이 아니다.
    if (file.size > MAX_IMPORT_FILE_BYTES) {
      setState({
        phase: 'refused',
        refusal: { reason: 'fileTooLarge', actual: file.size, limit: MAX_IMPORT_FILE_BYTES },
      });
      return;
    }
    // **상한 조회를 읽기와 나란히 띄운다.** 서버 왕복을 파일 읽기 뒤에 줄 세우면 고른
    // 순간부터 미리보기까지가 그만큼 길어지는데, 두 일은 서로를 기다릴 이유가 없다.
    //
    // **`catch` 를 여기에 둔다 — 서비스가 이미 던지지 않기로 약속했는데도.** 약속은
    // `dashboardLimitsService` 의 것이고 이 인자는 **주입 가능**하다. 이 자리에서 예외가
    // 새면 결과가 "상한이 낡는다" 가 아니라 **"가져오기를 아예 못 한다"** 이므로, 한 줄로
    // 그 낙차를 막는다. 띄우는 시점에 붙이는 것이 요점이다 — `await` 자리에서 잡으면 읽기가
    // 먼저 실패한 경로에서 처리기 없는 거부가 남는다.
    const limitsPromise = fetchLimits().catch(
      (): DashboardLimits => ({ maxCanvasElements: undefined, payloadBudgetBytes: undefined }),
    );

    let text: string;
    try {
      text = await readFile(file);
    } catch {
      // 읽지 못한 것은 문서가 나쁜 것과 다르다 — 다른 문구로 말한다.
      setState({ phase: 'refused', refusal: { reason: 'unreadable', actual: 0, limit: 0 } });
      return;
    }

    // **서버가 말하지 못한 축은 컴파일 기본값으로 선다.** 오프라인·구형 서버·권한 없음
    // 어느 쪽이든 가져오기 자체는 계속되어야 한다 — 상한이 낡는 것보다 못 쓰는 것이 나쁘다.
    const { maxCanvasElements } = await limitsPromise;
    const plan = planSvgImport(text, canvas, { limits: resolveImportLimits(maxCanvasElements) });
    // 여기서 `setNotesOpen(false)` 를 부르지 **않는다** — 이 자리에 닿는 길은 대기와 거절
    // 둘뿐이고 그 둘로 들어오는 길은 전부 `reset()` 을 지나므로(또는 첫 렌더이므로) 목록은
    // 이미 접혀 있다. 부르면 어느 시험으로도 관측되지 않는 줄이 하나 늘고, 관측되지 않는
    // 줄은 다음 사람에게 "여기서 무언가 일어난다" 고 거짓말한다(뮤테이션 M9-3 에서 드러났다).
    if (!plan.ok) {
      setState({ phase: 'refused', refusal: plan.refusal });
      return;
    }
    setState({
      phase: 'ready',
      ready: { shapes: plan.shapes, texts: plan.texts, report: plan.report },
    });
  };

  /** [취소] · 놓은 뒤 — **무동작이다**(불변식 K15). 배열도 config 도 건드리지 않는다. */
  const reset = (): void => {
    setState({ phase: 'idle' });
    setNotesOpen(false);
  };

  const Chevron = collapsed ? ChevronRight : ChevronDown;
  const NotesChevron = notesOpen ? ChevronDown : ChevronRight;

  const fileInput = (again: boolean): React.ReactElement => (
    <label className={PICK_LABEL_CLASS} data-testid="canvas-svg-import-pick">
      <FileUp className={ICON_CLASS} aria-hidden="true" />
      <span>{t(again ? `${EDIT}.importPickAgain` : `${EDIT}.importPick`)}</span>
      {/* 화면에서만 숨는다 — `hidden` 이나 `display:none` 은 키보드 초점을 함께 빼앗는다. */}
      <input
        type="file"
        accept=".svg,image/svg+xml"
        data-testid="canvas-svg-import-file"
        aria-label={t(again ? `${EDIT}.importPickAgain` : `${EDIT}.importPick`)}
        className="sr-only"
        onChange={(event) => {
          void pick(event.target.files?.[0], event.target);
        }}
      />
    </label>
  );

  return (
    <section role="group" aria-labelledby={titleId} className="flex flex-col gap-0.5">
      <p id={titleId} className={SECTION_TITLE_CLASS}>
        {t(`${EDIT}.dockImport`)}
      </p>
      <button
        type="button"
        data-testid="canvas-svg-import-group"
        aria-expanded={!collapsed}
        aria-controls={collapsed ? undefined : bodyId}
        className={GROUP_HEAD_CLASS}
        onClick={() => setCollapsed((prev) => !prev)}
      >
        <Chevron className={CHEVRON_CLASS} aria-hidden="true" />
        <span>{t(`${EDIT}.importGroup`)}</span>
      </button>
      {!collapsed && (
        <div id={bodyId} data-testid="canvas-svg-import-group-body" className="flex flex-col gap-1">
          {state.phase === 'idle' && (
            <>
              {fileInput(false)}
              <p className={HINT_CLASS}>{t(`${EDIT}.importHint`)}</p>
            </>
          )}

          {state.phase === 'refused' && (
            <>
              {/* 사유는 **읽히는 자리**에 선다 — `role="status"` 라 초점을 빼앗지 않고 보조
                  기술에는 알려진다. 실제 수와 상한을 **함께** 말해야 얼마나 줄일지 안다. */}
              <p
                data-testid="canvas-svg-import-refused"
                role="status"
                className="px-1 text-[11px] leading-tight text-red-500"
              >
                {refusalText(t(REFUSAL_KEYS[state.refusal.reason]), state.refusal)}
              </p>
              {fileInput(true)}
            </>
          )}

          {state.phase === 'ready' && (
            <>
              <ImportPreview elements={preview} canvas={canvas} />
              {/* **요약 한 줄은 접히지 않는다**(REQ-04). 갈래 셋 가운데 "들어옴" 이 이 줄이다. */}
              <p data-testid="canvas-svg-import-summary" className={SUMMARY_CLASS}>
                {t(`${EDIT}.importSummary`)
                  .replaceAll('{shapes}', String(state.ready.report.shapes))
                  .replaceAll('{texts}', String(state.ready.report.texts))
                  .replaceAll('{commands}', String(state.ready.report.commands))
                  .replaceAll('{kb}', String(kb(state.ready.report.estimatedBytes)))}
              </p>
              <ImportNotes
                notes={state.ready.report.notes}
                open={notesOpen}
                onToggle={() => setNotesOpen((prev) => !prev)}
                bodyId={notesId}
                Chevron={NotesChevron}
              />
              {placeableCount(state.ready) === 0 && (
                <p data-testid="canvas-svg-import-empty" role="status" className={HINT_CLASS}>
                  {t(`${EDIT}.importNothing`)}
                </p>
              )}
              <button
                type="button"
                data-testid="canvas-svg-import-place"
                aria-label={t(`${EDIT}.importPlace`)}
                title={t(`${EDIT}.importPlace`)}
                // **가져올 것이 하나도 없으면 서지 않는다**(REQ-03) — 빈 요소를 만들지 않는다.
                disabled={placeableCount(state.ready) === 0}
                className={ROW_BUTTON_CLASS}
                // 처리자를 **여기서** 만든다. 밖으로 빼면 `state` 가 좁혀지지 않아
                // `state.phase !== 'ready'` 재확인이 필요하고, 그 가지는 **닿을 수 없다** —
                // 이 단추는 준비됨 상태에서만 그려지기 때문이다. 닿을 수 없는 가지는
                // 커버리지에 구멍으로 남고, 읽는 사람에게 "여기로 올 수도 있다" 고 거짓말한다.
                onClick={() => {
                  onPlace(state.ready.shapes, state.ready.texts);
                  reset();
                }}
              >
                <Check className={ICON_CLASS} aria-hidden="true" />
                <span>{t(`${EDIT}.importPlace`)}</span>
              </button>
              <button
                type="button"
                data-testid="canvas-svg-import-cancel"
                aria-label={t(`${EDIT}.importCancel`)}
                title={t(`${EDIT}.importCancel`)}
                className={ROW_BUTTON_CLASS}
                onClick={reset}
              >
                <X className={ICON_CLASS} aria-hidden="true" />
                <span>{t(`${EDIT}.importCancel`)}</span>
              </button>
            </>
          )}
        </div>
      )}
    </section>
  );
}

// --- 보고 목록 ------------------------------------------------------------

interface ImportNotesProps {
  notes: readonly ImportNote[];
  open: boolean;
  onToggle: () => void;
  bodyId: string;
  Chevron: typeof ChevronDown;
}

/**
 * 근사됨 · 버림 두 갈래. **목록만 접힌다** — 개수를 든 토글 줄은 언제나 보인다.
 *
 * 항목이 하나도 없으면 토글 자체를 내지 않는다. "근사됨 0개 · 버림 0개" 는 아무것도 말하지
 * 않으면서 한 줄을 먹고, 그런 줄이 쌓이면 보고가 잡음이 된다(위험 R7).
 */
function ImportNotes({
  notes,
  open,
  onToggle,
  bodyId,
  Chevron,
}: ImportNotesProps): React.ReactElement | null {
  const { t } = useTranslation();
  if (notes.length === 0) return null;

  const approximated = notes.filter((n) => n.kind === 'approximated');
  const dropped = notes.filter((n) => n.kind === 'dropped');
  const item = t(`${EDIT}.importNoteItem`);
  const line = (note: ImportNote): string =>
    item.replaceAll('{label}', t(NOTE_KEYS[note.reason])).replaceAll('{count}', String(note.count));

  return (
    <div className="flex flex-col gap-0.5">
      <button
        type="button"
        data-testid="canvas-svg-import-notes-toggle"
        aria-expanded={open}
        aria-controls={open ? bodyId : undefined}
        className={GROUP_HEAD_CLASS}
        onClick={onToggle}
      >
        <Chevron className={CHEVRON_CLASS} aria-hidden="true" />
        <span>
          {t(`${EDIT}.importNotesToggle`)
            .replaceAll('{approx}', String(approximated.length))
            .replaceAll('{dropped}', String(dropped.length))}
        </span>
      </button>
      {open && (
        <div id={bodyId} data-testid="canvas-svg-import-notes" className={NOTE_LIST_CLASS}>
          {approximated.length > 0 && (
            <>
              <p className={NOTE_HEAD_CLASS}>{t(`${EDIT}.importNotesApproximated`)}</p>
              <ul className="flex flex-col gap-0.5">
                {approximated.map((note) => (
                  <li key={note.reason} data-testid={`canvas-svg-import-note-${note.reason}`} className={NOTE_ITEM_CLASS}>
                    {line(note)}
                  </li>
                ))}
              </ul>
            </>
          )}
          {dropped.length > 0 && (
            <>
              <p className={NOTE_HEAD_CLASS}>{t(`${EDIT}.importNotesDropped`)}</p>
              <ul className="flex flex-col gap-0.5">
                {dropped.map((note) => (
                  <li key={note.reason} data-testid={`canvas-svg-import-note-${note.reason}`} className={NOTE_ITEM_CLASS}>
                    {line(note)}
                  </li>
                ))}
              </ul>
            </>
          )}
          {/* 명령 상한으로 버려진 도형이 있으면 **다른 길을 이름으로 가리킨다**(§결정 4).
              거절이 사용자를 막다른 길에 두지 않는 것이 이 SPEC 이 (B) 대신 (C) 를 고른 값이다. */}
          {dropped.some((n) => n.reason === 'commandLimitDropped') && (
            <p data-testid="canvas-svg-import-command-limit-hint" className={NOTE_ITEM_CLASS}>
              {t(`${EDIT}.importCommandLimitHint`)}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
