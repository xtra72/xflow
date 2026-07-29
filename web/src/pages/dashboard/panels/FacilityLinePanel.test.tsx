// FacilityLinePanel 테스트 (SPEC-FACILITY-DASHBOARD-001 M4).
// 라인도 노드(order 정렬) + 라인 통계 표시, line 셀렉터 일괄 제어, 빈/미등록 호선 안내(REQ-01-06),
// 미분류(미등록 station) 카운트 표기(REQ-05-03/UB-004)를 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { AirDevice, AirStation } from '@/hooks/useStation';
import type { ControlResponse } from '@/hooks/useAirpurifierControl';

const rosterMock = vi.hoisted(() => ({
  current: {
    devices: [] as AirDevice[],
    stations: [] as AirStation[],
    isLoading: false,
    isError: false,
  },
}));

const controlMock = vi.hoisted(() => ({
  setPower: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
  setFanSpeed: { mutate: vi.fn(), data: undefined as ControlResponse | undefined, isPending: false, error: null as unknown },
}));

vi.mock('@/hooks/useAirpurifierControl', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useAirpurifierControl')>();
  return {
    ...actual,
    useFacilityRoster: () => rosterMock.current,
    useAirpurifierControl: () => controlMock,
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import FacilityLinePanel from './FacilityLinePanel';

function device(id: string, station: string, overrides: Partial<AirDevice> = {}): AirDevice {
  return {
    device_id: id,
    name: id,
    group_id: '',
    station,
    place: 'PL-1',
    index: 1,
    online: true,
    power: true,
    fan_speed: 1,
    source: 'config',
    ...overrides,
  };
}

const st1: AirStation = { station: 'ST-1', line: '2호선', display_name: '강남역', order: 1, places: [] };
const st2: AirStation = { station: 'ST-2', line: '2호선', display_name: '역삼역', order: 2, places: [] };

function renderPanel(config: Record<string, unknown>) {
  return render(<FacilityLinePanel panelId="p1" title="라인" config={config} />);
}

beforeEach(() => {
  controlMock.setPower.mutate.mockReset();
  controlMock.setFanSpeed.mutate.mockReset();
  controlMock.setPower.data = undefined;
  controlMock.setFanSpeed.data = undefined;
  controlMock.setPower.isPending = false;
  controlMock.setFanSpeed.isPending = false;
  rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
});

describe('FacilityLinePanel', () => {
  it('라인도 노드를 order 순으로 정렬해 렌더하고 라인 통계를 표시한다', () => {
    // 입력 순서를 역순(order 2 먼저)으로 줘도 order 오름차순으로 정렬되어야 한다.
    rosterMock.current = {
      devices: [device('d1', 'ST-1'), device('d2', 'ST-2')],
      stations: [st2, st1],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선' });
    const diagram = screen.getByTestId('line-diagram');
    expect(within(diagram).getByText('강남역')).toBeInTheDocument();
    expect(within(diagram).getByText('역삼역')).toBeInTheDocument();
    // order 정렬: 강남역(order1) 이 역삼역(order2) 보다 앞.
    const text = diagram.textContent ?? '';
    expect(text.indexOf('강남역')).toBeLessThan(text.indexOf('역삼역'));
    // 라인 통계: 총 2대.
    expect(within(screen.getByTestId('stat-tile-total')).getByText('2')).toBeInTheDocument();
  });

  it('일괄 제어가 {line} 셀렉터로 setPower 를 호출한다(REQ-01-05)', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선' });
    fireEvent.click(screen.getByTestId('facility-bulk-power-on'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ line: '2호선', power: true });
  });

  it('빈/미등록 호선이면 안내를 표시한다(REQ-01-06)', () => {
    rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', line: '없는호선' });
    expect(screen.getByText('dashboard.facility.noData')).toBeInTheDocument();
    // 일괄 제어 컨트롤은 렌더되지 않는다.
    expect(screen.queryByTestId('facility-bulk-power-on')).toBeNull();
  });

  it('미분류(미등록 station) 기기가 있으면 카운트를 표기한다(REQ-05-03, UB-004)', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1'), device('dx', 'ST-UNREG')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선' });
    const note = screen.getByTestId('unclassified-note');
    expect(within(note).getByText('1')).toBeInTheDocument();
  });
});
