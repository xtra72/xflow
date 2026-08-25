// 결측 구간 점선 표기 (SPEC-TSDB-004 §2.19).
//
// 라인 차트는 값이 없는 버킷을 `connectNulls` 로 조용히 이어 왔다. 그 결과 3시간
// 정전 구간이 "실제로 그렇게 측정된 직선" 과 똑같이 보인다. 구간을 **점선**으로
// 그리면 이은 것과 잰 것을 눈으로 가를 수 있다.
//
// 구현 방식: 원래 라인은 `connectNulls={false}` 로 끊고, 결측 구간만 담은 **덧그림
// 시리즈**를 하나 더 그린다. 덧그림에는 결측 구간의 양 끝(마지막 실측 · 다음 실측)과
// 그 사이 선형 보간값이 들어가고, 나머지 구간은 null 이라 아무것도 그리지 않는다.
//
// 보간값을 채우는 이유: 양 끝점만 넣고 `connectNulls={true}` 로 이으면, 한 시리즈에
// 결측 구간이 둘 이상일 때 **구간 1의 끝과 구간 2의 시작**까지 이어져 실측 구간 위에
// 가짜 점선이 겹친다. 보간값을 채우고 `connectNulls={false}` 로 두면 각 구간이
// 독립적으로 그려진다.

/** 덧그림 시리즈 키의 접두사. 원래 시리즈 키와 절대 겹치지 않아야 한다. */
export const GAP_SERIES_PREFIX = '__gap__';

/** 원래 시리즈 키에 대응하는 덧그림 키. */
export function gapSeriesKey(key: string): string {
  return `${GAP_SERIES_PREFIX}${key}`;
}

/** 덧그림 라인의 점선 패턴. 사용자가 고른 선 모양과 구분되도록 고정한다. */
export const GAP_DASHARRAY = '3 4';

type Row = Record<string, unknown>;

/** 숫자면 그 값, 아니면 null. `undefined`·문자열·boolean 을 모두 결측으로 본다. */
function numAt(row: Row | undefined, key: string): number | null {
  const v = row?.[key];
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

/**
 * 결측 구간 덧그림 시리즈를 만들어 행에 얹는다.
 *
 * @param rows      차트 행(시간 오름차순). 원본은 변경하지 않는다.
 * @param keys      대상 시리즈 키.
 * @param threshold 점선으로 표기할 **최소 연속 결측 개수**. 0 이하면 아무것도 하지 않는다.
 * @returns         덧그림 키가 추가된 새 행 배열과, 실제로 덧그림이 생긴 키 목록.
 *
 * 양쪽 끝(맨 앞·맨 뒤)의 결측은 이을 상대가 없으므로 건너뛴다 — 없는 값을 향해
 * 선을 뻗으면 그것이야말로 지어낸 데이터다.
 */
export function buildGapOverlay(
  rows: readonly Row[],
  keys: readonly string[],
  threshold: number,
): { rows: Row[]; gapKeys: string[] } {
  if (threshold <= 0 || rows.length === 0 || keys.length === 0) {
    return { rows: rows as Row[], gapKeys: [] };
  }
  const out: Row[] = rows.map((r) => ({ ...r }));
  const gapKeys: string[] = [];

  for (const key of keys) {
    const gk = gapSeriesKey(key);
    let touched = false;
    let i = 0;
    while (i < out.length) {
      if (numAt(out[i], key) !== null) {
        i++;
        continue;
      }
      // 결측 구간 [start, end] 을 찾는다.
      const start = i;
      let end = i;
      while (end + 1 < out.length && numAt(out[end + 1], key) === null) end++;
      i = end + 1;

      const runLength = end - start + 1;
      if (runLength < threshold) continue;
      const left = numAt(out[start - 1], key);
      const right = numAt(out[end + 1], key);
      // 이을 상대가 한쪽이라도 없으면 건너뛴다(선두·말미 결측).
      if (left === null || right === null) continue;

      const steps = runLength + 1; // left → right 사이 구간 수
      out[start - 1]![gk] = left;
      out[end + 1]![gk] = right;
      for (let n = 0; n < runLength; n++) {
        out[start + n]![gk] = left + ((right - left) * (n + 1)) / steps;
      }
      touched = true;
    }
    if (touched) gapKeys.push(gk);
  }
  return { rows: out, gapKeys };
}
