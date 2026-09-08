// 트리거 노드 스케줄 에디터 컴포넌트.
// interval / cron / once / times / weekly / monthly 6종 스케줄을 행 단위로 편집한다.
// 각 타입별로 전용 입력 위젯을 제공하고, 잘못된 형식은 인라인 검증 메시지로 표시한다.
// 각 스케줄 항목은 선택적으로 자체 페이로드를 가질 수 있으며(none | set 토글),
// 미입력 시 노드 레벨 페이로드로 폴백한다 (v1.2.0, REQ-NODE-004-03-04).
// v1.3.0: static/template 이원 구조를 제거하고 단일 JSON 페이로드 에디터로 통합.
// 저장은 항상 `payload` 키로 정규화하되, 로드 시 하위호환으로 `payload_template`(map)도 수용한다.
//
// 저장 형식은 백엔드(trigger.go)와 호환되는 배열이다:
//   [{ type: 'interval', value: '5s' },
//    { type: 'cron',     value: '0 */5 * * *' },
//    { type: 'once',     value: '2026-04-15T10:00:00Z' },
//    { type: 'times',    value: ['09:00', '12:00'] },
//    { type: 'weekly',   days: ['mon','wed','fri'], times: ['09:00','18:00'] },
//    { type: 'monthly',  day: 15, times: ['08:30'] },
//    { type: 'monthly',  day: 'last', times: ['23:59'], payload: { report: 'month-end' } }]

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

type ScheduleType = 'interval' | 'cron' | 'once' | 'times' | 'weekly' | 'monthly';

/** monthly.day 값: 정수 1~31 | "first" | "last" */
type MonthlyDay = number | 'first' | 'last';

interface Schedule {
  /** 내부 key (React 렌더 + 추적용, 저장 시 제외) */
  key: string;
  type: ScheduleType;
  /** interval/cron/once 는 string, times 는 string[] */
  value: string | string[];
  /** weekly 전용: 요일 토큰 (sun~sat, 소문자) */
  days?: string[];
  /** monthly 전용: 일자 (1~31 | "first" | "last") */
  day?: MonthlyDay;
  /** weekly/monthly 전용: 시각 배열 ("HH:MM") */
  times?: string[];
  /** 이 스케줄이 자체 페이로드 오버라이드를 갖는지 여부 (UI 전용, 저장 시 제외).
   *  false 면 노드 레벨(또는 기본) 페이로드로 폴백한다 (v1.3.0). */
  payloadSet: boolean;
  /** 단일 통합 페이로드 값 (payloadSet === true 일 때 emit). map/스칼라/배열 허용.
   *  백엔드는 통합 템플릿 엔진($.<var> 치환 + $$ 이스케이프)으로 평가한다. */
  payload?: unknown;
}

interface TriggerScheduleEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

// ---------------------------------------------------------------------------
// 프리셋
// ---------------------------------------------------------------------------

const INTERVAL_PRESETS = ['1s', '5s', '30s', '1m', '5m', '15m', '1h'];

// 라벨은 i18n 키만 보관하고, 렌더 시 컴포넌트 내부에서 t(labelKey) 로 변환한다.
const CRON_PRESETS: { labelKey: string; value: string }[] = [
  { labelKey: 'property.schedule.cronEveryMinute', value: '0 * * * * *' },
  { labelKey: 'property.schedule.cronEvery5Minutes', value: '0 */5 * * * *' },
  { labelKey: 'property.schedule.cronHourly', value: '0 0 * * * *' },
  { labelKey: 'property.schedule.cronDailyMidnight', value: '0 0 0 * * *' },
  { labelKey: 'property.schedule.cronDaily9am', value: '0 0 9 * * *' },
  { labelKey: 'property.schedule.cronWeekday9am', value: '0 0 9 * * 1-5' },
];

// 요일 토큰 (일~토). 백엔드는 sun~sat 소문자 3자 약어를 소비한다.
const WEEKDAY_TOKENS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'] as const;

// weekly 요일 프리셋 (라벨 i18n 키 + 요일 토큰 집합).
const WEEKLY_PRESETS: { labelKey: string; days: string[] }[] = [
  { labelKey: 'property.schedule.presetWeekdays', days: ['mon', 'tue', 'wed', 'thu', 'fri'] },
  { labelKey: 'property.schedule.presetWeekend', days: ['sun', 'sat'] },
  { labelKey: 'property.schedule.presetEveryday', days: [...WEEKDAY_TOKENS] },
];

// monthly 일자 프리셋.
const MONTHLY_PRESETS: { labelKey: string; day: MonthlyDay }[] = [
  { labelKey: 'property.schedule.presetDay1', day: 1 },
  { labelKey: 'property.schedule.presetDay15', day: 15 },
  { labelKey: 'property.schedule.presetLastDay', day: 'last' },
];

// ---------------------------------------------------------------------------
// 유틸리티
// ---------------------------------------------------------------------------

let keyCounter = 0;
function nextKey(): string {
  return `sch-${++keyCounter}-${Date.now()}`;
}

/** unknown → string[] ("HH:MM" 등 문자열 배열) */
function toStringArray(val: unknown): string[] {
  return Array.isArray(val) ? (val as unknown[]).map((v) => String(v ?? '')) : [];
}

/** unknown → MonthlyDay (정수 1~31 | "first" | "last"). 파싱 실패 시 1. */
function parseMonthlyDay(val: unknown): MonthlyDay {
  if (val === 'first' || val === 'last') return val;
  const n = typeof val === 'number' ? val : Number(val);
  if (Number.isInteger(n) && n >= 1 && n <= 31) return n;
  return 1;
}

/**
 * raw 객체에서 스케줄별 페이로드를 해석한다 (v1.3.0 단일 통합 에디터).
 * 하위호환: payload(any)와 payload_template(map)를 모두 수용하여 단일 값으로 병합한다.
 * - 둘 다 map이면 병합(payload가 우선), 하나만 있으면 그 값을 사용.
 * - 아무 것도 없으면 payloadSet=false (노드 레벨 폴백).
 */
function parsePayload(obj: Record<string, unknown>): Pick<Schedule, 'payloadSet' | 'payload'> {
  const tmpl = obj.payload_template;
  const hasTmpl =
    tmpl != null && typeof tmpl === 'object' && !Array.isArray(tmpl) && Object.keys(tmpl).length > 0;
  // payload 키 존재 여부로 판정 (0/false/"" 등 falsy 값도 유효한 오버라이드).
  const hasPayload = 'payload' in obj && obj.payload !== undefined && obj.payload !== null;

  if (hasPayload && hasTmpl && isPlainObject(obj.payload)) {
    return {
      payloadSet: true,
      payload: { ...(tmpl as Record<string, unknown>), ...(obj.payload as Record<string, unknown>) },
    };
  }
  if (hasPayload) return { payloadSet: true, payload: obj.payload };
  if (hasTmpl) return { payloadSet: true, payload: tmpl };
  return { payloadSet: false };
}

/** map 여부(플레인 오브젝트, 배열/null 제외). */
function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v != null && typeof v === 'object' && !Array.isArray(v);
}

/** 페이로드가 비어있지 않은지 판정(미설정/빈 문자열/빈 오브젝트 → 폴백). */
function isNonEmptyPayload(payload: unknown): boolean {
  if (payload === undefined || payload === null) return false;
  if (typeof payload === 'string') return payload.trim() !== '';
  if (isPlainObject(payload)) return Object.keys(payload).length > 0;
  return true; // 숫자/불리언/배열 등은 값이 있는 것으로 간주.
}

/** 외부 value(unknown) → Schedule[] 변환 */
function parseSchedules(val: unknown): Schedule[] {
  if (!Array.isArray(val)) return [];
  const result: Schedule[] = [];
  for (const raw of val as unknown[]) {
    if (!raw || typeof raw !== 'object') continue;
    const obj = raw as Record<string, unknown>;
    const type = obj.type;
    const payloadFields = parsePayload(obj);
    switch (type) {
      case 'interval':
      case 'cron':
      case 'once':
        result.push({ key: nextKey(), type, value: String(obj.value ?? ''), ...payloadFields });
        break;
      case 'times':
        result.push({ key: nextKey(), type, value: toStringArray(obj.value), ...payloadFields });
        break;
      case 'weekly':
        result.push({
          key: nextKey(),
          type,
          value: '',
          days: toStringArray(obj.days).map((d) => d.toLowerCase()),
          times: toStringArray(obj.times),
          ...payloadFields,
        });
        break;
      case 'monthly':
        result.push({
          key: nextKey(),
          type,
          value: '',
          day: parseMonthlyDay(obj.day),
          times: toStringArray(obj.times),
          ...payloadFields,
        });
        break;
      default:
        continue;
    }
  }
  return result;
}

/** Schedule[] → 저장용 배열 (key/UI 전용 필드 제거). */
function serialize(schedules: Schedule[]): unknown[] {
  return schedules.map((s) => {
    const base: Record<string, unknown> = { type: s.type };
    switch (s.type) {
      case 'weekly':
        // 요일 토큰은 소문자로 정규화하여 emit.
        base.days = (s.days ?? []).map((d) => d.toLowerCase());
        base.times = s.times ?? [];
        break;
      case 'monthly':
        base.day = s.day ?? 1;
        base.times = s.times ?? [];
        break;
      default:
        base.value = s.value;
        break;
    }
    // 스케줄별 페이로드(v1.3.0): 설정됨이고 비어있지 않을 때만 `payload`로 정규화 emit.
    // 미설정/빈 값이면 어떤 키도 emit 하지 않는다(백엔드가 노드 레벨→기본으로 폴백).
    // payload_template 은 더 이상 emit 하지 않는다(로드 시 하위호환 수용).
    if (s.payloadSet && isNonEmptyPayload(s.payload)) {
      base.payload = s.payload;
    }
    return base;
  });
}

/** 새 스케줄의 기본값 */
function defaultScheduleForType(type: ScheduleType): Schedule {
  const common = { key: nextKey(), payloadSet: false };
  switch (type) {
    case 'interval':
      return { ...common, type, value: '5s' };
    case 'cron':
      return { ...common, type, value: '0 */5 * * * *' };
    case 'once':
      return { ...common, type, value: '' };
    case 'times':
      return { ...common, type, value: [] };
    case 'weekly':
      return { ...common, type, value: '', days: [], times: [] };
    case 'monthly':
      return { ...common, type, value: '', day: 1, times: [] };
  }
}

// ---------------------------------------------------------------------------
// 검증
// ---------------------------------------------------------------------------

const DURATION_RE = /^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/;
const HHMM_RE = /^([01]?\d|2[0-3]):([0-5]\d)$/;

/**
 * 검증 결과. 에러는 i18n 키와 (필요 시) 보간 슬롯 값으로 표현한다.
 * 모듈 스코프에서는 t() 를 호출할 수 없으므로, 렌더 시점에 컴포넌트가
 * t(key).replace('{time}', time) 으로 최종 문자열을 만든다.
 */
interface ScheduleError {
  key: string;
  /** times 형식 오류의 {time} 보간 값. */
  time?: string;
}

/** "HH:MM" 배열 검증 (필수 + 형식). weekly/monthly/times 공용. */
function validateTimes(times: string[]): ScheduleError | null {
  if (times.length === 0) return { key: 'property.schedule.errTimesRequired' };
  const bad = times.find((t) => !HHMM_RE.test(t));
  if (bad) return { key: 'property.schedule.errTimesFormat', time: bad };
  return null;
}

function validate(sched: Schedule): ScheduleError | null {
  switch (sched.type) {
    case 'interval': {
      const v = (sched.value as string).trim();
      if (!v) return { key: 'property.schedule.errIntervalRequired' };
      if (!DURATION_RE.test(v)) return { key: 'property.schedule.errIntervalFormat' };
      return null;
    }
    case 'cron': {
      const v = (sched.value as string).trim();
      if (!v) return { key: 'property.schedule.errCronRequired' };
      const parts = v.split(/\s+/);
      if (parts.length < 5 || parts.length > 6) return { key: 'property.schedule.errCronFieldCount' };
      return null;
    }
    case 'once': {
      const v = (sched.value as string).trim();
      if (!v) return { key: 'property.schedule.errOnceRequired' };
      const t = Date.parse(v);
      if (Number.isNaN(t)) return { key: 'property.schedule.errOnceInvalid' };
      if (t <= Date.now()) return { key: 'property.schedule.errOnceFuture' };
      return null;
    }
    case 'times':
      return validateTimes(sched.value as string[]);
    case 'weekly': {
      if ((sched.days ?? []).length === 0) return { key: 'property.schedule.errWeeklyDaysRequired' };
      return validateTimes(sched.times ?? []);
    }
    case 'monthly': {
      const day = sched.day;
      const dayOk =
        day === 'first' ||
        day === 'last' ||
        (typeof day === 'number' && Number.isInteger(day) && day >= 1 && day <= 31);
      if (!dayOk) return { key: 'property.schedule.errMonthlyDayInvalid' };
      return validateTimes(sched.times ?? []);
    }
  }
}

// ---------------------------------------------------------------------------
// datetime-local <-> RFC3339 변환
// ---------------------------------------------------------------------------

/** RFC3339/ISO 문자열 → <input type="datetime-local"> 값 (로컬 시간, 초 단위까지) */
function isoToLocalInput(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** <input type="datetime-local"> 값 → RFC3339 문자열 (로컬 타임존 오프셋 포함) */
function localInputToIso(local: string): string {
  if (!local) return '';
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return '';
  // toISOString()은 UTC로 변환하므로, 사용자가 지정한 로컬 시각이 그대로 의도에 맞다.
  return d.toISOString();
}

// ---------------------------------------------------------------------------
// 스타일
// ---------------------------------------------------------------------------

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

const cellInputError = 'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500';

// readOnly 스타일.
//
// 주의: text/number/datetime-local 등 readOnly attr 를 지원하는 input 에는
// `disabled` 가 아닌 `readOnly` 를 사용한다. `disabled` 를 적용하면
// 브라우저가 텍스트를 흐리게(회색조로) 렌더링해 다크모드에서 값이 거의
// 보이지 않는 가시성 회귀가 발생한다 (commit b4ad829 / 309966e 와 동일 패턴).
// opacity 는 낮추지 않고 배경만 살짝 다르게 표시한다.
//
// select 는 readOnly attr 미지원이므로 `disabled={readOnly}` 를 그대로 사용한다.
const readOnlyStyle = 'cursor-not-allowed bg-(--color-bg-elevated)';

const chipButton = cn(
  'rounded border px-1.5 py-0.5 text-[10px] font-medium transition-colors',
  'border-(--color-border-default) bg-(--color-bg-primary) text-(--color-text-secondary)',
  'hover:border-blue-400 hover:text-blue-600',
  'dark:hover:border-blue-500 dark:hover:text-blue-400',
);

// ---------------------------------------------------------------------------
// 메인 컴포넌트
// ---------------------------------------------------------------------------

export function TriggerScheduleEditor({ value, onChange, readOnly }: TriggerScheduleEditorProps) {
  const { t } = useTranslation();
  const [schedules, setSchedules] = useState<Schedule[]>(() => parseSchedules(value));
  const internalUpdate = useRef(false);

  // 외부 value 변경 시 내부 동기화 (내부 업데이트가 아닌 경우만)
  useEffect(() => {
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setSchedules(parseSchedules(value));
  }, [value]);

  // 내부 → 외부 emit
  const emit = useCallback(
    (updated: Schedule[]) => {
      setSchedules(updated);
      internalUpdate.current = true;
      onChange(serialize(updated));
    },
    [onChange],
  );

  const handleAdd = useCallback(
    (type: ScheduleType) => {
      emit([...schedules, defaultScheduleForType(type)]);
    },
    [schedules, emit],
  );

  const handleRemove = useCallback(
    (key: string) => {
      emit(schedules.filter((s) => s.key !== key));
    },
    [schedules, emit],
  );

  // 스케줄 항목의 일부 필드를 부분 갱신한다 (value/days/day/times/payload*).
  const handlePatch = useCallback(
    (key: string, patch: Partial<Schedule>) => {
      emit(schedules.map((s) => (s.key === key ? { ...s, ...patch } : s)));
    },
    [schedules, emit],
  );

  const handleTypeChange = useCallback(
    (key: string, nextType: ScheduleType) => {
      emit(
        schedules.map((s) => {
          if (s.key !== key) return s;
          // 타입이 바뀌면 스케줄 타이밍 필드는 기본값으로 리셋하되,
          // 스케줄별 페이로드(타이밍과 직교)는 보존한다.
          const fresh = defaultScheduleForType(nextType);
          return {
            ...fresh,
            key: s.key,
            payloadSet: s.payloadSet,
            payload: s.payload,
          };
        }),
      );
    },
    [schedules, emit],
  );

  return (
    <div className="space-y-2">
      {schedules.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.schedule.empty')}
        </p>
      )}

      {schedules.map((sched) => (
        <ScheduleRow
          key={sched.key}
          schedule={sched}
          readOnly={readOnly}
          onTypeChange={(type) => handleTypeChange(sched.key, type)}
          onPatch={(patch) => handlePatch(sched.key, patch)}
          onRemove={() => handleRemove(sched.key)}
        />
      ))}

      {!readOnly && (
        <AddScheduleMenu onAdd={handleAdd} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 스케줄 행 컴포넌트
// ---------------------------------------------------------------------------

interface ScheduleRowProps {
  schedule: Schedule;
  readOnly?: boolean;
  onTypeChange: (type: ScheduleType) => void;
  onPatch: (patch: Partial<Schedule>) => void;
  onRemove: () => void;
}

function ScheduleRow({ schedule, readOnly, onTypeChange, onPatch, onRemove }: ScheduleRowProps) {
  const { t } = useTranslation();
  const error = useMemo(() => validate(schedule), [schedule]);
  // 보간이 필요한 times 형식 오류는 {time} 슬롯을 치환한다.
  const errorText = error
    ? error.time !== undefined
      ? t(error.key).replace('{time}', error.time)
      : t(error.key)
    : null;

  // 타입별 위젯이 value 만 다루는 경우(interval/cron/once/times)를 위한 어댑터.
  const onValueChange = (v: string | string[]) => onPatch({ value: v });

  return (
    <div
      className={cn(
        'rounded-md border p-2 space-y-1.5',
        'border-(--color-border-default) bg-(--color-bg-primary)',
      )}
    >
      {/* 헤더: 타입 셀렉트 + 삭제 */}
      <div className="flex items-center gap-1.5">
        <select
          value={schedule.type}
          disabled={readOnly}
          onChange={(e) => onTypeChange(e.target.value as ScheduleType)}
          className={cn(cellInput, 'w-24 shrink-0', readOnly && readOnlyStyle)}
          aria-label={t('property.schedule.typeAria')}
        >
          <option value="interval">{t('property.schedule.typeInterval')}</option>
          <option value="cron">{t('property.schedule.typeCron')}</option>
          <option value="once">{t('property.schedule.typeOnce')}</option>
          <option value="times">{t('property.schedule.typeTimes')}</option>
          <option value="weekly">{t('property.schedule.typeWeekly')}</option>
          <option value="monthly">{t('property.schedule.typeMonthly')}</option>
        </select>

        <div className="flex-1" />

        {!readOnly && (
          <button
            type="button"
            onClick={onRemove}
            className="shrink-0 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            aria-label={t('property.schedule.deleteAria')}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {/* 타입별 입력 위젯 */}
      {schedule.type === 'interval' && (
        <IntervalInput
          value={schedule.value as string}
          readOnly={readOnly}
          error={error}
          onChange={onValueChange}
        />
      )}
      {schedule.type === 'cron' && (
        <CronInput
          value={schedule.value as string}
          readOnly={readOnly}
          error={error}
          onChange={onValueChange}
        />
      )}
      {schedule.type === 'once' && (
        <OnceInput
          value={schedule.value as string}
          readOnly={readOnly}
          error={error}
          onChange={onValueChange}
        />
      )}
      {schedule.type === 'times' && (
        <TimesInput
          value={schedule.value as string[]}
          readOnly={readOnly}
          error={error}
          onChange={onValueChange}
        />
      )}
      {schedule.type === 'weekly' && (
        <WeeklyInput
          days={schedule.days ?? []}
          times={schedule.times ?? []}
          readOnly={readOnly}
          error={error}
          onChange={(patch) => onPatch(patch)}
        />
      )}
      {schedule.type === 'monthly' && (
        <MonthlyInput
          day={schedule.day ?? 1}
          times={schedule.times ?? []}
          readOnly={readOnly}
          error={error}
          onChange={(patch) => onPatch(patch)}
        />
      )}

      {/* 에러 */}
      {errorText && (
        <p className="text-xs text-red-500 dark:text-red-400">{errorText}</p>
      )}

      {/* 스케줄별 페이로드 (선택) */}
      <SchedulePayloadEditor
        payloadSet={schedule.payloadSet}
        payload={schedule.payload}
        readOnly={readOnly}
        onChange={(patch) => onPatch(patch)}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// interval 입력
// ---------------------------------------------------------------------------

function IntervalInput({
  value,
  readOnly,
  error,
  onChange,
}: {
  value: string;
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (v: string) => void;
}) {
  return (
    <div className="space-y-1">
      <input
        type="text"
        value={value}
        // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
        readOnly={readOnly}
        onChange={(e) => onChange(e.target.value)}
        placeholder="5s, 1m, 1h"
        className={cn(cellInput, error && cellInputError, readOnly && readOnlyStyle)}
      />
      {!readOnly && (
        <div className="flex flex-wrap gap-1">
          {INTERVAL_PRESETS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => onChange(p)}
              className={chipButton}
            >
              {p}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// cron 입력
// ---------------------------------------------------------------------------

function CronInput({
  value,
  readOnly,
  error,
  onChange,
}: {
  value: string;
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <input
        type="text"
        value={value}
        // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
        readOnly={readOnly}
        onChange={(e) => onChange(e.target.value)}
        placeholder="0 */5 * * * *"
        className={cn(cellInput, 'font-mono', error && cellInputError, readOnly && readOnlyStyle)}
      />
      <p className="text-[10px] text-(--color-text-muted)">
        {t('property.schedule.cronHelp')}
      </p>
      {!readOnly && (
        <div className="flex flex-wrap gap-1">
          {CRON_PRESETS.map((p) => (
            <button
              key={p.labelKey}
              type="button"
              onClick={() => onChange(p.value)}
              className={chipButton}
              title={p.value}
            >
              {t(p.labelKey)}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// once 입력
// ---------------------------------------------------------------------------

function OnceInput({
  value,
  readOnly,
  error,
  onChange,
}: {
  value: string;
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const localValue = useMemo(() => isoToLocalInput(value), [value]);

  return (
    <div className="space-y-1">
      <input
        type="datetime-local"
        value={localValue}
        // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
        readOnly={readOnly}
        onChange={(e) => onChange(localInputToIso(e.target.value))}
        className={cn(cellInput, error && cellInputError, readOnly && readOnlyStyle)}
      />
      {value && !error && (
        <p className="text-[10px] text-(--color-text-muted)">
          {t('property.schedule.saved')} <span className="font-mono">{value}</span>
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// "HH:MM" 칩 목록 (times / weekly / monthly 공용 시각 편집 위젯)
// ---------------------------------------------------------------------------

function TimeChipList({
  value,
  readOnly,
  invalid,
  onChange,
}: {
  value: string[];
  readOnly?: boolean;
  invalid?: boolean;
  onChange: (v: string[]) => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<string>('');

  const handleAdd = () => {
    const time = draft.trim();
    if (!time || !HHMM_RE.test(time)) return;
    if (value.includes(time)) {
      setDraft('');
      return;
    }
    const sorted = [...value, time].sort();
    onChange(sorted);
    setDraft('');
  };

  const handleRemove = (time: string) => {
    onChange(value.filter((v) => v !== time));
  };

  return (
    <div className="space-y-1.5">
      {/* 추가된 시각 목록 */}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {value.map((time) => (
            <span
              key={time}
              className={cn(
                'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs font-mono',
                'border border-blue-200 bg-blue-50 text-blue-700',
                'dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300',
              )}
            >
              {time}
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => handleRemove(time)}
                  className="rounded hover:bg-blue-200 dark:hover:bg-blue-800"
                  aria-label={t('property.schedule.removeTimeAria').replace('{time}', time)}
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}

      {/* 추가 입력 */}
      {!readOnly && (
        <div className="flex items-center gap-1">
          <input
            type="time"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                handleAdd();
              }
            }}
            className={cn(cellInput, invalid && cellInputError, 'flex-1')}
          />
          <button
            type="button"
            onClick={handleAdd}
            disabled={!draft}
            className={cn(
              'shrink-0 rounded-md border border-dashed px-2 py-1 text-xs font-medium transition-colors',
              'border-(--color-border-default) text-(--color-text-muted)',
              'hover:border-blue-400 hover:text-blue-600 disabled:opacity-40',
              'dark:hover:border-blue-500 dark:hover:text-blue-400',
            )}
          >
            <Plus className="mr-0.5 inline h-3 w-3" />
            {t('property.schedule.addTime')}
          </button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// times 입력 (HH:MM 칩 목록)
// ---------------------------------------------------------------------------

function TimesInput({
  value,
  readOnly,
  error,
  onChange,
}: {
  value: string[];
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (v: string[]) => void;
}) {
  return (
    <TimeChipList value={value} readOnly={readOnly} invalid={!!error} onChange={onChange} />
  );
}

// ---------------------------------------------------------------------------
// weekly 입력 (요일 토글 + 시각 칩 목록)
// ---------------------------------------------------------------------------

function WeeklyInput({
  days,
  times,
  readOnly,
  error,
  onChange,
}: {
  days: string[];
  times: string[];
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (patch: Partial<Schedule>) => void;
}) {
  const { t } = useTranslation();

  const toggleDay = (token: string) => {
    const next = days.includes(token)
      ? days.filter((d) => d !== token)
      : // WEEKDAY_TOKENS 순서를 유지하도록 정렬.
        WEEKDAY_TOKENS.filter((d) => d === token || days.includes(d));
    onChange({ days: next });
  };

  const daysInvalid = !!error && (days.length === 0);

  return (
    <div className="space-y-1.5">
      {/* 요일 토글 */}
      <div className="space-y-1">
        <p className="text-[10px] text-(--color-text-muted)">{t('property.schedule.weeklyDays')}</p>
        <div className="flex flex-wrap gap-1">
          {WEEKDAY_TOKENS.map((token) => {
            const active = days.includes(token);
            return (
              <button
                key={token}
                type="button"
                disabled={readOnly}
                onClick={() => toggleDay(token)}
                aria-pressed={active}
                className={cn(
                  'h-7 w-7 rounded border text-xs font-medium transition-colors',
                  active
                    ? 'border-blue-400 bg-blue-500 text-white dark:border-blue-500'
                    : 'border-(--color-border-default) bg-(--color-bg-primary) text-(--color-text-secondary) hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400',
                  daysInvalid && !active && 'border-red-400 dark:border-red-500',
                  readOnly && 'cursor-not-allowed opacity-70',
                )}
              >
                {t(`property.schedule.weekday.${token}`)}
              </button>
            );
          })}
        </div>
        {!readOnly && (
          <div className="flex flex-wrap gap-1">
            {WEEKLY_PRESETS.map((p) => (
              <button
                key={p.labelKey}
                type="button"
                onClick={() => onChange({ days: [...p.days] })}
                className={chipButton}
              >
                {t(p.labelKey)}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* 시각 목록 */}
      <div className="space-y-1">
        <p className="text-[10px] text-(--color-text-muted)">{t('property.schedule.timesLabel')}</p>
        <TimeChipList
          value={times}
          readOnly={readOnly}
          invalid={!!error && days.length > 0}
          onChange={(v) => onChange({ times: v })}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// monthly 입력 (일자 선택 + 시각 칩 목록)
// ---------------------------------------------------------------------------

const MONTHLY_DAY_NUMBERS = Array.from({ length: 31 }, (_, i) => i + 1);

function MonthlyInput({
  day,
  times,
  readOnly,
  error,
  onChange,
}: {
  day: MonthlyDay;
  times: string[];
  readOnly?: boolean;
  error: ScheduleError | null;
  onChange: (patch: Partial<Schedule>) => void;
}) {
  const { t } = useTranslation();

  const handleDayChange = (raw: string) => {
    const next: MonthlyDay = raw === 'first' || raw === 'last' ? raw : Number(raw);
    onChange({ day: next });
  };

  // 존재하지 않는 일자(29~31)를 정수로 지정하면 안내 힌트 노출.
  const showMissingDayHint = typeof day === 'number' && day >= 29 && day <= 31;
  const daySuffix = t('property.schedule.monthlyDaySuffix');

  return (
    <div className="space-y-1.5">
      {/* 일자 선택 */}
      <div className="space-y-1">
        <p className="text-[10px] text-(--color-text-muted)">{t('property.schedule.monthlyDay')}</p>
        <select
          value={String(day)}
          disabled={readOnly}
          onChange={(e) => handleDayChange(e.target.value)}
          className={cn(cellInput, readOnly && readOnlyStyle)}
          aria-label={t('property.schedule.monthlyDay')}
        >
          <option value="first">{t('property.schedule.monthlyDayFirst')}</option>
          <option value="last">{t('property.schedule.monthlyDayLast')}</option>
          {MONTHLY_DAY_NUMBERS.map((n) => (
            <option key={n} value={String(n)}>
              {`${n}${daySuffix}`}
            </option>
          ))}
        </select>
        {!readOnly && (
          <div className="flex flex-wrap gap-1">
            {MONTHLY_PRESETS.map((p) => (
              <button
                key={p.labelKey}
                type="button"
                onClick={() => onChange({ day: p.day })}
                className={chipButton}
              >
                {t(p.labelKey)}
              </button>
            ))}
          </div>
        )}
        {showMissingDayHint && (
          <p className="text-[10px] text-amber-600 dark:text-amber-400">
            {t('property.schedule.monthlyDayHint')}
          </p>
        )}
      </div>

      {/* 시각 목록 */}
      <div className="space-y-1">
        <p className="text-[10px] text-(--color-text-muted)">{t('property.schedule.timesLabel')}</p>
        <TimeChipList
          value={times}
          readOnly={readOnly}
          invalid={!!error}
          onChange={(v) => onChange({ times: v })}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// 스케줄별 페이로드 편집 (접힘 섹션)
// ---------------------------------------------------------------------------

function SchedulePayloadEditor({
  payloadSet,
  payload,
  readOnly,
  onChange,
}: {
  payloadSet: boolean;
  payload: unknown;
  readOnly?: boolean;
  onChange: (patch: Partial<Schedule>) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState<boolean>(false);

  // none | set 토글. set 으로 전환 시 기존 값 보존(없으면 빈 오브젝트), none 이면 값 제거.
  const setEnabled = (enabled: boolean) => {
    if (enabled) {
      onChange({ payloadSet: true, payload: payload ?? {} });
    } else {
      onChange({ payloadSet: false, payload: undefined });
    }
  };

  return (
    <div className="rounded border border-dashed border-(--color-border-default)">
      {/* 헤더 (토글) */}
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-1 px-2 py-1 text-left text-[11px] text-(--color-text-secondary) transition-colors hover:text-blue-600 dark:hover:text-blue-400"
      >
        {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
        <span className="flex-1">{t('property.schedule.payloadSection')}</span>
        {payloadSet && (
          <span className="rounded bg-blue-100 px-1.5 py-0.5 text-[9px] font-medium text-blue-700 dark:bg-blue-950 dark:text-blue-300">
            {t('property.schedule.payloadSetBadge')}
          </span>
        )}
      </button>

      {open && (
        <div className="space-y-1.5 border-t border-(--color-border-default) p-2">
          <p className="text-[10px] text-(--color-text-muted)">
            {payloadSet
              ? t('property.schedule.payloadOverrideNote')
              : t('property.schedule.payloadFallbackNote')}
          </p>

          {/* none | set 2단 토글 */}
          <div className="flex gap-1">
            <button
              type="button"
              disabled={readOnly}
              onClick={() => setEnabled(false)}
              aria-pressed={!payloadSet}
              className={cn(
                'flex-1 rounded border px-2 py-1 text-xs font-medium transition-colors',
                !payloadSet
                  ? 'border-blue-400 bg-blue-500 text-white dark:border-blue-500'
                  : 'border-(--color-border-default) bg-(--color-bg-primary) text-(--color-text-secondary) hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400',
                readOnly && 'cursor-not-allowed opacity-70',
              )}
            >
              {t('property.schedule.payloadOptionNone')}
            </button>
            <button
              type="button"
              disabled={readOnly}
              onClick={() => setEnabled(true)}
              aria-pressed={payloadSet}
              className={cn(
                'flex-1 rounded border px-2 py-1 text-xs font-medium transition-colors',
                payloadSet
                  ? 'border-blue-400 bg-blue-500 text-white dark:border-blue-500'
                  : 'border-(--color-border-default) bg-(--color-bg-primary) text-(--color-text-secondary) hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400',
                readOnly && 'cursor-not-allowed opacity-70',
              )}
            >
              {t('property.schedule.payloadOptionSet')}
            </button>
          </div>

          {/* set: 단일 JSON 오브젝트 에디터 (노드 레벨과 동일 패턴) + 인라인 힌트 */}
          {payloadSet && (
            <div className="space-y-1">
              <textarea
                rows={4}
                value={typeof payload === 'string' ? payload : JSON.stringify(payload ?? {}, null, 2)}
                readOnly={readOnly}
                onChange={(e) => {
                  try {
                    onChange({ payload: JSON.parse(e.target.value) as unknown });
                  } catch {
                    // JSON 파싱 실패 시 문자열 그대로 저장 (object 필드와 동일 동작).
                    onChange({ payload: e.target.value });
                  }
                }}
                placeholder='{"shift": "morning", "at": "$.trigger_time"}'
                className={cn(cellInput, 'font-mono text-xs', readOnly && readOnlyStyle)}
              />
              <p className="text-[10px] text-(--color-text-muted)">
                {t('property.schedule.payloadHint')}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 추가 메뉴 (하단 "+ 추가" 버튼)
// ---------------------------------------------------------------------------

function AddScheduleMenu({ onAdd }: { onAdd: (type: ScheduleType) => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
      >
        <Plus className="h-3.5 w-3.5" />
        {t('property.schedule.add')}
      </button>
    );
  }

  const choose = (type: ScheduleType) => {
    onAdd(type);
    setOpen(false);
  };

  return (
    <div className="flex flex-wrap items-center gap-1 rounded-md border border-dashed border-(--color-border-default) p-1.5">
      <span className="px-1 text-[10px] text-(--color-text-muted)">{t('property.schedule.selectType')}</span>
      <button type="button" onClick={() => choose('interval')} className={chipButton}>{t('property.schedule.typeInterval')}</button>
      <button type="button" onClick={() => choose('cron')} className={chipButton}>{t('property.schedule.typeCron')}</button>
      <button type="button" onClick={() => choose('once')} className={chipButton}>{t('property.schedule.typeOnce')}</button>
      <button type="button" onClick={() => choose('times')} className={chipButton}>{t('property.schedule.typeTimes')}</button>
      <button type="button" onClick={() => choose('weekly')} className={chipButton}>{t('property.schedule.typeWeekly')}</button>
      <button type="button" onClick={() => choose('monthly')} className={chipButton}>{t('property.schedule.typeMonthly')}</button>
      <button
        type="button"
        onClick={() => setOpen(false)}
        className="ml-auto rounded p-0.5 text-(--color-text-muted) hover:text-(--color-text-secondary)"
        aria-label={t('property.schedule.cancelAria')}
      >
        <X className="h-3 w-3" />
      </button>
    </div>
  );
}
