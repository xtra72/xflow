// scheduleLogConfigService 단위 테스트.
//
// getScheduleLogStorageType: GET /system/schedule-log-config → data.storage_type 언래핑,
//   null/빈 값 → 기본값 "sqlite".
// setScheduleLogStorageType: PUT /system/schedule-log-config { storage_type } →
//   data.needs_restart 언래핑({ needsRestart }), 누락 → false.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  put: putMock,
}));

import {
  getScheduleLogStorageType,
  isScheduleLogStorageType,
  setScheduleLogStorageType,
} from './scheduleLogConfigService';

describe('getScheduleLogStorageType', () => {
  beforeEach(() => {
    getMock.mockReset();
    putMock.mockReset();
  });

  it('GET 후 storage_type 을 언래핑해 반환한다', async () => {
    getMock.mockResolvedValueOnce({ storage_type: 'file' });

    const value = await getScheduleLogStorageType();

    expect(getMock).toHaveBeenCalledWith('/system/schedule-log-config');
    expect(value).toBe('file');
  });

  it('빈 storage_type 은 기본값 "sqlite" 로 정규화한다', async () => {
    getMock.mockResolvedValueOnce({ storage_type: '' });

    const value = await getScheduleLogStorageType();

    expect(value).toBe('sqlite');
  });

  it('null payload 는 기본값 "sqlite" 로 정규화한다', async () => {
    getMock.mockResolvedValueOnce(null);

    const value = await getScheduleLogStorageType();

    expect(value).toBe('sqlite');
  });

  it('storage_type 누락 시 기본값 "sqlite" 로 정규화한다', async () => {
    getMock.mockResolvedValueOnce({});

    const value = await getScheduleLogStorageType();

    expect(value).toBe('sqlite');
  });
});

describe('setScheduleLogStorageType', () => {
  beforeEach(() => {
    getMock.mockReset();
    putMock.mockReset();
  });

  it('선택 값을 storage_type 바디로 PUT 하고 needsRestart 를 언래핑한다', async () => {
    putMock.mockResolvedValueOnce({ applied: 'storage.schedule_log.type', needs_restart: true });

    const res = await setScheduleLogStorageType('memory');

    expect(putMock).toHaveBeenCalledWith('/system/schedule-log-config', { storage_type: 'memory' });
    expect(res).toEqual({ needsRestart: true });
  });

  it('needs_restart 누락 시 false 로 정규화한다', async () => {
    putMock.mockResolvedValueOnce({ applied: 'storage.schedule_log.type' });

    const res = await setScheduleLogStorageType('sqlite');

    expect(res).toEqual({ needsRestart: false });
  });

  it('null payload 는 needsRestart false 로 정규화한다', async () => {
    putMock.mockResolvedValueOnce(null);

    const res = await setScheduleLogStorageType('file');

    expect(res).toEqual({ needsRestart: false });
  });
});

describe('isScheduleLogStorageType', () => {
  it('알려진 값은 true, 그 외는 false 를 반환한다', () => {
    expect(isScheduleLogStorageType('sqlite')).toBe(true);
    expect(isScheduleLogStorageType('file')).toBe(true);
    expect(isScheduleLogStorageType('memory')).toBe(true);
    expect(isScheduleLogStorageType('postgres')).toBe(false);
    expect(isScheduleLogStorageType('')).toBe(false);
  });
});
