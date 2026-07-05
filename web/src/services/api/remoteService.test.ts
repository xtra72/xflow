// remoteService 단위 테스트 (SPEC-REMOTE-001 M5).
//
// client get/post 헬퍼를 mock 하여 각 함수가 올바른 URL/메서드/바디로 호출하는지
// 검증한다. 에러 전파(APIError)도 함께 확인한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const patchMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());
const delMock = vi.hoisted(() => vi.fn());
// apiClient 는 multipart 업로드(uploadReleaseAsset)에서 직접 사용된다.
const apiClientPostMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  patch: patchMock,
  put: putMock,
  del: delMock,
  apiClient: { post: apiClientPostMock },
}));

import { APIError } from '@/types/api';

import * as remoteService from './remoteService';

beforeEach(() => {
  getMock.mockReset();
  postMock.mockReset();
  patchMock.mockReset();
  putMock.mockReset();
  delMock.mockReset();
  apiClientPostMock.mockReset();
});

describe('remoteService — 릴리스 저장소', () => {
  it('listReleases 는 GET /remote/releases 를 호출하고 .releases 를 언래핑한다', async () => {
    getMock.mockResolvedValueOnce({ releases: [{ version: 'v1.0.0' }] });
    const res = await remoteService.listReleases();
    expect(getMock).toHaveBeenCalledWith('/remote/releases');
    expect(res).toEqual([{ version: 'v1.0.0' }]);
  });

  it('listReleases 는 releases 누락 시 빈 배열을 반환한다', async () => {
    getMock.mockResolvedValueOnce({});
    const res = await remoteService.listReleases();
    expect(res).toEqual([]);
  });

  it('createRelease 는 POST /remote/releases 를 본문과 함께 호출한다', async () => {
    postMock.mockResolvedValueOnce({ version: 'v1.3.0', channel: 'beta' });
    const req = { version: 'v1.3.0', channel: 'beta', notes: '변경' };
    await remoteService.createRelease(req);
    expect(postMock).toHaveBeenCalledWith('/remote/releases', req);
  });

  it('createRelease 의 400 에러는 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('BAD_REQUEST', 'invalid semver', 400));
    await expect(
      remoteService.createRelease({ version: 'bad' }),
    ).rejects.toBeInstanceOf(APIError);
  });

  it('uploadReleaseAsset 는 apiClient.post 로 multipart FormData 를 전송한다', async () => {
    apiClientPostMock.mockResolvedValueOnce({ data: { os: 'linux', arch: 'amd64' } });
    const binary = new File(['bin'], 'xflowd', { type: 'application/octet-stream' });
    const signature = new File(['sig'], 'xflowd.sig', { type: 'application/octet-stream' });
    const res = await remoteService.uploadReleaseAsset(
      'v1.3.0',
      'linux',
      'amd64',
      binary,
      signature,
    );
    expect(apiClientPostMock).toHaveBeenCalledTimes(1);
    const [url, form] = apiClientPostMock.mock.calls[0] as [string, FormData];
    expect(url).toBe('/remote/releases/v1.3.0/assets');
    expect(form).toBeInstanceOf(FormData);
    expect(form.get('os')).toBe('linux');
    expect(form.get('arch')).toBe('amd64');
    expect(form.get('binary')).toBe(binary);
    expect(form.get('signature')).toBe(signature);
    expect(res).toEqual({ os: 'linux', arch: 'amd64' });
  });

  it('uploadReleaseAsset 는 version 을 URL 인코딩한다', async () => {
    apiClientPostMock.mockResolvedValueOnce({ data: {} });
    const f = new File(['x'], 'x');
    await remoteService.uploadReleaseAsset('v1/3', 'linux', 'arm', f, f);
    const [url] = apiClientPostMock.mock.calls[0] as [string];
    expect(url).toBe('/remote/releases/v1%2F3/assets');
  });

  it('deleteRelease 는 DELETE /remote/releases/{version} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteRelease('v1.3.0');
    expect(delMock).toHaveBeenCalledWith('/remote/releases/v1.3.0');
  });

  it('deleteReleaseAsset 는 DELETE /remote/releases/{version}/assets/{os}/{arch} 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.deleteReleaseAsset('v1.3.0', 'linux', 'amd64');
    expect(delMock).toHaveBeenCalledWith(
      '/remote/releases/v1.3.0/assets/linux/amd64',
    );
  });
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

describe('remoteService — 노드 그룹핑 + 상세 (M9, 그룹 K)', () => {
  it('listRemoteGroups 는 GET /remote/groups 를 호출한다', async () => {
    getMock.mockResolvedValueOnce([{ group_name: '', node_count: 2 }]);
    const res = await remoteService.listRemoteGroups();
    expect(getMock).toHaveBeenCalledWith('/remote/groups');
    expect(res).toEqual([{ group_name: '', node_count: 2 }]);
  });

  it('getRemoteNodeDetail 는 GET /remote/nodes/{id} 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ instance_id: 'n-1', uptime: null });
    await remoteService.getRemoteNodeDetail('n-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/n-1');
  });

  it('getRemoteNodeDetail 는 instance_id 를 URL 인코딩한다', async () => {
    getMock.mockResolvedValueOnce({});
    await remoteService.getRemoteNodeDetail('a/b');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/a%2Fb');
  });

  it('setRemoteNodeGroup 는 PUT .../group 을 group_name 본문과 함께 호출한다', async () => {
    putMock.mockResolvedValueOnce(undefined);
    await remoteService.setRemoteNodeGroup('n-1', 'prod');
    expect(putMock).toHaveBeenCalledWith('/remote/nodes/n-1/group', {
      group_name: 'prod',
    });
  });

  it('clearRemoteNodeGroup 는 DELETE .../group 을 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.clearRemoteNodeGroup('n-1');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/n-1/group');
  });

  it('setRemoteNodeDisplay 는 PUT .../display 를 width/height 본문과 함께 호출한다', async () => {
    putMock.mockResolvedValueOnce(undefined);
    await remoteService.setRemoteNodeDisplay('n-1', 1920, 1080);
    expect(putMock).toHaveBeenCalledWith('/remote/nodes/n-1/display', {
      width: 1920,
      height: 1080,
    });
  });

  it('setRemoteNodeDisplay 는 instance_id 를 URL 인코딩한다', async () => {
    putMock.mockResolvedValueOnce(undefined);
    await remoteService.setRemoteNodeDisplay('a/b', 800, 480);
    expect(putMock).toHaveBeenCalledWith('/remote/nodes/a%2Fb/display', {
      width: 800,
      height: 480,
    });
  });

  it('setRemoteNodeDisplay 의 400 에러는 호출자로 전파된다', async () => {
    putMock.mockRejectedValueOnce(new APIError('BAD_REQUEST', 'non-positive', 400));
    await expect(
      remoteService.setRemoteNodeDisplay('n-1', 0, 0),
    ).rejects.toBeInstanceOf(APIError);
  });

  it('clearRemoteNodeDisplay 는 DELETE .../display 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.clearRemoteNodeDisplay('n-1');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/n-1/display');
  });

  it('clearRemoteNodeDisplay 는 instance_id 를 URL 인코딩한다', async () => {
    delMock.mockResolvedValueOnce(undefined);
    await remoteService.clearRemoteNodeDisplay('a/b');
    expect(delMock).toHaveBeenCalledWith('/remote/nodes/a%2Fb/display');
  });

  it('getRemoteNodeDetail 의 404 에러는 호출자로 전파된다', async () => {
    getMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'missing', 404));
    await expect(remoteService.getRemoteNodeDetail('x')).rejects.toBeInstanceOf(
      APIError,
    );
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

describe('remoteService — 노드별 라이브 목록 (M8, 그룹 J, REQ-J04)', () => {
  it('getRemoteAgentsLive 는 GET .../agents/live 를 호출하고 내부 .data 를 언래핑한다', async () => {
    // 이중 중첩 본문: axios 인터셉터가 바깥 envelope 을 벗긴 뒤 호출자에게 노드
    // 본문 { data: [...] } 가 도달한다. 서비스 함수는 내부 .data 를 언래핑한다.
    getMock.mockResolvedValueOnce({
      data: [{ id: 'a1', name: 'A1', type: 'mqtt', status: 'running', connected: true }],
    });
    const res = await remoteService.getRemoteAgentsLive('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/agents/live');
    expect(res).toHaveLength(1);
    expect(res[0]!.id).toBe('a1');
    expect(res[0]!.connected).toBe(true);
  });

  it('getRemoteFlowsLive 는 GET .../flows/live 를 호출하고 내부 .data 를 언래핑한다', async () => {
    getMock.mockResolvedValueOnce({
      data: [{ id: 'f1', name: 'F1', status: 'running', node_count: 2 }],
    });
    const res = await remoteService.getRemoteFlowsLive('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows/live');
    expect(res[0]!.node_count).toBe(2);
  });

  it('getRemoteDevicesLive 는 GET .../devices/live 를 호출하고 내부 .data 를 언래핑한다', async () => {
    getMock.mockResolvedValueOnce({
      data: [{ id: 'd1', name: 'D1', online: true }],
    });
    const res = await remoteService.getRemoteDevicesLive('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/devices/live');
    expect(res[0]!.online).toBe(true);
  });

  it('내부 data 가 누락되면 빈 배열로 폴백한다(방어적)', async () => {
    getMock.mockResolvedValueOnce({});
    const res = await remoteService.getRemoteAgentsLive('node-1');
    expect(res).toEqual([]);
  });

  it('instance_id 를 URL 인코딩한다', async () => {
    getMock.mockResolvedValueOnce({ data: [] });
    await remoteService.getRemoteAgentsLive('node/with space');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node%2Fwith%20space/agents/live');
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

describe('remoteService — READ/QUERY 프록시 (M8, 그룹 J)', () => {
  it('getRemoteFlow 는 GET .../flows/{id} 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ id: 'f1' });
    await remoteService.getRemoteFlow('node-1', 'f1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/flows/f1');
  });

  it('getRemoteFlowStatus / getRemoteFlowNodes 는 하위 경로를 호출한다', async () => {
    getMock.mockResolvedValue({});
    await remoteService.getRemoteFlowStatus('node-1', 'f1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/flows/f1/status');
    await remoteService.getRemoteFlowNodes('node-1', 'f1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/flows/f1/nodes');
  });

  it('getRemoteFlowNode 는 node_id 를 인코딩해 호출한다', async () => {
    getMock.mockResolvedValueOnce({});
    await remoteService.getRemoteFlowNode('n/1', 'f/1', 'nd 1');
    expect(getMock).toHaveBeenCalledWith(
      '/remote/nodes/n%2F1/flows/f%2F1/nodes/nd%201',
    );
  });

  it('agent READ 프록시는 각 하위 경로를 호출한다', async () => {
    getMock.mockResolvedValue({});
    await remoteService.getRemoteAgent('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1');
    await remoteService.getRemoteAgentStats('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/stats');
    await remoteService.getRemoteAgentConfig('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/config');
    await remoteService.getRemoteAgentDevices('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/devices');
    await remoteService.getRemoteAgentTopics('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/topics');
    await remoteService.getRemoteAgentStore('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/store');
    await remoteService.getRemoteAgentSessions('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/sessions');
    await remoteService.getRemoteAgentSeries('node-1', 'a1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/agents/a1/series');
  });

  it('device READ 프록시는 각 하위 경로를 호출한다', async () => {
    getMock.mockResolvedValue({});
    await remoteService.getRemoteDevice('node-1', 'd1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/devices/d1');
    await remoteService.getRemoteDeviceState('node-1', 'd1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/devices/d1/state');
    await remoteService.getRemoteDeviceCommands('node-1', 'd1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/devices/d1/commands');
    await remoteService.getRemoteDeviceMetadata('node-1', 'd1');
    expect(getMock).toHaveBeenLastCalledWith('/remote/nodes/node-1/devices/d1/metadata');
  });

  it('프록시 GET 의 502/503/504/404 에러는 호출자로 전파된다', async () => {
    getMock.mockRejectedValueOnce(new APIError('SERVICE_UNAVAILABLE', 'offline', 503));
    await expect(
      remoteService.getRemoteDeviceState('node-1', 'd1'),
    ).rejects.toBeInstanceOf(APIError);
    getMock.mockRejectedValueOnce(new APIError('QUERY_TIMEOUT', 'timeout', 504));
    await expect(
      remoteService.getRemoteAgentStats('node-1', 'a1'),
    ).rejects.toBeInstanceOf(APIError);
    getMock.mockRejectedValueOnce(new APIError('QUERY_FAILED', 'node error', 502));
    await expect(
      remoteService.getRemoteFlow('node-1', 'f1'),
    ).rejects.toBeInstanceOf(APIError);
    getMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'out of scope', 404));
    await expect(
      remoteService.getRemoteAgent('node-1', 'a1'),
    ).rejects.toBeInstanceOf(APIError);
  });
});

describe('remoteService — 라이브 스트림 URL (M8, REQ-J08)', () => {
  it('device.state SSE URL 을 구성한다', () => {
    expect(
      remoteService.remoteStreamUrl('node-1', { domain: 'device', action: 'state' }, 'd1'),
    ).toBe('/api/v1/remote/nodes/node-1/devices/d1/state/stream');
  });

  it('agent.stats / agent.series SSE URL 을 구성한다', () => {
    expect(
      remoteService.remoteStreamUrl('node-1', { domain: 'agent', action: 'stats' }, 'a1'),
    ).toBe('/api/v1/remote/nodes/node-1/agents/a1/stats/stream');
    expect(
      remoteService.remoteStreamUrl('node-1', { domain: 'agent', action: 'series' }, 'a1'),
    ).toBe('/api/v1/remote/nodes/node-1/agents/a1/series/stream');
  });

  it('token 이 주어지면 ?token= 쿼리로 부착하고 인코딩한다', () => {
    expect(
      remoteService.remoteStreamUrl(
        'n/1',
        { domain: 'device', action: 'state' },
        'd 1',
        'jwt.a/b',
      ),
    ).toBe('/api/v1/remote/nodes/n%2F1/devices/d%201/state/stream?token=jwt.a%2Fb');
  });
});

describe('remoteService — 원격 대시보드 패리티 (M10, 그룹 L)', () => {
  it('getRemoteDashboard(shared) 는 GET .../dashboards/shared 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ payload: {} });
    await remoteService.getRemoteDashboard('node-1', 'shared');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/dashboards/shared');
  });

  it('getRemoteDashboard(mine) 는 GET .../dashboards/mine 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ payload: {} });
    await remoteService.getRemoteDashboard('node-1', 'mine');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/dashboards/mine');
  });

  it('getRemoteDashboard 의 instance_id 를 URL 인코딩한다', async () => {
    getMock.mockResolvedValueOnce({ payload: {} });
    await remoteService.getRemoteDashboard('n/1', 'shared');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/n%2F1/dashboards/shared');
  });

  it('getRemoteMetrics 는 GET .../metrics 를 호출한다', async () => {
    getMock.mockResolvedValueOnce({ cpu: 1 });
    await remoteService.getRemoteMetrics('node-1');
    expect(getMock).toHaveBeenCalledWith('/remote/nodes/node-1/metrics');
  });

  it('remoteChartStreamUrl 은 .../charts/{channel}/stream SSE URL 을 구성한다', () => {
    expect(remoteService.remoteChartStreamUrl('node-1', 'temp')).toBe(
      '/api/v1/remote/nodes/node-1/charts/temp/stream',
    );
  });

  it('remoteChartStreamUrl 은 token 을 ?token= 쿼리로 부착·인코딩한다', () => {
    expect(remoteService.remoteChartStreamUrl('n/1', 'c h', 'jwt/x')).toBe(
      '/api/v1/remote/nodes/n%2F1/charts/c%20h/stream?token=jwt%2Fx',
    );
  });

  it('remoteLogsStreamUrl 은 .../logs/stream SSE URL 을 구성한다', () => {
    expect(remoteService.remoteLogsStreamUrl('node-1')).toBe(
      '/api/v1/remote/nodes/node-1/logs/stream',
    );
  });

  it('remoteLogsStreamUrl 은 token 을 부착한다', () => {
    expect(remoteService.remoteLogsStreamUrl('node-1', 'tok')).toBe(
      '/api/v1/remote/nodes/node-1/logs/stream?token=tok',
    );
  });
});
