// SPEC-DASHBOARD-001 v0.2.0 — 대시보드 snapshot 동기화 훅.
//
// 책임:
// 1. 마운트 시점에 1회 마이그레이션 토스트(`consumeMigrationToastFlag`) 노출.
// 2. `Promise.all([getShared, getMine])` 병렬 호출로 두 snapshot 을 store 에 채움.
//    404 → 빌트인 기본 snapshot (version=0) 으로 메모리 상태 초기화.
// 3. 활성 스코프의 snapshot.payload 변경을 감지(JSON content fingerprint) 하여
//    500ms debounce 후 해당 스코프 엔드포인트로 PUT.
// 4. PUT 응답:
//    - 200: 새 snapshot 적용 + lastSyncedPayload fingerprint 갱신.
//    - 403 (shared only): 토스트 1회, 재시도 안 함 (lastSynced 를 현재 fingerprint 로
//      덮어써 무한 PUT 루프 방지).
//    - 409: 서버 snapshot 적용 + 1회 재PUT (If-Match: server.version). 두 번째 409
//      발생 시 서버 snapshot 강제 적용 + 토스트.
//    - 기타 (네트워크/500): 토스트 + 다음 변경 시 재시도 (lastSynced 는 변경 없음).
// 5. in-flight PUT 동안 추가 변경은 큐잉 — 응답 수신 후 한 번 더 PUT (동시 PUT 금지).
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  ConflictResult,
  DashboardForbiddenError,
  DashboardServerError,
  DashboardUnauthorizedError,
  PutResult,
  getMyDashboard,
  getSharedDashboard,
  putMyDashboard,
  putSharedDashboard,
} from '@/services/api/dashboardService';
import { useAuthStore } from '@/stores/authStore';
import {
  buildDefaultSnapshot,
  collectActivePayload,
  consumeMigrationToastFlag,
  readActiveScopeFromSession,
  useUIStore,
} from '@/stores/uiStore';
import type { DashboardPayload, DashboardSnapshot } from '@/types/dashboard';

/** 500 ms — SPEC ASM-005. */
const DEBOUNCE_MS = 500;

/** 마이그레이션 안내 토스트 메시지 (SPEC §비고). */
const MIGRATION_TOAST_MESSAGE =
  '이번 업데이트(v0.2.0)로 대시보드 구성이 서버 저장으로 전환되었습니다. 기존 로컬 구성은 초기화됩니다. 공유 대시보드는 관리자가 다시 구성해 주세요.';

const FORBIDDEN_SHARED_TOAST = '공유 대시보드 편집 권한이 없습니다.';
const CONFLICT_RESOLVED_TOAST = '다른 브라우저에서 변경이 있어 동기화했습니다.';
const NETWORK_ERROR_TOAST = '대시보드 저장에 실패했습니다. 잠시 후 다시 시도해 주세요.';

/** useDashboardSync 반환값 — DashboardPage 가 UI 인디케이터에 사용. */
export interface DashboardSyncStatus {
  /** 부팅 GET shared+mine 진행 중 여부. */
  isLoading: boolean;
  /** 마지막 발생한 치명적 에러 (null = 정상). */
  error: Error | null;
  /** 현재 미동기화 변경 (debounce 중 또는 in-flight). */
  pendingSync: boolean;
}

type Scope = 'shared' | 'mine';

/** payload 의 content fingerprint — 동일 payload 의 spurious PUT 방지용. */
function fingerprint(payload: DashboardPayload): string {
  return JSON.stringify(payload);
}

/** 활성 스코프의 snapshot 을 store 에서 읽는다 (selector 외부 호출용). */
function getSnapshotForScope(scope: Scope): DashboardSnapshot | null {
  const state = useUIStore.getState();
  return scope === 'shared' ? state.sharedSnapshot : state.mineSnapshot;
}

/**
 * 대시보드 서버 snapshot 과 클라이언트 메모리 상태를 양방향 동기화하는 훅.
 *
 * App 루트(예: DashboardPage 또는 AppLayout) 에서 단 한 번만 호출해야 한다.
 * 여러 곳에서 호출하면 부팅 GET 과 PUT 이 중복 발생할 수 있다.
 */
export function useDashboardSync(): DashboardSyncStatus {
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [pendingSync, setPendingSync] = useState(false);

  const addNotification = useUIStore((s) => s.addNotification);
  const setSharedSnapshot = useUIStore((s) => s.setSharedSnapshot);
  const setMineSnapshot = useUIStore((s) => s.setMineSnapshot);
  const setActiveDashboardScope = useUIStore((s) => s.setActiveDashboardScope);
  // 자체 초기화 1회 가드.
  const bootedRef = useRef(false);
  // 스코프별 "마지막으로 서버에 보낸/받은 fingerprint" 를 추적해
  // 같은 내용에 대한 spurious PUT 을 방지한다.
  const lastSyncedFingerprintRef = useRef<Record<Scope, string | null>>({
    shared: null,
    mine: null,
  });
  // debounce 타이머 (스코프별).
  const debounceTimerRef = useRef<Record<Scope, ReturnType<typeof setTimeout> | null>>({
    shared: null,
    mine: null,
  });
  // 진행 중 PUT 표시 (동시 PUT 금지) — 큐가 있는 경우 응답 후 한 번 더 발사.
  const inFlightRef = useRef<Record<Scope, boolean>>({ shared: false, mine: false });
  const queuedRef = useRef<Record<Scope, boolean>>({ shared: false, mine: false });

  // 활성 스코프 외 forbidden 토스트는 1회만.
  const forbiddenToastShownRef = useRef(false);

  // 마운트 해제 가드.
  const mountedRef = useRef(true);

  const showToast = useCallback(
    (type: 'info' | 'success' | 'warning' | 'error', message: string): void => {
      addNotification({ type, message });
    },
    [addNotification],
  );

  /** 서버 응답으로 받은 snapshot 을 store 에 반영 + fingerprint 동기. */
  const applyServerSnapshot = useCallback(
    (scope: Scope, snapshot: DashboardSnapshot): void => {
      lastSyncedFingerprintRef.current[scope] = fingerprint(snapshot.payload);
      if (scope === 'shared') {
        setSharedSnapshot(snapshot, { fromServer: true });
      } else {
        setMineSnapshot(snapshot, { fromServer: true });
      }
    },
    [setSharedSnapshot, setMineSnapshot],
  );

  /** 404 응답 처리 — 빌트인 기본 snapshot (version=0) 으로 메모리 초기화. */
  const applyDefaultForScope = useCallback(
    (scope: Scope): void => {
      const owner =
        scope === 'mine' ? (useAuthStore.getState().user?.name ?? null) : null;
      const defaultSnap = buildDefaultSnapshot(scope === 'shared' ? 'global' : 'user', owner);
      // 404 fallback 도 lastSynced 를 갱신해 spurious PUT 을 방지한다.
      // (첫 변경 시점에 사용자가 명시적으로 mutate 하면 fingerprint 가 바뀌어 PUT 된다.)
      lastSyncedFingerprintRef.current[scope] = fingerprint(defaultSnap.payload);
      if (scope === 'shared') {
        setSharedSnapshot(defaultSnap, { fromServer: true });
      } else {
        setMineSnapshot(defaultSnap, { fromServer: true });
      }
    },
    [setSharedSnapshot, setMineSnapshot],
  );

  /**
   * 실제 PUT 을 수행한다. 동시 PUT 금지 — in-flight 가 있으면 queue 표시만.
   *
   * @param scope 'shared' | 'mine'
   * @param payload 전송할 페이로드
   * @param ifMatch If-Match 헤더 값 (0 또는 undefined 면 헤더 생략)
   * @param retried 이미 409 한 번 재시도 했는가?
   */
  const performPut = useCallback(
    async (
      scope: Scope,
      payload: DashboardPayload,
      ifMatch: number | undefined,
      retried: boolean,
    ): Promise<void> => {
      if (inFlightRef.current[scope]) {
        queuedRef.current[scope] = true;
        return;
      }

      inFlightRef.current[scope] = true;
      if (mountedRef.current) setPendingSync(true);

      const putFn = scope === 'shared' ? putSharedDashboard : putMyDashboard;
      const fp = fingerprint(payload);

      try {
        const result: PutResult = await putFn(payload, ifMatch);
        if ('conflict' in result && result.conflict) {
          // 409 — 서버 snapshot 적용 + 사용자 변경 재시도 (SPEC AC-10).
          const conflict = result as ConflictResult;

          if (retried) {
            // 두 번째 409 — 무한 루프 방지, 서버 상태로 강제 동기.
            applyServerSnapshot(scope, conflict.serverSnapshot);
            showToast('warning', CONFLICT_RESOLVED_TOAST);
          } else {
            // 1차 409: 사용자가 보내려던 payload 는 `payload` 인자에 그대로 있다.
            // 서버 snapshot 의 메타데이터(version) 만 받아 If-Match 를 갱신하고 1회 재시도.
            // 서버측 최신 payload 와 user payload 가 동일하면 재시도 불필요.
            if (fingerprint(payload) !== fingerprint(conflict.serverSnapshot.payload)) {
              inFlightRef.current[scope] = false;
              if (mountedRef.current) setPendingSync(false);
              // 재PUT — 사용자의 원래 payload 를 서버의 새 version 으로 재시도.
              await performPut(scope, payload, conflict.serverSnapshot.version, true);
              return;
            } else {
              // 동일한 payload — 단순히 server snapshot 으로 동기.
              applyServerSnapshot(scope, conflict.serverSnapshot);
            }
          }
        } else {
          // 200 — 정상 적용.
          applyServerSnapshot(scope, result as DashboardSnapshot);
        }
      } catch (err) {
        // CRITICAL: 모든 비-성공/비-409 에러 경로에서 lastSynced 를 현재 fingerprint 로
        // 갱신해야 한다. 갱신하지 않으면 showToast → addNotification → store 변경 →
        // subscribe 재발화 → schedulePut → 또 PUT → 또 401/500 → ... 무한 루프가 발생.
        // 사용자가 새로 변경하면 fingerprint 가 다시 바뀌어 의도된 PUT 이 트리거된다.
        lastSyncedFingerprintRef.current[scope] = fp;

        if (err instanceof DashboardForbiddenError) {
          // 403 — shared PUT 시 admin 아님. 토스트 1회 노출.
          if (scope === 'shared' && !forbiddenToastShownRef.current) {
            forbiddenToastShownRef.current = true;
            showToast('warning', FORBIDDEN_SHARED_TOAST);
          }
        } else if (err instanceof DashboardUnauthorizedError) {
          // 401 — interceptor (`interceptors.ts`) 가 refresh 흐름을 처리하므로
          // 여기서는 별도 redirect 하지 않는다. 에러만 보관.
          if (mountedRef.current) setError(err);
        } else if (err instanceof DashboardServerError || err instanceof Error) {
          showToast('error', NETWORK_ERROR_TOAST);
          if (mountedRef.current) setError(err);
        }
      } finally {
        inFlightRef.current[scope] = false;
        // 큐가 있으면 한 번 더 발사 (변경이 in-flight 동안 누적된 경우).
        let dispatchedFromQueue = false;
        if (queuedRef.current[scope] && mountedRef.current) {
          queuedRef.current[scope] = false;
          const latestPayload = collectActivePayload(useUIStore.getState());
          const latestFp = fingerprint(latestPayload);
          if (latestFp !== lastSyncedFingerprintRef.current[scope]) {
            const snap = getSnapshotForScope(scope);
            const v = snap?.version ?? 0;
            // 비동기 재호출 — 큐 처리.
            void performPut(scope, latestPayload, v > 0 ? v : undefined, false);
            dispatchedFromQueue = true;
          }
        }
        if (!dispatchedFromQueue && mountedRef.current) {
          setPendingSync(false);
        }
      }
    },
    [applyServerSnapshot, showToast],
  );

  /** 변경 감지 시 debounce 후 PUT 을 예약한다. */
  const schedulePut = useCallback(
    (scope: Scope): void => {
      const existing = debounceTimerRef.current[scope];
      if (existing) {
        clearTimeout(existing);
      }
      if (mountedRef.current) setPendingSync(true);
      debounceTimerRef.current[scope] = setTimeout(() => {
        debounceTimerRef.current[scope] = null;
        const state = useUIStore.getState();
        // 스코프가 도중에 바뀌었어도 그 스코프에 대해 PUT 한다 — 사용자의 의도된 변경.
        const payload = scope === state.activeDashboardScope
          ? collectActivePayload(state)
          : null;
        if (!payload) {
          if (mountedRef.current) setPendingSync(false);
          return;
        }
        const snap = scope === 'shared' ? state.sharedSnapshot : state.mineSnapshot;
        const version = snap?.version ?? 0;
        void performPut(scope, payload, version > 0 ? version : undefined, false);
      }, DEBOUNCE_MS);
    },
    [performPut],
  );

  // ─────────────────────────────────────────────────────────────────────
  // 마운트 시 1회: 마이그레이션 토스트 + 부팅 GET shared+mine
  // ─────────────────────────────────────────────────────────────────────

  useEffect(() => {
    mountedRef.current = true;
    if (bootedRef.current) return;
    bootedRef.current = true;

    // (1) 마이그레이션 토스트 (v0.2 첫 부팅 1회 한정 — uiStore 모듈 로드 시 LS 정리 완료).
    if (consumeMigrationToastFlag()) {
      // info 로 표시 (안내 성격). 사용자가 이미 토스트를 본 뒤에는 LS 플래그가 있어 다시 안 뜸.
      addNotification({ type: 'info', message: MIGRATION_TOAST_MESSAGE });
    }

    // (2) 병렬 GET. 인증되지 않은 경우 호출은 401 을 받고 interceptor 가 처리한다.
    void (async () => {
      try {
        const [shared, mine] = await Promise.all([
          getSharedDashboard().catch((err): null | Error => err instanceof Error ? err : null),
          getMyDashboard().catch((err): null | Error => err instanceof Error ? err : null),
        ]);

        // shared 결과 처리.
        if (shared instanceof Error) {
          if (!(shared instanceof DashboardUnauthorizedError) && mountedRef.current) {
            setError(shared);
          }
          // unauthorized 인 경우 굳이 default 로 채우지 않음 — 로그인 후 재초기화 흐름에 위임.
        } else if (shared === null) {
          applyDefaultForScope('shared');
        } else {
          applyServerSnapshot('shared', shared);
        }

        // mine 결과 처리.
        if (mine instanceof Error) {
          if (!(mine instanceof DashboardUnauthorizedError) && mountedRef.current) {
            setError(mine);
          }
        } else if (mine === null) {
          applyDefaultForScope('mine');
        } else {
          applyServerSnapshot('mine', mine);
        }

        // (3) 활성 스코프 결정 — sessionStorage > 역할 기반 기본값.
        const sessionScope = readActiveScopeFromSession();
        const user = useAuthStore.getState().user;
        const role = user?.role;
        const mineSnap = useUIStore.getState().mineSnapshot;
        const mineHasContent =
          mineSnap !== null && mineSnap.version > 0;

        let initialScope: 'shared' | 'mine';
        if (sessionScope === 'shared' || sessionScope === 'mine') {
          initialScope = sessionScope;
        } else if (role === 'admin') {
          initialScope = 'shared';
        } else if (mineHasContent) {
          initialScope = 'mine';
        } else {
          initialScope = 'shared';
        }
        // 현재 store 값과 다르면 갱신 (legacy 필드도 함께 sync 됨).
        if (useUIStore.getState().activeDashboardScope !== initialScope) {
          setActiveDashboardScope(initialScope);
        }
      } finally {
        if (mountedRef.current) setIsLoading(false);
      }
    })();

    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ─────────────────────────────────────────────────────────────────────
  // store 변경 구독 — 활성 스코프의 snapshot.payload 가 fingerprint 와 다르면 PUT 스케줄
  // ─────────────────────────────────────────────────────────────────────

  useEffect(() => {
    const unsubscribe = useUIStore.subscribe((state, prev) => {
      // 부팅 GET 완료 전에는 PUT 하지 않는다.
      if (isLoading) return;

      const activeScope = state.activeDashboardScope;
      const snap = activeScope === 'shared' ? state.sharedSnapshot : state.mineSnapshot;
      if (!snap) return;

      const currentFp = fingerprint(snap.payload);
      const lastFp = lastSyncedFingerprintRef.current[activeScope];
      if (lastFp === currentFp) return;

      // 활성 스코프가 방금 바뀌었을 경우 (탭 전환), 새 스코프의 payload 는 PUT 하지 않는다.
      // 이는 prev.activeDashboardScope 와 비교하여 식별한다.
      if (prev.activeDashboardScope !== activeScope) {
        // 탭 전환은 변경이 아니므로 fingerprint 만 갱신해 spurious PUT 방지.
        lastSyncedFingerprintRef.current[activeScope] = currentFp;
        return;
      }

      schedulePut(activeScope);
    });
    return unsubscribe;
  }, [isLoading, schedulePut]);

  // 언마운트 시 진행 중 타이머 정리.
  useEffect(() => {
    // ref 값은 effect 본문 진입 시 캡처하여 cleanup 에서 안전하게 사용한다.
    const timersRef = debounceTimerRef;
    return () => {
      const timers = timersRef.current;
      if (timers.shared) clearTimeout(timers.shared);
      if (timers.mine) clearTimeout(timers.mine);
    };
  }, []);

  return { isLoading, error, pendingSync };
}
