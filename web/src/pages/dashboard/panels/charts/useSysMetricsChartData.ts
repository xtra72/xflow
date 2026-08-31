// sysmetrics 소스 → Store 조회 경로 어댑터.
//
// **에이전트가 이력을 들고 있다.** 설정된 기간만큼 표본을 버퍼에 보관하고
// (`sysmetrics_history.go`), 이 훅은 그 구간을 질의한다. 브라우저가 마운트 시점부터
// 점을 누적하던 종전 방식과 달리, 패널을 열자마자 과거 구간이 그려지고 닫았다 열어도
// 선이 초기화되지 않는다.
//
// 조회·버킷팅·이름 짓기·폴링은 **`useStoreChartData` 가 그대로 한다**. 그 훅은 매트릭스
// 조회 함수를 주입받으므로(`queryMatrixFn`), 소스별로 다른 것은 두 가지뿐이다 — config
// 형상을 옮기는 변환(`toStoreShapedConfig`)과 조회 함수(`useSysmetricsQueryFn`). 훅
// 사본을 만들면 두 소스의 버킷 경계·이름 규칙·폴링 정책이 갈라진다.
//
// 그래서 이 모듈은 **훅을 따로 내보내지 않는다**. `usePanelSeriesData` 가
// `useStoreChartData` 를 한 번만 부르고 위 둘을 주입한다 — 두 번 부르면 진 쪽이 idle
// 이라도 훅이 중복되고, 어느 호출이 실제 조회인지 읽기 어려워진다.
//
// 누적 카운터(네트워크·디스크 I/O)는 에이전트가 저장 시점에 **초당 증가량**으로 환산해
// 둔다. 그래서 이 경로에는 증가량 축이 없고, 패널 설정도 Store · TSDB 와 같아진다.

import { useCallback } from 'react';

import { querySysmetricsMatrix } from '@/services/api/sysmetricsHistory';
import type { SeriesMatrixQuery } from '@/services/api/seriesDataSource';

import {
  DEFAULT_STORE_SOURCE_WINDOW,
  type StoreSourceConfig,
  type SysmetricsSourceConfig,
} from './chartChannelTypes';
import { resolveSysmetricsSeries } from './sysmetricsSource';
import type { QueryMatrixFn } from './useStoreChartData';

/**
 * sysmetrics 소스 config 를 **Store 소스 형상**으로 옮긴다.
 *
 * 두 소스가 이미 같은 어휘(measurement + tags)를 쓰므로 옮기는 일은 자리 바꾸기뿐이다.
 * `key` 에는 measurement, `tags` 에는 분류·대상이 들어가며, 이름·색·선 모양은 그대로
 * 물려준다 — 그래야 `useStoreChartData` 가 만드는 계열 이름이 이 소스의 이름 규칙과
 * 일치한다.
 *
 * 이 변환이 하중 지지점이다. 여기서 어휘가 어긋나면 조회는 되는데 이름이 달라져,
 * 시리즈 스타일이 엉뚱한 줄에 붙는다.
 */
export function toStoreShapedConfig(
  source: SysmetricsSourceConfig | undefined,
): StoreSourceConfig | undefined {
  if (!source) return undefined;
  const lines = resolveSysmetricsSeries(source);
  return {
    agent_id: source.agent_id,
    agent_name: source.agent_name,
    series: lines.map((line) => ({
      key: line.measurement,
      tags: line.tags,
      alias: line.ref.alias,
      color: line.ref.color,
      stroke_style: line.ref.stroke_style,
      stroke_width: line.ref.stroke_width,
      smooth: line.ref.smooth,
      graph_style: line.ref.graph_style,
    })),
    series_name_format: source.series_name_format,
    range: source.range,
    ...sysmetricsWindow(source),
  };
}

/**
 * 저장된 소스의 조회 창 — 없는 필드는 기본값으로 메운다.
 *
 * 이 필드들은 "에이전트가 이력을 보관" 으로 바뀌면서 생겼다. 그 전에 저장된 패널의
 * config 에는 없으므로(타입은 필수라고 말하지만 디스크의 JSON 은 그렇지 않다),
 * 메우지 않으면 `useStoreChartData` 가 창을 해석하지 못해 조회를 아예 시작하지
 * 않는다 — 화면에는 오류 없이 **빈 차트**로만 보인다.
 */
export function sysmetricsWindow(
  source: SysmetricsSourceConfig,
): Pick<
  StoreSourceConfig,
  'time_window_ms' | 'interval_ms' | 'aggregation' | 'refresh_interval_ms'
> {
  return {
    time_window_ms: source.time_window_ms ?? DEFAULT_STORE_SOURCE_WINDOW.time_window_ms,
    interval_ms: source.interval_ms ?? DEFAULT_STORE_SOURCE_WINDOW.interval_ms,
    aggregation: source.aggregation ?? DEFAULT_STORE_SOURCE_WINDOW.aggregation,
    refresh_interval_ms:
      source.refresh_interval_ms ?? DEFAULT_STORE_SOURCE_WINDOW.refresh_interval_ms,
  };
}

/**
 * sysmetrics 이력을 조회하는 매트릭스 함수를 만든다.
 *
 * `useStoreChartData` 가 이름으로 조회하는 것과 달리 이력 커맨드는
 * `/agents/{id}/query` 라 **id** 가 필요하다. 그래서 id 를 닫아 잡고 넘겨받은 이름은
 * 쓰지 않는다.
 */
export function useSysmetricsQueryFn(agentId: string): QueryMatrixFn {
  return useCallback<QueryMatrixFn>(
    (_agentName: string, params: SeriesMatrixQuery, signal: AbortSignal) =>
      querySysmetricsMatrix(agentId, params, signal),
    [agentId],
  );
}
