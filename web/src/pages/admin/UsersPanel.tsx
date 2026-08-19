// 사용자 목록 패널 (SPEC-AUTH-006 M2.2 — U2, AC-03).
//
// 목록·등록·역할 변경(인라인)·비밀번호 재설정·삭제를 한 화면에서 처리한다.
// 화면 제목은 앱 헤더(components/layout/Header.tsx)가 그리고, 탭 전환은 상위
// UserManagementPage 가 담당한다 — 이 패널은 본문만 그린다.
//
// 표시하지 않는 것: 비밀번호 해시. 서버 DTO(dto.UserResponse)가 애초에 내려주지
// 않으므로 컬럼 자체가 없다 (AC-03).
//
// 409 잠금 방지 거부(마지막 관리자 삭제·강등, 자기 자신 삭제)는 원인을 사용자
// 언어로 안내하고 목록은 건드리지 않는다. 낙관적 갱신을 쓰지 않으므로 실패한
// 요청은 목록 상태에 아무 흔적도 남기지 않는다 (AC-03).
//
// 권한 판정은 전부 usePermission 을 거친다 — 인증 비활성·구버전 서버 폴백 규칙이
// 그 훅 한 곳에만 있어야 하기 때문이다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.2)

import { useMemo, useState } from 'react';
import { AlertTriangle, KeyRound, Loader2, Trash2, UserPlus } from 'lucide-react';

import {
  useAdminRoles,
  useAdminUsers,
  useCreateUser,
  useDeleteUser,
  useResetUserPassword,
  useUpdateUserRole,
} from '@/hooks/useUserAdmin';
import { usePermission } from '@/hooks/usePermission';
import PermissionButton from '@/components/common/PermissionButton';
import type { UserResponse } from '@/services/api/userService';
import { useAuthStore } from '@/stores/authStore';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';
import { formatEpochMs } from '@/lib/utils/format';
import { adminErrorMessageKey } from './adminErrors';

const INPUT_CLASS =
  'w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 ' +
  'text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 ' +
  'focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50';

const TH_CLASS =
  'px-3 py-2 text-left text-xs font-medium uppercase tracking-wide text-(--color-text-muted)';

const TD_CLASS = 'px-3 py-2 text-sm text-(--color-text-primary)';

/** 삭제·비밀번호 재설정 확인 대화상자 상태. 둘은 동시에 열리지 않는다. */
type DialogState =
  | { kind: 'reset'; username: string }
  | { kind: 'delete'; username: string }
  | null;

export default function UsersPanel(): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();
  const addNotification = useUIStore((s) => s.addNotification);
  // 자기 자신 행에서 삭제 컨트롤을 빼기 위한 현재 로그인 계정.
  // authStore 의 User.name 이 서버 DTO 의 username 에 대응한다(authService 매핑).
  const currentUsername = useAuthStore((s) => s.user?.name ?? null);

  const canUpdate = hasPermission('user.update');
  const canDelete = hasPermission('user.delete');
  // 역할 선택지는 GET /roles 에서 온다. role.read 가 없으면 서버가 403 이므로
  // 아예 조회하지 않고 자유 입력으로 대체한다.
  const canReadRoles = hasPermission('role.read');

  const usersQuery = useAdminUsers();
  const rolesQuery = useAdminRoles(canReadRoles);

  const createUser = useCreateUser();
  const updateRole = useUpdateUserRole();
  const resetPassword = useResetUserPassword();
  const deleteUser = useDeleteUser();

  const [actionError, setActionError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState('');
  const [dialog, setDialog] = useState<DialogState>(null);
  const [dialogPassword, setDialogPassword] = useState('');

  /** 빌트인 3종 + 커스텀 역할 이름. 조회 실패 시 빈 배열이다. */
  const roleNames = useMemo(
    () => (rolesQuery.data ?? []).map((role) => role.name),
    [rolesQuery.data],
  );
  const rolesUnavailable = canReadRoles ? rolesQuery.isError : true;

  /** 실패 원인을 사용자 언어로 남긴다. 성공 시에는 비운다. */
  function reportError(error: unknown): void {
    setActionError(t(adminErrorMessageKey(error)));
  }

  function notifySuccess(messageKey: string): void {
    setActionError(null);
    addNotification({ type: 'success', message: t(messageKey) });
  }

  function closeCreateForm(): void {
    setCreateOpen(false);
    setNewUsername('');
    setNewPassword('');
    setNewRole('');
  }

  function closeDialog(): void {
    setDialog(null);
    setDialogPassword('');
  }

  async function handleCreate(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    const username = newUsername.trim();
    const role = newRole.trim();
    if (!username || !newPassword || !role) return;

    try {
      await createUser.mutateAsync({ username, password: newPassword, role });
      notifySuccess('admin.users.created');
      closeCreateForm();
    } catch (error) {
      // 목록은 그대로 둔다 — 실패한 등록이 화면에 반영되면 안 된다.
      reportError(error);
    }
  }

  async function handleRoleChange(username: string, role: string): Promise<void> {
    try {
      await updateRole.mutateAsync({ username, role });
      notifySuccess('admin.users.roleChanged');
    } catch (error) {
      // 409(LAST_ADMIN_USER) 등 거부 시 select 는 서버 값으로 되돌아간다.
      reportError(error);
    }
  }

  async function handleResetPassword(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    if (dialog?.kind !== 'reset' || !dialogPassword) return;

    try {
      await resetPassword.mutateAsync({
        username: dialog.username,
        password: dialogPassword,
      });
      notifySuccess('admin.users.passwordReset');
      closeDialog();
    } catch (error) {
      reportError(error);
    }
  }

  async function handleDelete(): Promise<void> {
    if (dialog?.kind !== 'delete') return;

    try {
      await deleteUser.mutateAsync(dialog.username);
      notifySuccess('admin.users.deleted');
      closeDialog();
    } catch (error) {
      // AC-03: 마지막 관리자 삭제 거부 — 원인을 안내하고 목록은 변하지 않는다.
      reportError(error);
    }
  }

  /**
   * 역할 선택지. 현재 역할이 카탈로그에 없어도(삭제된 역할) 옵션에 남겨
   * select 가 값을 잃고 다른 역할로 보이는 일을 막는다.
   */
  function roleOptions(current: string): string[] {
    return roleNames.includes(current) ? roleNames : [current, ...roleNames];
  }

  const permissionTitle = t('admin.permissionRequired');

  return (
    <div className="space-y-4" data-testid="admin-users-panel">
      {/* 액션 행. 플로우/디바이스 등록 버튼과 같은 배치(우측 정렬)·같은 버튼
          레이아웃을 쓴다 — 화면마다 등록 버튼이 다르게 보이지 않도록. */}
      <div className="flex items-center justify-end">
        <PermissionButton
          type="button"
          permission="user.create"
          deniedTitle={permissionTitle}
          onClick={() => {
            setActionError(null);
            setCreateOpen((open) => !open);
          }}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <UserPlus className="h-4 w-4" />
          {t('admin.users.create')}
        </PermissionButton>
      </div>

      {actionError && (
        <div
          role="alert"
          data-testid="admin-action-error"
          className="flex items-start gap-2 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {createOpen && (
        <form
          onSubmit={handleCreate}
          data-testid="admin-users-create-form"
          className="space-y-3 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
        >
          <h2 className="text-sm font-semibold text-(--color-text-primary)">
            {t('admin.users.createTitle')}
          </h2>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="space-y-1 text-sm">
              <span className="text-(--color-text-muted)">{t('admin.users.username')}</span>
              <input
                className={INPUT_CLASS}
                value={newUsername}
                onChange={(e) => setNewUsername(e.target.value)}
                required
              />
            </label>
            <label className="space-y-1 text-sm">
              <span className="text-(--color-text-muted)">{t('admin.users.password')}</span>
              <input
                type="password"
                className={INPUT_CLASS}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder={t('admin.users.passwordHint')}
                required
              />
            </label>
            <label className="space-y-1 text-sm">
              <span className="text-(--color-text-muted)">{t('admin.users.role')}</span>
              {/* 역할 목록을 못 읽는 경우에만 자유 입력으로 낮춘다. 서버가 존재하지
                  않는 역할을 400 으로 거부하므로 오탈자는 그대로 드러난다. */}
              {rolesUnavailable ? (
                <input
                  className={INPUT_CLASS}
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value)}
                  required
                />
              ) : (
                <select
                  className={INPUT_CLASS}
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value)}
                  required
                >
                  <option value="">{t('admin.users.rolePlaceholder')}</option>
                  {roleNames.map((name) => (
                    <option key={name} value={name}>
                      {name}
                    </option>
                  ))}
                </select>
              )}
            </label>
          </div>
          {rolesUnavailable && (
            <p className="text-xs text-(--color-text-muted)">
              {t('admin.users.rolesUnavailable')}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={closeCreateForm}
              className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={createUser.isPending}
              className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {t('admin.users.submit')}
            </button>
          </div>
        </form>
      )}

      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="min-w-full divide-y divide-(--color-border-default)">
          <thead className="bg-(--color-bg-secondary)">
            <tr>
              <th className={TH_CLASS}>{t('admin.users.username')}</th>
              <th className={TH_CLASS}>{t('admin.users.role')}</th>
              <th className={TH_CLASS}>{t('admin.users.createdAt')}</th>
              <th className={TH_CLASS}>{t('admin.users.updatedAt')}</th>
              <th className={TH_CLASS}>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {usersQuery.isLoading && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  <span className="inline-flex items-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('common.loading')}
                  </span>
                </td>
              </tr>
            )}
            {usersQuery.isError && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  {t(adminErrorMessageKey(usersQuery.error))}
                </td>
              </tr>
            )}
            {usersQuery.data?.length === 0 && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  {t('admin.users.empty')}
                </td>
              </tr>
            )}
            {usersQuery.data?.map((user: UserResponse) => (
              <tr key={user.username} data-testid={`admin-user-row-${user.username}`}>
                <td className={TD_CLASS}>{user.username}</td>
                <td className={TD_CLASS}>
                  <select
                    className={`${INPUT_CLASS} max-w-[12rem] truncate py-1`}
                    aria-label={t('admin.users.roleColumnAria')}
                    value={user.role}
                    disabled={!canUpdate || roleNames.length === 0}
                    aria-disabled={!canUpdate || roleNames.length === 0}
                    title={canUpdate ? undefined : permissionTitle}
                    onChange={(e) => void handleRoleChange(user.username, e.target.value)}
                  >
                    {roleOptions(user.role).map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                </td>
                <td className={`${TD_CLASS} whitespace-nowrap text-(--color-text-muted)`}>
                  {formatEpochMs(user.created_at)}
                </td>
                <td className={`${TD_CLASS} whitespace-nowrap text-(--color-text-muted)`}>
                  {formatEpochMs(user.updated_at)}
                </td>
                <td className={TD_CLASS}>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => {
                        setActionError(null);
                        setDialog({ kind: 'reset', username: user.username });
                      }}
                      disabled={!canUpdate}
                      aria-disabled={!canUpdate}
                      title={canUpdate ? t('admin.users.resetPassword') : permissionTitle}
                      aria-label={`${t('admin.users.resetPassword')} ${user.username}`}
                      className="rounded p-1.5 text-(--color-text-muted) hover:bg-(--color-bg-secondary) disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      <KeyRound className="h-4 w-4" />
                    </button>
                    {/* 자기 자신 행에는 삭제 컨트롤을 아예 렌더하지 않는다.
                        이 프로젝트의 기본 규칙은 "숨기지 말고 비활성 + 사유"
                        (PermissionButton)지만, 자기 자신 삭제는 권한을 더 받아도
                        절대 성공할 수 없는 동작이다(서버가 409 SELF_DELETION 으로
                        항상 거부한다 — internal/api/handler/user.go). 요청할 여지가
                        없는 동작을 비활성 버튼으로 남기면 "권한만 있으면 되는 일"로
                        읽히므로 제거가 맞다. 이 한 곳만의 의도적 예외이며,
                        비활성 버튼으로 되돌리지 말 것. 서버 가드는 그대로 유지되며
                        실제 강제는 계속 서버가 한다. */}
                    {user.username !== currentUsername && (
                      <button
                        type="button"
                        onClick={() => {
                          setActionError(null);
                          setDialog({ kind: 'delete', username: user.username });
                        }}
                        disabled={!canDelete}
                        aria-disabled={!canDelete}
                        title={canDelete ? t('common.delete') : permissionTitle}
                        aria-label={`${t('common.delete')} ${user.username}`}
                        className="rounded p-1.5 text-red-500 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-40"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {dialog?.kind === 'reset' && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={t('admin.users.resetPassword')}
        >
          <form
            onSubmit={handleResetPassword}
            className="mx-4 w-full max-w-sm space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl"
          >
            <h2 className="text-base font-semibold text-(--color-text-primary)">
              {t('admin.users.resetPassword')} — {dialog.username}
            </h2>
            <label className="block space-y-1 text-sm">
              <span className="text-(--color-text-muted)">{t('admin.users.newPassword')}</span>
              <input
                type="password"
                className={INPUT_CLASS}
                value={dialogPassword}
                onChange={(e) => setDialogPassword(e.target.value)}
                placeholder={t('admin.users.passwordHint')}
                required
              />
            </label>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={closeDialog}
                className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="submit"
                disabled={resetPassword.isPending}
                className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {t('common.confirm')}
              </button>
            </div>
          </form>
        </div>
      )}

      {dialog?.kind === 'delete' && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={t('admin.users.deleteTitle')}
        >
          <div className="mx-4 w-full max-w-sm space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
            <h2 className="text-base font-semibold text-(--color-text-primary)">
              {t('admin.users.deleteTitle')}
            </h2>
            <p className="text-sm text-(--color-text-muted)">
              {t('admin.users.deleteConfirm').replace('{username}', dialog.username)}
            </p>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={closeDialog}
                className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={() => void handleDelete()}
                disabled={deleteUser.isPending}
                className="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                {t('common.delete')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
