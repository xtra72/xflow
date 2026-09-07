// 게이지 **현재값 글자** — 자리 기술자와 그것을 그리는 오버레이.
//
// 값은 한때 도형과 같은 SVG 안에 있었다. 그래서 도형에 걸린 크기·위치 변형이 값까지
// 끌고 갔고, 캔버스 밖으로도 나갈 수 없었다(보고된 결함). 지금은 도형과 **형제**로
// 그리고, 오프셋은 패널 상자 대비 백분율이다.
//
// 도형 모듈(`gaugeShapes.tsx`)과 파일을 나눈 이유는 그 모듈이 컴포넌트를 내보내지 않는
// 상수·함수 모듈이기 때문이다 — 한 파일에 섞으면 Fast Refresh 가 동작하지 않는다.

import type { ReactElement } from 'react';

import { cn } from '@/lib/utils/cn';

import {
  HALF_RAINBOW_LAYOUT,
  VBAR_BOTTOM_Y,
  VBAR_CENTER_X,
  type parseConfig,
} from './gaugeShapes';

/**
 * 값 글자가 놓이는 자리와 모양 — 유형마다 다르다.
 *
 * 값을 도형과 **같은 SVG 안**에 두었을 때는 도형에 걸린 크기·위치 변형이 값까지 끌고
 * 갔고, 캔버스 밖으로도 나갈 수 없었다(보고된 결함). 그래서 값은 도형과 형제인 별도
 * 오버레이로 그린다 — 다만 **같은 viewBox 와 같은 좌표**를 쓰므로 기본 자리는 종전과
 * 한 픽셀도 다르지 않다.
 */
export interface GaugeValueLayout {
  viewBox: string;
  x: number;
  y: number;
  valueSize: number;
  unitSize: number;
  valueFill?: string;
  unitClassName?: string;
  unitOpacity?: number;
  /** 값 뒤에 까는 판(니들 유형의 어두운 배지). 값과 함께 움직여야 한다. */
  badge?: { dx: number; dy: number; width: number; height: number; rx: number; fill: string };
}

/** 유형별 값 배치. 도형 컴포넌트가 쓰던 좌표를 그대로 옮겨 온 것이다. */
export function gaugeValueLayout(parsed: ReturnType<typeof parseConfig>): GaugeValueLayout {
  const muted = 'fill-(--color-text-muted)';
  switch (parsed.gaugeType) {
    case 'half':
      return { viewBox: '0 0 240 140', x: 120, y: 90, valueSize: 24, unitSize: 12, unitClassName: muted };
    case 'needle':
      return {
        viewBox: '0 0 200 200', x: 100, y: 138, valueSize: 10, unitSize: 7,
        valueFill: '#FFFFFF', unitOpacity: 0.7,
        badge: { dx: -26, dy: -10, width: 52, height: 20, rx: 4, fill: '#1E293B' },
      };
    case 'needle-rainbow':
      return { viewBox: '0 0 220 210', x: 110, y: 129, valueSize: 12, unitSize: 8, unitClassName: muted };
    case 'vertical-bar':
      return {
        viewBox: '0 0 140 224',
        x: VBAR_CENTER_X,
        // 바 아래 끝은 치수와 무관하게 고정이므로 값 자리도 고정이다.
        y: VBAR_BOTTOM_Y + 16,
        valueSize: 13, unitSize: 8, unitClassName: muted,
      };
    case 'half-rainbow': {
      const l = HALF_RAINBOW_LAYOUT[parsed.halfRainbowDirection];
      return {
        viewBox: l.viewBox, x: l.cx + l.valueDx, y: l.cy + l.valueDy,
        valueSize: 18, unitSize: 11, unitClassName: muted,
      };
    }
    default:
      return { viewBox: '0 0 200 200', x: 100, y: 98, valueSize: 28, unitSize: 14, unitClassName: muted };
  }
}

/**
 * 값 글자 오버레이 — 도형과 **형제**로 그린다.
 *
 * 도형 상자에 걸린 변형(크기·위치)이 여기에 닿지 않으므로 값이 도형을 따라 움직이지
 * 않는다. 오프셋은 패널 상자 대비 백분율이라 도형이 아니라 **패널** 어디든 갈 수 있고,
 * `overflow-visible` 이라 캔버스 가장자리에서 잘리지도 않는다.
 */
export function GaugeValueOverlay({
  parsed,
  hasValue,
  offsetX,
  offsetY,
  edit,
}: {
  parsed: ReturnType<typeof parseConfig>;
  hasValue: boolean;
  /** 패널 상자 대비 변위(%). */
  offsetX: number;
  offsetY: number;
  edit?: { props: Record<string, unknown>; outline: string; showHandle?: boolean };
}): ReactElement {
  const l = gaugeValueLayout(parsed);
  return (
    // 상자는 패널 전체를 덮는다 — SVG 의 레터박스 계산이 기본 자리를 정하려면 도형과
    // 같은 크기여야 하기 때문이다. 다만 **포인터도 윤곽선도 이 상자에 걸지 않는다**:
    // 걸면 잡히는 영역과 편집 표시가 패널만 해져서, 값이 아니라 패널을 고른 것처럼
    // 보이고 도형 위 클릭까지 값이 가로챈다. 둘 다 글자 자신이 가진다.
    <div
      className="pointer-events-none absolute inset-0"
      // 크기 손잡이는 글자의 형제라 글자로 거슬러 올라갈 수 없다. 드래그 레이어가
      // "이 손잡이는 값의 것" 이라고 판정할 수 있도록 담는 상자에 표식을 둔다.
      data-gauge-value-box=""
      data-testid="gauge-value-overlay"
      style={{ transform: `translate(${offsetX}%, ${offsetY}%)` }}
    >
      <svg viewBox={l.viewBox} className="h-full w-full overflow-visible">
        {l.badge && (
          <rect
            x={l.x + l.badge.dx} y={l.y + l.badge.dy}
            width={l.badge.width} height={l.badge.height}
            rx={l.badge.rx} fill={l.badge.fill}
          />
        )}
        <GaugeValueText
          edit={edit}
          x={l.x}
          y={l.y}
          valueText={parsed.valueText}
          unit={parsed.unit}
          valueSize={l.valueSize}
          unitSize={l.unitSize}
          hasValue={hasValue}
          valueFill={l.valueFill}
          unitClassName={l.unitClassName}
          unitOpacity={l.unitOpacity}
          scale={parsed.valueScale}
        />
      </svg>
    </div>
  );
}

function GaugeValueText({
  x,
  y,
  valueText,
  unit,
  valueSize,
  unitSize,
  hasValue,
  valueFill,
  unitClassName,
  unitOpacity,
  scale = 1,
  offsetX = 0,
  offsetY = 0,
  edit,
}: {
  x: number;
  y: number;
  valueText: string;
  unit: string;
  valueSize: number;
  unitSize: number;
  hasValue: boolean;
  /** 값 글자색 클래스 대신 직접 지정할 때(니들 배지처럼 어두운 바탕 위). */
  valueFill?: string;
  unitClassName?: string;
  unitOpacity?: number;
  /** 글자 크기 배율(기본 1). */
  scale?: number;
  /** 가로·세로 변위(viewBox 좌표, 기본 0). */
  offsetX?: number;
  offsetY?: number;
  /** 편집 표면 — 잡히는 영역과 윤곽선은 **글자 자신**이 가져야 한다. */
  edit?: { props: Record<string, unknown>; outline: string; showHandle?: boolean };
}): ReactElement {
  const twoLine = hasValue && unit !== '';
  const vSize = valueSize * scale;
  const uSize = unitSize * scale;
  // 값과 단위의 행간.
  //
  // 종전에는 `uSize * 1.15` 로 **단위 크기만** 기준이었다. 그런데 아래로 뻗는 것은 값의
  // 디센더(≈ 값 크기의 20%)이고 위로 뻗는 것은 단위의 캡 높이(≈ 단위 크기의 70%)라,
  // 값이 단위보다 두 배 큰 기본 배치(28 / 14)에서 둘 사이가 1px 도 남지 않았다.
  // 두 크기를 함께 세어 값이 커져도 간격이 따라 벌어지게 한다.
  const lineStep = vSize * 0.35 + uSize * 0.95;
  // 두 행이면 블록 전체가 아래로 한 행 내려가므로 시작점을 그 절반만큼 올린다.
  const startY = (twoLine ? y - lineStep / 2 : y) + offsetY;
  const cx = x + offsetX;
  // 글자 블록의 아래 끝(baseline + 디센더 몫). 손잡이를 여기에 붙인다.
  const bottomY = startY + (twoLine ? lineStep : 0) + vSize * 0.4;
  return (
    <>
    <text
      // 설정 미리보기의 드래그 레이어가 이 표시로 "값 글자를 잡았다" 를 판정한다.
      // 대시보드에 놓인 패널에는 드래그 레이어가 없으므로 표시만 남고 아무 일도 없다.
      data-gauge-value-text=""
      x={cx}
      y={startY}
      textAnchor="middle"
      dominantBaseline="central"
      className={cn(
        valueFill ? undefined : 'fill-(--color-text-primary)',
        // 담는 상자는 포인터를 받지 않으므로 글자가 스스로 받아야 한다.
        edit && 'pointer-events-auto',
        edit?.outline,
      )}
      fill={valueFill}
      fontWeight={700}
      {...(edit?.props ?? {})}
    >
      <tspan x={cx} fontSize={vSize}>
        {hasValue ? valueText : '--'}
      </tspan>
      {twoLine && (
        <tspan
          x={cx}
          dy={lineStep}
          fontSize={uSize}
          className={unitClassName}
          opacity={unitOpacity}
        >
          {unit}
        </tspan>
      )}
    </text>
    {edit?.showHandle && (
      <GaugeValueResizeHandle cx={cx} bottomY={bottomY} size={twoLine ? uSize : vSize} />
    )}
    </>
  );
}

/**
 * 값 글자의 크기 손잡이 — 글자 **바로 아래 가운데**에 둔다.
 *
 * 다른 요소처럼 오른쪽 아래 모서리에 두지 못하는 이유는 글자 폭을 알 수 없기 때문이다.
 * 값은 가운데 정렬이라 폭이 자릿수에 따라 매번 달라지고, 그것을 알려면 그린 뒤 재야
 * 하는데 그러면 렌더가 한 번 더 돌고 테스트에서는 아예 0 이 나온다. 세로 방향은 글자
 * 크기에서 정확히 계산되므로, 아래 가운데는 자릿수와 무관하게 늘 글자에 붙어 있다.
 *
 * viewBox 좌표라 캔버스와 함께 커지고 작아진다 — 손잡이가 글자에 대해 늘 같은 비율로
 * 보이는 편이, 화면 픽셀로 고정되어 작은 패널에서 글자를 덮는 것보다 낫다.
 */
function GaugeValueResizeHandle({
  cx,
  bottomY,
  size,
}: {
  cx: number;
  bottomY: number;
  size: number;
}): ReactElement {
  const r = Math.max(3, size * 0.28);
  return (
    <rect
      data-panel-resize="value"
      data-testid="panel-resize-value"
      x={cx - r}
      y={bottomY + r}
      width={r * 2}
      height={r * 2}
      rx={r * 0.4}
      className="pointer-events-auto cursor-nwse-resize"
      fill="#3b82f6"
      stroke="#FFFFFF"
      strokeWidth={r * 0.25}
    />
  );
}

// ---- 게이지 렌더러 ----

/** 1. Simple Gauge (도넛형) — 360° 도넛 */
