// GaugePanel 테스트.
// chart-emitter 데이터 소스 바인딩의 live value 반영 + fallback 동작을 검증한다.
// 실제 SVG 경로 수식까지는 검증하지 않고 value 가 DOM 에 반영되는지만 확인한다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

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
import { useUIStore } from '@/stores/uiStore';

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

  // chart-emitter(채널) 바인딩은 사라졌다 — 같은 데이터는 Store 소스로 받는다.
  // 남은 것은 "바인딩이 없을 때의 static 값" 뿐이다.
  describe('바인딩 없음 — static config.value', () => {
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

  describe('게이지 크기·위치', () => {
    const box = () => screen.getByTestId('gauge-box');

    it('기본은 가득 채우고 transform 을 남기지 않는다 — 불필요한 레이어를 만들지 않는다', () => {
      renderPanel({ gaugeType: 'simple', value: 50, min: 0, max: 100 });
      expect(box().getAttribute('style') ?? '').not.toContain('transform');
    });

    it('크기를 줄이면 scale 로 실린다', () => {
      renderPanel({ gaugeType: 'simple', value: 50, min: 0, max: 100, gauge_size: 60 });
      expect(box().getAttribute('style') ?? '').toContain('scale(0.6)');
    });

    it('오프셋은 백분율로 실린다 — 패널 크기가 바뀌어도 상대 위치가 유지된다', () => {
      renderPanel({
        gaugeType: 'simple',
        value: 50,
        min: 0,
        max: 100,
        gauge_offset_x: 10,
        gauge_offset_y: -5,
      });
      expect(box().getAttribute('style') ?? '').toContain('translate(10%, -5%)');
    });

    it('범위를 벗어난 크기·오프셋은 죈다 — 손으로 고친 config 가 게이지를 날리지 않게', () => {
      renderPanel({
        gaugeType: 'simple',
        value: 50,
        min: 0,
        max: 100,
        gauge_size: 500,
        gauge_offset_x: 999,
      });
      const style = box().getAttribute('style') ?? '';
      // 범위 밖 크기는 무시하고 가득(=scale 1), 오프셋은 상한으로 죈다.
      expect(style).toContain('scale(1)');
      expect(style).toContain('translate(40%, 0%)');
    });

    it('드래그 레이어가 잡을 수 있도록 표식을 남긴다', () => {
      renderPanel({ gaugeType: 'simple', value: 50, min: 0, max: 100 });
      expect(box().hasAttribute('data-gauge-body')).toBe(true);
    });
  });

  describe('임계값 범례', () => {
    const base = { gaugeType: 'simple', value: 50, min: 0, max: 100 } as const;

    it('기본은 그리지 않는다 — 켠 사람만 본다', () => {
      renderPanel({ ...base });
      expect(screen.queryByTestId('gauge-threshold-legend')).toBeNull();
    });

    it('켜면 구간마다 한 줄씩 — 이름이 없으면 범위를 적는다', () => {
      renderPanel({ ...base, show_threshold_legend: true });
      const items = screen.getAllByTestId('gauge-threshold-legend-item');
      // 기본 3구간(0~60 / 60~80 / 80~100), 이름은 비어 있다.
      expect(items.map((e) => e.textContent)).toEqual(['0~60', '60~80', '80~100']);
    });

    it('이름을 붙이면 이름이 이긴다', () => {
      renderPanel({
        ...base,
        show_threshold_legend: true,
        thresholds: [{ name: '정상', color: '#10b981', from: 0, to: 100 }],
      });
      expect(screen.getByTestId('gauge-threshold-legend-item').textContent).toBe('정상');
    });

    it('기본은 가로형 — 항목이 한 줄로 흐른다', () => {
      renderPanel({ ...base, show_threshold_legend: true });
      const el = screen.getByTestId('gauge-threshold-legend');
      expect(el.getAttribute('data-orientation')).toBe('horizontal');
      expect(el.className).toContain('flex-wrap');
    });

    it('세로형이면 한 칸씩 쌓는다', () => {
      renderPanel({
        ...base,
        show_threshold_legend: true,
        threshold_legend_orientation: 'vertical',
      });
      const el = screen.getByTestId('gauge-threshold-legend');
      expect(el.getAttribute('data-orientation')).toBe('vertical');
      expect(el.className).toContain('flex-col');
    });

    it('게이지 위에 겹쳐 뜬다 — 자리를 나눠 가지지 않는다', () => {
      renderPanel({ ...base, show_threshold_legend: true });
      const cls = screen.getByTestId('gauge-threshold-legend').className;
      expect(cls).toContain('absolute');
      expect(cls).toContain('z-10');
      // 겹친 자리에서 글자가 읽히도록 옅은 판을 깐다.
      expect(cls).toContain('bg-(--color-bg-surface)/80');
    });

    it('오프셋은 담는 상자 대비 % — 패널 크기가 달라도 같은 자리를 가리킨다', () => {
      renderPanel({
        ...base,
        show_threshold_legend: true,
        threshold_legend_offset_x: 12,
        threshold_legend_offset_y: -8,
      });
      const style = screen.getByTestId('gauge-threshold-legend').getAttribute('style') ?? '';
      // 픽셀이던 시절에는 미리보기(≈1870px)와 패널(≈1500px 이하)에서 자리가 갈렸다.
      expect(style).toContain('left: calc(62%)');
      expect(style).toContain('bottom: 8%');
      expect(style).toContain('translateX(-50%)');
    });

    it('드래그 레이어가 잡을 수 있도록 표식을 남긴다', () => {
      renderPanel({ ...base, show_threshold_legend: true });
      expect(
        screen.getByTestId('gauge-threshold-legend').hasAttribute('data-gauge-threshold-legend'),
      ).toBe(true);
    });

    it('위치와 글자 스타일이 실린다', () => {
      renderPanel({
        ...base,
        show_threshold_legend: true,
        threshold_legend_position: 'top',
        threshold_legend_font_size: 15,
        threshold_legend_font_color: '#ff0000',
        threshold_legend_font_family: 'mono',
      });
      const el = screen.getByTestId('gauge-threshold-legend');
      expect(el.getAttribute('data-position')).toBe('top');
      const style = el.getAttribute('style') ?? '';
      expect(style).toContain('font-size: 15px');
      expect(style).toContain('color: rgb(255, 0, 0)');
      expect(style).toContain('monospace');
    });
  });

  describe('시리즈 이름(타일 캡션)', () => {
    it('기본은 게이지 아래', () => {
      renderPanel({ gaugeType: 'simple', value: 50, min: 0, max: 100 });
      // 단일 게이지 경로에는 타일이 없다 — 위치 규칙은 타일 경로에서 확인한다.
      expect(screen.queryByTestId('gauge-tile-body')).toBeNull();
    });
  });

  describe('패널 편집 모드 (히트맵과 같은 규칙)', () => {
    const base = { gaugeType: 'simple', value: 50, min: 0, max: 100 } as const;

    afterEach(() => {
      useUIStore.getState().setDashboardEditMode(false);
    });

    it('편집모드가 아니면 토글을 표시하지 않는다 — gear/삭제 버튼과 같은 게이팅', () => {
      useUIStore.getState().setDashboardEditMode(false);
      render(
        <GaugePanel panelId="p1" title="t" config={{ ...base }} onConfigChange={vi.fn()} />,
      );
      expect(screen.queryByTestId('gauge-edit-toggle')).toBeNull();
    });

    it('config 를 쓸 콜백이 없으면 토글을 표시하지 않는다 — 끌어도 저장할 곳이 없다', () => {
      useUIStore.getState().setDashboardEditMode(true);
      render(<GaugePanel panelId="p1" title="t" config={{ ...base }} />);
      expect(screen.queryByTestId('gauge-edit-toggle')).toBeNull();
    });

    it('편집모드 + 콜백이면 토글이 뜨고, 누르면 눌린 상태가 된다', () => {
      useUIStore.getState().setDashboardEditMode(true);
      render(
        <GaugePanel panelId="p1" title="t" config={{ ...base }} onConfigChange={vi.fn()} />,
      );
      const btn = screen.getByTestId('gauge-edit-toggle');
      expect(btn.getAttribute('aria-pressed')).toBe('false');
      fireEvent.click(btn);
      expect(btn.getAttribute('aria-pressed')).toBe('true');
    });

    it('forceEdit(설정 미리보기)는 토글을 감춘다 — 끌 수 있다는 것이 맥락으로 드러난다', () => {
      useUIStore.getState().setDashboardEditMode(true);
      render(
        <GaugePanel
          panelId="p1"
          title="t"
          config={{ ...base }}
          onConfigChange={vi.fn()}
          forceEdit
        />,
      );
      expect(screen.queryByTestId('gauge-edit-toggle')).toBeNull();
    });
  });
});
