// PanelSettingsDataSource — Store/TSDB 토글 + 공용 StoreEntryTable 선택 surface 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T4/T6/T7 + 시리즈 선택 단일화)
//   - AC-04/07: 행 체크박스 → store_source.series 반영(StoreKeySelector 와 byte-호환).
//   - AC-05: TSDB placeholder + Store 설정 파괴 없음.
//   - AC-06/09: 공용 StoreEntryTable, actions 대신 Alias 컬럼.
//   - AC-08: 필터/정렬/표시숨김 패널별 localStorage 영속/복원.
//   - AC-10: 빈 store / 미선택 에이전트 graceful.
//   - AC-15: 선택 상한(series 개수) 가드.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';
import { pickSeriesColor } from './panels/charts/chartChannelTypes';

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
  // Store 모드지만 에이전트 미선택 — 선택 테이블은 렌더되되 no-agent 안내를 보인다.
  return {
    id: 'p1',
    type: 'heatmap',
    title: 'h',
    config: { data_source: 'store' },
  } as unknown as PanelConfig;
}

beforeEach(() => {
  window.localStorage.clear();
  state.agents = [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }];
  state.keyObjects = [
    { key: 'k1', registration: 'auto', data_type: 'float', metric_type: 'temperature', tags: { room: '1' } },
    { key: 'k2', registration: 'auto', data_type: 'int', metric_type: 'humidity', tags: {} },
  ];
});

describe('T4 — 단일 데이터소스 토글 [채널 | Store | TSDB]', () => {
  it('데이터 소스 토글은 하나만 존재하고 Store 모드에 공용 선택 테이블을 렌더한다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    // 단일 "데이터 소스" 토글(StoreSourceSection 소유) — 채널/Store/TSDB 3옵션.
    expect(screen.getByTestId('chart-data-source-channel')).toBeInTheDocument();
    expect(screen.getByTestId('chart-data-source-store')).toBeInTheDocument();
    expect(screen.getByTestId('chart-data-source-tsdb')).toBeInTheDocument();
    // 중복 래퍼 토글/헤딩 제거 확인.
    expect(screen.queryByTestId('panel-datasource-store')).not.toBeInTheDocument();
    expect(screen.queryByTestId('panel-datasource-tsdb')).not.toBeInTheDocument();
    // Store 모드(config.data_source='store') → 공용 선택 테이블 렌더.
    const select = screen.getByTestId('panel-store-select');
    expect(within(select).getByRole('table')).toBeInTheDocument();
  });

  it('TSDB 선택 시 placeholder + 선택 테이블 미노출 + Store config 보존(AC-05)', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    // 후속 SPEC 안내 placeholder.
    expect(screen.getByTestId('chart-data-source-tsdb-placeholder')).toBeInTheDocument();
    expect(screen.getByText('dashboard.settings.dataSourceTsdbBody')).toBeInTheDocument();
    // 선택 테이블 미노출(Store 모드 아님).
    expect(screen.queryByTestId('panel-store-select')).not.toBeInTheDocument();
    // TSDB 는 UI 전용 — config 미변경(Store 설정 보존).
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('채널 모드에서는 공용 선택 테이블을 렌더하지 않는다', () => {
    const channelPanel = {
      id: 'p1',
      type: 'line-chart',
      title: 'l',
      config: { data_source: 'channel' },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={channelPanel} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-data-source-channel')).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByTestId('panel-store-select')).not.toBeInTheDocument();
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

describe('T6/AC-07 — 행 체크박스 → series (StoreKeySelector byte-호환)', () => {
  it('체크박스 토글이 store_source.series 에 byte-호환 항목으로 추가된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    expect(checkboxes.length).toBe(2);
    fireEvent.click(checkboxes[0]!);
    // 구 StoreKeySelector.onChange 가 만들던 항목과 동일 형태(key/metric_type/tags/data_type/alias/color).
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: expect.objectContaining({
        series: [
          {
            key: 'k1',
            metric_type: 'temperature',
            tags: { room: '1' },
            data_type: 'float',
            alias: 'k1',
            color: pickSeriesColor(0),
          },
        ],
      }),
    });
  });

  it('태그 없는 키는 tags/metric 이 undefined 로 생략된 항목이 된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    // 두 번째 행 = k2 (humidity, 태그 없음).
    fireEvent.click(checkboxes[1]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: expect.objectContaining({
        series: [
          {
            key: 'k2',
            metric_type: 'humidity',
            tags: undefined,
            data_type: 'int',
            alias: 'k2',
            color: pickSeriesColor(0),
          },
        ],
      }),
    });
  });

  it('이미 series 에 있는 행은 체크 상태로 렌더되고 토글 시 series 에서 제거된다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({
          series: [{ key: 'k1', metric_type: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const checkboxes = screen.getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    // k1(첫 행)은 체크, k2 는 미체크.
    expect(checkboxes[0]!.checked).toBe(true);
    expect(checkboxes[1]!.checked).toBe(false);
    fireEvent.click(checkboxes[0]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: expect.objectContaining({ series: [] }),
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

describe('AC-15 — 선택 상한 가드 (series 개수 기준)', () => {
  it('series 상한 초과 추가 시 안내가 표시되고 config 에 반영되지 않는다', () => {
    // 50개 키, 이미 48개(상한) series 선택된 상태.
    state.keyObjects = Array.from({ length: 50 }, (_, i) => ({
      key: `k${i}`,
      registration: 'auto',
      data_type: 'float',
      metric_type: 'm',
      tags: {},
    }));
    // series 항목은 seriesId(key,'m',{}) 로 매칭되도록 key/metric_type 만 채워도 충분.
    const series = Array.from({ length: 48 }, (_, i) => ({ key: `k${i}`, metric_type: 'm' }));
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({ series })}
        onConfigChange={onConfigChange}
      />,
    );
    const select = screen.getByTestId('panel-store-select');
    const checkboxes = within(select).getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    const unchecked = checkboxes.find((c) => !c.checked)!;
    fireEvent.click(unchecked);
    // 안내 표시 + 반영 억제.
    expect(screen.getByTestId('panel-store-select-overlimit')).toBeInTheDocument();
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('상한 미만에서는 정상적으로 추가되고 안내가 없다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    const checkboxes = within(select).getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    fireEvent.click(checkboxes[0]!);
    expect(screen.queryByTestId('panel-store-select-overlimit')).not.toBeInTheDocument();
    expect(onConfigChange).toHaveBeenCalled();
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
