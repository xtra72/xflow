import yaml from 'js-yaml';

import { generateUUID } from '@/lib/utils/uuid';
import type { TranslationFn } from '@/lib/i18n';

/**
 * 가져오기 대상 항목 하나를 나타낸다.
 */
export interface ImportItem {
  /** 원본 이름 */
  name: string;
  /** 편집 가능한 이름 (사용자가 변경 가능) */
  editedName: string;
  /** 항목 타입 (에이전트 전용) */
  type?: string;
  /** 설명 */
  description?: string;
  /** 플로우 정의 (플로우 전용) */
  definition?: unknown;
  /** 설정 (에이전트 전용) */
  config?: Record<string, unknown>;
  /** 플로우가 참조하는 에이전트 목록 (플로우 전용) */
  requiredAgents?: RequiredAgent[];
  /** 원본 데이터 전체 */
  raw: Record<string, unknown>;
}

/**
 * 유효성 검사 결과.
 */
export interface ValidationResult {
  valid: boolean;
  errors: string[];
  items: ImportItem[];
}

/**
 * 플로우가 참조하는 에이전트 정보.
 */
export interface RequiredAgent {
  name: string;
  type?: string;
  config?: Record<string, unknown>;
  /** 내보내기 시 백엔드가 제거한 비밀 필드 이름 목록 (export 힌트).
   *  가져오기 시 이 필드들의 값을 사용자에게 재입력받는다. */
  sensitiveFields?: string[];
}

/** 에이전트 해결 방식 */
export type AgentResolution = 'create' | 'substitute' | 'skip';

/** 누락 에이전트별 해결 상태 */
export interface AgentResolutionState {
  resolution: AgentResolution;
  substituteAgentName?: string;
}

/** 드롭다운에 표시할 기존 에이전트 옵션 */
export interface ExistingAgentOption {
  name: string;
  type: string;
  status: string;
}

/**
 * 플로우 정의에서 에이전트 이름을 치환한다.
 * 딥 카피 후 remapTable에 따라 agent_ref.agent_name을 교체하고,
 * 치환된 에이전트의 agent_ref.agent_id는 빈 문자열로 초기화한다.
 */
export function remapAgentNames(
  definition: Record<string, unknown>,
  remapTable: Record<string, string>,
): Record<string, unknown> {
  const cloned = structuredClone(definition);

  const nodes = (cloned as Record<string, unknown>).nodes;
  if (!Array.isArray(nodes)) return cloned;

  for (const node of nodes) {
    if (node == null || typeof node !== 'object') continue;
    const n = node as Record<string, unknown>;
    const agentRef = n.agent_ref as Record<string, unknown> | undefined;
    if (
      agentRef &&
      typeof agentRef.agent_name === 'string' &&
      remapTable[agentRef.agent_name]
    ) {
      agentRef.agent_name = remapTable[agentRef.agent_name];
      agentRef.agent_id = '';
    }
  }

  return cloned;
}

/**
 * 파일을 읽어서 JSON 또는 YAML로 파싱한다.
 * 확장자에 따라 자동으로 파서를 선택한다.
 */
export async function parseImportFile(file: File, t: TranslationFn): Promise<unknown> {
  const text = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error(t('import.validate.fileReadError')));
    reader.readAsText(file);
  });

  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';

  if (ext === 'json') {
    return JSON.parse(text);
  }

  if (ext === 'yaml' || ext === 'yml') {
    return yaml.load(text);
  }

  // 확장자를 알 수 없는 경우 JSON을 먼저 시도하고, 실패 시 YAML로 시도
  try {
    return JSON.parse(text);
  } catch {
    return yaml.load(text);
  }
}

/**
 * 플로우 가져오기 데이터를 검증한다.
 * 단일 객체 또는 배열을 모두 지원한다.
 */
export function validateFlowImport(data: unknown, t: TranslationFn): ValidationResult {
  const errors: string[] = [];
  const items: ImportItem[] = [];

  if (data == null || typeof data !== 'object') {
    return { valid: false, errors: [t('import.validate.notObject')], items: [] };
  }

  const arr = Array.isArray(data) ? data : [data];

  for (let i = 0; i < arr.length; i++) {
    const item = arr[i];
    if (item == null || typeof item !== 'object') {
      errors.push(t('import.validate.itemNotObject').replace('{index}', String(i + 1)));
      continue;
    }

    const record = item as Record<string, unknown>;

    if (!record.name || typeof record.name !== 'string') {
      errors.push(t('import.validate.itemNameRequired').replace('{index}', String(i + 1)));
      continue;
    }

    // definition 래핑 포맷 또는 플랫 포맷(nodes/wires 최상위) 모두 지원
    let definition = record.definition;
    if (!definition || typeof definition !== 'object') {
      // 플랫 포맷: nodes가 최상위에 있으면 자동 래핑
      if (Array.isArray(record.nodes)) {
        definition = { nodes: record.nodes, wires: record.wires ?? record.edges ?? [] };
      } else {
        errors.push(
          t('import.validate.itemDefinitionRequired').replace('{index}', String(i + 1)),
        );
        continue;
      }
    }

    items.push({
      name: record.name,
      editedName: record.name,
      description: typeof record.description === 'string' ? record.description : undefined,
      definition,
      requiredAgents: extractRequiredAgents(record),
      raw: record,
    });
  }

  return {
    valid: errors.length === 0 && items.length > 0,
    errors,
    items,
  };
}

/**
 * 에이전트 가져오기 데이터를 검증한다.
 * 단일 객체 또는 배열을 모두 지원한다.
 */
export function validateAgentImport(data: unknown, t: TranslationFn): ValidationResult {
  const errors: string[] = [];
  const items: ImportItem[] = [];

  if (data == null || typeof data !== 'object') {
    return { valid: false, errors: [t('import.validate.notObject')], items: [] };
  }

  const arr = Array.isArray(data) ? data : [data];

  for (let i = 0; i < arr.length; i++) {
    const item = arr[i];
    if (item == null || typeof item !== 'object') {
      errors.push(t('import.validate.itemNotObject').replace('{index}', String(i + 1)));
      continue;
    }

    const record = item as Record<string, unknown>;

    if (!record.name || typeof record.name !== 'string') {
      errors.push(t('import.validate.itemNameRequired').replace('{index}', String(i + 1)));
      continue;
    }

    if (!record.type || typeof record.type !== 'string') {
      errors.push(t('import.validate.itemTypeRequired').replace('{index}', String(i + 1)));
      continue;
    }

    items.push({
      name: record.name,
      editedName: record.name,
      type: record.type,
      config: typeof record.config === 'object' && record.config !== null
        ? (record.config as Record<string, unknown>)
        : undefined,
      raw: record,
    });
  }

  return {
    valid: errors.length === 0 && items.length > 0,
    errors,
    items,
  };
}

/**
 * 플로우 데이터에서 참조된 에이전트 목록을 추출한다.
 * required_agents 필드가 있으면 우선 사용하고,
 * 없으면 definition.nodes[].agent_ref.agent_name 에서 스캔한다.
 */
export function extractRequiredAgents(flowData: Record<string, unknown>): RequiredAgent[] {
  // 1. required_agents 필드 우선
  if (Array.isArray(flowData.required_agents)) {
    return flowData.required_agents
      .filter((a): a is Record<string, unknown> => a != null && typeof a === 'object')
      .filter(a => typeof a.name === 'string' && a.name !== '')
      .map(a => ({
        name: a.name as string,
        type: typeof a.type === 'string' ? a.type : undefined,
        config:
          typeof a.config === 'object' && a.config !== null
            ? (a.config as Record<string, unknown>)
            : undefined,
        // export 힌트: 백엔드가 제거한 비밀 필드 이름 목록.
        sensitiveFields: Array.isArray(a.sensitive_fields)
          ? a.sensitive_fields.filter((f): f is string => typeof f === 'string')
          : undefined,
      }));
  }

  // 2. 폴백: definition.nodes 또는 최상위 nodes 에서 agent_ref.agent_name 스캔
  let nodes: unknown[] | undefined;
  const def = flowData.definition;
  if (def != null && typeof def === 'object') {
    const defNodes = (def as Record<string, unknown>).nodes;
    if (Array.isArray(defNodes)) nodes = defNodes;
  }
  // 플랫 포맷: 최상위 nodes
  if (!nodes && Array.isArray(flowData.nodes)) {
    nodes = flowData.nodes;
  }
  if (!nodes) return [];

  const seen = new Set<string>();
  const result: RequiredAgent[] = [];

  for (const node of nodes) {
    if (node == null || typeof node !== 'object') continue;
    const n = node as Record<string, unknown>;

    const agentRef = n.agent_ref as Record<string, unknown> | undefined;
    if (agentRef && typeof agentRef.agent_name === 'string' && agentRef.agent_name !== '') {
      if (!seen.has(agentRef.agent_name)) {
        seen.add(agentRef.agent_name);
        result.push({ name: agentRef.agent_name });
      }
    }
  }

  return result;
}

/**
 * 백엔드 create flow 요청에서 노드 id 재생성을 트리거하는 플래그 이름.
 *
 * 가져오기 시 프런트엔드가 이미 id 를 재생성하지만(에디터/미리보기 충돌 방지),
 * 백엔드에도 동일 동작을 요청해 belt-and-suspenders 로 동작하게 한다("둘 다" 결정).
 * 백엔드 계약이 다른 키를 쓰면 이 상수만 변경하면 된다.
 */
export const REGENERATE_IDS_FLAG = 'regenerate_ids';

/**
 * 가져온 플로우 정의의 모든 노드 id 와 엣지 id 를 새 UUID 로 재생성한다.
 *
 * 목적: 같은 플로우를 여러 번 가져오더라도 에디터/미리보기 안에서 노드/엣지
 * id 가 충돌하지 않도록 신선한 id 를 부여한다.
 *
 * 동작:
 *  1. 정의를 깊은 복사한 뒤 nodes 각각에 새 UUID 를 부여하고 oldId→newId 맵을 만든다.
 *  2. 엣지(`wires` 우선, 없으면 `edges`) 의 source/target 을 맵으로 재작성한다.
 *  3. 각 엣지 id 도 새 UUID 로 교체한다(파생된 `xy-edge__...` id 를 유지하지 않음).
 *     sourceHandle/targetHandle(포트 이름) 은 그대로 둔다.
 *  4. 노드 내부의 노드 id 참조: 본 스키마에는 노드가 다른 노드 id 를 참조하는
 *     필드가 없다(에이전트는 agent_ref.agent_name 으로 이름 참조). 따라서 추가
 *     재작성 대상이 없으며, agent_ref 등 에이전트 바인딩 정보는 건드리지 않는다.
 *
 * 입력 정의는 변경하지 않고 새 객체를 반환한다(순수 함수).
 */
export function regenerateDefinitionIds(
  definition: Record<string, unknown>,
): Record<string, unknown> {
  const cloned = structuredClone(definition) as Record<string, unknown>;

  const nodes = cloned.nodes;
  if (!Array.isArray(nodes)) {
    // 노드가 없으면 재생성할 대상이 없다.
    return cloned;
  }

  // 1. 노드 id 재생성 + oldId→newId 맵 구성.
  const idMap = new Map<string, string>();
  for (const node of nodes) {
    if (node == null || typeof node !== 'object') continue;
    const n = node as Record<string, unknown>;
    const oldId = typeof n.id === 'string' ? n.id : undefined;
    const newId = generateUUID();
    if (oldId != null) idMap.set(oldId, newId);
    n.id = newId;
  }

  // 2~3. 엣지 source/target 재작성 + 엣지 id 재생성.
  //  내부 직렬화 키는 `edges`(에디터 저장) 또는 `wires`(export) 두 가지가 있다.
  const edgeKey = Array.isArray(cloned.wires)
    ? 'wires'
    : Array.isArray(cloned.edges)
      ? 'edges'
      : undefined;

  if (edgeKey) {
    const edges = cloned[edgeKey] as unknown[];
    for (const edge of edges) {
      if (edge == null || typeof edge !== 'object') continue;
      const e = edge as Record<string, unknown>;
      if (typeof e.source === 'string' && idMap.has(e.source)) {
        e.source = idMap.get(e.source);
      }
      if (typeof e.target === 'string' && idMap.has(e.target)) {
        e.target = idMap.get(e.target);
      }
      // 파생된 xy-edge__ id 를 유지하지 않고 새 UUID 부여.
      // sourceHandle/targetHandle(포트 이름) 은 변경하지 않는다.
      e.id = generateUUID();
    }
  }

  return cloned;
}

/**
 * 생성 예정인 누락 에이전트에 대해 사용자 재입력이 필요한 비밀 필드 이름 목록을 계산한다.
 *
 * 비밀 필드는 두 출처의 합집합이다:
 *  - export 힌트(`agent.sensitiveFields`): 내보내기 시 백엔드가 제거한 키.
 *  - 스키마(`schemaSensitiveFieldNames`): sensitive:true 로 표시된 필드 중,
 *    가져온 config 에 값이 비어 있는(누락/빈 문자열) 키.
 *
 * 반환 순서는 안정적이며 중복은 제거된다.
 */
export function collectSecretFieldNames(
  agent: RequiredAgent,
  schemaSensitiveFieldNames: string[],
): string[] {
  const config = agent.config ?? {};
  const result: string[] = [];
  const seen = new Set<string>();

  const add = (name: string): void => {
    if (!seen.has(name)) {
      seen.add(name);
      result.push(name);
    }
  };

  // 1. export 힌트로 제거된 필드는 항상 재입력 대상.
  for (const name of agent.sensitiveFields ?? []) {
    add(name);
  }

  // 2. 스키마 sensitive 필드 중 값이 비어 있는 것.
  for (const name of schemaSensitiveFieldNames) {
    const v = config[name];
    if (v === undefined || v === null || v === '') {
      add(name);
    }
  }

  return result;
}
