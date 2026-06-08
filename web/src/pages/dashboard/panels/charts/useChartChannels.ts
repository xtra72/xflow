// 다중 차트 채널 동시 구독 React 훅.
// useChartChannel 의 다채널 변종 — 내부적으로 채널별 client 를 Map 으로 관리한다.

import { useEffect, useRef, useState } from 'react';

import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { remoteChartStreamUrl } from '@/services/api/remoteService';
import {
  ChartChannelClient,
  type ChartChannelClientOptions,
  type ChartConnectionStatus,
  type ChartServerMessage,
} from '@/services/ws/chartChannel';
import { RemoteChartChannelClient } from '@/services/ws/remoteChartChannel';
import type { ChartEntry } from './chartChannelTypes';
import type { ChartChannelClientLike } from './useChartChannel';

/** 다중 채널의 단일 ref */
export interface ChannelRef {
  /** chart-emitter 채널명 */
  name: string;
  /** 라인 표시 별칭. 미지정 시 name 사용 */
  alias?: string;
  /** 채널별 표시 필드 (미지정 시 패널 기본값) */
  display_field?: string;
  /** 라인 색상 지정 (미지정 시 팔레트) */
  color?: string;
}

/** 채널별 상태 */
export interface ChannelState {
  entries: ChartEntry[];
  status: ChartConnectionStatus;
  closedReason?: string;
  errorReason?: string;
}

export interface UseChartChannelsOptions {
  maxPoints?: number;
  wsBaseUrl?: string;
  createClient?: (opts: ChartChannelClientOptions) => ChartChannelClientLike;
}

export interface UseChartChannelsResult {
  /** 채널 이름 → 상태 */
  channels: Map<string, ChannelState>;
}

const DEFAULT_MAX_POINTS = 1000;

function deriveWsBaseUrl(): string {
  if (typeof window === 'undefined') return 'ws://localhost';
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}`;
}

function buildChannelUrl(base: string, channelName: string): string {
  const trimmed = base.replace(/\/+$/, '');
  return `${trimmed}/ws/chart/${channelName}`;
}

const EMPTY_STATE: ChannelState = { entries: [], status: 'connecting' };

/**
 * 여러 채널을 동시에 구독한다. refs 를 diff 하여 신규 채널은 connect,
 * 사라진 채널은 disconnect 한다. 각 채널은 자신의 entries / status 를 보유한다.
 *
 * - refs 변경 시 추가/삭제만 반영하고 기존 채널 구독은 유지
 * - 빈/공백 name 은 무시
 * - unmount 시 모든 client.disconnect()
 */
export function useChartChannels(
  refs: ChannelRef[],
  options: UseChartChannelsOptions = {},
): UseChartChannelsResult {
  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L07): 원격이면 SSE 스트림 프록시로
  // 채널을 구독한다(별도 WS 경로 미신설). 로컬은 기존 WS 직결 그대로.
  const target = useTargetContext();
  const remoteInstanceId = isRemoteTarget(target) ? target.instanceId : '';

  const [channels, setChannels] = useState<Map<string, ChannelState>>(
    () => new Map(),
  );

  const clientsRef = useRef<Map<string, ChartChannelClientLike>>(new Map());
  const maxPointsRef = useRef(options.maxPoints ?? DEFAULT_MAX_POINTS);
  maxPointsRef.current = options.maxPoints ?? DEFAULT_MAX_POINTS;

  const optionsRef = useRef(options);
  optionsRef.current = options;

  const validNames = refs
    .map((r) => r.name?.trim())
    .filter((n): n is string => !!n);
  // remoteInstanceId 가 바뀌면(로컬↔원격 전환) 전체 재구독을 유도한다.
  const namesKey = remoteInstanceId + '|' + validNames.join('\u0001');

  useEffect(() => {
    const next = new Set(validNames);
    const current = clientsRef.current;

    // 제거: 더 이상 없는 채널 disconnect
    for (const name of Array.from(current.keys())) {
      if (!next.has(name)) {
        current.get(name)?.disconnect();
        current.delete(name);
        setChannels((m) => {
          if (!m.has(name)) return m;
          const nm = new Map(m);
          nm.delete(name);
          return nm;
        });
      }
    }

    // 추가: 신규 채널 connect
    for (const name of next) {
      if (current.has(name)) continue;

      const baseUrl = optionsRef.current.wsBaseUrl ?? deriveWsBaseUrl();
      const url = buildChannelUrl(baseUrl, name);

      const onMessage = (msg: ChartServerMessage) => {
        setChannels((m) => {
          const prev = m.get(name) ?? EMPTY_STATE;
          const cap = maxPointsRef.current;
          let next: ChannelState = prev;
          if (msg.type === 'chart.backfill') {
            const incoming = msg.entries;
            const entries =
              incoming.length > cap ? incoming.slice(-cap) : incoming.slice();
            next = { ...prev, entries };
          } else if (msg.type === 'chart.append') {
            const arr = prev.entries.concat(msg.entry);
            const entries =
              arr.length > cap ? arr.slice(arr.length - cap) : arr;
            next = { ...prev, entries };
          } else if (msg.type === 'chart.closed') {
            next = { ...prev, closedReason: msg.reason };
          } else if (msg.type === 'chart.error') {
            next = { ...prev, errorReason: msg.reason };
          }
          if (next === prev) return m;
          const nm = new Map(m);
          nm.set(name, next);
          return nm;
        });
      };

      const onStatus = (s: ChartConnectionStatus, detail?: string) => {
        setChannels((m) => {
          const prev = m.get(name) ?? EMPTY_STATE;
          const next: ChannelState = { ...prev, status: s };
          if (s === 'closed' && detail) next.closedReason = detail;
          if (s === 'error' && detail) next.errorReason = detail;
          const nm = new Map(m);
          nm.set(name, next);
          return nm;
        });
      };

      // 초기 상태
      setChannels((m) => {
        if (m.has(name)) return m;
        const nm = new Map(m);
        nm.set(name, EMPTY_STATE);
        return nm;
      });

      const clientOpts: ChartChannelClientOptions = { url, onMessage, onStatus };
      // 우선순위: 명시 createClient(테스트 주입) > 원격 SSE > 로컬 WS.
      let client: ChartChannelClientLike;
      if (optionsRef.current.createClient) {
        client = optionsRef.current.createClient(clientOpts);
      } else if (remoteInstanceId) {
        client = new RemoteChartChannelClient(
          clientOpts,
          remoteChartStreamUrl(remoteInstanceId, name),
        ) as ChartChannelClientLike;
      } else {
        client = new ChartChannelClient(clientOpts) as ChartChannelClientLike;
      }
      current.set(name, client);
      client.connect();
    }
    // namesKey 만으로 diff (refs 의 alias/color 변경은 구독 자체에 영향 없음)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namesKey]);

  // unmount 시 모든 client 정리
  useEffect(() => {
    const map = clientsRef.current;
    return () => {
      for (const c of map.values()) c.disconnect();
      map.clear();
    };
  }, []);

  return { channels };
}
