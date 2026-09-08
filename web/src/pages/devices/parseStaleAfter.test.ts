// 갱신 시간 제한 입력 해석 — 0 과 미설정을 가르는 것이 요점이다.

import { describe, it, expect } from 'vitest';

import { parseStaleAfter } from './deviceDisplay';

describe('parseStaleAfter', () => {
  it('양수는 초로 읽는다', () => {
    expect(parseStaleAfter('300')).toBe(300);
    expect(parseStaleAfter(' 60 ')).toBe(60);
  });

  it('비우면 미설정 — 제한을 지우는 유일한 수단이다', () => {
    expect(parseStaleAfter('')).toBeUndefined();
    expect(parseStaleAfter('   ')).toBeUndefined();
  });

  it('0 과 음수는 미설정으로 본다 — 그대로 보내면 방금 온 값도 오래된 것이 된다', () => {
    expect(parseStaleAfter('0')).toBeUndefined();
    expect(parseStaleAfter('-30')).toBeUndefined();
  });

  it('숫자가 아니면 미설정', () => {
    expect(parseStaleAfter('abc')).toBeUndefined();
  });

  it('소수는 초 단위로 반올림한다', () => {
    expect(parseStaleAfter('60.4')).toBe(60);
    expect(parseStaleAfter('60.6')).toBe(61);
  });
});
