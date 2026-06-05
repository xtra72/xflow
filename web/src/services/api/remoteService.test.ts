// remoteService 단위 테스트 (SPEC-REMOTE-001 M5).
//
// client get/post 헬퍼를 mock 하여 각 함수가 올바른 URL/메서드/바디로 호출하는지
// 검증한다. 에러 전파(APIError)도 함께 확인한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
}));

import { APIError } from '@/types/api';

import * as remoteService from './remoteService';

beforeEach(() => {
  getMock.mockReset();
  postMock.mockReset();
});

describe('remoteService — 동작 모드', () => {
  it('getRemoteMode 는 GET /remote/mode 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ mode: 'server' });
    const res = await remoteService.getRemoteMode();
    expect(getMock).toHaveBeenCalledWith('/remote/mode');
    expect(res).toEqual({ mode: 'server' });
  });
});

describe('remoteService — 목록 조회', () => {
  it('listNodes 는 GET /remote/nodes 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listNodes();
    expect(getMock).toHaveBeenCalledWith('/remote/nodes');
  });

  it('listPendingNodes 는 GET /remote/nodes/pending 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listPendingNodes();
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/pending');
  });
});

describe('remoteService — 상태 머신 액션', () => {
  it('approveNode 는 POST /remote/nodes/{id}/approve 를 호출한다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await remoteService.approveNode('node-1');
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/approve');
  });

  it('rejectNode 는 사유 없이 본문 undefined 로 POST 한다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await remoteService.rejectNode('node-1');
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/reject', undefined);
  });

  it('rejectNode 는 사유가 있으면 { reason } 본문으로 POST 한다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await remoteService.rejectNode('node-1', '미인가 노드');
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/reject', {
      reason: '미인가 노드',
    });
  });

  it('revokeNode 는 POST /remote/nodes/{id}/revoke 를 호출한다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await remoteService.revokeNode('node-1');
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/revoke');
  });

  it('instance_id 는 URL 인코딩된다', async () => {
    postMock.mockResolvedValueOnce(undefined);
    await remoteService.approveNode('a/b c');
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/a%2Fb%20c/approve');
  });
});

describe('remoteService — 명령 발행', () => {
  it('sendCommand 는 POST /remote/nodes/{id}/command 를 { domain, action, args } 로 호출한다', async () => {
    postMock.mockResolvedValueOnce({
      instance_id: 'node-1',
      domain: 'flow',
      action: 'start',
      result: null,
    });
    const req = { domain: 'flow', action: 'start', args: { id: 'f1' } };
    await remoteService.sendCommand('node-1', req);
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/command', req);
  });

  it('sendCommand 의 503 에러는 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('SERVICE_UNAVAILABLE', 'offline', 503));
    await expect(
      remoteService.sendCommand('node-1', { domain: 'flow', action: 'start' }),
    ).rejects.toBeInstanceOf(APIError);
  });
});

describe('remoteService — 노드별 미러', () => {
  it('listNodeFlows 는 GET /remote/nodes/{id}/flows 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listNodeFlows('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows');
  });

  it('listNodeAgents 는 GET /remote/nodes/{id}/agents 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listNodeAgents('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/agents');
  });

  it('listNodeDevices 는 GET /remote/nodes/{id}/devices 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listNodeDevices('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/devices');
  });
});

describe('remoteService — 통합 미러', () => {
  it('listAllFlows 는 GET /remote/flows 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listAllFlows();
    expect(getMock).toHaveBeenCalledWith('/remote/flows');
  });

  it('listAllAgents 는 GET /remote/agents 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listAllAgents();
    expect(getMock).toHaveBeenCalledWith('/remote/agents');
  });

  it('listAllDevices 는 GET /remote/devices 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listAllDevices();
    expect(getMock).toHaveBeenCalledWith('/remote/devices');
  });
});
