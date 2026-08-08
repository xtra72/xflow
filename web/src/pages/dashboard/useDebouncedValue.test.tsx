// useDebouncedValue — 디바운스 훅 테스트.
// @spec SPEC-PANEL-SETTINGS-001 (T9, AC-13)

import { renderHook, act } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useDebouncedValue } from './useDebouncedValue';

afterEach(() => vi.useRealTimers());

describe('useDebouncedValue', () => {
  it('delay 경과 전에는 직전 값을, 경과 후에는 새 값을 반환한다', () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 200), {
      initialProps: { v: 'a' },
    });
    expect(result.current).toBe('a');

    rerender({ v: 'b' });
    // delay 경과 전.
    expect(result.current).toBe('a');
    act(() => vi.advanceTimersByTime(199));
    expect(result.current).toBe('a');
    // delay 경과 후.
    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe('b');
  });

  it('연속 변경 시 마지막 값으로만 안정된다(중간값 스킵)', () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 100), {
      initialProps: { v: 0 },
    });
    rerender({ v: 1 });
    act(() => vi.advanceTimersByTime(50));
    rerender({ v: 2 });
    act(() => vi.advanceTimersByTime(50)); // 1 의 타이머 취소됨
    expect(result.current).toBe(0);
    act(() => vi.advanceTimersByTime(50));
    expect(result.current).toBe(2);
  });
});
