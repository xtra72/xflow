// X축 범위(SeriesRange) · 툴팁 · Y축 소수 자릿수의 **패널 통합** 검증.
//
// 세 가지 모두 config 를 읽어 recharts 로 넘기는 배선이라, 순수 함수 테스트
// (seriesRange.test.ts)만으로는 "설정은 맞는데 화면이 안 변한다" 를 잡지 못한다.
// 특히 X축은 구 `time_window_mode` 어휘를 폴백으로 읽으므로, 저장된 패널이
// 종전과 같은 축을 그리는지가 이 파일의 핵심이다.

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';

const mockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    seriesEntries: new Map<string, ChartEntry[]>(),
    seriesStyles: new Map(),
    seriesNames: [] as string[],
    booleanSeries: new Set<string>(),
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));
const hookOpts = vi.hoisted(() => ({ maxPoints: [] as unknown[] }));

vi.mock('./useChartChannel', () => ({
  useChartChannel: (_name?: string, opts?: { maxPoints?: number }) => {
    hookOpts.maxPoints.push(opts?.maxPoints);
    return mockResult.current;
  },
}));
vi.mock('./useChartChannels', () => ({
  useChartChannels: () => ({
    states: new Map(),
    seriesNames: [],
    booleanSeries: new Set<string>(),
  }),
}));
vi.mock('./useStoreChartData', () => ({
  useStoreChartData: () => ({
    seriesEntries: new Map(),
    seriesNames: [],
    booleanSeries: new Set<string>(),
    status: 'idle',
  }),
}));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import LineChartPanel from './LineChartPanel';

function wrapper({ children }: { children: ReactNode }) {
  return <>{children}</>;
}

const NOW = 1_700_000_000_000;

function renderPanel(config: Record<string, unknown>) {
  return render(<LineChartPanel panelId="p1" title="t" config={{ channel_name: 'c', ...config }} />, {
    wrapper,
  });
}

function xDomain(): [number, number] | string[] {
  const raw = screen.getByTestId('rc-xaxis').getAttribute('data-domain');
  return JSON.parse(raw ?? 'null');
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  hookOpts.maxPoints.length = 0;
  mockResult.current.entries = [];
});

describe('X축 범위 — 새 어휘와 구 어휘가 같은 축을 그린다', () => {
  it('최근(relative) 범위는 [now-w, now] 축을 만든다', () => {
    renderPanel({ x_range: { mode: 'relative', window_ms: 60_000 } });
    expect(xDomain()).toEqual([NOW - 60_000, NOW]);
  });

  it('구 time_window_mode=recent 도 같은 축을 만든다', () => {
    renderPanel({ time_window_mode: 'recent', recent_window_sec: 60 });
    expect(xDomain()).toEqual([NOW - 60_000, NOW]);
  });

  it('구간(absolute) 범위는 지정한 양끝을 축으로 쓴다', () => {
    renderPanel({ x_range: { mode: 'absolute', start_ms: 1000, end_ms: 5000 } });
    expect(xDomain()).toEqual([1000, 5000]);
  });

  it('구 time_window_mode=fixed 도 같은 축을 만든다', () => {
    renderPanel({ time_window_mode: 'fixed', fixed_start_ms: 1000, fixed_end_ms: 5000 });
    expect(xDomain()).toEqual([1000, 5000]);
  });

  it('포인트(count) 범위는 시간축을 고정하지 않는다 — 데이터가 정한다', () => {
    renderPanel({ x_range: { mode: 'count', count: 50 } });
    expect(xDomain()).toEqual(['dataMin', 'dataMax']);
  });

  it('버퍼 크기는 범위가 정한다 — 갯수는 그 값', () => {
    renderPanel({ x_range: { mode: 'count', count: 321 } });
    expect(hookOpts.maxPoints.at(-1)).toBe(321);
  });

  it('버퍼 크기는 범위가 정한다 — 최근 기간은 1Hz 가정 2배', () => {
    renderPanel({ x_range: { mode: 'relative', window_ms: 600_000 } });
    expect(hookOpts.maxPoints.at(-1)).toBe(1200);
  });

  it('구 max_points 도 그대로 버퍼 크기가 된다', () => {
    renderPanel({ max_points: 321 });
    expect(hookOpts.maxPoints.at(-1)).toBe(321);
  });
});

describe('툴팁', () => {
  it('기본은 켬 + 전체 시리즈 — 저장된 패널의 동작', () => {
    renderPanel({});
    // 커스텀 content 를 주지 않는다 = 축 위의 모든 시리즈를 그대로 보여 준다.
    expect(screen.getByTestId('rc-tooltip').getAttribute('data-single')).toBeNull();
  });

  it('사용을 끄면 툴팁을 아예 렌더하지 않는다', () => {
    renderPanel({ tooltip: { enabled: false } });
    expect(screen.queryByTestId('rc-tooltip')).toBeNull();
  });

  it('단일 값이면 payload 를 좁히는 content 를 넘긴다', () => {
    // `shared={false}` 는 v3 LineChart 에서 무시된다 — 그래서 content 로 좁힌다.
    renderPanel({ tooltip: { single: true } });
    expect(screen.getByTestId('rc-tooltip').getAttribute('data-single')).toBe('true');
  });
});

describe('Y축 소수 자릿수', () => {
  function tickSample(): string | null {
    return screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample');
  }

  it('자릿수도 단위도 없으면 포맷터를 주지 않는다 — 기본 표기 유지', () => {
    renderPanel({});
    expect(tickSample()).toBeNull();
  });

  it('자릿수를 정하면 눈금이 그 자리까지 반올림된다', () => {
    renderPanel({ decimal_places: 2 });
    expect(tickSample()).toBe('12.35');
  });

  it('0 자리도 유효하다 — 정수로 끊는다', () => {
    renderPanel({ decimal_places: 0 });
    expect(tickSample()).toBe('12');
  });

  it('단위만 있으면 값을 그대로 두고 단위만 붙인다(종전 동작)', () => {
    renderPanel({ y_unit: 'C' });
    expect(tickSample()).toBe('12.3456C');
  });

  it('자릿수와 단위는 함께 적용된다', () => {
    renderPanel({ decimal_places: 1, y_unit: 'C' });
    expect(tickSample()).toBe('12.3C');
  });
});

describe('툴팁 값의 소수 자릿수', () => {
  function fmtNumber(): string | null {
    return screen.getByTestId('rc-tooltip').getAttribute('data-fmt-number');
  }

  it('자릿수가 없으면 포맷터를 주지 않는다 — 기본 표기 유지', () => {
    renderPanel({});
    expect(fmtNumber()).toBeNull();
  });

  it('설정한 자릿수로 끊는다 — 축 눈금과 같은 값', () => {
    renderPanel({ decimal_places: 2 });
    expect(fmtNumber()).toBe('12.35');
    // 같은 설정이 축에도 적용되어 두 곳의 자릿수가 일치한다.
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample')).toBe('12.35');
  });

  it('0 자리도 유효하다', () => {
    renderPanel({ decimal_places: 0 });
    expect(fmtNumber()).toBe('12');
  });

  it('단위는 툴팁에 붙이지 않는다 — 축 눈금 전용이다', () => {
    renderPanel({ decimal_places: 1, y_unit: 'C' });
    expect(fmtNumber()).toBe('12.3');
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample')).toBe('12.3C');
  });

  it('열거형 축에서 매핑에 없는 값도 자릿수를 따른다', () => {
    renderPanel({
      decimal_places: 1,
      y_axis_type: 'enum',
      y_enum_labels: [{ value: 0, label: '정지' }],
    });
    // 12.3456 은 매핑에 없다 → 라벨이 아니라 숫자로 떨어진다.
    expect(fmtNumber()).toBe('12.3');
  });

  it('열거형 매핑에 있는 값은 라벨이 이긴다 — 자릿수 개념이 없다', () => {
    renderPanel({
      decimal_places: 2,
      y_axis_type: 'enum',
      y_enum_labels: [{ value: 12.3456, label: '가동' }],
    });
    expect(fmtNumber()).toBe('가동');
  });
});
