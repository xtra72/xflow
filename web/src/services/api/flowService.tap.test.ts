// flowService 노드 출력 tap(관찰) API 단위 테스트.
//
// setNodeTap 은 노드 tap 상태를 토글(POST /tap)하고, getFlowTaps 는 현재 관찰
// 중인 노드 목록(GET /taps)을 조회한다. 요청 URL/바디 형상과 응답 전파를 검증한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: vi.fn(),
  del: vi.fn(),
  getList: vi.fn(),
}));

import { APIError } from '@/types/api';

import { getFlowTaps, setNodeTap } from './flowService';

describe('setNodeTap', () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it('enabled=true 로 올바른 URL 과 { enabled } 바디로 POST 한다', async () => {
    postMock.mockResolvedValueOnce({
      flow_id: 'flow-1',
      node_id: 'node-9',
      enabled: true,
    });

    const res = await setNodeTap('flow-1', 'node-9', true);

    expect(postMock).toHaveBeenCalledTimes(1);
    expect(postMock).toHaveBeenCalledWith('/flows/flow-1/nodes/node-9/tap', {
      enabled: true,
    });
    expect(res).toEqual({ flow_id: 'flow-1', node_id: 'node-9', enabled: true });
  });

  it('enabled=false 로 토글 OFF 를 POST 한다', async () => {
    postMock.mockResolvedValueOnce({
      flow_id: 'f',
      node_id: 'n',
      enabled: false,
    });

    await setNodeTap('f', 'n', false);

    expect(postMock).toHaveBeenCalledWith('/flows/f/nodes/n/tap', {
      enabled: false,
    });
  });

  it('서버 오류는 호출자로 전파된다', async () => {
    postMock.mockRejectedValueOnce(new APIError('INTERNAL', 'boom', 500));
    await expect(setNodeTap('f', 'n', true)).rejects.toBeInstanceOf(APIError);
  });
});

describe('getFlowTaps', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('올바른 URL 로 GET 하고 node_ids 를 반환한다', async () => {
    getMock.mockResolvedValueOnce({
      flow_id: 'flow-1',
      node_ids: ['node-1', 'node-2'],
    });

    const res = await getFlowTaps('flow-1');

    expect(getMock).toHaveBeenCalledTimes(1);
    expect(getMock).toHaveBeenCalledWith('/flows/flow-1/taps');
    expect(res.node_ids).toEqual(['node-1', 'node-2']);
  });

  it('관찰 노드가 없으면 빈 배열을 반환한다', async () => {
    getMock.mockResolvedValueOnce({ flow_id: 'flow-1', node_ids: [] });

    const res = await getFlowTaps('flow-1');

    expect(res.node_ids).toEqual([]);
  });
});
