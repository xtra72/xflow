// 대시보드 관리 툴바.
// 대시보드 페이지 선택/추가/삭제/기본 설정 및 패널 추가 버튼을 제공한다.

import { useState } from 'react';
import { Plus, Trash2, Star, PanelTop } from 'lucide-react';

import { useUIStore } from '@/stores/uiStore';

import CreateDashboardDialog from './CreateDashboardDialog';
import AddPanelDialog from './AddPanelDialog';

/** 대시보드 관리 툴바 */
export default function DashboardToolbar() {
  const dashboardPages = useUIStore((s) => s.dashboardPages);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const removeDashboardPage = useUIStore((s) => s.removeDashboardPage);
  const setDefaultDashboardPage = useUIStore((s) => s.setDefaultDashboardPage);
  const editMode = useUIStore((s) => s.dashboardEditMode);

  const activePage = dashboardPages.find((p) => p.id === activeDashboardId);
  const isOnlyPage = dashboardPages.length <= 1;
  const isDefaultPage = activePage?.isDefault ?? false;

  const [createOpen, setCreateOpen] = useState(false);
  const [addPanelOpen, setAddPanelOpen] = useState(false);

  /** 대시보드 삭제 (확인 대화상자 포함) */
  const handleDelete = () => {
    if (isOnlyPage) return;
    const confirmed = window.confirm(
      `"${activePage?.name}" 대시보드를 삭제하시겠습니까?`,
    );
    if (confirmed) {
      removeDashboardPage(activeDashboardId);
    }
  };

  /** 기본 대시보드로 설정 */
  const handleSetDefault = () => {
    if (isDefaultPage) return;
    setDefaultDashboardPage(activeDashboardId);
  };

  return (
    <>
      <div className="flex items-center gap-2">
        {/* 대시보드 페이지 선택 */}
        <select
          value={activeDashboardId}
          onChange={(e) => setActiveDashboard(e.target.value)}
          className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-sm text-(--color-text-primary)"
          aria-label="대시보드 선택"
        >
          {dashboardPages.map((page) => (
            <option key={page.id} value={page.id}>
              {page.isDefault ? `\u2605 ${page.name}` : page.name}
            </option>
          ))}
        </select>

        {/* 대시보드 추가 */}
        <button
          type="button"
          onClick={() => setCreateOpen(true)}
          className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
          aria-label="대시보드 추가"
          title="대시보드 추가"
        >
          <Plus className="h-4 w-4" />
        </button>

        {/* 대시보드 삭제 */}
        <button
          type="button"
          onClick={handleDelete}
          disabled={isOnlyPage}
          className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="대시보드 삭제"
          title="대시보드 삭제"
        >
          <Trash2 className="h-4 w-4" />
        </button>

        {/* 기본 대시보드 설정 */}
        <button
          type="button"
          onClick={handleSetDefault}
          disabled={isDefaultPage}
          className={`rounded-md border p-1.5 transition-colors ${
            isDefaultPage
              ? 'border-yellow-400 text-yellow-500 dark:border-yellow-500 dark:text-yellow-400'
              : 'border-(--color-border-strong) text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
          } disabled:cursor-not-allowed disabled:opacity-40`}
          aria-label="기본 대시보드로 설정"
          title="기본 대시보드로 설정"
        >
          <Star className={`h-4 w-4 ${isDefaultPage ? 'fill-current' : ''}`} />
        </button>

        {/* 패널 추가 (편집 모드에서만 표시) */}
        {editMode && (
          <button
            type="button"
            onClick={() => setAddPanelOpen(true)}
            className="inline-flex items-center gap-1 rounded-md border border-blue-300 bg-blue-50 px-2.5 py-1.5 text-sm font-medium text-blue-700 transition-colors hover:bg-blue-100 dark:border-blue-600 dark:bg-blue-900/20 dark:text-blue-400 dark:hover:bg-blue-900/30"
            aria-label="패널 추가"
          >
            <PanelTop className="h-4 w-4" />
            패널 추가
          </button>
        )}
      </div>

      {/* 대시보드 생성 다이얼로그 */}
      <CreateDashboardDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
      />

      {/* 패널 추가 다이얼로그 */}
      <AddPanelDialog
        open={addPanelOpen}
        onClose={() => setAddPanelOpen(false)}
      />
    </>
  );
}
