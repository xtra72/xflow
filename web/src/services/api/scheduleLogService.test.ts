// scheduleLogService.getScheduleLogs 단위 테스트(SPEC-SCHEDULE-VIEW-001 M6).
//
// camelCase 인자 → snake_case 쿼리 파라미터 매핑, 빈 필터 생략,
// envelope 해제 payload 전파, null payload → 빈 배열 정규화를 검증한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
  getList: vi.fn(),
}));

import { getScheduleLogs } from './scheduleLogService';

describe('getScheduleLogs', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('camelCase 인자를 snake_case 쿼리로 매핑하고 빈 필터는 생략한다', async () => {
    getMock.mockResolvedValueOnce([]);

    await getScheduleLogs({ scheduleId: 'sch-1', agentId: 'ag-1', limit: 50, offset: 10 });

    expect(getMock).toHaveBeenCalledTimes(1);
    expect(getMock).toHaveBeenCalledWith('/schedules/logs', {
      params: { schedule_id: 'sch-1', agent_id: 'ag-1', limit: 50, offset: 10 },
    });
  });

  it('인자 없이 호출하면 빈 params 로 조회한다', async () => {
    getMock.mockResolvedValueOnce([]);

    await getScheduleLogs();

    expect(getMock).toHaveBeenCalledWith('/schedules/logs', { params: {} });
  });

  it('응답 배열을 그대로 전파한다', async () => {
    getMock.mockResolvedValueOnce([{ id: 1, correlation_id: 'c:1' }]);

    const res = await getScheduleLogs({ ruleName: '규칙' });

    expect(getMock).toHaveBeenCalledWith('/schedules/logs', { params: { rule_name: '규칙' } });
    expect(res).toHaveLength(1);
    expect(res[0]?.id).toBe(1);
  });

  it('null payload 는 빈 배열로 정규화한다', async () => {
    getMock.mockResolvedValueOnce(null);

    const res = await getScheduleLogs();

    expect(res).toEqual([]);
  });
});
