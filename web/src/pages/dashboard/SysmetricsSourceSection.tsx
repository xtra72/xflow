// sysmetrics 데이터소스 편집 섹션 — 에이전트 + 시리즈 표.
//
// `data_source: 'sysmetrics'` 일 때 `StoreSourceSection` 이 이 컴포넌트를 마운트한다.
// 시리즈 선택은 Store · TSDB 와 **같은 표**(`SeriesSelectTable`)를 쓴다 — 선택 표를 소스마다
// 새로 만들면 체크박스 규칙과 필터 동작이 갈리고, 사용자는 같은 일을 소스 수만큼 배운다.
//
// 표의 한 행이 곧 한 시리즈다:
//
//   measurement ← 필드 이름 (`bytes_recv`)
//   tags        ← 분류 + 대상 (`{ category: 'network', interface: 'en0' }`)
//
// 그룹은 measurement 안에 눌러붙이지 않고 `category` 태그로 뺀다 — Store 어휘에서 그룹은
// 분류이지 이름의 일부가 아니고, 태그여야 거를 수 있다.
//
// 이 소스에만 있는 두 가지가 화면을 가른다.
//
//   1. **버킷도 집계도 없다.** 에이전트 폴링 주기가 곧 표본 간격이므로 인터벌·집계
//      컨트롤을 두지 않는다. 대신 폴링 주기와 표시 창을 고른다.
//   2. **이력이 없다.** 패널이 마운트된 이후 구간만 그려진다는 사실을 고지한다 —
//      감추면 사용자는 앞 구간이 비어 있는 이유를 찾아 헤맨다.
//
// 설정 순서는 Store · TSDB 와 **같다**(`ChartPanelSections.StoreSourceSection` 머리말의
// 정본 순서). 없는 축(집계 · 빈 구간 처리 · 구간 대표값)은 건너뛴다.

import React, { useCallback, useMemo } from 'react';
import { Info } from 'lucide-react';

import { useAgents, useAgent } from '@/hooks/useAgent';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { SeriesSelectTable, type SeriesRow } from '@/pages/agents/SeriesSelectTable';
import type { PanelConfig } from '@/stores/uiStore';

import ColorSwatchButton from './colorSwatchPalette';
import { SeriesNameFormatField } from './SeriesNameFormatField';
import { toSnapshot } from './panels/sysmetrics/sysMetricsSeries';
import {
  defaultSysmetricsSource,
  type SysmetricsSeriesRef,
  type SysmetricsSourceConfig,
} from './panels/charts/chartChannelTypes';
import {
  resolveSysmetricsSeries,
  sysmetricRowId,
  sysmetricsCandidateRows,
} from './panels/charts/sysmetricsSource';
import { findChartField } from './panels/sysmetrics/sysMetricsFields';
import { SYSMETRICS_COUNTER_MODES } from './panels/sysmetrics/sysMetricsItemOptions';
import { SeriesRangeField } from './SeriesRangeField';
import { readSeriesRange } from './panels/charts/seriesRange';
import { StoreIntervalField } from './ChartPanelSections';
import { sysmetricsWindow } from './panels/charts/useSysMetricsChartData';
import { FillStrategyField } from './TsdbSourceSection';
import {
  CAPABILITY_REASON_KEYS,
  panelSourceCapabilities,
} from './panels/charts/panelDataSource';

type OnConfig = (config: Record<string, unknown>) => void;

/**
 * 집계 옵션(UI 표기). Store·TSDB 와 **같은 라벨 키**를 재사용한다 — 소스를 갈아탄
 * 사용자가 같은 함수를 다른 이름으로 만나지 않게 하려는 것이다.
 */
const SYSMETRICS_AGG_OPTIONS: {
  value: SysmetricsSourceConfig['aggregation'];
  labelKey: string;
}[] = [
  { value: 'min', labelKey: 'tsdb.aggMin' },
  { value: 'max', labelKey: 'tsdb.aggMax' },
  { value: 'average', labelKey: 'tsdb.aggAverage' },
  { value: 'first', labelKey: 'tsdb.aggFirst' },
  { value: 'last', labelKey: 'tsdb.aggLast' },
];

function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
}

function LabeledField(props: {
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

/**
 * sysmetrics 소스 편집 섹션.
 *
 * 표의 후보 행은 **값 카탈로그 × 에이전트가 보고한 대상**이다. 대상을 손으로 입력받지
 * 않는 이유는 하나다 — 이름을 잘못 적으면 조용히 빈 선이 되고, 사용자는 오타를 확인할
 * 방법이 없다. 이미 고른 대상이 보고 목록에서 사라져도 행은 남는다(`sysmetricsCandidateRows`).
 */
export function SysmetricsSourceSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const source =
    (config.sysmetrics_source as SysmetricsSourceConfig | undefined) ??
    defaultSysmetricsSource();
  // 조회 경로와 같은 헬퍼로 읽는다(옛 config 는 이 필드들이 비어 있다).
  const queryWindow = sysmetricsWindow(source);
  // 조회 방식. 미지정은 이력이다 — 이 축이 생기기 전에 저장된 패널의 동작이 곧 기본값이다.
  const queryMode = source.query_mode ?? 'history';
  const isLive = queryMode === 'live';

  // --- 에이전트 선택 (필수) ---

  const { data: agentsResult } = useAgents();
  const sysAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a: { type: string }) => a.type === 'sysmetrics'),
    [agentsResult],
  );
  const selectedAgent = useMemo(
    () =>
      source.agent_id
        ? sysAgents.find((a: { id: string }) => a.id === source.agent_id)
        : sysAgents.find((a: { name: string }) => a.name === source.agent_name),
    [sysAgents, source.agent_id, source.agent_name],
  );
  const selectValue = selectedAgent?.id ?? source.agent_id ?? '';

  const patch = useCallback(
    (p: Partial<SysmetricsSourceConfig>): void => {
      onConfigChange({ sysmetrics_source: { ...source, ...p } });
    },
    [onConfigChange, source],
  );

  // --- 관측 대상 목록 (에이전트 스냅샷에서 읽는다) ---
  //
  // 쿼리 키는 패널 렌더 경로(`useSysMetricsSnapshot`)와 같은 것을 쓰므로 요청이 늘지 않는다.
  const { data: agentDetail } = useAgent(selectValue, 'summary');
  const targets = useMemo(
    () => toSnapshot((agentDetail as { state?: unknown } | undefined)?.state)?.targets,
    [agentDetail],
  );

  // --- 시리즈 표 ---

  const series = useMemo(() => source.series ?? [], [source.series]);
  const rows = useMemo(() => sysmetricsCandidateRows(targets, series), [targets, series]);
  const selectedIds = useMemo(() => series.map(sysmetricRowId), [series]);

  /** 행 id → 그 행이 가리키는 (값, 대상). 체크할 때 시리즈로 추가할 값이다. */
  const refById = useMemo(() => {
    const map = new Map<string, SysmetricsSeriesRef>();
    for (const row of rows) {
      // config 에 넣는 키는 **카탈로그 키**(`metricKey`)다. 표의 `key` 컬럼은 표시용
      // measurement(`bytes_recv`)라 그대로 저장하면 카탈로그를 찾지 못한다.
      map.set(row.id, { key: row.metricKey, ...(row.target ? { target: row.target } : {}) });
    }
    return map;
  }, [rows]);

  const applySelection = useCallback(
    (next: SysmetricsSeriesRef[]): void => patch({ series: next }),
    [patch],
  );

  const handleToggle = useCallback(
    (id: string): void => {
      const idx = series.findIndex((s) => sysmetricRowId(s) === id);
      if (idx >= 0) {
        applySelection(series.filter((_, i) => i !== idx));
        return;
      }
      const ref = refById.get(id);
      if (ref) applySelection([...series, ref]);
    },
    [series, refById, applySelection],
  );

  const handleSelectMany = useCallback(
    (ids: string[]): void => {
      const have = new Set(selectedIds);
      const added = ids
        .filter((id) => !have.has(id))
        .map((id) => refById.get(id))
        .filter((r): r is SysmetricsSeriesRef => r !== undefined);
      if (added.length > 0) applySelection([...series, ...added]);
    },
    [selectedIds, refById, series, applySelection],
  );

  const handleClearMany = useCallback(
    (ids: string[]): void => {
      const drop = new Set(ids);
      applySelection(series.filter((s) => !drop.has(sysmetricRowId(s))));
    },
    [series, applySelection],
  );

  /** 실제로 그려질 줄 — 이름 placeholder(내장 표기)를 행 상세에서 쓴다. */
  const resolved = useMemo(() => resolveSysmetricsSeries(source), [source]);
  const lineById = useMemo(
    () => new Map(resolved.map((line) => [sysmetricRowId(line.ref), line])),
    [resolved],
  );

  const patchSeriesAt = useCallback(
    (idx: number, patchRef: Partial<SysmetricsSeriesRef>): void => {
      applySelection(series.map((s, i) => (i === idx ? { ...s, ...patchRef } : s)));
    },
    [series, applySelection],
  );

  /**
   * 고른 행 아래에 이름·색 편집을 붙인다.
   *
   * 표 바깥에 두 번째 목록으로 빼지 않는 이유: 같은 시리즈가 두 번 보이고 어느 쪽이
   * 정본인지 알 수 없게 된다(TSDB 섹션이 같은 규칙을 쓴다).
   */
  const renderRowDetail = useCallback(
    (row: SeriesRow): React.ReactNode => {
      const idx = series.findIndex((s) => sysmetricRowId(s) === row.id);
      if (idx < 0) return null; // 고르지 않은 행에는 설정할 것이 없다
      const ref = series[idx]!;
      return (
        <span className="flex items-center gap-1">
          <ColorSwatchButton
            color={ref.color}
            onChange={(c) => patchSeriesAt(idx, { color: c })}
            ariaLabel={t('dashboard.chart.storeSeriesColorAria')}
            testId={`chart-sysmetrics-color-${row.id}`}
          />
          <input
            type="text"
            data-testid={`chart-sysmetrics-alias-${row.id}`}
            value={ref.alias ?? ''}
            aria-label={t('dashboard.chart.sysmetricsSeriesAlias')}
            placeholder={lineById.get(row.id)?.defaultName ?? ''}
            onChange={(e) => patchSeriesAt(idx, { alias: e.target.value || undefined })}
            className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-0.5 text-[11px]"
          />
          {/* 누적 카운터에만 낸다. 상태값(비율·용량)에는 환산할 것이 없어 고를 것이 없다. */}
          {findChartField(ref.key)?.rate && (
            <select
              data-testid={`chart-sysmetrics-mode-${row.id}`}
              value={ref.mode ?? 'rate'}
              aria-label={t('sysmetrics.settings.counterMode')}
              onChange={(e) =>
                // 기본값은 저장하지 않는다 — 남기면 config 가 기본값으로 불어나고,
                // 나중에 기본이 바뀌어도 옛 패널만 옛 값에 묶인다.
                patchSeriesAt(idx, { mode: e.target.value === 'total' ? 'total' : undefined })
              }
              className="rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-0.5 text-[11px]"
            >
              {SYSMETRICS_COUNTER_MODES.map((mode) => (
                <option key={mode} value={mode}>
                  {t(`sysmetrics.counterModes.${mode}`)}
                </option>
              ))}
            </select>
          )}
        </span>
      );
    },
    [series, lineById, patchSeriesAt, t],
  );

  return (
    <div className="space-y-3" data-testid="chart-sysmetrics-section">
      {/* 에이전트 선택 — TSDB/Store 와 같은 자리·같은 모양. */}
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[10rem] flex-1">
          <LabeledField label={t('dashboard.chart.sysmetricsAgent')}>
            <select
              data-testid="chart-sysmetrics-agent-select"
              value={selectValue}
              onChange={(e) => {
                const id = e.target.value;
                if (!id) {
                  patch({ agent_id: undefined, agent_name: '', series: [] });
                  return;
                }
                const agent = sysAgents.find((a: { id: string }) => a.id === id);
                // 에이전트가 바뀌면 대상 목록도 달라지므로 시리즈를 비운다 — 다른 호스트의
                // 인터페이스 이름이 남아 있으면 조용히 빈 선이 된다.
                patch({ agent_id: id, agent_name: agent?.name ?? '', series: [] });
              }}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.sysmetricsAgentSelect')}</option>
              {sysAgents.map((a: { id: string; name: string }) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </LabeledField>
        </div>
      </div>

      {/* 조회 방식 — 무엇을 그릴지가 아니라 **언제 것을 그릴지**를 고른다.
          이력은 에이전트 버퍼를 구간 질의하고, 실시간은 스냅샷을 오는 대로 이어 붙인다.
          아래 창·인터벌·집계는 이력에서만 뜻이 있으므로 실시간에서는 내린다 — 남겨 두면
          "고쳤는데 아무 일도 안 일어나는" 칸이 된다. */}
      <LabeledField label={t('dashboard.chart.sysmetricsQueryMode')}>
        <div className="flex gap-1" role="radiogroup">
          {(['history', 'live'] as const).map((m) => {
            const selected = queryMode === m;
            return (
              <button
                key={m}
                type="button"
                role="radio"
                aria-checked={selected}
                data-testid={`chart-sysmetrics-query-mode-${m}`}
                onClick={() => patch({ query_mode: m })}
                className={cn(
                  'rounded-md border px-2.5 py-1 text-xs transition-colors',
                  selected
                    ? 'border-blue-500 bg-blue-500/10 text-blue-600 dark:text-blue-400'
                    : 'border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                )}
              >
                {t(`dashboard.chart.sysmetricsQueryMode${m === 'history' ? 'History' : 'Live'}`)}
              </button>
            );
          })}
        </div>
      </LabeledField>

      {/* 모드가 감수하는 것을 고지한다 — 감추면 이력에서는 과거가 안 보이는 이유를,
          실시간에서는 패널을 닫으면 선이 사라지는 이유를 찾아 헤맨다. */}
      <div className="flex items-start gap-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2">
        <Info className="mt-0.5 size-3.5 shrink-0 text-(--color-text-muted)" />
        <p
          data-testid="chart-sysmetrics-mode-notice"
          className="text-[11px] leading-snug text-(--color-text-muted)"
        >
          {t(isLive ? 'dashboard.chart.sysmetricsLiveNotice' : 'dashboard.chart.sysmetricsHistoryNotice')}
        </p>
      </div>

      {/* 창·인터벌·집계·빈 버킷은 **이력에서만** 뜻이 있다. 실시간에는 버킷이 없고
          표시 창은 오래된 점을 버리는 기준일 뿐이라, 같은 칸을 두면 뜻이 갈린다. */}
      {!isLive && (
        <>
        {/* 조회 창 — Store 와 **같은 컴포넌트**를 같은 차례로 쓴다. 에이전트가 이력을
            들고 있으므로 이 소스도 "구간을 질의해 버킷으로 접는" 같은 모델이다.
            값은 조회 경로와 **같은 헬퍼**로 읽는다 — 이 필드들이 생기기 전에 저장된
            패널에는 값이 없고, 화면과 조회가 서로 다른 기본값을 쓰면 설정에 보이는
            창과 실제 조회 구간이 어긋난다. */}
        <SeriesRangeField
          range={readSeriesRange(source.range, queryWindow.time_window_ms)}
          onChange={(range) => patch({ range })}
          testIdPrefix="chart-sysmetrics"
        />

        <StoreIntervalField
          intervalMs={queryWindow.interval_ms}
          timeWindowMs={queryWindow.time_window_ms}
          onChange={(interval_ms) => patch({ interval_ms })}
          testIdPrefix="chart-sysmetrics"
        />

        {/* 인터벌 집계 — 버킷 하나를 대표하는 값. Store·TSDB 와 같은 어휘를 같은 자리에서
            고른다. 종전에는 config 에만 있고 조작 통로가 없어 저장된 값이 무엇이든
            바꿀 수 없었다. */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.chart.tsdbAggregation')}
          </label>
          <select
            data-testid="chart-sysmetrics-aggregation"
            value={queryWindow.aggregation}
            onChange={(e) =>
              patch({ aggregation: e.target.value as SysmetricsSourceConfig['aggregation'] })
            }
            className={inputClass()}
          >
            {SYSMETRICS_AGG_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.labelKey)}
              </option>
            ))}
          </select>
        </div>

        {/* 빈 버킷 처리 — 이 소스도 지원하지 않으므로 **비활성 + 사유**로 표시한다.
            숨기지 않는 이유는 Store 쪽과 같다: 선택지가 없으면 사용자는 "이 소스에는
            없는 기능" 인지 "내가 못 찾는 것" 인지 구분할 수 없다. */}
        <FillStrategyField
          value=""
          onChange={() => {}}
          supported={panelSourceCapabilities('sysmetrics').fillStrategies}
          avgSupported={panelSourceCapabilities('sysmetrics').fillAvg}
          reasonKey={CAPABILITY_REASON_KEYS.fillSysmetrics}
          testId="chart-sysmetrics-fill"
        />
        </>
      )}

      {/* 시리즈 이름 형식 — store/tsdb 와 같은 편집기, 토큰 표본도 같은 어휘다. */}
      <SeriesNameFormatField
        value={source.series_name_format}
        onChange={(v) => patch({ series_name_format: v })}
        sample={
          resolved[0] ? { key: resolved[0].measurement, tags: resolved[0].tags } : undefined
        }
        testIdPrefix="chart-sysmetrics-series-name-format"
      />

      {/* 시리즈 표 — Store·TSDB 와 같은 표. 한 행이 한 시리즈다. */}
      <LabeledField
        label={t('dashboard.chart.sysmetricsSeries')}
        hint={t('dashboard.chart.sysmetricsSeriesHint')}
      >
        <div data-testid="chart-sysmetrics-series-table">
          <SeriesSelectTable
            rows={rows}
            selectedIds={selectedIds}
            onToggle={handleToggle}
            onSelectMany={handleSelectMany}
            onClearMany={handleClearMany}
            showRegistration={false}
            renderRowDetail={renderRowDetail}
          />
        </div>
      </LabeledField>

      {/* 선택 요약 — 개별 편집은 위 표의 각 행에서 한다(TSDB 와 같은 규칙).
          두 목록으로 나누면 같은 시리즈가 두 번 보인다. */}
      <div data-testid="chart-sysmetrics-selected-count" className="flex items-center gap-2">
        <span className="text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.chart.sysmetricsSelected')}
        </span>
        <span className="text-[11px] text-(--color-text-muted)">{series.length}</span>
        {series.length > 0 && (
          <button
            type="button"
            data-testid="chart-sysmetrics-selected-clear"
            onClick={() => applySelection([])}
            className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-muted)"
          >
            {t('agents.sysResource.clear')}
          </button>
        )}
      </div>
    </div>
  );
}
