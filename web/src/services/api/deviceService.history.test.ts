// deviceService.getDeviceHistory 단위 테스트 — URL/params 형상 + 404 전파.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  getList: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
}));

import { APIError } from '@/types/api';

import { getDeviceHistory } from './deviceService';

describe('getDeviceHistory', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('limit 지정 시 ?limit 쿼리로 GET 한다', async () => {
    getMock.mockResolvedValueOnce({ device_id: 'd1', count: 0, entries: [] });

    await getDeviceHistory('d1', 50);

    expect(getMock).toHaveBeenCalledWith('/devices/d1/history', { params: { limit: 50 } });
  });

  it('limit 생략/0 이하면 params 없이 GET 한다(서버 기본값)', async () => {
    getMock.mockResolvedValueOnce({ device_id: 'd1', count: 0, entries: [] });

    await getDeviceHistory('d1');
    expect(getMock).toHaveBeenCalledWith('/devices/d1/history', { params: undefined });

    getMock.mockResolvedValueOnce({ device_id: 'd1', count: 0, entries: [] });
    await getDeviceHistory('d1', 0);
    expect(getMock).toHaveBeenLastCalledWith('/devices/d1/history', { params: undefined });
  });

  it('404(이력 비활성) 는 호출자로 전파된다', async () => {
    getMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not enabled', 404));
    await expect(getDeviceHistory('d1', 100)).rejects.toBeInstanceOf(APIError);
  });
});
