// 캔들 하나를 그리는 모양 함수.
//
// recharts 에 캔들 프리미티브가 없어 `Bar` 의 커스텀 `shape` 로 그린다. Bar 는
// 몸통(시가~종가) 구간에 사각형을 세우고, 이 함수가 그 사각형을 받아 몸통을 다시
// 그리면서 고가·저가 꼬리를 함께 얹는다.
//
// 꼬리 좌표는 몸통의 픽셀↔값 대응에서 역산한다 — Bar 가 넘겨주는 것은 몸통의
// 픽셀 상자와 그 값 범위뿐이라, 축 스케일을 따로 받지 않고도 같은 축 위에 그릴 수
// 있는 유일한 방법이다.

import {
  candleKey,
  CANDLE_DOWN_COLOR,
  CANDLE_UP_COLOR,
  makeValueToPixel,
} from './candle';

export interface CandleShapeProps {
  /** 몸통 사각형(픽셀). */
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  /** Bar 가 읽은 값 — 몸통의 [아래, 위]. */
  value?: [number, number] | number;
  /** 이 행 전체. 고가·저가를 여기서 읽는다. */
  payload?: Record<string, unknown>;
  /** 원래 시리즈 키 — 행에서 축별 값을 찾는 접두사. */
  seriesKey: string;
  /** 몸통 색. 지정하지 않으면 방향색을 쓴다. */
  color?: string;
}

export function CandleShape(props: CandleShapeProps): React.ReactElement | null {
  const { x, y, width, height, payload, seriesKey, color } = props;
  if (x === undefined || y === undefined || width === undefined || height === undefined) {
    return null;
  }

  const open = payload?.[candleKey(seriesKey, 'open')];
  const high = payload?.[candleKey(seriesKey, 'high')];
  const low = payload?.[candleKey(seriesKey, 'low')];
  const close = payload?.[candleKey(seriesKey, 'close')];
  if (
    typeof open !== 'number' ||
    typeof high !== 'number' ||
    typeof low !== 'number' ||
    typeof close !== 'number'
  ) {
    return null;
  }

  const fill = color ?? (close >= open ? CANDLE_UP_COLOR : CANDLE_DOWN_COLOR);
  const bodyLo = Math.min(open, close);
  const bodyHi = Math.max(open, close);
  const toPixel = makeValueToPixel(bodyLo, bodyHi, y, height);
  const cx = x + width / 2;

  return (
    <g>
      {/* 꼬리 — 고가에서 저가까지. 몸통 높이가 0 이면 축척을 못 구해 생략한다. */}
      {toPixel && (
        <line x1={cx} x2={cx} y1={toPixel(high)} y2={toPixel(low)} stroke={fill} strokeWidth={1} />
      )}
      {/* 몸통 — 시가와 종가가 같으면 선 하나로 남는다(높이 0). */}
      <rect
        x={x}
        y={y}
        width={width}
        height={Math.max(height, 1)}
        fill={fill}
        stroke={fill}
      />
    </g>
  );
}
