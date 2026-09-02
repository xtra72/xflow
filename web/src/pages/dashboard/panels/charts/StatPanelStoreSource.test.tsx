// StatPanel 의 data_source 분기 테스트(SPEC-WEB-005).
// useChartChannel / useStoreChartData 를 모두 모킹해, config.data_source 에 따라
// 올바른 소스가 활성화되는지 검증한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';

const channelResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

const storeResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    seriesEntries: new Map<string, ChartEntry[]>(),
    seriesNames: [] as string[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

// 어떤 인자로 호출됐는지 추적해 비활성 경로가 idle 로 호출되는지 확인한다.
const channelCalls = vi.hoisted(() => ({ args: [] as unknown[] }));
const storeCalls = vi.hoisted(() => ({ args: [] as unknown[] }));

vi.mock('./useChartChannel', () => ({
  useChartChannel: (channelName: string | undefined) => {
    channelCalls.args.push(channelName);
    return channelResult.current;
  },
}));

// useStoreChartData 는 훅만 교체하고 나머지 export(matrixToEntries 등)는 실제 구현을
// 유지한다. 특성화 픽스처를 손으로 만들지 않고 실제 변환 규칙으로 만들기 위함이다.
vi.mock('./useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (config: unknown, enabled: boolean) => {
      storeCalls.args.push({ config, enabled });
      return storeResult.current;
    },
  };
});

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { SeriesMatrix } from '@/services/api/seriesDataSource';
import type { StoreSourceConfig } from './chartChannelTypes';
import { matrixToEntries } from './useStoreChartData';
import StatPanel from './StatPanel';

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// acceptance.md 공통 픽스처 F1(3시리즈 × 5버킷)을 실제 matrixToEntries 로 변환해
// "다중 시리즈 + series_reduce 부재" 의 현재 렌더 결과를 잠근다.
// ---------------------------------------------------------------------------

/** F1 — 3시리즈 매트릭스(temp.room3 는 전 버킷 null). */
const F1_MATRIX: SeriesMatrix = {
  columns: ['k.room1', 'k.room2', 'k.room3'],
  rows: [
    { bucketStartMs: 1000, values: [20, 18, null] },
    { bucketStartMs: 2000, values: [22, null, null] },
    { bucketStartMs: 3000, values: [26, 19, null] },
    { bucketStartMs: 4000, values: [24, 19, null] },
    { bucketStartMs: 5000, values: [21, 23, null] },
  ],
};

const F1_CONFIG: StoreSourceConfig = {
  agent_name: 'store-1',
  namespace: 'default',
  selection_mode: 'keys',
  series: [
    { key: 'k.room1', alias: 'temp.room1' },
    { key: 'k.room2', alias: 'temp.room2' },
    { key: 'k.room3', alias: 'temp.room3' },
  ],
  time_window_ms: 60_000,
  interval_ms: 1_000,
  aggregation: 'average',
};

describe('StatPanel Store 다중 시리즈 특성화 (SPEC-CHART-002 M2)', () => {
  it('CH-04: store 다중 시리즈 + series_reduce 부재 → 평탄화 마지막 값 1개만 표시한다', () => {
    const derived = matrixToEntries(F1_MATRIX, F1_CONFIG);
    // 전제: 시리즈 축은 이미 존재하지만(3개) StatPanel 은 이를 읽지 않는다.
    expect(derived.seriesNames).toEqual(['temp.room1', 'temp.room2', 'temp.room3']);
    expect(derived.seriesEntries.size).toBe(3);

    storeResult.current = {
      ...storeResult.current,
      entries: derived.entries,
      seriesEntries: derived.seriesEntries,
      seriesNames: derived.seriesNames,
    };

    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c1',
          decimal_places: 0,
          data_source: 'store',
          store_source: F1_CONFIG,
        }}
      />,
    );

    // 타일 배열이 아니라 값 슬롯 1개.
    expect(screen.getAllByTestId('stat-value')).toHaveLength(1);
    // 평탄화 타임라인은 timestamp 오름차순 + 동일 timestamp 내 시리즈 순서다. 마지막
    // 버킷(5000)의 마지막 수치 표본은 temp.room2 의 23 이다(temp.room3 는 전부 null
    // 이라 평탄화 타임라인에 아예 없다).
    expect(screen.getByTestId('stat-value').textContent).toContain('23');
    // 어떤 시리즈 이름도 화면에 나타나지 않는다(레거시 경로는 라벨을 그리지 않는다).
    expect(screen.queryByText('temp.room1')).toBeNull();
    expect(screen.queryByText('temp.room2')).toBeNull();
  });

  it('CH-04: 보조 delta 는 시리즈 구분 없이 평탄화 직전 entry 와 비교한다', () => {
    const derived = matrixToEntries(F1_MATRIX, F1_CONFIG);
    storeResult.current = {
      ...storeResult.current,
      entries: derived.entries,
      seriesEntries: derived.seriesEntries,
      seriesNames: derived.seriesNames,
    };

    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c1',
          decimal_places: 0,
          data_source: 'store',
          store_source: F1_CONFIG,
        }}
      />,
    );

    // 마지막 = temp.room2 의 23, 직전 = temp.room1 의 21 → +2(↑).
    // 즉 delta 는 "같은 시리즈의 변화량" 이 아니라 서로 다른 시리즈 간 차이다.
    // 이는 현재 구현의 사실이며 특성화 대상이다(개선 여부는 본 마일스톤 범위 밖).
    const delta = screen.getByTestId('stat-delta');
    expect(delta.textContent).toContain('↑');
    expect(delta.textContent).toContain('+2');
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `StatPanel.tsx:80` 의 소스 활성 판정을 5분기로 잠근다. 여기에 더해 `series_reduce`
// 부재 시 **레거시(단일 값) 렌더가 유지**된다는 사실도 함께 잠근다 — M3 의 치환이
// 대표값 경로를 건드리지 않아야 한다(SPEC-CHART-002 §2.9 소유).
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.1 CT-01 ~ CT-05 / AC-09
// ---------------------------------------------------------------------------