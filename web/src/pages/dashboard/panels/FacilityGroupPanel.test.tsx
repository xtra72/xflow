// FacilityGroupPanel 테스트 (SPEC-XSFM-GROUP-001 Module 6, AC 6.5).
//
// config.groupId 로 지정된 "단일 그룹"(역사·라인·커스텀)을 역사 패널과 동형으로 렌더하는지 검증한다:
//   - 헤더 type 배지 + 그룹 통계(StatTiles) + 그룹 일괄 제어(group_id 셀렉터) + 소속 디바이스 개별 제어.
//   - 로스터 대조로 실재 멤버만 통계·목록에 반영(RD-3, 유령 멤버 제외).
//   - deviceLabelMode / offlineAsOff / showStats config 옵션(역사 패널에서 흡수한 커버리지).
//   - fan-out 응답 멤버별 결과 렌더(REQ-06-02).
//   - 레거시 config.station → groupId="station:<code>" 파생(하위호환) — 기존 역사 패널 스냅샷 렌더.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { AirDevice, AirStation } from '@/hooks/useStation';
import type { Group } from '@/hooks/useGroups';
import type { ControlResponse } from '@/hooks/useXsfmControl';

const rosterMock = vi.hoisted(() => ({
  current: { devices: [] as AirDevice[], stations: [] as AirStation[], isLoading: false, isError: false },
}));
const groupsMock = vi.hoisted(() => ({ current: [] as Group[] }));

const controlMock = vi.hoisted(() => ({
  setPower: {
    mutate: vi.fn(),
    data: undefined as ControlResponse | undefined,
    isPending: false,
    error: null as unknown,
    variables: undefined as { power?: boolean; fan_speed?: number } | undefined,
  },
  setFanSpeed: {
    mutate: vi.fn(),
    data: undefined as ControlResponse | undefined,
    isPending: false,
    error: null as unknown,
    variables: undefined as { power?: boolean; fan_speed?: number } | undefined,
  },
}));

vi.mock('@/hooks/useXsfmControl', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useXsfmControl')>();
  return {
    ...actual,
    useFacilityRoster: () => rosterMock.current,
    useXsfmControl: () => controlMock,
  };
});

vi.mock('@/hooks/useGroups', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useGroups')>();
  return { ...actual, useGroups: () => ({ data: groupsMock.current, isLoading: false }) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import FacilityGroupPanel from './FacilityGroupPanel';

function device(id: string, overrides: Partial<AirDevice> = {}): AirDevice {
  return {
    device_id: id,
    name: id,
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

const stationEntry: AirStation = {
  station: 'ST-1',
  line: '2호선',
  display_name: '강남역',
  order: 1,
  places: [{ place: 'PL-1', display_name: '대합실', order: 1 }],
};

const stationGroup: Group = { id: 'station:ST-1', name: '강남역', type: 'station', member_count: 2, members: ['d1', 'd2'] };
const lineGroup: Group = { id: 'line:L1', name: '2호선', type: 'line', member_count: 2, members: ['d1', 'd2'] };
const customGroup: Group = { id: 'custom:floor2', name: '2층', type: 'custom', member_count: 1, members: ['d1'] };

function renderPanel(config: Record<string, unknown>) {
  return render(<FacilityGroupPanel panelId="p1" title="설비 그룹" config={config} />);
}

beforeEach(() => {
  controlMock.setPower.mutate.mockReset();
  controlMock.setFanSpeed.mutate.mockReset();
  controlMock.setPower.data = undefined;
  controlMock.setFanSpeed.data = undefined;
  controlMock.setPower.isPending = false;
  controlMock.setFanSpeed.isPending = false;
  controlMock.setPower.error = null;
  controlMock.setFanSpeed.error = null;
  rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
  groupsMock.current = [];
});

describe('FacilityGroupPanel (config-driven single group, AC 6.5)', () => {
  it('config.groupId 로 지정된 단일 그룹을 렌더한다(type 배지 + 멤버 개별 제어 + 통계)', () => {
    rosterMock.current = { devices: [device('d1'), device('d2', { device_id: 'd2' })], stations: [stationEntry], isLoading: false, isError: false };
    groupsMock.current = [stationGroup, lineGroup, customGroup];
    renderPanel({ agentId: 'a1', groupId: 'station:ST-1' });

    // 헤더 우상단 type 배지 = 역사(station).
    expect(screen.getByTestId('facility-group-type')).toHaveTextContent('dashboard.facility.group.type.station');
    // 지정 그룹(station:ST-1)의 소속 디바이스만 개별 제어 행으로 표시(다른 그룹 선택기는 없음).
    const list = screen.getByTestId('facility-group-device-list');
    expect(within(list).getByTestId('facility-device-power-off-d1')).toBeInTheDocument();
    expect(within(list).getByTestId('facility-device-power-off-d2')).toBeInTheDocument();
    // 통계 총 2대.
    expect(within(screen.getByTestId('stat-tile-total')).getByText('2')).toBeInTheDocument();
    // 드릴다운 그룹 선택기는 제거되었다.
    expect(screen.queryByTestId('facility-group-list')).toBeNull();
  });

  it('그룹 일괄 제어(OFF)가 { group_id } 셀렉터로 setPower 를 호출한다(REQ-06-04/06-05)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ group_id: 'custom:floor2', power: false });
  });

  it('소속 디바이스 개별 제어(OFF/풍량)가 {device_id} 셀렉터로 뮤테이션을 호출한다', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    fireEvent.click(screen.getByTestId('facility-device-power-off-d1'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ device_id: 'd1', power: false });
    fireEvent.click(screen.getByTestId('facility-device-fan-3-d1'));
    expect(controlMock.setFanSpeed.mutate).toHaveBeenCalledWith({ device_id: 'd1', fan_speed: 3 });
  });

  it('통계·목록을 로스터 대조로 계산한다(실재 멤버만, 유령 멤버 무시 RD-3)', () => {
    // members=[d1, d-ghost] 이지만 로스터엔 d1 만 → total=1, 개별 행은 d1 만.
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [{ ...customGroup, member_count: 2, members: ['d1', 'd-ghost'] }];
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    expect(within(screen.getByTestId('stat-tile-total')).getByText('1')).toBeInTheDocument();
    expect(screen.getByTestId('facility-device-power-off-d1')).toBeInTheDocument();
    expect(screen.queryByTestId('facility-device-power-off-d-ghost')).toBeNull();
  });

  it('showStats=false 면 통계 타일을 숨긴다(목록/제어는 유지)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2', showStats: false });
    expect(screen.queryByTestId('stat-tile-total')).toBeNull();
    expect(screen.getByTestId('facility-device-power-off-d1')).toBeInTheDocument();
  });

  it('deviceLabelMode: 기본(placeIndex)은 <place표시명>-<index>, name 이면 name 을 보여준다', () => {
    rosterMock.current = {
      devices: [device('d1', { name: 'DEV-X', place: 'PL-1', index: 3 })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    groupsMock.current = [customGroup];
    // 기본(미지정) → placeIndex: 대합실-3.
    const shown = renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    expect(within(screen.getByTestId('facility-group-device-list')).getByText('대합실-3')).toBeInTheDocument();
    shown.unmount();
    // name → 기기 name 필드.
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2', deviceLabelMode: 'name' });
    expect(within(screen.getByTestId('facility-group-device-list')).getByText('DEV-X')).toBeInTheDocument();
  });

  it('offlineAsOff 옵션이 켜지면 오프라인 기기를 꺼짐(off)으로 표시한다', () => {
    rosterMock.current = {
      devices: [device('d1', { online: false, power: true })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    groupsMock.current = [customGroup];
    // 기본(미지정) → 전원 상태 기반(power true → 켜짐).
    const shown = renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    expect(screen.getByTestId('facility-device-status-d1')).toHaveTextContent('dashboard.facility.device.on');
    shown.unmount();
    // offlineAsOff true → 꺼짐 + OFF 버튼 활성 강조.
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2', offlineAsOff: true });
    expect(screen.getByTestId('facility-device-status-d1')).toHaveTextContent('dashboard.facility.device.off');
    expect(screen.getByTestId('facility-device-power-off-d1').className).toContain('ring-slate-500');
  });

  it('fan-out 응답의 멤버별 ok/timeout/error 요약 + 실패 상세를 렌더한다(REQ-06-02)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    controlMock.setPower.data = {
      selector: { type: 'group_id', value: 'custom:floor2' },
      results: [
        { device_id: 'd1', status: 'ok' },
        { device_id: 'd2', status: 'timeout', error: 'ErrControlTimeout' },
        { device_id: 'd3', status: 'error', error: 'boom' },
      ],
      status: 'partial',
    };
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    expect(screen.getByTestId('result-ok')).toHaveTextContent('1');
    expect(screen.getByTestId('result-timeout')).toHaveTextContent('1');
    expect(screen.getByTestId('result-error')).toHaveTextContent('1');
    const failed = screen.getByTestId('result-failed-list');
    expect(within(failed).getByText('d2')).toBeInTheDocument();
    expect(within(failed).getByText('d3')).toBeInTheDocument();
  });

  it('멤버 0 그룹은 일괄 제어를 비활성화하고 소속 안내를 표시한다', () => {
    groupsMock.current = [{ ...customGroup, member_count: 0, members: [] }];
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2' });
    expect(screen.getByTestId('facility-bulk-power-off')).toBeDisabled();
    expect(screen.getByText('dashboard.facility.group.noMembers')).toBeInTheDocument();
    expect(screen.queryByTestId('facility-group-device-list')).toBeNull();
  });

  it('지정 groupId 가 그룹 목록에 없으면 안내 문구를 표시한다', () => {
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1', groupId: 'custom:missing' });
    expect(screen.getByText('dashboard.facility.group.noGroups')).toBeInTheDocument();
  });

  it('agentId 또는 groupId 미설정이면 미설정 안내를 표시한다', () => {
    // agentId 없음.
    const shown = renderPanel({ groupId: 'custom:floor2' });
    expect(screen.getByText('dashboard.facility.notConfigured')).toBeInTheDocument();
    shown.unmount();
    // groupId 없음(레거시 station 도 없음).
    renderPanel({ agentId: 'a1' });
    expect(screen.getByText('dashboard.facility.notConfigured')).toBeInTheDocument();
  });

  // ---- 하위호환: 레거시 facility-station config.station 별칭 ----

  it('레거시 config.station 을 groupId="station:<code>" 로 파생해 그 역사 그룹을 렌더한다', () => {
    // groupId 없이 station 만 있는 기존 역사 패널 스냅샷 → station:ST-1 그룹으로 렌더.
    rosterMock.current = { devices: [device('d1'), device('d2', { device_id: 'd2' })], stations: [stationEntry], isLoading: false, isError: false };
    groupsMock.current = [stationGroup, customGroup];
    renderPanel({ agentId: 'a1', station: 'ST-1' });

    // station:ST-1 그룹(강남역, station 배지)의 멤버 d1/d2 개별 제어가 렌더된다.
    expect(screen.getByTestId('facility-group-type')).toHaveTextContent('dashboard.facility.group.type.station');
    const list = screen.getByTestId('facility-group-device-list');
    expect(within(list).getByTestId('facility-device-power-off-d1')).toBeInTheDocument();
    expect(within(list).getByTestId('facility-device-power-off-d2')).toBeInTheDocument();
    // 레거시 별칭에서도 그룹 일괄 제어는 { group_id: 'station:ST-1' } 로 fan-out.
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ group_id: 'station:ST-1', power: false });
  });

  it('config.groupId 가 있으면 레거시 station 보다 우선한다', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [stationGroup, customGroup];
    // groupId(custom:floor2) 우선 → custom 배지, d1 만.
    renderPanel({ agentId: 'a1', groupId: 'custom:floor2', station: 'ST-1' });
    expect(screen.getByTestId('facility-group-type')).toHaveTextContent('dashboard.facility.group.type.custom');
  });
});
