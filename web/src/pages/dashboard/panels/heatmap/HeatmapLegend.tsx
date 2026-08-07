// 값→색 색표(colorbar) 범례 오버레이 (additive).
//
// 히트맵과 **동일 color_table**(정렬된 stop 0..1)로 CSS linear-gradient 막대를 만들어 값이
// 어떤 색인지 보여준다(별도 색 계산 없음). colorTable 이 비면 DEFAULT_COLOR_TABLE(idw.ts)로
// 폴백한다. 등간 눈금 라벨을 막대 옆(세로: 오른쪽, 가로: 아래)에 배치하며, bounds 퇴화
// (min==max) 시 단일 라벨로 방어한다.
//
// 레이어 배치: HeatmapPanel 스택 최상단(z-25) + pointer-events-none 이라 편집/드래그를
// 가로막지 않는다. 편집모드와 무관하게 표시(뷰어도 범례는 봄).

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { ColorStop, LegendConfig } from './heatmapConfig';
import { DEFAULT_COLOR_TABLE } from './idw';

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
}

/** 눈금 값 표시(소수 1자리). */
function formatTick(v: number): string {
  return v.toFixed(1);
}

export default function HeatmapLegend({ bounds, colorTable, legend }: HeatmapLegendProps) {
  const { t } = useTranslation();

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
    .replace('{min}', formatTick(min))
    .replace('{max}', formatTick(max));

  return (
    <div
      data-testid="heatmap-legend"
      role="img"
      aria-label={ariaLabel}
      className={cn(
        'pointer-events-none absolute z-[25] flex rounded-md bg-(--color-bg-elevated) p-1.5 shadow',
        POSITION_CLASSES[legend.position],
        isVertical ? 'flex-row items-stretch gap-1' : 'flex-col gap-1',
      )}
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
              {formatTick(v)}
            </span>
          );
        })}
      </div>
    </div>
  );
}
