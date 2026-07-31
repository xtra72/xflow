// 스케줄 로그 저장 방식(storage backend) 설정 카드.
//
// 스케줄 실행 로그를 어디에 저장할지(sqlite/file/memory)를 선택·저장한다.
// admin 전용 엔드포인트를 소비하며, 변경은 재시작 후 적용된다(needs_restart).
// SettingsPage 시스템 탭에 배치되고, 다른 admin 전용 섹션과 동일하게
// viewer 역할에서는 컨트롤을 비활성화한다.

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Loader2, RotateCw, Save } from 'lucide-react';

import {
  getScheduleLogStorageType,
  isScheduleLogStorageType,
  setScheduleLogStorageType,
} from '@/services/api/scheduleLogConfigService';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

/** 스케줄 로그 설정 쿼리 키. */
const SCHEDULE_LOG_CONFIG_KEY = ['system', 'schedule-log-config'] as const;

/** 저장 방식 옵션 메타데이터. labelKey/descKey 는 렌더 시 t()로 변환한다. */
const STORAGE_OPTIONS: { value: string; labelKey: string; descKey: string }[] = [
  { value: 'sqlite', labelKey: 'settings.scheduleLog.storageSqlite', descKey: 'settings.scheduleLog.descSqlite' },
  { value: 'file', labelKey: 'settings.scheduleLog.storageFile', descKey: 'settings.scheduleLog.descFile' },
  { value: 'memory', labelKey: 'settings.scheduleLog.storageMemory', descKey: 'settings.scheduleLog.descMemory' },
];

/** 셀렉트 기본 스타일(다른 설정 섹션의 inputClass 와 동일 규칙). */
const inputClass = cn(
  'block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) shadow-sm',
  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
  'disabled:cursor-not-allowed disabled:opacity-50',
);

/**
 * 스케줄 로그 저장 방식 설정 카드.
 *
 * @param isViewer - viewer 역할이면 컨트롤을 비활성화한다(다른 admin 섹션과 동일).
 */
export function ScheduleLogStorageCard({ isViewer }: { isViewer: boolean }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const qc = useQueryClient();

  const { data: current, isLoading } = useQuery({
    queryKey: SCHEDULE_LOG_CONFIG_KEY,
    queryFn: () => getScheduleLogStorageType(),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const mutation = useMutation({
    mutationFn: (storageType: string) => {
      // 유니온 타입으로 좁혀 전송한다(알 수 없는 값은 저장 버튼에서 이미 차단).
      if (!isScheduleLogStorageType(storageType)) {
        return Promise.reject(new Error('invalid storage type'));
      }
      return setScheduleLogStorageType(storageType);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: SCHEDULE_LOG_CONFIG_KEY });
    },
  });

  // 선택 드래프트. 서버 값 로드 시 초기화한다.
  const [selected, setSelected] = useState<string>('sqlite');
  // 재시작 후 적용 안내(마지막 저장이 needs_restart 였을 때 표시).
  const [showRestartNotice, setShowRestartNotice] = useState(false);

  useEffect(() => {
    if (current != null) setSelected(current);
  }, [current]);

  const unchanged = current != null && selected === current;
  const controlsDisabled = isViewer || isLoading || mutation.isPending;

  async function handleSave() {
    try {
      const res = await mutation.mutateAsync(selected);
      setShowRestartNotice(res.needsRestart);
      addNotification({
        type: 'success',
        message: res.needsRestart
          ? t('settings.scheduleLog.savedRestart')
          : t('settings.scheduleLog.saved'),
      });
    } catch {
      addNotification({ type: 'error', message: t('settings.scheduleLog.saveError') });
    }
  }

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
      <h3 className="text-lg font-semibold text-(--color-text-primary)">
        {t('settings.scheduleLog.title')}
      </h3>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('settings.scheduleLog.desc')}
      </p>

      <div className="mt-4 max-w-md space-y-3">
        <div>
          <label
            htmlFor="schedule-log-storage"
            className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
          >
            {t('settings.scheduleLog.selectLabel')}
          </label>
          <select
            id="schedule-log-storage"
            value={selected}
            onChange={(e) => {
              setSelected(e.target.value);
              setShowRestartNotice(false);
            }}
            disabled={controlsDisabled}
            className={inputClass}
          >
            {STORAGE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {t(option.labelKey)}
              </option>
            ))}
          </select>
        </div>

        {/* 각 옵션 설명 */}
        <ul className="space-y-1 text-xs text-(--color-text-muted)">
          {STORAGE_OPTIONS.map((option) => (
            <li key={option.value}>
              <span className="font-medium text-(--color-text-secondary)">{t(option.labelKey)}</span>
              {' — '}
              {t(option.descKey)}
            </li>
          ))}
        </ul>

        {/* 재시작 후 적용 안내 */}
        {showRestartNotice && (
          <div
            role="status"
            className="flex items-center gap-2 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
          >
            <RotateCw className="h-4 w-4 shrink-0" aria-hidden="true" />
            <span>{t('settings.scheduleLog.restartNotice')}</span>
          </div>
        )}

        <div className="pt-1">
          <button
            type="button"
            onClick={handleSave}
            disabled={controlsDisabled || unchanged}
            className={cn(
              'inline-flex items-center gap-1 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors',
              'hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-800',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {mutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Save className="h-4 w-4" aria-hidden="true" />
            )}
            {t('settings.scheduleLog.save')}
          </button>
        </div>
      </div>
    </div>
  );
}
