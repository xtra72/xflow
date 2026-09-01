// HeatmapPanel 견고성/렌더 분기 테스트 (SPEC-HEATMAP-PANEL-001 T6/T8/T9).
// AC-E1(빈 상태), AC-E2(미배치 안내), AC-E3(폴링 실패 시 마지막 렌더 유지 + 오류 배지),
// 정상 경로(배치 센서 → canvas 렌더)를 컴포넌트 레벨에서 커버한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useState } from 'react';

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
// SPEC-TSDB-002 M2 특성화용 호출 인자 기록기. 반환값은 종전과 동일하므로 기존 테스트의
// 동작은 바뀌지 않고, 히트맵이 **어떤 소스 객체로** 조회하는지만 관측 가능해진다.
const storeHookCalls: { args: Array<{ source: unknown; enabled: unknown }> } = { args: [] };
vi.mock('../charts/useStoreChartData', () => ({
  useStoreChartData: (source: unknown, enabled: unknown) => {
    storeHookCalls.args.push({ source, enabled });
    return storeMock.current;
  },
}));

// TSDB 조회 훅도 주입 가능한 mock 으로 대체한다 — 히트맵이 store 가 아닌 소스에 붙었을 때의
// 좌표 키 공간을 네트워크 없이 관측하기 위해서다(아래 "조회 키 공간" 회귀 테스트).
const tsdbMock: { current: UseStoreChartDataResult } = {
  current: {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  },
};
vi.mock('../charts/useTsdbChartData', () => ({
  useTsdbChartData: () => tsdbMock.current,
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import HeatmapPanel from './HeatmapPanel';
import { PanelChromeProvider } from '../../PanelChromeProvider';
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
  tsdbMock.current = {
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
    // s1 좌표만 삭제되고 s2 는 보존된다. 좌표를 쓰면 공간이 'stage' 로 승격된다(스테이지 도입).
    expect(onConfigChange).toHaveBeenCalledWith({
      sensor_space: 'stage',
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
  const DUP_A = { key: 'dup', field: 'temperature', tags: { room: 'A' } };
  const DUP_B = { key: 'dup', field: 'temperature', tags: { room: 'B' } };

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

  // -------------------------------------------------------------------------
  // 표시 결함: 좌표/매칭은 동일성으로 분리됐지만 라벨은 key 만 찍혀, 서로 다른 센서가
  // 한 좌표를 공유하는 것처럼 보였다. 마커/칩 텍스트가 시리즈를 실제로 구분해야 한다.
  // -------------------------------------------------------------------------

  it('한 key 를 공유하는 형제 센서의 마커 라벨이 서로 구분된다(보고된 표시 결함)', () => {
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
        // 생성 시 기본값 그대로(alias=key) — 예전에는 두 마커가 똑같이 'dup' 으로 찍혔다.
        config={storeConfig(
          [
            { ...DUP_A, alias: 'dup' },
            { ...DUP_B, alias: 'dup' },
          ],
          { [idA]: { x: 0.2, y: 0.2 }, [idB]: { x: 0.8, y: 0.8 } },
        )}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    expect(screen.getByText('dup · temperature{room=A}')).toBeInTheDocument();
    expect(screen.getByText('dup · temperature{room=B}')).toBeInTheDocument();
    // 구분 불가였던 옛 표기('dup' 단독)는 더 이상 나타나지 않는다.
    expect(screen.queryByText('dup')).toBeNull();
  });

  it('사용자가 이름을 바꾼 센서는 그 이름을, 나머지 형제는 서술 표기를 유지한다', () => {
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
            { ...DUP_A, alias: '거실' }, // 사용자가 붙인 이름.
            { ...DUP_B, alias: 'dup' }, // 생성 시 기본값.
          ],
          { [idA]: { x: 0.2, y: 0.2 }, [idB]: { x: 0.8, y: 0.8 } },
        )}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    expect(screen.getByText('거실')).toBeInTheDocument();
    expect(screen.getByText('dup · temperature{room=B}')).toBeInTheDocument();
  });

  it('미배치 팔레트 칩도 형제 센서를 구분해 보여준다', () => {
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
        // 좌표 없음 → 둘 다 미배치 팔레트 칩으로 나열된다.
        config={storeConfig(
          [
            { ...DUP_A, alias: 'dup' },
            { ...DUP_B, alias: 'dup' },
          ],
          {},
        )}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    const palette = screen.getByTestId('sensor-unplaced-palette');
    expect(palette).toHaveTextContent('dup · temperature{room=A}');
    expect(palette).toHaveTextContent('dup · temperature{room=B}');
  });

  it('metric/tags 가 없는 센서는 key 하나로 깔끔히 표시된다(구분자/후행 공백 없음)', () => {
    // storeSeriesId 는 이 경우 후행 공백이 남는 기계용 문자열을 만든다 — 라벨은 그걸 물려받지 않는다.
    const id = sid('plain');
    setStore({
      seriesNames: [id],
      seriesEntries: new Map([[id, reading(22)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={storeConfig([{ key: 'plain', alias: 'plain' }], { [id]: { x: 0.5, y: 0.5 } })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    const label = screen.getByText('plain');
    expect(label).toBeInTheDocument();
    expect(label.textContent).toBe('plain');
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

describe('HeatmapPanel — 스테이지(기준 도면 종횡비 박스)', () => {
  it('스택이 렌더되면 모든 레이어를 담는 스테이지가 존재한다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />,
    );
    const stage = screen.getByTestId('heatmap-stage');
    expect(stage).toBeInTheDocument();
    // 도면/히트맵/마커가 같은 좌표 공간을 공유하도록 스테이지 안에 들어간다.
    expect(stage.querySelector('[data-testid="heatmap-canvas"]')).not.toBeNull();
  });

  it('다중 도면 레이어를 모두 스테이지 안에 렌더한다', () => {
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig(
          {},
          {
            floor_plans: [
              { image: 'data:image/png;base64,AAAA' },
              { image: 'data:image/png;base64,BBBB', x: 0.5, y: 0.5, w: 0.5, h: 0.5 },
            ],
          },
        )}
      />,
    );
    const stage = screen.getByTestId('heatmap-stage');
    expect(stage.querySelector('[data-testid="floor-plan-background"]')).not.toBeNull();
    const overlay = stage.querySelector('[data-testid="floor-plan-layer-1"]') as HTMLElement | null;
    expect(overlay).not.toBeNull();
    expect(overlay!.style.left).toBe('50%');
    expect(overlay!.style.width).toBe('50%');
  });
});

// stage_fit: 레터박스 여백을 없애는 두 대안(잘림/왜곡). 좌표는 세 모드 모두 스테이지 정규화라
// 마커는 도면 위 같은 지점에 붙는다 — 달라지는 것은 스테이지 박스뿐이다.
describe('HeatmapPanel — stage_fit', () => {
  /** 본문 실측을 고정한다(jsdom 은 rect 가 0 이라 스테이지가 컨테이너 폴백을 탄다). */
  function withBodyRect(width: number, height: number) {
    return vi
      .spyOn(Element.prototype, 'getBoundingClientRect')
      .mockReturnValue({ width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0, toJSON: () => ({}) } as DOMRect);
  }

  /** 1:1 도면(200×200) — 400×200 본문과 종횡비가 어긋나 여백/잘림이 관찰된다. */
  const squarePlan = (stage_fit?: string) =>
    makeConfig(
      { s1: { x: 0.5, y: 0.5 } },
      {
        sensor_space: 'stage',
        floor_plans: [
          { image: 'data:image/png;base64,AAAA', natural_width: 200, natural_height: 200 },
        ],
        ...(stage_fit ? { stage_fit } : {}),
      },
    );

  beforeEach(() => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
  });

  it('기본(contain)은 도면 비율을 지키고 좌우 여백을 남긴다', () => {
    const spy = withBodyRect(400, 200);
    try {
      render(<HeatmapPanel panelId="p" config={squarePlan()} />);
      const stage = screen.getByTestId('heatmap-stage');
      expect(stage.style.width).toBe('200px');
      expect(stage.style.height).toBe('200px');
      expect(stage.style.left).toBe('100px');
    } finally {
      spy.mockRestore();
    }
  });

  it('cover 는 본문을 여백 없이 덮고 넘치는 쪽이 잘린다(음수 top)', () => {
    const spy = withBodyRect(400, 200);
    try {
      render(<HeatmapPanel panelId="p" config={squarePlan('cover')} />);
      const stage = screen.getByTestId('heatmap-stage');
      expect(stage.style.width).toBe('400px');
      expect(stage.style.height).toBe('400px');
      expect(stage.style.top).toBe('-100px');
    } finally {
      spy.mockRestore();
    }
  });

  it('stretch 는 본문을 그대로 쓰고 도면을 fill 로 늘린다', () => {
    const spy = withBodyRect(400, 200);
    try {
      render(<HeatmapPanel panelId="p" config={squarePlan('stretch')} />);
      const stage = screen.getByTestId('heatmap-stage');
      expect(stage.style.width).toBe('400px');
      expect(stage.style.height).toBe('200px');
      // 이미지가 자기 fit(contain)을 유지하면 이미지 안에서 여백이 되살아난다.
      const bg = screen.getByTestId('floor-plan-background') as HTMLImageElement;
      expect(bg.style.objectFit).toBe('fill');
    } finally {
      spy.mockRestore();
    }
  });

  it('cover 로 잘려도 본문이 잘라내므로 패널 밖으로 새지 않는다', () => {
    const spy = withBodyRect(400, 200);
    try {
      const { container } = render(<HeatmapPanel panelId="p" config={squarePlan('cover')} />);
      const body = container.querySelector('[data-testid="heatmap-stage"]')!.parentElement!;
      expect(body.className).toContain('overflow-hidden');
    } finally {
      spy.mockRestore();
    }
  });
});

// 타이틀 바: 히트맵은 지금까지 제목을 렌더하지 않은 유일한 패널이었다. 다른 패널과 동형의
// 헤더를 신설하고 공통 옵션(showTitle)으로 끌 수 있다.
describe('HeatmapPanel — 타이틀 바', () => {
  beforeEach(() => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
  });

  it('제목이 있으면 타이틀 바를 그린다(기본 표시)', () => {
    render(
      <HeatmapPanel panelId="p" title="1층 온도" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />,
    );
    expect(screen.getByTestId('heatmap-title').textContent).toContain('1층 온도');
  });

  it('showTitle=false 면 타이틀 바를 그리지 않는다', () => {
    render(
      <PanelChromeProvider config={{ showTitle: false }}>
        <HeatmapPanel panelId="p" title="1층 온도" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />
      </PanelChromeProvider>,
    );
    expect(screen.queryByTestId('heatmap-title')).toBeNull();
  });

  it('제목이 비어 있으면 표시 옵션과 무관하게 그리지 않는다(도면 영역을 잠식하지 않음)', () => {
    render(<HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />);
    expect(screen.queryByTestId('heatmap-title')).toBeNull();
  });

  it('배치 편집 토글은 타이틀 바가 있으면 그 아래로 내려간다(제목 가림 방지)', () => {
    useUIStore.getState().setDashboardEditMode(true);
    try {
      const { rerender } = render(
        <HeatmapPanel
          panelId="p"
          title="1층 온도"
          config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.getByTestId('heatmap-edit-toggle').className).toContain('top-9');

      // 제목이 없으면 헤더가 없으므로 원래 위치(top-2)를 그대로 쓴다.
      rerender(
        <HeatmapPanel
          panelId="p"
          config={makeConfig({ s1: { x: 0.5, y: 0.5 } })}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.getByTestId('heatmap-edit-toggle').className).toContain('top-2');
    } finally {
      useUIStore.getState().setDashboardEditMode(false);
    }
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `HeatmapPanel.tsx:122` 는 다른 5종 패널과 **형태만 같고 읽는 대상이 다르다.**
// `config.store_source` 를 직접 보지 않고, 렌더 시점 `useMemo` 로 파생한
// `heatmapStoreSource`(체크된 시리즈만 + `selection_mode:'keys'` 강제 +
// `tag_filters` 제거 + alias 를 센서 동일성 키로 치환)의 시리즈 길이를 본다.
//
// 이 사실을 테스트가 알게 해 두지 않으면 M3 의 기계적 치환이 히트맵을 조용히
// 비운다(또는 tag 모드 히트맵을 새로 활성화한다).
//
// @spec SPEC-TSDB-002 §2.3 (U3) — plan.md §3.2 CT-06 ~ CT-08 / AC-10
// ---------------------------------------------------------------------------
describe('HeatmapPanel 소스 활성 판정 특성화 (SPEC-TSDB-002 M2, CT-06~CT-08)', () => {
  function lastStoreCall(): { source: unknown; enabled: unknown } {
    return storeHookCalls.args.at(-1)!;
  }

  beforeEach(() => {
    storeHookCalls.args = [];
  });

  it('CT-06: store 모드 + 히트맵 시리즈 N개면 **파생 소스**로 store 경로를 탄다', () => {
    const positions = { 'room:1:temp': { x: 0.2, y: 0.3 } };
    setStore({
      seriesNames: [sid('room:1:temp')],
      seriesEntries: new Map([[sid('room:1:temp'), reading(22)]]),
      status: 'connected',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig(positions)} />);

    const call = lastStoreCall();
    expect(call.enabled).toBe(true);
    const source = call.source as {
      selection_mode?: string;
      tag_filters?: unknown;
      series: Array<{ key: string; alias?: string }>;
    };
    // 전달된 것은 config.store_source 가 아니라 파생 소스다 —
    // selection_mode 는 'keys' 로 강제되고 tag_filters 는 제거되며 alias 가 치환된다.
    expect(source.selection_mode).toBe('keys');
    expect(source.tag_filters).toBeUndefined();
    expect(source.series).toHaveLength(1);
    expect(source.series[0]!.alias).toBe(sid('room:1:temp'));
  });

  it('CT-07: store 모드 + 히트맵 시리즈 0개면 비활성이다', () => {
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(<HeatmapPanel panelId="p" config={makeConfig({}, {}, [])} />);

    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
  });

  it('CT-08: store_source 는 있으나 히트맵이 읽는 키가 비면 비활성이다(tag 모드는 활성화하지 않는다)', () => {
    // 이 config 는 spec.md §2.3 의 **일반 store 활성 조건**(tag 모드 + tag_filters 1개
    // 이상)을 만족한다. 그럼에도 히트맵은 비활성이다 — 히트맵은 `store_source` 를 직접
    // 읽지 않고, `series` 에서 파생한 키 목록만 보기 때문이다.
    // M3 에서 이 지점을 `resolvePanelSourceBinding(config).active` 로 그대로 바꾸면
    // 이 테스트가 RED 가 된다(= tag 모드 히트맵이 새로 활성화되는 동작 변경).
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    const config = {
      data_source: 'store',
      store_source: {
        agent_name: 'a',
        namespace: 'default',
        selection_mode: 'tag',
        tag_filters: { room: '1' },
        series: [],
        time_window_ms: 1000,
        interval_ms: 1000,
        aggregation: 'last',
      },
      sensor_positions: {},
      idw: { power: 2, grid_resolution: 8 },
    } as Record<string, unknown>;

    render(<HeatmapPanel panelId="p" config={config} />);

    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });
  // -------------------------------------------------------------------------
  // 조회 키 공간과 마커 화이트리스트의 일치 (결함: "포인트가 사라져 한 번 이동 후 이동 불가")
  //
  // 팔레트 칩 / 좌표 쓰기 키는 **조회 결과 키 공간**(resolveSensorSeries 의 ids)인데,
  // 마커 화이트리스트를 `store_source.series` 파생 키로만 두면 두 공간이 어긋나는 경로에서
  // 방금 배치한 좌표가 그 자리에서 사라진다 — 좌표가 생겨 칩은 없어지고 마커는 걸러진다.
  // -------------------------------------------------------------------------

  /** config 를 shallow merge 로 보관하는 부모(대시보드 `updatePanelConfig` 와 같은 규칙). */
  function PlacementHost({ initial }: { initial: Record<string, unknown> }) {
    const [config, setConfig] = useState(initial);
    return (
      <HeatmapPanel
        panelId="p"
        config={config}
        onConfigChange={(c) => setConfig((prev) => ({ ...prev, ...c }))}
        forcePlacement
      />
    );
  }

  /** 오버레이 위에서 칩/마커를 한 번 끌어 놓는다. */
  function dropOnOverlay(handle: HTMLElement, clientX: number, clientY: number) {
    const overlay = screen.getByTestId('sensor-placement-overlay');
    fireEvent.pointerDown(handle);
    fireEvent.pointerMove(overlay, { clientX, clientY });
    fireEvent.pointerUp(overlay, { clientX, clientY });
  }

  it('TSDB 소스: 팔레트 칩을 끌어 놓으면 그 자리에 마커가 남는다', () => {
    // TSDB 히트맵에는 `store_source` 가 없다 → refs 가 비어 화이트리스트도 비었었다.
    tsdbMock.current = {
      ...tsdbMock.current,
      seriesNames: ['temp.A'],
      seriesEntries: new Map([['temp.A', reading(21)]]),
      status: 'connected',
    };
    render(
      <PlacementHost
        initial={{
          data_source: 'tsdb',
          tsdb_source: {
            agent_name: 'influx',
            series: [{ measurement: 'm', field: 'temperature' }],
          },
          sensor_positions: {},
          idw: { power: 2, grid_resolution: 8 },
        }}
      />,
    );

    dropOnOverlay(screen.getByTestId('sensor-unplaced-temp.A'), 10, 10);

    // 칩은 좌표를 얻어 사라지고, 그 자리를 마커가 이어받는다(둘 다 없으면 결함).
    expect(screen.queryByTestId('sensor-unplaced-temp.A')).toBeNull();
    expect(screen.getByTestId('sensor-marker-handle-temp.A')).toBeInTheDocument();
  });

  it('store 소스: 컬럼 수가 series 수와 어긋나도 배치한 마커가 남는다', () => {
    // 한 key 가 여러 컬럼으로 펼쳐지면 동일성 키로 정렬할 수 없어(resolveSensorSeries
    // aligned=false) 조회 이름이 그대로 키가 된다 — 그 키도 마커로 인정해야 한다.
    setStore({
      seriesNames: ['s1 (1)', 's1 (2)'],
      seriesEntries: new Map([
        ['s1 (1)', reading(22)],
        ['s1 (2)', reading(23)],
      ]),
      status: 'connected',
    });
    render(<PlacementHost initial={makeConfig({}, {}, ['s1'])} />);

    dropOnOverlay(screen.getByTestId('sensor-unplaced-s1 (1)'), 10, 10);

    expect(screen.queryByTestId('sensor-unplaced-s1 (1)')).toBeNull();
    expect(screen.getByTestId('sensor-marker-handle-s1 (1)')).toBeInTheDocument();
  });

  it('배치한 마커를 연속으로 두 번 이동할 수 있다', () => {
    setStore({
      seriesNames: [sid('s1')],
      seriesEntries: new Map([[sid('s1'), reading(22)]]),
      status: 'connected',
    });
    render(<PlacementHost initial={makeConfig({ [sid('s1')]: { x: 0.5, y: 0.5 } })} />);

    const handleId = `sensor-marker-handle-${tid(sid('s1'))}`;
    dropOnOverlay(screen.getByTestId(handleId), 10, 10);
    expect(screen.getByTestId(handleId)).toBeInTheDocument();
    dropOnOverlay(screen.getByTestId(handleId), 30, 30);
    expect(screen.getByTestId(handleId)).toBeInTheDocument();
  });
  it('판독값이 아직 없어도 바인딩된 센서는 미배치 팔레트에 뜬다(자리 정하기는 값보다 먼저다)', () => {
    // 조회는 붙었지만 컬럼/값이 하나도 없는 상태(설정 미리보기에서 실제 데이터를 끈 경우 포함).
    // 종전에는 팔레트가 통째로 비어 배치 자체가 불가능했다.
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({}, {}, ['s1', 's2'])}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    expect(screen.getByTestId(`sensor-unplaced-${tid(sid('s1'))}`)).toBeInTheDocument();
    expect(screen.getByTestId(`sensor-unplaced-${tid(sid('s2'))}`)).toBeInTheDocument();
  });

  it('마커 오버레이가 도면 이동 오버레이보다 위에 온다(마커를 잡으면 도면이 아니라 마커가 움직이도록)', () => {
    setStore({
      seriesNames: [sid('s1')],
      seriesEntries: new Map([[sid('s1'), reading(22)]]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ [sid('s1')]: { x: 0.5, y: 0.5 } }, {
          floor_plan: { image: 'data:image/png;base64,AAAA' },
        })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    const zOf = (el: HTMLElement): number => {
      const m = /z-\[?(\d+)\]?/.exec(el.className);
      return m ? Number(m[1]) : NaN;
    };
    const sensorZ = zOf(screen.getByTestId('sensor-placement-overlay'));
    const transformZ = zOf(screen.getByTestId('floorplan-transform-overlay'));
    expect(sensorZ).toBeGreaterThan(transformZ);
  });
});
