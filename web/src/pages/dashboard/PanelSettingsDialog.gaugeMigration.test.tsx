// 게이지 레거시 바인딩 → 공용 Store 데이터 소스 이관 액션 통합 테스트.
//
// @spec SPEC-CHART-002 §2.8 [E2] / AC-20 — M5.7
//
// 순수 변환 로직은 `panels/charts/gaugeLegacyBinding.test.ts` 가 전수로 잠근다. 이
// 파일은 그 로직이 **설정 화면의 실제 조작 경로에 연결되어 있는가**를 본다.
//
// 검증 축 4개(AC-20):
//   1. 액션이 data_source:'store' + store_source + series_reduce:'last' 를 기록한다
//   2. 이관 후 config.dataSources 가 이관 전과 deep-equal 이다(비파괴)
//   3. 채널 모드로 되돌리면 레거시 경로가 다시 유효해진다(롤백 계획의 근거)
//   4. 유효한 store 바인딩이 없으면 액션이 비활성이고 안내를 표시한다
//
// 3번은 설정 화면이 아니라 GaugePanel 의 값 해석 결과로만 증명할 수 있으므로, 같은
// 파일 안에서 이관 결과 config 를 실제 패널에 넣어 렌더한다. "되돌릴 수 있다"는 것이
// 이관을 비파괴로 만든 유일한 이유이기 때문에(§4.5) 액션 테스트와 붙여 둔다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'gauge', title: '게이지', config: {} } as PanelConfig,
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
  useAgents: () => ({
    data: { data: [{ id: 'a1', name: 'store-a', type: 'store' }] },
  }),
  useAgent: () => ({ data: undefined }),
}));

vi.mock('@/hooks/useFlow', () => ({ useFlows: () => ({ data: { data: [] } }) }));

vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

// 레거시 store 폴링(useStoreLatestValue)의 POST 를 가로챈다. 되돌리기 검증에서
// 레거시 값이 실제로 다시 흘러 들어오는지 보려면 실제 훅을 살려 둬야 한다.
const mockPost = vi.hoisted(() => ({
  fn: vi.fn(async (_url: string, _body: unknown) => ({
    entries: [] as Array<{ value: unknown; timestamp: number }>,
  })),
}));
vi.mock('@/services/api/client', () => ({
  post: (url: string, body: unknown) => mockPost.fn(url, body),
}));

// chart-emitter 구독은 이 파일 범위 밖이므로 idle 로 고정한다.
vi.mock('./panels/charts/useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle' as const,
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';
import GaugePanel from './panels/GaugePanel';

/** F2 의 store 레거시 바인딩(acceptance.md 공통 픽스처). */
const F2_STORE_BINDING = {
  sourceType: 'store',
  storeAgentId: 'a1',
  storeAgent: 'store-a',
  storeKey: 'k1',
  storeNamespace: 'default',
} as const;

/** 이관 전 게이지 config — 레거시 바인딩만 갖고 신규 필드는 없다. */
function legacyGaugeConfig(
  dataSources: readonly Record<string, unknown>[] = [{ ...F2_STORE_BINDING }],
): Record<string, unknown> {
  return {
    gaugeType: 'simple',
    min: 0,
    max: 100,
    unit: '%',
    value: 7, // 바인딩이 값을 못 낼 때 렌더되는 static 값
    dataSources: dataSources.map((d) => ({ ...d })),
  };
}

const MIGRATE_TESTID = 'gauge-migrate-to-store';
const HINT_TESTID = 'gauge-migrate-to-store-hint';
const APPLY_LABEL = 'dashboard.settings.apply';

async function renderDialog(config: Record<string, unknown>) {
  storeMock.panel = { id: 'p1', type: 'gauge', title: '게이지', config };
  await act(async () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  });
}

/** 이관 액션 클릭 → 저장까지 수행하고 스토어에 승격된 config 를 돌려준다. */
async function migrateAndApply(config: Record<string, unknown>) {
  await renderDialog(config);
  await act(async () => {
    fireEvent.click(screen.getByTestId(MIGRATE_TESTID));
  });
  await act(async () => {
    fireEvent.click(screen.getByText(APPLY_LABEL));
  });
  expect(storeMock.updatePanelConfig).toHaveBeenCalled();
  const calls = storeMock.updatePanelConfig.mock.calls;
  const next = calls[calls.length - 1]![1] as Record<string, unknown>;
  // 설정 다이얼로그는 라이브 미리보기로 GaugePanel 을 한 번 더 마운트한다. 되돌리기
  // 검증은 그 뒤에 패널을 단독 렌더하므로, 두 트리가 겹쳐 조회가 모호해지지 않도록
  // 여기서 언마운트한다.
  cleanup();
  return next;
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  mockPost.fn.mockReset();
  mockPost.fn.mockImplementation(async () => ({ entries: [] }));
  window.localStorage.clear();
});

describe('게이지 이관 액션 (AC-20)', () => {
  it("이관 액션이 store_source + series_reduce:'last' + data_source:'store' 를 기록한다", async () => {
    const next = await migrateAndApply(legacyGaugeConfig());

    expect(next.data_source).toBe('store');
    expect(next.series_reduce).toBe('last');
    expect(next.store_source).toEqual({
      agent_id: 'a1',
      agent_name: 'store-a',
      namespace: 'default',
      selection_mode: 'keys',
      series: [{ key: 'k1' }],
      time_window_ms: 60 * 60 * 1000,
      interval_ms: 60 * 1000,
      aggregation: 'average',
      refresh_interval_ms: 5000,
    });
  });

  it('이관 후에도 config.dataSources 가 보존된다(비파괴)', async () => {
    const before = legacyGaugeConfig();
    const beforeSnapshot = structuredClone(before.dataSources);

    const next = await migrateAndApply(before);

    expect(next.dataSources).toEqual(beforeSnapshot);
    // 얕은 병합이므로 이관과 무관한 다른 키도 그대로 남아야 한다.
    expect(next.value).toBe(7);
    expect(next.gaugeType).toBe('simple');
  });

  it('resource/flow/chart-emitter 가 섞여 있어도 첫 유효 store 항목만 이관되고 배열 전체가 보존된다', async () => {
    const before = legacyGaugeConfig([
      { sourceType: 'resource', resource: 'cpu' },
      { sourceType: 'flow', flowId: 'f1', dataField: 'x' },
      { ...F2_STORE_BINDING },
      { sourceType: 'store', storeAgent: 'store-b', storeKey: 'k2' },
    ]);
    const beforeSnapshot = structuredClone(before.dataSources);

    const next = await migrateAndApply(before);

    expect(
      (next.store_source as { series: { key: string }[] }).series,
    ).toEqual([{ key: 'k1' }]);
    expect(next.dataSources).toEqual(beforeSnapshot);
  });

  it('유효한 store 바인딩이 없으면 액션이 비활성이고 안내를 표시한다', async () => {
    await renderDialog(
      legacyGaugeConfig([
        { sourceType: 'resource', resource: 'cpu' },
        { sourceType: 'flow', flowId: 'f1', dataField: 'x' },
        { sourceType: 'chart-emitter', channelName: 'ch1' },
      ]),
    );

    const button = screen.getByTestId(MIGRATE_TESTID);
    expect(button).toBeDisabled();
    expect(screen.getByTestId(HINT_TESTID)).toHaveTextContent(
      'dashboard.settings.gaugeSection.migrateToStoreDisabled',
    );
  });

  it('storeKey 가 빠진 반쪽 store 바인딩도 이관 대상이 아니다', async () => {
    await renderDialog(
      legacyGaugeConfig([{ sourceType: 'store', storeAgent: 'store-a' }]),
    );

    expect(screen.getByTestId(MIGRATE_TESTID)).toBeDisabled();
  });

  it('이관 가능한 config 에서는 액션이 활성이고 비파괴 안내를 표시한다', async () => {
    await renderDialog(legacyGaugeConfig());

    expect(screen.getByTestId(MIGRATE_TESTID)).toBeEnabled();
    expect(screen.getByTestId(HINT_TESTID)).toHaveTextContent(
      'dashboard.settings.gaugeSection.migrateToStoreHint',
    );
  });

  it('액션을 누르기 전에는 config 를 조용히 다시 쓰지 않는다', async () => {
    // §2.8 [E2]: "시스템은 저장된 config 를 자동으로 조용히 다시 쓰지 않아야 한다."
    await renderDialog(legacyGaugeConfig());
    await act(async () => {
      fireEvent.click(screen.getByText(APPLY_LABEL));
    });

    const applied = storeMock.updatePanelConfig.mock.calls.at(-1)![1] as Record<
      string,
      unknown
    >;
    expect(applied.data_source).toBeUndefined();
    expect(applied.store_source).toBeUndefined();
    expect(applied.series_reduce).toBeUndefined();
  });
});

describe('이관 결과 config 의 되돌리기 (AC-20 / §4.5 롤백 계획)', () => {
  /** 레거시 폴링이 값 33 을 돌려주도록 응답을 세팅한다. */
  function respondWithLegacyValue(value: unknown) {
    mockPost.fn.mockImplementation(async () => ({
      entries: [{ value, timestamp: 1 }],
    }));
  }

  /** 패널만 단독 렌더하고, 그 서브트리로 스코프된 조회기를 돌려준다. */
  async function renderGauge(config: Record<string, unknown>) {
    let view!: ReturnType<typeof render>;
    await act(async () => {
      view = render(<GaugePanel panelId="p1" title="게이지" config={config} />);
    });
    // 마운트 직후 폴링의 마이크로태스크를 비운다.
    await act(async () => {
      await Promise.resolve();
    });
    return within(view.container);
  }

  it('이관 전에는 레거시 store 폴링 값이 표시된다', async () => {
    respondWithLegacyValue(33);
    const panel = await renderGauge(legacyGaugeConfig());

    expect(panel.getByText('33')).toBeInTheDocument();
    expect(mockPost.fn).toHaveBeenCalled();
  });

  it('이관 후에는 store-source 경로가 이기고 레거시 폴링이 멈춘다', async () => {
    respondWithLegacyValue(33);
    const migrated = await migrateAndApply(legacyGaugeConfig());
    mockPost.fn.mockClear();

    const panel = await renderGauge(migrated);

    // 신규 경로가 이기면 레거시 훅은 idle 이므로 폴링이 발생하지 않는다.
    expect(mockPost.fn).not.toHaveBeenCalled();
    // 레거시 값(33)도 static 값(7)도 표시되지 않는다 — 신규 경로의 빈 상태다(§2.4).
    expect(panel.queryByText('33')).toBeNull();
    expect(panel.queryByText('7')).toBeNull();
    expect(panel.getByText('--')).toBeInTheDocument();
  });

  it("채널 모드로 되돌리면 레거시 경로가 다시 유효해진다", async () => {
    respondWithLegacyValue(33);
    const migrated = await migrateAndApply(legacyGaugeConfig());
    mockPost.fn.mockClear();
    respondWithLegacyValue(33);

    // 사용자가 데이터 소스 토글을 'channel' 로 되돌린 상태.
    const reverted: Record<string, unknown> = { ...migrated, data_source: 'channel' };
    const panel = await renderGauge(reverted);

    expect(panel.getByText('33')).toBeInTheDocument();
    expect(mockPost.fn).toHaveBeenCalled();
    // 되돌린 config 에는 신규 필드가 그대로 남아 있다 — 삭제하지 않는다(§2.10 [S2]).
    expect(reverted.series_reduce).toBe('last');
    expect(reverted.store_source).toBeDefined();
  });

  it('되돌린 config 를 다시 store 로 바꾸면 신규 경로가 즉시 되살아난다', async () => {
    const migrated = await migrateAndApply(legacyGaugeConfig());
    const reverted = { ...migrated, data_source: 'channel' };
    const reapplied = { ...reverted, data_source: 'store' };

    respondWithLegacyValue(33);
    const panel = await renderGauge(reapplied);

    expect(panel.queryByText('33')).toBeNull();
    expect(panel.getByText('--')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// M6 추가 (A) — 이관 액션 재실행 가드(통합).
//
// 순수 판정은 `panels/charts/gaugeLegacyBinding.test.ts` 의
// `resolveGaugeMigrationState` 전수 테스트가 잠근다. 여기서는 그 판정이 실제 버튼의
// 비활성 상태와 안내 문구에 연결되어 있는지, 그리고 재클릭으로 사용자의
// `store_source` 손질이 사라지지 않는지를 본다.
// ---------------------------------------------------------------------------

/** 이관을 마치고 사용자가 시리즈·시간창을 손질한 뒤의 config. */
function customizedAfterMigration(): Record<string, unknown> {
  return {
    ...legacyGaugeConfig(),
    data_source: 'store',
    series_reduce: 'max',
    store_source: {
      agent_id: 'a1',
      agent_name: 'store-a',
      namespace: 'default',
      selection_mode: 'keys',
      // 사용자가 직접 늘린 시리즈 — 이관 기본값(1개)과 명확히 다르다.
      series: [{ key: 'k1' }, { key: 'k2' }, { key: 'k3' }],
      // 사용자가 직접 늘린 시간창 — 이관 기본값(1시간)과 명확히 다르다.
      time_window_ms: 24 * 60 * 60 * 1000,
      interval_ms: 5 * 60 * 1000,
      aggregation: 'max',
      refresh_interval_ms: 5000,
    },
  };
}

describe('이관 액션 재실행 가드 (M6 / §2.8 [E2] 비파괴)', () => {
  it('이관 직후 액션이 비활성이 되고 "이미 이전됨" 안내로 바뀐다', async () => {
    await renderDialog(legacyGaugeConfig());
    expect(screen.getByTestId(MIGRATE_TESTID)).toBeEnabled();

    await act(async () => {
      fireEvent.click(screen.getByTestId(MIGRATE_TESTID));
    });

    const button = screen.getByTestId(MIGRATE_TESTID);
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('data-migration-state', 'already-migrated');
    expect(screen.getByTestId(HINT_TESTID)).toHaveTextContent(
      'dashboard.settings.gaugeSection.migrateToStoreDone',
    );
  });

  it('레거시 바인딩이 보존되어 있어도 재실행은 잠긴다', async () => {
    // 보존이 곧 재실행 위험이다 — findMigratableGaugeStoreBinding 은 이관 후에도 계속
    // 같은 항목을 찾아내므로, 그 함수만으로 활성 여부를 정하면 버튼이 영구히 활성이다.
    const config = customizedAfterMigration();
    expect(config.dataSources).toBeDefined();

    await renderDialog(config);

    expect(screen.getByTestId(MIGRATE_TESTID)).toBeDisabled();
    expect(screen.getByTestId(HINT_TESTID)).toHaveTextContent(
      'dashboard.settings.gaugeSection.migrateToStoreDone',
    );
  });

  it('재클릭해도 사용자가 손질한 store_source 가 기본값으로 덮어써지지 않는다', async () => {
    const before = customizedAfterMigration();
    const beforeStore = structuredClone(before.store_source);

    await renderDialog(before);
    await act(async () => {
      fireEvent.click(screen.getByTestId(MIGRATE_TESTID));
    });
    await act(async () => {
      fireEvent.click(screen.getByText(APPLY_LABEL));
    });

    const applied = storeMock.updatePanelConfig.mock.calls.at(-1)![1] as Record<
      string,
      unknown
    >;
    expect(applied.store_source).toEqual(beforeStore);
    expect(applied.series_reduce).toBe('max');
    // 레거시 바인딩도 그대로다 — 가드는 무언가를 지우는 것이 아니라 쓰지 않는 것이다.
    expect(applied.dataSources).toEqual(legacyGaugeConfig().dataSources);
  });

  it('store 모드지만 시리즈를 아직 고르지 않았으면 액션이 다시 활성이다', async () => {
    // 덮어쓸 사용자 설정이 없는 상태 — 여기서까지 잠그면 토글만 눌러 본 사용자가
    // 이관 경로를 영영 잃는다.
    await renderDialog({
      ...legacyGaugeConfig(),
      data_source: 'store',
      store_source: {
        agent_name: 'store-a',
        namespace: 'default',
        selection_mode: 'keys',
        series: [],
        time_window_ms: 60 * 60 * 1000,
        interval_ms: 60 * 1000,
        aggregation: 'average',
      },
    });

    const button = screen.getByTestId(MIGRATE_TESTID);
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute('data-migration-state', 'available');
  });
});
