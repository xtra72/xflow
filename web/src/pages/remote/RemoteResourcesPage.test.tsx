// RemoteResourcesPage 테스트 (SPEC-REMOTE-001 M5, G03 + G04).
//
// 범위:
//   - G03: 통합 뷰에서 출처 노드 태그 표시 + 오프라인 last-known 표식
//   - G03: 노드별 뷰 전환 + 노드 선택
//   - G04: 명령 버튼 → sendCommand mutation + 성공/실패 toast 피드백
//   - G04: 오프라인 자원의 명령 버튼 비활성화

import { render, screen, fireEvent, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';
import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, MirroredResource } from '@/types/remote';

// ---- useRemote mock ----
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());
const useAllMirrorMock = vi.hoisted(() => vi.fn());
const useNodeMirrorMock = vi.hoisted(() => vi.fn());
const sendCommandMutateMock = vi.hoisted(() => vi.fn());
const createAgentMutateAsyncMock = vi.hoisted(() => vi.fn());
const updateAgentMutateAsyncMock = vi.hoisted(() => vi.fn());
const deleteFlowMutateMock = vi.hoisted(() => vi.fn());
const deleteAgentMutateMock = vi.hoisted(() => vi.fn());

// ---- navigate mock (react-router) ----
const navigateMock = vi.hoisted(() => vi.fn());
vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>();
  return { ...actual, useNavigate: () => navigateMock };
});

vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: useManagedNodesMock,
  useRemoteMode: useRemoteModeMock,
  useAllMirror: useAllMirrorMock,
  useNodeMirror: useNodeMirrorMock,
  useSendCommand: () => ({
    mutate: sendCommandMutateMock,
    isPending: false,
    variables: undefined,
  }),
  useCreateRemoteAgent: () => ({
    mutateAsync: createAgentMutateAsyncMock,
    isPending: false,
  }),
  useUpdateRemoteAgent: () => ({
    mutateAsync: updateAgentMutateAsyncMock,
    isPending: false,
  }),
  useDeleteRemoteFlow: () => ({ mutate: deleteFlowMutateMock, isPending: false }),
  useDeleteRemoteAgent: () => ({ mutate: deleteAgentMutateMock, isPending: false }),
}));

// ---- uiStore mock (toast 캡처) ----
const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import RemoteResourcesPage from './RemoteResourcesPage';

function renderPage() {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <RemoteResourcesPage />
      </I18nProvider>
    </MemoryRouter>,
  );
}

function makeNode(overrides: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-1',
    hostname: 'host-1',
    version: 'v1',
    status: 'approved',
    online: true,
    last_seen: Date.now(),
    ...overrides,
  };
}

function makeResource(overrides: Partial<MirroredResource> = {}): MirroredResource {
  return {
    id: 'res-1',
    source_instance_id: 'node-1',
    name: 'Resource 1',
    kind: 'flow',
    status: 'running',
    updated_at: Date.now(),
    online: true,
    ...overrides,
  };
}

beforeEach(() => {
  useManagedNodesMock.mockReset();
  useRemoteModeMock.mockReset();
  useAllMirrorMock.mockReset();
  useNodeMirrorMock.mockReset();
  sendCommandMutateMock.mockReset();
  createAgentMutateAsyncMock.mockReset();
  updateAgentMutateAsyncMock.mockReset();
  deleteFlowMutateMock.mockReset();
  deleteAgentMutateMock.mockReset();
  navigateMock.mockReset();
  addNotificationMock.mockReset();

  // 기본: server 모드 (M5 동작과 동일).
  useRemoteModeMock.mockReturnValue({ data: { mode: 'server' } });
  useManagedNodesMock.mockReturnValue({ data: [makeNode()] });
  useAllMirrorMock.mockReturnValue({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
  useNodeMirrorMock.mockReturnValue({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
});

// 모드 게이팅 — server 모드가 아니면 안내 표시 + 미러 쿼리 비활성
describe('RemoteResourcesPage — 모드 게이팅', () => {
  it.each(['disabled', 'client'] as const)(
    'mode=%s 이면 안내를 표시하고 미러 쿼리를 비활성화한다',
    (mode) => {
      useRemoteModeMock.mockReturnValue({ data: { mode } });
      renderPage();

      // 안내 패널이 표시된다.
      expect(screen.getByTestId('remote-not-server')).toBeInTheDocument();
      // 자원 테이블/탭/빈 상태는 렌더되지 않는다.
      expect(screen.queryByTestId('remote-resources-empty')).not.toBeInTheDocument();
      expect(screen.queryByTestId('remote-mode-aggregated')).not.toBeInTheDocument();
      // 미러/노드 쿼리는 enabled=false 로 호출된다 (발행되지 않음).
      expect(useAllMirrorMock.mock.calls.at(-1)![1]).toBe(false);
      expect(useManagedNodesMock.mock.calls.at(-1)![1]).toBe(false);
    },
  );
});

// G03 — 통합 뷰 + 출처 태그
describe('RemoteResourcesPage — G03 통합 뷰', () => {
  it('통합 뷰에서 각 행에 출처 노드 태그를 표시한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [
        makeResource({ id: 'r-1', source_instance_id: 'node-a', online: true }),
        makeResource({ id: 'r-2', source_instance_id: 'node-b', online: false }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    renderPage();

    const tags = screen.getAllByTestId('source-node-tag');
    expect(tags).toHaveLength(2);
    expect(tags[0]).toHaveAttribute('data-source-instance-id', 'node-a');
    expect(tags[1]).toHaveAttribute('data-source-instance-id', 'node-b');
  });

  it('오프라인 출처 노드는 last-known 표식을 표시한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ source_instance_id: 'node-off', online: false })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    renderPage();

    const tag = screen.getByTestId('source-node-tag');
    expect(tag).toHaveAttribute('data-online', 'false');
    expect(within(tag).getByText(/마지막 정보/)).toBeInTheDocument();
  });

  it('자원이 없으면 빈 상태를 표시한다', () => {
    renderPage();
    expect(screen.getByTestId('remote-resources-empty')).toBeInTheDocument();
  });
});

// G03 — 노드별 뷰
describe('RemoteResourcesPage — G03 노드별 뷰', () => {
  it('노드별 모드로 전환하면 노드 선택 + 출처 태그 미표시', () => {
    useNodeMirrorMock.mockReturnValue({
      data: [makeResource()],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-mode-per-node'));

    // 노드 선택 셀렉트 노출.
    expect(screen.getByTestId('remote-node-select')).toBeInTheDocument();
    // 노드별 뷰에서는 출처 태그를 표시하지 않는다.
    expect(screen.queryByTestId('source-node-tag')).not.toBeInTheDocument();
    // 자원 행은 표시된다.
    expect(screen.getByTestId('mirror-resource-row')).toBeInTheDocument();
  });
});

// G04 — 명령 발행 피드백
describe('RemoteResourcesPage — G04 명령', () => {
  it('명령 버튼 클릭 시 sendCommand 를 올바른 인자로 호출한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-command-start'));
    expect(sendCommandMutateMock).toHaveBeenCalledTimes(1);
    const [vars] = sendCommandMutateMock.mock.calls[0]!;
    expect(vars).toEqual({
      instanceID: 'node-a',
      req: { domain: 'flow', action: 'start', args: { id: 'f-1' } },
    });
  });

  it('명령 성공 시 success toast 를 발송한다', () => {
    sendCommandMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-command-start'));
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('명령 실패(503) 시 error toast 를 발송한다', () => {
    sendCommandMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onError?: (e: unknown) => void }) =>
        opts?.onError?.(new APIError('SERVICE_UNAVAILABLE', 'offline', 503)),
    );
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-command-start'));
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });

  it('오프라인 자원의 명령 버튼은 비활성화된다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ kind: 'flow', online: false })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('remote-command-start')).toBeDisabled();
  });
});

// M7 (그룹 I) — 편집/생성/삭제 액션 + 게이팅
describe('RemoteResourcesPage — M7 편집 게이팅', () => {
  it('승인+온라인 노드의 노출 flow 는 편집/삭제 버튼이 활성화된다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('mirror-resource-edit')).toBeEnabled();
    expect(screen.getByTestId('mirror-resource-delete')).toBeEnabled();
  });

  it('오프라인 노드의 자원은 편집/삭제 버튼이 비활성화된다 (게이팅)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: false })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: false })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('mirror-resource-edit')).toBeDisabled();
    expect(screen.getByTestId('mirror-resource-delete')).toBeDisabled();
  });

  it('미승인(pending) 노드의 자원은 편집/삭제가 비활성화된다 (게이팅)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'pending', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('mirror-resource-edit')).toBeDisabled();
    expect(screen.getByTestId('mirror-resource-delete')).toBeDisabled();
  });

  it('device 자원에는 편집/삭제 버튼이 없다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'd-1', source_instance_id: 'node-a', kind: 'device', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    // device 탭으로 전환.
    fireEvent.click(screen.getByTestId('remote-kind-device'));

    expect(screen.queryByTestId('mirror-resource-edit')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mirror-resource-delete')).not.toBeInTheDocument();
  });
});

describe('RemoteResourcesPage — M7 flow 편집/생성 라우팅', () => {
  it('flow 편집 버튼은 시각 편집기 라우트로 이동한다 (REQ-I08)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('mirror-resource-edit'));
    expect(navigateMock).toHaveBeenCalledWith(
      '/admin/remote/nodes/node-a/flows/f-1/edit',
    );
  });

  it('per-node 뷰의 flow 생성 버튼은 신규 편집기 라우트로 이동한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useNodeMirrorMock.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-mode-per-node'));
    const createBtn = screen.getByTestId('remote-resource-create');
    expect(createBtn).toBeEnabled();
    fireEvent.click(createBtn);
    expect(navigateMock).toHaveBeenCalledWith('/admin/remote/nodes/node-a/flows/new');
  });

  it('오프라인 대상 노드에서는 생성 버튼이 비활성화된다 (게이팅)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: false })],
    });
    useNodeMirrorMock.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('remote-mode-per-node'));
    expect(screen.getByTestId('remote-resource-create')).toBeDisabled();
  });
});

describe('RemoteResourcesPage — M7 agent 편집/삭제', () => {
  it('agent 편집 버튼은 에이전트 설정 다이얼로그를 연다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [
        makeResource({
          id: 'a-1',
          source_instance_id: 'node-a',
          kind: 'agent',
          online: true,
          definition: JSON.stringify({ type: 'mqtt-client', broker: 'tcp://h:1883' }),
        }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    fireEvent.click(screen.getByTestId('remote-kind-agent'));

    fireEvent.click(screen.getByTestId('mirror-resource-edit'));
    expect(screen.getByTestId('remote-agent-edit-dialog')).toBeInTheDocument();
  });

  it('agent 설정 저장은 시크릿을 생략하고 updateAgent 를 호출한다 (REQ-I07)', async () => {
    updateAgentMutateAsyncMock.mockResolvedValue({ id: 'a-1', name: 'a', status: '' });
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [
        makeResource({
          id: 'a-1',
          source_instance_id: 'node-a',
          kind: 'agent',
          online: true,
          // redacted 정의 — password 키가 부재한다.
          definition: JSON.stringify({ type: 'mqtt-client', broker: 'tcp://h:1883' }),
        }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    fireEvent.click(screen.getByTestId('remote-kind-agent'));
    fireEvent.click(screen.getByTestId('mirror-resource-edit'));

    // config 에 빈 password 를 추가해도 페이로드에서 생략되어야 한다.
    fireEvent.change(screen.getByTestId('remote-agent-config'), {
      target: {
        value: JSON.stringify({ type: 'mqtt-client', broker: 'tcp://h:1883', password: '' }),
      },
    });
    fireEvent.click(screen.getByTestId('remote-agent-edit-submit'));

    await vi.waitFor(() => {
      expect(updateAgentMutateAsyncMock).toHaveBeenCalledTimes(1);
    });
    const [vars] = updateAgentMutateAsyncMock.mock.calls[0]!;
    expect(vars.instanceID).toBe('node-a');
    expect(vars.agentID).toBe('a-1');
    // 빈 password 는 생략되어야 한다(필드 부재).
    expect('password' in vars.req.config).toBe(false);
    expect(vars.req.config).toEqual({ type: 'mqtt-client', broker: 'tcp://h:1883' });
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('flow 삭제는 확인 후 deleteFlow 를 호출하고 성공 토스트를 띄운다', () => {
    deleteFlowMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('mirror-resource-delete'));
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));

    expect(deleteFlowMutateMock).toHaveBeenCalledTimes(1);
    expect(deleteFlowMutateMock.mock.calls[0]![0]).toEqual({
      instanceID: 'node-a',
      flowID: 'f-1',
    });
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('삭제 실패(502) 시 노드 오류 사유를 포함한 error 토스트를 띄운다', () => {
    deleteAgentMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onError?: (e: unknown) => void }) =>
        opts?.onError?.(new APIError('BAD_GATEWAY', '검증 실패', 502)),
    );
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'node-a', status: 'approved', online: true })],
    });
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'a-1', source_instance_id: 'node-a', kind: 'agent', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    fireEvent.click(screen.getByTestId('remote-kind-agent'));

    fireEvent.click(screen.getByTestId('mirror-resource-delete'));
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));

    expect(deleteAgentMutateMock).toHaveBeenCalledTimes(1);
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });
});
