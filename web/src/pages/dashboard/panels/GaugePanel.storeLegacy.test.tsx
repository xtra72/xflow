// GaugePanel 레거시 store 바인딩 특성화 테스트 (SPEC-CHART-002 M2 / DDD PRESERVE).
//
// 대상: GaugePanel.tsx 의 `useStoreLatestValue` — `config.dataSources[]` 의
// `sourceType:'store'` 항목을 `POST /store/{agent}/query` 로 `mode:'latest'` 5초 폴링한다.
// 시간창 · 버킷 집계 · 다중 시리즈 · 태그 바인딩 개념이 전혀 없는 경로이며(spec.md §1.2.2),
// M4/M5 가 이 흐름 옆에 신규 store_source 경로를 끼워 넣는다. 그 전에 요청 형상 ·
// 폴링 주기 · 실패 시 값 유지 규칙을 여기서 고정한다.
//
// 잠그는 특성화 ID: CH-14(요청 형상 + 5초 인터벌), CH-18(폴링 실패 시 이전 값 유지).
// spec.md §2.9 [S1] / §2.13 [UB2] 3,4.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen } from '@testing-library/react';

const mockChannel = vi.hoisted(() => ({
  current: {
    entries: [] as Array<{ timestamp: number; value: unknown }>,
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
  lastCalledWith: undefined as string | undefined,
}));

vi.mock('./charts/useChartChannel', () => ({
  useChartChannel: (channelName: string | undefined) => {
    mockChannel.lastCalledWith = channelName;
    return mockChannel.current;
  },
}));

// 에이전트 목록은 테스트마다 바꿀 수 있어야 한다(storeAgentId → 현재 이름 해석 검증).
const mockAgents = vi.hoisted(() => ({
  list: [] as Array<{ id: string; name: string }>,
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: mockAgents.list } }),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

const mockPost = vi.hoisted(() => ({
  fn: vi.fn(async (_url: string, _body: unknown) => ({
    entries: [] as Array<{ value: unknown; timestamp: number }>,
  })),
}));

vi.mock('@/services/api/client', () => ({
  post: (url: string, body: unknown) => mockPost.fn(url, body),
}));

import GaugePanel from './GaugePanel';

/** F2 의 store 레거시 바인딩 항목. */
const F2_STORE_SOURCE = {
  sourceType: 'store',
  storeAgentId: 'a1',
  storeAgent: 'store-a',
  storeKey: 'k1',
  storeNamespace: 'default',
};

function renderPanel(config: Record<string, unknown>) {
  return render(<GaugePanel panelId="p1" title="테스트 게이지" config={config} />);
}

/** 마운트 직후 즉시 실행되는 fetch 의 마이크로태스크를 비운다. */
async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

/** 가짜 타이머를 ms 만큼 진행시키고 그 사이 발생한 비동기 갱신을 반영한다. */
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe('GaugePanel store 레거시 폴링 특성화 (SPEC-CHART-002 M2)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockChannel.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    mockChannel.lastCalledWith = undefined;
    mockAgents.list = [];
    mockPost.fn.mockReset();
    mockPost.fn.mockResolvedValue({ entries: [] });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("CH-14: mode:'latest' 단일 키 요청 형상으로 폴링하고 값을 표시한다", async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      unit: '%',
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();

    // 요청 형상: 경로는 에이전트 "이름" 주소이고 본문은 key / mode / namespace 3개뿐이다.
    // 시간창(start/end) · 버킷(interval) · 집계(aggregation) · 다중 키 개념이 없다.
    expect(mockPost.fn).toHaveBeenCalledTimes(1);
    expect(mockPost.fn).toHaveBeenCalledWith('/store/store-a/query', {
      key: 'k1',
      mode: 'latest',
      namespace: 'default',
    });
    expect(screen.getByText('42.00')).toBeInTheDocument();
    expect(screen.getByText('%')).toBeInTheDocument();
  });

  it('CH-14: 폴링 주기는 5초이며 그 전에는 재요청하지 않는다', async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();
    expect(mockPost.fn).toHaveBeenCalledTimes(1); // 마운트 즉시 1회

    await advance(4_999);
    expect(mockPost.fn).toHaveBeenCalledTimes(1); // 5초 미만에는 늘지 않는다

    await advance(1);
    expect(mockPost.fn).toHaveBeenCalledTimes(2); // 정확히 5000ms 에서 2회차

    await advance(5_000);
    expect(mockPost.fn).toHaveBeenCalledTimes(3);
  });

  it('CH-14: storeNamespace 미지정이면 default 로 요청한다', async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 7, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [{ sourceType: 'store', storeAgent: 'store-a', storeKey: 'k1' }],
    });
    await flush();

    expect(mockPost.fn).toHaveBeenCalledWith('/store/store-a/query', {
      key: 'k1',
      mode: 'latest',
      namespace: 'default',
    });
  });

  it('CH-14: storeAgentId 가 현재 에이전트 이름으로 해석되어 호출된다(SPEC-WEB-006 승계)', async () => {
    // 저장된 이름은 store-a 이지만 에이전트가 store-renamed 로 개명된 상황.
    mockAgents.list = [{ id: 'a1', name: 'store-renamed' }];
    mockPost.fn.mockResolvedValue({ entries: [{ value: 9, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();

    expect(mockPost.fn).toHaveBeenCalledWith('/store/store-renamed/query', {
      key: 'k1',
      mode: 'latest',
      namespace: 'default',
    });
  });

  it("CH-14: displayField 가 'value' 가 아니면 값을 해석하지 못하고 -- 가 된다", async () => {
    // 레거시 store 경로는 응답의 entries[0].value 만 읽는다. dot-path 해석이 없으므로
    // displayField 가 'value' 이외이면 raw 가 undefined 가 되어 값이 사라진다.
    mockPost.fn.mockResolvedValue({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 77,
      min: 0,
      max: 100,
      dataSources: [{ ...F2_STORE_SOURCE, displayField: 'labels.temp' }],
    });
    await flush();

    expect(mockPost.fn).toHaveBeenCalled();
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('42.00')).toBeNull();
    expect(screen.queryByText('77.00')).toBeNull(); // static 폴백 없음
  });

  it('CH-14: 에이전트 이름을 해석할 수 없으면 폴링하지 않고 -- 를 표시한다', async () => {
    // storeAgentId 만 있고 storeAgent 스냅샷이 없으며 에이전트 목록도 비어 있으면
    // 해석 결과가 빈 문자열이라 요청 자체가 나가지 않는다. 그래도 바인딩은 "있음"
    // 으로 판정되므로 static config.value 로 폴백하지 않는다.
    renderPanel({
      gaugeType: 'simple',
      value: 77,
      min: 0,
      max: 100,
      dataSources: [{ sourceType: 'store', storeAgentId: 'a1', storeKey: 'k1' }],
    });
    await flush();

    expect(mockPost.fn).not.toHaveBeenCalled();
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('77.00')).toBeNull();
  });

  it('CH-18: 폴링이 실패해도 마지막 성공 값을 유지한다', async () => {
    mockPost.fn.mockResolvedValueOnce({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();
    expect(screen.getByText('42.00')).toBeInTheDocument();

    // 2회차 실패(네트워크 오류) → catch 로 삼키고 이전 값 유지.
    mockPost.fn.mockRejectedValue(new Error('network down'));
    await advance(5_000);
    expect(mockPost.fn).toHaveBeenCalledTimes(2);
    expect(screen.getByText('42.00')).toBeInTheDocument();
    expect(screen.queryByText('--')).toBeNull();

    // 3회차도 실패 — 여전히 유지된다.
    await advance(5_000);
    expect(mockPost.fn).toHaveBeenCalledTimes(3);
    expect(screen.getByText('42.00')).toBeInTheDocument();
  });

  it('CH-18: 응답에 entries 가 비어 있어도 이전 값을 유지한다', async () => {
    mockPost.fn.mockResolvedValueOnce({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();
    expect(screen.getByText('42.00')).toBeInTheDocument();

    // 성공했지만 entries 0개 → setValue 를 호출하지 않아 이전 값이 남는다.
    mockPost.fn.mockResolvedValue({ entries: [] });
    await advance(5_000);
    expect(screen.getByText('42.00')).toBeInTheDocument();
  });

  it('CH-18: 성공 폴링이 비수치 값을 주면 이전 값이 지워진다(실패와 구분된다)', async () => {
    mockPost.fn.mockResolvedValueOnce({ entries: [{ value: 42, timestamp: 1 }] });

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();
    expect(screen.getByText('42.00')).toBeInTheDocument();

    // "실패 → 유지" 와 달리 "성공 + 비수치 → undefined" 로 값이 사라진다.
    mockPost.fn.mockResolvedValue({ entries: [{ value: 'abc', timestamp: 2 }] });
    await advance(5_000);
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('42.00')).toBeNull();
  });

  it('CH-18: 언마운트하면 인터벌이 정리되어 추가 폴링이 없다', async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 42, timestamp: 1 }] });

    const view = renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [F2_STORE_SOURCE],
    });
    await flush();
    expect(mockPost.fn).toHaveBeenCalledTimes(1);

    view.unmount();
    await advance(15_000);
    expect(mockPost.fn).toHaveBeenCalledTimes(1);
  });
});
