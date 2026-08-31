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

export function Pie({
  data,
  dataKey,
  children,
}: {
  data?: Array<{ name?: string; value?: number }>;
  dataKey?: string | number;
  nameKey?: string;
  children?: React.ReactNode;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  label?: any;
}) {
  const rows = data ?? [];
  return (
    <div data-testid="rc-pie" data-pie-key={String(dataKey)}>
      {rows.map((r, i) => (
        <div
          key={i}
          data-testid="rc-pie-slice"
          className="recharts-pie-sector"
          data-name={r.name}
          data-value={r.value}
        />
      ))}
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
