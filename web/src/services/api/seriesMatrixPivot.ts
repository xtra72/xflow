// 시리즈 매트릭스 피벗 — 소스 중립 공용 기계.
//
// "요청 1건 = 시리즈 1개, 응답은 평탄한 엔트리 배열, 클라이언트가 라벨로 그룹화하고
// 버킷 합집합으로 피벗한다" 파이프라인의 뒷단이다. 컬럼 구성(요청 순서 평탄화 ·
// 중복 키 라벨 강제 · 0행 자리 보존)과 버킷 합집합 피벗은 데이터가 어디서 왔는지
// 알지 못하므로, Store 와 외부 TSDB 가 같은 코드를 쓴다.
//
// **피벗 기계는 이 모듈 1벌뿐이다.** TSDB 전용 사본을 만드는 것은 금지된다
// (spec.md §2.16 #25 · UB1-25). 소스별로 다른 것은 "키별로 시리즈를 어떻게
// 가져오는가" 하나뿐이며, 그것을 `KeySeriesFetcher` 로 주입받는다.
//
// 주입 단위가 키 1개가 아니라 키 배열 전체인 이유: 병렬 수집 정책이 소스마다
// 다르다. Store 는 `Promise.all`(개별 실패 = 전체 실패)이고, 외부 TSDB 는
// `Promise.allSettled`(부분 실패 격리, spec.md §2.19)다. 그 정책까지 이 모듈이
// 정하면 소스 중립성이 깨진다.
//
// @spec SPEC-TSDB-002 §4.3 (UB1-25)

import { seriesDisplayName } from './seriesLabels';
import type { SeriesMatrix, SeriesMatrixQuery } from './seriesDataSource';

/**
 * 피벗 입력이 되는 시리즈 하나 — 라벨과 버킷 맵만 있으면 된다.
 *
 * 소스별 페처는 이보다 많은 필드를 가진 타입(예: store 의 `KeySeries` 는 서명을
 * 함께 갖는다)을 돌려줘도 된다. 피벗이 읽는 것은 이 두 필드뿐이다.
 */
export interface PivotKeySeries {
  /** 원본 labels 맵(metric `__field__` + tags). 라벨 없는 단일 시리즈는 undefined. */
  labels: Record<string, string> | undefined;
  /** 버킷 시작 시각 → 집계값. */
  buckets: Map<number, number>;
}

/**
 * 키별 시리즈 페처 — 이 모듈의 유일한 주입 지점.
 *
 * 반환 배열의 인덱스는 `params.keys` 의 인덱스와 1:1 로 대응해야 한다. 같은 key 가
 * 시리즈별 선택으로 여러 인덱스에 중복될 수 있으므로 key 가 아니라 **인덱스**가
 * 대응 축이다. 어떤 인덱스의 조회가 결과 없이 끝나면 빈 배열을 둔다.
 */
export type KeySeriesFetcher = (
  params: SeriesMatrixQuery,
) => Promise<Array<PivotKeySeries[] | undefined>>;

/**
 * 키별 조회 결과를 하나의 매트릭스로 병합한다.
 *
 * - 입력 검증(키 0개 · 시간 역전 · 비양수 인터벌)을 페처 호출 **전에** 수행한다.
 *   키가 0개면 페처를 호출하지 않고 빈 매트릭스를 돌려준다.
 * - 컬럼 순서는 요청 key 순서 → 각 key 안에서 시리즈 등장 순서를 보존한다.
 * - 한 key 의 시리즈가 2개 이상이거나 같은 key 가 여러 인덱스로 중복 요청되면
 *   라벨 표기를 덧붙여 컬럼명을 구분한다.
 * - 데이터가 전혀 없는 key 도 단일 컬럼(전부 null)으로 노출한다. 인덱스 기반
 *   색상·라벨 매칭이 이 자리 보존에 의존한다.
 * - 행은 전체 버킷 시작 시각의 합집합을 오름차순 정렬해 만들고, 해당 컬럼에 값이
 *   없으면 null 을 채운다.
 */
export async function buildSeriesMatrix(
  params: SeriesMatrixQuery,
  fetchAllKeySeries: KeySeriesFetcher,
): Promise<SeriesMatrix> {
  if (params.keys.length === 0) {
    return { columns: [], rows: [] };
  }
  if (params.endMs <= params.startMs) {
    throw new Error('종료 시각은 시작 시각 이후여야 합니다');
  }
  if (!Number.isFinite(params.intervalMs) || params.intervalMs <= 0) {
    throw new Error('인터벌은 양수여야 합니다');
  }

  // 키별 시리즈 배열. 같은 인덱스의 결과가 같은 요청 (key, seriesFilters[idx]) 에
  // 대응한다. 병렬 수집 정책은 페처가 소유한다.
  const perKeySeries = await fetchAllKeySeries(params);

  // 같은 key 가 몇 번 요청되었는지 — 시리즈별 선택으로 한 key 가 여러 인덱스에
  // 나뉘어 오면 컬럼명이 충돌하므로 라벨 표기를 강제한다.
  const keyRequestCount = new Map<string, number>();
  for (const key of params.keys) {
    keyRequestCount.set(key, (keyRequestCount.get(key) ?? 0) + 1);
  }

  // 요청 순서를 보존하며 모든 시리즈를 컬럼으로 평탄화한다.
  const columns: string[] = [];
  const columnBuckets: Array<Map<number, number>> = [];
  // 컬럼 j 가 어느 요청 인덱스에서 나왔는지 함께 기록한다(SPEC-TSDB-004 §4.1).
  // group by 는 요청 1건이 컬럼 N개를 만들므로 위치 대응이 깨지며, 소비자가
  // 표시 메타데이터를 귀속시키려면 이 축이 필요하다.
  const columnOrigins: number[] = [];
  const columnLabels: Array<Record<string, string> | undefined> = [];
  params.keys.forEach((key, idx) => {
    const seriesList = perKeySeries[idx] ?? [];
    // 데이터가 전혀 없는 key 도 단일 컬럼(전부 null)으로 노출해 기존 동작을 보존한다.
    if (seriesList.length === 0) {
      columns.push(key);
      columnBuckets.push(new Map<number, number>());
      columnOrigins.push(idx);
      columnLabels.push(undefined);
      return;
    }
    const withLabel = seriesList.length > 1 || (keyRequestCount.get(key) ?? 0) > 1;
    for (const series of seriesList) {
      columns.push(seriesDisplayName(key, series.labels, withLabel));
      columnBuckets.push(series.buckets);
      columnOrigins.push(idx);
      columnLabels.push(series.labels);
    }
  });

  // 전체 버킷 시작 시각의 합집합을 수집하고 정렬한다.
  const allBuckets = new Set<number>();
  for (const m of columnBuckets) {
    for (const ts of m.keys()) allBuckets.add(ts);
  }
  const sortedBuckets = Array.from(allBuckets).sort((a, b) => a - b);

  // 각 버킷에 대해 컬럼 순서대로 값을 배치한다 (없으면 null).
  const rows: SeriesMatrix['rows'] = sortedBuckets.map((bucketStartMs) => ({
    bucketStartMs,
    values: columnBuckets.map((m) => {
      const v = m.get(bucketStartMs);
      return v === undefined ? null : v;
    }),
  }));

  return {
    columns,
    rows,
    columnOrigins,
    columnLabels,
  };
}
