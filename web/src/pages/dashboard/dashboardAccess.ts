// 대시보드 단위 인가 판정 (SPEC-DASHBOARD-004 M6, spec.md §2.11 S2 / §2.12 O1).
//
// 판정 규칙(spec.md §2.2)은 **서버가 소유한다.** 목록 응답이 항목마다 실어 보내는
// `can_edit` · `can_delete` · `can_grant` 를 그대로 읽을 뿐, 여기서 소유권·공개범위·
// ACL·전역 권한을 재합성하지 않는다 — 재구현하면 규칙이 두 곳에 생겨 한쪽만
// 갱신되고, 그 순간 UI 와 서버 판정이 어긋난다.
//
// 역할 이름(`isAdmin`) 비교는 하지 않는다(spec.md §2.14 #2, SPEC-AUTH-006 §4.1).
// 커스텀 역할이 생기는 순간 역할 이름 열거가 성립하지 않기 때문이다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.11, §2.12)

import type { Dashboard } from '@/types/dashboard';

/** 대시보드 1장에 대한 요청자의 액션 가능 여부. */
export interface DashboardAccess {
  /** 본문 저장·이름 변경·정렬 (서버 PUT / PATCH{name,sort_order}). */
  canEdit: boolean;
  /** 공개범위·기본 대시보드 지정 (서버 PATCH{visibility,is_default}). */
  canGrant: boolean;
  /** 삭제 (서버 DELETE). */
  canDelete: boolean;
}

/** 판정 근거가 없을 때의 값 — 전부 허용. */
const PERMISSIVE: DashboardAccess = { canEdit: true, canGrant: true, canDelete: true };

/**
 * 목록 항목에서 액션 가능 여부를 읽는다.
 *
 * 항목이 없으면(목록 미도착·부팅 전 자리표시자·목록에서 사라진 uid) **허용**으로
 * 둔다. 근거 없이 선제 차단하면 부팅 중 화면이 통째로 잠기고, 실제 차단은 서버가
 * 403 으로 수행하므로 UI 가 앞서 잠글 이유가 없다.
 */
export function dashboardAccessOf(dashboard: Dashboard | undefined): DashboardAccess {
  if (!dashboard) return PERMISSIVE;
  return {
    canEdit: dashboard.can_edit,
    canGrant: dashboard.can_grant,
    canDelete: dashboard.can_delete,
  };
}

/** uid 로 목록에서 찾아 판정한다. 목록에 없으면 허용(위와 동일한 이유). */
export function dashboardAccessByUid(
  dashboards: readonly Dashboard[],
  uid: string,
): DashboardAccess {
  return dashboardAccessOf(dashboards.find((d) => d.uid === uid));
}
