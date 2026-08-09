// StoreSourceSection tag 자동 모드 UI 테스트 (SPEC-WEB-005).
// 시리즈 선택 방식 토글(키 직접 선택 ↔ 태그로 자동)과 태그 선택기를 검증한다.
// useAgents / useStoreKeysWithTags / useStoreTagPairs 를 모킹해 네트워크 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';

// i18n 스텁 — 키를 그대로 반환하되, 카운트 보간이 필요한 키만 템플릿을 돌려준다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({
    t: (k: string) =>
      k === 'dashboard.chart.storeTagMatchCount' ? '{count} keys matched' : k,
  }),
}));

// store 에이전트 1개.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] },
  }),
}));

// 키 객체(매칭 카운트 계산용) + 태그 페어 목록.
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({
    data: {
      keyObjects: [
        {
          key: 'room:1:temp',
          registration: 'auto',
          data_type: 'float',
          metric_type: 'gauge',
          tags: { room: '1', type: 'temperature' },
        },
        {
          key: 'room:1:humidity',
          registration: 'auto',
          data_type: 'float',
          metric_type: 'gauge',
          tags: { room: '1', type: 'humidity' },
        },
        {
          key: 'room:2:temp',
          registration: 'auto',
          data_type: 'float',
          metric_type: 'gauge',
          tags: { room: '2', type: 'temperature' },
        },
      ],
    },
    isLoading: false,
    isError: false,
  }),
  useStoreTagPairs: () => ({
    data: [
      { key: 'room', values: ['1', '2'] },
      { key: 'type', values: ['temperature', 'humidity'] },
    ],
    isLoading: false,
    isError: false,
  }),
}));

import { StoreSourceSection } from './ChartPanelSections';

function makePanel(config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type: 'line-chart', title: '테스트', config };
}

/** keys 모드(기본) store config. */
function keysConfig(): Record<string, unknown> {
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
  return { data_source: 'store', store_source };
}

/** tag 모드 store config(필터 지정). */
function tagConfig(tag_filters: Record<string, string> = {}): Record<string, unknown> {
  const store_source: StoreSourceConfig = {
    agent_id: 'store-uuid-1',
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'tag',
    tag_filters,
    series: [],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
  };
  return { data_source: 'store', store_source };
}

describe('StoreSourceSection 시리즈 선택 방식 토글(SPEC-WEB-005)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('에이전트 선택 시 키/태그 모드 토글이 노출된다', () => {
    render(<StoreSourceSection panel={makePanel(keysConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-selection-mode-keys')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-selection-mode-tag')).toBeInTheDocument();
  });

  it('기본(미지정) 은 keys 모드 → 태그 선택기는 없다(키 선택은 공용 StoreEntryTable 로 외부화)', () => {
    render(<StoreSourceSection panel={makePanel(keysConfig())} onConfigChange={vi.fn()} />);
    // keys 모드에서는 태그 자동 선택기가 없다. 키 선택 체크박스 테이블은 상위
    // PanelSettingsDataSource 의 공용 StoreEntryTable 로 이동했으므로 여기(StoreSourceSection)
    // 에는 렌더되지 않는다.
    expect(screen.queryByTestId('chart-store-tag-selection')).toBeNull();
    // keys 토글이 선택 상태다.
    expect(screen.getByTestId('chart-store-selection-mode-keys')).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('"태그로 자동" 클릭 시 selection_mode:tag + series:[] 로 저장한다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(keysConfig())} onConfigChange={onConfigChange} />,
    );
    fireEvent.click(screen.getByTestId('chart-store-selection-mode-tag'));
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.selection_mode).toBe('tag');
    // 모드 전환 시 series[] 는 초기화된다.
    expect(patch.store_source.series).toEqual([]);
  });

  it('tag 모드 config 는 태그 선택기를 렌더하고 키 테이블은 없다', () => {
    render(<StoreSourceSection panel={makePanel(tagConfig())} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('chart-store-tag-selection')).toBeInTheDocument();
    expect(screen.queryByTestId('chart-store-key-table')).toBeNull();
    // 태그 키별 셀렉트가 노출된다.
    expect(screen.getByTestId('chart-store-tag-select-room')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-tag-select-type')).toBeInTheDocument();
  });

  it('태그 값 선택 시 tag_filters 에 key:value 가 저장된다(selection_mode 유지)', () => {
    const onConfigChange = vi.fn();
    render(<StoreSourceSection panel={makePanel(tagConfig())} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-tag-select-room'), {
      target: { value: '1' },
    });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.tag_filters).toEqual({ room: '1' });
    expect(patch.store_source.selection_mode).toBe('tag');
  });

  it('선택된 태그 값을 비우면 해당 키가 tag_filters 에서 제거된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel(tagConfig({ room: '1' }))}
        onConfigChange={onConfigChange}
      />,
    );
    // 현재 값이 반영되어 있다.
    expect(
      (screen.getByTestId('chart-store-tag-select-room') as HTMLSelectElement).value,
    ).toBe('1');
    fireEvent.change(screen.getByTestId('chart-store-tag-select-room'), {
      target: { value: '' },
    });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.tag_filters).toEqual({});
  });

  it('라이브 매칭 키 수를 미리보기로 표시한다(room=1 → 2개)', () => {
    render(
      <StoreSourceSection panel={makePanel(tagConfig({ room: '1' }))} onConfigChange={vi.fn()} />,
    );
    // room=1 을 가진 distinct 키: room:1:temp, room:1:humidity → 2.
    expect(screen.getByTestId('chart-store-tag-match-count').textContent).toContain('2');
  });

  it('AND 필터: room=1 AND type=humidity → 1개', () => {
    render(
      <StoreSourceSection
        panel={makePanel(tagConfig({ room: '1', type: 'humidity' }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-tag-match-count').textContent).toContain('1');
  });
});
