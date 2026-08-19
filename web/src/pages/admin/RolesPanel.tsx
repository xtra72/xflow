// 역할 목록 패널 (SPEC-AUTH-006 M2.3 — U2, AC-04).
//
// 사용자 관리 화면(UserManagementPage)의 '역할' 탭 본문이다. 제목은 앱 헤더가,
// 탭 노출 판정(nav.role)은 상위 화면이 담당한다 — 이 패널은 본문만 그린다.
//
// 목록(이름·설명·빌트인 여부·권한 개수)과 생성·수정 폼을 제공한다. 권한 선택 축은
// `GET /permissions` 카탈로그에서 파생한다. 리소스·액션 목록을 화면에 하드코딩하면
// 서버 카탈로그가 바뀔 때 조용히 어긋나므로, 축은 언제나 응답에서 만든다 (AC-04).
//
// 권한 선택은 두 구획으로 나눈다 — 카탈로그에 성격이 다른 두 축이 섞여 있기 때문이다.
//   1. 데이터 권한: 리소스×액션 행렬 (read/create/update/delete/execute)
//   2. 메뉴 노출:   `nav.<메뉴>` 체크리스트
// nav 는 액션 자리에 메뉴 식별자가 온다(internal/rbac/catalog.go 의 ResourceNav 주석:
// 데이터를 읽을 수 있어도 메뉴는 감출 수 있어야 한다). 이를 행렬 행으로 두면 메뉴 이름
// 11개가 액션 열로 승격되어 두 축이 서로 다른 열 범위를 차지하고, 16×13 표의 76% 가
// 빈 칸이 된다. 폭 문제가 아니라 축 설계 문제이므로 구획을 나눈다.
//
// 두 구획 모두 같은 `selected` 집합에 쓴다 — 제출은 전체 교체(PUT) 그대로다.
//
// 해당 리소스에 없는 액션(예: node 는 read 만, dashboard 는 read/update 만)은 셀을
// 비운다. 존재하지 않는 권한을 비활성 체크박스로 그리면 "권한은 있는데 잠겨 있다"로
// 읽히기 때문이다. 메뉴 체크리스트에 카탈로그에 없는 메뉴가 아예 나타나지 않는 것도
// 같은 이유다.
//
// 빌트인 역할(admin/editor/viewer)은 수정·삭제 컨트롤을 비활성화하고 사유를 표시한다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.3)

import { useMemo, useState } from 'react';
import { AlertTriangle, Loader2, Pencil, Plus, Trash2 } from 'lucide-react';

import {
  useAdminRoles,
  useCreateRole,
  useDeleteRole,
  usePermissionCatalog,
  useUpdateRole,
} from '@/hooks/useUserAdmin';
import { usePermission } from '@/hooks/usePermission';
import PermissionButton from '@/components/common/PermissionButton';
import type { RoleResponse, UpdateRoleRequest } from '@/services/api/userService';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';
import { adminErrorMessageKey } from './adminErrors';
import { buildNavMenuItems, buildPermissionMatrix } from './permissionMatrix';

const INPUT_CLASS =
  'w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 ' +
  'text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 ' +
  'focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50';

const TH_CLASS =
  'px-3 py-2 text-left text-xs font-medium uppercase tracking-wide text-(--color-text-muted)';

const TD_CLASS = 'px-3 py-2 text-sm text-(--color-text-primary)';

/** 편집 중인 대상. create 는 신규, edit 는 기존 역할 수정이다. */
type EditorState =
  | { mode: 'create' }
  | { mode: 'edit'; original: RoleResponse }
  | null;

export default function RolesPanel(): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();
  const addNotification = useUIStore((s) => s.addNotification);

  const canUpdate = hasPermission('role.update');
  const canDelete = hasPermission('role.delete');

  const rolesQuery = useAdminRoles();
  const catalogQuery = usePermissionCatalog();

  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const deleteRole = useDeleteRole();

  const [actionError, setActionError] = useState<string | null>(null);
  const [editor, setEditor] = useState<EditorState>(null);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [deleteTarget, setDeleteTarget] = useState<RoleResponse | null>(null);

  const matrix = useMemo(
    () => buildPermissionMatrix(catalogQuery.data ?? []),
    [catalogQuery.data],
  );
  // 메뉴 노출 항목도 카탈로그에서 파생한다 — 서버에 `nav.<새메뉴>` 가 생기면
  // 화면 수정 없이 체크리스트에 나타난다.
  const navMenus = useMemo(
    () => buildNavMenuItems(catalogQuery.data ?? []),
    [catalogQuery.data],
  );

  function reportError(error: unknown): void {
    setActionError(t(adminErrorMessageKey(error)));
  }

  function notifySuccess(messageKey: string): void {
    setActionError(null);
    addNotification({ type: 'success', message: t(messageKey) });
  }

  function openCreate(): void {
    setActionError(null);
    setEditor({ mode: 'create' });
    setName('');
    setDescription('');
    setSelected(new Set());
  }

  function openEdit(role: RoleResponse): void {
    setActionError(null);
    setEditor({ mode: 'edit', original: role });
    setName(role.name);
    setDescription(role.description);
    setSelected(new Set(role.permissions));
  }

  function closeEditor(): void {
    setEditor(null);
    setSelected(new Set());
  }

  function toggle(key: string): void {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function handleSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    if (!editor) return;

    const trimmed = name.trim();
    if (!trimmed) return;
    // 체크된 권한만 요청에 담는다. 비운 셀은 애초에 키가 없어 담길 수 없다.
    const permissions = [...selected];

    try {
      if (editor.mode === 'create') {
        await createRole.mutateAsync({
          name: trimmed,
          description: description.trim(),
          permissions,
        });
        notifySuccess('admin.roles.created');
      } else {
        // 생략한 필드는 서버가 변경하지 않는다 — 이름이 그대로면 보내지 않는다.
        const req: UpdateRoleRequest = { permissions };
        if (trimmed !== editor.original.name) req.name = trimmed;
        await updateRole.mutateAsync({ name: editor.original.name, req });
        notifySuccess('admin.roles.updated');
      }
      closeEditor();
    } catch (error) {
      // 목록은 그대로 둔다 — 거부된 변경이 화면에 반영되면 안 된다.
      reportError(error);
    }
  }

  async function handleDelete(): Promise<void> {
    if (!deleteTarget) return;

    try {
      await deleteRole.mutateAsync(deleteTarget.name);
      notifySuccess('admin.roles.deleted');
      setDeleteTarget(null);
    } catch (error) {
      // ROLE_IN_USE / BUILTIN_ROLE_IMMUTABLE 등 — 원인을 안내하고 목록은 유지한다.
      reportError(error);
    }
  }

  const permissionTitle = t('admin.permissionRequired');

  // 서버 계약(internal/api/handler/role.go)을 그대로 비춘다. 빌트인이라고 전부
  // 막으면 신규 설치처럼 커스텀 역할이 없는 환경에서 편집 가능한 역할이 하나도
  // 없어진다. 서버가 실제로 거부하는 것은 셋뿐이다.
  //   - admin 역할의 수정 전체        (ADMIN_ROLE_IMMUTABLE)
  //   - 빌트인 역할의 이름 변경        (BUILTIN_ROLE_IMMUTABLE)
  //   - 빌트인 역할의 삭제            (BUILTIN_ROLE_IMMUTABLE)
  // editor / viewer 의 권한 수정은 허용된다.
  const isAdminRole = (role: RoleResponse): boolean => role.name === 'admin';

  /** 수정 불가 사유. 수정 가능하면 undefined. */
  function editDisabledReason(role: RoleResponse): string | undefined {
    if (isAdminRole(role)) return t('admin.roles.adminImmutableReason');
    if (!canUpdate) return permissionTitle;
    return undefined;
  }

  /** 삭제 불가 사유. 삭제 가능하면 undefined. */
  function deleteDisabledReason(role: RoleResponse): string | undefined {
    if (role.builtin) return t('admin.roles.builtinDeleteReason');
    if (!canDelete) return permissionTitle;
    return undefined;
  }

  // 빌트인 역할 편집 시 이름은 잠근다 — 서버가 이름 변경을 거부한다.
  const nameLocked = editor?.mode === 'edit' && editor.original.builtin === true;

  return (
    <div className="space-y-4" data-testid="admin-roles-panel">
      {/* 액션 행. 사용자 탭의 등록 버튼과 같은 배치·레이아웃을 쓴다. */}
      <div className="flex items-center justify-end">
        <PermissionButton
          type="button"
          permission="role.create"
          deniedTitle={permissionTitle}
          onClick={openCreate}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-4 w-4" />
          {t('admin.roles.create')}
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

      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="min-w-full divide-y divide-(--color-border-default)">
          <thead className="bg-(--color-bg-secondary)">
            <tr>
              <th className={TH_CLASS}>{t('admin.roles.name')}</th>
              <th className={TH_CLASS}>{t('admin.roles.description')}</th>
              <th className={TH_CLASS}>{t('admin.roles.builtin')}</th>
              <th className={TH_CLASS}>{t('admin.roles.permissionCount')}</th>
              <th className={TH_CLASS}>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {rolesQuery.isLoading && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  <span className="inline-flex items-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('common.loading')}
                  </span>
                </td>
              </tr>
            )}
            {rolesQuery.isError && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  {t(adminErrorMessageKey(rolesQuery.error))}
                </td>
              </tr>
            )}
            {rolesQuery.data?.length === 0 && (
              <tr>
                <td className={`${TD_CLASS} text-(--color-text-muted)`} colSpan={5}>
                  {t('admin.roles.empty')}
                </td>
              </tr>
            )}
            {rolesQuery.data?.map((role) => (
              <tr key={role.name} data-testid={`admin-role-row-${role.name}`}>
                <td className={TD_CLASS}>
                  <span className="block max-w-[12rem] truncate">{role.name}</span>
                </td>
                <td className={`${TD_CLASS} text-(--color-text-muted)`}>
                  <span className="block max-w-[20rem] truncate">{role.description}</span>
                </td>
                <td className={TD_CLASS}>
                  {role.builtin ? (
                    <span className="rounded bg-(--color-bg-secondary) px-1.5 py-0.5 text-xs text-(--color-text-muted)">
                      {t('admin.roles.builtinBadge')}
                    </span>
                  ) : (
                    <span className="text-(--color-text-muted)">—</span>
                  )}
                </td>
                <td className={TD_CLASS}>{role.permissions.length}</td>
                <td className={TD_CLASS}>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => openEdit(role)}
                      disabled={editDisabledReason(role) !== undefined}
                      aria-disabled={editDisabledReason(role) !== undefined}
                      title={editDisabledReason(role) ?? t('admin.roles.edit')}
                      aria-label={`${t('admin.roles.edit')} ${role.name}`}
                      className="rounded p-1.5 text-(--color-text-muted) hover:bg-(--color-bg-secondary) disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      <Pencil className="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setActionError(null);
                        setDeleteTarget(role);
                      }}
                      disabled={deleteDisabledReason(role) !== undefined}
                      aria-disabled={deleteDisabledReason(role) !== undefined}
                      title={deleteDisabledReason(role) ?? t('common.delete')}
                      aria-label={`${t('common.delete')} ${role.name}`}
                      className="rounded p-1.5 text-red-500 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                  {role.builtin && (
                    <p className="mt-0.5 text-xs text-(--color-text-muted)">
                      {isAdminRole(role)
                        ? t('admin.roles.adminImmutableReason')
                        : t('admin.roles.builtinDeleteReason')}
                    </p>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {editor && (
        // 목록은 전체 폭을 쓰지만(1506ca71) 폼은 제한한다. 폭이 풀리면 이름·설명
        // 입력 두 칸이 화면 절반씩 차지하고, 6열짜리 권한 행렬은 체크박스 사이가
        // 수백 px 로 벌어져 어느 행의 어느 열인지 눈으로 따라가기 어려워진다.
        <form
          onSubmit={handleSubmit}
          data-testid="admin-roles-editor"
          className="max-w-4xl space-y-4 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
        >
          <h2 className="text-sm font-semibold text-(--color-text-primary)">
            {editor.mode === 'create'
              ? t('admin.roles.createTitle')
              : `${t('admin.roles.editTitle')} — ${editor.original.name}`}
          </h2>

          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-1 text-sm">
              <span className="text-(--color-text-muted)">{t('admin.roles.name')}</span>
              {/* 빌트인 역할의 이름은 users.role 및 코드 상수와 결합되어 있어
                  서버가 변경을 거부한다(BUILTIN_ROLE_IMMUTABLE). 입력을 열어 두면
                  사용자가 값을 바꾸고 저장 시점에야 409 를 받는다. */}
              <input
                className={INPUT_CLASS}
                value={name}
                onChange={(e) => setName(e.target.value)}
                readOnly={nameLocked}
                aria-readonly={nameLocked}
                title={nameLocked ? t('admin.roles.builtinNameLocked') : undefined}
                required
              />
              {nameLocked && (
                <p className="text-xs text-(--color-text-muted)">
                  {t('admin.roles.builtinNameLocked')}
                </p>
              )}
            </label>
            <label className="space-y-1 text-sm">
              <span className="text-(--color-text-muted)">
                {t('admin.roles.description')}
              </span>
              {/* 서버는 설명 변경을 지원하지 않는다(PUT 은 name/permissions 만). */}
              <input
                className={INPUT_CLASS}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={editor.mode === 'edit'}
                title={
                  editor.mode === 'edit'
                    ? t('admin.roles.descriptionImmutable')
                    : undefined
                }
              />
            </label>
          </div>

          <fieldset className="space-y-2">
            <legend className="text-sm text-(--color-text-muted)">
              {t('admin.roles.permissions')}
            </legend>

            {catalogQuery.isLoading && (
              <p className="inline-flex items-center gap-2 text-sm text-(--color-text-muted)">
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('common.loading')}
              </p>
            )}
            {catalogQuery.isError && (
              <p className="text-sm text-(--color-text-muted)">
                {t('admin.roles.catalogError')}
              </p>
            )}

            {matrix.rows.length > 0 && (
              <div className="overflow-x-auto rounded-md border border-(--color-border-default)">
                <table
                  className="min-w-full text-sm"
                  data-testid="permission-matrix"
                  aria-label={t('admin.roles.permissions')}
                >
                  <thead className="bg-(--color-bg-secondary)">
                    <tr>
                      <th className={TH_CLASS}>{t('admin.roles.resource')}</th>
                      {matrix.actions.map((action) => (
                        <th key={action} className={`${TH_CLASS} text-center`}>
                          {action}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {matrix.rows.map((row) => (
                      <tr key={row.resource} data-testid={`matrix-row-${row.resource}`}>
                        <td className={TD_CLASS}>{row.resource}</td>
                        {row.cells.map((key, index) => (
                          <td
                            key={matrix.actions[index]}
                            className={`${TD_CLASS} text-center`}
                            data-testid={`matrix-cell-${row.resource}-${matrix.actions[index]}`}
                          >
                            {/* 카탈로그에 없는 조합은 셀을 비운다 — 비활성 체크박스를
                                그리면 존재하지 않는 권한이 있는 것처럼 읽힌다. */}
                            {key === null ? null : (
                              <input
                                type="checkbox"
                                aria-label={key}
                                checked={selected.has(key)}
                                onChange={() => toggle(key)}
                                className="h-4 w-4 accent-blue-600"
                              />
                            )}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </fieldset>

          {/* 메뉴 노출 축. 표로 만들지 않는다 — 액션 자리에 메뉴 식별자가 오는
              단일 축이라 열이 하나뿐이고, 표로 그리면 1열 13행이 된다. */}
          {navMenus.length > 0 && (
            <fieldset className="space-y-2" data-testid="nav-menu-section">
              <legend className="text-sm text-(--color-text-muted)">
                {t('admin.roles.navMenus')}
              </legend>
              <p className="text-xs text-(--color-text-muted)">
                {t('admin.roles.navMenusHint')}
              </p>
              <div className="flex flex-wrap gap-x-4 gap-y-2 rounded-md border border-(--color-border-default) p-3">
                {navMenus.map((item) => (
                  <label
                    key={item.key}
                    data-testid={`nav-menu-item-${item.menu}`}
                    className="inline-flex items-center gap-2 text-sm text-(--color-text-primary)"
                  >
                    <input
                      type="checkbox"
                      aria-label={item.key}
                      checked={selected.has(item.key)}
                      onChange={() => toggle(item.key)}
                      className="h-4 w-4 accent-blue-600"
                    />
                    {/* 메뉴 식별자를 그대로 보인다. 행렬이 리소스·액션 원문을
                        보이는 것과 같은 규약이며, 서버가 새 메뉴를 추가해도
                        번역 키 누락으로 빈 라벨이 되지 않는다. */}
                    <span>{item.menu}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}

          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={closeEditor}
              className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={createRole.isPending || updateRole.isPending}
              className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {t('common.save')}
            </button>
          </div>
        </form>
      )}

      {deleteTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={t('admin.roles.deleteTitle')}
        >
          <div className="mx-4 w-full max-w-sm space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
            <h2 className="text-base font-semibold text-(--color-text-primary)">
              {t('admin.roles.deleteTitle')}
            </h2>
            <p className="text-sm text-(--color-text-muted)">
              {t('admin.roles.deleteConfirm').replace('{name}', deleteTarget.name)}
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
                disabled={deleteRole.isPending}
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
