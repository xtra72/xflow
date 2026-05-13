// WSClient 단위 테스트 — SPEC-AUTH-003 U1, UB1 검증.
// jsdom 환경에서 globalThis.WebSocket 을 직접 모킹하여 외부 의존성 없이 검증한다.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

import { createWSClient } from './wsClient';

// --- MockWebSocket ---
// 브라우저 WebSocket 의 최소 인터페이스를 흉내내는 직접 모킹.
// 각 테스트가 _simulateOpen/_simulateClose/_simulateError 로 라이프사이클을 제어한다.
class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  // 생성된 모든 인스턴스 추적 — 재연결 시도 횟수 검증에 사용.
  static instances: MockWebSocket[] = [];

  readyState: number = MockWebSocket.CONNECTING;
  readonly url: string;

  onopen: ((ev?: Event) => void) | null = null;
  onclose: ((ev: { code: number; reason: string; wasClean: boolean }) => void) | null = null;
  onerror: ((ev?: Event) => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;

  closeCalled = false;
  closeArgs: Array<[number?, string?]> = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  _simulateOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  _simulateError(): void {
    this.onerror?.();
  }

  _simulateClose(code = 1006, reason = '', wasClean = false): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code, reason, wasClean });
  }

  _simulateMessage(data: string): void {
    this.onmessage?.({ data });
  }

  // 브라우저 표준 close — 명시적 client-side 종료.
  close(code = 1000, reason = ''): void {
    this.closeCalled = true;
    this.closeArgs.push([code, reason]);
    this.readyState = MockWebSocket.CLOSED;
    // 클라이언트 측 close 는 wasClean: true 로 onclose 트리거.
    this.onclose?.({ code, reason, wasClean: true });
  }

  send(_data: string): void {
    // no-op (heartbeat ping 등 — 본 테스트에서 검증 안 함)
  }

  static reset(): void {
    MockWebSocket.instances = [];
  }
}

// 원본 WebSocket 백업용
let originalWebSocket: typeof globalThis.WebSocket | undefined;

beforeEach(() => {
  MockWebSocket.reset();
  originalWebSocket = globalThis.WebSocket;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).WebSocket = MockWebSocket;
  vi.useFakeTimers();
});

afterEach(() => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).WebSocket = originalWebSocket;
  vi.useRealTimers();
});

describe('WSClient — U1 핸드셰이크 토큰 전달 정책', () => {
  it('U1-T1: tokenGetter 미제공 시 ?token= 없이 연결한다', () => {
    // Given: tokenGetter 미제공 (backward compatibility)
    const client = createWSClient({ url: 'ws://localhost/ws' });

    // When: connect 호출
    client.connect();

    // Then: 정확히 1회 WebSocket 생성, URL 에 ?token= 없음
    expect(MockWebSocket.instances).toHaveLength(1);
    const ws = MockWebSocket.instances[0]!;
    expect(ws.url).toBe('ws://localhost/ws');
    expect(ws.url).not.toContain('token=');
    expect(client.getLastConnectAttemptHadToken()).toBe(false);
  });

  it('U1-T2: tokenGetter가 truthy 토큰 반환 시 ?token=<jwt>를 URL에 포함한다', () => {
    // Given: tokenGetter 가 유효 JWT 반환
    const tokenGetter = vi.fn(() => 'JWT_AAA');
    const client = createWSClient({ url: 'ws://localhost/ws', tokenGetter });

    // When: connect 호출
    client.connect();

    // Then: URL 에 ?token=JWT_AAA 포함, tokenGetter 1회 호출
    expect(MockWebSocket.instances).toHaveLength(1);
    const ws = MockWebSocket.instances[0]!;
    expect(ws.url).toBe('ws://localhost/ws?token=JWT_AAA');
    expect(tokenGetter).toHaveBeenCalledTimes(1);
    expect(client.getLastConnectAttemptHadToken()).toBe(true);
  });

  it('U1-T3: 매 connect 시도마다 tokenGetter를 재호출한다', () => {
    // Given: 호출마다 다른 토큰을 반환하는 tokenGetter (토큰 회전 시뮬레이션)
    const tokens = ['JWT_A', 'JWT_B', 'JWT_C'];
    let callCount = 0;
    const tokenGetter = vi.fn(() => tokens[callCount++]);
    const client = createWSClient({ url: 'ws://localhost/ws', tokenGetter });

    // When: 3회 connect (각 사이 disconnect)
    client.connect();
    expect(MockWebSocket.instances[0]!.url).toBe('ws://localhost/ws?token=JWT_A');

    client.disconnect();
    client.connect();
    expect(MockWebSocket.instances[1]!.url).toBe('ws://localhost/ws?token=JWT_B');

    client.disconnect();
    client.connect();
    expect(MockWebSocket.instances[2]!.url).toBe('ws://localhost/ws?token=JWT_C');

    // Then: tokenGetter 가 매번 재호출됨 (총 3회)
    expect(tokenGetter).toHaveBeenCalledTimes(3);
  });
});

describe('WSClient — UB1 인증 실패 시 무한 재연결 차단', () => {
  it('UB1-T1: code 1006 + lastConnectAttemptHadToken=true + !connectionEverEstablished → reconnect 안 함, lastFailureWasAuth=true', () => {
    // Given: 유효 토큰으로 connect 시도 (서버가 토큰을 거절하는 시나리오)
    const tokenGetter = vi.fn(() => 'INVALID_JWT');
    const client = createWSClient({ url: 'ws://localhost/ws', tokenGetter });
    client.connect();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(client.getLastConnectAttemptHadToken()).toBe(true);
    expect(client.getConnectionEverEstablished()).toBe(false);

    // When: 서버가 401 로 거절 → 브라우저는 1006/abnormal closure 로 관찰
    const ws = MockWebSocket.instances[0]!;
    ws._simulateError();
    ws._simulateClose(1006, '', false);

    // Then: 인증 실패로 판정 → reconnect 시도 없음
    expect(client.getLastFailureWasAuth()).toBe(true);
    expect(client.getState()).toBe('disconnected');

    // 음성 검증: 충분한 시간 경과해도 추가 WebSocket 생성 없음 (재시도 차단)
    vi.advanceTimersByTime(60000);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  it('UB1-T2: code 1006 + connectionEverEstablished=true → 기존 reconnect 정책 적용', () => {
    // Given: 정상 토큰으로 connect, 한 번 onopen 발생 후 네트워크 장애로 close
    const tokenGetter = vi.fn(() => 'VALID_JWT');
    const client = createWSClient({ url: 'ws://localhost/ws', tokenGetter });
    client.connect();

    const ws = MockWebSocket.instances[0]!;
    ws._simulateOpen();
    expect(client.getConnectionEverEstablished()).toBe(true);
    expect(client.getState()).toBe('connected');

    // When: 네트워크 장애로 1006 close (직접 통신 후의 비정상 종료)
    ws._simulateClose(1006, '', false);

    // Then: 인증 실패로 판정되지 않음 → 기존 reconnect 정책 적용 (reconnecting 상태)
    expect(client.getLastFailureWasAuth()).toBe(false);
    expect(client.getState()).toBe('reconnecting');

    // 첫 backoff(1000ms) 경과 후 재연결 시도
    vi.advanceTimersByTime(1000);
    expect(MockWebSocket.instances.length).toBeGreaterThan(1);
  });

  it('UB1-T3: code 1006 + lastConnectAttemptHadToken=false (basic_auth: true, no-token) → reconnect 안 함, lastFailureWasAuth=true', () => {
    // Given: tokenGetter 없이 connect → basic_auth: true 서버에 토큰 미동반 시나리오
    const client = createWSClient({ url: 'ws://localhost/ws' });
    client.connect();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(client.getLastConnectAttemptHadToken()).toBe(false);

    // When: 서버 401 → 1006 close (onopen 없이 즉시 종료)
    const ws = MockWebSocket.instances[0]!;
    ws._simulateError();
    ws._simulateClose(1006, '', false);

    // Then: 토큰 없는 시도였더라도 onopen 미발생 + 1006 조합으로 인증 실패 판정
    expect(client.getLastFailureWasAuth()).toBe(true);
    expect(client.getState()).toBe('disconnected');

    // 음성 검증: 추가 재시도 없음
    vi.advanceTimersByTime(60000);
    expect(MockWebSocket.instances).toHaveLength(1);
  });
});
