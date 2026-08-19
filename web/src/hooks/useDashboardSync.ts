// SPEC-DASHBOARD-004 (M5) — 대시보드 동기화 훅.
//
// 구 모델(SPEC-DASHBOARD-001)의 스코프 2슬롯 동기화를 **대시보드 단위**로 재구성한
// 것이다. 전송 단위가 "묶음 전체" 에서 "대시보드 1장" 으로 바뀌면서, A 를 편집하는
// 동안 B 의 내용이 함께 전송되어 타인의 변경을 덮어쓰던 문제가 사라진다(spec.md §2.8).
//
// 책임:
//  1. 마운트 시 1회 마이그레이션 토스트(`consumeMigrationToastFlag`).
//  2. 부팅 시 `GET /dashboards` **1회** + `GET /dashboard-state` 1회.
//     구 모델의 `Promise.all([getShared, getMine])` 스코프 이중 조회는 제거되었다
//     (spec.md §2.14 UB2 #1).
//  3. 활성 대시보드가 정해지면 그 1장만 `GET /dashboards/{uid}` 로 본문을 받는다.
//     활성 대시보드가 바뀔 때마다 다시 받는다 — 그리드 설정 3종이 대시보드마다
//     다르므로(spec.md §2.1) 캐시된 본문만으로는 전환 후 상태를 복원할 수 없다.
//  4. 활성 대시보드의 본문이 바뀌면 500ms debounce 후 `PUT /dashboards/{uid}`.
//     활성이 아닌 대시보드로는 PUT 하지 않는다.
//  5. 활성 uid · deviceGridLayout 변경은 `PUT /dashboard-state` 로 따로 저장한다.
//
// 구 모델에서 그대로 승계한 것(키만 스코프 → uid 로 바뀐다):
//  - 500ms debounce
//  - fingerprint 기반 spurious PUT 방지
//  - 단일 비행(single-flight) + 큐잉
//  - 409 → 1회 재PUT, 두 번째 409 면 서버 상태 강제 적용
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.8 E2, §2.13 UB1 #11, §2.14 UB2)

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  DashboardForbiddenError,
  DashboardNotFoundError,
  DashboardServerError,
  DashboardUnauthorizedError,
  getDashboard,
  getState,
  listDashboards,
  putState,
  updateDashboard,
  type ConflictResult,
  type DashboardPutResult,
} from '@/services/api/dashboardService';
import { useTranslation } from '@/lib/i18n';
import {
  collectActiveDashboardContent,
  consumeMigrationToastFlag,
  useUIStore,
} from '@/stores/uiStore';
import type { Dashboard, DashboardContent, DashboardDetail } from '@/types/dashboard';

/** 500 ms — SPEC-DASHBOARD-001 ASM-005 승계. */
const DEBOUNCE_MS = 500;

// 토스트 메시지 i18n 키. 모듈 스코프에는 키만 두고 실제 문자열은 t(key) 로 해석한다.
const MIGRATION_TOAST_KEY = 'dashboard.sync.migrationToast';
const FORBIDDEN_TOAST_KEY = 'dashboard.sync.forbiddenDashboard';
const REMOVED_TOAST_KEY = 'dashboard.sync.dashboardRemoved';
const CONFLICT_RESOLVED_TOAST_KEY = 'dashboard.sync.conflictResolved';
const NETWORK_ERROR_TOAST_KEY = 'dashboard.sync.networkError';

/** useDashboardSync 반환값 — DashboardPage 가 UI 인디케이터에 사용. */
export interface DashboardSyncStatus {
  /** 부팅 조회(목록 + UI 상태 + 활성 대시보드 본문) 진행 중 여부. */
  isLoading: boolean;
  /** 마지막 발생한 치명적 에러 (null = 정상). */
  error: Error | null;
  /** 현재 미동기화 변경 (debounce 중 또는 in-flight). */
  pendingSync: boolean;
}

/** 대시보드 본문의 content fingerprint — 동일 내용의 spurious PUT 방지용. */
function fingerprint(content: DashboardContent): string {
  return JSON.stringify(content);
}

/** 사용자 UI 상태의 fingerprint. */
function stateFingerprint(activeUid: string, layout: unknown): string {
  return JSON.stringify({ activeUid, layout });
}

/**
 * 폴백 대상 대시보드를 고른다 — `is_default` 우선, 없으면 첫 항목 (spec.md §2.13 UB1 #11).
 * 접근 가능한 대시보드가 0장이면 빈 문자열.
 *
 * M6 의 삭제·403 복구도 이 함수를 재사용한다 — 폴백 규칙이 두 곳에 생기면
 * 한쪽만 갱신되어 "어떤 경로로 잃었는지" 에 따라 착지점이 달라진다.
 */
export function pickFallbackUid(list: Dashboard[]): string {
  return list.find((d) => d.is_default)?.uid ?? list[0]?.uid ?? '';
}

/**
 * 대시보드 서버 상태와 클라이언트 메모리 상태를 동기화하는 훅.
 *
 * 호출 위치는 `AppLayout` 단 한 곳이다. 상태를 읽기만 하는 화면은
 * `useDashboardSyncStatus()` (DashboardSyncContext) 를 쓴다.
 *
 * - 여러 곳에서 호출하면 부팅 조회와 PUT 이 중복 발생한다.
 * - 라우트 컴포넌트(예: DashboardPage)에서 호출하면 형제 라우트(`/panels/new`)로
 *   이동할 때 언마운트되어 저장 PUT 이 유실되고, 복귀 시 부팅 조회가 로컬 변경을
 *   덮어쓴다.
 */
export function useDashboardSync(): DashboardSyncStatus {
  const { t } = useTranslation();
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [pendingSync, setPendingSync] = useState(false);

  const addNotification = useUIStore((s) => s.addNotification);
  const setDashboards = useUIStore((s) => s.setDashboards);
  const applyDashboardDetail = useUIStore((s) => s.applyDashboardDetail);
  const applyDashboardMeta = useUIStore((s) => s.applyDashboardMeta);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const setDeviceGridLayout = useUIStore((s) => s.setDeviceGridLayout);
  const activeUid = useUIStore((s) => s.activeDashboardId);

  // 자체 초기화 1회 가드.
  const bootedRef = useRef(false);
  // uid 별 "마지막으로 서버에 보낸/받은 fingerprint".
  const lastSyncedFingerprintRef = useRef<Record<string, string>>({});
  // uid 별 debounce 타이머.
  const debounceTimerRef = useRef<Record<string, ReturnType<typeof setTimeout> | null>>({});
  // uid 별 진행 중 PUT 표시 (동시 PUT 금지) — 큐가 있으면 응답 후 한 번 더 발사.
  const inFlightRef = useRef<Record<string, boolean>>({});
  const queuedRef = useRef<Record<string, boolean>>({});
  // uid 별 "보내야 할 최신 본문". debounce 만료 시점의 store 를 다시 읽지 않는다 —
  // 그 사이 활성 대시보드가 바뀌었으면 남의 본문을 남의 uid 로 보내게 된다.
  const pendingContentRef = useRef<Record<string, DashboardContent>>({});
  // uid 별 403 토스트 1회 제한.
  const forbiddenToastShownRef = useRef<Record<string, boolean>>({});

  // 사용자 UI 상태(/dashboard-state) 동기화용.
  const stateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastStateFingerprintRef = useRef<string | null>(null);

  // 목록 재조회(권한 상실·삭제 복구) 중복 실행 방지 — 무한 재시도 금지(AC-21).
  const recoveringRef = useRef(false);
  // 마지막으로 본문을 받아온 uid. 전환 시에만 다시 받는다.
  const loadedUidRef = useRef<string | null>(null);
  // 서버 응답을 store 에 적용하는 동안 구독의 본문 분기를 억제한다.
  // 적용은 동기이므로 구독이 fingerprint 갱신 **이전에** 발화하며, 억제하지 않으면
  // 방금 저장한 내용을 곧바로 다시 PUT 한다.
  const applyingRef = useRef(false);

  // 마운트 해제 가드.
  const mountedRef = useRef(true);

  const showToast = useCallback(
    (type: 'info' | 'success' | 'warning' | 'error', message: string): void => {
      addNotification({ type, message });
    },
    [addNotification],
  );

  /** 서버 본문 응답을 store 에 반영하고 fingerprint 를 맞춘다. */
  const applyServerDetail = useCallback(
    (detail: DashboardDetail): void => {
      applyingRef.current = true;
      try {
        applyDashboardDetail(detail);
      } finally {
        applyingRef.current = false;
      }
      // 적용 결과를 store 에서 다시 읽어 fingerprint 를 만든다. 서버 payload 가
      // 그리드 설정을 생략했을 때 store 의 기본값 보정과 어긋나면 즉시 spurious
      // PUT 이 발생하므로, "적용 후 상태" 를 기준으로 삼아야 한다.
      const state = useUIStore.getState();
      if (state.activeDashboardId === detail.uid) {
        const content = collectActiveDashboardContent(state);
        if (content) lastSyncedFingerprintRef.current[detail.uid] = fingerprint(content);
      }
    },
    [applyDashboardDetail],
  );

  /**
   * 접근을 잃은 대시보드에서 복구한다 (spec.md §2.13 UB1 #11, AC-21).
   *
   * 목록을 **1회** 재조회하고, 대상이 목록에서 사라졌으면 `is_default` → 첫 항목
   * 순으로 전환한다. 재시도하지 않는다 — 재시도하면 삭제된 대시보드에 대해
   * 무한 루프가 된다.
   */
  const recoverFromAccessLoss = useCallback(
    async (uid: string, kind: 'forbidden' | 'removed'): Promise<void> => {
      if (recoveringRef.current) return;
      recoveringRef.current = true;
      try {
        const list = await listDashboards();
        if (!mountedRef.current) return;
        setDashboards(list);

        if (list.some((d) => d.uid === uid)) {
          // 대시보드는 남아 있고 편집 권한만 잃었다 — 읽기 전용으로 두고 안내한다.
          if (kind === 'forbidden' && !forbiddenToastShownRef.current[uid]) {
            forbiddenToastShownRef.current[uid] = true;
            showToast('warning', t(FORBIDDEN_TOAST_KEY));
          }
          return;
        }

        // 목록에서 사라졌다 — 폴백. 접근 가능한 대시보드가 0장이면 빈 문자열이며
        // 화면은 빈 상태를 표시한다(재시도하지 않는다).
        const fallback = pickFallbackUid(list);
        if (useUIStore.getState().activeDashboardId === uid) {
          setActiveDashboard(fallback);
          showToast('warning', t(REMOVED_TOAST_KEY));
        }
      } catch {
        // 재조회 실패는 그대로 둔다. 다음 사용자 조작에서 다시 시도된다.
      } finally {
        recoveringRef.current = false;
      }
    },
    [setDashboards, setActiveDashboard, showToast, t],
  );

  /** 활성 대시보드 1장의 본문을 받아 적용한다. */
  const loadActiveDetail = useCallback(
    async (uid: string): Promise<void> => {
      loadedUidRef.current = uid;
      try {
        const detail = await getDashboard(uid);
        if (!mountedRef.current) return;
        if (detail === null) {
          // 조회 시점에 이미 없어진 대시보드 — 목록을 다시 받아 폴백한다.
          void recoverFromAccessLoss(uid, 'removed');
          return;
        }
        applyServerDetail(detail);
      } catch (err) {
        if (err instanceof DashboardForbiddenError) {
          void recoverFromAccessLoss(uid, 'forbidden');
          return;
        }
        if (err instanceof DashboardUnauthorizedError) {
          // 401 은 interceptor 의 refresh 흐름에 위임한다.
          return;
        }
        if (mountedRef.current && err instanceof Error) setError(err);
      }
    },
    [applyServerDetail, recoverFromAccessLoss],
  );

  /**
   * 실제 PUT 을 수행한다. 동시 PUT 금지 — in-flight 가 있으면 queue 표시만.
   *
   * @param uid     대상 대시보드 uid
   * @param content 전송할 본문
   * @param ifMatch If-Match 헤더 값 (0 또는 undefined 면 헤더 생략)
   * @param retried 이미 409 로 한 번 재시도 했는가?
   */
  const performPut = useCallback(
    async (
      uid: string,
      content: DashboardContent,
      ifMatch: number | undefined,
      retried: boolean,
    ): Promise<void> => {
      if (inFlightRef.current[uid]) {
        queuedRef.current[uid] = true;
        return;
      }

      inFlightRef.current[uid] = true;
      if (mountedRef.current) setPendingSync(true);

      const fp = fingerprint(content);

      try {
        const result: DashboardPutResult = await updateDashboard(uid, content, ifMatch);

        if ('conflict' in result && result.conflict) {
          const conflict = result as ConflictResult;

          if (retried) {
            // 두 번째 409 — 무한 루프 방지, 서버 상태로 강제 동기.
            applyServerDetail(conflict.serverDashboard);
            showToast('warning', t(CONFLICT_RESOLVED_TOAST_KEY));
          } else if (fp !== fingerprint(conflict.serverDashboard.payload)) {
            // 1차 409 — 서버의 새 version 으로 사용자의 원래 본문을 1회 재시도.
            inFlightRef.current[uid] = false;
            if (mountedRef.current) setPendingSync(false);
            await performPut(uid, content, conflict.serverDashboard.version, true);
            return;
          } else {
            // 서버 본문과 동일 — 재시도 없이 서버 상태로 동기.
            applyServerDetail(conflict.serverDashboard);
          }
        } else {
          // 200 — 메타(version)만 반영한다. 본문을 서버 응답으로 덮으면 in-flight
          // 동안 사용자가 가한 변경이 조용히 사라진다.
          const { payload: _payload, ...meta } = result as DashboardDetail;
          lastSyncedFingerprintRef.current[uid] = fp;
          applyingRef.current = true;
          try {
            applyDashboardMeta(meta);
          } finally {
            applyingRef.current = false;
          }
        }
      } catch (err) {
        // CRITICAL: 모든 비-성공 경로에서 lastSynced 를 방금 보낸 fingerprint 로
        // 갱신해야 한다. 갱신하지 않으면 showToast → addNotification → store 변경
        // → subscribe 재발화 → schedulePut → 또 PUT → 또 실패 … 무한 루프가 된다.
        // 사용자가 새로 변경하면 fingerprint 가 다시 바뀌어 의도된 PUT 이 트리거된다.
        lastSyncedFingerprintRef.current[uid] = fp;

        if (err instanceof DashboardForbiddenError) {
          // 편집 권한 상실 — 재시도하지 않고 목록을 재조회한다(spec.md §2.8).
          void recoverFromAccessLoss(uid, 'forbidden');
        } else if (err instanceof DashboardNotFoundError) {
          // 타 세션에서 삭제됨 — 목록 1회 재조회 후 폴백(AC-21).
          void recoverFromAccessLoss(uid, 'removed');
        } else if (err instanceof DashboardUnauthorizedError) {
          // 401 — interceptor 가 refresh 흐름을 처리하므로 여기서는 보관만.
          if (mountedRef.current) setError(err);
        } else if (err instanceof DashboardServerError || err instanceof Error) {
          showToast('error', t(NETWORK_ERROR_TOAST_KEY));
          if (mountedRef.current) setError(err);
        }
      } finally {
        inFlightRef.current[uid] = false;
        // 큐가 있으면 한 번 더 발사 (in-flight 동안 변경이 누적된 경우).
        let dispatchedFromQueue = false;
        if (queuedRef.current[uid] && mountedRef.current) {
          queuedRef.current[uid] = false;
          const latest = pendingContentRef.current[uid];
          if (latest && fingerprint(latest) !== lastSyncedFingerprintRef.current[uid]) {
            const version =
              useUIStore.getState().dashboards.find((d) => d.uid === uid)?.version ?? 0;
            void performPut(uid, latest, version > 0 ? version : undefined, false);
            dispatchedFromQueue = true;
          }
        }
        if (!dispatchedFromQueue && mountedRef.current) {
          setPendingSync(false);
        }
      }
    },
    [applyServerDetail, applyDashboardMeta, recoverFromAccessLoss, showToast, t],
  );

  /** 예약된 PUT 을 취소한다 (내용이 기준선으로 되돌아온 경우). */
  const cancelPendingPut = useCallback((uid: string): void => {
    const timer = debounceTimerRef.current[uid];
    if (timer) {
      clearTimeout(timer);
      debounceTimerRef.current[uid] = null;
    }
    delete pendingContentRef.current[uid];
    queuedRef.current[uid] = false;
    if (!inFlightRef.current[uid] && mountedRef.current) setPendingSync(false);
  }, []);

  /** 변경 감지 시 debounce 후 PUT 을 예약한다. */
  const schedulePut = useCallback(
    (uid: string): void => {
      const existing = debounceTimerRef.current[uid];
      if (existing) clearTimeout(existing);
      if (mountedRef.current) setPendingSync(true);

      debounceTimerRef.current[uid] = setTimeout(() => {
        debounceTimerRef.current[uid] = null;
        const content = pendingContentRef.current[uid];
        if (!content) {
          if (mountedRef.current) setPendingSync(false);
          return;
        }
        const version = useUIStore.getState().dashboards.find((d) => d.uid === uid)?.version ?? 0;
        void performPut(uid, content, version > 0 ? version : undefined, false);
      }, DEBOUNCE_MS);
    },
    [performPut],
  );

  /** 사용자 UI 상태(/dashboard-state) 를 debounce 후 저장한다. */
  const scheduleStatePut = useCallback((): void => {
    if (stateTimerRef.current) clearTimeout(stateTimerRef.current);
    stateTimerRef.current = setTimeout(() => {
      stateTimerRef.current = null;
      const state = useUIStore.getState();
      const fp = stateFingerprint(state.activeDashboardId, state.deviceGridLayout);
      // 실패해도 재시도하지 않는다 — UI 상태는 다음 조작에서 다시 저장된다.
      lastStateFingerprintRef.current = fp;
      void putState({
        active_dashboard_uid: state.activeDashboardId,
        device_grid_layout: state.deviceGridLayout,
      }).catch(() => undefined);
    }, DEBOUNCE_MS);
  }, []);

  // ─────────────────────────────────────────────────────────────────────
  // 마운트 시 1회: 마이그레이션 토스트 + 부팅 조회
  // ─────────────────────────────────────────────────────────────────────

  useEffect(() => {
    mountedRef.current = true;
    if (bootedRef.current) return;
    bootedRef.current = true;

    // (1) 마이그레이션 토스트 (v0.2 첫 부팅 1회 한정).
    if (consumeMigrationToastFlag()) {
      addNotification({ type: 'info', message: t(MIGRATION_TOAST_KEY) });
    }

    void (async () => {
      try {
        // (2) 목록 1회 + UI 상태 1회. 서로 다른 리소스이므로 병렬로 받는다.
        //     구 모델의 스코프 이중 조회(shared + mine)와는 무관하다.
        const [list, serverState] = await Promise.all([
          listDashboards().catch((err: unknown) => (err instanceof Error ? err : null)),
          getState().catch(() => null),
        ]);

        if (!mountedRef.current) return;

        if (list instanceof Error || list === null) {
          if (list instanceof Error && !(list instanceof DashboardUnauthorizedError)) {
            setError(list);
          }
          return;
        }

        setDashboards(list);
        if (serverState) {
          setDeviceGridLayout(serverState.device_grid_layout ?? {});
        }

        // (3) 활성 대시보드 결정 — 서버가 기억한 uid 우선, 없거나 접근 불가면
        //     is_default → 첫 항목 순으로 폴백한다(spec.md §2.13 UB1 #11).
        const remembered = serverState?.active_dashboard_uid ?? '';
        const resolved = list.some((d) => d.uid === remembered)
          ? remembered
          : pickFallbackUid(list);
        setActiveDashboard(resolved);

        lastStateFingerprintRef.current = stateFingerprint(
          resolved,
          useUIStore.getState().deviceGridLayout,
        );

        // (4) 서버가 기억한 값과 다르면 정정 저장한다(AC-21).
        if (serverState && resolved !== serverState.active_dashboard_uid) {
          void putState({
            active_dashboard_uid: resolved,
            device_grid_layout: useUIStore.getState().deviceGridLayout,
          }).catch(() => undefined);
        }

        // (5) 활성 대시보드 1장의 본문을 받는다.
        if (resolved) {
          await loadActiveDetail(resolved);
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
  // 활성 대시보드 전환 — 그 1장의 본문을 다시 받는다.
  // ─────────────────────────────────────────────────────────────────────

  useEffect(() => {
    if (isLoading || !activeUid) return;
    if (loadedUidRef.current === activeUid) return;
    void loadActiveDetail(activeUid);
  }, [activeUid, isLoading, loadActiveDetail]);

  // ─────────────────────────────────────────────────────────────────────
  // store 변경 구독 — 활성 대시보드 본문 / 사용자 UI 상태
  // ─────────────────────────────────────────────────────────────────────

  useEffect(() => {
    const unsubscribe = useUIStore.subscribe((state, prev) => {
      // 부팅 조회 완료 전에는 저장하지 않는다.
      if (isLoading) return;

      // (a) 사용자 UI 상태 — 활성 uid / 디바이스 그리드 레이아웃.
      const stateFp = stateFingerprint(state.activeDashboardId, state.deviceGridLayout);
      if (stateFp !== lastStateFingerprintRef.current) {
        scheduleStatePut();
      }

      // (b) 활성 대시보드 본문. 서버 응답 적용 중에는 건너뛴다.
      if (applyingRef.current) return;
      const uid = state.activeDashboardId;
      if (!uid) return;
      // 방금 활성이 바뀐 경우는 변경이 아니다 — 전환 이펙트가 본문을 다시 받는다.
      if (prev.activeDashboardId !== uid) return;

      const content = collectActiveDashboardContent(state);
      if (!content) return;
      const fp = fingerprint(content);

      const baseline = lastSyncedFingerprintRef.current[uid];
      // 서버 본문을 아직 받지 못한 대시보드는 저장하지 않는다. 기준선이 없으면
      // "사용자의 변경" 과 "아직 비어 있는 자리표시자" 를 구분할 수 없어, 폴백 직후
      // 빈 본문을 새 대시보드에 덮어쓰게 된다.
      if (baseline === undefined) return;

      if (baseline === fp) {
        // 내용이 기준선으로 되돌아왔다 — 예약된 PUT 을 취소한다.
        cancelPendingPut(uid);
        return;
      }

      pendingContentRef.current[uid] = content;
      schedulePut(uid);
    });
    return unsubscribe;
  }, [isLoading, schedulePut, scheduleStatePut, cancelPendingPut]);

  // 언마운트 시 진행 중 타이머 정리.
  useEffect(() => {
    const timersRef = debounceTimerRef;
    const stateRef = stateTimerRef;
    return () => {
      for (const timer of Object.values(timersRef.current)) {
        if (timer) clearTimeout(timer);
      }
      if (stateRef.current) clearTimeout(stateRef.current);
    };
  }, []);

  return { isLoading, error, pendingSync };
}
