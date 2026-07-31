// SPEC-TRIGGER-SCHED-001 M2~M6: 설비 제어 예약 패널 순수 유틸리티.
//
// 각 "규칙(rule)" 은 trigger 노드 config.schedules 의 한 원소이며, 다음을 담는다:
//   - PLAN   : 스케줄 타이밍(type/value/days/times/day) — TriggerScheduleEditor 형식과 동일.
//   - 확장 메타: name/valid_from/valid_to/priority/enabled (백엔드 M1 모델, RD-1/RD-8/RD-9).
//   - payload: xsfm 제어 명령 = TARGET(셀렉터) + ACTION(제어 명령). (RD-2/RD-4/RD-6)
//
// 이 파일은 부수효과 없는 함수만 담아 UI 없이 vitest 로 검증한다(TARGET/ACTION 조립,
// 파싱, PLAN/유효기간 요약, priority 정렬, 규칙↔스케줄 직렬화). dual-write 는 선행
// SPEC-TRIGGER-PANEL-001 의 triggerPanelUtils 를 그대로 재사용한다(신규 저장소 없음).

import type { SerializedSchedule } from '../triggerPanelUtils';

export type { SerializedSchedule };

// ---------------------------------------------------------------------------
// TARGET (셀렉터 조립/파싱) — RD-6: 전체(line) / 그룹(group) / 개별(device)
// ---------------------------------------------------------------------------

/** TARGET 종류. 전체=호선(line), 그룹=group_id/group_name, 개별=device_id/device_name. */
export type TargetKind = 'all' | 'group' | 'device';

/**
 * TARGET 선택 사양.
 * - kind: 전체/그룹/개별
 * - value: 셀렉터 값(호선 코드 / group_id(또는 이름) / device_id(또는 이름))
 * - byName: true 면 이름 셀렉터(group_name/device_name)로 조립(전체=line 은 무관). RD-6.
 */
export interface TargetSpec {
  kind: TargetKind;
  value: string;
  byName?: boolean;
}

/** TARGET 종류 한글 배지 라벨. */
export function targetKindLabel(kind: TargetKind): string {
  switch (kind) {
    case 'all':
      return '전체';
    case 'group':
      return '그룹';
    case 'device':
      return '개별';
  }
}

/**
 * TARGET 사양 → xsfm 셀렉터 payload 조각(단일 종류만 기록해 모호성 방지). RD-6.
 * - 전체 → { line }           (호선 전체, 전역 "all" 셀렉터 미도입)
 * - 그룹 → { group_id }       (byName 이면 { group_name })
 * - 개별 → { device_id }      (byName 이면 { device_name })
 */
export function buildTargetSelector(spec: TargetSpec): Record<string, unknown> {
  const v = spec.value;
  switch (spec.kind) {
    case 'all':
      return { line: v };
    case 'group':
      return spec.byName ? { group_name: v } : { group_id: v };
    case 'device':
      return spec.byName ? { device_name: v } : { device_id: v };
  }
}

/**
 * payload 에서 TARGET 셀렉터를 역파싱한다. 셀렉터 우선순위(device > group > line)를 따르며,
 * 이름 셀렉터(*_name)면 byName=true 로 표시한다. 인식 가능한 셀렉터가 없으면 null.
 */
export function parseTargetSelector(
  payload: Record<string, unknown> | undefined,
): TargetSpec | null {
  if (!payload || typeof payload !== 'object') return null;
  const str = (k: string): string | null =>
    typeof payload[k] === 'string' && (payload[k] as string).length > 0 ? (payload[k] as string) : null;

  const deviceId = str('device_id');
  if (deviceId) return { kind: 'device', value: deviceId };
  const deviceName = str('device_name');
  if (deviceName) return { kind: 'device', value: deviceName, byName: true };

  const groupId = str('group_id');
  if (groupId) return { kind: 'group', value: groupId };
  const groupName = str('group_name');
  if (groupName) return { kind: 'group', value: groupName, byName: true };

  const line = str('line');
  if (line) return { kind: 'all', value: line };

  return null;
}

/**
 * TARGET 설명 문자열(테이블 desc). resolvedName 이 주어지면(에이전트 열거로 이름 매핑) 우선한다.
 * - 전체: resolvedName ?? "<line>호선 전체"
 * - 그룹/개별: resolvedName ?? value
 */
export function formatTargetDescription(spec: TargetSpec, resolvedName?: string): string {
  if (resolvedName && resolvedName.length > 0) return resolvedName;
  if (spec.kind === 'all') return `${spec.value}호선 전체`;
  return spec.value;
}

// ---------------------------------------------------------------------------
// ACTION (제어 명령 조립/파싱) — RD-4: 2축(power/fan_speed)만, 모드 축 없음
// ---------------------------------------------------------------------------

/** 최소/최대 풍량(xsfm control.go: ErrInvalidFanSpeed 1~3). */
export const FAN_SPEED_MIN = 1;
export const FAN_SPEED_MAX = 3;

/**
 * ACTION 선택 사양. null = 해당 축 무변경.
 * - power: true(ON)/false(OFF)/null(무변경)
 * - fanSpeed: 1~3/null(무변경)
 */
export interface ActionSpec {
  power: boolean | null;
  fanSpeed: number | null;
}

/** 풍량 값이 유효 범위(정수 1~3)인지. */
export function isValidFanSpeed(v: number | null): boolean {
  if (v === null) return true; // 무변경은 유효
  return Number.isInteger(v) && v >= FAN_SPEED_MIN && v <= FAN_SPEED_MAX;
}

/** ACTION 사양이 최소 1축 이상 지정되고 풍량 범위가 유효한지(REQ-05-05). */
export function isValidAction(spec: ActionSpec): boolean {
  const hasAxis = spec.power !== null || spec.fanSpeed !== null;
  return hasAxis && isValidFanSpeed(spec.fanSpeed);
}

/**
 * ACTION 사양 → 제어 명령 { command, params }. RD-4.
 * - 전원만 → set_power       { power }
 * - 풍량만 → set_fan_speed   { fan_speed }
 * - 전원+풍량 → set_multiple { power, fan_speed }
 * 유효하지 않으면(무지정 또는 풍량 범위 밖) null 을 반환한다(조립 차단). params 에 mode 키 없음.
 */
export function buildActionCommand(
  spec: ActionSpec,
): { command: string; params: Record<string, unknown> } | null {
  if (!isValidAction(spec)) return null;
  const hasPower = spec.power !== null;
  const hasFan = spec.fanSpeed !== null;
  if (hasPower && hasFan) {
    return { command: 'set_multiple', params: { power: spec.power, fan_speed: spec.fanSpeed } };
  }
  if (hasPower) {
    return { command: 'set_power', params: { power: spec.power } };
  }
  return { command: 'set_fan_speed', params: { fan_speed: spec.fanSpeed } };
}

/**
 * payload.params 에서 ACTION 사양을 역파싱한다(견고성 위해 top-level 도 폴백 확인).
 * command 필드에 의존하지 않고 실제 파라미터로 판정한다. 풍량 범위 밖은 무변경(null)으로 취급.
 */
export function parseActionCommand(payload: Record<string, unknown> | undefined): ActionSpec {
  const params =
    payload && typeof payload.params === 'object' && payload.params !== null
      ? (payload.params as Record<string, unknown>)
      : (payload ?? {});
  const rawPower = params.power;
  const rawFan = params.fan_speed;
  const power = typeof rawPower === 'boolean' ? rawPower : null;
  const fanNum = typeof rawFan === 'number' ? rawFan : null;
  const fanSpeed = fanNum !== null && isValidFanSpeed(fanNum) ? fanNum : null;
  return { power, fanSpeed };
}

/**
 * ACTION 요약 라벨(테이블/모달 미리보기). 모드 표기 없음(RD-4).
 * 예: "전원 ON", "전원 OFF", "풍량 2", "전원 ON · 풍량 2". 무지정 → "-".
 */
export function formatActionLabel(spec: ActionSpec): string {
  const parts: string[] = [];
  if (spec.power !== null) parts.push(spec.power ? '전원 ON' : '전원 OFF');
  if (spec.fanSpeed !== null) parts.push(`풍량 ${spec.fanSpeed}`);
  return parts.length > 0 ? parts.join(' · ') : '-';
}

/**
 * TARGET + ACTION → 규칙 payload(= 제어 명령). RD-2.
 * 형태: { command, <셀렉터 키>, params }. 유효하지 않은 ACTION 이면 null.
 * 예) { command:'set_power', line:'2', params:{ power:false } }
 */
export function buildControlPayload(
  target: TargetSpec,
  action: ActionSpec,
): Record<string, unknown> | null {
  const cmd = buildActionCommand(action);
  if (!cmd) return null;
  const selector = buildTargetSelector(target);
  return { command: cmd.command, ...selector, params: cmd.params };
}

// ---------------------------------------------------------------------------
// PLAN (스케줄 타이밍 요약) + 유효기간 표기
// ---------------------------------------------------------------------------

/** 요일 토큰(백엔드 sun~sat) → 한글 1자. */
const WEEKDAY_KO: Record<string, string> = {
  sun: '일',
  mon: '월',
  tue: '화',
  wed: '수',
  thu: '목',
  fri: '금',
  sat: '토',
};
const WEEKDAY_ORDER = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];

/** 스케줄 타입 → 한글 접두. */
const PLAN_TYPE_KO: Record<string, string> = {
  interval: '주기',
  cron: '크론',
  once: '단발',
  times: '매일',
  weekly: '주간',
  monthly: '월간',
};

function toStringArray(val: unknown): string[] {
  return Array.isArray(val) ? (val as unknown[]).map((v) => String(v ?? '')) : [];
}

/** RFC3339/ISO → "YYYY-MM-DD HH:MM"(로컬). 파싱 실패 시 원본 문자열. */
function formatOnce(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/**
 * PLAN(스케줄) 사람이 읽는 요약. 예:
 *   "주간 · 월화수목금 05:30", "단발 · 2026-07-22 02:00", "주기 · 5s", "월간 · 15일 08:30".
 */
export function formatPlanSummary(schedule: SerializedSchedule | undefined): string {
  if (!schedule || typeof schedule !== 'object') return '-';
  const type = typeof schedule.type === 'string' ? schedule.type : '';
  const prefix = PLAN_TYPE_KO[type] ?? type ?? '?';
  switch (type) {
    case 'interval':
    case 'cron':
      return `${prefix} · ${String(schedule.value ?? '')}`;
    case 'once':
      return `${prefix} · ${formatOnce(String(schedule.value ?? ''))}`;
    case 'times': {
      const times = toStringArray(schedule.value).join(' ');
      return `${prefix} · ${times}`;
    }
    case 'weekly': {
      const days = toStringArray(schedule.days)
        .map((d) => d.toLowerCase())
        .sort((a, b) => WEEKDAY_ORDER.indexOf(a) - WEEKDAY_ORDER.indexOf(b))
        .map((d) => WEEKDAY_KO[d] ?? d)
        .join('');
      const times = toStringArray(schedule.times).join(' ');
      return `${prefix} · ${days} ${times}`.trim();
    }
    case 'monthly': {
      const day = schedule.day;
      const dayLabel = day === 'last' ? '말일' : day === 'first' ? '1일' : `${String(day ?? '')}일`;
      const times = toStringArray(schedule.times).join(' ');
      return `${prefix} · ${dayLabel} ${times}`.trim();
    }
    default:
      return prefix || '-';
  }
}

/**
 * 유효기간 표기(SCHEDULE 컬럼 2번째 줄). RD-8: 빈 valid_to=무기한, 빈 valid_from=하한 무제한.
 * - 둘 다 빈 값     → "무기한"
 * - to 빈 값        → "<from> ~ 무기한"
 * - from 빈 값      → "~ <to>"
 * - 둘 다 지정      → "<from> ~ <to>"
 */
export function formatValidity(validFrom: string, validTo: string): string {
  const from = (validFrom ?? '').trim();
  const to = (validTo ?? '').trim();
  if (!from && !to) return '무기한';
  if (from && !to) return `${from} ~ 무기한`;
  if (!from && to) return `~ ${to}`;
  return `${from} ~ ${to}`;
}

// ---------------------------------------------------------------------------
// 규칙(rule) ↔ 스케줄(schedule) 직렬화
// ---------------------------------------------------------------------------

/** 파싱된 규칙(테이블/모달 소비용). raw 는 원본 스케줄 원소(다른 키 보존용). */
export interface FacilityRule {
  /** 원본 schedules 배열에서의 인덱스(정렬 전 위치). */
  index: number;
  name: string;
  validFrom: string;
  validTo: string;
  priority: number;
  enabled: boolean;
  /** PLAN(타이밍) 스케줄. 확장 메타/payload 를 뺀 타이밍 키만. */
  schedule: SerializedSchedule;
  target: TargetSpec | null;
  action: ActionSpec;
  /** 원본 스케줄 원소(직렬화 시 알 수 없는 키 보존용). */
  raw: SerializedSchedule;
}

/** 타이밍 키만 추출한다(확장 메타/payload 제거) — TriggerScheduleEditor 시드용. */
export function extractTiming(schedule: SerializedSchedule): SerializedSchedule {
  const out: SerializedSchedule = {};
  for (const k of ['type', 'value', 'days', 'times', 'day'] as const) {
    if (k in schedule && schedule[k] !== undefined) out[k] = schedule[k];
  }
  return out;
}

/** enabled 파싱: 부재/nil → true(하위호환), 명시 false 만 비활성. */
function parseEnabled(v: unknown): boolean {
  return v !== false;
}

/** 스케줄 원소 → 규칙(파싱). REQ-SCHED-02-xx. */
export function parseRule(schedule: SerializedSchedule, index: number): FacilityRule {
  const payload =
    schedule.payload && typeof schedule.payload === 'object' && !Array.isArray(schedule.payload)
      ? (schedule.payload as Record<string, unknown>)
      : undefined;
  return {
    index,
    name: typeof schedule.name === 'string' ? schedule.name : '',
    validFrom: typeof schedule.valid_from === 'string' ? schedule.valid_from : '',
    validTo: typeof schedule.valid_to === 'string' ? schedule.valid_to : '',
    priority: typeof schedule.priority === 'number' ? schedule.priority : Number(schedule.priority ?? 0) || 0,
    enabled: parseEnabled(schedule.enabled),
    schedule: extractTiming(schedule),
    target: parseTargetSelector(payload),
    action: parseActionCommand(payload),
    raw: schedule,
  };
}

/** config.schedules(unknown) → 규칙 배열(원본 인덱스 보존). */
export function parseRules(schedules: unknown): FacilityRule[] {
  if (!Array.isArray(schedules)) return [];
  return (schedules as unknown[])
    .map((s, i) => (s && typeof s === 'object' ? parseRule(s as SerializedSchedule, i) : null))
    .filter((r): r is FacilityRule => r !== null);
}

/** priority 오름차순 안정 정렬(동률은 원래 순서 유지). REQ-SCHED-02-06 / AC-4. */
export function sortRulesByPriority(rules: FacilityRule[], descending = false): FacilityRule[] {
  return rules
    .map((r, i) => ({ r, i }))
    .sort((a, b) => {
      const diff = descending ? b.r.priority - a.r.priority : a.r.priority - b.r.priority;
      return diff !== 0 ? diff : a.i - b.i;
    })
    .map((x) => x.r);
}

/** 모달 편집 결과(규칙 초안). */
export interface RuleDraft {
  name: string;
  validFrom: string;
  validTo: string;
  priority: number;
  enabled: boolean;
  /** PLAN(타이밍) 스케줄(TriggerScheduleEditor 출력의 첫 원소). */
  schedule: SerializedSchedule | undefined;
  target: TargetSpec | null;
  action: ActionSpec;
}

/** 규칙 초안 검증(REQ-SCHED-03-05 / AC-8). 필수: name/PLAN/TARGET/ACTION. */
export interface RuleValidation {
  ok: boolean;
  errors: { name?: boolean; plan?: boolean; target?: boolean; action?: boolean };
}

export function validateRuleDraft(draft: RuleDraft): RuleValidation {
  const errors: RuleValidation['errors'] = {};
  if (!draft.name.trim()) errors.name = true;
  if (!draft.schedule || typeof draft.schedule.type !== 'string') errors.plan = true;
  if (!draft.target || !draft.target.value.trim()) errors.target = true;
  if (!draft.action || !isValidAction(draft.action)) errors.action = true;
  return { ok: Object.keys(errors).length === 0, errors };
}

/**
 * 규칙 초안 → 스케줄 원소(직렬화). 확장 메타 + payload(제어 명령)를 타이밍에 병합한다.
 * base(기존 원소)가 있으면 알 수 없는 키를 보존하되 타이밍/메타/payload 는 초안 값으로 덮어쓴다.
 * 초안이 유효하지 않으면(payload 조립 불가 등) null.
 */
export function buildScheduleFromDraft(
  draft: RuleDraft,
  base?: SerializedSchedule,
): SerializedSchedule | null {
  if (!draft.schedule || !draft.target) return null;
  const payload = buildControlPayload(draft.target, draft.action);
  if (!payload) return null;
  const timing = extractTiming(draft.schedule);
  return {
    ...(base ?? {}),
    ...timing,
    name: draft.name.trim(),
    valid_from: draft.validFrom.trim(),
    valid_to: draft.validTo.trim(),
    priority: draft.priority,
    enabled: draft.enabled,
    payload,
  };
}

/** 규칙 → 모달 초안(편집 프리필). target 이 없으면 기본 device 빈 값. */
export function ruleToDraft(rule: FacilityRule): RuleDraft {
  return {
    name: rule.name,
    validFrom: rule.validFrom,
    validTo: rule.validTo,
    priority: rule.priority,
    enabled: rule.enabled,
    schedule: rule.schedule,
    target: rule.target ?? { kind: 'device', value: '' },
    action: rule.action,
  };
}

/** 빈(신규) 규칙 초안. */
export function emptyDraft(): RuleDraft {
  return {
    name: '',
    validFrom: '',
    validTo: '',
    priority: 0,
    enabled: true,
    schedule: { type: 'weekly', days: [], times: [] },
    target: { kind: 'all', value: '' },
    action: { power: true, fanSpeed: null },
  };
}

/** 스케줄 배열의 index 원소 enabled 를 반전한 새 배열(STATE 토글). */
export function toggleEnabledAt(schedules: SerializedSchedule[], index: number): SerializedSchedule[] {
  return schedules.map((s, i) => {
    if (i !== index) return s;
    const cur = parseEnabled(s.enabled);
    return { ...s, enabled: !cur };
  });
}

/** 스케줄 배열의 index 원소를 교체(편집)하거나, index<0 이면 말미에 추가(신규). */
export function upsertScheduleAt(
  schedules: SerializedSchedule[],
  index: number,
  next: SerializedSchedule,
): SerializedSchedule[] {
  if (index < 0 || index >= schedules.length) return [...schedules, next];
  return schedules.map((s, i) => (i === index ? next : s));
}
