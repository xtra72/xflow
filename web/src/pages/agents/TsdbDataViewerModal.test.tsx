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
//   - 시리즈 행에 data_type / field / auto 배지 칩 표시.
//   - data_type / field / registration 필터 UI 동작.
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
import {
  makeSeriesId,
  SERIES_ID_SEPARATOR,
} from '@/services/api/seriesLabels';

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

// i18n: t() 를 ko.json 키 해석으로 모킹해 한국어 단언을 유지한다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<
    string,
    unknown
  >;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) =>
        o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined,
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({
      t: resolve,
      locale: 'ko' as const,
      setLocale: () => {},
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
 * 표준 keyObjects fixture — 4개 키 (혼합 data_type / field / registration).
 */
function makeKeyObjects(): StoreKeyObject[] {
  return [
    {
      key: 'indoor:1:temp',
      registration: 'manual',
      data_type: 'float',
      field: 'temperature',
      tags: { room: '1', metric: 'temperature' },
    },
    {
      key: 'indoor:2:temp',
      registration: 'manual',
      data_type: 'float',
      field: 'temperature',
      tags: { room: '2', metric: 'temperature' },
    },
    {
      key: 'indoor:1:hum',
      registration: 'auto',
      data_type: 'int',
      field: 'humidity',
      tags: { room: '1', metric: 'humidity' },
    },
    {
      key: 'misc:status',
      registration: 'auto',
      data_type: 'string',
      field: 'unknown',
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

    it('field 이 설정된 키는 일반 칩, unknown 은 muted 칩으로 노출된다', () => {
      renderStoreModal();
      // 'temperature' / 'humidity' 등 일반 field 칩.
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

  });

  // SPEC-STORE-004 (M5): 같은 key 의 metric/tags 별 다중 시리즈를 구분된 행으로 표시.
  describe('다중 시리즈 행 표시 (SPEC-STORE-004 M5)', () => {
    /** 같은 key 'sensor' 가 metric/tags 별 3개 시리즈로 반환되는 fixture. */
    function makeMultiSeriesObjects(): StoreKeyObject[] {
      return [
        {
          key: 'sensor',
          registration: 'manual',
          data_type: 'float',
          field: 'temp',
          tags: { room: '1' },
        },
        {
          key: 'sensor',
          registration: 'manual',
          data_type: 'float',
          field: 'temp',
          tags: { room: '2' },
        },
        {
          key: 'sensor',
          registration: 'auto',
          data_type: 'int',
          field: 'humid',
          tags: { room: '1' },
        },
        // 단일 시리즈 key (구분 행 미표시 대조군).
        {
          key: 'lonely',
          registration: 'manual',
          data_type: 'string',
          field: 'status',
          tags: {},
        },
      ];
    }

    function renderMultiSeriesModal() {
      const objs = makeMultiSeriesObjects();
      // allSeriesKeys 는 key 단위 dedupe 된 풀 (fetchStoreKeys 와 동일 의미).
      const uniqueKeys = [...new Set(objs.map((o) => o.key))];
      storeKeysState.current = {
        keys: uniqueKeys,
        tags: {},
        keyObjects: objs,
      };
      return render(
        <TsdbDataViewerModal
          isOpen
          onClose={vi.fn()}
          allSeriesKeys={uniqueKeys}
          dataSource={storeDataSource()}
          agentName="agent-test"
        />,
      );
    }

    /** 행 체크박스 testid (SeriesID 의 NUL 을 '~' 로 치환한 형식). */
    const sid = (key: string, metric: string, tags: Record<string, string>) =>
      `series-select-${makeSeriesId(key, metric, tags)
        .split(SERIES_ID_SEPARATOR)
        .join('~')}`;

    it('모든 시리즈가 평면 테이블 행으로 노출된다 (저장소 스타일)', () => {
      renderMultiSeriesModal();
      const checkboxes = screen
        .getAllByRole('checkbox')
        .filter((el) => (el as HTMLInputElement).type === 'checkbox');
      // sensor 3개 + lonely 1개 = 4개 행 체크박스 (그룹 헤더 없음).
      expect(checkboxes.length).toBe(4);
      expect(screen.getByTestId(sid('sensor', 'temp', { room: '1' }))).toBeInTheDocument();
      expect(screen.getByTestId(sid('sensor', 'temp', { room: '2' }))).toBeInTheDocument();
      expect(screen.getByTestId(sid('sensor', 'humid', { room: '1' }))).toBeInTheDocument();
      expect(screen.getByTestId(sid('lonely', 'status', {}))).toBeInTheDocument();
    });

    it('단일 시리즈만 선택하면 mutate 에 그 key + 해당 metric/tags 필터가 전달된다 (#2)', () => {
      renderMultiSeriesModal();
      // sensor 의 첫 시리즈(temp, room=1)만 선택.
      fireEvent.click(screen.getByTestId(sid('sensor', 'temp', { room: '1' })));
      const execute = screen.getByRole('button', {
        name: /^실행$/,
      }) as HTMLButtonElement;
      expect(execute.disabled).toBe(false);
      fireEvent.click(execute);
      const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
      // 다중 시리즈 key 이므로 key 1개 + 시리즈 필터로 정확히 한 시리즈만 좁힌다.
      expect(arg.keys).toEqual(['sensor']);
      expect(arg.seriesFilters).toEqual([
        { fieldName: 'temp', tags: { room: '1' } },
      ]);
    });

    it('다중 시리즈에서 두 시리즈 선택 시 같은 key 가 두 번 + 각자 필터로 전달된다 (#2)', () => {
      renderMultiSeriesModal();
      fireEvent.click(screen.getByTestId(sid('sensor', 'temp', { room: '1' })));
      fireEvent.click(screen.getByTestId(sid('sensor', 'humid', { room: '1' })));
      fireEvent.click(
        screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement,
      );
      const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
      expect(arg.keys).toEqual(['sensor', 'sensor']);
      expect(arg.seriesFilters).toEqual([
        { fieldName: 'temp', tags: { room: '1' } },
        { fieldName: 'humid', tags: { room: '1' } },
      ]);
    });

    it('단일 시리즈 key 선택은 필터 없이 key 만 전달한다 (기존 동작 보존)', () => {
      renderMultiSeriesModal();
      fireEvent.click(screen.getByTestId(sid('lonely', 'status', {})));
      fireEvent.click(
        screen.getByRole('button', { name: /^실행$/ }) as HTMLButtonElement,
      );
      const arg = mutationState.current.mutate.mock.calls[0]![0] as SeriesMatrixQuery;
      expect(arg.keys).toEqual(['lonely']);
      // 단일 시리즈는 필터를 전송하지 않는다(undefined → 키 생략).
      expect(arg.seriesFilters).toBeUndefined();
    });
  });
});
