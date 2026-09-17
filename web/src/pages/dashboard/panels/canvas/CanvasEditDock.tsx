// 캔버스 편집 도구 도크 — 스테이지 위에 떠 있던 띠를 **패널 밖 왼쪽 영역**으로 옮긴 자리
// (SPEC-CANVAS-002 T9 후속 · 사용 시험: "도형 팔레트 크기가 너무 작음").
//
// 무엇이 달라졌는가: 002 는 도구를 스테이지 왼쪽 위에 뜨는 아이콘 띠로 두었다. 그 자리가
// 준 것은 "캔버스 가까이" 하나였고 대신 셋을 잃었다 — 그림을 가리고, 스테이지 폭에 갇혀
// 라벨을 달 수 없고(아이콘뿐이라 이름을 아는 길이 `title` 툴팁 하나였다), 칸을 키우면
// 그릴 자리를 그만큼 먹는다. 이 도구는 **패널 설정에서만 쓰므로**(대시보드에 놓인 패널은
// 표시와 배치 조정만 한다) 미리보기 옆에 제 영역을 갖는 편이 셋을 모두 되돌려준다.
//
// **패널 안이 아니라 패널 옆이다.** 이것이 이 파일에서 가장 중요한 제약이다. 설정
// 미리보기는 패널을 **대시보드에서의 실제 픽셀 크기로 렌더한 뒤 통째로 축소**한다
// (`PanelSettingsDialog` §미리보기 — `computePreviewStage` + `previewZoom`). 도크를 패널
// **안**에 두면 패널 자신의 레이아웃이 달라져 미리보기가 대시보드와 다른 화면이 되고,
// 캔버스 스테이지도 도크 폭만큼 좁아진다 — 그 미리보기는 더 이상 미리보기가 아니다.
// 그래서 도크는 축소되는 상자(`preview-stage`) **바깥**, fit 컨테이너와 나란한 형제로
// 선다(`CanvasEditDockRegion`). fit 컨테이너의 실측 폭이 도크를 뺀 값이 되므로 미리보기
// 배율 계산도 저절로 맞는다 — 빼기를 어디에도 적지 않는다.
//
// **그런데 도구를 만드는 쪽은 여전히 오버레이다.** 도형을 놓는 일도 서랍에 담는 일도
// 선택과 요소 배열을 모두 봐야 하고 그 값들은 오버레이 안에 산다. 상태를 다이얼로그로
// 올리면 스테이지가 잴 때마다·요소가 끌릴 때마다 설정 화면 전체가 다시 그려진다. 그래서
// 방향을 뒤집어 **자리만 아래로 내리고**(`canvasEditDockHost` 컨텍스트) 오버레이가 그
// 자리에 `createPortal` 로 그린다. React 트리 상으로는 여전히 오버레이의 자식이므로
// 선택 컨텍스트도, 이벤트 전파도 종전과 같다 — 실제로 팔레트 위 누름을 끊는 한 줄
// (`onPointerDown` 에서 `stopPropagation`)이 그대로 필요한 이유가 그것이다.
//
// **도크는 이제 도구 한 벌의 절반이다**(SPEC-CANVAS-011 후속 · 사용자 결정 2026-09-16).
// 열 절 가운데 일곱이 미리보기 제목 아래 가로 띠로 갔고(`CanvasEditToolbar`), 여기 남은
// 것은 **캔버스에 재료를 놓는** 셋 — 도형 · 가져오기 · 서랍 — 뿐이다. 가르는 자는 절이
// 무엇에 대해 말하는가 하나이며, 남은 셋은 덤으로 하나를 더 공유한다: 셋 다 **목록**을
// 들어 세로로 자란다. 그것이 세로 상자인 이 도크가 남은 이유다. 자리를 내는 컴포넌트
// (`CanvasEditDockRegion`)는 여전히 이 파일에 있고 **두 자리를 함께** 낸다.
//
// **크기와 이름.** 아이콘 한 칸 `p-0.5` 는 누르기도 어렵고 무엇인지도 알 수 없었다. 도형
// 버튼은 폭을 가득 쓰는 줄이 되어 아이콘 옆에 **이름**을 단다. 글자 크기는 주변 설정
// 화면의 눈금을 그대로 쓴다 — 본문 `text-xs`(12px), 곁들이 `text-[11px]`. 9·10px 은
// 이 저장소가 캔버스 편집기에서 이미 걷어낸 크기이고(`CanvasElementsEditor.test.tsx`
// §글자 크기) 그 가드가 이 파일도 함께 훑는다.
//
// **보이는 이름과 접근성 이름은 같은 것을 가리킨다.** 버튼의 `aria-label` 은 종전 그대로
// "사각형 놓기" 이고 눈에 보이는 글자는 "사각형" 이다 — 접근성 이름이 보이는 라벨을
// 포함하므로 음성 조작이 보이는 대로 통한다(WCAG 2.5.3). 라벨을 붙였다고 `aria-label` 을
// 걷지 않는 이유가 그것이다: 걷으면 이름이 "사각형" 으로 줄어 무엇을 하는 버튼인지가
// 사라진다.
//
// @spec SPEC-CANVAS-002 REQ-01 REQ-04

import { useId, useState, type ReactNode } from 'react';

import { useTranslation } from '@/lib/i18n';

import { type CanvasPrimitiveKind, type CanvasSize } from './canvasConfig';
import { CanvasEditDockHostContext, CanvasEditToolbarHostContext } from './canvasEditDockHost';
import { CanvasScratchpad } from './scratchpad/CanvasScratchpad';
import { CanvasSvgImport } from './svgimport/CanvasSvgImport';
import type { ImportedShapeSpec, ImportedTextSpec } from './svgimport/svgImportPlan';
import type { ScratchpadDropPoint } from './scratchpad/canvasScratchpadDrop';
import type { ScratchpadEntry } from './scratchpad/scratchpadTypes';
import { CanvasCellPreview, CanvasShapeCatalog, CELL_CLASS } from './shapes/CanvasShapeCatalog';
import { primitivePreviewElement } from './shapes/previewElements';
import { usePaletteCollapse } from './shapes/paletteGroups';
import type { ShapeCatalogEntry } from './shapes/shapeCatalog';

// --- 겉모습 --------------------------------------------------------------

/**
 * 도크 영역. 폭은 고정이다 — 미리보기와 폭을 나눠 가지면 패널이 커질 때마다 도구가
 * 흔들리고, 도구는 넓어질 이유가 없다(가장 긴 칸이 도형 이름 한 줄이다).
 *
 * `shrink-0` 이 없으면 미리보기가 넓어질 때 도크가 눌려 라벨이 잘린다 — 라벨을 달려고
 * 옮긴 자리에서 라벨이 잘리면 옮긴 뜻이 없다.
 */
const DOCK_CLASS =
  'flex w-44 shrink-0 flex-col overflow-y-auto rounded-md border border-(--color-border-default) ' +
  'bg-(--color-bg-surface) p-2';

/** 묶음 제목. 본문(12px)보다 한 눈금 작은 곁들이 글자다 — 주변 설정 절과 같은 눈금이다. */
const SECTION_TITLE_CLASS = 'px-1 pb-1 text-[11px] font-medium text-(--color-text-muted)';

// --- 도형 표 ------------------------------------------------------------

/**
 * 도크가 내는 도형 4종. **목록 편집기의 나열 순서와 같다** — 같은 것을 두 자리에서 다른
 * 순서로 내면 사용자가 두 목록을 따로 외워야 한다.
 *
 * 원소 타입이 `CanvasPrimitiveKind` 인 것이 **넷이라는 사실을 지키는 가드다.** 배열은
 * 그 자체로 총망라를 요구하지 않으므로 한 줄을 더해도 컴파일러가 울지 않지만, 원소
 * 타입이 원시형 넷이면 다섯 번째로 경로를 더하는 순간 운다. 아래 세 표도 같은 키
 * 집합을 쓰므로, 다섯 번째 단추를 세우려면 그 세 표를 함께 채워야 한다.
 */
const PALETTE_KINDS: readonly CanvasPrimitiveKind[] = ['rect', 'ellipse', 'line', 'text'];

/**
 * 버튼의 `aria-label` i18n 키("사각형 놓기"). **키 이름 안에 점을 넣지 않는다**(프로젝트 규약).
 *
 * 보이는 이름(`PALETTE_NAME_KEYS`)이 생긴 뒤에도 남는다 — 이쪽이 **무엇을 하는가**를
 * 말하고 보이는 쪽은 **무엇인가**를 말한다.
 */
const PALETTE_ARIA_KEYS: Record<CanvasPrimitiveKind, string> = {
  rect: 'dashboard.canvas.edit.paletteRect',
  ellipse: 'dashboard.canvas.edit.paletteEllipse',
  line: 'dashboard.canvas.edit.paletteLine',
  text: 'dashboard.canvas.edit.paletteText',
};

/** 아이콘 옆에 보이는 도형 이름. `aria-label` 이 이 문자열을 포함한다(WCAG 2.5.3). */
const PALETTE_NAME_KEYS: Record<CanvasPrimitiveKind, string> = {
  rect: 'dashboard.canvas.edit.shapeRect',
  ellipse: 'dashboard.canvas.edit.shapeEllipse',
  line: 'dashboard.canvas.edit.shapeLine',
  text: 'dashboard.canvas.edit.shapeText',
};

// --- 자리 --------------------------------------------------------------

export interface CanvasEditDockRegionProps {
  /**
   * 도크를 낼 것인가. 캔버스 패널의 설정 미리보기에서만 참이다 — 다른 종류의 패널은
   * 이 도구를 쓰지 않고, 대시보드에 놓인 패널에는 이 컴포넌트 자체가 없다.
   */
  enabled: boolean;
  /** 미리보기 fit 컨테이너. 도크는 이것을 **감싸지 않고 옆에 선다**. */
  children: ReactNode;
}

/**
 * 미리보기 영역을 **띠 / (도크 | 미리보기)** 로 가른다.
 *
 * 꺼져 있으면 받은 자식을 **그대로** 돌려준다 — 감싸는 요소를 하나도 더하지 않으므로,
 * 캔버스가 아닌 패널의 미리보기 DOM 은 이 변경 전과 완전히 같다. 조건부로 한 겹을 더하는
 * 대신 언제나 감싸 두면 그 한 겹이 모든 패널 종류의 레이아웃 계산에 끼어든다.
 *
 * `flex-1 min-h-0` 은 fit 컨테이너에서 **옮겨 온 것이 아니라 이어받은 것**이다. 세로
 * 컬럼 안에서 남은 높이를 받던 자리를 이 컬럼이 대신 받고, 그 안의 행에서 fit 컨테이너가
 * 다시 `flex-1` 로 남은 **폭**을 받는다. 그 폭이 곧 `computePreviewStage` 가 실측하는
 * 값이므로 도크 폭도 띠 높이도 미리보기 배율에 저절로 반영된다 — **빼기를 어디에도
 * 적지 않는다**(그것이 002 가 도크에 세운 그 성질이고, 띠가 그 성질을 물려받는다).
 *
 * **띠는 다이얼로그가 아니라 여기서 자리를 받는다.** 이 컴포넌트는 미리보기 제목 줄
 * **바로 아래**에 통째로 꽂히므로(`PanelSettingsDialog` §미리보기 — 제목 행과 `{preview}`
 * 가 한 컬럼의 이웃이다), 이 컬럼의 첫 자식이 곧 "제목 바로 아래, 미리보기 폭 전체" 다.
 * 다이얼로그에 두 번째 슬롯을 뚫지 않는 것에 값이 있다 — 그 파일은 패널 종류 전부가
 * 함께 쓰는 8천 줄이고, 캔버스 수입은 허용목록 하나로 묶여 있다
 * (`test/panelSettingsDialogCanvasSurface.ts`). 자리를 여기서 내면 그 목록이 한 줄도
 * 늘지 않는다.
 */
export function CanvasEditDockRegion({
  enabled,
  children,
}: CanvasEditDockRegionProps): React.ReactElement {
  // 훅은 분기보다 앞이다 — `enabled` 가 바뀌어도 훅 수가 달라지지 않는다.
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  // 띠의 자리. 도크의 그것과 **같은 커밋에서** 붙으므로 둘은 언제나 함께 있다
  // (`canvasEditDockHost` §자리가 둘인 이유).
  const [toolbarHost, setToolbarHost] = useState<HTMLDivElement | null>(null);

  if (!enabled) return <>{children}</>;

  return (
    <CanvasEditDockHostContext value={host}>
      <CanvasEditToolbarHostContext value={toolbarHost}>
        <div className="flex min-h-0 flex-1 flex-col gap-2">
          {/*
            띠의 포털 목적지. **겉모습을 하나도 갖지 않는다** — 테두리도 배경도 그리는
            쪽(`CanvasEditToolbarBody`)이 지므로, 오버레이가 아직 그리지 않은 한 프레임에
            빈 상자가 번쩍이지 않는다. 도크 쪽이 `DOCK_CLASS` 로 제 상자를 미리 그려 두는
            것과 갈라지는 자리이고, 그 차이의 근거는 **폭**이다: 도크는 폭을 차지해야 그
            옆의 미리보기가 제 폭을 알지만, 띠는 비어 있으면 높이 0 이어야 옳다.
          */}
          <div ref={setToolbarHost} data-testid="canvas-toolbar" className="shrink-0 empty:hidden" />
          <div className="flex min-h-0 flex-1 gap-2">
            {/*
              포털 목적지. 콜백 ref 로 state 에 담아야 붙는 순간이 렌더로 전해진다
              (`canvasEditDockHost` 주석). 오버레이가 그리기 전까지는 빈 상자다 —
              캔버스 패널이 아직 편집을 켜지 않았거나 아직 마운트되지 않은 그 한 프레임이다.
            */}
            <div ref={setHost} data-testid="canvas-dock" className={DOCK_CLASS} />
            {children}
          </div>
        </div>
      </CanvasEditToolbarHostContext>
    </CanvasEditDockHostContext>
  );
}

// --- 도구 --------------------------------------------------------------

export interface CanvasEditDockBodyProps {
  /**
   * 도형을 놓는다 — **요소를 만드는 유일한 입구**다. 목록 편집기 하단에도 종류별 추가
   * 버튼이 있었으나, 같은 함수(`canvasElementFactory.appendElement`)를 부르는 입구가 둘일
   * 이유가 없어 그쪽을 걷었다. 생성 경로 자체는 그대로다(가정 A7).
   */
  onPlace: (kind: CanvasPrimitiveKind) => void;
  /**
   * 카탈로그 도형을 놓는다 — **같은 생성 모듈의 다른 입구**다(`appendPathElement`).
   *
   * `onPlace` 와 합치지 않는 이유는 인자에 있다: 원시형은 씨앗 기하만으로 만들어지고 경로는
   * 명령 목록을 요구하므로, 하나의 콜백으로 합치려면 "원시형일 때는 무시되는 인자" 가 생긴다.
   */
  onPlaceShape: (entry: ShapeCatalogEntry) => void;
  /**
   * 캔버스 좌표계 크기 — **가져오기에만** 쓴다(들여온 그림이 앉을 상자를 짓는다).
   *
   * 격자 자투리 고지도 이 값을 보았으나 그 절이 띠로 갔다(`CanvasEditToolbar`). 그리는
   * 일에는 여전히 쓰지 않는다 — 그것은 표면의 몫이다.
   */
  canvas: CanvasSize;
  /**
   * 고른 것을 스크래치패드에 넣는다 — **끌어 넣기와 같은 함수**다(불변식 J11).
   *
   * 이 컴포넌트가 저장 자체를 하지 않는 것에 뜻이 있다: 저장할 요소는 오버레이가 든
   * 선택과 배열에서 나오고, 끌어 넣기의 되돌림도 오버레이의 드래그 상태를 요구한다.
   * 두 경로가 만나는 자리를 여기에 두면 도크가 상태를 갖게 된다(이 파일의 규율에 반한다).
   */
  onScratchpadSave: () => void;
  /** 고른 것이 있는가 — 없으면 저장 단추를 끈다(REQ-03). */
  canScratchpadSave: boolean;
  /** 끌던 손이 지금 드롭 존 위에 있는가. 판정은 오버레이가 한다(불변식 J7). */
  scratchpadDropActive: boolean;
  /**
   * 서랍의 항목을 캔버스에 놓는다. `at` 이 `null` 이면 계단 자리(`seedOffset`)다.
   *
   * 클라이언트 좌표를 그대로 지나 보내는 것에 뜻이 있다 — 스테이지로 옮기는 함수는
   * 오버레이에 하나뿐이며(불변식 J7), 도크가 그 환산을 알면 두 벌이 된다.
   */
  onScratchpadPlace: (entry: ScratchpadEntry, at: ScratchpadDropPoint | null) => boolean;
  /**
   * 가져온 도형들을 놓는다 — **같은 생성 모듈의 또 다른 입구**다
   * (`appendImportedElements`). 만드는 일도 놓은 뒤의 선택도 이 파일이 하지 않는다.
   *
   * 상자를 함께 받는 것에 뜻이 있다: 가져오기의 산출은 **한 그림의 조각들**이라 상자를
   * 공유하고, 계단 오프셋도 무리 전체에 한 번만 더해진다(REQ-06). 조각마다 상자를 짓게
   * 두면 정수 반올림이 조각마다 최대 0.5 단위씩 어긋나 그림이 갈라진다.
   *
   * 문구를 **둘째 인자로** 받는다 — 한 배열로 합치면 두 갈래를 판별할 표식이 값에
   * 들어가야 하고, 그 표식은 요소의 어휘(`kind`)를 흉내 내게 된다(결함 B 정정).
   */
  onSvgImport: (
    shapes: readonly ImportedShapeSpec[],
    texts: readonly ImportedTextSpec[],
  ) => void;
}

/**
 * 도크에 그려지는 도구 한 벌. **상태를 하나도 갖지 않는다** — 선택도 요소도 전부
 * 오버레이가 들고 있고 이 컴포넌트는 그것을 그리고 되돌려 줄 뿐이다. 그래서 이 파일은
 * jsdom 없이도 읽히는 순수한 표현층이고, "무엇이 일어나는가" 는 여전히 한 곳에 있다.
 *
 * **남은 절은 셋이다** — 도형 · 가져오기 · 서랍. 셋의 공통점이 이 도크의 뜻이다:
 * 전부 **캔버스에 무언가를 놓는 입구**이고, 전부 **목록**을 들어 세로로 자란다. 캔버스와
 * 고른 것에 작용하던 절 일곱은 미리보기 제목 아래 가로 띠로 갔다(`CanvasEditToolbar`).
 *
 * 묶음마다 제목을 단다. 아이콘 띠였을 때는 구분선 하나가 그 일을 했지만, 구분선은
 * 스크린 리더에게 아무것도 아니다 — 제목은 보는 사람과 듣는 사람 모두에게 같은 경계를
 * 준다(`aria-labelledby` 로 묶음에 이름을 준다).
 */
export function CanvasEditDockBody({
  onPlace,
  onPlaceShape,
  canvas,
  onScratchpadSave,
  canScratchpadSave,
  scratchpadDropActive,
  onScratchpadPlace,
  onSvgImport,
}: CanvasEditDockBodyProps): React.ReactElement {
  const { t } = useTranslation();
  // 묶음 넷의 접힘 상태. 기기 지역에 남되 읽지 못해도 기능이 성립한다(REQ-06).
  const { collapsed, toggle } = usePaletteCollapse();
  // 한 화면에 캔버스 설정이 둘 이상 뜰 수 있으므로 고정 id 를 쓸 수 없다.
  const shapesId = useId();
  const scratchpadId = useId();

  return (
    <div
      data-testid="canvas-dock-panel"
      role="group"
      aria-label={t('dashboard.canvas.edit.paletteAria')}
      className="flex w-full flex-col gap-3"
      // 도크는 DOM 상으로 패널 밖에 있지만 **React 트리 상으로는 오버레이의 자식**이다
      // (포털). 그래서 누름이 오버레이 루트의 히트 테스트까지 흘러가며, 끊지 않으면
      // 버튼을 눌렀는데 그 뒤에 있는 도형이 함께 골라지고 이동 드래그까지 시작된다.
      onPointerDown={(event) => event.stopPropagation()}
      // **키도 같은 길로 오른다.** 떠 있는 배율 줄은 이 한 줄을 처음부터 갖고 있었고
      // (`CanvasEditOverlay` §줄 위의 누름과 키), 도크에는 없었다 — 그래서 수치 칸에서
      // 방향키를 누르면 수는 그대로이고 **요소가 움직였다**. SPEC-CANVAS-010 이
      // Delete·Backspace 를 오버레이에 달면서 그 구멍의 대가가 "요소가 한 칸 움직인다"
      // 에서 **"그림이 지워진다"** 로 커졌다(**서랍 항목의 이름 칸**이 도크에 남은 그
      // 자리다). 되돌리기가 없으므로 여기서 막는다. 띠도 같은 한 줄을 제 자리에서 든다
      // (`CanvasEditToolbar` — 격자 간격 칸이 그리로 갔다).
      //
      // 표적을 보지 않고 **도크 전체**를 끊는다. 어느 칸이 글자를 받는지 세는 목록은
      // 칸이 늘 때마다 조용히 낡지만, "도크에서 누른 키는 도크의 것" 이라는 문장은
      // 낡지 않는다. `preventDefault` 는 쓰지 않는다 — 칸의 초점과 캐럿이 살아 있어야
      // 하고, `defaultPrevented` 는 `previewPan` 이 읽는 표시라 뜻이 번진다.
      onKeyDown={(event) => event.stopPropagation()}
    >
      {/* 도형 — 목록 하나에서 **접히는 묶음 셋**으로 바뀐 절이다(REQ-02 · REQ-06 · 011 REQ-01).

          바뀌지 않은 것을 먼저 적는다: 원시형 넷의 `data-testid` · 이름 · 차례 · 클릭이
          부르는 것은 그대로이고, 넷이 서 있는 묶음은 **기본으로 펼쳐진다.** 카탈로그 30칸을
          한 목록으로 펴면 도크 폭이 `w-44` 고정이라 자주 쓰는 넷이 스크롤 아래로 밀리므로
          (위험 R10), 나머지 묶음 둘은 접힌 채로 태어난다.

          바뀐 것은 셋이다. ① 011 이 원시형 묶음을 걷어내고 그 넷을 `기본` 묶음으로 옮겼다.
          ② 넷이 **줄 버튼이기를 그만두고 격자 칸이 되었다.** 자리만 옮기고 생김새를 두었더니
          한 묶음 몸통 안에 세로 줄 넷과 2열 격자 서른이 함께 서서, 옮긴 자리가 "같은 묶음"
          으로 읽히지 않았다. ③ 그 칸의 그림이 **lucide 글리프에서 실제 렌더 경로로** 바뀌었다.
          칸 크기를 맞춰도 윤곽선 글리프와 파란 도형은 여전히 두 벌의 잉크였고, 이제 넷도
          카탈로그 30종과 같은 `drawElements` 를 지난다 — 칸에 보이는 모습이 곧 눌렀을 때
          놓이는 모습이다(REQ-06 이 카탈로그에 세운 그 규율이 넷에도 걸린다).

          칸의 겉모습(`CELL_CLASS`)도 그림(`CanvasCellPreview`)도 카탈로그가 **내보낸 그것**을
          쓴다 — 여기서 비슷한 클래스나 두 번째 그리기를 적으면 두 생김새가 다시 갈라진다.

          그래도 넷을 세우는 쪽은 도크다 — 이름·접근성 이름·클릭이 부르는 `onPlace` 가
          카탈로그의 관심이 아니기 때문이다. */}
      <section role="group" aria-labelledby={shapesId} className="flex flex-col gap-0.5">
        <p id={shapesId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockShapes')}
        </p>
        <CanvasShapeCatalog
          collapsed={collapsed}
          onToggle={toggle}
          onPlace={onPlaceShape}
          leading={PALETTE_KINDS.map((kind) => (
            <button
              key={kind}
              type="button"
              data-testid={`canvas-palette-add-${kind}`}
              aria-label={t(PALETTE_ARIA_KEYS[kind])}
              title={t(PALETTE_ARIA_KEYS[kind])}
              className={CELL_CLASS}
              onClick={() => onPlace(kind)}
            >
              <CanvasCellPreview
                testId={`canvas-palette-preview-${kind}`}
                element={primitivePreviewElement(kind)}
              />
              <span className="w-full truncate text-center">{t(PALETTE_NAME_KEYS[kind])}</span>
            </button>
          ))}
        />
      </section>

      {/* 가져오기 — 도형 절과 스크래치패드 사이다(SPEC-CANVAS-007 REQ-04).

          007 이 이 자리를 고른 근거는 "위 다섯 절의 자리가 한 줄도 달라지지 않는다" 였다.
          그 다섯 중 넷이 띠로 떠난 지금, 남은 셋은 **뜻으로 이어진 차례**가 되었다 —
          우리가 준 재료(도형) · 당신이 가져온 재료(가져오기) · 당신이 만든 재료(서랍).
          자리는 한 줄도 옮기지 않았는데 이유가 더 곧아졌다.

          **기본으로 접혀 있다** — 도크 폭이 `w-44` 고정이고 가져오기는 한 번 쓰고 닫는
          일이다. 그래서 펴지 않은 화면은 007 이전과 **묶음 머리 한 줄만** 다르다(AC-E1).

          **캔버스 표면에 아무것도 그리지 않는다**(불변식 K10). 파일을 캔버스 위로 끌어놓는
          길을 두지 않는 것이 그 성질의 값이며, 006 이 배달한 I23 결함의 재발을 형상으로 막는다. */}
      <CanvasSvgImport canvas={canvas} onPlace={onSvgImport} />

      {/* 스크래치패드 — **도크의 맨 끝**이다(SPEC-CANVAS-008 REQ-03).

          이 절은 드롭 존과 목록을 함께 들어 도크에서 가장 키가 크고, 도크는 폭 `w-44` 의
          세로 스크롤 상자다 — 그래서 맨 끝이다. 008 이 이 자리를 고를 때의 근거는 "가운데에
          두면 격자 · 정렬 · 순서가 한꺼번에 스크롤 아래로 밀린다" 였고, 그 셋이 띠로 떠난
          지금은 근거가 하나 줄어 **키 하나**만 남았다. 자리는 그대로다. */}
      <CanvasScratchpad
        titleId={scratchpadId}
        canSave={canScratchpadSave}
        onSave={onScratchpadSave}
        dropActive={scratchpadDropActive}
        onPlace={onScratchpadPlace}
      />
    </div>
  );
}
