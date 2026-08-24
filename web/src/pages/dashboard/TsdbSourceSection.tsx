// TSDB(외부 시계열 DB) 데이터소스 편집 섹션 — 에이전트 · bucket · 3단 드릴다운 선택.
//
// `data_source: 'tsdb'` 일 때 `StoreSourceSection` 이 이 컴포넌트를 마운트한다. Store
// 쪽 편집기와 **같은 조작감**을 목표로 하며, 시리즈 선택은 `SeriesSelectTable` 을
// 그대로 재사용한다(§2.15 [O1]) — 선택 표를 새로 만들면 두 소스에서 체크박스 규칙과
// 필터 동작이 갈린다.
//
// 세 가지가 Store 와 다르다.
//
//   1. **에이전트가 필수**다(§2.18). 고르기 전에는 소스가 비활성이므로 조회하지 않는다.
//   2. **bucket 축이 있다.** v2 는 `GET /buckets` 목록에서 고르고, v3 는 자유 입력이다
//      (OQ10). v3 에서는 그 값이 질의에 도달하지 않는다는 사실까지 드러낸다(§2.13).
//   3. **field 가 필수**다(§2.2). "첫 번째 숫자 필드" 폴백은 조용한 오답이므로 두지
//      않으며, 그래서 measurement → field → tag 3단 드릴다운이 필요하다(§2.15 [O1]).
//
// 디스커버리 응답은 캐시하지 않는다(§2.10 · UB1-12). react-query 대신 `useEffect` +
// `useState` 를 쓰는 이유도 같다 — 캐시가 없는 편이 계약에 맞고, Provider 없이도
// 렌더되므로 설정 다이얼로그 테스트가 가벼워진다.
//
// @spec SPEC-TSDB-002 §2.11 (E1) · §2.12 (E2) · §2.13 (S1) · §2.15 (O1) · §2.18 (U11)

import React, { useCallback, useEffect, useMemo, useState } from 'react';

import { useAgents } from '@/hooks/useAgent';
import { useTranslation } from '@/lib/i18n';
import {
  deriveGroupCombos,
  enumerateTsdbSeries,
  type TsdbSeriesEnumResult,
} from '@/services/api/tsdbSeriesEnum';
import { SeriesSelectTable, type SeriesRow } from '@/pages/agents/SeriesSelectTable';
import {
  fetchInfluxBuckets,
  fetchInfluxFieldKeys,
  fetchInfluxMeasurements,
  fetchInfluxTagKeys,
  fetchInfluxTagValues,
} from '@/services/api/influxdbManagement';
import type { PanelConfig } from '@/stores/uiStore';

import {
  defaultTsdbSource,
  storeSeriesId,
  STORE_SERIES_LIMIT,
  type TsdbSeriesRef,
  type TsdbSourceConfig,
} from './panels/charts/chartChannelTypes';
import {
  CAPABILITY_REASON_KEYS,
  panelSourceCapabilities,
  resolveInfluxVersion,
  TSDB_BACKEND_CAPABILITIES,
  type InfluxBackendVersion,
} from './panels/charts/panelDataSource';

type OnConfig = (config: Record<string, unknown>) => void;

// --- 지역 입력 헬퍼 ---
//
// `ChartPanelSections.tsx` 의 동명 헬퍼와 같은 시각 계약이지만 그쪽에서 import 하지
// 않는다 — `ChartPanelSections` 가 이 파일을 import 하므로 순환이 된다. 두 줄짜리
// 스타일 상수를 공유하려고 순환 의존을 만드는 것은 남는 장사가 아니다.

function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-50';
}

function FieldLabel(props: {
  label: string;
  children: React.ReactNode;
  hint?: string;
}): React.ReactElement {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {props.label}
      </label>
      {props.children}
      {props.hint && (
        <p className="mt-1 text-[10px] leading-snug text-(--color-text-muted)">{props.hint}</p>
      )}
    </div>
  );
}

/** 디스커버리 조회기 묶음. 기본값은 실제 REST 클라이언트이며 테스트에서 주입한다. */
export interface TsdbDiscoveryFetchers {
  fetchBuckets: (agentName: string) => Promise<Array<{ name: string }>>;
  fetchMeasurements: (agentName: string, bucket: string) => Promise<string[]>;
  fetchFieldKeys: (
    agentName: string,
    measurement: string,
    bucket?: string,
  ) => Promise<string[]>;
  fetchTagKeys: (
    agentName: string,
    measurement: string,
    bucket?: string,
  ) => Promise<string[]>;
  fetchTagValues: (
    agentName: string,
    measurement: string,
    tagKey: string,
    bucket?: string,
    filters?: Record<string, string>,
  ) => Promise<string[]>;
  /**
   * 시리즈 열거(D5) — 그룹 미리보기용.
   *
   * 그룹 기준을 걸면 어떤 태그 값들이 실제로 시리즈가 되는지 보여 줘야 한다.
   * 값 목록(D3 `fetchTagValues`)이 아니라 **열거**를 쓰는 이유는 사전 필터를
   * 반영한 **실재 조합**이 필요하기 때문이다 — `location=사무실` 로 좁힌 뒤의
   * `device.dev_eui` 는 전체 값 목록보다 작을 수 있다.
   */
  fetchSeriesEnum: (
    agentName: string,
    measurement: string,
    tags: Record<string, string>,
    bucket?: string,
    window?: { startMs: number; endMs: number },
  ) => Promise<TsdbSeriesEnumResult>;
}

const DEFAULT_FETCHERS: TsdbDiscoveryFetchers = {
  fetchBuckets: fetchInfluxBuckets,
  fetchMeasurements: fetchInfluxMeasurements,
  fetchFieldKeys: fetchInfluxFieldKeys,
  fetchTagKeys: fetchInfluxTagKeys,
  fetchTagValues: fetchInfluxTagValues,
  fetchSeriesEnum: (agentName, measurement, tags, bucket, window) =>
    enumerateTsdbSeries(agentName, {
      measurement,
      ...(bucket ? { bucket } : {}),
      ...(Object.keys(tags).length > 0 ? { tags } : {}),
      // 창을 넘기지 않으면 서버 기본값(30일)이 적용된다. 그러면 원시 행 상한
      // (20,000)이 옛 데이터로 먼저 차서 최근 시리즈가 열거에서 빠진다 —
      // 사용자에게는 "장비가 안 보인다" 로 나타난다. 패널이 실제로 그리는 창을
      // 그대로 쓴다.
      ...(window ? { startMs: window.startMs, endMs: window.endMs } : {}),
    }),
};

/**
 * 취소 가능한 목록 조회 훅.
 *
 * `enabled` 가 거짓이면 조회하지 않고 빈 목록을 유지한다 — 에이전트를 고르기 전에
 * 네트워크로 나가지 않게 하는 것이 이 플래그의 유일한 목적이다. 언마운트/의존성
 * 변경 시 `alive` 플래그로 늦게 도착한 응답을 버린다(경합 시 이전 목록이 나중 목록을
 * 덮어쓰는 사고 방지).
 */
function useDiscoveryList(
  enabled: boolean,
  load: () => Promise<string[]>,
  deps: readonly unknown[],
): { items: string[]; error: boolean; loading: boolean } {
  const [items, setItems] = useState<string[]>([]);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setError(false);
      setLoading(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError(false);
    void load()
      .then((list) => {
        if (!alive) return;
        setItems(list);
        setLoading(false);
      })
      .catch(() => {
        if (!alive) return;
        // 사유는 백엔드마다 다르지만(501 · 404 · 네트워크) 사용자가 할 일은 같다 —
        // 목록 없이 자유 입력으로 진행하거나 에이전트를 고쳐야 한다. 그래서 불리언 하나로 접는다.
        setItems([]);
        setError(true);
        setLoading(false);
      });
    return () => {
      alive = false;
    };
    // load 는 매 렌더 새 클로저이므로 의존성에서 제외하고 호출부가 deps 를 명시한다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { items, error, loading };
}

/**
 * 그룹 미리보기 — 현재 사전 필터 아래에서 그룹 기준이 만들 **실제 조합**을 구한다.
 *
 * 별도 훅인 이유는 `useDiscoveryList` 가 `string[]` 전용이기 때문이다. 조합은
 * `Record<string,string>` 이라 그 형상에 담기지 않는다.
 */
function useGroupCombos(
  enabled: boolean,
  load: () => Promise<TsdbSeriesEnumResult>,
  groupKeys: readonly string[],
  deps: readonly unknown[],
): {
  combos: Array<Record<string, string>>;
  error: boolean;
  loading: boolean;
  truncated: boolean;
} {
  const [combos, setCombos] = useState<Array<Record<string, string>>>([]);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(false);
  /**
   * 열거가 상한에 걸렸는가.
   *
   * **버리면 안 되는 신호다.** 상한에 걸리면 목록이 실제보다 짧은데, 알리지 않으면
   * 사용자는 그것이 전부인 줄 알고 "장비가 2개뿐" 이라고 결론 내린다.
   */
  const [truncated, setTruncated] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setCombos([]);
      setError(false);
      setLoading(false);
      setTruncated(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError(false);
    void load()
      .then((r) => {
        if (!alive) return;
        setCombos(deriveGroupCombos(r.series, groupKeys));
        setTruncated(r.truncated);
        setLoading(false);
      })
      .catch(() => {
        if (!alive) return;
        // 미리보기 실패가 편집을 막아서는 안 된다 — 목록 없이도 그룹 설정은 유효하다.
        setCombos([]);
        setTruncated(false);
        setError(true);
        setLoading(false);
      });
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { combos, error, loading, truncated };
}

/** 시리즈 참조 → `SeriesSelectTable` 의 선택 식별자. Store 와 같은 규칙을 쓴다. */
function seriesIdOf(ref: Pick<TsdbSeriesRef, 'key' | 'field' | 'tags'>): string {
  return storeSeriesId(ref.key, ref.field, ref.tags ?? {});
}

export function TsdbSourceSection({
  panel,
  onConfigChange,
  fetchers = DEFAULT_FETCHERS,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
  /** 디스커버리 조회기 주입(테스트용). 프로덕션은 실제 REST 클라이언트를 쓴다. */
  fetchers?: TsdbDiscoveryFetchers;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const tsdbSource =
    (config.tsdb_source as TsdbSourceConfig | undefined) ?? defaultTsdbSource();

  const caps = panelSourceCapabilities('tsdb');

  // --- 에이전트 선택 (§2.18: 필수) ---

  const { data: agentsResult } = useAgents();
  const tsdbAgents = useMemo(
    () =>
      (agentsResult?.data ?? []).filter(
        (a: { type: string }) => a.type === 'influxdb',
      ),
    [agentsResult],
  );
  const selectedAgent = useMemo(
    () =>
      tsdbSource.agent_id
        ? tsdbAgents.find((a: { id: string }) => a.id === tsdbSource.agent_id)
        : tsdbAgents.find((a: { name: string }) => a.name === tsdbSource.agent_name),
    [tsdbAgents, tsdbSource.agent_id, tsdbSource.agent_name],
  );
  const selectValue = selectedAgent?.id ?? tsdbSource.agent_id ?? '';
  const agentName = tsdbSource.agent_name;

  // 버전은 에이전트 config 에서 읽는다. 에이전트를 못 찾으면 v2 로 본다(넓은 쪽).
  const version: InfluxBackendVersion = resolveInfluxVersion(
    (selectedAgent as { config?: Record<string, unknown> } | undefined)?.config,
  );
  const backendCaps = TSDB_BACKEND_CAPABILITIES[version];

  const patch = useCallback(
    (p: Partial<TsdbSourceConfig>): void => {
      onConfigChange({ tsdb_source: { ...tsdbSource, ...p } });
    },
    [onConfigChange, tsdbSource],
  );

  // --- 드릴다운 로컬 상태 (config 가 아니라 편집 커서다) ---

  /**
   * 드릴다운 커서를 **저장된 선택에서 복원**한다(SPEC-TSDB-004 UB1-15).
   *
   * 커서 자체는 config 가 아니지만, 빈 값으로 시작하면 다이얼로그를 다시 열 때
   * measurement 셀렉트가 비어 보이고 그룹 기준 체크가 전부 풀려 있다 — 사용자에게는
   * "설정이 저장되지 않았다" 로 읽힌다. 실제로는 series 에 다 들어 있다.
   *
   * lazy 초기화라 최초 렌더의 값만 쓴다. 이후 사용자의 편집을 덮어쓰지 않는다.
   */
  const [measurement, setMeasurement] = useState(() => tsdbSource.series[0]?.key ?? '');
  const [tagKey, setTagKey] = useState('');
  const [tagValue, setTagValue] = useState('');
  const [tagFilters, setTagFilters] = useState<Record<string, string>>(
    () => tsdbSource.series[0]?.tags ?? {},
  );
  const [overLimit, setOverLimit] = useState(false);
  /**
   * 그룹 기준 태그 키(SPEC-TSDB-004 §2.1). 선택 시점에 시리즈 항목으로 옮겨진다.
   *
   * config 가 아니라 편집 커서인 이유는 tagFilters 와 같다 — 이미 선택된 시리즈의
   * 그룹 축을 바꾸는 것이 아니라, **앞으로 선택할** 시리즈에 붙일 축이다.
   */
  const [groupKeys, setGroupKeys] = useState<string[]>(
    () => tsdbSource.series[0]?.group_by ?? [],
  );

  const bucket = tsdbSource.bucket ?? '';

  const buckets = useDiscoveryList(
    agentName !== '' && backendCaps.bucketList,
    () => fetchers.fetchBuckets(agentName).then((bs) => bs.map((b) => b.name)),
    [agentName, backendCaps.bucketList],
  );
  const measurements = useDiscoveryList(
    agentName !== '',
    () => fetchers.fetchMeasurements(agentName, bucket),
    [agentName, bucket],
  );
  const fieldKeys = useDiscoveryList(
    agentName !== '' && measurement !== '',
    () => fetchers.fetchFieldKeys(agentName, measurement, bucket || undefined),
    [agentName, measurement, bucket],
  );
  const tagKeys = useDiscoveryList(
    agentName !== '' && measurement !== '',
    () => fetchers.fetchTagKeys(agentName, measurement, bucket || undefined),
    [agentName, measurement, bucket],
  );
  const groupPreview = useGroupCombos(
    agentName !== '' && measurement !== '' && groupKeys.length > 0,
    async () => {
      // 그룹 키가 **하나면** 태그 값 조회(D3)를 쓴다.
      //
      // 시리즈 열거는 접기 전 원시 행 20,000 에 상한이 걸려 있어, 고빈도
      // measurement 에서는 그 행이 소수 값으로만 채워져 값 일부만 나온다. 태그 값
      // 조회는 메타데이터 질의라 그 상한과 무관하며, 사전 필터도 서버가 반영한다.
      //
      // 키가 여럿이면 **조합**이 필요한데 태그 값 조회는 키별 값만 주므로
      // (곱하면 실재하지 않는 조합이 생긴다) 열거로 간다. 그쪽은 상한에 걸릴 수
      // 있으므로 truncated 를 그대로 표면화한다.
      if (groupKeys.length === 1) {
        const key = groupKeys[0]!;
        const values = await fetchers.fetchTagValues(
          agentName,
          measurement,
          key,
          bucket || undefined,
          tagFilters,
        );
        return {
          series: values.map((v) => ({ tags: { [key]: v }, fields: [] })),
          field_exact: true,
          count: values.length,
          truncated: false,
          window: { start_ms: 0, end_ms: 0 },
        };
      }
      // 패널이 실제로 그리는 창을 그대로 쓴다. 창을 안 넘기면 서버 기본값(30일)이
      // 적용되고, 그러면 원시 행 상한이 옛 데이터로 먼저 차서 최근 시리즈가
      // 열거에서 빠진다.
      const endMs = Date.now();
      const windowMs = tsdbSource.time_window_ms > 0 ? tsdbSource.time_window_ms : 0;
      return fetchers.fetchSeriesEnum(
        agentName,
        measurement,
        tagFilters,
        bucket || undefined,
        windowMs > 0 ? { startMs: endMs - windowMs, endMs } : undefined,
      );
    },
    groupKeys,
    [
      agentName,
      measurement,
      bucket,
      groupKeys.join(','),
      JSON.stringify(tagFilters),
      tsdbSource.time_window_ms,
    ],
  );

  const tagValues = useDiscoveryList(
    agentName !== '' && measurement !== '' && tagKey !== '',
    () => fetchers.fetchTagValues(agentName, measurement, tagKey, bucket || undefined),
    [agentName, measurement, tagKey, bucket],
  );

  // --- 선택 표 (SeriesSelectTable 재사용) ---

  /**
   * 선택 식별자 — 그룹 항목은 **고른 조합마다** id 를 하나씩 낸다.
   *
   * config 항목 1개가 표에서는 행 N개로 보이므로, 체크 상태도 그 축으로 펼쳐야
   * 한다. 펼치지 않으면 고른 그룹이 표에 반영되지 않는다.
   */
  const selectedIds = useMemo(() => {
    const out: string[] = [];
    for (const sr of tsdbSource.series) {
      const picks = sr.group_by && sr.group_by.length > 0 ? (sr.group_filter ?? []) : [];
      if (picks.length === 0) {
        out.push(seriesIdOf(sr));
        continue;
      }
      for (const combo of picks) {
        out.push(storeSeriesId(sr.key, sr.field, { ...(sr.tags ?? {}), ...combo }));
      }
    }
    return out;
  }, [tsdbSource.series]);

  /**
   * 후보 행 = 현재 measurement 의 field 키 × 현재 태그 필터.
   *
   * 태그를 드릴다운의 **3단계**로 두었으므로 행의 태그는 전부 같다. 태그별로 행을
   * 쪼개려면 `tag-values` 를 조합 폭발로 순회해야 하는데, 그 비용은 InfluxDB 쪽에서
   * 온전히 발생한다(§5). 사용자가 태그를 좁힌 뒤 필드를 고르는 순서가 실제 조작
   * 순서와도 맞는다.
   */
  /**
   * 후보 행 — 그룹 기준이 있으면 **조합마다 행이 하나씩** 생긴다
   * (SPEC-TSDB-004 §2.11). 사용자는 그중 볼 것을 고르고, 고른 조합이 항목의
   * `group_filter` 로 모인다.
   *
   * 그룹 기준이 없으면 종전과 같이 field 당 한 행이다.
   */
  const rows: SeriesRow[] = useMemo(() => {
    if (measurement === '') return [];
    if (groupKeys.length === 0) {
      return fieldKeys.items.map((field) => ({
        id: storeSeriesId(measurement, field, tagFilters),
        key: measurement,
        field,
        dataType: '',
        registration: '',
        tags: tagFilters,
      }));
    }
    // 조합이 아직 안 왔으면 행을 만들지 않는다 — 사전 필터를 반영하지 않은
    // 임시 행을 보여 주면 사용자가 없는 시리즈를 고르게 된다.
    return fieldKeys.items.flatMap((field) =>
      groupPreview.combos.map((combo) => ({
        id: storeSeriesId(measurement, field, { ...tagFilters, ...combo }),
        key: measurement,
        field,
        dataType: '',
        registration: '',
        tags: { ...tagFilters, ...combo },
      })),
    );
  }, [measurement, fieldKeys.items, tagFilters, groupKeys, groupPreview.combos]);

  /** 행 id → 그 행이 나타내는 그룹 조합. 그룹 기준이 없으면 비어 있다. */
  const comboById = useMemo(() => {
    const m = new Map<string, Record<string, string>>();
    if (groupKeys.length === 0 || measurement === '') return m;
    for (const field of fieldKeys.items) {
      for (const combo of groupPreview.combos) {
        m.set(storeSeriesId(measurement, field, { ...tagFilters, ...combo }), combo);
      }
    }
    return m;
  }, [measurement, fieldKeys.items, tagFilters, groupKeys, groupPreview.combos]);

  /** 현재 편집 커서의 그룹 축을 시리즈 항목 형태로 만든다. */
  const groupByPatch = useMemo(
    () => (groupKeys.length > 0 ? { group_by: [...groupKeys].sort() } : {}),
    [groupKeys],
  );

  /**
   * 선택 집합을 갱신한다. **48 상한을 강제**한다(§2.9) — 안내만 하고 통과시키면
   * 폴링당 요청 수가 상한 없이 늘어난다. 초과분은 반영하지 않고 배너를 띄운다.
   */
  const applySelection = useCallback(
    (next: TsdbSeriesRef[]): void => {
      if (next.length > STORE_SERIES_LIMIT) {
        setOverLimit(true);
        patch({ series: next.slice(0, STORE_SERIES_LIMIT) });
        return;
      }
      setOverLimit(false);
      patch({ series: next });
    },
    [patch],
  );

  const rowById = useCallback(
    (id: string): SeriesRow | undefined => rows.find((r) => r.id === id),
    [rows],
  );

  /**
   * 그룹 축 변경을 **이미 선택된 시리즈**에 반영한다.
   *
   * 선택 시점에만 반영하면 "시리즈를 고른 뒤 그룹 기준을 체크" 하는 순서에서
   * 아무 일도 일어나지 않는다 — 사용자는 기능이 고장 난 것으로 읽는다.
   *
   * 현재 measurement 의 항목만 건드린다. 다른 measurement 의 시리즈는 태그 키
   * 집합이 다르므로 같은 축을 걸 수 없다.
   */
  const applyGroupKeysToSelection = useCallback(
    (next: string[]): void => {
      if (measurement === '') return;
      let changed = false;
      const updated = tsdbSource.series.map((sr) => {
        if (sr.key !== measurement) return sr;
        const cur = sr.group_by ?? [];
        if (cur.length === next.length && cur.every((v, i) => v === next[i])) return sr;
        changed = true;
        const { group_by: _drop, ...rest } = sr;
        return next.length > 0 ? { ...rest, group_by: next } : rest;
      });
      if (changed) patch({ series: updated });
    },
    [measurement, patch, tsdbSource.series],
  );

  /**
   * 그룹 조합 하나를 켜고 끈다.
   *
   * 항목은 (measurement, field) 당 **하나**로 유지하고, 고른 조합을 그 항목의
   * `group_filter` 에 모은다. 조합마다 항목을 따로 만들면 요청이 조합 수만큼
   * 늘어 group by 로 얻은 이점이 사라진다 — 한 요청이 여러 그룹을 돌려주는 것이
   * 이 기능의 요지다.
   *
   * 마지막 조합을 끄면 항목 자체를 없앤다. 빈 `group_filter` 는 백엔드 규약상
   * "전 그룹" 이라 남겨 두면 끈 것이 오히려 전부 켜진다.
   */
  const toggleGroupPick = useCallback(
    (field: string, combo: Record<string, string>): void => {
      const same = (a: Record<string, string>, b: Record<string, string>): boolean => {
        const ka = Object.keys(a).sort();
        const kb = Object.keys(b).sort();
        return ka.length === kb.length && ka.every((k, i) => kb[i] === k && a[k] === b[k]);
      };
      const idx = tsdbSource.series.findIndex(
        (sr) => sr.key === measurement && sr.field === field && (sr.group_by?.length ?? 0) > 0,
      );
      const next = [...tsdbSource.series];
      if (idx < 0) {
        next.push({
          key: measurement,
          field,
          ...(Object.keys(tagFilters).length > 0 ? { tags: { ...tagFilters } } : {}),
          group_by: [...groupKeys].sort(),
          group_filter: [combo],
        });
        applySelection(next);
        return;
      }
      const cur = next[idx]!;
      const picks = cur.group_filter ?? [];
      const has = picks.some((c) => same(c, combo));
      const updated = has ? picks.filter((c) => !same(c, combo)) : [...picks, combo];
      if (updated.length === 0) {
        next.splice(idx, 1);
      } else {
        next[idx] = { ...cur, group_filter: updated };
      }
      applySelection(next);
    },
    [applySelection, groupKeys, measurement, tagFilters, tsdbSource.series],
  );

  const handleToggle = useCallback(
    (id: string): void => {
      const combo = comboById.get(id);
      if (combo) {
        const row = rowById(id);
        if (row) toggleGroupPick(row.field, combo);
        return;
      }
      const existing = tsdbSource.series.filter((s) => seriesIdOf(s) !== id);
      if (existing.length !== tsdbSource.series.length) {
        applySelection(existing);
        return;
      }
      const row = rowById(id);
      if (!row) return;
      applySelection([
        ...tsdbSource.series,
        {
          key: row.key,
          field: row.field,
          ...(Object.keys(row.tags).length > 0 ? { tags: { ...row.tags } } : {}),
          ...groupByPatch,
        },
      ]);
    },
    [applySelection, comboById, groupByPatch, rowById, toggleGroupPick, tsdbSource.series],
  );

  const handleSelectMany = useCallback(
    (ids: string[]): void => {
      // 그룹 모드에서는 조합을 항목별로 모아 **한 번에** 반영한다. id 마다
      // toggleGroupPick 을 부르면 앞선 호출의 결과를 뒤 호출이 덮어쓴다
      // (tsdbSource.series 가 그 사이 갱신되지 않는다).
      if (comboById.size > 0) {
        const byField = new Map<string, Array<Record<string, string>>>();
        for (const id of ids) {
          const combo = comboById.get(id);
          const row = rowById(id);
          if (!combo || !row) continue;
          const list = byField.get(row.field);
          if (list) list.push(combo);
          else byField.set(row.field, [combo]);
        }
        if (byField.size === 0) return;
        const next = [...tsdbSource.series];
        for (const [field, combos] of byField) {
          const idx = next.findIndex(
            (sr) =>
              sr.key === measurement && sr.field === field && (sr.group_by?.length ?? 0) > 0,
          );
          const sig = (c: Record<string, string>): string =>
            Object.keys(c)
              .sort()
              .map((k) => `${k}=${c[k]}`)
              .join(',');
          if (idx < 0) {
            next.push({
              key: measurement,
              field,
              ...(Object.keys(tagFilters).length > 0 ? { tags: { ...tagFilters } } : {}),
              group_by: [...groupKeys].sort(),
              group_filter: combos,
            });
            continue;
          }
          const cur = next[idx]!;
          const seen = new Set((cur.group_filter ?? []).map(sig));
          const merged = [...(cur.group_filter ?? [])];
          for (const c of combos) {
            if (seen.has(sig(c))) continue;
            seen.add(sig(c));
            merged.push(c);
          }
          next[idx] = { ...cur, group_filter: merged };
        }
        applySelection(next);
        return;
      }

      const known = new Set(tsdbSource.series.map(seriesIdOf));
      const added: TsdbSeriesRef[] = [];
      for (const id of ids) {
        if (known.has(id)) continue;
        const row = rowById(id);
        if (!row) continue;
        known.add(id);
        added.push({
          key: row.key,
          field: row.field,
          ...(Object.keys(row.tags).length > 0 ? { tags: { ...row.tags } } : {}),
          ...groupByPatch,
        });
      }
      if (added.length === 0) return;
      applySelection([...tsdbSource.series, ...added]);
    },
    [
      applySelection,
      comboById,
      groupByPatch,
      groupKeys,
      measurement,
      rowById,
      tagFilters,
      tsdbSource.series,
    ],
  );

  const handleClearMany = useCallback(
    (ids: string[]): void => {
      if (comboById.size > 0) {
        const drop = new Set(ids);
        const next: TsdbSeriesRef[] = [];
        for (const sr of tsdbSource.series) {
          if ((sr.group_by?.length ?? 0) === 0) {
            if (!drop.has(seriesIdOf(sr))) next.push(sr);
            continue;
          }
          const kept = (sr.group_filter ?? []).filter(
            (c) => !drop.has(storeSeriesId(sr.key, sr.field, { ...(sr.tags ?? {}), ...c })),
          );
          // 전부 해제된 항목은 없앤다 — 빈 group_filter 는 "전 그룹" 이라
          // 남겨 두면 해제가 오히려 전부 켜는 결과가 된다.
          if (kept.length > 0) next.push({ ...sr, group_filter: kept });
        }
        applySelection(next);
        return;
      }
      const drop = new Set(ids);
      applySelection(tsdbSource.series.filter((s) => !drop.has(seriesIdOf(s))));
    },
    [applySelection, comboById, tsdbSource.series],
  );

  return (
    <div className="space-y-3" data-testid="chart-tsdb-source">
      {/* Row 1: 에이전트 + 백엔드 표기. 에이전트는 필수이므로 가장 앞에 둔다(§2.18). */}
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[10rem] flex-1">
          <FieldLabel label={t('dashboard.chart.tsdbAgent')}>
            <select
              data-testid="chart-tsdb-agent-select"
              value={selectValue}
              onChange={(e) => {
                const id = e.target.value;
                setMeasurement('');
                setTagKey('');
                setTagValue('');
                setTagFilters({});
                setOverLimit(false);
                if (!id) {
                  // 미선택으로 초기화 — 소스가 비활성이 되어 조회가 멈춘다(§2.3).
                  patch({ agent_id: undefined, agent_name: '', series: [] });
                  return;
                }
                const agent = tsdbAgents.find((a: { id: string }) => a.id === id);
                // agent_id(정본) + agent_name(현재 이름 스냅샷). 에이전트를 바꾸면
                // 이전 스키마의 시리즈는 해석되지 않으므로 함께 비운다.
                patch({ agent_id: id, agent_name: agent?.name ?? '', series: [] });
              }}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.tsdbAgentSelect')}</option>
              {tsdbAgents.map((a: { id: string; name: string }) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
              {/* 저장된 에이전트가 목록에 없으면(비활성/삭제) 저장된 이름으로 선택 유지 */}
              {selectValue && !selectedAgent && (
                <option value={selectValue}>
                  {t('dashboard.chart.tsdbAgentInactive').replace(
                    '{name}',
                    tsdbSource.agent_name || selectValue,
                  )}
                </option>
              )}
            </select>
          </FieldLabel>
        </div>
        <div className="min-w-[8rem]">
          <FieldLabel label={t('dashboard.chart.tsdbBackendLabel')}>
            <p
              data-testid="chart-tsdb-backend"
              className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-1.5 font-mono text-xs text-(--color-text-secondary)"
            >
              {t(
                version === '3'
                  ? 'dashboard.chart.tsdbBackendInfluxV3'
                  : 'dashboard.chart.tsdbBackendInfluxV2',
              )}
            </p>
          </FieldLabel>
        </div>
      </div>

      {/* v3 관리 조작 안내(§2.13). 관리 버튼 자체는 이 화면에 두지 않는다 — 차트 설정에서
          버킷 삭제/초기화 같은 파괴적 조작을 노출하지 않는다. 능력 사실만 드러낸다. */}
      {!backendCaps.management && (
        <p
          data-testid="chart-tsdb-management-notice"
          aria-disabled="true"
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-[11px] leading-snug text-(--color-text-muted)"
        >
          {t(CAPABILITY_REASON_KEYS.managementV3)}
        </p>
      )}

      {/* Row 2: bucket. v2 = 목록, v3 = 자유 입력(OQ10). */}
      <FieldLabel
        label={t('dashboard.chart.tsdbBucket')}
        hint={
          backendCaps.bucketAffectsQuery
            ? undefined
            : t(CAPABILITY_REASON_KEYS.bucketV3)
        }
      >
        {backendCaps.bucketList ? (
          <select
            data-testid="chart-tsdb-bucket-select"
            value={bucket}
            disabled={agentName === ''}
            onChange={(e) => {
              setMeasurement('');
              setTagKey('');
              setTagFilters({});
              patch({ bucket: e.target.value || undefined });
            }}
            className={inputClass()}
          >
            <option value="">{t('dashboard.chart.tsdbBucketDefault')}</option>
            {buckets.items.map((b) => (
              <option key={b} value={b}>
                {b}
              </option>
            ))}
            {/* 저장된 버킷이 목록에 없어도 선택을 잃지 않는다(권한/삭제/조회 실패). */}
            {bucket && !buckets.items.includes(bucket) && (
              <option value={bucket}>{bucket}</option>
            )}
          </select>
        ) : (
          <input
            type="text"
            data-testid="chart-tsdb-bucket-input"
            value={bucket}
            disabled={agentName === ''}
            placeholder={t('dashboard.chart.tsdbBucketDefault')}
            onChange={(e) => patch({ bucket: e.target.value || undefined })}
            className={inputClass()}
          />
        )}
      </FieldLabel>
      {buckets.error && (
        <p data-testid="chart-tsdb-bucket-error" className="text-[11px] text-amber-500">
          {t('dashboard.chart.tsdbDiscoveryError')}
        </p>
      )}

      {/* Row 3: 빈 버킷 처리 전략. `avg` 는 어느 백엔드에도 대응물이 없어 비활성이다(§2.13). */}
      <FillStrategyField
        value={tsdbSource.fill ?? ''}
        onChange={(fill) => patch({ fill: fill === '' ? undefined : fill })}
        supported={caps.fillStrategies}
        avgSupported={caps.fillAvg}
        reasonKey={
          caps.fillStrategies ? CAPABILITY_REASON_KEYS.fillAvg : CAPABILITY_REASON_KEYS.fillStore
        }
        testId="chart-tsdb-fill"
      />

      {/* Row 4: measurement → field → tag 3단 드릴다운(§2.15 [O1]). */}
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[10rem] flex-1">
          <FieldLabel label={t('dashboard.chart.tsdbMeasurement')}>
            <select
              data-testid="chart-tsdb-measurement-select"
              value={measurement}
              disabled={agentName === ''}
              onChange={(e) => {
                setMeasurement(e.target.value);
                setTagKey('');
                setTagValue('');
                setTagFilters({});
                // 태그 키 집합이 measurement 마다 다르므로 그룹 축도 함께 비운다.
                setGroupKeys([]);
              }}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.tsdbMeasurementSelect')}</option>
              {measurements.items.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </FieldLabel>
        </div>
        <div className="min-w-[8rem] flex-1">
          <FieldLabel label={t('dashboard.chart.tsdbTagKey')}>
            <select
              data-testid="chart-tsdb-tag-key-select"
              value={tagKey}
              disabled={measurement === ''}
              onChange={(e) => {
                setTagKey(e.target.value);
                setTagValue('');
              }}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.tsdbTagKeySelect')}</option>
              {tagKeys.items.map((k) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </select>
          </FieldLabel>
        </div>
        <div className="min-w-[8rem] flex-1">
          <FieldLabel label={t('dashboard.chart.tsdbTagValue')}>
            <select
              data-testid="chart-tsdb-tag-value-select"
              value={tagValue}
              disabled={tagKey === ''}
              onChange={(e) => {
                const v = e.target.value;
                setTagValue(v);
                // 값을 고르는 즉시 필터에 반영한다 — "추가" 버튼을 한 단계 더 두면
                // 사용자가 고르고도 반영되지 않은 상태를 만들 수 있다.
                setTagFilters((prev) => {
                  const next = { ...prev };
                  if (v === '') delete next[tagKey];
                  else next[tagKey] = v;
                  return next;
                });
                // UB1-3 — 값을 고정한 키로는 나눌 수 없다(그룹이 항상 1개다).
                // 서버가 400 으로 거부하므로 UI 에서 먼저 해소한다.
                if (v !== '') {
                  const next = groupKeys.filter((k) => k !== tagKey);
                  setGroupKeys(next);
                  applyGroupKeysToSelection(next);
                }
              }}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.tsdbTagValueAny')}</option>
              {tagValues.items.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </FieldLabel>
        </div>
      </div>

      {/* 그룹 기준(group by) — 태그 값으로 시리즈를 나눈다(SPEC-TSDB-004 §2.1).
          값을 고정한 키는 후보에서 제외한다 — 그 키로 나누면 그룹이 항상 1개이고
          서버가 400 으로 거부한다(UB1-3). 숨기지 않고 **비활성 + 사유**로 두어
          "왜 못 고르는가" 가 화면에서 읽히게 한다(§2.13 [S1] 원칙 승계). */}
      {measurement !== '' && tagKeys.items.length > 0 && (
        <div data-testid="chart-tsdb-group-by">
          <FieldLabel label={t('dashboard.chart.tsdbGroupBy')}>
            <div className="flex flex-wrap gap-x-3 gap-y-1">
              {tagKeys.items.map((k) => {
                const pinned = tagFilters[k] !== undefined;
                const checked = groupKeys.includes(k);
                return (
                  <label
                    key={k}
                    className={`flex items-center gap-1 text-xs ${
                      pinned ? 'opacity-50' : ''
                    }`}
                    {...(pinned
                      ? { title: t('dashboard.chart.tsdbGroupByPinnedReason') }
                      : {})}
                  >
                    <input
                      type="checkbox"
                      data-testid={`chart-tsdb-group-by-${k}`}
                      checked={checked}
                      disabled={pinned}
                      aria-disabled={pinned}
                      onChange={(e) => {
                        const next = e.target.checked
                          ? [...groupKeys, k].sort()
                          : groupKeys.filter((x) => x !== k);
                        setGroupKeys(next);
                        applyGroupKeysToSelection(next);
                      }}
                    />
                    <span>{k}</span>
                  </label>
                );
              })}
            </div>
          </FieldLabel>
          <p className="mt-1 text-[11px] text-(--color-text-muted)">
            {groupKeys.length === 0
              ? t('dashboard.chart.tsdbGroupByNone')
              : t('dashboard.chart.tsdbGroupByHint').replace(
                  '{keys}',
                  [...groupKeys].sort().join(', '),
                )}
          </p>

          {/* 그룹 미리보기 — 실제로 어떤 태그 값이 시리즈가 되는지 보여 준다.
              그룹 기준만 표시하고 값을 감추면 사용자는 "몇 개가 나오는지" 를 적용
              후에야 알게 되고, 고카디널리티 태그를 실수로 걸었을 때 되돌리는
              비용이 커진다. 사전 필터를 반영한 **실재 조합**을 쓴다. */}
          {groupKeys.length > 0 && (
            <div data-testid="chart-tsdb-group-preview" className="mt-1">
              {groupPreview.loading ? (
                <p className="text-[11px] text-(--color-text-muted)">
                  {t('dashboard.chart.tsdbGroupPreviewLoading')}
                </p>
              ) : groupPreview.error ? (
                <p
                  data-testid="chart-tsdb-group-preview-error"
                  className="text-[11px] text-(--color-text-muted)"
                >
                  {t('dashboard.chart.tsdbGroupPreviewError')}
                </p>
              ) : groupPreview.combos.length === 0 ? (
                <p
                  data-testid="chart-tsdb-group-preview-empty"
                  className="text-[11px] text-amber-500"
                >
                  {t('dashboard.chart.tsdbGroupPreviewEmpty')}
                </p>
              ) : (
                <>
                  <p
                    data-testid="chart-tsdb-group-preview-count"
                    className="text-[11px] text-(--color-text-muted)"
                  >
                    {t('dashboard.chart.tsdbGroupPreviewCount').replace(
                      '{count}',
                      String(groupPreview.combos.length),
                    )}
                  </p>
                  {groupPreview.truncated && (
                    <p
                      data-testid="chart-tsdb-group-preview-truncated"
                      role="alert"
                      className="text-[11px] text-amber-500"
                    >
                      {t('dashboard.chart.tsdbGroupPreviewTruncated')}
                    </p>
                  )}
                  <ul className="mt-0.5 flex flex-wrap gap-1">
                    {groupPreview.combos.map((c) => {
                      const label = Object.keys(c)
                        .sort()
                        .map((k) => `${k}=${c[k] === '' ? '(없음)' : c[k]}`)
                        .join(', ');
                      return (
                        <li
                          key={label}
                          data-testid="chart-tsdb-group-preview-item"
                          className="rounded bg-(--color-surface-2) px-1.5 py-0.5 font-mono text-[10px] text-(--color-text-muted)"
                        >
                          {label}
                        </li>
                      );
                    })}
                  </ul>
                </>
              )}
            </div>
          )}
          {tagKeys.items.some((k) => tagFilters[k] !== undefined) && (
            <p
              data-testid="chart-tsdb-group-by-pinned"
              className="mt-1 text-[11px] text-(--color-text-muted)"
            >
              {t('dashboard.chart.tsdbGroupByPinnedReason')}
            </p>
          )}
        </div>
      )}

      {Object.keys(tagFilters).length > 0 && (
        <p data-testid="chart-tsdb-tag-filters" className="text-[11px] text-(--color-text-muted)">
          {Object.keys(tagFilters)
            .sort()
            .map((k) => `${k}=${tagFilters[k]}`)
            .join(', ')}
        </p>
      )}

      {/* 선택 표 — Store 와 같은 컴포넌트를 쓴다(§2.15 [O1]). 등록(registration) 컬럼은
          Store 메타데이터이므로 TSDB 에서는 노출하지 않는다. */}
      {agentName === '' ? (
        <p data-testid="chart-tsdb-no-agent" className="text-xs text-(--color-text-muted)">
          {t('dashboard.chart.tsdbSeriesNoAgent')}
        </p>
      ) : measurement === '' ? (
        <p data-testid="chart-tsdb-no-measurement" className="text-xs text-(--color-text-muted)">
          {t('dashboard.chart.tsdbSeriesNoMeasurement')}
        </p>
      ) : (
        <div data-testid="chart-tsdb-series-select">
          <SeriesSelectTable
            rows={rows}
            selectedIds={selectedIds}
            onToggle={handleToggle}
            onSelectMany={handleSelectMany}
            onClearMany={handleClearMany}
            showRegistration={false}
          />
        </div>
      )}

      {/* 선택된 항목 중 group by 축을 가진 것을 드러낸다(§2.11 [S1]).
          정확 일치 항목과 시각적으로 구분되어야 한다 — 하나가 런타임에 여러
          시리즈로 펼쳐진다는 사실이 설정 화면에서 보이지 않으면, 사용자는 차트에
          예상보다 많은 라인이 나오는 이유를 알 수 없다. */}
      {tsdbSource.series.some((sr) => (sr.group_by?.length ?? 0) > 0) && (
        <p
          data-testid="chart-tsdb-grouped-series"
          className="text-[11px] text-(--color-text-muted)"
        >
          {tsdbSource.series
            .filter((sr) => (sr.group_by?.length ?? 0) > 0)
            .map(
              (sr) =>
                `${sr.key}.${sr.field} — ` +
                t('dashboard.chart.tsdbGroupByBadge').replace(
                  '{keys}',
                  [...(sr.group_by ?? [])].sort().join(', '),
                ),
            )
            .join(' / ')}
        </p>
      )}

      {overLimit && (
        <p data-testid="chart-tsdb-over-limit" role="alert" className="text-[11px] text-amber-500">
          {t('dashboard.chart.tsdbSeriesOverLimit').replace(
            '{limit}',
            String(STORE_SERIES_LIMIT),
          )}
        </p>
      )}
    </div>
  );
}

/**
 * 빈 버킷 처리 전략 선택기 — **미지원 선택지를 숨기지 않고 비활성 + 사유로 표시**한다
 * (§2.13 [S1], SPEC-AUTH-006 §4.2 원칙 승계).
 *
 * 두 소스가 같은 컴포넌트를 쓴다.
 *
 *   - `store`: 백엔드가 fill 을 아예 모르므로 셀렉트 전체가 비활성이다.
 *   - `tsdb`: `avg` 만 비활성이다(Flux · InfluxQL 대응물 없음 → 400).
 *
 * 비활성 사유를 **툴팁이 아니라 읽히는 문구**로 두는 이유: 툴팁은 키보드/스크린리더
 * 사용자에게 도달하지 않는다. `aria-disabled` 는 포커스를 뺏지 않으면서 상태를 알린다.
 */
export function FillStrategyField({
  value,
  onChange,
  supported,
  avgSupported,
  reasonKey,
  testId,
}: {
  value: '' | 'null' | 'zero' | 'previous';
  onChange: (v: '' | 'null' | 'zero' | 'previous') => void;
  /** 소스가 fill 전략 자체를 지원하는가(`store` 는 거짓). */
  supported: boolean;
  /** `fill: 'avg'` 지원 여부. 현재 어느 소스도 지원하지 않는다. */
  avgSupported: boolean;
  /** 비활성 사유 i18n 키. */
  reasonKey: string;
  testId: string;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.chart.tsdbFill')}
      </label>
      <select
        data-testid={testId}
        value={value}
        aria-disabled={!supported}
        disabled={!supported}
        onChange={(e) => onChange(e.target.value as '' | 'null' | 'zero' | 'previous')}
        className={inputClass()}
      >
        <option value="">{t('dashboard.chart.tsdbFillNone')}</option>
        <option value="null">{t('dashboard.chart.tsdbFillNull')}</option>
        <option value="zero">{t('dashboard.chart.tsdbFillZero')}</option>
        <option value="previous">{t('dashboard.chart.tsdbFillPrevious')}</option>
        {/* 미지원 선택지도 목록에 남긴다 — 없애면 "왜 없지" 가 되고, 남기면 "왜 안 되는지" 가 보인다. */}
        <option value="avg" disabled aria-disabled="true" data-testid={`${testId}-avg`}>
          {t('dashboard.chart.tsdbFillAvg')}
        </option>
      </select>
      {(!supported || !avgSupported) && (
        <p data-testid={`${testId}-reason`} className="mt-1 text-[10px] leading-snug text-(--color-text-muted)">
          {t(reasonKey)}
        </p>
      )}
    </div>
  );
}
