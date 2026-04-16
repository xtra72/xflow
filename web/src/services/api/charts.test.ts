// charts.ts 단위 테스트.
// `./client` 의 `get` 함수를 vi.mock 으로 교체해 axios 호출 없이 검증한다.

import { describe, expect, it, vi, beforeEach } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
}));

import { listChartChannels, type ChartChannelSummary } from './charts';

describe('listChartChannels', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('GET /charts/channels 를 호출하고 channels 배열을 반환한다', async () => {
    const sample: ChartChannelSummary[] = [
      {
        name: 'demo_temp',
        flow_id: 'flow-1',
        node_id: 'node-a',
        buffer_size: 100,
        retention_sec: 3600,
        subscriber_count: 2,
        last_message_ms: 1713312000000,
      },
    ];
    getMock.mockResolvedValueOnce({ channels: sample });

    const result = await listChartChannels();

    expect(getMock).toHaveBeenCalledWith('/charts/channels');
    expect(result).toEqual(sample);
  });

  it('빈 응답이면 빈 배열을 반환한다', async () => {
    getMock.mockResolvedValueOnce({ channels: [] });
    const result = await listChartChannels();
    expect(result).toEqual([]);
  });

  it('channels 필드가 없으면 빈 배열을 반환한다 (방어적)', async () => {
    getMock.mockResolvedValueOnce({});
    const result = await listChartChannels();
    expect(result).toEqual([]);
  });

  it('에러가 발생하면 rejected promise 를 반환한다', async () => {
    const err = new Error('network failure');
    getMock.mockRejectedValueOnce(err);
    await expect(listChartChannels()).rejects.toThrow('network failure');
  });
});
