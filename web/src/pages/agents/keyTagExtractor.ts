// 시리즈 키 패턴에서 태그를 자동 추출하는 유틸.
//
// 자동 추출(키 구조 기반) 결과와 정적 태그(서버 제공) 를 **병합** 하여 반환한다.
// 충돌하는 태그 키가 있을 경우 정적 태그가 우선한다.
// 사용자가 변경 가능한 세그먼트 구분자가 모든 키에 일관되게 적용된다.
//
// 지원 패턴:
//   1. InfluxDB 스타일 (콤마 + `=`): "measurement,k1=v1,k2=v2"
//      → { measurement: <첫 세그먼트>, k1: v1, k2: v2 }
//   2. 사용자 지정 세그먼트 구분자 (기본: `:`): "indoor:1:room_temp"
//      → { seg0: "indoor", seg1: "1", seg2: "current_temperature" }
//      구분자가 키에 없으면 자동 추출 결과는 빈 객체. 폴백은 수행하지 않는다.
//
// @spec SPEC-WEB-005

/** 세그먼트 구분자 기본값. */
export const DEFAULT_SEGMENT_SEPARATOR = ':';

/**
 * 키 패턴에서 태그를 자동 추출한다.
 *
 * 우선순위:
 *   1. InfluxDB (콤마 + `=`) — 패턴이 매칭되면 우선 사용.
 *   2. 사용자 지정 구분자 — 키에 포함되어 있으면 분리하여 seg0..segN 으로 매핑.
 *   3. 둘 다 매칭되지 않으면 빈 객체 반환 (폴백 없음).
 *
 * InfluxDB 스타일은 첫 세그먼트를 `measurement` 로,
 * 이후 `k=v` 페어들을 그대로 매핑한다. `=` 가 전혀 없는 콤마 문자열은
 * InfluxDB 패턴으로 인식하지 않고 다음 단계로 fall through 한다.
 *
 * 폴백을 두지 않는 이유: 사용자가 구분자를 바꿔도 결과가 동일하면 변경
 * 인지가 어렵다. 명시적인 빈 결과로 사용자가 키 패턴과 구분자 불일치를
 * 즉시 인식할 수 있게 한다.
 *
 * @param key 시리즈 키
 * @param separator 세그먼트 구분자 (기본: `:`). 빈 문자열이면 자동 추출 스킵.
 */
export function extractTagsFromKey(
  key: string,
  separator: string = DEFAULT_SEGMENT_SEPARATOR,
): Record<string, string> {
  // InfluxDB 스타일: 콤마와 `=` 가 모두 존재해야 한다.
  if (key.includes(',') && key.includes('=')) {
    const parts = key.split(',');
    const tags: Record<string, string> = {};
    if (parts.length > 0 && parts[0]) {
      tags.measurement = parts[0];
    }
    for (let i = 1; i < parts.length; i++) {
      const part = parts[i];
      if (!part) continue;
      const eq = part.indexOf('=');
      if (eq > 0) {
        const k = part.slice(0, eq).trim();
        const v = part.slice(eq + 1).trim();
        if (k && v) {
          tags[k] = v;
        }
      }
    }
    return tags;
  }
  // 사용자 지정 구분자.
  if (separator && key.includes(separator)) {
    return Object.fromEntries(
      key.split(separator).map((seg, i) => [`seg${i}`, seg]),
    );
  }
  return {};
}

/**
 * 자동 추출 태그와 정적 태그를 병합한다. 충돌 시 정적 태그가 우선한다.
 * 정적 태그가 있는 키도 구분자 변경의 영향을 받게 하기 위함.
 */
function mergeTags(
  extracted: Record<string, string>,
  staticForKey: Record<string, string> | undefined,
): Record<string, string> {
  if (!staticForKey || Object.keys(staticForKey).length === 0) {
    return extracted;
  }
  return { ...extracted, ...staticForKey };
}

/**
 * 키 목록과 정적 태그 맵을 받아 통합 태그 페어 목록을 만든다.
 * 정적 태그가 있는 키는 정적 태그 사용, 없으면 `extractTagsFromKey` 결과 사용.
 *
 * 결과는 태그 키 알파벳 순으로 정렬되며, 각 태그 키의 값들도 정렬된다.
 */
export function buildExtractedTagPairs(
  keys: string[],
  staticTags: Record<string, Record<string, string>>,
  separator: string = DEFAULT_SEGMENT_SEPARATOR,
): { key: string; values: string[] }[] {
  const buckets = new Map<string, Set<string>>();
  for (const k of keys) {
    // 자동 추출 + 정적 태그 병합 (정적 태그가 충돌 시 우선).
    // 정적 태그가 있는 키도 구분자 변경의 영향을 받도록 한다.
    const tags = mergeTags(extractTagsFromKey(k, separator), staticTags[k]);
    for (const [tk, tv] of Object.entries(tags)) {
      if (!buckets.has(tk)) {
        buckets.set(tk, new Set());
      }
      buckets.get(tk)!.add(tv);
    }
  }
  return [...buckets.entries()]
    .map(([k, vs]) => ({ key: k, values: [...vs].sort() }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

/**
 * 키 → 태그 맵 빌드.
 *
 * 정적 태그가 있는 키는 정적 태그 그대로 사용, 없으면 자동 추출.
 * `matchesTagFilter` 가 키 단위 매칭에 사용한다.
 */
export function buildExtractedTagsByKey(
  keys: string[],
  staticTags: Record<string, Record<string, string>>,
  separator: string = DEFAULT_SEGMENT_SEPARATOR,
): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const k of keys) {
    // 자동 추출 + 정적 태그 병합. 정적 태그가 있어도 구분자 변경의
    // 영향을 받도록 자동 추출 결과를 베이스로 사용한다.
    out[k] = mergeTags(extractTagsFromKey(k, separator), staticTags[k]);
  }
  return out;
}

/**
 * 키 이름에서 metric_type 후보를 추출한다 (TSDB 모드 자동 메타데이터 추출용).
 *
 * Store 모드는 백엔드가 `StoreKeyObject.metric_type` 을 명시적으로 제공하지만,
 * TSDB 모드는 메타데이터 소스가 없으므로 키 이름의 구조에서 추정해야 한다.
 * 이 함수는 사용자에게 친숙한 두 가지 패턴을 우선순위에 따라 시도한다.
 *
 * 우선순위:
 *   1. InfluxDB 라인 프로토콜 스타일 ("measurement,k=v,k=v") → measurement
 *   2. separator 로 분리된 첫 번째 segment ("seg0:seg1:seg2" with separator=':' → seg0)
 *   3. 추출 실패 시 (separator 미포함, 콤마 없음) 키 전체를 그대로 반환
 *
 * 빈 입력은 빈 문자열을 반환한다 — 호출자가 'unknown' 등 fallback 을 결정.
 *
 * @param key 시리즈 키
 * @param separator 세그먼트 구분자 (예: ':', '/'). 빈 문자열이면 segment 분리 스킵.
 *
 * @spec SPEC-WEB-005 v0.7.0 (Option A)
 */
export function extractMetricTypeFromKey(
  key: string,
  separator: string,
): string {
  if (!key) return '';
  const trimmed = key.trim();
  if (!trimmed) return '';
  // 1. InfluxDB 스타일 — 콤마 앞부분이 measurement.
  //    `=` 동반 여부와 무관하게 콤마 위치만으로 판단한다.
  //    이는 `extractTagsFromKey` 의 InfluxDB 인식보다 느슨하지만,
  //    metric_type 추정에는 첫 토큰이면 충분하다.
  const commaIdx = trimmed.indexOf(',');
  if (commaIdx > 0) {
    const measurement = trimmed.slice(0, commaIdx).trim();
    if (measurement) return measurement;
  }
  // 2. separator 로 분리된 첫 segment.
  if (separator && trimmed.includes(separator)) {
    const segments = trimmed.split(separator);
    const first = segments[0]?.trim();
    if (first) return first;
  }
  // 3. 구조 없음 — 키 전체를 fallback 으로 사용.
  return trimmed;
}
