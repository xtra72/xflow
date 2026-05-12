// SPEC-DASHBOARD-001 v0.2.0 — 대시보드 서버 영속화 타입.
//
// 백엔드(`internal/api/dto/dashboard.go`) 의 `DashboardSnapshot` 과 1:1 매핑되며,
// payload 의 페이지/패널/레이아웃 타입은 uiStore 에서 이미 정의된 것을 재사용한다.
// (중복 정의 금지 — uiStore.ts 가 source-of-truth)
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import type { DashboardLayoutItem, DashboardPageConfig } from '@/stores/uiStore';

/** 대시보드 스코프.
 *
 * - `'global'`: 모든 인증된 사용자가 공유하는 운영용 대시보드 (admin 만 편집).
 * - `'user'`:   사용자 본인 전용 개인 대시보드 (본인만 GET/PUT/DELETE).
 */
export type DashboardScope = 'global' | 'user';

/** 클라이언트가 PUT 으로 전송하는 페이로드. version/owner/scope 등 메타는 서버가 부여. */
export interface DashboardPayload {
  dashboardPages: DashboardPageConfig[];
  activeDashboardId: string;
  dashboardGridCols: number;
  dashboardShowGridLines: boolean;
  dashboardRefreshInterval: number;
  /** 디바이스 그리드 레이아웃 (deviceId -> 단일 레이아웃 항목). */
  deviceGridLayout: Record<string, DashboardLayoutItem>;
}

/** 서버에서 받는 단일 대시보드 snapshot. */
export interface DashboardSnapshot {
  scope: DashboardScope;
  /** scope=global 시 null, scope=user 시 username. */
  owner: string | null;
  /** 서버가 부여하는 단조 증가 정수. 클라이언트 메모리 시작 시 (404 fallback) 0. */
  version: number;
  /** 서버가 부여하는 epoch ms. 404 fallback 시 0. */
  updatedAt: number;
  payload: DashboardPayload;
}
