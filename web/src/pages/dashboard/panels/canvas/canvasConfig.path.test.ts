// 경로 요소의 관용 파서 테스트 (SPEC-CANVAS-008 M2).
//
// **이 파일이 막는 실패는 크래시가 아니다.** 파서의 `kind` 화이트리스트에 `'path'` 가 없으면
// 저장된 경로 요소는 예외 하나 없이 **읽을 때 사라진다** — 사용자가 저술한 도형이 새로고침
// 한 번에 없어지고, 화면에는 아무 말도 나오지 않는다. 008 에서 가장 나쁜 실패이며(위험 R2),
// 아래 첫 시험이 그 유일한 가드다.
//
// 고정 입력은 **기본값 모양이 아니다**(시험 규율 D2·D3): 상자는 `x=37, y=61, w=160, h=90`
// 으로 원점이 0 이 아니고 `x ≠ y` 이며 정사각형이 아니다. 명령 목록은 **비대칭 도형**이고
// 네 명령(`M`·`L`·`C`·`Z`)을 모두 쓴다 — 대칭 도형이나 다각형만 쓰면 x·y 를 뒤바꾼 결함과
// `C` 인자 여섯의 순서 결함이 함께 숨는다.
//
// @spec SPEC-CANVAS-008 REQ-01 · AC-01 · AC-E4

import { describe, expect, it } from 'vitest';

import {
  parseCanvasConfig,
  type CanvasElement,
  type CanvasPanelConfig,
  type PathElement,
} from './canvasConfig';
import type { CanvasNode } from './group/groupTypes';
import { DEFAULT_PATH, MAX_PATH_COMMANDS, PATH_LOCAL_EXTENT } from './shapes/pathTypes';

/** 비대칭·네 명령 전부. 대칭 도형은 전치 결함을 감춘다(D2). */
const ASYMMETRIC_PATH = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 7000, y: PATH_LOCAL_EXTENT },
  { c: 'C', x1: 9100, y1: 8200, x2: 9700, y2: 5300, x: 8800, y: 1400 },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
] as const;

/** 기본값 모양이 아닌 상자 — 원점 ≠ 0 · `x ≠ y` · 비정사각(D2·D3). */
const BOX = { x: 37, y: 61, w: 160, h: 90 } as const;

function rawPathElement(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'el-7',
    kind: 'path',
    geometry: { ...BOX },
    path: ASYMMETRIC_PATH.map((c) => ({ ...c })),
    catalog_id: 'rightTriangle',
    style: { fill: '#3b82f6', stroke: '#1e40af', strokeWidth: 2 },
    ...over,
  };
}

function rawConfig(elements: readonly unknown[]): Record<string, unknown> {
  return { channel_name: '', canvas: { width: 500, height: 400 }, elements };
}

/**
 * 최상위 노드를 요소로 좁힌다(SPEC-CANVAS-004 M1 — `elements` 의 원소가 `CanvasNode` 로
 * 넓어졌다). 이 파일은 경로 요소만 넣으므로 좁히기는 타입을 맞추는 일 하나뿐이다.
 */
function asElement(node: CanvasNode | undefined): CanvasElement | undefined {
  return node === undefined || node.kind === 'group' ? undefined : node;
}

/** 파싱된 config 에서 경로 요소 하나를 꺼낸다. 없으면 시험이 그 자리에서 죽는다. */
function onlyPath(cfg: CanvasPanelConfig): PathElement {
  const el: CanvasElement | undefined = asElement(cfg.elements[0]);
  expect(el, '경로 요소가 파서를 지나 살아남아야 한다').toBeDefined();
  expect(el?.kind).toBe('path');
  return el as PathElement;
}

describe('parseCanvasConfig — 경로 요소가 저장 왕복을 지난다 (AC-01)', () => {
  it('읽고 · JSON 왕복하고 · 다시 읽어도 명령 목록이 한 값도 바뀌지 않는다', () => {
    const once = parseCanvasConfig(rawConfig([rawPathElement()]));
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify(once)));

    const el = onlyPath(twice);
    expect(el.geometry).toEqual(BOX);
    expect(el.path).toEqual(ASYMMETRIC_PATH.map((c) => ({ ...c })));
    expect(el.catalog_id).toBe('rightTriangle');
    // 왕복 전후가 통째로 같아야 한다 — 필드 하나가 조용히 사라지는 것을 잡는다.
    expect(twice).toEqual(once);
  });

  it('`C` 의 인자 여섯이 순서 그대로다 — 제어점 둘과 끝점이 뒤섞이지 않는다', () => {
    const el = onlyPath(parseCanvasConfig(rawConfig([rawPathElement()])));
    expect(el.path[2]).toEqual({
      c: 'C',
      x1: 9100,
      y1: 8200,
      x2: 9700,
      y2: 5300,
      x: 8800,
      y: 1400,
    });
  });

  it('경로 요소가 없는 config(001~006 이 쓴 것)는 008 이전과 같이 읽힌다', () => {
    const legacy = rawConfig([
      { id: 'a', kind: 'rect', geometry: { x: 10, y: 20, w: 30, h: 40 } },
      { id: 'b', kind: 'text', geometry: { x: 5, y: 6 }, text: 'hi' },
    ]);
    const cfg = parseCanvasConfig(legacy);
    expect(cfg.elements.map((e) => e.kind)).toEqual(['rect', 'text']);
    expect(cfg.elements.some((e) => 'path' in e)).toBe(false);
  });
});

describe('parseCanvasConfig — 손상 경로는 요소를 죽이지 않는다 (AC-E4)', () => {
  /** 씨앗 경로의 **사본**인가 — 같은 값이되 같은 객체는 아니어야 한다. */
  function expectSeed(el: PathElement): void {
    expect(el.path).toEqual(DEFAULT_PATH.map((c) => ({ ...c })));
    expect(el.path).not.toBe(DEFAULT_PATH);
    el.path.forEach((cmd, i) => expect(cmd).not.toBe(DEFAULT_PATH[i]));
  }

  it('`path` 가 배열이 아니면 씨앗 경로다', () => {
    for (const bad of [undefined, null, 'M 0 0', 42, {}]) {
      expectSeed(onlyPath(parseCanvasConfig(rawConfig([rawPathElement({ path: bad })]))));
    }
  });

  it('유효 명령이 하나도 남지 않으면 씨앗 경로다', () => {
    const el = onlyPath(
      parseCanvasConfig(rawConfig([rawPathElement({ path: [{ c: 'Q' }, 7, null] })])),
    );
    expectSeed(el);
  });

  it('첫 유효 명령이 `M` 이 아니면 씨앗 경로다 — `M(0,0)` 을 지어내지 않는다', () => {
    const el = onlyPath(
      parseCanvasConfig(
        rawConfig([
          rawPathElement({
            path: [
              { c: 'L', x: 100, y: 200 },
              { c: 'L', x: 300, y: 400 },
              { c: 'Z' },
            ],
          }),
        ]),
      ),
    );
    // 반쯤 성립하는 경로를 지어내면 화면에 정체 모를 형상이 나온다.
    expectSeed(el);
    expect(el.path.some((c) => c.c === 'L' && c.x === 100)).toBe(false);
  });

  it('좌표가 유한하지 않은 명령은 **그 명령만** 버린다', () => {
    const el = onlyPath(
      parseCanvasConfig(
        rawConfig([
          rawPathElement({
            path: [
              { c: 'M', x: 10, y: 20 },
              { c: 'L', x: Number.NaN, y: 40 },
              { c: 'C', x1: 1, y1: 2, x2: 3, y2: 4, x: 5, y: Number.POSITIVE_INFINITY },
              { c: 'L', x: 50, y: 60 },
            ],
          }),
        ]),
      ),
    );
    expect(el.path).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 50, y: 60 },
    ]);
  });

  it('모르는 명령 문자는 **그 명령만** 버린다', () => {
    const el = onlyPath(
      parseCanvasConfig(
        rawConfig([
          rawPathElement({
            path: [
              { c: 'M', x: 1, y: 2 },
              { c: 'Q', x: 3, y: 4 },
              { c: 'A', rx: 5 },
              { c: 'L', x: 6, y: 7 },
            ],
          }),
        ]),
      ),
    );
    expect(el.path).toEqual([
      { c: 'M', x: 1, y: 2 },
      { c: 'L', x: 6, y: 7 },
    ]);
  });

  it('좌표는 `coordinate()` 의 규율대로 정수로 반올림된다 — 두 번째 수치 규율이 없다', () => {
    const el = onlyPath(
      parseCanvasConfig(
        rawConfig([
          rawPathElement({
            path: [
              { c: 'M', x: 10.4, y: 20.5 },
              { c: 'C', x1: 1.5, y1: -2.5, x2: 3.49, y2: 4.51, x: -5.5, y: 6.6 },
            ],
          }),
        ]),
      ),
    );
    // Math.round 의 규율 그대로다(-2.5 → -2, -5.5 → -5). 다른 반올림을 쓰면 여기서 갈린다.
    expect(el.path).toEqual([
      { c: 'M', x: 10, y: 21 },
      { c: 'C', x1: 2, y1: -2, x2: 3, y2: 5, x: -5, y: 7 },
    ]);
  });

  it('로컬 좌표를 0..PATH_LOCAL_EXTENT 로 clamp 하지 않는다 — 말풍선 꼬리가 산다', () => {
    const el = onlyPath(
      parseCanvasConfig(
        rawConfig([
          rawPathElement({
            path: [
              { c: 'M', x: -4200, y: 13500 },
              { c: 'L', x: PATH_LOCAL_EXTENT + 900, y: -70 },
            ],
          }),
        ]),
      ),
    );
    expect(el.path).toEqual([
      { c: 'M', x: -4200, y: 13500 },
      { c: 'L', x: PATH_LOCAL_EXTENT + 900, y: -70 },
    ]);
  });

  it('명령이 상한을 넘으면 **앞에서부터 상한까지만** 살린다 — 전부 버리지 않는다', () => {
    const many: unknown[] = [{ c: 'M', x: 0, y: 0 }];
    for (let i = 1; i < MAX_PATH_COMMANDS + 40; i += 1) many.push({ c: 'L', x: i, y: i * 2 });
    const el = onlyPath(parseCanvasConfig(rawConfig([rawPathElement({ path: many })])));
    expect(el.path).toHaveLength(MAX_PATH_COMMANDS);
    expect(el.path[0]).toEqual({ c: 'M', x: 0, y: 0 });
    expect(el.path[MAX_PATH_COMMANDS - 1]).toEqual({
      c: 'L',
      x: MAX_PATH_COMMANDS - 1,
      y: (MAX_PATH_COMMANDS - 1) * 2,
    });
  });

  it('기하가 손상되어도 요소는 살아 있고 씨앗 상자로 대체된다(001 의 정책 그대로)', () => {
    const el = onlyPath(
      parseCanvasConfig(rawConfig([rawPathElement({ geometry: { x: 1, y: 2, w: 0, h: 0 } })])),
    );
    expect(el.geometry.w).toBeGreaterThan(0);
    expect(el.geometry.h).toBeGreaterThan(0);
    expect(el.path).toHaveLength(ASYMMETRIC_PATH.length);
  });

  it('요소를 버리는 경우는 id 결측 하나뿐이다', () => {
    const cfg = parseCanvasConfig(rawConfig([rawPathElement({ id: '   ' })]));
    expect(cfg.elements).toHaveLength(0);
  });
});

describe('parseCanvasConfig — catalog_id 는 출처 기록일 뿐이다', () => {
  it('빈 문자열·비문자열은 미지정으로 떨어진다', () => {
    for (const bad of [undefined, null, '', '   ', 42, {}]) {
      const el = onlyPath(parseCanvasConfig(rawConfig([rawPathElement({ catalog_id: bad })])));
      expect(el.catalog_id).toBeUndefined();
      // 출처가 없어도 명령 목록은 온전하다 — 렌더가 그 값을 읽지 않기 때문이다.
      expect(el.path).toHaveLength(ASYMMETRIC_PATH.length);
    }
  });

  it('카탈로그에 없는 id 여도 그대로 보존한다 — 결손 참조라는 실패 모드를 만들지 않는다', () => {
    const el = onlyPath(
      parseCanvasConfig(rawConfig([rawPathElement({ catalog_id: 'notInAnyCatalog' })])),
    );
    expect(el.catalog_id).toBe('notInAnyCatalog');
  });
});
