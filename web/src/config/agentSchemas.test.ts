// xsfm 에이전트 설정 스키마 — 상태 방출 옵션 3종 테스트 (SPEC-XSFM-AGENT-IO-001 M5, REQ-05).
//
// 검증 대상:
//   - forward_received_to_node(boolean, 기본 false)
//   - state_emit_mode(enum event/interval/both, 기본 event)
//   - state_emit_interval(duration string, 기본 60s, mode=interval|both 일 때만 표시)
// 세 필드는 백엔드 Transport.Options 키와 1:1 매핑되어야 하며, 기본값은 현행 동작과
// 바이트 동일(forward off, mode=event)이어야 한다(무회귀, REQ-04-01/REQ-05-02).

import { describe, expect, it } from 'vitest';

import { getAgentConfigDefaults, getAgentConfigSchema } from './agentSchemas';
import type { ConfigField } from '@/types/node';
import { isFieldVisible } from '@/types/node';

/** xsfm 스키마에서 name 으로 필드 하나를 찾는다. */
function xsfmField(name: string): ConfigField | undefined {
  return getAgentConfigSchema('xsfm')?.fields.find((f) => f.name === name);
}

/**
 * AgentDetailPanel.TwoColumnConfigLayout 의 filterVisible 술어와 동형.
 * visibleWhen 조건을 현재 config 값에 대해 평가한다(패널 렌더 로직을 그대로 미러).
 */
function isVisible(field: ConfigField, data: Record<string, unknown>): boolean {
  if (!field.visibleWhen) return true;
  const actual = data[field.visibleWhen.field];
  const expected = field.visibleWhen.value;
  if (Array.isArray(expected)) return (expected as unknown[]).includes(actual);
  return actual === expected;
}

describe('xsfm 상태 방출 옵션 스키마 (SPEC-XSFM-AGENT-IO-001 M5)', () => {
  it('세 컨트롤이 xsfm 스키마에 노출된다(백엔드 Transport.Options 키와 정확히 일치)', () => {
    const forward = xsfmField('forward_received_to_node');
    const mode = xsfmField('state_emit_mode');
    const interval = xsfmField('state_emit_interval');

    expect(forward).toBeDefined();
    expect(mode).toBeDefined();
    expect(interval).toBeDefined();

    // 위젯 타입: boolean 토글 / enum select / duration string
    expect(forward!.type).toBe('boolean');
    expect(mode!.type).toBe('select');
    expect(interval!.type).toBe('string');
  });

  it('state_emit_mode enum 은 event/interval/both 이며 기본값은 event 이다', () => {
    const mode = xsfmField('state_emit_mode');
    expect(mode!.options).toEqual(['event', 'interval', 'both']);
    expect(mode!.default).toBe('event');
  });

  it('기본값은 현행 동작과 동일하다(forward off, mode=event, interval 60s — 무회귀)', () => {
    const defaults = getAgentConfigDefaults('xsfm');
    expect(defaults.forward_received_to_node).toBe(false);
    expect(defaults.state_emit_mode).toBe('event');
    expect(defaults.state_emit_interval).toBe('60s');
  });

  it('state_emit_interval 은 mode=interval|both 일 때만 표시되고 event 에서는 숨겨진다', () => {
    const interval = xsfmField('state_emit_interval')!;
    // 조건부 표시 규약이 state_emit_mode 에 종속되어야 한다.
    expect(interval.visibleWhen?.field).toBe('state_emit_mode');

    expect(isVisible(interval, { state_emit_mode: 'event' })).toBe(false);
    expect(isVisible(interval, { state_emit_mode: 'interval' })).toBe(true);
    expect(isVisible(interval, { state_emit_mode: 'both' })).toBe(true);
  });

  it('forward_received_to_node 는 항상 표시된다(state_emit_mode 와 독립, REQ-01-06)', () => {
    const forward = xsfmField('forward_received_to_node')!;
    expect(forward.visibleWhen).toBeUndefined();
    expect(isVisible(forward, { state_emit_mode: 'event' })).toBe(true);
    expect(isVisible(forward, { state_emit_mode: 'interval' })).toBe(true);
  });

  it('세 필드 모두 운영(operation) 섹션에 속한다', () => {
    expect(xsfmField('forward_received_to_node')!.section).toBe('operation');
    expect(xsfmField('state_emit_mode')!.section).toBe('operation');
    expect(xsfmField('state_emit_interval')!.section).toBe('operation');
  });
});

describe('samsung_hvacr01 미러 동기화 스키마 (SPEC-HVACR-SYNC-001)', () => {
  const samsungField = (name: string): ConfigField | undefined =>
    getAgentConfigSchema('samsung_hvacr01')?.fields.find((f) => f.name === name);

  it('연결 방식(transport_type) 옵션에 mirror-mqtt 와 mirror-message 가 포함된다', () => {
    const tt = samsungField('transport_type')!;
    expect(tt.options).toContain('mirror-mqtt');
    expect(tt.options).toContain('mirror-message');
    // 이전 단일 'mirror' 값은 mirror-mqtt 로 rename 됨(제거 확인)
    expect(tt.options).not.toContain('mirror');
    // 기존 옵션도 보존(무회귀)
    expect(tt.options).toEqual(['serial', 'tcp-client', 'tcp-server', 'mirror-mqtt', 'mirror-message']);
  });

  it('mirror_broker/gateway_id 는 mirror-mqtt(서버) 또는 업링크(게이트웨이) 양쪽에서 표시된다(OR)', () => {
    const broker = samsungField('mirror_broker')!;
    expect(broker.visibleWhenAny).toBeDefined();
    // 서버 역할: transport_type=mirror-mqtt
    expect(isFieldVisible(broker, { transport_type: 'mirror-mqtt' })).toBe(true);
    // 게이트웨이 역할: serial + 업링크 활성
    expect(isFieldVisible(broker, { transport_type: 'serial', mirror_uplink_enabled: true })).toBe(true);
    // 순수 serial(업링크 미활성): 숨김
    expect(isFieldVisible(broker, { transport_type: 'serial' })).toBe(false);
    expect(isFieldVisible(broker, { transport_type: 'tcp-client' })).toBe(false);
    // mirror-message: MQTT 없음 → 브로커 필드 숨김
    expect(isFieldVisible(broker, { transport_type: 'mirror-message' })).toBe(false);
  });

  it('mirror_uplink_enabled 는 serial/tcp(게이트웨이)에서만 표시되고 mirror-mqtt/mirror-message 에서는 숨겨진다', () => {
    const uplink = samsungField('mirror_uplink_enabled')!;
    expect(isFieldVisible(uplink, { transport_type: 'serial' })).toBe(true);
    expect(isFieldVisible(uplink, { transport_type: 'tcp-server' })).toBe(true);
    expect(isFieldVisible(uplink, { transport_type: 'mirror-mqtt' })).toBe(false);
    expect(isFieldVisible(uplink, { transport_type: 'mirror-message' })).toBe(false);
  });

  it('미러 옵션 키가 백엔드 Transport.Options 와 일치한다', () => {
    for (const key of [
      'mirror_uplink_enabled', 'mirror_broker', 'mirror_gateway_id',
      'mirror_topic_prefix', 'mirror_qos', 'mirror_control_enabled',
      'mirror_ack_enabled', 'mirror_snapshot_enabled',
      // M9: 미러 브로커 보안 (인증 + TLS)
      'mirror_username', 'mirror_password', 'mirror_tls', 'mirror_ca_cert',
    ]) {
      expect(samsungField(key), `누락된 미러 필드: ${key}`).toBeDefined();
    }
  });

  it('mirror_password 는 sensitive 로 마스킹된다 (M9)', () => {
    const pw = samsungField('mirror_password')!;
    expect(pw.sensitive).toBe(true);
  });
});
