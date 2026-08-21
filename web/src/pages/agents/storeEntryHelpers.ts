// Store 엔트리 표시 헬퍼(순수 함수, 공용).
//
// @spec SPEC-PANEL-SETTINGS-001 (T5)
//
// `AgentDetailPanel` 와 공용 `StoreEntryTable` 이 함께 쓰는 순수 표시 헬퍼를 별도
// 모듈로 분리한다(컴포넌트 파일과 분리 — react-refresh fast-refresh 규약 준수).

import type { TranslationFn } from '@/lib/i18n';

// 모듈 스코프에서는 t()를 호출할 수 없으므로 번역 함수를 인자로 받는다.
export function formatTimeAgo(date: Date, t: TranslationFn): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return t('agents.detail.time.secondsAgo').replace('{n}', String(seconds));
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return t('agents.detail.time.minutesAgo').replace('{n}', String(minutes));
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('agents.detail.time.hoursAgo').replace('{n}', String(hours));
  const days = Math.floor(hours / 24);
  return t('agents.detail.time.daysAgo').replace('{n}', String(days));
}

/**
 * 엔트리의 `tags` 필드에서 태그 맵을 추출한다.
 * 정적 키가 아닌 동적 키 엔트리는 `tags` 를 가지지 않아 null 을 반환한다.
 *
 * @spec SPEC-STORE-003
 */
export function extractEntryTags(
  entry: Record<string, unknown>,
): Record<string, string> | null {
  const raw = entry.tags;
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (typeof v === 'string') out[k] = v;
  }
  return Object.keys(out).length > 0 ? out : null;
}

/**
 * 엔트리의 `field` 필드를 추출한다.
 * 백엔드는 모든 엔트리에 field 을 포함하며(동적 키는 "unknown"),
 * 누락/비문자열인 경우 빈 문자열을 반환해 호출자가 "unknown" 으로 표시하도록 한다.
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export function extractEntryMetricType(entry: Record<string, unknown>): string {
  const raw = entry.field;
  return typeof raw === 'string' ? raw : '';
}
