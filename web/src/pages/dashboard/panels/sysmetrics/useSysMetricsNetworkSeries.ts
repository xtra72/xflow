// sysmetrics 네트워크 시계열 누적 훅.
//
// 에이전트는 그 시점의 누적 카운터만 준다. 차트에 선을 그리려면 표본이 올 때마다
// 증가량을 계산해 창(window)에 쌓아야 한다. 계열은 훅 안에서만 유지되므로 패널이
// 사라지면 함께 정리된다 — 되살릴 히스토리가 필요하면 저장 경로(storage-write)의 몫이다.

import { useEffect, useMemo, useRef, useState } from 'react';

import { appendPoint, type UnitTime } from '@/pages/monitoring/networkSeries';
import type { MetricDataPoint } from '@/pages/monitoring/MetricsChart';

import { deltaRate, type SysMetricsSnapshot } from './sysMetricsSeries';
import type { SysMetricsCounterMode } from './sysMetricsItemOptions';

/** 보관할 최대 포인트 수 — 표시 구간을 넉넉히 덮는 상한 */
const MAX_POINTS = 1_800;

/** 전체 합산 계열의 이름 (모니터링 패널과 같은 어휘) */
export const TOTAL_TARGET = 'total';

/**
 * 대상 목록을 하나의 키로 접을 때 쓰는 구분자.
 *
 * 인터페이스·장치 이름에 나타날 수 없는 문자여야 두 목록이 우연히 같은 키가 되지 않는다.
 */
const TARGET_SEPARATOR = ' ';

/** 대상 이름 → 필드 이름 → 시계열 */
export type RateSeries = Record<string, Record<string, MetricDataPoint[]>>;

/**
 * 인스턴스별 누적 카운터를 증가량 시계열로 쌓는다.
 *
 * @param snapshot 최신 스냅샷
 * @param previous 직전 스냅샷 (rate 의 기준점)
 * @param pick 스냅샷에서 인스턴스 그룹을 꺼내는 함수 (network / diskIo)
 * @param targets 그릴 대상 이름. **빈 배열이면 전체 합산 하나**를 그린다.
 * @param fields 계열로 만들 필드 이름
 * @param unit 단위시간
 * @param modes 필드별 표시 방식. 빠진 필드는 `rate`(증가량)다. `total` 이면 누적
 *   원값을 그대로 쌓는다 — 환산하지 않으므로 되감김·경과 0 에도 점이 끊기지 않는다.
 *
 * 호출부가 `targets` / `fields` 를 매 렌더 새 배열로 넘겨도 안전하다 — 내용 기준
 * 키로 접어 쓴다. 배열 참조를 그대로 effect 의존성에 걸면 "effect → setState →
 * 리렌더 → 새 배열 → effect" 가 끝없이 돌아 렌더 깊이 초과로 화면이 죽는다
 * (useNetworkSeries 가 같은 함정을 겪었다).
 */
export function useSysMetricsRateSeries(
  snapshot: SysMetricsSnapshot | null,
  previous: SysMetricsSnapshot | null,
  pick: (s: SysMetricsSnapshot) => Record<string, Record<string, number>> | undefined,
  targets: string[],
  fields: string[],
  unit: UnitTime,
  modes: Record<string, SysMetricsCounterMode> = {},
): RateSeries {
  const [series, setSeries] = useState<RateSeries>({});

  const targetKey = targets.length > 0 ? targets.join(TARGET_SEPARATOR) : TOTAL_TARGET;
  const fieldKey = fields.join(TARGET_SEPARATOR);
  // 표시 방식도 내용 기준 키로 접는다 — 호출부가 매 렌더 새 객체를 넘겨도 안전해야
  // 한다(`targets` / `fields` 와 같은 이유).
  const modeKey = fields.map((f) => modes[f] ?? 'rate').join(TARGET_SEPARATOR);
  const resolvedTargets = useMemo(() => targetKey.split(TARGET_SEPARATOR), [targetKey]);
  const resolvedFields = useMemo(() => fieldKey.split(TARGET_SEPARATOR), [fieldKey]);
  const resolvedModes = useMemo(
    () => modeKey.split(TARGET_SEPARATOR) as SysMetricsCounterMode[],
    [modeKey],
  );

  // 대상 구성이 바뀌면 계열을 비운다. 남겨 두면 지운 인터페이스의 선이 차트에 계속 남는다.
  // 표시 방식이 바뀔 때도 비운다 — 증가량과 누적값은 자릿수가 달라 한 축에 섞이면
  // 앞 구간이 바닥에 눌린 선으로 남는다.
  const shapeRef = useRef('');
  useEffect(() => {
    const shape = `${targetKey}|${fieldKey}|${modeKey}|${unit}`;
    if (shapeRef.current === shape) return;
    shapeRef.current = shape;
    setSeries({});
  }, [targetKey, fieldKey, modeKey, unit]);

  useEffect(() => {
    if (!snapshot || snapshot.collectedAt === null) return;
    if (!previous || previous.collectedAt === null) return;

    const nextGroup = pick(snapshot);
    const prevGroup = pick(previous);
    if (!nextGroup || !prevGroup) return;

    // 좁혀진 타입을 콜백 안까지 들고 가려면 지역 변수로 붙들어야 한다.
    const at = snapshot.collectedAt;
    const prevAt = previous.collectedAt;

    setSeries((current) => {
      const updated: RateSeries = { ...current };

      for (const target of resolvedTargets) {
        const nextMetrics =
          target === TOTAL_TARGET ? sumGroup(nextGroup) : nextGroup[target];
        const prevMetrics =
          target === TOTAL_TARGET ? sumGroup(prevGroup) : prevGroup[target];
        // 대상이 사라졌으면 이번 표본은 건너뛴다 — 나머지 선은 계속 그린다.
        if (!nextMetrics || !prevMetrics) continue;

        const perField: Record<string, MetricDataPoint[]> = { ...(updated[target] ?? {}) };
        resolvedFields.forEach((field, i) => {
          const nextValue = nextMetrics[field];
          const prevValue = prevMetrics[field];
          if (nextValue === undefined || prevValue === undefined) return;

          // 누적 모드는 환산하지 않는다 — 원값이 곧 그릴 값이다.
          const value =
            resolvedModes[i] === 'total'
              ? nextValue
              : deltaRate({ value: prevValue, at: prevAt }, { value: nextValue, at }, unit);
          // null 은 "그릴 수 없다"이지 0 이 아니다 (되감김·경과 0). 점을 만들지 않는다.
          if (value === null) return;

          perField[field] = appendPoint(perField[field] ?? [], { ts: at, value }, MAX_POINTS);
        });
        updated[target] = perField;
      }

      return updated;
    });
    // resolvedTargets / resolvedFields 는 키에서 파생되므로 참조가 안정적이다.
  }, [snapshot, previous, pick, resolvedTargets, resolvedFields, resolvedModes, unit]);

  return series;
}

/** 인스턴스 그룹의 필드를 전부 더한다 (합산 계열용). */
function sumGroup(group: Record<string, Record<string, number>>): Record<string, number> {
  const out: Record<string, number> = {};
  for (const metrics of Object.values(group)) {
    for (const [field, value] of Object.entries(metrics)) {
      out[field] = (out[field] ?? 0) + value;
    }
  }
  return out;
}
