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
  controlMock.setPower.variables = undefined;
  controlMock.setFanSpeed.variables = undefined;
  rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
});

describe('FacilityStationPanel', () => {
  it('역사 통계 + 기기 목록을 표시한다(기본 라벨 placeIndex)', () => {
    rosterMock.current = {
      devices: [device('d1', { index: 1 }), device('d2', { index: 2, online: false })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    // 통계: 총 2대.
    expect(within(screen.getByTestId('stat-tile-total')).getByText('2')).toBeInTheDocument();
    // 기기 목록에 2개 기기(기본 placeIndex 라벨: <place표시명>-<index> = "대합실-N").
    const list = screen.getByTestId('station-device-list');
    expect(within(list).getByText('대합실-1')).toBeInTheDocument();
    expect(within(list).getByText('대합실-2')).toBeInTheDocument();
  });

  it('deviceLabelMode: 기본(placeIndex)은 <place표시명>-<index>, name 이면 name 을 보여준다', () => {
    rosterMock.current = {
      devices: [device('d1', { name: 'DEV-X', place: 'PL-1', index: 3 })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    // 기본(미지정) → placeIndex: PL-1(대합실) + '-' + 3.
    const shown = renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(within(screen.getByTestId('station-device-list')).getByText('대합실-3')).toBeInTheDocument();
    shown.unmount();
    // 'name' → 기기 name 필드.
    renderPanel({ agentId: 'a1', station: 'ST-1', deviceLabelMode: 'name' });
    expect(within(screen.getByTestId('station-device-list')).getByText('DEV-X')).toBeInTheDocument();
  });

  it('per-device 제어 버튼이 일괄 제어와 동일한 공용 스타일을 쓴다(비활성 base 일치)', () => {
    rosterMock.current = {
      devices: [device('d1', { fan_speed: 2 })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    // 비활성 풍량 버튼(레벨 3, 현재 상태 아님)은 일괄 제어 풍량 버튼과 동일한 공용 base 스타일.
    const bulkFan3 = screen.getByTestId('facility-bulk-fan-3');
    const devFan3 = screen.getByTestId('facility-device-fan-3-d1');
    expect(bulkFan3.className).toContain('bg-(--color-bg-elevated)');
    expect(devFan3.className).toContain('bg-(--color-bg-elevated)');
  });

  it('개별 풍량 버튼이 현재 fan_speed 를 레벨 색상으로 강조한다(1단 노랑·2단 초록·3단 파랑)', () => {
    // fan_speed 2(전원 ON) → 풍량 2 버튼이 레벨2(초록)로 활성, 나머지는 비활성 base.
    rosterMock.current = { devices: [device('d1', { fan_speed: 2 })], stations: [stationEntry], isLoading: false, isError: false };
    const r2 = renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-fan-2-d1').className).toContain('ring-green-500');
    expect(screen.getByTestId('facility-device-fan-1-d1').className).toContain('bg-(--color-bg-elevated)');
    expect(screen.getByTestId('facility-device-fan-3-d1').className).toContain('bg-(--color-bg-elevated)');
    r2.unmount();
    // fan_speed 1 → 레벨1(노랑).
    rosterMock.current = { devices: [device('d1', { fan_speed: 1 })], stations: [stationEntry], isLoading: false, isError: false };
    const r1 = renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-fan-1-d1').className).toContain('ring-yellow-500');
    r1.unmount();
    // fan_speed 3 → 레벨3(파랑).
    rosterMock.current = { devices: [device('d1', { fan_speed: 3 })], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-fan-3-d1').className).toContain('ring-blue-500');
  });

  it('전원 OFF 기기는 OFF 버튼을 활성 상태로 강조한다', () => {
    rosterMock.current = { devices: [device('d1', { power: false })], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-power-off-d1').className).toContain('ring-slate-500');
  });

  it('일괄 제어(OFF)가 {station} 셀렉터로 setPower(power:false) 를 호출하고 ON 버튼은 없다(A1)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.queryByTestId('facility-bulk-power-on')).toBeNull();
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ station: 'ST-1', power: false });
  });

  it('기기별 개별 제어가 {device_id} 셀렉터로 setPower/setFanSpeed 를 호출한다(C3)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    fireEvent.click(screen.getByTestId('facility-device-power-off-d1'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ device_id: 'd1', power: false });
    fireEvent.click(screen.getByTestId('facility-device-fan-2-d1'));
    expect(controlMock.setFanSpeed.mutate).toHaveBeenCalledWith({ device_id: 'd1', fan_speed: 2 });
  });

  it('전원 OFF 기기는 개별 풍량 버튼을 비활성화한다(UB-005)', () => {
    rosterMock.current = {
      devices: [device('d1', { power: false })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-fan-1-d1')).toBeDisabled();
    // 전원 OFF 버튼은 이미 꺼져 있으므로 비활성.
    expect(screen.getByTestId('facility-device-power-off-d1')).toBeDisabled();
  });

  it('showStats config 로 역사 통계(StatTiles) 섹션을 토글한다(C4)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    // 기본(미지정) → 표시.
    const shown = renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('stat-tile-total')).toBeInTheDocument();
    shown.unmount();
    // false → 숨김(기기 목록은 유지).
    renderPanel({ agentId: 'a1', station: 'ST-1', showStats: false });
    expect(screen.queryByTestId('stat-tile-total')).toBeNull();
    expect(screen.getByTestId('station-device-list')).toBeInTheDocument();
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

  it('offlineAsOff 옵션이 켜지면 오프라인 기기를 꺼짐(off)으로 표시한다(item 2)', () => {
    // 오프라인 + power:true 기기. 옵션 off(기본)이면 기존 로직(power?on) 유지, 옵션 on이면 꺼짐 표시.
    rosterMock.current = {
      devices: [device('d1', { online: false, power: true })],
      stations: [stationEntry],
      isLoading: false,
      isError: false,
    };
    // 기본(offlineAsOff 미지정) → 기존 표기(전원 상태 기반, power true → 켜짐).
    const shown = renderPanel({ agentId: 'a1', station: 'ST-1' });
    expect(screen.getByTestId('facility-device-status-d1')).toHaveTextContent(
      'dashboard.facility.device.on',
    );
    shown.unmount();
    // offlineAsOff true → 상태 텍스트가 꺼짐(off) + OFF 버튼 활성 강조.
    renderPanel({ agentId: 'a1', station: 'ST-1', offlineAsOff: true });
    expect(screen.getByTestId('facility-device-status-d1')).toHaveTextContent(
      'dashboard.facility.device.off',
    );
    expect(screen.getByTestId('facility-device-power-off-d1').className).toContain('ring-slate-500');
  });

  it('제어 진행 중이면 클릭한 버튼에 로딩(스피너/aria-busy)을 표시한다(item 3)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    // 전원 OFF 뮤테이션 진행 중 → OFF 버튼에 스피너.
    controlMock.setPower.isPending = true;
    controlMock.setPower.variables = { power: false };
    const shown = renderPanel({ agentId: 'a1', station: 'ST-1' });
    const offBtn = screen.getByTestId('facility-device-power-off-d1');
    expect(offBtn).toHaveAttribute('aria-busy', 'true');
    expect(within(offBtn).getByTestId('control-btn-spinner')).toBeInTheDocument();
    // 다른 버튼(풍량)에는 스피너가 없다.
    expect(within(screen.getByTestId('facility-device-fan-1-d1')).queryByTestId('control-btn-spinner')).toBeNull();
    shown.unmount();

    // 풍량 2 뮤테이션 진행 중 → 풍량 2 버튼에 스피너.
    controlMock.setPower.isPending = false;
    controlMock.setPower.variables = undefined;
    controlMock.setFanSpeed.isPending = true;
    controlMock.setFanSpeed.variables = { fan_speed: 2 };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    const fan2 = screen.getByTestId('facility-device-fan-2-d1');
    expect(fan2).toHaveAttribute('aria-busy', 'true');
    expect(within(fan2).getByTestId('control-btn-spinner')).toBeInTheDocument();
  });

  it('제어 실패 메시지를 상태 정보 텍스트 뒤에 인라인으로 표시한다(item 4, UB-002)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    // 단일 기기 실패 응답(status error).
    controlMock.setFanSpeed.data = { device_id: 'd1', status: 'error', error: 'boom' };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    // 실행(풍량 2 클릭) → lastAction=fan → 실패 문구 노출.
    fireEvent.click(screen.getByTestId('facility-device-fan-2-d1'));
    const status = screen.getByTestId('facility-device-status-d1');
    const failure = screen.getByTestId('facility-device-failure-d1');
    expect(failure).toHaveTextContent('dashboard.facility.device.result.error');
    // 순서: 실패 문구가 상태 정보 텍스트 뒤(DOM following)에 온다.
    expect(status.compareDocumentPosition(failure) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('제어 성공(ok) 응답에는 실패 메시지를 표시하지 않는다(item 4)', () => {
    rosterMock.current = { devices: [device('d1')], stations: [stationEntry], isLoading: false, isError: false };
    controlMock.setFanSpeed.data = { device_id: 'd1', status: 'ok' };
    renderPanel({ agentId: 'a1', station: 'ST-1' });
    fireEvent.click(screen.getByTestId('facility-device-fan-2-d1'));
    expect(screen.queryByTestId('facility-device-failure-d1')).toBeNull();
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
    expect(screen.getByTestId('facility-bulk-power-off')).toBeDisabled();
    expect(screen.getByTestId('facility-bulk-fan-1')).toBeDisabled();
  });
});
