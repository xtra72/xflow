// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Pause, Play, TrendingUp } from 'lucide-react';
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceArea,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import {
  buildEnumLabelMap,
  formatEnumValue,
  getByPath,
  resolveAxisFont,
  SERIES_PALETTE,
  STROKE_DASHARRAY,
  THRESHOLD_DEFAULT_COLORS,
  type AxisFontStyle,
  type ChannelRefConfig,
  type ChartEntry,
  type LegendConfig,
  type LineChartPanelConfig,
  type StoreSourceConfig,
  type TimeWindowMode,
  type YAxisDataType,
  type YEnumLabel,
  type YThreshold,
  type YAxisMode,
} from './chartChannelTypes';
import { ChartLegend } from './ChartLegend';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  computeNiceTimeTicks,
  formatTimeShort,
  formatTimestamp,
  toLineValue,
} from './chartChannelUtils';
import type { ChartConnectionStatus } from '@/services/ws/chartChannel';
import { chartDataToCsv, downloadCsv } from './csvExport';
import { useChartChannel } from './useChartChannel';
import { useChartChannels, type ChannelState } from './useChartChannels';
import { useStoreChartData } from './useStoreChartData';
import { usePanelTitleVisible } from '../../panelChromeContext';

interface LineChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

const DEFAULT_MAX_POINTS = 100;

/**
 * 사용자가 max_points 를 명시하지 않았고 시간 윈도우(recent_window_sec) 가 설정된 경우,
 * 1Hz 업데이트를 가정해 윈도우 길이의 2배(안전 마진)를 기본값으로 사용한다.
 * 최대 5,000 으로 캡 — 메모리 폭주 방지.
 *
 * 이전: 항상 DEFAULT_MAX_POINTS(100) 사용 → 10분 윈도우 + 1Hz 업데이트 시
 *       ~1.67분만 버퍼 보유 → 차트가 중간에서 끊겨 보이던 문제 해결.
 */
function resolveMaxPoints(cfg: LineChartPanelConfig): number {
  if (cfg.max_points !== undefined && cfg.max_points > 0) return cfg.max_points;
  if (cfg.time_window_mode === 'recent' && cfg.recent_window_sec && cfg.recent_window_sec > 0) {
    return Math.min(5000, Math.max(DEFAULT_MAX_POINTS, cfg.recent_window_sec * 2));
  }
  return DEFAULT_MAX_POINTS;
}
const DEFAULT_RECENT_WINDOW_SEC = 600;
const DEFAULT_REFRESH_MS = 1000;
const MIN_REFRESH_MS = 200;
const MAX_REFRESH_MS = 60_000;
const DEFAULT_Y_PAD_PCT = 5;
const MAX_Y_PAD_PCT = 50;

/** multi-series 색상 팔레트 */
// 시리즈 자동 색상 팔레트(설정 편집기와 공유). @see chartChannelTypes.SERIES_PALETTE
const SERIES_COLORS = SERIES_PALETTE;

function parseConfig(config: Record<string, unknown>): LineChartPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    channels: config.channels as ChannelRefConfig[] | undefined,
    display_field: (config.display_field as string) ?? 'value',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    x_label: config.x_label as string | undefined,
    x_label_font: config.x_label_font as AxisFontStyle | undefined,
    x_tick_font: config.x_tick_font as AxisFontStyle | undefined,
    y_label_font: config.y_label_font as AxisFontStyle | undefined,
    y_tick_font: config.y_tick_font as AxisFontStyle | undefined,
    y_axis_type: config.y_axis_type as YAxisDataType | undefined,
    y_enum_labels: config.y_enum_labels as YEnumLabel[] | undefined,
    y_min: config.y_min as number | undefined,
    y_max: config.y_max as number | undefined,
    y_axis_mode: config.y_axis_mode as YAxisMode | undefined,
    y_axis_padding_pct: config.y_axis_padding_pct as number | undefined,
    y_label: config.y_label as string | undefined,
    y_unit: config.y_unit as string | undefined,
    y_thresholds: config.y_thresholds as YThreshold[] | undefined,
    time_window_mode: config.time_window_mode as TimeWindowMode | undefined,
    recent_window_sec: config.recent_window_sec as number | undefined,
    fixed_start_ms: config.fixed_start_ms as number | undefined,
    fixed_end_ms: config.fixed_end_ms as number | undefined,
    time_window_refresh_ms: config.time_window_refresh_ms as number | undefined,
    legend: config.legend as LegendConfig | undefined,
    smooth: (config.smooth as boolean) ?? false,
    multi_series_field: config.multi_series_field as string | undefined,
  };
}

/** 채널 상태들의 status 를 통합 — 가장 심각한 상태가 우세. */
function aggregateStatus(states: ChartConnectionStatus[]): ChartConnectionStatus {
  if (states.length === 0) return 'idle';
  const order: ChartConnectionStatus[] = [
    'error',
    'closed',
    'disconnected',
    'connecting',
    'connected',
    'idle',
  ];
  for (const s of order) {
    if (states.includes(s)) return s;
  }
  return states[0]!;
}

/**
 * 차트의 최신 timestamp 행에서 critical 임계 초과 여부 판정.
 * 다중 시리즈 시 어느 한 시리즈라도 critical 임계 위면 true.
 */
function isCriticalBreached(
  rows: Array<Record<string, unknown>>,
  seriesKeys: string[],
  thresholds: YThreshold[] | undefined,
): boolean {
  if (!thresholds || thresholds.length === 0 || rows.length === 0) return false;
  const criticals = thresholds.filter((t) => t.severity === 'critical');
  if (criticals.length === 0) return false;
  const last = rows[rows.length - 1]!;
  for (const key of seriesKeys) {
    const v = last[key];
    if (typeof v !== 'number' || !Number.isFinite(v)) continue;
    for (const t of criticals) {
      if (v >= t.value) return true;
    }
  }
  return false;
}

function clamp(n: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, n));
}

/**
 * 시간 윈도우 모드에 따라 entries 를 필터링한다.
 *
 * recent / fixed 모드에서는 윈도우 시작 시각 직전의 마지막 데이터 포인트(앵커)를
 * 한 개 추가로 포함시킨다 — recharts 가 라인을 그릴 때 이 앵커 → 첫 가시 포인트
 * 구간을 그리면서 X축 시작 경계를 자연스럽게 가로지른다.
 * X축 도메인은 변하지 않으므로 앵커 포인트 자체는 화면 밖에 있고, 라인만
 * 시작 경계까지 이어져 보인다.
 *
 * entries 는 timestamp 오름차순으로 정렬되어 있다고 가정한다.
 */
function filterByTimeWindow(
  entries: ChartEntry[],
  mode: TimeWindowMode,
  now: number,
  windowSec: number,
  startMs: number | undefined,
  endMs: number | undefined,
): ChartEntry[] {
  if (mode === 'recent') {
    const start = now - windowSec * 1000;
    return filterWithLeftAnchor(entries, start, now);
  }
  if (mode === 'fixed') {
    const s = startMs ?? Number.NEGATIVE_INFINITY;
    const e = endMs ?? Number.POSITIVE_INFINITY;
    return filterWithLeftAnchor(entries, s, e);
  }
  return entries;
}

/**
 * `[start, end]` 범위 안의 entries 에 더해, start 직전의 마지막 entry 한 개를
 * 앵커로 포함시킨다. 라인이 좌측 경계를 가로질러 그려지도록 하기 위함.
 *
 * 알고리즘:
 *   - start 이상 ~ end 이하 entries 를 모은다.
 *   - 그 첫 entry 가 정확히 start 가 아니라면, start 직전의 가장 가까운 entry 를
 *     앞에 prepend 한다 (앵커). end 도 동일 원리로 우측에 적용 가능하지만 recent
 *     모드에서는 end=now 라 의미가 적어 좌측만 처리한다.
 */
function filterWithLeftAnchor(
  entries: ChartEntry[],
  start: number,
  end: number,
): ChartEntry[] {
  const visible: ChartEntry[] = [];
  let anchor: ChartEntry | undefined;
  for (const e of entries) {
    if (e.timestamp < start) {
      // start 이전 entries 중 가장 마지막 것을 앵커로 보관 (overwrite).
      anchor = e;
      continue;
    }
    if (e.timestamp > end) break;
    visible.push(e);
  }
  if (anchor && (visible.length === 0 || visible[0]!.timestamp > start)) {
    return [anchor, ...visible];
  }
  return visible;
}

interface NormalizedChannel {
  ref: ChannelRefConfig;
  state: ChannelState;
}

export default function LineChartPanel({ panelId: _panelId, title, config }: LineChartPanelProps) {
  const showTitle = usePanelTitleVisible();
  const { t } = useTranslation();
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source === 'store' 면 Store 소스에서 시리즈를 가져온다(공존).
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  // 태그 자동 확장 모드는 series[] 가 비어 있고 tag_filters 로 키를 폴링 시점마다 동적
  // 해석하므로, series 길이가 아닌 tag_filters 존재로도 store 모드를 활성화한다(SPEC-WEB-005).
  const storeTagActive =
    storeSource?.selection_mode === 'tag' &&
    Object.keys(storeSource.tag_filters ?? {}).length > 0;
  const isStore =
    config.data_source === 'store' &&
    ((storeSource?.series?.length ?? 0) > 0 || storeTagActive);
  const isMultiMode = !isStore && (cfg.channels?.length ?? 0) > 0;
  // recent_window_sec 가 있으면 거기에 맞춰 버퍼 크기 자동 결정.
  const effectiveMaxPoints = resolveMaxPoints(cfg);

  // 세 hook 모두 항상 호출 (React hook 규칙). 비활성 경로는 idle 상태로 유지.
  const singleResult = useChartChannel(
    isStore || isMultiMode ? undefined : cfg.channel_name || undefined,
    { maxPoints: effectiveMaxPoints },
  );
  const multiResult = useChartChannels(
    !isStore && isMultiMode ? cfg.channels! : [],
    { maxPoints: effectiveMaxPoints },
  );
  const storeResult = useStoreChartData(isStore ? storeSource : undefined, isStore);

  // 모드별 채널 정규화
  const channelStates: NormalizedChannel[] = useMemo(
    () =>
      isMultiMode
        ? cfg.channels!.map((ref) => ({
            ref,
            state: multiResult.channels.get(ref.name) ?? {
              entries: [],
              status: 'connecting' as ChartConnectionStatus,
            },
          }))
        : [
            {
              ref: {
                name: cfg.channel_name || '',
                display_field: cfg.display_field,
              },
              state: {
                entries: singleResult.entries,
                status: singleResult.status,
                closedReason: singleResult.closedReason,
                errorReason: singleResult.errorReason,
              },
            },
          ],
    [
      isMultiMode,
      cfg.channels,
      cfg.channel_name,
      cfg.display_field,
      multiResult.channels,
      singleResult.entries,
      singleResult.status,
      singleResult.closedReason,
      singleResult.errorReason,
    ],
  );

  // 통합 상태 (가장 심각한 status 우세). store 모드는 storeResult 상태를 사용한다.
  const status = isStore
    ? storeResult.status
    : aggregateStatus(channelStates.map((c) => c.state.status));
  const closedReason = isStore
    ? storeResult.closedReason
    : channelStates.find((c) => c.state.closedReason)?.state.closedReason;
  const errorReason = isStore
    ? storeResult.errorReason
    : channelStates.find((c) => c.state.errorReason)?.state.errorReason;

  // 시간 윈도우 설정
  const timeWindowMode: TimeWindowMode = cfg.time_window_mode ?? 'points';
  const recentWindowSec = cfg.recent_window_sec ?? DEFAULT_RECENT_WINDOW_SEC;
  const refreshMs = clamp(
    cfg.time_window_refresh_ms ?? DEFAULT_REFRESH_MS,
    MIN_REFRESH_MS,
    MAX_REFRESH_MS,
  );

  // 'recent' 모드에서 현재 시각을 주기적으로 갱신
  const [now, setNow] = useState<number>(() => Date.now());

  // 일시정지: 클릭 시점의 chartData/seriesKeys/now 를 스냅샷으로 보관
  // (단일/다중 모드 공통: 최종 렌더 데이터 동결 방식)
  const [pauseSnapshot, setPauseSnapshot] = useState<
    {
      chartData: Array<Record<string, unknown>>;
      seriesKeys: string[];
      now: number;
    } | null
  >(null);
  const isPaused = pauseSnapshot !== null;

  // recent 모드 또는 store 모드(설정 윈도우로 X축 고정)에서 현재 시각을 주기 갱신해
  // X축 도메인 끝(now)이 계속 전진하도록 한다.
  useEffect(() => {
    if ((timeWindowMode !== 'recent' && !isStore) || isPaused) return;
    const id = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(id);
  }, [timeWindowMode, isStore, refreshMs, isPaused]);

  // raw 데이터 계산 (모드별 분기)
  const {
    chartData: rawChartData,
    seriesKeys: rawSeriesKeys,
    booleanKeys,
  } = useMemo(() => {
    const seriesField = cfg.multi_series_field;
    const filterArgs = [
      timeWindowMode,
      now,
      recentWindowSec,
      cfg.fixed_start_ms,
      cfg.fixed_end_ms,
    ] as const;

    // 라인 차트 데이터 소스 값 규칙(SPEC): number(int/float 혼합)은 그대로, boolean 은
    // 1/0 으로, string 등 그 외 타입은 제외(NaN). 시리즈별로 boolean/number 원시 타입을
    // 추적해, 순수 boolean 시리즈는 Y축/툴팁을 true/false 로 표시한다.
    const boolSeen = new Set<string>();
    const numSeen = new Set<string>();
    const coerce = (raw: unknown, key: string): number => {
      if (typeof raw === 'boolean') boolSeen.add(key);
      else if (typeof raw === 'number' && Number.isFinite(raw)) numSeen.add(key);
      return toLineValue(raw);
    };
    // boolean 원시값만 있고 숫자 원시값은 없는 시리즈 = boolean 시리즈.
    const computeBoolKeys = (): Set<string> =>
      new Set([...boolSeen].filter((k) => !numSeen.has(k)));

    // SPEC-WEB-005: Store 모드 — 시리즈별 타임라인을 timestamp 기준으로 병합한다.
    // 각 시리즈 이름이 하나의 라인(컬럼)이 되며, 시간 윈도우 필터는 store_source 의
    // time_window_ms 로 백엔드 조회 시 이미 적용되므로 클라이언트 재필터는 생략한다.
    if (isStore) {
      const rows = new Map<number, Record<string, unknown>>();
      const seen: string[] = [];
      for (const [name, seriesArr] of storeResult.seriesEntries) {
        if (!seen.includes(name)) seen.push(name);
        for (const e of seriesArr) {
          if (!rows.has(e.timestamp)) {
            rows.set(e.timestamp, { timestamp: e.timestamp });
          }
          // store 엔트리 value 는 number|null. null 은 connectNulls 로 이어진다.
          rows.get(e.timestamp)![name] =
            typeof e.value === 'number' ? e.value : null;
        }
      }
      const data = Array.from(rows.values()).sort(
        (a, b) => (a.timestamp as number) - (b.timestamp as number),
      );
      // store 값은 number|null 로 집계됨(boolean 은 storeChartValue 로 1/0 변환).
      // data_type='boolean' 시리즈는 storeResult.booleanSeries 로 true/false 표시한다.
      const boolKeys = new Set(
        seen.filter((name) => storeResult.booleanSeries.has(name)),
      );
      return { chartData: data, seriesKeys: seen, booleanKeys: boolKeys };
    }

    if (isMultiMode) {
      // 다채널: 채널마다 alias 기반 시리즈 키 (multi_series_field 시 alias::label)
      const rows = new Map<number, Record<string, unknown>>();
      const seen = new Set<string>();
      for (const { ref, state } of channelStates) {
        const filtered = filterByTimeWindow(state.entries, ...filterArgs);
        const baseKey = ref.alias ?? ref.name;
        const channelField = ref.display_field ?? cfg.display_field ?? 'value';
        for (const e of filtered) {
          let key: string;
          if (seriesField) {
            const sRaw = getByPath(e, seriesField);
            const s = sRaw == null ? 'default' : String(sRaw);
            key = `${baseKey}::${s}`;
          } else {
            key = baseKey;
          }
          seen.add(key);
          const v = coerce(getByPath(e, channelField), key);
          if (!rows.has(e.timestamp)) {
            rows.set(e.timestamp, { timestamp: e.timestamp });
          }
          rows.get(e.timestamp)![key] = v;
        }
      }
      const data = Array.from(rows.values()).sort(
        (a, b) => (a.timestamp as number) - (b.timestamp as number),
      );
      return {
        chartData: data,
        seriesKeys: Array.from(seen),
        booleanKeys: computeBoolKeys(),
      };
    }

    // 단일 채널 (기존 동작 유지)
    const filtered = filterByTimeWindow(
      channelStates[0]!.state.entries,
      ...filterArgs,
    );
    const displayField = cfg.display_field ?? 'value';
    if (!seriesField) {
      const data = filtered.map((e) => ({
        timestamp: e.timestamp,
        value: coerce(getByPath(e, displayField), 'value'),
      }));
      return {
        chartData: data,
        seriesKeys: ['value'],
        booleanKeys: computeBoolKeys(),
      };
    }
    const rows = new Map<number, Record<string, unknown>>();
    const seen = new Set<string>();
    for (const e of filtered) {
      const sRaw = getByPath(e, seriesField);
      const s = sRaw == null ? 'default' : String(sRaw);
      seen.add(s);
      const v = coerce(getByPath(e, displayField), s);
      if (!rows.has(e.timestamp)) {
        rows.set(e.timestamp, { timestamp: e.timestamp });
      }
      rows.get(e.timestamp)![s] = v;
    }
    const data = Array.from(rows.values()).sort(
      (a, b) => (a.timestamp as number) - (b.timestamp as number),
    );
    return {
      chartData: data,
      seriesKeys: Array.from(seen),
      booleanKeys: computeBoolKeys(),
    };
  }, [
    isStore,
    storeResult.seriesEntries,
    storeResult.booleanSeries,
    isMultiMode,
    channelStates,
    cfg.display_field,
    cfg.multi_series_field,
    timeWindowMode,
    now,
    recentWindowSec,
    cfg.fixed_start_ms,
    cfg.fixed_end_ms,
  ]);

  // 일시정지 시 스냅샷 사용
  const chartData = pauseSnapshot ? pauseSnapshot.chartData : rawChartData;
  const seriesKeys = pauseSnapshot ? pauseSnapshot.seriesKeys : rawSeriesKeys;
  const effectiveNow = pauseSnapshot ? pauseSnapshot.now : now;

  // 렌더 중인 시리즈가 모두 boolean 이면 Y축/툴팁을 true/false 로 표시한다.
  // (혼합 시엔 숫자 축을 유지하되 boolean 시리즈 값만 툴팁에서 true/false 로 표기)
  const allBoolean =
    seriesKeys.length > 0 && seriesKeys.every((k) => booleanKeys.has(k));

  const togglePause = useCallback(() => {
    setPauseSnapshot((prev) =>
      prev
        ? null
        : {
            chartData: rawChartData as Array<Record<string, unknown>>,
            seriesKeys: rawSeriesKeys,
            now: Date.now(),
          },
    );
  }, [rawChartData, rawSeriesKeys]);

  const handleExportCsv = useCallback(
    (rows: Array<Record<string, unknown>>, keys: string[]) => {
      const csv = chartDataToCsv(
        rows.map((r) => r as { timestamp: number; [k: string]: unknown }),
        keys,
        // boolean 시리즈는 CSV 에도 true/false 로 내보낸다.
        booleanKeys,
      );
      const baseName =
        isMultiMode && cfg.channels && cfg.channels.length > 0
          ? cfg.channels.map((c) => c.alias ?? c.name).join('_')
          : cfg.channel_name || 'chart';
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      downloadCsv(csv, `${baseName}-${ts}.csv`);
    },
    [cfg.channel_name, cfg.channels, isMultiMode, booleanKeys],
  );

  // X축 도메인
  // recent 모드: 항상 설정된 윈도우 크기(now - windowSec ~ now) 로 고정.
  // 데이터가 윈도우보다 짧으면 좌측에 빈 공간이 생기지만, X축 크기 자체는
  // 사용자가 설정한 시간 범위를 일관되게 유지한다 (스케일 안정성 우선).
  const xDomain = useMemo<[number | 'dataMin', number | 'dataMax']>(() => {
    if (timeWindowMode === 'recent') {
      return [effectiveNow - recentWindowSec * 1000, effectiveNow];
    }
    if (timeWindowMode === 'fixed') {
      const end = cfg.fixed_end_ms ?? Date.now();
      const start = cfg.fixed_start_ms ?? end;
      return [start, end];
    }
    // store 모드(기본): X축을 설정된 시간 윈도우(store_source.time_window_ms)로 고정한다.
    // 데이터가 윈도우보다 짧아도 X축 범위는 설정값을 일관되게 유지한다(스케일 안정성 우선).
    if (isStore) {
      const windowMs = storeSource?.time_window_ms ?? 0;
      if (windowMs > 0) return [effectiveNow - windowMs, effectiveNow];
    }
    return ['dataMin', 'dataMax'];
  }, [
    timeWindowMode,
    effectiveNow,
    recentWindowSec,
    cfg.fixed_start_ms,
    cfg.fixed_end_ms,
    isStore,
    storeSource?.time_window_ms,
  ]);

  // X축 도메인이 고정 수치 범위인지(recent/fixed/store 윈도우) — 데이터 오버플로 클립 여부 결정.
  const xDomainFixed =
    typeof xDomain[0] === 'number' && typeof xDomain[1] === 'number';

  // X축 nice ticks
  const xTicks = useMemo(() => {
    if (typeof xDomain[0] === 'number' && typeof xDomain[1] === 'number') {
      return computeNiceTimeTicks(xDomain[0], xDomain[1]);
    }
    if (chartData.length >= 2) {
      const first = chartData[0] as Record<string, unknown>;
      const last = chartData[chartData.length - 1] as Record<string, unknown>;
      const s = first.timestamp as number | undefined;
      const e = last.timestamp as number | undefined;
      if (typeof s === 'number' && typeof e === 'number' && e > s) {
        return computeNiceTimeTicks(s, e);
      }
    }
    return undefined;
  }, [xDomain, chartData]);

  // Y축 도메인
  const yAxisMode: YAxisMode = cfg.y_axis_mode ?? 'auto';
  const yPadPct = clamp(cfg.y_axis_padding_pct ?? DEFAULT_Y_PAD_PCT, 0, MAX_Y_PAD_PCT);

  // 열거형 Y축(패널 단위): y_axis_type==='enum' + 유효 매핑이 있으면 값→라벨로 표시한다.
  // enum 이 우선하며, 미설정 boolean 시리즈는 아래 boolAxis 자동 처리로 폴백한다.
  const enumMap = useMemo(
    () => buildEnumLabelMap(cfg.y_enum_labels),
    [cfg.y_enum_labels],
  );
  const enumMode = cfg.y_axis_type === 'enum' && enumMap.size > 0;
  // 눈금은 매핑된 값들을 오름차순으로 사용한다.
  const enumTicks = useMemo(
    () => (enumMode ? [...enumMap.keys()].sort((a, b) => a - b) : undefined),
    [enumMode, enumMap],
  );

  // boolean 시리즈: Y축을 [0,1] 두 눈금(false/true)으로 고정한다. boolean 값은 0/1 이
  // 유일한 의미이므로 수동 Y축 범위(manual)도 적용하지 않는다.
  // 단, 명시적 열거형(enumMode)이 설정되면 그쪽이 우선하므로 boolAxis 는 끈다.
  const boolAxis = allBoolean && !enumMode;
  const yDomain = useMemo<[number | 'auto', number | 'auto']>(() => {
    // 열거형 축: 매핑된 최소/최대 값 ±0.5 여백으로 고정(모든 눈금이 보이도록).
    if (enumMode && enumTicks && enumTicks.length > 0) {
      const lo = enumTicks[0]!;
      const hi = enumTicks[enumTicks.length - 1]!;
      return [lo - 0.5, hi + 0.5];
    }
    if (boolAxis) {
      return [-0.1, 1.1];
    }
    if (yAxisMode === 'manual') {
      return [cfg.y_min ?? 'auto', cfg.y_max ?? 'auto'];
    }
    if (yAxisMode === 'auto_padded') {
      let minV = Number.POSITIVE_INFINITY;
      let maxV = Number.NEGATIVE_INFINITY;
      for (const row of chartData) {
        for (const k of seriesKeys) {
          const v = row[k as keyof typeof row];
          if (typeof v === 'number' && Number.isFinite(v)) {
            if (v < minV) minV = v;
            if (v > maxV) maxV = v;
          }
        }
      }
      if (!Number.isFinite(minV) || !Number.isFinite(maxV)) {
        return ['auto', 'auto'];
      }
      const range = maxV - minV || Math.abs(maxV) || 1;
      const pad = (range * yPadPct) / 100;
      return [minV - pad, maxV + pad];
    }
    // 'auto'
    return ['auto', 'auto'];
  }, [enumMode, enumTicks, boolAxis, yAxisMode, cfg.y_min, cfg.y_max, yPadPct, chartData, seriesKeys]);

  // 글로벌 smooth fallback (하위 호환)
  const globalSmooth = cfg.smooth ?? false;
  const legendCfg: LegendConfig = (cfg.legend as LegendConfig | undefined) ?? {};

  // 축 폰트(레이블/눈금) — 미지정 필드는 기본값(size 10, #9ca3af, normal)으로 폴백.
  const xTickFont = resolveAxisFont(cfg.x_tick_font);
  const xLabelFont = resolveAxisFont(cfg.x_label_font);
  const yTickFont = resolveAxisFont(cfg.y_tick_font);
  const yLabelFont = resolveAxisFont(cfg.y_label_font);

  const criticalBreached = useMemo(
    () =>
      isCriticalBreached(
        chartData as Array<Record<string, unknown>>,
        seriesKeys,
        cfg.y_thresholds,
      ),
    [chartData, seriesKeys, cfg.y_thresholds],
  );

  const containerClass = criticalBreached
    ? 'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-2 ring-red-500 animate-pulse'
    : 'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)';

  return (
    <div className={containerClass}>
      {/* 상단 우측: CSV 다운로드 + 일시정지 토글 + 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3 z-10 flex items-center gap-1">
        <button
          type="button"
          onClick={() => handleExportCsv(chartData as Array<Record<string, unknown>>, seriesKeys)}
          data-testid="line-chart-csv-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label={t('dashboard.chart.exportCsv')}
          title={t('dashboard.chart.exportCsv')}
        >
          <Download className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={togglePause}
          data-testid="line-chart-pause-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label={isPaused ? t('dashboard.chart.resume') : t('dashboard.chart.pause')}
          title={isPaused ? t('dashboard.chart.resume') : t('dashboard.chart.pause')}
        >
          {isPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
        </button>
        <ConnectionStatusIcon status={status} />
      </div>

      {showTitle && (
      <div className="mb-2 flex shrink-0 items-center gap-2 pr-24">
        <TrendingUp className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)">
          {title || (isMultiMode
            ? cfg
                .channels!.map((c) => c.alias ?? c.name)
                .filter((n) => !!n)
                .join(', ') || t('dashboard.settings.preview.channelUnset')
            : cfg.channel_name || t('dashboard.settings.preview.channelUnset'))}
        </span>
      </div>
      )}

      {/* 채널 상태는 범례(Legend)에 통합 — 별도 배지 불필요 */}

      {isPaused && (
        <div
          data-testid="line-chart-pause-badge"
          className="absolute left-3 top-3 z-10 rounded bg-amber-500/90 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow"
        >
          PAUSED
        </div>
      )}

      <div
        className={cn(
          'min-h-0 flex-1 flex',
          legendCfg.position === 'left' && 'flex-row-reverse',
          legendCfg.position === 'right' && 'flex-row',
          (!legendCfg.position || legendCfg.position === 'bottom') && 'flex-col',
        )}
        data-testid="line-chart-container"
      >
        <div className="min-h-0 min-w-0 flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={chartData.length > 0 ? chartData : [{ timestamp: Date.now() }]}
            margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="timestamp"
              type="number"
              domain={xDomain}
              allowDataOverflow={xDomainFixed}
              ticks={xTicks}
              tickFormatter={(v: number) => formatTimeShort(v)}
              tick={{ fontSize: xTickFont.fontSize, fill: xTickFont.fill, fontWeight: xTickFont.fontWeight }}
              stroke="#9ca3af"
              // insideBottom 은 y = 축상단 + height - offset (verticalAnchor 'end') 로 계산되므로
              // offset 을 양수로 주어 텍스트를 축 하단(=SVG 경계)에서 위로 띄워 잘림을 방지한다.
              label={cfg.x_label ? { value: cfg.x_label, position: 'insideBottom', offset: 6, style: { textAnchor: 'middle', fontSize: xLabelFont.fontSize, fill: xLabelFont.fill, fontWeight: xLabelFont.fontWeight } } : undefined}
              height={cfg.x_label ? 48 : 30}
            />
            <YAxis
              domain={yDomain}
              tick={{ fontSize: yTickFont.fontSize, fill: yTickFont.fill, fontWeight: yTickFont.fontWeight }}
              stroke="#9ca3af"
              width={cfg.y_label || cfg.y_unit ? 56 : 50}
              label={cfg.y_label || cfg.y_unit ? { value: [cfg.y_label, cfg.y_unit ? `(${cfg.y_unit})` : ''].filter(Boolean).join(' '), angle: -90, position: 'insideLeft', style: { fontSize: yLabelFont.fontSize, fill: yLabelFont.fill, fontWeight: yLabelFont.fontWeight } } : undefined}
              // 열거형 축은 매핑된 값 눈금을 라벨로, boolean 축은 0/1 을 false/true 로 표시한다.
              ticks={enumMode ? enumTicks : boolAxis ? [0, 1] : undefined}
              tickFormatter={
                enumMode
                  ? (v: number) => formatEnumValue(v, enumMap)
                  : boolAxis
                    ? (v: number) => (v === 1 ? 'true' : v === 0 ? 'false' : '')
                    : cfg.y_unit
                      ? (v: number) => `${v}${cfg.y_unit}`
                      : undefined
              }
            />
            <Tooltip
              // 열거형 축(패널 단위)이면 모든 시리즈 값을 라벨로 표시한다.
              // 그 외에는 boolean 시리즈 값만 true/false 로 표시(혼합 차트에서도 시리즈별 적용).
              formatter={
                enumMode
                  ? (value, name) => [
                      typeof value === 'number'
                        ? formatEnumValue(value, enumMap)
                        : value,
                      name,
                    ]
                  : booleanKeys.size > 0
                    ? (value, name) =>
                        booleanKeys.has(String(name))
                          ? [value === 1 ? 'true' : 'false', name]
                          : [value, name]
                    : undefined
              }
              labelFormatter={(v) => {
                const n = typeof v === 'number' ? v : Number(v);
                return Number.isFinite(n) ? formatTimestamp(n) : String(v ?? '');
              }}
              // 다크모드에서 흰색 배경 + 흰색 시간 라벨로 인해 타임이 보이지 않던 문제 해결.
              // CSS 변수로 surface 배경 + primary 텍스트 색상 적용.
              contentStyle={{
                fontSize: '0.75rem',
                backgroundColor: 'var(--color-bg-surface)',
                border: '1px solid var(--color-border-default)',
                borderRadius: '6px',
                color: 'var(--color-text-primary)',
              }}
              labelStyle={{ color: 'var(--color-text-primary)' }}
              itemStyle={{ color: 'var(--color-text-primary)' }}
            />
            {cfg.y_thresholds?.map((t, i) => {
              const color = t.color ?? THRESHOLD_DEFAULT_COLORS[t.severity ?? 'info'];
              return (
                <ReferenceLine
                  key={`th-${i}`}
                  y={t.value}
                  stroke={color}
                  strokeDasharray="4 2"
                  label={
                    t.label
                      ? { value: t.label, position: 'right', fontSize: 10, fill: color }
                      : undefined
                  }
                />
              );
            })}
            {cfg.y_thresholds
              ?.filter((t) => t.fill_direction || t.fill_to != null)
              .map((t, i) => {
                let y1: number;
                let y2: number;
                if (t.fill_direction === 'below') {
                  y1 = -1e9;
                  y2 = t.value;
                } else if (t.fill_direction === 'above') {
                  y1 = t.value;
                  y2 = 1e9;
                } else {
                  y1 = Math.min(t.value, t.fill_to!);
                  y2 = Math.max(t.value, t.fill_to!);
                }
                return (
                  <ReferenceArea
                    key={`fill-${i}`}
                    y1={y1}
                    y2={y2}
                    fill={t.color}
                    fillOpacity={0.1}
                    strokeOpacity={0}
                  />
                );
              })}
            {seriesKeys.map((key, i) => {
              let stroke = SERIES_COLORS[i % SERIES_COLORS.length]!;
              let strokeDasharray: string | undefined;
              let strokeWidth = 2;
              let lineSmooth = globalSmooth;

              if (isStore) {
                // SPEC-WEB-005: store 시리즈는 seriesStyles(시리즈 표시 이름 기준)에서
                // per-line 스타일을 적용한다. 미지정 필드는 기본값을 유지한다.
                const st = storeResult.seriesStyles.get(key);
                if (st) {
                  if (st.color) stroke = st.color;
                  if (st.stroke_width) strokeWidth = st.stroke_width;
                  if (st.smooth != null) lineSmooth = st.smooth;
                  const dash = STROKE_DASHARRAY[st.stroke_style ?? 'solid'];
                  if (dash) strokeDasharray = dash;
                }
              } else if (isMultiMode) {
                const baseKey = key.includes('::') ? key.split('::')[0]! : key;
                const ref = cfg.channels!.find(
                  (c) => (c.alias ?? c.name) === baseKey,
                );
                if (ref) {
                  if (ref.color) stroke = ref.color;
                  if (ref.stroke_width) strokeWidth = ref.stroke_width;
                  if (ref.smooth != null) lineSmooth = ref.smooth;
                  const style = ref.stroke_style ?? 'solid';
                  const dash = STROKE_DASHARRAY[style];
                  if (dash) strokeDasharray = dash;
                }
              }
              return (
                <Line
                  key={key}
                  type={lineSmooth ? 'monotone' : 'linear'}
                  dataKey={key}
                  stroke={stroke}
                  strokeWidth={strokeWidth}
                  strokeDasharray={strokeDasharray}
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              );
            })}
          </LineChart>
        </ResponsiveContainer>
        </div>

        {/* 범례 — recharts 바깥, CSS flex로 배치 */}
        <ChartLegend
          seriesKeys={seriesKeys}
          seriesColors={seriesKeys.map((key, i) => {
            if (isStore) {
              return (
                storeResult.seriesStyles.get(key)?.color ??
                SERIES_COLORS[i % SERIES_COLORS.length]!
              );
            }
            if (isMultiMode) {
              const baseKey = key.includes('::') ? key.split('::')[0]! : key;
              const ref = cfg.channels!.find(
                (c) => (c.alias ?? c.name) === baseKey,
              );
              return ref?.color ?? SERIES_COLORS[i % SERIES_COLORS.length]!;
            }
            return SERIES_COLORS[i % SERIES_COLORS.length]!;
          })}
          channelStates={channelStates}
          isMultiMode={isMultiMode}
          legendCfg={legendCfg}
          chartData={chartData}
          // 범례 마지막값도 축/툴팁과 동일하게 표시한다.
          // enum 축이면 라벨로, boolean 시리즈면 true/false, 그 외는 소수 1자리.
          formatValue={(key: string, v: number) => {
            if (enumMode) return formatEnumValue(v, enumMap);
            if (booleanKeys.has(key)) return v === 1 ? 'true' : v === 0 ? 'false' : String(v);
            return v.toFixed(1);
          }}
        />
      </div>

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="line-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
