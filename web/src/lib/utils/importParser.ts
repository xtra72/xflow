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

    if (!record.definition || typeof record.definition !== 'object') {
      errors.push(`항목 ${i + 1}: definition 필드가 필요합니다.`);
      continue;
    }

    items.push({
      name: record.name,
      editedName: record.name,
      description: typeof record.description === 'string' ? record.description : undefined,
      definition: record.definition,
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
