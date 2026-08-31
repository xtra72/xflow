// 캔들 모양 — 꼬리 좌표 역산이 핵심이다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import { candleKey, makeValueToPixel } from './candle';
import { CandleShape } from './CandleShape';

describe('makeValueToPixel — 몸통에서 축척을 역산한다', () => {
  it('몸통 값 범위가 픽셀 범위에 대응한다', () => {
    // 값 [10,20] 이 픽셀 y=100(위) ~ y=200(아래) 에 대응.
    const f = makeValueToPixel(10, 20, 100, 100)!;
    expect(f(20)).toBe(100); // 위쪽 = 큰 값
    expect(f(10)).toBe(200); // 아래쪽 = 작은 값
    expect(f(15)).toBe(150);
  });

  it('몸통 밖의 값도 같은 축척으로 늘린다 — 꼬리가 그렇다', () => {
    const f = makeValueToPixel(10, 20, 100, 100)!;
    expect(f(25)).toBe(50);
    expect(f(5)).toBe(250);
  });

  it('높이 0 인 몸통에서는 축척을 구할 수 없다', () => {
    expect(makeValueToPixel(10, 10, 100, 100)).toBeUndefined();
    expect(makeValueToPixel(20, 10, 100, 100)).toBeUndefined();
  });
});

function renderShape(payload: Record<string, unknown>) {
  const { container } = render(
    <svg>
      <CandleShape x={10} y={100} width={8} height={100} payload={payload} seriesKey="temp" />
    </svg>,
  );
  return container;
}

const full = {
  [candleKey('temp', 'open')]: 10,
  [candleKey('temp', 'high')]: 25,
  [candleKey('temp', 'low')]: 5,
  [candleKey('temp', 'close')]: 20,
};

describe('CandleShape', () => {
  it('몸통과 꼬리를 함께 그린다', () => {
    const c = renderShape(full);
    expect(c.querySelector('rect')).not.toBeNull();
    expect(c.querySelector('line')).not.toBeNull();
  });

  it('꼬리는 고가에서 저가까지 같은 축척으로 뻗는다', () => {
    const line = renderShape(full).querySelector('line')!;
    // 몸통 [10,20] → y 200~100. 고가 25 → 50, 저가 5 → 250.
    expect(line.getAttribute('y1')).toBe('50');
    expect(line.getAttribute('y2')).toBe('250');
  });

  it('오르막·내리막의 색이 다르다', () => {
    const up = renderShape(full).querySelector('rect')!.getAttribute('fill');
    const down = renderShape({ ...full, [candleKey('temp', 'close')]: 8 })
      .querySelector('rect')!
      .getAttribute('fill');
    expect(up).not.toBe(down);
  });

  it('값이 하나라도 없으면 아무것도 그리지 않는다', () => {
    const { [candleKey('temp', 'high')]: _omit, ...missing } = full;
    expect(renderShape(missing).querySelector('rect')).toBeNull();
  });

  it('몸통 높이가 0 이면 꼬리를 생략한다 — 축척을 구할 수 없다', () => {
    const flat = { ...full, [candleKey('temp', 'close')]: 10 };
    const c = renderShape(flat);
    expect(c.querySelector('line')).toBeNull();
    // 몸통은 최소 1px 로 남아 존재를 보인다.
    expect(c.querySelector('rect')).not.toBeNull();
  });
});
