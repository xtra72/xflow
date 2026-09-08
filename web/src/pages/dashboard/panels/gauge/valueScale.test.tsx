// 현재값 크기 배율과 위치 변위.
//
// 유형마다 기본 글자 크기와 자리가 다르다(도넛 28 / 바늘 10 …). 그 값을 통째로 설정에
// 노출하면 유형을 바꿀 때마다 다시 잡아야 하므로 **배율과 변위**로 둔다 — 유형을 바꿔도
// "조금 크게, 조금 위로" 라는 뜻이 유지된다.

import { describe, expect, it } from 'vitest';
import type { ReactElement } from 'react';
import { render } from '@testing-library/react';

import {
  readValueOffset,
  readValueScale,
  VALUE_SCALE_MAX,
  VALUE_SCALE_MIN,
} from '../charts/valueScale';
import { parseConfig, renderGaugeByType } from './gaugeShapes';
import { GaugeValueOverlay } from './gaugeValue';

const TYPES = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'vertical-bar',
  'half-rainbow',
] as const;

/** 값 텍스트의 좌표와 두 행의 글자 크기. */
function valueText(config: Record<string, unknown>): {
  x: number;
  y: number;
  sizes: number[];
} {
  const view = render(renderGaugeWithValue(parseConfig({ value: 50, unit: '%', ...config }), true));
  const el = Array.from(view.container.querySelectorAll('text')).find(
    (t) => t.querySelectorAll('tspan').length > 0,
  )!;
  const sizes = Array.from(el.querySelectorAll('tspan')).map((sp) =>
    Number(sp.getAttribute('font-size')),
  );
  const out = { x: Number(el.getAttribute('x')), y: Number(el.getAttribute('y')), sizes };
  view.unmount();
  return out;
}


/**
 * 도형과 값 오버레이를 **패널과 같은 구조**로 함께 그린다.
 *
 * 값은 더 이상 도형 SVG 안에 없다 — 도형에 걸린 크기·위치 변형을 따라가지 않도록
 * 형제 오버레이로 뺐다. 그래서 값 글자를 보려면 둘을 함께 그려야 한다.
 */
function renderGaugeWithValue(
  parsed: ReturnType<typeof parseConfig>,
  hasValue: boolean,
): ReactElement {
  return (
    <>
      {renderGaugeByType(parsed, hasValue)}
      <GaugeValueOverlay parsed={parsed} hasValue={hasValue} offsetX={0} offsetY={0} />
    </>
  );
}

describe('설정값 읽기', () => {
  it('미지정이면 배율 1 · 변위 0', () => {
    expect(readValueScale(undefined)).toBe(1);
    expect(readValueOffset(undefined)).toBe(0);
    expect(parseConfig({ value: 50 }).valueScale).toBe(1);
  });

  it('배율은 범위 안으로 죈다 — 손으로 편집한 config 가 글자를 날리지 않는다', () => {
    expect(readValueScale(100)).toBe(VALUE_SCALE_MAX);
    expect(readValueScale(0)).toBe(VALUE_SCALE_MIN);
    expect(readValueScale(-5)).toBe(VALUE_SCALE_MIN);
  });

  it('수가 아니면 기본값', () => {
    expect(readValueScale('2')).toBe(1);
    expect(readValueScale(Number.NaN)).toBe(1);
    expect(readValueOffset('10')).toBe(0);
    expect(readValueOffset(Number.POSITIVE_INFINITY)).toBe(0);
  });
});

describe('배율과 변위가 렌더에 닿는다', () => {
  for (const gaugeType of TYPES) {
    it(`${gaugeType}: 배율이 두 행 글자 크기에 함께 걸린다`, () => {
      const base = valueText({ gaugeType });
      const big = valueText({ gaugeType, value_scale: 2 });
      expect(big.sizes).toEqual(base.sizes.map((n) => n * 2));
    });

    // 변위는 더 이상 SVG 좌표를 옮기지 않는다 — 값이 도형 밖 오버레이가 되면서
    // **패널 대비 백분율**로 오버레이 상자를 옮긴다. 그래서 도형을 줄이거나 옮겨도
    // 값이 따라가지 않고, 캔버스 밖으로도 나갈 수 있다.
    it(`${gaugeType}: 변위는 오버레이 상자를 백분율로 옮긴다`, () => {
      const view = render(
        <GaugeValueOverlay
          parsed={parseConfig({ value: 50, unit: '%', gaugeType })}
          hasValue
          offsetX={12}
          offsetY={-7}
        />,
      );
      const box = view.container.querySelector('[data-testid="gauge-value-overlay"]') as HTMLElement;
      expect(box.style.transform).toBe('translate(12%, -7%)');
      // 좌표 자체는 기본 자리 그대로다 — 옮긴 것은 상자이지 글자가 아니다.
      expect(Number(view.container.querySelector('text[x]')?.getAttribute('x'))).toBe(
        valueText({ gaugeType }).x,
      );
      view.unmount();
    });
  }

  it('배율을 키워도 두 행 간격이 함께 늘어 겹치지 않는다', () => {
    const view = render(
      renderGaugeWithValue(parseConfig({ value: 50, unit: '%', value_scale: 2 }), true),
    );
    const spans = Array.from(view.container.querySelectorAll('tspan'));
    const dy = Number(spans[1]?.getAttribute('dy'));
    const unitSize = Number(spans[1]?.getAttribute('font-size'));
    view.unmount();
    // 행 간격은 글자 크기에 비례해야 한다 — 고정값이면 크게 키울 때 겹친다.
    expect(dy).toBeGreaterThan(unitSize);
  });
});
