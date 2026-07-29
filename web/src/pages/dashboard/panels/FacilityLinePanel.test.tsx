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

/** 라인도에서 특정 역사 노드의 data-status 를 읽는다. */
function nodeStatus(container: HTMLElement, station: string): string | null {
  return container.querySelector(`[data-station="${station}"]`)?.getAttribute('data-status') ?? null;
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

  it('역사 노드 상태 점을 집계 우선순위대로 도출한다(offline/warning/off/normal)', () => {
    const stations: AirStation[] = [
      { station: 'ST-N', line: '2호선', display_name: '정상역', order: 1, places: [] },
      { station: 'ST-W', line: '2호선', display_name: '경고역', order: 2, places: [] },
      { station: 'ST-F', line: '2호선', display_name: '꺼짐역', order: 3, places: [] },
      { station: 'ST-O', line: '2호선', display_name: '전부오프역', order: 4, places: [] },
    ];
    rosterMock.current = {
      devices: [
        // ST-N: 온라인 + 가동 → normal
        device('n1', 'ST-N', { online: true, power: true }),
        // ST-W: 일부 오프라인 → warning
        device('w1', 'ST-W', { online: true, power: true }),
        device('w2', 'ST-W', { online: false }),
        // ST-F: 전부 온라인·전원 꺼짐 → off
        device('f1', 'ST-F', { online: true, power: false }),
        // ST-O: 전부 오프라인 → offline
        device('o1', 'ST-O', { online: false }),
      ],
      stations,
      isLoading: false,
      isError: false,
    };
    const { container } = renderPanel({ agentId: 'a1', line: '2호선' });
    expect(nodeStatus(container, 'ST-N')).toBe('normal');
    expect(nodeStatus(container, 'ST-W')).toBe('warning');
    expect(nodeStatus(container, 'ST-F')).toBe('off');
    expect(nodeStatus(container, 'ST-O')).toBe('offline');
  });

  it('상세 모드는 기기 행(<위치명>-<index> + 풍량 배지)을 표시하고 심플 모드는 감춘다', () => {
    const stationWithPlace: AirStation = {
      station: 'ST-P',
      line: '2호선',
      display_name: '역P',
      order: 1,
      places: [{ place: 'PL-1', display_name: '승강장', order: 1 }],
    };
    rosterMock.current = {
      devices: [device('d1', 'ST-P', { place: 'PL-1', index: 1, online: true, power: true, fan_speed: 2 })],
      stations: [stationWithPlace],
      isLoading: false,
      isError: false,
    };
    const { container } = renderPanel({ agentId: 'a1', line: '2호선' });
    // 기본(상세): place code 를 표시명으로 해석한 행 + 풍량 배지(fan2).
    expect(screen.getByText('승강장-1')).toBeInTheDocument();
    expect(container.querySelector('[data-testid="device-fan-badge"]')?.getAttribute('data-fan')).toBe(
      'fan2',
    );
    // 심플 전환: 기기 행이 사라진다.
    fireEvent.click(screen.getByTestId('line-mode-simple'));
    expect(screen.queryByText('승강장-1')).toBeNull();
    expect(container.querySelector('[data-testid="device-fan-badge"]')).toBeNull();
    // 다시 상세로 전환하면 복귀한다.
    fireEvent.click(screen.getByTestId('line-mode-detail'));
    expect(screen.getByText('승강장-1')).toBeInTheDocument();
  });

  it('showStationStatus/showLineStats config 로 섹션을 토글한다', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    // 기본값(미지정) → 두 섹션 모두 표시.
    const { unmount } = renderPanel({ agentId: 'a1', line: '2호선' });
    expect(screen.getByTestId('line-station-list')).toBeInTheDocument();
    expect(screen.getByTestId('stat-tile-total')).toBeInTheDocument();
    unmount();

    // false → 두 섹션 숨김(라인도 + 일괄 제어는 유지).
    renderPanel({ agentId: 'a1', line: '2호선', showStationStatus: false, showLineStats: false });
    expect(screen.queryByTestId('line-station-list')).toBeNull();
    expect(screen.queryByTestId('stat-tile-total')).toBeNull();
    expect(screen.getByTestId('line-diagram')).toBeInTheDocument();
    expect(screen.getByTestId('facility-bulk-power-on')).toBeInTheDocument();
  });

  it('nodeSize config 를 노드 크기에 적용한다(lg)', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    const { container } = renderPanel({ agentId: 'a1', line: '2호선', nodeSize: 'lg' });
    const node = container.querySelector('[data-testid="diagram-node"]');
    expect(node?.className).toContain('min-w-44');
  });
});
