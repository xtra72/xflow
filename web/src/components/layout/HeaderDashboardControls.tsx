// 헤더의 대시보드 선택/관리 컨트롤 (SPEC-DASHBOARD-004 M6 6.2 / 6.5).
//
// 목록 축(`dashboards`, `GET /api/v1/dashboards`)에서 직접 렌더한다. 구 모델은
// 본문 축(`dashboardPages`)을 렌더했는데, 그 축에는 항목별 인가 판정
// (`can_edit` · `can_grant` · `can_delete`)이 없어 컨트롤을 대시보드 단위로
// 게이팅할 수 없었다.
//
// 컨트롤은 **숨기지 않고 비활성 + 사유 툴팁**으로 둔다(SPEC-AUTH-006 §4.2).
// 컨트롤 ↔ 플래그 대응은 서버의 인가 분기(internal/api/handler/dashboard.go)와
// 1:1 이다:
//
//   이름 변경 (PATCH{name})        → can_edit
//   기본 지정 (PATCH{is_default})  → can_grant
//   삭제      (DELETE)             → can_delete
//   생성      (POST)               → 전역 dashboard.create (대시보드 단위 아님)
//
// 알려진 사실: `Header` 는 대시보드 라우트('/')에서 null 을 반환하므로 현재 이
// 컨트롤은 실행 중 앱에서 도달하지 않는다(DashboardPage 가 자체 헤더를 렌더한다).
// M7 의 대시보드 관리 화면이 이 자리를 대신할 때까지 계약만 맞춰 둔다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.11 S2, §2.12 O1)

import { useEffect, useRef, useState } from 'react';
import { Pencil, Plus, Trash2, Star } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { dashboardAccessByUid } from '@/pages/dashboard/dashboardAccess';
import { useDashboardMutations } from '@/pages/dashboard/useDashboardMutations';
import CreateDashboardDialog from '@/pages/dashboard/CreateDashboardDialog';
import { useUIStore } from '@/stores/uiStore';

/** 헤더 좌측의 대시보드 셀렉터 + 관리 버튼 묶음. */
export default function HeaderDashboardControls(): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();

  // 목록 API 축이 단일 진실이다 — 이름·기본 여부·인가 판정이 전부 여기 있다.
  const dashboards = useUIStore((s) => s.dashboards);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);

  // 세 컨트롤 모두 서버에 반영한다 — 로컬 스토어만 바꾸면 새로고침에서 잃는다.
  const { rename, setDefault, remove } = useDashboardMutations();

  const activeDashboard = dashboards.find((d) => d.uid === activeDashboardId);
  const access = dashboardAccessByUid(dashboards, activeDashboardId);
  const canCreate = hasPermission('dashboard.create');
  const isOnlyDashboard = dashboards.length <= 1;
  const isDefaultDashboard = activeDashboard?.is_default ?? false;

  const [createOpen, setCreateOpen] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [renameName, setRenameName] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);

  // 이름 편집 모드 활성화 시 포커스
  useEffect(() => {
    if (renaming) {
      requestAnimationFrame(() => {
        renameInputRef.current?.focus();
        renameInputRef.current?.select();
      });
    }
  }, [renaming]);

  /** 이름 편집 시작 */
  const handleStartRename = () => {
    setRenameName(activeDashboard?.name ?? '');
    setRenaming(true);
  };

  /** 이름 편집 확정 — 서버 PATCH{name}. 거부되면 훅이 되돌리고 알린다. */
  const handleConfirmRename = () => {
    const trimmed = renameName.trim();
    if (trimmed && trimmed !== activeDashboard?.name && access.canEdit) {
      void rename(activeDashboardId, trimmed);
    }
    setRenaming(false);
  };

  /** 이름 편집 키보드 핸들러 */
  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleConfirmRename();
    } else if (e.key === 'Escape') {
      setRenaming(false);
    }
  };

  /** 대시보드 삭제 — 서버 DELETE. 활성 대시보드였으면 M5 폴백 사슬로 착지한다. */
  const handleDelete = () => {
    if (isOnlyDashboard || !access.canDelete) return;
    const confirmed = window.confirm(
      t('header.dashboard.deleteConfirm').replace('{name}', activeDashboard?.name ?? ''),
    );
    if (confirmed) {
      void remove(activeDashboardId);
    }
  };

  const renameDisabled = renaming || !access.canEdit;
  const deleteDisabled = isOnlyDashboard || !access.canDelete;
  const defaultDisabled = isDefaultDashboard || !access.canGrant;

  return (
    <div className="flex items-center gap-2">
      {/* 대시보드 선택 / 이름 편집 */}
      {renaming ? (
        <input
          ref={renameInputRef}
          type="text"
          value={renameName}
          onChange={(e) => setRenameName(e.target.value)}
          onBlur={handleConfirmRename}
          onKeyDown={handleRenameKeyDown}
          className="rounded-md border border-blue-500 bg-(--color-bg-surface) px-2 py-1 text-sm font-semibold text-(--color-text-primary) outline-none ring-1 ring-blue-500"
          aria-label={t('header.dashboard.renameAria')}
        />
      ) : (
        <select
          value={activeDashboardId}
          onChange={(e) => setActiveDashboard(e.target.value)}
          className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm font-semibold text-(--color-text-primary)"
          aria-label={t('header.dashboard.selectAria')}
        >
          {dashboards.map((dashboard) => (
            <option key={dashboard.uid} value={dashboard.uid}>
              {dashboard.is_default ? `★ ${dashboard.name}` : dashboard.name}
            </option>
          ))}
        </select>
      )}

      {/* 이름 편집 — PATCH{name} 은 edit 인가다. */}
      <button
        type="button"
        onClick={handleStartRename}
        disabled={renameDisabled}
        aria-disabled={renameDisabled}
        className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('header.dashboard.renameAria')}
        title={access.canEdit ? t('header.dashboard.renameAria') : t('dashboard.gate.editDenied')}
      >
        <Pencil className="h-3.5 w-3.5" />
      </button>

      {/* 대시보드 추가 — 전역 dashboard.create 판정(spec.md §2.7 E1). */}
      <button
        type="button"
        onClick={() => setCreateOpen(true)}
        disabled={!canCreate}
        aria-disabled={!canCreate}
        className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('header.dashboard.add')}
        title={canCreate ? t('header.dashboard.add') : t('dashboard.gate.createDenied')}
      >
        <Plus className="h-3.5 w-3.5" />
      </button>

      {/* 대시보드 삭제 — DELETE 는 delete 인가다. */}
      <button
        type="button"
        onClick={handleDelete}
        disabled={deleteDisabled}
        aria-disabled={deleteDisabled}
        className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('header.dashboard.delete')}
        title={access.canDelete ? t('header.dashboard.delete') : t('dashboard.gate.deleteDenied')}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>

      {/* 기본 대시보드 설정 — PATCH{is_default} 는 grant 인가다. */}
      <button
        type="button"
        onClick={() => !defaultDisabled && void setDefault(activeDashboardId)}
        disabled={defaultDisabled}
        aria-disabled={defaultDisabled}
        className={cn(
          'rounded-md p-1 transition-colors',
          isDefaultDashboard
            ? 'text-yellow-500 dark:text-yellow-400'
            : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)',
          'disabled:cursor-not-allowed disabled:opacity-40',
        )}
        aria-label={t('header.dashboard.setDefault')}
        title={access.canGrant ? t('header.dashboard.setDefault') : t('dashboard.gate.grantDenied')}
      >
        <Star className={cn('h-3.5 w-3.5', isDefaultDashboard && 'fill-current')} />
      </button>

      {/* 대시보드 생성 다이얼로그 (서버 POST) */}
      <CreateDashboardDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  );
}
