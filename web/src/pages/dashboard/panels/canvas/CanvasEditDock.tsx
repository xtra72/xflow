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
  Circle,
  Grid3x3,
  Minus,
  SendToBack,
  Square,
  Type,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { type CanvasElementKind } from './canvasConfig';
import { CANVAS_GRID_STEP_CHOICES, type AlignAxis, type AlignMode } from './canvasEditArrange';
import { CanvasEditDockHostContext } from './canvasEditDockHost';

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

/** 격자 간격 고르개. 줄 버튼과 같은 폭·같은 글자 크기라 한 묶음으로 읽힌다. */
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
 */
const PALETTE_KINDS: readonly CanvasElementKind[] = ['rect', 'ellipse', 'line', 'text'];

/**
 * 버튼의 `aria-label` i18n 키("사각형 놓기"). **키 이름 안에 점을 넣지 않는다**(프로젝트 규약).
 *
 * 보이는 이름(`PALETTE_NAME_KEYS`)이 생긴 뒤에도 남는다 — 이쪽이 **무엇을 하는가**를
 * 말하고 보이는 쪽은 **무엇인가**를 말한다.
 */
const PALETTE_ARIA_KEYS: Record<CanvasElementKind, string> = {
  rect: 'dashboard.canvas.edit.paletteRect',
  ellipse: 'dashboard.canvas.edit.paletteEllipse',
  line: 'dashboard.canvas.edit.paletteLine',
  text: 'dashboard.canvas.edit.paletteText',
};

/** 아이콘 옆에 보이는 도형 이름. `aria-label` 이 이 문자열을 포함한다(WCAG 2.5.3). */
const PALETTE_NAME_KEYS: Record<CanvasElementKind, string> = {
  rect: 'dashboard.canvas.edit.shapeRect',
  ellipse: 'dashboard.canvas.edit.shapeEllipse',
  line: 'dashboard.canvas.edit.shapeLine',
  text: 'dashboard.canvas.edit.shapeText',
};

/** 도형 아이콘. */
const PALETTE_ICONS: Record<CanvasElementKind, typeof Square> = {
  rect: Square,
  ellipse: Circle,
  line: Minus,
  text: Type,
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
  /** 도형을 놓는다 — 목록 편집기의 추가 버튼과 **같은 생성 경로**여야 한다(가정 A7). */
  onPlace: (kind: CanvasElementKind) => void;
  /** 격자 표시·붙임(하나의 토글이 둘을 함께 켠다 — T12). */
  snapToGrid: boolean;
  onSnapToGridChange: (next: boolean) => void;
  /** 격자 간격(%). 그려지는 격자와 붙는 눈금이 **같은 값 하나**를 본다. */
  gridStep: number;
  onGridStepChange: (next: number) => void;
  /** 맞출 상대가 있는가(2개 이상). */
  canAlign: boolean;
  /** 순서를 옮길 것이 있는가(1개 이상). */
  canOrder: boolean;
  onAlign: (axis: AlignAxis, mode: AlignMode) => void;
  /** 참이면 맨 앞으로, 거짓이면 맨 뒤로. */
  onOrder: (toFront: boolean) => void;
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
  snapToGrid,
  onSnapToGridChange,
  gridStep,
  onGridStepChange,
  canAlign,
  canOrder,
  onAlign,
  onOrder,
}: CanvasEditDockBodyProps): React.ReactElement {
  const { t } = useTranslation();
  // 한 화면에 캔버스 설정이 둘 이상 뜰 수 있으므로 고정 id 를 쓸 수 없다.
  const shapesId = useId();
  const gridId = useId();
  const alignId = useId();
  const orderId = useId();

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
    >
      <section role="group" aria-labelledby={shapesId} className="flex flex-col gap-0.5">
        <p id={shapesId} className={SECTION_TITLE_CLASS}>
          {t('dashboard.canvas.edit.dockShapes')}
        </p>
        {PALETTE_KINDS.map((kind) => {
          const Icon = PALETTE_ICONS[kind];
          return (
            <button
              key={kind}
              type="button"
              data-testid={`canvas-palette-add-${kind}`}
              aria-label={t(PALETTE_ARIA_KEYS[kind])}
              title={t(PALETTE_ARIA_KEYS[kind])}
              className={ROW_BUTTON_CLASS}
              onClick={() => onPlace(kind)}
            >
              <Icon className={ICON_CLASS} aria-hidden="true" />
              <span>{t(PALETTE_NAME_KEYS[kind])}</span>
            </button>
          );
        })}
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
        {/* 격자 간격 — 고른 값 하나가 **그려지는 격자와 붙는 눈금을 함께** 정한다.
            자유 입력이 아니라 목록인 이유: 임의의 수를 받으면 100 을 나누어떨어지지 않는
            간격(예: 7%)이 들어와 마지막 칸이 잘리고, 그 잘린 칸에도 붙기 때문에 "왜 저기
            붙지" 가 된다. 고를 수 있는 값은 `canvasEditArrange` 가 소유한다.

            격자가 꺼져 있으면 끈다 — 눌러도 화면이 그대로인 컨트롤은 고장으로 보인다
            (정렬 버튼이 같은 이유로 같은 일을 한다). */}
        <select
          data-testid="canvas-grid-step"
          aria-label={t('dashboard.canvas.edit.gridStep')}
          title={t('dashboard.canvas.edit.gridStep')}
          disabled={!snapToGrid}
          value={gridStep}
          onChange={(event) => onGridStepChange(Number(event.target.value))}
          className={SELECT_CLASS}
        >
          {CANVAS_GRID_STEP_CHOICES.map((choice) => (
            // 눈금 이름은 숫자와 `%` 뿐이라 번역할 것이 없다.
            <option key={choice} value={choice}>{`${choice}%`}</option>
          ))}
        </select>
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
    </div>
  );
}
