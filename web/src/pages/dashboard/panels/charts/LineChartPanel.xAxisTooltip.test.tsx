// X축 범위(SeriesRange) · 툴팁 · Y축 소수 자릿수의 **패널 통합** 검증.
//
// 세 가지 모두 config 를 읽어 recharts 로 넘기는 배선이라, 순수 함수 테스트
// (seriesRange.test.ts)만으로는 "설정은 맞는데 화면이 안 변한다" 를 잡지 못한다.
// 특히 X축은 구 `time_window_mode` 어휘를 폴백으로 읽으므로, 저장된 패널이
// 종전과 같은 축을 그리는지가 이 파일의 핵심이다.

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';
import { MIN_Y_AXIS_WIDTH } from './axisSize';

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

// 채널이 패널 소스에서 빠진 뒤로 데이터는 **시리즈 소스 훅 하나**로 들어온다.
// 그래서 이음매도 하나다 — 종전에는 채널 훅 둘 + store 훅을 각각 흉내 내야 했다.
vi.mock('./usePanelSeriesData', () => ({
  usePanelSeriesData: () => mockResult.current,
  // 실물을 끌어오지 않는다 — 그 모듈이 조회 계층을 함께 들여와 QueryClient 를 요구한다.
  isPanelSeriesSource: (binding: { active: boolean }) => binding.active,
}));
// 캔들은 조회 계층(react-query)을 쓰므로 QueryClient 없이는 렌더되지 않는다. 이 파일은
// 축·툴팁을 보므로 캔들은 비활성으로 흉내 낸다 — 종전에는 store 훅 모킹이 이 몫을 겸했다.
vi.mock('./useCandleSeriesData', () => ({
  useCandleSeriesData: () => new Map(),
}));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import LineChartPanel from './LineChartPanel';

function wrapper({ children }: { children: ReactNode }) {
  return <>{children}</>;
}

const NOW = 1_700_000_000_000;

/** 활성 store 소스 — 소스가 활성이어야 패널이 시리즈를 그린다. */
const STORE_CFG = {
  data_source: 'store',
  store_source: { agent_name: 'a', series: [{ key: 'k', field: 'value' }] },
} as const;

function renderPanel(config: Record<string, unknown>) {
  return render(<LineChartPanel panelId="p1" title="t" config={{ ...STORE_CFG, ...config }} />, {
    wrapper,
  });
}

function xDomain(): [number, number] | string[] {
  const raw = screen.getByTestId('rc-xaxis').getAttribute('data-domain');
  return JSON.parse(raw ?? 'null');
}

/**
 * 한 시리즈짜리 결과를 심는다.
 *
 * 패널은 `entries` 평탄 배열이 아니라 `seriesEntries`(이름 → 점 목록)를 읽는다 — 채널
 * 시절의 단일 타임라인과 달리 시리즈 소스는 여러 줄을 동시에 낸다. 테스트가 `entries` 만
 * 심으면 차트는 비어 있고, 그러면 축·툴팁 검증이 전부 헛돈다.
 */
function setEntries(entries: ChartEntry[], name = 'value'): void {
  mockResult.current.entries = entries;
  mockResult.current.seriesEntries = new Map([[name, entries]]);
  mockResult.current.seriesNames = entries.length > 0 ? [name] : [];
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  setEntries([]);
});

// 버퍼 크기(maxPoints) 검증은 없앴다 — 채널 링버퍼가 사라지면서 "몇 점을 들고 있을지" 라는
// 축 자체가 없어졌다. 시리즈 소스는 구간을 질의하고 그 결과를 그대로 그린다.
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
});

describe('툴팁', () => {
  it('기본은 켬 + 전체 시리즈 — 저장된 패널의 동작', () => {
    renderPanel({});
    // 좁히지 않는 공용 content = 축 위의 모든 시리즈를 그대로 보여 준다.
    // (그리기는 두 모드 모두 자체 content 가 맡는다 — 레이블 왼쪽·값 오른쪽 정렬 때문에.)
    expect(screen.getByTestId('rc-tooltip').getAttribute('data-single')).toBeNull();
    expect(screen.getByTestId('rc-tooltip').getAttribute('data-content')).toBe(
      'ChartTooltipContent',
    );
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

  it('눈금에는 단위를 붙이지 않고 자릿수만 크기가 정한다', () => {
    // 표본 12.3456 은 10 초과라 정수다.
    renderPanel({ y_unit: 'C' });
    expect(tickSample()).toBe('12');
  });

  it('자릿수와 단위는 함께 적용된다', () => {
    renderPanel({ decimal_places: 1, y_unit: 'C' });
    expect(tickSample()).toBe('12.3');
  });
});

describe('툴팁 값의 소수 자릿수', () => {
  function fmtNumber(): string | null {
    return screen.getByTestId('rc-tooltip').getAttribute('data-fmt-number');
  }

  it('자릿수를 지정하지 않아도 값은 기본 2자리로 끊는다', () => {
    // 종전에는 포맷터를 주지 않아 원값(12.3456)이 툴팁에 그대로 나왔다.
    renderPanel({});
    expect(fmtNumber()).toBe('12.35');
  });

  it('축 눈금은 기본값을 따르지 않는다 — 명시했을 때만 포맷한다', () => {
    // 눈금은 값 읽기가 아니라 눈금자다. 기본 2자리를 걸면 아무 설정도 안 한 패널의
    // 축이 `0.00 · 25.00` 이 된다(decimalPlaces.ts 머리말).
    renderPanel({});
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample')).toBeNull();
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

  it('단위는 툴팁에도 눈금에도 붙지 않는다 — 축 라벨이 한 번만 말한다', () => {
    // 라인 차트는 축 라벨에 `전력 (kW)` 형태로 단위를 이미 적는다. 눈금마다 다시 달면
    // 축을 따라 같은 글자가 반복되어 좁은 자리에서 겹친다.
    renderPanel({ decimal_places: 1, y_unit: 'C' });
    expect(fmtNumber()).toBe('12.3');
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample')).toBe('12.3');
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

describe('그래프 스타일', () => {
  function kinds(): string[] {
    return [
      ...screen.queryAllByTestId('rc-line'),
      ...screen.queryAllByTestId('rc-area'),
      ...screen.queryAllByTestId('rc-bar'),
    ]
      .filter((el) => el.getAttribute('data-line-key') === 'value')
      .map((el) => el.getAttribute('data-testid')!);
  }
  function stackIdOf(kind: string): string | null {
    const el = screen
      .queryAllByTestId(kind)
      .find((e) => e.getAttribute('data-line-key') === 'value');
    return el?.getAttribute('data-stack-id') ?? null;
  }

  it('미지정이면 라인 — 저장된 패널의 동작', () => {
    renderPanel({});
    expect(kinds()).toEqual(['rc-line']);
  });

  it('영역 스타일이면 Area 로 그린다', () => {
    renderPanel({ graph_style: 'area' });
    expect(kinds()).toEqual(['rc-area']);
  });

  it('바 스타일이면 Bar 로 그린다', () => {
    renderPanel({ graph_style: 'bar' });
    expect(kinds()).toEqual(['rc-bar']);
  });

  it('모르는 스타일은 라인으로 떨어진다', () => {
    renderPanel({ graph_style: 'hologram' });
    expect(kinds()).toEqual(['rc-line']);
  });

  it('스택킹은 바에서 stackId 를 붙인다', () => {
    renderPanel({ graph_style: 'bar', stacked: true });
    expect(stackIdOf('rc-bar')).toBe('stack');
  });

  it('스택킹은 영역에서도 붙는다', () => {
    renderPanel({ graph_style: 'area', stacked: true });
    expect(stackIdOf('rc-area')).toBe('stack');
  });

  it('라인은 스택킹을 켜도 붙지 않는다 — 쌓아도 누적으로 읽히지 않는다', () => {
    renderPanel({ graph_style: 'line', stacked: true });
    expect(stackIdOf('rc-line')).toBeNull();
  });

  it('스택킹을 끄면 어느 스타일이든 붙지 않는다', () => {
    renderPanel({ graph_style: 'bar', stacked: false });
    expect(stackIdOf('rc-bar')).toBeNull();
  });
});

describe('Y축 폭 — 제목이 잘리지 않게', () => {
  function width(): number {
    return Number(screen.getByTestId('rc-yaxis').getAttribute('data-width'));
  }

  it('제목이 있으면 없을 때보다 넓다 — 제목 자리를 눈금에서 뺏지 않는다', () => {
    renderPanel({});
    const without = width();
    cleanup();

    renderPanel({ y_label: '온도' });
    expect(width()).toBeGreaterThan(without);
  });

  it('제목 글꼴을 키우면 폭이 따라 는다', () => {
    renderPanel({ y_label: '온도', y_label_font: { size: 10 } });
    const small = width();
    cleanup();

    renderPanel({ y_label: '온도', y_label_font: { size: 28 } });
    expect(width()).toBeGreaterThan(small);
  });

  it('단위를 붙이면 눈금이 길어져 폭이 는다', () => {
    // 눈금 표본은 Y축 도메인에서 나온다 — 수동 범위를 줘야 숫자 눈금이 생긴다.
    const range = { y_axis_mode: 'manual', y_min: 0, y_max: 1000 };
    renderPanel({ y_label: '전력', ...range });
    const plain = width();
    cleanup();

    renderPanel({ y_label: '전력', y_unit: 'kW', decimal_places: 3, ...range });
    expect(width()).toBeGreaterThan(plain);
  });

  // 보고된 결함: 자동 축(기본값)에서 큰 값의 앞자리가 잘렸다(`100005270112` 이 `0000000`).
  // 눈금 표본을 도메인 양끝에서만 뽑아, 자동 축에서는 표본이 비고 폭이 최소값으로
  // 주저앉았다. 자동 축이 기본값이므로 잘림이 곧 기본 동작이었다.
  it('자동 축에서도 값이 크면 폭이 는다 — 눈금이 잘리지 않는다', () => {
    setEntries([{ timestamp: NOW, value: 12 }]);
    renderPanel({});
    const small = width();
    cleanup();

    setEntries([{ timestamp: NOW, value: 100_005_270_112 }]);
    renderPanel({});
    expect(width()).toBeGreaterThan(small);
  });

  it('자동 축 + 데이터 없음이면 종전 폭 그대로다 — 빈 패널의 그림이 변하지 않는다', () => {
    renderPanel({});
    expect(width()).toBe(MIN_Y_AXIS_WIDTH);
  });

  it('제목과 단위를 함께 표기한다', () => {
    renderPanel({ y_label: '온도', y_unit: 'C' });
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-label')).toBe('온도 (C)');
  });

  it('제목이 없으면 제목을 넘기지 않는다', () => {
    renderPanel({});
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-label')).toBeNull();
  });
});

// 경계 채우기 — recharts 의 ReferenceArea 는 기본값이 ifOverflow:'discard' 라
// 한 끝이라도 축 도메인 밖이면 통째로 그리지 않는다. 종전에는 `경계 이하`/`경계
// 이상` 을 ±1e9 로 표현해 두 모드가 화면에 아예 나오지 않았다.
describe('경계 채우기 (회귀)', () => {
  const range = { y_axis_mode: 'manual', y_min: 0, y_max: 100 };

  function areas(): Array<{ y1: number; y2: number; fill: string }> {
    return screen.queryAllByTestId('rc-reference-area').map((el) => ({
      y1: Number(el.getAttribute('data-ref-y1')),
      y2: Number(el.getAttribute('data-ref-y2')),
      fill: el.getAttribute('data-ref-fill') ?? '',
    }));
  }

  it('경계 이하가 그려진다 — 축 바닥부터 경계까지', () => {
    renderPanel({
      ...range,
      y_thresholds: [{ value: 30, color: '#f00', fill_direction: 'below' }],
    });
    expect(areas()).toEqual([{ y1: 0, y2: 30, fill: '#f00' }]);
  });

  it('경계 이상이 그려진다 — 경계부터 축 꼭대기까지', () => {
    renderPanel({
      ...range,
      y_thresholds: [{ value: 30, color: '#f00', fill_direction: 'above' }],
    });
    expect(areas()).toEqual([{ y1: 30, y2: 100, fill: '#f00' }]);
  });

  it('구간이 축 도메인을 넘지 않는다 — 넘으면 통째로 버려진다', () => {
    renderPanel({
      ...range,
      y_thresholds: [{ value: 30, color: '#f00', fill_direction: 'below' }],
    });
    for (const a of areas()) {
      expect(a.y1).toBeGreaterThanOrEqual(0);
      expect(a.y2).toBeLessThanOrEqual(100);
    }
  });

  it('색을 지정하지 않아도 채워진다 — 선과 같은 색 규칙', () => {
    renderPanel({ ...range, y_thresholds: [{ value: 30, fill_direction: 'above' }] });
    const [a] = areas();
    expect(a).toBeDefined();
    expect(a!.fill).not.toBe('');
  });

  it('채우기 없음은 영역을 만들지 않는다', () => {
    renderPanel({ ...range, y_thresholds: [{ value: 30, color: '#f00' }] });
    expect(areas()).toHaveLength(0);
  });

  it('여러 경계를 각각 채운다', () => {
    renderPanel({
      ...range,
      y_thresholds: [
        { value: 20, color: '#00f', fill_direction: 'below' },
        { value: 80, color: '#f00', fill_direction: 'above' },
      ],
    });
    expect(areas()).toEqual([
      { y1: 0, y2: 20, fill: '#00f' },
      { y1: 80, y2: 100, fill: '#f00' },
    ]);
  });
});

describe('자동 환산 단위 — 축·툴팁·범례가 한 값을 말한다', () => {
  // 보고된 결함: 축은 바이트를 1024 로 접어 `137`(KB) 로 그리는데 값 표기는 원값
  // `137355.20`(B) 을 그대로 찍어, 같은 점이 축과 범례에서 1000배 다르게 읽혔다.
  const BYTES = 137_355.2;

  function renderBytes(extra: Record<string, unknown> = {}) {
    setEntries([{ timestamp: NOW, value: BYTES }]);
    return renderPanel({ y_unit: 'auto:bytes', decimal_places: 2, ...extra });
  }

  it('툴팁 값도 축과 같은 배율로 접는다(접미사는 축 라벨이 한 번만 말한다)', () => {
    renderBytes();
    // 137355.2 / 1024 = 134.14…
    expect(screen.getByTestId('rc-tooltip').getAttribute('data-fmt-number-large')).toBe('134.14');
  });

  it('축 눈금도 같은 배율이다', () => {
    renderBytes();
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-tick-sample-large')).toBe('134.14');
  });

  it('축 라벨은 저장값이 아니라 실제로 접은 배율을 말한다', () => {
    // `auto:bytes` 는 접미사가 아니라 규칙이다. 그대로 붙이면 눈금은 KB 인데 라벨은
    // 규칙 이름을 말해, 축이 무엇을 세는지 알 수 없다.
    renderBytes();
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-label')).toBe('(KB)');
  });

  it('일반 단위는 종전대로 저장값을 그대로 라벨에 쓴다(회귀 0)', () => {
    setEntries([{ timestamp: NOW, value: 12.3456 }]);
    renderPanel({ y_unit: 'kW', y_label: '전력' });
    expect(screen.getByTestId('rc-yaxis').getAttribute('data-label')).toBe('전력 (kW)');
  });
});

describe('그림 상자 크기·자리', () => {
  function plotStyle(): string {
    return screen.getByTestId('line-chart-plot').getAttribute('style') ?? '';
  }

  it('기본값이면 transform 을 붙이지 않는다 — 저장된 대시보드의 그림이 변하지 않는다', () => {
    renderPanel({});
    expect(plotStyle()).not.toContain('transform');
  });

  it('크기를 줄이면 축소한다', () => {
    renderPanel({ plot_size: 80 });
    expect(plotStyle()).toContain('scale(0.8)');
  });

  it('옮긴 뒤 줄인다 — 오프셋이 크기에 휘둘리지 않게', () => {
    renderPanel({ plot_size: 50, plot_offset_x: 10, plot_offset_y: -5 });
    expect(plotStyle()).toContain('translate(10%, -5%) scale(0.5)');
  });

  it('범위 밖 크기는 미지정으로 본다 — 0 이하는 그림이 사라져 화면에서 되돌릴 수 없다', () => {
    renderPanel({ plot_size: 0 });
    expect(plotStyle()).not.toContain('transform');
  });

  it('저장된 오프셋은 ±상한으로 죈다', () => {
    renderPanel({ plot_offset_x: 999, plot_offset_y: -999 });
    expect(plotStyle()).toContain('translate(40%, -40%)');
  });

  it('끌 수 있는 대상 표식을 낸다', () => {
    renderPanel({});
    expect(screen.getByTestId('line-chart-plot').hasAttribute('data-chart-plot-area')).toBe(true);
  });
});
