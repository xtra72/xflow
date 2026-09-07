// GaugePanel 테스트.
// chart-emitter 데이터 소스 바인딩의 live value 반영 + fallback 동작을 검증한다.
// 실제 SVG 경로 수식까지는 검증하지 않고 value 가 DOM 에 반영되는지만 확인한다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';

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

// ---------------------------------------------------------------------------
// SPEC-CHART-005 M8 — 특성화 테스트 (DDD PRESERVE).
//
// M8 은 **편집 표면만** 공용화했다 — 그리드·중심 표식·정렬 툴바·선택 구분. 드래그
// 계산은 `GaugeDragLayer` 가 계속 소유한다(viewBox 좌표와 변별 죄기 규칙이 거기 있다).
// ---------------------------------------------------------------------------
describe('GaugePanel 편집 표면 (SPEC-CHART-005 M8)', () => {
  const edit = (extra: Record<string, unknown> = {}) => (
    <GaugePanel
      panelId="p1"
      title=""
      config={{ value: 50, ...extra }}
      onConfigChange={vi.fn()}
      onTitleChange={() => {}}
      forceEdit
    />
  );

  it('AC-17: 편집 중에는 그리드와 중심 표식이 보인다', () => {
    render(edit());
    expect(screen.getByTestId('panel-edit-grid')).toBeInTheDocument();
    expect(screen.getByTestId('panel-edit-center')).toBeInTheDocument();
  });

  it('AC-17: 편집이 꺼져 있으면 그리드도 툴바도 없다', () => {
    render(<GaugePanel panelId="p1" title="" config={{ value: 50 }} />);
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    expect(screen.queryByTestId('panel-align-toolbar')).toBeNull();
  });

  it('AC-18: 정렬 툴바에 스냅 토글이 켜진 채로 나온다', () => {
    render(edit());
    expect(screen.getByTestId('panel-align-toolbar')).toBeInTheDocument();
    expect(screen.getByTestId('panel-align-reset')).toBeInTheDocument();
    // 드래그 레이어가 격자 붙임을 하게 됐으므로 스위치가 죽은 컨트롤이 아니다.
    // 값 글자만은 여전히 빠진다 — viewBox 좌표라 백분율 격자와 단위가 맞지 않는다.
    expect(screen.getByTestId('panel-snap-toggle')).toHaveAttribute('aria-pressed', 'true');
  });

  it('AC-19: 게이지 상자·값 글자·임계값 범례가 정렬 대상 표식을 갖는다', () => {
    const { container } = render(edit({ show_threshold_legend: true, thresholds: [{ value: 30, color: '#f00' }] }));
    const kinds = [...container.querySelectorAll('[data-panel-drag]')].map((e) =>
      e.getAttribute('data-panel-drag'),
    );
    expect(kinds).toContain('body');
    // 값 글자는 한때 빠져 있었다 — 오프셋 단위가 viewBox 라 백분율을 쓰는 공용 정렬과
    // 섞을 수 없었다. 도형 밖 오버레이가 되면서 같은 축을 쓰게 되어 대상에 들어온다.
    expect(kinds).toContain('value');
  });

  it('AC-19: 게이지 상자를 누르면 선택 윤곽이 진해진다', () => {
    render(edit());
    const box = screen.getByTestId('gauge-box');
    expect(box.className).toContain('outline-dashed');

    fireEvent.pointerDown(box);
    expect(screen.getByTestId('gauge-box').className).toContain('outline-2');
  });

  it('AC-20: 배치 초기화가 상자·범례 오프셋을 한 번에 지운다 — 값 글자는 건드리지 않는다', () => {
    const onConfigChange = vi.fn();
    render(
      <GaugePanel
        panelId="p1"
        title=""
        config={{ value: 50, gauge_offset_x: 10, value_offset_x: 7 }}
        onConfigChange={onConfigChange}
        onTitleChange={() => {}}
        forceEdit
      />,
    );

    fireEvent.click(screen.getByTestId('panel-align-reset'));
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as Record<string, unknown>;
    expect(patch).toEqual({
      gauge_offset_x: undefined,
      gauge_offset_y: undefined,
      threshold_legend_offset_x: undefined,
      threshold_legend_offset_y: undefined,
    });
    expect(patch).not.toHaveProperty('value_offset_x');
  });

  it('기존 드래그 레이어는 편집 중에 켜져 있다(값 글자 이동 보존)', () => {
    render(edit());
    expect(screen.getByTestId('gauge-drag-layer').className).toContain('cursor-move');
  });
});

// ---------------------------------------------------------------------------
// 세로바 게이지의 폭·높이 — 사각형 도형만 축을 나눈다.
// ---------------------------------------------------------------------------
describe('세로바 게이지는 도형 치수로 폭·높이를 잡는다', () => {
  const gauge = (extra: Record<string, unknown> = {}) => (
    <GaugePanel
      panelId="p1"
      title=""
      config={{ value: 50, ...extra }}
      onConfigChange={vi.fn()}
      onTitleChange={() => {}}
    />
  );

  /** 바(트랙) 사각형 — 가장 큰 rect 가 트랙이다. */
  const track = (c: HTMLElement) => {
    const rects = [...c.querySelectorAll('svg rect')];
    return rects.reduce((a, b) =>
      Number(b.getAttribute('height')) > Number(a.getAttribute('height')) ? b : a,
    );
  };

  it('폭을 줄이면 바가 좁아지되 글자는 눌리지 않는다 — CSS 배율이 아니라 도형 치수다', () => {
    const { container } = render(gauge({ gaugeType: 'vertical-bar', gauge_bar_width: 50 }));
    // 기본 폭 48 의 50% = 24. 가로 중심(84)은 그대로이므로 x 는 84-12 = 72.
    expect(track(container).getAttribute('width')).toBe('24');
    expect(track(container).getAttribute('x')).toBe('72');
    // 상자에는 배율이 걸리지 않는다 — 걸리면 글자까지 함께 눌린다(보고된 결함).
    expect(screen.getByTestId('gauge-box').style.transform).toBe('');
  });

  it('높이를 줄이면 아래 끝은 그대로고 위에서 줄어든다 — 읽는 기준선이 움직이지 않는다', () => {
    const { container } = render(gauge({ gaugeType: 'vertical-bar', gauge_bar_height: 50 }));
    // 기본 높이 180 의 50% = 90. 아래 끝(190)이 고정이므로 y 는 100.
    expect(track(container).getAttribute('height')).toBe('90');
    expect(track(container).getAttribute('y')).toBe('100');
  });

  it('치수를 주지 않으면 종전 도형 그대로다 — 저장된 대시보드의 모양이 바뀌지 않는다', () => {
    const { container } = render(gauge({ gaugeType: 'vertical-bar' }));
    const t = track(container);
    expect([t.getAttribute('x'), t.getAttribute('y'), t.getAttribute('width'), t.getAttribute('height')])
      .toEqual(['60', '10', '48', '180']);
  });

  it('범위 밖 저장값은 기본으로 되돌린다 — 0 이 바를 지우지 않는다', () => {
    const { container } = render(gauge({ gaugeType: 'vertical-bar', gauge_bar_width: 0 }));
    expect(track(container).getAttribute('width')).toBe('48');
  });

  it('게이지 상자 배율은 종전대로 균일하다 — 축을 나누지 않는다', () => {
    render(gauge({ gaugeType: 'vertical-bar', gauge_size: 60 }));
    expect(screen.getByTestId('gauge-box').style.transform).toContain('scale(0.6)');
  });
});

// ---------------------------------------------------------------------------
// 값 글자의 **잡히는 영역**은 글자만 해야 한다.
//
// 값을 도형 밖으로 뺄 때 오버레이를 패널 전체(`inset-0`)로 두었는데, 편집 표식과
// 포인터까지 그 상자에 걸어 두어 윤곽선이 패널만 해지고 도형 위 클릭까지 값이
// 가로챘다("값의 영역이 너무 크다"). 상자는 자리 계산에만 쓰고, 잡히는 영역과
// 윤곽선은 글자 자신이 갖는다.
// ---------------------------------------------------------------------------
describe('값 글자의 잡히는 영역은 글자만 하다', () => {
  const edit = (extra: Record<string, unknown> = {}) => (
    <GaugePanel
      panelId="p1"
      title=""
      config={{ value: 50, ...extra }}
      onConfigChange={vi.fn()}
      onTitleChange={() => {}}
      forceEdit
    />
  );

  it('패널만 한 오버레이 상자는 포인터를 받지 않는다', () => {
    render(edit());
    const box = screen.getByTestId('gauge-value-overlay');
    expect(box.className).toContain('pointer-events-none');
  });

  it('편집 표식과 윤곽선은 오버레이 상자가 아니라 글자에 붙는다', () => {
    const { container } = render(edit());
    const box = screen.getByTestId('gauge-value-overlay');
    // 상자에 붙으면 패널 전체가 값으로 잡힌다.
    expect(box.hasAttribute('data-gauge-value-text')).toBe(false);
    expect(box.hasAttribute('data-panel-drag')).toBe(false);

    const text = container.querySelector('text[data-gauge-value-text]');
    expect(text).not.toBeNull();
    expect(text!.getAttribute('data-panel-drag')).toBe('value');
    expect(text!.getAttribute('class')).toContain('outline-dashed');
  });

  it('편집이 아니면 글자도 포인터를 받지 않는다 — 대시보드에서 클릭을 삼키지 않는다', () => {
    const { container } = render(
      <GaugePanel panelId="p1" title="" config={{ value: 50 }} onTitleChange={() => {}} />,
    );
    const text = container.querySelector('text[data-gauge-value-text]')!;
    expect(text.getAttribute('class') ?? '').not.toContain('pointer-events-auto');
  });
});

// ---------------------------------------------------------------------------
// 현재값 크기도 끌어서 조절한다.
//
// 값에는 크기 손잡이가 없어 설정 슬라이더로만 배율을 바꿀 수 있었다("드래그로 조절되지
// 않음"). 손잡이는 글자 **아래 가운데**에 붙는다 — 값은 가운데 정렬이라 폭이 자릿수에
// 따라 달라져 오른쪽 모서리를 계산할 수 없기 때문이다.
// ---------------------------------------------------------------------------
describe('현재값 크기를 손잡이로 조절한다', () => {
  const stubRect = (el: Element) =>
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
      width: 400, height: 400, top: 0, left: 0, right: 400, bottom: 400, x: 0, y: 0,
      toJSON: () => ({}),
    } as DOMRect);

  const editPanel = (onConfigChange = vi.fn()) =>
    render(
      <GaugePanel
        panelId="p1"
        title=""
        config={{ value: 50, gaugeType: 'simple' }}
        onConfigChange={onConfigChange}
        onTitleChange={() => {}}
        forceEdit
      />,
    );

  it('고르기 전에는 손잡이가 없다 — 값 아래 상시로 점이 붙지 않는다', () => {
    const { container } = editPanel();
    expect(container.querySelector('[data-panel-resize="value"]')).toBeNull();
  });

  it('값을 고르면 손잡이가 나온다', async () => {
    const { container } = editPanel();
    fireEvent.pointerDown(container.querySelector('text[data-gauge-value-text]')!);
    expect(container.querySelector('[data-panel-resize="value"]')).not.toBeNull();
  });

  it('손잡이를 끌면 배율이 바뀐다 — 100px 이 배율 1 이다', async () => {
    const onConfigChange = vi.fn();
    const { container } = editPanel(onConfigChange);
    fireEvent.pointerDown(container.querySelector('text[data-gauge-value-text]')!);
    const box = container.querySelector('[data-gauge-value-box]')!;
    stubRect(box);
    stubRect(box.parentElement!);

    const handle = container.querySelector('[data-panel-resize="value"]')!;
    onConfigChange.mockClear();
    fireEvent(
      handle,
      new MouseEvent('pointerdown', { clientX: 200, clientY: 200, bubbles: true }),
    );
    fireEvent(
      document,
      new MouseEvent('pointermove', { clientX: 300, clientY: 300, bubbles: true }),
    );
    await act(async () => {
      await new Promise<void>((r) => requestAnimationFrame(() => r()));
    });

    // 배율은 px 이 아니라 0.3~3 의 수다 — 1:1 로 세면 조금만 끌어도 상한에 닿는다.
    expect(onConfigChange.mock.calls.flat()).toContainEqual({ value_scale: 2 });
  });
});
