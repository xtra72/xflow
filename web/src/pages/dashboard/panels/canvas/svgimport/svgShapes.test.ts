// 단축 요소 시험 (SPEC-CANVAS-007 M3 · AC-03).
//
// **이 파일이 무너뜨리는 기본값** — 모서리 없는 `rect`, 반지름이 상자보다 작은 `rx`,
// 짝수 개 좌표만 든 `points`. 셋 다 실제 파일이 자주 어기는 자리이며, 어기지 않는 고정
// 입력만으로는 죔(clamp) 도 홀수 처리도 실행되지 않는다.
//
// **확인한 뮤테이션(E12)**
//   1. `rx` 죔(`Math.min(…, w / 2)`)을 지우면 → "과대 rx 가 상자 안에 든다" 가 빨개진다.
//   2. `rx` 만 있을 때 `ry = rx` 로 잇는 폴백을 지우면 → "rx 만 있으면 ry 가 따라온다" 가
//      빨개진다.
//   3. `<line>`/`<polyline>` 에 `Z` 를 붙이면 → "열린 채로 둔다" 가 빨개진다.
//   4. `polygon` 의 `Z` 를 지우면 → "polygon 은 닫힌다" 가 빨개진다.
//   5. 타원의 `sweep` 을 0 으로 두면 → "감김 방향이 SVG 등가 경로와 같다" 가 빨개진다.
//   6. `parseLength` 가 `%` 를 받게 하면 → "% 는 크기가 아니다" 가 빨개진다.
//   7. `points` 의 짝짓기를 `numbers[i + 1]` → `numbers[i]` 로 어긋내면 → "좌표가 어긋나
//      짝지어지면" 이 빨개진다.
//   8. `points` 의 유한 검사를 지우면 → "비유한 좌표를 든 점은 버린다" 가 빨개진다.
//   9. 타원의 정지점 순서를 뒤집으면(반시계) → "감김 방향" 이 빨개진다.
//
// **물지 않아 자를 바꾼 가드 하나(E12)** — `points` 의 **홀수 꼬리 자르기**(`i + 1 <`)는
// 지워도 어느 시험도 빨개지지 않는다. `noUncheckedIndexedAccess` 아래에서 인덱스 접근이
// `| undefined` 라 바로 아래 검사가 같은 일을 하기 때문이다. 둘은 같은 가드를 두 번 적은
// 것이며, 실제로 무는 것은 **짝짓기**와 **유한 검사**다 — 그 둘로 자를 옮겼다.

import { describe, expect, it } from 'vitest';

import type { PathCommand } from '../shapes/pathTypes';

import { parseLength } from './svgPathData';
import {
  circleCommands,
  ellipseCommands,
  isShorthandShapeTag,
  lineCommands,
  polygonCommands,
  polylineCommands,
  rectCommands,
  shorthandShapeCommands,
} from './svgShapes';

/** 명령 목록이 지나는 모든 좌표(제어점 포함). */
function allPoints(cmds: readonly PathCommand[]): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = [];
  for (const cmd of cmds) {
    if (cmd.c === 'Z') continue;
    if (cmd.c === 'C') {
      out.push({ x: cmd.x1, y: cmd.y1 }, { x: cmd.x2, y: cmd.y2 });
    }
    out.push({ x: cmd.x, y: cmd.y });
  }
  return out;
}

/** 3차 베지어 목록을 촘촘히 뽑는다(제어점이 아니라 곡선 위 점을 재기 위해서다). */
function sample(cmds: readonly PathCommand[], steps = 32): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = [];
  let cur = { x: 0, y: 0 };
  for (const cmd of cmds) {
    if (cmd.c === 'M' || cmd.c === 'L') {
      out.push({ x: cmd.x, y: cmd.y });
      cur = { x: cmd.x, y: cmd.y };
      continue;
    }
    if (cmd.c !== 'C') continue;
    const p0 = cur;
    for (let i = 1; i <= steps; i += 1) {
      const t = i / steps;
      const u = 1 - t;
      out.push({
        x: u * u * u * p0.x + 3 * u * u * t * cmd.x1 + 3 * u * t * t * cmd.x2 + t * t * t * cmd.x,
        y: u * u * u * p0.y + 3 * u * u * t * cmd.y1 + 3 * u * t * t * cmd.y2 + t * t * t * cmd.y,
      });
    }
    cur = { x: cmd.x, y: cmd.y };
  }
  return out;
}

/** 닫은 다각형의 부호 있는 넓이 — 부호가 곧 감김 방향이다. */
function signedArea(points: readonly { x: number; y: number }[]): number {
  let sum = 0;
  for (let i = 0; i < points.length; i += 1) {
    const a = points[i];
    const b = points[(i + 1) % points.length];
    if (a === undefined || b === undefined) continue;
    sum += a.x * b.y - b.x * a.y;
  }
  return sum / 2;
}

describe('길이 속성 (뮤테이션 6)', () => {
  it('단위 없는 수와 px 만 크기다', () => {
    expect(parseLength('40')).toBe(40);
    expect(parseLength('40px')).toBe(40);
    expect(parseLength(' -2.5 ')).toBe(-2.5);
    expect(parseLength('.5')).toBe(0.5);
  });

  it('% 는 크기가 아니다 — 뷰포트가 없으므로 "무엇의 몇 %" 에 답할 자리가 없다', () => {
    expect(parseLength('50%')).toBeUndefined();
  });

  it('모르는 단위를 부분만 삼키지 않는다 — 50em 을 50 으로 읽으면 그림이 조용히 작아진다', () => {
    expect(parseLength('50em')).toBeUndefined();
    expect(parseLength('banana')).toBeUndefined();
    expect(parseLength(undefined)).toBeUndefined();
  });
});

describe('rect', () => {
  it('모서리가 없으면 M L L L Z 다', () => {
    expect(rectCommands({ x: '5', y: '5', width: '40', height: '20' })).toEqual([
      { c: 'M', x: 5, y: 5 },
      { c: 'L', x: 45, y: 5 },
      { c: 'L', x: 45, y: 25 },
      { c: 'L', x: 5, y: 25 },
      { c: 'Z' },
    ]);
  });

  it('rx 만 있으면 ry 가 따라온다 (뮤테이션 2)', () => {
    const onlyRx = rectCommands({ x: '0', y: '0', width: '40', height: '40', rx: '6' });
    const both = rectCommands({ x: '0', y: '0', width: '40', height: '40', rx: '6', ry: '6' });
    expect(onlyRx).toEqual(both);
  });

  it('ry 만 있으면 rx 가 따라온다 (뮤테이션 2)', () => {
    const onlyRy = rectCommands({ x: '0', y: '0', width: '40', height: '40', ry: '6' });
    const both = rectCommands({ x: '0', y: '0', width: '40', height: '40', rx: '6', ry: '6' });
    expect(onlyRy).toEqual(both);
  });

  it('과대 rx 가 w/2 로 죄여 상자 안에 든다 (뮤테이션 1)', () => {
    // `rx="99"` 는 실제 파일에 흔하다(도구가 "완전히 둥글게" 를 큰 수로 적는다).
    // 죄지 않으면 모서리 호가 상자 밖으로 나가 도형이 뒤집힌다.
    const cmds = rectCommands({ x: '5', y: '5', width: '40', height: '20', rx: '99' });
    expect(cmds.length).toBeGreaterThan(0);
    for (const p of sample(cmds)) {
      expect(p.x).toBeGreaterThanOrEqual(5 - 1e-9);
      expect(p.x).toBeLessThanOrEqual(45 + 1e-9);
      expect(p.y).toBeGreaterThanOrEqual(5 - 1e-9);
      expect(p.y).toBeLessThanOrEqual(25 + 1e-9);
    }
    // 죈 뒤 rx = 20 = w/2 이므로 첫 명령이 상자의 가로 중앙이다.
    expect(cmds[0]).toEqual({ c: 'M', x: 25, y: 5 });
  });

  it('음수 rx 는 지정하지 않은 것으로 읽는다', () => {
    const negative = rectCommands({ x: '0', y: '0', width: '40', height: '20', rx: '-5' });
    const none = rectCommands({ x: '0', y: '0', width: '40', height: '20' });
    expect(negative).toEqual(none);
  });

  it('모서리 있는 rect 는 3차 넷과 직선 넷으로 적힌다', () => {
    const cmds = rectCommands({ x: '0', y: '0', width: '40', height: '20', rx: '4' });
    expect(cmds.filter((c) => c.c === 'C')).toHaveLength(4);
    expect(cmds.filter((c) => c.c === 'L')).toHaveLength(4);
    expect(cmds[cmds.length - 1]).toEqual({ c: 'Z' });
  });

  it('폭이나 높이가 0 이하면 그릴 것이 없다', () => {
    expect(rectCommands({ width: '0', height: '10' })).toEqual([]);
    expect(rectCommands({ width: '10', height: '-1' })).toEqual([]);
    expect(rectCommands({})).toEqual([]);
  });
});

describe('circle · ellipse', () => {
  it('원은 4분원 넷 + Z 다', () => {
    const cmds = circleCommands({ cx: '10', cy: '20', r: '5' });
    expect(cmds.map((c) => c.c)).toEqual(['M', 'C', 'C', 'C', 'C', 'Z']);
  });

  it('타원 위의 점이 실제로 타원 위에 있다 — 비정사각 반지름으로 잰다', () => {
    // 정원(rx = ry)은 **두 축을 맞바꾼 결함을 감춘다.** 반지름을 다르게 둔다.
    const rx = 30;
    const ry = 12;
    const cmds = ellipseCommands({ cx: '7', cy: '-3', rx: String(rx), ry: String(ry) });
    // **허용 오차가 호 근사의 실측 최악값이다** — 두 수를 따로 세지 않는다(불변식 K5).
    // 4분원은 조각각이 상한(90°)에 닿는 자리라 오차가 정확히 그 최악값에 선다.
    const worstArcError = 2.72529e-4;
    for (const p of sample(cmds)) {
      const radius = Math.hypot((p.x - 7) / rx, (p.y + 3) / ry);
      expect(Math.abs(radius - 1)).toBeLessThanOrEqual(worstArcError);
    }
  });

  it('감김 방향이 SVG 등가 경로와 같다 — sweep = 1 (뮤테이션 5)', () => {
    // nonzero 채움에서 도넛의 구멍이 뚫리려면 브라우저가 그리는 방향과 같아야 한다.
    expect(signedArea(sample(circleCommands({ cx: '0', cy: '0', r: '10' })))).toBeGreaterThan(0);
    expect(signedArea(sample(rectCommands({ x: '0', y: '0', width: '10', height: '10' })))).toBeGreaterThan(0);
  });

  it('반지름이 0 이하면 그릴 것이 없다', () => {
    expect(circleCommands({ cx: '1', cy: '1', r: '0' })).toEqual([]);
    expect(circleCommands({ cx: '1', cy: '1', r: '-3' })).toEqual([]);
    expect(ellipseCommands({ rx: '5' })).toEqual([]);
  });
});

describe('line · polyline · polygon — 열림과 닫힘 (뮤테이션 3 · 4)', () => {
  it('line 은 Z 없이 열린 채로 둔다', () => {
    // 닫으면 `pathSeedStyle` 이 **채움 씨앗**을 심어 저술한 적 없는 변이 하나 생긴다
    // (실측 `canvasElementFactory.ts:225`).
    const cmds = lineCommands({ x1: '0', y1: '40', x2: '120', y2: '40' });
    expect(cmds).toEqual([
      { c: 'M', x: 0, y: 40 },
      { c: 'L', x: 120, y: 40 },
    ]);
    expect(cmds.some((c) => c.c === 'Z')).toBe(false);
  });

  it('polyline 도 열린 채로 둔다', () => {
    const cmds = polylineCommands({ points: '0,0 10,5 20,0' });
    expect(cmds.map((c) => c.c)).toEqual(['M', 'L', 'L']);
  });

  it('polygon 은 닫힌다', () => {
    const cmds = polygonCommands({ points: '0,0 10,5 20,0' });
    expect(cmds.map((c) => c.c)).toEqual(['M', 'L', 'L', 'Z']);
  });

  it('짝을 이루지 못한 좌표는 점이 아니다 (뮤테이션 7)', () => {
    // 홀수 개 좌표를 든 `points` 는 실제 파일에 나온다(수동 편집 · 잘린 내보내기).
    expect(polylineCommands({ points: '0,0 10,5 20' })).toEqual([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10, y: 5 },
    ]);
  });

  it('비유한 좌표를 든 점은 버린다 — 나머지가 산다', () => {
    expect(polylineCommands({ points: '0,0 1e999,5 20,0' })).toEqual([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 20, y: 0 },
    ]);
  });

  it('좌표가 어긋나 짝지어지면 그림이 달라진다 — 짝짓기 자체를 잰다', () => {
    // 홀수 꼬리를 자르는 루프 상한은 `noUncheckedIndexedAccess` 가 강제하는 인덱스
    // 검사와 **같은 가드**라 하나만 지워도 거동이 같다. 실제로 무는 것은 짝짓기다.
    expect(polygonCommands({ points: '0,0 10,5 20,0' })).toEqual([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10, y: 5 },
      { c: 'L', x: 20, y: 0 },
      { c: 'Z' },
    ]);
  });

  it('구분자가 공백이든 쉼표든 부호든 같은 점 목록이다', () => {
    const commas = polygonCommands({ points: '0,0 10,5 20,0' });
    expect(polygonCommands({ points: '0 0 10 5 20 0' })).toEqual(commas);
    expect(polygonCommands({ points: '0-0,10 5,20-0' })).toEqual([
      { c: 'M', x: 0, y: -0 },
      { c: 'L', x: 10, y: 5 },
      { c: 'L', x: 20, y: -0 },
      { c: 'Z' },
    ]);
  });

  it('점이 하나뿐이거나 없으면 그릴 것이 없다', () => {
    expect(polylineCommands({ points: '1,2' })).toEqual([]);
    expect(polygonCommands({ points: '' })).toEqual([]);
    expect(polygonCommands({})).toEqual([]);
  });
});

describe('갈래 나누기', () => {
  it('단축 요소 여섯을 안다', () => {
    for (const tag of ['rect', 'circle', 'ellipse', 'line', 'polyline', 'polygon']) {
      expect(isShorthandShapeTag(tag)).toBe(true);
      expect(shorthandShapeCommands(tag, { width: '4', height: '4', r: '2', rx: '2', ry: '2', points: '0,0 1,1' })).toBeDefined();
    }
  });

  it('단축 요소가 아니면 undefined 다 — "그릴 것이 없다"(빈 배열)와 값으로 가른다', () => {
    expect(isShorthandShapeTag('path')).toBe(false);
    expect(shorthandShapeCommands('path', { d: 'M 0 0' })).toBeUndefined();
    expect(shorthandShapeCommands('text', {})).toBeUndefined();
    expect(shorthandShapeCommands('rect', {})).toEqual([]);
  });

  it('손상된 속성에 예외가 없다 (REQ-07)', () => {
    expect(() => rectCommands({ x: 'a', y: 'b', width: 'c', height: 'd' })).not.toThrow();
    expect(() => polygonCommands({ points: 'banana' })).not.toThrow();
    expect(() => circleCommands({ r: '1e999' })).not.toThrow();
    expect(circleCommands({ r: '1e999' })).toEqual([]);
  });

  it('모든 산출이 M · L · C · Z 넷뿐이다(값으로 단언한다)', () => {
    const bags: [string, Record<string, string>][] = [
      ['rect', { width: '10', height: '10', rx: '3' }],
      ['circle', { r: '5' }],
      ['ellipse', { rx: '5', ry: '3' }],
      ['line', { x2: '5' }],
      ['polyline', { points: '0,0 1,1' }],
      ['polygon', { points: '0,0 1,1 2,0' }],
    ];
    for (const [tag, attrs] of bags) {
      for (const cmd of shorthandShapeCommands(tag, attrs) ?? []) {
        expect(['M', 'L', 'C', 'Z']).toContain(cmd.c);
      }
    }
  });

  it('모서리 있는 rect 의 좌표가 전부 유한하다', () => {
    for (const p of allPoints(rectCommands({ width: '10', height: '10', rx: '5', ry: '5' }))) {
      expect(Number.isFinite(p.x)).toBe(true);
      expect(Number.isFinite(p.y)).toBe(true);
    }
  });
});
