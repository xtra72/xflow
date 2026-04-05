// 앱 라우터 설정.
// react-router v7의 createBrowserRouter와 React.lazy를 사용하여
// 모든 페이지 컴포넌트를 지연 로딩한다.

import { lazy, Suspense } from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router';

import AppLayout from '@/components/layout/AppLayout';
import AuthGuard from '@/components/layout/AuthGuard';
import LoadingSpinner from '@/components/layout/LoadingSpinner';

// 지연 로딩 페이지 컴포넌트
const LoginPage = lazy(() => import('@/pages/auth/LoginPage'));
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'));
const FlowListPage = lazy(() => import('@/pages/flows/FlowListPage'));
const EditorPage = lazy(() => import('@/pages/editor/EditorPage'));
const MonitoringPage = lazy(() => import('@/pages/monitoring/MonitoringPage'));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const AgentListPage = lazy(() => import('@/pages/agents/AgentListPage'));
const NodeTypesPage = lazy(() => import('@/pages/nodes/NodeTypesPage'));
const AgentTypesPage = lazy(() => import('@/pages/agents/AgentTypesPage'));
const DeviceListPage = lazy(() => import('@/pages/devices/DeviceListPage'));

/** Suspense 래퍼 - 지연 로딩 중 로딩 스피너를 표시한다 */
function SuspenseWrapper({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<LoadingSpinner />}>{children}</Suspense>;
}

/** 앱 라우트 정의 */
const router = createBrowserRouter([
  // 인증 불필요 - 로그인 페이지
  {
    path: '/login',
    element: (
      <SuspenseWrapper>
        <LoginPage />
      </SuspenseWrapper>
    ),
  },
  // 인증 필요 - AuthGuard + AppLayout으로 감싸진 보호 라우트
  {
    element: <AuthGuard />,
    children: [
      {
        element: <AppLayout />,
        children: [
          {
            path: '/',
            element: (
              <SuspenseWrapper>
                <DashboardPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/flows',
            element: (
              <SuspenseWrapper>
                <FlowListPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/editor/:flowId',
            element: (
              <SuspenseWrapper>
                <EditorPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/agents',
            element: (
              <SuspenseWrapper>
                <AgentListPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/devices',
            element: (
              <SuspenseWrapper>
                <DeviceListPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/nodes',
            element: (
              <SuspenseWrapper>
                <NodeTypesPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/agent-types',
            element: (
              <SuspenseWrapper>
                <AgentTypesPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/monitoring',
            element: (
              <SuspenseWrapper>
                <MonitoringPage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/settings',
            element: (
              <SuspenseWrapper>
                <SettingsPage />
              </SuspenseWrapper>
            ),
          },
        ],
      },
    ],
  },
]);

/**
 * 앱 라우터 컴포넌트.
 * createBrowserRouter로 생성된 라우터를 RouterProvider로 제공한다.
 */
export default function Router() {
  return <RouterProvider router={router} />;
}
