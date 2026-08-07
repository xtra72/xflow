// placement 순수 함수 단위 테스트 (SPEC-HEATMAP-PANEL-002 T5).
// 경계값 clamp(AC-E2/R5), 도면 밖 포인터 clamp, 왕복 변환(R4), 스냅 반올림/no-op 을 커버한다.

import { describe, it, expect } from 'vitest';

import {
  clamp01,
  toNormalized,
  fromNormalized,
  applySnap,
  type ContainerRect,
} from './placement';

const RECT: ContainerRect = { left: 100, top: 50, width: 200, height: 400 };

describe('clamp01', () => {
  it('범위 내 값은 그대로 통과한다', () => {
    expect(clamp01(0.42)).toBe(0.42);
  });

  it('경계(0/1)를 벗어난 값은 clamp 한다', () => {
    expect(clamp01(-0.3)).toBe(0);
    expect(clamp01(1.7)).toBe(1);
    expect(clamp01(0)).toBe(0);
    expect(clamp01(1)).toBe(1);
  });

  it('비유한(NaN/Infinity)은 0 으로 방어한다', () => {
    expect(clamp01(NaN)).toBe(0);
    expect(clamp01(Infinity)).toBe(0);
    expect(clamp01(-Infinity)).toBe(0);
  });
});

describe('toNormalized', () => {
  it('rect 내부 포인터를 0..1 로 변환한다', () => {
    // 중심(200, 250) → (0.5, 0.5).
    expect(toNormalized(200, 250, RECT)).toEqual({ x: 0.5, y: 0.5 });
    // 좌상단 모서리 → (0, 0).
    expect(toNormalized(100, 50, RECT)).toEqual({ x: 0, y: 0 });
    // 우하단 모서리 → (1, 1).
    expect(toNormalized(300, 450, RECT)).toEqual({ x: 1, y: 1 });
  });

  it('AC-E2: rect 밖 포인터는 [0,1] 로 clamp 한다(도면 밖 저장 금지)', () => {
    // 좌/상 밖(음수) → 0.
    expect(toNormalized(0, 0, RECT)).toEqual({ x: 0, y: 0 });
    // 우/하 밖(초과) → 1.
    expect(toNormalized(9999, 9999, RECT)).toEqual({ x: 1, y: 1 });
  });

  it('width/height 가 0 이면 0 으로 폴백한다(0 나눗셈 방어)', () => {
    expect(toNormalized(10, 10, { left: 0, top: 0, width: 0, height: 0 })).toEqual({
      x: 0,
      y: 0,
    });
  });
});

describe('fromNormalized', () => {
  it('정규화 좌표를 표시 크기 기준 픽셀 배치로 파생한다', () => {
    expect(fromNormalized({ x: 0.5, y: 0.25 }, { width: 200, height: 400 })).toEqual({
      left: 100,
      top: 100,
    });
  });

  it('toNormalized ↔ fromNormalized 왕복은 원 좌표를 보존한다', () => {
    const norm = toNormalized(240, 150, RECT); // (0.7, 0.25)
    const px = fromNormalized(norm, { width: RECT.width, height: RECT.height });
    // 다시 client 좌표로: left/top 오프셋을 더하면 원 포인터가 된다.
    expect(px.left + RECT.left).toBeCloseTo(240, 6);
    expect(px.top + RECT.top).toBeCloseTo(150, 6);
  });
});

describe('applySnap', () => {
  it('snap 이 없으면(undefined/0/음수) 원본 그대로 반환한다(no-op)', () => {
    const pos = { x: 0.37, y: 0.61 };
    expect(applySnap(pos)).toBe(pos);
    expect(applySnap(pos, 0)).toBe(pos);
    expect(applySnap(pos, -0.1)).toBe(pos);
  });

  it('유효 snap 은 가장 가까운 배수로 반올림하고 clamp 한다', () => {
    // snap 0.1: 0.37→0.4, 0.61→0.6 (부동소수 오차 대비 toBeCloseTo).
    const snapped = applySnap({ x: 0.37, y: 0.61 }, 0.1);
    expect(snapped.x).toBeCloseTo(0.4, 6);
    expect(snapped.y).toBeCloseTo(0.6, 6);
    // snap 0.25(이진수 정확): 0.1→0, 0.9→1.
    expect(applySnap({ x: 0.1, y: 0.9 }, 0.25)).toEqual({ x: 0, y: 1 });
  });
});
