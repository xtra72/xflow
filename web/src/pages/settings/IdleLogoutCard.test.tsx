// 유휴 로그아웃 설정 카드의 계약 (@SPEC:SPEC-AUTH-IDLE-001).

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import { IdleLogoutCard } from './IdleLogoutCard';

const fetchMock = vi.hoisted(() => vi.fn());
const saveMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/idleLogoutService', () => ({
  fetchIdleLogoutSetting: fetchMock,
  saveIdleLogoutSetting: saveMock,
  IDLE_LOGOUT_SETTING_KEY: 'auth.idle-logout',
}));

function renderCard(isReadOnly = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>
        <IdleLogoutCard isReadOnly={isReadOnly} />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  saveMock.mockReset();
  fetchMock.mockResolvedValue({ enabled: true, timeoutMinutes: 10 });
  saveMock.mockImplementation((s) => Promise.resolve(s));
});

describe('IdleLogoutCard', () => {
  it('서버 값으로 폼을 채운다', async () => {
    fetchMock.mockResolvedValue({ enabled: false, timeoutMinutes: 25 });
    renderCard();
    await waitFor(() => {
      expect(screen.getByTestId('idle-logout-minutes')).toHaveValue(25);
    });
    expect(screen.getByTestId('idle-logout-enabled')).not.toBeChecked();
  });

  it('바꾼 값을 저장한다', async () => {
    renderCard();
    await waitFor(() => expect(screen.getByTestId('idle-logout-minutes')).toHaveValue(10));

    fireEvent.change(screen.getByTestId('idle-logout-minutes'), { target: { value: '30' } });
    fireEvent.click(screen.getByTestId('idle-logout-save'));

    await waitFor(() =>
      expect(saveMock).toHaveBeenCalledWith({ enabled: true, timeoutMinutes: 30 }),
    );
  });

  it('바꾼 것이 없으면 저장 단추가 눌리지 않는다', async () => {
    renderCard();
    await waitFor(() => expect(screen.getByTestId('idle-logout-minutes')).toHaveValue(10));
    expect(screen.getByTestId('idle-logout-save')).toBeDisabled();
  });

  it('허용 범위를 벗어난 값으로는 저장할 수 없다', async () => {
    renderCard();
    await waitFor(() => expect(screen.getByTestId('idle-logout-minutes')).toHaveValue(10));
    fireEvent.change(screen.getByTestId('idle-logout-minutes'), { target: { value: '0' } });
    expect(screen.getByTestId('idle-logout-save')).toBeDisabled();
    fireEvent.change(screen.getByTestId('idle-logout-minutes'), { target: { value: '9999' } });
    expect(screen.getByTestId('idle-logout-save')).toBeDisabled();
  });

  it('쓰기 권한이 없으면 모든 컨트롤이 비활성이다', async () => {
    renderCard(true);
    await waitFor(() => expect(screen.getByTestId('idle-logout-minutes')).toBeDisabled());
    expect(screen.getByTestId('idle-logout-enabled')).toBeDisabled();
    expect(screen.getByTestId('idle-logout-save')).toBeDisabled();
  });
});
