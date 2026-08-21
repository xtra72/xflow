// aliasTemplate 태그 토큰 리졸버 단위 테스트 (SPEC-WEB-005).

import { describe, it, expect } from 'vitest';

import {
  ALIAS_TOKEN_REGEX,
  availableAliasTokens,
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
  it('makeAliasToken 은 {$.measurement} 형태를 만든다', () => {
    expect(makeAliasToken('room')).toBe('{$.room}');
  });
  it('정규식 소스가 태그 키 문자셋을 매칭한다', () => {
    const re = new RegExp(ALIAS_TOKEN_REGEX.source, 'g');
    const m = re.exec('{$.room}');
    expect(m?.[1]).toBe('room');
  });
});

// --- 키 / field 토큰 (시리즈 이름 조립) ---

describe('resolveSeriesAlias — 키/필드 토큰', () => {
  const ctx = {
    measurement: 'dev-1',
    field: 'temperature',
    tags: { name: 'TempSensor', type: 'inside', room: '1' },
  };

  it('{$.measurement} 를 시리즈 키로 치환한다', () => {
    expect(resolveSeriesAlias('{$.measurement}', ctx)).toBe('dev-1');
  });

  it('{$.field} 를 field 으로 치환한다', () => {
    expect(resolveSeriesAlias('{$.field}', ctx)).toBe('temperature');
  });

  it('{$.tags.NAME} 명시 형식으로 태그를 가리킨다', () => {
    expect(resolveSeriesAlias('{$.tags.room}', ctx)).toBe('1');
  });

  it('키·필드·태그·리터럴을 섞어 이름을 조립한다', () => {
    expect(resolveSeriesAlias('[{$.tags.room}] {$.measurement}/{$.field}', ctx)).toBe(
      '[1] dev-1/temperature',
    );
  });

  it('키/필드이 없으면 빈 문자열로 치환한다', () => {
    expect(resolveSeriesAlias('{$.measurement}-{$.field}', { tags: {} })).toBe('-');
  });

  it('태그 맵만 넘기는 기존 호출 형태를 그대로 지원한다', () => {
    expect(resolveSeriesAlias('{$.name}', tags)).toBe('TempSensor');
  });

  it('key/field 이름의 태그는 단축 형식이 예약어에 가려지고 명시 형식으로 접근한다', () => {
    const shadowed = { measurement: 'dev-1', tags: { measurement: 'TAG-KEY' } };
    expect(resolveSeriesAlias('{$.measurement}', shadowed)).toBe('dev-1');
    expect(resolveSeriesAlias('{$.tags.measurement}', shadowed)).toBe('TAG-KEY');
  });
});

describe('availableAliasTokens', () => {
  it('measurement·field 를 먼저, 이어서 태그 토큰을 나열한다', () => {
    expect(
      availableAliasTokens({ measurement: 'dev-1', field: 'temperature', tags: { room: '1' } }),
    ).toEqual(['measurement', 'field', 'room']);
  });

  it('없는 값의 토큰은 제외한다', () => {
    expect(availableAliasTokens({ measurement: 'dev-1', tags: {} })).toEqual(['measurement']);
    expect(availableAliasTokens({})).toEqual([]);
  });

  it('예약어와 겹치는 태그는 명시 형식으로 노출한다', () => {
    expect(availableAliasTokens({ measurement: 'dev-1', tags: { measurement: 'x', room: '1' } })).toEqual([
      'measurement',
      'tags.measurement',
      'room',
    ]);
  });
});

describe('extractAliasTokens — 경로 토큰', () => {
  it('예약어와 명시 태그 경로를 원문 그대로 반환한다', () => {
    expect(extractAliasTokens('{$.measurement}/{$.field} [{$.tags.room}]')).toEqual([
      'measurement',
      'field',
      'tags.room',
    ]);
  });
});
