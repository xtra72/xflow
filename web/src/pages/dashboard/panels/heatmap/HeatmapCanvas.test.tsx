// HeatmapCanvas 렌더 파이프라인 focused 테스트 (SPEC-HEATMAP-PANEL-001 T5/T9).
//
// 실제 픽셀 스냅샷 대신, canvas 2D context 를 스텁으로 대체해 렌더 경로가 끝까지 실행되는지
// (저해상 ImageData 채움 → putImageData → DPR 스케일 업스케일 blit)를 검증한다. DPR 백킹
// 버퍼 크기(AC-E4 선명도)와 리사이즈 재계산도 확인한다.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup, act } from '@testing-library/react';

import HeatmapCanvas from './HeatmapCanvas';
import { DEFAULT_COLOR_TABLE } from './idw';

// ---- ResizeObserver 오버라이드: observe 시 지정 크기로 콜백을 발화한다 ----
type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
let roInstances: { cb: RoCallback }[] = [];
let currentSize = { width: 100, height: 80 };

class TriggeringResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
    roInstances.push(this);
  }
  observe() {
    // 관찰 시작 즉시 현재 크기로 콜백을 발화한다(jsdom 은 실제 레이아웃이 없다).
    this.cb([{ contentRect: { width: currentSize.width, height: currentSize.height } }]);
  }
  unobserve() {}
  disconnect() {}
}

/** 렌더 호출을 기록하는 2D context 스텁을 만든다. */
function makeCtxStub() {
  return {
    createImageData: vi.fn((w: number, h: number) => ({
      data: new Uint8ClampedArray(w * h * 4),
      width: w,
      height: h,
    })),
    putImageData: vi.fn(),
    drawImage: vi.fn(),
    clearRect: vi.fn(),
    imageSmoothingEnabled: false,
  };
}

let ctxStub: ReturnType<typeof makeCtxStub>;

beforeEach(() => {
  roInstances = [];
  currentSize = { width: 100, height: 80 };
  ctxStub = makeCtxStub();
  vi.stubGlobal('ResizeObserver', TriggeringResizeObserver as unknown as typeof ResizeObserver);
  vi.stubGlobal('devicePixelRatio', 2);
  // 모든 canvas(오프스크린 포함)가 동일 스텁 context 를 반환한다.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    ctxStub as unknown as CanvasRenderingContext2D,
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const baseProps = {
  points: [
    { x: 0.2, y: 0.2, value: 20 },
    { x: 0.8, y: 0.8, value: 26 },
  ],
  bounds: { min: 18, max: 26 },
  colorTable: DEFAULT_COLOR_TABLE,
  power: 2,
  gridResolution: 8,
};

describe('HeatmapCanvas', () => {
  it('렌더 경로가 끝까지 실행된다(ImageData 채움 → putImageData → drawImage 업스케일)', () => {
    render(<HeatmapCanvas {...baseProps} />);
    // 저해상 격자 크기(8x8)로 ImageData 를 만든다.
    expect(ctxStub.createImageData).toHaveBeenCalledWith(8, 8);
    expect(ctxStub.putImageData).toHaveBeenCalled();
    // 저해상 오프스크린 → 표시 canvas 로 업스케일 blit.
    expect(ctxStub.drawImage).toHaveBeenCalled();
    const [, sx, sy, sw, sh] = ctxStub.drawImage.mock.calls[0]!;
    // source rect 는 저해상 격자 전체(0,0,8,8).
    expect([sx, sy, sw, sh]).toEqual([0, 0, 8, 8]);
  });

  it('표시 canvas 백킹 버퍼가 devicePixelRatio 로 스케일된다(AC-E4 선명도)', () => {
    const { container } = render(<HeatmapCanvas {...baseProps} />);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    // 표시 100x80 × dpr 2 → 백킹 200x160, CSS 크기는 100x80px.
    expect(canvas.width).toBe(200);
    expect(canvas.height).toBe(160);
    expect(canvas.style.width).toBe('100px');
    expect(canvas.style.height).toBe('80px');
  });

  it('gridResolution 은 상한(MAX_GRID_RESOLUTION)으로 clamp 된다(R2 성능 가드)', () => {
    render(<HeatmapCanvas {...baseProps} gridResolution={9999} />);
    // 128 으로 clamp.
    expect(ctxStub.createImageData).toHaveBeenCalledWith(128, 128);
  });

  it('리사이즈 시 새 표시 크기(및 DPR)로 백킹 버퍼를 재계산한다', () => {
    const { container } = render(<HeatmapCanvas {...baseProps} />);
    const canvas = container.querySelector('canvas') as HTMLCanvasElement;
    expect(canvas.width).toBe(200);
    // 관찰자 콜백을 새 크기로 다시 발화 → 재렌더 → 백킹 버퍼 갱신.
    act(() => {
      roInstances[0]!.cb([{ contentRect: { width: 50, height: 40 } }]);
    });
    expect(canvas.width).toBe(100); // 50 × dpr 2
    expect(canvas.height).toBe(80);
  });
});
