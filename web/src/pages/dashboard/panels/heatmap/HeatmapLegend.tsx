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
// 자리와 크기는 **설정 항목이 아니라 조작 결과**다. 자리는 본문을 끌어서(legend.offset),
// 크기는 모서리 손잡이를 끌어서(bar_length/bar_thickness) 정한다 — 드롭박스로 고르던 4모서리
// 프리셋과 S/M/L 프리셋은 화면을 보면서 맞추는 일을 설정 창으로 밀어내고 있었다. 두 프리셋
// (position/size)은 config 에 남아 **이미 저장된 패널의 파생 원본**으로만 쓰인다(하위호환).
//
// 눈금 라벨은 막대와 겹치지 않는다. 라벨 상자에 실제 크기를 주어 범례 배경이 라벨까지 감싸고,
// 양 끝 라벨은 막대 밖으로 삐져나가지 않도록 끝에서 정렬을 바꾼다(가운데 정렬 유지 시 절반이
// 막대 범위를 벗어난다).

import { useRef, useState } from 'react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import {
  MAX_LEGEND_BAR_LENGTH,
  MAX_LEGEND_BAR_THICKNESS,
  MIN_LEGEND_BAR_LENGTH,
  MIN_LEGEND_BAR_THICKNESS,
  type ColorStop,
  type LegendConfig,
  type SensorPosition,
} from './heatmapConfig';
import { DEFAULT_COLOR_TABLE } from './idw';
import { clamp01 } from './placement';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import { formatDecimal } from '@/pages/dashboard/panels/charts/decimalPlaces';

/** 크기 프리셋(막대 길이/두께/폰트, px) — 치수를 직접 저장하지 않은 기존 패널의 파생 원본. */
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

/** 크기 조절 중인 드래그 상태(시작 지점 + 시작 치수). */
interface ResizeState {
  startX: number;
  startY: number;
  length: number;
  thickness: number;
}

/** 값을 [lo, hi] 로 죈다. */
function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}

interface HeatmapLegendProps {
  /** 값 범위(색 매핑 clamp 범위와 동일). */
  bounds: { min: number; max: number };
  /** 히트맵과 동일 색상표(정렬 전 stop 0..1). 비면 기본 gradient 폴백. */
  colorTable: ColorStop[];
  /** 범례 설정(파싱 완료 — enabled/orientation/position/size/tick_count). */
  legend: LegendConfig;
  /**
   * 드래그 이동 허용(배치 편집 중에만 true). false(기본)면 pointer-events-none 이 유지돼
   * 뷰어 동작이 기존과 동일하다. 크기 조절 손잡이도 이때만 나타난다.
   */
  draggable?: boolean;
  /** 드래그 결과 자유 위치(정규화 0..1) 저장. draggable 일 때만 호출된다. */
  onOffsetChange?: (offset: SensorPosition) => void;
  /** 손잡이 드래그 결과 막대 치수(px) 저장. draggable 일 때만 호출된다. */
  onSizeChange?: (size: { bar_length: number; bar_thickness: number }) => void;
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
  onSizeChange,
  decimals,
}: HeatmapLegendProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  // 잡은 지점의 범례 내부 오프셋(px). 드래그 시작 시 커서가 튀지 않도록 유지한다.
  const grabRef = useRef<{ dx: number; dy: number } | null>(null);
  const resizeRef = useRef<ResizeState | null>(null);
  const [dragging, setDragging] = useState(false);
  const [resizing, setResizing] = useState(false);

  const isVertical = legend.orientation === 'vertical';
  const preset = SIZE_PRESETS[legend.size];
  // 치수·글자 서식은 저장값이 이기고, 없으면 프리셋에서 파생한다(기존 패널 그림 보존).
  const barLength = legend.bar_length ?? preset.length;
  const barThickness = legend.bar_thickness ?? preset.thickness;
  const font = legend.font_size ?? preset.font;

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

  // --- 크기 조절(모서리 손잡이) ---
  // 손잡이는 범례 오른쪽 아래 모서리에 있으므로, 오른쪽·아래로 끌면 커진다. 어느 축이 길이이고
  // 어느 축이 두께인지는 방향이 정한다(세로 막대는 아래로 길어지고 옆으로 두꺼워진다).
  // 이동 드래그와 같은 누름을 나눠 갖지 않도록 전파를 끊는다.
  function handleResizeDown(e: React.PointerEvent<HTMLSpanElement>) {
    if (!draggable || !onSizeChange) return;
    e.preventDefault();
    e.stopPropagation();
    resizeRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      length: barLength,
      thickness: barThickness,
    };
    // 도면 이동 오버레이와 같은 방식으로 선택적 호출 — jsdom 등 capture API 가 없는 환경에서
    // 여기서 던지면 손잡이 자체가 못 쓰게 된다. 캡처는 있으면 좋은 보강이지 전제가 아니다.
    e.currentTarget.setPointerCapture?.(e.pointerId);
    setResizing(true);
  }

  function handleResizeMove(e: React.PointerEvent<HTMLSpanElement>) {
    const st = resizeRef.current;
    if (!st) return;
    e.stopPropagation();
    const dx = e.clientX - st.startX;
    const dy = e.clientY - st.startY;
    const dLength = isVertical ? dy : dx;
    const dThickness = isVertical ? dx : dy;
    onSizeChange?.({
      bar_length: clamp(st.length + dLength, MIN_LEGEND_BAR_LENGTH, MAX_LEGEND_BAR_LENGTH),
      bar_thickness: clamp(
        st.thickness + dThickness,
        MIN_LEGEND_BAR_THICKNESS,
        MAX_LEGEND_BAR_THICKNESS,
      ),
    });
  }

  function handleResizeUp(e: React.PointerEvent<HTMLSpanElement>) {
    if (!resizeRef.current) return;
    e.stopPropagation();
    handleResizeMove(e);
    resizeRef.current = null;
    setResizing(false);
    if (e.currentTarget.hasPointerCapture?.(e.pointerId)) {
      e.currentTarget.releasePointerCapture?.(e.pointerId);
    }
  }

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

  const labels = ticks.map((v) => formatTick(v, decimals));

  // 라벨 상자의 실제 크기. 라벨이 전부 absolute 라 상자를 비워 두면 폭/높이가 0 이 되어
  // 범례 배경이 라벨을 감싸지 못하고 글자가 막대 바로 옆에 떠 히트맵과 뒤엉킨다(보고된 겹침).
  // 폭은 가장 긴 라벨을 글자 크기로 어림한다 — 숫자·부호·소수점은 대략 0.6em 이다.
  const widestLabel = labels.reduce((n, s) => Math.max(n, s.length), 1);
  const labelBoxWidth = Math.ceil(widestLabel * font * 0.6);
  const labelBoxHeight = Math.ceil(font * 1.2);
  // 막대와 라벨 사이 여백도 글자 크기를 따라간다 — 고정 4px 은 글자를 키울수록 붙어 보인다.
  const labelGap = Math.max(4, Math.round(font * 0.5));

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
          ? cn(
              'pointer-events-auto touch-none select-none',
              dragging ? 'cursor-grabbing ring-2 ring-blue-400' : 'cursor-grab',
              resizing && 'ring-2 ring-blue-400',
            )
          : 'pointer-events-none',
        !freePos && POSITION_CLASSES[legend.position],
        isVertical ? 'flex-row items-stretch' : 'flex-col',
      )}
      style={{
        ...(freePos ? { left: `${offset.x * 100}%`, top: `${offset.y * 100}%` } : undefined),
        gap: labelGap,
      }}
    >
      {/* 그라디언트 막대(히트맵과 동일 색상표). */}
      <div
        className="rounded-sm"
        style={
          isVertical
            ? { width: barThickness, height: barLength, background: gradient }
            : { width: barLength, height: barThickness, background: gradient }
        }
      />

      {/* 등간 눈금 라벨: 세로는 막대 오른쪽, 가로는 막대 아래. 막대 길이축에 절대 배치. */}
      <div
        data-testid="heatmap-legend-ticks"
        className="relative"
        style={
          isVertical
            ? { width: labelBoxWidth, height: barLength, fontSize: font, color: legend.font_color }
            : { width: barLength, height: labelBoxHeight, fontSize: font, color: legend.font_color }
        }
      >
        {labels.map((label, i) => {
          // 세로: i=0(min) 아래(top:100%) → i=last(max) 위(top:0%).
          // 가로: i=0(min) 좌(left:0%) → i=last(max) 우(left:100%).
          const frac = degenerate ? 0 : i / (tickCount - 1);
          // 양 끝은 가운데 정렬을 버린다 — 유지하면 라벨의 절반이 막대 범위 밖으로 나가
          // 범례 상자를 넘치고, 아래 히트맵 위에 글자가 떠서 겹쳐 보인다.
          const atStart = i === 0;
          const atEnd = !degenerate && i === tickCount - 1;
          const along = atStart ? '0%' : atEnd ? '-100%' : '-50%';
          const posStyle = isVertical
            ? // 세로는 위치축이 반대라 시작(min)이 아래쪽이다.
              { top: `${(1 - frac) * 100}%`, transform: `translateY(${atStart ? '-100%' : atEnd ? '0%' : '-50%'})` }
            : { left: `${frac * 100}%`, transform: `translateX(${along})` };
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
              {label}
            </span>
          );
        })}
      </div>

      {/* 크기 조절 손잡이 — 편집 중에만. 오른쪽 아래 모서리라 "바깥으로 끌면 커진다" 가 성립한다. */}
      {draggable && onSizeChange && (
        <span
          data-testid="heatmap-legend-resize"
          role="slider"
          aria-label={t('dashboard.heatmap.legendResizeAria')}
          aria-valuenow={barLength}
          aria-valuemin={MIN_LEGEND_BAR_LENGTH}
          aria-valuemax={MAX_LEGEND_BAR_LENGTH}
          tabIndex={-1}
          onPointerDown={handleResizeDown}
          onPointerMove={handleResizeMove}
          onPointerUp={handleResizeUp}
          onPointerCancel={handleResizeUp}
          className="absolute -bottom-1 -right-1 h-2.5 w-2.5 cursor-nwse-resize rounded-sm border border-white bg-blue-500 shadow"
        />
      )}
    </div>
  );
}
