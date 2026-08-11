// 표시 필터 통일 + 바인딩 분리(v0.3.0) — 태그 컬럼은 표시(display) 필터 전용이고,
// 동적 바인딩은 명시적 토글로만 제어된다.
//
// @spec SPEC-PANEL-SETTINGS-001 v0.3.0 (REQ-15 정제 + REQ-22)
//
// v0.2.0 의 "태그 인 헤더 전용 AND 팝오버 → tag_filters + selection_mode 암묵 전환" 모델을
// 폐기하고, (1) 태그를 포함한 모든 컬럼 필터를 통일된 표시 필터(같은 컬럼 OR·컬럼 간 AND)로
// 취급하며, (2) selection_mode 는 명시적 "동적 바인딩" 토글이 단독 제어한다(AC-24).
// useAgents/useExecAgent/useStoreKeysWithTags 를 모킹해 네트워크 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';

// i18n 스텁 — 키를 그대로 반환한다(라벨=키).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// store 에이전트 1개 + StoreEntryTable 이 소비하는 useExecAgent.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] },
  }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

// 키 객체(태그 표시 필터/매칭 계산용).
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
    isFetching: false,
    refetch: vi.fn(),
  }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import { PanelSettingsDataSource } from './PanelSettingsDataSource';

/** store 모드 패널 — store_source 오버라이드로 selection_mode/tag_filters 상태를 구성한다. */
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

/** 태그 컬럼의 표준 필터 버튼(ColumnFilterButton)을 연다. */
function openTagsColumnFilter(): void {
  fireEvent.click(screen.getByTestId('store-filter-agents.detail.store.colTags'));
}

/** 그룹 태그 필터 팝오버에서 특정 "k=v" 값 체크박스를 토글한다. */
function toggleTagValue(pair: string): void {
  const label = screen.getByTitle(pair).closest('label')!;
  fireEvent.click(label.querySelector('input')!);
}

function rowCheckboxes(): HTMLInputElement[] {
  const select = screen.getByTestId('panel-store-select');
  return within(select).getAllByLabelText(
    'agents.detail.store.selectRowAriaLabel',
  ) as HTMLInputElement[];
}

beforeEach(() => {
  window.localStorage.clear();
  vi.clearAllMocks();
});

describe('표시 필터 통일 + 바인딩 분리 (SPEC-PANEL-SETTINGS-001 v0.3.0)', () => {
  it('구 keys/tag 선택 방식 토글은 렌더되지 않는다', () => {
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('chart-store-selection-mode-keys')).toBeNull();
    expect(screen.queryByTestId('chart-store-selection-mode-tag')).toBeNull();
    // 구 전용 태그 팝오버(태그 인 헤더)도 제거됐다.
    expect(screen.queryByTestId('panel-store-tags-header-filter')).toBeNull();
  });

  it('AC-24(a): 신규 패널은 동적 바인딩 토글 OFF, 전체 행 표시', () => {
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={vi.fn()} />);
    const toggle = screen.getByTestId('chart-dynamic-binding-toggle') as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    expect(rowCheckboxes().length).toBe(3);
  });

  it('AC-24(a) 표시/바인딩 분리: 토글 OFF 에서 태그 필터는 표시 행만 좁히고 selection_mode 를 바꾸지 않는다', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={onConfigChange} />);
    expect(rowCheckboxes().length).toBe(3);
    openTagsColumnFilter();
    toggleTagValue('room=1');
    // 표시 행만 좁혀짐: room=1 매칭 2행.
    expect(rowCheckboxes().length).toBe(2);
    // 표시 전용 — config(selection_mode/tag_filters) 미변경.
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('AC-24(b): 동적 바인딩 토글 ON → selection_mode:tag', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('chart-dynamic-binding-toggle'));
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.selection_mode).toBe('tag');
  });

  it('AC-24(b): 동적 바인딩 ON 에서 태그 표시 필터 변경 → tag_filters 파생(모드 tag 유지)', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag' })}
        onConfigChange={onConfigChange}
      />,
    );
    openTagsColumnFilter();
    toggleTagValue('room=1');
    const patch = onConfigChange.mock.calls.at(-1)![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.selection_mode).toBe('tag');
    expect(patch.store_source.tag_filters).toEqual({ room: '1' });
  });

  it('AC-24(c) 하위호환: 기존 selection_mode:tag 패널은 토글 ON 로드 + tag_filters 기준 표시 시드', () => {
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(
      (screen.getByTestId('chart-dynamic-binding-toggle') as HTMLInputElement).checked,
    ).toBe(true);
    // 저장된 tag_filters(room=1) 기준으로 표시가 좁혀진다(시드) → 매칭 2행.
    expect(rowCheckboxes().length).toBe(2);
  });

  it('AC-24(c) 하위호환: 기존 tag 패널 로드만으로 config 를 변경하지 않는다(보존)', () => {
    const onConfigChange = vi.fn();
    render(
      <PanelSettingsDataSource
        panel={makePanel({ selection_mode: 'tag', tag_filters: { room: '1' } })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('keys 모드(신규): 체크박스 클릭 → series 추가, selection_mode 미강제', () => {
    const onConfigChange = vi.fn();
    render(<PanelSettingsDataSource panel={makePanel()} onConfigChange={onConfigChange} />);
    const checkboxes = rowCheckboxes();
    expect(checkboxes.length).toBe(3);
    expect(checkboxes.every((c) => !c.checked)).toBe(true);
    fireEvent.click(checkboxes[0]!);
    const patch = onConfigChange.mock.calls[0]![0] as { store_source: StoreSourceConfig };
    expect(patch.store_source.series.length).toBe(1);
    expect(patch.store_source.selection_mode).toBeUndefined();
  });
});
