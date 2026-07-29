// FacilityLinePanel 테스트 (SPEC-FACILITY-DASHBOARD-001 M4).
// 라인도 노드(order 정렬) + 라인 통계 표시, line 셀렉터 일괄 제어, 빈/미등록 호선 안내(REQ-01-06),
// 미분류(미등록 station) 카운트 표기(REQ-05-03/UB-004)를 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

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

  it('행을 가로지르는 row-spanning 연결선(inset-x-0)을 그리고 노드는 order 순으로 배치된다', () => {
    // 입력 역순으로 줘도 order 오름차순 배치. 한 행 → row-spanning 연결선 1개(행당 1개).
    rosterMock.current = {
      devices: [device('d1', 'ST-1'), device('d2', 'ST-2')],
      stations: [st2, st1],
      isLoading: false,
      isError: false,
    };
    const { container } = renderPanel({ agentId: 'a1', line: '2호선' });
    // 행마다 row-spanning 연결선 1개(1행 → 1개).
    const connectors = screen.getAllByTestId('line-connector');
    expect(connectors).toHaveLength(1);
    // 연결선은 행을 가로지르는 full-width(inset-x-0) 이며 nodeSize 기준 고정폭(w-6)이 아니다.
    expect(connectors[0]?.className).toContain('inset-x-0');
    expect(connectors[0]?.className).not.toContain('w-6');
    // 행(line-row)은 패널 전체 폭 균등 분산(justify-between) 이며 콘텐츠 폭(w-max) 좌측 정렬이 아니다.
    const rows = screen.getAllByTestId('line-row');
    expect(rows).toHaveLength(1);
    expect(rows[0]?.className).toContain('justify-between');
    expect(rows[0]?.className).not.toContain('w-max');
    // full-width 균등 분산이므로 라인도는 가로 스크롤 컨테이너가 아니다.
    expect(screen.getByTestId('line-diagram').className).not.toContain('overflow-x-auto');
    // 역 마커(점)는 노드 순서와 동일하게 order 정렬(강남역 → 역삼역)된다.
    const nodes = Array.from(container.querySelectorAll('[data-testid="diagram-node"]')).map((n) =>
      n.getAttribute('data-station'),
    );
    expect(nodes).toEqual(['ST-1', 'ST-2']);
  });

  it('여러 행이면 각 행마다 row-spanning 연결선을 그린다(stationsPerRow 4, 13 → 4행 4연결선)', () => {
    const stations: AirStation[] = Array.from({ length: 13 }, (_, i) => ({
      station: `ST-${i}`,
      line: '2호선',
      display_name: `역${i}`,
      order: i,
      places: [],
    }));
    rosterMock.current = {
      devices: stations.map((s) => device(`d-${s.station}`, s.station)),
      stations,
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선', stationsPerRow: 4 });
    // 4행 → 각 행 full-width 균등 분산 + 행마다 연결선 1개.
    expect(screen.getAllByTestId('line-row')).toHaveLength(4);
    expect(screen.getAllByTestId('line-connector')).toHaveLength(4);
  });

  it('stationsPerRow 를 지정하면 정확히 그 수로 행을 나눈다(13, spr 4 → 4/4/4/1)', () => {
    // 사용자 지정 1줄당 역사 수 = 정확 청킹(마지막 행만 더 적을 수 있음).
    const stations: AirStation[] = Array.from({ length: 13 }, (_, i) => ({
      station: `ST-${i}`,
      line: '2호선',
      display_name: `역${i}`,
      order: i,
      places: [],
    }));
    rosterMock.current = {
      devices: stations.map((s) => device(`d-${s.station}`, s.station)),
      stations,
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선', stationsPerRow: 4 });
    const rows = screen.getAllByTestId('line-row');
    expect(rows).toHaveLength(4);
    const counts = rows.map((r) => r.querySelectorAll('[data-testid="diagram-node"]').length);
    expect(counts).toEqual([4, 4, 4, 1]);
  });

  it('stationsPerRow 가 0/무효면 nodeSize 기반 자동 행 배분으로 폴백한다(13 → 7+6)', () => {
    const stations: AirStation[] = Array.from({ length: 13 }, (_, i) => ({
      station: `ST-${i}`,
      line: '2호선',
      display_name: `역${i}`,
      order: i,
      places: [],
    }));
    rosterMock.current = {
      devices: stations.map((s) => device(`d-${s.station}`, s.station)),
      stations,
      isLoading: false,
      isError: false,
    };
    // stationsPerRow: 0 → 무효 → 자동(레벨2 최대 8) 폴백.
    renderPanel({ agentId: 'a1', line: '2호선', nodeSize: '2', stationsPerRow: 0 });
    const counts = screen
      .getAllByTestId('line-row')
      .map((r) => r.querySelectorAll('[data-testid="diagram-node"]').length);
    expect(counts).toEqual([7, 6]);
  });

  it('노드 수가 많으면 균등한 여러 행으로 배분한다(13 → 7+6)', () => {
    // 레벨 '2'(한 행 최대 8) → 13개는 2행으로 7+6 균등 배분(그리디 8+5 아님).
    const stations: AirStation[] = Array.from({ length: 13 }, (_, i) => ({
      station: `ST-${i}`,
      line: '2호선',
      display_name: `역${i}`,
      order: i,
      places: [],
    }));
    rosterMock.current = {
      devices: stations.map((s) => device(`d-${s.station}`, s.station)),
      stations,
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선', nodeSize: '2' });
    const rows = screen.getAllByTestId('line-row');
    expect(rows).toHaveLength(2);
    const counts = rows.map((r) => r.querySelectorAll('[data-testid="diagram-node"]').length);
    expect(counts).toEqual([7, 6]);
  });

  it('일괄 제어(OFF)가 {line} 셀렉터로 setPower(power:false) 를 호출하고 ON 버튼은 없다(A1)', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    renderPanel({ agentId: 'a1', line: '2호선' });
    // 전원 ON 버튼은 제거되었다.
    expect(screen.queryByTestId('facility-bulk-power-on')).toBeNull();
    fireEvent.click(screen.getByTestId('facility-bulk-power-off'));
    expect(controlMock.setPower.mutate).toHaveBeenCalledWith({ line: '2호선', power: false });
  });

  it('빈/미등록 호선이면 안내를 표시한다(REQ-01-06)', () => {
    rosterMock.current = { devices: [], stations: [], isLoading: false, isError: false };
    renderPanel({ agentId: 'a1', line: '없는호선' });
    expect(screen.getByText('dashboard.facility.noData')).toBeInTheDocument();
    // 일괄 제어 컨트롤은 렌더되지 않는다.
    expect(screen.queryByTestId('facility-bulk-power-off')).toBeNull();
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
    expect(screen.getByTestId('facility-bulk-power-off')).toBeInTheDocument();
  });

  it('nodeSize 5단계를 노드 크기에 적용하고 레거시 lg 를 레벨3 으로 매핑한다', () => {
    rosterMock.current = {
      devices: [device('d1', 'ST-1')],
      stations: [st1],
      isLoading: false,
      isError: false,
    };
    // 레거시 'lg' → 레벨3(min-w-44) 하위 호환.
    const legacy = renderPanel({ agentId: 'a1', line: '2호선', nodeSize: 'lg' });
    expect(legacy.container.querySelector('[data-testid="diagram-node"]')?.className).toContain('min-w-44');
    legacy.unmount();
    // 신규 최상위 레벨 '5' → min-w-64.
    const level5 = renderPanel({ agentId: 'a1', line: '2호선', nodeSize: '5' });
    expect(level5.container.querySelector('[data-testid="diagram-node"]')?.className).toContain('min-w-64');
  });

  it('offlineAsOff 가 켜지면 전부 오프라인 역사를 꺼짐(off)으로 표시한다', () => {
    const stations: AirStation[] = [
      { station: 'ST-O', line: '2호선', display_name: '전부오프역', order: 1, places: [] },
    ];
    rosterMock.current = {
      devices: [device('o1', 'ST-O', { online: false })],
      stations,
      isLoading: false,
      isError: false,
    };
    // 기본(offlineAsOff 미지정) → offline.
    const off = renderPanel({ agentId: 'a1', line: '2호선' });
    expect(nodeStatus(off.container, 'ST-O')).toBe('offline');
    off.unmount();
    // offlineAsOff true → off 로 표시.
    const on = renderPanel({ agentId: 'a1', line: '2호선', offlineAsOff: true });
    expect(nodeStatus(on.container, 'ST-O')).toBe('off');
  });
});
