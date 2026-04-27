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
