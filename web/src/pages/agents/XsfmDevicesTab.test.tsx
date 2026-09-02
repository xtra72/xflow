// XsfmDevicesTab — 다중선택(체크박스) + 선택 삭제 로직 테스트.
//
// 범위(이번 SPEC — 일괄 삭제):
//   - 행 체크박스 + 헤더 전체선택(현재 보이는 선택가능 행 기준).
//   - Source="config" 디바이스는 삭제 불가 → 체크박스 disabled(오삭제 방지).
//   - "선택 삭제(N)" 버튼은 N>0 일 때만 활성, 확인 다이얼로그 후 선택된 각 id 에
//     remove_device 를 반복 호출(useBulkRemoveDevices).
//
// execAgent 를 command 로 라우팅 mock 하여 실제 훅/컴포넌트를 렌더한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { AirDevice } from '@/hooks/useStation';

const execAgentMock = vi.hoisted(() => vi.fn());
// list_devices / list_stations 는 읽기 전용이라 queryAgent(POST /agents/{id}/query)로 나간다.
// 쓰기(add_station / remove_device ...)만 execAgent 로 남는다.
const queryAgentMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
  queryAgent: queryAgentMock,
}));

import XsfmDevicesTab from './XsfmDevicesTab';

// 2개 bridge(선택가능) + 1개 config(삭제 보호) 디바이스. 이름 오름차순 기본 정렬(A,B,C).
const DEVICES: AirDevice[] = [
  { device_id: 'd1', name: 'A', group_id: '', station: 'ST-1', place: 'p1', index: 1, online: true, power: false, fan_speed: 0, source: 'bridge' },
  { device_id: 'd2', name: 'B', group_id: '', station: 'ST-1', place: 'p1', index: 2, online: false, power: false, fan_speed: 0, source: 'bridge' },
  { device_id: 'd3', name: 'C', group_id: '', station: 'ST-1', place: 'p1', index: 3, online: false, power: false, fan_speed: 0, source: 'config' },
];
const STATIONS = [
  { station: 'ST-1', line: '2호선', display_name: '시청', order: 1, places: [{ place: 'p1', display_name: '승강장', order: 1 }] },
];

beforeEach(() => {
  execAgentMock.mockReset();
  queryAgentMock.mockReset();
  execAgentMock.mockResolvedValue({ status: 'ok' });
  queryAgentMock.mockImplementation((_id: string, arg: { command: string }) => {
    if (arg.command === 'list_devices') return Promise.resolve({ status: 'ok', devices: DEVICES });
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
        <XsfmDevicesTab agentId="agent-1" />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe('XsfmDevicesTab 다중선택 삭제', () => {
  it('config 디바이스 체크박스는 disabled(삭제 보호), 나머지 행 체크박스는 활성', async () => {
    renderTab();
    // 로드 완료: 헤더 전체선택 + 3행 = 체크박스 4개.
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(4));
    const rowChecks = screen.getAllByLabelText('행 선택') as HTMLInputElement[];
    expect(rowChecks).toHaveLength(3);
    // config(3번째, 이름 C) 만 disabled.
    expect(rowChecks.filter((c) => c.disabled)).toHaveLength(1);
  });

  it('전체선택은 선택가능(bridge) 행만 선택하고 "선택 삭제(N)" 카운트에 반영된다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(4));

    const deleteBtn = screen.getByRole('button', { name: /선택 삭제/ });
    expect(deleteBtn).toBeDisabled();

    // 헤더 전체선택 클릭.
    fireEvent.click(screen.getByLabelText('전체 선택'));

    // config 제외 2개만 선택 → 버튼 활성 + (2).
    expect(deleteBtn).toBeEnabled();
    expect(deleteBtn.textContent).toContain('(2)');
    // config 체크박스는 여전히 미선택.
    const rowChecks = screen.getAllByLabelText('행 선택') as HTMLInputElement[];
    expect(rowChecks.filter((c) => c.checked)).toHaveLength(2);
  });

  it('선택 삭제 확인 시 선택된 각 device_id 에 remove_device 를 호출한다(config 제외)', async () => {
    renderTab();
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(4));

    fireEvent.click(screen.getByLabelText('전체 선택'));
    fireEvent.click(screen.getByRole('button', { name: /선택 삭제/ }));

    // 확인 다이얼로그 → 확인.
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: '확인' }));

    await waitFor(() => {
      const removeCalls = execAgentMock.mock.calls.filter((c) => c[1]?.command === 'remove_device');
      expect(removeCalls).toHaveLength(2);
    });
    const removed = execAgentMock.mock.calls
      .filter((c) => c[1]?.command === 'remove_device')
      .map((c) => c[1].params.device_id);
    expect(removed.sort()).toEqual(['d1', 'd2']);
    // config(d3)는 삭제 호출 없음.
    expect(removed).not.toContain('d3');
  });
});

// 페이지네이션: 정렬/필터 뒤 슬라이스 + 페이지 크기 변경 + 페이지 기준 전체선택.
describe('XsfmDevicesTab 페이지네이션', () => {
  // 12개 bridge 디바이스(이름 01~12, 오름차순 정렬 → 1페이지=01~10).
  const MANY: AirDevice[] = Array.from({ length: 12 }, (_, i) => ({
    device_id: `dev-${i + 1}`,
    name: String(i + 1).padStart(2, '0'),
    group_id: '',
    station: 'ST-1',
    place: 'p1',
    index: i + 1,
    online: false,
    power: false,
    fan_speed: 0,
    source: 'bridge',
  }));

  function mockMany() {
    queryAgentMock.mockImplementation((_id: string, arg: { command: string }) => {
      if (arg.command === 'list_devices') return Promise.resolve({ status: 'ok', devices: MANY });
      if (arg.command === 'list_stations') return Promise.resolve({ status: 'ok', stations: STATIONS });
      return Promise.resolve({ status: 'ok' });
    });
  }

  it('기본 페이지 크기 10 → 현재 페이지 10행만 렌더(12개 중)', async () => {
    mockMany();
    renderTab();
    // 헤더 전체선택 + 10행 = 체크박스 11개.
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(11));
    // 1페이지: dev-1~dev-10 표시, dev-11/dev-12 미표시(device_id 로 검증 — 숫자 라벨은 페이지 크기 옵션과 충돌).
    expect(screen.getByText('dev-10')).toBeTruthy();
    expect(screen.queryByText('dev-11')).toBeNull();
  });

  it('전체선택은 현재 페이지의 보이는 행만 선택한다(10개, 12개 아님)', async () => {
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
    // 헤더 + 12행 = 13.
    await waitFor(() => expect(screen.getAllByRole('checkbox')).toHaveLength(13));
    expect(screen.getByText('dev-11')).toBeTruthy();
    expect(screen.getByText('dev-12')).toBeTruthy();
  });
});
