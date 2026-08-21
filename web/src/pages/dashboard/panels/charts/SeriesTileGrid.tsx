// 다중 출력 공용 그리드 (SPEC-CHART-002 §2.4 [U4]).
//
// `series_reduce` 가 지정된 Store 모드에서 시리즈당 출력 1개를 반응형 그리드로 배치한다.
// stat 타일과 gauge(M4) 게이지가 같은 배치 규칙을 공유하도록 항목 형상에 대해 제네릭으로
// 두고, 실제 타일 내용은 호출자의 `renderItem` 이 그린다 — 그리드는 "무엇을 그리는가" 를
// 모르고 "어떻게 배치하고 어디서 자르는가" 만 안다.
//
// 상한(`multi_output_limit`)과 기본값(`DEFAULT_MULTI_OUTPUT_LIMIT`)을 이 모듈이 소유한다
// (spec.md §2.4 — M1 이 아니라 M3.1 소유). bar/pie 는 그리드를 쓰지 않지만 같은 상한을
// 적용해야 하므로 `applyMultiOutputLimit` / `MultiOutputTruncationNotice` 를 공유한다.

import React from 'react';

import { useTranslation } from '@/lib/i18n';

/**
 * 다중 출력 표시 개수 기본 상한. @spec SPEC-CHART-002 §2.4 / §4.6 / OQ1
 *
 * 시리즈 **선택** 상한(`STORE_SERIES_LIMIT` = 48)과 다른 축이다 — 선택 상한은 조회 부하를,
 * 이 상한은 가독성을 보호한다. 48개를 조회하되 12개만 그리는 상태는 정상이다.
 */
export const DEFAULT_MULTI_OUTPUT_LIMIT = 12;

/** 타일 최소 폭(px). 이 폭이 확보되지 않으면 열 수가 자동으로 줄어든다(§2.4). */
export const MIN_TILE_WIDTH_PX = 120;

/** 타일 간격(px). 열 폭 계산에 들어가므로 상수로 고정한다. */
export const TILE_GAP_PX = 8;

/**
 * 항목 수에 대한 **열 수 상한**을 돌려준다 — `ceil(sqrt(N))`.
 *
 * 실제 열 수는 `min(ceil(sqrt(N)), 패널 폭이 허용하는 최대 열)` 이며(§2.4), 뒤쪽 항은
 * CSS `auto-fit` + `minmax` 가 렌더 시점에 결정한다. 여기서는 앞쪽 항만 계산한다 —
 * jsdom 에는 레이아웃이 없으므로 폭 기반 판정을 JS 로 흉내 내면 테스트가 거짓말을 한다.
 */
export function autoColumnCount(count: number): number {
  if (!Number.isFinite(count) || count <= 1) return 1;
  return Math.ceil(Math.sqrt(count));
}

/** 상한 적용 결과. `truncated` 는 잘려 나간 개수(0 이면 잘림 없음). */
export interface MultiOutputSlice<T> {
  visible: T[];
  truncated: number;
}

/**
 * 표시 상한을 적용한다. 잘림은 **순서상 뒤에서부터**이며 정렬을 바꾸지 않는다(UB1-11).
 *
 * `limit` 이 미지정이거나 정상 범위를 벗어나면(0 이하 / 비유한) 기본 상한으로 되돌린다 —
 * `multi_output_limit: 0` 을 "아무것도 그리지 않는다" 로 해석하면 사용자가 패널을 실수로
 * 비울 수 있고, 그 상태는 "값이 없다" 와 화면에서 구분되지 않는다.
 */
export function applyMultiOutputLimit<T>(
  items: readonly T[],
  limit?: number,
): MultiOutputSlice<T> {
  const raw = limit === undefined ? Number.NaN : Math.trunc(limit);
  const max = Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_MULTI_OUTPUT_LIMIT;
  if (items.length <= max) return { visible: [...items], truncated: 0 };
  return { visible: items.slice(0, max), truncated: items.length - max };
}

/**
 * 잘림 표기(`+K`). 개수를 **접근 가능한 텍스트로** 전달한다(§5 접근성).
 *
 * 화면에는 `+K` 만 두고(공간이 좁다), 스크린리더에는 개수를 보간한 완결 문장을 준다.
 * `+8` 만으로는 무엇이 8개인지 알 수 없으므로 시각 표기를 `aria-hidden` 으로 감추고
 * 문장 쪽에 개수를 실어 중복 낭독을 피한다.
 */
export function MultiOutputTruncationNotice({
  truncated,
}: {
  truncated: number;
}): React.ReactElement | null {
  const { t } = useTranslation();
  if (truncated <= 0) return null;
  return (
    <p
      data-testid="series-tile-truncation"
      className="mt-2 shrink-0 text-center text-xs text-(--color-text-muted)"
    >
      <span aria-hidden="true" className="font-medium text-(--color-text-secondary)">
        {`+${truncated}`}
      </span>
      <span className="sr-only">
        {t('dashboard.chart.multiOutputTruncated').replace('{count}', String(truncated))}
      </span>
    </p>
  );
}

export interface SeriesTileGridProps<T> {
  /** 출력 항목(시리즈 순서 그대로). 상한 적용은 이 컴포넌트가 수행한다. */
  items: readonly T[];
  /** `multi_output_limit`. 미지정이면 `DEFAULT_MULTI_OUTPUT_LIMIT`. */
  limit?: number;
  /** 타일 1개의 내용. 배치/잘림은 그리드가 소유하고 내용은 호출자가 소유한다. */
  renderItem: (item: T, index: number) => React.ReactNode;
  /** 타일 key. 미지정이면 인덱스를 쓴다(표시 이름은 중복될 수 있으므로 기본값이 아니다). */
  itemKey?: (item: T, index: number) => string;
}

/**
 * 다중 출력 반응형 그리드.
 *
 * 열 수는 `ceil(sqrt(N))` 을 상한으로 하되, 각 트랙의 최소 폭을
 * `max(MIN_TILE_WIDTH_PX, "N열일 때의 이상적 폭")` 으로 잡고 `auto-fit` 에 맡긴다.
 * 결과적으로 폭이 좁으면 열이 자동으로 줄어든다(§2.4 "타일 최소 폭이 확보되지 않으면
 * 열 수를 줄인다"). N=1 이면 1열 1행이며 기존 단일 출력과 시각적으로 같다.
 */
export function SeriesTileGrid<T>({
  items,
  limit,
  renderItem,
  itemKey,
}: SeriesTileGridProps<T>): React.ReactElement | null {
  const { visible, truncated } = applyMultiOutputLimit(items, limit);
  if (visible.length === 0) return null;

  const columns = autoColumnCount(visible.length);
  const idealWidth = `calc((100% - ${(columns - 1) * TILE_GAP_PX}px) / ${columns})`;
  const gridTemplateColumns = `repeat(auto-fit, minmax(max(${MIN_TILE_WIDTH_PX}px, ${idealWidth}), 1fr))`;

  return (
    <>
      <div
        data-testid="series-tile-grid"
        data-columns={String(columns)}
        className="grid min-h-0 flex-1 content-center"
        style={{ gridTemplateColumns, gap: `${TILE_GAP_PX}px` }}
      >
        {visible.map((item, i) => (
          <div
            key={itemKey ? itemKey(item, i) : i}
            data-testid="series-tile"
            className="flex min-w-0 flex-col items-center justify-center"
          >
            {renderItem(item, i)}
          </div>
        ))}
      </div>
      <MultiOutputTruncationNotice truncated={truncated} />
    </>
  );
}
