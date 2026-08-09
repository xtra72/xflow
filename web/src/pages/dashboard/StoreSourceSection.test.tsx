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
          metric_type: 'gauge',
          tags: { room: '1' },
        },
        {
          key: 'room:2:humidity',
          registration: 'manual',
          data_type: 'int',
          metric_type: 'counter',
          tags: { room: '2' },
        },
        {
          key: 'system:status',
          registration: 'auto',
          data_type: 'string',
          metric_type: 'state',
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

/** 라인 차트 패널(통합 per-line 스타일 편집 노출). */
function makeLinePanel(config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type: 'line-chart', title: '테스트', config };
}

/** store 모드 + 시리즈 1개를 가진 기본 config. */
function storeConfig(seriesOverride?: Partial<StoreSourceConfig['series'][number]>): Record<string, unknown> {
  const store_source: StoreSourceConfig = {
    agent_name: 'store-1',
    namespace: 'default',
    series: [{ key: 'room:1:temp', metric_type: 'gauge', tags: { room: '1' }, ...seriesOverride }],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
  };
  return { data_source: 'store', store_source };
}

describe('StoreSourceSection 시리즈 이름(alias) 편집', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('store 모드에서 선택된 시리즈 목록과 이름 입력을 렌더한다', () => {
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.getByTestId('chart-store-selected-series')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-series-row-0')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-series-alias-0')).toBeInTheDocument();
  });

  it('이름 입력 시 series[i].alias 가 갱신된 store_source 가 저장된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />,
    );
    const input = screen.getByTestId('chart-store-series-alias-0');
    fireEvent.change(input, { target: { value: '실내 온도' } });

    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series[0]!.alias).toBe('실내 온도');
    // 키/메타데이터는 보존된다.
    expect(patch.store_source.series[0]!.key).toBe('room:1:temp');
    expect(patch.store_source.series[0]!.metric_type).toBe('gauge');
  });

  it('빈 문자열 입력 시 alias 를 undefined 로 저장한다(키명 폴백)', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel(storeConfig({ alias: '기존 이름' }))}
        onConfigChange={onConfigChange}
      />,
    );
    const input = screen.getByTestId('chart-store-series-alias-0');
    // 기존 alias 가 입력값으로 반영되어야 한다.
    expect((input as HTMLInputElement).value).toBe('기존 이름');

    fireEvent.change(input, { target: { value: '   ' } });
    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series[0]!.alias).toBeUndefined();
  });

  it('시리즈 제거 버튼이 해당 시리즈를 series[] 에서 제외한다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />,
    );
    fireEvent.click(screen.getByTestId('chart-store-series-remove-0'));
    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series).toHaveLength(0);
  });
});

describe('StoreSourceSection 라인 차트 통합 per-line 스타일(store 모드)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('stat 패널에서는 선택 시리즈 행에 라인 스타일 펼침 토글이 없다', () => {
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.queryByTestId('chart-store-series-expand-0')).toBeNull();
  });

  it('라인 차트 패널에서는 선택 시리즈 행에 라인 스타일 펼침 토글이 있다', () => {
    render(
      <StoreSourceSection panel={makeLinePanel(storeConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.getByTestId('chart-store-series-expand-0')).toBeInTheDocument();
  });

  it('펼침 후 stroke_style 변경 시 store_source.series[i].stroke_style 가 갱신된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makeLinePanel(storeConfig())}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('chart-store-series-expand-0'));
    const styleSelect = screen.getByTestId('chart-store-series-0-stroke-style');
    fireEvent.change(styleSelect, { target: { value: 'dashed' } });

    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series[0]!.stroke_style).toBe('dashed');
    // 키/메타데이터는 보존된다.
    expect(patch.store_source.series[0]!.key).toBe('room:1:temp');
  });

  it('펼침 후 두께/곡선/표시필드 변경이 series[i] 에 반영된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makeLinePanel(storeConfig())}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('chart-store-series-expand-0'));

    fireEvent.change(screen.getByTestId('chart-store-series-0-stroke-width'), {
      target: { value: '4' },
    });
    expect(
      (onConfigChange.mock.calls.at(-1)![0] as { store_source: StoreSourceConfig })
        .store_source.series[0]!.stroke_width,
    ).toBe(4);

    fireEvent.click(screen.getByTestId('chart-store-series-0-smooth'));
    expect(
      (onConfigChange.mock.calls.at(-1)![0] as { store_source: StoreSourceConfig })
        .store_source.series[0]!.smooth,
    ).toBe(true);

    fireEvent.change(screen.getByTestId('chart-store-series-0-display-field'), {
      target: { value: 'value.inner' },
    });
    expect(
      (onConfigChange.mock.calls.at(-1)![0] as { store_source: StoreSourceConfig })
        .store_source.series[0]!.display_field,
    ).toBe('value.inner');
  });
});

/** 다중 태그를 가진 store config(토큰 버튼/미리보기 테스트용). */
function taggedStoreConfig(
  seriesOverride?: Partial<StoreSourceConfig['series'][number]>,
): Record<string, unknown> {
  const store_source: StoreSourceConfig = {
    agent_name: 'store-1',
    namespace: 'default',
    series: [
      {
        key: 'room:1:temp',
        metric_type: 'gauge',
        tags: { name: 'TempSensor', type: 'inside' },
        ...seriesOverride,
      },
    ],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
  };
  return { data_source: 'store', store_source };
}

describe('StoreSourceSection alias 태그 토큰 템플릿(SPEC-WEB-005)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('태그가 있는 시리즈는 태그 키마다 토큰 삽입 버튼을 노출한다', () => {
    render(
      <StoreSourceSection panel={makePanel(taggedStoreConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.getByTestId('chart-store-series-token-0-name')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-series-token-0-type')).toBeInTheDocument();
    // 버튼 라벨은 {$.key} 토큰 형태다.
    expect(screen.getByTestId('chart-store-series-token-0-name').textContent).toBe(
      '{$.name}',
    );
  });

  it('토큰 버튼 클릭 시 alias 에 {$.key} 토큰이 삽입된다(빈 alias → 끝에 추가)', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel(taggedStoreConfig())}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('chart-store-series-token-0-name'));
    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series[0]!.alias).toBe('{$.name}');
  });

  it('미리보기가 해석된 이름을 보여준다(여러 토큰 + 리터럴)', () => {
    render(
      <StoreSourceSection
        panel={makePanel(taggedStoreConfig({ alias: '{$.name}-{$.type}' }))}
        onConfigChange={vi.fn()}
      />,
    );
    const preview = screen.getByTestId('chart-store-series-preview-0');
    // i18n 스텁은 키를 그대로 반환하므로 '{value}' 슬롯이 해석값으로 치환된다.
    expect(preview.textContent).toContain('TempSensor-inside');
  });

  it('누락 태그 토큰은 미리보기에서 빈 문자열로 해석된다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(taggedStoreConfig({ alias: '{$.name}-{$.missing}' }))}
        onConfigChange={vi.fn()}
      />,
    );
    const preview = screen.getByTestId('chart-store-series-preview-0');
    expect(preview.textContent).toContain('TempSensor-');
    expect(preview.textContent).not.toContain('missing');
  });

  it('alias 가 비어있으면 미리보기는 키명으로 폴백한다', () => {
    render(
      <StoreSourceSection panel={makePanel(taggedStoreConfig())} onConfigChange={vi.fn()} />,
    );
    const preview = screen.getByTestId('chart-store-series-preview-0');
    expect(preview.textContent).toContain('room:1:temp');
  });

  it('태그가 없는 시리즈는 토큰 서브행을 렌더하지 않는다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(taggedStoreConfig({ tags: undefined }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-store-series-tokens-0')).toBeNull();
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
