// 도면 이미지 배경 레이어 (SPEC-HEATMAP-PANEL-002 T3 + 다중 이미지 확장).
//
// 히트맵 canvas '아래' 레이어에 data-URL 도면 이미지를 렌더한다(배경 합성 방식 (a):
// 별도 배경 DOM 요소 + CSS opacity 로 히트맵 합성 — plan.md 결정).
//
// 다중 레이어: 각 레이어는 **스테이지 정규화 박스**(x/y/w/h, 0..1)에 % 로 배치되고 박스 안에서
// object-fit(contain/cover)으로 맞춰진다. 배열 순서가 그리기 순서(뒤 항목이 위)다. 기준 레이어
// (index 0)는 기본 박스 {0,0,1,1} 로 스테이지를 정확히 채우며, 스테이지 자체가 그 이미지의
// 종횡비를 갖기 때문에 여백 없이 딱 맞는다(stage.ts 참조).
//
// 순수 프레젠테이션(데이터 조회/드래그 로직 없음). 레이어가 없으면 null 을 렌더해 graceful
// 하게 비활성(AC-E1).
//
// @spec SPEC-HEATMAP-PANEL-002

import type { FloorPlanLayer } from './heatmapConfig';

interface FloorPlanBackgroundProps {
  /** 도면 레이어 목록(그리기 순서). 비면 배경 없음(null 렌더). */
  layers: FloorPlanLayer[];
  /**
   * 레이어별 해석된 이미지 src(같은 길이, 같은 순서). 자산 id 는 비동기 조회 결과이므로
   * 아직 도착하지 않은 레이어는 빈 문자열이고 렌더에서 제외된다 — 빈 src 를 <img> 에 걸면
   * 브라우저가 현재 페이지를 다시 요청한다.
   */
  sources: string[];
  /**
   * 스테이지가 도면 종횡비를 버리고 컨테이너로 늘어난 상태(stage_fit='stretch').
   *
   * 이때는 레이어별 fit 을 무시하고 모두 'fill' 로 그린다 — 스테이지가 이미 비등방으로 늘어났는데
   * 이미지가 'contain' 이면 이미지 안에서 다시 레터박스가 생겨 여백이 그대로 돌아온다.
   */
  stretch?: boolean;
  /**
   * 기준 레이어(index 0)가 실제로 그려졌을 때 그 원본 종횡비(폭/높이)를 알린다.
   *
   * 스테이지는 기준 도면과 같은 종횡비여야 마커의 정규화 좌표가 도면에 고정된다. 종횡비를
   * 못 구하면 스테이지가 패널 전체로 퇴화하고, 그러면 도면만 `object-fit` 으로 안에서 다시
   * 레터박스되어 **마커는 패널을, 도면은 자기 비율을 따르는** 어긋남이 생긴다(보고된 증상).
   * config 에 저장된 크기도, 별도 `new Image()` 실측도 실패할 수 있으므로(자산 URL·인증·캐시)
   * **브라우저가 이미 그린 그 이미지**에서 받아오는 이 경로를 마지막 보루로 둔다.
   */
  onBaseAspect?: (aspect: number) => void;
}

/** 도면 이미지 배경. 그릴 레이어가 없으면 null(AC-E1). */
export default function FloorPlanBackground({
  layers,
  sources,
  stretch,
  onBaseAspect,
}: FloorPlanBackgroundProps) {
  // 레이어가 없거나 아직 해석된 src 가 하나도 없으면 배경을 렌더하지 않는다.
  if (layers.length === 0 || sources.every((s) => !s)) return null;
  return (
    <>
      {layers.map((layer, i) =>
        !sources[i] ? null : (
        <img
          key={i}
          src={sources[i]}
          alt=""
          aria-hidden="true"
          data-testid={i === 0 ? 'floor-plan-background' : `floor-plan-layer-${i}`}
          // 기준 레이어만 스테이지 종횡비를 정한다(나머지는 그 위에 얹히는 겹판이다).
          onLoad={
            i === 0 && onBaseAspect
              ? (e) => {
                  const { naturalWidth: w, naturalHeight: h } = e.currentTarget;
                  if (w > 0 && h > 0) onBaseAspect(w / h);
                }
              : undefined
          }
          // 스테이지 정규화 박스에 % 배치. 포인터 이벤트는 위 레이어(canvas/오버레이)로 통과.
          className="pointer-events-none absolute"
          style={{
            left: `${layer.x * 100}%`,
            top: `${layer.y * 100}%`,
            width: `${layer.w * 100}%`,
            height: `${layer.h * 100}%`,
            objectFit: stretch ? 'fill' : layer.fit,
            opacity: layer.opacity,
          }}
        />
        ),
      )}
    </>
  );
}
