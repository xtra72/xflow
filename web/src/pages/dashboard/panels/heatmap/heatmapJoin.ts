// 센서 최신값 + 배치 좌표 결합 순수 로직 (SPEC-HEATMAP-PANEL-001 T6).
//
// useStoreChartData 가 반환한 시리즈(표시 이름 → 타임라인)에서 각 시리즈의 최신 유한값을
// 센서 1개 판독값으로 추출하고(aggregation:'last'), config.sensor_positions 좌표와 결합해
// IDW 보간 입력점을 만든다. 좌표가 없는 센서는 보간 입력에서 제외하되 설정 UI 노출용으로
// 별도 목록에 담는다(AC-E2). DOM 의존이 없어 단위 테스트로 커버한다.
//
// @spec SPEC-HEATMAP-PANEL-001

import type { ChartEntry } from '../charts/chartChannelTypes';
import type { SensorPosition } from './heatmapConfig';
import type { IdwPoint } from './idw';

/** 시리즈 타임라인에서 마지막 유한 숫자값(최신 판독값)을 추출한다. 없으면 null. */
export function latestFiniteValue(entries: ChartEntry[] | undefined): number | null {
  if (!entries) return null;
  for (let i = entries.length - 1; i >= 0; i--) {
    const v = entries[i]!.value;
    if (typeof v === 'number' && Number.isFinite(v)) return v;
  }
  return null;
}

/** 센서-좌표 결합 결과. */
export interface JoinResult {
  /** 좌표가 배치된 센서(IDW 보간 입력점). */
  points: IdwPoint[];
  /** 좌표가 배치된 센서 표시 이름. */
  placedNames: string[];
  /** 최신 판독값은 있으나 좌표가 미지정된 센서 이름(설정 UI 안내용, AC-E2). */
  unplacedNames: string[];
  /** 배치 센서값의 자동 범위(value_bounds 미설정 시 폴백). 배치 0개면 null. */
  autoBounds: { min: number; max: number } | null;
}

/**
 * store 시리즈 최신값을 sensor_positions 좌표와 결합해 IDW 입력점을 만든다.
 *
 * - 최신 유한값이 없는 시리즈는 판독값이 없으므로 무시한다(placed/unplaced 어디에도 넣지 않음).
 * - 좌표가 있는 센서만 points 에 넣고(placedNames), 좌표 없는 센서는 unplacedNames 에 담는다.
 * - autoBounds 는 배치 센서값의 [min, max]. 배치가 없으면 null.
 */
export function joinSensorPoints(
  seriesNames: string[],
  seriesEntries: Map<string, ChartEntry[]>,
  sensorPositions: Record<string, SensorPosition>,
): JoinResult {
  const points: IdwPoint[] = [];
  const placedNames: string[] = [];
  const unplacedNames: string[] = [];
  let min = Infinity;
  let max = -Infinity;

  for (const name of seriesNames) {
    const value = latestFiniteValue(seriesEntries.get(name));
    if (value === null) continue; // 판독값 없음 → 무시.
    const pos = sensorPositions[name];
    if (pos) {
      points.push({ x: pos.x, y: pos.y, value });
      placedNames.push(name);
      if (value < min) min = value;
      if (value > max) max = value;
    } else {
      unplacedNames.push(name);
    }
  }

  const autoBounds = points.length > 0 ? { min, max } : null;
  return { points, placedNames, unplacedNames, autoBounds };
}
