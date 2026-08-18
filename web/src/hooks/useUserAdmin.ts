// 사용자·역할·권한 카탈로그 React Query 훅 (SPEC-AUTH-006 M2).
//
// 사용자 관리(`/admin/users`)와 역할 관리(`/admin/roles`) 두 화면이 역할 목록을
// 공유하므로, 쿼리 키와 무효화 규칙을 한 곳에 모아 두 화면이 서로 다른 키를 쓰는
// drift 를 막는다.
//
// 재시도: 권한 오류(403)나 잠금 방지 거부(409)는 같은 요청을 다시 보내도 결과가
// 같으므로 조회 재시도를 끈다 (spec.md §2.5 UB1-2 — 무한 재시도 금지).
//
// @spec SPEC-AUTH-006 v0.1.0 (M2 — U2, AC-03/AC-04)

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as userService from '@/services/api/userService';
import type {
  CreateRoleRequest,
  CreateUserRequest,
  UpdateRoleRequest,
} from '@/services/api/userService';

// ---- 쿼리 키 ----

const usersKey = ['admin', 'users'] as const;
const rolesKey = ['admin', 'roles'] as const;
const permissionCatalogKey = ['admin', 'permission-catalog'] as const;

// ---- 조회 ----

/** 사용자 목록. `user.read` 가 없으면 호출부가 `enabled: false` 로 막는다. */
export function useAdminUsers(enabled = true) {
  return useQuery({
    queryKey: usersKey,
    queryFn: userService.getUsers,
    enabled,
    retry: false,
  });
}

/**
 * 역할 목록. 사용자 등록 폼의 역할 선택지와 역할 관리 화면이 함께 소비한다.
 *
 * `role.read` 가 없는 사용자(예: user.create 만 보유)는 서버가 403 을 반환하므로
 * 호출부가 `enabled` 로 막고 대체 입력을 제공한다.
 */
export function useAdminRoles(enabled = true) {
  return useQuery({
    queryKey: rolesKey,
    queryFn: userService.getRoles,
    enabled,
    retry: false,
  });
}

/**
 * 권한 키 카탈로그. 인증만 요구하며 프로세스 수명 동안 사실상 불변이므로
 * 자주 갱신하지 않는다.
 */
export function usePermissionCatalog(enabled = true) {
  return useQuery({
    queryKey: permissionCatalogKey,
    queryFn: userService.getPermissionCatalog,
    enabled,
    retry: false,
    staleTime: 5 * 60 * 1000,
  });
}

// ---- 사용자 변경 ----

/**
 * 사용자 목록을 다시 읽는다.
 *
 * 실패한 뮤테이션에서는 호출하지 않는다 — 서버가 409 로 거부한 경우 목록 상태가
 * 그대로 유지되어야 한다 (AC-03).
 */
function useInvalidateUsers() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: usersKey });
}

export function useCreateUser() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: (req: CreateUserRequest) => userService.createUser(req),
    onSuccess: invalidate,
  });
}

export function useUpdateUserRole() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: ({ username, role }: { username: string; role: string }) =>
      userService.updateUserRole(username, { role }),
    onSuccess: invalidate,
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) =>
      userService.resetUserPassword(username, { password }),
  });
}

export function useDeleteUser() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: (username: string) => userService.deleteUser(username),
    onSuccess: invalidate,
  });
}

// ---- 역할 변경 ----

/**
 * 역할 목록을 다시 읽는다.
 *
 * 역할 이름 변경은 서버에서 users.role 로 전파되므로 사용자 목록도 함께 무효화한다.
 */
function useInvalidateRoles() {
  const queryClient = useQueryClient();
  return () => {
    queryClient.invalidateQueries({ queryKey: rolesKey });
    queryClient.invalidateQueries({ queryKey: usersKey });
  };
}

export function useCreateRole() {
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: (req: CreateRoleRequest) => userService.createRole(req),
    onSuccess: invalidate,
  });
}

export function useUpdateRole() {
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: ({ name, req }: { name: string; req: UpdateRoleRequest }) =>
      userService.updateRole(name, req),
    onSuccess: invalidate,
  });
}

export function useDeleteRole() {
  const invalidate = useInvalidateRoles();
  return useMutation({
    mutationFn: (name: string) => userService.deleteRole(name),
    onSuccess: invalidate,
  });
}
