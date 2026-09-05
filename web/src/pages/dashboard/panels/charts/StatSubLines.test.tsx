// StatSubLines 단위 테스트.
//
// @spec SPEC-CHART-003 AC-11 / AC-13 / AC-21 / AC-22 / AC-24

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { DEFAULT_DELTA_COLORS } from './statDisplayOptions';
import { SUB_LINE_PX, StatDeltaLine, StatWindowStatsLine } from './StatSubLines';

describe('StatDeltaLine (AC-11 / AC-13 / AC-24)', () => {
  const colors = { up: '#123456', down: '#654321', flat: '#abcdef' };

  it('방향별로 지정된 색을 인라인으로 싣는다', () => {
    const up = render(
      <StatDeltaLine arrow="↑" text="+1.5" colors={colors} scale={1} testId="stat-delta" />,
    );
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(18, 52, 86)');
    up.unmount();

    const down = render(
      <StatDeltaLine arrow="↓" text="-1.5" colors={colors} scale={1} testId="stat-delta" />,
    );
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(101, 67, 33)');
    down.unmount();

    render(<StatDeltaLine arrow="→" text="+0" colors={colors} scale={1} testId="stat-delta" />);
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(171, 205, 239)');
  });

  it('색을 지정하지 않으면 현행 기본색을 쓴다', () => {
    render(
      <StatDeltaLine
        arrow="↑"
        text="+1"
        colors={DEFAULT_DELTA_COLORS}
        scale={1}
        testId="stat-delta"
      />,
    );
    // #10b981
    expect(screen.getByTestId('stat-delta').style.color).toBe('rgb(16, 185, 129)');
  });

  it('AC-13: 증가·감소에 같은 색을 줘도 방향은 화살표와 부호가 전달한다', () => {
    const same = { up: '#888888', down: '#888888', flat: '#888888' };
    const up = render(
      <StatDeltaLine arrow="↑" text="+4" colors={same} scale={1} testId="stat-delta" />,
    );
    expect(screen.getByTestId('stat-delta').textContent).toContain('↑');
    expect(screen.getByTestId('stat-delta').textContent).toContain('+4');
    up.unmount();

    render(<StatDeltaLine arrow="↓" text="-4" colors={same} scale={1} testId="stat-delta" />);
    expect(screen.getByTestId('stat-delta').textContent).toContain('↓');
    expect(screen.getByTestId('stat-delta').textContent).toContain('-4');
  });

  it('AC-24: 크기는 기본 px 에 배율을 곱한 값이다', () => {
    render(<StatDeltaLine arrow="↑" text="+1" colors={colors} scale={2} testId="stat-delta" />);
    expect(screen.getByTestId('stat-delta').style.fontSize).toBe(`${SUB_LINE_PX * 2}px`);
  });

  it('배율 1 은 종전 text-sm 과 같은 14px 다', () => {
    expect(SUB_LINE_PX).toBe(14);
    render(<StatDeltaLine arrow="↑" text="+1" colors={colors} scale={1} testId="stat-delta" />);
    expect(screen.getByTestId('stat-delta').style.fontSize).toBe('14px');
  });
});

describe('StatWindowStatsLine (AC-21 / AC-22 / AC-24)', () => {
  const items = [
    { kind: 'avg' as const, text: '22.6' },
    { kind: 'max' as const, text: '26' },
    { kind: 'min' as const, text: '20' },
  ];

  it('받은 순서 그대로 한 줄에 인라인 배치한다', () => {
    render(
      <StatWindowStatsLine items={items} compact={false} scale={1} testId="stat-window-stats" />,
    );
    const text = screen.getByTestId('stat-window-stats').textContent ?? '';
    expect(text.indexOf('22.6')).toBeLessThan(text.indexOf('26'));
    expect(text.indexOf('26')).toBeLessThan(text.indexOf('20'));
  });

  it('켠 항목만 그린다', () => {
    render(
      <StatWindowStatsLine
        items={[{ kind: 'max', text: '26' }]}
        compact={false}
        scale={1}
        testId="stat-window-stats"
      />,
    );
    const text = screen.getByTestId('stat-window-stats').textContent ?? '';
    expect(text).toContain('26');
    expect(text).not.toContain('22.6');
  });

  it('항목이 없으면 줄 자체를 그리지 않는다', () => {
    render(<StatWindowStatsLine items={[]} compact={false} scale={1} testId="stat-window-stats" />);
    expect(screen.queryByTestId('stat-window-stats')).toBeNull();
  });

  it('AC-21: 값 없는 항목은 자리를 지키고 — 를 그린다', () => {
    render(
      <StatWindowStatsLine
        items={[
          { kind: 'avg', text: null },
          { kind: 'max', text: null },
          { kind: 'min', text: null },
        ]}
        compact={false}
        scale={1}
        testId="stat-window-stats"
      />,
    );
    const line = screen.getByTestId('stat-window-stats');
    // 항목이 생략되면 켜 둔 항목의 자리가 조합마다 움직인다 — 3칸이 그대로 남아야 한다.
    expect(line.querySelectorAll('[data-stat-kind]')).toHaveLength(3);
    expect((line.textContent ?? '').match(/—/g)).toHaveLength(3);
  });

  it('AC-22: compact 는 축약 라벨을 쓰되 스크린리더에는 완결 낱말을 준다', () => {
    render(
      <StatWindowStatsLine items={items} compact scale={1} testId="stat-window-stats" />,
    );
    const line = screen.getByTestId('stat-window-stats');
    // 화면용 축약 라벨은 aria-hidden, 낭독용 완결 라벨은 sr-only 로 함께 존재한다.
    expect(line.querySelector('[aria-hidden="true"]')).not.toBeNull();
    expect(line.querySelectorAll('.sr-only').length).toBe(items.length);
    expect(line.textContent).toContain('dashboard.chart.windowStat.avg');
  });

  it('compact 가 아니면 완결 라벨만 그린다', () => {
    render(
      <StatWindowStatsLine items={items} compact={false} scale={1} testId="stat-window-stats" />,
    );
    const line = screen.getByTestId('stat-window-stats');
    expect(line.querySelectorAll('.sr-only').length).toBe(0);
    expect(line.textContent).toContain('dashboard.chart.windowStat.avg');
  });

  it('AC-24: 크기는 변화량과 같은 기본 px × 배율이다', () => {
    render(<StatWindowStatsLine items={items} compact={false} scale={2} testId="stat-window-stats" />);
    expect(screen.getByTestId('stat-window-stats').style.fontSize).toBe(`${SUB_LINE_PX * 2}px`);
  });
});
