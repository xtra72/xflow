// VersionManagementSection 은 노드 대시보드의 "버전 관리" 섹션이다(버전 관리 Phase 1/2).
//
// - 현재 버전 + 목표 버전 대비 구버전 여부(outdated) 표시
// - 서버 전역 목표 버전 조회/설정(GET/PUT /remote/target-version)
// - 노드 버전 변경 이력 타임라인(GET /remote/nodes/{id}/version-history)
// - 원격 자가 업데이트 트리거(POST /remote/nodes/{id}/update) + 확인 다이얼로그
//
// 업데이트는 승인+온라인 노드에만 가능하므로, 오프라인이면 버튼을 비활성화한다.

import { useEffect, useState } from 'react';
import { History, Loader2, RefreshCw } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import {
  useNodeVersionHistory,
  useSetTargetVersion,
  useTargetVersion,
  useUpdateNode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { errorDetail } from '@/lib/remote/errorMessage';
import { formatDate } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';

interface VersionManagementSectionProps {
  instanceId: string;
  /** 노드가 보고한 현재 버전(detail.version). */
  currentVersion: string;
  /** 노드 온라인 여부 — 오프라인이면 업데이트 불가. */
  online: boolean;
}

/**
 * semver 비교: a < b 이면 음수, a == b 이면 0, a > b 이면 양수.
 * 'v' 접두사를 허용하고 숫자 세그먼트만 비교한다. 파싱 불가 세그먼트는 0 으로 취급한다.
 */
function compareVersions(a: string, b: string): number {
  const parse = (v: string): number[] =>
    v.replace(/^v/, '').split('.').map((s) => parseInt(s, 10) || 0);
  const pa = parse(a);
  const pb = parse(b);
  const len = Math.max(pa.length, pb.length);
  for (let i = 0; i < len; i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d < 0 ? -1 : 1;
  }
  return 0;
}

/** semver 형식(vMAJOR.MINOR.PATCH[...]) 대략 검증. */
function isValidSemver(v: string): boolean {
  return /^v?\d+\.\d+\.\d+/.test(v.trim());
}

export function VersionManagementSection({
  instanceId,
  currentVersion,
  online,
}: VersionManagementSectionProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const targetQuery = useTargetVersion();
  const historyQuery = useNodeVersionHistory(instanceId);
  const setTarget = useSetTargetVersion();
  const updateNode = useUpdateNode();

  const target = targetQuery.data?.version ?? '';
  const [draft, setDraft] = useState('');
  const [restart, setRestart] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  // 서버 목표 버전이 로드/변경되면 입력 초안을 동기화한다.
  useEffect(() => {
    setDraft(target);
  }, [target]);

  const outdated =
    target !== '' &&
    isValidSemver(currentVersion) &&
    isValidSemver(target) &&
    compareVersions(currentVersion, target) < 0;

  const history = historyQuery.data ?? [];

  const handleSaveTarget = (): void => {
    const next = draft.trim();
    if (next !== '' && !isValidSemver(next)) {
      addNotification({ type: 'error', message: t('remote.toast.updateFailed') });
      return;
    }
    setTarget.mutate(next, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('remote.toast.targetSaved') }),
      onError: () =>
        addNotification({ type: 'error', message: t('remote.toast.actionFailed') }),
    });
  };

  const handleConfirmUpdate = (): void => {
    updateNode.mutate(
      { instanceID: instanceId, req: { version: target || undefined, restart } },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.toast.updateStarted') });
          setConfirmOpen(false);
        },
        onError: (err: unknown) => {
          addNotification({
            type: 'error',
            message: t('remote.toast.updateFailed') + errorDetail(err),
          });
          setConfirmOpen(false);
        },
      },
    );
  };

  return (
    <section
      aria-labelledby="node-version-heading"
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      data-testid="version-management"
    >
      <div className="mb-3 flex items-center gap-2">
        <RefreshCw className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
        <h2
          id="node-version-heading"
          className="text-sm font-semibold text-(--color-text-primary)"
        >
          {t('remote.version.title')}
        </h2>
      </div>

      {/* 현재/목표 버전 + outdated 배지 */}
      <div className="mb-4 flex flex-wrap items-end gap-x-6 gap-y-3">
        <div>
          <div className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.version.current')}
          </div>
          <div className="mt-1 flex items-center gap-2">
            <span className="font-mono text-sm text-(--color-text-primary)" data-testid="version-current">
              {currentVersion || '-'}
            </span>
            {outdated ? (
              <span
                className="rounded border border-yellow-200 bg-yellow-50 px-1.5 py-0.5 text-[11px] font-medium text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-200"
                data-testid="version-outdated-badge"
              >
                {t('remote.version.outdated')}
              </span>
            ) : target !== '' ? (
              <span
                className="rounded border border-emerald-200 bg-emerald-50 px-1.5 py-0.5 text-[11px] font-medium text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200"
                data-testid="version-uptodate-badge"
              >
                {t('remote.version.upToDate')}
              </span>
            ) : null}
          </div>
        </div>

        {/* 서버 전역 목표 버전 설정 */}
        <div className="flex-1">
          <label
            htmlFor="target-version-input"
            className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
          >
            {t('remote.version.target')}
          </label>
          <div className="mt-1 flex items-center gap-2">
            <input
              id="target-version-input"
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('remote.version.targetPlaceholder')}
              data-testid="target-version-input"
              className="w-40 rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 font-mono text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
            <button
              type="button"
              onClick={handleSaveTarget}
              disabled={setTarget.isPending || draft.trim() === target}
              data-testid="target-version-save"
              className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2.5 py-1 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              {setTarget.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('remote.version.save')}
            </button>
          </div>
        </div>

        {/* 원격 업데이트 트리거 */}
        <div className="flex flex-col items-end gap-1.5">
          <label className="flex items-center gap-1.5 text-xs text-(--color-text-muted)">
            <input
              type="checkbox"
              checked={restart}
              onChange={(e) => setRestart(e.target.checked)}
              data-testid="update-restart-checkbox"
              className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
            />
            {t('remote.version.restart')}
          </label>
          <button
            type="button"
            onClick={() => setConfirmOpen(true)}
            disabled={!online || updateNode.isPending}
            data-testid="update-node-button"
            title={!online ? t('remote.offline') : undefined}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {updateNode.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('remote.version.update')}
          </button>
        </div>
      </div>

      {/* 버전 이력 타임라인 */}
      <div>
        <div className="mb-2 flex items-center gap-1.5">
          <History className="h-3.5 w-3.5 text-(--color-text-muted)" aria-hidden="true" />
          <h3 className="text-xs font-semibold uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.version.history')}
          </h3>
        </div>
        {history.length === 0 ? (
          <p className="text-xs text-(--color-text-muted)" data-testid="version-history-empty">
            {t('remote.version.historyEmpty')}
          </p>
        ) : (
          <ul className="space-y-1" data-testid="version-history-list">
            {history.map((h, i) => (
              <li
                key={`${h.version}-${h.changed_at}-${i}`}
                className="flex items-center justify-between rounded border border-(--color-border-default) px-2.5 py-1 text-xs"
              >
                <span className="font-mono text-(--color-text-primary)">{h.version}</span>
                <span className="text-(--color-text-muted)">
                  {h.changed_at > 0 ? formatDate(new Date(h.changed_at), 'long') : '-'}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <ConfirmDialog
        open={confirmOpen}
        title={t('remote.confirm.updateTitle')}
        description={t('remote.confirm.updateDesc')}
        confirmLabel={t('remote.action.update')}
        destructive={false}
        pending={updateNode.isPending}
        onConfirm={handleConfirmUpdate}
        onCancel={() => setConfirmOpen(false)}
      />
    </section>
  );
}
