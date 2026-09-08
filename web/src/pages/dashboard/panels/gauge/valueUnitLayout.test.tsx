// 게이지 가운데의 **값 + 단위** 배치.
//
// 한 행에 이어 붙이면 단위가 길어질수록(`10.99KB/s`) 숫자가 밀려 작아지고, 좁은 타일에서는
// 줄바꿈이 값과 단위를 임의의 자리에서 끊는다. 행을 나누면 숫자 크기가 단위 길이에
// 영향받지 않는다.

import { describe, expect, it } from 'vitest';
import type { ReactElement } from 'react';
import { render } from '@testing-library/react';

import { AUTO_BYTES_RATE_UNIT } from '../charts/unitOptions';
import { parseConfig, renderGaugeByType } from './gaugeShapes';
import { GaugeValueOverlay } from './gaugeValue';

/** 값 + 단위를 그리는 유형(반원 RB 는 단위를 그리지 않는다). */
const VALUE_UNIT_TYPES = ['simple', 'half', 'needle', 'needle-rainbow', 'vertical-bar'] as const;

/** 값 텍스트 요소의 tspan 들을 `내용@dy` 로 뽑는다. */
function valueSpans(config: Record<string, unknown>): string[] {
  const view = render(renderGaugeWithValue(parseConfig(config), true));
  // 값 텍스트는 tspan 을 갖는 유일한 text 요소다(눈금 라벨은 tspan 이 없다).
  const el = Array.from(view.container.querySelectorAll('text')).find(
    (t) => t.querySelectorAll('tspan').length > 0,
  );
  const out = Array.from(el?.querySelectorAll('tspan') ?? []).map(
    (sp) => `${sp.textContent}@${sp.getAttribute('dy') ?? '-'}`,
  );
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

describe('값과 단위는 두 행이다', () => {
  for (const gaugeType of VALUE_UNIT_TYPES) {
    it(`${gaugeType}: 값과 단위가 각각 한 행씩`, () => {
      const spans = valueSpans({
        value: 11252.8,
        gaugeType,
        min: 0,
        max: 100000,
        unit: AUTO_BYTES_RATE_UNIT,
      });
      expect(spans).toHaveLength(2);
      // 둘째 행은 dy 로 아래에 쌓인다 — 같은 행에 이어 붙은 것이 아니다.
      expect(spans[0]).toBe('10.99@-');
      expect(spans[1]).toMatch(/^KB\/s@[\d.]+$/);
    });

    it(`${gaugeType}: 두 행 모두 같은 x 에서 시작한다 — 가운데 정렬이 유지된다`, () => {
      const view = render(
        renderGaugeWithValue(
          parseConfig({ value: 50, gaugeType, unit: '%' }),
          true,
        ),
      );
      const el = Array.from(view.container.querySelectorAll('text')).find(
        (t) => t.querySelectorAll('tspan').length > 0,
      );
      const xs = Array.from(el?.querySelectorAll('tspan') ?? []).map((sp) => sp.getAttribute('x'));
      view.unmount();
      expect(new Set(xs).size).toBe(1);
      expect(xs[0]).not.toBeNull();
    });

    it(`${gaugeType}: 단위가 없으면 둘째 행을 만들지 않는다`, () => {
      // 빈 행이 값을 위로 밀어 올린다.
      expect(valueSpans({ value: 50, gaugeType, unit: '' })).toHaveLength(1);
    });

    it(`${gaugeType}: 값이 없으면 한 행이다`, () => {
      const view = render(
        renderGaugeWithValue(parseConfig({ gaugeType, unit: '%' }), false),
      );
      const el = Array.from(view.container.querySelectorAll('text')).find(
        (t) => t.querySelectorAll('tspan').length > 0,
      );
      const spans = Array.from(el?.querySelectorAll('tspan') ?? []).map((sp) => sp.textContent);
      view.unmount();
      expect(spans).toEqual(['--']);
    });
  }
});

describe('눈금에는 단위가 없다', () => {
  for (const gaugeType of ['half', 'needle', 'needle-rainbow', 'vertical-bar'] as const) {
    it(`${gaugeType}: 눈금 라벨에 단위 글자가 없다`, () => {
      const view = render(
        renderGaugeWithValue(
          parseConfig({
            value: 11252.8,
            gaugeType,
            min: 0,
            max: 100000,
            unit: AUTO_BYTES_RATE_UNIT,
          }),
          true,
        ),
      );
      // 눈금 라벨은 tspan 이 없는 text 요소다.
      const ticks = Array.from(view.container.querySelectorAll('text'))
        .filter((t) => t.querySelectorAll('tspan').length === 0)
        .map((t) => t.textContent ?? '');
      view.unmount();
      expect(ticks.length).toBeGreaterThan(0);
      for (const label of ticks) {
        expect(label, `눈금 "${label}" 에 단위가 붙어 있다`).not.toMatch(/[A-Za-z]|\//);
      }
    });
  }
});
