// 설정 미리보기의 **배치 편집** — 미리보기에서 요소를 끌어 배치할 수 있어야 한다.
//
// 보고된 증상: 통계·게이지 모두 설정 미리보기에서 배치 편집이 되지 않는다(게이지는
// 대시보드에서는 된다). 두 패널이 함께 안 되므로 미리보기 쪽 공통 원인을 의심했고,
// 이 테스트가 어느 경로에서 끊기는지 가른다.
//
// @spec SPEC-CHART-004 AC-41

import { QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'stat', title: 's', config: {} } as PanelConfig,
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
vi.mock('@/hooks/useAgent', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/useAgent')>()),
  useAgents: () => ({ data: { data: [] } }),
  useAgent: () => ({ data: undefined }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

function open(panel: PanelConfig) {
  storeMock.panel = panel;
  return render(
    <QueryClientProvider client={inertQueryClient()}>
      <PanelSettingsDialog panelId="p1" onClose={() => {}} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  window.localStorage.clear();
});

describe('통계 미리보기 (AC-41)', () => {
  it('채널 모드에서도 미리보기에 배치 편집 표식이 있다', () => {
    const { container } = open({
      id: 'p1',
      type: 'stat',
      title: 's',
      config: { channel_name: 'c' },
    });

    expect(screen.getByTestId('stat-preview-wrapper')).toBeInTheDocument();
    // 미리보기는 항상 편집이다(`forceEdit`) — 끌 수 있는 표식이 있어야 한다.
    expect(container.querySelectorAll('[data-panel-drag]').length).toBeGreaterThan(0);
    expect(screen.getByTestId('panel-align-toolbar')).toBeInTheDocument();
  });
});

describe('게이지 미리보기 (AC-41)', () => {
  it('Store 대표값 경로가 아니어도 미리보기에서 값 글자를 끌 수 있다', () => {
    const { container } = open({
      id: 'p1',
      type: 'gauge',
      title: 'g',
      config: { channelName: 'c' },
    });

    expect(screen.getByTestId('gauge-preview-wrapper')).toBeInTheDocument();
    // 표식이 있는 것만으로는 부족하다 — 미니 프리뷰도 진짜 GaugePanel 을 쓰므로 표식은
    // 늘 있었다(그래서 이 단언만으로는 오탐이 났다). 실제로 **끌 수 있는가**를 본다:
    // 드래그 레이어가 켜지면 대상에 cursor-move 를 거는 클래스가 붙는다.
    const layer = screen.getByTestId('gauge-drag-layer');
    expect(layer.className).toContain('cursor-move');
    void container;
  });

  // 게이지 패널은 뿌리에 `flex-1` 을 걸어 부모가 준 높이를 채운다. 그 부모가 flex
  // 컨테이너가 아니면 `flex-1` 이 아무 뜻도 갖지 못해 높이가 **내용**으로 정해지고,
  // 게이지 내용의 높이는 SVG 의 고유 비율이라 유형마다 달라진다 — 반원(240×140)에서
  // 패널이 미리보기 상자보다 짧아져 그리드가 일부만 덮이고 도형의 이동 범위가 위로
  // 치우쳤다. jsdom 은 레이아웃을 하지 않으므로 높이 대신 **이 계약**을 잠근다.
  it('패널을 담는 상자는 flex 컬럼이다 — 아니면 높이가 게이지 유형에 휘둘린다', () => {
    open({ id: 'p1', type: 'gauge', title: 'g', config: { channelName: 'c', gaugeType: 'half' } });

    const panelRoot = screen.getByTestId('gauge-drag-layer').closest('[class*="rounded-2xl"]');
    const holder = panelRoot?.parentElement;
    expect(holder).not.toBeNull();
    expect(holder!.className).toContain('flex');
    expect(holder!.className).toContain('flex-col');
    expect(holder!.className).toContain('flex-1');
  });
});

describe('미리보기에서 실제로 끌린다 (AC-41)', () => {
  function stub(el: Element, left: number, top: number, w: number, h: number): void {
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
      left,
      top,
      width: w,
      height: h,
      right: left + w,
      bottom: top + h,
      x: left,
      y: top,
      toJSON: () => ({}),
    } as DOMRect);
  }

  function pointer(type: string, x: number, y: number): PointerEvent {
    return new MouseEvent(type, {
      clientX: x,
      clientY: y,
      bubbles: true,
    }) as unknown as PointerEvent;
  }

  it('통계 본값을 끌면 미리보기가 따라온다', async () => {
    const { container } = open({
      id: 'p1',
      type: 'stat',
      title: 's',
      config: { channel_name: 'c' },
    });

    const bounds = container.querySelector('[data-panel-bounds]')!;
    const value = screen.getByTestId('stat-value');
    stub(bounds, 0, 0, 200, 100);
    stub(value, 75, 40, 50, 20);

    fireEvent(value, pointer('pointerdown', 100, 50));
    fireEvent(document, pointer('pointermove', 140, 50));
    await act(async () => {
      await new Promise<void>((r) => requestAnimationFrame(() => r()));
    });

    // 40px / 폭 200 = 20%p. 격자(10%)에 붙어도 0 은 아니다.
    expect(screen.getByTestId('stat-value').style.left).not.toBe('');
  });
});
