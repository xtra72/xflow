// 기준 도면 레이어의 종횡비(폭/높이) 해석 훅.
//
// 스테이지(stage.ts)는 기준 도면과 같은 종횡비를 가져야 하므로 그 값이 필요하다. 우선순위:
//   1) config 에 저장된 natural_width/height (즉시 사용 — 첫 페인트부터 정확)
//   2) 없으면 이미지를 실제로 로드해 naturalWidth/Height 실측 (레거시 config 대응)
// 둘 다 없으면 undefined → 스테이지가 컨테이너 전체가 된다(도면 없음과 동일, 기존 동작).
//
// 실측값은 **런타임 상태로만** 들고 config 에 쓰지 않는다 — 렌더가 config 를 쓰기 시작하면
// 미리보기/대시보드가 서로의 draft 를 덮어쓰는 경합이 생긴다. 사용자가 도면을 다시 첨부하면
// 그 경로에서 natural_* 가 저장된다.

import { useEffect, useState } from 'react';

import type { FloorPlanLayer } from './heatmapConfig';

/** 레이어에 저장된 원본 크기로 종횡비를 구한다. 둘 다 양수일 때만 유효. */
function storedAspect(layer: FloorPlanLayer | undefined): number | undefined {
  if (!layer) return undefined;
  const w = layer.natural_width;
  const h = layer.natural_height;
  if (w !== undefined && h !== undefined && w > 0 && h > 0) return w / h;
  return undefined;
}

/**
 * 기준 도면(레이어 0)의 종횡비를 반환한다. 도면이 없으면 undefined.
 * 저장된 natural_* 가 있으면 즉시 그 값을, 없으면 이미지 로드 후 실측값을 반환한다.
 *
 * `resolvedSrc` 는 useFloorPlanSources 가 해석한 실제 src 다 — 자산 경로(asset_id)는
 * config 에 이미지 바이트가 없으므로 실측하려면 해석된 src 가 필요하다.
 */
export function useFloorPlanAspect(
  base: FloorPlanLayer | undefined,
  resolvedSrc?: string,
): number | undefined {
  const stored = storedAspect(base);
  const src = resolvedSrc && resolvedSrc !== '' ? resolvedSrc : base?.image;
  const [measured, setMeasured] = useState<number | undefined>(undefined);

  useEffect(() => {
    // 저장값이 있으면 실측이 불필요하다(네트워크/디코드 비용 회피).
    if (!src || stored !== undefined) {
      setMeasured(undefined);
      return;
    }
    if (typeof Image === 'undefined') return;
    let alive = true;
    const img = new Image();
    img.onload = () => {
      if (!alive) return;
      const { naturalWidth: w, naturalHeight: h } = img;
      setMeasured(w > 0 && h > 0 ? w / h : undefined);
    };
    // 디코드 실패는 조용히 무시한다 — 종횡비를 못 구하면 스테이지가 컨테이너 전체가 될 뿐이다.
    img.onerror = () => {
      if (alive) setMeasured(undefined);
    };
    img.src = src;
    return () => {
      alive = false;
    };
  }, [src, stored]);

  return stored ?? measured;
}
