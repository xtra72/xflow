// SPEC-WEB-006 v0.1.0 (M4) — 헤더 우측 업데이트 가능 배지.
//
// 항상 노출되는 32px 아이콘 버튼이며, 업데이트가 있을 때 작은 노란 점
// 인디케이터와 툴팁/aria-label 로 사용자에게 알린다. 비관리자 컨텍스트에서는
// disabled 로 흐려져 클릭이 차단된다.
//
// 시각 변형:
//   available=false           → 평이한 새로고침 아이콘 + "최신 버전입니다"
//   available=true            → 동일 아이콘 + 우상단 노란 점 + "새 버전 X 사용 가능"
//   disabled=true             → opacity-50 + "관리자 권한 필요" aria-label
//
// 클릭 시 onClick (보통 /admin/system 라우트로 이동)을 호출한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { RefreshCw } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

/** UpdateAvailableBadge 의 외부 인터페이스. */
export interface UpdateAvailableBadgeProps {
  /** true 면 노란 점 인디케이터와 "업데이트 가능" 라벨을 표시한다. */
  available: boolean;
  /** 새 버전 라벨. available=true 일 때만 의미가 있다. null 이면 "확인 필요" fallback. */
  latestVersion?: string | null;
  /** 클릭 핸들러 — 보통 /admin/system 으로 이동하거나 UpdateDialog 를 연다. */
  onClick: () => void;
  /** 비관리자/미인증 사용자를 위해 비활성화한다. */
  disabled?: boolean;
}

/**
 * 헤더 우측에 상시 표시되는 업데이트 가능 배지.
 * disabled 상태에서는 클릭이 차단되며 흐려진 모습으로 렌더된다.
 */
export function UpdateAvailableBadge({
  available,
  latestVersion,
  onClick,
  disabled = false,
}: UpdateAvailableBadgeProps): React.ReactElement {
  const { t } = useTranslation();
  // null/undefined 모두 fallback 문자열로 치환한다 (UI 일관성 + 메시지 길이 안정).
  const versionLabel = latestVersion ?? t('system.badge.versionFallback');

  // disabled 가 가장 우선 → 권한 안내 라벨 노출.
  const ariaLabel = disabled
    ? t('system.badge.adminRequired')
    : available
      ? t('system.badge.updateAvailableLabel').replace('{version}', versionLabel)
      : t('system.badge.statusLatest');

  // 툴팁 (브라우저 기본 title) — 호버 시 자세한 설명.
  const titleText = available
    ? t('system.badge.newVersionAvailable').replace('{version}', versionLabel)
    : t('system.badge.upToDate');

  return (
    <button
      type="button"
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      className={cn(
        'relative inline-flex h-8 w-8 items-center justify-center rounded-md',
        'text-(--color-text-muted) transition-colors',
        'hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
        disabled && 'opacity-50 cursor-not-allowed',
      )}
      aria-label={ariaLabel}
      title={titleText}
    >
      <RefreshCw className="h-4 w-4" aria-hidden="true" />
      {available && !disabled && (
        <span
          data-testid="update-badge-dot"
          aria-hidden="true"
          className={cn(
            'absolute right-1 top-1 inline-block h-2 w-2 rounded-full',
            'bg-yellow-400 ring-2 ring-(--color-bg-surface)',
          )}
        />
      )}
    </button>
  );
}
