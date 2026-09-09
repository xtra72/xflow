// 미리보기 이동(팬)의 **실제 배선** — 설정 다이얼로그를 통째로 세워 재는 자리.
//
// `previewPan.test.tsx` 는 훅과 이음매를 재고, 이 파일은 그 훅이 **실제 미리보기에 붙어
// 있는가**를 잰다. 둘을 나누는 이유는 하나다: 훅만 세운 시험은 "다이얼로그가 이 훅을
// 부르지 않는다" 를 통과시킨다.
//
// 여기서 못박는 것 셋:
//   1. **캔버스가 아닌 패널도 옮겨진다.** 확대는 종류를 가리지 않으므로 갇히는 것도, 그
//      답인 팬도 종류를 가리지 않는다. 고정 입력을 게이지로 두는 것에 그 뜻이 있다.
//   2. **줌이 팬을 켜고 끈다.** 처음(들어맞음)에는 잡을 수 없고, 확대해 넘치면 잡을 수
//      있으며, 다시 줄이면 자리가 0 으로 되죄어진다.
//   3. **평행이동은 배율 바깥이다.** `translate(...) scale(...)` 순서라 이동량이 화면 px
//      그대로이고, 그래서 캔버스 포인터 환산에 보정이 필요 없다(`previewPan.ts` §좌표 보정).
//
// jsdom 은 레이아웃을 하지 않아 `clientWidth` 가 언제나 0 이다 — fit 컨테이너의 실측이
// 0 이면 스테이지가 아예 만들어지지 않으므로, 이 파일은 그 두 값을 프로토타입에 심어
// **영역 크기를 가진 화면**을 흉내 낸다(각 시험 뒤에 되돌린다).

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { DashboardLayoutItem, PanelConfig } from '@/stores/uiStore';
import { GRID_MARGIN_PX, unitsToPx } from './gridGeometry';
import { PREVIEW_PAN_ARROW_PX } from './previewPan';

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

// --- 고정 입력 -----------------------------------------------------------

/** 미리보기 영역(px). 아래 패널보다 작아 처음에는 축소되어 꼭 맞는다. */
const AREA_W = 200;
const AREA_H = 150;

/** 4 × 3 패널의 대시보드 픽셀 크기 — 채움 모드의 100% 가 영역에 정확히 맞는다. */
const PANEL_PX_W = unitsToPx(4, CELL);
const PANEL_PX_H = unitsToPx(3, CELL);

const originals = {
  width: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth'),
  height: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight'),
};

/** 영역 크기를 가진 화면을 흉내 낸다. 이 저장소에서 fit 컨테이너를 재는 유일한 통로다. */
function stubLayout(): void {
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get: () => AREA_W,
  });
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get: () => AREA_H,
  });
}

afterEach(() => {
  for (const [key, desc] of [
    ['clientWidth', originals.width],
    ['clientHeight', originals.height],
  ] as const) {
    if (desc) Object.defineProperty(HTMLElement.prototype, key, desc);
    else delete (HTMLElement.prototype as unknown as Record<string, unknown>)[key];
  }
});

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.setDashboardLayout.mockReset();
  // 게이지 — **캔버스가 아닌** 패널이다. 캔버스로 재면 "캔버스에서만 되는 것" 과
  // 구별되지 않는다.
  storeMock.panel = { id: 'p1', type: 'gauge', title: 'g', config: {} };
  storeMock.layout = [{ i: 'p1', x: 0, y: 0, w: 4, h: 3 }];
  storeMock.showGridLines = false;
  window.localStorage.clear();
  stubLayout();
});

function surface(): HTMLElement {
  return screen.getByTestId('preview-pan-surface');
}

function stageEl(): HTMLElement {
  return screen.getByTestId('preview-stage');
}

/** 지금 상자에 얹힌 이동량. `translate` 가 없으면 (0, 0) 이다. */
function stageOffset(): { x: number; y: number } {
  const m = /translate\((-?[\d.]+)px, (-?[\d.]+)px\)/.exec(stageEl().style.transform);
  return m === null ? { x: 0, y: 0 } : { x: Number(m[1]), y: Number(m[2]) };
}

function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): MouseEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

function dragSurface(dx: number, dy: number): void {
  fireEvent(surface(), pointer('pointerdown', 100, 100));
  fireEvent(surface(), pointer('pointermove', 100 + dx, 100 + dy));
  fireEvent(surface(), pointer('pointerup', 100 + dx, 100 + dy));
}

/** 확대 단추를 n 번 누른다(한 번에 10%). */
function zoomIn(n: number): void {
  for (let i = 0; i < n; i += 1) {
    fireEvent.click(screen.getByTestId('panel-settings-preview-zoom-in'));
  }
}

function renderDialog(): void {
  render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
}

describe('미리보기 팬 배선 — 캔버스가 아닌 패널도 확대하면 끌어 옮긴다', () => {
  it('들어맞는 동안에는 팬 표면이 잠들어 있다', () => {
    renderDialog();

    // 채움 모드 100% 는 영역에 정확히 맞는다 — 넘치는 것이 없으니 옮길 곳도 없다.
    expect(surface().dataset.canPan).toBe('false');
    expect(surface().className).not.toContain('cursor-grab');
    expect(surface().getAttribute('tabindex')).toBeNull();

    dragSurface(40, 20);
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });

  it('확대해 넘치면 잡을 수 있고, 끈 만큼 화면 px 그대로 움직인다', () => {
    renderDialog();
    zoomIn(5); // 100% → 150%

    expect(surface().dataset.canPan).toBe('true');
    expect(surface().className).toContain('cursor-grab');
    expect(surface().getAttribute('tabindex')).toBe('0');

    dragSurface(20, 10);
    expect(stageOffset()).toEqual({ x: 20, y: 10 });
  });

  it('방향키로도 옮겨진다 — 포인터를 쓰지 않는 길이 함께 열린다', () => {
    renderDialog();
    zoomIn(5);

    fireEvent.keyDown(surface(), { key: 'ArrowRight' });
    expect(stageOffset()).toEqual({ x: PREVIEW_PAN_ARROW_PX, y: 0 });
  });

  it('가장자리를 넘겨 끌 수 없다 — 그림이 영역 안으로 물러나지 않는다', () => {
    renderDialog();
    zoomIn(5);

    // 150% 에서 화면 크기는 (영역 × 1.5) 이므로 넘치는 절반은 (영역 × 0.25) 다.
    dragSurface(9999, 9999);
    expect(stageOffset()).toEqual({ x: AREA_W * 0.25, y: AREA_H * 0.25 });
  });

  it('다시 줄이면 자리가 0 으로 되죄어지고 조작도 잠긴다', () => {
    renderDialog();
    zoomIn(5);
    dragSurface(9999, 9999);
    expect(stageOffset().x).toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId('panel-settings-preview-zoom-reset'));

    expect(stageOffset()).toEqual({ x: 0, y: 0 });
    expect(surface().dataset.canPan).toBe('false');
  });
});

describe('평행이동은 배율 **바깥**이다 (포인터 환산에 보정이 필요 없는 이유)', () => {
  it('옮기지 않으면 변환 문자열이 이 기능이 들어오기 전과 한 글자도 다르지 않다', () => {
    renderDialog();

    expect(stageEl().style.transform).toMatch(/^scale\([^)]*\)$/);
  });

  it('옮기면 translate 가 scale **앞**에 선다 — 이동량이 화면 px 그대로여야 한다', () => {
    renderDialog();
    zoomIn(5);
    dragSurface(20, 10);

    // 순서가 뒤집히면 이동량에 배율이 곱해져, 끈 거리와 옮겨진 거리가 달라진다.
    expect(stageEl().style.transform).toMatch(/^translate\(20px, 10px\) scale\(/);
  });

  it('상자의 CSS 크기는 평행이동으로 달라지지 않는다 — 환산의 분모가 그대로다', () => {
    renderDialog();
    const before = { w: stageEl().style.width, h: stageEl().style.height };

    zoomIn(5);
    dragSurface(20, 10);

    // `getBoundingClientRect().width / stage.width` 가 환산의 분모다. 평행이동은 상자의
    // 크기를 건드리지 않으므로 그 몫이 달라지지 않고, 그래서 보정항이 필요 없다.
    expect(stageEl().style.width).toBe(before.w);
    expect(stageEl().style.height).toBe(before.h);
    expect(Number.parseFloat(stageEl().style.width)).toBe(PANEL_PX_W);
    expect(Number.parseFloat(stageEl().style.height)).toBe(PANEL_PX_H);
  });
});
