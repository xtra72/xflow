// 앱 라우터 설정.
// react-router v7의 createBrowserRouter와 React.lazy를 사용하여
// 모든 페이지 컴포넌트를 지연 로딩한다.
//
// SPEC-WEB-006 v0.1.0 (M1, M11): `/admin/system` 라우트 등록.
//   - AppLayout 안에 위치 (Header + Sidebar 유지).
//   - 추가 AuthGuard 로 admin role 검증 (Decision Point 5: admin-only).
//
// @spec SPEC-WEB-006 v0.1.0 (M1, M11)

import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router';

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
// SPEC-SCHEDULE-VIEW-001 M5: 스케줄 뷰(에이전트별 관리 + 실행 로그). 전체 인증
//   사용자 접근(RD-5, AC-11/AC-17) — 추가 admin AuthGuard 없이 AppLayout 하위 라우트.
const SchedulesPage = lazy(() => import('@/pages/schedules/SchedulesPage'));
const SystemStatusPage = lazy(() =>
  import('@/pages/system/SystemStatusPage').then((m) => ({
    default: m.SystemStatusPage,
  })),
);
// SPEC-REMOTE-001 M9 (그룹 K): 원격 관리 IA 재편.
//   노드 관리(운영, 디렉토리+대시보드) + 등록 관리(온보딩, 토큰+승인).
//   기존 RemoteNodesPage(관리 노드) + RemoteControlPage(원격 노드 제어)를 대체한다.
const NodeManagementPage = lazy(() => import('@/pages/remote/NodeManagementPage'));
const GroupManagementPage = lazy(
  () => import('@/pages/remote/GroupManagementPage'),
);
const EnrollmentManagementPage = lazy(
  () => import('@/pages/remote/EnrollmentManagementPage'),
);
// 릴리스 저장소(관리 서버 호스팅 프로그램 이미지): 아키텍처별 xflowd 이미지 관리.
const ReleaseStorePage = lazy(() => import('@/pages/remote/ReleaseStorePage'));

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
            path: '/schedules',
            element: (
              <SuspenseWrapper>
                <SchedulesPage />
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
          // SPEC-WEB-006 v0.1.0 (M1, M11): admin 전용 라우트 그룹.
          //   - 외곽 AuthGuard 가 인증을 보장한 뒤, 내부 AuthGuard 가
          //     role==='admin' 까지 추가 검증한다.
          //   - 일치하지 않으면 ForbiddenPage 가 렌더된다.
          {
            path: '/admin',
            element: <AuthGuard requireRole="admin" />,
            children: [
              {
                path: 'system',
                element: (
                  <SuspenseWrapper>
                    <SystemStatusPage />
                  </SuspenseWrapper>
                ),
              },
              // SPEC-REMOTE-001 M9 (그룹 K, REQ-K11/K12): 노드 관리(운영).
              //   디렉토리 뷰(그룹 트리 + 그룹 배정) + 노드 대시보드(시스템 정보 +
              //   운영 요약 + Flow/Agent/Device 서브탭 = M8 통합 제어 재사용).
              {
                path: 'remote',
                element: (
                  <SuspenseWrapper>
                    <NodeManagementPage />
                  </SuspenseWrapper>
                ),
              },
              // 그룹 관리(서브 페이지): 좌측 그룹 트리 + 우측 그룹 제어/노드 상세.
              {
                path: 'remote/groups',
                element: (
                  <SuspenseWrapper>
                    <GroupManagementPage />
                  </SuspenseWrapper>
                ),
              },
              // SPEC-REMOTE-001 M9 (그룹 K, REQ-K15): 등록 관리(온보딩).
              //   토큰 관리 + 노드 등록 관리(승인 큐/사전 등록).
              {
                path: 'remote/enrollment',
                element: (
                  <SuspenseWrapper>
                    <EnrollmentManagementPage />
                  </SuspenseWrapper>
                ),
              },
              // 릴리스 저장소(관리 서버 호스팅 프로그램 이미지): 아키텍처별 xflowd
              //   이미지 업로드/조회/삭제. 업데이트 소스를 이 서버로 지정한 노드가
              //   자기 아키텍처 이미지를 자동 다운로드한다.
              {
                path: 'remote/releases',
                element: (
                  <SuspenseWrapper>
                    <ReleaseStorePage />
                  </SuspenseWrapper>
                ),
              },
              // SPEC-REMOTE-001 M9 (그룹 K, REQ-K16, OQ-K7): 구 제어 셀렉터 딥링크
              //   보존 — /admin/remote/control → 노드 관리로 리다이렉트(북마크 보존).
              {
                path: 'remote/control',
                element: <Navigate to="/admin/remote" replace />,
              },
              // SPEC-REMOTE-001 M7 (그룹 I, REQ-I08): 원격 플로우를 기존 시각
              // 편집기(EditorPage)로 열어 대상 노드에 저장(PATCH/POST 명령 전파).
              //   .../flows/new       → 신규 생성(POST, 노드 채번 id)
              //   .../flows/:flowId/edit → 기존 수정(PATCH)
              {
                path: 'remote/nodes/:instanceId/flows/new',
                element: (
                  <SuspenseWrapper>
                    <EditorPage />
                  </SuspenseWrapper>
                ),
              },
              {
                path: 'remote/nodes/:instanceId/flows/:flowId/edit',
                element: (
                  <SuspenseWrapper>
                    <EditorPage />
                  </SuspenseWrapper>
                ),
              },
            ],
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
