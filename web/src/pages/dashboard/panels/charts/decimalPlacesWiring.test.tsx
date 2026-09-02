// 값 표기 자릿수 배선 테스트 — 차트 계열 패널이 실제로 공용 규칙을 쓰는가.
//
// `decimalPlaces.test.ts` 가 규칙 자체를 잠그고, 이 파일은 **패널이 그 규칙에 연결되어
// 있는가**를 잠근다. 종전에는 패널마다 자릿수 처리가 흩어져 있어(통계 `?? 2`, 라인
// "미지정이면 원값", 게이지·바·파이·테이블은 반올림 없음) 같은 시리즈가 패널마다 다른
// 자릿수로 읽혔다.
//
// 축·눈금 계열(라인 Y축 · 바 Y축)은 **기본값을 따르지 않는다**. 그 구분도 함께 잠근다.

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('recharts', async () => {
  const stub = await import('./__mocks__/rechartsStub');
  return stub;
});

// 채널 훅은 고정 엔트리를 돌려준다 — 자릿수만 보므로 데이터 경로는 최소로 둔다.
const ENTRIES = [
  { timestamp: 1_000, value: 21.533333333333335, labels: { name: 'a' } },
  { timestamp: 2_000, value: 10, labels: { name: 'b' } },
];

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => ({ entries: ENTRIES, status: 'connected' }),
}));

vi.mock('../../panelChromeContext', () => ({
  usePanelTitleVisible: () => true,
  usePanelTitleStyle: () => undefined,
}));

// 공용 데이터 경로가 에이전트 목록을 조회한다 — 조회 컨텍스트 없이 렌더하려고 스텁한다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

import BarChartPanel from './BarChartPanel';
import PieChartPanel from './PieChartPanel';
import { formatCell } from './tableColumns';
import { parseConfig, withGaugeValue } from '../gauge/gaugeShapes';

/** 툴팁 스텁이 노출하는 포맷터 표본(입력 12.3456). */
function tooltipSample(): string | null {
  return screen.getByTestId('rc-tooltip').getAttribute('data-fmt-number');
}

/** Y축 스텁이 노출하는 눈금 포맷터 표본. 포맷터가 없으면 null. */
function axisSample(): string | null {
  return screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample');
}

describe('바 차트 — 값 툴팁', () => {
  it('자릿수를 지정하지 않아도 기본 2자리로 끊는다', () => {
    // 종전에는 포맷터가 없어 원값(12.3456)이 그대로 나왔다.
    render(<BarChartPanel panelId="p" title="t" config={{ channel_name: 'c' }} />);
    expect(tooltipSample()).toBe('12.35');
  });

  it('지정한 자릿수를 따른다', () => {
    render(
      <BarChartPanel panelId="p" title="t" config={{ channel_name: 'c', decimal_places: 0 }} />,
    );
    expect(tooltipSample()).toBe('12');
  });
});

describe('바 차트 — Y축 눈금', () => {
  it('자릿수를 지정하지 않으면 눈금 크기가 자릿수를 정한다', () => {
    // 눈금은 값 읽기가 아니라 눈금자다. 값 설정을 그대로 걸면 `0.00 · 25.00` 이 되므로
    // 눈금 자릿수는 **그 눈금의 크기**가 정한다(표본 12.3456 은 10 초과 → 정수).
    render(<BarChartPanel panelId="p" title="t" config={{ channel_name: 'c' }} />);
    expect(axisSample()).toBe('12');
  });

  it('자릿수를 지정하면 눈금도 같은 자릿수를 쓴다', () => {
    render(
      <BarChartPanel panelId="p" title="t" config={{ channel_name: 'c', decimal_places: 1 }} />,
    );
    expect(axisSample()).toBe('12.3');
  });
});

describe('파이 차트 — 값 툴팁', () => {
  it('자릿수를 지정하지 않아도 기본 2자리로 끊는다', () => {
    render(<PieChartPanel panelId="p" title="t" config={{ channel_name: 'c' }} />);
    expect(tooltipSample()).toBe('12.35');
  });

  it('지정한 자릿수를 따른다', () => {
    render(
      <PieChartPanel panelId="p" title="t" config={{ channel_name: 'c', decimal_places: 3 }} />,
    );
    expect(tooltipSample()).toBe('12.346');
  });
});

describe('테이블 — 수치 열', () => {
  it('기본 2자리로 끊는다 (종전에는 String(n) 이라 원값이 나왔다)', () => {
    expect(formatCell(21.533333333333335, 'number')).toBe('21.53');
  });

  it('지정 자릿수를 따르고, 수치가 아닌 열은 영향이 없다', () => {
    expect(formatCell(21.5, 'number', 0)).toBe('22');
    expect(formatCell('abc', 'string', 0)).toBe('abc');
    // 시각 열은 자릿수와 무관하게 포맷된다.
    expect(formatCell(1_700_000_000_000, 'datetime', 4)).not.toContain('.0000');
  });
});

describe('게이지 — 가운데 숫자', () => {
  it('기본 2자리로 끊는다 (종전에는 반올림 자체가 없었다)', () => {
    expect(parseConfig({ value: 21.533333333333335 }).valueText).toBe('21.53');
  });

  it('지정 자릿수를 따른다', () => {
    expect(parseConfig({ value: 21.5, decimal_places: 0 }).valueText).toBe('22');
  });

  it('자동 데이터 량은 값과 접미사가 함께 정해진다', () => {
    const cfg = parseConfig({ value: 1024 * 1024 * 3, decimal_places: 0, unit: 'auto:bytes' });
    expect(cfg.valueText).toBe('3');
    expect(cfg.unit).toBe('MB');
    // 저장값은 따로 남겨 둔다 — 값이 바뀔 때 접미사를 다시 정하려면 필요하다.
    expect(cfg.configuredUnit).toBe('auto:bytes');
  });

  it('라이브 값으로 갈아 끼우면 접미사도 함께 바뀐다', () => {
    // 값이 자리를 넘어가면 단위도 넘어가야 한다 — `3MB` 옆에 `KB` 가 남으면 안 된다.
    const base = parseConfig({ value: 512, decimal_places: 0, unit: 'auto:bytes' });
    expect(base.unit).toBe('B');
    const next = withGaugeValue(base, 1024 * 1024 * 3);
    expect(next.valueText).toBe('3');
    expect(next.unit).toBe('MB');
  });

  it('라이브 값으로 갈아 끼워도 표기가 함께 바뀐다', () => {
    // `{ ...base, value }` 로 덮으면 표기만 옛 값에 멈춘다 — 그 함정을 잠근다.
    const base = parseConfig({ value: 1, decimal_places: 1 });
    const next = withGaugeValue(base, 42.789);
    expect(next.value).toBe(42.789);
    expect(next.valueText).toBe('42.8');
  });
});
