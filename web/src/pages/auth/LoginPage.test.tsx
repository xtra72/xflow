// 로그인 후 도착 지점 검증.
//
// 로그인 직후 시작점은 언제나 대시보드다. 예전에는 세션이 끊긴 화면으로
// 되돌려 보냈는데(returnUrl), 그 경로가 되살아나면 사용자는 로그인할 때마다
// 다른 화면에 떨어진다.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

const loginMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({ login: loginMock, isLoading: false }),
}));

import { I18nProvider } from '@/lib/i18n';
import LoginPage from './LoginPage';

/** `/login` 에서 시작해 로그인 후 어느 라우트에 도착하는지 관찰한다. */
function renderAt(initialEntry: string) {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div data-testid="dashboard" />} />
          <Route path="/devices" element={<div data-testid="devices" />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

/** 폼을 채우고 제출한다. */
function submitLogin() {
  fireEvent.change(screen.getByLabelText('사용자명'), { target: { value: 'admin' } });
  fireEvent.change(screen.getByLabelText('비밀번호'), { target: { value: 'pw' } });
  fireEvent.submit(screen.getByRole('button', { name: '로그인' }));
}

beforeEach(() => {
  loginMock.mockReset();
  loginMock.mockResolvedValue(undefined);
});

describe('LoginPage 로그인 후 이동', () => {
  it('로그인에 성공하면 대시보드로 간다', async () => {
    renderAt('/login');
    submitLogin();

    await waitFor(() => expect(screen.getByTestId('dashboard')).toBeInTheDocument());
  });

  it('returnUrl 이 붙어 있어도 대시보드로 간다 — 항상 대시보드가 시작점이다', async () => {
    renderAt('/login?returnUrl=%2Fdevices');
    submitLogin();

    await waitFor(() => expect(screen.getByTestId('dashboard')).toBeInTheDocument());
    expect(screen.queryByTestId('devices')).not.toBeInTheDocument();
  });

  it('로그인에 실패하면 이동하지 않고 오류를 보여준다', async () => {
    loginMock.mockRejectedValue(new Error('bad credentials'));
    renderAt('/login');
    submitLogin();

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    expect(screen.queryByTestId('dashboard')).not.toBeInTheDocument();
  });

  it('입력이 비어 있으면 로그인 API 를 부르지 않는다', () => {
    renderAt('/login');
    fireEvent.submit(screen.getByRole('button', { name: '로그인' }));

    expect(loginMock).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });
});
