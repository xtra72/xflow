// userService 단위 테스트 (SPEC-AUTH-006 M2.1).
//
// 범위: 각 함수가 올바른 메서드·경로·본문으로 요청하는지. 경로 세그먼트에 들어가는
// username / 역할 이름은 인코딩되어야 한다 — 인코딩이 빠지면 특수문자가 든 이름에서
// 경로가 깨진다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.1)

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());
const delMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: putMock,
  del: delMock,
}));

import * as userService from './userService';

beforeEach(() => {
  vi.clearAllMocks();
});

describe('사용자 API', () => {
  it('GET /users 로 목록을 조회한다', async () => {
    const users = [{ username: 'alice', role: 'editor', created_at: 1, updated_at: 2 }];
    getMock.mockResolvedValue(users);

    await expect(userService.getUsers()).resolves.toEqual(users);
    expect(getMock).toHaveBeenCalledWith('/users');
  });

  it('POST /users 로 username·password·role 을 보낸다', async () => {
    postMock.mockResolvedValue({});
    await userService.createUser({
      username: 'carol',
      password: 'secret-password',
      role: 'operator',
    });

    expect(postMock).toHaveBeenCalledWith('/users', {
      username: 'carol',
      password: 'secret-password',
      role: 'operator',
    });
  });

  it('PUT /users/{username} 으로 역할만 보낸다', async () => {
    putMock.mockResolvedValue({});
    await userService.updateUserRole('alice', { role: 'viewer' });

    expect(putMock).toHaveBeenCalledWith('/users/alice', { role: 'viewer' });
  });

  it('PUT /users/{username}/password 로 새 비밀번호만 보낸다 (현재 비밀번호 불요)', async () => {
    putMock.mockResolvedValue(undefined);
    await userService.resetUserPassword('alice', { password: 'new-password' });

    expect(putMock).toHaveBeenCalledWith('/users/alice/password', {
      password: 'new-password',
    });
  });

  it('DELETE /users/{username} 으로 삭제한다', async () => {
    delMock.mockResolvedValue(undefined);
    await userService.deleteUser('alice');

    expect(delMock).toHaveBeenCalledWith('/users/alice');
  });

  it('경로 세그먼트의 username 을 인코딩한다', async () => {
    delMock.mockResolvedValue(undefined);
    await userService.deleteUser('a/b c');

    expect(delMock).toHaveBeenCalledWith('/users/a%2Fb%20c');
  });
});

describe('역할 API', () => {
  it('GET /roles 로 목록을 조회한다', async () => {
    const roles = [
      {
        name: 'admin',
        description: '전체 권한',
        builtin: true,
        permissions: ['user.read'],
        created_at: 1,
        updated_at: 2,
      },
    ];
    getMock.mockResolvedValue(roles);

    await expect(userService.getRoles()).resolves.toEqual(roles);
    expect(getMock).toHaveBeenCalledWith('/roles');
  });

  it('POST /roles 로 이름·설명·권한 목록을 보낸다', async () => {
    postMock.mockResolvedValue({});
    await userService.createRole({
      name: 'auditor',
      description: '감사',
      permissions: ['agent.read'],
    });

    expect(postMock).toHaveBeenCalledWith('/roles', {
      name: 'auditor',
      description: '감사',
      permissions: ['agent.read'],
    });
  });

  it('PUT /roles/{name} 은 전달한 필드만 보낸다 (부분 갱신)', async () => {
    putMock.mockResolvedValue({});
    await userService.updateRole('operator', { permissions: ['agent.read'] });

    expect(putMock).toHaveBeenCalledWith('/roles/operator', {
      permissions: ['agent.read'],
    });
  });

  it('PUT /roles/{name} 으로 이름 변경(rename)을 보낼 수 있다', async () => {
    putMock.mockResolvedValue({});
    await userService.updateRole('operator', { name: 'operator-2' });

    expect(putMock).toHaveBeenCalledWith('/roles/operator', { name: 'operator-2' });
  });

  it('DELETE /roles/{name} 으로 삭제하며 이름을 인코딩한다', async () => {
    delMock.mockResolvedValue(undefined);
    await userService.deleteRole('a b');

    expect(delMock).toHaveBeenCalledWith('/roles/a%20b');
  });
});

describe('권한 카탈로그 API', () => {
  it('GET /permissions 응답의 permissions 배열을 평면 배열로 돌려준다', async () => {
    getMock.mockResolvedValue({ permissions: ['agent.read', 'node.read'] });

    await expect(userService.getPermissionCatalog()).resolves.toEqual([
      'agent.read',
      'node.read',
    ]);
    expect(getMock).toHaveBeenCalledWith('/permissions');
  });
});
