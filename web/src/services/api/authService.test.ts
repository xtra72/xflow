// authService 단위 테스트 (SPEC-AUTH-006 M1.3).
//
// 범위: `GET /auth/me` 응답 매핑. 서버는 `username` 을 보내고 클라이언트 도메인
//       타입은 `name` 이며 client.ts 의 get 은 키를 변환하지 않는다. 이전 구현은
//       `get<User>('/auth/me')` 로 단언만 해서 세션 복원 시 user.name 이
//       undefined 였다 — 그 회귀를 이 테스트가 고정한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
vi.mock('./client', () => ({
  get: getMock,
  post: vi.fn(),
  put: vi.fn(),
}));

import { getCurrentUser } from './authService';

beforeEach(() => {
  getMock.mockReset();
});

describe('getCurrentUser — /auth/me 응답 매핑', () => {
  it('서버의 username 을 도메인 타입의 name 으로 매핑한다', async () => {
    getMock.mockResolvedValue({
      username: 'alice',
      role: 'operator',
      permissions: ['agent.read'],
    });

    const user = await getCurrentUser();

    expect(getMock).toHaveBeenCalledWith('/auth/me');
    // 회귀 방지의 핵심: 매핑이 없으면 name 은 undefined 가 된다.
    expect(user.name).toBe('alice');
    expect(user.role).toBe('operator');
  });

  it('permissions 배열을 그대로 전달한다', async () => {
    getMock.mockResolvedValue({
      username: 'alice',
      role: 'viewer',
      permissions: ['agent.read', 'device.read'],
    });

    const user = await getCurrentUser();

    expect(user.permissions).toEqual(['agent.read', 'device.read']);
  });

  it('permissions 가 없는 구버전 서버 응답은 undefined 로 남긴다', async () => {
    // 빈 배열로 정규화하면 "권한 0개"와 구분되지 않아 폴백 판단이 무너진다.
    getMock.mockResolvedValue({ username: 'alice', role: 'admin' });

    const user = await getCurrentUser();

    expect(user.name).toBe('alice');
    expect(user.permissions).toBeUndefined();
  });

  it('빈 permissions 배열은 undefined 로 뭉개지 않는다', async () => {
    getMock.mockResolvedValue({
      username: 'alice',
      role: 'norole',
      permissions: [],
    });

    const user = await getCurrentUser();

    expect(user.permissions).toEqual([]);
  });
});
