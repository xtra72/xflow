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
  ) => Promise<string[]>;
}

const DEFAULT_FETCHERS: TsdbDiscoveryFetchers = {
  fetchBuckets: fetchInfluxBuckets,
  fetchMeasurements: fetchInfluxMeasurements,
  fetchFieldKeys: fetchInfluxFieldKeys,
  fetchTagKeys: fetchInfluxTagKeys,
  fetchTagValues: fetchInfluxTagValues,
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
  const tagValues = useDiscoveryList(
    agentName !== '' && measurement !== '' && tagKey !== '',
    () => fetchers.fetchTagValues(agentName, measurement, tagKey, bucket || undefined),
    [agentName, measurement, tagKey, bucket],
  );

  // --- 선택 표 (SeriesSelectTable 재사용) ---

  const selectedIds = useMemo(
    () => tsdbSource.series.map(seriesIdOf),
    [tsdbSource.series],
  );

  /**
   * 후보 행 = 현재 measurement 의 field 키 × 현재 태그 필터.
   *
   * 태그를 드릴다운의 **3단계**로 두었으므로 행의 태그는 전부 같다. 태그별로 행을
   * 쪼개려면 `tag-values` 를 조합 폭발로 순회해야 하는데, 그 비용은 InfluxDB 쪽에서
   * 온전히 발생한다(§5). 사용자가 태그를 좁힌 뒤 필드를 고르는 순서가 실제 조작
   * 순서와도 맞는다.
   */
  const rows: SeriesRow[] = useMemo(() => {
    if (measurement === '') return [];
    return fieldKeys.items.map((field) => ({
      id: storeSeriesId(measurement, field, tagFilters),
      key: measurement,
      field,
      dataType: '',
      registration: '',
      tags: tagFilters,
    }));
  }, [measurement, fieldKeys.items, tagFilters]);

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

  const handleToggle = useCallback(
    (id: string): void => {
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
    [applySelection, groupByPatch, rowById, tsdbSource.series],
  );

  const handleSelectMany = useCallback(
    (ids: string[]): void => {
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
    [applySelection, groupByPatch, rowById, tsdbSource.series],
  );

  const handleClearMany = useCallback(
    (ids: string[]): void => {
      const drop = new Set(ids);
      applySelection(tsdbSource.series.filter((s) => !drop.has(seriesIdOf(s))));
    },
    [applySelection, tsdbSource.series],
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
