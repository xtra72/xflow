// aliasTemplate 태그 토큰 리졸버 단위 테스트 (SPEC-WEB-005).

import { describe, it, expect } from 'vitest';

import {
  ALIAS_TOKEN_REGEX,
  extractAliasTokens,
  makeAliasToken,
  resolveSeriesAlias,
} from './aliasTemplate';

const tags = { name: 'TempSensor', type: 'inside', room: '1' };

describe('resolveSeriesAlias', () => {
  it('단일 토큰을 태그 값으로 치환한다', () => {
    expect(resolveSeriesAlias('{$.name}', tags)).toBe('TempSensor');
  });

  it('여러 토큰 + 리터럴 텍스트를 모두 치환한다', () => {
    expect(resolveSeriesAlias('{$.name}-{$.type}', tags)).toBe('TempSensor-inside');
    expect(resolveSeriesAlias('[{$.room}] {$.name}', tags)).toBe('[1] TempSensor');
  });

  it('누락된 태그 키는 빈 문자열로 치환한다(리터럴 유지)', () => {
    expect(resolveSeriesAlias('{$.name}-{$.missing}', tags)).toBe('TempSensor-');
    expect(resolveSeriesAlias('a{$.missing}b', tags)).toBe('ab');
  });

  it('빈 태그 값은 빈 문자열로 치환한다', () => {
    expect(resolveSeriesAlias('{$.x}', { x: '' })).toBe('');
  });

  it('토큰이 없는 템플릿은 입력을 그대로 반환한다(plain 텍스트 하위 호환)', () => {
    expect(resolveSeriesAlias('plain name', tags)).toBe('plain name');
    expect(resolveSeriesAlias('', tags)).toBe('');
  });

  it('잘못된 중괄호 패턴은 토큰으로 인식되지 않아 리터럴로 남는다', () => {
    // `{$.}` (키 없음), `{name}` (접두사 없음), `{$ .name}` (공백) 등.
    expect(resolveSeriesAlias('{$.}', tags)).toBe('{$.}');
    expect(resolveSeriesAlias('{name}', tags)).toBe('{name}');
    expect(resolveSeriesAlias('{$ .name}', tags)).toBe('{$ .name}');
    expect(resolveSeriesAlias('{$.na me}', tags)).toBe('{$.na me}');
  });

  it('같은 토큰이 여러 번 등장해도 모두 치환한다', () => {
    expect(resolveSeriesAlias('{$.name}/{$.name}', tags)).toBe('TempSensor/TempSensor');
  });

  it('태그 키 문자셋(영숫자/_/-)을 지원한다', () => {
    expect(
      resolveSeriesAlias('{$.a_b}-{$.c-d}', { a_b: 'X', 'c-d': 'Y' }),
    ).toBe('X-Y');
  });

  it('정규식은 전역 상태(lastIndex)에 영향받지 않고 반복 호출 가능하다', () => {
    expect(resolveSeriesAlias('{$.name}', tags)).toBe('TempSensor');
    expect(resolveSeriesAlias('{$.name}', tags)).toBe('TempSensor');
  });
});

describe('extractAliasTokens', () => {
  it('등장 순서대로 중복 제거된 토큰 키를 반환한다', () => {
    expect(extractAliasTokens('{$.name}-{$.type}-{$.name}')).toEqual(['name', 'type']);
  });
  it('토큰이 없으면 빈 배열', () => {
    expect(extractAliasTokens('plain')).toEqual([]);
    expect(extractAliasTokens('')).toEqual([]);
  });
});

describe('makeAliasToken / ALIAS_TOKEN_REGEX', () => {
  it('makeAliasToken 은 {$.key} 형태를 만든다', () => {
    expect(makeAliasToken('room')).toBe('{$.room}');
  });
  it('정규식 소스가 태그 키 문자셋을 매칭한다', () => {
    const re = new RegExp(ALIAS_TOKEN_REGEX.source, 'g');
    const m = re.exec('{$.room}');
    expect(m?.[1]).toBe('room');
  });
});
