// 원격 관리 비-server 모드 안내 (SPEC-REMOTE-001).
//
// 인스턴스가 원격 관리 server 모드가 아닐 때(client/disabled) admin `/remote/*`
// 페이지에 표시한다. server 전용 노드/자원 쿼리는 발행되지 않으므로 404 노이즈가
// 발생하지 않으며, 사용자에게 설정 방법을 안내한다.

import { Info } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

/**
 * server 모드가 아닐 때 표시하는 안내 패널.
 * 스크린리더를 위해 status region(role="region" + aria-label)으로 노출한다.
 */
export function RemoteNotServerNotice(): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <div
      role="region"
      aria-label={t('remote.notServerTitle')}
      data-testid="remote-not-server"
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-12 px-6 text-center"
    >
      <Info
        className="mx-auto h-12 w-12 text-(--color-text-muted)"
        aria-hidden="true"
      />
      <h2 className="mt-4 text-base font-semibold text-(--color-text-primary)">
        {t('remote.notServerTitle')}
      </h2>
      <p className="mx-auto mt-2 max-w-prose text-sm text-(--color-text-muted)">
        {t('remote.notServerDesc')}
      </p>
    </div>
  );
}
