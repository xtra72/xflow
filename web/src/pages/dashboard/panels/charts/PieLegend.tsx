// 파이 범례 — 실제 패널과 설정 미리보기가 공유하는 단일 구현.
//
// 파이 **위에 겹쳐** 뜬다(절대 배치). 종전처럼 flex 로 자리를 나눠 가지면 범례 위치를
// 바꿀 때마다 남는 폭이 달라져 파이가 따라 움직였다 — 파이 자리는 패널이 정하고 범례는
// 그 위에 올라가야 둘이 서로를 밀지 않는다. 겹치는 자리에서는 범례가 보인다.
//
// recharts 내장 `<Legend>` 를 쓰지 않는 이유는 세 가지다.
//   1. 이름·비율·값을 **칸 맞춰** 세로로 쌓아야 한다(표처럼). 내장 범례는 항목을
//      한 덩어리 텍스트로 그려서 값의 자리가 이름 길이에 따라 들쭉날쭉해진다.
//   2. 끌어서 옮긴 오프셋을 얹어야 하는데, 내장 범례는 차트 영역을 자기 몫만큼
//      줄여 잡아 오프셋과 레이아웃이 서로를 밀어낸다.
//   3. 그래프 차트가 이미 같은 이유로 `ChartLegend` 를 밖으로 뺐다 — 두 패널이
//      다른 방식으로 범례를 그리면 같은 설정이 다르게 보인다.

import { cn } from '@/lib/utils/cn';

import { legendTransform } from './pieDrag';
import { resolveFontFamily } from './textStyle';
import { formatPiePercent, pieLegendValueText } from './pieLabel';
import type { ChartFontFamily } from './textStyle';
import type { PieLegendPosition } from './chartChannelTypes';

export interface PieLegendItem {
  name: string;
  value: number;
  /** 전체 대비 비중(0~1). */
  percent: number;
  color: string;
}

export function PieLegend({
  items,
  position,
  showPercentage,
  showValue,
  fontSize,
  fontFamily,
  fontColor,
  decimals,
  unit,
  offsetX,
  offsetY,
}: {
  items: readonly PieLegendItem[];
  position: PieLegendPosition;
  showPercentage: boolean;
  showValue: boolean;
  fontSize: number;
  /** 글꼴 토큰. 미지정은 상속. */
  fontFamily?: ChartFontFamily;
  /** 글자색. 미지정은 테마 글자색(`--color-text-primary`). */
  fontColor?: string;
  decimals: number;
  unit?: string;
  offsetX: number;
  offsetY: number;
}): React.ReactElement | null {
  if (items.length === 0) return null;

  const isVert = position === 'left' || position === 'right';
  const transform = legendTransform(position, offsetX, offsetY);

  // 세로 나열에서만 칸을 맞춘다(표처럼). 가로 나열은 항목이 줄바꿈으로 흘러가므로
  // 열을 맞출 기준선 자체가 없다 — 억지로 맞추면 한 줄에 한 항목만 남는다.
  const columns = ['auto', showPercentage ? 'auto' : null, showValue ? 'auto' : null]
    .filter(Boolean)
    .join(' ');

  return (
    <div
      data-testid="pie-chart-legend"
      data-pie-legend=""
      data-position={position}
      data-aligned={isVert ? 'true' : 'false'}
      className={cn(
        // 파이 위에 뜬다. 겹치는 자리에서 글자를 읽으려면 조각 색이 비쳐서는 안 되므로
        // 옅은 패널색 판을 깔아 준다 — 판이 없으면 "위에 있다" 가 "읽을 수 있다" 로
        // 이어지지 않는다.
        'absolute z-10 max-h-full max-w-full overflow-auto rounded-md bg-(--color-bg-surface)/80 px-2 py-1',
        fontColor ? undefined : 'text-(--color-text-primary)',
        isVert
          // `content-center` 가 없으면 grid 행이 늘어나 항목 사이가 벌어진다.
          // 행은 제 높이만 차지하고, 덩어리째 세로 가운데에 놓는다.
          ? 'grid content-center items-center gap-x-3 gap-y-1'
          : 'flex flex-wrap items-center justify-center gap-x-4 gap-y-1',
        // 기준 자리. 되물림(가운데 정렬)은 transform 이 맡는다.
        position === 'bottom' ? 'bottom-0 left-1/2' : 'top-1/2',
        position === 'left' ? 'left-0' : position === 'right' ? 'right-0' : undefined,
      )}
      style={{
        fontSize: `${fontSize}px`,
        fontFamily: resolveFontFamily(fontFamily),
        // 색을 지정하면 이름·비율·값이 함께 따라간다 — 한 범례 안에서 색이 갈리면
        // 어느 칸이 강조인지 읽히지 않는다(비율·값의 흐린 톤은 불투명도로 남긴다).
        color: fontColor,
        transform,
        ...(isVert ? { gridTemplateColumns: columns } : null),
      }}
    >
      {items.map((item) => {
        const percentText = showPercentage ? formatPiePercent(item.percent) : undefined;
        const valueText = showValue
          ? pieLegendValueText(item.value, { decimals, unit })
          : undefined;

        // 세로: 셀을 grid 의 직계 자식으로 흘려보내야 열이 맞는다. 가로: 항목 하나를
        // 한 덩어리로 묶어야 줄바꿈이 항목 경계에서 일어난다.
        const cells = [
          <span
            key="name"
            className="inline-flex min-w-0 items-center gap-1.5"
            data-testid="pie-legend-name"
          >
            <span
              className="inline-block h-2 w-2 shrink-0 rounded-full"
              style={{ backgroundColor: item.color }}
            />
            <span className="truncate">{item.name}</span>
          </span>,
          percentText !== undefined ? (
            <span
              key="percent"
              className={cn(
                'tabular-nums',
                fontColor ? 'opacity-70' : 'text-(--color-text-muted)',
                isVert && 'text-right',
              )}
              data-testid="pie-legend-percent"
            >
              {percentText}
            </span>
          ) : null,
          valueText !== undefined ? (
            <span
              key="value"
              className={cn(
                'tabular-nums',
                fontColor ? 'opacity-70' : 'text-(--color-text-muted)',
                isVert && 'text-right',
              )}
              data-testid="pie-legend-value"
            >
              {valueText}
            </span>
          ) : null,
        ].filter(Boolean);

        if (isVert) {
          // Fragment 로 감싸면 셀이 grid 직계 자식으로 남는다(칸 맞춤 유지).
          return (
            <div key={item.name} className="contents" data-testid="pie-legend-item" data-name={item.name}>
              {cells}
            </div>
          );
        }
        return (
          <span
            key={item.name}
            className="inline-flex items-center gap-1.5 whitespace-nowrap"
            data-testid="pie-legend-item"
            data-name={item.name}
          >
            {cells}
          </span>
        );
      })}
    </div>
  );
}
