// 유휴 로그아웃 배선 층의 계약 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 순수 층(idlePolicy)은 따로 시험한다. 여기서 고정하는 것은 **이음매**다 —
// 이벤트가 활동으로 기록되는가, 탭 사이로 전해지는가, 만료가 정말 로그아웃을
// 부르는가, 꺼져 있을 때 아무 일도 하지 않는가.

import { act, render } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '@/stores/authStore';
import { IDLE_ACTIVITY_STORAGE_KEY, useIdleLogout } from './useIdleLogout';
import type { IdleLogoutSetting } from '@/lib/idle/idlePolicy';

const logoutMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({ logout: logoutMock }),
}));

const fetchSettingMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/idleLogoutService', () => ({
  fetchIdleLogoutSetting: fetchSettingMock,
  saveIdleLogoutSetting: vi.fn(),
  IDLE_LOGOUT_SETTING_KEY: 'auth.idle-logout',
}));

/** 훅 상태를 밖으로 꺼내는 프로브. */
function Probe({ onState }: { onState: (s: ReturnType<typeof useIdleLogout>) => void }) {
  const state = useIdleLogout();
  onState(state);
  return <div data-testid="phase">{state.phase}</div>;
}

function renderIdle(setting: IdleLogoutSetting) {
  fetchSettingMock.mockResolvedValue(setting);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  let last: ReturnType<typeof useIdleLogout> | null = null;
  const utils = render(
    <QueryClientProvider client={qc}>
      <Probe
        onState={(s) => {
          last = s;
        }}
      />
    </QueryClientProvider>,
  );
  return { ...utils, state: () => last };
}

/** 설정 쿼리가 해소되고 타이머가 한 바퀴 돌 때까지 진행시킨다. */
async function settle(ms = 1000) {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    vi.advanceTimersByTime(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-20T09:00:00Z'));
  logoutMock.mockReset();
  fetchSettingMock.mockReset();
  localStorage.clear();
  useAuthStore.setState({ authEnabled: true, isAuthenticated: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useIdleLogout — 무활동이 한도를 넘으면 로그아웃한다', () => {
  it('한도 직전에는 경고, 한도를 넘으면 로그아웃', async () => {
    const { state } = renderIdle({ enabled: true, timeoutMinutes: 10 });
    await settle();
    expect(state()?.phase).toBe('active');

    // 9분 경과 — 남은 1분이므로 경고.
    await act(async () => {
      vi.advanceTimersByTime(9 * 60_000);
    });
    expect(state()?.phase).toBe('warning');
    expect(logoutMock).not.toHaveBeenCalled();

    // 10분 경과 — 로그아웃.
    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    expect(state()?.phase).toBe('expired');
    expect(logoutMock).toHaveBeenCalledTimes(1);
  });

  it('로그아웃은 한 번만 부른다 — 만료 뒤 tick 이 이어져도', async () => {
    renderIdle({ enabled: true, timeoutMinutes: 1 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(5 * 60_000);
    });
    expect(logoutMock).toHaveBeenCalledTimes(1);
  });

  it('입력이 있으면 한도가 다시 시작된다', async () => {
    const { state } = renderIdle({ enabled: true, timeoutMinutes: 10 });
    await settle();

    await act(async () => {
      vi.advanceTimersByTime(9 * 60_000);
    });
    expect(state()?.phase).toBe('warning');

    // 키를 하나 누른다 = 활동.
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }));
      vi.advanceTimersByTime(1000);
    });
    expect(state()?.phase).toBe('active');

    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it("'계속 사용'은 한도를 되돌린다", async () => {
    const { state } = renderIdle({ enabled: true, timeoutMinutes: 10 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(9 * 60_000);
    });
    expect(state()?.phase).toBe('warning');

    await act(async () => {
      state()?.extend();
      vi.advanceTimersByTime(1000);
    });
    expect(state()?.phase).toBe('active');
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it('다른 탭의 활동도 내 한도를 연장한다', async () => {
    const { state } = renderIdle({ enabled: true, timeoutMinutes: 10 });
    await settle();

    await act(async () => {
      vi.advanceTimersByTime(9 * 60_000);
    });
    expect(state()?.phase).toBe('warning');

    // 다른 탭이 방금 활동을 기록했다.
    await act(async () => {
      localStorage.setItem(IDLE_ACTIVITY_STORAGE_KEY, String(Date.now()));
      vi.advanceTimersByTime(1000);
    });
    expect(state()?.phase).toBe('active');
    expect(logoutMock).not.toHaveBeenCalled();
  });
});

describe('useIdleLogout — 돌지 않아야 할 때', () => {
  it('설정이 꺼져 있으면 아무 일도 하지 않는다', async () => {
    const { state } = renderIdle({ enabled: false, timeoutMinutes: 1 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(state()?.phase).toBe('active');
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it('인증이 꺼진 서버에서는 돌지 않는다', async () => {
    useAuthStore.setState({ authEnabled: false, isAuthenticated: false });
    renderIdle({ enabled: true, timeoutMinutes: 1 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it('미인증(로그인 화면)에서는 돌지 않는다', async () => {
    useAuthStore.setState({ authEnabled: true, isAuthenticated: false });
    renderIdle({ enabled: true, timeoutMinutes: 1 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it('설정을 읽지 못해도 기본 정책(10분·켜짐)으로 돈다 — 조용히 꺼지지 않는다', async () => {
    // 서비스가 폴백을 책임지므로 여기서는 기본값이 온 상황을 그대로 쓴다.
    renderIdle({ enabled: true, timeoutMinutes: 10 });
    await settle();
    await act(async () => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(logoutMock).toHaveBeenCalledTimes(1);
  });
});
