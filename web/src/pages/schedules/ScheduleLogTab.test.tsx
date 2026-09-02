// SPEC-SCHEDULE-VIEW-001 M6 — ScheduleLogTab.
// correlation_id 병합(AC-16/AC-3): fire+result → 1행 / fire-only → 결과 컬럼 없음(AC-15) /
// 필터 입력이 쿼리 파라미터로 전달(AC-10) / 빈 상태(에러 아님, AC-10) /
// actor≠declared 불일치 가시성(RD-6) / 페이지네이션 limit·offset 배선 / 초기화 확인 흐름.
//
// useScheduleLogs·useClearScheduleLogs 훅만 스텁하고 mergeScheduleLogs(순수 함수)는
// 실제를 사용해 병합 로직을 컴포넌트와 함께 검증한다. ConfirmDialog 가 useTranslation 을
// 쓰므로 I18nProvider 없이 ko.json 을 점 표기 키로 해석하는 i18n mock 을 둔다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';

import type { ScheduleLogRecord } from '@/types/schedule';

// i18n: I18nProvider 없이 실제 ko 번역을 반환하는 mock(ConfirmDialog 기본 라벨용).
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

const logs = vi.hoisted(() => ({
  value: {
    data: { items: [] as ScheduleLogRecord[], total: 0 },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  },
}));
const useScheduleLogsMock = vi.hoisted(() => vi.fn());
const clearState = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
  isPending: false,
}));

vi.mock('@/hooks/useScheduleLogs', () => ({
  useScheduleLogs: (params: unknown) => {
    useScheduleLogsMock(params);
    return logs.value;
  },
  useClearScheduleLogs: () => clearState,
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

/** { items, total } 페이지 헬퍼(total 미지정 시 items 길이). */
function page(items: ScheduleLogRecord[], total = items.length) {
  return { items, total };
}

beforeEach(() => {
  useScheduleLogsMock.mockReset();
  clearState.mutateAsync.mockClear();
  clearState.isPending = false;
  logs.value = {
    data: page([]),
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  };
});

describe('correlation_id 병합(AC-16/AC-3)', () => {
  it('같은 correlation_id 의 fire+result 를 한 행으로 병합한다', () => {
    logs.value = {
      ...logs.value,
      data: page([
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
      ]),
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
      data: page([
        rec({
          id: 1,
          correlation_id: 'sch-2:2000',
          record_kind: 'fire',
          schedule_id: 'sch-2',
          trigger_time: 2000,
          timestamp: 2000,
        }),
      ]),
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
  it('필터 입력이 useScheduleLogs 쿼리로 전달된다(limit/offset 포함)', () => {
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
      target: undefined,
      action: undefined,
      result: undefined,
      order: 'desc',
      limit: 25,
      offset: 0,
    });
  });

  it('대상/동작/결과 필터 입력이 쿼리로 전달된다', () => {
    render(<ScheduleLogTab />);

    fireEvent.change(screen.getByTestId('log-filter-target'), {
      target: { value: 'group_id=station:0150' },
    });
    fireEvent.change(screen.getByTestId('log-filter-action'), {
      target: { value: 'set_power' },
    });
    fireEvent.change(screen.getByTestId('log-filter-result'), {
      target: { value: 'error' },
    });

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: undefined,
      ruleName: undefined,
      agentId: undefined,
      target: 'group_id=station:0150',
      action: 'set_power',
      result: 'error',
      order: 'desc',
      limit: 25,
      offset: 0,
    });
  });
});

describe('실행 시각 정렬(order)', () => {
  it('실행 시각 헤더 클릭 시 order 가 asc 로 토글되고 표시자가 바뀐다', () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })], 1),
    };
    render(<ScheduleLogTab />);

    // 기본은 최신순(desc) → ▼ 표시자.
    const sortTh = screen.getByTestId('log-sort-time');
    expect(sortTh).toHaveTextContent('▼');

    fireEvent.click(sortTh);

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: undefined,
      ruleName: undefined,
      agentId: undefined,
      target: undefined,
      action: undefined,
      result: undefined,
      order: 'asc',
      limit: 25,
      offset: 0,
    });
    // asc 로 바뀌면 ▲ 표시자.
    expect(screen.getByTestId('log-sort-time')).toHaveTextContent('▲');
  });
});

describe('페이지네이션(limit/offset 배선)', () => {
  it('기본 limit 25, offset 0 으로 조회한다', () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })], 60),
    };
    render(<ScheduleLogTab />);

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: undefined,
      ruleName: undefined,
      agentId: undefined,
      target: undefined,
      action: undefined,
      result: undefined,
      order: 'desc',
      limit: 25,
      offset: 0,
    });
    // total 60 / pageSize 25 → 3 페이지.
    expect(screen.getByTestId('log-page-indicator')).toHaveTextContent('1 / 3');
    expect(screen.getByTestId('log-range-indicator')).toHaveTextContent('총 60개 레코드 중 1–1번째');
  });

  it('다음 페이지 클릭 시 offset 이 페이지 크기만큼 증가한다', () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })], 60),
    };
    render(<ScheduleLogTab />);

    fireEvent.click(screen.getByTestId('log-next-page'));

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: undefined,
      ruleName: undefined,
      agentId: undefined,
      target: undefined,
      action: undefined,
      result: undefined,
      order: 'desc',
      limit: 25,
      offset: 25,
    });
    expect(screen.getByTestId('log-page-indicator')).toHaveTextContent('2 / 3');
  });

  it('페이지 크기 변경 시 offset 을 0 으로 리셋한다', () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })], 60),
    };
    render(<ScheduleLogTab />);

    // 먼저 2페이지로 이동(offset 25).
    fireEvent.click(screen.getByTestId('log-next-page'));
    // 페이지 크기 50 으로 변경 → offset 0 리셋 + limit 50.
    fireEvent.change(screen.getByTestId('log-page-size'), { target: { value: '50' } });

    expect(useScheduleLogsMock).toHaveBeenLastCalledWith({
      scheduleId: undefined,
      ruleName: undefined,
      agentId: undefined,
      target: undefined,
      action: undefined,
      result: undefined,
      order: 'desc',
      limit: 50,
      offset: 0,
    });
  });
});

describe('빈 상태(AC-10)', () => {
  it('로그가 없으면 에러가 아닌 빈 상태를 표시한다', () => {
    logs.value = { ...logs.value, data: page([]) };
    render(<ScheduleLogTab />);

    expect(screen.getByTestId('schedule-log-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-log-error')).not.toBeInTheDocument();
  });
});

describe('actor≠declared 가시성(RD-6)', () => {
  it('실제 실행 에이전트가 선언과 다르면 양쪽을 노출한다', () => {
    logs.value = {
      ...logs.value,
      data: page([
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
      ]),
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
      data: page([
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
      ]),
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

describe('초기화(전체 삭제) 확인 흐름', () => {
  it('확인 없이 단일 클릭으로는 삭제하지 않는다', () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })]),
    };
    render(<ScheduleLogTab />);

    fireEvent.click(screen.getByTestId('log-clear'));

    // 확인 다이얼로그만 열리고 아직 삭제는 호출되지 않는다.
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(clearState.mutateAsync).not.toHaveBeenCalled();
  });

  it('확인 시 clearScheduleLogs 뮤테이션을 호출한다', async () => {
    logs.value = {
      ...logs.value,
      data: page([rec({ correlation_id: 'c:1', record_kind: 'fire' })]),
    };
    render(<ScheduleLogTab />);

    fireEvent.click(screen.getByTestId('log-clear'));
    const dialog = screen.getByRole('dialog');
    expect(
      within(dialog).getByText(/저장된 모든 실행 로그를 삭제합니다/),
    ).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole('button', { name: '초기화' }));

    await waitFor(() => expect(clearState.mutateAsync).toHaveBeenCalledTimes(1));
  });
});
