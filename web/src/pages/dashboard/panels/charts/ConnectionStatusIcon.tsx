// 채널 연결 상태 아이콘 (REQ-M4-09).
// connecting/connected/disconnected/closed/error 상태별로 서로 다른 아이콘을 렌더한다.

import { clsx } from 'clsx';
import { AlertTriangle, CircleDashed, PlugZap, Wifi, WifiOff } from 'lucide-react';

import type { ChartConnectionStatus } from './chartChannelTypes';

interface ConnectionStatusIconProps {
  status: ChartConnectionStatus;
  className?: string;
}

export function ConnectionStatusIcon({ status, className }: ConnectionStatusIconProps) {
  const base = clsx('h-4 w-4', className);
  switch (status) {
    case 'connected':
      return (
        <Wifi
          data-testid="chart-status-icon"
          data-status="connected"
          className={clsx(base, 'text-emerald-500')}
          aria-label="연결됨"
        />
      );
    case 'connecting':
      return (
        <CircleDashed
          data-testid="chart-status-icon"
          data-status="connecting"
          className={clsx(base, 'animate-spin text-amber-500')}
          aria-label="연결 중"
        />
      );
    case 'disconnected':
      return (
        <WifiOff
          data-testid="chart-status-icon"
          data-status="disconnected"
          className={clsx(base, 'text-gray-400')}
          aria-label="연결 끊김"
        />
      );
    case 'closed':
      return (
        <PlugZap
          data-testid="chart-status-icon"
          data-status="closed"
          className={clsx(base, 'text-gray-500')}
          aria-label="채널 종료"
        />
      );
    case 'error':
      return (
        <AlertTriangle
          data-testid="chart-status-icon"
          data-status="error"
          className={clsx(base, 'text-rose-500')}
          aria-label="오류"
        />
      );
    case 'idle':
    default:
      return (
        <CircleDashed
          data-testid="chart-status-icon"
          data-status="idle"
          className={clsx(base, 'text-gray-300')}
          aria-label="대기"
        />
      );
  }
}
