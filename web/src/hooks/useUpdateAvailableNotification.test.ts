// SPEC-WEB-006 v0.1.0 (M4) — useUpdateAvailableNotification 훅 테스트.
//
// useSystemVersion 의 update_available 이 false → true 로 전이되는 순간에만
// uiStore.addNotification(type='info') 을 호출한다. 초기 로드(prev=undefined)
// 또는 연속된 true 폴링에서는 발화하지 않는다 (리프레시 스팸 방지).
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// ─────────────────────────────────────────────────────────────────────
// Mocks
// ─────────────────────────────────────────────────────────────────────

const useSystemVersionMock = vi.hoisted(() => vi.fn());
const addNotificationMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/systemUpdate', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useSystemVersion: useSystemVersionMock,
  };
});

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: typeof addNotificationMock }) => unknown) =>
    selector({ addNotification: addNotificationMock }),
}));

import type { VersionInfo } from '@/services/api/systemUpdate';

import { useUpdateAvailableNotification } from './useUpdateAvailableNotification';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v1.0.0',
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

function setVersionData(data: VersionInfo | undefined): void {
  useSystemVersionMock.mockReturnValue({
    data,
    isLoading: false,
    isError: false,
  });
}

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

describe('useUpdateAvailableNotification', () => {
  beforeEach(() => {
    addNotificationMock.mockReset();
    useSystemVersionMock.mockReset();
  });

  it('초기 로드(prev=undefined, current=true)에서는 발화하지 않는다', () => {
    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v1.1.0' }),
    );
    renderHook(() => useUpdateAvailableNotification());
    expect(addNotificationMock).not.toHaveBeenCalled();
  });

  it('false → true 전이에서 정확히 한 번 발화한다', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    expect(addNotificationMock).not.toHaveBeenCalled();

    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v1.1.0' }),
    );
    rerender();
    expect(addNotificationMock).toHaveBeenCalledTimes(1);
  });

  it('true → false 전이에서는 발화하지 않는다 (이미 업데이트 됨)', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v1.1.0' }),
    );
    rerender();
    addNotificationMock.mockClear();

    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    rerender();
    expect(addNotificationMock).not.toHaveBeenCalled();
  });

  it('연속된 true 폴링에서는 한 번만 발화한다', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v1.1.0' }),
    );
    rerender();
    expect(addNotificationMock).toHaveBeenCalledTimes(1);

    rerender();
    rerender();
    expect(addNotificationMock).toHaveBeenCalledTimes(1);
  });

  it('알림 메시지에 latest_version 이 포함된다', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v2.5.7' }),
    );
    rerender();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('v2.5.7') as string,
      }),
    );
  });

  it('latest_version=null 일 때 메시지는 "확인 필요" fallback', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    setVersionData(
      makeVersion({ update_available: true, latest_version: null }),
    );
    rerender();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('확인 필요') as string,
      }),
    );
  });

  it('알림은 type=info 로 전달된다', () => {
    setVersionData(
      makeVersion({ update_available: false, latest_version: null }),
    );
    const { rerender } = renderHook(() => useUpdateAvailableNotification());
    setVersionData(
      makeVersion({ update_available: true, latest_version: 'v1.1.0' }),
    );
    rerender();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'info' }),
    );
  });
});
