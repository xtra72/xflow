// marchingSquares 순수 알고리즘 골든 케이스 단위 테스트 (SPEC-HEATMAP-PANEL-003 T1/T9).
//
// DOM 없이 알려진 소형 격자 → 알려진 세그먼트를 검증한다: 셀 case, saddle(5/10) 결정성,
// resolveLevels 우선순위/균등분할/범위밖 필터, 빈/퇴화 격자 방어. 좌표는 셀-중심 정규화
// 규약(((gx+0.5)/gridW, (gy+0.5)/gridH))이므로 toBeCloseTo 근사 비교한다.

import { describe, it, expect } from 'vitest';

import {
  resolveLevels,
  computeContours,
  segmentsToPath,
  type Segment,
} from './marchingSquares';

/** 세그먼트를 순서 무관하게 비교하기 위한 정규화 키(양끝 정렬 + 반올림). */
function segKey(s: Segment): string {
  const round = (v: number) => Math.round(v * 1e6) / 1e6;
  const a = [round(s.x1), round(s.y1)];
  const b = [round(s.x2), round(s.y2)];
  const [p, q] = a[0]! < b[0]! || (a[0] === b[0] && a[1]! <= b[1]!) ? [a, b] : [b, a];
  return `${p[0]},${p[1]}|${q[0]},${q[1]}`;
}

function expectSegments(actual: Segment[], expected: Segment[]) {
  expect(actual.map(segKey).sort()).toEqual(expected.map(segKey).sort());
}

describe('resolveLevels', () => {
  it('explicit 값 목록이 있으면 count 를 무시하고 그대로 사용한다(AC-03)', () => {
    expect(resolveLevels({ min: 18, max: 26 }, 5, [20, 24])).toEqual([20, 24]);
  });

  it('explicit 없으면 (min,max) 를 count 등분한 내부 경계값을 산출한다(AC-03)', () => {
    // count=5, min=18, max=26 → 18 + 8*i/6, i=1..5.
    const levels = resolveLevels({ min: 18, max: 26 }, 5);
    expect(levels).toHaveLength(5);
    expect(levels[0]).toBeCloseTo(18 + (8 * 1) / 6, 6);
    expect(levels[2]).toBeCloseTo(22, 6); // 가운데 = (min+max)/2
    expect(levels[4]).toBeCloseTo(18 + (8 * 5) / 6, 6);
    // 끝점(min/max)은 포함하지 않는다(내부값만).
    for (const l of levels) {
      expect(l).toBeGreaterThan(18);
      expect(l).toBeLessThan(26);
    }
  });

  it('범위 밖 explicit 등치값은 필터한다(AC-E3)', () => {
    // 범위 18..26 에서 30 은 제외, 경계값(18/26)도 제외(strict 내부).
    expect(resolveLevels({ min: 18, max: 26 }, 5, [20, 30, 24, 18, 26])).toEqual([20, 24]);
  });

  it('퇴화 범위(max<=min)·비유한·count<=0 은 빈 배열', () => {
    expect(resolveLevels({ min: 10, max: 10 }, 5)).toEqual([]);
    expect(resolveLevels({ min: 20, max: 10 }, 5)).toEqual([]);
    expect(resolveLevels({ min: NaN, max: 10 }, 5)).toEqual([]);
    expect(resolveLevels({ min: 0, max: 10 }, 0)).toEqual([]);
    expect(resolveLevels({ min: 0, max: 10 })).toEqual([]); // count 미지정
  });

  it('explicit 이 전부 범위 밖이면 빈 배열', () => {
    expect(resolveLevels({ min: 0, max: 10 }, 5, [-1, 20, 100])).toEqual([]);
  });
});

describe('computeContours — 단일 셀 골든 케이스(2x2)', () => {
  // 2x2 격자 → 셀 1개. 정규화 좌표: xL=0.25, xR=0.75, yT=0.25, yB=0.75.
  it('case 2(BR 만 안): bottom-right 코너 격리 세그먼트', () => {
    // TL=0, TR=0, BR=10, BL=0. level=5.
    const grid = new Float32Array([0, 0, 0, 10]); // [TL,TR,BL,BR] 행 우선
    const segs = computeContours(grid, 2, 2, 5);
    // bottomPt(BL0-BR10, t=0.5) = (0.5,0.75), rightPt(TR0-BR10, t=0.5) = (0.75,0.5).
    expectSegments(segs, [{ x1: 0.5, y1: 0.75, x2: 0.75, y2: 0.5 }]);
  });

  it('case 6(TR+BR 안 = 우측 열): 상단↔하단 관통 세그먼트', () => {
    const grid = new Float32Array([0, 10, 0, 10]); // TL0,TR10,BL0,BR10
    const segs = computeContours(grid, 2, 2, 5);
    // topPt(TL0-TR10)=(0.5,0.25), bottomPt(BL0-BR10)=(0.5,0.75).
    expectSegments(segs, [{ x1: 0.5, y1: 0.25, x2: 0.5, y2: 0.75 }]);
  });

  it('case 3(BL+BR 안 = 하단 행): 좌↔우 관통 세그먼트', () => {
    const grid = new Float32Array([0, 0, 10, 10]); // TL0,TR0,BL10,BR10
    const segs = computeContours(grid, 2, 2, 5);
    // leftPt(TL0-BL10)=(0.25,0.5), rightPt(TR0-BR10)=(0.75,0.5).
    expectSegments(segs, [{ x1: 0.25, y1: 0.5, x2: 0.75, y2: 0.5 }]);
  });

  it('전부 안/전부 밖이면 세그먼트 없음', () => {
    expect(computeContours(new Float32Array([9, 9, 9, 9]), 2, 2, 5)).toEqual([]);
    expect(computeContours(new Float32Array([0, 0, 0, 0]), 2, 2, 5)).toEqual([]);
  });

  // 나머지 단일 코너/행·열 case 전수(2x2, level=5). grid 는 행 우선 [TL,TR,BL,BR].
  it.each<[string, number[], Segment]>([
    ['case 4 (TR)', [0, 10, 0, 0], { x1: 0.5, y1: 0.25, x2: 0.75, y2: 0.5 }],
    ['case 8 (TL)', [10, 0, 0, 0], { x1: 0.5, y1: 0.25, x2: 0.25, y2: 0.5 }],
    ['case 7 (~TL)', [0, 10, 10, 10], { x1: 0.5, y1: 0.25, x2: 0.25, y2: 0.5 }],
    ['case 11 (~TR)', [10, 0, 10, 10], { x1: 0.5, y1: 0.25, x2: 0.75, y2: 0.5 }],
    ['case 12 (상단 행 TL+TR)', [10, 10, 0, 0], { x1: 0.25, y1: 0.5, x2: 0.75, y2: 0.5 }],
    ['case 9 (좌측 열 TL+BL)', [10, 0, 10, 0], { x1: 0.5, y1: 0.25, x2: 0.5, y2: 0.75 }],
    ['case 13 (~BR)', [10, 10, 10, 0], { x1: 0.5, y1: 0.75, x2: 0.75, y2: 0.5 }],
    ['case 14 (~BL)', [10, 10, 0, 10], { x1: 0.25, y1: 0.5, x2: 0.5, y2: 0.75 }],
  ])('%s 는 단일 세그먼트를 낸다', (_label, grid, expected) => {
    const segs = computeContours(new Float32Array(grid), 2, 2, 5);
    expectSegments(segs, [expected]);
  });
});

describe('computeContours — saddle(5/10) 결정성(AC-E1)', () => {
  it('case 10 중앙값>=level: high(TL,BR) 연결 → 바깥 코너(TR,BL) 격리 2세그먼트', () => {
    // TL=10,TR=0,BR=10,BL=0. center=5 >= level 5.
    const grid = new Float32Array([10, 0, 0, 10]); // TL10,TR0,BL0,BR10
    const segs = computeContours(grid, 2, 2, 5);
    expect(segs).toHaveLength(2);
    // topPt=(0.5,0.25), rightPt=(0.75,0.5), leftPt=(0.25,0.5), bottomPt=(0.5,0.75).
    // TR 격리: topPt-rightPt / BL 격리: leftPt-bottomPt.
    expectSegments(segs, [
      { x1: 0.5, y1: 0.25, x2: 0.75, y2: 0.5 },
      { x1: 0.25, y1: 0.5, x2: 0.5, y2: 0.75 },
    ]);
  });

  it('case 10 중앙값<level: low(TR,BL) 연결 → 바깥 코너(TL,BR) 격리 2세그먼트', () => {
    // TL=6,TR=0,BR=6,BL=0. center=3 < level 5.
    const grid = new Float32Array([6, 0, 0, 6]);
    const segs = computeContours(grid, 2, 2, 5);
    expect(segs).toHaveLength(2);
    // TL 격리: topPt-leftPt / BR 격리: rightPt-bottomPt.
    // topPt(TL6-TR0): t=(5-6)/(0-6)=1/6 → x=0.25+ (1/6)*0.5=0.25+0.08333=0.33333, y=0.25.
    // leftPt(TL6-BL0): t=1/6 → x=0.25, y=0.25+(1/6)*0.5=0.33333.
    // rightPt(TR0-BR6): t=(5-0)/6=0.83333 → x=0.75, y=0.25+0.83333*0.5=0.66667.
    // bottomPt(BL0-BR6): t=0.83333 → x=0.25+0.83333*0.5=0.66667, y=0.75.
    expectSegments(segs, [
      { x1: 0.25 + 0.5 / 6, y1: 0.25, x2: 0.25, y2: 0.25 + 0.5 / 6 },
      { x1: 0.75, y1: 0.25 + (0.5 * 5) / 6, x2: 0.25 + (0.5 * 5) / 6, y2: 0.75 },
    ]);
  });

  it('case 5 중앙값>=level: high(TR,BL) 연결 → 바깥 코너(TL,BR) 격리', () => {
    // TR=10,BL=10,TL=0,BR=0 이면 caseIndex=5(TR+BL). center=5>=5.
    const grid = new Float32Array([0, 10, 10, 0]); // TL0,TR10,BL10,BR0
    const segs = computeContours(grid, 2, 2, 5);
    expect(segs).toHaveLength(2);
    // TL 격리: topPt-leftPt / BR 격리: rightPt-bottomPt.
    expectSegments(segs, [
      { x1: 0.5, y1: 0.25, x2: 0.25, y2: 0.5 },
      { x1: 0.75, y1: 0.5, x2: 0.5, y2: 0.75 },
    ]);
  });

  it('saddle 은 결정적이다(동일 입력 → 동일 세그먼트, 끊김/교차 없음)', () => {
    const grid = new Float32Array([10, 0, 0, 10]);
    const a = computeContours(grid, 2, 2, 5);
    const b = computeContours(grid, 2, 2, 5);
    expect(a).toEqual(b);
  });
});

describe('computeContours — 방어/엣지', () => {
  it('빈 격자·1행·1열 격자는 빈 배열(셀 없음)', () => {
    expect(computeContours(new Float32Array(0), 0, 0, 5)).toEqual([]);
    expect(computeContours(new Float32Array([1, 2, 3]), 3, 1, 2)).toEqual([]);
    expect(computeContours(new Float32Array([1, 2, 3]), 1, 3, 2)).toEqual([]);
  });

  it('비유한 level 은 빈 배열', () => {
    expect(computeContours(new Float32Array([0, 0, 0, 10]), 2, 2, NaN)).toEqual([]);
    expect(computeContours(new Float32Array([0, 0, 0, 10]), 2, 2, Infinity)).toEqual([]);
  });

  it('grid 길이가 gridW*gridH 보다 짧으면 빈 배열', () => {
    expect(computeContours(new Float32Array([0, 0]), 2, 2, 5)).toEqual([]);
  });

  it('3x3 격자 다중 셀에서도 예외 없이 세그먼트를 잇는다', () => {
    // 왼쪽 낮고 오른쪽 높은 수평 경사 → level 5 등치선이 세로로 형성된다.
    const grid = new Float32Array([0, 5, 10, 0, 5, 10, 0, 5, 10]); // 3x3
    const segs = computeContours(grid, 3, 3, 5);
    expect(segs.length).toBeGreaterThan(0);
    // 모든 좌표가 정규화(0..1) 범위 안이다.
    for (const s of segs) {
      for (const v of [s.x1, s.y1, s.x2, s.y2]) {
        expect(v).toBeGreaterThanOrEqual(0);
        expect(v).toBeLessThanOrEqual(1);
      }
    }
  });
});

describe('segmentsToPath', () => {
  it('각 세그먼트를 "M x1 y1 L x2 y2" 로 직렬화하고 공백으로 잇는다', () => {
    const segs: Segment[] = [
      { x1: 0.1, y1: 0.2, x2: 0.3, y2: 0.4 },
      { x1: 0.5, y1: 0.6, x2: 0.7, y2: 0.8 },
    ];
    expect(segmentsToPath(segs)).toBe('M 0.1 0.2 L 0.3 0.4 M 0.5 0.6 L 0.7 0.8');
  });

  it('빈 입력은 빈 문자열', () => {
    expect(segmentsToPath([])).toBe('');
  });
});
