// 히트맵 패널 진입점 (SPEC-HEATMAP-PANEL-001 T3/T6).
//
// LineChartPanel 의 isStore 분기를 미러링해 useStoreChartData 로 공간 온도 센서를 태그
// 바인딩하고(REQ-02), 시리즈별 최신 값을 센서 판독값으로 추출해 config.sensor_positions
// 좌표와 결합한다(joinSensorPoints, T6). 좌표가 배치된 센서점을 <HeatmapCanvas> 에 넘겨
// Canvas 2D IDW 온도장을 렌더한다(REQ-03).
//
// 견고성(REQ-04):
//   - 센서 0개 또는 배치 좌표 0개 → 빈 상태 안내(예외 없음, AC-E1).
//   - 좌표 미지정 센서는 보간 입력에서 제외 + 안내(AC-E2).
//   - 폴링 실패 시 useStoreChartData 가 직전 시리즈(entries)를 보존한 채 status='error' 만
//     세팅하므로, 마지막 렌더(온도장)를 파괴하지 않고 오류 배지만 덧띄운다(AC-E3).

import { useMemo } from 'react';
import { Thermometer } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import type { StoreSourceConfig } from '../charts/chartChannelTypes';
import { useStoreChartData } from '../charts/useStoreChartData';
import { parseHeatmapConfig } from './heatmapConfig';
import { joinSensorPoints } from './heatmapJoin';
import { DEFAULT_COLOR_TABLE } from './idw';
import HeatmapCanvas from './HeatmapCanvas';

interface HeatmapPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

export default function HeatmapPanel({ config }: HeatmapPanelProps) {
  const { t } = useTranslation();
  const cfg = parseHeatmapConfig(config);

  // LineChartPanel isStore 분기 미러링: data_source==='store' + (series 또는 tag_filters 존재).
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const storeTagActive =
    storeSource?.selection_mode === 'tag' &&
    Object.keys(storeSource.tag_filters ?? {}).length > 0;
  const isStore =
    config.data_source === 'store' &&
    ((storeSource?.series?.length ?? 0) > 0 || storeTagActive);

  // hook 은 항상 호출(React 규칙). 비활성 경로는 idle 로 유지된다.
  const storeResult = useStoreChartData(isStore ? storeSource : undefined, isStore);

  // 센서 최신값(시리즈별) 추출 + 좌표 결합(T6). 좌표 미지정 센서는 보간 입력에서 제외(AC-E2).
  const { points, unplacedNames, autoBounds } = useMemo(
    () =>
      joinSensorPoints(
        storeResult.seriesNames,
        storeResult.seriesEntries,
        cfg.sensor_positions,
      ),
    [storeResult.seriesNames, storeResult.seriesEntries, cfg.sensor_positions],
  );

  // 상하한: config 지정값 우선, 미지정 시 자동(센서값 범위), 그마저 없으면 0..1(REQ-05).
  const bounds = cfg.value_bounds ?? autoBounds ?? { min: 0, max: 1 };
  const colorTable = cfg.color_table ?? DEFAULT_COLOR_TABLE;

  const isError = storeResult.status === 'error';
  // 빈 상태(REQ-04 / AC-E1): 좌표가 배치된 센서점이 0개면 안내 문구를 표시한다(렌더 예외 없음).
  const isEmpty = points.length === 0;

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-2 shadow">
      {isEmpty ? (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 rounded-md border border-dashed border-(--color-border-default) p-4 text-center">
          <Thermometer className="h-6 w-6 text-(--color-text-muted)" />
          <span className="text-sm text-(--color-text-muted)">
            {t('dashboard.heatmap.emptyState')}
          </span>
          {unplacedNames.length > 0 && (
            <span className="text-xs text-(--color-text-muted)">
              {t('dashboard.heatmap.unplaced').replace('{count}', String(unplacedNames.length))}
            </span>
          )}
        </div>
      ) : (
        <HeatmapCanvas
          points={points}
          bounds={bounds}
          colorTable={colorTable}
          power={cfg.idw.power}
          gridResolution={cfg.idw.grid_resolution}
        />
      )}

      {/* 오류 배지(AC-E3): 폴링 실패 시에도 마지막 렌더를 유지한 채 상태만 덧띄운다. */}
      {isError && (
        <span
          data-testid="heatmap-error"
          className="pointer-events-none absolute right-2 top-2 rounded bg-red-500/90 px-2 py-0.5 text-xs text-white"
        >
          {t('dashboard.heatmap.error')}
        </span>
      )}
    </div>
  );
}
