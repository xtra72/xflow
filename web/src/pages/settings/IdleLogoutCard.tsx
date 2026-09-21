// 유휴 로그아웃 설정 카드 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 서버 전역 1벌 설정이므로 여기서 바꾸면 모든 사용자에게 적용된다. 읽기는 빌트인
// 세 역할 모두 가능하고, 저장은 system.update 권한이 있어야 한다(없으면 비활성).
// ScheduleLogStorageCard 와 같은 카드/쿼리/알림 패턴을 따른다.

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Loader2, Save } from 'lucide-react';

import {
  fetchIdleLogoutSetting,
  saveIdleLogoutSetting,
} from '@/services/api/idleLogoutService';
import {
  DEFAULT_IDLE_SETTING,
  MAX_TIMEOUT_MINUTES,
  MIN_TIMEOUT_MINUTES,
  clampTimeoutMinutes,
} from '@/lib/idle/idlePolicy';
import { IDLE_LOGOUT_QUERY_KEY } from '@/hooks/useIdleLogout';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

const inputClass = cn(
  'block w-32 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) shadow-sm',
  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
  'disabled:cursor-not-allowed disabled:opacity-50',
);

/**
 * 유휴 로그아웃 설정 카드.
 *
 * @param isReadOnly - system.update 미보유 시 컨트롤을 비활성화한다.
 */
export function IdleLogoutCard({ isReadOnly }: { isReadOnly: boolean }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const qc = useQueryClient();

  const { data: current, isLoading } = useQuery({
    queryKey: IDLE_LOGOUT_QUERY_KEY,
    queryFn: fetchIdleLogoutSetting,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const [enabled, setEnabled] = useState(DEFAULT_IDLE_SETTING.enabled);
  const [minutes, setMinutes] = useState(String(DEFAULT_IDLE_SETTING.timeoutMinutes));

  // 서버 값이 도착하면 폼을 맞춘다(사용자가 편집 중인 값은 덮지 않도록 값 변경 시에만).
  useEffect(() => {
    if (!current) return;
    setEnabled(current.enabled);
    setMinutes(String(current.timeoutMinutes));
  }, [current]);

  const mutation = useMutation({
    mutationFn: () =>
      saveIdleLogoutSetting({ enabled, timeoutMinutes: Number(minutes) }),
    onSuccess: (saved) => {
      qc.setQueryData(IDLE_LOGOUT_QUERY_KEY, saved);
      setMinutes(String(saved.timeoutMinutes));
      addNotification({ type: 'success', message: t('settings.idleLogout.saved') });
    },
    onError: () => {
      addNotification({ type: 'error', message: t('settings.idleLogout.saveFailed') });
    },
  });

  const parsed = Number(minutes);
  const minutesValid =
    minutes.trim() !== '' &&
    Number.isFinite(parsed) &&
    parsed >= MIN_TIMEOUT_MINUTES &&
    parsed <= MAX_TIMEOUT_MINUTES;

  const dirty =
    !!current && (enabled !== current.enabled || clampTimeoutMinutes(parsed) !== current.timeoutMinutes);

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
      <h3 className="text-lg font-semibold text-(--color-text-primary)">
        {t('settings.idleLogout.title')}
      </h3>
      <p className="mt-1 text-sm text-(--color-text-muted)">{t('settings.idleLogout.desc')}</p>

      {isLoading ? (
        <div className="mt-4 flex items-center gap-2 text-sm text-(--color-text-muted)">
          <Loader2 className="h-4 w-4 animate-spin" /> {t('common.loading')}
        </div>
      ) : (
        <div className="mt-4 space-y-4">
          <label className="flex items-center gap-2 text-sm text-(--color-text-primary)">
            <input
              type="checkbox"
              checked={enabled}
              disabled={isReadOnly || mutation.isPending}
              onChange={(e) => setEnabled(e.target.checked)}
              className="h-4 w-4 rounded border-(--color-border-strong)"
              data-testid="idle-logout-enabled"
            />
            {t('settings.idleLogout.enabledLabel')}
          </label>

          <div>
            <label
              htmlFor="idle-timeout-minutes"
              className="block text-sm font-medium text-(--color-text-primary)"
            >
              {t('settings.idleLogout.minutesLabel')}
            </label>
            <input
              id="idle-timeout-minutes"
              type="number"
              min={MIN_TIMEOUT_MINUTES}
              max={MAX_TIMEOUT_MINUTES}
              step={1}
              value={minutes}
              disabled={isReadOnly || !enabled || mutation.isPending}
              onChange={(e) => setMinutes(e.target.value)}
              className={cn(inputClass, 'mt-1')}
              data-testid="idle-logout-minutes"
            />
            <p className="mt-1 text-xs text-(--color-text-muted)">
              {t('settings.idleLogout.minutesHint')
                .replace('{min}', String(MIN_TIMEOUT_MINUTES))
                .replace('{max}', String(MAX_TIMEOUT_MINUTES))}
            </p>
          </div>

          <button
            type="button"
            onClick={() => mutation.mutate()}
            disabled={isReadOnly || !dirty || !minutesValid || mutation.isPending}
            className={cn(
              'inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors',
              'hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50',
            )}
            data-testid="idle-logout-save"
          >
            {mutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t('common.save')}
          </button>
        </div>
      )}
    </div>
  );
}
