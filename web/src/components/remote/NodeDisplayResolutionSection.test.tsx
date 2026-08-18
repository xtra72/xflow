// NodeDisplayResolutionSection 테스트 (SPEC-REMOTE-001 M12).
//
// useRemote 의 set/clear 디스플레이 뮤테이션, uiStore(toast), useAuth(admin)를
// mock 하여 검증한다. 범위:
//   - EFFECTIVE 해상도 + 출처(오버라이드/노드 보고/기본값) 표시.
//   - 저장: 입력한 W×H 로 setRemoteNodeDisplay 호출(쿼리 무효화는 훅 책임 — 별도 검증).
//   - 프리셋 선택 → 해당 W×H 로 저장.
//   - 검증: 비양수(0/음수/공백) 입력은 거부하고 mutate 를 호출하지 않는다.
//   - 해제: 오버라이드가 있을 때만 노출되고 clearRemoteNodeDisplay 를 호출한다.
//   - 성공/실패 시 토스트(editError 매핑) 표시.
//   - 비-admin 은 컨트롤을 숨기고 읽기 표시만 남긴다.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { NodeDetail } from '@/types/remote';

// ---- useRemote mock (set/clear 뮤테이션) ----
const setMutateMock = vi.hoisted(() => vi.fn());
const clearMutateMock = vi.hoisted(() => vi.fn());
const setPendingRef = vi.hoisted(() => ({ value: false }));
const clearPendingRef = vi.hoisted(() => ({ value: false }));

vi.mock('@/hooks/useRemote', () => ({
  useSetNodeDisplay: () => ({ mutate: setMutateMock, isPending: setPendingRef.value }),
  useClearNodeDisplay: () => ({
    mutate: clearMutateMock,
    isPending: clearPendingRef.value,
  }),
}));

// ---- uiStore mock (toast 캡처) ----
const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (
    selector?: (s: { addNotification: typeof addNotificationMock }) => unknown,
  ) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

// ---- useAuth mock (admin 게이팅) ----
const useAuthMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({
  useAuth: useAuthMock,
}));

// ---- usePermission mock ----
// SPEC-AUTH-006 M3: 편집 컨트롤 게이팅이 role==='admin' 에서 'remote.update'
// 권한 키 판정으로 바뀌었다. 기본은 전원 허용이며, 권한 부족은 denied 로 만든다.
const permissionMock = vi.hoisted(() => ({ denied: new Set<string>() }));
vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => !permissionMock.denied.has(key),
    hasAnyPermission: (keys: readonly string[]) =>
      keys.some((key) => !permissionMock.denied.has(key)),
    isPermissionUnavailable: false,
  }),
}));

import { NodeDisplayResolutionSection } from './NodeDisplayResolutionSection';

function detail(o: Partial<NodeDetail> = {}): NodeDetail {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '1.2.3',
    status: 'approved',
    online: true,
    group_name: 'prod',
    os: 'linux',
    arch: 'arm64',
    started_at: 1_700_000_000_000,
    uptime: 3_600_000,
    last_seen: 1_700_000_100_000,
    display_width: 1280,
    display_height: 720,
    display_override_width: 0,
    display_override_height: 0,
    display_reported_width: 1280,
    display_reported_height: 720,
    summary: {
      flows: { total: 0, running: 0, stopped: 0 },
      agents: { total: 0, connected: 0 },
      devices: { total: 0, online: 0 },
    },
    ...o,
  };
}

function renderSection(d: NodeDetail = detail()) {
  return render(
    <I18nProvider>
      <NodeDisplayResolutionSection instanceId="node-a" detail={d} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  setMutateMock.mockReset();
  clearMutateMock.mockReset();
  addNotificationMock.mockReset();
  setPendingRef.value = false;
  clearPendingRef.value = false;
  permissionMock.denied = new Set<string>();
  // 기본은 admin(컨트롤 노출).
  useAuthMock.mockReset().mockReturnValue({ user: { role: 'admin' }, authEnabled: true });
});

describe('NodeDisplayResolutionSection — EFFECTIVE 해상도 + 출처', () => {
  it('EFFECTIVE 해상도를 W×H 로 표시한다', () => {
    renderSection(detail({ display_width: 1280, display_height: 720 }));
    expect(screen.getByTestId('display-effective')).toHaveTextContent('1280 × 720');
  });

  it('오버라이드가 있으면 출처를 "오버라이드"로 표시한다', () => {
    renderSection(
      detail({
        display_width: 800,
        display_height: 480,
        display_override_width: 800,
        display_override_height: 480,
        display_reported_width: 1280,
        display_reported_height: 720,
      }),
    );
    const src = screen.getByTestId('display-source');
    expect(src).toHaveAttribute('data-source', 'override');
    expect(src).toHaveTextContent('오버라이드');
  });

  it('오버라이드가 없고 노드 보고가 있으면 출처를 "노드 보고"로 표시한다', () => {
    renderSection(
      detail({
        display_override_width: 0,
        display_override_height: 0,
        display_reported_width: 1280,
        display_reported_height: 720,
      }),
    );
    const src = screen.getByTestId('display-source');
    expect(src).toHaveAttribute('data-source', 'reported');
    expect(src).toHaveTextContent('노드 보고');
  });

  it('오버라이드/보고 모두 없으면 출처를 "기본값"으로 표시하고 1920×1080 폴백을 보여준다', () => {
    renderSection(
      detail({
        display_width: 0,
        display_height: 0,
        display_override_width: 0,
        display_override_height: 0,
        display_reported_width: 0,
        display_reported_height: 0,
      }),
    );
    const src = screen.getByTestId('display-source');
    expect(src).toHaveAttribute('data-source', 'fallback');
    expect(screen.getByTestId('display-effective')).toHaveTextContent('1920 × 1080');
  });
});

describe('NodeDisplayResolutionSection — 오버라이드 설정(SET)', () => {
  it('입력한 W×H 로 setRemoteNodeDisplay 를 호출한다(직접 입력)', () => {
    // 보고값(1280×720)이 프리셋과 일치하므로 직접 입력으로 전환해 임의 값 입력.
    renderSection();
    fireEvent.change(screen.getByTestId('display-preset'), {
      target: { value: 'custom' },
    });
    fireEvent.change(screen.getByTestId('display-width-input'), {
      target: { value: '1600' },
    });
    fireEvent.change(screen.getByTestId('display-height-input'), {
      target: { value: '900' },
    });
    fireEvent.click(screen.getByTestId('display-save-button'));

    expect(setMutateMock).toHaveBeenCalledTimes(1);
    const [vars] = setMutateMock.mock.calls[0]!;
    expect(vars).toEqual({ instanceID: 'node-a', width: 1600, height: 900 });
  });

  it('프리셋을 선택하면 해당 W×H 로 저장한다', () => {
    renderSection();
    // 800×480 프리셋 선택(직접 입력 행은 숨겨진다).
    fireEvent.change(screen.getByTestId('display-preset'), {
      target: { value: '800x480' },
    });
    expect(screen.queryByTestId('display-width-input')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('display-save-button'));

    const [vars] = setMutateMock.mock.calls[0]!;
    expect(vars).toEqual({ instanceID: 'node-a', width: 800, height: 480 });
  });

  it('성공 시 성공 토스트를 표시한다', () => {
    setMutateMock.mockImplementation((_vars, opts?: { onSuccess?: () => void }) => {
      opts?.onSuccess?.();
    });
    renderSection();
    fireEvent.change(screen.getByTestId('display-preset'), {
      target: { value: '1024x768' },
    });
    fireEvent.click(screen.getByTestId('display-save-button'));

    expect(addNotificationMock).toHaveBeenCalledWith({
      type: 'success',
      message: expect.stringContaining('오버라이드를 적용'),
    });
  });

  it('실패(503) 시 editError 매핑 메시지로 에러 토스트를 표시한다', () => {
    setMutateMock.mockImplementation(
      (_vars, opts?: { onError?: (e: unknown) => void }) => {
        opts?.onError?.({ status: 503, message: 'offline' });
      },
    );
    renderSection();
    fireEvent.change(screen.getByTestId('display-preset'), {
      target: { value: '1024x768' },
    });
    fireEvent.click(screen.getByTestId('display-save-button'));

    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });
});

describe('NodeDisplayResolutionSection — 입력 검증', () => {
  it.each([
    ['0', '0'],
    ['-100', '720'],
    ['', '720'],
    ['12.5', '720'],
    ['abc', '720'],
  ])('비양수/비정수 입력(%s × %s)은 거부하고 mutate 를 호출하지 않는다', (w, h) => {
    renderSection();
    fireEvent.change(screen.getByTestId('display-preset'), {
      target: { value: 'custom' },
    });
    fireEvent.change(screen.getByTestId('display-width-input'), {
      target: { value: w },
    });
    fireEvent.change(screen.getByTestId('display-height-input'), {
      target: { value: h },
    });
    fireEvent.click(screen.getByTestId('display-save-button'));

    expect(setMutateMock).not.toHaveBeenCalled();
    expect(screen.getByTestId('display-validation-error')).toBeInTheDocument();
  });
});

describe('NodeDisplayResolutionSection — 오버라이드 해제(CLEAR)', () => {
  it('오버라이드가 없으면 해제 버튼을 노출하지 않는다', () => {
    renderSection(detail({ display_override_width: 0, display_override_height: 0 }));
    expect(screen.queryByTestId('display-clear-button')).not.toBeInTheDocument();
  });

  it('오버라이드가 있으면 해제 버튼이 노출되고 clearRemoteNodeDisplay 를 호출한다', () => {
    renderSection(
      detail({
        display_width: 800,
        display_height: 480,
        display_override_width: 800,
        display_override_height: 480,
      }),
    );
    const clearBtn = screen.getByTestId('display-clear-button');
    expect(clearBtn).toBeInTheDocument();
    fireEvent.click(clearBtn);
    expect(clearMutateMock).toHaveBeenCalledTimes(1);
    expect(clearMutateMock.mock.calls[0]![0]).toBe('node-a');
  });

  it('해제 성공 시 성공 토스트를 표시한다', () => {
    clearMutateMock.mockImplementation((_id, opts?: { onSuccess?: () => void }) => {
      opts?.onSuccess?.();
    });
    renderSection(
      detail({ display_override_width: 800, display_override_height: 480 }),
    );
    fireEvent.click(screen.getByTestId('display-clear-button'));
    expect(addNotificationMock).toHaveBeenCalledWith({
      type: 'success',
      message: expect.stringContaining('오버라이드를 해제'),
    });
  });
});

describe('NodeDisplayResolutionSection — 권한 게이팅', () => {
  it('remote.update 권한이 없으면 SET/CLEAR 컨트롤을 숨기고 읽기 표시만 남긴다', () => {
    // SPEC-AUTH-006 M3: 판정 기준이 역할 이름에서 권한 키로 바뀌었다.
    permissionMock.denied = new Set(['remote.update']);
    useAuthMock.mockReturnValue({ user: { role: 'viewer' }, authEnabled: true });
    renderSection(
      detail({ display_override_width: 800, display_override_height: 480 }),
    );
    // 읽기 표시(EFFECTIVE/출처)는 유지된다.
    expect(screen.getByTestId('display-effective')).toBeInTheDocument();
    expect(screen.getByTestId('display-source')).toBeInTheDocument();
    // 편집 컨트롤은 노출되지 않는다.
    expect(screen.queryByTestId('display-controls')).not.toBeInTheDocument();
    expect(screen.queryByTestId('display-save-button')).not.toBeInTheDocument();
    expect(screen.queryByTestId('display-clear-button')).not.toBeInTheDocument();
  });

  it('authEnabled=false(dev 단일 사용자)는 admin 으로 간주해 컨트롤을 노출한다', () => {
    useAuthMock.mockReturnValue({ user: null, authEnabled: false });
    renderSection();
    expect(screen.getByTestId('display-controls')).toBeInTheDocument();
  });
});
