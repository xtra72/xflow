import yaml from 'js-yaml';

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
export async function parseImportFile(file: File): Promise<unknown> {
  const text = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error('파일을 읽을 수 없습니다.'));
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
export function validateFlowImport(data: unknown): ValidationResult {
  const errors: string[] = [];
  const items: ImportItem[] = [];

  if (data == null || typeof data !== 'object') {
    return { valid: false, errors: ['유효한 JSON/YAML 객체가 아닙니다.'], items: [] };
  }

  const arr = Array.isArray(data) ? data : [data];

  for (let i = 0; i < arr.length; i++) {
    const item = arr[i];
    if (item == null || typeof item !== 'object') {
      errors.push(`항목 ${i + 1}: 객체가 아닙니다.`);
      continue;
    }

    const record = item as Record<string, unknown>;

    if (!record.name || typeof record.name !== 'string') {
      errors.push(`항목 ${i + 1}: name 필드가 필요합니다.`);
      continue;
    }

    // definition 래핑 포맷 또는 플랫 포맷(nodes/wires 최상위) 모두 지원
    let definition = record.definition;
    if (!definition || typeof definition !== 'object') {
      // 플랫 포맷: nodes가 최상위에 있으면 자동 래핑
      if (Array.isArray(record.nodes)) {
        definition = { nodes: record.nodes, wires: record.wires ?? record.edges ?? [] };
      } else {
        errors.push(`항목 ${i + 1}: definition 또는 nodes 필드가 필요합니다.`);
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
export function validateAgentImport(data: unknown): ValidationResult {
  const errors: string[] = [];
  const items: ImportItem[] = [];

  if (data == null || typeof data !== 'object') {
    return { valid: false, errors: ['유효한 JSON/YAML 객체가 아닙니다.'], items: [] };
  }

  const arr = Array.isArray(data) ? data : [data];

  for (let i = 0; i < arr.length; i++) {
    const item = arr[i];
    if (item == null || typeof item !== 'object') {
      errors.push(`항목 ${i + 1}: 객체가 아닙니다.`);
      continue;
    }

    const record = item as Record<string, unknown>;

    if (!record.name || typeof record.name !== 'string') {
      errors.push(`항목 ${i + 1}: name 필드가 필요합니다.`);
      continue;
    }

    if (!record.type || typeof record.type !== 'string') {
      errors.push(`항목 ${i + 1}: type 필드가 필요합니다.`);
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
