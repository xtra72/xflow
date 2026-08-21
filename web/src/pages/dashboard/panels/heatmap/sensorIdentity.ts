// 히트맵 센서 동일성(identity) 키 + 하위호환 마이그레이션.
//
// 배경(결함): 히트맵의 센서 좌표(`sensor_positions`)와 마커 매칭은 원래 store **key** 하나로
// 키잉되어 있었다. 그러나 시리즈의 동일성은 `storeSeriesId(key, field, tags)` 이고,
// 백엔드 `GET /keys` 는 같은 key 에 대해 metric/tags 조합마다 별도의 시리즈 행을 돌려준다.
// 따라서 한 key 를 공유하는 N 개의 시리즈가:
//   1) 같은 좌표 한 칸을 공유하고(하나를 옮기면 전부 따라 움직임),
//   2) 그 중 하나를 체크 해제하면 공유 항목이 삭제되어 **아직 체크된 형제가 미배치**가 되고
//      (배치 좌표 0개 → 히트맵이 아무것도 렌더하지 않음 = 보고된 "시리즈값 적용 안됨"),
//   3) 표시 이름(alias)을 바꾸면 조회 시리즈 이름은 alias 를 따라가지만 좌표는 raw key 로
//      남아 매칭이 끊긴다.
//
// 해결: 좌표/마커/배치 매칭의 키를 **시리즈 동일성 키**(`heatmapSensorId`)로 재키잉한다.
// alias 는 사용자가 편집하는 표시 라벨일 뿐이므로 절대 키로 쓰지 않는다(위 3번의 원인).
//
// @spec SPEC-HEATMAP-PANEL-001 (센서 동일성 재키잉)

import {
  storeSeriesId,
  storeSeriesLabel,
  type StoreSeriesRef,
} from '../charts/chartChannelTypes';
import type { SensorPosition } from './heatmapConfig';

/** 동일성 키 계산에 필요한 최소 시리즈 형상. */
type IdentityRef = Pick<StoreSeriesRef, 'key' | 'field' | 'tags'>;

/**
 * 센서(시리즈) 동일성 키. `sensor_positions` 의 맵 키이자 마커/배치 매칭 기준이다.
 *
 * `storeSeriesId` 를 그대로 재사용한다 — 이미 선택 체크박스(추가/제거/중복판정)의 동일성
 * 기준이므로 "체크된 것 == 좌표를 가진 것" 이 정의상 일치한다. 태그는 키 정렬 후 직렬화되어
 * 같은 태그 집합이면 항상 같은 문자열이 나온다(삽입 순서 무관).
 */
export function heatmapSensorId(ref: IdentityRef): string {
  return storeSeriesId(ref.key, ref.field ?? '', ref.tags ?? {});
}

/**
 * 마커/칩에 표시할 사람이 읽는 라벨.
 *
 * 사용자가 붙인 이름(alias)이 있으면 그 이름, 없으면 시리즈를 실제로 구분하는 서술 표기
 * (`key · metric{k=v}`)를 쓴다 — key 만 쓰면 한 key 를 metric/tags 로 나눠 갖는 형제 센서들이
 * 마커에서 같은 글자로 보여 어느 점이 어느 센서인지 알 수 없다(보고된 결함).
 *
 * 규칙 자체는 설정 목록과 공유한다(`storeSeriesLabel`) — 같은 센서가 목록과 마커에서 서로
 * 다르게 불리면 안 된다. 표시 전용이며 매칭에는 절대 쓰지 않는다(이름을 바꿔도 배치 유지).
 */
export function sensorSeriesLabel(
  ref: Pick<StoreSeriesRef, 'key' | 'field' | 'tags' | 'alias'>,
  nameFormat?: string,
): string {
  return storeSeriesLabel(ref, nameFormat);
}

/** `migrateSensorPositions` 결과. */
export interface SensorPositionMigration {
  /** 동일성 키로 재키잉된 좌표 맵. */
  positions: Record<string, SensorPosition>;
  /** 입력과 달라졌는지(= raw key 항목이 하나라도 이관/제거됨). 멱등 실행 시 false. */
  changed: boolean;
  /**
   * 모호하게 해석된 raw key 목록 — 체크된 시리즈 2개 이상이 그 key 를 공유해서, 옛 좌표가
   * 어느 센서의 것이었는지 데이터만으로는 알 수 없는 경우다. config 순서상 **첫 번째**
   * 시리즈로 결정적으로 귀속시킨다(아래 주석 참조).
   */
  ambiguousKeys: string[];
}

/**
 * raw key 로 키잉된 기존 `sensor_positions` 를 동일성 키로 이관한다(하위호환).
 *
 * 규칙:
 *   - **이미 이관됨**: 키가 현재 체크된 시리즈의 동일성 키면 그대로 둔다 → 멱등성의 근거.
 *   - **명확(unambiguous)**: 그 key 를 가진 체크된 시리즈가 정확히 1개 → 그 동일성 키로 옮긴다.
 *   - **모호(ambiguous)**: 2개 이상이 그 key 를 공유 → 옛 데이터에는 어느 센서의 좌표였는지
 *     정보가 없다. 무작위로 고르지 않고 **config.series 순서상 첫 시리즈**에 결정적으로
 *     귀속시키고 `ambiguousKeys` 에 기록한다. 버리지 않는 이유: 버리면 기존 다중 시리즈
 *     히트맵이 전부 빈 화면이 되어 지금보다 나빠진다. 반면 옛 동작에서도 그 key 를 공유하는
 *     시리즈들은 **한 점**으로 합쳐져 렌더됐으므로(같은 좌표 + 같은 표시 이름으로 타임라인
 *     병합), 첫 시리즈로 귀속시키는 편이 기존 렌더 결과를 보존한다. 나머지 형제는 미배치가
 *     되며 패널의 "미배치 N" 안내로 노출되어 사용자가 배치할 수 있다.
 *   - **고아(orphan)**: 어느 체크된 시리즈와도 key 가 맞지 않는 좌표 → **그대로 보존**한다.
 *     렌더(joinSensorPoints)와 마커(boundIds) 모두 체크된 시리즈만 보므로 무해하고, 나중에
 *     그 key 를 다시 체크하면 이 마이그레이션이 다시 집어 올릴 수 있다. 삭제하면 되돌릴 수
 *     없는 데이터 손실인데 얻는 것이 없다.
 *
 * 이관 대상 동일성 키에 이미 값이 있으면(신규 경로에서 저장된 좌표) 그 값을 우선하고 raw
 * 항목은 버린다 — 입력 객체의 키 순서와 무관하게 결과가 같도록 2-pass 로 처리한다.
 */
export function migrateSensorPositions(
  positions: Record<string, SensorPosition>,
  series: readonly StoreSeriesRef[] | undefined,
): SensorPositionMigration {
  const refs = Array.isArray(series) ? series : [];
  const ids = refs.map((r) => heatmapSensorId(r));
  const idSet = new Set(ids);
  // raw key → 동일성 키 후보(config.series 순서 보존).
  const byKey = new Map<string, string[]>();
  refs.forEach((ref, i) => {
    const list = byKey.get(ref.key);
    if (list) list.push(ids[i]!);
    else byKey.set(ref.key, [ids[i]!]);
  });

  const out: Record<string, SensorPosition> = {};
  const ambiguousKeys: string[] = [];
  let changed = false;

  // 1-pass: 이미 동일성 키인 항목 + 고아 항목을 먼저 확정한다(이들이 권위 있는 값).
  for (const [k, pos] of Object.entries(positions)) {
    if (idSet.has(k) || !byKey.has(k)) out[k] = pos;
  }
  // 2-pass: 남은 raw key 항목을 동일성 키로 이관한다(대상이 비어 있을 때만).
  for (const [k, pos] of Object.entries(positions)) {
    if (idSet.has(k) || !byKey.has(k)) continue;
    const candidates = byKey.get(k)!;
    if (candidates.length > 1) ambiguousKeys.push(k);
    const target = candidates[0]!;
    if (out[target] === undefined) out[target] = pos;
    changed = true;
  }

  return { positions: changed ? out : positions, changed, ambiguousKeys };
}
