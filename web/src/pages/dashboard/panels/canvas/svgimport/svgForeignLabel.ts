// draw.io 이름표의 순수 산술 — HTML 공백 접기 · 흐름 정렬 · 이름표 상자 (SPEC-CANVAS-007 M16).
//
// **이 모듈은 DOM 을 모른다**(불변식 K7). 문서 층이 `<foreignObject>` 하위 트리에서 긁어 온
// **문자열과 속성 자루**만 받는다. `svgText.ts` 가 SVG `<text>` 에 대해 선 그 자리에 이
// 파일이 XHTML 이름표에 대해 선다 — 두 파일이 갈린 이유는 **공백 규칙이 서로 다르기
// 때문**이고, 그 다름이 이 모듈의 존재 이유 전부다.
//
// **SVG 의 `xml:space="default"` 와 CSS 의 `white-space: normal` 은 같은 규칙이 아니다.**
//   - SVG(`normalizeSvgText`): 줄바꿈을 **지운다**. `"가\n나"` → `"가나"`.
//   - CSS(`collapseHtmlText`): 줄바꿈을 **공백으로 접는다**. `"가\n나"` → `"가 나"`.
//   draw.io 는 이름표를 XHTML 로 내보내므로 뒤의 규칙이 맞다. 앞의 함수를 재사용하면 예쁘게
//   찍어 낸 `<div>` 의 들여쓰기가 **낱말을 붙여** `"표시 필드"` 가 `"표시필드"` 로 들어온다.
//
// **U+00A0 은 공백이 아니다** — 그것이 이 파일에서 가장 틀리기 쉬운 자리다. CSS 는
// non-breaking space 를 **접지도 떼지도 않고** 보통 글자로 둔다. 그래서 여기서는 `\s` 도
// `String.prototype.trim()` 도 쓸 수 없다: 둘 다 U+00A0 을 공백으로 세어(실측) `"X: "` 의
// 꼬리를 떼어 버린다. 문자 무리를 손으로 적는 것이 게으름의 반대인 자리다.
//
// **엔티티를 풀지 않는다 — 풀 자리가 없다.** `DOMParser` 가 `image/svg+xml` 로 읽으므로
// 문서는 XML 이고, XML 이 미리 정한 엔티티는 다섯뿐이다(`lt` `gt` `amp` `apos` `quot`).
// 맨 `&nbsp;` 는 **문서를 통째로 죽인다** — 실측: `1:88: undefined entity.` 가 나고 루트가
// `parsererror` 가 되어 `readSvgDocument` 가 `notSvg` 로 거절한다. 그래서 이 층에 닿을 수
// 있는 꼴은 `&#160;` 과 리터럴 U+00A0 뿐이고, **그 둘은 파서가 이미 풀어 준다**(실측).
// 여기에 엔티티 표를 두면 **한 번도 돌지 않는 코드**가 되고, 돌지 않는 코드는 틀려도
// 빨개지지 않는다.
//
// @spec SPEC-CANVAS-007 REQ-04 · REQ-05

import { parseLength } from './svgPathData';
import type { AttrBag } from './svgShapes';

/**
 * CSS 가 **접을 수 있는** 공백. `\s` 를 쓰지 않는 이유가 이 무리에 있다 — `\s` 는 U+00A0 과
 * U+2003 따위 조판 공백까지 삼키는데, CSS 는 그것들을 보통 글자로 두고 접지 않는다.
 */
const CSS_COLLAPSIBLE = /[ \t\n\r\f]+/g;

/** 같은 무리로 앞뒤를 뗀다. `trim()` 이 아닌 이유는 위와 같다(그 함수는 U+00A0 을 뗀다). */
const CSS_LEADING = /^[ \t\n\r\f]+/;
const CSS_TRAILING = /[ \t\n\r\f]+$/;

/**
 * `white-space: normal` 규칙으로 이름표를 한 줄로 편다.
 *
 * `<br>` 이 넣어 둔 줄바꿈도 여기서 **공백 하나가 된다**. 문구 요소는 한 점 위에 서는 한
 * 줄이므로(§결정 10) 두 줄을 나를 자리가 없고, 줄바꿈을 **지우면** 낱말이 붙는다 —
 * 공백으로 접는 쪽이 원본에서 눈이 본 것에 가깝다.
 */
export function collapseHtmlText(raw: string): string {
  return raw.replace(CSS_COLLAPSIBLE, ' ').replace(CSS_LEADING, '').replace(CSS_TRAILING, '');
}

/**
 * 이름표의 가로 정렬 — **`text-anchor` 값으로 낸다**.
 *
 * `ElementAlign` 을 바로 내지 않는 것이 요점이다. `start` 를 config 에 적지 않는 규율은
 * `svgText.alignFromTextAnchor` 가 이미 소유하고 있고(렌더 기본이 `'left'` 이므로 "지정
 * 안 함" 과 "우연히 기본값" 을 가른다), 여기서 `ElementAlign` 을 내면 그 규율이 두 벌이
 * 된다. SVG 의 어휘로 말해 두면 문서 층이 그 함수를 **그대로** 지난다.
 *
 * **`justify-content` 가 `text-align` 을 이긴다.** 앞은 이름표 덩이를 상자 안 어디에 놓을지
 * 를 정하고 뒤는 그 덩이 안에서 줄을 어디에 놓을지를 정하는데, 우리가 옮기는 것은 **덩이의
 * 자리**다. draw.io 는 둘을 함께 내보내며 값이 서로 맞으므로 실사용에서 갈리지 않지만,
 * 갈리는 문서에서 상자를 따르는 쪽이 그림에 가깝다.
 *
 * `unsafe`/`safe` 접두는 넘침이 생겼을 때의 거동을 정할 뿐 정렬 자체가 아니므로 떼어 읽는다
 * (draw.io 가 실제로 `unsafe center` 를 내보낸다).
 */
export function textAnchorFromHtmlFlow(
  justifyContent: string | undefined,
  textAlign: string | undefined,
): 'start' | 'middle' | 'end' {
  return anchorFromJustify(justifyContent) ?? anchorFromTextAlign(textAlign) ?? 'start';
}

/** `justify-content` 한 값 → `text-anchor`. 읽지 못하는 값(`space-between` 등)은 `undefined`. */
function anchorFromJustify(raw: string | undefined): 'start' | 'middle' | 'end' | undefined {
  const words = (raw ?? '').trim().toLowerCase().split(/\s+/);
  // `unsafe`/`safe` 는 넘침 거동이지 정렬이 아니다 — 떼고 남은 마디를 읽는다.
  const value = words[words.length - 1];
  if (value === 'center') return 'middle';
  if (value === 'flex-start' || value === 'start' || value === 'left') return 'start';
  if (value === 'flex-end' || value === 'end' || value === 'right') return 'end';
  return undefined;
}

/** `text-align` 한 값 → `text-anchor`. `justify` 는 한 줄에서 `start` 와 같다. */
function anchorFromTextAlign(raw: string | undefined): 'start' | 'middle' | 'end' | undefined {
  const value = raw?.trim().toLowerCase();
  if (value === 'center') return 'middle';
  if (value === 'left' || value === 'start' || value === 'justify') return 'start';
  if (value === 'right' || value === 'end') return 'end';
  return undefined;
}

/**
 * 이름표 상자 — draw.io 가 `<switch>` 안에 함께 내보내는 래스터 대안(`<image>`)의 자리.
 *
 * 좌표는 `<foreignObject>` 안의 CSS px 와 **같은 단계**다(그 요소가 `width="100%"
 * height="100%"` 이고 `x`/`y` 가 없으므로 CSS px 와 사용자 단위가 1:1 이다 — draw.io 가
 * 실제로 내보내는 꼴). 조상 `transform` 은 아직 녹지 않았다.
 */
export interface ForeignLabelBox {
  readonly minX: number;
  readonly minY: number;
  readonly width: number;
  readonly height: number;
}

/**
 * `<image>` 속성 넷 → 이름표 상자. **넷이 모두 읽히고 두 치수가 양수일 때만** 성립한다
 * (`readSize` 와 같은 규율 — 길이를 읽는 두 번째 규칙을 만들지 않는다).
 *
 * 퇴화한 상자(치수 0)를 받지 않는 것에 뜻이 있다: 그 상자에서는 세로 가운데가 위 변과 같아
 * 져 기준점이 글자 **위**에 서는데, 그것은 문서가 말한 자리가 아니라 우리가 고른 자리다.
 * 받지 못하면 이름표를 세우지 않고, 그 사실은 호출부가 `unenteredContainerDropped` 로 말한다.
 */
export function readForeignLabelBox(attrs: AttrBag): ForeignLabelBox | undefined {
  const minX = parseLength(attrs['x']);
  const minY = parseLength(attrs['y']);
  const width = parseLength(attrs['width']);
  const height = parseLength(attrs['height']);
  if (minX === undefined || minY === undefined || width === undefined || height === undefined) {
    return undefined;
  }
  if (!(width > 0) || !(height > 0)) return undefined;
  return { minX, minY, width, height };
}

/**
 * 상자와 정렬 → **기준점의 가로 좌표**.
 *
 * 문구 요소는 상자를 갖지 않고 한 점 위에 서므로(§결정 10 · `ImportedText`), 상자는 여기서
 * 점으로 접힌다. 어느 점이냐는 정렬이 정한다 — 렌더 층이 `resolveTextOrigin` 에서 같은
 * 규칙으로 다시 펴기 때문에(`center` 는 폭의 절반을 빼고 `end` 는 폭을 뺀다), 상자의 그
 * 자리를 그대로 주면 글자가 원본과 같은 칸에 앉는다.
 */
export function anchorXInBox(
  anchor: 'start' | 'middle' | 'end',
  box: ForeignLabelBox,
): number {
  if (anchor === 'middle') return box.minX + box.width / 2;
  if (anchor === 'end') return box.minX + box.width;
  return box.minX;
}

/**
 * 기준점의 세로 좌표 — **상자의 가운데**.
 *
 * 이 패널이 문구를 `textBaseline = 'middle'` 로 그리므로(실측 `drawElement.TEXT_BASELINE`)
 * 기준점의 뜻이 곧 "글줄의 세로 가운데" 다. 상자의 위 변을 주면 이름표가 제자리보다 반 줄
 * 내려앉는다.
 */
export function anchorYInBox(box: ForeignLabelBox): number {
  return box.minY + box.height / 2;
}
