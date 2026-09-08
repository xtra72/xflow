// 미리보기 크기 조절 — 실제 패널처럼 그리드 단위로 끌고, 저장 버튼이 커밋한다.
//
// 잠그는 것 셋:
//   1. 그리드는 **현재 대시보드 설정**을 쓴다(칼럼 수·셀 크기·가이드 라인 표시).
//   2. 끌기는 draft 에만 쌓이고 저장 전에는 대시보드 레이아웃이 그대로다.
//   3. 저장은 그 패널의 w/h 만 바꾸고 다른 패널의 자리는 건드리지 않는다.
//
// store 폴링/네트워크 훅은 정적 값으로 대체한다(resize.test.tsx 와 같은 패턴).

import { QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { inertQueryClient } from '@/hooks/inertQueryClient';

import type { DashboardLayoutItem, PanelConfig } from '@/stores/uiStore';
import { GRID_MARGIN_PX, unitsToPx } from './gridGeometry';

const GRID_COLS = 12;
/** 셀 한 변 — gridCellSize(gridWidth, cols) 가 이 값을 내도록 gridWidth 를 맞춘다. */
const CELL = 100;
const GRID_WIDTH = CELL * GRID_COLS + GRID_MARGIN_PX * (GRID_COLS - 1);

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: 'orig', config: {} } as PanelConfig,
  layout: [] as DashboardLayoutItem[],
  showGridLines: true,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
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
    setDashboardLayout: storeMock.setDashboardLayout,
    dashboardGridCols: GRID_COLS,
    dashboardGridWidth: GRID_WIDTH,
    dashboardShowGridLines: storeMock.showGridLines,
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
// 실제 AgentPanel 을 그리므로 그 패널이 쓰는 뮤테이션 훅까지 대체해야 한다
// (미리보기가 목업이던 시절에는 필요 없던 것들이다).
vi.mock('@/hooks/useAgent', () => {
  // vi.mock 은 호이스팅되므로 팩토리 **안에서** 만든다(바깥 변수는 아직 초기화 전).
  const idleMutation = () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() });
  return {
  useAgents: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useAgent: () => ({ data: undefined }),
  useAgentStats: () => ({ data: undefined }),
  useExecAgent: idleMutation,
  useStartAgent: idleMutation,
  useStopAgent: idleMutation,
  useRestartAgent: idleMutation,
  useEnableAgent: idleMutation,
  useDisableAgent: idleMutation,
  useDeleteAgent: idleMutation,
  useConfigureAgent: idleMutation,
  useQueryAgent: idleMutation,
  };
});
// 실제 패널을 그리므로 데이터 계층의 **조회 진입점만** 정적으로 덮는다. 모듈 전체를
// 대체하면 패널이 쓰는 다른 export(startFlow 등)까지 사라져 렌더가 깨진다.
vi.mock('@/services/api/flowService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/api/flowService')>()),
  getFlows: () => Promise.resolve({ data: [] }),
}));
vi.mock('@/services/api/monitorService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/api/monitorService')>()),
  getMetrics: () => Promise.resolve({}),
  useNetworkStats: () => ({ data: undefined }),
  useSystemMetrics: () => ({ data: undefined, isLoading: false }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

/**
 * 프레임의 렌더 크기를 지정한다.
 *
 * jsdom 은 레이아웃을 하지 않아 getBoundingClientRect 가 0 이다. 배율 환산은 이
 * 크기를 기준으로 하므로, 실제 화면에서 축소되어 보이는 상태(여기서는 1/2 배율)를
 * 흉내 낸다.
 */
function stubFrameSize(w: number, h: number) {
  const frame = screen.getByTestId('panel-resize-frame');
  frame.getBoundingClientRect = () =>
    ({ width: w, height: h, top: 0, left: 0, right: w, bottom: h, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;
  return frame;
}

/** 손잡이를 (dx, dy) 만큼 끈다. */
function dragHandle(dx: number, dy: number) {
  const handle = screen.getByTestId('panel-resize-handle');
  fireEvent.mouseDown(handle, { clientX: 0, clientY: 0 });
  fireEvent.mouseMove(window, { clientX: dx, clientY: dy });
  fireEvent.mouseUp(window);
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.setDashboardLayout.mockReset();
  storeMock.panel = { id: 'p1', type: 'heatmap', title: 'orig', config: {} };
  storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 4, h: 3 }];
  storeMock.showGridLines = true;
  window.localStorage.clear();
});

describe('미리보기 크기 조절 — 표시', () => {
  it('대시보드에 배치된 패널은 크기 조절 손잡이를 갖는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-resize-handle')).toBeInTheDocument();
  });

  it('현재 크기를 배지로 보여준다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-resize-size-badge')).toHaveTextContent('4 × 3');
  });

  it('대시보드에 배치되지 않은 패널은 기준 크기가 없어 손잡이를 내지 않는다', () => {
    storeMock.layout = [];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('panel-resize-handle')).not.toBeInTheDocument();
  });

  it('프레임과 그리드는 패널 위에 그려진다 — 아래에 두면 패널 배경에 가려 보이지 않는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 미리보기 패널은 transform 으로 축소되고, transform 은 스택 컨텍스트를 만들어
    // z-index:auto 인 오버레이보다 위에 그려진다. z 값이 빠지면 조용히 사라진다.
    expect(screen.getByTestId('panel-resize-overlay').className).toMatch(/\bz-\d+\b/);
  });

  it('프레임은 포인터를 가로막지 않는다 — 미리보기 안의 드래그를 방해하면 안 된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-resize-overlay').className).toContain('pointer-events-none');
    expect(screen.getByTestId('panel-resize-handle').className).toContain('pointer-events-auto');
  });
});

describe('미리보기 형태 — 사이징 정본은 스테이지 하나다', () => {
  // 종전에는 타입마다 상자 크기를 따로 정했다(3/2, 4/3, 16/9, 1/1). 그 값들이 패널의
  // 실제 그리드 크기와 달라, 프레임·그리드와 어긋나고 대시보드와도 다른 모양이 됐다.

  it('게이지도 패널 상자를 채운다 — 타입별 고정 비율은 대시보드와 달랐다', () => {
    storeMock.panel = { id: 'p1', type: 'gauge', title: 'g', config: {} };
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 4, h: 2 }];

    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 4×2 패널의 게이지는 대시보드에서도 가로로 넓은 상자를 채운다.
    // 미리보기만 1:1 로 두면 대시보드와 다른 모양을 보여주게 된다.
    const wrapper = screen.getByTestId('gauge-preview-wrapper');
    expect(wrapper.style.width).toBe('100%');
    expect(wrapper.style.height).toBe('100%');
  });

  it('목업이 있던 타입은 이제 실제 패널을 그린다 — 손으로 그린 미니는 남아 있지 않다', () => {
    storeMock.panel = { id: 'p1', type: 'device', title: 'd', config: {} };
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 6, h: 2 }];

    // 실제 패널은 조회 훅을 타므로 비활성 클라이언트로 감싼다(요청은 나가지 않는다).
    render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    expect(screen.getByTestId('real-panel-preview')).toBeInTheDocument();
    expect(screen.queryByTestId('nasa-mini-preview-wrapper')).not.toBeInTheDocument();
  });

  // 반대 방향(실패널 미리보기가 그리드 비율을 따르는 것)은
  // PanelSettingsDialog.heatmap.test.tsx 의 "미리보기 — 패널 실제 종횡비" 가 덮는다.
});

describe('미리보기 내용 — 대시보드와 같은 렌더러', () => {
  // 종전에는 타입별 목업을 그려 컬럼도 값도 실제 패널과 달랐다(가짜 행 sample-1 등).
  // 대시보드와 같은 renderDashboardPanel 을 타는지 확인한다.

  /** 비활성 클라이언트로 감싼다 — 실제 패널은 조회 훅을 탄다(요청은 나가지 않는다). */
  function renderWithClient() {
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it.each([
    ['flows'],
    ['agents'],
    ['devices'],
    ['resource'],
    ['logs'],
    ['monitor-stats'],
    ['properties-grid'],
  ])('%s 미리보기는 실제 패널을 그린다', (type) => {
    storeMock.panel = { id: 'p1', type, title: 't', config: {} } as PanelConfig;
    renderWithClient();

    expect(screen.getByTestId('real-panel-preview')).toBeInTheDocument();
  });

  it('차트 계열은 각자의 실패널 분기를 그대로 쓴다', () => {
    // heatmap 은 종전부터 실패널을 그렸다 — 이 전환의 대상이 아니다.
    storeMock.panel = { id: 'p1', type: 'heatmap', title: 't', config: {} };
    renderWithClient();

    expect(screen.queryByTestId('real-panel-preview')).not.toBeInTheDocument();
    expect(screen.getByTestId('heatmap-preview-wrapper')).toBeInTheDocument();
  });
});

describe('미리보기 기하 — 대시보드 그리드를 그대로 재현한다', () => {
  it('패널 상자는 그리드 단위 수만큼 차지한다 — 내부 마진 포함', () => {
    // 4×3, 셀 100, 마진 16 → 448 × 332. 단위 비 4/3 과 다르다.
    window.localStorage.setItem('panelSettings.previewFillMode', 'fit');
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 4, h: 3 }];
    storeMock.panel = { id: 'p1', type: 'flows', title: 't', config: {} };

    render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    // 영역 미실측(jsdom)에서는 종횡비 폴백을 타되, 그 비율은 그리드 기하를 따른다.
    expect(screen.getByTestId('preview-stage').style.aspectRatio).toBe(`${448 / 332} / 1`);
  });

  it('모든 타입이 같은 스테이지를 쓴다 — 상자가 갈라지면 프레임과 어긋난다', () => {
    storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 4, h: 3 }];
    render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    expect(screen.getAllByTestId('preview-stage')).toHaveLength(1);
  });

  // 스테이지 배율·간격 계산 자체는 previewStage.test.ts 가 DOM 없이 덮는다.
  // 여기서는 배선(스토어 → 기하 → DOM)만 확인한다.
});

describe('컬럼 설정 — 다른 옵션과 같은 draft 규율을 따른다', () => {
  // 종전에는 이 섹션만 updatePanelConfig 로 스토어에 직접 썼다. 그러면 draft 가 옛 값을
  // 들고 있어 미리보기가 바뀌지 않고, 저장이 그 옛 draft 를 커밋하며 변경이 되돌아간다.

  function renderFlows() {
    storeMock.panel = {
      id: 'p1',
      type: 'flows',
      title: 't',
      config: { visibleColumns: ['name', 'status', 'node_count', 'updated_at', 'actions'] },
    } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('컬럼을 꺼도 저장 전에는 스토어를 건드리지 않는다', () => {
    renderFlows();

    fireEvent.click(screen.getByTestId('column-toggle-node_count'));

    expect(storeMock.updatePanelConfig).not.toHaveBeenCalled();
  });

  it('체크 상태가 즉시 바뀐다 — draft 가 반영되지 않으면 눌러도 그대로였다', () => {
    renderFlows();

    const toggle = screen.getByTestId('column-toggle-node_count');
    expect(toggle).toBeChecked();

    fireEvent.click(toggle);

    expect(screen.getByTestId('column-toggle-node_count')).not.toBeChecked();
  });

  it('저장하면 끈 컬럼이 빠진 채로 커밋된다', () => {
    renderFlows();

    fireEvent.click(screen.getByTestId('column-toggle-node_count'));
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({
        visibleColumns: ['name', 'status', 'updated_at', 'actions'],
      }),
    );
  });

  it('마지막 한 컬럼은 끌 수 없다 — 빈 표가 되면 패널이 쓸모없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'flows',
      title: 't',
      config: { visibleColumns: ['name'] },
    } as PanelConfig;
    render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByTestId('column-toggle-name'));

    expect(screen.getByTestId('column-toggle-name')).toBeChecked();
  });
});

describe('플로우 현황 — 디자인 설정은 한 곳씩', () => {
  function renderFlows(config: Record<string, unknown> = {}) {
    storeMock.panel = { id: 'p1', type: 'flows', title: 't', config } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('타이틀 색을 정할 곳은 타이틀 디자인뿐이다 — 스타일 섹션에 중복 항목이 없다', () => {
    renderFlows();

    // 종전에는 악센트 header 그룹이 타이틀 색을 함께 정해, 두 곳 중 어느 쪽이 이기는지
    // 알 수 없었다.
    expect(screen.getByTestId('panel-title-design-button')).toBeInTheDocument();
    expect(screen.queryByTestId('accent-group-header')).not.toBeInTheDocument();
  });

  it('스타일 섹션을 내지 않는다 — 모양은 각 설정의 디자인 팝업이 갖는다', () => {
    renderFlows();

    // 타이틀·컬럼·요약 배지가 각자 디자인을 갖게 되면서 남은 항목이 없다.
    // 하나뿐인 항목을 위해 고르기 → 편집 두 단계를 남기면 빈 껍데기가 된다.
    expect(screen.queryByTestId('accent-group-picker')).not.toBeInTheDocument();
  });

  it('컬럼 디자인은 컬럼 설정 안의 디자인 팝업에 있다 — 헤더와 요소를 따로 정한다', () => {
    renderFlows();

    // 접혀 있다가 디자인 배지를 눌러야 열린다(타이틀 디자인과 같은 조작).
    expect(screen.queryByTestId('table-header-font-size')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('flow-columns-design-button'));

    expect(screen.getByTestId('table-header-font-size')).toBeInTheDocument();
    expect(screen.getByTestId('table-cell-font-size')).toBeInTheDocument();
  });

  it('요약 배지 디자인은 배지 설정 옆 팝업에 있다', () => {
    renderFlows();

    fireEvent.click(screen.getByTestId('badge-design-button'));

    expect(screen.getByTestId('badge-font-size')).toBeInTheDocument();
  });

  it('요약 배지는 기본으로 켜져 있고 끄면 디자인 배지가 사라진다', () => {
    renderFlows();

    const toggle = screen.getByTestId('show-summary-badges');
    expect(toggle).toBeChecked();
    expect(screen.getByTestId('badge-design-button')).toBeInTheDocument();

    fireEvent.click(toggle);

    expect(screen.getByTestId('show-summary-badges')).not.toBeChecked();
    // 감춘 배지에는 디자인을 걸 곳이 없다.
    expect(screen.queryByTestId('badge-design-button')).not.toBeInTheDocument();
  });

  it('배지를 끄면 저장 시 config 에 남는다 — 기본(켬)은 남기지 않는다', () => {
    renderFlows();

    fireEvent.click(screen.getByTestId('show-summary-badges'));
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({ showSummaryBadges: false }),
    );
  });
});

describe('에이전트 현황 — 플로우 현황과 같은 디자인 구조', () => {
  // 목록형 패널 둘이 같은 config 키(table_header_font / table_cell_font / badge_font)와
  // 같은 조작을 쓴다. 한쪽만 바뀌면 같은 화면에서 다른 규칙을 배워야 한다.

  function renderAgents(config: Record<string, unknown> = {}) {
    storeMock.panel = { id: 'p1', type: 'agents', title: 't', config } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('스타일 섹션 대신 항목별 디자인 팝업을 쓴다', () => {
    renderAgents();

    expect(screen.queryByTestId('accent-group-picker')).not.toBeInTheDocument();
    expect(screen.getByTestId('panel-title-design-button')).toBeInTheDocument();
    expect(screen.getByTestId('agent-columns-design-button')).toBeInTheDocument();
    expect(screen.getByTestId('badge-design-button')).toBeInTheDocument();
  });

  it('컬럼 디자인은 헤더와 요소를 따로 정한다', () => {
    renderAgents();

    fireEvent.click(screen.getByTestId('agent-columns-design-button'));

    expect(screen.getByTestId('table-header-font-size')).toBeInTheDocument();
    expect(screen.getByTestId('table-cell-font-size')).toBeInTheDocument();
  });

  it('요약 배지를 끄면 디자인 배지가 사라지고 저장 시 남는다', () => {
    renderAgents();

    fireEvent.click(screen.getByTestId('show-summary-badges'));
    expect(screen.queryByTestId('badge-design-button')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('panel-settings-apply'));
    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({ showSummaryBadges: false }),
    );
  });

  it('컬럼 설정도 draft 를 따른다 — 저장 전에는 스토어를 건드리지 않는다', () => {
    renderAgents({ visibleColumns: ['name', 'type', 'status'] });

    fireEvent.click(screen.getByTestId('column-toggle-type'));

    expect(storeMock.updatePanelConfig).not.toHaveBeenCalled();
    expect(screen.getByTestId('column-toggle-type')).not.toBeChecked();
  });
});

describe('디바이스 목록 — 목록형 패널과 같은 디자인 구조', () => {
  function renderDevices(config: Record<string, unknown> = {}) {
    storeMock.panel = { id: 'p1', type: 'devices', title: 't', config } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('스타일 섹션 대신 항목별 디자인 팝업을 쓴다', () => {
    renderDevices();

    expect(screen.queryByTestId('accent-group-picker')).not.toBeInTheDocument();
    expect(screen.getByTestId('panel-title-design-button')).toBeInTheDocument();
    expect(screen.getByTestId('device-columns-design-button')).toBeInTheDocument();
    expect(screen.getByTestId('badge-design-button')).toBeInTheDocument();
  });

  it('컬럼 디자인은 헤더와 요소를 따로 정한다', () => {
    renderDevices();

    fireEvent.click(screen.getByTestId('device-columns-design-button'));

    expect(screen.getByTestId('table-header-font-size')).toBeInTheDocument();
    expect(screen.getByTestId('table-cell-font-size')).toBeInTheDocument();
  });

  it('요약 배지를 끄면 저장 시 남는다', () => {
    renderDevices();

    fireEvent.click(screen.getByTestId('show-summary-badges'));
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({ showSummaryBadges: false }),
    );
  });
});

describe('디바이스 상태(속성 그리드) — 타일·배치·표시 항목', () => {
  function renderGrid(config: Record<string, unknown> = {}) {
    storeMock.panel = { id: 'p1', type: 'properties-grid', title: 't', config } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  /** 카드 설정은 디자인 팝업 안에 있다. */
  function openCardDesign(): void {
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
  }

  it('카드 분할을 정한다 — 기본 3×3', () => {
    renderGrid();
    openCardDesign();

    expect((screen.getByTestId('card-rows') as HTMLInputElement).value).toBe('3');
    expect((screen.getByTestId('card-cols') as HTMLInputElement).value).toBe('3');

    fireEvent.change(screen.getByTestId('card-cols'), { target: { value: '4' } });
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({ cardCols: 4 }),
    );
  });

  it('조각마다 시작 위치와 칸 수를 정한다', () => {
    renderGrid();
    openCardDesign();
    fireEvent.click(screen.getByTestId('card-layout-mode-table'));

    // 값을 위쪽 한 줄 전체로 — 칸 하나만 쓰던 종전에는 만들 수 없던 배치다.
    fireEvent.change(screen.getByTestId('card-area-value-colSpan'), { target: { value: '3' } });
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    const call = storeMock.updatePanelConfig.mock.calls.at(-1)!;
    expect((call[1] as Record<string, unknown>).cardAreas).toMatchObject({
      value: { row: 2, col: 1, rowSpan: 1, colSpan: 3 },
    });
  });

  it('네 값(시작 행·열, 행·열 수)을 조각마다 낸다', () => {
    renderGrid();
    openCardDesign();
    fireEvent.click(screen.getByTestId('card-layout-mode-table'));

    for (const element of ['label', 'value', 'time']) {
      for (const field of ['row', 'col', 'rowSpan', 'colSpan']) {
        expect(screen.getByTestId(`card-area-${element}-${field}`)).toBeInTheDocument();
      }
    }
  });

  it('갱신 시각 표시를 끄면 저장에 남는다', () => {
    renderGrid();

    fireEvent.click(screen.getByTestId('properties-grid-show-updated'));
    fireEvent.click(screen.getByTestId('panel-settings-apply'));

    expect(storeMock.updatePanelConfig).toHaveBeenCalledWith(
      'p1',
      expect.objectContaining({ showUpdatedAt: false }),
    );
  });

  it('카드 조각별 디자인 팝업이 항목명·값·시각을 따로 정한다', () => {
    renderGrid();

    openCardDesign();

    expect(screen.getByTestId('property-label-font-size')).toBeInTheDocument();
    expect(screen.getByTestId('property-value-font-size')).toBeInTheDocument();
    expect(screen.getByTestId('property-time-font-size')).toBeInTheDocument();
  });

  it('스타일 섹션 대신 디자인 팝업을 쓴다', () => {
    renderGrid();
    expect(screen.queryByTestId('accent-group-picker')).not.toBeInTheDocument();
  });

  it('고른 항목에만 배치 순번 칸이 나온다', () => {
    // "전체"(미선택)는 명시 목록이 없어 순서를 지정할 자리가 없다.
    renderGrid();
    expect(screen.queryByTestId(/^property-order-/)).not.toBeInTheDocument();
  });
});

describe('에이전트 상태 — 죽은 스타일 대신 배지 디자인', () => {
  function renderAgentStatus() {
    storeMock.panel = { id: 'p1', type: 'agent-status', title: 't', config: {} } as PanelConfig;
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('스타일 섹션을 내지 않는다 — 이 패널은 악센트를 읽지 않아 아무 일도 하지 않았다', () => {
    renderAgentStatus();
    expect(screen.queryByTestId('accent-group-picker')).not.toBeInTheDocument();
  });

  it('상태 배지 디자인 팝업이 있다', () => {
    renderAgentStatus();

    fireEvent.click(screen.getByTestId('badge-design-button'));

    expect(screen.getByTestId('badge-font-size')).toBeInTheDocument();
  });

  it('배지를 감추는 토글은 두지 않는다 — 이 패널의 배지는 본문 자체다', () => {
    renderAgentStatus();
    expect(screen.queryByTestId('show-summary-badges')).not.toBeInTheDocument();
  });
});

describe('악센트 그룹 — 목록에서 고른다', () => {
  // 목록형 패널(플로우·에이전트·디바이스)은 디자인 팝업으로 옮겨 가 스타일 섹션이
  // 없다. 그룹 선택기 자체는 아직 악센트 그룹을 갖는 다른 패널에서 그대로 쓰인다.
  //
  // `_base`(= panelColor)는 그룹 목록에서 빠졌다 — 타입과 무관한 패널 속성이라
  // 패널 옵션의 패널 색상으로 옮겼다. 그래서 여기서는 실제 악센트 그룹인
  // `header` 로 선택 동작을 검증한다.
  function renderWithClient() {
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  }

  it('미리보기 클릭 없이도 그룹 목록이 보인다', () => {
    storeMock.panel = { id: 'p1', type: 'logs', title: 't', config: {} };
    renderWithClient();

    // 종전에는 목업 영역을 클릭해야 스타일 섹션이 열렸다.
    expect(screen.getByTestId('accent-group-picker')).toBeInTheDocument();
    expect(screen.getByTestId('accent-group-header')).toBeInTheDocument();
  });

  it('그룹을 고르면 색 편집이 열리고, 다시 누르면 닫힌다', () => {
    storeMock.panel = { id: 'p1', type: 'logs', title: 't', config: {} };
    renderWithClient();

    const base = screen.getByTestId('accent-group-header');
    expect(base).toHaveAttribute('aria-pressed', 'false');

    fireEvent.click(base);
    expect(screen.getByTestId('accent-group-header')).toHaveAttribute('aria-pressed', 'true');

    fireEvent.click(screen.getByTestId('accent-group-header'));
    expect(screen.getByTestId('accent-group-header')).toHaveAttribute('aria-pressed', 'false');
  });
});

describe('미리보기 크기 조절 — 끌기', () => {
  it('끄는 거리를 화면 배율로 환산해 그리드 단위를 바꾼다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    // 4×3 패널을 1/2 배율로 보여주는 상태.
    stubFrameSize(unitsToPx(4, CELL) / 2, unitsToPx(3, CELL) / 2);

    // 화면에서 한 칸의 절반(=대시보드 한 칸)만큼 가로로 끈다 → 폭 +1.
    dragHandle(unitsToPx(1, CELL) / 2, 0);

    expect(screen.getByTestId('panel-resize-size-badge')).toHaveTextContent('5 × 3');
  });

  it('칼럼 수를 넘겨 끌 수 없다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    stubFrameSize(unitsToPx(4, CELL) / 2, unitsToPx(3, CELL) / 2);

    dragHandle(10000, 0);

    expect(screen.getByTestId('panel-resize-size-badge')).toHaveTextContent(`${GRID_COLS} × 3`);
  });

  it('끌기만으로는 대시보드 레이아웃이 바뀌지 않는다 — 저장 버튼이 커밋한다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    stubFrameSize(unitsToPx(4, CELL) / 2, unitsToPx(3, CELL) / 2);

    dragHandle(unitsToPx(1, CELL) / 2, 0);

    expect(storeMock.setDashboardLayout).not.toHaveBeenCalled();
  });
});

describe('미리보기 크기 조절 — 저장', () => {
  /** 저장 버튼을 누른다. */
  function save() {
    fireEvent.click(screen.getByTestId('panel-settings-apply'));
  }

  it('저장하면 그 패널의 w/h 만 바뀐다', () => {
    storeMock.layout = [
      { i: 'p1', x: 0, y: 0, w: 4, h: 3 },
      { i: 'p2', x: 4, y: 0, w: 2, h: 2 },
    ];
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    stubFrameSize(unitsToPx(4, CELL) / 2, unitsToPx(3, CELL) / 2);

    dragHandle(unitsToPx(1, CELL) / 2, 0);
    save();

    expect(storeMock.setDashboardLayout).toHaveBeenCalledWith([
      { i: 'p1', x: 0, y: 0, w: 5, h: 3 },
      { i: 'p2', x: 4, y: 0, w: 2, h: 2 },
    ]);
  });

  it('크기를 건드리지 않았다면 레이아웃을 다시 쓰지 않는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    save();
    expect(storeMock.setDashboardLayout).not.toHaveBeenCalled();
  });

  it('끌었다가 원래 크기로 되돌리면 레이아웃을 다시 쓰지 않는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    stubFrameSize(unitsToPx(4, CELL) / 2, unitsToPx(3, CELL) / 2);

    // 한 칸 늘렸다가 다시 한 칸 줄인다. 각 드래그는 그 시점의 크기에서 시작한다.
    dragHandle(unitsToPx(1, CELL) / 2, 0);
    expect(screen.getByTestId('panel-resize-size-badge')).toHaveTextContent('5 × 3');

    dragHandle(-unitsToPx(1, CELL) / 2, 0);
    expect(screen.getByTestId('panel-resize-size-badge')).toHaveTextContent('4 × 3');

    save();

    expect(storeMock.setDashboardLayout).not.toHaveBeenCalled();
  });
});
