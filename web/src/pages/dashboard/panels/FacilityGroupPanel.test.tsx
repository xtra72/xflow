// FacilityGroupPanel 테스트 (SPEC-XSFM-GROUP-001 Module 6, AC 6.5).
//
// 설비 그룹 패널이 역사(station)·라인(line)·커스텀(custom) 그룹을 한 종류로 표시하고,
// 각 그룹에 type 배지 + 멤버 수 + 통계(StatTiles) + 그룹 일괄 제어(group_id 셀렉터)를 렌더하는지,
// 로스터 대조로 실재 멤버만 통계에 반영하는지 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { AirDevice } from '@/hooks/useStation';
import type { Group } from '@/hooks/useGroups';
import type { ControlResponse } from '@/hooks/useXsfmControl';

const rosterMock = vi.hoisted(() => ({
  current: { devices: [] as AirDevice[], stations: [], isLoading: false, isError: false },
}));
const groupsMock = vi.hoisted(() => ({ current: [] as Group[] }));

const controlMock = vi.hoisted(() => ({
  setPower: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
  setFanSpeed: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
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

const stationGroup: Group = { id: 'station:ST-1', name: '강남역', type: 'station', member_count: 2, members: ['d1', 'd2'] };
const lineGroup: Group = { id: 'line:L1', name: '2호선', type: 'line', member_count: 2, members: ['d1', 'd2'] };
const customGroup: Group = { id: 'custom:floor2', name: '2층', type: 'custom', member_count: 1, members: ['d1'] };

function renderPanel(config: Record<string, unknown>) {
  return render(<FacilityGroupPanel panelId="p1" title="설비 그룹" config={config} />);
}

beforeEach(() => {
  controlMock.setPower.mutate.mockReset();
  controlMock.setFanSpeed.mutate.mockReset();
  rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
  groupsMock.current = [];
});

describe('FacilityGroupPanel (AC 6.5)', () => {
  it('역사·라인·커스텀 그룹을 한 종류로 표시하고 type 배지 + 멤버 수를 렌더한다', () => {
    rosterMock.current = { devices: [device('d1'), device('d2')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [stationGroup, lineGroup, customGroup];
    renderPanel({ agentId: 'a1' });

    const list = screen.getByTestId('facility-group-list');
    expect(within(list).getByTestId('facility-group-item-station:ST-1')).toBeInTheDocument();
    expect(within(list).getByTestId('facility-group-item-line:L1')).toBeInTheDocument();
    expect(within(list).getByTestId('facility-group-item-custom:floor2')).toBeInTheDocument();
    // type 배지(역사가 그룹의 한 종류로 표시됨 — REQ-06-05).
    expect(screen.getByTestId('facility-group-type-station:ST-1')).toHaveTextContent(
      'dashboard.facility.group.type.station',
    );
    expect(screen.getByTestId('facility-group-type-line:L1')).toHaveTextContent('dashboard.facility.group.type.line');
    expect(screen.getByTestId('facility-group-type-custom:floor2')).toHaveTextContent(
      'dashboard.facility.group.type.custom',
    );
  });

  it('그룹별 통계(StatTiles)를 로스터 대조로 계산한다(실재 멤버만, 유령 멤버 무시)', () => {
    // custom 그룹 members=[d1, d-ghost] 이지만 로스터엔 d1(online) 만 존재 → total=1.
    rosterMock.current = { devices: [device('d1', { online: true })], stations: [], isLoading: false, isError: false };
    groupsMock.current = [{ ...customGroup, member_count: 2, members: ['d1', 'd-ghost'] }];
    renderPanel({ agentId: 'a1' });
    // StatTiles total 타일 = 1(로스터에 실재하는 d1 만).
    expect(within(screen.getByTestId('stat-tile-total')).getByText('1')).toBeInTheDocument();
    expect(within(screen.getByTestId('stat-tile-online')).getByText('1')).toBeInTheDocument();
  });

  it('showStats=false 면 통계 타일을 숨긴다(목록/제어는 유지)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1', showStats: false });
    expect(screen.queryByTestId('stat-tile-total')).toBeNull();
    expect(screen.getByTestId('facility-group-item-custom:floor2')).toBeInTheDocument();
  });

  it('그룹 일괄 제어(OFF)가 { group_id } 셀렉터로 setPower 를 호출한다(REQ-06-04/06-05)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [], isLoading: false, isError: false };
    groupsMock.current = [customGroup];
    renderPanel({ agentId: 'a1' });
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ group_id: 'custom:floor2', power: false });
  });

  it('멤버 0 그룹은 일괄 제어를 비활성화한다', () => {
    groupsMock.current = [{ ...customGroup, member_count: 0, members: [] }];
    renderPanel({ agentId: 'a1' });
    expect(screen.getByTestId('facility-bulk-power-off')).toBeDisabled();
  });

  it('그룹이 없으면 안내 문구를 표시한다', () => {
    groupsMock.current = [];
    renderPanel({ agentId: 'a1' });
    expect(screen.getByText('dashboard.facility.group.noGroups')).toBeInTheDocument();
  });

  it('agentId 미설정이면 미설정 안내를 표시한다', () => {
    renderPanel({});
    expect(screen.getByText('dashboard.facility.notConfigured')).toBeInTheDocument();
  });
});
