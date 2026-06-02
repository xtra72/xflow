// nodeService.configureNode 단위 테스트 — 노드 라이브 설정 적용 경로.
//
// configureNode 는 실행 중 플로우의 노드 설정을 저장/재배포 없이 즉시
// 적용한다. 요청 URL/바디 형상과 에러 전파(404 포함) 동작을 검증한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: vi.fn(),
  post: postMock,
}));

import { APIError } from '@/types/api';

import { configureNode } from './nodeService';

describe('configureNode', () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it('올바른 URL 과 { config } 바디로 POST 한다', async () => {
    postMock.mockResolvedValueOnce(undefined);

    await configureNode('flow-1', 'node-9', { output_enabled: false });

    expect(postMock).toHaveBeenCalledTimes(1);
    expect(postMock).toHaveBeenCalledWith(
      '/flows/flow-1/nodes/node-9/configure',
      { config: { output_enabled: false } },
    );
  });

  it('성공 시 void 로 resolve 한다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await expect(
      configureNode('f', 'n', { output_enabled: true }),
    ).resolves.toBeUndefined();
  });

  it('404 (실행 중 아님/노드 없음) 는 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not running', 404));
    await expect(
      configureNode('f', 'n', { output_enabled: false }),
    ).rejects.toBeInstanceOf(APIError);
  });

  it('서버 오류(500) 도 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('INTERNAL', 'boom', 500));
    await expect(
      configureNode('f', 'n', { output_enabled: false }),
    ).rejects.toBeInstanceOf(APIError);
  });
});
