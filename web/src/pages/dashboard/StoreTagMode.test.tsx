// 태그 인 헤더 — keys/tag 토글 제거 + 목록 헤더의 전용 AND 태그 피커 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (태그 인 헤더) / SPEC-WEB-005
//
// keys/tag 선택 방식 토글이 제거되고, 태그 피커가 PanelSettingsDataSource 의 공용
// 선택 테이블 헤더로 이관됐다. tag_filters 존재로 tag 모드가 함축되며(별도 토글 없음),
// tag 모드에서는 AND 매칭 행이 read-only 미리보기로 표시된다.
// useAgents/useExecAgent/useStoreKeysWithTags/useStoreTagPairs 를 모킹해 네트워크 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';

// i18n 스텁 — 키를 그대로 반환하되, 카운트 보간이 필요한 키만 템플릿을 돌려준다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({
    t: (k: string) =>
      k === 'dashboard.chart.storeTagMatchCount' ? '{count} keys matched' : k,
  }),
}));

// store 에이전트 1개 + StoreEntryTable 이 소비하는 useExecAgent.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] },
  }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

// 키 객체(매칭 계산용) + 태그 페어 목록.
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({
    data: {
      keyObjects: [
        { key: 'room:1:temp', registration: 'auto', data_type: 'float', metric_type: 'gauge', tags: { room: '1', type: 'temperature' } },
        { key: 'room:1:humidity', registration: 'auto', data_type: 'float', metric_type: 'gauge', tags: { room: '1', type: 'humidity' } },
        { key: 'room:2:temp', registration: 'auto', data_type: 'float', metric_type: 'gauge', tags: { room: '2', type: 'temperature' } },
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

import { PanelSettingsDataSource } from './PanelSettingsDataSource';

/** store 모드 패널 — store_source 오버라이드로 keys/tag 상태를 구성한다. */
function makePanel(store_source: Partial<StoreSourceConfig> = {}): PanelConfig {
  const base: StoreSourceConfig = {
    agent_id: 'store-uuid-1',
    agent_name: 'store-1',
    namespace: 'default',
    series: [],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
  };
  return {
    id: 'p1',
    type: 'line-chart',
    title: '테스트',
    config: { data_source: 'store', store_source: { ...base, ...store_source } },
  } as unknown as PanelConfig;
}

beforeEach(() => {
  window.localStorage.clear();
  vi.clearAllMocks();
});

describe('태그 인 헤더 — 토글 제거 + 전용 태그 피커(SPEC-PANEL-SETTINGS-001)', () => {
  it('keys/tag 선택 방식 토글은 더 이상 렌더되지 않는다', () => {
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-selection-mode-keys')).toBeNull();
    expect(screen.queryByTestId('chart-store-selection-mode-tag')).toBeNull();
  });

  it('Store 모드에서 목록과 태그 피커가 항상 함께 노출된다(목록이 사라지지 않음)', () => {
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={vi.fn()} />);
    // 공용 선택 테이블(목록)이 항상 보인다.
    expect(screen.getByTestId('panel-store-select')).toBeInTheDocument();
    // 목록 헤더의 전용 태그 피커.
    expect(screen.getByTestId('chart-store-tag-selection')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-tag-select-room')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-tag-select-type')).toBeInTheDocument();
  });

  it('태그 값 선택 → tag_filters + selection_mode:tag 로 저장(구 편집기와 byte-호환)', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('chart-store-tag-select-room'), {
      target: { value: '1' },
    });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.tag_filters).toEqual({ room: '1' });
    expect(patch.store_source.selection_mode).toBe('tag');
  });

  it('AND 필터: room=1 에 type=humidity 추가 시 tag_filters 에 두 키가 함께 저장된다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('chart-store-tag-select-type'), {
      target: { value: 'humidity' },
    });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.tag_filters).toEqual({ room: '1', type: 'humidity' });
    expect(patch.store_source.selection_mode).toBe('tag');
  });

  it('마지막 태그를 비우면 selection_mode:keys 로 복귀하고 tag_filters 를 제거한다', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    // 기존 태그가 피커에 반영돼 있다(기존 config 프리-채움).
    expect(
      (screen.getByTestId('chart-store-tag-select-room') as HTMLSelectElement).value,
    ).toBe('1');
    fireEvent.change(screen.getByTestId('chart-store-tag-select-room'), {
      target: { value: '' },
    });
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.tag_filters).toBeUndefined();
    expect(patch.store_source.selection_mode).toBe('keys');
  });

  it('tag 모드: AND 매칭 행만 read-only 미리보기로 보여준다(room=1 → 2행, 모두 체크)', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    const select = screen.getByTestId('panel-store-select');
    // room=1 매칭: room:1:temp, room:1:humidity → 2행.
    const checkboxes = within(select).getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    expect(checkboxes.length).toBe(2);
    // 바인딩 집합이므로 모두 체크.
    expect(checkboxes.every((c) => c.checked)).toBe(true);
    // read-only: 클릭해도 config 변경이 발생하지 않는다.
    fireEvent.click(checkboxes[0]!);
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('tag 모드 AND 미리보기: room=1 AND type=humidity → 1행', () => {
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1', type: 'humidity' } })}
        onConfigChange={vi.fn()}
      />,
    );
    const select = screen.getByTestId('panel-store-select');
    const checkboxes = within(select).getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    expect(checkboxes.length).toBe(1);
  });

  it('라이브 매칭 키 수 미리보기(room=1 → 2)', () => {
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('chart-store-tag-match-count').textContent).toContain('2');
  });

  it('keys 모드(태그 필터 없음): 체크박스가 활성이고 클릭 시 series 에 추가된다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={onConfigChange} />);
    const select = screen.getByTestId('panel-store-select');
    // 태그 필터 없음 → 전체 3행이 노출되고 체크박스는 활성.
    const checkboxes = within(select).getAllByLabelText(
      'agents.detail.store.selectRowAriaLabel',
    ) as HTMLInputElement[];
    expect(checkboxes.length).toBe(3);
    expect(checkboxes.every((c) => !c.checked)).toBe(true);
    fireEvent.click(checkboxes[0]!);
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.series.length).toBe(1);
  });
});
