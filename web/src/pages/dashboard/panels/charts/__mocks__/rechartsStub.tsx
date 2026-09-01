// 테스트용 Recharts 스텁.
// Recharts 3.x 는 jsdom 환경에서 SVG 를 정상 렌더하지 못하므로,
// 각 컴포넌트를 단순 div 로 치환하고 data 를 data-* 속성으로 노출해 검증한다.

import React from 'react';

interface ChildrenProps { children?: React.ReactNode }

export function ResponsiveContainer({ children }: ChildrenProps) {
  return (
    <div data-testid="rc-responsive" style={{ width: 400, height: 300 }}>
      {children}
    </div>
  );
}

export function LineChart({
  children,
  data,
}: ChildrenProps & { data?: unknown[]; margin?: unknown }) {
  // Line children 에서 payload 추출 (Legend content 렌더러에 전달)
  const payload: Array<{ value: string; color: string }> = [];
  React.Children.forEach(children, function collect(child) {
    if (!React.isValidElement(child)) return;
    if (child.type === Line) {
      const props = child.props as { dataKey?: string; stroke?: string };
      if (props.dataKey) {
        payload.push({ value: String(props.dataKey), color: props.stroke ?? '#000' });
      }
    }
    // fragment / array 지원
    if ((child.props as ChildrenProps).children) {
      React.Children.forEach((child.props as ChildrenProps).children, collect);
    }
  });

  // Legend 에 payload 주입 (Legend 가 Line 보다 앞에 렌더되므로 2-pass)
  const enhanced = React.Children.map(children, (child) => {
    if (React.isValidElement(child) && child.type === Legend) {
      return React.cloneElement(
        child as React.ReactElement<{ _payload?: typeof payload }>,
        { _payload: payload },
      );
    }
    return child;
  });

  return (
    <div
      data-testid="rc-line-chart"
      data-rows={JSON.stringify(data ?? [])}
      className="recharts-wrapper"
    >
      {enhanced}
    </div>
  );
}

export function BarChart({
  children,
  data,
}: ChildrenProps & { data?: unknown[] }) {
  return (
    <div
      data-testid="rc-bar-chart"
      data-rows={JSON.stringify(data ?? [])}
      className="recharts-wrapper"
    >
      {children}
    </div>
  );
}

export function PieChart({ children }: ChildrenProps) {
  return (
    <div data-testid="rc-pie-chart" className="recharts-wrapper">
      {children}
    </div>
  );
}

export function Line({
  dataKey,
  stroke,
  strokeWidth,
  strokeDasharray,
  type,
}: {
  dataKey?: string | number;
  stroke?: string;
  strokeWidth?: number;
  strokeDasharray?: string;
  type?: string;
}) {
  // 스타일 props 를 data-* 로 노출해 테스트에서 per-line 스타일을 검증한다.
  return (
    <div
      data-testid="rc-line"
      data-line-key={String(dataKey)}
      data-line-stroke={stroke ?? ''}
      data-line-width={strokeWidth ?? ''}
      data-line-dash={strokeDasharray ?? ''}
      data-line-type={type ?? ''}
      className="recharts-line"
    />
  );
}

export function Bar({ dataKey, stackId }: { dataKey?: string | number; stackId?: string }) {
  return (
    <div
      data-testid="rc-bar"
      data-bar-key={String(dataKey)}
      // 그래프 차트가 시리즈를 바로 그릴 때 쓰는 축 — 라인/영역과 같은 키 이름을 쓴다.
      data-line-key={dataKey === undefined ? undefined : String(dataKey)}
      data-stack-id={stackId}
      className="recharts-bar"
    />
  );
}

/**
 * 조각 라벨 렌더러에 넘길 기하 props 를 만든다.
 *
 * 스텁은 실제 파이를 그리지 않으므로 recharts 가 계산해 주는 값이 없다. 그래도
 * **라벨을 호출은 해야** 한다 — 호출하지 않으면 "작은 조각은 라벨을 접는다" 같은
 * 규칙이 테스트에 전혀 걸리지 않는다(실제로 그 공백에서 결함이 났다).
 *
 * 비중(`percent`)은 실제와 같은 규칙(값 / 합계)으로, 나머지는 고정 기하로 채운다.
 */
function pieLabelProps(
  rows: Array<{ name?: string; value?: number }>,
  index: number,
): Record<string, unknown> {
  const total = rows.reduce((a, r) => a + (r.value ?? 0), 0);
  const row = rows[index];
  return {
    // 바깥 배치에서 recharts 가 계산해 주는 자리(안쪽 계산값과 구분되도록 다른 값).
    x: 180,
    y: 100,
    textAnchor: 'start',
    cx: 100,
    cy: 100,
    innerRadius: 0,
    outerRadius: 50,
    midAngle: 0,
    percent: total > 0 ? (row?.value ?? 0) / total : 0,
    value: row?.value ?? 0,
    name: row?.name,
    index,
  };
}

export function Pie({
  data,
  dataKey,
  children,
  label,
  cx,
  cy,
  outerRadius,
}: {
  data?: Array<{ name?: string; value?: number }>;
  dataKey?: string | number;
  nameKey?: string;
  children?: React.ReactNode;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  label?: any;
  cx?: string | number;
  cy?: string | number;
  outerRadius?: string | number;
}) {
  const rows = data ?? [];
  return (
    <div
      data-testid="rc-pie"
      data-pie-key={String(dataKey)}
      // 파이 기하(중심·반지름)는 그려진 결과가 아니라 recharts 에 넘긴 값으로만
      // 관측할 수 있다(스텁은 실제 파이를 그리지 않는다).
      data-cx={cx === undefined ? undefined : String(cx)}
      data-cy={cy === undefined ? undefined : String(cy)}
      data-outer-radius={outerRadius === undefined ? undefined : String(outerRadius)}
    >
      {rows.map((r, i) => (
        <div
          key={i}
          data-testid="rc-pie-slice"
          className="recharts-pie-sector"
          data-name={r.name}
          data-value={r.value}
        />
      ))}
      {typeof label === 'function' && (
        <svg data-testid="rc-pie-labels">
          {rows.map((_, i) => (
            <React.Fragment key={i}>{label(pieLabelProps(rows, i))}</React.Fragment>
          ))}
        </svg>
      )}
      {children}
    </div>
  );
}

export function Cell() {
  return null;
}

export function XAxis({ domain }: { domain?: unknown }) {
  return (
    <div
      data-testid="rc-xaxis"
      data-domain={domain !== undefined ? JSON.stringify(domain) : undefined}
    />
  );
}

export function YAxis({
  domain,
  tickFormatter,
  width,
  label,
}: {
  domain?: unknown;
  tickFormatter?: (v: number) => string;
  width?: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  label?: any;
}) {
  return (
    <div
      data-testid="rc-yaxis"
      // 축 제목이 잘리는지는 폭이 정한다 — 그 값을 노출해 테스트가 관계를 볼 수 있게 한다.
      data-width={width === undefined ? undefined : String(width)}
      data-label={typeof label?.value === 'string' ? label.value : undefined}
      data-domain={domain !== undefined ? JSON.stringify(domain) : undefined}
      // 눈금 포맷터를 표본 값에 적용해 노출한다 — 포맷 배선(소수 자릿수·단위)을
      // 실제 눈금 문자열로 확인할 수 있게 하기 위함. 포맷터가 없으면 속성도 없다.
      data-tick-sample={tickFormatter ? String(tickFormatter(12.3456)) : undefined}
      // 큰 표본 — 자동 환산 단위의 배율이 축과 값에서 같은지 비교하는 데 쓴다.
      data-tick-sample-large={tickFormatter ? String(tickFormatter(137_355.2)) : undefined}
    />
  );
}

/** ComposedChart 는 LineChart 와 같은 자리를 쓴다 — 스텁도 같은 동작을 준다. */
export const ComposedChart = LineChart;

/* eslint-disable @typescript-eslint/no-explicit-any */
export function Area({ dataKey, stackId }: any) {
  return <div data-testid="rc-area" data-line-key={dataKey} data-stack-id={stackId} />;
}

/* eslint-enable @typescript-eslint/no-explicit-any */

export function CartesianGrid() {
  return <div data-testid="rc-grid" />;
}

export function Tooltip({
  content,
  formatter,
}: {
  // 단일 값 모드는 커스텀 content 로 payload 를 좁힌다 — 그 유무만 노출한다.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  content?: any;
  // 실제 시그니처보다 좁게 받는다 — 스텁이 쓰는 것은 (값, 이름) 두 개뿐이다.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  formatter?: any;
}) {
  // 값 포맷터를 표본에 적용해 노출한다 — 소수 자릿수·열거형 라벨·boolean 표기가
  // 실제로 배선됐는지 툴팁을 띄우지 않고 확인할 수 있게 하기 위함.
  const sample = (v: unknown, name: string): string | undefined => {
    if (!formatter) return undefined;
    const out = formatter(v, name);
    return String(Array.isArray(out) ? out[0] : out);
  };
  return (
    <div
      data-testid="rc-tooltip"
      data-single={content ? 'true' : undefined}
      data-fmt-number={sample(12.3456, 'value')}
      // 자동 환산 단위(바이트 접기)는 표본이 접기 밑(1024)을 넘어야 배율이 드러난다.
      // 작은 표본만 노출하면 "축은 KB, 값은 B" 같은 배율 어긋남을 잡을 수 없다.
      data-fmt-number-large={sample(137_355.2, 'value')}
      data-fmt-bool={sample(1, '__bool__')}
    />
  );
}

export function Legend(props: {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  content?: React.FC<any>;
  _payload?: Array<{ value: string; color: string }>;
  wrapperStyle?: unknown;
  verticalAlign?: string;
  align?: string;
  layout?: string;
}) {
  const { content, _payload } = props;
  if (typeof content === 'function') {
    const Content = content;
    return (
      <div data-testid="rc-legend" className="recharts-legend-wrapper">
        <Content payload={_payload ?? []} />
      </div>
    );
  }
  return <div data-testid="rc-legend" className="recharts-legend-wrapper" />;
}

export function ReferenceArea({
  y1,
  y2,
  fill,
}: {
  y1?: number;
  y2?: number;
  fill?: string;
  fillOpacity?: number;
  strokeOpacity?: number;
  ifOverflow?: string;
}) {
  return (
    <div
      data-testid="rc-reference-area"
      data-ref-y1={String(y1 ?? '')}
      data-ref-y2={String(y2 ?? '')}
      data-ref-fill={fill ?? ''}
      className="recharts-reference-area"
    />
  );
}

export function ReferenceLine({
  y,
  stroke,
  label,
}: {
  y?: number | string;
  stroke?: string;
  label?: unknown;
}) {
  return (
    <div
      data-testid="rc-reference-line"
      data-ref-y={String(y ?? '')}
      data-ref-stroke={stroke ?? ''}
      data-ref-label={typeof label === 'string' ? label : ''}
      className="recharts-reference-line"
    />
  );
}
