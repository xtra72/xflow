// 대시보드 디바이스 패널 테스트.
//
// 요구: 표시 항목(이름/ID/타입/프로토콜/상태/에이전트/등록/최근 확인),
// 컬럼별 정렬, 필터링, 검색 — 디바이스 탭과 동일한 기능 집합.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetProvider';
import { LOCAL_TARGET } from '@/lib/remote/target';
import { ALL_DEVICE_COLUMNS } from '@/hooks/useDeviceColumns';
import type { DeviceInfo } from '@/types/device';

const useDevicesRealtimeMock = vi.hoisted(() => vi.fn());
const useDevicesTargetMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDevice', () => ({ useDevicesRealtime: useDevicesRealtimeMock }));
vi.mock('@/hooks/useResourceTargets', () => ({ useDevicesTarget: useDevicesTargetMock }));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import DevicePanel from './DevicePanel';

function makeDevice(o: Partial<DeviceInfo> = {}): DeviceInfo {
  return {
    id: 'dev-1',
    uid: 'uid-1',
    name: 'Alpha',
    type: 'HVACR.IDU',
    protocol: 'samsung_nasa',
    agent_name: 'agent-a',
    source: 'auto',
    online: true,
    last_seen: '2026-08-26T01:00:00Z',
    ...o,
  } as DeviceInfo;
}

const DEVICES: DeviceInfo[] = [
  makeDevice({ id: 'd1', uid: 'uid-alpha', name: 'Alpha', protocol: 'samsung_nasa', type: 'HVACR.IDU', agent_name: 'agent-a', online: true, source: 'auto' }),
  makeDevice({ id: 'd2', uid: 'uid-bravo', name: 'Bravo', protocol: 'lgap', type: 'HVACR.ODU', agent_name: 'agent-b', online: false, source: 'config' }),
  makeDevice({ id: 'd3', uid: 'uid-charlie', name: 'Charlie', protocol: 'modbus', type: 'sensor', agent_name: 'agent-c', online: true, source: 'pinned' }),
];

function renderPanel(config: Record<string, unknown> = {}) {
  return render(
    <TargetProvider target={LOCAL_TARGET}>
      <DevicePanel panelId="p1" title="디바이스" config={config} refreshMs={0} />
    </TargetProvider>,
  );
}

/** 표 헤더 라벨을 읽는다(SortableHeader 가 붙이는 정렬 표시 글리프는 제거). */
function headerLabels(): string[] {
  return Array.from(document.querySelectorAll('thead th')).map((th) =>
    (th.textContent ?? '').replace(/[\u25b2\u25bc\u2195]/g, '').trim(),
  );
}

/** 표 본문 행의 이름 컬럼(첫 셀) 순서를 읽는다. */
function rowNames(): string[] {
  const body = document.querySelectorAll('tbody tr');
  return Array.from(body).map((tr) => tr.querySelector('td')?.textContent?.trim() ?? '');
}

function typeSearch(value: string) {
  fireEvent.change(screen.getByPlaceholderText('devices.filter.searchPlaceholder'), {
    target: { value },
  });
}

beforeEach(() => {
  useDevicesRealtimeMock.mockReset().mockReturnValue({ data: { data: DEVICES }, isLoading: false });
  useDevicesTargetMock.mockReset().mockReturnValue({ data: { data: [] }, isLoading: false });
});

describe('DevicePanel — 표시 항목', () => {
  it('요구된 8개 컬럼 헤더를 모두 표시한다', () => {
    renderPanel();
    // i18n stub 이 키를 그대로 반환하므로 컬럼 키의 라벨 키로 검증한다.
    expect(headerLabels()).toEqual([
      'devices.column.name',
      'devices.column.id',
      'devices.column.type',
      'devices.column.protocol',
      'devices.column.status',
      'devices.column.agent',
      'devices.column.source',
      'devices.column.lastSeen',
    ]);
  });

  it('컬럼 집합이 디바이스 탭의 SSOT 와 동일하다', () => {
    expect(ALL_DEVICE_COLUMNS).toEqual([
      'name', 'id', 'type', 'protocol', 'status', 'agent', 'source', 'last_seen',
    ]);
  });

  it('ID / 프로토콜 / 등록 셀 값을 렌더한다', () => {
    renderPanel();
    const row = screen.getByText('Alpha').closest('tr')!;
    expect(within(row).getByText('uid-alpha')).toBeInTheDocument();
    expect(within(row).getByText('SAMSUNG_NASA')).toBeInTheDocument();
    expect(within(row).getByText('devices.source.auto')).toBeInTheDocument();
  });

  it('패널 config 로 컬럼을 줄일 수 있다', () => {
    renderPanel({ visibleColumns: ['name', 'protocol'] });
    expect(headerLabels()).toEqual(['devices.column.name', 'devices.column.protocol']);
  });
});

describe('DevicePanel — 컬럼별 정렬', () => {
  it('기본은 이름 오름차순이다', () => {
    renderPanel();
    expect(rowNames()).toEqual(['Alpha', 'Bravo', 'Charlie']);
  });

  it('이름 헤더를 다시 누르면 내림차순으로 뒤집힌다', () => {
    renderPanel();
    fireEvent.click(screen.getByText('devices.column.name'));
    expect(rowNames()).toEqual(['Charlie', 'Bravo', 'Alpha']);
  });

  it('프로토콜로 정렬한다', () => {
    renderPanel();
    fireEvent.click(screen.getByText('devices.column.protocol'));
    // lgap < modbus < samsung_nasa
    expect(rowNames()).toEqual(['Bravo', 'Charlie', 'Alpha']);
  });

  it('상태로 정렬하면 온라인이 먼저 온다 (탭과 동일 규칙)', () => {
    renderPanel();
    fireEvent.click(screen.getByText('devices.column.status'));
    expect(rowNames().slice(-1)).toEqual(['Bravo']); // 오프라인이 마지막
  });

  it('등록(source) 컬럼은 정렬 대상이 아니다', () => {
    renderPanel();
    const before = rowNames();
    fireEvent.click(screen.getByText('devices.column.source'));
    expect(rowNames()).toEqual(before);
  });
});

describe('DevicePanel — 검색', () => {
  it('이름으로 검색한다', () => {
    renderPanel();
    typeSearch('brav');
    expect(rowNames()).toEqual(['Bravo']);
  });

  it('UID / 에이전트로도 검색한다', () => {
    renderPanel();
    typeSearch('uid-charlie');
    expect(rowNames()).toEqual(['Charlie']);
    typeSearch('agent-b');
    expect(rowNames()).toEqual(['Bravo']);
  });

  it('결과가 없으면 빈 목록과 다른 안내를 표시한다', () => {
    renderPanel();
    typeSearch('zzz');
    expect(screen.getByText('dashboard.devicePanel.noMatch')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.devicePanel.empty')).not.toBeInTheDocument();
  });

  it('디바이스가 아예 없으면 등록 없음 안내를 표시한다', () => {
    useDevicesRealtimeMock.mockReturnValue({ data: { data: [] }, isLoading: false });
    renderPanel();
    expect(screen.getByText('dashboard.devicePanel.empty')).toBeInTheDocument();
  });
});

describe('DevicePanel — 필터', () => {
  it('상태 필터로 오프라인만 남긴다', () => {
    renderPanel();
    fireEvent.click(screen.getByTitle('devices.status.offline'));
    expect(rowNames()).toEqual(['Bravo']);
  });

  it('프로토콜 필터로 좁힌다', () => {
    renderPanel();
    fireEvent.change(screen.getByDisplayValue('devices.filter.allProtocols'), {
      target: { value: 'modbus' },
    });
    expect(rowNames()).toEqual(['Charlie']);
  });

  it('타입 필터로 좁힌다', () => {
    renderPanel();
    fireEvent.change(screen.getByDisplayValue('devices.filter.allTypes'), {
      target: { value: 'HVACR.ODU' },
    });
    expect(rowNames()).toEqual(['Bravo']);
  });

  it('요약 뱃지는 필터 결과를 반영한다', () => {
    renderPanel();
    fireEvent.click(screen.getByTitle('devices.status.offline'));
    expect(screen.getByText('dashboard.panel.total 1')).toBeInTheDocument();
    expect(screen.getByText('dashboard.panel.online 0')).toBeInTheDocument();
    expect(screen.getByText('dashboard.panel.offline 1')).toBeInTheDocument();
  });
});

describe('DevicePanel — 페이지네이션', () => {
  /** 이름이 dev-00 … dev-(n-1) 인 디바이스 n 개. */
  function manyDevices(n: number): DeviceInfo[] {
    return Array.from({ length: n }, (_, i) =>
      makeDevice({
        id: `d${i}`,
        uid: `uid-${i}`,
        name: `dev-${String(i).padStart(2, '0')}`,
        online: i % 2 === 0,
      }),
    );
  }

  function setDevices(list: DeviceInfo[]) {
    useDevicesRealtimeMock.mockReturnValue({ data: { data: list }, isLoading: false });
  }

  it('기본 페이지 크기(10)만큼만 렌더하고 나머지는 다음 페이지에 둔다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    expect(rowNames()).toHaveLength(10);
    expect(rowNames()[0]).toBe('dev-00');
    expect(screen.getByText('1 / 3')).toBeInTheDocument();
  });

  it('다음/이전 버튼으로 페이지를 넘긴다', () => {
    setDevices(manyDevices(23));
    renderPanel();

    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByText('2 / 3')).toBeInTheDocument();
    expect(rowNames()[0]).toBe('dev-10');

    fireEvent.click(screen.getByLabelText('common.pagination.prev'));
    expect(screen.getByText('1 / 3')).toBeInTheDocument();
    expect(rowNames()[0]).toBe('dev-00');
  });

  it('마지막 페이지는 남은 개수만 렌더한다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByText('3 / 3')).toBeInTheDocument();
    expect(rowNames()).toHaveLength(3);
  });

  it('첫/마지막 페이지에서 이전/다음 버튼이 비활성화된다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    expect(screen.getByLabelText('common.pagination.prev')).toBeDisabled();
    expect(screen.getByLabelText('common.pagination.next')).not.toBeDisabled();

    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByLabelText('common.pagination.next')).toBeDisabled();
  });

  it('페이지 크기를 바꾸면 첫 페이지로 돌아간다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByText('2 / 3')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('common.pagination.perPage'), {
      target: { value: '50' },
    });
    expect(screen.getByText('1 / 1')).toBeInTheDocument();
    expect(rowNames()).toHaveLength(23);
  });

  it('검색하면 첫 페이지로 돌아가고 총 건수가 갱신된다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByText('2 / 3')).toBeInTheDocument();

    typeSearch('dev-1');
    // dev-10 … dev-19 → 10건 = 1페이지. 2페이지에 있던 상태에서 1페이지로 돌아온다.
    expect(screen.getByText('1 / 1')).toBeInTheDocument();
    expect(rowNames()).toHaveLength(10);
    expect(rowNames()[0]).toBe('dev-10');
  });

  it('필터로 총량이 줄면 범위를 벗어난 페이지가 마지막 페이지로 당겨진다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    fireEvent.click(screen.getByLabelText('common.pagination.next'));
    expect(screen.getByText('3 / 3')).toBeInTheDocument();

    // 오프라인만: 23개 중 11개 → 2페이지. 현재 3페이지는 범위 밖.
    fireEvent.click(screen.getByTitle('devices.status.offline'));
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
  });

  it('디바이스 탭으로 이동하는 "더 보기" 링크는 없다', () => {
    setDevices(manyDevices(23));
    renderPanel();
    expect(screen.queryByText('dashboard.panel.more')).not.toBeInTheDocument();
    expect(document.querySelector('a[href="/devices"]')).toBeNull();
  });

  it('결과가 없으면 페이지네이션을 렌더하지 않는다', () => {
    renderPanel();
    typeSearch('zzz');
    expect(screen.queryByLabelText('common.pagination.next')).not.toBeInTheDocument();
  });
});
