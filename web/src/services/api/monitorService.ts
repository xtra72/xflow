import { get, put } from './client';

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
