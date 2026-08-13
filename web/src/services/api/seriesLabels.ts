// 시리즈 라벨(labels) 파싱·정규화·표시 유틸.
//
// SPEC-STORE-004 (M5): 백엔드 `POST /api/v1/store/{name}/query` 응답의 각 엔트리는
// `labels` 맵을 포함한다. 예약 키 `__metric__` 은 metric_type 을 의미하고, 그 외 키는
// tag key=value 이다. 같은 store key 라도 metric/tags 가 다른 다중 시리즈가 한 응답에
// 섞여 올 수 있으므로(labels 로 구분), 프론트엔드는 labels 기준으로 시리즈를 분리해야 한다.
//
// 본 모듈은 다음을 제공한다:
//   - METRIC_LABEL_KEY: 예약 metric 라벨 키 상수.
//   - parseSeriesLabels: labels 맵을 {metric, tags} 로 분해.
//   - seriesSignature: 같은 시리즈를 식별하는 결정적(정렬된) 서명 문자열.
//   - formatSeriesLabel: 사람이 읽기 좋은 라벨 표기(예: "metric{room=1, type=temp}").
//   - seriesDisplayName: store key + labels 를 합쳐 매트릭스 컬럼/차트 라인 이름을 만든다.
//
// 설계 메모: labels 가 비어있거나(undefined) `__metric__` 만 있는 단일 시리즈는
// 기존 동작(키 단위 단일 컬럼)과의 호환을 위해 별도 표기 없이 store key 만 사용한다.
// 다중 시리즈가 한 key 에서 발생할 때만 라벨 표기를 덧붙여 라인/컬럼을 구분한다.
//
// @spec SPEC-STORE-004

/** 백엔드가 metric_type 을 담는 예약 라벨 키. (Go: seriesLabels) */
export const METRIC_LABEL_KEY = '__metric__';

/** 시리즈 ID 의 `key` 와 서명 구분자 — 키/라벨에 등장하지 않는 NUL. */
export const SERIES_ID_SEPARATOR = String.fromCharCode(0);

/** labels 맵을 metric 과 tags 로 분해한 결과. */
export interface ParsedSeriesLabels {
  /** `__metric__` 값. 없으면 빈 문자열. */
  metric: string;
  /** `__metric__` 을 제외한 나머지 tag key=value 맵. */
  tags: Record<string, string>;
}

/**
 * labels 맵을 {metric, tags} 로 분해한다.
 *
 * - `__metric__` 예약 키를 metric 으로 추출한다 (없으면 빈 문자열).
 * - 그 외 키는 모두 tag 로 분류한다.
 * - 입력이 undefined/null 이면 metric="" + 빈 tags 를 반환한다.
 */
export function parseSeriesLabels(
  labels: Record<string, string> | undefined | null,
): ParsedSeriesLabels {
  if (!labels) return { metric: '', tags: {} };
  const tags: Record<string, string> = {};
  let metric = '';
  for (const [k, v] of Object.entries(labels)) {
    if (k === METRIC_LABEL_KEY) {
      metric = v;
    } else {
      tags[k] = v;
    }
  }
  return { metric, tags };
}

/**
 * 시리즈를 식별하는 결정적 서명 문자열을 만든다.
 *
 * tag key 를 사전순으로 정렬해 `metric|k1=v1,k2=v2` 형태로 직렬화한다.
 * 같은 metric/tags 조합은 항상 동일한 서명을 갖는다(렌더/병합 안정성 보장).
 *
 * labels 가 비어있으면 빈 문자열("")을 반환한다 — 이는 "라벨 없는 단일 시리즈"
 * 를 의미하며, 호출자는 이를 기존 키 단위 단일 컬럼으로 취급한다.
 */
export function seriesSignature(
  labels: Record<string, string> | undefined | null,
): string {
  if (!labels) return '';
  const keys = Object.keys(labels);
  if (keys.length === 0) return '';
  const { metric, tags } = parseSeriesLabels(labels);
  const tagKeys = Object.keys(tags).sort();
  const tagPart = tagKeys.map((k) => `${k}=${tags[k]}`).join(',');
  return `${metric}|${tagPart}`;
}

/**
 * (key + metric_type + tags) 로부터 결정적 시리즈 식별자를 만든다.
 *
 * 저장소 기준 분류(SeriesID = key + metric + tags)와 동일한 단위로, 시리즈별 선택
 * 상태의 키로 사용한다. `key` 와 `seriesSignature` 사이를 NUL()로 구분해
 * 키/라벨에 등장하지 않는 경계를 보장한다.
 *
 * metric 이 비어있고 tags 도 없으면 서명은 ""(라벨 없는 단일 시리즈)이 되어
 * `${key}` 형태가 된다.
 */
export function makeSeriesId(
  key: string,
  metric: string | undefined,
  tags: Record<string, string> | undefined,
): string {
  const labels: Record<string, string> = { ...(tags ?? {}) };
  if (metric) labels[METRIC_LABEL_KEY] = metric;
  return `${key}${SERIES_ID_SEPARATOR}${seriesSignature(labels)}`;
}

/**
 * (key + metric_type + tags) 로부터 사람이 읽는 시리즈 표시 이름을 만든다.
 *
 * `makeSeriesId` 와 같은 입력(시리즈 동일성 3요소)을 받되, 기계용 식별자가 아니라
 * 매트릭스 컬럼과 **같은 표기**(`key · metric{k=v, ...}`)를 돌려준다. 시리즈 선택 UI 나
 * 히트맵 마커처럼 ref(key/metric_type/tags)만 가진 자리에서, 컬럼명과 다른 두 번째 표기를
 * 만들지 않도록 `seriesDisplayName` 에 위임한다.
 *
 * metric 과 tags 가 모두 없으면 라벨이 빈 문자열이 되어 `key` 만 반환된다 — 구분자(`·`)가
 * 매달린 채 남지 않는다(`storeSeriesId` 의 기계용 형식과 달리 후행 공백도 없다).
 */
export function seriesRefDisplayName(
  key: string,
  metric: string | undefined,
  tags: Record<string, string> | undefined,
): string {
  const labels: Record<string, string> = { ...(tags ?? {}) };
  if (metric) labels[METRIC_LABEL_KEY] = metric;
  return seriesDisplayName(key, labels, true);
}

/**
 * metric/tags 를 사람이 읽기 좋은 라벨 문자열로 포맷한다.
 *
 * 형식 규칙:
 *   - metric 만 있고 tags 없음   → `metric`
 *   - metric + tags             → `metric{k1=v1, k2=v2}` (tag key 사전순)
 *   - metric 없고 tags 만 있음   → `{k1=v1, k2=v2}`
 *   - 둘 다 없음                 → 빈 문자열
 */
export function formatSeriesLabel(parsed: ParsedSeriesLabels): string {
  const tagKeys = Object.keys(parsed.tags).sort();
  const tagPart =
    tagKeys.length > 0
      ? `{${tagKeys.map((k) => `${k}=${parsed.tags[k]}`).join(', ')}}`
      : '';
  if (parsed.metric && tagPart) return `${parsed.metric}${tagPart}`;
  if (parsed.metric) return parsed.metric;
  return tagPart;
}

/**
 * store key 와 labels 를 합쳐 매트릭스 컬럼/차트 라인 표시 이름을 만든다.
 *
 * - `withLabel === false` (또는 라벨이 비어있음): store key 만 반환한다.
 *   동일 key 에서 시리즈가 1개뿐일 때 사용하여 기존 동작(단순 키 표기)을 보존한다.
 * - `withLabel === true`: `key · 라벨` 형태로 라벨을 덧붙여 다중 시리즈를 구분한다.
 *   라벨 포맷이 빈 문자열이면 key 만 반환한다(라벨이 사실상 없는 경우).
 *
 * 구분자(`·`, U+00B7)는 store key 에 거의 등장하지 않아 시각적 분리에 안전하다.
 */
export function seriesDisplayName(
  key: string,
  labels: Record<string, string> | undefined | null,
  withLabel: boolean,
): string {
  if (!withLabel) return key;
  const label = formatSeriesLabel(parseSeriesLabels(labels));
  return label ? `${key} · ${label}` : key;
}
