// 라인 차트 범례 — 실제 패널과 설정 미리보기가 공유하는 단일 구현.
//
// 분리 이유: 미리보기가 recharts 내장 <Legend> 를 쓰던 시절, 범례 옵션(이름/선/마지막 값
// 표시)과 차트-범례 구분선·여백이 실제 패널과 달라 "설정한 옵션이 미리보기에 안 먹는다" 는
// 결함이 반복됐다. 두 화면이 같은 컴포넌트를 쓰면 그 차이가 구조적으로 사라진다.
//
//
// 자리는 **흐름이 정한다**(차트의 형제). 파이 범례처럼 겹쳐 띄우지 않는 이유는, 이 범례가
// 구분선과 마지막 값 칸을 가진 표에 가깝고 이미 차트와 자리를 나눠 쓰기 때문이다 — 절대
// 배치로 바꾸면 저장된 모든 대시보드에서 차트가 커지고 범례가 그림 위를 덮는다. 끌어 옮긴
// 값은 그 자리에서의 **상대 변위**로만 얹는다(`inlineLegendOffsetStyle`).
//
// 그래서 상자가 두 겹이고, 두 겹이 **서로 다른 일**을 한다.
//   - 바깥(`line-chart-legend`): 자리·구분선 + **변위를 얹는 자리**.
//   - 안쪽(`line-chart-legend-items`): 실제로 보이는 항목들 + **끌 수 있는 범위를 재는 자리**.
//
// 변위가 바깥에 붙는 이유는 CSS 퍼센트의 기준 때문이다. `position: relative` 의 `left`·`top`
// 퍼센트는 **담는 블록**(부모)의 폭·높이를 기준으로 푼다. 변위를 안쪽에 붙이면 그 기준이
// 바깥 상자가 되는데, 좌·우 배치에서 바깥 상자는 세로로만 본문만큼 늘어나고 가로로는
// 내용 폭(수십 px)뿐이다 — 그래서 `top` 은 맞게 움직이는데 `left` 는 같은 10% 가 본문의
// 10%가 아니라 범례 폭의 10%가 되어 손이 움직인 것보다 훨씬 조금 움직였다. 바깥 상자의
// 담는 블록은 본문이므로 두 축의 기준이 같아진다.
//
// 재는 자리가 안쪽인 이유는 반대다. 바깥 상자는 flex 형제라 교차축으로 **늘어나므로**, 그것을
// 재서 죄면 늘어난 축의 남은 여백이 0이 되어 그 방향으로는 한 픽셀도 못 움직인다. 안쪽 상자는
// 내용만큼만 커서 남은 여백이 실제 여백이다.
//
// 옮긴 뒤에는 구분선을 지운다. 항목이 떠난 자리에 선만 남으면 무엇을 가르는 선인지 알 수
// 없다 — 자리(폭·높이)는 흐름이 정한 그대로이므로 차트 크기는 변하지 않는다.

import { useMemo } from 'react';

import { cn } from '@/lib/utils/cn';

import { clampStoredLegendOffset, inlineLegendOffsetStyle } from './legendOverlay';
import { resolveFontColor, resolveFontFamily, resolveFontSize } from './textStyle';
import { DEFAULT_CHART_LEGEND_FONT_SIZE, type LegendConfig } from './chartChannelTypes';

export function ChartLegend({
  seriesKeys,
  seriesColors,
  legendCfg,
  chartData,
  formatValue,
}: {
  seriesKeys: string[];
  seriesColors: string[];
  legendCfg: LegendConfig;
  chartData: Array<Record<string, unknown>>;
  /** 시리즈 마지막값 표시 포맷터. enum/boolean 은 라벨로, 그 외는 숫자로 표기한다. */
  formatValue: (key: string, value: number) => string;
}): React.ReactElement | null {
  const isVert = legendCfg.position === 'left' || legendCfg.position === 'right';
  const showName = legendCfg.show_name !== false;
  const showLine = legendCfg.show_line !== false;
  const showLastValue = legendCfg.show_last_value === true;
  const fontSize = resolveFontSize(legendCfg.font_size) ?? DEFAULT_CHART_LEGEND_FONT_SIZE;
  const fontFamily = resolveFontFamily(legendCfg.font_family);
  const fontColor = resolveFontColor(legendCfg.font_color);
  // 저장된 변위는 성긴 상한으로만 죈다 — 손으로 고쳐지거나 패널 크기가 바뀐 뒤에도
  // 범례가 통째로 사라지지 않게. 정확한 죄기는 끌 때 드래그 레이어가 한다.
  const offsetX = clampStoredLegendOffset(legendCfg.offset_x);
  const offsetY = clampStoredLegendOffset(legendCfg.offset_y);

  // 각 시리즈별 마지막 유효 값 (역순 탐색)
  // Hooks 규칙 준수: 조건부 early-return 보다 먼저 호출한다.
  const lastValues = useMemo(() => {
    if (!showLastValue || chartData.length === 0) return {};
    const result: Record<string, number | undefined> = {};
    for (const key of seriesKeys) {
      for (let i = chartData.length - 1; i >= 0; i--) {
        const v = chartData[i]![key as keyof (typeof chartData)[0]];
        if (typeof v === 'number' && Number.isFinite(v)) {
          result[key] = v;
          break;
        }
      }
    }
    return result;
  }, [showLastValue, chartData, seriesKeys]);

  if (seriesKeys.length === 0) return null;

  const moved = offsetX !== 0 || offsetY !== 0;

  return (
    <div
      className={cn(
        'flex shrink-0',
        isVert ? 'min-w-fit flex-col justify-center py-2 pl-3 pr-2' : 'justify-center py-1.5 px-2',
        // 옮긴 범례에는 구분선을 그리지 않는다 — 항목 없는 선만 남으면 무엇을 가르는지 모른다.
        !moved &&
          (isVert
            ? 'border-l border-(--color-border-default)'
            : 'border-t border-(--color-border-default)'),
      )}
      data-testid="line-chart-legend"
      // 드래그 레이어가 무엇을 잡았는지 가리는 표식 — 붙인 변은 범례가 스스로 알고 있다.
      data-chart-legend=""
      data-position={legendCfg.position ?? 'bottom'}
      style={{
        fontSize: `${fontSize}px`,
        fontFamily,
        color: fontColor,
        ...inlineLegendOffsetStyle(offsetX, offsetY),
      }}
    >
    <div
      className={cn(
        'flex',
        isVert ? 'flex-col gap-y-1' : 'w-full flex-wrap justify-center gap-x-4 gap-y-1',
      )}
      data-testid="line-chart-legend-items"
      // 끌 수 있는 범위를 재는 자리 — 늘어나는 바깥 상자가 아니라 내용의 실제 크기다.
      data-chart-legend-content=""
    >
      {seriesKeys.map((key, i) => {
        const lastVal = lastValues[key];
        const lastStr = lastVal !== undefined ? formatValue(key, lastVal) : '—';
        return (
          <span
            key={key}
            data-testid={`line-chart-legend-item-${key}`}
            className={cn(
              'inline-flex items-center gap-1',
              isVert && showLastValue && 'w-full',
            )}
          >
            {showLine && (
              <span
                className="inline-block h-0.5 w-3 shrink-0 rounded-full"
                style={{ backgroundColor: seriesColors[i] }}
              />
            )}
            {showName && (
              <span
                className={cn(
                  'shrink-0 whitespace-nowrap',
                  // 색을 지정했으면 상위에서 상속받는다 — 클래스를 그대로 두면 지정색이 먹지 않는다.
                  fontColor ? undefined : 'text-(--color-text-primary)',
                )}
              >
                {key}
              </span>
            )}
            {showLastValue && (
              <span className={cn(
                // 이름보다 한 톤 작게. `em` 이라 글자 크기를 키우면 값도 따라 커진다 —
                // px 로 못박으면 이름만 커지고 값은 그대로인 어긋난 범례가 된다.
                'shrink-0 whitespace-nowrap font-mono text-[0.91em]',
                fontColor ? 'opacity-70' : 'text-(--color-text-muted)',
                isVert && 'ml-auto text-right',
              )}>
                {lastStr}
              </span>
            )}
          </span>
        );
      })}
    </div>
    </div>
  );
}
