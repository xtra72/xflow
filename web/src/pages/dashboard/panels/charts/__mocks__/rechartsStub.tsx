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
}: ChildrenProps & { data?: unknown[] }) {
  return (
    <div
      data-testid="rc-line-chart"
      data-rows={JSON.stringify(data ?? [])}
      className="recharts-wrapper"
    >
      {children}
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

export function Line({ dataKey }: { dataKey?: string | number }) {
  return (
    <div
      data-testid="rc-line"
      data-line-key={String(dataKey)}
      className="recharts-line"
    />
  );
}

export function Bar({ dataKey }: { dataKey?: string | number }) {
  return <div data-testid="rc-bar" data-bar-key={String(dataKey)} className="recharts-bar" />;
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

export function YAxis({ domain }: { domain?: unknown }) {
  return (
    <div
      data-testid="rc-yaxis"
      data-domain={domain !== undefined ? JSON.stringify(domain) : undefined}
    />
  );
}

export function CartesianGrid() {
  return <div data-testid="rc-grid" />;
}

export function Tooltip() {
  return <div data-testid="rc-tooltip" />;
}

export function Legend() {
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
