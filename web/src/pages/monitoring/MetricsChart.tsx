// 실시간 메트릭 차트 컴포넌트.
// recharts 기반의 2x2 그리드 레이아웃으로 CPU, 메모리, 처리량, 에러율을 표시한다.

import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

/** 차트 데이터 포인트 */
export interface MetricDataPoint {
  time: string;
  value: number;
}

/** 4개 메트릭 채널의 데이터 묶음 */
export interface MetricsData {
  cpu: MetricDataPoint[];
  memory: MetricDataPoint[];
  throughput: MetricDataPoint[];
  errorRate: MetricDataPoint[];
}

/** 개별 차트 설정 */
interface ChartConfig {
  title: string;
  data: MetricDataPoint[];
  color: string;
  unit: string;
  domain: [number, number];
}

/** 단일 메트릭 라인 차트 */
function SingleChart({ title, data, color, unit, domain }: ChartConfig) {
  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow p-4">
      <h4 className="text-sm font-medium text-(--color-text-secondary) mb-2">
        {title}
      </h4>
      <ResponsiveContainer width="100%" height={180}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis
            dataKey="time"
            tick={{ fontSize: 10 }}
            stroke="#9ca3af"
            interval="preserveStartEnd"
          />
          <YAxis
            domain={domain}
            tick={{ fontSize: 10 }}
            stroke="#9ca3af"
            tickFormatter={(v: number) => `${v}${unit}`}
            width={50}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: '#1f2937',
              border: 'none',
              borderRadius: '0.375rem',
              color: '#f3f4f6',
              fontSize: '0.75rem',
            }}
            formatter={(v: number | undefined) => [
              `${(v ?? 0).toFixed(1)}${unit}`,
              title,
            ]}
          />
          <Line
            type="monotone"
            dataKey="value"
            stroke={color}
            strokeWidth={2}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

interface MetricsChartProps {
  data: MetricsData;
}

/** 2x2 그리드의 실시간 메트릭 차트 패널 */
export default function MetricsChart({ data }: MetricsChartProps) {
  const charts: ChartConfig[] = [
    {
      title: 'CPU 사용률',
      data: data.cpu,
      color: '#3b82f6',
      unit: '%',
      domain: [0, 100],
    },
    {
      title: '메모리 사용률',
      data: data.memory,
      color: '#8b5cf6',
      unit: '%',
      domain: [0, 100],
    },
    {
      title: '처리량 (msg/s)',
      data: data.throughput,
      color: '#10b981',
      unit: '',
      domain: [0, 'auto'] as unknown as [number, number],
    },
    {
      title: '에러율',
      data: data.errorRate,
      color: '#ef4444',
      unit: '%',
      domain: [0, 'auto'] as unknown as [number, number],
    },
  ];

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {charts.map((chart) => (
        <SingleChart key={chart.title} {...chart} />
      ))}
    </div>
  );
}
