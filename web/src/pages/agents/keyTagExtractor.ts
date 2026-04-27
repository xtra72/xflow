// 시리즈 키 패턴에서 태그를 자동 추출하는 유틸.
//
// 정적 태그가 없는 시리즈에 대해서도 태그 필터링을 가능하게 하기 위함.
// 정적 태그 (서버 제공) 가 우선이며, 정적 태그가 없으면 키 구조 기반으로
// 자동 추출한다.
//
// 지원 패턴:
//   1. InfluxDB 스타일 (콤마 + `=`): "measurement,k1=v1,k2=v2"
//      → { measurement: <첫 세그먼트>, k1: v1, k2: v2 }
//   2. Colon-separated 세그먼트: "indoor:1:room_temp"
//      → { seg0: "indoor", seg1: "1", seg2: "room_temp" }
//   3. Slash-separated 세그먼트: "indoor/1/room_temp"
//      → { seg0: "indoor", seg1: "1", seg2: "room_temp" }
//   4. 위 패턴이 아니면 빈 객체 반환
//
// @spec SPEC-WEB-005

/**
 * 키 패턴에서 태그를 자동 추출한다.
 *
 * 패턴 우선순위:
 *   InfluxDB (콤마 + `=`) → Colon (`:`) → Slash (`/`) → 빈 객체
 *
 * InfluxDB 스타일은 첫 세그먼트를 `measurement` 로,
 * 이후 `k=v` 페어들을 그대로 매핑한다. `=` 가 전혀 없는 콤마 문자열은
 * InfluxDB 패턴으로 인식하지 않고 다음 단계로 fall through 한다.
 */
export function extractTagsFromKey(key: string): Record<string, string> {
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
  // Colon-separated 세그먼트.
  if (key.includes(':')) {
    return Object.fromEntries(
      key.split(':').map((seg, i) => [`seg${i}`, seg]),
    );
  }
  // Slash-separated 세그먼트.
  if (key.includes('/')) {
    return Object.fromEntries(
      key.split('/').map((seg, i) => [`seg${i}`, seg]),
    );
  }
  return {};
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
): { key: string; values: string[] }[] {
  const buckets = new Map<string, Set<string>>();
  for (const k of keys) {
    const tags = staticTags[k] ?? extractTagsFromKey(k);
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
): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const k of keys) {
    out[k] = staticTags[k] ?? extractTagsFromKey(k);
  }
  return out;
}
