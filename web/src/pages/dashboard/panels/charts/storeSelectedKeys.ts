// 패널 설정 공용 Store 리스트의 행 선택(체크박스) 결과를 StoreSourceConfig.selected_keys
// 로 반영하기 위한 순수 헬퍼.
//
// @spec SPEC-PANEL-SETTINGS-001 (T6)
//
// additive-only: selected_keys 미설정/손상 시 빈 선택([])으로 해석하여 기존 동작을 보존한다.

import type { StoreSourceConfig } from './chartChannelTypes';

/**
 * 선택 계열(체크박스) 상한. 라이브 미리보기 성능 보호를 위한 합리적 상한(수십 개).
 * @spec SPEC-PANEL-SETTINGS-001 (T9, AC-15)
 */
export const SELECTED_KEYS_LIMIT = 48;

/** 현재 선택이 상한에 도달했는지(추가 선택 억제 판정용). 제거 방향은 항상 허용. */
export function isSelectionAtLimit(current: readonly string[]): boolean {
  return current.length >= SELECTED_KEYS_LIMIT;
}

/**
 * store_source.selected_keys 를 방어적으로 파싱한다.
 * 미설정(undefined)/비배열/비문자열 요소는 무시하고, 중복은 제거한다.
 * 반환 순서는 입력 순서를 보존한다(안정적).
 */
export function readSelectedKeys(
  store: StoreSourceConfig | undefined,
): string[] {
  const raw = store?.selected_keys;
  if (!Array.isArray(raw)) return [];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const v of raw) {
    if (typeof v === 'string' && v !== '' && !seen.has(v)) {
      seen.add(v);
      out.push(v);
    }
  }
  return out;
}

/** 특정 키의 선택 여부. */
export function isKeySelected(
  store: StoreSourceConfig | undefined,
  key: string,
): boolean {
  return readSelectedKeys(store).includes(key);
}

/**
 * 선택 집합에서 key 를 토글한 새 배열을 반환한다(불변). 이미 있으면 제거, 없으면 추가.
 * 입력 순서를 보존하며 중복을 만들지 않는다.
 */
export function toggleSelectedKey(current: readonly string[], key: string): string[] {
  if (current.includes(key)) {
    return current.filter((k) => k !== key);
  }
  return [...current, key];
}
