// scheduleLogService 단위 테스트(SPEC-SCHEDULE-VIEW-001 M6).
//
// getScheduleLogs: camelCase 인자 → snake_case 쿼리 파라미터 매핑, 빈 필터 생략,
// envelope 해제 payload({ items, total }) 정규화, null/누락 → { items: [], total: 0 }.
// clearScheduleLogs: DELETE /schedules/logs 호출.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const delMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: vi.fn(),
  put: vi.fn(),
  del: delMock,
  getList: vi.fn(),
}));

import { clearScheduleLogs, getScheduleLogs } from './scheduleLogService';

describe('getScheduleLogs', () => {
  beforeEach(() => {
    getMock.mockReset();
    delMock.mockReset();
  });

  it('camelCase 인자를 snake_case 쿼리로 매핑하고 빈 필터는 생략한다', async () => {
    getMock.mockResolvedValueOnce({ items: [], total: 0 });

    await getScheduleLogs({ scheduleId: 'sch-1', agentId: 'ag-1', limit: 50, offset: 10 });

    expect(getMock).toHaveBeenCalledTimes(1);
    expect(getMock).toHaveBeenCalledWith('/schedules/logs', {
      params: { schedule_id: 'sch-1', agent_id: 'ag-1', limit: 50, offset: 10 },
    });
  });

  it('인자 없이 호출하면 빈 params 로 조회한다', async () => {
    getMock.mockResolvedValueOnce({ items: [], total: 0 });

    await getScheduleLogs();

    expect(getMock).toHaveBeenCalledWith('/schedules/logs', { params: {} });
  });

  it('응답 { items, total } 을 정규화해 전파한다', async () => {
    getMock.mockResolvedValueOnce({ items: [{ id: 1, correlation_id: 'c:1' }], total: 7 });

    const res = await getScheduleLogs({ ruleName: '규칙' });

    expect(getMock).toHaveBeenCalledWith('/schedules/logs', { params: { rule_name: '규칙' } });
    expect(res.items).toHaveLength(1);
    expect(res.items[0]?.id).toBe(1);
    expect(res.total).toBe(7);
  });

  it('null payload 는 { items: [], total: 0 } 으로 정규화한다', async () => {
    getMock.mockResolvedValueOnce(null);

    const res = await getScheduleLogs();

    expect(res).toEqual({ items: [], total: 0 });
  });

  it('items/total 누락 시 각각 [] / 0 으로 정규화한다', async () => {
    getMock.mockResolvedValueOnce({});

    const res = await getScheduleLogs();

    expect(res).toEqual({ items: [], total: 0 });
  });
});

describe('clearScheduleLogs', () => {
  beforeEach(() => {
    getMock.mockReset();
    delMock.mockReset();
  });

  it('DELETE /schedules/logs 를 호출한다', async () => {
    delMock.mockResolvedValueOnce(undefined);

    await clearScheduleLogs();

    expect(delMock).toHaveBeenCalledTimes(1);
    expect(delMock).toHaveBeenCalledWith('/schedules/logs');
  });
});
