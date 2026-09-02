// 도면 레이어 → 렌더 가능한 이미지 src 해석 훅.
//
// 레이어는 두 경로 중 하나로 이미지를 가리킨다:
//   - asset_id (신규): 자산 API 에서 인증된 요청으로 data-URL 을 받아온다.
//   - image    (레거시): config 에 인라인 저장된 data-URL 을 그대로 쓴다.
//
// 왜 훅인가: 자산 조회는 비동기이고 인증 헤더가 필요해 <img src="/api/..."> 로 직접 걸 수
// 없다(브라우저가 Authorization 헤더를 싣지 않는다). 세션 캐시(dashboardAssetService)가
// 중복 요청을 막으므로, 같은 도면을 쓰는 패널이 여러 개여도 네트워크 요청은 한 번이다.
//
// 실패 정책: 자산 조회가 실패하면 그 레이어는 레거시 image 로 폴백하고, 그것도 없으면
// 빈 문자열이 되어 렌더에서 제외된다 — 예외를 던지지 않는다(패널 전체가 죽지 않도록).

import { useEffect, useState } from 'react';

import { fetchDashboardAssetUrl } from '@/services/api/dashboardAssetService';

import type { FloorPlanLayer } from './heatmapConfig';

/** 레이어의 즉시 사용 가능한 src(레거시 인라인 이미지). 없으면 빈 문자열. */
function inlineSrc(layer: FloorPlanLayer): string {
  return layer.image ?? '';
}

/**
 * 레이어 배열과 같은 길이의 src 배열을 반환한다. 자산은 도착하는 대로 채워지므로,
 * 첫 렌더에서는 레거시 이미지만 있는 상태(자산 레이어는 빈 문자열)일 수 있다.
 */
export function useFloorPlanSources(layers: FloorPlanLayer[]): string[] {
  // 자산 id → data-URL. 레이어 배열이 바뀌어도 이미 받은 자산은 재사용한다.
  const [resolved, setResolved] = useState<Record<string, string>>({});

  // 요청 대상 id 목록을 문자열로 접어 의존성으로 쓴다 — 배열 참조가 매 렌더 바뀌어도
  // 실제 id 집합이 같으면 재요청하지 않는다.
  const assetIds = layers
    .map((l) => l.asset_id)
    .filter((id): id is string => typeof id === 'string' && id !== '');
  const assetKey = assetIds.join(',');

  useEffect(() => {
    if (assetKey === '') return;
    let alive = true;
    const ids = assetKey.split(',');
    for (const id of ids) {
      fetchDashboardAssetUrl(id)
        .then((url) => {
          if (!alive) return;
          setResolved((prev) => (prev[id] === url ? prev : { ...prev, [id]: url }));
        })
        // 조회 실패는 삼킨다 — 레이어는 레거시 이미지로 폴백하거나 그려지지 않는다.
        .catch(() => {});
    }
    return () => {
      alive = false;
    };
  }, [assetKey]);

  return layers.map((layer) => {
    if (layer.asset_id) return resolved[layer.asset_id] ?? inlineSrc(layer);
    return inlineSrc(layer);
  });
}
