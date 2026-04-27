// 차트 채널 구독 React 훅.
// ChartChannelClient 를 래핑해 entries / status 를 상태로 관리한다.

import { useEffect, useRef, useState } from 'react';

import {
  ChartChannelClient,
  type ChartChannelClientOptions,
  type ChartConnectionStatus,
  type ChartServerMessage,
} from '@/services/ws/chartChannel';
import type { ChartEntry } from './chartChannelTypes';

/** 훅 결과 */
export interface UseChartChannelResult {
  entries: ChartEntry[];
  status: ChartConnectionStatus;
  closedReason?: string;
  errorReason?: string;
}

/** 훅에 주입할 수 있는 클라이언트 인터페이스 (테스트용) */
export interface ChartChannelClientLike {
  connect: () => void;
  disconnect: () => void;
  isConnected: () => boolean;
}

export interface UseChartChannelOptions {
  /** 링 버퍼 최대 길이 (default 1000) */
  maxPoints?: number;
  /** WS base URL (default: window.location 기반) */
  wsBaseUrl?: string;
  /** 테스트 주입용 클라이언트 팩토리 */
  createClient?: (opts: ChartChannelClientOptions) => ChartChannelClientLike;
}

const DEFAULT_MAX_POINTS = 1000;

/** window.location 기반으로 ws URL 을 생성 */
function deriveWsBaseUrl(): string {
  if (typeof window === 'undefined') return 'ws://localhost';
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}`;
}

/** 채널명을 WebSocket 엔드포인트로 변환 */
function buildChannelUrl(base: string, channelName: string): string {
  const trimmed = base.replace(/\/+$/, '');
  return `${trimmed}/ws/chart/${channelName}`;
}

/**
 * 채널을 구독하고 entries/status 를 제공한다.
 *
 * - channelName 이 undefined 면 idle 상태로 유지
 * - chart.backfill 수신 시 entries 를 교체 (과거분으로 교체)
 * - chart.append 수신 시 entries 에 append, maxPoints 초과 시 tail 유지
 * - chart.closed 수신 시 status='closed' + closedReason
 * - chart.error 수신 시 status='error' + errorReason
 */
export function useChartChannel(
  channelName: string | undefined,
  options: UseChartChannelOptions = {},
): UseChartChannelResult {
  const maxPoints = options.maxPoints ?? DEFAULT_MAX_POINTS;

  const [entries, setEntries] = useState<ChartEntry[]>([]);
  const [status, setStatus] = useState<ChartConnectionStatus>('idle');
  const [closedReason, setClosedReason] = useState<string | undefined>(undefined);
  const [errorReason, setErrorReason] = useState<string | undefined>(undefined);

  // maxPoints 변경 시에도 핸들러 클로저가 최신값을 참조하도록 ref 사용
  const maxPointsRef = useRef(maxPoints);
  maxPointsRef.current = maxPoints;

  useEffect(() => {
    if (!channelName) {
      setStatus('idle');
      setEntries([]);
      setClosedReason(undefined);
      setErrorReason(undefined);
      return;
    }

    setStatus('connecting');
    setClosedReason(undefined);
    setErrorReason(undefined);

    const baseUrl = options.wsBaseUrl ?? deriveWsBaseUrl();
    const url = buildChannelUrl(baseUrl, channelName);

    const onMessage = (msg: ChartServerMessage) => {
      if (msg.type === 'chart.backfill') {
        const cap = maxPointsRef.current;
        const incoming = msg.entries;
        setEntries(incoming.length > cap ? incoming.slice(-cap) : incoming.slice());
      } else if (msg.type === 'chart.append') {
        setEntries((prev) => {
          const next = prev.concat(msg.entry);
          const cap = maxPointsRef.current;
          return next.length > cap ? next.slice(next.length - cap) : next;
        });
      } else if (msg.type === 'chart.closed') {
        setClosedReason(msg.reason);
      } else if (msg.type === 'chart.error') {
        setErrorReason(msg.reason);
      }
    };

    const onStatus = (s: ChartConnectionStatus, detail?: string) => {
      setStatus(s);
      if (s === 'closed' && detail) setClosedReason(detail);
      if (s === 'error' && detail) setErrorReason(detail);
    };

    const clientOpts: ChartChannelClientOptions = {
      url,
      onMessage,
      onStatus,
    };

    const client = options.createClient
      ? options.createClient(clientOpts)
      : (new ChartChannelClient(clientOpts) as ChartChannelClientLike);

    client.connect();

    return () => {
      client.disconnect();
    };
    // options.createClient / wsBaseUrl 은 참조 안정 가정 (패널이 리렌더 시 재생성하지 않음)
    // channelName 변경 시에만 재구독
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelName]);

  return { entries, status, closedReason, errorReason };
}
