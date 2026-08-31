// 값→색 색표(colorbar) 범례 오버레이 (additive).
//
// 히트맵과 **동일 color_table**(정렬된 stop 0..1)로 CSS linear-gradient 막대를 만들어 값이
// 어떤 색인지 보여준다(별도 색 계산 없음). colorTable 이 비면 DEFAULT_COLOR_TABLE(idw.ts)로
// 폴백한다. 등간 눈금 라벨을 막대 옆(세로: 오른쪽, 가로: 아래)에 배치하며, bounds 퇴화
// (min==max) 시 단일 라벨로 방어한다.
//
// 레이어 배치: HeatmapPanel 스택 최상단(z-25) + pointer-events-none 이라 편집/드래그를
// 가로막지 않는다. 편집모드와 무관하게 표시(뷰어도 범례는 봄).
//
// 위치: 기본은 4모서리 프리셋(position)이고, 배치 편집 중 드래그로 옮기면 자유 좌표
// (legend.offset, 정규화 0..1)가 저장돼 그쪽이 우선한다. 드래그는 `draggable` 이 켜진 동안에만
// 포인터 이벤트를 열며(편집모드/설정 미리보기 한정), 뷰어에서는 pointer-events-none 이 유지돼
// 아래 레이어의 클릭을 가로채지 않는다.

import { useRef, useState } from 'react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { ColorStop, LegendConfig, SensorPosition } from './heatmapConfig';
import { DEFAULT_COLOR_TABLE } from './idw';
import { clamp01 } from './placement';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import { formatDecimal } from '@/pages/dashboard/panels/charts/decimalPlaces';

/** 크기 프리셋(막대 길이/두께/폰트, px). */
const SIZE_PRESETS: Record<LegendConfig['size'], { length: number; thickness: number; font: number }> = {
  sm: { length: 60, thickness: 8, font: 9 },
  md: { length: 96, thickness: 10, font: 11 },
  lg: { length: 140, thickness: 14, font: 13 },
};

/** 모서리 위치 → absolute 배치 클래스. */
const POSITION_CLASSES: Record<LegendConfig['position'], string> = {
  'top-left': 'top-2 left-2',
  'top-right': 'top-2 right-2',
  'bottom-left': 'bottom-2 left-2',
  'bottom-right': 'bottom-2 right-2',
};

interface HeatmapLegendProps {
  /** 값 범위(색 매핑 clamp 범위와 동일). */
  bounds: { min: number; max: number };
  /** 히트맵과 동일 색상표(정렬 전 stop 0..1). 비면 기본 gradient 폴백. */
  colorTable: ColorStop[];
  /** 범례 설정(파싱 완료 — enabled/orientation/position/size/tick_count). */
  legend: LegendConfig;
  /**
   * 드래그 이동 허용(배치 편집 중에만 true). false(기본)면 pointer-events-none 이 유지돼
   * 뷰어 동작이 기존과 동일하다.
   */
  draggable?: boolean;
  /** 드래그 결과 자유 위치(정규화 0..1) 저장. draggable 일 때만 호출된다. */
  onOffsetChange?: (offset: SensorPosition) => void;
  /**
   * 눈금 자릿수. 사용자가 패널 설정에서 **직접 지정했을 때만** 전달된다.
   * `undefined` 면 종전 표기(소수 1자리)를 유지한다.
   */
  decimals?: number;
}

/**
 * 눈금 값 표시.
 *
 * 범례는 색 눈금자이지 값 읽기가 아니므로 **자릿수 기본값(2)을 따르지 않는다** — 기본을
 * 걸면 아무 설정도 안 한 패널의 눈금이 `20.00 · 22.00` 이 되어 읽기만 나빠진다. 사용자가
 * 자릿수를 직접 지정하면 그때 따라간다(`decimalPlaces.ts` 머리말의 축 규칙과 같다).
 */
function formatTick(v: number, decimals: number | undefined): string {
  return decimals === undefined ? v.toFixed(1) : formatDecimal(v, decimals);
}

export default function HeatmapLegend({
  bounds,
  colorTable,
  legend,
  draggable = false,
  onOffsetChange,
  decimals,
}: HeatmapLegendProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  // 잡은 지점의 범례 내부 오프셋(px). 드래그 시작 시 커서가 튀지 않도록 유지한다.
  const grabRef = useRef<{ dx: number; dy: number } | null>(null);
  const [dragging, setDragging] = useState(false);

  /**
   * 포인터 위치 → 컨테이너 기준 정규화 좌표(범례 좌상단). 범례 크기를 빼서 clamp 하므로
   * 컨테이너 밖으로 끌어내 사라지게 만들 수 없다. 컨테이너는 범례의 부모(스택 레이어)다.
   */
  function resolveOffset(clientX: number, clientY: number): SensorPosition | null {
    const el = rootRef.current;
    const parent = el?.parentElement;
    const grab = grabRef.current;
    if (!el || !parent || !grab) return null;
    const rect = parent.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const box = el.getBoundingClientRect();
    // 범례가 컨테이너 안에 완전히 남도록 상한을 좁힌다(폭/높이가 컨테이너보다 크면 0).
    const maxX = Math.max(0, (rect.width - box.width) / rect.width);
    const maxY = Math.max(0, (rect.height - box.height) / rect.height);
    const nx = (clientX - grab.dx - rect.left) / rect.width;
    const ny = (clientY - grab.dy - rect.top) / rect.height;
    return { x: Math.min(clamp01(nx), maxX), y: Math.min(clamp01(ny), maxY) };
  }

  function handlePointerDown(e: React.PointerEvent<HTMLDivElement>) {
    if (!draggable || !onOffsetChange) return;
    const box = e.currentTarget.getBoundingClientRect();
    grabRef.current = { dx: e.clientX - box.left, dy: e.clientY - box.top };
    e.currentTarget.setPointerCapture(e.pointerId);
    e.preventDefault();
    setDragging(true);
  }

  function handlePointerMove(e: React.PointerEvent<HTMLDivElement>) {
    if (!dragging) return;
    const next = resolveOffset(e.clientX, e.clientY);
    if (next) onOffsetChange?.(next);
  }

  function handlePointerUp(e: React.PointerEvent<HTMLDivElement>) {
    if (!dragging) return;
    const next = resolveOffset(e.clientX, e.clientY);
    if (next) onOffsetChange?.(next);
    grabRef.current = null;
    setDragging(false);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  }

  const isVertical = legend.orientation === 'vertical';
  const preset = SIZE_PRESETS[legend.size];
  const { min, max } = bounds;
  const degenerate = !(max > min); // min==max 또는 역전 → 단일 라벨.

  // 히트맵과 동일 color_table 사용(별도 색 계산 없음). 비면 기본 gradient 폴백.
  const table = colorTable.length > 0 ? colorTable : DEFAULT_COLOR_TABLE;
  const stops = [...table].sort((a, b) => a.stop - b.stop);
  // 세로는 아래(min)→위(max) = 'to top', 가로는 좌(min)→우(max) = 'to right'.
  const dir = isVertical ? 'to top' : 'to right';
  const gradient = `linear-gradient(${dir}, ${stops
    .map((s) => `${s.color} ${(s.stop * 100).toFixed(1)}%`)
    .join(', ')})`;

  // 등간 눈금 값: min + (max-min)*i/(n-1), i=0..n-1. 퇴화 시 단일 라벨(min).
  const tickCount = legend.tick_count;
  const ticks: number[] = degenerate
    ? [min]
    : Array.from({ length: tickCount }, (_, i) => min + ((max - min) * i) / (tickCount - 1));

  // 막대 스타일(방향별 길이/두께 축 교차).
  const barStyle = isVertical
    ? { width: preset.thickness, height: preset.length, background: gradient }
    : { width: preset.length, height: preset.thickness, background: gradient };

  const ariaLabel = t('dashboard.heatmap.legendAria')
    .replace('{min}', formatTick(min, decimals))
    .replace('{max}', formatTick(max, decimals));

  // 자유 위치(드래그로 옮긴 좌표)가 있으면 모서리 프리셋 대신 % 배치를 쓴다. 정규화 좌표라
  // 패널 리사이즈/도면 교체에 불변이다(센서 마커와 동일 규약).
  const offset = legend.offset;
  const freePos = offset !== undefined;

  return (
    <div
      ref={rootRef}
      data-testid="heatmap-legend"
      role="img"
      aria-label={ariaLabel}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      className={cn(
        'absolute z-[25] flex rounded-md bg-(--color-bg-elevated) p-1.5 shadow',
        // 뷰어(비편집)에서는 기존과 동일하게 이벤트를 통과시킨다 — 아래 레이어 클릭 보존.
        draggable
          ? cn('pointer-events-auto touch-none select-none', dragging ? 'cursor-grabbing ring-2 ring-blue-400' : 'cursor-grab')
          : 'pointer-events-none',
        !freePos && POSITION_CLASSES[legend.position],
        isVertical ? 'flex-row items-stretch gap-1' : 'flex-col gap-1',
      )}
      style={freePos ? { left: `${offset.x * 100}%`, top: `${offset.y * 100}%` } : undefined}
    >
      {/* 그라디언트 막대(히트맵과 동일 색상표). */}
      <div className="rounded-sm" style={barStyle} />

      {/* 등간 눈금 라벨: 세로는 막대 오른쪽, 가로는 막대 아래. 막대 길이축에 절대 배치. */}
      <div
        className="relative text-(--color-text-secondary)"
        style={
          isVertical
            ? { height: preset.length, fontSize: preset.font }
            : { width: preset.length, fontSize: preset.font }
        }
      >
        {ticks.map((v, i) => {
          // 세로: i=0(min) 아래(top:100%) → i=last(max) 위(top:0%).
          // 가로: i=0(min) 좌(left:0%) → i=last(max) 우(left:100%).
          const frac = degenerate ? 0 : i / (tickCount - 1);
          const posStyle = isVertical
            ? { top: `${(1 - frac) * 100}%`, transform: 'translateY(-50%)' }
            : { left: `${frac * 100}%`, transform: 'translateX(-50%)' };
          return (
            <span
              key={i}
              data-testid={`heatmap-legend-tick-${i}`}
              className={cn(
                'absolute whitespace-nowrap leading-none',
                isVertical ? 'left-0' : 'top-0',
              )}
              style={posStyle}
            >
              {formatTick(v, decimals)}
            </span>
          );
        })}
      </div>
    </div>
  );
}
