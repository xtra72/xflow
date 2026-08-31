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
import { useTranslation } from '@/lib/i18n';

import { formatClock, windowDomain } from './chartTime';

/**
 * 차트 데이터 포인트.
 *
 * 시각은 epoch ms 다. 문자열 라벨이 아니라 수치라야 X축을 "표시 구간"에 고정할 수
 * 있다 — 라벨 축(category)은 데이터 개수만큼만 넓어져서, 시작 직후 축이 좁았다가
 * 데이터가 쌓이며 넓어진다.
 */
export interface MetricDataPoint {
  ts: number;
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
  /** Y축 범위 */
  domain: [number, number];
  /**
   * X축 표시 구간 `[시작, 끝]` (epoch ms).
   *
   * 데이터가 덜 모였어도 축은 이 구간 전체를 그린다.
   */
  timeDomain: [number, number];
  /**
   * 값 표기 포맷터 (미지정 시 `값 + 단위`).
   *
   * 네트워크 데이터량처럼 자릿수가 크게 변하는 계열은 단위를 함께 접어야 읽힌다
   * (1024 → 1.0 KB/s).
   */
  format?: (value: number) => string;
}

/** 단일 메트릭 라인 차트 */
function SingleChart({ title, data, color, unit, domain, timeDomain, format }: ChartConfig) {
  const fmt = format ?? ((v: number) => `${v}${unit}`);
  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow p-4">
      <h4 className="text-sm font-medium text-(--color-text-secondary) mb-2">
        {title}
      </h4>
      <ResponsiveContainer width="100%" height={180}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis
            dataKey="ts"
            type="number"
            scale="time"
            domain={timeDomain}
            tickFormatter={formatClock}
            tick={{ fontSize: 10 }}
            stroke="#9ca3af"
          />
          <YAxis
            domain={domain}
            tick={{ fontSize: 10 }}
            stroke="#9ca3af"
            tickFormatter={fmt}
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
            labelFormatter={(ts) => formatClock(Number(ts))}
            formatter={(v: number | undefined) => [
              format ? format(v ?? 0) : `${(v ?? 0).toFixed(1)}${unit}`,
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

/** 메트릭 채널 식별자 */
export type MetricChannel = keyof MetricsData;

/** 채널별 표시 설정 (제목은 i18n 키, 렌더 시 t() 로 변환) */
const CHANNEL_CONFIG: Record<
  MetricChannel,
  { titleKey: string; color: string; unit: string; domain: [number, number] }
> = {
  cpu: {
    titleKey: 'monitoring.cpu',
    color: '#3b82f6',
    unit: '%',
    domain: [0, 100],
  },
  memory: {
    titleKey: 'monitoring.memory',
    color: '#8b5cf6',
    unit: '%',
    domain: [0, 100],
  },
  throughput: {
    titleKey: 'monitoring.throughputLabel',
    color: '#10b981',
    unit: '',
    // 처리량은 상한을 알 수 없어 recharts 의 'auto' 를 쓴다.
    domain: [0, 'auto'] as unknown as [number, number],
  },
  errorRate: {
    titleKey: 'monitoring.errorRate',
    color: '#ef4444',
    unit: '%',
    domain: [0, 'auto'] as unknown as [number, number],
  },
};

/**
 * 임의 시계열 하나를 그리는 범용 차트.
 *
 * `MetricChannelChart` 는 WS 스트림의 고정 4채널 전용이다. 네트워크처럼 출처와
 * 단위가 다른 계열도 같은 모양으로 그리기 위해 설정을 통째로 받는 진입점을 연다.
 */
export function MetricSeriesChart(props: ChartConfig) {
  return <SingleChart {...props} />;
}

/** 채널 렌더 순서 (2x2 그리드 기본 배치) */
export const METRIC_CHANNELS: readonly MetricChannel[] = [
  'cpu',
  'memory',
  'throughput',
  'errorRate',
] as const;

/**
 * 단일 채널 차트.
 *
 * 모니터링 보드가 채널을 항목 단위로 추가·삭제하므로, 2x2 고정 그리드와 별개로
 * 차트 하나만 그릴 수 있는 진입점을 노출한다.
 */
export function MetricChannelChart({
  channel,
  data,
  color,
  windowMs = DEFAULT_WINDOW_MS,
}: {
  channel: MetricChannel;
  data: MetricsData;
  /** 선 색 override (대시보드 패널의 색 설정). 미지정 시 채널 기본색. */
  color?: string;
  /** X축 표시 구간(ms). 데이터가 덜 모였어도 축은 이 구간 전체를 그린다. */
  windowMs?: number;
}) {
  const { t } = useTranslation();
  const config = CHANNEL_CONFIG[channel];
  const series = data[channel];

  return (
    <SingleChart
      title={t(config.titleKey)}
      data={series}
      color={color ?? config.color}
      unit={config.unit}
      domain={config.domain}
      timeDomain={windowDomain([series], windowMs)}
    />
  );
}

/** 기본 표시 구간 (5분) — 스트림 버퍼 길이와 같다. */
const DEFAULT_WINDOW_MS = 300_000;

/** 2x2 그리드의 실시간 메트릭 차트 패널 */
export default function MetricsChart({ data }: MetricsChartProps) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {METRIC_CHANNELS.map((channel) => (
        <MetricChannelChart key={channel} channel={channel} data={data} />
      ))}
    </div>
  );
}
