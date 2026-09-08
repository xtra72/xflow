// 에이전트 현황 패널 설정 — 요약 타일 선택과 항목별 디자인.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'agents', title: '에이전트 현황', config: {} } as PanelConfig,
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
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks')>();
  return { ...actual, useAgents: () => ({ data: { data: [] }, isLoading: false }) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import PanelSettingsDialog from './PanelSettingsDialog';

function renderDialog() {
  render(
    <QueryClientProvider client={inertQueryClient()}>
      <PanelSettingsDialog panelId="p1" onClose={() => {}} />
    </QueryClientProvider>,
  );
}

function savedConfig(): Record<string, unknown> {
  fireEvent.click(screen.getByText('dashboard.settings.apply'));
  const calls = storeMock.updatePanelConfig.mock.calls;
  expect(calls.length).toBeGreaterThan(0);
  return calls[calls.length - 1]![1] as Record<string, unknown>;
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.panel = { id: 'p1', type: 'agents', title: '에이전트 현황', config: {} };
});

describe('요약 타일 설정', () => {
  it('세 타일을 모두 낸다', () => {
    renderDialog();
    for (const item of ['total', 'active', 'inactive']) {
      expect(screen.getByTestId(`summary-tile-${item}`)).toBeChecked();
    }
  });

  it('타일을 끄면 목록에서 빠진다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('summary-tile-total'));
    expect(savedConfig().summaryItems).toEqual(['active', 'inactive']);
  });

  it('순번으로 차례를 바꾼다', () => {
    renderDialog();
    fireEvent.change(screen.getByTestId('summary-tile-order-inactive'), { target: { value: '1' } });
    expect(savedConfig().summaryItems).toEqual(['inactive', 'total', 'active']);
  });

  it('타일별 글자 설정은 그 타일에만 쌓인다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('summary-tile-design-active-button'));
    fireEvent.change(screen.getByTestId('summary-tile-font-active-size'), { target: { value: '24' } });

    const styles = savedConfig().summaryStyles as Record<string, { size?: number }>;
    expect(styles.active).toMatchObject({ size: 24 });
    expect(styles.total).toBeUndefined();
  });

  it('타일 배경색을 정하고 되돌린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('summary-tile-design-active-button'));
    fireEvent.change(screen.getByTestId('summary-tile-bg-active'), { target: { value: '#111111' } });
    expect((savedConfig().summaryStyles as Record<string, { bg?: string }>).active!.bg).toBe('#111111');

    fireEvent.click(screen.getByTestId('summary-tile-bg-reset-active'));
    expect((savedConfig().summaryStyles as Record<string, { bg?: string }>).active!.bg).toBeUndefined();
  });

  it('배지를 통째로 끄면 타일 목록도 감춘다 — 걸 곳이 없는 설정을 남겨 두지 않는다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('show-summary-badges'));
    expect(screen.queryByTestId('summary-tile-total')).not.toBeInTheDocument();
  });
});

describe('에이전트 상태 패널의 통계 타일 설정', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('다섯 타일을 모두 낸다', () => {
    renderDialog();
    for (const tile of ['messagesIn', 'messagesOut', 'errors', 'uptime', 'dropped']) {
      expect(screen.getByTestId(`stat-tile-${tile}`)).toBeChecked();
    }
  });

  it('타일을 끄면 목록에서 빠진다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-uptime'));
    expect(savedConfig().statTiles).toEqual(['messagesIn', 'messagesOut', 'errors', 'dropped']);
  });

  it('타일 디자인은 목록의 그 줄에 있다 — 순번은 없다', () => {
    renderDialog();
    expect(screen.queryByTestId('stat-tile-order-errors')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('stat-tile-design-errors-button'));
    fireEvent.change(screen.getByTestId('stat-tile-errors-value_font-size'), {
      target: { value: '28' },
    });

    const styles = savedConfig().statTileStyles as Record<string, { value_font?: { size?: number } }>;
    expect(styles.errors!.value_font).toMatchObject({ size: 28 });
    expect(styles.uptime).toBeUndefined();
  });

  it('다이어그램 뷰에서는 타일 목록을 감춘다 — 그릴 타일이 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1', viewMode: 'diagram' },
    };
    renderDialog();
    expect(screen.queryByTestId('stat-tile-errors')).not.toBeInTheDocument();
  });
})

describe('메시지 상세 타일 설정', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('여섯 타일을 모두 낸다', () => {
    renderDialog();
    for (const tile of [
      'externalReceived',
      'externalSent',
      'externalErrored',
      'internalReceived',
      'internalSent',
      'internalErrored',
    ]) {
      expect(screen.getByTestId(`message-tile-${tile}`)).toBeChecked();
    }
  });

  it('타일을 끄면 목록에서 빠진다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('message-tile-internalSent'));
    expect(savedConfig().messageTiles).not.toContain('internalSent');
  });

  it('타일 디자인은 목록의 그 줄에 있다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('message-tile-design-externalErrored-button'));
    fireEvent.change(screen.getByTestId('message-tile-externalErrored-value_font-size'), {
      target: { value: '22' },
    });

    const styles = savedConfig().messageTileStyles as Record<
      string,
      { value_font?: { size?: number } }
    >;
    expect(styles.externalErrored!.value_font).toMatchObject({ size: 22 });
    expect(styles.internalErrored).toBeUndefined();
  });

  it('통계 타일과 메시지 타일은 서로 다른 자리에 쌓인다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-errors'));
    const saved = savedConfig();
    expect(saved.statTiles).toBeDefined();
    expect(saved.messageTiles).toBeUndefined();
  });
})

describe('타일 격자 배치 편집기', () => {
  /** 레이아웃은 팝업 안에 있다. */
  function openLayout(area: 'stat' | 'message'): void {
    fireEvent.click(screen.getByTestId(`${area}-layout-popup-button`));
  }

  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  /** jsdom 은 레이아웃을 계산하지 않으므로 격자 크기를 심는다(8x4, 칸당 20px). */
  function stubCanvas(testId: string, cols: number, rows: number): void {
    const el = screen.getByTestId(testId);
    el.getBoundingClientRect = () =>
      ({
        left: 0, top: 0, right: cols * 20, bottom: rows * 20,
        width: cols * 20, height: rows * 20, x: 0, y: 0, toJSON: () => ({}),
      }) as DOMRect;
  }

  it('통계 격자는 8x4 가 기본', () => {
    renderDialog();
    openLayout('stat');
    expect((screen.getByTestId('stat-layout-cols') as HTMLInputElement).value).toBe('8');
    expect((screen.getByTestId('stat-layout-rows') as HTMLInputElement).value).toBe('4');
  });

  it('메시지 격자는 8x2 가 기본', () => {
    renderDialog();
    openLayout('message');
    expect((screen.getByTestId('message-layout-cols') as HTMLInputElement).value).toBe('8');
    expect((screen.getByTestId('message-layout-rows') as HTMLInputElement).value).toBe('2');
  });

  it('격자 크기를 바꾸면 저장된다', () => {
    renderDialog();
    openLayout('stat');
    fireEvent.change(screen.getByTestId('stat-layout-cols'), { target: { value: '6' } });
    expect(savedConfig().statGrid).toMatchObject({ cols: 6, rows: 4 });
  });

  it('타일을 끌면 칸 단위로 옮겨진다', () => {
    renderDialog();
    openLayout('stat');
    stubCanvas('stat-layout-canvas', 8, 4);
    fireEvent.mouseDown(screen.getByTestId('stat-layout-tile-messagesIn'), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: 40, clientY: 40 });
    fireEvent.mouseUp(window);

    const areas = savedConfig().statTileAreas as Record<string, { x: number; y: number }>;
    expect(areas.messagesIn).toMatchObject({ x: 3, y: 3 });
  });

  it('모서리를 끌면 크기가 바뀐다', () => {
    renderDialog();
    openLayout('stat');
    stubCanvas('stat-layout-canvas', 8, 4);
    fireEvent.mouseDown(screen.getByTestId('stat-layout-tile-messagesIn-resize'), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: 40, clientY: 0 });
    fireEvent.mouseUp(window);

    const areas = savedConfig().statTileAreas as Record<string, { w: number }>;
    expect(areas.messagesIn!.w).toBe(4);
  });

  it('표에서 x·y·w·h 를 직접 찍는다', () => {
    renderDialog();
    openLayout('stat');
    fireEvent.click(screen.getByTestId('stat-layout-mode-table'));
    fireEvent.change(screen.getByTestId('stat-layout-area-errors-w'), { target: { value: '4' } });

    const areas = savedConfig().statTileAreas as Record<string, { w: number }>;
    expect(areas.errors!.w).toBe(4);
  });

  it('표는 격자 밖으로 나가지 못하게 가둔다', () => {
    renderDialog();
    openLayout('stat');
    fireEvent.click(screen.getByTestId('stat-layout-mode-table'));
    fireEvent.change(screen.getByTestId('stat-layout-area-errors-x'), { target: { value: '99' } });

    const areas = savedConfig().statTileAreas as Record<string, { x: number; w: number }>;
    expect(areas.errors!.x + areas.errors!.w - 1).toBeLessThanOrEqual(8);
  });

  it('그래픽과 표는 한 번에 하나만 보인다', () => {
    renderDialog();
    openLayout('stat');
    expect(screen.getByTestId('stat-layout-canvas')).toBeInTheDocument();
    expect(screen.queryByTestId('stat-layout-area-errors-x')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('stat-layout-mode-table'));
    expect(screen.queryByTestId('stat-layout-canvas')).not.toBeInTheDocument();
    expect(screen.getByTestId('stat-layout-area-errors-x')).toBeInTheDocument();
  });

  it('끈 타일은 배치에서도 빠진다 — 없는 것을 놓을 자리는 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1', statTiles: ['errors'] },
    };
    renderDialog();
    openLayout('stat');
    expect(screen.getByTestId('stat-layout-tile-errors')).toBeInTheDocument();
    expect(screen.queryByTestId('stat-layout-tile-uptime')).not.toBeInTheDocument();
  });

  it('메시지 배치의 단위는 묶음 카드 둘이다 — 카드 한 장이 값 셋을 담는다', () => {
    renderDialog();
    openLayout('message');
    expect(screen.getByTestId('message-layout-tile-external')).toBeInTheDocument();
    expect(screen.getByTestId('message-layout-tile-internal')).toBeInTheDocument();
    expect(screen.queryByTestId('message-layout-tile-externalReceived')).not.toBeInTheDocument();
  });
})

describe('레이아웃 팝업', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('격자는 열기 전에는 펼쳐져 있지 않다 — 아래 목록을 밀어내지 않는다', () => {
    renderDialog();
    expect(screen.queryByTestId('stat-layout-canvas')).not.toBeInTheDocument();
    expect(screen.queryByTestId('message-layout-canvas')).not.toBeInTheDocument();
    // 타일 목록은 팝업 밖에 그대로 있다.
    expect(screen.getByTestId('stat-tile-errors')).toBeInTheDocument();
  });

  it('통계 레이아웃 팝업에 행·열과 배치가 함께 있다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-layout-popup-button'));
    expect(screen.getByTestId('stat-layout-rows')).toBeInTheDocument();
    expect(screen.getByTestId('stat-layout-cols')).toBeInTheDocument();
    expect(screen.getByTestId('stat-layout-canvas')).toBeInTheDocument();
  });

  it('메시지 레이아웃 팝업도 같은 구성이다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('message-layout-popup-button'));
    expect(screen.getByTestId('message-layout-rows')).toBeInTheDocument();
    expect(screen.getByTestId('message-layout-canvas')).toBeInTheDocument();
  });

  it('두 팝업은 서로 다른 격자를 연다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-layout-popup-button'));
    expect(screen.queryByTestId('message-layout-canvas')).not.toBeInTheDocument();
  });
})

describe('레이아웃 표의 차례와 디자인', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  function openTable(): void {
    fireEvent.click(screen.getByTestId('stat-layout-popup-button'));
    fireEvent.click(screen.getByTestId('stat-layout-mode-table'));
  }

  it('아래로 누르면 다음 항목과 자리를 바꾼다', () => {
    renderDialog();
    openTable();
    fireEvent.click(screen.getByTestId('stat-layout-down-messagesIn'));
    expect(savedConfig().statTiles).toEqual([
      'messagesOut',
      'messagesIn',
      'errors',
      'uptime',
      'dropped',
    ]);
  });

  it('위로 누르면 앞으로 간다', () => {
    renderDialog();
    openTable();
    fireEvent.click(screen.getByTestId('stat-layout-up-errors'));
    expect(savedConfig().statTiles).toEqual([
      'messagesIn',
      'errors',
      'messagesOut',
      'uptime',
      'dropped',
    ]);
  });

  it('맨 위는 위로, 맨 아래는 아래로 누를 수 없다 — 눌러도 아무 일이 없으면 고장으로 보인다', () => {
    renderDialog();
    openTable();
    expect(screen.getByTestId('stat-layout-up-messagesIn')).toBeDisabled();
    expect(screen.getByTestId('stat-layout-down-dropped')).toBeDisabled();
  });

  it('타이틀과 값 글자를 따로 정한다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-design-errors-button'));
    fireEvent.change(screen.getByTestId('stat-tile-errors-label_font-size'), {
      target: { value: '10' },
    });
    fireEvent.change(screen.getByTestId('stat-tile-errors-value_font-size'), {
      target: { value: '30' },
    });

    const design = (savedConfig().statTileStyles as Record<string, {
      label_font?: { size?: number };
      value_font?: { size?: number };
    }>).errors!;
    expect(design.label_font).toMatchObject({ size: 10 });
    expect(design.value_font).toMatchObject({ size: 30 });
  });

  it('값에 따른 색 규칙을 더한다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-design-errors-button'));
    fireEvent.click(screen.getByTestId('stat-tile-errors-rule-add'));
    fireEvent.change(screen.getByTestId('stat-tile-errors-rule-value-0'), {
      target: { value: '10' },
    });

    const design = (savedConfig().statTileStyles as Record<string, {
      valueColors?: { op: string; value: string }[];
    }>).errors!;
    expect(design.valueColors).toEqual([expect.objectContaining({ op: 'gte', value: '10' })]);
  });

  it('타일 배경색을 정한다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-design-errors-button'));
    fireEvent.change(screen.getByTestId('stat-tile-errors-bg'), { target: { value: '#331111' } });
    expect((savedConfig().statTileStyles as Record<string, { bg?: string }>).errors!.bg).toBe(
      '#331111',
    );
  });

  it('그래픽 모드에는 표가 없다 — 차례는 표에 있다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-layout-popup-button'));
    expect(screen.queryByTestId('stat-layout-up-errors')).not.toBeInTheDocument();
  });
})

describe('레이아웃 초기화', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: {
        agentId: 'a-1',
        statGrid: { rows: 2, cols: 4 },
        statTileAreas: { errors: { x: 3, y: 1, w: 2, h: 2 } },
        messageGrid: { rows: 3, cols: 6 },
      },
    };
  });

  it('통계 레이아웃 초기화는 격자와 자리를 되돌린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-layout-popup-button'));
    fireEvent.click(screen.getByTestId('stat-layout-reset'));

    const saved = savedConfig();
    expect(saved.statGrid).toBeUndefined();
    expect(saved.statTileAreas).toBeUndefined();
    // 메시지 쪽은 건드리지 않는다.
    expect(saved.messageGrid).toMatchObject({ rows: 3, cols: 6 });
  });

  it('메시지 레이아웃도 제 것만 되돌린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('message-layout-popup-button'));
    fireEvent.click(screen.getByTestId('message-layout-reset'));

    const saved = savedConfig();
    expect(saved.messageGrid).toBeUndefined();
    expect(saved.statGrid).toMatchObject({ rows: 2, cols: 4 });
  });
})

describe('디바이스 상태와 같은 구성', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('배지 표시와 공통 속성이 목록보다 위에 있다', () => {
    renderDialog();
    const badges = screen.getByTestId('agent-status-show-badges');
    const common = screen.getByTestId('agent-status-common-button');
    const list = screen.getByTestId('stat-tile-select-all');
    expect(badges.compareDocumentPosition(common) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(common.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('공통 속성은 모든 타일에 걸린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('agent-status-common-button'));
    fireEvent.change(screen.getByTestId('agent-status-common-value_font-size'), {
      target: { value: '26' },
    });
    expect(savedConfig().tileValueFont).toMatchObject({ size: 26 });
  });

  it('공통 속성 초기화는 글자와 배경을 되돌린다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: {
        agentId: 'a-1',
        tileValueFont: { size: 26 },
        tileLabelFont: { size: 9 },
        tileBg: '#112233',
      },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('agent-status-common-button'));
    fireEvent.click(screen.getByTestId('agent-status-common-reset'));
    const saved = savedConfig();
    for (const key of ['tileValueFont', 'tileLabelFont', 'tileBg']) {
      expect(saved[key]).toBeUndefined();
    }
  });

  it('그룹마다 전체 · 전체 해제가 있다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('stat-tile-clear-all'));
    expect(savedConfig().statTiles).toEqual([]);

    fireEvent.click(screen.getByTestId('message-tile-select-all'));
    expect(savedConfig().messageTiles).toHaveLength(6);
  });

  it('배지를 끄면 저장에 남는다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('agent-status-show-badges'));
    expect(savedConfig().showBadges).toBe(false);
  });

  it('다이어그램 뷰에서는 타일 설정 전부를 감춘다 — 출력 형식은 그대로 고른다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1', viewMode: 'diagram' },
    };
    renderDialog();
    expect(screen.queryByTestId('agent-status-common-button')).not.toBeInTheDocument();
    expect(screen.queryByTestId('stat-tile-select-all')).not.toBeInTheDocument();
    // 출력 형식 선택은 남는다.
    expect(screen.getByTestId('agent-status-viewmode-select')).toBeInTheDocument();
  });
})

describe('배지 글자 설정 자리', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('배지 표시 바로 뒤에 있다 — 따로 놓인 "배지 글자" 줄은 없다', () => {
    renderDialog();
    const toggle = screen.getByTestId('agent-status-show-badges');
    const design = screen.getByTestId('badge-design-button');
    expect(toggle.compareDocumentPosition(design) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // 공통 속성보다는 앞이다.
    const common = screen.getByTestId('agent-status-common-button');
    expect(design.compareDocumentPosition(common) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('배지를 끄면 글자 설정도 감춘다 — 감춘 배지에는 걸 곳이 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1', showBadges: false },
    };
    renderDialog();
    expect(screen.queryByTestId('badge-design-button')).not.toBeInTheDocument();
  });

  it('배지 글자 설정은 그대로 저장된다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('badge-design-button'));
    fireEvent.change(screen.getByTestId('badge-font-size'), { target: { value: '14' } });
    expect(savedConfig().badge_font).toMatchObject({ size: 14 });
  });
})

describe('배지 항목 설정', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1' },
    };
  });

  it('배지 항목을 고른다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('agent-badge-enabled'));
    expect(savedConfig().badgeItems).toEqual(['type', 'status']);
  });

  it('항목마다 디자인을 정한다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('agent-badge-design-type-button'));
    fireEvent.change(screen.getByTestId('agent-badge-type-value_font-size'), {
      target: { value: '16' },
    });
    const styles = savedConfig().badgeStyles as Record<string, { value_font?: { size?: number } }>;
    expect(styles.type!.value_font).toMatchObject({ size: 16 });
    expect(styles.status).toBeUndefined();
  });

  it('배지를 끄면 항목 목록도 감춘다 — 걸 곳이 없는 설정을 남기지 않는다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'agent-status',
      title: '에이전트 상태',
      config: { agentId: 'a-1', showBadges: false },
    };
    renderDialog();
    expect(screen.queryByTestId('agent-badge-type')).not.toBeInTheDocument();
  });
})
