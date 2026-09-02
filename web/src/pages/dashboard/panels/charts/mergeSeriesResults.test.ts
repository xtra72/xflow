// 여러 소스의 결과를 한 패널로 합치는 규칙.
//
// 합치는 순간 생기는 물음들(이름 충돌 · 상태 · 순서)의 답을 여기서 잠근다. 답이 갈리면
// 같은 패널이 소스 조합에 따라 다르게 보인다.

import { describe, it, expect } from 'vitest';

import {
  mergeSeriesResults,
  mergeStatus,
  resolveMergedNames,
  type MergeableResult,
} from './mergeSeriesResults';

function result(over: Partial<MergeableResult> = {}): MergeableResult {
  return {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'connected',
    ...over,
  };
}

function withSeries(names: string[], at = 1000): MergeableResult {
  return result({
    seriesNames: names,
    seriesEntries: new Map(names.map((n) => [n, [{ timestamp: at, value: 1 }]])),
    seriesStyles: new Map(names.map((n) => [n, { color: '#111' }])),
    entries: names.map(() => ({ timestamp: at, value: 1 })),
  });
}

describe('mergeStatus', () => {
  it('가장 심각한 것이 이긴다', () => {
    expect(mergeStatus(['connected', 'error'])).toBe('error');
    expect(mergeStatus(['connected', 'connecting'])).toBe('connecting');
    expect(mergeStatus(['closed', 'connecting'])).toBe('closed');
  });

  it('idle 은 가장 낮다 — 고르지 않은 소스가 조회 중인 소스를 가리면 안 된다', () => {
    expect(mergeStatus(['idle', 'connected'])).toBe('connected');
    expect(mergeStatus(['idle', 'error'])).toBe('error');
  });

  it('전부 idle 이면 idle 이다', () => {
    expect(mergeStatus(['idle', 'idle'])).toBe('idle');
    expect(mergeStatus([])).toBe('idle');
  });
});

describe('resolveMergedNames', () => {
  it('겹치지 않는 이름은 그대로 둔다 — 저장된 범례 글자가 변하지 않는다', () => {
    const [a, b] = resolveMergedNames([
      { label: 'Store', names: ['온도'] },
      { label: '시스템 지표', names: ['CPU'] },
    ]);
    expect(a!.get('온도')).toBe('온도');
    expect(b!.get('CPU')).toBe('CPU');
  });

  it('겹치는 이름에만 소스를 덧붙인다', () => {
    const [a, b] = resolveMergedNames([
      { label: 'Store', names: ['온도', '습도'] },
      { label: 'TSDB', names: ['온도'] },
    ]);
    expect(a!.get('온도')).toBe('온도 (Store)');
    expect(b!.get('온도')).toBe('온도 (TSDB)');
    // 겹치지 않은 이름은 같은 소스 안에서도 그대로다.
    expect(a!.get('습도')).toBe('습도');
  });
});

describe('mergeSeriesResults', () => {
  it('소스가 하나면 입력을 그대로 돌려준다 — 참조까지 같다', () => {
    const only = withSeries(['a']);
    expect(mergeSeriesResults([{ label: 'Store', result: only }])).toBe(only);
  });

  it('소스가 없으면 빈 결과다', () => {
    const m = mergeSeriesResults([]);
    expect(m.seriesNames).toEqual([]);
    expect(m.status).toBe('idle');
  });

  it('시리즈를 소스 순서대로 잇는다', () => {
    const m = mergeSeriesResults([
      { label: 'Store', result: withSeries(['a', 'b']) },
      { label: 'TSDB', result: withSeries(['c']) },
    ]);
    expect(m.seriesNames).toEqual(['a', 'b', 'c']);
    expect(m.seriesEntries.size).toBe(3);
    expect(m.seriesStyles.size).toBe(3);
  });

  it('겹치는 이름은 꼬리표로 갈라 둘 다 남긴다 — 하나가 사라지면 안 된다', () => {
    const m = mergeSeriesResults([
      { label: 'Store', result: withSeries(['온도']) },
      { label: '시스템 지표', result: withSeries(['온도']) },
    ]);
    expect(m.seriesNames).toEqual(['온도 (Store)', '온도 (시스템 지표)']);
    expect(m.seriesEntries.has('온도 (Store)')).toBe(true);
    expect(m.seriesEntries.has('온도 (시스템 지표)')).toBe(true);
  });

  it('boolean 시리즈 표시도 새 이름으로 따라간다', () => {
    const a = withSeries(['flag']);
    a.booleanSeries = new Set(['flag']);
    const b = withSeries(['flag']);
    const m = mergeSeriesResults([
      { label: 'Store', result: a },
      { label: 'TSDB', result: b },
    ]);
    expect(m.booleanSeries.has('flag (Store)')).toBe(true);
    expect(m.booleanSeries.has('flag (TSDB)')).toBe(false);
  });

  it('평탄 타임라인은 시간순으로 이어 붙인다 — 소스 경계에서 되감기면 안 된다', () => {
    const m = mergeSeriesResults([
      { label: 'A', result: result({ entries: [{ timestamp: 30, value: 1 }] }) },
      { label: 'B', result: result({ entries: [{ timestamp: 10, value: 2 }] }) },
    ]);
    expect(m.entries.map((e) => e.timestamp)).toEqual([10, 30]);
  });

  it('부분 실패 개수는 더한다', () => {
    const m = mergeSeriesResults([
      { label: 'A', result: result({ partialFailureCount: 2 }) },
      { label: 'B', result: result({ partialFailureCount: 3 }) },
    ]);
    expect(m.partialFailureCount).toBe(5);
  });

  it('실패 사유는 처음 만난 것을 남긴다 — 순서에 따라 달라지지 않게', () => {
    const m = mergeSeriesResults([
      { label: 'A', result: result({ status: 'error', errorReason: '먼저' }) },
      { label: 'B', result: result({ status: 'error', errorReason: '나중' }) },
    ]);
    expect(m.status).toBe('error');
    expect(m.errorReason).toBe('먼저');
  });

  it('한 소스만 실패해도 패널은 실패를 알린다', () => {
    const m = mergeSeriesResults([
      { label: 'A', result: withSeries(['a']) },
      { label: 'B', result: result({ status: 'error', errorReason: '조회 실패' }) },
    ]);
    expect(m.status).toBe('error');
    // 성공한 줄은 그대로 남는다 — 하나가 실패했다고 나머지를 버리지 않는다.
    expect(m.seriesNames).toEqual(['a']);
  });

  it('쓰지 않는 키는 만들지 않는다 — 두 경로의 반환 키 집합을 어긋내지 않는다', () => {
    const m = mergeSeriesResults([
      { label: 'A', result: withSeries(['a']) },
      { label: 'B', result: withSeries(['b']) },
    ]);
    expect('partialFailureCount' in m).toBe(false);
    expect('backendMismatch' in m).toBe(false);
    expect('errorReason' in m).toBe(false);
  });
});
