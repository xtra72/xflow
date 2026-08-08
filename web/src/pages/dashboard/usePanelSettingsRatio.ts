// 패널 설정 3분할 셸의 2경계 비율(옵션 컬럼 폭 + 미리보기 높이 비율)을 패널별
// localStorage 로 영속/복원하는 훅.
//
// @spec SPEC-PANEL-SETTINGS-001 (T2/T3, REQ-02/REQ-03, AC-02/AC-03)
//
// - optionsWidth: 우측 옵션 컬럼 폭(px). 좌우(세로) 경계 위치. [OPTIONS_MIN, OPTIONS_MAX] 클램프.
// - previewRatio: 좌측 컬럼 내 미리보기(상단) 높이 비율(0..1). 상하(가로) 경계 위치.
//   [PREVIEW_MIN, PREVIEW_MAX] 클램프.
//
// 손상/부재 값은 기본 비율로 폴백하며 예외를 던지지 않는다(AC-03 edge).

import { useCallback, useEffect, useState } from 'react';

export interface PanelSettingsRatio {
  /** 우측 옵션 컬럼 폭(px). */
  optionsWidth: number;
  /** 좌측 컬럼 내 미리보기 높이 비율(0..1). */
  previewRatio: number;
}

export const OPTIONS_WIDTH_MIN = 240;
export const OPTIONS_WIDTH_MAX = 800;
export const PREVIEW_RATIO_MIN = 0.15;
export const PREVIEW_RATIO_MAX = 0.85;

/** 기본 비율(옵션 360px, 미리보기 50%). */
export function defaultPanelSettingsRatio(): PanelSettingsRatio {
  return { optionsWidth: 360, previewRatio: 0.5 };
}

const PREFIX = 'panel-settings-ratio:';

/** 패널별 localStorage 키. */
export function panelSettingsRatioKey(panelId: string): string {
  return `${PREFIX}${panelId}`;
}

function clampNumber(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

/**
 * 비율 값을 정규화한다. 각 필드가 유한 숫자가 아니면 기본값으로 대체하고,
 * 유효 범위로 클램프한다(경계 반올림/손상 방어 — R4).
 */
export function clampRatio(r: Partial<PanelSettingsRatio> | undefined): PanelSettingsRatio {
  const d = defaultPanelSettingsRatio();
  const ow = typeof r?.optionsWidth === 'number' && Number.isFinite(r.optionsWidth)
    ? clampNumber(r.optionsWidth, OPTIONS_WIDTH_MIN, OPTIONS_WIDTH_MAX)
    : d.optionsWidth;
  const pr = typeof r?.previewRatio === 'number' && Number.isFinite(r.previewRatio)
    ? clampNumber(r.previewRatio, PREVIEW_RATIO_MIN, PREVIEW_RATIO_MAX)
    : d.previewRatio;
  return { optionsWidth: ow, previewRatio: pr };
}

/** 패널별 비율을 localStorage 에서 로드한다(부재/손상 시 기본값, throw 없음). */
export function loadPanelSettingsRatio(panelId: string): PanelSettingsRatio {
  if (typeof window === 'undefined' || !window.localStorage) {
    return defaultPanelSettingsRatio();
  }
  let raw: string | null = null;
  try {
    raw = window.localStorage.getItem(panelSettingsRatioKey(panelId));
  } catch {
    return defaultPanelSettingsRatio();
  }
  if (raw === null) return defaultPanelSettingsRatio();
  try {
    const parsed = JSON.parse(raw) as Partial<PanelSettingsRatio>;
    return clampRatio(parsed);
  } catch {
    return defaultPanelSettingsRatio();
  }
}

/** 패널별 비율을 localStorage 에 저장한다(실패는 무시). */
export function savePanelSettingsRatio(panelId: string, ratio: PanelSettingsRatio): void {
  if (typeof window === 'undefined' || !window.localStorage) return;
  try {
    window.localStorage.setItem(panelSettingsRatioKey(panelId), JSON.stringify(ratio));
  } catch {
    // 저장 실패는 무시 — 세션 내 상태로만 동작.
  }
}

/**
 * 패널별 비율 상태를 관리한다. 마운트/패널 전환 시 로드하고, 변경 시 저장한다.
 * `setRatio` 는 부분 갱신을 받아 클램프 후 상태/영속에 반영한다(드래그 중 라이브 조정).
 */
export function usePanelSettingsRatio(panelId: string): {
  ratio: PanelSettingsRatio;
  setRatio: (patch: Partial<PanelSettingsRatio>) => void;
  resetToDefault: () => void;
} {
  const [ratio, setRatioState] = useState<PanelSettingsRatio>(() =>
    loadPanelSettingsRatio(panelId),
  );

  useEffect(() => {
    setRatioState(loadPanelSettingsRatio(panelId));
  }, [panelId]);

  useEffect(() => {
    savePanelSettingsRatio(panelId, ratio);
  }, [panelId, ratio]);

  const setRatio = useCallback((patch: Partial<PanelSettingsRatio>) => {
    setRatioState((prev) => clampRatio({ ...prev, ...patch }));
  }, []);

  const resetToDefault = useCallback(() => {
    setRatioState(defaultPanelSettingsRatio());
  }, []);

  return { ratio, setRatio, resetToDefault };
}
