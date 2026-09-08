// 목록형 패널 디자인 설정 해석 검증.
//
// 글자 모양(글꼴·크기·굵기·색)을 자리마다 고르던 설정 넷(`title_font` ·
// `table_header_font` · `table_cell_font` · `badge_font`)은 걷어냈다. 그래서 여기서
// 잠그는 것은 둘로 줄었다:
//   1. 남은 유일한 색 출처는 악센트 설정(`accentElements` / `panelColor`) 하나다.
//   2. 본문 셀은 색을 정할 자리가 아예 없다 — 패널 기본 클래스가 그대로 산다.

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
  it('아무것도 정하지 않으면 스타일을 만들지 않는다 — 패널 기본 모양이 산다', () => {
    const style = readListPanelStyle({});
    expect(style.headerStyle).toBeUndefined();
  });

  it('걷어낸 글자 설정이 남아 있어도 읽지 않는다 — 저장된 값이 되살아나면 안 된다', () => {
    const style = readListPanelStyle({
      table_header_font: { size: 20, weight: 'bold', color: '#00ff00' },
      table_cell_font: { size: 12 },
    });

    expect(style.headerStyle).toBeUndefined();
    expect(style.headerAccent).toBeUndefined();
  });
});

describe('악센트 색 폴백', () => {
  it('헤더 색은 table 악센트가 정한다', () => {
    const style = readListPanelStyle({ accentElements: { table: '#ff0000' } });
    expect(style.headerStyle).toMatchObject({ color: '#ff0000' });
    expect(style.headerAccent).toBe('#ff0000');
  });

  it('걷어낸 글자색이 악센트를 이기지 못한다 — 이제 색 출처는 악센트 하나다', () => {
    const style = readListPanelStyle({
      accentElements: { table: '#ff0000' },
      table_header_font: { color: '#00ff00' },
    });
    expect(style.headerStyle).toMatchObject({ color: '#ff0000' });
  });

  it('패널 바탕색이 그룹 색의 기본이 된다', () => {
    const style = readListPanelStyle({ panelColor: '#123456' });
    expect(style.headerStyle).toMatchObject({ color: '#123456' });
  });

  it('그룹을 끄면(false) 색을 물려받지 않는다', () => {
    const style = readListPanelStyle({ panelColor: '#123456', accentElements: { table: false } });
    expect(style.headerStyle).toBeUndefined();
  });
});

describe('요약 배지 디자인', () => {
  it('악센트 색을 정하면 같은 색을 옅게 깔아 알약 모양을 만든다', () => {
    const style = readListPanelStyle({ accentElements: { badges: '#3b82f6' } });
    expect(style.badgeStyle).toMatchObject({
      color: '#3b82f6',
      backgroundColor: '#3b82f620',
    });
  });

  it('색이 없으면 스타일을 만들지 않는다 — 상태별 기본색(초록·빨강)이 살아야 한다', () => {
    const style = readListPanelStyle({});
    expect(style.badgeStyle).toBeUndefined();
  });

  it('걷어낸 배지 글자 설정은 읽지 않는다', () => {
    const style = readListPanelStyle({ badge_font: { size: 14, color: '#3b82f6' } });
    expect(style.badgeStyle).toBeUndefined();
  });

  it('패널 바탕색도 배지 색의 기본이 된다', () => {
    const style = readListPanelStyle({ panelColor: '#abcdef' });
    expect(style.badgeStyle).toMatchObject({
      color: '#abcdef',
      backgroundColor: '#abcdef20',
    });
  });
});
