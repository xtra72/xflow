// 로그 항목 → 대상 페이지 링크 테스트.

import { describe, it, expect } from 'vitest';

import { isLinkableLogComponent, logComponentTarget } from './logNavigation';

describe('logComponentTarget', () => {
  it('에이전트·플로우·노드는 각 목록 페이지로 간다', () => {
    expect(logComponentTarget('agent', 'modbus-1')).toBe('/agents?name=modbus-1');
    expect(logComponentTarget('flow', 'main')).toBe('/flows?name=main');
    expect(logComponentTarget('node', 'inject')).toBe('/nodes?name=inject');
  });

  it('목록 페이지가 없는 소스는 링크하지 않는다', () => {
    expect(logComponentTarget('api', 'x')).toBeNull();
    expect(logComponentTarget('engine', 'x')).toBeNull();
    expect(logComponentTarget('system', 'x')).toBeNull();
  });

  it('소스나 이름이 비면 링크하지 않는다', () => {
    expect(logComponentTarget(undefined, 'x')).toBeNull();
    expect(logComponentTarget('agent', undefined)).toBeNull();
    expect(logComponentTarget('agent', '')).toBeNull();
  });

  it('이름을 URL 인코딩한다', () => {
    // 공백·한글·& 가 들어간 이름이 쿼리를 깨지 않아야 한다.
    expect(logComponentTarget('agent', 'a b&c')).toBe('/agents?name=a%20b%26c');
    expect(logComponentTarget('agent', '온도 센서')).toBe(
      '/agents?name=%EC%98%A8%EB%8F%84%20%EC%84%BC%EC%84%9C',
    );
  });
});

describe('isLinkableLogComponent', () => {
  it('이동 가능 여부를 알려준다', () => {
    expect(isLinkableLogComponent('agent', 'a')).toBe(true);
    expect(isLinkableLogComponent('system', 'a')).toBe(false);
  });
});
