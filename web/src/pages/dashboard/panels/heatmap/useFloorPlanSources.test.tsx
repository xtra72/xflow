// useFloorPlanSources — 도면 레이어 src 해석 훅 테스트.
//
// 자산 경로(asset_id)는 인증된 요청으로 data-URL 을 받아오고, 레거시 인라인 이미지는
// 그대로 쓴다. 조회 실패가 패널을 죽이지 않는 것(빈 src → 렌더 제외)까지 커버한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';

const fetchMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/dashboardAssetService', () => ({
  fetchDashboardAssetUrl: fetchMock,
}));

import { useFloorPlanSources } from './useFloorPlanSources';
import type { FloorPlanLayer } from './heatmapConfig';

function layer(overrides: Partial<FloorPlanLayer> = {}): FloorPlanLayer {
  return { x: 0, y: 0, w: 1, h: 1, opacity: 1, fit: 'contain', ...overrides };
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe('useFloorPlanSources', () => {
  it('레거시 인라인 이미지는 즉시(네트워크 없이) src 가 된다', () => {
    const layers = [layer({ image: 'data:image/png;base64,AAAA' })];
    const { result } = renderHook(() => useFloorPlanSources(layers));
    expect(result.current).toEqual(['data:image/png;base64,AAAA']);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('asset_id 는 자산 조회 결과로 채워진다', async () => {
    fetchMock.mockResolvedValue('data:image/png;base64,FROM-ASSET');
    const layers = [layer({ asset_id: 'abc' })];
    const { result } = renderHook(() => useFloorPlanSources(layers));
    // 첫 렌더는 아직 비어 있다(비동기).
    expect(result.current).toEqual(['']);
    await waitFor(() => expect(result.current).toEqual(['data:image/png;base64,FROM-ASSET']));
    expect(fetchMock).toHaveBeenCalledWith('abc');
  });

  it('자산 조회가 실패하면 레거시 인라인 이미지로 폴백한다', async () => {
    fetchMock.mockRejectedValue(new Error('network'));
    const layers = [layer({ asset_id: 'abc', image: 'data:image/png;base64,LEGACY' })];
    const { result } = renderHook(() => useFloorPlanSources(layers));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    // 예외가 새어나가지 않고 인라인 이미지가 그려진다.
    expect(result.current).toEqual(['data:image/png;base64,LEGACY']);
  });

  it('자산 조회 실패 + 인라인 이미지 없음 → 빈 src (렌더에서 제외)', async () => {
    fetchMock.mockRejectedValue(new Error('network'));
    const layers = [layer({ asset_id: 'abc' })];
    const { result } = renderHook(() => useFloorPlanSources(layers));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(result.current).toEqual(['']);
  });

  it('여러 레이어를 순서대로 해석한다(자산 + 레거시 혼합)', async () => {
    fetchMock.mockResolvedValue('data:image/png;base64,ASSET');
    const layers = [
      layer({ asset_id: 'a1' }),
      layer({ image: 'data:image/png;base64,INLINE' }),
    ];
    const { result } = renderHook(() => useFloorPlanSources(layers));
    await waitFor(() =>
      expect(result.current).toEqual([
        'data:image/png;base64,ASSET',
        'data:image/png;base64,INLINE',
      ]),
    );
  });

  it('레이어 배열이 새 참조로 재생성돼도 같은 id 집합이면 재요청하지 않는다', async () => {
    fetchMock.mockResolvedValue('data:image/png;base64,ASSET');
    const { rerender } = renderHook(({ ls }: { ls: FloorPlanLayer[] }) => useFloorPlanSources(ls), {
      initialProps: { ls: [layer({ asset_id: 'a1' })] },
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    // 동일 내용의 새 배열(참조만 다름) — 재요청이 없어야 한다.
    rerender({ ls: [layer({ asset_id: 'a1' })] });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
