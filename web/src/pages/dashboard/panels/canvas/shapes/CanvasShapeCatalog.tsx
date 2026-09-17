// 도형 카탈로그 팔레트 — 접히는 묶음과 그 격자 (SPEC-CANVAS-008 M7 · REQ-02 · REQ-06).
//
// **이 파일은 도크 안에서만 산다.** 도크 자리(`canvasEditDockHost`)가 없으면 오버레이가
// 도크 본문을 아예 그리지 않고, 이 컴포넌트는 그 본문의 자식이다. 그래서 대시보드에 놓인
// 패널에는 묶음 머리도 · 칸도 · 미리보기도 **하나도 없다.**
//
// 그 "없음" 이 006 이 배달한 결함(불변식 I23)의 되풀이가 아닌 이유를 이름으로 적는다:
// 팔레트는 **표시 층이 아니라 컨트롤**이다. 006 이 깬 것은 "캔버스 표면에 조건 없이 그려지는
// 층을 다스릴 손잡이가 그 표면에 없다" 는 형상이었는데, 카탈로그는 캔버스 표면에 **아무것도
// 그리지 않으므로** 다스릴 층 자체를 만들지 않는다. 놓인 경로 도형을 다스리는 컨트롤
// (선택 · 드래그 · 8핸들 · 정렬 · 순서)은 006 M7 이 작업 영역까지 넓혀 둔 그대로 대시보드
// 에서도 닿는다. 층과 손잡이의 렌더 조건이 **같은 한 값**이라 I23 은 형상으로 지켜진다.
//
// **미리보기는 실제 렌더 경로다**(REQ-06 · 004 §심볼 고르기 UI 의 규율). 별도 썸네일도 별도
// 그리기 코드도 만들지 않고 `drawElements` 를 그대로 부른다 — 그래야 "그리지 못하는 도형" 이
// 카탈로그에 들어온 순간 화면에서 즉시 드러난다. 두 번째 그리기 경로를 만들면 미리보기는
// 멀쩡한데 캔버스에서만 이상한 도형이 나올 수 있다.
//
// **씨앗 스타일도 실제 그것이다.** 미리보기는 `pathSeedStyle` 을 부르므로, 칸에 보이는 모습이
// 곧 눌렀을 때 놓이는 모습이다(닫힌 도형은 채워지고 곡선 화살표는 선으로만 그려진다).
//
// @spec SPEC-CANVAS-008 REQ-02 · REQ-06 · AC-03 · AC-E9

import { useEffect, useId, useMemo, useRef, type ReactNode } from 'react';

import { ChevronDown, ChevronRight } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { CanvasElement } from '../canvasConfig';
import { clearSurface, drawElements, type DrawContext2D } from '../drawElement';
import {
  PREVIEW,
  PREVIEW_PROJECTION,
  PREVIEW_SCALE,
  previewElement,
} from './previewElements';
import {
  PALETTE_GROUP_TITLE_KEYS,
  type PaletteCollapseState,
  type PaletteGroupId,
} from './paletteGroups';
import { SHAPE_GROUPS, type ShapeCatalogEntry } from './shapeCatalog';

// --- 겉모습 --------------------------------------------------------------

/**
 * 묶음 머리 — 폭을 가득 쓰는 줄이다. 도형 줄 버튼과 같은 눈금을 쓰되 제목처럼 보이도록
 * 글자를 한 눈금 줄인다(주변 절 제목과 같은 `text-[11px]`).
 */
const GROUP_HEAD_CLASS =
  'flex w-full items-center gap-1 rounded px-1 py-1 text-left text-[11px] font-medium ' +
  'text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 카탈로그 격자 — **두 칸**이다. 셋으로 늘리면 칸이 50px 남짓이 되어 "평행사변형" 같은
 * 이름이 두 글자만 남고, 이름을 달려고 도크를 옮긴 그 결정이 무의미해진다.
 */
const GRID_CLASS = 'grid grid-cols-2 gap-0.5 pt-0.5';

/**
 * 칸 하나 — 미리보기 위, 이름 아래.
 *
 * **내보내는 것에 뜻이 있다.** 011 이후 이 격자의 첫 네 칸은 도크가 그린다(원시형 넷).
 * 도크가 비슷한 클래스를 제 자리에 한 벌 더 적으면 두 벌이 언젠가 갈라지고, 그 갈라짐은
 * "한 묶음 안에 두 생김새" 로 화면에 나온다 — 011 이 방금 고친 그 결함이다.
 */
export const CELL_CLASS =
  'flex w-full flex-col items-center gap-0.5 rounded px-1 py-1 text-[11px] ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

const CHEVRON_CLASS = 'h-3 w-3 shrink-0';

// --- 미리보기 ------------------------------------------------------------


/**
 * 칸 하나의 미리보기. **어떤 요소든 받는다** — 카탈로그 경로 30종도, 원시형 넷도.
 *
 * 011 이 이 부품을 일반화했다. 넷이 lucide 글리프를 지고 있던 동안에는 한 격자 안에
 * 윤곽선 글리프와 파란 도형이라는 두 벌의 잉크가 서 있었다. 같은 `drawElements` 를 지나게
 * 하면 그 둘이 하나가 되고, 덤으로 **"그리지 못하는 원시형" 이 여기서 즉시 드러난다** —
 * 카탈로그가 미리보기를 별도 썸네일로 짓지 않은 그 이유(REQ-06)가 넷에도 걸린다.
 *
 * `element` 는 **참조가 안정적이어야 한다.** 렌더마다 새 객체를 주면 아래 효과가 매번 다시
 * 돌아 같은 그림을 거듭 그린다. 두 호출자 모두 그 규율을 지킨다(카탈로그는 `useMemo`,
 * 원시형은 모듈 상수).
 *
 * 2D context 를 얻지 못하면 **조용히 빈 칸으로 남는다.** jsdom 도 그 자리이고(시험 환경),
 * 실제 브라우저에서도 컨텍스트 소진 같은 이유로 `null` 이 올 수 있다. 미리보기 하나가
 * 팔레트 전체를 무너뜨리지 않아야 한다.
 */
export function CanvasCellPreview({
  testId,
  element,
}: {
  testId: string;
  element: CanvasElement;
}): React.ReactElement {
  const ref = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const node = ref.current;
    if (node === null) return;
    const ctx = node.getContext('2d') as DrawContext2D | null;
    if (ctx === null) return;
    // `clearSurface` 는 항등 변환(= 장치 px) 기준으로 지운다. 이 칸은 DPR 을 읽지 않으므로
    // 배율이 곧 `PREVIEW_SCALE` 이고, 그 값은 표면이 쓰는 것과 같은 형상으로 넘긴다.
    clearSurface(ctx, { ...PREVIEW_PROJECTION.stage, scale: PREVIEW_SCALE });
    // 스타일도 문구도 규칙이 없다 — 미리보기 칸에는 데이터가 붙지 않으므로 요소 자신의
    // 씨앗 스타일이 그대로 쓰인다(`drawElements` 가 `styles[id] ?? el.style` 로 떨어진다).
    //
    // **글자 폭 장부는 여기서 줄 것이 없다.** `drawElements` 의 뒤 두 인자는 스타일 맵과
    // 문구 맵이고, 장부는 인자가 아니라 **반환값**이다 — 그리는 쪽이 `measureText` 로 직접
    // 재기 때문이다. 그 장부를 받아 쓰는 것은 히트 판정과 윤곽 상자인데, 미리보기 칸에는
    // 둘 다 없다. 그래서 버린다.
    drawElements(ctx, [element], {}, {}, PREVIEW_PROJECTION);
  }, [element]);

  return (
    <canvas
      ref={ref}
      data-testid={testId}
      width={PREVIEW_PROJECTION.stage.width}
      height={PREVIEW_PROJECTION.stage.height}
      style={{ width: PREVIEW.width, height: PREVIEW.height }}
      // 그림은 이름을 나르지 않는다 — 이름은 아래 `<span>` 과 버튼의 `aria-label` 에 있다.
      aria-hidden="true"
    />
  );
}

/**
 * 카탈로그 도형 하나의 미리보기. `CanvasCellPreview` 에 얹는 얇은 어댑터다.
 *
 * `useMemo` 가 하는 일은 하나 — 카탈로그 항목이 얼어 있는 상수이므로 미리보기 요소도 그
 * 항목마다 **한 번만** 지어져, 011 이전과 정확히 같은 횟수로 그려진다.
 */
function CanvasShapePreview({ entry }: { entry: ShapeCatalogEntry }): React.ReactElement {
  const element = useMemo(() => previewElement(entry), [entry]);
  return <CanvasCellPreview testId={`canvas-catalog-preview-${entry.id}`} element={element} />;
}

// --- 접히는 묶음 ---------------------------------------------------------

export interface CanvasPaletteGroupProps {
  id: PaletteGroupId;
  collapsed: boolean;
  onToggle: (id: PaletteGroupId) => void;
  children: ReactNode;
}

/**
 * 접히는 묶음 한 개. 원시형 넷도 카탈로그 셋도 **같은 껍데기**를 쓴다 — 하나만 다른 모양을
 * 하면 사용자가 "이건 왜 접히지 않지" 를 묻게 된다.
 *
 * **접혔을 때 자식을 그리지 않는다**(`hidden` 이 아니다). 그래서 도크가 열리는 순간 만들어
 * 지는 미리보기 `<canvas>` 가 0개이고, 편 묶음의 칸만 생긴다. 그 대가로 `aria-controls` 는
 * 펼쳤을 때만 가리킬 것이 있으므로 그때만 단다 — 없는 id 를 가리키는 `aria-controls` 는
 * 보조기술에게 거짓말이다.
 */
export function CanvasPaletteGroup({
  id,
  collapsed,
  onToggle,
  children,
}: CanvasPaletteGroupProps): React.ReactElement {
  const { t } = useTranslation();
  const bodyId = useId();
  const Chevron = collapsed ? ChevronRight : ChevronDown;
  const title = t(PALETTE_GROUP_TITLE_KEYS[id]);

  return (
    <div className="flex flex-col">
      <button
        type="button"
        data-testid={`canvas-palette-group-${id}`}
        // 보이는 이름이 곧 접근성 이름이고, 눌린 상태는 `aria-expanded` 가 나른다 —
        // 도크의 격자 토글이 `aria-pressed` 로 하는 그 일과 같은 규율이다.
        aria-expanded={!collapsed}
        aria-controls={collapsed ? undefined : bodyId}
        className={GROUP_HEAD_CLASS}
        onClick={() => onToggle(id)}
      >
        <Chevron className={CHEVRON_CLASS} aria-hidden="true" />
        <span>{title}</span>
      </button>
      {!collapsed && (
        <div id={bodyId} data-testid={`canvas-palette-group-body-${id}`}>
          {children}
        </div>
      )}
    </div>
  );
}

// --- 카탈로그 ------------------------------------------------------------

export interface CanvasShapeCatalogProps {
  collapsed: PaletteCollapseState;
  onToggle: (id: PaletteGroupId) => void;
  /** 카탈로그 도형을 놓는다. 만드는 일은 `canvasElementFactory` 한 입구가 한다. */
  onPlace: (entry: ShapeCatalogEntry) => void;
  /**
   * `기본` 묶음 격자의 **첫 칸들**. 도크가 원시형 넷을 여기로 건넨다(SPEC-CANVAS-011 REQ-01).
   *
   * **격자 위가 아니라 격자 안이다.** 이 값은 `<div className={GRID_CLASS}>` 의 자식으로
   * 들어가 카탈로그 칸들 **앞에** 흐르므로, 넘기는 쪽은 감싸는 상자가 아니라 `<button>`
   * 조각을 주어야 한다. 상자로 감싸면 그 상자 하나가 칸 한 개를 차지하고 넷이 그 안에서
   * 다시 쌓인다 — 한 묶음 안에 두 배치가 서는 그 결함이 정확히 그렇게 생긴다.
   *
   * 칸의 겉모습은 `CELL_CLASS` 로, 그림은 `CanvasCellPreview` 로 내보낸다. 넘기는 쪽이 제
   * 클래스도 제 그리기도 적지 않아야 두 생김새가 다시 갈라지지 않는다.
   */
  leading?: ReactNode;
}

/**
 * 카탈로그 묶음 셋. 원시형 넷은 카탈로그가 아니지만 `기본` 묶음 **격자의 첫 네 칸**으로 선다
 * (011 REQ-01) — 도크가 `leading` 으로 건넨 그것이다. 그래서 그 묶음의 몸통은 배치가 하나다.
 */
export function CanvasShapeCatalog({
  collapsed,
  onToggle,
  onPlace,
  leading,
}: CanvasShapeCatalogProps): React.ReactElement {
  const { t } = useTranslation();
  // 치환자가 **두 번** 나오는 문구다. `replace` 는 첫 자리만 바꾸므로 뒤쪽에 `{shape}` 가
  // 벌거벗은 채 남는다 — 이 저장소가 이미 한 번 물린 자리이고(도크의 `gridStepPartial`),
  // 그래서 여기도 `replaceAll` 이다. 키를 그대로 돌려주는 i18n 대체 아래에서는 이 결함이
  // 드러나지 않으므로, 시험은 **진짜 번역 문구**로 잰다(시험 규율 D7).
  const aria = t('dashboard.canvas.edit.paletteShapeAria');

  return (
    <>
      {SHAPE_GROUPS.map((group) => (
        <CanvasPaletteGroup
          key={group.id}
          id={group.id}
          collapsed={collapsed[group.id]}
          onToggle={onToggle}
        >
          <div className={GRID_CLASS}>
            {group.id === 'basic' && leading}
            {group.entries.map((shape) => {
              const name = t(shape.nameKey);
              return (
                <button
                  key={shape.id}
                  type="button"
                  data-testid={`canvas-catalog-add-${shape.id}`}
                  // 접근성 이름이 보이는 라벨을 **포함한다**(WCAG 2.5.3 — 도크가 이미 지키는
                  // 규율). 보이는 것은 이름이고 이 문장은 그 이름으로 무엇을 하는지 말한다.
                  aria-label={aria.replaceAll('{shape}', name)}
                  title={aria.replaceAll('{shape}', name)}
                  className={cn(CELL_CLASS)}
                  onClick={() => onPlace(shape)}
                >
                  <CanvasShapePreview entry={shape} />
                  <span className="w-full truncate text-center">{name}</span>
                </button>
              );
            })}
          </div>
        </CanvasPaletteGroup>
      ))}
    </>
  );
}
