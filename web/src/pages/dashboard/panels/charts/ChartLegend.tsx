// 라인 차트 범례 — 실제 패널과 설정 미리보기가 공유하는 단일 구현.
//
// 분리 이유: 미리보기가 recharts 내장 <Legend> 를 쓰던 시절, 범례 옵션(이름/선/마지막 값
// 표시)과 차트-범례 구분선·여백이 실제 패널과 달라 "설정한 옵션이 미리보기에 안 먹는다" 는
// 결함이 반복됐다. 두 화면이 같은 컴포넌트를 쓰면 그 차이가 구조적으로 사라진다.
//
// 연결 상태 점(channelStates)은 채널 모드 전용이라 선택적이다 — 미지정이면 점을 그리지 않는다.

import { useMemo } from 'react';

import { cn } from '@/lib/utils/cn';

import type { ChannelRefConfig, LegendConfig } from './chartChannelTypes';

/** 채널 연결 상태(범례 점) — 채널 모드에서만 주입된다. */
export interface ChartLegendChannelState {
  ref: ChannelRefConfig;
  state: { status: string };
}

export function ChartLegend({
  seriesKeys,
  seriesColors,
  channelStates,
  isMultiMode,
  legendCfg,
  chartData,
  formatValue,
}: {
  seriesKeys: string[];
  seriesColors: string[];
  channelStates?: ChartLegendChannelState[];
  isMultiMode: boolean;
  legendCfg: LegendConfig;
  chartData: Array<Record<string, unknown>>;
  /** 시리즈 마지막값 표시 포맷터. enum/boolean 은 라벨로, 그 외는 숫자로 표기한다. */
  formatValue: (key: string, value: number) => string;
}): React.ReactElement | null {
  const states = channelStates ?? [];
  const isVert = legendCfg.position === 'left' || legendCfg.position === 'right';
  const showName = legendCfg.show_name !== false;
  const showLine = legendCfg.show_line !== false;
  const showLastValue = legendCfg.show_last_value === true;

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

  return (
    <div
      className={cn(
        'flex shrink-0 text-[11px]',
        isVert
          ? 'min-w-fit flex-col justify-center gap-y-1 border-l border-(--color-border-default) py-2 pl-3 pr-2'
          : 'flex-wrap justify-center gap-x-4 gap-y-1 border-t border-(--color-border-default) py-1.5 px-2',
      )}
      data-testid="line-chart-legend"
    >
      {seriesKeys.map((key, i) => {
        const baseKey = key.includes('::') ? key.split('::')[0]! : key;
        const chState = isMultiMode
          ? states.find((c) => (c.ref.alias ?? c.ref.name) === baseKey)
          : states[0];
        // 상태를 주입하지 않은 컨텍스트(미리보기)에서는 점을 그리지 않는다.
        const st = chState?.state.status;
        const statusDot =
          st === 'connected'
            ? 'bg-emerald-400'
            : st === 'error'
              ? 'bg-rose-400'
              : 'bg-gray-400';
        const lastVal = lastValues[key];
        const lastStr = lastVal !== undefined ? formatValue(key, lastVal) : '—';
        return (
          <span
            key={key}
            data-testid={`line-chart-channel-status-${chState?.ref.name ?? key}`}
            data-status={st}
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
              <span className="shrink-0 whitespace-nowrap text-(--color-text-primary)">{key}</span>
            )}
            {showLastValue && (
              <span className={cn(
                'shrink-0 whitespace-nowrap font-mono text-[10px] text-(--color-text-muted)',
                isVert && 'ml-auto text-right',
              )}>
                {lastStr}
              </span>
            )}
            {st !== undefined && (
              <span
                className={`inline-block h-1.5 w-1.5 shrink-0 rounded-full ${statusDot}`}
                title={st}
              />
            )}
          </span>
        );
      })}
    </div>
  );
}
