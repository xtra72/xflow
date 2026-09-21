// 재연결 간격 설정 칸의 계약 (@SPEC:SPEC-REMOTE-RECONNECT-001).

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import RemoteManagementTab from './RemoteManagementTab';

const getConfigMock = vi.hoisted(() => vi.fn());
const updateConfigMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/remoteConfigService', () => ({
  getRemoteClientConfig: getConfigMock,
  updateRemoteClientConfig: updateConfigMock,
}));

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>
        <RemoteManagementTab />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  getConfigMock.mockReset();
  updateConfigMock.mockReset();
  updateConfigMock.mockResolvedValue({ applied: [], needs_restart: [] });
  getConfigMock.mockResolvedValue({
    mode: 'client',
    server_url: 'wss://example',
    instance_id: 'n1',
    auto_register: true,
    heartbeat_interval: '30s',
    reconnect_initial: '1s',
    reconnect_max: '60s',
    enrollment_token_set: false,
    bootstrap_secret_set: false,
    exposure: { flows: 'all', agents: 'all', devices: 'all' },
    require_secure: false,
    insecure_skip_verify: false,
    display_width: 0,
    display_height: 0,
    restart_required_fields: [],
  });
});

describe('RemoteManagementTab — 재연결 간격', () => {
  it('서버 값으로 두 칸을 채운다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByTestId('remote-reconnect-initial')).toHaveValue('1s'));
    expect(screen.getByTestId('remote-reconnect-max')).toHaveValue('60s');
  });

  it('바꾼 값만 저장 요청에 실린다', async () => {
    renderTab();
    await waitFor(() => expect(screen.getByTestId('remote-reconnect-initial')).toHaveValue('1s'));

    fireEvent.change(screen.getByTestId('remote-reconnect-initial'), { target: { value: '2s' } });
    fireEvent.click(screen.getByRole('button', { name: /저장|Save/ }));

    await waitFor(() => expect(updateConfigMock).toHaveBeenCalled());
    const sent = updateConfigMock.mock.calls.at(-1)?.[0] ?? {};
    expect(sent).toMatchObject({ reconnect_initial: '2s' });
    expect(sent).not.toHaveProperty('reconnect_max'); // 안 바꾼 값은 보내지 않는다.
  });

  it('구버전 서버 응답(필드 없음)에서도 빈 칸으로 뜬다', async () => {
    getConfigMock.mockResolvedValue({
      ...(await getConfigMock.getMockImplementation()?.()),
      mode: 'client',
      server_url: 'wss://example',
      instance_id: 'n1',
      auto_register: true,
      heartbeat_interval: '30s',
      enrollment_token_set: false,
      bootstrap_secret_set: false,
      exposure: { flows: 'all', agents: 'all', devices: 'all' },
      require_secure: false,
      insecure_skip_verify: false,
      display_width: 0,
      display_height: 0,
      restart_required_fields: [],
      reconnect_initial: undefined,
      reconnect_max: undefined,
    });
    renderTab();
    await waitFor(() => expect(screen.getByTestId('remote-reconnect-initial')).toHaveValue(''));
  });
});
