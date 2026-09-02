// ScheduleLogStorageCard 컴포넌트 테스트.
//
// - 서버 값 로드 후 select 초기화, 변경 없으면 저장 버튼 비활성화.
// - select 변경 → 저장 클릭 시 setScheduleLogStorageType 가 선택 값으로 호출된다.
// - needs_restart=true 응답 시 "재시작 후 적용" 안내가 표시된다.
// - viewer 역할이면 컨트롤이 비활성화된다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const getStorageTypeMock = vi.hoisted(() => vi.fn());
const setStorageTypeMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/scheduleLogConfigService', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/scheduleLogConfigService')
  >('@/services/api/scheduleLogConfigService');
  return {
    ...actual,
    getScheduleLogStorageType: getStorageTypeMock,
    setScheduleLogStorageType: setStorageTypeMock,
  };
});

// uiStore mock — addNotification 호출만 캡처한다(SettingsPage.test.tsx 패턴).
const addNotificationMock = vi.hoisted(() => vi.fn());

vi.mock('@/stores/uiStore', async () => {
  const actual = await vi.importActual<typeof import('@/stores/uiStore')>('@/stores/uiStore');
  return {
    ...actual,
    useUIStore: Object.assign(
      (selector?: (state: { addNotification: typeof addNotificationMock }) => unknown) => {
        const state = { addNotification: addNotificationMock };
        return selector ? selector(state) : state;
      },
      { getState: () => ({ addNotification: addNotificationMock }) },
    ),
  };
});

import { I18nProvider } from '@/lib/i18n';

import { ScheduleLogStorageCard } from './ScheduleLogStorageCard';

function buildWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={client}>
        <I18nProvider>{children}</I18nProvider>
      </QueryClientProvider>
    );
  }
  return { Wrapper };
}

beforeEach(() => {
  localStorage.clear();
  getStorageTypeMock.mockReset();
  setStorageTypeMock.mockReset();
  addNotificationMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('ScheduleLogStorageCard', () => {
  it('서버 값을 로드해 select 를 초기화하고, 변경 전에는 저장 버튼을 비활성화한다', async () => {
    getStorageTypeMock.mockResolvedValue('sqlite');
    const { Wrapper } = buildWrapper();
    render(<ScheduleLogStorageCard isViewer={false} />, { wrapper: Wrapper });

    const select = (await screen.findByLabelText('저장 방식 선택')) as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe('sqlite'));

    const saveButton = screen.getByRole('button', { name: '저장' });
    expect(saveButton).toBeDisabled();
  });

  it('select 변경 후 저장 시 setScheduleLogStorageType 를 선택 값으로 호출하고, needs_restart 안내를 표시한다', async () => {
    getStorageTypeMock.mockResolvedValue('sqlite');
    setStorageTypeMock.mockResolvedValue({ needsRestart: true });
    const { Wrapper } = buildWrapper();
    render(<ScheduleLogStorageCard isViewer={false} />, { wrapper: Wrapper });

    const select = (await screen.findByLabelText('저장 방식 선택')) as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe('sqlite'));

    fireEvent.change(select, { target: { value: 'file' } });

    const saveButton = screen.getByRole('button', { name: '저장' });
    expect(saveButton).not.toBeDisabled();
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(setStorageTypeMock).toHaveBeenCalledWith('file');
    });

    // 재시작 후 적용 안내 + 성공 알림.
    expect(await screen.findByText('재시작 후 적용됩니다')).toBeInTheDocument();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('viewer 역할이면 select 와 저장 버튼을 비활성화한다', async () => {
    getStorageTypeMock.mockResolvedValue('memory');
    const { Wrapper } = buildWrapper();
    render(<ScheduleLogStorageCard isViewer={true} />, { wrapper: Wrapper });

    const select = (await screen.findByLabelText('저장 방식 선택')) as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe('memory'));

    expect(select).toBeDisabled();
    expect(screen.getByRole('button', { name: '저장' })).toBeDisabled();
  });
});
