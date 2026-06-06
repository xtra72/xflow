// RemoteResourcesPage 테스트 (SPEC-REMOTE-001 M5/M8, G03 + G04 + REQ-J14).
//
// 범위(통합 개요 전용으로 축소 — REQ-J14):
//   - 모드 게이팅: server 모드가 아니면 안내 표시 + 미러 쿼리 비활성
//   - G03: 통합 뷰에서 출처 노드 태그 표시 + 오프라인 last-known 표식
//   - G04: 명령 버튼 → sendCommand mutation + 성공/실패 toast 피드백
//   - G04: 오프라인 자원의 명령 버튼 비활성화
//   - REQ-J14: "이 노드에서 제어" 링크 → 통합 로컬 페이지로 네비게이트
//
// 본 페이지에서 제거된 동작(→ 원격 노드 제어/통합 로컬 페이지로 이전):
//   - 통합/노드별 뷰 토글, 노드 선택 드롭다운
//   - 자원 생성·수정·삭제, 에이전트 편집 다이얼로그

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
const sendCommandMutateMock = vi.hoisted(() => vi.fn());

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
  useSendCommand: () => ({
    mutate: sendCommandMutateMock,
    isPending: false,
    variables: undefined,
  }),
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
  sendCommandMutateMock.mockReset();
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
      expect(screen.queryByTestId('remote-kind-flow')).not.toBeInTheDocument();
      // 미러/노드 쿼리는 enabled=false 로 호출된다 (발행되지 않음).
      expect(useAllMirrorMock.mock.calls.at(-1)![1]).toBe(false);
      expect(useManagedNodesMock.mock.calls.at(-1)![1]).toBe(false);
    },
  );
});

// REQ-J14 — 통합 개요 전용: 뷰 토글/노드 셀렉터/편집 액션이 없다.
describe('RemoteResourcesPage — REQ-J14 통합 개요 전용', () => {
  it('뷰 모드 토글과 노드 선택 드롭다운을 렌더하지 않는다', () => {
    renderPage();
    expect(screen.queryByTestId('remote-mode-aggregated')).not.toBeInTheDocument();
    expect(screen.queryByTestId('remote-mode-per-node')).not.toBeInTheDocument();
    expect(screen.queryByTestId('remote-node-select')).not.toBeInTheDocument();
  });

  it('생성/수정/삭제 진입점을 렌더하지 않는다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    expect(screen.queryByTestId('remote-resource-create')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mirror-resource-edit')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mirror-resource-delete')).not.toBeInTheDocument();
    expect(screen.queryByTestId('remote-agent-edit-dialog')).not.toBeInTheDocument();
  });
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

// REQ-J14 — "이 노드에서 제어" 링크
describe('RemoteResourcesPage — REQ-J14 이 노드에서 제어', () => {
  it('flow 행의 제어 버튼은 통합 flow 페이지로 target 쿼리와 함께 이동한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'f-1', source_instance_id: 'node-a', kind: 'flow', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('mirror-resource-control'));
    expect(navigateMock).toHaveBeenCalledWith('/flows?target=remote:node-a');
  });

  it('agent 탭의 제어 버튼은 통합 agents 페이지로 이동한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'a-1', source_instance_id: 'node-b', kind: 'agent', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    fireEvent.click(screen.getByTestId('remote-kind-agent'));

    fireEvent.click(screen.getByTestId('mirror-resource-control'));
    expect(navigateMock).toHaveBeenCalledWith('/agents?target=remote:node-b');
  });

  it('device 탭의 제어 버튼은 통합 devices 페이지로 이동한다', () => {
    useAllMirrorMock.mockReturnValue({
      data: [makeResource({ id: 'd-1', source_instance_id: 'node-c', kind: 'device', online: true })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    fireEvent.click(screen.getByTestId('remote-kind-device'));

    fireEvent.click(screen.getByTestId('mirror-resource-control'));
    expect(navigateMock).toHaveBeenCalledWith('/devices?target=remote:node-c');
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
