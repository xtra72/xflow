// 모니터링 실시간 스트림 단일 구독 테스트.
//
// 핵심 계약: 소비자가 몇 개든 WS 핸들러는 한 벌만 걸리고, 마지막 소비자가 떠날 때
// 정확히 정리된다. 이 계약이 깨지면 대시보드에 모니터링 패널을 n개 올릴 때
// 로그 버퍼가 n벌 생긴다.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';

/** 핸들러를 기억하는 최소 WS 클라이언트 더블 */
function createClientDouble() {
  const handlers = new Map<string, Set<(data: unknown) => void>>();
  return {
    on(type: string, fn: (data: unknown) => void) {
      if (!handlers.has(type)) handlers.set(type, new Set());
      handlers.get(type)!.add(fn);
    },
    off(type: string, fn: (data: unknown) => void) {
      handlers.get(type)?.delete(fn);
    },
    /** 해당 타입에 걸린 핸들러 수 */
    count(type: string) {
      return handlers.get(type)?.size ?? 0;
    },
    emit(type: string, data: unknown) {
      for (const fn of handlers.get(type) ?? []) fn(data);
    },
  };
}

const clientRef = vi.hoisted(() => ({ current: null as ReturnType<typeof createClientDouble> | null }));

vi.mock('@/hooks', () => ({
  useWebSocket: () => ({ state: 'connected', client: clientRef.current }),
}));

import { useMonitorStream, resetMonitorStream } from './monitorStream';

/** 스트림 카운트를 표시하는 최소 소비자 */
function Consumer({ id }: { id: string }) {
  const { logsReceived, eventsReceived, logs } = useMonitorStream();
  return (
    <div
      data-testid={`consumer-${id}`}
      data-logs={logsReceived}
      data-events={eventsReceived}
      data-buffer={logs.length}
    />
  );
}

describe('monitorStream', () => {
  let client: ReturnType<typeof createClientDouble>;

  beforeEach(() => {
    resetMonitorStream();
    client = createClientDouble();
    clientRef.current = client;
  });

  it('소비자가 여럿이어도 WS 핸들러는 한 벌만 걸린다', () => {
    render(
      <>
        <Consumer id="a" />
        <Consumer id="b" />
        <Consumer id="c" />
      </>,
    );

    expect(client.count(WS_MESSAGE_TYPES.LOG_ENTRY)).toBe(1);
    expect(client.count(WS_MESSAGE_TYPES.SYSTEM_EVENT)).toBe(1);
    expect(client.count(WS_MESSAGE_TYPES.FLOW_METRICS)).toBe(1);
  });

  it('한 번 수신한 로그가 모든 소비자에게 한 번씩만 반영된다', () => {
    render(
      <>
        <Consumer id="a" />
        <Consumer id="b" />
      </>,
    );

    act(() => {
      client.emit(WS_MESSAGE_TYPES.LOG_ENTRY, { level: 'info', message: 'hello' });
    });

    // 소비자가 2개라도 버퍼에는 1건만 쌓여야 한다 (핸들러 중복의 직접 증거).
    expect(screen.getByTestId('consumer-a')).toHaveAttribute('data-buffer', '1');
    expect(screen.getByTestId('consumer-b')).toHaveAttribute('data-buffer', '1');
    expect(screen.getByTestId('consumer-a')).toHaveAttribute('data-logs', '1');
  });

  it('마지막 소비자가 떠날 때만 핸들러를 뗀다', () => {
    const { rerender, unmount } = render(
      <>
        <Consumer id="a" />
        <Consumer id="b" />
      </>,
    );
    expect(client.count(WS_MESSAGE_TYPES.LOG_ENTRY)).toBe(1);

    rerender(<Consumer id="a" />);
    expect(client.count(WS_MESSAGE_TYPES.LOG_ENTRY)).toBe(1);

    unmount();
    expect(client.count(WS_MESSAGE_TYPES.LOG_ENTRY)).toBe(0);
  });

  it('이벤트 수신 누적 카운트를 센다', () => {
    render(<Consumer id="a" />);

    act(() => {
      client.emit(WS_MESSAGE_TYPES.SYSTEM_EVENT, { type: 'error', message: 'boom' });
      client.emit(WS_MESSAGE_TYPES.SYSTEM_EVENT, { type: 'system' });
    });

    expect(screen.getByTestId('consumer-a')).toHaveAttribute('data-events', '2');
  });

  it('메시지가 없는 이벤트는 빈 문자열로 담아 렌더 시점 폴백에 맡긴다', () => {
    render(<Consumer id="a" />);

    act(() => {
      client.emit(WS_MESSAGE_TYPES.SYSTEM_EVENT, { type: 'system' });
    });

    // 스트림 모듈은 i18n 컨텍스트 밖이라 번역된 기본 문구를 넣을 수 없다.
    expect(screen.getByTestId('consumer-a')).toHaveAttribute('data-events', '1');
  });
});
