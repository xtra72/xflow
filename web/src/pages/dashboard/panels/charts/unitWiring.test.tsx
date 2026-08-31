// 값 단위 배선 테스트 — 게이지에만 있던 단위 설정이 다른 차트에도 실제로 닿는가.
//
// `decimalPlacesWiring.test.tsx` 가 자릿수 배선을 잠그는 것과 같은 자리다. 종전에는
// 단위 목록 UI 가 게이지 설정 안에만 있었고, 바·파이·테이블은 단위 개념 자체가 없어
// 같은 대시보드에서 온도 값이 패널마다 `21.5°C` 와 `21.5` 로 갈렸다.
//
// 단위는 **값에만** 붙는다. 파이의 조각 라벨(전체 대비 비중 %)처럼 축이 다른 표기에는
// 붙이지 않는다.

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('recharts', async () => {
  const stub = await import('./__mocks__/rechartsStub');
  return stub;
});

const ENTRIES = [
  { timestamp: 1_000, value: 21.533333333333335, labels: { name: 'a' } },
  { timestamp: 2_000, value: 10, labels: { name: 'b' } },
];

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => ({ entries: ENTRIES, status: 'connected' }),
}));

vi.mock('../../panelChromeContext', () => ({
  usePanelTitleVisible: () => true,
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

import BarChartPanel from './BarChartPanel';
import PieChartPanel from './PieChartPanel';
import { formatCell } from './tableColumns';
import { AUTO_BYTES_UNIT, isPresetUnit, withUnit } from './unitOptions';

/** 툴팁 스텁이 노출하는 포맷터 표본(입력 12.3456). */
function tooltipSample(): string | null {
  return screen.getByTestId('rc-tooltip').getAttribute('data-fmt-number');
}

/** Y축 스텁이 노출하는 눈금 포맷터 표본. 포맷터가 없으면 null. */
function axisSample(): string | null {
  return screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample');
}

describe('단위 잇기 규칙', () => {
  it('붙임표 없이 잇는다 — 계측 표기 관행이고 좁은 타일에서 갈라지지 않는다', () => {
    expect(withUnit('21.53', '°C')).toBe('21.53°C');
  });

  it('단위가 비면 값만 남는다', () => {
    expect(withUnit('21.53', undefined)).toBe('21.53');
    expect(withUnit('21.53', '')).toBe('21.53');
  });

  it('목록에 있는 단위와 직접 적은 단위를 구분한다', () => {
    expect(isPresetUnit('°C')).toBe(true);
    // 빈 값은 목록의 "없음" 항목이다 — 커스텀이 아니다.
    expect(isPresetUnit('')).toBe(true);
    expect(isPresetUnit('sccm')).toBe(false);
  });
});

describe('바 차트 — 단위', () => {
  it('툴팁 값에 단위가 붙는다', () => {
    render(<BarChartPanel panelId="p" title="t" config={{ channel_name: 'c', unit: 'kW' }} />);
    expect(tooltipSample()).toBe('12.35kW');
  });

  it('Y축 눈금에는 단위를 붙이지 않는다 — 자릿수만 크기가 정한다', () => {
    // 단위는 값 표기에서 한 번만 말한다. 표본 12.3456 은 10 초과라 정수다.
    render(<BarChartPanel panelId="p" title="t" config={{ channel_name: 'c', unit: 'kW' }} />);
    expect(axisSample()).toBe('12');
  });

  it('단위와 자릿수를 함께 지정하면 눈금이 둘 다 따른다', () => {
    render(
      <BarChartPanel
        panelId="p"
        title="t"
        config={{ channel_name: 'c', unit: 'kW', decimal_places: 1 }}
      />,
    );
    expect(axisSample()).toBe('12.3');
  });

  it('단위도 자릿수도 없어도 눈금 자릿수는 크기가 정한다', () => {
    render(<BarChartPanel panelId="p" title="t" config={{ channel_name: 'c' }} />);
    expect(axisSample()).toBe('12');
  });
});

describe('파이 차트 — 단위', () => {
  it('툴팁 값에 단위가 붙는다', () => {
    render(<PieChartPanel panelId="p" title="t" config={{ channel_name: 'c', unit: 'L/min' }} />);
    expect(tooltipSample()).toBe('12.35L/min');
  });

  it('단위가 없으면 종전 표기 그대로다', () => {
    render(<PieChartPanel panelId="p" title="t" config={{ channel_name: 'c' }} />);
    expect(tooltipSample()).toBe('12.35');
  });
});

describe('테이블 — 열 단위', () => {
  it('수치 열에 단위가 붙는다', () => {
    expect(formatCell(21.533333333333335, 'number', 2, '°C')).toBe('21.53°C');
  });

  it('수로 볼 수 없는 값에는 붙이지 않는다 — `abc°C` 는 값도 단위도 아니다', () => {
    expect(formatCell('abc', 'number', 2, '°C')).toBe('abc');
  });

  it('시각·문자열 열은 단위를 무시한다', () => {
    expect(formatCell('ok', 'string', 2, '°C')).toBe('ok');
    expect(formatCell(1_700_000_000_000, 'datetime', 2, '°C')).not.toContain('°C');
  });
});

describe('자동 데이터 량 — 패널 배선', () => {
  it('바 차트 툴팁이 값 크기에 맞춰 접는다', () => {
    // 툴팁 표본은 12.3456 — 1024 미만이라 B 자리에 남는다.
    render(
      <BarChartPanel
        panelId="p"
        title="t"
        config={{ channel_name: 'c', unit: AUTO_BYTES_UNIT, decimal_places: 1 }}
      />,
    );
    expect(tooltipSample()).toBe('12.3B');
  });

  it('바 차트 Y축은 자릿수를 명시하지 않아도 접는다 — 값 자체가 바뀌는 규칙이다', () => {
    render(
      <BarChartPanel panelId="p" title="t" config={{ channel_name: 'c', unit: AUTO_BYTES_UNIT }} />,
    );
    // 표본 12.3456 은 1024 미만이라 B 자리에 남고, 10 초과라 정수로 찍힌다.
    // 접미사는 눈금에 붙지 않는다.
    expect(axisSample()).toBe('12');
  });

  it('파이 차트 툴팁도 같은 규칙을 쓴다', () => {
    render(
      <PieChartPanel
        panelId="p"
        title="t"
        config={{ channel_name: 'c', unit: AUTO_BYTES_UNIT, decimal_places: 2 }}
      />,
    );
    expect(tooltipSample()).toBe('12.35B');
  });

  it('표 셀이 크기에 맞춰 접는다', () => {
    expect(formatCell(1024 * 1024 * 3, 'number', 0, AUTO_BYTES_UNIT)).toBe('3MB');
    expect(formatCell(512, 'number', 0, AUTO_BYTES_UNIT)).toBe('512B');
  });

  it('표의 시각·문자열 열은 자동 단위와 무관하다', () => {
    expect(formatCell('abc', 'string', 0, AUTO_BYTES_UNIT)).toBe('abc');
  });
});
