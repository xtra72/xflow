// 센서 최신값 + 배치 좌표 결합 순수 로직 (SPEC-HEATMAP-PANEL-001 T6).
//
// useStoreChartData 가 반환한 시리즈(표시 이름 → 타임라인)에서 각 시리즈의 최신 유한값을
// 센서 1개 판독값으로 추출하고(aggregation:'last'), config.sensor_positions 좌표와 결합해
// IDW 보간 입력점을 만든다. 좌표가 없는 센서는 보간 입력에서 제외하되 설정 UI 노출용으로
// 별도 목록에 담는다(AC-E2). DOM 의존이 없어 단위 테스트로 커버한다.
//
// @spec SPEC-HEATMAP-PANEL-001

import type { ChartEntry, StoreSeriesRef } from '../charts/chartChannelTypes';
import type { SensorPosition } from './heatmapConfig';
import { heatmapSensorId } from './sensorIdentity';
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

/** `resolveSensorSeries` 결과 — 조회 결과를 센서 동일성 키 공간으로 옮긴 것. */
export interface SensorSeriesResolution {
  /** 컬럼 순서를 보존한 센서 동일성 키 목록(joinSensorPoints 의 첫 인자로 넘긴다). */
  ids: string[];
  /** 동일성 키 → 타임라인. */
  entriesById: Map<string, ChartEntry[]>;
  /**
   * 컬럼과 config.series 를 1:1 로 정렬할 수 있었는지. false 면 동일성으로 옮기지 못하고
   * 조회 이름 공간을 그대로 쓴다(좌표 매칭 실패 → 전부 미배치로 graceful degrade).
   */
  aligned: boolean;
}

/**
 * 조회 결과(시리즈 표시 이름 기준)를 센서 동일성 키 기준으로 옮긴다.
 *
 * `useStoreChartData` 는 시리즈를 **표시 이름**(alias)으로 키잉하는데, 히트맵의 좌표/마커는
 * 동일성 키로 키잉된다. 두 공간이 다시 어긋나지 않도록 이 함수가 유일한 다리 역할을 한다:
 * 컬럼 수와 config.series 수가 같을 때(=요청 시리즈와 응답 컬럼이 1:1) 인덱스로 짝지어
 * 동일성 키를 붙인다. 히트맵 패널은 조회용 파생 소스의 alias 를 동일성 키로 치환해 넘기므로
 * 표시 이름이 중복되어 타임라인이 병합되는 일도 없다(같은 key 를 공유하는 형제 시리즈가
 * 기본 alias(=key)를 공유하던 문제).
 *
 * 정렬 불가(한 key 가 다중 컬럼으로 확장된 경우)면 조회 이름을 그대로 통과시킨다 — 동일성
 * 키와 맞지 않아 전부 미배치가 되지만 예외 없이 렌더된다(기존 degrade 동작과 동일).
 */
export function resolveSensorSeries(
  seriesNames: string[],
  seriesEntries: Map<string, ChartEntry[]>,
  refs: readonly StoreSeriesRef[],
): SensorSeriesResolution {
  const ids: string[] = [];
  const entriesById = new Map<string, ChartEntry[]>();
  const aligned = seriesNames.length === refs.length;

  seriesNames.forEach((name, j) => {
    const ref = aligned ? refs[j] : undefined;
    const id = ref ? heatmapSensorId(ref) : name;
    ids.push(id);
    const entries = seriesEntries.get(name);
    if (entries && !entriesById.has(id)) entriesById.set(id, entries);
  });

  return { ids, entriesById, aligned };
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
 * 첫 인자는 좌표 맵과 **같은 키 공간**이어야 한다 — 히트맵 패널은 `resolveSensorSeries` 로
 * 얻은 센서 동일성 키를 넘긴다. 반환되는 placed/unplaced 목록도 같은 키 공간이므로 배치
 * 오버레이가 그대로 좌표 쓰기 키로 쓸 수 있다(표시 라벨은 별도 맵으로 전달).
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
