// 시리즈 쿼리 결과 라인 차트 렌더러.
// `SeriesMatrix` (columns + rows 형태) 를 그대로 받아서 recharts `LineChart` 로
// 각 컬럼을 별도의 라인 트레이스로 렌더링한다.
//
// 데이터 변환:
//   matrix.rows[i] = { bucketStartMs, values: number[] }
//   → { bucketStartMs, [column0]: values[0], [column1]: values[1], ... }
//
// recharts 의 dataKey 는 문자열이며 컬론(`:`) 등 특수문자도 그대로 사용 가능하다.
//
// @spec SPEC-WEB-005

import { useCallback, useMemo, useState } from 'react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import { formatLocalTimestamp } from '@/services/api/tsdb';
import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

import {
  applyNullHandling,
  type ChartRow,
  type NullHandlingMode,
} from './tsdbChartNullHandling';

/**
 * 시리즈 라인 색상 팔레트. Pencil 디자인의 액센트 색상과 일치한다.
 * 시리즈가 8개를 초과하면 modulo 로 순환한다.
 */
const SERIES_COLORS = [
  '#3B82F6', // blue
  '#22C55E', // green
  '#F59E0B', // orange (amber)
  '#8B5CF6', // purple
  '#EF4444', // red
  '#14B8A6', // teal
  '#EC4899', // pink
  '#06B6D4', // cyan
] as const;

interface TsdbResultChartProps {
  /** 쿼리 응답 — 컬럼/행으로 pivot 된 매트릭스. */
  matrix: SeriesMatrix;
  /**
   * 집계 함수. `'average'` 일 때만 `decimalPrecision` 이 툴팁/축 라벨에 적용된다.
   * 미지정 시 `'average'` 로 가정한다.
   */
  aggregation?: SeriesMatrixQuery['aggregation'];
  /** 평균 집계 시 표시할 소수점 자릿수 (0-6). 미지정 시 1. */
  decimalPrecision?: number;
}

/**
 * 매트릭스 값을 사람이 읽기 좋은 문자열로 포맷한다.
 * (셀 포맷 로직은 `TsdbResultMatrix` 와 동일 정책을 따름.)
 */
function formatChartValue(
  value: number,
  aggregation: SeriesMatrixQuery['aggregation'] | undefined,
  precision: number,
): string {
  if (aggregation === 'average' && Number.isFinite(value)) {
    const safe = Math.max(0, Math.min(6, Math.floor(precision)));
    return value.toFixed(safe);
  }
  if (Number.isInteger(value)) {
    return value.toLocaleString();
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}


/**
 * 라인 차트 본체.
 * 컬럼이 0개이거나 행이 0개일 때 빈 안내 메시지를 렌더링한다.
 */
export default function TsdbResultChart({
  matrix,
  aggregation,
  decimalPrecision = 1,
}: TsdbResultChartProps) {
  const { columns, rows } = matrix;

  // SPEC-WEB-005 v0.5.0: 결측값 처리 모드.
  // 기본은 `gap` (라인 끊김) 으로 기존 동작 보존.
  const [nullMode, setNullMode] = useState<NullHandlingMode>('gap');
  const [fillValue, setFillValue] = useState<number>(0);

  /**
   * recharts 가 요구하는 row 객체 배열로 변환.
   * - bucketStartMs 는 X축 dataKey 로 사용 (timestamp 정렬 보장).
   * - 각 컬럼 키를 그대로 데이터 필드명으로 사용.
   * - null 값은 일단 그대로 두고 (`gap`), 이후 nullMode 에 따라 변환된다.
   */
  const baseChartData = useMemo<ChartRow[]>(() => {
    return rows.map((row) => {
      const point: ChartRow = {
        bucketStartMs: row.bucketStartMs,
      };
      columns.forEach((col, idx) => {
        point[col] = row.values[idx] ?? null;
      });
      return point;
    });
  }, [columns, rows]);

  // null 처리 변환을 적용한 차트 데이터.
  const chartData = useMemo(
    () => applyNullHandling(baseChartData, columns, nullMode, fillValue),
    [baseChartData, columns, nullMode, fillValue],
  );

  const handleFillValueChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const raw = Number(e.target.value);
      if (Number.isFinite(raw)) setFillValue(raw);
    },
    [],
  );

  if (columns.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        선택된 시리즈가 없습니다.
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        쿼리 결과가 비어 있습니다.
      </div>
    );
  }

  return (
    <div
      className="overflow-hidden rounded-md border border-(--color-border-default) bg-(--color-bg-primary)"
      data-testid="tsdb-result-chart"
    >
      {/*
        결측값 처리 툴바 — 차트 뷰에서만 의미가 있으므로 차트 컨테이너 내부에
        둔다. 4가지 모드를 select 로 제공하고, `value` 모드일 때만 상수 입력을
        노출한다.
      */}
      <div
        className="flex items-center gap-2 border-b border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-xs"
        data-testid="tsdb-result-chart-toolbar"
      >
        <label
          htmlFor="tsdb-chart-null-mode"
          className="font-medium text-(--color-text-muted)"
        >
          결측값 처리:
        </label>
        <select
          id="tsdb-chart-null-mode"
          data-testid="tsdb-chart-null-mode"
          value={nullMode}
          onChange={(e) => setNullMode(e.target.value as NullHandlingMode)}
          className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-0.5 text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
          <option value="gap">빈칸 (라인 끊김)</option>
          <option value="previous">이전 값</option>
          <option value="value">지정값</option>
          <option value="interpolate">선형 보간 (이전 값에서 추론)</option>
        </select>
        {nullMode === 'value' && (
          <label className="inline-flex items-center gap-1.5 text-(--color-text-muted)">
            <span>대체 값:</span>
            <input
              type="number"
              data-testid="tsdb-chart-null-fill-value"
              value={fillValue}
              onChange={handleFillValueChange}
              className="w-20 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-0.5 text-right font-mono text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </label>
        )}
      </div>
      <div className="h-[400px] w-full p-3">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={chartData}
            margin={{ top: 8, right: 16, bottom: 8, left: 0 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border-default)" />
            <XAxis
              dataKey="bucketStartMs"
              type="number"
              domain={['dataMin', 'dataMax']}
              scale="time"
              tickFormatter={(ms: number) => formatLocalTimestamp(ms)}
              stroke="var(--color-text-muted)"
              fontSize={11}
              minTickGap={40}
            />
            <YAxis
              stroke="var(--color-text-muted)"
              fontSize={11}
              tickFormatter={(v: number) =>
                formatChartValue(v, aggregation, decimalPrecision)
              }
              width={60}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: 'var(--color-bg-surface)',
                border: '1px solid var(--color-border-default)',
                borderRadius: '6px',
                fontSize: '12px',
              }}
              labelFormatter={(label) => {
                const ms = typeof label === 'number' ? label : Number(label);
                return Number.isFinite(ms) ? formatLocalTimestamp(ms) : '';
              }}
              formatter={(value) => {
                if (typeof value !== 'number') return String(value);
                return formatChartValue(value, aggregation, decimalPrecision);
              }}
            />
            <Legend
              wrapperStyle={{ fontSize: '11px', paddingTop: '4px' }}
              iconType="line"
            />
            {columns.map((col, idx) => (
              <Line
                key={col}
                type="monotone"
                dataKey={col}
                stroke={SERIES_COLORS[idx % SERIES_COLORS.length]}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4 }}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
