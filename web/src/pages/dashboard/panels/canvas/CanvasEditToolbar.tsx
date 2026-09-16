// 캔버스 편집 도구 띠 — 도크의 세로 목록에서 **미리보기 제목 아래 가로 띠**로 옮겨 온
// 절 일곱(SPEC-CANVAS-011 후속 · 사용자 결정 2026-09-16).
//
// ## 무엇이 갈라졌는가
//
// 002 가 도구 한 벌을 왼쪽 도크 한 자리에 모았고, 004 · 007 · 008 · 011 이 그 위에 절을
// 넷 더 쌓았다. 열 절이 폭 `w-44` 세로 상자에 서자 도크는 **스크롤 상자**가 되었다 —
// 정렬을 쓰려면 굴려 내려야 하고, 굴려 내린 동안 도형 팔레트가 화면에서 사라진다.
//
// 가르는 자는 **절이 무엇에 대해 말하는가** 하나다.
//
//   - 남는 셋(도형 · 가져오기 · 서랍)은 전부 **"캔버스에 무언가를 놓는다"** 를 말한다.
//     팔레트도 가져오기도 서랍도 같은 몸짓의 세 입구이며, 셋 다 **목록**을 들고 있어
//     세로로 자랄 자리를 원한다. 그래서 세로 상자인 도크에 남는다.
//   - 오는 일곱(작업 영역 · 격자 · 정렬 · 순서 · 그룹 · 앵커 · 연결선)은 전부 **"캔버스에,
//     또는 이미 고른 것에 작용한다"** 를 말한다. 어느 것도 목록을 들지 않고 모두 단추
//     하나 · 칸 하나짜리라, 가로로 늘어놓으면 한 줄에 다 보인다 — 찾으려고 굴릴 것이
//     없어진다.
//
// ## 왜 제목 **옆**이 아니라 **아래**인가
//
// 미리보기 제목 줄에는 이미 미리보기 자신의 컨트롤 여섯이 서 있다(배율 −/%/+ · 채움 토글 ·
// 배치 초기화 · 접기). 그 여섯은 **미리보기라는 창**에 대한 것이고 이 띠의 스물두 개는
// **캔버스의 내용물**에 대한 것이다. 한 줄에 섞으면 둘 다 읽히지 않고, 무엇보다 두 배율이
// 나란히 서서 같은 것의 두 버튼처럼 보인다 — 그래서 줄을 따로 쓰고, 이 띠의 배율은
// 제가 재는 상자의 이름(**작업 영역**)을 글자로 단다.
//
// ## 두 표면 규율은 그대로다 (불변식 I23 · I24)
//
// 그룹 · 앵커 · 연결선 컨트롤은 **여기서도 지어지지 않는다.** 노드를 통째로 받아 자리와
// 이름만 더하는 것이 도크가 지던 규율이었고, 그 규율이 파일과 함께 옮겨 왔을 뿐이다 —
// 짓는 자리는 두 표면(띠 · 대시보드의 떠 있는 줄)을 모두 가진 `CanvasEditOverlay` 하나다.
//
// @spec SPEC-CANVAS-011 REQ-02 REQ-03

import { useId } from 'react';

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
import { type CanvasSize } from './canvasConfig';
import {
  CANVAS_GRID_STEP_CHOICES,
  CANVAS_GRID_STEP_MAX,
  CANVAS_GRID_STEP_MIN,
  clampGridStep,
  gridDividesCanvas,
  type AlignAxis,
  type AlignMode,
} from './canvasEditArrange';

// --- 겉모습 --------------------------------------------------------------

/**
 * 띠 자체. **폭을 가득 쓰고 넘치면 접는다**(`flex-wrap`) — 설정 창은 좌우 분할 손잡이로
 * 좁아질 수 있고, 좁아진 폭에서 한 줄을 고집하면 절이 잘려 나간다. 접히면 두 줄이 되고
 * 그때도 절의 경계는 제목이 알린다.
 *
 * `gap-x-4` 와 절 안의 `gap-1` 이 두 배 이상 벌어지는 것에 뜻이 있다 — 근접이 곧 묶음
 * 이므로, 그 차이가 구분선을 대신한다(구분선은 접힌 줄머리에서 갈 곳을 잃는다).
 */
const BAND_CLASS =
  'flex w-full flex-wrap items-center gap-x-4 gap-y-2 rounded-md border ' +
  'border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5';

/** 절 하나 — 제목과 컨트롤이 **한 줄로** 선다(도크에서는 제목이 위에 있었다). */
const SECTION_CLASS = 'flex items-center gap-1.5';

/** 절 제목. 본문(12px)보다 한 눈금 작은 곁들이 글자다 — 도크가 쓰던 그 눈금 그대로다. */
const SECTION_TITLE_CLASS = 'whitespace-nowrap text-[11px] font-medium text-(--color-text-muted)';

/**
 * 이름을 단 줄 버튼(격자 붙임 · 순서). 도크의 그것에서 `w-full` 하나만 뺐다 — 세로 상자
 * 에서는 폭을 가득 쓰는 것이 누를 면적이었지만, 가로 띠에서 그러면 한 줄을 혼자 먹는다.
 * 글자 크기는 여전히 주변 설정 화면과 같은 `text-xs`(12px)다.
 */
const ROW_BUTTON_CLASS =
  'flex items-center gap-1.5 whitespace-nowrap rounded px-2 py-1 text-left text-xs ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 아이콘만 두는 칸(정렬 6종). 도크에서와 같은 28px 정사각이다 — 이름을 달지 않는 근거도
 * 같다: 정렬은 아이콘 자체가 방향을 그리고 있어 모양으로 읽힌다.
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

/**
 * 수치 칸(작업 영역 배율 · 격자 간격). **폭을 못 박는다** — 띠에서는 `flex-1` 이 남은
 * 폭을 통째로 먹어 뒤 절들을 다음 줄로 밀어낸다. 세 자리 수(100)가 들어가는 최소 폭이다.
 */
const FIELD_CLASS =
  'w-16 rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-xs ' +
  'tabular-nums text-(--color-text-secondary) focus:outline-none focus:ring-2 focus:ring-blue-300 ' +
  'disabled:pointer-events-none disabled:opacity-40';

/** 아이콘 크기. 줄 버튼과 아이콘 칸이 같은 크기를 쓴다 — 한 도구의 두 표현이다. */
const ICON_CLASS = 'h-4 w-4 shrink-0';

// --- 정렬 표 ------------------------------------------------------------

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

// --- 도구 --------------------------------------------------------------

export interface CanvasEditToolbarBodyProps {
  /** 격자 표시·붙임(하나의 토글이 둘을 함께 켠다 — T12). */
  snapToGrid: boolean;
  onSnapToGridChange: (next: boolean) => void;
  /** 격자 간격(정수 캔버스 단위). 그림·붙임·Shift 한 칸이 **같은 값 하나**를 본다. */
  gridStep: number;
  onGridStepChange: (next: number) => void;
  /**
   * 작업 영역 배율(**분수**) — 편집기가 작업 영역을 보여 주는 축척(SPEC-CANVAS-006 REQ-09).
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
   * 그룹 · 그룹 해제 · 부품 분리 컨트롤(SPEC-CANVAS-004 REQ-08).
   *
   * **노드를 통째로 받는 것에 뜻이 있다.** 같은 컨트롤이 대시보드의 떠 있는 줄에도 서야
   * 하므로(가정 A21 · 불변식 I23), 그 컴포넌트를 짓는 자리는 두 표면을 모두 가진 오버레이
   * 하나여야 한다. 띠가 콜백들을 받아 스스로 지으면 줄 쪽에도 같은 조립이 한 벌 더 생기고,
   * 그 둘은 갈라질 수 있다 — 배율 칸이 006 M10 에서 같은 이유로 같은 형상을 골랐다
   * (불변식 I24).
   *
   * 띠가 더하는 것은 **자리와 이름**뿐이다.
   */
  groupTools: React.ReactNode;
  /**
   * 앵커 도구(SPEC-CANVAS-011 REQ-02 · AC-61).
   *
   * `groupTools` 와 **같은 규율**이다 — 노드를 통째로 받고, 띠가 더하는 것은 자리와
   * 이름뿐이다. 도구 상태(`CanvasTool`)를 여기로 내리지 않는 것도 그 때문이다: 상태가
   * 띠에 있으면 띠가 없는 대시보드에서 도구가 통째로 사라진다 — 006 이 배달한 그 결함의
   * 형상이다.
   */
  anchorTools: React.ReactNode;
  /**
   * 연결선 도구 넷(SPEC-CANVAS-011 REQ-03 · AC-61).
   *
   * 앞의 둘과 **한 글자도 다르지 않은 규율**이다 — 노드를 통째로 받고, 짓는 자리는 두
   * 표면을 모두 가진 오버레이 하나다(불변식 I24).
   */
  connectorTools: React.ReactNode;
}

/**
 * 띠에 그려지는 도구 일곱 절. **상태를 하나도 갖지 않는다** — 격자·선택·요소는 전부
 * 오버레이가 들고 있고 이 컴포넌트는 그것을 그리고 되돌려 줄 뿐이다(도크가 지던 규율
 * 그대로다).
 *
 * 절마다 제목을 단다. 스물두 개를 한 줄로 펴 놓고 제목을 걷으면 정렬 여섯과 연결선 넷이
 * 구분 없는 아이콘 열이 되어, 찾는 일이 도크 시절보다 어려워진다 — 옮긴 뜻이 사라지는
 * 자리다. 제목은 보는 사람에게 경계를 주고 `aria-labelledby` 로 듣는 사람에게 같은
 * 경계를 준다.
 */
export function CanvasEditToolbarBody({
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
  groupTools,
  anchorTools,
  connectorTools,
}: CanvasEditToolbarBodyProps): React.ReactElement {
  const { t } = useTranslation();
  // 한 화면에 캔버스 설정이 둘 이상 뜰 수 있으므로 고정 id 를 쓸 수 없다.
  const viewId = useId();
  const gridId = useId();
  const alignId = useId();
  const orderId = useId();
  const groupId = useId();
  const anchorId = useId();
  const connectorId = useId();
  const suggestId = useId();
  const partialId = useId();

  /** 지금 간격이 캔버스 두 축을 나누어떨어뜨리지 못하는가 — 마지막 칸이 반 칸이 된다. */
  const partialCell = !gridDividesCanvas(gridStep, canvas);

  return (
    <div
      data-testid="canvas-toolbar-panel"
      role="group"
      aria-label={t('dashboard.canvas.edit.toolbarAria')}
      className={BAND_CLASS}
      // 띠는 DOM 상으로 패널 밖에 있지만 **React 트리 상으로는 오버레이의 자식**이다
      // (포털). 그래서 누름이 오버레이 루트의 히트 테스트까지 흘러가며, 끊지 않으면
      // 버튼을 눌렀는데 그 뒤에 있는 도형이 함께 골라지고 이동 드래그까지 시작된다.
      onPointerDown={(event) => event.stopPropagation()}
      // **키도 같은 길로 오른다.** 끊지 않으면 붙임 간격을 적으려고 수치 칸에서 방향키를
      // 누를 때 수는 그대로이고 **요소가 움직이며**, Backspace 는 **그림을 지운다**
      // (되돌리기가 없다 — SPEC-CANVAS-010). 도크가 같은 한 줄로 막고 있었고, 그 칸들이
      // 이리로 왔으므로 그 한 줄도 함께 온다.
      //
      // 표적을 보지 않고 **띠 전체**를 끊는다. 어느 칸이 글자를 받는지 세는 목록은 칸이
      // 늘 때마다 조용히 낡지만, "띠에서 누른 키는 띠의 것" 이라는 문장은 낡지 않는다.
      // `preventDefault` 는 쓰지 않는다 — 칸의 초점과 캐럿이 살아 있어야 하고,
      // `defaultPrevented` 는 `previewPan` 이 읽는 표시라 뜻이 번진다.
      onKeyDown={(event) => event.stopPropagation()}
    >
      {/* 작업 영역 — **띠의 맨 앞**이다(도크에서도 맨 앞이었다).

          이름이 "보기" 에서 **"작업 영역"** 으로 바뀐 것이 이 절의 유일한 새것이다.
          까닭은 이웃에 있다: 바로 위 제목 줄에 미리보기 자신의 `%` 가 서 있으므로, 두
          배율이 몇 센티미터 안에 나란히 선다. 이 칸이 재는 것은 **편집기가 작업 영역을
          보여 주는 축척**이고 위쪽 것이 재는 것은 **설정 미리보기가 그려지는 크기**다 —
          둘은 다른 상자를 재며, 그 상자의 이름을 `canvasWorkspace.ts` 가 이미 작업 영역 ·
          출력 영역으로 부르고 있다. 그 어휘를 화면이 그대로 쓴다.

          **키 이름은 `dock*` 그대로 둔다.** 옮겨 온 일곱 절이 전부 그 앞자리를 쓰고 있고,
          키 이름은 화면에 나오지 않는 **식별자**다 — 일곱을 한꺼번에 개명하면 그 이름을
          열거하는 i18n 시험 넷이 함께 흔들리는데, 사용자가 얻는 것은 없다. 바뀐 것은
          이름이 아니라 **값**이다(보기 → 작업 영역).

          칸 자체는 **`CanvasWorkspaceZoomField` 가 혼자 소유한다**(SPEC-CANVAS-006 M10).
          띠와 대시보드의 떠 있는 줄이 **같은 컴포넌트**를 그리므로 범위 · `min`/`max` ·
          `aria` 배선 · 제안 넷 · 환산이 두 벌이 되지 않는다(불변식 I24).

          **붙임 토글에 매이지 않는다** — 격자를 꺼도 시야는 바꿀 수 있다. 그래서 이
          칸에는 `disabled` 가 없고, 그것이 이 절이 격자 절과 갈라선 이유다. */}
      <section role="group" aria-labelledby={viewId} className={SECTION_CLASS}>
        <p id={viewId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockView')}
        </p>
        <CanvasWorkspaceZoomField
          zoom={zoom}
          onZoomChange={onZoomChange}
          className={FIELD_CLASS}
          // 상시 도움말 — 셋을 말한다: 편집 중 시야만 바꾼다 · 캔버스 크기도 요소 좌표도
          // 바꾸지 않는다 · **격자 간격이 배율을 눈금으로 나눈다**(가정 A20). 실제 축척을
          // 두 번째 수로 내지는 않는다 — 같은 것을 두 수로 말하면 어느 쪽이 참인지 화면이
          // 답하지 못한다. 도움말이 관계를 말하고 그림이 결과를 말한다.
          renderHelp={(describedById) => (
            <FieldHelp
              text={t('dashboard.canvas.edit.workspaceZoomHint')}
              describedById={describedById}
              testId="canvas-workspace-zoom-hint"
            />
          )}
        />
      </section>

      <section role="group" aria-labelledby={gridId} className={SECTION_CLASS}>
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
            스무 칸이며, 패널을 어떻게 늘여도 그 칸 수는 변하지 않는다.

            목록이 아니라 자유 입력인 이유(0.11.0): 목록 넷을 고른 근거는 "넷 다 기본
            캔버스의 두 축을 나누어떨어뜨린다" 였는데, 그 성질은 **기본 캔버스에 한해서만**
            참이다. 캔버스를 640 × 480 으로 잡는 순간 25 도 자투리를 내므로, 목록은 지키려던
            것을 지키지 못한 채 고를 자유만 빼앗는다. 그래서 정수 하나를 받고, 넷은
            `<datalist>` 제안으로 남긴다.

            **지역 상태를 두지 않는다**(이 컴포넌트의 규율). 한 글자마다 죄되, 읽을 수 없는
            입력(빈 칸 · 글자)에는 `clampGridStep` 이 옛 값을 그대로 돌려주므로 지우는 도중에
            격자가 무너지지 않는다.

            격자가 꺼져 있으면 끈다 — 눌러도 화면이 그대로인 컨트롤은 고장으로 보인다
            (정렬 버튼이 같은 이유로 같은 일을 한다). */}
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
          className={FIELD_CLASS}
        />
        {/* 자투리 고지 — **나누어떨어지지 않을 때만** 뜬다. 늘 떠 있으면 경고가 배경이 되어
            아무도 읽지 않고, 뜨고 지는 것 자체가 "지금 이 조합이 그렇다" 를 말한다. */}
        {partialCell && (
          <FieldHelp
            // `replaceAll` 이어야 한다 — 안내는 간격을 두 번 말한다("{step} 로 나누어
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

      <section role="group" aria-labelledby={alignId} className={SECTION_CLASS}>
        <p id={alignId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockAlign')}
        </p>
        {/* 도크에서는 3열 격자였다(가로 3종 윗줄 · 세로 3종 아랫줄). 띠에서는 **한 줄**로
            선다 — 두 줄짜리 격자를 가로 띠에 두면 띠 높이가 그 절 하나 때문에 두 배가 된다.
            차례는 그대로이므로 앞 셋이 가로, 뒤 셋이 세로다. */}
        <div className="flex items-center gap-1">
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

      <section role="group" aria-labelledby={orderId} className={SECTION_CLASS}>
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
          그 셋이 붙어 있어야 "고른 것들에 무엇을 할 수 있는가" 가 한 눈에 읽힌다.

          **절 안의 컨트롤은 이 파일이 짓지 않는다**(`groupTools` prop) — 띠가 더하는 것은
          자리와 이름뿐이다(불변식 I23 · I24). */}
      <section role="group" aria-labelledby={groupId} className={SECTION_CLASS}>
        <p id={groupId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockGroup')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{groupTools}</div>
      </section>

      {/* 앵커 — 그룹 절 바로 뒤다(SPEC-CANVAS-011 REQ-02).

          자리의 근거: 앵커 도구는 **선택 위에서 도는 연산이 아니다**(고른 것이 없어도 켜지고,
          켜진 동안 최상위 전부가 앵커를 보인다). 그래서 정렬 · 순서 · 그룹 셋의 뒤에 선다 —
          앞 셋이 "고른 것에 무엇을 할 수 있는가" 라면 이 절은 "지금 손이 무엇을 하는 중인가"
          이다. */}
      <section role="group" aria-labelledby={anchorId} className={SECTION_CLASS}>
        <p id={anchorId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockAnchor')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{anchorTools}</div>
      </section>

      {/* 연결선 — 앵커 절 **바로 뒤**다(SPEC-CANVAS-011 REQ-03 · M8).

          자리의 근거: 잇는 일은 앵커 위에서 일어난다. 두 절을 붙여 두면 훑는 손이 "붙을
          자리를 보이고 → 그 자리를 잇는다" 를 한 무리로 읽는다. 갈라 놓으면 연결선 도구를
          켰을 때 왜 점이 함께 뜨는지(REQ-02-b) 화면이 말하지 않는다. */}
      <section role="group" aria-labelledby={connectorId} className={SECTION_CLASS}>
        <p id={connectorId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockConnector')}
        </p>
        <div className="flex flex-wrap items-center gap-1">{connectorTools}</div>
      </section>
    </div>
  );
}
