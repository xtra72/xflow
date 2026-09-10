// 경로 로컬 → 스테이지 px 투영 테스트 (SPEC-CANVAS-008 M3).
//
// 여기서 재는 것은 **한 함수와 그 함수가 유일하다는 사실** 둘이다. `projectPathPoints` 는
// 렌더와 히트가 함께 지나는 자리이고, 그래서 `PATH_LOCAL_EXTENT` 로 나누는 산술이 두 곳에
// 생기면 두 소비자가 서로 다른 격자를 보게 된다(불변식 J3). 그 어긋남은 타입에도 시험에도
// 걸리지 않고 화면에서만 드러나므로, 나눗셈이 사는 자리를 **형상으로** 잰다.
//
// 고정 입력이 기본값 모양이 아니다:
//   - 축척 **0.5**(단위 축척이 아니다) — 항등 나눗셈이 결함을 감추지 못한다
//   - 상자 `{x:37, y:61, w:160, h:90}` — 원점 ≠ 0 · `x ≠ y` · **비정사각**(D2·D3).
//     정사각형이면 x·y 를 뒤바꾼 결함이 보이지 않고, 원점이 0 이면 원점 항을 빼먹은 결함이
//     보이지 않으며, `x === y` 면 x 원점을 y 에 쓰는 결함이 통과한다
//   - 나누어떨어지지 않는 스테이지(333×197)를 한 벌 더 쓴다 — 자투리가 없으면 시험이
//     실패할 수 없다
//   - **비대칭 명령 목록** — 대칭 도형만으로는 전치가 여전히 보이지 않는다
//
// DOM 무의존이라 jsdom 없이 잰다.
//
// @spec SPEC-CANVAS-008 REQ-01 · AC-02 · 불변식 J3

import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import type { PathElement } from './canvasConfig';
import {
  labelAnchor,
  projectBox,
  projectPathPoints,
  type CanvasProjection,
  type PxBox,
} from './canvasGeometry';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';

/** 축척 0.5 — 단위 축척이 아니다. */
const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { width: 500, height: 400 },
};

/** 나누어떨어지지 않는 스테이지. 자투리가 있어야 반올림을 몰래 넣은 구현이 걸린다. */
const ODD_PROJ: CanvasProjection = {
  stage: { width: 333, height: 197 },
  canvas: { width: 500, height: 400 },
};

const BOX = { x: 37, y: 61, w: 160, h: 90 } as const;

/** `{x:18.5, y:30.5, w:80, h:45}` — 비정사각이고 원점이 0 이 아니다. */
const PX_BOX: PxBox = projectBox(BOX, PROJ);

function pathElement(path: readonly PathCommand[]): PathElement {
  return { id: 'p1', kind: 'path', geometry: { ...BOX }, path: [...path], style: {} };
}

describe('projectPathPoints — 로컬 격자를 상자 안으로 편다', () => {
  it('상자가 잰 그대로다 — 이 시험이 딛는 수치를 먼저 못박는다', () => {
    expect(PX_BOX).toEqual({ x: 18.5, y: 30.5, w: 80, h: 45 });
  });

  it('네 모서리가 상자의 네 모서리에 앉는다 — 원점 항과 축척 항을 **둘 다** 쓴다', () => {
    const out = projectPathPoints(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: PATH_LOCAL_EXTENT, y: 0 },
        { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
        { c: 'L', x: 0, y: PATH_LOCAL_EXTENT },
      ],
      PX_BOX,
    );
    // 원점 항을 빼면 (0,0)·(80,0)·… 이 나오고, 축척 항을 빼면 네 점이 모두 상자 좌상단에
    // 겹친다. 둘 다 여기서 갈린다.
    expect(out).toEqual([
      { c: 'M', x: 18.5, y: 30.5 },
      { c: 'L', x: 98.5, y: 30.5 },
      { c: 'L', x: 98.5, y: 75.5 },
      { c: 'L', x: 18.5, y: 75.5 },
    ]);
  });

  it('x·y 를 뒤바꾸면 갈린다 — 비정사각 상자 + 비대칭 점 하나', () => {
    // 전치 구현은 (74.5, 41.75) 대신 (38.5, 86.75) 를 낸다.
    expect(projectPathPoints([{ c: 'L', x: 7000, y: 2500 }], PX_BOX)).toEqual([
      { c: 'L', x: 74.5, y: 41.75 },
    ]);
  });

  it('`C` 의 여섯 좌표가 각각 제 축으로 투영된다', () => {
    const out = projectPathPoints(
      [{ c: 'C', x1: 7000, y1: 2500, x2: 3333, y2: 7777, x: PATH_LOCAL_EXTENT, y: 0 }],
      PX_BOX,
    );
    expect(out).toEqual([
      { c: 'C', x1: 74.5, y1: 41.75, x2: 45.164, y2: 65.4965, x: 98.5, y: 30.5 },
    ]);
  });

  it('`Z` 는 좌표를 얻지 않고 그대로 지나간다', () => {
    expect(projectPathPoints([{ c: 'Z' }], PX_BOX)).toEqual([{ c: 'Z' }]);
  });

  it('나누어떨어지지 않는 스테이지에서도 반올림하지 않는다', () => {
    const box = projectBox(BOX, ODD_PROJ);
    const [cmd] = projectPathPoints([{ c: 'M', x: 3333, y: 7777 }], box);
    expect(cmd?.c).toBe('M');
    // 정수로 죄는 구현이면 (60, 65) 가 나온다.
    expect(cmd).toMatchObject({ x: expect.closeTo(60.158448, 9), y: expect.closeTo(64.5140525, 9) });
  });

  it('음수 크기 상자는 좌표가 뒤집힐 뿐 NaN 이 되지 않는다', () => {
    const flipped: PxBox = { x: 100, y: 80, w: -80, h: -45 };
    expect(projectPathPoints([{ c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT }], flipped)).toEqual([
      { c: 'L', x: 20, y: 35 },
    ]);
  });

  it('격자 밖 로컬 좌표를 clamp 하지 않는다 — 말풍선 꼬리가 상자 밖에 산다', () => {
    expect(
      projectPathPoints([{ c: 'L', x: -PATH_LOCAL_EXTENT, y: 2 * PATH_LOCAL_EXTENT }], PX_BOX),
    ).toEqual([{ c: 'L', x: -61.5, y: 120.5 }]);
  });

  it('비유한 좌표는 0 으로 접힌다 — NaN 이 경로 전체를 지우지 않는다', () => {
    expect(projectPathPoints([{ c: 'L', x: Number.NaN, y: 0 }], PX_BOX)).toEqual([
      { c: 'L', x: 18.5, y: 30.5 },
    ]);
  });

  it('빈 목록은 빈 목록이다', () => {
    expect(projectPathPoints([], PX_BOX)).toEqual([]);
  });
});

describe('projectPathPoints — 시그니처의 최소성 (시험 규율 D9)', () => {
  it('매개변수가 **정확히 둘**이고 기본값이 하나도 없다', () => {
    // `Function.length` 로 세지 않는다 — 그 값은 **첫 기본값 앞까지만** 세므로, 기본값을
    // 단 셋째 인자가 들어와도 여전히 2 를 돌려준다. 기본값 있는 셋째 인자가 들어오면
    // 경로 투영이 표시 상태를 보게 되고, 그것이 이 가드가 막는 바로 그 결함이다.
    const source = readFileSync(join(__dirname, 'canvasGeometry.ts'), 'utf-8');
    const match = /export function projectPathPoints\(([^)]*)\)/.exec(source);
    expect(match, 'projectPathPoints 의 선언을 찾지 못했다').not.toBeNull();
    const params = (match?.[1] ?? '')
      .split(',')
      .map((p) => p.trim())
      .filter((p) => p !== '');
    expect(params).toHaveLength(2);
    expect(params.some((p) => p.includes('='))).toBe(false);
  });
});

describe('PATH_LOCAL_EXTENT 로 나누는 자리는 하나뿐이다 (불변식 J3)', () => {
  it('로컬 격자로 나누는 코드가 canvasGeometry.ts 밖에 없다', () => {
    const dir = __dirname;
    const offenders = readdirSync(dir, { recursive: true, encoding: 'utf-8' })
      .filter((name) => /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name))
      .filter((name) => name !== 'canvasGeometry.ts')
      .filter((name) => {
        const code = readFileSync(join(dir, name), 'utf-8')
          .split('\n')
          .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
          .join('\n');
        return /\/\s*PATH_LOCAL_EXTENT/.test(code);
      });
    expect(offenders).toEqual([]);
  });

  it('그 가드가 무동작이 아니다 — canvasGeometry.ts 는 실제로 그 나눗셈을 갖는다', () => {
    const code = readFileSync(join(__dirname, 'canvasGeometry.ts'), 'utf-8')
      .split('\n')
      .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
      .join('\n');
    expect(code).toMatch(/\/\s*PATH_LOCAL_EXTENT/);
  });
});

describe('labelAnchor — 경로의 라벨은 상자의 **중심**이다', () => {
  it('좌상단이 아니라 중심을 돌려준다', () => {
    // `default:` 로 떨어지면 (18.5, 30.5) — 상자 기하가 구조적으로 문구 기준점에
    // 대입되므로 컴파일러는 그 실수를 잡지 못한다(가정 A6). 이 수가 그 유일한 가드다.
    expect(labelAnchor(pathElement([{ c: 'M', x: 0, y: 0 }]), PROJ)).toEqual({ x: 58.5, y: 53 });
  });

  it('명령 목록이 무엇이든 중심은 상자에서만 나온다 — 경로는 스테이지를 다시 재지 않는다', () => {
    const wild = pathElement([
      { c: 'M', x: -50000, y: 90000 },
      { c: 'L', x: 123, y: 456 },
    ]);
    expect(labelAnchor(wild, PROJ)).toEqual({ x: 58.5, y: 53 });
  });
});
