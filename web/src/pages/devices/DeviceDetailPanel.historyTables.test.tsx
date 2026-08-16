// DeviceDetailPanel 이력 섹션의 전용 표(게이트웨이 / 측정치) 렌더 + CSV 내보내기 테스트.
//
// 검증 축:
//   - chirpstack 디바이스: 두 전용 표가 렌더되고 JSON 덩어리 컬럼이 사라진다
//   - 한쪽 속성만 있는 디바이스: 없는 쪽 표는 아예 렌더되지 않는다
//   - 비 chirpstack 디바이스: 종전과 동일하게 렌더된다 (회귀 없음)
//   - 이월 값 구분: 흐린 셀 + 실제 수신 시각 툴팁
//   - CSV: BOM + 원본 정밀도

import { render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { DeviceDetail, DeviceHistoryEntry } from '@/types/device';

function renderWithI18n(ui: ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

const useDeviceDetailTargetMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useDetailTargets', () => ({
  useDeviceDetailTarget: useDeviceDetailTargetMock,
}));

const useDeviceHistoryMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useDevice', () => ({
  useDeviceHistory: useDeviceHistoryMock,
  useExecuteCommand: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpdateMetadata: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock('@/hooks/useOptimisticToggle', () => ({
  useOptimisticToggle: (v: boolean | undefined) => ({
    displayValue: v,
    setOptimistic: vi.fn(),
    isPendingConfirmation: false,
  }),
}));

vi.mock('@/lib/remote/TargetContext', () => ({
  useTargetContext: () => ({ type: 'local' }),
}));

// CSV 다운로드는 downloadCsv 경계에서 가로챈다. Blob/<a download> 자체는
// charts/csvExport.test.ts 가 이미 다루므로, 여기서는 "무엇을 내보내는가" 만 본다.
const downloadCsvMock = vi.hoisted(() => vi.fn());
vi.mock('@/pages/dashboard/panels/charts/csvExport', () => ({
  downloadCsv: downloadCsvMock,
}));

import DeviceDetailPanel from './DeviceDetailPanel';

function makeDetail(overrides: Partial<DeviceDetail> = {}): DeviceDetail {
  return {
    id: 'uuid-1',
    uid: 'uuid-1',
    name: 'sensor-1',
    type: 'sensor',
    protocol: 'chirpstack',
    agent_name: 'agent-a',
    source: 'config',
    online: true,
    last_seen: new Date().toISOString(),
    capabilities: [],
    state: { online: true, ready: true, last_seen: '', error_count: 0, properties: {} },
    commands: [],
    ...overrides,
  };
}

function entry(overrides: Partial<DeviceHistoryEntry> = {}): DeviceHistoryEntry {
  return {
    timestamp: 1_700_000_000_000,
    online: true,
    last_seen: 1_700_000_000_000,
    properties: {},
    ...overrides,
  };
}

function gw(id: string, over: Record<string, unknown> = {}) {
  return {
    gateway_id: id,
    rssi: -95,
    snr: 9.25,
    channel: 3,
    frequency_hz: 922_100_000,
    spreading_factor: 7,
    bandwidth: 125_000,
    last_seen_ms: 1_700_000_000_000,
    stale: true,
    ...over,
  };
}

/** 이력 훅이 주어진 엔트리를 반환하도록 설정한다. */
function withHistory(entries: DeviceHistoryEntry[]) {
  useDeviceHistoryMock.mockReturnValue({
    data: { device_id: 'uuid-1', count: entries.length, entries },
    isLoading: false,
    error: null,
    isFetching: false,
  });
}

beforeEach(() => {
  useDeviceDetailTargetMock.mockReset();
  useDeviceHistoryMock.mockReset();
  downloadCsvMock.mockReset();
  useDeviceDetailTargetMock.mockReturnValue({
    data: makeDetail(),
    isLoading: false,
    error: null,
  });
});

const CHIRP_ENTRIES = [
  entry({
    timestamp: 2_000_000_000_000,
    properties: {
      gateways: [gw('aa11'), gw('bb22', { rssi: -80 })],
      measurements: {
        temperature: { value: 29.875, time_ms: 2_000_000_000_000 },
        battery: { value: 91, time_ms: 1_000_000_000_000 },
      },
    },
  }),
];

describe('DeviceDetailPanel 이력 - 전용 표 렌더', () => {
  it('chirpstack 디바이스에 게이트웨이/측정치 표를 각각 렌더한다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('게이트웨이 수신 이력')).toBeInTheDocument();
    expect(screen.getByText('측정치 이력')).toBeInTheDocument();
  });

  it('gateways/measurements 를 JSON 덩어리 셀로 렌더하지 않는다', () => {
    withHistory(CHIRP_ENTRIES);
    const { container } = renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    // 예전에는 compact JSON 이 80자에서 잘려 표시됐다.
    expect(container.textContent).not.toContain('gateway_id":');
    expect(container.textContent).not.toContain('"time_ms"');
    expect(container.textContent).not.toContain('…');
  });

  it('게이트웨이 배열을 게이트웨이마다 한 행으로 펼친다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('aa11')).toBeInTheDocument();
    expect(screen.getByText('bb22')).toBeInTheDocument();
  });

  it('게이트웨이 이력에는 stale(조회 시각 파생값) 컬럼을 렌더하지 않는다', () => {
    // 픽스처의 링크는 stale: true 지만 시계열 행에서는 의미가 뒤바뀌므로 표시하지 않는다.
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.queryByText('오래됨')).toBeNull();
    expect(screen.queryByText('링크 상태')).toBeNull();
  });

  it('변조를 SF / 대역폭 조합으로 표시한다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getAllByText('SF7 / 125 kHz').length).toBeGreaterThan(0);
  });

  it('측정치 컬럼을 키별로 렌더한다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('온도')).toBeInTheDocument();
    expect(screen.getByText('Battery')).toBeInTheDocument();
    // 값 포맷은 기존 formatPropertyValue 를 그대로 재사용한다(온도 → °C 접미).
    expect(screen.getByText('29.875°C')).toBeInTheDocument();
  });
});

describe('DeviceDetailPanel 이력 - 이월 값 구분', () => {
  it('갱신되지 않은 측정치를 흐리게 표시하고 실제 수신 시각을 툴팁으로 제공한다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    const carried = screen.getByText('91');
    expect(carried.getAttribute('title')).toContain('이월된 이전 값');
    expect(carried.className).toContain('text-(--color-text-muted)');
  });

  it('이번 시점에 수신된 측정치는 강조하고 수신 시각을 툴팁으로 제공한다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    const fresh = screen.getByText('29.875°C');
    expect(fresh.getAttribute('title')).toContain('이 시점에 수신');
    expect(fresh.className).toContain('font-medium');
  });

  it('흐림의 의미를 글로 설명하는 범례를 함께 렌더한다 (색만으로 전달하지 않는다)', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText(/흐리게 표시된 값은/)).toBeInTheDocument();
  });
});

describe('DeviceDetailPanel 이력 - 부분 데이터 / 비 chirpstack', () => {
  it('게이트웨이만 있으면 측정치 표를 렌더하지 않는다 (빈 표 금지)', () => {
    withHistory([entry({ properties: { gateways: [gw('aa11')] } })]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('게이트웨이 수신 이력')).toBeInTheDocument();
    expect(screen.queryByText('측정치 이력')).toBeNull();
  });

  it('측정치만 있으면 게이트웨이 표를 렌더하지 않는다', () => {
    withHistory([
      entry({
        timestamp: 2000,
        properties: { measurements: { temperature: { value: 21, time_ms: 2000 } } },
      }),
    ]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('측정치 이력')).toBeInTheDocument();
    expect(screen.queryByText('게이트웨이 수신 이력')).toBeNull();
  });

  it('두 속성이 모두 없는 디바이스는 종전처럼 일반 표만 렌더한다', () => {
    useDeviceDetailTargetMock.mockReturnValue({
      data: makeDetail({ protocol: 'modbus', type: 'indoor' }),
      isLoading: false,
      error: null,
    });
    withHistory([entry({ properties: { temperature: 24, note_x: 'abc123' } })]);

    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    // 전용 표는 없다.
    expect(screen.queryByText('게이트웨이 수신 이력')).toBeNull();
    expect(screen.queryByText('측정치 이력')).toBeNull();
    // 일반 표의 속성 컬럼/값은 그대로 렌더된다.
    expect(screen.getByText('온도')).toBeInTheDocument();
    expect(screen.getByText('abc123')).toBeInTheDocument();
    expect(screen.getByText('온라인')).toBeInTheDocument();
  });

  it('빈 이력이면 전용 표 없이 안내 문구만 렌더한다', () => {
    withHistory([]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('최근 데이터 없음')).toBeInTheDocument();
    expect(screen.queryByText('게이트웨이 수신 이력')).toBeNull();
    expect(screen.queryByText('측정치 이력')).toBeNull();
  });
});

describe('DeviceDetailPanel 이력 - CSV 내보내기', () => {
  /** 내보내기 버튼을 눌러 downloadCsv 에 전달된 [csv, filename] 을 돌려준다. */
  function clickExport(label: string): { csv: string; filename: string } {
    screen.getByRole('button', { name: label }).click();
    expect(downloadCsvMock).toHaveBeenCalledTimes(1);
    const [csv, filename] = downloadCsvMock.mock.calls[0] as [string, string];
    return { csv, filename };
  }

  it('게이트웨이 이력을 원본 정밀도로 내보낸다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    const { csv, filename } = clickExport('게이트웨이 수신 이력 CSV 내보내기');

    // 주파수/대역폭은 표시 단위가 아니라 원본 Hz.
    expect(csv).toContain('922100000');
    expect(csv).toContain('125000');
    expect(csv).not.toContain('MHz');
    // 게이트웨이마다 한 줄(헤더 + 2행).
    expect(csv.trimEnd().split('\n')).toHaveLength(3);
    expect(filename).toBe('device-gateway-history-uuid-1.csv');
  });

  it('측정치 이력을 값 + 수신 시각 쌍으로 내보낸다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    const { csv, filename } = clickExport('측정치 이력 CSV 내보내기');

    expect(csv).toContain('온도 (temperature)');
    expect(csv).toContain('온도 (temperature) 수신 시각');
    // 표시용 '°C' 가 아니라 원본 값을 싣는다.
    expect(csv).toContain('29.875');
    expect(csv).not.toContain('29.875°C');
    expect(filename).toBe('device-measurement-history-uuid-1.csv');
  });

  it('한글 헤더가 Excel 에서 깨지지 않도록 UTF-8 BOM 을 붙인다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(clickExport('측정치 이력 CSV 내보내기').csv.charCodeAt(0)).toBe(0xfeff);
  });

  it('두 표의 내보내기는 서로 독립적이다 (각자 다른 내용/파일명)', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    screen.getByRole('button', { name: '게이트웨이 수신 이력 CSV 내보내기' }).click();
    screen.getByRole('button', { name: '측정치 이력 CSV 내보내기' }).click();

    expect(downloadCsvMock).toHaveBeenCalledTimes(2);
    const [gwCsv, gwName] = downloadCsvMock.mock.calls[0] as [string, string];
    const [msCsv, msName] = downloadCsvMock.mock.calls[1] as [string, string];

    expect(gwName).not.toBe(msName);
    expect(gwCsv).toContain('게이트웨이 ID');
    expect(gwCsv).not.toContain('온도 (temperature)');
    expect(msCsv).toContain('온도 (temperature)');
    expect(msCsv).not.toContain('게이트웨이 ID');
  });
});

describe('DeviceDetailPanel 이력 - 상태 변경 이력 표', () => {
  it('상태 변경 이력에 타이틀을 붙인다', () => {
    withHistory(CHIRP_ENTRIES);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('상태 변경 이력')).toBeInTheDocument();
  });

  it('last_seen 이 시각 컬럼과 같으면 최근 통신 컬럼을 렌더하지 않는다', () => {
    // chirpstack 은 HistoryEventTimed 를 구현해 timestamp 자체가 업링크 시각이므로
    // last_seen 과 항상 같은 값이 되어 컬럼이 중복된다.
    withHistory([
      entry({ timestamp: 2_000_000_000_000, last_seen: 2_000_000_000_000 }),
    ]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.queryByRole('columnheader', { name: '최근 통신' })).toBeNull();
  });

  it('last_seen 이 시각과 다르면 최근 통신 컬럼을 유지한다', () => {
    // 주기 스냅샷만 하는 프로바이더는 timestamp 가 샘플링 격자라 last_seen 이
    // 별도의 정보를 갖는다 — 이 경우까지 컬럼을 지우면 정보가 사라진다.
    withHistory([
      entry({ timestamp: 2_000_000_000_000, last_seen: 1_999_999_000_000 }),
    ]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByRole('columnheader', { name: '최근 통신' })).toBeInTheDocument();
  });

  it('엔트리 중 하나라도 다르면 컬럼을 유지한다', () => {
    withHistory([
      entry({ timestamp: 2_000_000_000_000, last_seen: 2_000_000_000_000 }),
      entry({ timestamp: 1_000_000_000_000, last_seen: 999_999_000_000 }),
    ]);
    renderWithI18n(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByRole('columnheader', { name: '최근 통신' })).toBeInTheDocument();
  });
});
