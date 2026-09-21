// 유휴 로그아웃 감시와 경고 화면 (@SPEC:SPEC-AUTH-IDLE-001).
//
// AppLayout 에 한 번 마운트된다 — 인증을 통과한 뒤에만 렌더되는 자리이므로
// 로그인 화면에서는 애초에 돌지 않는다. 로그아웃이 일어나면 AuthGuard 가
// `/login` 으로 보내고 이 컴포넌트도 함께 언마운트된다.

import { useEffect, useRef } from 'react';
import { AlertTriangle } from 'lucide-react';

import { useIdleLogout } from '@/hooks/useIdleLogout';
import { formatRemaining } from '@/lib/idle/idlePolicy';
import { useTranslation } from '@/lib/i18n';

/**
 * 유휴 경고 모달.
 *
 * 확인 다이얼로그들과 달리 배경 클릭·Esc 로 닫지 않는다 — 닫는 행위가 곧 '계속
 * 사용' 인지 '그냥 로그아웃' 인지 알 수 없고, 어느 쪽으로 읽어도 사용자가 의도하지
 * 않은 쪽이 될 수 있다. 버튼 하나로만 연장한다. 다만 화면 어디를 누르든 그것은
 * 활동이므로, 실제로는 모달 밖을 눌러도 타이머가 연장된다(훅의 활동 리스너).
 */
export default function IdleLogoutGuard() {
  const { t } = useTranslation();
  const { phase, remainingMs, setting, extend } = useIdleLogout();
  const buttonRef = useRef<HTMLButtonElement>(null);

  // 경고가 뜨면 연장 버튼으로 초점을 옮긴다 — 키보드만 쓰는 사용자가 Tab 을
  // 헤매지 않고 Enter 로 연장할 수 있다.
  useEffect(() => {
    if (phase === 'warning') buttonRef.current?.focus();
  }, [phase]);

  if (phase !== 'warning') return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="idle-logout-title"
      data-testid="idle-logout-warning"
    >
      <div className="w-full max-w-md rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 h-6 w-6 shrink-0 text-amber-500" aria-hidden="true" />
          <div className="min-w-0">
            <h2
              id="idle-logout-title"
              className="text-lg font-semibold text-(--color-text-primary)"
            >
              {t('auth.idleLogout.warningTitle')}
            </h2>
            <p className="mt-2 text-sm text-(--color-text-muted)">
              {t('auth.idleLogout.warningBody')
                .replace('{minutes}', String(setting.timeoutMinutes))
                .replace('{remaining}', formatRemaining(remainingMs))}
            </p>
          </div>
        </div>

        <div className="mt-6 flex justify-end">
          <button
            ref={buttonRef}
            type="button"
            onClick={extend}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700"
          >
            {t('auth.idleLogout.continue')}
          </button>
        </div>
      </div>
    </div>
  );
}
