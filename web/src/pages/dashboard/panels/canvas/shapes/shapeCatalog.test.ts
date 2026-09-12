// 도형 카탈로그 30종 (SPEC-CANVAS-008 M6).
//
// 이 파일이 겨누는 함정 셋을 먼저 적는다.
//
// **하나 — 볼록한 고정 입력**(시험 규율 D5). 도형 안쪽 깊은 곳만 찍는 히트 시험은 바운딩
// 박스 판정으로도, 볼록 껍질 판정으로도 전부 초록이다. 그래서 여기서는 **볼록 껍질 안이면서
// 도형 밖**인 점을 도형마다 **찾아내** 그 점이 빗나가는지 잰다. 껍질로 재는 것에 뜻이 있다:
// 상자 안이면서 도형 밖인 점은 삼각형에도 있으므로(모서리) 그것만으로는 바운딩 박스 결함만
// 잡고 볼록 껍질 결함은 놓친다.
//
// **둘 — 목록을 손으로 적은 시험.** 30종 가운데 하나를 빠뜨려도 손으로 적은 목록은 조용히
// 통과한다. 그래서 이 파일의 순회는 전부 `SHAPE_CATALOG` 자신을 돈다. 개수·묶음별 정원은
// 따로 수로 못박아, 목록이 줄어드는 쪽으로 어긋나면 그 수가 운다.
//
// **셋 — 가정을 추정으로 확인하는 시험**(가정 A2). "곡선을 쓰는 것은 7종" 은 SPEC 이 분류로
// 적어 둔 값이며, 여기서는 명령 목록을 **세어서** 확인한다. 세어 본 결과가 SPEC 과 다르면
// 참인 것은 세어 본 쪽이다.
//
// 고정 입력은 이 SPEC 의 다른 경로 시험과 같다 — 축척 가로 1.6 · 세로 1.5(단위 축척이
// 아니고 두 축이 같지도 않다), 상자 `{x:37, y:61, w:160, h:90}`(원점 ≠ 0 · `x ≠ y` ·
// 비정사각 — D2·D3).
//
// @spec SPEC-CANVAS-008 REQ-02 · REQ-07 · AC-03 · AC-04

import { describe, expect, it } from 'vitest';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import type { BoxGeometry, PathElement } from '../canvasConfig';
import { projectBox, projectPathPoints, type CanvasProjection, type PxPoint } from '../canvasGeometry';
import { hitTest } from '../canvasHitTest';
import { drawElement, type DrawContext2D } from '../drawElement';
import { FLATTEN_TOLERANCE_PX, flattenPath, isInsidePath } from './pathFlatten';
import { MAX_PATH_COMMANDS, PATH_LOCAL_EXTENT } from './pathTypes';
import { SHAPE_CATALOG, SHAPE_GROUPS, findShape, type ShapeCatalogEntry } from './shapeCatalog';

// --- 고정 입력 -----------------------------------------------------------

/** 축척 가로 1.6 · 세로 1.5. */
const PROJ: CanvasProjection = {
  stage: { width: 800, height: 600 },
  canvas: { width: 500, height: 400 },
};

/** 원점 ≠ 0 · `x ≠ y` · 비정사각. */
const BOX: BoxGeometry = { x: 37, y: 61, w: 160, h: 90 };

const NO_WIDTHS: Record<string, number> = {};

/** 묶음별 정원. 목록이 줄어드는 쪽으로 어긋나면 이 수가 운다. */
const GROUP_SIZES: Readonly<Record<string, number>> = { general: 10, basic: 12, arrow: 8 };

/**
 * 3차 베지어를 요구한다고 SPEC 이 분류한 7종. **이 배열은 기대값이지 사실이 아니다** —
 * 사실은 명령 목록을 세어 얻고, 아래 시험이 둘을 맞대어 본다.
 */
const SPEC_CUBIC_IDS: readonly string[] = [
  'roundedRect',
  'document',
  'cylinder',
  'callout',
  'actor',
  'cloud',
  'arrowCurved',
];

/**
 * **실측한** 오목 도형 목록 — 볼록 껍질 안이면서 도형 밖인 점을 실제로 찾을 수 있는 것들.
 *
 * spec.md §카탈로그 30종은 오목 12종을 `cross · star4 · star5 · arrowRight ·
 * arrowLeftRight · arrowUpDown · arrowElbow · arrowNotched · chevron · callout · note ·
 * step` 으로 적었다. **실측은 12 가 아니라 16 이고, 목록도 다르다.**
 *
 *   - 빠지는 것 하나: `note`. 실루엣이 볼록이다 — 모서리를 앞으로 접으면 종이의 그림자는
 *     여전히 오각형이고, 접힘을 구멍으로 파면 접힌 종이가 아니라 ㄱ 자로 잘린 종이가 된다
 *     (`shapeCatalog.ts` §메모).
 *   - 더해지는 것 다섯: `process`(반대로 감은 세로 홈 둘) · `document`(물결의 올라오는 쪽) ·
 *     `actor`(겨드랑이와 가랑이) · `cloud`(혹과 혹 사이의 골) · `arrowHead`(꼬리가 파인 다트).
 *
 * **열린 도형(`arrowCurved`)은 여기 세지 않는다.** 닫힌 부분 경로가 하나도 없으면 내부 판정이
 * 언제나 거짓이라 껍질 안 아무 점이나 탐침이 되고, 그것은 오목이 아니라 **열림**이라는 다른
 * 성질이다. 그 성질은 아래 제 시험이 따로 잰다.
 *
 * 이 목록을 여기 적어 두는 것은 **줄어드는 쪽을 막기 위해서**다. 카탈로그를 고치다 오목
 * 도형이 볼록해지면(예: 십자의 겨드랑이를 메우면) 이 목록이 울고, 그때 잃은 것은 도형 하나가
 * 아니라 REQ-07 의 가드다 — 바운딩 박스 판정과 갈라지는 자리가 그만큼 줄기 때문이다.
 */
const CONCAVE_IDS: readonly string[] = [
  'process',
  'document',
  'step',
  'callout',
  'actor',
  'cross',
  'star4',
  'star5',
  'cloud',
  'arrowRight',
  'arrowLeftRight',
  'arrowUpDown',
  'arrowElbow',
  'chevron',
  'arrowNotched',
  'arrowHead',
];

/**
 * 오목 탐침이 도형의 모든 변에서 떨어져 있어야 하는 거리(px).
 *
 * `canvasHitTest.HIT_TOLERANCE_PX` 가 6 이므로 그보다 커야 탐침이 **선에 걸려서** 잡히는
 * 일이 없다. 1px 만 더 두는 것에 뜻이 있다 — 여유를 크게 잡으면 좁은 오목 지역(처리의 세로
 * 홈 · 문서의 물결)이 탐침을 못 만들어 오목이 아닌 것으로 잘못 세어진다.
 */
const PROBE_MARGIN_PX = 7;

// --- 기록 스텁 -----------------------------------------------------------

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  return {
    calls,
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {
      calls.push(['beginPath']);
    },
    rect(x, y, w, h) {
      calls.push(['rect', x, y, w, h]);
    },
    ellipse(x, y, rx, ry) {
      calls.push(['ellipse', x, y, rx, ry]);
    },
    moveTo(x, y) {
      calls.push(['moveTo', x, y]);
    },
    lineTo(x, y) {
      calls.push(['lineTo', x, y]);
    },
    closePath() {
      calls.push(['closePath']);
    },
    bezierCurveTo(a, b, c, d, e2, f) {
      calls.push(['bezierCurveTo', a, b, c, d, e2, f]);
    },
    stroke() {
      calls.push(['stroke']);
    },
    fill() {
      calls.push(['fill']);
    },
    fillText() {},
    measureText(text: string) {
      return { width: text.length * 10 };
    },
    clearRect() {},
    fillRect() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'middle',
  };
}

// --- 순수 도우미 ---------------------------------------------------------

function element(entry: ShapeCatalogEntry): PathElement {
  return {
    id: entry.id,
    kind: 'path',
    geometry: { ...BOX },
    // 카탈로그의 배열은 얼려 있다. 요소는 언제나 **사본**을 든다(REQ-02).
    path: entry.path.map((cmd) => ({ ...cmd })),
    catalog_id: entry.id,
    style: { fill: '#3b82f6', stroke: '#3b82f6', strokeWidth: 2 },
  };
}

/** 도형을 고정 상자 위에서 평탄화한 폴리라인들. */
function flatten(entry: ShapeCatalogEntry) {
  return flattenPath(projectPathPoints(entry.path, projectBox(BOX, PROJ)), FLATTEN_TOLERANCE_PX);
}

/** 평탄화 결과의 모든 점(껍질과 상자를 여기서 얻는다). */
function allPoints(entry: ShapeCatalogEntry): PxPoint[] {
  return flatten(entry).flatMap((s) => [...s.points]);
}

/**
 * 볼록 껍질(Andrew monotone chain). **구현에서 빌려 오지 않고 시험이 스스로 센다** —
 * 껍질을 재는 코드가 저장소에 없으므로 빌려 올 것도 없지만, 있더라도 빌리면 같은 셈을
 * 두 번 쓰는 셈이 된다.
 */
function convexHull(pts: readonly PxPoint[]): PxPoint[] {
  const sorted = [...pts].sort((a, b) => a.x - b.x || a.y - b.y);
  if (sorted.length < 3) return sorted;
  const cross = (o: PxPoint, a: PxPoint, b: PxPoint): number =>
    (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x);
  const half = (input: readonly PxPoint[]): PxPoint[] => {
    const out: PxPoint[] = [];
    for (const p of input) {
      while (out.length >= 2) {
        const a = out[out.length - 2];
        const b = out[out.length - 1];
        if (a === undefined || b === undefined || cross(a, b, p) > 0) break;
        out.pop();
      }
      out.push(p);
    }
    out.pop();
    return out;
  };
  return [...half(sorted), ...half([...sorted].reverse())];
}

/** 점이 볼록 껍질 **안**인가(경계 포함하지 않는다). */
function insideHull(hull: readonly PxPoint[], p: PxPoint): boolean {
  let sign = 0;
  for (let i = 0; i < hull.length; i++) {
    const a = hull[i];
    const b = hull[(i + 1) % hull.length];
    if (a === undefined || b === undefined) return false;
    const cr = (b.x - a.x) * (p.y - a.y) - (b.y - a.y) * (p.x - a.x);
    if (cr === 0) return false;
    const s = cr > 0 ? 1 : -1;
    if (sign === 0) sign = s;
    else if (sign !== s) return false;
  }
  return sign !== 0;
}

/** 점–선분 거리. `canvasHitTest` 의 그것과 같은 셈이지만 시험이 제 것을 갖는다. */
function distanceToSegment(a: PxPoint, b: PxPoint, p: PxPoint): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const lenSq = dx * dx + dy * dy;
  if (lenSq === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / lenSq));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

/** 도형의 모든 변까지의 최소 거리. */
function distanceToOutline(entry: ShapeCatalogEntry, p: PxPoint): number {
  let best = Number.POSITIVE_INFINITY;
  for (const sub of flatten(entry)) {
    const pts = sub.points;
    const last = sub.closed ? pts.length : pts.length - 1;
    for (let i = 0; i < last; i++) {
      const a = pts[i];
      const b = pts[(i + 1) % pts.length];
      if (a === undefined || b === undefined) continue;
      best = Math.min(best, distanceToSegment(a, b, p));
    }
  }
  return best;
}

/**
 * `fill()` 의 규칙을 그대로 흉내낸다 — **열린 부분 경로도 암묵적으로 닫고** nonzero 로 센다.
 *
 * 이 함수가 따로 있는 이유가 곧 이 파일이 한 번 물린 자리다. `isInsidePath` 는 `Z` 로 닫힌
 * 부분 경로만 세므로(히트 판정의 규칙), 열린 부분 경로의 **감김 방향을 뒤집어도 눈 하나
 * 깜짝하지 않는다.** 그런데 화면을 칠하는 `fill()` 은 열린 부분 경로를 암묵적으로 닫고
 * nonzero 로 채우므로, 뒤집힌 감김은 **거기서 구멍이 된다.** 두 규칙이 갈라지는 이 틈에서
 * 재면, 감김을 뒤집는 변경이 "잡히는 자리" 로는 관측되지 않고 **화면에서만** 드러난다.
 *
 * 그래서 감김 가드는 히트 규칙이 아니라 **채움 규칙**으로 잰다.
 */
function insideFilled(entry: ShapeCatalogEntry, p: PxPoint): boolean {
  const side = (a: PxPoint, b: PxPoint): number =>
    (b.x - a.x) * (p.y - a.y) - (p.x - a.x) * (b.y - a.y);
  let wn = 0;
  for (const sub of flatten(entry)) {
    const pts = sub.points;
    for (let i = 0; i < pts.length; i++) {
      const a = pts[i];
      const b = pts[(i + 1) % pts.length];
      if (a === undefined || b === undefined) continue;
      if (a.y <= p.y) {
        if (b.y > p.y && side(a, b) > 0) wn++;
      } else if (b.y <= p.y && side(a, b) < 0) {
        wn--;
      }
    }
  }
  return wn !== 0;
}

/**
 * **볼록 껍질 안이면서 도형 밖**인 점을 찾는다. 없으면 `undefined` — 그때 그 도형은
 * 볼록이며, 오목 판정의 유일한 잣대가 이 탐색이다.
 *
 * 격자를 훑는 것에 뜻이 있다: 오목 지역이 어디인지 손으로 적으면 도형을 고칠 때마다 그
 * 좌표를 함께 고쳐야 하고, 고치는 것을 잊으면 시험은 **엉뚱한 자리를 재면서 초록**이 된다.
 */
function findConcaveProbe(entry: ShapeCatalogEntry): PxPoint | undefined {
  const pts = allPoints(entry);
  const hull = convexHull(pts);
  if (hull.length < 3) return undefined;
  const subpaths = flatten(entry);
  // 닫힌 부분 경로가 없으면 내부 판정이 언제나 거짓이라 껍질 안 아무 점이나 탐침이 된다.
  // 그것은 오목이 아니라 **열림**이므로 이 탐색에서 뺀다(아래 열린 도형 시험이 따로 잰다).
  if (!subpaths.some((s) => s.closed)) return undefined;
  const minX = Math.min(...pts.map((p) => p.x));
  const maxX = Math.max(...pts.map((p) => p.x));
  const minY = Math.min(...pts.map((p) => p.y));
  const maxY = Math.max(...pts.map((p) => p.y));
  const STEPS = 90;
  for (let i = 1; i < STEPS; i++) {
    for (let j = 1; j < STEPS; j++) {
      const p = {
        x: minX + ((maxX - minX) * i) / STEPS,
        y: minY + ((maxY - minY) * j) / STEPS,
      };
      if (isInsidePath(subpaths, p)) continue;
      if (!insideHull(hull, p)) continue;
      if (distanceToOutline(entry, p) < PROBE_MARGIN_PX) continue;
      return p;
    }
  }
  return undefined;
}

/** 도형의 **안쪽 깊은 곳** 한 점(잡혀야 하는 자리). */
function findInteriorProbe(entry: ShapeCatalogEntry): PxPoint | undefined {
  const pts = allPoints(entry);
  const subpaths = flatten(entry);
  const minX = Math.min(...pts.map((p) => p.x));
  const maxX = Math.max(...pts.map((p) => p.x));
  const minY = Math.min(...pts.map((p) => p.y));
  const maxY = Math.max(...pts.map((p) => p.y));
  const STEPS = 60;
  let best: PxPoint | undefined;
  let bestDepth = 0;
  for (let i = 1; i < STEPS; i++) {
    for (let j = 1; j < STEPS; j++) {
      const p = {
        x: minX + ((maxX - minX) * i) / STEPS,
        y: minY + ((maxY - minY) * j) / STEPS,
      };
      if (!isInsidePath(subpaths, p)) continue;
      const depth = distanceToOutline(entry, p);
      if (depth > bestDepth) {
        bestDepth = depth;
        best = p;
      }
    }
  }
  return best;
}

/** i18n 트리에서 점 표기 키를 조회한다. */
function resolve(tree: unknown, key: string): unknown {
  return key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
      tree,
    );
}

// --- 목록의 형상 ---------------------------------------------------------

describe('카탈로그의 형상 (SPEC-CANVAS-008 M6 · REQ-02)', () => {
  it('묶음 셋에 30종이 정원대로 들어 있다', () => {
    expect(SHAPE_GROUPS.map((g) => g.id)).toEqual(['general', 'basic', 'arrow']);
    for (const group of SHAPE_GROUPS) {
      expect(group.entries.length, `${group.id} 묶음의 정원`).toBe(GROUP_SIZES[group.id]);
      // 항목이 제 묶음을 제대로 가리키는가 — 복사해 붙이다 어긋나기 쉬운 자리다.
      for (const e of group.entries) expect(e.group, e.id).toBe(group.id);
    }
    expect(SHAPE_CATALOG.length).toBe(30);
  });

  it('id 가 유일하고 점이 없으며 로케일에 기대지 않는다', () => {
    const ids = SHAPE_CATALOG.map((e) => e.id);
    expect(new Set(ids).size).toBe(ids.length);
    for (const id of ids) {
      // 점이 들어가면 그 id 로 만든 i18n 키가 **어떤 경로로도 닿지 않는다**(프로젝트 규약).
      expect(id, `${id} 에 점이 있다`).not.toContain('.');
      expect(id, `${id} 가 영문 식별자가 아니다`).toMatch(/^[a-z][A-Za-z0-9]*$/);
    }
  });

  it('명령 목록이 서로 다르다 — 같은 도형을 두 이름으로 내지 않는다', () => {
    const seen = new Map<string, string>();
    for (const e of SHAPE_CATALOG) {
      const key = JSON.stringify(e.path);
      const prev = seen.get(key);
      expect(prev, `${e.id} 와 ${prev ?? ''} 의 명령 목록이 같다`).toBeUndefined();
      seen.set(key, e.id);
    }
  });

  it('명령 어휘가 넷뿐이고, 첫 명령이 `M` 이며, 좌표가 정수다', () => {
    for (const e of SHAPE_CATALOG) {
      expect(e.path.length, `${e.id} 의 명령 수`).toBeGreaterThan(0);
      // 상한을 넘으면 파서가 앞에서부터 잘라 도형이 반쯤 남는다.
      expect(e.path.length, `${e.id} 의 명령 수`).toBeLessThanOrEqual(MAX_PATH_COMMANDS);
      // 첫 명령이 `M` 이 아니면 **파서가 통째로 씨앗 경로로 떨어뜨린다** — 카탈로그에서
      // 놓은 도형이 저장 왕복 뒤 사각형이 되는 부류이며 화면에서만 드러난다.
      expect(e.path[0]?.c, `${e.id} 의 첫 명령`).toBe('M');
      for (const cmd of e.path) {
        expect(['M', 'L', 'C', 'Z'], `${e.id}`).toContain(cmd.c);
        const coords =
          cmd.c === 'C'
            ? [cmd.x1, cmd.y1, cmd.x2, cmd.y2, cmd.x, cmd.y]
            : cmd.c === 'Z'
              ? []
              : [cmd.x, cmd.y];
        for (const v of coords) {
          // 정수 격자 위의 값이어야 파서의 `coordinate()` 가 왕복에서 값을 바꾸지 않는다.
          expect(Number.isInteger(v), `${e.id} 의 좌표 ${v}`).toBe(true);
          expect(v, `${e.id} 의 좌표 ${v}`).toBeGreaterThanOrEqual(0);
          expect(v, `${e.id} 의 좌표 ${v}`).toBeLessThanOrEqual(PATH_LOCAL_EXTENT);
        }
      }
    }
  });

  it('`findShape` 는 모르는 id 와 결측에 대해 조용히 비운다', () => {
    expect(findShape('star5')?.id).toBe('star5');
    expect(findShape('nope')).toBeUndefined();
    expect(findShape(undefined)).toBeUndefined();
  });
});

// --- 가정 A2 — 인터페이스에 더한 둘이 왜 둘인가 --------------------------

describe('곡선을 요구하는 도형을 센다 (가정 A2 · AC-E2)', () => {
  it('3차 베지어를 쓰는 것이 정확히 7종이고, SPEC 이 이름 적은 그 7종이다', () => {
    const measured = SHAPE_CATALOG.filter((e) => e.path.some((c) => c.c === 'C')).map((e) => e.id);
    // 수와 이름을 **둘 다** 잰다. 수만 재면 다른 도형이 곡선을 얻고 이 도형이 잃어도 통과한다.
    expect(measured.length).toBe(7);
    expect([...measured].sort()).toEqual([...SPEC_CUBIC_IDS].sort());
  });

  it('나머지 23종은 `M`·`L`·`Z` 만으로 그려진다 — 그 23종에 인터페이스 변경이 없다', () => {
    const polygonal = SHAPE_CATALOG.filter((e) => !e.path.some((c) => c.c === 'C'));
    expect(polygonal.length).toBe(23);
    for (const e of polygonal) {
      expect(e.path.every((c) => c.c === 'M' || c.c === 'L' || c.c === 'Z'), e.id).toBe(true);
    }
  });

  it('닫히지 않은 도형은 하나뿐이다 — 그것이 D4 의 고정 입력이다', () => {
    const open = SHAPE_CATALOG.filter((e) => !e.path.some((c) => c.c === 'Z')).map((e) => e.id);
    expect(open).toEqual(['arrowCurved']);
  });
});

// --- 30종 전량 렌더 ------------------------------------------------------

describe('30종이 한 종도 빠짐없이 그려진다 (REQ-02 · AC-02)', () => {
  it.each(SHAPE_CATALOG.map((e) => [e.id, e] as const))('%s 를 그린다', (_id, entry) => {
    const ctx = makeRecorder();
    drawElement(ctx, element(entry), { fill: '#3b82f6', stroke: '#3b82f6', strokeWidth: 2 }, undefined, PROJ);
    const names = ctx.calls.map((c) => c[0]);

    expect(names[0]).toBe('beginPath');
    expect(names).toContain('moveTo');
    // 채움과 선을 **둘 다** 잰다 — 하나만 재면 스타일 한쪽이 조용히 빠져도 통과한다.
    expect(names).toContain('fill');
    expect(names).toContain('stroke');

    // 기록된 좌표가 전부 투영 상자 안이다. 로컬 좌표가 0..10000 이므로 이것이 참이어야
    // 하고, 참이 아니면 투영이 원점 항이나 축척 항을 빠뜨린 것이다(D2·D3).
    const box = projectBox(BOX, PROJ);
    for (const call of ctx.calls) {
      const [name, ...args] = call;
      if (name !== 'moveTo' && name !== 'lineTo' && name !== 'bezierCurveTo') continue;
      for (let i = 0; i < args.length; i += 2) {
        const x = args[i] as number;
        const y = args[i + 1] as number;
        expect(x, `${entry.id} 의 x`).toBeGreaterThanOrEqual(box.x - 1e-6);
        expect(x, `${entry.id} 의 x`).toBeLessThanOrEqual(box.x + box.w + 1e-6);
        expect(y, `${entry.id} 의 y`).toBeGreaterThanOrEqual(box.y - 1e-6);
        expect(y, `${entry.id} 의 y`).toBeLessThanOrEqual(box.y + box.h + 1e-6);
      }
    }
  });

  it('닫힌 도형은 `closePath` 를 기록하고 열린 도형은 기록하지 않는다 (D4)', () => {
    // 앞쪽만 재면 "언제나 닫는다" 는 결함이 통과하고, 뒤쪽만 재면 "한 번도 닫지 않는다" 가
    // 통과한다. **둘을 함께** 재는 것이 이 시험의 전부다.
    for (const entry of SHAPE_CATALOG) {
      const ctx = makeRecorder();
      drawElement(ctx, element(entry), { stroke: '#000', strokeWidth: 2 }, undefined, PROJ);
      const names = ctx.calls.map((c) => c[0]);
      const closes = names.filter((n) => n === 'closePath').length;
      const zs = entry.path.filter((c) => c.c === 'Z').length;
      expect(closes, `${entry.id} 의 closePath 기록 수`).toBe(zs);
    }
    const curved = findShape('arrowCurved');
    expect(curved).toBeDefined();
    if (curved === undefined) return;
    const ctx = makeRecorder();
    drawElement(ctx, element(curved), { stroke: '#000', strokeWidth: 2 }, undefined, PROJ);
    const names = ctx.calls.map((c) => c[0]);
    expect(names).not.toContain('closePath');
    expect(names).toContain('bezierCurveTo');
  });
});

// --- 오목 — 바운딩 박스와 갈라지는 자리 -----------------------------------

describe('오목 도형은 제 윤곽으로 잡힌다 (REQ-07 · AC-04 · D5)', () => {
  it('실측한 오목 목록이 기록과 같다', () => {
    const measured = SHAPE_CATALOG.filter((e) => findConcaveProbe(e) !== undefined).map((e) => e.id);
    expect(measured).toEqual([...CONCAVE_IDS]);
    // 카탈로그의 절반이 오목이라는 사실 자체가 REQ-07 의 근거다 — 이 수가 0 이 되면
    // 바운딩 박스 판정과 갈라지는 자리가 사라진다. **12 는 SPEC 이 적은 수이고 실측은 16**
    // 이므로, 하한을 12 로 두면 넷을 잃어도 조용하다. 실측한 수 그대로 못박는다.
    expect(measured.length).toBe(16);
  });

  it('열린 도형은 안쪽으로 잡히지 않고 선 위에서만 잡힌다', () => {
    // 열림은 오목과 다른 성질이라 위 목록에서 뺐다. 빼 놓고 재지 않으면 그 성질에 가드가
    // 하나도 없게 되므로 여기서 잰다 — spec.md §히트 테스트가 이름 적어 둔 그 문장이다.
    const entry = findShape('arrowCurved');
    expect(entry).toBeDefined();
    if (entry === undefined) return;

    const el = element(entry);
    const box = projectBox(BOX, PROJ);
    const subpaths = flatten(entry);
    // 닫힌 부분 경로가 하나도 없다 — 그래서 내부 판정이 참이 되는 점이 존재하지 않는다.
    expect(subpaths.some((s) => s.closed)).toBe(false);

    // 곡선 활 안쪽의 빈 자리(로컬 5000, 6500). 상자 안이고 껍질 안이지만 선에서 멀다.
    const hollow: PxPoint = {
      x: box.x + (5000 * box.w) / PATH_LOCAL_EXTENT,
      y: box.y + (6500 * box.h) / PATH_LOCAL_EXTENT,
    };
    expect(isInsidePath(subpaths, hollow)).toBe(false);
    expect(distanceToOutline(entry, hollow)).toBeGreaterThan(PROBE_MARGIN_PX);
    expect(hitTest([el], hollow, PROJ, NO_WIDTHS)).toBeUndefined();

    // 곡선 위의 한 점(명령 목록의 끝점 = 화살촉의 꼭짓점)은 잡힌다.
    const onLine: PxPoint = {
      x: box.x + (9200 * box.w) / PATH_LOCAL_EXTENT,
      y: box.y + (2400 * box.h) / PATH_LOCAL_EXTENT,
    };
    expect(hitTest([el], onLine, PROJ, NO_WIDTHS)?.nodeId).toBe('arrowCurved');
  });

  it.each(CONCAVE_IDS)('%s 의 오목 지역은 상자 안이지만 빗나간다', (id) => {
    const entry = findShape(id);
    expect(entry, id).toBeDefined();
    if (entry === undefined) return;

    const probe = findConcaveProbe(entry);
    expect(probe, `${id} 의 오목 탐침`).toBeDefined();
    if (probe === undefined) return;

    const el = element(entry);
    const box = projectBox(BOX, PROJ);

    // ① 탐침은 **바운딩 박스 안**이다 — 바운딩 박스 판정이라면 잘못 잡았을 자리다.
    expect(probe.x).toBeGreaterThan(box.x);
    expect(probe.x).toBeLessThan(box.x + box.w);
    expect(probe.y).toBeGreaterThan(box.y);
    expect(probe.y).toBeLessThan(box.y + box.h);

    // ② 그런데 빗나간다. 이 한 줄이 REQ-07 의 유일한 가드다.
    expect(hitTest([el], probe, PROJ, NO_WIDTHS), `${id} 의 오목 지역이 잡혔다`).toBeUndefined();

    // ③ 안쪽 깊은 곳은 잡힌다 — ②가 "아무것도 잡지 못한다" 로 통과하는 것을 막는다.
    const inside = findInteriorProbe(entry);
    expect(inside, `${id} 의 안쪽 탐침`).toBeDefined();
    if (inside === undefined) return;
    expect(hitTest([el], inside, PROJ, NO_WIDTHS)?.nodeId, `${id} 안쪽이 빗나갔다`).toBe(id);
  });

  it('반대로 감은 부분 경로는 구멍이고, 같이 감은 것은 구멍이 아니다', () => {
    // 감김 방향은 **채움 규칙으로만** 관측된다(`insideFilled` 머리말). 일부러 구멍을 낸
    // 곳(처리의 세로 홈)과 실수로 구멍이 되면 안 되는 곳(메모의 접힘 선 · 정육면체의 모서리
    // 선)을 **한 시험에서** 맞대어 본다 — 앞쪽만 재면 "언제나 구멍" 이, 뒤쪽만 재면
    // "구멍이 하나도 없다" 가 통과한다.
    const box = projectBox(BOX, PROJ);
    const toPx = (lx: number, ly: number): PxPoint => ({
      x: box.x + (lx * box.w) / PATH_LOCAL_EXTENT,
      y: box.y + (ly * box.h) / PATH_LOCAL_EXTENT,
    });

    const holed = findShape('process');
    expect(holed).toBeDefined();
    if (holed === undefined) return;
    // 처리의 왼쪽 홈 한가운데(로컬 x 1350) — 구멍이므로 칠해지지 않는다.
    expect(insideFilled(holed, toPx(1350, 5000))).toBe(false);
    // 홈 사이의 몸통(로컬 x 5000) 은 칠해진다.
    expect(insideFilled(holed, toPx(5000, 5000))).toBe(true);
    // 홈이 `Z` 로 닫혀 있으므로 **잡히는 자리에서도** 구멍이다 — 그린 그림과 잡히는 자리가
    // 갈라지지 않는다.
    expect(isInsidePath(flatten(holed), toPx(1350, 5000))).toBe(false);

    const note = findShape('note');
    expect(note).toBeDefined();
    if (note === undefined) return;
    // 메모의 접힘 삼각형 한가운데(로컬 8000, 2000) 는 **칠해져야 한다.** 접힘 선의 감김을
    // 뒤집으면 여기에 구멍이 뚫리고, 그 결함은 **히트 판정으로는 영영 보이지 않는다**
    // (접힘 선은 `Z` 가 없어 내부 판정에서 빠지기 때문이다).
    expect(insideFilled(note, toPx(8000, 2000))).toBe(true);

    const cube = findShape('cube');
    expect(cube).toBeDefined();
    if (cube === undefined) return;
    // 정육면체의 윗면 **모서리 선이 그리는 삼각형 안쪽**(로컬 6000, 1900)이 칠해진다.
    // 자리를 고른 근거를 적어 둔다: 윗면 아무 데나 찍으면 안 된다 — 뒤집힌 감김이 뚫는
    // 구멍은 정확히 그 삼각형 `(0,2500)-(7500,2500)-(10000,0)` 이므로, 그 밖을 찍은 탐침은
    // 감김이 뒤집혀도 눈 하나 깜짝하지 않는다(처음 고른 `(5000,1200)` 이 그 밖이었다).
    expect(insideFilled(cube, toPx(6000, 1900))).toBe(true);
    // 앞면 한가운데도 칠해진다 — 실루엣 자체가 뒤집히면 여기가 빈다.
    expect(insideFilled(cube, toPx(3500, 6000))).toBe(true);
  });
});

// --- 이름 ----------------------------------------------------------------

describe('이름은 키로만 있고 두 로케일에 모두 있다 (REQ-06)', () => {
  it('30종의 이름 키와 묶음 제목 키가 ko·en 양쪽에서 문자열로 잡힌다', () => {
    // `i18nKeyShape.test.ts` 의 `t('...')` 정규식은 **리터럴 호출만** 잡는다. 카탈로그의
    // 이름은 자료에서 나오는 계산된 키라 그 가드가 닿지 않으므로, 이 파일이 직접 센다.
    const keys = [
      ...SHAPE_GROUPS.map((g) => g.titleKey),
      ...SHAPE_CATALOG.map((e) => e.nameKey),
    ];
    for (const key of keys) {
      expect(typeof resolve(ko, key), `ko 에 ${key} 가 없다`).toBe('string');
      expect(typeof resolve(en, key), `en 에 ${key} 가 없다`).toBe('string');
      // 네임스페이스 객체를 가리키면 화면에 원문 키가 뜬다(그 형태의 결함이 배포된 적이 있다).
      expect(resolve(ko, key), key).not.toBe('');
      expect(resolve(en, key), key).not.toBe('');
    }
  });

  it('이름 키의 마지막 마디가 도형 id 다 — 둘이 갈라지면 이름이 엉뚱한 칸에 붙는다', () => {
    for (const e of SHAPE_CATALOG) {
      expect(e.nameKey.split('.').at(-1), e.id).toBe(e.id);
    }
  });
});
