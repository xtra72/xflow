// PanelSettingsDialog — 통계/바/파이의 채널 섹션 제거 + 채널 모드 자동 이관.
//
// 이 4종(통계/게이지/바/파이)은 store/tsdb 로 일원화됐다. 통계/바/파이는 단일 channel_name
// 편집 섹션을 노출하지 않으며, 채널 모드로 저장된 구 패널은 설정 진입 시 store 로 옮겨
// 편집 가능한 상태가 된다. 게이지는 레거시 dataSources 편집기가 그대로 있어 제외된다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'stat', title: '통계', config: {} } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [
      { id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] },
    ],
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
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] } }),
  useAgent: () => ({ data: undefined }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

// 게이지의 레거시 dataSources 편집기가 쓰는 의존성 — 이 파일은 렌더 경로만 보므로 고정한다.
vi.mock('@/hooks/useFlow', () => ({ useFlows: () => ({ data: { data: [] } }) }));
vi.mock('@/services/api/client', () => ({
  post: async () => ({ entries: [] as Array<{ value: unknown; timestamp: number }> }),
}));
vi.mock('./panels/charts/useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle' as const,
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

const CHANNEL_SECTION_LABEL = 'dashboard.settings.channel';
const APPLY_LABEL = 'dashboard.settings.apply';

async function renderDialog(type: string, config: Record<string, unknown>) {
  storeMock.panel = { id: 'p1', type, title: '패널', config } as unknown as PanelConfig;
  await act(async () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  });
}

/** 저장 버튼을 눌러 스토어에 승격된 config 를 돌려준다. */
async function apply(): Promise<Record<string, unknown>> {
  await act(async () => {
    fireEvent.click(screen.getByText(APPLY_LABEL));
  });
  const calls = storeMock.updatePanelConfig.mock.calls;
  expect(calls.length).toBeGreaterThan(0);
  return calls[calls.length - 1]![1] as Record<string, unknown>;
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  cleanup();
});

describe('채널 섹션 노출 대상', () => {
  for (const type of ['stat', 'bar-chart', 'pie-chart', 'gauge', 'table'] as const) {
    it(`${type}: 단일 channel_name 편집 섹션을 노출하지 않는다`, async () => {
      await renderDialog(type, { channel_name: 'c1' });
      expect(screen.queryByText(CHANNEL_SECTION_LABEL)).toBeNull();
    });
  }

  it('table: 채널 모드 config 로 열어도 store 로 이관되어 채널 섹션이 없다', async () => {
    await renderDialog('table', { channel_name: 'c1' });
    expect(screen.queryByText(CHANNEL_SECTION_LABEL)).toBeNull();
    expect((await apply()).data_source).toBe('store');
  });
});

describe('차트 배열 행 수 설정 노출 대상', () => {
  // 타일 배열을 쓰는 패널에만 의미가 있다 — 바/파이는 시리즈를 한 차트 안의 막대·조각으로
  // 그리므로 "행" 개념이 없다.
  for (const type of ['stat', 'gauge'] as const) {
    it(`${type}: 행 수 컨트롤을 노출한다`, async () => {
      await renderDialog(type, { data_source: 'store' });
      expect(screen.getByTestId('chart-tile-rows')).toBeInTheDocument();
    });

    it(`${type}: 미지정이면 기본값 1 을 표시한다`, async () => {
      await renderDialog(type, { data_source: 'store' });
      expect((screen.getByTestId('chart-tile-rows') as HTMLInputElement).value).toBe('1');
    });

    it(`${type}: 저장된 값을 표시한다`, async () => {
      await renderDialog(type, { data_source: 'store', tile_rows: 3 });
      expect((screen.getByTestId('chart-tile-rows') as HTMLInputElement).value).toBe('3');
    });
  }

  for (const type of ['bar-chart', 'pie-chart', 'graph-chart', 'table'] as const) {
    it(`${type}: 행 수 컨트롤을 노출하지 않는다`, async () => {
      await renderDialog(type, { data_source: 'store' });
      expect(screen.queryByTestId('chart-tile-rows')).toBeNull();
    });
  }

  it('기본값(1)로 되돌리면 config 키를 남기지 않는다', async () => {
    await renderDialog('stat', { data_source: 'store', tile_rows: 3 });
    await act(async () => {
      fireEvent.change(screen.getByTestId('chart-tile-rows'), { target: { value: '1' } });
    });
    expect((await apply()).tile_rows).toBeUndefined();
  });

  it('상한을 넘는 입력은 잘라서 저장한다', async () => {
    await renderDialog('stat', { data_source: 'store' });
    await act(async () => {
      fireEvent.change(screen.getByTestId('chart-tile-rows'), { target: { value: '99' } });
    });
    expect((await apply()).tile_rows).toBe(12);
  });
});

describe('채널 모드 → store 자동 이관', () => {
  for (const type of ['stat', 'bar-chart', 'pie-chart'] as const) {
    it(`${type}: data_source 미지정(구 패널)이 store 로 이관된다`, async () => {
      await renderDialog(type, { channel_name: 'c1', display_field: 'value' });
      const applied = await apply();
      expect(applied.data_source).toBe('store');
      expect(applied.store_source).toMatchObject({ agent_name: '', series: [] });
    });

    it(`${type}: data_source: 'channel' 도 이관된다`, async () => {
      await renderDialog(type, { channel_name: 'c1', data_source: 'channel' });
      expect((await apply()).data_source).toBe('store');
    });

    it(`${type}: channel_name 은 지우지 않는다(시리즈 선택 전까지 채널 값이 계속 보인다)`, async () => {
      await renderDialog(type, { channel_name: 'c1' });
      expect((await apply()).channel_name).toBe('c1');
    });

    it(`${type}: 남아 있던 store_source 를 기본형으로 덮지 않는다`, async () => {
      const kept = {
        agent_name: 'store-1',
        namespace: 'default',
        series: [{ key: 'room:temp' }],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      };
      await renderDialog(type, { data_source: 'channel', store_source: kept });
      expect((await apply()).store_source).toEqual(kept);
    });

    it(`${type}: 이미 store 면 다시 쓰지 않는다`, async () => {
      const store = {
        agent_name: 'store-1',
        series: [{ key: 'room:temp' }],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      };
      await renderDialog(type, { data_source: 'store', store_source: store });
      const applied = await apply();
      expect(applied.data_source).toBe('store');
      expect(applied.store_source).toEqual(store);
    });

    it(`${type}: tsdb 모드는 건드리지 않는다`, async () => {
      await renderDialog(type, { data_source: 'tsdb' });
      expect((await apply()).data_source).toBe('tsdb');
    });
  }

  it('게이지는 이관 대상이 아니다 — 레거시 dataSources 편집기가 남아 있어 갇히지 않는다', async () => {
    // SPEC-CHART-002 §2.8 [E2] "저장된 config 를 자동으로 조용히 다시 쓰지 않는다"(AC-20).
    await renderDialog('gauge', {
      gaugeType: 'simple',
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1' }],
    });
    const applied = await apply();
    expect(applied.data_source).toBeUndefined();
    expect(applied.store_source).toBeUndefined();
  });

  it('저장하지 않으면 스토어에 아무것도 쓰지 않는다(draft 전용)', async () => {
    await renderDialog('stat', { channel_name: 'c1' });
    expect(storeMock.updatePanelConfig).not.toHaveBeenCalled();
  });
});
