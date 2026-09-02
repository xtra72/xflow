// 시스템 지표 패널.
//
// 항목의 단위는 **값 하나**다 (`sysMetricsFields.ts`). CPU 사용률·메모리 사용량·
// 네트워크 수신량처럼 보고 싶은 값만 골라 배치하고, 값마다 스타일(타일·게이지·
// 진행막대·라인·영역·막대)과 정렬·글자·색을 따로 정한다.
//
// 누적 카운터(네트워크·디스크 I/O)는 기본적으로 두 표본의 차이를 단위시간당 증가량으로
// 환산해 보여준다. 부팅 이후 누적값을 그대로 두면 언제나 커지기만 하는 수라 추이를 읽을
// 수 없기 때문이다. 총량이 필요한 경우를 위해 항목마다 `counterMode` 로 누적값 표시를
// 고를 수 있다(`sysMetricsItemOptions.ts`).
//
// 종합의 범위를 패널이 다시 고르지 않는다. 무엇을 관측할지는 에이전트 설정이 정하고,
// 이 패널은 그 에이전트가 보는 전부를 더한다.

import { useCallback, useMemo } from 'react';
import { Cpu } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import {
  formatBytes,
  formatPackets,
  interfaceColors,
  normalizeUnitTime,
} from '@/pages/monitoring/networkSeries';

import { useAutoColumns } from '../monitor/useAutoColumns';
import { SysMetricsPanelShell } from './SysMetricsPanelShell';
import { SysMetricsItemView } from './SysMetricsItemView';
import {
  DEFAULT_SYSTEM_FIELDS,
  findField,
  normalizeSystemItems,
  type SysMetricField,
} from './sysMetricsFields';
import { readAccent, readAgentRef, readMaxCols, readRefreshMs } from './sysMetricsPanelConfig';
import {
  isTimeSeriesStyle,
  resolveItemOptions,
  resolveValueColor,
  type SysMetricsCounterMode,
  type SysMetricsItemOptions,
} from './sysMetricsItemOptions';
import {
  deltaRate,
  isCollected,
  sumInstances,
  type SysMetricsSnapshot,
} from './sysMetricsSeries';
import { useSysMetricsRateSeries } from './useSysMetricsNetworkSeries';
import { useSysMetricsSnapshot } from './useSysMetricsSnapshot';

/** 타일 하나가 읽히려면 필요한 최소 폭(px) */
const MIN_CARD_WIDTH = 160;
/** 기본 열 개수 상한 */
const DEFAULT_MAX_COLS = 4;
/** 기본 폴링 주기 */
const DEFAULT_REFRESH_MS = 5_000;

interface SysMetricsSystemPanelProps {
  panelId: string;
  title: string;
  config?: Record<string, unknown>;
}

/**
 * 스냅샷에서 그룹의 값을 꺼낸다.
 *
 * 인스턴스 축이 있는 그룹(디스크·네트워크)은 전체 합이다 — 이 패널은 종합을 본다.
 */
function groupValues(
  snapshot: SysMetricsSnapshot,
  group: SysMetricField['group'],
): Record<string, number> | undefined {
  if (!isCollected(snapshot, group)) return undefined;
  if (group === 'cpu' || group === 'memory') return snapshot[group];
  return sumInstances(snapshot[group]);
}

/**
 * 그룹의 필드별 표시 방식 맵 — 계열 훅에 넘길 형상이다.
 *
 * 누적 카운터가 아닌 항목은 담지 않는다. 훅은 빠진 필드를 `rate` 로 읽으므로 담아 봐야
 * 뜻이 없고, 키가 늘면 훅 안에서 계열을 비우는 판정(shape)이 헛돌아 선이 끊긴다.
 */
function counterModes(
  fields: SysMetricField[],
  options: Record<string, SysMetricsItemOptions>,
  group: SysMetricField['group'],
): Record<string, SysMetricsCounterMode> {
  const out: Record<string, SysMetricsCounterMode> = {};
  for (const field of fields) {
    if (field.group !== group || !field.rate) continue;
    out[field.field] = options[field.key]?.counterMode ?? 'rate';
  }
  return out;
}

export function SysMetricsSystemPanel({ panelId, title, config }: SysMetricsSystemPanelProps) {
  const { t } = useTranslation();

  const { agentId, agentName } = readAgentRef(config);
  // 옛 그룹 키(cpu / memory / …)를 담은 저장된 대시보드도 그대로 열린다.
  const itemKey = (normalizeSystemItems(config?.items) ?? DEFAULT_SYSTEM_FIELDS).join(',');
  const fields = useMemo(
    () =>
      itemKey
        .split(',')
        .map(findField)
        .filter((f): f is SysMetricField => f !== undefined),
    [itemKey],
  );

  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const maxCols = readMaxCols(config, DEFAULT_MAX_COLS, fields.length);
  const unit = normalizeUnitTime(config?.unitTime);
  const { accentColor } = readAccent(config);
  const [gridRef, cols] = useAutoColumns(maxCols, MIN_CARD_WIDTH);

  const { snapshot, previous, state } = useSysMetricsSnapshot(agentId, refreshMs);

  const options = useMemo(
    () =>
      Object.fromEntries(
        fields.map((f) => [f.key, resolveItemOptions(config, f.key, f.kind, 'sysmetrics-system')]),
      ),
    [fields, config],
  );

  // 시계열 스타일을 고른 값이 있으면 계열을 쌓는다. 종합이므로 대상 축은 없다.
  const seriesTargets = useMemo(() => [] as string[], []);
  const diskFields = useMemo(
    () => fields.filter((f) => f.group === 'diskIo').map((f) => f.field),
    [fields],
  );
  const netFields = useMemo(
    () => fields.filter((f) => f.group === 'network').map((f) => f.field),
    [fields],
  );
  // 필드별 표시 방식 — 계열을 쌓는 훅이 항목 설정을 그대로 따라야 타일과 차트가
  // 같은 수를 말한다. 키는 그룹 안의 필드 이름이다(훅의 계열 키와 같은 축).
  const diskModes = useMemo(
    () => counterModes(fields, options, 'diskIo'),
    [fields, options],
  );
  const netModes = useMemo(
    () => counterModes(fields, options, 'network'),
    [fields, options],
  );
  const pickDisk = useCallback((s: SysMetricsSnapshot) => s.diskIo, []);
  const pickNet = useCallback((s: SysMetricsSnapshot) => s.network, []);

  const diskSeries = useSysMetricsRateSeries(
    snapshot, previous, pickDisk, seriesTargets, diskFields, unit, diskModes,
  );
  const netSeries = useSysMetricsRateSeries(
    snapshot, previous, pickNet, seriesTargets, netFields, unit, netModes,
  );

  const labelColor = accentColor('label');
  const seriesColor = interfaceColors(['total'])['total']!;

  /**
   * 값 하나를 표기 문자열로 만든다.
   *
   * 단위시간(`/s`)은 **증가량일 때만** 붙는다. 누적값에 `/s` 가 붙으면 총량을 속도로
   * 읽게 되어 자릿수를 그대로 오해한다.
   */
  const formatValue = (field: SysMetricField, value: number, asRate: boolean): string => {
    if (field.format === 'percent') return `${value.toFixed(1)}%`;
    if (field.format === 'bytes') return formatBytes(value, asRate ? unit : undefined);
    return formatPackets(value, asRate ? unit : undefined);
  };

  /** 그 항목을 증가량으로 그리는가 — 누적 카운터이면서 `rate` 모드일 때만 참이다. */
  const isRateValue = (field: SysMetricField): boolean =>
    field.rate && options[field.key]!.counterMode === 'rate';

  /**
   * 값 하나의 지금 수치. 그릴 수 없으면 undefined.
   *
   * 증가량은 두 표본의 차이가 필요하다 — 첫 표본이나 되감김 구간에서는 undefined 이고
   * 화면에 `-` 가 뜬다(0 으로 채우면 흐름이 끊긴 것처럼 보인다). 누적 모드는 원값이
   * 곧 그릴 값이라 기준점 없이 첫 표본부터 나온다.
   */
  const currentValue = (field: SysMetricField): number | undefined => {
    if (!snapshot) return undefined;
    const next = groupValues(snapshot, field.group)?.[field.field];
    if (next === undefined) return undefined;

    if (!isRateValue(field)) return next;

    if (!previous || previous.collectedAt === null || snapshot.collectedAt === null) {
      return undefined;
    }
    const before = groupValues(previous, field.group)?.[field.field];
    if (before === undefined) return undefined;

    return (
      deltaRate(
        { value: before, at: previous.collectedAt },
        { value: next, at: snapshot.collectedAt },
        unit,
      ) ?? undefined
    );
  };

  /** 값 하나를 그린다. */
  const renderField = (field: SysMetricField) => {
    if (!snapshot) return null;
    const opts = options[field.key]!;
    const value = currentValue(field);

    const seriesProps: Record<string, unknown> = {};
    if (isTimeSeriesStyle(opts.style) && field.rate) {
      const bucket = field.group === 'diskIo' ? diskSeries : netSeries;
      const now = snapshot.collectedAt ?? Date.now();
      seriesProps.series = [
        { name: t(field.labelKey), color: seriesColor, data: bucket.total?.[field.field] ?? [] },
      ];
      seriesProps.timeDomain = [now - opts.windowSec * 1_000, now];
      seriesProps.format = (v: number) => formatValue(field, v, isRateValue(field));
    }

    return (
      <SysMetricsItemView
        key={field.key}
        testId={`sysmetrics-field-${field.key}`}
        options={opts}
        label={t(field.labelKey)}
        value={value === undefined ? undefined : formatValue(field, value, isRateValue(field))}
        percent={field.format === 'percent' ? value : undefined}
        disabled={!isCollected(snapshot, field.group)}
        labelColor={labelColor}
        valueColor={resolveValueColor(value, opts) ?? accentColor('value')}
        {...seriesProps}
      />
    );
  };

  return (
    <SysMetricsPanelShell
      panelId={panelId}
      title={title}
      icon={Cpu}
      headerColor={accentColor('header')}
      state={state}
      agentName={agentName}
    >
      {fields.length === 0 ? (
        <p
          data-testid="sysmetrics-system-empty"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('monitoring.emptyPanel')}
        </p>
      ) : (
        <div
          ref={gridRef}
          data-testid="sysmetrics-system-grid"
          data-cols={cols}
          data-fields={fields.map((f) => f.key).join(',')}
          className="grid min-h-0 flex-1 gap-2 overflow-y-auto"
          style={{
            gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
            // 행이 남은 높이를 나눠 갖는다 (패널을 채운다).
            gridAutoRows: 'minmax(120px, 1fr)',
          }}
        >
          {fields.map(renderField)}
        </div>
      )}
    </SysMetricsPanelShell>
  );
}

export default SysMetricsSystemPanel;
