// 대시보드 동기화 상태 컨텍스트.
//
// `useDashboardSync()` 는 앱 셸(AppLayout)에서 단 한 번만 호출된다 —
// 대시보드 라우트(`/`)에서 호출하면 `/panels/new` 등 형제 라우트로 이동할 때
// DashboardPage 가 언마운트되어 (1) 저장 PUT 을 발사하는 store 구독이 끊기고
// (2) 복귀 시 부팅 GET 이 다시 서버 snapshot 을 덮어써 변경이 유실된다.
//
// 훅은 한 곳에서만 호출해야 하므로(중복 호출 시 부팅 GET/PUT 이 두 번 발생),
// 상태는 컨텍스트로 내려보내고 소비자는 `useDashboardSyncStatus()` 로 읽는다.
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import { createContext, useContext } from 'react';

import type { DashboardSyncStatus } from '@/hooks/useDashboardSync';

/**
 * Provider 가 없을 때의 기본값(무동작).
 *
 * AppLayout 밖에서 렌더되는 경우(단위 테스트에서 페이지를 단독 렌더하는 경우 등)
 * 동기화 인디케이터만 표시되지 않을 뿐 페이지는 정상 렌더된다.
 * 예외를 던지지 않는 이유: 인디케이터는 부가 UI 이며, Provider 부재로 페이지
 * 전체가 깨지는 편이 더 나쁘다.
 */
export const IDLE_DASHBOARD_SYNC_STATUS: DashboardSyncStatus = {
  isLoading: false,
  error: null,
  pendingSync: false,
};

/** AppLayout 이 제공하는 대시보드 동기화 상태. */
export const DashboardSyncContext = createContext<DashboardSyncStatus>(
  IDLE_DASHBOARD_SYNC_STATUS,
);

/**
 * 대시보드 동기화 상태를 읽는다 (부팅 로딩 / 에러 / 미저장 변경 여부).
 *
 * 이 훅은 상태를 "읽기"만 한다 — 실제 동기화(GET/PUT)를 수행하는
 * `useDashboardSync()` 는 AppLayout 이 소유한다.
 */
export function useDashboardSyncStatus(): DashboardSyncStatus {
  return useContext(DashboardSyncContext);
}
