// resolveRemoteNodeLabel / shortenInstanceId 테스트 — 호스트명 우선 + 폴백 검증.

import { describe, expect, it } from 'vitest';

import { resolveRemoteNodeLabel, shortenInstanceId } from './nodeLabel';

describe('shortenInstanceId', () => {
  it('8자 이하면 원본을 그대로 반환한다', () => {
    expect(shortenInstanceId('short')).toBe('short');
    expect(shortenInstanceId('12345678')).toBe('12345678');
  });

  it('8자 초과면 앞 8자 + 생략부호로 단축한다', () => {
    expect(shortenInstanceId('inst-uuid-1234')).toBe('inst-uui…');
  });
});

describe('resolveRemoteNodeLabel', () => {
  it('호스트명이 있으면 호스트명을 우선 사용한다', () => {
    expect(resolveRemoteNodeLabel('gw-1', 'inst-uuid-1234')).toBe('gw-1');
  });

  it('호스트명이 없으면(미조회/지연/실패) 단축 instanceId 로 폴백한다', () => {
    expect(resolveRemoteNodeLabel('', 'inst-uuid-1234')).toBe('inst-uui…');
    expect(resolveRemoteNodeLabel(undefined, 'inst-uuid-1234')).toBe('inst-uui…');
    expect(resolveRemoteNodeLabel(null, 'inst-uuid-1234')).toBe('inst-uui…');
  });

  it('둘 다 없으면 빈 문자열을 반환한다', () => {
    expect(resolveRemoteNodeLabel('', '')).toBe('');
    expect(resolveRemoteNodeLabel(undefined, undefined)).toBe('');
  });
});
