// 게이지 차트 패널 컴포넌트.
// 7가지 게이지 타입을 SVG로 렌더링한다.
// simple(도넛), half(반원), needle(원형 니들),
// needle-rainbow(레인보우), vertical-bar(세로 바), half-rainbow(5단계 등급).
//
// SPEC-CHART-002 M4: 다른 차트 패널과 동일한 공용 Store 데이터 소스(`store_source`)를
// 읽는 **신규 경로**가 추가되었다. 신규 경로는 `data_source === 'store'` + store_source
// 활성 + `series_reduce` 지정이 모두 성립할 때만 진입하며(§2.9 [S1]), 그 외에는 기존
// `config.dataSources[]` 레거시 경로가 한 픽셀도 바뀌지 않는다.
//
// SPEC-CHART-002 M5: 두 경로의 **우선순위 판정**을 `charts/gaugeLegacyBinding.ts` 의
// 순수 함수(`resolveGaugeValueSource`)로 옮겼다. 이 컴포넌트는 판정 결과로 경로를 고를
// 뿐 조건식을 갖지 않는다 — 값 해석 경로가 7지점에 분산되어 있어 조건을 인라인으로
// 두면 조용한 회귀를 만들기 때문이다(plan.md §4). 판정 진리표 7행은
// `gaugeLegacyBinding.test.ts` 가 전수로 잠근다.

import { useCallback, useEffect, useMemo, useState, type ReactElement } from 'react';
import { Gauge as GaugeIcon } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { post } from '@/services/api/client';
import { useAgents } from '@/hooks/useAgent';

import {
  DEFAULT_CHART_LEGEND_FONT_SIZE,
  type SeriesReduceFunc,
} from './charts/chartChannelTypes';
import { ConnectionStatusIcon } from './charts/ConnectionStatusIcon';
import { toNumber } from './charts/chartChannelUtils';
import {
  gaugeValueSourceFlags,
  resolveGaugeValueSource,
} from './charts/gaugeLegacyBinding';
import { reduceAllSeries, type ReducedSeries } from './charts/seriesReduce';
import { SeriesTileGrid } from './charts/SeriesTileGrid';
import { usePanelSeriesData } from './charts/usePanelSeriesData';
import { resolveStoreAgentName } from './charts/storeAgentResolve';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';
// 게이지 모양 8종은 sysmetrics 패널과 공유한다 (panels/gauge/gaugeShapes).
import {
  parseConfig,
  readVBarSize,
  renderGaugeByType,
  VBAR_SIZE_MAX,
  VBAR_SIZE_MIN,
  withGaugeValue,
} from './gauge/gaugeShapes';
import { GaugeValueOverlay } from './gauge/gaugeValue';
import { resolveValuePlacement } from './gauge/valueOffsetMigration';
import { readValueScale, VALUE_SCALE_MAX, VALUE_SCALE_MIN } from './charts/valueScale';
import {
  panelBoxTransform,
  PANEL_OFFSET_LIMIT,
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
  readPanelOffset,
  readPanelSize,
} from './charts/panelGeometry';
import { STAT_OFFSET_LIMIT } from './charts/statLayout';
import {
  resolveFontColor,
  resolveFontFamily,
  resolveFontSize,
  type ChartFontFamily,
} from './charts/textStyle';
import { usePanelEditMode } from './PanelEditToggle';
import { GaugeDragLayer } from '../GaugeDragLayer';
import { useTranslation } from '@/lib/i18n';

import { isAxisSplitGauge } from './gauge/gaugeAxis';

import { PanelEditGrid } from '../PanelEditGrid';
import { PanelAlignToolbar } from '../PanelAlignToolbar';
import { usePanelElementEdit } from '../usePanelElementEdit';
import {
  PanelResizeHandle,
  PANEL_EDIT_OUTLINE_CLASS,
  PANEL_SELECTED_OUTLINE_CLASS,
} from '../PanelDragLayer';
import {
  GaugeThresholdLegend,
  type ThresholdLegendOrientation,
  type ThresholdLegendPosition,
} from './gauge/GaugeThresholdLegend';
import { thresholdLegendItems } from './gauge/thresholdLegend';
import { LEGEND_OFFSET_SAFETY_LIMIT, clampStoredLegendOffset } from './charts/legendOverlay';

// ---- 타입 정의 ----


export type { GaugeType } from './gauge/gaugeShapes';

interface GaugePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
  /** 설정 미리보기처럼 **항상** 편집인 자리인가(토글을 감춘다). */
  forceEdit?: boolean;
}

/** GaugeSection 에서 저장하는 데이터 소스 바인딩 형상 (PanelSettingsDialog 와 동일) */
interface GaugeDataSource {
  sourceType: 'resource' | 'flow' | 'chart-emitter' | 'store';
  resource?: string;
  flowId?: string;
  dataField?: string;
  channelName?: string;
  displayField?: string;
  /** store 소스 전용: Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  storeAgentId?: string;
  /** store 소스 전용: Store 에이전트 이름(표시/폴백). @spec SPEC-WEB-006 */
  storeAgent?: string;
  storeKey?: string;
  storeNamespace?: string;
}

function pickStoreSource(config: Record<string, unknown>): GaugeDataSource | undefined {
  const list = config.dataSources as GaugeDataSource[] | undefined;
  if (!Array.isArray(list)) return undefined;
  // SPEC-WEB-006: storeAgentId(정본) 또는 storeAgent(구 config 이름) 중 하나로 바인딩 판별.
  return list.find(
    (d) => d?.sourceType === 'store' && (!!d.storeAgentId || !!d.storeAgent) && !!d.storeKey,
  );
}

/** Store 최신 값 폴링 훅 (5초 주기) */
function useStoreLatestValue(
  source: GaugeDataSource | undefined,
): number | undefined {
  const [value, setValue] = useState<number | undefined>(undefined);

  // SPEC-WEB-006: 저장된 storeAgentId 를 현재 에이전트 이름으로 해석해 호출한다.
  // 에이전트 이름이 바뀌어도 id 는 불변이므로 항상 현재 이름으로 조회된다.
  // 구 config(storeAgentId 부재)는 저장된 storeAgent 이름을 그대로 사용(하위호환).
  const { data: agentsResult } = useAgents();
  const resolvedAgentName = resolveStoreAgentName(
    source?.storeAgentId,
    source?.storeAgent ?? '',
    agentsResult?.data,
  );

  const fetchValue = useCallback(async () => {
    if (!resolvedAgentName || !source?.storeKey) return;
    try {
      const resp = await post<{
        entries: Array<{ value: unknown; timestamp: number }>;
      }>(`/store/${encodeURIComponent(resolvedAgentName)}/query`, {
        key: source.storeKey,
        mode: 'latest',
        namespace: source.storeNamespace ?? 'default',
      });
      if (resp.entries && resp.entries.length > 0) {
        const field = source.displayField ?? 'value';
        const raw = field === 'value'
          ? resp.entries[0]!.value
          : undefined;
        const n = toNumber(raw);
        setValue(Number.isFinite(n) ? n : undefined);
      }
    } catch {
      // 조회 실패 시 이전 값 유지
    }
  }, [resolvedAgentName, source?.storeKey, source?.storeNamespace, source?.displayField]);

  useEffect(() => {
    if (!resolvedAgentName || !source?.storeKey) {
      setValue(undefined);
      return;
    }
    fetchValue();
    const id = window.setInterval(fetchValue, 5000);
    return () => window.clearInterval(id);
  }, [resolvedAgentName, source?.storeKey, fetchValue]);

  return value;
}


/**
 * 다중 출력 게이지 1개 — 게이지 + 시리즈 이름 캡션.
 *
 * 색상 축 분리(§2.6): **호(arc) 색은 기존 `thresholds` 가 계속 소유**하고 시리즈 색은
 * 캡션 라벨에만 적용한다. 그래서 `base`(패널 공통 min/max/unit/thresholds/gaugeType)를
 * 그대로 넘기고 `value` 만 시리즈 대표값으로 갈아 끼운다.
 *
 * 대표값이 없으면(`undefined`) 슬롯은 유지하고 값 자리에 `--` 를 표시한다(§2.4) —
 * 각 렌더러가 `hasValue={false}` 에서 이미 그렇게 그린다.
 */
/**
 * 게이지 그림 상자 — **크기와 자리**를 얹는다.
 *
 * 게이지 모양은 6종이고 저마다 다른 `viewBox` 로 그려지므로, 크기·자리를 모양마다
 * 손보면 6곳이 갈린다. 대신 그려진 결과를 상자째 CSS transform 으로 옮기고 줄인다 —
 * 한 곳에서 6종 모두에 같은 규칙이 걸린다.
 *
 * 파이와 같은 어휘(백분율 크기 · 백분율 오프셋)를 쓴다(`panelGeometry`) — 두 패널의
 * 같은 설정이 다른 범위를 갖지 않게 한다.
 */
function GaugeBox({
  config,
  children,
  edit,
}: {
  config: Record<string, unknown>;
  children: React.ReactNode;
  /** 편집 표식(정렬이 상자를 찾고, 선택 윤곽을 입는다). 미지정이면 종전 DOM 이다. */
  edit?: { props: Record<string, unknown>; outline: string; handle?: React.ReactNode };
}): ReactElement {
  const transform = panelBoxTransform(
    readPanelSize(config.gauge_size) ?? PANEL_SIZE_MAX,
    readPanelOffset(config.gauge_offset_x),
    readPanelOffset(config.gauge_offset_y),
  );
  return (
    <div
      // 크기 손잡이가 모서리에 붙으려면 기준이 필요하다. 이미 영역을 꽉 채우는
      // 상자라 `relative` 는 배치를 바꾸지 않는다.
      className={`relative h-full w-full ${edit?.outline ?? ''}`}
      data-testid="gauge-box"
      data-gauge-body=""
      style={{ transform }}
      {...(edit?.props ?? {})}
    >
      {edit?.handle}
      {children}
    </div>
  );
}

/** 시리즈 이름(타일 캡션)의 글자 스타일과 자리. */
interface CaptionStyle {
  family?: ChartFontFamily;
  size?: number;
  color?: string;
  position: 'top' | 'bottom';
}

function readCaptionStyle(config: Record<string, unknown>): CaptionStyle {
  return {
    family: config.caption_font_family as ChartFontFamily | undefined,
    size: resolveFontSize(config.caption_font_size),
    color: resolveFontColor(config.caption_font_color),
    position: config.caption_position === 'top' ? 'top' : 'bottom',
  };
}

function GaugeTile({
  item,
  base,
  config,
  caption,
}: {
  item: ReducedSeries;
  base: ReturnType<typeof parseConfig>;
  /** 크기·자리를 얹기 위한 원본 config(타일마다 같은 규칙이 걸린다). */
  config: Record<string, unknown>;
  caption: CaptionStyle;
}): ReactElement {
  const hasValue = item.value !== undefined && Number.isFinite(item.value);
  const parsed = hasValue ? withGaugeValue(base, item.value!) : base;
  const valuePlacement = resolveValuePlacement(config);
  return (
    <div
      className={cn(
        'flex min-h-0 w-full min-w-0 flex-1 flex-col',
        // 이름을 위에 두려면 열 순서를 뒤집는다 — 두 요소의 순서만 바뀌고 각자의
        // 레이아웃(게이지가 남는 높이를 차지)은 그대로다.
        caption.position === 'top' && 'flex-col-reverse',
      )}
      data-testid="gauge-tile-body"
      data-caption-position={caption.position}
    >
      {/*
        게이지 상자는 **행이 준 높이를 그대로 받는다**(`flex-1` + `min-h-0`). 종횡비는 SVG
        의 `viewBox` + 기본 `preserveAspectRatio`(=meet)가 맞추므로, 상자가 넓든 좁든 그림은
        두 축 안에 들어가도록 축소되고 남는 쪽에 여백이 생긴다.

        종전에는 상자에 `aspect-ratio` 를 주고 `w-full max-h-full` 로 폭 기준 정사각형을
        만들었다. 그런데 `max-height: 100%` 는 부모 높이가 확정되어야 의미가 있는데 행 높이가
        내용으로 결정되던 탓에(순환) 무시됐고, 낮고 넓은 패널에서 상자가 폭만큼 높아져
        `overflow-hidden` 에 위아래가 잘렸다. 행 높이는 이제 `fillRows` 가 먼저 확정한다.
      */}
      <div
        data-testid="gauge-tile-chart"
        className="min-h-0 w-full min-w-0 flex-1"
      >
        <GaugeBox config={config}>{renderGaugeByType(parsed, hasValue)}</GaugeBox>
        {/* 값은 도형과 **형제**다 — 도형에 걸린 크기·위치 변형을 따라가지 않는다. */}
        <GaugeValueOverlay
          parsed={parsed}
          hasValue={hasValue}
          offsetX={valuePlacement.percentX}
          offsetY={valuePlacement.percentY}
          viewBoxOffsetX={valuePlacement.viewBoxX}
          viewBoxOffsetY={valuePlacement.viewBoxY}
        />
      </div>
      <span
        data-testid="gauge-tile-caption"
        title={item.name}
        className={cn(
          'w-full truncate text-center font-medium',
          // 크기를 지정하지 않았을 때만 기본 클래스가 산다 — 두 곳이 함께 걸리면
          // 지정한 크기가 클래스에 덮인다.
          caption.size === undefined && 'text-xs',
          // 색 우선순위: 지정색 > 시리즈 색 > 흐린 테마색. 시리즈 색은 그 시리즈를
          // 가리키는 표시이므로 지정색이 없을 때만 이긴다.
          !caption.color && !item.color && 'text-(--color-text-muted)',
        )}
        style={{
          fontFamily: resolveFontFamily(caption.family),
          fontSize: caption.size === undefined ? undefined : `${caption.size}px`,
          color: caption.color ?? item.color,
        }}
      >
        {item.name}
      </span>
    </div>
  );
}

/**
 * 판정이 `legacy` 일 때 공용 시리즈 훅에 넘기는 빈 config.
 *
 * `data_source` 가 없으므로 계약이 `channel` 로 해석하고, 그러면 store·tsdb 훅 모두
 * 인자를 받지 못해 구독도 폴링도 일어나지 않는다 — 종전 `useStoreChartData(undefined,
 * false)` 와 같은 idle 상태다. 모듈 상수로 두어 렌더마다 새 객체가 생기지 않게 한다.
 */
const IDLE_SERIES_CONFIG: Record<string, unknown> = Object.freeze({});

// ---- 메인 컴포넌트 ----

/** 게이지 차트 패널 */
/**
 * 공용 편집 표면이 다루는 요소 — 게이지 상자 · 값 글자 · 임계값 범례.
 *
 * 값 글자는 한때 빠져 있었다. 그 오프셋이 SVG `viewBox` 좌표여서 백분율을 쓰는 공용
 * 정렬과 단위가 맞지 않았기 때문이다. 지금은 값이 도형 밖 오버레이가 되면서 **패널
 * 상자 대비 백분율**을 쓰므로 그 이유가 사라졌다 — 셋이 같은 축을 쓴다.
 */
type GaugeElementKind = 'body' | 'value' | 'legend';
const GAUGE_ELEMENT_KINDS: readonly GaugeElementKind[] = ['body', 'value', 'legend'];

export default function GaugePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  onTitleChange: _onTitleChange,
  forceEdit = false,
}: GaugePanelProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();

  // ---- SPEC-CHART-002 M5: 값 소스 판정 ----
  //
  // 판정 자체는 순수 함수가 소유한다(`charts/gaugeLegacyBinding.ts`). 여기서는 결과로
  // 경로를 고르기만 한다. `'store-source'` 는 세 조건의 논리곱이 성립할 때만 나온다 —
  //   1) `data_source === 'store'`   — 사용자가 토글로 명시한 상태
  //   2) `store_source` 활성          — keys 모드 시리즈 ≥ 1 또는 tag 모드 태그 ≥ 1
  //   3) `series_reduce` 지정         — 부재는 "기본값 last" 가 아니라 레거시 경로다
  // 즉 **신규 경로가 실제로 값을 낼 수 있을 때만** 레거시를 밀어낸다(§2.9 [S1] / §4.5).
  const seriesReduce = config.series_reduce as SeriesReduceFunc | undefined;
  const isStoreSourcePath =
    resolveGaugeValueSource(gaugeValueSourceFlags(config)) === 'store-source';

  // ---- 두 경로의 훅 배선 ----
  //
  // 세 훅 모두 조건 없이 항상 호출하고, 진 쪽을 `undefined` 인자로 idle 에 둔다
  // (React 훅 규칙 — `LineChartPanel` / stat / bar / pie 와 같은 형태). idle 경로에서는
  // 구독도 폴링도 일어나지 않으므로, store-source 가 이긴 게이지는 레거시
  // `mode:'latest'` 5초 폴링을 더 이상 발생시키지 않는다.
  //
  // 레거시 계산 코드는 **삭제하지 않는다** — 판정이 `'legacy'` 인 순간 그대로 되살아나며
  // 그것이 이관의 되돌리기 경로다(§4.5). 특성화 CH-11~CH-18 이 이 가지를 계속 지킨다.
  const storeSource = isStoreSourcePath ? undefined : pickStoreSource(config);
  const storeValue = useStoreLatestValue(storeSource);

  // 공용 시리즈 훅 하나로 Store 와 TSDB 를 모두 받는다(다른 차트 패널과 같은 배선).
  // 종전에는 `useStoreChartData` 를 직접 불러 store 만 조회했으므로, 게이지에서 TSDB 를
  // 고르면 설정 화면에는 토글이 보이는데 값이 오지 않는 상태였다.
  //
  // 게이지 고유의 레거시 우선순위(§2.9 진리표)는 **여기서 그대로 유지**한다. config 를
  // 조건 없이 넘기면 `series_reduce` 없는 store 게이지까지 폴링이 시작되어, 판정이
  // `legacy` 인데 조회는 도는 상태가 된다. 그래서 판정이 진 경우 빈 config 를 넘겨
  // 종전 `useStoreChartData(undefined, false)` 와 같은 idle 로 둔다.
  const storeChart = usePanelSeriesData(isStoreSourcePath ? config : IDLE_SERIES_CONFIG);

  // 레거시 경로의 값은 이제 store 하나다.
  //
  // 종전에는 `chart-emitter > store > static` 이라는 **값** 우선순위가 있었다. chart-emitter
  // 는 채널로 들어오는 값이었고, 채널이 패널 소스에서 빠지면서 그 항이 사라졌다 — 같은
  // 데이터는 Store 소스로 받는다. 우선순위 자체가 없어진 것이지 store 항이 바뀐 것은 아니다.
  //
  // store-source 경로가 이긴 경우 레거시 훅이 idle 이므로 값은 undefined 가 되고
  // `hasBinding` 은 false 가 된다. 렌더 분기가 그 경우 `hasValue` 를 강제로 false 로
  // 넘기므로 표시 결과는 종전과 같다.
  const liveValue = storeValue;
  const hasBinding = !!storeSource;

  const parsedBase = parseConfig(config);
  const hasValue = !hasBinding || liveValue !== undefined;
  const parsed = liveValue !== undefined ? withGaugeValue(parsedBase, liveValue) : parsedBase;

  // 시리즈 순서 그대로 대표값 1개씩. 재조회 없이 렌더 시점에만 계산된다(§2.7 [E1]).
  const reduced = useMemo<ReducedSeries[] | null>(
    () =>
      isStoreSourcePath && seriesReduce !== undefined
        ? reduceAllSeries(
            storeChart.seriesEntries,
            storeChart.seriesNames,
            storeChart.seriesStyles,
            seriesReduce,
          )
        : null,
    [
      isStoreSourcePath,
      seriesReduce,
      storeChart.seriesEntries,
      storeChart.seriesNames,
      storeChart.seriesStyles,
    ],
  );
  const showGauges = reduced !== null && reduced.length > 0;

  const caption = readCaptionStyle(config);
  // 손잡이의 스크린리더 이름에만 쓴다. 정렬 툴바가 이미 같은 훅을 쓰므로 이 패널의
  // 렌더 트리에 새로운 Provider 요구가 생기지 않는다.
  const { t } = useTranslation();

  // 대시보드 패널에서도 값·범례·게이지를 끌어 배치한다(히트맵과 같은 규칙).
  const edit = usePanelEditMode({
    canEdit: typeof onConfigChange === 'function',
    forced: forceEdit,
    testId: 'gauge-edit-toggle',
    below: showTitle,
  });

  /**
   * 끌 수 있는 요소 — 드래그 계산은 `GaugeDragLayer` 가 계속 소유한다(범례는 변별 죄기
   * 규칙이라 공용 레이어가 대신할 수 없다). 여기서 공용화하는 것은 **편집 표면**
   * (그리드·중심 표식·정렬·선택 구분)뿐이다.
   */
  // 값 글자의 자리 — 지금 좌표(백분율)와 옛 좌표(viewBox)를 함께 읽는다. 옛 좌표는
  // 그리기에만 쓴다: 끌기·정렬은 화면에서 잰 자리를 기준으로 삼으므로 옛 몫이 이미
  // 들어간 자리에서 출발하고, 저장은 백분율 쪽에만 쌓인다.
  const valuePlacement = resolveValuePlacement(config);
  const valueOffsets = { x: valuePlacement.percentX, y: valuePlacement.percentY };
  // 상한은 요소마다 다르다 — 게이지 상자는 영역을 채우는 그림이라 `readPanelOffset` 과
  // 같은 ±40, 값 글자와 범례는 작은 글자 덩어리라 ±50 이다(`GaugeDragLayer` 의 죄기와
  // 같은 값이어야 끌기와 정렬이 서로 다른 자리에서 멈추지 않는다).
  const gaugeOffsets = {
    body: {
      x: readPanelOffset(config.gauge_offset_x),
      y: readPanelOffset(config.gauge_offset_y),
      limit: PANEL_OFFSET_LIMIT,
    },
    value: { ...valueOffsets, limit: STAT_OFFSET_LIMIT },
    legend: {
      x: clampStoredLegendOffset(config.threshold_legend_offset_x),
      y: clampStoredLegendOffset(config.threshold_legend_offset_y),
      limit: LEGEND_OFFSET_SAFETY_LIMIT,
    },
  };
  const {
    selection,
    setSelection,
    snap,
    setSnap,
    boundsRef,
    align,
    reset,
  } = usePanelElementEdit<GaugeElementKind>({
    kinds: GAUGE_ELEMENT_KINDS,
    enabled: edit.active,
    offsets: gaugeOffsets,
    writeOffsets: (patches) => {
      const next: Record<string, unknown> = {};
      for (const p of patches) {
        if (p.kind === 'body') {
          next.gauge_offset_x = p.x;
          next.gauge_offset_y = p.y;
        } else if (p.kind === 'value') {
          next.value_pos_x = p.x;
          next.value_pos_y = p.y;
          // 지우는 패치(정렬 툴바의 배치 초기화)면 옛 좌표도 함께 지운다. 신규 키만
          // 지우면 옛 몫이 남아 상자·범례만 제자리로 가고 값은 그대로 — 초기화가
          // 반쪽이 된다(설정의 값 초기화 단추가 이미 같은 이유로 둘 다 지운다).
          // 옮기는 패치(정렬·무리 이동)에서는 지우지 않는다: 옛 몫은 화면에서 잰
          // 출발 자리에 이미 들어가 있으므로, 여기서 빼면 그만큼 값이 튄다.
          if (p.x === undefined && p.y === undefined) {
            next.value_offset_x = undefined;
            next.value_offset_y = undefined;
          }
        } else {
          next.threshold_legend_offset_x = p.x;
          next.threshold_legend_offset_y = p.y;
        }
      }
      onConfigChange?.(next);
    },
  });
  /** 요소 상자에 붙는 편집 표식 — 정렬이 상자를 찾고, 누르면 선택된다. */
  const editProps = (kind: GaugeElementKind) =>
    edit.active
      ? {
          'data-panel-drag': kind,
          onPointerDown: () =>
            setSelection(
              selection.has(kind) ? selection : new Set<GaugeElementKind>([kind]),
            ),
        }
      : {};
  const outlineOf = (kind: GaugeElementKind): string =>
    edit.active
      ? selection.has(kind)
        ? PANEL_SELECTED_OUTLINE_CLASS
        : PANEL_EDIT_OUTLINE_CLASS
      : '';

  // 임계값 범례 — 패널에 하나만. 임계값은 패널 설정이라 타일마다 붙이면 같은 문구가
  // 시리즈 수만큼 반복되면서 게이지 자리를 잡아먹는다.
  const thresholdLegend = useMemo(() => {
    if (config.show_threshold_legend !== true) return null;
    const items = thresholdLegendItems(
      parsedBase.thresholds,
      parsedBase.unit,
      config.decimal_places as number | undefined,
    );
    return items.length > 0 ? items : null;
  }, [config.show_threshold_legend, config.decimal_places, parsedBase.thresholds, parsedBase.unit]);
  const thresholdLegendNode = thresholdLegend && (
    <GaugeThresholdLegend
      items={thresholdLegend}
      position={
        (config.threshold_legend_position === 'top' ? 'top' : 'bottom') as ThresholdLegendPosition
      }
      orientation={
        (config.threshold_legend_orientation === 'vertical'
          ? 'vertical'
          : 'horizontal') as ThresholdLegendOrientation
      }
      edit={
        edit.active
          ? {
              props: editProps('legend'),
              outline: outlineOf('legend'),
              handle: selection.has('legend') ? (
                <PanelResizeHandle
                  kind="legend"
                  enabled
                  label={t('dashboard.chart.legendFontSize')}
                  // 범례는 넘침을 자르므로(`overflow-auto`) 손잡이를 안쪽 모서리에 둔다.
                  className="sticky bottom-0 left-full h-3 w-3 shrink-0 cursor-nwse-resize rounded-sm border border-white bg-blue-500 shadow"
                />
              ) : undefined,
            }
          : undefined
      }
      fontFamily={config.threshold_legend_font_family as ChartFontFamily | undefined}
      fontSize={resolveFontSize(config.threshold_legend_font_size)}
      fontColor={resolveFontColor(config.threshold_legend_font_color)}
      offsetX={clampStoredLegendOffset(config.threshold_legend_offset_x)}
      offsetY={clampStoredLegendOffset(config.threshold_legend_offset_y)}
    />
  );

  return (
    <div className={cn(
      'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4',
      'ring-1 ring-(--color-border-default)',
    )}>
      {/*
        연결 상태 아이콘. Store 데이터 소스 경로에서는 조회 상태를 노출한다(M4.6).
        레거시 경로의 규칙은 그대로다 — chart-emitter 구독 중일 때만 표시하고 레거시
        store 폴링에는 표시하지 않는다(특성화 CH-17 이 잠근 동작).
      */}
      {isStoreSourcePath ? (
        <div className="absolute right-3 top-3 z-10">
          <ConnectionStatusIcon status={storeChart.status} />
        </div>
      ) : null}
      {/* 헤더 */}
      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <GaugeIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)" style={titleStyle}>{title}</span>
        </div>
      )}
      {/*
        게이지 영역. 임계값 범례가 이 안에 **겹쳐** 뜨므로 기준 상자가 필요하다 —
        흐름에 끼워 넣으면 범례가 자리를 나눠 가져 게이지가 그만큼 작아지고, 끌어 옮긴
        오프셋과 레이아웃이 서로를 밀어낸다.
      */}
      {edit.toggle}
      <GaugeDragLayer
        enabled={edit.active}
        snap={snap}
        selection={selection}
        onSelectionChange={setSelection}
        value={{
          offsetX: valueOffsets.x,
          offsetY: valueOffsets.y,
          onChange: ({ x, y }) => onConfigChange?.({ value_pos_x: x, value_pos_y: y }),
          // 값의 크기는 px 이 아니라 **배율**(0.3~3)이다. 1px 을 1 로 세면 조금만
          // 끌어도 상한에 닿으므로 100px 에 배율 1 이 되도록 잘게 센다.
          size: readValueScale(config.value_scale),
          sizeRange: { min: VALUE_SCALE_MIN, max: VALUE_SCALE_MAX },
          sizeStep: 0.01,
          onResize: (value_scale) => onConfigChange?.({ value_scale }),
        }}
        legend={{
          offsetX: clampStoredLegendOffset(config.threshold_legend_offset_x),
          offsetY: clampStoredLegendOffset(config.threshold_legend_offset_y),
          onChange: ({ x, y }) =>
            onConfigChange?.({ threshold_legend_offset_x: x, threshold_legend_offset_y: y }),
          // 범례는 글자 덩어리라 손잡이가 글자 크기(px)를 바꾼다.
          size: resolveFontSize(config.threshold_legend_font_size) ?? DEFAULT_CHART_LEGEND_FONT_SIZE,
          onResize: (threshold_legend_font_size) =>
            onConfigChange?.({ threshold_legend_font_size }),
        }}
        body={{
          offsetX: readPanelOffset(config.gauge_offset_x),
          offsetY: readPanelOffset(config.gauge_offset_y),
          onChange: ({ x, y }) => onConfigChange?.({ gauge_offset_x: x, gauge_offset_y: y }),
          // 게이지는 글자가 아니므로 손잡이가 백분율(`gauge_size`)을 바꾼다.
          size: readPanelSize(config.gauge_size) ?? PANEL_SIZE_MAX,
          sizeRange: { min: PANEL_SIZE_MIN, max: PANEL_SIZE_MAX },
          onResize: (gauge_size) => onConfigChange?.({ gauge_size }),
          // 세로바만 축을 나눈다 — 가로로 끌면 바의 폭, 세로로 끌면 바의 높이.
          //
          // 이때 손잡이가 잡는 것은 게이지 상자의 CSS 배율이 아니라 **도형의 치수**다.
          // 배율로 잡으면 눈금 글자와 값 글자까지 함께 눌린다(보고된 결함).
          ...(isAxisSplitGauge(config)
            ? {
                size: readVBarSize(config.gauge_bar_width),
                sizeRange: { min: VBAR_SIZE_MIN, max: VBAR_SIZE_MAX },
                onResize: (gauge_bar_width: number) => onConfigChange?.({ gauge_bar_width }),
                sizeY: readVBarSize(config.gauge_bar_height),
                sizeYRange: { min: VBAR_SIZE_MIN, max: VBAR_SIZE_MAX },
                onResizeY: (gauge_bar_height: number) => onConfigChange?.({ gauge_bar_height }),
              }
            : {}),
        }}
      >
      <div
        ref={boundsRef}
        className="relative flex min-h-0 min-w-0 flex-1 flex-col"
        data-panel-bounds=""
      >
      {/* 배치 그리드와 중심 표식 — 요소 뒤에 깔리고 포인터를 받지 않는다. */}
      <PanelEditGrid enabled={edit.active} />
      {showGauges ? (
        <div
          data-testid="gauge-tiles"
          className="flex min-h-0 flex-1 flex-col justify-center overflow-hidden"
        >
          <SeriesTileGrid
            items={reduced}
            limit={config.multi_output_limit as number | undefined}
            rows={config.tile_rows as number | undefined}
            itemKey={(item, i) => `${i}:${item.name}`}
            renderItem={(item) => (
              <GaugeTile item={item} base={parsedBase} config={config} caption={caption} />
            )}
            // 게이지는 내용이 아니라 행이 높이를 정해야 한다 — 그래야 낮은 패널에서
            // 잘리지 않고 축소된다(stat 타일은 종전대로 내용 높이를 쓴다).
            fillRows
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 items-center justify-center">
          {/* Store 경로인데 시리즈가 0개면 신규 경로의 빈 상태(`--`)를 보여준다(§2.4).
              레거시 값으로 몰래 되돌아가지 않는다. */}
          <GaugeBox
            config={config}
            edit={
              edit.active
                ? {
                    props: editProps('body'),
                    outline: outlineOf('body'),
                    handle: selection.has('body') ? (
                      <PanelResizeHandle
                        kind="body"
                        enabled
                        label={t('dashboard.settings.gaugeSection.gaugeSize')}
                      />
                    ) : undefined,
                  }
                : undefined
            }
          >
            {renderGaugeByType(parsed, isStoreSourcePath ? false : hasValue)}
          </GaugeBox>
          <GaugeValueOverlay
            parsed={parsed}
            hasValue={isStoreSourcePath ? false : hasValue}
            offsetX={valueOffsets.x}
            offsetY={valueOffsets.y}
            viewBoxOffsetX={valuePlacement.viewBoxX}
            viewBoxOffsetY={valuePlacement.viewBoxY}
            edit={
              edit.active
                ? {
                    props: editProps('value'),
                    outline: outlineOf('value'),
                    // 손잡이는 고른 요소에만 — 늘 띄우면 값 아래에 상시로 점이 붙는다.
                    showHandle: selection.has('value'),
                  }
                : undefined
            }
          />
        </div>
      )}
      {thresholdLegendNode}
      </div>
      </GaugeDragLayer>

      {/* 정렬 툴바 — 편집 중에만. 격자 붙임은 이 패널의 드래그 레이어가 아직 하지 않으므로
          토글을 내지 않는다(없는 기능의 스위치를 두면 죽은 컨트롤이 된다). */}
      <PanelAlignToolbar
        enabled={edit.active}
        snap={snap}
        onSnapChange={setSnap}
        onAlign={align}
        onReset={reset}
      />
    </div>
  );
}

