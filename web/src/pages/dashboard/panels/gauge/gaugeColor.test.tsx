// 게이지 값 색은 **유형이 아니라 설정**이 정한다.
//
// 종전에는 유형마다 색이 따로 박혀 있었다:
//   도넛·반원 `#5B8FB9` · 바늘 `#EF4444` · 세로 바 `#F97316` ·
//   멀티링은 링 3색(`#2C6E8A`/`#2ABFBF`/`#F0A04B`)을 갖고 임계값 색을 아예 안 썼다.
// 유형은 **모양**을 고르는 설정인데 색까지 따라 바뀌어, 같은 값·같은 설정에서 유형만
// 바꿔도 색이 청회색 ↔ 빨강으로 튀었다.
//
// 그리고 설정의 `연속` 모드가 고르는 컬러 테마는 config 에 저장만 되고 게이지 렌더
// 경로에 참조가 하나도 없어, 테마를 바꿔도 화면이 그대로였다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import {
  DEFAULT_GAUGE_COLOR,
  GAUGE_COLOR_THEMES,
  parseConfig,
  renderGaugeByType,
  resolveGaugeValueColor,
  sampleColorTheme,
} from './gaugeShapes';

const GAUGE_TYPES = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'vertical-bar',
  'half-rainbow',
] as const;

/**
 * 종전에 **값 색 폴백**으로 박혀 있던 색들.
 *
 * 니들 RB · 반원 RB 의 기본 세그먼트 팔레트(3구간 · 5단계 등급)는 여기 넣지 않는다 —
 * 그것은 값 색이 아니라 임계값이 없을 때 아크를 채우는 별개 축이고, 이번 변경 대상이
 * 아니다. 두 축을 섞으면 "색이 남아 있다" 가 실제와 다른 것을 가리킨다.
 */
const RETIRED_VALUE_COLORS: Record<string, string[]> = {
  needle: ['#EF4444'],
  'vertical-bar': ['#F97316'],
};

function draw(config: Record<string, unknown>): string {
  const view = render(renderGaugeByType(parseConfig(config), true));
  const html = view.container.innerHTML;
  view.unmount();
  return html;
}

describe('유형이 색을 바꾸지 않는다', () => {
  it('임계값이 없으면 전 유형이 같은 기본색을 쓴다', () => {
    for (const gaugeType of GAUGE_TYPES) {
      expect(parseConfig({ value: 50, gaugeType, thresholds: [] }).valueColor).toBe(
        DEFAULT_GAUGE_COLOR,
      );
    }
  });

  it('임계값이 있으면 전 유형이 같은 구간 색을 쓴다', () => {
    const thresholds = [{ name: '', color: '#123456', from: 0, to: 100 }];
    for (const gaugeType of GAUGE_TYPES) {
      expect(parseConfig({ value: 50, gaugeType, thresholds }).valueColor).toBe('#123456');
    }
  });

  it('은퇴한 유형별 값 색이 남아 있지 않다', () => {
    // 임계값을 비워 폴백 경로로 몰아넣는다 — 종전이라면 유형마다 다른 색이 나왔다.
    for (const [gaugeType, retiredColors] of Object.entries(RETIRED_VALUE_COLORS)) {
      const html = draw({ value: 50, gaugeType, thresholds: [] });
      for (const retired of retiredColors) {
        expect(html, `${gaugeType} 에 ${retired} 가 남아 있다`).not.toContain(retired);
      }
    }
  });
});

describe('연속 컬러 테마가 화면에 도달한다', () => {
  it('테마를 바꾸면 색이 바뀐다', () => {
    const themes = Object.keys(GAUGE_COLOR_THEMES);
    const colors = themes.map(
      (colorTheme) => parseConfig({ value: 50, colorMode: 'continuous', colorTheme }).valueColor,
    );
    expect(new Set(colors).size).toBe(themes.length);
  });

  it('값 위치에 따라 색이 이어서 바뀐다 — 구간을 끊지 않는다', () => {
    const at = (value: number): string =>
      parseConfig({ value, colorMode: 'continuous', colorTheme: 'green-red' }).valueColor;
    // 양 끝은 테마의 첫 색·끝 색과 정확히 같다.
    expect(at(0)).toBe('#10b981');
    expect(at(100)).toBe('#ef4444');
    // 중간값은 이웃한 두 지점과 모두 달라야 한다(끊어 쓰면 같은 값이 반복된다).
    expect(new Set([at(10), at(30), at(50), at(70), at(90)]).size).toBe(5);
  });

  it('범위를 벗어난 값은 양 끝으로 죈다', () => {
    const cfg = { colorMode: 'continuous', colorTheme: 'green-red', min: 0, max: 100 };
    expect(parseConfig({ ...cfg, value: -50 }).valueColor).toBe('#10b981');
    expect(parseConfig({ ...cfg, value: 500 }).valueColor).toBe('#ef4444');
  });

  it('모르는 테마는 기본 테마로 접는다 — 손으로 편집한 config 가 색을 깨뜨리지 않는다', () => {
    expect(sampleColorTheme('없는테마', 0)).toBe(sampleColorTheme('green-red', 0));
    expect(sampleColorTheme(undefined, 1)).toBe(sampleColorTheme('green-red', 1));
  });

  it('연속 모드는 임계값보다 우선한다 — 두 축이 섞이지 않는다', () => {
    const thresholds = [{ name: '', color: '#123456', from: 0, to: 100 }];
    expect(
      resolveGaugeValueColor({
        value: 0,
        min: 0,
        max: 100,
        thresholds,
        colorMode: 'continuous',
        colorTheme: 'green-red',
      }),
    ).toBe('#10b981');
  });

  it('전 유형이 연속 테마를 따른다', () => {
    for (const gaugeType of GAUGE_TYPES) {
      expect(
        parseConfig({ value: 0, gaugeType, colorMode: 'continuous', colorTheme: 'cyan-blue' })
          .valueColor,
      ).toBe('#06b6d4');
    }
  });
});

describe('기본 모드는 종전 그대로다', () => {
  it('colorMode 미지정은 개별(임계값 색)이다', () => {
    const parsed = parseConfig({ value: 50 });
    expect(parsed.colorMode).toBe('individual');
    // 기본 임계값 3구간 중 값 50 이 드는 첫 구간 색.
    expect(parsed.valueColor).toBe('#10b981');
  });
});
