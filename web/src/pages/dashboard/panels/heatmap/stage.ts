// 히트맵 스테이지 좌표계 순수 함수.
//
// 스테이지란: 패널 본문 안에 놓이는 "기준 도면의 종횡비를 가진 박스"다. 히트맵 canvas, 등고선,
// 센서 마커, 도면 레이어가 모두 이 박스 안에 그려지고 그 안에서 0..1 로 정규화된다.
//
// 왜 필요한가: 정규화 좌표를 **패널 컨테이너** 기준으로 잡으면, 도면은 object-fit 으로
// 레터박싱되는데 좌표는 컨테이너를 따르므로 컨테이너 종횡비가 달라질 때마다(설정 미리보기 3:2 vs
// 대시보드 그리드 셀) 마커가 도면 위에서 미끄러진다. 스테이지가 기준 도면과 같은 종횡비를 가지면
// 기준 도면이 스테이지를 정확히 채우므로 "스테이지 정규화 좌표 = 도면 정규화 좌표" 가 되고,
// 컨테이너 크기/비율이 무엇이든 마커는 도면의 같은 지점에 붙는다.
//
// DOM 에 의존하지 않도록 크기를 인자로 받는다(placement.ts 와 동일 규율 — 단위 테스트 대상).

import { clamp01 } from './placement';
import type { SensorPosition } from './heatmapConfig';

/** 컨테이너 안에 놓인 스테이지의 픽셀 사각형. */
export interface StageBox {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * 스테이지를 컨테이너에 맞추는 방식.
 *
 * - `contain`(기본): 도면 종횡비를 지키며 컨테이너 안에 들어간다 → 남는 축에 여백(레터박스).
 * - `cover`: 종횡비를 지키며 컨테이너를 덮는다 → 여백 없음, 대신 넘치는 쪽이 **잘린다**.
 * - `stretch`: 종횡비를 버리고 컨테이너를 그대로 쓴다 → 여백 없음, 대신 도면이 **늘어난다**.
 *
 * 셋 모두 좌표는 스테이지 정규화(0..1)라 마커는 도면의 같은 지점에 붙는다. 다만 `cover` 에서는
 * 잘린 영역이 화면 밖이라 그곳의 마커를 배치 편집에서 잡을 수 없다.
 */
export type StageFit = 'contain' | 'cover' | 'stretch';

/**
 * 컨테이너(w×h) 안에서 종횡비 `aspect`(=폭/높이)에 맞춘 스테이지 박스를 중앙 정렬로 구한다.
 *
 * aspect 가 유한 양수가 아니거나 `fit`이 'stretch' 이면 컨테이너 전체를 그대로 돌려준다
 * (도면 없음 = 스테이지가 곧 컨테이너 → 기존 동작과 동일).
 *
 * `cover` 는 스테이지가 컨테이너보다 커지므로 left/top 이 음수가 된다 — 호출부가 컨테이너를
 * overflow-hidden 으로 잘라야 패널 밖으로 새지 않는다.
 */
export function computeStageBox(
  w: number,
  h: number,
  aspect: number | undefined,
  fit: StageFit = 'contain',
): StageBox {
  if (!(w > 0) || !(h > 0)) return { left: 0, top: 0, width: 0, height: 0 };
  if (aspect === undefined || !Number.isFinite(aspect) || aspect <= 0 || fit === 'stretch') {
    return { left: 0, top: 0, width: w, height: h };
  }
  // 컨테이너가 도면보다 넓을 때: contain 은 높이에 맞추고(좌우 여백), cover 는 폭에 맞춘다
  // (상하로 넘쳐 잘림). 좁을 때는 정확히 반대다.
  const containerWider = w / h >= aspect;
  const heightBound = fit === 'cover' ? !containerWider : containerWider;
  if (heightBound) {
    const width = h * aspect;
    return { left: (w - width) / 2, top: 0, width, height: h };
  }
  const height = w / aspect;
  return { left: 0, top: (h - height) / 2, width: w, height };
}

/**
 * 컨테이너 정규화 좌표(레거시) → 스테이지 정규화 좌표.
 *
 * 레거시 패널의 좌표는 "지금 화면에 보이는 그 위치"를 컨테이너 기준으로 담고 있다. 컨테이너와
 * 스테이지의 실측 픽셀을 모두 아는 렌더 시점에 환산하면 **보이는 위치가 그대로 보존**된다
 * (스테이지 도입으로 마커가 소리 없이 움직이지 않는다). 스테이지가 컨테이너와 같으면 항등이다.
 */
export function containerToStage(
  pos: SensorPosition,
  container: { width: number; height: number },
  stage: StageBox,
): SensorPosition {
  if (!(stage.width > 0) || !(stage.height > 0)) return pos;
  const px = pos.x * container.width;
  const py = pos.y * container.height;
  return {
    x: clamp01((px - stage.left) / stage.width),
    y: clamp01((py - stage.top) / stage.height),
  };
}

/** 좌표 맵 전체를 컨테이너 → 스테이지 공간으로 환산한다(레거시 1회 이관용). */
export function migratePositionsToStage(
  positions: Record<string, SensorPosition>,
  container: { width: number; height: number },
  stage: StageBox,
): Record<string, SensorPosition> {
  const out: Record<string, SensorPosition> = {};
  for (const [key, pos] of Object.entries(positions)) {
    out[key] = containerToStage(pos, container, stage);
  }
  return out;
}

/**
 * 사용자가 도면을 직접 옮기고 키운 결과(스테이지 변형).
 *
 * 스테이지는 도면과 같은 종횡비를 가지므로 "도면을 옮긴다 = 스테이지를 옮긴다" 이다. 마커
 * 좌표는 스테이지 정규화(0..1)라서, 변형을 스테이지 박스에만 적용하면 마커는 도면 위 같은
 * 지점에 그대로 붙어 따라온다 — 좌표를 다시 계산할 필요가 없다.
 *
 *   - offset_x / offset_y: 컨테이너 크기 대비 비율 이동(-1..1). 컨테이너가 커지거나 작아져도
 *     같은 상대 위치를 유지하도록 픽셀이 아니라 비율로 저장한다.
 *   - scale: 배율(> 0). 1 이면 변형 없음.
 */
export interface StageTransform {
  offset_x?: number;
  offset_y?: number;
  scale?: number;
}

/** 변형이 실질적으로 없는지(항등) 판정한다. */
export function isIdentityTransform(tr: StageTransform | undefined): boolean {
  if (!tr) return true;
  const { offset_x = 0, offset_y = 0, scale = 1 } = tr;
  return offset_x === 0 && offset_y === 0 && scale === 1;
}

/**
 * fit 으로 구한 스테이지 박스에 사용자 변형을 적용한다.
 *
 * 배율은 **박스 중심을 기준**으로 적용한다 — 좌상단 기준이면 크기를 키울 때 도면이 한쪽으로
 * 쏠려 "가운데를 키운다" 는 직관과 어긋난다. 이동은 컨테이너 크기 대비 비율이므로 컨테이너
 * 픽셀을 함께 받는다.
 *
 * 변형이 항등이거나 박스가 비어 있으면 입력을 그대로 돌려준다(기존 동작 보존).
 */
export function applyStageTransform(
  box: StageBox,
  transform: StageTransform | undefined,
  container: { width: number; height: number },
): StageBox {
  if (isIdentityTransform(transform) || !(box.width > 0) || !(box.height > 0)) return box;
  const { offset_x = 0, offset_y = 0, scale = 1 } = transform!;
  const s = Number.isFinite(scale) && scale > 0 ? scale : 1;
  const width = box.width * s;
  const height = box.height * s;
  return {
    // 중심 고정 배율 + 컨테이너 비율 이동.
    left: box.left - (width - box.width) / 2 + offset_x * container.width,
    top: box.top - (height - box.height) / 2 + offset_y * container.height,
    width,
    height,
  };
}

/**
 * 스테이지 정규화 좌표 → 컨테이너(패널 본문) 정규화 좌표.
 *
 * `containerToStage` 의 역변환이다. 스테이지 안에 그리던 오버레이(색표 범례)를 패널 본문으로
 * 옮길 때, 저장된 스테이지 좌표를 **보이던 픽셀 위치 그대로** 패널 좌표로 환산하는 데 쓴다.
 * 스테이지가 컨테이너와 같으면 항등이다.
 */
export function stageToContainer(
  pos: SensorPosition,
  container: { width: number; height: number },
  stage: StageBox,
): SensorPosition {
  if (!(container.width > 0) || !(container.height > 0)) return pos;
  const px = stage.left + pos.x * stage.width;
  const py = stage.top + pos.y * stage.height;
  return { x: clamp01(px / container.width), y: clamp01(py / container.height) };
}
