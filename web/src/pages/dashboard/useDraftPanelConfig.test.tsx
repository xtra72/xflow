// useDraftPanelConfig — draft/committed 분리 테스트.
// @spec SPEC-PANEL-SETTINGS-001 (T9, AC-14)

import { renderHook, act } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useDraftPanelConfig } from './useDraftPanelConfig';

describe('useDraftPanelConfig', () => {
  it('초기 draft 는 committed 와 같고 isDirty=false', () => {
    const { result } = renderHook(() =>
      useDraftPanelConfig({ a: 1 }, 'title', 'p1'),
    );
    expect(result.current.draftConfig).toEqual({ a: 1 });
    expect(result.current.draftTitle).toBe('title');
    expect(result.current.isDirty).toBe(false);
  });

  it('patchConfig 는 draft 만 갱신하고 isDirty=true (committed 불변)', () => {
    const committed = { a: 1 };
    const { result } = renderHook(() =>
      useDraftPanelConfig(committed, 'title', 'p1'),
    );
    act(() => result.current.patchConfig({ b: 2 }));
    expect(result.current.draftConfig).toEqual({ a: 1, b: 2 });
    expect(result.current.isDirty).toBe(true);
    // committed 원본 불변(누수 없음).
    expect(committed).toEqual({ a: 1 });
  });

  it('cancel 은 draft 를 committed 로 복원한다', () => {
    const { result } = renderHook(() =>
      useDraftPanelConfig({ a: 1 }, 'title', 'p1'),
    );
    act(() => {
      result.current.patchConfig({ b: 2 });
      result.current.setTitle('changed');
    });
    expect(result.current.isDirty).toBe(true);
    act(() => result.current.cancel());
    expect(result.current.draftConfig).toEqual({ a: 1 });
    expect(result.current.draftTitle).toBe('title');
    expect(result.current.isDirty).toBe(false);
  });

  it('panelId 변경 시 draft 를 새 committed 로 재초기화한다', () => {
    const { result, rerender } = renderHook(
      ({ cfg, title, id }) => useDraftPanelConfig(cfg, title, id),
      { initialProps: { cfg: { a: 1 } as Record<string, unknown>, title: 't1', id: 'p1' } },
    );
    act(() => result.current.patchConfig({ b: 2 }));
    expect(result.current.draftConfig).toEqual({ a: 1, b: 2 });
    // 같은 패널에서 committed 가 바뀌어도 draft 는 유지(편집 보존).
    rerender({ cfg: { a: 9 }, title: 't1', id: 'p1' });
    expect(result.current.draftConfig).toEqual({ a: 1, b: 2 });
    // 패널 전환 시에만 재초기화.
    rerender({ cfg: { c: 3 }, title: 't2', id: 'p2' });
    expect(result.current.draftConfig).toEqual({ c: 3 });
    expect(result.current.draftTitle).toBe('t2');
  });
});
