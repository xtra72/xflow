// PanelSettingsDialog 히트맵 설정 화면 테스트.
//
// 두 가지 회귀 방지를 검증한다:
//   (a) 히트맵 패널 설정 시 미리보기 영역에 실제 HeatmapPanel 이 렌더되고, forcePlacement 로
//       배치 오버레이가 활성화된다(설정 미리보기에서 센서 드래그 배치 가능).
//   (b) 데이터 소스 섹션(panel-settings-data-source)이 차트 패널과 동일하게 프리뷰 아래에 노출된다.
//
// store 폴링(useStoreChartData)과 StoreSourceSection 의 네트워크 훅을 정적 값으로 대체해
// QueryClientProvider 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: '히트맵', config: {} } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
  // 센서 좌표 목록이 라이브 시리즈만 반영하는지 검증하기 위한 주입 가능한 seriesNames.
  seriesNames: [] as string[],
  // 그리드 기하 — 미리보기 실비율(A)과 "도면 비율에 맞추기"(B)가 읽는다.
  layout: [] as { i: string; x: number; y: number; w: number; h: number }[],
  gridCols: 10,
  // 10칼럼·마진 16 에서 셀 100px 이 되는 폭(1000 + 16×9).
  gridWidth: 1144,
  setDashboardLayout: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [
      { id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: storeMock.layout },
    ],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
    // HeatmapPanel 이 배치편집 토글 게이팅에 참조한다(false = 뷰어).
    dashboardEditMode: false,
    dashboardGridCols: storeMock.gridCols,
    dashboardGridWidth: storeMock.gridWidth,
    setDashboardLayout: storeMock.setDashboardLayout,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// 도면 자산 API — 업로드는 고정 id, 조회는 고정 data-URL 로 대체(네트워크 없이 렌더).
const assetMock = vi.hoisted(() => ({
  upload: vi.fn(async () => ({ id: 'asset-1', mime: 'image/png', size: 10 })),
  fetchUrl: vi.fn(async () => 'data:image/png;base64,FROMASSET'),
}));
vi.mock('@/services/api/dashboardAssetService', () => ({
  uploadDashboardAsset: assetMock.upload,
  fetchDashboardAssetUrl: assetMock.fetchUrl,
}));

// store 폴링 훅(react-query 의존) — HeatmapPanel + HeatmapSettingsSection 공용. idle 정적 값으로 대체.
vi.mock('./panels/charts/useStoreChartData', () => ({
  useStoreChartData: () => ({
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: [],
    seriesNames: storeMock.seriesNames,
    booleanSeries: [],
    status: 'idle',
  }),
}));

// StoreSourceSection 의 네트워크 훅(에이전트/키 목록) — 정적 값으로 대체.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] } }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));
vi.mock('@/services/api/store', () => ({
  // 선택 테이블 행은 keyObjects 에서 파생된다. v0.4.0: 좌표 편집이 행 인라인 펼침 안에 있으므로
  // 선택된 시리즈 키가 목록 행으로 표시되어야 펼침·좌표 편집이 가능하다.
  useStoreKeysWithTags: () => ({
    data: {
      // metric_type/tags 는 config 의 series({key})와 seriesId 가 일치하도록 비운다.
      keyObjects: storeMock.seriesNames.map((key) => ({
        key,
        registration: 'auto',
        data_type: 'float',
        tags: {},
      })),
    },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.setDashboardLayout.mockReset();
  storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
  storeMock.seriesNames = [];
  storeMock.layout = [];
  storeMock.gridCols = 10;
  storeMock.gridWidth = 1144;
  window.localStorage.clear();
});

describe('PanelSettingsDialog 히트맵 설정 화면', () => {
  it('미리보기에 HeatmapPanel 을 렌더하고 forcePlacement 로 배치 오버레이를 활성화한다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 프리뷰 HeatmapPanel 이 forcePlacement 로 배치 오버레이를 켠다(대시보드 편집모드 false 이지만).
    // 오버레이가 있으면 실제 HeatmapPanel 이 프리뷰에 렌더되고 드래그 배치가 가능한 것이다.
    expect(screen.getByTestId('sensor-placement-overlay')).toBeInTheDocument();
    // 설정 미리보기에서는 편집 토글 버튼을 노출하지 않는다(항상 배치 활성).
    expect(screen.queryByTestId('heatmap-edit-toggle')).toBeNull();
  });

  it('도면 배경 이미지가 config 에 있으면 데이터 0개여도 프리뷰에 즉시 배경을 렌더한다', () => {
    // 시각 설정(floor_plan.image)은 debounce 없이 즉시 draft config 로 렌더돼야 한다.
    // series/데이터가 전혀 없어도(zero data) forcePlacement→showStack 으로 배경이 표시된다.
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: {
        floor_plan: { image: 'data:image/png;base64,AAAA', fit: 'contain' },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const bg = screen.getByTestId('floor-plan-background') as HTMLImageElement;
    expect(bg).toBeInTheDocument();
    expect(bg.getAttribute('src')).toBe('data:image/png;base64,AAAA');
    // 구 단일 floor_plan 은 스테이지를 가득 채우는 기준 레이어로 이관된다(0%,0%,100%,100%).
    // 0-height 붕괴 방지의 책임은 이제 스테이지 박스가 진다.
    expect(bg.style.left).toBe('0%');
    expect(bg.style.top).toBe('0%');
    expect(bg.style.width).toBe('100%');
    expect(bg.style.height).toBe('100%');

    // 프리뷰 wrapper 는 flex 컨테이너여야 HeatmapPanel(flex-1)이 높이를 채운다.
    // (plain block 이면 flex-1 no-op → 데이터 0개일 때 0-height 붕괴 → 배경 안 보임.)
    const wrapper = screen.getByTestId('heatmap-preview-wrapper');
    expect(wrapper.className).toContain('flex');
    expect(wrapper.className).toContain('flex-col');
    expect(wrapper.className).toContain('min-h-0');
  });

  it('데이터 소스 섹션을 프리뷰 아래에 노출한다(차트 패널과 동일)', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
  });

  it('센서 좌표 목록은 시리즈 리스트만 반영하고 잔존 좌표는 표시하지 않는다', () => {
    // 라이브 시리즈: s1(좌표 있음), s2(좌표 없음). leftover 는 좌표만 남고 시리즈엔 없음.
    storeMock.seriesNames = ['s1', 's2'];
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: {
        data_source: 'store',
        store_source: {
          agent_id: 'store-uuid-1',
          agent_name: 'store-1',
          series: [{ key: 's1' }, { key: 's2' }],
          time_window_ms: 60000,
          interval_ms: 5000,
          aggregation: 'last',
        },
        sensor_positions: {
          s1: { x: 0.5, y: 0.5 },
          leftover: { x: 0.1, y: 0.2 },
        },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // v0.4.0: 좌표는 선택 행 인라인 펼침 안에서 편집한다. 선택된 s1·s2 행을 펼친다.
    const expandButtons = screen.getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel');
    expandButtons.forEach((btn) => fireEvent.click(btn));

    // 선택 시리즈 s1(좌표 있음)·s2(좌표 없음, 미배치)는 펼침 상세에 x 입력이 노출된다.
    expect(screen.getByTestId('heatmap-pos-x-s1')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-pos-y-s1')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-pos-x-s2')).toBeInTheDocument();
    // 시리즈 리스트에 없는 잔존 좌표(leftover)는 선택/펼침 대상이 아니므로 나타나지 않는다.
    expect(screen.queryByTestId('heatmap-pos-x-leftover')).toBeNull();
    expect(screen.queryByTestId('heatmap-pos-y-leftover')).toBeNull();
  });
});

describe('도면 이미지 — 자산 분리 저장', () => {
  it('인라인 이미지가 있으면 경고와 이관 버튼을 노출한다(이 상태에선 대시보드 저장이 실패한다)', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: { floor_plan: { image: 'data:image/png;base64,LEGACY' } },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('heatmap-floorplan-inline-warning')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-floorplan-migrate')).toBeInTheDocument();
  });

  it('이관 버튼은 인라인 이미지를 자산으로 올리고 config 에서 data-URL 을 제거한다', async () => {
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: { floor_plan: { image: 'data:image/png;base64,LEGACY' } },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    fireEvent.click(screen.getByTestId('heatmap-floorplan-migrate'));

    await vi.waitFor(() => expect(assetMock.upload).toHaveBeenCalledWith('data:image/png;base64,LEGACY'));
    // 설정은 draft 에 반영된다(저장 버튼으로 승격). 관측 가능한 결과로 확인한다:
    // 인라인 경고가 사라지고, 썸네일이 자산에서 온 이미지로 바뀐다.
    await vi.waitFor(() =>
      expect(screen.queryByTestId('heatmap-floorplan-inline-warning')).toBeNull(),
    );
    await vi.waitFor(() => {
      const thumb = screen.getByTestId('heatmap-floorplan-preview') as HTMLImageElement;
      expect(thumb.getAttribute('src')).toBe('data:image/png;base64,FROMASSET');
    });
  });

  it('자산 id 로 저장된 레이어는 인라인 경고 없이 조회된 이미지를 썸네일로 보인다', async () => {
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: { floor_plans: [{ asset_id: 'asset-1' }] },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('heatmap-floorplan-inline-warning')).toBeNull();
    await vi.waitFor(() => {
      const thumb = screen.getByTestId('heatmap-floorplan-preview') as HTMLImageElement;
      expect(thumb.getAttribute('src')).toBe('data:image/png;base64,FROMASSET');
    });
  });
});

// 히트맵 스테이지는 도면 종횡비로 레터박스되므로(stage.ts), 미리보기가 실제 패널 비율이 아니면
// "여백이 얼마나 생기는지"를 확인할 방법이 없다. fit 모드가 대시보드 레이아웃에서 파생한 실비율을
// 쓰는지 검증한다(jsdom 은 fit 컨테이너 실측이 0 이라 CSS aspectRatio 폴백 경로를 탄다).
describe('미리보기 — 패널 실제 종횡비', () => {
  const useFitMode = () => window.localStorage.setItem('panelSettings.previewFillMode', 'fit');

  it('fit 모드에서 대시보드 레이아웃의 실제 픽셀 비율을 쓴다(마진 포함)', () => {
    useFitMode();
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 셀 100 + 마진 16 → 폭 6·100+16·5=680, 높이 4·100+16·3=448. 단위 비 1.5 가 아니다.
    const wrapper = screen.getByTestId('heatmap-preview-wrapper');
    expect(wrapper.style.aspectRatio).toBe(`${680 / 448} / 1`);
  });

  it('레이아웃 항목이 없으면 기존 3:2 로 폴백한다', () => {
    useFitMode();
    storeMock.layout = [];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('heatmap-preview-wrapper').style.aspectRatio).toBe('3 / 2');
  });

  it('fill 모드는 종전대로 영역을 가득 채운다(비율 미적용)', () => {
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const wrapper = screen.getByTestId('heatmap-preview-wrapper');
    expect(wrapper.style.width).toBe('100%');
    expect(wrapper.style.height).toBe('100%');
  });
});

// 여백의 원인은 패널 비율 ≠ 도면 비율이다. 버튼은 패널 높이를 도면 비율에 맞춰 원인을 없앤다.
describe('도면 비율에 맞추기', () => {
  // 800×600(4:3) 도면. natural_* 가 있으면 이미지 로드 없이 즉시 종횡비가 정해진다.
  const planPanel = () =>
    ({
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: {
        floor_plans: [
          { asset_id: 'asset-1', x: 0, y: 0, w: 1, h: 1, opacity: 1, fit: 'contain', natural_width: 800, natural_height: 600 },
        ],
      },
    }) as unknown as PanelConfig;

  it('패널 높이를 도면 종횡비에 맞춰 레이아웃을 갱신한다', () => {
    storeMock.panel = planPanel();
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.click(screen.getByTestId('heatmap-match-ratio'));

    // 폭 px = 680, 목표 높이 px = 680 / (4/3) = 510 → h = (510+16)/(100+16) ≈ 4.53 → 5.
    expect(storeMock.setDashboardLayout).toHaveBeenCalledWith([
      { i: 'p1', x: 0, y: 0, w: 6, h: 5 },
    ]);
  });

  it('다른 패널의 레이아웃 항목은 건드리지 않는다', () => {
    storeMock.panel = planPanel();
    storeMock.layout = [
      { i: 'p0', x: 0, y: 0, w: 2, h: 9 },
      { i: 'p1', x: 0, y: 0, w: 6, h: 4 },
    ];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.click(screen.getByTestId('heatmap-match-ratio'));

    const next = storeMock.setDashboardLayout.mock.calls[0]![0] as { i: string; h: number }[];
    expect(next.find((l) => l.i === 'p0')).toEqual({ i: 'p0', x: 0, y: 0, w: 2, h: 9 });
  });

  it('이미 도면 비율이면 버튼이 비활성이다', () => {
    storeMock.panel = planPanel();
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 5 }];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('heatmap-match-ratio')).toBeDisabled();
  });

  it('도면이 없으면 버튼 자체를 노출하지 않는다', () => {
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.queryByTestId('heatmap-match-ratio')).toBeNull();
  });
});

// 스테이지 맞춤 방식 — 패널 크기를 그대로 둔 채 여백만 없애는 길(잘림/왜곡을 감수).
describe('도면 맞춤 방식(stage_fit)', () => {
  const planPanelFit = (stage_fit?: string) =>
    ({
      id: 'p1',
      type: 'heatmap',
      title: '히트맵',
      config: {
        floor_plans: [{ asset_id: 'asset-1', natural_width: 800, natural_height: 600 }],
        ...(stage_fit ? { stage_fit } : {}),
      },
    }) as unknown as PanelConfig;

  it('도면이 없으면 선택기를 노출하지 않는다(설정할 대상이 없다)', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('heatmap-stage-fit')).toBeNull();
  });

  it('미설정 config 는 기본값 contain 을 표시한다', () => {
    storeMock.panel = planPanelFit();
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect((screen.getByTestId('heatmap-stage-fit') as HTMLSelectElement).value).toBe('contain');
  });

  it('cover 선택은 draft config 에 반영되고 프리뷰 힌트가 잘림 경고로 바뀐다', () => {
    storeMock.panel = planPanelFit();
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.change(screen.getByTestId('heatmap-stage-fit'), { target: { value: 'cover' } });

    expect((screen.getByTestId('heatmap-stage-fit') as HTMLSelectElement).value).toBe('cover');
    expect(screen.getByTestId('heatmap-stage-fit-hint').textContent).toBe(
      'dashboard.settings.heatmapStageFitCoverHint',
    );
  });

  it('contain 으로 되돌리면 config 에서 필드를 지운다(죽은 필드를 남기지 않음)', () => {
    storeMock.panel = planPanelFit('cover');
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.change(screen.getByTestId('heatmap-stage-fit'), { target: { value: 'contain' } });

    // draft 반영은 관측 가능한 결과로 확인한다: 선택값과 힌트가 기본 상태로 돌아온다.
    expect((screen.getByTestId('heatmap-stage-fit') as HTMLSelectElement).value).toBe('contain');
    expect(screen.getByTestId('heatmap-stage-fit-hint').textContent).toBe(
      'dashboard.settings.heatmapStageFitContainHint',
    );
  });
});

// 타이틀 바 표시 옵션(모든 패널 공통). 미리보기가 draft config 를 크롬 context 로 받으므로
// 체크 해제가 즉시 미리보기에 반영된다.
describe('타이틀 바 표시 옵션', () => {
  it('기본은 체크됨(표시) — 미설정 config 는 기존 동작', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-show-title')).toBeChecked();
  });

  it('체크 해제하면 미리보기 패널의 타이틀 바가 사라진다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '1층 온도',
      config: {},
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('heatmap-title')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('panel-show-title'));

    expect(screen.getByTestId('panel-show-title')).not.toBeChecked();
    expect(screen.queryByTestId('heatmap-title')).toBeNull();
  });

  it('showTitle=false 로 저장된 패널은 체크 해제 상태로 열린다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'heatmap',
      title: '1층 온도',
      config: { showTitle: false },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-show-title')).not.toBeChecked();
    expect(screen.queryByTestId('heatmap-title')).toBeNull();
  });
});
