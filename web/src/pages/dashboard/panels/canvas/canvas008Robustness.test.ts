// 008 의 견고성과 형상 가드 (SPEC-CANVAS-008 M12 · REQ-05 · REQ-07 · AC-E5 · 불변식 J10 · J12).
//
// **여기서 재는 것은 "이상한 값이 들어와도 무너지지 않는가" 다.** 008 의 시험 대부분은
// 카탈로그가 발행한 **멀쩡한** 명령 목록을 지난다 — 30종은 손으로 적었고 좌표는 전부
// 0..10000 안의 정수다. 그 고정 입력만으로는 다음 넷이 전부 보이지 않는다.
//
//   (1) **저장 자료는 사용자가 손으로 고칠 수 있다.** config 도 `localStorage` 도 텍스트다.
//   (2) **크기 조절은 상자를 0 으로 만들 수 있다.** 파서가 `MIN_ELEMENT_EXTENT` 로 죄는 것은
//       **읽는 자리**이고, 편집 중의 상자는 그 함수를 지나지 않은 채 투영에 닿는다.
//   (3) **003(애니메이션)의 보간 중간값**은 파서를 지나지 않는다.
//   (4) 그리고 **NaN 은 던지지 않는다** — 조용히 퍼져 화면에서 도형 하나가 사라질 뿐이다.
//       예외는 시험이 잡지만 NaN 은 아무도 잡지 않으므로, 여기서는 **수를 하나하나 센다.**
//
// **멈추지 않는 것도 함께 잰다.** SPEC-CANVAS-006 이 배달한 결함 하나는 빨개지는 대신
// **매달렸다**(`derivedCanvasSize` 가 받은 객체를 그대로 돌려주어 참조 비교가 쓰기를 삼킨
// 자리). 그 부류는 초록인 시험 전량에게 보이지 않는다. 그래서 평탄화처럼 **재귀로 도는**
// 자리는 결과의 점 개수에 상한을 두고 잰다 — 상한을 넘으면 매달린 것이고, 매달린 것은
// 시험 시간 초과로 드러나기 전에 이 단언이 먼저 잡는다.
//
// 이 파일은 **DOM 을 만지지 않는다**(`canvasHitTest` · `scratchpadTypes` 와 같은 규율).
// 렌더 층은 기록 스텁으로 대신한다.
//
// @spec SPEC-CANVAS-008 REQ-05 · REQ-07 · AC-E4 · AC-E5 · J3 · J10 · J12

import fs from 'node:fs';
import path from 'node:path';

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import {
  actualCanvasIdentifiers,
  actualCanvasImports,
  allowedCanvasIdentifiers,
  allowedCanvasImports,
} from '@/test/panelSettingsDialogCanvasSurface';

import {
  DEFAULT_CANVAS_SIZE,
  parseCanvasConfig,
  type CanvasElement,
  type PathElement,
} from './canvasConfig';
import {
  projectBox,
  projectPathPoints,
  type CanvasProjection,
  type PxBox,
} from './canvasGeometry';
import { hitTest } from './canvasHitTest';
import { drawElements, type DrawContext2D } from './drawElement';
import { flattenPath, isInsidePath, FLATTEN_TOLERANCE_PX } from './shapes/pathFlatten';
import {
  DEFAULT_COLLAPSED,
  PALETTE_GROUP_IDS,
  PALETTE_STORAGE_KEY,
  readCollapsed,
  type PaletteCollapseState,
} from './shapes/paletteGroups';
import { SHAPE_CATALOG } from './shapes/shapeCatalog';
import {
  DEFAULT_PATH,
  MAX_PATH_COMMANDS,
  PATH_LOCAL_EXTENT,
  type PathCommand,
} from './shapes/pathTypes';

beforeEach(() => {
  globalThis.localStorage.clear();
});

afterEach(() => {
  globalThis.localStorage.clear();
});

// --- 고정 입력 -----------------------------------------------------------

/** 축척 0.5 · 비정사각(시험 규율 D2). 나누어떨어지지 않는 축이 하나 있다. */
const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { width: 500, height: 400 },
};

/** 원점 ≠ 0 · `x ≠ y` · 비정사각(D2 · D3). */
const BOX = { x: 37, y: 61, w: 160, h: 90 } as const;

/** 기록 스텁 — 인자를 그대로 남긴다. 여기서 재는 것은 **그린 수**다. */
function recorder() {
  const calls: Array<[string, ...unknown[]]> = [];
  const handler: ProxyHandler<Record<string, unknown>> = {
    get(_target, prop: string) {
      if (prop === '__calls') return calls;
      // 스타일 속성은 대입만 되고 읽히지 않는다.
      return (...args: unknown[]) => {
        calls.push([prop, ...args]);
        return prop === 'measureText' ? { width: 0 } : undefined;
      };
    },
    set() {
      return true;
    },
  };
  const ctx = new Proxy({} as Record<string, unknown>, handler);
  return { ctx: ctx as unknown as DrawContext2D, calls };
}

/** 그린 명령의 인자 가운데 수인 것 전부. NaN 을 세는 자리다. */
function drawnNumbers(calls: ReadonlyArray<[string, ...unknown[]]>): number[] {
  return calls.flatMap(([, ...args]) => args.filter((a): a is number => typeof a === 'number'));
}

function pathEl(over: Partial<PathElement> = {}): PathElement {
  return {
    id: 'p1',
    kind: 'path',
    geometry: { ...BOX },
    path: [...DEFAULT_PATH],
    style: { fill: '#123456', stroke: '#654321', strokeWidth: 2 },
    ...over,
  };
}

// --- 망가진 명령 목록 -----------------------------------------------------

/**
 * 각 이름이 곧 그 고정 입력이 감추는 것이다. **파서를 지나지 않은 값**들이라, 이 목록은
 * 편집 중 상태 · 003 의 보간 중간값 · 손으로 고친 저장 자료를 함께 대신한다.
 */
const DEGENERATE_PATHS: ReadonlyArray<readonly [name: string, cmds: readonly PathCommand[]]> = [
  ['빈 명령 목록', []],
  ['점 하나뿐', [{ c: 'M', x: 5000, y: 5000 }]],
  ['닫기만 있다', [{ c: 'Z' }]],
  ['M 없이 시작한다', [{ c: 'L', x: 100, y: 200 }, { c: 'L', x: 300, y: 400 }, { c: 'Z' }]],
  [
    // `L` 로 시작하는 경우와 **다른 갈래**다: 곡선의 원점은 끝점이 아니라 **첫 제어점**
    // 이어야 한다(끝점을 쓰면 곡선이 한 점으로 접혀 보이지 않는다 — `pathFlatten` §C).
    'M 없이 **곡선**으로 시작한다',
    [
      { c: 'C', x1: 1000, y1: 2000, x2: 8000, y2: 9000, x: PATH_LOCAL_EXTENT, y: 0 },
      { c: 'Z' },
    ],
  ],
  ['닫기가 연달아 온다', [{ c: 'M', x: 0, y: 0 }, { c: 'Z' }, { c: 'Z' }, { c: 'Z' }]],
  [
    '좌표가 전부 비유한',
    [
      { c: 'M', x: Number.NaN, y: Number.NaN },
      { c: 'L', x: Number.POSITIVE_INFINITY, y: Number.NEGATIVE_INFINITY },
      {
        c: 'C',
        x1: Number.NaN,
        y1: Number.NaN,
        x2: Number.NaN,
        y2: Number.NaN,
        x: Number.NaN,
        y: Number.NaN,
      },
      { c: 'Z' },
    ],
  ],
  [
    '제어점이 천문학적인 곡선',
    [
      { c: 'M', x: 0, y: 0 },
      { c: 'C', x1: 1e300, y1: -1e300, x2: 1e300, y2: 1e300, x: PATH_LOCAL_EXTENT, y: 0 },
      { c: 'Z' },
    ],
  ],
  [
    // **재귀를 실제로 몰아붙이는 자리.** 위 `1e300` 은 현 거리 계산이 넘쳐 `NaN` 이 되고
    // `!(flatness > tol)` 이 그것을 평평한 것으로 읽어 **즉시** 끝난다(실측: 점 3개) —
    // 즉 그 고정 입력은 깊이 상한을 한 번도 건드리지 않는다. 1e9 은 넘치지 않으면서
    // 평탄도가 tol 의 1e7 배라, 상한이 없으면 2^24 번 쪼개진다.
    '제어점이 크지만 넘치지는 않는 곡선(재귀 상한을 실제로 친다)',
    [
      { c: 'M', x: 0, y: 0 },
      { c: 'C', x1: 1e9, y1: -1e9, x2: 1e9, y2: 1e9, x: PATH_LOCAL_EXTENT, y: 0 },
      { c: 'Z' },
    ],
  ],
  [
    '제어점이 겹친 곡선(길이 0)',
    [
      { c: 'M', x: 5000, y: 5000 },
      { c: 'C', x1: 5000, y1: 5000, x2: 5000, y2: 5000, x: 5000, y: 5000 },
      { c: 'Z' },
    ],
  ],
  ['상한 길이의 목록', Array.from({ length: MAX_PATH_COMMANDS }, (_u, i) =>
    i === 0
      ? ({ c: 'M', x: 0, y: 0 } as PathCommand)
      : ({ c: 'L', x: (i * 37) % PATH_LOCAL_EXTENT, y: (i * 91) % PATH_LOCAL_EXTENT } as PathCommand),
  )],
];

/** 상자 쪽의 망가짐. 파서의 `MIN_ELEMENT_EXTENT` 를 지나지 않은 값들이다. */
const DEGENERATE_BOXES: ReadonlyArray<readonly [name: string, box: PxBox]> = [
  ['크기 0 상자', { x: 37, y: 61, w: 0, h: 0 }],
  ['한 축만 0', { x: 37, y: 61, w: 160, h: 0 }],
  ['극단적인 종횡비', { x: 37, y: 61, w: 1, h: 100000 }],
  ['음수 크기', { x: 37, y: 61, w: -160, h: -90 }],
  ['비유한 원점', { x: Number.NaN, y: 61, w: 160, h: 90 }],
];

/**
 * 평탄화 결과 점 개수의 상한.
 *
 * 이 수를 두는 것이 **매달림을 잡는 자리**다. 재귀 이등분은 깊이 상한
 * (`MAX_SUBDIVISION_DEPTH` = 10)을 갖지만, 그 상한이 명령 수를 곱하면
 * 256 × 2^10 ≈ 26 만이다. 실제로는 그 근처에도 가지 않으므로 넉넉히 잡되 **유한하게**
 * 잡는다 — 무한히 도는 구현은 시간 초과 이전에 여기서 잡힌다.
 */
const MAX_FLAT_POINTS = 400_000;

describe('망가진 명령 목록이 파이프라인을 지나도 NaN 도 예외도 없다 (REQ-07 · AC-E4)', () => {
  for (const [name, cmds] of DEGENERATE_PATHS) {
    it(`투영 — ${name}`, () => {
      const px = projectPathPoints(cmds, { ...BOX });
      expect(px).toHaveLength(cmds.length);
      for (const cmd of px) {
        for (const [k, v] of Object.entries(cmd)) {
          if (k === 'c') continue;
          expect(Number.isFinite(v as number), `${name}: ${k}`).toBe(true);
        }
      }
    });

    it(`평탄화 — ${name}`, () => {
      const subs = flattenPath(projectPathPoints(cmds, { ...BOX }), FLATTEN_TOLERANCE_PX);
      const total = subs.reduce((n, s) => n + s.points.length, 0);
      // 매달리지 않았다 — 유한한 점을 유한한 시간에 냈다.
      expect(total).toBeLessThan(MAX_FLAT_POINTS);
      for (const sub of subs) {
        for (const p of sub.points) {
          expect(Number.isFinite(p.x), name).toBe(true);
          expect(Number.isFinite(p.y), name).toBe(true);
        }
      }
    });

    it(`내부 판정 — ${name} 은 참·거짓 하나로 답한다`, () => {
      const subs = flattenPath(projectPathPoints(cmds, { ...BOX }), FLATTEN_TOLERANCE_PX);
      expect(typeof isInsidePath(subs, { x: 100, y: 100 })).toBe('boolean');
    });

    it(`히트 — ${name} 은 예외 없이 판정된다`, () => {
      const el = pathEl({ path: [...cmds] });
      expect(() => hitTest([el], { x: 80, y: 70 }, PROJ, {})).not.toThrow();
    });

    it(`렌더 — ${name} 을 그린 인자에 NaN 이 하나도 없다`, () => {
      const { ctx, calls } = recorder();
      drawElements(ctx, [pathEl({ path: [...cmds] })], {}, {}, PROJ);
      const nums = drawnNumbers(calls);
      expect(nums.filter((n) => Number.isNaN(n)), name).toEqual([]);
    });
  }
});

describe('재귀 이등분은 **상한에서 멈춘다** — 매달리지 않는다 (REQ-07 · 위험 R3)', () => {
  /** 재귀 상한이 실제로 잘라 냄을 값으로 잰다. `2^깊이 + 1` 이 그 자국이다. */
  const WILD: readonly PathCommand[] = [
    { c: 'M', x: 0, y: 0 },
    { c: 'C', x1: 1e9, y1: -1e9, x2: 1e9, y2: 1e9, x: PATH_LOCAL_EXTENT, y: 0 },
    { c: 'Z' },
  ];

  it('평탄도가 허용 오차의 1천만 배여도 점이 **1,025개**에서 멎는다 (2^10 + 1)', () => {
    const subs = flattenPath(projectPathPoints(WILD, { ...BOX }), FLATTEN_TOLERANCE_PX);
    const total = subs.reduce((n, s) => n + s.points.length, 0);
    // `MAX_SUBDIVISION_DEPTH`(=10)가 잘라 낸 자국이다. 이 수가 커지면 상한이 풀린 것이고,
    // 그때 이 자리는 빨개지는 대신 **매달린다** — 006 이 배달한 결함과 같은 부류다.
    expect(total).toBe(1025);
  });

  it('넘치는 제어점(1e300)은 상한에 닿기 전에 `NaN` 규칙이 먼저 끝낸다 — 다른 가드다', () => {
    const wilder: readonly PathCommand[] = [
      { c: 'M', x: 0, y: 0 },
      { c: 'C', x1: 1e300, y1: -1e300, x2: 1e300, y2: 1e300, x: PATH_LOCAL_EXTENT, y: 0 },
      { c: 'Z' },
    ];
    const subs = flattenPath(projectPathPoints(wilder, { ...BOX }), FLATTEN_TOLERANCE_PX);
    // 점 3개 = 쪼개지 않았다. 두 가드가 **서로 다른 입력**을 맡는다는 사실을 적어 둔다 —
    // 이것을 모르면 1e300 하나로 재귀 상한까지 지켰다고 착각한다.
    expect(subs.reduce((n, s) => n + s.points.length, 0)).toBe(3);
  });
});

describe('망가진 상자가 파이프라인을 지나도 NaN 도 예외도 없다 (REQ-07)', () => {
  for (const [name, box] of DEGENERATE_BOXES) {
    it(`투영 — ${name}`, () => {
      const px = projectPathPoints(DEFAULT_PATH, box);
      for (const cmd of px) {
        for (const [k, v] of Object.entries(cmd)) {
          if (k === 'c') continue;
          expect(Number.isFinite(v as number), `${name}: ${k}`).toBe(true);
        }
      }
    });

    it(`평탄화와 판정 — ${name}`, () => {
      const subs = flattenPath(projectPathPoints(DEFAULT_PATH, box), FLATTEN_TOLERANCE_PX);
      expect(subs.reduce((n, s) => n + s.points.length, 0)).toBeLessThan(MAX_FLAT_POINTS);
      expect(typeof isInsidePath(subs, { x: 50, y: 70 })).toBe('boolean');
    });

    it(`렌더 — ${name} 을 그린 인자에 NaN 이 하나도 없다`, () => {
      const { ctx, calls } = recorder();
      drawElements(ctx, [pathEl({ geometry: { ...box } })], {}, {}, PROJ);
      expect(drawnNumbers(calls).filter((n) => Number.isNaN(n)), name).toEqual([]);
    });
  }

  it('크기 0 상자는 로컬 격자를 한 점으로 접는다 — 그 점이 상자의 원점이다', () => {
    // 나눗셈이 한 곳뿐이라는 성질(J3)이 여기서 값으로 드러난다: `w/EXTENT` 가 0 이면
    // 모든 로컬 좌표가 원점으로 떨어지고, 그것은 사라지는 것이 아니라 **한 점**이다.
    const px = projectPathPoints(DEFAULT_PATH, { x: 37, y: 61, w: 0, h: 0 });
    for (const cmd of px) {
      if (cmd.c === 'Z') continue;
      expect(cmd.x).toBe(37);
      expect(cmd.y).toBe(61);
    }
  });

  it('비유한 포인터 좌표는 히트 판정 입구에서 끊긴다 — 모든 비교가 거짓이 되지 않는다', () => {
    expect(hitTest([pathEl()], { x: Number.NaN, y: 70 }, PROJ, {})).toBeUndefined();
    expect(hitTest([pathEl()], { x: 80, y: Number.POSITIVE_INFINITY }, PROJ, {})).toBeUndefined();
  });
});

// --- 파서 쪽의 같은 입력 ---------------------------------------------------

describe('손상 입력은 요소를 죽이지 않는다 — 파서 갈래 (AC-E4)', () => {
  function parseOne(raw: Record<string, unknown>): CanvasElement | undefined {
    const node = parseCanvasConfig({ canvas: { ...DEFAULT_CANVAS_SIZE }, elements: [raw] }).elements[0];
    // SPEC-CANVAS-004 M1 — 최상위 원소가 `CanvasNode` 로 넓어졌다. 이 시험은 경로 요소만
    // 넣으므로 좁히기는 타입을 맞추는 일 하나뿐이다(단언은 한 글자도 바뀌지 않았다).
    return node === undefined || node.kind === 'group' ? undefined : node;
  }

  const CORRUPT: ReadonlyArray<readonly [name: string, path: unknown]> = [
    ['배열이 아님', { nope: true }],
    ['빈 배열', []],
    ['첫 명령이 M 이 아님', [{ c: 'L', x: 1, y: 2 }]],
    ['좌표가 NaN', [{ c: 'M', x: Number.NaN, y: Number.NaN }]],
    ['모르는 명령 문자', [{ c: 'Q', x: 1, y: 2 }]],
    ['명령이 상한을 넘음', Array.from({ length: MAX_PATH_COMMANDS + 50 }, () => ({ c: 'L', x: 1, y: 1 }))],
    ['원소가 전부 null', [null, null]],
  ];

  for (const [name, raw] of CORRUPT) {
    it(`${name} — 요소가 살아 있고 그릴 명령이 있다`, () => {
      const el = parseOne({ id: 'p1', kind: 'path', path: raw });
      expect(el, name).toBeDefined();
      expect(el?.kind).toBe('path');
      const cmds = (el as PathElement).path;
      // 씨앗 경로든 살아남은 명령이든 **첫 명령은 언제나 `M`** 이다 — 그래야 없는
      // 자리에서 선이 뻗지 않는다.
      expect(cmds.length, name).toBeGreaterThan(0);
      expect(cmds[0]?.c, name).toBe('M');
      expect(cmds.length, name).toBeLessThanOrEqual(MAX_PATH_COMMANDS);
    });

    it(`${name} — 그 결과를 그려도 NaN 이 없다`, () => {
      const el = parseOne({ id: 'p1', kind: 'path', path: raw }) as PathElement;
      const { ctx, calls } = recorder();
      drawElements(ctx, [el], {}, {}, PROJ);
      expect(drawnNumbers(calls).filter((n) => Number.isNaN(n)), name).toEqual([]);
      // 씨앗 폴백이 실제로 그려졌다 — "예외가 없다" 만으로는 빈 그림도 통과한다.
      expect(calls.some(([m]) => m === 'moveTo'), name).toBe(true);
    });
  }

  it('요소를 버리는 경우는 id 결측 하나뿐이다 (001 의 정책과 같다)', () => {
    expect(parseOne({ kind: 'path', path: [] })).toBeUndefined();
    expect(parseOne({ id: 'p1', kind: 'path' })).toBeDefined();
  });
});

// --- 기기 지역 저장 자료 ---------------------------------------------------

describe('팔레트 접힘 상태도 손으로 고쳐질 수 있다 — 관용 파서 (REQ-06)', () => {
  const cases: ReadonlyArray<readonly [name: string, raw: string]> = [
    ['JSON 이 아니다', '{{{'],
    ['배열이다', '[true, false]'],
    ['null 이다', 'null'],
    ['값이 불리언이 아니다', '{"general":"yes","basic":1,"arrow":null}'],
    ['모르는 키가 섞였다', '{"general":false,"hologram":true}'],
  ];

  for (const [name, raw] of cases) {
    it(`${name} — 예외 없이 기본값으로 떨어지고 아는 키만 살린다`, () => {
      globalThis.localStorage.setItem(PALETTE_STORAGE_KEY, raw);
      let state!: PaletteCollapseState;
      expect(() => {
        state = readCollapsed();
      }, name).not.toThrow();
      // 키 집합은 언제나 넷이다 — 저장 자료가 그 집합을 넓히지 못한다.
      expect(Object.keys(state).sort(), name).toEqual([...PALETTE_GROUP_IDS].sort());
      for (const v of Object.values(state)) expect(typeof v, name).toBe('boolean');
    });
  }

  it('읽을 수 있는 값 하나는 살아나고 나머지는 기본값이다', () => {
    globalThis.localStorage.setItem(PALETTE_STORAGE_KEY, '{"general":false,"basic":"nope"}');
    const state = readCollapsed();
    expect(state.general).toBe(false); // 저장된 값
    expect(state.basic).toBe(DEFAULT_COLLAPSED.basic); // 읽히지 않아 기본값
    expect(state.arrow).toBe(DEFAULT_COLLAPSED.arrow);
  });
});

// --- 불변식 J10 · J12 -----------------------------------------------------

const CANVAS_DIR = __dirname;

function readSource(rel: string): string {
  return fs.readFileSync(path.resolve(CANVAS_DIR, rel), 'utf8');
}

describe('불변식 J10 — `CanvasProjection` 은 두 칸이다', () => {
  it('그 인터페이스의 멤버가 정확히 `stage` · `canvas` 다 (형상 판정)', () => {
    const src = readSource('canvasGeometry.ts');
    const block = /export interface CanvasProjection \{([\s\S]*?)\n\}/.exec(src)?.[1];
    expect(block, '인터페이스 선언을 찾지 못했다 — 개명되었다면 이 가드를 먼저 옮겨라').toBeTypeOf(
      'string',
    );
    // 주석을 걷고 `이름:` 꼴만 센다. 이름 열거가 아니라 **개수와 이름 집합**을 함께 본다 —
    // 셋째 칸(예: `zoom`)이 들어오면 006 의 배율 결정이 이 인터페이스로 새는 것이다.
    const members = [...(block ?? '').matchAll(/^\s*([a-zA-Z_$][\w$]*)\s*[?]?\s*:/gm)].map(
      (m) => m[1],
    );
    expect(members).toEqual(['stage', 'canvas']);
  });

  it('값으로도 두 칸이다 — 투영 함수가 셋째 칸을 읽지 않는다', () => {
    expect(Object.keys(PROJ).sort()).toEqual(['canvas', 'stage']);
    // 셋째 칸을 실어도 결과가 같다(= 아무도 읽지 않는다).
    const spiked = { ...PROJ, zoom: 4 } as unknown as CanvasProjection;
    expect(projectBox({ ...BOX }, spiked)).toEqual(projectBox({ ...BOX }, PROJ));
  });
});

describe('불변식 J12 — 캔버스 작업이 `PanelSettingsDialog.tsx` 로 새지 않는다 (006 I7)', () => {
  // **행 수 등식을 걷어냈다(0.3.0).** 이 자리가 오래 들고 있던 자는 `toBe(8371)` →
  // `toBe(8373)` 이었다. 그 수는 불변식이 아니라 **대리**다 — 막으려는 것은 캔버스 작업이
  // 이 파일로 새는 것인데, 등식은 캔버스와 무관한 편집에도(그리고 **줄이는** 편집에도)
  // 울린다. 실제로 패널 색상 자유 입력 두 줄(import 1 + JSX 1)에 울렸고, 다음 사람은 왜
  // 울렸는지 보는 대신 수를 고쳤다. **재기준되어야 하는 가드는 보호처럼 보이는 소음이다.**
  //
  // 대신 불변식을 **직접** 잰다. 자는 `src/test/panelSettingsDialogCanvasSurface.ts` 에 있고
  // 007 K13 과 **같은 자를 나눠 쓴다**(허용목록이 두 벌이면 갈라진다 — 지금 SPEC 과 시험이
  // 갈라진 것이 바로 그 부류의 사고다).
  it('`panels/canvas/` 에서 들이는 모듈과 이름이 허용목록 그대로다', () => {
    expect(
      actualCanvasImports(),
      '006 I7 이 깨졌다 — 다이얼로그의 캔버스 수입이 허용목록과 다르다.\n' +
        '늘었다면 캔버스 작업이 다이얼로그로 샌 것이다: 그 코드를 `panels/canvas/` 안으로 옮겨라.\n' +
        '다이얼로그만 펼 수 있는 자리라면(도크 자리처럼) `src/test/panelSettingsDialogCanvasSurface.ts` 의\n' +
        '`ALLOWED_CANVAS_IMPORTS` 에 적고, **왜 그 코드가 캔버스 밖에 사는지**를 SPEC 에 남겨라.',
    ).toEqual(allowedCanvasImports());
  });

  it('수입 없이 펴 넣은 캔버스 코드도 없다 — `Canvas…` 이름이 허용목록에서만 나온다', () => {
    // 수입 허용목록이 못 보는 한 가지가 **인라인**이다. 그 자리를 등식이 막고 있었으므로,
    // 등식을 걷는 대신 같은 구멍을 **파생**으로 막는다 — 고정 열거가 아니라 허용목록에서
    // 뽑으므로, 새로 이름 붙인 캔버스 표면(`CanvasScratchpad` 든 `CanvasFoo` 든)도 걸린다.
    expect(
      actualCanvasIdentifiers(),
      '허용목록에 없는 `Canvas…` 이름이 다이얼로그 본문에 서 있다 — 수입하지 않고 펴 넣은 캔버스 코드다.\n' +
        '캔버스 어휘를 쓰는 코드는 `panels/canvas/` 에 산다. 그리로 옮기고 여기서는 수입만 해라.',
    ).toEqual(allowedCanvasIdentifiers());
  });

  it('008 이 그 파일에 심은 것은 도크 자리 하나뿐이다 — 카탈로그도 서랍도 들어가지 않았다', () => {
    const src = fs.readFileSync(
      path.resolve(CANVAS_DIR, '../../PanelSettingsDialog.tsx'),
      'utf8',
    );
    for (const token of ['CanvasShapeCatalog', 'CanvasScratchpad', 'SHAPE_CATALOG', 'scratchpad']) {
      expect(src.includes(token), token).toBe(false);
    }
  });
});

// --- AC-E5 config 크기 실측 -----------------------------------------------

/** 곡선을 쓰는 도형(명령에 `C` 가 있다)과 다각형을 가른다. */
const CURVED = SHAPE_CATALOG.filter((s) => s.path.some((c) => c.c === 'C'));
const POLYGONAL = SHAPE_CATALOG.filter((s) => !s.path.some((c) => c.c === 'C'));

/** 대시보드 snapshot PUT 상한(`internal/api/handler/dashboard.go` — spec.md 가 실측). */
const SNAPSHOT_BUDGET_BYTES = 256 * 1024;

/** 실측한 값. 시험 이름이 아니라 **여기**가 그 수의 자리다. */
function measuredConfigBytes(): { bytes: number; ratio: number } {
  const elements: CanvasElement[] = [];
  for (let i = 0; i < 20; i++) {
    const shape = POLYGONAL[i % POLYGONAL.length];
    if (shape === undefined) continue;
    elements.push({
      id: `el-${elements.length + 1}`,
      kind: 'path',
      geometry: { x: 10 + i * 7, y: 20 + i * 5, w: 120, h: 80 },
      path: shape.path.map((c) => ({ ...c })),
      catalog_id: shape.id,
      style: { fill: '#3b82f6', stroke: '#1e3a8a', strokeWidth: 2 },
    });
  }
  for (let i = 0; i < 10; i++) {
    const shape = CURVED[i % CURVED.length];
    if (shape === undefined) continue;
    elements.push({
      id: `el-${elements.length + 1}`,
      kind: 'path',
      geometry: { x: 200 + i * 9, y: 30 + i * 11, w: 140, h: 100 },
      path: shape.path.map((c) => ({ ...c })),
      catalog_id: shape.id,
      style: { stroke: '#ef4444', strokeWidth: 3 },
    });
  }
  const bytes = new TextEncoder().encode(
    JSON.stringify({ canvas: { ...DEFAULT_CANVAS_SIZE }, elements }),
  ).length;
  return { bytes, ratio: bytes / SNAPSHOT_BUDGET_BYTES };
}

describe('config 크기 실측 (AC-E5 · 가정 A4 · 위험 R4)', () => {
  it('다각형 20 + 곡선 10 을 놓은 config 는 **13,380 바이트**이고 256KB 예산의 **5.10%** 다', () => {
    const { bytes, ratio } = measuredConfigBytes();
    // 수를 시험에 적는다 — 도형의 명령 수가 늘면 이 줄이 먼저 운다(위험 R4 의 신호는
    // "카탈로그를 줄이자" 가 아니라 "명령 수를 줄이자" 다).
    //
    // 요소 하나에 **446 바이트**다(13,380 ÷ 30). spec.md §config 크기가 어림한
    // "다각형 150~250B · 곡선 300~500B" 보다 다각형 쪽이 크다 — 어림이 세지 않은
    // `catalog_id` · `style` · `geometry` 가 요소마다 130B 남짓 붙기 때문이다.
    expect(bytes).toBe(13380);
    expect(Number((ratio * 100).toFixed(2))).toBe(5.1);
  });

  it('예산의 25% 를 넘지 않는다 — 넘으면 이 시험 이름이 거짓말이 된다', () => {
    expect(measuredConfigBytes().ratio).toBeLessThan(0.25);
  });

  it('곡선 도형은 정확히 7종이다 (가정 A2 — 30종 가운데)', () => {
    expect(CURVED).toHaveLength(7);
    expect(POLYGONAL).toHaveLength(23);
  });
});
