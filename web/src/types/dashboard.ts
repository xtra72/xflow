// 대시보드 타입.
//
// 두 계약이 공존한다 — 서로 다른 소비자를 향하므로 한쪽에 맞춰 다른 쪽을 바꾸면
// 그 소비자가 깨진다(internal/api/dto/dashboard.go 의 주석과 동일한 이유).
//
//  1) 1급 엔티티 계약 (SPEC-DASHBOARD-004 §2.3) — snake_case.
//     `/api/v1/dashboards*` · `/api/v1/dashboard-state` 가 쓴다. 로컬 대시보드의
//     조회·저장·권한 부여는 전부 이 계약을 탄다.
//  2) 레거시 스냅샷 계약 (SPEC-DASHBOARD-001) — camelCase.
//     원격 노드 프록시(`/remote/nodes/{id}/dashboards/{shared,mine}`) 전용
//     읽기 전용 shim 이다(SPEC-DASHBOARD-004 §4.4). 원격 노드는 본 SPEC 미적용
//     버전이 혼재할 수 있으므로 형상을 고정한다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.1, §2.3)

import type { DashboardLayoutItem, DashboardPageConfig, PanelConfig } from '@/stores/uiStore';

// ---------------------------------------------------------------------------
// (1) 1급 엔티티 계약 — SPEC-DASHBOARD-004
// ---------------------------------------------------------------------------

/** 대시보드 공개범위 (spec.md §2.1).
 *
 * - `'private'`: 소유자만 접근.
 * - `'shared'`:  인증된 모든 사용자가 조회(view).
 * - `'acl'`:     `dashboard_acl` 에 등재된 대상만 접근.
 */
export type DashboardVisibility = 'private' | 'shared' | 'acl';

/** 대시보드 1장의 메타데이터 + 요청자 인가 판정 결과.
 *
 * `can_edit` · `can_delete` · `can_grant` 는 서버가 spec.md §2.2 판정을 수행해
 * 실어 보낸다. 프론트가 판정 규칙을 재구현하면 한쪽만 갱신되어 UI 와 서버가
 * 어긋나므로 여기서 재계산하지 않는다.
 *
 * 목록 응답(`GET /dashboards`)은 payload 를 포함하지 않는다(spec.md §2.3).
 */
export interface Dashboard {
  uid: string;
  name: string;
  owner: string;
  visibility: DashboardVisibility;
  is_default: boolean;
  sort_order: number;
  /** 서버 부여 단조 증가. `If-Match` 로 낙관적 동시성에 사용. */
  version: number;
  created_at: number;
  updated_at: number;
  can_edit: boolean;
  can_delete: boolean;
  can_grant: boolean;
}

/** 대시보드 1장의 본문 (spec.md §2.1 payload).
 *
 * 그리드 설정 3종은 이관 시 원본이 없을 수 있어 서버가 생략할 수 있다
 * (`omitempty`). 호출자는 기본값으로 폴백한다.
 */
export interface DashboardContent {
  panels: PanelConfig[];
  layout: DashboardLayoutItem[];
  gridCols?: number;
  showGridLines?: boolean;
  refreshInterval?: number;
}

/** 단건 조회·생성·저장 응답 — 메타 + payload. */
export interface DashboardDetail extends Dashboard {
  payload: DashboardContent;
}

/** PATCH /dashboards/{uid} 요청 본문. 생략한 필드는 변경되지 않는다. */
export interface DashboardPatch {
  name?: string;
  visibility?: DashboardVisibility;
  is_default?: boolean;
  sort_order?: number;
}

/** ACL 권한 레벨. `edit` > `view` (spec.md §2.2). */
export type DashboardAclLevel = 'view' | 'edit';

/** 대시보드 권한 부여 대상 1건.
 *
 * `subject` 형식은 `user:<username>` 또는 `role:<rolename>` 이다.
 * `granted_by` · `granted_at` 은 서버가 부여하므로 요청 시 생략한다.
 */
export interface DashboardAclEntry {
  subject: string;
  level: DashboardAclLevel;
  granted_by?: string;
  granted_at?: number;
}

/** 사용자별 대시보드 UI 상태 (`/api/v1/dashboard-state`).
 *
 * 개별 대시보드에 속하지 않는 두 값을 담는다(spec.md §2.1).
 * `active_dashboard_uid` 는 존재하지 않는 uid 를 가리키면 서버가 빈 문자열로
 * 정규화한다(400 이 아니다 — spec.md §2.13 UB1 #11).
 */
export interface DashboardUserState {
  active_dashboard_uid: string;
  device_grid_layout: Record<string, DashboardLayoutItem>;
  version: number;
  updated_at: number;
}

// ---------------------------------------------------------------------------
// (2) 레거시 스냅샷 계약 — 원격 노드 프록시 전용 (읽기 전용)
// ---------------------------------------------------------------------------

/** 레거시 대시보드 스코프 (원격 노드 shim 전용).
 *
 * - `'global'`: 모든 인증된 사용자가 공유하는 운영용 대시보드.
 * - `'user'`:   사용자 본인 전용 개인 대시보드.
 */
export type DashboardScope = 'global' | 'user';

/** 레거시 스냅샷 페이로드 (원격 노드 shim 전용). */
export interface DashboardPayload {
  dashboardPages: DashboardPageConfig[];
  activeDashboardId: string;
  dashboardGridCols: number;
  dashboardShowGridLines: boolean;
  dashboardRefreshInterval: number;
  /** 디바이스 그리드 레이아웃 (deviceId -> 단일 레이아웃 항목). */
  deviceGridLayout: Record<string, DashboardLayoutItem>;
}

/** 레거시 단일 스냅샷 (원격 노드 shim 전용). */
export interface DashboardSnapshot {
  scope: DashboardScope;
  /** scope=global 시 null, scope=user 시 username. */
  owner: string | null;
  version: number;
  updatedAt: number;
  payload: DashboardPayload;
}
