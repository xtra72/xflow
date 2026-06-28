// useDeviceColumns 훅 테스트.
//
// settingsService 를 mock 하여 다음을 검증한다:
//   - 미저장(404) → 기본 컬럼 세트 폴백
//   - 저장된 value → 정의 순서로 정규화 (알 수 없는 키 제거)
//   - 빈 columns → 기본값 폴백 (최소 1개 보장)
//   - setColumns → putSetting 호출 (정의 순서 정규화)

import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';

const getSettingMock = vi.hoisted(() => vi.fn());
const putSettingMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/settingsService', () => ({
  getSetting: getSettingMock,
  putSetting: putSettingMock,
}));

import {
  useDeviceColumns,
  DEFAULT_DEVICE_COLUMNS,
} from './useDeviceColumns';

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  getSettingMock.mockReset();
  putSettingMock.mockReset();
});

describe('useDeviceColumns', () => {
  it('미저장(404)이면 기본 컬럼 세트로 폴백한다', async () => {
    getSettingMock.mockRejectedValueOnce(new APIError('NOT_FOUND', 'not found', 404));

    const { result } = renderHook(() => useDeviceColumns(), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.columns).toEqual(DEFAULT_DEVICE_COLUMNS);
  });

  it('저장된 value 를 정의 순서로 정규화하고 알 수 없는 키는 제거한다', async () => {
    // 순서 뒤섞임 + 알 수 없는 키 'bogus'
    getSettingMock.mockResolvedValueOnce({ columns: ['status', 'name', 'bogus', 'id'] });

    const { result } = renderHook(() => useDeviceColumns(), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    // 정의 순서(name, id, ..., status, ...) 로 정렬, bogus 제거
    expect(result.current.columns).toEqual(['name', 'id', 'status']);
  });

  it('빈 columns 는 기본값으로 폴백한다(최소 1개 보장)', async () => {
    getSettingMock.mockResolvedValueOnce({ columns: [] });

    const { result } = renderHook(() => useDeviceColumns(), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.columns).toEqual(DEFAULT_DEVICE_COLUMNS);
  });

  it('setColumns 는 정의 순서로 정규화하여 putSetting 을 호출한다', async () => {
    getSettingMock.mockResolvedValueOnce({ columns: ['name', 'id'] });
    putSettingMock.mockResolvedValue({ columns: ['name', 'type'] });

    const { result } = renderHook(() => useDeviceColumns(), { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      // 입력 순서 뒤섞임 → 정의 순서로 저장되어야 함
      result.current.setColumns(['type', 'name']);
    });

    await waitFor(() => expect(putSettingMock).toHaveBeenCalledTimes(1));
    expect(putSettingMock).toHaveBeenCalledWith('device-list-columns', {
      columns: ['name', 'type'],
    });
  });
});
