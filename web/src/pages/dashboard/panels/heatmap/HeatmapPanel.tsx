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

import { useMemo, useState } from 'react';
import { Move, Thermometer } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { StoreSourceConfig } from '../charts/chartChannelTypes';
import { useStoreChartData } from '../charts/useStoreChartData';
import { parseHeatmapConfig } from './heatmapConfig';
import { joinSensorPoints } from './heatmapJoin';
import { DEFAULT_COLOR_TABLE, interpolateIDW } from './idw';
import HeatmapCanvas, { MIN_GRID_RESOLUTION, MAX_GRID_RESOLUTION } from './HeatmapCanvas';
import ContourLayer from './ContourLayer';
import FloorPlanBackground from './FloorPlanBackground';
import SensorPlacementOverlay, { type PlacedSensor } from './SensorPlacementOverlay';
import type { NormalizedPos } from './placement';

/** 값을 [lo, hi] 로 clamp 한다(격자 해상도 성능 가드). */
function clamp(value: number, lo: number, hi: number): number {
  return value < lo ? lo : value > hi ? hi : value;
}

interface HeatmapPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
  /**
   * 패널 config 부분 갱신(불투명 JSON, 다른 패널과 동일 경로). 제공되면 인패널 배치 편집
   * (드래그로 sensor_positions 쓰기)이 활성화된다. 미제공 시 편집 진입 버튼이 숨겨져 MVP 와
   * 동일하게 동작한다(행위 보존, SPEC-002 T7).
   */
  onConfigChange?: (config: Record<string, unknown>) => void;
}

export default function HeatmapPanel({ config, onConfigChange }: HeatmapPanelProps) {
  const { t } = useTranslation();
  const cfg = parseHeatmapConfig(config);

  // 배치 편집 모드(런타임 상태, 비영속 — REQ-03/T7). onConfigChange 가 있을 때만 진입 가능.
  const [editing, setEditing] = useState(false);
  const canEdit = typeof onConfigChange === 'function';

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

  // SPEC-003: 격자 계산을 패널로 상승한다. 히트맵 canvas 와 등고선(ContourLayer)이 **동일 field
  // 참조**를 공유하도록 interpolateIDW 는 여기서 패널당 1회만 호출한다(재보간 금지, R3/AC-E5).
  // clamp 상수는 HeatmapCanvas(성능 가드)에서, 보간은 idw.ts 에서 재사용한다.
  const grid = useMemo(
    () => Math.round(clamp(cfg.idw.grid_resolution, MIN_GRID_RESOLUTION, MAX_GRID_RESOLUTION)),
    [cfg.idw.grid_resolution],
  );
  const field = useMemo(
    () => interpolateIDW(points, grid, grid, cfg.idw.power),
    [points, grid, cfg.idw.power],
  );

  const isError = storeResult.status === 'error';
  // 빈 상태(REQ-04 / AC-E1): 좌표가 배치된 센서점이 0개면 안내 문구를 표시한다(렌더 예외 없음).
  const isEmpty = points.length === 0;

  // SPEC-002: 도면 배경 + 히트맵 합성 불투명도(REQ-01/REQ-05). 도면 미첨부 시 배경은 null 이고,
  // 불투명도는 적용하지 않아 MVP 시각(배경 없는 온도장)과 동일하다(행위 보존). 도면이 있을 때만
  // 히트맵 레이어를 heatmap_opacity 로 합성해 도면이 비쳐 보이게 한다.
  const hasBackground = Boolean(cfg.floor_plan?.image);
  const heatmapLayerStyle = hasBackground ? { opacity: cfg.heatmap_opacity } : undefined;

  // 편집 대상: 좌표가 있는 센서(마커) + 라이브 시리즈지만 좌표 없는 센서(미배치 팔레트).
  // 마커는 라이브 판독값과 무관하게 좌표를 가진 모든 센서를 표시한다(joinSensorPoints 의 points 는
  // 판독값이 있어야 하므로 여기선 config 좌표를 직접 사용).
  const placed: PlacedSensor[] = useMemo(
    () => Object.entries(cfg.sensor_positions).map(([key, pos]) => ({ key, pos })),
    [cfg.sensor_positions],
  );

  // 좌표 갱신(드래그 미리보기 + 드롭). 부분 config 병합으로 sensor_positions 만 쓴다(additive).
  const handlePositionChange = (key: string, pos: NormalizedPos) => {
    onConfigChange?.({ sensor_positions: { ...cfg.sensor_positions, [key]: pos } });
  };
  // 좌표 항목만 삭제(센서 자체는 store 바인딩에서 유지, AC-04).
  const handleRemove = (key: string) => {
    const next = { ...cfg.sensor_positions };
    delete next[key];
    onConfigChange?.({ sensor_positions: next });
  };

  // 편집 중에는 배치 표면(배경/좌표 공간)을 항상 렌더한다(도면/데이터 없어도 배치 가능, AC-E1 편집 경로).
  const showStack = !isEmpty || editing;

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-2 shadow">
      {/* 배치 편집 진입/종료 토글(REQ-03/T7). onConfigChange 가 있을 때만 표시 → MVP(콜백 없음) 불변. */}
      {canEdit && (
        <button
          type="button"
          data-testid="heatmap-edit-toggle"
          aria-pressed={editing}
          onClick={() => setEditing((v) => !v)}
          className={cn(
            'absolute left-2 top-2 z-30 inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium shadow transition-colors',
            editing
              ? 'bg-blue-600 text-white dark:bg-blue-500'
              : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-surface)',
          )}
        >
          <Move className="h-3 w-3" />
          {editing ? t('dashboard.heatmap.editExit') : t('dashboard.heatmap.editEnter')}
        </button>
      )}

      {!showStack ? (
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
        // 레이어 스택: 도면 배경(z-0) → 히트맵 canvas(z-10, opacity 합성) → 마커 오버레이(z-20, 편집 시).
        // 세 레이어가 동일 컨테이너 rect 위에 겹쳐 정규화 좌표 공간을 공유한다. 편집은 오버레이 레이어에
        // 국한되어 store 폴링/히트맵 렌더를 파괴하지 않는다(REQ-04, R3, AC-03).
        <div className="relative flex min-h-0 w-full flex-1">
          <FloorPlanBackground image={cfg.floor_plan?.image} fit={cfg.floor_plan?.fit} />
          <div className="relative z-10 flex min-h-0 w-full flex-1" style={heatmapLayerStyle}>
            {/* 온도장은 배치 센서점이 있을 때만 렌더(편집 중 빈 좌표 공간 위 배치도 허용, AC-E1). */}
            {!isEmpty && (
              <HeatmapCanvas
                field={field}
                gridW={grid}
                gridH={grid}
                hasData={points.length > 0}
                bounds={bounds}
                colorTable={colorTable}
              />
            )}
          </div>
          {/* SPEC-003: 등고선 오버레이(z-15) — 히트맵(z-10) 위, 센서 마커(z-20) 아래. 히트맵과
              동일 field 참조를 공유한다(재보간 없음). contour off/빈 격자 시 ContourLayer 가 null. */}
          {!isEmpty && cfg.contour?.enabled && (
            <ContourLayer
              field={field}
              gridW={grid}
              gridH={grid}
              bounds={bounds}
              contour={cfg.contour}
            />
          )}
          {editing && (
            <SensorPlacementOverlay
              placed={placed}
              unplaced={unplacedNames}
              onPositionChange={handlePositionChange}
              onRemove={handleRemove}
              snap={cfg.editor?.snap}
              markerSize={cfg.editor?.marker_size}
            />
          )}
        </div>
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
