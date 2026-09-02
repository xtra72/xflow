// 통계 패널의 현재값 크기 배율.
//
// 종전에는 글자 크기가 Tailwind 클래스로 박혀 있었다(본값 `text-4xl` · 타일 `text-2xl`).
// 클래스로는 배율을 걸 수 없으므로 수치로 꺼내고 배율을 곱한다.
//
// 값과 단위가 **함께** 커져야 한다 — 값만 키우면 단위가 상대적으로 작아져 두 글자의
// 균형이 배율마다 달라진다.

import { describe, expect, it, vi } from 'vitest';
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

// 채널이 빠진 뒤 데이터 이음매는 하나다 — 채널 형상을 시리즈 소스 결과로 옮겨 준다.
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
vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: [] } }) }));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('../../panelChromeContext', () => ({
  usePanelTitleVisible: () => true,
  usePanelTitleStyle: () => undefined,
}));

import StatPanel from './StatPanel';
import { readValueScale, VALUE_SCALE_MAX, VALUE_SCALE_MIN } from './valueScale';

/** 본값과 단위의 글자 크기(px). */
function sizes(config: Record<string, unknown>): { value: number; unit: number } {
  mockResult.current.entries = [{ timestamp: 1, value: 42 }];
  const view = render(<StatPanel panelId="p" config={{ channel_name: 'c', unit: '%', ...config }} />);
  const el = screen.getByTestId('stat-value');
  const unitEl = el.querySelector('span')!;
  const out = {
    value: parseFloat(el.style.fontSize),
    unit: parseFloat(unitEl.style.fontSize),
  };
  view.unmount();
  return out;
}

describe('배율 읽기', () => {
  it('미지정이면 1', () => {
    expect(readValueScale(undefined)).toBe(1);
  });

  it('범위를 벗어나면 죈다 — 손으로 편집한 config 가 글자를 날리지 않는다', () => {
    expect(readValueScale(99)).toBe(VALUE_SCALE_MAX);
    expect(readValueScale(0)).toBe(VALUE_SCALE_MIN);
  });

  it('수가 아니면 기본값', () => {
    expect(readValueScale('2')).toBe(1);
    expect(readValueScale(Number.NaN)).toBe(1);
  });
});

describe('통계 본값에 배율이 걸린다', () => {
  it('미지정이면 종전 크기다 — 기존 패널이 달라지지 않는다', () => {
    // `text-4xl`(36) · `text-xl`(20) 를 수치로 옮긴 값이다.
    expect(sizes({})).toEqual({ value: 36, unit: 20 });
  });

  it('배율만큼 커진다', () => {
    expect(sizes({ value_scale: 2 })).toEqual({ value: 72, unit: 40 });
  });

  it('배율만큼 작아진다', () => {
    expect(sizes({ value_scale: 0.5 })).toEqual({ value: 18, unit: 10 });
  });

  it('값과 단위의 비율이 배율과 무관하게 유지된다', () => {
    const base = sizes({});
    for (const scale of [0.5, 1.5, 3]) {
      const s = sizes({ value_scale: scale });
      expect(s.value / s.unit).toBeCloseTo(base.value / base.unit);
    }
  });

  it('범위를 벗어난 설정도 죄여서 그려진다', () => {
    expect(sizes({ value_scale: 99 }).value).toBe(36 * VALUE_SCALE_MAX);
  });
});
