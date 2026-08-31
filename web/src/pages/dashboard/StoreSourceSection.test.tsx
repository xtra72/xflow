// StoreSourceSection 의 시리즈 표시 이름(alias) 편집 테스트 (SPEC-WEB-005).
// useAgents / useStoreKeysWithTags 를 모킹해 네트워크 없이 렌더한다.

import type React from 'react';
import { useState } from 'react';
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

  it('시간 윈도우는 여전히 정보 표시 전용이다(인라인 편집 없음)', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-time-window')).toBeNull();
  });

  it('집계 함수는 저장된 값을 선택한 셀렉트로 편집한다', () => {
    // 종전에는 config 에만 있고 조작 통로가 없어 `average` 로 고정이었다.
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={onConfigChange} />);
    const sel = screen.getByTestId('chart-store-aggregation') as HTMLSelectElement;
    expect(sel.value).toBe('last');

    fireEvent.change(sel, { target: { value: 'min' } });
    const patch = onConfigChange.mock.calls.at(-1)?.[0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.aggregation).toBe('min');
  });

  // ---- 빈 버킷 채우기 (Store 도 서버가 계산한다) ----

  it('채우기 셀렉트가 활성이고 고른 전략이 저장된다', () => {
    // 종전에는 "Store 백엔드가 지원하지 않는다" 사유와 함께 통째로 비활성이었다.
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={onConfigChange} />);
    const sel = screen.getByTestId('chart-store-fill') as HTMLSelectElement;
    expect(sel.disabled).toBe(false);

    fireEvent.change(sel, { target: { value: 'zero' } });
    const patch = onConfigChange.mock.calls.at(-1)?.[0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.fill).toBe('zero');
  });

  it('채우지 않음을 고르면 config 에서 지운다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel({
          data_source: 'store',
          store_source: { ...(infoConfig().store_source as StoreSourceConfig), fill: 'zero' },
        })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('chart-store-fill'), { target: { value: '' } });
    const patch = onConfigChange.mock.calls.at(-1)?.[0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.fill).toBeUndefined();
  });

  it('avg 선택지는 남기되 비활성이다 — 없애면 "왜 없지" 가 된다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    const avg = screen.getByTestId('chart-store-fill-avg') as HTMLOptionElement;
    expect(avg.disabled).toBe(true);
  });

  it('사용 기간 제한은 직전값 사용을 골랐을 때만 나온다', () => {
    const { rerender } = render(
      <StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.queryByTestId('chart-store-fill-prev-max')).toBeNull();

    rerender(
      <StoreSourceSection
        panel={makePanel({
          data_source: 'store',
          store_source: { ...(infoConfig().store_source as StoreSourceConfig), fill: 'previous' },
        })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-fill-prev-max')).toBeInTheDocument();
  });

  // ---- 인터벌 편집 (Store 도 TSDB 와 같은 조작) ----

  it('Store 모드에서 인터벌 셀렉트를 렌더하고 저장된 값을 선택한다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    const sel = screen.getByTestId('chart-store-interval') as HTMLSelectElement;
    expect(sel.value).toBe('60000');
  });

  it('채널 모드에서는 인터벌 셀렉트를 렌더하지 않는다', () => {
    render(
      <StoreSourceSection
        panel={makePanel({ data_source: 'channel' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-store-interval')).toBeNull();
  });

  it('프리셋을 고르면 store_source.interval_ms 만 갱신한다', () => {
    const onConfigChange = vi.fn();
    const cfg = infoConfig();
    render(<StoreSourceSection panel={makePanel(cfg)} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-interval'), { target: { value: '300000' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: { ...(cfg.store_source as object), interval_ms: 300_000 },
    });
  });

  it('"직접 입력" 선택만으로는 값을 바꾸지 않는다', () => {
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-interval'), { target: { value: 'custom' } });
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('프리셋에 없는 값이면 직접 입력칸을 초 단위로 보여준다', () => {
    const cfg = infoConfig();
    (cfg.store_source as { interval_ms: number }).interval_ms = 45_000;
    render(<StoreSourceSection panel={makePanel(cfg)} onConfigChange={vi.fn()} />);
    expect((screen.getByTestId('chart-store-interval') as HTMLSelectElement).value).toBe('custom');
    expect((screen.getByTestId('chart-store-interval-custom') as HTMLInputElement).value).toBe('45');
  });

  it('직접 입력은 초를 ms 로 저장하고, 0 이하는 무시한다', () => {
    const onConfigChange = vi.fn();
    const cfg = infoConfig();
    (cfg.store_source as { interval_ms: number }).interval_ms = 45_000;
    render(<StoreSourceSection panel={makePanel(cfg)} onConfigChange={onConfigChange} />);
    const input = screen.getByTestId('chart-store-interval-custom');

    fireEvent.change(input, { target: { value: '0' } });
    expect(onConfigChange).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '90' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      store_source: { ...(cfg.store_source as object), interval_ms: 90_000 },
    });
  });

  it('인터벌이 시간 윈도우보다 크면 경고를 표시한다', () => {
    const cfg = infoConfig();
    (cfg.store_source as { interval_ms: number }).interval_ms = 86_400_000; // 1d > 1h
    render(<StoreSourceSection panel={makePanel(cfg)} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-interval-warning')).toBeInTheDocument();
  });

  it('정상 범위에서는 경고를 표시하지 않는다', () => {
    render(<StoreSourceSection panel={makePanel(infoConfig())} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-interval-warning')).toBeNull();
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

  it('선택된 시리즈 기준 토큰 버튼을 물음표 뒤에 제공한다', () => {
    render(<StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />);

    // 토큰은 접혀 있다 — 상시 펼치면 입력·미리보기를 밀어낸다.
    expect(screen.queryByTestId('chart-store-series-name-format-token-measurement')).toBeNull();

    fireEvent.click(screen.getByTestId('chart-store-series-name-format-token-help'));

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
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE) + M6.9/M6.10 제자리 반전.
//
// 4개 설정 UI 게이트(StoreInfoPopover · 에이전트 셀렉트 · 이름 형식 · 대표값)의 참 조건을
// 잠근다. 네 지점 모두 이제 `config.data_source` 파생 모드가 `'store'` 인지만 본다 —
// 로컬 `tsdbMode` 상태는 §2.11 [E1] 이 제거했다(CT-21 이 그 이전 동작을 기준선으로 잠갔다).
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
  });

  it("CT-20: data_source:'channel' 이면 4개 게이트가 모두 미렌더다", () => {
    render(
      <StoreSourceSection
        panel={makePanel({ data_source: 'channel', channel_name: 'c1' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(gates()).toEqual([false, false, false, false]);
  });

  it('CT-21 [반전]: TSDB 선택이 config 에 기록되고 tsdb config 에서는 4개 게이트가 미렌더다', () => {
    // [SPEC-TSDB-002 §2.11 반전] SPEC-PANEL-SETTINGS-001 REQ-05 는 TSDB 토글을
    // config 무기록(no-op)으로 규정했고 이 테스트가 그것을 잠갔다. SPEC-TSDB-002 가
    // 그 비목표를 대체하므로 단언을 반전한다. 삭제하지 않는 이유는 "왜 바뀌었는가"의
    // 기록을 diff 밖으로 내보내지 않기 위함이다.
    const onConfigChange = vi.fn();
    const { unmount } = render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />,
    );
    // 전제(무변경): store 모드에서는 4개 게이트가 살아 있다.
    expect(gates()).toEqual([true, true, true, true]);

    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));

    // 반전 (a): 로컬 상태 no-op 이 아니라 config 기록이다.
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ data_source: 'tsdb' }),
    );
    unmount();

    // 반전 (b): 모드가 config 파생이므로 게이트는 tsdb config 에서 닫힌다.
    render(
      <StoreSourceSection
        panel={makePanel({ ...storeConfig(), data_source: 'tsdb' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(gates()).toEqual([false, false, false, false]);
    // 반전 (c): placeholder 는 렌더 트리에서 은퇴했다(testid 소멸).
  });

  it('CT-21 [반전]: 모드가 config 파생이므로 store 로 되돌리면 게이트가 되살아난다', () => {
    // 위와 같은 사유의 반전이다(§2.11). 이전에는 `setTsdbMode(false)` 로 로컬 상태만
    // 풀렸고 config 는 내내 store 였다. 이제는 왕복 자체가 config 를 거치므로, 제어
    // 컴포넌트로 감싸 부모가 패치를 반영해야 모드가 되돌아온다.
    function Harness(): React.ReactElement {
      const [config, setConfig] = useState<Record<string, unknown>>(storeConfig());
      return (
        <StoreSourceSection
          panel={makePanel(config)}
          onConfigChange={(patch) => setConfig((prev) => ({ ...prev, ...patch }))}
        />
      );
    }
    render(<Harness />);
    expect(gates()).toEqual([true, true, true, true]);

    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    expect(gates()).toEqual([false, false, false, false]);

    fireEvent.click(screen.getByTestId('chart-data-source-store'));
    expect(gates()).toEqual([true, true, true, true]);
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 §2.12 [E2] — 소스 전환이 비파괴다 (AC-36)
//
// 되돌리기가 가능해야 사용자가 전환을 시도한다. 파괴적 전환은 "다른 소스를 한번
// 눌러보는" 행위를 되돌릴 수 없는 결정으로 만든다.
// ---------------------------------------------------------------------------
describe('소스 전환 비파괴 (SPEC-TSDB-002 §2.12 [E2], AC-36)', () => {
  it('store → tsdb 전환 시 store_source 가 보존된다', () => {
    const onConfigChange = vi.fn();
    const initial = storeConfig();
    render(
      <StoreSourceSection panel={makePanel(initial)} onConfigChange={onConfigChange} />,
    );

    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));

    const patch = onConfigChange.mock.calls[0]?.[0] as Record<string, unknown>;
    // 패치가 store_source 를 건드리지 않는다 — 종류만 바꾸고 다른 소스의 블록은 그대로 둔다.
    expect(patch).not.toHaveProperty('store_source');
    expect(patch.data_source).toBe('tsdb');
    // 병합된 결과에도 원본 store 블록이 그대로 남는다.
    const merged = { ...initial, ...patch };
    expect(merged.store_source).toEqual(initial.store_source);
  });

  it('tsdb → store 왕복 후 두 블록이 모두 보존된다', () => {
    // 제어 컴포넌트로 감싸 실제 왕복(부모가 패치를 반영)을 재현한다.
    const seen: Array<Record<string, unknown>> = [];
    function Harness(): React.ReactElement {
      const [config, setConfig] = useState<Record<string, unknown>>(storeConfig());
      seen.push(config);
      return (
        <StoreSourceSection
          panel={makePanel(config)}
          onConfigChange={(patch) => setConfig((prev) => ({ ...prev, ...patch }))}
        />
      );
    }
    render(<Harness />);
    const original = seen[0]!.store_source;

    fireEvent.click(screen.getByTestId('chart-data-source-tsdb'));
    fireEvent.click(screen.getByTestId('chart-data-source-store'));

    const final = seen[seen.length - 1]!;
    expect(final.data_source).toBe('store');
    // 왕복 후에도 두 블록이 공존한다 — tsdb 로 갔다 왔다고 store 선택이 사라지지 않고,
    // store 로 돌아왔다고 방금 만든 tsdb 블록이 지워지지도 않는다.
    expect(final.store_source).toEqual(original);
    expect(final.tsdb_source).toBeDefined();
    expect((final.tsdb_source as { backend: string }).backend).toBe('influxdb');
  });
});

describe('가져올 데이터 범위 — 기간(상대·절대) / 갯수', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function rangeConfig(range?: Record<string, unknown>): Record<string, unknown> {
    const store_source = {
      agent_id: 'store-uuid-1',
      agent_name: 'store-1',
      namespace: 'default',
      series: [],
      time_window_ms: 3_600_000,
      interval_ms: 60_000,
      aggregation: 'last',
      refresh_interval_ms: 5_000,
      ...(range ? { range } : {}),
    } as unknown as StoreSourceConfig;
    return { data_source: 'store', store_source };
  }

  it('range 가 없는 구 config 는 상대 기간으로 열리고 창 길이를 그대로 보여준다', () => {
    render(<StoreSourceSection panel={makePanel(rangeConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-range-mode-relative')).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect((screen.getByTestId('chart-store-range-window') as HTMLSelectElement).value)
      .toBe('3600000');
  });

  it('세 방식 모두 선택지로 노출된다', () => {
    render(<StoreSourceSection panel={makePanel(rangeConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-range-mode-relative')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-range-mode-absolute')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-range-mode-count')).toBeInTheDocument();
  });

  it('갯수로 바꾸면 기본 갯수를 채워 저장한다', () => {
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(rangeConfig())} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('chart-store-range-mode-count'));
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.range).toMatchObject({ mode: 'count', count: 100 });
  });

  it('방식을 바꿔도 다른 방식의 값은 지우지 않는다 (되돌리면 살아난다)', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'relative', window_ms: 900_000, count: 42 }))}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('chart-store-range-mode-count'));
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.range).toMatchObject({ mode: 'count', count: 42, window_ms: 900_000 });
  });

  it('절대 구간에서는 시작/끝 입력을 보여준다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'absolute', start_ms: 1, end_ms: 2 }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-range-start')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-range-end')).toBeInTheDocument();
    expect(screen.queryByTestId('chart-store-range-window')).toBeNull();
  });

  it('절대 구간이 뒤집혔거나 비면 경고를 표시한다', () => {
    const { unmount } = render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'absolute', start_ms: 500, end_ms: 100 }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-range-absolute-warning')).toBeInTheDocument();
    unmount();

    render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'absolute' }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-range-absolute-warning')).toBeInTheDocument();
  });

  it('정상 절대 구간에서는 경고가 없다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'absolute', start_ms: 100, end_ms: 500 }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-store-range-absolute-warning')).toBeNull();
  });

  it('갯수 입력은 0 이하를 무시하고 상한으로 클램프한다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel(rangeConfig({ mode: 'count', count: 10 }))}
        onConfigChange={onConfigChange}
      />,
    );
    const input = screen.getByTestId('chart-store-range-count');

    fireEvent.change(input, { target: { value: '0' } });
    expect(onConfigChange).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '999999' } });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.range).toMatchObject({ count: 10_000 });
  });
});
