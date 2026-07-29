// FacilityStationPanel 테스트 (SPEC-FACILITY-DASHBOARD-001 M3).
// 역사 통계 + 기기 목록 표시, station 셀렉터 일괄 제어 호출, fan-out 멤버별 결과 렌더(REQ-06-02),
// 미등록 역사 안내 + 컨트롤 비활성(REQ-02-05)을 검증한다.

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

import FacilityStationPanel from './FacilityStationPanel';

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

function renderPanel(config: Record<string, unknown>) {
  return render(<FacilityStationPanel panelId="p1" title="역사" config={config} />);
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
});

describe('FacilityStationPanel', () => {
  it('역사 통계 + 기기 목록을 표시한다', () => {
    rosterMock.current = {
      devices: [device('d1'), device('d2', { online: false })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    // 통계: 총 2대.
    expect(within(screen.getByTestId('stat-tile-total')).getByText('2')).toBeInTheDocument();
    // 기기 목록에 2개 기기.
    const list = screen.getByTestId('station-device-list');
    expect(within(list).getByText('d1')).toBeInTheDocument();
    expect(within(list).getByText('d2')).toBeInTheDocument();
  });

  it('일괄 전원 제어가 {station} 셀렉터로 setPower 를 호출한다(REQ-02-04)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    fireEvent.click(screen.getByTestId('facility-bulk-power-on'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ station: 'ST-1', power: true });
  });

  it('fan-out 응답의 멤버별 ok/timeout/error 요약 + 실패 상세를 렌더한다(REQ-06-02)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    controlMock.setPower.data = {
      selector: { type: 'station', value: 'ST-1' },
      results: [
        { device_id: 'd1', status: 'ok' },
        { device_id: 'd2', status: 'timeout', error: 'ErrControlTimeout' },
        { device_id: 'd3', status: 'error', error: 'boom' },
      ],
      status: 'partial',
    };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    // 요약 카운트.
    expect(screen.getByTestId('result-ok')).toHaveTextContent('1');
    expect(screen.getByTestId('result-timeout')).toHaveTextContent('1');
    expect(screen.getByTestId('result-error')).toHaveTextContent('1');
    // 실패/타임아웃 멤버 상세.
    const failed = screen.getByTestId('result-failed-list');
    expect(within(failed).getByText('d2')).toBeInTheDocument();
    expect(within(failed).getByText('d3')).toBeInTheDocument();
    expect(within(failed).queryByText('d1')).toBeNull();
  });

  it('미등록 역사이면 안내 + 일괄 제어를 비활성화한다(REQ-02-05)', () => {
    // stations 에 ST-9 미등록. 로스터에는 해당 기기가 존재.
    rosterMock.current = {
      devices: [device('d1', { station: 'ST-9' })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', station: 'ST-9' });
    expect(screen.getByText('dashboard.facility.unregisteredStation')).toBeInTheDocument();
    expect(screen.getByTestId('facility-bulk-power-on')).toBeDisabled();
    expect(screen.getByTestId('facility-bulk-fan-1')).toBeDisabled();
  });
});
