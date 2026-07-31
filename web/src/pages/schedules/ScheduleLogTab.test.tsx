// SPEC-SCHEDULE-VIEW-001 M6 — ScheduleLogTab.
// correlation_id 병합(AC-16/AC-3): fire+result → 1행 / fire-only → 결과 컬럼 없음(AC-15) /
// 필터 입력이 쿼리 파라미터로 전달(AC-10) / 빈 상태(에러 아님, AC-10) /
// actor≠declared 불일치 가시성(RD-6).
//
// useScheduleLogs 훅만 스텁하고 mergeScheduleLogs(순수 함수)는 실제를 사용해
// 병합 로직을 컴포넌트와 함께 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { ScheduleLogRecord } from '@/types/schedule';

const logs = vi.hoisted(() => ({
  value: {
    data: [] as ScheduleLogRecord[],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  },
}));
const useScheduleLogsMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useScheduleLogs', () => ({
  useScheduleLogs: (params: unknown) => {
    useScheduleLogsMock(params);
    return logs.value;
  },
}));

import ScheduleLogTab from './ScheduleLogTab';

/** 부분 필드로 완전한 ScheduleLogRecord 를 만드는 헬퍼. */
function rec(over: Partial<ScheduleLogRecord>): ScheduleLogRecord {
  return {
    id: 0,
    correlation_id: 'sch:0',
    record_kind: 'fire',
    schedule_id: 'sch',
    rule_name: '규칙',
    declared_agent_id: 'ag-decl',
    actor_agent_id: '',
    trigger_time: 1000,
    target: '',
    action: '',
    result: '',
    targets: [],
    reason: '',
    timestamp: 1000,
    ...over,
  };
}

beforeEach(() => {
  useScheduleLogsMock.mockReset();
  logs.value = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
});

describe('correlation_id 병합(AC-16/AC-3)', () => {
  it('같은 correlation_id 의 fire+result 를 한 행으로 병합한다', () => {
    logs.value = {
      ...logs.value,
      data: [
        rec({
          id: 2,
          correlation_id: 'sch-1:1000',
          record_kind: 'result',
          schedule_id: 'sch-1',
          rule_name: '전원 규칙',
          declared_agent_id: 'ag-1',
          actor_agent_id: 'ag-1',
          action: 'set_power',
          target: 'group_id=station:0150',
          result: 'ok',
          timestamp: 1005,
        }),
        rec({
          id: 1,
          correlation_id: 'sch-1:1000',
          record_kind: 'fire',
          schedule_id: 'sch-1',
          rule_name: '전원 규칙',
          declared_agent_id: 'ag-1',
          trigger_time: 1000,
          timestamp: 1000,
        }),
      ],
    };
    render(<ScheduleLogTab />);

    // 두 레코드가 한 행으로 병합.
    const rows = screen.getAllByTestId(/^log-row-/);
    expect(rows).toHaveLength(1);
    expect(screen.getByTestId('log-row-sch-1:1000')).toBeInTheDocument();
    // result 측 데이터가 결합되어 노출.
    expect(screen.getByTestId('log-result-sch-1:1000')).toHaveTextContent('ok');
    expect(screen.getByText('set_power')).toBeInTheDocument();
  });
});

describe('fire-only 행(AC-15)', () => {
  it('result 가 없는 fire 그룹은 결과 컬럼 없이 렌더한다', () => {
    logs.value = {
      ...logs.value,
      data: [
        rec({
          id: 1,
          correlation_id: 'sch-2:2000',
          record_kind: 'fire',
          schedule_id: 'sch-2',
          trigger_time: 2000,
          timestamp: 2000,
        }),
      ],
    };
    render(<ScheduleLogTab />);

    expect(screen.getByTestId('log-row-sch-2:2000')).toBeInTheDocument();
    // 결과 배지가 '결과 없음' 이며 실제 ok/error 결과 배지는 없다.
    expect(screen.getByTestId('log-result-pending-sch-2:2000')).toBeInTheDocument();
    expect(screen.queryByTestId('log-result-sch-2:2000')).not.toBeInTheDocument();
    // 상세 펼침 버튼도 없다(targets 없음).
    expect(screen.queryByTestId('log-expand-sch-2:2000')).not.toBeInTheDocument();
  });
});

describe('필터 → 쿼리 파라미터(AC-10)', () => {
  it('필터 입력이 useScheduleLogs 쿼리로 전달된다', () => {
    render(<ScheduleLogTab />);

    fireEvent.change(screen.getByTestId('log-filter-schedule-id'), {
      target: { value: 'sch-9' },
    });
    fireEvent.change(screen.getByTestId('log-filter-agent-id'), {
      target: { value: 'ag-9' },
    });

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: 'sch-9',
      ruleName: undefined,
      agentId: 'ag-9',
    });
  });
});

describe('빈 상태(AC-10)', () => {
  it('로그가 없으면 에러가 아닌 빈 상태를 표시한다', () => {
    logs.value = { ...logs.value, data: [] };
    render(<ScheduleLogTab />);

    expect(screen.getByTestId('schedule-log-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-log-error')).not.toBeInTheDocument();
  });
});

describe('actor≠declared 가시성(RD-6)', () => {
  it('실제 실행 에이전트가 선언과 다르면 양쪽을 노출한다', () => {
    logs.value = {
      ...logs.value,
      data: [
        rec({
          id: 1,
          correlation_id: 'sch-3:3000',
          record_kind: 'result',
          schedule_id: 'sch-3',
          declared_agent_id: 'ag-decl',
          actor_agent_id: 'ag-actor',
          action: 'set_power',
          result: 'ok',
          trigger_time: 3000,
          timestamp: 3000,
        }),
      ],
    };
    render(<ScheduleLogTab />);

    expect(screen.getByTestId('log-actor-mismatch-sch-3:3000')).toBeInTheDocument();
    expect(screen.getByText(/ag-actor/)).toBeInTheDocument();
  });
});

describe('대상별 결과 상세 펼침', () => {
  it('targets 가 있으면 펼침 버튼으로 대상별 결과를 표시한다', () => {
    logs.value = {
      ...logs.value,
      data: [
        rec({
          id: 1,
          correlation_id: 'sch-4:4000',
          record_kind: 'result',
          schedule_id: 'sch-4',
          action: 'set_multiple',
          result: 'error',
          targets: [
            { target: 'm1', result: 'ok', reason: '' },
            { target: 'm2', result: 'error', reason: 'timeout' },
          ],
          trigger_time: 4000,
          timestamp: 4000,
        }),
      ],
    };
    render(<ScheduleLogTab />);

    // ok=1/2 요약.
    expect(screen.getByText('ok=1/2')).toBeInTheDocument();
    // 펼침 전에는 상세 없음.
    expect(screen.queryByTestId('log-targets-sch-4:4000')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('log-expand-sch-4:4000'));
    expect(screen.getByTestId('log-targets-sch-4:4000')).toBeInTheDocument();
    expect(screen.getByText('timeout')).toBeInTheDocument();
  });
});
