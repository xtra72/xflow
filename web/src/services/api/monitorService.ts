import { del, get, put } from './client';

/**
 * Retrieve runtime metrics (Prometheus-style or custom).
 */
export async function getMetrics(): Promise<Record<string, unknown>> {
  return get<Record<string, unknown>>('/monitor/metrics');
}

/**
 * Change the server-side log level at runtime.
 * Valid levels typically include: debug, info, warn, error.
 */
export async function setLogLevel(level: string): Promise<void> {
  await put<void>('/monitor/loglevel', { level });
}

// --- 컴포넌트별 로그 레벨 ---

/** 서버에서 반환하는 로그 레벨 정보 */
export interface LogLevelInfo {
  default_level: string;
  components: Record<string, string>;
}

/**
 * 전체 로그 레벨 정보를 조회한다.
 * 기본 레벨과 컴포넌트별 오버라이드를 포함한다.
 */
export async function getLogLevels(): Promise<LogLevelInfo> {
  return get<LogLevelInfo>('/monitor/loglevel');
}

/**
 * 특정 컴포넌트의 로그 레벨을 설정한다.
 */
export async function setComponentLogLevel(component: string, level: string): Promise<void> {
  await put<void>(`/monitor/loglevel/${component}`, { level });
}

/**
 * 특정 컴포넌트의 로그 레벨 오버라이드를 제거하여 기본값으로 되돌린다.
 */
export async function resetComponentLogLevel(component: string): Promise<void> {
  await del(`/monitor/loglevel/${component}`);
}
