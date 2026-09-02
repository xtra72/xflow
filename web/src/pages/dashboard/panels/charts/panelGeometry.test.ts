// panelGeometry 순수 모듈 테스트 — 파이·게이지가 공유하는 크기·자리 규칙.

import { describe, it, expect } from 'vitest';

import {
  clampPercentOffset,
  PANEL_OFFSET_LIMIT,
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
  panelBoxTransform,
  pixelsToPercent,
  readPanelOffset,
  readPanelSize,
} from './panelGeometry';

describe('pixelsToPercent', () => {
  it('기준 변 대비 백분율로 환산한다 — 패널 크기가 바뀌어도 상대 위치가 유지된다', () => {
    expect(pixelsToPercent(40, 400)).toBe(10);
    expect(pixelsToPercent(-40, 400)).toBe(-10);
  });

  it('기준 변을 잴 수 없으면 0 — 0으로 나눈 Infinity 를 오프셋에 더하지 않는다', () => {
    expect(pixelsToPercent(40, 0)).toBe(0);
    expect(pixelsToPercent(40, Number.NaN)).toBe(0);
  });
});

describe('clampPercentOffset', () => {
  it('상한을 넘기면 죈다 — 파이가 절반 넘게 잘리지 않도록', () => {
    expect(clampPercentOffset(90)).toBe(PANEL_OFFSET_LIMIT);
    expect(clampPercentOffset(-90)).toBe(-PANEL_OFFSET_LIMIT);
  });

  it('범위 안이면 그대로 둔다', () => {
    expect(clampPercentOffset(12)).toBe(12);
  });

  it('수가 아니면 0', () => {
    expect(clampPercentOffset(Number.NaN)).toBe(0);
  });
});

describe('readPanelSize', () => {
  it('허용 범위 안의 수만 받는다 — 그림이 사라지거나 영역 밖으로 나가지 않게', () => {
    expect(readPanelSize(45)).toBe(45);
    expect(readPanelSize(PANEL_SIZE_MIN)).toBe(PANEL_SIZE_MIN);
    expect(readPanelSize(PANEL_SIZE_MAX)).toBe(PANEL_SIZE_MAX);
  });

  it('범위 밖·수가 아닌 값은 미지정 — 자동으로 돌아간다', () => {
    expect(readPanelSize(0)).toBeUndefined();
    expect(readPanelSize(-10)).toBeUndefined();
    expect(readPanelSize(150)).toBeUndefined();
    expect(readPanelSize('80')).toBeUndefined();
  });
});

describe('readPanelOffset', () => {
  it('읽으면서 상한으로 죈다 — 손으로 고친 config 가 그림을 날리지 않게', () => {
    expect(readPanelOffset(12)).toBe(12);
    expect(readPanelOffset(999)).toBe(PANEL_OFFSET_LIMIT);
    expect(readPanelOffset(undefined)).toBe(0);
  });
});

describe('panelBoxTransform', () => {
  it('기본값이면 transform 을 남기지 않는다 — 불필요한 레이어를 만들지 않는다', () => {
    expect(panelBoxTransform(100, 0, 0)).toBeUndefined();
  });

  it('옮긴 뒤 키운다 — 오프셋 백분율이 크기에 휘둘리지 않는 순서', () => {
    expect(panelBoxTransform(50, 10, -5)).toBe('translate(10%, -5%) scale(0.5)');
  });

  it('크기만 바뀌어도 transform 을 만든다', () => {
    expect(panelBoxTransform(60, 0, 0)).toBe('translate(0%, 0%) scale(0.6)');
  });
});
