// @spec SPEC-TSDB-002 §2.14

import { describe, expect, it } from 'vitest';

// ===== 그룹 페이지 판정 (SPEC-TSDB-004 §2.7) =====

import { resolveGroupPageDisplay } from './panelSeriesStatus';

describe('resolveGroupPageDisplay', () => {
  it('group by 항목이 없으면 표시하지 않는다', () => {
    expect(resolveGroupPageDisplay(undefined).show).toBe(false);
    expect(resolveGroupPageDisplay([]).show).toBe(false);
  });

  it('페이지가 하나뿐이고 절단도 없으면 표시하지 않는다', () => {
    // 그림 위에 겹치는 표기는 조작할 것이나 알릴 것이 있을 때만 값을 한다.
    // "그룹 3개 · 1/1 페이지" 는 넘길 페이지도 경고도 없는 순수 소음이다.
    expect(
      resolveGroupPageDisplay([{ total: 3, page: 0, pageCount: 1, truncated: false }]).show,
    ).toBe(false);
  });

  it('페이지가 하나여도 절단되면 표시한다 — 알릴 것이 있다', () => {
    expect(
      resolveGroupPageDisplay([{ total: 99, page: 0, pageCount: 1, truncated: true }]).show,
    ).toBe(true);
  });

  it('단일 항목의 총계와 페이지를 그대로 쓴다', () => {
    const d = resolveGroupPageDisplay([
      { total: 5, page: 1, pageCount: 3, truncated: false },
    ]);
    expect(d).toMatchObject({ show: true, total: 5, page: 1, pageCount: 3 });
    expect(d.canPrev).toBe(true);
    expect(d.canNext).toBe(true);
  });

  it('첫 페이지에서는 이전이, 마지막에서는 다음이 막힌다', () => {
    expect(
      resolveGroupPageDisplay([{ total: 4, page: 0, pageCount: 2, truncated: false }]).canPrev,
    ).toBe(false);
    expect(
      resolveGroupPageDisplay([{ total: 4, page: 1, pageCount: 2, truncated: false }]).canNext,
    ).toBe(false);
  });

  it('여러 항목은 총계 합 · 페이지 수 최댓값으로 접는다', () => {
    // 페이지 커서는 패널당 하나이므로 항목별로 나누지 않는다.
    const d = resolveGroupPageDisplay([
      { total: 3, page: 0, pageCount: 2, truncated: false },
      { total: 7, page: 0, pageCount: 4, truncated: false },
    ]);
    expect(d.total).toBe(10);
    expect(d.pageCount).toBe(4);
  });

  it('한 항목이라도 절단되면 절단으로 본다', () => {
    const d = resolveGroupPageDisplay([
      { total: 3, page: 0, pageCount: 1, truncated: false },
      { total: 9, page: 0, pageCount: 1, truncated: true },
    ]);
    expect(d.truncated).toBe(true);
  });
});
