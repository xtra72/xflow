// XsfmStationsTab — 역사 다중선택(체크박스) + 선택 삭제 로직 테스트.
//
// 범위(이번 SPEC — 일괄 삭제): 역사(station)만.
//   - 행 체크박스 + 헤더 전체선택(보이는 역사 전체 기준, indeterminate 지원).
//   - "선택 삭제(N)" 버튼은 N>0 일 때만 활성, 확인 다이얼로그 후 선택된 각 code 에
//     remove_station 을 반복 호출(useBulkRemoveStations).
//   - 체크박스 클릭이 역사 행 확장/접기를 트리거하지 않는다.
//
// execAgent 를 command 로 라우팅 mock 하여 실제 훅/컴포넌트를 렌더한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

const execAgentMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/agentService', () => ({ execAgent: execAgentMock }));

import XsfmStationsTab from './XsfmStationsTab';

const STATIONS = [
  { station: 'ST-1', line: '2호선', display_name: '시청', order: 1, places: [] },
  { station: 'ST-2', line: '2호선', display_name: '을지로', order: 2, places: [] },
];

beforeEach(() => {
  execAgentMock.mockReset();
  execAgentMock.mockImplementation((_id: string, arg: { command: string }) => {
    if (arg.command === 'list_stations') return Promise.resolve({ status: 'ok', stations: STATIONS });
    return Promise.resolve({ status: 'ok' });
  });
});

function renderTab() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <XsfmStationsTab agentId="agent-1" />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe('XsfmStationsTab 다중선택 삭제', () => {
  it('전체선택은 보이는 역사 전체를 선택하고 "선택 삭제(N)" 카운트에 반영된다', async () => {
    renderTab();
    // 헤더 전체선택 + 2행 = 체크박스 3개.
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(3));

    const deleteBtn = screen.getByRole('button', { name: /선택 삭제/ });
    expect(deleteBtn).toBeDisabled();

    fireEvent.click(screen.getByLabelText('전체 선택'));

    expect(deleteBtn).toBeEnabled();
    expect(deleteBtn.textContent).toContain('(2)');
    const rowChecks = screen.getAllByLabelText('행 선택') as HTMLInputElement[];
    expect(rowChecks.filter((c) => c.checked)).toHaveLength(2);
  });

  it('행 체크박스 클릭은 역사 확장/접기를 트리거하지 않는다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(3));

    const [firstRow] = screen.getAllByLabelText('행 선택') as HTMLInputElement[];
    if (!firstRow) throw new Error('행 선택 체크박스를 찾지 못했습니다.');
    fireEvent.click(firstRow);

    // 확장되면 노출되는 위치 섹션 헤더 텍스트가 나타나지 않아야 한다.
    expect(screen.queryByText('등록된 위치가 없습니다.')).toBeNull();
    expect(firstRow.checked).toBe(true);
  });

  it('선택 삭제 확인 시 선택된 각 station code 에 remove_station 을 호출한다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(3));

    fireEvent.click(screen.getByLabelText('전체 선택'));
    fireEvent.click(screen.getByRole('button', { name: /선택 삭제/ }));

    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: '확인' }));

    await waitFor(() => {
      const removeCalls = execAgentMock.mock.calls.filter((c) => c[1]?.command === 'remove_station');
      expect(removeCalls).toHaveLength(2);
    });
    const removed = execAgentMock.mock.calls
      .filter((c) => c[1]?.command === 'remove_station')
      .map((c) => c[1].params.station);
    expect(removed.sort()).toEqual(['ST-1', 'ST-2']);
  });
});

// 페이지네이션: 전체 역사에 슬라이스 + 페이지 크기 변경 + 페이지 기준 전체선택.
describe('XsfmStationsTab 페이지네이션', () => {
  // 12개 역사(ST-01~ST-12). 역사 탭은 정렬이 없어 반환 순서 그대로 1페이지=처음 10개.
  const MANY = Array.from({ length: 12 }, (_, i) => ({
    station: `ST-${String(i + 1).padStart(2, '0')}`,
    line: '2호선',
    display_name: `역${String(i + 1).padStart(2, '0')}`,
    order: i + 1,
    places: [],
  }));

  function mockMany() {
    execAgentMock.mockImplementation((_id: string, arg: { command: string }) => {
      if (arg.command === 'list_stations') return Promise.resolve({ status: 'ok', stations: MANY });
      return Promise.resolve({ status: 'ok' });
    });
  }

  it('기본 페이지 크기 10 → 현재 페이지 10행만 렌더(12개 중)', async () => {
    mockMany();
    renderTab();
    // 헤더 전체선택 + 10행 = 체크박스 11개.
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(11));
    expect(screen.getByText('역10')).toBeTruthy();
    expect(screen.queryByText('역11')).toBeNull();
  });

  it('전체선택은 현재 페이지의 보이는 역사만 선택한다(10개)', async () => {
    mockMany();
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(11));
    fireEvent.click(screen.getByLabelText('전체 선택'));
    const deleteBtn = screen.getByRole('button', { name: /선택 삭제/ });
    expect(deleteBtn.textContent).toContain('(10)');
  });

  it('페이지 크기를 25로 늘리면 12개 전체가 렌더된다', async () => {
    mockMany();
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(11));
    fireEvent.change(screen.getByLabelText('페이지당'), { target: { value: '25' } });
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(13));
    expect(screen.getByText('역12')).toBeTruthy();
  });
});
