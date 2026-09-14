// 최근 사용 색 — 모든 고르개가 공유하는 MRU 12칸.
//
// `localStorage` 에 두되 **실패해도 동작한다**. 사파리 사생활 보호 모드처럼
// `setItem` 이 던지는 환경이 실재하므로, 던지는 것을 잡아 메모리 목록으로 떨어진다.
// 같은 세션 안에서는 최근색이 계속 동작하고 세션을 넘길 때만 잃는다.
//
// 저장 키는 `xflow-ui` 같은 공용 키에 얹지 않고 제 키를 쓴다 —
// `PALETTE_STORAGE_KEY`(`canvas/shapes/paletteGroups.ts`)가 세운 선례 그대로다.
//
// @spec SPEC-COLOR-001 §결정 9 (M4)

import { normalizeColor } from './colorFormat';

/** 기기 지역 저장 키. */
export const RECENT_COLORS_STORAGE_KEY = 'xflow-recent-colors';

/** 기억하는 칸 수. */
export const RECENT_COLORS_LIMIT = 12;

/**
 * `localStorage` 가 막힌 환경의 폴백.
 *
 * 모듈 수준 변수인 것이 의도다 — 고르개가 40자리에 흩어져 있고 그것들이 **같은
 * 목록**을 봐야 하므로, 컴포넌트 상태로 들면 자리마다 따로 놀게 된다.
 */
let memoryFallback: string[] = [];

/** 저장소가 막혔는지. 한 번 막히면 계속 폴백을 쓴다. */
let storageBlocked = false;

/** 시험이 모듈 상태를 비울 수 있게 연다 — 시험 간 오염을 막는 유일한 길이다. */
export function resetRecentColorsForTest(): void {
  memoryFallback = [];
  storageBlocked = false;
}

/**
 * 저장된 목록을 관용적으로 읽는다.
 *
 * 저장소 내용은 사용자가 손으로 고칠 수 있으므로 **모르는 값은 버린다** — 배열이
 * 아니면 빈 목록, 정규화를 통과하지 못하는 항목은 빼고, 길이는 잘라 낸다. 001 파서와
 * 같은 규율이다.
 */
export function readRecentColors(): string[] {
  if (storageBlocked) return [...memoryFallback];
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(RECENT_COLORS_STORAGE_KEY);
  } catch {
    storageBlocked = true;
    return [...memoryFallback];
  }
  if (raw === null) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  const out: string[] = [];
  for (const item of parsed) {
    if (typeof item !== 'string') continue;
    const norm = normalizeColor(item, { alpha: true });
    if (norm === null || out.includes(norm)) continue;
    out.push(norm);
    if (out.length === RECENT_COLORS_LIMIT) break;
  }
  return out;
}

/**
 * 색 하나를 목록 맨 앞으로 올린다. 이미 있으면 **승격**이지 추가가 아니다.
 *
 * `undefined`(비우기)는 목록을 바꾸지 않는다 — 비우기는 "이 색을 썼다" 가 아니다.
 * 정규화를 통과하지 못하는 값도 마찬가지로 무시한다.
 */
export function pushRecentColor(color: string | undefined): string[] {
  if (color === undefined) return readRecentColors();
  const norm = normalizeColor(color, { alpha: true });
  if (norm === null) return readRecentColors();

  const next = [norm, ...readRecentColors().filter((c) => c !== norm)].slice(
    0,
    RECENT_COLORS_LIMIT,
  );

  memoryFallback = next;
  if (storageBlocked) return [...next];
  try {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify(next));
  } catch {
    // 던지는 환경이면 이 세션은 메모리로 간다. 예외를 밖으로 내지 않는다 —
    // 색을 고른 사용자에게 저장소 사정은 알 바가 아니다.
    storageBlocked = true;
  }
  return [...next];
}

/**
 * 고르개가 실제로 보여 줄 목록.
 *
 * `alpha` 가 꺼진 자리에서는 8자리 항목을 **거른다.** 보여 주고 눌렀을 때 거절하면
 * 그 자리가 왜 안 되는지 알 길이 없고, 8자리를 6자리로 잘라 보여 주면 같은 칸이
 * 자리마다 다른 색이 된다.
 */
export function visibleRecentColors(opts: { alpha: boolean }): string[] {
  const all = readRecentColors();
  return opts.alpha ? all : all.filter((c) => c.length === 7);
}
