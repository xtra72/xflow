// 설정(편집) 모드의 대시보드 선택 + 관리 컨트롤 (SPEC-DASHBOARD-004, M7 재작업).
//
// 별도 '대시보드 관리' 메뉴/화면(DashboardAdminPage)을 없애고 그 기능을 편집 모드
// 안으로 들여왔다. 관리 대상 대시보드를 고르는 행위와 관리하는 행위가 한 목록에서
// 일어나므로, "어느 대시보드를 관리 중인가" 를 사용자가 화면 두 곳에서 맞춰볼 필요가
// 없다.
//
// 목록 축은 uiStore 의 `dashboards` **하나**다(`GET /api/v1/dashboards` 를 AppLayout 의
// useDashboardSync 가 채운다). 활성 대시보드 1장이 아니라 **볼 수 있는 전부**를 싣는다 —
// 비활성 대시보드를 빼면 여기서 다른 대시보드를 고를 수 없고, 편집 불가 대시보드를 빼면
// "왜 목록에 없는가" 를 사용자가 알 길이 없다. 편집할 수 없는 항목도 행은 남기고 컨트롤만
// 비활성한다.
//
// 두 게이팅 축을 구분한다 — 섞으면 규칙이 무너진다.
//   1) 메뉴 축(`nav.dashboard`): 관리 어포던스를 **아예 렌더할지**. 메뉴 클래스 판정이므로
//      부재 = 미노출이다(SPEC-AUTH-006 E2). 보기(선택)는 이 축과 무관하게 남는다.
//   2) 대시보드 단위 인가(`can_edit`/`can_grant`/`can_delete`): 렌더된 컨트롤을 **비활성**할지.
//      숨기지 않는다(spec.md §2.11 S2, SPEC-AUTH-006 §4.2) — 사라진 버튼은 "기능이 없다"로
//      읽히지만 비활성 버튼은 관리자에게 권한을 요청할 여지를 남긴다.
//
// 변경 경로는 M6 의 useDashboardMutations / useCreateDashboard 를 그대로 쓴다. 두 번째
// 변경 경로를 만들면 낙관적 반영·롤백·403 재조회 정책이 갈라진다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.7, §2.9, §2.11, §2.12)

import { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, KeyRound, Pencil, Plus, Star, Trash2, X } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { useUIStore } from '@/stores/uiStore';
import type { Dashboard, DashboardVisibility } from '@/types/dashboard';

import { dashboardAccessOf } from './dashboardAccess';
import DashboardAclPanel from './DashboardAclPanel';
import { useCreateDashboard } from './useCreateDashboard';
import { useDashboardMutations } from './useDashboardMutations';

/** 공개범위 선택지 — 서버 CHECK 제약(spec.md §2.1)과 동일한 3모드다. */
const VISIBILITIES: readonly DashboardVisibility[] = ['private', 'shared', 'acl'];

const ICON_BUTTON_CLASS =
  'shrink-0 rounded p-1 transition-colors disabled:cursor-not-allowed disabled:opacity-40';

const SELECT_CLASS =
  'shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-0.5 ' +
  'text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none ' +
  'disabled:cursor-not-allowed disabled:opacity-40';

interface DashboardSettingsSelectorProps {
  /** 헤더에 표시할 활성 대시보드 이름. 본문 축(dashboardPages)이 소유한다. */
  activeName: string;
}

/**
 * 편집 모드 헤더 좌측 — 대시보드 타이틀(드롭다운) + 편집 모드 뱃지.
 *
 * 드롭다운의 각 행이 곧 선택지이자 관리 단위다. 행 클릭 = 선택, 행 우측 아이콘 = 관리.
 */
export default function DashboardSettingsSelector({
  activeName,
}: DashboardSettingsSelectorProps): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission, canSeeMenu } = usePermission();

  const dashboards = useUIStore((s) => s.dashboards);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);

  // 메뉴 축 — 관리 어포던스의 노출 여부. 보기(행 선택)는 이 축과 무관하다.
  const canManage = canSeeMenu('nav.dashboard');
  // 생성은 대시보드 단위가 아니라 전역 권한 판정이다(spec.md §2.7 E1) —
  // 아직 존재하지 않는 대시보드에는 can_* 를 붙일 대상이 없다.
  const canCreate = hasPermission('dashboard.create');

  const { create, isCreating } = useCreateDashboard();
  const { rename, setDefault, setVisibility, remove, isMutating } = useDashboardMutations();

  const [open, setOpen] = useState(false);
  const [renamingUid, setRenamingUid] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Dashboard | null>(null);
  const [aclTarget, setAclTarget] = useState<Dashboard | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);

  // 바깥 클릭 닫기. 대화상자(삭제 확인·권한 부여)는 이 루트 안에 그리므로
  // 그 안의 클릭이 "바깥 클릭" 으로 읽히지 않는다.
  useEffect(() => {
    if (!open) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [open]);

  function startRename(dashboard: Dashboard): void {
    setRenamingUid(dashboard.uid);
    setRenameValue(dashboard.name);
  }

  function cancelRename(): void {
    setRenamingUid(null);
    setRenameValue('');
  }

  /** 이름 확정 — 서버가 거부하면 훅이 낙관적 반영을 되돌리고 사유를 알린다. */
  async function confirmRename(uid: string): Promise<void> {
    await rename(uid, renameValue);
    cancelRename();
  }

  async function handleDelete(): Promise<void> {
    if (!deleteTarget) return;
    // 삭제는 낙관적으로 지우지 않는다(useDashboardMutations 주석 참조).
    await remove(deleteTarget.uid);
    setDeleteTarget(null);
  }

  async function handleCreate(): Promise<void> {
    // 서버가 발급한 uid 를 그대로 쓴다 — uid 를 지어내면 서버 행과 어긋난다.
    const created = await create(t('dashboard.header.newDashboardName'));
    if (created) setOpen(false);
  }

  return (
    <div className="relative flex items-center gap-3" ref={rootRef}>
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-label={t('header.dashboard.selectAria')}
        aria-expanded={open}
        data-testid="dashboard-settings-selector-toggle"
        className="inline-flex items-center gap-1.5 rounded-lg p-1 text-base font-semibold text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
      >
        {activeName}
        <ChevronDown
          className={`h-4 w-4 text-(--color-text-muted) transition-transform ${open ? 'rotate-180' : ''}`}
        />
      </button>
      <span className="inline-flex items-center rounded-md bg-amber-50 px-2.5 py-1 text-[11px] font-semibold text-amber-500 dark:bg-amber-900/30 dark:text-amber-400">
        {t('dashboard.editMode')}
      </span>

      {open && (
        <div
          data-testid="dashboard-settings-list"
          className="absolute left-0 top-full z-50 mt-1 w-[460px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg"
        >
          {canManage && (
            <>
              <div className="px-4 py-2.5">
                <span className="text-[13px] font-semibold text-(--color-text-primary)">
                  {t('dashboard.manage.heading')}
                </span>
              </div>
              <div className="h-px bg-(--color-border-default)" />
            </>
          )}

          <div className="flex max-h-[60vh] flex-col gap-0.5 overflow-y-auto p-1.5">
            {dashboards.length === 0 && (
              <p
                data-testid="dashboard-settings-empty"
                className="px-3 py-2 text-sm text-(--color-text-muted)"
              >
                {t('dashboard.manage.empty')}
              </p>
            )}

            {dashboards.map((dashboard) => {
              // 행마다 그 대시보드의 판정을 쓴다 — 활성 대시보드 하나로 목록 전체를
              // 잠그면 편집 가능한 대시보드까지 함께 잠긴다.
              const access = dashboardAccessOf(dashboard);
              const isRenaming = renamingUid === dashboard.uid;
              return (
                <div
                  key={dashboard.uid}
                  data-testid={`dashboard-settings-row-${dashboard.uid}`}
                  className={`flex items-center gap-1.5 rounded-md px-3 py-2 text-sm transition-colors hover:bg-(--color-bg-elevated) ${
                    dashboard.uid === activeDashboardId
                      ? 'bg-(--color-bg-elevated) font-medium'
                      : ''
                  }`}
                >
                  {isRenaming ? (
                    <>
                      <input
                        type="text"
                        aria-label={t('dashboard.manage.renameInputAria')}
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') void confirmRename(dashboard.uid);
                          if (e.key === 'Escape') cancelRename();
                        }}
                        maxLength={64}
                        autoFocus
                        className="min-w-0 flex-1 rounded border border-blue-400 bg-(--color-bg-surface) px-1 py-0.5 text-sm outline-none focus:ring-1 focus:ring-blue-400"
                      />
                      <button
                        type="button"
                        onClick={() => void confirmRename(dashboard.uid)}
                        aria-label={t('dashboard.manage.renameSave')}
                        title={t('dashboard.manage.renameSave')}
                        data-testid={`dashboard-settings-rename-save-${dashboard.uid}`}
                        className={`${ICON_BUTTON_CLASS} text-(--color-text-muted) hover:text-(--color-text-primary)`}
                      >
                        <Check className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={cancelRename}
                        aria-label={t('dashboard.manage.renameCancel')}
                        title={t('dashboard.manage.renameCancel')}
                        data-testid={`dashboard-settings-rename-cancel-${dashboard.uid}`}
                        className={`${ICON_BUTTON_CLASS} text-(--color-text-muted) hover:text-(--color-text-primary)`}
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      onClick={() => {
                        setActiveDashboard(dashboard.uid);
                        setOpen(false);
                      }}
                      data-testid={`dashboard-settings-select-${dashboard.uid}`}
                      className="min-w-0 flex-1 truncate text-left text-(--color-text-primary)"
                    >
                      {dashboard.name}
                    </button>
                  )}

                  {/* 관리 권한이 없으면 기본 대시보드 표식만 읽기 전용으로 남긴다 —
                      정보는 잃지 않되 조작 수단은 주지 않는다. */}
                  {!canManage && dashboard.is_default && (
                    <Star
                      className="h-3.5 w-3.5 shrink-0 fill-current text-yellow-500"
                      aria-hidden="true"
                      data-testid={`dashboard-settings-default-badge-${dashboard.uid}`}
                    />
                  )}

                  {canManage && !isRenaming && (
                    <>
                      {/* 기본 지정 → 서버가 PATCH{is_default} 를 grant 인가로 검사한다. */}
                      <button
                        type="button"
                        onClick={() => void setDefault(dashboard.uid)}
                        disabled={!access.canGrant}
                        aria-disabled={!access.canGrant}
                        aria-label={`${t('dashboard.header.setDefault')} ${dashboard.name}`}
                        data-testid={`dashboard-settings-default-${dashboard.uid}`}
                        title={
                          !access.canGrant
                            ? t('dashboard.gate.grantDenied')
                            : dashboard.is_default
                              ? t('dashboard.header.defaultTitle')
                              : t('dashboard.header.setDefault')
                        }
                        className={`${ICON_BUTTON_CLASS} ${
                          dashboard.is_default
                            ? 'text-yellow-500'
                            : 'text-(--color-text-muted) hover:text-yellow-400'
                        }`}
                      >
                        <Star
                          className={`h-3.5 w-3.5 ${dashboard.is_default ? 'fill-current' : ''}`}
                        />
                      </button>

                      {/* 이름 변경 → 서버가 PATCH{name} 을 edit 인가로 검사한다. */}
                      <button
                        type="button"
                        onClick={() => startRename(dashboard)}
                        disabled={!access.canEdit}
                        aria-disabled={!access.canEdit}
                        aria-label={`${t('dashboard.manage.rename')} ${dashboard.name}`}
                        data-testid={`dashboard-settings-rename-${dashboard.uid}`}
                        title={
                          access.canEdit
                            ? t('dashboard.manage.rename')
                            : t('dashboard.gate.editDenied')
                        }
                        className={`${ICON_BUTTON_CLASS} text-(--color-text-muted) hover:text-(--color-text-primary)`}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>

                      {/* 공개범위 → 서버가 PATCH{visibility} 를 grant 인가로 검사한다. */}
                      <select
                        aria-label={`${t('dashboard.manage.visibilityAria')} ${dashboard.name}`}
                        data-testid={`dashboard-settings-visibility-${dashboard.uid}`}
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
                        className={SELECT_CLASS}
                      >
                        {VISIBILITIES.map((value) => (
                          <option key={value} value={value}>
                            {t(`dashboard.visibilityLabel.${value}`)}
                          </option>
                        ))}
                      </select>

                      {/* 권한 부여 패널은 grant 인가에서만 열린다(spec.md §2.2). */}
                      <button
                        type="button"
                        onClick={() => setAclTarget(dashboard)}
                        disabled={!access.canGrant}
                        aria-disabled={!access.canGrant}
                        aria-label={`${t('dashboard.manage.manageAcl')} ${dashboard.name}`}
                        data-testid={`dashboard-settings-acl-${dashboard.uid}`}
                        title={
                          access.canGrant
                            ? t('dashboard.manage.manageAcl')
                            : t('dashboard.gate.grantDenied')
                        }
                        className={`${ICON_BUTTON_CLASS} text-(--color-text-muted) hover:text-(--color-text-primary)`}
                      >
                        <KeyRound className="h-3.5 w-3.5" />
                      </button>

                      {/* 삭제 → 서버가 DELETE 를 delete 인가로 검사한다. 활성 대시보드를
                          지우면 M5 의 폴백 사슬이 착지점을 정한다. */}
                      <button
                        type="button"
                        onClick={() => setDeleteTarget(dashboard)}
                        disabled={!access.canDelete}
                        aria-disabled={!access.canDelete}
                        aria-label={`${t('header.dashboard.delete')} ${dashboard.name}`}
                        data-testid={`dashboard-settings-delete-${dashboard.uid}`}
                        title={
                          access.canDelete
                            ? t('header.dashboard.delete')
                            : t('dashboard.gate.deleteDenied')
                        }
                        className={`${ICON_BUTTON_CLASS} text-(--color-text-muted) hover:text-red-500`}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </>
                  )}
                </div>
              );
            })}
          </div>

          {canManage && (
            <div className="border-t border-(--color-border-default)">
              <button
                type="button"
                onClick={() => void handleCreate()}
                disabled={!canCreate || isCreating}
                aria-disabled={!canCreate || isCreating}
                data-testid="dashboard-settings-create"
                title={canCreate ? undefined : t('dashboard.gate.createDenied')}
                className={`flex w-full items-center justify-center gap-2 px-4 py-2.5 text-sm transition-colors ${
                  !canCreate || isCreating
                    ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                    : 'text-blue-600 hover:bg-(--color-bg-elevated) dark:text-blue-400'
                }`}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('dashboard.manage.create')}
              </button>
            </div>
          )}
        </div>
      )}

      {/* 삭제 확인 — 되돌릴 수 없으므로 한 번 더 받는다. */}
      {deleteTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={t('dashboard.manage.deleteTitle')}
        >
          <div className="mx-4 w-full max-w-sm space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
            <h2 className="text-base font-semibold text-(--color-text-primary)">
              {t('dashboard.manage.deleteTitle')}
            </h2>
            <p className="text-sm text-(--color-text-muted)">
              {t('dashboard.manage.deleteConfirm').replace('{name}', deleteTarget.name)}
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
