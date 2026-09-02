// FloorPlanBackground 렌더 테스트 (SPEC-HEATMAP-PANEL-002 T3 + 다중 레이어).
// 레이어 미첨부 시 null(AC-E1), 첨부 시 object-fit/박스/불투명도 렌더를 커버한다.

import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import FloorPlanBackground from './FloorPlanBackground';
import type { FloorPlanLayer } from './heatmapConfig';

/** 스테이지를 가득 채우는 기본 레이어(파서가 채운 형태). */
function layer(overrides: Partial<FloorPlanLayer> = {}): FloorPlanLayer {
  return {
    image: 'data:image/png;base64,AAAA',
    x: 0,
    y: 0,
    w: 1,
    h: 1,
    opacity: 1,
    fit: 'contain',
    ...overrides,
  };
}

/** 레이어의 인라인 image 를 그대로 해석 결과로 쓰는 헬퍼(패널이 하는 해석의 최소 대역). */
function srcsOf(layers: FloorPlanLayer[]): string[] {
  return layers.map((l) => l.image ?? '');
}

describe('FloorPlanBackground', () => {
  it('AC-E1: 레이어가 없으면 아무 것도 렌더하지 않는다(null)', () => {
    const { container } = render(<FloorPlanBackground layers={[]} sources={[]} />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByTestId('floor-plan-background')).toBeNull();
  });

  it('레이어가 있으면 data-URL 배경 이미지를 기본 fit(contain)으로 렌더한다', () => {
    render(<FloorPlanBackground layers={[layer()]} sources={srcsOf([layer()])} />);
    const img = screen.getByTestId('floor-plan-background');
    expect(img).toBeInTheDocument();
    expect(img.getAttribute('src')).toBe('data:image/png;base64,AAAA');
    expect((img as HTMLElement).style.objectFit).toBe('contain');
  });

  it('fit=cover 를 object-fit 으로 반영한다', () => {
    const cover = [layer({ image: 'data:image/png;base64,BBBB', fit: 'cover' as const })];
    render(<FloorPlanBackground layers={cover} sources={srcsOf(cover)} />);
    const img = screen.getByTestId('floor-plan-background');
    expect((img as HTMLElement).style.objectFit).toBe('cover');
  });

  it('기준 레이어는 스테이지를 가득 채우고(0,0,100%,100%) 불투명도가 반영된다', () => {
    render(<FloorPlanBackground layers={[layer({ opacity: 0.4 })]} sources={srcsOf([layer({ opacity: 0.4 })])} />);
    const img = screen.getByTestId('floor-plan-background') as HTMLElement;
    expect(img.style.left).toBe('0%');
    expect(img.style.top).toBe('0%');
    expect(img.style.width).toBe('100%');
    expect(img.style.height).toBe('100%');
    expect(img.style.opacity).toBe('0.4');
  });

  it('다중 레이어는 배열 순서대로 각자의 정규화 박스에 배치된다', () => {
    const MULTI = [
      layer(),
      layer({ image: 'data:image/png;base64,CCCC', x: 0.25, y: 0.5, w: 0.5, h: 0.25 }),
    ];
    render(
      <FloorPlanBackground
        layers={MULTI}
        sources={srcsOf(MULTI)}
      />,
    );
    // index 0 은 기준 레이어(기존 testid 유지 — 회귀 0), 이후는 layer-N.
    expect(screen.getByTestId('floor-plan-background')).toBeInTheDocument();
    const second = screen.getByTestId('floor-plan-layer-1') as HTMLElement;
    expect(second.getAttribute('src')).toBe('data:image/png;base64,CCCC');
    expect(second.style.left).toBe('25%');
    expect(second.style.top).toBe('50%');
    expect(second.style.width).toBe('50%');
    expect(second.style.height).toBe('25%');
  });
});

describe('FloorPlanBackground — fit 모드', () => {
  it('fit=fill 은 비율을 무시하고 박스를 정확히 채운다(잘리지 않음)', () => {
    // 보고된 결함: 폭을 줄이면 cover 는 잘라내지만, fill 은 이미지가 줄어들며 꽉 찬다.
    render(<FloorPlanBackground layers={[layer({ fit: 'fill', w: 0.4 })]} sources={srcsOf([layer({ fit: 'fill', w: 0.4 })])} />);
    const img = screen.getByTestId('floor-plan-background') as HTMLElement;
    expect(img.style.objectFit).toBe('fill');
    expect(img.style.width).toBe('40%');
  });

  it('fit=contain 은 기본값이며 여백을 남긴다', () => {
    render(<FloorPlanBackground layers={[layer()]} sources={srcsOf([layer()])} />);
    expect((screen.getByTestId('floor-plan-background') as HTMLElement).style.objectFit).toBe(
      'contain',
    );
  });
});

// stage_fit='stretch' 는 스테이지 자체가 비등방으로 늘어난 상태다. 이때 이미지가 자기 fit 을
// 유지하면 이미지 안에서 다시 레터박스가 생겨 없애려던 여백이 그대로 돌아온다.
describe('FloorPlanBackground — stretch 오버라이드', () => {
  it('stretch 면 레이어 fit 과 무관하게 fill 로 그린다', () => {
    const layers = [layer({ fit: 'contain' }), layer({ fit: 'cover' })];
    render(<FloorPlanBackground layers={layers} sources={srcsOf(layers)} stretch />);

    expect((screen.getByTestId('floor-plan-background') as HTMLImageElement).style.objectFit).toBe('fill');
    expect((screen.getByTestId('floor-plan-layer-1') as HTMLImageElement).style.objectFit).toBe('fill');
  });

  it('stretch 가 아니면 레이어별 fit 을 그대로 지킨다(회귀 0)', () => {
    const layers = [layer({ fit: 'contain' })];
    render(<FloorPlanBackground layers={layers} sources={srcsOf(layers)} />);

    expect((screen.getByTestId('floor-plan-background') as HTMLImageElement).style.objectFit).toBe('contain');
  });
});

describe('FloorPlanBackground — 기준 레이어 원본 크기 보고', () => {
  it('기준 레이어가 뜨면 원본 크기를 알린다(스테이지를 도면에 고정하는 마지막 보루)', () => {
    const onBaseSize = vi.fn();
    const { getByTestId } = render(
      <FloorPlanBackground
        layers={[layer(), layer({ image: 'data:image/png;base64,BBBB' })]}
        sources={['data:image/png;base64,AAAA', 'data:image/png;base64,BBBB']}
        onBaseSize={onBaseSize}
      />,
    );
    const img = getByTestId('floor-plan-background') as HTMLImageElement;
    Object.defineProperty(img, 'naturalWidth', { value: 800, configurable: true });
    Object.defineProperty(img, 'naturalHeight', { value: 400, configurable: true });
    fireEvent.load(img);
    expect(onBaseSize).toHaveBeenCalledWith(800, 400);
  });

  it('겹판(비기준) 레이어는 크기를 알리지 않는다', () => {
    const onBaseSize = vi.fn();
    const { getByTestId } = render(
      <FloorPlanBackground
        layers={[layer(), layer({ image: 'data:image/png;base64,BBBB' })]}
        sources={['data:image/png;base64,AAAA', 'data:image/png;base64,BBBB']}
        onBaseSize={onBaseSize}
      />,
    );
    const img = getByTestId('floor-plan-layer-1') as HTMLImageElement;
    Object.defineProperty(img, 'naturalWidth', { value: 100, configurable: true });
    Object.defineProperty(img, 'naturalHeight', { value: 100, configurable: true });
    fireEvent.load(img);
    expect(onBaseSize).not.toHaveBeenCalled();
  });

  it('원본 크기를 못 읽으면(0) 알리지 않는다(0 나눗셈·엉뚱한 비율 방지)', () => {
    const onBaseSize = vi.fn();
    const { getByTestId } = render(
      <FloorPlanBackground
        layers={[layer()]}
        sources={['data:image/png;base64,AAAA']}
        onBaseSize={onBaseSize}
      />,
    );
    fireEvent.load(getByTestId('floor-plan-background'));
    expect(onBaseSize).not.toHaveBeenCalled();
  });
  it('이미 캐시돼 load 이벤트가 오지 않는 이미지도 원본 크기를 알린다', () => {
    // React 가 onLoad 를 붙이기 전에 로드가 끝난 이미지는 load 이벤트를 다시 내지 않는다.
    // 같은 도면을 쓰는 패널이 여럿이거나 대시보드를 다시 여는 흔한 경로가 전부 이 경우다.
    const proto = HTMLImageElement.prototype as unknown as Record<string, unknown>;
    Object.defineProperty(proto, 'complete', { value: true, configurable: true });
    Object.defineProperty(proto, 'naturalWidth', { value: 1200, configurable: true });
    Object.defineProperty(proto, 'naturalHeight', { value: 400, configurable: true });
    try {
      const onBaseSize = vi.fn();
      render(
        <FloorPlanBackground
          layers={[layer()]}
          sources={['data:image/png;base64,AAAA']}
          onBaseSize={onBaseSize}
        />,
      );
      expect(onBaseSize).toHaveBeenCalledWith(1200, 400);
    } finally {
      // 프로토타입 오염을 되돌린다(다른 테스트는 크기 0 을 기대한다).
      for (const k of ['complete', 'naturalWidth', 'naturalHeight']) delete proto[k];
    }
  });
});

describe('FloorPlanBackground — SVG 도면', () => {
  /** URL 인코딩 SVG data-URL. */
  function svgUrl(body: string): string {
    return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(body)}`;
  }
  const PLAN = '<svg viewBox="0 0 1200 400"><rect /></svg>';

  it('늘려서 채우기: SVG 는 문서의 preserveAspectRatio 를 none 으로 바꿔 그린다', () => {
    // CSS object-fit: fill 은 상자만 늘린다. 그림을 가두는 것은 SVG 자신의 이 속성이라,
    // 재작성하지 않으면 상자만 커지고 도면은 가운데 레터박스된 채 남는다(보고된 증상).
    const { getByTestId } = render(
      <FloorPlanBackground layers={[layer({ image: svgUrl(PLAN) })]} sources={[svgUrl(PLAN)]} stretch />,
    );
    const img = getByTestId('floor-plan-background') as HTMLImageElement;
    expect(decodeURIComponent(img.getAttribute('src') ?? '')).toContain(
      'preserveAspectRatio="none"',
    );
    expect(img.style.objectFit).toBe('fill');
  });

  it('여백 맞춤에서는 SVG 문서를 손대지 않는다(비율 유지가 목적이다)', () => {
    const { getByTestId } = render(
      <FloorPlanBackground layers={[layer({ image: svgUrl(PLAN) })]} sources={[svgUrl(PLAN)]} />,
    );
    const img = getByTestId('floor-plan-background') as HTMLImageElement;
    expect(decodeURIComponent(img.getAttribute('src') ?? '')).not.toContain(
      'preserveAspectRatio',
    );
  });

  it('SVG 원본 크기는 viewBox 에서 읽는다(브라우저 보고값을 믿지 않는다)', () => {
    // sizeless SVG 의 naturalWidth/Height 는 원본이 아니라 대체 요소 기본값이라 비율이 틀어진다.
    const proto = HTMLImageElement.prototype as unknown as Record<string, unknown>;
    Object.defineProperty(proto, 'complete', { value: true, configurable: true });
    Object.defineProperty(proto, 'naturalWidth', { value: 300, configurable: true });
    Object.defineProperty(proto, 'naturalHeight', { value: 150, configurable: true });
    try {
      const onBaseSize = vi.fn();
      render(
        <FloorPlanBackground
          layers={[layer({ image: svgUrl(PLAN) })]}
          sources={[svgUrl(PLAN)]}
          onBaseSize={onBaseSize}
        />,
      );
      expect(onBaseSize).toHaveBeenCalledWith(1200, 400);
      expect(onBaseSize).not.toHaveBeenCalledWith(300, 150);
    } finally {
      for (const k of ['complete', 'naturalWidth', 'naturalHeight']) delete proto[k];
    }
  });
});
