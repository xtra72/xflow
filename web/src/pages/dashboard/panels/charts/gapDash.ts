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
//
// **결측은 두 모습으로 온다.** 빈 구간 처리를 `비우기(null)` 로 두면 행은 있고 값만
// null 이지만, 기본값인 `채우지 않음` 이면 서버가 빈 버킷을 **아예 보내지 않아 행이
// 통째로 없다**. 후자는 null 을 세는 방식으로는 하나도 찾을 수 없다 — 그래서 버킷
// 간격을 받아 **시간 차이**로 빠진 개수를 센다.

/** 덧그림 시리즈 키의 접두사. 원래 시리즈 키와 절대 겹치지 않아야 한다. */
export const GAP_SERIES_PREFIX = '__gap__';

/** 원래 시리즈 키에 대응하는 덧그림 키. */
export function gapSeriesKey(key: string): string {
  return `${GAP_SERIES_PREFIX}${key}`;
}

/** 덧그림 라인의 점선 패턴. 사용자가 고른 선 모양과 구분되도록 고정한다. */
export const GAP_DASHARRAY = '3 4';

/** 경계 점 시리즈 키의 접두사. 선 덧그림 키와도 원래 키와도 겹치지 않아야 한다. */
export const GAP_DOT_PREFIX = '__gapdot__';

/**
 * 원래 시리즈 키에 대응하는 **경계 점** 키.
 *
 * 실선이 끊기고 점선이 시작되는 자리 — 마지막 실측과 다음 실측 — 에만 값이 있다.
 * 그 두 점은 잰 값이므로 점을 찍어도 거짓이 아니고, 어디까지가 측정이고 어디부터가
 * 이은 것인지 경계를 눈으로 짚어 준다. 구간 내부에는 찍지 않는다 — 그 자리의 값은
 * 보간이라 점을 찍으면 잰 것처럼 보인다.
 */
export function gapDotSeriesKey(key: string): string {
  return `${GAP_DOT_PREFIX}${key}`;
}

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
 * @param threshold  점선으로 표기할 **최소 연속 결측 개수**. 0 이하면 아무것도 하지 않는다.
 * @param intervalMs 버킷 간격(ms). 0 이면 행 인덱스로 세고(행이 남아 있는 결측만
 *                   보인다), 양수이면 시간 차이로 세어 **행이 없는 결측**까지 찾는다.
 * @returns         덧그림 키가 추가된 새 행 배열과, 실제로 덧그림이 생긴 키 목록.
 *
 * 양쪽 끝(맨 앞·맨 뒤)의 결측은 이을 상대가 없으므로 건너뛴다 — 없는 값을 향해
 * 선을 뻗으면 그것이야말로 지어낸 데이터다.
 */
export function buildGapOverlay(
  rows: readonly Row[],
  keys: readonly string[],
  threshold: number,
  intervalMs = 0,
): { rows: Row[]; gapKeys: string[] } {
  if (threshold <= 0 || rows.length === 0 || keys.length === 0) {
    return { rows: rows as Row[], gapKeys: [] };
  }
  const out: Row[] = rows.map((r) => ({ ...r }));
  const gapKeys: string[] = [];
  const timeAt = (i: number): number => {
    const t = out[i]?.['timestamp'];
    return typeof t === 'number' ? t : 0;
  };

  for (const key of keys) {
    const gk = gapSeriesKey(key);
    const dk = gapDotSeriesKey(key);
    let touched = false;
    // 값이 있는 위치만 훑고, 이웃한 두 위치 사이에 몇 개가 빠졌는지 센다.
    // 행이 남아 있든(null) 통째로 없든 같은 방식으로 다뤄진다.
    let prev = -1;
    for (let i = 0; i < out.length; i++) {
      const v = numAt(out[i], key);
      if (v === null) continue;
      if (prev >= 0) {
        const left = numAt(out[prev], key)!;
        const missing =
          intervalMs > 0
            ? // 간격을 알면 시간 차이로 센다 — 행이 없는 결측은 이 길로만 보인다.
              Math.max(0, Math.round((timeAt(i) - timeAt(prev)) / intervalMs) - 1)
            : // 간격을 모르면 사이에 남아 있는 행(null) 개수로 센다.
              i - prev - 1;
        if (missing >= threshold) {
          out[prev]![gk] = left;
          out[i]![gk] = v;
          // 경계 점은 **양 끝에만** 찍는다(구간 내부는 보간값이다).
          out[prev]![dk] = left;
          out[i]![dk] = v;
          // 사이에 행이 남아 있으면 보간값을 채운다. 채우지 않으면 결측 구간이
          // 둘 이상일 때 서로 이어져 실측 구간 위에 가짜 점선이 겹친다.
          const steps = i - prev;
          for (let n = prev + 1; n < i; n++) {
            out[n]![gk] = left + ((v - left) * (n - prev)) / steps;
          }
          touched = true;
        }
      }
      prev = i;
    }
    if (touched) gapKeys.push(gk);
  }
  return { rows: out, gapKeys };
}
