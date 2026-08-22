// 레지스터 리맵 에디터의 순수 모델 계층 — 상수 / 행 타입 / 방출 타입 /
// 변환·방출 유틸 / 템플릿 구체화 / 일괄등록 파서.
//
// 컴포넌트 파일(RegisterRemapEditor.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

// ---- 상수 ----

export const AREA_OPTIONS = [
  'coils',
  'discrete_inputs',
  'holding_registers',
  'input_registers',
] as const;

export type AreaKey = (typeof AREA_OPTIONS)[number];

export const DEFAULT_AREA: AreaKey = 'holding_registers';

/** 영역 → 1글자 약어(목록 표시용). */
export const AREA_ABBREV: Record<string, string> = {
  coils: 'C',
  discrete_inputs: 'D',
  holding_registers: 'H',
  input_registers: 'I',
};

/** 약어(대소문자 무관) → 영역. */
export const ABBREV_TO_AREA: Record<string, AreaKey> = {
  c: 'coils',
  d: 'discrete_inputs',
  h: 'holding_registers',
  i: 'input_registers',
};

/** 약어 또는 전체 이름 문자열 → 영역(없으면 null). */
export function resolveArea(cell: string): AreaKey | null {
  const s = cell.trim();
  if ((AREA_OPTIONS as readonly string[]).includes(s)) return s as AreaKey;
  const ab = ABBREV_TO_AREA[s.toLowerCase()];
  return ab ?? null;
}

export function abbrev(area: string): string {
  return AREA_ABBREV[area] ?? area;
}

// ---- 내부 행 타입 (rules: 구체/절대) ----

export interface TargetRow {
  key: string;
  targetUnitId: number;
  targetArea: string; // '' = keep source_area
  targetAddress: number;
}

export interface RuleRow {
  key: string;
  sourceUnitId: string; // '' = omit
  sourceArea: string;
  sourceAddress: number;
  count: number;
  targets: TargetRow[]; // ≥1
}

// ---- 내부 행 타입 (templates: 패턴/상대) ----

export interface PatternTargetRow {
  key: string;
  targetArea: string; // '' = keep
  targetOffset: number;
  targetUnitOffset: string; // '' = omit(default 0)
}

export interface PatternRuleRow {
  key: string;
  sourceUnitId: string; // '' = omit
  sourceArea: string;
  sourceOffset: number;
  count: number;
  targets: PatternTargetRow[]; // ≥1
}

export interface TemplateDef {
  key: string;
  name: string;
  rules: PatternRuleRow[];
}

// ---- 방출 타입 ----

export interface EmittedTarget {
  target_unit_id: number;
  target_area?: string;
  target_address: number;
}
export interface EmittedRule {
  source_unit_id?: number;
  source_area: string;
  source_address: number;
  count: number;
  targets: EmittedTarget[];
}
export interface EmittedPatternTarget {
  target_area?: string;
  target_offset: number;
  target_unit_offset?: number;
}
export interface EmittedPatternRule {
  source_unit_id?: number;
  source_area: string;
  source_offset: number;
  count: number;
  targets: EmittedPatternTarget[];
}
export interface EmittedTemplate {
  name: string;
  rules: EmittedPatternRule[];
}

// ---- 변환 유틸 ----

let keyCounter = 0;
export function nextKey(prefix: string): string {
  return `${prefix}-${++keyCounter}-${Date.now()}`;
}

export function asObject(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}
export function asString(v: unknown): string {
  return typeof v === 'string' ? v : '';
}
export function numOr(v: unknown, def: number): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return def;
}
export function optNumToText(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? String(v) : '';
}
export function isNonNegInt(s: string): boolean {
  return /^\d+$/.test(s);
}
export function toArray(value: unknown): unknown[] {
  let source: unknown = value;
  if (typeof value === 'string') {
    if (value.trim() === '') return [];
    try {
      source = JSON.parse(value);
    } catch {
      return [];
    }
  }
  return Array.isArray(source) ? source : [];
}

// --- rules 파싱/방출 ---

export function toTargetRow(item: unknown): TargetRow {
  const o = asObject(item);
  return {
    key: nextKey('tgt'),
    targetUnitId: numOr(o.target_unit_id, 1),
    targetArea: asString(o.target_area),
    targetAddress: numOr(o.target_address, 0),
  };
}
export function newTargetRow(): TargetRow {
  return { key: nextKey('tgt'), targetUnitId: 1, targetArea: '', targetAddress: 0 };
}
export function toRuleRow(item: unknown): RuleRow {
  const o = asObject(item);
  let targets: TargetRow[];
  if (Array.isArray(o.targets) && o.targets.length > 0) {
    targets = o.targets.map(toTargetRow);
  } else if (
    o.target_unit_id !== undefined ||
    o.target_area !== undefined ||
    o.target_address !== undefined
  ) {
    targets = [
      {
        key: nextKey('tgt'),
        targetUnitId: numOr(o.target_unit_id, 1),
        targetArea: asString(o.target_area),
        targetAddress: numOr(o.target_address, 0),
      },
    ];
  } else {
    targets = [newTargetRow()];
  }
  return {
    key: nextKey('rule'),
    sourceUnitId: optNumToText(o.source_unit_id),
    sourceArea: asString(o.source_area) || DEFAULT_AREA,
    sourceAddress: numOr(o.source_address, 0),
    count: numOr(o.count, 1),
    targets,
  };
}
export function toRuleRows(value: unknown): RuleRow[] {
  return toArray(value).map(toRuleRow);
}
export function newRuleRow(): RuleRow {
  return {
    key: nextKey('rule'),
    sourceUnitId: '',
    sourceArea: DEFAULT_AREA,
    sourceAddress: 0,
    count: 1,
    targets: [newTargetRow()],
  };
}
export function toEmitTarget(t: TargetRow): EmittedTarget {
  const out: EmittedTarget = {
    target_unit_id: t.targetUnitId,
    target_address: t.targetAddress,
  };
  if (t.targetArea.trim() !== '') out.target_area = t.targetArea;
  return out;
}
export function toEmitRule(r: RuleRow): EmittedRule {
  const out: EmittedRule = {
    source_area: r.sourceArea,
    source_address: r.sourceAddress,
    count: r.count,
    targets: r.targets.map(toEmitTarget),
  };
  if (r.sourceUnitId.trim() !== '') out.source_unit_id = numOr(r.sourceUnitId, 0);
  return out;
}

// --- templates 파싱/방출 ---

export function toPatternTargetRow(item: unknown): PatternTargetRow {
  const o = asObject(item);
  return {
    key: nextKey('ptgt'),
    targetArea: asString(o.target_area),
    targetOffset: numOr(o.target_offset, 0),
    targetUnitOffset: optNumToText(o.target_unit_offset),
  };
}
export function newPatternTargetRow(): PatternTargetRow {
  return { key: nextKey('ptgt'), targetArea: '', targetOffset: 0, targetUnitOffset: '' };
}
export function toPatternRuleRow(item: unknown): PatternRuleRow {
  const o = asObject(item);
  const targets =
    Array.isArray(o.targets) && o.targets.length > 0
      ? o.targets.map(toPatternTargetRow)
      : [newPatternTargetRow()];
  return {
    key: nextKey('prule'),
    sourceUnitId: optNumToText(o.source_unit_id),
    sourceArea: asString(o.source_area) || DEFAULT_AREA,
    sourceOffset: numOr(o.source_offset, 0),
    count: numOr(o.count, 1),
    targets,
  };
}
export function newPatternRuleRow(): PatternRuleRow {
  return {
    key: nextKey('prule'),
    sourceUnitId: '',
    sourceArea: DEFAULT_AREA,
    sourceOffset: 0,
    count: 1,
    targets: [newPatternTargetRow()],
  };
}
export function toTemplateDef(item: unknown): TemplateDef {
  const o = asObject(item);
  return {
    key: nextKey('tpl'),
    name: asString(o.name),
    rules: Array.isArray(o.rules) ? o.rules.map(toPatternRuleRow) : [],
  };
}
export function toTemplateDefs(value: unknown): TemplateDef[] {
  return toArray(value).map(toTemplateDef);
}
export function newTemplateDef(name: string): TemplateDef {
  return { key: nextKey('tpl'), name, rules: [newPatternRuleRow()] };
}
export function toEmitPatternTarget(t: PatternTargetRow): EmittedPatternTarget {
  const out: EmittedPatternTarget = { target_offset: t.targetOffset };
  if (t.targetArea.trim() !== '') out.target_area = t.targetArea;
  if (t.targetUnitOffset.trim() !== '') out.target_unit_offset = numOr(t.targetUnitOffset, 0);
  return out;
}
export function toEmitPatternRule(r: PatternRuleRow): EmittedPatternRule {
  const out: EmittedPatternRule = {
    source_area: r.sourceArea,
    source_offset: r.sourceOffset,
    count: r.count,
    targets: r.targets.map(toEmitPatternTarget),
  };
  if (r.sourceUnitId.trim() !== '') out.source_unit_id = numOr(r.sourceUnitId, 0);
  return out;
}
export function toEmitTemplate(tpl: TemplateDef): EmittedTemplate {
  return { name: tpl.name, rules: tpl.rules.map(toEmitPatternRule) };
}

// --- apply: 템플릿 패턴 → 구체 rules (순수 함수, 단위 테스트 대상) ---

/**
 * 템플릿 패턴을 start/device_id 로 구체 rules 로 materialize 한다.
 *   source_address = start + source_offset
 *   target_address = start + target_offset
 *   target_unit_id = device_id + (target_unit_offset||0)
 */
export function materializeTemplate(
  tpl: EmittedTemplate,
  start: number,
  deviceId: number,
): EmittedRule[] {
  return tpl.rules.map((pr) => {
    const out: EmittedRule = {
      source_area: pr.source_area,
      source_address: start + pr.source_offset,
      count: pr.count,
      targets: pr.targets.map((pt) => {
        const t: EmittedTarget = {
          target_unit_id: deviceId + (pt.target_unit_offset ?? 0),
          target_address: start + pt.target_offset,
        };
        if (pt.target_area) t.target_area = pt.target_area;
        return t;
      }),
    };
    if (pr.source_unit_id !== undefined) out.source_unit_id = pr.source_unit_id;
    return out;
  });
}

// --- 일괄등록 파서 (순수 함수, 단위 테스트 대상) ---

export interface BulkParseError {
  line: number;
  code: string;
}
export interface BulkParseResult {
  rules: EmittedRule[];
  errors: BulkParseError[];
}

export function splitCells(line: string): string[] {
  const parts = line.includes('\t') ? line.split('\t') : line.split(',');
  return parts.map((c) => c.trim());
}

/**
 * 붙여넣기 텍스트 → rules(각 1 타깃). 순수 함수.
 * 컬럼: source_area, source_address, count, target_unit_id, target_area, target_address [, source_unit_id]
 *  - area 는 약어(C/D/H/I) 또는 전체 이름 모두 허용. target_area 빈 셀 → 생략(keep).
 *  - 6 또는 7 컬럼(7번째 = source_unit_id, 선택). 그 외 → wrongColumnCount.
 *  - 빈 줄 무시. 첫 non-empty 줄의 첫 셀이 영역으로 해석 안 되면 헤더로 보고 무시.
 *  - 오류가 있어도 유효 rule 은 담아 반환(호출부 block-on-error).
 */
export function parseBulkRules(text: string): BulkParseResult {
  const rules: EmittedRule[] = [];
  const errors: BulkParseError[] = [];
  const lines = text.split(/\r?\n/);
  let firstSeen = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!.trim();
    if (line === '') continue;
    const cells = splitCells(line);

    if (!firstSeen) {
      firstSeen = true;
      if (resolveArea(cells[0] ?? '') === null) continue; // 헤더 스킵
    }

    const lineNo = i + 1;
    const n = cells.length;
    if (n !== 6 && n !== 7) {
      errors.push({ line: lineNo, code: 'wrongColumnCount' });
      continue;
    }
    const srcArea = resolveArea(cells[0] ?? '');
    if (!srcArea) {
      errors.push({ line: lineNo, code: 'invalidSourceArea' });
      continue;
    }
    const srcAddr = cells[1] ?? '';
    const countCell = cells[2] ?? '';
    const tgtUnit = cells[3] ?? '';
    const tgtAreaCell = cells[4] ?? '';
    const tgtAddr = cells[5] ?? '';
    const srcUnitCell = cells[6] ?? '';

    if (!isNonNegInt(srcAddr)) {
      errors.push({ line: lineNo, code: 'invalidAddress' });
      continue;
    }
    if (!isNonNegInt(countCell) || Number(countCell) < 1) {
      errors.push({ line: lineNo, code: 'invalidCount' });
      continue;
    }
    if (!isNonNegInt(tgtUnit)) {
      errors.push({ line: lineNo, code: 'invalidTargetUnitId' });
      continue;
    }
    let tgtArea: AreaKey | null = null;
    if (tgtAreaCell !== '') {
      tgtArea = resolveArea(tgtAreaCell);
      if (!tgtArea) {
        errors.push({ line: lineNo, code: 'invalidTargetArea' });
        continue;
      }
    }
    if (!isNonNegInt(tgtAddr)) {
      errors.push({ line: lineNo, code: 'invalidTargetAddress' });
      continue;
    }
    if (n === 7 && srcUnitCell !== '' && !isNonNegInt(srcUnitCell)) {
      errors.push({ line: lineNo, code: 'invalidSourceUnitId' });
      continue;
    }

    const target: EmittedTarget = {
      target_unit_id: Number(tgtUnit),
      target_address: Number(tgtAddr),
    };
    if (tgtArea) target.target_area = tgtArea;
    const rule: EmittedRule = {
      source_area: srcArea,
      source_address: Number(srcAddr),
      count: Number(countCell),
      targets: [target],
    };
    if (n === 7 && srcUnitCell !== '') rule.source_unit_id = Number(srcUnitCell);
    rules.push(rule);
  }
  return { rules, errors };
}
