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
// **그런데 도구를 만드는 쪽은 여전히 오버레이다.** 격자 토글·간격·정렬·순서는 스테이지
// 크기, 선택, 요소 배열을 모두 봐야 하고 그 값들은 오버레이 안에 산다. 상태를 다이얼로그로
// 올리면 스테이지가 잴 때마다·요소가 끌릴 때마다 설정 화면 전체가 다시 그려진다. 그래서
// 방향을 뒤집어 **자리만 아래로 내리고**(`canvasEditDockHost` 컨텍스트) 오버레이가 그
// 자리에 `createPortal` 로 그린다. React 트리 상으로는 여전히 오버레이의 자식이므로
// 선택 컨텍스트도, 이벤트 전파도 종전과 같다 — 실제로 팔레트 위 누름을 끊는 한 줄
// (`onPointerDown` 에서 `stopPropagation`)이 그대로 필요한 이유가 그것이다.
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

import {
  AlignHorizontalJustifyCenter,
  AlignHorizontalJustifyEnd,
  AlignHorizontalJustifyStart,
  AlignVerticalJustifyCenter,
  AlignVerticalJustifyEnd,
  AlignVerticalJustifyStart,
  BringToFront,
  Grid3x3,
  SendToBack,
  Square,
} from 'lucide-react';

import { FieldHelp } from '@/components/property/FieldHelp';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { CanvasWorkspaceZoomField } from './CanvasWorkspaceZoomField';
import { type CanvasPrimitiveKind, type CanvasSize } from './canvasConfig';
import {
  CANVAS_GRID_STEP_CHOICES,
  CANVAS_GRID_STEP_MAX,
  CANVAS_GRID_STEP_MIN,
  clampGridStep,
  gridDividesCanvas,
  type AlignAxis,
  type AlignMode,
} from './canvasEditArrange';
import { CanvasEditDockHostContext } from './canvasEditDockHost';
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

/**
 * 이름을 단 줄 버튼(도형 · 격자 붙임 · 순서). 폭을 가득 쓰므로 눌리는 면적이 아이콘 칸의
 * 여러 배이고, 그것이 이 변경의 요지다.
 */
const ROW_BUTTON_CLASS =
  'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 아이콘만 두는 칸(정렬 6종). 여기서는 이름을 달지 않는다 — 여섯 줄이 되면 도형 넷보다
 * 긴 목록이 되어, 자주 쓰는 도형이 스크롤 아래로 밀린다. 대신 아이콘 자체가 방향을 그리고
 * 있고(정렬은 모양으로 읽힌다) 칸을 28px 정사각으로 키워 누르기 쉬움만 되찾는다.
 */
const ICON_BUTTON_CLASS =
  'flex h-7 w-7 items-center justify-center rounded text-(--color-text-secondary) ' +
  'hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 쓸 수 없는 칸(정렬은 2개 이상, 순서는 1개 이상 골라야 한다).
 *
 * `pointer-events-none` 을 함께 두는 것에 뜻이 있다 — 없으면 흐려진 버튼 위에서 호버
 * 겉모습이 여전히 살아나 "누를 수 있다" 고 말한다.
 */
const DISABLED_CLASS = 'disabled:pointer-events-none disabled:opacity-40';

/** 묶음 제목. 본문(12px)보다 한 눈금 작은 곁들이 글자다 — 주변 설정 절과 같은 눈금이다. */
const SECTION_TITLE_CLASS = 'px-1 pb-1 text-[11px] font-medium text-(--color-text-muted)';

/** 격자 간격 칸. 줄 버튼과 같은 폭·같은 글자 크기라 한 묶음으로 읽힌다. */
const SELECT_CLASS =
  'w-full rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-xs ' +
  'tabular-nums text-(--color-text-secondary) focus:outline-none focus:ring-2 focus:ring-blue-300 ' +
  'disabled:pointer-events-none disabled:opacity-40';

/** 아이콘 크기. 줄 버튼과 아이콘 칸이 같은 크기를 쓴다 — 한 도구의 두 표현이다. */
const ICON_CLASS = 'h-4 w-4 shrink-0';

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

/** 정렬 버튼 하나 — 축·방식·아이콘·라벨 키가 한 자리에 산다. */
interface AlignControl {
  id: string;
  axis: AlignAxis;
  mode: AlignMode;
  icon: typeof Square;
  ariaKey: string;
}

/**
 * 정렬 버튼 6개 — 가로 3(왼쪽·가운데·오른쪽) · 세로 3(위·가운데·아래).
 *
 * 축 이름은 `panelEditAlign` 의 어휘를 **그대로** 쓴다(`horizontal` = 가로 변을 맞춘다).
 * 여기서 이름을 바꾸면 같은 것을 두 파일이 다른 말로 부르게 되고, 그 어긋남은 축을
 * 뒤바꾼 계산으로 나타나 화면에서만 드러난다.
 */
const ALIGN_CONTROLS: readonly AlignControl[] = [
  {
    id: 'left',
    axis: 'horizontal',
    mode: 'start',
    icon: AlignHorizontalJustifyStart,
    ariaKey: 'dashboard.canvas.edit.alignLeft',
  },
  {
    id: 'center-x',
    axis: 'horizontal',
    mode: 'center',
    icon: AlignHorizontalJustifyCenter,
    ariaKey: 'dashboard.canvas.edit.alignCenterX',
  },
  {
    id: 'right',
    axis: 'horizontal',
    mode: 'end',
    icon: AlignHorizontalJustifyEnd,
    ariaKey: 'dashboard.canvas.edit.alignRight',
  },
  {
    id: 'top',
    axis: 'vertical',
    mode: 'start',
    icon: AlignVerticalJustifyStart,
    ariaKey: 'dashboard.canvas.edit.alignTop',
  },
  {
    id: 'center-y',
    axis: 'vertical',
    mode: 'center',
    icon: AlignVerticalJustifyCenter,
    ariaKey: 'dashboard.canvas.edit.alignCenterY',
  },
  {
    id: 'bottom',
    axis: 'vertical',
    mode: 'end',
    icon: AlignVerticalJustifyEnd,
    ariaKey: 'dashboard.canvas.edit.alignBottom',
  },
];

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
 * 미리보기 영역을 **도크 | 미리보기** 두 칸으로 가른다.
 *
 * 꺼져 있으면 받은 자식을 **그대로** 돌려준다 — 감싸는 요소를 하나도 더하지 않으므로,
 * 캔버스가 아닌 패널의 미리보기 DOM 은 이 변경 전과 완전히 같다. 조건부로 한 겹을 더하는
 * 대신 언제나 감싸 두면 그 한 겹이 모든 패널 종류의 레이아웃 계산에 끼어든다.
 *
 * `flex-1 min-h-0` 은 fit 컨테이너에서 **옮겨 온 것이 아니라 이어받은 것**이다. 세로
 * 컬럼 안에서 남은 높이를 받던 자리를 이 행이 대신 받고, 행 안에서 fit 컨테이너가
 * 다시 `flex-1` 로 남은 **폭**을 받는다. 그 폭이 곧 `computePreviewStage` 가 실측하는
 * 값이므로 도크 폭은 미리보기 배율에 저절로 반영된다.
 */
export function CanvasEditDockRegion({
  enabled,
  children,
}: CanvasEditDockRegionProps): React.ReactElement {
  // 훅은 분기보다 앞이다 — `enabled` 가 바뀌어도 훅 수가 달라지지 않는다.
  const [host, setHost] = useState<HTMLDivElement | null>(null);

  if (!enabled) return <>{children}</>;

  return (
    <CanvasEditDockHostContext value={host}>
      <div className="flex min-h-0 flex-1 gap-2">
        {/*
          포털 목적지. 콜백 ref 로 state 에 담아야 붙는 순간이 렌더로 전해진다
          (`canvasEditDockHost` 주석). 오버레이가 그리기 전까지는 빈 상자다 —
          캔버스 패널이 아직 편집을 켜지 않았거나 아직 마운트되지 않은 그 한 프레임이다.
        */}
        <div ref={setHost} data-testid="canvas-dock" className={DOCK_CLASS} />
        {children}
      </div>
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
  /** 격자 표시·붙임(하나의 토글이 둘을 함께 켠다 — T12). */
  snapToGrid: boolean;
  onSnapToGridChange: (next: boolean) => void;
  /** 격자 간격(정수 캔버스 단위). 그림·붙임·Shift 한 칸이 **같은 값 하나**를 본다. */
  gridStep: number;
  onGridStepChange: (next: number) => void;
  /**
   * 보기 배율(**분수**) — 편집기가 작업 영역을 보여 주는 축척(SPEC-CANVAS-006 REQ-09).
   *
   * 이 컴포넌트는 값을 **그대로 지나 보낼 뿐** 백분율을 알지 못한다 — 칸도 환산도
   * `CanvasWorkspaceZoomField` 가 혼자 소유하므로(SPEC-CANVAS-006 M10) 이 파일에는
   * `100` 이라는 수가 한 번도 나오지 않는다(불변식 I4 와 같은 규율).
   */
  zoom: number;
  onZoomChange: (next: number) => void;
  /**
   * 캔버스 좌표계 크기 — **자투리 고지에만** 쓴다.
   *
   * 간격이 이 두 축을 나누어떨어뜨리는지가 "마지막 칸이 잘리는가" 를 정하고, 그것을 화면이
   * 미리 말하는 것이 자유 입력의 대가다(§격자 간격). 그리는 일에는 쓰지 않는다 — 그것은
   * 여전히 표면의 몫이다.
   */
  canvas: CanvasSize;
  /** 맞출 상대가 있는가(2개 이상). */
  canAlign: boolean;
  /** 순서를 옮길 것이 있는가(1개 이상). */
  canOrder: boolean;
  onAlign: (axis: AlignAxis, mode: AlignMode) => void;
  /** 참이면 맨 앞으로, 거짓이면 맨 뒤로. */
  onOrder: (toFront: boolean) => void;
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
  /**
   * 그룹 · 그룹 해제 컨트롤(SPEC-CANVAS-004 REQ-08).
   *
   * **노드를 통째로 받는 것에 뜻이 있다.** 같은 컨트롤이 대시보드의 떠 있는 줄에도 서야
   * 하므로(가정 A21 · 불변식 I23), 그 컴포넌트를 짓는 자리는 두 표면을 모두 가진 오버레이
   * 하나여야 한다. 도크가 콜백 넷(`canGroup`·`canUngroup`·`onGroup`·`onUngroup`)을 받아
   * 스스로 지으면 줄 쪽에도 같은 조립이 한 벌 더 생기고, 그 둘은 갈라질 수 있다 —
   * 배율 칸이 006 M10 에서 같은 이유로 같은 형상을 골랐다(불변식 I24).
   *
   * 도크가 더하는 것은 **자리와 이름**뿐이다.
   */
  groupTools: React.ReactNode;
  /**
   * 앵커 도구(SPEC-CANVAS-011 REQ-02 · AC-61).
   *
   * `groupTools` 와 **같은 규율**이다 — 노드를 통째로 받고, 도크가 더하는 것은 자리와
   * 이름뿐이다. 같은 컨트롤이 대시보드의 떠 있는 줄에도 서야 하므로 짓는 자리는 두
   * 표면을 모두 가진 오버레이 하나다(불변식 I24). 도구 상태(`CanvasTool`)를 여기로
   * 내리지 않는 것도 그 때문이다: 상태가 도크에 있으면 도크가 없는 대시보드에서 도구가
   * 통째로 사라진다 — 006 이 배달한 그 결함의 형상이다.
   */
  anchorTools: React.ReactNode;
  /**
   * 연결선 도구 넷(SPEC-CANVAS-011 REQ-03 · AC-61).
   *
   * `groupTools` · `anchorTools` 와 **한 글자도 다르지 않은 규율**이다 — 노드를 통째로
   * 받고, 도크가 더하는 것은 자리와 이름뿐이다. 짓는 자리는 두 표면을 모두 가진 오버레이
   * 하나이며(불변식 I24), 도구 상태는 여전히 그쪽에 산다.
   */
  connectorTools: React.ReactNode;
}

/**
 * 도크에 그려지는 도구 한 벌. **상태를 하나도 갖지 않는다** — 격자·선택·요소는 전부
 * 오버레이가 들고 있고 이 컴포넌트는 그것을 그리고 되돌려 줄 뿐이다. 그래서 이 파일은
 * jsdom 없이도 읽히는 순수한 표현층이고, "무엇이 일어나는가" 는 여전히 한 곳에 있다.
 *
 * 묶음마다 제목을 단다. 아이콘 띠였을 때는 구분선 하나가 그 일을 했지만, 구분선은
 * 스크린 리더에게 아무것도 아니다 — 제목은 보는 사람과 듣는 사람 모두에게 같은 경계를
 * 준다(`aria-labelledby` 로 묶음에 이름을 준다).
 */
export function CanvasEditDockBody({
  onPlace,
  onPlaceShape,
  snapToGrid,
  onSnapToGridChange,
  gridStep,
  onGridStepChange,
  zoom,
  onZoomChange,
  canvas,
  canAlign,
  canOrder,
  onAlign,
  onOrder,
  onScratchpadSave,
  canScratchpadSave,
  scratchpadDropActive,
  onScratchpadPlace,
  onSvgImport,
  groupTools,
  anchorTools,
  connectorTools,
}: CanvasEditDockBodyProps): React.ReactElement {
  const { t } = useTranslation();
  // 묶음 넷의 접힘 상태. 기기 지역에 남되 읽지 못해도 기능이 성립한다(REQ-06).
  const { collapsed, toggle } = usePaletteCollapse();
  // 한 화면에 캔버스 설정이 둘 이상 뜰 수 있으므로 고정 id 를 쓸 수 없다.
  const viewId = useId();
  const shapesId = useId();
  const gridId = useId();
  const alignId = useId();
  const orderId = useId();
  const groupId = useId();
  const anchorId = useId();
  const connectorId = useId();
  const suggestId = useId();
  const partialId = useId();
  const scratchpadId = useId();

  /** 지금 간격이 캔버스 두 축을 나누어떨어뜨리지 못하는가 — 마지막 칸이 반 칸이 된다. */
  const partialCell = !gridDividesCanvas(gridStep, canvas);

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
      // (`CanvasEditOverlay` §줄 위의 누름과 키), 도크에는 없었다 — 그래서 붙임 간격을
      // 적으려고 수치 칸에서 방향키를 누르면 수는 그대로이고 **요소가 움직였다**.
      // SPEC-CANVAS-010 이 Delete·Backspace 를 오버레이에 달면서 그 구멍의 대가가
      // "요소가 한 칸 움직인다" 에서 **"그림이 지워진다"** 로 커졌다(서랍 항목의 이름
      // 칸이 바로 그 자리다). 되돌리기가 없으므로 여기서 막는다.
      //
      // 표적을 보지 않고 **도크 전체**를 끊는다. 어느 칸이 글자를 받는지 세는 목록은
      // 칸이 늘 때마다 조용히 낡지만, "도크에서 누른 키는 도크의 것" 이라는 문장은
      // 낡지 않는다. `preventDefault` 는 쓰지 않는다 — 칸의 초점과 캐럿이 살아 있어야
      // 하고, `defaultPrevented` 는 `previewPan` 이 읽는 표시라 뜻이 번진다.
      onKeyDown={(event) => event.stopPropagation()}
    >
      {/* 보기 — **도크의 맨 앞**이다. 배율은 "보이지 않을 때 손이 가는" 컨트롤이라, 스크롤
          해야 찾을 수 있으면 바로 그 순간에 실패한다. 격자 묶음에 넣지 않는 이유는 그
          묶음의 간격 칸이 붙임 토글에 매여 있기 때문이다(`disabled={!snapToGrid}`) —
          배율은 붙임과 아무 상관이 없으므로 끌 수 있는 토글 아래에 그 토글을 따르지 않는
          칸을 두면 화면이 거짓말을 한다. 대가는 도형 넷이 두 줄만큼 아래로 밀리는 것이고,
          그 값이 56px 정도이며 대개는 스크롤이 아예 생기지 않는다(REQ-09). */}
      <section role="group" aria-labelledby={viewId} className="flex flex-col gap-1">
        <p id={viewId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockView')}
        </p>
        {/* 칸 자체는 **`CanvasWorkspaceZoomField` 가 혼자 소유한다**(SPEC-CANVAS-006 M10).
            도크와 대시보드의 떠 있는 줄이 **같은 컴포넌트**를 그리므로 범위 · `min`/`max` ·
            `aria` 배선 · 제안 넷 · 환산이 두 벌이 되지 않는다(불변식 I24). 이 파일이 주는
            것은 겉모습 하나(제 칸들과 같은 폭·같은 글자 크기)와 **도움말 표현**뿐이다.

            **붙임 토글에 매이지 않는다** — 격자를 꺼도 시야는 바꿀 수 있다. 그래서 이
            칸에는 `disabled` 가 없고, 그것이 이 묶음이 격자 묶음과 갈라선 이유다. */}
        <CanvasWorkspaceZoomField
          zoom={zoom}
          onZoomChange={onZoomChange}
          className={cn(SELECT_CLASS, 'flex-1')}
          // 상시 도움말 — 셋을 말한다: 편집 중 시야만 바꾼다 · 캔버스 크기도 요소 좌표도
          // 바꾸지 않는다 · **격자 간격이 배율을 눈금으로 나눈다**(가정 A20). 실제 축척을
          // 두 번째 수로 내지는 않는다 — 같은 것을 두 수로 말하면 어느 쪽이 참인지 화면이
          // 답하지 못한다. 도움말이 관계를 말하고 그림이 결과를 말한다.
          //
          // 도크에서는 **클릭 팝오버**다. 떠 있는 줄은 같은 문구를 `sr-only` 문단으로
          // 꽂는다 — 그 자리에서는 팝오버가 `overflow-hidden` 에 잘리기 때문이다.
          renderHelp={(describedById) => (
            <FieldHelp
              text={t('dashboard.canvas.edit.workspaceZoomHint')}
              describedById={describedById}
              testId="canvas-workspace-zoom-hint"
            />
          )}
        />
      </section>

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

      <section role="group" aria-labelledby={gridId} className="flex flex-col gap-1">
        <p id={gridId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockGrid')}
        </p>
        {/* 격자 붙임 — 표시와 붙임을 함께 켜는 **하나의** 토글이다(T12). 눌린 상태를
            `aria-pressed` 로 알린다 — 겉모습만으로는 스크린 리더가 읽을 것이 없다. */}
        <button
          type="button"
          data-testid="canvas-grid-toggle"
          aria-label={t('dashboard.canvas.edit.gridSnap')}
          aria-pressed={snapToGrid}
          title={t('dashboard.canvas.edit.gridSnap')}
          className={cn(ROW_BUTTON_CLASS, snapToGrid && 'bg-(--color-bg-elevated) text-blue-500')}
          onClick={() => onSnapToGridChange(!snapToGrid)}
        >
          <Grid3x3 className={ICON_CLASS} aria-hidden="true" />
          <span>{t('dashboard.canvas.edit.gridSnap')}</span>
        </button>
        {/* 격자 간격 — 적은 값 하나가 **그려지는 격자 · 붙는 눈금 · Shift+방향키 한 칸**을
            함께 정한다. 단위는 **캔버스 좌표 그대로**다: 500 폭 캔버스에 25 를 적으면 가로
            스무 칸이며, 패널을 어떻게 늘여도 그 칸 수는 변하지 않는다(옛 백분율 간격은
            스테이지 종횡비에 따라 세로에 반 칸짜리 자투리를 남겼다).

            목록이 아니라 자유 입력인 이유(0.11.0): 목록 넷을 고른 근거는 "넷 다 기본
            캔버스의 두 축을 나누어떨어뜨린다" 였는데, 그 성질은 **기본 캔버스에 한해서만**
            참이다. 캔버스를 640 × 480 으로 잡는 순간 25 도 자투리를 내므로, 목록은 지키려던
            것을 지키지 못한 채 고를 자유만 빼앗는다. 그래서 정수 하나를 받고, 넷은
            `<datalist>` 제안으로 남긴다 — 빈 칸 앞에서 "몇을 적지" 를 묻지 않아도 된다.

            나누어떨어지지 않는 값의 대가는 **숨기지 않고 말한다** — 옆의 `?` 가 그 일을 하며
            (아래 `FieldHelp`), 그것이 자유를 연 대가로 이 컨트롤이 새로 지는 몫이다.

            **지역 상태를 두지 않는다**(이 컴포넌트의 규율). 한 글자마다 죄되, 읽을 수 없는
            입력(빈 칸 · 글자)에는 `clampGridStep` 이 옛 값을 그대로 돌려주므로 지우는 도중에
            격자가 무너지지 않는다.

            격자가 꺼져 있으면 끈다 — 눌러도 화면이 그대로인 컨트롤은 고장으로 보인다
            (정렬 버튼이 같은 이유로 같은 일을 한다). */}
        <div className="flex items-center gap-1">
          <input
            type="number"
            inputMode="numeric"
            data-testid="canvas-grid-step"
            aria-label={t('dashboard.canvas.edit.gridStep')}
            title={t('dashboard.canvas.edit.gridStep')}
            aria-describedby={partialCell ? partialId : undefined}
            list={suggestId}
            min={CANVAS_GRID_STEP_MIN}
            max={CANVAS_GRID_STEP_MAX}
            step={1}
            disabled={!snapToGrid}
            value={gridStep}
            onChange={(event) => onGridStepChange(clampGridStep(event.target.value, gridStep))}
            className={cn(SELECT_CLASS, 'flex-1')}
          />
          {/* 자투리 고지 — **나누어떨어지지 않을 때만** 뜬다. 늘 떠 있으면 경고가 배경이 되어
              아무도 읽지 않고, 뜨고 지는 것 자체가 "지금 이 조합이 그렇다" 를 말한다. */}
          {partialCell && (
            <FieldHelp
              // `replaceAll` 이어야 한다 — 문구는 간격을 두 번 말한다("{step} 로 나누어
              // 떨어지지 않아… 그대로 {step} 단위로 동작합니다"). `replace` 는 첫 자리만
              // 바꾸므로 뒤쪽에 `{step}` 이 벌거벗은 채 남는다.
              text={t('dashboard.canvas.edit.gridStepPartial')
                .replaceAll('{step}', String(gridStep))
                .replaceAll('{width}', String(canvas.width))
                .replaceAll('{height}', String(canvas.height))}
              describedById={partialId}
              testId="canvas-grid-step-partial"
            />
          )}
        </div>
        {/* 제안값 — 고를 수 있는 값의 전부가 아니라 **곁들이**다(`canvasEditArrange`). */}
        <datalist id={suggestId} data-testid="canvas-grid-step-suggestions">
          {CANVAS_GRID_STEP_CHOICES.map((choice) => (
            // 단위 이름은 번역한다 — 벌거벗은 숫자만 보이면 그것이 백분율인지 좌표인지
            // 알 수 없고, 좌표계가 바뀐 그 변경에서 그 모호함이 바로 오해의 씨앗이었다.
            <option key={choice} value={choice}>
              {t('dashboard.canvas.edit.gridStepOption').replace('{step}', String(choice))}
            </option>
          ))}
        </datalist>
      </section>

      <section role="group" aria-labelledby={alignId} className="flex flex-col">
        <p id={alignId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockAlign')}
        </p>
        {/* 3열 — 가로 3종이 윗줄, 세로 3종이 아랫줄에 놓여 축이 줄로 읽힌다. */}
        <div className="grid grid-cols-3 gap-1">
          {ALIGN_CONTROLS.map((control) => {
            const Icon = control.icon;
            return (
              <button
                key={control.id}
                type="button"
                data-testid={`canvas-align-${control.id}`}
                aria-label={t(control.ariaKey)}
                title={t(control.ariaKey)}
                // 둘 미만이면 끈다 — 눌러도 아무 일이 없는 버튼은 사용자에게 고장으로 보인다.
                disabled={!canAlign}
                className={cn(ICON_BUTTON_CLASS, DISABLED_CLASS)}
                onClick={() => onAlign(control.axis, control.mode)}
              >
                <Icon className={ICON_CLASS} aria-hidden="true" />
              </button>
            );
          })}
        </div>
      </section>

      <section role="group" aria-labelledby={orderId} className="flex flex-col gap-0.5">
        <p id={orderId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockOrder')}
        </p>
        <button
          type="button"
          data-testid="canvas-order-front"
          aria-label={t('dashboard.canvas.edit.bringToFront')}
          title={t('dashboard.canvas.edit.bringToFront')}
          disabled={!canOrder}
          className={cn(ROW_BUTTON_CLASS, DISABLED_CLASS)}
          onClick={() => onOrder(true)}
        >
          <BringToFront className={ICON_CLASS} aria-hidden="true" />
          <span>{t('dashboard.canvas.edit.bringToFront')}</span>
        </button>
        <button
          type="button"
          data-testid="canvas-order-back"
          aria-label={t('dashboard.canvas.edit.sendToBack')}
          title={t('dashboard.canvas.edit.sendToBack')}
          disabled={!canOrder}
          className={cn(ROW_BUTTON_CLASS, DISABLED_CLASS)}
          onClick={() => onOrder(false)}
        >
          <SendToBack className={ICON_CLASS} aria-hidden="true" />
          <span>{t('dashboard.canvas.edit.sendToBack')}</span>
        </button>
      </section>

      {/* 그룹 — 순서 절 바로 뒤다(SPEC-CANVAS-004 REQ-08).

          자리의 근거: 묶기는 **선택 위에서 도는 연산**이라 정렬 · 순서와 같은 부류이고,
          그 셋이 붙어 있어야 "고른 것들에 무엇을 할 수 있는가" 가 한 눈에 읽힌다. 가져오기와
          서랍 앞에 두는 것도 그 때문이다 — 뒤엣둘은 "재료를 들여오는" 절이라 결이 다르다.

          **묶음 안의 컨트롤은 이 파일이 짓지 않는다**(`groupTools` prop). 같은 것이
          대시보드의 떠 있는 줄에도 서야 하므로 짓는 자리는 오버레이 하나여야 한다 —
          도크가 더하는 것은 자리와 이름뿐이다(불변식 I23 · I24). */}
      <section role="group" aria-labelledby={groupId} className="flex flex-col gap-0.5">
        <p id={groupId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockGroup')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{groupTools}</div>
      </section>

      {/* 앵커 — 그룹 절 바로 뒤다(SPEC-CANVAS-011 REQ-02).

          자리의 근거: 앵커 도구는 **선택 위에서 도는 연산이 아니다**(고른 것이 없어도 켜지고,
          켜진 동안 최상위 전부가 앵커를 보인다). 그래서 정렬 · 순서 · 그룹 셋의 뒤에 서되
          재료를 들여오는 절(가져오기 · 서랍) 앞이다 — 앞 셋이 "고른 것에 무엇을 할 수
          있는가" 라면 이 절은 "지금 손이 무엇을 하는 중인가" 이고, 그 물음은 재료보다
          먼저 온다.

          **묶음 안의 컨트롤은 이 파일이 짓지 않는다**(`anchorTools` prop) — `groupTools` 와
          한 글자도 다르지 않은 규율이다(불변식 I23 · I24). */}
      <section role="group" aria-labelledby={anchorId} className="flex flex-col gap-0.5">
        <p id={anchorId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockAnchor')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{anchorTools}</div>
      </section>

      {/* 연결선 — 앵커 절 **바로 뒤**다(SPEC-CANVAS-011 REQ-03 · M8).

          자리의 근거: 잇는 일은 앵커 위에서 일어난다. 두 절을 붙여 두면 목록을 훑는 손이
          "붙을 자리를 보이고 → 그 자리를 잇는다" 를 한 무리로 읽는다. 갈라 놓으면 연결선
          도구를 켰을 때 왜 점이 함께 뜨는지(REQ-02-b) 화면이 말하지 않는다.

          **묶음 안의 컨트롤은 이 파일이 짓지 않는다**(`connectorTools` prop) — 앞의 두
          묶음과 같은 규율이다(불변식 I23 · I24). */}
      <section role="group" aria-labelledby={connectorId} className="flex flex-col gap-0.5">
        <p id={connectorId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockConnector')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{connectorTools}</div>
      </section>

      {/* 가져오기 — 순서 절과 스크래치패드 사이다(SPEC-CANVAS-007 REQ-04).

          **위 다섯 절의 자리가 한 줄도 달라지지 않는 자리**를 고른 것이다(008 이 서랍을 맨
          끝에 둔 그 판단과 같다). 뜻으로는 도형 절 옆에 서야 하지만(카탈로그가 "우리가 준
          재료" 라면 가져오기는 "당신이 가져온 재료" 다), 가운데에 끼우면 격자 · 정렬 · 순서가
          한꺼번에 스크롤 아래로 밀린다.

          **기본으로 접혀 있다** — 도크 폭이 `w-44` 고정이고 가져오기는 한 번 쓰고 닫는
          일이다. 그래서 펴지 않은 화면은 007 이전과 **묶음 머리 한 줄만** 다르다(AC-E1).

          **캔버스 표면에 아무것도 그리지 않는다**(불변식 K10). 파일을 캔버스 위로 끌어놓는
          길을 두지 않는 것이 그 성질의 값이며, 006 이 배달한 I23 결함의 재발을 형상으로 막는다. */}
      <CanvasSvgImport canvas={canvas} onPlace={onSvgImport} />

      {/* 스크래치패드 — **도크의 맨 끝**이다(SPEC-CANVAS-008 REQ-03).

          뜻으로는 도형 절 옆에 서야 한다(카탈로그가 "우리가 준 재료" 라면 서랍은 "당신이
          만든 재료" 다). 그런데 이 절은 드롭 존과 목록을 함께 들어 새 절 가운데 가장 키가
          크고, 도크는 폭 `w-44` 의 세로 스크롤 상자다 — 가운데에 두면 격자 · 정렬 · 순서가
          한꺼번에 스크롤 아래로 밀린다. M7 이 카탈로그 묶음 셋을 기본 접힘으로 둔 것과 같은
          판단이며, 덤으로 위 다섯 절의 자리가 이 변경 전과 **한 줄도 달라지지 않는다**. */}
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
