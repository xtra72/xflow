// 값 글자 자리의 **옛 좌표 읽기**(SPEC-CHART-005 IN-5 회귀 복구).
//
// 값이 도형 밖 오버레이로 나가면서 변위의 좌표계가 viewBox 단위(`value_offset_x/y`)
// 에서 패널 대비 백분율(`value_pos_x/y`)로 바뀌었는데, 옛 키를 읽는 코드가 남지 않아
// 저장된 대시보드에서 끌어 옮긴 값 자리가 기본 위치로 되돌아갔다.
//
// 두 좌표를 잇는 상수는 config 만으로는 알 수 없다(SVG meet 배율이 그려진 상자의
// 종횡비에 달렸다). 그래서 옛 값은 환산하지 않고 **옛 좌표 그대로 SVG 안에서** 그린다.
// 여기서 잠그는 것은 두 가지다 — 순수 모듈의 읽기·죄기 규칙, 그리고 그 값이 실제로
// 글자 좌표에 실리는가.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import { PANEL_OFFSET_LIMIT } from '../charts/panelGeometry';
import { STAT_OFFSET_LIMIT } from '../charts/statLayout';
import { parseConfig } from './gaugeShapes';
import { GaugeValueOverlay } from './gaugeValue';
import {
  clampViewBoxOffset,
  parseViewBox,
  resolveValuePlacement,
} from './valueOffsetMigration';

describe('viewBox 파싱', () => {
  it('네 수를 폭·높이로 읽는다', () => {
    expect(parseViewBox('0 0 240 140')).toEqual({ w: 240, h: 140 });
    // 쉼표 구분과 여분 공백도 SVG 가 받아들이는 형식이다.
    expect(parseViewBox(' 0, 0, 200, 200 ')).toEqual({ w: 200, h: 200 });
  });

  it('형식이 아니면 null — 옛 자리를 못 죄느니 안 옮기는 편이 낫다', () => {
    expect(parseViewBox(null)).toBeNull();
    expect(parseViewBox('')).toBeNull();
    expect(parseViewBox('0 0 200')).toBeNull();
    expect(parseViewBox('0 0 200 abc')).toBeNull();
    // 폭·높이가 0 이면 배율을 구할 수 없다.
    expect(parseViewBox('0 0 0 200')).toBeNull();
  });
});

describe('옛 변위 죄기', () => {
  it('한 변의 절반까지 — 그보다 밀면 캔버스 밖으로 나가 다시 잡을 수 없다', () => {
    expect(clampViewBoxOffset(30, 200)).toBe(30);
    expect(clampViewBoxOffset(500, 200)).toBe(100);
    expect(clampViewBoxOffset(-500, 200)).toBe(-100);
  });

  it('수가 아니거나 기준 변이 없으면 0', () => {
    expect(clampViewBoxOffset(Number.NaN, 200)).toBe(0);
    expect(clampViewBoxOffset(10, 0)).toBe(0);
  });
});

describe('config 에서 값 자리를 읽는다', () => {
  it('옛 키만 있으면 옛 좌표로 살아난다 — 이관 누락으로 잃었던 자리', () => {
    expect(resolveValuePlacement({ value_offset_x: 12, value_offset_y: -8 })).toEqual({
      percentX: 0,
      percentY: 0,
      viewBoxX: 12,
      viewBoxY: -8,
    });
  });

  it('신규 키만 있으면 백분율만 쓴다', () => {
    expect(resolveValuePlacement({ value_pos_x: 15, value_pos_y: -20 })).toEqual({
      percentX: 15,
      percentY: -20,
      viewBoxX: 0,
      viewBoxY: 0,
    });
  });

  it('둘 다 있으면 **더한다** — 옛 자리가 새 조작의 출발점이다', () => {
    // 신규가 옛것을 덮으면, 옛 config 를 처음 끌거나 정렬하는 순간 옛 몫이 사라져
    // 값이 그만큼 튄다: 끌기·정렬은 화면에서 잰 자리(옛 몫이 이미 들어간 자리)를
    // 기준으로 새 백분율을 계산하는데 저장되는 것은 그 백분율뿐이기 때문이다.
    expect(resolveValuePlacement({ value_pos_x: 5, value_pos_y: 5, value_offset_x: 12, value_offset_y: -8 })).toEqual({
      percentX: 5,
      percentY: 5,
      viewBoxX: 12,
      viewBoxY: -8,
    });
  });

  it('아무 키도 없으면 기본 자리', () => {
    expect(resolveValuePlacement({})).toEqual({
      percentX: 0,
      percentY: 0,
      viewBoxX: 0,
      viewBoxY: 0,
    });
  });

  it('수가 아닌 값은 무시한다 — 손으로 고친 config 가 값을 날리지 않는다', () => {
    expect(
      resolveValuePlacement({
        value_pos_x: '15',
        value_pos_y: Number.POSITIVE_INFINITY,
        value_offset_x: null,
        value_offset_y: Number.NaN,
      }),
    ).toEqual({ percentX: 0, percentY: 0, viewBoxX: 0, viewBoxY: 0 });
  });

  it('백분율은 ±50 까지 죈다 — 값 글자는 게이지 상자가 아니라 글자 덩어리다', () => {
    // 게이지 상자(±40)와 다르다. 값은 도형 영역에 갇히지 않는 작은 글자 덩어리이고
    // 흐름상 가운데에서 시작하므로, 패널 어느 모서리에든 놓으려면 축마다 50 이 필요하다.
    const p = resolveValuePlacement({ value_pos_x: 999, value_pos_y: -999 });
    expect(p.percentX).toBe(STAT_OFFSET_LIMIT);
    expect(p.percentY).toBe(-STAT_OFFSET_LIMIT);
    // 게이지 상자의 상한과 같은 값이 아니어야 한다 — 같으면 이 구분이 사라진 것이다.
    expect(STAT_OFFSET_LIMIT).toBeGreaterThan(PANEL_OFFSET_LIMIT);
  });

  it('40 과 50 사이에 저장된 자리를 그대로 읽는다 — 끌어 놓은 곳으로 되돌아간다', () => {
    // 끄는 쪽(`GaugeDragLayer`)이 50 까지 저장하므로 읽기도 50 이어야 한다. 40 으로
    // 읽으면 사용자가 45 에 놓아 둔 값이 다음에 열 때 40 으로 튄다(보고된 결함).
    const p = resolveValuePlacement({ value_pos_x: 45, value_pos_y: -45 });
    expect(p.percentX).toBe(45);
    expect(p.percentY).toBe(-45);
  });
});

// ---------------------------------------------------------------------------
// 읽은 값이 실제로 글자에 실리는가.
// ---------------------------------------------------------------------------

/** 값 글자(tspan 을 가진 유일한 text)의 좌표와 오버레이 transform. */
function valuePosition(
  props: Partial<React.ComponentProps<typeof GaugeValueOverlay>>,
): { x: number; y: number; transform: string | undefined } {
  const view = render(
    <GaugeValueOverlay
      parsed={parseConfig({ value: 50, unit: '%' })}
      hasValue
      offsetX={0}
      offsetY={0}
      {...props}
    />,
  );
  const el = Array.from(view.container.querySelectorAll('text')).find(
    (t) => t.querySelectorAll('tspan').length > 0,
  )!;
  const out = {
    x: Number(el.getAttribute('x')),
    y: Number(el.getAttribute('y')),
    transform: view.getByTestId('gauge-value-overlay').style.transform || undefined,
  };
  view.unmount();
  return out;
}

describe('옛 변위가 글자 좌표에 실린다', () => {
  const base = valuePosition({});

  it('옛 변위는 viewBox 좌표로 글자에 더해진다 — 배율은 SVG 가 건다', () => {
    const moved = valuePosition({ viewBoxOffsetX: 20, viewBoxOffsetY: -10 });
    expect(moved.x).toBe(base.x + 20);
    expect(moved.y).toBe(base.y - 10);
    // 옛 변위는 상자 transform 을 건드리지 않는다 — 백분율과 좌표계가 다르다.
    expect(moved.transform).toBe(base.transform);
  });

  it('신규 변위는 상자 transform 으로, 글자 좌표는 그대로', () => {
    const moved = valuePosition({ offsetX: 10, offsetY: -5 });
    expect(moved.x).toBe(base.x);
    expect(moved.y).toBe(base.y);
    expect(moved.transform).toContain('translate(10%, -5%)');
  });

  it('둘 다 주면 각자의 좌표계에 실려 함께 움직인다', () => {
    const moved = valuePosition({ offsetX: 10, offsetY: -5, viewBoxOffsetX: 20, viewBoxOffsetY: -10 });
    expect(moved.x).toBe(base.x + 20);
    expect(moved.y).toBe(base.y - 10);
    expect(moved.transform).toContain('translate(10%, -5%)');
  });

  it('옛 변위는 그 유형의 viewBox 절반까지만 — 기본 유형은 200 이므로 ±100', () => {
    const moved = valuePosition({ viewBoxOffsetX: 9999, viewBoxOffsetY: -9999 });
    expect(moved.x).toBe(base.x + 100);
    expect(moved.y).toBe(base.y - 100);
  });

  it('40 을 넘겨 저장된 자리도 그 자리에 그려진다 — 읽기와 쓰기의 상한이 같다', () => {
    // config 부터 화면까지 한 번에 잠근다: 45 로 저장된 값은 45%에 그려지고,
    // 손으로 고친 999 는 상한(50%)에서 멈춘다.
    const kept = resolveValuePlacement({ value_pos_x: 45, value_pos_y: -45 });
    expect(valuePosition({ offsetX: kept.percentX, offsetY: kept.percentY }).transform).toContain(
      'translate(45%, -45%)',
    );

    const clamped = resolveValuePlacement({ value_pos_x: 999, value_pos_y: -999 });
    expect(
      valuePosition({ offsetX: clamped.percentX, offsetY: clamped.percentY }).transform,
    ).toContain(`translate(${STAT_OFFSET_LIMIT}%, -${STAT_OFFSET_LIMIT}%)`);
  });

  it('니들 유형의 배지는 값과 함께 옛 변위를 받는다', () => {
    const badgeX = (viewBoxOffsetX: number): number => {
      const view = render(
        <GaugeValueOverlay
          parsed={parseConfig({ value: 50, unit: '%', gaugeType: 'needle' })}
          hasValue
          offsetX={0}
          offsetY={0}
          viewBoxOffsetX={viewBoxOffsetX}
        />,
      );
      const x = Number(view.container.querySelector('rect')!.getAttribute('x'));
      view.unmount();
      return x;
    };
    expect(badgeX(15)).toBe(badgeX(0) + 15);
  });
});
