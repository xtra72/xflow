// 대시보드 메타 변경·삭제의 서버 반영 (SPEC-DASHBOARD-004 M6).
//
// 이 SPEC 이전에는 대시보드 이름이 스냅샷 payload 안에 있었고 묶음 단위 PUT 이
// 통째로 실어 보내 영속되었다. 1급 엔티티로 승격되면서 `name` · `is_default` 는
// 컬럼이 되었고, 본문 PUT(`updateDashboard`)은 `{ payload }` 만 보낸다.
// 따라서 이름/기본 지정은 **PATCH 로 따로 보내지 않으면 저장되지 않는다.**
// 로컬 스토어만 바꾸면 사용자는 바뀐 화면을 보고 새로고침에서 잃는다.
//
// 정책:
//  - 이름 변경·기본 지정: 낙관적 반영 후 실패 시 **원상 복구 + 토스트**.
//    서버가 거부한 변경을 화면에 남겨두지 않는다.
//  - 삭제: 낙관적으로 지우지 않는다. 낙관적 삭제 후 실패하면 목록을 되돌려도
//    그 대시보드의 본문(panels·layout)이 로컬에서 사라진 뒤라 되살릴 수 없다.
//    응답을 받은 뒤에 지운다.
//  - 403: 권한이 사용자 몰래 바뀌었을 수 있으므로 **목록을 재조회해 판정을 다시
//    읽는다.** 로컬 판정을 추측하지 않는다.
//  - 활성 대시보드를 지웠으면 M5 의 폴백 사슬(`pickFallbackUid`:
//    `is_default` → 첫 항목)을 그대로 쓴다. 새 폴백 로직을 만들지 않는다.
//
// `is_default` 상호 배타(이전 기본 해제)는 건드리지 않는다 — 서버가 이전 기본을
// 지우지 않고 spec 도 침묵한다. M8 이 문서화된 갭으로 기록한다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.13 UB1 #11)

import { useCallback, useState } from 'react';

import { pickFallbackUid } from '@/hooks/useDashboardSync';
import { useTranslation } from '@/lib/i18n';
import {
  DashboardForbiddenError,
  deleteDashboard,
  listDashboards,
  patchDashboard,
} from '@/services/api/dashboardService';
import { useUIStore } from '@/stores/uiStore';
import type { Dashboard } from '@/types/dashboard';

/** `useDashboardMutations()` 반환값. 각 동작은 성공 여부를 돌려준다. */
export interface DashboardMutationsApi {
  /** 이름 변경 → `PATCH {name}` (edit 인가). */
  rename: (uid: string, name: string) => Promise<boolean>;
  /** 기본 대시보드 지정 → `PATCH {is_default}` (grant 인가). */
  setDefault: (uid: string) => Promise<boolean>;
  /** 삭제 → `DELETE` (delete 인가). */
  remove: (uid: string) => Promise<boolean>;
  /** 요청 진행 중 여부. */
  isMutating: boolean;
}

/**
 * 대시보드 메타 변경·삭제를 서버에 반영한다.
 *
 * 낙관적 반영은 `applyDashboardMeta` 로 한다 — 이 액션이 목록 축(`dashboards`)과
 * 본문 축(`dashboardPages`)을 함께 갱신하므로, 한 축만 바뀌어 화면과 판정이
 * 어긋나는 상태가 생기지 않는다. 되돌릴 때도 같은 액션에 직전 메타를 넣는다.
 */
export function useDashboardMutations(): DashboardMutationsApi {
  const { t } = useTranslation();
  const applyDashboardMeta = useUIStore((s) => s.applyDashboardMeta);
  const setDashboards = useUIStore((s) => s.setDashboards);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const addNotification = useUIStore((s) => s.addNotification);
  const [isMutating, setIsMutating] = useState(false);

  const notify = useCallback(
    (messageKey: string) => {
      addNotification({ type: 'error', message: t(messageKey) });
    },
    [addNotification, t],
  );

  /**
   * 403 복구 — 목록을 재조회해 판정을 다시 읽는다.
   *
   * 대상이 목록에서 사라졌고 그것이 활성 대시보드였다면 M5 의 폴백 사슬로
   * 착지한다. 재조회가 실패하면 그대로 둔다(재시도하지 않는다 — 무한 루프 방지).
   */
  const refreshAccess = useCallback(
    async (uid: string): Promise<void> => {
      try {
        const list = await listDashboards();
        setDashboards(list);
        if (
          !list.some((d) => d.uid === uid) &&
          useUIStore.getState().activeDashboardId === uid
        ) {
          setActiveDashboard(pickFallbackUid(list));
        }
      } catch {
        // 재조회 실패는 다음 사용자 조작에서 다시 시도된다.
      }
    },
    [setDashboards, setActiveDashboard],
  );

  /** 현재 목록 축에서 대시보드 메타를 읽는다. 서버에 없는 항목이면 undefined. */
  const metaOf = useCallback(
    (uid: string): Dashboard | undefined =>
      useUIStore.getState().dashboards.find((d) => d.uid === uid),
    [],
  );

  /**
   * 낙관적 PATCH 공통 경로.
   *
   * 1) 직전 메타를 잡아두고 2) 낙관적으로 반영한 뒤 3) 실패하면 되돌린다.
   */
  const patchOptimistic = useCallback(
    async (
      uid: string,
      optimistic: Dashboard,
      patch: Parameters<typeof patchDashboard>[1],
      deniedKey: string,
      failedKey: string,
    ): Promise<boolean> => {
      const previous = metaOf(uid);
      if (!previous) return false;

      applyDashboardMeta(optimistic);
      setIsMutating(true);
      try {
        // If-Match 를 보내지 않는다 — 본문 PUT(debounce)이 version 을 올린 직후면
        // 로컬 version 이 뒤처져 이름 변경이 409 로 막힌다. 메타 변경은 본문과
        // 다른 필드를 건드리므로 무조건 저장으로 둔다(서버 기존 정책).
        const updated = await patchDashboard(uid, patch);
        const { payload: _payload, ...meta } = updated;
        applyDashboardMeta(meta);
        return true;
      } catch (err) {
        // 서버가 거부한 변경을 화면에 남기지 않는다.
        applyDashboardMeta(previous);
        if (err instanceof DashboardForbiddenError) {
          notify(deniedKey);
          await refreshAccess(uid);
        } else {
          notify(failedKey);
        }
        return false;
      } finally {
        setIsMutating(false);
      }
    },
    [applyDashboardMeta, metaOf, notify, refreshAccess],
  );

  const rename = useCallback(
    async (uid: string, name: string): Promise<boolean> => {
      const trimmed = name.trim();
      const previous = metaOf(uid);
      if (!trimmed || !previous || trimmed === previous.name) return false;
      return patchOptimistic(
        uid,
        { ...previous, name: trimmed },
        { name: trimmed },
        'dashboard.gate.editDenied',
        'dashboard.gate.renameFailed',
      );
    },
    [metaOf, patchOptimistic],
  );

  const setDefault = useCallback(
    async (uid: string): Promise<boolean> => {
      const previous = metaOf(uid);
      if (!previous || previous.is_default) return false;
      return patchOptimistic(
        uid,
        { ...previous, is_default: true },
        { is_default: true },
        'dashboard.gate.grantDenied',
        'dashboard.gate.defaultFailed',
      );
    },
    [metaOf, patchOptimistic],
  );

  /**
   * 삭제 — 응답을 받은 뒤에 지운다(낙관적 삭제 금지, 위 주석 참조).
   *
   * 성공하면 목록에서 제거하고, 지운 것이 활성 대시보드였으면 M5 의 폴백 사슬로
   * 착지한다. 남은 대시보드가 0장이면 빈 문자열이 되어 화면이 빈 상태를 표시한다
   * (spec.md 엣지 케이스 "마지막 대시보드 삭제 → 허용").
   */
  const remove = useCallback(
    async (uid: string): Promise<boolean> => {
      if (!metaOf(uid)) return false;

      setIsMutating(true);
      try {
        await deleteDashboard(uid);
      } catch (err) {
        // 실패 시 로컬은 손대지 않았으므로 되돌릴 것이 없다.
        if (err instanceof DashboardForbiddenError) {
          notify('dashboard.gate.deleteDenied');
          await refreshAccess(uid);
        } else {
          notify('dashboard.gate.deleteFailed');
        }
        return false;
      } finally {
        setIsMutating(false);
      }

      const remaining = useUIStore.getState().dashboards.filter((d) => d.uid !== uid);
      setDashboards(remaining);
      if (useUIStore.getState().activeDashboardId === uid) {
        setActiveDashboard(pickFallbackUid(remaining));
      }
      return true;
    },
    [metaOf, notify, refreshAccess, setActiveDashboard, setDashboards],
  );

  return { rename, setDefault, remove, isMutating };
}
