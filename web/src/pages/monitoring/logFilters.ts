// 로그 정렬·필터 순수 로직.
//
// 컴포넌트 파일(LogViewer.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만 내보내야
// Fast Refresh 가 동작하고(react-refresh/only-export-components), 정렬·필터는
// 렌더와 무관한 순수 함수라 단위 테스트가 쉬워진다 — logBuffer.ts 와 같은 이유다.

import type { LogEntry, LogLevel } from './LogViewer';

/** 정렬 방향. 정렬 기준은 시간 하나뿐이다. */
export type SortDir = 'asc' | 'desc';

/** 컬럼 헤더 필터 상태 — 빈 Set / 빈 문자열은 "필터 없음"을 뜻한다. */
export interface LogFilterState {
  /** 레벨 선택 (빈 Set = 전체) */
  levels: Set<string>;
  /** 소스 선택 (빈 Set = 전체) */
  sources: Set<string>;
  /** 타입(componentKind) 선택 (빈 Set = 전체) */
  kinds: Set<string>;
  /** 이름(componentName) 선택 (빈 Set = 전체) */
  names: Set<string>;
  /** 시작 시각 "HH:MM" 또는 "HH:MM:SS" (빈 문자열 = 하한 없음) */
  timeFrom: string;
  /** 종료 시각 "HH:MM" 또는 "HH:MM:SS" (빈 문자열 = 상한 없음) */
  timeTo: string;
}

/** 필터가 걸리지 않은 초기 상태 */
export function emptyFilterState(): LogFilterState {
  return {
    levels: new Set(),
    sources: new Set(),
    kinds: new Set(),
    names: new Set(),
    timeFrom: '',
    timeTo: '',
  };
}

/** 하나라도 걸린 필터가 있는지 */
export function hasAnyFilter(state: LogFilterState): boolean {
  return (
    state.levels.size > 0 ||
    state.sources.size > 0 ||
    state.kinds.size > 0 ||
    state.names.size > 0 ||
    state.timeFrom !== '' ||
    state.timeTo !== ''
  );
}

/**
 * "HH:MM" / "HH:MM:SS" 를 자정 기준 초로 바꾼다. 형식이 아니면 null.
 *
 * `<input type="time">` 은 초 단위를 생략한 "HH:MM" 을 주기도 하므로 둘 다 받는다.
 */
export function parseTimeOfDay(value: string): number | null {
  const m = /^(\d{1,2}):(\d{2})(?::(\d{2}))?$/.exec(value.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  const sec = m[3] ? Number(m[3]) : 0;
  if (h > 23 || min > 59 || sec > 59) return null;
  return h * 3_600 + min * 60 + sec;
}

/**
 * 로그 항목의 "그날 몇 초" 값. 구할 수 없으면 null.
 *
 * epoch(ts)가 있으면 그쪽이 정확하다. 없으면 표시용 "HH:MM:SS" 문자열에서 읽는다
 * (구버전 항목·직접 만든 테스트 픽스처 대비).
 */
export function entryTimeOfDay(entry: LogEntry): number | null {
  if (typeof entry.ts === 'number' && Number.isFinite(entry.ts)) {
    const d = new Date(entry.ts);
    return d.getHours() * 3_600 + d.getMinutes() * 60 + d.getSeconds();
  }
  return parseTimeOfDay(entry.timestamp);
}

/**
 * 시각이 [from, to] 구간에 드는지.
 *
 * from > to 이면 자정을 넘는 구간으로 본다(예: 22:00~02:00). 이렇게 두지 않으면
 * 자정을 걸친 조회가 항상 0건이 되어 조용히 잘못된 답을 준다.
 */
function inTimeRange(sec: number, from: number | null, to: number | null): boolean {
  if (from === null && to === null) return true;
  if (from === null) return sec <= (to as number);
  if (to === null) return sec >= from;
  return from <= to ? sec >= from && sec <= to : sec >= from || sec <= to;
}

/** 선택 필터 통과 여부 — 빈 Set 은 전체 통과 */
function passesSelection(value: string | undefined, selected: Set<string>): boolean {
  if (selected.size === 0) return true;
  return value !== undefined && value !== '' && selected.has(value);
}

/** 컬럼 필터를 모두 AND 로 적용한다. */
export function filterLogs(entries: LogEntry[], state: LogFilterState): LogEntry[] {
  const from = parseTimeOfDay(state.timeFrom);
  const to = parseTimeOfDay(state.timeTo);
  const timeFiltered = from !== null || to !== null;

  return entries.filter((e) => {
    if (!passesSelection(e.level, state.levels)) return false;
    if (!passesSelection(e.source, state.sources)) return false;
    if (!passesSelection(e.componentKind, state.kinds)) return false;
    if (!passesSelection(e.componentName, state.names)) return false;
    if (timeFiltered) {
      const sec = entryTimeOfDay(e);
      // 시각을 읽을 수 없는 항목은 구간 조회에서 제외한다 — 포함시키면 필터가
      // 걸렸는데도 무관한 줄이 섞여 결과를 믿을 수 없게 된다.
      if (sec === null) return false;
      if (!inTimeRange(sec, from, to)) return false;
    }
    return true;
  });
}

/**
 * 시간 기준 정렬. 정렬 기준은 시간 하나뿐이다.
 *
 * `ts` 가 같거나 없는 항목끼리는 수신 순서를 유지한다(안정 정렬) — 같은 밀리초에
 * 쏟아진 로그의 순서가 렌더마다 뒤바뀌지 않게 하기 위함이다.
 */
export function sortLogs(entries: LogEntry[], dir: SortDir): LogEntry[] {
  const decorated = entries.map((entry, index) => ({ entry, index }));
  decorated.sort((a, b) => {
    const at = a.entry.ts;
    const bt = b.entry.ts;
    if (typeof at === 'number' && typeof bt === 'number' && at !== bt) {
      return dir === 'asc' ? at - bt : bt - at;
    }
    // 시각을 비교할 수 없으면 수신 순서가 곧 시간 순서다.
    return dir === 'asc' ? a.index - b.index : b.index - a.index;
  });
  return decorated.map((d) => d.entry);
}

/**
 * 컬럼 값에서 선택 옵션 목록을 만든다 (중복 제거 + 사전순, 빈 값 제외).
 *
 * 타입·이름은 값 집합이 런타임에 정해지므로 현재 버퍼에서 뽑는다.
 */
export function collectOptions(
  entries: LogEntry[],
  pick: (e: LogEntry) => string | undefined,
): string[] {
  const set = new Set<string>();
  for (const entry of entries) {
    const value = pick(entry);
    if (value) set.add(value);
  }
  return [...set].sort((a, b) => a.localeCompare(b));
}

/** 레벨 필터에 쓰는 고정 옵션 (데이터에 없어도 항상 노출) */
export const ALL_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];

/** 소스 필터에 쓰는 고정 옵션 */
export const ALL_SOURCES: string[] = ['agent', 'node', 'flow', 'api', 'engine', 'system'];
