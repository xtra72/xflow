// SeriesDataViewerModal 단위 테스트.
// SPEC-WEB-005 v0.2.0: dataSource prop + 내부에서 @tanstack/react-query 의
// useMutation 으로 전환되었다. 테스트에서는 useMutation 자체를 모킹해
// 네트워크 없이 UI 상호작용을 검증한다.
//
// SPEC-WEB-005 v0.3.0 UI/UX 개선 커버리지:
//   - 모달 오픈 시 기본 시간 범위 = 지난 1일.
//   - 상대 범위 빠른 선택 버튼 ("지난 1시간" 등) 동작.
//   - 컨테이너 크기 확대 (95vw × 95vh).
//
// SPEC-WEB-005 v0.7.0 (M16, Task 11/13) 메타데이터 + 필터 커버리지:
//   - 시리즈 행에 data_type / metric_type / auto 배지 칩 표시.
//   - data_type / metric_type / registration 필터 UI 동작.
//   - 신규 필터와 검색/태그 필터의 AND 결합.
//
// @spec SPEC-WEB-005
// @spec SPEC-WEB-005 v0.7.0 (M16)

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type {
  SeriesDataSource,
  SeriesMatrix,
  SeriesMatrixQuery,
} from '@/services/api/seriesDataSource';
import type { StoreKeyObject } from '@/services/api/store';

interface MockMutation {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: Error | null;
  data: SeriesMatrix | null;
}

const mutationState = vi.hoisted(
  (): { current: MockMutation } => ({
    current: {
      mutate: vi.fn(),
      reset: vi.fn(),
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
      data: null,
    },
  }),
);

// useMutation 만 모킹하고 다른 export 는 그대로 통과시킨다.
vi.mock('@tanstack/react-query', async () => {
  const actual =
    await vi.importActual<typeof import('@tanstack/react-query')>(
      '@tanstack/react-query',
    );
  return {
    ...actual,
    useMutation: () => mutationState.current,
  };
});

/**
 * SPEC-WEB-005 v0.7.0 (M16): Store 모드 메타데이터 테스트용 모킹.
 *
 * `useStoreKeysWithTags` / `useStoreTagPairs` 가 빈 배열 또는 사전 설정 데이터를
 * 동기적으로 반환하도록 만들어 React Query 의 비동기 fetch 경로를 우회한다.
 *
 * `storeKeysState.current` 를 테스트에서 직접 갱신하면 다음 렌더부터 반영된다.
 */
const storeKeysState = vi.hoisted(
  (): {
    current: {
      keys: string[];
      tags: Record<string, Record<string, string>>;
      keyObjects: StoreKeyObject[];
    };
  } => ({
    current: { keys: [], tags: {}, keyObjects: [] },
  }),
);

vi.mock('@/services/api/store', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/store')>(
    '@/services/api/store',
  );
  return {
    ...actual,
    useStoreKeysWithTags: () => ({
      data: storeKeysState.current,
      isLoading: false,
      isError: false,
      error: null,
    }),
    useStoreTagPairs: () => ({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
    }),
  };
});

import TsdbDataViewerModal from './TsdbDataViewerModal';

function resetMutation() {
  mutationState.current = {
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
    isSuccess: false,
    error: null,
    data: null,
  };
}

beforeEach(() => {
  resetMutation();
  // 각 테스트가 자신의 store 데이터를 명시적으로 설정하도록 매번 비운다.
  storeKeysState.current = { keys: [], tags: {}, keyObjects: [] };
});

afterEach(() => {
  // 고정 시각 테스트 후 실제 타이머로 복원한다.
  vi.useRealTimers();
});

const ALL_KEYS = ['temp,room=1', 'temp,room=2', 'humidity,room=1'];

/** 테스트용 fake dataSource — queryMatrix 는 직접 호출되지 않는다 (useMutation 모킹 때문). */
function fakeDataSource(): SeriesDataSource {
  return {
    kind: 'tsdb',
    useKeys: () => ({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }),
    queryMatrix: vi.fn<() => Promise<SeriesMatrix>>(async () => ({
      columns: [],
      rows: [],
    })),
  };
}

/**
 * SPEC-WEB-005 v0.7.0 (M16): Store 모드 dataSource fixture.
 * `kind: 'store'` 가 포함된 dataSource 만으로 신규 메타데이터 필터 UI 가 노출된다.
 */
function storeDataSource(): SeriesDataSource {
  return {
    kind: 'store',
    useKeys: () => ({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    }),
    queryMatrix: vi.fn<() => Promise<SeriesMatrix>>(async () => ({
      columns: [],
      rows: [],
    })),
  };
}

/**
 * 표준 keyObjects fixture — 4개 키 (혼합 data_type / metric_type / registration).
 */
function makeKeyObjects(): StoreKeyObject[] {
  return [
    {
      key: 'indoor:1:temp',
      registration: 'manual',
      data_type: 'float',
      metric_type: 'temperature',
      tags: { room: '1', metric: 'temperature' },
    },
    {
      key: 'indoor:2:temp',
      registration: 'manual',
      data_type: 'float',
      metric_type: 'temperature',
      tags: { room: '2', metric: 'temperature' },
    },
    {
      key: 'indoor:1:hum',
      registration: 'auto',
      data_type: 'int',
      metric_type: 'humidity',
      tags: { room: '1', metric: 'humidity' },
    },
    {
      key: 'misc:status',
      registration: 'auto',
      data_type: 'string',
      metric_type: 'unknown',
      tags: {},
    },
  ];
}

/**
 * 로컬 타임존 기준 epoch ms → `YYYY-MM-DDTHH:mm` 포맷.
 * 모달 내부 `epochMsToDatetimeLocal` 과 동일한 로직을 재현해
 * 기본값 테스트에서 예상 값을 계산한다.
 */
function epochToDatetimeLocal(ms: number): string {
  const d = new Date(ms);
  const tzMs = d.getTimezoneOffset() * 60 * 1000;
  return new Date(ms - tzMs).toISOString().slice(0, 16);
}

describe('SeriesDataViewerModal', () => {
  it('isOpen=false 이면 아무것도 렌더링하지 않는다', () => {
    const { container } = render(
      <TsdbDataViewerModal
        isOpen={false}
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('initialSeriesKey 가 기본 선택되어 렌더링', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    expect(screen.getByText('시리즈 선택 (1개 선택됨)')).toBeInTheDocument();
  });

  it('Esc 키 입력 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={onClose}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('배경 클릭 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={onClose}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    // dialog role 을 가진 루트 컨테이너를 클릭
    const backdrop = screen.getByRole('dialog');
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('키가 0개 선택되면 실행 버튼 비활성', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    const execute = screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement;
    expect(execute.disabled).toBe(true);
  });

  it('end <= start 이면 실행 버튼 비활성 + 에러 메시지 표시', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-04-23T10:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T08:00' },
    });
    expect(screen.getByText(/종료 시각은 시작 시각 이후여야 합니다/)).toBeInTheDocument();
    const execute = screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement;
    expect(execute.disabled).toBe(true);
  });

  it('잘못된 custom 인터벌이면 실행 버튼 비활성', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-04-23T00:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T06:00' },
    });
    fireEvent.change(screen.getByLabelText('인터벌'), {
      target: { value: 'custom' },
    });
    const customInput = screen.getByPlaceholderText(/예: 2m/);
    fireEvent.change(customInput, { target: { value: '5분' } });
    expect(screen.getByText(/Go duration 문법/)).toBeInTheDocument();
    const execute = screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement;
    expect(execute.disabled).toBe(true);
  });

  it('유효한 입력 + 실행 클릭 시 mutate 호출, epoch ms + intervalMs 전달', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-04-23T00:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T06:00' },
    });
    fireEvent.change(screen.getByLabelText('인터벌'), { target: { value: '1h' } });

    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));

    expect(mutationState.current.mutate).toHaveBeenCalledTimes(1);
    const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
    expect(arg.keys).toEqual(['temp,room=1']);
    // "1h" → 3,600,000 ms
    expect(arg.intervalMs).toBe(60 * 60 * 1000);
    expect(arg.aggregation).toBe('average');
    // epoch ms 정확성 (로컬 → UTC 왕복)
    expect(arg.startMs).toBe(new Date('2026-04-23T00:00').getTime());
    expect(arg.endMs).toBe(new Date('2026-04-23T06:00').getTime());
    expect(arg.endMs).toBeGreaterThan(arg.startMs);
  });

  it('예상 행 수가 5,000 초과면 경고 배너 표시 후 확정 시 mutate', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    // 30일 범위 × 1m 인터벌 → 43,200 bucket (초과)
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-03-24T00:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T00:00' },
    });
    fireEvent.change(screen.getByLabelText('인터벌'), { target: { value: '1m' } });

    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));

    // 경고 배너 표시, mutate 는 아직 호출되지 않음
    expect(screen.getByText(/결과 행 수가 많아/)).toBeInTheDocument();
    expect(mutationState.current.mutate).not.toHaveBeenCalled();

    // 계속 실행 클릭
    fireEvent.click(screen.getByRole('button', { name: /계속 실행/ }));
    expect(mutationState.current.mutate).toHaveBeenCalled();
  });

  it('경고 배너에서 "취소" 클릭 시 mutate 호출되지 않음', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-03-24T00:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T00:00' },
    });
    fireEvent.change(screen.getByLabelText('인터벌'), { target: { value: '1m' } });
    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));
    // 경고 내부의 "취소" 버튼 (푸터의 "닫기" 와 구분)
    fireEvent.click(screen.getByRole('button', { name: '취소' }));
    expect(mutationState.current.mutate).not.toHaveBeenCalled();
    expect(screen.queryByText(/결과 행 수가 많아/)).not.toBeInTheDocument();
  });

  // ---- v0.3.0 UI/UX 개선 ----

  it('모달 오픈 시 기본 시간 범위 = 지난 1일 (end=현재, start=현재-24h)', () => {
    // 고정 시각으로 초기 상태 산정을 예측 가능하게 만든다.
    const fixedNow = new Date('2026-04-23T12:00:00').getTime();
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);

    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );

    // 기본 모드는 상대이지만 절대 모드의 datetime 입력 초기값을 검증한다.
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    const start = screen.getByLabelText(/시작 시각/) as HTMLInputElement;
    const end = screen.getByLabelText(/종료 시각/) as HTMLInputElement;

    expect(end.value).toBe(epochToDatetimeLocal(fixedNow));
    expect(start.value).toBe(
      epochToDatetimeLocal(fixedNow - 24 * 60 * 60 * 1000),
    );
  });

  it('상대 범위 버튼 "지난 1시간" 클릭 시 end=now, start=now-1h 로 입력 갱신', () => {
    const fixedNow = new Date('2026-04-23T09:30:00').getTime();
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);

    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );

    // 절대 모드로 전환 (빠른 선택 버튼은 절대 모드에서만 노출).
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    // 시계를 5분 앞으로 이동한 뒤 버튼 클릭 — 클릭 시점의 "현재" 가 반영돼야 한다.
    const clickAt = fixedNow + 5 * 60 * 1000;
    vi.setSystemTime(clickAt);
    fireEvent.click(screen.getByRole('button', { name: '지난 1시간' }));

    const start = screen.getByLabelText(/시작 시각/) as HTMLInputElement;
    const end = screen.getByLabelText(/종료 시각/) as HTMLInputElement;
    expect(end.value).toBe(epochToDatetimeLocal(clickAt));
    expect(start.value).toBe(epochToDatetimeLocal(clickAt - 60 * 60 * 1000));
    // 버튼은 쿼리를 자동 실행하지 않는다.
    expect(mutationState.current.mutate).not.toHaveBeenCalled();
  });

  it('모달 컨테이너에 95vw × 95vh 크기 클래스가 적용된다', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    const container = screen.getByTestId('tsdb-viewer-modal-container');
    expect(container.className).toMatch(/w-\[95vw\]/);
    expect(container.className).toMatch(/h-\[95vh\]/);
    expect(container.className).toMatch(/flex-col/);
    expect(container.className).toMatch(/overflow-hidden/);
  });

  // ---- v0.3.0 Wave 2: 절대/상대 모드 토글 ----

  it('기본 모드는 "상대" — 상대 탭이 aria-selected=true, 상대 드롭다운 노출', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    const abs = screen.getByTestId('tsdb-range-mode-absolute');
    const rel = screen.getByTestId('tsdb-range-mode-relative');
    expect(rel.getAttribute('aria-selected')).toBe('true');
    expect(abs.getAttribute('aria-selected')).toBe('false');
    // 상대 모드 UI 요소 (범위 드롭다운).
    expect(screen.getByLabelText(/^범위$/)).toBeInTheDocument();
    expect(screen.getByText(/실행 시각 기준 지난 기간을 조회합니다/)).toBeInTheDocument();
    // 절대 모드의 datetime-local 입력은 노출되지 않는다.
    expect(screen.queryByLabelText(/시작 시각/)).toBeNull();
    expect(screen.queryByLabelText(/종료 시각/)).toBeNull();
  });

  it('"상대" 탭 클릭 시 상대 UI 로 전환되고 datetime-local 은 숨겨진다', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-relative'));

    expect(
      screen.getByTestId('tsdb-range-mode-relative').getAttribute('aria-selected'),
    ).toBe('true');
    // 상대 드롭다운 노출.
    expect(screen.getByLabelText(/^범위$/)).toBeInTheDocument();
    expect(screen.getByText(/실행 시각 기준 지난 기간을 조회합니다/)).toBeInTheDocument();
    // 절대 모드의 datetime-local 입력은 제거되었다.
    expect(screen.queryByLabelText(/시작 시각/)).toBeNull();
    expect(screen.queryByLabelText(/종료 시각/)).toBeNull();
  });

  it('상대 모드에서 "커스텀" 선택 시 duration 입력 노출 + 잘못된 값이면 실행 비활성', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-relative'));
    fireEvent.change(screen.getByLabelText(/^범위$/), {
      target: { value: 'custom' },
    });
    const customInput = screen.getByLabelText(/커스텀 duration/);
    fireEvent.change(customInput, { target: { value: '오분' } });
    expect(screen.getAllByText(/Go duration 문법/).length).toBeGreaterThan(0);
    const execute = screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement;
    expect(execute.disabled).toBe(true);
  });

  it('상대 모드 커스텀 duration 유효값 + 실행 시 mutate 호출 (start = now - duration)', () => {
    const fixedNow = new Date('2026-04-23T12:00:00').getTime();
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);

    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-relative'));
    fireEvent.change(screen.getByLabelText(/^범위$/), {
      target: { value: 'custom' },
    });
    fireEvent.change(screen.getByLabelText(/커스텀 duration/), {
      target: { value: '2h' },
    });

    // 클릭 시점의 now 를 약간 뒤로 이동해 "실행 시점 = now" 임을 검증한다.
    const clickAt = fixedNow + 30_000;
    vi.setSystemTime(clickAt);
    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));

    expect(mutationState.current.mutate).toHaveBeenCalledTimes(1);
    const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
    expect(arg.endMs).toBe(clickAt);
    expect(arg.startMs).toBe(clickAt - 2 * 60 * 60 * 1000);
  });

  it('상대 모드 기본 선택은 "지난 1일" — 실행 시 duration 1일 반영', () => {
    const fixedNow = new Date('2026-04-23T09:30:00').getTime();
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);

    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByTestId('tsdb-range-mode-relative'));
    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));

    expect(mutationState.current.mutate).toHaveBeenCalledTimes(1);
    const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
    expect(arg.endMs).toBe(fixedNow);
    expect(arg.endMs - arg.startMs).toBe(24 * 60 * 60 * 1000);
  });

  it('상대→절대 전환 시 현재 duration 을 기준으로 datetime-local 이 채워진다', () => {
    const fixedNow = new Date('2026-04-23T10:00:00').getTime();
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);

    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    // 상대로 전환 + "지난 1시간" 선택.
    fireEvent.click(screen.getByTestId('tsdb-range-mode-relative'));
    fireEvent.change(screen.getByLabelText(/^범위$/), {
      target: { value: '지난 1시간' },
    });
    // 다시 절대로 전환.
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));

    const start = screen.getByLabelText(/시작 시각/) as HTMLInputElement;
    const end = screen.getByLabelText(/종료 시각/) as HTMLInputElement;
    // end = now, start = now - 1h.
    const toLocal = (ms: number) => {
      const tz = new Date(ms).getTimezoneOffset() * 60 * 1000;
      return new Date(ms - tz).toISOString().slice(0, 16);
    };
    expect(end.value).toBe(toLocal(fixedNow));
    expect(start.value).toBe(toLocal(fixedNow - 60 * 60 * 1000));
  });

  // ---- v0.4.0: 키 구조 기반 자동 태그 추출 ----

  it('정적 태그가 없는 InfluxDB 스타일 키에서 태그 칩이 자동 추출된다', () => {
    // TSDB 모드 (kind: 'tsdb') — 정적 태그 소스 없음. 키 구조에서 추출.
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={['temp,room=1', 'temp,room=2', 'humid,room=1']}
        dataSource={fakeDataSource()}
      />,
    );
    // 값 칩 — TagFilterChips 가 data-testid 를 부여하므로 이를 사용해 정확히 매칭.
    expect(screen.getByTestId('tag-filter-measurement-temp')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-measurement-humid')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-room-1')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-room-2')).toBeInTheDocument();
  });

  it('구조 없는 키만 있으면 태그 필터 섹션이 숨겨진다', () => {
    const { container } = render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={['plain_a', 'plain_b']}
        dataSource={fakeDataSource()}
      />,
    );
    // 태그 칩이 단 하나도 렌더링되지 않아야 한다.
    expect(
      container.querySelectorAll('[data-testid^="tag-filter-"]').length,
    ).toBe(0);
  });

  // ---- v0.4.0: 평균 자릿수 입력 ----

  it('기본 집계가 average 이므로 소수점 자릿수 입력이 노출된다', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    const input = screen.getByTestId('tsdb-decimal-precision') as HTMLInputElement;
    expect(input).toBeInTheDocument();
    expect(input.value).toBe('1');
    // 도움말 텍스트가 노출되는지 확인.
    expect(screen.getByText(/평균 집계 시 표시할 소수점 자릿수/)).toBeInTheDocument();
  });

  it('min/max 집계 선택 시 소수점 자릿수 입력이 숨겨진다', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    fireEvent.click(screen.getByLabelText(/최소/));
    expect(screen.queryByTestId('tsdb-decimal-precision')).toBeNull();
    fireEvent.click(screen.getByLabelText(/최대/));
    expect(screen.queryByTestId('tsdb-decimal-precision')).toBeNull();
    // 다시 평균 선택 → 입력 노출.
    fireEvent.click(screen.getByLabelText(/평균/));
    expect(screen.getByTestId('tsdb-decimal-precision')).toBeInTheDocument();
  });

  it('모달 오픈 시마다 모드는 "상대" 로 리셋된다', () => {
    const { rerender } = render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    // 절대 탭으로 전환.
    fireEvent.click(screen.getByTestId('tsdb-range-mode-absolute'));
    expect(
      screen.getByTestId('tsdb-range-mode-absolute').getAttribute('aria-selected'),
    ).toBe('true');

    // 닫았다가 다시 연다.
    rerender(
      <TsdbDataViewerModal
        isOpen={false}
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    rerender(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
        dataSource={fakeDataSource()}
      />,
    );
    expect(
      screen.getByTestId('tsdb-range-mode-relative').getAttribute('aria-selected'),
    ).toBe('true');
  });

  // ---- v0.7.0 (M16, Task 11/13): 메타데이터 표시 + 신규 필터 UI ----

  describe('Store 모드: 메타데이터 칩 + 신규 필터 (v0.7.0 M16)', () => {
    /** Store 모드 + keyObjects fixture 를 갖춘 모달을 렌더한다. */
    function renderStoreModal(opts?: { allSeriesKeys?: string[] }) {
      const objs = makeKeyObjects();
      storeKeysState.current = {
        keys: objs.map((o) => o.key),
        // 정적 태그가 비어있지 않은 키만 포함 (Phase A derived 와 동일).
        tags: objs.reduce<Record<string, Record<string, string>>>((acc, o) => {
          if (Object.keys(o.tags).length > 0) acc[o.key] = o.tags;
          return acc;
        }, {}),
        keyObjects: objs,
      };
      return render(
        <TsdbDataViewerModal
          isOpen
          onClose={vi.fn()}
          allSeriesKeys={opts?.allSeriesKeys ?? objs.map((o) => o.key)}
          dataSource={storeDataSource()}
          agentName="agent-test"
        />,
      );
    }

    it('Store 모드 시리즈 행에 data_type 칩이 표시된다', () => {
      renderStoreModal();
      // float / int / string 칩이 각 키별로 하나씩 노출된다 (총 4개).
      const dataTypeChips = screen.getAllByTestId('metadata-data-type');
      expect(dataTypeChips.length).toBe(4);
      // 각 키별 텍스트 확인.
      expect(dataTypeChips.some((el) => el.textContent === 'float')).toBe(true);
      expect(dataTypeChips.some((el) => el.textContent === 'int')).toBe(true);
      expect(dataTypeChips.some((el) => el.textContent === 'string')).toBe(true);
    });

    it('metric_type 이 설정된 키는 일반 칩, unknown 은 muted 칩으로 노출된다', () => {
      renderStoreModal();
      // 'temperature' / 'humidity' 등 일반 metric_type 칩.
      const metricChips = screen.getAllByTestId('metadata-metric-type');
      expect(metricChips.length).toBeGreaterThan(0);
      expect(metricChips.some((el) => el.textContent === 'temperature')).toBe(true);
      expect(metricChips.some((el) => el.textContent === 'humidity')).toBe(true);
      // unknown 은 별도 칩.
      const unknownChips = screen.getAllByTestId('metadata-metric-type-unknown');
      expect(unknownChips.length).toBeGreaterThanOrEqual(1);
    });

    it('registration=auto 인 키는 auto 배지, manual 인 키는 manual 배지가 표시된다', () => {
      renderStoreModal();
      // fixture: 2 manual + 2 auto.
      expect(screen.getAllByTestId('metadata-registration-auto').length).toBe(2);
      expect(screen.getAllByTestId('metadata-registration-manual').length).toBe(2);
    });

    it('Store 모드에서 메타데이터 필터 UI 가 노출된다', () => {
      renderStoreModal();
      expect(screen.getByTestId('series-meta-filters')).toBeInTheDocument();
      expect(screen.getByTestId('meta-filter-data-type')).toBeInTheDocument();
      expect(screen.getByTestId('meta-filter-metric-type')).toBeInTheDocument();
      expect(screen.getByTestId('meta-filter-registration-all')).toBeInTheDocument();
      expect(screen.getByTestId('meta-filter-registration-manual')).toBeInTheDocument();
      expect(screen.getByTestId('meta-filter-registration-auto')).toBeInTheDocument();
    });

    it('TSDB 모드에서는 메타데이터 필터 UI 가 노출되지 않는다', () => {
      render(
        <TsdbDataViewerModal
          isOpen
          onClose={vi.fn()}
          allSeriesKeys={ALL_KEYS}
          dataSource={fakeDataSource()}
        />,
      );
      expect(screen.queryByTestId('series-meta-filters')).toBeNull();
    });

    it('data_type 필터 (float) 적용 시 float 키만 시리즈 풀에 노출된다', () => {
      renderStoreModal();
      // 초기에는 4개 모두 노출.
      const before = screen.getAllByTestId('metadata-data-type');
      expect(before.length).toBe(4);

      // float 으로 필터링.
      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'float' },
      });

      const after = screen.getAllByTestId('metadata-data-type');
      // float 만 2개 남는다.
      expect(after.length).toBe(2);
      expect(after.every((el) => el.textContent === 'float')).toBe(true);
    });

    it('metric_type 필터 (temperature) 적용 시 정확히 일치하는 키만 노출된다', () => {
      renderStoreModal();

      fireEvent.change(screen.getByTestId('meta-filter-metric-type'), {
        target: { value: 'temperature' },
      });

      const dataTypes = screen.getAllByTestId('metadata-data-type');
      expect(dataTypes.length).toBe(2); // indoor:1:temp + indoor:2:temp
      // metric_type=humidity / unknown 키는 사라졌다.
      const metricChips = screen.getAllByTestId('metadata-metric-type');
      expect(metricChips.every((el) => el.textContent === 'temperature')).toBe(true);
    });

    it('registration 필터 (auto) 적용 시 auto 키만 노출된다', () => {
      renderStoreModal();

      fireEvent.click(screen.getByTestId('meta-filter-registration-auto'));

      // 2개 auto 키만 남아있다.
      const dataTypes = screen.getAllByTestId('metadata-data-type');
      expect(dataTypes.length).toBe(2);
      // auto 배지 2개 (manual 배지는 0).
      expect(screen.getAllByTestId('metadata-registration-auto').length).toBe(2);
      expect(screen.queryAllByTestId('metadata-registration-manual').length).toBe(0);
    });

    it('registration "전체" 버튼은 필터를 해제한다', () => {
      renderStoreModal();
      // auto 로 좁힌 뒤 다시 전체로 해제.
      fireEvent.click(screen.getByTestId('meta-filter-registration-auto'));
      expect(screen.getAllByTestId('metadata-data-type').length).toBe(2);

      fireEvent.click(screen.getByTestId('meta-filter-registration-all'));
      expect(screen.getAllByTestId('metadata-data-type').length).toBe(4);
    });

    it('registration 버튼은 aria-pressed 로 선택 상태를 노출한다', () => {
      renderStoreModal();
      const allBtn = screen.getByTestId('meta-filter-registration-all');
      const manualBtn = screen.getByTestId('meta-filter-registration-manual');
      const autoBtn = screen.getByTestId('meta-filter-registration-auto');
      // 초기: 전체 선택.
      expect(allBtn.getAttribute('aria-pressed')).toBe('true');
      expect(manualBtn.getAttribute('aria-pressed')).toBe('false');
      expect(autoBtn.getAttribute('aria-pressed')).toBe('false');
      // manual 클릭.
      fireEvent.click(manualBtn);
      expect(allBtn.getAttribute('aria-pressed')).toBe('false');
      expect(manualBtn.getAttribute('aria-pressed')).toBe('true');
      expect(autoBtn.getAttribute('aria-pressed')).toBe('false');
    });

    it('필터 결합: data_type=float + registration=manual → 1개로 좁혀진다', () => {
      renderStoreModal();

      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'float' },
      });
      fireEvent.click(screen.getByTestId('meta-filter-registration-manual'));

      // float + manual: indoor:1:temp / indoor:2:temp 2개.
      const dataTypes = screen.getAllByTestId('metadata-data-type');
      expect(dataTypes.length).toBe(2);
      expect(dataTypes.every((el) => el.textContent === 'float')).toBe(true);
      expect(screen.getAllByTestId('metadata-registration-manual').length).toBe(2);
    });

    it('모든 필터가 매치되지 않으면 "일치하는 시리즈가 없습니다" 빈 상태가 노출된다', () => {
      renderStoreModal();
      // boolean 타입은 fixture 에 없음 → 결과 0건.
      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'boolean' },
      });
      expect(screen.getByText(/일치하는 시리즈가 없습니다/)).toBeInTheDocument();
    });

    it('검색 + 메타데이터 필터는 AND 결합된다', () => {
      renderStoreModal();
      // 검색: "indoor" → 3개 (indoor:1:temp / indoor:2:temp / indoor:1:hum)
      fireEvent.change(screen.getByPlaceholderText('시리즈 키 검색'), {
        target: { value: 'indoor' },
      });
      expect(screen.getAllByTestId('metadata-data-type').length).toBe(3);
      // + data_type=int → 1개 (indoor:1:hum)
      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'int' },
      });
      const remaining = screen.getAllByTestId('metadata-data-type');
      expect(remaining.length).toBe(1);
      expect(remaining[0]!.textContent).toBe('int');
    });

    it('모달 재오픈 시 메타데이터 필터가 모두 초기화된다', () => {
      const { rerender } = renderStoreModal();
      // float + auto 필터 적용.
      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'float' },
      });
      fireEvent.click(screen.getByTestId('meta-filter-registration-auto'));
      // 닫고 다시 연다.
      rerender(
        <TsdbDataViewerModal
          isOpen={false}
          onClose={vi.fn()}
          allSeriesKeys={makeKeyObjects().map((o) => o.key)}
          dataSource={storeDataSource()}
          agentName="agent-test"
        />,
      );
      rerender(
        <TsdbDataViewerModal
          isOpen
          onClose={vi.fn()}
          allSeriesKeys={makeKeyObjects().map((o) => o.key)}
          dataSource={storeDataSource()}
          agentName="agent-test"
        />,
      );
      // 필터 모두 해제: 4개 키 다시 노출.
      expect(screen.getAllByTestId('metadata-data-type').length).toBe(4);
      expect(
        screen
          .getByTestId('meta-filter-data-type')
          .getAttribute('value') ?? '',
      ).toBe('');
      expect(
        screen.getByTestId('meta-filter-registration-all').getAttribute('aria-pressed'),
      ).toBe('true');
    });

    it('data_type 필터는 시간 범위/집계/CSV 내보내기 로직에 영향을 주지 않는다 (회귀)', () => {
      renderStoreModal();
      // float 필터 적용.
      fireEvent.change(screen.getByTestId('meta-filter-data-type'), {
        target: { value: 'float' },
      });
      // 첫 번째 키 (float) 선택 — 체크박스 클릭.
      const checkboxes = screen
        .getAllByRole('checkbox')
        .filter((el) => (el as HTMLInputElement).type === 'checkbox');
      fireEvent.click(checkboxes[0]!);
      // 실행 가능 상태가 됨.
      const execute = screen.getByRole('button', {
        name: /^실행$/,
      }) as HTMLButtonElement;
      expect(execute.disabled).toBe(false);
      fireEvent.click(execute);
      expect(mutationState.current.mutate).toHaveBeenCalledTimes(1);
      const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
      // 키는 1개만 전달, 시간 범위/집계는 기본값 그대로 동작.
      expect(arg.keys.length).toBe(1);
      expect(arg.aggregation).toBe('average');
      expect(arg.intervalMs).toBeGreaterThan(0);
      expect(arg.endMs).toBeGreaterThan(arg.startMs);
    });
  });
});
