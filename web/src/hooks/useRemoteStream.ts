// useRemoteStream — 원격 라이브 스트림(SSE) 소비 훅 (SPEC-REMOTE-001 M8, 그룹 J,
// REQ-J08/J08b).
//
// 서버는 노드의 실시간 데이터(device.state / agent.stats / agent.series)를 SSE 로
// 브라우저에 중계한다(remote_stream.go). 본 훅은 그 SSE 연결을 캡슐화한다:
//
//   - 토큰: EventSource 는 헤더를 설정할 수 없으므로 JWT 를 `?token=` 쿼리로
//     운반한다(authStore.access_token → remoteService.remoteStreamUrl). 토큰
//     변경 시(refresh) 재구독한다.
//   - 프레임: `data: <redacted-json>` 이벤트를 파싱해 최신 payload 로 노출한다.
//   - 터미널: `event: end` / `event: error` 수신 또는 onerror 시 연결을 닫고
//     status 를 'closed' 로 전이한다. 그러면 호출자(타깃 훅)는 폴링 폴백으로
//     전환한다(REQ-J08 — 폴링은 폴백).
//   - 재연결/백오프: 일시적 onerror(터미널 이벤트 없이 끊김)는 지수 백오프로
//     제한된 횟수만 재시도한다. 한도 초과 시 'closed' 로 전이해 폴백을 유도한다.
//   - teardown: 언마운트/비활성/URL 변경 시 EventSource 를 close 한다(REQ-J08b —
//     브라우저 종료 → 노드 구독 해제 전파, 누수 없음).
//
// READ-ONLY: 본 훅은 데이터를 수신만 하며 어떤 변경도 수행하지 않는다(REQ-J03).

import { useEffect, useRef, useState } from 'react';

import { getAuthState } from '@/stores';

/** 스트림 연결 상태. */
export type RemoteStreamStatus = 'idle' | 'connecting' | 'open' | 'closed';

/** useRemoteStream 반환 형태. */
export interface RemoteStreamState<T> {
  /** 최근 수신한 redacted payload(미수신 시 undefined). */
  data: T | undefined;
  /** 현재 연결 상태. */
  status: RemoteStreamStatus;
  /**
   * 스트림이 사용 불가(터미널/재시도 한도 초과)하여 폴링 폴백이 필요한지 여부.
   * 호출자는 이 값이 true 면 폴링 쿼리를 활성화한다(REQ-J08 — 폴링 폴백).
   */
  shouldFallback: boolean;
}

/** useRemoteStream 옵션. */
export interface RemoteStreamOptions {
  /** false 면 연결하지 않는다(예: 비활성 탭/로컬 타깃). 기본 true. */
  enabled?: boolean;
  /** 최대 재연결 시도 횟수(초과 시 폴백). 기본 3. */
  maxRetries?: number;
  /** 백오프 기본 간격(ms). 기본 1000. */
  baseBackoffMs?: number;
  /**
   * EventSource 생성자 주입(테스트용). 미지정 시 전역 EventSource 를 사용한다.
   */
  eventSourceFactory?: (url: string) => EventSourceLike;
}

/** 테스트 가능성을 위한 최소 EventSource 인터페이스. */
export interface EventSourceLike {
  addEventListener(type: string, listener: (ev: MessageEvent) => void): void;
  close(): void;
  onerror: ((ev: Event) => void) | null;
  onopen: ((ev: Event) => void) | null;
}

/** access_token 을 authStore 에서 읽는다(없으면 undefined). */
function currentToken(): string | undefined {
  return getAuthState().tokens?.access_token ?? undefined;
}

/**
 * 원격 라이브 스트림을 구독한다.
 *
 * @param url - remoteService.remoteStreamUrl 로 생성한 SSE URL(token 미포함 —
 *   본 훅이 현재 토큰을 부착한다). 빈/undefined 면 비활성.
 * @param options - 활성 여부·재시도·백오프·EventSource 팩토리.
 */
export function useRemoteStream<T = unknown>(
  url: string | undefined,
  options: RemoteStreamOptions = {},
): RemoteStreamState<T> {
  const {
    enabled = true,
    maxRetries = 3,
    baseBackoffMs = 1000,
    eventSourceFactory,
  } = options;

  const [data, setData] = useState<T | undefined>(undefined);
  const [status, setStatus] = useState<RemoteStreamStatus>('idle');
  const [shouldFallback, setShouldFallback] = useState(false);

  // 재시도 카운터·타이머·현재 EventSource 는 effect 간 유지된다.
  const retryRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const active = enabled && !!url;

  useEffect(() => {
    if (!active) {
      setStatus('idle');
      setShouldFallback(false);
      return;
    }

    let source: EventSourceLike | null = null;
    let disposed = false;
    retryRef.current = 0;

    const clearTimer = (): void => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    const teardown = (): void => {
      clearTimer();
      if (source) {
        source.close();
        source = null;
      }
    };

    // 터미널 종료: 연결을 닫고 폴백을 유도한다(REQ-J08).
    const terminate = (): void => {
      teardown();
      if (disposed) return;
      setStatus('closed');
      setShouldFallback(true);
    };

    const connect = (): void => {
      if (disposed) return;
      setStatus('connecting');
      // 매 연결마다 최신 토큰을 부착한다(refresh 반영).
      const token = currentToken();
      const fullUrl = token
        ? `${url}${url!.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}`
        : url!;

      const factory =
        eventSourceFactory ??
        ((u: string) => new EventSource(u) as unknown as EventSourceLike);
      source = factory(fullUrl);

      source.onopen = () => {
        if (disposed) return;
        retryRef.current = 0;
        setStatus('open');
        setShouldFallback(false);
      };

      // data: <json> — 정상 프레임.
      source.addEventListener('message', (ev: MessageEvent) => {
        if (disposed) return;
        const parsed = parseFrame<T>(ev.data);
        if (parsed !== undefined) setData(parsed);
      });

      // event: end — 노드가 스트림 종료(오프라인/터미널). 폴백 전환.
      source.addEventListener('end', () => terminate());
      // event: error — 구독 실패 통지(헤더 전송 후). 폴백 전환.
      source.addEventListener('error', () => terminate());

      // 전송 계층 오류(연결 끊김 등). 백오프 재시도 후 한도 초과 시 폴백.
      source.onerror = () => {
        if (disposed) return;
        if (source) {
          source.close();
          source = null;
        }
        if (retryRef.current >= maxRetries) {
          terminate();
          return;
        }
        const delay = baseBackoffMs * 2 ** retryRef.current;
        retryRef.current += 1;
        setStatus('connecting');
        clearTimer();
        timerRef.current = setTimeout(connect, delay);
      };
    };

    setShouldFallback(false);
    setData(undefined);
    connect();

    return () => {
      disposed = true;
      teardown();
    };
    // url/active/옵션 변경 시 재구독. 토큰은 connect 내부에서 매번 읽는다.
  }, [active, url, maxRetries, baseBackoffMs, eventSourceFactory]);

  return { data, status, shouldFallback };
}

/** SSE data 페이로드(JSON 문자열)를 파싱한다. 손상 시 undefined. */
function parseFrame<T>(raw: unknown): T | undefined {
  if (typeof raw !== 'string' || raw.length === 0) return undefined;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return undefined;
  }
}
