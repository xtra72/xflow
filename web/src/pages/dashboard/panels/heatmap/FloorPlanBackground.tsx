// 도면 이미지 배경 레이어 (SPEC-HEATMAP-PANEL-002 T3).
//
// 히트맵 canvas '아래' 레이어에 data-URL 도면 이미지를 렌더한다(배경 합성 방식 (a):
// 별도 배경 DOM 요소 + CSS opacity 로 히트맵 합성 — plan.md 결정). 종횡비 보존은 CSS
// object-fit(contain/cover)으로 처리한다(REQ-01/REQ-05).
//
// 순수 프레젠테이션(데이터 조회/드래그 로직 없음 — 드래그 배치는 stage 2). 이미지 미첨부 시
// null 을 렌더해 graceful 하게 비활성(AC-E1).
//
// @spec SPEC-HEATMAP-PANEL-002

import { DEFAULT_FLOOR_PLAN_FIT } from './heatmapConfig';

interface FloorPlanBackgroundProps {
  /** 도면 이미지 data-URL. 미지정 시 배경 없음(null 렌더). */
  image?: string;
  /** 배경 맞춤(종횡비 보존). 기본 contain. */
  fit?: 'contain' | 'cover';
}

/** 도면 이미지 배경. image 미지정 시 null(AC-E1). */
export default function FloorPlanBackground({
  image,
  fit = DEFAULT_FLOOR_PLAN_FIT,
}: FloorPlanBackgroundProps) {
  // 이미지가 없으면 배경을 렌더하지 않는다(graceful 비활성, 렌더 예외 없음).
  if (!image) return null;
  return (
    <img
      src={image}
      alt=""
      aria-hidden="true"
      data-testid="floor-plan-background"
      // 컨테이너 전체를 채우는 배경 레이어. 포인터 이벤트는 위 레이어(canvas/오버레이)로 통과.
      className="pointer-events-none absolute inset-0 h-full w-full"
      // 종횡비 보존: contain(레터박스) / cover(꽉 채움). object-fit 으로 처리.
      style={{ objectFit: fit }}
    />
  );
}
