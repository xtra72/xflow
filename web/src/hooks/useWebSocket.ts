// WebSocket connection management via React Context (singleton pattern).
// A single WSClient is shared across the entire component tree.

import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { createElement } from 'react';

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
 * WebSocketProvider: 컴포넌트 트리 전체에 단일 WSClient 인스턴스를 공유한다.
 * StrictMode 이중 마운트에도 안전하게 동작한다.
 */
export function WebSocketProvider({ children }: { children: ReactNode }) {
  const [client, setClient] = useState<WSClient | null>(null);
  const [state, setState] = useState<ConnectionState>('disconnected');

  useEffect(() => {
    const ws = createWSClient();

    ws.onStateChange((next) => {
      setState(next);
    });

    ws.connect();
    setClient(ws);

    return () => {
      ws.disconnect();
      setClient(null);
    };
  }, []);

  const connect = useCallback(() => {
    client?.connect();
  }, [client]);

  const disconnect = useCallback(() => {
    client?.disconnect();
  }, [client]);

  return createElement(
    WebSocketContext.Provider,
    {
      value: {
        client,
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
