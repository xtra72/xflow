// 상자 안 투영 3종과 008 경로 투영의 합치 (SPEC-CANVAS-004 M2).
//
// 이 파일이 가장 먼저 재는 것은 **합치 항등식**이다(AC-02). 004 와 008 은 같은 로컬
// 격자 위에 서지만 **다른 함수**를 지나므로, 둘이 시간이 지나며 갈라지지 않게 잡아 두는
// 것은 테스트뿐이다.
//
// ## 이 파일이 SPEC 을 하나 정정한다 — "정확히 같다" 는 성립하지 않는다
//
// acceptance.md AC-02 은 두 경로의 결과가 **정확히 같다**고 적었다. 부동소수 결합 법칙
// 때문에 그것은 참이 아니다:
//
//   - `projectPathPoints` : `원점 + 로컬 × (상자변 ÷ EXTENT)`   — 먼저 축척을 짓는다
//   - `projectPointIn`    : `원점 + (로컬 ÷ EXTENT) × 상자변`   — 먼저 좌표를 나눈다
//
// `x × (w/E)` 와 `(x/E) × w` 는 실수에서는 같고 배정밀도에서는 **마지막 비트가 다를 수
// 있다**(실측: 표본의 약 22% 에서 다르고 최대 어긋남은 2.9e-11 px). 004 가 그 어긋남을
// 없애려면 둘 중 하나를 고쳐야 하는데, `projectPathPoints` 는 008 의 산출이라 **바꾸지
// 않고**(REQ-05) `project*In` 이 제 나눗셈을 새로 적으면 **불변식 G2 가 깨진다.**
//
// 그래서 이 파일은 지키려던 성질을 **잴 수 있는 형태로** 적는다:
//   ① 어긋남이 부동소수 잔여(≤ 1e-9 px) 안에 있다 — 두 격자가 같은 격자다.
//   ② **정수로 죄면 같다** — 화면과 저장이 보는 값은 한 자리도 다르지 않다.
//   ③ 어긋남이 0 인 좌표에서는 `===` 로도 같다(항등식이 실제로 성립하는 자리가 있다).
// 어느 한 쪽만 재면 "정확히 같다" 는 거짓 주장이 되거나(①만), 갈라짐이 숨는다(②만).
//
// 고정 입력은 시험 규율이 요구하는 형상이다 — 상자 `317 × 181`(E-C · E-B), 원점
// `(73, 41)` 과 음수 원점(E-D), 비단위 축척.
//
// @spec SPEC-CANVAS-004 REQ-03 · AC-02 · 불변식 G2

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import {
  coordinate,
  type BoxGeometry,
  type LineGeometry,
  type PointGeometry,
} from './canvasConfig';
import {
  localProjection,
  projectBox,
  projectBoxIn,
  projectLineIn,
  projectPathPoints,
  projectPointIn,
  type PxBox,
} from './canvasGeometry';
import { GROUP_LOCAL_EXTENT, GROUP_LOCAL_SIZE } from './group/groupTypes';

/** 그룹 px 상자 — 비정사각 · 원점 ≠ 0 · 두 변 모두 EXTENT 를 나누어떨어뜨리지 않는다. */
const BOX: PxBox = { x: 73, y: 41, w: 317, h: 181 };
/** 원점이 음수인 상자(E-D — 캔버스 밖 저술은 합법이다). */
const NEG_BOX: PxBox = { x: -120, y: -37, w: 317, h: 181 };

/** 부동소수 잔여의 상한(px). 화면 한 픽셀의 10 억분의 1 이다. */
const FLOAT_RESIDUE = 1e-9;

/** 격자를 두루 훑는 로컬 좌표 — 0 · EXTENT 같은 가장자리뿐 아니라 안쪽을 포함한다(E-A). */
const LOCALS: readonly number[] = [
  0, 1, 7, 137, 1234, 4100, 5000, 6789, 9999, GROUP_LOCAL_EXTENT,
  -1, -3300, GROUP_LOCAL_EXTENT + 2500,
];

function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function source(name: string): string {
  return stripComments(fs.readFileSync(path.join(__dirname, name), 'utf8'));
}

// --- ① 합치 항등식 (AC-02) -------------------------------------------------

describe('합치 항등식 — 경로 투영과 부품 투영이 같은 격자를 본다 (AC-02)', () => {
  it('두 경로의 어긋남이 부동소수 잔여 안이다', () => {
    for (const box of [BOX, NEG_BOX]) {
      for (const x of LOCALS) {
        for (const y of LOCALS) {
          const viaPath = projectPathPoints([{ c: 'M', x, y }], box)[0];
          const viaPart = projectPointIn({ x, y }, box);
          expect(viaPath?.c).toBe('M');
          if (viaPath?.c !== 'M') throw new Error('M 명령이어야 한다');
          expect(Math.abs(viaPath.x - viaPart.x), `x=${x} w=${box.w}`).toBeLessThanOrEqual(
            FLOAT_RESIDUE,
          );
          expect(Math.abs(viaPath.y - viaPart.y), `y=${y} h=${box.h}`).toBeLessThanOrEqual(
            FLOAT_RESIDUE,
          );
        }
      }
    }
  });

  it('정수로 죄면 **한 자리도 다르지 않다** — 화면과 저장이 보는 값이 같다', () => {
    for (const box of [BOX, NEG_BOX, { x: 0, y: 0, w: 100000, h: 100000 }]) {
      for (const x of LOCALS) {
        for (const y of LOCALS) {
          const viaPath = projectPathPoints([{ c: 'M', x, y }], box)[0];
          const viaPart = projectPointIn({ x, y }, box);
          if (viaPath?.c !== 'M') throw new Error('M 명령이어야 한다');
          expect([coordinate(viaPath.x, 0), coordinate(viaPath.y, 0)], `${x},${y}`).toEqual([
            coordinate(viaPart.x, 0),
            coordinate(viaPart.y, 0),
          ]);
        }
      }
    }
  });

  it('어긋남이 0 인 자리에서는 `===` 로도 같다 — 항등식이 빈 주장이 아니다', () => {
    // 한 표본이라도 정확히 같아야 "두 식이 같은 식" 이라는 주장이 실체를 갖는다.
    const exact = LOCALS.filter((x) => {
      const a = projectPathPoints([{ c: 'M', x, y: 0 }], BOX)[0];
      const b = projectPointIn({ x, y: 0 }, BOX);
      return a?.c === 'M' && a.x === b.x;
    });
    expect(exact.length).toBeGreaterThan(0);
  });
});

// --- ② 새 나눗셈이 없다 (불변식 G2) ---------------------------------------

describe('새 나눗셈 자리를 만들지 않았다 (불변식 G2 · 008 J3)', () => {
  const geometry = source('canvasGeometry.ts');

  it('`PATH_LOCAL_EXTENT` 로 나누는 **함수**는 여전히 하나다 — `projectPathPoints`', () => {
    // 008 불변식 J3 이 세는 것은 등장 횟수가 아니라 **함수**다(그 함수 안에 축마다 한 번씩
    // 두 자리가 있다). 그래서 자리마다 "그 함수 몸체 안인가" 를 묻는다.
    const start = geometry.indexOf('export function projectPathPoints(');
    const end = geometry.indexOf('\n}', start);
    expect(start, '`projectPathPoints` 가 있어야 한다').toBeGreaterThan(-1);
    const outside: number[] = [];
    const re = /\/\s*PATH_LOCAL_EXTENT/g;
    for (let m = re.exec(geometry); m !== null; m = re.exec(geometry)) {
      if (m.index < start || m.index > end) outside.push(m.index);
    }
    // 켜져 있음을 먼저 잰다: 그 함수 안에 실제로 자리가 있어야 아래 단언이 무엇인가를 잰다.
    expect(geometry.slice(start, end).match(/\/\s*PATH_LOCAL_EXTENT/g) ?? []).toHaveLength(2);
    expect(outside).toEqual([]);
  });

  it('`GROUP_LOCAL_EXTENT` 로 나누는 자리는 **없다** — 격자는 데이터로 넘어간다', () => {
    expect(geometry.match(/\/\s*GROUP_LOCAL_EXTENT/g) ?? []).toHaveLength(0);
    // 좌표 변환 모듈에도 그 글자가 없다(같은 이유).
    const coords = source('group/groupCoords.ts');
    expect(coords).not.toContain('GROUP_LOCAL_EXTENT');
    expect(coords).not.toContain('PATH_LOCAL_EXTENT');
  });

  it('`project*In` 셋이 모두 `localProjection(box)` 로 `project*` 을 부른다', () => {
    for (const [fn, delegate] of [
      ['projectBoxIn', 'projectBox(geo, localProjection(box))'],
      ['projectLineIn', 'projectLine(geo, localProjection(box))'],
      ['projectPointIn', 'projectPoint(geo, localProjection(box))'],
    ] as const) {
      const at = geometry.indexOf(`export function ${fn}(`);
      expect(at, fn).toBeGreaterThan(-1);
      const body = geometry.slice(at, geometry.indexOf('\n}', at));
      expect(body, fn).toContain(delegate);
    }
  });

  it('셋 다 인자가 **둘뿐**이다 — 매개변수 목록의 형상으로 잰다', () => {
    // `Function.length` 는 첫 기본값 앞까지만 세므로 쓰지 않는다(시험 규율).
    for (const fn of ['projectBoxIn', 'projectLineIn', 'projectPointIn'] as const) {
      const decl = new RegExp(
        `export function ${fn}\\(\\s*geo: \\w+,\\s*box: PxBox,?\\s*\\): \\w+ \\{`,
      );
      expect(decl.test(geometry), fn).toBe(true);
    }
  });
});

// --- ③ 산술 (E-A · E-B · E-C · E-D) ---------------------------------------

describe('상자 안 투영의 산술', () => {
  it('상자를 가득 채우는 부품의 px 상자가 그룹의 px 상자와 같다', () => {
    const full: BoxGeometry = { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT };
    expect(projectBoxIn(full, BOX)).toEqual(BOX);
  });

  it('안쪽에 떠 있는 부품은 원점과 축척을 **둘 다** 실행시킨다 (E-A · E-D)', () => {
    const inner: BoxGeometry = { x: 4100, y: 3300, w: 1700, h: 900 };
    const px = projectBoxIn(inner, BOX);
    // 73 + 0.41×317 = 202.97 / 41 + 0.33×181 = 100.73
    expect(px.x).toBeCloseTo(73 + (4100 / GROUP_LOCAL_EXTENT) * 317, 10);
    expect(px.y).toBeCloseTo(41 + (3300 / GROUP_LOCAL_EXTENT) * 181, 10);
    expect(px.w).toBeCloseTo((1700 / GROUP_LOCAL_EXTENT) * 317, 10);
    expect(px.h).toBeCloseTo((900 / GROUP_LOCAL_EXTENT) * 181, 10);
    // 원점을 잊은 구현과 빼는 것을 잊은 구현이 함께 통과하지 못하게, 원점이 실제로
    // 더해졌음을 **값으로** 못박는다(두 축이 서로 다르다).
    expect(px.x).not.toBe(px.x - 73);
    expect(Math.round(px.x)).toBe(203);
    expect(Math.round(px.y)).toBe(101);
  });

  it('음수 원점에서도 원점이 더해진다 (E-D)', () => {
    const inner: BoxGeometry = { x: 5000, y: 5000, w: 100, h: 100 };
    const px = projectBoxIn(inner, NEG_BOX);
    expect(px.x).toBeCloseTo(-120 + 0.5 * 317, 10);
    expect(px.y).toBeCloseTo(-37 + 0.5 * 181, 10);
  });

  it('비균등 상자에서 부품이 같은 비율로 일그러진다 — 종횡비를 따로 지키지 않는다(A7)', () => {
    const square: BoxGeometry = { x: 0, y: 0, w: 5000, h: 5000 };
    const px = projectBoxIn(square, BOX);
    // 두 축의 축척이 다르므로 정사각 부품이 직사각형이 된다.
    expect(px.w).not.toBeCloseTo(px.h, 6);
    expect(px.w / px.h).toBeCloseTo(317 / 181, 10);
  });

  it('선은 끝점 둘 다 원점을 얻는다', () => {
    const line: LineGeometry = { x1: 1200, y1: 7400, x2: 8800, y2: 6100 };
    const px = projectLineIn(line, BOX);
    expect(px.x1).toBeCloseTo(73 + 0.12 * 317, 10);
    expect(px.y1).toBeCloseTo(41 + 0.74 * 181, 10);
    expect(px.x2).toBeCloseTo(73 + 0.88 * 317, 10);
    expect(px.y2).toBeCloseTo(41 + 0.61 * 181, 10);
  });

  it('점도 같다', () => {
    const p: PointGeometry = { x: 5000, y: 9200 };
    expect(projectPointIn(p, BOX)).toEqual({ x: 73 + 0.5 * 317, y: 41 + 0.92 * 181 });
  });

  it('`localProjection` 의 `canvas` 축은 **로컬 격자**다 — `proj.canvas` 가 아니다', () => {
    expect(localProjection(BOX)).toEqual({
      stage: { width: 317, height: 181 },
      canvas: GROUP_LOCAL_SIZE,
    });
  });
});

// --- ④ 방어 (0 · 음수 · 비유한) -------------------------------------------

describe('퇴화·손상 상자가 NaN 을 흘리지 않는다', () => {
  it('0 크기 상자는 모든 부품을 원점 한 점으로 모은다(유한하다)', () => {
    const px = projectBoxIn({ x: 5000, y: 5000, w: 100, h: 100 }, { x: 10, y: 20, w: 0, h: 0 });
    expect(px).toEqual({ x: 10, y: 20, w: 0, h: 0 });
    expect(Object.values(px).every(Number.isFinite)).toBe(true);
  });

  it('비유한 상자도 유한한 값을 낸다', () => {
    const px = projectBoxIn(
      { x: Number.NaN, y: 5000, w: 100, h: 100 },
      { x: Number.POSITIVE_INFINITY, y: 20, w: Number.NaN, h: 181 },
    );
    expect(Object.values(px).every(Number.isFinite)).toBe(true);
  });

  it('음수 크기 상자에서 **008 의 경로와 004 의 부품이 갈라진다** — 그 사실을 적어 둔다', () => {
    const neg: PxBox = { x: 50, y: 50, w: -200, h: -100 };
    const viaPath = projectPathPoints([{ c: 'M', x: 5000, y: 5000 }], neg)[0];
    const viaPart = projectPointIn({ x: 5000, y: 5000 }, neg);
    if (viaPath?.c !== 'M') throw new Error('M 명령이어야 한다');
    // 008 은 곱셈뿐이라 좌표가 **뒤집힌다**.
    expect(viaPath.x).toBe(50 + 5000 * (-200 / GROUP_LOCAL_EXTENT));
    // 004 는 `project` 의 `positiveOrZero(stage)` 관문을 지나 **원점으로 모인다**.
    expect(viaPart.x).toBe(50);
    // spec.md §방어는 "곱셈일 뿐이라 좌표가 뒤집히거나 한 점으로 모일 뿐" 이라고 적었으나,
    // 004 갈래에서는 뒤집힘이 일어나지 않는다 — `project` 가 음수 스테이지를 0 으로
    // 떨어뜨리기 때문이다. 그 관문은 001 의 것이라 004 가 고치지 않는다.
    expect(viaPath.x).not.toBe(viaPart.x);
  });
});

// --- ⑤ 기존 투영이 불변이다 (REQ-05) --------------------------------------

describe('001·002·008 의 투영은 한 글자도 바뀌지 않았다', () => {
  it('`projectBox` 는 종전 그대로 캔버스 크기로 나눈다', () => {
    const geo: BoxGeometry = { x: 50, y: 40, w: 100, h: 80 };
    const proj = { stage: { width: 400, height: 200 }, canvas: { width: 500, height: 400 } };
    expect(projectBox(geo, proj)).toEqual({ x: 40, y: 20, w: 80, h: 40 });
  });

  it('`projectPathPoints` 의 셋째 인자가 여전히 없다', () => {
    const decl = /export function projectPathPoints\(\s*path: readonly PathCommand\[\],\s*box: PxBox,?\s*\)/;
    expect(decl.test(source('canvasGeometry.ts'))).toBe(true);
  });
});
