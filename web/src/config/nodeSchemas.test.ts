// computePortsForNode 의 switch 동적 포트 파생 테스트 (SPEC-SWITCH-001).
// 백엔드 SwitchNode.Ports() 와의 정렬을 보증한다:
//   route.name = 포트명 = 와이어 SourcePort = _target_port,
//   default_port 값(고정 'default' 아님)이 출력 포트가 된다.

import { describe, expect, it } from 'vitest';

import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  NODE_SCHEMAS,
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

  it('modbus 명령셋 노드(write/read/control)는 emit_agent 토글이 없다 (agent_ref + command_set 만)', () => {
    expect(findField('modbus-write', 'emit_agent')).toBeUndefined();
    expect(findField('modbus-read', 'emit_agent')).toBeUndefined();
    expect(findField('modbus-control', 'emit_agent')).toBeUndefined();
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

// SPEC-HVACR-SYNC-001 M10: 통합 samsung-hvacr01 노드의 mirror-in/mirror-out 포트.
describe('samsung-hvacr01 통합 노드 mirror-message 포트 (SPEC-HVACR-SYNC-001 M10)', () => {
  it('mirror-in(input) 과 mirror-out(output) 포트를 노출한다', () => {
    const ports = getDefaultPorts('samsung-hvacr01');
    expect(ports).toContainEqual({ name: 'mirror-in', direction: 'input' });
    expect(ports).toContainEqual({ name: 'mirror-out', direction: 'output' });
  });

  it('기존 in/out/error 포트도 보존한다 (무회귀)', () => {
    const ports = getDefaultPorts('samsung-hvacr01');
    expect(ports).toContainEqual({ name: 'in', direction: 'input' });
    expect(ports).toContainEqual({ name: 'out', direction: 'output' });
    expect(ports).toContainEqual({ name: 'error', direction: 'error' });
  });

  it('legacy samsung_hvacr01 alias 도 동일한 포트로 해석된다', () => {
    expect(getDefaultPorts('samsung_hvacr01')).toEqual(getDefaultPorts('samsung-hvacr01'));
  });

  it('status/control 분리 노드에는 mirror 포트가 없다 (통합 노드 전용)', () => {
    for (const nodeType of ['samsung-hvacr01-status', 'samsung-hvacr01-control']) {
      const names = getDefaultPorts(nodeType).map((p) => p.name);
      expect(names).not.toContain('mirror-in');
      expect(names).not.toContain('mirror-out');
    }
  });
});

// ---------------------------------------------------------------------------
// NODE_SCHEMAS 완전성 회귀 테스트
//
// nodeSchemas.ts 는 프론트엔드에서만 관리되는 수기 레지스트리이고, 백엔드
// /nodes 엔드포인트는 config 스키마를 내려주지 않는다(=폼을 자동 생성할 수
// 없다). 따라서 Go 레지스트리에 노드 타입이 추가되어도 여기에 항목을 추가하지
// 않으면 설정 폼이 통째로 사라지고, getRequiredFieldErrors 가 빈 배열을 반환해
// 필수 필드 검증(배너 / Apply 가드)까지 조용히 무력화된다.
//
// 기대 타입 목록을 이 파일에 하드코딩하면 그 목록 자체가 똑같이 썩으므로,
// internal/node/registry.go 의 builtins 슬라이스를 테스트 실행 시점에 직접
// 읽어 파싱한다.
// ---------------------------------------------------------------------------

const REGISTRY_GO_PATH = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../internal/node/registry.go',
);

/**
 * internal/node/registry.go 의 `builtins := []struct{...}{...}` 리터럴에서
 * 등록되는 노드 타입 문자열을 추출한다.
 *
 * 각 원소는 `{"type-name", Factory, "category", "description"}` 형태이므로
 * 여는 중괄호 직후의 첫 문자열 리터럴만 취한다. deprecated `_` 별칭은 별도
 * 맵(deprecatedHVACAliases)에 있으므로 여기 포함되지 않는다 — 별칭은
 * normalizeNodeType 으로 canonical 키에 해석되며 기존 테스트가 이미 보증한다.
 */
function parseGoBuiltinNodeTypes(): string[] {
  const src = readFileSync(REGISTRY_GO_PATH, 'utf8');

  const declIdx = src.indexOf('builtins := []struct');
  if (declIdx === -1) {
    throw new Error(
      `registry.go 에서 'builtins := []struct' 선언을 찾지 못했습니다 (${REGISTRY_GO_PATH}). ` +
        '백엔드 레지스트리 구조가 바뀌었다면 이 파서를 갱신해야 합니다.',
    );
  }
  // 필드 선언부를 닫고 리터럴 본문이 시작되는 `}{` 이후부터,
  // 슬라이스를 닫는 첫 `\n\t}\n` 직전까지가 원소 목록이다.
  const bodyStart = src.indexOf('}{', declIdx);
  const bodyEnd = src.indexOf('\n\t}\n', bodyStart);
  if (bodyStart === -1 || bodyEnd === -1) {
    throw new Error(
      `registry.go 의 builtins 리터럴 본문 경계를 찾지 못했습니다 (${REGISTRY_GO_PATH}).`,
    );
  }
  const body = src.slice(bodyStart + 2, bodyEnd);

  const types: string[] = [];
  for (const m of body.matchAll(/^\s*\{"([^"]+)",/gm)) {
    const typeName = m[1];
    if (typeName) types.push(typeName);
  }
  if (types.length === 0) {
    throw new Error(`registry.go 의 builtins 에서 노드 타입을 하나도 파싱하지 못했습니다.`);
  }
  return types;
}

/**
 * NODE_SCHEMAS 에 정적 항목이 없어도 되는 타입.
 *
 * bridge 는 연결된 에이전트 타입에 따라 getBridgeConfigFields 로 스키마를
 * 동적 생성하므로(getNodeSchema 의 bridge 분기) 정적 항목을 두지 않는다.
 */
const DYNAMIC_SCHEMA_TYPES = new Set(['bridge']);

/**
 * defaultPorts 가 비어 있어도 되는 타입.
 *
 * flow-node 는 참조 플로우를 선택하기 전에는 핸들이 결정되지 않으며,
 * computePortsForNode 가 flow_id 해결 후 input_ports/output_ports 로 포트를
 * 파생한다(SPEC-SUBFLOW-001 REQ-SUBFLOW-C02).
 */
const DYNAMIC_PORT_TYPES = new Set(['flow-node']);

describe('NODE_SCHEMAS 완전성 — Go 레지스트리(builtins)와의 정렬', () => {
  const goTypes = parseGoBuiltinNodeTypes();

  it('registry.go 를 실제로 읽어 builtins 타입 목록을 파싱한다 (하드코딩 아님)', () => {
    // 파서가 조용히 빈/축소된 목록으로 퇴화하면 완전성 검사가 무의미해지므로
    // 최소 규모와 대표 타입 존재를 함께 확인한다.
    expect(goTypes.length).toBeGreaterThan(40);
    expect(goTypes).toContain('filter');
    expect(goTypes).toContain('mqtt-subscriber');
    expect(new Set(goTypes).size).toBe(goTypes.length);
  });

  it('모든 빌트인 노드 타입이 NODE_SCHEMAS 키를 갖는다 (bridge 만 예외)', () => {
    const missing = goTypes.filter(
      (t) => !DYNAMIC_SCHEMA_TYPES.has(t) && !(t in NODE_SCHEMAS),
    );
    expect(
      missing,
      `NODE_SCHEMAS 에 항목이 없는 빌트인 노드 타입: ${missing.join(', ')}. ` +
        '설정 폼과 필수 필드 검증이 통째로 누락되므로 nodeSchemas.ts 에 항목을 추가해야 합니다.',
    ).toEqual([]);
  });

  it('chirpstack 3종이 NODE_SCHEMAS 에 등록되어 있다 (회귀 방지)', () => {
    for (const t of ['chirpstack-in', 'chirpstack-control', 'chirpstack-status']) {
      expect(goTypes, `${t} 는 Go 레지스트리에 있어야 함`).toContain(t);
      expect(NODE_SCHEMAS[t], `${t} 스키마가 있어야 함`).toBeDefined();
    }
  });

  it('bridge 는 정적 항목 없이 동적 스키마로 해석된다 (문서화된 예외)', () => {
    expect(NODE_SCHEMAS.bridge).toBeUndefined();
    expect(getNodeSchema('bridge')?.configSchema.fields.length).toBeGreaterThan(0);
    expect(getDefaultPorts('bridge').length).toBeGreaterThan(0);
  });

  it('모든 NODE_SCHEMAS 항목은 포트를 1개 이상 선언한다 (flow-node 만 예외)', () => {
    const portless = Object.entries(NODE_SCHEMAS)
      .filter(([type, schema]) => !DYNAMIC_PORT_TYPES.has(type) && schema.defaultPorts.length === 0)
      .map(([type]) => type);
    expect(
      portless,
      `defaultPorts 가 비어 있는 노드 타입: ${portless.join(', ')}. ` +
        '포트가 없으면 캔버스에서 연결할 수 없습니다.',
    ).toEqual([]);
  });

  it('agent_ref 필드를 노출하는 항목은 모두 required: true 로 표시한다', () => {
    const notRequired = Object.entries(NODE_SCHEMAS)
      .filter(([, schema]) => {
        const field = schema.configSchema.fields.find((f) => f.name === 'agent_ref');
        return field !== undefined && field.required !== true;
      })
      .map(([type]) => type);
    expect(
      notRequired,
      `agent_ref 가 required 로 표시되지 않은 노드 타입: ${notRequired.join(', ')}. ` +
        '백엔드 Configure() 가 빈 agent_ref 를 거부하므로 배포 시점에야 실패합니다.',
    ).toEqual([]);
  });
});
