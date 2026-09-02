// Store 에이전트 시리즈 매트릭스를 주기적으로 폴링해 차트용 ChartEntry 로 변환하는 훅.
//
// 시리즈 소스(store · TSDB · 시스템 지표)가 공유하는 조회 훅. 채널 경로가 패널에서 빠진 뒤로 유일한 데이터
// 소스다. 패널은 config.data_source === 'store' 일 때 이 훅을 사용하고, 그 외에는
// 기존 채널 훅을 사용한다. 반환 형상은 채널 훅과 최대한 일치시켜 패널 렌더 코드의
// 변경을 최소화한다(entries / status / closedReason / errorReason).
//
// 매트릭스(SeriesMatrix) → ChartEntry 변환 규칙:
//   - 컬럼(시리즈)마다 행을 순회하며 value=row.values[col], timestamp=bucketStartMs.
//   - flattened `entries`: 모든 시리즈를 timestamp 오름차순으로 평탄화(값이 null 인
//     버킷은 제외). Stat/Bar/Pie/Table 처럼 단일 타임라인을 소비하는 패널이 사용한다.
//     각 entry 는 labels.name = 시리즈 표시 이름을 가져 Bar/Pie 의 카테고리 분리에 쓰인다.
//   - `seriesEntries`: 시리즈 표시 이름 → ChartEntry[](null 포함). LineChart 가 시리즈별
//     라인을 그릴 때 사용한다(connectNulls 로 빈 버킷을 잇는다).
//
// 폴링: refresh_interval_ms(기본 5000ms) 주기로 queryMatrix 를 호출하고, 각 호출은
// AbortController 로 중단 가능하다. 언마운트/설정 변경/비활성화 시 인터벌 정리 + abort.
//
// @spec SPEC-WEB-005

import {
  limitToRecent,
  readSeriesRange,
  resolveSeriesWindow,
  seriesRangeKey,
} from './seriesRange';

import { useEffect, useMemo, useRef, useState } from 'react';

import type {
  SeriesMatrix,
  SeriesMatrixQuery,
  SeriesSelectorFilter,
} from '@/services/api/seriesDataSource';
import { startVisiblePolling } from './visiblePolling';
import { groupComboSignature } from '@/services/api/tsdbSeriesEnum';
import { fetchStoreKeys, storeSeriesDataSource } from '@/services/api/store';
import { useAgents } from '@/hooks/useAgent';
import { resolveStoreAgentName } from './storeAgentResolve';
import { pickSeriesColor, storeSeriesLabel } from './chartChannelTypes';
import type { GraphStyle } from './graphStyle';
import {
  METRIC_LABEL_KEY,
  parseSeriesLabels,
} from '@/services/api/seriesLabels';
import type {
  ChartConnectionStatus,
  ChartEntry,
  StoreSeriesRef,
  StoreSourceConfig,
} from './chartChannelTypes';

/** 기본 폴링 주기(ms). */
const DEFAULT_REFRESH_MS = 5_000;
/** 최소 폴링 주기(ms) — 과도한 요청 방지. */
const MIN_REFRESH_MS = 1_000;

/**
 * 매트릭스 쿼리 실행 함수 시그니처(테스트 주입용).
 * 기본 구현은 `storeSeriesDataSource(agentName).queryMatrix` 를 사용한다.
 */
export type QueryMatrixFn = (
  agentName: string,
  params: SeriesMatrixQuery,
  signal: AbortSignal,
) => Promise<SeriesMatrix>;

/**
 * tag 모드 키 해석 함수 시그니처(테스트 주입용).
 * `tag_filters` AND 필터로 현재 매칭되는 store 키 이름 배열을 반환한다.
 * 기본 구현은 `fetchStoreKeys(agentName, tagFilters, signal)` 를 사용한다.
 */
export type ResolveKeysFn = (
  agentName: string,
  tagFilters: Record<string, string>,
  signal: AbortSignal,
) => Promise<string[]>;

/** 훅 옵션(테스트 주입). */
export interface UseStoreChartDataOptions {
  /** 매트릭스 쿼리 실행기(테스트에서 네트워크 없이 주입). */
  queryMatrixFn?: QueryMatrixFn;
  /** 현재 시각 제공기(테스트 결정성 확보). 기본 Date.now. */
  nowFn?: () => number;
  /** tag 모드 키 해석기(테스트에서 네트워크 없이 주입). 기본 fetchStoreKeys. */
  resolveKeysFn?: ResolveKeysFn;
}

/**
 * 시리즈별 라인 스타일(LineChart 렌더용). SPEC-WEB-005.
 * store_source.series 의 per-line 스타일을 시리즈 표시 이름 기준으로 노출한다.
 */
export interface StoreSeriesStyle {
  color?: string;
  stroke_style?: 'solid' | 'dashed' | 'dotted';
  stroke_width?: number;
  smooth?: boolean;
  /** 이 시리즈를 그리는 모양. 미지정이면 패널 기본값을 따른다. */
  graph_style?: GraphStyle;
}

/** 훅 결과 — 채널 훅과 형상을 맞춘다. */
export interface UseStoreChartDataResult {
  /** 평탄화된 단일 타임라인(값이 null 인 버킷 제외). */
  entries: ChartEntry[];
  /** 시리즈 표시 이름 → 타임라인(null 포함). LineChart 다중 라인용. */
  seriesEntries: Map<string, ChartEntry[]>;
  /** 시리즈 표시 이름 → per-line 스타일. LineChart 가 라인 색/두께/스타일 적용에 사용. */
  seriesStyles: Map<string, StoreSeriesStyle>;
  /** 컬럼(시리즈) 표시 이름 순서(요청/응답 순서 보존). */
  seriesNames: string[];
  /**
   * data_type='boolean' 인 시리즈의 표시 이름 집합. LineChart 가 해당 시리즈를
   * true/false(0/1 축·툴팁)로 표시하는 데 사용한다. 값은 store 변환 시 이미 1/0 이다.
   */
  booleanSeries: Set<string>;
  /** 연결/조회 상태. idle | connecting | connected | error 만 사용한다. */
  status: ChartConnectionStatus;
  /** error 상태일 때의 사유 메시지. */
  errorReason?: string;
  /** 채널 훅 형상 호환을 위한 필드(Store 소스에서는 항상 undefined). */
  closedReason?: string;
}

/**
 * 기본 매트릭스 쿼리 실행기 — 실제 Store 백엔드를 호출한다.
 */
const defaultQueryMatrix: QueryMatrixFn = (agentName, params, signal) =>
  storeSeriesDataSource(agentName).queryMatrix(params, signal);

/**
 * 기본 tag 모드 키 해석기 — 실제 Store 백엔드의 `GET /keys?tag=k:v` 를 호출한다.
 */
const defaultResolveKeys: ResolveKeysFn = (agentName, tagFilters, signal) =>
  fetchStoreKeys(agentName, tagFilters, signal);

/**
 * tag 모드에서 해석된 키 목록을 `series[]` 로 확장한 effective config 를 만든다.
 *
 * 각 키는 하나의 시리즈가 되며(별칭 미지정 → 컬럼명 폴백), 인덱스 기준으로 기본 팔레트 색을 배정한다
 * (사용자 지정 색은 tag 모드에 없으므로 항상 자동 배정). 집계/시간/인터벌/네임스페이스는
 * 원본 config 를 그대로 물려받고, `series[]` 만 동적으로 교체한다. field/tags 는
 * 부여하지 않으므로 각 키의 모든 시리즈가 조회된다(태그에 걸린 키 전체를 라인으로).
 */
export function tagResolvedConfig(
  config: StoreSourceConfig,
  resolvedKeys: string[],
): StoreSourceConfig {
  // alias 는 부여하지 않는다 — 비면 컬럼명(=key)으로 폴백하므로 표시 결과는 같고,
  // "사용자가 붙인 이름" 과 기본값이 뒤섞이지 않는다.
  const series: StoreSeriesRef[] = resolvedKeys.map((key, i) => ({
    key,
    color: pickSeriesColor(i),
  }));
  return { ...config, series };
}

/**
 * config.series 를 SeriesMatrixQuery 의 keys/seriesFilters 로 변환한다.
 *
 * - keys: 각 시리즈의 key.
 * - seriesFilters: field/tags 중 하나라도 있으면 SeriesSelectorFilter 를 만들고,
 *   둘 다 없으면 undefined(해당 key 의 모든 시리즈 조회).
 *   필터가 하나도 없으면 seriesFilters 자체를 생략한다(기존 동작 보존).
 */
function buildKeysAndFilters(config: StoreSourceConfig): {
  keys: string[];
  seriesFilters?: Array<SeriesSelectorFilter | undefined>;
} {
  const keys: string[] = [];
  const filters: Array<SeriesSelectorFilter | undefined> = [];
  let anyFilter = false;
  for (const ref of config.series) {
    keys.push(ref.key);
    const hasTags = ref.tags && Object.keys(ref.tags).length > 0;
    if (ref.field || hasTags) {
      anyFilter = true;
      filters.push({
        fieldName: ref.field,
        tags: hasTags ? ref.tags : undefined,
      });
    } else {
      filters.push(undefined);
    }
  }
  return anyFilter ? { keys, seriesFilters: filters } : { keys };
}

/**
 * SeriesMatrix 를 시리즈별/평탄화 ChartEntry 로 변환한다.
 *
 * 컬럼 인덱스와 config.series 의 인덱스가 1:1 로 정렬되는 일반적인 경우, 시리즈의
 * alias/color/tags 메타데이터를 컬럼에 매핑한다. queryMatrix 가 한 key 를 다중
 * 시리즈로 확장해 컬럼 수가 더 많아지면, alias 매핑은 생략하고 컬럼명을 그대로 쓴다.
 */
export function matrixToEntries(
  matrix: SeriesMatrix,
  // 변환기가 실제로 읽는 것은 시리즈 목록과 이름 형식뿐이다. 전체 소스 설정을
  // 요구하면 TSDB 투영(`asSeriesConfig`)이 쓰지도 않는 조회 창 값을 지어내야
  // 하고, 두 소스의 집계 어휘가 다른 지금은 그 값이 거짓이 된다(§2.18).
  config: Pick<StoreSourceConfig, 'series' | 'series_name_format'>,
): {
  entries: ChartEntry[];
  seriesEntries: Map<string, ChartEntry[]>;
  seriesStyles: Map<string, StoreSeriesStyle>;
  seriesNames: string[];
  booleanSeries: Set<string>;
} {
  const seriesEntries = new Map<string, ChartEntry[]>();
  const seriesStyles = new Map<string, StoreSeriesStyle>();
  const booleanSeries = new Set<string>();
  const flat: ChartEntry[] = [];
  // 컬럼 → config 시리즈 귀속 (SPEC-TSDB-004 §4.1).
  //
  // 종전에는 "컬럼 수 == 시리즈 수" 일 때만 위치 순서로 매핑했다. group by 는
  // 요청 1건이 컬럼 N개를 만들므로 그 등식이 설계상 깨지고, 등식이 패널 전체에
  // 대한 단일 불리언이었기 때문에 group by 항목 하나가 같은 패널의 정확 일치
  // 항목까지 메타데이터를 잃게 만들었다(UB1-7).
  //
  // 매트릭스가 출처 인덱스를 실어 주면 그것으로 귀속한다. 실어 주지 않는
  // 생산자에는 종전 위치 정렬을 그대로 적용해 동작을 보존한다.
  const origins = matrix.columnOrigins;
  const aligned = matrix.columns.length === config.series.length;
  // 같은 출처를 가리키는 컬럼이 둘 이상이면 그 항목은 group by 로 펼쳐진 것이다.
  const originFanout = new Map<number, number>();
  if (origins) {
    for (const o of origins) originFanout.set(o, (originFanout.get(o) ?? 0) + 1);
  }
  const refAt = (j: number): StoreSeriesRef | undefined => {
    if (!origins) return aligned ? config.series[j] : undefined;
    const idx = origins[j];
    return idx === undefined ? undefined : config.series[idx];
  };
  // group by 파생 컬럼은 자동 팔레트를 쓴다(OQ1) — 항목의 color 는 하나뿐이라
  // N 개 그룹에 나눠 줄 수 없다. 선 모양(stroke/smooth)은 "이 항목의 선 모양"
  // 이라는 의미가 그룹에 그대로 이어지므로 전 그룹이 공유한다.
  const isGroupDerived = (j: number): boolean => {
    if (!origins) return false;
    const idx = origins[j];
    return idx !== undefined && (originFanout.get(idx) ?? 0) > 1;
  };
  // 그룹 파생 컬럼의 표시 이름은 **그 그룹의 실제 태그 값**으로 만든다.
  // 이름 문자열에서 역파싱하지 않고 매트릭스가 실어 준 labels 를 쓴다(§4.3).
  const effectiveRefAt = (j: number): StoreSeriesRef | undefined => {
    const ref = refAt(j);
    if (!ref || !isGroupDerived(j)) return ref;
    const labels = matrix.columnLabels?.[j];
    if (!labels) return ref;
    const groupTags: Record<string, string> = {};
    for (const [k, v] of Object.entries(labels)) {
      if (k !== METRIC_LABEL_KEY) groupTags[k] = v;
    }
    const merged = { ...ref, tags: { ...(ref.tags ?? {}), ...groupTags } };
    // 그룹별 개별 이름 (SPEC-TSDB-004 §2.12).
    //
    // 항목의 `alias` 는 하나뿐이라 N개 그룹에 서로 다른 이름을 줄 수 없다.
    // 사용자가 특정 그룹에 이름을 붙여 뒀으면 그것을 이 컬럼의 alias 로 세운다 —
    // 그러면 아래 이름 결정은 종전 경로(alias 우선)를 그대로 탄다.
    // 여기 도달했다는 것은 이미 그룹 파생 컬럼이라는 뜻이다(위 조기 반환).
    // 조합 서명은 `group_by` 가 있을 때만 만들 수 있다 — 매트릭스가 출처만 알려
    // 주고 축을 알려 주지 않는 경우(Store 경로)에는 그룹별 지정이 성립하지 않는다.
    const groupKeys = [...(ref.group_by ?? [])].sort();
    const sig = groupKeys.length > 0 ? groupComboSignature(groupTags, groupKeys) : undefined;
    const named = sig ? ref.group_alias?.[sig]?.trim() : undefined;
    // 색은 **항상** 덮어쓴다 — 지정이 없으면 undefined 로 덮어 항목 color 가 파생
    // 줄로 새는 것을 막는다(OQ1). 색 하나를 N개 그룹에 나눠 줄 수 없으므로,
    // 그룹에 색을 주려면 그룹별로 지정해야 한다.
    return {
      ...merged,
      ...(named ? { alias: named } : {}),
      color: sig ? ref.group_color?.[sig] : undefined,
    };
  };

  // 출처별 **고정 태그** — 그 출처의 모든 컬럼에서 값이 같은 태그 키
  // (SPEC-TSDB-004 §2.15). group by 항목의 사전 필터(`tags`)가 여기 해당한다.
  //
  // 기본 이름(내장 서술 표기)은 태그를 전부 나열하는데, 모든 줄에서 값이 같은
  // 태그는 줄을 가르지 않으므로 이름에서 순수 군더더기다 — 실제 보고 예:
  // `temperature · value{device.dev_eui=…, device.type=EM300-TH, location=실습실}`
  // 에서 device.type 과 location 은 세 줄 모두 같다.
  const fixedTagsByOrigin = new Map<number, Set<string>>();
  if (origins) {
    const perOrigin = new Map<number, Array<Record<string, string>>>();
    origins.forEach((o, j) => {
      const tags = parseSeriesLabels(matrix.columnLabels?.[j]).tags;
      const list = perOrigin.get(o);
      if (list) list.push(tags);
      else perOrigin.set(o, [tags]);
    });
    for (const [o, list] of perOrigin) {
      if (list.length < 2) continue; // 줄이 하나면 "모두 같다" 가 성립하지 않는다
      const keys = new Set<string>();
      for (const tset of list) for (const k of Object.keys(tset)) keys.add(k);
      const fixed = new Set(
        [...keys].filter((k) => new Set(list.map((tset) => tset[k] ?? '')).size === 1),
      );
      if (fixed.size > 0) fixedTagsByOrigin.set(o, fixed);
    }
  }

  const seriesNames: string[] = matrix.columns.map((colName, j) => {
    const ref = effectiveRefAt(j);
    if (!ref) return colName;
    // 고정 태그는 **기본 이름에서만** 걷어낸다. 별칭·형식은 템플릿이라
    // `{$.tags.location}` 처럼 고정 태그를 참조할 수 있어, 걷어내면 이름이 빈다.
    const named =
      (ref.alias?.trim() ?? '') !== '' || (config.series_name_format?.trim() ?? '') !== '';
    const fixed = origins ? fixedTagsByOrigin.get(origins[j] ?? -1) : undefined;
    if (!named && fixed && ref.tags) {
      const kept: Record<string, string> = {};
      for (const [k, v] of Object.entries(ref.tags)) if (!fixed.has(k)) kept[k] = v;
      return storeSeriesLabel({ ...ref, tags: kept }, config.series_name_format);
    }
    // 표시 이름은 storeSeriesLabel 한 곳에서 결정한다 — 데이터 소스 목록·히트맵 마커와
    // 같은 규칙을 쓰지 않으면 "설정한 이름과 출력이 다르다" 는 불일치가 생긴다.
    //   이름(alias) 직접 입력 → 패널의 시리즈 이름 형식 → 내장 서술 표기.
    //
    // 그룹 파생 컬럼에서도 alias/형식은 **템플릿으로 해석된다** — 사용자가
    // `CPU {$.tags.host}` 처럼 쓰면 그룹마다 다른 이름이 나온다. 토큰 없는 고정
    // 문자열이면 그룹이 전부 같은 이름을 갖게 되는데, 그 충돌은 아래에서 태그
    // 표기를 덧붙여 해소한다. alias 를 조용히 버리지 않는 이유는 사용자가 지정한
    // 이름이 사라지는 편이 더 놀랍기 때문이다.
    return storeSeriesLabel(ref, config.series_name_format);
  });

  // 그룹 파생 이름 충돌 해소 (SPEC-TSDB-004 OQ1 · §2.13).
  //
  // 같은 출처에서 나온 컬럼들이 같은 이름을 가지면 아래 루프에서 한 시리즈로
  // 병합되어 그룹이 통째로 사라진다. 그래서 이름은 서로 달라야 한다.
  //
  // **구분자에 태그 정보를 쓰지 않는다.** 종전에는 값이 다른 태그 키를 덧붙여
  // `실습실 {device.dev_eui=24e124136d151523}` 처럼 되었는데, dev_eui 같은 값은
  // 읽어도 무엇인지 알 수 없으면서 지정한 이름보다 길다. 범례에 이름을 지으라고
  // 형식·별칭을 제공해 놓고 그 뒤에 기계 문자열을 덧붙이면 이름을 지은 의미가
  // 없어진다. 어느 줄이 어느 장비인지 정확히 통제하려면 그룹별 개별 이름을
  // 지정하면 된다(§2.12) — 그러면 이름이 갈려 이 분기 자체를 타지 않는다.
  //
  // **충돌한 줄에만** 붙인다. 종전에는 같은 출처의 줄이 하나라도 충돌하면 전부에
  // 표기를 붙여, 이미 유일한 이름까지 오염됐다.
  if (origins) {
    const byOrigin = new Map<number, number[]>();
    origins.forEach((o, j) => {
      const list = byOrigin.get(o);
      if (list) list.push(j);
      else byOrigin.set(o, [j]);
    });
    for (const [, cols] of byOrigin) {
      if (cols.length < 2) continue;
      const byName = new Map<string, number[]>();
      for (const j of cols) {
        const name = seriesNames[j]!;
        const list = byName.get(name);
        if (list) list.push(j);
        else byName.set(name, [j]);
      }
      const taken = new Set(cols.map((j) => seriesNames[j]!));
      for (const [name, dup] of byName) {
        if (dup.length < 2) continue; // 유일한 이름은 손대지 않는다
        dup.forEach((j, n) => {
          // 붙인 이름이 다른 줄과 또 겹치지 않을 때까지 번호를 올린다.
          let seq = n + 1;
          let next = `${name} (${seq})`;
          while (taken.has(next)) next = `${name} (${++seq})`;
          taken.add(next);
          seriesNames[j] = next;
        });
      }
    }
  }

  matrix.columns.forEach((_colName, j) => {
    const name = seriesNames[j]!;
    const ref = effectiveRefAt(j);
    // 엔트리 라벨 = 사전 필터 태그 + **매트릭스가 실어 준 실제 컬럼 태그** + name.
    //
    // `ref.tags` 는 사용자가 건 사전 필터일 뿐이라, 필터에 쓰지 않은 태그(예: host)는
    // 거기에 없다. 그 상태로 라벨을 만들면 테이블의 `$.tags.host` 컬럼이 빈칸이 된다
    // — group by 로 펼쳐진 컬럼만 실제 태그를 받고 있었다(effectiveRefAt).
    // 실제 태그가 사전 필터를 이긴다(같은 키면 값이 같고, 다르면 실제 쪽이 사실이다).
    //
    // **이름(name)에는 관여하지 않는다.** 이름은 종전대로 `ref` 에서 파생되므로
    // 기존 패널의 시리즈 이름·범례는 그대로다 — 여기서 늘어난 태그를 이름에도 넣으면
    // 모든 store/tsdb 패널의 범례가 한꺼번에 길어진다.
    const columnTags = parseSeriesLabels(matrix.columnLabels?.[j]).tags;
    const baseLabels: Record<string, string> = { ...(ref?.tags ?? {}), ...columnTags, name };
    // per-line 스타일을 시리즈 이름 기준으로 노출(LineChart 렌더용). 같은 이름이 둘
    // 이상이면 처음 등장한 시리즈의 스타일을 유지한다.
    // data_type='boolean' 시리즈는 true/false 표시 대상으로 표기(값은 store 변환에서 1/0).
    if (ref?.data_type === 'boolean') booleanSeries.add(name);
    if (ref && !seriesStyles.has(name)) {
      seriesStyles.set(name, {
        // OQ1 은 effectiveRefAt 에서 처리한다 — 그룹 파생 컬럼의 color 는 거기서
        // 그룹별 색(없으면 undefined)으로 덮인다. 비면 자동 팔레트로 넘어간다.
        color: ref.color,
        stroke_style: ref.stroke_style,
        stroke_width: ref.stroke_width,
        smooth: ref.smooth,
        graph_style: ref.graph_style,
      });
    }
    const perSeries: ChartEntry[] = [];
    for (const row of matrix.rows) {
      const v = row.values[j];
      const entry: ChartEntry = {
        timestamp: row.bucketStartMs,
        value: v,
        labels: baseLabels,
        meta: { seriesName: name },
      };
      // 시리즈별 타임라인은 null 도 포함(라인 gap 표현용).
      perSeries.push(entry);
      // 평탄화 타임라인은 숫자 값만 포함(stat/bar/pie/table 오염 방지).
      if (v !== null && Number.isFinite(v)) {
        flat.push(entry);
      }
    }
    // 같은 표시 이름이 둘 이상이면 뒤 시리즈가 앞 시리즈를 덮어쓰지 않도록 병합한다.
    const existing = seriesEntries.get(name);
    if (existing) existing.push(...perSeries);
    else seriesEntries.set(name, perSeries);
  });

  // 평탄화 타임라인을 timestamp 오름차순으로 정렬(여러 시리즈가 섞이므로).
  flat.sort((a, b) => a.timestamp - b.timestamp);

  return { entries: flat, seriesEntries, seriesStyles, seriesNames, booleanSeries };
}

const EMPTY_RESULT: UseStoreChartDataResult = {
  entries: [],
  seriesEntries: new Map(),
  seriesStyles: new Map(),
  seriesNames: [],
  booleanSeries: new Set(),
  status: 'idle',
};

/**
 * Store 소스 차트 데이터 훅.
 *
 * @param config Store 소스 설정. undefined 면 비활성(idle).
 * @param enabled false 면 폴링하지 않고 idle 을 반환한다(패널이 채널 모드일 때).
 * @param options 테스트 주입(queryMatrixFn / nowFn).
 */
export function useStoreChartData(
  config: StoreSourceConfig | undefined,
  enabled: boolean,
  options: UseStoreChartDataOptions = {},
): UseStoreChartDataResult {
  const [result, setResult] = useState<UseStoreChartDataResult>(EMPTY_RESULT);

  // 옵션은 참조만 하므로 ref 로 안정화(불필요한 재구독 방지).
  const optionsRef = useRef(options);
  optionsRef.current = options;

  // SPEC-WEB-006: 저장된 agent_id 를 현재 에이전트 이름으로 해석한다. 에이전트
  // 이름이 바뀌어도 id 는 불변이므로 항상 현재 이름으로 조회된다. agent_id 가
  // 없는 구 config 는 저장된 agent_name 을 그대로 사용(하위호환).
  const { data: agentsResult } = useAgents();
  const resolvedAgentName = config
    ? resolveStoreAgentName(config.agent_id, config.agent_name, agentsResult?.data)
    : '';
  // effect 내부(run)에서 참조하되 pollKey 재계산 없이 최신값을 쓰도록 ref 로 안정화한다.
  // 단, 이름 변경 시에는 재조회가 필요하므로 pollKey 에도 resolvedAgentName 을 포함한다.
  const resolvedAgentNameRef = useRef(resolvedAgentName);
  resolvedAgentNameRef.current = resolvedAgentName;

  // 폴링 재시작을 결정하는 키 — config 의 의미있는 필드만 직렬화한다.
  // series 의 순서/필터/시간 파라미터가 바뀌면 재구독한다.
  const pollKey = useMemo(() => {
    if (!enabled || !config) return '';
    if (config.interval_ms <= 0) return '';
    // 범위가 해석되지 않으면(빈 값·뒤집힌 절대 구간·0 이하 갯수) 조회하지 않는다.
    // 해석 가능 여부만 보므로 `now` 는 판정에 영향이 없는 상수를 쓴다.
    const rangeSpec = readSeriesRange(config.range, config.time_window_ms);
    if (!resolveSeriesWindow(rangeSpec, config.interval_ms, 0)) return '';
    let selectionPart: string;
    if (config.selection_mode === 'tag') {
      const tagFilters = config.tag_filters ?? {};
      // tag 모드는 태그가 하나도 없으면 비활성(idle) — 전체 키 폭주를 막는다.
      if (Object.keys(tagFilters).length === 0) return '';
      selectionPart =
        'tag:' +
        Object.keys(tagFilters)
          .sort()
          .map((k) => `${k}=${tagFilters[k]}`)
          .join(',');
    } else {
      if (!config.series || config.series.length === 0) return '';
      selectionPart =
        'keys:' +
        config.series
          .map((s) => {
            const tagPart = s.tags
              ? Object.keys(s.tags)
                  .sort()
                  .map((k) => `${k}=${s.tags![k]}`)
                  .join(',')
              : '';
            // alias(시리즈 표시 이름)도 포함해, 별칭 편집 시 재구독→재변환으로 범례
            // 이름이 반영되게 한다. seriesName 은 조회 시점에 alias 로 확정되므로,
            // alias 를 pollKey 에서 빼면 편집이 반영되지 않는다(범례 이름 안바뀜 버그).
            return `${s.key}|${s.field ?? ''}|${tagPart}|${s.alias ?? ''}`;
          })
          .join('');
    }
    return [
      // 해석된 현재 이름을 키에 포함해, 에이전트 이름 변경 시 재조회되게 한다.
      resolvedAgentName,
      config.namespace ?? 'default',
      // 범위(상대 창 / 절대 구간 / 갯수)가 재구독 축이다. 상대·갯수는 해석된 시각이
      // 아니라 설정값만 넣는다 — 해석 시각을 넣으면 폴링마다 키가 바뀐다.
      seriesRangeKey(rangeSpec),
      config.interval_ms,
      config.aggregation,
      // 채우기도 재구독 축이다 — 빼면 전략을 바꿔도 다음 폴링까지 화면이 그대로다.
      config.fill ?? '',
      config.fill_previous_max_ms ?? 0,
      config.fill_previous_overflow ?? '',
      config.fill_previous_overflow_value ?? 0,
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
      // 패널 단위 이름 형식도 재구독 축이다. 위 alias 주석이 기록한 "범례 이름
      // 안바뀜" 과 같은 결함이 패널 축에서 남아 있었다 — 이름은 시리즈별(alias)과
      // 패널별(format) 두 축에서 오므로 둘 다 키에 있어야 한다.
      config.series_name_format ?? '',
      selectionPart,
    ].join('|');
  }, [enabled, config, resolvedAgentName]);

  useEffect(() => {
    // 비활성/무효 설정: idle 로 리셋하고 폴링하지 않는다.
    if (pollKey === '' || !config) {
      setResult(EMPTY_RESULT);
      return;
    }

    let cancelled = false;
    let controller: AbortController | null = null;

    const refreshMs = Math.max(
      MIN_REFRESH_MS,
      config.refresh_interval_ms ?? DEFAULT_REFRESH_MS,
    );

    const run = async () => {
      const queryFn = optionsRef.current.queryMatrixFn ?? defaultQueryMatrix;
      const resolveKeysFn = optionsRef.current.resolveKeysFn ?? defaultResolveKeys;
      const nowFn = optionsRef.current.nowFn ?? Date.now;
      const now = nowFn();
      // 이전 진행 중 요청을 중단하고 새 컨트롤러를 만든다.
      // (tag 모드의 키 해석 fetch 와 매트릭스 조회가 같은 signal 을 공유한다.)
      controller?.abort();
      controller = new AbortController();
      const signal = controller.signal;
      // SPEC-WEB-006: 저장된 이름이 아닌 해석된 현재 에이전트 이름으로 조회한다.
      const agentName = resolvedAgentNameRef.current;
      try {
        // tag 모드: 폴링마다 tag_filters 매칭 키를 먼저 해석해 series[] 를 동적으로 만든다.
        // 태그 하위 키가 추가/삭제되면 다음 폴링에서 자동 반영된다.
        // keys 모드: 저장된 series[] 를 그대로 사용한다(기존 동작).
        let effectiveConfig = config;
        if (config.selection_mode === 'tag') {
          const resolvedKeys = await resolveKeysFn(
            agentName,
            config.tag_filters ?? {},
            signal,
          );
          if (cancelled || signal.aborted) return;
          // 매칭 키 0개: 빈 차트(기존 no-data 상태)로 표시하고 매트릭스 조회를 생략한다.
          if (resolvedKeys.length === 0) {
            setResult({
              entries: [],
              seriesEntries: new Map(),
              seriesStyles: new Map(),
              seriesNames: [],
              booleanSeries: new Set(),
              status: 'connected',
            });
            return;
          }
          effectiveConfig = tagResolvedConfig(config, resolvedKeys);
        }
        const { keys, seriesFilters } = buildKeysAndFilters(effectiveConfig);
        // 조회 구간은 범위 설정에서 해석한다(상대 기간 / 절대 구간 / 최근 N개).
        // pollKey 가 이미 해석 가능 여부를 걸렀으므로 여기서는 성립한다.
        const win = resolveSeriesWindow(
          readSeriesRange(config.range, config.time_window_ms),
          config.interval_ms,
          now,
        );
        if (!win) return;
        const query: SeriesMatrixQuery = {
          keys,
          ...(seriesFilters ? { seriesFilters } : {}),
          startMs: win.startMs,
          endMs: win.endMs,
          intervalMs: config.interval_ms,
          aggregation: config.aggregation,
          // 채우기는 서버가 계산한다. 값이 없을 때만 싣지 않는 규칙은 어댑터가
          // 소유하므로(`queryStoreMatrix`) 여기서는 설정을 그대로 넘긴다.
          ...(config.fill ? { fill: config.fill } : {}),
          ...(config.fill === 'previous' && config.fill_previous_max_ms
            ? { fillPreviousMaxMs: config.fill_previous_max_ms }
            : {}),
          ...(config.fill === 'previous' && config.fill_previous_overflow
            ? {
                fillPreviousOverflow: config.fill_previous_overflow,
                fillPreviousOverflowValue: config.fill_previous_overflow_value ?? 0,
              }
            : {}),
        };
        const matrix = await queryFn(agentName, query, signal);
        if (cancelled || signal.aborted) return;
        // 갯수 방식: 경계 정렬로 버킷이 더 딸려 올 수 있어 마지막 N개로 자른다.
        const limited =
          win.limitBuckets === undefined
            ? matrix
            : { ...matrix, rows: limitToRecent(matrix.rows, win.limitBuckets) };
        const converted = matrixToEntries(limited, effectiveConfig);
        setResult({
          entries: converted.entries,
          seriesEntries: converted.seriesEntries,
          seriesStyles: converted.seriesStyles,
          seriesNames: converted.seriesNames,
          booleanSeries: converted.booleanSeries,
          status: 'connected',
        });
      } catch (err) {
        // abort 로 인한 취소는 에러로 보지 않는다.
        if (cancelled || signal.aborted) return;
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setResult((prev) => ({
          ...prev,
          status: 'error',
          errorReason: err instanceof Error ? err.message : String(err),
        }));
      }
    };

    // 즉시 1회 + 인터벌 폴링.
    //
    // 탭이 숨으면 인터벌이 멈추고, 다시 보이면 즉시 1회 돈 뒤 재개한다 —
    // 아무도 보지 않는 동안의 조회를 내지 않으면서, 돌아왔을 때 옛 값이 떠
    // 있는 시간도 없앤다.
    setResult((prev) => ({ ...prev, status: 'connecting' }));
    const stopPolling = startVisiblePolling({
      intervalMs: refreshMs,
      run: () => {
        void run();
      },
    });

    return () => {
      cancelled = true;
      controller?.abort();
      stopPolling();
    };
    // pollKey 가 변경될 때만 재구독한다(config 객체 참조 변화는 무시).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pollKey]);

  // per-line 스타일(color/stroke_style/stroke_width/smooth)은 순수 config 표현값이므로
  // 데이터 재조회 없이도 즉시 반영되어야 한다. 폴링 effect 는 pollKey(키/필터/시간 파라미터)
  // 변경만 감지하므로, style-only 변경(예: 곡선 토글)은 pollKey 를 바꾸지 않아 fetch 시점에
  // 계산된 seriesStyles 가 stale 로 남는다(다음 조회/리마운트 전까지 미반영).
  // 따라서 마지막으로 알려진 시리즈 이름(result.seriesNames)에 현재 config 의 스타일을
  // 재매핑해 반환한다. 정렬 매핑 가능한 경우(컬럼 수 == 시리즈 수)에만 매핑하며, 그 외에는
  // 조회 시점 결과를 그대로 사용한다(matrixToEntries 의 aligned 규칙과 동일).
  const reactiveSeriesStyles = useMemo(() => {
    // tag 모드는 series[] 를 조회 시점에 동적으로 만들므로(config.series 는 무시),
    // fetch 시 계산된 스타일(인덱스 팔레트 색)을 그대로 사용한다.
    if (config?.selection_mode === 'tag') return result.seriesStyles;
    if (!config?.series || result.seriesNames.length !== config.series.length) {
      return result.seriesStyles;
    }
    const styles = new Map<string, StoreSeriesStyle>();
    result.seriesNames.forEach((name, j) => {
      const ref = config.series[j];
      if (ref && !styles.has(name)) {
        // 조회 시점 매핑(`matrixToEntries`)과 **같은 필드 집합**이어야 한다. 하나라도
        // 빠뜨리면 조회 때는 실렸던 값이 이 재매핑에서 조용히 사라진다 — 이 경로는
        // 스타일 전용 변경뿐 아니라 매 렌더에서 결과를 대체하므로, 빠진 필드는
        // "골랐는데 안 바뀜" 으로 영구히 남는다(시리즈별 그래프 모양이 그랬다).
        styles.set(name, {
          color: ref.color,
          stroke_style: ref.stroke_style,
          stroke_width: ref.stroke_width,
          smooth: ref.smooth,
          graph_style: ref.graph_style,
        });
      }
    });
    return styles;
  }, [config, result.seriesNames, result.seriesStyles]);

  // booleanSeries 도 순수 config(data_type) 파생값이므로 재조회 없이 반응적으로 계산한다.
  const reactiveBooleanSeries = useMemo(() => {
    // tag 모드는 동적 시리즈이므로 fetch 시 계산된 booleanSeries 를 그대로 사용한다.
    if (config?.selection_mode === 'tag') return result.booleanSeries;
    if (!config?.series || result.seriesNames.length !== config.series.length) {
      return result.booleanSeries;
    }
    const set = new Set<string>();
    result.seriesNames.forEach((name, j) => {
      if (config.series[j]?.data_type === 'boolean') set.add(name);
    });
    return set;
  }, [config, result.seriesNames, result.booleanSeries]);

  return {
    ...result,
    seriesStyles: reactiveSeriesStyles,
    booleanSeries: reactiveBooleanSeries,
  };
}
