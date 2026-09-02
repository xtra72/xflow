// 임계값 영역 표시 — 게이지 **7종 전부**가 토글을 따르는가.
//
// 종전에는 반응이 세 갈래로 갈려 있었다:
//   - 기본 도넛 · 도넛 반원 · 바늘  → 토글대로 내부 부채꼴을 그렸다
//   - 멀티링                        → 임계값을 아예 그리지 않았다
//   - 니들 RB · 세로 바 · 반원 RB   → 임계값을 늘 그리고 토글을 무시했다
//
// 설정 화면의 체크박스는 게이지 타입과 무관하게 늘 떠 있으므로, 네 타입에서는
// 눌러도 아무 일이 없었다. "있는데 아무 효과가 없는 컨트롤" 은 없는 것보다 나쁘다.
//
// 그리는 **방식**은 타입마다 다르다(안쪽 부채꼴 · 배경 띠 · 세그먼트 아크) — 축의
// 생김새가 다르기 때문이다. 스위치는 하나이고 표현만 갈린다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import { parseConfig, renderGaugeByType } from './gaugeShapes';

const THRESHOLDS = [
  { name: '정상', color: '#10b981', from: 0, to: 60 },
  { name: '주의', color: '#f59e0b', from: 60, to: 80 },
  { name: '위험', color: '#ef4444', from: 80, to: 100 },
];

const GAUGE_TYPES = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'vertical-bar',
  'half-rainbow',
] as const;

/** 게이지를 그려 SVG 마크업을 돌려준다. */
function draw(gaugeType: string, showThresholdZones: boolean): string {
  const view = render(
    renderGaugeByType(
      parseConfig({ value: 50, gaugeType, thresholds: THRESHOLDS, showThresholdZones }),
      true,
    ),
  );
  const html = view.container.innerHTML;
  view.unmount();
  return html;
}

/** 마크업에 나타난 임계값 색의 종류 수. */
function zoneColorCount(html: string): number {
  return THRESHOLDS.filter((t) => html.includes(t.color)).length;
}

describe('임계값 영역 표시 — 전 게이지 타입', () => {
  for (const gaugeType of GAUGE_TYPES) {
    it(`${gaugeType}: 켜면 임계값 색이 나타난다`, () => {
      // 진행 방식(도넛 · 반원 · 세로 바)은 **값까지만** 칠하므로 값이 든 구간의 색만
      // 나온다. 구간 방식(바늘 · 니들 RB · 반원 RB)은 세 색이 모두 나온다.
      expect(zoneColorCount(draw(gaugeType, true))).toBeGreaterThan(0);
    });

    it(`${gaugeType}: 끄면 그림이 달라진다 — 토글이 살아 있다`, () => {
      expect(draw(gaugeType, false)).not.toBe(draw(gaugeType, true));
    });

    it(`${gaugeType}: 끄면 임계값 색이 줄어든다`, () => {
      // 값 색은 임계값에서 오므로 한 색은 남을 수 있다 — 남는 색이 영역 표시가 아니라
      // 값 색이라는 뜻이다.
      expect(zoneColorCount(draw(gaugeType, false))).toBeLessThan(THRESHOLDS.length);
    });
  }

  it('임계값이 없으면 켜도 그릴 것이 없다', () => {
    for (const gaugeType of GAUGE_TYPES) {
      const html = render(
        renderGaugeByType(
          parseConfig({ value: 50, gaugeType, thresholds: [], showThresholdZones: true }),
          true,
        ),
      );
      expect(zoneColorCount(html.container.innerHTML)).toBe(0);
      html.unmount();
    }
  });

  it('기본값은 임계값이 있으면 켜짐이다 — 기존 패널의 그림이 달라지지 않는다', () => {
    // showThresholdZones 를 지정하지 않은 저장 config.
    const parsed = parseConfig({ value: 50, thresholds: THRESHOLDS });
    expect(parsed.showThresholdZones).toBe(true);
  });

  it('임계값이 없으면 기본값은 꺼짐이다', () => {
    expect(parseConfig({ value: 50, thresholds: [] }).showThresholdZones).toBe(false);
  });

  it('명시적으로 끄면 임계값이 있어도 꺼짐이다', () => {
    const parsed = parseConfig({ value: 50, thresholds: THRESHOLDS, showThresholdZones: false });
    expect(parsed.showThresholdZones).toBe(false);
  });
});
