// 요소의 **윤곽 상자** — 고른 것을 두르고, 앵커가 설 자리를 내는 한 벌 (SPEC-CANVAS-011 M2).
//
// 이 함수는 002 가 선택 외곽선을 위해 지었고 004·009 가 그룹과 부품으로 넓혔다. 011 은
// 그것을 **다시 짓지 않고 옮겼을 뿐이다** — 값은 한 글자도 바뀌지 않았고, 그 무동작성은
// `CanvasEditOverlay.outline.test.tsx` 가 옮기기 전에 먼저 못박았다.
//
// **왜 오버레이 밖으로 나왔는가.** 앵커(M3)와 연결선 그리기(M6)가 같은 상자를 필요로 한다.
// 오버레이의 모듈 private 으로 두면 그쪽이 제 상자를 다시 짓게 되고, 그 순간 "보이는 상자와
// 선이 붙는 상자가 다르다" 가 **표현 가능해진다** — 002 가 위험 R1 로 이름 적어 둔 자리다.
// 정의를 한 자리로 두는 것이 그 결함을 문법적으로 불가능하게 만드는 유일한 방법이다.
//
// **이 모듈은 DOM 을 모른다**(AC-08). 투영과 글자 폭 장부만 받는다 — 스스로 재지 않으므로
// 측정원이 둘이 될 수 없고, 렌더 없이도 값으로 시험할 수 있다.

import { DEFAULT_FONT_SIZE } from './canvasConfig';
import type { BoxGeometry, CanvasElement, PointGeometry } from './canvasConfig';
import {
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  type CanvasProjection,
  type PxBox,
  type PxPoint,
} from './canvasGeometry';
import type { OutlinedNode } from './group/groupTypes';
// 각도의 산술은 잎 모듈이 소유한다(SPEC-CANVAS-014). 여기서 제 손으로 모서리를 돌리면
// 회전 산술이 둘이 된다.
import { rotatedAabb } from './canvasRotation';

/** 유효한 글자 크기(px). `canvasHitTest.resolveFontSize` 와 같은 판정이다. */
export function resolveFontSize(size: number | undefined): number {
  if (size === undefined) return DEFAULT_FONT_SIZE;
  return Number.isFinite(size) && size > 0 ? size : DEFAULT_FONT_SIZE;
}

/** 실측 글자 폭. 아직 한 프레임도 그리지 않았으면 0 이다(AC-E7). */
export function resolveMeasuredWidth(width: number | undefined): number {
  return width !== undefined && Number.isFinite(width) && width > 0 ? width : 0;
}

/**
 * 선택 외곽선이 두를 상자(스테이지 로컬 CSS px, **양수 범위로 정규화**).
 *
 * 투영은 `canvasGeometry` 의 같은 함수를 지난다 — 렌더·히트·외곽선이 한 투영을 공유하면
 * 셋이 갈라질 수 없다(위험 R1).
 *
 * 문구 상자의 **세로 기준이 이 함수에서 가장 틀리기 쉬운 지점이다**(위험 R8).
 * `drawElement.TEXT_BASELINE` 이 `'middle'` 이므로 기준점 y 는 상자의 **세로 중심**이고,
 * 따라서 상단은 `기준점 y - fontSize/2` 다. 상단으로 착각하면 외곽선이 글자 아래로 반 줄
 * 내려가 앉는다. `canvasHitTest` 의 문구 상자와 같은 식이어야 **보이는 대로 잡힌다**.
 *
 * **경로는 rect 와 같은 상자다.** 종전의 `default:` 는 경로의 상자 기하를 문구 기준점으로
 * 읽어(구조적으로 대입된다 — 가정 A6) 외곽선을 실측 글자 폭 0 · 기본 글자 크기의 작은
 * 상자로 그렸다. 컴파일러가 울지 않던 자리이므로 갈래를 이름으로 적는다.
 *
 * **연결선은 여기 오지 않는다**(SPEC-CANVAS-011 M4). 인자가 `CanvasNode` 가 아니라
 * `OutlinedNode` 인 것이 그 금지의 전부다 — 연결선에는 `geometry` 가 없어 낼 상자가 없고,
 * 두 끝을 감싸는 상자를 지어 돌려주면 그 상자가 곧 8핸들이 잡는 상자이자 마키가 재는
 * 상자가 된다(REQ-07 이 금지한 그것). 뜻 없는 값을 돌려주느니 **타입이 "여기 올 수 없다"
 * 고 말하게 두는 편**이 낫고, 그러면 부르는 쪽이 건너뛰기를 잊는 것이 불가능해진다.
 */
export function outlineBox(
  el: OutlinedNode,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): PxBox {
  switch (el.kind) {
    // 그룹의 윤곽은 **제 상자**다(REQ-08). 부품의 합집합을 여기서 다시 재지 않는다 —
    // 상자는 묶는 순간 적힌 **저장된 값**이고(가정 A16), 파생으로 되재면 8핸들이
    // 잡는 상자와 윤곽이 두는 상자가 갈라진다. 그 갈라짐은 "늘리면 테두리만 안 따라온다"
    // 로만 보고되는 부류다.
    case 'group':
    case 'rect':
    case 'ellipse':
    case 'path': {
      const box = projectBox(el.geometry, proj);
      return normalizeBox(box);
    }
    case 'line': {
      const line = projectLine(el.geometry, proj);
      return normalizeBox({
        x: line.x1,
        y: line.y1,
        w: line.x2 - line.x1,
        h: line.y2 - line.y1,
      });
    }
    case 'text': {
      const width = resolveMeasuredWidth(textWidths[el.id]);
      const fontSize = resolveFontSize(el.style.fontSize);
      const origin = resolveTextOrigin(
        projectPoint(el.geometry, proj),
        el.style.align ?? 'left',
        width,
      );
      return normalizeBox({ x: origin.x, y: origin.y - fontSize / 2, w: width, h: fontSize });
    }
  }
}

/**
 * 회전 축 — **이 요소의 px 윤곽 상자 가운데**다 (SPEC-CANVAS-014 §결정 3 · K1).
 *
 * ## 왜 이 함수가 여기 있는가
 *
 * 그리는 쪽(`drawElement`)은 이 축 둘레로 좌표계를 돌리고, 잡는 쪽(`canvasHitTest`)은 이 축
 * 둘레로 점을 되돌린다. **두 자리가 축을 따로 구하면 그 등식이 깨지고**, 그 어긋남은 각도가
 * 0 일 때 보이지 않으므로 돌린 뒤에야 드러난다 — 002 위험 R1 의 그 부류다.
 *
 * 그래서 함수를 하나 두고 둘이 그것을 부른다. K1("그리기와 잡기가 같은 각도 하나를 본다")이
 * 주석이 아니라 **문법**으로 지켜지는 자리다.
 *
 * ## 투영을 받지 않는다
 *
 * 부르는 쪽이 이미 가진 투영 도우미를 그대로 받는다. 그 둘은 부품이면 그룹 상자 안으로,
 * 최상위면 스테이지로 가는 갈래이며 **두 파일이 이미 같은 형상으로** 들고 있다 — 여기서
 * 투영을 다시 고르면 세 번째 갈래가 생긴다.
 *
 * 문구 가운데의 y 가 원점 y 와 같은 것은 상자가 `원점y − 글자크기/2` 에서 시작해 높이가
 * 글자 크기이기 때문이다. 그래서 글자 크기를 여기서 구할 필요가 없다.
 *
 * `line` 은 각도를 갖지 않으므로(§D6) `undefined` 다 — 부르는 쪽이 그 갈래에 닿지 않는다.
 */
export function rotationPivotIn(
  el: CanvasElement,
  pxBox: (geo: BoxGeometry) => PxBox,
  pxPoint: (geo: PointGeometry) => PxPoint,
  measuredWidth: number,
): PxPoint | undefined {
  switch (el.kind) {
    case 'rect':
    case 'ellipse':
    case 'path': {
      const box = pxBox(el.geometry);
      return { x: box.x + box.w / 2, y: box.y + box.h / 2 };
    }
    case 'text': {
      const origin = resolveTextOrigin(
        pxPoint(el.geometry),
        el.style.align ?? 'left',
        measuredWidth,
      );
      return { x: origin.x + measuredWidth / 2, y: origin.y };
    }
    default:
      return undefined;
  }
}

/**
 * 이 노드가 돌아간 각도(정수 도). 회전 축이 없는 종류는 **0** 이다.
 *
 * `kind:'line'` 이 0 인 것은 결측이 아니라 **금지의 귀결**이다 — 선의 임의 각도는 두
 * 끝점으로 표현되므로 필드가 서지 않는다(014 §D6). 여기서 `?? 0` 한 줄이 그 사실을
 * 소비자에게 옮겨 준다.
 */
export function outlineAngle(el: OutlinedNode): number {
  return 'rotation' in el && el.rotation !== undefined ? el.rotation : 0;
}

/**
 * 돌아간 잉크를 덮는 **축-나란 상자**(SPEC-CANVAS-014 §결정 2).
 *
 * ## 왜 `outlineBox` 를 고치지 않고 파생시키는가
 *
 * 돌아간 도형에는 상자가 둘이다 — 요소의 로컬 축을 그대로 둔 **방향 상자**(8핸들 · 앵커 ·
 * 선택 윤곽이 읽는다)와 그 잉크를 덮는 **축-나란 상자**(마키 · 정렬이 읽는다). 한 이름으로
 * 부르면 손잡이가 마키의 상자에 서거나 그 반대가 된다.
 *
 * 그렇다고 `outlineBox` 의 뜻을 바꾸지는 않는다. 그 함수가 내는 것은 **요소 제 상자**이고
 * 그 뜻은 014 이전과 한 글자도 다르지 않다 — 각도는 `outlineAngle` 이 따로 말하며, 둘을
 * 합친 것이 곧 방향 상자다.
 *
 * **그리고 이 함수는 그 상자에서 파생된다**(K2). 제 손으로 다시 재면 두 번째 측정이 생기고,
 * 그 갈라짐은 크기나 각도를 바꾼 뒤에야 화면에서만 드러난다 — `anchors.ts` 가 "상자는
 * 여기서 **한 번** 나온다(AC-33)" 고 못박아 둔 그 규율이다.
 *
 * **각도가 0 이면 `outlineBox` 와 바이트 동일하다**(K3). 014 이전의 모든 화면이 그 등식
 * 위에 서 있으므로, 여기서 부동소수 왕복을 한 번이라도 태우면 안 된다.
 */
export function outlineAabb(
  el: OutlinedNode,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): PxBox {
  const box = outlineBox(el, proj, textWidths);
  return rotatedAabb(box, outlineAngle(el));
}

/** 음수 크기를 양수 범위로 편다. 001 은 음수 크기 박스를 그릴 수 있게 해 두었다. */
function normalizeBox(box: PxBox): PxBox {
  return {
    x: Math.min(box.x, box.x + box.w),
    y: Math.min(box.y, box.y + box.h),
    w: Math.abs(box.w),
    h: Math.abs(box.h),
  };
}
