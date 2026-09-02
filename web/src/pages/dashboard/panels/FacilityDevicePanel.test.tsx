// FacilityDevicePanel 테스트 (SPEC-FACILITY-DASHBOARD-001 M2).
// useFacilityRoster / useXsfmControl 을 mock 하여 상태 표시, set_power 호출 인자,
// timeout 응답의 명시 표시(REQ-03-02), 전원 OFF 시 풍량 비활성(REQ-03-03/UB-005)을 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { AirDevice, AirStation } from '@/hooks/useStation';
import type { ControlResponse } from '@/hooks/useXsfmControl';

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

vi.mock('@/hooks/useXsfmControl', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useXsfmControl')>();
  return {
    ...actual,
    useFacilityRoster: () => rosterMock.current,
    useXsfmControl: () => controlMock,
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import FacilityDevicePanel from './FacilityDevicePanel';

function device(overrides: Partial<AirDevice> = {}): AirDevice {
  return {
    device_id: 'd1',
    name: 'ST-1:PL-1:1',
    group_id: '',
    station: 'ST-1',
    place: 'PL-1',
    index: 1,
    online: true,
    power: true,
    fan_speed: 2,
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
  return render(<FacilityDevicePanel panelId="p1" title="기기" config={config} />);
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

describe('FacilityDevicePanel', () => {
  it('미설정(deviceId 없음)이면 안내를 표시한다', () => {
    renderPanel({ agentId: 'a1' });
    expect(screen.getByText('dashboard.facility.notConfigured')).toBeInTheDocument();
  });

  it('기기 미발견이면 안내를 표시한다', () => {
    rosterMock.current = { devices: [], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', deviceId: 'missing' });
    expect(screen.getByText('dashboard.facility.deviceNotFound')).toBeInTheDocument();
  });

  it('기기 상태(전원/풍량/역사명/인덱스)를 표시한다', () => {
    rosterMock.current = { devices: [device()], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    // 역사 표시명(레지스트리 해석) + 위치 표시명.
    expect(screen.getByText('강남역')).toBeInTheDocument();
    expect(screen.getByText('대합실')).toBeInTheDocument();
    // 전원 ON 상태(power 필드 = 켜짐).
    expect(screen.getByText('dashboard.facility.device.on')).toBeInTheDocument();
  });

  it('전원 OFF 클릭 시 setPower 를 {device_id, power:false} 로 호출한다', () => {
    rosterMock.current = { devices: [device({ power: true })], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    fireEvent.click(screen.getByTestId('facility-device-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ device_id: 'd1', power: false });
  });

  it('풍량 클릭 시 setFanSpeed 를 {device_id, fan_speed} 로 호출한다', () => {
    rosterMock.current = { devices: [device({ power: true })], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    fireEvent.click(screen.getByTestId('facility-device-fan-3'));
    expect(controlMock.setFanSpeed.mutate).toHaveBeenCalledWith({ device_id: 'd1', fan_speed: 3 });
  });

  it('timeout 응답을 성공이 아닌 타임아웃으로 명시 표시한다(REQ-03-02)', () => {
    rosterMock.current = { devices: [device()], stations: [stationEntry], isLoading: false, isError: false };
    controlMock.setPower.data = { device_id: 'd1', status: 'timeout', error: 'ErrControlTimeout' };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    expect(screen.getByText('dashboard.facility.device.result.timeout')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.facility.device.result.ok')).toBeNull();
  });

  it('ok 응답을 성공(에코 반영)으로 표시한다', () => {
    rosterMock.current = { devices: [device()], stations: [stationEntry], isLoading: false, isError: false };
    controlMock.setPower.data = { device_id: 'd1', status: 'ok' };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    expect(screen.getByText('dashboard.facility.device.result.ok')).toBeInTheDocument();
  });

  it('전원 OFF 이면 풍량 컨트롤을 비활성화한다(REQ-03-03, UB-005)', () => {
    rosterMock.current = { devices: [device({ power: false })], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', deviceId: 'd1' });
    expect(screen.getByTestId('facility-device-fan-1')).toBeDisabled();
    expect(screen.getByTestId('facility-device-fan-2')).toBeDisabled();
    expect(screen.getByTestId('facility-device-fan-3')).toBeDisabled();
    // 전원 OFF 안내 힌트 표시.
    expect(screen.getByText('dashboard.facility.device.fanDisabledHint')).toBeInTheDocument();
  });
});
