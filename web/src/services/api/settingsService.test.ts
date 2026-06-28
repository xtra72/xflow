// settingsService 단위 테스트 — getSetting/putSetting URL·바디·value 언래핑.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  put: putMock,
}));

import { APIError } from '@/types/api';

import { getSetting, putSetting } from './settingsService';

describe('settingsService', () => {
  beforeEach(() => {
    getMock.mockReset();
    putMock.mockReset();
  });

  it('getSetting 은 /settings/{key} 를 GET 하고 value 를 언래핑한다', async () => {
    getMock.mockResolvedValueOnce({ key: 'device-list-columns', value: { columns: ['name'] } });

    const value = await getSetting<{ columns: string[] }>('device-list-columns');

    expect(getMock).toHaveBeenCalledWith('/settings/device-list-columns');
    expect(value).toEqual({ columns: ['name'] });
  });

  it('getSetting 은 미저장(404)을 전파한다', async () => {
    getMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not found', 404));
    await expect(getSetting('missing')).rejects.toBeInstanceOf(APIError);
  });

  it('putSetting 은 value 를 바디로 PUT 하고 저장된 value 를 반환한다', async () => {
    putMock.mockResolvedValueOnce({ key: 'k', value: { columns: ['name', 'type'] } });

    const value = await putSetting('k', { columns: ['name', 'type'] });

    expect(putMock).toHaveBeenCalledWith('/settings/k', { columns: ['name', 'type'] });
    expect(value).toEqual({ columns: ['name', 'type'] });
  });
});
