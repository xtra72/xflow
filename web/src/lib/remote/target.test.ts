// target 직렬화/파싱 단위 테스트 (SPEC-REMOTE-001 M8, REQ-J13).

import { describe, expect, it } from 'vitest';

import {
  isRemoteTarget,
  LOCAL_TARGET,
  parseTargetParam,
  serializeTargetParam,
  type ResourceTarget,
} from './target';

describe('parseTargetParam', () => {
  it('미지정/null/local 은 로컬 타깃이다', () => {
    expect(parseTargetParam(null)).toEqual(LOCAL_TARGET);
    expect(parseTargetParam(undefined)).toEqual(LOCAL_TARGET);
    expect(parseTargetParam('')).toEqual(LOCAL_TARGET);
    expect(parseTargetParam('local')).toEqual(LOCAL_TARGET);
  });

  it('remote:{id} 는 원격 타깃으로 파싱된다', () => {
    expect(parseTargetParam('remote:node-1')).toEqual({
      type: 'remote',
      instanceId: 'node-1',
    });
  });

  it('인코딩된 instanceId 를 디코딩한다', () => {
    expect(parseTargetParam('remote:a%2Fb%20c')).toEqual({
      type: 'remote',
      instanceId: 'a/b c',
    });
  });

  it('remote: 접두사 뒤가 비면 로컬로 폴백한다', () => {
    expect(parseTargetParam('remote:')).toEqual(LOCAL_TARGET);
  });

  it('알 수 없는 형식은 로컬로 폴백한다', () => {
    expect(parseTargetParam('garbage')).toEqual(LOCAL_TARGET);
  });
});

describe('serializeTargetParam', () => {
  it('로컬은 빈 문자열을 반환한다', () => {
    expect(serializeTargetParam(LOCAL_TARGET)).toBe('');
  });

  it('원격은 remote:{encodedId} 를 반환한다', () => {
    const t: ResourceTarget = { type: 'remote', instanceId: 'a/b c' };
    expect(serializeTargetParam(t)).toBe('remote:a%2Fb%20c');
  });

  it('파싱 ↔ 직렬화 라운드트립이 일치한다', () => {
    const t: ResourceTarget = { type: 'remote', instanceId: 'node-77' };
    expect(parseTargetParam(serializeTargetParam(t))).toEqual(t);
  });
});

describe('isRemoteTarget', () => {
  it('타입 가드로 동작한다', () => {
    expect(isRemoteTarget(LOCAL_TARGET)).toBe(false);
    expect(isRemoteTarget({ type: 'remote', instanceId: 'n' })).toBe(true);
  });
});
