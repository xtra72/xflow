// StatPanel 테스트.
// useChartChannel 을 vi.mock 으로 교체해 entries/status 를 직접 주입한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

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
