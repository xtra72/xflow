// computePortsForNode 의 switch 동적 포트 파생 테스트 (SPEC-SWITCH-001).
// 백엔드 SwitchNode.Ports() 와의 정렬을 보증한다:
//   route.name = 포트명 = 와이어 SourcePort = _target_port,
//   default_port 값(고정 'default' 아님)이 출력 포트가 된다.

import { describe, expect, it } from 'vitest';

import { computePortsForNode, getConfigSchema, type PortDef } from './nodeSchemas';
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
});

// 중첩 메타데이터 그룹 emit 토글 (P4 / SPEC nested-metadata-group).
//   - agent / device 그룹은 기본 ON: 스키마 default=true 로 직렬화 시 OFF 만 false 를 보낸다.
//   - HVACR/디바이스 노드는 emit_agent + emit_device 둘 다, 그 외 에이전트 노드(serial/tcp/mqtt/modbus)는 emit_agent 만.
describe('emit_agent / emit_device 그룹 토글 (P4)', () => {
  // emit_agent 가 default ON 으로 노출되는 노드 — HVACR(디바이스) + 그 외 에이전트 IO 노드.
  const AGENT_TOGGLE_NODES = [
    'samsung_hvacr01_status',
    'samsung_hvacr01_control',
    'samsung_hvacr01',
    'lgap',
    'lg_hvacr02_status',
    'lg_hvacr02_control',
    'lg_hvacr02',
    'lg_hvacr01_status',
    'lg_hvacr01',
    'century_hvacr01_status',
    'century_hvacr01',
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
    'samsung_hvacr01_status',
    'samsung_hvacr01_control',
    'samsung_hvacr01',
    'lgap',
    'lg_hvacr02_status',
    'lg_hvacr02_control',
    'lg_hvacr02',
    'lg_hvacr01_status',
    'lg_hvacr01',
    'century_hvacr01_status',
    'century_hvacr01',
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
