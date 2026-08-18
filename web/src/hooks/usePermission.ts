// 권한 판정 훅 (SPEC-AUTH-006 U1).
//
// 폴백 규칙은 이 파일 한 곳에만 있다. 호출부가 `!authEnabled || ...` 같은 폴백을
// 각자 손으로 적으면 규칙이 갈라지므로, 판정은 전부 여기를 거친다.
//
//   authEnabled === false   → 항상 true (서버가 검사하지 않는데 UI 만 잠그면 회귀)
//   permissions 필드 없음    → 항상 true (구버전 서버 하위 호환)
//   권한 조회 실패           → 항상 false (폴백 금지 — 서버가 어차피 차단한다)
//   그 외                   → 메모리 집합 조회
//
// 판정은 메모리 집합 조회이며 렌더당 추가 네트워크 요청이 없다 (spec.md §5).

import { useCallback, useMemo } from 'react';

import { useAuthStore } from '@/stores/authStore';
import type { PermissionStatus } from '@/stores/authStore';

/** 판정에 필요한 최소 스냅샷. 훅과 비-React 경로가 공유한다. */
interface PermissionSnapshot {
  authEnabled: boolean | null;
  permissions: ReadonlySet<string>;
  permissionStatus: PermissionStatus;
}

/**
 * 폴백 규칙을 포함한 단일 권한 판정.
 *
 * `permissionStatus` 가 'loaded' 도 'error' 도 아닌 값(미확정·구버전)은 모두
 * 전원 허용으로 수렴시킨다. 인증 도입 이전 동작을 유지하는 쪽이 안전하고,
 * 실제 차단은 서버가 수행한다.
 */
function resolve(snapshot: PermissionSnapshot, key: string): boolean {
  if (snapshot.authEnabled === false) return true;
  if (snapshot.permissionStatus === 'error') return false;
  if (snapshot.permissionStatus !== 'loaded') return true;
  return snapshot.permissions.has(key);
}

/**
 * 비-React 컨텍스트(스토어 콜백, 이벤트 핸들러)용 권한 판정.
 *
 * 훅과 동일한 폴백 규칙을 쓰되 구독하지 않고 현재 스냅샷만 읽는다.
 */
export function hasPermissionOf(key: string): boolean {
  const state = useAuthStore.getState();
  return resolve(state, key);
}

/** `usePermission()` 이 돌려주는 판정 수단. */
export interface PermissionApi {
  /** 단일 권한 보유 여부. */
  hasPermission: (key: string) => boolean;
  /** 나열한 권한 중 하나라도 보유하는지 여부. 빈 배열은 false. */
  hasAnyPermission: (keys: readonly string[]) => boolean;
  /**
   * 권한 조회에 실패한 상태인지 여부.
   *
   * 이 값이 true 면 모든 판정이 false 이므로, 호출부는 "권한 없음" 대신
   * 재시도 안내를 노출할 수 있다 (구버전 서버 폴백과 구분된다).
   */
  isPermissionUnavailable: boolean;
}

/**
 * 현재 사용자의 권한 판정 수단을 제공한다.
 *
 * 메뉴·라우트·액션 컨트롤 게이팅이 모두 이 훅을 통해 판정한다.
 */
export function usePermission(): PermissionApi {
  const authEnabled = useAuthStore((s) => s.authEnabled);
  const permissions = useAuthStore((s) => s.permissions);
  const permissionStatus = useAuthStore((s) => s.permissionStatus);

  const snapshot = useMemo<PermissionSnapshot>(
    () => ({ authEnabled, permissions, permissionStatus }),
    [authEnabled, permissions, permissionStatus],
  );

  const hasPermission = useCallback(
    (key: string) => resolve(snapshot, key),
    [snapshot],
  );

  const hasAnyPermission = useCallback(
    (keys: readonly string[]) => keys.some((key) => resolve(snapshot, key)),
    [snapshot],
  );

  return {
    hasPermission,
    hasAnyPermission,
    isPermissionUnavailable:
      authEnabled !== false && permissionStatus === 'error',
  };
}
