// 현재값 드래그의 좌표 환산.
//
// 게이지는 viewBox 좌표로 그려지고 화면에는 패널 크기에 맞춰 축소·확대되어 나온다.
// 픽셀 이동량을 그대로 오프셋에 더하면 패널이 작을수록 값이 과하게 움직이고 클수록
// 굼뜨다. 환산이 틀리면 "조금 끌었는데 값이 화면 밖으로 날아간다" 로 나타나므로,
// 브라우저 없이 잠글 수 있게 순수 함수로 두고 여기서 검증한다.

import { describe, expect, it } from 'vitest';

import { clampOffset, parseViewBox, pixelsToViewBox } from './valueDrag';

describe('viewBox 읽기', () => {
  it('네 수를 읽어 폭·높이를 준다', () => {
    expect(parseViewBox('0 0 200 150')).toEqual({ w: 200, h: 150 });
  });

  it('쉼표·여러 공백도 받는다', () => {
    expect(parseViewBox('0,0, 240  140')).toEqual({ w: 240, h: 140 });
  });

  it('형식이 아니면 null — 환산을 시작하지 않는다', () => {
    for (const raw of [null, undefined, '', '0 0 200', '0 0 a b', '0 0 0 150', '0 0 200 0']) {
      expect(parseViewBox(raw)).toBeNull();
    }
  });
});

describe('픽셀 → viewBox 환산', () => {
  it('같은 배율이면 그대로 나눈다', () => {
    // 200 짜리 viewBox 가 400px 로 그려지면 배율 2 — 20px 은 10 단위다.
    expect(pixelsToViewBox(20, 40, { width: 400, height: 400 }, { w: 200, h: 200 })).toEqual({
      dx: 10,
      dy: 20,
    });
  });

  it('여백이 있는 축이 아니라 **작은 쪽 배율**을 쓴다', () => {
    // SVG 기본 preserveAspectRatio(meet)는 가로·세로 같은 배율로 축소한다.
    // 축마다 따로 계산하면 여백이 있는 축에서 이동량이 부풀려진다.
    // 400x800 상자에 200x200 viewBox → 배율 min(2, 4) = 2.
    expect(pixelsToViewBox(20, 20, { width: 400, height: 800 }, { w: 200, h: 200 })).toEqual({
      dx: 10,
      dy: 10,
    });
  });

  it('음수 이동도 그대로 환산한다', () => {
    expect(pixelsToViewBox(-20, -20, { width: 400, height: 400 }, { w: 200, h: 200 })).toEqual({
      dx: -10,
      dy: -10,
    });
  });

  it('크기를 잴 수 없으면 이동 없음 — 0 으로 나눈 무한대를 오프셋에 더하지 않는다', () => {
    expect(pixelsToViewBox(20, 20, { width: 0, height: 0 }, { w: 200, h: 200 })).toEqual({
      dx: 0,
      dy: 0,
    });
  });
});

describe('오프셋 죄기', () => {
  it('한 변의 절반을 넘지 않는다 — 끌어 옮긴 것을 다시 잡을 수 있어야 한다', () => {
    expect(clampOffset(500, 200)).toBe(100);
    expect(clampOffset(-500, 200)).toBe(-100);
  });

  it('범위 안이면 그대로 둔다', () => {
    expect(clampOffset(30, 200)).toBe(30);
    expect(clampOffset(0, 200)).toBe(0);
  });
});
