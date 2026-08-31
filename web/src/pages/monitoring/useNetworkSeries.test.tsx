// useNetworkSeries 회귀 테스트.
//
// 핵심 계약: 호출부가 `interfaces` 를 매 렌더 새 배열로 넘겨도 렌더가 폭주하지 않는다.
// 이 계약이 깨지면 "effect → setState → 리렌더 → 새 배열 → effect" 가 끝없이 돌아
// React 가 렌더 깊이 초과(#185)로 화면을 통째로 날린다.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import type { NetworkStats } from '@/services/api/monitorService';

/** 폴링 훅이 돌려줄 응답 (테스트가 갈아 끼운다) */
const statsRef = vi.hoisted(() => ({ current: undefined as NetworkStats | undefined }));

vi.mock('@/services/api/monitorService', () => ({
  useNetworkStats: () => ({ data: statsRef.current, isLoading: false }),
}));

import { useNetworkSeries } from './useNetworkSeries';

function iface(name: string, over: Partial<Record<string, number>> = {}) {
  return {
    name,
    bytes_sent: 0, bytes_recv: 0, packets_sent: 0, packets_recv: 0,
    err_in: 0, err_out: 0, drop_in: 0, drop_out: 0,
    ...over,
  } as NetworkStats['total'];
}

function stats(total: NetworkStats['total'], interfaces: NetworkStats['interfaces']): NetworkStats {
  return { total, interfaces };
}

/** 렌더 횟수를 세는 소비자. `interfaces` 를 일부러 매번 새 배열로 넘긴다. */
let renderCount = 0;
function Probe({ names, unit = 'sec' as const }: { names: string[]; unit?: 'sec' | 'min' }) {
  renderCount += 1;
  // 실제 패널이 config 에서 매 렌더 새 배열을 만들던 상황을 그대로 재현한다.
  const result = useNetworkSeries(5_000, [...names], unit);
  return (
    <div
      data-testid="probe"
      data-ifaces={Object.keys(result.series).join(',')}
      data-missing={result.missing.join(',')}
      data-points={String(result.series.total?.rxBytesTotal?.length ?? 0)}
    />
  );
}

describe('useNetworkSeries', () => {
  beforeEach(() => {
    renderCount = 0;
    statsRef.current = undefined;
  });

  it('매 렌더 새 배열을 받아도 렌더가 폭주하지 않는다', () => {
    statsRef.current = stats(iface('total', { bytes_recv: 100 }), [iface('en0', { bytes_recv: 100 })]);

    render(<Probe names={[]} />);

    // 최초 렌더 + 계열 반영 1회 정도면 충분하다. 루프였다면 수천 번 돌다 죽는다.
    expect(renderCount).toBeLessThan(5);
    expect(screen.getByTestId('probe')).toHaveAttribute('data-ifaces', 'total');
  });

  it('부모가 다시 렌더해도 계열이 계속 늘어나지 않는다', () => {
    statsRef.current = stats(iface('total', { bytes_recv: 100 }), []);
    const { rerender } = render(<Probe names={[]} />);

    const after = () => Number(screen.getByTestId('probe').getAttribute('data-points'));
    const first = after();

    // 응답이 그대로인데 부모만 다시 그린다 — 포인트가 늘면 매 렌더 append 하고 있다는 뜻이다.
    rerender(<Probe names={[]} />);
    rerender(<Probe names={[]} />);

    expect(after()).toBe(first);
    expect(renderCount).toBeLessThan(8);
  });

  it('새 응답이 오면 포인트가 하나 늘어난다', () => {
    statsRef.current = stats(iface('total', { bytes_recv: 100 }), []);
    const { rerender } = render(<Probe names={[]} />);
    const first = Number(screen.getByTestId('probe').getAttribute('data-points'));

    act(() => {
      statsRef.current = stats(iface('total', { bytes_recv: 200 }), []);
    });
    rerender(<Probe names={[]} />);

    expect(Number(screen.getByTestId('probe').getAttribute('data-points'))).toBe(first + 1);
  });

  it('선택한 인터페이스가 응답에 없으면 missing 으로 알린다', () => {
    // 이 보고는 updater 밖에서 계산해야 한다 — updater 안에서 모으면 비어서 나온다.
    statsRef.current = stats(iface('total'), [iface('en0')]);

    render(<Probe names={['en0', 'eth9']} />);

    expect(screen.getByTestId('probe')).toHaveAttribute('data-missing', 'eth9');
  });

  it('대상이 바뀌면 계열을 새로 시작한다', () => {
    statsRef.current = stats(iface('total', { bytes_recv: 100 }), [iface('en0', { bytes_recv: 50 })]);
    const { rerender } = render(<Probe names={[]} />);
    expect(screen.getByTestId('probe')).toHaveAttribute('data-ifaces', 'total');

    rerender(<Probe names={['en0']} />);

    // 이전 대상의 계열이 남아 있으면 축이 섞인다.
    expect(screen.getByTestId('probe')).toHaveAttribute('data-ifaces', 'en0');
  });

  it('단위시간이 바뀌어도 계열을 새로 시작한다', () => {
    statsRef.current = stats(iface('total', { bytes_recv: 100 }), []);
    const { rerender } = render(<Probe names={[]} unit="sec" />);

    rerender(<Probe names={[]} unit="min" />);

    // 초 기준 값과 분 기준 값이 한 차트에 섞이면 안 된다.
    expect(Number(screen.getByTestId('probe').getAttribute('data-points'))).toBeLessThanOrEqual(1);
  });
});
