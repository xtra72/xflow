// sysmetrics 실시간 계열 누적 훅 — 이력 조회의 짝.
//
// 시스템 지표 소스는 두 가지 방식으로 볼 수 있다.
//
//   - **이력**(기본): 에이전트가 버퍼에 보관한 구간을 질의한다(`useSysMetricsChartData`).
//     패널을 열자마자 과거가 그려지고, 닫았다 열어도 선이 남는다. 대신 버킷 경계와 집계로
//     접힌 값이라 표본 하나하나가 아니라 "그 구간의 대표값" 을 본다.
//   - **실시간**(이 파일): 에이전트 스냅샷을 폴링해 오는 대로 이어 붙인다. 접지 않으므로
//     표본이 그대로 보이지만, **패널을 연 뒤 구간만** 그려지고 닫으면 사라진다.
//
// 둘을 한 훅에 섞지 않는 이유: 이력은 "구간을 질의해 버킷으로 접는" 모델이고 실시간은
// "표본을 쌓는" 모델이라 창·인터벌·집계의 뜻이 서로 다르다. 한 코드에 넣으면 어느 축이
// 어느 모드에서 의미 있는지가 조건문 속으로 숨는다.
//
// 반환 형상은 `UseStoreChartDataResult` 그대로다 — 패널 렌더 코드가 소스도 모드도 모른 채
// 같은 결과를 소비해야 하기 때문이다(`usePanelSeriesData` 머리말의 계약).

import { useEffect, useMemo, useRef, useState } from 'react';

import { useSysMetricsSnapshot } from '@/pages/dashboard/panels/sysmetrics/useSysMetricsSnapshot';

import { DEFAULT_STORE_SOURCE_WINDOW, type SysmetricsSourceConfig } from './chartChannelTypes';
import type { ChartEntry } from './chartChannelTypes';
import { readSeriesRange, resolveLiveRetention } from './seriesRange';
import { readSeriesValue, resolveSysmetricsSeries } from './sysmetricsSource';
import type { StoreSeriesStyle, UseStoreChartDataResult } from './useStoreChartData';

/**
 * 누적 카운터를 환산할 단위시간.
 *
 * **초 고정이다.** 에이전트는 이력에 초당 증가량을 저장해 두므로(`sysmetrics_history.go`),
 * 실시간이 분당으로 환산하면 같은 시리즈가 모드에 따라 60배 다른 값으로 보인다. 모드는
 * "언제 것을 보느냐" 이지 "무엇을 보느냐" 가 아니어야 한다.
 */
const RATE_UNIT = 'sec' as const;

/**
 * 계열당 보관 **하드 상한**. 표시 창으로 이미 자르지만, 창이 아주 길거나 갯수 방식으로
 * 크게 잡았을 때의 메모리 상한이다 — 창은 사용자가 정하지만 브라우저가 버티는 양은 아니다.
 */
const MAX_POINTS = 1_800;

/** 조회하지 않는 경로가 돌려주는 idle 결과(형상 유지). */
const IDLE: UseStoreChartDataResult = {
  entries: [],
  seriesEntries: new Map(),
  seriesStyles: new Map(),
  seriesNames: [],
  booleanSeries: new Set(),
  status: 'idle',
};

/**
 * 스냅샷을 폴링해 계열로 쌓는다.
 *
 * @param source 시스템 지표 소스 config. 없으면 idle.
 * @param enabled 이 경로가 실제 조회 담당인지. false 면 폴링하지 않는다(훅 규칙상 호출은 한다).
 */
export function useSysmetricsLiveSeries(
  source: SysmetricsSourceConfig | undefined,
  enabled: boolean,
): UseStoreChartDataResult {
  const agentId = source?.agent_id ?? '';
  const refreshMs = source?.refresh_interval_ms ?? DEFAULT_STORE_SOURCE_WINDOW.refresh_interval_ms;
  // 보관 기준은 **조회 범위**가 정한다. 종전에는 `time_window_ms` 만 봤는데, 실시간에서는
  // 소스 설정이 구간 칸을 내리므로(버킷이 없어 조회 창의 뜻이 다르다) 그 값을 고칠 자리가
  // 어디에도 없었다 — 이제 X축 섹션이 같은 필드(`range`)를 고친다.
  const retention = useMemo(
    () =>
      resolveLiveRetention(
        readSeriesRange(
          source?.range,
          source?.time_window_ms ?? DEFAULT_STORE_SOURCE_WINDOW.time_window_ms,
        ),
        MAX_POINTS,
      ),
    [source?.range, source?.time_window_ms],
  );
  const { windowMs, maxPoints } = retention;

  // 조회 담당이 아니면 빈 에이전트로 넘겨 폴링 자체를 끈다(훅은 조건 없이 호출).
  const { snapshot, previous, state } = useSysMetricsSnapshot(
    enabled ? agentId : '',
    refreshMs,
  );

  const lines = useMemo(() => (enabled ? resolveSysmetricsSeries(source) : []), [enabled, source]);

  // 계열 구성이 바뀌면 쌓아 둔 것을 버린다. 남겨 두면 해제한 시리즈의 선이 계속 남고,
  // 이름이 바뀐 줄은 옛 이름으로 한 번 더 그려진다.
  //
  // 구성 비교는 **내용 기준 키**로 한다 — `lines` 는 매 렌더 새 배열이라 참조로 비교하면
  // "effect → setState → 리렌더 → 새 배열 → effect" 가 끝없이 돈다(useSysMetricsRateSeries
  // 가 같은 함정을 겪었다).
  const shapeKey = useMemo(
    () => `${agentId}|${lines.map((l) => `${l.name}:${l.field.key}:${l.target ?? ''}:${l.mode}`).join('|')}`,
    [agentId, lines],
  );
  const [series, setSeries] = useState<Map<string, ChartEntry[]>>(new Map());
  const shapeRef = useRef('');
  // 마지막으로 쌓은 표본 시각. 같은 표본을 두 번 쌓지 않기 위한 빗장이다.
  const lastAppendedRef = useRef<number | null>(null);
  useEffect(() => {
    if (shapeRef.current === shapeKey) return;
    shapeRef.current = shapeKey;
    // 구성이 바뀌면 빗장도 함께 푼다 — 안 그러면 현재 표본을 다시 쌓지 못해 다음 폴링까지
    // 빈 차트로 남는다.
    lastAppendedRef.current = null;
    setSeries(new Map());
  }, [shapeKey]);

  // 새 표본이 올 때마다 한 점씩 이어 붙인다. 같은 표본을 다시 받으면 `useSysMetricsSnapshot`
  // 이 기준점을 갱신하지 않으므로 여기서도 값이 그대로여서 점이 겹쳐 쌓이지는 않는다.
  const collectedAt = snapshot?.collectedAt ?? null;
  useEffect(() => {
    if (!enabled || !snapshot || collectedAt === null) return;
    // **표본 하나는 한 번만 쌓는다.** 스냅샷 객체의 참조가 렌더마다 새로 만들어져도
    // (호출부 사정으로 얼마든지 그럴 수 있다) 표본 시각이 같으면 이미 쌓은 것이다.
    // 이 빗장이 없으면 "effect → setState → 리렌더 → 새 참조 → effect" 가 끝없이 돌아
    // 렌더 깊이 초과로 화면이 죽는다(useSysMetricsRateSeries 가 같은 함정을 겪었다).
    if (lastAppendedRef.current === collectedAt) return;
    lastAppendedRef.current = collectedAt;
    setSeries((current) => {
      const next = new Map(current);
      const cutoff = collectedAt - windowMs;
      let appended = false;
      for (const line of lines) {
        const value = readSeriesValue(snapshot, previous, line, RATE_UNIT);
        // null 은 "그릴 수 없다"이지 0 이 아니다(기준점 없음·되감김·경과 0). 점을 만들지 않는다.
        if (value === null) continue;
        const kept = (next.get(line.name) ?? []).filter((e) => e.timestamp >= cutoff);
        kept.push({ timestamp: collectedAt, value });
        next.set(
          line.name,
          kept.length > maxPoints ? kept.slice(kept.length - maxPoints) : kept,
        );
        appended = true;
      }
      // 쌓을 것이 없으면 **같은 참조를 돌려준다** — 새 Map 을 만들면 값이 그대로인데도
      // 리렌더가 한 번 더 돈다(첫 표본처럼 전부 null 인 구간에서 매번 일어난다).
      return appended ? next : current;
    });
    // `lines` 는 shapeKey 로 이미 구성 변화를 다루므로 의존성에서 뺀다 — 넣으면 매 렌더
    // 새 배열이라 effect 가 표본과 무관하게 계속 돈다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, snapshot, previous, collectedAt, windowMs, maxPoints]);

  return useMemo<UseStoreChartDataResult>(() => {
    if (!enabled) return IDLE;

    const seriesEntries = new Map<string, ChartEntry[]>();
    const seriesStyles = new Map<string, StoreSeriesStyle>();
    const seriesNames: string[] = [];
    const entries: ChartEntry[] = [];

    for (const line of lines) {
      seriesNames.push(line.name);
      const points = series.get(line.name) ?? [];
      seriesEntries.set(line.name, points);
      entries.push(...points);
      seriesStyles.set(line.name, {
        color: line.color,
        stroke_style: line.ref.stroke_style,
        stroke_width: line.ref.stroke_width,
        smooth: line.ref.smooth,
        graph_style: line.ref.graph_style,
      });
    }
    // 평탄 타임라인은 시간순이어야 한다 — 계열별로 이어 붙이면 계열 경계에서 되감긴다.
    entries.sort((a, b) => a.timestamp - b.timestamp);

    return {
      entries,
      seriesEntries,
      seriesStyles,
      seriesNames,
      booleanSeries: new Set<string>(),
      status: liveStatus(state),
    };
  }, [enabled, lines, series, state]);
}

/**
 * 스냅샷 상태 → 차트 연결 상태.
 *
 * 멈춘 에이전트·첫 표본 이전은 **오류가 아니다** — 조회는 되고 있고 그릴 점이 아직 없을
 * 뿐이다. 오류로 올리면 패널이 마지막 렌더를 버리고 오류 배지를 띄운다.
 */
function liveStatus(state: ReturnType<typeof useSysMetricsSnapshot>['state']): UseStoreChartDataResult['status'] {
  switch (state) {
    case 'loading':
      return 'connecting';
    case 'unavailable':
    case 'incompatible':
      return 'error';
    default:
      return 'connected';
  }
}
