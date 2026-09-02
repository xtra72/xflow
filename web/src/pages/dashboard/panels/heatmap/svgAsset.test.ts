// SVG 도면 자산 취급 테스트.
//
// 두 결함을 막는다.
//   1) 늘려서 채우기가 SVG 에서 안 먹는다 — SVG 문서의 `preserveAspectRatio` 가 CSS 를 이긴다.
//   2) sizeless SVG 의 종횡비를 브라우저 보고값(대체 요소 기본 크기)으로 잘못 잡는다.

import { describe, it, expect } from 'vitest';

import {
  decodeSvgDataUrl,
  encodeSvgDataUrl,
  isSvgDataUrl,
  stretchedSvgSrc,
  svgIntrinsicSize,
  svgSrcIntrinsicSize,
  withStretchedAspect,
} from './svgAsset';

/** URL 인코딩 형태의 SVG data-URL. */
function urlSvg(body: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(body)}`;
}

/** base64 형태의 SVG data-URL(UTF-8 바이트 기준). */
function b64Svg(body: string): string {
  const bytes = new TextEncoder().encode(body);
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return `data:image/svg+xml;base64,${btoa(bin)}`;
}

describe('svgAsset — 판정과 디코드', () => {
  it('SVG data-URL 만 SVG 로 본다', () => {
    expect(isSvgDataUrl('data:image/svg+xml;base64,AAAA')).toBe(true);
    expect(isSvgDataUrl('data:image/svg+xml,%3Csvg%3E')).toBe(true);
    expect(isSvgDataUrl('data:image/png;base64,AAAA')).toBe(false);
    expect(isSvgDataUrl('https://x/y.svg')).toBe(false);
  });

  it('URL 인코딩·base64 두 형태를 모두 되돌린다', () => {
    const doc = '<svg viewBox="0 0 10 5"><title>도면</title></svg>';
    expect(decodeSvgDataUrl(urlSvg(doc))).toBe(doc);
    // 한글이 든 문서도 깨지지 않는다(atob 의 latin1 바이트를 UTF-8 로 되돌린다).
    expect(decodeSvgDataUrl(b64Svg(doc))).toBe(doc);
  });

  it('SVG 가 아니거나 손상된 입력은 undefined 로 떨어진다(예외 없음)', () => {
    expect(decodeSvgDataUrl('data:image/png;base64,AAAA')).toBeUndefined();
    expect(decodeSvgDataUrl('data:image/svg+xml;base64,!!!not-base64!!!')).toBeUndefined();
  });
});

describe('svgAsset — 원본 크기', () => {
  it('viewBox 를 우선한다(width/height 가 100% 여도 정확한 비율을 얻는다)', () => {
    const doc = '<svg width="100%" height="100%" viewBox="0 0 1200 400"></svg>';
    expect(svgIntrinsicSize(doc)).toEqual({ width: 1200, height: 400 });
  });

  it('viewBox 가 없으면 width/height 속성을 쓴다(px 단위 허용)', () => {
    expect(svgIntrinsicSize('<svg width="800" height="400"></svg>')).toEqual({
      width: 800,
      height: 400,
    });
    expect(svgIntrinsicSize('<svg width="800px" height="400px"></svg>')).toEqual({
      width: 800,
      height: 400,
    });
  });

  it('크기를 알 수 없으면 undefined(호출부가 브라우저 보고값으로 폴백한다)', () => {
    expect(svgIntrinsicSize('<svg></svg>')).toBeUndefined();
    expect(svgIntrinsicSize('<svg width="100%" height="50%"></svg>')).toBeUndefined();
    expect(svgIntrinsicSize('not svg at all')).toBeUndefined();
  });

  it('src 에서 바로 읽는다', () => {
    expect(svgSrcIntrinsicSize(urlSvg('<svg viewBox="0 0 30 10"></svg>'))).toEqual({
      width: 30,
      height: 10,
    });
    expect(svgSrcIntrinsicSize('data:image/png;base64,AAAA')).toBeUndefined();
  });
});

describe('svgAsset — 늘려서 채우기 재작성', () => {
  it('preserveAspectRatio 가 없으면 none 을 박는다', () => {
    const out = withStretchedAspect('<svg viewBox="0 0 10 5"><rect /></svg>');
    expect(out).toContain('preserveAspectRatio="none"');
    // 나머지 문서는 그대로다(그림을 건드리지 않는다).
    expect(out).toContain('<rect />');
    expect(out).toContain('viewBox="0 0 10 5"');
  });

  it('이미 있으면 값을 none 으로 갈아 끼운다(중복 속성을 만들지 않는다)', () => {
    const out = withStretchedAspect('<svg preserveAspectRatio="xMidYMid meet" viewBox="0 0 10 5"></svg>');
    expect(out).toContain('preserveAspectRatio="none"');
    expect(out).not.toContain('xMidYMid');
    expect(out.match(/preserveAspectRatio/g)).toHaveLength(1);
  });

  it('루트 태그가 아닌 곳은 건드리지 않는다', () => {
    const out = withStretchedAspect(
      '<svg viewBox="0 0 10 5"><svg preserveAspectRatio="xMidYMid meet"></svg></svg>',
    );
    // 안쪽 중첩 svg 의 값은 그대로 남는다(루트만 바꾼다).
    expect(out).toContain('xMidYMid meet');
  });

  it('stretchedSvgSrc 는 SVG 만 재작성하고 래스터는 손대지 않는다', () => {
    const src = urlSvg('<svg viewBox="0 0 10 5"></svg>');
    const out = stretchedSvgSrc(src);
    expect(out).toBeDefined();
    expect(decodeSvgDataUrl(out!)).toContain('preserveAspectRatio="none"');
    // 래스터는 object-fit: fill 만으로 늘어나므로 재작성 대상이 아니다.
    expect(stretchedSvgSrc('data:image/png;base64,AAAA')).toBeUndefined();
  });

  it('encode/decode 왕복이 문서를 보존한다', () => {
    const doc = '<svg viewBox="0 0 10 5"><text>가나다</text></svg>';
    expect(decodeSvgDataUrl(encodeSvgDataUrl(doc))).toBe(doc);
  });
});
