// 여러 계열을 한 차트에 겹쳐 그리는 라인 차트.
//
// 네트워크는 인터페이스마다 선이 하나씩 필요하다(en0 / lo0 / total ...).
// 단일 계열용 MetricSeriesChart 와 축·툴팁 모양을 맞춰 두 차트가 나란히 놓여도
// 이질감이 없게 했다.

import {
  Area,
  Bar,
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import type { MetricDataPoint } from './MetricsChart';
import { formatClock } from './chartTime';

/** 계열 하나 */
export interface ChartSeries {
  /** 범례에 표시할 이름 (인터페이스 이름) */
  name: string;
  color: string;
  data: MetricDataPoint[];
}

/**
 * 계열을 그리는 모양.
 *
 * 이름과 뜻은 `panels/charts/graphStyle.ts` 의 GraphStyle 과 맞춘다 — 두 곳이 다른
 * 어휘를 쓰면 설정 화면과 렌더가 서로 다른 말을 하게 된다. 여기서는 캔들을 뺀다
 * (시가·고가·저가·종가가 필요한데 시스템 지표에는 그런 축이 없다).
 */
export type SeriesChartStyle = 'line' | 'area' | 'bar';

/** 범례 위치. 'none' 은 범례를 그리지 않는다. */
export type LegendPosition = 'none' | 'top' | 'bottom' | 'right';

interface MultiSeriesChartProps {
  title: string;
  series: ChartSeries[];
  /** 값 표기 포맷터 (축 눈금 + 툴팁 공용) */
  format: (value: number) => string;
  /**
   * X축 표시 구간 `[시작, 끝]` (epoch ms).
   *
   * 데이터가 덜 모였어도 축은 이 구간 전체를 그린다.
   */
  timeDomain: [number, number];
  /**
   * 차트 높이(px). 생략하면 기본값을 쓴다.
   *
   * 부모 칸을 채우려면 `fillParent` 를 켠다 — 그때 이 값은 무시된다.
   */
  height?: number;
  /**
   * 부모 칸을 채운다.
   *
   * 켜면 카드가 세로로 늘어나고 차트가 남은 높이를 모두 쓴다. 부모에 확정된 높이가
   * 있어야 한다(recharts ResponsiveContainer 의 제약) — 격자의 `1fr` 행이 그 조건을
   * 만족한다.
   */
  fillParent?: boolean;
  /**
   * 계열을 그리는 모양. 기본은 라인이다(기존 호출부 동작 보존).
   *
   * 영역·막대는 쌓지 않고 겹쳐 그린다. 인터페이스별 수신량처럼 각 계열이 독립적인
   * 값일 때 누적으로 읽히면 총량을 잘못 읽게 되기 때문이다.
   */
  style?: SeriesChartStyle;
  /**
   * 범례 위치. 기본은 자동이다 — 계열이 둘 이상일 때만 아래에 그린다(기존 동작).
   *
   * 명시하면 계열 수와 무관하게 그 위치를 따른다. 'none' 이면 그리지 않는다.
   */
  legend?: LegendPosition;
  /** 제목을 숨긴다. 항목 이름이 이미 바깥에 있을 때 쓴다. */
  hideTitle?: boolean;
  /**
   * 곡선 보간. 차트 패널의 `smooth` 와 같은 뜻이다.
   *
   * 끄면 점과 점을 직선으로 잇는다 — 표본 사이를 부드럽게 이으면 없던 값이 있는
   * 것처럼 보이므로, 기본은 끄지 않고 호출부가 정하도록 둔다.
   */
  smooth?: boolean;
  /**
   * 계열 누적. 차트 패널의 `stacked` 와 같은 뜻이며 영역·막대에서만 적용된다
   * (라인은 쌓아도 겹친 선일 뿐이다).
   */
  stacked?: boolean;
}

/**
 * 계열들을 시각(ts) 기준으로 한 테이블로 합친다.
 *
 * recharts 는 하나의 data 배열에서 여러 dataKey 를 읽는다. 계열마다 표본 시각이
 * 어긋날 수 있으므로(인터페이스가 중간에 생기거나 사라짐) 시각을 키로 합치고,
 * 값이 없는 칸은 비워 둔다 — `connectNulls` 로 선이 끊기지 않게 이어 준다.
 *
 * 수치 축이라 행은 시각 오름차순이어야 한다 — 계열마다 시작 시점이 다르면 등장
 * 순서가 곧 시간 순서가 아니다.
 */
function mergeByTime(series: ChartSeries[]): Record<string, number>[] {
  const rows = new Map<number, Record<string, number>>();

  for (const s of series) {
    for (const point of s.data) {
      let row = rows.get(point.ts);
      if (!row) {
        row = { ts: point.ts };
        rows.set(point.ts, row);
      }
      row[s.name] = point.value;
    }
  }

  return [...rows.values()].sort((a, b) => a.ts! - b.ts!);
}

/** 범례 위치 → recharts 배치 props */
const LEGEND_LAYOUT: Record<
  Exclude<LegendPosition, 'none'>,
  { verticalAlign: 'top' | 'bottom'; align: 'center' | 'right'; layout: 'horizontal' | 'vertical' }
> = {
  top: { verticalAlign: 'top', align: 'center', layout: 'horizontal' },
  bottom: { verticalAlign: 'bottom', align: 'center', layout: 'horizontal' },
  right: { verticalAlign: 'middle', align: 'right', layout: 'vertical' } as never,
};

export default function MultiSeriesChart({
  title,
  series,
  format,
  timeDomain,
  height = 180,
  style = 'line',
  legend,
  hideTitle,
  smooth,
  stacked,
  fillParent,
}: MultiSeriesChartProps) {
  const data = mergeByTime(series);
  // 지정이 없으면 종전 규칙을 따른다: 선이 하나뿐이면 범례가 정보를 더하지 않는다
  // (제목에 이미 드러난다).
  const legendPosition: LegendPosition =
    legend ?? (series.length > 1 ? 'bottom' : 'none');
  const legendLayout = legendPosition === 'none' ? null : LEGEND_LAYOUT[legendPosition];
  // 곡선 여부는 recharts 의 보간 타입으로 옮긴다.
  const lineType = smooth ? 'monotone' : 'linear';
  // 누적은 영역·막대에서만 뜻이 있다 (차트 패널의 isStackable 과 같은 규칙).
  const stackId = stacked && (style === 'area' || style === 'bar') ? 'sysmetrics' : undefined;

  return (
    <div
      className={
        fillParent
          ? 'flex h-full min-h-0 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow'
          : 'rounded-lg bg-(--color-bg-surface) p-4 shadow'
      }
    >
      {!hideTitle && (
        <h4 className="mb-2 truncate text-sm font-medium text-(--color-text-secondary)" title={title}>
          {title}
        </h4>
      )}
      <div className={fillParent ? 'min-h-0 w-full flex-1' : undefined}>
      <ResponsiveContainer width="100%" height={fillParent ? '100%' : height}>
        <ComposedChart data={data}>
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
            domain={[0, 'auto']}
            tick={{ fontSize: 10 }}
            stroke="#9ca3af"
            tickFormatter={format}
            width={64}
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
            formatter={(v: number | undefined, name) => [format(v ?? 0), name]}
          />
          {legendLayout && <Legend wrapperStyle={{ fontSize: '0.7rem' }} {...legendLayout} />}
          {series.map((s) => {
            if (style === 'bar') {
              return (
                <Bar
                  key={s.name}
                  dataKey={s.name}
                  fill={s.color}
                  stackId={stackId}
                  isAnimationActive={false}
                />
              );
            }
            if (style === 'area') {
              return (
                <Area
                  key={s.name}
                  type={lineType}
                  stackId={stackId}
                  dataKey={s.name}
                  stroke={s.color}
                  fill={s.color}
                  fillOpacity={0.25}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              );
            }
            return (
              <Line
                key={s.name}
                type={lineType}
                dataKey={s.name}
                stroke={s.color}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
                connectNulls
              />
            );
          })}
        </ComposedChart>
      </ResponsiveContainer>
      </div>
    </div>
  );
}
