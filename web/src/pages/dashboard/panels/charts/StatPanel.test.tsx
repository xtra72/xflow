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

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => mockResult.current,
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
    mockResult.current.entries = [{ timestamp: 1, value: 80 }];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c', decimal_places: 0 }} />);
    expect(screen.getByTestId('stat-value').getAttribute('style')).toBeNull();
  });
});
