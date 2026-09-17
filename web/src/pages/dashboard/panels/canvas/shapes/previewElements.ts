// 미리보기 칸이 그리는 **요소 모델** — DOM 도 React 도 모른다 (SPEC-CANVAS-008 REQ-06 ·
// SPEC-CANVAS-011 REQ-01).
//
// `CanvasShapeCatalog.tsx` 에서 갈라 나온 파일이다. 가른 이유는 둘.
//
// **하나 — 이 자리는 순수하다.** 카탈로그 항목이나 원시형 종류를 받아 `CanvasElement` 를
// 돌려줄 뿐이라 렌더 없이 잴 수 있다. 컴포넌트 파일 안에 두면 그 사실이 가려진다.
//
// **둘 — 컴포넌트 파일은 컴포넌트만 내보낸다.** 011 이 `primitivePreviewElement` 를 더하며
// 그 규율을 깼고(`react-refresh/only-export-components`), 규칙을 끄는 대신 자리를 옮겼다.
//
// @spec SPEC-CANVAS-008 REQ-06 · SPEC-CANVAS-011 REQ-01

import type {
  CanvasElement,
  CanvasPrimitiveKind,
  PathElement,
} from '../canvasConfig';
import { newElement, pathSeedStyle } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import type { ShapeCatalogEntry } from './shapeCatalog';

/**
 * 미리보기 칸의 CSS 크기(px)와 그 안의 도형 상자.
 *
 * 좌표 공간을 CSS px 와 **같은 수**로 두어(`canvas` = 이 크기) 읽는 사람이 두 단위를
 * 환산하지 않게 한다. 도형 상자를 **정사각**으로 두는 것에는 뜻이 있다 — 카탈로그의 명령은
 * 정사각 로컬 격자(0..10000) 위에서 그려졌으므로, 직사각 상자에 넣으면 원이 타원이 되듯
 * 30종 전부가 눌린 채로 보인다.
 */
export const PREVIEW = { width: 44, height: 32, side: 26 } as const;

/**
 * 뒷면 배율. 미리보기는 26px 안에 별의 꼭짓점 열을 그리므로 장치 픽셀이 모자란다. DPR 을
 * 읽지 않고 2 로 고정하는 것은 이 칸이 **그림이 아니라 아이콘**이기 때문이다 — 표면
 * (`CanvasSurface`)이 DPR 을 읽는 것과 달리 여기서는 선명도 한 눈금이면 족하고, DPR 을
 * 읽으면 이 파일이 표면의 그 배선을 한 벌 더 갖게 된다.
 */
export const PREVIEW_SCALE = 2;

/** 미리보기 좌표계 → 뒷면 px. 도형은 언제나 이 투영을 지난다. */
export const PREVIEW_PROJECTION: CanvasProjection = {
  stage: { width: PREVIEW.width * PREVIEW_SCALE, height: PREVIEW.height * PREVIEW_SCALE },
  canvas: { width: PREVIEW.width, height: PREVIEW.height },
};

/** 미리보기 안의 도형 상자 — 가운데 놓인 정사각. */
const PREVIEW_BOX = {
  x: (PREVIEW.width - PREVIEW.side) / 2,
  y: (PREVIEW.height - PREVIEW.side) / 2,
  w: PREVIEW.side,
  h: PREVIEW.side,
} as const;

/** 미리보기가 그릴 카탈로그 요소. **놓았을 때와 같은 씨앗 스타일**을 입는다. */
export function previewElement(entry: ShapeCatalogEntry): PathElement {
  return {
    id: entry.id,
    kind: 'path',
    geometry: { ...PREVIEW_BOX },
    path: entry.path.map((cmd) => ({ ...cmd })),
    catalog_id: entry.id,
    style: pathSeedStyle(entry.path),
  };
}

/**
 * 문구 미리보기가 그리는 글자 — **대문자 `T` 하나**(SPEC-CANVAS-011 · 사용자 결정).
 *
 * 셋을 함께 고른 결과다.
 * - **로케일과 무관하다.** 실제 씨앗(`SEED_TEXT` = `{name} {value}{unit}`)을 그리면 44×32
 *   칸에서 잘리고, 번역된 낱말을 그리면 ko/en 이 갈려 미리보기가 로케일 분기를 갖는다.
 * - **칸 아래 이름과 겹치지 않는다.** 칸은 이미 "텍스트"/"Text" 를 글자로 달고 있으므로,
 *   그림까지 같은 낱말이면 한 칸이 같은 말을 두 번 한다.
 * - **정체성이 바뀌지 않는다.** 이 넷은 008 이전부터 lucide `Type` 아이콘으로 같은 모양을
 *   지고 있었다. 011 이 바꾼 것은 **잉크**(글리프 → 실제 렌더 경로)이지 모양이 아니므로
 *   AC-E10("팔레트의 오늘이 그대로 있다")의 근거가 그대로 선다.
 *
 * 상수인 것에 뜻이 있다 — 바꾸고 싶어지는 날 고칠 자리가 이 한 줄이다.
 */
const TEXT_PREVIEW_SAMPLE = 'T';

/**
 * 문구 미리보기의 글자 크기(**뒷면 px**).
 *
 * `fontSpec` 은 이 값을 투영에 통과시키지 않고 `ctx.font` 에 그대로 싣는다. 이 칸의 뒷면은
 * CSS px 의 `PREVIEW_SCALE` 배이므로, 도형이 차지하는 변(`PREVIEW.side`)과 글자 em 상자를
 * 같게 두려면 그 둘을 곱한 값이다. 숫자를 적지 않고 **파생**시키는 것이 요점이다 — 칸을
 * 키우는 날 도형과 글자가 함께 자란다.
 *
 * 결과: em 상자 26 CSS px(= 도형 변), 대문자 높이는 그 0.7 남짓인 18 CSS px 남짓이다.
 * 칸 높이 32 CSS px 안에 여유를 두고 앉으므로 `TEXT_BASELINE === 'middle'` 로 세로 가운데에
 * 놓아도 위아래가 잘리지 않는다.
 */
const TEXT_PREVIEW_FONT_SIZE = PREVIEW.side * PREVIEW_SCALE;

/**
 * 원시형 넷의 미리보기 요소. **한 번만 짓고 돌려 쓴다** — 렌더마다 새 객체를 지으면
 * `CanvasCellPreview` 의 효과가 매 렌더 다시 돌아 같은 그림을 두 번 그린다.
 *
 * 스타일은 **공장에서 그대로 가져온다**(`newElement`). 미리보기 전용 색을 지어내면 칸에
 * 보이는 모습과 눌렀을 때 놓이는 모습이 갈리고, 그것은 이 미리보기가 실제 렌더 경로를
 * 지나는 이유 자체를 무효로 만든다. 공장의 씨앗은 네 종류 모두 **보이는** 스타일이다
 * (rect·ellipse 는 채움, line 은 선과 두께, text 는 글자색과 글자) — 001 이 "기본 색을
 * 지어내지 않는다" 로 남긴 빈칸을 공장이 저술 시점에 이미 메워 두었기 때문이다.
 *
 * 바뀌는 것은 **기하뿐**이고, 그 기하는 카탈로그 30종이 쓰는 `PREVIEW_BOX` 안에 든다.
 */
const PRIMITIVE_PREVIEW_ELEMENTS: Readonly<Record<CanvasPrimitiveKind, CanvasElement>> =
  Object.freeze({
    rect: primitivePreview('rect'),
    ellipse: primitivePreview('ellipse'),
    line: primitivePreview('line'),
    text: primitivePreview('text'),
  });

function primitivePreview(kind: CanvasPrimitiveKind): CanvasElement {
  const id = `preview-${kind}`;
  // 공장이 심는 씨앗 스타일 그대로. 계단(`count`)은 미리보기에 뜻이 없으므로 0 이다.
  const style = newElement(id, kind, 0).style;
  switch (kind) {
    case 'rect':
      return { id, kind, geometry: { ...PREVIEW_BOX }, style };
    case 'ellipse':
      return { id, kind, geometry: { ...PREVIEW_BOX }, style };
    case 'line':
      // **대각선**이다. 가로 막대로 두면 26px 칸에서 구분선이나 하이픈으로 읽히고, 이웃한
      // `arrowRight`·`chevron` 이 이미 가로로 누워 있어 그 사이에서 "선" 이라는 뜻이
      // 흐려진다. 상자의 대각을 쓰면 잉크가 26 이 아니라 37 단위로 늘어 같은 칸에서
      // 가장 길게 읽히고, 왼쪽 아래에서 오른쪽 위로 오르는 방향은 선 도구의 통상적인 그림이다.
      return {
        id,
        kind,
        geometry: {
          x1: PREVIEW_BOX.x,
          y1: PREVIEW_BOX.y + PREVIEW_BOX.h,
          x2: PREVIEW_BOX.x + PREVIEW_BOX.w,
          y2: PREVIEW_BOX.y,
        },
        style,
      };
    // `default:` 가 아니라 이름으로 적는다 — 다섯 번째 원시형이 들어오면 컴파일러가 이
    // 자리를 가리켜야 한다(`newElement` 가 같은 이유로 같은 꼴을 쓴다).
    case 'text':
      return {
        id,
        kind,
        // 기준점은 상자 한가운데다. 세로는 `TEXT_BASELINE === 'middle'` 이 그 점을 글줄의
        // 가운데로 읽고, 가로는 `align: 'center'` 가 잰 폭의 절반만큼 원점을 물린다 —
        // 그 둘이 없으면 글자가 기준점의 오른쪽 위로 비켜 앉아 칸에서 잘린다.
        geometry: {
          x: PREVIEW_BOX.x + PREVIEW_BOX.w / 2,
          y: PREVIEW_BOX.y + PREVIEW_BOX.h / 2,
        },
        style: { ...style, align: 'center', fontSize: TEXT_PREVIEW_FONT_SIZE },
        text: TEXT_PREVIEW_SAMPLE,
      };
  }
}

/** 원시형 하나의 미리보기 요소. 도크가 제 칸에 실을 때 부른다. */
export function primitivePreviewElement(kind: CanvasPrimitiveKind): CanvasElement {
  return PRIMITIVE_PREVIEW_ELEMENTS[kind];
}
