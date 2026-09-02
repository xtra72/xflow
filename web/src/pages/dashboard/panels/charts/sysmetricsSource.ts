// sysmetrics 소스의 시리즈 해석 + 값 추출 — 의존성 없는 순수 모듈.
//
// React 를 끌어오지 않으므로 UI 없이 전수 테스트가 가능하다. 훅(`useSysMetricsChartData`)과
// 설정 미리보기(`previewSeries`)가 **같은 이름**을 만들어야 "미리보기와 대시보드가 다르다"가
// 되지 않으므로, 이름 규칙의 정본을 여기 하나로 둔다.
//
// 이 소스에는 버킷도 집계도 없다. 에이전트는 그 시점 스냅샷 하나만 주고, 훅이 폴링하며
// 점을 쌓는다. 그래서 여기서 하는 일은 두 가지뿐이다:
//   1. (값 × 대상) 곱을 펼쳐 **그릴 줄 목록**을 만든다.
//   2. 스냅샷 한 쌍에서 그 줄의 **값 하나**를 뽑는다(누적 카운터는 기본이 증가량이고,
//      줄이 `mode: 'total'` 을 고르면 누적 원값 그대로다).

import type {
  SysmetricsSeriesRef,
  SysmetricsSourceConfig,
} from './chartChannelTypes';
import type { SysMetricsCounterMode } from '@/pages/dashboard/panels/sysmetrics/sysMetricsItemOptions';
import { pickSeriesColor } from './chartChannelTypes';
import { resolveSeriesAlias } from './aliasTemplate';
// 시리즈 표시 이름은 Store · TSDB 와 같은 함수를 쓴다 — 두 번째 표기를 만들지 않는다.
import { makeSeriesId, seriesRefDisplayName } from '@/services/api/seriesLabels';
import {
  SYSMETRIC_CHART_FIELDS,
  findChartField,
  type SysMetricField,
} from '@/pages/dashboard/panels/sysmetrics/sysMetricsFields';
import {
  deltaRate,
  sumInstances,
  type InstanceGroup,
  type MetricGroup,
  type SysMetricKind,
  type SysMetricsSnapshot,
  type SysMetricsTargets,
} from '@/pages/dashboard/panels/sysmetrics/sysMetricsSeries';
import type { UnitTime } from '@/pages/monitoring/networkSeries';

/**
 * 인스턴스 축이 있는 지표 그룹 → **태그 키**.
 *
 * cpu · memory 는 여기 없다 — 인스턴스 축이 없어 대상을 고를 것이 없고 언제나 한 줄이다.
 *
 * 대상 이름을 Store 와 같은 어휘의 태그로 싣는다(`{ interface: 'en0' }`). 백엔드 대상 축
 * 이름(`interfaces` · `devices` · `mountpoints`)의 단수형이라, 저장 경로
 * (`sysmetrics-in` → storage-write)로 옮겨 Store 소스로 볼 때 태그 이름을 다시 배우지 않는다.
 */
const TAG_KEY_BY_GROUP: Partial<Record<SysMetricKind, string>> = {
  network: 'interface',
  diskIo: 'device',
  storage: 'mountpoint',
};

/** 인스턴스 축이 있는 그룹인가. */
export function hasInstanceAxis(group: SysMetricKind): boolean {
  return TAG_KEY_BY_GROUP[group] !== undefined;
}

/**
 * 인스턴스 축이 있는 그룹 → 에이전트 스냅샷 `targets` 의 키.
 *
 * 태그 키(단수형)와 짝이지만 **다른 축**이다 — 하나는 표시 어휘, 하나는 스냅샷 형상이다.
 */
const TARGET_AXIS_BY_GROUP: Partial<Record<SysMetricKind, keyof SysMetricsTargets>> = {
  network: 'interfaces',
  diskIo: 'devices',
  storage: 'mountpoints',
};

/** 시리즈 표의 한 행. `SeriesSelectTable` 의 `SeriesRow` 에 대상 축을 더한 것이다. */
export interface SysmetricRow {
  id: string;
  /**
   * 표의 measurement 컬럼 — **필드 이름**(`bytes_recv`)이다.
   *
   * 값 카탈로그 키가 아니다. config 에 넣을 키는 `metricKey` 에 따로 싣는다 — 둘을
   * 같은 자리에 두면 표시용 이름이 그대로 저장돼 카탈로그를 찾지 못한다.
   */
  key: string;
  /** 값 카탈로그 키(`network.bytes_recv`). config 의 `series[].key` 에 그대로 들어간다. */
  metricKey: string;
  field: string;
  dataType: string;
  registration: string;
  tags: Record<string, string>;
  /** 이 행이 가리키는 인스턴스 대상. 종합·인스턴스 축 없음이면 undefined. */
  target?: string;
  /** 보고되지 않는 대상 표시(선택). */
  badge?: string;
}

/**
 * 값의 **measurement** — 그룹을 뗀 필드 이름(`usage_percent` · `bytes_recv`).
 *
 * 카탈로그 키(`cpu.usage_percent`)를 통째로 measurement 로 쓰면 그룹이 measurement 안에
 * 눌러붙어 태그로 거를 수 없다. Store 어휘에서 그룹은 **분류 태그**이지 이름의 일부가
 * 아니다 — 그래서 여기서 둘을 가른다.
 */
export function sysmetricMeasurement(field: SysMetricField): string {
  return field.field;
}

/**
 * 값의 **분류(category)** — 카탈로그 키의 앞부분(`cpu` · `network` · `disk_io`).
 *
 * `field.group` 이 아니라 키 접두를 쓰는 이유: `group` 은 스냅샷 형상의 camelCase 키
 * (`diskIo`)이고, 키 접두는 에이전트가 배치에 싣는 그룹 이름(`disk_io`)과 같다. 저장
 * 경로(`sysmetrics-in` → storage-write)로 옮겨 Store 소스로 볼 때 같은 값이어야 한다.
 */
export function sysmetricCategory(field: SysMetricField): string {
  const dot = field.key.indexOf('.');
  return dot > 0 ? field.key.slice(0, dot) : field.key;
}

/**
 * 줄 하나의 **태그**.
 *
 * Store 시리즈의 `tags` 와 같은 자리이며, 이름 형식의 `{$.tags.category}` ·
 * `{$.tags.interface}` 토큰이 여기서 해석된다. 두 축이 실린다:
 *
 *   `category`                          → 지표 분류. 모든 줄에 있다.
 *   `interface` / `device` / `mountpoint` → 인스턴스 대상. 종합에는 없다.
 *
 * measurement 는 필드 이름뿐이라 `usage_percent` 가 cpu · memory · storage 에 모두
 * 나타난다 — 그 셋을 가르는 것이 `category` 다.
 */
export function sysmetricSeriesTags(
  field: SysMetricField,
  target: string | undefined,
  mode?: SysMetricsCounterMode,
): Record<string, string> {
  const tags: Record<string, string> = { category: sysmetricCategory(field) };
  const tagKey = TAG_KEY_BY_GROUP[field.group];
  if (tagKey !== undefined && target !== undefined) tags[tagKey] = target;
  // 누적 원값만 표현 축을 싣는다. 증가량은 에이전트에서도 `mode` 없이 오므로
  // (`sysmetrics_history.go`), 여기서 실으면 이미 저장된 패널이 시리즈를 찾지 못한다.
  if (mode === 'total' && field.rate) tags.mode = mode;
  return tags;
}

/**
 * 줄이 실제로 쓰는 표현 방식.
 *
 * `rate: false` 인 값(비율·용량)에는 환산할 것이 없으므로 `mode` 가 남아 있어도 무시한다 —
 * 값을 바꾸며 남을 수 있고, 그대로 태그로 실으면 있지도 않은 축이 생긴다.
 */
export function sysmetricMode(
  field: SysMetricField,
  ref: Pick<SysmetricsSeriesRef, 'mode'>,
): SysMetricsCounterMode {
  return field.rate && ref.mode === 'total' ? 'total' : 'rate';
}

/**
 * 시리즈 표의 **행 id** — Store 와 같은 결정적 SeriesID 규약을 쓴다.
 *
 * `makeSeriesId(measurement, field, tags)` 를 그대로 부르므로, 같은 (값, 대상)은 언제나
 * 같은 id 를 낸다. 표의 선택 상태(`selectedIds`)와 config 의 시리즈 목록이 이 id 하나로
 * 대응된다 — 두 곳이 서로 다른 키를 쓰면 체크가 config 와 어긋난다.
 */
export function sysmetricRowId(ref: Pick<SysmetricsSeriesRef, 'key' | 'target'>): string {
  const field = findChartField(ref.key);
  if (!field) return makeSeriesId(ref.key, undefined, {});
  return makeSeriesId(
    sysmetricMeasurement(field),
    undefined,
    sysmetricSeriesTags(field, ref.target),
  );
}

/**
 * 시리즈 표에 그릴 **후보 행** 목록.
 *
 * 행 하나가 곧 시리즈 하나다(값 + 대상). 구성은 값 카탈로그 순서를 따르며, 각 값마다:
 *
 *   - 인스턴스 축 없음(cpu · memory) → 행 하나(태그 없음)
 *   - 인스턴스 축 있음               → **종합** 행 하나(태그 없음) + 보고된 대상마다 한 행
 *
 * 종합 행을 함께 두는 이유: 인터페이스 전체 합은 개별 인터페이스와 다른 시리즈이고,
 * 대상 목록이 비었을 때만 나오는 숨은 모드로 두면 고를 방법이 없다.
 *
 * **이미 고른 대상이 보고 목록에 없어도 행을 남긴다.** 인터페이스가 내려갔거나, 에이전트
 * 수집 필터가 좁혀졌거나, 다른 장비의 설정을 복사했거나, 아직 첫 표본을 뜨기 전이면
 * 그 행이 사라져 "고른 시리즈가 조용히 없어진" 것처럼 보이고 해제할 수도 없다. 같은
 * 함정을 에이전트 설정의 `SysResourceSelector` 가 먼저 겪고 같은 규칙으로 막았다.
 */
export function sysmetricsCandidateRows(
  targets: SysMetricsTargets | undefined,
  selected: readonly Pick<SysmetricsSeriesRef, 'key' | 'target'>[],
): SysmetricRow[] {
  const rows: SysmetricRow[] = [];

  for (const field of SYSMETRIC_CHART_FIELDS) {
    const axis = TARGET_AXIS_BY_GROUP[field.group];
    // 인스턴스 축이 없는 값은 언제나 한 행이다.
    if (axis === undefined) {
      rows.push(makeRow(field, undefined, true));
      continue;
    }

    // 종합 행 — 전체 합은 개별 대상과 다른 시리즈다.
    rows.push(makeRow(field, undefined, true));

    const reported = targets?.[axis] ?? [];
    const known = new Set(reported);
    for (const name of reported) rows.push(makeRow(field, name, true));
    // 보고 목록에 없는데 이미 고른 대상 — 행을 남기고 표시로 구분한다.
    for (const ref of selected) {
      if (ref.key !== field.key || ref.target === undefined) continue;
      if (known.has(ref.target)) continue;
      rows.push(makeRow(field, ref.target, false));
    }
  }

  return rows;
}

/** 후보 행 하나를 만든다. */
function makeRow(field: SysMetricField, target: string | undefined, present: boolean): SysmetricRow {
  return {
    id: sysmetricRowId({ key: field.key, target }),
    // Store 의 measurement 컬럼 — 필드 이름만 들어간다. 그룹은 `category` 태그로 간다.
    key: sysmetricMeasurement(field),
    metricKey: field.key,
    // sysmetrics 에는 measurement 아래 별도 field 축이 없다(Store 의 `field` 자리).
    field: '',
    // Store 의 데이터타입 컬럼 자리 — sysmetrics 에는 표기 방식(percent/bytes/count)이
    // 그 자리에서 가장 쓸모 있는 구분이다.
    dataType: field.format,
    registration: '',
    tags: sysmetricSeriesTags(field, target),
    target,
    ...(present ? {} : { badge: SYSMETRIC_MISSING_BADGE }),
  };
}

/** 보고되지 않는 대상 행에 붙는 배지 문자열. */
export const SYSMETRIC_MISSING_BADGE = 'missing';

/** 값 하나가 펼쳐진 **그릴 줄** 하나. */
export interface ResolvedSysmetricSeries {
  /** 표시 이름 = 계열 키. 패널 전체에서 이 이름으로 시리즈를 식별한다. */
  name: string;
  /**
   * 별칭·형식을 **빼고** 계산한 내장 표기.
   *
   * 설정 화면의 이름 입력 placeholder 로 쓴다. `name` 을 placeholder 로 쓰면 사용자가
   * 이름을 넣은 순간 placeholder 가 그 이름으로 바뀌어, 비웠을 때 무엇으로 돌아가는지
   * 알 수 없다.
   */
  defaultName: string;
  /** 값 정의(카탈로그 항목). */
  field: SysMetricField;
  /**
   * 인스턴스 대상 이름. 인스턴스 축이 없거나 **종합**이면 undefined 다.
   *
   * `''` 로 두지 않는 이유: 종합과 "이름이 빈 대상"을 구분해야 값 추출이 갈린다.
   */
  target?: string;
  /**
   * 이 줄이 쓰는 표현 방식. 누적 카운터가 아니면 언제나 `rate` 다(뜻 없음).
   *
   * 증가량과 누적 원값은 **다른 시리즈**다 — 태그의 `mode` 축으로 갈리고, 에이전트도
   * 둘을 따로 싣는다(`sysmetrics_history.go`).
   */
  mode: SysMetricsCounterMode;
  /** Store 어휘의 measurement — 값 카탈로그 키다. */
  measurement: string;
  /** Store 어휘의 tags — 대상 축(`{ interface: 'en0' }`). 대상이 없으면 빈 객체다. */
  tags: Record<string, string>;
  /** 색. 지정이 없으면 줄 순서 기준 자동 팔레트. */
  color: string;
  /** 원본 참조(선 모양 등 표시 축을 그대로 물려준다). */
  ref: SysmetricsSeriesRef;
}

/**
 * 줄의 **내장 서술 표기**를 만든다. 별칭도 형식도 없을 때의 최종 폴백이다.
 *
 * Store · TSDB 와 **같은 함수**(`seriesRefDisplayName`)를 쓴다 — 세 소스가 같은 표기를
 * 내야 소스를 갈아탄 사용자가 같은 시리즈를 알아본다. 축 대응은 다음과 같다:
 *
 *   measurement ← 필드 이름 (`bytes_recv`)
 *   tags        ← 분류 + 대상 (`{ category: 'network', interface: 'en0' }`)
 *
 * 결과 예: `bytes_recv · {category=network, interface=en0}` ·
 *         대상이 없으면 `usage_percent · {category=cpu}`.
 *
 * 로케일 독립이다 — 카탈로그 키는 번역되지 않으므로 언어를 바꿔도 계열 키가 흔들리지
 * 않는다. 계열 이름은 legend 라벨이자 누적 맵의 **키**라, 언어를 전환하는 순간 키가
 * 전부 달라지면 쌓아 둔 점이 버려진다.
 */
export function describeSysmetricSeries(
  field: SysMetricField,
  target: string | undefined,
  mode?: SysMetricsCounterMode,
): string {
  return seriesRefDisplayName(
    sysmetricMeasurement(field),
    undefined,
    sysmetricSeriesTags(field, target, mode),
  );
}

/**
 * 줄의 표시 이름을 정한다.
 * 우선순위: **항목 별칭 > 소스 형식 > 내장 서술 표기** — store/tsdb 와 같다.
 *
 * 대상별 이름 축(`target_alias`)은 없다. 시리즈 하나가 대상 하나를 가리키므로 `alias`
 * 하나면 줄 하나에 이름 하나가 붙는다 — TSDB 가 group by 로 N줄을 펼치느라 필요했던
 * 두 번째 축이 여기서는 필요 없다.
 *
 * 형식 토큰의 대응(Store 와 같은 어휘):
 *   `{$.measurement}`     → 필드 이름(`bytes_recv`)
 *   `{$.tags.category}`   → 지표 분류(`network` · `cpu` · `disk_io`)
 *   `{$.tags.interface}`  → 네트워크 인터페이스(`en0`)
 *   `{$.tags.device}`     → 디스크 장치(`disk0`)
 *   `{$.tags.mountpoint}` → 스토리지 마운트(`/data`)
 *
 * 대상이 없는 줄(종합 · cpu · memory)에는 인스턴스 태그가 없어 그 토큰이 빈 문자열이
 * 된다. `category` 는 모든 줄에 있다.
 */
export function sysmetricSeriesName(
  ref: SysmetricsSeriesRef,
  field: SysMetricField,
  target: string | undefined,
  format: string | undefined,
): string {
  const mode = sysmetricMode(field, ref);
  // Store 와 같은 어휘의 컨텍스트 — 이름 형식 토큰이 세 소스에서 같은 뜻을 갖는다.
  const ctx = {
    measurement: sysmetricMeasurement(field),
    field: undefined,
    tags: sysmetricSeriesTags(field, target, mode),
  };
  // 1) 항목에 직접 붙인 이름이 항상 이긴다. 토큰을 쓴 이름도 해석한다.
  const alias = ref.alias?.trim() ?? '';
  if (alias !== '') return resolveSeriesAlias(alias, ctx);
  // 2) 소스가 지정한 이름 형식. 해석 결과가 비면(토큰이 전부 빈 값) 폴백한다.
  const fmt = format?.trim() ?? '';
  if (fmt !== '') {
    const resolved = resolveSeriesAlias(fmt, ctx).trim();
    if (resolved !== '') return resolved;
  }
  // 3) 내장 서술 표기.
  return describeSysmetricSeries(field, target, mode);
}

/**
 * 소스 config 를 **그릴 줄 목록**으로 옮긴다. 시리즈 하나가 줄 하나다(1:1).
 *
 * 종전에는 값 목록 × 대상 목록의 곱이었다. 그 모델로는 "값 A 는 en0, 값 B 는 en1" 을
 * 표현할 수 없고, 설정 화면을 Store 처럼 시리즈 표로 그릴 수도 없었다 — 표의 한 행이
 * 곧 한 시리즈여야 하기 때문이다.
 *
 * 카탈로그에 없는 키는 버린다. 패널 유형을 바꾸며 남은 다른 어휘가 섞일 수 있다.
 *
 * 이름이 겹치면 뒤에 온 줄에 `#2` 를 붙여 가른다. 겹친 채 두면 Map 키가 충돌해 한 줄이
 * 조용히 사라진다 — 사용자에게는 "고른 값이 안 그려짐" 으로 보인다.
 */
export function resolveSysmetricsSeries(
  source: SysmetricsSourceConfig | undefined,
): ResolvedSysmetricSeries[] {
  if (!source) return [];

  const out: ResolvedSysmetricSeries[] = [];
  const used = new Map<string, number>();

  for (const ref of source.series ?? []) {
    const field = findChartField(ref.key);
    if (!field) continue;
    // 인스턴스 축이 없는 값에 대상이 남아 있으면 무시한다 — 값을 바꾸며 남을 수 있고,
    // 태그로 실으면 있지도 않은 축이 생긴다.
    const target = hasInstanceAxis(field.group) ? ref.target : undefined;

    const mode = sysmetricMode(field, ref);
    const base = sysmetricSeriesName(ref, field, target, source.series_name_format);
    const seen = (used.get(base) ?? 0) + 1;
    used.set(base, seen);
    out.push({
      name: seen === 1 ? base : `${base} #${seen}`,
      defaultName: describeSysmetricSeries(field, target, mode),
      field,
      target,
      mode,
      measurement: sysmetricMeasurement(field),
      tags: sysmetricSeriesTags(field, target, mode),
      color: ref.color ?? pickSeriesColor(out.length),
      ref,
    });
  }

  return out;
}

// ---- 값 추출 ----

/** 스냅샷에서 그룹을 꺼낸다. 인스턴스 축 유무에 따라 형태가 다르다. */
function pickGroup(
  snapshot: SysMetricsSnapshot,
  group: SysMetricKind,
): MetricGroup | InstanceGroup | undefined {
  return snapshot[group];
}

/**
 * 한 줄이 참조하는 **원값**(누적 카운터라면 누적값 그대로)을 꺼낸다.
 *
 * 스토리지 사용률의 종합만 특별하다 — 마운트별 퍼센트를 더하면 200% 가 나온다.
 * 합계 용량 기준으로 다시 계산해야 한다(`sumStorage` 와 같은 규칙).
 */
export function readRawValue(
  snapshot: SysMetricsSnapshot,
  series: ResolvedSysmetricSeries,
): number | undefined {
  const { field, target } = series;
  const group = pickGroup(snapshot, field.group);
  if (group === undefined) return undefined;

  if (!hasInstanceAxis(field.group)) {
    return (group as MetricGroup)[field.field];
  }

  const instances = group as InstanceGroup;
  if (target !== undefined) return instances[target]?.[field.field];

  // 종합 — 마운트/장치/인터페이스 전체 합.
  const summed = sumInstances(instances);
  if (field.key === 'storage.usage_percent') {
    const total = summed.total_bytes ?? 0;
    return total > 0 ? ((summed.used_bytes ?? 0) / total) * 100 : 0;
  }
  return summed[field.field];
}

/**
 * 한 줄의 **그릴 값**을 만든다. 그릴 수 없으면 null.
 *
 * 누적 카운터(`rate: true`)는 기본적으로 두 표본의 증가량을 단위시간으로 환산한다.
 * 그래서 첫 표본에서는 null 이다 — 기준점이 없으면 증가량을 만들 수 없다. null 은
 * "그리지 않는다"이지 0 이 아니다(`deltaRate` 와 같은 규약).
 *
 * 줄이 누적 원값(`mode: 'total'`)을 고르면 환산하지 않는다 — 원값이 곧 그릴 값이라
 * 기준점 없이 첫 표본부터 나온다.
 */
export function readSeriesValue(
  snapshot: SysMetricsSnapshot,
  previous: SysMetricsSnapshot | null,
  series: ResolvedSysmetricSeries,
  unit: UnitTime,
): number | null {
  const next = readRawValue(snapshot, series);
  if (next === undefined) return null;
  if (!series.field.rate || series.mode === 'total') return next;

  if (!previous || previous.collectedAt === null || snapshot.collectedAt === null) return null;
  const prev = readRawValue(previous, series);
  if (prev === undefined) return null;

  return deltaRate(
    { value: prev, at: previous.collectedAt },
    { value: next, at: snapshot.collectedAt },
    unit,
  );
}
