// TsdbDataViewerModal 단위 테스트.
// useTsdbQuery 훅을 vi.mock 으로 교체해 네트워크 없이 UI 상호작용을 검증한다.
//
// @spec SPEC-WEB-005

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

// useTsdbQuery 훅 모킹 — mutation 인터페이스 흉내를 낸다.
interface MockMutation {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: Error | null;
  data: unknown;
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

vi.mock('@/hooks/useTsdb', () => ({
  useTsdbQuery: () => mutationState.current,
}));

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
});

const ALL_KEYS = ['temp,room=1', 'temp,room=2', 'humidity,room=1'];

describe('TsdbDataViewerModal', () => {
  it('isOpen=false 이면 아무것도 렌더링하지 않는다', () => {
    const { container } = render(
      <TsdbDataViewerModal
        isOpen={false}
        onClose={vi.fn()}
        allSeriesKeys={ALL_KEYS}
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
      />,
    );
    expect(screen.getByText('시리즈 선택 (1개 선택됨)')).toBeInTheDocument();
  });

  it('Esc 키 입력 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <TsdbDataViewerModal isOpen onClose={onClose} allSeriesKeys={ALL_KEYS} />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('배경 클릭 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <TsdbDataViewerModal isOpen onClose={onClose} allSeriesKeys={ALL_KEYS} />,
    );
    // dialog role 을 가진 루트 컨테이너를 클릭
    const backdrop = screen.getByRole('dialog');
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('키가 0개 선택되면 실행 버튼 비활성', () => {
    render(
      <TsdbDataViewerModal isOpen onClose={vi.fn()} allSeriesKeys={ALL_KEYS} />,
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
      />,
    );
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
      />,
    );
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

  it('유효한 입력 + 실행 클릭 시 mutate 호출, epoch ms 변환 포함', () => {
    render(
      <TsdbDataViewerModal
        isOpen
        onClose={vi.fn()}
        initialSeriesKey="temp,room=1"
        allSeriesKeys={ALL_KEYS}
      />,
    );
    fireEvent.change(screen.getByLabelText(/시작 시각/), {
      target: { value: '2026-04-23T00:00' },
    });
    fireEvent.change(screen.getByLabelText(/종료 시각/), {
      target: { value: '2026-04-23T06:00' },
    });
    fireEvent.change(screen.getByLabelText('인터벌'), { target: { value: '1h' } });

    fireEvent.click(screen.getByRole('button', { name: /^실행$/ }));

    expect(mutationState.current.mutate).toHaveBeenCalledTimes(1);
    const arg = mutationState.current.mutate.mock.calls[0]![0] as {
      keys: string[];
      startMs: number;
      endMs: number;
      interval: string;
      aggregation: string;
    };
    expect(arg.keys).toEqual(['temp,room=1']);
    expect(arg.interval).toBe('1h');
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
      />,
    );
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
      />,
    );
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
});
