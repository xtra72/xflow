// 사용자·역할·권한 카탈로그 API 클라이언트 (SPEC-AUTH-006 M2.1).
//
// SPEC-AUTH-005 가 서버에 도입한 `/users`, `/roles`, `/permissions` 엔드포인트를
// 타입 안전하게 감싼다. client.ts 의 get/post/put/del 이 envelope 를 이미 언래핑
// 하므로 본 모듈은 도메인 페이로드만 다룬다.
//
// 보안: 어떤 응답 타입에도 password_hash 는 없다. 서버 DTO(internal/api/dto/user.go)
// 가 애초에 내려보내지 않으므로, 화면에 표시할 해시 컬럼도 존재하지 않는다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.1 — U2, AC-03/AC-04)

import { del, get, post, put } from './client';

// ---- 사용자 ----

/**
 * 사용자 목록·단건 응답 (서버 dto.UserResponse).
 *
 * created_at / updated_at 은 epoch milliseconds(int64) 이다. 프로젝트 전역
 * 타임스탬프 규약과 동일하므로 `formatEpochMs` 로 표시한다.
 */
export interface UserResponse {
  username: string;
  role: string;
  created_at: number;
  updated_at: number;
}

/** 사용자 등록 요청 (POST /users). */
export interface CreateUserRequest {
  username: string;
  password: string;
  role: string;
}

/** 사용자 역할 변경 요청 (PUT /users/{username}). */
export interface UpdateUserRequest {
  role: string;
}

/** 관리자 비밀번호 재설정 요청 (PUT /users/{username}/password). */
export interface ResetPasswordRequest {
  password: string;
}

// ---- 역할 ----

/** 역할 목록·단건 응답 (서버 dto.RoleResponse). */
export interface RoleResponse {
  name: string;
  description: string;
  builtin: boolean;
  permissions: string[];
  created_at: number;
  updated_at: number;
}

/** 역할 생성 요청 (POST /roles). */
export interface CreateRoleRequest {
  name: string;
  description: string;
  permissions: string[];
}

/**
 * 역할 수정 요청 (PUT /roles/{name}).
 *
 * 두 필드 모두 선택적이다. 생략한 필드는 서버가 변경하지 않는다.
 * - name: 역할 이름 변경. 해당 역할을 쓰던 users.role 도 서버가 함께 갱신한다.
 * - permissions: 권한 집합 전체 교체(부분 추가가 아니다).
 */
export interface UpdateRoleRequest {
  name?: string;
  permissions?: string[];
}

// ---- 권한 카탈로그 ----

/** 권한 키 카탈로그 응답 (GET /permissions — 인증만 요구). */
export interface PermissionCatalogResponse {
  permissions: string[];
}

// ---- 사용자 API ----

/** 사용자 목록을 조회한다. `user.read` 필요. */
export async function getUsers(): Promise<UserResponse[]> {
  return get<UserResponse[]>('/users');
}

/** 사용자를 등록한다. `user.create` 필요. */
export async function createUser(req: CreateUserRequest): Promise<UserResponse> {
  return post<UserResponse>('/users', req);
}

/** 사용자 역할을 변경한다. `user.update` 필요. */
export async function updateUserRole(
  username: string,
  req: UpdateUserRequest,
): Promise<UserResponse> {
  return put<UserResponse>(`/users/${encodeURIComponent(username)}`, req);
}

/**
 * 관리자 권한으로 사용자 비밀번호를 재설정한다. `user.update` 필요.
 *
 * 본인 비밀번호 변경(`PUT /auth/password`)과 달리 현재 비밀번호를 요구하지 않는다.
 */
export async function resetUserPassword(
  username: string,
  req: ResetPasswordRequest,
): Promise<void> {
  await put<void>(`/users/${encodeURIComponent(username)}/password`, req);
}

/** 사용자를 삭제한다. `user.delete` 필요. */
export async function deleteUser(username: string): Promise<void> {
  await del(`/users/${encodeURIComponent(username)}`);
}

// ---- 역할 API ----

/** 역할 목록을 조회한다. `role.read` 필요. 빌트인 3종 + 커스텀이 함께 반환된다. */
export async function getRoles(): Promise<RoleResponse[]> {
  return get<RoleResponse[]>('/roles');
}

/** 역할을 생성한다. `role.create` 필요. */
export async function createRole(req: CreateRoleRequest): Promise<RoleResponse> {
  return post<RoleResponse>('/roles', req);
}

/** 역할을 수정한다. `role.update` 필요. 생략한 필드는 변경되지 않는다. */
export async function updateRole(
  name: string,
  req: UpdateRoleRequest,
): Promise<RoleResponse> {
  return put<RoleResponse>(`/roles/${encodeURIComponent(name)}`, req);
}

/** 역할을 삭제한다. `role.delete` 필요. 빌트인 역할과 사용 중인 역할은 서버가 거부한다. */
export async function deleteRole(name: string): Promise<void> {
  await del(`/roles/${encodeURIComponent(name)}`);
}

// ---- 권한 카탈로그 API ----

/**
 * 권한 키 카탈로그를 조회한다. 인증만 필요하며 별도 권한은 요구하지 않는다.
 *
 * 응답은 `<resource>.<action>` 형식 문자열의 평면 배열이다. 역할 관리 화면의
 * 리소스×액션 행렬은 이 목록에서 축을 파생하므로, 서버 카탈로그가 바뀌면
 * 화면 코드를 고치지 않아도 행렬이 따라간다.
 */
export async function getPermissionCatalog(): Promise<string[]> {
  const data = await get<PermissionCatalogResponse>('/permissions');
  return data.permissions;
}
