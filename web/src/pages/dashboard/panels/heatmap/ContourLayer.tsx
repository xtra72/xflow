// 등고선(contour lines) SVG 오버레이 레이어 (SPEC-HEATMAP-PANEL-003 T3).
//
// HeatmapPanel 이 계산한 **동일 스칼라 격자(field)**를 입력으로 marching squares 를 수행해
// 등치선 경로를 SVG <path> 로 렌더한다(재보간 금지, R3/AC-E5). field 참조 기준 useMemo 로
// 격자 불변 시 등고선 재계산을 생략한다(AC-E4). 히트맵/센서와의 정렬은 viewBox 0..1 +
// preserveAspectRatio="none" 으로 자동 처리되므로 리사이즈 시 추가 코드가 필요 없다(R5).
//
// 레이어 배치: 히트맵(z-10) 위, 센서 마커 오버레이(z-20) 아래(z-15). pointer-events-none 이라
// 편집 상호작용을 가로막지 않는다. 등고선 off/빈 격자/레벨 0개 → null(예외 없음, AC-E2).
//
// @spec SPEC-HEATMAP-PANEL-003

import { useMemo } from 'react';

import { resolveLevels, computeContours, segmentsToPath } from './marchingSquares';
import type { ContourConfig } from './heatmapConfig';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import { formatDecimal } from '@/pages/dashboard/panels/charts/decimalPlaces';

/** 미지정 시 선 색 기본값(히트맵 위에서 대비되는 진한 슬레이트). */
export const DEFAULT_CONTOUR_LINE_COLOR = '#334155';
/** 미지정 시 선 두께 기본값(px, non-scaling-stroke). */
export const DEFAULT_CONTOUR_LINE_WIDTH = 1;
/** 라벨 글자 크기(정규화 user-unit). viewBox 0..1 기준 소형. */
const LABEL_FONT_SIZE = 0.03;

interface ContourLayerProps {
  /** HeatmapPanel 이 계산한 스칼라 격자(행 우선). 재보간 없이 그대로 소비. */
  field: Float32Array;
  /** 격자 폭. */
  gridW: number;
  /** 격자 높이. */
  gridH: number;
  /** 격자 값 범위(레벨 산출 기준). */
  bounds: { min: number; max: number };
  /** 등고선 설정(파싱 완료 — enabled/level_count/levels/line/labels). */
  contour: ContourConfig;
  /**
   * 라벨 자릿수. 사용자가 패널 설정에서 **직접 지정했을 때만** 전달된다.
   * `undefined` 면 종전 표기(정수는 그대로, 소수는 1자리)를 유지한다.
   */
  decimals?: number;
}

/**
 * 등치값 라벨 표시 문자열.
 *
 * 등고선 라벨은 값 읽기가 아니라 **등치선의 눈금**이므로 자릿수 기본값(2)을 따르지
 * 않는다 — 레벨은 대개 반듯한 수(20 · 22 · 24)라 기본을 걸면 `20.00` 이 된다.
 * 사용자가 직접 지정했을 때만 따라간다(범례 눈금과 같은 규칙).
 *
 * 미지정 시 종전 표기: 정수는 그대로, 소수는 소수 1자리.
 */
function formatLevel(level: number, decimals: number | undefined): string {
  if (decimals !== undefined) return formatDecimal(level, decimals);
  return Number.isInteger(level) ? String(level) : level.toFixed(1);
}

/** 레벨별 렌더 기술자(path d + 라벨 대표점). */
interface LevelPath {
  level: number;
  d: string;
  labelX: number;
  labelY: number;
}

export default function ContourLayer({
  field,
  gridW,
  gridH,
  bounds,
  contour,
  decimals,
}: ContourLayerProps) {
  // field 참조 기준 memoize: 격자/레벨/범위 불변 시 marching squares 재계산 생략(AC-E4).
  const levelPaths = useMemo<LevelPath[]>(() => {
    if (!contour.enabled) return [];
    if (gridW <= 0 || gridH <= 0 || field.length === 0) return [];
    const levels = resolveLevels(bounds, contour.level_count, contour.levels);
    if (levels.length === 0) return [];

    const out: LevelPath[] = [];
    for (const level of levels) {
      const segs = computeContours(field, gridW, gridH, level);
      if (segs.length === 0) continue; // 이 레벨은 격자에 등치선이 없다(정상, 건너뜀).
      const first = segs[0]!;
      out.push({
        level,
        d: segmentsToPath(segs),
        // 라벨 대표점: 첫 세그먼트 중점(기본 배치 — 라벨 충돌 회피 고도화는 후속 SPEC).
        labelX: (first.x1 + first.x2) / 2,
        labelY: (first.y1 + first.y2) / 2,
      });
    }
    return out;
  }, [field, gridW, gridH, bounds, contour]);

  // off/빈 격자/레벨 0개 → 렌더 없음(예외 없음, AC-E2).
  if (!contour.enabled || gridW <= 0 || gridH <= 0 || levelPaths.length === 0) return null;

  const stroke = contour.line.color ?? DEFAULT_CONTOUR_LINE_COLOR;
  const strokeWidth = contour.line.width ?? DEFAULT_CONTOUR_LINE_WIDTH;
  const dash =
    contour.line.dash && contour.line.dash.length > 0 ? contour.line.dash.join(' ') : undefined;

  return (
    <svg
      data-testid="contour-layer"
      className="pointer-events-none absolute inset-0 z-[15] h-full w-full"
      viewBox="0 0 1 1"
      preserveAspectRatio="none"
    >
      {levelPaths.map((lp, i) => (
        <path
          key={`p-${i}`}
          data-testid={`contour-path-${i}`}
          d={lp.d}
          fill="none"
          stroke={stroke}
          strokeWidth={strokeWidth}
          strokeDasharray={dash}
          // 두께를 표시 픽셀 기준으로 유지(viewBox 스케일에 안 늘어남).
          vectorEffect="non-scaling-stroke"
        />
      ))}
      {contour.labels &&
        levelPaths.map((lp, i) => (
          <text
            key={`l-${i}`}
            data-testid={`contour-label-${i}`}
            x={lp.labelX}
            y={lp.labelY}
            fill={stroke}
            fontSize={LABEL_FONT_SIZE}
            textAnchor="middle"
            dominantBaseline="middle"
          >
            {formatLevel(lp.level, decimals)}
          </text>
        ))}
    </svg>
  );
}
