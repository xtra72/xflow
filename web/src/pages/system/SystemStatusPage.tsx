// SPEC-WEB-006 v0.1.0 (M2, M3, M5, M6, M7, M8, M9) — System Status Page.
//
// `/admin/system` 라우트의 페이지 컴포넌트. (라우팅 wire-up 은 Phase F 가 담당)
//
// 페이지 책임:
//   - useSystemVersion 훅으로 60초 폴링 (Phase A 제공)
//   - 로딩 / 에러 / 성공 상태 분기 렌더링
//   - SystemVersionCard 에 데이터 + 콜백 주입
//   - useUpdateCheck mutation 으로 명시적 폴링 트리거
//     - 성공: useSystemVersion 쿼리 무효화 → 즉시 재페칭
//     - 실패: mapUpdateError 로 한글 메시지 변환 → toast 알림
//   - Phase D: SystemVersionCard 의 "업데이트 시작" 버튼 → UpdateDialog 오픈
//
// Phase E 가 추가할 영역:
//   - update_available false→true 전환 토스트 (헤더 인디케이터와 연계)
//
// @spec SPEC-WEB-006 v0.1.0 (M2, M3, M5, M6, M7, M8, M9)

import { useCallback, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { SystemVersionCard } from '@/components/system/SystemVersionCard';
import { UpdateDialog } from '@/components/system/UpdateDialog';
import { mapUpdateError } from '@/lib/errors/updaterErrorMapper';
import { useSystemVersion, useUpdateCheck } from '@/services/api/systemUpdate';
import { useUIStore } from '@/stores/uiStore';

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemStatusPage() {
  const versionQuery = useSystemVersion();
  const checkMutation = useUpdateCheck();
  const queryClient = useQueryClient();
  const addNotification = useUIStore((s) => s.addNotification);
  // Phase D: UpdateDialog 의 open 상태를 페이지에서 관리한다.
  const [updateOpen, setUpdateOpen] = useState(false);

  // Check 버튼 핸들러 — mutation 트리거 + 결과 후처리.
  const handleCheck = useCallback(() => {
    checkMutation.mutate(undefined, {
      onSuccess: () => {
        // 채널 폴링 직후 GET /system/version 재페칭 → 최신 latest_version 반영.
        queryClient.invalidateQueries({ queryKey: ['system', 'version'] });
        addNotification({
          type: 'success',
          message: '업데이트 채널을 확인했습니다.',
        });
      },
      onError: (err) => {
        const mapped = mapUpdateError(err);
        addNotification({
          type: 'error',
          message: mapped.userMessage,
        });
      },
    });
  }, [checkMutation, queryClient, addNotification]);

  // dataUpdatedAt (epoch ms, 0 = 미수신) → Date 변환.
  const lastCheckedAt = useMemo(() => {
    if (!versionQuery.dataUpdatedAt) return undefined;
    return new Date(versionQuery.dataUpdatedAt);
  }, [versionQuery.dataUpdatedAt]);

  return (
    <div className="mx-auto max-w-3xl space-y-6 p-6">
      <header data-testid="system-status-header">
        <h1 className="text-2xl font-semibold text-(--color-text-primary)">
          시스템 상태
        </h1>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          xflowd 데몬의 버전 정보와 업데이트 상태를 확인합니다.
        </p>
      </header>

      {versionQuery.isLoading ? <LoadingSkeleton /> : null}

      {versionQuery.isError ? (
        <ErrorState
          error={versionQuery.error}
          onRetry={() => versionQuery.refetch()}
        />
      ) : null}

      {versionQuery.data ? (
        <SystemVersionCard
          version={versionQuery.data}
          lastCheckedAt={lastCheckedAt}
          onCheck={handleCheck}
          isChecking={checkMutation.isPending}
          // Phase D: update_available=true 일 때 SystemVersionCard 가
          // "업데이트 시작" 버튼을 노출하도록 핸들러 주입.
          onUpdate={() => setUpdateOpen(true)}
        />
      ) : null}

      {/* Phase D — UpdateDialog: VersionInfo 가 있을 때만 마운트. */}
      {versionQuery.data ? (
        <UpdateDialog
          open={updateOpen}
          onClose={() => setUpdateOpen(false)}
          version={versionQuery.data}
        />
      ) : null}

      {/* 향후 영역: 업데이트 이력 / changelog / 백업 정보 */}
      <section
        data-testid="system-status-future"
        className="rounded-lg border border-dashed border-(--color-border) p-6 text-center text-sm text-(--color-text-muted)"
      >
        업데이트 이력 및 changelog — 곧 추가 예정
      </section>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub-components
// ─────────────────────────────────────────────────────────────────────

function LoadingSkeleton() {
  return (
    <div
      data-testid="system-status-loading"
      className="space-y-3 rounded-lg border border-(--color-border) bg-(--color-bg-elevated) p-6"
      aria-busy="true"
      aria-label="시스템 정보를 불러오는 중"
    >
      <div className="h-5 w-1/3 animate-pulse rounded bg-(--color-bg-hover)" />
      <div className="h-10 w-1/2 animate-pulse rounded bg-(--color-bg-hover)" />
      <div className="h-4 w-2/3 animate-pulse rounded bg-(--color-bg-hover)" />
      <div className="h-4 w-1/2 animate-pulse rounded bg-(--color-bg-hover)" />
    </div>
  );
}

function ErrorState({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  // mapUpdateError 가 unknown → 한글 메시지로 안전 변환.
  const mapped = mapUpdateError(error);
  return (
    <div
      data-testid="system-status-error"
      role="alert"
      className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-200"
    >
      <p className="font-medium">시스템 정보를 불러오지 못했습니다</p>
      <p className="mt-1">{mapped.userMessage}</p>
      <button
        type="button"
        data-testid="system-status-retry-button"
        onClick={onRetry}
        className="mt-3 inline-flex items-center gap-1 rounded border border-red-300 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100 dark:border-red-700 dark:bg-red-900 dark:text-red-100 dark:hover:bg-red-800"
      >
        다시 시도
      </button>
    </div>
  );
}

export default SystemStatusPage;
