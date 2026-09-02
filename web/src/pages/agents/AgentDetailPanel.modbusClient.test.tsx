// AgentDetailPanel — modbus-client 디바이스 관리 UX 재구성 테스트 (SPEC-MODBUS-009).
//
// 범위:
//   - M3/AC-05: 설정 탭에서 modbus-client 의 devices 편집기(modbus_devices)를 숨긴다.
//     다른 필드는 그대로 렌더되고, 다른 agentType(modbus-gateway)의 설정 폼은 영향이 없다.
//   - M4/AC-06: 장치 탭에서 modbus-client 전용 섹션(ModbusClientDevicesSection)이 렌더되어
//     list_devices 결과 목록을 표시한다.
//   - M4/AC-07: 추가/제거/수정이 useExecAgent 경로로 각각 add_device/remove_device/update_device
//     명령을 발행하고, 편집 폼은 ModbusDevicesEditor 필드(DeviceEditDialog)를 재사용한다.
//
// 데이터/뮤테이션/부수 의존 훅은 스텁으로 격리한다(configTab.test 패턴 준용).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetProvider';
import type { ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';

const LOCAL_TARGET: ResourceTarget = { type: 'local' };

// ── 테스트별 가변 상태 ──
// currentAgent 는 useAgent / useAgentDetailTarget 이 반환할 에이전트다.
let currentAgent: AgentInfo;
// listResponse 는 exec list_devices 가 반환할 응답(백엔드 F1 형상 { data: [...] })이다.
let listResponse: { data: unknown[] };
// modelsFail 이 true 면 list_models 가 오류로 응답한다(fail-open 검증용).
let modelsFail: boolean;

// exec(쓰기) 명령 스파이. add/update/remove_device 는 성공을 즉시 콜백한다(refetch 유발).
const execMutate = vi.hoisted(() =>
  vi.fn(
    (
      _vars: { id: string; req: { command: string; params?: Record<string, unknown> } },
      opts?: { onSuccess?: (res: unknown) => void; onError?: (e: unknown) => void },
    ) => {
      opts?.onSuccess?.({});
    },
  ),
);

// query(읽기) 명령 스파이. list_devices 는 읽기 전용이라 useQueryAgent 경로로 나간다
// (POST /agents/{id}/query — agent.read 만 필요).
//
// 커맨드별로 응답을 나눈다. list_models 가 디바이스 응답을 되돌려 받는 스텁은
// 실제 백엔드와 다르고, 두 조회가 서로를 밀어내는 버그도 감춘다.
const queryMutate = vi.hoisted(() =>
  vi.fn(
    (
      vars: { id: string; req: { command: string; params?: Record<string, unknown> } },
      opts?: { onSuccess?: (res: unknown) => void; onError?: (e: unknown) => void },
    ) => {
      if (vars.req.command === 'list_models') {
        if (modelsFail) opts?.onError?.(new Error('models unavailable'));
        else opts?.onSuccess?.({ data: [], model_count: 0 });
        return;
      }
      opts?.onSuccess?.(listResponse);
    },
  ),
);

// useQueryAgent() 호출마다 **별도 인스턴스**를 돌려준다. 실제 useMutation 도 호출마다
// 독립 observer 를 만들며, 한 인스턴스에 mutate 를 연속 호출하면 앞선 호출의
// per-call 콜백이 사라진다(@tanstack/query-core mutationObserver.ts: mutate() 가
// #mutateOptions 를 덮어쓰고 이전 mutation 에서 removeObserver 한다).
// 인스턴스를 공유하는 스텁은 그 사고를 재현하지 못해 회귀를 통과시킨다.
const queryAgentInstances = vi.hoisted(() => [] as Array<{ mutate: ReturnType<typeof vi.fn> }>);

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: currentAgent, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: undefined, isLoading: false, error: null }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgent: () => ({ data: currentAgent, isLoading: false }),
  useConfigureAgent: () => ({ isPending: false, isError: false, mutateAsync: vi.fn() }),
  useExecAgent: () => ({ isPending: false, mutate: execMutate }),
  useQueryAgent: () => {
    // 인스턴스마다 고유 mutate 스파이를 부여하되, 집계용 queryMutate 로도 함께 기록한다.
    const inst = {
      isPending: false,
      mutate: vi.fn(
        (
          vars: { id: string; req: { command: string; params?: Record<string, unknown> } },
          opts?: { onSuccess?: (res: unknown) => void; onError?: (e: unknown) => void },
        ) => queryMutate(vars, opts),
      ),
    };
    queryAgentInstances.push(inst);
    return inst;
  },
}));

vi.mock('@/hooks/useDevice', () => ({
  useDevicesRealtime: () => ({ data: { data: [] }, isLoading: false }),
  useDeleteDevice: () => ({ isPending: false, mutate: vi.fn(), mutateAsync: vi.fn() }),
  useSetDeviceReport: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  }),
}));

// i18n: 키를 그대로 반환하여 렌더 문자열을 키로 단언한다(스키마 라벨은 원문 유지).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: () => Promise.resolve(),
  resetComponentLogLevel: () => Promise.resolve(),
}));

vi.mock('@/services/api/seriesDataSource', () => ({
  useSeriesDataSource: () => ({ useKeys: () => ({ data: { keys: [] } }) }),
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification: vi.fn() }),
}));

import AgentDetailPanel from './AgentDetailPanel';

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <TargetProvider target={LOCAL_TARGET}>
        <AgentDetailPanel
          agentId={currentAgent.id}
          agentType={currentAgent.type}
          agentName={currentAgent.name}
        />
      </TargetProvider>
    </QueryClientProvider>,
  );
}

const openConfigTab = () =>
  fireEvent.click(screen.getByRole('button', { name: 'agents.detail.tabs.config' }));
const openDevicesTab = () =>
  fireEvent.click(screen.getByRole('button', { name: 'agents.detail.tabs.devices' }));

/** exec(쓰기) 명령별 마지막 호출 인자를 반환한다. */
function lastExecCall(command: string) {
  const calls = execMutate.mock.calls.filter((c) => c[0].req.command === command);
  return calls.length ? calls[calls.length - 1]![0] : undefined;
}

/** query(읽기) 명령별 마지막 호출 인자를 반환한다. */
function lastQueryCall(command: string) {
  const calls = queryMutate.mock.calls.filter((c) => c[0].req.command === command);
  return calls.length ? calls[calls.length - 1]![0] : undefined;
}

beforeEach(() => {
  vi.clearAllMocks();
  queryAgentInstances.length = 0;
  listResponse = { data: [] };
  modelsFail = false;
});

describe('M3 / AC-05 — 설정 탭 modbus-client devices 숨김', () => {
  it('modbus-client 설정 탭은 devices 편집기를 렌더하지 않고 다른 필드는 유지한다', async () => {
    currentAgent = {
      id: 'mc-1',
      name: 'modbus-client-1',
      type: 'modbus-client',
      status: 'running',
      connected: true,
      uptime: '1m',
      config: {
        transport: 'tcp',
        mode: 'interval',
        read_mode: 'cached',
        request_timeout: '3s',
        devices: [{ id: 'd1', host: '10.0.0.1', port: 502, unit_id: 1, register_groups: [] }],
      },
      stats: undefined,
    };
    renderPanel();
    openConfigTab();
    await act(async () => {});

    // devices 필드 라벨과 ModbusDevicesEditor 의 추가 버튼이 렌더되지 않는다.
    expect(screen.queryByText('디바이스 설정')).toBeNull();
    expect(screen.queryByText('property.modbusDevices.addDevice')).toBeNull();
    // devices 외 필드(요청 타임아웃)는 그대로 렌더된다(필드 스코프 무영향).
    expect(screen.getByText('요청 타임아웃')).toBeInTheDocument();
  });

  it('modbus-gateway 설정 탭은 영향을 받지 않고 자체 필드를 렌더한다(타입 스코프)', async () => {
    currentAgent = {
      id: 'gw-1',
      name: 'modbus-gateway-1',
      type: 'modbus-gateway',
      status: 'running',
      connected: true,
      uptime: '1m',
      config: { transport: 'tcp', listen_address: '0.0.0.0', listen_port: 502 },
      stats: undefined,
    };
    renderPanel();
    openConfigTab();
    await act(async () => {});

    // 게이트웨이 자체 필드(수신 주소)가 정상 렌더된다 — 숨김 게이트가 타입 스코프임을 확인.
    expect(screen.getByText('수신 주소')).toBeInTheDocument();
  });
});

describe('M4 / AC-06,07 — 장치 탭 modbus-client 전용 섹션', () => {
  const DEVICE = {
    device_id: 'dev-1',
    id: 'dev-1',
    host: '10.0.0.1',
    port: 502,
    unit_id: 1,
    transport: 'tcp',
    online: true,
    share_session: false,
    register_groups: [{ function_code: 3, start_address: 0, quantity: 10, data_type: 'uint16' }],
  };

  beforeEach(() => {
    currentAgent = {
      id: 'mc-1',
      name: 'modbus-client-1',
      type: 'modbus-client',
      status: 'running',
      connected: true,
      uptime: '1m',
      config: { transport: 'tcp' },
      stats: undefined,
    };
  });

  it('AC-06: 전용 섹션이 렌더되고 list_devices 결과 목록을 표시한다', async () => {
    listResponse = { data: [DEVICE] };
    renderPanel();
    openDevicesTab();
    await act(async () => {});

    // 읽기 전용 목록 조회는 exec 가 아니라 query 로 나가야 한다.
    expect(lastQueryCall('list_devices')).toBeDefined();
    expect(lastExecCall('list_devices')).toBeUndefined();
    expect(screen.getByText('agents.detail.devices.modbusSectionTitle')).toBeInTheDocument();
    expect(screen.getByText('dev-1')).toBeInTheDocument();
  });

  it('AC-07 add: 추가 폼(ModbusDevicesEditor 필드)에서 저장 시 add_device 를 발행한다', async () => {
    listResponse = { data: [] };
    renderPanel();
    openDevicesTab();
    await act(async () => {});

    fireEvent.click(screen.getByTestId('modbus-client-add-device'));
    // DeviceEditDialog 의 id/host 필드를 채운다(add 는 id 필수 + TCP host 필수).
    fireEvent.change(screen.getByPlaceholderText('device-1'), { target: { value: 'dev-new' } });
    fireEvent.change(screen.getByPlaceholderText('192.168.1.10'), { target: { value: '10.0.0.9' } });
    fireEvent.click(screen.getByRole('button', { name: 'property.modbusDevices.save' }));

    const call = lastExecCall('add_device');
    expect(call).toBeDefined();
    const params = call!.req.params as Record<string, unknown>;
    expect(params.id).toBe('dev-new');
    expect(params.host).toBe('10.0.0.9');
    expect(params.unit_id).toBe(1);
  });

  it('AC-07 update: 수정 폼에서 저장 시 update_device 를 device_id/register_groups 로 발행한다', async () => {
    listResponse = { data: [DEVICE] };
    renderPanel();
    openDevicesTab();
    await act(async () => {});

    fireEvent.click(screen.getByRole('button', { name: 'agents.detail.devices.modbusEditTooltip' }));
    fireEvent.click(screen.getByRole('button', { name: 'property.modbusDevices.save' }));

    const call = lastExecCall('update_device');
    expect(call).toBeDefined();
    const params = call!.req.params as Record<string, unknown>;
    expect(params.device_id).toBe('dev-1');
    expect(params.unit_id).toBe(1);
    expect(Array.isArray(params.register_groups)).toBe(true);
    expect((params.register_groups as unknown[]).length).toBe(1);
  });

  it('AC-07 remove: 제거 확인 후 remove_device 를 device_id 로 발행한다', async () => {
    listResponse = { data: [DEVICE] };
    renderPanel();
    openDevicesTab();
    await act(async () => {});

    fireEvent.click(
      screen.getByRole('button', { name: 'agents.detail.devices.modbusRemoveTooltip' }),
    );
    // ConfirmDialog 의 confirm 버튼(common.delete).
    fireEvent.click(screen.getByRole('button', { name: 'common.delete' }));

    const call = lastExecCall('remove_device');
    expect(call).toBeDefined();
    expect((call!.req.params as Record<string, unknown>).device_id).toBe('dev-1');
  });
});


// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 회귀 — 모델 카탈로그 조회가 디바이스 목록 조회를 밀어내지 않는다
// ---------------------------------------------------------------------------
//
// 사고: 두 조회가 하나의 useQueryAgent() 인스턴스를 공유하면, 두 번째 mutate 가
// MutationObserver 의 #mutateOptions 를 덮어쓰고 이전 mutation 에서 removeObserver 한다.
// 그러면 먼저 보낸 list_devices 의 per-call onSuccess 가 실행되지 않아 isLoading 이
// true 로 굳고, 장치 탭은 animate-pulse 스켈레톤만 반복 렌더한다(화면이 "깜박"인다).

describe('SPEC-MODBUS-013 — 모델 카탈로그 조회 격리', () => {
  const CLIENT_AGENT: AgentInfo = {
    id: 'mc-1',
    name: 'modbus-client-1',
    type: 'modbus-client',
    status: 'running',
    connected: true,
    uptime: '1m',
    config: { transport: 'tcp' },
    stats: undefined,
  };

  beforeEach(() => {
    currentAgent = { ...CLIENT_AGENT };
  });

  it('list_devices 와 list_models 를 서로 다른 mutation 인스턴스로 보낸다', () => {
    renderPanel();
    openDevicesTab();

    const owner = (command: string) =>
      queryAgentInstances.findIndex((inst) =>
        inst.mutate.mock.calls.some((c) => c[0]?.req?.command === command),
      );

    expect(owner('list_devices'), 'list_devices 가 발행되어야 한다').toBeGreaterThanOrEqual(0);
    expect(owner('list_models'), 'list_models 가 발행되어야 한다').toBeGreaterThanOrEqual(0);
    expect(
      owner('list_devices'),
      '두 조회가 같은 mutation 인스턴스를 쓰면 앞선 조회의 per-call 콜백이 유실된다',
    ).not.toBe(owner('list_models'));
  });

  it('모델 조회가 함께 나가도 디바이스 목록이 렌더된다(스켈레톤에 갇히지 않음)', () => {
    listResponse = {
      data: [{ device_id: 'plc-1', id: 'plc-1', host: '10.0.0.1', port: 502, unit_id: 1 }],
    };
    renderPanel();
    openDevicesTab();

    expect(screen.getByText('plc-1')).toBeTruthy();
  });

  it('모델 조회가 실패해도 디바이스 목록은 정상 렌더된다(fail-open)', () => {
    modelsFail = true;
    listResponse = {
      data: [{ device_id: 'plc-2', id: 'plc-2', host: '10.0.0.2', port: 502, unit_id: 2 }],
    };
    renderPanel();
    openDevicesTab();

    expect(screen.getByText('plc-2')).toBeTruthy();
  });
});

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 M7 — 편집 폼에서 host/port 변경
// ---------------------------------------------------------------------------

describe('SPEC-MODBUS-013 — 디바이스 편집 host/port 변경', () => {
  const CLIENT_AGENT: AgentInfo = {
    id: 'mc-1',
    name: 'modbus-client-1',
    type: 'modbus-client',
    status: 'running',
    connected: true,
    uptime: '1m',
    config: { transport: 'tcp' },
    stats: undefined,
  };

  beforeEach(() => {
    currentAgent = { ...CLIENT_AGENT };
    listResponse = {
      data: [
        {
          device_id: 'plc-1',
          id: 'plc-1',
          host: '10.0.0.1',
          port: 502,
          unit_id: 1,
          register_groups: [
            { name: 'g', function_code: 3, start_address: 0, quantity: 2, enabled: true },
          ],
        },
      ],
    };
  });

  /** 목록의 편집 버튼을 눌러 수정 다이얼로그를 연다. i18n 목은 키를 그대로 돌려준다. */
  async function openEdit() {
    renderPanel();
    openDevicesTab();
    await act(async () => {});
    fireEvent.click(
      screen.getByRole('button', { name: 'agents.detail.devices.modbusEditTooltip' }),
    );
  }

  it('편집 폼에 host/port 입력이 노출된다', async () => {
    await openEdit();
    expect(screen.getByDisplayValue('10.0.0.1')).toBeTruthy();
    expect(screen.getByDisplayValue('502')).toBeTruthy();
  });

  it('host/port 를 바꿔 저장하면 update_device params 에 실려 나간다', async () => {
    await openEdit();

    fireEvent.change(screen.getByDisplayValue('10.0.0.1'), {
      target: { value: '192.168.0.50' },
    });
    fireEvent.change(screen.getByDisplayValue('502'), { target: { value: '5020' } });
    fireEvent.click(screen.getByRole('button', { name: 'property.modbusDevices.save' }));

    const call = lastExecCall('update_device');
    expect(call, 'update_device 가 발행되어야 한다').toBeTruthy();
    expect(call!.req.params).toMatchObject({
      device_id: 'plc-1',
      host: '192.168.0.50',
      port: 5020,
    });
  });
});
