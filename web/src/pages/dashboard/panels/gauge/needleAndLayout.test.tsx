// 니들 색 설정과 값 텍스트가 캔버스 안에 들어가는가.
//
// 보고된 두 결함:
//   - 세로 바: 값이 두 행이 되면서 둘째 행(단위)이 캔버스 높이 210 을 넘어 잘리고
//     타일 캡션(시리즈 이름)에 가려졌다.
//   - 반원 RB: 단위를 아예 그리지 않아 보이지 않았다.

import { describe, expect, it } from 'vitest';
import type { ReactElement } from 'react';
import { render } from '@testing-library/react';

import { parseConfig, renderGaugeByType } from './gaugeShapes';
import { GaugeValueOverlay } from './gaugeValue';

/** 니들이 있는 유형. */
const NEEDLE_TYPES = ['needle', 'needle-rainbow', 'half-rainbow'] as const;

/** 값 + 단위를 그리는 유형. */
const VALUE_UNIT_TYPES = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'vertical-bar',
  'half-rainbow',
] as const;

/** 값 텍스트의 마지막 행 baseline 과 캔버스 높이. */
function valueTextBottom(gaugeType: string): { bottom: number; height: number; rows: number } {
  const view = render(
    renderGaugeWithValue(parseConfig({ value: 50, gaugeType, unit: 'KB/s' }), true),
  );
  // 값은 오버레이의 SVG 에 있다(도형 SVG 와 형제다). 두 SVG 는 같은 viewBox 를 쓰므로
  // 캔버스 높이는 어느 쪽에서 재도 같지만, 글자는 오버레이에서만 찾을 수 있다.
  const svg = view.container.querySelector(
    '[data-testid="gauge-value-overlay"] svg',
  ) as SVGSVGElement;
  const height = Number((svg.getAttribute('viewBox') ?? '').split(/\s+/)[3]);
  const el = Array.from(svg.querySelectorAll('text')).find(
    (t) => t.querySelectorAll('tspan').length > 0,
  )!;
  const spans = Array.from(el.querySelectorAll('tspan'));
  const dy = Number(spans[spans.length - 1]?.getAttribute('dy') ?? 0);
  const bottom = Number(el.getAttribute('y')) + dy;
  view.unmount();
  return { bottom, height, rows: spans.length };
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

describe('값 텍스트가 캔버스를 넘지 않는다', () => {
  for (const gaugeType of VALUE_UNIT_TYPES) {
    it(`${gaugeType}: 마지막 행이 캔버스 안에 있다`, () => {
      const { bottom, height } = valueTextBottom(gaugeType);
      // 글자 높이 절반의 여유를 둔다 — baseline 이 딱 경계면 아래가 잘린다.
      expect(bottom).toBeLessThan(height);
    });

    it(`${gaugeType}: 단위가 둘째 행으로 나온다`, () => {
      expect(valueTextBottom(gaugeType).rows).toBe(2);
    });
  }

  it('세로 바는 캔버스를 넓혀 둘째 행을 담는다', () => {
    // 종전 높이 210 에서는 둘째 행이 210.8 로 넘어갔다.
    expect(valueTextBottom('vertical-bar').height).toBeGreaterThan(210);
  });
});

describe('니들 색 설정', () => {
  /** 니들 도형(polygon 또는 클래스 없는 line)의 색 정보. */
  function needleShape(gaugeType: string, needleColor?: string): Element {
    const view = render(
      renderGaugeWithValue(
        parseConfig({
          value: 50,
          gaugeType,
          ...(needleColor === undefined ? {} : { needle_color: needleColor }),
        }),
        true,
      ),
    );
    // 니들은 유형에 따라 polygon(삼각형) 또는 마지막 line(바늘 선)이다.
    const polygon = view.container.querySelector('polygon');
    const lines = Array.from(view.container.querySelectorAll('line'));
    const el = (polygon ?? lines[lines.length - 1])!.cloneNode(true) as Element;
    view.unmount();
    return el;
  }

  for (const gaugeType of NEEDLE_TYPES) {
    it(`${gaugeType}: 미지정이면 본문 글자색을 따른다`, () => {
      // 색을 박지 않고 테마 변수를 써야 다크·라이트 양쪽에서 보인다.
      const el = needleShape(gaugeType);
      expect(el.getAttribute('class')).toMatch(/--color-text-primary/);
      expect(el.getAttribute('fill')).toBeNull();
      expect(el.getAttribute('stroke')).toBeNull();
    });

    it(`${gaugeType}: 색을 지정하면 그 색으로 그린다`, () => {
      const el = needleShape(gaugeType, '#00ffcc');
      expect(el.getAttribute('class')).toBeNull();
      const painted = el.getAttribute('fill') ?? el.getAttribute('stroke');
      expect(painted).toBe('#00ffcc');
    });

    it(`${gaugeType}: 빈 문자열은 미지정과 같다`, () => {
      expect(needleShape(gaugeType, '').getAttribute('class')).toMatch(/--color-text-primary/);
    });
  }
});
