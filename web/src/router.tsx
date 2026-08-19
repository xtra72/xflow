// 앱 라우터 설정.
// react-router v7의 createBrowserRouter와 React.lazy를 사용하여
// 모든 페이지 컴포넌트를 지연 로딩한다.
//
// SPEC-WEB-006 v0.1.0 (M1, M11): `/admin/system` 라우트 등록.
//   - AppLayout 안에 위치 (Header + Sidebar 유지).
//   - 추가 AuthGuard 로 admin role 검증 (Decision Point 5: admin-only).
//
// SPEC-AUTH-006 S1 (M3.4): `/admin` 그룹의 `requireRole="admin"` 을 권한 키
//   기반 `requirePermission` 으로 교체했다. `/admin` 하위는 요구 권한이 서로
//   다르므로(system.read / remote.read / user.read / role.read) 그룹 하나에
//   단일 권한을 걸 수 없다 — 권한별 레이아웃 라우트로 나눠 감싼다.
//
// @spec SPEC-WEB-006 v0.1.0 (M1, M11)
// @spec SPEC-AUTH-006 v0.1.0 (M3.4)

import { lazy, Suspense } from 'react';
import {
  createBrowserRouter,
  Navigate,
  RouterProvider,
  type RouteObject,
} from 'react-router';

import AppLayout from '@/components/layout/AppLayout';
import AuthGuard from '@/components/layout/AuthGuard';
import LoadingSpinner from '@/components/layout/LoadingSpinner';

// 지연 로딩 페이지 컴포넌트
const LoginPage = lazy(() => import('@/pages/auth/LoginPage'));
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'));
// 패널 생성/설정: 기존 모달을 딥링크 라우트로 승격(AppLayout 하위, Header+Sidebar 유지).
const PanelCreatePage = lazy(() => import('@/pages/dashboard/PanelCreatePage'));
const PanelSettingsPage = lazy(() => import('@/pages/dashboard/PanelSettingsPage'));
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
// SPEC-AUTH-006 M3.4: 사용자 관리. 역할 관리는 별도 화면이 아니라 이 화면의
//   '역할' 탭이다(`/admin/users?tab=roles`). 구 `/admin/roles` 는 북마크 보존용
//   리다이렉트로만 남는다.
const AdminUserManagementPage = lazy(() => import('@/pages/admin/UserManagementPage'));

/** Suspense 래퍼 - 지연 로딩 중 로딩 스피너를 표시한다 */
function SuspenseWrapper({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<LoadingSpinner />}>{children}</Suspense>;
}

/**
 * 앱 라우트 정의.
 *
 * 배열로 분리해 export 하는 이유: 테스트가 `createMemoryRouter(appRoutes)` 로
 * 실제 라우트 트리(부모-자식 중첩 관계 포함)를 그대로 마운트할 수 있어야 한다.
 * 특히 `/`, `/panels/new`, `/panels/:panelId/settings` 가 AppLayout 의 형제
 * 자식이라는 사실이 대시보드 동기화 동작에 영향을 준다(AppLayout 주석 참조).
 *
 * react-refresh 경고 억제 사유: 라우트 테이블은 컴포넌트가 아니지만 이 파일이
 * 유일한 소유자이고, 라우트 정의가 바뀌면 어차피 라우터 전체가 재마운트되므로
 * fast refresh 세분화의 이점이 없다.
 */
// eslint-disable-next-line react-refresh/only-export-components
export const appRoutes: RouteObject[] = [
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
          // 패널 생성/설정 딥링크 (대시보드 전용, 새로고침/북마크로 직접 진입 가능).
          {
            path: '/panels/new',
            element: (
              <SuspenseWrapper>
                <PanelCreatePage />
              </SuspenseWrapper>
            ),
          },
          {
            path: '/panels/:panelId/settings',
            element: (
              <SuspenseWrapper>
                <PanelSettingsPage />
              </SuspenseWrapper>
            ),
          },
          // SPEC-DASHBOARD-004 (M7 재작업): `/dashboards/admin` 라우트는 제거했다.
          //   대시보드 관리는 별도 화면이 아니라 대시보드 편집(설정) 모드 안의
          //   셀렉터가 담당한다 — 라우트가 없으므로 주소창 직접 진입 경로 자체가
          //   존재하지 않는다(구 AC-10 라우트 가드의 후속 형태).
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
          // SPEC-AUTH-006 §2.3: 설정 = system.read.
          {
            element: <AuthGuard requireMenu="nav.system" requirePermission="system.read" />,
            children: [
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
          // 관리 라우트 그룹.
          //   - 외곽 AuthGuard 가 인증을 보장한 뒤, 권한별 내부 AuthGuard 가
          //     해당 화면의 요구 권한을 추가 검증한다.
          //   - 권한이 없으면 대시보드로 리다이렉트되고 안내가 표시된다(AC-06).
          //   - `/admin` 자체는 경로 그룹일 뿐이며 element 가 없다(암묵 Outlet).
          {
            path: '/admin',
            children: [
              // SPEC-WEB-006 (M1, M11): 시스템 상태 — 시스템 설정 조회 권한.
              {
                element: <AuthGuard requireMenu="nav.system" requirePermission="system.read" />,
                children: [
                  {
                    path: 'system',
                    element: (
                      <SuspenseWrapper>
                        <SystemStatusPage />
                      </SuspenseWrapper>
                    ),
                  },
                ],
              },
              // SPEC-REMOTE-001 M9 (그룹 K): 원격 관리 화면 전체.
              //   하위 세분 권한은 범위 밖이므로 remote.read 단일 키로 묶는다
              //   (spec.md §1.3) — 서버 조회 라우트도 remote.read 로 통일돼 있다.
              {
                element: <AuthGuard requireMenu="nav.remote" requirePermission="remote.read" />,
                children: [
                  // REQ-K11/K12: 노드 관리(운영). 디렉토리 뷰(그룹 트리 + 그룹
                  //   배정) + 노드 대시보드(시스템 정보 + 운영 요약 +
                  //   Flow/Agent/Device 서브탭 = M8 통합 제어 재사용).
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
                  // REQ-K15: 등록 관리(온보딩).
                  //   토큰 관리 + 노드 등록 관리(승인 큐/사전 등록).
                  {
                    path: 'remote/enrollment',
                    element: (
                      <SuspenseWrapper>
                        <EnrollmentManagementPage />
                      </SuspenseWrapper>
                    ),
                  },
                  // 릴리스 저장소(관리 서버 호스팅 프로그램 이미지): 아키텍처별
                  //   xflowd 이미지 업로드/조회/삭제. 업데이트 소스를 이 서버로
                  //   지정한 노드가 자기 아키텍처 이미지를 자동 다운로드한다.
                  {
                    path: 'remote/releases',
                    element: (
                      <SuspenseWrapper>
                        <ReleaseStorePage />
                      </SuspenseWrapper>
                    ),
                  },
                  // REQ-K16, OQ-K7: 구 제어 셀렉터 딥링크 보존 —
                  //   /admin/remote/control → 노드 관리로 리다이렉트(북마크 보존).
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
              // SPEC-AUTH-006 §2.3 (M3.4): 사용자 관리 — user.read.
              //   역할 관리는 이 화면의 '역할' 탭이며 노출은 nav.role 이 가른다.
              {
                element: <AuthGuard requireMenu="nav.user" requirePermission="user.read" />,
                children: [
                  {
                    path: 'users',
                    element: (
                      <SuspenseWrapper>
                        <AdminUserManagementPage />
                      </SuspenseWrapper>
                    ),
                  },
                ],
              },
              // 구 역할 관리 화면 딥링크 보존 — /admin/roles → 사용자 관리의
              //   역할 탭으로 리다이렉트(북마크 보존). `/admin/remote/control`
              //   → `/admin/remote` 와 같은 방식이다. 기존 게이트(nav.role +
              //   role.read)는 그대로 두어, 역할을 볼 수 없는 사용자가 이 경로로
              //   우회 진입하지 않게 한다.
              {
                element: <AuthGuard requireMenu="nav.role" requirePermission="role.read" />,
                children: [
                  {
                    path: 'roles',
                    element: <Navigate to="/admin/users?tab=roles" replace />,
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
];

const router = createBrowserRouter(appRoutes);

/**
 * 앱 라우터 컴포넌트.
 * createBrowserRouter로 생성된 라우터를 RouterProvider로 제공한다.
 */
export default function Router() {
  return <RouterProvider router={router} />;
}
