// @spec SPEC-COLOR-001 §명세 · 팝오버 배치 (M4) — AC-E10 · 함정 E5
//
// 함정 E5: jsdom 에 레이아웃이 없어 팝오버 배치를 컴포넌트로는 못 잰다. 그래서
// 산술을 순수 함수로 빼서 **직접 부른다.** 이 파일이 그 호출이다.

import { describe, expect, it } from 'vitest';

import {
  placePopover,
  POPOVER_FALLBACK_HEIGHT,
  POPOVER_GAP,
  POPOVER_MARGIN,
  POPOVER_WIDTH,
} from './popoverPlacement';

const VIEWPORT = 800;

/** 화면 한가운데의 평범한 트리거. */
function trigger(top: number, right = 500, height = 24) {
  return { top, bottom: top + height, right };
}

describe('아래로 여는 기본 갈래', () => {
  it('트리거 바로 아래 틈만큼 띄운다', () => {
    const p = placePopover({
      trigger: trigger(100),
      popoverHeight: 200,
      viewportHeight: VIEWPORT,
    });
    expect(p.flipped).toBe(false);
    expect(p.top).toBe(124 + POPOVER_GAP);
  });

  it('우측 정렬이다', () => {
    const p = placePopover({
      trigger: trigger(100, 500),
      popoverHeight: 200,
      viewportHeight: VIEWPORT,
    });
    expect(p.left).toBe(500 - POPOVER_WIDTH);
  });
});

describe('뒤집기 경계', () => {
  it('화면 아래 끝이면 위로 뒤집는다', () => {
    const p = placePopover({
      trigger: trigger(700),
      popoverHeight: 200,
      viewportHeight: VIEWPORT,
    });
    expect(p.flipped).toBe(true);
  });

  it('딱 들어맞으면 뒤집지 않는다 — 경계 바로 아래', () => {
    // below + h === viewportHeight - MARGIN 이면 넘치지 않는다(엄격 부등호).
    const top = 100;
    const below = top + 24 + POPOVER_GAP;
    const h = VIEWPORT - POPOVER_MARGIN - below;
    expect(
      placePopover({ trigger: trigger(top), popoverHeight: h, viewportHeight: VIEWPORT }).flipped,
    ).toBe(false);
    // 한 픽셀만 더 크면 뒤집는다.
    expect(
      placePopover({ trigger: trigger(top), popoverHeight: h + 1, viewportHeight: VIEWPORT })
        .flipped,
    ).toBe(true);
  });

  it('뒤집어도 화면 위로 넘치지 않는다 — 여백까지만', () => {
    const p = placePopover({
      trigger: trigger(10),
      popoverHeight: 600,
      viewportHeight: 300,
    });
    expect(p.flipped).toBe(true);
    expect(p.top).toBe(POPOVER_MARGIN);
  });
});

describe('좌우 가두기', () => {
  it('왼쪽 끝이면 여백 아래로 내려가지 않는다', () => {
    const p = placePopover({
      trigger: trigger(100, 20),
      popoverHeight: 200,
      viewportHeight: VIEWPORT,
    });
    expect(p.left).toBe(POPOVER_MARGIN);
  });

  it('가두기 경계 — `right - WIDTH` 가 정확히 여백이면 그 값을 쓴다', () => {
    const p = placePopover({
      trigger: trigger(100, POPOVER_WIDTH + POPOVER_MARGIN),
      popoverHeight: 200,
      viewportHeight: VIEWPORT,
    });
    expect(p.left).toBe(POPOVER_MARGIN);
  });
});

describe('측정 높이가 0 일 때 (jsdom)', () => {
  it('폴백 높이를 쓴다 — 0 은 "높이 0" 이 아니라 "재지 못함" 이다', () => {
    const withZero = placePopover({
      trigger: trigger(600),
      popoverHeight: 0,
      viewportHeight: VIEWPORT,
    });
    const withFallback = placePopover({
      trigger: trigger(600),
      popoverHeight: POPOVER_FALLBACK_HEIGHT,
      viewportHeight: VIEWPORT,
    });
    expect(withZero).toEqual(withFallback);
  });

  it('0 을 그대로 믿었다면 뒤집지 않았을 자리에서 뒤집는다', () => {
    // 높이를 0 으로 치면 `below + 0` 은 절대 안 넘치므로 뒤집기 판정이 죽는다.
    const p = placePopover({ trigger: trigger(600), popoverHeight: 0, viewportHeight: VIEWPORT });
    expect(p.flipped).toBe(true);
  });
});
