// SVG 도면 자산 취급 — 종횡비 추출과 "늘려서 채우기" 재작성.
//
// 왜 필요한가: SVG 는 자기 문서 안에 `preserveAspectRatio`(기본 `xMidYMid meet`)를 갖고 있고,
// 그 규칙이 CSS 를 이긴다. `<img>` 상자에 `object-fit: fill` 을 걸어 상자를 늘려도 **그림은
// 비율을 지키며 상자 안에서 다시 레터박스**된다 — 보고된 "늘려서 채우기가 안 먹는다" 의 정체다.
// 같은 이유로 sizeless SVG 는 `naturalWidth/Height` 가 원본 크기가 아니라 대체 요소 기본값
// (브라우저에 따라 0 또는 300×150)이라, 스테이지 종횡비도 엉뚱해지거나 아예 못 구한다.
//
// 해결: **SVG 문서 자체를 고쳐서** 쓴다.
//   - 종횡비: `viewBox`(없으면 width/height 속성)에서 직접 읽는다 — 브라우저 보고값보다 정확하다.
//   - 늘려서 채우기: 루트 `<svg>` 의 `preserveAspectRatio` 를 `none` 으로 바꿔 다시 data-URL 로 만든다.
//
// 인라인(dangerouslySetInnerHTML)이 아니라 **data-URL 재작성**인 것이 요점이다. `<img>` 로 그리면
// SVG 안의 스크립트가 실행되지 않는다 — 사용자가 올린 도면을 인라인하면 그 격리가 사라진다.

/** SVG data-URL 인지. 자산 조회 결과도 data-URL 이므로 이 판정 하나로 두 경로를 덮는다. */
export function isSvgDataUrl(src: string): boolean {
  return /^data:image\/svg\+xml[;,]/i.test(src);
}

/**
 * SVG data-URL 을 문서 텍스트로 되돌린다. base64 와 URL 인코딩 두 형태를 모두 받는다.
 * 디코드에 실패하면 undefined — 호출부는 원본 src 를 그대로 쓰면 되므로 예외를 던지지 않는다.
 */
export function decodeSvgDataUrl(src: string): string | undefined {
  if (!isSvgDataUrl(src)) return undefined;
  const comma = src.indexOf(',');
  if (comma < 0) return undefined;
  const meta = src.slice(0, comma);
  const payload = src.slice(comma + 1);
  try {
    if (/;base64$/i.test(meta)) {
      if (typeof atob !== 'function') return undefined;
      // atob 는 latin1 바이트를 준다 — 한글 라벨이 든 도면이 깨지지 않도록 UTF-8 로 되돌린다.
      const bytes = Uint8Array.from(atob(payload), (c) => c.charCodeAt(0));
      return new TextDecoder().decode(bytes);
    }
    return decodeURIComponent(payload);
  } catch {
    return undefined;
  }
}

/** SVG 문서 텍스트를 data-URL 로 만든다(그리기는 계속 `<img>` 가 하므로 스크립트는 실행되지 않는다). */
export function encodeSvgDataUrl(text: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}`;
}

/** 루트 `<svg ...>` 여는 태그를 찾는다. 없으면 null. */
function rootSvgTag(text: string): { start: number; end: number; tag: string } | null {
  const m = /<svg\b[^>]*>/i.exec(text);
  if (!m) return null;
  return { start: m.index, end: m.index + m[0].length, tag: m[0] };
}

/** 여는 태그에서 속성 값을 읽는다(따옴표 양식 모두). */
function attr(tag: string, name: string): string | undefined {
  const m = new RegExp(`\\b${name}\\s*=\\s*("([^"]*)"|'([^']*)')`, 'i').exec(tag);
  return m ? (m[2] ?? m[3]) : undefined;
}

/** 단위가 붙은 길이("1024", "1024px")를 숫자로. 퍼센트/상대 단위는 크기가 아니므로 버린다. */
function lengthOf(raw: string | undefined): number | undefined {
  if (raw === undefined) return undefined;
  const m = /^\s*([0-9]*\.?[0-9]+)\s*(px)?\s*$/i.exec(raw);
  if (!m) return undefined;
  const n = Number(m[1]);
  return Number.isFinite(n) && n > 0 ? n : undefined;
}

/**
 * SVG 문서의 원본 크기(폭/높이). `viewBox` 를 먼저 본다 — width/height 속성은 없거나 `100%`
 * 인 경우가 흔하고, 그런 문서에서 브라우저는 대체 요소 기본값을 보고해 종횡비가 틀어진다.
 * 둘 다 못 읽으면 undefined(호출부가 브라우저 보고값으로 폴백한다).
 */
export function svgIntrinsicSize(text: string): { width: number; height: number } | undefined {
  const root = rootSvgTag(text);
  if (!root) return undefined;
  const viewBox = attr(root.tag, 'viewBox');
  if (viewBox) {
    const parts = viewBox.trim().split(/[\s,]+/).map(Number);
    if (parts.length === 4 && parts.every((n) => Number.isFinite(n))) {
      const [, , w, h] = parts as [number, number, number, number];
      if (w > 0 && h > 0) return { width: w, height: h };
    }
  }
  const w = lengthOf(attr(root.tag, 'width'));
  const h = lengthOf(attr(root.tag, 'height'));
  return w !== undefined && h !== undefined ? { width: w, height: h } : undefined;
}

/**
 * 루트 `<svg>` 의 `preserveAspectRatio` 를 `none` 으로 만든다(있으면 교체, 없으면 추가).
 * 이 속성이 `none` 이면 그림이 뷰포트를 **비등방으로** 채운다 = 늘려서 채우기.
 */
export function withStretchedAspect(text: string): string {
  const root = rootSvgTag(text);
  if (!root) return text;
  const existing = /\bpreserveAspectRatio\s*=\s*("[^"]*"|'[^']*')/i;
  const tag = existing.test(root.tag)
    ? root.tag.replace(existing, 'preserveAspectRatio="none"')
    : root.tag.replace(/^<svg\b/i, '<svg preserveAspectRatio="none"');
  return text.slice(0, root.start) + tag + text.slice(root.end);
}

/**
 * 늘려서 채우기용 src. SVG 면 `preserveAspectRatio="none"` 을 박은 새 data-URL 을, 아니면
 * undefined 를 돌려준다(래스터는 `object-fit: fill` 만으로 늘어나므로 손댈 것이 없다).
 */
export function stretchedSvgSrc(src: string): string | undefined {
  const text = decodeSvgDataUrl(src);
  if (text === undefined) return undefined;
  const stretched = withStretchedAspect(text);
  return stretched === text ? undefined : encodeSvgDataUrl(stretched);
}

/** src 가 SVG data-URL 이면 그 문서에서 읽은 원본 크기. 아니면 undefined. */
export function svgSrcIntrinsicSize(
  src: string,
): { width: number; height: number } | undefined {
  const text = decodeSvgDataUrl(src);
  return text === undefined ? undefined : svgIntrinsicSize(text);
}
