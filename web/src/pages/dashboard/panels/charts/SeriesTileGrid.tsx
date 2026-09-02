// 다중 출력 공용 그리드 (SPEC-CHART-002 §2.4 [U4]).
//
// `series_reduce` 가 지정된 Store 모드에서 시리즈당 출력 1개를 반응형 그리드로 배치한다.
// stat 타일과 gauge(M4) 게이지가 같은 배치 규칙을 공유하도록 항목 형상에 대해 제네릭으로
// 두고, 실제 타일 내용은 호출자의 `renderItem` 이 그린다 — 그리드는 "무엇을 그리는가" 를
// 모르고 "어떻게 배치하고 어디서 자르는가" 만 안다.
//
// 상한(`multi_output_limit`)과 기본값(`DEFAULT_MULTI_OUTPUT_LIMIT`)은 multiOutputLimit.ts 가 소유한다
// (spec.md §2.4 — M1 이 아니라 M3.1 소유). bar/pie 는 그리드를 쓰지 않지만 같은 상한을
// 적용해야 하므로 `applyMultiOutputLimit` / `MultiOutputTruncationNotice` 를 공유한다.

import React from 'react';

import { useTranslation } from '@/lib/i18n';

import {
  MIN_TILE_WIDTH_PX,
  TILE_GAP_PX,
  applyMultiOutputLimit,
  normalizeTileRows,
  tileColumnCount,
} from './multiOutputLimit';


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
  /**
   * 행 높이를 그리드 높이에서 **균등 분배**할지(기본: 내용 높이).
   *
   * 기본값(false)에서 행은 내용 높이로 결정된다. 내용이 자기 높이를 스스로 아는 타일
   * (stat 처럼 텍스트로 채워지는 경우)에는 그것이 맞다.
   *
   * 게이지처럼 **내용이 컨테이너 높이를 받아야 하는** 타일에는 맞지 않는다. 그런 타일은
   * 높이를 알기 위해 행을 봐야 하는데 행은 높이를 알기 위해 타일을 봐야 하므로 순환이
   * 생기고, CSS 는 그 순환에서 백분율 높이 제약(`max-height: 100%`)을 `none` 처럼 다룬다.
   * 그러면 타일이 폭 기준으로 커져 그리드 밖으로 넘치고, 부모의 `overflow-hidden` 에
   * 잘린다 — 폭은 맞는데 위아래가 잘리는 게이지가 그 결과다.
   *
   * `true` 면 `grid-auto-rows: minmax(0, 1fr)` 로 행 높이를 **먼저** 확정해 순환을 끊는다.
   * 그러면 타일이 확정된 높이를 물려받아 두 축 모두에 맞출 수 있다.
   */
  fillRows?: boolean;
  /**
   * 타일 배열의 **목표 행 수**(`tile_rows`). 미지정이면 `DEFAULT_TILE_ROWS`(1) — 한 줄.
   *
   * 열 수는 `ceil(N / rows)` 로 파생된다. 상한이 아니라 목표인 이유는 `tileColumnCount`
   * 주석 참고 — 폭이 좁으면 `auto-fit` 이 열을 줄이고 행이 목표보다 늘어난다.
   */
  rows?: number;
}

/**
 * 다중 출력 반응형 그리드.
 *
 * 열 수는 목표 행 수에서 파생한 `ceil(N / rows)` 를 상한으로 하되, 각 트랙의 최소 폭을
 * `max(MIN_TILE_WIDTH_PX, "N열일 때의 이상적 폭")` 으로 잡고 `auto-fit` 에 맡긴다.
 * 결과적으로 폭이 좁으면 열이 자동으로 줄어든다(§2.4 "타일 최소 폭이 확보되지 않으면
 * 열 수를 줄인다"). N=1 이면 1열 1행이며 기존 단일 출력과 시각적으로 같다.
 */
export function SeriesTileGrid<T>({
  items,
  limit,
  renderItem,
  itemKey,
  fillRows = false,
  rows,
}: SeriesTileGridProps<T>): React.ReactElement | null {
  const { visible, truncated } = applyMultiOutputLimit(items, limit);
  if (visible.length === 0) return null;

  const columns = tileColumnCount(visible.length, rows);
  const idealWidth = `calc((100% - ${(columns - 1) * TILE_GAP_PX}px) / ${columns})`;
  const gridTemplateColumns = `repeat(auto-fit, minmax(max(${MIN_TILE_WIDTH_PX}px, ${idealWidth}), 1fr))`;

  return (
    <>
      <div
        data-testid="series-tile-grid"
        data-columns={String(columns)}
        // 실제 행 수는 폭에 따라 CSS 가 정한다(레이아웃 없는 환경에서는 알 수 없다).
        // 여기 싣는 것은 **목표** 행 수다.
        data-rows={String(normalizeTileRows(rows))}
        className="grid min-h-0 flex-1 content-center"
        style={{
          gridTemplateColumns,
          gap: `${TILE_GAP_PX}px`,
          // 기본 경로에서는 키 자체를 싣지 않는다 — 행 높이 결정 방식이 바뀌지 않아야 한다.
          ...(fillRows ? { gridAutoRows: 'minmax(0, 1fr)' } : {}),
        }}
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
