// PanelSettingsDialog — 3분할 셸(3-split shell) 구성 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T1 / REQ-01 / AC-01)
//
// 검증: Store 바인딩 패널(heatmap / line-chart) 설정 셸이 미리보기·옵션·데이터소스
// 3영역을 모두 렌더하고, 기존 편집 슬롯(패널 옵션/데이터소스)이 각 영역에 손실 없이
// 배치된다(셸 골격 교체 + 슬롯 이관, 편집 로직 보존). 배치 기본값:
//   좌측 컬럼 상단 = 미리보기 / 좌측 컬럼 하단 = 데이터소스 / 우측 = 옵션.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: '패널', config: {} } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [{ id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] }],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

vi.mock('./panels/charts/useStoreChartData', () => ({
  useStoreChartData: () => ({
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: [],
    seriesNames: [],
    booleanSeries: [],
    status: 'idle',
  }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] } }),
  // PanelStoreSelectTable(공용 시리즈 선택 테이블)이 상세 조회에 사용한다.
  // @spec SPEC-CHART-002 M2 — Store 모드 config 로 렌더하는 특성화 테스트에 필요.
  useAgent: () => ({ data: undefined }),
}));

// GaugeSection(게이지 설정 섹션)은 플로우 목록을 React Query 로 조회한다.
// QueryClient 없이 렌더하기 위해 빈 결과로 모킹한다.
// @spec SPEC-CHART-002 M2 — CH-19(gauge 기준선) 렌더에 필요.
// chart-emitter 채널 목록(@/services/api/charts)은 조회 실패 시 빈 목록을 유지하는
// 경로가 이미 있어 모킹하지 않는다(모킹하면 기존 테스트에 act 경고가 생긴다).
vi.mock('@/hooks/useFlow', () => ({
  useFlows: () => ({ data: { data: [] } }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  window.localStorage.clear();
});

describe('PanelSettingsDialog 3분할 셸 (AC-01)', () => {
  it('heatmap 패널: 미리보기·옵션·데이터소스 3영역이 모두 존재한다', () => {
    storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('panel-settings-preview')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-options')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
  });

  it('line-chart 패널: 3영역이 모두 존재하고 편집 슬롯이 각 영역에 배치된다', () => {
    storeMock.panel = { id: 'p1', type: 'graph-chart', title: '라인', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const preview = screen.getByTestId('panel-settings-preview');
    const options = screen.getByTestId('panel-settings-options');
    const dataSource = screen.getByTestId('panel-settings-data-source');
    expect(preview).toBeInTheDocument();
    expect(options).toBeInTheDocument();
    expect(dataSource).toBeInTheDocument();

    // 옵션 영역에 기존 "패널 옵션" 편집 슬롯이 이관되어 있다(편집 기능 손실 없음).
    expect(within(options).getByText('dashboard.settings.panelOptions')).toBeInTheDocument();
    // 데이터소스 영역은 좌측 컬럼(미리보기 영역) 안에 위치한다(좌측 상단 미리보기 / 하단 데이터소스).
    expect(preview).toContainElement(dataSource);
  });

  it('기본 배치: 미리보기·데이터소스는 좌측 컬럼(order-1), 옵션은 우측(order-3)', () => {
    storeMock.panel = { id: 'p1', type: 'graph-chart', title: '라인', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 좌측 컬럼(미리보기+데이터소스)은 order-1, 우측 옵션 컬럼은 order-3 클래스를 갖는다.
    expect(screen.getByTestId('panel-settings-preview').className).toContain('order-1');
    expect(screen.getByTestId('panel-settings-options').className).toContain('order-3');
  });
});

describe('미리보기 채움/맞춤 토글 (fill/fit)', () => {
  it('툴바에 토글이 있고 기본값은 fill(종횡비 없음)이다', () => {
    storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    const toggle = screen.getByTestId('panel-settings-preview-fill-toggle');
    expect(toggle).toBeInTheDocument();
    // 기본 fill → aria-label 은 fill 모드(클릭 시 fit). 미리보기 wrapper 는 종횡비 style 이 없다.
    expect(toggle).toHaveAttribute('aria-label', 'dashboard.settings.previewModeFillAria');
    const preview = screen.getByTestId('panel-settings-preview');
    expect(preview.querySelector('[style*="aspect-ratio"]')).toBeNull();
  });

  it('클릭 시 fit 으로 전환되고 localStorage 에 영속되며 wrapper 가 종횡비를 갖는다', () => {
    storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    fireEvent.click(screen.getByTestId('panel-settings-preview-fill-toggle'));
    // fit 모드로 전환 → aria-label 변경 + localStorage 영속.
    expect(screen.getByTestId('panel-settings-preview-fill-toggle')).toHaveAttribute(
      'aria-label',
      'dashboard.settings.previewModeFitAria',
    );
    expect(window.localStorage.getItem('panelSettings.previewFillMode')).toBe('fit');
    // fit 모드 → 미리보기 wrapper 가 종횡비(aspect-ratio) style 을 갖는다(fill 과 다름).
    const preview = screen.getByTestId('panel-settings-preview');
    expect(preview.querySelector('[style*="aspect-ratio"]')).not.toBeNull();
  });

  it('영속된 fit 모드를 재마운트 시 복원한다', () => {
    window.localStorage.setItem('panelSettings.previewFillMode', 'fit');
    storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-settings-preview-fill-toggle')).toHaveAttribute(
      'aria-label',
      'dashboard.settings.previewModeFitAria',
    );
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 설정 화면 특성화 (DDD PRESERVE).
//
// **[M4 에서 CH-19 를 의도적으로 반전했다 — 삭제하지 않고 기대값만 뒤집었다]**
//
// CH-19 는 원래 "gauge 는 데이터소스 섹션을 렌더하지 **않는다**" 는 변경 전 기준선이었다.
// M4.1 이 `dataSourceBelowPreview` 조건에 `panel.type === 'gauge'` 를 추가하면서
// (spec.md §2.3 [U3] — 게이지도 라인 차트와 동일한 Store 소스 선택 surface 를 갖는다)
// 그 기준선은 설계상 거짓이 되었다. 이는 하위 호환 위반이 아니라 이 SPEC 이 명시적으로
// 요구한 변경이며, CH-19 는 본 SPEC 전체에서 반전이 허용된 **유일한** 특성화다
// (plan.md §3 각주). CH-01~CH-18 · CH-20 은 그대로 GREEN 을 유지해야 한다.
//
// 반전된 기대값은 AC-08(공용 데이터 소스 surface 렌더)과 같은 사실을 가리킨다.
// ---------------------------------------------------------------------------

/** 대표값 선택기가 붙을 자리를 식별하는 후보(테스트 ID + i18n 라벨 키). */
const REDUCE_SELECTOR_TESTID = 'chart-series-reduce';
const REDUCE_LABEL_KEY = 'dashboard.chart.seriesReduce';

/** Store 모드 config — 대표값 선택기가 노출될 수 있는 유일한 조건(spec.md §2.10). */
const STORE_CONFIG = {
  data_source: 'store',
  store_source: {
    agent_id: 'store-uuid-1',
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'keys',
    series: [{ key: 'k1' }],
    time_window_ms: 3_600_000,
    interval_ms: 60_000,
    aggregation: 'average',
  },
};

describe('PanelSettingsDialog 특성화 (SPEC-CHART-002 M2)', () => {
  it('CH-19 [M4 반전]: gauge 는 공용 데이터소스 섹션을 렌더한다', async () => {
    // 반전 전(M2 기준선): dataSourceBelowPreview =
    //   isChartPanel(CHART_PANEL_TYPES) || panel.type === 'heatmap'
    // 이라 gauge 가 두 조건 어디에도 걸리지 않아 섹션이 없었다(spec.md §1.2.1).
    // 반전 후(M4.1): 조건에 `|| panel.type === 'gauge'` 가 추가되어 섹션이 존재한다.
    // `CHART_PANEL_TYPES` 자체는 불변이다(UB1-9) — heatmap 과 같은 노출 방식이다.
    storeMock.panel = { id: 'p1', type: 'gauge', title: '게이지', config: {} };
    // GaugeSection 이 마운트 시 채널 목록을 비동기 조회하므로 act 로 감싸 flush 한다.
    await act(async () => {
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    });

    expect(screen.getByTestId('panel-settings-preview')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-options')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
  });

  it('반원 RB 를 고르면 반원 방향 선택이 나온다', async () => {
    storeMock.panel = {
      id: 'p1',
      type: 'gauge',
      title: '게이지',
      config: { gaugeType: 'half-rainbow' },
    };
    await act(async () => {
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    });
    for (const dir of ['up', 'down', 'left', 'right']) {
      expect(screen.getByTestId(`gauge-half-rainbow-direction-${dir}`)).toBeInTheDocument();
    }
    // 미지정이면 위쪽이 눌린 상태다.
    expect(
      screen.getByTestId('gauge-half-rainbow-direction-up').getAttribute('aria-pressed'),
    ).toBe('true');
  });

  it('다른 게이지 타입에서는 방향 선택을 내린다 — 효과 없는 칸을 두지 않는다', async () => {
    storeMock.panel = { id: 'p1', type: 'gauge', title: '게이지', config: { gaugeType: 'half' } };
    await act(async () => {
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    });
    expect(screen.queryByTestId('gauge-half-rainbow-direction-up')).toBeNull();
  });

  it('방향을 고르면 config 에 저장된다', async () => {
    storeMock.panel = {
      id: 'p1',
      type: 'gauge',
      title: '게이지',
      config: { gaugeType: 'half-rainbow' },
    };
    await act(async () => {
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    });
    fireEvent.click(screen.getByTestId('gauge-half-rainbow-direction-left'));
    expect(
      screen.getByTestId('gauge-half-rainbow-direction-left').getAttribute('aria-pressed'),
    ).toBe('true');
  });

  it('CH-19 [M4 반전]: gauge 는 Store 모드에서 데이터소스 섹션 + 대표값 선택기를 갖는다', async () => {
    // 반전 전: 설정이 저장될 수는 있어도(불투명 JSON) 편집 surface 가 없었다.
    // 반전 후: 섹션이 렌더되고, gauge 가 REDUCE_PANEL_TYPES 에 속하므로 Store 모드에서
    // 대표값 선택기까지 노출된다(spec.md §2.3 / AC-08 · AC-09).
    storeMock.panel = { id: 'p1', type: 'gauge', title: '게이지', config: { ...STORE_CONFIG } };
    await act(async () => {
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    });

    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
    expect(screen.getByTestId(REDUCE_SELECTOR_TESTID)).toBeInTheDocument();
    expect(screen.getByText(REDUCE_LABEL_KEY)).toBeInTheDocument();
  });

  it('CH-20: line-chart / table / heatmap 은 Store 모드에서도 대표값 선택기를 갖지 않는다', async () => {
    // spec.md §2.3 / UB1-10: REDUCE_PANEL_TYPES 는 stat/gauge/bar-chart/pie-chart 뿐이며
    // 이 3종은 같은 StoreSourceSection 을 써도 선택기가 노출되지 않아야 한다.
    for (const type of ['graph-chart', 'table', 'heatmap'] as const) {
      storeMock.panel = { id: 'p1', type, title: type, config: { ...STORE_CONFIG } };
      // 공용 시리즈 선택 테이블이 마운트 시 비동기 상태를 갱신하므로 flush 한다.
      let view!: ReturnType<typeof render>;
      await act(async () => {
        view = render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
      });

      // 데이터소스 섹션 자체는 존재한다(= 선택기가 있었다면 여기 보였을 것이다).
      expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
      expect(screen.queryByTestId(REDUCE_SELECTOR_TESTID)).toBeNull();
      expect(screen.queryByText(REDUCE_LABEL_KEY)).toBeNull();

      view.unmount();
    }
  });
});
