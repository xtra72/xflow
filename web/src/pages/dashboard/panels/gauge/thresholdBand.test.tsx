// 임계값 영역의 **형태** — 파이가 아니라 트랙을 따르는 도넛 띠다.
//
// 종전에는 중심에서 퍼지는 부채꼴(파이)이었다. 파이는 게이지 한가운데를 덮어 값 숫자와
// 눈금 라벨 위로 색이 깔렸고, 아크로 값을 읽는 게이지 안에 축이 다른 도형이 하나 더
// 생기는 꼴이었다. 띠로 바꾸면 임계값 구간이 값 아크와 같은 길 위에 놓여, 니들 RB ·
// 반원 RB 의 세그먼트 아크와도 같은 언어가 된다.

import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import { parseConfig, renderGaugeByType } from './gaugeShapes';

/** 띠를 그리는 네 유형. 나머지 셋은 원래 아크/배경 띠라 해당이 없다. */
const BAND_TYPES = ['simple', 'half', 'needle'] as const;

const THRESHOLDS = [
  { name: '정상', color: '#10b981', from: 0, to: 60 },
  { name: '주의', color: '#f59e0b', from: 60, to: 80 },
  { name: '위험', color: '#ef4444', from: 80, to: 100 },
];

/** 임계값 색으로 칠해진 path 들의 `d` 목록. */
function zonePaths(gaugeType: string, thresholds = THRESHOLDS): string[] {
  const view = render(
    renderGaugeByType(
      parseConfig({ value: 50, gaugeType, thresholds, showThresholdZones: true }),
      true,
    ),
  );
  const out = Array.from(view.container.querySelectorAll('path'))
    .filter((p) => thresholds.some((t) => p.getAttribute('fill') === t.color))
    .map((p) => p.getAttribute('d') ?? '');
  view.unmount();
  return out;
}

/**
 * 파이(부채꼴)인가.
 *
 * 부채꼴은 중심점에서 시작해 외곽으로 직선을 긋는다(`M cx,cy L x,y A …`).
 * 띠는 호 위의 점에서 시작한다.
 */
function isPie(d: string): boolean {
  return /^M\s[\d.]+,[\d.]+\sL\s/.test(d);
}

describe('임계값 영역은 파이가 아니다', () => {
  for (const gaugeType of BAND_TYPES) {
    it(`${gaugeType}: 부채꼴 path 가 하나도 없다`, () => {
      const paths = zonePaths(gaugeType);
      expect(paths.length).toBeGreaterThan(0);
      expect(paths.filter(isPie)).toHaveLength(0);
    });

    it(`${gaugeType}: 값까지 채운 조각이 나온다`, () => {
      // 채움은 값까지만이다(그 뒤는 기본색 + 테두리). 값 50 이면 첫 구간 하나.
      expect(zonePaths(gaugeType).length).toBeGreaterThan(0);
    });
  }
});

describe('전 구간을 덮는 단일 임계값', () => {
  const FULL = [{ name: '전체', color: '#123456', from: 0, to: 100 }];

  for (const gaugeType of BAND_TYPES) {
    it(`${gaugeType}: 360° 구간도 고리로 그린다`, () => {
      // 시작점과 끝점이 겹쳐 호가 성립하지 않는 경우다 — 바깥/안쪽 원을 반대
      // 방향으로 그려 가운데가 뚫린 고리를 만든다. 빠뜨리면 이 구간만 사라진다.
      expect(zonePaths(gaugeType, FULL).length).toBeGreaterThan(0);
    });
  }
});

describe('그리지 않는 구간', () => {
  it('뒤집힌 구간(to < from)은 그리지 않는다', () => {
    const REVERSED = [{ name: '뒤집힘', color: '#abcdef', from: 80, to: 20 }];
    for (const gaugeType of BAND_TYPES) {
      expect(zonePaths(gaugeType, REVERSED)).toHaveLength(0);
    }
  });

  it('폭이 없는 구간(from === to)은 그리지 않는다', () => {
    // 구간을 값(50)에서 떨어뜨린다 — 값 아크도 임계값 색을 쓰므로, 값이 구간 안에
    // 들어가면 그 아크가 영역으로 잘못 세어진다.
    const EMPTY = [{ name: '점', color: '#abcdef', from: 90, to: 90 }];
    for (const gaugeType of BAND_TYPES) {
      expect(zonePaths(gaugeType, EMPTY)).toHaveLength(0);
    }
  });
});
