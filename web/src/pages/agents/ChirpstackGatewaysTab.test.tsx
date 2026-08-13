// ChirpstackGatewaysTab — 게이트웨이 로스터 렌더 / 확장 서브리스트 / (게이트웨이, 디바이스)
// 쌍 보존 / stale 표시 / 빈 상태 한계 고지 테스트 (SPEC-CHIRPSTACK-003 AC-8, AC-8b).
//
// execAgent 를 command 로 라우팅 mock 하여 실제 훅/컴포넌트를 렌더한다
// (XsfmStationsTab.test.tsx 관용구). i18n 은 실제 ko 번역을 사용한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

const execAgentMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/agentService', () => ({ execAgent: execAgentMock }));

import ChirpstackGatewaysTab from './ChirpstackGatewaysTab';

const NOW = Date.now();
const FRESH = NOW - 10_000;
const OLD = NOW - 600_000;

// 핵심 픽스처: aaaa0001 은 gwA / gwB 두 게이트웨이 아래에 각자의 rssi/snr/channel 로
// 등장한다 — (게이트웨이, 디바이스) 쌍이 보존되어야 하며 중복 제거되면 안 된다.
const GATEWAYS = [
  {
    gateway_id: 'gwA',
    device_count: 2,
    last_seen_ms: FRESH,
    devices: [
      {
        dev_eui: 'aaaa0001',
        device_id: 'c7fc7745-0000-4000-8000-000000000001',
        device_name: 'dev-aaaa0001',
        device_profile_name: 'WS301',
        rssi: -87,
        snr: 9.25,
        channel: 3,
        frequency_hz: 922100000,
        spreading_factor: 7,
        bandwidth: 125000,
        last_seen_ms: FRESH,
        stale: false,
      },
      {
        dev_eui: 'bbbb0002',
        device_id: 'c7fc7745-0000-4000-8000-000000000002',
        device_name: 'dev-bbbb0002',
        device_profile_name: 'WS202',
        rssi: -70,
        snr: 12,
        channel: 0,
        frequency_hz: 922300000,
        spreading_factor: 9,
        bandwidth: 125000,
        last_seen_ms: OLD,
        stale: true,
      },
    ],
  },
  {
    gateway_id: 'gwB',
    device_count: 1,
    last_seen_ms: FRESH,
    devices: [
      {
        dev_eui: 'aaaa0001',
        device_id: 'c7fc7745-0000-4000-8000-000000000001',
        device_name: 'dev-aaaa0001',
        device_profile_name: 'WS301',
        rssi: -95,
        snr: 3.5,
        channel: 5,
        frequency_hz: 922100000,
        spreading_factor: 7,
        bandwidth: 125000,
        last_seen_ms: FRESH,
        stale: false,
      },
    ],
  },
];

function mockGateways(gateways: unknown) {
  execAgentMock.mockImplementation((_id: string, arg: { command: string }) => {
    if (arg.command === 'list_gateways') return Promise.resolve({ gateways });
    return Promise.resolve({});
  });
}

beforeEach(() => {
  execAgentMock.mockReset();
  mockGateways(GATEWAYS);
});

function renderTab() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <ChirpstackGatewaysTab agentId="agent-1" />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

/** 확장된 디바이스 행의 셀 텍스트 배열.
 *  컬럼 순서: devEui, name, profile, rssi, snr, channel, frequency, modulation, lastSeen, state. */
function deviceRowCells(devEui: string): string[][] {
  return screen
    .getAllByText(devEui)
    .map((el) => el.closest('tr'))
    .filter((tr): tr is HTMLTableRowElement => tr !== null)
    .map((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent ?? ''));
}

describe('ChirpstackGatewaysTab 로스터 렌더', () => {
  it('list_gateways 로 조회해 게이트웨이별 행을 hex ID 로 렌더한다', async () => {
    renderTab();

    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());
    expect(screen.getByText('gwB')).toBeTruthy();
    expect(screen.getByText('게이트웨이 2개')).toBeTruthy();

    const listCalls = execAgentMock.mock.calls.filter((c) => c[1]?.command === 'list_gateways');
    expect(listCalls.length).toBeGreaterThanOrEqual(1);
  });

  it('게이트웨이 행을 펼치면 연결 디바이스 서브리스트가 나타난다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());

    // 접힌 상태에서는 디바이스 행이 없다.
    expect(screen.queryByText('bbbb0002')).toBeNull();

    const [expandA] = screen.getAllByRole('button', { name: '디바이스 목록 펼치기' });
    fireEvent.click(expandA!);

    expect(screen.getByText('bbbb0002')).toBeTruthy();
    expect(screen.getByText('dev-bbbb0002')).toBeTruthy();
    expect(screen.getByText('WS301')).toBeTruthy();
  });

  it('여러 게이트웨이를 동시에 펼칠 수 있다(다중 확장)', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());

    const toggles = screen.getAllByRole('button', { name: '디바이스 목록 펼치기' });
    expect(toggles).toHaveLength(2);
    fireEvent.click(toggles[0]!);
    fireEvent.click(screen.getAllByRole('button', { name: '디바이스 목록 펼치기' })[0]!);

    // 두 게이트웨이 모두 펼쳐진 상태 → 접기 버튼 2개.
    expect(screen.getAllByRole('button', { name: '디바이스 목록 접기' })).toHaveLength(2);
  });
});

describe('ChirpstackGatewaysTab (게이트웨이, 디바이스) 쌍 보존', () => {
  it('같은 디바이스가 두 게이트웨이 아래에 각자의 rssi/snr/channel 로 표시된다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());

    // 두 게이트웨이를 모두 펼친다.
    fireEvent.click(screen.getAllByRole('button', { name: '디바이스 목록 펼치기' })[0]!);
    fireEvent.click(screen.getAllByRole('button', { name: '디바이스 목록 펼치기' })[0]!);

    const rows = deviceRowCells('aaaa0001');
    // 중복 제거되지 않고 게이트웨이마다 1행씩 = 2행.
    expect(rows).toHaveLength(2);

    // rssi(3) / snr(4) / channel(5) 이 게이트웨이별로 다르다.
    const linkTriples = rows.map((cells) => [cells[3], cells[4], cells[5]]);
    expect(linkTriples).toContainEqual(['-87', '9.25', '3']);
    expect(linkTriples).toContainEqual(['-95', '3.5', '5']);

    // frequency_hz 는 프레임 레벨이라 두 링크에서 동일하다.
    expect(rows.map((cells) => cells[6])).toEqual(['922.1 MHz', '922.1 MHz']);
  });

  it('채널 컬럼은 게이트웨이 IF 채널로 표기되고 주파수와 분리되어 있다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());
    fireEvent.click(screen.getAllByRole('button', { name: '디바이스 목록 펼치기' })[0]!);

    const channelHeader = screen.getByText('게이트웨이 IF 채널');
    expect(channelHeader).toBeTruthy();
    // 채널 헤더가 "주파수" 로 표기되지 않는다.
    expect(channelHeader.textContent).not.toContain('주파수');
    // 주파수는 별도 컬럼이며 frequency_hz 에서 온다.
    expect(screen.getByText('주파수')).toBeTruthy();
    expect(screen.getAllByText('922.1 MHz').length).toBeGreaterThan(0);
    // 채널 0 은 "미상" 이 아니라 0 으로 그대로 표시된다(proto3 기본값 생략 규약).
    expect(deviceRowCells('bbbb0002')[0]![5]).toBe('0');
  });

  it('stale 링크는 서버 파생값 그대로 "오래됨" 으로 구분 표시된다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText('gwA')).toBeTruthy());
    fireEvent.click(screen.getAllByRole('button', { name: '디바이스 목록 펼치기' })[0]!);

    const staleRow = deviceRowCells('bbbb0002')[0]!;
    const liveRow = deviceRowCells('aaaa0001')[0]!;
    expect(staleRow[9]).toBe('오래됨');
    expect(liveRow[9]).toBe('수신 중');

    // 흐린 처리(opacity)로도 live 링크와 시각적으로 구분된다.
    const staleTr = screen.getByText('bbbb0002').closest('tr');
    expect(staleTr?.className).toContain('opacity-60');
  });
});

describe('ChirpstackGatewaysTab 빈 상태', () => {
  it('게이트웨이 0건이면 로스터가 업링크 파생이라는 한계를 안내한다(AC-8b)', async () => {
    mockGateways([]);
    renderTab();

    await waitFor(() => expect(screen.getByText('수신된 게이트웨이가 없습니다.')).toBeTruthy());

    const notice = screen.getByText(/업링크에서만 추론/);
    expect(notice.textContent).toContain('침묵 중인 게이트웨이');
    expect(notice.textContent).toContain('커버하는 디바이스가 하나도 없는 게이트웨이');
    expect(notice.textContent).toContain('이름과 위치를 알 수 없어');
  });

  it('gateways 키가 없는 응답도 빈 상태로 안전하게 처리한다', async () => {
    execAgentMock.mockImplementation(() => Promise.resolve({}));
    renderTab();

    await waitFor(() => expect(screen.getByText('수신된 게이트웨이가 없습니다.')).toBeTruthy());
  });

});
