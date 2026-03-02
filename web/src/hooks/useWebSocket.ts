// WebSocket connection management hook.

import { useCallback, useEffect, useRef, useState } from 'react';

import { type ConnectionState, type WSClient, createWSClient } from '@/services/ws/wsClient';

/**
 * Manages a singleton WSClient for the component tree.
 * Auto-connects on mount and disconnects on unmount.
 */
export function useWebSocket() {
  const clientRef = useRef<WSClient | null>(null);
  const [state, setState] = useState<ConnectionState>('disconnected');

  useEffect(() => {
    const client = createWSClient();
    clientRef.current = client;

    client.onStateChange((next) => {
      setState(next);
    });

    client.connect();

    return () => {
      client.disconnect();
      clientRef.current = null;
    };
  }, []);

  const connect = useCallback(() => {
    clientRef.current?.connect();
  }, []);

  const disconnect = useCallback(() => {
    clientRef.current?.disconnect();
  }, []);

  return {
    state,
    client: clientRef.current,
    connect,
    disconnect,
  };
}
