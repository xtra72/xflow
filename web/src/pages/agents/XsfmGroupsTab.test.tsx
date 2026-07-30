// XsfmGroupsTab 테스트 (SPEC-XSFM-GROUP-001 Module 6, AC 6.1~6.4).
//
// 그룹 목록(type 배지 + 멤버 수) 표시, 커스텀 그룹 CRUD 명령 호출(add/set/remove),
// 멤버 편집(커스텀만) + 기본 그룹 읽기 전용, 그룹 일괄 제어(group_id 셀렉터)를 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { AirDevice, AirStation } from '@/hooks/useStation';
import type { Group } from '@/hooks/useGroups';
import type { ControlResponse } from '@/hooks/useXsfmControl';

// ---- mocks ----

const groupsMock = vi.hoisted(() => ({ current: [] as Group[] }));
const devicesMock = vi.hoisted(() => ({ current: [] as AirDevice[] }));
const stationsMock = vi.hoisted(() => ({ current: [] as AirStation[] }));
const mutations = vi.hoisted(() => ({
  add: vi.fn(),
  set: vi.fn(),
  remove: vi.fn(),
}));

vi.mock('@/hooks/useGroups', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useGroups')>();
  return {
    ...actual, // isCustomGroup 는 실제 구현 사용.
    useGroups: () => ({ data: groupsMock.current, isLoading: false }),
    useAddGroup: () => ({ mutate: mutations.add, isPending: false }),
    useSetGroup: () => ({ mutate: mutations.set, isPending: false }),
    useRemoveGroup: () => ({ mutate: mutations.remove, isPending: false }),
  };
});

vi.mock('@/hooks/useStation', () => ({
  useXsfmDevices: () => ({ data: devicesMock.current, isLoading: false }),
  useStations: () => ({ data: stationsMock.current, isLoading: false }),
}));

// FacilityBulkControl 이 사용하는 제어 훅 스텁(fan-out 재구현 없음).
const controlMock = vi.hoisted(() => ({
  setPower: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
  setFanSpeed: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
}));
vi.mock('@/hooks/useXsfmControl', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useXsfmControl')>();
  return { ...actual, useXsfmControl: () => controlMock };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) => selector({ addNotification }),
}));

import XsfmGroupsTab from './XsfmGroupsTab';

function device(id: string, name = id, overrides: Partial<AirDevice> = {}): AirDevice {
  return {
    device_id: id,
    name,
    group_id: '',
    station: 'ST-1',
    place: 'PL-1',
    index: 1,
    online: true,
    power: true,
    fan_speed: 1,
    source: 'config',
    ...overrides,
  };
}

// 역사 레지스트리(라인 파생 + 표시명 해석용). ST-1=강남역(2호선), ST-2=역삼역(3호선).
const stationRegistry: AirStation[] = [
  {
    station: 'ST-1',
    line: '2호선',
    display_name: '강남역',
    order: 1,
    places: [{ place: 'PL-1', display_name: '대합실', order: 1 }],
  },
  {
    station: 'ST-2',
    line: '3호선',
    display_name: '역삼역',
    order: 2,
    places: [{ place: 'PL-2', display_name: '승강장', order: 1 }],
  },
];

/** 모달을 연 뒤 멤버 테이블의 tbody 행 device_id 순서를 반환한다(정렬 검증용). */
function memberRowIds(): string[] {
  const table = screen.getByTestId('group-member-table');
  return Array.from(table.querySelectorAll('tbody tr')).map(
    (tr) => tr.getAttribute('data-testid')?.replace('group-member-row-', '') ?? '',
  );
}

const customGroup: Group = { id: 'custom:x', name: '2층', type: 'custom', member_count: 1, members: ['d1'] };
const stationGroup: Group = { id: 'station:ST-1', name: '강남역', type: 'station', member_count: 2, members: ['d1', 'd2'] };

beforeEach(() => {
  mutations.add.mockReset();
  mutations.set.mockReset();
  mutations.remove.mockReset();
  controlMock.setPower.mutate.mockReset();
  controlMock.setFanSpeed.mutate.mockReset();
  addNotification.mockReset();
  groupsMock.current = [];
  devicesMock.current = [];
  stationsMock.current = [];
});

describe('XsfmGroupsTab', () => {
  it('그룹 목록을 type 배지 + 멤버 수와 함께 표시한다(AC 6.1)', () => {
    groupsMock.current = [stationGroup, customGroup];
    devicesMock.current = [device('d1'), device('d2')];
    render(<XsfmGroupsTab agentId="a1" />);

    expect(screen.getByTestId('group-list')).toBeInTheDocument();
    // type 배지: station/custom 라벨(i18n 키 그대로).
    expect(screen.getByTestId('group-type-station:ST-1')).toHaveTextContent('agents.detail.groups.type.station');
    expect(screen.getByTestId('group-type-custom:x')).toHaveTextContent('agents.detail.groups.type.custom');
    // 멤버 수.
    expect(screen.getByTestId('group-member-count-station:ST-1')).toHaveTextContent('2');
    expect(screen.getByTestId('group-member-count-custom:x')).toHaveTextContent('1');
  });

  it('기본 그룹(station)은 편집/삭제 버튼이 없고 멤버를 읽기 전용으로 표시한다(AC 6.3)', () => {
    groupsMock.current = [stationGroup];
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2')];
    render(<XsfmGroupsTab agentId="a1" />);

    expect(screen.queryByTestId('group-edit-station:ST-1')).toBeNull();
    expect(screen.queryByTestId('group-remove-station:ST-1')).toBeNull();
    // 읽기 전용 멤버 목록에 디바이스 이름이 표시된다.
    const readonly = screen.getByTestId('group-members-readonly-station:ST-1');
    expect(within(readonly).getByText('기기1')).toBeInTheDocument();
    expect(within(readonly).getByText('기기2')).toBeInTheDocument();
  });

  it('커스텀 그룹 생성: add_group 을 { name, members } 로 호출한다(AC 6.2)', () => {
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2')];
    render(<XsfmGroupsTab agentId="a1" />);

    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));
    // 이름 입력.
    const nameInput = screen.getByPlaceholderText('agents.detail.groups.namePlaceholder');
    fireEvent.change(nameInput, { target: { value: '3층' } });
    // 멤버 체크(d2).
    fireEvent.click(screen.getByTestId('group-member-checkbox-d2'));
    fireEvent.click(screen.getByTestId('group-form-submit'));

    expect(mutations.add).toHaveBeenCalledTimes(1);
    expect(mutations.add).toHaveBeenCalledWith({ name: '3층', members: ['d2'] }, expect.anything());
  });

  it('커스텀 그룹 수정: set_group 을 { group_id, name, members } 로 호출한다(AC 6.2/6.3)', () => {
    groupsMock.current = [customGroup];
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2')];
    render(<XsfmGroupsTab agentId="a1" />);

    fireEvent.click(screen.getByTestId('group-edit-custom:x'));
    // 기존 멤버 d1 은 체크되어 있어야 한다.
    expect(screen.getByTestId('group-member-checkbox-d1')).toBeChecked();
    // d2 추가.
    fireEvent.click(screen.getByTestId('group-member-checkbox-d2'));
    fireEvent.click(screen.getByTestId('group-form-submit'));

    expect(mutations.set).toHaveBeenCalledTimes(1);
    expect(mutations.set).toHaveBeenCalledWith(
      { group_id: 'custom:x', name: '2층', members: ['d1', 'd2'] },
      expect.anything(),
    );
  });

  it('커스텀 그룹 삭제: 확인 후 remove_group 을 group_id 로 호출한다(AC 6.2)', () => {
    groupsMock.current = [customGroup];
    render(<XsfmGroupsTab agentId="a1" />);

    fireEvent.click(screen.getByTestId('group-remove-custom:x'));
    // ConfirmDialog 확인 버튼 클릭(공용 컴포넌트 — confirm 역할).
    fireEvent.click(screen.getByRole('button', { name: 'common.confirm' }));

    expect(mutations.remove).toHaveBeenCalledTimes(1);
    expect(mutations.remove).toHaveBeenCalledWith('custom:x', expect.anything());
  });

  it('그룹 일괄 제어(OFF)가 { group_id } 셀렉터로 setPower 를 호출한다(AC 6.4)', () => {
    groupsMock.current = [customGroup];
    render(<XsfmGroupsTab agentId="a1" />);

    // FacilityBulkControl 의 OFF 버튼 클릭 → group_id 셀렉터 fan-out.
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ group_id: 'custom:x', power: false });
  });

  it('이름 없이 저장하면 에러 알림을 내고 명령을 호출하지 않는다', () => {
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));
    // submit 버튼은 이름이 비면 disabled → 클릭해도 mutate 안 됨.
    expect(screen.getByTestId('group-form-submit')).toBeDisabled();
    expect(mutations.add).not.toHaveBeenCalled();
  });

  // ---- 멤버 선택 테이블(라인/역사/위치 컬럼 + 필터 + 정렬) ----

  it('멤버 선택 테이블에 라인(파생)/역사/위치/이름을 해석해 렌더한다', () => {
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2', { station: 'ST-2', place: 'PL-2' })];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    expect(screen.getByTestId('group-member-table')).toBeInTheDocument();
    // d1: 라인 2호선 파생 + 역사 강남역 + 위치 대합실.
    const row1 = screen.getByTestId('group-member-row-d1');
    expect(within(row1).getByText('2호선')).toBeInTheDocument();
    expect(within(row1).getByText('강남역')).toBeInTheDocument();
    expect(within(row1).getByText('대합실')).toBeInTheDocument();
    // d2: 라인 3호선 + 역삼역 + 승강장.
    const row2 = screen.getByTestId('group-member-row-d2');
    expect(within(row2).getByText('3호선')).toBeInTheDocument();
    expect(within(row2).getByText('역삼역')).toBeInTheDocument();
  });

  it('라인 필터가 해당 라인의 디바이스만 남긴다', () => {
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2', { station: 'ST-2', place: 'PL-2' })];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    fireEvent.change(screen.getByTestId('group-member-filter-line'), { target: { value: '2호선' } });
    expect(screen.getByTestId('group-member-row-d1')).toBeInTheDocument();
    expect(screen.queryByTestId('group-member-row-d2')).toBeNull();
  });

  it('역사 필터가 해당 역사의 디바이스만 남긴다', () => {
    devicesMock.current = [device('d1', '기기1'), device('d2', '기기2', { station: 'ST-2', place: 'PL-2' })];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    fireEvent.change(screen.getByTestId('group-member-filter-station'), { target: { value: 'ST-2' } });
    expect(screen.queryByTestId('group-member-row-d1')).toBeNull();
    expect(screen.getByTestId('group-member-row-d2')).toBeInTheDocument();
  });

  it('위치 컬럼 정렬 헤더 클릭이 정렬 방향을 토글한다(동률은 이름 tiebreak)', () => {
    // 위치 표시명: d1 대합실, d2 승강장. asc = 대합실 → 승강장, desc = 승강장 → 대합실.
    devicesMock.current = [device('d2', '기기2', { station: 'ST-2', place: 'PL-2' }), device('d1', '기기1')];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    // 위치 헤더 클릭 → 위치 asc(대합실=d1 먼저).
    fireEvent.click(screen.getByText('agents.detail.groups.place'));
    expect(memberRowIds()).toEqual(['d1', 'd2']);
    // 다시 클릭 → desc(승강장=d2 먼저).
    fireEvent.click(screen.getByText('agents.detail.groups.place'));
    expect(memberRowIds()).toEqual(['d2', 'd1']);
  });

  it('빈 station/place 는 "-" 로 표시되고 여전히 선택·필터(미지정) 가능하다', () => {
    devicesMock.current = [
      device('d1', '기기1'),
      device('dU', '미지정기기', { station: '', place: '' }),
    ];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    // 미지정 행: 라인/역사/위치가 '-' placeholder.
    const rowU = screen.getByTestId('group-member-row-dU');
    expect(within(rowU).getAllByText('-').length).toBeGreaterThanOrEqual(3);
    // 미지정 필터(역사)로 걸러도 여전히 표시·선택 가능.
    fireEvent.change(screen.getByTestId('group-member-filter-station'), { target: { value: '__unassigned__' } });
    expect(screen.queryByTestId('group-member-row-d1')).toBeNull();
    expect(screen.getByTestId('group-member-row-dU')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('group-member-checkbox-dU'));
    fireEvent.change(screen.getByPlaceholderText('agents.detail.groups.namePlaceholder'), {
      target: { value: '기타' },
    });
    fireEvent.click(screen.getByTestId('group-form-submit'));
    expect(mutations.add).toHaveBeenCalledWith({ name: '기타', members: ['dU'] }, expect.anything());
  });

  it('행 전체 클릭으로도 멤버를 토글한다(체크박스 외 영역)', () => {
    devicesMock.current = [device('d1', '기기1')];
    stationsMock.current = stationRegistry;
    render(<XsfmGroupsTab agentId="a1" />);
    fireEvent.click(screen.getByText('agents.detail.groups.addGroup'));

    // 행 클릭 → 체크됨.
    fireEvent.click(screen.getByTestId('group-member-row-d1'));
    expect(screen.getByTestId('group-member-checkbox-d1')).toBeChecked();
    fireEvent.change(screen.getByPlaceholderText('agents.detail.groups.namePlaceholder'), {
      target: { value: '한개' },
    });
    fireEvent.click(screen.getByTestId('group-form-submit'));
    expect(mutations.add).toHaveBeenCalledWith({ name: '한개', members: ['d1'] }, expect.anything());
  });
});
