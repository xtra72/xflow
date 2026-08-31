// 패널 config 에서 **표의 태그 열 후보**를 뽑는 순수 함수.
//
// 표는 `$.tags.<키>` 표기로 시리즈 태그를 열로 쓸 수 있지만, 종전에는 그 키를 손으로
// 적는 것 말고는 방법이 없었다. 어떤 태그가 있는지는 데이터 소스 설정 안에 이미
// 적혀 있는데(필터로 건 태그 · group by 축), 표 편집기는 그것을 읽지 않았다.
//
// **순수 함수인 이유**: 표 열 편집기(`TableChartSection`)는 조회 훅을 쓰지 않는다.
// 훅을 붙이면 QueryClient 프로바이더 없이는 렌더할 수 없게 되어, 설정 UI 단위 테스트가
// 프로바이더를 요구하게 된다. config 만 읽으면 그 대가 없이 후보를 낼 수 있다.
//
// **한계**: 여기서 나오는 것은 "설정에 적힌" 태그다. 실제 데이터가 실어 오는 태그가
// 더 많을 수 있다(필터에 쓰지 않은 태그는 config 에 없지만 엔트리 라벨에는 있다 —
// `useStoreChartData` 의 `baseLabels` 주석 참조). 그래서 후보 목록은 **거들 뿐**이고,
// 필드 직접 입력은 그대로 남는다.

/** 태그 열의 필드 접두 — `tableColumns.resolveCellValue` 와 같은 표기다. */
export const TAG_FIELD_PREFIX = '$.tags.';

/** 태그 키를 표의 열 필드 표기로 바꾼다. */
export function tagFieldPath(tagKey: string): string {
  return `${TAG_FIELD_PREFIX}${tagKey}`;
}

/** 열 필드가 태그 열이면 그 태그 키를, 아니면 `undefined`. */
export function tagKeyOfField(field: string): string | undefined {
  if (!field.startsWith(TAG_FIELD_PREFIX)) return undefined;
  const key = field.slice(TAG_FIELD_PREFIX.length);
  return key === '' ? undefined : key;
}

/** 시리즈 참조에서 태그 키를 모은다. `tags`(필터)와 `group_by`(분할 축) 둘 다 태그다. */
function collectFromSeries(series: unknown, out: Set<string>): void {
  if (!Array.isArray(series)) return;
  for (const ref of series) {
    if (typeof ref !== 'object' || ref === null) continue;
    const r = ref as { tags?: unknown; group_by?: unknown };
    if (typeof r.tags === 'object' && r.tags !== null) {
      for (const k of Object.keys(r.tags as Record<string, unknown>)) out.add(k);
    }
    if (Array.isArray(r.group_by)) {
      for (const k of r.group_by) if (typeof k === 'string' && k !== '') out.add(k);
    }
  }
}

/**
 * 패널의 데이터 소스 설정에 등장하는 태그 키(정렬·중복 제거).
 *
 * store · tsdb 소스를 모두 훑는다 — 소스 종류를 판정해 하나만 보지 않는 이유는,
 * 사용자가 소스를 바꿔 가며 편집하는 동안 이전 소스 설정이 config 에 남아 있고,
 * 그 태그도 여전히 유효한 후보이기 때문이다(고르는 것은 사용자다).
 *
 * 시스템 지표 소스는 태그 축이 고정이라 별도로 다룬다 —
 * `sysmetricsTagKeys` 참조.
 */
export function panelTagKeys(config: Record<string, unknown> | undefined): string[] {
  const out = new Set<string>();
  if (!config) return [];

  const store = config.store_source as
    | { series?: unknown; tag_filters?: unknown }
    | undefined;
  if (store) {
    collectFromSeries(store.series, out);
    if (typeof store.tag_filters === 'object' && store.tag_filters !== null) {
      for (const k of Object.keys(store.tag_filters as Record<string, unknown>)) out.add(k);
    }
  }

  const tsdb = config.tsdb_source as { series?: unknown } | undefined;
  if (tsdb) collectFromSeries(tsdb.series, out);

  if (config.sysmetrics_source) for (const k of SYSMETRICS_TAG_KEYS) out.add(k);

  return Array.from(out).sort((a, b) => a.localeCompare(b));
}

/**
 * 시스템 지표 소스의 태그 축 — 고정 목록이다.
 *
 * `sysmetricsSource.sysmetricSeriesTags` 가 싣는 키와 같다: 분류(`category`)는 모든
 * 줄에 있고, 인스턴스 축은 그룹마다 하나씩(`interface` · `device` · `mountpoint`),
 * `mode` 는 누적 원값 줄에만 붙는다. 선택된 필드를 다시 해석하지 않고 전부 제시하는
 * 이유: 후보 목록은 고르기를 돕는 것이고, 없는 태그를 고르면 그 열이 빈칸일 뿐이다.
 */
export const SYSMETRICS_TAG_KEYS: readonly string[] = [
  'category',
  'device',
  'interface',
  'mode',
  'mountpoint',
];
