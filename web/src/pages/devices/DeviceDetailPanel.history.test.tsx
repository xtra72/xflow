// DeviceDetailPanel 최근 데이터(이력) 섹션 테스트.
//
// 상세/이력/명령 훅을 mock 하여 다음을 검증한다:
//   - 이력 항목 렌더 (최신순 행 + online/시각)
//   - 빈 이력 → "최근 데이터 없음"
//   - 이력 비활성(404) → 안내 문구
//   - limit 셀렉트 변경 → useDeviceHistory 가 새 limit 으로 재호출

import { render, screen, fireEvent } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';
import type { DeviceDetail, DeviceHistoryEntry } from '@/types/device';

// ---- 상세 훅 mock (이력 섹션 외 패널 본문은 최소 데이터로 채움) ----
const useDeviceDetailTargetMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useDetailTargets', () => ({
  useDeviceDetailTarget: useDeviceDetailTargetMock,
}));

// ---- 이력 훅 mock ----
const useDeviceHistoryMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useDevice', () => ({
  useDeviceHistory: useDeviceHistoryMock,
  useExecuteCommand: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUpdateMetadata: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock('@/hooks/useOptimisticToggle', () => ({
  useOptimisticToggle: (v: boolean | undefined) => ({
    displayValue: v,
    setOptimistic: vi.fn(),
    isPendingConfirmation: false,
  }),
}));

// 타깃 컨텍스트는 로컬로 고정.
vi.mock('@/lib/remote/TargetContext', () => ({
  useTargetContext: () => ({ type: 'local' }),
}));

import DeviceDetailPanel from './DeviceDetailPanel';

function makeDetail(overrides: Partial<DeviceDetail> = {}): DeviceDetail {
  return {
    id: 'uuid-1',
    uid: 'uuid-1',
    name: '거실 에어컨',
    type: 'indoor',
    protocol: 'modbus',
    agent_name: 'agent-a',
    source: 'config',
    online: true,
    last_seen: new Date().toISOString(),
    capabilities: [],
    state: { online: true, ready: true, last_seen: '', error_count: 0, properties: {} },
    commands: [],
    ...overrides,
  };
}

function makeEntry(overrides: Partial<DeviceHistoryEntry> = {}): DeviceHistoryEntry {
  return {
    timestamp: 1_700_000_000_000,
    online: true,
    last_seen: 1_700_000_000_000,
    properties: { temperature: 24 },
    ...overrides,
  };
}

beforeEach(() => {
  useDeviceDetailTargetMock.mockReset();
  useDeviceHistoryMock.mockReset();
  useDeviceDetailTargetMock.mockReturnValue({
    data: makeDetail(),
    isLoading: false,
    error: null,
  });
});

describe('DeviceDetailPanel 이력 섹션', () => {
  it('이력 항목을 렌더한다 (시각 + 온라인 배지)', () => {
    useDeviceHistoryMock.mockReturnValue({
      data: { device_id: 'uuid-1', count: 2, entries: [makeEntry(), makeEntry({ online: false })] },
      isLoading: false,
      error: null,
      isFetching: false,
    });

    render(<DeviceDetailPanel deviceId="uuid-1" />);

    expect(screen.getByText('최근 데이터(이력)')).toBeInTheDocument();
    expect(screen.getByText('온라인')).toBeInTheDocument();
    expect(screen.getByText('오프라인')).toBeInTheDocument();
  });

  it('속성을 개별 컬럼(셀)으로 분리해 표시한다 (요약 문자열 아님)', () => {
    useDeviceHistoryMock.mockReturnValue({
      data: {
        device_id: 'uuid-1',
        count: 1,
        entries: [makeEntry({ properties: { note_x: 'abc123' } })],
      },
      isLoading: false,
      error: null,
      isFetching: false,
    });

    render(<DeviceDetailPanel deviceId="uuid-1" />);

    // 값이 개별 셀로 렌더된다.
    expect(screen.getByText('abc123')).toBeInTheDocument();
    // 이전의 "키=값, 키=값" 요약 문자열은 더 이상 없다.
    expect(screen.queryByText(/note_x=abc123/)).toBeNull();
  });

  it('빈 이력이면 "최근 데이터 없음" 을 표시한다', () => {
    useDeviceHistoryMock.mockReturnValue({
      data: { device_id: 'uuid-1', count: 0, entries: [] },
      isLoading: false,
      error: null,
      isFetching: false,
    });

    render(<DeviceDetailPanel deviceId="uuid-1" />);
    expect(screen.getByText('최근 데이터 없음')).toBeInTheDocument();
  });

  it('이력 비활성(404)이면 안내 문구를 표시한다', () => {
    useDeviceHistoryMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new APIError('NOT_FOUND', 'device history is not enabled', 404),
      isFetching: false,
    });

    render(<DeviceDetailPanel deviceId="uuid-1" />);
    expect(screen.getByText('이력 기록이 비활성화되어 있습니다.')).toBeInTheDocument();
  });

  it('limit 셀렉트 변경 시 새 limit 으로 useDeviceHistory 를 호출한다', () => {
    useDeviceHistoryMock.mockReturnValue({
      data: { device_id: 'uuid-1', count: 1, entries: [makeEntry()] },
      isLoading: false,
      error: null,
      isFetching: false,
    });

    render(<DeviceDetailPanel deviceId="uuid-1" />);

    // 초기 호출은 기본 limit(100) — uid 우선 식별자.
    expect(useDeviceHistoryMock).toHaveBeenCalledWith('uuid-1', 100, true);

    // 개수 셀렉트를 200 으로 변경
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: '200' } });

    expect(useDeviceHistoryMock).toHaveBeenLastCalledWith('uuid-1', 200, true);
  });
});
