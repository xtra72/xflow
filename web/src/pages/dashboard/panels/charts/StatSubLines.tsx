// 통계 패널의 보조 줄 2종 — 변화량과 구간 통계.
//
// 두 컴포넌트는 config 를 받지 않고 **이미 해석된 값**(색 3종 · 배율 · 항목 배열)만
// 받는다. 해석은 `statDisplayOptions` 가, 값 계산은 `seriesReduce` 가 소유한다.
// 이렇게 나누면 레거시 경로와 타일 경로가 같은 컴포넌트를 `compact` 만 바꿔 쓴다.
//
// @spec SPEC-CHART-003 §4 (렌더 형태)

import { useTranslation } from '@/lib/i18n';

import type { DeltaColors, WindowStatKind } from './statDisplayOptions';

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
  scale,
  testId,
}: {
  arrow: DeltaArrow;
  /** 이미 단위·자릿수 규칙이 적용된 표기. 부호를 포함한다. */
  text: string;
  colors: DeltaColors;
  scale: number;
  testId: string;
}): React.ReactElement {
  const color = arrow === '↑' ? colors.up : arrow === '↓' ? colors.down : colors.flat;
  return (
    <div
      data-testid={testId}
      className="mt-1 tabular-nums"
      style={{ color, fontSize: SUB_LINE_PX * scale }}
    >
      {arrow} {text}
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
  scale,
  testId,
}: {
  items: readonly WindowStatItem[];
  /** 타일 경로처럼 폭이 좁은 자리에서 라벨을 축약한다. 개수가 아니라 경로가 정한다. */
  compact: boolean;
  scale: number;
  testId: string;
}): React.ReactElement | null {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return (
    <div
      data-testid={testId}
      className="mt-1 flex flex-wrap items-baseline justify-center gap-x-2 gap-y-0.5 text-(--color-text-muted) tabular-nums"
      style={{ fontSize: SUB_LINE_PX * scale }}
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
    </div>
  );
}
