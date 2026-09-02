// 표의 **시각 기준 행** 변환 — 긴 형식(long)을 넓은 형식(wide)으로 돌린다.
//
// 종전 표는 엔트리 하나가 행 하나였다. 시리즈가 둘 이상이면 같은 시각의 값들이
// 서로 다른 행에 흩어져, 같은 순간의 온도와 습도를 나란히 볼 수 없었다. 행을 눈으로
// 짝지어야 하는 표는 표가 아니라 로그다.
//
// 시각 기준 행은 그 축을 돌린다 — 행 하나가 한 시각이고, 시리즈마다 열이 하나다.
//
// **정렬·필터·자릿수는 그대로다.** 피벗 결과도 `ChartEntry` 이므로 기존 표 파이프라인
// (`applyTableColumnFilters` · `sortEntries` · `formatCell`)이 손대지 않고 그대로 돈다.
// 시리즈 값은 `$.series.<이름>` 경로로 읽는다 — 태그 열(`$.tags.<키>`)과 같은 방식이다.

import type { ChartEntry, TableColumn } from './chartChannelTypes';

/** 시리즈 열의 필드 접두. 태그 열 접두와 같은 규약이다. */
export const SERIES_FIELD_PREFIX = '$.series.';

/** 시리즈 이름을 표의 열 필드 표기로 바꾼다. */
export function seriesFieldPath(seriesName: string): string {
  return `${SERIES_FIELD_PREFIX}${seriesName}`;
}

/** 열 필드가 시리즈 열이면 그 시리즈 이름을, 아니면 `undefined`. */
export function seriesNameOfField(field: string): string | undefined {
  if (!field.startsWith(SERIES_FIELD_PREFIX)) return undefined;
  const name = field.slice(SERIES_FIELD_PREFIX.length);
  return name === '' ? undefined : name;
}

/**
 * 피벗 행. `ChartEntry` 를 그대로 만족하되 `series` 칸을 하나 더 갖는다.
 *
 * `value` 는 남겨 두지 않는다(undefined) — 시리즈가 여럿인 행에서 "그 행의 값"이라는
 * 것은 없고, 하나를 골라 넣으면 어느 시리즈인지 모르는 수가 셀에 앉는다.
 */
export interface PivotRow extends ChartEntry {
  series: Record<string, number | null>;
}

/**
 * 엔트리가 속한 시리즈 이름.
 *
 * store·tsdb 경로는 `meta.seriesName` 을 싣고(그 값이 범례·라인 이름과 같은 정본),
 * 채널 경로는 `labels.name` 만 갖는다. 둘 다 없으면 이름 없는 한 줄로 본다 —
 * 시리즈 축이 없는 소스에서도 시각 기준 행은 여전히 뜻이 있다(행 = 시각).
 */
export function entrySeriesName(entry: ChartEntry, fallback: string): string {
  const meta = entry.meta?.seriesName;
  if (typeof meta === 'string' && meta !== '') return meta;
  const label = entry.labels?.name;
  if (typeof label === 'string' && label !== '') return label;
  return fallback;
}

/** 수로 볼 수 있으면 수를, 아니면 null. 셀은 빈칸이 된다. */
function toNumber(value: unknown): number | null {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  if (typeof value === 'string' && value.trim() !== '') {
    const n = Number(value);
    return Number.isFinite(n) ? n : null;
  }
  if (typeof value === 'boolean') return value ? 1 : 0;
  return null;
}

/**
 * 엔트리 목록을 시각 기준 행으로 접는다.
 *
 * - 행 키는 `timestamp` **정확히 같은 값**이다. store·tsdb 소스는 같은 버킷 시작
 *   시각을 시리즈마다 똑같이 싣기 때문에 행이 저절로 맞는다. 시각이 어긋나는 소스
 *   (채널 스트림 등)에서는 어긋난 만큼 행이 나뉘고 나머지 열이 빈칸이 된다 —
 *   임의의 허용 오차로 묶으면 실제로 다른 시각의 값이 한 행에 앉는다.
 * - 같은 시각·같은 시리즈에 값이 둘 이상이면 **마지막 값**이 남는다(뒤에 온 것이 최신).
 * - 행은 시각 오름차순이다. 다른 순서가 필요하면 헤더를 눌러 정렬한다.
 *
 * @returns `rows` 시각 오름차순 피벗 행, `seriesNames` 등장 순서를 지킨 시리즈 이름
 */
export function pivotByTimestamp(
  entries: readonly ChartEntry[],
  fallbackSeriesName: string,
): { rows: PivotRow[]; seriesNames: string[] } {
  const byTimestamp = new Map<number, PivotRow>();
  const order: string[] = [];
  const seen = new Set<string>();

  for (const e of entries) {
    if (typeof e.timestamp !== 'number' || !Number.isFinite(e.timestamp)) continue;
    const name = entrySeriesName(e, fallbackSeriesName);
    if (!seen.has(name)) {
      seen.add(name);
      order.push(name);
    }
    let row = byTimestamp.get(e.timestamp);
    if (!row) {
      row = { timestamp: e.timestamp, value: undefined, series: {} };
      byTimestamp.set(e.timestamp, row);
    }
    row.series[name] = toNumber(e.value);
  }

  const rows = Array.from(byTimestamp.values()).sort((a, b) => a.timestamp - b.timestamp);
  return { rows, seriesNames: order };
}

/**
 * 피벗 행에서 시리즈 값을 읽는다. 그 시각에 값이 없던 시리즈는 `undefined` —
 * `null`(값이 비어 있음)과 구분되지만, 셀 표기는 둘 다 빈칸이라 화면에서는 같다.
 */
export function resolveSeriesValue(entry: ChartEntry, seriesName: string): unknown {
  const series = (entry as Partial<PivotRow>).series;
  if (!series) return undefined;
  return series[seriesName] ?? undefined;
}

/** 시각 기준 행에서 쓰는 시각 열의 필드. 긴 형식과 같은 경로다. */
const TIME_FIELD = 'timestamp';

/**
 * 시각 기준 행의 **열 목록을 데이터에서 만든다**.
 *
 * 시리즈는 소스 설정과 데이터에 따라 늘고 준다. 그것을 사용자가 손으로 열에 옮겨
 * 적게 하면 시리즈를 하나 추가할 때마다 표 설정도 따라 고쳐야 한다 — 그래서 열은
 * 파생값이다.
 *
 * 다만 **폭과 이름은 사용자 것**이다. 이미 저장된 `columns` 에 같은 필드가 있으면
 * 그 열의 `width` · `header` · `sortable` · `filterable` 을 물려받는다. 덕분에 헤더
 * 경계 드래그(폭 조절)가 시각 기준 행에서도 그대로 동작하고, 사라진 시리즈의 열은
 * 물려줄 자리가 없으므로 저절로 빠진다.
 *
 * @param seriesNames 열로 펼칠 시리즈 이름(등장 순서)
 * @param configured  저장된 열 설정 — 폭·이름 override 의 출처
 * @param timeHeader  시각 열 이름(미지정이면 호출부의 기본 문구)
 * @param unit        시리즈 열 공통 단위. 시리즈마다 다른 단위를 섞을 수 없으므로
 *                    패널 하나에 하나다(섞어야 한다면 표를 나누는 편이 읽힌다).
 */
export function pivotColumns(
  seriesNames: readonly string[],
  configured: readonly TableColumn[],
  timeHeader: string,
  unit?: string,
): TableColumn[] {
  const byField = new Map(configured.map((c) => [c.field, c]));

  const savedTime = byField.get(TIME_FIELD);
  const time: TableColumn = {
    ...savedTime,
    field: TIME_FIELD,
    header: timeHeader,
    // 시각 열의 형식은 고정이다 — 시각을 수로 찍으면 epoch ms 원값이 나온다.
    format: 'datetime',
  };

  const series = seriesNames.map((name): TableColumn => {
    const saved = byField.get(seriesFieldPath(name));
    return {
      ...saved,
      field: seriesFieldPath(name),
      // 이름을 직접 고쳤으면 그 이름이 이긴다. 고치지 않았으면 시리즈 이름 그대로다.
      header: saved?.header && saved.header !== '' ? saved.header : name,
      format: 'number',
      unit,
    };
  });

  return [time, ...series];
}
