// 경로의 편집 수학 (SPEC-CANVAS-008 M5).
//
// 이 파일이 겨누는 함정은 **D1** 이다. `BoxGeometry` 는 구조적으로 `PointGeometry` 에
// 대입되므로(가정 A6), 다섯 번째 종류는 **타입 오류 없이 문구로 읽힌다.** 그때의 증상은
// "손잡이가 하나뿐 · 엉뚱한 자리 · 끌어도 움직이지 않음" 인데, 그 셋 어느 것도 경로 요소가
// 없는 시험에서는 관측되지 않는다.
//
// M1 이 컴파일러의 침묵을 **실측했고**, M5 는 그 목록을 메운다. 여기서 재는 것은 그 가운데
// 순수 수학 셋이다:
//   (b) `handlesFor('path')` 가 **여덟**이다(`['font']` 가 아니다)
//   (b') `handlePositions` 가 그 여덟을 **상자 위**에 앉힌다(문구 기준점이 아니다)
//   (d) `patchNodeGeometry` 가 상자를 **실제로 쓴다**(요소를 그대로 돌려주지 않는다)
//
// 고정 입력이 기본값 모양이 아니다: 축척 **가로 1.6 · 세로 1.5**(단위도 아니고 두 축이
// 같지도 않다), 상자 `{x:37, y:61, w:160, h:90}`(원점 ≠ 0 · `x ≠ y` · 비정사각 — D2·D3).
//
// @spec SPEC-CANVAS-008 REQ-01 · REQ-02 · AC-03 · D1 · 불변식 J2

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import type { BoxGeometry, CanvasElement, PathElement } from './canvasConfig';
import { MIN_ELEMENT_EXTENT } from './canvasConfig';
import {
  BOX_HANDLE_IDS,
  TEXT_HANDLE_IDS,
  handlePositions,
  handlesFor,
  moveGeometry,
  patchNodeGeometry,
} from './canvasEditGeometry';
import { projectBox, type CanvasProjection } from './canvasGeometry';
import { DEFAULT_PATH, PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';

/** 축척 가로 1.6 · 세로 1.5. */
const PROJ: CanvasProjection = {
  stage: { width: 800, height: 600 },
  canvas: { width: 500, height: 400 },
};

const BOX: BoxGeometry = { x: 37, y: 61, w: 160, h: 90 };

/** 비대칭 삼각형 — 대칭 도형은 전치 결함을 감춘다. */
const TRIANGLE: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

function pathEl(overrides: Partial<PathElement> = {}): PathElement {
  return {
    id: 'p1',
    kind: 'path',
    geometry: { ...BOX },
    path: [...TRIANGLE],
    catalog_id: 'rightTriangle',
    style: {},
    ...overrides,
  };
}

// --- (b) 손잡이 집합 -----------------------------------------------------

describe('handlesFor — 경로는 상자 여덟 손잡이를 갖는다 (D1(b))', () => {
  it('여덟이다 — 글자 크기 손잡이 하나가 아니다', () => {
    // `default:` 로 떨어지면 `['font']` 하나가 나온다. 손잡이가 하나뿐인 도형은 "크기를
    // 바꿀 수 없는 도형" 이며, 그 증상은 화면에서만 드러난다.
    expect(handlesFor('path')).toHaveLength(8);
    expect(handlesFor('path')).not.toEqual(TEXT_HANDLE_IDS);
  });

  it('rect 와 **같은 배열**을 돌려준다 — 두 번째 목록을 만들지 않는다', () => {
    expect(handlesFor('path')).toBe(BOX_HANDLE_IDS);
    expect(handlesFor('path')).toBe(handlesFor('rect'));
  });
});

// --- (b') 손잡이 자리 ----------------------------------------------------

describe('handlePositions — 경로의 손잡이는 상자 위에 앉는다 (D1(b))', () => {
  const handles = handlePositions(pathEl(), PROJ);

  it('상자가 잰 그대로다 — 이 절이 딛는 수치를 먼저 고정한다', () => {
    expect(projectBox(BOX, PROJ)).toMatchObject({
      x: expect.closeTo(59.2, 12),
      y: 91.5,
      w: 256,
      h: 135,
    });
  });

  it('여덟 손잡이가 상자의 모서리와 변 한가운데에 선다', () => {
    // 문구로 읽히면 손잡이 하나가 기준점(59.2, 91.5) 근처에 홀로 선다. 아래 여덟 수는
    // 원점 항과 축척 항을 **둘 다** 쓴 값이며, 둘 중 하나를 빼면 전부 어긋난다.
    expect(handles.map((h) => h.id)).toEqual(['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']);
    const at = (id: string) => handles.find((h) => h.id === id)?.point;
    expect(at('nw')).toMatchObject({ x: expect.closeTo(59.2, 12), y: 91.5 });
    expect(at('ne')).toMatchObject({ x: expect.closeTo(315.2, 12), y: 91.5 });
    expect(at('se')).toMatchObject({ x: expect.closeTo(315.2, 12), y: 226.5 });
    expect(at('sw')).toMatchObject({ x: expect.closeTo(59.2, 12), y: 226.5 });
    // 변 한가운데 — 비정사각 상자라 x 중점과 y 중점이 서로 다른 수다(D2).
    expect(at('n')).toMatchObject({ x: expect.closeTo(187.2, 12), y: 91.5 });
    expect(at('w')).toMatchObject({ x: expect.closeTo(59.2, 12), y: 159 });
  });

  it('여덟 모두 **기하**를 쓴다 — 글자 크기를 쓰는 손잡이가 하나도 없다', () => {
    expect(handles.every((h) => h.role === 'geometry')).toBe(true);
  });

  it('명령 목록이 무엇이든 손잡이는 상자에서만 나온다 — 경로는 스테이지를 다시 재지 않는다', () => {
    const wild = handlePositions(
      pathEl({ path: [{ c: 'M', x: -90000, y: 90000 }] }),
      PROJ,
    );
    expect(wild.map((h) => h.point)).toEqual(handles.map((h) => h.point));
  });

  it('음수 크기 상자도 양수 범위로 펴서 앉힌다', () => {
    const flipped = handlePositions(
      pathEl({ geometry: { x: 197, y: 151, w: -160, h: -90 } }),
      PROJ,
    );
    expect(flipped.find((h) => h.id === 'nw')?.point).toMatchObject({
      x: expect.closeTo(59.2, 12),
      y: 91.5,
    });
  });
});

// --- (d) 기하 쓰기 -------------------------------------------------------

describe('patchNodeGeometry — 경로는 상자를 실제로 쓴다 (D1(d))', () => {
  it('상자를 주면 그 상자가 실린다 — 요소를 그대로 돌려주지 않는다', () => {
    // M1 이 남긴 `case 'path': return el` 이 살아 있으면 이 단언이 빨개진다. 그 상태의
    // 증상은 "경로가 영영 움직이지도 커지지도 않는다" 이며, 다른 어떤 시험도 그것을
    // 보지 못한다.
    const [out] = patchNodeGeometry([pathEl()], 'p1', { x: 10, y: 20, w: 30, h: 40 });
    expect(out?.geometry).toEqual({ x: 10, y: 20, w: 30, h: 40 });
  });

  it('rect 와 **같은 규율**로 쓴다 — 정수 반올림 · 최소 크기 · 음수 크기 정규화', () => {
    const [rounded] = patchNodeGeometry([pathEl()], 'p1', { x: 10.6, y: 20.4, w: 30.5, h: 40.5 });
    expect(rounded?.geometry).toEqual({ x: 11, y: 20, w: 31, h: 41 });

    const [tiny] = patchNodeGeometry([pathEl()], 'p1', { x: 0, y: 0, w: 0, h: 0 });
    expect(tiny?.geometry).toEqual({
      x: 0,
      y: 0,
      w: MIN_ELEMENT_EXTENT,
      h: MIN_ELEMENT_EXTENT,
    });

    const [negative] = patchNodeGeometry([pathEl()], 'p1', { x: 40, y: 30, w: -20, h: -10 });
    expect(negative?.geometry).toEqual({ x: 40, y: 30, w: 20, h: 10 });
  });

  it('**명령 목록과 출처는 그대로 살아남는다** — 기하 쓰기는 경로 자료를 건드리지 않는다', () => {
    const [out] = patchNodeGeometry([pathEl()], 'p1', { x: 10, y: 20, w: 30, h: 40 });
    expect(out?.kind).toBe('path');
    expect((out as PathElement).path).toEqual(TRIANGLE);
    expect((out as PathElement).catalog_id).toBe('rightTriangle');
  });

  it('형상이 어긋난 기하(선·점)는 요소를 **그대로 둔다**', () => {
    const el = pathEl();
    expect(patchNodeGeometry([el], 'p1', { x1: 0, y1: 0, x2: 5, y2: 5 })[0]).toBe(el);
    expect(patchNodeGeometry([el], 'p1', { x: 1, y: 2 })[0]).toBe(el);
  });

  it('무리 이동이 경로에도 걸린다 — `moveGeometry` 로 옮긴 상자가 그대로 실린다', () => {
    const moved = moveGeometry(pathEl().geometry, { dx: 13, dy: -7 });
    const [out] = patchNodeGeometry([pathEl()], 'p1', moved);
    expect(out?.geometry).toEqual({ x: 50, y: 54, w: 160, h: 90 });
  });

  it('씨앗 경로를 가진 요소도 같다 — 명령 목록의 내용은 이 통로와 무관하다', () => {
    const seed = pathEl({ path: [...DEFAULT_PATH] });
    const [out] = patchNodeGeometry([seed], 'p1', { x: 1, y: 2, w: 3, h: 4 });
    expect(out?.geometry).toEqual({ x: 1, y: 2, w: 3, h: 4 });
    expect((out as PathElement).path).toEqual(DEFAULT_PATH);
  });
});

// --- 불변식 J2 -----------------------------------------------------------

describe('불변식 J2 — 경로 명령은 기하 쓰기 통로를 지나지 않는다', () => {
  const source = readFileSync(join(__dirname, 'canvasEditGeometry.ts'), 'utf-8')
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
    .join('\n');

  it('그 모듈 안에 `path` 필드를 **대입하는** 자리가 없다', () => {
    // 이름이 아니라 **형상**으로 잰다: `path:` 로 시작하는 객체 리터럴 항목이 하나라도
    // 생기면 경로 명령이 기하 통로를 타기 시작한 것이다.
    expect(source).not.toMatch(/[{,]\s*path\s*:/);
  });

  it('요소 배열을 돌려주는 export 가 **하나뿐이다**', () => {
    // 둘이 되는 순간 "기하 쓰기의 유일한 통로" 라는 문장이 거짓이 된다.
    const signatures = [...source.matchAll(/export function (\w+)\([^)]*\):\s*([^{;\n]+)/g)];
    const elementArrayExports = signatures
      .filter(([, , ret]) => (ret ?? '').trim() === 'CanvasElement[]')
      .map(([, name]) => name);
    expect(elementArrayExports).toEqual(['patchNodeGeometry']);
  });

  it('그 가드가 무동작이 아니다 — 그 함수가 실제로 경로 갈래를 갖는다', () => {
    expect(source).toMatch(/case 'path':/);
  });
});

// --- 종류 목록의 총망라 --------------------------------------------------

describe('요소 종류를 다루는 자리가 다섯을 안다', () => {
  it('`handlesFor` 가 다섯 종류 전부에 답한다 — 답하지 못하는 종류가 없다', () => {
    const kinds = ['rect', 'ellipse', 'line', 'text', 'path'] as const;
    for (const kind of kinds) {
      expect(handlesFor(kind).length, kind).toBeGreaterThan(0);
    }
  });

  it('`handlePositions` 가 다섯 종류 전부에 손잡이를 낸다', () => {
    const els: CanvasElement[] = [
      { id: 'r', kind: 'rect', geometry: { ...BOX }, style: {} },
      { id: 'e', kind: 'ellipse', geometry: { ...BOX }, style: {} },
      { id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 10, y2: 10 }, style: {} },
      { id: 't', kind: 'text', geometry: { x: 5, y: 5 }, style: {} },
      pathEl(),
    ];
    for (const el of els) {
      expect(handlePositions(el, PROJ).length, el.kind).toBe(handlesFor(el.kind).length);
    }
  });
});
