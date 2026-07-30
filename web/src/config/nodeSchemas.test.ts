// computePortsForNode 의 switch 동적 포트 파생 테스트 (SPEC-SWITCH-001).
// 백엔드 SwitchNode.Ports() 와의 정렬을 보증한다:
//   route.name = 포트명 = 와이어 SourcePort = _target_port,
//   default_port 값(고정 'default' 아님)이 출력 포트가 된다.

import { describe, expect, it } from 'vitest';

import {
  computePortsForNode,
  getConfigSchema,
  getDefaultPorts,
  getFlowNodeMode,
  getNodeDescription,
  getNodeIODesc,
  getNodeSchema,
  type PortDef,
} from './nodeSchemas';
import type { ConfigField } from '@/types/node';

/** nodeType 의 config 필드 중 name 으로 하나를 찾는다. */
function findField(nodeType: string, name: string): ConfigField | undefined {
  return getConfigSchema(nodeType)?.fields.find((f) => f.name === name);
}

/** 출력 포트 이름 집합(순서 무시)을 추출한다. */
function outputNames(ports: PortDef[]): string[] {
  return ports.filter((p) => p.direction === 'output').map((p) => p.name);
}

describe('computePortsForNode — switch 동적 포트 (SPEC-SWITCH-001)', () => {
  it('routes 마다 출력 포트를 순서대로 파생한다 (AC-SWITCH-030)', () => {
    const ports = computePortsForNode('switch', {
      routes: [
        { name: 'hot', condition: '$.payload.temp >= 30' },
        { name: 'cold', condition: '$.payload.temp < 10' },
      ],
    });
    // 입력 in 포함
    expect(ports).toContainEqual({ name: 'in', direction: 'input' });
    // 출력은 route.name 순서대로
    expect(outputNames(ports)).toEqual(['hot', 'cold']);
  });

  it('default_port 값을 출력 포트로 포함한다 (고정 "default" 아님) (AC-SWITCH-030/032)', () => {
    const ports = computePortsForNode('switch', {
      routes: [
        { name: 'hot', condition: '...' },
        { name: 'cold', condition: '...' },
      ],
      default_port: 'other',
    });
    const outs = outputNames(ports);
    expect(outs).toContain('hot');
    expect(outs).toContain('cold');
    expect(outs).toContain('other');
    // 고정 'default' 포트는 더 이상 생성되지 않는다.
    expect(outs).not.toContain('default');
    expect(ports).toContainEqual({ name: 'in', direction: 'input' });
  });

  it('default_port 가 비어있으면 default 출력 포트를 만들지 않는다 (AC-SWITCH-021)', () => {
    const ports = computePortsForNode('switch', {
      routes: [{ name: 'hot', condition: '...' }],
      default_port: '',
    });
    expect(outputNames(ports)).toEqual(['hot']);
  });

  it('default_port 미설정이면 라우트 포트만 만든다', () => {
    const ports = computePortsForNode('switch', {
      routes: [{ name: 'hot', condition: '...' }],
    });
    expect(outputNames(ports)).toEqual(['hot']);
  });

  it('routes 가 비면 [in, out] 으로 폴백한다 (AC-SWITCH-031)', () => {
    const ports = computePortsForNode('switch', { routes: [] });
    expect(ports).toEqual([
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
  });

  it('config 가 없거나 routes 미설정이면 [in, out] 으로 폴백한다 (AC-SWITCH-050)', () => {
    expect(computePortsForNode('switch')).toEqual([
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
    expect(computePortsForNode('switch', {})).toEqual([
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
  });

  it('빈 name 라우트는 건너뛰고 유효한 이름이 없으면 폴백한다', () => {
    const ports = computePortsForNode('switch', {
      routes: [
        { name: '', condition: '...' },
        { name: '   ', condition: '...' },
      ],
    });
    expect(ports).toEqual([
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
  });

  it('중복 route.name 은 한 번만 포트로 만든다 (dedupe)', () => {
    const ports = computePortsForNode('switch', {
      routes: [
        { name: 'hot', condition: 'a' },
        { name: 'hot', condition: 'b' },
        { name: 'cold', condition: 'c' },
      ],
    });
    expect(outputNames(ports)).toEqual(['hot', 'cold']);
  });

  it('default_port 가 라우트 이름과 같으면 중복을 제거한다 (dedupe)', () => {
    const ports = computePortsForNode('switch', {
      routes: [{ name: 'hot', condition: 'a' }],
      default_port: 'hot',
    });
    expect(outputNames(ports)).toEqual(['hot']);
  });

  it('default_port 의 앞뒤 공백을 정리하여 포트명으로 사용한다', () => {
    const ports = computePortsForNode('switch', {
      routes: [{ name: 'hot', condition: 'a' }],
      default_port: '  other  ',
    });
    expect(outputNames(ports)).toEqual(['hot', 'other']);
  });
});

// computePortsForNode 의 flow-node 동적 포트 파생 테스트 (SPEC-SUBFLOW-001 그룹 C).
//   핸들 = 참조 플로우의 플로우 레벨 포트 (input_ports → 입력 핸들, output_ports → 출력 핸들).
//   값들은 flow_id 선택 시 참조 플로우 정의에서 비정규화된 에디터 표시 전용 캐시이다.

/** 입력 포트 이름 집합(순서 유지)을 추출한다. */
function inputNames(ports: PortDef[]): string[] {
  return ports.filter((p) => p.direction === 'input').map((p) => p.name);
}

describe('computePortsForNode — flow-node 동적 포트 (SPEC-SUBFLOW-001)', () => {
  it('input_ports → 입력 핸들, output_ports → 출력 핸들로 파생한다 (REQ-SUBFLOW-C02)', () => {
    const ports = computePortsForNode('flow-node', {
      flow_id: 'flow-abc',
      input_ports: ['in1', 'in2'],
      output_ports: ['out1'],
    });
    expect(inputNames(ports)).toEqual(['in1', 'in2']);
    expect(outputNames(ports)).toEqual(['out1']);
    expect(ports).toContainEqual({ name: 'in1', direction: 'input' });
    expect(ports).toContainEqual({ name: 'out1', direction: 'output' });
  });

  it('flow_id 미선택(미해결)이면 핸들 없이 빈 배열을 반환한다 (기본값)', () => {
    expect(computePortsForNode('flow-node')).toEqual([]);
    expect(computePortsForNode('flow-node', {})).toEqual([]);
    expect(computePortsForNode('flow-node', { flow_id: 'flow-abc' })).toEqual([]);
  });

  it('포트가 0개로 해결되면 핸들 없이 빈 배열을 반환한다', () => {
    const ports = computePortsForNode('flow-node', {
      flow_id: 'flow-abc',
      input_ports: [],
      output_ports: [],
    });
    expect(ports).toEqual([]);
  });

  it('입력만/출력만 있는 참조 플로우도 처리한다', () => {
    const inOnly = computePortsForNode('flow-node', {
      flow_id: 'f',
      input_ports: ['trigger'],
    });
    expect(inOnly).toEqual([{ name: 'trigger', direction: 'input' }]);

    const outOnly = computePortsForNode('flow-node', {
      flow_id: 'f',
      output_ports: ['result'],
    });
    expect(outOnly).toEqual([{ name: 'result', direction: 'output' }]);
  });

  it('빈 문자열/공백/중복/비문자열 포트 이름은 안전하게 걸러낸다', () => {
    const ports = computePortsForNode('flow-node', {
      flow_id: 'f',
      input_ports: ['in1', '', '  ', 'in1', '  in2  ', 42, null],
      output_ports: ['out1', 'out1'],
    });
    expect(inputNames(ports)).toEqual(['in1', 'in2']);
    expect(outputNames(ports)).toEqual(['out1']);
  });

  it('input_ports / output_ports 가 배열이 아니면 무시한다', () => {
    const ports = computePortsForNode('flow-node', {
      flow_id: 'f',
      input_ports: 'in1' as unknown as string[],
      output_ports: { a: 1 } as unknown as string[],
    });
    expect(ports).toEqual([]);
  });

  // SPEC-SUBFLOW-002 REQ-SUBFLOW2-P01: 핸들 파생은 mode 와 무관하다.
  it('mode(shared/instance) 와 무관하게 핸들 파생 규칙이 동일하다', () => {
    const shared = computePortsForNode('flow-node', {
      flow_id: 'f',
      mode: 'shared',
      input_ports: ['in1'],
      output_ports: ['out1'],
    });
    const instance = computePortsForNode('flow-node', {
      flow_id: 'f',
      mode: 'instance',
      input_ports: ['in1'],
      output_ports: ['out1'],
    });
    expect(shared).toEqual(instance);
    expect(inputNames(shared)).toEqual(['in1']);
    expect(outputNames(shared)).toEqual(['out1']);
  });
});

// SPEC-SUBFLOW-002 그룹 W (REQ-SUBFLOW2-W01) + M (M02/M03): flow-node mode 토글.
describe('flow-node mode 토글 / 정규화 (SPEC-SUBFLOW-002)', () => {
  it('flow-node 스키마에 mode select 필드(shared/instance, 기본 shared)가 존재한다', () => {
    const field = findField('flow-node', 'mode');
    expect(field).toBeDefined();
    expect(field?.type).toBe('select');
    expect(field?.options).toEqual(['shared', 'instance']);
    expect(field?.default).toBe('shared');
    // 편집 반영 안내(W03)가 설명에 포함되어 있다.
    expect(field?.description).toContain('재시작');
  });

  it('flow_id picker 필드는 보존된다(회귀 0)', () => {
    const field = findField('flow-node', 'flow_id');
    expect(field?.type).toBe('flow_picker');
    expect(field?.required).toBe(true);
  });

  it('getFlowNodeMode 는 미지정/빈/알 수 없는 값을 shared 로 정규화한다 (M02/M03)', () => {
    expect(getFlowNodeMode(undefined)).toBe('shared');
    expect(getFlowNodeMode(null)).toBe('shared');
    expect(getFlowNodeMode('')).toBe('shared');
    expect(getFlowNodeMode('bogus')).toBe('shared');
    expect(getFlowNodeMode('shared')).toBe('shared');
  });

  it('getFlowNodeMode 는 instance 명시만 instance 로 판별한다 (MG02)', () => {
    expect(getFlowNodeMode('instance')).toBe('instance');
  });
});

// 중첩 메타데이터 그룹 emit 토글 (P4 / SPEC nested-metadata-group).
//   - agent / device 그룹은 기본 ON: 스키마 default=true 로 직렬화 시 OFF 만 false 를 보낸다.
//   - HVACR/디바이스 노드는 emit_agent + emit_device 둘 다, 그 외 에이전트 노드(serial/tcp/mqtt/modbus)는 emit_agent 만.
describe('emit_agent / emit_device 그룹 토글 (P4)', () => {
  // emit_agent 가 default ON 으로 노출되는 노드 — HVACR(디바이스) + 그 외 에이전트 IO 노드.
  const AGENT_TOGGLE_NODES = [
    'samsung-hvacr01-status',
    'samsung-hvacr01-control',
    'samsung-hvacr01',
    'lgap',
    'lg-hvacr02-status',
    'lg-hvacr02-control',
    'lg-hvacr02',
    'lg-hvacr01-status',
    'lg-hvacr01',
    'century-hvacr01-status',
    'century-hvacr01',
    'xsfm-status',
    'xsfm-control',
    'xsfm',
    'modbus',
    'modbus-writer',
    'mqtt-subscriber',
    'mqtt-publisher',
    'serial-in',
    'serial-out',
    'tcp-in',
    'tcp-out',
  ] as const;

  // emit_device 까지 노출되는 노드 — HVACR/디바이스 그룹만.
  const DEVICE_TOGGLE_NODES = [
    'samsung-hvacr01-status',
    'samsung-hvacr01-control',
    'samsung-hvacr01',
    'lgap',
    'lg-hvacr02-status',
    'lg-hvacr02-control',
    'lg-hvacr02',
    'lg-hvacr01-status',
    'lg-hvacr01',
    'century-hvacr01-status',
    'century-hvacr01',
    'xsfm-status',
    'xsfm-control',
    'xsfm',
  ] as const;

  // emit_agent 만 노출하고 emit_device 는 노출하지 않는 노드(디바이스 아님).
  const AGENT_ONLY_NODES = [
    'modbus',
    'modbus-writer',
    'mqtt-subscriber',
    'mqtt-publisher',
    'serial-in',
    'serial-out',
    'tcp-in',
    'tcp-out',
  ] as const;

  it.each(AGENT_TOGGLE_NODES)('%s 는 emit_agent 토글을 default=true / advanced 로 노출한다', (nodeType) => {
    const field = findField(nodeType, 'emit_agent');
    expect(field, `${nodeType} 에 emit_agent 필드가 있어야 함`).toBeDefined();
    expect(field?.type).toBe('boolean');
    // 기본 ON: default=true 여야 폼이 미변경 시 아무것도 보내지 않고(absent=ON), OFF 시에만 false 직렬화.
    expect(field?.default).toBe(true);
    expect(field?.advanced).toBe(true);
  });

  it.each(DEVICE_TOGGLE_NODES)('%s 는 emit_device 토글을 default=true / advanced 로 노출한다', (nodeType) => {
    const field = findField(nodeType, 'emit_device');
    expect(field, `${nodeType} 에 emit_device 필드가 있어야 함`).toBeDefined();
    expect(field?.type).toBe('boolean');
    expect(field?.default).toBe(true);
    expect(field?.advanced).toBe(true);
  });

  it.each(AGENT_ONLY_NODES)('%s 는 emit_device 토글을 노출하지 않는다 (디바이스 노드 아님)', (nodeType) => {
    expect(findField(nodeType, 'emit_device')).toBeUndefined();
  });

  it('modbus-poller 는 agent 그룹을 emit 하지 않으므로 emit_agent 토글이 없다', () => {
    expect(findField('modbus-poller', 'emit_agent')).toBeUndefined();
  });
});

// 옛 `_` HVAC 타입이 스키마/포트/설명 조회에서 canonical `-` 키로 해석되는지 검증.
// 저장된 플로우가 옛 `_` 타입을 들고 있어도 에디터가 정상 동작해야 한다.
describe('옛 `_` HVAC 타입의 스키마/메타 정규화 해석', () => {
  // [옛 `_` 타입, 대응 canonical `-` 타입]
  const pairs: Array<[string, string]> = [
    ['samsung_hvacr01_status', 'samsung-hvacr01-status'],
    ['samsung_hvacr01_control', 'samsung-hvacr01-control'],
    ['samsung_hvacr01', 'samsung-hvacr01'],
    ['lg_hvacr01_status', 'lg-hvacr01-status'],
    ['lg_hvacr02', 'lg-hvacr02'],
    ['century_hvacr01_status', 'century-hvacr01-status'],
    ['xsfm_status', 'xsfm-status'],
    ['xsfm_control', 'xsfm-control'],
  ];

  it.each(pairs)('%s 의 configSchema 가 canonical %s 와 동일하게 해석된다', (legacy, canonical) => {
    const legacySchema = getConfigSchema(legacy);
    const canonicalSchema = getConfigSchema(canonical);
    expect(legacySchema).toBeDefined();
    expect(legacySchema).toEqual(canonicalSchema);
  });

  it.each(pairs)('%s 의 NodeTypeSchema 가 canonical %s 와 동일하게 해석된다', (legacy, canonical) => {
    expect(getNodeSchema(legacy)).toEqual(getNodeSchema(canonical));
  });

  it.each(pairs)('%s 의 기본 포트가 canonical %s 와 동일하게 해석된다', (legacy, canonical) => {
    const ports = getDefaultPorts(legacy);
    expect(ports.length).toBeGreaterThan(0);
    expect(ports).toEqual(getDefaultPorts(canonical));
    // computePortsForNode 도 fallback 경로에서 동일하게 정규화된다.
    expect(computePortsForNode(legacy)).toEqual(computePortsForNode(canonical));
  });

  it.each(pairs)('%s 의 설명/입출력 설명이 canonical %s 와 동일하게 해석된다', (legacy, canonical) => {
    expect(getNodeDescription(legacy)).toBe(getNodeDescription(canonical));
    expect(getNodeDescription(legacy)).toBeTruthy();
    expect(getNodeIODesc(legacy)).toEqual(getNodeIODesc(canonical));
  });
});

// XSFM(설비) 노드 3종 스키마 존재/필드/포트 검증 (SPEC-XSFM-001).
// 백엔드 XSFMNodeConfig: agent_ref(필수) + timeout(기본 5s) + emit_metadata.
// push + port drain 모델이라 device_id/poll 필드 없음. 포트는 in/out 만.
describe('XSFM(설비) 노드 3종 스키마 (SPEC-XSFM-001)', () => {
  const XSFM_NODES = ['xsfm-status', 'xsfm-control', 'xsfm'] as const;

  it.each(XSFM_NODES)('%s 스키마가 존재하고 설명/입출력 설명을 갖는다', (nodeType) => {
    const schema = getNodeSchema(nodeType);
    expect(schema, `${nodeType} 스키마가 있어야 함`).toBeDefined();
    expect(getNodeDescription(nodeType)).toBeTruthy();
    const io = getNodeIODesc(nodeType);
    expect(io?.inputDesc).toBeTruthy();
    expect(io?.outputDesc).toBeTruthy();
  });

  it.each(XSFM_NODES)('%s 는 agent_ref agent_select(required, options:[xsfm]) 필드를 노출한다', (nodeType) => {
    const field = findField(nodeType, 'agent_ref');
    expect(field, `${nodeType} 에 agent_ref 필드가 있어야 함`).toBeDefined();
    expect(field?.type).toBe('agent_select');
    expect(field?.required).toBe(true);
    expect(field?.options).toEqual(['xsfm']);
  });

  it.each(XSFM_NODES)('%s 는 timeout 필드를 기본값 5s 로 노출한다', (nodeType) => {
    const field = findField(nodeType, 'timeout');
    expect(field, `${nodeType} 에 timeout 필드가 있어야 함`).toBeDefined();
    expect(field?.type).toBe('string');
    expect(field?.default).toBe('5s');
  });

  it.each(XSFM_NODES)('%s 는 device_id / poll 류 필드를 노출하지 않는다 (push+port 모델)', (nodeType) => {
    expect(findField(nodeType, 'device_id')).toBeUndefined();
    expect(findField(nodeType, 'poll_interval')).toBeUndefined();
    expect(findField(nodeType, 'poll_command')).toBeUndefined();
  });

  it.each(XSFM_NODES)('%s 의 기본 포트는 in/out 만이다 (에러 포트 없음)', (nodeType) => {
    expect(getDefaultPorts(nodeType)).toEqual([
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
  });
});
