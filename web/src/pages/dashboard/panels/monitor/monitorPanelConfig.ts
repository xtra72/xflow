// 모니터링 패널 공통 config 헬퍼.
//
// 네 패널(통계/메트릭/로그/이벤트)은 모두 `config.items` 하나로 "무엇을 보여줄지"를
// 정한다. 항목 어휘는 모니터링 페이지와 공유하므로(monitoringLayout) 검증도 그쪽
// 함수를 그대로 쓴다.

import {
  DEFAULT_LAYOUT,
  sanitizeSectionItems,
  type MonitorSectionKey,
} from '@/pages/monitoring/monitoringLayout';

/**
 * 패널 config 에서 표시 항목 목록을 읽는다.
 *
 * `items` 가 없으면(구버전 패널·직접 생성) 해당 섹션의 기본 항목을 쓴다. 빈 배열은
 * "사용자가 모두 껐다"는 정상 상태이므로 그대로 존중한다.
 */
export function readPanelItems(
  section: MonitorSectionKey,
  config: Record<string, unknown> | undefined,
): string[] {
  const sanitized = sanitizeSectionItems(section, config?.items);
  return sanitized ?? [...DEFAULT_LAYOUT[section]];
}

// --- 표시 옵션 (열 개수 / 갱신 주기 / 악센트 색) ---

/** 열 개수 상한의 절대 상한 — 이보다 많은 열은 어떤 폭에서도 읽히지 않는다. */
export const MAX_COLS_CEILING = 12;

/**
 * 고를 수 있는 열 개수 목록.
 *
 * 상한은 "표시 항목 수"다. 항목이 3개인데 6열을 고를 수 있으면 나머지 3칸은
 * 영원히 비어 있어 고를 이유가 없는 선택지가 된다.
 */
export function maxColsOptions(itemCount: number): number[] {
  const top = Math.min(Math.max(itemCount, 1), MAX_COLS_CEILING);
  return Array.from({ length: top }, (_, i) => i + 1);
}

/** 갱신 주기 하한 (ms) — 이보다 짧으면 폴링이 스스로를 앞지른다. */
export const MIN_REFRESH_MS = 1_000;
/** 갱신 주기 상한 (ms) — 24시간. */
export const MAX_REFRESH_MS = 24 * 60 * 60 * 1_000;

/** ms → 시/분/초 분해 */
export function splitDuration(ms: number): { hours: number; minutes: number; seconds: number } {
  const total = Math.max(0, Math.floor(ms / 1_000));
  return {
    hours: Math.floor(total / 3_600),
    minutes: Math.floor((total % 3_600) / 60),
    seconds: total % 60,
  };
}

/** 시/분/초 → ms (허용 범위로 죈다) */
export function joinDuration(hours: number, minutes: number, seconds: number): number {
  const safe = (n: number) => (Number.isFinite(n) && n > 0 ? Math.floor(n) : 0);
  const ms = (safe(hours) * 3_600 + safe(minutes) * 60 + safe(seconds)) * 1_000;
  return Math.min(Math.max(ms, MIN_REFRESH_MS), MAX_REFRESH_MS);
}

/**
 * 열 개수 상한을 읽는다.
 *
 * "상한"이지 고정값이 아니다 — 패널이 좁으면 실제 열 수는 이보다 줄어든다
 * (useAutoColumns). 범위를 벗어난 값은 기본값으로 되돌린다.
 */
export function readMaxCols(
  config: Record<string, unknown> | undefined,
  fallback: number,
  itemCount?: number,
): number {
  const raw = config?.maxCols;
  const base =
    typeof raw === 'number' && Number.isFinite(raw) && raw >= 1 && raw <= MAX_COLS_CEILING
      ? Math.round(raw)
      : fallback;
  // 항목 수를 알면 그보다 많은 열은 무의미하다 — 저장값이 남아 있어도 죈다.
  if (typeof itemCount === 'number' && itemCount > 0) return Math.min(base, itemCount);
  return base;
}

/** 갱신 주기(ms)를 읽는다. 범위를 벗어나면 기본값. */
export function readRefreshMs(
  config: Record<string, unknown> | undefined,
  fallback: number,
): number {
  const raw = config?.refreshMs;
  if (typeof raw !== 'number' || !Number.isFinite(raw)) return fallback;
  // 0 이하나 과도하게 긴 값은 사실상 "갱신 안 함"이라 사고를 유발한다.
  return raw >= MIN_REFRESH_MS && raw <= MAX_REFRESH_MS ? Math.round(raw) : fallback;
}

/**
 * 패널 악센트 색 해석기.
 *
 * 대시보드 공통 규약을 그대로 따른다(ResourceWidget 과 동일):
 *   - `config.panelColor`: 패널 전체 기본 색
 *   - `config.accentElements[group]`: 그룹별 개별 색. `false` 면 그 그룹만 색 해제.
 */
export function readAccent(config: Record<string, unknown> | undefined): {
  panelColor: string | undefined;
  accentColor: (group: string) => string | undefined;
} {
  const panelColor = config?.panelColor as string | undefined;
  const elements = (config?.accentElements as Record<string, string | boolean>) ?? {};

  return {
    panelColor,
    accentColor: (group: string) => {
      if (elements[group] === false) return undefined;
      const value = elements[group];
      if (typeof value === 'string' && value) return value;
      return panelColor;
    },
  };
}

/**
 * 네트워크 패널이 그릴 인터페이스 이름 목록을 읽는다.
 *
 * 빈 배열은 "전체 합산만"이며 기본값이다. 선택한 인터페이스가 사라져도 설정은
 * 남는데, 그 경우 패널이 안내를 띄운다(useNetworkSeries.missing).
 */
export function readInterfaces(config: Record<string, unknown> | undefined): string[] {
  const raw = config?.interfaces;
  if (!Array.isArray(raw)) return [];
  return raw.filter((v): v is string => typeof v === 'string' && v !== '');
}

/** 표시 구간 옵션 (초) */
export const WINDOW_SEC_OPTIONS = [60, 300, 600, 1_800, 3_600] as const;

/**
 * 표시 구간(초)을 읽는다.
 *
 * 차트에 보일 최근 구간이다. 버퍼는 더 길게 갖고 있으므로, 구간을 늘리면 이미
 * 쌓인 데이터가 그대로 드러난다.
 */
export function readWindowSec(
  config: Record<string, unknown> | undefined,
  fallback: number,
): number {
  const raw = config?.windowSec;
  if (typeof raw !== 'number' || !Number.isFinite(raw)) return fallback;
  return raw >= 10 && raw <= 86_400 ? Math.round(raw) : fallback;
}
