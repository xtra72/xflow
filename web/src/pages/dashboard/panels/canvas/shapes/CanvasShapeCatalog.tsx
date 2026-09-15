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

import { useEffect, useId, useRef, type ReactNode } from 'react';

import { ChevronDown, ChevronRight } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { PathElement } from '../canvasConfig';
import { pathSeedStyle } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import { clearSurface, drawElements, type DrawContext2D } from '../drawElement';
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

/**
 * 칸의 그림 자리 — `CanvasShapePreview` 가 차지하는 것과 **같은 상자**(44×32 CSS px).
 *
 * 도크의 원시형 넷은 캔버스 미리보기가 아니라 lucide 글리프를 놓지만, 이 상자를 지나므로
 * 칸 높이가 카탈로그 칸과 한 픽셀도 다르지 않다. 크기를 `PREVIEW` 에서 **파생**시키는 것이
 * 이 부품의 전부다 — 숫자를 도크에 베껴 적으면 미리보기를 키우는 날 한쪽만 자란다.
 *
 * 그림은 이름을 나르지 않는다(`aria-hidden`) — 미리보기 `<canvas>` 와 같은 규율이다. 이
 * 자리는 **넣는 것이 무엇이든** 장식이므로 상자가 스스로를 감춘다. 다만 008 의 a11y 가드
 * (`canvas008I18n.test.tsx` — "장식은 이름을 나르지 않는다")는 묶음 안의 `svg`·`canvas` 를
 * **직접** 훑으므로, 넣는 그림 자신도 `aria-hidden` 을 들어야 한다. 조상이 감추는 것으로
 * 충분하다고 가드를 느슨하게 하지 않는다 — 배치를 고치는 김에 출시된 가드를 깎는 것이
 * 이 변경이 할 일이 아니다.
 */
export function CanvasCellGlyph({ children }: { children: ReactNode }): React.ReactElement {
  return (
    <span
      className="flex shrink-0 items-center justify-center"
      style={{ width: PREVIEW.width, height: PREVIEW.height }}
      aria-hidden="true"
    >
      {children}
    </span>
  );
}

// --- 미리보기 ------------------------------------------------------------

/**
 * 미리보기 칸의 CSS 크기(px)와 그 안의 도형 상자.
 *
 * 좌표 공간을 CSS px 와 **같은 수**로 두어(`canvas` = 이 크기) 읽는 사람이 두 단위를
 * 환산하지 않게 한다. 도형 상자를 **정사각**으로 두는 것에는 뜻이 있다 — 카탈로그의 명령은
 * 정사각 로컬 격자(0..10000) 위에서 그려졌으므로, 직사각 상자에 넣으면 원이 타원이 되듯
 * 30종 전부가 눌린 채로 보인다.
 */
const PREVIEW = { width: 44, height: 32, side: 26 } as const;

/**
 * 뒷면 배율. 미리보기는 26px 안에 별의 꼭짓점 열을 그리므로 장치 픽셀이 모자란다. DPR 을
 * 읽지 않고 2 로 고정하는 것은 이 칸이 **그림이 아니라 아이콘**이기 때문이다 — 표면
 * (`CanvasSurface`)이 DPR 을 읽는 것과 달리 여기서는 선명도 한 눈금이면 족하고, DPR 을
 * 읽으면 이 파일이 표면의 그 배선을 한 벌 더 갖게 된다.
 */
const PREVIEW_SCALE = 2;

/** 미리보기 좌표계 → 뒷면 px. 도형은 언제나 이 투영을 지난다. */
const PREVIEW_PROJECTION: CanvasProjection = {
  stage: { width: PREVIEW.width * PREVIEW_SCALE, height: PREVIEW.height * PREVIEW_SCALE },
  canvas: { width: PREVIEW.width, height: PREVIEW.height },
};

/** 미리보기 안의 도형 상자 — 가운데 놓인 정사각. */
const PREVIEW_BOX = {
  x: (PREVIEW.width - PREVIEW.side) / 2,
  y: (PREVIEW.height - PREVIEW.side) / 2,
  w: PREVIEW.side,
  h: PREVIEW.side,
} as const;

/** 미리보기가 그릴 요소. **놓았을 때와 같은 씨앗 스타일**을 입는다. */
function previewElement(entry: ShapeCatalogEntry): PathElement {
  return {
    id: entry.id,
    kind: 'path',
    geometry: { ...PREVIEW_BOX },
    path: entry.path.map((cmd) => ({ ...cmd })),
    catalog_id: entry.id,
    style: pathSeedStyle(entry.path),
  };
}

/**
 * 도형 하나의 미리보기.
 *
 * 2D context 를 얻지 못하면 **조용히 빈 칸으로 남는다.** jsdom 도 그 자리이고(시험 환경),
 * 실제 브라우저에서도 컨텍스트 소진 같은 이유로 `null` 이 올 수 있다. 미리보기 하나가
 * 팔레트 전체를 무너뜨리지 않아야 한다.
 */
function CanvasShapePreview({ entry }: { entry: ShapeCatalogEntry }): React.ReactElement {
  const ref = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const node = ref.current;
    if (node === null) return;
    const ctx = node.getContext('2d') as DrawContext2D | null;
    if (ctx === null) return;
    // `clearSurface` 는 항등 변환(= 장치 px) 기준으로 지운다. 이 칸은 DPR 을 읽지 않으므로
    // 배율이 곧 `PREVIEW_SCALE` 이고, 그 값은 표면이 쓰는 것과 같은 형상으로 넘긴다.
    clearSurface(ctx, { ...PREVIEW_PROJECTION.stage, scale: PREVIEW_SCALE });
    // 스타일도 문구도 규칙이 없다 — 카탈로그 칸에는 데이터가 붙지 않으므로 요소 자신의
    // 씨앗 스타일이 그대로 쓰인다(`drawElements` 가 `styles[id] ?? el.style` 로 떨어진다).
    drawElements(ctx, [previewElement(entry)], {}, {}, PREVIEW_PROJECTION);
  }, [entry]);

  return (
    <canvas
      ref={ref}
      data-testid={`canvas-catalog-preview-${entry.id}`}
      width={PREVIEW_PROJECTION.stage.width}
      height={PREVIEW_PROJECTION.stage.height}
      style={{ width: PREVIEW.width, height: PREVIEW.height }}
      // 그림은 이름을 나르지 않는다 — 이름은 아래 `<span>` 과 버튼의 `aria-label` 에 있다.
      aria-hidden="true"
    />
  );
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
   * 칸의 겉모습은 `CELL_CLASS` 와 `CanvasCellGlyph` 로 내보낸다. 넘기는 쪽이 제 클래스를
   * 적지 않아야 두 생김새가 다시 갈라지지 않는다.
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
