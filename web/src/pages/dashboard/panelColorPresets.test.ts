// 패널 색상 자유 입력의 값 판정 — `normalizePanelColor`.
//
// 이 함수가 저장 형식의 유일한 문지기다. 통과한 값은 `--panel-accent` 와
// `border: 4px solid ${panelColor}` 에 그대로 들어가므로(`RemoteDashboardView`),
// "무엇을 통과시키지 않는가" 가 "무엇을 통과시키는가" 만큼 중요하다.

import { describe, expect, it } from 'vitest';

import { PANEL_COLORS, normalizePanelColor } from './panelColorPresets';

describe('normalizePanelColor — 받아들이는 값', () => {
  it('6자리 헥스를 그대로 통과시킨다', () => {
    expect(normalizePanelColor('#3b82f6')).toBe('#3b82f6');
  });

  it('# 없이 쳐도 붙여서 받는다 — 붙여넣기 습관을 막지 않는다', () => {
    expect(normalizePanelColor('3b82f6')).toBe('#3b82f6');
  });

  it('대문자는 소문자로 내린다', () => {
    expect(normalizePanelColor('#8B5CF6')).toBe('#8b5cf6');
    expect(normalizePanelColor('ABCDEF')).toBe('#abcdef');
  });

  it('3자리 약식은 6자리로 편다', () => {
    // `<input type="color">` 는 7자(#rrggbb)만 읽으므로 약식을 남겨두면
    // 네이티브 피커가 값을 잃고 검정으로 되돌아간다.
    expect(normalizePanelColor('#abc')).toBe('#aabbcc');
    expect(normalizePanelColor('f00')).toBe('#ff0000');
    expect(normalizePanelColor('#ABC')).toBe('#aabbcc');
  });

  it('앞뒤 공백을 흘려보낸다', () => {
    expect(normalizePanelColor('  #3b82f6  ')).toBe('#3b82f6');
  });

  it('소문자로 내린 값이 프리셋과 문자열로 같아진다', () => {
    // 선택 표시가 `panelColor === color` 비교라 대소문자가 어긋나면
    // 같은 색인데도 스와치에 표시가 붙지 않는다.
    expect(normalizePanelColor('#8B5CF6')).toBe(PANEL_COLORS[1]);
  });
});

describe('normalizePanelColor — 거절하는 값', () => {
  it.each([
    ['빈 문자열', ''],
    ['미완성 입력', '#ab'],
    ['4자리', '#abcd'],
    ['5자리', '#abcde'],
    ['7자리', '#abcdef0'],
    ['8자리 알파 채널', '#abcdef80'],
    ['헥스가 아닌 글자', '#gggggg'],
    ['색 이름', 'red'],
    ['rgb 함수', 'rgb(1,2,3)'],
    ['CSS 주입 시도', '#fff; background: url(x)'],
    ['한글', '파랑'],
  ])('%s 은 null 이다', (_label, input) => {
    expect(normalizePanelColor(input)).toBeNull();
  });
});
