// HeatmapPanel 견고성/렌더 분기 테스트 (SPEC-HEATMAP-PANEL-001 T6/T8/T9).
// AC-E1(빈 상태), AC-E2(미배치 안내), AC-E3(폴링 실패 시 마지막 렌더 유지 + 오류 배지),
// 정상 경로(배치 센서 → canvas 렌더)를 컴포넌트 레벨에서 커버한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { ChartEntry } from '../charts/chartChannelTypes';
import type { UseStoreChartDataResult } from '../charts/useStoreChartData';

// useStoreChartData 를 주입 가능한 mock 으로 대체한다(네트워크/폴링 없이 결정적 렌더).
const storeMock: { current: UseStoreChartDataResult } = {
  current: {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  },
};
vi.mock('../charts/useStoreChartData', () => ({
  useStoreChartData: () => storeMock.current,
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import HeatmapPanel from './HeatmapPanel';
import { heatmapSensorId } from './sensorIdentity';
import { useUIStore } from '@/stores/uiStore';

/** 최신값 1개짜리 시리즈 타임라인. */
function reading(value: number): ChartEntry[] {
  return [{ timestamp: 1, value }];
}

/**
 * 좌표/마커의 키 공간은 **시리즈 동일성 키**다. 이 파일의 테스트 config 는 metric/tags 가 없는
 * 시리즈만 쓰므로 key 만으로 동일성 키를 만든다. (raw key 로 적힌 sensor_positions 는 읽는
 * 시점에 이 키로 이관된다 — 하위호환 마이그레이션.)
 */
function sid(key: string): string {
  return heatmapSensorId({ key });
}

/**
 * data-testid 조회용 정규화. Testing Library 는 DOM 속성값을 trim + 공백 축약한 뒤 매처와
 * 비교하므로, metric/tags 가 빈 시리즈의 동일성 키(뒤에 공백이 붙는다)를 그대로 넣으면
 * 조회가 어긋난다. 실제 쓰기 키(config)는 항상 정규화하지 않은 `sid()` 를 쓴다.
 */
function tid(sensorId: string): string {
  return sensorId.trim().replace(/\s+/g, ' ');
}

// 히트맵은 "명시적으로 체크된 series(keys)"만 렌더한다(체크박스 = 단일 진실원). 따라서 테스트
// config 도 keys 모드 + 명시적 series 로 구성한다. 마커(placed)는 series 키에서 파생되므로,
// 기본 series 는 배치된 sensor_positions 키에서 유도한다. stale(비선택) 키를 검증하는 테스트는
// seriesKeys 로 series 를 명시적으로 좁힌다.
function makeConfig(
  sensorPositions: Record<string, { x: number; y: number }>,
  extras: Record<string, unknown> = {},
  seriesKeys: string[] = Object.keys(sensorPositions),
) {
  return {
    data_source: 'store',
    store_source: {
      agent_name: 'a',
      namespace: 'default',
      selection_mode: 'keys',
      series: seriesKeys.map((k) => ({ key: k, alias: k })),
      time_window_ms: 1000,
      interval_ms: 1000,
      aggregation: 'last',
    },
    sensor_positions: sensorPositions,
    idw: { power: 2, grid_resolution: 8 },
    ...extras,
  } as Record<string, unknown>;
}

function setStore(partial: Partial<UseStoreChartDataResult>) {
  storeMock.current = { ...storeMock.current, ...partial };
}

beforeEach(() => {
  storeMock.current = {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  };
  // 배치편집 진입 버튼은 대시보드 편집모드에서만 노출된다. 각 테스트 기본은 비편집(false).
  useUIStore.getState().setDashboardEditMode(false);
});

describe('HeatmapPanel', () => {
  it('AC-E1: 센서 0개면 빈 상태 안내를 표시하고 canvas 를 렌더하지 않는다', () => {
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(<HeatmapPanel panelId="p" config={makeConfig({})} />);
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('AC-E2: 좌표 미지정 센서만 있으면 빈 상태 + 미배치 안내를 표시한다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({})} />);
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    // 미배치 안내(키 반환 mock 이므로 키 문자열로 확인).
    expect(screen.getByText('dashboard.heatmap.unplaced')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('배치된 센서가 있으면 HeatmapCanvas 를 렌더한다(정상 경로)', () => {
    setStore({
      seriesNames: ['s1', 's2'],
      seriesEntries: new Map([
        ['s1', reading(20)],
        ['s2', reading(26)],
      ]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.2, y: 0.2 }, s2: { x: 0.8, y: 0.8 } })}
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.heatmap.emptyState')).toBeNull();
  });

  // SPEC-HEATMAP-PANEL-002 T4: 도면 배경 레이어 + 히트맵 합성 불투명도.
  it('도면 미첨부(MVP)면 배경을 렌더하지 않고 canvas 는 그대로 렌더한다(행위 보존)', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />);
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.queryByTestId('floor-plan-background')).toBeNull();
  });

  it('REQ-01: floor_plan.image 가 있으면 배경을 히트맵 아래에 렌더한다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig(
          { s1: { x: 0.5, y: 0.5 } },
          { floor_plan: { image: 'data:image/png;base64,AAAA' }, heatmap_opacity: 0.4 },
        )}
      />,
    );
    // 배경 + 히트맵 canvas 가 함께 존재한다(레이어 스택).
    const bg = screen.getByTestId('floor-plan-background');
    expect(bg).toBeInTheDocument();
    expect(bg.getAttribute('src')).toBe('data:image/png;base64,AAAA');
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
  });

  // SPEC-HEATMAP-PANEL-002 T7: 인패널 배치 편집 모드.
  it('onConfigChange 미제공(MVP)이면 편집 토글을 렌더하지 않는다(행위 보존)', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />);
    expect(screen.queryByTestId('heatmap-edit-toggle')).toBeNull();
    expect(screen.queryByTestId('sensor-placement-overlay')).toBeNull();
  });

  it('편집모드가 아니면(dashboardEditMode=false) onConfigChange 가 있어도 배치편집 버튼을 표시하지 않는다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    // beforeEach 에서 편집모드 false 로 리셋됨. 콜백은 있으나 편집모드가 아니므로 숨김.
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('heatmap-edit-toggle')).toBeNull();
    // 편집모드로 전환하면 버튼이 나타난다.
    useUIStore.getState().setDashboardEditMode(true);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getAllByTestId('heatmap-edit-toggle').length).toBeGreaterThan(0);
  });

  it('T7: 편집 토글을 누르면 마커 오버레이가 히트맵 위에 마운트된다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(true);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
      />,
    );
    // 진입 전에는 오버레이 없음.
    expect(screen.queryByTestId('sensor-placement-overlay')).toBeNull();
    fireEvent.click(screen.getByTestId('heatmap-edit-toggle'));
    // 진입 후 오버레이 + canvas 공존(폴링/렌더 비파괴, R3).
    expect(screen.getByTestId('sensor-placement-overlay')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${tid(sid('s1'))}`)).toBeInTheDocument();
  });

  it('AC-04: 편집 모드에서 마커 제거는 해당 센서 좌표만 config 에서 삭제한다', () => {
    const onConfigChange = vi.fn();
    setStore({
      seriesNames: ['s1', 's2'],
      seriesEntries: new Map([
        ['s1', reading(20)],
        ['s2', reading(26)],
      ]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(true);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.2, y: 0.2 }, s2: { x: 0.8, y: 0.8 } })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('heatmap-edit-toggle'));
    fireEvent.click(screen.getByTestId(`sensor-remove-${tid(sid('s1'))}`));
    // s1 좌표만 삭제되고 s2 는 보존된다.
    expect(onConfigChange).toHaveBeenCalledWith({
      sensor_positions: { [sid('s2')]: { x: 0.8, y: 0.8 } },
    });
  });

  it('AC-E1(편집 경로): 도면/데이터가 없어도 편집 진입 시 오버레이가 마운트된다', () => {
    // 좌표 미지정 센서만 존재(points 0개 → 평소엔 빈 상태). 편집 진입 시 배치 표면을 렌더.
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(true);
    render(
      <HeatmapPanel panelId="p" config={makeConfig({})} onConfigChange={vi.fn()} />,
    );
    // 진입 전: 빈 상태 안내.
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('heatmap-edit-toggle'));
    // 진입 후: 오버레이 + 미배치 팔레트(s1)가 나타난다.
    expect(screen.getByTestId('sensor-placement-overlay')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-unplaced-${tid(sid('s1'))}`)).toBeInTheDocument();
  });

  // SPEC-HEATMAP-PANEL-003 T4: 등고선 오버레이 마운트(additive, contour off 시 무영향).
  it('회귀 0: contour 미설정이면 등고선 레이어를 렌더하지 않는다(MVP/002 무영향)', () => {
    setStore({
      seriesNames: ['s1', 's2'],
      seriesEntries: new Map([
        ['s1', reading(20)],
        ['s2', reading(26)],
      ]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.2, y: 0.2 }, s2: { x: 0.8, y: 0.8 } })}
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.queryByTestId('contour-layer')).toBeNull();
  });

  it('AC-01: contour.enabled 이면 히트맵 위에 등고선 레이어를 마운트한다(동일 격자)', () => {
    setStore({
      seriesNames: ['s1', 's2'],
      seriesEntries: new Map([
        ['s1', reading(18)],
        ['s2', reading(26)],
      ]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig(
          { s1: { x: 0.1, y: 0.5 }, s2: { x: 0.9, y: 0.5 } },
          { value_bounds: { min: 18, max: 26 }, contour: { enabled: true, level_count: 3 } },
        )}
      />,
    );
    // 히트맵 + 등고선 공존.
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.getByTestId('contour-layer')).toBeInTheDocument();
  });

  it('AC-E2: contour.enabled 이지만 배치 센서 0개면 등고선 레이어가 없다(graceful)', () => {
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(
      <HeatmapPanel panelId="p" config={makeConfig({}, { contour: { enabled: true } })} />,
    );
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    expect(screen.queryByTestId('contour-layer')).toBeNull();
  });

  it('AC-E3: 폴링 실패(status=error)여도 마지막 온도장을 유지하고 오류 배지를 덧띄운다', () => {
    setStore({
      seriesNames: ['s1'],
      // 직전 폴링의 시리즈가 보존된 상태(useStoreChartData 가 prev 를 유지).
      seriesEntries: new Map([['s1', reading(24)]]),
      status: 'error',
      errorReason: 'network',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />);
    // canvas(마지막 렌더)는 파괴되지 않는다.
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    // 오류 배지가 함께 표시된다.
    expect(screen.getByTestId('heatmap-error')).toBeInTheDocument();
  });

  // SPEC-PANEL-SETTINGS-001: 설정 미리보기 배치 편집(forcePlacement). 대시보드 편집모드에
  // 의존하지 않고 오버레이를 항상 켜며, 대시보드 경로(forcePlacement 미지정)는 불변이다.
  it('forcePlacement: dashboardEditMode 없이도 오버레이가 활성화되고 편집 토글은 숨긴다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    // 대시보드 편집모드 false — 그래도 forcePlacement 로 배치 활성.
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    // 토글 버튼 없이도 오버레이 + 마커가 즉시 나타난다(설정 미리보기 배치).
    expect(screen.queryByTestId('heatmap-edit-toggle')).toBeNull();
    expect(screen.getByTestId('sensor-placement-overlay')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${tid(sid('s1'))}`)).toBeInTheDocument();
  });

  it('forcePlacement: 라이브 값이 없어도 좌표만 있는 시리즈는 배치 마커를 렌더한다', () => {
    // 방금 선택된 시리즈: 좌표(중앙)만 있고 라이브 판독값은 아직 없음(seriesEntries 비어 있음).
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map(), // 라이브 값 없음
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    // placed 는 cfg.sensor_positions 에서 직접 파생되므로 값 없이도 드래그 마커가 뜬다.
    expect(screen.getByTestId('sensor-placement-overlay')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${tid(sid('s1'))}`)).toBeInTheDocument();
  });

  it('마커 정렬: 현재 바인딩된 시리즈에만 마커를 렌더하고 잔존 좌표는 유령 마커를 만들지 않는다', () => {
    // 체크된 series 는 s1 뿐(seriesKeys=['s1']). sensor_positions 에 stale(미선택) 키가 남아 있음.
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 }, stale: { x: 0.1, y: 0.1 } }, {}, ['s1'])}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    // 바인딩된 s1 은 마커가 있고, 선택에서 빠진 stale 좌표는 마커가 없다(유령 마커 제거).
    expect(screen.getByTestId(`sensor-marker-${tid(sid('s1'))}`)).toBeInTheDocument();
    expect(screen.queryByTestId(`sensor-marker-${tid(sid('stale'))}`)).toBeNull();
  });

  it('keys 모드: 라이브 값이 없어도 config.series 에 있으면 마커를 렌더한다(방금 선택)', () => {
    // keys 모드 + 라이브 데이터 없음(seriesNames 비어 있음). boundKeys 는 config.series 에서 온다.
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    useUIStore.getState().setDashboardEditMode(false);
    const config = {
      data_source: 'store',
      store_source: {
        agent_name: 'a',
        namespace: 'default',
        selection_mode: 'keys',
        series: [{ key: 's1', alias: 's1' }],
        time_window_ms: 1000,
        interval_ms: 1000,
        aggregation: 'last',
      },
      sensor_positions: { s1: { x: 0.5, y: 0.5 }, stale: { x: 0.2, y: 0.2 } },
      idw: { power: 2, grid_resolution: 8 },
    } as Record<string, unknown>;
    render(
      <HeatmapPanel panelId="p" config={config} onConfigChange={vi.fn()} forcePlacement />,
    );
    expect(screen.getByTestId(`sensor-marker-${tid(sid('s1'))}`)).toBeInTheDocument();
    // config.series 에 없는 stale 은 마커 없음.
    expect(screen.queryByTestId(`sensor-marker-${tid(sid('stale'))}`)).toBeNull();
  });

  it('tag_filters 만 있고 series 가 비면(미체크) 마커/필드 없이 배경만 렌더한다(체크된 series 만 렌더)', () => {
    // 사용자 요구: 히트맵은 체크박스로 명시 선택된 series 만 렌더한다. tag_filters 는 렌더에
    // 사용하지 않으므로, series 가 비면 keys 강제 바인딩으로 데이터가 없다(seriesNames []).
    // sensor_positions 에 태그 매칭 잔존 좌표(indoor)가 있어도 boundKeys 가 비어 마커가 없다.
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'idle' });
    useUIStore.getState().setDashboardEditMode(false);
    const config = {
      data_source: 'store',
      store_source: {
        agent_name: 'a',
        namespace: 'default',
        selection_mode: 'tag',
        tag_filters: { type: 'temperature' },
        series: [],
        time_window_ms: 1000,
        interval_ms: 1000,
        aggregation: 'last',
      },
      sensor_positions: { indoor: { x: 0.5, y: 0.5 } },
      idw: { power: 2, grid_resolution: 8 },
      floor_plan: { image: 'data:image/png;base64,AAAA' },
    } as Record<string, unknown>;
    render(
      <HeatmapPanel panelId="p" config={config} onConfigChange={vi.fn()} forcePlacement />,
    );
    // 배경은 렌더되지만, 미체크(태그 매칭) 시리즈의 마커/필드는 렌더되지 않는다.
    expect(screen.getByTestId('floor-plan-background')).toBeInTheDocument();
    expect(screen.queryByTestId(`sensor-marker-${tid(sid('indoor'))}`)).toBeNull();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('도면 배경: 데이터가 없어도 floor_plan.image 가 있으면 배경을 렌더한다(빈상태 안내 대신)', () => {
    // 데이터 0개(seriesNames 비어 있음) + 편집/배치 아님. 도면 이미지만 설정됨.
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({}, { floor_plan: { image: 'data:image/png;base64,AAAA' } })}
      />,
    );
    // 배경 이미지가 렌더되고, 빈상태 안내는 표시되지 않는다.
    expect(screen.getByTestId('floor-plan-background')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.heatmap.emptyState')).toBeNull();
    // 히트맵 canvas 는 데이터가 없으므로 여전히 렌더되지 않는다.
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('forcePlacement: 드래그 이동이 sensor_positions 를 갱신한다', () => {
    const onConfigChange = vi.fn();
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={onConfigChange}
        forcePlacement
      />,
    );
    // 마커 핸들 드래그(pointerDown → move) → onPositionChange → sensor_positions 갱신.
    fireEvent.pointerDown(screen.getByTestId(`sensor-marker-handle-${tid(sid('s1'))}`));
    fireEvent.pointerMove(screen.getByTestId('sensor-placement-overlay'), {
      clientX: 10,
      clientY: 10,
    });
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        sensor_positions: expect.objectContaining({ [sid('s1')]: expect.any(Object) }),
      }),
    );
  });

  // -------------------------------------------------------------------------
  // 센서 동일성 재키잉(결함 수정) — 좌표/마커 매칭이 store key 나 사용자 편집 이름(alias)이
  // 아니라 시리즈 동일성 키를 따른다.
  // -------------------------------------------------------------------------

  /** 같은 key('dup')를 room 태그로 나눠 쓰는 형제 시리즈. */
  const DUP_A = { key: 'dup', metric_type: 'temperature', tags: { room: 'A' } };
  const DUP_B = { key: 'dup', metric_type: 'temperature', tags: { room: 'B' } };

  function storeConfig(
    series: Array<Record<string, unknown>>,
    sensorPositions: Record<string, { x: number; y: number }>,
    extras: Record<string, unknown> = {},
  ) {
    return {
      data_source: 'store',
      store_source: {
        agent_name: 'a',
        namespace: 'default',
        selection_mode: 'keys',
        series,
        time_window_ms: 1000,
        interval_ms: 1000,
        aggregation: 'last',
      },
      sensor_positions: sensorPositions,
      idw: { power: 2, grid_resolution: 8 },
      ...extras,
    } as Record<string, unknown>;
  }

  it('한 key 를 공유하는 형제 시리즈가 각자의 좌표로 독립 배치된다', () => {
    const idA = heatmapSensorId(DUP_A);
    const idB = heatmapSensorId(DUP_B);
    // 실제 훅은 히트맵이 넘긴 파생 alias(=동일성 키)로 시리즈를 키잉해 돌려준다.
    setStore({
      seriesNames: [idA, idB],
      seriesEntries: new Map([
        [idA, reading(20)],
        [idB, reading(26)],
      ]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={storeConfig(
          [
            { ...DUP_A, alias: 'dup' },
            { ...DUP_B, alias: 'dup' }, // 기본 alias 는 둘 다 key → 이름으로는 구분 불가.
          ],
          { [idA]: { x: 0.2, y: 0.2 }, [idB]: { x: 0.8, y: 0.8 } },
        )}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    // 두 형제가 각각의 마커를 갖는다(예전에는 좌표 한 칸을 공유해 하나로 합쳐졌다).
    expect(screen.getByTestId(`sensor-marker-${idA}`)).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${idB}`)).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
  });

  it('형제 중 하나가 선택 해제되어도 남은 형제는 배치를 유지하고 계속 렌더된다(보고된 결함)', () => {
    // A 는 해제되어 series 에서 빠졌고 좌표도 지워진 상태. B 는 그대로 체크 + 배치되어 있다.
    // 예전 key 키잉에서는 A 해제가 공유 항목을 지워 B 까지 미배치가 됐다(→ 아무것도 안 그려짐).
    //
    // 훅이 시리즈를 **표시 이름**('dup')으로 키잉해 돌려주는 상황을 일부러 흉내낸다. 좌표는
    // 동일성 키로 저장돼 있으므로, 이름으로 매칭했다면 어긋나 미배치가 된다. 패널은 컬럼 순서를
    // config.series 와 인덱스로 짝지어 동일성 키로 옮기므로 이름과 무관하게 결합된다.
    const idB = heatmapSensorId(DUP_B);
    setStore({
      seriesNames: ['dup'],
      seriesEntries: new Map([['dup', reading(26)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={storeConfig([{ ...DUP_B, alias: 'dup' }], { [idB]: { x: 0.8, y: 0.8 } })}
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-unplaced-hint')).toBeNull();
  });

  it('이름(alias) 변경은 배치를 유지한다 — 매칭이 표시 이름에 의존하지 않는다', () => {
    const id = heatmapSensorId(DUP_A);
    setStore({
      seriesNames: [id],
      seriesEntries: new Map([[id, reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        // alias 를 사용자가 '거실'로 바꾼 상태. 좌표는 동일성 키로 남아 있다.
        config={storeConfig([{ ...DUP_A, alias: '거실' }], { [id]: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    // 온도장 + 마커가 모두 유지되고,
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${id}`)).toBeInTheDocument();
    // 마커 라벨은 바뀐 이름을 보여준다(표시와 매칭의 분리).
    expect(screen.getByText('거실')).toBeInTheDocument();
  });

  it('진단성: 도면 배경이 있어도 미배치 센서 수를 노출한다(그림은 가리지 않음)', () => {
    // 판독값은 오는데 좌표가 없어 아무것도 그려지지 않는 상태 + 도면 배경.
    // 예전에는 showStack 분기 때문에 안내/카운트가 전혀 렌더되지 않아 원인 추적이 어려웠다.
    const id = heatmapSensorId(DUP_A);
    setStore({
      seriesNames: [id],
      seriesEntries: new Map([[id, reading(22)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={storeConfig([{ ...DUP_A, alias: 'dup' }], {}, {
          floor_plan: { image: 'data:image/png;base64,AAAA' },
        })}
      />,
    );
    // 배경은 그대로 렌더되고,
    expect(screen.getByTestId('floor-plan-background')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
    // 미배치 카운트가 보인다 — 가리지 않도록 pointer-events-none 인 작은 배지.
    const hint = screen.getByTestId('heatmap-unplaced-hint');
    expect(hint).toBeInTheDocument();
    expect(hint.className).toContain('pointer-events-none');
    expect(hint.className).toContain('absolute');
  });

  it('진단성: 일부만 배치된 경우에도(온도장이 그려지는 중) 미배치 카운트를 노출한다', () => {
    const idA = heatmapSensorId(DUP_A);
    const idB = heatmapSensorId(DUP_B);
    setStore({
      seriesNames: [idA, idB],
      seriesEntries: new Map([
        [idA, reading(20)],
        [idB, reading(26)],
      ]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={storeConfig(
          [
            { ...DUP_A, alias: 'A' },
            { ...DUP_B, alias: 'B' },
          ],
          { [idA]: { x: 0.2, y: 0.2 } }, // B 는 미배치.
        )}
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-unplaced-hint')).toBeInTheDocument();
  });

  it('하위호환: raw key 로 저장된 좌표는 렌더 시점에 동일성 키로 이관되어 그대로 그려진다', () => {
    const id = heatmapSensorId(DUP_A);
    setStore({
      seriesNames: [id],
      seriesEntries: new Map([[id, reading(22)]]),
      status: 'connected',
    });
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        // 옛 스키마: store key 로 키잉된 좌표.
        config={storeConfig([{ ...DUP_A, alias: 'dup' }], { dup: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-marker-${id}`)).toBeInTheDocument();
  });

  it('회귀 0: forcePlacement 미지정 대시보드 경로는 편집모드 게이팅을 유지한다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    // 편집모드 off + forcePlacement 미지정 → 오버레이/토글 모두 없음(기존 동작).
    useUIStore.getState().setDashboardEditMode(false);
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('sensor-placement-overlay')).toBeNull();
    expect(screen.queryByTestId('heatmap-edit-toggle')).toBeNull();
  });
});
