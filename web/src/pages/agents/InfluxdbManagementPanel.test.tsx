// InfluxdbManagementPanel 컴포넌트 테스트.
//
// 범위:
//   - 버킷 목록 렌더 (name / retention / id)
//   - 빈 상태 / v3 미지원(APIError 501) 안내
//   - 버킷 삭제: 확인 다이얼로그 → 확인 시 뮤테이션 호출
//   - 버킷 초기화(truncate): 강한 확인(이름 입력 일치) 후에만 확인 활성화
//   - 버킷 선택 시 measurement 목록 표시 + 삭제 확인
//
// 훅(useBuckets 등)과 uiStore, i18n 은 목킹한다. i18n 은 키를 그대로 반환한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';
import type { InfluxBucket } from '@/services/api/influxdbManagement';

// --- i18n: 키를 그대로 반환하되 {name}/{bucket} 치환은 컴포넌트가 수행하므로 원본 키만 확인 ---
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// --- uiStore: addNotification 스파이 ---
const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification }),
}));

// --- 훅 목킹 ---
const useBucketsMock = vi.hoisted(() => vi.fn());
const useMeasurementsMock = vi.hoisted(() => vi.fn());
const createMutate = vi.hoisted(() => vi.fn());
const deleteBucketMutateAsync = vi.hoisted(() => vi.fn());
const truncateMutateAsync = vi.hoisted(() => vi.fn());
const deleteMeasurementMutateAsync = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useInfluxdbManagement', () => ({
  useBuckets: () => useBucketsMock(),
  useMeasurements: () => useMeasurementsMock(),
  useCreateBucket: () => ({ mutate: createMutate, isPending: false }),
  useDeleteBucket: () => ({
    mutateAsync: deleteBucketMutateAsync,
    isPending: false,
  }),
  useTruncateBucket: () => ({
    mutateAsync: truncateMutateAsync,
    isPending: false,
  }),
  useDeleteMeasurement: () => ({
    mutateAsync: deleteMeasurementMutateAsync,
    isPending: false,
  }),
}));

import InfluxdbManagementPanel from './InfluxdbManagementPanel';

const BUCKETS: InfluxBucket[] = [
  { id: 'b1', name: 'sensors', orgId: 'org1', retentionSeconds: 0 },
  { id: 'b2', name: 'logs', orgId: 'org1', retentionSeconds: 86400 },
];

/** 조회 훅의 기본(성공/빈) 반환값 헬퍼. */
function queryResult<T>(over: Partial<Record<string, unknown>> & { data?: T }) {
  return {
    data: over.data,
    isLoading: false,
    isFetching: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...over,
  };
}

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <InfluxdbManagementPanel agentName="influx-a" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  // 기본: measurement 는 빈 목록.
  useMeasurementsMock.mockReturnValue(queryResult<string[]>({ data: [] }));
});

describe('버킷 목록 렌더', () => {
  it('버킷 이름/보관기간/ID 를 표시한다', () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: BUCKETS }));
    renderPanel();

    expect(screen.getByText('sensors')).toBeInTheDocument();
    expect(screen.getByText('logs')).toBeInTheDocument();
    expect(screen.getByText('b1')).toBeInTheDocument();
    // retention 0 → 무제한 라벨 키
    expect(
      screen.getAllByText('agents.detail.influxdb.retentionInfinite').length,
    ).toBeGreaterThan(0);
    // retention 86400 → 1d
    expect(screen.getByText('1d')).toBeInTheDocument();
  });

  it('버킷이 없으면 빈 상태를 표시한다', () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: [] }));
    renderPanel();
    expect(
      screen.getByText('agents.detail.influxdb.noBuckets'),
    ).toBeInTheDocument();
  });
});

describe('v3 미지원 처리', () => {
  it('501 APIError 시 미지원 안내를 표시하고 생성 폼을 숨긴다', () => {
    useBucketsMock.mockReturnValue(
      queryResult<InfluxBucket[]>({
        data: undefined,
        isError: true,
        error: new APIError('NOT_IMPLEMENTED', 'v3 unsupported', 501),
      }),
    );
    renderPanel();

    expect(
      screen.getByText('agents.detail.influxdb.unsupportedVersion'),
    ).toBeInTheDocument();
    // 생성 폼(버킷 이름 입력)은 숨겨진다.
    expect(
      screen.queryByLabelText('agents.detail.influxdb.bucketName'),
    ).not.toBeInTheDocument();
  });
});

describe('버킷 생성', () => {
  it('이름 입력 후 생성 버튼으로 mutate 를 호출한다', () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: BUCKETS }));
    renderPanel();

    const nameInput = screen.getByLabelText(
      'agents.detail.influxdb.bucketName',
    );
    fireEvent.change(nameInput, { target: { value: 'metrics' } });
    fireEvent.click(screen.getByText('agents.detail.influxdb.create'));

    expect(createMutate).toHaveBeenCalledTimes(1);
    expect(createMutate.mock.calls[0]?.[0]).toEqual({
      name: 'metrics',
      retentionSeconds: 0,
    });
  });
});

describe('버킷 삭제 (확인 모달)', () => {
  it('삭제 버튼 → 확인 다이얼로그 → 확인 시 mutateAsync 호출', async () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: BUCKETS }));
    deleteBucketMutateAsync.mockResolvedValue(undefined);
    renderPanel();

    // sensors 행(첫 행)의 삭제 버튼 클릭.
    // i18n 목킹으로 aria-label 이 키 그대로라 모든 버킷 행이 동일 라벨을 가지므로
    // 첫 번째(sensors)를 선택한다.
    fireEvent.click(
      screen.getAllByLabelText(
        'agents.detail.influxdb.deleteBucketAriaLabel',
      )[0]!,
    );

    // 확인 다이얼로그 노출.
    const dialog = screen.getByRole('dialog');
    expect(
      within(dialog).getByText(
        'agents.detail.influxdb.deleteBucketConfirmTitle',
      ),
    ).toBeInTheDocument();

    // 확인 버튼 클릭 → mutateAsync 호출.
    fireEvent.click(
      within(dialog).getByText('agents.detail.influxdb.delete'),
    );

    await waitFor(() =>
      expect(deleteBucketMutateAsync).toHaveBeenCalledWith('sensors'),
    );
  });
});

describe('버킷 초기화 (강한 확인 모달)', () => {
  it('버킷 이름을 정확히 입력해야 초기화가 실행된다', async () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: BUCKETS }));
    truncateMutateAsync.mockResolvedValue(undefined);
    renderPanel();

    // sensors 행(첫 행)의 초기화 버튼 클릭 (동일 라벨 다중 → 첫 번째 선택).
    fireEvent.click(
      screen.getAllByLabelText(
        'agents.detail.influxdb.truncateAriaLabel',
      )[0]!,
    );

    const dialog = screen.getByRole('dialog');
    const confirmButton = within(dialog).getByText(
      'agents.detail.influxdb.truncate',
    );

    // 이름 미입력 상태: 확인 버튼 비활성화 → 클릭해도 mutate 안 됨.
    expect(confirmButton).toBeDisabled();

    // 잘못된 이름 입력: 여전히 비활성화.
    const confirmInput = within(dialog).getByLabelText(
      'agents.detail.influxdb.truncateConfirmPrompt',
    );
    fireEvent.change(confirmInput, { target: { value: 'wrong' } });
    expect(confirmButton).toBeDisabled();
    expect(
      within(dialog).getByText(
        'agents.detail.influxdb.truncateConfirmMismatch',
      ),
    ).toBeInTheDocument();

    // 정확한 이름 입력: 활성화되고 클릭 시 mutateAsync 호출.
    fireEvent.change(confirmInput, { target: { value: 'sensors' } });
    expect(confirmButton).not.toBeDisabled();
    fireEvent.click(confirmButton);

    await waitFor(() =>
      expect(truncateMutateAsync).toHaveBeenCalledWith('sensors'),
    );
  });
});

describe('measurement 목록 + 삭제', () => {
  it('버킷 선택 시 measurement 를 표시하고 삭제 확인 후 mutateAsync 호출', async () => {
    useBucketsMock.mockReturnValue(queryResult<InfluxBucket[]>({ data: BUCKETS }));
    useMeasurementsMock.mockReturnValue(
      queryResult<string[]>({ data: ['cpu', 'mem'] }),
    );
    deleteMeasurementMutateAsync.mockResolvedValue(undefined);
    renderPanel();

    // 버킷 행 클릭 → measurement 섹션 노출.
    fireEvent.click(screen.getByText('sensors'));

    expect(screen.getByText('cpu')).toBeInTheDocument();
    expect(screen.getByText('mem')).toBeInTheDocument();

    // cpu measurement 삭제 버튼 클릭 → 확인 다이얼로그.
    // i18n 목킹으로 aria-label 이 키 그대로라 두 measurement 가 동일 라벨을 가지므로
    // 첫 번째(cpu) 버튼을 선택한다.
    const measurementDeleteButtons = screen.getAllByLabelText(
      'agents.detail.influxdb.deleteMeasurementAriaLabel',
    );
    fireEvent.click(measurementDeleteButtons[0]!);

    const dialog = screen.getByRole('dialog');
    fireEvent.click(
      within(dialog).getByText('agents.detail.influxdb.delete'),
    );

    await waitFor(() =>
      expect(deleteMeasurementMutateAsync).toHaveBeenCalledWith({
        bucket: 'sensors',
        measurement: 'cpu',
      }),
    );
  });
});
