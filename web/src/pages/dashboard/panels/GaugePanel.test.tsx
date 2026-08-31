// GaugePanel 테스트.
// chart-emitter 데이터 소스 바인딩의 live value 반영 + fallback 동작을 검증한다.
// 실제 SVG 경로 수식까지는 검증하지 않고 value 가 DOM 에 반영되는지만 확인한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import type { ChartEntry } from './charts/chartChannelTypes';

const mockChannel = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
  // 훅 호출 시 전달된 채널명 캡처 (바인딩 없을 때 호출되지 않음을 검증)
  lastCalledWith: undefined as string | undefined,
}));

vi.mock('./charts/useChartChannel', () => ({
  useChartChannel: (channelName: string | undefined) => {
    mockChannel.lastCalledWith = channelName;
    return mockChannel.current;
  },
}));

// GaugePanel(useStoreLatestValue)이 useAgents(React Query)를 호출하므로 QueryClient
// 없이 렌더 가능하도록 빈 목록으로 모킹한다. 목록이 비면 store 소스는 저장된
// 이름을 그대로 사용(fallback)해 기존 동작이 유지된다. (SPEC-WEB-006)
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// store 레거시 바인딩(useStoreLatestValue)의 POST /store/{agent}/query 를 가로챈다.
// 기존 테스트는 store 바인딩이 없어 호출되지 않으므로 영향이 없다.
const mockPost = vi.hoisted(() => ({
  fn: vi.fn(async (_url: string, _body: unknown) => ({
    entries: [] as Array<{ value: unknown; timestamp: number }>,
  })),
}));

vi.mock('@/services/api/client', () => ({
  post: (url: string, body: unknown) => mockPost.fn(url, body),
}));

import GaugePanel from './GaugePanel';

function renderPanel(config: Record<string, unknown>) {
  return render(
    <GaugePanel panelId="p1" title="테스트 게이지" config={config} />,
  );
}

describe('GaugePanel', () => {
  beforeEach(() => {
    mockChannel.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    mockChannel.lastCalledWith = undefined;
  });

  describe('chart-emitter 바인딩 없음', () => {
    it('config.value 가 그대로 표시됨 (simple 게이지)', () => {
      renderPanel({ gaugeType: 'simple', value: 42, min: 0, max: 100, unit: '%' });
      expect(screen.getByText('42.00')).toBeInTheDocument();
      expect(screen.getByText('%')).toBeInTheDocument();
    });

    it('useChartChannel 이 undefined 채널명으로 호출됨 (idle)', () => {
      renderPanel({ gaugeType: 'simple', value: 10 });
      expect(mockChannel.lastCalledWith).toBeUndefined();
    });

    it('연결 상태 아이콘 미표시', () => {
      renderPanel({ gaugeType: 'simple', value: 10 });
      expect(screen.queryByTestId('chart-status-icon')).toBeNull();
    });
  });

  describe('chart-emitter 바인딩 있음', () => {
    const baseConfig = {
      gaugeType: 'needle',
      min: 0,
      max: 100,
      unit: '°C',
      value: 0, // static fallback
      dataSources: [
        { sourceType: 'chart-emitter', channelName: 'room1_temperature' },
      ],
    };

    it('최신 entry 의 value 가 static config.value 를 덮어씀', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 20 },
        { timestamp: 2000, value: 27 },
      ];
      renderPanel(baseConfig);
      expect(screen.getByText('27.00')).toBeInTheDocument();
      expect(screen.getByText('°C')).toBeInTheDocument();
    });

    it('구독 채널명이 훅에 전달됨', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 1 }];
      renderPanel(baseConfig);
      expect(mockChannel.lastCalledWith).toBe('room1_temperature');
    });

    it('displayField dot-path 지원 (labels.temp)', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 999, labels: { temp: '42' } },
      ];
      renderPanel({
        ...baseConfig,
        dataSources: [
          {
            sourceType: 'chart-emitter',
            channelName: 'room1_temperature',
            displayField: 'labels.temp',
          },
        ],
      });
      expect(screen.getByText('42.00')).toBeInTheDocument();
    });

    it('entries 비어있으면 값 -- 로 표시 (바늘 없음)', () => {
      mockChannel.current.entries = [];
      renderPanel({ ...baseConfig, value: 55 });
      expect(screen.getByText('--')).toBeInTheDocument();
    });

    it('displayField 가 숫자로 변환 불가면 -- 로 표시', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 'not-a-number' },
      ];
      renderPanel({ ...baseConfig, value: 77 });
      expect(screen.getByText('--')).toBeInTheDocument();
    });

    it('연결 상태 아이콘 표시', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 1 }];
      renderPanel(baseConfig);
      expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
    });
  });

  describe('복수 dataSources', () => {
    it('첫 번째 chart-emitter 바인딩만 구독', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 99 }];
      renderPanel({
        gaugeType: 'simple',
        min: 0,
        max: 200,
        unit: '%',
        value: 10,
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' }, // 무시됨
          { sourceType: 'chart-emitter', channelName: 'ch-a' }, // 선택됨
          { sourceType: 'chart-emitter', channelName: 'ch-b' }, // 스킵
        ],
      });
      expect(mockChannel.lastCalledWith).toBe('ch-a');
    });

    it('chart-emitter 바인딩에 channelName 이 없으면 스킵하고 다음 탐색', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 5 }];
      renderPanel({
        gaugeType: 'simple',
        min: 0,
        max: 100,
        unit: '%',
        value: 0,
        dataSources: [
          { sourceType: 'chart-emitter' }, // channelName 없음 → 스킵
          { sourceType: 'chart-emitter', channelName: 'ch-valid' },
        ],
      });
      expect(mockChannel.lastCalledWith).toBe('ch-valid');
    });

    it('chart-emitter 바인딩 없으면 구독 안 함', () => {
      renderPanel({
        gaugeType: 'simple',
        value: 10,
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' },
          { sourceType: 'flow', flowId: 'f-1' },
        ],
      });
      expect(mockChannel.lastCalledWith).toBeUndefined();
    });
  });

  describe('showThresholdZones (옵션 — 임계 구간 표시)', () => {
    it('미설정 + thresholds 존재: 기본 ON 으로 sector 렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
        ],
      });
      // 기본값 ON 이므로 sector path 가 존재
      expect(container.querySelector('path[fill="#10b981"]')).not.toBeNull();
    });

    it('showThresholdZones=false (사용자 명시 해제): sector 미렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: false,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
        ],
      });
      const greenFill = container.querySelector('path[fill="#10b981"]');
      expect(greenFill).toBeNull();
    });

    it('showThresholdZones=true: 임계값 개수만큼 sector path 렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: true,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
          { name: '주의', from: 60, to: 80, color: '#f59e0b' },
          { name: '위험', from: 80, to: 100, color: '#ef4444' },
        ],
      });
      // 바늘은 **구간 방식**이라 값과 무관하게 세 구간이 모두 채워진다
      // (값은 니들이 가리킨다). 진행 방식인 도넛·반원·세로 바와는 다르다.
      expect(container.querySelector('path[fill="#10b981"]')).not.toBeNull();
      expect(container.querySelector('path[fill="#f59e0b"]')).not.toBeNull();
      expect(container.querySelector('path[fill="#ef4444"]')).not.toBeNull();
    });

    it('showThresholdZones=true 이지만 thresholds 가 비어있으면 sector 미렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: true,
        thresholds: [],
      });
      // 임계값 컬러 fill 이 없어야 함
      const sectors = container.querySelectorAll('path[opacity="0.25"]');
      expect(sectors.length).toBe(0);
    });
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 게이지 레거시 바인딩 특성화 (DDD PRESERVE).
//
// GaugePanel 의 값 해석은 pickChartEmitterSource / pickStoreSource /
// useStoreLatestValue / chartLiveValue / liveValue / hasBinding / parsedBase
// 7개 지점에 분산되어 있다. M5 가 이 흐름에 신규 분기를 끼워 넣으므로 현재 진리표를
// 여기서 고정한다. spec.md §2.9 [S1] / §1.2.4 / UB2-3,4.
// ---------------------------------------------------------------------------
describe('GaugePanel 레거시 바인딩 특성화 (SPEC-CHART-002 M2)', () => {
  beforeEach(() => {
    mockChannel.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    mockChannel.lastCalledWith = undefined;
    mockPost.fn.mockReset();
    mockPost.fn.mockResolvedValue({ entries: [] });
  });

  it('CH-11: resource 바인딩은 값 소스로 해석되지 않고 static config.value 가 표시된다', async () => {
    renderPanel({
      gaugeType: 'simple',
      value: 42,
      min: 0,
      max: 100,
      unit: '%',
      dataSources: [{ sourceType: 'resource', resource: 'cpu' }],
    });

    // GaugePanel 에는 resource 를 읽는 경로가 없다(pickChartEmitterSource /
    // pickStoreSource 뿐). 따라서 hasBinding=false → static config.value 가 그대로 표시된다.
    expect(screen.getByText('42.00')).toBeInTheDocument();
    expect(screen.getByText('%')).toBeInTheDocument();
    expect(screen.queryByText('--')).toBeNull();
    // 채널 구독도, store 폴링도 일어나지 않는다.
    expect(mockChannel.lastCalledWith).toBeUndefined();
    expect(mockPost.fn).not.toHaveBeenCalled();
    // chart-emitter 가 아니므로 연결 상태 아이콘도 없다.
    expect(screen.queryByTestId('chart-status-icon')).toBeNull();
  });

  it('CH-12: flow 바인딩도 값 소스로 해석되지 않고 static config.value 가 표시된다', () => {
    renderPanel({
      gaugeType: 'simple',
      value: 63,
      min: 0,
      max: 100,
      unit: '%',
      dataSources: [{ sourceType: 'flow', flowId: 'f1', dataField: 'x' }],
    });

    expect(screen.getByText('63.00')).toBeInTheDocument();
    expect(screen.queryByText('--')).toBeNull();
    expect(mockChannel.lastCalledWith).toBeUndefined();
    expect(mockPost.fn).not.toHaveBeenCalled();
  });

  it('CH-13: chart-emitter 바인딩은 최신 entry 를 displayField(dot-path) 로 해석한다', () => {
    mockChannel.current.entries = [
      { timestamp: 1000, value: 1, labels: { temp: '17' } },
      { timestamp: 2000, value: 999, labels: { temp: '42' } },
    ];
    const dot = renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      unit: '°C',
      dataSources: [
        { sourceType: 'chart-emitter', channelName: 'ch1', displayField: 'labels.temp' },
      ],
    });
    // 마지막 entry 의 labels.temp = '42' (문자열도 toNumber 로 변환된다).
    expect(screen.getByText('42.00')).toBeInTheDocument();
    expect(mockChannel.lastCalledWith).toBe('ch1');
    dot.unmount();

    // displayField 가 빈 문자열이면 'value' 로 폴백한다.
    mockChannel.current.entries = [{ timestamp: 1000, value: 37 }];
    const empty = renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1', displayField: '' }],
    });
    expect(screen.getByText('37.00')).toBeInTheDocument();
    empty.unmount();

    // displayField 미지정도 'value' 로 폴백한다.
    mockChannel.current.entries = [{ timestamp: 1000, value: 38 }];
    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1' }],
    });
    expect(screen.getByText('38.00')).toBeInTheDocument();
  });

  it('CH-15: 우선순위는 chart-emitter > store(latest) > static config.value 다', async () => {
    const bothSources = [
      { sourceType: 'chart-emitter', channelName: 'ch1' },
      {
        sourceType: 'store',
        storeAgentId: 'a1',
        storeAgent: 'store-a',
        storeKey: 'k1',
        storeNamespace: 'default',
      },
    ];

    // (a) chart-emitter 가 값을 내면 store 값이 있어도 chart-emitter 가 이긴다.
    mockPost.fn.mockResolvedValue({ entries: [{ value: 88, timestamp: 1 }] });
    mockChannel.current.entries = [{ timestamp: 1, value: 27 }];
    const win = renderPanel({
      gaugeType: 'simple',
      value: 55,
      min: 0,
      max: 100,
      dataSources: bothSources,
    });
    await waitFor(() => expect(mockPost.fn).toHaveBeenCalled());
    expect(screen.getByText('27.00')).toBeInTheDocument();
    expect(screen.queryByText('88.00')).toBeNull();
    win.unmount();

    // (b) chart-emitter 가 바인딩되어 있어도 값을 못 내면(entries 0개) store 로 내려간다.
    //     즉 우선순위는 "바인딩 존재" 가 아니라 "값 존재" 기준이다.
    mockPost.fn.mockResolvedValue({ entries: [{ value: 88, timestamp: 1 }] });
    mockChannel.current.entries = [];
    const fall = renderPanel({
      gaugeType: 'simple',
      value: 55,
      min: 0,
      max: 100,
      dataSources: bothSources,
    });
    await waitFor(() => expect(screen.getByText('88.00')).toBeInTheDocument());
    fall.unmount();

    // (c) 둘 다 바인딩되었지만 어느 쪽도 값을 못 내면 static 으로 폴백하지 않고 -- 다.
    mockPost.fn.mockResolvedValue({ entries: [] });
    mockChannel.current.entries = [];
    const none = renderPanel({
      gaugeType: 'simple',
      value: 55,
      min: 0,
      max: 100,
      dataSources: bothSources,
    });
    await waitFor(() => expect(mockPost.fn).toHaveBeenCalled());
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('55.00')).toBeNull();
    none.unmount();

    // (d) 바인딩이 하나도 없을 때만 static config.value 가 쓰인다.
    renderPanel({ gaugeType: 'simple', value: 55, min: 0, max: 100 });
    expect(screen.getByText('55.00')).toBeInTheDocument();
  });

  it('CH-16: 복수 dataSources 에서 타입별 첫 유효 항목만 사용한다', async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 12, timestamp: 1 }] });
    mockChannel.current.entries = [{ timestamp: 1, value: 5 }];

    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [
        // 무시: 해석 경로 없음
        { sourceType: 'resource', resource: 'cpu' },
        // 무시: channelName 없음 → 유효하지 않음
        { sourceType: 'chart-emitter' },
        // 선택됨 (첫 유효 chart-emitter)
        { sourceType: 'chart-emitter', channelName: 'ch-a' },
        // 스킵
        { sourceType: 'chart-emitter', channelName: 'ch-b' },
        // 무시: storeKey 없음 → 유효하지 않음
        { sourceType: 'store', storeAgent: 'store-x' },
        // 선택됨 (첫 유효 store)
        { sourceType: 'store', storeAgent: 'store-b', storeKey: 'k2', storeNamespace: 'ns2' },
        // 스킵
        { sourceType: 'store', storeAgent: 'store-c', storeKey: 'k3' },
      ],
    });

    expect(mockChannel.lastCalledWith).toBe('ch-a');
    await waitFor(() => expect(mockPost.fn).toHaveBeenCalled());
    // store 도 첫 유효 항목(store-b / k2 / ns2) 하나만 폴링한다.
    expect(mockPost.fn).toHaveBeenCalledWith('/store/store-b/query', {
      key: 'k2',
      mode: 'latest',
      namespace: 'ns2',
    });
    const urls = mockPost.fn.mock.calls.map((c) => c[0]);
    expect(urls.every((u) => u === '/store/store-b/query')).toBe(true);
  });

  it('CH-17: 바인딩이 있는데 값이 없거나 비수치면 -- 를 표시한다(static 폴백 없음)', async () => {
    // (a) chart-emitter 바인딩 + entries 0개
    mockChannel.current.entries = [];
    const empty = renderPanel({
      gaugeType: 'simple',
      value: 77,
      min: 0,
      max: 100,
      unit: '°C',
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1' }],
    });
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('77.00')).toBeNull();
    // 값이 없으면 단위도 렌더되지 않는다.
    expect(screen.queryByText('°C')).toBeNull();
    empty.unmount();

    // (b) chart-emitter 바인딩 + 숫자로 변환 불가한 값
    mockChannel.current.entries = [{ timestamp: 1, value: 'not-a-number' }];
    const nan = renderPanel({
      gaugeType: 'simple',
      value: 77,
      min: 0,
      max: 100,
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1' }],
    });
    expect(screen.getByText('--')).toBeInTheDocument();
    nan.unmount();

    // (c) store 바인딩 + 응답 값이 비수치
    mockPost.fn.mockResolvedValue({ entries: [{ value: 'abc', timestamp: 1 }] });
    renderPanel({
      gaugeType: 'simple',
      value: 77,
      min: 0,
      max: 100,
      dataSources: [
        { sourceType: 'store', storeAgent: 'store-a', storeKey: 'k1' },
      ],
    });
    await waitFor(() => expect(mockPost.fn).toHaveBeenCalled());
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('77.00')).toBeNull();
  });

  it('CH-17: 연결 상태 아이콘은 chart-emitter 바인딩일 때만 노출된다(store 는 미노출)', async () => {
    mockPost.fn.mockResolvedValue({ entries: [{ value: 30, timestamp: 1 }] });
    renderPanel({
      gaugeType: 'simple',
      value: 0,
      min: 0,
      max: 100,
      dataSources: [
        { sourceType: 'store', storeAgent: 'store-a', storeKey: 'k1' },
      ],
    });
    await waitFor(() => expect(screen.getByText('30.00')).toBeInTheDocument());
    expect(screen.queryByTestId('chart-status-icon')).toBeNull();
  });
});
