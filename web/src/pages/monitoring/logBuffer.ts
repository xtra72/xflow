// 로그 버퍼 유틸 — 보관 상한과 상한 적용 append.
//
// 컴포넌트 파일(LogViewer.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만 내보내야
// Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

import type { LogEntry } from './LogViewer';

/** 최대 보관 로그 수. */
export const MAX_ENTRIES = 10_000;

/**
 * 로그 배열에 새 항목을 추가하면서 MAX_ENTRIES 제한을 적용한다.
 * MonitoringPage에서 상태 업데이트에 사용할 유틸리티 함수.
 */
export function appendLog(
  prev: LogEntry[],
  entry: LogEntry,
): LogEntry[] {
  const next = [...prev, entry];
  return next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
}
