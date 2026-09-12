// 경로 데이터 토크나이저·축약 시험 (SPEC-CANVAS-007 M1 · AC-01).
//
// **시험 규율(acceptance.md §시험 규율)이 이 파일의 고정 입력을 정한다.** SVG 의 기본값은
// 거의 전부 "항등" 이라 기본값 모양의 고정 입력은 이 층의 산술을 하나도 시험하지 않는다.
// 아래 각 시험 이름 뒤의 `[E#]` 가 그 시험이 무너뜨리는 기본값을 가리킨다.
//
//   E4 — **절대 명령만 쓴 `d`** 는 상대 축약 전부와 `Z` 뒤 현재 점 규칙을 감춘다.
//        절대 명령에서는 그 규칙이 결과에 영향을 주지 않는다.
//   E5 — **`M x y` 뒤에 좌표쌍이 하나뿐인 `d`** 는 암묵 `L` 규칙을 실행조차 하지 않는다.
//
// **확인한 뮤테이션(E12)** — 각각 어느 단언이 빨개지는지 아래 시험 이름에 적었다.
//   1. `Z` 뒤 현재 점을 `state.sx/sy` 대신 직전 점으로 두면
//      → "Z 뒤의 상대 m 은 부분 경로 시작점에서 출발한다 [E4]" 가 빨개진다.
//   2. `reflect` 의 폴백을 "현재 점" 대신 "직전 제어점 그대로" 로 바꾸면
//      → "직전이 C 가 아닌 S 의 첫 제어점은 현재 점이다 [E4]" 가 빨개진다.
//   3. `IMPLICIT_REPEAT` 에서 `M: 'L'` 을 지우면
//      → "M 뒤에 이어진 좌표쌍은 암묵 L 이다 [E5]" 가 빨개진다.
//   4. `readFlag` 를 `readNumber` 로 바꾸면
//      → "붙은 플래그 0160 은 세 토큰이다" 가 빨개진다.
//   5. `ReduceState` 의 원점 초기화(`cx: 0, cy: 0`)를 `cx: 100, cy: 100` 으로 바꾸면
//      → "첫 m 은 절대다" 가 빨개진다.
//
// **물지 않아 강화한 가드(E12)** — 처음에는 첫 이동을 `firstMove` 분기로 절대 취급했다.
// 그 분기를 지워도 **어느 시험도 빨개지지 않았다**: 첫 명령은 반드시 이동이고 그때 현재
// 점은 언제나 `(0,0)` 이라 상대 덧셈이 곧 절대 대입이기 때문이다. 아무것도 지키지 않는
// 가드를 남기는 대신 지우고, 그 규칙을 실제로 지고 있는 **원점 초기화**를 뮤테이션 5 로
// 옮겼다 — 그쪽은 문다.

import { describe, expect, it } from 'vitest';

import type { PathCommand } from '../shapes/pathTypes';

import {
  parseNumberList,
  reducePathData,
  reducePathSegments,
  tokenizePathData,
  type ArcReducer,
} from './svgPathData';

/**
 * 호 축약기 대역. **M1 은 호를 축약하지 않는다** — 여기서는 위임이 일어났는가와 인자가
 * 무엇이었는가만 본다. 진짜 축약은 M2(`svgArc.ts`)의 몫이다.
 */
function recordingArcReducer(): { calls: unknown[][]; fn: ArcReducer } {
  const calls: unknown[][] = [];
  const fn: ArcReducer = (x0, y0, rx, ry, rot, fa, fs, x1, y1) => {
    calls.push([x0, y0, rx, ry, rot, fa, fs, x1, y1]);
    return [{ c: 'L', x: x1, y: y1 }];
  };
  return { calls, fn };
}

/** 축약이 호에 닿지 않는 고정 입력에서 쓰는 대역 — 불려서는 안 된다. */
const neverArc: ArcReducer = () => {
  throw new Error('arc reducer must not be called');
};

describe('수 문법', () => {
  it('5.5.5 는 두 수다 — parseFloat 한 번으로 읽으면 하나가 된다', () => {
    expect(parseNumberList('5.5.5')).toEqual([5.5, 0.5]);
  });

  it('부호가 곧 구분자다 — 10-20 은 두 수다', () => {
    expect(parseNumberList('10-20')).toEqual([10, -20]);
  });

  it('.5 · -.5 · 1e-5 · 1E5 가 전부 합법이다', () => {
    expect(parseNumberList('.5 -.5 1e-5 1E5')).toEqual([0.5, -0.5, 1e-5, 1e5]);
  });

  it('지수부에 자릿수가 없으면 e 는 수의 일부가 아니다', () => {
    // `1e` 뒤에 `M` 이 온다고 보면 수는 `1` 하나다. 되돌리기가 빠지면 NaN 이 된다.
    expect(parseNumberList('1e')).toEqual([1]);
  });

  it('수가 아닌 것을 만나면 거기서 멈추고 지금까지를 돌려준다', () => {
    expect(parseNumberList('1 2 banana 3')).toEqual([1, 2]);
  });

  it('쉼표와 공백이 섞여도 같은 목록이다', () => {
    expect(parseNumberList(' 1 , 2,3  4 ')).toEqual([1, 2, 3, 4]);
  });
});

describe('토크나이저', () => {
  it('붙은 플래그 0160 은 세 토큰이다 — fa=0 · fs=1 · x=60 (뮤테이션 4)', () => {
    // SVGO · Illustrator · Figma 의 내보내기가 정확히 이 형태를 낸다(위험 R2).
    const { segments } = tokenizePathData('M 0 0 a50 30 20 0160 20');
    expect(segments[1]).toEqual({ code: 'a', args: [50, 30, 20, 0, 1, 60, 20] });
  });

  it('플래그가 0/1 이 아니면 그 명령을 버리고 나머지를 살린다', () => {
    const { segments, dropped } = tokenizePathData('M 0 0 A 50 30 20 7 1 60 20 L 9 9');
    expect(dropped).toBe(1);
    expect(segments.map((s) => s.code)).toEqual(['M', 'L']);
    expect(segments[1]?.args).toEqual([9, 9]);
  });

  it('M 뒤에 이어진 좌표쌍은 암묵 L 이다 [E5] (뮤테이션 3)', () => {
    // 좌표쌍이 하나뿐인 고정 입력에서는 이 규칙이 **실행되지 않는다**.
    const { segments } = tokenizePathData('M 1 2 3 4 5 6');
    expect(segments.map((s) => s.code)).toEqual(['M', 'L', 'L']);
  });

  it('m 뒤에 이어진 좌표쌍은 암묵 l 이다(상대를 유지한다) [E5]', () => {
    const { segments } = tokenizePathData('m 1 2 3 4');
    expect(segments.map((s) => s.code)).toEqual(['m', 'l']);
  });

  it('Z 는 되풀이되지 않는다', () => {
    const { segments } = tokenizePathData('M 0 0 Z 5 5');
    expect(segments.map((s) => s.code)).toEqual(['M', 'Z']);
  });

  it('비유한 좌표는 그 명령만 버리고 위치를 잃지 않는다', () => {
    const { segments, dropped } = tokenizePathData('M 0 0 L 1e999 0 L 7 8');
    expect(dropped).toBe(1);
    expect(segments.map((s) => s.code)).toEqual(['M', 'L']);
    expect(segments[1]?.args).toEqual([7, 8]);
  });

  it('인자가 모자란 명령은 버려진다', () => {
    const { segments, dropped } = tokenizePathData('M 1');
    expect(segments).toEqual([]);
    expect(dropped).toBe(1);
  });
});

describe('축약 — 어휘', () => {
  it('결과의 모든 원소가 M · L · C · Z 넷 가운데 하나다(값으로 단언한다)', () => {
    // 타입은 런타임에 없다. 값으로 재지 않으면 다섯 번째 갈래가 조용히 실린다.
    const d = 'M 1 2 3 4 H 9 V 8 C 1 1 2 2 3 3 S 4 4 5 5 Q 6 6 7 7 T 8 8 Z';
    const out = reducePathData(d, neverArc);
    expect(out.length).toBeGreaterThan(0);
    for (const cmd of out) expect(['M', 'L', 'C', 'Z']).toContain(cmd.c);
  });

  it('첫 명령이 이동이 아니면 아무것도 그리지 않는다', () => {
    expect(reducePathData('L 10 10', neverArc)).toEqual([]);
    expect(reducePathData('', neverArc)).toEqual([]);
    expect(reducePathData('banana', neverArc)).toEqual([]);
  });
});

describe('축약 — 무손실 갈래', () => {
  it('H 와 V 의 빠진 축은 현재 점의 값이다', () => {
    const out = reducePathData('M 10 20 H 30 V 40', neverArc);
    expect(out).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 30, y: 20 },
      { c: 'L', x: 30, y: 40 },
    ]);
  });

  it('상대 h/v 는 현재 점에 더한다 [E4]', () => {
    const out = reducePathData('M 10 20 h 5 v -7', neverArc);
    expect(out).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 15, y: 20 },
      { c: 'L', x: 15, y: 13 },
    ]);
  });

  it('Q → C 가 c1 = p0 + ⅔(q − p0) · c2 = p + ⅔(q − p) 와 정확히 같다', () => {
    const out = reducePathData('M 0 0 Q 30 90 60 0', neverArc);
    const cubic = out[1];
    expect(cubic?.c).toBe('C');
    if (cubic?.c !== 'C') throw new Error('unreachable');
    expect(cubic.x1).toBeCloseTo(20, 9);
    expect(cubic.y1).toBeCloseTo(60, 9);
    expect(cubic.x2).toBeCloseTo(40, 9);
    expect(cubic.y2).toBeCloseTo(60, 9);
    expect(cubic.x).toBeCloseTo(60, 9);
    expect(cubic.y).toBeCloseTo(0, 9);
  });

  it('직전이 C 인 S 의 첫 제어점은 반사다', () => {
    const out = reducePathData('M 0 0 C 10 40 30 40 40 0 S 70 -40 80 0', neverArc);
    const s = out[2];
    if (s?.c !== 'C') throw new Error('expected cubic');
    // 2·(40,0) − (30,40) = (50,−40)
    expect([s.x1, s.y1]).toEqual([50, -40]);
    expect([s.x2, s.y2]).toEqual([70, -40]);
  });

  it('직전이 C 가 아닌 S 의 첫 제어점은 현재 점이다 [E4] (뮤테이션 2)', () => {
    // **폴백 갈래는 앞의 경우만으로는 절대 실행되지 않는다.** 두 갈래를 모두 둔다.
    const out = reducePathData('M 0 0 L 10 0 S 30 40 40 0', neverArc);
    const s = out[2];
    if (s?.c !== 'C') throw new Error('expected cubic');
    expect([s.x1, s.y1]).toEqual([10, 0]);
  });

  it('직전이 Q 인 T 의 2차 제어점은 반사다', () => {
    const out = reducePathData('M 0 0 Q 10 20 20 0 T 40 0', neverArc);
    const t = out[2];
    if (t?.c !== 'C') throw new Error('expected cubic');
    // q = 2·(20,0) − (10,20) = (30,−20) → c1 = (20,0) + ⅔((30,−20) − (20,0))
    expect(t.x1).toBeCloseTo(20 + 20 / 3, 9);
    expect(t.y1).toBeCloseTo(-40 / 3, 9);
  });

  it('직전이 Q 가 아닌 T 의 2차 제어점은 현재 점이다 [E4]', () => {
    const out = reducePathData('M 0 0 L 10 0 T 30 0', neverArc);
    const t = out[2];
    if (t?.c !== 'C') throw new Error('expected cubic');
    // q = 현재 점 (10,0) → c1 = (10,0), c2 = (30,0) + ⅔((10,0) − (30,0))
    expect(t.x1).toBeCloseTo(10, 9);
    expect(t.x2).toBeCloseTo(30 - 40 / 3, 9);
  });

  it('S 는 연쇄한다 — 직전 S 의 c2 도 반사원이 된다', () => {
    const out = reducePathData('M 0 0 C 0 10 10 10 10 0 S 20 -10 20 0 S 30 10 30 0', neverArc);
    const third = out[3];
    if (third?.c !== 'C') throw new Error('expected cubic');
    // 직전 S 의 c2 = (20,−10), 현재 점 (20,0) → 2·(20,0) − (20,−10) = (20,10)
    expect([third.x1, third.y1]).toEqual([20, 10]);
  });
});

describe('축약 — 상대와 부분 경로 [E4]', () => {
  it('첫 m 은 절대다 — 현재 점이 원점에서 열리기 때문이다 (뮤테이션 5)', () => {
    // 사양 §8.3.2. 원점 초기화가 이 규칙을 지는 자리이며, 그것을 옮기면 빨개진다.
    const out = reducePathData('m 10 20 l 5 0', neverArc);
    expect(out).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 15, y: 20 },
    ]);
  });

  it('Z 뒤의 상대 m 은 부분 경로 시작점에서 출발한다 [E4] (뮤테이션 1)', () => {
    // 직전 점은 (30,20) 이고 부분 경로 시작점은 (10,20) 이다 — 둘이 다른 고정 입력이라야
    // 이 규칙이 관측된다.
    const out = reducePathData('M 10 20 L 30 20 Z m 10 10 l 5 0', neverArc);
    expect(out).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 30, y: 20 },
      { c: 'Z' },
      { c: 'M', x: 20, y: 30 },
      { c: 'L', x: 25, y: 30 },
    ]);
  });

  it('두 번째 M 은 새 부분 경로의 시작점을 다시 정한다', () => {
    const out = reducePathData('M 0 0 L 5 0 M 100 100 L 105 100 Z l 1 1', neverArc);
    // Z 뒤 현재 점은 두 번째 부분 경로의 시작점 (100,100) 이다.
    expect(out[out.length - 1]).toEqual({ c: 'L', x: 101, y: 101 });
  });
});

describe('축약 — 호는 위임된다(불변식 K2)', () => {
  it('호 인자가 그대로 축약기에 넘어간다 — 현재 점이 시작점이다', () => {
    const { calls, fn } = recordingArcReducer();
    reducePathData('M 5 5 a 50 30 20 0 1 60 20', fn);
    expect(calls).toEqual([[5, 5, 50, 30, 20, 0, 1, 65, 25]]);
  });

  it('축약기가 빈 목록을 내면 현재 점도 움직이지 않는다', () => {
    const empty: ArcReducer = () => [];
    const out = reducePathData('M 5 5 A 10 10 0 0 1 5 5 L 7 7', empty);
    expect(out).toEqual([
      { c: 'M', x: 5, y: 5 },
      { c: 'L', x: 7, y: 7 },
    ]);
  });

  it('축약기가 낸 명령들이 그대로 실린다', () => {
    const two: ArcReducer = (_x0, _y0, _rx, _ry, _rot, _fa, _fs, x1, y1) => [
      { c: 'C', x1: 1, y1: 1, x2: 2, y2: 2, x: 3, y: 3 },
      { c: 'L', x: x1, y: y1 },
    ];
    const out = reducePathData('M 0 0 A 1 1 0 0 0 9 9', two);
    expect(out.map((c) => c.c)).toEqual(['M', 'C', 'L']);
  });
});

describe('견고성 — 예외가 밖으로 나오지 않는다 (REQ-07 · AC-E4)', () => {
  const corrupt = ['', 'banana', 'M', 'M 1', 'M 1 NaN', 'M 1e999 0', 'Z', 'M 0 0 L', '   '];

  for (const d of corrupt) {
    it(`손상 입력 ${JSON.stringify(d)} 이 값으로 돌아온다`, () => {
      let out: PathCommand[] | undefined;
      expect(() => {
        out = reducePathData(d, neverArc);
      }).not.toThrow();
      expect(Array.isArray(out)).toBe(true);
    });
  }

  it('손상 좌표를 만나도 그 명령 하나만 버리고 나머지가 산다', () => {
    const out = reducePathData('M 0 0 L 10 banana L 20 20', neverArc);
    expect(out).toEqual([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 20, y: 20 },
    ]);
  });

  it('빈 명령 목록을 축약해도 빈 목록이다', () => {
    expect(reducePathSegments([], neverArc)).toEqual([]);
  });
});
