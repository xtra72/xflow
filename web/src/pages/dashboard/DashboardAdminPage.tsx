// 대시보드 관리 화면 (SPEC-DASHBOARD-004 M7 7.1, spec.md §1.1 4번 축).
//
// 목록·생성·이름변경·삭제·공개범위 변경을 한 화면에서 처리하고, `can_grant` 인
// 대시보드는 권한 부여 패널을 연다.
//
// 데이터 축은 uiStore 의 `dashboards` **하나**다(`GET /api/v1/dashboards` 를
// AppLayout 의 useDashboardSync 가 채운다). 이 화면이 목록을 따로 조회하지 않는
// 이유는 두 축이 갈리면 여기서 이름을 바꾼 결과가 대시보드 화면에 반영되지 않기
// 때문이다. 변경도 같은 이유로 M6 의 useDashboardMutations 를 그대로 쓴다 —
// 두 번째 변경 경로를 만들면 낙관적 반영·403 재조회 정책이 갈라진다.
//
// 컨트롤은 숨기지 않고 **비활성 + 사유 툴팁**이다(SPEC-AUTH-006 §4.2,
// spec.md §2.11 S2). 메뉴는 숨기고(`nav.dashboard`) 컨트롤은 비활성하는 이 비대칭이
// 의도된 것이다 — 메뉴 축과 액션 축은 서로 다른 축이다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.7, §2.11, §2.12)

import { useState } from 'react';
import { Check, KeyRound, Pencil, Plus, Star, Trash2, X } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { formatEpochMs } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';
import type { Dashboard, DashboardVisibility } from '@/types/dashboard';
import { dashboardAccessOf } from './dashboardAccess';
import DashboardAclPanel from './DashboardAclPanel';
import { useCreateDashboard } from './useCreateDashboard';
import { useDashboardMutations } from './useDashboardMutations';

const INPUT_CLASS =
  'w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 ' +
  'text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 ' +
  'focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50';

const TH_CLASS =
  'px-3 py-2 text-left text-xs font-medium uppercase tracking-wide text-(--color-text-muted)';

const TD_CLASS = 'px-3 py-2 text-sm text-(--color-text-primary)';

const ICON_BUTTON_CLASS =
  'rounded p-1.5 text-(--color-text-muted) hover:bg-(--color-bg-secondary) ' +
  'disabled:cursor-not-allowed disabled:opacity-40';

/** 공개범위 선택지 — 서버 CHECK 제약(spec.md §2.1)과 동일한 3모드다. */
const VISIBILITIES: readonly DashboardVisibility[] = ['private', 'shared', 'acl'];

export default function DashboardAdminPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();

  // 목록 축이 단일 진실이다 — 이름·공개범위·인가 판정이 전부 여기 있다.
  const dashboards = useUIStore((s) => s.dashboards);

  const canCreate = hasPermission('dashboard.create');
  const { create, isCreating } = useCreateDashboard();
  const { rename, setVisibility, remove, isMutating } = useDashboardMutations();

  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [renamingUid, setRenamingUid] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Dashboard | null>(null);
  const [aclTarget, setAclTarget] = useState<Dashboard | null>(null);

  async function handleCreate(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    const trimmed = newName.trim();
    if (!trimmed) return;
    // 실패는 훅이 토스트로 알린다. 폼은 실패 시 열어 두어 입력을 잃지 않게 한다.
    const created = await create(trimmed);
    if (created) {
      setNewName('');
      setCreateOpen(false);
    }
  }

  function startRename(dashboard: Dashboard): void {
    setRenamingUid(dashboard.uid);
    setRenameValue(dashboard.name);
  }

  function cancelRename(): void {
    setRenamingUid(null);
    setRenameValue('');
  }

  async function confirmRename(uid: string): Promise<void> {
    await rename(uid, renameValue);
    cancelRename();
  }

  async function handleDelete(): Promise<void> {
    if (!deleteTarget) return;
    await remove(deleteTarget.uid);
    setDeleteTarget(null);
  }

  return (
    <div className="mx-auto max-w-5xl space-y-4 p-6" data-testid="dashboard-admin-page">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-(--color-text-primary)">
            {t('dashboard.admin.title')}
          </h1>
          <p className="text-sm text-(--color-text-muted)">
            {t('dashboard.admin.description')}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setCreateOpen((open) => !open)}
          disabled={!canCreate}
          aria-disabled={!canCreate}
          title={canCreate ? undefined : t('dashboard.gate.createDenied')}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Plus className="h-4 w-4" />
          {t('dashboard.admin.create')}
        </button>
      </header>

      {createOpen && (
        <form
          onSubmit={(e) => void handleCreate(e)}
          data-testid="dashboard-admin-create-form"
          className="space-y-3 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
        >
          <h2 className="text-sm font-semibold text-(--color-text-primary)">
            {t('dashboard.admin.createTitle')}
          </h2>
          <label className="block space-y-1 text-sm">
            <span className="text-(--color-text-muted)">{t('dashboard.admin.nameLabel')}</span>
            <input
              className={INPUT_CLASS}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t('dashboard.admin.namePlaceholder')}
              maxLength={64}
              required
            />
          </label>
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setCreateOpen(false);
                setNewName('');
              }}
              className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={isCreating}
              className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {t('dashboard.admin.submit')}
            </button>
          </div>
        </form>
      )}

      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="min-w-full divide-y divide-(--color-border-default)">
          <thead className="bg-(--color-bg-secondary)">
            <tr>
              <th className={TH_CLASS}>{t('dashboard.admin.nameLabel')}</th>
              <th className={TH_CLASS}>{t('dashboard.admin.owner')}</th>
              <th className={TH_CLASS}>{t('dashboard.admin.visibility')}</th>
              <th className={TH_CLASS}>{t('dashboard.admin.updatedAt')}</th>
              <th className={TH_CLASS}>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {dashboards.length === 0 && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  {t('dashboard.admin.empty')}
                </td>
              </tr>
            )}
            {dashboards.map((dashboard) => {
              const access = dashboardAccessOf(dashboard);
              const isRenaming = renamingUid === dashboard.uid;
              return (
                <tr key={dashboard.uid} data-testid={`dashboard-admin-row-${dashboard.uid}`}>
                  <td className={TD_CLASS}>
                    {isRenaming ? (
                      <div className="flex items-center gap-1">
                        <input
                          className={`${INPUT_CLASS} max-w-[14rem] py-1`}
                          aria-label={t('dashboard.admin.renameInputAria')}
                          value={renameValue}
                          onChange={(e) => setRenameValue(e.target.value)}
                          maxLength={64}
                          autoFocus
                        />
                        <button
                          type="button"
                          onClick={() => void confirmRename(dashboard.uid)}
                          aria-label={t('dashboard.admin.renameSave')}
                          title={t('dashboard.admin.renameSave')}
                          className={ICON_BUTTON_CLASS}
                        >
                          <Check className="h-4 w-4" />
                        </button>
                        <button
                          type="button"
                          onClick={cancelRename}
                          aria-label={t('dashboard.admin.renameCancel')}
                          title={t('dashboard.admin.renameCancel')}
                          className={ICON_BUTTON_CLASS}
                        >
                          <X className="h-4 w-4" />
                        </button>
                      </div>
                    ) : (
                      <span className="inline-flex items-center gap-1.5">
                        {dashboard.name}
                        {/* 기본 대시보드 표시는 읽기 전용이다 — 기본 지정 컨트롤은
                            대시보드 화면이 이미 제공한다(중복 경로를 만들지 않는다). */}
                        {dashboard.is_default && (
                          <span
                            className="inline-flex items-center gap-1 rounded bg-(--color-bg-secondary) px-1.5 py-0.5 text-xs text-(--color-text-muted)"
                            data-testid={`dashboard-admin-default-${dashboard.uid}`}
                          >
                            <Star className="h-3 w-3 fill-current" aria-hidden="true" />
                            {t('dashboard.admin.defaultBadge')}
                          </span>
                        )}
                      </span>
                    )}
                  </td>
                  <td className={`${TD_CLASS} text-(--color-text-muted)`}>
                    {dashboard.owner || t('dashboard.admin.ownerless')}
                  </td>
                  <td className={TD_CLASS}>
                    <select
                      className={`${INPUT_CLASS} max-w-[10rem] py-1`}
                      aria-label={`${t('dashboard.admin.visibilityAria')} ${dashboard.name}`}
                      value={dashboard.visibility}
                      disabled={!access.canGrant || isMutating}
                      aria-disabled={!access.canGrant}
                      title={access.canGrant ? undefined : t('dashboard.gate.grantDenied')}
                      onChange={(e) =>
                        void setVisibility(
                          dashboard.uid,
                          e.target.value as DashboardVisibility,
                        )
                      }
                    >
                      {VISIBILITIES.map((value) => (
                        <option key={value} value={value}>
                          {t(`dashboard.visibilityLabel.${value}`)}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className={`${TD_CLASS} whitespace-nowrap text-(--color-text-muted)`}>
                    {formatEpochMs(dashboard.updated_at)}
                  </td>
                  <td className={TD_CLASS}>
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => startRename(dashboard)}
                        disabled={!access.canEdit || isRenaming}
                        aria-disabled={!access.canEdit}
                        aria-label={`${t('dashboard.admin.renameStart')} ${dashboard.name}`}
                        title={
                          access.canEdit
                            ? t('dashboard.admin.renameStart')
                            : t('dashboard.gate.editDenied')
                        }
                        className={ICON_BUTTON_CLASS}
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                      {/* 권한 부여 패널은 grant 인가에서만 열린다(spec.md §2.2). */}
                      <button
                        type="button"
                        onClick={() => setAclTarget(dashboard)}
                        disabled={!access.canGrant}
                        aria-disabled={!access.canGrant}
                        aria-label={`${t('dashboard.admin.manageAcl')} ${dashboard.name}`}
                        title={
                          access.canGrant
                            ? t('dashboard.admin.manageAcl')
                            : t('dashboard.gate.grantDenied')
                        }
                        className={ICON_BUTTON_CLASS}
                      >
                        <KeyRound className="h-4 w-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setDeleteTarget(dashboard)}
                        disabled={!access.canDelete}
                        aria-disabled={!access.canDelete}
                        aria-label={`${t('common.delete')} ${dashboard.name}`}
                        title={
                          access.canDelete ? t('common.delete') : t('dashboard.gate.deleteDenied')
                        }
                        className="rounded p-1.5 text-red-500 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-40"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {deleteTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={t('dashboard.admin.deleteTitle')}
        >
          <div className="mx-4 w-full max-w-sm space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
            <h2 className="text-base font-semibold text-(--color-text-primary)">
              {t('dashboard.admin.deleteTitle')}
            </h2>
            <p className="text-sm text-(--color-text-muted)">
              {t('dashboard.admin.deleteConfirm').replace('{name}', deleteTarget.name)}
            </p>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setDeleteTarget(null)}
                className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={() => void handleDelete()}
                disabled={isMutating}
                className="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                {t('common.delete')}
              </button>
            </div>
          </div>
        </div>
      )}

      {aclTarget && (
        <DashboardAclPanel dashboard={aclTarget} onClose={() => setAclTarget(null)} />
      )}
    </div>
  );
}
