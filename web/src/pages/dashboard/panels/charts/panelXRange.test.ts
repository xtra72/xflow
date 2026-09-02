// X축 범위는 **패널이 소유한다** — 소스와 무관하다.
//
// 소스가 여럿이 되면서 "어느 소스의 구간인가" 가 답이 없는 물음이 됐다. Store A 와 B 가
// 서로 다른 구간을 보면 한 X축에 그릴 수 없다. 그래서 구간은 패널 옵션 하나이고, 소스는
// 그것을 받아 조회한다.

import { describe, it, expect } from 'vitest';

import {
  panelXRangePatch,
  readPanelXRange,
  readPanelXRangeOverride,
  withPanelRange,
} from './panelXRange';

const RANGE = { mode: 'relative' as const, window_ms: 60_000 };

describe('readPanelXRange', () => {
  it('패널의 x_range 를 읽는다', () => {
    expect(readPanelXRange({ x_range: RANGE })).toEqual(RANGE);
  });

  it('구 어휘도 해석한다 — 저장된 패널이 그대로 동작해야 한다', () => {
    expect(readPanelXRange({ time_window_mode: 'recent', recent_window_sec: 30 })).toEqual({
      mode: 'relative',
      window_ms: 30_000,
    });
    expect(readPanelXRange({ max_points: 250 })).toEqual({ mode: 'count', count: 250 });
  });

  it('소스 종류·소스 블록을 보지 않는다 — 구간은 소스와 무관하다', () => {
    const withSources = {
      data_source: 'store',
      store_source: { range: { mode: 'count' as const, count: 7 }, time_window_ms: 999 },
      x_range: RANGE,
    };
    expect(readPanelXRange(withSources)).toEqual(RANGE);
  });
});

describe('panelXRangePatch', () => {
  it('언제나 x_range 하나만 고친다', () => {
    expect(panelXRangePatch({}, RANGE)).toEqual({ x_range: RANGE });
    expect(panelXRangePatch({ data_source: 'store' }, RANGE)).toEqual({ x_range: RANGE });
  });

  it('소스 블록을 건드리지 않는다 — 소스가 여럿이면 어느 블록인지 답이 없다', () => {
    const patch = panelXRangePatch(
      { data_source: 'store', store_source: { agent_name: 'a' } },
      RANGE,
    );
    expect(patch).not.toHaveProperty('store_source');
    expect(patch).not.toHaveProperty('sources');
  });
});

describe('readPanelXRangeOverride — 조회에 얹을 구간', () => {
  it('패널이 구간을 선언했으면 그 값이다', () => {
    expect(readPanelXRangeOverride({ x_range: RANGE })).toEqual(RANGE);
    expect(readPanelXRangeOverride({ time_window_mode: 'recent', recent_window_sec: 30 })).toEqual({
      mode: 'relative',
      window_ms: 30_000,
    });
  });

  it('선언하지 않았으면 undefined 다 — 소스의 조회 창을 건드리지 않는다', () => {
    expect(readPanelXRangeOverride({})).toBeUndefined();
  });

  // max_points 는 라인 차트에서 "X축 포인트 수" 지만 바·파이·통계에서는 **막대(조각) 수**다.
  // 표시용 폴백으로 읽는 것은 무해하지만 조회 구간으로 밀어 넣으면, 막대 12개짜리 패널이
  // 데이터를 12점만 긁어 온다.
  it('max_points 는 조회 구간으로 쓰지 않는다 — 뜻이 패널마다 다르다', () => {
    expect(readPanelXRangeOverride({ max_points: 12 })).toBeUndefined();
    // 표시용 읽기에서는 여전히 폴백으로 해석된다.
    expect(readPanelXRange({ max_points: 12 })).toEqual({ mode: 'count', count: 12 });
  });
});

describe('withPanelRange', () => {
  it('소스 블록의 구간을 패널 구간으로 덮는다', () => {
    const source = { agent_name: 'a', range: { mode: 'count' as const, count: 5 } };
    expect(withPanelRange(source, RANGE).range).toEqual(RANGE);
  });

  it('패널이 선언하지 않았으면 소스 값을 그대로 둔다', () => {
    const source = { agent_name: 'a', range: { mode: 'count' as const, count: 5 } };
    expect(withPanelRange(source, undefined).range).toEqual({ mode: 'count', count: 5 });
  });

  it('비활성 슬롯(undefined)은 그대로 통과시킨다', () => {
    expect(withPanelRange(undefined, RANGE)).toBeUndefined();
  });

  it('소스의 다른 설정을 지우지 않는다', () => {
    const source = { agent_name: 'a', interval_ms: 5000 } as {
      agent_name: string;
      interval_ms: number;
      range?: typeof RANGE;
    };
    const out = withPanelRange(source, RANGE);
    expect(out.agent_name).toBe('a');
    expect(out.interval_ms).toBe(5000);
  });
});
