// 디바이스 목록 셀의 순수 표시 헬퍼 (배지 색 / 등록 출처 / 상대 시간).
//
// 디바이스 탭(DeviceListPage)과 대시보드 디바이스 패널(DevicePanel)이 같은 표시
// 규칙을 쓰도록 이 모듈 하나만 공유한다. 컴포넌트를 export 하지 않는 순수 모듈이라
// DeviceCell.tsx(컴포넌트 전용)와 분리한다.

import type { TranslationFn } from '@/lib/i18n';

// 디바이스 source 값을 사용자 친화적 라벨로 매핑.
// 수동(manual)=config|pinned, 자동(auto)=auto|bridge.
// `labelKey`가 있으면 i18n 키(`devices.source.*`)이고, 없으면 원본 source 문자열을
// 그대로 표시한다(미지정 종류). 렌더 시 t()로 변환한다(컴포넌트 밖 t() 호출 금지).
export function sourceVariant(
  source: string,
): { labelKey?: string; rawLabel?: string; manual: boolean } | null {
  switch (source) {
    case 'config':
      return { labelKey: 'devices.source.config', manual: true };
    case 'pinned':
      return { labelKey: 'devices.source.pinned', manual: true };
    case 'auto':
      return { labelKey: 'devices.source.auto', manual: false };
    case 'bridge':
      return { labelKey: 'devices.source.bridge', manual: false };
    default:
      return source ? { rawLabel: source, manual: false } : null;
  }
}

/** 프로토콜 배지 색상 */
export const PROTOCOL_COLORS: Record<string, string> = {
  samsung_nasa: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  lgap: 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400',
  modbus: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
};

/** 상대 시간 포맷 (예: "3분 전"). t()를 인자로 받아 컴포넌트 밖 호출을 피한다. */
export function formatRelativeTime(dateStr: string, t: TranslationFn): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const then = date.getTime();
  if (isNaN(then)) return '-';
  if (date.getUTCFullYear() < 2000) return '-';

  const now = Date.now();
  const diffMs = now - then;
  if (diffMs < 0) return t('devices.relativeTime.justNow');

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return t('devices.relativeTime.secondsAgo').replace('{n}', String(seconds));

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return t('devices.relativeTime.minutesAgo').replace('{n}', String(minutes));

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('devices.relativeTime.hoursAgo').replace('{n}', String(hours));

  const days = Math.floor(hours / 24);
  return t('devices.relativeTime.daysAgo').replace('{n}', String(days));
}
