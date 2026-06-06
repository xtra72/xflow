// remoteService 단위 테스트 (SPEC-REMOTE-001 M5).
//
// client get/post 헬퍼를 mock 하여 각 함수가 올바른 URL/메서드/바디로 호출하는지
// 검증한다. 에러 전파(APIError)도 함께 확인한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const patchMock = vi.hoisted(() => vi.fn());
const delMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  patch: patchMock,
  del: delMock,
}));

import { APIError } from '@/types/api';

import * as remoteService from './remoteService';

beforeEach(() => {
  getMock.mockReset();
  postMock.mockReset();
  patchMock.mockReset();
  delMock.mockReset();
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

describe('remoteService — 수동 등록 / 삭제', () => {
  it('preRegisterNode 는 POST /remote/nodes 를 본문과 함께 호출한다', async () => {
    postMock.mockResolvedValueOnce({
      instance_id: 'n-1',
      hostname: '',
      version: '',
      status: 'approved',
      online: false,
      last_seen: 0,
    });
    await remoteService.preRegisterNode({ instance_id: 'n-1', name: '게이트웨이' });
    expect(postMock).toHaveBeenCalledWith('/remote/nodes', {
      instance_id: 'n-1',
      name: '게이트웨이',
    });
  });

  it('preRegisterNode 의 409 에러는 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('CONFLICT', 'duplicate', 409));
    await expect(
      remoteService.preRegisterNode({ instance_id: 'dup' }),
    ).rejects.toBeInstanceOf(APIError);
  });

  it('deleteNode 는 DELETE /remote/nodes/{id} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteNode('n-1');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/n-1');
  });

  it('deleteNode 는 instance_id 를 URL 인코딩한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteNode('a/b c');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/a%2Fb%20c');
  });
});

describe('remoteService — Enrollment 토큰', () => {
  it('createEnrollmentToken 는 POST /remote/enrollment-tokens 를 본문과 함께 호출한다', async () => {
    postMock.mockResolvedValueOnce({ id: 't-1', token: 'raw-token' });
    const req = { label: '1층', expires_in: '24h', max_uses: 5 };
    await remoteService.createEnrollmentToken(req);
    expect(postMock).toHaveBeenCalledWith('/remote/enrollment-tokens', req);
  });

  it('listEnrollmentTokens 는 GET /remote/enrollment-tokens 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([]);
    await remoteService.listEnrollmentTokens();
    expect(getMock).toHaveBeenCalledWith('/remote/enrollment-tokens');
  });

  it('revokeEnrollmentToken 는 DELETE /remote/enrollment-tokens/{id} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.revokeEnrollmentToken('t-1');
    expect(delMock).toHaveBeenCalledWith('/remote/enrollment-tokens/t-1');
  });

  it('revokeEnrollmentToken 는 id 를 URL 인코딩한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.revokeEnrollmentToken('a/b');
    expect(delMock).toHaveBeenCalledWith('/remote/enrollment-tokens/a%2Fb');
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

describe('remoteService — 원격 자원 편집 (M7, 그룹 I)', () => {
  it('createRemoteFlow 는 POST /remote/nodes/{id}/flows 를 { name, definition } 으로 호출한다', async () => {
    postMock.mockResolvedValueOnce({ id: 'f-new', name: 'flow', status: '' });
    const req = { name: 'flow', definition: { nodes: [], wires: [] } };
    const res = await remoteService.createRemoteFlow('node-1', req);
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows', req);
    expect(res).toEqual({ id: 'f-new', name: 'flow', status: '' });
  });

  it('updateRemoteFlow 는 PATCH /remote/nodes/{id}/flows/{flowId} 를 호출한다', async () => {
    patchMock.mockResolvedValueOnce({ id: 'f1', name: 'flow', status: '' });
    const req = { definition: { nodes: [] } };
    await remoteService.updateRemoteFlow('node-1', 'f1', req);
    expect(patchMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows/f1', req);
  });

  it('updateRemoteFlow 는 instance_id 와 flow_id 를 URL 인코딩한다', async () => {
    patchMock.mockResolvedValueOnce({ id: 'a/b', name: '', status: '' });
    await remoteService.updateRemoteFlow('n/1', 'a/b', { definition: {} });
    expect(patchMock).toHaveBeenCalledWith(
      '/remote/nodes/n%2F1/flows/a%2Fb',
      { definition: {} },
    );
  });

  it('deleteRemoteFlow 는 DELETE /remote/nodes/{id}/flows/{flowId} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteRemoteFlow('node-1', 'f1');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows/f1');
  });

  it('createRemoteAgent 는 POST /remote/nodes/{id}/agents 를 { name, type, config } 으로 호출한다', async () => {
    postMock.mockResolvedValueOnce({ id: 'a-new', name: 'a', status: '' });
    const req = { name: 'a', type: 'mqtt', config: { host: 'h' } };
    await remoteService.createRemoteAgent('node-1', req);
    expect(postMock).toHaveBeenCalledWith('/remote/nodes/node-1/agents', req);
  });

  it('updateRemoteAgent 는 PATCH /remote/nodes/{id}/agents/{agentId} 를 호출한다', async () => {
    patchMock.mockResolvedValueOnce({ id: 'a1', name: 'a', status: '' });
    const req = { config: { host: 'h' } };
    await remoteService.updateRemoteAgent('node-1', 'a1', req);
    expect(patchMock).toHaveBeenCalledWith('/remote/nodes/node-1/agents/a1', req);
  });

  it('deleteRemoteAgent 는 DELETE /remote/nodes/{id}/agents/{agentId} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteRemoteAgent('node-1', 'a1');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/node-1/agents/a1');
  });

  it('편집 함수의 502/503/504/404 에러는 호출자로 전파된다', async () => {
    patchMock.mockRejectedValueOnce(new APIError('BAD_GATEWAY', 'apply fail', 502));
    await expect(
      remoteService.updateRemoteFlow('node-1', 'f1', { definition: {} }),
    ).rejects.toBeInstanceOf(APIError);

    postMock.mockRejectedValueOnce(
      new APIError('SERVICE_UNAVAILABLE', 'offline', 503),
    );
    await expect(
      remoteService.createRemoteFlow('node-1', { name: 'x', definition: {} }),
    ).rejects.toBeInstanceOf(APIError);
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
