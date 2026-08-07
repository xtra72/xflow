// 히트맵 Canvas 2D 렌더 컴포넌트 (SPEC-HEATMAP-PANEL-001 T5).
//
// props 로 받은 배치 센서점 + 상하한 + 색상표 + IDW 파라미터로 온도장을 그린다(순수 렌더,
// 데이터 조회 없음). 성능(R2)을 위해 IDW 는 저해상 격자(grid_resolution)에서만 계산하고,
// 표시 크기로 업스케일한다(디스플레이 픽셀마다 IDW 계산하지 않는다). 선명도(R1)를 위해
// devicePixelRatio 로 백킹 버퍼를 스케일하고, ResizeObserver 로 리사이즈 시 재계산한다(AC-E4).
//
// 렌더 파이프라인:
//   1) interpolateIDW → 저해상 Float32Array 온도장
//   2) mapValueToColor → 저해상 오프스크린 canvas 의 ImageData(putImageData)
//   3) drawImage(오프스크린 → DPR 스케일 표시 canvas) 로 업스케일 blit(부드러운 보간)
//
// @spec SPEC-HEATMAP-PANEL-001

import { useEffect, useRef, useState } from 'react';

import { interpolateIDW, mapValueToColor } from './idw';
import type { IdwPoint } from './idw';
import type { ColorStop } from './heatmapConfig';

/** 격자 해상도 하한/상한(R2 성능 가드 — 디스플레이 픽셀 IDW 폭주 방지). */
export const MIN_GRID_RESOLUTION = 4;
export const MAX_GRID_RESOLUTION = 128;

interface HeatmapCanvasProps {
  /** 좌표가 배치된 센서점(정규화 0..1 좌표 + 값). */
  points: IdwPoint[];
  /** 색상 매핑 clamp 범위(자동 또는 config 값을 상위에서 해석해 전달). */
  bounds: { min: number; max: number };
  /** 색상표(비어 있으면 mapValueToColor 가 기본 gradient 로 폴백). */
  colorTable: ColorStop[];
  /** IDW 거리 감쇠 지수. */
  power: number;
  /** 격자 해상도(clamp 되어 저해상 계산에 사용). */
  gridResolution: number;
}

/** 값을 [lo, hi] 로 clamp 한다. */
function clamp(value: number, lo: number, hi: number): number {
  return value < lo ? lo : value > hi ? hi : value;
}

export default function HeatmapCanvas({
  points,
  bounds,
  colorTable,
  power,
  gridResolution,
}: HeatmapCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState<{ width: number; height: number }>({ width: 0, height: 0 });

  // 표시 영역 크기를 ResizeObserver 로 추적한다(부모 리사이즈/DPR 대응, AC-E4).
  useEffect(() => {
    const el = containerRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const cr = entry.contentRect;
        setSize({ width: Math.floor(cr.width), height: Math.floor(cr.height) });
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // 온도장 렌더: 저해상 격자 계산 → 오프스크린 ImageData → DPR 스케일 표시 canvas 로 업스케일.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const { width, height } = size;
    if (width <= 0 || height <= 0) return;

    const dpr =
      typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1;
    const grid = Math.round(clamp(gridResolution, MIN_GRID_RESOLUTION, MAX_GRID_RESOLUTION));

    // 1) 저해상 온도장.
    const field = interpolateIDW(points, grid, grid, power);

    // 2) 저해상 오프스크린 canvas 에 ImageData 채움.
    const off = document.createElement('canvas');
    off.width = grid;
    off.height = grid;
    const octx = off.getContext('2d');
    if (!octx) return;
    const img = octx.createImageData(grid, grid);
    const hasData = points.length > 0;
    for (let i = 0; i < field.length; i++) {
      const [r, g, b, a] = mapValueToColor(field[i]!, bounds, colorTable);
      const o = i * 4;
      img.data[o] = r;
      img.data[o + 1] = g;
      img.data[o + 2] = b;
      // 데이터가 없으면 완전 투명으로 둔다(빈 상태는 패널이 별도 안내 — 여기선 blank).
      img.data[o + 3] = hasData ? a : 0;
    }
    octx.putImageData(img, 0, 0);

    // 3) DPR 스케일 표시 canvas 에 업스케일 blit.
    canvas.width = Math.max(1, Math.round(width * dpr));
    canvas.height = Math.max(1, Math.round(height * dpr));
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.imageSmoothingEnabled = true; // 저해상 격자를 부드럽게 업스케일.
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(off, 0, 0, grid, grid, 0, 0, canvas.width, canvas.height);
  }, [size, points, bounds, colorTable, power, gridResolution]);

  return (
    <div ref={containerRef} className="relative min-h-0 w-full flex-1">
      <canvas ref={canvasRef} data-testid="heatmap-canvas" className="block h-full w-full" />
    </div>
  );
}
