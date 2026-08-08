// PanelSettingsDataSource — Store/TSDB 토글 + 공용 StoreEntryTable 선택 surface 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T4/T6/T7)
//   - AC-04/07: 행 체크박스 → store_source.selected_keys 반영.
//   - AC-05: TSDB placeholder + Store 설정 파괴 없음.
//   - AC-06/09: 공용 StoreEntryTable, actions 대신 Alias 컬럼.
//   - AC-08: 필터/정렬/표시숨김 패널별 localStorage 영속/복원.
//   - AC-10: 빈 store / 미선택 에이전트 graceful.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';

const state = vi.hoisted(() => ({
  agents: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] as Array<{
    id: string;
    name: string;
    type: string;
  }>,
  keyObjects: [
    { key: 'k1', registration: 'auto', data_type: 'float', metric_type: 'temperature', tags: { room: '1' } },
    { key: 'k2', registration: 'auto', data_type: 'int', metric_type: 'humidity', tags: {} },
  ] as unknown[],
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: state.agents } }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({
    data: { keys: [], tags: {}, keyObjects: state.keyObjects },
    isLoading: false,
    isError: false,
  }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import { PanelSettingsDataSource } from './PanelSettingsDataSource';

const STORE_SOURCE = {
  agent_id: 'store-uuid-1',
  agent_name: 'store-1',
  series: [],
  time_window_ms: 60000,
  interval_ms: 5000,
  aggregation: 'last',
};

function panelWithAgent(extra: Record<string, unknown> = {}): PanelConfig {
  return {
    id: 'p1',
    type: 'heatmap',
    title: 'h',
    config: { data_source: 'store', store_source: { ...STORE_SOURCE, ...extra } },
  } as unknown as PanelConfig;
}

function emptyPanel(): PanelConfig {
  return { id: 'p1', type: 'heatmap', title: 'h', config: {} } as unknown as PanelConfig;
}

beforeEach(() => {
  window.localStorage.clear();
  state.agents = [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }];
  state.keyObjects = [
    { key: 'k1', registration: 'auto', data_type: 'float', metric_type: 'temperature', tags: { room: '1' } },
    { key: 'k2', registration: 'auto', data_type: 'int', metric_type: 'humidity', tags: {} },
  ];
});

describe('T4 — Store/TSDB 토글', () => {
  it('기본 Store 모드는 기존 편집기 + 공용 선택 테이블을 렌더한다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('panel-datasource-store')).toHaveAttribute('aria-selected', 'true');
    // 기존 StoreSourceSection(채널/Store 토글)이 보존된다.
    expect(screen.getByTestId('chart-data-source-store')).toBeInTheDocument();
    // 공용 선택 테이블이 렌더된다.
    const select = screen.getByTestId('panel-store-select');
    expect(within(select).getByRole('table')).toBeInTheDocument();
  });

  it('TSDB 선택 시 placeholder 만 렌더하고 config 를 건드리지 않는다(AC-05)', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('panel-datasource-tsdb'));
    expect(screen.getByTestId('panel-datasource-tsdb-placeholder')).toBeInTheDocument();
    // 안내 문구 노출 + Store 편집기/테이블 미노출.
    expect(screen.getByText('dashboard.settings.dataSourceTsdbBody')).toBeInTheDocument();
    expect(screen.queryByTestId('panel-store-select')).not.toBeInTheDocument();
    expect(screen.queryByTestId('chart-data-source-store')).not.toBeInTheDocument();
    // 토글은 로컬 UI 상태 — config 파괴 없음.
    expect(onConfigChange).not.toHaveBeenCalled();
  });
});

describe('T6/AC-06/09 — 공용 테이블 + Alias 컬럼', () => {
  it('actions 대신 Alias 컬럼을 렌더한다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('agents.detail.store.colAlias')).toBeInTheDocument();
    expect(within(table).queryByText('agents.detail.store.colActions')).not.toBeInTheDocument();
  });
});

describe('T6/AC-07 — 행 체크박스 → selected_keys', () => {
  it('체크박스 토글이 store_source.selected_keys 로 반영된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    expect(checkboxes.length).toBe(2);
    fireEvent.click(checkboxes[0]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: expect.objectContaining({ selected_keys: ['k1'] }),
    });
  });

  it('이미 선택된 키는 체크 상태로 렌더되고 토글 시 제거된다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({ selected_keys: ['k1'] })}
        onConfigChange={onConfigChange}
      />,
    );
    const checkboxes = screen.getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    expect(checkboxes[0]!.checked).toBe(true);
    fireEvent.click(checkboxes[0]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: expect.objectContaining({ selected_keys: [] }),
    });
  });
});

describe('AC-10 — 빈 store graceful', () => {
  it('에이전트 미선택 시 안내 문구(throw 없음)', () => {
    render(<PanelSettingsDataSource panel={emptyPanel()} onConfigChange={vi.fn()} />);
    expect(
      screen.getByText('dashboard.settings.dataSourceStoreSelectNoAgent'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('에이전트는 있으나 키가 없으면 빈 안내', () => {
    state.keyObjects = [];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    expect(
      screen.getByText('dashboard.settings.dataSourceStoreSelectEmpty'),
    ).toBeInTheDocument();
  });
});

describe('T7/AC-08 — 필터/정렬/표시숨김 영속', () => {
  it('컬럼 숨김이 패널별 localStorage 로 영속되고 재마운트 시 복원된다', () => {
    const { unmount } = render(
      <PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />,
    );
    const select = () => screen.getByTestId('panel-store-select');
    expect(
      within(within(select()).getByRole('table')).getByText('agents.detail.store.colMetric'),
    ).toBeInTheDocument();

    // 컬럼 설정 메뉴에서 metric 숨김.
    fireEvent.click(within(select()).getByTestId('store-columns-settings'));
    const menu = within(select()).getByRole('menu');
    const metricLabel = within(menu)
      .getByText('agents.detail.store.colMetric')
      .closest('label')!;
    fireEvent.click(metricLabel.querySelector('input[type="checkbox"]')!);

    expect(
      within(within(select()).getByRole('table')).queryByText('agents.detail.store.colMetric'),
    ).not.toBeInTheDocument();
    // localStorage 에 hidden 영속.
    const raw = window.localStorage.getItem('panel-settings.storeTable.p1');
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw!).hidden).toContain('metric');

    // 재마운트 → 숨김 복원.
    unmount();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(
      within(table).queryByText('agents.detail.store.colMetric'),
    ).not.toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colKey')).toBeInTheDocument();
  });

  it('키 컬럼 전체 확장 토글이 동작한다(축약↔전체 키)', () => {
    state.keyObjects = [
      { key: 'very-long-key-abcdefgh', registration: 'auto', data_type: 'float', metric_type: 'm', tags: {} },
    ];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    // 축약: 키 컬럼은 앞 8자만 표시(Alias 컬럼은 전체 키를 표시하므로 축약 텍스트로 구분).
    expect(within(select).getByText('very-lon')).toBeInTheDocument();
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyExpandColumnAriaLabel'));
    // 확장: 축약 텍스트가 사라지고 전체 키가 표시된다.
    expect(within(select).queryByText('very-lon')).not.toBeInTheDocument();
    expect(within(select).getAllByText('very-long-key-abcdefgh').length).toBeGreaterThanOrEqual(1);
  });

  it('정렬 상태가 패널별 localStorage 로 영속된다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    // 공용 선택 테이블의 첫 정렬 헤더(key) 클릭 → asc.
    const select = screen.getByTestId('panel-store-select');
    fireEvent.click(within(select).getAllByLabelText('agents.detail.store.sortAriaLabel')[0]!);
    const raw = window.localStorage.getItem('panel-settings.storeTable.p1');
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw!).sort).toEqual({ column: 'key', direction: 'asc' });
  });
});
