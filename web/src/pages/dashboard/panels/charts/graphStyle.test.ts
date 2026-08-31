// 그래프 스타일 판정.

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_GRAPH_STYLE,
  effectiveStacked,
  hasGapDash,
  hasStrokeStyle,
  isStackable,
  readGraphStyle,
  requiresBuckets,
  resolveSeriesStyle,
} from './graphStyle';

describe('readGraphStyle', () => {
  it('아는 값은 그대로', () => {
    for (const s of ['line', 'area', 'bar', 'candle'] as const) {
      expect(readGraphStyle(s)).toBe(s);
    }
  });

  it('모르는 값·빈 값은 기본값(라인)으로', () => {
    expect(readGraphStyle('pie')).toBe(DEFAULT_GRAPH_STYLE);
    expect(readGraphStyle(undefined)).toBe('line');
    expect(readGraphStyle(null)).toBe('line');
  });
});

describe('resolveSeriesStyle — 시리즈가 패널을 덮어쓴다', () => {
  it('시리즈 지정이 없으면 패널을 따른다', () => {
    expect(resolveSeriesStyle(undefined, 'bar')).toBe('bar');
    expect(resolveSeriesStyle('', 'area')).toBe('area');
  });

  it('시리즈 지정이 있으면 그것이 이긴다', () => {
    expect(resolveSeriesStyle('line', 'bar')).toBe('line');
  });

  it('"패널을 따름"과 "라인 고정"은 다르다', () => {
    // 패널이 바뀌면 전자만 따라 바뀐다.
    expect(resolveSeriesStyle(undefined, 'bar')).toBe('bar');
    expect(resolveSeriesStyle('line', 'bar')).toBe('line');
  });

  it('시리즈 값이 이상하면 패널 값으로 떨어진다', () => {
    expect(resolveSeriesStyle('nope', 'area')).toBe('area');
  });
});

describe('스타일별 제약', () => {
  it('스택킹은 영역·바에서만 뜻이 있다', () => {
    expect(isStackable('area')).toBe(true);
    expect(isStackable('bar')).toBe(true);
    // 라인은 쌓아도 누적으로 읽히지 않는다.
    expect(isStackable('line')).toBe(false);
    // 캔들은 네 값이 한 덩어리라 쌓을 수 없다.
    expect(isStackable('candle')).toBe(false);
  });

  it('선 모양·곡선은 라인·영역에서만', () => {
    expect(hasStrokeStyle('line')).toBe(true);
    expect(hasStrokeStyle('area')).toBe(true);
    expect(hasStrokeStyle('bar')).toBe(false);
    expect(hasStrokeStyle('candle')).toBe(false);
  });

  it('결측 점선은 라인·영역에서만 — 바에 그으면 없는 막대를 잇는 선이 된다', () => {
    expect(hasGapDash('line')).toBe(true);
    expect(hasGapDash('area')).toBe(true);
    expect(hasGapDash('bar')).toBe(false);
    expect(hasGapDash('candle')).toBe(false);
  });

  it('캔들만 버킷 집계를 요구한다', () => {
    expect(requiresBuckets('candle')).toBe(true);
    for (const s of ['line', 'area', 'bar'] as const) {
      expect(requiresBuckets(s)).toBe(false);
    }
  });
});

describe('effectiveStacked — 켜져 있어도 스타일이 받쳐야 적용된다', () => {
  it('영역·바 + 켬 = 적용', () => {
    expect(effectiveStacked('area', true)).toBe(true);
    expect(effectiveStacked('bar', true)).toBe(true);
  });

  it('라인·캔들은 켜도 무시한다', () => {
    expect(effectiveStacked('line', true)).toBe(false);
    expect(effectiveStacked('candle', true)).toBe(false);
  });

  it('끄면 어느 스타일이든 적용하지 않는다', () => {
    expect(effectiveStacked('bar', false)).toBe(false);
    expect(effectiveStacked('bar', undefined)).toBe(false);
  });
});
