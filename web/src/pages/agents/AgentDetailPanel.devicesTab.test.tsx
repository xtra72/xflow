// AgentDetailPanel — "디바이스" 탭 컬럼 정렬 테스트.
//
// 범위:
//   - 최초 렌더 기본 정렬은 이름(colName) 오름차순.
//   - 헤더 클릭 시 asc → desc 토글, 다른 컬럼 클릭 시 그 컬럼 asc.
//   - 한글 이름 정렬(ko-KR) + 숫자 접미사 자연 정렬(실습실2 < 실습실10).
//   - 동률 tiebreak 는 디바이스 식별자(uid) 오름차순이며 정렬 방향과 무관.
//   - 빈 값(타입 미지정)은 asc/desc 모두 마지막.
//   - 데이터 refetch 후에도 정렬 상태가 유지된다.
//   - '등록'/액션 컬럼은 정렬 대상이 아니다.
//
// 데이터/뮤테이션/게이팅 훅과 로그레벨/시리즈 등 부수 의존은
// AgentDetailPanel.gatewaysTab 테스트와 동일하게 스텁으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { AgentInfo } from '@/types/agent';
import type { DeviceInfo } from '@/types/device';

const AGENT: AgentInfo = {
  id: 'a-1',
  name: 'nasa-agent',
  type: 'samsung_hvacr01',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: {},
  stats: undefined,
};

/** 디바이스 픽스처 생성 헬퍼(정렬에 무관한 필수 필드는 고정값). */
function device(
  uid: string,
  name: string,
  type: string,
  online: boolean,
): DeviceInfo {
  return {
    id: uid,
    uid,
    name,
    type,
    protocol: 'samsung_nasa',
    agent_name: AGENT.name,
    source: 'bridge',
    online,
    last_seen: '2026-01-01T00:00:00Z',
    capabilities: [],
  };
}

// 입력 순서는 의도적으로 뒤섞여 있다(기본 정렬이 실제로 적용되는지 확인).
// u-5 / u-6 은 이름이 같아 tiebreak(uid 오름차순) 대상이다.
// u-3 은 타입이 비어 있어 빈 값 배치 정책 대상이다.
const BASE_DEVICES: DeviceInfo[] = [
  device('u-4', '하늘', 'HVACR.IDU', false),
  device('u-6', '사무실 밖', 'HVACR.ODU', false),
  device('u-2', '실습실10', 'HVACR.IDU', false),
  device('u-1', '가람', 'sensor', true),
  device('u-5', '사무실 밖', 'HVACR.ODU', true),
  device('u-3', '실습실2', '', true),
];

// list_devices exec 응답: uid → bus address (ID 컬럼 표시값).
const ADDRESS_ITEMS = [
  { device_id: 'u-1', address: '20.00.03', source: 'bridge' },
  { device_id: 'u-2', address: '20.00.10', source: 'bridge' },
  { device_id: 'u-3', address: '20.00.02', source: 'bridge' },
  { device_id: 'u-4', address: '20.00.01', source: 'bridge' },
  { device_id: 'u-5', address: '20.00.20', source: 'bridge' },
  { device_id: 'u-6', address: '20.00.05', source: 'bridge' },
];

// 테스트별로 교체 가능한 디바이스 목록(refetch 시뮬레이션용).
const state = vi.hoisted(() => ({ devices: [] as unknown[] }));

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: AGENT, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: undefined, isLoading: false, error: null }),
}));

// list_devices 는 읽기 전용이라 useQueryAgent(POST /agents/{id}/query) 경로로 나간다.
// 주소 맵을 채우는 응답은 이 스파이가 돌려준다.
const queryMutate = vi.hoisted(() =>
  vi.fn((_args: unknown, opts?: { onSuccess?: (res: unknown) => void }) => {
    opts?.onSuccess?.({ data: ADDRESS_ITEMS });
  }),
);
// 쓰기(add_device 등) 경로. 이 테스트는 정렬만 보므로 호출되지 않는다.
const execMutate = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useAgent', () => ({
  useAgent: () => ({ data: AGENT, isLoading: false }),
  useConfigureAgent: () => ({ isPending: false, isError: false, mutateAsync: vi.fn() }),
  useExecAgent: () => ({ isPending: false, mutate: execMutate }),
  useQueryAgent: () => ({ isPending: false, mutate: queryMutate }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  }),
}));

vi.mock('@/hooks/useDevice', () => ({
  useDevicesRealtime: () => ({ data: { data: state.devices }, isLoading: false }),
  useDeleteDevice: () => ({ isPending: false, mutate: vi.fn() }),
  useSetDeviceReport: () => ({ isPending: false, mutate: vi.fn() }),
}));

// 라벨을 i18n 키 그대로 렌더해 헤더/셀 단정을 키 기준으로 수행한다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: () => Promise.resolve(),
  resetComponentLogLevel: () => Promise.resolve(),
}));

vi.mock('@/services/api/seriesDataSource', () => ({
  useSeriesDataSource: () => ({ useKeys: () => ({ data: { keys: [] } }) }),
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification: vi.fn() }),
}));

import AgentDetailPanel from './AgentDetailPanel';

const DEVICES_TAB = 'agents.detail.tabs.devices';
const COL_NAME = 'agents.detail.devices.colName';
const COL_ID = 'agents.detail.devices.colId';
const COL_TYPE = 'agents.detail.devices.colType';
const COL_CONNECTION = 'agents.detail.devices.colConnection';
const COL_SOURCE = 'agents.detail.devices.colSource';

/** 디바이스 탭을 연 상태로 패널을 렌더한다. */
function renderDevicesTab() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <AgentDetailPanel agentId={AGENT.id} agentType={AGENT.type} agentName={AGENT.name} />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole('button', { name: DEVICES_TAB }));
  return utils;
}

/** 본문 행의 n 번째 셀 텍스트 배열(0=이름, 1=ID, 2=타입). */
function cells(index: number): string[] {
  const rows = screen.getAllByRole('row').slice(1); // 0 번은 헤더
  return rows.map((r) => within(r).getAllByRole('cell')[index]?.textContent?.trim() ?? '');
}

const names = () => cells(0);
const ids = () => cells(1);

beforeEach(() => {
  vi.clearAllMocks();
  state.devices = BASE_DEVICES;
});

describe('AgentDetailPanel — 디바이스 탭 정렬', () => {
  it('최초 렌더는 이름 오름차순이며 한글/숫자 접미사가 올바로 정렬된다', () => {
    renderDevicesTab();
    // 가 < 사 < 실 < 하 (ko-KR), 그리고 실습실2 < 실습실10 (numeric).
    expect(names()).toEqual([
      '가람',
      '사무실 밖',
      '사무실 밖',
      '실습실2',
      '실습실10',
      '하늘',
    ]);
  });

  it('동률(같은 이름)은 uid 오름차순 tiebreak 로 결정적으로 정렬된다', () => {
    renderDevicesTab();
    // u-5(20.00.20) 가 u-6(20.00.05) 보다 앞 — 표시값(주소)이 아니라 uid 기준.
    expect(ids().slice(1, 3)).toEqual(['20.00.20', '20.00.05']);
  });

  it('이름 헤더 클릭 시 desc 로 토글되고 tiebreak 는 방향과 무관하게 유지된다', () => {
    renderDevicesTab();
    fireEvent.click(screen.getByText(COL_NAME));
    expect(names()).toEqual([
      '하늘',
      '실습실10',
      '실습실2',
      '사무실 밖',
      '사무실 밖',
      '가람',
    ]);
    // 동률 두 행의 순서는 asc 와 동일(u-5 → u-6).
    expect(ids().slice(3, 5)).toEqual(['20.00.20', '20.00.05']);

    // 한 번 더 클릭하면 asc 로 복귀.
    fireEvent.click(screen.getByText(COL_NAME));
    expect(names()[0]).toBe('가람');
  });

  it('다른 컬럼(ID) 클릭 시 그 컬럼 asc 로 전환된다', () => {
    renderDevicesTab();
    fireEvent.click(screen.getByText(COL_ID));
    expect(ids()).toEqual([
      '20.00.01',
      '20.00.02',
      '20.00.03',
      '20.00.05',
      '20.00.10',
      '20.00.20',
    ]);
  });

  it('타입 정렬에서 빈 값은 asc/desc 모두 마지막에 위치한다', () => {
    renderDevicesTab();
    fireEvent.click(screen.getByText(COL_TYPE));
    // 센서 < 실내기 < 실외기, 빈 타입(u-3, 실습실2)은 마지막.
    expect(names()).toEqual([
      '가람',
      '실습실10',
      '하늘',
      '사무실 밖',
      '사무실 밖',
      '실습실2',
    ]);

    fireEvent.click(screen.getByText(COL_TYPE));
    expect(names()).toEqual([
      '사무실 밖',
      '사무실 밖',
      '실습실10',
      '하늘',
      '가람',
      '실습실2', // 방향이 바뀌어도 빈 값은 여전히 마지막
    ]);
  });

  it('연결 컬럼은 asc 에서 온라인 우선으로 정렬된다', () => {
    renderDevicesTab();
    fireEvent.click(screen.getByText(COL_CONNECTION));
    // online: u-1(가람), u-3(실습실2), u-5(사무실 밖) → uid 오름차순.
    expect(names().slice(0, 3)).toEqual(['가람', '실습실2', '사무실 밖']);
  });

  it('데이터 refetch 후에도 정렬 상태가 유지된다', () => {
    const { rerender } = renderDevicesTab();
    fireEvent.click(screen.getByText(COL_NAME)); // desc
    expect(names()[0]).toBe('하늘');

    // realtime 갱신으로 목록이 교체되는 상황(신규 디바이스 추가).
    state.devices = [...BASE_DEVICES, device('u-7', '회의실', 'sensor', true)];
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    rerender(
      <QueryClientProvider client={queryClient}>
        <AgentDetailPanel agentId={AGENT.id} agentType={AGENT.type} agentName={AGENT.name} />
      </QueryClientProvider>,
    );

    // 여전히 이름 desc — 새 디바이스(회의실)가 desc 순서에 맞게 삽입된다.
    expect(names()).toEqual([
      '회의실',
      '하늘',
      '실습실10',
      '실습실2',
      '사무실 밖',
      '사무실 밖',
      '가람',
    ]);
  });

  it("'등록' 컬럼과 액션 컬럼 헤더는 정렬 대상이 아니다", () => {
    renderDevicesTab();
    const headerRow = screen.getAllByRole('row')[0]!;
    const headerCells = within(headerRow).getAllByRole('columnheader');
    // 이름/ID/타입/연결 4개만 클릭 가능(정렬), 등록/액션은 비대상.
    const sortable = headerCells.filter((th) => th.className.includes('cursor-pointer'));
    expect(sortable.map((th) => th.textContent?.replace(/[▲▼]/g, '').trim())).toEqual([
      COL_NAME,
      COL_ID,
      COL_TYPE,
      COL_CONNECTION,
    ]);

    // 등록 헤더 클릭은 순서를 바꾸지 않는다.
    const before = names();
    fireEvent.click(screen.getByText(COL_SOURCE));
    expect(names()).toEqual(before);
  });
});
