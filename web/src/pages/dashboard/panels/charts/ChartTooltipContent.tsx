// 차트 툴팁의 내용 — 레이블은 왼쪽, 값은 오른쪽에 세운다.
//
// recharts 기본 내용(`DefaultTooltipContent`)은 한 줄을 `이름 : 값` 으로 이어 붙인다.
// 시리즈가 여럿이면 이름 길이가 제각각이라 값의 시작 위치가 줄마다 어긋나고, 자릿수를
// 세로로 비교할 수 없다 — 값을 비교하려고 여는 것이 툴팁인데 그 일이 가장 어렵다.
//
// 그래서 한 줄을 두 칸으로 나눈다: 이름은 왼쪽에 붙이고, 값은 오른쪽 끝에 붙여 자릿수를
// 세로로 맞춘다(`tabular-nums`). 그리기만 여기서 하고, 무엇을 그릴지(포맷터·레이블
// 포맷터·스타일)는 `Tooltip` 이 넘겨준 프롭을 그대로 따른다 — 옮겨 적으면 한쪽만
// 고쳐져 어긋난다.

import type { TooltipContentProps } from 'recharts';
import type { NameType, ValueType } from 'recharts/types/component/DefaultTooltipContent';

/** 기본 상자 모양. 호출부의 `contentStyle` 이 이 위에 덮인다. */
const BASE_CONTENT_STYLE: React.CSSProperties = {
  margin: 0,
  padding: '6px 8px',
  whiteSpace: 'nowrap',
};

export function ChartTooltipContent(
  props: TooltipContentProps<ValueType, NameType>,
): React.ReactElement | null {
  const { active, payload, label, labelFormatter, formatter, contentStyle, labelStyle, itemStyle } =
    props;

  // 비활성이거나 그릴 줄이 없으면 상자를 만들지 않는다 — 빈 상자가 커서를 따라다니면
  // 데이터가 있는 것처럼 보인다.
  if (active === false) return null;
  const items = payload ?? [];
  if (items.length === 0) return null;

  const labelNode = labelFormatter ? labelFormatter(label, items) : label;

  return (
    <div style={{ ...BASE_CONTENT_STYLE, ...contentStyle }} data-testid="chart-tooltip-content">
      {labelNode !== undefined && labelNode !== null && labelNode !== '' && (
        <div style={labelStyle} className="mb-1 text-left">
          {labelNode}
        </div>
      )}
      <div className="flex flex-col gap-0.5">
        {items.map((item, index) => {
          // 포맷터는 값 하나 또는 [값, 이름] 쌍을 돌려준다(recharts 규약).
          const formatted = formatter
            ? formatter(item.value, item.name, item, index, items)
            : undefined;
          const [valueNode, nameNode] = Array.isArray(formatted)
            ? [formatted[0], formatted[1]]
            : [formatted ?? item.value, item.name];
          const swatch = item.color ?? item.stroke ?? item.fill;
          return (
            <div
              key={`${String(item.dataKey ?? item.name ?? '')}-${index}`}
              style={itemStyle}
              className="flex items-center gap-3"
              data-testid="chart-tooltip-row"
            >
              <span className="flex min-w-0 items-center gap-1.5">
                {swatch !== undefined && (
                  <span
                    className="inline-block h-0.5 w-3 shrink-0 rounded-full"
                    style={{ backgroundColor: swatch }}
                  />
                )}
                <span className="truncate text-left" data-testid="chart-tooltip-name">
                  {nameNode as React.ReactNode}
                </span>
              </span>
              <span
                className="ml-auto shrink-0 text-right tabular-nums"
                data-testid="chart-tooltip-value"
              >
                {valueNode as React.ReactNode}
                {item.unit}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
