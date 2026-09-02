// 네트워크 채널 차트.
//
// 채널 하나를 그리되, 선택한 인터페이스마다 선을 하나씩 겹쳐 그린다.
// 판별 함수·색·포맷터는 networkSeries.ts 가 갖는다 — 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작한다(logBuffer / logFilters 와 같은 관례).

import { useTranslation } from '@/lib/i18n';

import MultiSeriesChart, { type ChartSeries } from './MultiSeriesChart';
import { sliceToWindow, windowDomain } from './chartTime';
import { findItemMeta } from './monitoringCatalog';
import {
  UNIT_TIME_SUFFIX,
  channelFormatter,
  isTotalChannel,
  type NetworkChannel,
  type NetworkSeries,
  type UnitTime,
} from './networkSeries';

interface NetworkChartProps {
  channel: NetworkChannel;
  /** 인터페이스 이름 → 채널 → 시계열 */
  series: NetworkSeries;
  /** 그릴 인터페이스 목록 (순서가 곧 범례 순서) */
  interfaces: string[];
  /** 인터페이스 이름 → 선 색 */
  colors: Record<string, string>;
  /** rate 계열의 단위시간 */
  unit: UnitTime;
  /** 표시 구간 길이(ms). 데이터가 덜 모였어도 축은 이 구간 전체를 그린다. */
  windowMs: number;
  /** 차트 높이(px) */
  height?: number;
}

export default function NetworkChart({
  channel,
  series,
  interfaces,
  colors,
  unit,
  windowMs,
  height,
}: NetworkChartProps) {
  const { t } = useTranslation();
  const meta = findItemMeta('network', channel);
  const base = meta ? t(meta.labelKey) : channel;
  // 누적 계열은 총량이라 단위시간 표기가 붙지 않는다.
  const title = isTotalChannel(channel) ? base : `${base} (${UNIT_TIME_SUFFIX[unit]})`;

  // 축을 먼저 정하고, 그 구간에 드는 포인트만 남긴다. 개수가 아니라 시각으로 자르는
  // 이유는 폴링 주기가 바뀌거나 표본이 걸러진 구간이 있을 때 개수 기준이 실제 구간과
  // 어긋나기 때문이다.
  const seriesList = interfaces.map((name) => series[name]?.[channel] ?? []);
  const timeDomain = windowDomain(seriesList, windowMs);

  const chartSeries: ChartSeries[] = interfaces.map((name, i) => ({
    name,
    color: colors[name] ?? '#0ea5e9',
    data: sliceToWindow(seriesList[i]!, timeDomain),
  }));

  return (
    <MultiSeriesChart
      title={title}
      series={chartSeries}
      format={channelFormatter(channel, unit)}
      timeDomain={timeDomain}
      height={height}
    />
  );
}
