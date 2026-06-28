// secretOmission 단위 테스트 (SPEC-REMOTE-001 M7, REQ-I07).
//
// 마스킹/미변경 시크릿 필드가 갱신 페이로드에서 생략되고(필드 부재), 사용자가
// 실제로 입력한 새 시크릿 값은 보존되는지 검증한다.

import { describe, expect, it } from 'vitest';

import {
  isSensitiveConfigKey,
  omitMaskedSecrets,
  SENSITIVE_CONFIG_KEYS,
} from './secretOmission';

describe('secretOmission — 키 집합 (백엔드 SoT 일치)', () => {
  it('백엔드 secret_fields.go 와 동일한 키 집합을 갖는다', () => {
    expect([...SENSITIVE_CONFIG_KEYS].sort()).toEqual(
      [
        'access_token',
        'api_key',
        'apikey',
        'auth_token',
        'client_secret',
        'passphrase',
        'password',
        'private_key',
        'secret',
        'token',
        'username',
      ].sort(),
    );
  });

  it('isSensitiveConfigKey 는 대소문자를 무시한다', () => {
    expect(isSensitiveConfigKey('PASSWORD')).toBe(true);
    expect(isSensitiveConfigKey('Api_Key')).toBe(true);
    expect(isSensitiveConfigKey('host')).toBe(false);
  });
});

describe('secretOmission — omitMaskedSecrets', () => {
  it('빈 시크릿 필드는 완전히 생략한다 (필드 부재)', () => {
    const result = omitMaskedSecrets({
      host: 'mqtt.local',
      password: '',
      token: '   ',
      username: undefined,
    });
    expect(result).toEqual({ host: 'mqtt.local' });
    expect('password' in result).toBe(false);
    expect('token' in result).toBe(false);
    expect('username' in result).toBe(false);
  });

  it('미러에 부재한 시크릿 키는 그대로 부재한다 (마스킹 라운드트립 보존)', () => {
    // 노드가 redaction 으로 키를 삭제한 정의 — 시크릿 키 자체가 없다.
    const redacted = { host: 'mqtt.local', port: 1883 };
    const result = omitMaskedSecrets(redacted);
    expect(result).toEqual({ host: 'mqtt.local', port: 1883 });
  });

  it('사용자가 새로 입력한 시크릿 값은 보존한다', () => {
    const result = omitMaskedSecrets({
      host: 'mqtt.local',
      password: 'new-secret',
    });
    expect(result).toEqual({ host: 'mqtt.local', password: 'new-secret' });
  });

  it('중첩 맵 안의 시크릿 필드도 재귀적으로 생략한다', () => {
    const result = omitMaskedSecrets({
      nodes: {
        n1: { type: 'mqtt-in', config: { topic: 't', password: '' } },
        n2: { type: 'http', config: { url: 'u', api_key: 'kept' } },
      },
    });
    expect(result).toEqual({
      nodes: {
        n1: { type: 'mqtt-in', config: { topic: 't' } },
        n2: { type: 'http', config: { url: 'u', api_key: 'kept' } },
      },
    });
  });

  it('배열 요소 안의 시크릿 필드도 재귀적으로 생략한다', () => {
    const result = omitMaskedSecrets({
      nodes: [
        { id: 'a', secret: '' },
        { id: 'b', secret: 'real' },
      ],
    });
    expect(result).toEqual({
      nodes: [{ id: 'a' }, { id: 'b', secret: 'real' }],
    });
  });

  it('입력 객체를 변경하지 않는다 (깊은 사본 반환)', () => {
    const input = { config: { password: '' } };
    const result = omitMaskedSecrets(input);
    expect(input).toEqual({ config: { password: '' } });
    expect(result).not.toBe(input);
  });

  it('스칼라/널 값은 그대로 반환한다', () => {
    expect(omitMaskedSecrets(42)).toBe(42);
    expect(omitMaskedSecrets('text')).toBe('text');
    expect(omitMaskedSecrets(null)).toBe(null);
  });
});
