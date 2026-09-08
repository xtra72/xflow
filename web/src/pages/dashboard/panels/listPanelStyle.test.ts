// 플로우 현황 패널 디자인 설정 해석 검증.
//
// 잠그는 것 둘:
//   1. 각 자리(테이블 헤더·요소·배지)의 모양은 **디자인 설정**이 정한다.
//   2. 디자인 설정이 색을 정하기 전까지는 종전 악센트 색이 그대로 산다 — 설정 화면이
//      바뀌었다고 이미 쓰던 패널의 색이 달라지면 안 된다.

import { describe, expect, it } from 'vitest';

import { readListPanelStyle } from './listPanelStyle';

describe('요약 배지 사용 여부', () => {
  it('기본은 표시다', () => {
    expect(readListPanelStyle(undefined).showSummaryBadges).toBe(true);
    expect(readListPanelStyle({}).showSummaryBadges).toBe(true);
  });

  it('명시적으로 false 일 때만 숨긴다', () => {
    expect(readListPanelStyle({ showSummaryBadges: false }).showSummaryBadges).toBe(false);
    expect(readListPanelStyle({ showSummaryBadges: true }).showSummaryBadges).toBe(true);
  });
});

describe('테이블 디자인', () => {
  it('헤더와 요소의 글자 모양을 따로 정한다', () => {
    const style = readListPanelStyle({
      table_header_font: { size: 20, weight: 'bold' },
      table_cell_font: { size: 12 },
    });

    expect(style.headerStyle).toMatchObject({ fontSize: '20px', fontWeight: 'bold' });
    expect(style.cellStyle).toMatchObject({ fontSize: '12px' });
    expect(style.cellStyle?.fontWeight).toBeUndefined();
  });

  it('아무것도 정하지 않으면 스타일을 만들지 않는다 — 패널 기본 모양이 산다', () => {
    const style = readListPanelStyle({});
    expect(style.headerStyle).toBeUndefined();
    expect(style.cellStyle).toBeUndefined();
  });
});

describe('악센트 색 폴백', () => {
  it('헤더 색을 정하지 않았으면 종전 table 악센트를 쓴다', () => {
    const style = readListPanelStyle({ accentElements: { table: '#ff0000' } });
    expect(style.headerStyle).toMatchObject({ color: '#ff0000' });
  });

  it('디자인 설정의 색이 악센트를 이긴다', () => {
    const style = readListPanelStyle({
      accentElements: { table: '#ff0000' },
      table_header_font: { color: '#00ff00' },
    });
    expect(style.headerStyle).toMatchObject({ color: '#00ff00' });
  });

  it('패널 바탕색이 그룹 색의 기본이 된다', () => {
    const style = readListPanelStyle({ panelColor: '#123456' });
    expect(style.headerStyle).toMatchObject({ color: '#123456' });
  });

  it('그룹을 끄면(false) 색을 물려받지 않는다', () => {
    const style = readListPanelStyle({ panelColor: '#123456', accentElements: { table: false } });
    expect(style.headerStyle).toBeUndefined();
  });

  it('본문 셀은 악센트를 물려받지 않는다 — 종전에도 색이 걸리지 않던 자리다', () => {
    const style = readListPanelStyle({ accentElements: { table: '#ff0000' } });
    expect(style.cellStyle).toBeUndefined();
  });
});

describe('요약 배지 디자인', () => {
  it('색을 정하면 같은 색을 옅게 깔아 알약 모양을 만든다', () => {
    const style = readListPanelStyle({ badge_font: { color: '#3b82f6' } });
    expect(style.badgeStyle).toMatchObject({
      color: '#3b82f6',
      backgroundColor: '#3b82f620',
    });
  });

  it('색이 없으면 배경을 깔지 않는다 — 상태별 기본색(초록·빨강)이 살아야 한다', () => {
    const style = readListPanelStyle({ badge_font: { size: 14 } });
    expect(style.badgeStyle).toMatchObject({ fontSize: '14px' });
    expect(style.badgeStyle?.backgroundColor).toBeUndefined();
  });

  it('종전 badges 악센트 색도 그대로 적용된다', () => {
    const style = readListPanelStyle({ accentElements: { badges: '#abcdef' } });
    expect(style.badgeStyle).toMatchObject({
      color: '#abcdef',
      backgroundColor: '#abcdef20',
    });
  });
});
