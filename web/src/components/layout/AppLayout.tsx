// 앱 레이아웃 컴포넌트.
// Sidebar + Header + 메인 콘텐츠 영역(Outlet)으로 구성된 셸 레이아웃이다.

import { Outlet } from 'react-router';

import Header from '@/components/layout/Header';
import { NotificationToast } from '@/components/layout/NotificationToast';
import Sidebar from '@/components/layout/Sidebar';

/**
 * 인증된 사용자를 위한 앱 셸 레이아웃.
 * 좌측 사이드바, 상단 헤더, 스크롤 가능한 메인 콘텐츠로 구성된다.
 */
export default function AppLayout() {
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
          <Outlet />
        </main>
      </div>
    </div>
  );
}
