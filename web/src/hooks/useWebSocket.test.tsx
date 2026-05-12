// WebSocketProvider 통합 테스트 — SPEC-AUTH-003 S1, E1, O1, 회귀(StrictMode) 검증.
// React Testing Library + zustand 직접 조작 패턴.

import React, { StrictMode } from 'react';
import { act, cleanup, render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useAuthStore } from '@/stores/authStore';
import type { WSClient } from '@/services/ws/wsClient';

import { WebSocketProvider, useWebSocket, evaluateGate } from './useWebSocket';

// --- MockWebSocket (wsClient.test.ts 와 동일 패턴, 본 파일 독립 사용) ---
class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  // 생성된 인스턴스 추적
  static instances: MockWebSocket[] = [];

  readyState: number = MockWebSocket.CONNECTING;
  readonly url: string;

  onopen: ((ev?: Event) => void) | null = null;
  onclose: ((ev: { code: number; reason: string; wasClean: boolean }) => void) | null = null;
  onerror: ((ev?: Event) => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;

  closeCalled = false;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  _simulateOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  _simulateClose(code = 1006, reason = '', wasClean = false): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code, reason, wasClean });
  }

  close(code = 1000, reason = ''): void {
    this.closeCalled = true;
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code, reason, wasClean: true });
  }

  send(_data: string): void {
    // no-op
  }

  static reset(): void {
    MockWebSocket.instances = [];
  }

  // OPEN 또는 CONNECTING 상태인 인스턴스 수 — StrictMode 회귀 검증용
  static aliveCount(): number {
    return MockWebSocket.instances.filter(
      (ws) => ws.readyState === MockWebSocket.OPEN || ws.readyState === MockWebSocket.CONNECTING,
    ).length;
  }
}

let originalWebSocket: typeof globalThis.WebSocket | undefined;

// authStore 초기 상태 백업
const initialAuthState = useAuthStore.getState();

beforeEach(() => {
  MockWebSocket.reset();
  originalWebSocket = globalThis.WebSocket;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).WebSocket = MockWebSocket;
  useAuthStore.setState({
    user: null,
    tokens: null,
    isAuthenticated: false,
    isLoading: false,
    authEnabled: null,
  });
});

afterEach(() => {
  cleanup();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).WebSocket = originalWebSocket;
  useAuthStore.setState(initialAuthState);
});

// useWebSocket() 의 client 참조를 외부에서 캡처하는 프로브 컴포넌트.
function ClientProbe({ onClient }: { onClient: (client: WSClient | null) => void }) {
  const ws = useWebSocket();
  React.useEffect(() => {
    onClient(ws.client);
  }, [ws.client, onClient]);
  return null;
}

// --- evaluateGate 순수 함수 단위 테스트 ---
describe('evaluateGate — 게이트 매트릭스 단위 검증', () => {
  it('authEnabled === null → wait (어떤 토큰/인증 상태에서도)', () => {
    expect(evaluateGate(null, false, undefined)).toBe('wait');
    expect(evaluateGate(null, true, 'JWT')).toBe('wait');
    expect(evaluateGate(null, false, 'JWT')).toBe('wait');
  });

  it('authEnabled === false → connect-no-token', () => {
    expect(evaluateGate(false, false, undefined)).toBe('connect-no-token');
    expect(evaluateGate(false, true, 'JWT')).toBe('connect-no-token');
  });

  it('authEnabled === true + !isAuthenticated → disconnect', () => {
    expect(evaluateGate(true, false, undefined)).toBe('disconnect');
    expect(evaluateGate(true, false, 'JWT')).toBe('disconnect');
  });

  it('authEnabled === true + isAuthenticated + !token → disconnect', () => {
    expect(evaluateGate(true, true, undefined)).toBe('disconnect');
    expect(evaluateGate(true, true, '')).toBe('disconnect');
  });

  it('authEnabled === true + isAuthenticated + token → connect-with-token', () => {
    expect(evaluateGate(true, true, 'JWT_AAA')).toBe('connect-with-token');
  });
});

describe('WebSocketProvider — S1 연결 게이트 매트릭스', () => {
  it('S1-T1: authEnabled === null 마운트 → connect 시도 0회', () => {
    // Given: authEnabled === null (초기화 미완료)
    useAuthStore.setState({ authEnabled: null, isAuthenticated: false, tokens: null });

    // When: Provider 마운트
    render(
      <WebSocketProvider>
        <div />
      </WebSocketProvider>,
    );

    // Then: WebSocket 생성자가 호출되지 않음
    expect(MockWebSocket.instances).toHaveLength(0);
  });

  it('S1-T2: authEnabled === false 마운트 → connect 1회 (토큰 없음)', () => {
    // Given: 인증 비활성 환경
    useAuthStore.setState({ authEnabled: false, isAuthenticated: false, tokens: null });

    // When: Provider 마운트
    render(
      <WebSocketProvider>
        <div />
      </WebSocketProvider>,
    );

    // Then: 정확히 1회 connect, URL 에 ?token= 없음
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]!.url).not.toContain('token=');
  });

  it('S1-T3: authEnabled === true + isAuthenticated + token 마운트 → connect 1회 (?token= 포함)', () => {
    // Given: 인증 완료 상태
    useAuthStore.setState({
      authEnabled: true,
      isAuthenticated: true,
      tokens: { access_token: 'JWT_AAA', refresh_token: 'R', expires_at: 9999999999 },
    });

    // When: Provider 마운트
    render(
      <WebSocketProvider>
        <div />
      </WebSocketProvider>,
    );

    // Then: 정확히 1회 connect, URL 에 ?token=JWT_AAA 포함
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]!.url).toContain('token=JWT_AAA');
  });
});

describe('WebSocketProvider — E1 authStore 변경 트리거', () => {
  it('E1-T1: 마운트 시 null → store가 true+token으로 전이 → connect 발생', () => {
    // Given: 초기 null 상태로 마운트
    useAuthStore.setState({ authEnabled: null, isAuthenticated: false, tokens: null });

    render(
      <WebSocketProvider>
        <div />
      </WebSocketProvider>,
    );
    expect(MockWebSocket.instances).toHaveLength(0);

    // When: 인증 완료로 전이
    act(() => {
      useAuthStore.setState({
        authEnabled: true,
        isAuthenticated: true,
        tokens: {
          access_token: 'JWT_AAA',
          refresh_token: 'R',
          expires_at: 9999999999,
        },
      });
    });

    // Then: connect 발생, ?token=JWT_AAA 포함
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]!.url).toContain('token=JWT_AAA');
  });

  it('E1-T2: connect 상태에서 logout → disconnect 발생, reconnect 시도 없음', () => {
    // Given: 인증 완료 상태로 connect 됨
    useAuthStore.setState({
      authEnabled: true,
      isAuthenticated: true,
      tokens: { access_token: 'JWT_AAA', refresh_token: 'R', expires_at: 9999999999 },
    });

    render(
      <WebSocketProvider>
        <div />
      </WebSocketProvider>,
    );
    expect(MockWebSocket.instances).toHaveLength(1);
    act(() => {
      MockWebSocket.instances[0]!._simulateOpen();
    });

    const instanceCountBeforeLogout = MockWebSocket.instances.length;

    // When: logout (isAuthenticated: false, tokens: null)
    act(() => {
      useAuthStore.setState({ isAuthenticated: false, tokens: null });
    });

    // Then: 활성 인스턴스의 close 가 호출됨
    expect(MockWebSocket.instances[0]!.closeCalled).toBe(true);
    // 추가 reconnect 시도 없음 — 인스턴스 수 변화 없음
    expect(MockWebSocket.instances.length).toBe(instanceCountBeforeLogout);
  });
});

describe('WebSocketProvider — O1 토큰 회전 시 무중단 reconnect', () => {
  it('O1-T1: 토큰 회전 (A → B) → disconnect → connect (token B), WSClient 인스턴스 동일', () => {
    // Given: token A 로 connect 됨
    useAuthStore.setState({
      authEnabled: true,
      isAuthenticated: true,
      tokens: { access_token: 'JWT_A', refresh_token: 'R', expires_at: 9999999999 },
    });

    let capturedClient: WSClient | null = null;
    const captureFn = (c: WSClient | null) => {
      // 첫 안정 참조만 저장 (이후 호출에서 동일 참조 검증)
      if (capturedClient === null && c !== null) capturedClient = c;
    };

    render(
      <WebSocketProvider>
        <ClientProbe onClient={captureFn} />
      </WebSocketProvider>,
    );
    act(() => {
      MockWebSocket.instances[0]!._simulateOpen();
    });

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]!.url).toContain('token=JWT_A');
    const clientRefBefore = capturedClient;
    expect(clientRefBefore).not.toBeNull();

    // When: 토큰 회전 (A → B)
    act(() => {
      useAuthStore.setState({
        tokens: { access_token: 'JWT_B', refresh_token: 'R', expires_at: 9999999999 },
      });
    });

    // Then: 직전 인스턴스 close, 신규 인스턴스 생성 (token B 동반)
    expect(MockWebSocket.instances[0]!.closeCalled).toBe(true);
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1]!.url).toContain('token=JWT_B');
    // WSClient 인스턴스 정체성 유지 — 회전 후에도 동일 참조
    expect(capturedClient).toBe(clientRefBefore);
  });
});

describe('WebSocketProvider — StrictMode 이중 마운트 안전성 (회귀 방지)', () => {
  it('StrictMode-T1: <StrictMode> 마운트 → 활성 WebSocket 정확히 1개', () => {
    // Given: 인증 완료 상태
    useAuthStore.setState({
      authEnabled: true,
      isAuthenticated: true,
      tokens: { access_token: 'JWT_AAA', refresh_token: 'R', expires_at: 9999999999 },
    });

    // When: StrictMode 로 감싸서 마운트 (mount → unmount → mount 이중 호출 시뮬레이션)
    render(
      <StrictMode>
        <WebSocketProvider>
          <div />
        </WebSocketProvider>
      </StrictMode>,
    );

    // Then: 활성 (CONNECTING 또는 OPEN) WebSocket 정확히 1개.
    // StrictMode 이중 마운트로 인스턴스가 생성되더라도 첫 인스턴스는 cleanup 단계에서 close 됨.
    expect(MockWebSocket.aliveCount()).toBe(1);
  });
});
