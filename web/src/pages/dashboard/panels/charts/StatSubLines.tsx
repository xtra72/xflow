// 통계 패널의 보조 줄 2종 — 변화량과 구간 통계.
//
// 두 컴포넌트는 config 를 받지 않고 **이미 해석된 값**(색 3종 · 배율 · 항목 배열)만
// 받는다. 해석은 `statDisplayOptions` 가, 값 계산은 `seriesReduce` 가 소유한다.
// 이렇게 나누면 레거시 경로와 타일 경로가 같은 컴포넌트를 `compact` 만 바꿔 쓴다.
//
// @spec SPEC-CHART-003 §4 (렌더 형태)

import { clsx } from 'clsx';

import { useTranslation } from '@/lib/i18n';

import { PANEL_EDIT_OUTLINE_CLASS, PANEL_SELECTED_OUTLINE_CLASS } from '../../PanelDragLayer';
import type { DeltaColors, WindowStatKind } from './statDisplayOptions';
import type { StatElementKind } from './statLayout';

/**
 * 배치 편집이 켜졌을 때만 붙는 부가 속성 (SPEC-CHART-004).
 *
 * 컴포넌트가 드래그 레이어를 알 필요는 없다 — 표식과 오프셋만 받아 자기 상자에 붙이고,
 * 손잡이는 호출자가 넘긴 노드를 그대로 그린다. 편집이 꺼져 있으면 `undefined` 이며
 * 그때의 DOM 은 종전과 같다.
 */
export interface StatSubLineEdit {
  kind: StatElementKind;
  /** 지금 고른 요소인가 — 윤곽의 모양이 갈린다. */
  selected?: boolean;
  /** 크기 손잡이 등 상자 안에 겹쳐 그릴 것. */
  overlay?: React.ReactNode;
  onDoubleClick?: (e: React.MouseEvent<HTMLDivElement>) => void;
  onKeyDown?: (e: React.KeyboardEvent<HTMLDivElement>) => void;
}

/** 선택 여부에 따른 윤곽 클래스. 편집이 꺼져 있으면 아무것도 없다. */
function editOutline(edit: StatSubLineEdit | undefined): string | false {
  if (!edit) return false;
  return edit.selected ? PANEL_SELECTED_OUTLINE_CLASS : PANEL_EDIT_OUTLINE_CLASS;
}

/** 편집 표식을 상자 속성으로 편다. 편집이 꺼져 있으면 빈 객체다. */
function editProps(edit: StatSubLineEdit | undefined): Record<string, unknown> {
  if (!edit) return {};
  return {
    'data-panel-drag': edit.kind,
    tabIndex: 0,
    onDoubleClick: edit.onDoubleClick,
    onKeyDown: edit.onKeyDown,
  };
}

/**
 * 저장된 오프셋(패널 상자 대비 백분율)을 CSS 로 편다.
 *
 * `transform: translate(X%)` 을 쓰지 않는다 — 그 백분율은 **요소 자신의 크기** 기준이라,
 * 패널 상자 기준으로 계산한 값을 넣으면 요소가 작을수록 느리게 움직인다(요소가 패널
 * 폭의 절반이면 정확히 절반 속도다). 실제로 "마우스 이동의 절반 속도로 움직인다" 로
 * 보고됐다.
 *
 * 대신 상대 위치의 `left` / `top` 을 쓴다. 이 둘의 백분율은 **담는 상자**(기준 상자)의
 * 폭·높이 기준이라 저장 값의 뜻과 정확히 맞고, 형제 요소의 흐름 배치도 건드리지 않는다.
 *
 * **편집 여부와 무관하게** 적용한다. 저장된 자리는 읽기 전용 대시보드에서도 그대로
 * 그려져야 한다 — 편집할 때만 옮겨 보이면 맞춘 배치가 저장되지 않은 것처럼 읽힌다.
 */
export function statOffsetStyle(
  offsetX: number,
  offsetY: number,
): { position: 'relative'; left: string; top: string } | undefined {
  if (!offsetX && !offsetY) return undefined;
  return { position: 'relative', left: `${offsetX}%`, top: `${offsetY}%` };
}

/**
 * 보조 줄의 기본 글자 크기(px). 종전 `text-sm` 과 같은 14px 이며, 배율 1 이
 * 종전 외형과 일치해야 한다(§2.4 U4-4).
 *
 * 클래스가 아니라 수로 두는 이유는 배율을 곱해야 하기 때문이다 — 본값이
 * `STAT_VALUE_PX` 를 같은 이유로 상수화한 것과 같다.
 */
export const SUB_LINE_PX = 14;

/** 변화량 방향 글리프. 색만으로 방향을 알리지 않기 위한 시각·텍스트 이중 신호다. */
export type DeltaArrow = '↑' | '↓' | '→';

/**
 * 변화량 한 줄 — 화살표 + 부호 있는 값.
 *
 * 색은 방향이 정하고, 방향은 화살표와 부호가 **함께** 전달한다(§4.3). 사용자가
 * 증가·감소에 같은 색을 지정해도 방향을 읽을 수 있어야 한다.
 */
export function StatDeltaLine({
  arrow,
  text,
  colors,
  fontSize,
  offsetX = 0,
  offsetY = 0,
  testId,
  edit,
}: {
  arrow: DeltaArrow;
  /** 이미 단위·자릿수 규칙이 적용된 표기. 부호를 포함한다. */
  text: string;
  colors: DeltaColors;
  /** 최종 글자 크기(px). 폴백까지 끝낸 값을 받는다 — 해석은 `statLayout` 이 소유한다. */
  fontSize: number;
  /** 배치 오프셋(백분율 포인트). 편집 여부와 무관하게 적용된다. */
  offsetX?: number;
  offsetY?: number;
  testId: string;
  edit?: StatSubLineEdit;
}): React.ReactElement {
  const color = arrow === '↑' ? colors.up : arrow === '↓' ? colors.down : colors.flat;
  return (
    <div
      data-testid={testId}
      className={clsx('mt-1 tabular-nums', edit && ['relative', editOutline(edit)])}
      style={{ color, fontSize, ...statOffsetStyle(offsetX, offsetY) }}
      {...editProps(edit)}
    >
      {arrow} {text}
      {edit?.overlay}
    </div>
  );
}

/** 구간 통계 항목 1개. `text` 가 `null` 이면 표본이 없어 값을 낼 수 없다는 뜻이다. */
export interface WindowStatItem {
  kind: WindowStatKind;
  /** 이미 단위·자릿수 규칙이 적용된 표기. 값 없음은 `null`. */
  text: string | null;
}

/**
 * 구간 통계 한 줄 — 켠 항목을 인라인으로 늘어놓는다.
 *
 * 항목 순서는 호출자가 정한 그대로 쓴다(`readWindowStats` 가 고정 순서를 소유한다).
 * 값이 없는 항목도 자리를 지킨다 — 생략하면 켜 둔 항목의 자리가 데이터에 따라
 * 움직여 눈이 값을 쫓지 못한다.
 */
export function StatWindowStatsLine({
  items,
  compact,
  fontSize,
  offsetX = 0,
  offsetY = 0,
  testId,
  edit,
}: {
  items: readonly WindowStatItem[];
  /** 타일 경로처럼 폭이 좁은 자리에서 라벨을 축약한다. 개수가 아니라 경로가 정한다. */
  compact: boolean;
  /** 최종 글자 크기(px). */
  fontSize: number;
  /** 배치 오프셋(백분율 포인트). 편집 여부와 무관하게 적용된다. */
  offsetX?: number;
  offsetY?: number;
  testId: string;
  edit?: StatSubLineEdit;
}): React.ReactElement | null {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return (
    <div
      data-testid={testId}
      className={clsx(
        'mt-1 flex flex-wrap items-baseline justify-center gap-x-2 gap-y-0.5 text-(--color-text-muted) tabular-nums',
        edit && ['relative', editOutline(edit)],
      )}
      style={{ fontSize, ...statOffsetStyle(offsetX, offsetY) }}
      {...editProps(edit)}
    >
      {items.map((item) => {
        const full = t(`dashboard.chart.windowStat.${item.kind}`);
        const short = t(`dashboard.chart.windowStatShort.${item.kind}`);
        return (
          <span key={item.kind} data-stat-kind={item.kind} className="whitespace-nowrap">
            {compact ? (
              <>
                {/* 화면에는 축약 라벨, 낭독에는 완결 낱말 — 중복 낭독을 피한다. */}
                <span aria-hidden="true">{short}</span>
                <span className="sr-only">{full}</span>
              </>
            ) : (
              full
            )}{' '}
            <span className="font-medium text-(--color-text-secondary)">{item.text ?? '—'}</span>
          </span>
        );
      })}
      {edit?.overlay}
    </div>
  );
}
