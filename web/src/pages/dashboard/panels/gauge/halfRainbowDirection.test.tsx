// 반원 RB 게이지의 방향 설정.
//
// 종전 기본값은 `startAngle: 180`(왼쪽 반원)이었는데 캔버스는 240×150 이라, 아크가
// y=190 까지 내려가 **아래쪽 40px 이 잘려** 있었다. 오른쪽 절반은 늘 비어 있었다.
// 방향을 고를 수 있게 하면서 기본값을 위쪽 반원으로 바로잡는다.
//
// 방향마다 캔버스를 따로 두는 것이 이 기능의 핵심이다 — 반원은 방향에 따라 차지하는
// 상자가 가로로 넓거나(위·아래) 세로로 길다(왼쪽·오른쪽). 한 캔버스를 돌려 쓰면
// 어느 한 방향은 반드시 잘린다. 그래서 "네 방향 모두 캔버스 안에 들어가는가" 를 잠근다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import {
  HALF_RAINBOW_DIRECTIONS,
  parseConfig,
  readHalfRainbowDirection,
  renderGaugeByType,
  type HalfRainbowDirection,
} from './gaugeShapes';

/** 반원 RB 를 그려 SVG 엘리먼트를 돌려준다. */
function drawSvg(direction?: string): SVGSVGElement {
  const view = render(
    renderGaugeByType(
      parseConfig({
        value: 50,
        gaugeType: 'half-rainbow',
        ...(direction === undefined ? {} : { half_rainbow_direction: direction }),
      }),
      true,
    ),
  );
  const svg = view.container.querySelector('svg')!;
  // 마크업만 쓰므로 unmount 전에 복제해 둔다.
  const clone = svg.cloneNode(true) as SVGSVGElement;
  view.unmount();
  return clone;
}

/** SVG 안에서 실제로 쓰인 좌표의 최대치. 캔버스를 넘는지 판정한다. */
function maxCoords(svg: SVGSVGElement): { x: number; y: number } {
  const xs: number[] = [];
  const ys: number[] = [];
  for (const path of Array.from(svg.querySelectorAll('path'))) {
    const d = path.getAttribute('d') ?? '';
    // `M x,y` · `L x,y` · 호의 종점 좌표를 훑는다. 반지름·플래그 쌍도 함께 잡히지만
    // 그 값들은 좌표보다 작아 최대치 판정을 흐리지 않는다.
    for (const m of d.matchAll(/(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/g)) {
      xs.push(parseFloat(m[1]!));
      ys.push(parseFloat(m[2]!));
    }
  }
  for (const el of Array.from(svg.querySelectorAll('text, circle'))) {
    const x = el.getAttribute('x') ?? el.getAttribute('cx');
    const y = el.getAttribute('y') ?? el.getAttribute('cy');
    if (x !== null) xs.push(parseFloat(x));
    if (y !== null) ys.push(parseFloat(y));
  }
  return { x: Math.max(...xs), y: Math.max(...ys) };
}

/** viewBox 의 폭·높이. */
function viewBoxSize(svg: SVGSVGElement): { w: number; h: number } {
  const parts = (svg.getAttribute('viewBox') ?? '').split(/\s+/).map(Number);
  return { w: parts[2]!, h: parts[3]! };
}

describe('방향 값 읽기', () => {
  it('미지정이면 위쪽이다 — 잘려 있던 왼쪽이 기본으로 남지 않는다', () => {
    expect(readHalfRainbowDirection(undefined)).toBe('up');
    expect(parseConfig({ gaugeType: 'half-rainbow' }).halfRainbowDirection).toBe('up');
  });

  it('네 방향을 그대로 읽는다', () => {
    for (const dir of HALF_RAINBOW_DIRECTIONS) {
      expect(readHalfRainbowDirection(dir)).toBe(dir);
    }
  });

  it('모르는 값은 기본으로 접는다 — 손으로 편집한 config 가 게이지를 깨뜨리지 않는다', () => {
    expect(readHalfRainbowDirection('diagonal')).toBe('up');
    expect(readHalfRainbowDirection(7)).toBe('up');
    expect(readHalfRainbowDirection(null)).toBe('up');
  });
});

describe('방향별 렌더', () => {
  for (const dir of HALF_RAINBOW_DIRECTIONS) {
    it(`${dir}: 그림이 캔버스를 넘지 않는다`, () => {
      const svg = drawSvg(dir);
      const { w, h } = viewBoxSize(svg);
      const max = maxCoords(svg);
      // 종전 결함: 왼쪽 반원의 아크가 y=190 까지 내려가 높이 150 을 넘었다.
      expect(max.x).toBeLessThanOrEqual(w);
      expect(max.y).toBeLessThanOrEqual(h);
    });
  }

  it('네 방향이 서로 다른 그림을 낸다', () => {
    const drawn = HALF_RAINBOW_DIRECTIONS.map((d) => drawSvg(d).innerHTML);
    expect(new Set(drawn).size).toBe(HALF_RAINBOW_DIRECTIONS.length);
  });

  it('위·아래는 가로로 넓고, 왼쪽·오른쪽은 세로로 길다', () => {
    // 반원이 차지하는 상자가 방향에 따라 다르므로 캔버스도 따라간다.
    for (const dir of ['up', 'down'] as HalfRainbowDirection[]) {
      const { w, h } = viewBoxSize(drawSvg(dir));
      expect(w).toBeGreaterThan(h);
    }
    for (const dir of ['left', 'right'] as HalfRainbowDirection[]) {
      const { w, h } = viewBoxSize(drawSvg(dir));
      expect(h).toBeGreaterThanOrEqual(w);
    }
  });

  it('기본 렌더는 위쪽과 같다', () => {
    expect(drawSvg(undefined).outerHTML).toBe(drawSvg('up').outerHTML);
  });

  it('위쪽은 9시에서 시작해 3시에서 끝난다', () => {
    // cx=120, cy=110, outerR=80 → 시작 (40,110), 끝 (200,110).
    const d = drawSvg('up').querySelector('path')!.getAttribute('d') ?? '';
    expect(d).toMatch(/^M 40,110/);
  });

  it('다른 게이지 타입은 방향 설정을 읽지 않는다', () => {
    // 저장돼 있어도 무해해야 한다 — 타입을 바꿔 가며 편집하면 남는 값이다.
    const svgA = render(
      renderGaugeByType(parseConfig({ value: 50, gaugeType: 'half' }), true),
    );
    const a = svgA.container.innerHTML;
    svgA.unmount();
    const svgB = render(
      renderGaugeByType(
        parseConfig({ value: 50, gaugeType: 'half', half_rainbow_direction: 'left' }),
        true,
      ),
    );
    const b = svgB.container.innerHTML;
    svgB.unmount();
    expect(a).toBe(b);
  });
});
