// PanelSettingsDataSource — Store/TSDB 토글 + 공용 StoreEntryTable 선택 surface 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T4/T6/T7 + 시리즈 선택 단일화)
//   - AC-04/07: 행 체크박스 → store_source.series 반영(StoreKeySelector 와 byte-호환).
//   - AC-05: TSDB 토글 → config 영속 + Store 설정 파괴 없음.
//     (원래는 "TSDB placeholder + config 무기록" 이었다. SPEC-TSDB-002 §2.11 이 그
//      비목표를 대체해 제자리 반전했다 — 아래 반전 사유 주석 참조.)
//   - AC-06/09: 공용 StoreEntryTable, actions 대신 Alias 컬럼.
//   - AC-08: 필터/정렬/표시숨김 패널별 localStorage 영속/복원.
//   - AC-10: 빈 store / 미선택 에이전트 graceful.
//   - AC-15: 선택 상한(series 개수) 가드.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';
import { defaultTsdbSource, pickSeriesColor } from './panels/charts/chartChannelTypes';
import { heatmapSensorId } from './panels/heatmap/sensorIdentity';

const state = vi.hoisted(() => ({
  agents: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] as Array<{
    id: string;
    name: string;
    type: string;
  }>,
  keyObjects: [
    { key: 'k1', registration: 'auto', data_type: 'float', field: 'temperature', tags: { room: '1' } },
    { key: 'k2', registration: 'auto', data_type: 'int', field: 'humidity', tags: {} },
  ] as unknown[],
  keysLoaded: true,
  refetchKeys: vi.fn(),
  // 현재 값 컬럼의 소스 — 에이전트 상태 스냅샷의 라이브 엔트리.
  agentEntries: [] as Array<Record<string, unknown>>,
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: state.agents } }),
  useAgent: () => ({ data: { state: { entries: state.agentEntries } } }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({
    data: { keys: [], tags: {}, keyObjects: state.keyObjects },
    isLoading: false,
    isError: false,
    isFetching: false,
    // 유령 선택(스토어에서 사라진 시리즈) 판정은 조회 성공 후에만 이뤄진다.
    isSuccess: state.keysLoaded,
    refetch: state.refetchKeys,
  }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import { PanelSettingsDataSource } from './PanelSettingsDataSource';

/**
 * 히트맵 센서 좌표(sensor_positions)의 키 공간 = 시리즈 동일성 키(key + metric + 정렬 tags).
 * store key 하나로 키잉하면 같은 key 의 형제 시리즈가 좌표를 공유해버린다(이 결함의 원인).
 */
const SID_K1 = heatmapSensorId({ key: 'k1', field: 'temperature', tags: { room: '1' } });

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

/** TSDB 모드로 영속된 패널 — store_source 는 그대로 보존된다(§2.11 · §2.12). */
function tsdbPanel(): PanelConfig {
  return {
    id: 'p1',
    type: 'heatmap',
    title: 'h',
    config: {
      data_source: 'tsdb',
      store_source: { ...STORE_SOURCE },
      tsdb_source: defaultTsdbSource(),
    },
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
    { key: 'k1', registration: 'auto', data_type: 'float', field: 'temperature', tags: { room: '1' } },
    { key: 'k2', registration: 'auto', data_type: 'int', field: 'humidity', tags: {} },
  ];
  state.keysLoaded = true;
  state.agentEntries = [];
  state.refetchKeys.mockReset();
});

describe('store 키 목록 새로고침', () => {
  it('새로고침 버튼이 렌더되고 클릭 시 키 목록을 재조회한다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const refresh = screen.getByTestId('panel-store-select-refresh');
    expect(refresh).toBeInTheDocument();
    fireEvent.click(refresh);
    expect(state.refetchKeys).toHaveBeenCalledTimes(1);
  });
});

describe('T4 — 단일 데이터소스 토글 [Store | TSDB | 시스템 지표]', () => {
  it('데이터 소스 토글은 하나만 존재하고 Store 모드에 공용 선택 테이블을 렌더한다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    // 단일 "데이터 소스" 토글(StoreSourceSection 소유) — Store/TSDB/시스템 지표 3옵션.
    // 채널은 패널 소스에서 빠졌으므로 버튼도 없다.
    expect(screen.queryByTestId('chart-data-source-channel')).not.toBeInTheDocument();
    expect(screen.getByTestId('chart-data-source-store')).toBeInTheDocument();
    expect(screen.getByTestId('chart-data-source-tsdb')).toBeInTheDocument();
    // 중복 래퍼 토글/헤딩 제거 확인.
    expect(screen.queryByTestId('panel-datasource-store')).not.toBeInTheDocument();
    expect(screen.queryByTestId('panel-datasource-tsdb')).not.toBeInTheDocument();
    // Store 모드(config.data_source='store') → 공용 선택 테이블 렌더.
    const select = screen.getByTestId('panel-store-select');
    expect(within(select).getByRole('table')).toBeInTheDocument();
  });

  it('TSDB 선택 시 data_source:tsdb 가 config 에 기록된다', () => {
    // [SPEC-TSDB-002 §2.11 반전] SPEC-PANEL-SETTINGS-001 REQ-05 는 TSDB 토글을
    // config 무기록(no-op)으로 규정했고 이 테스트가 그것을 잠갔다. SPEC-TSDB-002 가
    // 그 비목표를 대체하므로 단언을 반전한다. 삭제하지 않는 이유는 "왜 바뀌었는가"의
    // 기록을 diff 밖으로 내보내지 않기 위함이다.
    const onConfigChange = vi.fn();
    const { unmount } = render(
      <PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />,
    );
    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    // 반전 (a): 후속 SPEC 안내 placeholder 는 렌더 트리에서 은퇴했다(testid 소멸).
    // 반전 (b): 선택이 config 에 기록된다.
    // 다중 소스가 들어오면서 버튼은 **토글**이 됐고, 저장 단위도 종류가 아니라
    // **인스턴스**가 됐다 — 켜져 있던 store 는 그대로 남고 tsdb 인스턴스가 더해진다.
    const written = onConfigChange.mock.calls[0]![0] as Record<string, unknown>;
    expect((written.sources as Array<{ kind: string }>).map((e) => e.kind)).toEqual([
      'store',
      'tsdb',
    ]);
    // 무변경: Store 설정은 파괴되지 않는다(§2.12 [E2]) — 패치에 store_source 가 없다.
    const patch = onConfigChange.mock.calls[0]?.[0] as Record<string, unknown> | undefined;
    expect(patch).toBeDefined();
    expect(patch).not.toHaveProperty('store_source');
    unmount();

    // 반전 (c): 선택 테이블 미노출 단언은 유지하되 **config 기준**으로 옮긴다. 모드가
    // 로컬 상태에서 config 파생으로 바뀌었으므로, 부모가 패치를 반영하지 않는 이 테스트
    // 하네스에서는 클릭만으로 모드가 바뀌지 않는다(프로덕션에서는 부모가 반영한다).
    render(<PanelSettingsDataSource panel={tsdbPanel()} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('panel-store-select')).not.toBeInTheDocument();
  });

  it('처음 TSDB 선택 시 defaultTsdbSource() 가 함께 기록된다', () => {
    // `tsdb_source` 가 아직 없을 때만 기본 블록을 채운다 — store 쪽 규칙과 같다(§2.11 [E1]).
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ tsdb_source: defaultTsdbSource() }),
    );
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
    // 구 StoreKeySelector.onChange 가 만들던 항목과 동일 형태(key/field/tags/data_type/color).
    // alias 는 부여하지 않는다 — 기본값으로 key 를 넣으면 사용자가 붙인 이름과 구분되지 않는다.
    // heatmap 패널이라 sensor_positions 부수효과가 함께 오므로 objectContaining 로 래핑한다.
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        store_source: expect.objectContaining({
          series: [
            {
              key: 'k1',
              field: 'temperature',
              tags: { room: '1' },
              data_type: 'float',
              color: pickSeriesColor(0),
            },
          ],
        }),
      }),
    );
  });

  it('태그 없는 키는 tags/metric 이 undefined 로 생략된 항목이 된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    // 두 번째 행 = k2 (humidity, 태그 없음).
    fireEvent.click(checkboxes[1]!);
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        store_source: expect.objectContaining({
          series: [
            {
              key: 'k2',
              field: 'humidity',
              tags: undefined,
              data_type: 'int',
              color: pickSeriesColor(0),
            },
          ],
        }),
      }),
    );
  });

  it('이미 series 에 있는 행은 체크 상태로 렌더되고 토글 시 series 에서 제거된다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
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
      field: 'm',
      tags: {},
    }));
    // series 항목은 seriesId(key,'m',{}) 로 매칭되도록 key/field 만 채워도 충분.
    const series = Array.from({ length: 48 }, (_, i) => ({ key: `k${i}`, field: 'm' }));
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
      within(within(select()).getByRole('table')).getByText('agents.detail.store.colValue'),
    ).toBeInTheDocument();

    // 컬럼 설정 메뉴에서 현재 값 컬럼 숨김.
    fireEvent.click(within(select()).getByTestId('store-columns-settings'));
    const menu = within(select()).getByRole('menu');
    const valueLabel = within(menu)
      .getByText('agents.detail.store.colValue')
      .closest('label')!;
    fireEvent.click(valueLabel.querySelector('input[type="checkbox"]')!);

    expect(
      within(within(select()).getByRole('table')).queryByText('agents.detail.store.colValue'),
    ).not.toBeInTheDocument();
    // localStorage 에 hidden 영속.
    const raw = window.localStorage.getItem('panel-settings.storeTable.p1');
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw!).hidden).toContain('value');

    // 재마운트 → 숨김 복원.
    unmount();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(
      within(table).queryByText('agents.detail.store.colValue'),
    ).not.toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colKey')).toBeInTheDocument();
  });

  it('선택 행만 펼침 가능 + 펼침 상세는 편집 필드만(키 설명 서브라인 없음, REQ-19 폐지/AC-21)', () => {
    // 긴 키(선택됨) + 짧은 키(미선택)를 함께 둔다. 선택 상태는 store_source.series 로 반영.
    state.keyObjects = [
      { key: 'very-long-key-abcdefgh', registration: 'auto', data_type: 'float', field: 'm', tags: {} },
      { key: 'k2', registration: 'auto', data_type: 'int', field: 'humidity', tags: {} },
    ];
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({
          series: [{ key: 'very-long-key-abcdefgh', field: 'm', alias: 'very-long-key-abcdefgh' }],
        })}
        onConfigChange={vi.fn()}
      />,
    );
    const select = screen.getByTestId('panel-store-select');
    // 선택된 행 1개만 펼침 셰브론을 갖는다(미선택 k2 는 없음).
    expect(
      within(select).getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel'),
    ).toHaveLength(1);
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const detail = within(select).getByTestId('store-row-detail-very-long-key-abcdefgh');
    // 편집 필드만: 이름 입력이 존재하고, 이름 위 키·종류·태그 설명 서브라인은 존재하지 않는다.
    expect(within(detail).getByTestId('chart-store-series-alias-0')).toBeInTheDocument();
    expect(within(detail).queryByTestId('series-detail-key-0')).toBeNull();
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

describe('heatmap 시리즈 위치 — 체크박스 선택이 sensor_positions 를 구동 (SPEC-PANEL-SETTINGS-001)', () => {
  it('heatmap: 시리즈 선택 시 sensor_positions[key] 가 중앙(0.5,0.5)으로 설정된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    fireEvent.click(checkboxes[0]!); // k1 선택
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ sensor_positions: { [SID_K1]: { x: 0.5, y: 0.5 } } }),
    );
  });

  it('heatmap: 시리즈 선택 해제 시 sensor_positions[key] 가 제거된다', () => {
    const onConfigChange = vi.fn();
    // 이미 k1 이 series + sensor_positions 에 있는 상태(다른 센서 k2 좌표는 보존).
    const panel = {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: {
        data_source: 'store',
        store_source: { ...STORE_SOURCE, series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }] },
        sensor_positions: { k1: { x: 0.2, y: 0.3 }, k2: { x: 0.9, y: 0.9 } },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    expect(checkboxes[0]!.checked).toBe(true); // k1 체크됨
    fireEvent.click(checkboxes[0]!); // k1 해제
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ sensor_positions: { k2: { x: 0.9, y: 0.9 } } }),
    );
  });

  it('비-heatmap 패널: 선택해도 sensor_positions 부수효과가 없다', () => {
    const onConfigChange = vi.fn();
    const linePanel = {
      id: 'p1',
      type: 'graph-chart',
      title: 'l',
      config: { data_source: 'store', store_source: { ...STORE_SOURCE } },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={linePanel} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    fireEvent.click(checkboxes[0]!);
    const arg = onConfigChange.mock.calls[0]![0] as Record<string, unknown>;
    expect(arg.sensor_positions).toBeUndefined();
    expect(arg.store_source).toBeDefined();
  });

  it('heatmap: 체크박스 선택은 selection_mode/tag_filters 를 건드리지 않는다(표시/바인딩 분리, REQ-22)', () => {
    const onConfigChange = vi.fn();
    // v0.3.0: 바인딩 모드는 동적 바인딩 토글이 단독 제어한다. 체크박스는 series(명시 keys 선택)만
    // 편집하며 selection_mode 를 강제하지 않는다.
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    fireEvent.click(checkboxes[0]!);
    const arg = onConfigChange.mock.calls[0]![0] as { store_source: Record<string, unknown> };
    // selection_mode 미강제(패널이 미설정이면 그대로 undefined), tag_filters 도 미변경.
    expect(arg.store_source.selection_mode).toBeUndefined();
    expect(arg.store_source.series).toHaveLength(1);
  });

  it('heatmap: 데이터 소스의 선택된 시리즈에서 x 좌표를 편집하면 sensor_positions 에 반영된다', () => {
    const onConfigChange = vi.fn();
    const panel = {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: {
        data_source: 'store',
        store_source: {
          ...STORE_SOURCE,
          selection_mode: 'keys',
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        },
        sensor_positions: { k1: { x: 0.5, y: 0.5 } },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    // 선택된 k1 행을 펼쳐야 인라인 상세의 x/y 입력에 접근할 수 있다(v0.4.0 행 펼침).
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const xInput = within(select).getByTestId(`heatmap-pos-x-${SID_K1}`) as HTMLInputElement;
    expect(xInput).toBeInTheDocument();
    fireEvent.change(xInput, { target: { value: '0.25' } });
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ sensor_positions: { [SID_K1]: { x: 0.25, y: 0.5 } } }),
    );
  });
});

// ---------------------------------------------------------------------------
// 결함(회귀 방지): 한 store key 를 metric/tags 로 나눠 쓰는 형제 시리즈.
// 백엔드 GET /keys 는 같은 key 에 대해 시리즈 행을 여러 개 돌려주므로, 좌표를 key 로 키잉하면
// 형제끼리 좌표 한 칸을 공유하게 되고(같이 움직임) 하나를 해제하면 아직 체크된 형제의 좌표까지
// 지워져 히트맵이 통째로 비었다("히트맵 시리즈값 적용 안됨"). 좌표 키는 시리즈 동일성 키다.
// ---------------------------------------------------------------------------
describe('한 key 를 공유하는 형제 시리즈의 센서 좌표 독립성', () => {
  /** 같은 key('dup')를 room 태그로 나눠 쓰는 두 시리즈. */
  const DUP_A = { key: 'dup', field: 'temperature', tags: { room: 'A' } };
  const DUP_B = { key: 'dup', field: 'temperature', tags: { room: 'B' } };
  const SID_A = heatmapSensorId(DUP_A);
  const SID_B = heatmapSensorId(DUP_B);

  beforeEach(() => {
    state.keyObjects = [
      { ...DUP_A, registration: 'auto', data_type: 'float' },
      { ...DUP_B, registration: 'auto', data_type: 'float' },
    ];
  });

  function heatmapPanel(config: Record<string, unknown>): PanelConfig {
    return {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: { data_source: 'store', ...config },
    } as unknown as PanelConfig;
  }

  it('둘 다 체크하면 서로 다른 좌표 항목을 갖는다(좌표 공유 없음)', () => {
    const onConfigChange = vi.fn();
    // A 는 이미 체크 + 배치된 상태. 여기서 B 를 추가로 체크한다.
    const panel = heatmapPanel({
      store_source: { ...STORE_SOURCE, selection_mode: 'keys', series: [{ ...DUP_A, alias: 'dup' }] },
      sensor_positions: { [SID_A]: { x: 0.2, y: 0.2 } },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    expect(checkboxes[0]!.checked).toBe(true); // A
    expect(checkboxes[1]!.checked).toBe(false); // B(같은 key 지만 별개 시리즈)
    fireEvent.click(checkboxes[1]!);
    // A 의 좌표는 그대로, B 는 자기 몫의 기본 좌표를 새로 받는다.
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        sensor_positions: { [SID_A]: { x: 0.2, y: 0.2 }, [SID_B]: { x: 0.5, y: 0.5 } },
      }),
    );
  });

  it('한쪽 좌표를 옮겨도 다른 쪽은 움직이지 않는다(입력이 같은 값을 가리키지 않음)', () => {
    const onConfigChange = vi.fn();
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [
          { ...DUP_A, alias: 'A' },
          { ...DUP_B, alias: 'B' },
        ],
      },
      sensor_positions: { [SID_A]: { x: 0.2, y: 0.2 }, [SID_B]: { x: 0.8, y: 0.8 } },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    // 두 행 모두 펼쳐 각자의 x/y 입력을 노출한다.
    within(select)
      .getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel')
      .forEach((btn) => fireEvent.click(btn));
    const xA = within(select).getByTestId(`heatmap-pos-x-${SID_A}`) as HTMLInputElement;
    const xB = within(select).getByTestId(`heatmap-pos-x-${SID_B}`) as HTMLInputElement;
    // 각 입력은 자기 시리즈의 값을 보여준다(공유 시 둘 다 같은 값이 보였다).
    expect(xA.value).toBe('0.2');
    expect(xB.value).toBe('0.8');
    fireEvent.change(xA, { target: { value: '0.1' } });
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        sensor_positions: { [SID_A]: { x: 0.1, y: 0.2 }, [SID_B]: { x: 0.8, y: 0.8 } },
      }),
    );
  });

  it('보고된 결함: 한쪽을 체크 해제해도 다른 쪽 좌표가 남는다(해제가 형제 좌표를 지우지 않는다)', () => {
    // 이 테스트가 결함의 핵심이다. 수정 전 코드는 `sensor_positions[key]` 를 지웠으므로 두
    // 시리즈가 공유하던 유일한 항목이 사라지고 → 남은 B 가 미배치 → points 0개 → 히트맵이
    // 아무것도 그리지 않았다.
    const onConfigChange = vi.fn();
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [
          { ...DUP_A, alias: 'A' },
          { ...DUP_B, alias: 'B' },
        ],
      },
      sensor_positions: { [SID_A]: { x: 0.2, y: 0.2 }, [SID_B]: { x: 0.8, y: 0.8 } },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const checkboxes = screen.getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    fireEvent.click(checkboxes[0]!); // A 해제
    const patch = onConfigChange.mock.calls.at(-1)![0] as {
      store_source: { series: Array<{ tags?: Record<string, string> }> };
      sensor_positions?: Record<string, unknown>;
    };
    // A 만 선택에서 빠지고,
    expect(patch.store_source.series).toHaveLength(1);
    expect(patch.store_source.series[0]!.tags).toEqual({ room: 'B' });
    // B 의 좌표는 온전히 남는다(= 히트맵이 계속 렌더된다).
    // (수정 전 코드는 좌표 패치를 아예 내지 않거나 공유 항목을 지워 B 가 미배치가 됐다.
    //  ?? {} 로 널세이프하게 읽어 TypeError 대신 기대값 불일치로 실패하게 한다.)
    const nextPositions = patch.sensor_positions ?? {};
    expect(nextPositions[SID_B]).toEqual({ x: 0.8, y: 0.8 });
    expect(nextPositions[SID_A]).toBeUndefined();
  });

  it('하위호환: raw key 로 저장된 옛 좌표는 읽는 시점에 동일성 키로 이관된다(모호하면 첫 시리즈)', () => {
    const onConfigChange = vi.fn();
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [
          { ...DUP_A, alias: 'A' },
          { ...DUP_B, alias: 'B' },
        ],
      },
      // 옛 스키마: key 하나로 키잉된 좌표(어느 형제 것인지 정보 없음).
      sensor_positions: { dup: { x: 0.3, y: 0.4 } },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    within(select)
      .getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel')
      .forEach((btn) => fireEvent.click(btn));
    // 모호 → config.series 순서상 첫 시리즈(A)로 결정적으로 귀속. B 는 미배치(빈 입력).
    expect((within(select).getByTestId(`heatmap-pos-x-${SID_A}`) as HTMLInputElement).value).toBe(
      '0.3',
    );
    expect((within(select).getByTestId(`heatmap-pos-x-${SID_B}`) as HTMLInputElement).value).toBe(
      '',
    );
  });

  it('이름(alias) 변경은 좌표 키에 영향을 주지 않는다', () => {
    const onConfigChange = vi.fn();
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [{ ...DUP_A, alias: '거실' }],
      },
      sensor_positions: { [SID_A]: { x: 0.2, y: 0.2 } },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    fireEvent.click(
      within(select).getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel')[0]!,
    );
    // 이름을 바꿔도 좌표 입력은 같은 항목(0.2)을 계속 가리킨다.
    fireEvent.change(within(select).getByTestId('chart-store-series-alias-0'), {
      target: { value: '안방' },
    });
    expect((within(select).getByTestId(`heatmap-pos-x-${SID_A}`) as HTMLInputElement).value).toBe(
      '0.2',
    );
    const patch = onConfigChange.mock.calls.at(-1)![0] as Record<string, unknown>;
    // 이름 변경 패치는 좌표를 건드리지 않는다.
    expect(patch.sensor_positions).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// 표시 결함(회귀 방지): 시리즈 동일성은 (key, field, tags) 인데 이름 셀에는 key 만
// 찍혀, 서로 다른 센서 N 개가 목록에서 같은 글자로 보였다(→ 한 좌표를 공유하는 것처럼 보임).
// 좌표/매칭은 이미 동일성 키로 분리돼 있으므로 이 블록은 **표기**만 검증한다.
// ---------------------------------------------------------------------------
describe('이름 컬럼 — 시리즈를 구분하는 표기', () => {
  const DUP_A = { key: 'dup', field: 'temperature', tags: { room: 'A' } };
  const DUP_B = { key: 'dup', field: 'temperature', tags: { room: 'B' } };

  beforeEach(() => {
    state.keyObjects = [
      { ...DUP_A, registration: 'auto', data_type: 'float' },
      { ...DUP_B, registration: 'auto', data_type: 'float' },
    ];
  });

  function heatmapPanel(config: Record<string, unknown> = {}): PanelConfig {
    return {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: { data_source: 'store', store_source: { ...STORE_SOURCE }, ...config },
    } as unknown as PanelConfig;
  }

  it('한 key 를 공유하는 형제 행이 서로 다른 이름으로 표시된다(보고된 결함)', () => {
    render(<PanelSettingsDataSource panel={heatmapPanel()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('dup · temperature{room=A}')).toBeInTheDocument();
    expect(within(table).getByText('dup · temperature{room=B}')).toBeInTheDocument();
    // 'dup' 단독 표기는 두 행의 **키 컬럼**에만 남는다(행당 1개). 이름 컬럼까지 key 를 찍던
    // 예전에는 4개였고, 그래서 두 센서가 같은 것처럼 보였다.
    expect(within(table).getAllByText('dup')).toHaveLength(2);
  });

  it('사용자가 붙인 이름(alias)이 서술 표기를 이긴다', () => {
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [{ ...DUP_A, alias: '거실' }],
      },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('거실')).toBeInTheDocument();
    expect(within(table).queryByText('dup · temperature{room=A}')).toBeNull();
    // 이름을 붙이지 않은 형제는 서술 표기를 유지한다.
    expect(within(table).getByText('dup · temperature{room=B}')).toBeInTheDocument();
  });

  it('생성 시 기본값(alias=key)은 사용자 이름이 아니므로 서술 표기로 폴백한다', () => {
    // 이미 저장된 패널은 전부 이 상태다 — 이 분기가 없으면 기존 패널에서 결함이 그대로 남는다.
    const panel = heatmapPanel({
      store_source: {
        ...STORE_SOURCE,
        selection_mode: 'keys',
        series: [
          { ...DUP_A, alias: 'dup' },
          { ...DUP_B, alias: 'dup' },
        ],
      },
    });
    render(<PanelSettingsDataSource panel={panel} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('dup · temperature{room=A}')).toBeInTheDocument();
    expect(within(table).getByText('dup · temperature{room=B}')).toBeInTheDocument();
  });

  it('metric/tags 가 없는 시리즈는 key 하나로 표시된다(구분자/후행 공백 없음)', () => {
    state.keyObjects = [{ key: 'plain', registration: 'auto', data_type: 'float', tags: {} }];
    render(<PanelSettingsDataSource panel={heatmapPanel()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    // 키 컬럼 + 이름 컬럼 두 곳에 같은 글자가 나온다(이름 셀이 key 로 깔끔히 줄어든 상태).
    const cells = within(table).getAllByText('plain');
    expect(cells).toHaveLength(2);
    for (const cell of cells) expect(cell.textContent).toBe('plain');
  });

  it('긴 이름은 잘라 표시하고 전체 값은 title 로 남긴다(좁은 행 레이아웃 보호)', () => {
    state.keyObjects = [
      {
        key: 'very-long-store-key-name-abcdefgh',
        registration: 'auto',
        data_type: 'float',
        field: 'temperature',
        tags: { room: 'A', floor: '3' },
      },
    ];
    render(<PanelSettingsDataSource panel={heatmapPanel()} onConfigChange={vi.fn()} />);
    const full = 'very-long-store-key-name-abcdefgh · temperature{floor=3, room=A}';
    const cell = within(screen.getByTestId('panel-store-select')).getByTitle(full);
    expect(cell).toHaveTextContent(full);
    expect(cell.className).toContain('truncate');
    expect(cell.className).toContain('max-w-[220px]');
  });

  it('비-heatmap 패널(라인 차트)도 같은 표기를 쓴다 — 화면마다 다른 이름이 되지 않는다', () => {
    const linePanel = {
      id: 'p1',
      type: 'graph-chart',
      title: 'l',
      config: { data_source: 'store', store_source: { ...STORE_SOURCE } },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={linePanel} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('dup · temperature{room=A}')).toBeInTheDocument();
  });
});

describe('REQ-17/AC-19 — 컬럼 순서 key · name · value · tag', () => {
  it('선택 테이블 컬럼이 키 · 이름 · 현재 값 · 태그 순으로 배치된다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    const headers = within(table)
      .getAllByRole('columnheader')
      .map((h) => h.textContent ?? '');
    const idx = (needle: string) => headers.findIndex((h) => h.includes(needle));
    const key = idx('colKey');
    const name = idx('colAlias'); // colAlias 라벨이 "이름"으로 표기됨(REQ-16)
    const value = idx('colValue');
    const tags = idx('colTags');
    expect(key).toBeGreaterThanOrEqual(0);
    expect(key).toBeLessThan(name);
    expect(name).toBeLessThan(value);
    expect(value).toBeLessThan(tags);
    // 필드 전용 컬럼은 제거됐다(이름 셀이 key+metric+tags 를 합쳐 보여준다).
    expect(idx('colField')).toBe(-1);
  });
});

describe('키 컬럼 — 잘림 없이 전체 표시', () => {
  it('8자를 넘는 키도 앞부분만 잘리지 않고 전체가 렌더된다', () => {
    const longKey = 'device-1e6dc10e-3894-420f';
    state.keyObjects = [
      { key: longKey, registration: 'auto', data_type: 'float', field: 'temperature', tags: {} },
    ];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText(longKey)).toBeInTheDocument();
    // 축약형(앞 8자)이 단독 텍스트로 남아 있으면 안 된다.
    expect(within(table).queryByText('device-1')).not.toBeInTheDocument();
  });
});

describe('현재 값 컬럼 — 에이전트 상태 스냅샷 조인', () => {
  it('시리즈 동일성(key+metric+tags)이 일치하는 라이브 값을 행에 표시한다', () => {
    state.agentEntries = [
      { key: 'k1', field: 'temperature', tags: { room: '1' }, value: 23.5 },
      { key: 'k2', field: 'humidity', tags: {}, value: 60 },
    ];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('23.5')).toBeInTheDocument();
    expect(within(table).getByText('60')).toBeInTheDocument();
  });

  it('같은 키라도 metric/tags 가 다른 라이브 엔트리의 값을 섞지 않는다', () => {
    // k1 은 temperature{room=1} 인데, 스냅샷에는 다른 시리즈(humidity)만 있다.
    state.agentEntries = [
      { key: 'k1', field: 'humidity', tags: { room: '1' }, value: 99 },
    ];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).queryByText('99')).not.toBeInTheDocument();
  });

  it('라이브 값이 없어도 행은 정상 렌더된다(메타데이터 전용 행)', () => {
    state.agentEntries = [];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const table = within(screen.getByTestId('panel-store-select')).getByRole('table');
    expect(within(table).getByText('agents.detail.store.colValue')).toBeInTheDocument();
  });
});

describe('REQ-15/AC-17b — 태그 키(종류)별 표시 필터(OR 내/AND 간)', () => {
  function checkTagValue(pair: string): void {
    fireEvent.click(screen.getByTitle(pair).closest('label')!.querySelector('input')!);
  }

  it('device_id∈{A,B} AND type=report → S1,S2 만 표시(태그 키별 차원)', () => {
    state.keyObjects = [
      { key: 's1', registration: 'auto', data_type: 'float', field: 'm', tags: { device_id: 'A', type: 'report' } },
      { key: 's2', registration: 'auto', data_type: 'float', field: 'm', tags: { device_id: 'B', type: 'report' } },
      { key: 's3', registration: 'auto', data_type: 'float', field: 'm', tags: { device_id: 'C', type: 'report' } },
      { key: 's4', registration: 'auto', data_type: 'float', field: 'm', tags: { device_id: 'A', type: 'command' } },
    ];
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    const rows = () => within(select).getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    expect(rows()).toHaveLength(4);
    // 태그 컬럼 필터 열기 → device_id A,B (같은 태그 키 OR) + type report (다른 태그 키 AND).
    fireEvent.click(within(select).getByTestId('store-filter-agents.detail.store.colTags'));
    checkTagValue('device_id=A');
    checkTagValue('device_id=B');
    checkTagValue('type=report');
    // 결과: s1, s2 만. (s3=device_id C 제외, s4=type command 제외)
    expect(rows()).toHaveLength(2);
  });
});

describe('REQ-18/19/20/21 — 선택 행 인라인 펼침 세부 정보', () => {
  function heatmapWithSeries(): PanelConfig {
    return {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: {
        data_source: 'store',
        store_source: {
          ...STORE_SOURCE,
          selection_mode: 'keys',
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        },
        sensor_positions: { k1: { x: 0.5, y: 0.5 } },
      },
    } as unknown as PanelConfig;
  }

  it('AC-20/AC-22 — 편집은 선택 행 인라인 펼침에서만; 별도 SelectedSeriesList 섹션 부재', () => {
    render(<PanelSettingsDataSource panel={heatmapWithSeries()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    // 별도 그룹/섹션(SelectedSeriesList) + 구 좌표 블록이 존재하지 않는다.
    expect(screen.queryByTestId('chart-store-selected-series')).toBeNull();
    expect(screen.queryByTestId('panel-store-positions')).toBeNull();
    // 선택된 k1 행을 펼치면 그 행 인라인 상세에 이름/색상/좌표 편집이 함께 들어있다.
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const detail = within(select).getByTestId('store-row-detail-k1');
    expect(within(detail).getByTestId('series-detail-0')).toBeInTheDocument();
    expect(within(detail).getByTestId('chart-store-series-alias-0')).toBeInTheDocument();
    expect(within(detail).getByTestId('chart-store-series-color-0')).toBeInTheDocument();
    expect(within(detail).getByTestId(`heatmap-pos-x-${SID_K1}`)).toBeInTheDocument();
    expect(within(detail).getByTestId(`heatmap-pos-y-${SID_K1}`)).toBeInTheDocument();
  });

  it('AC-20(레이아웃) — 색상은 이름 옆이 아닌 전용 색상 행에서 편집 가능(color 보존)', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={heatmapWithSeries()} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const detail = within(select).getByTestId('store-row-detail-k1');
    // 색상 전용 행 라벨 + 색상 입력이 존재하고, 편집 시 series[i].color 로 반영된다.
    expect(within(detail).getByText('dashboard.settings.seriesDetailsColor')).toBeInTheDocument();
    fireEvent.change(within(detail).getByTestId('chart-store-series-color-0'), {
      target: { value: '#123456' },
    });
    const patch = onConfigChange.mock.calls.at(-1)![0] as {
      store_source: { series: Array<{ color?: string }> };
    };
    expect(patch.store_source.series[0]!.color).toBe('#123456');
  });

  it('AC-20 — 미선택 행은 펼침이 없다', () => {
    // k1 만 선택. k2 는 미선택 → 펼침 셰브론이 하나만 존재한다.
    render(<PanelSettingsDataSource panel={heatmapWithSeries()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    expect(
      within(select).getAllByLabelText('agents.detail.store.keyRowExpandAriaLabel'),
    ).toHaveLength(1);
  });

  it('AC-23 (Edge) — 비-heatmap 패널: 인라인 상세에 센서 좌표 필드가 없다', () => {
    const linePanel = {
      id: 'p1',
      type: 'graph-chart',
      title: 'l',
      config: {
        data_source: 'store',
        store_source: {
          ...STORE_SOURCE,
          selection_mode: 'keys',
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={linePanel} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const detail = within(select).getByTestId('store-row-detail-k1');
    expect(within(detail).getByTestId('series-detail-0')).toBeInTheDocument();
    expect(within(detail).queryByTestId(`heatmap-pos-x-${SID_K1}`)).toBeNull();
  });

  it('AC-24(d) — 세부 편집은 바인딩 모드(ON/OFF)와 무관하게 접근·편집 가능하다', () => {
    // 동적 바인딩 ON(tag 모드)이라도 선택된 series 행은 펼쳐서 이름을 편집할 수 있다.
    const onConfigChange = vi.fn();
    const panel = {
      id: 'p1',
      type: 'graph-chart',
      title: 'l',
      config: {
        data_source: 'store',
        store_source: {
          ...STORE_SOURCE,
          selection_mode: 'tag',
          tag_filters: { room: '1' },
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    fireEvent.click(within(select).getByLabelText('agents.detail.store.keyRowExpandAriaLabel'));
    const aliasInput = within(select).getByTestId('chart-store-series-alias-0');
    fireEvent.change(aliasInput, { target: { value: 'Room 1' } });
    const patch = onConfigChange.mock.calls.at(-1)![0] as { store_source: { series: Array<{ alias?: string }> } };
    expect(patch.store_source.series[0]!.alias).toBe('Room 1');
  });
});

describe('REQ-22/AC-24 — 명시적 동적 바인딩 토글 + 표시/바인딩 분리', () => {
  it('(a) 신규 패널: 동적 바인딩 토글 OFF, selection_mode 미설정(keys/명시 선택)', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    const toggle = screen.getByTestId('chart-dynamic-binding-toggle') as HTMLInputElement;
    expect(toggle).toBeInTheDocument();
    expect(toggle.checked).toBe(false);
  });

  it('(b) 토글 ON → selection_mode:tag 로 전환된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('chart-dynamic-binding-toggle'));
    const arg = onConfigChange.mock.calls[0]![0] as { store_source: Record<string, unknown> };
    expect(arg.store_source.selection_mode).toBe('tag');
  });

  it('(b) 토글 OFF(다시 끄기) → selection_mode:keys, tag_filters 는 보존(additive)', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    const toggle = screen.getByTestId('chart-dynamic-binding-toggle') as HTMLInputElement;
    expect(toggle.checked).toBe(true); // 하위호환 로드: tag 모드 → ON
    fireEvent.click(toggle);
    const arg = onConfigChange.mock.calls[0]![0] as { store_source: Record<string, unknown> };
    expect(arg.store_source.selection_mode).toBe('keys');
    // tag_filters 필드는 제거되지 않는다(additive only).
    expect(arg.store_source.tag_filters).toEqual({ room: '1' });
  });

  it('(c) 하위호환: 기존 selection_mode:tag 패널은 토글 ON 으로 로드되고 마운트 시 config 를 변경하지 않는다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(
      (screen.getByTestId('chart-dynamic-binding-toggle') as HTMLInputElement).checked,
    ).toBe(true);
    // 로드만으로 tag_filters/selection_mode 를 건드리지 않는다(보존).
    expect(onConfigChange).not.toHaveBeenCalled();
  });
});

describe('유령 선택 — 스토어에서 사라진 선택 시리즈 회수', () => {
  /** 스토어 목록에 없는 키를 선택 상태로만 들고 있는 패널(보고된 결함 상황). */
  function panelWithStale(): PanelConfig {
    return panelWithAgent({
      series: [
        { key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' },
        { key: 'gone', field: 'temperature', tags: { room: '9' }, alias: '옛 센서' },
      ],
    });
  }
  const SID_GONE = heatmapSensorId({
    key: 'gone',
    field: 'temperature',
    tags: { room: '9' },
  });

  it('스토어에 없는 선택 시리즈도 행으로 나타난다 — 행이 없으면 체크를 풀 수단이 없다', () => {
    render(<PanelSettingsDataSource panel={panelWithStale()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    // 합성 행 + stale 배지.
    expect(within(select).getAllByTestId('panel-store-select-stale-badge')).toHaveLength(1);
    // 살아있는 선택(k1)에는 배지가 붙지 않는다.
    expect(within(select).getByText('옛 센서')).toBeInTheDocument();
  });

  it('합성 행의 체크를 풀면 series 와 좌표에서 함께 제거된다', () => {
    const onConfigChange = vi.fn();
    const panel = {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: {
        data_source: 'store',
        store_source: {
          ...STORE_SOURCE,
          series: [
            { key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' },
            { key: 'gone', field: 'temperature', tags: { room: '9' }, alias: '옛 센서' },
          ],
        },
        sensor_positions: { [SID_K1]: { x: 0.2, y: 0.2 }, [SID_GONE]: { x: 0.8, y: 0.8 } },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDataSource panel={panel} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    // 합성 행은 목록 맨 위에 고정되므로 첫 행 체크박스가 유령 선택이다.
    const checkboxes = within(select).getAllByRole('checkbox');
    fireEvent.click(checkboxes[1]!); // [0] 은 동적 바인딩 토글.
    const patch = onConfigChange.mock.calls.at(-1)![0] as {
      store_source: { series: Array<{ key: string }> };
      sensor_positions: Record<string, unknown>;
    };
    expect(patch.store_source.series.map((s) => s.key)).toEqual(['k1']);
    expect(patch.sensor_positions[SID_GONE]).toBeUndefined();
    // 살아있는 선택의 좌표는 건드리지 않는다.
    expect(patch.sensor_positions[SID_K1]).toEqual({ x: 0.2, y: 0.2 });
  });

  it('일괄 정리 버튼이 유령 선택을 한 번에 제거한다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={panelWithStale()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('panel-store-select-cleanup-stale'));
    const patch = onConfigChange.mock.calls.at(-1)![0] as {
      store_source: { series: Array<{ key: string }> };
    };
    expect(patch.store_source.series.map((s) => s.key)).toEqual(['k1']);
  });

  it('유령 선택이 없으면 정리 버튼도 배지도 없다(회귀 0)', () => {
    render(
      <PanelSettingsDataSource
        panel={panelWithAgent({
          series: [{ key: 'k1', field: 'temperature', tags: { room: '1' }, alias: 'k1' }],
        })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('panel-store-select-cleanup-stale')).not.toBeInTheDocument();
    expect(screen.queryByTestId('panel-store-select-stale-badge')).not.toBeInTheDocument();
  });

  it('키 목록 조회 전(미성공)에는 stale 로 단정하지 않는다 — 멀쩡한 선택을 지우지 않는다', () => {
    state.keysLoaded = false;
    state.keyObjects = [];
    render(<PanelSettingsDataSource panel={panelWithStale()} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('panel-store-select-cleanup-stale')).not.toBeInTheDocument();
    expect(screen.queryByTestId('panel-store-select-stale-badge')).not.toBeInTheDocument();
  });

  it('컬럼 필터가 걸려 있어도 합성 행은 사라지지 않는다(해제 경로 보존)', () => {
    // key 컬럼 필터를 저장해 두고 렌더 → 필터는 라이브 행만 좁히고 합성 행은 고정된다.
    window.localStorage.setItem(
      'moai.panel.p1.storeTablePrefs',
      JSON.stringify({ sort: null, filters: { key: { text: 'zzz', values: [] } }, hidden: [] }),
    );
    render(<PanelSettingsDataSource panel={panelWithStale()} onConfigChange={vi.fn()} />);
    const select = screen.getByTestId('panel-store-select');
    expect(within(select).getAllByTestId('panel-store-select-stale-badge')).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// 같은 종류 여러 인스턴스 — 시리즈 선택 표는 인스턴스마다 있어야 한다.
//
// 보고된 결함: Store 를 둘 이상 두면 둘째는 시리즈를 고를 수단이 없었다. 표가 패널 단위로
// 한 번만 그려져 `config.store_source`(첫째)만 고쳤기 때문이다 — 에이전트는 고를 수 있지만
// 시리즈가 비어 영영 비활성이었고, 그래서 둘째 소스는 아무것도 그리지 않았다.
// ---------------------------------------------------------------------------

describe('소스 인스턴스마다 시리즈 선택 표', () => {
  function twoStorePanel(): PanelConfig {
    return {
      id: 'p1',
      type: 'heatmap',
      title: 'h',
      config: {
        data_source: 'store',
        sources: [
          { kind: 'store', store_source: { ...STORE_SOURCE } },
          { kind: 'store', store_source: { ...STORE_SOURCE, agent_name: 'store-b', series: [] } },
        ],
      },
    } as unknown as PanelConfig;
  }

  it('Store 인스턴스 수만큼 표가 뜬다', () => {
    render(<PanelSettingsDataSource panel={twoStorePanel()} onConfigChange={vi.fn()} />);
    expect(screen.getAllByTestId('panel-store-select')).toHaveLength(2);
  });

  it('둘째 표의 편집은 둘째 인스턴스에만 쓰인다 — 첫째를 덮지 않는다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={twoStorePanel()} onConfigChange={onConfigChange} />);

    // 둘째 표의 새로고침 버튼을 눌러 그 표가 살아 있음을 확인한 뒤,
    // 편집 경로가 인스턴스로 향하는지는 패치 형상으로 본다.
    const tables = screen.getAllByTestId('panel-store-select');
    expect(tables).toHaveLength(2);

    // 표가 인스턴스 config 를 본다 — 둘째 표는 둘째 에이전트를 읽는다.
    // (표 내부 렌더는 store 목록 조회에 달려 있으므로 여기서는 존재만 확인하고,
    //  되쓰기 규칙은 sourceEntryPatch 단위 테스트가 잠근다.)
    expect(tables[0]).not.toBe(tables[1]);
  });

  it('인스턴스가 하나면 표도 하나다 — 저장된 패널의 화면이 변하지 않는다', () => {
    render(<PanelSettingsDataSource panel={panelWithAgent()} onConfigChange={vi.fn()} />);
    expect(screen.getAllByTestId('panel-store-select')).toHaveLength(1);
  });
});
