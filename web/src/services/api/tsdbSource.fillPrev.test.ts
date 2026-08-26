// 사용 기간 제한이 실제 요청 본문까지 실려 가는지.
//
// 설정 UI 와 백엔드가 모두 맞아도 이 구간이 빠지면 조용히 무제한이 된다.

import { describe, it, expect, vi, beforeEach } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: vi.fn(),
  del: vi.fn(),
  delWith: vi.fn(),
}));

import type { SeriesMatrixQuery } from './seriesDataSource';
import { queryTsdbSourceMatrix } from './tsdbSource';

const INFLUX_AGENT = { id: 'a-1', name: 'ix', type: 'influxdb' } as never;

const base: SeriesMatrixQuery = {
  keys: ['cpu'],
  seriesFilters: [{ fieldName: 'usage', tags: { host: 'a' } }],
  startMs: 0,
  endMs: 3_600_000,
  intervalMs: 60_000,
  aggregation: 'average',
};

async function run(params: SeriesMatrixQuery): Promise<Record<string, unknown>> {
  await queryTsdbSourceMatrix({ agent_id: 'a-1', agent_name: 'ix' }, [INFLUX_AGENT], params);
  return postMock.mock.calls.at(-1)![1] as Record<string, unknown>;
}

beforeEach(() => {
  postMock.mockReset();
  postMock.mockResolvedValue({ entries: [] });
});

describe('직전값 사용 기간 — 요청 전달', () => {
  it('제한이 없으면 관련 필드를 싣지 않는다', async () => {
    const body = await run({ ...base, fill: 'previous' });
    expect(body.fill).toBe('previous');
    expect(body).not.toHaveProperty('fill_previous_max_ms');
  });

  it('기간을 정하면 ms 로 싣는다', async () => {
    const body = await run({ ...base, fill: 'previous', fillPreviousMaxMs: 300_000 });
    expect(body.fill_previous_max_ms).toBe(300_000);
    // 비움이 기본이라 초과 처리는 싣지 않는다.
    expect(body).not.toHaveProperty('fill_previous_overflow');
  });

  it('지정 값 채우기는 값까지 함께 싣는다', async () => {
    const body = await run({
      ...base,
      fill: 'previous',
      fillPreviousMaxMs: 300_000,
      fillPreviousOverflow: 'value',
      fillPreviousOverflowValue: -1,
    });
    expect(body.fill_previous_overflow).toBe('value');
    expect(body.fill_previous_overflow_value).toBe(-1);
  });

  it('지정 값이 0 이어도 싣는다 — 비움과 다르다', async () => {
    const body = await run({
      ...base,
      fill: 'previous',
      fillPreviousMaxMs: 300_000,
      fillPreviousOverflow: 'value',
      fillPreviousOverflowValue: 0,
    });
    expect(body.fill_previous_overflow_value).toBe(0);
  });

  it('다른 채우기 전략에는 싣지 않는다 — 뜻이 없다', async () => {
    const body = await run({ ...base, fill: 'zero', fillPreviousMaxMs: 300_000 });
    expect(body).not.toHaveProperty('fill_previous_max_ms');
  });
});
