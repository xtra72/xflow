// SPEC-WEB-006 v0.1.0 (M2, M3) — SystemStatusPage 통합 테스트.
//
// useSystemVersion + useUpdateCheck 훅을 mock 으로 주입하여 페이지 동작을
// 결정적으로 검증한다. uiStore 의 toast 알림도 검증한다.
//
// 테스트 범위:
//   - 로딩 / 에러 / 성공 상태 렌더링
//   - Check 버튼 mutation 트리거 + 성공 시 query 무효화
//   - mapUpdateError 통한 에러 메시지 변환
//   - update_available 전환 인라인 알림
//
// @spec SPEC-WEB-006 v0.1.0 (M2, M3)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';

// ─────────────────────────────────────────────────────────────────────
// systemUpdate 모듈 mock — 훅의 반환값을 테스트별로 제어한다.
// ─────────────────────────────────────────────────────────────────────

const useSystemVersionMock = vi.hoisted(() => vi.fn());
const useUpdateCheckMock = vi.hoisted(() => vi.fn());
// Phase D — UpdateDialog 의 훅들도 함께 mock 처리한다 (실제 React Query 클라이언트 의존성 차단).
const useUpdateApplyMock = vi.hoisted(() => vi.fn());
const useUpdateStatusMock = vi.hoisted(() => vi.fn());
const useUpdateRollbackMock = vi.hoisted(() => vi.fn());
// Phase C (SPEC-UPDATE-002 v0.1.0 M8) — ChannelChangeDialog 의 훅들도 mock.
const useChannelInfoMock = vi.hoisted(() => vi.fn());
const useChangeChannelMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/systemUpdate', async () => {
  // 실제 모듈에서 type-only re-export 가 필요하므로, 부분 mock 사용.
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useSystemVersion: useSystemVersionMock,
    useUpdateCheck: useUpdateCheckMock,
    useUpdateApply: useUpdateApplyMock,
    useUpdateStatus: useUpdateStatusMock,
    useUpdateRollback: useUpdateRollbackMock,
    useChannelInfo: useChannelInfoMock,
    useChangeChannel: useChangeChannelMock,
  };
});

// useAuth mock — admin / non-admin 분기를 테스트에서 제어.
const useAuthMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({
  useAuth: useAuthMock,
}));

// usePermission mock — SPEC-AUTH-006 M3: 채널 변경 게이팅이 role==='admin'
// 에서 'system.update' 권한 키 판정으로 바뀌었다. 기본은 전원 허용이며,
// 권한 부족 상황은 각 테스트가 denied 에 키를 넣어 만든다.
const permissionMock = vi.hoisted(() => ({ denied: new Set<string>() }));
vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => !permissionMock.denied.has(key),
    hasAnyPermission: (keys: readonly string[]) =>
      keys.some((key) => !permissionMock.denied.has(key)),
    isPermissionUnavailable: false,
  }),
}));

// uiStore 알림 캡처용 mock — addNotification 호출 검증.
const addNotificationMock = vi.hoisted(() => vi.fn());

vi.mock('@/stores/uiStore', async () => {
  const actual = await vi.importActual<typeof import('@/stores/uiStore')>(
    '@/stores/uiStore',
  );
  return {
    ...actual,
    useUIStore: Object.assign(
      // 함수형 selector 호출과 직접 호출 둘 다 지원
      (selector?: (state: { addNotification: typeof addNotificationMock }) => unknown) => {
        const state = { addNotification: addNotificationMock };
        return selector ? selector(state) : state;
      },
      {
        getState: () => ({ addNotification: addNotificationMock }),
      },
    ),
  };
});

// i18n 모킹: I18nProvider 없이 렌더하기 위해 useTranslation 을 교체한다.
// 에러 토스트 메시지가 실제 한국어(mapUpdateError → t(key))를 검사하므로
// ko.json 을 해석해 반환한다. (async factory 내부 import 로 호이스팅 회피.)
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolveKo = (key: string): string => {
    const value = key
      .split('.')
      .reduce<unknown>(
        (obj, part) =>
          obj != null && typeof obj === 'object'
            ? (obj as Record<string, unknown>)[part]
            : undefined,
        ko,
      );
    return typeof value === 'string' ? value : key;
  };
  return {
    useTranslation: () => ({ t: (k: string) => resolveKo(k) }),
  };
});

import type { VersionInfo } from '@/services/api/systemUpdate';

import { SystemStatusPage } from './SystemStatusPage';

// ─────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v0.3.0',
    commit: 'abc1234',
    build_date: '2026-04-30T12:00:00Z',
    go_version: 'go1.25.0',
    channel: 'stable',
    update_available: false,
    latest_version: null,
    // SPEC-WEB-007 추가 필드.
    os: 'linux',
    arch: 'amd64',
    hostname: 'xflow-node-01',
    mode: 'server',
    uptime_seconds: 3600,
    ...overrides,
  };
}

function buildWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return { client, Wrapper };
}

// ─────────────────────────────────────────────────────────────────────
// Default mock states
// ─────────────────────────────────────────────────────────────────────

interface VersionQueryState {
  data?: VersionInfo;
  isLoading?: boolean;
  isError?: boolean;
  error?: Error;
  dataUpdatedAt?: number;
  refetch?: () => void;
}

function setVersionQueryState(state: VersionQueryState) {
  useSystemVersionMock.mockReturnValue({
    data: state.data,
    isLoading: state.isLoading ?? false,
    isError: state.isError ?? false,
    error: state.error,
    dataUpdatedAt: state.dataUpdatedAt ?? 0,
    refetch: state.refetch ?? vi.fn(),
  });
}

interface CheckMutationState {
  mutate?: ReturnType<typeof vi.fn>;
  isPending?: boolean;
}

function setCheckMutationState(state: CheckMutationState = {}) {
  useUpdateCheckMock.mockReturnValue({
    mutate: state.mutate ?? vi.fn(),
    isPending: state.isPending ?? false,
  });
}

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  useSystemVersionMock.mockReset();
  useUpdateCheckMock.mockReset();
  useUpdateApplyMock.mockReset();
  useUpdateStatusMock.mockReset();
  useUpdateRollbackMock.mockReset();
  useChannelInfoMock.mockReset();
  useChangeChannelMock.mockReset();
  useAuthMock.mockReset();
  addNotificationMock.mockReset();
  permissionMock.denied = new Set<string>();

  // Phase D — UpdateDialog 의 훅 default state (idle).
  useUpdateApplyMock.mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
    data: null,
    reset: vi.fn(),
  });
  useUpdateStatusMock.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
  });
  useUpdateRollbackMock.mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  });
  // Phase C (SPEC-UPDATE-002 M8) — ChannelChangeDialog 의 훅 default.
  useChannelInfoMock.mockReturnValue({
    data: { current: 'stable', available: ['stable', 'beta', 'nightly'] },
    isLoading: false,
    isError: false,
  });
  useChangeChannelMock.mockReturnValue({
    mutateAsync: vi.fn(),
    isPending: false,
  });
  // Default: admin 사용자.
  useAuthMock.mockReturnValue({
    user: { name: 'admin', role: 'admin' },
    isAuthenticated: true,
    isLoading: false,
    authEnabled: true,
    initialize: vi.fn(),
  });
});

afterEach(() => {
  vi.useRealTimers();
});

// 1. 로딩 상태
describe('SystemStatusPage — 로딩', () => {
  it('isLoading=true → 로딩 스켈레톤을 표시한다', () => {
    setVersionQueryState({ isLoading: true });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-status-loading')).toBeInTheDocument();
  });
});

// 2. 성공 데이터 렌더링
describe('SystemStatusPage — 성공', () => {
  it('VersionInfo 가 있으면 SystemVersionCard 가 표시된다', () => {
    setVersionQueryState({
      data: makeVersion({ version: 'v0.3.0' }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-version-current')).toHaveTextContent(
      'v0.3.0',
    );
  });

  it('페이지 헤더 "시스템 상태" 가 항상 표시된다', () => {
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-status-header')).toHaveTextContent(
      /시스템 상태/,
    );
  });
});

// 3. 에러 상태
describe('SystemStatusPage — 에러', () => {
  it('isError=true → 에러 메시지 + 재시도 버튼을 표시한다', () => {
    const refetchMock = vi.fn();
    setVersionQueryState({
      isError: true,
      error: new APIError('UNAUTHORIZED', 'unauthorized', 401),
      refetch: refetchMock,
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-status-error')).toBeInTheDocument();

    const retryBtn = screen.getByTestId('system-status-retry-button');
    fireEvent.click(retryBtn);
    expect(refetchMock).toHaveBeenCalledTimes(1);
  });

  it('에러 메시지가 mapUpdateError 로 한글화된다', () => {
    setVersionQueryState({
      isError: true,
      error: new APIError('UNAUTHORIZED', 'unauthorized', 401),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    // mapUpdateError 의 unauthorized → "권한이 없습니다"
    expect(screen.getByTestId('system-status-error')).toHaveTextContent(
      /권한이 없습니다/,
    );
  });
});

// 4. Check 버튼 mutation
describe('SystemStatusPage — Check 버튼', () => {
  it('Check 버튼 클릭 시 mutation.mutate 가 호출된다', () => {
    const mutateMock = vi.fn();
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState({ mutate: mutateMock });

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId('system-check-button'));
    expect(mutateMock).toHaveBeenCalledTimes(1);
  });

  it('mutation.isPending=true → Check 버튼이 비활성화된다', () => {
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState({ isPending: true });

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-check-button')).toBeDisabled();
  });
});

// 5. mutation 성공 → query 무효화 (onSuccess 콜백)
describe('SystemStatusPage — Check 성공', () => {
  it('Check 성공 시 useSystemVersion 쿼리가 무효화된다', async () => {
    const mutateMock = vi.fn(
      (
        _vars: void,
        opts?: {
          onSuccess?: () => void;
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onSuccess?.();
      },
    );
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState({ mutate: mutateMock });

    const { client, Wrapper } = buildWrapper();
    const invalidateSpy = vi.spyOn(client, 'invalidateQueries');

    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId('system-check-button'));

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: expect.arrayContaining(['system', 'version']),
        }),
      );
    });
  });
});

// 6. mutation 에러 → toast (mapUpdateError)
describe('SystemStatusPage — Check 에러', () => {
  it('Check 에러 시 한글 메시지로 toast 알림을 발송한다', async () => {
    const err = new Error('updater: checksum mismatch');
    const mutateMock = vi.fn(
      (
        _vars: void,
        opts?: {
          onSuccess?: () => void;
          onError?: (e: Error) => void;
        },
      ) => {
        opts?.onError?.(err);
      },
    );
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState({ mutate: mutateMock });

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId('system-check-button'));

    await waitFor(() => {
      expect(addNotificationMock).toHaveBeenCalled();
    });
    const call = addNotificationMock.mock.calls[0]![0] as {
      type: string;
      message: string;
    };
    expect(call.type).toBe('error');
    expect(call.message).toMatch(/체크섬/);
  });
});

// 7. update_available 전환 인라인 표시
describe('SystemStatusPage — update_available 전환', () => {
  it('update_available=true 데이터일 때 새 버전 인라인 안내가 표시된다', () => {
    setVersionQueryState({
      data: makeVersion({
        update_available: true,
        latest_version: 'v0.4.0',
      }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-update-available')).toHaveTextContent(
      /v0\.4\.0/,
    );
  });
});

// 8. 향후 변경 영역 placeholder
describe('SystemStatusPage — 향후 영역 placeholder', () => {
  it('업데이트 이력/changelog placeholder 가 표시된다', () => {
    setVersionQueryState({
      data: makeVersion(),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.getByTestId('system-status-future')).toHaveTextContent(
      /곧 추가 예정/,
    );
  });
});

// 9. Phase D — UpdateDialog wire-up
describe('SystemStatusPage — UpdateDialog 결합', () => {
  it('초기 상태에서는 UpdateDialog 가 렌더되지 않는다 (open=false)', () => {
    setVersionQueryState({
      data: makeVersion({
        update_available: true,
        latest_version: 'v0.4.0',
      }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    expect(screen.queryByTestId('update-dialog')).not.toBeInTheDocument();
  });

  it('SystemVersionCard 의 "업데이트 시작" 버튼 클릭 시 UpdateDialog 가 열린다', () => {
    setVersionQueryState({
      data: makeVersion({
        update_available: true,
        latest_version: 'v0.4.0',
      }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    // "업데이트 시작" 버튼은 onUpdate prop 가 주입되었을 때만 노출된다.
    const startBtn = screen.getByTestId('system-update-start-button');
    fireEvent.click(startBtn);

    expect(screen.getByTestId('update-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('update-dialog-step-info')).toBeInTheDocument();
  });

  it('UpdateDialog 의 취소 버튼 클릭 시 dialog 가 닫힌다', () => {
    setVersionQueryState({
      data: makeVersion({
        update_available: true,
        latest_version: 'v0.4.0',
      }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId('system-update-start-button'));
    expect(screen.getByTestId('update-dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('update-dialog-cancel'));
    expect(screen.queryByTestId('update-dialog')).not.toBeInTheDocument();
  });
});

// 10. SPEC-UPDATE-002 v0.1.0 (M8) — ChannelChangeDialog wire-up
describe('SystemStatusPage — ChannelChangeDialog 결합 (SPEC-UPDATE-002 M8)', () => {
  it('admin 사용자: 채널 배지 클릭 시 ChannelChangeDialog 가 열린다', () => {
    useAuthMock.mockReturnValue({
      user: { name: 'admin', role: 'admin' },
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      initialize: vi.fn(),
    });
    setVersionQueryState({
      data: makeVersion({ channel: 'stable' }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    // 초기에는 ChannelChangeDialog 가 노출되지 않는다.
    expect(
      screen.queryByTestId('channel-change-dialog'),
    ).not.toBeInTheDocument();

    // admin 의 채널 배지는 button.
    const channelBtn = screen.getByTestId('system-version-channel');
    expect(channelBtn.tagName.toLowerCase()).toBe('button');

    fireEvent.click(channelBtn);

    expect(screen.getByTestId('channel-change-dialog')).toBeInTheDocument();
  });

  it('system.update 권한 없음: 채널 배지가 클릭 불가 (button 미사용) + dialog 미렌더', () => {
    // SPEC-AUTH-006 M3: 판정 기준이 역할 이름에서 권한 키로 바뀌었다.
    permissionMock.denied = new Set(['system.update']);
    useAuthMock.mockReturnValue({
      user: { name: 'viewer', role: 'viewer' },
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      initialize: vi.fn(),
    });
    setVersionQueryState({
      data: makeVersion({ channel: 'stable' }),
      dataUpdatedAt: Date.now(),
    });
    setCheckMutationState();

    const { Wrapper } = buildWrapper();
    render(
      <Wrapper>
        <SystemStatusPage />
      </Wrapper>,
    );

    const channelEl = screen.getByTestId('system-version-channel');
    expect(channelEl.tagName.toLowerCase()).toBe('span');

    // 클릭해도 dialog 가 열리지 않는다.
    fireEvent.click(channelEl);
    expect(
      screen.queryByTestId('channel-change-dialog'),
    ).not.toBeInTheDocument();
  });
});
