// StoreSourceSection 의 시리즈 표시 이름(alias) 편집 테스트 (SPEC-WEB-005).
// useAgents / useStoreKeysWithTags 를 모킹해 네트워크 없이 렌더한다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';

// i18n 스텁 — 보간 슬롯({key})을 가진 키는 치환해 반환한다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({
    t: (k: string) => k,
  }),
}));

// 에이전트 목록 — store 에이전트 1개. 각 테스트가 필요 시 mockAgents 를 바꿔
// 이름/ID 매핑(리네임 시나리오)을 시뮬레이션한다. (SPEC-WEB-006)
let mockAgents: Array<{ id: string; name: string; type: string }> = [
  { id: 'store-uuid-1', name: 'store-1', type: 'store' },
];
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: { data: mockAgents },
  }),
}));

// 키 목록 — 단일 키 객체.
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({
    data: {
      keyObjects: [
        {
          key: 'room:1:temp',
          registration: 'manual',
          data_type: 'float',
          field: 'gauge',
          tags: { room: '1' },
        },
        {
          key: 'room:2:humidity',
          registration: 'manual',
          data_type: 'int',
          field: 'counter',
          tags: { room: '2' },
        },
        {
          key: 'system:status',
          registration: 'auto',
          data_type: 'string',
          field: 'state',
          tags: {},
        },
      ],
    },
    isLoading: false,
    isError: false,
  }),
}));

import { StoreSourceSection } from './ChartPanelSections';

function makePanel(config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type: 'stat', title: '테스트', config };
}

/** store 모드 + 시리즈 1개를 가진 기본 config. */
function storeConfig(seriesOverride?: Partial<StoreSourceConfig['series'][number]>): Record<string, unknown> {
  const store_source: StoreSourceConfig = {
    agent_name: 'store-1',
    namespace: 'default',
    series: [{ key: 'room:1:temp', field: 'gauge', tags: { room: '1' }, ...seriesOverride }],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
  };
  return { data_source: 'store', store_source };
}

describe('StoreSourceSection 시리즈 세부 편집은 별도 섹션으로 렌더하지 않는다 (SPEC-PANEL-SETTINGS-001 v0.4.0)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // v0.4.0(REQ-20/AC-22): 시리즈 세부 편집(이름/색상/선 스타일/토큰)은 더 이상 StoreSourceSection
  // 의 별도 섹션(SelectedSeriesList)이 아니라, 선택 행 인라인 펼침으로 이동했다. 위젯 동작 커버리지는
  // PanelSettingsDataSource.test.tsx(행 펼침 상세)로 이관됐다. 여기서는 별도 섹션 부재만 확인한다.
  it('store 모드에서 별도 SelectedSeriesList 섹션(그룹)을 렌더하지 않는다', () => {
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.queryByTestId('chart-store-selected-series')).toBeNull();
    expect(screen.queryByTestId('chart-store-series-row-0')).toBeNull();
    expect(screen.queryByTestId('chart-store-series-alias-0')).toBeNull();
  });
});

/**
 * agent_id 정본 저장 + 리네임 자동 반영 (SPEC-WEB-006).
 *
 * 에이전트 선택 시 안정적인 agent_id 를 정본으로, 현재 이름을 스냅샷으로 함께
 * 저장한다. 에이전트 이름이 바뀌어도 저장된 id 로 현재 이름을 해석해 선택이
 * 유지되고 현재 이름이 표시되어야 한다.
 */
describe('StoreSourceSection agent_id 정본 저장(SPEC-WEB-006)', () => {
  const originalAgents = mockAgents;
  beforeEach(() => {
    vi.clearAllMocks();
    mockAgents = [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }];
  });
  afterEach(() => {
    mockAgents = originalAgents;
  });

  /** 에이전트 미선택 store config(에이전트 셀렉트가 빈 상태). */
  function noAgentConfig(): Record<string, unknown> {
    const store_source: StoreSourceConfig = {
      agent_name: '',
      namespace: 'default',
      series: [],
      time_window_ms: 60_000,
      interval_ms: 10_000,
      aggregation: 'average',
      refresh_interval_ms: 5_000,
    };
    return { data_source: 'store', store_source };
  }

  it('에이전트 선택 시 agent_id(정본)와 agent_name(스냅샷)을 함께 저장한다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(noAgentConfig())} onConfigChange={onConfigChange} />,
    );
    const select = screen.getByTestId('chart-store-agent-select');
    // 옵션 value 는 agent id 기준이다.
    fireEvent.change(select, { target: { value: 'store-uuid-1' } });

    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.agent_id).toBe('store-uuid-1');
    expect(patch.store_source.agent_name).toBe('store-1');
    // 에이전트 변경 시 시리즈는 초기화된다.
    expect(patch.store_source.series).toEqual([]);
  });

  it('저장된 이름이 옛 이름이어도 agent_id 로 현재 이름을 셀렉트에 표시한다', () => {
    // 저장 config: agent_id 는 불변, 저장된 이름은 옛 이름('store-1').
    // 현재 목록: 같은 id 의 이름이 'renamed-store' 로 변경됨.
    mockAgents = [{ id: 'store-uuid-1', name: 'renamed-store', type: 'store' }];
    const store_source: StoreSourceConfig = {
      agent_id: 'store-uuid-1',
      agent_name: 'store-1',
      namespace: 'default',
      series: [],
      time_window_ms: 60_000,
      interval_ms: 10_000,
      aggregation: 'average',
      refresh_interval_ms: 5_000,
    };
    render(
      <StoreSourceSection
        panel={makePanel({ data_source: 'store', store_source })}
        onConfigChange={vi.fn()}
      />,
    );
    const select = screen.getByTestId('chart-store-agent-select') as HTMLSelectElement;
    // 셀렉트 선택값은 id 이며, 유효한(비활성 아님) 옵션으로 유지된다.
    expect(select.value).toBe('store-uuid-1');
    // 현재 이름 옵션이 렌더된다(옛 이름이 아닌 현재 이름).
    expect(screen.getByRole('option', { name: 'renamed-store' })).toBeInTheDocument();
  });
});

describe('Store 조회 설정 정보 "i" 말풍선 (SPEC-PANEL-SETTINGS-001)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  /** store 모드 config — 조회 설정 값 지정(정보 말풍선 표시용). */
  function infoConfig(): Record<string, unknown> {
    const store_source: StoreSourceConfig = {
      agent_id: 'store-uuid-1',
      agent_name: 'store-1',
      namespace: 'default',
      series: [],
      time_window_ms: 3_600_000, // 3600초
      interval_ms: 60_000, // 60초
      aggregation: 'last',
      refresh_interval_ms: 5_000, // 5초
    };
    return { data_source: 'store', store_source };
  }

  it('인라인 편집 입력(시간 윈도우/인터벌/집계)은 더 이상 렌더되지 않는다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-time-window')).toBeNull();
    expect(screen.queryByTestId('chart-store-interval')).toBeNull();
    expect(screen.queryByTestId('chart-store-aggregation')).toBeNull();
  });

  it('Store 모드에서 데이터소스 토글 옆에 "i" 정보 아이콘을 렌더한다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-info-button')).toBeInTheDocument();
    // Store 토글과 같은 영역에 있다(데이터소스 토글 존재 확인).
    expect(screen.getByTestId('chart-data-source-store')).toBeInTheDocument();
  });

  it('채널 모드에서는 "i" 정보 아이콘을 렌더하지 않는다', () => {
    render(
      <StoreSourceSection
        panel={makePanel({ data_source: 'channel' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-store-info-button')).toBeNull();
  });

  it('기본(닫힘)에서는 말풍선이 없고, "i" 클릭 시 열린다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-info-popover')).toBeNull();
    fireEvent.click(screen.getByTestId('chart-store-info-button'));
    expect(screen.getByTestId('chart-store-info-popover')).toBeInTheDocument();
  });

  it('말풍선은 현재 조회 설정 값 + 설명을 읽기전용으로 표시한다(입력 없음)', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId('chart-store-info-button'));
    const popover = screen.getByTestId('chart-store-info-popover');
    const text = popover.textContent ?? '';
    // 값: 3600초 / 60초 / 5초(단위 키는 i18n 스텁이 키를 그대로 반환).
    expect(text).toContain('3600');
    expect(text).toContain('60');
    expect(text).toContain('5');
    // 집계 라벨(last → tsdb.aggLast, i18n 스텁이 키 반환).
    expect(text).toContain('tsdb.aggLast');
    // 설명(기존 hint 키 재사용).
    expect(text).toContain('dashboard.chart.storeTimeWindowHint');
    // 읽기전용: 편집 입력/셀렉트가 없다.
    expect(popover.querySelector('input')).toBeNull();
    expect(popover.querySelector('select')).toBeNull();
  });
});

describe('시리즈 이름 형식 (패널 옵션)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('Store 모드에서 이름 형식 입력을 노출한다', () => {
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-series-name-format')).toBeInTheDocument();
  });

  it('선택된 시리즈 기준 토큰 버튼을 제공한다', () => {
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />);
    expect(
      screen.getByTestId('chart-store-series-name-format-token-measurement'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-series-name-format-token-field')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-series-name-format-token-room')).toBeInTheDocument();
  });

  it('입력값이 store_source.series_name_format 로 저장된다', () => {
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-series-name-format-input'), {
      target: { value: '{$.measurement}/{$.field}' },
    });
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        store_source: expect.objectContaining({
          series_name_format: '{$.measurement}/{$.field}',
        }),
      }),
    );
  });

  it('공백만 입력하면 undefined 로 저장되어 기본 표기로 돌아간다', () => {
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-series-name-format-input'), {
      target: { value: '   ' },
    });
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({
        store_source: expect.objectContaining({ series_name_format: undefined }),
      }),
    );
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `ChartPanelSections.tsx:398 · 404 · 445 · 458` 4개 설정 UI 게이트의 현재 참 조건을
// 잠근다. 네 지점 모두 `!tsdbMode && dataSource === 'store'` 형태이며, `tsdbMode` 는
// **config 가 아니라 로컬 `useState`** 다(`:320`).
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.11 (E1) — plan.md §3.6 CT-19 ~ CT-21
// ---------------------------------------------------------------------------
describe('설정 UI 게이팅 특성화 (SPEC-TSDB-002 M2, CT-19~CT-21)', () => {
  /** 4개 게이트의 렌더 여부(순서: StoreInfoPopover · 에이전트 셀렉트 · 이름 형식 · 대표값). */
  function gates(): boolean[] {
    return [
      screen.queryByTestId('chart-store-info-button') !== null,
      screen.queryByTestId('chart-store-agent-select') !== null,
      screen.queryByTestId('chart-store-series-name-format') !== null,
      screen.queryByTestId('chart-series-reduce') !== null,
    ];
  }

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("CT-19: data_source:'store' 면 4개 게이트가 모두 렌더된다", () => {
    // panel.type='stat' 은 REDUCE_PANEL_TYPES 에 속하므로 대표값 선택기까지 노출된다.
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />);
    expect(gates()).toEqual([true, true, true, true]);
    expect(screen.queryByTestId('chart-data-source-tsdb-placeholder')).toBeNull();
  });

  it("CT-20: data_source:'channel' 이면 4개 게이트가 모두 미렌더다", () => {
    render(
      <StoreSourceSection
        panel={makePanel({ data_source: 'channel', channel_name: 'c1' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(gates()).toEqual([false, false, false, false]);
    expect(screen.queryByTestId('chart-data-source-tsdb-placeholder')).toBeNull();
  });

  it('CT-21: 로컬 tsdbMode 가 true 면 4개 게이트 미렌더 + placeholder 렌더 (반전 기준선)', () => {
    // ─── 이 테스트는 **M6.9 · M6.10 에서 반전될 기준선**이다(AC-35). ───
    // 현재 TSDB 토글은 로컬 `useState` 에만 기록되고 `onConfigChange` 를 호출하지 않는다
    // (SPEC-PANEL-SETTINGS-001 REQ-05 의 의도된 no-op). SPEC-TSDB-002 §2.11 [E1] 이 그
    // 비목표를 대체하면 (a) placeholder 는 사라지고 (b) 토글이 config 에 영속되며
    // (c) TSDB 선택 UI 가 렌더된다. 그 시점에 이 단언은 제자리에서 반전된다 — 삭제하지
    // 않는 이유는 "왜 바뀌었는가" 의 기록을 diff 밖으로 내보내지 않기 위함이다.
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />,
    );
    // 전제: store 모드에서는 4개 게이트가 살아 있다.
    expect(gates()).toEqual([true, true, true, true]);

    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));

    expect(gates()).toEqual([false, false, false, false]);
    expect(screen.getByTestId('chart-data-source-tsdb-placeholder')).toBeInTheDocument();
    // config 는 건드리지 않는다(= Store 설정 보존). AC-05 의 현행 계약.
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('CT-21: tsdbMode 는 config.data_source 를 바꾸지 않으므로 채널로 되돌리면 게이트가 되살아난다', () => {
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    expect(gates()).toEqual([false, false, false, false]);

    // store 버튼을 누르면 setTsdbMode(false) 로 로컬 상태만 풀린다(config 는 이미 store).
    fireEvent.click(screen.getByTestId('chart-data-source-store'));
    expect(gates()).toEqual([true, true, true, true]);
  });
});
