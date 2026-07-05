// StoreSourceSection 의 시리즈 표시 이름(alias) 편집 테스트 (SPEC-WEB-005).
// useAgents / useStoreKeysWithTags 를 모킹해 네트워크 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';

// i18n 스텁 — 보간 슬롯({key})을 가진 키는 치환해 반환한다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({
    t: (k: string) => k,
  }),
}));

// 에이전트 목록 — store 에이전트 1개.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: { data: [{ name: 'store-1', type: 'store' }] },
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

/** 선택된 시리즈가 없는 store config(후보 테이블 행이 비선택 상태로 렌더됨). */
function emptyStoreConfig(): Record<string, unknown> {
  const store_source: StoreSourceConfig = {
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

describe('StoreSourceSection 후보 키 테이블', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('후보 키 목록을 테이블(키/메트릭/데이터/태그 컬럼)로 렌더한다', () => {
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={vi.fn()} />,
    );
    // 테이블과 정렬 가능한 컬럼 헤더가 렌더된다.
    const table = screen.getByTestId('chart-store-key-table');
    expect(table).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-sort-key')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-sort-metric_type')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-sort-data_type')).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-sort-tags')).toBeInTheDocument();

    // 첫 행 셀 내용(키/metric/data_type)이 렌더되고, 태그는 "키: 값" 칩이다.
    const row = screen.getByTestId('chart-store-key-row-0');
    expect(row.textContent).toContain('room:1:temp');
    expect(row.textContent).toContain('gauge'); // metric_type 값
    expect(row.textContent).toContain('float'); // data_type 값
    // 태그 칩은 "키: 값" 형식(연결 문자열 'room=1' 아님).
    expect(row.textContent).toContain('room: 1');
    expect(row.textContent).not.toContain('room=1');
  });

  it('필터는 컬럼 헤더 안에 인라인으로 배치된다(별도 필터 바 없음)', () => {
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={vi.fn()} />,
    );
    // 헤더 내 필터 컨트롤이 테이블 내부에 존재한다.
    const table = screen.getByTestId('chart-store-key-table');
    expect(table.contains(screen.getByTestId('chart-store-key-search'))).toBe(true);
    expect(table.contains(screen.getByTestId('chart-store-metric-filter'))).toBe(true);
    expect(table.contains(screen.getByTestId('chart-store-datatype-filter'))).toBe(true);
    expect(table.contains(screen.getByTestId('chart-store-tag-filter'))).toBe(true);
  });

  it('키 컬럼 헤더 클릭 시 행이 키 기준으로 정렬된다(asc → desc)', () => {
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={vi.fn()} />,
    );
    const keyOf = (idx: number): string =>
      screen.getByTestId(`chart-store-key-row-${idx}`).querySelector('td:nth-child(2)')!
        .textContent ?? '';

    // 정렬 전 원래 순서: room:1:temp, room:2:humidity, system:status.
    expect(keyOf(0)).toBe('room:1:temp');

    // asc 정렬: 사전순(localeCompare).
    fireEvent.click(screen.getByTestId('chart-store-sort-key'));
    expect(keyOf(0)).toBe('room:1:temp');
    expect(keyOf(2)).toBe('system:status');

    // 재클릭 → desc.
    fireEvent.click(screen.getByTestId('chart-store-sort-key'));
    expect(keyOf(0)).toBe('system:status');
    expect(keyOf(2)).toBe('room:1:temp');
  });

  it('헤더 인라인 검색 필터가 행을 좁힌다', () => {
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={vi.fn()} />,
    );
    fireEvent.change(screen.getByTestId('chart-store-key-search'), {
      target: { value: 'humidity' },
    });
    expect(screen.getByTestId('chart-store-key-row-0').textContent).toContain(
      'room:2:humidity',
    );
    // 다른 키 행은 사라진다.
    expect(screen.queryByTestId('chart-store-key-row-1')).toBeNull();
  });

  it('구조화 태그 필터: 태그 키 선택 → 값 칩 토글로 행을 좁힌다', () => {
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={vi.fn()} />,
    );
    // 태그 키 'room' 선택 시 값 칩(1, 2)이 노출된다.
    fireEvent.change(screen.getByTestId('chart-store-tag-key-select'), {
      target: { value: 'room' },
    });
    const chip1 = screen.getByTestId('chart-store-tag-value-room-1');
    expect(chip1).toBeInTheDocument();
    expect(screen.getByTestId('chart-store-tag-value-room-2')).toBeInTheDocument();

    // room=1 칩 토글 → room=1 태그를 가진 행만 남는다.
    fireEvent.click(chip1);
    expect(screen.getByTestId('chart-store-key-row-0').textContent).toContain(
      'room:1:temp',
    );
    expect(screen.queryByTestId('chart-store-key-row-1')).toBeNull();
    // 선택된 태그 요약 칩이 "키: 값" 으로 표시된다.
    expect(screen.getByTestId('chart-store-tag-selected').textContent).toContain('room: 1');
  });

  it('행 체크박스 선택 시 store_source.series[] 에 시리즈가 추가된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(emptyStoreConfig())} onConfigChange={onConfigChange} />,
    );
    const checkbox = screen.getByTestId('chart-store-key-checkbox-room:1:temp');
    expect((checkbox as HTMLInputElement).checked).toBe(false);

    fireEvent.click(checkbox);
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as {
      store_source: StoreSourceConfig;
    };
    expect(patch.store_source.series).toHaveLength(1);
    expect(patch.store_source.series[0]!.key).toBe('room:1:temp');
    expect(patch.store_source.series[0]!.metric_type).toBe('gauge');
    expect(patch.store_source.series[0]!.tags).toEqual({ room: '1' });
  });

  it('이미 선택된 키의 행 체크박스는 체크 상태로 렌더된다', () => {
    // storeConfig() 는 room:1:temp(gauge, room=1)를 선택 상태로 가진다.
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={vi.fn()} />,
    );
    const checkbox = screen.getByTestId('chart-store-key-checkbox-room:1:temp');
    expect((checkbox as HTMLInputElement).checked).toBe(true);
  });

  it('선택된 행 체크박스 해제 시 해당 시리즈가 series[] 에서 제거된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StoreSourceSection panel={makePanel(storeConfig())} onConfigChange={onConfigChange} />,
    );
    const checkbox = screen.getByTestId('chart-store-key-checkbox-room:1:temp');
    fireEvent.click(checkbox);
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
