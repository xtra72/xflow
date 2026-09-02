// useLine 훅 테스트 (SPEC-XSFM-LINE-001 Module 6, M6 / AC-7.1).
//
// agentService.execAgent 를 mock 하여 라인 명령이 표준 exec 계약대로 인자를 `params`
// 아래에 중첩해 전송하는지, list_lines 응답을 Order 오름차순으로 파싱하는지, 코드 포맷·
// ErrLineInUse 판별 헬퍼가 올바른지 검증한다(useStation.test.tsx 계약 미러).

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());
// 읽기 전용 명령은 queryAgent(POST /agents/{id}/query)로 나간다 — exec 와 별도 스파이.
const queryAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
  queryAgent: queryAgentMock,
}));

import {
  isLineInUseError,
  isValidLineCode,
  useAddLine,
  useLines,
  useRemoveLine,
} from './useLine';

let queryClient: QueryClient;

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  execAgentMock.mockReset();
  queryAgentMock.mockReset();
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});

describe('useLines', () => {
  it('list_lines 응답의 lines 를 파싱한다 (AC-1.1)', async () => {
    queryAgentMock.mockResolvedValueOnce({
      status: 'ok',
      lines: [{ code: 'line_2', name: '2호선', order: 1 }],
    });

    const { result } = renderHook(() => useLines('agent-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(queryAgentMock).toHaveBeenCalledWith('agent-1', { command: 'list_lines' });
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0]).toEqual({ code: 'line_2', name: '2호선', order: 1 });
  });

  it('lines 를 Order 오름차순으로 방어적 정렬한다 (AC-1.3)', async () => {
    queryAgentMock.mockResolvedValueOnce({
      status: 'ok',
      lines: [
        { code: 'line_2', name: '2호선', order: 2 },
        { code: 'line_1', name: '1호선', order: 1 },
      ],
    });

    const { result } = renderHook(() => useLines('agent-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.map((l) => l.code)).toEqual(['line_1', 'line_2']);
  });

  it('lines 누락 시 빈 배열을 반환한다(빈 라인 레지스트리, AC-1.5)', async () => {
    queryAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useLines('agent-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});

describe('useAddLine', () => {
  it('add_line 인자를 params 아래에 중첩해 전송한다 (AC-7.1)', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useAddLine('agent-1'), { wrapper });
    result.current.mutate({ code: 'line_2', name: '2호선', order: 1 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_line',
      params: { code: 'line_2', name: '2호선', order: 1 },
    });
  });

  it('order 생략 시 params 에서 order 를 omit 한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useAddLine('agent-1'), { wrapper });
    result.current.mutate({ code: 'line_9', name: '9호선' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_line',
      params: { code: 'line_9', name: '9호선' },
    });
  });
});

describe('useRemoveLine', () => {
  it('remove_line 을 code 를 params 로 전송한다 (AC-1.7)', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useRemoveLine('agent-1'), { wrapper });
    result.current.mutate('line_9');

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'remove_line',
      params: { code: 'line_9' },
    });
  });

  it('참조 역사 존재 시 백엔드 ErrLineInUse 를 그대로 전파한다 (AC-1.8)', async () => {
    execAgentMock.mockRejectedValueOnce(
      new Error('xsfm: line is in use (referenced by one or more stations)'),
    );

    const { result } = renderHook(() => useRemoveLine('agent-1'), { wrapper });
    result.current.mutate('line_2');

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(isLineInUseError(result.current.error)).toBe(true);
  });
});

describe('isValidLineCode', () => {
  it('통일 포맷(^[a-z0-9][a-z0-9_-]*$)을 만족하는 코드만 허용한다 (AC-1.10)', () => {
    // 적합.
    expect(isValidLineCode('line_2')).toBe(true);
    expect(isValidLineCode('st01')).toBe(true);
    expect(isValidLineCode('a-b_c')).toBe(true);
    expect(isValidLineCode('2')).toBe(true);
    // 비적합: 공백 / 대문자 / 선두 언더스코어 / 한글 / 빈 문자열.
    expect(isValidLineCode('2 호선')).toBe(false);
    expect(isValidLineCode('Line2')).toBe(false);
    expect(isValidLineCode('_pump')).toBe(false);
    expect(isValidLineCode('2호선')).toBe(false);
    expect(isValidLineCode('')).toBe(false);
  });
});

describe('isLineInUseError', () => {
  it('메시지에 "in use" 를 포함하는 Error 만 ErrLineInUse 로 판별한다', () => {
    expect(isLineInUseError(new Error('xsfm: line is in use (referenced ...)'))).toBe(true);
    expect(isLineInUseError(new Error('xsfm: line not found'))).toBe(false);
    expect(isLineInUseError('in use')).toBe(false); // 문자열은 Error 가 아님.
    expect(isLineInUseError(null)).toBe(false);
  });
});
