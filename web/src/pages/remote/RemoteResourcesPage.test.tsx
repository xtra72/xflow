// RemoteResourcesPage 테스트 (SPEC-REMOTE-001 M5, G03 + G04).
//
// 범위:
//   - G03: 통합 뷰에서 출처 노드 태그 표시 + 오프라인 last-known 표식
//   - G03: 노드별 뷰 전환 + 노드 선택
//   - G04: 명령 버튼 → sendCommand mutation + 성공/실패 toast 피드백
//   - G04: 오프라인 자원의 명령 버튼 비활성화

import { render, screen, fireEvent, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';
import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, MirroredResource } from '@/types/remote';

// ---- useRemote mock ----
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useAllMirrorMock = vi.hoisted(() => vi.fn());
const useNodeMirrorMock = vi.hoisted(() => vi.fn());
const sendCommandMutateMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: useManagedNodesMock,
  useAllMirror: useAllMirrorMock,
  useNodeMirror: useNodeMirrorMock,
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
    <I18nProvider>
      <RemoteResourcesPage />
    </I18nProvider>,
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
  useAllMirrorMock.mockReset();
  useNodeMirrorMock.mockReset();
  sendCommandMutateMock.mockReset();
  addNotificationMock.mockReset();

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
