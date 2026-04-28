// 라인 차트의 결측값(null) 처리 유틸.
// `TsdbResultChart` 가 사용하는 변환 함수 + 모드 타입 정의를 모은다.
// 컴포넌트 파일과 분리한 이유: 순수 함수를 별도 파일로 두어 단위 테스트가 가능하고,
// `react-refresh/only-export-components` 규칙 충돌을 피한다.
//
// @spec SPEC-WEB-005

/**
 * 결측값 (null) 처리 모드.
 *
 * - `gap`: 라인을 끊는다 (기본, 누락 구간이 시각적으로 드러남).
 * - `previous`: 마지막으로 알려진 값으로 forward-fill (계단형 효과).
 * - `value`: 사용자 지정 상수로 대체.
 * - `interpolate`: 이전·다음 알려진 값 사이를 선형 보간 (이전 값에서 추론).
 *   양쪽이 모두 알려져 있을 때만 보간하며, 한쪽만 있으면 forward-fill 로 대체된다.
 */
export type NullHandlingMode = 'gap' | 'previous' | 'value' | 'interpolate';

/** 차트 데이터의 단일 행 형상. bucketStartMs 외에 컬럼별 값을 가진다. */
export type ChartRow = Record<string, number | null>;

/**
 * 컬럼별로 null 처리 변환을 적용한 새 데이터 배열을 반환한다.
 *
 * 입력 데이터는 변경하지 않으며, 각 행을 얕게 복제한 후 컬럼 값만 갱신한다.
 *
 * - `gap`: 변환 없이 그대로 반환.
 * - `value`: null 을 `fillValue` 로 대체.
 * - `previous`: 컬럼별 마지막 알려진 값으로 forward-fill.
 *   초기에 알려진 값이 없으면 null 을 유지한다 (시작 지점에서는 라인이 끊김).
 * - `interpolate`: 양쪽으로 알려진 값이 있을 때만 인덱스 기반 선형 보간.
 *   한쪽만 알려져 있으면 forward-fill (이전), 또는 leading null 이면 미보간.
 */
export function applyNullHandling(
  rows: ChartRow[],
  columns: readonly string[],
  mode: NullHandlingMode,
  fillValue: number,
): ChartRow[] {
  if (mode === 'gap' || rows.length === 0 || columns.length === 0) {
    return rows;
  }

  // 한 번 얕게 복제해 두면 이후 모든 모드가 안전하게 in-place 수정 가능.
  const out: ChartRow[] = rows.map((r) => ({ ...r }));

  if (mode === 'value') {
    for (const row of out) {
      for (const col of columns) {
        if (row[col] === null || row[col] === undefined) {
          row[col] = fillValue;
        }
      }
    }
    return out;
  }

  if (mode === 'previous') {
    const lastSeen: Record<string, number | undefined> = {};
    for (const row of out) {
      for (const col of columns) {
        const v = row[col];
        if (typeof v === 'number' && Number.isFinite(v)) {
          lastSeen[col] = v;
        } else {
          const prev = lastSeen[col];
          if (prev !== undefined) row[col] = prev;
        }
      }
    }
    return out;
  }

  // mode === 'interpolate'
  for (const col of columns) {
    // 컬럼별 알려진 인덱스 목록 (오름차순).
    const knownIdx: number[] = [];
    for (let i = 0; i < out.length; i++) {
      const v = out[i]?.[col];
      if (typeof v === 'number' && Number.isFinite(v)) knownIdx.push(i);
    }
    if (knownIdx.length === 0) continue;

    let cursor = 0; // knownIdx 내에서 현재 검색 시작 위치
    for (let i = 0; i < out.length; i++) {
      const row = out[i];
      if (!row) continue;
      const v = row[col];
      if (typeof v === 'number' && Number.isFinite(v)) continue;

      // 이전·다음 알려진 인덱스 탐색 (cursor 활용으로 O(N) 보장).
      while (cursor < knownIdx.length && (knownIdx[cursor] ?? -1) <= i) cursor++;
      const nextIdx = knownIdx[cursor];
      const prevIdx = cursor > 0 ? knownIdx[cursor - 1] : undefined;

      if (
        prevIdx !== undefined &&
        nextIdx !== undefined &&
        prevIdx < i &&
        nextIdx > i
      ) {
        const prevVal = out[prevIdx]?.[col] as number;
        const nextVal = out[nextIdx]?.[col] as number;
        const t = (i - prevIdx) / (nextIdx - prevIdx);
        row[col] = prevVal + (nextVal - prevVal) * t;
      } else if (prevIdx !== undefined) {
        // 후방에 알려진 값이 없으면 forward-fill 로 폴백.
        row[col] = out[prevIdx]?.[col] ?? null;
      }
      // leading null (prevIdx 없음) 은 그대로 두어 시작 지점 표시.
    }
  }
  return out;
}
