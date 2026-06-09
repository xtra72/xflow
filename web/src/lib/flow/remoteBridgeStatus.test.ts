// SPEC-SUBFLOW-001 v1.3 (REQ-RU06): 원격 브릿지 상태 매핑 단위 테스트.

import { describe, expect, it } from 'vitest';

import {
  BRIDGE_STATUS_DOT_CLASS,
  BRIDGE_STATUS_I18N_KEY,
  mapRuntimeStateToBridgeStatus,
  type RemoteBridgeStatus,
} from './remoteBridgeStatus';

describe('mapRuntimeStateToBridgeStatus', () => {
  it('비어있음/undefined 는 unknown (라이브 상태 없음)', () => {
    expect(mapRuntimeStateToBridgeStatus(undefined)).toBe('unknown');
    expect(mapRuntimeStateToBridgeStatus('')).toBe('unknown');
    expect(mapRuntimeStateToBridgeStatus('   ')).toBe('unknown');
  });

  it('running/active 는 running', () => {
    expect(mapRuntimeStateToBridgeStatus('running')).toBe('running');
    expect(mapRuntimeStateToBridgeStatus('active')).toBe('running');
    // 대소문자/공백 정규화
    expect(mapRuntimeStateToBridgeStatus('  RUNNING  ')).toBe('running');
  });

  it('connected/online/ready 는 connected', () => {
    expect(mapRuntimeStateToBridgeStatus('connected')).toBe('connected');
    expect(mapRuntimeStateToBridgeStatus('online')).toBe('connected');
    expect(mapRuntimeStateToBridgeStatus('ready')).toBe('connected');
  });

  it('offline/disconnected/stopped/idle 는 offline', () => {
    expect(mapRuntimeStateToBridgeStatus('offline')).toBe('offline');
    expect(mapRuntimeStateToBridgeStatus('disconnected')).toBe('offline');
    expect(mapRuntimeStateToBridgeStatus('stopped')).toBe('offline');
    expect(mapRuntimeStateToBridgeStatus('idle')).toBe('offline');
  });

  it('error/failed/fault 는 error', () => {
    expect(mapRuntimeStateToBridgeStatus('error')).toBe('error');
    expect(mapRuntimeStateToBridgeStatus('failed')).toBe('error');
    expect(mapRuntimeStateToBridgeStatus('fault')).toBe('error');
  });

  it('알 수 없는 비어있지 않은 state 는 connected(중립 활성)로 폴백', () => {
    expect(mapRuntimeStateToBridgeStatus('whatever')).toBe('connected');
    expect(mapRuntimeStateToBridgeStatus('starting')).toBe('connected');
  });

  it('모든 상태에 대응하는 색상 클래스/i18n 키가 정의되어 있다', () => {
    const all: RemoteBridgeStatus[] = [
      'running',
      'connected',
      'offline',
      'error',
      'unknown',
    ];
    for (const s of all) {
      expect(BRIDGE_STATUS_DOT_CLASS[s]).toBeTruthy();
      expect(BRIDGE_STATUS_I18N_KEY[s]).toMatch(/^remote\.bridge\.status\./);
    }
  });
});
