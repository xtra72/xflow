// xsfm 에이전트 설정 스키마 — 상태 방출 옵션 3종 테스트 (SPEC-XSFM-AGENT-IO-001 M5, REQ-05).
//
// 검증 대상:
//   - forward_received_to_node(boolean, 기본 false)
//   - state_emit_mode(enum event/interval/both, 기본 event)
//   - state_emit_interval(duration string, 기본 60s, mode=interval|both 일 때만 표시)
// 세 필드는 백엔드 Transport.Options 키와 1:1 매핑되어야 하며, 기본값은 현행 동작과
// 바이트 동일(forward off, mode=event)이어야 한다(무회귀, REQ-04-01/REQ-05-02).

import { describe, expect, it } from 'vitest';

import { AGENT_TYPES, getAgentConfigDefaults, getAgentConfigSchema } from './agentSchemas';
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

// chirpstack 측정치 방출 모드 — opt-in combined 노브.
//
// 백엔드 Transport.Options 키(internal/agent/chirpstack/config.go 의
// measurement_emit_mode)와 1:1 매핑되어야 하며, 기본값은 현행 동작(측정치별 fan-out)과
// 바이트 동일해야 한다(무회귀, REQ-FROZEN-01/02).
describe('chirpstack 측정치 방출 모드 스키마', () => {
  const chirpField = (name: string): ConfigField | undefined =>
    getAgentConfigSchema('chirpstack-client')?.fields.find((f) => f.name === name);

  it('measurement_emit_mode 가 select 위젯으로 노출된다', () => {
    const mode = chirpField('measurement_emit_mode');
    expect(mode).toBeDefined();
    expect(mode!.type).toBe('select');
  });

  it('enum 은 per_measurement/combined 이며 기본값은 per_measurement 이다', () => {
    const mode = chirpField('measurement_emit_mode')!;
    expect(mode.options).toEqual(['per_measurement', 'combined']);
    expect(mode.default).toBe('per_measurement');
  });

  it('기본값이 현행 동작과 동일하다(측정치별 fan-out — 무회귀)', () => {
    const defaults = getAgentConfigDefaults('chirpstack-client');
    expect(defaults.measurement_emit_mode).toBe('per_measurement');
  });

  it('설명이 두 모드의 결과를 각각 명시한다', () => {
    const desc = chirpField('measurement_emit_mode')!.description ?? '';
    expect(desc).toContain('per_measurement');
    expect(desc).toContain('combined');
  });

  it('기존 chirpstack 노브는 그대로 유지된다(필드 제거/이름 변경 없음)', () => {
    for (const key of [
      'broker', 'client_id', 'username', 'password', 'topics', 'qos',
      'keep_alive_sec', 'auto_reconnect', 'clean_session', 'buffer_size',
      'connect_timeout_sec', 'emit_comm_state', 'comm_report_interval',
      'offline_threshold',
    ]) {
      expect(chirpField(key), `누락된 chirpstack 필드: ${key}`).toBeDefined();
    }
  });
});

// chirpstack 타임스탬프 소스 — opt-in server(수신 시각) 노브.
//
// 백엔드 Transport.Options 키(internal/agent/chirpstack/config.go 의 timestamp_source)와
// 1:1 매핑되어야 하며, 기본값은 현행 동작(업링크 time 필드)과 바이트 동일해야 한다
// (무회귀, REQ-FROZEN-02 / REQ-FROZEN-A).
describe('chirpstack 타임스탬프 소스 스키마', () => {
  const chirpField = (name: string): ConfigField | undefined =>
    getAgentConfigSchema('chirpstack-client')?.fields.find((f) => f.name === name);

  it('timestamp_source 가 select 위젯으로 노출된다', () => {
    const src = chirpField('timestamp_source');
    expect(src).toBeDefined();
    expect(src!.type).toBe('select');
  });

  it('enum 은 uplink/server 이며 기본값은 uplink 이다', () => {
    const src = chirpField('timestamp_source')!;
    expect(src.options).toEqual(['uplink', 'server']);
    expect(src.default).toBe('uplink');
  });

  it('기본값이 현행 동작과 동일하다(업링크 time 필드 — 무회귀)', () => {
    const defaults = getAgentConfigDefaults('chirpstack-client');
    expect(defaults.timestamp_source).toBe('uplink');
  });

  it('설명이 두 값의 의미와 server 선택 이유(시계 오차)를 명시한다', () => {
    const desc = chirpField('timestamp_source')!.description ?? '';
    expect(desc).toContain('uplink');
    expect(desc).toContain('server');
    expect(desc).toContain('시계');
  });
});

describe('sysmetrics 에이전트 스키마', () => {
  /** 등록된 필드 목록 (미등록이면 테스트가 즉시 실패한다) */
  function fields() {
    const schema = getAgentConfigSchema('sysmetrics');
    expect(schema, 'sysmetrics 스키마가 등록되어야 한다').toBeDefined();
    return schema!.fields;
  }

  it('에이전트 타입 목록에 등록되어 있다', () => {
    expect(AGENT_TYPES.map((t) => t.value)).toContain('sysmetrics');
  });

  it('요청된 다섯 지표 토글을 모두 갖는다', () => {
    const names = fields().map((f) => f.name);

    for (const key of [
      'collect_cpu',
      'collect_memory',
      'collect_storage',
      'collect_disk_io',
      'collect_network',
    ]) {
      expect(names, `${key} 토글이 있어야 한다`).toContain(key);
    }
  });

  it('지표 토글은 기본으로 모두 켜져 있다', () => {
    // 만든 직후 아무 값도 안 나오면 설정 화면을 찾아 헤매게 된다.
    const toggles = fields().filter((f) => f.name.startsWith('collect_'));

    expect(toggles).toHaveLength(5);
    for (const f of toggles) {
      expect(f.type, `${f.name} 은 boolean 이어야 한다`).toBe('boolean');
      expect(f.default, `${f.name} 기본값`).toBe(true);
    }
  });

  it('표본 주기 기본값이 백엔드 기본값과 같다', () => {
    // 백엔드 defaultSysMetricsInterval(5s) 과 어긋나면 화면과 실제 동작이 달라진다.
    expect(getAgentConfigDefaults('sysmetrics').interval).toBe('5s');
  });

  it('대상 목록 필드는 필수가 아니다 (비우면 전체)', () => {
    for (const name of ['mountpoints', 'devices', 'interfaces']) {
      const f = fields().find((x) => x.name === name);
      expect(f, `${name} 필드가 있어야 한다`).toBeDefined();
      expect(f?.required ?? false, `${name} 은 선택 항목`).toBe(false);
    }
  });

  it('대상 목록은 호스트 목록에서 고르는 위젯을 쓴다', () => {
    // 손으로 적게 두면 오타 하나로 조용히 아무것도 관측하지 않는다.
    const expected: Record<string, string> = {
      mountpoints: 'mountpoints',
      devices: 'devices',
      interfaces: 'interfaces',
    };

    for (const [name, kind] of Object.entries(expected)) {
      const f = fields().find((x) => x.name === name);
      expect(f?.type, `${name} 위젯 타입`).toBe('sysresource_select');
      expect(f?.resourceKind, `${name} 조회 축`).toBe(kind);
    }
  });

  it('대상 목록에는 기본값이 없다 (기본 = 전체)', () => {
    // 기본값을 넣으면 만들자마자 특정 대상만 관측하게 되어 규약과 어긋난다.
    const defaults = getAgentConfigDefaults('sysmetrics');
    for (const name of ['mountpoints', 'devices', 'interfaces']) {
      expect(defaults[name], `${name} 기본값`).toBeUndefined();
    }
  });

  it.each([
    ['mountpoints', 'collect_storage'],
    ['devices', 'collect_disk_io'],
    ['interfaces', 'collect_network'],
  ])('%s 는 %s 를 켰을 때만 보인다', (target, toggle) => {
    const f = fields().find((x) => x.name === target)!;
    expect(isFieldVisible(f, { [toggle]: true })).toBe(true);
    expect(isFieldVisible(f, { [toggle]: false })).toBe(false);
  });

  it.each([
    ['mountpoints', 'collect_storage'],
    ['devices', 'collect_disk_io'],
    ['interfaces', 'collect_network'],
  ])('%s 는 %s 키가 아예 없는 config 에서도 보인다', (target, toggle) => {
    // 설정 파일·API 로 만든 에이전트에는 키가 없을 수 있다. 백엔드는 없는 키를
    // 기본값 true 로 읽으므로(defaultSysMetricsConfig), 여기서 숨기면 수집은 도는데
    // 대상만 고를 수 없는 상태가 된다.
    const f = fields().find((x) => x.name === target)!;
    expect(isFieldVisible(f, {})).toBe(true);
    void toggle;
  });

  it('대상 선택기는 그 지표 토글 바로 다음에 온다', () => {
    // 떨어져 있으면 어느 토글에 딸린 목록인지 읽히지 않는다.
    const names = fields().map((f) => f.name);
    for (const [toggle, target] of [
      ['collect_storage', 'mountpoints'],
      ['collect_disk_io', 'devices'],
      ['collect_network', 'interfaces'],
    ]) {
      expect(names.indexOf(target!), `${target} 위치`).toBe(names.indexOf(toggle!) + 1);
    }
  });
});
