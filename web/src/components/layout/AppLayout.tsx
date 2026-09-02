// 앱 레이아웃 컴포넌트.
// Sidebar + Header + 메인 콘텐츠 영역(Outlet)으로 구성된 셸 레이아웃이다.
//
// SPEC-WEB-006 v0.1.0 (M4): useUpdateAvailableNotification 을 여기서 호출해
// 라우트와 무관하게 false→true 전이 시 토스트가 발화하도록 한다.
// (Header 는 대시보드 라우트에서 null 을 반환하므로 헤더 안에서 호출하면
//  대시보드 경로에서 토스트가 발화하지 않는 문제가 발생한다.)
//
// SPEC-DASHBOARD-001 v0.2.0: useDashboardSync 도 같은 이유로 여기서 호출한다.
// `/`(DashboardPage), `/panels/new`, `/panels/:panelId/settings` 는 모두 이
// 레이아웃의 형제 자식 라우트이므로, 훅을 DashboardPage 안에서 호출하면 패널
// 추가/설정 라우트로 이동하는 순간 훅이 언마운트되어
//   (1) 저장 PUT 을 발사하는 store 구독과 debounce 타이머가 사라지고,
//   (2) 대시보드로 복귀할 때 부팅 GET 이 다시 실행되어 서버 snapshot 이
//       로컬 변경을 덮어쓴다.
// 훅을 레이아웃으로 끌어올리면 라우트 전환과 무관하게 구독이 유지된다.
// 상태(pendingSync 등)는 DashboardSyncContext 로 하위 라우트에 전달한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M4), SPEC-DASHBOARD-001 v0.2.0

import { useMemo } from 'react';
import { Outlet } from 'react-router';

import Header from '@/components/layout/Header';
import { NotificationToast } from '@/components/layout/NotificationToast';
import Sidebar from '@/components/layout/Sidebar';
import { DashboardSyncContext } from '@/contexts/DashboardSyncContext';
import { useDashboardSync } from '@/hooks/useDashboardSync';
import { useUpdateAvailableNotification } from '@/hooks/useUpdateAvailableNotification';

/**
 * 인증된 사용자를 위한 앱 셸 레이아웃.
 * 좌측 사이드바, 상단 헤더, 스크롤 가능한 메인 콘텐츠로 구성된다.
 */
export default function AppLayout() {
  // 시스템 업데이트 가용성 토스트 트리거 (사이드 이펙트 전용 훅).
  useUpdateAvailableNotification();

  // 대시보드 snapshot 동기화 (앱 전체에서 단 한 번만 호출되는 지점).
  const { isLoading, error, pendingSync } = useDashboardSync();
  // 훅은 매 렌더마다 새 객체를 반환하므로, 값이 실제로 바뀔 때만
  // 컨텍스트 소비자가 재렌더되도록 필드 단위로 메모이즈한다.
  const syncStatus = useMemo(
    () => ({ isLoading, error, pendingSync }),
    [isLoading, error, pendingSync],
  );

  return (
    <div className="flex h-screen overflow-hidden bg-(--color-bg-primary)">
      {/* 알림 토스트 */}
      <NotificationToast />

      {/* 사이드바 */}
      <Sidebar />

      {/* 메인 영역: 헤더 + 콘텐츠 */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <Header />

        {/* 스크롤 가능한 메인 콘텐츠 */}
        <main className="flex-1 overflow-y-auto p-6">
          <DashboardSyncContext.Provider value={syncStatus}>
            <Outlet />
          </DashboardSyncContext.Provider>
        </main>
      </div>
    </div>
  );
}
