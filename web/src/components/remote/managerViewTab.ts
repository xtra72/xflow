// 관리자 뷰 서브탭 식별자와 URL `?tab=` 파싱 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M06).
//
// 컴포넌트 파일(ManagerViewTopBar.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

/** 관리자 뷰 서브탭 식별자(NodeDashboard 와 동일 집합). */
export type ManagerViewTab = 'overview' | 'flows' | 'agents' | 'devices' | 'dashboard';

/** 유효한 서브탭 식별자 집합(URL 파라미터 검증용). */
export const MANAGER_VIEW_TABS: readonly ManagerViewTab[] = [
  'overview',
  'flows',
  'agents',
  'devices',
  'dashboard',
];

/** URL `?tab=` 원시 값을 ManagerViewTab 으로 파싱한다(미지정/무효 → overview). */
export function parseManagerViewTab(raw: string | null): ManagerViewTab {
  return MANAGER_VIEW_TABS.includes(raw as ManagerViewTab)
    ? (raw as ManagerViewTab)
    : 'overview';
}
