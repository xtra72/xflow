// useSysMetricsSnapshot 테스트.
//
// 훅이 지켜야 할 계약 셋:
//   - 상태 분기가 정확하다 (없음 / 호환 안 됨 / 표본 이전 / 멈춤 / 정상)
//   - 같은 표본을 다시 받아도 rate 기준점을 흔들지 않는다
//   - 에이전트를 바꾸면 기준점을 버린다 (다른 호스트의 카운터를 이어 붙이지 않는다)

import { describe, expect, it, beforeEach, vi } from 'vitest';
import { render, act } from '@testing-library/react';

/** useQuery 가 돌려줄 응답 (테스트가 갈아 끼운다) */
const queryRef = vi.hoisted(() => ({
  current: { data: undefined as unknown, isLoading: false, isError: false },
}));

// 훅은 Provider 부재를 견디려고 `QueryClientContext` / `QueryClient` 를 쓴다
// (inertQueryClient 패턴). 원본을 펼치고 `useQuery` 만 덮어 그 export 들을 살려 둔다.
vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => queryRef.current,
}));

vi.mock('@/services/api/agentService', () => ({
  getAgent: vi.fn(),
}));

import { useSysMetricsSnapshot } from './useSysMetricsSnapshot';

/** 에이전트 응답 픽스처 */
function agentWithSample(collectedAt: number, bytesRecv = 1_000) {
  return {
    state: {
      status: 'running',
      collected_at: collectedAt,
      interval_seconds: 5,
      cpu: { usage_percent: 10 },
      network: { en0: { bytes_recv: bytesRecv } },
      targets: { mountpoints: [], devices: [], interfaces: ['en0'] },
    },
  };
}

/** 훅 결과를 DOM 속성으로 노출하는 관찰자 */
function Probe({ agentId = 'a1' }: { agentId?: string }) {
  const { snapshot, previous, state } = useSysMetricsSnapshot(agentId, 5_000);
  return (
    <div
      data-testid="probe"
      data-state={state}
      data-at={String(snapshot?.collectedAt ?? '')}
      data-prev={String(previous?.collectedAt ?? '')}
    />
  );
}

function probeEl(container: HTMLElement): HTMLElement {
  return container.querySelector('[data-testid="probe"]') as HTMLElement;
}

describe('useSysMetricsSnapshot', () => {
  beforeEach(() => {
    queryRef.current = { data: undefined, isLoading: false, isError: false };
  });

  describe('상태 분기', () => {
    it('에이전트 ID 가 없으면 unavailable', () => {
      const { container } = render(<Probe agentId="" />);
      expect(probeEl(container).dataset.state).toBe('unavailable');
    });

    it('조회 실패는 unavailable', () => {
      queryRef.current = { data: undefined, isLoading: false, isError: true };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('unavailable');
    });

    it('첫 응답 대기는 loading', () => {
      queryRef.current = { data: undefined, isLoading: true, isError: false };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('loading');
    });

    it('state 를 해석할 수 없으면 incompatible', () => {
      // sysmetrics 가 아닌 에이전트에 바인딩된 경우.
      queryRef.current = { data: { state: undefined }, isLoading: false, isError: false };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('incompatible');
    });

    it('표본 이전은 no_sample', () => {
      queryRef.current = {
        data: { state: { status: 'no_sample', interval_seconds: 5 } },
        isLoading: false,
        isError: false,
      };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('no_sample');
    });

    it('멈춘 에이전트는 stopped 이고 마지막 표본은 남는다', () => {
      queryRef.current = {
        data: { state: { ...agentWithSample(1_000).state, status: 'stopped' } },
        isLoading: false,
        isError: false,
      };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('stopped');
      // 값을 지우면 "멈췄다"와 "0 이 되었다"가 구별되지 않는다.
      expect(probeEl(container).dataset.at).toBe('1000');
    });

    it('정상 응답은 ready', () => {
      queryRef.current = { data: agentWithSample(1_000), isLoading: false, isError: false };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.state).toBe('ready');
    });
  });

  describe('rate 기준점', () => {
    it('첫 표본에는 기준점이 없다', () => {
      queryRef.current = { data: agentWithSample(1_000), isLoading: false, isError: false };
      const { container } = render(<Probe />);
      expect(probeEl(container).dataset.prev).toBe('');
    });

    it('새 표본이 오면 직전 표본이 기준점이 된다', () => {
      queryRef.current = { data: agentWithSample(1_000), isLoading: false, isError: false };
      const { container, rerender } = render(<Probe />);

      act(() => {
        queryRef.current = {
          data: agentWithSample(6_000, 2_000),
          isLoading: false,
          isError: false,
        };
        rerender(<Probe />);
      });

      expect(probeEl(container).dataset.at).toBe('6000');
      expect(probeEl(container).dataset.prev).toBe('1000');
    });

    it('같은 표본을 다시 받아도 기준점이 흔들리지 않는다', () => {
      // 패널 폴링 주기가 에이전트 표본 주기보다 짧으면 흔히 일어난다. 여기서
      // 기준점을 갱신하면 경과 0 인 구간이 생겨 rate 가 계단처럼 끊긴다.
      queryRef.current = { data: agentWithSample(1_000), isLoading: false, isError: false };
      const { container, rerender } = render(<Probe />);

      act(() => {
        queryRef.current = { data: agentWithSample(6_000), isLoading: false, isError: false };
        rerender(<Probe />);
      });
      expect(probeEl(container).dataset.prev).toBe('1000');

      // 같은 표본(collected_at 6000)이 한 번 더 온다.
      act(() => {
        queryRef.current = { data: agentWithSample(6_000), isLoading: false, isError: false };
        rerender(<Probe />);
      });
      expect(probeEl(container).dataset.prev).toBe('1000');
      expect(probeEl(container).dataset.at).toBe('6000');
    });

    it('에이전트를 바꾸면 기준점을 버린다', () => {
      // 다른 호스트의 누적 카운터를 이어 붙이면 첫 구간에 거대한 스파이크가 생긴다.
      queryRef.current = { data: agentWithSample(1_000), isLoading: false, isError: false };
      const { container, rerender } = render(<Probe agentId="a1" />);

      act(() => {
        queryRef.current = { data: agentWithSample(6_000), isLoading: false, isError: false };
        rerender(<Probe agentId="a1" />);
      });
      expect(probeEl(container).dataset.prev).toBe('1000');

      act(() => {
        rerender(<Probe agentId="a2" />);
      });
      expect(probeEl(container).dataset.prev).toBe('');
    });
  });
});
