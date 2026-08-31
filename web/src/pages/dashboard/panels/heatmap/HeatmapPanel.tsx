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

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Move, Thermometer } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';

import { normalizeStoreSeriesAlias } from '../charts/chartChannelTypes';
import type { StoreSeriesRef, StoreSourceConfig } from '../charts/chartChannelTypes';
import { usePanelSeriesData } from '../charts/usePanelSeriesData';
import { parseHeatmapConfig } from './heatmapConfig';
// 히트맵에는 값 읽기(툴팁·타일)가 없고 범례 눈금과 등고선 라벨만 있다. 둘 다 눈금자이므로
// 자릿수 기본값(2)은 걸지 않고, 사용자가 직접 지정했을 때만 따른다.
import {
  hasExplicitDecimalPlaces,
  readDecimalPlaces,
} from '@/pages/dashboard/panels/charts/decimalPlaces';
import { joinSensorPoints, resolveSensorSeries } from './heatmapJoin';
import { heatmapSensorId, sensorSeriesLabel } from './sensorIdentity';
import { DEFAULT_COLOR_TABLE, interpolateIDW } from './idw';
import HeatmapCanvas, { MIN_GRID_RESOLUTION, MAX_GRID_RESOLUTION } from './HeatmapCanvas';
import ContourLayer from './ContourLayer';
import HeatmapLegend from './HeatmapLegend';
import FloorPlanBackground from './FloorPlanBackground';
import FloorPlanTransformOverlay from './FloorPlanTransformOverlay';
import SensorPlacementOverlay, { type PlacedSensor } from './SensorPlacementOverlay';
import type { NormalizedPos } from './placement';
import {
  applyStageTransform,
  computeStageBox,
  migratePositionsToStage,
  stageToContainer,
} from './stage';
import { useFloorPlanAspect } from './useFloorPlanAspect';
import { useFloorPlanSources } from './useFloorPlanSources';
import { usePanelTitleVisible } from '../../panelChromeContext';

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
  /**
   * 배치 편집(드래그) 강제 활성화. 설정 다이얼로그 미리보기 전용 — 대시보드 편집모드
   * (dashboardEditMode)에 의존하지 않고 배치 오버레이를 항상 켠다. 기본 false(대시보드
   * 경로 불변): 대시보드는 기존 dashboardEditMode + 편집 토글 게이팅을 그대로 사용한다.
   * @spec SPEC-PANEL-SETTINGS-001 (heatmap 시리즈 위치)
   */
  forcePlacement?: boolean;
}

export default function HeatmapPanel({
  title,
  config,
  onConfigChange,
  forcePlacement = false,
}: HeatmapPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const cfg = parseHeatmapConfig(config);
  const explicitDecimals = hasExplicitDecimalPlaces(config)
    ? readDecimalPlaces(config)
    : undefined;

  // 배치 편집 모드(런타임 상태, 비영속 — REQ-03/T7). onConfigChange 가 있을 때만 진입 가능.
  const [editing, setEditing] = useState(false);
  const canEdit = typeof onConfigChange === 'function';
  // 대시보드 편집모드(gear/삭제 버튼과 동일 게이팅). 편집모드일 때만 배치편집 진입 버튼을 노출한다.
  // 원격 읽기전용 뷰(RemoteDashboardView)는 dashboardEditMode=false 라 자동 숨김(회귀 0).
  const editMode = useUIStore((s) => s.dashboardEditMode);

  // 편집모드를 벗어나면 진행 중이던 배치편집 상태를 강제 해제한다(오버레이 잔존 방지).
  useEffect(() => {
    if (!editMode) setEditing(false);
  }, [editMode]);

  const storeSource = config.store_source as StoreSourceConfig | undefined;
  // 히트맵은 "명시적으로 체크된 series(keys)"만 렌더한다(사용자 요구: 체크박스 = 단일 진실원).
  // tag_filters 는 렌더 바인딩에 사용하지 않는다 — 태그 헤더 필터는 데이터소스 리스트를 좁히는
  // 용도일 뿐이며, 태그 매칭(미체크) 시리즈가 필드/마커로 그려지면 안 된다. 따라서 store 데이터
  // 바인딩을 keys 로 강제하고 tag_filters 를 제거한 파생 소스로 조회한다(line/gauge 등 다른
  // 패널의 tag 동작은 useStoreChartData 를 그대로 두어 영향받지 않는다).
  // 체크된 시리즈(단일 진실원). 손상 config 방어로 배열이 아니면 빈 배열.
  // legacy 기본 alias(=key)를 "이름 없음" 으로 되돌린 뒤 사용한다. 그대로 두면 사용자가
  // 붙인 이름과 구분되지 않아, 형제 센서가 모두 같은 이름(=key)으로 보인다.
  const refs = useMemo<StoreSeriesRef[]>(
    () =>
      normalizeStoreSeriesAlias(Array.isArray(storeSource?.series) ? storeSource.series : []),
    [storeSource],
  );
  // 조회용 파생 소스에서 alias 를 **센서 동일성 키**로 치환한다. 한 store key 를 공유하는
  // 형제 시리즈들이 기본 alias(=key)를 공유하면 useStoreChartData 가 같은 표시 이름으로
  // 타임라인을 병합해 센서별 판독값이 뭉개진다. 동일성 키는 시리즈마다 고유하므로 병합이
  // 없고, 좌표 맵과 같은 키 공간이 되어 매칭이 이름(편집 가능)에 의존하지 않는다.
  const heatmapStoreSource = useMemo<StoreSourceConfig | undefined>(
    () =>
      storeSource
        ? {
            ...storeSource,
            selection_mode: 'keys',
            tag_filters: undefined,
            series: refs.map((ref) => ({ ...ref, alias: heatmapSensorId(ref) })),
          }
        : undefined,
    [storeSource, refs],
  );
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정과 조회를 `panelDataSource` / `usePanelSeriesData`
  // 계약에 위임한다. 히트맵은 `config.store_source` 가 아니라 위에서 파생한
  // `heatmapStoreSource` 로 조회하므로 그 파생 소스를 `storeSourceOverride` 로 **주입**한다.
  // 계약이 `config.store_source` 를 직접 읽게 두면 tag 모드 히트맵이 새로 활성화되어
  // 동작이 바뀐다(파생 소스는 `selection_mode:'keys'` 로 강제되어 tag 항이 늘 거짓이므로,
  // 주입한 쪽의 활성 판정은 종전 `heatmapStoreSource.series.length > 0` 과 정확히 같다).
  //
  // hook 은 항상 호출(React 규칙). 비활성 경로는 idle 로 유지된다.
  const storeResult = usePanelSeriesData(config, {
    storeSourceOverride: heatmapStoreSource,
  });

  // 조회 결과(표시 이름 공간) → 센서 동일성 키 공간. 좌표/마커와 같은 공간에서만 결합한다.
  const resolved = useMemo(
    () => resolveSensorSeries(storeResult.seriesNames, storeResult.seriesEntries, refs),
    [storeResult.seriesNames, storeResult.seriesEntries, refs],
  );

  // --- 스테이지(기준 도면 종횡비 박스) ---
  // 패널 본문을 실측하고, 기준 도면의 종횡비로 레터박스 박스를 계산한다. 히트맵/등고선/마커/
  // 범례/도면이 모두 이 박스 안에 그려지므로 정규화 좌표가 도면에 고정된다 → 설정 미리보기와
  // 대시보드가 컨테이너 비율과 무관하게 같은 그림을 낸다(stage.ts 참조).
  const [body, setBody] = useState<HTMLDivElement | null>(null);
  const [bodySize, setBodySize] = useState({ width: 0, height: 0 });
  useEffect(() => {
    if (!body) return;
    const measure = () => {
      const r = body.getBoundingClientRect();
      setBodySize({ width: r.width, height: r.height });
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(measure);
    ro.observe(body);
    return () => ro.disconnect();
  }, [body]);
  // 레이어 이미지 해석(자산 id → data-URL, 레거시 인라인 이미지는 그대로).
  const floorPlanSources = useFloorPlanSources(cfg.floor_plans);
  const baseLayer = cfg.floor_plans[0];
  const baseAspect = useFloorPlanAspect(baseLayer, floorPlanSources[0]);
  // stage_fit: 여백(contain, 기본) / 잘림(cover) / 왜곡(stretch) 중 무엇을 감수할지의 선택.
  // fit 으로 기본 박스를 구한 뒤 사용자가 직접 옮기고 키운 변형을 얹는다. 마커는 스테이지
  // 정규화 좌표라 변형을 따로 반영할 필요 없이 도면 위 같은 지점에 그대로 붙어 따라온다.
  const stage = useMemo(
    () =>
      applyStageTransform(
        computeStageBox(bodySize.width, bodySize.height, baseAspect, cfg.stage_fit),
        cfg.stage_transform,
        bodySize,
      ),
    [bodySize, baseAspect, cfg.stage_fit, cfg.stage_transform],
  );

  // 레거시 좌표(컨테이너 기준) → 스테이지 기준 환산. 실측 rect 로 환산하므로 **보이던 위치가
  // 그대로 보존**된다(스테이지 도입으로 마커가 소리 없이 움직이지 않는다). 이미 'stage' 로
  // 승격된 config 는 그대로 쓴다. 측정 전(0×0)에는 환산할 수 없으므로 원본을 그대로 둔다.
  const needsSpaceMigration =
    cfg.sensor_space !== 'stage' && stage.width > 0 && stage.height > 0 &&
    (stage.width !== bodySize.width || stage.height !== bodySize.height);
  const sensorPositions = useMemo(
    () =>
      needsSpaceMigration
        ? migratePositionsToStage(cfg.sensor_positions, bodySize, stage)
        : cfg.sensor_positions,
    [needsSpaceMigration, cfg.sensor_positions, bodySize, stage],
  );

  // 센서 최신값(시리즈별) 추출 + 좌표 결합(T6). 좌표 미지정 센서는 보간 입력에서 제외(AC-E2).
  const { points, unplacedNames, autoBounds } = useMemo(
    () => joinSensorPoints(resolved.ids, resolved.entriesById, sensorPositions),
    [resolved, sensorPositions],
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
  const hasBackground = cfg.floor_plans.length > 0;
  const heatmapLayerStyle = hasBackground ? { opacity: cfg.heatmap_opacity } : undefined;

  // 마커/좌표는 "명시적으로 체크된 series"에만 표시한다(체크박스 = 단일 진실원). 라이브 값이
  // 없어도 방금 선택한 센서 마커를 유지해 드래그 배치가 가능하다. tag 매칭(미체크) 시리즈는
  // 절대 마커로 그리지 않으며, 잔존 sensor_positions(선택에서 빠진 옛 키)로 인한 유령 마커도 막는다.
  // 기준은 동일성 키다 — alias 기준이면 이름을 바꾼 순간 마커가 사라진다.
  const boundIds = useMemo(() => new Set(refs.map((ref) => heatmapSensorId(ref))), [refs]);
  // 동일성 키 → 표시 라벨. 사용자가 붙인 이름이 있으면 그 이름, 없으면 시리즈를 구분하는
  // 서술 표기(`key · metric{k=v}`)다 — key 만 쓰면 한 key 를 공유하는 형제 센서들의 마커가
  // 같은 글자로 찍혀 서로 구분되지 않는다. 마커/칩 텍스트 전용이며 매칭에는 쓰지 않는다.
  const sensorLabels = useMemo(() => {
    const map: Record<string, string> = {};
    for (const ref of refs)
      map[heatmapSensorId(ref)] = sensorSeriesLabel(ref, storeSource?.series_name_format);
    return map;
  }, [refs, storeSource?.series_name_format]);
  // 편집 대상: 현재 바인딩된 시리즈 중 좌표가 있는 센서(마커). 라이브 판독값과 무관하게
  // config 좌표를 직접 쓰되(joinSensorPoints 의 points 는 판독값 필요), 바운드 집합으로 거른다.
  const placed: PlacedSensor[] = useMemo(
    () =>
      Object.entries(sensorPositions)
        .filter(([id]) => boundIds.has(id))
        .map(([id, pos]) => ({ key: id, pos })),
    [sensorPositions, boundIds],
  );

  // 좌표 쓰기는 항상 **환산된 맵 전체**를 내보내고 공간을 'stage' 로 승격한다. 레거시 맵에
  // 스테이지 좌표 한 건만 섞어 쓰면 두 공간이 한 맵에 공존해 나머지 마커가 틀어진다.
  const writePositions = useCallback(
    (next: Record<string, NormalizedPos>) => {
      onConfigChange?.({ sensor_positions: next, sensor_space: 'stage' });
    },
    [onConfigChange],
  );
  // 좌표 갱신(드래그 미리보기 + 드롭). 부분 config 병합으로 sensor_positions 만 쓴다(additive).
  const handlePositionChange = (key: string, pos: NormalizedPos) => {
    writePositions({ ...sensorPositions, [key]: pos });
  };
  // 좌표 항목만 삭제(센서 자체는 store 바인딩에서 유지, AC-04).
  const handleRemove = (key: string) => {
    const next = { ...sensorPositions };
    delete next[key];
    writePositions(next);
  };
  // 범례 드래그 이동 → legend.offset 만 부분 갱신한다(다른 legend 필드 보존).
  // 범례는 이제 패널 본문 기준이다. 표식(offset_space)이 없는 구 config 는 스테이지 좌표이므로
  // 실측 스테이지로 환산해 **보이던 위치를 보존**한다(마커의 sensor_space 이관과 같은 방식).
  const legendInPanelSpace = useMemo(() => {
    const lg = cfg.legend;
    if (!lg?.offset || lg.offset_space === 'panel') return lg;
    if (!(stage.width > 0) || !(bodySize.width > 0)) return lg;
    return { ...lg, offset: stageToContainer(lg.offset, bodySize, stage) };
  }, [cfg.legend, stage, bodySize]);

  const handleLegendOffsetChange = (offset: NormalizedPos) => {
    if (!cfg.legend) return;
    // 새 좌표는 패널 공간이다 — 표식을 함께 남겨 다음 렌더에서 다시 환산되지 않게 한다.
    onConfigChange?.({ legend: { ...cfg.legend, offset, offset_space: 'panel' } });
  };

  // 배치 활성 = 런타임 편집 토글(대시보드) OR forcePlacement(설정 미리보기). 후자는
  // dashboardEditMode 에 의존하지 않는다(설정 다이얼로그에서 드래그 배치 허용).
  const placementActive = editing || forcePlacement;
  // 스택(배경/좌표 공간) 렌더 조건: 데이터가 있거나, 배치 편집 중이거나, 도면 배경 이미지가
  // 설정돼 있으면 렌더한다. 도면이 있으면 데이터 0개여도 배경을 보여준다(빈상태 안내로 배경이
  // 가려지지 않도록). 히트맵 canvas 는 여전히 !isEmpty 일 때만 렌더된다.
  const showStack = !isEmpty || placementActive || hasBackground;
  // 타이틀 바: 패널 옵션(공통) + 제목 존재 여부. 히트맵은 지금까지 제목을 렌더하지 않았으므로
  // 이 조건이 곧 신규 노출 조건이다.
  const headerVisible = showTitle && !!title;

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-2 shadow">
      {/* 타이틀 바(다른 패널과 동형: 아이콘 + 제목). 패널 옵션으로 숨길 수 있고, 제목이 비어 있으면
          도면 영역을 잡아먹지 않도록 렌더하지 않는다. */}
      {headerVisible && (
        <div className="mb-1 flex shrink-0 items-center gap-2" data-testid="heatmap-title">
          <Thermometer className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</span>
        </div>
      )}
      {/* 배치 편집 진입/종료 토글(REQ-03/T7). onConfigChange 가 있고(콜백 없는 MVP 불변) 대시보드
          편집모드일 때만 표시 → gear/삭제 버튼과 동일 게이팅.
          타이틀 바가 있으면 그 아래로 내려 제목을 가리지 않게 한다(둘 다 좌상단을 쓴다). */}
      {canEdit && editMode && !forcePlacement && (
        <button
          type="button"
          data-testid="heatmap-edit-toggle"
          aria-pressed={editing}
          onClick={() => setEditing((v) => !v)}
          className={cn(
            'absolute left-2 z-30 inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium shadow transition-colors',
            headerVisible ? 'top-9' : 'top-2',
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
        // 본문(컨테이너)은 패널을 가득 채우고, 그 안의 스테이지가 기준 도면 종횡비로 레터박스된다.
        // 도면이 없으면 스테이지 = 컨테이너라 기존과 동일한 그림이다.
        // overflow-hidden: stage_fit='cover' 는 스테이지가 본문보다 커지므로(음수 left/top)
        // 여기서 자르지 않으면 도면이 패널 밖으로 새어 다른 패널 위에 그려진다.
        <div ref={setBody} className="relative flex min-h-0 w-full flex-1 overflow-hidden">
        <div
          data-testid="heatmap-stage"
          className="absolute overflow-hidden"
          style={
            stage.width > 0
              ? { left: stage.left, top: stage.top, width: stage.width, height: stage.height }
              : { inset: 0 }
          }
        >
          <FloorPlanBackground
            layers={cfg.floor_plans}
            sources={floorPlanSources}
            stretch={cfg.stage_fit === 'stretch'}
          />
          <div className="absolute inset-0 z-10 flex" style={heatmapLayerStyle}>
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
              decimals={explicitDecimals}
            />
          )}
          {placementActive && (
            <SensorPlacementOverlay
              placed={placed}
              unplaced={unplacedNames}
              labels={sensorLabels}
              onPositionChange={handlePositionChange}
              onRemove={handleRemove}
              snap={cfg.editor?.snap}
              markerSize={cfg.editor?.marker_size}
            />
          )}
          {/* 진단용 미배치 배지(비가림). 스택이 렌더되는 경로(특히 도면 배경이 붙은 경우)에서는
              "미배치 N" 안내가 빈 상태 분기에만 있어 영영 표시되지 않았다 → 판독값은 오는데
              좌표가 없어 아무것도 안 그려지는 히트맵이 "그냥 조용한 히트맵"처럼 보였고, 이것이
              이 결함의 진단을 어렵게 만든 원인이다. 배경 유무와 무관하게 카운트를 노출하되,
              그림을 가리지 않도록 작은 모서리 배지 + pointer-events-none 으로 둔다. 배치 편집
              중에는 미배치 팔레트가 같은 정보를 더 잘 보여주므로 생략한다. */}
          {!placementActive && unplacedNames.length > 0 && (
            <span
              data-testid="heatmap-unplaced-hint"
              className="pointer-events-none absolute bottom-2 left-2 z-30 max-w-[70%] rounded-md bg-(--color-bg-elevated) px-2 py-1 text-[10px] leading-tight text-(--color-text-muted) shadow"
            >
              {t('dashboard.heatmap.unplaced').replace('{count}', String(unplacedNames.length))}
            </span>
          )}
        </div>
          {/* 도면 직접 배치 — 배치 편집 중에만. 스테이지 바깥에 두어야 도면을 패널 밖까지
              끌어낼 수 있다(스테이지 안이면 자기 자신에 잘린다). 마커는 스테이지 정규화
              좌표라 변형을 따라 자동으로 함께 움직인다. */}
          {placementActive && canEdit && hasBackground && (
            <FloorPlanTransformOverlay
              stage={stage}
              container={bodySize}
              transform={cfg.stage_transform}
              onChange={(next) => onConfigChange?.({ stage_transform: next })}
              onReset={() => onConfigChange?.({ stage_transform: undefined })}
            />
          )}
          {/* 값→색 색표 범례 — 스테이지(도면 박스) 바깥, 패널 본문에 둔다. 스테이지 안에 두면
              overflow-hidden 에 잘려 도면 밖으로 내보낼 수 없었다(보고된 제약). 본문 기준이므로
              도면 여백/잘림과 무관하게 패널 어디에나 놓을 수 있다. */}
          {cfg.legend?.enabled && legendInPanelSpace && (
            <HeatmapLegend
              bounds={bounds}
              colorTable={colorTable}
              legend={legendInPanelSpace}
              // 드래그는 배치 편집 중(대시보드 편집모드 토글 또는 설정 미리보기)에만 허용한다 —
              // 센서 마커 배치와 동일한 게이팅이라 뷰어 동작은 그대로다.
              draggable={placementActive && canEdit}
              onOffsetChange={handleLegendOffsetChange}
              decimals={explicitDecimals}
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
