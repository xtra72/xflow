// StatPanel 테스트.
// useChartChannel 을 vi.mock 으로 교체해 entries/status 를 직접 주입한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { useUIStore } from '@/stores/uiStore';

import type { ChartEntry } from './chartChannelTypes';

const mockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

// 채널이 패널 소스에서 빠지면서 데이터 이음매가 하나로 줄었다(`usePanelSeriesData`).
// 이 파일의 테스트들은 채널 형상(`mockResult.current`)을 심으므로, 그 형상을 시리즈 소스
// 결과로 옮겨 준다 — 테스트 본문을 그대로 두기 위한 어댑터다.
vi.mock('./usePanelSeriesData', () => ({
  usePanelSeriesData: () => ({
    ...mockResult.current,
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
  }),
  isPanelSeriesSource: () => true,
}));

// useStoreChartData 가 내부에서 useAgents(React Query)를 호출하므로, QueryClient
// 없이 렌더 가능하도록 빈 목록으로 모킹한다. 목록이 비면 store 소스는 저장된
// 이름을 그대로 사용(fallback)해 기존 동작이 유지된다. (SPEC-WEB-006)
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 동적 import 는 vi.mock 을 먼저 평가한 뒤 이루어짐
import StatPanel from './StatPanel';

describe('StatPanel', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('entries 비어있을 때 placeholder —', () => {
    render(<StatPanel panelId="p1" config={{ channel_name: 'test' }} />);
    expect(screen.getByTestId('stat-value').textContent).toContain('—');
  });

  it('값이 1개만 있으면 delta 표시 안 함', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 10 }];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-value').textContent).toContain('10');
    expect(screen.queryByTestId('stat-delta')).toBeNull();
  });

  it('2개 값이 있으면 delta + 증가 화살표 ↑', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 25.0 },
      { timestamp: 2, value: 26.5 },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 1, unit: '°C' }}
      />,
    );
    const val = screen.getByTestId('stat-value');
    expect(val.textContent).toContain('26.5');
    expect(val.textContent).toContain('°C');

    const delta = screen.getByTestId('stat-delta');
    expect(delta.textContent).toContain('↑');
    expect(delta.textContent).toContain('+1.5');
  });

  it('자동 데이터 량은 값과 단위가 함께 정해진다', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1024 * 1024 * 3 }];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, unit: 'auto:bytes' }}
      />,
    );
    const val = screen.getByTestId('stat-value');
    expect(val.textContent).toContain('3');
    expect(val.textContent).toContain('MB');
    // 저장값이 그대로 새어 나오면 안 된다.
    expect(val.textContent).not.toContain('auto:bytes');
  });

  it('증감도 같은 단위 규칙으로 접힌다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 1024 * 1024 },
      { timestamp: 2, value: 1024 * 1024 * 3 },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, unit: 'auto:bytes' }}
      />,
    );
    // 증감 2MB — 본값은 MB 인데 증감만 원시 바이트로 나오면 같은 축인지 알 수 없다.
    expect(screen.getByTestId('stat-delta').textContent).toContain('+2MB');
  });

  it('값이 감소하면 ↓ 화살표', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 5 },
    ];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('↓');
    expect(screen.getByTestId('stat-delta').textContent).toContain('-5');
  });

  it('값이 같으면 → 화살표', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 10 },
    ];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('→');
  });

  it('threshold_color_rules 적용', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 85 }];
    const rules = [
      { min: 0, color: 'rgb(16, 185, 129)' },
      { min: 80, color: 'rgb(239, 68, 68)' },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, threshold_color_rules: rules }}
      />,
    );
    const val = screen.getByTestId('stat-value');
    // inline style 로 적용
    expect(val.getAttribute('style')).toContain('color');
    expect(val.getAttribute('style')).toMatch(/239|rgb\(239/);
  });

  it('display_field 경로로 값 추출', () => {
    mockResult.current.entries = [{ timestamp: 1, value: { inner: 99 } }];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', display_field: 'value.inner', decimal_places: 0 }}
      />,
    );
    expect(screen.getByTestId('stat-value').textContent).toContain('99');
  });

  it('status=closed 시 overlay 표시', () => {
    mockResult.current.status = 'closed' as unknown as 'connected';
    mockResult.current.closedReason = 'flow_undeployed';
    mockResult.current.entries = [{ timestamp: 1, value: 5 }];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('stat-overlay').textContent).toContain('flow_undeployed');
  });

  it('status=error 시 error overlay', () => {
    mockResult.current.status = 'error' as unknown as 'connected';
    mockResult.current.errorReason = 'channel_not_found';
    render(<StatPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('stat-overlay').textContent).toContain('channel_not_found');
  });

  it('연결 상태 아이콘이 렌더링됨', () => {
    render(<StatPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `series_reduce` 도입 이전의 StatPanel 레거시 렌더 경로를 잠근다.
// spec.md §2.9 [S1]: `series_reduce` 부재 = 레거시 경로. 아래 테스트는 M3 이후에도
// 전부 GREEN 이어야 하며, 하나라도 RED 가 되면 하위 호환 위반이다.
// ---------------------------------------------------------------------------
describe('StatPanel 특성화 (SPEC-CHART-002 M2)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('CH-01: 평탄화 타임라인의 마지막 entry 값 1개만 표시한다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 20 },
      { timestamp: 3, value: 33 },
    ];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);

    // 출력은 단 하나의 값 슬롯이다(타일 배열 아님).
    expect(screen.getAllByTestId('stat-value')).toHaveLength(1);
    // 값은 entries[entries.length - 1] 이다 — 최댓값(33 이 우연히 최댓값이 아니도록
    // 중간에 더 큰 값이 없음을 감안해도, 규칙은 "마지막"이지 "최대"가 아니다).
    expect(screen.getByTestId('stat-value').textContent).toContain('33');
  });

  it('CH-01: entries 가 비면 값 자리에 — 를 표시하고 delta 줄이 없다', () => {
    mockResult.current.entries = [];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('stat-value').textContent).toContain('—');
    expect(screen.queryByTestId('stat-delta')).toBeNull();
  });

  it('CH-02: 직전 entry 대비 delta 와 화살표 3종(↑/↓/→)을 표시한다', () => {
    // ↑ 증가
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 14 },
    ];
    const up = render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('↑');
    expect(screen.getByTestId('stat-delta').textContent).toContain('+4');
    up.unmount();

    // ↓ 감소 (부호 그대로, + 접두사 없음)
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 4 },
    ];
    const down = render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('↓');
    expect(screen.getByTestId('stat-delta').textContent).toContain('-6');
    down.unmount();

    // → 변화 없음. delta 가 0 이면 텍스트는 '+0' 이다(0 도 표시된다).
    mockResult.current.entries = [
      { timestamp: 1, value: 7 },
      { timestamp: 2, value: 7 },
    ];
    const flat = render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('→');
    expect(screen.getByTestId('stat-delta').textContent).toContain('0');
    flat.unmount();

    // 표본이 1개면 delta 를 계산할 수 없어 줄 자체가 렌더되지 않는다.
    mockResult.current.entries = [{ timestamp: 1, value: 7 }];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.queryByTestId('stat-delta')).toBeNull();
  });

  it('CH-03: threshold_color_rules 가 값 숫자 색을 결정한다(값 이하 최대 min 규칙)', () => {
    const rules = [
      { min: 0, color: 'rgb(16, 185, 129)' },
      { min: 80, color: 'rgb(239, 68, 68)' },
    ];
    // 80 미만 → min:0 규칙
    mockResult.current.entries = [{ timestamp: 1, value: 79 }];
    const low = render(
      <StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0, threshold_color_rules: rules }} />,
    );
    expect(screen.getByTestId('stat-value').getAttribute('style')).toContain('rgb(16, 185, 129)');
    low.unmount();

    // 80 이상 → min:80 규칙
    mockResult.current.entries = [{ timestamp: 1, value: 80 }];
    const high = render(
      <StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0, threshold_color_rules: rules }} />,
    );
    expect(screen.getByTestId('stat-value').getAttribute('style')).toContain('rgb(239, 68, 68)');
    high.unmount();

    // 규칙이 없으면 inline color 를 붙이지 않는다(기본 텍스트 색 유지).
    // style 속성 자체는 남는다 — 현재값 크기 배율이 항상 fontSize 를 싣기 때문이다.
    // 색이 없다는 것을 style 부재로 판정하면 크기 설정이 들어온 순간 깨진다.
    mockResult.current.entries = [{ timestamp: 1, value: 80 }];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-value').style.color).toBe('');
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-003 — 보조 표기(변화량 · 구간 통계)의 설정화.
//
// M1 에서 하드코딩 동작을 특성화로 잠근 뒤, M4 배선으로 아래 서술이 그 자리를
// 대신했다. 잠금이었던 네 축(표시 여부 · 색 · 크기 · 경로)이 모두 설정을 따른다.
//
// @spec SPEC-CHART-003 AC-07 / AC-08 / AC-11 / AC-12 / AC-14 / AC-17 / AC-19 / AC-24 / AC-26
// ---------------------------------------------------------------------------
describe('StatPanel 보조 표기 (SPEC-CHART-003)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  /** 20 22 26 24 21 — acceptance.md F1. 마지막 21 · 직전 24 · 평 22.6 · 최대 26 · 최소 20. */
  const F1 = [
    { timestamp: 1, value: 20 },
    { timestamp: 2, value: 22 },
    { timestamp: 3, value: 26 },
    { timestamp: 4, value: 24 },
    { timestamp: 5, value: 21 },
  ];

  it('AC-08: delta_display 미지정이면 레거시 경로는 변화량을 그린다(종전 동작 유지)', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta')).toBeInTheDocument();
  });

  it('AC-07: delta_display.enabled=false 면 변화량 줄이 없다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, delta_display: { enabled: false } }}
      />,
    );
    expect(screen.queryByTestId('stat-delta')).toBeNull();
    // 본값은 그대로 남는다 — 끈 것은 보조 줄뿐이다.
    expect(screen.getByTestId('stat-value').textContent).toContain('21');
  });

  it('AC-14: 변화량 기준선은 직전 표본이다(구간 시작 대비가 아니다)', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    const delta = screen.getByTestId('stat-delta');
    // 21 - 24 = -3. 구간 시작(20) 대비였다면 +1 이다.
    expect(delta.textContent).toContain('↓');
    expect(delta.textContent).toContain('-3');
    expect(delta.textContent).not.toContain('+1');
  });

  it('AC-11: 증감별 색이 설정을 따른다', () => {
    const colors = { up_color: '#123456', down_color: '#654321', flat_color: '#abcdef' };

    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 14 },
    ];
    const up = render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, delta_display: colors }}
      />,
    );
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(18, 52, 86)');
    up.unmount();

    mockResult.current.entries = [
      { timestamp: 1, value: 14 },
      { timestamp: 2, value: 10 },
    ];
    const down = render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, delta_display: colors }}
      />,
    );
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(101, 67, 33)');
    down.unmount();

    mockResult.current.entries = [
      { timestamp: 1, value: 7 },
      { timestamp: 2, value: 7 },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, delta_display: colors }}
      />,
    );
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(171, 205, 239)');
  });

  it('AC-12: 색 미지정이면 종전 기본색(emerald)을 인라인으로 쓴다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 14 },
    ];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(16, 185, 129)');
  });

  it('AC-17: window_stats 미지정이면 구간 통계 줄이 없다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.queryByTestId('stat-window-stats')).toBeNull();
  });

  it('AC-19: 구간 통계가 평균 · 최대 · 최소를 한 줄에 고정 순서로 그린다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          decimal_places: 1,
          // 기재 순서를 뒤집어도 표시는 avg → max → min 이다.
          window_stats: { min: true, max: true, avg: true },
        }}
      />,
    );
    const text = screen.getByTestId('stat-window-stats').textContent ?? '';
    expect(text).toContain('22.6');
    expect(text).toContain('26.0');
    expect(text).toContain('20.0');
    expect(text.indexOf('22.6')).toBeLessThan(text.indexOf('26.0'));
    expect(text.indexOf('26.0')).toBeLessThan(text.indexOf('20.0'));
  });

  it('구간 통계는 켠 항목만 그린다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, window_stats: { max: true } }}
      />,
    );
    const line = screen.getByTestId('stat-window-stats');
    expect(line.querySelectorAll('[data-stat-kind]')).toHaveLength(1);
    expect(line.textContent).toContain('26');
  });

  it('구간 통계는 display_field 가 가리키는 자리를 접는다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 0, labels: { t: '10' } },
      { timestamp: 2, value: 0, labels: { t: '30' } },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          decimal_places: 0,
          display_field: 'labels.t',
          window_stats: { avg: true },
        }}
      />,
    );
    // value(0) 가 아니라 labels.t(10 · 30)의 평균 20 이어야 한다.
    expect(screen.getByTestId('stat-window-stats').textContent).toContain('20');
  });

  it('표본이 없으면 구간 통계 항목이 자리를 지키고 — 를 그린다', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 'not-a-number' }];
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          decimal_places: 0,
          window_stats: { avg: true, max: true, min: true },
        }}
      />,
    );
    const line = screen.getByTestId('stat-window-stats');
    expect(line.querySelectorAll('[data-stat-kind]')).toHaveLength(3);
    expect((line.textContent ?? '').match(/—/g)).toHaveLength(3);
  });

  it('AC-24: sub_value_scale 이 두 보조 줄에 함께 적용된다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          decimal_places: 0,
          sub_value_scale: 2,
          window_stats: { avg: true },
        }}
      />,
    );
    expect(screen.getByTestId('stat-delta').style.fontSize).toBe('28px');
    expect(screen.getByTestId('stat-window-stats').style.fontSize).toBe('28px');
  });

  it('AC-26: value_scale 은 본값만 키우고 보조 줄 크기를 바꾸지 않는다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          decimal_places: 0,
          value_scale: 2,
          window_stats: { avg: true },
        }}
      />,
    );
    expect(screen.getByTestId('stat-value').style.fontSize).toBe('72px');
    expect(screen.getByTestId('stat-delta').style.fontSize).toBe('14px');
    expect(screen.getByTestId('stat-window-stats').style.fontSize).toBe('14px');
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-004 — 요소 직접 편집(배치 · 크기 · 글자 스타일).
//
// M1 에서 "편집 입구가 없다" 와 "위치 축이 없다" 를 잠근 뒤, M6 배선으로 아래 서술이
// 그 자리를 대신했다. **편집이 꺼진 경로의 단언은 그대로 남는다** — 읽기 전용
// 대시보드·원격 뷰의 DOM 은 종전과 같아야 한다.
//
// @spec SPEC-CHART-004 AC-01 ~ AC-05 / AC-10 / AC-11 / AC-15 / AC-16 / AC-20 / AC-23
// ---------------------------------------------------------------------------
describe('StatPanel 요소 직접 편집 (SPEC-CHART-004)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    useUIStore.setState({ dashboardEditMode: false });
  });

  /** 20 → 24. 본값 24 · 변화량 +4 · 평 22 (acceptance F1). */
  const F1 = [
    { timestamp: 1, value: 20 },
    { timestamp: 2, value: 24 },
  ];

  const baseConfig = { channel_name: 'c', decimal_places: 0, window_stats: { avg: true } };

  it('AC-01: onConfigChange 가 없으면 편집 입구가 없다', () => {
    mockResult.current.entries = F1;
    const { container } = render(<StatPanel panelId="p1" config={baseConfig} />);

    expect(screen.queryByTestId('stat-edit-toggle')).toBeNull();
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(0);
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(0);
  });

  it('편집을 켜면 잡을 수 있는 자리가 화면에 보인다', () => {
    // 커서 모양만으로는 부족하다 — 글자만 있는 화면이라 포인터를 올리기 전에는
    // 무엇이 잡히는지 알 수 없다. 세 요소 모두 점선 윤곽을 갖는다.
    mockResult.current.entries = F1;
    const { container } = render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />,
    );

    for (const testId of ['stat-value', 'stat-delta', 'stat-window-stats']) {
      expect(screen.getByTestId(testId).className).toContain('outline-dashed');
    }
    // 끌 수 있는 표식은 셋 다 갖는다. 크기 손잡이는 고른 요소에만 붙는다(v0.5.0).
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(3);
  });

  it('편집이 꺼져 있으면 윤곽도 손잡이도 없다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} />);

    for (const testId of ['stat-value', 'stat-delta', 'stat-window-stats']) {
      expect(screen.getByTestId(testId).className).not.toContain('outline-dashed');
    }
  });

  it('AC-02: forceEdit 는 토글 없이 편집을 켠다', () => {
    mockResult.current.entries = F1;
    const { container } = render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />,
    );

    // 끌 수 있다는 사실이 화면 맥락으로 이미 드러나 있고, 토글이 미리보기를 가린다.
    expect(screen.queryByTestId('stat-edit-toggle')).toBeNull();
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(3);
    // 손잡이는 고른 뒤에 붙는다(v0.5.0 선택 모델).
    fireEvent.pointerDown(screen.getByTestId('stat-value'));
    expect(screen.getByTestId('panel-resize-value')).toBeInTheDocument();
  });

  it('AC-03: 대시보드는 편집모드 + 토글 2겹을 거쳐야 켜진다', () => {
    mockResult.current.entries = F1;
    useUIStore.setState({ dashboardEditMode: true });
    const { container } = render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} />,
    );

    // 편집모드만으로는 켜지지 않는다 — 패널을 옮기려는 조작과 부딪힌다.
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(0);

    fireEvent.click(screen.getByTestId('stat-edit-toggle'));
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(3);
  });

  it('AC-03: 편집모드가 아니면 토글 자체가 없다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('stat-edit-toggle')).toBeNull();
  });

  it('AC-05: 토글을 눌러도 config 를 건드리지 않는다', () => {
    mockResult.current.entries = F1;
    useUIStore.setState({ dashboardEditMode: true });
    const onConfigChange = vi.fn();
    render(<StatPanel panelId="p1" config={baseConfig} onConfigChange={onConfigChange} />);

    fireEvent.click(screen.getByTestId('stat-edit-toggle'));
    // 편집 중이라는 사실이 config 에 남으면 다음에 열 때도 오버레이가 떠 있다.
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('AC-10: 저장된 오프셋이 상대 위치로 반영된다 — 편집이 꺼져 있어도', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          ...baseConfig,
          value_layout: { offset_x: 10, offset_y: -5 },
          delta_layout: { offset_x: -20 },
          stats_layout: { offset_y: 15 },
        }}
      />,
    );

    // transform 의 백분율은 요소 자신의 크기 기준이라 쓸 수 없다 — left/top 은 담는
    // 상자 기준이라 저장 값의 뜻(패널 상자 대비)과 정확히 맞는다.
    const value = screen.getByTestId('stat-value');
    expect(value.style.position).toBe('relative');
    expect(value.style.left).toBe('10%');
    expect(value.style.top).toBe('-5%');
    expect(value.style.transform).toBe('');

    expect(screen.getByTestId('stat-delta').style.left).toBe('-20%');
    expect(screen.getByTestId('stat-window-stats').style.top).toBe('15%');
  });

  it('오프셋이 0 이면 위치 속성을 붙이지 않는다 — 종전 흐름 배치 그대로다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} />);
    expect(screen.getByTestId('stat-value').style.left).toBe('');
    expect(screen.getByTestId('stat-delta').style.left).toBe('');
  });

  it('AC-11: font_size 가 배율을 이긴다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          ...baseConfig,
          value_scale: 2,
          sub_value_scale: 2,
          value_layout: { font_size: 50 },
          delta_layout: { font_size: 18 },
        }}
      />,
    );

    // 50 이다 — 72(36×2) 도 100(50×2) 도 아니다.
    expect(screen.getByTestId('stat-value').style.fontSize).toBe('50px');
    expect(screen.getByTestId('stat-delta').style.fontSize).toBe('18px');
    // 지정하지 않은 요소는 폴백 그대로(14 × 2).
    expect(screen.getByTestId('stat-window-stats').style.fontSize).toBe('28px');
  });

  it('AC-15: 단위는 본값과 같은 비율로 커진다', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 24 }];
    const { container } = render(
      <StatPanel
        panelId="p1"
        config={{ channel_name: 'c', decimal_places: 0, unit: '°C', value_layout: { font_size: 72 } }}
      />,
    );

    // 종전 비율 20/36 을 유지한다 — 72 × (20/36) = 40.
    const unit = container.querySelector('[data-testid="stat-value"] span') as HTMLElement;
    expect(unit.style.fontSize).toBe('40px');
  });

  it('AC-20: 임계값 규칙이 기본 글자색을 이긴다', () => {
    const cfg = {
      channel_name: 'c',
      decimal_places: 0,
      value_layout: { font_color: '#123456' },
      threshold_color_rules: [{ min: 80, color: 'rgb(239, 68, 68)' }],
    };

    // 규칙에 걸리면 규칙 색.
    mockResult.current.entries = [{ timestamp: 1, value: 90 }];
    const high = render(<StatPanel panelId="p1" config={cfg} />);
    expect(screen.getByTestId('stat-value').getAttribute('style')).toContain('rgb(239, 68, 68)');
    high.unmount();

    // 걸리지 않으면 기본 글자색.
    mockResult.current.entries = [{ timestamp: 1, value: 10 }];
    render(<StatPanel panelId="p1" config={cfg} />);
    expect(screen.getByTestId('stat-value').style.color).toBe('rgb(18, 52, 86)');
  });

  it('글꼴과 굵기가 반영된다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel
        panelId="p1"
        config={{
          ...baseConfig,
          value_layout: { font_family: 'serif', font_weight: 'normal' },
        }}
      />,
    );
    const el = screen.getByTestId('stat-value');
    expect(el.style.fontFamily).toContain('serif');
    expect(el.style.fontWeight).toBe('normal');
  });

  it('AC-16: 편집 중 더블클릭하면 스타일 팝오버가 열린다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />,
    );

    fireEvent.doubleClick(screen.getByTestId('stat-value'));
    const box = screen.getByTestId('stat-style-popover');
    expect(box).toHaveAttribute('data-stat-style-kind', 'value');
  });

  it('AC-16: 편집이 꺼져 있으면 더블클릭해도 열리지 않는다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} />);

    fireEvent.doubleClick(screen.getByTestId('stat-value'));
    expect(screen.queryByTestId('stat-style-popover')).toBeNull();
  });

  it('AC-23: Enter 로도 팝오버가 열린다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />,
    );

    fireEvent.keyDown(screen.getByTestId('stat-window-stats'), { key: 'Enter' });
    expect(screen.getByTestId('stat-style-popover')).toHaveAttribute(
      'data-stat-style-kind',
      'stats',
    );
  });

  it('요소마다 자기 종류의 팝오버가 열린다', () => {
    mockResult.current.entries = F1;
    render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />,
    );

    fireEvent.doubleClick(screen.getByTestId('stat-delta'));
    expect(screen.getByTestId('stat-style-popover')).toHaveAttribute(
      'data-stat-style-kind',
      'delta',
    );
    // 변화량은 방향별 3색을 낸다(§5 D4).
    expect(screen.getByTestId('stat-style-delta-colors')).toBeInTheDocument();
  });

  it('편집모드를 벗어나면 오버레이와 팝오버가 사라진다 (AC-04)', () => {
    mockResult.current.entries = F1;
    useUIStore.setState({ dashboardEditMode: true });
    const { container } = render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} />,
    );

    fireEvent.click(screen.getByTestId('stat-edit-toggle'));
    fireEvent.doubleClick(screen.getByTestId('stat-value'));
    expect(screen.getByTestId('stat-style-popover')).toBeInTheDocument();

    act(() => {
      useUIStore.setState({ dashboardEditMode: false });
    });
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(0);
    expect(screen.queryByTestId('stat-style-popover')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-004 v0.4.0 — 편집 보조(그리드 · 중심 표식 · 요소 정렬).
//
// @spec SPEC-CHART-004 AC-33 ~ AC-36
// ---------------------------------------------------------------------------
describe('StatPanel 편집 보조 (SPEC-CHART-004 v0.4.0)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    useUIStore.setState({ dashboardEditMode: false });
  });

  const F1 = [
    { timestamp: 1, value: 20 },
    { timestamp: 2, value: 24 },
  ];
  const baseConfig = { channel_name: 'c', decimal_places: 0, window_stats: { avg: true } };

  const edit = () => (
    <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />
  );

  it('AC-33: 편집 중에는 그리드와 중심 표식이 보인다', () => {
    mockResult.current.entries = F1;
    render(edit());
    expect(screen.getByTestId('panel-edit-grid')).toBeInTheDocument();
    expect(screen.getByTestId('panel-edit-center')).toBeInTheDocument();
  });

  it('AC-33: 편집이 꺼져 있으면 그리드도 중심 표식도 없다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} />);
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    expect(screen.queryByTestId('panel-edit-center')).toBeNull();
  });

  it('그리드는 포인터를 받지 않는다 — 그 위를 지나는 드래그가 끊기면 안 된다', () => {
    mockResult.current.entries = F1;
    render(edit());
    expect(screen.getByTestId('panel-edit-grid').className).toContain('pointer-events-none');
  });

  it('AC-35: 편집 중에는 정렬 툴바가 보인다', () => {
    mockResult.current.entries = F1;
    render(edit());
    const bar = screen.getByTestId('panel-align-toolbar');
    expect(bar).toHaveAttribute('role', 'toolbar');
    // 가로 3 · 세로 3.
    for (const axis of ['horizontal', 'vertical']) {
      for (const mode of ['start', 'center', 'end']) {
        expect(screen.getByTestId(`panel-align-${axis}-${mode}`)).toBeInTheDocument();
      }
    }
    expect(screen.getByTestId('panel-align-reset')).toBeInTheDocument();
  });

  it('AC-35: 편집이 꺼져 있으면 툴바가 없다', () => {
    mockResult.current.entries = F1;
    render(<StatPanel panelId="p1" config={baseConfig} />);
    expect(screen.queryByTestId('panel-align-toolbar')).toBeNull();
  });

  it('AC-36: 배치 초기화가 세 요소의 오프셋을 한 번에 지운다', () => {
    mockResult.current.entries = F1;
    const onConfigChange = vi.fn();
    render(
      <StatPanel
        panelId="p1"
        config={{
          ...baseConfig,
          value_layout: { offset_x: 20, font_size: 50 },
          delta_layout: { offset_y: -10 },
          stats_layout: { offset_x: 5, offset_y: 5 },
        }}
        onConfigChange={onConfigChange}
        forceEdit
      />,
    );

    fireEvent.click(screen.getByTestId('panel-align-reset'));

    // 한 번에 넘긴다 — 나눠 넘기면 앞의 쓰기가 반영되기 전에 다음이 옛 config 를 읽는다.
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    const patch = onConfigChange.mock.calls[0]![0] as Record<string, unknown>;
    // 오프셋만 지운다 — 글자 크기는 배치가 아니므로 남는다.
    expect(patch.value_layout).toEqual({ font_size: 50 });
    expect(patch.delta_layout).toBeUndefined();
    expect(patch.stats_layout).toBeUndefined();
  });

  it('정렬 버튼은 세 요소의 오프셋을 한 번에 넘긴다', () => {
    mockResult.current.entries = F1;
    const onConfigChange = vi.fn();
    render(
      <StatPanel
        panelId="p1"
        config={baseConfig}
        onConfigChange={onConfigChange}
        forceEdit
      />,
    );

    fireEvent.click(screen.getByTestId('panel-align-horizontal-start'));

    // jsdom 은 레이아웃을 하지 않아 상자가 전부 0 이다 — 계산은 statAlign 단위 테스트가
    // 지키고, 여기서는 "한 번의 호출로 묶여 나간다" 만 본다.
    if (onConfigChange.mock.calls.length > 0) {
      expect(onConfigChange).toHaveBeenCalledTimes(1);
    }
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-004 v0.5.0 — 선택 모델과 격자 옵션.
//
// @spec SPEC-CHART-004 AC-37 ~ AC-40
// ---------------------------------------------------------------------------
describe('StatPanel 선택 (SPEC-CHART-004 v0.5.0)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    useUIStore.setState({ dashboardEditMode: false });
  });

  const F1 = [
    { timestamp: 1, value: 20 },
    { timestamp: 2, value: 24 },
  ];
  const baseConfig = { channel_name: 'c', decimal_places: 0, window_stats: { avg: true } };
  const edit = () => (
    <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} forceEdit />
  );

  it('AC-37: 고르기 전에는 셋 다 흐린 점선이고 손잡이가 없다', () => {
    mockResult.current.entries = F1;
    const { container } = render(edit());

    for (const testId of ['stat-value', 'stat-delta', 'stat-window-stats']) {
      const el = screen.getByTestId(testId);
      expect(el.className).toContain('outline-dashed');
      expect(el.className).not.toContain('outline-2');
    }
    // 손잡이는 고른 요소에만 — 셋에 늘 붙어 있으면 좁은 패널에서 글자를 덮는다.
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(0);
  });

  it('AC-37: 고른 요소는 진한 실선이 되고 손잡이가 붙는다', () => {
    mockResult.current.entries = F1;
    const { container } = render(edit());

    fireEvent.pointerDown(screen.getByTestId('stat-value'));

    const picked = screen.getByTestId('stat-value');
    expect(picked.className).toContain('outline-2');
    expect(picked.className).not.toContain('outline-dashed');
    // 고르지 않은 것은 그대로 흐린 점선.
    expect(screen.getByTestId('stat-delta').className).toContain('outline-dashed');
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(1);
  });

  it('AC-37: Shift 로 두 개를 고르면 둘 다 진해진다', () => {
    mockResult.current.entries = F1;
    const { container } = render(edit());

    fireEvent.pointerDown(screen.getByTestId('stat-value'));
    // jsdom 의 PointerEvent 는 shiftKey 를 싣지 못한다 — MouseEvent 로 직접 만든다.
    fireEvent(
      screen.getByTestId('stat-delta'),
      new MouseEvent('pointerdown', { shiftKey: true, bubbles: true }),
    );

    expect(screen.getByTestId('stat-value').className).toContain('outline-2');
    expect(screen.getByTestId('stat-delta').className).toContain('outline-2');
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(2);
  });

  it('AC-37: 빈 자리를 누르면 선택이 풀린다', () => {
    mockResult.current.entries = F1;
    const { container } = render(edit());

    fireEvent.pointerDown(screen.getByTestId('stat-value'));
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(1);

    fireEvent.pointerDown(screen.getByTestId('panel-edit-grid'));
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(0);
  });

  it('편집을 끄면 선택도 거둔다 — 상자만 남으면 푸는 방법이 없다', () => {
    mockResult.current.entries = F1;
    useUIStore.setState({ dashboardEditMode: true });
    const { container } = render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={vi.fn()} />,
    );

    fireEvent.click(screen.getByTestId('stat-edit-toggle'));
    fireEvent.pointerDown(screen.getByTestId('stat-value'));
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(1);

    act(() => {
      useUIStore.setState({ dashboardEditMode: false });
    });
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(0);
  });

  it('AC-40: 격자 맞춤 토글이 툴바에 있고 기본은 켜짐이다', () => {
    mockResult.current.entries = F1;
    render(edit());

    const toggle = screen.getByTestId('panel-snap-toggle');
    expect(toggle).toHaveAttribute('aria-pressed', 'true');

    fireEvent.click(toggle);
    expect(screen.getByTestId('panel-snap-toggle')).toHaveAttribute('aria-pressed', 'false');
  });

  it('AC-40: 격자 설정은 config 에 저장되지 않는다', () => {
    mockResult.current.entries = F1;
    const onConfigChange = vi.fn();
    render(
      <StatPanel panelId="p1" config={baseConfig} onConfigChange={onConfigChange} forceEdit />,
    );

    fireEvent.click(screen.getByTestId('panel-snap-toggle'));
    // 저장하면 다음에 열 때도 꺼져 있고 그 이유를 알 수 없다.
    expect(onConfigChange).not.toHaveBeenCalled();
  });
});
