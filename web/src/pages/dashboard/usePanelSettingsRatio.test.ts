// usePanelSettingsRatio — 2경계 비율 영속/복원 테스트.
// @spec SPEC-PANEL-SETTINGS-001 (T2/T3, AC-02/AC-03)

import { renderHook, act } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';

import {
  clampRatio,
  defaultPanelSettingsRatio,
  loadPanelSettingsRatio,
  OPTIONS_WIDTH_MAX,
  OPTIONS_WIDTH_MIN,
  panelSettingsRatioKey,
  PREVIEW_RATIO_MAX,
  PREVIEW_RATIO_MIN,
  savePanelSettingsRatio,
  usePanelSettingsRatio,
} from './usePanelSettingsRatio';

beforeEach(() => window.localStorage.clear());

describe('clampRatio', () => {
  it('유효 범위로 클램프한다', () => {
    expect(clampRatio({ optionsWidth: 10_000, previewRatio: 2 })).toEqual({
      optionsWidth: OPTIONS_WIDTH_MAX,
      previewRatio: PREVIEW_RATIO_MAX,
    });
    expect(clampRatio({ optionsWidth: 0, previewRatio: 0 })).toEqual({
      optionsWidth: OPTIONS_WIDTH_MIN,
      previewRatio: PREVIEW_RATIO_MIN,
    });
  });
  it('비유한/부재 값은 기본으로 대체한다', () => {
    expect(clampRatio({ optionsWidth: NaN, previewRatio: undefined })).toEqual(
      defaultPanelSettingsRatio(),
    );
    expect(clampRatio(undefined)).toEqual(defaultPanelSettingsRatio());
  });
});

describe('load/save', () => {
  it('키 포맷', () => {
    expect(panelSettingsRatioKey('p1')).toBe('panel-settings-ratio:p1');
  });
  it('부재/손상 시 기본값(throw 없음) — AC-03 edge', () => {
    expect(loadPanelSettingsRatio('none')).toEqual(defaultPanelSettingsRatio());
    window.localStorage.setItem(panelSettingsRatioKey('p'), '{bad');
    expect(loadPanelSettingsRatio('p')).toEqual(defaultPanelSettingsRatio());
  });
  it('save→load 왕복(클램프 적용)', () => {
    savePanelSettingsRatio('p1', { optionsWidth: 500, previewRatio: 0.3 });
    expect(loadPanelSettingsRatio('p1')).toEqual({ optionsWidth: 500, previewRatio: 0.3 });
  });
});

describe('usePanelSettingsRatio 훅', () => {
  it('기본값 로드 + setRatio 로 부분 갱신·영속(클램프)', () => {
    const { result } = renderHook(() => usePanelSettingsRatio('p1'));
    expect(result.current.ratio).toEqual(defaultPanelSettingsRatio());

    act(() => result.current.setRatio({ optionsWidth: 500 }));
    expect(result.current.ratio.optionsWidth).toBe(500);
    // 상한 초과는 클램프.
    act(() => result.current.setRatio({ optionsWidth: 9999 }));
    expect(result.current.ratio.optionsWidth).toBe(OPTIONS_WIDTH_MAX);

    // localStorage 영속 확인.
    expect(loadPanelSettingsRatio('p1').optionsWidth).toBe(OPTIONS_WIDTH_MAX);
  });

  it('resetToDefault 로 기본 복원', () => {
    const { result } = renderHook(() => usePanelSettingsRatio('p1'));
    act(() => result.current.setRatio({ previewRatio: 0.2 }));
    expect(result.current.ratio.previewRatio).toBe(0.2);
    act(() => result.current.resetToDefault());
    expect(result.current.ratio).toEqual(defaultPanelSettingsRatio());
  });

  it('panelId 변경 시 해당 패널의 저장값을 재로드한다', () => {
    savePanelSettingsRatio('p2', { optionsWidth: 700, previewRatio: 0.7 });
    const { result, rerender } = renderHook(({ id }) => usePanelSettingsRatio(id), {
      initialProps: { id: 'p1' },
    });
    expect(result.current.ratio).toEqual(defaultPanelSettingsRatio());
    rerender({ id: 'p2' });
    expect(result.current.ratio).toEqual({ optionsWidth: 700, previewRatio: 0.7 });
  });
});
