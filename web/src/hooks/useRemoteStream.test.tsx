// useRemoteStream 단위 테스트 (SPEC-REMOTE-001 M8, REQ-J08/J08b).
//
// 가짜 EventSource 를 주입하여 검증한다:
//   - data: 프레임 파싱 → data 노출, status='open'.
//   - event: end / event: error → status='closed' + shouldFallback=true (폴백).
//   - 전송 오류(onerror) 백오프 재시도 → 한도 초과 시 폴백.
//   - 언마운트 teardown → close 호출(누수 없음).
//   - enabled=false / url 미지정 → 비활성.

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useRemoteStream, type EventSourceLike } from './useRemoteStream';

// authStore mock — 토큰은 본 테스트의 관심사가 아니므로 비운다.
vi.mock('@/stores', () => ({
  getAuthState: () => ({ tokens: { access_token: 'test-token' } }),
}));

/** 테스트용 가짜 EventSource — 수동으로 이벤트를 발사한다. */
class FakeEventSource implements EventSourceLike {
  static instances: FakeEventSource[] = [];
  url: string;
  closed = false;
  onerror: ((ev: Event) => void) | null = null;
  onopen: ((ev: Event) => void) | null = null;
  private listeners = new Map<string, ((ev: MessageEvent) => void)[]>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (ev: MessageEvent) => void): void {
    const arr = this.listeners.get(type) ?? [];
    arr.push(listener);
    this.listeners.set(type, arr);
  }

  close(): void {
    this.closed = true;
  }

  // --- 테스트 헬퍼 ---
  emitOpen(): void {
    this.onopen?.(new Event('open'));
  }
  emit(type: string, data: string): void {
    for (const l of this.listeners.get(type) ?? []) {
      l({ data } as MessageEvent);
    }
  }
  emitError(): void {
    this.onerror?.(new Event('error'));
  }
}

function factory(url: string): EventSourceLike {
  return new FakeEventSource(url);
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useRemoteStream — 정상 스트림', () => {
  it('data: 프레임을 파싱해 노출하고 status=open 으로 전이한다', () => {
    const { result } = renderHook(() =>
      useRemoteStream<{ temp: number }>('/sse/state', {
        eventSourceFactory: factory,
      }),
    );

    const es = FakeEventSource.instances[0]!;
    expect(result.current.status).toBe('connecting');

    act(() => es.emitOpen());
    expect(result.current.status).toBe('open');

    act(() => es.emit('message', JSON.stringify({ temp: 22 })));
    expect(result.current.data).toEqual({ temp: 22 });
    expect(result.current.shouldFallback).toBe(false);
  });

  it('현재 토큰을 ?token= 쿼리로 부착한다', () => {
    renderHook(() =>
      useRemoteStream('/sse/state', { eventSourceFactory: factory }),
    );
    expect(FakeEventSource.instances[0]!.url).toBe('/sse/state?token=test-token');
  });

  it('손상된 data 프레임은 무시한다', () => {
    const { result } = renderHook(() =>
      useRemoteStream('/sse/state', { eventSourceFactory: factory }),
    );
    const es = FakeEventSource.instances[0]!;
    act(() => es.emit('message', 'not-json{'));
    expect(result.current.data).toBeUndefined();
  });
});

describe('useRemoteStream — 터미널 이벤트 → 폴백', () => {
  it('event: end 수신 시 closed + shouldFallback=true 로 전이하고 close 한다', () => {
    const { result } = renderHook(() =>
      useRemoteStream('/sse/state', { eventSourceFactory: factory }),
    );
    const es = FakeEventSource.instances[0]!;
    act(() => es.emitOpen());
    act(() => es.emit('end', 'stream ended'));
    expect(result.current.status).toBe('closed');
    expect(result.current.shouldFallback).toBe(true);
    expect(es.closed).toBe(true);
  });

  it('event: error 수신 시 폴백으로 전이한다', () => {
    const { result } = renderHook(() =>
      useRemoteStream('/sse/state', { eventSourceFactory: factory }),
    );
    const es = FakeEventSource.instances[0]!;
    act(() => es.emit('error', 'subscribe failed'));
    expect(result.current.status).toBe('closed');
    expect(result.current.shouldFallback).toBe(true);
  });
});

describe('useRemoteStream — 전송 오류 백오프', () => {
  it('onerror 는 백오프로 재시도하고 한도 초과 시 폴백한다', () => {
    const { result } = renderHook(() =>
      useRemoteStream('/sse/state', {
        eventSourceFactory: factory,
        maxRetries: 2,
        baseBackoffMs: 100,
      }),
    );

    // 1차 오류 → 재시도 1.
    act(() => FakeEventSource.instances[0]!.emitError());
    expect(result.current.shouldFallback).toBe(false);
    act(() => vi.advanceTimersByTime(100));
    expect(FakeEventSource.instances.length).toBe(2);

    // 2차 오류 → 재시도 2.
    act(() => FakeEventSource.instances[1]!.emitError());
    act(() => vi.advanceTimersByTime(200));
    expect(FakeEventSource.instances.length).toBe(3);

    // 3차 오류 → 한도(2) 초과 → 폴백.
    act(() => FakeEventSource.instances[2]!.emitError());
    expect(result.current.status).toBe('closed');
    expect(result.current.shouldFallback).toBe(true);
  });
});

describe('useRemoteStream — 라이프사이클', () => {
  it('언마운트 시 EventSource 를 close 한다 (teardown, REQ-J08b)', () => {
    const { unmount } = renderHook(() =>
      useRemoteStream('/sse/state', { eventSourceFactory: factory }),
    );
    const es = FakeEventSource.instances[0]!;
    unmount();
    expect(es.closed).toBe(true);
  });

  it('enabled=false 면 연결하지 않고 idle 이다', () => {
    const { result } = renderHook(() =>
      useRemoteStream('/sse/state', {
        enabled: false,
        eventSourceFactory: factory,
      }),
    );
    expect(FakeEventSource.instances.length).toBe(0);
    expect(result.current.status).toBe('idle');
  });

  it('url 미지정이면 비활성이다', () => {
    const { result } = renderHook(() =>
      useRemoteStream(undefined, { eventSourceFactory: factory }),
    );
    expect(FakeEventSource.instances.length).toBe(0);
    expect(result.current.status).toBe('idle');
  });
});
