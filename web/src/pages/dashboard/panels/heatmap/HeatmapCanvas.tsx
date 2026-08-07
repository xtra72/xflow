// 히트맵 Canvas 2D 렌더 컴포넌트 (SPEC-HEATMAP-PANEL-001 T5, SPEC-003 T4b 리팩터).
//
// 상위(HeatmapPanel)가 계산해 전달한 저해상 스칼라 격자(field)를 상하한 + 색상표로 그린다
// (순수 렌더, 보간/조회 없음). SPEC-003 에서 격자 계산을 HeatmapPanel 로 상승해 등고선
// (ContourLayer)과 **동일 field 참조**를 공유한다(재보간 금지, R3). 성능(R2)을 위해 격자는
// 저해상이고 표시 크기로 업스케일한다. 선명도(R1)를 위해 devicePixelRatio 로 백킹 버퍼를
// 스케일하고, ResizeObserver 로 리사이즈 시 재계산한다(AC-E4).
//
// 렌더 파이프라인(행위 보존 — 출력 byte 동일):
//   1) 전달받은 field(저해상 Float32Array 온도장, gridW×gridH)
//   2) mapValueToColor → 저해상 오프스크린 canvas 의 ImageData(putImageData)
//   3) drawImage(오프스크린 → DPR 스케일 표시 canvas) 로 업스케일 blit(부드러운 보간)
//
// @spec SPEC-HEATMAP-PANEL-001
// @spec SPEC-HEATMAP-PANEL-003

import { useEffect, useRef, useState } from 'react';

import { mapValueToColor } from './idw';
import type { ColorStop } from './heatmapConfig';

/** 격자 해상도 하한/상한(R2 성능 가드 — 디스플레이 픽셀 IDW 폭주 방지). HeatmapPanel 이 clamp 에 사용. */
export const MIN_GRID_RESOLUTION = 4;
export const MAX_GRID_RESOLUTION = 128;

interface HeatmapCanvasProps {
  /** 저해상 스칼라 온도장(행 우선, idx = py*gridW + px). HeatmapPanel 이 interpolateIDW 로 계산해 전달. */
  field: Float32Array;
  /** 격자 폭(field 의 열 수). */
  gridW: number;
  /** 격자 높이(field 의 행 수). */
  gridH: number;
  /** 색상 매핑 clamp 범위(자동 또는 config 값을 상위에서 해석해 전달). */
  bounds: { min: number; max: number };
  /** 색상표(비어 있으면 mapValueToColor 가 기본 gradient 로 폴백). */
  colorTable: ColorStop[];
  /** 데이터 존재 여부(false 면 alpha=0 으로 blank — 빈 상태 규약 보존). */
  hasData: boolean;
}

export default function HeatmapCanvas({
  field,
  gridW,
  gridH,
  bounds,
  colorTable,
  hasData,
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

  // 온도장 렌더: 전달받은 저해상 격자 → 오프스크린 ImageData → DPR 스케일 표시 canvas 로 업스케일.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const { width, height } = size;
    if (width <= 0 || height <= 0) return;
    // 빈/무효 격자 방어(렌더 예외 없음).
    if (gridW <= 0 || gridH <= 0 || field.length < gridW * gridH) return;

    const dpr =
      typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1;

    // 1) 저해상 오프스크린 canvas 에 ImageData 채움(전달받은 field 소비 — 재보간 없음).
    const off = document.createElement('canvas');
    off.width = gridW;
    off.height = gridH;
    const octx = off.getContext('2d');
    if (!octx) return;
    const img = octx.createImageData(gridW, gridH);
    for (let i = 0; i < gridW * gridH; i++) {
      const [r, g, b, a] = mapValueToColor(field[i]!, bounds, colorTable);
      const o = i * 4;
      img.data[o] = r;
      img.data[o + 1] = g;
      img.data[o + 2] = b;
      // 데이터가 없으면 완전 투명으로 둔다(빈 상태는 패널이 별도 안내 — 여기선 blank).
      img.data[o + 3] = hasData ? a : 0;
    }
    octx.putImageData(img, 0, 0);

    // 2) DPR 스케일 표시 canvas 에 업스케일 blit.
    canvas.width = Math.max(1, Math.round(width * dpr));
    canvas.height = Math.max(1, Math.round(height * dpr));
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.imageSmoothingEnabled = true; // 저해상 격자를 부드럽게 업스케일.
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(off, 0, 0, gridW, gridH, 0, 0, canvas.width, canvas.height);
  }, [size, field, gridW, gridH, bounds, colorTable, hasData]);

  return (
    <div ref={containerRef} className="relative min-h-0 w-full flex-1">
      <canvas ref={canvasRef} data-testid="heatmap-canvas" className="block h-full w-full" />
    </div>
  );
}
