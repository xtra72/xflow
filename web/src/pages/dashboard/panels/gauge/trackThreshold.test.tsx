// 임계 색을 게이지의 **원·반원 자체**에 칠한다.
//
// 종전에는 트랙이 회색(도넛·반원)이거나 값 색 한 가지(바늘)였고, 임계 색은 안쪽 얇은
// 띠에만 있었다. 게이지의 본체가 무채색이라 값이 어느 구간에 있는지 읽으려면 눈을
// 안쪽 띠로 옮겨야 했다.
//
// 값까지는 진하게, 값을 넘은 구간은 연하게 그린다 — 같은 고리 위에서 "여기까지 왔다"
// 와 "이 다음은 무슨 구간이다" 가 함께 읽힌다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import {
  DEFAULT_TRACK_FILL,
  parseConfig,
  renderGaugeByType,
  trackThresholdSegments,
} from './gaugeShapes';

/**
 * **진행(progress) 방식** 유형 — 0~값만 채우고 구간 경계는 테두리로 알린다.
 *
 * 세로 바는 축이 각도가 아니라 선형이지만 **같은 계산**(`trackThresholdSegments`)을
 * 쓰므로 같은 표에서 검증한다 — 유형별로 다시 짜면 잘리는 자리가 조용히 어긋난다.
 */
const TRACK_TYPES = ['simple', 'half', 'vertical-bar'] as const;

/**
 * **구간(zone) 방식** 유형 — 구간 전체를 그 색으로 채운다.
 *
 * 니들이 값을 가리키므로 채움을 값 표시에 쓸 필요가 없고, 눈금 전체가 색으로 읽힌다.
 */
const ZONE_TYPES = ['needle', 'needle-rainbow', 'half-rainbow'] as const;

/** 세로 바는 `<rect>`, 나머지는 `<path>` 로 트랙 조각을 그린다. */
function trackShape(gaugeType: string): string {
  return gaugeType === 'vertical-bar' ? 'rect' : 'path';
}

const THRESHOLDS = [
  { name: '정상', color: '#10b981', from: 0, to: 60 },
  { name: '주의', color: '#f59e0b', from: 60, to: 80 },
  { name: '위험', color: '#ef4444', from: 80, to: 100 },
];

/** 임계 색으로 칠해진 트랙 조각의 `색@불투명도` 목록. */
function coloredTrack(config: Record<string, unknown>): string[] {
  const view = render(renderGaugeByType(parseConfig(config), true));
  const out = Array.from(
    view.container.querySelectorAll(trackShape(String(config.gaugeType ?? 'simple'))),
  )
    .filter((el) => THRESHOLDS.some((t) => el.getAttribute('fill') === t.color))
    .map((el) => `${el.getAttribute('fill')}@${el.getAttribute('opacity')}`);
  view.unmount();
  return out;
}

/** 임계 색으로 그려진 **테두리**의 색 목록. */
function outlines(config: Record<string, unknown>): string[] {
  const view = render(renderGaugeByType(parseConfig(config), true));
  const out = Array.from(
    view.container.querySelectorAll(trackShape(String(config.gaugeType ?? 'simple'))),
  )
    .filter((el) => el.getAttribute('fill') === 'none' && el.getAttribute('stroke'))
    .map((el) => el.getAttribute('stroke') ?? '');
  view.unmount();
  return out.filter((c) => THRESHOLDS.some((t) => t.color === c));
}

describe('조각 나누기', () => {
  it('값 지점에서 한 번 자른다 — 값이 든 구간이 진한/연한 둘로 나뉜다', () => {
    expect(trackThresholdSegments(THRESHOLDS, 0, 100, 0.5)).toEqual([
      { startRatio: 0, endRatio: 0.5, color: '#10b981', filled: true },
      { startRatio: 0.5, endRatio: 0.6, color: '#10b981', filled: false },
      { startRatio: 0.6, endRatio: 0.8, color: '#f59e0b', filled: false },
      { startRatio: 0.8, endRatio: 1, color: '#ef4444', filled: false },
    ]);
  });

  it('값이 최소면 전부 연하다', () => {
    const segs = trackThresholdSegments(THRESHOLDS, 0, 100, 0);
    expect(segs).toHaveLength(3);
    expect(segs.every((s) => !s.filled)).toBe(true);
  });

  it('값이 최대면 전부 진하다', () => {
    const segs = trackThresholdSegments(THRESHOLDS, 0, 100, 1);
    expect(segs).toHaveLength(3);
    expect(segs.every((s) => s.filled)).toBe(true);
  });

  it('구간 경계에 정확히 놓이면 쪼개지지 않는다', () => {
    const segs = trackThresholdSegments(THRESHOLDS, 0, 100, 0.6);
    expect(segs).toHaveLength(3);
    expect(segs.map((s) => s.filled)).toEqual([true, false, false]);
  });

  it('뒤집힌 구간·폭 0 구간은 조각을 만들지 않는다', () => {
    expect(trackThresholdSegments([{ name: '', color: '#000', from: 80, to: 20 }], 0, 100, 0.5))
      .toHaveLength(0);
    expect(trackThresholdSegments([{ name: '', color: '#000', from: 50, to: 50 }], 0, 100, 0.5))
      .toHaveLength(0);
  });

  it('값 범위를 따른다 — 0~1000 에서도 같은 자리에서 잘린다', () => {
    const th = [{ name: '', color: '#000', from: 0, to: 1000 }];
    const segs = trackThresholdSegments(th, 0, 1000, 0.25);
    expect(segs.map((s) => [s.startRatio, s.endRatio, s.filled])).toEqual([
      [0, 0.25, true],
      [0.25, 1, false],
    ]);
  });
});

describe('원·반원에 칠해진다', () => {
  for (const gaugeType of TRACK_TYPES) {
    it(`${gaugeType}: 임계 색이 트랙 조각으로 나온다`, () => {
      const drawn = coloredTrack({ value: 50, gaugeType, thresholds: THRESHOLDS });
      // 값 50 → 채움은 첫 구간의 0~50 하나뿐이다. 나머지 구간은 채우지 않고
      // 테두리로만 알린다(기본색이 그 안을 채운다).
      expect(drawn).toEqual(['#10b981@1']);
    });

    it(`${gaugeType}: 값이 오르면 진한 조각이 늘어난다`, () => {
      const at = (value: number): number =>
        coloredTrack({ value, gaugeType, thresholds: THRESHOLDS }).filter((d) => d.endsWith('@1'))
          .length;
      expect(at(90)).toBeGreaterThan(at(10));
    });

    it(`${gaugeType}: 영역 표시를 끄면 구간 테두리가 사라진다`, () => {
      // 종전 표시(값 아크 하나)로 돌아간다. 그 아크는 여전히 임계 색을 쓸 수 있으므로
      // "채움이 없다" 가 아니라 "구간 테두리가 없다" 로 판정한다.
      expect(
        outlines({ value: 50, gaugeType, thresholds: THRESHOLDS, showThresholdZones: false }),
      ).toHaveLength(0);
    });

    it(`${gaugeType}: 구간 테두리는 그리지 않는다`, () => {
      // 채움만으로 "여기까지 왔고 지금 어느 구간이다" 를 말한다. 테두리를 더하면
      // 값이 닿지 않은 구간까지 선으로 남아 트랙이 복잡해진다.
      expect(outlines({ value: 50, gaugeType, thresholds: THRESHOLDS })).toHaveLength(0);
    });

    it(`${gaugeType}: 임계값이 없으면 종전 표시로 돌아간다`, () => {
      // 조각이 없으면 도넛·반원은 값 아크 하나, 바늘은 단색 외곽 원이다.
      const view = render(
        renderGaugeByType(parseConfig({ value: 50, gaugeType, thresholds: [] }), true),
      );
      const html = view.container.innerHTML;
      view.unmount();
      expect(html).toContain('#5B8FB9');
    });
  }
});

describe('세로 바 — 선형 축에서도 같은 규약', () => {
  /** 세로 바의 임계 조각을 `색@불투명도@y..y+h` 로 뽑는다. */
  function bars(config: Record<string, unknown>): string[] {
    const view = render(
      renderGaugeByType(parseConfig({ gaugeType: 'vertical-bar', ...config }), true),
    );
    const out = Array.from(view.container.querySelectorAll('rect'))
      .filter((r) => THRESHOLDS.some((t) => r.getAttribute('fill') === t.color))
      // 좌표는 소수 오차가 붙으므로(82.00000000000001) 반올림해 비교한다.
      .map((r) => {
        const top = Math.round(Number(r.getAttribute('y')));
        const bottom = Math.round(Number(r.getAttribute('y')) + Number(r.getAttribute('height')));
        return `${r.getAttribute('fill')}@${r.getAttribute('opacity')}@${top}-${bottom}`;
      });
    view.unmount();
    return out;
  }

  it('값 바가 임계 띠를 덮지 않는다', () => {
    // 종전 결함: 흐린 띠(op 0.3) 위에 단색 값 바가 0~값 구간을 통째로 덮어,
    // 임계 색이 값 **위쪽**에만 흐릿하게 남았다.
    // 바(y 10~190, 아래가 min)에서 값 50 → 진한 조각은 y 100~190 이어야 한다.
    expect(bars({ value: 50, thresholds: THRESHOLDS })).toEqual(['#10b981@1@100-190']);
  });

  it('아래에서 위로 쌓인다 — 최솟값이 바닥이다', () => {
    const low = bars({ value: 10, thresholds: THRESHOLDS });
    const high = bars({ value: 90, thresholds: THRESHOLDS });
    // 값이 오르면 진한 조각의 윗변이 위로 올라간다(y 가 작아진다).
    const topOf = (list: string[]): number =>
      Math.min(...list.filter((d) => d.includes('@1@')).map((d) => Number(d.split('@')[2]!.split('-')[0])));
    expect(topOf(high)).toBeLessThan(topOf(low));
  });

  it('임계값이 하나여도 그린다', () => {
    // 종전에는 `thresholds.length >= 2` 라 1개면 띠가 통째로 사라졌다.
    const one = [{ name: '단일', color: '#10b981', from: 0, to: 100 }];
    expect(bars({ value: 50, thresholds: one })).toEqual(['#10b981@1@100-190']);
  });
});

describe('기본색 — 테두리 안쪽 채움', () => {
  const ALL_TYPES = [
    'simple',
    'half',
    'needle',
    'needle-rainbow',
    'vertical-bar',
    'half-rainbow',
  ] as const;

  /** 트랙 도형 중 `fill` 로 칠해진 색 목록(테두리는 제외). */
  function fills(config: Record<string, unknown>): string[] {
    const view = render(renderGaugeByType(parseConfig(config), true));
    const out = Array.from(view.container.querySelectorAll('path, rect'))
      .map((el) => el.getAttribute('fill'))
      .filter((f): f is string => f !== null && f !== 'none');
    view.unmount();
    return out;
  }

  it('미지정이면 종전 트랙 회색으로 채운다', () => {
    expect(parseConfig({ value: 50 }).baseColor).toBe(DEFAULT_TRACK_FILL);
    expect(fills({ value: 50, thresholds: THRESHOLDS })).toContain(DEFAULT_TRACK_FILL);
  });

  it('지정한 색으로 채운다', () => {
    expect(fills({ value: 50, thresholds: THRESHOLDS, base_color: '#111827' })).toContain(
      '#111827',
    );
  });

  it('빈 문자열은 "채우지 않음" 이다 — 값까지의 채움만 남는다', () => {
    // 미지정과 빈 문자열은 다른 뜻이다. 끈 상태에서는 트랙 바탕이 비어야 한다.
    const drawn = fills({ value: 50, thresholds: THRESHOLDS, base_color: '' });
    expect(drawn).not.toContain(DEFAULT_TRACK_FILL);
    // 값까지의 채움은 그대로 남는다.
    expect(drawn).toContain('#10b981');
  });

  it('전 유형이 같은 기본색을 쓴다', () => {
    for (const gaugeType of ALL_TYPES) {
      expect(
        fills({ value: 50, gaugeType, thresholds: THRESHOLDS, base_color: '#111827' }),
        `${gaugeType} 가 기본색을 쓰지 않는다`,
      ).toContain('#111827');
    }
  });

});

describe('구간 방식 — 바늘 · 니들 RB · 반원 RB', () => {
  /** 임계 색으로 **채워진** 도형의 색 목록. */
  function zoneFills(config: Record<string, unknown>): string[] {
    const view = render(renderGaugeByType(parseConfig(config), true));
    const out = Array.from(view.container.querySelectorAll('path'))
      .map((el) => el.getAttribute('fill'))
      .filter((f): f is string => THRESHOLDS.some((t) => t.color === f));
    view.unmount();
    return out;
  }

  for (const gaugeType of ZONE_TYPES) {
    it(`${gaugeType}: 구간 전체가 그 색으로 채워진다`, () => {
      // 값(50)이 첫 구간에 있어도 세 구간이 모두 칠해진다 — 값은 니들이 가리킨다.
      expect(zoneFills({ value: 50, gaugeType, thresholds: THRESHOLDS })).toEqual([
        '#10b981',
        '#f59e0b',
        '#ef4444',
      ]);
    });

    it(`${gaugeType}: 값이 바뀌어도 채움은 그대로다`, () => {
      const low = zoneFills({ value: 0, gaugeType, thresholds: THRESHOLDS });
      const high = zoneFills({ value: 100, gaugeType, thresholds: THRESHOLDS });
      expect(low).toEqual(high);
    });

    it(`${gaugeType}: 구간 테두리는 그리지 않는다 — 면이 이미 구간을 말한다`, () => {
      const view = render(
        renderGaugeByType(parseConfig({ value: 50, gaugeType, thresholds: THRESHOLDS }), true),
      );
      const strokes = Array.from(view.container.querySelectorAll('path'))
        .filter((el) => el.getAttribute('fill') === 'none')
        .map((el) => el.getAttribute('stroke'));
      view.unmount();
      for (const t of THRESHOLDS) {
        expect(strokes).not.toContain(t.color);
      }
    });
  }
});
