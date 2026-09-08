// 요약 타일 — 표시 선택과 항목별 디자인.

import { describe, it, expect } from 'vitest';

import {
  readListPanelStyle,
  readSummaryItems,
  readSummaryTileFont,
  resolveSummaryTileStyle,
  SUMMARY_ITEMS,
} from './listPanelStyle';

describe('readSummaryItems', () => {
  it('미설정이면 셋 다 기본 순서로 — 지금까지의 화면이 그대로 산다', () => {
    expect(readSummaryItems(undefined)).toEqual(SUMMARY_ITEMS);
    expect(readSummaryItems({})).toEqual(SUMMARY_ITEMS);
  });

  it('고른 것만 고른 차례로 낸다', () => {
    expect(readSummaryItems({ summaryItems: ['active', 'total'] })).toEqual(['active', 'total']);
  });

  it('전부 끄면 하나도 내지 않는다', () => {
    expect(readSummaryItems({ summaryItems: [] })).toEqual([]);
  });

  it('알 수 없는 값과 중복은 걸러 낸다', () => {
    expect(readSummaryItems({ summaryItems: ['active', 'nope', 'active'] })).toEqual(['active']);
  });

  it('배열이 아니면 기본으로 돌아간다', () => {
    expect(readSummaryItems({ summaryItems: 'active' })).toEqual(SUMMARY_ITEMS);
  });
});

describe('readSummaryTileFont', () => {
  it('없으면 빈 설정', () => {
    expect(readSummaryTileFont(undefined, 'total')).toEqual({});
    expect(readSummaryTileFont({ summaryStyles: {} }, 'total')).toEqual({});
  });

  it('타일 이름으로 찾는다', () => {
    const config = { summaryStyles: { active: { size: 20 } } };
    expect(readSummaryTileFont(config, 'active')).toEqual({ size: 20 });
    expect(readSummaryTileFont(config, 'total')).toEqual({});
  });
});

describe('resolveSummaryTileStyle', () => {
  const base = readListPanelStyle({ badge_font: { size: 12 } }).badgeStyle;

  it('타일별 설정이 없으면 공통 배지 설정을 그대로 쓴다', () => {
    const got = resolveSummaryTileStyle(base, {});
    expect(got.style).toBe(base);
  });

  it('타일별 설정이 공통 설정을 덮는다', () => {
    const got = resolveSummaryTileStyle(base, { size: 24 });
    expect(got.style).toMatchObject({ fontSize: '24px' });
  });

  it('타일 색을 정하면 배경 틴트도 그 색으로 다시 만든다 — 남겨 두면 글자와 배경이 어긋난다', () => {
    const withColor = readListPanelStyle({ badge_font: { color: '#ff0000' } }).badgeStyle;
    const got = resolveSummaryTileStyle(withColor, { color: '#00ff00' });
    expect(got.style).toMatchObject({ color: '#00ff00', backgroundColor: '#00ff0020' });
  });

  it('배경색을 직접 정하면 그것이 이긴다', () => {
    const got = resolveSummaryTileStyle(base, { color: '#00ff00', bg: '#111111' });
    expect(got.style).toMatchObject({ backgroundColor: '#111111' });
    expect(got.hasOwnBackground).toBe(true);
  });

  it('아무것도 정하지 않았으면 배경을 갖지 않는다 — 타일별 기본 색이 살아야 한다', () => {
    expect(resolveSummaryTileStyle(undefined, {}).hasOwnBackground).toBe(false);
  });

  it('형식이 아닌 배경색은 무시한다', () => {
    expect(resolveSummaryTileStyle(undefined, { bg: 'red' }).hasOwnBackground).toBe(false);
    expect(resolveSummaryTileStyle(undefined, { bg: 123 }).hasOwnBackground).toBe(false);
  });
});
