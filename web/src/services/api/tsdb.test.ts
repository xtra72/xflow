// tsdb.ts 단위 테스트 — API 래퍼, 시간 변환, 인터벌 파싱, 버킷 추정, fan-out 쿼리.
// `./client` 의 get/post 를 vi.mock 으로 교체해 axios 호출 없이 검증한다.
//
// @spec SPEC-WEB-005

import { describe, expect, it, vi, beforeEach } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
}));

import {
  datetimeLocalToEpochMs,
  estimateBucketCount,
  formatLocalTimestamp,
  fromEpochMs,
  isValidInterval,
  listTsdbSeries,
  parseIntervalToMs,
  queryTsdbMatrix,
  toEpochMs,
} from './tsdb';

// ---- Time conversion utilities ----

describe('toEpochMs / fromEpochMs', () => {
  it('왕복 변환이 손실 없이 동작한다', () => {
    const original = new Date(2026, 3, 23, 12, 34, 56, 789); // 로컬
    const ms = toEpochMs(original);
    const restored = fromEpochMs(ms);
    expect(restored.getTime()).toBe(ms);
    expect(restored.getFullYear()).toBe(2026);
    expect(restored.getMonth()).toBe(3);
    expect(restored.getDate()).toBe(23);
    expect(restored.getHours()).toBe(12);
    expect(restored.getMinutes()).toBe(34);
    expect(restored.getSeconds()).toBe(56);
  });
});

describe('datetimeLocalToEpochMs', () => {
  it('빈 문자열은 NaN 반환', () => {
    expect(Number.isNaN(datetimeLocalToEpochMs(''))).toBe(true);
  });

  it('ISO 로컬 문자열을 epoch ms 로 변환', () => {
    const ms = datetimeLocalToEpochMs('2026-04-23T10:00');
    expect(Number.isFinite(ms)).toBe(true);
    const d = new Date(ms);
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(3);
    expect(d.getDate()).toBe(23);
    expect(d.getHours()).toBe(10);
    expect(d.getMinutes()).toBe(0);
  });
});

describe('formatLocalTimestamp', () => {
  it('YYYY-MM-DD HH:mm:ss 포맷으로 출력', () => {
    const ms = new Date(2026, 3, 23, 5, 7, 9).getTime();
    expect(formatLocalTimestamp(ms)).toBe('2026-04-23 05:07:09');
  });
});

// ---- Interval parsing ----

describe('parseIntervalToMs', () => {
  it('ms/s/m/h 단위를 파싱한다', () => {
    expect(parseIntervalToMs('100ms')).toBe(100);
    expect(parseIntervalToMs('30s')).toBe(30_000);
    expect(parseIntervalToMs('1m')).toBe(60_000);
    expect(parseIntervalToMs('2h')).toBe(2 * 60 * 60 * 1000);
  });

  it('잘못된 입력은 NaN 반환', () => {
    expect(Number.isNaN(parseIntervalToMs('5분'))).toBe(true);
    expect(Number.isNaN(parseIntervalToMs('abc'))).toBe(true);
    expect(Number.isNaN(parseIntervalToMs(''))).toBe(true);
    expect(Number.isNaN(parseIntervalToMs('0s'))).toBe(true);
    expect(Number.isNaN(parseIntervalToMs('1d'))).toBe(true); // d 미지원
  });
});

describe('isValidInterval', () => {
  it('프리셋과 custom 유효성을 판단', () => {
    expect(isValidInterval('10s')).toBe(true);
    expect(isValidInterval('1m')).toBe(true);
    expect(isValidInterval('xyz')).toBe(false);
    expect(isValidInterval('')).toBe(false);
  });
});

// ---- Bucket estimation ----

describe('estimateBucketCount', () => {
  it('30일 × 1m 인터벌에서 43,200 을 반환한다 (5,000 초과 케이스)', () => {
    const start = new Date(2026, 2, 24, 0, 0, 0).getTime();
    const end = new Date(2026, 3, 23, 0, 0, 0).getTime();
    expect(estimateBucketCount(start, end, '1m')).toBe(43_200);
  });

  it('end <= start 인 경우 0 반환', () => {
    const start = Date.now();
    expect(estimateBucketCount(start, start, '1m')).toBe(0);
    expect(estimateBucketCount(start, start - 1000, '1m')).toBe(0);
  });

  it('잘못된 인터벌이면 0 반환', () => {
    const start = Date.now();
    const end = start + 60_000;
    expect(estimateBucketCount(start, end, 'bogus')).toBe(0);
  });

  it('6시간 × 1h 인터벌은 6 버킷', () => {
    const start = new Date(2026, 3, 23, 0, 0, 0).getTime();
    const end = new Date(2026, 3, 23, 6, 0, 0).getTime();
    expect(estimateBucketCount(start, end, '1h')).toBe(6);
  });
});

// ---- listTsdbSeries ----

describe('listTsdbSeries', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('page/size 파라미터를 쿼리스트링으로 전달한다', async () => {
    getMock.mockResolvedValueOnce({
      series: ['a', 'b'],
      count: 2,
      pagination: { page: 1, size: 25, total: 2, total_pages: 1 },
    });

    const res = await listTsdbSeries({ page: 1, size: 25 });

    expect(getMock).toHaveBeenCalledWith('/tsdb/series', {
      params: { page: 1, size: 25 },
    });
    expect(res.series).toEqual(['a', 'b']);
    expect(res.pagination?.total).toBe(2);
  });

  it('agentId 가 있으면 agent_id 파라미터를 포함한다', async () => {
    getMock.mockResolvedValueOnce({ series: [], count: 0 });
    await listTsdbSeries({ page: 2, size: 50, agentId: 'tsdb-1' });
    expect(getMock).toHaveBeenCalledWith('/tsdb/series', {
      params: { page: 2, size: 50, agent_id: 'tsdb-1' },
    });
  });
});

// ---- queryTsdbMatrix ----

describe('queryTsdbMatrix', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('키가 없으면 백엔드 호출 없이 빈 결과 반환', async () => {
    const res = await queryTsdbMatrix({
      keys: [],
      startMs: 0,
      endMs: 1,
      interval: '1m',
      aggregation: 'average',
    });
    expect(postMock).not.toHaveBeenCalled();
    expect(res.results).toEqual([]);
  });

  it('end <= start 이면 에러를 던진다', async () => {
    await expect(
      queryTsdbMatrix({
        keys: ['a'],
        startMs: 100,
        endMs: 100,
        interval: '1m',
        aggregation: 'average',
      }),
    ).rejects.toThrow(/종료 시각/);
  });

  it('잘못된 인터벌이면 에러를 던진다', async () => {
    await expect(
      queryTsdbMatrix({
        keys: ['a'],
        startMs: 0,
        endMs: 60_000,
        interval: 'bogus',
        aggregation: 'average',
      }),
    ).rejects.toThrow(/인터벌/);
  });

  it('선택된 키마다 병렬 요청을 보낸다 (fan-out)', async () => {
    postMock.mockResolvedValue({
      results: [
        {
          series_key: 'ignore',
          points: [
            { timestamp: '2026-04-23T00:00:00Z', fields: { value: 10 } },
            { timestamp: '2026-04-23T01:00:00Z', fields: { value: 20 } },
          ],
        },
      ],
    });

    const startMs = Date.UTC(2026, 3, 23, 0, 0, 0);
    const endMs = Date.UTC(2026, 3, 23, 6, 0, 0);

    const res = await queryTsdbMatrix({
      keys: ['temp,room=1', 'temp,room=2'],
      startMs,
      endMs,
      interval: '1h',
      aggregation: 'average',
    });

    expect(postMock).toHaveBeenCalledTimes(2);
    // 첫 번째 호출 파라미터 검증
    const firstCall = postMock.mock.calls[0]!;
    const firstBody = firstCall[1] as {
      series_key: string;
      start: string;
      end: string;
      bucket: string;
      aggregation: string;
    };
    expect(firstBody).toMatchObject({
      series_key: 'temp,room=1',
      bucket: '1h',
      aggregation: 'avg', // average -> avg 매핑
    });
    // RFC3339 변환 확인 (ISO 문자열, Z 접미)
    expect(firstBody.start).toBe(new Date(startMs).toISOString());
    expect(firstBody.end).toBe(new Date(endMs).toISOString());

    // 결과 순서 유지, 숫자 필드 추출
    expect(res.results).toHaveLength(2);
    expect(res.results[0]!.key).toBe('temp,room=1');
    expect(res.results[0]!.points).toEqual([
      { timestampMs: Date.parse('2026-04-23T00:00:00Z'), value: 10 },
      { timestampMs: Date.parse('2026-04-23T01:00:00Z'), value: 20 },
    ]);
    expect(res.results[1]!.key).toBe('temp,room=2');
  });

  it('백엔드에서 results 가 비어있으면 빈 points 반환', async () => {
    postMock.mockResolvedValue({ results: [] });
    const res = await queryTsdbMatrix({
      keys: ['no-data'],
      startMs: 0,
      endMs: 1000,
      interval: '1s',
      aggregation: 'min',
    });
    expect(res.results).toEqual([{ key: 'no-data', points: [] }]);
  });

  it('숫자가 아닌 필드는 null 로 변환', async () => {
    postMock.mockResolvedValueOnce({
      results: [
        {
          series_key: 'x',
          points: [
            { timestamp: '2026-04-23T00:00:00Z', fields: { note: 'nope' } },
          ],
        },
      ],
    });
    const res = await queryTsdbMatrix({
      keys: ['x'],
      startMs: 0,
      endMs: 60_000,
      interval: '1m',
      aggregation: 'max',
    });
    expect(res.results[0]!.points[0]!.value).toBeNull();
  });

  it('min/max 집계는 값 그대로 전송 (매핑 없음)', async () => {
    postMock.mockResolvedValueOnce({ results: [] });
    await queryTsdbMatrix({
      keys: ['x'],
      startMs: 0,
      endMs: 60_000,
      interval: '1m',
      aggregation: 'min',
    });
    const body = postMock.mock.calls[0]![1] as { aggregation: string };
    expect(body.aggregation).toBe('min');
  });
});
