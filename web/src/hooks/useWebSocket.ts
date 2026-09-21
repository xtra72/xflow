// WebSocket connection management via React Context (singleton pattern).
// A single WSClient is shared across the entire component tree.
//
// SPEC-AUTH-003: authStore 상태에 의해 connect/wait/disconnect 가 결정된다.
// - S1: authEnabled × isAuthenticated × tokens.access_token 매트릭스 평가
// - E1: useAuthStore.subscribe() 콜백으로 상태 변경 시마다 게이트 재평가
// - O1: 토큰 회전 (A → B) 시 인스턴스 정체성 유지하며 reconnect

import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { createElement } from 'react';

import { useAuthStore } from '@/stores/authStore';
import { getCurrentUser } from '@/services/api/authService';
import { APIError } from '@/types/api';
import { type ConnectionState, type WSClient, createWSClient } from '@/services/ws/wsClient';

interface WebSocketContextValue {
  client: WSClient | null;
  state: ConnectionState;
  connect: () => void;
  disconnect: () => void;
}

const NOOP = () => {};

const DEFAULT_VALUE: WebSocketContextValue = {
  client: null,
  state: 'disconnected',
  connect: NOOP,
  disconnect: NOOP,
};

const WebSocketContext = createContext<WebSocketContextValue>(DEFAULT_VALUE);

/**
 * SPEC-AUTH-003 S1: 연결 게이트 매트릭스의 행동 분류.
 * - wait: connect/disconnect 모두 보류 (아직 결정할 수 없음)
 * - connect-no-token: 토큰 없이 즉시 connect (auth-disabled 환경)
 * - connect-with-token: ?token=<jwt> 동반 connect (인증 완료)
 * - disconnect: 활성 연결 종료, 자동 재시도 큐 비움
 */
export type GateAction = 'wait' | 'connect-no-token' | 'connect-with-token' | 'disconnect';

/**
 * SPEC-AUTH-003 S1: authStore 상태 조합을 단일 게이트 행동으로 환원하는 순수 함수.
 * 테스트 가능성을 위해 export 한다.
 */
export function evaluateGate(
  authEnabled: boolean | null,
  isAuthenticated: boolean,
  accessToken: string | undefined,
): GateAction {
  // authEnabled === null: 서버 인증 활성화 여부 미확인 — 어떤 connect 시도도 금지
  if (authEnabled === null) return 'wait';
  // authEnabled === false: 인증 비활성 환경 — 토큰 없이 즉시 connect
  if (authEnabled === false) return 'connect-no-token';
  // 이하 authEnabled === true 분기
  if (!isAuthenticated) return 'disconnect';
  if (!accessToken) return 'disconnect';
  return 'connect-with-token';
}

/**
 * WS 가 인증 실패로 끊긴 것 같을 때, 정말 세션이 끝났는지 REST 로 확인한다.
 *
 * # 왜 확인이 필요한가
 *
 * wsClient 의 인증 실패 판정은 `code 1006 + 비정상 종료 + onopen 미발생` 이라는
 * **정황**이다. 같은 정황은 서버 다운 · 프록시 거부 · 오프라인에서도 만들어진다.
 * 그 정황만 믿고 로그아웃하면 네트워크가 잠깐 끊긴 사용자를 로그인 화면으로
 * 내쫓는다 — 고치려는 것보다 나쁜 결함이다.
 *
 * 그래서 `/auth/me` 를 한 번 물어 답을 받는다. 이 요청은 갱신 인터셉터를 지나므로
 * 세 갈래가 각자 옳게 끝난다.
 *   - 토큰만 만료 + 갱신 가능 → 인터셉터가 조용히 갱신한다. 새 토큰이 스토어에
 *     들어가면 게이트가 토큰 회전으로 보고 WS 를 다시 붙인다.
 *   - 세션이 정말 끝남(401, 갱신 불가) → 인터셉터가 이미 logout 했다. 여기서
 *     한 번 더 부르는 것은 멱등이며, 인터셉터가 닿지 못한 경로를 덮는다.
 *   - 네트워크/서버 문제(401 아님) → 아무것도 하지 않는다. 세션은 건드리지 않는다.
 *
 * WS 자체의 재연결은 여기서 손대지 않는다 — 인증 실패 정황 뒤의 재연결 정책은
 * 이 결함과 별개의 문제이고, 섣불리 다시 붙이면 연결 실패 루프가 된다.
 */
async function verifySessionAfterSuspectedAuthFailure(): Promise<void> {
  try {
    await getCurrentUser();
  } catch (err) {
    if (err instanceof APIError && err.status === 401) {
      useAuthStore.getState().logout();
    }
    // 그 밖의 실패는 세션 판정의 근거가 되지 못한다 — 그대로 둔다.
  }
}

/**
 * WebSocketProvider: 컴포넌트 트리 전체에 단일 WSClient 인스턴스를 공유한다.
 * StrictMode 이중 마운트에도 안전하게 동작한다.
 *
 * SPEC-AUTH-003:
 * - useRef 로 WSClient singleton 보존 (StrictMode + 토큰 회전 모두 안전)
 * - useAuthStore.subscribe() 로 store 변경마다 게이트 재평가
 * - 토큰 회전 시 disconnect → connect 순으로 인스턴스 정체성 유지
 */
export function WebSocketProvider({ children }: { children: ReactNode }) {
  // SPEC-AUTH-003: useRef 로 WSClient singleton 보존 — StrictMode 이중 마운트와
  // 토큰 회전 모두에서 동일 참조 유지. lazy 초기화는 useEffect 내부에서 수행.
  const wsRef = useRef<WSClient | null>(null);
  // 직전 토큰 값 — 회전 (A → B) 감지에 사용.
  const previousTokenRef = useRef<string | undefined>(undefined);
  // 직전에 적용한 게이트 행동 — idempotent 호출 회피용.
  const previousActionRef = useRef<GateAction | null>(null);

  const [state, setState] = useState<ConnectionState>('disconnected');

  useEffect(() => {
    // SPEC-AUTH-003 U1: tokenGetter 는 connect 시점마다 호출되어 최신 토큰을 반영.
    if (!wsRef.current) {
      wsRef.current = createWSClient({
        tokenGetter: () => useAuthStore.getState().tokens?.access_token,
      });
      wsRef.current.onStateChange((next) => {
        setState(next);
        // 인증 실패 정황으로 끊긴 경우에만 확인한다(정황 → 확인 → 판정).
        if (next === 'disconnected' && wsRef.current?.getLastFailureWasAuth()) {
          void verifySessionAfterSuspectedAuthFailure();
        }
      });
    }
    const ws = wsRef.current;

    /**
     * SPEC-AUTH-003 S1/E1/O1: 현재 store 스냅샷을 읽어 게이트 행동을 결정하고 적용한다.
     * 토큰 회전 (truthy A → truthy B) 도 본 함수에서 함께 처리한다.
     */
    const applyGate = () => {
      const snapshot = useAuthStore.getState();
      const currentToken = snapshot.tokens?.access_token;
      const action = evaluateGate(snapshot.authEnabled, snapshot.isAuthenticated, currentToken);

      const prevAction = previousActionRef.current;
      const prevToken = previousTokenRef.current;

      // SPEC-AUTH-003 O1: 토큰 회전 (A → B 둘 다 truthy 이며 값이 다름) → 인스턴스 유지 reconnect.
      const isTokenRotation =
        action === 'connect-with-token' &&
        prevAction === 'connect-with-token' &&
        !!prevToken &&
        !!currentToken &&
        prevToken !== currentToken;

      if (isTokenRotation) {
        ws.disconnect();
        ws.connect();
      } else if (action !== prevAction) {
        // 행동 전이 — connect/disconnect 명시적 호출
        if (action === 'connect-no-token' || action === 'connect-with-token') {
          ws.connect();
        } else if (action === 'disconnect') {
          ws.disconnect();
        }
        // 'wait' 는 명시적 no-op
      }
      // action === prevAction (회전 아님) → idempotent no-op

      previousActionRef.current = action;
      previousTokenRef.current = currentToken;
    };

    // 마운트 직후 1회 동기 평가
    applyGate();
    // store 변경 시마다 재평가
    const unsubscribe = useAuthStore.subscribe(applyGate);

    return () => {
      unsubscribe();
      ws.disconnect();
      previousActionRef.current = null;
      previousTokenRef.current = undefined;
      // SPEC-AUTH-003: wsRef.current 는 null 화하지 않는다 — singleton 정체성 유지.
    };
  }, []);

  const connect = useCallback(() => {
    wsRef.current?.connect();
  }, []);

  const disconnect = useCallback(() => {
    wsRef.current?.disconnect();
  }, []);

  return createElement(
    WebSocketContext.Provider,
    {
      value: {
        client: wsRef.current,
        state,
        connect,
        disconnect,
      },
    },
    children,
  );
}

/**
 * 컴포넌트 트리 내 공유 WSClient에 접근하는 훅.
 * Provider 바깥에서 호출해도 안전한 기본값을 반환한다.
 */
export function useWebSocket(): WebSocketContextValue {
  return useContext(WebSocketContext);
}
