// SPEC-WEB-006 v0.1.0 (M5, M6, M7, M8, M9) — UpdateDialog 통합 테스트.
// SPEC-UPDATE-002 v0.1.0 (M9, M10, M14) — confirm step 의 target dropdown +
// auto_restart 체크박스 + 11-state machine 검증.
//
// 5-step UX (info → confirm → apply → progress → result) 와 11-state machine
// 시각화, terminal 상태별 result variant, Rollback/Retry 동작을 모두 검증한다.
//
// 훅(useUpdateApply / useUpdateStatus / useUpdateRollback) 은 vi.mock 으로 격리.
//
// @spec SPEC-WEB-006 v0.1.0 (M5, M6, M7, M8, M9)
// @spec SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ─────────────────────────────────────────────────────────────────────
// systemUpdate hooks mocks (Phase A 의 훅 시그니처를 그대로 사용)
// ─────────────────────────────────────────────────────────────────────

const useUpdateApplyMock = vi.hoisted(() => vi.fn());
const useUpdateStatusMock = vi.hoisted(() => vi.fn());
const useUpdateRollbackMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/systemUpdate', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useUpdateApply: useUpdateApplyMock,
    useUpdateStatus: useUpdateStatusMock,
    useUpdateRollback: useUpdateRollbackMock,
  };
});

import type {
  ApplyRequest,
  ApplyResponse,
  OperationStatus,
  UpdateOperation,
  VersionInfo,
} from '@/services/api/systemUpdate';

import { UpdateDialog } from './UpdateDialog';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v0.3.0',
    commit: 'abc1234',
    build_date: '2026-04-30T12:00:00Z',
    go_version: 'go1.25.0',
    channel: 'stable',
    update_available: true,
    latest_version: 'v0.4.0',
    // SPEC-WEB-007 추가 필드.
    os: 'linux',
    arch: 'amd64',
    hostname: 'xflow-node-01',
    mode: 'server',
    uptime_seconds: 3600,
    ...overrides,
  };
}

interface ApplyMutationState {
  mutate?: ReturnType<typeof vi.fn>;
  isPending?: boolean;
  isError?: boolean;
  error?: Error | null;
  data?: ApplyResponse | null;
  reset?: () => void;
}

function setApplyMutation(state: ApplyMutationState = {}) {
  useUpdateApplyMock.mockReturnValue({
    mutate: state.mutate ?? vi.fn(),
    isPending: state.isPending ?? false,
    isError: state.isError ?? false,
    error: state.error ?? null,
    data: state.data ?? null,
    reset: state.reset ?? vi.fn(),
  });
}

interface StatusQueryState {
  data?: UpdateOperation;
  isLoading?: boolean;
  isError?: boolean;
}

function setStatusQuery(state: StatusQueryState = {}) {
  useUpdateStatusMock.mockReturnValue({
    data: state.data,
    isLoading: state.isLoading ?? false,
    isError: state.isError ?? false,
  });
}

interface RollbackMutationState {
  mutate?: ReturnType<typeof vi.fn>;
  isPending?: boolean;
}

function setRollbackMutation(state: RollbackMutationState = {}) {
  useUpdateRollbackMock.mockReturnValue({
    mutate: state.mutate ?? vi.fn(),
    isPending: state.isPending ?? false,
  });
}

function makeOperation(
  status: OperationStatus,
  overrides: Partial<UpdateOperation> = {},
): UpdateOperation {
  return {
    operation_id: 'op-test-1',
    status,
    from_version: 'v0.3.0',
    to_version: 'v0.4.0',
    started_at: '2026-05-01T12:00:00Z',
    completed_at: status === 'completed' ? '2026-05-01T12:05:00Z' : null,
    error: null,
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────
// Default state setup
// ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  useUpdateApplyMock.mockReset();
  useUpdateStatusMock.mockReset();
  useUpdateRollbackMock.mockReset();
  setApplyMutation();
  setStatusQuery();
  setRollbackMutation();
});

afterEach(() => {
  vi.useRealTimers();
});

// ─────────────────────────────────────────────────────────────────────
// 1. Render guard
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — 렌더 가드', () => {
  it('open=false → 다이얼로그가 렌더되지 않는다', () => {
    render(
      <UpdateDialog
        open={false}
        onClose={vi.fn()}
        version={makeVersion()}
      />,
    );
    expect(screen.queryByTestId('update-dialog')).not.toBeInTheDocument();
  });

  it('open=true + version → info 단계로 시작한다', () => {
    render(
      <UpdateDialog
        open={true}
        onClose={vi.fn()}
        version={makeVersion()}
      />,
    );
    expect(screen.getByTestId('update-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('update-dialog-step-info')).toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 2. Step 1: info
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — info step', () => {
  it('현재 버전 → 최신 버전 정보가 표시된다', () => {
    render(
      <UpdateDialog
        open={true}
        onClose={vi.fn()}
        version={makeVersion({ version: 'v0.3.0', latest_version: 'v0.4.0' })}
      />,
    );
    const stepInfo = screen.getByTestId('update-dialog-step-info');
    expect(stepInfo).toHaveTextContent('v0.3.0');
    expect(stepInfo).toHaveTextContent('v0.4.0');
  });

  it('Cancel 버튼 클릭 시 onClose 가 호출된다', () => {
    const onClose = vi.fn();
    render(
      <UpdateDialog
        open={true}
        onClose={onClose}
        version={makeVersion()}
      />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Next 버튼 클릭 시 confirm step 으로 이동한다', () => {
    render(
      <UpdateDialog
        open={true}
        onClose={vi.fn()}
        version={makeVersion()}
      />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    expect(
      screen.getByTestId('update-dialog-step-confirm'),
    ).toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 3. Step 2: confirm
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — confirm step', () => {
  function gotoConfirm(version: VersionInfo = makeVersion()) {
    const onClose = vi.fn();
    render(<UpdateDialog open={true} onClose={onClose} version={version} />);
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    return { onClose };
  }

  it('재시작 경고가 표시된다', () => {
    gotoConfirm();
    expect(
      screen.getByTestId('update-dialog-step-confirm'),
    ).toHaveTextContent(/재시작/);
  });

  it('일반 업그레이드는 force 체크박스가 보이지 않는다', () => {
    gotoConfirm(
      makeVersion({ version: 'v0.3.0', latest_version: 'v0.4.0' }),
    );
    expect(
      screen.queryByTestId('update-dialog-force-checkbox'),
    ).not.toBeInTheDocument();
  });

  it('다운그레이드 (latest < current) 시 force 체크박스가 표시된다', () => {
    gotoConfirm(
      makeVersion({ version: 'v0.4.0', latest_version: 'v0.3.0' }),
    );
    expect(
      screen.getByTestId('update-dialog-force-checkbox'),
    ).toBeInTheDocument();
  });

  it('Back 버튼 클릭 시 info step 으로 돌아간다', () => {
    gotoConfirm();
    fireEvent.click(screen.getByTestId('update-dialog-back'));
    expect(screen.getByTestId('update-dialog-step-info')).toBeInTheDocument();
  });

  it('"업데이트 적용" 클릭 시 useUpdateApply.mutate 가 호출된다', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm();
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    expect(mutateMock).toHaveBeenCalledTimes(1);
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg).toMatchObject({ version: 'v0.4.0' });
  });

  it('다운그레이드 + force 체크 → mutate 인자에 force=true 가 포함된다', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm(makeVersion({ version: 'v0.4.0', latest_version: 'v0.3.0' }));
    fireEvent.click(screen.getByTestId('update-dialog-force-checkbox'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    expect(mutateMock).toHaveBeenCalledTimes(1);
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.force).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 3.5 confirm step — SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)
// auto_restart 체크박스 + target dropdown (admin only)
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — confirm step (SPEC-UPDATE-002 v0.1.0)', () => {
  function gotoConfirm(opts: {
    isAdmin?: boolean;
    version?: VersionInfo;
  } = {}) {
    const onClose = vi.fn();
    render(
      <UpdateDialog
        open={true}
        onClose={onClose}
        version={opts.version ?? makeVersion()}
        isAdmin={opts.isAdmin ?? false}
      />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    return { onClose };
  }

  // M-1: auto_restart checkbox
  it('confirm step 에 auto_restart 체크박스가 노출된다', () => {
    gotoConfirm();
    expect(
      screen.getByTestId('update-dialog-auto-restart-checkbox'),
    ).toBeInTheDocument();
  });

  it('auto_restart 체크박스 default 는 unchecked (v0.1.0 호환)', () => {
    gotoConfirm();
    const checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('auto_restart 미체크 → mutate 인자에 auto_restart 누락 (v0.1.0 default 흐름)', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm();
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.auto_restart).toBeUndefined();
  });

  it('auto_restart 체크 → mutate 인자에 auto_restart=true 포함 (M-1)', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm();
    fireEvent.click(screen.getByTestId('update-dialog-auto-restart-checkbox'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.auto_restart).toBe(true);
  });

  // M9: target dropdown (admin only)
  it('비-admin (isAdmin=false) → target dropdown 미노출', () => {
    gotoConfirm({ isAdmin: false });
    expect(
      screen.queryByTestId('update-dialog-target-select'),
    ).not.toBeInTheDocument();
  });

  it('admin (isAdmin=true) → target dropdown 노출', () => {
    gotoConfirm({ isAdmin: true });
    expect(
      screen.getByTestId('update-dialog-target-select'),
    ).toBeInTheDocument();
  });

  it('target dropdown 의 default 값은 xflowd', () => {
    gotoConfirm({ isAdmin: true });
    const select = screen.getByTestId(
      'update-dialog-target-select',
    ) as HTMLSelectElement;
    expect(select.value).toBe('xflowd');
  });

  it('target dropdown 에 3개 옵션 (xflowd, xflow-agent, xflow) 이 있다', () => {
    gotoConfirm({ isAdmin: true });
    const select = screen.getByTestId(
      'update-dialog-target-select',
    ) as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).toEqual(['xflowd', 'xflow-agent', 'xflow']);
  });

  it('target=xflow-agent 선택 → mutate 인자에 target=xflow-agent 포함 (Scenario 10)', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm({ isAdmin: true });
    fireEvent.change(screen.getByTestId('update-dialog-target-select'), {
      target: { value: 'xflow-agent' },
    });
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.target).toBe('xflow-agent');
  });

  it('target=xflowd (default) 일 때는 mutate 인자에 target 미포함 (백엔드 default 활용)', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm({ isAdmin: true });
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.target).toBeUndefined();
  });

  // Scenario 11 + Cross-cutting edge: target=xflow + auto_restart 비활성화
  it('target=xflow 선택 시 auto_restart 체크박스가 disabled 된다 (Scenario 11)', () => {
    gotoConfirm({ isAdmin: true });
    fireEvent.change(screen.getByTestId('update-dialog-target-select'), {
      target: { value: 'xflow' },
    });
    const checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.disabled).toBe(true);
  });

  it('target=xflow 로 변경되면 이전에 체크된 auto_restart 가 자동 해제된다 (Scenario 11)', () => {
    gotoConfirm({ isAdmin: true });
    // 먼저 auto_restart 체크.
    fireEvent.click(screen.getByTestId('update-dialog-auto-restart-checkbox'));
    let checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.checked).toBe(true);

    // target=xflow 로 변경.
    fireEvent.change(screen.getByTestId('update-dialog-target-select'), {
      target: { value: 'xflow' },
    });
    checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
    expect(checkbox.disabled).toBe(true);
  });

  it('target=xflow + auto_restart 체크 시도 → mutate 인자에 auto_restart 미포함', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock });
    gotoConfirm({ isAdmin: true });
    fireEvent.change(screen.getByTestId('update-dialog-target-select'), {
      target: { value: 'xflow' },
    });
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    const arg = mutateMock.mock.calls[0]![0] as ApplyRequest;
    expect(arg.auto_restart).toBeUndefined();
    expect(arg.target).toBe('xflow');
  });

  it('target=xflowd + auto_restart 체크박스 활성 (재시작 권장)', () => {
    gotoConfirm({ isAdmin: true });
    const checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.disabled).toBe(false);
  });

  it('target=xflow-agent + auto_restart 체크박스 활성 (별도 프로세스 재시작 가능)', () => {
    gotoConfirm({ isAdmin: true });
    fireEvent.change(screen.getByTestId('update-dialog-target-select'), {
      target: { value: 'xflow-agent' },
    });
    const checkbox = screen.getByTestId(
      'update-dialog-auto-restart-checkbox',
    ) as HTMLInputElement;
    expect(checkbox.disabled).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 4. Step 3: apply (transient)
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — apply step', () => {
  it('mutation.isPending=true → 로딩 스피너가 표시된다', () => {
    const mutateMock = vi.fn();
    setApplyMutation({ mutate: mutateMock, isPending: true });
    render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    expect(screen.getByTestId('update-dialog-step-apply')).toBeInTheDocument();
  });

  it('mutate 성공 시 progress step 으로 전환된다', async () => {
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: 'op-1',
          status: 'starting',
          from_version: 'v0.3.0',
          to_version: 'v0.4.0',
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: makeOperation('starting') });

    render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-progress'),
      ).toBeInTheDocument();
    });
  });

  it('mutate 실패 시 result step (failure) 으로 전환된다', async () => {
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onError?.(new Error('updater: insufficient disk space'));
      },
    );
    setApplyMutation({ mutate: mutateMock });

    render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-result'),
      ).toBeInTheDocument();
    });
    expect(screen.getByTestId('update-dialog-result-failure')).toHaveTextContent(
      /디스크/,
    );
  });
});

// ─────────────────────────────────────────────────────────────────────
// 5. Step 4: progress
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — progress step', () => {
  function gotoProgress(operationStatus: OperationStatus = 'downloading') {
    const op = makeOperation(operationStatus);
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: op });
    const onClose = vi.fn();
    render(<UpdateDialog open={true} onClose={onClose} version={makeVersion()} />);
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    return { onClose };
  }

  it('progress step 에서 stepper 가 표시된다', async () => {
    gotoProgress('downloading');
    await waitFor(() => {
      expect(
        screen.getByTestId('update-progress-stepper'),
      ).toBeInTheDocument();
    });
  });

  it('useUpdateStatus 가 enabled=true 로 호출된다 (폴링 활성)', async () => {
    gotoProgress('downloading');
    await waitFor(() => {
      expect(useUpdateStatusMock).toHaveBeenCalled();
    });
    const lastCall =
      useUpdateStatusMock.mock.calls[
        useUpdateStatusMock.mock.calls.length - 1
      ]![0];
    expect(lastCall).toMatchObject({ enabled: true });
  });

  it('"닫기" 버튼은 dialog 를 닫지만 작업은 중단하지 않는다', async () => {
    const { onClose } = gotoProgress('downloading');
    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-progress-close'),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('update-dialog-progress-close'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('status=completed → result success 로 전환된다', async () => {
    // 처음에는 downloading 으로 진입.
    const op = makeOperation('downloading');
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });

    // 첫 번째 렌더: downloading
    setStatusQuery({ data: op });
    const { rerender } = render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-progress'),
      ).toBeInTheDocument();
    });

    // 두 번째 렌더: completed
    setStatusQuery({ data: makeOperation('completed') });
    rerender(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-success'),
      ).toBeInTheDocument();
    });
  });

  it('status=ready_to_restart → result restart-required 로 전환된다', async () => {
    const op = makeOperation('downloading');
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: op });

    const { rerender } = render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-progress'),
      ).toBeInTheDocument();
    });

    setStatusQuery({ data: makeOperation('ready_to_restart') });
    rerender(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-restart'),
      ).toBeInTheDocument();
    });
    // RestartGuide 가 안에 포함되어야 한다.
    expect(screen.getByTestId('restart-guide-title')).toBeInTheDocument();
  });

  it('status=failed → result failure 로 전환된다', async () => {
    const op = makeOperation('downloading');
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: op });

    const { rerender } = render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-progress'),
      ).toBeInTheDocument();
    });

    setStatusQuery({
      data: makeOperation('failed', {
        error: 'updater: checksum mismatch',
      }),
    });
    rerender(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-failure'),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByTestId('update-dialog-result-failure'),
    ).toHaveTextContent(/체크섬/);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 6. Step 5: result
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — result step actions', () => {
  function setupFailureResult() {
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onError?.(new Error('updater: apply failed'));
      },
    );
    setApplyMutation({ mutate: mutateMock });
    const onClose = vi.fn();
    render(<UpdateDialog open={true} onClose={onClose} version={makeVersion()} />);
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    return { onClose };
  }

  it('failure: 닫기 / 다시 시도 / Rollback 시도 3개 버튼이 모두 표시된다', async () => {
    setupFailureResult();
    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-failure'),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByTestId('update-dialog-result-close'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('update-dialog-result-retry'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('update-dialog-result-rollback'),
    ).toBeInTheDocument();
  });

  it('Rollback 버튼 클릭 시 useUpdateRollback.mutate 가 호출된다', async () => {
    const rollbackMutate = vi.fn();
    setRollbackMutation({ mutate: rollbackMutate });
    setupFailureResult();
    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-rollback'),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('update-dialog-result-rollback'));
    expect(rollbackMutate).toHaveBeenCalledTimes(1);
  });

  it('"다시 시도" 클릭 시 dialog 가 info step 으로 리셋된다', async () => {
    const resetMock = vi.fn();
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onError?.(new Error('updater: apply failed'));
      },
    );
    setApplyMutation({ mutate: mutateMock, reset: resetMock });
    render(<UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />);
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-failure'),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId('update-dialog-result-retry'));
    expect(screen.getByTestId('update-dialog-step-info')).toBeInTheDocument();
  });

  it('result success: 닫기 버튼만 표시된다', async () => {
    const op = makeOperation('downloading');
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: op });
    const { rerender } = render(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));
    setStatusQuery({ data: makeOperation('completed') });
    rerender(
      <UpdateDialog open={true} onClose={vi.fn()} version={makeVersion()} />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-result-success'),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByTestId('update-dialog-result-close'),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('update-dialog-result-retry'),
    ).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 7. Keyboard / backdrop
// ─────────────────────────────────────────────────────────────────────

describe('UpdateDialog — 키보드 및 backdrop', () => {
  it('Esc 키 (info step) → onClose 가 호출된다', () => {
    const onClose = vi.fn();
    render(
      <UpdateDialog open={true} onClose={onClose} version={makeVersion()} />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('progress 진행 중 Esc 키 → 자동으로 닫지 않는다 (작업 백그라운드 보존)', async () => {
    // 작업 진행 중인 상태로 진입.
    const op = makeOperation('downloading');
    const mutateMock = vi.fn(
      (
        _vars: ApplyRequest,
        opts?: {
          onSuccess?: (data: ApplyResponse) => void;
        },
      ) => {
        opts?.onSuccess?.({
          operation_id: op.operation_id,
          status: 'starting',
          from_version: op.from_version,
          to_version: op.to_version,
        });
      },
    );
    setApplyMutation({ mutate: mutateMock });
    setStatusQuery({ data: op });

    const onClose = vi.fn();
    render(<UpdateDialog open={true} onClose={onClose} version={makeVersion()} />);
    fireEvent.click(screen.getByTestId('update-dialog-next'));
    fireEvent.click(screen.getByTestId('update-dialog-apply'));

    await waitFor(() => {
      expect(
        screen.getByTestId('update-dialog-step-progress'),
      ).toBeInTheDocument();
    });

    // Esc 키 — progress 단계에서는 명시적 "닫기" 버튼만 허용.
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
  });
});
