// 트리거 노드 스케줄 에디터 컴포넌트.
// interval / cron / once / times 4종 스케줄을 행 단위로 편집한다.
// 각 타입별로 전용 입력 위젯을 제공하고, 잘못된 형식은 인라인 검증 메시지로 표시한다.
//
// 저장 형식은 백엔드(trigger.go)와 호환되는 배열이다:
//   [{ type: 'interval', value: '5s' },
//    { type: 'cron',     value: '0 */5 * * *' },
//    { type: 'once',     value: '2026-04-15T10:00:00Z' },
//    { type: 'times',    value: ['09:00', '12:00'] }]

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

type ScheduleType = 'interval' | 'cron' | 'once' | 'times';

interface Schedule {
  /** 내부 key (React 렌더 + 추적용, 저장 시 제외) */
  key: string;
  type: ScheduleType;
  /** interval/cron/once 는 string, times 는 string[] */
  value: string | string[];
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

const CRON_PRESETS: { label: string; value: string }[] = [
  { label: '매분', value: '0 * * * * *' },
  { label: '5분마다', value: '0 */5 * * * *' },
  { label: '매시 정각', value: '0 0 * * * *' },
  { label: '매일 자정', value: '0 0 0 * * *' },
  { label: '매일 오전 9시', value: '0 0 9 * * *' },
  { label: '평일 오전 9시', value: '0 0 9 * * 1-5' },
];

// ---------------------------------------------------------------------------
// 유틸리티
// ---------------------------------------------------------------------------

let keyCounter = 0;
function nextKey(): string {
  return `sch-${++keyCounter}-${Date.now()}`;
}

/** 외부 value(unknown) → Schedule[] 변환 */
function parseSchedules(val: unknown): Schedule[] {
  if (!Array.isArray(val)) return [];
  const result: Schedule[] = [];
  for (const raw of val as unknown[]) {
    if (!raw || typeof raw !== 'object') continue;
    const obj = raw as Record<string, unknown>;
    const type = obj.type;
    if (type !== 'interval' && type !== 'cron' && type !== 'once' && type !== 'times') {
      continue;
    }
    if (type === 'times') {
      const arr = Array.isArray(obj.value) ? (obj.value as unknown[]).map((v) => String(v ?? '')) : [];
      result.push({ key: nextKey(), type, value: arr });
    } else {
      result.push({ key: nextKey(), type, value: String(obj.value ?? '') });
    }
  }
  return result;
}

/** Schedule[] → 저장용 배열 (key 제거) */
function serialize(schedules: Schedule[]): unknown[] {
  return schedules.map((s) => ({ type: s.type, value: s.value }));
}

/** 새 스케줄의 기본값 */
function defaultScheduleForType(type: ScheduleType): Schedule {
  switch (type) {
    case 'interval':
      return { key: nextKey(), type, value: '5s' };
    case 'cron':
      return { key: nextKey(), type, value: '0 */5 * * * *' };
    case 'once':
      return { key: nextKey(), type, value: '' };
    case 'times':
      return { key: nextKey(), type, value: [] };
  }
}

// ---------------------------------------------------------------------------
// 검증
// ---------------------------------------------------------------------------

const DURATION_RE = /^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/;
const HHMM_RE = /^([01]?\d|2[0-3]):([0-5]\d)$/;

function validate(sched: Schedule): string | null {
  switch (sched.type) {
    case 'interval': {
      const v = (sched.value as string).trim();
      if (!v) return '간격을 입력하세요 (예: 5s, 1m)';
      if (!DURATION_RE.test(v)) return '형식 오류 (예: 5s, 1m, 1h)';
      return null;
    }
    case 'cron': {
      const v = (sched.value as string).trim();
      if (!v) return 'cron 표현식을 입력하세요';
      const parts = v.split(/\s+/);
      if (parts.length < 5 || parts.length > 6) return '필드 개수 오류 (5 또는 6)';
      return null;
    }
    case 'once': {
      const v = (sched.value as string).trim();
      if (!v) return '실행 시각을 선택하세요';
      const t = Date.parse(v);
      if (Number.isNaN(t)) return '유효한 시각이 아닙니다';
      if (t <= Date.now()) return '현재보다 미래 시각이어야 합니다';
      return null;
    }
    case 'times': {
      const arr = sched.value as string[];
      if (arr.length === 0) return '시각을 1개 이상 추가하세요';
      const bad = arr.find((t) => !HHMM_RE.test(t));
      if (bad) return `형식 오류: ${bad} (HH:MM)`;
      return null;
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

  const handleValueChange = useCallback(
    (key: string, nextValue: string | string[]) => {
      emit(schedules.map((s) => (s.key === key ? { ...s, value: nextValue } : s)));
    },
    [schedules, emit],
  );

  const handleTypeChange = useCallback(
    (key: string, nextType: ScheduleType) => {
      emit(
        schedules.map((s) => {
          if (s.key !== key) return s;
          // 타입이 바뀌면 기본값으로 리셋
          const fresh = defaultScheduleForType(nextType);
          return { ...fresh, key: s.key };
        }),
      );
    },
    [schedules, emit],
  );

  return (
    <div className="space-y-2">
      {schedules.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          스케줄이 없습니다
        </p>
      )}

      {schedules.map((sched) => (
        <ScheduleRow
          key={sched.key}
          schedule={sched}
          readOnly={readOnly}
          onTypeChange={(t) => handleTypeChange(sched.key, t)}
          onValueChange={(v) => handleValueChange(sched.key, v)}
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
  onValueChange: (value: string | string[]) => void;
  onRemove: () => void;
}

function ScheduleRow({ schedule, readOnly, onTypeChange, onValueChange, onRemove }: ScheduleRowProps) {
  const error = useMemo(() => validate(schedule), [schedule]);

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
          aria-label="스케줄 타입"
        >
          <option value="interval">주기</option>
          <option value="cron">cron</option>
          <option value="once">1회</option>
          <option value="times">매일 시각</option>
        </select>

        <div className="flex-1" />

        {!readOnly && (
          <button
            type="button"
            onClick={onRemove}
            className="shrink-0 rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            aria-label="스케줄 삭제"
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

      {/* 에러 */}
      {error && (
        <p className="text-xs text-red-500 dark:text-red-400">{error}</p>
      )}
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
  error: string | null;
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
  error: string | null;
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
        placeholder="0 */5 * * * *"
        className={cn(cellInput, 'font-mono', error && cellInputError, readOnly && readOnlyStyle)}
      />
      <p className="text-[10px] text-(--color-text-muted)">
        형식: 초 분 시 일 월 요일 (6필드) 또는 분 시 일 월 요일 (5필드)
      </p>
      {!readOnly && (
        <div className="flex flex-wrap gap-1">
          {CRON_PRESETS.map((p) => (
            <button
              key={p.label}
              type="button"
              onClick={() => onChange(p.value)}
              className={chipButton}
              title={p.value}
            >
              {p.label}
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
  error: string | null;
  onChange: (v: string) => void;
}) {
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
          저장: <span className="font-mono">{value}</span>
        </p>
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
  error: string | null;
  onChange: (v: string[]) => void;
}) {
  const [draft, setDraft] = useState<string>('');

  const handleAdd = () => {
    const t = draft.trim();
    if (!t || !HHMM_RE.test(t)) return;
    if (value.includes(t)) {
      setDraft('');
      return;
    }
    const sorted = [...value, t].sort();
    onChange(sorted);
    setDraft('');
  };

  const handleRemove = (t: string) => {
    onChange(value.filter((v) => v !== t));
  };

  return (
    <div className="space-y-1.5">
      {/* 추가된 시각 목록 */}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {value.map((t) => (
            <span
              key={t}
              className={cn(
                'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs font-mono',
                'border border-blue-200 bg-blue-50 text-blue-700',
                'dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300',
              )}
            >
              {t}
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => handleRemove(t)}
                  className="rounded hover:bg-blue-200 dark:hover:bg-blue-800"
                  aria-label={`${t} 제거`}
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
            className={cn(cellInput, error && cellInputError, 'flex-1')}
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
            추가
          </button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 추가 메뉴 (하단 "+ 추가" 버튼)
// ---------------------------------------------------------------------------

function AddScheduleMenu({ onAdd }: { onAdd: (type: ScheduleType) => void }) {
  const [open, setOpen] = useState(false);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
      >
        <Plus className="h-3.5 w-3.5" />
        스케줄 추가
      </button>
    );
  }

  const choose = (type: ScheduleType) => {
    onAdd(type);
    setOpen(false);
  };

  return (
    <div className="flex flex-wrap items-center gap-1 rounded-md border border-dashed border-(--color-border-default) p-1.5">
      <span className="px-1 text-[10px] text-(--color-text-muted)">타입 선택:</span>
      <button type="button" onClick={() => choose('interval')} className={chipButton}>주기</button>
      <button type="button" onClick={() => choose('cron')} className={chipButton}>cron</button>
      <button type="button" onClick={() => choose('once')} className={chipButton}>1회</button>
      <button type="button" onClick={() => choose('times')} className={chipButton}>매일 시각</button>
      <button
        type="button"
        onClick={() => setOpen(false)}
        className="ml-auto rounded p-0.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
        aria-label="취소"
      >
        <X className="h-3 w-3" />
      </button>
    </div>
  );
}
