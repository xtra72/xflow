// `/dashboards/admin` 라우트 제거 회귀 가드 (SPEC-DASHBOARD-004, M7 재작업).
//
// 삭제된 DashboardAdminPage.route.test.tsx 를 대체한다. 그 테스트는 구 AC-10 의
// 두 번째 요구("메뉴만 숨기면 주소창 직접 입력으로 도달한다")를 지키기 위해
// `nav.dashboard` 라우트 가드가 **등록되어 있는지**를 확인했다.
//
// 관리 화면 자체가 사라진 지금, 그 요구의 후속 형태는 더 강하다 — 가드할 라우트가
// 아니라 **라우트가 없다.** 도달 경로가 존재하지 않으므로 가드가 필요 없다.
// 이 가드는 라우트가 조용히 되살아나는 것을 막는다(되살아나면 게이팅 판정 사본이
// 다시 둘이 된다).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.5)

import type { RouteObject } from 'react-router';
import { describe, expect, it } from 'vitest';

import { appRoutes } from '@/router';

/** 라우트 트리의 모든 path 를 평탄화한다(중첩 라우트 포함). */
function collectPaths(routes: readonly RouteObject[]): string[] {
  return routes.flatMap((route) => [
    ...(route.path ? [route.path] : []),
    ...(route.children ? collectPaths(route.children) : []),
  ]);
}

describe('appRoutes — 대시보드 관리 화면 라우트 제거', () => {
  it('/dashboards/admin 라우트가 존재하지 않는다', () => {
    expect(collectPaths(appRoutes)).not.toContain('/dashboards/admin');
  });

  it('/dashboards 하위 라우트를 아예 두지 않는다', () => {
    // 관리 기능은 대시보드 편집(설정) 모드 안에 있으므로 별도 경로가 필요 없다.
    expect(collectPaths(appRoutes).filter((p) => p.startsWith('/dashboards'))).toEqual([]);
  });

  it('대시보드 보기 경로(/)는 그대로 남는다', () => {
    expect(collectPaths(appRoutes)).toContain('/');
  });
});
