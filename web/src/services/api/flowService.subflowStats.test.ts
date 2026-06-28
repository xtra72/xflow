// flowService getSubflowStats API 단위 테스트 (Fix 2).
//
// getSubflowStats 는 서브플로우의 LIVE per-original-node 통계
// (GET /flows/{id}/subflow-stats)를 조회한다. 요청 URL 형상과 응답 전파,
// 참조 부모가 없을 때의 빈 nodes 처리를 검증한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
  getList: vi.fn(),
}));

import { APIError } from '@/types/api';

import { getSubflowStats } from './flowService';

describe('getSubflowStats', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('올바른 URL 로 GET 하고 응답을 그대로 전파한다', async () => {
    getMock.mockResolvedValueOnce({
      flow_id: 'sub-1',
      nodes: [
        {
          node_id: 'inner',
          processed: 12,
          errors: 0,
          state: 'running',
          ports: [
            { name: 'out', direction: 'output', messages: 12, delivered: 11 },
          ],
        },
      ],
    });

    const res = await getSubflowStats('sub-1');

    expect(getMock).toHaveBeenCalledTimes(1);
    expect(getMock).toHaveBeenCalledWith('/flows/sub-1/subflow-stats');
    expect(res.flow_id).toBe('sub-1');
    expect(res.nodes).toHaveLength(1);
    expect(res.nodes[0]?.node_id).toBe('inner');
  });

  it('참조 부모가 없으면 nodes 빈 배열을 반환한다', async () => {
    getMock.mockResolvedValueOnce({ flow_id: 'sub-1', nodes: [] });

    const res = await getSubflowStats('sub-1');

    expect(res.nodes).toEqual([]);
  });

  it('서버 오류는 호출자로 전파된다', async () => {
    getMock.mockRejectedValueOnce(new APIError('INTERNAL', 'boom', 500));
    await expect(getSubflowStats('sub-1')).rejects.toBeInstanceOf(APIError);
  });
});
