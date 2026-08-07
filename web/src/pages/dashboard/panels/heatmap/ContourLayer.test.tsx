// ContourLayer 렌더/메모이즈 테스트 (SPEC-HEATMAP-PANEL-003 T3/T9).
//
// AC-01(격자 재사용 등고선), AC-04(선 스타일/라벨), AC-E2(off/빈 격자 graceful),
// AC-E4(격자 불변 시 재계산 생략)를 컴포넌트 레벨에서 커버한다. field 는 idw.ts 격자를
// 그대로 소비한다(재보간 없음, AC-E5).

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/react';

import ContourLayer from './ContourLayer';
import * as marching from './marchingSquares';
import { interpolateIDW } from './idw';
import type { ContourConfig } from './heatmapConfig';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/** 수평 경사 격자(왼쪽 낮고 오른쪽 높음) — level 중간값이 세로 등치선을 만든다. */
function gradientField(w = 8, h = 8): Float32Array {
  const f = new Float32Array(w * h);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      f[y * w + x] = 18 + (x / (w - 1)) * 8; // 18..26
    }
  }
  return f;
}

function baseContour(overrides: Partial<ContourConfig> = {}): ContourConfig {
  return { enabled: true, level_count: 3, line: {}, labels: false, ...overrides };
}

describe('ContourLayer — 렌더(AC-01)', () => {
  it('활성 격자에서 등치선 <path> 를 SVG 오버레이로 렌더한다', () => {
    const { getByTestId, container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour()}
      />,
    );
    const svg = getByTestId('contour-layer');
    expect(svg.tagName.toLowerCase()).toBe('svg');
    expect(svg.getAttribute('viewBox')).toBe('0 0 1 1');
    expect(svg.getAttribute('preserveAspectRatio')).toBe('none');
    // 최소 1개 이상의 등치선 path.
    expect(container.querySelectorAll('path').length).toBeGreaterThan(0);
  });

  it('idw.ts 격자를 그대로 입력으로 사용한다(재보간 없음, AC-E5)', () => {
    const field = interpolateIDW(
      [
        { x: 0.1, y: 0.5, value: 18 },
        { x: 0.9, y: 0.5, value: 26 },
      ],
      8,
      8,
      2,
    );
    const spy = vi.spyOn(marching, 'computeContours');
    render(
      <ContourLayer
        field={field}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour()}
      />,
    );
    // computeContours 는 전달받은 동일 field 참조로 호출된다.
    expect(spy).toHaveBeenCalled();
    for (const call of spy.mock.calls) {
      expect(call[0]).toBe(field);
    }
  });
});

describe('ContourLayer — 선 스타일/라벨(AC-04)', () => {
  it('선 색/두께/dash 를 path 에 반영한다', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour({ line: { color: '#ff0000', width: 3, dash: [4, 2] } })}
      />,
    );
    const path = container.querySelector('path')!;
    expect(path.getAttribute('stroke')).toBe('#ff0000');
    expect(path.getAttribute('stroke-width')).toBe('3');
    expect(path.getAttribute('stroke-dasharray')).toBe('4 2');
    expect(path.getAttribute('vector-effect')).toBe('non-scaling-stroke');
  });

  it('미지정 선 스타일은 기본 색/두께로 렌더하고 dash 는 없다(실선)', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour()}
      />,
    );
    const path = container.querySelector('path')!;
    expect(path.getAttribute('stroke')).toBe('#334155');
    expect(path.getAttribute('stroke-width')).toBe('1');
    expect(path.getAttribute('stroke-dasharray')).toBeNull();
  });

  it('labels=true 면 각 등치선에 <text> 값 라벨을 배치한다', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour({ levels: [22], level_count: 3, labels: true })}
      />,
    );
    const texts = container.querySelectorAll('text');
    expect(texts.length).toBeGreaterThan(0);
    // explicit [22] → 라벨 텍스트에 22 가 포함된다.
    expect(Array.from(texts).some((t) => t.textContent === '22')).toBe(true);
  });

  it('labels=false 면 <text> 라벨을 렌더하지 않는다', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour({ labels: false })}
      />,
    );
    expect(container.querySelectorAll('text').length).toBe(0);
  });
});

describe('ContourLayer — graceful(AC-E2)', () => {
  it('enabled=false 면 null 을 반환한다(예외 없음)', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour({ enabled: false })}
      />,
    );
    expect(container.querySelector('[data-testid="contour-layer"]')).toBeNull();
  });

  it('빈 격자(gridW<=0)면 null 을 반환한다', () => {
    const { container } = render(
      <ContourLayer
        field={new Float32Array(0)}
        gridW={0}
        gridH={0}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour()}
      />,
    );
    expect(container.querySelector('[data-testid="contour-layer"]')).toBeNull();
  });

  it('레벨이 전부 범위 밖(explicit)이면 null 을 반환한다(AC-E3)', () => {
    const { container } = render(
      <ContourLayer
        field={gradientField()}
        gridW={8}
        gridH={8}
        bounds={{ min: 18, max: 26 }}
        contour={baseContour({ levels: [30, 40] })}
      />,
    );
    expect(container.querySelector('[data-testid="contour-layer"]')).toBeNull();
  });
});

describe('ContourLayer — memoize(AC-E4)', () => {
  it('격자 참조 불변 시 재렌더에도 등고선을 재계산하지 않는다', () => {
    const field = gradientField();
    const bounds = { min: 18, max: 26 };
    const contour = baseContour();
    const spy = vi.spyOn(marching, 'computeContours');

    const { rerender } = render(
      <ContourLayer field={field} gridW={8} gridH={8} bounds={bounds} contour={contour} />,
    );
    const firstCallCount = spy.mock.calls.length;
    expect(firstCallCount).toBeGreaterThan(0);

    // 동일 참조(field/bounds/contour)로 재렌더 → useMemo 캐시 → 재계산 없음.
    rerender(
      <ContourLayer field={field} gridW={8} gridH={8} bounds={bounds} contour={contour} />,
    );
    expect(spy.mock.calls.length).toBe(firstCallCount);
  });

  it('격자 참조가 바뀌면 재계산한다', () => {
    const bounds = { min: 18, max: 26 };
    const contour = baseContour();
    const spy = vi.spyOn(marching, 'computeContours');

    const { rerender } = render(
      <ContourLayer field={gradientField()} gridW={8} gridH={8} bounds={bounds} contour={contour} />,
    );
    const firstCallCount = spy.mock.calls.length;

    // 새 격자 참조 → 재계산.
    rerender(
      <ContourLayer field={gradientField()} gridW={8} gridH={8} bounds={bounds} contour={contour} />,
    );
    expect(spy.mock.calls.length).toBeGreaterThan(firstCallCount);
  });
});
