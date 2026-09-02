// 게이지 유형과 임계값의 일치.
//
// 세 갈래로 어긋나 있었다:
//   ① 설정 편집기는 기본 3구간을 보여주는데 게이지는 0개로 읽었다 — 기본값이 편집기
//      안에만 하드코딩되어 있었고, 게이지는 `config.thresholds ?? []` 였다.
//      체크박스는 켜져 보이는데 화면에는 아무것도 안 나오는 상태.
//   ② 기본 구간이 `0/60/80/100` 고정이라 값 범위를 바꾸면 따라오지 않았다.
//      범위 0~1000 이면 세 구간이 아크 앞 10% 에 몰리고 값 600 은 폴백 색이 됐다.
//   ③ 니들 RB · 반원 RB 만 `thresholds.length >= 2` 라, 임계값이 1개면 기본 팔레트로
//      돌아갔다 — 같은 설정이 게이지 유형에 따라 그려지기도 하고 안 그려지기도 했다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import {
  defaultGaugeThresholds,
  getThresholdColor,
  parseConfig,
  renderGaugeByType,
} from './gaugeShapes';

const GAUGE_TYPES = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'vertical-bar',
  'half-rainbow',
] as const;

/** 게이지를 그려 마크업에 나타난 색을 센다. */
function drawnColors(config: Record<string, unknown>, colors: string[]): number {
  const view = render(renderGaugeByType(parseConfig(config), true));
  const html = view.container.innerHTML;
  view.unmount();
  return colors.filter((c) => html.includes(c)).length;
}

describe('① 기본 임계값이 게이지에 도달한다', () => {
  it('임계값을 지정하지 않아도 게이지가 3구간을 읽는다', () => {
    const parsed = parseConfig({ value: 50 });
    expect(parsed.thresholds).toHaveLength(3);
  });

  it('영역 표시도 함께 켜진다 — 체크박스와 화면이 어긋나지 않는다', () => {
    // 종전: 편집기 체크박스는 켜져 보이는데 게이지는 thresholds 0개라 꺼져 있었다.
    expect(parseConfig({ value: 50 }).showThresholdZones).toBe(true);
  });

  it('빈 배열을 명시하면 그것이 이긴다 — 기본값이 사용자 설정을 덮지 않는다', () => {
    const parsed = parseConfig({ value: 50, thresholds: [] });
    expect(parsed.thresholds).toHaveLength(0);
    expect(parsed.showThresholdZones).toBe(false);
  });
});

describe('② 기본 임계값이 값 범위를 따른다', () => {
  it('0~100 은 종전 경계와 같다', () => {
    expect(defaultGaugeThresholds(0, 100).map((t) => [t.from, t.to])).toEqual([
      [0, 60],
      [60, 80],
      [80, 100],
    ]);
  });

  it('범위를 키우면 경계도 비례해 따라간다', () => {
    expect(defaultGaugeThresholds(0, 1000).map((t) => [t.from, t.to])).toEqual([
      [0, 600],
      [600, 800],
      [800, 1000],
    ]);
  });

  it('음수 시작 범위도 덮는다', () => {
    const th = defaultGaugeThresholds(-50, 50);
    expect(th[0]!.from).toBe(-50);
    expect(th[2]!.to).toBe(50);
  });

  it('범위 전체가 어느 한 구간에는 걸린다 — 폴백 색으로 떨어지지 않는다', () => {
    // 종전: 범위 0~1000 에 임계값 0~100 이라 값 600 이 어디에도 안 걸렸다.
    const th = defaultGaugeThresholds(0, 1000);
    for (const v of [0, 1, 600, 800, 999, 1000]) {
      expect(getThresholdColor(v, th, 'FALLBACK')).not.toBe('FALLBACK');
    }
  });

  it('설정 화면과 게이지가 같은 경계를 쓴다', () => {
    // 편집기는 이름만 덮어쓰므로 구간·색은 정본과 같아야 한다.
    const fromGauge = parseConfig({ value: 5, min: 0, max: 1000 }).thresholds;
    const canonical = defaultGaugeThresholds(0, 1000);
    expect(fromGauge.map((t) => [t.from, t.to, t.color])).toEqual(
      canonical.map((t) => [t.from, t.to, t.color]),
    );
  });
});

describe('③ 임계값 1개도 전 유형이 그린다', () => {
  const ONE = [{ name: '단일', color: '#123456', from: 0, to: 50 }];

  for (const gaugeType of GAUGE_TYPES) {
    it(`${gaugeType}: 임계값이 하나여도 그 색이 나타난다`, () => {
      expect(
        drawnColors({ value: 25, gaugeType, thresholds: ONE, showThresholdZones: true }, [
          '#123456',
        ]),
      ).toBe(1);
    });
  }

  it('세그먼트형은 덮이지 않은 구간에 중립 트랙을 깐다', () => {
    // 트랙이 없으면 임계값 1개일 때 아크 나머지가 통째로 빈다 — 그것을 피하려던
    // `>= 2` 가림막이 유형별 불일치의 원인이었다.
    for (const gaugeType of ['needle-rainbow', 'half-rainbow'] as const) {
      const view = render(
        renderGaugeByType(
          parseConfig({ value: 25, gaugeType, thresholds: ONE, showThresholdZones: true }),
          true,
        ),
      );
      expect(view.container.innerHTML).toContain('#E2E8F0');
      view.unmount();
    }
  });

  it('영역 표시를 끄면 트랙도 함께 사라진다 — 기본 팔레트로 돌아간다', () => {
    for (const gaugeType of ['needle-rainbow', 'half-rainbow'] as const) {
      const view = render(
        renderGaugeByType(
          parseConfig({ value: 25, gaugeType, thresholds: ONE, showThresholdZones: false }),
          true,
        ),
      );
      const html = view.container.innerHTML;
      view.unmount();
      expect(html).not.toContain('#123456');
    }
  });
});

describe('멀티링 제거 후 하위 호환', () => {
  it('저장된 multi-ring 패널은 기본 도넛으로 그려진다', () => {
    // 유형 목록에서 빠졌으므로 설정 화면에서는 더 고를 수 없다. 이미 저장된 config 는
    // `renderGaugeByType` 의 default 분기가 받아 도넛으로 떨어진다 — 빈 화면이나
    // 예외가 아니라 읽을 수 있는 게이지가 나와야 한다.
    const retired = render(
      renderGaugeByType(parseConfig({ value: 50, gaugeType: 'multi-ring' }), true),
    );
    const retiredHtml = retired.container.innerHTML;
    retired.unmount();

    const donut = render(renderGaugeByType(parseConfig({ value: 50, gaugeType: 'simple' }), true));
    const donutHtml = donut.container.innerHTML;
    donut.unmount();

    expect(retiredHtml).toBe(donutHtml);
  });

  it('멀티링 전용이던 values 축은 더 이상 읽히지 않는다', () => {
    // 링 2·3 이 `values[0] ?? 74` · `values[1] ?? 82` 로 가짜 숫자를 그리던 자리다.
    // 쓰는 코드가 없던 죽은 config 축이므로 남겨 두지 않는다.
    const parsed = parseConfig({ value: 50, values: [34, 56] }) as Record<string, unknown>;
    expect(parsed.values).toBeUndefined();
  });

  it('74 · 82 데모 숫자가 어디에도 남아 있지 않다', () => {
    for (const gaugeType of GAUGE_TYPES) {
      const view = render(
        renderGaugeByType(parseConfig({ value: 50, gaugeType, decimal_places: 0 }), true),
      );
      const texts = Array.from(view.container.querySelectorAll('text')).map((t) => t.textContent);
      view.unmount();
      expect(texts.join(' ')).not.toMatch(/\b(74|82)\b/);
    }
  });
});
